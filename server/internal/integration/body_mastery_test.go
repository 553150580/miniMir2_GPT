// 本文件用真实 MongoDB 和 WebSocket 验证元宝炼体、断线重放和个人首领控制。
package integration

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestBodyMasteryMongoWS 验证三个首级、一次明确失败及一次重放，之后进入真实首领场。
func TestBodyMasteryMongoWS(t *testing.T) {
	trial, player := startBodyMasteryTrial(t, func(f *devconfig.Fixture) {
		// 调整本轮对拍首领的站位、攻击区间和生命，保留原冻结触发合同。
		boss := f.PersonalBoss
		boss.Map.PlayerSpawn, boss.Map.MonsterSpawns = devconfig.Point{X: 5, Y: 5}, []devconfig.Point{{X: 6, Y: 5}}
		boss.Monster.HP, boss.Monster.AttackMS = 2000, 5000
		boss.Monster.Agility, boss.Monster.Accuracy, boss.Monster.AttackMin, boss.Monster.AttackMax = 0, 10000, 120, 120
		if !f.ValidPersonalBoss() {
			t.Fatal("元宝对拍缺少有效个人首领模板")
		}
	})
	roleID := domain.Hex(player.summary.Key.RoleId)
	c := trial.attach(t, &player, false)
	original := proto.CloneOf(challengePlayerDeathBody(c))
	quote := requestBodyTraining(t, c, devconfig.BodyCritical)
	if !masteryTerms(quote.Terms, 150, 50, 100) {
		t.Fatal("暴击起始报价应使用绑定余额 150 与普通余额 50", quote.Terms)
	}
	commit := confirmBodyTraining(c, quote)
	c.send(p.MsgC2SBodyTrainingCommit, commit)
	stored := waitBodyTrainingOperation(t, trial, roleID, domain.Hex(commit.Operation.OpId))
	scene, binding := c.entry.SceneEpoch, c.binding
	c.close()
	c = trial.attach(t, &player, false)
	replay := c.asset(p.MsgC2SBodyTrainingCommit, commit)
	if c.entry.SceneEpoch != scene || c.binding == binding || replay.ErrorCode != 0 || !replay.GetBodyTraining().GetSuccess() ||
		!proto.Equal(replay.BodyTraining, stored.BodyTraining.Proto()) {
		t.Fatal("元宝尝试重连后未取回原回执", replay)
	}
	for _, track := range []uint32{devconfig.BodySpeed, devconfig.BodyResistance} {
		quote = requestBodyTraining(t, c, track)
		if !masteryTerms(quote.Terms, 0, 200, 50) {
			t.Fatal("攻速、全抗起始报价应只付普通元宝 200", quote.Terms)
		}
		result := c.asset(p.MsgC2SBodyTrainingCommit, confirmBodyTraining(c, quote))
		if result.ErrorCode != 0 || !result.GetBodyTraining().GetSuccess() || result.GetBodyTraining().GetAfterLevel() != 1 {
			t.Fatal("元宝炼体首级未按原确认结算", result)
		}
		c.inventoryAt(result.NewRevision)
	}
	quote = requestBodyTraining(t, c, devconfig.BodyCritical)
	if quote.Terms.SuccessPpm != 900_000 {
		t.Fatal("第二次暴击炼体没有使用 90% 档", quote.Terms)
	}
	// 在隔离 Mongo 选择失败边界，核对读回恰好同一次扣费而且保留档位。
	changed, err := trial.store.DB.Collection("body_training_quotes").UpdateOne(trial.ctx,
		bson.M{"_id": roleID, "quote_id": domain.Hex(quote.QuoteId)}, bson.M{"$set": bson.M{"success_roll": 900_000}})
	must(t, err)
	if changed.MatchedCount != 1 {
		t.Fatal("失败样本未对应当前角色原报价")
	}
	commit = confirmBodyTraining(c, quote)
	failed := c.asset(p.MsgC2SBodyTrainingCommit, commit)
	if failed.ErrorCode != 0 || failed.BodyTraining == nil || failed.BodyTraining.Success || failed.BodyTraining.AfterLevel != 1 {
		t.Fatal("概率失败应扣费并保留原档", failed)
	}
	replay = c.asset(p.MsgC2SBodyTrainingCommit, commit)
	if !proto.Equal(replay.BodyTraining, failed.BodyTraining) || replay.ErrorCode != 0 {
		t.Fatal("已经结算的失败必须重放原回执", replay)
	}
	c.inventoryAt(failed.NewRevision)
	levels := c.inventory.BodyTraining
	if !proto.Equal(levels, &pb.BodyTrainingState{CriticalLevel: 1, SpeedLevel: 1, ResistanceLevel: 1}) {
		t.Fatal("炼体四金币和三个元宝系发生串级", levels)
	}
	travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
		stats := challengePlayerDeathBody(c).GetCombatAttributes()
		return f.ID == p.MsgS2CSceneBaseline && stats.GetCriticalBp() == 100 && stats.GetAttackIntervalReductionBp() == 50 &&
			stats.GetAllResistanceBp() == 50 && stats.GetLuckResistancePpm() == 5000
	})
	body := proto.CloneOf(challengePlayerDeathBody(c))
	if body.AttackIntervalMs != original.AttackIntervalMs-original.AttackIntervalMs/200 ||
		body.HpMax != original.HpMax || body.MpMax != original.MpMax {
		t.Fatal("攻速未缩短实际攻击间隔或串入原生命魔法", original, body)
	}
	t.Logf("普通攻击间隔 %d→%d 毫秒，三元宝系等级均为 1，幸运抵抗 5000 PPM", original.AttackIntervalMs, body.AttackIntervalMs)
	testBodyMasteryBoss(t, c, trial.store.Cfg.Fixture.PersonalBoss.TemplateID, body)

	role, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	audits, err := trial.store.DB.Collection("audit_outbox").CountDocuments(trial.ctx,
		bson.M{"role_id": roleID, "kind": persistence.AssetBodyTrainingCommit})
	must(t, err)
	if role.BodyTrainingLevels() != (domain.BodyTrainingState{Critical: 1, Speed: 1, Resistance: 1}) ||
		role.Wallet != (domain.Wallet{Gold: 5000, BoundGold: 1000, Yuanbao: 350}) || audits != 4 {
		t.Fatal("三次成功和一次概率失败后，重复提交造成支出、金币或审计变化", role.BodyTrainingLevels(), role.Wallet, audits)
	}
}

