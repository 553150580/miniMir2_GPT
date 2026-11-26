// 本文件在独立 MongoDB 环境核对打造报价、真失败、真成功、原回执与审计。
package persistence

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestForgeSettlementMongo 只控制当前隔离报价的随机样本，以确定性实测两个结算分支和重放。
func TestForgeSettlementMongo(t *testing.T) {
	ctx, account, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_FORGE_TEST_FIXTURE")
	fixture := &game.Cfg.Fixture
	if fixture.Forge == nil || len(fixture.Forge.Recipes) != 1 {
		t.Fatal("本轮独立打造配置必须装配一张候选图纸")
	}
	recipe := fixture.Forge.Recipes[0]
	if recipe.Quality != 2 || recipe.Gold != 30000 || recipe.OreCount != 4 || fixture.Forge.GuaranteedPurity != 95 {
		t.Fatal("独立打造样本不匹配工作簿投影和装备身份", recipe)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	blueprint := domain.Item{ID: domain.ID(), ItemID: recipe.BlueprintItemID, Quantity: 1, Version: 1, Quality: recipe.Quality,
		SourceKind: "personal_boss", SourceID: domain.ID()}
	items := []domain.Item{blueprint}
	for index, purity := range []uint32{10, 15, 17, 18, 95, 97, 98, 100} {
		items = append(items, domain.Item{ID: domain.ID(), ItemID: fixture.Forge.OreItemID, Quantity: 1, Version: 1,
			SourceKind: "mining", SourceID: domain.ID(), OrePurity: purity, Bound: index == 4})
	}
	wallet := domain.Wallet{Gold: 60000, BoundGold: 1234, Yuanbao: 4, BoundYuanbao: 7}
	_, err := account.DB.Collection("roles").UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": items, "wallet": wallet, "capacity": len(items)}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	const scene = uint64(99999)
	preview := func(ores []domain.Item, allowBound bool) *pb.ForgeQuote {
		t.Helper()
		reply, err := game.PreviewForge(ctx, auth, forgeQuoteRefs(scene, blueprint, ores, allowBound))
		must(err)
		if len(reply.QuoteId) != 16 || reply.RoleRevision == 0 || reply.SceneEpoch != scene {
			t.Fatal("报价未固定原场景或角色版本", reply)
		}
		return reply
	}
	commit := func(cur domain.Role, quote *pb.ForgeQuote, acceptedAt time.Time) AssetRequest {
		t.Helper()
		command := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(cur.EconomicSeq + 1),
			ExpectedRevision: quote.RoleRevision, ExpectedRuleVersion: fixture.RuleVersion}
		wire := &pb.ForgeCommitRequest{QuoteId: quote.QuoteId, Operation: command}
		payload, err := proto.Marshal(wire)
		must(err)
		return AssetRequest{Kind: AssetForgeCommit, Fingerprint: Hash(payload), Context: command, ForgeCommit: wire,
			SceneEpoch: scene, ForgeAcceptedAt: acceptedAt}
	}
	expectAssets := func(actual domain.Role, wantItems []domain.Item, wantWallet domain.Wallet, wantSeq int64) {
		t.Helper()
		if !reflect.DeepEqual(actual.Items, wantItems) || actual.Wallet != wantWallet || actual.EconomicSeq != wantSeq {
			t.Fatal("持久资产或主动序号与原账本不一致", actual.Items, actual.Wallet, actual.EconomicSeq)
		}
	}
	findQuote := func(reply *pb.ForgeQuote) bson.M {
		return bson.M{"_id": role.ID, "quote_id": domain.Hex(reply.QuoteId)}
	}
	low, high := items[1:5], items[5:]
	// 过期报价在边界毫秒不能成交，角色只记录拒绝回执，不扣原材料。
	expired := preview(low, false)
	expiredAt := time.UnixMilli(int64(expired.ExpiresAtUnixMs))
	rejected, rejectedResult, rejectReceipt, err := game.AssetWithReceipt(ctx, auth, commit(role, expired, expiredAt))
	must(err)
	if rejectedResult.ErrorCode != p.CodeForgeQuoteChanged || rejectedResult.Forge != nil || rejectReceipt.Forge != nil {
		t.Fatal("过期报价未被拒绝或冒充成已打造", rejectedResult, rejectReceipt)
	}
	expectAssets(rejected, items, wallet, role.EconomicSeq+1)

	failureQuote := preview(low, false)
	if failureQuote.SuccessPpm != 150000 || failureQuote.Gold != 30000 || failureQuote.BoundGold != 0 || failureQuote.OutputBound {
		t.Fatal("低纯度报价、关闭绑金或原产物绑态不符", failureQuote)
	}
	// 只在本隔离库覆盖私有成功随机值，让实际数据库事务走失败分支。
	changed, err := game.DB.Collection(forgeQuoteCollection).UpdateOne(ctx, findQuote(failureQuote), bson.M{"$set": bson.M{"success_roll": 999999}})
	must(err)
	if changed.MatchedCount != 1 {
		t.Fatal("找不到测试专属的原失败报价")
	}
	failCommand := commit(rejected, failureQuote, time.Now())
	failed, failResult, originalFailure, err := game.AssetWithReceipt(ctx, auth, failCommand)
	must(err)
	if failResult.ErrorCode != 0 || failResult.Forge == nil || failResult.Forge.Success || failResult.Forge.Output != nil ||
		originalFailure.Forge == nil || originalFailure.Forge.Output != nil || !reflect.DeepEqual(originalFailure.Forge.Plan.Ores, low) ||
		originalFailure.Forge.Plan.Blueprint != blueprint {
		t.Fatal("失败没有封存原图纸、耗材和扣款", failResult, originalFailure.Forge)
	}
	wallet.Gold -= recipe.Gold
	remaining := append([]domain.Item{blueprint}, high...)
	expectAssets(failed, remaining, wallet, rejected.EconomicSeq+1)

	successQuote := preview(high, true)
	if successQuote.SuccessPpm != forgeMillion || !successQuote.OutputBound || successQuote.Gold != 28766 || successQuote.BoundGold != 1234 {
		t.Fatal("高纯度未显示必成，或未按实际绑金优先扣费", successQuote)
	}
	// 只固定隔离的幸运随机值，必成仍来自服务端按原平均纯度计算。
	changed, err = game.DB.Collection(forgeQuoteCollection).UpdateOne(ctx, findQuote(successQuote), bson.M{"$set": bson.M{
		"luck_roll": 0, "luck_weight_roll": fixture.Forge.LuckWeights[0],
	}})
	must(err)
	if changed.MatchedCount != 1 {
		t.Fatal("找不到测试专属的原成功报价")
	}
	successCommand := commit(failed, successQuote, time.Now())
	finished, successResult, originalSuccess, err := game.AssetWithReceipt(ctx, auth, successCommand)
	must(err)
	if successResult.ErrorCode != 0 || successResult.Forge == nil || !successResult.Forge.Success || successResult.Forge.Output.GetInitialLuck() != 2 ||
		originalSuccess.Forge == nil || originalSuccess.Forge.Output == nil || !originalSuccess.Forge.Output.Bound ||
		originalSuccess.Forge.Output.ItemID != recipe.OutputItemID || originalSuccess.Forge.Output.SourceKind != forgeSourceKind ||
		!reflect.DeepEqual(originalSuccess.Forge.Plan.Ores, high) {
		t.Fatal("成功没有保存原成品及其初始幸运、纯度来源", successResult, originalSuccess.Forge)
	}
	wallet.Gold -= 28766
	wallet.BoundGold -= 1234
	output := *originalSuccess.Forge.Output
	expectAssets(finished, []domain.Item{output}, wallet, failed.EconomicSeq+1)
	quotes, err := game.DB.Collection(forgeQuoteCollection).CountDocuments(ctx, bson.M{"_id": role.ID})
	must(err)
	if quotes != 0 {
		t.Fatal("已确认报价还可被新命令重新消费")
	}

	for _, original := range []struct {
		request AssetRequest
		result  *pb.ForgeReceipt
		op      Operation
	}{
		{failCommand, failResult.Forge, originalFailure},
		{successCommand, successResult.Forge, originalSuccess},
	} {
		// 重放时旧场景已退役，并已超出原报价时限。查询原回执仍返回相同结果。
		replay := original.request
		replay.SceneEpoch, replay.ForgeGate, replay.ForgeAcceptedAt = scene+1, p.CodeForgeUnavailable, time.Now().Add(time.Hour)
		current, replayResult, op, err := game.AssetWithReceipt(ctx, auth, replay)
		must(err)
		if replayResult.ErrorCode != 0 || !proto.Equal(replayResult.Forge, original.result) || !reflect.DeepEqual(op.Forge, original.op.Forge) ||
			op.CommandSeq != original.op.CommandSeq {
			t.Fatal("已提交原报价没有按操作身份重放", replayResult, op.Forge)
		}
		expectAssets(current, []domain.Item{output}, wallet, finished.EconomicSeq)
		var audit struct {
			ResultCode uint32              `bson:"result_code"`
			Forge      *ForgeResultReceipt `bson:"forge"`
		}
		must(game.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": op.ID, "kind": AssetForgeCommit}).Decode(&audit))
		if audit.ResultCode != 0 || !reflect.DeepEqual(original.op.Forge, audit.Forge) {
			t.Fatal("成交和审计的原材料、钱包或产物不同", audit)
		}
	}
	persisted, err := game.Role(ctx, role.ID)
	must(err)
	if slices.Contains(persisted.Items, blueprint) {
		t.Fatal("成功后原图纸仍留在背包")
	}
	expectAssets(persisted, []domain.Item{output}, wallet, finished.EconomicSeq)
}
