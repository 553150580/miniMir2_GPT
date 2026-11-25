// 资产命令和收益请求的持久化入口。以操作编号、内容摘要、角色版本和经济命令水位处理业务拒绝与幂等重放，并在同一事务应用资产变更。

package persistence

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"strconv"
	"time"
)

// AssetRequest 是游戏逻辑提交给持久层的已路由资产命令。
type AssetRequest struct {
	Kind             string                  // 业务分派键，决定应用哪类资产命令。
	Fingerprint      string                  // 稳定请求摘要，用于拒绝相同操作编号的不同内容。
	Context          *pb.OperationContext    // 客户端操作编号、命令序号与预期版本。
	ItemID           string                  // 单物品操作选中的库存实例编号。
	ItemIDs          []string                // 批量操作选择的库存实例编号列表。
	Slot             uint32                  // 换装或卸装指定的装备槽。
	Benefit          uint32                  // 当前领取托管时长所请求的权益类型。
	ValidRevive      bool                    // 房间判断本次复活选择是否有对应的有效现场。
	ReviveDeathID    string                  // 请求复活的原死亡身份。
	ItemDefinitionID uint32                  // 需要和物品实例一致的配置物品编号。
	SceneEpoch       uint64                  // 药效等临时结果所属场景代次。
	PotionGateCode   uint32                  // 房间对用药条件的预先判定。
	SkillID          uint32                  // 学习、配置耗材或实际施法对应的技能。
	PreviewID        string                  // 提交时必须复核的短期技能资产计划标识。
	ItemVersion      uint64                  // 客户端或房间选定物品时的实例版本。
	CasterLife       uint64                  // 需要应用耗材效果的施法者生命代次。
	CastGateCode     uint32                  // 房间对技能与耗材使用条件的预先判定。
	Unequip          bool                    // 装备命令是否表示卸下槽位物品。
	SourceSeq        int64                   // 服务端自动资产命令的独立来源水位。
	AutoRecycle      *pb.AutoRecyclePolicy   // 待提交的自动回收策略。
	ApplyExisting    bool                    // 保存策略时是否同时尝试回收现有物品。
	Locked           bool                    // 库存实例要切换到的锁定状态。
	Warehouse        *pb.WarehouseAction     // 仓库动作的完整协议参数。
	Vendor           *pb.VendorCommitRequest // NPC 买卖或修理的报价提交参数。
	VendorShop       string                  // 房间根据 NPC 与当前地图确定的商店身份。

	TaskCodex *pb.TaskCodexClaimRequest // 携带待领取定义及当日资格的完整请求。
}

// AssetAutoUsePotion 区分服务端自动扣药和玩家主动用药。
const AssetAutoUsePotion = "auto_use_item"

// add 安全增加非负数量，超出 int64 范围时拒绝此次业务变更。
// a 是已有余额或计数，b 是本次待累加的非负增量。
func add(a, b int64) (int64, error) {
	if b < 0 || a > math.MaxInt64-b {
		return a, Fail(p.CodeInvalidArgument, "整数余额溢出")
	}
	return a + b, nil
}

// Asset 执行资产命令，面向无需读取分类回执的调用者。
// s 提供业务存储，ctx 限定执行时长，a 提供账号与角色身份，q 携带本次资产请求。
func (s *Store) Asset(ctx context.Context, a Auth, q AssetRequest) (domain.Role, *pb.OperationResult, error) {
	// role 是事务确认的角色状态，result 是协议操作结果，err 区分事务失败与确定的业务拒绝。
	role, result, _, err := s.AssetWithReceipt(ctx, a, q)
	return role, result, err
}

