//go:build m4_economy_sampling

// 本文件展开普通挂机经济样本。在隔离真实联机环境中按同一时长与配置运行所选组合。
package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
)

// economyKits 指定当前规则已经具备的三个对照配装，装备实际属性全部来自夹具。
var economyKits = map[string][]uint32{
	"low":  {218, 193},
	"mid":  {223, 193, 259},
	"high": {225, 195, 261},
}

// economyCardProducts 固定配组所使用的真实月卡商品，费用和有效期从原配置取得。
var economyCardProducts = map[string][]uint32{
	"none": {}, "privilege": {7}, "hosting": {8}, "both": {7, 8},
}

// economySelection 是隔离启动工具传入的实验条件。所有计时以真实服务端同机时钟推进。
type economySelection struct {
	Level       uint32   `json:"level"`       // 固定同档起点，让配装比较不混入不同初始等级。
	Seconds     int64    `json:"seconds"`     // 单样本实际墙钟时长。
	MapID       uint64   `json:"map_id"`      // 当前规则的目标个人挂机实例模板。
	Mode        string   `json:"mode"`        // 在线自动战斗或真正断开后的离线托管。
	Jobs        []uint32 `json:"jobs"`        // 职业选取。
	Monsters    []uint32 `json:"monsters"`    // 每个样本的活动怪槽数。
	Kits        []string `json:"kits"`        // 使用原装备模板的分组。
	Cards       []string `json:"cards"`       // 双卡组合。
	Repeat      uint32   `json:"repeat"`      // 每个组合的独立角色样本数。
	Maintenance bool     `json:"maintenance"` // 收窗后是否去实际 NPC 补齐药量及修理。
}

// economyScenario 固定一个样本的所有维度，输出报告保留该身份。
type economyScenario struct {
	Job     uint32 `json:"job"`     // 普通职业配置号。
	Level   uint32 `json:"level"`   // 本样本设定的起始等级。
	Slots   uint32 `json:"slots"`   // 经首基线确认的怪槽数。
	Kit     string `json:"kit"`     // 对照组装备标识。
	Cards   string `json:"cards"`   // 角色实际生效的月卡组合。
	Repeat  uint32 `json:"repeat"`  // 独立角色样本序号。
	MapID   uint64 `json:"map_id"`  // 个人场景模板。
	Mode    string `json:"mode"`    // 挂机执行方式。
	Seconds int64  `json:"seconds"` // 单样本墙钟时长。
}

func (s economyScenario) key() string {
	return fmt.Sprintf("j%d-m%d-%s-%s-r%d", s.Job, s.Slots, s.Kit, s.Cards, s.Repeat)
}

// TestOrdinaryEconomyMatrixMongoWS 只由显式隔离启动工具执行，常规回归不会启动长窗。
func TestOrdinaryEconomyMatrixMongoWS(t *testing.T) {
	cfg := m1FieldConfig(t)
	if filename := os.Getenv("MINIMIR_ECONOMY_FIXTURE"); filename != "" {
		raw, err := os.ReadFile(filename)
		must(t, err)
		var fixture devconfig.Fixture
		must(t, json.Unmarshal(raw, &fixture))
		must(t, fixture.Validate())
		fixture.Hash = persistence.Hash(raw)
		cfg.Fixture, cfg.FixtureFile = fixture, filename
	}
	var selection economySelection
	must(t, json.Unmarshal([]byte(os.Getenv("MINIMIR_ECONOMY_SELECTION")), &selection))
	scenarios := selection.scenarios(t, &cfg.Fixture)
	directory := os.Getenv("MINIMIR_ECONOMY_SAMPLES")
	if directory == "" || !filepath.IsAbs(directory) {
		t.Fatal("缺少本次隔离运行建立的样本目录")
	}
	cfg, ctx, store := m1FieldServices(t, cfg)
	must(t, prepareEconomyIndexes(ctx, store))
	shopAccess := make(chan struct{}, 1)  // 同一紧凑城镇的测试角色错开修理路径。
	loginAccess := make(chan struct{}, 4) // 遵守账号服现有的本地密码派生并发槽。
	for _, scenario := range scenarios {
		t.Run(scenario.key(), func(t *testing.T) {
			t.Parallel()
			profile := prepareEconomyActor(t, ctx, cfg, store, scenario, loginAccess)
			report := profile.observe(time.Duration(scenario.Seconds)*time.Second, selection.Maintenance, shopAccess)
			raw, err := json.MarshalIndent(report, "", "  ")
			must(t, err)
			name := filepath.Join(directory, scenario.key()+".json")
			// 一份成功复核的样本独立落盘，后续样本失败仍可复盘已经完成的原证据。
			must(t, os.WriteFile(name+".partial", append(raw, '\n'), 0600))
			must(t, os.Rename(name+".partial", name))
			t.Logf("实际采样结束 %s，击杀 %d，原收益回执 %d", scenario.key(), report.Gross.Kills, report.Sources)
		})
	}
}

