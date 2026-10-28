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

type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type Fighter struct {
	CanPush           bool   `json:"can_push"`
	Level             uint32 `json:"level"`
	JobID             uint32 `json:"job_id"`
	DefinitionID      uint32 `json:"definition_id"`
	HP                int64  `json:"hp"`
	MP                int64  `json:"mp"`
	AttackMin         int64  `json:"attack_min"`
	AttackMax         int64  `json:"attack_max"`
	Defence           int64  `json:"defence"`
	AttackMS          int64  `json:"attack_ms"`
	AttackSpeed       int64  `json:"attack_speed"`
	MoveMS            int64  `json:"move_ms"`
	MagicMin          int64  `json:"magic_min"`
	MagicMax          int64  `json:"magic_max"`
	TaoMin            int64  `json:"tao_min"`
	TaoMax            int64  `json:"tao_max"`
	DefenceMin        int64  `json:"defence_min"`
	DefenceMax        int64  `json:"defence_max"`
	MagicDefenceMin   int64  `json:"magic_defence_min"`
	MagicDefenceMax   int64  `json:"magic_defence_max"`
	Accuracy          int64  `json:"accuracy"`
	Agility           int64  `json:"agility"`
	MagicResist       int64  `json:"magic_resist"`
	MagicResistWeight int64  `json:"magic_resist_weight"`
	canPushExplicit   bool
}
type Monster struct {
	Fighter
	ID        uint32 `json:"id"`
	Exp       int64  `json:"exp"`
	GoldMin   int64  `json:"gold_min"`
	GoldMax   int64  `json:"gold_max"`
	RespawnMS int64  `json:"respawn_ms"`
	Undead    bool   `json:"undead"`
}
type Item struct {
	ID                   uint32   `json:"id"`
	Name                 string   `json:"name"`
	EquipmentSlot        uint32   `json:"equipment_slot"`
	IsEquipment          bool     `json:"is_equipment"`
	AttackBonus          int64    `json:"attack_bonus"`
	AttackMinBonus       int64    `json:"attack_min_bonus"`
	AttackMaxBonus       int64    `json:"attack_max_bonus"`
	AttackSpeedBonus     int64    `json:"attack_speed_bonus"`
	DefenceBonus         int64    `json:"defence_bonus"`
	RecycleGold          int64    `json:"recycle_gold"`
	Stackable            bool     `json:"stackable"`
	AllowedJobs          []uint32 `json:"allowed_jobs"`
	BookSkillID          uint32   `json:"book_skill_id"`
	IsSkillPage          bool     `json:"is_skill_page"`
	DropQuality          uint32   `json:"drop_quality"`
	MaxStack             int64    `json:"max_stack"`
	DefenceMinBonus      int64    `json:"defence_min_bonus"`
	DefenceMaxBonus      int64    `json:"defence_max_bonus"`
	MagicDefenceMinBonus int64    `json:"magic_defence_min_bonus"`
	MagicDefenceMaxBonus int64    `json:"magic_defence_max_bonus"`
}
type PotionEffect struct {
	ItemID uint32 `json:"item_id"`
	HP     int64  `json:"hp"`
	MP     int64  `json:"mp"`
}
type PotionRules struct {
	TickMS        int64          `json:"tick_ms"`
	BasePerTick   int64          `json:"base_per_tick"`
	LevelsPerStep int64          `json:"levels_per_step"`
	UseDelayMS    int64          `json:"use_delay_ms"`
	QueueCap      int64          `json:"queue_cap"`
	Effects       []PotionEffect `json:"effects"`
}

func (p PotionRules) Effect(itemID uint32) (PotionEffect, bool) {
	for _, effect := range p.Effects {
		if effect.ItemID == itemID {
			return effect, true
		}
	}
	return PotionEffect{}, false
}

