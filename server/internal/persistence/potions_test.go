// 验证手动药水的库存扣除和效果回执，核对角色状态与来源物品的约束。

package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

// potionAssetFixture 处理药水资产开发测试配置。
func potionAssetFixture() (domain.Role, AssetRequest, devconfig.Fixture) {
	// id 区分当前步骤指向的目标对象。
	id := domain.ID()
	// r 组装角色所需的数据。
	r := domain.Role{Items: []domain.Item{{ID: id, ItemID: 91, Quantity: 2, Bound: true, Version: 4}, {ID: domain.ID(), ItemID: 92, Quantity: 1, Version: 1}}, Wallet: domain.Wallet{BoundGold: 10}}
	// q 组装资产请求所需的数据。
	q := AssetRequest{ItemID: id, ItemDefinitionID: 91, SceneEpoch: 20}
	// f 组装开发测试配置所需的数据。
	f := devconfig.Fixture{Items: []devconfig.Item{{ID: 91}, {ID: 92}}, Potions: devconfig.PotionRules{Effects: []devconfig.PotionEffect{{ItemID: 91, HP: 7}}}}
	return r, q, f
}

// TestPotionConsumeCopiesInventoryAndRetainsOwnershipAndBinding 核对「扣药未准确减一、保留身份/绑定或只使用配置效果」与「最后一个药品未移除或修改了其它物品/原库存」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPotionConsumeCopiesInventoryAndRetainsOwnershipAndBinding(t *testing.T) {
	// r 接收药水资产开发测试配置供后续步骤使用，q 接收药水资产开发测试配置供后续步骤使用，f 接收药水资产开发测试配置供后续步骤使用。
	r, q, f := potionAssetFixture()
	// before 保留加入新条目后的操作前的状态。
	before := append([]domain.Item(nil), r.Items...)
	// out 保存由消费药水取得的输出，effect 保存由消费药水取得的效果，code 保存由消费药水取得的结果码。
	out, effect, code := consumePotion(r, q, f)
	if code != 0 || effect.HP != 7 || out.Items[0].Quantity != 1 || out.Items[0].Version != 5 || !out.Items[0].Bound || out.Items[0].ID != q.ItemID || out.Wallet != r.Wallet {
		t.Fatal("扣药未准确减一、保留身份/绑定或只使用配置效果")
	}
	if !reflect.DeepEqual(r.Items, before) {
		t.Fatal("药品修改污染原角色共享切片")
	}
	// last 保存由消费药水取得的上一次状态。
	last, _, code := consumePotion(out, q, f)
	if code != 0 || len(last.Items) != 1 || last.Items[0].ItemID != 92 || out.Items[0].Quantity != 1 {
		t.Fatal("最后一个药品未移除或修改了其它物品/原库存")
	}
}

// TestPotionConsumeFailuresDoNotMutateInventory 核对「拒绝码=，预期，或拒绝修改了库存」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPotionConsumeFailuresDoNotMutateInventory(t *testing.T) {
	// tests 初始化用作测试集合的临时结构列表。
	tests := []struct {
		// name 记录名称，使用文本表示。
		name string
		// change 记录变更，使用供当前流程调用的函数表示。
		change func(*domain.Role, *AssetRequest, *devconfig.Fixture)
		// code 记录结果码，使用数值表示。
		code uint32
	}{
		// q 记录资产请求。
		{"不存在的实例", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemID = domain.ID() }, p.CodeInvalidArgument},
		// q 记录资产请求。
		{"伪造物品类型", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemDefinitionID = 92 }, p.CodeInvalidArgument},
		// r 记录角色。
		{"库存类型与效果不符", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].ItemID = 92 }, p.CodeInvalidArgument},
		// r 记录角色。
		{"数量不足", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].Quantity = 0 }, p.CodeInvalidArgument},
		// r 记录角色。
		{"版本溢出", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].Version = math.MaxInt64 }, p.CodeInvalidArgument},
		// q 记录资产请求。
		{"无场景", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.SceneEpoch = 0 }, p.CodeInvalidArgument},
		// f 记录开发测试配置。
		{"装备不能作药品", func(_ *domain.Role, _ *AssetRequest, f *devconfig.Fixture) { f.Items[0].IsEquipment = true }, p.CodeInvalidArgument},
		// q 记录资产请求。
		{"死亡", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.PotionGateCode = p.CodeItemUseBlocked }, p.CodeItemUseBlocked},
		// q 记录资产请求。
		{"共享节流", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.PotionGateCode = p.CodeItemUseCooldown }, p.CodeItemUseCooldown},
		// q 记录资产请求。
		{"旧场景新请求", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.PotionGateCode = p.CodeStaleSession }, p.CodeStaleSession},
	}
	// test 接收遍历测试集合时当前的元素。
	for _, test := range tests {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(test.name, func(t *testing.T) {
			// r 接收药水资产开发测试配置供后续步骤使用，q 接收药水资产开发测试配置供后续步骤使用，f 接收药水资产开发测试配置供后续步骤使用。
			r, q, f := potionAssetFixture()
			test.change(&r, &q, &f)
			// before 保留加入新条目后的操作前的状态。
			before := append([]domain.Item(nil), r.Items...)
			// out 保存由消费药水取得的输出，code 保存由消费药水取得的结果码。
			out, _, code := consumePotion(r, q, f)
			if code != test.code || !reflect.DeepEqual(before, r.Items) || !reflect.DeepEqual(before, out.Items) {
				t.Fatalf("拒绝码=%d，预期%d，或拒绝修改了库存", code, test.code)
			}
		})
	}
}
