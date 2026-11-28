// 本文件通过实际 MongoDB、WebSocket 和个人首领验证炼体伤害及吸收的结算。
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

// TestBodyHitsMongoWS 从正常报价升级，验证重放、挑战快照、飘字及真正扣血。
func TestBodyHitsMongoWS(t *testing.T) {
	trial, player := startBodyHitsTrial(t, true, func(f *devconfig.Fixture) {
		// 固定双方首击的攻击与防御，避免随机结果掩盖扩展伤害的先后顺序。
		for index := range f.PlayerGrowth {
			if growth := &f.PlayerGrowth[index]; growth.JobID == 0 {
				growth.AttackDivisor, growth.DefenceMaxDivisor = 0.125, 0
			}
		}
		for index := range f.Players {
			if job := &f.Players[index]; job.JobID == 0 {
				job.DefenceMin, job.DefenceMax, job.AttackMS = 0, 0, 5000
			}
		}
		for index := range f.Monsters {
			f.Monsters[index].AttackMin, f.Monsters[index].AttackMax = 0, 0
		}
		boss := f.PersonalBoss
		boss.Map.PlayerSpawn, boss.Map.MonsterSpawns = devconfig.Point{X: 5, Y: 5}, []devconfig.Point{{X: 6, Y: 5}}
		boss.Monster.HP, boss.Monster.AttackMS = 2000, 5000
		boss.Monster.Agility, boss.Monster.Accuracy, boss.Monster.AttackMin, boss.Monster.AttackMax = 0, 10000, 120, 120
		boss.Monster.DefenceMin, boss.Monster.DefenceMax = 21, 21
		if !f.ValidPersonalBoss() {
			t.Fatal("个人首领对拍参数无效")
		}
	})
	roleID := domain.Hex(player.summary.Key.RoleId)
	c := trial.attach(t, &player, false)
	initial := proto.CloneOf(challengePlayerDeathBody(c))
	quote := requestBodyTraining(t, c, devconfig.BodyDamage)
	if quote.Terms.Gold != 2000 || quote.Terms.BoundGold != 1000 || quote.Terms.TargetBonus != 100 {
		t.Fatal("伤害首级报价或钱包拆分不符", quote.Terms)
	}
	commit := confirmBodyTraining(c, quote)
	c.send(p.MsgC2SBodyTrainingCommit, commit)
	stored := waitBodyTrainingOperation(t, trial, roleID, domain.Hex(commit.Operation.OpId))
	scene, binding := c.entry.SceneEpoch, c.binding
	c.close()
	c = trial.attach(t, &player, false)
	replay := c.asset(p.MsgC2SBodyTrainingCommit, commit)
	if c.entry.SceneEpoch != scene || c.binding == binding || replay.ErrorCode != 0 ||
		!proto.Equal(replay.BodyTraining, stored.BodyTraining.Proto()) || replay.BodyTraining.GetAfterLevel() != 1 {
		t.Fatal("没有在新连接取回同一笔伤害炼体结果", replay)
	}
	quote = requestBodyTraining(t, c, devconfig.BodyAbsorption)
	replay = c.asset(p.MsgC2SBodyTrainingCommit, confirmBodyTraining(c, quote))
	if replay.ErrorCode != 0 || !replay.BodyTraining.GetSuccess() || replay.BodyTraining.GetAfterLevel() != 1 ||
		quote.Terms.Gold != 1500 || quote.Terms.BoundGold != 0 || quote.Terms.TargetBonus != 100 {
		t.Fatal("吸收首级报价或确认不符", replay, quote)
	}
	c.inventoryAt(replay.NewRevision)
	levels := c.inventory.GetBodyTraining()
	if levels.GetDamageLevel() != 1 || levels.GetAbsorptionLevel() != 1 || levels.GetHealthLevel() != 0 || levels.GetManaLevel() != 0 {
		t.Fatal("各系炼体等级没有独立同步", levels)
	}
	travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
		stats := challengePlayerDeathBody(c).GetCombatAttributes()
		return f.ID == p.MsgS2CSceneBaseline && stats.GetExpandedDamageBp() == 100 && stats.GetExpandedAbsorptionBp() == 100
	})
	normal := challengePlayerDeathBody(c)
	if normal.HpMax != initial.HpMax || normal.MpMax != initial.MpMax || normal.Life != initial.Life ||
		normal.GetCombatAttributes().GetPhysicalAttackMin() != 160 || normal.GetCombatAttributes().GetPhysicalAttackMax() != 160 {
		t.Fatal("扩展命中倍率串入原生命、魔法或面板攻击", normal)
	}

	req := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: c.entry.SceneEpoch,
		ChallengeId: domain.ID(), TemplateId: trial.store.Cfg.Fixture.PersonalBoss.TemplateID})
	frame := travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool { return f.Request == req })
	if frame.ID != p.MsgS2CPersonalBossState || frame.Error != 0 {
		t.Fatalf("个人首领预约失败：%+v", frame)
	}
	var booked pb.PersonalBossState
	must(t, proto.Unmarshal(frame.Payload, &booked))
	travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	body, boss := proto.CloneOf(challengePlayerDeathBody(c)), proto.CloneOf(travelEntity(c.scene, 3))
	if !proto.Equal(body.GetCombatAttributes(), normal.GetCombatAttributes()) || boss == nil || body.Hp <= 119 {
		t.Fatal("挑战初始快照没有固定两组已确认属性和对拍生命", body, boss)
	}
	travelAcknowledge(c, false, true)
	var outgoing, incoming int64
	travelUntilWithin(c, 8*time.Second, func(f wire.Frame) bool {
		if f.ID != p.MsgS2CCombatImpact {
			return false
		}
		var impact pb.CombatImpact
		must(t, proto.Unmarshal(f.Payload, &impact))
		if impact.Healing || impact.SceneEpoch != c.entry.SceneEpoch {
			return false
		}
		if outgoing == 0 && impact.CasterHandle == body.EntityHandle && impact.TargetHandle == boss.EntityHandle && impact.TargetLife == boss.Life {
			outgoing = impact.Amount
		}
		if incoming == 0 && impact.CasterHandle == boss.EntityHandle && impact.TargetHandle == body.EntityHandle && impact.TargetLife == body.Life {
			incoming = impact.Amount
		}
		return outgoing > 0 && incoming > 0
	})
	if outgoing != 140 || incoming != 119 {
		t.Fatalf("扩展伤害与飘字对拍错误，首领承伤=%d 角色承伤=%d", outgoing, incoming)
	}
	travelUntilWithin(c, 3*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && challengePlayerDeathBody(c).Hp == body.Hp-incoming &&
			travelEntity(c.scene, 3).GetHp() == boss.Hp-outgoing
	})
	t.Logf("个人首领净伤139→%d，角色净伤120→%d，与权威基线扣血一致", outgoing, incoming)
	c.send(p.MsgC2SPersonalBossLeave, &pb.PersonalBossLeave{SceneEpoch: c.entry.SceneEpoch, ChallengeId: booked.ChallengeId})
	returnFromWorldBossTest(c, booked.ChallengeId)
	role, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	audits, err := trial.store.DB.Collection("audit_outbox").CountDocuments(trial.ctx,
		bson.M{"role_id": roleID, "kind": persistence.AssetBodyTrainingCommit})
	must(t, err)
	if role.BodyTrainingLevels() != (domain.BodyTrainingState{Damage: 1, Absorption: 1}) ||
		role.Wallet.Gold != 2500 || role.Wallet.BoundGold != 0 || audits != 2 {
		t.Fatal("重连、挑战和两系扣费后的永久账本不符", role.BodyTrainingLevels(), role.Wallet, audits)
	}
}

