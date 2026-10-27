package persistence

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"strconv"
	"time"
)

type AssetRequest struct {
	Kind             string
	Fingerprint      string
	Context          *pb.OperationContext
	ItemID           string
	ItemIDs          []string
	Slot             uint32
	Benefit          uint32
	ValidRevive      bool
	ItemDefinitionID uint32
	SceneEpoch       uint64
	PotionGateCode   uint32
}

func add(a, b int64) (int64, error) {
	if b < 0 || a > math.MaxInt64-b {
		return a, Fail(p.CodeInvalidArgument, "整数余额溢出")
	}
	return a + b, nil
}
func (s *Store) Asset(ctx context.Context, a Auth, q AssetRequest) (domain.Role, *pb.OperationResult, error) {
	role, result, _, err := s.AssetWithReceipt(ctx, a, q)
	return role, result, err
}

// 回执保留首次提交的原命令序号；OperationResult仍返回客户端需要的当前经济水位。
func (s *Store) AssetWithReceipt(ctx context.Context, a Auth, q AssetRequest) (domain.Role, *pb.OperationResult, Operation, error) {
	var out domain.Role
	var receipt Operation
	result := &pb.OperationResult{}
	if q.Context == nil || len(q.Context.OpId) != 16 {
		return out, result, receipt, Fail(p.CodeInvalidArgument, "资产请求缺少16字节op_id")
	}
	result.OpId = q.Context.OpId
	opID := "asset:" + a.RoleID + ":" + domain.Hex(q.Context.OpId)
	e := s.tx(ctx, func(tc context.Context) error {
		receipt = Operation{}
		if e := s.fence(tc, a); e != nil {
			return e
		}
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
		var old Operation
		e = s.DB.Collection("operations").FindOne(tc, bson.M{"_id": opID}).Decode(&old)
		if e == nil {
			if old.Digest != q.Fingerprint {
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
		if q.Context.CommandSeq != uint64(r.EconomicSeq+1) || q.Context.ExpectedRevision != uint64(r.Revision) || q.Context.ExpectedRuleVersion != s.Cfg.Fixture.RuleVersion {
			return Fail(p.CodeOperationConflict, "操作序列/版本变化，请刷新后保留原操作身份")
		}
		oldRevision := r.Revision
		businessCode := uint32(0)
		var potion *PotionReceipt
		switch q.Kind {
		case "equip":
			index := -1
			for i, item := range r.Items {
				if item.ID == q.ItemID {
					index = i
					break
				}
			}
			if index < 0 {
				businessCode = p.CodeInvalidArgument
				break
			}
			item := r.Items[index]
			def, ok := s.Cfg.Fixture.Item(item.ItemID)
			eligible := len(def.AllowedJobs) == 0
			for _, job := range def.AllowedJobs {
				if job == r.Job {
					eligible = true
				}
			}
			if !ok || !def.IsEquipment || def.EquipmentSlot != q.Slot || !eligible {
				businessCode = p.CodeInvalidArgument
				break
			}
			r.Items = append(r.Items[:index], r.Items[index+1:]...)
			replacement := -1
			for i, eq := range r.Equipped {
				if eq.Slot == q.Slot {
					replacement = i
					r.Items = append(r.Items, eq.Item)
					break
				}
			}
			item.Version++
			if replacement >= 0 {
				r.Equipped[replacement] = domain.Equipped{Slot: q.Slot, Item: item}
			} else {
				r.Equipped = append(r.Equipped, domain.Equipped{Slot: q.Slot, Item: item})
			}
		case "recycle":
			if len(q.ItemIDs) == 0 || len(q.ItemIDs) > r.Capacity {
				businessCode = p.CodeInvalidArgument
				break
			}
			wanted := map[string]bool{}
			for _, id := range q.ItemIDs {
				if wanted[id] {
					businessCode = p.CodeInvalidArgument
				}
				wanted[id] = true
			}
			var keep []domain.Item
			count := 0
			gold, boundGold := int64(0), int64(0)
			for _, item := range r.Items {
				if !wanted[item.ID] {
					keep = append(keep, item)
					continue
				}
				count++
				def, ok := s.Cfg.Fixture.Item(item.ItemID)
				if !ok || !def.IsEquipment || def.RecycleGold < 0 || item.Quantity != 1 {
					businessCode = p.CodeInvalidArgument
					continue
				}
				if item.Bound {
					boundGold, e = add(boundGold, def.RecycleGold)
				} else {
					gold, e = add(gold, def.RecycleGold)
				}
				if e != nil {
					return e
				}
			}
			if count != len(wanted) {
				businessCode = p.CodeInvalidArgument
			}
			if businessCode == 0 {
				r.Wallet.Gold, e = add(r.Wallet.Gold, gold)
				if e != nil {
					return e
				}
				r.Wallet.BoundGold, e = add(r.Wallet.BoundGold, boundGold)
				if e != nil {
					return e
				}
				r.Items = keep
			}
		case "claim_hosting":
			if q.Benefit != 0 && q.Benefit != 1 {
				businessCode = p.CodeInvalidArgument
				break
			}
			location, _ := time.LoadLocation("Asia/Shanghai")
			now := time.Now()
			day := now.In(location).Format("2006-01-02")
			if r.ClaimDay != day {
				r.ClaimDay = day
				r.ClaimedSeconds = 0
			}
			limit := int64(6 * 3600)
			if r.HostingCardUntil.After(now) {
				limit = 12 * 3600
			}
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
			if !q.ValidRevive {
				businessCode = p.CodeInvalidArgument
			}
			// 开发样例只记录操作防重；临时位置和HP由权威场景在提交后处理，不保存数据库。
		case "use_item":
			var effect devconfig.PotionEffect
			r, effect, businessCode = consumePotion(r, q, s.Cfg.Fixture)
			if businessCode == 0 {
				potion = &PotionReceipt{Scene: strconv.FormatUint(q.SceneEpoch, 10), ItemID: effect.ItemID, HP: effect.HP, MP: effect.MP}
			}
		default:
			businessCode = p.CodeInvalidArgument
		}
		if businessCode != 0 {
			r = out
		}
		if businessCode == 0 && len(r.Items) < r.Capacity {
			r.PickupStopped = false
		}
		r.EconomicSeq++
		r.Revision++
		if e = s.replace(tc, r, oldRevision); e != nil {
			return e
		}
		op := Operation{ID: opID, Digest: q.Fingerprint, RoleID: r.ID, CommandSeq: r.EconomicSeq, Revision: r.Revision, Code: businessCode, Kind: q.Kind, Created: time.Now(), Potion: potion}
		if _, e = s.DB.Collection("operations").InsertOne(tc, op); e != nil {
			return e
		}
		if _, e = s.DB.Collection("audit_outbox").InsertOne(tc, bson.M{"_id": opID, "role_id": r.ID, "account_id": a.AccountID, "kind": q.Kind, "op_id": domain.Hex(q.Context.OpId), "revision": r.Revision, "result_code": businessCode, "at": time.Now()}); e != nil {
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
func consumePotion(r domain.Role, q AssetRequest, fixture devconfig.Fixture) (domain.Role, devconfig.PotionEffect, uint32) {
	effect, ok := fixture.Potions.Effect(q.ItemDefinitionID)
	definition, exists := fixture.Item(q.ItemDefinitionID)
	if !ok || !exists || definition.IsEquipment || effect.HP < 0 || effect.MP < 0 || (effect.HP == 0 && effect.MP == 0) || q.SceneEpoch == 0 || len(q.ItemID) != 32 {
		return r, effect, p.CodeInvalidArgument
	}
	if q.PotionGateCode != 0 {
		return r, effect, q.PotionGateCode
	}
	index := -1
	for i, item := range r.Items {
		if item.ID == q.ItemID {
			index = i
			break
		}
	}
	if index < 0 || r.Items[index].ItemID != q.ItemDefinitionID || r.Items[index].Quantity < 1 || r.Items[index].Version < 1 || r.Items[index].Version == math.MaxInt64 {
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

type Settlement struct {
	SourceSeq        int64
	Kills            int64
	Experience       int64
	Gold             int64
	Bound            bool
	Item             *domain.Item
	NoEquipmentKills int
	HostSeconds      int64
	HostingSession   string
	StopReason       uint32
}

func (s *Store) Settle(ctx context.Context, a Auth, q Settlement) (domain.Role, error) {
	var out domain.Role
	e := s.tx(ctx, func(tc context.Context) error {
		if e := s.fence(tc, a); e != nil {
			return e
		}
		r, e := s.Role(tc, a.RoleID)
		if e != nil {
			return e
		}
		if e = s.checkRole(r, a); e != nil {
			return e
		}
		if r.SourceSeq >= q.SourceSeq {
			out = r
			return nil
		}
		if r.SourceSeq+1 != q.SourceSeq {
			return Fail(p.CodeOperationConflict, "收益来源游标缺口")
		}
		if q.HostSeconds < 0 || r.HostingSeconds < q.HostSeconds {
			return Fail(p.CodeOperationConflict, "托管时长不足")
		}
		before := r.Revision
		r.SourceSeq = q.SourceSeq
		r.HostingSeconds -= q.HostSeconds
		r.Experience, e = add(r.Experience, q.Experience)
		if e != nil {
			return e
		}
		for int(r.Level) <= len(s.Cfg.Fixture.Progression.ExpPerLevel) {
			required := s.Cfg.Fixture.Progression.ExpPerLevel[r.Level-1]
			if required <= 0 || r.Experience < required {
				break
			}
			r.Experience -= required
			r.Level++
		}
		coin := q.Gold
		grantedItem := false
		if q.Item != nil {
			if len(r.Items) < r.Capacity {
				r.Items = append(r.Items, *q.Item)
				grantedItem = true
			} else {
				// 首循环未配置玩家自动回收条件，满包明确放弃此物并停止托管，绝不擅自按售价换币。
				r.PickupStopped = true
				r.LastDiscardItem = q.Item.ItemID
				r.LastDiscardBound = q.Item.Bound
			}
		}
		if q.Bound {
			r.Wallet.BoundGold, e = add(r.Wallet.BoundGold, coin)
		} else {
			r.Wallet.Gold, e = add(r.Wallet.Gold, coin)
		}
		if e != nil {
			return e
		}
		if q.Kills > 0 {
			r.NoEquipmentKills = q.NoEquipmentKills
		}
		if q.HostingSession != "" {
			if r.Report.SessionID != q.HostingSession {
				r.Report = domain.Report{SessionID: q.HostingSession, Drops: []domain.Item{}}
			}
			r.Report.UsedSeconds += q.HostSeconds
			r.Report.Kills += q.Kills
			r.Report.Experience += q.Experience
			if q.Bound {
				r.Report.Earned.BoundGold += coin
			} else {
				r.Report.Earned.Gold += coin
			}
			if grantedItem && len(r.Report.Drops) < 100 {
				r.Report.Drops = append(r.Report.Drops, *q.Item)
			}
			r.Report.Sequence++
			if r.PickupStopped {
				r.Report.StopReason = 3
			}
			if q.StopReason != 0 {
				r.Report.StopReason = q.StopReason
			}
			if r.HostingSeconds == 0 && r.Report.StopReason == 0 {
				r.Report.StopReason = 1
			}
		}
		r.Revision++
		if e = s.replace(tc, r, before); e != nil {
			return e
		}
		out = r
		return nil
	})
	return out, e
}
