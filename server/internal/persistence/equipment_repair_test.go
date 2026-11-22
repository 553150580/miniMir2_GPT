package persistence

import (
	"math"
	"reflect"
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func repairFixture() *devconfig.Fixture {
	return &devconfig.Fixture{
		EquipmentDurability: &devconfig.EquipmentDurabilityRules{
			WeaponLossU: 1, ArmorHitLossU: 1, DeathLossU: 1000, RepairRangeCells: 1,
			Ordinary: devconfig.RepairRule{Kind: devconfig.RepairOrdinary, UpperLossDivisor: 30, MinMaxDurabilityU: 1},
			Special:  &devconfig.RepairRule{Kind: devconfig.RepairSpecial, FullRepairCostU: 20},
		},
		Items: []devconfig.Item{{ID: 218, IsEquipment: true, OriginalDurabilityU: 4000, RepairBasePrice: 5}},
	}
}

func TestEquipmentRepairUsesExactDeficitAndPreservesBinding(t *testing.T) {
	f := repairFixture()
	item := domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 3, Bound: true, Locked: true, SourceKind: "npc_shop", SourceID: "source", OriginalDurabilityU: 4000, MaxDurabilityU: 3900, DurabilityU: 1800}
	role := domain.Role{Equipped: []domain.Equipped{{Slot: 0, Item: item}}, Wallet: domain.Wallet{BoundGold: 2, Gold: 10, BoundYuanbao: 3, Yuanbao: 10}}
	ordinary, code := BuildRepairPlan(role, f, f.EquipmentDurability.Ordinary, item.ID, 3)
	if code != 0 || !ordinary.Equipped || ordinary.Slot != 0 || ordinary.After.DurabilityU != 3830 || ordinary.After.MaxDurabilityU != 3830 ||
		ordinary.After.OriginalDurabilityU != 4000 || ordinary.Spend != (domain.Wallet{BoundGold: 2, Gold: 1}) {
		t.Fatalf("普通修理报价、修后上限或穿戴槽错误: %+v，状态%d", ordinary, code)
	}
	after, code := ApplyRepairPlan(role, ordinary)
	expected := item
	expected.DurabilityU, expected.MaxDurabilityU, expected.Version = 3830, 3830, 4
	if code != 0 || after.Equipped[0].Item != expected || role.Equipped[0].Item != item || after.Wallet != (domain.Wallet{Gold: 9, BoundYuanbao: 3, Yuanbao: 10}) {
		t.Fatal("修理未只改原实例的上限、耐久、版本及原报价钱包")
	}
	if again, code := ApplyRepairPlan(after, ordinary); code != p.CodeEquipmentChanged || !reflect.DeepEqual(after, again) {
		t.Fatal("旧计划重复扣款或覆盖了新耐久")
	}
	role.Equipped, role.Items = nil, []domain.Item{item}
	special, code := BuildRepairPlan(role, f, *f.EquipmentDurability.Special, item.ID, 3)
	if code != 0 || special.Equipped || special.After.DurabilityU != 3900 || special.After.MaxDurabilityU != 3900 ||
		special.Spend != (domain.Wallet{BoundYuanbao: 3, Yuanbao: 8}) {
		t.Fatalf("特殊修理报价、背包身份或上限错误: %+v，状态%d", special, code)
	}
	role.Wallet.Yuanbao = 0
	if denied, code := ApplyRepairPlan(role, special); code != p.CodeEquipmentBlocked || !reflect.DeepEqual(denied, role) {
		t.Fatal("余额不足仍然扣款或修改了原实例")
	}
	if fee := repairCost(math.MaxInt64-1, math.MaxInt64, math.MaxInt64); fee != math.MaxInt64-1 {
		t.Fatalf("大整数比例费用溢出: %d", fee)
	}
}

func TestDurabilityHydrationDoesNotRepairBrokenEquipment(t *testing.T) {
	f := repairFixture()
	old := domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1}
	broken := InitializeItemDurability(old, f)
	broken.ID, broken.DurabilityU, broken.MaxDurabilityU = domain.ID(), 0, 3970
	r := domain.Role{Items: []domain.Item{old}, Equipped: []domain.Equipped{{Slot: 0, Item: broken}}}
	loaded := hydrateRoleDurability(r, f)
	if loaded.Items[0].DurabilityU != 4000 || loaded.Items[0].OriginalDurabilityU != 4000 || loaded.Items[0].Version != old.Version ||
		loaded.Equipped[0].Item != broken || r.Items[0] != old || loaded.Inventory().Equipped[0].Item.OriginalDurabilityU != 4000 || loaded.Inventory().Equipped[0].Item.DurabilityU != 0 {
		t.Fatal("旧无耐久实例初始化、已损坏0值保留或背包/穿戴投影错误")
	}
}

func TestEquipmentWearReceiptPinsExactInstanceAndDeath(t *testing.T) {
	f := repairFixture()
	item := InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, Bound: true}, f)
	item.DurabilityU = 3
	role := domain.Role{Equipped: []domain.Equipped{{Slot: 0, Item: item}}}
	death := domain.ID()
	costs := []EquipmentWear{{Slot: 0, InstanceID: item.ID, Version: 1, AttackU: 1, DeathU: 2}}
	after, proof, err := applyEquipmentWear(role, costs, death, f)
	if err != nil || len(proof) != 1 || proof[0].Before != item || proof[0].After.ID != item.ID || !proof[0].After.Bound || after.Equipped[0].Item.DurabilityU != 0 || after.Equipped[0].Item.Version != 2 || role.Equipped[0].Item != item {
		t.Fatal("合批未精确应用，或损坏时销毁/修改了原实例身份", proof, err)
	}
	for _, invalid := range []struct {
		role  domain.Role
		death string
	}{{after, death}, {role, ""}, {role, "错误"}} {
		got, proof, err := applyEquipmentWear(invalid.role, costs, invalid.death, f)
		if err == nil || len(proof) != 0 || !reflect.DeepEqual(got, invalid.role) {
			t.Fatal("版本变化或死亡身份不匹配仍应用了旧成本")
		}
	}
}
