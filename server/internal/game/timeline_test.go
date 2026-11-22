// 本文件回归战斗事件的实际到期时刻、排序、目标身份、资源预算和异常隔离。
package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"testing"
	"time"
)

// stageAWorker 处理阶段a房间执行器。
// t 提供断言失败报告、子用例和清理注册，job 记录任务或职业编号，使用数值表示。
func stageAWorker(t *testing.T, job uint32) (*worker, *room, *capturedConnection) {
	t.Helper()
	// w 接收技能房间执行器供后续步骤使用，r 接收技能房间执行器供后续步骤使用，c 接收技能房间执行器供后续步骤使用。
	w, r, c := skillWorker(t, job)
	if !w.sourceTimeline() {
		t.Fatal("本组需要唯一Excel导出的技能/战斗时序候选")
	}
	// id 接收遍历数值列表时当前的元素。
	for _, id := range []uint32{1, 2, 3, 4, 5, 11, 23, 29} {
		// exists 保存由技能取得的存在。
		if _, exists := w.e.Store.Cfg.Fixture.Skill(id); !exists {
			t.Fatal("原8技能回归输入不完整")
		}
	}
	// 本组隔离原8项策略；新阶段技能由各自回归显式授予，不能悄悄改变本组输入。
	// old 初始化用作原状态的按数值索引的布尔状态映射。
	old := map[uint32]bool{1: true, 2: true, 3: true, 4: true, 5: true, 11: true, 23: true, 29: true}
	// learned 保存当前处理所用的已学习。
	learned := r.Role.Skills[:0]
	// s 接收遍历技能集合时当前的元素。
	for _, s := range r.Role.Skills {
		if old[s.SkillID] {
			learned = append(learned, s)
		}
	}
	r.Role.Skills = learned
	r.Role.Level = 31
	w.refreshPlayerStats(r)
	r.Player.HP, r.Player.MP = r.Player.MaxHP, r.Player.MaxMP
	r.Monsters[0].HP, r.Monsters[0].MaxHP = 1000, 1000
	// definition 保存当前处理所用的定义。
	definition := *r.Monsters[0].Monster
	definition.Undead = false
	r.Monsters[0].Monster = &definition // 普通目标与亡灵分支分别验证。
	return w, r, c
}

// clearAction 清除“action”。
// r 指向当前执行器拥有并更新的目标房间。
func clearAction(r *room) {
	r.Player.ActionReadyAt = time.Time{}
	r.Player.NextMove = time.Time{}
	r.NextSpell = time.Time{}
	r.SkillCooldowns = nil
	r.Casts = nil
	r.Attacks = nil
	r.Player.MP = r.Player.MaxMP
}

