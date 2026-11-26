// 本文件核对矿区可选夹具、边界校验及原矿区映射的启动兼容性。
package devconfig

import (
	"encoding/json"
	"os"
	"testing"
)

// miningConfigTestFixture 只提供测试独立矿图和单件物品，候选数值以工作簿生成物另验。
func miningConfigTestFixture() Fixture {
	f := Fixture{Map: MapTemplate{ID: 1}}
	f.Mining = &MiningConfig{
		IntervalMS: 3000, SuccessPPM: 300000, ToolWearU: 1,
		Layers: []MiningLayer{{Layer: 1, MinimumLevel: 20, Map: MapTemplate{ID: 901, SceneKind: SceneKindMining,
			DisplayName: "隔离矿图", ResourceKey: "development/maps/personal-map", Width: 13, Height: 13,
			PlayerSpawn: Point{X: 6, Y: 6}, Blocked: []Point{{X: 2, Y: 3}}}}},
		Ores: []MiningOre{{Key: "copper", ItemID: 920001, LayerWeights: [2]uint32{58, 34}},
			{Key: "iron", ItemID: 920002, LayerWeights: [2]uint32{30, 40}},
			{Key: "gold", ItemID: 920003, LayerWeights: [2]uint32{10, 20}},
			{Key: "black_iron", ItemID: 920004, LayerWeights: [2]uint32{2, 6}}},
		Purity: []MiningPurityBand{{Min: 1, Max: 20, WeightPPM: 450000}, {Min: 21, Max: 100, WeightPPM: 550000}},
		Tools:  []MiningTool{{ItemID: 920011, Layer: 1, DurabilityU: 3}},
	}
	f.Items = []Item{{ID: 920001, Name: "铜矿", MaxStack: 1}, {ID: 920002, Name: "铁矿", MaxStack: 1},
		{ID: 920003, Name: "金矿", MaxStack: 1}, {ID: 920004, Name: "黑铁矿", MaxStack: 1},
		{ID: 920011, Name: "隔离锄头", MaxStack: 1}}
	f.NPCShops = []NPCShop{{Key: "grocery", Offers: []SkillShop{{ID: 920011, ItemID: 920011, Currency: "gold", PriceUnits: 50,
		Quantity: 1, MaxPurchaseQuantity: 1, RequiredRoleLevel: 20}}}}
	return f
}

// TestMiningConfigRejectsUnsafeMappings 核对配置出错时会关闭对应候选。
func TestMiningConfigRejectsUnsafeMappings(t *testing.T) {
	valid := miningConfigTestFixture()
	if err := valid.validateMining(); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name string
		edit func(*Fixture)
	}{
		{"含半秒的判定窗口", func(f *Fixture) { f.Mining.IntervalMS = 1500 }},
		{"概率超出百万分率", func(f *Fixture) { f.Mining.SuccessPPM = 1000001 }},
		{"耐久不足一次", func(f *Fixture) { f.Mining.ToolWearU = 4 }},
		{"重用既有地图编号", func(f *Fixture) { f.Mining.Layers[0].Map.ID = f.Map.ID }},
		{"矿图携带战斗刷怪", func(f *Fixture) { f.Mining.Layers[0].Map.MonsterSpawns = []Point{{X: 7, Y: 7}} }},
		{"矿图出生点被阻挡", func(f *Fixture) { f.Mining.Layers[0].Map.PlayerSpawn = Point{X: 2, Y: 3} }},
		{"矿图超出隔离预算", func(f *Fixture) { f.Mining.Layers[0].Map.Width = 30 }},
		{"矿图资源越界", func(f *Fixture) { f.Mining.Layers[0].Map.ResourceKey = "../maps/mine" }},
		{"纯度档重叠", func(f *Fixture) { f.Mining.Purity[1].Min = 20 }},
		{"纯度档权重不满", func(f *Fixture) { f.Mining.Purity[1].WeightPPM-- }},
		{"第二洞层矿种权重缺失", func(f *Fixture) { f.Mining.Ores[0].LayerWeights[1]-- }},
		{"矿石模板变可堆叠", func(f *Fixture) { f.Items[0].Stackable = true }},
		{"锄头占普通武器槽", func(f *Fixture) { f.Items[4].IsEquipment = true }},
		{"锄头无NPC购买途径", func(f *Fixture) { f.NPCShops[0].Offers = nil }},
		{"普通商人误收矿石", func(f *Fixture) { f.NPCShops[0].SellPrices = []NPCSellPrice{{ItemID: 920001, PriceGold: 1}} }},
		{"矿石使用通用回收价", func(f *Fixture) { f.Items[0].RecycleGold = 1 }},
	} {
		t.Run(row.name, func(t *testing.T) {
			fixture := miningConfigTestFixture()
			row.edit(&fixture)
			if err := fixture.validateMining(); err == nil {
				t.Fatal("无效挖矿配置通过启动检查")
			}
		})
	}
	for _, value := range []string{
		`{"ores":[{"key":"copper","item_id":1,"layer_weights":[10,20,30]}]}`,
		`{"ores":[{"key":"copper","item_id":1,"layer_weights":[10]}]}`,
		`{"purity":[{"min":1,"max":100,"weight_ppm":1.5}]}`,
		`{"ore_recycle_rounding":"floor_per_ore"}`,
	} {
		var mining MiningConfig
		if err := json.Unmarshal([]byte(value), &mining); err == nil {
			t.Fatal("不完整或未装配的挖矿配置字段通过检查", value)
		}
	}
}

// TestMiningConfigDefaultClosedAndWorkbookProjection 核对正常加载入口和隔离的真实投影文件。
func TestMiningConfigDefaultClosedAndWorkbookProjection(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Fixture.Mining != nil {
		t.Fatal("规则30默认提前装配矿区")
	}
	if _, exists := cfg.Fixture.MapByID(202601); exists {
		t.Fatal("默认地图目录泄漏隔离矿图")
	}
	if path := os.Getenv("MINIMIR_MINING_TEST_FIXTURE"); path != "" {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var projected Fixture
		if err := json.Unmarshal(body, &projected); err != nil {
			t.Fatal(err)
		}
		if err := projected.Validate(); err != nil {
			t.Fatal(err)
		}
		mining := projected.Mining
		if mining == nil || mining.IntervalMS != 3000 || mining.SuccessPPM != 300000 || mining.ToolWearU != 1 || len(mining.Purity) != 5 {
			t.Fatal("与真实工作簿的时间、概率、耐久或纯度投影不符", mining)
		}
		for _, row := range []struct {
			id               uint64
			floor, min, tool uint32
		}{{202601, 1, 20, 910011}, {202602, 2, 36, 910012}} {
			layer, ok := projected.MiningLayer(row.id)
			tool, toolOK := projected.MiningTool(row.tool)
			m, mapOK := projected.MapByID(row.id)
			if !ok || !toolOK || !mapOK || m != &layer.Map || m.SceneKindValue() != SceneKindMining || m.CellBudget() != 169 ||
				!m.Walkable(m.PlayerSpawn) || layer.Layer != row.floor || layer.MinimumLevel != row.min || tool.Layer != row.floor {
				t.Fatal("实际加载后的矿层、坐标、工具或等级不符", row)
			}
		}
	}
}
