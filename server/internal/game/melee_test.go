package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"testing"
	"time"
)

func stageBWorker(t *testing.T, job uint32, ids ...uint32) (*worker, *room, *capturedConnection) {
	t.Helper()
	w, r, c := skillWorker(t, job)
	r.Role.Level = 40
	r.Role.Skills = nil
	for _, id := range ids {
		s, ok := w.e.Store.Cfg.Fixture.Skill(id)
		if !ok {
			t.Fatalf("唯一Excel夹具尚缺本组技能%d", id)
		}
		r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: id, Level: s.LearnedLevel})
	}
	for i := range w.e.Store.Cfg.Fixture.PlayerGrowth {
		if w.e.Store.Cfg.Fixture.PlayerGrowth[i].JobID == job {
			w.e.Store.Cfg.Fixture.PlayerGrowth[i].AttackDivisor = 1
		}
	}
	w.e.Store.Cfg.Fixture.Map.Blocked = nil
	w.refreshPlayerStats(r)
	r.Player.HP, r.Player.MP = r.Player.MaxHP, r.Player.MaxMP
	r.Monsters[0].HP, r.Monsters[0].MaxHP = 1000, 1000
	r.Monsters[0].Monster = &devconfig.Monster{Fighter: devconfig.Fighter{Agility: 0, MagicResistWeight: 10}}
	return w, r, c
}

func TestSlayingPassiveAndNextAttackReadiness(t *testing.T) {
	w, r, c := stageBWorker(t, 0, 7)
	now := time.Now()
	s, _ := w.e.Store.Cfg.Fixture.Skill(7)
	for rank, bonus := range []int64{5, 6, 7, 8} {
		r.Role.Skills[0].Level = uint32(rank)
		stats := w.fighter(r, &r.Player)
		if stats.AttackMax != 40+bonus || stats.Accuracy != 5+int64(rank) {
			t.Fatalf("攻杀%d阶永久DC/准确错误:%+v", rank, stats)
		}
	}
	r.Role.Skills[0].Level = 0
	if w.attackFacing(r, 4, 7, 1, now) != p.CodeSkillStateBlocked || r.Player.AttackSequence != 0 || r.Melee.Slaying {
		t.Fatal("客户端伪造攻杀准备或拒绝改变动作")
	}
	if w.attackFacing(r, 4, 0, 2, now) != 0 || r.Player.AttackSkillID != 0 || !r.Melee.Slaying {
		t.Fatal("本次普通挥刀没有准备下一次攻杀")
	}
	// 已受理时钟内拒绝不得吞掉已准备状态。
	if w.attackFacing(r, 4, 7, 3, now.Add(time.Millisecond)) != p.CodeSkillStateBlocked || !r.Melee.Slaying {
		t.Fatal("被动作锁拒绝的输入消耗了准备")
	}
	w.advanceEvents(r, r.Attacks[0].Due)
	next := now.Add(time.Duration(w.fighter(r, &r.Player).AttackMS) * time.Millisecond)
	if w.attackFacing(r, 4, 0, 4, next) != 0 || r.Player.AttackSkillID != s.ID || !r.Melee.Slaying {
		t.Fatal("攻杀未消费后重掷下一次准备，或没有权威选技")
	}
	found := false
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CAttackAccepted {
			var accepted pb.AttackAccepted
			if err := proto.Unmarshal(frame.Payload, &accepted); err != nil {
				t.Fatal(err)
			}
			if accepted.SkillId == 7 && accepted.SkillLevel == 0 && accepted.CasterLife == r.Player.Life && accepted.DelayMs == 300 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("AA缺实际技能/零阶/生命代次/命中延迟")
	}
}