// TestHumanAttackActionImpactAndLevelSpeedAreIndependent 核验550ms边界应解除施法动作锁，以及普攻应分离受理与延迟伤害。
// t 提供断言失败报告、子用例和清理注册。
func TestHumanAttackActionImpactAndLevelSpeedAreIndependent(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用，c 接收阶段a房间执行器供后续步骤使用。
	w, r, c := stageAWorker(t, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// m 保存从怪物集合取出的怪物集合中的指定项。
	m := r.Monsters[0]
	r.Role.Equipped = []domain.Equipped{{Item: domain.Item{ItemID: 218}}}
	// timing 保存战斗时序，用于当前步骤中的时序。
	timing := w.e.Store.Cfg.Fixture.CombatTiming
	if w.fighter(r, &r.Player).AttackMS != 1030 {
		t.Fatal("31级攻速未按来源370ms封顶")
	}
	w.attack(r, &r.Player, m, now, 7)
	if len(r.Attacks) != 1 || m.HP != 1000 || r.Player.AttackSequence != 1 || c.frames[0].ID != p.MsgS2CAttackAccepted {
		t.Fatal("普攻没有分离受理与延迟伤害")
	}
	// mana 保存魔法值，用于当前步骤中的魔法值。
	mana := r.Player.MP
	if w.cast(r, 1, m.Handle, 8, now.Add(time.Duration(timing.PlayerAttackActionMS)*time.Millisecond-time.Nanosecond)) != p.CodeSkillStateBlocked || r.Player.MP != mana {
		t.Fatal("普攻动作锁内可以施法")
	}
	// pos 保存当前处理所用的位置。
	pos := r.Player.Pos
	w.move(r, &r.Player, devconfig.Point{X: 9, Y: 8}, now.Add(200*time.Millisecond), 600)
	if r.Player.Pos != pos {
		t.Fatal("普攻动作锁内仍可移动")
	}
	w.advanceEvents(r, r.Attacks[0].Due.Add(-time.Nanosecond))
	if m.HP != 1000 {
		t.Fatal("普通攻击早于300ms命中")
	}
	w.advanceEvents(r, r.Attacks[0].Due)
	if m.HP >= 1000 {
		t.Fatal("普通攻击到期未造成伤害")
	}
	if w.cast(r, 1, m.Handle, 9, now.Add(time.Duration(timing.PlayerAttackActionMS)*time.Millisecond)) != 0 {
		t.Fatal("550ms边界没有解除施法动作锁")
	}
}

// TestAttackActorRefBlocksNewLifeAndWorldButAttachmentSceneOnlyGatesObservation 核对「旧场景命中特效泄漏到新场景」与「执行引用未隔离新生命/新World，或连接Scene错误取消了原World动作」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestAttackActorRefBlocksNewLifeAndWorldButAttachmentSceneOnlyGatesObservation(t *testing.T) {
	// change 接收遍历文本列表时当前的元素。
	for _, change := range []string{"life", "world", "scene"} {
		// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用，c 接收阶段a房间执行器供后续步骤使用。
		w, r, c := stageAWorker(t, 1)
		r.Role.Equipped = []domain.Equipped{{Item: domain.Item{ItemID: 218}}}
		// now 保存由Unix 时间取得的当前时间。
		now := time.Unix(1800000000, 0)
		w.attack(r, &r.Player, r.Monsters[0], now)
		// due 保存当前处理所用的已到期。
		due := r.Attacks[0].Due
		if change == "scene" {
			r.Scene++
		} else if change == "world" {
			r.currentWorld().Ref.ID++
		} else {
			r.Monsters[0].Life++
		}
		c.frames = nil
		w.advanceEvents(r, due)
		if change == "scene" && r.Monsters[0].HP >= 1000 || change != "scene" && r.Monsters[0].HP != 1000 {
			t.Fatal("执行引用未隔离新生命/新World，或连接Scene错误取消了原World动作")
		}
		if change == "scene" {
			// f 接收遍历协议帧集合时当前的元素。
			for _, f := range c.frames {
				if f.ID == p.MsgS2CCombatImpact {
					t.Fatal("旧场景命中特效泄漏到新场景")
				}
			}
		}
	}
}

// TestReadyOnlyGatesInputAndObservationNotAuthoritativeWorld 核验观察ACK前输入应门禁，用诊断「观察未就绪时发送战斗特效/基线」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestReadyOnlyGatesInputAndObservationNotAuthoritativeWorld(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用，c 接收阶段a房间执行器供后续步骤使用。
	w, r, c := stageAWorker(t, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	if w.cast(r, 1, 10, 1, now) != 0 {
		t.Fatal("准备火球失败")
	}
	// due 保存当前处理所用的已到期。
	due := r.Casts[0].Due
	r.Ready = false
	r.AwaitUntil = due.Add(time.Second)
	c.frames = nil
	r.Auto = false
	w.step(r, due)
	if r.Monsters[0].HP >= 1000 || len(r.Casts) != 0 {
		t.Fatal("等待基线ACK暂停已释放法术")
	}
	if len(c.frames) != 0 {
		t.Fatal("观察未就绪时发送战斗特效/基线")
	}
	w.command(command{role: "role", conn: c, frame: wire.Message(p.MsgC2SMoveIntent, 1, 3, &pb.MoveIntent{SceneEpoch: r.Scene, InputSequence: 2, Direction: 2})})
	if c.frames[len(c.frames)-1].Error != p.CodeLineRecovering || r.Manual {
		t.Fatal("观察ACK前输入没有门禁")
	}
	r.Ready = true
	w.snapshot(r)
	// scene 保存当前处理所用的场景。
	scene := &pb.SceneBaselineChunk{}
	// err 接收反序列化过程中可能产生的错误，供后续失败分支处理。
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, scene); err != nil {
		t.Fatal(err)
	}
	if scene.Entities[0].CastSequence != 1 || scene.Entities[0].Life != r.Player.Life {
		t.Fatal("恢复观察未同步当前代次/施法水位")
	}
}