// masteryTerms 对拍最小单位和为零的两个金币钱包。
func masteryTerms(terms *pb.BodyTrainingTerms, bound, ordinary, target int64) bool {
	return terms != nil && terms.Gold == 0 && terms.BoundGold == 0 &&
		terms.BoundYuanbaoUnits == bound && terms.YuanbaoUnits == ordinary && terms.TargetBonus == target
}

// testBodyMasteryBoss 核对挑战快照和已发生怪物命中后的实际冰冻状态。
func testBodyMasteryBoss(t *testing.T, c *client, template uint32, normal *pb.EntityState) {
	t.Helper()
	request := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: c.entry.SceneEpoch,
		ChallengeId: domain.ID(), TemplateId: template})
	frame := travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool { return f.Request == request })
	if frame.ID != p.MsgS2CPersonalBossState || frame.Error != 0 {
		t.Fatalf("个人首领预约失败：%+v", frame)
	}
	var booked pb.PersonalBossState
	must(t, proto.Unmarshal(frame.Payload, &booked))
	travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	bossBody := proto.CloneOf(challengePlayerDeathBody(c))
	if bossBody.AttackIntervalMs != normal.AttackIntervalMs || !proto.Equal(bossBody.CombatAttributes, normal.CombatAttributes) {
		t.Fatal("挑战未冻结普通地图实际生效的三个元宝系", bossBody)
	}
	travelAcknowledge(c, false, true)
	var frozen *pb.EntityStatusState
	travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
		if f.ID != p.MsgS2CSceneBaseline || challengePlayerDeathBody(c).Hp >= bossBody.Hp {
			return false
		}
		for _, status := range challengePlayerDeathBody(c).Statuses {
			if status.StatusId == devconfig.MasteryFrozen && status.RemainingMs > 0 {
				frozen = status
				return true
			}
		}
		return false
	})
	if frozen.RemainingMs > 995 || frozen.CasterHandle != travelEntity(c.scene, 3).GetEntityHandle() {
		t.Fatal("首领实际攻击未按冰冻抗性缩短 1000 毫秒原状态", frozen)
	}
	t.Logf("真实首领命中后同步冰冻剩余 %d 毫秒，原持续时间 1000 毫秒", frozen.RemainingMs)
	c.send(p.MsgC2SPersonalBossLeave, &pb.PersonalBossLeave{SceneEpoch: c.entry.SceneEpoch, ChallengeId: booked.ChallengeId})
	returnFromWorldBossTest(c, booked.ChallengeId)
}

// startBodyMasteryTrial 只准备隔离钱包及角色等级，三个新等级通过真实经济操作取得。
func startBodyMasteryTrial(t *testing.T, configure func(*devconfig.Fixture)) (worldBossRewardTrial, worldBossRewardPlayer) {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		path := os.Getenv("MINIMIR_BODY_MASTERY_TEST_FIXTURE")
		raw, err := os.ReadFile(path)
		must(t, err)
		var content devconfig.Fixture
		must(t, json.Unmarshal(raw, &content))
		must(t, content.Validate())
		content.Hash = persistence.Hash(raw)
		if content.MasteryCombat == nil || content.BodyTraining == nil || len(content.BodyTraining.Tracks) != 7 || content.PersonalBoss == nil {
			t.Fatal("隔离候选缺少七系、进阶战斗预算或个人首领")
		}
		if configure != nil {
			configure(&content)
		}
		cfg.Fixture, cfg.FixtureFile = content, path
	})
	trial := worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
	player := trial.create(t, "元宝炼体", time.Now().Add(-time.Hour), false)
	_, err := store.DB.Collection("roles").UpdateByID(ctx, domain.Hex(player.summary.Key.RoleId), bson.M{"$set": bson.M{
		"level": 20, "wallet": domain.Wallet{Gold: 5000, BoundGold: 1000, Yuanbao: 1000, BoundYuanbao: 150},
	}})
	must(t, err)
	return trial, player
}
