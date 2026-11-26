// 自动回收策略和单次收益内的回收结果。按保护条件筛选库存与新掉落，计算绑定归属、货币收入和原实例去向。

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
// EquipmentRecyclingReceipt 固定批量回收使用的策略、输入实例、回收金币和跳过原因。
type EquipmentRecyclingReceipt struct {
	// PolicyRevision 记录策略修订号，使用数值表示。
	PolicyRevision int64 `bson:"policy_revision"`
	// Policy 记录策略，使用自动回收策略表示。
	Policy *domain.AutoRecyclePolicy `bson:"policy,omitempty"`
	// Recycled 记录“recycled”，使用道具列表表示。
	Recycled []domain.Item `bson:"recycled"`
	// Gold 记录金币，使用数值表示。
	Gold int64 `bson:"gold"`
	// BoundGold 记录绑定金币，使用数值表示。
	BoundGold int64 `bson:"bound_gold"`
	// DiscardedCount 记录“discarded”数量，使用数值表示。
	DiscardedCount int64 `bson:"discarded_count"`
	// DiscardReason 记录丢弃原因，使用文本表示。
	DiscardReason string `bson:"discard_reason"`
	// Skipped 记录跳过，使用回收“skip”列表表示。
	Skipped []RecycleSkip `bson:"skipped,omitempty"`
	// Trigger 记录触发条件，使用数值表示。
	Trigger uint32 `bson:"trigger"`
	// OilRolls 保存此次实际手动分解中合格原件的逐件抽样和实际产物。
	OilRolls []RecycleOilRoll `bson:"oil_rolls,omitempty"`
}

// policySnapshot 复制回收策略及其名单切片，避免后续修改影响已确认回执。
// policy 记录策略，使用自动回收策略表示。
func policySnapshot(policy domain.AutoRecyclePolicy) *domain.AutoRecyclePolicy {
	// frozen 保存由副本取得的已冻结状态。
	frozen := policy.Clone()
	return &frozen
}

// RecycleSkip 记录一件未被回收的原实例及具体保护或拒绝原因。
type RecycleSkip struct {
	// InstanceID 标识当前业务使用的进程或道具实例。
	InstanceID string `bson:"instance_id"`
	// Reason 记录原因，使用文本表示。
	Reason string `bson:"reason"`
}

// policyFromProto 将协议策略转换为领域模型，核查配置支持、阈值和名单范围。
// x 记录自动回收策略，f 记录开发测试配置。
func policyFromProto(x *pb.AutoRecyclePolicy, f devconfig.Fixture) (domain.AutoRecyclePolicy, uint32) {
	if x == nil || x.Revision > math.MaxInt64 || len(x.QualityIds) > 5 || len(x.ProtectedItemIds) > len(f.Items) || x.Enabled && len(x.QualityIds) == 0 {
		return domain.AutoRecyclePolicy{}, p.CodeInvalidArgument
	}
	// q 组装自动回收策略所需的数据。
	q := domain.AutoRecyclePolicy{Enabled: x.Enabled, QualityIDs: append([]uint32(nil), x.QualityIds...), MaxRequiredLevel: x.MaxRequiredLevel, ProtectedItemIDs: append([]uint32(nil), x.ProtectedItemIds...), ProtectBound: x.ProtectBound, ProtectUnbound: x.ProtectUnbound, Revision: int64(x.Revision)}
	// seen 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	seen := map[uint32]bool{}
	// quality 接收遍历品质i“ds”时当前的元素。
	for _, quality := range q.QualityIDs {
		if quality > 4 || seen[quality] {
			return q, p.CodeInvalidArgument
		}
		seen[quality] = true
	}
	seen = map[uint32]bool{}
	// id 接收遍历“protected”道具i“ds”时当前的元素。
	for _, id := range q.ProtectedItemIDs {
		// d 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
		d, ok := f.Item(id)
		if !ok || !d.IsEquipment || seen[id] {
			return q, p.CodeInvalidArgument
		}
		seen[id] = true
	}
	// i 记录当前位置的下标，使用数值表示，j 记录当前位置的下标，使用数值表示。
	sort.Slice(q.QualityIDs, func(i, j int) bool { return q.QualityIDs[i] < q.QualityIDs[j] })
	// i 记录当前位置的下标，使用数值表示，j 记录当前位置的下标，使用数值表示。
	sort.Slice(q.ProtectedItemIDs, func(i, j int) bool { return q.ProtectedItemIDs[i] < q.ProtectedItemIDs[j] })
	return q, 0
}

