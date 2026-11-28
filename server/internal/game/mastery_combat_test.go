// 本文件对拍真实战斗伤害、角色时钟、六种抗性和原协议反馈。
package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestMasteryDirectHitAndPeriodicResistance 使用镜像的真实两侧身体验证三个独立结算位置。
func TestMasteryDirectHitAndPeriodicResistance(t *testing.T) {
	w, config, fighters, at := mirrorCombatFixture(t)
	fixture := &w.e.Store.Cfg.Fixture
	fixture.ExpandedHitBudget = &devconfig.ExpandedHitBudget{DamageCapBP: 2000, AbsorptionCapBP: 2000}
	fixture.MasteryCombat = &devconfig.MasteryCombat{CriticalCapBP: 1200, CriticalExtraBP: 2500, AttackSpeedCapBP: 600, ResistanceCapBP: 2000}
	fighters[0].Fighter.MasteryBonuses.CriticalBP = 100
	fighters[1].Fighter.HP = 800
	fighters[1].Fighter.DefenceMin, fighters[1].Fighter.DefenceMax = 20, 20
	fighters[1].Fighter.MagicDefenceMin, fighters[1].Fighter.MagicDefenceMax = 20, 20
	fighters[1].Fighter.MasteryBonuses.ResistanceBP = 1000
	fighters[1].Fighter.LuckResistancePPM = 300_000
	battle := w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 77)
	if battle == nil {
		t.Fatal("未能打开双角色战斗")
	}
	left, right := battle.sides[0].room, battle.sides[1].room
	world := left.currentWorld()
	w.roll = func(n int64) int64 { return min(n-1, 99) }
	attack := pendingAttack{Caster: referenceIn(world, &left.Player), Target: referenceIn(world, &right.Player),
		Scene: left.Scene, Due: at, Amount: 120, Defence: defenceAC, Sequence: 1}
	w.resolveAttack(left, attack, at)
	expectMasteryImpact(t, battle, &right.Player, 675, 125, true)

	// 满足了命中、护甲和净伤条件仍要过独立概率，等于阈值时只扣原伤。
	w.roll = func(n int64) int64 { return min(n-1, 100) }
	attack.Sequence++
	w.resolveAttack(left, attack, at)
	expectMasteryImpact(t, battle, &right.Player, 575, 100, false)
	// 幸运读取对方冻结的抵抗，300000 减去后本次取原区间结果。
	if got := w.damageWithLuck(left, &right.Player, 80, luckyDamage{attack: 500_000, roll: 200_000, upper: 120}); got != 80 {
		t.Fatal("幸运抵抗没有参与原区间上下限选择", got)
	}

	// 火墙沿已确认的原魔防、剩余灼烧伤害计算，每次周期触发仍是一条结算。
	w.fireWallHit(left, world, &left.Player, &right.Player,
		&fireWallField{Scene: left.Scene, SkillID: 22, Amount: 120}, at, hitContext{At: at})
	expectMasteryImpact(t, battle, &right.Player, 485, 90, false)
	poison := pendingCast{Caster: referenceIn(world, &left.Player), Target: right.Player.Handle, TargetLife: right.Player.Life,
		Scene: left.Scene, Due: at, Amount: 50, PoisonColor: devconfig.PoisonGreen}
	w.resolvePoisoning(left, world, &left.Player, poison, at)
	expectMasteryImpact(t, battle, &right.Player, 481, 4, false)
	green := right.Player.Poisons.Green
	if green == nil || green.Remaining != 96 {
		t.Fatal("绿毒抵抗未缩短 107 个原状态刻度而保留每跳威力", green)
	}
	poison.PoisonColor = devconfig.PoisonRed
	w.resolvePoisoning(left, world, &left.Player, poison, at)
	red := right.Player.Poisons.Red
	if red == nil || red.Remaining != 96 || poisonArmour(&right.Player, 20) != 10 {
		t.Fatal("红毒抵抗应缩短原状态且保留独立降防", red)
	}
}

// expectMasteryImpact 比较原血量和实际收到的同一条伤害通知。
func expectMasteryImpact(t *testing.T, battle *mirrorCombat, target *entity, hp, amount int64, critical bool) {
	t.Helper()
	for _, row := range battle.drainObservations() {
		if impact, ok := row.Message.(*pb.CombatImpact); ok && impact.TargetHandle == target.Handle && impact.TargetLife == target.Life {
			if impact.Amount != amount || impact.Critical != critical || target.HP != hp {
				t.Fatalf("镜像目标实际扣血和暴击反馈不符，血量=%d，命中=%v", target.HP, impact)
			}
			return
		}
	}
	t.Fatal("缺少真实伤害投影")
}

