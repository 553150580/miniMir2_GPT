// 本文件回归角色攻击动作锁、攻击冷却和移动节流各自的边界。
package game

import (
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"testing"
	"time"
)

// clockActors 处理时钟行动实体集合。
// r 指向当前执行器拥有并更新的目标房间，kind 记录类型，使用数值表示。
func clockActors(r *room, kind uint32) (*entity, *entity) {
	switch kind {
	case 1:
		return &r.Player, r.Monsters[0]
	case 2:
		return r.Pet, r.Monsters[0]
	default:
		return r.Monsters[0], &r.Player
	}
}

// TestAttackUsesActionAndAttackClocksIndependentOfMoveThrottle 核对「攻击被移动节流拖到500ms或提前释放时钟」与「动作锁未结束却接受攻击/改写时钟」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAttackUsesActionAndAttackClocksIndependentOfMoveThrottle(t *testing.T) {
	// kind 接收遍历数值列表时当前的元素。
	for _, kind := range []uint32{1, 2, 3} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[uint32]string{1: "player_source_attack", 2: "pet", 3: "monster"}[kind], func(t *testing.T) {
			// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
			w, r, _ := stageAWorker(t, 1)
			// from 保存由时钟行动实体集合取得的来源，target 保存由时钟行动实体集合取得的目标。
			from, target := clockActors(r, kind)
			if kind == 2 {
				target.Pos = devconfig.Point{X: 7, Y: 9}
			}
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// 只构造分离的动作/移动截止前态，不实现推挤、不设置运行参数。
			from.ActionReadyAt = now.Add(300 * time.Millisecond)
			from.NextMove = now.Add(500 * time.Millisecond)
			from.NextAttack = now.Add(-time.Millisecond)
			// originalMove 保存下一状态移动，用于当前步骤中的原始移动，originalAttack 保存下一状态攻击，用于当前步骤中的原始攻击。
			originalMove, originalAttack := from.NextMove, from.NextAttack
			w.attack(r, from, target, now.Add(300*time.Millisecond-time.Nanosecond))
			if from.AttackSequence != 0 || from.NextAttack != originalAttack || from.NextMove != originalMove {
				t.Fatal("动作锁未结束却接受攻击/改写时钟")
			}
			w.attack(r, from, target, now.Add(300*time.Millisecond))
			if from.AttackSequence != 1 || from.NextMove != originalMove || !from.NextAttack.Equal(now.Add(300*time.Millisecond+time.Duration(w.fighter(r, from).AttackMS)*time.Millisecond)) {
				t.Fatal("攻击被移动节流拖到500ms或提前释放时钟")
			}
		})
	}
}

// TestIndependentMoveThrottleStillWaitsUntil500Boundary 核验既有>=边界应解除移动节流，用诊断「移动锁未结束却移动」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestIndependentMoveThrottleStillWaitsUntil500Boundary(t *testing.T) {
	// kind 接收遍历数值列表时当前的元素。
	for _, kind := range []uint32{1, 2, 3} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[uint32]string{1: "player", 2: "pet", 3: "monster"}[kind], func(t *testing.T) {
			// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
			w, r, _ := stageAWorker(t, 1)
			// from 保存由时钟行动实体集合取得的来源。
			from, _ := clockActors(r, kind)
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// origin 保存位置，用于当前步骤中的大区或起点。
			origin := from.Pos
			from.ActionReadyAt = now.Add(300 * time.Millisecond)
			from.NextMove = now.Add(500 * time.Millisecond)
			// at 接收遍历时间点列表时当前的元素。
			for _, at := range []time.Time{now.Add(300 * time.Millisecond), now.Add(500*time.Millisecond - time.Nanosecond)} {
				w.move(r, from, devconfig.Point{X: 9, Y: 9}, at, w.fighter(r, from).MoveMS)
				if from.Pos != origin {
					t.Fatal("移动锁未结束却移动")
				}
			}
			w.move(r, from, devconfig.Point{X: 9, Y: 9}, now.Add(500*time.Millisecond), w.fighter(r, from).MoveMS)
			if from.Pos == origin {
				t.Fatal("既有>=边界没有解除移动节流")
			}
		})
	}
}

// TestWalkPreservesLaterAttackDeadlineAndPlayerActionLock 核对「玩家普通walk丢失原动作锁」与「普通行走缩短了原攻击CD」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestWalkPreservesLaterAttackDeadlineAndPlayerActionLock(t *testing.T) {
	// kind 接收遍历数值列表时当前的元素。
	for _, kind := range []uint32{1, 2, 3} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(map[uint32]string{1: "player", 2: "pet", 3: "monster"}[kind], func(t *testing.T) {
			// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
			w, r, _ := stageAWorker(t, 1)
			// from 保存由时钟行动实体集合取得的来源。
			from, _ := clockActors(r, kind)
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// later 保存由新增取得的“later”。
			later := now.Add(10 * time.Second)
			from.NextAttack = later
			w.move(r, from, devconfig.Point{X: 9, Y: 9}, now, w.fighter(r, from).MoveMS)
			if from.NextAttack != later {
				t.Fatal("普通行走缩短了原攻击CD")
			}
			if kind == 1 && from.ActionReadyAt != from.NextMove {
				t.Fatal("玩家普通walk丢失原动作锁")
			}
		})
	}
}

// TestAttackCannotReleaseExistingAttackOrSpellDeadlines 核对「原攻击CD未到被移动时钟解耦误释放」与「普通攻击改写移动/共享法术/单技CD」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAttackCannotReleaseExistingAttackOrSpellDeadlines(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.Player.NextMove = now.Add(500 * time.Millisecond)
	r.Player.ActionReadyAt = now.Add(300 * time.Millisecond)
	r.Player.NextAttack = now.Add(700 * time.Millisecond)
	r.NextSpell = now.Add(10 * time.Second)
	r.SkillCooldowns = map[uint32]time.Time{1: now.Add(20 * time.Second)}
	// move 保存下一状态移动，用于当前步骤中的移动，spell 保存下一状态法术，用于当前步骤中的法术，skill 保存从技能冷却集合取出的技能。
	move, spell, skill := r.Player.NextMove, r.NextSpell, r.SkillCooldowns[1]
	if w.attackFacing(r, 4, 0, 1, now.Add(700*time.Millisecond-time.Nanosecond)) != p.CodeSkillStateBlocked || r.Player.AttackSequence != 0 {
		t.Fatal("原攻击CD未到被移动时钟解耦误释放")
	}
	if w.attackFacing(r, 4, 0, 2, now.Add(700*time.Millisecond)) != 0 || r.Player.AttackSequence != 1 {
		t.Fatal("原攻击CD等号边界仍错误拒绝")
	}
	if r.Player.NextMove != move || r.NextSpell != spell || r.SkillCooldowns[1] != skill {
		t.Fatal("普通攻击改写移动/共享法术/单技CD")
	}
	// mana 保存魔法值，用于当前步骤中的魔法值。
	mana := r.Player.MP
	if w.cast(r, 1, r.Monsters[0].Handle, 3, now.Add(2*time.Second)) != p.CodeSkillCooldown || r.Player.MP != mana {
		t.Fatal("普通攻击释放了未到期共享法术CD")
	}
	r.NextSpell = time.Time{}
	if w.cast(r, 1, r.Monsters[0].Handle, 4, now.Add(2*time.Second)) != p.CodeSkillCooldown || r.Player.MP != mana {
		t.Fatal("普通攻击释放了未到期单技CD")
	}
}
