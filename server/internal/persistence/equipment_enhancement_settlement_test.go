// 本文件用真实 MongoDB 对拍原报价、部位档案、石堆、回执和首领判绑。
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
)

// TestEnhanceEquipmentSlotSettlementMongo 验证强化失败留级、原石守恒及事务回滚，并核对首领判绑时点。
func TestEnhanceEquipmentSlotSettlementMongo(t *testing.T) {
	t.Run("强制审计回滚及并发重放", testEquipmentEnhancementTransaction)
	t.Run("手动首领按击杀时权益判绑", testEquipmentEnhancementRewardBinding)
}

// testEquipmentEnhancementTransaction 把原成功报价置于真实事务故障和同号并行重放下。
func testEquipmentEnhancementTransaction(t *testing.T) {
	ctx, _, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_EQUIPMENT_ENHANCEMENT_TEST_FIXTURE")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fixture := &game.Cfg.Fixture
	rules := fixture.EquipmentEnhancement
	if rules == nil || role.EquipmentEnhancement != nil {
		t.Fatal("强化候选或历史空部位档案不符")
	}
	bound := domain.Item{ID: domain.ID(), ItemID: rules.StoneItemID, Quantity: 11, Version: 2, Bound: true,
		SourceKind: "personal_boss", SourceID: domain.ID()}
	free := domain.Item{ID: domain.ID(), ItemID: rules.StoneItemID, Quantity: 10, Version: 1,
		SourceKind: "personal_boss", SourceID: domain.ID()}
	_, err := game.DB.Collection("roles").UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"level": 2, "items": []domain.Item{bound, free}}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	const scene, slot = 214, 1 // 空置盔甲部位可以先成长。
	ui := domain.Bytes(domain.ID())
	request := func(item domain.Item, allowBound bool) *pb.EquipmentEnhancementQuoteRequest {
		return &pb.EquipmentEnhancementQuoteRequest{SceneEpoch: scene, Slot: slot, UiSessionId: ui, AllowBound: allowBound,
			Stones: []*pb.EquipmentEnhancementStoneRef{{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version), Quantity: 10}}}
	}
	quote, err := game.PreviewEquipmentEnhancement(ctx, auth, request(bound, true))
	must(err)
	if quote.CurrentLevel != 0 || quote.TargetLevel != 1 || quote.TargetBonusBp != 10 || quote.SuccessPpm != 1_000_000 ||
		len(quote.Stones) != 1 || !proto.Equal(quote.Stones[0].Item, bound.Proto()) || quote.Stones[0].Quantity != 10 {
		t.Fatal("空部位首次报价未复述原绑定堆和工作簿条款", quote)
	}
	commit := func(quote *pb.EquipmentEnhancementQuote) AssetRequest {
		op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
			ExpectedRevision: quote.RoleRevision, ExpectedRuleVersion: fixture.RuleVersion}
		input := &pb.EquipmentEnhancementCommitRequest{QuoteId: quote.QuoteId, UiSessionId: ui, Slot: slot, Operation: op}
		raw, err := proto.Marshal(input)
		must(err)
		return AssetRequest{Kind: AssetEquipmentEnhancement, EquipmentEnhancement: input, Context: op, SceneEpoch: scene,
			EquipmentEnhancementAcceptedAt: time.Now(), Fingerprint: Hash(raw)}
	}
	first := commit(quote)
	// 不相关的普通收益推进角色修订，仍应核对并接受同一原报价和原材料。
	role, err = game.Settle(ctx, auth, Settlement{SourceSeq: role.SourceSeq + 1, Gold: 1})
	must(err)
	key := bson.M{"_id": "asset:" + role.ID + ":" + domain.Hex(first.Context.OpId)}
	audit := game.DB.Collection("audit_outbox")
	_, err = audit.InsertOne(ctx, key)
	must(err)
	_, _, _, err = game.AssetWithReceipt(ctx, auth, first)
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatal("本次故障须来自最后一步原审计键冲突", err)
	}
	current, err := game.Role(ctx, role.ID)
	must(err)
	count, err := game.DB.Collection("operations").CountDocuments(ctx, key)
	must(err)
	if !reflect.DeepEqual(current, role) || count != 0 {
		t.Fatal("原石堆、部位档案及回执没有整体回滚", current.EquipmentEnhancement, count)
	}
	_, err = audit.DeleteOne(ctx, key)
	must(err)
	type result struct {
		role  domain.Role         // 当前已确认角色档案。
		reply *pb.OperationResult // 返回原请求的强化结果。
		err   error               // 事务或网络错误。
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			out, reply, _, err := game.AssetWithReceipt(ctx, auth, first)
			results <- result{out, reply, err}
		}()
	}
	one, duplicate := <-results, <-results
	must(one.err)
	must(duplicate.err)
	remaining := bound
	remaining.Version++
	remaining.Quantity -= 10
	success := one.reply.GetEquipmentEnhancement()
	if one.reply.ErrorCode != 0 || duplicate.reply.ErrorCode != 0 || !success.GetSuccess() || success.GetAfterLevel() != 1 ||
		!proto.Equal(success, duplicate.reply.EquipmentEnhancement) || one.role.EnhancementLevel(slot) != 1 ||
		!reflect.DeepEqual(one.role.EquipmentEnhancement, duplicate.role.EquipmentEnhancement) || one.role.Wallet != role.Wallet ||
		!reflect.DeepEqual(one.role.Items, []domain.Item{remaining, free}) || !reflect.DeepEqual(one.role.Equipped, role.Equipped) {
		t.Fatal("同号并发应对原绑定石堆扣10一次，仅升原部位", one.reply, duplicate.reply, one.role.Items)
	}
	role = one.role
	second, err := game.PreviewEquipmentEnhancement(ctx, auth, request(free, false))
	must(err)
	_, err = game.DB.Collection(enhancementQuoteCollection).UpdateOne(ctx, bson.M{"_id": role.ID, "quote_id": domain.Hex(second.QuoteId)},
		bson.M{"$set": bson.M{"success_roll": 999_999}})
	must(err)
	failed := commit(second)
	out, reply, secondOp, err := game.AssetWithReceipt(ctx, auth, failed)
	must(err)
	if second.SuccessPpm != 900_000 || reply.ErrorCode != 0 || secondOp.EquipmentEnhancement == nil ||
		reply.EquipmentEnhancement.GetSuccess() || reply.EquipmentEnhancement.GetAfterLevel() != 1 ||
		!reflect.DeepEqual(out.EquipmentEnhancement, role.EquipmentEnhancement) || !reflect.DeepEqual(out.Items, []domain.Item{remaining}) ||
		out.Wallet != role.Wallet || !reflect.DeepEqual(out.Equipped, role.Equipped) {
		t.Fatal("概率失败必须扣原非绑石但保持部位等级、钱包和装备", reply, out.EquipmentEnhancement, out.Items)
	}
	count, err = audit.CountDocuments(ctx, key)
	must(err)
	if count != 1 {
		t.Fatal("并发重放出现重复原审计", count)
	}
}

