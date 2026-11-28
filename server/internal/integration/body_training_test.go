// 本文件用真实 MongoDB 和 WebSocket 验证炼体的角色属性、首领快照及重连。
package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestBodyTrainingMongoWS 通过原角色连接升级两系，并核对新连与首领原快照。
func TestBodyTrainingMongoWS(t *testing.T) {
	trial, player := startBodyTrainingTrial(t, true)
	roleID := domain.Hex(player.summary.Key.RoleId)
	c := trial.attach(t, &player, true)
	raw := proto.CloneOf(challengePlayerDeathBody(c))
	quote := requestBodyTraining(t, c, devconfig.BodyHealth)
	if quote.Terms.Gold != 500 || quote.Terms.BoundGold != 1000 || quote.Terms.SuccessPpm != 1_000_000 {
		t.Fatal("生命首级报价未正确拆分两类金币", quote.Terms)
	}
	first := confirmBodyTraining(c, quote)
	c.send(p.MsgC2SBodyTrainingCommit, first)
	committed := waitBodyTrainingOperation(t, trial, roleID, domain.Hex(first.Operation.OpId))
	// 首个连接在数据库成交后断开，保留原经济上下文并获取同一角色的新票。
	scene, binding := c.entry.SceneEpoch, c.binding
	c.close()
	c = trial.attach(t, &player, false)
	if c.entry.SceneEpoch != scene || c.binding == binding {
		t.Fatal("重连未恢复原场景并得到新附着")
	}
	replayed := c.asset(p.MsgC2SBodyTrainingCommit, first)
	if replayed.ErrorCode != 0 || !proto.Equal(replayed.BodyTraining, committed.BodyTraining.Proto()) ||
		replayed.BodyTraining.GetAfterLevel() != 1 || !replayed.BodyTraining.GetSuccess() {
		t.Fatal("重连未取回同一生命系回执", replayed)
	}
	trained := challengePlayerDeathBody(c)
	if trained.HpMax != raw.HpMax+2 || trained.MpMax != raw.MpMax || trained.Life != raw.Life ||
		c.inventory.GetBodyTraining().GetHealthLevel() != 1 || c.inventory.GetBodyTraining().GetManaLevel() != 0 {
		t.Fatal("生命系未在原身体上只提升生命上限", trained)
	}

	manaQuote := requestBodyTraining(t, c, devconfig.BodyMana)
	manaCommit := confirmBodyTraining(c, manaQuote)
	mana := c.asset(p.MsgC2SBodyTrainingCommit, manaCommit)
	if mana.ErrorCode != 0 || !mana.BodyTraining.GetSuccess() || mana.BodyTraining.GetAfterLevel() != 1 ||
		mana.BodyTraining.GetTerms().GetGold() != 1500 || mana.BodyTraining.GetTerms().GetBoundGold() != 0 {
		t.Fatal("魔法系初次确认或钱包拆分不符", mana)
	}
	c.inventoryAt(mana.NewRevision)
	travelUntilWithin(c, 4*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && challengePlayerDeathBody(c).MpMax == raw.MpMax+1
	})
	normal := proto.CloneOf(challengePlayerDeathBody(c))
	if normal.HpMax != raw.HpMax+2 || normal.Life != raw.Life ||
		c.inventory.GetBodyTraining().GetHealthLevel() != 1 || c.inventory.GetBodyTraining().GetManaLevel() != 1 {
		t.Fatal("两系等级、数值或原身体没有独立保存")
	}
	beforeBoss, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	if beforeBoss.Wallet.Gold != 3000 || beforeBoss.Wallet.BoundGold != 0 {
		t.Fatal("成功与重连产生的钱包开支不符", beforeBoss.Wallet)
	}
	req := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: c.entry.SceneEpoch,
		ChallengeId: domain.ID(), TemplateId: trial.store.Cfg.Fixture.PersonalBoss.TemplateID})
	var start pb.PersonalBossState
	frame := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.Request == req })
	if frame.ID != p.MsgS2CPersonalBossState || frame.Error != 0 {
		t.Fatalf("个人首领未正常预约：%+v", frame)
	}
	must(t, proto.Unmarshal(frame.Payload, &start))
	travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss && c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	bossBody := challengePlayerDeathBody(c)
	if bossBody.HpMax != normal.HpMax || bossBody.MpMax != normal.MpMax {
		t.Fatal("首领原快照与普通场景没有复用同一属性", bossBody)
	}
	travelAcknowledge(c, false, false)
	bossReplay := c.asset(p.MsgC2SBodyTrainingCommit, manaCommit)
	if !proto.Equal(bossReplay.BodyTraining, mana.BodyTraining) || bossReplay.ErrorCode != 0 {
		t.Fatal("挑战场景无法只读核对原回执", bossReplay)
	}
	c.send(p.MsgC2SPersonalBossLeave, &pb.PersonalBossLeave{SceneEpoch: c.entry.SceneEpoch, ChallengeId: start.ChallengeId})
	returnFromWorldBossTest(c, start.ChallengeId)
	after, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	if after.Wallet != beforeBoss.Wallet || after.EconomicSeq != beforeBoss.EconomicSeq ||
		after.BodyTrainingLevels() != beforeBoss.BodyTrainingLevels() {
		t.Fatal("挑战中重放造成新的扣费、升级或经济操作")
	}
}

