// 验证位移技能资产预览的身份、职业和消耗边界，确保可预览能力与持久提交规则一致。

package persistence

import (
	"minimir/internal/devconfig"
	"testing"
)

// TestDisplacementUpgradePreviewShowsApprovedDistanceAndActualDamage 核验抗拒不能把玩家来源分支的基础威力冒充推怪伤害，用诊断「升级预览未显示唯一Excel的用户已审定距离」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestDisplacementUpgradePreviewShowsApprovedDistanceAndActualDamage(t *testing.T) {
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	if err != nil {
		t.Fatal(err)
	}
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint32{8, 27} {
		// from 保存由无符号整数“32”取得的来源。
		for from := uint32(0); from < 3; from++ {
			// quote 初始化用作报价的技能报价。
			quote := skillQuote{Plan: skillPlan{Kind: 2, SkillID: id, From: from, To: from + 1}}
			// preview 保存由协议取得的预览。
			preview := quote.Proto(cfg.Fixture)
			// distance 保存从统计或属性取出的距离。
			distance := preview.Stats[0]
			// s 接收技能供后续步骤使用。
			s, _ := cfg.Fixture.Skill(id)
			// d 保存当前处理所用的位移。
			d := s.Displacement
			if distance.BeforeMin != int64(d.DistanceBaseByLevel[from]) || distance.AfterMax != int64(d.DistanceBaseByLevel[from+1])+d.DistanceRandomBound-1 {
				t.Fatal("升级预览未显示唯一Excel的用户已审定距离")
			}
			// keys 初始化用作键集合的按文本索引的布尔状态映射。
			keys := map[string]bool{}
			// value 接收遍历统计或属性时当前的元素。
			for _, value := range preview.Stats {
				keys[value.Key] = true
				if id == 8 && value.Key == "monster_damage" && (value.BeforeMin != 0 || value.AfterMin != 0) {
					t.Fatal("抗拒预览伪造对怪物伤害")
				}
			}
			if id == 8 && (keys["base_power"] || !keys["monster_damage"] || !keys["push_base_success_percent"]) {
				t.Fatal("抗拒不能把玩家来源分支的基础威力冒充推怪伤害")
			}
			if id == 27 && (!keys["base_power"] || !keys["dash_distance"]) {
				t.Fatal("野蛮缺少单次威力或冲撞距离预览")
			}
		}
	}
}
