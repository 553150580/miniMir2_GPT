// 本文件定义独立矿层、矿种、纯度和工具候选配置，供隔离挖矿运行验收。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
)

const SceneKindMining uint32 = 7 // 独立矿区活动，不使用普通战斗的怪物目录。

const miningProbabilityScale uint32 = 1_000_000 // 成功率和纯度权重都用整数百万分率。

var miningResourceKey = regexp.MustCompile(`^[A-Za-z0-9_/-]+$`) // 本包可加载的相对资源键。

// MiningConfig 固定一次挖矿执行所需的不可变规则。
type MiningConfig struct {
	IntervalMS  int64                    `json:"interval_ms"`            // 相邻判定之间的服务端时长。
	SuccessPPM  uint32                   `json:"success_ppm"`            // 每次有效判定的出矿率。
	ToolWearU   int64                    `json:"tool_wear_u"`            // 每次有效判定消耗的工具耐久存储单位。
	Layers      []MiningLayer            `json:"layers"`                 // 具备隔离地图映射的楼层。
	Ores        []MiningOre              `json:"ores"`                   // 四种矿石的物品映射及两层抽样权重。
	Purity      []MiningPurityBand       `json:"purity"`                 // 按纯度升序的连续抽样档。
	Tools       []MiningTool             `json:"tools"`                  // 每个隔离楼层可使用的独立锄头。
	AutoRecycle *MiningAutoRecycleConfig `json:"auto_recycle,omitempty"` // 矿石自动回收可引用的商店价格和矿种范围。
}

// MiningLayer 保存工具和矿种抽样依据的永久楼层编号与准入条件。
type MiningLayer struct {
	Layer        uint32      `json:"layer"`         // 比奇矿区的第一层或第二层。
	MinimumLevel uint32      `json:"minimum_level"` // 角色进该楼层需要达到的等级。
	Map          MapTemplate `json:"map"`           // 隔离验收显式指定的静态地形。
}

// MiningOre 把工作簿唯一矿种映射到实际可入背包的独立道具。
type MiningOre struct {
	Key          string    `json:"key"`           // 工作簿中的矿种键。
	ItemID       uint32    `json:"item_id"`       // 矿石实例使用的配置编号。
	LayerWeights [2]uint32 `json:"layer_weights"` // 第一层、第二层的抽样权重，分别在全矿种间合计100。
}

// MiningPurityBand 先以百万分率选档，再在闭区间内均匀抽一个整数。
type MiningPurityBand struct {
	Min       uint32 `json:"min"`        // 最小纯度，包含此值。
	Max       uint32 `json:"max"`        // 最大纯度，包含此值。
	WeightPPM uint32 `json:"weight_ppm"` // 本档在所有成功样本中的百万分率。
}

// MiningTool 保存明确指定的工具模板、楼层及消耗品耐久。
type MiningTool struct {
	ItemID      uint32 `json:"item_id"`      // 背包中锄头的物品配置编号。
	Layer       uint32 `json:"layer"`        // 此隔离工具对应的矿层编号。
	DurabilityU int64  `json:"durability_u"` // 以存储单位表示的初始耐久。
}

// UnmarshalJSON 拒绝隔离挖矿配置混入未装配的回收、商城或正式开关。
func (m *MiningConfig) UnmarshalJSON(body []byte) error {
	type miningFields MiningConfig
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*miningFields)(m))
}

// UnmarshalJSON 要求每个矿种恰好保存两层权重，不忽略输入末尾多出的权重。
func (m *MiningOre) UnmarshalJSON(body []byte) error {
	var row struct {
		Key          string   `json:"key"`           // 原表矿种键。
		ItemID       uint32   `json:"item_id"`       // 矿石物品模板编号。
		LayerWeights []uint32 `json:"layer_weights"` // 暂按切片读取以核对精确长度。
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&row); err != nil {
		return err
	}
	if len(row.LayerWeights) != len(m.LayerWeights) {
		return errors.New("矿种抽样须明确提供第一层和第二层的权重")
	}
	*m = MiningOre{Key: row.Key, ItemID: row.ItemID, LayerWeights: [2]uint32{row.LayerWeights[0], row.LayerWeights[1]}}
	return nil
}

// MiningLayer 使用地图编号查找已经装配的矿层，普通地图及缺省配置均关闭挖矿。
func (f *Fixture) MiningLayer(mapID uint64) (*MiningLayer, bool) {
	if f == nil || f.Mining == nil || mapID == 0 {
		return nil, false
	}
	for i := range f.Mining.Layers {
		if f.Mining.Layers[i].Map.ID == mapID {
			return &f.Mining.Layers[i], true
		}
	}
	return nil, false
}

// MiningTool 按原实例的物品配置编号查找锄头规则。
func (f *Fixture) MiningTool(itemID uint32) (MiningTool, bool) {
	if f == nil || f.Mining == nil || itemID == 0 {
		return MiningTool{}, false
	}
	for _, tool := range f.Mining.Tools {
		if tool.ItemID == itemID {
			return tool, true
		}
	}
	return MiningTool{}, false
}

