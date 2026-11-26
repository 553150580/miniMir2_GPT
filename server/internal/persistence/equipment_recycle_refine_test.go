// 本文件验证分解产石的原装备资格、抽样身份、绑定继承以及旧回收隔离。
package persistence

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// recycleRefineTestFixture 在现有油回归配置上加入精良原武器与独立洗炼石。
func recycleRefineTestFixture(t *testing.T) devconfig.Fixture {
	t.Helper()
	fixture := recycleOilTestFixture(t)
	var weapon devconfig.Item
	if err := json.Unmarshal([]byte(`{"id":220,"is_equipment":true,"required_level":5,"drop_quality":1,"recycle_gold":3000,"equipment_stage":0}`), &weapon); err != nil {
		t.Fatal(err)
	}
	const stoneID = 900051
	fixture.Items = append(fixture.Items, weapon, devconfig.Item{ID: stoneID, Stackable: true, MaxStack: 99, EquipmentRefineStone: true})
	fixture.EquipmentRefine = &devconfig.EquipmentRefineConfig{StoneItemID: stoneID, Recycle: devconfig.EquipmentRefineRecycleRule{
		ItemID: weapon.ID, SourceKind: ordinaryEquipmentItemSource, Quality: 1, Stage: 0, ProbabilityPPM: 50_000, Quantity: 1,
	}}
	return fixture
}

// TestRefineRecycleOnlyEligibleOriginals 确定性覆盖命中、失手、商店、破损、原绑定和同一事件重算。
func TestRefineRecycleOnlyEligibleOriginals(t *testing.T) {
	fixture := recycleRefineTestFixture(t)
	role := recycleRole()
	attempt := &equipmentRecycleAttempt{operationID: "asset:recycle:test", seed: "单位测试固定抽样状态"}
	rule := fixture.EquipmentRefine.Recycle
	// sample 选择具备指定结果的合法原件，随机边界仍沿用百万分比。
	sample := func(start int, bound, hit bool) domain.Item {
		t.Helper()
		item := recycleItem(rule.ItemID, bound)
		item.Quality, item.SourceID = rule.Quality, "9"
		item.DurabilityU, item.MaxDurabilityU, item.OriginalDurabilityU = 1, 4000, 4000
		for offset := range 1024 {
			item.ID = fmt.Sprintf("%032x", start+offset)
			draw, err := attempt.draw(equipmentRecycleRefineRandomDomain, item.ID, equipmentRecycleRefineDenominator)
			if err != nil {
				t.Fatal(err)
			}
			if (draw < rule.ProbabilityPPM) == hit {
				return item
			}
		}
		t.Fatal("未找到本次固定种子要求的判定")
		return domain.Item{}
	}
	bound, free, miss := sample(2000, true, true), sample(4000, false, true), sample(6000, false, false)
	shop, broken, legacyBroken, malformed, lucky := free, free, free, free, free
	shop.ID, shop.SourceKind, shop.SourceID = domain.ID(), "npc_shop", "武器店"
	broken.ID, broken.DurabilityU = domain.ID(), 0
	legacyBroken.ID, legacyBroken.DurabilityU = domain.ID(), -1
	malformed.ID, malformed.SourceID = domain.ID(), "09"
	lucky.ID, lucky.InitialLuck, lucky.CurrentLuck = domain.ID(), 2, 9
	kept := recycleItem(35, false)
	role.Items = []domain.Item{bound, free, miss, shop, broken, legacyBroken, malformed, lucky, kept}
	role.Capacity = len(role.Items)
	ids := []string{lucky.ID, miss.ID, shop.ID, free.ID, bound.ID, broken.ID, legacyBroken.ID, malformed.ID}
	updated, receipt, code, err := recycleManual(role, ids, fixture, attempt)
	if err != nil || code != 0 || receipt == nil || len(receipt.RefineRolls) != 3 || len(receipt.OilRolls) != 1 || len(updated.Items) < 3 {
		t.Fatal("没有只对各自合格原件判定独立副产物", receipt, code, err)
	}
	if !reflect.DeepEqual(updated.Items[0], kept) || receipt.Gold != 15_000 || receipt.BoundGold != 3_000 ||
		updated.Wallet.Gold != 15_000 || updated.Wallet.BoundGold != 3_000 {
		t.Fatal("破损发金币、正常分账或其他库存发生变化", receipt, updated)
	}
	for index, input := range []domain.Item{bound, free, miss} {
		fact := receipt.RefineRolls[index]
		if fact.EquipmentInstanceID != input.ID || fact.Numerator != 50_000 || fact.Denominator != 1_000_000 ||
			fact.Draw >= fact.Denominator || (fact.Draw < fact.Numerator) != (fact.Stone != nil) {
			t.Fatal("原件或概率回执不符", input, fact)
		}
		if fact.Stone != nil && (fact.Stone.Bound != input.Bound || fact.Stone.SourceID != input.ID || fact.Stone.ID == input.ID ||
			fact.Stone.SourceKind != equipmentRecycleRefineSource || !updated.HasItemInstance(fact.Stone.ID)) {
			t.Fatal("分解衍生物没有保持原绑定和来源", input, fact)
		}
	}
	view := receipt.refineProto()
	if len(view.GetRolls()) != 3 || view.Rolls[0].GetStone() == nil || view.Rolls[2].GetStone() != nil ||
		domain.Hex(view.Rolls[0].EquipmentInstanceId) != bound.ID {
		t.Fatal("只读协议未保留命中和失手的原事实", view)
	}
	// 同一事务重算使用同一私有种子。客户端选中发送顺序不会改变库存顺序或结果。
	ids[0], ids[1] = ids[1], ids[0]
	replayed, original, code, err := recycleManual(role, ids, fixture, attempt)
	if err != nil || code != 0 || !reflect.DeepEqual(replayed, updated) || !reflect.DeepEqual(original, receipt) {
		t.Fatal("同一分解尝试的重算没有完全重放原事实", code, err)
	}
	// 玩家本就启用自动回收时，仍沿用旧金币策略，不发隔离分解石。
	role.AutoRecycle.QualityIDs = []uint32{1}
	automatic := new(EquipmentRecyclingReceipt)
	if _, err = recycleExisting(role, fixture, automatic); err != nil || len(automatic.Recycled) != 8 || automatic.refineProto() != nil {
		t.Fatal("自动策略误进入隔离产石范围", automatic, err)
	}
	defaultFixture := fixture
	defaultFixture.EquipmentRefine = nil
	_, defaultReceipt, code, err := recycleManual(role, []string{bound.ID}, defaultFixture, attempt)
	if err != nil || code != 0 || defaultReceipt.BoundGold != 3000 || defaultReceipt.refineProto() != nil {
		t.Fatal("默认规则的原资产行为变化", defaultReceipt, code, err)
	}
}

