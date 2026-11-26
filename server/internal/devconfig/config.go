// 本文件加载启动配置与规则快照，锁定回环服务和开发副本集，分派各玩法校验并计算快照摘要。

package devconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Point 表示以网格为单位的地图横纵坐标。
type Point struct {
	// X 记录横坐标，使用数值表示。
	X int `json:"x"`
	// Y 记录纵坐标，使用数值表示。
	Y int `json:"y"`
}

// Fighter 定义按职业或来源读取的战斗属性、成长及动作时序参数。
type Fighter struct {
	// CanPush 标记是否处于“can”推入状态。
	CanPush bool `json:"can_push"`
	// Level 记录等级，使用数值表示。
	Level uint32 `json:"level"`
	// JobID 标识当前业务使用的任务或职业编号。
	JobID uint32 `json:"job_id"`
	// DefinitionID 标识当前业务使用的“definition”。
	DefinitionID uint32 `json:"definition_id"`
	// HP 记录生命值，使用数值表示。
	HP int64 `json:"hp"`
	// MP 记录魔法值，使用数值表示。
	MP int64 `json:"mp"`
	// AttackMin 记录攻击最小，使用数值表示。
	AttackMin int64 `json:"attack_min"`
	// AttackMax 记录攻击最大，使用数值表示。
	AttackMax int64 `json:"attack_max"`
	// Defence 记录防御，使用数值表示。
	Defence int64 `json:"defence"`
	// AttackMS 记录攻击毫秒，使用数值表示。
	AttackMS int64 `json:"attack_ms"`
	// AttackSpeed 记录攻击速度，使用数值表示。
	AttackSpeed int64 `json:"attack_speed"`
	// MoveMS 记录移动毫秒，使用数值表示。
	MoveMS int64 `json:"move_ms"`
	// MagicMin 记录魔法最小，使用数值表示。
	MagicMin int64 `json:"magic_min"`
	// MagicMax 记录魔法最大，使用数值表示。
	MagicMax int64 `json:"magic_max"`
	// TaoMin 记录“tao”最小，使用数值表示。
	TaoMin int64 `json:"tao_min"`
	// TaoMax 记录“tao”最大，使用数值表示。
	TaoMax int64 `json:"tao_max"`
	// DefenceMin 记录防御最小，使用数值表示。
	DefenceMin int64 `json:"defence_min"`
	// DefenceMax 记录防御最大，使用数值表示。
	DefenceMax int64 `json:"defence_max"`
	// MagicDefenceMin 记录魔法防御最小，使用数值表示。
	MagicDefenceMin int64 `json:"magic_defence_min"`
	// MagicDefenceMax 记录魔法防御最大，使用数值表示。
	MagicDefenceMax int64 `json:"magic_defence_max"`
	// Accuracy 记录“accuracy”，使用数值表示。
	Accuracy int64 `json:"accuracy"`
	// Agility 记录“agility”，使用数值表示。
	Agility int64 `json:"agility"`
	// MagicResist 记录魔法“resist”，使用数值表示。
	MagicResist int64 `json:"magic_resist"`
	// MagicResistWeight 记录魔法“resist”权重，使用数值表示。
	MagicResistWeight int64 `json:"magic_resist_weight"`
	// canPushExplicit 标记是否处于“can”推入显式状态。
	canPushExplicit bool
}

// Monster 定义怪物的战斗属性、掉落范围与编号。
type Monster struct {
	// Fighter 提供嵌入的战斗单位能力。
	Fighter
	// ID 区分当前步骤指向的目标对象。
	ID uint32 `json:"id"`
	// Exp 记录经验，使用数值表示。
	Exp int64 `json:"exp"`
	// GoldMin 记录金币最小，使用数值表示。
	GoldMin int64 `json:"gold_min"`
	// GoldMax 记录金币最大，使用数值表示。
	GoldMax int64 `json:"gold_max"`
	// RespawnMS 记录复活毫秒，使用数值表示。
	RespawnMS int64 `json:"respawn_ms"`
	// Undead 标记是否处于“undead”状态。
	Undead bool `json:"undead"`
	// CoolEye 对应原怪物的冷眼能力，等级足够时可识破隐身。
	CoolEye bool `json:"cool_eye,omitempty"`
	// CanTame 标记可被诱惑之光转化为临时宠物的普通怪物。
	CanTame bool `json:"can_tame,omitempty"`
	// DropPoolID 指向普通击杀的道具与装备子池。历史配置使用全局掉落池。
	DropPoolID uint32 `json:"drop_pool_id,omitempty"`
}