// TestNewSingleSkillsUseExactPowerFlightAndUndeadRules 核对「大火球无视CanFly阻挡」与「雷电亡灵1.5倍未截断」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestNewSingleSkillsUseExactPowerFlightAndUndeadRules(t *testing.T) {
	// test 接收遍历临时结构列表时当前的元素。
	for _, test := range []struct {
		// id 区分当前步骤指向的目标对象。
		id uint32
		// want 记录期望值，使用数值表示。
		want int64
	}{{5, 16}, {11, 15}} {
		// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
		w, r, _ := stageAWorker(t, 1)
		// now 保存由Unix 时间取得的当前时间。
		now := time.Unix(1800000000, 0)
		// code 保存由施法取得的结果码。
		if code := w.cast(r, test.id, 10, 1, now); code != 0 || r.Casts[0].Amount != test.want {
			t.Fatalf("技能%d来源威力不符:%d", test.id, code)
		}
	}
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 1)
	r.Monsters[0].Monster = &devconfig.Monster{Fighter: r.Monsters[0].Monster.Fighter, Undead: true}
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	if w.cast(r, 11, 10, 1, now) != 0 || r.Casts[0].Amount != 22 {
		t.Fatal("雷电亡灵1.5倍未截断")
	}
	clearAction(r)
	r.Monsters[0].Pos = devconfig.Point{X: 8, Y: 6}
	w.e.Store.Cfg.Fixture.Map.Blocked = append(w.e.Store.Cfg.Fixture.Map.Blocked, devconfig.Point{X: 8, Y: 7})
	if w.cast(r, 5, 10, 2, now) != p.CodeSkillTarget {
		t.Fatal("大火球无视CanFly阻挡")
	}
	if w.cast(r, 11, 10, 3, now) != 0 {
		t.Fatal("雷电误套火球飞行阻挡")
	}
}

// TestSpiritSwordUsesNonlinearAccuracyAtEveryRank 核对「精神力阶准确错误」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestSpiritSwordUsesNonlinearAccuracyAtEveryRank(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 2)
	// rank 定位数值列表中正在处理的成员，want 接收遍历数值列表时当前的元素。
	for rank, want := range []int64{0, 3, 5, 8} {
		// i 定位技能集合中正在处理的成员，s 接收遍历技能集合时当前的元素。
		for i, s := range r.Role.Skills {
			if s.SkillID == 4 {
				r.Role.Skills[i].Level = uint32(rank)
			}
		}
		if w.fighter(r, &r.Player).Accuracy != 5+want {
			t.Fatalf("精神力%d阶准确错误", rank)
		}
	}
}

