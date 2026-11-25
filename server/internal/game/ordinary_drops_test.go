// 本文件验证普通怪的来源掉落池、堆叠数量和跨地图的角色保底。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestOrdinaryMonsterDropPoolsUseOriginalKillAndSharedPity 验证两地图原怪掉落和同一角色第20杀保底。
func TestOrdinaryMonsterDropPoolsUseOriginalKillAndSharedPity(t *testing.T) {
	w, r, _ := stageAWorker(t, 2)
	f := &w.e.Store.Cfg.Fixture
	f.OrdinaryDropPools = []devconfig.OrdinaryDropPool{
		{ID: 11, Equipment: []devconfig.OrdinaryDropEntry{{ItemID: 218, Weight: 1, QuantityMin: 1, QuantityMax: 1}},
			Items: []devconfig.OrdinaryDropEntry{{ItemID: 35, Weight: 3, QuantityMin: 1, QuantityMax: 1},
				{ItemID: 181, Weight: 2, QuantityMin: 3, QuantityMax: 5}}},
		{ID: 22, Equipment: []devconfig.OrdinaryDropEntry{{ItemID: 193, Weight: 1, QuantityMin: 1, QuantityMax: 1}},
			Items: []devconfig.OrdinaryDropEntry{{ItemID: 187, Weight: 1, QuantityMin: 2, QuantityMax: 2}}},
	}
	// 选定道具区、道具权重区间末端和数量上界，避免依赖真实随机。
	w.roll = func(limit int64) int64 {
		if limit == 1_000_000 {
			return 850_000
		}
		return limit - 1
	}
	now := time.Unix(1800000000, 0)
	firstMonster := r.Monsters[0]
	firstMonster.Monster.DropPoolID = 11
	r.Role.NoEquipmentKills = 18
	w.enqueueKillFact(r, firstMonster, now, &r.Player, "", now.Add(time.Second))
	first := w.killSettlement(r, r.KillFacts[0])
	if first.DropPoolID != 11 || first.MonsterID != firstMonster.Monster.ID || first.NoEquipmentKills != 19 ||
		first.Item == nil || first.Item.ItemID != 181 || first.Item.Quantity != 5 || first.Item.Bound || first.Gold != 0 {
		t.Fatalf("第一张图的道具、数量、绑定或保底不符：%+v", first)
	}

	// 原候选不被之后的随机流、出生配置或角色特权重新计算。
	firstMonster.Monster.DropPoolID = 22
	w.roll = func(int64) int64 { return 0 }
	again := w.killSettlement(r, r.KillFacts[0])
	if again.DropPoolID != first.DropPoolID || again.NoEquipmentKills != first.NoEquipmentKills || again.Item == nil || *again.Item != *first.Item {
		t.Fatal("重查原击杀时重抽了掉落或改变了归属")
	}

	// 跨地图后仍使用同一角色的19杀水位。原随机是空，新的怪物池须替换为唯一装备结果。
	r.Role.NoEquipmentKills = first.NoEquipmentKills
	r.Role.SourceSeq = first.SourceSeq
	secondWorld := &worldState{MapID: 92, PlayerPosition: devconfig.Point{X: 8, Y: 8}}
	w.enqueueKillFactInWorld(r, secondWorld, firstMonster, now.Add(time.Second), &r.Player, "hosting", now.Add(time.Second))
	second := w.killSettlement(r, r.KillFacts[1])
	if second.SourceSeq != first.SourceSeq+1 || second.MapID != secondWorld.MapID || second.DropPoolID != 22 ||
		second.NoEquipmentKills != 0 || second.HostingSession != "hosting" || second.Item == nil ||
		second.Item.ItemID != 193 || second.Item.Quantity != 1 || !second.Item.Bound || second.Gold != 0 {
		t.Fatalf("第二张图的保底装备或绑定不符：%+v", second)
	}
}

func TestOrdinaryEquipmentDropFreezesPityAndUsesConfirmedLoot(t *testing.T) {
	w, r, _ := stageAWorker(t, 0)
	f := &w.e.Store.Cfg.Fixture
	pool := devconfig.OrdinaryDropPool{ID: 11, Items: []devconfig.OrdinaryDropEntry{{ItemID: 35, Weight: 1, QuantityMin: 1, QuantityMax: 1}}}
	base, _ := f.Item(218)
	for quality := uint32(0); quality <= 3; quality++ {
		item := base
		item.ID, item.DropQuality = 61001+quality, quality
		f.Items = append(f.Items, item)
		pool.Equipment = append(pool.Equipment, devconfig.OrdinaryDropEntry{ItemID: item.ID, Weight: 1, QuantityMin: 1, QuantityMax: 1})
	}
	f.OrdinaryDropPools = []devconfig.OrdinaryDropPool{pool}
	f.OrdinaryEquipmentQuotas = []devconfig.OrdinaryEquipmentQuota{
		{Stage: base.EquipmentStage, Quality: 3, SourceKind: devconfig.OrdinaryMonsterDropSource,
			ExhaustedDowngradeWeights: map[uint32]int64{0: 5, 2: 7}},
		{Stage: base.EquipmentStage, Quality: 2, SourceKind: devconfig.OrdinaryMonsterDropSource,
			ExhaustedDowngradeWeights: map[uint32]int64{0: 8, 1: 1}},
	}
	w.roll = func(n int64) int64 { return n - 1 }
	fact := killFact{Monster: devconfig.Monster{DropPoolID: pool.ID}, InstanceID: domain.ID(), Bound: true}
	if !w.captureOrdinaryDrop(&fact) {
		t.Fatal("同池同阶段高品质链未成功冻结")
	}
	// 空白掷点在连续19杀后才改为装备，已冻结的候选从史诗逐级降为稀有、精良。
	if early := w.killSettlement(r, fact); early.Item != nil || early.OrdinaryEquipment != nil {
		t.Fatalf("未达保底的空白结果产生装备：%+v", early)
	}
	r.Role.NoEquipmentKills = f.Drops.EquipmentGuarantee - 1
	q := w.killSettlement(r, fact)
	if q.OrdinaryEquipment == nil || len(q.OrdinaryEquipment.Fallbacks) != 2 || q.NoEquipmentKills != 0 ||
		q.Item.ItemID != 61004 || q.OrdinaryEquipment.Fallbacks[0].ItemID != 61003 || q.OrdinaryEquipment.Fallbacks[1].ItemID != 61002 {
		t.Fatalf("第二十杀的原装备或冻结降档不符：%+v", q)
	}
	actual := q.OrdinaryEquipment.Fallbacks[1]
	details := &persistence.SettlementReceipt{GrantedItem: true, OrdinaryEquipment: &persistence.OrdinaryEquipmentReceipt{Item: actual}}
	op := persistence.Operation{Kind: "settlement", SourceSeq: q.SourceSeq, Settlement: details}
	if loot, ok := confirmedSettlementLoot(q, op); !ok || loot == nil || *loot != actual {
		t.Fatalf("掉落展示未采用已确认的精良装备：%+v", loot)
	}
	details.GrantedItem = false
	if loot, ok := confirmedSettlementLoot(q, op); !ok || loot != nil {
		t.Fatal("未入包的装备不能作为已确认战利品展示")
	}
	details.OrdinaryEquipment.Item.ItemID = 62222
	if _, ok := confirmedSettlementLoot(q, op); ok {
		t.Fatal("不接受与冻结候选无关的回执")
	}
}
