// 本文件用真实 MongoDB 副本集注入审计冲突，验证转生的整笔回滚与同号幂等。
package persistence

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestRebirthSettlementMongo 核对伪凭证、审计失败、原操作并发以及停开后的重放。
func TestRebirthSettlementMongo(t *testing.T) {
	ctx, _, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_REBIRTH_TEST_FIXTURE")
	f := &game.Cfg.Fixture
	stage := f.Rebirth.Stages[0]
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	source := domain.ID() // 本用例构造原胜利文档以专门测试事务回滚和凭证归属。
	proof := domain.Item{ID: domain.ID(), ItemID: stage.CredentialItemID, Version: 1, Quantity: 1,
		Bound: true, SourceKind: "personal_boss", SourceID: source}
	material := domain.Item{ID: domain.ID(), ItemID: stage.MaterialItemID, Version: 4, Quantity: 1,
		Bound: true, SourceKind: "personal_boss", SourceID: source}
	second := material
	second.ID, second.Quantity = domain.ID(), 3
	_, err := game.DB.Collection("roles").UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{
		"level": 50, "experience": 0, "pending_experience": 100, "progression_format_version": 1,
		"progression_curve_version": f.Progression.CurveVersion, "wallet": domain.Wallet{BoundGold: stage.GoldCost + 20},
		"items": []domain.Item{proof, material, second},
	}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	scene := uint64(412)
	ui := domain.Bytes(domain.ID())
	query := &pb.RebirthQuoteRequest{SceneEpoch: scene, UiSessionId: ui}
	challenges := game.DB.Collection(personalBossCollection)
	challenge := PersonalBossChallenge{ChallengeID: source, RoleID: domain.ID(), TemplateID: stage.ChallengeTemplateID,
		Outcome: PersonalBossDefeated, RewardItems: []domain.Item{proof, material, second}}
	_, err = challenges.InsertOne(ctx, challenge)
	must(err)
	if _, err := game.PreviewRebirth(ctx, auth, query); Code(err) != p.CodeRebirthInputs {
		t.Fatal("别人的胜利记录不能作为转生凭证", err)
	}
	_, err = challenges.UpdateByID(ctx, source, bson.M{"$set": bson.M{"role_id": role.ID}})
	must(err)
	quote, err := game.PreviewRebirth(ctx, auth, query)
	must(err)
	if len(quote.Preview.Materials) != 2 || quote.Preview.Materials[0].Quantity != 1 || quote.Preview.Materials[1].Quantity != 1 {
		t.Fatal("报价未按同一费用使用两个原堆", quote)
	}
	op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
		ExpectedRevision: quote.RoleRevision, ExpectedRuleVersion: f.RuleVersion}
	confirmation := &pb.RebirthCommitRequest{QuoteId: quote.Preview.QuoteId, UiSessionId: ui, Operation: op}
	payload, err := proto.Marshal(confirmation)
	must(err)
	request := AssetRequest{Kind: AssetRebirthCommit, Context: op, RebirthCommit: confirmation, SceneEpoch: scene,
		RebirthAcceptedAt: time.Now(), Fingerprint: Hash(payload)}
	key := bson.M{"_id": "asset:" + role.ID + ":" + domain.Hex(op.OpId)}
	audit := game.DB.Collection("audit_outbox")
	_, err = audit.InsertOne(ctx, key)
	must(err)
	_, _, _, err = game.AssetWithReceipt(ctx, auth, request)
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatal("原审计冲突应使整笔事务失败", err)
	}
	rolledBack, err := game.Role(ctx, role.ID)
	must(err)
	quotes, err := game.DB.Collection(rebirthQuoteCollection).CountDocuments(ctx, bson.M{"_id": role.ID})
	must(err)
	operations, err := game.DB.Collection("operations").CountDocuments(ctx, key)
	must(err)
	if !reflect.DeepEqual(rolledBack, role) || quotes != 1 || operations != 0 {
		t.Fatal("失败未完整恢复角色、原报价或事务回执", quotes, operations)
	}
	_, err = audit.DeleteOne(ctx, key)
	must(err)
	type result struct {
		reply *pb.OperationResult // 服务端经济响应。
		err   error               // 数据库或业务事务错误。
	}
	outcomes := make(chan result, 2)
	for range 2 {
		go func() {
			_, reply, _, err := game.AssetWithReceipt(ctx, auth, request)
			outcomes <- result{reply, err}
		}()
	}
	for range 2 {
		out := <-outcomes
		must(out.err)
		if out.reply.GetErrorCode() != 0 || !proto.Equal(out.reply.GetRebirth(), quote.Preview) {
			t.Fatal("并发原提交未返回同一份报价结果", out.reply)
		}
	}
	after, err := game.Role(ctx, role.ID)
	must(err)
	var audited struct {
		Rebirth *RebirthCommitReceipt `bson:"rebirth"` // 永久审计保存原转生和精确消费。
	}
	must(audit.FindOne(ctx, key).Decode(&audited))
	if after.Level != 50 || after.RebirthCount != 1 || after.PendingExperience != 0 || after.Experience != 100 ||
		after.Wallet.BoundGold != 20 || after.EconomicSeq != role.EconomicSeq+1 || len(after.Items) != 1 ||
		after.Items[0].ID != second.ID || after.Items[0].Quantity != 2 || after.Items[0].Version != second.Version+1 ||
		!proto.Equal(audited.Rebirth.Proto(), quote.Preview) {
		t.Fatal("角色状态与凭证、跨堆扣料、金币或审计未对账", after.ProgressionSnapshot())
	}
	f.Progression.RebirthEnabled = false
	request.SceneEpoch++
	request.RebirthGate = p.CodeRebirthUnavailable
	closed, replayed, _, err := game.AssetWithReceipt(ctx, auth, request)
	must(err)
	if !reflect.DeepEqual(after, closed) || replayed.GetErrorCode() != 0 || !proto.Equal(replayed.GetRebirth(), quote.Preview) {
		t.Fatal("停开后同号重放必须保留原成长和已成交回执")
	}
}
