// 本文件定义洗炼隔离候选，包括费用、可生效词条及精确的掉落装备分解来源。
package devconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

const (
	RefineAttributeAttack  uint32 = 1         // 两端物理攻击区间固定加成。
	RefineAttributeDefence uint32 = 2         // 两端物理防御区间固定加成。
	RefineAttributeMaxHP   uint32 = 3         // 最大生命固定加成。
	RefineAttributeMaxMP   uint32 = 5         // 最大魔法值固定加成。
	RefineAttributeMagic   uint32 = 12        // 两端魔法攻击区间固定加成。
	RefineAttributeTao     uint32 = 13        // 两端道术攻击区间固定加成。
	refineBasisPoints             = 10_000    // 档位倍率的一个单位属性所含万分刻度。
	refineMillion                 = 1_000_000 // 百万分概率的完整取样区间。
)

// EquipmentRefineConfig 用同一组候选配置计算真实可洗炼词条和操作费用。
type EquipmentRefineConfig struct {
	StoneItemID uint32                     `json:"stone_item_id"` // 原洗炼石模板。
	Targets     []EquipmentRefineTarget    `json:"targets"`       // 本轮实际支持的装备与品质。
	Qualities   []EquipmentRefineQuality   `json:"qualities"`     // 当前支持品质的槽数及材料费。
	Affixes     []EquipmentRefineAffix     `json:"affixes"`       // 本轮已接实际战斗的候选词条。
	Tiers       []EquipmentRefineTier      `json:"tiers"`         // 五档乘数和权重。
	GoldBands   []EquipmentRefineGoldBand  `json:"gold_bands"`    // 按原装备需求等级所处档支付金币。
	LockCosts   []EquipmentRefineLockCost  `json:"lock_costs"`    // 锁槽后每次生成新候选的元宝费。
	Recycle     EquipmentRefineRecycleRule `json:"recycle"`       // 只接确切的原怪物装备来源。
}

// EquipmentRefineTarget 限定当前隔离候选中的实际装备模板和实例品质。
type EquipmentRefineTarget struct {
	ItemID  uint32 `json:"item_id"` // 原装备模板编号。
	Quality uint32 `json:"quality"` // 必须与装备的实际品质一致。
}

// EquipmentRefineQuality 使用工作簿的原槽数和实际材料数量。
type EquipmentRefineQuality struct {
	Quality            uint32 `json:"quality"`              // 实际实例品质。
	SlotCount          uint32 `json:"slot_count"`           // 同时有效的词条槽数量。
	StoneCost          int64  `json:"stone_cost"`           // 一次生成新候选应扣的洗炼石数量。
	AllowDuplicateAttr bool   `json:"allow_duplicate_attr"` // 此隔离池是否允许同件重复词条。
}

// EquipmentRefineAffix 保存原稳定编号、出现资格和按阶段计算的基值。
type EquipmentRefineAffix struct {
	ID          uint32           `json:"id"`            // 唯一工作簿的词条编号。
	Key         string           `json:"key"`           // 战斗管线采用的原属性键。
	Name        string           `json:"name"`          // 公开中文名称。
	MinQuality  uint32           `json:"min_quality"`   // 此词条最低品质。
	Weight      uint32           `json:"weight"`        // 隔离候选的类型相对权重。
	BaseValuesU map[uint32]int64 `json:"base_values_u"` // 阶段到固定基值的万分倍率前取值，单位为十分之一属性。
}

// EquipmentRefineTier 由原五档乘数和百万分率组成。
type EquipmentRefineTier struct {
	Tier         uint32 `json:"tier"`          // 档位取值为一至五。
	MultiplierBP uint32 `json:"multiplier_bp"` // 固定基值的万分倍率。
	WeightPPM    uint32 `json:"weight_ppm"`    // 百万分率，整组和为一百万。
}

// EquipmentRefineGoldBand 按原装备需求等级计算金币价。
type EquipmentRefineGoldBand struct {
	MinLevel uint32 `json:"min_level"` // 本档最低需求等级。
	MaxLevel uint32 `json:"max_level"` // 本档最高需求等级，零表示无上限。
	Gold     int64  `json:"gold"`      // 每次生成新结果实际扣除的金币总量。
}

