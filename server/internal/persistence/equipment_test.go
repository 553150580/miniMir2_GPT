// 验证装备属性、实例版本、替换及卸装时的库存和穿戴一致性。

package persistence

import (
	"encoding/json"
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

// equipmentFixture 处理装备开发测试配置。
// t 提供断言失败报告、子用例和清理注册。
func equipmentFixture(t *testing.T) (domain.Role, devconfig.Fixture) {
	t.Helper()
	// items 记录道具集合，使用道具列表表示。
	var items []devconfig.Item
	// 数值是隔离管线资格，不向主目录添加装备或回收价格。
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := json.Unmarshal([]byte(`[{"id":218,"is_equipment":true,"equipment_slot":0,"required_level":1,"equipment_stage":0,"magic_min_bonus":0,"magic_max_bonus":1,"tao_min_bonus":0,"tao_max_bonus":2,"recycle_gold":50,"drop_quality":0,"allowed_jobs":[1]},{"id":193,"is_equipment":true,"equipment_slot":1,"required_level":1,"equipment_stage":0,"magic_min_bonus":0,"magic_max_bonus":0,"tao_min_bonus":0,"tao_max_bonus":0,"recycle_gold":400,"drop_quality":0,"allowed_jobs":[1]}]`), &items); err != nil {
		t.Fatal(err)
	}
	// f 组装开发测试配置所需的数据。
	f := devconfig.Fixture{RuleVersion: 22, Hash: "equipment-test", Items: items, Players: []devconfig.Fighter{{JobID: 1, HP: 100, MP: 100, MagicMin: 2, MagicMax: 2, TaoMin: 3, TaoMax: 3}}}
	// r 组装角色所需的数据。
	r := domain.Role{ID: domain.ID(), Job: 1, Level: 7, Capacity: 2, Items: []domain.Item{{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 1, Bound: true, SourceKind: "monster_drop", SourceID: "original-source", Locked: true}, {ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 4, SourceKind: "npc_shop", SourceID: "original-shop"}}}
	return r, f
}

// TestEquipmentExplicitWearSwapUnequipPreserveOriginalItems 核验穿入应保留原锁/来源/版本或污染原切片，以及满包卸下应原子拒绝。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentExplicitWearSwapUnequipPreserveOriginalItems(t *testing.T) {
	// r 接收装备开发测试配置供后续步骤使用，f 接收装备开发测试配置供后续步骤使用。
	r, f := equipmentFixture(t)
	// original 保留加入新条目后的“original”。
	original := append([]domain.Item(nil), r.Items...)
	// q 组装资产请求所需的数据。
	q := AssetRequest{ItemID: r.Items[0].ID, ItemVersion: 1, Slot: 0}
	// worn 保存由穿戴装备装备取得的“worn”，receipt 保存由穿戴装备装备取得的回执，code 保存由穿戴装备装备取得的结果码。
	worn, receipt, code := equipEquipment(r, q, f)
	if code != 0 || len(worn.Items) != 1 || len(worn.Equipped) != 1 || worn.Equipped[0].Item.Version != 2 || !reflect.DeepEqual(r.Items, original) || !worn.Equipped[0].Item.Locked || len(receipt.Moves) != 1 {
		t.Fatalf("穿入没有保留原锁/来源/版本或污染原切片: %d %+v", code, worn)
	}
	worn.Capacity = 1 // 已满背包仍可做同槽等量替换。
	q.ItemID, q.ItemVersion = worn.Items[0].ID, 4
	// swapped 保存由穿戴装备装备取得的“swapped”。
	swapped, receipt, code := equipEquipment(worn, q, f)
	if code != 0 || len(swapped.Items) != 1 || swapped.Items[0].ID != original[0].ID || swapped.Items[0].Version != 3 || swapped.Equipped[0].Item.Version != 5 || len(receipt.Moves) != 2 {
		t.Fatalf("满包换件未等量归还双方原件: %d %+v", code, swapped)
	}
	if swapped.Items[0].SourceID != original[0].SourceID || swapped.Items[0].Bound != original[0].Bound || !swapped.Items[0].Locked {
		t.Fatal("换装改写绑定/来源/锁")
	}
	q.ItemID, q.ItemVersion, q.Unequip = swapped.Equipped[0].Item.ID, 5, true
	// out 保存由穿戴装备装备取得的输出，op 保存由穿戴装备装备取得的操作，code 保存由穿戴装备装备取得的结果码。
	if out, op, code := equipEquipment(swapped, q, f); code != p.CodeEquipmentBagFull || op != nil || !reflect.DeepEqual(out, swapped) {
		t.Fatal("满包卸下没有原子拒绝")
	}
	swapped.Capacity = 2
	// bag 保存由穿戴装备装备取得的背包。
	bag, receipt, code := equipEquipment(swapped, q, f)
	if code != 0 || len(bag.Equipped) != 0 || len(bag.Items) != 2 || bag.Items[1].Version != 6 || !receipt.Unequip || receipt.Moves[0].Before.SourceID != original[1].SourceID {
		t.Fatal("真正卸下未保留原物品")
	}
	// copyReceipt 保存由副本装备回执取得的“copy”回执。
	copyReceipt := cloneEquipmentReceipt(receipt)
	receipt.Moves[0].After.SourceID = "later-mutation"
	if copyReceipt.Moves[0].After.SourceID != original[1].SourceID {
		t.Fatal("回执快照别名被后续修改")
	}
}

