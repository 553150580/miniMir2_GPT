// 本文件核验冻结降档方案的边界和普通装备处置回执。
package persistence

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// testOrdinaryEquipmentPlan 为真实模板和同一个原实例建立可重放的定值候选。
func testOrdinaryEquipmentPlan() (devconfig.Fixture, Settlement) {
	f := devconfig.Fixture{
		Items: []devconfig.Item{
			{ID: 101, IsEquipment: true, DropQuality: 3, EquipmentStage: 1},
			{ID: 102, IsEquipment: true, DropQuality: 2, EquipmentStage: 1},
			{ID: 103, IsEquipment: true, DropQuality: 1, EquipmentStage: 1},
			{ID: 104, IsEquipment: true, DropQuality: 1, EquipmentStage: 0},
		},
		Monsters: []devconfig.Monster{{ID: 5, DropPoolID: 7}},
		OrdinaryDropPools: []devconfig.OrdinaryDropPool{{ID: 7, Equipment: []devconfig.OrdinaryDropEntry{
			{ItemID: 101}, {ItemID: 102}, {ItemID: 103}, {ItemID: 104},
		}}},
		OrdinaryEquipmentQuotas: []devconfig.OrdinaryEquipmentQuota{
			{ID: 1, Stage: 1, Quality: 3, SourceKind: "ordinary_monster", ExhaustedDowngradeWeights: map[uint32]int64{2: 1}},
			{ID: 2, Stage: 1, Quality: 2, SourceKind: "ordinary_monster", ExhaustedDowngradeWeights: map[uint32]int64{1: 1}},
		},
	}
	primary := domain.Item{ID: "1234567890abcdef1234567890abcdef", ItemID: 101, Quantity: 1, Version: 1,
		Bound: true, SourceKind: "monster_drop", SourceID: "1", Quality: 3}
	rare, fine := primary, primary
	rare.ItemID, rare.Quality = 102, 2
	fine.ItemID, fine.Quality = 103, 1
	q := Settlement{SourceSeq: 1, Kills: 1, MonsterID: 5, MapID: 1, DropPoolID: 7, Bound: true,
		Item: &primary, EventAt: time.Date(2026, 11, 25, 13, 0, 0, 0, time.UTC),
		OrdinaryEquipment: &OrdinaryEquipmentPlan{Fallbacks: []domain.Item{rare, fine}}}
	return f, q
}

