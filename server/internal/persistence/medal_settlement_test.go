// 本文件用真实副本集验证勋章、原碎片、最新报价及原经济回执同一事务提交。
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

// TestMedalSettlementMongo 覆盖最终审计失败回滚、同号并发重放和关闭配置后的持久档案。
func TestMedalSettlementMongo(t *testing.T) {
	ctx, _, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_MEDAL_TEST_FIXTURE")
	fixture := &game.Cfg.Fixture
	if fixture.Medal == nil || role.Medal != nil {
		t.Fatal("勋章实测必须装配隔离候选，并从未激活角色开始")
	}
	t.Logf("勋章持久化实际加载候选 SHA-256=%s", fixture.Hash)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	bound, free := medalTestStacks(fixture.Medal.FragmentItemID)
	_, err := game.DB.Collection("roles").UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{
		"level": 2, "items": []domain.Item{bound, free},
	}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	const scene = 418
	ui := domain.Bytes(domain.ID())
	quote, err := game.PreviewMedal(ctx, auth, &pb.MedalQuoteRequest{SceneEpoch: scene, UiSessionId: ui, AllowBound: true,
		Fragments: []*pb.MedalFragmentRef{medalTestRef(free, 1), medalTestRef(bound, 1)}})
	must(err)
	if quote.Medal != nil || quote.TargetLevel != 1 || quote.TargetMainAttack != 1 || quote.TargetDefense != 1 ||
		len(quote.Fragments) != 2 || quote.Fragments[0].Bound || !quote.Fragments[1].Bound {
		t.Fatal("首次原报价未显示真实绑和非绑碎片及确定目标", quote)
	}
	op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
		ExpectedRevision: quote.RoleRevision, ExpectedRuleVersion: fixture.RuleVersion}
	confirmation := &pb.MedalCommitRequest{QuoteId: quote.QuoteId, UiSessionId: ui, Operation: op}
	payload, err := proto.Marshal(confirmation)
	must(err)
	request := AssetRequest{Kind: AssetMedalCommit, Context: op, MedalCommit: confirmation, SceneEpoch: scene,
		MedalAcceptedAt: time.Now(), Fingerprint: Hash(payload)}
	// 不改变原堆或勋章的收益可以先推进原修订，随后仍按原报价确认。
	role, err = game.Settle(ctx, auth, Settlement{SourceSeq: role.SourceSeq + 1, Gold: 1})
	must(err)
	key := bson.M{"_id": "asset:" + role.ID + ":" + domain.Hex(op.OpId)}
	audit := game.DB.Collection("audit_outbox")
	_, err = audit.InsertOne(ctx, key)
	must(err)
	_, _, _, err = game.AssetWithReceipt(ctx, auth, request)
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatal("应由事务最后写入原审计失败触发整笔回滚", err)
	}
	rolledBack, err := game.Role(ctx, role.ID)
	must(err)
	quoteCount, err := game.DB.Collection(medalQuoteCollection).CountDocuments(ctx, bson.M{"_id": role.ID, "quote_id": domain.Hex(quote.QuoteId)})
	must(err)
	operationCount, err := game.DB.Collection("operations").CountDocuments(ctx, key)
	must(err)
	if !reflect.DeepEqual(rolledBack, role) || rolledBack.Medal != nil || quoteCount != 1 || operationCount != 0 {
		t.Fatal("审计失败未整笔回滚原碎片、勋章、报价和回执", quoteCount, operationCount)
	}
	_, err = audit.DeleteOne(ctx, key)
	must(err)
	type outcome struct {
		reply *pb.OperationResult // 原提交返回的唯一资产结果。
		err   error               // 对应的事务或传输错误。
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			_, reply, _, err := game.AssetWithReceipt(ctx, auth, request)
			results <- outcome{reply, err}
		}()
	}
	first, second := <-results, <-results
	must(first.err)
	must(second.err)
	receipt := first.reply.GetMedal()
	if first.reply.ErrorCode != 0 || second.reply.ErrorCode != 0 || !proto.Equal(receipt, second.reply.GetMedal()) ||
		receipt.GetBeforeLevel() != 0 || receipt.GetMedal().GetLevel() != 1 || receipt.GetMedal().GetVersion() != 1 ||
		len(receipt.GetFragments()) != 2 || !proto.Equal(receipt.Fragments[0], quote.Fragments[0]) ||
		!proto.Equal(receipt.Fragments[1], quote.Fragments[1]) {
		t.Fatal("同号并发请求未取得相同原勋章回执", first.reply, second.reply)
	}
	confirmed, err := game.Role(ctx, role.ID)
	must(err)
	var operation Operation
	must(game.DB.Collection("operations").FindOne(ctx, key).Decode(&operation))
	var audited struct {
		Medal *MedalCommitReceipt `bson:"medal"` // 审计保存与操作完全相同的原档案和材料。
	}
	must(audit.FindOne(ctx, key).Decode(&audited))
	docs, err := game.DB.Collection(roleMedalCollection).CountDocuments(ctx, bson.M{"_id": role.ID})
	must(err)
	consumed, err := game.DB.Collection(medalQuoteCollection).CountDocuments(ctx, bson.M{"_id": role.ID})
	must(err)
	if docs != 1 || consumed != 0 || confirmed.Medal == nil || confirmed.Medal.OwnerRoleID != role.ID || confirmed.Medal.Level != 1 ||
		confirmed.EconomicSeq != role.EconomicSeq+1 || len(confirmed.Items) != 2 || confirmed.Items[0].Quantity != 3 ||
		confirmed.Items[1].Quantity != 4 || !reflect.DeepEqual(operation.Medal, audited.Medal) ||
		!proto.Equal(receipt, operation.Medal.Proto()) {
		t.Fatal("一次扣包、独立角色档案、原报价消耗和审计未能对账", docs, consumed)
	}
	// 关闭候选后保留独立档案，原成功回执优先于当前场景门禁和原报价有效期。
	fixture.Medal = nil
	request.SceneEpoch++
	request.MedalGate = p.CodeMedalUnavailable
	closed, replayed, _, err := game.AssetWithReceipt(ctx, auth, request)
	must(err)
	if closed.Medal != nil || replayed.ErrorCode != 0 || !proto.Equal(replayed.GetMedal(), receipt) {
		t.Fatal("关闭后不可展示勋章，但原确认仍应可重放且不扣料", replayed)
	}
	var stored domain.MedalState
	must(game.DB.Collection(roleMedalCollection).FindOne(ctx, bson.M{"_id": role.ID}).Decode(&stored))
	if !reflect.DeepEqual(stored, *confirmed.Medal) || !reflect.DeepEqual(closed.Items, confirmed.Items) {
		t.Fatal("关闭候选后的原档案或原库存发生变化")
	}
}
