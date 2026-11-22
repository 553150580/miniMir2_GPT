// 本文件回归角色升级恢复、原生命与原世界验证，以及宠物训练使用实际确认经验。
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

// enableGrowthUnit 启用成长单位。
// w 记录房间执行器，r 指向当前执行器拥有并更新的目标房间。
func enableGrowthUnit(w *worker, r *room) {
	w.e.Store.Cfg.Fixture.RuleVersion = 20
	w.e.Store.Cfg.Fixture.Progression = devconfig.ProgressionRules{CurveVersion: "unit-growth-20", InitialLevelCap: 50, RebirthLevelCapIncrement: 1, OperatingLevelCap: 50, CatalogMaxLevel: 50, PendingPoolPolicy: "one_next_level", NewOverflowPolicy: "reject_new_with_receipt", LegacyOverflowPolicy: "preserve_frozen", LevelUpResources: "source_full_if_alive_original_birth"}
	// i 标记循环或列表中的当前位置。
	for i := 0; i < 50; i++ {
		w.e.Store.Cfg.Fixture.Progression.ExpPerLevel = append(w.e.Store.Cfg.Fixture.Progression.ExpPerLevel, 10)
	}
	w.e.Store.Cfg.Fixture.Progression.ExpPerLevel[49] = 100
	r.Role.Level = 1
	r.Role.Experience = 0
	r.Role.ProgressionFormatVersion = 1
	r.Role.ProgressionCurveVersion = "unit-growth-20"
}

