// 本文件回归锁定目标的生命代次、射程、攻击覆盖区域，以及治疗和死亡对锁定的影响。
package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// prepareAutoTargetTest 返回准备自动目标测试供调用方使用。
// w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间，now 提供当前时点，作为状态有效期和计时判断的基准，positions 记录位置集合，使用可变数量的点位表示。
func prepareAutoTargetTest(w *worker, r *room, now time.Time, positions ...devconfig.Point) []*entity {
	// first 保存从怪物集合取出的第一项。
	first := r.Monsters[0]
	r.Auto, r.Pet = true, nil
	r.Monsters = nil
	// index 定位位置集合中正在处理的成员，point 接收遍历位置集合时当前的元素。
	for index, point := range positions {
		// m 保存当前处理所用的第一项。
		m := *first
		m.Handle, m.Life, m.Pos = uint32(10+index), uint64(index), point
		m.NextMove, m.NextAttack = now.Add(time.Minute), now.Add(time.Minute)
		r.Monsters = append(r.Monsters, &m)
	}
	r.MonsterCount = len(r.Monsters)
	return r.Monsters
}

// TestAutomaticTargetWaitsForSameLifeEvenWithAdjacentAlternative 核验首次合法自动施法应锁定真实怪物与生命，以及原怪死亡后应挑选下一只合法怪。
// t 提供断言失败报告、子用例和清理注册。
func TestAutomaticTargetWaitsForSameLifeEvenWithAdjacentAlternative(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 1, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	r.Player.Pos = devconfig.Point{X: 3, Y: 8}
	// monsters 保存由准备自动目标测试取得的怪物集合。
	monsters := prepareAutoTargetTest(w, r, now, devconfig.Point{X: 3, Y: 5}, devconfig.Point{X: 8, Y: 8})
	// chosen 保存从怪物集合取出的“chosen”，other 保存从怪物集合取出的另一项。
	chosen, other := monsters[0], monsters[1]
	// origin 保存位置，用于当前步骤中的大区或起点。
	origin := r.Player.Pos
	w.step(r, now)
	// ref 接收引用供后续步骤使用。
	ref := reference(r, chosen)
	if r.AutoTarget != ref || len(r.Casts) != 1 || r.Casts[0].Target != chosen.Handle {
		t.Fatal("首次合法自动施法没有锁定真实怪物与生命")
	}
	clearAction(r)
	other.Pos = devconfig.Point{X: 4, Y: 8}
	// where 接收遍历点位列表时当前的元素。
	for _, where := range []devconfig.Point{{X: 16, Y: 16}, {X: 3, Y: 5}} {
		chosen.Pos = where
		w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 3, Y: 7}}
		now = now.Add(time.Second)
		w.step(r, now)
		if r.AutoTarget != ref || len(r.Casts) != 0 || r.Player.AttackSequence != 0 || r.Player.Pos != origin {
			t.Fatal("锁定怪越界或被挡时，对贴脸的另一只怪换靶、普攻或走位")
		}
	}
	w.e.Store.Cfg.Fixture.Map.Blocked = nil
	if !w.autoSkill(r, now) || r.Casts[0].Target != chosen.Handle || r.Casts[0].TargetLife != ref.Life {
		t.Fatal("重获射程和视线后没攻击原生命")
	}
	w.applyDamage(r, &r.Player, chosen, chosen.HP, now, hitContext{At: now, PrivilegeCaptured: true})
	if r.AutoTarget != (actorRef{}) {
		t.Fatal("锁定怪实际死亡后没有释放引用")
	}
	r.KillFacts = nil
	clearAction(r)
	if !w.autoSkill(r, now) || r.AutoTarget != reference(r, other) || r.Casts[0].Target != other.Handle {
		t.Fatal("原怪死亡后没有挑选下一只合法怪")
	}
	clearAction(r)
	chosen.HP, chosen.Life, chosen.Pos = chosen.MaxHP, chosen.Life+1, other.Pos
	if !w.autoSkill(r, now) || r.AutoTarget != reference(r, other) || r.Casts[0].Target != other.Handle {
		t.Fatal("旧句柄重生的新生命抢占了新锁")
	}
}