// TestEnemyGridAreaUsesImpactPositionsAndFiltersFriendsAndInactiveSlots 核验地面AOE应使用到期位置，用诊断「3×3边界/友方/未激活过滤不符」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestEnemyGridAreaUsesImpactPositionsAndFiltersFriendsAndInactiveSlots(t *testing.T) {
	// d 接收遍历朝向集合时当前的元素。
	for _, d := range directions {
		// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
		w, r, _ := stageAWorker(t, 1)
		w.e.Store.Cfg.Fixture.Map.Blocked = nil
		// center 建立带初始值的点位，供本段继续处理。
		center := devconfig.Point{X: 8 + d.X*2, Y: 8 + d.Y*2}
		// def 保存怪物，用于当前步骤中的“def”。
		def := r.Monsters[0].Monster
		r.Monsters = []*entity{{Handle: 10, Kind: 3, HP: 1000, MaxHP: 1000, Pos: center, Monster: def}, {Handle: 11, Kind: 3, HP: 1000, MaxHP: 1000, Pos: devconfig.Point{X: center.X + 1, Y: center.Y + 1}, Monster: def}, {Handle: 12, Kind: 3, HP: 1000, MaxHP: 1000, Pos: devconfig.Point{X: center.X + 2, Y: center.Y + 2}, Monster: def}, {Handle: 13, Kind: 3, HP: 1000, MaxHP: 1000, Pos: center, Monster: def}}
		r.MonsterCount = 3
		r.Player.HP = 10
		r.Pet.HP = 10
		// beforeMP 保存魔法值，用于当前步骤中的操作前的状态魔法值。
		beforeMP := r.Player.MP
		// now 保存由Unix 时间取得的当前时间。
		now := time.Unix(1800000000, 0)
		// code 保存由施法取得的结果码。
		if code := w.cast(r, 23, 0, 1, now, pointPB(center)); code != 0 {
			t.Fatal(code)
		}
		// s 接收技能供后续步骤使用。
		s, _ := w.e.Store.Cfg.Fixture.Skill(23)
		if r.Player.MP != beforeMP-s.MPBaseCost || r.Monsters[0].HP != 1000 {
			t.Fatal("范围受理提前伤害或错误扣蓝")
		}
		// due 保存当前处理所用的已到期。
		due := r.Casts[0].Due
		w.advanceEvents(r, due.Add(-time.Nanosecond))
		if r.Monsters[0].HP != 1000 {
			t.Fatal("爆裂提前命中")
		}
		w.advanceEvents(r, due)
		if r.Monsters[0].HP >= 1000 || r.Monsters[1].HP >= 1000 || r.Monsters[2].HP != 1000 || r.Monsters[3].HP != 1000 || r.Player.HP != 10 || r.Pet.HP != 10 {
			t.Fatal("3×3边界/友方/未激活过滤不符")
		}
	}
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// grid 初始化用作网格的点位。
	grid := devconfig.Point{X: 8, Y: 9}
	if w.cast(r, 23, 0, 1, now, pointPB(grid)) != 0 {
		t.Fatal("准备爆裂失败")
	}
	r.Monsters[0].Pos = devconfig.Point{X: 14, Y: 14}
	w.advanceEvents(r, r.Casts[0].Due)
	if r.Monsters[0].HP != 1000 {
		t.Fatal("地面AOE没有使用到期位置")
	}
}

// TestMassHealingUsesIndependentSCPathAndFriendlyQueues 核验群疗应复用玩家动态/宠物固定分跳，用诊断「群疗提前加HP或友方过滤/队列不符」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestMassHealingUsesIndependentSCPathAndFriendlyQueues(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 2)
	r.Player.HP = 1
	r.Pet.HP = 1
	r.Monsters[0].HP = 1
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// code 保存由施法取得的结果码。
	if code := w.cast(r, 29, 0, 1, now, pointPB(r.Player.Pos)); code != 0 || r.Casts[0].Amount != 10 {
		t.Fatal("群疗误用单疗SC乘2或施法者等级")
	}
	// due 保存当前处理所用的已到期。
	due := r.Casts[0].Due
	w.advanceEvents(r, due)
	if r.Player.HP != 1 || r.Pet.HP != 1 || r.Player.HealAmount != 10 || r.Pet.HealAmount != 10 || r.Monsters[0].HealAmount != 0 {
		t.Fatal("群疗提前加HP或友方过滤/队列不符")
	}
	w.tickHealing(r, due)
	if r.Player.HP != 10 || r.Pet.HP != 6 {
		t.Fatal("群疗没有复用玩家动态/宠物固定分跳")
	}
}