// startBodyTrainingTrial 只准备隔离角色等级和钱包，两个等级必须由原确认取得。
func startBodyTrainingTrial(t *testing.T, enabled bool) (worldBossRewardTrial, worldBossRewardPlayer) {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if !enabled {
			return
		}
		path := os.Getenv("MINIMIR_BODY_TRAINING_TEST_FIXTURE")
		raw, err := os.ReadFile(path)
		must(t, err)
		var f devconfig.Fixture
		must(t, json.Unmarshal(raw, &f))
		f.Hash = persistence.Hash(raw)
		must(t, f.Validate())
		if f.BodyTraining == nil || len(f.BodyTraining.Tracks) != 2 {
			t.Fatal("隔离炼体缺少两系数据")
		}
		cfg.Fixture, cfg.FixtureFile = f, path
	})
	trial := worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
	player := trial.create(t, "炼体验收", time.Now().Add(-time.Hour), false)
	_, err := store.DB.Collection("roles").UpdateByID(ctx, domain.Hex(player.summary.Key.RoleId), bson.M{"$set": bson.M{
		"level": 20, "wallet": domain.Wallet{Gold: 5000, BoundGold: 1000},
	}})
	must(t, err)
	return trial, player
}

// requestBodyTraining 只接收原场景请求的炼体报价。
func requestBodyTraining(t *testing.T, c *client, track uint32) *pb.BodyTrainingQuote {
	t.Helper()
	seq := c.send(p.MsgC2SBodyTrainingQuote, &pb.BodyTrainingQuoteRequest{SceneEpoch: c.entry.SceneEpoch,
		UiSessionId: domain.Bytes(domain.ID()), Track: track})
	frame := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.Request == seq })
	if frame.ID != p.MsgS2CBodyTrainingQuote || frame.Error != 0 {
		t.Fatalf("炼体报价失败：%+v", frame)
	}
	var q pb.BodyTrainingQuote
	must(t, proto.Unmarshal(frame.Payload, &q))
	return &q
}

// confirmBodyTraining 保留原报价修订和原界面身份。
func confirmBodyTraining(c *client, q *pb.BodyTrainingQuote) *pb.BodyTrainingCommitRequest {
	context := c.operation()
	context.ExpectedRevision = q.RoleRevision
	return &pb.BodyTrainingCommitRequest{QuoteId: q.QuoteId, UiSessionId: q.UiSessionId, Track: q.Terms.Track, Operation: context}
}

// waitBodyTrainingOperation 等已受理原确认持久化后，测试可故意断开未读回执的连接。
func waitBodyTrainingOperation(t *testing.T, trial worldBossRewardTrial, roleID, opID string) persistence.Operation {
	t.Helper()
	ctx, cancel := context.WithTimeout(trial.ctx, 7*time.Second)
	defer cancel()
	var op persistence.Operation
	for {
		err := trial.store.DB.Collection("operations").FindOne(ctx, bson.M{"_id": "asset:" + roleID + ":" + opID}).Decode(&op)
		if !errors.Is(err, mongo.ErrNoDocuments) || ctx.Err() != nil {
			must(t, err)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if op.Code != 0 || op.BodyTraining == nil {
		t.Fatal("原炼体没有持久化确定回执", op)
	}
	return op
}
