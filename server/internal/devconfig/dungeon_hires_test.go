// 本文件验证 AI 报价、原职业技能范围和运行态快照的装配边界。
package devconfig

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestDungeonHireOptionsValidateAndStayFrozen(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	fixture := &cfg.Fixture
	definition := dungeonDefinitionForTest(fixture.ChallengeMonsters[0].ID)
	quotedGold := int64(1<<53 + 1) // 隔离用例专测高精度，不作为价格建议。
	definition.Hires = []DungeonHireOption{{ID: 10, DisplayName: "隔离法师", Job: 1, Level: 30,
		FeeGold: quotedGold, Skills: []DungeonHireSkill{{ID: 1, Rank: 0}}}}
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	var decoded DungeonDefinition
	if err = json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	fixture.DungeonReleases = []DungeonDefinition{decoded}
	if err = fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	template, found := fixture.DungeonTemplateByID(definition.TemplateID)
	if !found {
		t.Fatal("合法候选未装入")
	}
	fixture.DungeonReleases[0].Hires[0].FeeGold = 0
	fixture.DungeonReleases[0].Hires[0].Skills[0].Rank = 2
	option, found := template.HireOption(10)
	if !found || option.FeeGold != quotedGold || option.Skills[0].Rank != 0 {
		t.Fatal("装配未保持原准确报价及技能", option)
	}
}

func TestDungeonHireOptionsRejectInvalidSources(t *testing.T) {
	cfg, err := Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	base := DungeonHireOption{ID: 10, DisplayName: "隔离法师", Job: 1, Level: 30, FeeGold: 14,
		Skills: []DungeonHireSkill{{ID: 1, Rank: 0}}}
	for name, edit := range map[string]func(*DungeonHireOption){
		"零编号":     func(o *DungeonHireOption) { o.ID = 0 },
		"空名称":     func(o *DungeonHireOption) { o.DisplayName = " " },
		"名称过长":    func(o *DungeonHireOption) { o.DisplayName = strings.Repeat("法", 30) },
		"错误职业技能":  func(o *DungeonHireOption) { o.Job = 0 },
		"非法职业":    func(o *DungeonHireOption) { o.Job = 3 },
		"不满足学习等级": func(o *DungeonHireOption) { o.Level = 1 },
		"等级溢出":    func(o *DungeonHireOption) { o.Level = math.MaxUint32 },
		"负价格":     func(o *DungeonHireOption) { o.FeeGold = -1 },
		"两席合计溢出":  func(o *DungeonHireOption) { o.FeeGold = math.MaxInt64 },
		"超阶":      func(o *DungeonHireOption) { o.Skills = []DungeonHireSkill{{ID: 1, Rank: 255}} },
		"重复技能":    func(o *DungeonHireOption) { o.Skills = append(o.Skills, o.Skills[0]) },
		"场内位移":    func(o *DungeonHireOption) { o.Skills = []DungeonHireSkill{{ID: 8, Rank: 0}} },
	} {
		t.Run(name, func(t *testing.T) {
			option := base
			edit(&option)
			if _, valid := cfg.Fixture.cloneDungeonHireOptions([]DungeonHireOption{option}); valid {
				t.Fatal("无效选项通过了装配", option)
			}
		})
	}
	if _, valid := cfg.Fixture.cloneDungeonHireOptions([]DungeonHireOption{base, base}); valid {
		t.Fatal("同一模板不可使用重复报价编号")
	}
}