func TestThrustingEightDirectionsSkipsFrontWallAndIgnoresOnlyArmour(t *testing.T) {
	for direction := uint32(0); direction < 8; direction++ {
		t.Run(string(rune('0'+direction)), func(t *testing.T) {
			w, r, _ := stageBWorker(t, 0, 12)
			now := time.Now()
			s, _ := w.e.Store.Cfg.Fixture.Skill(12)
			front := stepPoint(r.Player.Pos, direction, 1)
			w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{front}
			target := r.Monsters[0]
			target.Pos = stepPoint(r.Player.Pos, direction, 2)
			target.Monster.DefenceMin, target.Monster.DefenceMax = 1000, 1000
			if w.setSkillMode(r, s.ID, true, now) != 0 || w.attackFacing(r, direction, 0, 1, now) != 0 || target.HP != 990 || r.Player.AttackSkillID != s.ID || len(r.Attacks) != 0 {
				t.Fatal("刺杀没有隔墙命中精确第二格/错误应用AC/未立即命中")
			}
			// Agility不是必中；高敏捷仍会闪避，不能将‘无AC’误作‘无防御’。
			target.HP, target.Monster.Agility = 1000, 1000
			w.roll = func(n int64) int64 { return n - 1 }
			r.Player.NextAttack, r.Player.ActionReadyAt = time.Time{}, time.Time{}
			if w.attackFacing(r, direction, 0, 2, now) != 0 || target.HP != 1000 {
				t.Fatal("刺杀第二格没有进行独立敏捷检定")
			}
		})
	}
}

func TestHalfMoonExactAsymmetricFourCellsAndSharedSecondaryDamage(t *testing.T) {
	for direction := uint32(0); direction < 8; direction++ {
		t.Run(string(rune('0'+direction)), func(t *testing.T) {
			w, r, _ := stageBWorker(t, 0, 25)
			now := time.Now()
			s, _ := w.e.Store.Cfg.Fixture.Skill(25)
			definition := r.Monsters[0].Monster
			r.Monsters = nil
			for d := uint32(0); d < 8; d++ {
				r.Monsters = append(r.Monsters, &entity{Handle: 10 + d, Kind: 3, HP: 1000, MaxHP: 1000, Pos: stepPoint(r.Player.Pos, d, 1), Monster: definition})
			}
			r.MonsterCount = 8
			mana := r.Player.MP
			if w.setSkillMode(r, s.ID, true, now) != 0 || w.attackFacing(r, direction, 0, 1, now) != 0 || r.Player.MP != mana-s.MPBaseCost || len(r.Attacks) != 1 {
				t.Fatal("半月受理/共享扣蓝/前格延期不符")
			}
			for d, target := range r.Monsters {
				offset := (d - int(direction) + 8) % 8
				want := int64(1000)
				if offset == 7 || offset == 1 || offset == 2 {
					want = 988
				}
				if target.HP != want {
					t.Fatalf("方向%d相对格%d HP%d/%d", direction, offset, target.HP, want)
				}
			}
			w.advanceEvents(r, now.Add(300*time.Millisecond))
			if r.Monsters[direction].HP != 960 {
				t.Fatal("半月前格错误使用旁格倍率/命中时序")
			}
		})
	}
}