// TestRefineRecycleProtectsOccupiedAndFlowIdentity 验证待确定原件不能分解，石头流通不能绕过绑定或来源。
func TestRefineRecycleProtectsOccupiedAndFlowIdentity(t *testing.T) {
	fixture := recycleRefineTestFixture(t)
	role := recycleRole()
	input := recycleItem(220, false)
	input.Quality, input.SourceID, input.PendingResultID = 1, "1", domain.ID()
	input.DurabilityU, input.MaxDurabilityU, input.OriginalDurabilityU = 100, 100, 100
	role.Items = []domain.Item{input}
	attempt := equipmentRecycleAttempt{operationID: "asset:protected:source"}
	next, receipt, code, err := recycleManual(role, []string{input.ID}, fixture, &attempt)
	if err != nil || code != p.CodeInvalidArgument || receipt != nil || attempt.seed != "" || !reflect.DeepEqual(next, role) {
		t.Fatal("待确定洗炼词条的原件没有保护", next, receipt, code, err)
	}
	definition, _ := fixture.Item(fixture.EquipmentRefine.StoneItemID)
	stone := domain.Item{ID: domain.ID(), ItemID: definition.ID, Quantity: 1, Version: 1, SourceKind: equipmentRecycleRefineSource, SourceID: input.ID}
	bound := stone
	bound.Bound = true
	if !CanTrade(stone, definition) || !warehouseItemValid(stone, fixture) || CanTrade(bound, definition) || !warehouseItemValid(bound, fixture) {
		t.Fatal("绑定石只能个人存储，非绑定原石应可流通")
	}
	for name, change := range map[string]func(*domain.Item){
		"伪造原件": func(item *domain.Item) { item.SourceID = item.ID },
		"假石定义": func(item *domain.Item) { item.ItemID = fixture.LuckOil.ItemID },
		"虚增数量": func(item *domain.Item) { item.Quantity++ },
		"携带油幸": func(item *domain.Item) { item.CurrentLuck = 9 },
	} {
		t.Run(name, func(t *testing.T) {
			item := stone
			change(&item)
			actual, _ := fixture.Item(item.ItemID)
			if CanTrade(item, actual) || warehouseItemValid(item, fixture) {
				t.Fatal("不符合原石来源的道具可流通", item)
			}
		})
	}
}
