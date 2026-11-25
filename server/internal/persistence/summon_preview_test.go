// 验证召唤类技能的材料报价及规则前置，覆盖不同召唤技能的开放和消耗边界。

package persistence

import (
	"minimir/internal/devconfig"
	"reflect"
	"testing"
)

// TestSummonUpgradePreviewDescribesNewBirthInsteadOfDamage 核对「召唤预览含无效通用伤害或遗漏新生限定」与「阶新生字段未反映来源投影」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSummonUpgradePreviewDescribesNewBirthInsteadOfDamage(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// c 接收加载召唤候选集合供后续步骤使用。
	c, err := devconfig.LoadSummonCandidates("../../../config/development/summon-candidates.json")
	if err != nil {
		t.Fatal(err)
	}
	// f 保存当前处理所用的开发测试配置。
	f := cfg.Fixture
	if _, active := f.Skill(devconfig.SummonSkeletonSkillID); !active {
		f.Skills, f.Items, f.SkillUpgrades = append(f.Skills, c.Skills...), append(f.Items, c.Books...), append(f.SkillUpgrades, c.Upgrades...)
	}
	// err 接收校验过程中可能产生的错误，供后续失败分支处理。
	if err := f.Validate(); err != nil {
		t.Fatal(err)
	}
	// d 接收定义“by”标识供后续步骤使用，found 保存由定义“by”标识取得的已找到标记。
	d, found := f.PetRuntime.DefinitionByID(c.PetDefinitionIDs[0])
	if !found {
		t.Fatal("没有候选宠物")
	}
	// rank 保存由无符号整数“32”取得的排序位次。
	for rank := uint32(0); rank < 3; rank++ {
		// preview 保存由协议取得的预览。
		preview := (skillQuote{Plan: skillPlan{Kind: 2, SkillID: 17, From: rank, To: rank + 1}}).Proto(f)
		// keys 初始化用作键集合的文本列表。
		keys := []string{}
		// s 接收遍历统计或属性时当前的元素。
		for _, s := range preview.Stats {
			keys = append(keys, s.Key)
		}
		if !reflect.DeepEqual(keys, []string{"mp_cost", "new_pet_training_level", "new_pet_max_training_level", "new_pet_material_quantity", "new_pet_hp_max"}) {
			t.Fatal("召唤预览含无效通用伤害或遗漏新生限定", keys)
		}
		// wantBefore 初始化用作期望值操作前的状态的数值列表。
		wantBefore := []int64{12 + int64(rank)*4, int64(rank), 4 + int64(rank), 1, d.Base.HP + int64(rank)*20}
		// wantAfter 初始化用作期望值操作后的状态的数值列表。
		wantAfter := []int64{16 + int64(rank)*4, int64(rank) + 1, 5 + int64(rank), 1, d.Base.HP + int64(rank+1)*20}
		// i 定位统计或属性中正在处理的成员，s 接收遍历统计或属性时当前的元素。
		for i, s := range preview.Stats {
			if s.BeforeMin != wantBefore[i] || s.BeforeMax != wantBefore[i] || s.AfterMin != wantAfter[i] || s.AfterMax != wantAfter[i] {
				t.Fatalf("阶%d新生字段%s未反映来源投影:%v", rank, s.Key, s)
			}
		}
	}
}
