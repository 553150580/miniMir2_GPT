// 本文件回归神兽与骷髅的单宠替换、免费召回，以及两格喷焰的命中时间线。
package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
	"testing"
	"time"
)

// shinsuWorker 添加隔离的神兽模板与正式技能身份，宠物初始为空。
func shinsuWorker(t *testing.T) (*worker, *room, time.Time) {
	t.Helper()
	w, r, _, _, _, now := summonWorker(t, false)
	fixture := &w.e.Store.Cfg.Fixture
	definition := fixture.PetRuntime.Definitions[0].Clone()
	definition.ID, definition.Base.DefinitionID, definition.MonsterTemplateID = 2, 80030, 80030
	definition.RecallName, definition.ResourceKey = "Shinsu", "pet_shinsu"
	definition.SourceSkillID = devconfig.SummonShinsuSkillID
	definition.Training.MaximumLevelByRank = []uint32{1, 3, 5, 7}
	fixture.PetRuntime.Definitions = append(fixture.PetRuntime.Definitions, definition)
	skill, _ := fixture.Skill(devconfig.SummonSkeletonSkillID)
	skill.ID, skill.BookItemID, skill.LearnLevel, skill.MaterialQuantity = devconfig.SummonShinsuSkillID, 33, 35, 5
	skill.Summon = &devconfig.SkillSummonRules{DefinitionID: definition.ID}
	if _, exists := fixture.Skill(skill.ID); !exists {
		fixture.Skills = append(fixture.Skills, skill)
	}
	r.Role.Level = 40
	r.Role.Skills = append(r.Role.Skills, domain.LearnedSkill{SkillID: skill.ID, Level: 2})
	return w, r, now
}

// TestShinsuSwapsOneLivePetOnlyAtBirthAndRecallsWithoutMaterials 验证两宠可互换、拒绝和取消不清旧宠，以及召回保留原生命。
func TestShinsuSwapsOneLivePetOnlyAtBirthAndRecallsWithoutMaterials(t *testing.T) {
	w, r, now := shinsuWorker(t)
	old, ok := w.newPet(r, 1, 0, r.Player.Pos, now)
	if !ok || !w.installPet(r, r.currentWorld(), old) {
		t.Fatal("无法安装测试用骷髅")
	}
	id := devconfig.SummonShinsuSkillID
	if _, code := w.prepareCast(r, id, 0, 2, now, false); code != p.CodeSkillMaterialRequired || r.Pet != old {
		t.Fatal("缺符应拒绝，旧骷髅应保留", code)
	}
	plan, code := w.prepareCast(r, id, 0, 2, now, true)
	if code != 0 || plan.Summon.Recall || !w.reserveSummonPet(r, plan, "cancel-shinsu", now) {
		t.Fatal("切换预留失败", code)
	}
	w.cancelSummonPet(r, plan)
	if r.Pet != old || r.PendingPetBirth != nil {
		t.Fatal("取消切换影响了活动骷髅")
	}
	if !w.reserveSummonPet(r, plan, "confirm-shinsu", now) || w.acceptPreparedCast(r, plan, 1, now) != 0 {
		t.Fatal("切换施法未受理")
	}
	due := now.Add(500 * time.Millisecond)
	w.advanceEvents(r, due.Add(-time.Nanosecond))
	if r.Pet != old || r.PendingPetBirth == nil {
		t.Fatal("旧骷髅应保留至神兽实际出生")
	}
	w.advanceEvents(r, due)
	shinsu := r.Pet
	if shinsu == nil || shinsu.Def != 80030 || shinsu.Direction != 2 || shinsu.Pos != plan.Summon.Front ||
		shinsu.Life <= old.Life || shinsu.Handle == old.Handle || shinsu.PetRuntime.Level != 2 || shinsu.PetRuntime.Maximum != 5 ||
		r.PendingPetBirth != nil || len(r.currentWorld().PetCorpses) != 0 || len(r.petBodies(r.currentWorld())) != 1 {
		t.Fatal("新神兽的定义、朝向、训练或单宠名额错误")
	}
	owned := w.ownedPetPB(r)
	if len(owned.RecallSkillIds) != 1 || owned.RecallSkillIds[0] != id {
		t.Fatal("活动神兽仅应使用对应技能免费召回")
	}
	now = now.Add(5 * time.Second)
	if code := w.cast(r, id, 0, 0, now); code != 0 || r.PendingPetBirth != nil {
		t.Fatal("同一神兽召回错误要求材料或新生名额", code)
	}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet != shinsu {
		t.Fatal("同图召回更换了原神兽")
	}
	now = now.Add(5 * time.Second)
	paidSummonPlan(t, w, r, now)
	// 神兽在切换期间自然死亡，原身体应保留自己的尸体周期。
	shinsu.HP = 0
	if !w.archivePetDeath(r, r.currentWorld(), reference(r, shinsu), now) {
		t.Fatal("新生预留阻挡了旧神兽死亡归档")
	}
	w.advanceEvents(r, now.Add(500*time.Millisecond))
	if r.Pet == nil || r.Pet.Def != old.Def || r.Pet.Life <= shinsu.Life ||
		len(r.currentWorld().PetCorpses) != 1 || r.currentWorld().PetCorpses[0].Body != shinsu {
		t.Fatal("替换回骷髅失败，或旧神兽尸体被清除")
	}
}

