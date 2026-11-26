// 本文件验证只有已采纳词条进入普通战斗和挑战快照，并与强化同类取整。
package game

import (
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
)

// TestRefineEquipmentUsesOnlyAdoptedStats 校验原半点强化与半点词条合计一次，待选和损坏不生效。
func TestRefineEquipmentUsesOnlyAdoptedStats(t *testing.T) {
	w, r, _ := equipmentWorker(t, 0)
	fixture := &w.e.Store.Cfg.Fixture
	fixture.EquipmentRefine = &devconfig.EquipmentRefineConfig{}
	fixture.EquipmentEnhancement = &devconfig.EquipmentEnhancementConfig{Levels: []devconfig.EquipmentEnhancementLevel{{BonusBP: 100}}}
	if fixture.EquipmentDurability == nil {
		fixture.EquipmentDurability = &devconfig.EquipmentDurabilityRules{}
	}
	for at := range fixture.Items {
		definition := &fixture.Items[at]
		if definition.ID == 218 {
			definition.AttackBonus, definition.AttackMinBonus, definition.AttackMaxBonus = 0, 50, 50
		}
	}
	base := w.basePlayerFighter(r)
	r.Role.EquipmentEnhancement = &domain.EquipmentEnhancement{Levels: [domain.EquipmentEnhancementSlots]uint32{1}}
	item := domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, DurabilityU: 100, OriginalDurabilityU: 100,
		PendingResultID: domain.ID(), Refine: domain.EquipmentRefineState{Candidate: domain.EquipmentAffixes{
			{AttributeID: devconfig.RefineAttributeAttack, Tier: 1, ValueUnits: 5},
			{AttributeID: devconfig.RefineAttributeMaxHP, Tier: 1, ValueUnits: 17},
		}}}
	r.Role.Equipped = []domain.Equipped{{Slot: 0, Item: item}}
	pending := w.basePlayerFighter(r)
	if pending.AttackMin != base.AttackMin+50 || pending.AttackMax != base.AttackMax+50 || pending.HP != base.HP {
		t.Fatal("待选词条提前进入战斗或强化提前按整点进位", pending)
	}
	item.Refine = domain.EquipmentRefineState{Current: item.Refine.Candidate}
	item.PendingResultID = ""
	r.Role.Equipped[0].Item = item
	adopted := w.basePlayerFighter(r)
	if adopted.AttackMin != base.AttackMin+51 || adopted.AttackMax != base.AttackMax+51 || adopted.HP != base.HP+1 {
		t.Fatal("两份半点或已采纳生命没有从统一取整点生效", adopted)
	}
	frozen := w.freezeBossCombatSnapshot(r, &fixture.Map)
	if frozen.Fighter.AttackMax != adopted.AttackMax || frozen.Fighter.HP != adopted.HP {
		t.Fatal("挑战快照没有使用已采纳属性")
	}
	r.Role.Equipped[0].Item.DurabilityU = 0
	if broken := w.basePlayerFighter(r); broken != base || frozen.Fighter.AttackMax != adopted.AttackMax {
		t.Fatal("破损装备仍生效或改写了既有挑战快照", broken)
	}
}
