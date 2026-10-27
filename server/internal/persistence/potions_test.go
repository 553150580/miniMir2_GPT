package persistence

import (
	"math"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"reflect"
	"testing"
)

func potionAssetFixture() (domain.Role, AssetRequest, devconfig.Fixture) {
	id := domain.ID()
	r := domain.Role{Items: []domain.Item{{ID: id, ItemID: 91, Quantity: 2, Bound: true, Version: 4}, {ID: domain.ID(), ItemID: 92, Quantity: 1, Version: 1}}, Wallet: domain.Wallet{BoundGold: 10}}
	q := AssetRequest{ItemID: id, ItemDefinitionID: 91, SceneEpoch: 20}
	f := devconfig.Fixture{Items: []devconfig.Item{{ID: 91}, {ID: 92}}, Potions: devconfig.PotionRules{Effects: []devconfig.PotionEffect{{ItemID: 91, HP: 7}}}}
	return r, q, f
}

func TestPotionConsumeCopiesInventoryAndRetainsOwnershipAndBinding(t *testing.T) {
	r, q, f := potionAssetFixture()
	before := append([]domain.Item(nil), r.Items...)
	out, effect, code := consumePotion(r, q, f)
	if code != 0 || effect.HP != 7 || out.Items[0].Quantity != 1 || out.Items[0].Version != 5 || !out.Items[0].Bound || out.Items[0].ID != q.ItemID || out.Wallet != r.Wallet {
		t.Fatal("扣药未准确减一、保留身份/绑定或只使用配置效果")
	}
	if !reflect.DeepEqual(r.Items, before) {
		t.Fatal("药品修改污染原角色共享切片")
	}
	last, _, code := consumePotion(out, q, f)
	if code != 0 || len(last.Items) != 1 || last.Items[0].ItemID != 92 || out.Items[0].Quantity != 1 {
		t.Fatal("最后一个药品未移除或修改了其它物品/原库存")
	}
}

func TestPotionConsumeFailuresDoNotMutateInventory(t *testing.T) {
	tests := []struct {
		name   string
		change func(*domain.Role, *AssetRequest, *devconfig.Fixture)
		code   uint32
	}{
		{"不存在的实例", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemID = domain.ID() }, p.CodeInvalidArgument},
		{"伪造物品类型", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.ItemDefinitionID = 92 }, p.CodeInvalidArgument},
		{"库存类型与效果不符", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].ItemID = 92 }, p.CodeInvalidArgument},
		{"数量不足", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].Quantity = 0 }, p.CodeInvalidArgument},
		{"版本溢出", func(r *domain.Role, _ *AssetRequest, _ *devconfig.Fixture) { r.Items[0].Version = math.MaxInt64 }, p.CodeInvalidArgument},
		{"无场景", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.SceneEpoch = 0 }, p.CodeInvalidArgument},
		{"装备不能作药品", func(_ *domain.Role, _ *AssetRequest, f *devconfig.Fixture) { f.Items[0].IsEquipment = true }, p.CodeInvalidArgument},
		{"死亡", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.PotionGateCode = p.CodeItemUseBlocked }, p.CodeItemUseBlocked},
		{"共享节流", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.PotionGateCode = p.CodeItemUseCooldown }, p.CodeItemUseCooldown},
		{"旧场景新请求", func(_ *domain.Role, q *AssetRequest, _ *devconfig.Fixture) { q.PotionGateCode = p.CodeStaleSession }, p.CodeStaleSession},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r, q, f := potionAssetFixture()
			test.change(&r, &q, &f)
			before := append([]domain.Item(nil), r.Items...)
			out, _, code := consumePotion(r, q, f)
			if code != test.code || !reflect.DeepEqual(before, r.Items) || !reflect.DeepEqual(before, out.Items) {
				t.Fatalf("拒绝码=%d，预期%d，或拒绝修改了库存", code, test.code)
			}
		})
	}
}
