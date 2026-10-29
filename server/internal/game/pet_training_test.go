package game

import (
	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"testing"
	"time"
)

func TestPetRuntimeConfirmedExperienceTrainsOriginalBirthOnce(t *testing.T) {
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now)
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	pet.HP = 7
	monster := personal.Monsters[0]
	monster.Monster.Exp = 10000
	w.enqueueKillFactInWorld(r, personal, monster, now, &r.Player, "", r.Role.PrivilegeUntil)
	credit := r.KillFacts[0].PetTraining
	if credit == nil {
		t.Fatal("主人最后一击未捕获合格宠物训练")
	}
	w.flushKillFacts(r)
	j := <-w.e.jobs
	q := w.killSettlement(r, r.KillFacts[0])
	binding := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, generation: r.SettlementGeneration, id: r.PendingOperationID, fingerprint: r.PendingFingerprint}
	finish := func() {
		w.finishKillFact(r, credit.KillID, q)
	}
	w.finishSettlement(binding, q, r.Role, persistence.Fail(p.CodeOperationPending, "unknown"), &j, finish)
	if !r.Pending || r.PendingRetry != &j || pet.PetRuntime.Experience != 0 || pet.PetRuntime.Level != 0 {
		t.Fatal("unknown先发训练或换掉原job")
	}
	role := r.Role
	role.SourceSeq, role.Revision = q.SourceSeq, role.Revision+1
	w.finishSettlement(binding, q, role, nil, &j, finish)
	if pet.PetRuntime.Level != 1 || pet.PetRuntime.Experience != 10000 || pet.HP != 7 || pet.MaxHP <= 60 || !credit.Applied {
		t.Fatal("已确认RoleEXP没有一次训练，或成长回满HP")
	}
	w.finishSettlement(binding, q, role, nil, &j, finish)
	if pet.PetRuntime.Level != 1 || pet.PetRuntime.Experience != 10000 {
		t.Fatal("重复成功回调重复训练")
	}
}

func TestPetRuntimeTrainingCreditNeverMovesToNewBirth(t *testing.T) {
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now)
	old := spawnRuntimePet(t, w, r, personal, 0, now)
	credit := w.capturePetTraining(r, "kill")
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 10000}
	if !w.preparePetTraining(r, credit, q) {
		t.Fatal("训练前态准备失败")
	}
	w.applyDamageInWorld(r, personal, personal.Monsters[0], old, old.HP, now)
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	r.Role.SourceSeq = q.SourceSeq
	if !w.awardPetTraining(r, credit, q) || old.PetRuntime.Level != 1 || old.HP != 0 || !old.IsPetCorpse || pet.PetRuntime.Level != 0 || pet.PetRuntime.Experience != 0 {
		t.Fatal("原合格对象死亡后旧经验转送当前新宠或复活尸体")
	}
	if w.awardPetTraining(r, credit, q) {
		t.Fatal("同一credit二次生效")
	}
}

func TestPetRuntimeEligibilityAndSingleLevelCarryAtMaximum(t *testing.T) {
	w, r, _, personal, town, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now)
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	pet.Pos = devconfig.Point{X: r.Player.Pos.X + 17, Y: r.Player.Pos.Y}
	if w.capturePetTraining(r, "far") != nil {
		t.Fatal("超16格宠物仍取得训练资格")
	}
	pet.Pos = r.Player.Pos
	r.worldState = town
	if w.capturePetTraining(r, "other-world") != nil {
		t.Fatal("异图宠物仍取得训练资格")
	}
	r.worldState = personal
	for i := 0; i < 4; i++ {
		credit := w.capturePetTraining(r, "carry")
		q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 100000}
		if credit == nil || !w.preparePetTraining(r, credit, q) {
			t.Fatal("合格训练缺失")
		}
		r.Role.SourceSeq = q.SourceSeq
		w.awardPetTraining(r, credit, q)
		if pet.PetRuntime.Level != uint32(i+1) {
			t.Fatal("单次grant不止升一级")
		}
	}
	carry := pet.PetRuntime.Experience
	if carry == 0 || w.capturePetTraining(r, "max") != nil || petTrainingPB(pet).NextLevelExperience != 0 || petTrainingPB(pet).Experience != carry {
		t.Fatal("满级没有保留来源超额经验，或继续取得训练资格")
	}
	pet.PetRuntime.Level--
	credit := w.capturePetTraining(r, "zero")
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1}
	w.preparePetTraining(r, credit, q)
	r.Role.SourceSeq = q.SourceSeq
	w.awardPetTraining(r, credit, q)
	if pet.PetRuntime.Level != 3 || pet.PetRuntime.Experience != carry {
		t.Fatal("零角色EXP借超额carry继续升级")
	}
}

func TestPetRuntimeTrainingDoesNotRestoreDespawnedTemporaryBody(t *testing.T) {
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now)
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	credit := w.capturePetTraining(r, "old")
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 10000}
	w.preparePetTraining(r, credit, q)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], pet, pet.HP, now)
	w.expirePetCorpses(r, now.Add(time.Second))
	r.Role.SourceSeq = q.SourceSeq
	if w.awardPetTraining(r, credit, q) || r.findPetBirth(credit.Pet) != nil || r.Pet != nil {
		t.Fatal("确认EXP从旧credit恢复已Despawn的临时宠物")
	}
}

func TestPetRuntimeQueuedCreditAtMaximumDoesNotRejectIgnoredExperience(t *testing.T) {
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now)
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	pet.PetRuntime.Level = 3 // 可达的临升满级前态；两次死亡事实先捕获，再按原队列确认。
	a, b := w.capturePetTraining(r, "first"), w.capturePetTraining(r, "second")
	qA := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 6148914691236517205}
	if a == nil || b == nil || !w.preparePetTraining(r, a, qA) {
		t.Fatal("合法uint64训练极值前态准备失败")
	}
	r.Role.SourceSeq = qA.SourceSeq
	if !w.awardPetTraining(r, a, qA) || pet.PetRuntime.Level != 4 || pet.PetRuntime.Experience != 18446744073709471615 {
		t.Fatal("首个确认grant未按单次一级保留carry")
	}
	qB := persistence.Settlement{SourceSeq: qA.SourceSeq + 1, Experience: 100000}
	if !w.preparePetTraining(r, b, qB) {
		t.Fatal("已满级的在途credit应忽略训练，不因本不会累积的经验溢出拒绝原RoleEXP")
	}
	r.Role.SourceSeq = qB.SourceSeq
	w.awardPetTraining(r, b, qB)
	if pet.PetRuntime.Level != 4 || pet.PetRuntime.Experience != 18446744073709471615 || r.Isolated || !b.Applied {
		t.Fatal("满级后改写carry、隔离owner或未消费原credit")
	}
}