// Item 定义商品和掉落引用的物品模板及其用途和属性。
type Item struct {
	// ID 区分当前步骤指向的目标对象。
	ID uint32 `json:"id"`
	// Name 记录名称，使用文本表示。
	Name string `json:"name"`
	// EquipmentSlot 记录装备槽位，使用数值表示。
	EquipmentSlot uint32 `json:"equipment_slot"`
	// IsEquipment 标记是否处于“is”装备状态。
	IsEquipment bool `json:"is_equipment"`
	// AttackBonus 记录攻击加成，使用数值表示。
	AttackBonus int64 `json:"attack_bonus"`
	// AttackMinBonus 记录攻击最小加成，使用数值表示。
	AttackMinBonus int64 `json:"attack_min_bonus"`
	// AttackMaxBonus 记录攻击最大加成，使用数值表示。
	AttackMaxBonus int64 `json:"attack_max_bonus"`
	// MagicMinBonus 记录魔法最小加成，使用数值表示。
	MagicMinBonus int64 `json:"magic_min_bonus"`
	// MagicMaxBonus 记录魔法最大加成，使用数值表示。
	MagicMaxBonus int64 `json:"magic_max_bonus"`
	// TaoMinBonus 记录“tao”最小加成，使用数值表示。
	TaoMinBonus int64 `json:"tao_min_bonus"`
	// TaoMaxBonus 记录“tao”最大加成，使用数值表示。
	TaoMaxBonus int64 `json:"tao_max_bonus"`
	// EquipmentStage 记录装备阶段，使用数值表示。
	EquipmentStage uint32 `json:"equipment_stage"`
	// OriginalDurabilityU 记录原始耐久u，使用数值表示。
	OriginalDurabilityU int64 `json:"original_durability_u,omitempty"`
	// RepairBasePrice 记录修理基础值“price”，使用数值表示。
	RepairBasePrice int64 `json:"repair_base_price,omitempty"`
	// AttackSpeedBonus 记录攻击速度加成，使用数值表示。
	AttackSpeedBonus int64 `json:"attack_speed_bonus"`
	// DefenceBonus 记录防御加成，使用数值表示。
	DefenceBonus int64 `json:"defence_bonus"`
	// RecycleGold 记录回收金币，使用数值表示。
	RecycleGold int64 `json:"recycle_gold"`
	// RequiredLevel 记录“required”等级，使用数值表示。
	RequiredLevel *uint32 `json:"required_level,omitempty"`
	// Stackable 标记是否处于“stackable”状态。
	Stackable bool `json:"stackable"`
	// AllowedJobs 记录允许任务或职业编号集合，使用数值列表表示。
	AllowedJobs []uint32 `json:"allowed_jobs"`
	// BookSkillID 标识当前业务使用的技能书技能。
	BookSkillID uint32 `json:"book_skill_id"`
	// IsSkillPage 标记是否处于“is”技能页面或分页状态。
	IsSkillPage bool `json:"is_skill_page"`
	// DropQuality 记录掉落品质，使用数值表示。
	DropQuality uint32 `json:"drop_quality"`
	// MaxStack 记录最大堆叠，使用数值表示。
	MaxStack int64 `json:"max_stack"`
	// DefenceMinBonus 记录防御最小加成，使用数值表示。
	DefenceMinBonus int64 `json:"defence_min_bonus"`
	// DefenceMaxBonus 记录防御最大加成，使用数值表示。
	DefenceMaxBonus int64 `json:"defence_max_bonus"`
	// MagicDefenceMinBonus 记录魔法防御最小加成，使用数值表示。
	MagicDefenceMinBonus int64 `json:"magic_defence_min_bonus"`
	// MagicDefenceMaxBonus 记录魔法防御最大加成，使用数值表示。
	MagicDefenceMaxBonus int64 `json:"magic_defence_max_bonus"`
	// recycleGoldExplicit 标记是否处于回收金币显式状态。
	recycleGoldExplicit bool
	// dropQualityExplicit 标记是否处于掉落品质显式状态。
	dropQualityExplicit bool
	// equipmentStatsExplicit 记录装备统计或属性显式，使用布尔状态列表表示。
	equipmentStatsExplicit [5]bool
}