// TestEquipmentRejectsChangedBlockedAndAmbiguousOwnership 核对「拒绝发生部分移物或错误码不符」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentRejectsChangedBlockedAndAmbiguousOwnership(t *testing.T) {
	// tests 初始化用作测试集合的临时结构列表。
	tests := []struct {
		// name 记录名称，使用文本表示。
		name string
		// mutate 记录变更，使用供当前流程调用的函数表示。
		mutate func(*domain.Role, *AssetRequest, *devconfig.Fixture)
		// code 记录结果码，使用数值表示。
		code uint32
	}{
		// q 记录资产请求。
		{"零版本", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemVersion = 0 }, p.CodeEquipmentChanged},
		// q 记录资产请求。
		{"旧版本", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemVersion = 2 }, p.CodeEquipmentChanged},
		// r 记录角色，q 记录资产请求。
		{"溢出版本", func(r *domain.Role, q *AssetRequest, _ *devconfig.Fixture) {
			r.Items[0].Version = math.MaxInt64
			q.ItemVersion = math.MaxInt64
		}, p.CodeEquipmentChanged},
		// r 记录角色。
		{"非单件", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].Quantity = 2 }, p.CodeEquipmentChanged},
		// r 记录角色。
		{"未确认结果", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].PendingResultID = "preview" }, p.CodeEquipmentBlocked},
		// r 记录角色。
		{"占用", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].ReservationID = "reserved" }, p.CodeEquipmentBlocked},
		// q 记录资产请求。
		{"错误槽", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.Slot = 1 }, p.CodeEquipmentChanged},
		// q 记录资产请求。
		{"护符槽", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.Slot = 100 }, p.CodeEquipmentChanged},
		// r 记录角色。
		{"职业", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Job = 0 }, p.CodeEquipmentChanged},
		// r 记录角色，f 记录开发测试配置。
		{"等级", func(r *domain.Role, _ *AssetRequest, f *devconfig.Fixture) {
			r.Level = 1
			f.Items[0].RequiredLevel = new(uint32(7))
		}, p.CodeEquipmentChanged},
		// r 记录角色。
		{"伪品质", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].Quality = 4 }, p.CodeEquipmentChanged},
		// r 记录角色。
		{"重复背包ID", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[1].ID = r.Items[0].ID }, p.CodeEquipmentChanged},
		// r 记录角色。
		{"背包穿戴重复", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) {
			r.Equipped = []domain.Equipped{{Slot: 0, Item: r.Items[0]}}
		}, p.CodeEquipmentChanged},
		// r 记录角色。
		{"未知定义", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].ItemID = 999 }, p.CodeEquipmentChanged},
	}
	// test 接收遍历测试集合时当前的元素。
	for _, test := range tests {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(test.name, func(t *testing.T) {
			// r 接收装备开发测试配置供后续步骤使用，f 接收装备开发测试配置供后续步骤使用。
			r, f := equipmentFixture(t)
			// q 组装资产请求所需的数据。
			q := AssetRequest{ItemID: r.Items[0].ID, ItemVersion: 1, Slot: 0}
			test.mutate(&r, &q, &f)
			// raw 保存由序列化取得的原始值。
			raw, _ := json.Marshal(r)
			// out 保存由穿戴装备装备取得的输出，op 保存由穿戴装备装备取得的操作，code 保存由穿戴装备装备取得的结果码。
			out, op, code := equipEquipment(r, q, f)
			// after 保存由序列化取得的操作后的状态。
			after, _ := json.Marshal(r)
			if code != test.code || op != nil || !reflect.DeepEqual(out, r) || string(raw) != string(after) {
				t.Fatalf("拒绝发生部分移物或错误码不符: %d", code)
			}
		})
	}
}