// TestTimelineOrdersDifferentEventKindsAndFreezesPaidFactsAcrossRetry 核对「混合事件次序/真实killAt/绑定不符」与「逐杀保底/绑定被提交时刻或重抽改变」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestTimelineOrdersDifferentEventKindsAndFreezesPaidFactsAcrossRetry(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// m 保存从怪物集合取出的怪物集合中的指定项。
	m := r.Monsters[0]
	// s 接收技能供后续步骤使用。
	s, _ := w.e.Store.Cfg.Fixture.Skill(1)
	m.HP = 1
	r.Casts = []pendingCast{{Scene: r.Scene, Skill: s, Target: m.Handle, TargetLife: m.Life, TargetPoint: m.Pos, Caster: reference(r, &r.Player), Amount: 1, Due: now.Add(850 * time.Millisecond), ID: 1, Order: 1}}
	r.Attacks = []pendingAttack{{Caster: reference(r, &r.Player), Target: reference(r, m), Amount: 100, Due: now.Add(900 * time.Millisecond), Order: 2}}
	// draws 规定“draws”的初始取值。
	draws := 0
	w.roll = func(int64) int64 { draws++; return 0 }
	r.Role.PrivilegeUntil = now.Add(900 * time.Millisecond)
	w.advanceEvents(r, now.Add(time.Second))
	if len(r.KillFacts) != 1 || r.KillFacts[0].Target.Handle != 10 || r.KillFacts[0].Bound || !r.KillFacts[0].At.Equal(now.Add(850*time.Millisecond)) {
		t.Fatal("混合事件次序/真实killAt/绑定不符")
	}
	// snapshot 保存从击杀“facts”取出的快照。
	snapshot := r.KillFacts[0]
	r.Role.NoEquipmentKills = w.e.Store.Cfg.Fixture.Drops.EquipmentGuarantee - 1
	r.Role.PrivilegeUntil = time.Time{}
	// q 接收击杀结算供后续步骤使用。
	q := w.killSettlement(r, snapshot)
	if q.Item == nil || q.Bound || q.SourceSeq != r.Role.SourceSeq+1 || q.Item.ID != snapshot.InstanceID {
		t.Fatal("逐杀保底/绑定被提交时刻或重抽改变")
	}
	// beforeDraws 保存当前处理所用的操作前的状态“draws”。
	beforeDraws := draws
	w.flushKillFacts(r)
	// retry 保存待按原身份重试的操作，用于当前步骤中的重试。
	retry := r.PendingRetry
	if retry == nil || !r.Pending || !r.KillFacts[0].Prepared {
		t.Fatal("队列饱和未保留原经济提交")
	}
	w.step(r, r.RetryAt.Add(time.Millisecond))
	if draws != beforeDraws || r.PendingRetry != retry || len(r.KillFacts) != 1 || !reflect.DeepEqual(snapshot.Roll, r.KillFacts[0].Roll) {
		t.Fatal("未知/提交等待重抽或漏掉kill事实")
	}
}

// TestCombatBudgetsAndInvalidGroundRejectBeforeCosts 核对「非法/缺地格消耗蓝或受理」与「事件队列满仍扣费」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestCombatBudgetsAndInvalidGroundRejectBeforeCosts(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// mana 保存魔法值，用于当前步骤中的魔法值。
	mana := r.Player.MP
	// grid 接收遍历网格点位列表时当前的元素。
	for _, grid := range []*pb.GridPoint{nil, {X: -1, Y: 8}, {X: 30, Y: 8}} {
		if w.cast(r, 23, 0, 1, now, grid) != p.CodeSkillTarget || r.Player.MP != mana {
			t.Fatal("非法/缺地格消耗蓝或受理")
		}
	}
	r.Casts = make([]pendingCast, w.eventCapacity())
	if w.cast(r, 1, 10, 2, now) != p.CodeSkillStateBlocked || r.Player.MP != mana {
		t.Fatal("事件队列满仍扣费")
	}
	r.Casts = nil
	r.KillFacts = make([]killFact, w.e.Store.Cfg.Fixture.CombatTiming.KillQueueCapacity)
	if w.cast(r, 23, 0, 3, now, pointPB(r.Player.Pos)) != p.CodeSkillStateBlocked || r.Player.MP != mana {
		t.Fatal("逐杀预算满仍受理")
	}
}

