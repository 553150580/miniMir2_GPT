package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"time"
)

// 永久来源结果只保存资产事实，不保存临时战斗世界。
type SettlementReceipt struct {
	RuleVersion uint64                    `bson:"rule_version"`
	ConfigHash  string                    `bson:"config_hash"`
	GrantedItem bool                      `bson:"granted_item"`
	Recycling   EquipmentRecyclingReceipt `bson:"recycling"`
	Experience  *ExperienceGrantReceipt   `bson:"experience_grant,omitempty"`
}

func (s *Store) Settle(ctx context.Context, a Auth, q Settlement) (domain.Role, error) {
	r, _, err := s.SettleWithReceipt(ctx, a, q)
	return r, err
}

func (s *Store) SettleWithReceipt(ctx context.Context, a Auth, q Settlement) (domain.Role, Operation, error) {
	var out domain.Role
	var receipt Operation
	if q.SourceSeq <= 0 {
		return out, receipt, Fail(p.CodeInvalidArgument, "收益来源序号必须为正")
	}
	raw, err := json.Marshal(q)
	if err != nil {
		return out, receipt, err
	}
	digest := Hash(raw)
	id := fmt.Sprintf("settle:%s:%d", a.RoleID, q.SourceSeq)
	err = s.tx(ctx, func(tc context.Context) error {
		receipt = Operation{}
		if err := s.fence(tc, a); err != nil {
			return err
		}
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
		var old Operation
		err = s.DB.Collection("operations").FindOne(tc, bson.M{"_id": id}).Decode(&old)
		if err == nil {
			if old.Kind != "settlement" || old.RoleID != a.RoleID || old.SourceSeq != q.SourceSeq || old.Digest != digest || old.Settlement == nil {
				return Fail(p.CodeOperationConflict, "同来源结算内容改变")
			}
			out, receipt = r, old
			return nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
		if r.SourceSeq >= q.SourceSeq {
			if r.SettlementReceiptStartSeq > 0 && q.SourceSeq >= r.SettlementReceiptStartSeq {
				return Fail(p.CodeOperationConflict, "新收益来源缺少永久回执")
			}
			// 升级前的历史来源只有水位；不构造或补播不存在的回收结果。
			out = r
			return nil
		}
		if r.SourceSeq == math.MaxInt64 || r.SourceSeq+1 != q.SourceSeq {
			return Fail(p.CodeOperationConflict, "收益来源游标缺口")
		}
		before := r.Revision
		next, details, err := applySettlement(r, q, s.Cfg.Fixture)
		if err != nil {
			return err
		}
		if err = s.replace(tc, next, before); err != nil {
			return err
		}
		receipt = Operation{ID: id, Digest: digest, RoleID: r.ID, CommandSeq: r.EconomicSeq, SourceSeq: q.SourceSeq, Revision: next.Revision, Kind: "settlement", Created: time.Now(), Settlement: &details, Recycling: &details.Recycling}
		if _, err = s.DB.Collection("operations").InsertOne(tc, receipt); err != nil {
			return err
		}
		eventAt := q.EventAt
		if eventAt.IsZero() {
			eventAt = time.Now()
		}
		if _, err = s.DB.Collection("audit_outbox").InsertOne(tc, bson.M{"_id": id, "kind": "settlement", "role_id": r.ID, "source_seq": q.SourceSeq, "event_at": eventAt, "bound": q.Bound, "kills": q.Kills, "experience": q.Experience, "gold": q.Gold, "item": q.Item, "host_seconds": q.HostSeconds, "hosting_session": q.HostingSession, "target_handle": q.TargetHandle, "target_life": q.TargetLife, "caster_handle": q.CasterHandle, "rule_version": details.RuleVersion, "config_hash": details.ConfigHash, "settlement": details, "at": time.Now()}); err != nil {
			return err
		}
		out = next
		return nil
	})
	return out, receipt, err
}

func applySettlement(r domain.Role, q Settlement, f devconfig.Fixture) (domain.Role, SettlementReceipt, error) {
	details := SettlementReceipt{RuleVersion: f.RuleVersion, ConfigHash: f.Hash, Recycling: EquipmentRecyclingReceipt{PolicyRevision: r.AutoRecycle.Revision, Policy: policySnapshot(r.AutoRecycle), Trigger: 1}}
	if q.HostSeconds < 0 || r.HostingSeconds < q.HostSeconds || q.Kills < 0 || r.Level == 0 || r.Revision == math.MaxInt64 {
		return r, details, Fail(p.CodeOperationConflict, "托管时长或角色结算状态无效")
	}
	if f.RuleVersion >= 19 {
		if _, code := policyFromProto(r.AutoRecycle.Proto(), f); code != 0 || r.AutoRecycle.Revision < 0 {
			return r, details, Fail(p.CodeOperationConflict, "角色回收策略无效")
		}
	}
	out := r
	out.SourceSeq = q.SourceSeq
	if f.RuleVersion >= 19 && out.SettlementReceiptStartSeq == 0 {
		out.SettlementReceiptStartSeq = q.SourceSeq
	}
	out.HostingSeconds -= q.HostSeconds
	var err error
	actualExperience := q.Experience
	if f.RuleVersion >= 20 {
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
	if q.Kills > 0 || q.Item != nil {
		out, err = recycleExisting(out, f, &details.Recycling)
		if err != nil {
			return r, details, err
		}
	}
	if q.Item != nil {
		recycled := false
		if f.RuleVersion >= 19 && autoRecycleMatches(out, *q.Item, f) {
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
	if q.HostingSession != "" {
		if out.Report.SessionID != q.HostingSession {
			out.Report = domain.Report{SessionID: q.HostingSession, Drops: []domain.Item{}}
		}
		report := out.Report
		// 累计值全部检查；未知提交重试读取原回执，不会再次累计。
		fields := []struct {
			dst   *int64
			delta int64
		}{
			{&report.UsedSeconds, q.HostSeconds}, {&report.Kills, q.Kills}, {&report.Experience, actualExperience}, {&report.RawExperience, q.Experience}, {&report.Sequence, 1},
			{&report.RecycleCount, int64(len(details.Recycling.Recycled))}, {&report.RecycleGold, details.Recycling.Gold}, {&report.RecycleBoundGold, details.Recycling.BoundGold}, {&report.DiscardCount, details.Recycling.DiscardedCount},
			{&report.Earned.Gold, details.Recycling.Gold}, {&report.Earned.BoundGold, details.Recycling.BoundGold},
		}
		if details.Experience != nil {
			fields = append(fields, struct {
				dst   *int64
				delta int64
			}{&report.RejectedExperience, details.Experience.Rejected}, struct {
				dst   *int64
				delta int64
			}{&report.PooledExperience, details.Experience.ToPool})
		}
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
		if out.PickupStopped {
			report.StopReason = 3
		}
		if q.StopReason != 0 {
			report.StopReason = q.StopReason
		}
		if out.HostingSeconds == 0 && report.StopReason == 0 {
			report.StopReason = 1
		}
		out.Report = report
	}
	out.Revision++
	return out, details, nil
}
