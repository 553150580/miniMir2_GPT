// 本文件复核玩家选中的原图纸、黑铁和实际扣款，执行一次已经定价的打造。
package persistence

import (
	"math"
	"slices"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

const forgeSourceKind = "forge_craft"  // 成品的永久来源名称。
const forgeMillion = uint32(1_000_000) // 成功和幸运概率的分母。

// forgePlan 固定确认窗里的所有投入、钱包分账和可能的成品。
type forgePlan struct {
	Blueprint     domain.Item   `bson:"blueprint"`      // 成功才消耗的原图纸。
	Ores          []domain.Item `bson:"ores"`           // 每次实际打造都消耗的独立黑铁。
	Spend         domain.Wallet `bson:"spend"`          // 成功和失败均按此扣原钱包。
	AllowBound    bool          `bson:"allow_bound"`    // 此次报价的绑定材料授权。
	OutputItemID  uint32        `bson:"output_item_id"` // 配方确定的成品配置编号。
	Quality       uint32        `bson:"quality"`        // 配方确定的成品品质。
	RequiredLevel uint32        `bson:"required_level"` // 成品穿戴等级，不从原矿计算。
	Slot          uint32        `bson:"slot"`           // 成品装备槽位。
	SuccessPPM    uint32        `bson:"success_ppm"`    // 实际成功概率，单位百万分之一。
	OutputBound   bool          `bson:"output_bound"`   // 根据本次实际使用的图纸、黑铁和金币汇总。
}

// buildForgePlan 使用当次角色数据计算报价，提交前再次调用并逐字段对比。
func buildForgePlan(role domain.Role, fixture *devconfig.Fixture, q *pb.ForgeQuoteRequest) (forgePlan, uint32) {
	if fixture.Forge == nil {
		return forgePlan{}, p.CodeForgeUnavailable
	}
	if q == nil || q.SceneEpoch == 0 || len(q.ProtoReflect().GetUnknown()) != 0 || !equipmentRoleUnique(role) {
		return forgePlan{}, p.CodeInvalidArgument
	}
	blueprint, ok := forgeOwnedItem(role.Items, q.Blueprint)
	if !ok || !validForgeBlueprint(blueprint, fixture) || blueprint.Bound && !q.AllowBound {
		return forgePlan{}, p.CodeForgeInputs
	}
	recipe, exists := fixture.ForgeRecipe(blueprint.ItemID)
	if !exists || recipe.OreCount == 0 || uint32(len(q.Ores)) != recipe.OreCount {
		return forgePlan{}, p.CodeForgeInputs
	}
	output, hasOutput := fixture.Item(recipe.OutputItemID)
	oreDefinition, hasOre := fixture.Item(fixture.Forge.OreItemID)
	if !hasOutput || !hasOre || !output.IsEquipment || output.RequiredLevel == nil || recipe.Gold <= 0 {
		return forgePlan{}, p.CodeForgeUnavailable
	}
	plan := forgePlan{Blueprint: blueprint, Ores: make([]domain.Item, 0, len(q.Ores)), AllowBound: q.AllowBound,
		OutputItemID: output.ID, Quality: recipe.Quality, RequiredLevel: *output.RequiredLevel,
		Slot: output.EquipmentSlot, OutputBound: blueprint.Bound}
	seen := map[string]bool{blueprint.ID: true} // 图纸与所有黑铁的原实例均不得重复。
	var purity uint32                           // 矿石纯度总和，保持整数供成功率计算。
	for _, ref := range q.Ores {
		ore, owned := forgeOwnedItem(role.Items, ref)
		if !owned || ore.ItemID != oreDefinition.ID || seen[ore.ID] || !validSingleMiningOre(ore, oreDefinition) ||
			ore.Bound && !q.AllowBound || ore.OriginalDurabilityU != 0 || ore.MaxDurabilityU != 0 || ore.DurabilityU != 0 || ore.InitialLuck != 0 {
			return forgePlan{}, p.CodeForgeInputs
		}
		seen[ore.ID] = true
		plan.Ores = append(plan.Ores, ore)
		purity += ore.OrePurity
		plan.OutputBound = plan.OutputBound || ore.Bound
	}
	if q.AllowBound {
		plan.Spend.BoundGold = min(max(role.Wallet.BoundGold, 0), recipe.Gold)
	}
	plan.Spend.Gold = recipe.Gold - plan.Spend.BoundGold
	if !vendorCanPay(role.Wallet, plan.Spend) {
		return forgePlan{}, p.CodeVendorFunds
	}
	// 两种结果的背包格数相同。成功替换图纸槽，失败保留图纸，两者都移除黑铁。
	if len(role.Items)-len(plan.Ores) > role.Capacity {
		return forgePlan{}, p.CodeEquipmentBagFull
	}
	plan.OutputBound = plan.OutputBound || plan.Spend.BoundGold > 0
	plan.SuccessPPM = purity * 10_000 / recipe.OreCount
	if purity >= fixture.Forge.GuaranteedPurity*recipe.OreCount {
		plan.SuccessPPM = forgeMillion
	}
	return plan, 0
}

// forgeOwnedItem 查找玩家选定的原背包实例、版本及可消耗状态。
func forgeOwnedItem(items []domain.Item, ref *pb.ForgeItemRef) (domain.Item, bool) {
	if ref == nil || len(ref.InstanceId) != 16 || ref.Version < 1 || ref.Version > math.MaxInt64 ||
		len(ref.ProtoReflect().GetUnknown()) != 0 {
		return domain.Item{}, false
	}
	id := domain.Hex(ref.InstanceId)
	for _, item := range items {
		if item.ID == id && uint64(item.Version) == ref.Version && item.Quantity == 1 && !item.Locked && !equipmentOccupied(item) {
			return item, true
		}
	}
	return domain.Item{}, false
}

// validForgeBlueprint 核对真实来源和图纸身份；绑定限制由本次明确的授权判定。
func validForgeBlueprint(item domain.Item, fixture *devconfig.Fixture) bool {
	definition, exists := fixture.Item(item.ItemID)
	if !exists || definition.IsEquipment || definition.Stackable || item.OrePurity != 0 || item.InitialLuck != 0 ||
		item.OriginalDurabilityU != 0 || item.MaxDurabilityU != 0 || item.DurabilityU != 0 {
		return false
	}
	item.Bound = false // 只借交易校验核对永久资产结构与来源白名单。
	return CanTrade(item, definition)
}

// applyForgePlan 对被冻结的计划执行原扣款和消耗。随机值只读报价，事务重试得到同一结果。
func applyForgePlan(role domain.Role, fixture *devconfig.Fixture, quote forgeStoredQuote, operationID string) (domain.Role, ForgeResultReceipt) {
	plan := quote.Plan
	result := ForgeResultReceipt{QuoteID: quote.ID, Scene: quote.Scene, Plan: plan, Success: quote.SuccessRoll < plan.SuccessPPM}
	spent := make(map[string]bool, len(plan.Ores)+1)
	for _, ore := range plan.Ores {
		spent[ore.ID] = true
	}
	if result.Success {
		spent[plan.Blueprint.ID] = true
	}
	role.Items = slices.DeleteFunc(slices.Clone(role.Items), func(item domain.Item) bool { return spent[item.ID] })
	role.Wallet.Gold -= plan.Spend.Gold
	role.Wallet.BoundGold -= plan.Spend.BoundGold
	if result.Success {
		// 角色身份独立进入摘要，两个角色使用相同的客户端命令编号也不会生成同一原件。
		output := domain.Item{ID: Hash([]byte("forge-output:" + role.ID + ":" + operationID))[:32], ItemID: plan.OutputItemID,
			Quantity: 1, Version: 1, Bound: plan.OutputBound, Quality: plan.Quality, SourceKind: forgeSourceKind,
			SourceID: quote.ID, InitialLuck: forgeInitialLuck(quote, fixture.Forge)}
		output = InitializeItemDurability(output, fixture)
		role.Items = append(role.Items, output)
		result.Output = &output
	}
	return role, result
}

// forgeInitialLuck 仅在已经成功生成装备后读取独立随机值和原规则权重。
func forgeInitialLuck(quote forgeStoredQuote, rules *devconfig.ForgeConfig) uint32 {
	if quote.LuckRoll >= rules.LuckPPM {
		return 0
	}
	var threshold uint64
	for index, weight := range rules.LuckWeights {
		threshold += uint64(weight)
		if quote.LuckWeightRoll < threshold {
			return uint32(index + 1)
		}
	}
	return 0
}