// testEquipmentEnhancementRewardBinding 用隔离卡期和真实首领胜利事务覆盖入场后生效及入场后过期。
func testEquipmentEnhancementRewardBinding(t *testing.T) {
	ctx, _, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_EQUIPMENT_ENHANCEMENT_TEST_FIXTURE")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fixture := &game.Cfg.Fixture
	template := fixture.PersonalBossByTemplate(1)
	rules := fixture.EquipmentEnhancement
	if template == nil || rules == nil || template.ItemReward == nil {
		t.Fatal("缺少个人首领强化石配置")
	}
	seed := func(changes bson.M) {
		t.Helper()
		_, err := game.DB.Collection("roles").UpdateByID(ctx, role.ID, bson.M{"$set": changes})
		must(err)
		role, err = game.Role(ctx, role.ID)
		must(err)
	}
	for _, expired := range []bool{false, true} {
		started := bossTime(time.Now().Add(-2 * time.Second))
		if expired {
			seed(bson.M{"personal_boss.cooldowns." + bossTemplateKey(template.TemplateID): started.Add(-time.Second),
				"privilege_since": started.Add(-time.Hour), "privilege_until": started.Add(time.Second)})
		}
		entry := PersonalBossBegin{ChallengeID: domain.ID(), Execution: 51, TemplateID: template.TemplateID,
			CombatSnapshot: Hash([]byte("独立强化石奖励角色快照")), ExpectedRevision: role.Revision,
			RuleVersion: fixture.RuleVersion, ConfigHash: fixture.Hash, StartedAt: started, Duration: template.Duration,
			SuccessCooldown: template.SuccessCooldown, RetryCooldown: template.RetryCooldown, BossHP: template.Monster.HP}
		active, grant, err := game.BeginPersonalBoss(ctx, auth, entry)
		must(err)
		role = active
		if !expired {
			seed(bson.M{"privilege_since": started.Add(time.Second), "privilege_until": started.Add(time.Hour)})
		}
		finish := PersonalBossFinish{ChallengeID: grant.ChallengeID, ReservationID: grant.ReservationID, Execution: grant.Execution,
			RuleVersion: grant.RuleVersion, ConfigHash: grant.ConfigHash, Outcome: PersonalBossDefeated, FinishedAt: time.Now(), Damage: grant.BossHP}
		next, receipt, err := game.FinishPersonalBoss(ctx, auth, finish)
		must(err)
		role = next
		if len(receipt.RewardItems) != 1 || receipt.RewardItems[0].ItemID != rules.StoneItemID ||
			receipt.RewardItems[0].Quantity != 10 || receipt.RewardItems[0].Bound != expired {
			t.Fatal("原强化石没有按实际击杀时的特权状态判绑", expired, receipt.RewardItems)
		}
	}
}
