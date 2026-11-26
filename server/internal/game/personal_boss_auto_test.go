// 本文件验证后台首领的资格指纹、数据库背压和普通场景的附着保护。
package game

import (
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
)

// TestPersonalBossAutomaticQualification 保留无关资产变化，拒绝原战斗输入变化。
func TestPersonalBossAutomaticQualification(t *testing.T) {
	owner := &room{Role: domain.Role{Equipped: []domain.Equipped{
		{Slot: 1, Item: domain.Item{ID: "布衣"}},
		{Slot: 0, Item: domain.Item{ID: "木剑", DurabilityU: 3000}},
	}}}
	snapshot := personalBossSnapshot{Fighter: devconfig.Fighter{HP: 20}, SkillSlotIDs: []uint32{2, 0, 0, 0},
		Pet: &entity{Def: 2001, PetRuntime: &petRuntimeState{Level: 2, Experience: 10}}}
	key := personalBossQualification(owner, snapshot)
	if len(key) != 64 {
		t.Fatal("有效战斗条件未生成摘要")
	}
	owner.Role.Equipped[0], owner.Role.Equipped[1] = owner.Role.Equipped[1], owner.Role.Equipped[0]
	owner.Role.Equipped[0].Item.DurabilityU--
	owner.Role.Revision++
	snapshot.Pet.PetRuntime.Experience++
	if personalBossQualification(owner, snapshot) != key || snapshot.Pet.PetRuntime.Experience != 11 {
		t.Fatal("相同战斗条件因穿戴顺序、普通损耗或无关经验失去资格，或覆盖了原宠物数据")
	}
	for _, variant := range []struct {
		name   string // 改变的原战斗输入。
		change func(*personalBossSnapshot)
	}{
		{"战斗属性", func(copy *personalBossSnapshot) { copy.Fighter.HP++ }},
		{"技能槽", func(copy *personalBossSnapshot) { copy.SkillSlotIDs = []uint32{0, 2, 0, 0} }},
		{"自动施法偏好", func(copy *personalBossSnapshot) { copy.autoSkills = &autoSkillSettings{enabled: false} }},
		{"宠物不在场", func(copy *personalBossSnapshot) { copy.Pet = nil }},
	} {
		t.Run(variant.name, func(t *testing.T) {
			copy := snapshot
			variant.change(&copy)
			if personalBossQualification(owner, copy) == key {
				t.Fatal("换了实际战斗条件却仍能使用旧通关证明")
			}
		})
	}
	owner.Role.Equipped[0].Item.ID = "另一把同属性木剑"
	if personalBossQualification(owner, snapshot) == key {
		t.Fatal("装备换成另一实例后仍沿用旧通关证明")
	}
}

// TestPersonalBossAutomaticQueueFullKeepsOrdinaryReady 不把未受理的自动预约留在普通资产等待槽。
func TestPersonalBossAutomaticQueueFullKeepsOrdinaryReady(t *testing.T) {
	jobs := make(chan job, 1)
	jobs <- job{}
	w := &worker{e: &Engine{jobs: jobs}}
	owner := &room{Ready: true}
	if w.submitPersonalBossJob(owner, "start", "自动预约", persistence.Auth{}, false, true, nil, nil) || owner.Pending ||
		owner.PendingOperationID != "" || owner.PendingRetry != nil || !owner.Ready {
		t.Fatal("队列满时未受理的自动预约仍冻结了普通输入或创建了确认未知任务")
	}
}

// TestPersonalBossAutomaticKeepsOrdinaryPetAttachment 后台场与隐藏状态都不夺取普通场的出生附件。
func TestPersonalBossAutomaticKeepsOrdinaryPetAttachment(t *testing.T) {
	_, owner, _, _, _, _ := petBornWorker(t)
	owner.personalBoss = &personalBossRun{begin: persistence.PersonalBossBegin{Automatic: true}, battle: &room{}}
	if capturePetBornAttachment(owner) == nil {
		t.Fatal("有后台首领时普通地图失去了对本角色宠物出生的观察资格")
	}
	owner.personalBoss.begin.Automatic = false
	owner.personalBoss.shown = true
	if capturePetBornAttachment(owner) != nil {
		t.Fatal("正在看手动首领时仍在采集普通场景的宠物出生附件")
	}
}