// EquipmentRefineLockCost 保存锁槽数与每次支付的小单位元宝。
type EquipmentRefineLockCost struct {
	LockedSlots uint32 `json:"locked_slots"` // 本次保留的当前词条数量。
	IngotU      int64  `json:"ingot_u"`      // 非绑定或绑定元宝副本采用相同的整数单位。
}

// EquipmentRefineRecycleRule 独立描述本轮已核对的手动分解渠道。
type EquipmentRefineRecycleRule struct {
	ItemID         uint32 `json:"item_id"`         // 唯一允许分解产石的装备模板。
	SourceKind     string `json:"source_kind"`     // 原装备必须来自普通怪物掉落。
	Quality        uint32 `json:"quality"`         // 原装备的真实实例品质。
	Stage          uint32 `json:"stage"`           // 原装备对应的等级阶段。
	ProbabilityPPM uint32 `json:"probability_ppm"` // 该来源渠道的分解抽样概率。
	Quantity       int64  `json:"quantity"`        // 每次命中的实际石数。
}

// UnmarshalJSON 拒绝拼写错误的规则和未定义的投产字段。
func (c *EquipmentRefineConfig) UnmarshalJSON(data []byte) error {
	type fields EquipmentRefineConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*fields)(c))
}

// Target 检查装备模板与实际品质是否在本轮候选中。
func (c *EquipmentRefineConfig) Target(itemID, quality uint32) bool {
	if c == nil {
		return false
	}
	for _, target := range c.Targets {
		if target.ItemID == itemID && target.Quality == quality {
			return true
		}
	}
	return false
}

// Quality 查找已装配品质的槽数及石头费用。
func (c *EquipmentRefineConfig) Quality(quality uint32) (EquipmentRefineQuality, bool) {
	if c != nil {
		for _, row := range c.Qualities {
			if row.Quality == quality {
				return row, true
			}
		}
	}
	return EquipmentRefineQuality{}, false
}

// Affix 读取当前已开放的属性定义。
func (c *EquipmentRefineConfig) Affix(id uint32) (EquipmentRefineAffix, bool) {
	if c != nil {
		for _, row := range c.Affixes {
			if row.ID == id {
				return row, true
			}
		}
	}
	return EquipmentRefineAffix{}, false
}

// GoldCost 读取原装备需求等级对应的金币档位。
func (c *EquipmentRefineConfig) GoldCost(requiredLevel uint32) (int64, bool) {
	if c != nil {
		for _, row := range c.GoldBands {
			if requiredLevel >= row.MinLevel && (row.MaxLevel == 0 || requiredLevel <= row.MaxLevel) {
				return row.Gold, true
			}
		}
	}
	return 0, false
}

// LockCost 读取本次锁槽数对应的小单位元宝成本。
func (c *EquipmentRefineConfig) LockCost(count uint32) (int64, bool) {
	if c != nil && count < uint32(len(c.LockCosts)) && c.LockCosts[count].LockedSlots == count {
		return c.LockCosts[count].IngotU, true
	}
	return 0, false
}

// ValueUnits 在十分之一属性单位上应用原档位倍率，结果只截断一次。
func (c *EquipmentRefineConfig) ValueUnits(attributeID, stage, tier uint32) (int64, bool) {
	row, exists := c.Affix(attributeID)
	if !exists || tier == 0 || tier > uint32(len(c.Tiers)) || c.Tiers[tier-1].Tier != tier {
		return 0, false
	}
	base := row.BaseValuesU[stage]
	multiplier := int64(c.Tiers[tier-1].MultiplierBP)
	if base <= 0 || multiplier <= 0 || base > math.MaxInt64/multiplier {
		return 0, false
	}
	value := base * multiplier / refineBasisPoints
	return value, value > 0
}

