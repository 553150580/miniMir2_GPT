// 本文件按角色等级、原勋章版本与玩家选定的碎片原堆核算下一等级。
package persistence

import (
	"math"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// medalFragmentSpend 记住玩家实际确认的一笔原堆用量和绑定状态。
type medalFragmentSpend struct {
	ID       string `bson:"instance_id"` // 被选中的原背包堆编号。
	Version  int64  `bson:"version"`     // 消耗前的原背包堆版本。
	Quantity int64  `bson:"quantity"`    // 该堆在本次升级真正消耗的数量。
	Bound    bool   `bson:"bound"`       // 产出时已经确认的绑定状态。
}

// medalGrowthPlan 冻结当前与目标档案和全部确切投入，提交时不换料。
type medalGrowthPlan struct {
	Before    *domain.MedalState         `bson:"before,omitempty"` // 为空时先激活唯一角色档案。
	Target    devconfig.MedalLevelConfig `bson:"target"`           // 目标等级的权威价格与属性。
	Fragments []medalFragmentSpend       `bson:"fragments"`        // 原选择顺序中的每个实际碎片堆。
}

// ValidMedalQuoteRequest 核对场景、面板会话和原勋章版本的协议形状。
func ValidMedalQuoteRequest(request *pb.MedalQuoteRequest) bool {
	return request != nil && request.SceneEpoch != 0 && len(request.UiSessionId) == 16 &&
		request.MedalVersion < math.MaxInt64 && len(request.Fragments) > 0 &&
		len(request.ProtoReflect().GetUnknown()) == 0
}

// buildMedalGrowthPlan 只用当前角色、工作簿投影和原堆构造一次严格逐级的升级。
func buildMedalGrowthPlan(role domain.Role, fixture *devconfig.Fixture, request *pb.MedalQuoteRequest) (medalGrowthPlan, uint32) {
	rules := fixture.Medal // 缺省规则表示当前运行服未开放。
	if rules == nil {
		return medalGrowthPlan{}, p.CodeMedalUnavailable
	}
	if !ValidMedalQuoteRequest(request) || len(request.Fragments) > role.Capacity || !equipmentRoleUnique(role) {
		return medalGrowthPlan{}, p.CodeInvalidArgument
	}
	targetLevel := uint32(1) // 未激活角色的首笔确认直接达到一级。
	if medal := role.Medal; medal != nil {
		if medal.OwnerRoleID != role.ID || medal.Version <= 0 || uint64(medal.Version) != request.MedalVersion {
			return medalGrowthPlan{}, p.CodeMedalQuoteChanged
		}
		if medal.Level == 0 || medal.Level >= role.Level {
			return medalGrowthPlan{}, p.CodeMedalInputs
		}
		targetLevel = medal.Level + 1
	} else if request.MedalVersion != 0 {
		return medalGrowthPlan{}, p.CodeMedalQuoteChanged
	}
	if targetLevel > role.Level {
		return medalGrowthPlan{}, p.CodeMedalInputs
	}
	target, configured := rules.Level(targetLevel)
	definition, found := fixture.Item(rules.FragmentItemID)
	if !configured || target.FragmentCost <= 0 || !found || definition.IsEquipment || !definition.Stackable || definition.MaxStack <= 0 {
		return medalGrowthPlan{}, p.CodeMedalUnavailable
	}

	owned := make(map[string]domain.Item, len(role.Items)) // 按真实原实例查找角色背包内的投入。
	for _, item := range role.Items {
		owned[item.ID] = item
	}
	fragments := make([]medalFragmentSpend, 0, len(request.Fragments))
	remaining := target.FragmentCost // 按笔减少应付总量，并避免在输入边界求和溢出。
	for _, reference := range request.Fragments {
		if reference == nil || len(reference.InstanceId) != 16 || reference.Version == 0 || reference.Version >= math.MaxInt64 ||
			reference.Quantity <= 0 || reference.Quantity > remaining || len(reference.ProtoReflect().GetUnknown()) != 0 {
			return medalGrowthPlan{}, p.CodeMedalInputs
		}
		id := domain.Hex(reference.InstanceId)
		item, available := owned[id]
		if !available || !validGrowthMaterial(item, definition) || uint64(item.Version) != reference.Version ||
			reference.Quantity > item.Quantity || item.Bound && !request.AllowBound {
			return medalGrowthPlan{}, p.CodeMedalInputs
		}
		delete(owned, id) // 一堆只允许选择一次，避免重复输入导致错误扣量。
		fragments = append(fragments, medalFragmentSpend{ID: id, Version: item.Version, Quantity: reference.Quantity, Bound: item.Bound})
		remaining -= reference.Quantity
	}
	if remaining != 0 {
		return medalGrowthPlan{}, p.CodeMedalInputs
	}
	return medalGrowthPlan{Before: role.Medal, Target: target, Fragments: fragments}, 0
}

// applyMedalGrowthPlan 对已复核的原堆扣料，只递增角色唯一勋章的一阶和一个版本。
func applyMedalGrowthPlan(role domain.Role, plan medalGrowthPlan) domain.Role {
	medal := domain.MedalState{OwnerRoleID: role.ID}
	if plan.Before != nil {
		medal = *plan.Before
	}
	medal.Level, medal.Version = plan.Target.Level, medal.Version+1
	uses := make(map[string]int64, len(plan.Fragments)) // 已验证每个编号最多对应一笔。
	for _, fragment := range plan.Fragments {
		uses[fragment.ID] = fragment.Quantity
	}
	role.Items, role.Medal = spendGrowthMaterials(role.Items, uses), &medal
	return role
}

// medalSpendProto 将确切的原碎片堆和消费数量返回报价面板和永久回执。
func medalSpendProto(spent []medalFragmentSpend) []*pb.MedalFragmentSpend {
	out := make([]*pb.MedalFragmentSpend, 0, len(spent))
	for _, fragment := range spent {
		out = append(out, &pb.MedalFragmentSpend{InstanceId: domain.Bytes(fragment.ID), Version: uint64(fragment.Version),
			Quantity: fragment.Quantity, Bound: fragment.Bound})
	}
	return out
}