// TestAutomaticOffensiveGeometryMustIncludeLockedMonster 核对「锁定怪进入合法效果范围后未施放或选心不正确」与「自动群伤打向了完全不含锁定怪的格子」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutomaticOffensiveGeometryMustIncludeLockedMonster(t *testing.T) {
	// tc 接收遍历临时结构列表时当前的元素。
	for _, tc := range []struct {
		// name 记录名称，使用文本表示。
		name string
		// skill 记录技能，使用数值表示。
		skill uint32
		// other 记录另一项，使用点位列表表示。
		other []devconfig.Point
		// reachable 记录“reachable”，使用点位表示。
		reachable devconfig.Point
		// aim 记录“aim”，使用点位表示。
		aim devconfig.Point
		// direction 记录朝向，使用数值表示。
		direction uint32
	}{
		{"直线", 10, []devconfig.Point{{X: 8, Y: 5}, {X: 8, Y: 4}}, devconfig.Point{X: 8, Y: 6}, devconfig.Point{X: 8, Y: 8}, 0},
		{"地面范围", 33, []devconfig.Point{{X: 9, Y: 5}, {X: 7, Y: 5}}, devconfig.Point{X: 8, Y: 5}, devconfig.Point{X: 8, Y: 5}, 0},
		{"自身范围", 24, []devconfig.Point{{X: 8, Y: 9}, {X: 9, Y: 8}, {X: 7, Y: 8}}, devconfig.Point{X: 8, Y: 7}, devconfig.Point{X: 8, Y: 8}, 0},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(tc.name, func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
			w, r, _ := stageBWorker(t, 1, tc.skill)
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// monsters 保存由准备自动目标测试取得的怪物集合。
			monsters := prepareAutoTargetTest(w, r, now, append([]devconfig.Point{{X: 16, Y: 16}}, tc.other...)...)
			// chosen 保存从怪物集合取出的“chosen”。
			chosen := monsters[0]
			r.AutoTarget = reference(r, chosen)
			// mana 保存魔法值，用于当前步骤中的魔法值。
			mana := r.Player.MP
			if w.autoSkill(r, now) || r.Player.MP != mana || r.AutoTarget != reference(r, chosen) {
				t.Fatal("自动群伤打向了完全不含锁定怪的格子")
			}
			chosen.Pos = tc.reachable
			if !w.autoSkill(r, now) || len(r.Casts) != 1 || r.AutoTarget != reference(r, chosen) ||
				r.Casts[0].TargetPoint != tc.aim || tc.skill == 10 && r.Casts[0].Direction != tc.direction {
				t.Fatal("锁定怪进入合法效果范围后未施放或选心不正确")
			}
		})
	}
}

// TestAutoTargetIntentKeepsLiveLockAndRejectsOldLife 核验手动显式施法不能独立选择其他目标，或把其写成自动锁，以及当前活怪不能成为自动首目标。
// t 提供断言失败报告、子用例和清理注册。
func TestAutoTargetIntentKeepsLiveLockAndRejectsOldLife(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 1, 1)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// monsters 保存由准备自动目标测试取得的怪物集合。
	monsters := prepareAutoTargetTest(w, r, now, devconfig.Point{X: 16, Y: 16}, devconfig.Point{X: 4, Y: 4})
	// chosen 保存从怪物集合取出的“chosen”，other 保存从怪物集合取出的另一项。
	chosen, other := monsters[0], monsters[1]
	// send 封装发送对应的后续处理，q 记录自动目标意图。
	send := func(q *pb.AutoTargetIntent) uint32 {
		t.Helper()
		// before 记录协议帧集合中的条目数。
		before := len(c.frames)
		w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SAutoTargetIntent, q.InputSequence, 3, q)})
		if len(c.frames) == before {
			return 0
		}
		return c.frames[len(c.frames)-1].Error
	}
	// request 封装请求对应的后续处理，sequence 记录序号，使用数值表示，target 记录目标，使用实体表示。
	request := func(sequence uint32, target *entity) *pb.AutoTargetIntent {
		return &pb.AutoTargetIntent{SceneEpoch: r.Scene, InputSequence: sequence, TargetHandle: target.Handle, TargetLife: target.Life}
	}
	// oldScene 保存由请求取得的原状态场景。
	oldScene := request(1, chosen)
	oldScene.SceneEpoch--
	// code 保存由发送取得的结果码。
	if code := send(oldScene); code != p.CodeInvalidArgument {
		t.Fatal("错误场景接受了点选", code)
	}
	// oldLife 保存由请求取得的原状态生命代次。
	oldLife := request(1, other)
	oldLife.TargetLife--
	// code 保存由发送取得的结果码。
	if code := send(oldLife); code != p.CodeSkillTarget {
		t.Fatal("原句柄错误生命接受了点选", code)
	}
	// code 保存由发送取得的结果码。
	if code := send(request(1, chosen)); code != p.CodeStaleInput {
		t.Fatal("点选绕过共享输入水位", code)
	}
	// code 保存由发送取得的结果码。
	if code := send(request(2, chosen)); code != 0 || r.AutoTarget != reference(r, chosen) {
		t.Fatal("当前活怪不能成为自动首目标", code)
	}
	// code 保存由发送取得的结果码。
	if code := send(request(3, other)); code != 0 || r.AutoTarget != reference(r, chosen) {
		t.Fatal("其他合法点选劫持了活锁", code)
	}
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SCastIntent, 4, 3,
		&pb.CastIntent{SceneEpoch: r.Scene, InputSequence: 4, SkillId: 1, TargetHandle: other.Handle})})
	if len(r.Casts) != 1 || r.Casts[0].Target != other.Handle || r.AutoTarget != reference(r, chosen) {
		t.Fatal("手动显式施法不能独立选择其他目标，或把其写成自动锁")
	}
	w.command(command{role: "role", conn: c, disconnect: true, at: now})
	if r.AutoTarget != reference(r, chosen) {
		t.Fatal("断线清除了仍存活的本地图目标")
	}
	r.Conn, r.Ready = c, true
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SAutoBattle, 5, 3, &pb.AutoBattleIntent{SceneEpoch: r.Scene})})
	if r.AutoTarget != (actorRef{}) || r.Auto {
		t.Fatal("显式关闭自动战斗仍保留旧目标")
	}
}

