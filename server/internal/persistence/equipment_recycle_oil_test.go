// 本文件核对手动分解用当前幸运、每件独立来源以及旧金-only 自动回收边界。
package persistence

import (
	"fmt"
	"reflect"
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// recycleOilTestFixture 补上手动分解所需的唯一油定义，不修改已有回收金币。
func recycleOilTestFixture(t *testing.T) devconfig.Fixture {
	t.Helper()
	f := recycleFixture(t)
	const oilID = 900021
	f.LuckOil = &devconfig.LuckOilConfig{ItemID: oilID}
	f.EquipmentRecycleOil = &devconfig.EquipmentRecycleOilConfig{Denominator: 30}
	f.Items = append(f.Items, devconfig.Item{ID: oilID, Stackable: true, MaxStack: 99, EquipmentRecycleMaterial: true})
	return f
}

// TestManualRecycleOilFrozenSource 核对当前幸运、命中和未中、旧装备及同事务重算的原结果。
func TestManualRecycleOilFrozenSource(t *testing.T) {
	f := recycleOilTestFixture(t)
	role := recycleRole()
	attempt := equipmentRecycleAttempt{operationID: "asset:" + role.ID + ":" + domain.ID(), seed: "unit-test-original-seed"}
	sample := func(bound, hit bool) domain.Item {
		item := recycleItem(218, bound)
		item.InitialLuck, item.CurrentLuck, item.DurabilityU, item.MaxDurabilityU, item.OriginalDurabilityU = 2, 8, 100, 200, 300
		for index := range 1024 {
			item.ID = fmt.Sprintf("%032x", index+1)
			draw, err := attempt.draw(equipmentRecycleOilRandomDomain, item.ID, 30)
			if err != nil {
				t.Fatal(err)
			}
			if (draw < item.CurrentLuck) == hit {
				return item
			}
		}
		t.Fatal("没有合乎预期判定的固定测试样本")
		return domain.Item{}
	}
	looted := sample(true, true)
	miss := sample(false, false)
	broken := recycleItem(218, false)
	broken.InitialLuck, broken.MaxDurabilityU, broken.OriginalDurabilityU = 3, 100, 100
	withoutLuck := recycleItem(218, false)
	withoutLuck.DurabilityU, withoutLuck.MaxDurabilityU, withoutLuck.OriginalDurabilityU = 1, 100, 100
	role.Items = []domain.Item{looted, miss, broken, withoutLuck}
	ids := []string{looted.ID, miss.ID, broken.ID, withoutLuck.ID}
	got, receipt, code, err := recycleManual(role, ids, f, &attempt)
	if err != nil || code != 0 || len(got.Items) != 1 || receipt == nil || len(receipt.Recycled) != 4 || len(receipt.OilRolls) != 2 {
		t.Fatal("满包销毁、命中以及两种不具资格的原装备处理不正确", got.Items, receipt, code, err)
	}
	first, second := receipt.OilRolls[0], receipt.OilRolls[1]
	if first.EquipmentInstanceID != looted.ID || first.Luck != 8 || first.Denominator != 30 || first.Draw >= first.Luck || first.Oil == nil ||
		*first.Oil != got.Items[0] || first.Oil.SourceID != looted.ID || first.Oil.ID == looted.ID || !first.Oil.Bound ||
		second.EquipmentInstanceID != miss.ID || second.Luck != 8 || second.Denominator != 30 || second.Draw < second.Luck || second.Oil != nil {
		t.Fatal("原来源、当前幸运判定或逐件绑定没有保留", first, second)
	}
	if got.Wallet.Gold != 100 || got.Wallet.BoundGold != 50 || receipt.Gold != 100 || receipt.BoundGold != 50 ||
		got.HasItemInstance(looted.ID) || !role.HasItemInstance(looted.ID) {
		t.Fatal("破损装备分解发金、原回收金币改变或原角色被污染", got.Wallet, receipt)
	}
	view := receipt.oilProto()
	if view == nil || len(view.Rolls) != 2 || view.Rolls[0].Oil == nil || view.Rolls[1].Oil != nil || view.Rolls[0].Luck != 8 {
		t.Fatal("没有对原命中或未命中生成一致回执", view)
	}
	// 驱动重试重新应用同一原事务。抽样、原油实例与批处理顺序无关。
	dup, persisted, code, err := recycleManual(role, []string{withoutLuck.ID, broken.ID, miss.ID, looted.ID}, f, &attempt)
	if err != nil || code != 0 || !reflect.DeepEqual(dup, got) || !reflect.DeepEqual(persisted, receipt) {
		t.Fatal("同一事务的重算改变了抽样或生成产物", dup, persisted, code, err)
	}
}

// TestRecycleOilScopeAndFlowProtection 核对无效请求、自动回收、历史默认与合法流通保护。
func TestRecycleOilScopeAndFlowProtection(t *testing.T) {
	f := recycleOilTestFixture(t)
	role := recycleRole()
	item := recycleItem(218, false)
	item.InitialLuck, item.DurabilityU, item.MaxDurabilityU, item.OriginalDurabilityU = 3, 1, 100, 100
	broken := item
	broken.ID, broken.DurabilityU = domain.ID(), 0
	other := recycleItem(35, false)
	role.Items = []domain.Item{item, broken, other}
	attempt := equipmentRecycleAttempt{operationID: "asset:" + role.ID + ":" + domain.ID()}
	for _, ids := range [][]string{{item.ID, other.ID}, {item.ID, domain.ID()}, {item.ID, item.ID}} {
		next, receipt, code, err := recycleManual(role, ids, f, &attempt)
		if err != nil || code != p.CodeInvalidArgument || receipt != nil || attempt.seed != "" || !reflect.DeepEqual(next, role) {
			t.Fatal("非法同批对象消耗选择物或提前抽样", ids, next, receipt, code, err)
		}
	}
	for _, protect := range []func(*domain.Role){
		func(r *domain.Role) { r.Items[0].Locked = true },
		func(r *domain.Role) { r.Items[0].PendingResultID = "pending" },
		func(r *domain.Role) { r.Items[0].ReservationID = "reserved" },
		func(r *domain.Role) { r.Equipped = []domain.Equipped{{Item: item}} },
	} {
		withProtection := role
		withProtection.Items = append([]domain.Item(nil), role.Items...)
		protect(&withProtection)
		next, receipt, code, err := recycleManual(withProtection, []string{item.ID}, f, &attempt)
		if err != nil || code != p.CodeInvalidArgument || receipt != nil || attempt.seed != "" || !reflect.DeepEqual(next, withProtection) {
			t.Fatal("手动候选绕过锁定、占用或穿戴保护", next, receipt, code, err)
		}
	}
	automatic := &EquipmentRecyclingReceipt{}
	after, err := recycleExisting(role, f, automatic)
	if err != nil || len(after.Items) != 1 || after.Items[0] != other || automatic.Gold != 100 || len(automatic.OilRolls) != 0 {
		t.Fatal("自动回收未经授权出油或修改历史金币价", after, automatic, err)
	}
	defaultFixture := f
	defaultFixture.EquipmentRecycleOil = nil
	_, plain, code, err := recycleManual(role, []string{broken.ID}, defaultFixture, nil)
	if err != nil || code != 0 || plain.Gold != 50 || plain.oilProto() != nil {
		t.Fatal("候选关闭时改变了原手动分解金币或产物", plain, code, err)
	}
	legacy := role
	legacy.Items = []domain.Item{broken}
	legacy.Items[0].DurabilityU = -1
	legacyAfter, legacyReceipt, code, err := recycleManual(legacy, []string{broken.ID}, f, &attempt)
	if err != nil || code != 0 || legacyReceipt.Gold != 0 || legacyReceipt.BoundGold != 0 ||
		legacyReceipt.oilProto() != nil || len(legacyAfter.Items) != 0 || legacyAfter.Wallet != legacy.Wallet {
		t.Fatal("历史负耐久原装备不应发放金币或油", legacyAfter, legacyReceipt, code, err)
	}
	definition, _ := f.Item(f.LuckOil.ItemID)
	valid := domain.Item{ID: domain.ID(), ItemID: definition.ID, Quantity: 1, Version: 1, SourceKind: equipmentRecycleOilSource, SourceID: item.ID}
	if !CanTrade(valid, definition) || !warehouseItemValid(valid, f) {
		t.Fatal("合法非绑定原瓶不能整件流通", valid)
	}
	bound := valid
	bound.Bound = true
	if CanTrade(bound, definition) || !warehouseItemValid(bound, f) {
		t.Fatal("绑定瓶必须禁止市场但保留私人存取资格", bound)
	}
	for _, alter := range []func(*domain.Item){
		func(i *domain.Item) { i.SourceID = "invalid" },
		func(i *domain.Item) { i.SourceID = i.ID },
		func(i *domain.Item) { i.ItemID = 35 },
		func(i *domain.Item) { i.OrePurity = 80 },
		func(i *domain.Item) { i.CurrentLuck = 1 },
		func(i *domain.Item) { i.Quantity = 2 },
	} {
		malformed := valid
		alter(&malformed)
		if CanTrade(malformed, definition) || warehouseItemValid(malformed, f) {
			t.Fatal("市场或仓储放行无原来源的分解材料", malformed)
		}
	}
}

// TestRecycleOilUsesLuckAllowedByEquipmentSlot 确认幸运异常不会生成客户端无法接收的原已确认回执。
func TestRecycleOilUsesLuckAllowedByEquipmentSlot(t *testing.T) {
	f := recycleOilTestFixture(t)
	for index, def := range f.Items {
		if def.ID == 193 {
			f.Items[index].EquipmentSlot = 3
		}
	}
	tests := []struct {
		item    uint32 // 当前原装备定义。
		initial uint32 // 原始掉落或打造的幸运。
		current uint32 // 历史记录中的培养幸运。
		want    uint32 // 分解实际采用的合法幸运，零表示不抽样。
	}{
		{218, 2, 0, 2}, {218, 2, 9, 9}, {218, 3, 1, 0}, {218, 4, 7, 0}, {218, 2, 10, 0},
		{193, 2, 99, 2}, {193, 3, 0, 3}, {193, 0, 8, 0}, {193, 4, 4, 0}, {35, 3, 3, 0},
	}
	for _, test := range tests {
		item := domain.Item{ItemID: test.item, InitialLuck: test.initial, CurrentLuck: test.current}
		if luck := equipmentRecycleLuck(item, &f); luck != test.want {
			t.Fatal("分解未按原部位选择合法幸运", test, luck)
		}
	}
	role := recycleRole()
	invalidWeapon := recycleItem(218, false)
	invalidWeapon.InitialLuck, invalidWeapon.CurrentLuck = 2, 10
	invalidWeapon.DurabilityU, invalidWeapon.MaxDurabilityU, invalidWeapon.OriginalDurabilityU = 10, 20, 20
	role.Items = []domain.Item{invalidWeapon}
	attempt := equipmentRecycleAttempt{operationID: "valid-scope-only"}
	after, receipt, code, err := recycleManual(role, []string{invalidWeapon.ID}, f, &attempt)
	if err != nil || code != 0 || receipt == nil || receipt.Gold != 50 || after.Wallet.Gold != role.Wallet.Gold+50 ||
		len(after.Items) != 0 || receipt.oilProto() != nil || attempt.seed != "" {
		t.Fatal("异常培养幸运不应抽油或改变原有回收金币", after, receipt, code, err)
	}
}
