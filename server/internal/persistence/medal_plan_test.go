// 本文件验证原碎片、绑定授权、严格逐级和角色等级上限。
package persistence

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// medalTestStacks 构造两个已登记来源且绑定状态不同的碎片堆。
func medalTestStacks(itemID uint32) (bound, free domain.Item) {
	bound = domain.Item{ID: domain.ID(), ItemID: itemID, Quantity: 4, Version: 2, Bound: true,
		SourceKind: "world_boss", SourceID: domain.ID()}
	free = domain.Item{ID: domain.ID(), ItemID: itemID, Quantity: 5, Version: 1,
		SourceKind: "world_boss", SourceID: domain.ID()}
	return bound, free
}

// medalTestRef 使用真实堆编号和版本选择确切数量。
func medalTestRef(item domain.Item, quantity int64) *pb.MedalFragmentRef {
	return &pb.MedalFragmentRef{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version), Quantity: quantity}
}

// TestMedalGrowthPlan 验证激活与升阶只扣玩家授权的原版本和固定整数数量。
func TestMedalGrowthPlan(t *testing.T) {
	const itemID = 900061
	bound, free := medalTestStacks(itemID)
	role := domain.Role{ID: domain.ID(), Capacity: 12, Level: 2, Items: []domain.Item{bound, free}}
	fixture := devconfig.Fixture{Items: []devconfig.Item{{ID: itemID, Stackable: true, MaxStack: 128}},
		Medal: &devconfig.MedalConfig{FragmentItemID: itemID, Levels: []devconfig.MedalLevelConfig{
			{Level: 1, FragmentCost: 2, MainAttack: 1, Defense: 1}, {Level: 2, FragmentCost: 3, MainAttack: 2, Defense: 1},
		}}}
	request := &pb.MedalQuoteRequest{SceneEpoch: 1, UiSessionId: domain.Bytes(domain.ID()), AllowBound: true,
		Fragments: []*pb.MedalFragmentRef{medalTestRef(free, 1), medalTestRef(bound, 1)}}
	for _, test := range []struct {
		name   string                      // 描述该请求与原报价的具体差别。
		change func(*pb.MedalQuoteRequest) // 只修改此次复制的请求。
		want   uint32                      // 期望的稳定拒绝码。
	}{
		{"未授权绑定", func(q *pb.MedalQuoteRequest) { q.AllowBound = false }, p.CodeMedalInputs},
		{"重复原堆", func(q *pb.MedalQuoteRequest) { q.Fragments[1] = q.Fragments[0] }, p.CodeMedalInputs},
		{"原版本改变", func(q *pb.MedalQuoteRequest) { q.Fragments[0].Version++ }, p.CodeMedalInputs},
		{"缺少碎片", func(q *pb.MedalQuoteRequest) { q.Fragments = q.Fragments[:1] }, p.CodeMedalInputs},
		{"超过费用", func(q *pb.MedalQuoteRequest) { q.Fragments[0].Quantity = 2 }, p.CodeMedalInputs},
		{"不存在的勋章版本", func(q *pb.MedalQuoteRequest) { q.MedalVersion = 1 }, p.CodeMedalQuoteChanged},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := proto.CloneOf(request)
			test.change(candidate)
			if _, code := buildMedalGrowthPlan(role, &fixture, candidate); code != test.want {
				t.Fatalf("期望拒绝 %d，实际 %d", test.want, code)
			}
		})
	}
	plan, code := buildMedalGrowthPlan(role, &fixture, request)
	if code != 0 || plan.Before != nil || plan.Target.Level != 1 || !reflect.DeepEqual(medalSpendProto(plan.Fragments),
		[]*pb.MedalFragmentSpend{{InstanceId: domain.Bytes(free.ID), Version: 1, Quantity: 1},
			{InstanceId: domain.Bytes(bound.ID), Version: 2, Quantity: 1, Bound: true}}) {
		t.Fatalf("首次报价必须固定原选料顺序及绑料条款，得到 %+v，%d", plan, code)
	}
	activated := applyMedalGrowthPlan(role, plan)
	if activated.Medal == nil || activated.Medal.OwnerRoleID != role.ID || activated.Medal.Level != 1 ||
		activated.Medal.Version != 1 || activated.Items[0].Quantity != 3 || activated.Items[1].Quantity != 4 ||
		activated.Items[0].Version != 3 || activated.Items[1].Version != 2 || !reflect.DeepEqual(role.Items, []domain.Item{bound, free}) {
		t.Fatalf("原勋章或原碎片扣费结果不符，得到 %+v", activated.Medal)
	}
	request.MedalVersion, request.Fragments = 1, []*pb.MedalFragmentRef{medalTestRef(activated.Items[0], 3)}
	activated.Level = 1
	if _, code := buildMedalGrowthPlan(activated, &fixture, request); code != p.CodeMedalInputs {
		t.Fatalf("角色一级不可升二，实际拒绝码 %d", code)
	}
	activated.Level = 2
	plan, code = buildMedalGrowthPlan(activated, &fixture, request)
	if code != 0 {
		t.Fatalf("无法从已激活角色进入下一等级，拒绝码 %d", code)
	}
	next := applyMedalGrowthPlan(activated, plan)
	if next.Medal.OwnerRoleID != role.ID || next.Medal.Level != 2 || next.Medal.Version != 2 || len(next.Items) != 1 ||
		next.Items[0].ID != free.ID || next.Items[0].Quantity != 4 || activated.Medal.Level != 1 {
		t.Fatalf("升阶必须复用唯一勋章，且只消耗原绑定堆，得到 %+v", next.Medal)
	}
}
