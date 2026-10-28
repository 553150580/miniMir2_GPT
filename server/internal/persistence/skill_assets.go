package persistence

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/protobuf/proto"
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"sort"
	"time"
)

type skillMaterial struct {
	Item     domain.Item `bson:"item"`
	Quantity int64       `bson:"quantity"`
}
type skillPlan struct {
	Kind      uint32          `bson:"kind"`
	OfferID   uint32          `bson:"offer_id"`
	Quantity  int64           `bson:"quantity"`
	SkillID   uint32          `bson:"skill_id"`
	From      uint32          `bson:"from"`
	To        uint32          `bson:"to"`
	Spend     domain.Wallet   `bson:"spend"`
	Materials []skillMaterial `bson:"materials"`
	Outputs   []domain.Item   `bson:"outputs"`
}

// 每角色至多一个可替换预览；仅预览允许过期，永久操作回执不受其清理影响。
type skillQuote struct {
	RoleID      string    `bson:"_id"`
	ID          string    `bson:"preview_id"`
	Revision    int64     `bson:"revision"`
	RuleVersion uint64    `bson:"rule_version"`
	Hash        string    `bson:"config_hash"`
	Expires     time.Time `bson:"expires_at"`
	Request     []byte    `bson:"request"`
	Plan        skillPlan `bson:"plan"`
}

func multiply(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || (a > 0 && b > math.MaxInt64/a) {
		return 0, false
	}
	return a * b, true
}
func selectSkillMaterials(r domain.Role, q *pb.SkillAssetPreviewRequest, need map[uint32]int64, limit int) ([]skillMaterial, uint32) {
	selected := q.Materials
	if len(selected) == 0 || len(selected) > limit {
		return nil, p.CodeSkillMaterials
	}
	out := []skillMaterial{}
	seen := map[string]bool{}
	actual := map[uint32]int64{}
	for _, c := range selected {
		if c == nil || len(c.InstanceId) != 16 || c.Quantity <= 0 || c.Version == 0 || c.Version > math.MaxInt64 {
			return nil, p.CodeSkillMaterials
		}
		id := domain.Hex(c.InstanceId)
		if seen[id] {
			return nil, p.CodeSkillMaterials
		}
		seen[id] = true
		found := false
		for _, item := range r.Items {
			if item.ID != id {
				continue
			}
			if item.Version != int64(c.Version) || item.Version == math.MaxInt64 || item.Quantity < c.Quantity {
				return nil, p.CodeSkillMaterials
			}
			total, err := add(actual[item.ItemID], c.Quantity)
			if err != nil {
				return nil, p.CodeSkillMaterials
			}
			actual[item.ItemID] = total
			out = append(out, skillMaterial{item, c.Quantity})
			found = true
			break
		}
		if !found {
			return nil, p.CodeSkillMaterials
		}
	}
	if q.Kind == 2 && !reflect.DeepEqual(actual, need) {
		return nil, p.CodeSkillMaterials
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Item.ID < out[j].Item.ID })
	return out, 0
}

