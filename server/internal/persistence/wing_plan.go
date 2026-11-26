// 本文件在报价和提交时核对原羽毛、原翅膀、逐阶费用及绑定授权。
package persistence

import (
	"math"
	"slices"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const (
	wingActionGrow       = 1 // 激活未拥有的翅膀，或给同一实例提升下一阶。
	wingActionAppearance = 2 // 更新角色已经解锁的外观及隐藏状态。
)

// wingFeatherSpend 冻结所选原羽毛堆和实际扣除的数量，原堆可被部分消耗。
type wingFeatherSpend struct {
	Item     domain.Item `bson:"item"`     // 玩家确认时完整的原羽毛堆。
	Quantity int64       `bson:"quantity"` // 本次实际从原堆扣除的整数数量。
}

// wingGrowthPlan 定义 0→1 或下一阶报价中所有已核验的事实。
type wingGrowthPlan struct {
	Before               *domain.WingState  `bson:"before,omitempty"`       // nil 表示首次激活。
	TargetRank           uint32             `bson:"target_rank"`            // 原报价承诺且只会上升一阶的目标。
	TargetAppearanceTier uint32             `bson:"target_appearance_tier"` // 本次外观进化后的可见设置。
	Feathers             []wingFeatherSpend `bson:"feathers"`               // 按请求顺序存下全部原材料。
	AllowBound           bool               `bson:"allow_bound"`            // 此次报价是否允许扣绑定羽毛。
}

// validWingRef 区分未激活与已拥有实例，阻止缺失、溢出或伪造版本的引用。
func validWingRef(id []byte, version uint64, allowEmpty bool) bool {
	if len(id) == 0 {
		return allowEmpty && version == 0
	}
	return len(id) == 16 && version > 0 && version < math.MaxInt64
}

// validWingQuoteRequest 检查当前原场景、界面会话与基本原件形状，实际成本由服务端查询。
func validWingQuoteRequest(q *pb.WingQuoteRequest) bool {
	return q != nil && q.SceneEpoch != 0 && len(q.UiSessionId) == 16 && len(q.Feathers) > 0 &&
		validWingRef(q.WingId, q.WingVersion, true) && len(q.ProtoReflect().GetUnknown()) == 0
}

// wingRefMatches 核对原翅膀状态。没有存续实例只能使用空目标激活一次。
func wingRefMatches(wing *domain.WingState, id []byte, version uint64) bool {
	if wing == nil {
		return len(id) == 0 && version == 0
	}
	return wing.ID == domain.Hex(id) && version == uint64(wing.Version)
}

// validWingFeather 校验实际的角色背包材料，允许合规的世界首领、兑换及其他已登记来源。
func validWingFeather(item domain.Item, definition devconfig.Item) bool {
	return item.ItemID == definition.ID && item.Quantity > 0 && item.Quantity <= definition.MaxStack &&
		item.Version > 0 && item.Version < math.MaxInt64 && !item.Locked && !equipmentOccupied(item) &&
		item.SourceKind != "" && item.SourceID != "" && item.OrePurity == 0 && item.InitialLuck == 0 && item.CurrentLuck == 0 &&
		item.LuckOilToSix == 0 && item.LuckOilToNine == 0 && item.DurabilityU == 0 && item.MaxDurabilityU == 0 && item.OriginalDurabilityU == 0
}

// buildWingGrowthPlan 以原角色快照计算一笔可重复核对的逐阶用料。空白阶位不产生零价升级。
func buildWingGrowthPlan(role domain.Role, fixture *devconfig.Fixture, q *pb.WingQuoteRequest) (wingGrowthPlan, uint32) {
	rules := fixture.Wings
	if rules == nil {
		return wingGrowthPlan{}, p.CodeWingUnavailable
	}
	if !validWingQuoteRequest(q) || len(q.Feathers) > role.Capacity || !equipmentRoleUnique(role) {
		return wingGrowthPlan{}, p.CodeInvalidArgument
	}
	if !wingRefMatches(role.Wing, q.WingId, q.WingVersion) {
		return wingGrowthPlan{}, p.CodeWingQuoteChanged
	}
	plan := wingGrowthPlan{Before: role.Wing, TargetRank: 1, TargetAppearanceTier: 1,
		Feathers: make([]wingFeatherSpend, 0, len(q.Feathers)), AllowBound: q.AllowBound}
	if role.Wing != nil {
		if !validRoleWing(role.Wing, role.ID, rules) || role.Wing.Rank >= rules.MaxRank {
			return wingGrowthPlan{}, p.CodeWingInputs
		}
		plan.TargetRank = role.Wing.Rank + 1
		plan.TargetAppearanceTier = role.Wing.AppearanceTier
		if slices.Contains(rules.AppearanceTiers, plan.TargetRank) {
			plan.TargetAppearanceTier = plan.TargetRank
		}
	}
	var remaining int64 // 本阶明确登记的羽毛数。每次扣入报价都逐项减少，避免求和溢出。
	for _, rank := range rules.Ranks {
		if rank.Rank == plan.TargetRank {
			remaining = rank.Feathers
			break
		}
	}
	definition, found := fixture.Item(rules.FeatherItemID)
	if remaining <= 0 || !found || definition.IsEquipment || !definition.Stackable || definition.MaxStack <= 0 {
		return wingGrowthPlan{}, p.CodeWingUnavailable
	}
	owned := make(map[string]domain.Item, len(role.Items))
	for _, item := range role.Items {
		owned[item.ID] = item
	}
	for _, ref := range q.Feathers {
		if ref == nil || len(ref.InstanceId) != 16 || ref.Version == 0 || ref.Version >= math.MaxInt64 ||
			ref.Quantity <= 0 || ref.Quantity > remaining || len(ref.ProtoReflect().GetUnknown()) != 0 {
			return wingGrowthPlan{}, p.CodeWingInputs
		}
		id := domain.Hex(ref.InstanceId)
		item, found := owned[id]
		if !found || !validWingFeather(item, definition) || uint64(item.Version) != ref.Version ||
			ref.Quantity > item.Quantity || item.Bound && !q.AllowBound {
			return wingGrowthPlan{}, p.CodeWingInputs
		}
		delete(owned, id) // 同一原堆只能出现一次，避免重复扣取或重解释原授权。
		plan.Feathers = append(plan.Feathers, wingFeatherSpend{Item: item, Quantity: ref.Quantity})
		remaining -= ref.Quantity
	}
	if remaining != 0 {
		return wingGrowthPlan{}, p.CodeWingInputs
	}
	return plan, 0
}

// applyWingGrowthPlan 严格落实已核验的原消费。升阶时复制原实例，仅调整阶数和应解锁的外观。
func applyWingGrowthPlan(role domain.Role, plan wingGrowthPlan, operationID string) (domain.Role, domain.WingState) {
	wing := domain.WingState{ID: Hash([]byte("wing-activate:" + operationID))[:32], OwnerRoleID: role.ID}
	if plan.Before != nil {
		wing = *plan.Before
	}
	wing.Rank, wing.AppearanceTier = plan.TargetRank, plan.TargetAppearanceTier
	wing.Version++
	uses := make(map[string]int64, len(plan.Feathers))
	for _, material := range plan.Feathers {
		uses[material.Item.ID] = material.Quantity
	}
	items := make([]domain.Item, 0, len(role.Items))
	for _, item := range role.Items {
		amount, used := uses[item.ID]
		if used {
			item.Quantity -= amount
			item.Version++
			if item.Quantity == 0 {
				continue
			}
		}
		items = append(items, item)
	}
	role.Items, role.Wing = items, &wing
	return role, wing
}

// wingSpendProto 发布原羽毛堆和确认扣量，以便前端逐笔核对原绑定和材料来源。
func wingSpendProto(spent []wingFeatherSpend) []*pb.WingFeatherSpend {
	if len(spent) == 0 {
		return nil
	}
	out := make([]*pb.WingFeatherSpend, 0, len(spent))
	for _, material := range spent {
		out = append(out, &pb.WingFeatherSpend{Item: material.Item.Proto(), Quantity: material.Quantity})
	}
	return out
}
