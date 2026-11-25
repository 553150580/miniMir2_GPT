// 战斗收益、离线托管扣时和装备磨损的合并结算。按提交内容和来源水位重放结果，并在同一事务更新角色、回执与审计。

package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"time"
)

// 永久来源结果只保存资产事实，不保存临时战斗世界。
type SettlementReceipt struct {
	RuleVersion   uint64                    `bson:"rule_version"`               // 首次结算采用的玩法规则。
	ConfigHash    string                    `bson:"config_hash"`                // 首次结算采用的不可变配置摘要。
	HostSeconds   int64                     `bson:"host_seconds,omitempty"`     // 本来源实际扣除的托管整秒。
	Kills         int64                     `bson:"kills,omitempty"`            // 本来源实际确认的击杀次数。
	MonsterID     uint32                    `bson:"monster_id,omitempty"`       // 已确认击杀的真实怪物配置号。
	MapID         uint64                    `bson:"map_id,omitempty"`           // 击杀归属的原地图配置号。
	DropPoolID    uint32                    `bson:"drop_pool_id,omitempty"`     // 击杀时使用的原掉落池号。
	GrantedItem   bool                      `bson:"granted_item"`               // 本次原掉落是否进入角色背包。
	Recycling     EquipmentRecyclingReceipt `bson:"recycling"`                  // 回收的实例、保护跳过和金币结果。
	Experience    *ExperienceGrantReceipt   `bson:"experience_grant,omitempty"` // 经验转换、限制和迁移明细。
	EquipmentWear []EquipmentWearReceipt    `bson:"equipment_wear,omitempty"`   // 本次扣除耐久对应的原装备实例。
	WearDeathID   string                    `bson:"wear_death_id,omitempty"`    // 并入此次结算的死亡磨损标识。

	OrdinaryEquipment *OrdinaryEquipmentReceipt `bson:"ordinary_equipment,omitempty"` // 高品质候选实际发放或降档的永久事实。
	HostingKillBonus  *HostingKillBonus         `bson:"hosting_kill_bonus,omitempty"` // 击杀时冻结的托管卡直接收益。
}

// Settle 提交固定来源的战斗或托管事实，只返回已确认的角色状态。
// s 提供数据库，ctx 限定提交时长，a 提供围栏所需身份，q 是交给持久层的原结算事实。
func (s *Store) Settle(ctx context.Context, a Auth, q Settlement) (domain.Role, error) {
	// r 接收事务确认后的角色状态，err 反映确定失败或需要沿原来源查证的状态。
	r, _, err := s.SettleWithReceipt(ctx, a, q)
	return r, err
}