// PotionEffect 定义单种药水每次使用后的恢复参数。
type PotionEffect struct {
	// ItemID 标识当前业务使用的道具。
	ItemID uint32 `json:"item_id"`
	// HP 记录生命值，使用数值表示。
	HP int64 `json:"hp"`
	// MP 记录魔法值，使用数值表示。
	MP int64 `json:"mp"`
}

// PotionRules 维护可识别药水及其使用节拍。
type PotionRules struct {
	// TickMS 记录模拟帧毫秒，使用数值表示。
	TickMS int64 `json:"tick_ms"`
	// BasePerTick 记录基础值“per”模拟帧，使用数值表示。
	BasePerTick int64 `json:"base_per_tick"`
	// LevelsPerStep 记录等级集合“per”步骤，使用数值表示。
	LevelsPerStep int64 `json:"levels_per_step"`
	// UseDelayMS 记录使用“delay”毫秒，使用数值表示。
	UseDelayMS int64 `json:"use_delay_ms"`
	// QueueCap 记录队列上限，使用数值表示。
	QueueCap int64 `json:"queue_cap"`
	// Effects 记录效果集合，使用药水效果列表表示。
	Effects []PotionEffect `json:"effects"`
}

// Effect 查询给定物品定义配置的药水效果。
// p 是调用该方法的药水规则集合实例，itemID 标识当前业务使用的道具。
func (p PotionRules) Effect(itemID uint32) (PotionEffect, bool) {
	// effect 接收遍历效果集合时当前的元素。
	for _, effect := range p.Effects {
		if effect.ItemID == itemID {
			return effect, true
		}
	}
	return PotionEffect{}, false
}

