// 本文件验证原职业动作在真实双方角色和宠物之间共用同一伤害、移动及临时资源规则。
package game

import (
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func mirrorCombatFixture(t *testing.T) (*worker, *devconfig.MirrorArenaConfig, [2]domain.MirrorCombatSnapshot, time.Time) {
	t.Helper()
	w, _, _ := skillWorker(t, 1)
	config := &devconfig.MirrorArenaConfig{PetDeathPolicy: devconfig.MirrorArenaPetFollowsOwnerDeath,
		TimeoutPolicy: devconfig.MirrorArenaTimeoutEffectiveHPRatio,
		Map:           devconfig.MapTemplate{ID: 10001, Width: 9, Height: 9, SceneKind: devconfig.SceneKindMirrorArena},
		AttackerSpawn: devconfig.Point{X: 2, Y: 2}, DefenderSpawn: devconfig.Point{X: 6, Y: 2},
		AttackerPetSpawn: devconfig.Point{X: 2, Y: 3}, DefenderPetSpawn: devconfig.Point{X: 6, Y: 3}}
	stats := devconfig.Fighter{Level: 20, HP: 200, MP: 60, AttackMin: 8, AttackMax: 8, Accuracy: 20,
		MagicMin: 12, MagicMax: 12, TaoMin: 18, TaoMax: 18, MoveMS: 100, AttackMS: 400}
	fighters := [2]domain.MirrorCombatSnapshot{
		{Name: "镜像法师", Job: 1, Level: 20, Fighter: stats},
		{Name: "镜像道士", Job: 2, Level: 20, Fighter: stats},
	}
	return w, config, fighters, time.Unix(1800000000, 0)
}

func equipMirrorSkill(t *testing.T, w *worker, snapshot *domain.MirrorCombatSnapshot, id uint32) devconfig.Skill {
	t.Helper()
	skill, found := w.e.Store.Cfg.Fixture.Skill(id)
	if !found || skill.JobID != snapshot.Job {
		t.Fatalf("原技能 %d 不属于该侧 %d", id, snapshot.Job)
	}
	snapshot.SkillSlotIDs = []uint32{id, 0, 0, 0}
	snapshot.Skills = []domain.LearnedSkill{{SkillID: id, Level: skill.LearnedLevel}}
	return skill
}

func TestMirrorCombatBothProfessionsCastAndKeepOriginalAssets(t *testing.T) {
	w, config, fighters, at := mirrorCombatFixture(t)
	ball := equipMirrorSkill(t, w, &fighters[0], 1)
	seal := equipMirrorSkill(t, w, &fighters[1], 13)
	fighters[1].Material = &domain.MirrorCombatSupply{ItemID: seal.MaterialItemID, Quantity: 3}
	fighters[1].Pet = &domain.MirrorPetSnapshot{DefinitionID: 2, Fighter: fighters[1].Fighter}
	b := w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 77)
	if b == nil {
		t.Fatal("不能装配合法镜像战斗")
	}
	left, right := b.sides[0].room, b.sides[1].room
	if left.MonsterCount != 0 || right.MonsterCount != 0 || w.skillTarget(left, ball, right.Player.Handle) != &right.Player ||
		w.skillTarget(right, seal, right.Pet.Handle) != nil || w.skillTarget(right, seal, left.Player.Handle) != &left.Player {
		t.Fatal("敌我目标类型或原怪物槽发生混淆")
	}
	w.advanceMirrorCombat(b, at)
	if left.Player.MP != left.Player.MaxMP-ball.MPBaseCost-int64(ball.LearnedLevel)*ball.MPLevelCost ||
		right.Player.MP != right.Player.MaxMP-seal.MPBaseCost-int64(seal.LearnedLevel)*seal.MPLevelCost ||
		b.sides[1].material.Quantity != 2 || fighters[1].Material.Quantity != 3 || left.CastID != 1 || right.CastID != 2 {
		t.Fatalf("双方技能、临时扣料或全场施法号错误：法师 MP=%d，道士 MP=%d，扣料=%d，施法号=%d/%d",
			left.Player.MP, right.Player.MP, b.sides[1].material.Quantity, left.CastID, right.CastID)
	}
	if len(left.Casts) != 1 || len(right.Casts) != 1 || right.Pet.Pos == config.DefenderPetSpawn {
		t.Fatal("法师、道士或道士职业宠物没有使用原战斗规则")
	}
	w.advanceMirrorCombat(b, at.Add(750*time.Millisecond))
	if left.Player.HP >= left.Player.MaxHP || right.Player.HP >= right.Player.MaxHP || left.Player.Kind != 1 || right.Pet.Kind != 2 ||
		len(left.KillFacts) != 0 || len(right.KillFacts) != 0 || left.Pending || right.Pending {
		t.Fatal("命中没有在原双方临时身体结算，或镜像污染普通持久任务")
	}
	views := w.mirrorCombatEntities(b)
	if len(views) != 3 || views[0].DisplayName != "镜像法师" || views[1].EntityKind != 1 || views[2].EntityKind != 2 || views[2].EntityHandle != 102 {
		t.Fatalf("双方角色与宠物投影错误：%+v", views)
	}
	impacts := map[uint32]uint32{}
	for _, event := range b.drainObservations() {
		if event.ID != p.MsgS2CCombatImpact {
			continue
		}
		impact := event.Message.(*pb.CombatImpact)
		if impact.SkillId == ball.ID || impact.SkillId == seal.ID {
			impacts[impact.CasterHandle] = impact.TargetHandle
		}
	}
	if impacts[1] != 2 || impacts[2] != 1 || len(b.drainObservations()) != 0 {
		t.Fatalf("真实跨职业命中或观测流水错误：%v", impacts)
	}
}

