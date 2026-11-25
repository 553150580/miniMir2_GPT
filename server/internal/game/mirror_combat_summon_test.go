// 本文件验证镜像补召的实际技能时序、双方身份、原尸体以及超时结算边界。
package game

import (
	"slices"
	"testing"
	"time"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// mirrorSummonFixture 使用普通发布规则中的骷髅、神兽与护符，临时召唤策略只装配在本测试中。
func mirrorSummonFixture(t *testing.T) (*worker, *devconfig.MirrorArenaConfig, [2]domain.MirrorCombatSnapshot, [2]devconfig.Skill, time.Time) {
	t.Helper()
	w, config, fighters, at := mirrorCombatFixture(t)
	config.Summoning = &devconfig.MirrorArenaSummoningRule{AllowWithoutEntryPet: true, MaxBirthsPerSide: 2}
	var skills [2]devconfig.Skill
	for i, id := range [...]uint32{devconfig.SummonSkeletonSkillID, devconfig.SummonShinsuSkillID} {
		fighters[i].Job, fighters[i].Level, fighters[i].Fighter.MP = 2, 40, 1000
		skills[i] = equipMirrorSkill(t, w, &fighters[i], id)
		fighters[i].Material = &domain.MirrorCombatSupply{ItemID: skills[i].MaterialItemID, Quantity: 50}
	}
	return w, config, fighters, skills, at
}

// TestMirrorCombatSummonBothSidesUsesSlotsAndBirthTimeline 覆盖双方从普通技能槽到正式出生事实的完整时序。
func TestMirrorCombatSummonBothSidesUsesSlotsAndBirthTimeline(t *testing.T) {
	w, config, fighters, skills, at := mirrorSummonFixture(t)
	// 两侧的面向前格都阻挡，必须选择不切墙角的相邻落点。
	config.Map.Blocked = []devconfig.Point{{X: 3, Y: 2}, {X: 5, Y: 2}}
	b := w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 92)
	w.advanceMirrorCombat(b, at)
	var due time.Time
	for i, side := range b.sides {
		child, skill := side.room, skills[i]
		cost := skill.MPBaseCost + int64(skill.LearnedLevel)*skill.MPLevelCost
		if side.pendingSummon == nil || child.Pet != nil || len(child.Casts) != 1 || child.Player.CastSkillID != skill.ID ||
			child.Player.MP != child.Player.MaxMP-cost || fighters[i].Material.Quantity != 50 || side.material.Quantity != 50-skill.MaterialQuantity ||
			!child.NextSpell.Equal(at.Add(time.Duration(skill.GlobalSpellMS)*time.Millisecond)) ||
			!child.SkillCooldowns[skill.ID].Equal(at.Add(time.Duration(skill.CooldownBaseMS-int64(skill.LearnedLevel)*skill.CooldownReductionMS)*time.Millisecond)) ||
			!child.Casts[0].Due.Equal(at.Add(time.Duration(skill.DelayBaseMS)*time.Millisecond)) || child.Pending || child.PendingPetBirth != nil {
			t.Fatalf("第%d侧未沿原施法时钟扣除一次临时资源，或错误访问原持久槽", i)
		}
		if w.mirrorMaterialCast(child, skill, 0, child.Player.Direction, nil, at, &automaticManaNeed{}, 0) {
			t.Fatalf("第%d侧重复预约相同出生", i)
		}
		if child.Casts[0].Due.After(due) {
			due = child.Casts[0].Due
		}
	}
	for _, o := range b.observations {
		if o.ID == p.MsgS2CPetBorn {
			t.Fatal("施法受理早于实际出生却已发出生观察")
		}
	}
	w.advanceMirrorCombat(b, due)
	for i, side := range b.sides {
		child, skill := side.room, skills[i]
		pet := child.Pet
		if pet == nil || pet.Handle != uint32(103+i) || pet.Life != 1 || pet.HP <= 0 || pet.PetRuntime.Definition.SourceSkillID != skill.ID ||
			!config.Map.Walkable(pet.Pos) || distance(child.Player.Pos, pet.Pos) != 1 || side.pendingSummon != nil || side.births != 1 ||
			w.free(b.sides[1-i].room, pet.Pos, nil) {
			t.Fatalf("第%d侧出生身份、落点或共享活体占格异常：%+v", i, pet)
		}
		definition := pet.PetRuntime.Definition
		if pet.PetRuntime.Level != definition.Training.InitialLevelByRank[skill.LearnedLevel] ||
			!pet.ActionReadyAt.Equal(due.Add(time.Duration(definition.Lifecycle.SpawnReadyMS)*time.Millisecond)) {
			t.Fatalf("第%d侧未应用原技能等级宠物或出生行动时钟", i)
		}
		if i == 0 && pet.Direction != skeletonSpawnDirection || i == 1 && pet.Direction != child.Player.Direction {
			t.Fatalf("第%d侧未采用该宠物原出生朝向", i)
		}
		found := 0
		for _, o := range b.observations {
			born, ok := o.Message.(*pb.PetBorn)
			if ok && o.ID == p.MsgS2CPetBorn && born.EntityHandle == pet.Handle {
				found++
				if born.EntityLife != pet.Life || born.DefinitionId != pet.Def || born.MapId != config.Map.ID || born.SceneEpoch != child.Scene ||
					born.SourceCastId != uint64(i+1) || born.SourceSkillId != skill.ID || born.Role != nil || !o.bornAt.Equal(due) {
					t.Fatal("出生观察丢失原场、技能、身体或单调时钟", born)
				}
			}
		}
		if found != 1 {
			t.Fatalf("第%d侧新生观察次数为%d", i, found)
		}
	}
}