// SettleWithReceipt 只接受下一个收益来源，重复提交同一内容时查证并返回原回执。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，a 提供请求所属的账号、角色和会话代次，供事务围栏验证。
// q 封装已发生且待原子落账的击杀、托管扣时与装备磨损事实。
func (s *Store) SettleWithReceipt(ctx context.Context, a Auth, q Settlement) (domain.Role, Operation, error) {
	// out 保存事务确认后交给房间执行器的角色状态。
	var out domain.Role
	// receipt 保存首次结算的永久回执，重复请求从同一操作文档重放。
	var receipt Operation
	if q.SourceSeq <= 0 {
		return out, receipt, Fail(p.CodeInvalidArgument, "收益来源序号必须为正")
	}
	// raw 是收益事实的序列化字节，用于计算幂等比对所需的内容摘要，err 接收序列化过程中可能产生的错误，供后续失败分支处理。
	raw, err := json.Marshal(q)
	if err != nil {
		return out, receipt, err
	}
	// 在事务之外冻结来源身份和摘要，使驱动重试或结果查证使用同一事实。
	// digest 冻结收益内容摘要，防止同一来源序号被替换为不同事实。
	digest := Hash(raw)
	// id 以角色编号和收益来源序号构造永久回执和审计的共同主键。
	id := fmt.Sprintf("settle:%s:%d", a.RoleID, q.SourceSeq)
	// tc 保证围栏、角色属性和账本操作属于同一个 MongoDB 事务。
	err = s.guardedTx(ctx, a, func(tc context.Context) error {
		receipt = Operation{}
		// fence 携带必须和结算事实同一事务执行的运行组和账号条件写入，err 接收权限围栏写入集合过程中可能产生的错误，供后续失败分支处理。
		fence, err := s.fenceWrites(tc, a)
		if err != nil {
			return err
		}
		// r 是当前事务快照中的角色聚合，用于校验归属和持久收益水位。
		r, err := s.Role(tc, a.RoleID)
		if err != nil {
			return err
		}
		if err = s.checkRole(r, a); err != nil {
			return err
		}
		if r.SettlementReceiptStartSeq < 0 || r.SettlementReceiptStartSeq > r.SourceSeq {
			return Fail(p.CodeOperationConflict, "收益回执界限无效")
		}
		if r.SourceSeq >= q.SourceSeq {
			// 新来源由角色水位和事务内的唯一键保护；已到达水位时才读取永久回执。
			// err 接收写入权限围栏过程中可能产生的错误，供后续失败分支处理。
			if err := s.writeFence(tc, fence); err != nil {
				return err
			}
			// old 接收同一来源已经持久化的旧回执，等身份和内容核对后再交回。
			var old Operation
			err = s.DB.Collection("operations").FindOne(tc, bson.M{"_id": id}).Decode(&old)
			if err == nil {
				if old.Kind != "settlement" || old.RoleID != a.RoleID || old.SourceSeq != q.SourceSeq || old.Digest != digest ||
					old.Settlement == nil || !sameHostingKillBonus(old.Settlement.HostingKillBonus, q.HostingKillBonus) {
					return Fail(p.CodeOperationConflict, "同来源结算内容改变")
				}
				out, receipt = r, old
				return nil
			}
			if !errors.Is(err, mongo.ErrNoDocuments) {
				return err
			}
			if q.OrdinaryEquipment != nil || q.HostingKillBonus != nil || (r.SettlementReceiptStartSeq > 0 && q.SourceSeq >= r.SettlementReceiptStartSeq) {
				return Fail(p.CodeOperationConflict, "新收益来源缺少永久回执")
			}
			// 升级前的历史来源只有水位；不构造或补播不存在的回收结果。
			out = r
			return nil
		}
		if r.SourceSeq == math.MaxInt64 || r.SourceSeq+1 != q.SourceSeq {
			return Fail(p.CodeOperationConflict, "收益来源游标缺口")
		}
		// 新来源先基于当前角色算出完整资产变更，再与两道围栏、回执和审计合批。
		// before 是计算收益前的角色修订号，用作数据库的并发更新条件。
		before := r.Revision
		// effective 只供本事务应用实际结果，原请求身份和摘要不随可用额度变化。
		effective := q
		choice, err := s.resolveOrdinaryEquipment(tc, q)
		if err != nil {
			return err
		}
		if choice != nil {
			effective.Item = &choice.receipt.Item
		}
		// next 记录本次收益及扣时应用后的候选角色状态，details 保存本次实际产生的经验、掉落、回收、整秒扣减与磨损明细。
		next, details, err := applySettlement(r, effective, s.Cfg.Fixture)
		if err != nil {
			return err
		}
		if err = s.confirmOrdinaryEquipment(tc, choice, &details); err != nil {
			return err
		}
		// proposed 是候选永久回执，事务确认后才会作为结算结果返回。
		proposed := Operation{ID: id, Digest: digest, RoleID: r.ID, CommandSeq: r.EconomicSeq, SourceSeq: q.SourceSeq, Revision: next.Revision, Kind: "settlement", Created: time.Now(), Settlement: &details, Recycling: &details.Recycling}
		// eventAt 优先保留原战斗或计量事件时间，旧请求未附时间时补用当前时点。
		eventAt := q.EventAt
		if eventAt.IsZero() {
			eventAt = time.Now()
		}
		// audit 记录与候选永久回执同主键的来源证据和实际结算详情。
		audit := bson.M{"_id": id, "kind": "settlement", "role_id": r.ID, "source_seq": q.SourceSeq, "event_at": eventAt, "bound": q.Bound, "kills": q.Kills, "experience": q.Experience, "gold": q.Gold, "item": effective.Item, "host_seconds": q.HostSeconds, "hosting_session": q.HostingSession, "target_handle": q.TargetHandle, "target_life": q.TargetLife, "caster_handle": q.CasterHandle, "rule_version": details.RuleVersion, "config_hash": details.ConfigHash, "settlement": details, "at": time.Now()}
		if err = s.writeSettlement(tc, fence, next, before, proposed, audit); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return Fail(p.CodeOperationConflict, "收益来源回执或审计先于角色水位")
			}
			return err
		}
		out, receipt = next, proposed
		return nil
	})
	return out, receipt, err
}

