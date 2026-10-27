package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
	"reflect"
	"testing"
	"time"
)

func stageAWorker(t *testing.T, job uint32) (*worker, *room, *capturedConnection) {
	t.Helper()
	w, r, c := skillWorker(t, job)
	if !w.sourceTimeline() {
		t.Fatal("本组需要唯一Excel导出的技能/战斗时序候选")
	}
	for _, id := range []uint32{1, 2, 3, 4, 5, 11, 23, 29} {
		if _, exists := w.e.Store.Cfg.Fixture.Skill(id); !exists {
			t.Fatal("原8技能回归输入不完整")
		}
	}
	// 本组隔离原8项策略；新阶段技能由各自回归显式授予，不能悄悄改变本组输入。
	old := map[uint32]bool{1: true, 2: true, 3: true, 4: true, 5: true, 11: true, 23: true, 29: true}
	learned := r.Role.Skills[:0]
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
	definition := *r.Monsters[0].Monster
	definition.Undead = false
	r.Monsters[0].Monster = &definition // 普通目标与亡灵分支分别验证。
	return w, r, c
}
func clearAction(r *room) {
	r.Player.ActionReadyAt = time.Time{}
	r.Player.NextMove = time.Time{}
	r.NextSpell = time.Time{}
	r.SkillCooldowns = nil
	r.Casts = nil
	r.Attacks = nil
	r.Player.MP = r.Player.MaxMP
}

func TestHumanAttackActionImpactAndLevelSpeedAreIndependent(t *testing.T) {
	w, r, c := stageAWorker(t, 1)
	now := time.Unix(1800000000, 0)
	m := r.Monsters[0]
	r.Role.Equipped = []domain.Equipped{{Item: domain.Item{ItemID: 218}}}
	timing := w.e.Store.Cfg.Fixture.CombatTiming
	if w.fighter(r, &r.Player).AttackMS != 1030 {
		t.Fatal("31级攻速未按来源370ms封顶")
	}
	w.attack(r, &r.Player, m, now, 7)
	if len(r.Attacks) != 1 || m.HP != 1000 || r.Player.AttackSequence != 1 || c.frames[0].ID != p.MsgS2CAttackAccepted {
		t.Fatal("普攻没有分离受理与延迟伤害")
	}
	mana := r.Player.MP
	if w.cast(r, 1, m.Handle, 8, now.Add(time.Duration(timing.PlayerAttackActionMS)*time.Millisecond-time.Nanosecond)) != p.CodeSkillStateBlocked || r.Player.MP != mana {
		t.Fatal("普攻动作锁内可以施法")
	}
	pos := r.Player.Pos
	w.move(r, &r.Player, devconfig.Point{X: 9, Y: 8}, now.Add(200*time.Millisecond), 600)
	if r.Player.Pos != pos {
		t.Fatal("普攻动作锁内仍可移动")
	}
	w.advanceAttacks(r, r.Attacks[0].Due.Add(-time.Nanosecond))
	if m.HP != 1000 {
		t.Fatal("普通攻击早于300ms命中")
	}
	w.advanceAttacks(r, r.Attacks[0].Due)
	if m.HP >= 1000 {
		t.Fatal("普通攻击到期未造成伤害")
	}
	if w.cast(r, 1, m.Handle, 9, now.Add(time.Duration(timing.PlayerAttackActionMS)*time.Millisecond)) != 0 {
		t.Fatal("550ms边界没有解除施法动作锁")
	}
}

func TestAttackActorRefBlocksNewLifeAndOldScene(t *testing.T) {
	for _, newScene := range []bool{false, true} {
		w, r, c := stageAWorker(t, 1)
		r.Role.Equipped = []domain.Equipped{{Item: domain.Item{ItemID: 218}}}
		now := time.Unix(1800000000, 0)
		w.attack(r, &r.Player, r.Monsters[0], now)
		due := r.Attacks[0].Due
		if newScene {
			r.Scene++
		} else {
			r.Monsters[0].Life++
		}
		c.frames = nil
		w.advanceAttacks(r, due)
		if r.Monsters[0].HP != 1000 {
			t.Fatal("旧普通攻击命中新生命/新场景")
		}
		if newScene {
			for _, f := range c.frames {
				if f.ID == p.MsgS2CCombatImpact {
					t.Fatal("旧场景命中特效泄漏到新场景")
				}
			}
		}
	}
}

