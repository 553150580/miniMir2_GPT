package persistence

import (
	"math"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"sort"
)

// 回执逐原件冻结身份和绑定；不从重试后的钱包差额反推奖励。
type EquipmentRecyclingReceipt struct {
	PolicyRevision int64                     `bson:"policy_revision"`
	Policy         *domain.AutoRecyclePolicy `bson:"policy,omitempty"`
	Recycled       []domain.Item             `bson:"recycled"`
	Gold           int64                     `bson:"gold"`
	BoundGold      int64                     `bson:"bound_gold"`
	DiscardedCount int64                     `bson:"discarded_count"`
	DiscardReason  string                    `bson:"discard_reason"`
	Skipped        []RecycleSkip             `bson:"skipped,omitempty"`
	Trigger        uint32                    `bson:"trigger"`
}

func policySnapshot(policy domain.AutoRecyclePolicy) *domain.AutoRecyclePolicy {
	frozen := policy.Clone()
	return &frozen
}

type RecycleSkip struct {
	InstanceID string `bson:"instance_id"`
	Reason     string `bson:"reason"`
}

func policyFromProto(x *pb.AutoRecyclePolicy, f devconfig.Fixture) (domain.AutoRecyclePolicy, uint32) {
	if x == nil || x.Revision > math.MaxInt64 || len(x.QualityIds) > 5 || len(x.ProtectedItemIds) > len(f.Items) || x.Enabled && len(x.QualityIds) == 0 {
		return domain.AutoRecyclePolicy{}, p.CodeInvalidArgument
	}
	q := domain.AutoRecyclePolicy{Enabled: x.Enabled, QualityIDs: append([]uint32(nil), x.QualityIds...), MaxRequiredLevel: x.MaxRequiredLevel, ProtectedItemIDs: append([]uint32(nil), x.ProtectedItemIds...), ProtectBound: x.ProtectBound, ProtectUnbound: x.ProtectUnbound, Revision: int64(x.Revision)}
	seen := map[uint32]bool{}
	for _, quality := range q.QualityIDs {
		if quality > 4 || seen[quality] {
			return q, p.CodeInvalidArgument
		}
		seen[quality] = true
	}
	seen = map[uint32]bool{}
	for _, id := range q.ProtectedItemIDs {
		d, ok := f.Item(id)
		if !ok || !d.IsEquipment || seen[id] {
			return q, p.CodeInvalidArgument
		}
		seen[id] = true
	}
	sort.Slice(q.QualityIDs, func(i, j int) bool { return q.QualityIDs[i] < q.QualityIDs[j] })
	sort.Slice(q.ProtectedItemIDs, func(i, j int) bool { return q.ProtectedItemIDs[i] < q.ProtectedItemIDs[j] })
	return q, 0
}

func containsID(ids []uint32, id uint32) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func recyclePrice(r domain.Role, item domain.Item, f devconfig.Fixture) (int64, string) {
	if r.RecycleBlockFlags(item) != 0 {
		return 0, "protected"
	}
	d, ok := f.Item(item.ItemID)
	if !ok || !d.IsEquipment || item.Quantity != 1 || item.Version < 1 || len(domain.Bytes(item.ID)) != 16 {
		return 0, "invalid_equipment"
	}
	if item.Quality > 4 || item.Quality != d.DropQuality {
		return 0, "quality_unknown"
	}
	if f.RuleVersion >= 19 {
		if d.RequiredLevel == nil {
			return 0, "required_level_unknown"
		}
		if !d.EquipmentRecycleKnown() {
			return 0, "recipe_unknown"
		}
		c := f.EquipmentRecycling
		if c == nil || c.Mode != "development_gold_only" || c.Outputs == nil || len(c.Outputs) != 0 {
			return 0, "recipe_unknown"
		}
	}
	if d.RecycleGold < 0 {
		return 0, "recipe_unknown"
	}
	return d.RecycleGold, ""
}

func autoRecycleMatches(r domain.Role, item domain.Item, f devconfig.Fixture) bool {
	q := r.AutoRecycle
	if !q.Enabled || !containsID(q.QualityIDs, item.Quality) || containsID(q.ProtectedItemIDs, item.ItemID) || item.Bound && q.ProtectBound || !item.Bound && q.ProtectUnbound {
		return false
	}
	d, ok := f.Item(item.ItemID)
	if !ok || !d.IsEquipment {
		return false
	}
	// 缺来源需求等级不等价于0，即使策略未设等级上限也不能吞掉该原件。
	return d.RequiredLevel == nil || q.MaxRequiredLevel == 0 || *d.RequiredLevel <= q.MaxRequiredLevel
}