// Fixture 集中持有当前只读开发规则快照，供各玩法读取同一版本。
type Fixture struct {
	// MirrorArenaRelease 是显式选入的镜像赛季候选；缺省时入口关闭。
	MirrorArenaRelease *MirrorArenaDefinition `json:"mirror_arena,omitempty"`
	// MirrorArena 保存已装配的独立战斗地图和赛季规则，不加入普通换图目录。
	MirrorArena *MirrorArenaConfig `json:"-"`
	// DungeonReleases 是原玩法配置显式启用的讨伐链模板。缺失时不开放。
	DungeonReleases []DungeonDefinition `json:"dungeons,omitempty"`
	// Dungeons 保存通过装配校验的讨伐链快照，不加入普通地图目录。
	Dungeons []DungeonConfig `json:"-"`
	// WorldBoss 暂由隔离装配提供，经玩法源校验后才注册可发布模板。
	WorldBoss *WorldBossConfig `json:"-"`
	// WorldBossRelease 是配置工作簿中已批准且显式开放的共享首领候选。
	WorldBossRelease *WorldBossDefinition `json:"world_boss,omitempty"`
	// WorldBossVariantReleases 补充按非重叠时段投入同一运行服的世界首领。
	WorldBossVariantReleases []WorldBossDefinition `json:"world_boss_variants,omitempty"`
	// WorldBossVariants 是装载校验后的其他世界场次模板。
	WorldBossVariants []WorldBossConfig `json:"-"`
	// PersonalBossRelease 是唯一工作簿的显式开发候选定义。
	PersonalBossRelease *PersonalBossDefinition `json:"personal_boss,omitempty"`
	// PersonalBossVariantReleases 补充玩家可选的独立个人首领。
	PersonalBossVariantReleases []PersonalBossDefinition `json:"personal_boss_variants,omitempty"`
	// PersonalBossVariants 是装载检查后允许选择的个人挑战。
	PersonalBossVariants []PersonalBossConfig `json:"-"`
	// PersonalBoss 保存已由来源编号解析完成的挑战配置。
	PersonalBoss *PersonalBossConfig `json:"-"`
	// ChallengeMonsters 保存特殊挑战的怪物来源，不并入普通挂机的刷怪集合。
	ChallengeMonsters []Monster `json:"challenge_monsters,omitempty"`
	// TasksCodex 保存已经获得发布数据的任务和图鉴定义。
	TasksCodex *TaskCodexRules `json:"tasks_codex,omitempty"`
	// Warehouses 记录仓库集合，使用仓库规则集合表示。
	Warehouses *WarehouseRules `json:"warehouses,omitempty"`
	// SchemaVersion 记录“schema”版本，使用数值表示。
	SchemaVersion int `json:"schema_version"`
	// RuleVersion 记录规则版本，使用数值表示。
	RuleVersion uint64 `json:"rule_version"`
	// DevelopmentOnly 标记是否处于“development”“only”状态。
	DevelopmentOnly bool `json:"development_only"`
	// RuntimeReady 标记是否处于运行时就绪状态。
	RuntimeReady bool `json:"runtime_ready"`
	// SourceStatus 记录来源状态，使用文本表示。
	SourceStatus string `json:"source_status"`
	// DamageFormula 记录伤害“formula”，使用文本表示。
	DamageFormula string `json:"damage_formula"`
	// Map 记录地图，使用地图模板表示。
	Map MapTemplate `json:"map"`
	// Maps 记录地图集合，使用地图模板列表表示。
	Maps []MapTemplate `json:"maps,omitempty"`
	// Players 记录玩家集合，使用战斗单位列表表示。
	Players []Fighter `json:"players"`
	// Pet 记录宠物，使用战斗单位表示。
	Pet Fighter `json:"pet"`
	// PetRuntime 记录宠物运行时，使用宠物运行时规则集合表示。
	PetRuntime *PetRuntimeRules `json:"pet_runtime,omitempty"`
	// EquipmentRecycling 记录装备自动回收，使用装备自动回收规则集合表示。
	EquipmentRecycling *EquipmentRecyclingRules `json:"equipment_recycling,omitempty"`
	// EquipmentDurability 记录装备耐久，使用装备耐久规则集合表示。
	EquipmentDurability *EquipmentDurabilityRules `json:"equipment_durability,omitempty"`
	// Monsters 记录怪物集合，使用怪物列表表示。
	Monsters []Monster `json:"monsters"`
	// Items 记录道具集合，使用道具列表表示。
	Items []Item `json:"items"`
	// Potions 记录药水集合，使用药水规则集合表示。
	Potions PotionRules `json:"potions"`
	// Skills 记录技能集合，使用技能列表表示。
	Skills []Skill `json:"skills"`
	// Buffs 记录“buffs”，使用状态效果定义列表表示。
	Buffs []BuffDefinition `json:"buffs"`
	// CastingSupply 记录施法耗材，使用施法耗材规则集合表示。
	CastingSupply CastingSupplyRules `json:"casting_supply"`
	// Healing 记录治疗，使用治疗规则集合表示。
	Healing HealingRules `json:"healing"`
	// PlayerGrowth 记录玩家成长，使用玩家成长列表表示。
	PlayerGrowth []PlayerGrowth `json:"player_growth"`
	// InitialSkillBooks 记录初始值技能技能书集合，使用初始值技能技能书列表表示。
	InitialSkillBooks []InitialSkillBook `json:"initial_skill_books"`
	// SkillUpgrades 记录技能“upgrades”，使用技能升级列表表示。
	SkillUpgrades []SkillUpgrade `json:"skill_upgrades"`
	// SkillShop 记录技能商店，使用技能商店列表表示。
	SkillShop []SkillShop `json:"skill_shop"`
	// NPCShops 记录NPC商店集合，使用NPC商店列表表示。
	NPCShops []NPCShop `json:"npc_shops,omitempty"`
	// SkillBookRecycle 记录技能技能书回收，使用技能技能书回收列表表示。
	SkillBookRecycle []SkillBookRecycle `json:"skill_book_recycle"`
	// SkillEconomy 记录技能经济。
	SkillEconomy SkillEconomy `json:"skill_economy"`
	// CombatTiming 记录战斗时序。
	CombatTiming CombatTiming `json:"combat_timing"`
	// Binding 记录会话绑定，使用会话绑定规则集合表示。
	Binding *BindingRules `json:"binding,omitempty"`
	// TownTravel 记录城镇地图切换，使用城镇地图切换配置表示。
	TownTravel *TownTravelConfig `json:"town_travel,omitempty"`
	// TownNavigation 记录城镇导航，使用城镇导航配置表示。
	TownNavigation *TownNavigationConfig `json:"town_navigation,omitempty"`
	// Drops 记录掉落集合，使用临时结构表示。
	Drops struct {
		// EquipmentItemIDs 记录装备道具i“ds”，使用数值列表表示。
		EquipmentItemIDs []uint32 `json:"equipment_item_ids"`
		// ItemIDs 记录道具i“ds”，使用数值列表表示。
		ItemIDs []uint32 `json:"item_ids"`
		// EmptyPPM 记录空“ppm”，使用数值表示。
		EmptyPPM int `json:"empty_ppm"`
		// GoldPPM 记录金币“ppm”，使用数值表示。
		GoldPPM int `json:"gold_ppm"`
		// ItemPPM 记录道具“ppm”，使用数值表示。
		ItemPPM int `json:"item_ppm"`
		// EquipmentPPM 记录装备“ppm”，使用数值表示。
		EquipmentPPM int `json:"equipment_ppm"`
		// EquipmentGuarantee 记录装备“guarantee”，使用数值表示。
		EquipmentGuarantee int `json:"equipment_guarantee"`
	} `json:"drops"`
	// OrdinaryDropPools 收录可由普通怪物模板引用的每组掉落候选。
	OrdinaryDropPools []OrdinaryDropPool `json:"ordinary_drop_pools,omitempty"`
	// OrdinaryEquipmentQuotas 收录明确启用的普通怪稀有和史诗装备额度。
	OrdinaryEquipmentQuotas []OrdinaryEquipmentQuota `json:"drop_quotas,omitempty"`
	// HostingBenefits 包含当日领时、普通怪击杀加成及个人首领自动挑战权益。
	HostingBenefits *HostingBenefits `json:"hosting_benefits,omitempty"`
	// RoleCards 包含角色级非绑定元宝月卡和特权卡每日绑元领取。历史配置缺省关闭。
	RoleCards *RoleCardConfig `json:"role_cards,omitempty"`
	// ItemMarket 包含已装配的道具竞购与共享佣金规则。历史配置缺省关闭。
	ItemMarket *ItemMarketConfig `json:"item_market,omitempty"`
	// HostingTimePrices 仅包含显式发布的补时价格版本，缺省时关闭金币购时。
	HostingTimePrices []HostingTimePriceVersion `json:"hosting_time_prices,omitempty"`
	// Progression 记录成长，使用成长规则集合表示。
	Progression ProgressionRules `json:"progression"`
	// InitialInventory 记录初始值背包资产，使用数值列表表示。
	InitialInventory []uint32 `json:"initial_inventory"`
	// InitialBoundGold 记录初始值绑定金币，使用数值表示。
	InitialBoundGold int64 `json:"initial_bound_gold"`
	// InventoryCapacity 记录背包资产容量，使用数值表示。
	InventoryCapacity int `json:"inventory_capacity"`
	// AllowFreeDevRevive 标记是否处于“allow”免费或空闲“dev”复活状态。
	AllowFreeDevRevive bool `json:"allow_free_dev_revive"`
	// Hash 记录摘要，使用文本表示。
	Hash string `json:"-"`
}