// containsID 判断策略列表是否包含某个物品定义。
// ids 记录标识集合，使用数值列表表示，id 区分当前步骤指向的目标对象。
func containsID(ids []uint32, id uint32) bool {
	// v 接收遍历标识集合时当前的元素。
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// recyclePrice 验证候选实例和所有保护规则，计算可接受的回收价。
// r 记录角色，item 记录道具，f 记录开发测试配置。
func recyclePrice(r domain.Role, item domain.Item, f devconfig.Fixture) (int64, string) {
	if r.RecycleBlockFlags(item) != 0 {
		return 0, "protected"
	}
	// d 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
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
		// c 保存当前处理所用的装备自动回收。
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

// autoRecycleMatches 判断物品的等级、品质和定义身份是否落在当前自动回收规则内。
// r 记录角色，item 记录道具，f 记录开发测试配置。
func autoRecycleMatches(r domain.Role, item domain.Item, f devconfig.Fixture) bool {
	// q 保存当前处理所用的自动回收。
	q := r.AutoRecycle
	if !q.Enabled || !containsID(q.QualityIDs, item.Quality) || containsID(q.ProtectedItemIDs, item.ItemID) || item.Bound && q.ProtectBound || !item.Bound && q.ProtectUnbound {
		return false
	}
	// d 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
	d, ok := f.Item(item.ItemID)
	if !ok || !d.IsEquipment {
		return false
	}
	// 缺来源需求等级不等价于0，即使策略未设等级上限也不能吞掉该原件。
	return d.RequiredLevel == nil || q.MaxRequiredLevel == 0 || *d.RequiredLevel <= q.MaxRequiredLevel
}

// creditRecycled 按原物品的绑定状态累加回收款并登记回收实例。
// out 记录输出，使用角色表示，receipt 记录回执，使用装备自动回收回执表示，item 记录道具，gold 记录金币，使用数值表示。
func creditRecycled(out *domain.Role, receipt *EquipmentRecyclingReceipt, item domain.Item, gold int64) error {
	// err 保存当前操作的失败原因，供调用方检查或返回。
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
// recycleExisting 按已保存的自动策略检查并回收角色当前背包内的候选物品。
// r 记录角色，f 记录开发测试配置，receipt 记录回执，使用装备自动回收回执表示。
func recycleExisting(r domain.Role, f devconfig.Fixture, receipt *EquipmentRecyclingReceipt) (domain.Role, error) {
	if !r.AutoRecycle.Enabled || f.RuleVersion < 19 {
		return r, nil
	}
	// out 保存当前处理所用的输出。
	out := r
	out.Items = make([]domain.Item, 0, len(r.Items))
	// item 接收遍历道具集合时当前的元素。
	for _, item := range r.Items {
		if !autoRecycleMatches(r, item, f) {
			out.Items = append(out.Items, item)
			continue
		}
		// gold 保存由回收“price”取得的金币，reason 保存由回收“price”取得的原因。
		gold, reason := recyclePrice(r, item, f)
		if reason != "" {
			receipt.Skipped = append(receipt.Skipped, RecycleSkip{item.ID, reason})
			out.Items = append(out.Items, item)
			continue
		}
		// err 接收入账“recycled”过程中可能产生的错误，供后续失败分支处理。
		if err := creditRecycled(&out, receipt, item, gold); err != nil {
			return r, err
		}
	}
	return out, nil
}

// recycleManual 先核对全部原装备，再销毁装备并发放各自绑定的回收金币和分解产物。
func recycleManual(r domain.Role, ids []string, f devconfig.Fixture, oil *recycleOilAttempt) (domain.Role, *EquipmentRecyclingReceipt, uint32, error) {
	if len(ids) == 0 || len(ids) > r.Capacity {
		return r, nil, p.CodeInvalidArgument, nil
	}
	wanted := make(map[string]bool, len(ids)) // 首次处理每个原实例，缺失或重复均拒绝整笔分解。
	for _, id := range ids {
		if len(domain.Bytes(id)) != 16 || wanted[id] {
			return r, nil, p.CodeInvalidArgument, nil
		}
		wanted[id] = true
	}
	type input struct {
		item domain.Item // 通过原实例、持有情况和回收保护检查的装备。
		gold int64       // 按原规则确认的该件金币。
	}
	planned := make([]input, 0, len(ids)) // 全部确认后才创建审核回执，不记录被拒绝操作的部分销毁。
	out := r
	out.Items = make([]domain.Item, 0, len(r.Items))
	for _, item := range r.Items {
		if !wanted[item.ID] {
			out.Items = append(out.Items, item)
			continue
		}
		gold, reason := recyclePrice(r, item, f)
		if reason != "" {
			return r, nil, p.CodeInvalidArgument, nil
		}
		if f.EquipmentRecycleOil != nil && item.DurabilityU <= 0 &&
			item.MaxDurabilityU > 0 && item.OriginalDurabilityU >= item.MaxDurabilityU {
			gold = 0 // 已有持久上限的原件在零或历史负值时明确破损，可销毁但不发金币或油。
		}
		planned = append(planned, input{item: item, gold: gold})
	}
	if len(planned) != len(wanted) {
		return r, nil, p.CodeInvalidArgument, nil
	}
	receipt := &EquipmentRecyclingReceipt{PolicyRevision: r.AutoRecycle.Revision, Trigger: 3}
	for _, plan := range planned {
		if err := creditRecycled(&out, receipt, plan.item, plan.gold); err != nil {
			return r, nil, 0, err
		}
	}
	if err := applyManualRecycleOil(r, &out, receipt, &f, oil); err != nil {
		return r, nil, 0, err
	}
	return out, receipt, 0, nil
}

// setAutoRecycle 保存新回收策略并按请求决定是否立即处理既有物品。
// r 记录角色，q 记录资产请求，f 记录开发测试配置。
func setAutoRecycle(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, *EquipmentRecyclingReceipt, uint32, error) {
	// policy 保存由策略来源协议取得的策略，code 保存由策略来源协议取得的结果码。
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
	// out 保存当前处理所用的输出。
	out := r
	out.AutoRecycle = policy
	// receipt 保存当前处理所用的回执。
	receipt := &EquipmentRecyclingReceipt{PolicyRevision: policy.Revision, Policy: policySnapshot(policy), Trigger: 2}
	if q.ApplyExisting {
		// err 保存当前操作的失败原因，供调用方检查或返回。
		var err error
		out, err = recycleExisting(out, f, receipt)
		if err != nil {
			return r, nil, 0, err
		}
	}
	return out, receipt, 0, nil
}

// setItemLock 按期望实例版本修改物品锁定状态，以改变经济操作的保护资格。
// r 记录角色，q 记录资产请求，f 记录开发测试配置。
func setItemLock(r domain.Role, q AssetRequest, f devconfig.Fixture) (domain.Role, uint32) {
	if f.RuleVersion < 19 || q.ItemVersion == 0 || q.ItemVersion >= math.MaxInt64 || len(domain.Bytes(q.ItemID)) != 16 {
		return r, p.CodeInvalidArgument
	}
	// index 保存当前处理所用的下标，worn 保存当前处理所用的“worn”，count 规定数量的初始取值。
	index, worn, count := -1, false, 0
	// i 定位道具集合中正在处理的成员，v 接收遍历道具集合时当前的元素。
	for i, v := range r.Items {
		if v.ID == q.ItemID {
			index = i
			count++
		}
	}
	// i 定位已穿戴装备中正在处理的成员，v 接收遍历已穿戴装备时当前的元素。
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
	// item 记录道具。
	var item domain.Item
	if worn {
		item = r.Equipped[index].Item
	} else {
		item = r.Items[index]
	}
	// d 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
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