// validateMining 核对候选抽样、地图和逐实例资产。旧规则保持全部缺省关闭。
func (f *Fixture) validateMining() error {
	mining := f.Mining
	if mining == nil {
		return nil
	}
	if mining.IntervalMS <= 0 || mining.IntervalMS > math.MaxInt64/int64(time.Millisecond) || mining.IntervalMS%1000 != 0 ||
		mining.SuccessPPM == 0 || mining.SuccessPPM > miningProbabilityScale || mining.ToolWearU <= 0 ||
		len(mining.Layers) < 1 || len(mining.Layers) > 2 || len(mining.Tools) != len(mining.Layers) || len(mining.Ores) != 4 {
		return errors.New("隔离挖矿须有合法的时长、出矿率、工具损耗、矿层和四种矿石")
	}
	layers := make(map[uint32]bool, len(mining.Layers))
	maps := map[uint64]bool{f.Map.ID: true}
	for _, existing := range f.Maps {
		maps[existing.ID] = true
	}
	for i := range mining.Layers {
		layer := &mining.Layers[i]
		m := &layer.Map
		if layer.Layer < 1 || layer.Layer > 2 || layer.MinimumLevel == 0 || layers[layer.Layer] || maps[m.ID] ||
			!validChallengeGrid(m, SceneKindMining) || strings.TrimSpace(m.DisplayName) == "" {
			return fmt.Errorf("矿层%d的进入条件、场景身份或静态地图无效", layer.Layer)
		}
		if !miningResourceKey.MatchString(m.ResourceKey) || strings.HasPrefix(m.ResourceKey, "/") ||
			strings.Contains(m.ResourceKey, "//") || strings.HasSuffix(m.ResourceKey, "/") ||
			m.Source != nil || len(m.SourceStaticBits) != 0 || !m.Walkable(m.PlayerSpawn) {
			return fmt.Errorf("矿层%d缺少可用的隔离资源键或出生点", layer.Layer)
		}
		maps[m.ID], layers[layer.Layer] = true, true
	}
	if err := validateMiningPurity(mining.Purity); err != nil {
		return err
	}
	if err := f.validateMiningItems(layers); err != nil {
		return err
	}
	return f.validateMiningAutoRecycle()
}

// validateMiningPurity 保证纯度1至100恰好覆盖一次，档权重保持可精确抽样。
func validateMiningPurity(bands []MiningPurityBand) error {
	var total uint64
	next := uint32(1)
	for _, band := range bands {
		if band.Min != next || band.Max < band.Min || band.Max > 100 || band.WeightPPM == 0 || band.WeightPPM > miningProbabilityScale {
			return errors.New("矿石纯度档须按1至100连续覆盖，且每个档都有正的整数权重")
		}
		next = band.Max + 1
		total += uint64(band.WeightPPM)
	}
	if next != 101 || total != uint64(miningProbabilityScale) {
		return errors.New("矿石纯度档缺失或总权重并非1000000")
	}
	return nil
}

// validateMiningItems 核对锄头、矿石独占物品身份和两层完整矿种权重。
func (f *Fixture) validateMiningItems(layers map[uint32]bool) error {
	mining := f.Mining
	items := make(map[uint32]bool, len(mining.Tools)+len(mining.Ores))
	toolLayers := make(map[uint32]bool, len(layers))
	for _, tool := range mining.Tools {
		if !layers[tool.Layer] || toolLayers[tool.Layer] || items[tool.ItemID] || tool.DurabilityU < mining.ToolWearU ||
			!f.validMiningItem(tool.ItemID) {
			return fmt.Errorf("矿层%d的锄头映射须唯一，并提供独立物品和足够耐久", tool.Layer)
		}
		items[tool.ItemID], toolLayers[tool.Layer] = true, true
	}
	keys := map[string]bool{"copper": true, "iron": true, "gold": true, "black_iron": true}
	var weight [2]uint64
	for _, ore := range mining.Ores {
		if !keys[ore.Key] || items[ore.ItemID] || !f.validMiningItem(ore.ItemID) {
			return fmt.Errorf("矿种%s与可独立保存纯度的矿石实例映射无效", ore.Key)
		}
		delete(keys, ore.Key)
		items[ore.ItemID] = true
		for i, amount := range ore.LayerWeights {
			if amount == 0 || amount > 100 {
				return fmt.Errorf("矿种%s的第%d层权重无效", ore.Key, i+1)
			}
			weight[i] += uint64(amount)
		}
	}
	if weight != [2]uint64{100, 100} {
		return errors.New("两个矿层的矿种权重须分别合计100")
	}
	return f.validateMiningShops(items)
}

// validateMiningShops 确保锄头可按金币报价获得，矿石只可走指定的纯度报价。
func (f *Fixture) validateMiningShops(items map[uint32]bool) error {
	offers := make(map[uint32]bool, len(f.Mining.Tools))
	for _, shop := range f.NPCShops {
		for _, price := range shop.SellPrices {
			if items[price.ItemID] {
				return errors.New("独立矿石和锄头不得列入通用商人的收购目录")
			}
		}
		for _, offer := range shop.Offers {
			if !items[offer.ItemID] {
				continue
			}
			if _, isTool := f.MiningTool(offer.ItemID); !isTool || offers[offer.ItemID] || offer.Currency != "gold" ||
				offer.PriceUnits <= 0 || offer.Quantity != 1 || offer.MaxPurchaseQuantity != 1 {
				return errors.New("采矿商品只可提供唯一的单把金币锄头报价")
			}
			offers[offer.ItemID] = true
		}
	}
	if len(offers) != len(f.Mining.Tools) {
		return errors.New("每把隔离锄头都须能通过实际 NPC 商店获取")
	}
	return nil
}

// validMiningItem 使本轮单件矿石和工具保留独立身份，避免被装备、技能或通用回收路径处理。
func (f *Fixture) validMiningItem(itemID uint32) bool {
	if itemID == 0 {
		return false
	}
	matches := 0
	for _, item := range f.Items {
		if item.ID != itemID {
			continue
		}
		matches++
		if strings.TrimSpace(item.Name) == "" || item.IsEquipment || item.EquipmentSlot != 0 || item.Stackable ||
			item.MaxStack != 1 || item.BookSkillID != 0 || item.IsSkillPage || item.RecycleGold != 0 {
			return false
		}
	}
	return matches == 1
}