// Config 保存在启动时完成校验的接入配置，以及随进程只读使用的玩法快照。
type Config struct {
	Environment        string            `json:"environment"`                     // 启动环境标识，当前只接受开发模式。
	AccountAddress     string            `json:"account_address"`                 // 账号节点的回环监听地址。
	GameAddress        string            `json:"game_address"`                    // 游戏节点的回环监听地址。
	GMAddress          string            `json:"gm_address"`                      // 开发管理节点的回环监听地址。
	MongoURI           string            `json:"mongo_uri"`                       // 专用副本集地址，平台覆盖后按锁定值核验。
	MongoURIByPlatform map[string]string `json:"mongo_uri_by_platform,omitempty"` // 针对不同操作系统选用的专用副本集地址。
	Database           string            `json:"database"`                        // 通过所有权检查的开发或隔离测试库名称。
	FixtureFile        string            `json:"fixture_file"`                    // 存放玩法规则快照的文件路径。
	NodeID             string            `json:"game_node_id"`                    // game 节点的稳定部署身份，兼容旧配置键 node_id。
	OriginID           uint64            `json:"origin_id"`                       // 试玩的入口服编号。
	RunID              uint64            `json:"run_id,omitempty"`                // 兼容旧单运行服配置；多运行服配置时保存首服编号。
	RunServerIDs       []uint64          `json:"run_server_ids,omitempty"`        // 本游戏进程启动后同时承载的逻辑运行服。
	OriginRoutes       []OriginRouteSeed `json:"origin_routes,omitempty"`         // 首次建库时登记的区服入口和路由。
	RunServers         []RunServerSeed   `json:"run_servers,omitempty"`           // 首次建库时登记的运行服与部署节点分配。
	AllowedOrigins     []string          `json:"allowed_origins"`                 // 配置文件保留的来源白名单字段，传输层另执行本机来源限制。
	SourcePath         string            `json:"-"`                               // 此次实际读取的启动配置绝对路径。
	Root               string            `json:"-"`                               // 解析相对玩法快照路径的仓库根目录。
	Fixture            Fixture           `json:"-"`                               // 已解析并校验的玩法和数值快照。
}