// TestMirrorCombatReplacementArchivesOldIdentityAndIgnoresNewLifeAtTimeout 验证原尸、补召和结算分别使用正确的出生身份。
func TestMirrorCombatReplacementArchivesOldIdentityAndIgnoresNewLifeAtTimeout(t *testing.T) {
	w, config, fighters, skills, at := mirrorSummonFixture(t)
	config.Summoning = &devconfig.MirrorArenaSummoningRule{MaxBirthsPerSide: 1}
	config.PetDeathPolicy = devconfig.MirrorArenaPetFightsAfterOwnerDeath
	definition, _ := w.e.Store.Cfg.Fixture.PetRuntime.DefinitionByID(skills[0].Summon.DefinitionID)
	fighters[0].Pet = &domain.MirrorPetSnapshot{DefinitionID: definition.Base.DefinitionID, Fighter: fighters[0].Fighter}
	b := w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 93)
	left, right := b.sides[0].room, b.sides[1].room
	old := left.Pet
	if old == nil || old.Handle != 101 || old.Life != 1 {
		t.Fatal("原入场宠物丢失固定身份")
	}
	if ok, _ := w.tryAttackSkillSlots(left, &right.Player, 2, 0, at, false); ok {
		t.Fatal("存在原活宠时队列错误切换活宠")
	}
	if ok, _ := w.tryAttackSkillSlots(right, &left.Player, 6, 0, at, false); ok {
		t.Fatal("禁止空槽首召的策略允许未带宠一方召唤")
	}
	oldRef := reference(left, old)
	w.applyMirrorDamage(right, right.currentWorld(), &right.Player, old, old.HP, at, nil)
	if len(left.PetCorpses) != 1 {
		t.Fatal("原宠死亡未留下唯一尸体", left.PetCorpses)
	}
	corpse := left.PetCorpses[0]
	if left.Pet != nil || corpse.Body != old || !old.IsPetCorpse || old.HP != 0 ||
		!corpse.ExpiresAt.Equal(at.Add(time.Duration(definition.Lifecycle.CorpseTTLMS)*time.Millisecond)) ||
		!w.free(right, old.Pos, nil) || slices.Contains(right.currentWorld().combatOpponents(), old) {
		t.Fatal("原宠尸体占据活体槽、未退攻击目标或更换原身份和绝对期限")
	}
	if views := w.mirrorCombatEntities(b); !slices.ContainsFunc(views, func(e *pb.EntityState) bool { return e.EntityHandle == 101 && e.IsPetCorpse && e.Hp == 0 }) {
		t.Fatal("旧尸未按已死亡实体投影")
	}
	// 已受理的旧攻击仍由原死亡身体完成，不借新手柄重放。
	rightHP := right.Player.HP
	w.resolveAttackInWorld(left, left.currentWorld(), pendingAttack{Caster: oldRef, Target: reference(right, &right.Player), Due: at, Amount: 1, Defence: defenceAC}, at)
	if right.Player.HP != rightHP-1 {
		t.Fatal("原已受理宠物攻击因尸体退场而改变")
	}
	if ok, _ := w.tryAttackSkillSlots(left, &right.Player, 2, 0, at, false); !ok {
		t.Fatal("原宠死亡后没有实际受理替补")
	}
	event := left.Casts[0]
	w.advanceMirrorCombat(b, event.Due)
	replacement := left.Pet
	if replacement == nil || replacement == old || replacement.Handle != 103 || replacement.Life != oldRef.Life+1 || left.PetCorpses[0] != corpse ||
		w.resolveInWorld(right, right.currentWorld(), oldRef) != nil {
		t.Fatal("替补重用了原宠身份，或旧目标能转接新身体")
	}
	// 相同完成事件重复进入时间线，不得生成第二只宠或重复发出生事实。
	w.resolveSummonPet(left, left.currentWorld(), event, event.Due)
	if b.sides[0].births != 1 || left.Pet != replacement {
		t.Fatal("已消费出生计划仍能重复安装")
	}
	left.Player.HP = 0
	b.checkDefeat(event.Due)
	if b.result.Cause != 0 || b.sides[0].survivingLife() != 0 || b.sides[0].initialLife != 400 {
		t.Fatal("全灭没有计当前活宠，或超时分子错误计入替补宠")
	}
	left.Player.HP, right.Player.HP = 100, 100
	b.finishTimeout()
	if b.result.Winner != 2 || b.result.Cause != mirrorTimeout {
		t.Fatal("原上阵宠物形成的固定生命比例被替补宠改写", b.result)
	}
	expireMirrorPetCorpses(left, corpse.ExpiresAt.Add(-time.Nanosecond))
	if len(left.PetCorpses) != 1 {
		t.Fatal("旧尸在原截止前被移除")
	}
	expireMirrorPetCorpses(left, corpse.ExpiresAt)
	if len(left.PetCorpses) != 0 || left.Pet != replacement {
		t.Fatal("旧尸过期或在清理时删除了当前补召活体")
	}
}