type Fixture struct {
	SchemaVersion     int                `json:"schema_version"`
	RuleVersion       uint64             `json:"rule_version"`
	DevelopmentOnly   bool               `json:"development_only"`
	RuntimeReady      bool               `json:"runtime_ready"`
	SourceStatus      string             `json:"source_status"`
	DamageFormula     string             `json:"damage_formula"`
	Map               MapTemplate        `json:"map"`
	Maps              []MapTemplate      `json:"maps,omitempty"`
	Players           []Fighter          `json:"players"`
	Pet               Fighter            `json:"pet"`
	Monsters          []Monster          `json:"monsters"`
	Items             []Item             `json:"items"`
	Potions           PotionRules        `json:"potions"`
	Skills            []Skill            `json:"skills"`
	Buffs             []BuffDefinition   `json:"buffs"`
	CastingSupply     CastingSupplyRules `json:"casting_supply"`
	Healing           HealingRules       `json:"healing"`
	PlayerGrowth      []PlayerGrowth     `json:"player_growth"`
	InitialSkillBooks []InitialSkillBook `json:"initial_skill_books"`
	SkillUpgrades     []SkillUpgrade     `json:"skill_upgrades"`
	SkillShop         []SkillShop        `json:"skill_shop"`
	SkillBookRecycle  []SkillBookRecycle `json:"skill_book_recycle"`
	SkillEconomy      SkillEconomy       `json:"skill_economy"`
	CombatTiming      CombatTiming       `json:"combat_timing"`
	Binding           *BindingRules      `json:"binding,omitempty"`
	Drops             struct {
		EquipmentItemIDs   []uint32 `json:"equipment_item_ids"`
		ItemIDs            []uint32 `json:"item_ids"`
		EmptyPPM           int      `json:"empty_ppm"`
		GoldPPM            int      `json:"gold_ppm"`
		ItemPPM            int      `json:"item_ppm"`
		EquipmentPPM       int      `json:"equipment_ppm"`
		EquipmentGuarantee int      `json:"equipment_guarantee"`
	} `json:"drops"`
	Progression struct {
		ExpPerLevel []int64 `json:"exp_per_level"`
	} `json:"progression"`
	InitialInventory   []uint32 `json:"initial_inventory"`
	InitialBoundGold   int64    `json:"initial_bound_gold"`
	InventoryCapacity  int      `json:"inventory_capacity"`
	AllowFreeDevRevive bool     `json:"allow_free_dev_revive"`
	Hash               string   `json:"-"`
}
type Config struct {
	Environment        string            `json:"environment"`
	AccountAddress     string            `json:"account_address"`
	GameAddress        string            `json:"game_address"`
	GMAddress          string            `json:"gm_address"`
	MongoURI           string            `json:"mongo_uri"`
	MongoURIByPlatform map[string]string `json:"mongo_uri_by_platform,omitempty"`
	Database           string            `json:"database"`
	FixtureFile        string            `json:"fixture_file"`
	NodeID             string            `json:"node_id"`
	OriginID           uint64            `json:"origin_id"`
	RunID              uint64            `json:"run_id"`
	AllowedOrigins     []string          `json:"allowed_origins"`
	Root               string            `json:"-"`
	Fixture            Fixture           `json:"-"`
}

// DevelopmentMongoURI限定本机平台端点；Windows新副本集与原Lima端点分开。
func DevelopmentMongoURI() string {
	if runtime.GOOS == "windows" {
		return "mongodb://127.0.0.1:27119/?replicaSet=minimirWinDev&directConnection=true"
	}
	return "mongodb://127.0.0.1:27118/?replicaSet=minimirDev&directConnection=true"
}