// scenarios 校验每个输入选项及目录真实可用性，然后按所选维度完整展开。
func (selection economySelection) scenarios(t *testing.T, f *devconfig.Fixture) []economyScenario {
	t.Helper()
	if selection.Seconds < 30 || selection.Seconds > 24*3600 || selection.Repeat < 1 || selection.Repeat > 3 ||
		selection.Level < 7 || selection.Level > 50 ||
		(selection.Mode != "online" && selection.Mode != "hosting") {
		t.Fatal("隔离采样时长、重复次数或模式不合法", selection)
	}
	if !f.OrdinaryMonsterRosterEnabled() || f.HostingBenefits == nil || f.RoleCards == nil || f.EquipmentDurability == nil {
		t.Fatal("采样要求当前普通怪、角色双卡和耐久的可运行配置")
	}
	world, found := f.MapByID(selection.MapID)
	if !found || world.SceneKind != devconfig.SceneKindPersonal || world.Width != 13 || world.Height != 13 || len(world.MonsterSpawnIDs) < 8 {
		t.Fatal("采样目标不是可比较的个人 13×13 八怪地图", selection.MapID)
	}
	var scenarios []economyScenario
	seen := make(map[string]bool)
	for _, job := range selection.Jobs {
		for _, count := range selection.Monsters {
			for _, kit := range selection.Kits {
				validateEconomyKit(t, f, job, selection.Level, kit)
				for _, cards := range selection.Cards {
					if _, ok := economyCardProducts[cards]; !ok || !slices.Contains([]uint32{1, 4, 8}, count) || job > 2 {
						t.Fatal("采样职业、怪槽或卡状态不在已登记组合内")
					}
					for repeat := uint32(1); repeat <= selection.Repeat; repeat++ {
						scenario := economyScenario{job, selection.Level, count, kit, cards, repeat, selection.MapID, selection.Mode, selection.Seconds}
						if seen[scenario.key()] {
							t.Fatal("采样组合重复", scenario.key())
						}
						seen[scenario.key()] = true
						scenarios = append(scenarios, scenario)
					}
				}
			}
		}
	}
	if len(scenarios) == 0 {
		t.Fatal("没有要运行的实际样本")
	}
	return scenarios
}

// validateEconomyKit 只允许同一等级和原职业真正能穿的装备，不覆盖原模板属性。
func validateEconomyKit(t *testing.T, f *devconfig.Fixture, job, level uint32, kit string) {
	t.Helper()
	ids, known := economyKits[kit]
	if !known {
		t.Fatal("未知配装档", kit)
	}
	occupied := map[uint32]bool{}
	for _, id := range ids {
		definition, found := f.Item(id)
		if !found || !definition.IsEquipment || definition.OriginalDurabilityU <= 0 ||
			definition.RequiredLevel == nil || *definition.RequiredLevel > level ||
			!slices.Contains(definition.AllowedJobs, job) || occupied[definition.EquipmentSlot] {
			t.Fatal("配装不满足原职业、需求等级、耐久或槽位条件", kit, id, job)
		}
		occupied[definition.EquipmentSlot] = true
	}
}