func buildSkillPlan(r domain.Role, q *pb.SkillAssetPreviewRequest, f devconfig.Fixture) (skillPlan, uint32) {
	plan := skillPlan{Kind: q.Kind, OfferID: q.OfferId, Quantity: q.Quantity, SkillID: q.SkillId}
	if r.Wallet.Gold < 0 || r.Wallet.BoundGold < 0 || r.Wallet.Yuanbao < 0 || r.Wallet.BoundYuanbao < 0 {
		return plan, p.CodeInvalidArgument
	}
	switch q.Kind {
	case 1:
		offer, ok := f.ShopOffer(q.OfferId)
		if !ok || q.Quantity <= 0 || q.Quantity > offer.MaxPurchaseQuantity || r.Level < offer.RequiredRoleLevel || len(q.Materials) > 0 || q.SkillId != 0 {
			return plan, p.CodeSkillPurchaseBlocked
		}
		quantity, valid := multiply(offer.Quantity, q.Quantity)
		if !valid {
			return plan, p.CodeSkillPurchaseBlocked
		}
		cost, valid := multiply(offer.PriceUnits, q.Quantity)
		if !valid {
			return plan, p.CodeSkillPurchaseBlocked
		}
		switch offer.Currency {
		case "gold":
			plan.Spend.BoundGold = cost
			if plan.Spend.BoundGold > r.Wallet.BoundGold {
				plan.Spend.BoundGold = r.Wallet.BoundGold
			}
			plan.Spend.Gold = cost - plan.Spend.BoundGold
		case "ingot":
			plan.Spend.Yuanbao = cost
		case "bound_ingot":
			plan.Spend.BoundYuanbao = cost
		default:
			return plan, p.CodeSkillPurchaseBlocked
		}
		plan.Outputs = []domain.Item{{ItemID: offer.ItemID, Quantity: quantity, Bound: offer.OutputBound, Version: 1, SourceKind: "npc_shop"}}
	case 2:
		s, exists := f.Skill(q.SkillId)
		level, learned := r.Skill(q.SkillId)
		if !exists || !learned || r.Job != s.JobID || level < s.LearnedLevel || level >= s.MaxLevel || q.OfferId != 0 || q.Quantity != 0 {
			return plan, p.CodeSkillUpgradeBlocked
		}
		row, exists := f.Upgrade(s.ID, level+1)
		if !exists || r.Level < row.RequiredRoleLevel {
			return plan, p.CodeSkillUpgradeBlocked
		}
		plan.From, plan.To = level, level+1
		need := map[uint32]int64{}
		for _, c := range row.BookCosts {
			n, err := add(need[c.ItemID], c.Quantity)
			if err != nil {
				return plan, p.CodeSkillMaterials
			}
			need[c.ItemID] = n
		}
		if row.PageCost > 0 {
			n, err := add(need[row.PageItemID], row.PageCost)
			if err != nil {
				return plan, p.CodeSkillMaterials
			}
			need[row.PageItemID] = n
		}
		materials, code := selectSkillMaterials(r, q, need, f.SkillEconomy.PreviewMaterialLimit)
		if code != 0 {
			return plan, code
		}
		plan.Materials = materials
	case 3:
		if q.OfferId != 0 || q.Quantity != 0 || q.SkillId != 0 {
			return plan, p.CodeSkillBookSource
		}
		materials, code := selectSkillMaterials(r, q, nil, f.SkillEconomy.PreviewMaterialLimit)
		if code != 0 {
			return plan, code
		}
		plan.Materials = materials
		for _, m := range materials {
			item := m.Item
			rule, ok := f.BookRecycle(item.ItemID, item.Quality)
			allowed := false
			for _, kind := range rule.AllowedSourceKinds {
				if item.SourceKind == kind {
					allowed = true
				}
			}
			if !ok || !allowed || item.SourceKind != "monster_drop" || item.SourceID == "" {
				return plan, p.CodeSkillBookSource
			}
			quantity, valid := multiply(m.Quantity, rule.PagesPerBook)
			if !valid {
				return plan, p.CodeSkillMaterials
			}
			// 不同原书独立输出，不合并来源或绑定，防止用堆叠抹掉输入来源。
			plan.Outputs = append(plan.Outputs, domain.Item{ItemID: rule.PageItemID, Quantity: quantity, Bound: item.Bound, Version: 1, SourceKind: "skill_recycle"})
		}
	default:
		return plan, p.CodeInvalidArgument
	}
	if plan.Spend.Gold > r.Wallet.Gold || plan.Spend.BoundGold > r.Wallet.BoundGold || plan.Spend.Yuanbao > r.Wallet.Yuanbao || plan.Spend.BoundYuanbao > r.Wallet.BoundYuanbao {
		return plan, p.CodeSkillPurchaseBlocked
	}
	remaining := len(r.Items)
	for _, m := range plan.Materials {
		if m.Quantity == m.Item.Quantity {
			remaining--
		}
	}
	split := []domain.Item{}
	for _, item := range plan.Outputs {
		definition, exists := f.Item(item.ItemID)
		if !exists {
			return plan, p.CodeInvalidArgument
		}
		maxStack := definition.MaxStack
		if maxStack <= 0 {
			maxStack = 1
		}
		for item.Quantity > 0 {
			part := item
			if part.Quantity > maxStack {
				part.Quantity = maxStack
			}
			split = append(split, part)
			item.Quantity -= part.Quantity
			if remaining+len(split) > r.Capacity {
				return plan, p.CodeSkillMaterials
			}
		}
	}
	plan.Outputs = split
	if remaining+len(plan.Outputs) > r.Capacity {
		return plan, p.CodeSkillMaterials
	}
	return plan, 0
}

