// 本文件使用原实例校验图纸打造的报价、绑定继承、取整、失败损失和幸运。
package persistence

import (
	"math"
	"reflect"
	"slices"
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// forgeQuoteRefs 表达背包里实际被选中的原件，测试中不会传入预算替代原件。
func forgeQuoteRefs(scene uint64, blueprint domain.Item, ores []domain.Item, bound bool) *pb.ForgeQuoteRequest {
	ref := func(item domain.Item) *pb.ForgeItemRef {
		return &pb.ForgeItemRef{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version)}
	}
	q := &pb.ForgeQuoteRequest{SceneEpoch: scene, Blueprint: ref(blueprint), AllowBound: bound}
	for _, ore := range ores {
		q.Ores = append(q.Ores, ref(ore))
	}
	return q
}

// forgePlanCase 建立只涉及打造规则的五个不同原件，以及待保留的同类材料。
func forgePlanCase() (domain.Role, devconfig.Fixture) {
	level := uint32(20)
	f := devconfig.Fixture{
		EquipmentDurability: &devconfig.EquipmentDurabilityRules{},
		Forge: &devconfig.ForgeConfig{OreItemID: 910004, GuaranteedPurity: 95, LuckPPM: 150000, LuckWeights: []uint32{1, 1, 1},
			Recipes: []devconfig.ForgeRecipe{{BlueprintItemID: 910021, OutputItemID: 910022, OreCount: 4, Quality: 2, Gold: 30000}}},
		Items: []devconfig.Item{{ID: 910004, MaxStack: 1}, {ID: 910021, MaxStack: 1},
			{ID: 910022, IsEquipment: true, EquipmentSlot: 2, RequiredLevel: &level, MaxStack: 1, DropQuality: 2, OriginalDurabilityU: 10000}},
	}
	role := domain.Role{ID: domain.ID(), Capacity: 6, Wallet: domain.Wallet{Gold: 30000, BoundGold: 6000, Yuanbao: 2, BoundYuanbao: 3}}
	role.Items = append(role.Items, domain.Item{ID: domain.ID(), ItemID: 910021, Quantity: 1, Version: 1,
		Quality: 2, SourceKind: "personal_boss", SourceID: domain.ID()})
	for _, purity := range []uint32{95, 95, 95, 94, 77} {
		role.Items = append(role.Items, domain.Item{ID: domain.ID(), ItemID: 910004, Quantity: 1, Version: 1,
			OrePurity: purity, SourceKind: "mining", SourceID: domain.ID()})
	}
	return role, f
}

func TestForgePlanBindsOnlyActualInputsAndUsesOriginalPurity(t *testing.T) {
	original, f := forgePlanCase()
	for _, row := range []struct {
		name         string
		allowBound   bool
		blueprint    bool
		ore          bool
		boundPayment bool
		otherBound   bool
		wantBound    bool
		wantCode     uint32
	}{
		{"关闭时既有绑金及其他绑定矿不参与", false, false, false, true, true, false, 0},
		{"开启但全部实际投入非绑定", true, false, false, false, true, false, 0},
		{"绑定图纸触发绑定", true, true, false, false, false, true, 0},
		{"任一实际绑定矿触发绑定", true, false, true, false, false, true, 0},
		{"实际扣一单位绑金触发绑定", true, false, false, true, false, true, 0},
		{"关闭时拒绝绑定图纸", false, true, false, false, false, false, p.CodeForgeInputs},
		{"关闭时拒绝绑定矿", false, false, true, false, false, false, p.CodeForgeInputs},
	} {
		t.Run(row.name, func(t *testing.T) {
			role := original
			role.Items = slices.Clone(original.Items)
			role.Items[0].Bound, role.Items[1].Bound, role.Items[5].Bound = row.blueprint, row.ore, row.otherBound
			role.Wallet.BoundGold = 0
			if row.boundPayment {
				role.Wallet.BoundGold = 1
			}
			quote := forgeQuoteRefs(1, role.Items[0], role.Items[1:5], row.allowBound)
			plan, code := buildForgePlan(role, &f, quote)
			if code != row.wantCode {
				t.Fatalf("报价结果 %d，应为 %d", code, row.wantCode)
			}
			if code != 0 {
				return
			}
			var wantBoundGold int64
			if row.allowBound && row.boundPayment {
				wantBoundGold = 1
			}
			if plan.OutputBound != row.wantBound || plan.Spend != (domain.Wallet{Gold: 30000 - wantBoundGold, BoundGold: wantBoundGold}) ||
				plan.SuccessPPM != 947500 || plan.Quality != 2 || plan.RequiredLevel != 20 || plan.Slot != 2 ||
				!reflect.DeepEqual(plan.Ores, role.Items[1:5]) {
				t.Fatal("报价未按原纯度、实际投入和规则计算", plan)
			}
		})
	}
}