// TestMirrorCombatSummonRejectsInvalidInputsAndCancelsLethalSameTick 确认失败门禁和同刻终态不会产生第二条出生事件。
func TestMirrorCombatSummonRejectsInvalidInputsAndCancelsLethalSameTick(t *testing.T) {
	w, config, fighters, skills, at := mirrorSummonFixture(t)
	blocked := []devconfig.Point{{X: 5, Y: 1}, {X: 6, Y: 1}, {X: 7, Y: 1}, {X: 7, Y: 2},
		{X: 7, Y: 3}, {X: 6, Y: 3}, {X: 5, Y: 3}, {X: 5, Y: 2}}
	cases := []struct {
		name   string // 各个失败门槛。
		change func(*devconfig.MirrorArenaConfig, *domain.MirrorCombatSnapshot)
	}{
		{"未装配策略", func(c *devconfig.MirrorArenaConfig, _ *domain.MirrorCombatSnapshot) { c.Summoning = nil }},
		{"技能未曾学习", func(_ *devconfig.MirrorArenaConfig, f *domain.MirrorCombatSnapshot) { f.Skills = nil }},
		{"技能不在队列", func(_ *devconfig.MirrorArenaConfig, f *domain.MirrorCombatSnapshot) {
			f.SkillSlotIDs = []uint32{0, 0, 0, 0}
		}},
		{"材料数量不足", func(_ *devconfig.MirrorArenaConfig, f *domain.MirrorCombatSnapshot) {
			f.Material = &domain.MirrorCombatSupply{ItemID: skills[1].MaterialItemID}
		}},
		{"无可走邻格", func(c *devconfig.MirrorArenaConfig, _ *domain.MirrorCombatSnapshot) { c.Map.Blocked = blocked }},
		{"禁止宠物地图", func(c *devconfig.MirrorArenaConfig, _ *domain.MirrorCombatSnapshot) {
			value := true
			c.Map.NoPets = &value
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			arena, snapshot := *config, fighters
			tc.change(&arena, &snapshot[1])
			b := w.newMirrorCombat(&arena, snapshot, at, at.Add(time.Minute), 94)
			side := &b.sides[1]
			child := side.room
			before := side.material
			if accepted, _ := w.tryAttackSkillSlots(child, &b.sides[0].room.Player, 6, 0, at, false); accepted ||
				side.material != before || child.Player.MP != child.Player.MaxMP || len(child.Casts) != 0 || side.pendingSummon != nil {
				t.Fatal("失败门禁错误受理技能、扣费或提交事件")
			}
		})
	}

	b := w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 95)
	left, right := b.sides[0].room, b.sides[1].room
	right.Player.MP = 0
	if accepted, need := w.tryAttackSkillSlots(right, &left.Player, 6, 0, at, false); accepted || need.cost <= 0 || b.sides[1].pendingSummon != nil {
		t.Fatal("仅缺蓝未保留自动用药机会，或提前预约宠物")
	}
	right.Player.MP = right.Player.MaxMP
	if accepted, _ := w.tryAttackSkillSlots(right, &left.Player, 6, 0, b.deadline, false); accepted || b.sides[1].pendingSummon != nil {
		t.Fatal("场次到期后仍受理补召")
	}
	if accepted, _ := w.tryAttackSkillSlots(right, &left.Player, 6, 0, at, false); !accepted {
		t.Fatal("合法镜像技能未写入到期队列")
	}
	due := right.Casts[0].Due
	left.Attacks = append(left.Attacks, pendingAttack{Caster: reference(left, &left.Player), Target: reference(right, &right.Player),
		Due: due, Amount: right.Player.MaxHP, Defence: defenceAC})
	w.advanceMirrorCombat(b, due)
	if right.Player.HP != 0 || right.Pet != nil || b.sides[1].births != 0 || b.sides[1].pendingSummon != nil ||
		b.sides[1].material.Quantity != 50-skills[1].MaterialQuantity || b.result.Winner != 1 || b.result.Cause != mirrorDefeat ||
		slices.ContainsFunc(b.observations, func(o mirrorCombatObservation) bool { return o.ID == p.MsgS2CPetBorn }) {
		t.Fatal("死亡后的同刻补召未取消，或已受理扣料被改写")
	}
}