func TestFlamingChargeExactManaAirSwingExpiryAndOriginalCooldown(t *testing.T) {
	w, r, _ := stageBWorker(t, 0, 26)
	now := time.Now()
	s, _ := w.e.Store.Cfg.Fixture.Skill(26)
	r.Player.MP = s.MPBaseCost
	if w.setSkillMode(r, s.ID, true, now) != p.CodeSkillResource || !r.Melee.FlamingUntil.IsZero() {
		t.Fatal("烈火错误允许MP等于费用")
	}
	r.Player.MP++
	r.Player.ActionReadyAt = now.Add(time.Second)
	if w.setSkillMode(r, s.ID, true, now) != 0 || r.Player.MP != 1 || !r.NextSpell.IsZero() {
		t.Fatal("充能被错误套用普通施法时钟或重复扣蓝")
	}
	r.Player.ActionReadyAt = time.Time{}
	r.Monsters[0].Pos = devconfig.Point{X: 1, Y: 1}
	deadline := r.Melee.FlamingUntil
	if w.attackFacing(r, 4, 0, 1, now) != 0 || !r.Melee.Flaming || r.Player.AttackSkillID != 0 {
		t.Fatal("普通空挥消费了烈火")
	}
	r.Player.NextAttack, r.Player.ActionReadyAt = time.Time{}, time.Time{}
	r.Monsters[0].Pos = stepPoint(r.Player.Pos, 4, 1)
	r.Monsters[0].Monster.Agility = 1000
	w.roll = func(n int64) int64 { return n - 1 }
	if w.attackFacing(r, 4, 0, 2, now.Add(time.Second)) != 0 || r.Melee.Flaming || r.Player.AttackSkillID != s.ID || !r.Melee.FlamingUntil.Equal(deadline) || r.Player.MP != 1 {
		t.Fatal("烈火未在合法前格受理时消耗或提前使用重置了原CD")
	}
	w.advanceEvents(r, now.Add(time.Second+300*time.Millisecond))
	if r.Monsters[0].HP != 944 {
		t.Fatal("烈火没有绕过敏捷执行AC伤害")
	}
	r.Player.MP = r.Player.MaxMP
	if w.setSkillMode(r, s.ID, true, deadline.Add(-time.Nanosecond)) != p.CodeSkillCooldown || w.setSkillMode(r, s.ID, true, deadline) != 0 {
		t.Fatal("烈火早用后原10秒CD边界错误")
	}
	r.Player.HP = 0
	w.expireModes(r, r.Melee.FlamingUntil)
	if r.Melee.Flaming {
		t.Fatal("死亡暂停了烈火绝对截止")
	}
}

func TestModeSequenceReplayBindingAndNavigationIsolation(t *testing.T) {
	w, r, c := stageBWorker(t, 0, 12, 25, 26)
	point := devconfig.Point{X: 6, Y: 8}
	r.Destination = &point
	send := func(binding uint64, seq, id uint32, on bool) {
		w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SSkillModeIntent, seq, binding, &pb.SkillModeIntent{SceneEpoch: r.Scene, InputSequence: seq, SkillId: id, Enabled: on})})
	}
	send(3, 1, 12, true)
	send(3, 2, 25, true)
	if !r.Melee.Thrusting || !r.Melee.HalfMoon || r.Destination != &point {
		t.Fatal("模式相互覆盖或清除导航")
	}
	send(3, 2, 12, false)
	send(2, 100, 12, false)
	if !r.Melee.Thrusting || r.InputSequence != 2 {
		t.Fatal("重放或旧连接输入改变模式/污染输入水位")
	}
	state := w.modeState(r, time.Now())
	if state.SceneEpoch != r.Scene || state.PlayerLife != r.Player.Life || len(state.States) < 3 {
		t.Fatal("临时模式全量缺身份/生命代次")
	}
	r.Ready = false
	w.baseline(r)
	var baseline pb.SceneBaselineChunk
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, &baseline); err != nil || baseline.SkillModeState == nil || baseline.SkillModeState.Revision != r.Melee.Revision {
		t.Fatal("重新观察基线没有恢复真实模式")
	}
}