// TestAutomaticThrustingReachesLockedSecondCellWithOtherMonsterInFront 核对「锁定二格怪时优先打错前格，或丢失刺杀原本附带的前格普攻」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutomaticThrustingReachesLockedSecondCellWithOtherMonsterInFront(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 0, 12, 25)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// monsters 保存由准备自动目标测试取得的怪物集合。
	monsters := prepareAutoTargetTest(w, r, now, stepPoint(r.Player.Pos, 4, 1), stepPoint(r.Player.Pos, 4, 2))
	// front 保存从怪物集合取出的前端，locked 保存从怪物集合取出的已锁定。
	front, locked := monsters[0], monsters[1]
	r.AutoTarget = reference(r, locked)
	r.Melee.Thrusting, r.Melee.HalfMoon = true, true
	w.step(r, now)
	if r.Player.AttackSkillID != 12 || locked.HP >= locked.MaxHP || r.AutoTarget != reference(r, locked) ||
		len(r.Attacks) != 1 || r.Attacks[0].Target.Handle != front.Handle {
		t.Fatal("锁定二格怪时优先打错前格，或丢失刺杀原本附带的前格普攻")
	}
}

// TestAutomaticSupportSpellsPreserveEnemyLock 核对「辅助法术改写了敌对锁或被敌对射程阻止」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAutomaticSupportSpellsPreserveEnemyLock(t *testing.T) {
	// tc 接收遍历临时结构列表时当前的元素。
	for _, tc := range []struct {
		// job 记录任务或职业编号，使用数值表示。
		job uint32
		// skill 记录技能，使用数值表示。
		skill uint32
		// injure 标记是否处于“injure”状态。
		injure bool
	}{
		{2, 2, true},
		{1, 31, false},
	} {
		// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
		w, r, _ := stageBWorker(t, tc.job, tc.skill)
		// now 保存由Unix 时间取得的当前时间。
		now := time.Unix(1800000000, 0)
		// monsters 保存由准备自动目标测试取得的怪物集合。
		monsters := prepareAutoTargetTest(w, r, now, devconfig.Point{X: 16, Y: 16})
		// ref 接收引用供后续步骤使用。
		ref := reference(r, monsters[0])
		r.AutoTarget = ref
		if tc.injure {
			r.Player.HP--
		}
		if !w.autoSkill(r, now) || len(r.Casts) != 1 || r.Casts[0].Skill.ID != tc.skill ||
			r.Casts[0].Target == ref.Handle || r.AutoTarget != ref {
			t.Fatalf("辅助法术%d改写了敌对锁或被敌对射程阻止", tc.skill)
		}
	}
}

// TestPlayerDeathImmediatelyClearsAutomaticTarget 核对「角色死亡时仍持有旧锁定目标」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPlayerDeathImmediatelyClearsAutomaticTarget(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 0)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// monsters 保存由准备自动目标测试取得的怪物集合。
	monsters := prepareAutoTargetTest(w, r, now, stepPoint(r.Player.Pos, 4, 1))
	r.AutoTarget = reference(r, monsters[0])
	w.applyDamage(r, monsters[0], &r.Player, r.Player.HP, now, hitContext{At: now, PrivilegeCaptured: true})
	if r.Player.HP != 0 || r.AutoTarget != (actorRef{}) {
		t.Fatal("角色死亡时仍持有旧锁定目标")
	}
}