// TestMasteryAttackCadence 保持原动作锁，实际攻击使用缩短后的下一攻击时刻。
func TestMasteryAttackCadence(t *testing.T) {
	w, r, _ := skillWorker(t, 0)
	fixture := &w.e.Store.Cfg.Fixture
	fixture.MasteryCombat = &devconfig.MasteryCombat{AttackSpeedCapBP: 600}
	fixture.BodyTraining = &devconfig.BodyTrainingConfig{
		Tracks: []devconfig.BodyTrainingTrack{{ID: devconfig.BodySpeed, BonusPerRank: 50, ShareCapBP: 600}},
		Steps:  make([]devconfig.BodyTrainingStep, 12),
	}
	r.Role.Level = 20
	r.Role.BodyTraining = &domain.BodyTrainingState{Speed: 12}
	base := fixture.CombatTiming.PlayerAttackInterval(r.Role.Level, w.fighter(r, &r.Player).AttackSpeed)
	wanted := base - base*600/10_000
	if wanted <= fixture.CombatTiming.PlayerAttackActionMS {
		t.Fatal("对拍时序应保留原动作锁之外的普攻间隔")
	}
	at := time.Unix(1800000000, 0)
	if code := w.attackWithSkill(r, 2, 1, at, devconfig.Skill{}, 0); code != 0 ||
		r.Player.NextAttack.Sub(at).Milliseconds() != wanted ||
		r.Player.ActionReadyAt.Sub(at).Milliseconds() != fixture.CombatTiming.PlayerAttackActionMS {
		t.Fatal("攻速没有应用于同一条攻击时钟并保留动作锁", code, r.Player.NextAttack)
	}
	if w.attackWithSkill(r, 2, 2, r.Player.NextAttack.Add(-time.Millisecond), devconfig.Skill{}, 0) != p.CodeSkillStateBlocked ||
		w.attackWithSkill(r, 2, 2, r.Player.NextAttack, devconfig.Skill{}, 0) != 0 {
		t.Fatal("下一次实际挥刀未遵守新时钟边界")
	}
}

// TestMasteryControlResistance 对拍三个独立状态、控制门禁、同种不刷新和到期恢复。
func TestMasteryControlResistance(t *testing.T) {
	w, r, _ := skillWorker(t, 1)
	f := &w.e.Store.Cfg.Fixture
	f.MasteryCombat = &devconfig.MasteryCombat{ResistanceCapBP: 1000, MonsterControls: map[uint32]devconfig.MonsterControl{}}
	f.BodyTraining = &devconfig.BodyTrainingConfig{
		Tracks: []devconfig.BodyTrainingTrack{{ID: devconfig.BodyResistance, BonusPerRank: 100, ShareCapBP: 1000}},
		Steps:  make([]devconfig.BodyTrainingStep, 10),
	}
	r.Role.Level = 20
	r.Role.BodyTraining = &domain.BodyTrainingState{Resistance: 10}
	monster, player, world := r.Monsters[0], &r.Player, r.currentWorld()
	player.Pos, monster.Pos = devconfig.Point{X: 5, Y: 5}, devconfig.Point{X: 6, Y: 5}
	step := devconfig.Point{X: 5, Y: 6}
	for _, row := range []struct {
		id, duration uint32 // 配置状态及原持续毫秒。
		wanted       int64  // 抵抗后的持续毫秒。
	}{{devconfig.MasteryFrozen, 1000, 900}, {devconfig.MasteryParalysis, 1200, 1200}, {devconfig.MasteryPetrified, 1400, 1260}} {
		at := time.Unix(1800000000, 0)
		f.MasteryCombat.MonsterControls[monster.Monster.ID] = devconfig.MonsterControl{
			StatusID: row.id, ChancePPM: 800_000, DurationMS: row.duration,
		}
		if row.id == devconfig.MasteryParalysis {
			w.roll = func(int64) int64 { return 720_000 }
			w.applyMonsterControl(r, world, monster, player, at)
			if masteryControlHeld(player, at) {
				t.Fatal("麻痹概率应该按原触发率比例缩减")
			}
		}
		w.roll = func(int64) int64 { return 719_999 }
		w.applyMonsterControl(r, world, monster, player, at)
		state := player.Statuses[row.id]
		if state == nil || state.RemainingMS != row.wanted || !masteryControlHeld(player, at) ||
			w.commitWalk(r, player, step, 2, at, 100) ||
			w.attackWithSkill(r, 2, 1, at, devconfig.Skill{}, 0) != p.CodeSkillStateBlocked ||
			w.cast(r, 1, monster.Handle, 1, at) != p.CodeSkillStateBlocked {
			t.Fatal("可用状态、动作锁或抵抗时间不符", row, state)
		}
		w.applyMonsterControl(r, world, monster, player, at.Add(time.Millisecond))
		if player.Statuses[row.id] != state || masteryControlHeld(player, state.NextProcess) {
			t.Fatal("控制重复命中延长了原时间或迟迟没有恢复")
		}
		tickEntityStatuses([]*entity{player}, state.NextProcess)
		if player.Statuses[row.id] != nil {
			t.Fatal("到期控制仍然占用状态位")
		}
	}
}
