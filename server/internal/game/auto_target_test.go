package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func prepareAutoTargetTest(w *worker, r *room, now time.Time, positions ...devconfig.Point) []*entity {
	first := r.Monsters[0]
	r.Auto, r.Pet = true, nil
	r.Monsters = nil
	for index, point := range positions {
		m := *first
		m.Handle, m.Life, m.Pos = uint32(10+index), uint64(index), point
		m.NextMove, m.NextAttack = now.Add(time.Minute), now.Add(time.Minute)
		r.Monsters = append(r.Monsters, &m)
	}
	r.MonsterCount = len(r.Monsters)
	return r.Monsters
}

func TestAutomaticTargetWaitsForSameLifeEvenWithAdjacentAlternative(t *testing.T) {
	w, r, _ := stageBWorker(t, 1, 1)
	now := time.Unix(1800000000, 0)
	r.Player.Pos = devconfig.Point{X: 3, Y: 8}
	monsters := prepareAutoTargetTest(w, r, now, devconfig.Point{X: 3, Y: 5}, devconfig.Point{X: 8, Y: 8})
	chosen, other := monsters[0], monsters[1]
	origin := r.Player.Pos
	w.step(r, now)
	ref := reference(r, chosen)
	if r.AutoTarget != ref || len(r.Casts) != 1 || r.Casts[0].Target != chosen.Handle {
		t.Fatal("首次合法自动施法没有锁定真实怪物与生命")
	}
	clearAction(r)
	other.Pos = devconfig.Point{X: 4, Y: 8}
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

func TestAutomaticOffensiveGeometryMustIncludeLockedMonster(t *testing.T) {
	for _, tc := range []struct {
		name      string
		skill     uint32
		other     []devconfig.Point
		reachable devconfig.Point
		aim       devconfig.Point
		direction uint32
	}{
		{"直线", 10, []devconfig.Point{{X: 8, Y: 5}, {X: 8, Y: 4}}, devconfig.Point{X: 8, Y: 6}, devconfig.Point{X: 8, Y: 8}, 0},
		{"地面范围", 33, []devconfig.Point{{X: 9, Y: 5}, {X: 7, Y: 5}}, devconfig.Point{X: 8, Y: 5}, devconfig.Point{X: 8, Y: 5}, 0},
		{"自身范围", 24, []devconfig.Point{{X: 8, Y: 9}, {X: 9, Y: 8}, {X: 7, Y: 8}}, devconfig.Point{X: 8, Y: 7}, devconfig.Point{X: 8, Y: 8}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, r, _ := stageBWorker(t, 1, tc.skill)
			now := time.Unix(1800000000, 0)
			monsters := prepareAutoTargetTest(w, r, now, append([]devconfig.Point{{X: 16, Y: 16}}, tc.other...)...)
			chosen := monsters[0]
			r.AutoTarget = reference(r, chosen)
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

func TestAutoTargetIntentKeepsLiveLockAndRejectsOldLife(t *testing.T) {
	w, r, c := stageBWorker(t, 1, 1)
	now := time.Now()
	monsters := prepareAutoTargetTest(w, r, now, devconfig.Point{X: 16, Y: 16}, devconfig.Point{X: 4, Y: 4})
	chosen, other := monsters[0], monsters[1]
	send := func(q *pb.AutoTargetIntent) uint32 {
		t.Helper()
		before := len(c.frames)
		w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SAutoTargetIntent, q.InputSequence, 3, q)})
		if len(c.frames) == before {
			return 0
		}
		return c.frames[len(c.frames)-1].Error
	}
	request := func(sequence uint32, target *entity) *pb.AutoTargetIntent {
		return &pb.AutoTargetIntent{SceneEpoch: r.Scene, InputSequence: sequence, TargetHandle: target.Handle, TargetLife: target.Life}
	}
	oldScene := request(1, chosen)
	oldScene.SceneEpoch--
	if code := send(oldScene); code != p.CodeInvalidArgument {
		t.Fatal("错误场景接受了点选", code)
	}
	oldLife := request(1, other)
	oldLife.TargetLife--
	if code := send(oldLife); code != p.CodeSkillTarget {
		t.Fatal("原句柄错误生命接受了点选", code)
	}
	if code := send(request(1, chosen)); code != p.CodeStaleInput {
		t.Fatal("点选绕过共享输入水位", code)
	}
	if code := send(request(2, chosen)); code != 0 || r.AutoTarget != reference(r, chosen) {
		t.Fatal("当前活怪不能成为自动首目标", code)
	}
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

func TestAutomaticThrustingReachesLockedSecondCellWithOtherMonsterInFront(t *testing.T) {
	w, r, _ := stageBWorker(t, 0, 12, 25)
	now := time.Unix(1800000000, 0)
	monsters := prepareAutoTargetTest(w, r, now, stepPoint(r.Player.Pos, 4, 1), stepPoint(r.Player.Pos, 4, 2))
	front, locked := monsters[0], monsters[1]
	r.AutoTarget = reference(r, locked)
	r.Melee.Thrusting, r.Melee.HalfMoon = true, true
	w.step(r, now)
	if r.Player.AttackSkillID != 12 || locked.HP >= locked.MaxHP || r.AutoTarget != reference(r, locked) ||
		len(r.Attacks) != 1 || r.Attacks[0].Target.Handle != front.Handle {
		t.Fatal("锁定二格怪时优先打错前格，或丢失刺杀原本附带的前格普攻")
	}
}

func TestAutomaticSupportSpellsPreserveEnemyLock(t *testing.T) {
	for _, tc := range []struct {
		job    uint32
		skill  uint32
		injure bool
	}{
		{2, 2, true},
		{1, 31, false},
	} {
		w, r, _ := stageBWorker(t, tc.job, tc.skill)
		now := time.Unix(1800000000, 0)
		monsters := prepareAutoTargetTest(w, r, now, devconfig.Point{X: 16, Y: 16})
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

func TestPlayerDeathImmediatelyClearsAutomaticTarget(t *testing.T) {
	w, r, _ := stageBWorker(t, 0)
	now := time.Unix(1800000000, 0)
	monsters := prepareAutoTargetTest(w, r, now, stepPoint(r.Player.Pos, 4, 1))
	r.AutoTarget = reference(r, monsters[0])
	w.applyDamage(r, monsters[0], &r.Player, r.Player.HP, now, hitContext{At: now, PrivilegeCaptured: true})
	if r.Player.HP != 0 || r.AutoTarget != (actorRef{}) {
		t.Fatal("角色死亡时仍持有旧锁定目标")
	}
}