func creditRecycled(out *domain.Role, receipt *EquipmentRecyclingReceipt, item domain.Item, gold int64) error {
	var err error
	if item.Bound {
		receipt.BoundGold, err = add(receipt.BoundGold, gold)
		if err == nil {
			out.Wallet.BoundGold, err = add(out.Wallet.BoundGold, gold)
		}
	} else {
		receipt.Gold, err = add(receipt.Gold, gold)
		if err == nil {
			out.Wallet.Gold, err = add(out.Wallet.Gold, gold)
		}
	}
	if err != nil {
		return err
	}
	receipt.Recycled = append(receipt.Recycled, item)
	return nil
}

// 只返回新切片，失败不得污染原角色。数量受当前背包容量限制。
func recycleExisting(r domain.Role, f devconfig.Fixture, receipt *EquipmentRecyclingReceipt) (domain.Role, error) {
	if !r.AutoRecycle.Enabled || f.RuleVersion < 19 {
		return r, nil
	}
	out := r
	out.Items = make([]domain.Item, 0, len(r.Items))
	for _, item := range r.Items {
		if !autoRecycleMatches(r, item, f) {
			out.Items = append(out.Items, item)
			continue
		}
		gold, reason := recyclePrice(r, item, f)
		if reason != "" {
			receipt.Skipped = append(receipt.Skipped, RecycleSkip{item.ID, reason})
			out.Items = append(out.Items, item)
			continue
		}
		if err := creditRecycled(&out, receipt, item, gold); err != nil {
			return r, err
		}
	}
	return out, nil
}

func recycleManual(r domain.Role, ids []string, f devconfig.Fixture) (domain.Role, *EquipmentRecyclingReceipt, uint32, error) {
	receipt := &EquipmentRecyclingReceipt{PolicyRevision: r.AutoRecycle.Revision, Trigger: 3}
	if len(ids) == 0 || len(ids) > r.Capacity {
		return r, receipt, p.CodeInvalidArgument, nil
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		if len(domain.Bytes(id)) != 16 || wanted[id] {
			return r, receipt, p.CodeInvalidArgument, nil
		}
		wanted[id] = true
	}
	out := r
	out.Items = make([]domain.Item, 0, len(r.Items))
	count := 0
	for _, item := range r.Items {
		if !wanted[item.ID] {
			out.Items = append(out.Items, item)
			continue
		}
		count++
		gold, reason := recyclePrice(r, item, f)
		if reason != "" {
			return r, receipt, p.CodeInvalidArgument, nil
		}
		if err := creditRecycled(&out, receipt, item, gold); err != nil {
			return r, receipt, 0, err
		}
	}
	if count != len(wanted) {
		return r, receipt, p.CodeInvalidArgument, nil
	}
	return out, receipt, 0, nil
}

func setAutoRecycle(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, *EquipmentRecyclingReceipt, uint32, error) {
	policy, code := policyFromProto(q.AutoRecycle, f)
	if f.RuleVersion < 19 || code != 0 || policy.Revision != r.AutoRecycle.Revision || policy.Revision == math.MaxInt64 || q.ApplyExisting && !policy.Enabled {
		if code == 0 && policy.Revision != r.AutoRecycle.Revision {
			code = p.CodeOperationConflict
		}
		if code == 0 {
			code = p.CodeInvalidArgument
		}
		return r, nil, code, nil
	}
	policy.Revision++
	out := r
	out.AutoRecycle = policy
	receipt := &EquipmentRecyclingReceipt{PolicyRevision: policy.Revision, Policy: policySnapshot(policy), Trigger: 2}
	if q.ApplyExisting {
		var err error
		out, err = recycleExisting(out, f, receipt)
		if err != nil {
			return r, nil, 0, err
		}
	}
	return out, receipt, 0, nil
}

func setItemLock(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, uint32) {
	if f.RuleVersion < 19 || q.ItemVersion == 0 || q.ItemVersion >= math.MaxInt64 || len(domain.Bytes(q.ItemID)) != 16 {
		return r, p.CodeInvalidArgument
	}
	index, worn, count := -1, false, 0
	for i, v := range r.Items {
		if v.ID == q.ItemID {
			index = i
			count++
		}
	}
	for i, v := range r.Equipped {
		if v.Item.ID == q.ItemID {
			index = i
			worn = true
			count++
		}
	}
	if count != 1 {
		return r, p.CodeInvalidArgument
	}
	var item domain.Item
	if worn {
		item = r.Equipped[index].Item
	} else {
		item = r.Items[index]
	}
	d, ok := f.Item(item.ItemID)
	if !ok || !d.IsEquipment || uint64(item.Version) != q.ItemVersion {
		return r, p.CodeOperationConflict
	}
	item.Locked = q.Locked
	item.Version++
	if worn {
		r.Equipped = append([]domain.Equipped(nil), r.Equipped...)
		r.Equipped[index].Item = item
	} else {
		r.Items = append([]domain.Item(nil), r.Items...)
		r.Items[index] = item
	}
	return r, 0
}