func TestMirrorCombatObstacleAndPotionUseSharedGates(t *testing.T) {
	w, config, fighters, at := mirrorCombatFixture(t)
	equipMirrorSkill(t, w, &fighters[0], 1)
	config.Map.Blocked = []devconfig.Point{{X: 4, Y: 2}}
	fighters[0].Potions = []domain.MirrorCombatSupply{{ItemID: 36, Quantity: 1}}
	b := w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 78)
	if b == nil {
		t.Fatal("短地图装配失败")
	}
	left, right := b.sides[0].room, b.sides[1].room
	w.advanceMirrorCombat(b, at)
	if left.Player.CastSequence != 0 || left.Player.Pos == config.AttackerSpawn || !config.Map.Walkable(left.Player.Pos) {
		t.Fatal("法师跨阻挡施法或未在真实可走格寻路")
	}
	if !w.free(left, devconfig.Point{X: 1, Y: 4}, &left.Player) || w.free(left, right.Player.Pos, &left.Player) {
		t.Fatal("真实对手没有占格")
	}
	left.Player.MP = 0
	if !w.tryManaPotion(left, automaticManaNeed{cost: 8}, at.Add(time.Second)) || left.Potions.MP == 0 ||
		fighters[0].Potions[0].Quantity != 1 || b.sides[0].potions[0].Quantity != 0 || left.Pending {
		t.Fatal("补蓝没有走原本地药水恢复池，或误改入场快照及永久任务")
	}
	if w.tryManaPotion(left, automaticManaNeed{cost: 8}, at.Add(time.Second)) {
		t.Fatal("用药公共冷却内重复扣药")
	}
}

func TestMirrorCombatTimeoutKeepsInitialPetDenominatorAndDeathPolicy(t *testing.T) {
	w, config, fighters, at := mirrorCombatFixture(t)
	fighters[0].Pet = &domain.MirrorPetSnapshot{DefinitionID: 2, Fighter: fighters[0].Fighter}
	fighters[1].Fighter.HP = 100
	fighters[1].Pet = &domain.MirrorPetSnapshot{DefinitionID: 2, Fighter: fighters[1].Fighter}
	deadline := at.Add(time.Minute)
	b := w.newMirrorCombat(config, fighters, at, deadline, 79)
	if b == nil {
		t.Fatal("双宠镜像装配失败")
	}
	left, right := b.sides[0].room, b.sides[1].room
	left.Player.HP, left.Pet.HP, right.Player.HP, right.Pet.HP = 100, 0, 50, 0
	w.advanceMirrorCombat(b, deadline)
	if b.result.Winner != 2 || b.result.Cause != mirrorTimeout || !b.result.FinishedAt.Equal(deadline) {
		t.Fatalf("同有效生命比例没有由防守方获胜：%+v", b.result)
	}
	if alive := b.sides[0].initialLife; alive != 400 {
		t.Fatalf("原宠死亡改变了超时分母：%d", alive)
	}
	for _, policy := range []string{devconfig.MirrorArenaPetFollowsOwnerDeath, devconfig.MirrorArenaPetFightsAfterOwnerDeath} {
		config.PetDeathPolicy = policy
		b = w.newMirrorCombat(config, fighters, at, deadline, 80)
		left, right = b.sides[0].room, b.sides[1].room
		w.applyDamage(left, &left.Player, &right.Player, right.Player.HP, at)
		b.checkDefeat(at)
		if policy == devconfig.MirrorArenaPetFollowsOwnerDeath {
			if right.Pet.HP != 0 || b.result.Winner != 1 || b.result.Cause != mirrorDefeat {
				t.Fatal("主人阵亡策略没有一起结束原宠物")
			}
		} else if right.Pet.HP == 0 || b.result.Cause != 0 {
			t.Fatal("存活原宠可续战策略过早结束原场")
		}
	}
}

