// 本文件核对战斗和挑战取得同一套部位强化结果，以及空槽和损坏装备的边界。
package game

import (
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// TestEnhanceEquipmentSlotCombatAppliesOnlyValidBase 验证每槽保留小数，汇总才进入当前战斗和新挑战快照。
func TestEnhanceEquipmentSlotCombatAppliesOnlyValidBase(t *testing.T) {
	w, r, _ := equipmentWorker(t, 0)
	fixture := &w.e.Store.Cfg.Fixture
	fixture.EquipmentEnhancement = &devconfig.EquipmentEnhancementConfig{Levels: make([]devconfig.EquipmentEnhancementLevel, 10)}
	for index := range fixture.EquipmentEnhancement.Levels {
		fixture.EquipmentEnhancement.Levels[index] = devconfig.EquipmentEnhancementLevel{CurrentLevel: uint32(index), BonusBP: 10}
	}
	if fixture.EquipmentDurability == nil {
		fixture.EquipmentDurability = &devconfig.EquipmentDurabilityRules{}
	}
	// 隔离属性用于跨过整数边界。正式木剑每槽的原始数值不够大，仍保留真实的系数。
	for index := range fixture.Items {
		item := &fixture.Items[index]
		if item.ID == 218 || item.ID == 193 {
			item.AttackBonus, item.AttackMinBonus, item.AttackMaxBonus = 0, 50, 50
		}
	}
	base := w.basePlayerFighter(r)
	r.Role.EquipmentEnhancement = &domain.EquipmentEnhancement{Levels: [domain.EquipmentEnhancementSlots]uint32{10, 10}}
	if got := w.basePlayerFighter(r); got != base {
		t.Fatal("空槽强化产生了战力", got)
	}
	r.Role.Equipped = []domain.Equipped{
		{Slot: 0, Item: domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, DurabilityU: 100, OriginalDurabilityU: 100}},
		{Slot: 1, Item: domain.Item{ID: domain.ID(), ItemID: 193, Quantity: 1, Version: 1, DurabilityU: 100, OriginalDurabilityU: 100}},
		{Slot: devconfig.CastingSupplySlot, Item: domain.Item{ID: domain.ID(), ItemID: 218}},
	}
	combined := w.basePlayerFighter(r)
	if combined.AttackMin != base.AttackMin+101 || combined.AttackMax != base.AttackMax+101 {
		t.Fatal("两个 0.5 点没有在同一次定点汇总进位", combined.AttackMin, combined.AttackMax, base)
	}
	challenger := w.freezeBossCombatSnapshot(r, &fixture.Map)
	if challenger.Fighter.AttackMin != combined.AttackMin || challenger.Fighter.AttackMax != combined.AttackMax {
		t.Fatal("挑战未采用同一权威强化结果", challenger.Fighter)
	}
	r.Role.Equipped[0].Item.DurabilityU = 0
	broken := w.basePlayerFighter(r)
	if broken.AttackMin != base.AttackMin+50 || broken.AttackMax != base.AttackMax+50 || r.Role.EnhancementLevel(0) != 10 ||
		challenger.Fighter.AttackMax != combined.AttackMax {
		t.Fatal("损坏装备仍产生基础或强化加成，或修改了原部位／原挑战快照", broken)
	}
	r.Role.Equipped[0].Item.ID = domain.ID()
	r.Role.Equipped[0].Item.DurabilityU = 100
	if got := w.basePlayerFighter(r); got != combined {
		t.Fatal("换同模板装备没有重新应用原部位的强化", got)
	}
}
