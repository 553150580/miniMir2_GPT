// 本文件对拍真实 MongoDB 的洗炼费用、锁槽、覆盖、采纳与审计回滚。
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

// TestEquipmentRefineSettlementMongo 使用合法稀有原件和少量绑定钱包，验证每次锁槽的实际分账。
func TestEquipmentRefineSettlementMongo(t *testing.T) {
	ctx, _, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_EQUIPMENT_REFINE_TEST_FIXTURE")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fixture := &game.Cfg.Fixture
	rules := fixture.EquipmentRefine
	definition, found := fixture.Item(910022)
	if rules == nil || !found || !rules.Target(definition.ID, 2) {
		t.Fatal("隔离目录需含实际稀有打造原件")
	}
	value, valid := rules.ValueUnits(devconfig.RefineAttributeAttack, definition.EquipmentStage, 1)
	if !valid {
		t.Fatal("稀有原件缺少可锁的原攻击词条")
	}
	before := InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: definition.ID, Quality: 2, Quantity: 1,
		Version: 4, SourceKind: forgeSourceKind, SourceID: domain.ID(), InitialLuck: 1}, fixture)
	before.Refine.Current[0] = domain.EquipmentAffix{AttributeID: devconfig.RefineAttributeAttack, Tier: 1, ValueUnits: value}
	stones := domain.Item{ID: domain.ID(), ItemID: rules.StoneItemID, Quantity: 8, Version: 1,
		SourceKind: equipmentRecycleRefineSource, SourceID: domain.ID()}
	wallet := domain.Wallet{Gold: 12_000, BoundGold: 1, Yuanbao: 5_000, BoundYuanbao: 1}
	_, err := game.DB.Collection("roles").UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": []domain.Item{before, stones}, "wallet": wallet}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	if !CanTrade(before, definition) {
		t.Fatal("待验目标没有满足原市场资格")
	}
	const scene = 112
	ui := domain.Bytes(domain.ID())
	ref := func(item domain.Item) *pb.EquipmentRefineItemRef {
		return &pb.EquipmentRefineItemRef{InstanceId: domain.Bytes(item.ID), Version: uint64(item.Version)}
	}
	preview := func(target, stone domain.Item, allowBound bool) *pb.EquipmentRefineQuote {
		t.Helper()
		result, err := game.PreviewEquipmentRefine(ctx, auth, &pb.EquipmentRefineQuoteRequest{SceneEpoch: scene, Target: ref(target),
			LockedSlots: []uint32{0}, Stones: []*pb.EquipmentRefineStoneRef{{InstanceId: domain.Bytes(stone.ID), Version: uint64(stone.Version), Quantity: 4}},
			AllowBound: allowBound, UiSessionId: ui})
		must(err)
		return result
	}
	command := func(input *pb.EquipmentRefineActionRequest, revision uint64) AssetRequest {
		t.Helper()
		input.Operation = &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
			ExpectedRevision: revision, ExpectedRuleVersion: fixture.RuleVersion}
		raw, err := proto.Marshal(input)
		must(err)
		return AssetRequest{Kind: AssetEquipmentRefine, Context: input.Operation, EquipmentRefine: input,
			SceneEpoch: scene, Fingerprint: Hash(raw), EquipmentRefineAcceptedAt: time.Now()}
	}
	generate := func(target domain.Item, quote *pb.EquipmentRefineQuote) AssetRequest {
		return command(&pb.EquipmentRefineActionRequest{Action: refineGenerate, Target: ref(target), QuoteId: quote.QuoteId,
			UiSessionId: ui, AllowBound: quote.AllowBound}, quote.RoleRevision)
	}
	freeQuote := preview(before, stones, false)
	if freeQuote.GetOutputBound() || freeQuote.GetGold() != 3_000 || freeQuote.GetYuanbaoUnits() != 2_000 ||
		freeQuote.GetBoundGold() != 0 || freeQuote.GetBoundYuanbaoUnits() != 0 {
		t.Fatal("关闭绑定授权时不得动绑定钱包", freeQuote)
	}
	quote := preview(before, stones, true)
	if !quote.OutputBound || quote.Gold != 2_999 || quote.BoundGold != 1 || quote.YuanbaoUnits != 1_999 || quote.BoundYuanbaoUnits != 1 ||
		quote.GetTarget().GetRefinePending() != nil || !reflect.DeepEqual(quote.LockedSlots, []uint32{0}) {
		t.Fatal("稀有锁一槽的报价未公开真实分账与原锁", quote)
	}
	first := generate(before, quote)
	auditKey := bson.M{"_id": "asset:" + role.ID + ":" + domain.Hex(first.Context.OpId)}
	audit := game.DB.Collection("audit_outbox")
	_, err = audit.InsertOne(ctx, auditKey)
	must(err)
	_, _, _, err = game.AssetWithReceipt(ctx, auth, first)
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatal("隔离故障应来自最后一步原审计冲突", err)
	}
	unchanged, err := game.Role(ctx, role.ID)
	must(err)
	if !reflect.DeepEqual(unchanged, role) {
		t.Fatal("原待选、绑定和钱包没有一并回滚", unchanged.Items)
	}
	_, err = audit.DeleteOne(ctx, auditKey)
	must(err)
	generated, reply, _, err := game.AssetWithReceipt(ctx, auth, first)
	must(err)
	firstReceipt := reply.GetEquipmentRefine()
	washed := generated.Items[0]
	stillExpected := before
	stillExpected.Bound, stillExpected.Version, stillExpected.PendingResultID = true, before.Version+1, washed.PendingResultID
	stillExpected.Refine.Candidate, stillExpected.Refine.LockedMask = washed.Refine.Candidate, 1
	if reply.ErrorCode != 0 || washed != stillExpected || !lowerHex(washed.PendingResultID, 16) ||
		!proto.Equal(firstReceipt.GetTarget(), washed.Proto()) || washed.Refine.Candidate[0] != before.Refine.Current[0] ||
		washed.Refine.Candidate[1].AttributeID == before.Refine.Current[0].AttributeID ||
		generated.Wallet != (domain.Wallet{Gold: 9001, Yuanbao: 3001}) || generated.Items[1].Quantity != 4 ||
		CanTrade(washed, definition) || !equipmentOccupied(washed) {
		t.Fatal("付费生成不能改变左词条、原件身份及其他属性，并须立即绑定占用", reply, washed, generated.Wallet)
	}
	_, duplicate, _, err := game.AssetWithReceipt(ctx, auth, first)
	must(err)
	if !proto.Equal(duplicate.GetEquipmentRefine(), firstReceipt) {
		t.Fatal("原付费请求重放更换了已确认的候选", duplicate)
	}
	role = generated
	secondQuote := preview(washed, role.Items[1], false)
	continued, reply, _, err := game.AssetWithReceipt(ctx, auth, generate(washed, secondQuote))
	must(err)
	latest := continued.Items[0]
	if reply.ErrorCode != 0 || len(continued.Items) != 1 || latest.PendingResultID == washed.PendingResultID ||
		latest.Refine.Current != washed.Refine.Current || latest.Refine.Candidate[0] != latest.Refine.Current[0] ||
		continued.Wallet != (domain.Wallet{Gold: 6001, Yuanbao: 1001}) {
		t.Fatal("继续洗炼应覆盖右侧并重新支付锁槽费", reply, continued.Items, continued.Wallet)
	}
	role = continued
	decide := func(id string) AssetRequest {
		return command(&pb.EquipmentRefineActionRequest{Action: refineAdopt, Target: ref(latest),
			PendingId: domain.Bytes(id), UiSessionId: ui}, uint64(role.Revision))
	}
	refused, result, _, err := game.AssetWithReceipt(ctx, auth, decide(washed.PendingResultID))
	must(err)
	if result.ErrorCode != p.CodeEquipmentRefineChanged || refused.Items[0] != latest || refused.Wallet != role.Wallet {
		t.Fatal("历史候选错误覆盖了同原件最新候选", result)
	}
	role = refused
	decision := decide(latest.PendingResultID)
	adopted, result, _, err := game.AssetWithReceipt(ctx, auth, decision)
	must(err)
	expected := latest
	expected.Version, expected.PendingResultID = latest.Version+1, ""
	expected.Refine = domain.EquipmentRefineState{Current: latest.Refine.Candidate}
	if result.ErrorCode != 0 || adopted.Items[0] != expected || adopted.Wallet != role.Wallet ||
		result.EquipmentRefine.GetTarget().GetRefinePending() != nil || len(result.EquipmentRefine.GetStones()) != 0 {
		t.Fatal("采纳只应改变原已采纳词条与去掉待选占用", result, adopted.Items[0])
	}
	_, duplicate, _, err = game.AssetWithReceipt(ctx, auth, decision)
	must(err)
	if !proto.Equal(duplicate.GetEquipmentRefine(), result.GetEquipmentRefine()) {
		t.Fatal("采纳重试没有重放原结果", duplicate)
	}
}