func (s *Store) PreviewSkillAsset(ctx context.Context, a Auth, q *pb.SkillAssetPreviewRequest) (*pb.SkillAssetPreview, error) {
	if q == nil {
		return nil, Fail(p.CodeInvalidArgument, "缺少技能预览")
	}
	var quote skillQuote
	err := s.tx(ctx, func(tc context.Context) error {
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
		plan, code := buildSkillPlan(r, q, s.Cfg.Fixture)
		if code != 0 {
			return Fail(code, "技能预览条件变化或材料不足")
		}
		normalized := proto.Clone(q).(*pb.SkillAssetPreviewRequest)
		normalized.Materials = nil
		for _, m := range plan.Materials {
			normalized.Materials = append(normalized.Materials, &pb.ItemConsumption{InstanceId: domain.Bytes(m.Item.ID), Version: uint64(m.Item.Version), Quantity: m.Quantity})
		}
		payload, err := proto.Marshal(normalized)
		if err != nil {
			return err
		}
		quote = skillQuote{RoleID: r.ID, ID: domain.ID(), Revision: r.Revision, RuleVersion: s.Cfg.Fixture.RuleVersion, Hash: s.Cfg.Fixture.Hash, Expires: time.Now().Add(time.Duration(s.Cfg.Fixture.SkillEconomy.PreviewTTLMS) * time.Millisecond), Request: payload, Plan: plan}
		_, err = s.DB.Collection("skill_quotes").ReplaceOne(tc, bson.M{"_id": r.ID}, quote, options.Replace().SetUpsert(true))
		return err
	})
	if err != nil {
		return nil, err
	}
	return quote.Proto(s.Cfg.Fixture), nil
}

func (s *Store) commitSkillAsset(ctx context.Context, r domain.Role, q AssetRequest) (domain.Role, uint32, *skillPlan, error) {
	var quote skillQuote
	err := s.DB.Collection("skill_quotes").FindOne(ctx, bson.M{"_id": r.ID, "preview_id": q.PreviewID}).Decode(&quote)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return r, p.CodeSkillPreviewStale, nil, nil
	}
	if err != nil {
		return r, 0, nil, err
	}
	if quote.Revision != r.Revision || quote.RuleVersion != s.Cfg.Fixture.RuleVersion || quote.Hash != s.Cfg.Fixture.Hash || !time.Now().Before(quote.Expires) {
		return r, p.CodeSkillPreviewStale, nil, nil
	}
	request := &pb.SkillAssetPreviewRequest{}
	if proto.Unmarshal(quote.Request, request) != nil {
		return r, p.CodeSkillPreviewStale, nil, nil
	}
	plan, code := buildSkillPlan(r, request, s.Cfg.Fixture)
	if code != 0 {
		return r, code, nil, nil
	}
	if !reflect.DeepEqual(plan, quote.Plan) {
		return r, p.CodeSkillPreviewStale, nil, nil
	}
	out := r
	out.Items = append([]domain.Item(nil), r.Items...)
	out.Skills = append([]domain.LearnedSkill(nil), r.Skills...)
	consumption := map[string]int64{}
	for _, m := range plan.Materials {
		consumption[m.Item.ID] = m.Quantity
	}
	kept := []domain.Item{}
	for _, item := range out.Items {
		n := consumption[item.ID]
		if n == 0 {
			kept = append(kept, item)
			continue
		}
		item.Quantity -= n
		item.Version++
		if item.Quantity > 0 {
			kept = append(kept, item)
		}
	}
	out.Items = kept
	for i, item := range plan.Outputs {
		item.ID = domain.ID()
		item.SourceID = domain.Hex(q.Context.OpId)
		out.Items = append(out.Items, item)
		plan.Outputs[i] = item
	}
	out.Wallet.Gold -= plan.Spend.Gold
	out.Wallet.BoundGold -= plan.Spend.BoundGold
	out.Wallet.Yuanbao -= plan.Spend.Yuanbao
	out.Wallet.BoundYuanbao -= plan.Spend.BoundYuanbao
	if plan.Kind == 2 {
		for i, skill := range out.Skills {
			if skill.SkillID == plan.SkillID {
				out.Skills[i].Level = plan.To
			}
		}
	}
	return out, 0, &plan, nil
}

