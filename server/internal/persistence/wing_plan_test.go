// 本文件核对翅膀逐阶、真实羽毛选择及外观边界，不依赖外部数据库。
package persistence

import (
	"reflect"
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// wingTestFeatherRef 将原物品固定为报价需要的原编号、版本和本次扣量。
func wingTestFeatherRef(item domain.Item, amount int64) *pb.WingFeatherRef {
	return &pb.WingFeatherRef{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version), Quantity: amount}
}

// TestWingGrowthPlanOwnsExactFeathers 核对两个原堆、已有幸运和待定阶段。
func TestWingGrowthPlanOwnsExactFeathers(t *testing.T) {
	fixture := devconfig.Fixture{
		Wings: &devconfig.WingConfig{FeatherItemID: 61, MaxRank: 10, AppearanceTiers: []uint32{1, 3, 5, 7, 10},
			Ranks: []devconfig.WingRankConfig{{Rank: 1, Feathers: 1}, {Rank: 2, Feathers: 1}, {Rank: 3, Feathers: 3}}},
		Items: []devconfig.Item{{ID: 61, Name: "羽毛", Stackable: true, MaxStack: 99}},
	}
	wing := domain.WingState{ID: domain.ID(), OwnerRoleID: domain.ID(), Version: 8, Rank: 2, AppearanceTier: 1,
		Hidden: true, Luck: 7, OilToSix: 111, OilToNine: 99}
	bound := domain.Item{ID: domain.ID(), ItemID: 61, SourceKind: "world_boss", SourceID: "1", Quantity: 1, Bound: true, Version: 8}
	free := domain.Item{ID: domain.ID(), ItemID: 61, SourceKind: "arena_exchange", SourceID: "1", Quantity: 3, Version: 4}
	wrong := domain.Item{ID: domain.ID(), ItemID: 62, SourceKind: "world_boss", SourceID: "1", Quantity: 3, Version: 4}
	role := domain.Role{ID: wing.OwnerRoleID, Wing: &wing, Items: []domain.Item{bound, free, wrong}, Capacity: 3}
	request := func() *pb.WingQuoteRequest {
		return &pb.WingQuoteRequest{SceneEpoch: 32, UiSessionId: domain.Bytes(domain.ID()), WingId: domain.Bytes(wing.ID),
			WingVersion: uint64(wing.Version), AllowBound: true,
			Feathers: []*pb.WingFeatherRef{wingTestFeatherRef(bound, 1), wingTestFeatherRef(free, 2)}}
	}
	for _, tc := range []struct {
		name   string
		change func(*pb.WingQuoteRequest)
		code   uint32
	}{
		{"未允许绑定", func(q *pb.WingQuoteRequest) { q.AllowBound = false }, p.CodeWingInputs},
		{"重复原堆", func(q *pb.WingQuoteRequest) { q.Feathers[0] = wingTestFeatherRef(free, 1) }, p.CodeWingInputs},
		{"错误模板", func(q *pb.WingQuoteRequest) { q.Feathers[1] = wingTestFeatherRef(wrong, 2) }, p.CodeWingInputs},
		{"原堆版本已变", func(q *pb.WingQuoteRequest) { q.Feathers[1].Version++ }, p.CodeWingInputs},
		{"总量不足", func(q *pb.WingQuoteRequest) { q.Feathers[1].Quantity-- }, p.CodeWingInputs},
		{"总量超出", func(q *pb.WingQuoteRequest) { q.Feathers[1].Quantity++ }, p.CodeWingInputs},
		{"原翅膀已变", func(q *pb.WingQuoteRequest) { q.WingId = domain.Bytes(domain.ID()) }, p.CodeWingQuoteChanged},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := request()
			tc.change(q)
			if bad, code := buildWingGrowthPlan(role, &fixture, q); code != tc.code || len(bad.Feathers) != 0 {
				t.Fatal("错误原件应拒绝整个报价", code, bad)
			}
		})
	}
	plan, code := buildWingGrowthPlan(role, &fixture, request())
	if code != 0 || plan.TargetRank != 3 || plan.TargetAppearanceTier != 3 || len(plan.Feathers) != 2 {
		t.Fatal("第三阶未用真实绑定、非绑定原件解锁对应外观", code, plan)
	}
	out, progressed := applyWingGrowthPlan(role, plan, "wing_three")
	remaining := free
	remaining.Quantity, remaining.Version = 1, free.Version+1
	expected := wing
	expected.Rank, expected.AppearanceTier, expected.Version = 3, 3, wing.Version+1
	if progressed != expected || *out.Wing != expected || !reflect.DeepEqual(out.Items, []domain.Item{remaining, wrong}) ||
		!reflect.DeepEqual(role.Items, []domain.Item{bound, free, wrong}) || *role.Wing != wing {
		t.Fatal("逐阶共用原翅膀身份和幸运，切换外观不应打开隐藏状态", progressed, out.Items)
	}
	q := request()
	q.WingVersion = uint64(progressed.Version)
	q.Feathers = []*pb.WingFeatherRef{wingTestFeatherRef(remaining, 1)}
	if _, code = buildWingGrowthPlan(out, &fixture, q); code != p.CodeWingUnavailable {
		t.Fatal("尚未明确费用的阶段应保持关闭", code)
	}
	progressed.Rank, progressed.AppearanceTier = 10, 10
	out.Wing = &progressed
	if _, code = buildWingGrowthPlan(out, &fixture, q); code != p.CodeWingInputs {
		t.Fatal("达到最高阶后不能继续升阶", code)
	}
}