// TestGrowthConfirmedOriginalBirthRestoresOnceAfterUnknownAndInventory 核对「成长结果早于完整Inventory」与「新升级未恢复原身体」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestGrowthConfirmedOriginalBirthRestoresOnceAfterUnknownAndInventory(t *testing.T) {
	// w 接收回收所有者供后续步骤使用，r 接收回收所有者供后续步骤使用，c 接收回收所有者供后续步骤使用。
	w, r, c, _, _ := recycleOwner(t)
	enableGrowthUnit(w, r)
	r.Player.HP, r.Player.MP = 2, 1
	r.Pending = true
	r.SettlementGeneration = 3
	r.PendingOperationID = "source-original"
	r.PendingFingerprint = "digest"
	// b 建立带初始值的结算会话绑定，供本段继续处理。
	b := settlementBinding{room: r, auth: r.Auth, world: r.currentWorld().Ref, generation: 3, id: r.PendingOperationID, fingerprint: r.PendingFingerprint, progressionActor: reference(r, &r.Player), sourceBefore: r.Role.SourceSeq}
	// q 组装结算所需的数据。
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 15}
	// contract 保存由成长规则合同取得的规则合同，err 接收成长规则合同过程中可能产生的错误，供后续失败分支处理。
	contract, err := persistence.GrowthContract(w.e.Store.Cfg.Fixture)
	if err != nil {
		t.Fatal(err)
	}
	// role 保存由应用经验取得的角色，grant 保存由应用经验取得的授予。
	role, grant, err := domain.ApplyExperience(r.Role, 15, contract)
	if err != nil {
		t.Fatal(err)
	}
	role.SourceSeq = q.SourceSeq
	role.Revision++
	// op 组装操作所需的数据。
	op := persistence.Operation{Kind: "settlement", RoleID: role.ID, SourceSeq: q.SourceSeq, Revision: role.Revision, Settlement: &persistence.SettlementReceipt{RuleVersion: 20, Experience: &grant}}
	// retry 保存当前处理所用的重试。
	retry := &job{}
	w.finishSettlementReceipt(b, q, role, op, persistence.Fail(p.CodeOperationPending, "unknown"), retry, nil)
	if !r.Pending || r.Player.HP != 2 || r.Player.MP != 1 {
		t.Fatal("未知提交提前恢复")
	}
	w.finishSettlementReceipt(b, q, role, op, nil, retry, nil)
	if r.Pending || r.Player.HP != r.Player.MaxHP || r.Player.MP != r.Player.MaxMP || r.Player.Life != 0 {
		t.Fatal("新升级未恢复原身体")
	}
	// inv 保存当前处理所用的“inv”，event 保存当前处理所用的事件。
	inv, event := -1, -1
	// i 定位协议帧集合中正在处理的成员，f 接收遍历协议帧集合时当前的元素。
	for i, f := range c.frames {
		if f.ID == p.MsgS2CInventory {
			inv = i
		}
		if f.ID == p.MsgS2CExperienceGrantResult {
			// x 记录经验赋予道具或权限结果。
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

// TestGrowthRecoveryRejectsDeadChangedBirthWorldAndReplay 核对「非原新奖励升级改变身体」所标示的断言。
// t 提供断言失败报告、子用例和清理注册。
func TestGrowthRecoveryRejectsDeadChangedBirthWorldAndReplay(t *testing.T) {
	// name 接收遍历文本列表时当前的元素。
	for _, name := range []string{"dead", "life", "world", "replayed", "migration_only", "old_auth"} {
		// t 提供断言失败报告、子用例和清理注册。
		t.Run(name, func(t *testing.T) {
			// w 接收回收所有者供后续步骤使用，r 接收回收所有者供后续步骤使用。
			w, r, _, _, _ := recycleOwner(t)
			enableGrowthUnit(w, r)
			r.Player.HP, r.Player.MP = 2, 1
			r.Pending = true
			r.SettlementGeneration = 3
			r.PendingOperationID = "original"
			r.PendingFingerprint = "digest"
			// personal 保存由当前状态场景世界取得的个人。
			personal := r.currentWorld()
			// b 建立带初始值的结算会话绑定，供本段继续处理。
			b := settlementBinding{room: r, auth: r.Auth, world: personal.Ref, generation: 3, id: r.PendingOperationID, fingerprint: r.PendingFingerprint, progressionActor: reference(r, &r.Player), sourceBefore: r.Role.SourceSeq}
			// q 组装结算所需的数据。
			q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 15}
			// contract 保存由成长规则合同取得的规则合同。
			contract, _ := persistence.GrowthContract(w.e.Store.Cfg.Fixture)
			// role 保存由应用经验取得的角色，g 保存应用经验返回的结果。
			role, g, _ := domain.ApplyExperience(r.Role, 15, contract)
			role.SourceSeq = q.SourceSeq
			role.Revision++
			// op 组装操作所需的数据。
			op := persistence.Operation{Kind: "settlement", RoleID: role.ID, SourceSeq: q.SourceSeq, Revision: role.Revision, Settlement: &persistence.SettlementReceipt{RuleVersion: 20, Experience: &g}}
			switch name {
			case "dead":
				r.Player.HP = 0
			case "life":
				r.Player.Life++
			case "world":
				// town 保存由创建场景世界取得的城镇。
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
			// hp 保存当前处理所用的生命值，mp 保存当前处理所用的魔法值，life 保存当前处理所用的生命代次。
			hp, mp, life := r.Player.HP, r.Player.MP, r.Player.Life
			w.finishSettlementReceipt(b, q, role, op, nil, &job{}, nil)
			if r.Player.HP != hp || r.Player.MP != mp || r.Player.Life != life {
				t.Fatal("非原新奖励升级改变身体", name)
			}
		})
	}
}

// TestGrowthPetTrainingUsesConfirmedActualInsteadOfRawOrLegacyMigration 核验预估应应用池容量，用诊断「没有原确认回执仍训练或消耗credit」定位对应断言。
// t 提供断言失败报告、子用例和清理注册。
func TestGrowthPetTrainingUsesConfirmedActualInsteadOfRawOrLegacyMigration(t *testing.T) {
	// w 接收场景世界房间执行器供后续步骤使用，r 接收场景世界房间执行器供后续步骤使用，personal 保存由场景世界房间执行器取得的个人。
	// now 保存由场景世界房间执行器取得的当前时间。
	w, r, _, personal, _, now := worldWorker(t)
	enablePetRuntimeTest(t, w, r)
	w.applyDamageInWorld(r, personal, personal.Monsters[0], r.Pet, r.Pet.HP, now)
	// pet 保存由生成或出生点运行时宠物取得的宠物。
	pet := spawnRuntimePet(t, w, r, personal, 0, now)
	enableGrowthUnit(w, r)
	r.Role.Level = 50
	r.Role.PendingExperience = 95
	// credit 保存由“capture”宠物训练取得的入账。
	credit := w.capturePetTraining(r, "kill")
	// q 组装结算所需的数据。
	q := persistence.Settlement{SourceSeq: r.Role.SourceSeq + 1, Experience: 15}
	if credit == nil || !w.preparePetTraining(r, credit, q) || credit.ActualEXP != 5 {
		t.Fatal("预估没有应用池容量")
	}
	r.Role.SourceSeq = q.SourceSeq
	if w.awardPetTraining(r, credit, q) || credit.Applied || pet.PetRuntime.Experience != 0 {
		t.Fatal("没有原确认回执仍训练或消耗credit")
	}
	// g 保存当前处理所用的g。
	g := &persistence.ExperienceGrantReceipt{RawExperience: 15, ActualGranted: 3, ToPool: 3, Rejected: 12}
	if !w.awardPetTraining(r, credit, q, g) || pet.PetRuntime.Experience != 9 || credit.ActualEXP != 3 {
		t.Fatal("没有以最终确认actual为准")
	}
	if w.awardPetTraining(r, credit, q, g) || pet.PetRuntime.Experience != 9 {
		t.Fatal("重复原credit训练")
	}
}
