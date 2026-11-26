// 本文件核对图纸配方的隔离开关、材料和成品引用。
package devconfig

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func TestForgeConfigValidatesReferences(t *testing.T) {
	const blueprintID, oreID, outputID = 910021, 910004, 910022
	newFixture := func() Fixture {
		level := uint32(20)
		return Fixture{
			Forge: &ForgeConfig{OreItemID: oreID, GuaranteedPurity: 95, LuckPPM: 150_000, LuckWeights: []uint32{1, 1, 1},
				Recipes: []ForgeRecipe{{BlueprintItemID: blueprintID, OutputItemID: outputID, OreCount: 4, Quality: 2, Gold: 30_000}}},
			Mining: &MiningConfig{Ores: []MiningOre{{Key: "black_iron", ItemID: oreID}}},
			Items: []Item{{ID: blueprintID, Name: "矿纹项链图纸", MaxStack: 1, DropQuality: 2},
				{ID: outputID, Name: "矿纹项链", EquipmentSlot: 3, IsEquipment: true, RequiredLevel: &level, EquipmentStage: 1,
					AllowedJobs: []uint32{0, 1, 2}, DropQuality: 2, OriginalDurabilityU: 7000}},
		}
	}
	fixture := newFixture()
	if err := fixture.validateForge(); err != nil {
		t.Fatal(err)
	}
	if got, ok := fixture.ForgeRecipe(blueprintID); !ok || got.OutputItemID != outputID {
		t.Fatal("图纸未映射到原配方", got)
	}
	for _, test := range []struct {
		name   string
		mutate func(*Fixture)
	}{
		{"普通矿不能作为黑铁", func(f *Fixture) { f.Mining.Ores[0].Key = "copper" }},
		{"必成纯度超界", func(f *Fixture) { f.Forge.GuaranteedPurity = 101 }},
		{"幸运概率超界", func(f *Fixture) { f.Forge.LuckPPM = 1_000_001 }},
		{"幸运权重溢出", func(f *Fixture) { f.Forge.LuckWeights[0] = math.MaxUint32 }},
		{"图纸误用药品", func(f *Fixture) { f.Potions.Effects = []PotionEffect{{ItemID: blueprintID}} }},
		{"图纸支持堆叠", func(f *Fixture) { f.Items[0].Stackable = true }},
		{"成品是 T1 装备", func(f *Fixture) { *f.Items[1].RequiredLevel = 10 }},
		{"成品品质不符", func(f *Fixture) { f.Items[1].DropQuality = 3 }},
		{"成品部位不合法", func(f *Fixture) { f.Items[1].EquipmentSlot = 10 }},
		{"成品职业重复", func(f *Fixture) { f.Items[1].AllowedJobs = []uint32{0, 0} }},
		{"耗矿数量超界", func(f *Fixture) { f.Forge.Recipes[0].OreCount = 9 }},
		{"缺少整数金币费用", func(f *Fixture) { f.Forge.Recipes[0].Gold = 0 }},
		{"图纸定义重复", func(f *Fixture) { f.Items = append(f.Items, f.Items[0]) }},
		{"配方重复", func(f *Fixture) { f.Forge.Recipes = append(f.Forge.Recipes, f.Forge.Recipes[0]) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newFixture()
			test.mutate(&fixture)
			if fixture.validateForge() == nil {
				t.Fatal("不合法配方通过装载")
			}
		})
	}
}

func TestForgeProjectedFixtureAndDefault(t *testing.T) {
	fixture, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Fixture.Forge != nil {
		t.Fatal("默认试玩已启用隔离打造")
	}
	path := os.Getenv("MINIMIR_FORGE_TEST_FIXTURE")
	if path == "" {
		return
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var candidate Fixture
	if err := json.Unmarshal(payload, &candidate); err != nil {
		t.Fatal(err)
	}
	if err := candidate.Validate(); err != nil {
		t.Fatal(err)
	}
	const blueprintID = 910021
	recipe, ok := candidate.ForgeRecipe(blueprintID)
	if !ok || recipe.OutputItemID != 910022 || recipe.Gold != 30_000 || recipe.OreCount != 4 {
		t.Fatal("隔离成品数据没有使用原公布候选", recipe)
	}
	boss := candidate.PersonalBossByTemplate(3)
	if boss == nil || boss.ItemReward == nil || len(boss.ItemReward.Items) != 1 ||
		boss.ItemReward.Items[0] != (ChallengeRewardItem{ItemID: blueprintID, Quantity: 1}) {
		t.Fatal("原个人首领模板未承载单张图纸胜利附件", boss)
	}
}
