package game

import (
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"testing"
)

func enableGrowthUnit(w *worker, r *room) {
	w.e.Store.Cfg.Fixture.RuleVersion = 20
	w.e.Store.Cfg.Fixture.Progression = devconfig.ProgressionRules{CurveVersion: "unit-growth-20", InitialLevelCap: 50, RebirthLevelCapIncrement: 1, OperatingLevelCap: 50, CatalogMaxLevel: 50, PendingPoolPolicy: "one_next_level", NewOverflowPolicy: "reject_new_with_receipt", LegacyOverflowPolicy: "preserve_frozen", LevelUpResources: "source_full_if_alive_original_birth"}
	for i := 0; i < 50; i++ {
		w.e.Store.Cfg.Fixture.Progression.ExpPerLevel = append(w.e.Store.Cfg.Fixture.Progression.ExpPerLevel, 10)
	}
	w.e.Store.Cfg.Fixture.Progression.ExpPerLevel[49] = 100
	r.Role.Level = 1
	r.Role.Experience = 0
	r.Role.ProgressionFormatVersion = 1
	r.Role.ProgressionCurveVersion = "unit-growth-20"
}

func TestGrowthConfirmedOriginalBirthRestoresOnceAfterUnknownAndInventory(t *testing.T) {
	w, r, c, _, _ := recycleOwner(t)
	enableGrowthUnit(w, r)
	r.Player.HP, r.Player.MP = 2, 1
	r.Pending = true
	r.SettlementGeneration = 3
	r.PendingOperationID = "source-original"
	r.PendingFingerprint = "digest"
	b := settlementBinding{room: r, auth: r.Auth, world: r.currentWorld().Ref, generation: 3, id: r.PendingOperationID, fingerprint: r.PendingFingerprint, progressionActor: reference(r, &r.Player), sourceBefore: r.Role.SourceSeq}
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 15}
	contract, err := persistence.GrowthContract(w.e.Store.Cfg.Fixture)
	if err != nil {
		t.Fatal(err)
	}
	role, grant, err := domain.ApplyExperience(r.Role, 15, contract)
	if err != nil {
		t.Fatal(err)
	}
	role.SourceSeq = q.SourceSeq
	role.Revision++
	op := persistence.Operation{Kind: "settlement", RoleID: role.ID, SourceSeq: q.SourceSeq, Revision: role.Revision, Settlement: &persistence.SettlementReceipt{RuleVersion: 20, Experience: &grant}}
	retry := &job{}
	w.finishSettlementReceipt(b, q, role, op, persistence.Fail(p.CodeOperationPending, "unknown"), retry, nil)
	if !r.Pending || r.Player.HP != 2 || r.Player.MP != 1 {
		t.Fatal("未知提交提前恢复")
	}
	w.finishSettlementReceipt(b, q, role, op, nil, retry, nil)
	if r.Pending || r.Player.HP != r.Player.MaxHP || r.Player.MP != r.Player.MaxMP || r.Player.Life != 0 {
		t.Fatal("新升级未恢复原身体")
	}
	inv, event := -1, -1
	for i, f := range c.frames {
		if f.ID == p.MsgS2CInventory {
			inv = i
		}
		if f.ID == p.MsgS2CExperienceGrantResult {
			var x pb.ExperienceGrantResult
			_ = proto.Unmarshal(f.Payload, &x)
			if x.AcceptedExperience != 15 || x.LevelBefore != 1 || x.LevelAfter != 2 || x.Migrated {
				t.Fatal(&x)
			}
			event = i
		}
	}
	if inv < 0 || event <= inv {
		t.Fatal("成长结果早于完整Inventory")
	}
	r.Player.HP, r.Player.MP = 1, 0
	w.finishSettlementReceipt(b, q, role, op, nil, retry, nil)
	if r.Player.HP != 1 || r.Player.MP != 0 {
		t.Fatal("旧成功回调重复恢复")
	}
}

func TestGrowthRecoveryRejectsDeadChangedBirthWorldAndReplay(t *testing.T) {
	for _, name := range []string{"dead", "life", "world", "replayed", "migration_only", "old_auth"} {
		t.Run(name, func(t *testing.T) {
			w, r, _, _, _ := recycleOwner(t)
			enableGrowthUnit(w, r)
			r.Player.HP, r.Player.MP = 2, 1
			r.Pending = true
			r.SettlementGeneration = 3
			r.PendingOperationID = "original"
			r.PendingFingerprint = "digest"
			personal := r.currentWorld()
			b := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, generation: 3, id: r.PendingOperationID, fingerprint: r.PendingFingerprint, progressionActor: reference(r, &r.Player), sourceBefore: r.Role.SourceSeq}
			q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 15}
			contract, _ := persistence.GrowthContract(w.e.Store.Cfg.Fixture)
			role, g, _ := domain.ApplyExperience(r.Role, 15, contract)
			role.SourceSeq = q.SourceSeq
			role.Revision++
			op := persistence.Operation{Kind: "settlement", RoleID: role.ID, SourceSeq: q.SourceSeq, Revision: role.Revision, Settlement: &persistence.SettlementReceipt{RuleVersion: 20, Experience: &g}}
			switch name {
			case "dead":
				r.Player.HP = 0
			case "life":
				r.Player.Life++
			case "world":
				town := r.newWorld(2)
				r.TownWorld = town
				r.worldState = town
			case "replayed":
				op.Revision--
			case "migration_only":
				g.LevelBefore = g.LevelAfter
				g.ActualGranted = 0
				g.ToProgress = 0
				g.Rejected = g.RawExperience
			case "old_auth":
				r.Auth.SessionEpoch++
			}
			hp, mp, life := r.Player.HP, r.Player.MP, r.Player.Life
			w.finishSettlementReceipt(b, q, role, op, nil, &job{}, nil)
			if r.Player.HP != hp || r.Player.MP != mp || r.Player.Life != life {
				t.Fatal("非原新奖励升级改变身体", name)
			}
		})
	}
}

func TestGrowthPetTrainingUsesConfirmedActualInsteadOfRawOrLegacyMigration(t *testing.T) {
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now)
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	enableGrowthUnit(w, r)
	r.Role.Level = 50
	r.Role.PendingExperience = 95
	credit := w.capturePetTraining(r, "kill")
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 15}
	if credit == nil || !w.preparePetTraining(r, credit, q) || credit.ActualEXP != 5 {
		t.Fatal("预估没有应用池容量")
	}
	r.Role.SourceSeq = q.SourceSeq
	if w.awardPetTraining(r, credit, q) || credit.Applied || pet.PetRuntime.Experience != 0 {
		t.Fatal("没有原确认回执仍训练或消耗credit")
	}
	g := &persistence.ExperienceGrantReceipt{RawExperience: 15, ActualGranted: 3, ToPool: 3, Rejected: 12}
	if !w.awardPetTraining(r, credit, q, g) || pet.PetRuntime.Experience != 9 || credit.ActualEXP != 3 {
		t.Fatal("没有以最终确认actual为准")
	}
	if w.awardPetTraining(r, credit, q, g) || pet.PetRuntime.Experience != 9 {
		t.Fatal("重复原credit训练")
	}
}
