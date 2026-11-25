// 本文件验证普通掉落配置启用前的相容约束和新池门禁。
package devconfig

import (
	"math"
	"testing"
)

// TestOrdinaryDropPoolValidation 核对旧全局保留、新怪物引用、分类、数量及品质范围。
func TestOrdinaryDropPoolValidation(t *testing.T) {
	valid := func() Fixture {
		f := Fixture{DamageFormula: "development_source_combat",
			Monsters: []Monster{{ID: 1, DropPoolID: 11}},
			Items:    []Item{{ID: 35, Stackable: true, MaxStack: 10}, {ID: 193, IsEquipment: true, DropQuality: 1}},
			OrdinaryDropPools: []OrdinaryDropPool{{ID: 11,
				Items:     []OrdinaryDropEntry{{ItemID: 35, Weight: 1, QuantityMin: 1, QuantityMax: 10}},
				Equipment: []OrdinaryDropEntry{{ItemID: 193, Weight: 1, QuantityMin: 1, QuantityMax: 1}}}}}
		f.CombatTiming.PlayerAttackActionMS = 1
		return f
	}
	f := valid()
	if err := f.validateOrdinaryDropPools(); err != nil {
		t.Fatal(err)
	}
	f.Monsters[0].DropPoolID, f.OrdinaryDropPools = 0, nil
	if err := f.validateOrdinaryDropPools(); err != nil {
		t.Fatalf("历史全局池突然不再可用：%v", err)
	}
	f.RuleVersion = 30
	if err := f.validateOrdinaryDropPools(); err == nil {
		t.Fatal("规则30不能漏掉普通怪的掉落池")
	}
	for _, check := range []struct {
		name   string
		change func(*Fixture)
	}{
		{"怪物没有自己的池", func(f *Fixture) { f.Monsters[0].DropPoolID = 0 }},
		{"怪物引用不存在的池", func(f *Fixture) { f.Monsters[0].DropPoolID = 99 }},
		{"无池却有怪物引用", func(f *Fixture) { f.OrdinaryDropPools = nil }},
		{"装备不能在道具池", func(f *Fixture) { f.OrdinaryDropPools[0].Items[0].ItemID = 193 }},
		{"道具不能在装备池", func(f *Fixture) { f.OrdinaryDropPools[0].Equipment[0].ItemID = 35 }},
		{"数量超过堆叠上限", func(f *Fixture) { f.OrdinaryDropPools[0].Items[0].QuantityMax = 11 }},
		{"空装备池破坏保底", func(f *Fixture) { f.OrdinaryDropPools[0].Equipment = nil }},
		{"高品质需要额度", func(f *Fixture) { f.Items[1].DropQuality = 2 }},
		{"权重总和溢出", func(f *Fixture) {
			f.Items = append(f.Items, Item{ID: 36})
			pool := &f.OrdinaryDropPools[0]
			pool.Items = append(pool.Items, OrdinaryDropEntry{ItemID: 36, Weight: math.MaxInt64, QuantityMin: 1, QuantityMax: 1})
		}},
	} {
		t.Run(check.name, func(t *testing.T) {
			f := valid()
			check.change(&f)
			if err := f.validateOrdinaryDropPools(); err == nil {
				t.Fatal("异常配置没有被拒绝")
			}
		})
	}
}