// TestEquipmentOutgoingProtectionAndLegacyIntent 核对「被替换原件的保护未在移物前核验」与「旧规则普通穿入合同改变」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentOutgoingProtectionAndLegacyIntent(t *testing.T) {
	// r 接收装备开发测试配置供后续步骤使用，f 接收装备开发测试配置供后续步骤使用。
	r, f := equipmentFixture(t)
	// worn 保存由穿戴装备装备取得的“worn”，code 保存由穿戴装备装备取得的结果码。
	worn, _, code := equipEquipment(r, AssetRequest{ItemID: r.Items[0].ID, ItemVersion: 1, Slot: 0}, f)
	if code != 0 {
		t.Fatal(code)
	}
	// block 接收遍历文本列表时当前的元素。
	for _, block := range []string{"pending", "reserved", "overflow", "duplicate-slot"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(block, func(t *testing.T) {
			// copyRole 保存当前处理所用的“copy”角色。
			copyRole := worn
			copyRole.Equipped = append([]domain.Equipped(nil), worn.Equipped...)
			// want 保存结果码装备阻挡，用于当前步骤中的期望值。
			want := p.CodeEquipmentBlocked
			switch block {
			case "pending":
				copyRole.Equipped[0].Item.PendingResultID = "p"
			case "reserved":
				copyRole.Equipped[0].Item.ReservationID = "r"
			case "overflow":
				copyRole.Equipped[0].Item.Version = math.MaxInt64
				want = p.CodeEquipmentChanged
			case "duplicate-slot":
				// another 保存从已穿戴装备取出的“another”。
				another := copyRole.Equipped[0]
				another.Item.ID = domain.ID()
				copyRole.Equipped = append(copyRole.Equipped, another)
				want = p.CodeEquipmentChanged
			}
			// q 组装资产请求所需的数据。
			q := AssetRequest{ItemID: copyRole.Items[0].ID, ItemVersion: 4, Slot: 0}
			// out 保存由穿戴装备装备取得的输出，op 保存由穿戴装备装备取得的操作，code 保存由穿戴装备装备取得的结果码。
			if out, op, code := equipEquipment(copyRole, q, f); code != want || op != nil || !reflect.DeepEqual(out, copyRole) {
				t.Fatal("被替换原件的保护未在移物前核验")
			}
		})
	}
	f.RuleVersion = 21
	// receipt 保存由穿戴装备装备取得的回执，code 保存由穿戴装备装备取得的结果码。
	if _, receipt, code := equipEquipment(r, AssetRequest{ItemID: r.Items[0].ID, Slot: 0}, f); code != 0 || receipt != nil {
		t.Fatal("旧规则普通穿入合同改变")
	}
	// code 保存由穿戴装备装备取得的结果码。
	if _, _, code := equipEquipment(worn, AssetRequest{ItemID: worn.Equipped[0].Item.ID, Unequip: true, Slot: 0}, f); code != p.CodeInvalidArgument {
		t.Fatal("旧规则接受新卸装意图")
	}
}