// TestAutomaticSkillPriorityUsesTargetsAndFallsBackWithoutTouchingNavigation 核验无满足技能时应回到原地普攻或等待，以及缺蓝应尝试可付出的低阶技能。
// t 提供断言失败报告、子用例和清理注册。
func TestAutomaticSkillPriorityUsesTargetsAndFallsBackWithoutTouchingNavigation(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	if !w.autoSkill(r, now) || r.Casts[0].Skill.ID != 11 {
		t.Fatal("单怪应按候选优先雷电而非先火球遮蔽新技能")
	}
	clearAction(r)
	// s 接收技能供后续步骤使用。
	s, _ := w.e.Store.Cfg.Fixture.Skill(1)
	r.Player.MP = s.MPBaseCost
	if !w.autoSkill(r, now) || r.Casts[0].Skill.ID != 1 {
		t.Fatal("缺蓝没有尝试可付出的低阶技能")
	}
	clearAction(r)
	r.Player.MP = 0
	if w.autoSkill(r, now) {
		t.Fatal("无满足技能时没有回到原地普攻或等待")
	}
}

// TestLateImpactUsesDueForBothPrivilegeBoundariesAndHostingSegment 核对「迟处理按执行时刻换绑定或错记后来的托管区段」与「在线重连改写已经到期事件的原托管归属」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLateImpactUsesDueForBothPrivilegeBoundariesAndHostingSegment(t *testing.T) {
	// test 接收遍历临时结构列表时当前的元素。
	for _, test := range []struct {
		// expiry 记录过期时间，使用时长表示。
		expiry time.Duration
		// bound 标记是否处于绑定状态。
		bound bool
	}{{900 * time.Millisecond, false}, {800 * time.Millisecond, true}} {
		// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
		w, r, _ := stageAWorker(t, 1)
		// now 保存由Unix 时间取得的当前时间。
		now := time.Unix(1800000000, 0)
		// s 接收技能供后续步骤使用。
		s, _ := w.e.Store.Cfg.Fixture.Skill(1)
		// m 保存从怪物集合取出的怪物集合中的指定项。
		m := r.Monsters[0]
		m.HP = 1
		r.Role.PrivilegeUntil = now.Add(test.expiry)
		r.Casts = []pendingCast{{Scene: r.Scene, Skill: s, Target: 10, TargetLife: m.Life, TargetPoint: m.Pos, Caster: reference(r, &r.Player), Amount: 1, Due: now.Add(850 * time.Millisecond), ID: 1, Order: 1}}
		transitionEventHosting(r, now.Add(900*time.Millisecond), "late-offline")
		r.Conn = nil
		r.HostSession = "late-offline"
		w.advanceEvents(r, now.Add(time.Second))
		if len(r.KillFacts) != 1 || r.KillFacts[0].Bound != test.bound || r.KillFacts[0].HostSession != "" || !r.KillFacts[0].At.Equal(now.Add(850*time.Millisecond)) {
			t.Fatal("迟处理按执行时刻换绑定或错记后来的托管区段")
		}
	}
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 1)
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	// s 接收技能供后续步骤使用。
	s, _ := w.e.Store.Cfg.Fixture.Skill(1)
	// m 保存从怪物集合取出的怪物集合中的指定项。
	m := r.Monsters[0]
	m.HP = 1
	r.Casts = []pendingCast{{Scene: r.Scene, Skill: s, Target: 10, TargetPoint: m.Pos, Caster: reference(r, &r.Player), Amount: 1, Due: now.Add(850 * time.Millisecond)}}
	transitionEventHosting(r, now.Add(800*time.Millisecond), "original-host")
	transitionEventHosting(r, now.Add(900*time.Millisecond), "")
	w.advanceEvents(r, now.Add(time.Second))
	if r.KillFacts[0].HostSession != "original-host" {
		t.Fatal("在线重连改写已经到期事件的原托管归属")
	}
}

