// 本文件验证圣言术的来源判定顺序、消耗、延迟效果与原生命击杀归属。
package game

import (
	"testing"
	"time"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// turnUndeadFixture 构造只依赖既有法师房间的隔离施法输入。t 提供测试结果报告。
func turnUndeadFixture(t *testing.T) (*worker, *room, *entity) {
	t.Helper()
	// w、r 是使用当前已批准房间规则的法师和其原分片。
	w, r, _ := stageAWorker(t, 1)
	// skill 是本组限定的圣言术候选，冻结延迟和消费以测试运行链。
	skill := devconfig.Skill{
		ID: 32, JobID: 1, Behavior: "turn_undead", TargetKind: "enemy_entity", RangeCells: 9,
		MPBaseCost: 52, MPLevelCost: 13, MaxLevel: 3, DelayBaseMS: 500, ActionMS: 600, GlobalSpellMS: 1800,
	}
	// f 是本测试独占的规则快照。主试玩是否已选择该技能不影响本组测试。
	f := &w.e.Store.Cfg.Fixture
	if _, exists := f.Skill(skill.ID); !exists {
		f.Skills = append(f.Skills, skill)
	}
	r.Role.Level = 32
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: skill.ID})
	r.Player.MP, r.Player.MaxMP = 110, 110
	// target 是原世界的第一个怪物。definition 单独复制，以免修改规则模板。
	target := r.Monsters[0]
	definition := *target.Monster
	definition.Undead, definition.Level = true, 30
	target.Monster = &definition
	return w, r, target
}

// TestTurnUndeadChecksLevelsBeforeSuccessRoll 固定掷点，覆盖等级闸门和升级后的概率边缘。t 报告断言。
func TestTurnUndeadChecksLevelsBeforeSuccessRoll(t *testing.T) {
	// w 直接测试来源公式，以受控掷点记录消耗顺序。
	w := &worker{}
	// draws 保存实际请求的随机区间，values 提供下次用于比较的实际掷点。
	var draws []int64
	values := []int64{0, 1, 23, 1, 24, 1, 24}
	w.roll = func(bound int64) int64 {
		// bound 是本次随机区间长度，result 是固定复现输入。
		draws = append(draws, bound)
		result := values[0]
		values = values[1:]
		return result
	}
	if w.turnUndeadSucceeds(31, 30, 0) || len(draws) != 1 || draws[0] != 2 {
		t.Fatal("仅高一级且掷零应被等级门禁拦下，不应提前消耗概率掷点")
	}
	if !w.turnUndeadSucceeds(31, 30, 0) || w.turnUndeadSucceeds(31, 30, 0) || !w.turnUndeadSucceeds(31, 30, 1) {
		t.Fatal("阶零阈值应为24且不含等号，升阶后同一掷点应成功")
	}
	if len(values) != 0 || len(draws) != 7 || draws[2] != 100 || draws[4] != 100 || draws[6] != 100 {
		t.Fatalf("概率抽样域或顺序错误：%v", draws)
	}
}

// TestTurnUndeadRejectsNonUndeadAndConsumesLegalMiss 验证普通目标失败与合法概率失败的资源区别。t 报告断言。
func TestTurnUndeadRejectsNonUndeadAndConsumesLegalMiss(t *testing.T) {
	// w、r 和 target 分别是执行器、施法者房间及同图候选目标。
	w, r, target := turnUndeadFixture(t)
	now := time.Unix(1800000000, 0)
	target.Monster.Undead = false
	w.roll = func(_ int64) int64 {
		t.Fatal("非法目标不应消耗掷点")
		return 0
	}
	if w.cast(r, 32, target.Handle, 1, now) != p.CodeSkillTarget || r.Player.MP != 110 || r.CastID != 0 {
		t.Fatal("普通怪物没有在受理和扣费前被拒绝")
	}
	target.Monster.Undead = true
	w.roll = func(bound int64) int64 {
		// bound 是合法随机区间，取最大值使概率判定失败。
		return bound - 1
	}
	if w.cast(r, 32, target.Handle, 2, now) != 0 || r.Player.MP != 58 || r.CastID != 1 || len(r.Casts) != 0 || len(r.KillFacts) != 0 || target.HP != 1000 {
		t.Fatal("合法概率失败应消耗魔法和动作，但不创建延迟击杀")
	}
}

// TestTurnUndeadDelaysAndVerifiesOriginalLife 验证成功命中时钟、范围和迟到对新生命的隔离。t 报告断言。
func TestTurnUndeadDelaysAndVerifiesOriginalLife(t *testing.T) {
	// replace 区分原生命移动与怪物被重新生成。
	for _, replace := range []bool{false, true} {
		// w、r 和 target 各指本次测试的原分片、房间和接收效果的怪物。
		w, r, target := turnUndeadFixture(t)
		now := time.Unix(1800000000, 0)
		w.roll = func(_ int64) int64 { return 0 }
		if w.cast(r, 32, target.Handle, 1, now) != 0 || len(r.Casts) != 1 || target.HP != 1000 {
			t.Fatal("成功施放应保留一个延迟处决事件")
		}
		// due 是冻结的生效时刻，life 是最初锁定的怪物出生代次。
		due, life := r.Casts[0].Due, target.Life
		if !due.Equal(now.Add(500 * time.Millisecond)) {
			t.Fatalf("圣言术命中时刻偏离来源：%v", due)
		}
		w.advanceEvents(r, due.Add(-time.Nanosecond))
		if target.HP != 1000 {
			t.Fatal("圣言术提前扣血")
		}
		// 已接受的处决跟踪原怪物而非原目标格。
		target.Pos = devconfig.Point{X: 0, Y: 0}
		if replace {
			target.Life++
		}
		w.advanceEvents(r, due.Add(time.Second))
		if replace {
			if target.HP != 1000 || len(r.KillFacts) != 0 {
				t.Fatal("圣言术迟到后处决了新的怪物生命")
			}
			continue
		}
		if target.HP != 0 || len(r.KillFacts) != 1 || r.KillFacts[0].Target.Life != life || !r.KillFacts[0].At.Equal(due) {
			t.Fatal("处决没有按原生命、应生效时刻和一次击杀事实结算")
		}
		w.advanceEvents(r, due.Add(2*time.Second))
		if len(r.KillFacts) != 1 {
			t.Fatal("圣言术重复追加击杀")
		}
	}
}