// 新来源的两道围栏和三份结算文档在同一事务批量写入，逐项检查条件匹配。
// s 是调用该方法的持久层存储实例，ctx 向下传递当前请求的截止时间和取消信号，fence 是由运行组与账号组成的两项条件更新，置于批量写入的开头。
// next 是要写入的资产聚合，oldRevision 是角色条件替换的原版本，receipt 是与本次来源合批的永久回执。
// audit 是与永久回执共享来源键的审计文档。
func (s *Store) writeSettlement(ctx context.Context, fence []mongo.ClientBulkWrite, next domain.Role, oldRevision int64, receipt Operation, audit bson.M) error {
	// database 指定批量命令涉及的全部集合所在的业务库名。
	database := s.DB.Name()
	// writes 按顺序保存两道围栏、角色条件替换及回执和审计插入。
	writes := append(fence,
		mongo.ClientBulkWrite{Database: database, Collection: "roles", Model: mongo.NewClientReplaceOneModel().SetFilter(roleReplaceFilter(next, oldRevision)).SetReplacement(next)},
		mongo.ClientBulkWrite{Database: database, Collection: "operations", Model: mongo.NewClientInsertOneModel().SetDocument(receipt)},
		mongo.ClientBulkWrite{Database: database, Collection: "audit_outbox", Model: mongo.NewClientInsertOneModel().SetDocument(audit)},
	)
	// result 提供每项条件更新的匹配数与插入总数，用于决定本批次能否确认，err 接收批量写入过程中可能产生的错误，供后续失败分支处理。
	result, err := s.Client.BulkWrite(ctx, writes, options.ClientBulkWrite().SetVerboseResults(true))
	if err != nil {
		return transactionBulkWriteError(err)
	}
	if err = verifyFenceWrites(result); err != nil {
		return err
	}
	if result.UpdateResults[2].MatchedCount != 1 {
		return Fail(p.CodeOperationConflict, "角色版本变化")
	}
	if result.InsertedCount != 2 {
		return Fail(p.CodeServerBusy, "收益回执或审计写入未确认")
	}
	return nil
}