// DevelopmentMongoURI限定本机平台端点；Windows新副本集与原Lima端点分开。
func DevelopmentMongoURI() string {
	if runtime.GOOS == "windows" {
		return "mongodb://127.0.0.1:27119/?replicaSet=minimirWinDev&directConnection=true"
	}
	return "mongodb://127.0.0.1:27118/?replicaSet=minimirDev&directConnection=true"
}

// Load 在读取玩法之前验证节点和数据库边界，再保存快照的精确字节摘要。
// path 指向要读取的启动配置文件。
func Load(path string) (Config, error) {
	// c 记录配置。
	var c Config
	// absolute 保存由绝对值取得的“absolute”，e 接收绝对值供后续步骤使用。
	absolute, e := filepath.Abs(path)
	if e != nil {
		return c, e
	}
	// b 保存由读取文件取得的b。
	b, e := os.ReadFile(absolute)
	if e != nil {
		return c, e
	}
	if e = c.decodeNodeConfiguration(b); e != nil {
		return c, e
	}
	c.SourcePath = absolute
	c.Root = filepath.Clean(filepath.Join(filepath.Dir(absolute), "../.."))
	// 启动参数同时限定监听环境、MongoDB 副本集和库名，阻止误接其他数据。
	if c.Environment != "development" {
		return c, errors.New("当前首循环仅可显式development启动；生产尚未提供TLS/发行配置，拒绝启动")
	}
	// address 接收遍历文本列表时当前的元素。
	for _, address := range []string{c.AccountAddress, c.GameAddress, c.GMAddress} {
		// host 保存由拆分网络主机或托管端口取得的网络主机或托管，err 接收拆分网络主机或托管端口过程中可能产生的错误，供后续失败分支处理。
		host, _, err := net.SplitHostPort(address)
		if err != nil || host != "127.0.0.1" {
			return c, errors.New("development监听必须显式127.0.0.1")
		}
	}
	// platformURI 保存从MongoDB“uri”“by”“platform”取出的“platform”“uri”，ok 表明映射中能否取得指定键。
	if platformURI, ok := c.MongoURIByPlatform[runtime.GOOS]; ok {
		c.MongoURI = platformURI
	}
	if c.MongoURI != DevelopmentMongoURI() {
		return c, errors.New("仅允许当前平台本任务独立loopback Mongo副本集")
	}
	if c.Database != "minimir_dev_game" && !strings.HasPrefix(c.Database, "minimir_dev_game_test_") {
		return c, errors.New("必须使用专用开发游戏数据库")
	}
	if e = c.validateTopology(); e != nil {
		return c, e
	}
	// fixturePath 保存开发测试配置文件，用于当前步骤中的开发测试配置路径。
	fixturePath := c.FixtureFile
	if !filepath.IsAbs(fixturePath) {
		fixturePath = filepath.Join(c.Root, fixturePath)
	}
	b, e = os.ReadFile(fixturePath)
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c.Fixture); e != nil {
		return c, e
	}
	// hash 保存由累加值“256”取得的摘要。
	hash := sha256.Sum256(b)
	c.Fixture.Hash = hex.EncodeToString(hash[:])
	return c, c.Fixture.Validate()
}