// validateEquipmentRefine 禁止未登记材料标志、错误的装备用途和无可靠来源的抽样池。
func (f *Fixture) validateEquipmentRefine() error {
	rules := f.EquipmentRefine
	var stones []Item // 独立洗炼石标志必须与启用的配置一一对应。
	for _, item := range f.Items {
		if item.EquipmentRefineStone {
			stones = append(stones, item)
		}
	}
	if rules == nil {
		if len(stones) != 0 {
			return errors.New("未配置装备洗炼时不能标记洗炼石")
		}
		return nil
	}
	if f.RuleVersion < 30 || !f.DevelopmentOnly || f.RuntimeReady || f.EquipmentRecycling == nil || f.EquipmentDurability == nil || len(stones) != 1 {
		return errors.New("洗炼候选要求单一原石、装备回收及耐久合同")
	}
	if err := f.validateEquipmentRefineStone(stones[0]); err != nil {
		return err
	}
	if err := rules.validateTables(); err != nil {
		return err
	}
	if err := f.validateEquipmentRefineTargets(); err != nil {
		return err
	}
	return f.validateEquipmentRefineRecycle(stones[0].MaxStack)
}

// validateEquipmentRefineStone 使洗炼石在库存、洗炼与非绑转移中保持单一模板。
func (f *Fixture) validateEquipmentRefineStone(stone Item) error {
	if stone.ID == 0 || stone.ID != f.EquipmentRefine.StoneItemID || strings.TrimSpace(stone.Name) == "" ||
		stone.IsEquipment || stone.EquipmentSlot != 0 || stone.DropQuality != 0 || stone.EquipmentRecycleMaterial ||
		!stone.Stackable || stone.MaxStack < 1 || stone.BookSkillID != 0 || stone.IsSkillPage || stone.RecycleGold != 0 ||
		stone.OriginalDurabilityU != 0 || f.IsCastingSupply(stone.ID) ||
		(f.Wings != nil && f.Wings.FeatherItemID == stone.ID) || (f.LuckOil != nil && f.LuckOil.ItemID == stone.ID) ||
		(f.EquipmentEnhancement != nil && f.EquipmentEnhancement.StoneItemID == stone.ID) {
		return errors.New("洗炼石须是唯一且独立的可堆叠材料")
	}
	count := 0
	for _, item := range f.Items {
		if item.ID == stone.ID {
			count++
		}
	}
	if count != 1 {
		return errors.New("原洗炼石模板编号重复")
	}
	for _, potion := range f.Potions.Effects {
		if potion.ItemID == stone.ID {
			return errors.New("洗炼石不能同时作为药品使用")
		}
	}
	return nil
}

// validateTables 拒绝遗漏价档、未知属性和无法抽样的固定档位。
func (c *EquipmentRefineConfig) validateTables() error {
	if len(c.Tiers) != 5 || len(c.LockCosts) != 4 || len(c.GoldBands) == 0 || len(c.Qualities) == 0 || len(c.Affixes) == 0 {
		return errors.New("洗炼候选缺少完整的价档、词条或固定档位")
	}
	var sum uint64 // 档位概率必须恰好覆盖一百万个样本。
	var multiplier uint32
	for index, row := range c.Tiers {
		if row.Tier != uint32(index+1) || row.MultiplierBP <= multiplier || row.WeightPPM == 0 {
			return errors.New("洗炼的五档概率与倍率无效")
		}
		multiplier = row.MultiplierBP
		sum += uint64(row.WeightPPM)
	}
	if sum != refineMillion {
		return errors.New("洗炼档位概率须合计一百万")
	}
	var previousCost int64
	for index, row := range c.LockCosts {
		if row.LockedSlots != uint32(index) || (index == 0 && row.IngotU != 0) || (index > 0 && row.IngotU <= previousCost) {
			return errors.New("洗炼锁槽费用须从零起连续配置并递增")
		}
		previousCost = row.IngotU
	}
	var nextLevel uint32 = 1 // 金币费用自需求一级开始连续覆盖。
	for index, row := range c.GoldBands {
		last := index == len(c.GoldBands)-1
		if row.MinLevel != nextLevel || row.Gold <= 0 || (last && row.MaxLevel != 0) ||
			(!last && (row.MaxLevel < row.MinLevel || row.MaxLevel == math.MaxUint32)) {
			return errors.New("按装备需求等级定价的洗炼金币档须连续且末档无上限")
		}
		nextLevel = row.MaxLevel + 1
	}
	qualities := map[uint32]bool{}
	for _, row := range c.Qualities {
		if row.Quality < 1 || row.Quality > 4 || row.SlotCount != row.Quality || qualities[row.Quality] ||
			row.StoneCost != int64(1)<<row.Quality {
			return errors.New("洗炼品质、实际槽数或材料费与原规则不符")
		}
		qualities[row.Quality] = true
	}
	return c.validateAffixes(multiplier)
}