// applySettlement 计算新角色和回执；调用层决定是否把两者同事务写入。
// r 是应用前的角色聚合，q 固定待计算的原来源、托管扣时、击杀与磨损，f 提供当前玩法版本及配置快照。
func applySettlement(r domain.Role, q Settlement, f devconfig.Fixture) (domain.Role, SettlementReceipt, error) {
	// details 建立带初始值的结算回执，供本段继续处理。
	details := SettlementReceipt{
		RuleVersion: f.RuleVersion, ConfigHash: f.Hash, HostSeconds: q.HostSeconds, Kills: q.Kills,
		MonsterID: q.MonsterID, MapID: q.MapID, DropPoolID: q.DropPoolID,
		Recycling: EquipmentRecyclingReceipt{PolicyRevision: r.AutoRecycle.Revision, Policy: policySnapshot(r.AutoRecycle), Trigger: 1},
	}
	if q.HostSeconds < 0 || r.HostingSeconds < q.HostSeconds || q.Kills < 0 || r.Level == 0 || r.Revision == math.MaxInt64 {
		return r, details, Fail(p.CodeOperationConflict, "托管时长或角色结算状态无效")
	}
	if err := validateHostingKillBonus(q, &f); err != nil {
		return r, details, err
	}
	details.HostingKillBonus = q.HostingKillBonus
	if f.RuleVersion >= 19 {
		// code 表示旧角色的回收策略是否仍满足当前规则。
		if _, code := policyFromProto(r.AutoRecycle.Proto(), f); code != 0 || r.AutoRecycle.Revision < 0 {
			return r, details, Fail(p.CodeOperationConflict, "角色回收策略无效")
		}
	}
	// out 从装备磨损后的候选角色继续计算其他收益，wear 保存磨损前后的实例快照，err 使无效事实整体失败。
	out, wear, err := applyEquipmentWear(r, q.EquipmentWear, q.WearDeathID, &f)
	if err != nil {
		return r, details, err
	}
	details.EquipmentWear, details.WearDeathID = wear, q.WearDeathID
	out.SourceSeq = q.SourceSeq
	if f.RuleVersion >= 19 && out.SettlementReceiptStartSeq == 0 {
		out.SettlementReceiptStartSeq = q.SourceSeq
	}
	out.HostingSeconds -= q.HostSeconds
	// actualExperience 记录按版本规则处理后实际授予角色的经验，用于写入战报。
	actualExperience := q.Experience
	if f.RuleVersion >= 20 {
		// contract 固定经验、等级和溢出上限，e 保存当前配置无法构造成长合同的错误。
		contract, e := GrowthContract(f)
		if e != nil {
			return r, details, e
		}
		out, details.Experience, err = experienceWithContract(out, q.Experience, contract)
		if err != nil {
			return r, details, err
		}
		actualExperience = details.Experience.ActualGranted
	} else {
		out.Experience, err = add(out.Experience, q.Experience)
		if err != nil {
			return r, details, err
		}
		for int(out.Level) <= len(f.Progression.ExpPerLevel) {
			// required 是当前等级升一级需要消耗的经验。
			required := f.Progression.ExpPerLevel[out.Level-1]
			if required <= 0 || out.Experience < required {
				break
			}
			if out.Level == math.MaxUint32 {
				return r, details, Fail(p.CodeOperationConflict, "角色等级耗尽")
			}
			out.Experience -= required
			out.Level++
		}
	}
	// 先尝试回收现有候选物品，再处理新掉落，尽可能为原掉落腾出背包格。
	if q.Kills > 0 || q.Item != nil {
		out, err = recycleExisting(out, f, &details.Recycling)
		if err != nil {
			return r, details, err
		}
	}
	if q.Item != nil {
		// item 是补齐耐久后用于回收判定或发到背包的原掉落。
		item := InitializeItemDurability(*q.Item, &f)
		q.Item = &item
		// recycled 表示该掉落已经折算成回收金币，后续无需占用库存。
		recycled := false
		if f.RuleVersion >= 19 && autoRecycleMatches(out, *q.Item, f) {
			// gold 是可回收时的应发金币，reason 非空表示受保护或其他拒绝。
			gold, reason := recyclePrice(out, *q.Item, f)
			if reason == "" {
				if err = creditRecycled(&out, &details.Recycling, *q.Item, gold); err != nil {
					return r, details, err
				}
				recycled = true
			} else {
				details.Recycling.Skipped = append(details.Recycling.Skipped, RecycleSkip{q.Item.ID, reason})
			}
		}
		if !recycled {
			if len(out.Items) < out.Capacity {
				out.Items = append(append([]domain.Item(nil), out.Items...), *q.Item)
				details.GrantedItem = true
			} else {
				out.PickupStopped = true
				out.LastDiscardItem = q.Item.ItemID
				out.LastDiscardBound = q.Item.Bound
				details.Recycling.DiscardedCount = 1
				details.Recycling.DiscardReason = "bag_full"
			}
		}
	}
	if len(out.Items) < out.Capacity {
		out.PickupStopped = false
	}
	if q.Bound {
		out.Wallet.BoundGold, err = add(out.Wallet.BoundGold, q.Gold)
	} else {
		out.Wallet.Gold, err = add(out.Wallet.Gold, q.Gold)
	}
	if err != nil {
		return r, details, err
	}
	if q.Kills > 0 {
		out.NoEquipmentKills = q.NoEquipmentKills
	}
	// 托管战报按会话聚合，切换到新会话时从空的战报开始。
	if q.HostingSession != "" {
		if out.Report.SessionID != q.HostingSession {
			out.Report = domain.Report{SessionID: q.HostingSession, Drops: []domain.Item{}}
		}
		// report 保存当前处理所用的报告或战报。
		report := out.Report
		// 累计值全部检查；未知提交重试读取原回执，不会再次累计。
		// fields 初始化用作字段集合的临时结构列表。
		fields := []struct {
			// dst 记录“dst”，使用数值表示。
			dst *int64
			// delta 记录增量，使用数值表示。
			delta int64
		}{
			{&report.UsedSeconds, q.HostSeconds}, {&report.Kills, q.Kills}, {&report.Experience, actualExperience}, {&report.RawExperience, q.Experience}, {&report.Sequence, 1},
			{&report.RecycleCount, int64(len(details.Recycling.Recycled))}, {&report.RecycleGold, details.Recycling.Gold}, {&report.RecycleBoundGold, details.Recycling.BoundGold}, {&report.DiscardCount, details.Recycling.DiscardedCount},
			{&report.Earned.Gold, details.Recycling.Gold}, {&report.Earned.BoundGold, details.Recycling.BoundGold},
		}
		if details.Experience != nil {
			fields = append(fields, struct {
				// dst 记录“dst”，使用数值表示。
				dst *int64
				// delta 记录增量，使用数值表示。
				delta int64
			}{&report.RejectedExperience, details.Experience.Rejected}, struct {
				// dst 记录“dst”，使用数值表示。
				dst *int64
				// delta 记录增量，使用数值表示。
				delta int64
			}{&report.PooledExperience, details.Experience.ToPool})
		}
		// field 接收遍历字段集合时当前的元素。
		for _, field := range fields {
			*field.dst, err = add(*field.dst, field.delta)
			if err != nil {
				return r, details, err
			}
		}
		if q.Bound {
			report.Earned.BoundGold, err = add(report.Earned.BoundGold, q.Gold)
		} else {
			report.Earned.Gold, err = add(report.Earned.Gold, q.Gold)
		}
		if err != nil {
			return r, details, err
		}
		if q.Item != nil {
			report.TotalDropCount, err = add(report.TotalDropCount, 1)
			if err != nil {
				return r, details, err
			}
		}
		if details.GrantedItem && len(report.Drops) < 100 {
			report.Drops = append(append([]domain.Item(nil), report.Drops...), *q.Item)
		}
		if details.Recycling.DiscardedCount > 0 {
			report.LastDiscardReason = details.Recycling.DiscardReason
		}
		if q.StopReason != 0 {
			report.StopReason = q.StopReason
		}
		// 旧报告3保留历史；真正耗尽时不能让旧容量原因遮住新终态。原回执不改写。
		if out.HostingSeconds == 0 && (report.StopReason == 0 || report.StopReason == 3) {
			report.StopReason = 1
		}
		out.Report = report
	}
	out = advanceSettlementTasks(r, out, &f, q, details)
	out.Revision++
	return out, details, nil
}