// Validate 验证玩法快照的来源状态、规则版本和各模块之间的引用。
// f 是调用该方法的开发测试配置实例。
func (f *Fixture) Validate() error {
	if f.SchemaVersion != 1 || !f.DevelopmentOnly || f.RuntimeReady || f.RuleVersion == 0 || f.SourceStatus != "candidate" || (f.DamageFormula != "development_range_minus_defence" && f.DamageFormula != "development_source_combat") {
		return errors.New("必须显式启用有来源的candidate开发fixture；不冒充1.80正式数值")
	}
	// err 接收校验地图过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateMap(); err != nil {
		return err
	}
	if err := f.validatePersonalBossRelease(); err != nil {
		return err
	}
	if err := f.validateWorldBossRelease(); err != nil {
		return err
	}
	if err := f.validateDungeonReleases(); err != nil {
		return err
	}
	if err := f.ValidateMirrorArena(); err != nil {
		return err
	}
	// err 接收校验会话绑定过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateBinding(); err != nil {
		return err
	}
	// err 接收校验城镇“configuration”过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateTownConfiguration(); err != nil {
		return err
	}
	// err 接收校验NPC商店集合过程中可能产生的错误，供后续失败分支处理。
	if err := f.validateNPCShops(); err != nil {
		return err
	}
	// err 接收校验瞬移过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateTeleport(); err != nil {
		return err
	}
	// err 接收校验宠物运行时过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidatePetRuntime(); err != nil {
		return err
	}
	// err 接收校验“summoning”过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateSummoning(); err != nil {
		return err
	}
	// err 接收校验装备自动回收过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateEquipmentRecycling(); err != nil {
		return err
	}
	// err 接收校验成长过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateProgression(); err != nil {
		return err
	}
	// err 接收校验装备统计或属性过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateEquipmentStats(); err != nil {
		return err
	}
	// err 接收校验装备耐久过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateEquipmentDurability(); err != nil {
		return err
	}
	// err 接收校验仓库集合过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateWarehouses(); err != nil {
		return err
	}
	if err := f.ValidateTasksCodex(); err != nil {
		return err
	}
	// err 接收校验位移“foundation”过程中可能产生的错误，供后续失败分支处理。
	if err := f.ValidateDisplacementFoundation(); err != nil {
		return err
	}
	if f.InventoryCapacity < 1 || f.InventoryCapacity > 100 || f.InitialBoundGold < 0 {
		return errors.New("开发背包容量/初始资金不合法")
	}
	if len(f.Players) == 0 || len(f.Monsters) == 0 || len(f.Items) == 0 {
		return errors.New("职业、怪物、物品不能为空")
	}
	// valid 封装有效对应的后续处理，a 记录a，使用战斗单位表示。
	valid := func(a Fighter) bool {
		return a.HP > 0 && a.HP < 1000000 && a.AttackMin >= 0 && a.AttackMax >= a.AttackMin && a.AttackMax < 100000 && a.Defence >= 0 && a.MoveMS >= 100 && a.AttackMS >= 100
	}
	// a 接收遍历玩家集合时当前的元素。
	for _, a := range f.Players {
		if a.JobID > 2 || !valid(a) {
			return errors.New("职业fixture参数缺失")
		}
	}
	if !valid(f.Pet) {
		return errors.New("宠物fixture缺失")
	}
	// 模板编号在目录内唯一。各地图出生槽可以反复引用同一明确模板。
	seenMonsters := make(map[uint32]bool, len(f.Monsters))
	seenAppearances := make(map[uint32]bool, len(f.Monsters))
	for _, m := range f.Monsters {
		if !valid(m.Fighter) || m.ID == 0 || seenMonsters[m.ID] || m.Exp < 0 || m.GoldMin < 0 || m.GoldMax < m.GoldMin || m.RespawnMS != 10000 {
			return errors.New("怪物参数须明确，普通怪独立10秒复活")
		}
		seenMonsters[m.ID] = true
		if f.OrdinaryMonsterRosterEnabled() {
			if m.DefinitionID == 0 || seenAppearances[m.DefinitionID] {
				return errors.New("普通怪物须有明确且唯一的客户端定义编号")
			}
			seenAppearances[m.DefinitionID] = true
		}
	}
	// i 接收遍历道具集合时当前的元素。
	for _, i := range f.Items {
		if i.ID == 0 || i.RecycleGold < 0 || i.EquipmentSlot > 9 && !(i.EquipmentSlot == CastingSupplySlot && f.IsCastingSupply(i.ID)) {
			return errors.New("物品参数不合法")
		}
	}
	// p 保存当前处理所用的药水集合。
	p := f.Potions
	// maxMS 保存根据整数“64”与整数“64”计算的最大毫秒。
	maxMS := int64(math.MaxInt64) / int64(time.Millisecond)
	if p.TickMS <= 0 || p.TickMS > maxMS || p.UseDelayMS <= 0 || p.UseDelayMS > maxMS || p.BasePerTick <= 0 || p.LevelsPerStep <= 0 || p.QueueCap <= 0 || len(p.Effects) == 0 {
		return errors.New("药品恢复周期、节流、等级步长和队列容量必须完整且可计时")
	}
	// seenPotions 建立带初始值的按数值索引的布尔状态映射，供本段继续处理。
	seenPotions := map[uint32]bool{}
	// effect 接收遍历效果集合时当前的元素。
	for _, effect := range p.Effects {
		// item 接收道具供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
		item, ok := f.Item(effect.ItemID)
		if !ok || item.IsEquipment || seenPotions[effect.ItemID] || effect.HP < 0 || effect.MP < 0 || (effect.HP == 0 && effect.MP == 0) {
			return errors.New("药品效果必须引用唯一的非装备物品，恢复量为非负且至少一项有效")
		}
		seenPotions[effect.ItemID] = true
	}
	if f.Drops.EmptyPPM != 650000 || f.Drops.GoldPPM != 200000 || f.Drops.ItemPPM != 100000 || f.Drops.EquipmentPPM != 50000 || f.Drops.EquipmentGuarantee != 20 || len(f.Drops.EquipmentItemIDs) == 0 {
		return errors.New("普通掉落须互斥65/20/10/5与20杀保底")
	}
	if err := f.validateOrdinaryDropPools(); err != nil {
		return err
	}
	if err := f.validateHostingBenefits(); err != nil {
		return err
	}
	if err := f.validateRoleCards(); err != nil {
		return err
	}
	if err := f.validateItemMarket(); err != nil {
		return err
	}
	if err := f.validateHostingTimePrices(); err != nil {
		return err
	}
	// id 接收遍历“append”时当前的元素。
	for _, id := range append(append([]uint32{}, f.Drops.EquipmentItemIDs...), f.InitialInventory...) {
		// ok 表明查询、校验或类型转换是否成功。
		if _, ok := f.Item(id); !ok {
			return fmt.Errorf("物品引用%d不存在", id)
		}
	}
	if f.DamageFormula == "development_source_combat" {
		return f.ValidateSkills()
	}
	return nil
}

// Player 返回职业编号对应的角色战斗配置。
// f 是调用该方法的开发测试配置实例，job 选择要查询的战士、法师或道士配置。
func (f *Fixture) Player(job uint32) (Fighter, bool) {
	// p 接收遍历玩家集合时当前的元素。
	for _, p := range f.Players {
		if p.JobID == job {
			return p, true
		}
	}
	return Fighter{}, false
}

// Item 按物品定义号查找配置模板。
// f 是调用该方法的开发测试配置实例，id 标识待查询的物品定义。
func (f *Fixture) Item(id uint32) (Item, bool) {
	// i 接收遍历道具集合时当前的元素。
	for _, i := range f.Items {
		if i.ID == id {
			return i, true
		}
	}
	return Item{}, false
}