func Load(path string) (Config, error) {
	var c Config
	absolute, e := filepath.Abs(path)
	if e != nil {
		return c, e
	}
	b, e := os.ReadFile(absolute)
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	c.Root = filepath.Clean(filepath.Join(filepath.Dir(absolute), "../.."))
	if c.Environment != "development" {
		return c, errors.New("当前首循环仅可显式development启动；生产尚未提供TLS/发行配置，拒绝启动")
	}
	for _, address := range []string{c.AccountAddress, c.GameAddress, c.GMAddress} {
		host, _, err := net.SplitHostPort(address)
		if err != nil || host != "127.0.0.1" {
			return c, errors.New("development监听必须显式127.0.0.1")
		}
	}
	if platformURI, ok := c.MongoURIByPlatform[runtime.GOOS]; ok {
		c.MongoURI = platformURI
	}
	if c.MongoURI != DevelopmentMongoURI() {
		return c, errors.New("仅允许当前平台本任务独立loopback Mongo副本集")
	}
	if c.Database != "minimir_dev_game" && !strings.HasPrefix(c.Database, "minimir_dev_game_test_") {
		return c, errors.New("必须使用专用开发游戏数据库")
	}
	if c.NodeID == "" || c.OriginID == 0 || c.RunID == 0 {
		return c, errors.New("节点与路由ID缺失")
	}
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
	hash := sha256.Sum256(b)
	c.Fixture.Hash = hex.EncodeToString(hash[:])
	return c, c.Fixture.Validate()
}
func (f *Fixture) Validate() error {
	if f.SchemaVersion != 1 || !f.DevelopmentOnly || f.RuntimeReady || f.RuleVersion == 0 || f.SourceStatus != "candidate" || (f.DamageFormula != "development_range_minus_defence" && f.DamageFormula != "development_source_combat") {
		return errors.New("必须显式启用有来源的candidate开发fixture；不冒充1.80正式数值")
	}
	if err := f.ValidateMap(); err != nil {
		return err
	}
	if err := f.ValidateBinding(); err != nil {
		return err
	}
	if err := f.ValidateDisplacementFoundation(); err != nil {
		return err
	}
	if f.InventoryCapacity < 1 || f.InventoryCapacity > 100 || f.InitialBoundGold < 0 {
		return errors.New("开发背包容量/初始资金不合法")
	}
	if len(f.Players) == 0 || len(f.Monsters) == 0 || len(f.Items) == 0 {
		return errors.New("职业、怪物、物品不能为空")
	}
	valid := func(a Fighter) bool {
		return a.HP > 0 && a.HP < 1000000 && a.AttackMin >= 0 && a.AttackMax >= a.AttackMin && a.AttackMax < 100000 && a.Defence >= 0 && a.MoveMS >= 100 && a.AttackMS >= 100
	}
	for _, a := range f.Players {
		if a.JobID > 2 || !valid(a) {
			return errors.New("职业fixture参数缺失")
		}
	}
	if !valid(f.Pet) {
		return errors.New("宠物fixture缺失")
	}
	for _, m := range f.Monsters {
		if !valid(m.Fighter) || m.ID == 0 || m.Exp < 0 || m.GoldMin < 0 || m.GoldMax < m.GoldMin || m.RespawnMS != 10000 {
			return errors.New("怪物参数须明确，普通怪独立10秒复活")
		}
	}
	for _, i := range f.Items {
		if i.ID == 0 || i.RecycleGold < 0 || i.EquipmentSlot > 9 && !(i.EquipmentSlot == CastingSupplySlot && f.IsCastingSupply(i.ID)) {
			return errors.New("物品参数不合法")
		}
	}
	p := f.Potions
	maxMS := int64(math.MaxInt64) / int64(time.Millisecond)
	if p.TickMS <= 0 || p.TickMS > maxMS || p.UseDelayMS <= 0 || p.UseDelayMS > maxMS || p.BasePerTick <= 0 || p.LevelsPerStep <= 0 || p.QueueCap <= 0 || len(p.Effects) == 0 {
		return errors.New("药品恢复周期、节流、等级步长和队列容量必须完整且可计时")
	}
	seenPotions := map[uint32]bool{}
	for _, effect := range p.Effects {
		item, ok := f.Item(effect.ItemID)
		if !ok || item.IsEquipment || seenPotions[effect.ItemID] || effect.HP < 0 || effect.MP < 0 || (effect.HP == 0 && effect.MP == 0) {
			return errors.New("药品效果必须引用唯一的非装备物品，恢复量为非负且至少一项有效")
		}
		seenPotions[effect.ItemID] = true
	}
	if f.Drops.EmptyPPM != 650000 || f.Drops.GoldPPM != 200000 || f.Drops.ItemPPM != 100000 || f.Drops.EquipmentPPM != 50000 || f.Drops.EquipmentGuarantee != 20 || len(f.Drops.EquipmentItemIDs) == 0 {
		return errors.New("普通掉落须互斥65/20/10/5与20杀保底")
	}
	for _, id := range append(append([]uint32{}, f.Drops.EquipmentItemIDs...), f.InitialInventory...) {
		if _, ok := f.Item(id); !ok {
			return fmt.Errorf("物品引用%d不存在", id)
		}
	}
	if f.DamageFormula == "development_source_combat" {
		return f.ValidateSkills()
	}
	return nil
}
func (f *Fixture) Player(job uint32) (Fighter, bool) {
	for _, p := range f.Players {
		if p.JobID == job {
			return p, true
		}
	}
	return Fighter{}, false
}
func (f *Fixture) Item(id uint32) (Item, bool) {
	for _, i := range f.Items {
		if i.ID == id {
			return i, true
		}
	}
	return Item{}, false
}
