// 本文件核对一瓶用油的逐段回退、保底边界、实际绑料和武器实例迁移。
package persistence

import (
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// testWeaponOilFixture 只构造用油与装备认定所需的假库存规则。
func testWeaponOilFixture() devconfig.Fixture {
	return devconfig.Fixture{
		Items: []devconfig.Item{{ID: 20, IsEquipment: true, EquipmentSlot: 0}, {ID: 21, IsEquipment: true, EquipmentSlot: 3},
			{ID: 22, Stackable: true, MaxStack: 9999}},
		LuckOil: &devconfig.LuckOilConfig{ItemID: 22, Steps: []devconfig.LuckOilStep{
			{SuccessPPM: 1_000_000}, {SuccessPPM: 800_000, FailureLuck: 1}, {SuccessPPM: 600_000, FailureLuck: 2},
			{SuccessPPM: 400_000, FailureLuck: 3, PityAfter: 100}, {SuccessPPM: 300_000, FailureLuck: 3, PityAfter: 100},
			{SuccessPPM: 200_000, FailureLuck: 3, PityAfter: 100}, {SuccessPPM: 100_000, FailureLuck: 6, PityAfter: 3000},
			{SuccessPPM: 70_000, FailureLuck: 6, PityAfter: 3000}, {SuccessPPM: 50_000, FailureLuck: 6, PityAfter: 3000},
		}},
	}
}

// weaponOilQuoteRequest 从角色当前选择构造原参考号，不跳过版本或位置核对。
func weaponOilQuoteRequest(item domain.Item, equipped, allowBound bool) *pb.WeaponOilQuoteRequest {
	return &pb.WeaponOilQuoteRequest{SceneEpoch: 100, UiSessionId: domain.Bytes(domain.ID()), AllowBound: allowBound,
		Weapon: &pb.WeaponOilItemRef{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version), Equipped: equipped}}
}

// TestWeaponOilThresholdKeepsTwoIndependentStages 核对阈值当瓶正常抽样、下瓶起逐级必成。
func TestWeaponOilThresholdKeepsTwoIndependentStages(t *testing.T) {
	fixture := testWeaponOilFixture()
	weapon := domain.Item{ID: domain.ID(), ItemID: 20, Quantity: 1, Version: 4, InitialLuck: 3, CurrentLuck: 5,
		LuckOilToSix: 99, SourceKind: "monster_drop", SourceID: "1", Quality: 2,
		DurabilityU: 7, MaxDurabilityU: 11, OriginalDurabilityU: 12}
	unbound := domain.Item{ID: domain.ID(), ItemID: 22, Quantity: 2, Version: 1, SourceKind: "task_reward", SourceID: "task1"}
	bound := domain.Item{ID: domain.ID(), ItemID: 22, Quantity: 10, Version: 3, Bound: true, SourceKind: "npc_shop", SourceID: "oil"}
	role := domain.Role{Items: []domain.Item{unbound, bound}, Equipped: []domain.Equipped{{Slot: 0, Item: weapon}},
		Wallet: domain.Wallet{Gold: 17, BoundYuanbao: 9}}
	apply := func(failLuck, wantLuck, beforeUsed, afterUsed, limit, rate uint32, guaranteed, success bool) {
		t.Helper()
		before := role.Equipped[0].Item
		plan, code := buildWeaponOilPlan(role, &fixture, weaponOilQuoteRequest(before, true, true))
		if code != 0 || plan.FailureLuck != failLuck || plan.PityUsed != beforeUsed || plan.PityAfter != limit ||
			plan.SuccessPPM != rate || plan.Guaranteed != guaranteed || !plan.Oil.Bound || !plan.OutputBound {
			t.Fatal("用油同段报价与原输入不符", code, plan)
		}
		next, result := applyWeaponOilPlan(role, weaponOilStoredQuote{Plan: plan, Roll: weaponOilPPM - 1})
		after := next.Equipped[0].Item
		if result.Weapon != after || !after.Bound || after.CurrentLuck != wantLuck || after.InitialLuck != weapon.InitialLuck || after.Version != before.Version+1 ||
			result.Success != success || result.PityUsed != afterUsed || result.OilConsumed != 1 || next.Wallet != role.Wallet ||
			next.Items[0] != unbound || next.Items[1].Quantity != role.Items[1].Quantity-1 || next.Items[1].Version != role.Items[1].Version+1 {
			t.Fatal("本瓶回退、实际绑定、扣瓶或确认后的保底不符", result, after)
		}
		if after.ID != weapon.ID || after.ItemID != weapon.ItemID || after.DurabilityU != weapon.DurabilityU ||
			after.MaxDurabilityU != weapon.MaxDurabilityU || after.OriginalDurabilityU != weapon.OriginalDurabilityU ||
			after.SourceKind != weapon.SourceKind || after.SourceID != weapon.SourceID || after.Quality != weapon.Quality {
			t.Fatal("用油更换原武器、来源、品质或耐久", after)
		}
		role = next
	}
	apply(3, 3, 99, 100, 100, 200_000, false, false)
	apply(3, 4, 100, 101, 100, weaponOilPPM, true, true)
	apply(3, 5, 101, 102, 100, weaponOilPPM, true, true)
	apply(3, 6, 102, 103, 100, weaponOilPPM, true, true)
	if role.Equipped[0].Item.LuckOilToSix != 103 || role.Equipped[0].Item.LuckOilToNine != 0 {
		t.Fatal("进入下一阶段时复用或重置了上一段计数", role.Equipped)
	}
	role.Equipped[0].Item.CurrentLuck, role.Equipped[0].Item.LuckOilToNine = 8, 2999
	apply(6, 6, 2999, 3000, 3000, 50_000, false, false)
	apply(6, 7, 3000, 3001, 3000, weaponOilPPM, true, true)
	apply(6, 8, 3001, 3002, 3000, weaponOilPPM, true, true)
	apply(6, 9, 3002, 3003, 3000, weaponOilPPM, true, true)
	final := role.Equipped[0].Item
	_, code := buildWeaponOilPlan(role, &fixture, weaponOilQuoteRequest(final, true, true))
	if final.LuckOilToSix != 103 || final.LuckOilToNine != 3003 || code != p.CodeWeaponOilInputs {
		t.Fatal("满档仍可消耗原油，或两段计数不同于实际耗用", final)
	}
}

