// 本文件用隔离 MongoDB 复核翅膀独立实例、原羽毛事务、审计和新附着回放。
package persistence

import (
	"bytes"
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

// TestWingSettlementMongo 以真实角色会话、永久资产和独立数据库验证翅膀操作。
func TestWingSettlementMongo(t *testing.T) {
	ctx, account, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_WING_TEST_FIXTURE")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fixture := &game.Cfg.Fixture
	rules := fixture.Wings
	if rules == nil || rules.MaxRank != 10 || len(rules.Ranks) < 2 || !reflect.DeepEqual(rules.AppearanceTiers, []uint32{1, 3, 5, 7, 10}) {
		t.Fatal("隔离候选需有真实第一、二阶及原外观开放次序", rules)
	}
	firstCost, secondCost := rules.Ranks[0].Feathers, rules.Ranks[1].Feathers
	definition, found := fixture.Item(rules.FeatherItemID)
	if !found || !definition.Stackable || firstCost <= 0 || secondCost <= 0 || firstCost >= definition.MaxStack || secondCost > definition.MaxStack {
		t.Fatal("隔离候选缺少可核对余量和完整消耗的羽毛堆", definition, rules)
	}
	if role.Wing != nil || role.Inventory().Wing != nil {
		t.Fatal("历史角色应当先读到未激活状态", role.Wing)
	}
	bound := domain.Item{ID: domain.ID(), ItemID: definition.ID, Quantity: firstCost + 1, Version: 2, Bound: true,
		SourceKind: "world_boss", SourceID: domain.ID()}
	free := domain.Item{ID: domain.ID(), ItemID: definition.ID, Quantity: secondCost, Version: 1,
		SourceKind: "world_boss", SourceID: domain.ID()}
	roles := game.DB.Collection("roles")
	_, err := roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": []domain.Item{bound, free}}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	const scene = 23
	ui := domain.Bytes(domain.ID())
	previewRequest := func(wing *domain.WingState, item domain.Item, amount int64, allowBound bool) *pb.WingQuoteRequest {
		q := &pb.WingQuoteRequest{SceneEpoch: scene, UiSessionId: ui, Feathers: []*pb.WingFeatherRef{wingTestFeatherRef(item, amount)}, AllowBound: allowBound}
		if wing != nil {
			q.WingId, q.WingVersion = domain.Bytes(wing.ID), uint64(wing.Version)
		}
		return q
	}
	preview := func(q *pb.WingQuoteRequest) *pb.WingQuote {
		t.Helper()
		quote, err := game.PreviewWing(ctx, auth, q)
		must(err)
		if len(quote.QuoteId) != 16 || quote.RoleRevision != uint64(role.Revision) || quote.SceneEpoch != scene ||
			!bytes.Equal(quote.UiSessionId, ui) || quote.AllowBound != q.AllowBound {
			t.Fatal("报价未冻结原角色、场景及界面会话", quote)
		}
		return quote
	}
	operation := func(revision uint64) *pb.OperationContext {
		return &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
			ExpectedRevision: revision, ExpectedRuleVersion: fixture.RuleVersion}
	}
	growth := func(quote *pb.WingQuote) AssetRequest {
		req := &pb.WingActionRequest{Action: wingActionGrow, Operation: operation(quote.RoleRevision), QuoteId: quote.QuoteId,
			UiSessionId: quote.UiSessionId, AllowBound: quote.AllowBound}
		if quote.Wing != nil {
			req.WingId, req.WingVersion = quote.Wing.WingId, quote.Wing.Version
		}
		return AssetRequest{Kind: AssetWingAction, Context: req.Operation, SceneEpoch: scene, WingAction: req, WingAcceptedAt: time.Now()}
	}
	appearance := func(tier uint32, hidden bool) AssetRequest {
		req := &pb.WingActionRequest{Action: wingActionAppearance, Operation: operation(uint64(role.Revision)), UiSessionId: ui,
			WingId: domain.Bytes(role.Wing.ID), WingVersion: uint64(role.Wing.Version), AppearanceTier: tier, Hidden: hidden}
		return AssetRequest{Kind: AssetWingAction, Context: req.Operation, SceneEpoch: scene, WingAction: req, WingAcceptedAt: time.Now()}
	}
	fingerprint := func(q AssetRequest) AssetRequest {
		raw, err := proto.Marshal(q.WingAction)
		must(err)
		q.Fingerprint = Hash(raw)
		return q
	}
	deny := func(q AssetRequest, expected uint32) {
		t.Helper()
		out, response, op, err := game.AssetWithReceipt(ctx, auth, fingerprint(q))
		must(err)
		if response.ErrorCode != expected || response.Wing != nil || op.Wing != nil || out.Wallet != role.Wallet ||
			!reflect.DeepEqual(out.Items, role.Items) || !reflect.DeepEqual(out.Wing, role.Wing) {
			t.Fatal("拒绝的翅膀操作不得改变背包或翅膀", response, op, out.Items, out.Wing)
		}
		role = out
	}
	_, err = game.PreviewWing(ctx, auth, previewRequest(nil, bound, firstCost, false))
	if Code(err) != p.CodeWingInputs {
		t.Fatal("不授权时无法将原绑定羽毛放进报价", err)
	}
	quote := preview(previewRequest(nil, bound, firstCost, true))
	if quote.Wing != nil || quote.TargetRank != 1 || quote.TargetAppearanceTier != 1 || len(quote.Feathers) != 1 ||
		!proto.Equal(quote.Feathers[0].Item, bound.Proto()) || quote.Feathers[0].Quantity != firstCost {
		t.Fatal("首次激活报价没有列出真实原绑定堆与扣量", quote)
	}
	q := growth(quote)
	q.WingAcceptedAt = time.UnixMilli(int64(quote.ExpiresAtUnixMs))
	deny(q, p.CodeWingQuoteChanged)
	q = growth(quote)
	q.WingAction.UiSessionId = domain.Bytes(domain.ID())
	deny(q, p.CodeWingQuoteChanged)
	q = growth(quote)
	q.WingAction.AllowBound = false
	deny(q, p.CodeWingQuoteChanged)
	q = growth(quote)
	q.SceneEpoch++
	deny(q, p.CodeWingQuoteChanged)
	q = growth(quote)
	q.WingGate = p.CodeWingUnavailable
	deny(q, p.CodeWingUnavailable)
	activation := fingerprint(growth(quote))
	// 角色资产全局修订可因正常收益前进，原羽毛和从未激活的事实仍须完全相同。
	role, err = game.Settle(ctx, auth, Settlement{SourceSeq: role.SourceSeq + 1, Gold: 1})
	must(err)
	// 审计最后写入。只在隔离库预置相同审计键，确证失败时扣料、建翼、耗报价及写回执一并回滚。
	auditKey := bson.M{"_id": "asset:" + role.ID + ":" + domain.Hex(activation.Context.OpId)}
	auditOutbox := game.DB.Collection("audit_outbox")
	_, err = auditOutbox.InsertOne(ctx, auditKey)
	must(err)
	_, _, _, err = game.AssetWithReceipt(ctx, auth, activation)
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatal("隔离故障应来自最终审计的原键冲突", err)
	}
	rolledBack, err := game.Role(ctx, role.ID)
	must(err)
	notWritten, err := game.DB.Collection("operations").CountDocuments(ctx, auditKey)
	must(err)
	if !reflect.DeepEqual(rolledBack, role) || notWritten != 0 {
		t.Fatal("审计失败后角色、翅膀和永久回执必须维持原状态", rolledBack.Wing, rolledBack.Items, notWritten)
	}
	_, err = auditOutbox.DeleteOne(ctx, auditKey)
	must(err)
	type attempt struct {
		role  domain.Role         // 实时角色资产与翅膀状态。
		reply *pb.OperationResult // 原操作确认结果。
		op    Operation           // 存库时冻结的同一事务回执。
		err   error               // 数据库或命令外围错误。
	}
	attempts := make(chan attempt, 2)
	for range 2 {
		go func() {
			out, response, op, err := game.AssetWithReceipt(ctx, auth, activation)
			attempts <- attempt{out, response, op, err}
		}()
	}
	first, duplicate := <-attempts, <-attempts
	must(first.err)
	must(duplicate.err)
	remaining := bound
	remaining.Quantity, remaining.Version = 1, bound.Version+1
	if first.reply.ErrorCode != 0 || duplicate.reply.ErrorCode != 0 || first.op.Wing == nil || first.op.Wing.Before != nil ||
		first.op.ID != duplicate.op.ID || !proto.Equal(first.reply.Wing, duplicate.reply.Wing) ||
		first.role.Wing == nil || first.role.Wing.OwnerRoleID != role.ID || first.role.Wing.Rank != 1 ||
		first.role.Wing.Version != 1 || first.role.Wing.Luck != 0 || first.role.Wing.OilToSix != 0 || first.role.Wing.OilToNine != 0 ||
		!reflect.DeepEqual(first.role.Items, []domain.Item{remaining, free}) || first.role.EconomicSeq != role.EconomicSeq+1 || first.role.Wallet != role.Wallet {
		t.Fatal("相同操作并发必须只创建一件翅膀和一笔羽毛扣除", first, duplicate)
	}
	role = first.role
	firstWingID := role.Wing.ID
	deny(growth(quote), p.CodeWingQuoteChanged)
	// 只在隔离库准备已有用油历史，校验升阶与外观不会改写幸运和两个保底累计。
	wings := game.DB.Collection(roleWingCollection)
	_, err = wings.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"luck": 7, "oil_to_six": 102, "oil_to_nine": 27}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	previous := *role.Wing
	quote = preview(previewRequest(&previous, free, secondCost, false))
	newTicket, err := account.IssueTicket(ctx, auth, role.Summary().Key)
	must(err)
	oldSession := auth.SessionEpoch
	role, auth, err = game.Enter(ctx, newTicket.Ticket)
	must(err)
	if oldSession == auth.SessionEpoch || !proto.Equal(role.Inventory().Wing, previous.Proto()) {
		t.Fatal("重新附着必须从独立集合恢复正在使用的翅膀", role.Wing, auth.SessionEpoch)
	}
	deny(growth(quote), p.CodeWingQuoteChanged)
	quote = preview(previewRequest(role.Wing, free, secondCost, false))
	changed := free
	changed.Version++
	_, err = roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": []domain.Item{remaining, changed}}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	deny(growth(quote), p.CodeWingQuoteChanged)
	quote = preview(previewRequest(role.Wing, changed, secondCost, false))
	role, reply, upgraded, err := game.AssetWithReceipt(ctx, auth, fingerprint(growth(quote)))
	must(err)
	want := previous
	want.Rank, want.Version = 2, previous.Version+1
	if reply.ErrorCode != 0 || upgraded.Wing == nil || !reflect.DeepEqual(upgraded.Wing.Before, &previous) ||
		upgraded.Wing.Wing != want || !proto.Equal(reply.Wing.GetWing(), want.Proto()) ||
		!reflect.DeepEqual(role.Items, []domain.Item{remaining}) || *role.Wing != want {
		t.Fatal("第二阶必须复用同一实例，完整扣原非绑定堆且保留幸运历史", reply, upgraded.Wing, role.Items, role.Wing)
	}
	deny(appearance(3, true), p.CodeWingInputs)
	role, reply, hidden, err := game.AssetWithReceipt(ctx, auth, fingerprint(appearance(1, true)))
	must(err)
	want.Hidden, want.Version = true, want.Version+1
	if reply.ErrorCode != 0 || hidden.Wing == nil || len(hidden.Wing.Feathers) != 0 || hidden.Wing.QuoteID != "" || *role.Wing != want {
		t.Fatal("隐藏外观只更新原实例外观状态", reply, role.Wing)
	}
	activation.SceneEpoch = 0
	activation.WingGate = p.CodeWingUnavailable
	activation.WingAcceptedAt = time.Now().Add(time.Hour)
	current, replay, original, err := game.AssetWithReceipt(ctx, auth, activation)
	must(err)
	if replay.ErrorCode != 0 || !proto.Equal(replay.Wing, first.reply.Wing) || !reflect.DeepEqual(original.Wing, first.op.Wing) ||
		!reflect.DeepEqual(current, role) || current.Wing.ID != firstWingID || current.Wing.Rank != 2 {
		t.Fatal("远离首次场景后的重放应返回原确认回执及当前翅膀", replay, current.Wing)
	}
	for _, op := range []Operation{first.op, upgraded, hidden} {
		var audit struct {
			Wing   *WingActionReceipt `bson:"wing"`
			Rule   uint64             `bson:"rule_version"`
			Config string             `bson:"config_hash"`
		}
		must(game.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": op.ID, "kind": AssetWingAction, "result_code": 0}).Decode(&audit))
		if !reflect.DeepEqual(audit.Wing, op.Wing) || audit.Rule != fixture.RuleVersion || audit.Config != fixture.Hash {
			t.Fatal("原翅膀与原羽毛变更必须与同事务正式审计逐项一致", audit)
		}
	}
	// 模拟旧规则的整档角色写入。回滚后恢复新规则时仍然能读到原实例与幸运。
	old := &Store{DB: game.DB, Cfg: game.Cfg}
	old.Cfg.Fixture.Wings = nil
	base, err := old.Role(ctx, role.ID)
	must(err)
	if base.Wing != nil || base.Inventory().Wing != nil {
		t.Fatal("未启用的旧规则不应装配翅膀", base.Wing)
	}
	_, err = roles.ReplaceOne(ctx, bson.M{"_id": role.ID}, base)
	must(err)
	baseRecord, err := roles.FindOne(ctx, bson.M{"_id": role.ID}).Raw()
	must(err)
	if _, err := baseRecord.LookupErr("wing"); err == nil {
		t.Fatal("完整角色档案不得存入会被旧版本覆盖的翅膀字段")
	}
	current, err = game.Role(ctx, role.ID)
	must(err)
	if !reflect.DeepEqual(current.Wing, &want) {
		t.Fatal("旧规则写回角色后应仍能恢复原翅膀", current.Wing)
	}
	duplicateWing := want
	duplicateWing.OwnerRoleID = domain.ID()
	_, err = wings.InsertOne(ctx, duplicateWing)
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatal("翅膀实例全库唯一约束未生效", err)
	}
	t.Log("翅膀原账本核对通过：原绑定与非绑定羽毛分别扣原堆，激活并发只产生一件，升阶和外观保留幸运、分段累计，重连与旧档整写可恢复原实例。")
}