// TestLateFatalAttackUsesHostingSegmentAtImpactDeadline 核验待决结束后原攻击应致死，用诊断「原世界、托管区段或到期时间发生变化」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestLateFatalAttackUsesHostingSegmentAtImpactDeadline(t *testing.T) {
	// check 接收遍历临时结构列表时当前的元素。
	for _, check := range []struct {
		// name 记录名称，使用文本表示。
		// dueSession 记录已到期会话，使用文本表示。
		// laterSession 记录“later”会话，使用文本表示。
		name, dueSession, laterSession string
	}{
		{name: "online_then_offline", laterSession: "later-offline"},
		{name: "offline_then_reconnect", dueSession: "original-host"},
	} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(check.name, func(t *testing.T) {
			// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用，c 接收阶段a房间执行器供后续步骤使用。
			w, r, c := stageAWorker(t, 1)
			// now 保存由Unix 时间取得的当前时间。
			now := time.Unix(1800000000, 0)
			// due 保存由新增取得的已到期。
			due := now.Add(300 * time.Millisecond)
			r.Player.HP = 1
			if check.dueSession != "" {
				r.Conn, r.HostSession = nil, check.dueSession
			}
			r.Attacks = []pendingAttack{{Scene: r.Scene, Caster: reference(r, r.Monsters[0]), Target: reference(r, &r.Player), Due: due, Amount: 10000, Defence: defenceAC, HostSession: r.HostSession}}
			r.Pending = true
			w.advanceEvents(r, due)
			if r.Player.HP != 1 || len(r.KillFacts) != 0 {
				t.Fatal("数据库待决期间处理了致死事件")
			}
			r.Conn, r.HostSession = c, check.laterSession
			if check.laterSession != "" {
				r.Conn = nil
			}
			transitionEventHosting(r, due.Add(time.Millisecond), r.HostSession)
			r.Pending = false
			w.advanceEvents(r, due.Add(time.Second))
			if r.Player.HP != 0 || r.DeathID == "" {
				t.Fatal("待决结束后原攻击没有致死")
			}
			if check.dueSession == "" {
				if len(r.KillFacts) != 0 {
					t.Fatal("在线致死事件误写成后续托管死亡")
				}
				return
			}
			if len(r.KillFacts) != 1 {
				t.Fatalf("重连后应保留一条原托管死亡事实，得到 %d 条", len(r.KillFacts))
			}
			// fact 保存从击杀“facts”取出的已发生的事实。
			fact := r.KillFacts[0]
			// q 接收击杀结算供后续步骤使用。
			q := w.killSettlement(r, fact)
			if fact.World != r.currentWorld().Ref || q.HostingSession != check.dueSession || q.StopReason != 2 || !q.EventAt.Equal(due) || q.Kills != 0 || q.HostSeconds != 0 {
				t.Fatalf("原世界、托管区段或到期时间发生变化: %+v", q)
			}
		})
	}
}

// TestHardSettlementFailureIsolatesWithoutDiscardingPaidFacts 核对「硬失败继续战斗、未隔离预览或丢弃原收益事实」与「隔离后新票据给旧事实提升授权」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestHardSettlementFailureIsolatesWithoutDiscardingPaidFacts(t *testing.T) {
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用，c 接收阶段a房间执行器供后续步骤使用。
	w, r, c := stageAWorker(t, 1)
	r.KillFacts = []killFact{{ID: "original", Prepared: true}}
	r.Auto = true
	r.Manual = true
	r.PreviewInFlight = true
	// generation 保存预览代次，用于当前步骤中的代次。
	generation := r.PreviewGeneration
	w.isolateRoom(r)
	if !r.Isolated || r.Auto || r.Manual || r.PreviewInFlight || r.PreviewGeneration <= generation || len(r.KillFacts) != 1 || r.KillFacts[0].ID != "original" {
		t.Fatal("硬失败继续战斗、未隔离预览或丢弃原收益事实")
	}
	// before 保存位置，用于当前步骤中的操作前的状态。
	before := r.Player.Pos
	w.step(r, time.Now())
	if r.Player.Pos != before {
		t.Fatal("隔离room仍推进")
	}
	w.command(command{role: "role", conn: c, ticket: &persistence.Ticket{}, frame: wire.Frame{Request: 1}})
	if c.frames[len(c.frames)-1].Error != p.CodeLineRecovering {
		t.Fatal("隔离后新票据给旧事实提升授权")
	}
}