// TestShinsuLineAttackFreezesTwoTimedTargetsAndStopsAtWalls 验证一条攻击的两格独立到期与生命围栏。
func TestShinsuLineAttackFreezesTwoTimedTargetsAndStopsAtWalls(t *testing.T) {
	w, r, now := shinsuWorker(t)
	pet, ok := w.newPet(r, 2, 0, devconfig.Point{X: 4, Y: 4}, now)
	if !ok || !w.installPet(r, r.currentWorld(), pet) {
		t.Fatal("无法安装测试用神兽")
	}
	now = pet.ActionReadyAt
	pet.PetRuntime.Fighter.AttackMin, pet.PetRuntime.Fighter.AttackMax = 20, 20
	pet.PetRuntime.Fighter.Accuracy = 20
	w.roll = func(int64) int64 { return 0 }
	monster := r.Monsters[0].Monster
	front := &entity{Kind: 3, Handle: 30, Pos: devconfig.Point{X: 5, Y: 4}, HP: 100, MaxHP: 100, Life: 1, Monster: monster}
	far := &entity{Kind: 3, Handle: 31, Pos: devconfig.Point{X: 6, Y: 4}, HP: 100, MaxHP: 100, Life: 1, Monster: monster}
	side := &entity{Kind: 3, Handle: 32, Pos: devconfig.Point{X: 6, Y: 5}, HP: 100, MaxHP: 100, Life: 1, Monster: monster}
	world := r.currentWorld()
	world.Monsters, world.MonsterCount = []*entity{front, far, side}, 3
	if w.tryPetAttack(r, pet, side, now) || len(world.Attacks) != 0 {
		t.Fatal("神兽错误攻击了非八方向目标")
	}
	w.e.Store.Cfg.Fixture.Map.Blocked = []devconfig.Point{front.Pos}
	if w.tryPetAttack(r, pet, far, now) || len(world.Attacks) != 0 {
		t.Fatal("神兽错误穿过静态阻挡攻击")
	}
	w.e.Store.Cfg.Fixture.Map.Blocked = nil
	if !w.tryPetAttack(r, pet, far, now) || len(world.Attacks) != 2 || pet.AttackSequence != 1 ||
		world.Attacks[0].Amount != world.Attacks[1].Amount || world.Attacks[0].Sequence != world.Attacks[1].Sequence ||
		world.Attacks[0].Target.Handle != front.Handle || world.Attacks[1].Target.Handle != far.Handle {
		t.Fatal("单次喷焰未冻结同线两格的目标和共用攻击力")
	}
	w.advanceEvents(r, now.Add(549*time.Millisecond))
	if front.HP != 100 || far.HP != 100 {
		t.Fatal("延迟喷焰错误提前命中")
	}
	w.advanceEvents(r, now.Add(550*time.Millisecond))
	if front.HP != 80 || far.HP != 100 || side.HP != 100 {
		t.Fatal("一格命中没有独立结算")
	}
	far.Life++ // 原二格目标换代后，不能把旧喷焰结算给新生命。
	w.advanceEvents(r, now.Add(600*time.Millisecond))
	if far.HP != 100 || side.HP != 100 || len(world.Attacks) != 0 {
		t.Fatal("换代后的目标或侧格错误受伤")
	}
}