// TestEquipmentLegacyWearCanBeRemovedAndReplaced 核验旧原件应完整返包，用诊断「旧等级/职业/合法品质变化把原穿戴物困住」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentLegacyWearCanBeRemovedAndReplaced(t *testing.T) {
	// replace 接收遍历布尔状态列表时当前的元素。
	for _, replace := range []bool{false, true} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[bool]string{false: "卸下", true: "替换"}[replace], func(t *testing.T) {
			// r 接收装备开发测试配置供后续步骤使用，f 接收装备开发测试配置供后续步骤使用。
			r, f := equipmentFixture(t)
			// old 保存从道具集合取出的原状态。
			old := r.Items[0]
			old.Version = 3
			old.Quality = 4
			r.Equipped = []domain.Equipped{{Slot: 0, Item: old}}
			r.Items = r.Items[1:]
			// 明示旧存档资格：原穿戴木剑的新定义已提高等级/改变职业；合法旧品质也不等于当前普通掉落品质。
			f.Items[0].RequiredLevel = new(uint32(19))
			f.Items[0].AllowedJobs = []uint32{0}
			// q 组装资产请求所需的数据。
			q := AssetRequest{Slot: 0, ItemID: old.ID, ItemVersion: 3, Unequip: true}
			if replace {
				// 只把已有布衣定义放到同测试槽，提供一个当前合格替换件；不新增装备或主目录数值。
				f.Items[1].EquipmentSlot = 0
				r.Items[0].ItemID = 193
				q.ItemID, q.ItemVersion, q.Unequip = r.Items[0].ID, uint64(r.Items[0].Version), false
			}
			// out 保存由穿戴装备装备取得的输出，receipt 保存由穿戴装备装备取得的回执，code 保存由穿戴装备装备取得的结果码。
			out, receipt, code := equipEquipment(r, q, f)
			if code != 0 || receipt == nil {
				t.Fatalf("旧等级/职业/合法品质变化把原穿戴物困住: code=%d", code)
			}
			// returned 记录已返回值，使用道具表示。
			var returned domain.Item
			// item 接收遍历道具集合时当前的元素。
			for _, item := range out.Items {
				if item.ID == old.ID {
					returned = item
				}
			}
			if returned.ID != old.ID || returned.Version != 4 || returned.Quality != 4 || returned.Bound != old.Bound || returned.SourceID != old.SourceID || !returned.Locked {
				t.Fatal("旧原件没有完整返包")
			}
			// before 保存当前处理所用的操作前的状态。
			before := out
			// rejected 保存由穿戴装备装备取得的拒绝，op 保存由穿戴装备装备取得的操作，code 保存由穿戴装备装备取得的结果码。
			if rejected, op, code := equipEquipment(out, AssetRequest{Slot: 0, ItemID: returned.ID, ItemVersion: 4}, f); code != p.CodeEquipmentChanged || op != nil || !reflect.DeepEqual(rejected, before) {
				t.Fatal("解除旧穿戴被误当成新穿资格")
			}
		})
	}
}

// TestEquipmentOutgoingInvalidQualityIsNotGuessed 核对「越界旧品质被默认为当前品质或被部分移动」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestEquipmentOutgoingInvalidQualityIsNotGuessed(t *testing.T) {
	// r 接收装备开发测试配置供后续步骤使用，f 接收装备开发测试配置供后续步骤使用。
	r, f := equipmentFixture(t)
	// old 保存从道具集合取出的原状态。
	old := r.Items[0]
	old.Quality = 5
	r.Items = r.Items[1:]
	r.Equipped = []domain.Equipped{{Slot: 0, Item: old}}
	// unequip 接收遍历布尔状态列表时当前的元素。
	for _, unequip := range []bool{false, true} {
		// q 组装资产请求所需的数据。
		q := AssetRequest{Slot: 0, ItemID: old.ID, ItemVersion: 1, Unequip: true}
		if !unequip {
			q.ItemID, q.ItemVersion, q.Unequip = r.Items[0].ID, 4, false
		}
		// out 保存由穿戴装备装备取得的输出，receipt 保存由穿戴装备装备取得的回执，code 保存由穿戴装备装备取得的结果码。
		if out, receipt, code := equipEquipment(r, q, f); code != p.CodeEquipmentChanged || receipt != nil || !reflect.DeepEqual(out, r) {
			t.Fatal("越界旧品质被默认为当前品质或被部分移动")
		}
	}
}