// startBodyHitsTrial 只写隔离角色等级和钱包，两系由客户端原操作取得。
func startBodyHitsTrial(t *testing.T, enabled bool, configure func(*devconfig.Fixture)) (worldBossRewardTrial, worldBossRewardPlayer) {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if !enabled {
			return
		}
		path := os.Getenv("MINIMIR_BODY_HITS_TEST_FIXTURE")
		raw, err := os.ReadFile(path)
		must(t, err)
		var content devconfig.Fixture
		must(t, json.Unmarshal(raw, &content))
		must(t, content.Validate())
		content.Hash = persistence.Hash(raw)
		if content.ExpandedHitBudget == nil || content.BodyTraining == nil || len(content.BodyTraining.Tracks) != 4 || content.PersonalBoss == nil {
			t.Fatal("隔离候选缺少炼体四系、扩展命中属性预算或个人首领")
		}
		if configure != nil {
			configure(&content)
		}
		cfg.Fixture, cfg.FixtureFile = content, path
	})
	trial := worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
	player := trial.create(t, "伤害炼体", time.Now().Add(-time.Hour), false)
	_, err := store.DB.Collection("roles").UpdateByID(ctx, domain.Hex(player.summary.Key.RoleId), bson.M{"$set": bson.M{
		"level": 20, "wallet": domain.Wallet{Gold: 6000, BoundGold: 1000},
	}})
	must(t, err)
	return trial, player
}