func powerBounds(s devconfig.Skill, rank uint32) (int64, int64) {
	round := func(m, power int64) int64 {
		return s.SourcePower(rank, m, power)
	}
	mpower, power := s.MPowerBase, s.PowerBase
	if s.MPowerBonus > 0 {
		mpower += s.MPowerBonus - 1
	}
	if s.PowerBonus > 0 {
		power += s.PowerBonus - 1
	}
	return round(s.MPowerBase, s.PowerBase), round(mpower, power)
}
func (q skillQuote) Proto(f devconfig.Fixture) *pb.SkillAssetPreview {
	p := q.Plan
	out := &pb.SkillAssetPreview{PreviewId: domain.Bytes(q.ID), Kind: p.Kind, RuleVersion: q.RuleVersion, ConfigHash: q.Hash, RoleRevision: uint64(q.Revision), ExpiresAtUnixMs: uint64(q.Expires.UnixMilli()), SkillId: p.SkillID, FromLevel: p.From, ToLevel: p.To, Spend: p.Spend.Proto(q.Revision), OfferId: p.OfferID, Quantity: p.Quantity}
	for _, m := range p.Materials {
		out.Materials = append(out.Materials, &pb.SkillMaterialView{Item: m.Item.Proto(), ConsumeQuantity: m.Quantity})
	}
	for _, item := range p.Outputs {
		out.Outputs = append(out.Outputs, item.Proto())
	}
	if p.Kind == 2 {
		s, _ := f.Skill(p.SkillID)
		beforeMin, beforeMax := powerBounds(s, p.From)
		afterMin, afterMax := powerBounds(s, p.To)
		out.Stats = []*pb.SkillNumericComparison{{Key: "base_power", BeforeMin: beforeMin, BeforeMax: beforeMax, AfterMin: afterMin, AfterMax: afterMax}, {Key: "accuracy_bonus", BeforeMin: s.AccuracyBonus(p.From), BeforeMax: s.AccuracyBonus(p.From), AfterMin: s.AccuracyBonus(p.To), AfterMax: s.AccuracyBonus(p.To)}, {Key: "mp_cost", BeforeMin: s.MPBaseCost + int64(p.From)*s.MPLevelCost, BeforeMax: s.MPBaseCost + int64(p.From)*s.MPLevelCost, AfterMin: s.MPBaseCost + int64(p.To)*s.MPLevelCost, AfterMax: s.MPBaseCost + int64(p.To)*s.MPLevelCost}}
		if d := s.Displacement; d != nil {
			cost := out.Stats[2]
			key := "push_distance"
			if s.Behavior == "shoulder_dash" {
				key = "dash_distance"
			}
			before, after := int64(d.DistanceBaseByLevel[p.From]), int64(d.DistanceBaseByLevel[p.To])
			distance := &pb.SkillNumericComparison{Key: key, BeforeMin: before, BeforeMax: before + d.DistanceRandomBound - 1, AfterMin: after, AfterMax: after + d.DistanceRandomBound - 1}
			if s.Behavior == "repulsion" {
				beforeChance := (d.SuccessBase + int64(p.From)*d.SuccessPerLevel) * 100 / d.SuccessDenominator
				afterChance := (d.SuccessBase + int64(p.To)*d.SuccessPerLevel) * 100 / d.SuccessDenominator
				out.Stats = []*pb.SkillNumericComparison{distance, {Key: "push_base_success_percent", BeforeMin: beforeChance, BeforeMax: beforeChance, AfterMin: afterChance, AfterMax: afterChance}, {Key: "monster_damage"}, cost}
			} else {
				out.Stats = []*pb.SkillNumericComparison{distance, out.Stats[0], cost}
			}
		}
		if b, found := f.Buff(s.BuffID); found {
			comparison := func(key string, before, after int64) *pb.SkillNumericComparison {
				return &pb.SkillNumericComparison{Key: key, BeforeMin: before, BeforeMax: before, AfterMin: after, AfterMax: after}
			}
			out.Stats = []*pb.SkillNumericComparison{comparison("mp_cost", s.MPBaseCost+int64(p.From)*s.MPLevelCost, s.MPBaseCost+int64(p.To)*s.MPLevelCost)}
			if b.Kind == "teleport_mana_penalty" {
				// Flux固定时长不使用道士逐阶秒数，也不附加伤害或护符比较项。
				out.Stats = append(out.Stats, comparison("duration_ms", b.DurationMS, b.DurationMS), comparison("teleport_mana_penalty_percent", b.ValueByLevel[p.From], b.ValueByLevel[p.To]))
			} else if b.Kind == "magic_shield" {
				out.Stats = append(out.Stats, comparison("damage_reduction_percent", b.ValueByLevel[p.From], b.ValueByLevel[p.To]), comparison("duration_rank_factor", int64(p.From)+1, int64(p.To)+1))
			} else {
				out.Stats = append(out.Stats, comparison("base_duration_seconds", b.DurationSecondsByLevel[p.From], b.DurationSecondsByLevel[p.To]), comparison("material_quantity", s.MaterialQuantity, s.MaterialQuantity))
			}
		}
		if s.Behavior == "slaying" {
			out.Stats = append(out.Stats, &pb.SkillNumericComparison{Key: "max_attack_bonus", BeforeMin: s.MaximumAttackBonus(p.From), BeforeMax: s.MaximumAttackBonus(p.From), AfterMin: s.MaximumAttackBonus(p.To), AfterMax: s.MaximumAttackBonus(p.To)})
		}
	}
	return out
}
