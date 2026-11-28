// 本文件用真实 MongoDB 验证原概率样本、失败扣费、原回执重放和审计回滚。
package persistence

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestBodyTrainingSettlementMongo 在原事务内验证失败扣钱及重复确认。
func TestBodyTrainingSettlementMongo(t *testing.T) {
	ctx, _, store, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_BODY_TRAINING_TEST_FIXTURE")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := store.DB.Collection("roles").UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{
		"level": 20, "wallet": domain.Wallet{Gold: 5000, BoundGold: 1000},
		"body_training": domain.BodyTrainingState{Health: 1},
	}})
	must(err)
	role, err = store.Role(ctx, role.ID)
	must(err)
	f := &store.Cfg.Fixture
	stageRole := role
	stageRole.BodyTraining = &domain.BodyTrainingState{Health: 9, Mana: 10}
	atNine, code := buildBodyTrainingPlan(stageRole, f, devconfig.BodyHealth)
	if code != 0 || atNine.Chance != 100_000 || atNine.Spend.Gold+atNine.Spend.BoundGold != 1500 {
		t.Fatal("十级阶段尾档应为10%和一倍费用", atNine, code)
	}
	atTen, code := buildBodyTrainingPlan(stageRole, f, devconfig.BodyMana)
	if code != 0 || atTen.Chance != 1_000_000 || atTen.Spend.Gold+atTen.Spend.BoundGold != 3000 {
		t.Fatal("下一阶段应重回100%并使用两倍费用", atTen, code)
	}
	stageRole.Level = 9
	if _, code := buildBodyTrainingPlan(stageRole, f, devconfig.BodyHealth); code != p.CodeBodyTrainingInputs {
		t.Fatal("匹配角色等级上限仍给出收费报价", code)
	}
	stageRole.Level = 50
	stageRole.BodyTraining = &domain.BodyTrainingState{Health: 12}
	if _, code := buildBodyTrainingPlan(stageRole, f, devconfig.BodyHealth); code != p.CodeBodyTrainingInputs {
		t.Fatal("配置到顶仍给出收费报价", code)
	}
	scene := uint64(127)
	ui := domain.Bytes(domain.ID())
	quote, err := store.PreviewBodyTraining(ctx, auth, &pb.BodyTrainingQuoteRequest{SceneEpoch: scene, UiSessionId: ui, Track: devconfig.BodyHealth})
	must(err)
	if quote.Terms.SuccessPpm != 900_000 || quote.Terms.BeforeLevel != 1 || quote.Terms.TargetLevel != 2 {
		t.Fatal("生命二级首次尝试应为90%", quote.Terms)
	}
	// 只在隔离测试数据库固定90%的排他边界，稳定验收实际失败路径。
	quoteCollection := store.DB.Collection(bodyTrainingQuotes)
	filter := bson.M{"_id": role.ID, "quote_id": domain.Hex(quote.QuoteId)}
	_, err = quoteCollection.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"success_roll": 900_000}})
	must(err)
	op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
		ExpectedRevision: quote.RoleRevision, ExpectedRuleVersion: f.RuleVersion}
	request := &pb.BodyTrainingCommitRequest{QuoteId: quote.QuoteId, UiSessionId: ui, Track: devconfig.BodyHealth, Operation: op}
	payload, err := proto.Marshal(request)
	must(err)
	asset := AssetRequest{Kind: AssetBodyTrainingCommit, Context: op, BodyTrainingCommit: request,
		SceneEpoch: scene, BodyTrainingAcceptedAt: time.Now(), Fingerprint: Hash(payload)}
	key := bson.M{"_id": "asset:" + role.ID + ":" + domain.Hex(op.OpId)}
	audit := store.DB.Collection("audit_outbox")
	_, err = audit.InsertOne(ctx, key)
	must(err)
	_, _, _, err = store.AssetWithReceipt(ctx, auth, asset)
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatal("原审计冲突应使整笔回滚", err)
	}
	afterAbort, err := store.Role(ctx, role.ID)
	must(err)
	var original bodyTrainingStoredQuote
	must(quoteCollection.FindOne(ctx, filter).Decode(&original))
	if !reflect.DeepEqual(afterAbort, role) || original.Roll != 900_000 {
		t.Fatal("交易失败后原钱包、等级、报价或抽样变化")
	}
	_, err = audit.DeleteOne(ctx, key)
	must(err)
	type outcome struct {
		value *pb.OperationResult // 相同原操作对应的网络结果。
		err   error               // 事务错误。
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			_, value, _, err := store.AssetWithReceipt(ctx, auth, asset)
			results <- outcome{value, err}
		}()
	}
	for range 2 {
		result := <-results
		must(result.err)
		receipt := result.value.BodyTraining
		if result.value.ErrorCode != 0 || receipt == nil || receipt.Success || receipt.AfterLevel != 1 ||
			!proto.Equal(receipt.Terms, quote.Terms) {
			t.Fatal("失败或并发重放未保留原报价与等级", result.value)
		}
	}
	confirmed, err := store.Role(ctx, role.ID)
	must(err)
	var ledger struct {
		BodyTraining *BodyTrainingReceipt `bson:"body_training"` // 已付的原尝试判定及样本。
	}
	must(audit.FindOne(ctx, key).Decode(&ledger))
	n, err := quoteCollection.CountDocuments(ctx, filter)
	must(err)
	if confirmed.BodyTrainingLevels() != role.BodyTrainingLevels() || confirmed.Wallet.Gold != 4500 ||
		confirmed.Wallet.BoundGold != 0 || confirmed.EconomicSeq != role.EconomicSeq+1 || n != 0 ||
		ledger.BodyTraining == nil || ledger.BodyTraining.Roll != 900_000 || ledger.BodyTraining.Success {
		t.Fatal("概率失败必须扣一次费用、保留原级并记录原样本")
	}
}