// TestMirrorCombatSummonRechecksLandingAndBoundsIdentities 重查受理后被占的前格，并验证全部八个出生身份的技术上界。
func TestMirrorCombatSummonRechecksLandingAndBoundsIdentities(t *testing.T) {
	w, config, fighters, skills, at := mirrorSummonFixture(t)
	config.Summoning.MaxBirthsPerSide = devconfig.MirrorArenaMaxBirthsPerSide
	b := w.newMirrorCombat(config, fighters, at, at.Add(time.Minute), 96)
	left, right := b.sides[0].room, b.sides[1].room
	for i := uint32(0); i < devconfig.MirrorArenaMaxBirthsPerSide; i++ {
		if accepted, _ := w.tryAttackSkillSlots(left, &right.Player, 2, 0, at, false); !accepted {
			t.Fatalf("第%d次合法出生未受理", i+1)
		}
		event := left.Casts[0]
		left.Casts = left.Casts[:0]
		before := right.Player.Pos
		if i == 0 && !w.commitRelocation(right, &right.Player, event.Summon.Front) {
			t.Fatal("不能预置出生前格的真实敌方占位")
		}
		w.resolveDueEvent(left, dueEvent{World: left.currentWorld(), At: event.Due, Cast: &event}, event.Due)
		pet := left.Pet
		if pet == nil || pet.Handle != 103+2*i || pet.Life != uint64(i+1) || pet.Pos == right.Player.Pos || !config.Map.Walkable(pet.Pos) {
			t.Fatalf("第%d次宠物未重查前格，或分配了旧身份：%+v", i+1, pet)
		}
		if right.Player.Pos != before && !w.commitRelocation(right, &right.Player, before) {
			t.Fatal("不能归还敌方试验位置")
		}
		w.applyMirrorDamage(right, right.currentWorld(), &right.Player, pet, pet.HP, event.Due, nil)
		if left.Pet != nil || len(left.PetCorpses) != int(i+1) {
			t.Fatal("补召活体死亡没有释放名额或擅自丢弃原尸")
		}
		at = left.NextSpell
		for _, gate := range [...]time.Time{left.SkillCooldowns[skills[0].ID], left.Player.ActionReadyAt, event.Due} {
			if gate.After(at) {
				at = gate
			}
		}
	}
	if accepted, _ := w.tryAttackSkillSlots(left, &right.Player, 2, 0, at, false); accepted ||
		!at.Before(b.deadline) || len(left.Casts) != 0 || b.sides[0].pendingSummon != nil ||
		b.sides[0].material.Quantity != 50-int64(devconfig.MirrorArenaMaxBirthsPerSide)*skills[0].MaterialQuantity {
		t.Fatal("上限耗尽后仍受理，或八个独立合法出生意外消耗多次")
	}
}
