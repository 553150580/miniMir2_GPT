// 本文件用真实个人首领战斗和 WebSocket 验证转生收费、等级及重连对账。
package integration

import (
	"context"
	"encoding/json"
	"os"
	"slices"
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

// TestRebirthPersonalBossMongoWS 从原首领取得凭证，验证旧面板拒绝和断线后原操作重放。
func TestRebirthPersonalBossMongoWS(t *testing.T) {
	trial, player := startRebirthTrial(t, true)
	c := trial.attach(t, &player, true)
	roleID := domain.Hex(player.summary.Key.RoleId)
	stage := trial.store.Cfg.Fixture.Rebirth.Stages[0]
	quote := func(c *client) uint32 {
		return c.send(p.MsgC2SRebirthQuote, &pb.RebirthQuoteRequest{SceneEpoch: c.entry.SceneEpoch, UiSessionId: domain.Bytes(domain.ID())})
	}
	travelExpectError(c, p.MsgC2SRebirthQuote, quote(c), p.CodeRebirthInputs)
	win := winRebirthPersonalBoss(t, c, stage.ChallengeTemplateID)
	original, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	oldQuote := readRebirthWS(t, c, quote(c))
	scene, binding := c.entry.SceneEpoch, c.binding
	c.close()
	c = trial.attach(t, &player, false)
	if c.entry.SceneEpoch != scene || c.binding == binding {
		t.Fatal("新连接未恢复原地图以及原角色")
	}
	stale := c.asset(p.MsgC2SRebirthCommit, confirmRebirthWS(c, oldQuote))
	if stale.ErrorCode != p.CodeRebirthQuoteChanged || stale.Rebirth != nil {
		t.Fatal("旧会话的未成交报价必须拒绝", stale)
	}
	c.inventoryAt(stale.NewRevision)
	fresh := readRebirthWS(t, c, quote(c))
	expect := fresh.GetPreview()
	if domain.Hex(expect.GetChallengeId()) != win || expect.GetGold() != 400 || expect.GetBoundGold() != 600 ||
		expect.GetCredential().GetItemId() != stage.CredentialItemID || len(expect.GetMaterials()) != 1 ||
		expect.Materials[0].GetQuantity() != stage.MaterialCost || expect.GetTransition().GetPreviousCap() != 50 ||
		expect.GetTransition().GetNewCap() != 51 || expect.GetTransition().GetNewLevel() != 51 ||
		expect.GetTransition().GetReleasedExperience() != original.PendingExperience {
		t.Fatal("原场次、钱包拆分、材料或待用经验与报价不符", expect)
	}
	confirmation := confirmRebirthWS(c, fresh)
	c.send(p.MsgC2SRebirthCommit, confirmation)
	// 等待数据库成交后中断原连接，不消费原提交的网络回执。
	waitCtx, cancel := context.WithTimeout(trial.ctx, 7*time.Second)
	defer cancel()
	key := bson.M{"_id": "asset:" + roleID + ":" + domain.Hex(confirmation.Operation.OpId)}
	var op persistence.Operation
	for {
		err := trial.store.DB.Collection("operations").FindOne(waitCtx, key).Decode(&op)
		if err == nil || waitCtx.Err() != nil {
			must(t, err)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.close()
	c = trial.attach(t, &player, false)
	result := c.asset(p.MsgC2SRebirthCommit, confirmation)
	if result.GetErrorCode() != 0 || !proto.Equal(result.GetRebirth(), expect) || !proto.Equal(result.GetRebirth(), op.Rebirth.Proto()) ||
		c.inventory.GetProgression().GetRebirthCount() != 1 || c.inventory.GetLevel() != 51 || c.inventory.GetProgression().GetLevelCap() != 51 {
		t.Fatal("断线重放、原原子回执和角色成长未能对账", result, c.inventory.GetProgression())
	}
	travelExpectError(c, p.MsgC2SRebirthQuote, quote(c), p.CodeRebirthUnavailable)
	after, err := trial.store.Role(trial.ctx, roleID)
	must(t, err)
	if after.PendingExperience != 0 || after.Experience != 0 || after.Wallet.Gold != original.Wallet.Gold-400 ||
		after.Wallet.BoundGold != original.Wallet.BoundGold-600 || after.EconomicSeq != original.EconomicSeq+2 ||
		after.SourceSeq != original.SourceSeq || !slices.Equal(after.Equipped, original.Equipped) {
		t.Fatal("存在重复扣费、经验重复释放或已穿装备变化")
	}
	for _, item := range after.Items {
		if item.ItemID == stage.CredentialItemID || item.ItemID == stage.MaterialItemID &&
			(item.Quantity != 1 || item.Version != int64(expect.Materials[0].Version)+1 || !item.Bound || item.SourceID != win) {
			t.Fatal("凭证未消耗或原绑定材料余量不符", item)
		}
	}
	t.Logf("真实场次 %s，释放 %d 经验，50→51，原操作仅成交一次", win, expect.Transition.ReleasedExperience)
}

// startRebirthTrial 只播种封顶等级、经验池和可核对钱包，凭证与材料从正式战斗入口取得。
func startRebirthTrial(t *testing.T, enabled bool) (worldBossRewardTrial, worldBossRewardPlayer) {
	t.Helper()
	fixtureFile := os.Getenv("MINIMIR_REBIRTH_TEST_FIXTURE")
	ctx, store, account, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if !enabled {
			return
		}
		raw, err := os.ReadFile(fixtureFile)
		must(t, err)
		var fixture devconfig.Fixture
		must(t, json.Unmarshal(raw, &fixture))
		fixture.Hash = persistence.Hash(raw)
		must(t, fixture.Validate())
		if fixture.Rebirth == nil || !fixture.Progression.RebirthEnabled {
			t.Fatal("缺少隔离转生档位")
		}
		cfg.Fixture, cfg.FixtureFile = fixture, fixtureFile
	})
	trial := worldBossRewardTrial{ctx: ctx, store: store, endpoint: account, route: route}
	// 即便角色有特权且普通附件可交易，转生凭证与材料仍绑定。
	player := trial.create(t, "转生验收", time.Now().Add(time.Hour), false)
	fixture := &store.Cfg.Fixture
	_, err := store.DB.Collection("roles").UpdateByID(ctx, domain.Hex(player.summary.Key.RoleId), bson.M{"$set": bson.M{
		"level": 50, "experience": 0, "pending_experience": fixture.Progression.ExpPerLevel[49],
		"progression_format_version": 1, "progression_curve_version": fixture.Progression.CurveVersion,
		"wallet": domain.Wallet{Gold: 1200, BoundGold: 600},
	}})
	must(t, err)
	return trial, player
}

// winRebirthPersonalBoss 通过原预约、基线与自动战斗击杀真实模板。
func winRebirthPersonalBoss(t *testing.T, c *client, template uint32) string {
	t.Helper()
	req := c.send(p.MsgC2SPersonalBossEnter, &pb.PersonalBossEnter{SceneEpoch: c.entry.SceneEpoch,
		ChallengeId: domain.ID(), TemplateId: template})
	frame := travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool { return f.Request == req })
	var booking pb.PersonalBossState
	if frame.ID != p.MsgS2CPersonalBossState || frame.Error != 0 {
		t.Fatalf("个人首领预约失败：%+v", frame)
	}
	must(t, proto.Unmarshal(frame.Payload, &booking))
	travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && c.entry.SceneKind == devconfig.SceneKindPersonalBoss &&
			c.scene.SceneEpoch == c.entry.SceneEpoch
	})
	travelAcknowledge(c, false)
	var victory pb.PersonalBossState
	travelUntilWithin(c, 30*time.Second, func(f wire.Frame) bool {
		if f.ID != p.MsgS2CPersonalBossState {
			return false
		}
		must(t, proto.Unmarshal(f.Payload, &victory))
		return victory.State >= 4
	})
	if victory.State != 4 || victory.ChallengeId != booking.ChallengeId {
		t.Fatal("真实个人首领未结算胜利", &victory)
	}
	returnFromWorldBossTest(c, booking.ChallengeId)
	return booking.ChallengeId
}

// readRebirthWS 读取同一个 WebSocket 原报价请求的响应。
func readRebirthWS(t *testing.T, c *client, req uint32) *pb.RebirthQuote {
	t.Helper()
	frame := travelUntilWithin(c, 5*time.Second, func(f wire.Frame) bool { return f.Request == req })
	if frame.ID != p.MsgS2CRebirthQuote || frame.Error != 0 {
		t.Fatalf("转生报价失败：%+v", frame)
	}
	var quote pb.RebirthQuote
	must(t, proto.Unmarshal(frame.Payload, &quote))
	return &quote
}

// confirmRebirthWS 使用原报价修订、面板会话和新的经济操作身份。
func confirmRebirthWS(c *client, quote *pb.RebirthQuote) *pb.RebirthCommitRequest {
	op := c.operation()
	op.ExpectedRevision = quote.RoleRevision
	return &pb.RebirthCommitRequest{QuoteId: quote.GetPreview().GetQuoteId(), UiSessionId: quote.UiSessionId, Operation: op}
}