// TestWeaponOilUsesOriginalBagInstances 核对关闭绑定、绑武器不解绑、旧幸运及合法流转。
func TestWeaponOilUsesOriginalBagInstances(t *testing.T) {
	fixture := testWeaponOilFixture()
	weapon := domain.Item{ID: domain.ID(), ItemID: 20, Quantity: 1, Version: 7, InitialLuck: 2, Bound: true}
	oil := domain.Item{ID: domain.ID(), ItemID: 22, Quantity: 1, Version: 2}
	bound := domain.Item{ID: domain.ID(), ItemID: 22, Quantity: 5, Version: 3, Bound: true}
	role := domain.Role{Items: []domain.Item{bound, oil, weapon}}
	request := weaponOilQuoteRequest(weapon, false, false)
	plan, code := buildWeaponOilPlan(role, &fixture, request)
	if code != 0 || plan.Oil != oil || plan.BeforeLuck != 2 || !plan.OutputBound || plan.PityUsed != 0 || plan.PityAfter != 0 {
		t.Fatal("关闭时误用绑定油或旧初始幸运未进入报价", code, plan)
	}
	result, receipt := applyWeaponOilPlan(role, weaponOilStoredQuote{Plan: plan, Roll: 600_000})
	if len(result.Items) != 2 || result.Items[0] != bound || result.Items[1] != receipt.Weapon || receipt.Success ||
		receipt.Weapon.CurrentLuck != 2 || receipt.Weapon.InitialLuck != 2 || !receipt.Weapon.Bound || receipt.OilRemainder != nil ||
		role.Items[1] != oil || role.Items[2] != weapon || receipt.Proto().Weapon.CurrentLuck != 2 {
		t.Fatal("早段失败清除了旧资产、错误扣了绑定油或改变原对象", result.Items, receipt)
	}
	check := weaponOilQuoteRequest(receipt.Weapon, false, false)
	if _, code = buildWeaponOilPlan(result, &fixture, check); code != p.CodeWeaponOilInputs {
		t.Fatal("没有非绑定油时可以隐式切换绑定投入", code)
	}
	check = weaponOilQuoteRequest(weapon, true, true)
	if _, code = buildWeaponOilPlan(role, &fixture, check); code != p.CodeWeaponOilInputs {
		t.Fatal("背包装备可以冒充已穿戴武器", code)
	}
	// 非绑定培养武器的合法全量流转保留初始幸运、培养值及两个保底计数。
	moving := weapon
	moving.Bound, moving.CurrentLuck, moving.LuckOilToSix, moving.LuckOilToNine = false, 7, 103, 10
	transferred, remainder, ok := warehouseSplit(moving, 1, "")
	moving.Version++
	if !ok || remainder != nil || transferred != moving {
		t.Fatal("移动合法非绑定武器时改变了原实例和幸运", transferred, remainder)
	}
}