// 回执保留首次提交的原命令序号；OperationResult仍返回客户端需要的当前经济水位。
// s 提供业务存储，ctx 约束排队与提交，a 用于运行、账号和会话围栏，q 保留原操作身份与参数。
func (s *Store) AssetWithReceipt(ctx context.Context, a Auth, q AssetRequest) (domain.Role, *pb.OperationResult, Operation, error) {
	// out 保存当前角色状态，提交确认后替换成该操作结果。
	var out domain.Role
	// receipt 保存首次确定的永久操作回执，兼供相同请求重放。
	var receipt Operation
	// result 返回客户端核对所需的操作编号、当前版本和经济序号。
	result := &pb.OperationResult{}
	if q.Context == nil || len(q.Context.OpId) != 16 {
		return out, result, receipt, Fail(p.CodeInvalidArgument, "资产请求缺少16字节op_id")
	}
	result.OpId = q.Context.OpId
	var taskEventAt time.Time // 事务重试在同一次任务处理内采用同一个运营时点。
	if s.Cfg.Fixture.TasksCodex != nil {
		taskEventAt = time.Now()
	}
	// 经济命令和服务端自动来源使用不同的序号与操作命名空间。
	// opID 标识当前业务使用的操作。
	opID := "asset:" + a.RoleID + ":" + domain.Hex(q.Context.OpId)
	if q.SourceSeq > 0 {
		// source 是自动施法材料或自动补蓝的操作键前缀。
		var source string
		switch q.Kind {
		case "cast_material":
			source = "source_cast:"
		case AssetAutoUsePotion:
			source = "source_potion:"
		}
		if source == "" || q.Context.CommandSeq != 0 {
			return out, result, receipt, Fail(p.CodeInvalidArgument, "自动资产操作必须使用独立来源流水")
		}
		opID = source + a.RoleID + ":" + domain.Hex(q.Context.OpId)
	} else if q.Kind == AssetAutoUsePotion {
		return out, result, receipt, Fail(p.CodeInvalidArgument, "自动用药缺少来源流水")
	}
	// e 保存整个事务的执行错误，tc 将回调里的所有数据库操作纳入同一次事务。
	e := s.guardedTx(ctx, a, func(tc context.Context) error {
		receipt = Operation{}
		// e 保存运行组或账号围栏未通过时的错误，提前终止本次事务。
		if e := s.fence(tc, a); e != nil {
			return e
		}
		// r 是当前事务快照中的角色聚合，e 保存读取失败原因。
		r, e := s.Role(tc, a.RoleID)
		if e != nil {
			return e
		}
		if e = s.checkRole(r, a); e != nil {
			return e
		}
		out = r
		result.NewRevision = uint64(r.Revision)
		result.CommandSeq = uint64(r.EconomicSeq)
		// old 保存相同操作键已有的永久回执，供幂等核对。
		var old Operation
		e = s.DB.Collection("operations").FindOne(tc, bson.M{"_id": opID}).Decode(&old)
		if e == nil {
			if old.Digest != q.Fingerprint || q.SourceSeq > 0 && old.SourceSeq != q.SourceSeq {
				return Fail(p.CodeOperationConflict, "同op_id请求内容改变")
			}
			result.CommandSeq = uint64(r.EconomicSeq)
			result.NewRevision = uint64(r.Revision)
			result.ErrorCode = old.Code
			receipt = old
			return nil
		}
		if !errors.Is(e, mongo.ErrNoDocuments) {
			return e
		}
		// 读取已有回执后才检查下一序号和期望版本，允许重放早已提交的原命令。
		// sequenceValid 检查请求是否恰好推进其所属流水，玩家命令用 EconomicSeq，服务端自动事实用 SourceSeq。
		sequenceValid := q.SourceSeq == 0 && q.Context.CommandSeq == uint64(r.EconomicSeq+1) || q.SourceSeq > 0 && q.SourceSeq == r.SourceSeq+1
		if r.Revision == math.MaxInt64 || q.SourceSeq == 0 && r.EconomicSeq == math.MaxInt64 || q.SourceSeq > 0 && r.SourceSeq == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "资产序号已耗尽")
		}
		if !sequenceValid || q.Context.ExpectedRevision != uint64(r.Revision) || q.Context.ExpectedRuleVersion != s.Cfg.Fixture.RuleVersion {
			return Fail(p.CodeOperationConflict, "操作序列/版本变化，请刷新后保留原操作身份")
		}
		// oldRevision 保存修订号，用于当前步骤中的原状态修订号。
		oldRevision := r.Revision
		// businessCode 保存本次确定的业务结果，零表示业务操作成功。
		businessCode := uint32(0)
		// potion 记录药水，使用药水回执表示。
		var potion *PotionReceipt
		// skillDetails 保存已经应用的技能材料、费用和产物计划。
		var skillDetails *skillPlan
		// casting 记录施法，使用施法回执表示。
		var casting *CastingReceipt
		// recycling 记录自动回收，使用装备自动回收回执表示。
		var recycling *EquipmentRecyclingReceipt
		// equipment 记录装备，使用装备回执表示。
		var equipment *EquipmentReceipt
		// warehouse 记录仓库，使用仓库回执表示。
		var warehouse *WarehouseReceipt
		// vendor 保存实际成交的商人买卖或修理结果。
		var vendor *VendorReceipt
		var taskCodex *TaskCodexClaimReceipt // 首次领取确认的任务或图鉴奖励。
		// 计算本次领域变更，并为需要专属回执的操作保留分类结果。
		switch q.Kind {
		case AssetVendor:
			r, businessCode, vendor, e = s.commitVendor(tc, a, r, q)
			if e != nil {
				return e
			}
		case "warehouse":
			if q.Warehouse == nil || !proto.Equal(q.Warehouse.Operation, q.Context) {
				businessCode = p.CodeInvalidArgument
				break
			}
			r, warehouse, businessCode, e = s.applyWarehouse(tc, r, q.Warehouse)
			if e != nil {
				return e
			}
		case "equip":
			r, equipment, businessCode = equipEquipment(r, q, s.Cfg.Fixture)
		case "recycle":
			r, recycling, businessCode, e = recycleManual(r, q.ItemIDs, s.Cfg.Fixture)
			if e != nil {
				return e
			}
		case "set_auto_recycle":
			r, recycling, businessCode, e = setAutoRecycle(r, q, s.Cfg.Fixture)
			if e != nil {
				return e
			}
		case "set_item_lock":
			r, businessCode = setItemLock(r, q, s.Cfg.Fixture)
		case "claim_hosting":
			if q.Benefit != 0 && q.Benefit != 1 {
				businessCode = p.CodeInvalidArgument
				break
			}
			// 按上海运营日结算当日差额，卡权益变化时仅补足未领取部分。
			// location 采用上海时区，用于确定运营日边界。
			location, _ := time.LoadLocation("Asia/Shanghai")
			// now 记录本次处理开始时的时间。
			now := time.Now()
			// day 是领取记录使用的上海日期键。
			day := now.In(location).Format("2006-01-02")
			if r.ClaimDay != day {
				r.ClaimDay = day
				r.ClaimedSeconds = 0
			}
			// limit 是普通角色当日可领取的托管秒数，有卡时改为十二小时。
			limit := int64(6 * 3600)
			if r.HostingCardUntil.After(now) {
				limit = 12 * 3600
			}
			// delta 是当天额度扣除已领取部分后的可补领秒数。
			delta := limit - r.ClaimedSeconds
			if delta <= 0 {
				businessCode = p.CodeOperationConflict
				break
			}
			r.HostingSeconds, e = add(r.HostingSeconds, delta)
			if e != nil {
				return e
			}
			r.ClaimedSeconds += delta
		case "revive":
			if !q.ValidRevive || r.RetiredDeathID != "" && r.RetiredDeathID != q.ReviveDeathID {
				businessCode = p.CodeInvalidArgument
				break
			}
			// 只有成功确认的原复活选择能清除死亡标记。临时位置和生命仍由场景处理。
			r.RetiredDeathID = ""
		case "use_item", AssetAutoUsePotion:
			// effect 记录效果，使用药水效果表示。
			var effect devconfig.PotionEffect
			r, effect, businessCode = consumePotion(r, q, s.Cfg.Fixture)
			if businessCode != 0 {
				if q.Kind == AssetAutoUsePotion {
					return Fail(businessCode, "未满足自动用药资格")
				}
				break
			}
			potion = &PotionReceipt{Scene: strconv.FormatUint(q.SceneEpoch, 10), ItemID: effect.ItemID, HP: effect.HP, MP: effect.MP}
		case "learn_skill":
			r, businessCode = learnSkill(r, q, s.Cfg.Fixture)
		case AssetTaskCodexClaim:
			if q.TaskCodex == nil || !proto.Equal(q.TaskCodex.Operation, q.Context) {
				businessCode = p.CodeInvalidArgument
				break
			}
			r, taskCodex, businessCode = claimTaskCodex(r, &s.Cfg.Fixture, q.TaskCodex, taskEventAt, opID)
		case "set_casting_supply":
			r, businessCode = setCastingSupply(r, q, s.Cfg.Fixture)
		case "cast_material":
			r, casting, businessCode = consumeCastingMaterial(r, q, s.Cfg.Fixture)
		case "skill_assets_commit":
			r, businessCode, skillDetails, e = s.commitSkillAsset(tc, r, q)
			if e != nil {
				return e
			}
		default:
			businessCode = p.CodeInvalidArgument
		}
		// 确定的业务拒绝保留原资产内容，随后写入原操作终态避免重试改判。
		if businessCode != 0 {
			r = out
			recycling = nil
			skillDetails = nil
			equipment = nil
			warehouse = nil
			vendor = nil
			taskCodex = nil
		} else {
			r = advanceAssetTasks(r, &s.Cfg.Fixture, q.Kind, recycling, taskEventAt)
		}
		if businessCode == 0 && len(r.Items) < r.Capacity {
			r.PickupStopped = false
		}
		if q.SourceSeq > 0 {
			r.SourceSeq++
		} else {
			r.EconomicSeq++
		}
		r.Revision++
		if e = s.replace(tc, r, oldRevision); e != nil {
			return e
		}
		// 角色变更、永久回执和审计同属本事务，其中任何一项失败都会回滚。
		// op 组装操作所需的数据。
		op := Operation{ID: opID, Digest: q.Fingerprint, RoleID: r.ID, CommandSeq: r.EconomicSeq, SourceSeq: q.SourceSeq, Revision: r.Revision, Code: businessCode, Kind: q.Kind, Created: time.Now(), Potion: potion, Casting: casting, Recycling: recycling}
		op.Skill = cloneSkillPlan(skillDetails)
		op.Equipment = cloneEquipmentReceipt(equipment)
		op.Warehouse = cloneWarehouseReceipt(warehouse)
		op.Vendor = vendor
		op.TaskCodex = taskCodex
		if q.Kind == "set_auto_recycle" && businessCode == 0 {
			op.AutoRecyclePolicy = policySnapshot(r.AutoRecycle)
		}
		if _, e = s.DB.Collection("operations").InsertOne(tc, op); e != nil {
			return e
		}
		// audit 初始化用作审计记录的MongoDB BSON 文档。
		audit := bson.M{"_id": opID, "role_id": r.ID, "account_id": a.AccountID, "kind": q.Kind, "op_id": domain.Hex(q.Context.OpId), "revision": r.Revision, "result_code": businessCode, "at": time.Now()}
		if op.AutoRecyclePolicy != nil {
			audit["auto_recycle_policy"] = op.AutoRecyclePolicy
		}
		if skillDetails != nil {
			audit["preview_id"] = q.PreviewID
			audit["skill_plan"] = skillDetails
		}
		if equipment != nil {
			audit["equipment"] = cloneEquipmentReceipt(equipment)
		}
		if warehouse != nil {
			audit["warehouse"] = cloneWarehouseReceipt(warehouse)
		}
		if vendor != nil {
			audit["npc_trade"] = vendor
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if casting != nil {
			audit["casting_use"] = casting
			audit["source_seq"] = q.SourceSeq
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if q.Kind == AssetAutoUsePotion {
			audit["potion"] = potion
			audit["item_instance_id"] = q.ItemID
			audit["source_seq"] = q.SourceSeq
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if recycling != nil {
			audit["recycling"] = recycling
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if taskCodex != nil {
			audit["tasks_codex_claim"] = taskCodex
		}
		if _, e = s.DB.Collection("audit_outbox").InsertOne(tc, audit); e != nil {
			return e
		}
		out = r
		result.NewRevision = uint64(r.Revision)
		result.CommandSeq = uint64(r.EconomicSeq)
		result.ErrorCode = businessCode
		receipt = op
		return nil
	})
	if e != nil {
		result.ErrorCode = Code(e)
		result.Pending = result.ErrorCode == p.CodeOperationPending
	}
	return out, result, receipt, e
}

// 全部条件先验证；修改独立切片，业务拒绝不能污染原角色的库存。
// r 是扣药前角色，q 指定原库存实例和场景，fixture 提供当前药品规则。
func consumePotion(r domain.Role, q AssetRequest, fixture devconfig.Fixture) (domain.Role, devconfig.PotionEffect, uint32) {
	// effect 是配置中的药效，ok 表示该物品有药效记录。
	effect, ok := fixture.Potions.Effect(q.ItemDefinitionID)
	// definition 是物品定义，exists 用来拒绝不存在或旧配置中已经移除的物品。
	definition, exists := fixture.Item(q.ItemDefinitionID)
	if !ok || !exists || definition.IsEquipment || effect.HP < 0 || effect.MP < 0 || (effect.HP == 0 && effect.MP == 0) || q.SceneEpoch == 0 || len(q.ItemID) != 32 {
		return r, effect, p.CodeInvalidArgument
	}
	if q.PotionGateCode != 0 {
		return r, effect, q.PotionGateCode
	}
	// index 保存当前处理所用的下标。
	index := -1
	// i 定位道具集合中正在处理的成员，item 接收遍历道具集合时当前的元素。
	for i, item := range r.Items {
		if item.ID == q.ItemID {
			index = i
			break
		}
	}
	if index < 0 || r.Items[index].ItemID != q.ItemDefinitionID || r.Items[index].Quantity < 1 || r.Items[index].Version < 1 || r.Items[index].Version == math.MaxInt64 {
		return r, effect, p.CodeInvalidArgument
	}
	if q.Kind == AssetAutoUsePotion && (!AutoManaPotionEligible(r.Items[index], r.Job, fixture) || uint64(r.Items[index].Version) != q.ItemVersion) {
		return r, effect, p.CodeInvalidArgument
	}
	r.Items = append([]domain.Item(nil), r.Items...)
	if r.Items[index].Quantity == 1 {
		r.Items = append(r.Items[:index], r.Items[index+1:]...)
	} else {
		r.Items[index].Quantity--
		r.Items[index].Version++
	}
	return r, effect, 0
}

// Settlement 固定服务端某个来源序号需要同时提交的收益、托管扣时与磨损事实。
type Settlement struct {
	SourceSeq        int64           // 同角色必须连续递增的来源序号。
	Kills            int64           // 本来源对应的击杀次数。
	MonsterID        uint32          `json:"monster_id,omitempty"`   // 实际击杀的怪物配置号，旧来源或非击杀为零。
	MapID            uint64          `json:"map_id,omitempty"`       // 原击杀发生的地图配置号，不随之后换图而变。
	DropPoolID       uint32          `json:"drop_pool_id,omitempty"` // 怪物原掉落池号，历史全局掉落和非击杀为零。
	Experience       int64           // 应用等级及经验规则前的原始经验。
	Gold             int64           // 本来源直接产生的金币，不含装备回收。
	Bound            bool            // 直接金币应计入绑定钱包还是普通钱包。
	Item             *domain.Item    // 本来源产生的原掉落实例，可为空。
	NoEquipmentKills int             // 应由本来源确认的连续未出装备次数。
	HostSeconds      int64           // 本来源需要从托管余额扣除的已计量整秒。
	HostingSession   string          // 托管扣时和战报归属的会话标识。
	StopReason       uint32          // 本来源确认的托管停止原因，零表示没有新原因。
	EventAt          time.Time       // 战斗或计量事实的发生时刻，供审计追溯。
	TargetHandle     uint32          // 结算目标在原场景中的实体句柄。
	TargetLife       uint64          // 结算目标的原生命代次。
	CasterHandle     uint32          // 产生该事实的施法者或攻击者句柄。
	EquipmentWear    []EquipmentWear `json:"equipment_wear,omitempty"` // 并入本来源提交的磨损事实明细。
	WearDeathID      string          `json:"wear_death_id,omitempty"`  // 死亡磨损对应的原死亡标识。
}