// validateAffixes 只允许已经由当前服务端战斗逻辑接收的稳定属性编号。
func (c *EquipmentRefineConfig) validateAffixes(multiplier uint32) error {
	allowed := map[uint32]struct {
		key        string // 唯一配置中的稳定属性键。
		minQuality uint32 // 当前策划定义的最低品质。
	}{
		RefineAttributeAttack: {"attack", 2}, RefineAttributeDefence: {"defence", 1}, RefineAttributeMaxHP: {"max_hp", 1},
		RefineAttributeMaxMP: {"max_mp", 1}, RefineAttributeMagic: {"magic", 2}, RefineAttributeTao: {"tao", 2},
	}
	var totalWeight uint64
	for _, row := range c.Affixes {
		original, exists := allowed[row.ID]
		if !exists || original.key != row.Key || original.minQuality != row.MinQuality || strings.TrimSpace(row.Name) == "" ||
			row.Weight == 0 || len(row.BaseValuesU) == 0 {
			return fmt.Errorf("洗炼词条 %d 未接当前战斗效果或未定候选抽样", row.ID)
		}
		delete(allowed, row.ID)
		totalWeight += uint64(row.Weight)
		if totalWeight > math.MaxUint32 {
			return errors.New("洗炼词条合计权重超出抽样范围")
		}
		for _, value := range row.BaseValuesU {
			if value <= 0 || value > math.MaxInt64/int64(multiplier) {
				return fmt.Errorf("洗炼词条 %d 固定值超出安全区间", row.ID)
			}
		}
	}
	return nil
}

// validateEquipmentRefineTargets 核对真实装备及档位至少有足够可抽取的词条。
func (f *Fixture) validateEquipmentRefineTargets() error {
	rules := f.EquipmentRefine
	if len(rules.Targets) == 0 {
		return errors.New("洗炼候选没有开启装备")
	}
	seen := map[uint32]bool{}
	for _, target := range rules.Targets {
		item, exists := f.Item(target.ItemID)
		quality, priced := rules.Quality(target.Quality)
		if !exists || !priced || seen[target.ItemID] || !item.IsEquipment || item.RequiredLevel == nil || item.DropQuality != target.Quality ||
			item.OriginalDurabilityU <= 0 || !item.EquipmentStatsKnown() {
			return fmt.Errorf("洗炼目标 %d 的装备、品质或属性来源无效", target.ItemID)
		}
		seen[target.ItemID] = true
		usableCount := uint32(0) // 可从该装备实际阶段抽中的词条类型数。
		for _, affix := range rules.Affixes {
			if affix.MinQuality <= target.Quality {
				if _, valid := rules.ValueUnits(affix.ID, item.EquipmentStage, 1); !valid {
					return fmt.Errorf("洗炼目标 %d 缺少词条 %d 的阶段基值", target.ItemID, affix.ID)
				}
				usableCount++
			}
		}
		if usableCount < quality.SlotCount {
			return fmt.Errorf("洗炼目标 %d 没有足够独立可用的词条", target.ItemID)
		}
	}
	return nil
}

// validateEquipmentRefineRecycle 只接受真实普通掉落的单件原装备，避免商店原价分解刷石。
func (f *Fixture) validateEquipmentRefineRecycle(maxStack int64) error {
	rules := f.EquipmentRefine
	row := rules.Recycle
	item, exists := f.Item(row.ItemID)
	if !exists || !rules.Target(row.ItemID, row.Quality) || !item.EquipmentRecycleKnown() || item.EquipmentStage != row.Stage ||
		row.SourceKind != "monster_drop" || row.ProbabilityPPM == 0 || row.ProbabilityPPM > refineMillion || row.Quantity != 1 || maxStack < row.Quantity {
		return errors.New("洗炼石分解须引用可核对的普通怪物原装备和单石概率")
	}
	for _, pool := range f.OrdinaryDropPools {
		for _, entry := range pool.Equipment {
			if entry.ItemID == row.ItemID && entry.QuantityMin == 1 && entry.QuantityMax == 1 {
				return nil
			}
		}
	}
	return errors.New("洗炼石分解的装备在普通怪物原掉落池中不可取得")
}
