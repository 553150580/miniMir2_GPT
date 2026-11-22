// 本文件回归普攻模式、刺杀与半月目标形状、烈火蓄力、动作顺序和复活后的状态修订。
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

// stageBWorker 处理阶段b房间执行器。
// t 提供断言失败报告、子用例和清理注册，job 记录任务或职业编号，使用数值表示，ids 记录标识集合，使用可变数量的数值表示。
func stageBWorker(t *testing.T, job uint32, ids ...uint32) (*worker, *room, *capturedConnection) {
	t.Helper()
	// w 接收技能房间执行器供后续步骤使用，r 接收技能房间执行器供后续步骤使用，c 接收技能房间执行器供后续步骤使用。
	w, r, c := skillWorker(t, job)
	r.Role.Level = 40
	r.Role.Skills = nil
	// id 接收遍历标识集合时当前的元素。
	for _, id := range ids {
		// s 接收技能供后续步骤使用，ok 表明查询、校验或类型转换是否成功。
		s, ok := w.e.Store.Cfg.Fixture.Skill(id)
		if !ok {
			t.Fatalf("唯一Excel夹具尚缺本组技能%d", id)
		}
		r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: id, Level: s.LearnedLevel})
	}
	// i 标记遍历玩家成长时访问的下标。
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

// TestSlayingPassiveAndNextAttackReadiness 核验攻杀未消费后重掷下一次准备，或应权威选技，以及本次普通挥刀应准备下一次攻杀。
// t 提供断言失败报告、子用例和清理注册。
func TestSlayingPassiveAndNextAttackReadiness(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 0, 7)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// s 接收技能供后续步骤使用。
	s, _ := w.e.Store.Cfg.Fixture.Skill(7)
	// rank 定位数值列表中正在处理的成员，bonus 接收遍历数值列表时当前的元素。
	for rank, bonus := range []int64{5, 6, 7, 8} {
		r.Role.Skills[0].Level = uint32(rank)
		// stats 保存由战斗单位取得的统计或属性。
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
	// next 保存由新增取得的下一状态。
	next := now.Add(time.Duration(w.fighter(r, &r.Player).AttackMS) * time.Millisecond)
	if w.attackFacing(r, 4, 0, 4, next) != 0 || r.Player.AttackSkillID != s.ID || !r.Melee.Slaying {
		t.Fatal("攻杀未消费后重掷下一次准备，或没有权威选技")
	}
	// found 保存当前处理所用的已找到标记。
	found := false
	// frame 接收遍历协议帧集合时当前的元素。
	for _, frame := range c.frames {
		if frame.ID == p.MsgS2CAttackAccepted {
			// accepted 记录已受理，使用攻击已受理表示。
			var accepted pb.AttackAccepted
			// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
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

// TestThrustingEightDirectionsSkipsFrontWallAndIgnoresOnlyArmour 核验刺杀应隔墙命中精确第二格/错误应用AC/未立即命中，以及刺杀第二格应进行独立敏捷检定。
// t 提供断言失败报告、子用例和清理注册。
func TestThrustingEightDirectionsSkipsFrontWallAndIgnoresOnlyArmour(t *testing.T) {
	// direction 保存由无符号整数“32”取得的朝向。
	for direction := uint32(0); direction < 8; direction++ {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(string(rune('0'+direction)), func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
			w, r, _ := stageBWorker(t, 0, 12)
			// now 记录本次处理开始时的时间。
			now := time.Now()
			// s 接收技能供后续步骤使用。
			s, _ := w.e.Store.Cfg.Fixture.Skill(12)
			// front 保存由步骤点位取得的前端。
			front := stepPoint(r.Player.Pos, direction, 1)
			w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{front}
			// target 保存从怪物集合取出的目标。
			target := r.Monsters[0]
			target.Pos = stepPoint(r.Player.Pos, direction, 2)
			target.Monster.DefenceMin, target.Monster.DefenceMax = 1000, 1000
			if w.setSkillMode(r, s.ID, true, now) != 0 || w.attackFacing(r, direction, 0, 1, now) != 0 || target.HP != 990 || r.Player.AttackSkillID != s.ID || len(r.Attacks) != 0 {
				t.Fatal("刺杀没有隔墙命中精确第二格/错误应用AC/未立即命中")
			}
			// Agility不是必中；高敏捷仍会闪避，不能将‘无AC’误作‘无防御’。
			target.HP, target.Monster.Agility = 1000, 1000
			// n 记录数值。
			w.roll = func(n int64) int64 { return n - 1 }
			r.Player.NextAttack, r.Player.ActionReadyAt = time.Time{}, time.Time{}
			if w.attackFacing(r, direction, 0, 2, now) != 0 || target.HP != 1000 {
				t.Fatal("刺杀第二格没有进行独立敏捷检定")
			}
		})
	}
}

// TestHalfMoonExactAsymmetricFourCellsAndSharedSecondaryDamage 核对「半月前格错误使用旁格倍率/命中时序」与「半月受理/共享扣蓝/前格延期不符」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHalfMoonExactAsymmetricFourCellsAndSharedSecondaryDamage(t *testing.T) {
	// direction 保存由无符号整数“32”取得的朝向。
	for direction := uint32(0); direction < 8; direction++ {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(string(rune('0'+direction)), func(t *testing.T) {
			// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
			w, r, _ := stageBWorker(t, 0, 25)
			// now 记录本次处理开始时的时间。
			now := time.Now()
			// s 接收技能供后续步骤使用。
			s, _ := w.e.Store.Cfg.Fixture.Skill(25)
			// definition 保存怪物，用于当前步骤中的定义。
			definition := r.Monsters[0].Monster
			r.Monsters = nil
			// d 接收无符号整数“32”供后续步骤使用。
			for d := uint32(0); d < 8; d++ {
				r.Monsters = append(r.Monsters, &entity{Handle: 10 + d, Kind: 3, HP: 1000, MaxHP: 1000, Pos: stepPoint(r.Player.Pos, d, 1), Monster: definition})
			}
			r.MonsterCount = 8
			// mana 保存魔法值，用于当前步骤中的魔法值。
			mana := r.Player.MP
			if w.setSkillMode(r, s.ID, true, now) != 0 || w.attackFacing(r, direction, 0, 1, now) != 0 || r.Player.MP != mana-s.MPBaseCost || len(r.Attacks) != 1 {
				t.Fatal("半月受理/共享扣蓝/前格延期不符")
			}
			// d 定位怪物集合中正在处理的成员，target 接收遍历怪物集合时当前的元素。
			for d, target := range r.Monsters {
				// offset 保存根据d与整数的计算结果与当前数据的计算结果与当前数据计算的偏移。
				offset := (d - int(direction) + 8) % 8
				// want 保存由整数“64”取得的期望值。
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

// TestFlamingChargeExactManaAirSwingExpiryAndOriginalCooldown 核验烈火应绕过敏捷执行AC伤害，用诊断「烈火未在合法前格受理时消耗或提前使用重置了原CD」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestFlamingChargeExactManaAirSwingExpiryAndOriginalCooldown(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 0, 26)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	// s 接收技能供后续步骤使用。
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
	// deadline 保存“flaming”有效截止时间，用于当前步骤中的截止时间。
	deadline := r.Melee.FlamingUntil
	if w.attackFacing(r, 4, 0, 1, now) != 0 || !r.Melee.Flaming || r.Player.AttackSkillID != 0 {
		t.Fatal("普通空挥消费了烈火")
	}
	r.Player.NextAttack, r.Player.ActionReadyAt = time.Time{}, time.Time{}
	r.Monsters[0].Pos = stepPoint(r.Player.Pos, 4, 1)
	r.Monsters[0].Monster.Agility = 1000
	// n 记录数值。
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

// TestModeSequenceReplayBindingAndNavigationIsolation 核验重新观察基线应恢复真实模式，用诊断「重放或旧连接输入改变模式/污染输入水位」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestModeSequenceReplayBindingAndNavigationIsolation(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 0, 12, 25, 26)
	// point 组装点位所需的数据。
	point := devconfig.Point{X: 6, Y: 8}
	r.Destination = &point
	// send 封装发送对应的后续处理，binding 携带连接上的会话代次，用于防止旧连接命令被接受，seq 记录序号，使用数值表示，id 区分当前步骤指向的目标对象。
	// on 标记是否处于启用状态状态。
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
	// state 保存由模式状态取得的状态。
	state := w.modeState(r, time.Now())
	if state.SceneEpoch != r.Scene || state.PlayerLife != r.Player.Life || len(state.States) < 3 {
		t.Fatal("临时模式全量缺身份/生命代次")
	}
	r.Ready = false
	w.baseline(r)
	// baseline 记录场景基线，使用场景场景基线分片表示。
	var baseline pb.SceneBaselineChunk
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, &baseline); err != nil || baseline.SkillModeState == nil || baseline.SkillModeState.Revision != r.Melee.Revision {
		t.Fatal("重新观察基线没有恢复真实模式")
	}
}

// TestHalfMoonSharedDrawAndFourKillFactsKeepActualImpactTimes 核验多杀应沿各自稳定事件推进SourceSeq，以及半月旁格应共享一次DC样本/没有形成三个独立kill事实。
// t 提供断言失败报告、子用例和清理注册。
func TestHalfMoonSharedDrawAndFourKillFactsKeepActualImpactTimes(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 0, 25)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	r.Role.PrivilegeUntil = now.Add(100 * time.Millisecond)
	r.Role.Equipped = []domain.Equipped{{Item: domain.Item{ItemID: 218}}}
	// definition 保存怪物，用于当前步骤中的定义。
	definition := r.Monsters[0].Monster
	r.Monsters = nil
	// i 定位数值列表中正在处理的成员，direction 接收遍历数值列表时当前的元素。
	for i, direction := range []uint32{7, 0, 1, 2} {
		r.Monsters = append(r.Monsters, &entity{Handle: uint32(10 + i), Kind: 3, Pos: stepPoint(r.Player.Pos, direction, 1), HP: 1, MaxHP: 1, Monster: definition})
	}
	r.MonsterCount = 4
	// dcDraws 规定“dc”“draws”的初始取值。
	dcDraws := 0
	// n 记录数值。
	w.roll = func(n int64) int64 {
		if n == 4 { // 测试木剑2..5与固定本体DC形成唯一四值攻击区间。
			dcDraws++
		}
		return 0
	}
	if w.setSkillMode(r, 25, true, now) != 0 || w.attackFacing(r, 0, 0, 1, now) != 0 || dcDraws != 1 || len(r.KillFacts) != 3 {
		t.Fatal("半月旁格没有共享一次DC样本/没有形成三个独立kill事实")
	}
	// i 定位数值列表中正在处理的成员，target 接收遍历数值列表时当前的元素。
	for i, target := range []uint32{10, 12, 13} {
		// fact 保存从击杀“facts”取出的已发生的事实。
		fact := r.KillFacts[i]
		if fact.Target.Handle != target || !fact.At.Equal(now) || fact.Bound {
			t.Fatal("立即旁格逐杀顺序或受理时权益绑定错误")
		}
	}
	w.advanceEvents(r, now.Add(time.Second))
	if len(r.KillFacts) != 4 || r.KillFacts[3].Target.Handle != 11 || !r.KillFacts[3].At.Equal(now.Add(300*time.Millisecond)) || !r.KillFacts[3].Bound {
		t.Fatal("晚处理前格丢杀/用处理时钟覆盖实际300ms事件时间")
	}
	// i 标记遍历击杀“facts”时访问的下标。
	for i := range r.KillFacts {
		// q 接收击杀结算供后续步骤使用。
		q := w.killSettlement(r, r.KillFacts[i])
		if q.SourceSeq != r.Role.SourceSeq+1 || !q.EventAt.Equal(r.KillFacts[i].At) || q.TargetLife != r.KillFacts[i].Target.Life {
			t.Fatal("多杀没有沿各自稳定事件推进SourceSeq")
		}
		r.Role.SourceSeq = q.SourceSeq
	}
}

// TestAttackBudgetRejectDoesNotConsumeModesManaOrReadiness 核对「有界事件拒绝消费了充能/攻杀/蓝或改变动作」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAttackBudgetRejectDoesNotConsumeModesManaOrReadiness(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用。
	w, r, _ := stageBWorker(t, 0, 7, 12, 25, 26)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	r.Melee.Thrusting, r.Melee.HalfMoon, r.Melee.Slaying, r.Melee.Flaming = true, true, true, true
	r.Melee.FlamingUntil = now.Add(time.Second)
	// state 保存近战，用于当前步骤中的状态，mana 保存魔法值，用于当前步骤中的魔法值。
	state, mana := r.Melee, r.Player.MP
	r.Casts = make([]pendingCast, w.eventCapacity())
	if w.attackFacing(r, 4, 0, 1, now) != p.CodeSkillStateBlocked || r.Melee != state || r.Player.MP != mana || r.Player.AttackSequence != 0 {
		t.Fatal("有界事件拒绝消费了充能/攻杀/蓝或改变动作")
	}
}

// TestSameConnectionReviveAdvancesModeRevisionAndRetainsOriginalModes 核验新life基线与随后全量状态不能解除客户端同步等待，用诊断「同连接复活清模式/重置CD，或未同步推进观察代次」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSameConnectionReviveAdvancesModeRevisionAndRetainsOriginalModes(t *testing.T) {
	// w 接收阶段b房间执行器供后续步骤使用，r 接收阶段b房间执行器供后续步骤使用，c 接收阶段b房间执行器供后续步骤使用。
	w, r, c := stageBWorker(t, 0, 7, 12, 25, 26)
	// now 记录本次处理开始时的时间。
	now := time.Now()
	r.Melee = meleeState{Thrusting: true, HalfMoon: true, Slaying: true, Flaming: true, FlamingUntil: now.Add(5 * time.Second), Revision: 9}
	r.Player.HP, r.DeathID, r.Auto = 0, "old_death", false
	// oldLife 保存生命代次，用于当前步骤中的原状态生命代次，oldRevision 保存修订号，用于当前步骤中的原状态修订号。
	// deadline 保存“flaming”有效截止时间，用于当前步骤中的截止时间。
	oldLife, oldRevision, deadline := r.Player.Life, r.Melee.Revision, r.Melee.FlamingUntil
	w.revivePlayer(r)
	w.snapshot(r)
	w.modes(r, now)
	if r.Player.Life != oldLife+1 || r.Melee.Revision != oldRevision+1 || !r.Melee.Thrusting || !r.Melee.HalfMoon || !r.Melee.Slaying || !r.Melee.Flaming || !r.Melee.FlamingUntil.Equal(deadline) || r.Conn != c || r.Auto {
		t.Fatal("同连接复活清模式/重置CD，或未同步推进观察代次")
	}
	// baseline 记录场景基线，使用场景场景基线分片表示。
	var baseline pb.SceneBaselineChunk
	// mode 记录模式，使用技能模式状态表示。
	var mode pb.SkillModeState
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(c.frames[len(c.frames)-2].Payload, &baseline); err != nil {
		t.Fatal(err)
	}
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, &mode); err != nil {
		t.Fatal(err)
	}
	if baseline.SkillModeState.Revision <= oldRevision || baseline.SkillModeState.PlayerLife != r.Player.Life || mode.Revision != baseline.SkillModeState.Revision || mode.PlayerLife != baseline.SkillModeState.PlayerLife {
		t.Fatal("新life基线与随后全量状态不能解除客户端同步等待")
	}
}