func TestForgePlanRejectsInvalidInputsWithoutChanges(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(*domain.Role, *devconfig.Fixture, *pb.ForgeQuoteRequest)
		code   uint32
	}{
		{"功能关闭", func(_ *domain.Role, f *devconfig.Fixture, _ *pb.ForgeQuoteRequest) { f.Forge = nil }, p.CodeForgeUnavailable},
		{"锁定原图纸", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.ForgeQuoteRequest) { r.Items[0].Locked = true }, p.CodeForgeInputs},
		{"原矿被预留", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.ForgeQuoteRequest) {
			r.Items[1].ReservationID = domain.ID()
		}, p.CodeForgeInputs},
		{"原矿版本已改变", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.ForgeQuoteRequest) { r.Items[1].Version++ }, p.CodeForgeInputs},
		{"图纸版本溢出", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.ForgeQuoteRequest) {
			q.Blueprint.Version = math.MaxUint64
		}, p.CodeForgeInputs},
		{"重复原矿", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.ForgeQuoteRequest) { q.Ores[1] = q.Ores[0] }, p.CodeForgeInputs},
		{"原矿来源伪造", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.ForgeQuoteRequest) {
			r.Items[1].SourceKind = "npc_shop"
		}, p.CodeForgeInputs},
		{"缺少原矿", func(_ *domain.Role, _ *devconfig.Fixture, q *pb.ForgeQuoteRequest) { q.Ores = q.Ores[:3] }, p.CodeForgeInputs},
		{"矿石须是有效单块纯度", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.ForgeQuoteRequest) { r.Items[1].OrePurity = 101 }, p.CodeForgeInputs},
		{"未授权不能自动补绑金", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.ForgeQuoteRequest) { r.Wallet.Gold-- }, p.CodeVendorFunds},
		{"背包格式不能容纳实际成品", func(r *domain.Role, _ *devconfig.Fixture, _ *pb.ForgeQuoteRequest) { r.Capacity = 1 }, p.CodeEquipmentBagFull},
	} {
		t.Run(c.name, func(t *testing.T) {
			role, f := forgePlanCase()
			q := forgeQuoteRefs(1, role.Items[0], role.Items[1:5], false)
			c.change(&role, &f, q)
			original := role
			original.Items = slices.Clone(role.Items)
			if _, code := buildForgePlan(role, &f, q); code != c.code || !reflect.DeepEqual(role, original) {
				t.Fatalf("期待无变更并返回 %d，实际 %d", c.code, code)
			}
		})
	}
}

func TestForgeRollAndSpendProduceStableOriginalReceipt(t *testing.T) {
	original, f := forgePlanCase()
	q := forgeQuoteRefs(1, original.Items[0], original.Items[1:5], true)
	plan, code := buildForgePlan(original, &f, q)
	if code != 0 {
		t.Fatal(code)
	}
	// 恰好等于概率分子的判定失败。失败消耗矿和金，图纸及其版本不变。
	failed, receipt := applyForgePlan(original, &f, forgeStoredQuote{ID: domain.ID(), Plan: plan, SuccessRoll: 947500}, "asset:failure")
	if receipt.Success || receipt.Output != nil || failed.Wallet != (domain.Wallet{Gold: 6000, Yuanbao: 2, BoundYuanbao: 3}) ||
		!reflect.DeepEqual(failed.Items, []domain.Item{original.Items[0], original.Items[5]}) || len(original.Items) != 6 {
		t.Fatal("实际失败损失不符原报价", failed.Items, receipt)
	}
	// 平均纯度恰好达到阈值，极端随机数也必须成功。
	original.Items[4].OrePurity = 95
	plan, code = buildForgePlan(original, &f, q)
	if code != 0 || plan.SuccessPPM != forgeMillion {
		t.Fatal("95纯度未显示为必成", plan, code)
	}
	quote := forgeStoredQuote{ID: domain.ID(), Plan: plan, SuccessRoll: 999999, LuckRoll: 149999, LuckWeightRoll: 2}
	next, first := applyForgePlan(original, &f, quote, "asset:success")
	again, second := applyForgePlan(original, &f, quote, "asset:success")
	if !first.Success || first.Output == nil || !first.Output.Bound || first.Output.InitialLuck != 3 || first.Output.DurabilityU != 10000 ||
		!reflect.DeepEqual(next, again) || !reflect.DeepEqual(first, second) || len(next.Items) != 2 ||
		next.Items[0] != original.Items[5] || next.Items[1] != *first.Output || first.Proto().Output.GetInitialLuck() != 3 {
		t.Fatal("同一原报价未稳定产生同一原成品与幸运", first, second)
	}
	other := original
	other.ID = domain.ID()
	_, otherReceipt := applyForgePlan(other, &f, quote, "asset:success")
	if otherReceipt.Output == nil || otherReceipt.Output.ID == first.Output.ID {
		t.Fatal("其他角色使用同一命令编号时和已有装备发生身份冲突")
	}
	def, _ := f.Item(first.Output.ItemID)
	unbound := *first.Output
	unbound.Bound = false
	if CanTrade(*first.Output, def) || !CanTrade(unbound, def) {
		t.Fatal("打造来源与实际绑定交易资格不一致", first.Output)
	}
	unbound.Quality = 1
	if CanTrade(unbound, def) {
		t.Fatal("低品质次级成品绕过了交易资格")
	}
}