// TestPrivilegeRenewalDoesNotRewriteAlreadyDueDamage 核验到期后续卡回溯改绑，或到期前续卡应生效，用诊断「准备续卡边界失败」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestPrivilegeRenewalDoesNotRewriteAlreadyDueDamage(t *testing.T) {
	// test 接收遍历临时结构列表时当前的元素。
	for _, test := range []struct {
		// change 记录变更，使用时长表示。
		change time.Duration
		// bound 标记是否处于绑定状态。
		bound bool
	}{{600 * time.Millisecond, true}, {500 * time.Millisecond, false}} {
		// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
		w, r, _ := stageAWorker(t, 1)
		// now 保存由Unix 时间取得的当前时间。
		now := time.Unix(1800000000, 0)
		r.Role.PrivilegeUntil = now.Add(400 * time.Millisecond)
		r.Monsters[0].HP = 1
		if w.cast(r, 1, 10, 1, now) != 0 {
			t.Fatal("准备续卡边界失败")
		}
		// due 保存当前处理所用的已到期。
		due := r.Casts[0].Due
		r.Role.PrivilegeUntil = now.Add(time.Hour)
		transitionEventPrivilege(r, now.Add(test.change), r.Role.PrivilegeUntil)
		w.advanceEvents(r, now.Add(time.Second))
		if len(r.KillFacts) != 1 || r.KillFacts[0].Bound != test.bound || !r.KillFacts[0].At.Equal(due) {
			t.Fatal("到期后续卡回溯改绑，或到期前续卡没有生效")
		}
	}
}

// TestBlockedAreaCenterStillAffectsWalkableNeighboursAndClipsMapEdges 核对「阻挡中心未治疗邻格友方或误影响敌方」与「合法地图边界内的阻挡中心被拒绝」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestBlockedAreaCenterStillAffectsWalkableNeighboursAndClipsMapEdges(t *testing.T) {
	// job 接收遍历数值列表时当前的元素。
	for _, job := range []uint32{1, 2} {
		// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
		w, r, _ := stageAWorker(t, job)
		w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 0, Y: 0}}
		r.Player.Pos = devconfig.Point{X: 1, Y: 0}
		r.Player.HP = 1
		r.Pet.Pos = devconfig.Point{X: 0, Y: 1}
		r.Pet.HP = 1
		r.Monsters[0].Pos = devconfig.Point{X: 1, Y: 1}
		// id 区分当前步骤指向的目标对象。
		id := uint32(23)
		if job == 2 {
			id = 29
		}
		// now 保存由Unix 时间取得的当前时间。
		now := time.Unix(1800000000, 0)
		if w.cast(r, id, 0, 1, now, &pb.GridPoint{X: 0, Y: 0}) != 0 {
			t.Fatal("合法地图边界内的阻挡中心被拒绝")
		}
		w.advanceEvents(r, r.Casts[0].Due)
		if job == 1 {
			if r.Monsters[0].HP >= 1000 || r.Player.HP != 1 || r.Pet.HP != 1 {
				t.Fatal("阻挡中心未伤邻格敌方或误伤友方")
			}
		} else if r.Player.HealAmount == 0 || r.Pet.HealAmount == 0 || r.Monsters[0].HP != 1000 {
			t.Fatal("阻挡中心未治疗邻格友方或误影响敌方")
		}
	}
	// w 接收阶段a房间执行器供后续步骤使用，r 接收阶段a房间执行器供后续步骤使用。
	w, r, _ := stageAWorker(t, 1)
	// f 保存当前处理所用的开发测试配置。
	f := &w.e.Store.Cfg.Fixture
	f.Map.Blocked = nil
	// edge 初始化用作边缘的点位。
	edge := devconfig.Point{X: f.Map.Width - 1, Y: f.Map.Height - 1}
	r.Player.Pos = devconfig.Point{X: edge.X - 1, Y: edge.Y - 1}
	r.Monsters[0].Pos = edge
	// now 保存由Unix 时间取得的当前时间。
	now := time.Unix(1800000000, 0)
	if w.cast(r, 23, 0, 1, now, pointPB(edge)) != 0 {
		t.Fatal("地图右下边缘中心被拒绝")
	}
	w.advanceEvents(r, r.Casts[0].Due)
	if r.Monsters[0].HP >= 1000 {
		t.Fatal("边缘3×3裁切漏掉有效目标")
	}
}