func TestReadyOnlyGatesInputAndObservationNotAuthoritativeWorld(t *testing.T) {
	w, r, c := stageAWorker(t, 1)
	now := time.Unix(1800000000, 0)
	if w.cast(r, 1, 10, 1, now) != 0 {
		t.Fatal("准备火球失败")
	}
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
	scene := &pb.SceneBaselineChunk{}
	if err := proto.Unmarshal(c.frames[len(c.frames)-1].Payload, scene); err != nil {
		t.Fatal(err)
	}
	if scene.Entities[0].CastSequence != 1 || scene.Entities[0].Life != r.Player.Life {
		t.Fatal("恢复观察未同步当前代次/施法水位")
	}
}

func TestNewSingleSkillsUseExactPowerFlightAndUndeadRules(t *testing.T) {
	for _, test := range []struct {
		id   uint32
		want int64
	}{{5, 16}, {11, 15}} {
		w, r, _ := stageAWorker(t, 1)
		now := time.Unix(1800000000, 0)
		if code := w.cast(r, test.id, 10, 1, now); code != 0 || r.Casts[0].Amount != test.want {
			t.Fatalf("技能%d来源威力不符:%d", test.id, code)
		}
	}
	w, r, _ := stageAWorker(t, 1)
	r.Monsters[0].Monster = &devconfig.Monster{Fighter: r.Monsters[0].Monster.Fighter, Undead: true}
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

func TestSpiritSwordUsesNonlinearAccuracyAtEveryRank(t *testing.T) {
	w, r, _ := stageAWorker(t, 2)
	for rank, want := range []int64{0, 3, 5, 8} {
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

func TestEnemyGridAreaUsesImpactPositionsAndFiltersFriendsAndInactiveSlots(t *testing.T) {
	for _, d := range directions {
		w, r, _ := stageAWorker(t, 1)
		w.e.Store.Cfg.Fixture.Map.Blocked = nil
		center := devconfig.Point{X: 8 + d.X*2, Y: 8 + d.Y*2}
		def := r.Monsters[0].Monster
		r.Monsters = []*entity{{Handle: 10, Kind: 3, HP: 1000, MaxHP: 1000, Pos: center, Monster: def}, {Handle: 11, Kind: 3, HP: 1000, MaxHP: 1000, Pos: devconfig.Point{X: center.X + 1, Y: center.Y + 1}, Monster: def}, {Handle: 12, Kind: 3, HP: 1000, MaxHP: 1000, Pos: devconfig.Point{X: center.X + 2, Y: center.Y + 2}, Monster: def}, {Handle: 13, Kind: 3, HP: 1000, MaxHP: 1000, Pos: center, Monster: def}}
		r.MonsterCount = 3
		r.Player.HP = 10
		r.Pet.HP = 10
		beforeMP := r.Player.MP
		now := time.Unix(1800000000, 0)
		if code := w.cast(r, 23, 0, 1, now, pointPB(center)); code != 0 {
			t.Fatal(code)
		}
		s, _ := w.e.Store.Cfg.Fixture.Skill(23)
		if r.Player.MP != beforeMP-s.MPBaseCost || r.Monsters[0].HP != 1000 {
			t.Fatal("范围受理提前伤害或错误扣蓝")
		}
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
	w, r, _ := stageAWorker(t, 1)
	now := time.Unix(1800000000, 0)
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

func TestMassHealingUsesIndependentSCPathAndFriendlyQueues(t *testing.T) {
	w, r, _ := stageAWorker(t, 2)
	r.Player.HP = 1
	r.Pet.HP = 1
	r.Monsters[0].HP = 1
	now := time.Unix(1800000000, 0)
	if code := w.cast(r, 29, 0, 1, now, pointPB(r.Player.Pos)); code != 0 || r.Casts[0].Amount != 10 {
		t.Fatal("群疗误用单疗SC乘2或施法者等级")
	}
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

func TestTimelineOrdersDifferentEventKindsAndFreezesPaidFactsAcrossRetry(t *testing.T) {
	w, r, _ := stageAWorker(t, 1)
	now := time.Unix(1800000000, 0)
	m := r.Monsters[0]
	s, _ := w.e.Store.Cfg.Fixture.Skill(1)
	m.HP = 1
	r.Casts = []pendingCast{{Scene: r.Scene, Skill: s, Target: m.Handle, TargetLife: m.Life, TargetPoint: m.Pos, Caster: reference(r, &r.Player), Amount: 1, Due: now.Add(850 * time.Millisecond), ID: 1, Order: 1}}
	r.Attacks = []pendingAttack{{Caster: reference(r, &r.Player), Target: reference(r, m), Amount: 100, Due: now.Add(900 * time.Millisecond), Order: 2}}
	draws := 0
	w.roll = func(int64) int64 { draws++; return 0 }
	r.Role.PrivilegeUntil = now.Add(900 * time.Millisecond)
	w.advanceEvents(r, now.Add(time.Second))
	if len(r.KillFacts) != 1 || r.KillFacts[0].Target.Handle != 10 || r.KillFacts[0].Bound || !r.KillFacts[0].At.Equal(now.Add(850*time.Millisecond)) {
		t.Fatal("混合事件次序/真实killAt/绑定不符")
	}
	snapshot := r.KillFacts[0]
	r.Role.NoEquipmentKills = w.e.Store.Cfg.Fixture.Drops.EquipmentGuarantee - 1
	r.Role.PrivilegeUntil = time.Time{}
	q := w.killSettlement(r, snapshot)
	if q.Item == nil || q.Bound || q.SourceSeq != r.Role.SourceSeq+1 || q.Item.ID != snapshot.InstanceID {
		t.Fatal("逐杀保底/绑定被提交时刻或重抽改变")
	}
	beforeDraws := draws
	w.flushKillFacts(r)
	retry := r.PendingRetry
	if retry == nil || !r.Pending || !r.KillFacts[0].Prepared {
		t.Fatal("队列饱和未保留原经济提交")
	}
	w.step(r, r.RetryAt.Add(time.Millisecond))
	if draws != beforeDraws || r.PendingRetry != retry || len(r.KillFacts) != 1 || !reflect.DeepEqual(snapshot.Roll, r.KillFacts[0].Roll) {
		t.Fatal("未知/提交等待重抽或漏掉kill事实")
	}
}

func TestCombatBudgetsAndInvalidGroundRejectBeforeCosts(t *testing.T) {
	w, r, _ := stageAWorker(t, 1)
	now := time.Unix(1800000000, 0)
	mana := r.Player.MP
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

func TestAutomaticSkillPriorityUsesTargetsAndFallsBackWithoutTouchingNavigation(t *testing.T) {
	w, r, _ := stageAWorker(t, 1)
	now := time.Unix(1800000000, 0)
	if _, handled := w.autoSkill(r, now); !handled || r.Casts[0].Skill.ID != 11 {
		t.Fatal("单怪应按候选优先雷电而非先火球遮蔽新技能")
	}
	clearAction(r)
	s, _ := w.e.Store.Cfg.Fixture.Skill(1)
	r.Player.MP = s.MPBaseCost
	if _, handled := w.autoSkill(r, now); !handled || r.Casts[0].Skill.ID != 1 {
		t.Fatal("缺蓝没有尝试可付出的低阶技能")
	}
	clearAction(r)
	r.Player.MP = 0
	if target, handled := w.autoSkill(r, now); handled || target != nil {
		t.Fatal("无满足技能没有回到普通攻击/追击")
	}
}

func TestLateImpactUsesDueForBothPrivilegeBoundariesAndHostingSegment(t *testing.T) {
	for _, test := range []struct {
		expiry time.Duration
		bound  bool
	}{{900 * time.Millisecond, false}, {800 * time.Millisecond, true}} {
		w, r, _ := stageAWorker(t, 1)
		now := time.Unix(1800000000, 0)
		s, _ := w.e.Store.Cfg.Fixture.Skill(1)
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
	w, r, _ := stageAWorker(t, 1)
	now := time.Unix(1800000000, 0)
	s, _ := w.e.Store.Cfg.Fixture.Skill(1)
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

func TestHardSettlementFailureIsolatesWithoutDiscardingPaidFacts(t *testing.T) {
	w, r, c := stageAWorker(t, 1)
	r.KillFacts = []killFact{{ID: "original", Prepared: true}}
	r.Auto = true
	r.Manual = true
	r.PreviewInFlight = true
	generation := r.PreviewGeneration
	w.isolateRoom(r)
	if !r.Isolated || r.Auto || r.Manual || r.PreviewInFlight || r.PreviewGeneration <= generation || len(r.KillFacts) != 1 || r.KillFacts[0].ID != "original" {
		t.Fatal("硬失败继续战斗、未隔离预览或丢弃原收益事实")
	}
	before := r.Player.Pos
	w.step(r, time.Now())
	if r.Player.Pos != before {
		t.Fatal("隔离room仍推进")
	}
	w.command(command{role: "role", conn: c, ticket: "new-ticket", frame: wire.Frame{Request: 1}})
	if c.frames[len(c.frames)-1].Error != p.CodeLineRecovering {
		t.Fatal("隔离后新票据给旧事实提升授权")
	}
}

func TestPrivilegeRenewalDoesNotRewriteAlreadyDueDamage(t *testing.T) {
	for _, test := range []struct {
		change time.Duration
		bound  bool
	}{{600 * time.Millisecond, true}, {500 * time.Millisecond, false}} {
		w, r, _ := stageAWorker(t, 1)
		now := time.Unix(1800000000, 0)
		r.Role.PrivilegeUntil = now.Add(400 * time.Millisecond)
		r.Monsters[0].HP = 1
		if w.cast(r, 1, 10, 1, now) != 0 {
			t.Fatal("准备续卡边界失败")
		}
		due := r.Casts[0].Due
		r.Role.PrivilegeUntil = now.Add(time.Hour)
		transitionEventPrivilege(r, now.Add(test.change), r.Role.PrivilegeUntil)
		w.advanceEvents(r, now.Add(time.Second))
		if len(r.KillFacts) != 1 || r.KillFacts[0].Bound != test.bound || !r.KillFacts[0].At.Equal(due) {
			t.Fatal("到期后续卡回溯改绑，或到期前续卡没有生效")
		}
	}
}

func TestBlockedAreaCenterStillAffectsWalkableNeighboursAndClipsMapEdges(t *testing.T) {
	for _, job := range []uint32{1, 2} {
		w, r, _ := stageAWorker(t, job)
		w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{{X: 0, Y: 0}}
		r.Player.Pos = devconfig.Point{X: 1, Y: 0}
		r.Player.HP = 1
		r.Pet.Pos = devconfig.Point{X: 0, Y: 1}
		r.Pet.HP = 1
		r.Monsters[0].Pos = devconfig.Point{X: 1, Y: 1}
		id := uint32(23)
		if job == 2 {
			id = 29
		}
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
	w, r, _ := stageAWorker(t, 1)
	r.Player.Pos = devconfig.Point{X: 14, Y: 14}
	r.Monsters[0].Pos = devconfig.Point{X: 15, Y: 15}
	now := time.Unix(1800000000, 0)
	if w.cast(r, 23, 0, 1, now, &pb.GridPoint{X: 15, Y: 15}) != 0 {
		t.Fatal("地图右下边缘中心被拒绝")
	}
	w.advanceEvents(r, r.Casts[0].Due)
	if r.Monsters[0].HP >= 1000 {
		t.Fatal("边缘3×3裁切漏掉有效目标")
	}
}
