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
	SkillID          uint32
	PreviewID        string
	ItemVersion      uint64
	CasterLife       uint64
	CastGateCode     uint32
	Unequip          bool
	SourceSeq        int64
	AutoRecycle      *pb.AutoRecyclePolicy
	ApplyExisting    bool
	Locked           bool
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
	if q.SourceSeq > 0 {
		if q.Kind != "cast_material" || q.Context.CommandSeq != 0 {
			return out, result, receipt, Fail(p.CodeInvalidArgument, "自动施法必须使用独立来源流水")
		}
		opID = "source_cast:" + a.RoleID + ":" + domain.Hex(q.Context.OpId)
	}
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
		sequenceValid := q.SourceSeq == 0 && q.Context.CommandSeq == uint64(r.EconomicSeq+1) || q.SourceSeq > 0 && q.SourceSeq == r.SourceSeq+1
		if r.Revision == math.MaxInt64 || q.SourceSeq == 0 && r.EconomicSeq == math.MaxInt64 || q.SourceSeq > 0 && r.SourceSeq == math.MaxInt64 {
			return Fail(p.CodeOperationConflict, "资产序号已耗尽")
		}
		if !sequenceValid || q.Context.ExpectedRevision != uint64(r.Revision) || q.Context.ExpectedRuleVersion != s.Cfg.Fixture.RuleVersion {
			return Fail(p.CodeOperationConflict, "操作序列/版本变化，请刷新后保留原操作身份")
		}
		oldRevision := r.Revision
		businessCode := uint32(0)
		var potion *PotionReceipt
		var skillDetails *skillPlan
		var casting *CastingReceipt
		var recycling *EquipmentRecyclingReceipt
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
			if !ok || !def.IsEquipment || def.EquipmentSlot != q.Slot || q.Slot > 9 || !eligible {
				businessCode = p.CodeInvalidArgument
				break
			}
			if s.Cfg.Fixture.RuleVersion >= 20 && (def.RequiredLevel == nil || *def.RequiredLevel < 1 || r.Level < *def.RequiredLevel) {
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
		case "learn_skill":
			r, businessCode = learnSkill(r, q, s.Cfg.Fixture)
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
		if businessCode != 0 {
			r = out
			recycling = nil
			skillDetails = nil
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
		op := Operation{ID: opID, Digest: q.Fingerprint, RoleID: r.ID, CommandSeq: r.EconomicSeq, SourceSeq: q.SourceSeq, Revision: r.Revision, Code: businessCode, Kind: q.Kind, Created: time.Now(), Potion: potion, Casting: casting, Recycling: recycling}
		op.Skill = cloneSkillPlan(skillDetails)
		if q.Kind == "set_auto_recycle" && businessCode == 0 {
			op.AutoRecyclePolicy = policySnapshot(r.AutoRecycle)
		}
		if _, e = s.DB.Collection("operations").InsertOne(tc, op); e != nil {
			return e
		}
		audit := bson.M{"_id": opID, "role_id": r.ID, "account_id": a.AccountID, "kind": q.Kind, "op_id": domain.Hex(q.Context.OpId), "revision": r.Revision, "result_code": businessCode, "at": time.Now()}
		if op.AutoRecyclePolicy != nil {
			audit["auto_recycle_policy"] = op.AutoRecyclePolicy
		}
		if skillDetails != nil {
			audit["preview_id"] = q.PreviewID
			audit["skill_plan"] = skillDetails
		}
		if casting != nil {
			audit["casting_use"] = casting
			audit["source_seq"] = q.SourceSeq
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
		}
		if recycling != nil {
			audit["recycling"] = recycling
			audit["rule_version"] = s.Cfg.Fixture.RuleVersion
			audit["config_hash"] = s.Cfg.Fixture.Hash
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
	EventAt          time.Time
	TargetHandle     uint32
	TargetLife       uint64
	CasterHandle     uint32
}