func TestMirrorCombatWarriorUsesTwoCellThrustAndAtomicSameTimeDefeat(t *testing.T) {
	w, config, fighters, at := mirrorCombatFixture(t)
	fighters[0].Job, fighters[0].Name = 0, "镜像战士"
	thrust := equipMirrorSkill(t, w, &fighters[0], 12)
	config.DefenderSpawn = devconfig.Point{X: config.AttackerSpawn.X + thrust.SecondaryDistanceCells, Y: config.AttackerSpawn.Y}
	b := w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 81)
	if b == nil {
		t.Fatal("两格刺杀镜像装配失败")
	}
	left, right := b.sides[0].room, b.sides[1].room
	w.advanceMirrorCombat(b, at)
	if left.Player.Pos != config.AttackerSpawn || left.Player.AttackSkillID != thrust.ID || len(left.Attacks) != 1 {
		t.Fatal("镜像战士没有在两格使用原刺杀技能")
	}
	attackObserved := false
	for _, event := range b.drainObservations() {
		if attack, ok := event.Message.(*pb.AttackAccepted); ok && event.ID == p.MsgS2CAttackAccepted {
			attackObserved = attack.CasterHandle == 1 && attack.SceneEpoch == 81 && attack.SkillId == thrust.ID
		}
	}
	if !attackObserved {
		t.Fatal("镜像视口没有收到原刺杀攻击动作")
	}
	w.advanceMirrorCombat(b, at.Add(time.Second))
	if right.Player.HP == right.Player.MaxHP {
		t.Fatal("镜像刺杀未沿原时间线命中另一职业身体")
	}

	config.DefenderSpawn.X--
	fighters[0].Skills, fighters[0].SkillSlotIDs = nil, []uint32{0, 0, 0, 0}
	fighters[0].Fighter.HP, fighters[1].Fighter.HP = 1, 1
	b = w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 82)
	if b == nil {
		t.Fatal("同刻终态镜像装配失败")
	}
	left, right = b.sides[0].room, b.sides[1].room
	w.advanceMirrorCombat(b, at)
	if len(left.Attacks) != 1 || len(right.Attacks) != 1 || !left.Attacks[0].Due.Equal(right.Attacks[0].Due) {
		t.Fatalf("缺少同一时刻已经受理的双方攻击：%+v/%+v", left.Attacks, right.Attacks)
	}
	due := left.Attacks[0].Due
	w.advanceMirrorCombat(b, due)
	if left.Player.HP != 0 || right.Player.HP != 0 || b.result.Winner != 2 || b.result.Cause != mirrorDefeat || !b.result.FinishedAt.Equal(due) {
		t.Fatalf("同归于尽在一方旧动作生效前提前出结果：%+v，双方 HP %d/%d", b.result, left.Player.HP, right.Player.HP)
	}
}

// TestMirrorCombatFireWallObservation 核对双方同格火墙合并、对侧独立地格和原有效期。
func TestMirrorCombatFireWallObservation(t *testing.T) {
	w, config, fighters, at := mirrorCombatFixture(t)
	b := w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 83)
	shared, other := devconfig.Point{X: 3, Y: 4}, devconfig.Point{X: 4, Y: 4}
	b.sides[0].room.currentWorld().FireWalls = []*fireWallField{
		{Count: 1, Expires: at.Add(time.Second), Cells: [...]devconfig.Point{shared, {}, {}, {}, {}}},
	}
	b.sides[1].room.currentWorld().FireWalls = []*fireWallField{
		{Count: 2, Expires: at.Add(2 * time.Second), Cells: [...]devconfig.Point{shared, other, {}, {}, {}}},
		{Count: 1, Expires: at.Add(-time.Second), Cells: [...]devconfig.Point{{X: 1, Y: 1}, {}, {}, {}, {}}},
	}
	for _, moment := range []time.Time{at, at.Add(1100 * time.Millisecond)} {
		cells := b.fireWallCells(moment)
		if len(cells) != 2 || cells[0].X != 3 || cells[0].Y != 4 || cells[1].X != 4 || cells[1].Y != 4 {
			t.Fatalf("双方火墙未按原当前地格投影：%v", cells)
		}
	}
	if len(b.fireWallCells(at.Add(3*time.Second))) != 0 {
		t.Fatal("过期火墙留在镜像观战帧")
	}
}