func TestHalfMoonSharedDrawAndFourKillFactsKeepActualImpactTimes(t *testing.T) {
	w, r, _ := stageBWorker(t, 0, 25)
	now := time.Now()
	r.Role.PrivilegeUntil = now.Add(100 * time.Millisecond)
	r.Role.Equipped = []domain.Equipped{{Item: domain.Item{ItemID: 218}}}
	definition := r.Monsters[0].Monster
	r.Monsters = nil
	for i, direction := range []uint32{7, 0, 1, 2} {
		r.Monsters = append(r.Monsters, &entity{Handle: uint32(10 + i), Kind: 3, Pos: stepPoint(r.Player.Pos, direction, 1), HP: 1, MaxHP: 1, Monster: definition})
	}
	r.MonsterCount = 4
	dcDraws := 0
	w.roll = func(n int64) int64 {
		if n == 4 { // 测试木剑2..5与固定本体DC形成唯一四值攻击区间。
			dcDraws++
		}
		return 0
	}
	if w.setSkillMode(r, 25, true, now) != 0 || w.attackFacing(r, 0, 0, 1, now) != 0 || dcDraws != 1 || len(r.KillFacts) != 3 {
		t.Fatal("半月旁格没有共享一次DC样本/没有形成三个独立kill事实")
	}
	for i, target := range []uint32{10, 12, 13} {
		fact := r.KillFacts[i]
		if fact.Target.Handle != target || !fact.At.Equal(now) || fact.Bound {
			t.Fatal("立即旁格逐杀顺序或受理时权益绑定错误")
		}
	}
	w.advanceEvents(r, now.Add(time.Second))
	if len(r.KillFacts) != 4 || r.KillFacts[3].Target.Handle != 11 || !r.KillFacts[3].At.Equal(now.Add(300*time.Millisecond)) || !r.KillFacts[3].Bound {
		t.Fatal("晚处理前格丢杀/用处理时钟覆盖实际300ms事件时间")
	}
	for i := range r.KillFacts {
		q := w.killSettlement(r, r.KillFacts[i])
		if q.SourceSeq != r.Role.SourceSeq+1 || !q.EventAt.Equal(r.KillFacts[i].At) || q.TargetLife != r.KillFacts[i].Target.Life {
			t.Fatal("多杀没有沿各自稳定事件推进SourceSeq")
		}
		r.Role.SourceSeq = q.SourceSeq
	}
}

func TestAttackBudgetRejectDoesNotConsumeModesManaOrReadiness(t *testing.T) {
	w, r, _ := stageBWorker(t, 0, 7, 12, 25, 26)
	now := time.Now()
	r.Melee.Thrusting, r.Melee.HalfMoon, r.Melee.Slaying, r.Melee.Flaming = true, true, true, true
	r.Melee.FlamingUntil = now.Add(time.Second)
	state, mana := r.Melee, r.Player.MP
	r.Casts = make([]pendingCast, w.eventCapacity())
	if w.attackFacing(r, 4, 0, 1, now) != p.CodeSkillStateBlocked || r.Melee != state || r.Player.MP != mana || r.Player.AttackSequence != 0 {
		t.Fatal("有界事件拒绝消费了充能/攻杀/蓝或改变动作")
	}
}

func TestSameConnectionReviveAdvancesModeRevisionAndRetainsOriginalModes(t *testing.T) {
	w, r, c := stageBWorker(t, 0, 7, 12, 25, 26)
	now := time.Now()
	r.Melee = meleeState{Thrusting: true, HalfMoon: true, Slaying: true, Flaming: true, FlamingUntil: now.Add(5 * time.Second), Revision: 9}
	r.Player.HP, r.DeathID, r.Auto = 0, "old_death", false
	oldLife, oldRevision, deadline := r.Player.Life, r.Melee.Revision, r.Melee.FlamingUntil
	w.revivePlayer(r)
	w.snapshot(r)
	w.modes(r, now)
	if r.Player.Life != oldLife+1 || r.Melee.Revision != oldRevision+1 || !r.Melee.Thrusting || !r.Melee.HalfMoon || !r.Melee.Slaying || !r.Melee.Flaming || !r.Melee.FlamingUntil.Equal(deadline) || r.Conn != c || r.Auto {
		t.Fatal("同连接复活清模式/重置CD，或未同步推进观察代次")
	}
	var baseline pb.SceneBaselineChunk
	var mode pb.SkillModeState
	if err := proto.Unmarshal(c.frames[len(c.frames)-2].Payload, &baseline); err != nil {
		t.Fatal(err)
	}
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, &mode); err != nil {
		t.Fatal(err)
	}
	if baseline.SkillModeState.Revision <= oldRevision || baseline.SkillModeState.PlayerLife != r.Player.Life || mode.Revision != baseline.SkillModeState.Revision || mode.PlayerLife != baseline.SkillModeState.PlayerLife {
		t.Fatal("新life基线与随后全量状态不能解除客户端同步等待")
	}
}