// TestOrdinaryEquipmentQuotaFreezesWholeChain 不接受跨阶段、不同绑定、无策略或非法最终档。
func TestOrdinaryEquipmentQuotaFreezesWholeChain(t *testing.T) {
	f, q := testOrdinaryEquipmentPlan()
	candidates, err := ordinaryEquipmentCandidates(&f, q)
	if err != nil || len(candidates) != 3 || candidates[0].quota != &f.OrdinaryEquipmentQuotas[0] ||
		candidates[1].quota != &f.OrdinaryEquipmentQuotas[1] || candidates[2].quota != nil || candidates[2].item != q.OrdinaryEquipment.Fallbacks[1] {
		t.Fatalf("未保留原同阶段降档和实例：%+v，%v", candidates, err)
	}
	for _, test := range []struct {
		name   string                                // 测试观察的方案边界。
		mutate func(*devconfig.Fixture, *Settlement) // 提交到额度前施加的一处变化。
	}{
		{"无原击杀时间", func(_ *devconfig.Fixture, q *Settlement) { q.EventAt = time.Time{} }},
		{"没有冻结计划", func(_ *devconfig.Fixture, q *Settlement) { q.OrdinaryEquipment = nil }},
		{"保底未归零", func(_ *devconfig.Fixture, q *Settlement) { q.NoEquipmentKills = 19 }},
		{"错怪物来源", func(_ *devconfig.Fixture, q *Settlement) { q.MonsterID++ }},
		{"篡改击杀绑定", func(_ *devconfig.Fixture, q *Settlement) { q.OrdinaryEquipment.Fallbacks[0].Bound = false }},
		{"篡改实例", func(_ *devconfig.Fixture, q *Settlement) { q.OrdinaryEquipment.Fallbacks[0].ID = domain.ID() }},
		{"跨阶段", func(_ *devconfig.Fixture, q *Settlement) { q.OrdinaryEquipment.Fallbacks[1].ItemID = 104 }},
		{"高品质无对应额度", func(f *devconfig.Fixture, _ *Settlement) { f.OrdinaryEquipmentQuotas = f.OrdinaryEquipmentQuotas[:1] }},
		{"最终降档仍是高品质", func(_ *devconfig.Fixture, q *Settlement) {
			q.OrdinaryEquipment.Fallbacks = q.OrdinaryEquipment.Fallbacks[:1]
		}},
		{"跳过未经配置的降档", func(_ *devconfig.Fixture, q *Settlement) {
			q.OrdinaryEquipment.Fallbacks = q.OrdinaryEquipment.Fallbacks[1:]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			f, q := testOrdinaryEquipmentPlan()
			test.mutate(&f, &q)
			if _, err := ordinaryEquipmentCandidates(&f, q); Code(err) != p.CodeInvalidArgument {
				t.Fatalf("非法方案未拒绝：%v", err)
			}
		})
	}
	delete(f.OrdinaryEquipmentQuotas[0].ExhaustedDowngradeWeights, 2)
	f.OrdinaryEquipmentQuotas[0].ExhaustedDowngradeWeights[1] = 1
	q.OrdinaryEquipment.Fallbacks = q.OrdinaryEquipment.Fallbacks[1:]
	if candidates, err = ordinaryEquipmentCandidates(&f, q); err != nil || len(candidates) != 2 || candidates[1].item.Quality != 1 {
		t.Fatalf("已配置的直接低档降级未保留：%v", err)
	}
}

// TestOrdinaryEquipmentQuotaLegacySettlementContract 表示只对实际高品质产物读取额度。
func TestOrdinaryEquipmentQuotaLegacySettlementContract(t *testing.T) {
	f, q := testOrdinaryEquipmentPlan()
	legacy := q.OrdinaryEquipment.Fallbacks[1]
	q.Item, q.OrdinaryEquipment = &legacy, nil
	candidates, err := ordinaryEquipmentCandidates(&f, q)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("普通品质绕行进入额度阶段：%v", err)
	}
	data, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, added := raw["ordinary_equipment"]; added {
		t.Fatal("空的新方案改变了旧请求序列化摘要")
	}
	// 高品质技能书仍属于独立的道具落点，不应被装备额度收缴或拒绝。
	f.Items = append(f.Items, devconfig.Item{ID: 111, DropQuality: 3})
	q.Item.ItemID, q.Item.Quality = 111, 3
	if candidates, err = ordinaryEquipmentCandidates(&f, q); err != nil || len(candidates) != 0 {
		t.Fatalf("普通高品质书籍走到了装备额度：%v", err)
	}
}

// TestOrdinaryEquipmentQuotaDiscardDoesNotConsume 只需要内存角色结果即可确认满包无奖不访问账本。
func TestOrdinaryEquipmentQuotaDiscardDoesNotConsume(t *testing.T) {
	_, q := testOrdinaryEquipmentPlan()
	choice := ordinaryEquipmentChoice{receipt: OrdinaryEquipmentReceipt{Item: *q.Item, CandidateItemID: q.Item.ItemID},
		window: ordinaryQuotaWindow{ID: "1:1:3:1732492800"}}
	store := &Store{} // 没有数据库客户端，访问配额会使此用例直接失败。
	details := SettlementReceipt{Recycling: EquipmentRecyclingReceipt{DiscardedCount: 1, DiscardReason: "bag_full"}}
	if err := store.confirmOrdinaryEquipment(context.Background(), &choice, &details); err != nil || details.OrdinaryEquipment == nil ||
		details.OrdinaryEquipment.QuotaConsumed || details.OrdinaryEquipment.Disposition != OrdinaryEquipmentDiscarded {
		t.Fatalf("满包未兑现仍扣额度，或缺少实际处置：%+v，%v", details.OrdinaryEquipment, err)
	}
}
