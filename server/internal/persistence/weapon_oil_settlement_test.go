// 本文件用真实 MongoDB 复核用油报价围栏、原油堆变更、保底和已确认回执重放。
package persistence

import (
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestWeaponOilSettlementMongo 验证过期/原条件变化零消耗和两次真实结算原子确认。
func TestWeaponOilSettlementMongo(t *testing.T) {
	ctx, account, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_LUCK_OIL_TEST_FIXTURE")
	fixture := &game.Cfg.Fixture
	if fixture.LuckOil == nil || len(fixture.LuckOil.Steps) != 9 || fixture.LuckOil.Steps[5].PityAfter != 100 ||
		fixture.LuckOil.Steps[8].PityAfter != 3000 || fixture.LuckOil.Steps[5].SuccessPPM != 200_000 {
		t.Fatal("隔离测试必须采用工作簿投影的真实武器祝福油规则")
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	weapon, hasWeapon := fixture.Item(218)
	oil, hasOil := fixture.Item(fixture.LuckOil.ItemID)
	if !hasWeapon || !weapon.IsEquipment || weapon.EquipmentSlot != 0 || !hasOil || !oil.Stackable || oil.MaxStack < 5 {
		t.Fatal("隔离夹具没有真实武器或可堆叠祝福油定义", weapon, oil)
	}
	// 只在隔离库给初始角色准备两段门槛前态，实际扣油、版本、绑定和回执由资产事务生成。
	worn := InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: weapon.ID, Quantity: 1, Version: 3,
		InitialLuck: 3, CurrentLuck: 5, LuckOilToSix: 99, Quality: weapon.DropQuality, SourceKind: "monster_drop", SourceID: "1"}, fixture)
	bag := InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: weapon.ID, Quantity: 1, Version: 7,
		InitialLuck: 2, CurrentLuck: 8, LuckOilToSix: 102, LuckOilToNine: 3000,
		Quality: weapon.DropQuality, SourceKind: "monster_drop", SourceID: "2"}, fixture)
	bound := domain.Item{ID: domain.ID(), ItemID: oil.ID, Quantity: 3, Version: 1, Bound: true, SourceKind: "npc_shop", SourceID: "oil"}
	free := domain.Item{ID: domain.ID(), ItemID: oil.ID, Quantity: 5, Version: 2, SourceKind: "task_reward", SourceID: "oil"}
	items := []domain.Item{bag, bound, free}
	equipped := []domain.Equipped{{Slot: 0, Item: worn}}
	wallet := domain.Wallet{Gold: 11, BoundGold: 15, Yuanbao: 2, BoundYuanbao: 3}
	roles := game.DB.Collection("roles")
	_, err := roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": items, "equipped": equipped, "wallet": wallet}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	preview := func(item domain.Item, equipped, allowBound bool) *pb.WeaponOilQuote {
		t.Helper()
		req := weaponOilQuoteRequest(item, equipped, allowBound)
		quote, err := game.PreviewWeaponOil(ctx, auth, req)
		must(err)
		if !proto.Equal(quote.Weapon, item.Proto()) || quote.Equipped != equipped || quote.AllowBound != allowBound ||
			!reflect.DeepEqual(quote.UiSessionId, req.UiSessionId) || len(quote.QuoteId) != 16 || quote.SceneEpoch != req.SceneEpoch {
			t.Fatal("报价未固定原武器、位置或当前确认界面", quote)
		}
		return quote
	}
	request := func(quote *pb.WeaponOilQuote) AssetRequest {
		t.Helper()
		command := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
			ExpectedRevision: quote.RoleRevision, ExpectedRuleVersion: fixture.RuleVersion}
		return AssetRequest{Kind: AssetWeaponOilCommit, Context: command, SceneEpoch: quote.SceneEpoch,
			WeaponOilAcceptedAt: time.Now(), WeaponOilCommit: &pb.WeaponOilCommitRequest{Operation: command, QuoteId: quote.QuoteId,
				UiSessionId: quote.UiSessionId, AllowBound: quote.AllowBound, Weapon: &pb.WeaponOilItemRef{InstanceId: quote.Weapon.InstanceId,
					Version: quote.Weapon.Version, Equipped: quote.Equipped}}}
	}
	withFingerprint := func(q AssetRequest) AssetRequest {
		t.Helper()
		raw, err := proto.Marshal(q.WeaponOilCommit)
		must(err)
		q.Fingerprint = Hash(raw)
		return q
	}
	denies := func(q AssetRequest, expected uint32) {
		t.Helper()
		after, response, operation, err := game.AssetWithReceipt(ctx, auth, withFingerprint(q))
		must(err)
		if response.ErrorCode != expected || response.WeaponOil != nil || operation.WeaponOil != nil || after.Wallet != role.Wallet ||
			!reflect.DeepEqual(after.Equipped, role.Equipped) || !reflect.DeepEqual(after.Items, role.Items) {
			t.Fatal("错误报价改变资产或没有返回原拒绝", response, operation, after.Items)
		}
		role = after
	}
	expired := preview(worn, true, true)
	q := request(expired)
	q.WeaponOilAcceptedAt = time.UnixMilli(int64(expired.ExpiresAtUnixMs))
	denies(q, p.CodeWeaponOilQuoteChanged)
	currentQuote := preview(worn, true, true)
	q = request(currentQuote)
	q.WeaponOilCommit.UiSessionId = domain.Bytes(domain.ID())
	denies(q, p.CodeWeaponOilQuoteChanged)
	q = request(currentQuote)
	q.WeaponOilCommit.AllowBound = false
	denies(q, p.CodeWeaponOilQuoteChanged)
	q = request(currentQuote)
	q.WeaponOilCommit.Weapon.Equipped = false
	denies(q, p.CodeWeaponOilQuoteChanged)
	q = request(currentQuote)
	q.SceneEpoch++
	denies(q, p.CodeWeaponOilQuoteChanged)
	q = request(currentQuote)
	q.WeaponOilGate = p.CodeWeaponOilUnavailable
	denies(q, p.CodeWeaponOilUnavailable)

	// 材料替换后必须重报价，不能偷偷换扣另一个同配置油堆。
	bound.Version++
	items[1] = bound
	_, err = roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": items}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	denies(request(currentQuote), p.CodeWeaponOilQuoteChanged)
	currentQuote = preview(worn, true, true)
	if !proto.Equal(currentQuote.Oil, bound.Proto()) || currentQuote.SuccessPpm != 200_000 || currentQuote.PityUsed != 99 ||
		currentQuote.PityAfter != 100 || currentQuote.Guaranteed || !currentQuote.OutputBound {
		t.Fatal("第100瓶必须展示实际绑定油和原200000百万分率", currentQuote)
	}
	// 只控制隔离库隐藏样本，实际工作簿概率、原物品和服务器事务保持原逻辑。
	quoteKey := bson.M{"_id": role.ID, "quote_id": domain.Hex(currentQuote.QuoteId)}
	changed, err := game.DB.Collection(weaponOilQuoteCollection).UpdateOne(ctx, quoteKey, bson.M{"$set": bson.M{"roll": weaponOilPPM - 1}})
	must(err)
	if changed.MatchedCount != 1 {
		t.Fatal("当前隔离报价缺失")
	}
	failure := withFingerprint(request(currentQuote))
	after, failReply, failOp, err := game.AssetWithReceipt(ctx, auth, failure)
	must(err)
	failedWeapon := worn
	failedWeapon.CurrentLuck, failedWeapon.LuckOilToSix, failedWeapon.Bound, failedWeapon.Version = 3, 100, true, worn.Version+1
	bound.Quantity, bound.Version = bound.Quantity-1, bound.Version+1
	if failReply.ErrorCode != 0 || failReply.WeaponOil == nil || failReply.WeaponOil.Success || failReply.WeaponOil.Guaranteed ||
		failReply.WeaponOil.PityUsed != 100 || failReply.WeaponOil.Weapon.CurrentLuck != 3 || failOp.WeaponOil == nil ||
		failOp.WeaponOil.OilConsumed != 1 || failOp.WeaponOil.Weapon != failedWeapon ||
		after.Equipped[0].Item != failedWeapon || after.Items[1] != bound || after.Items[2] != free ||
		after.Wallet != wallet || after.EconomicSeq != role.EconomicSeq+1 {
		t.Fatal("绑定油失败未同笔扣瓶、绑原武器、退到3并保留原计数", failReply, after.Items)
	}
	role = after
	obsolete := preview(bag, false, false)
	previousAuth := auth
	freshTicket, err := account.IssueTicket(ctx, auth, role.Summary().Key)
	must(err)
	role, auth, err = game.Enter(ctx, freshTicket.Ticket)
	must(err)
	if auth.SessionEpoch == previousAuth.SessionEpoch {
		t.Fatal("隔离角色没有换到新会话")
	}
	denies(request(obsolete), p.CodeWeaponOilQuoteChanged)
	currentQuote = preview(bag, false, false)
	if !proto.Equal(currentQuote.Oil, free.Proto()) || currentQuote.PityUsed != 3000 || !currentQuote.Guaranteed ||
		currentQuote.SuccessPpm != weaponOilPPM || currentQuote.OutputBound || currentQuote.SuccessLuck != 9 {
		t.Fatal("第3000瓶后的背包武器报价未逐档必成或泄漏绑定投入", currentQuote)
	}
	success := withFingerprint(request(currentQuote))
	settled, err := game.Settle(ctx, auth, Settlement{SourceSeq: role.SourceSeq + 1, Gold: 1})
	must(err)
	if settled.Revision <= role.Revision || settled.EconomicSeq != role.EconomicSeq || !reflect.DeepEqual(settled.Items, role.Items) {
		t.Fatal("准备的原挂机收益没有单独推进角色修订", settled)
	}
	role = settled
	// 两个相同的原请求竞跑，数据库只能生成一笔原扣油和一份确认事实。
	type attempt struct {
		role  domain.Role
		reply *pb.OperationResult
		op    Operation
		err   error
	}
	results := make(chan attempt, 2)
	for range 2 {
		go func() {
			out, reply, op, err := game.AssetWithReceipt(ctx, auth, success)
			results <- attempt{out, reply, op, err}
		}()
	}
	completed, duplicate := <-results, <-results
	must(completed.err)
	must(duplicate.err)
	bumped := bag
	bumped.CurrentLuck, bumped.LuckOilToNine, bumped.Version = 9, 3001, bag.Version+1
	remaining := free
	remaining.Quantity, remaining.Version = free.Quantity-1, free.Version+1
	if completed.reply.ErrorCode != 0 || completed.reply.WeaponOil == nil || !completed.reply.WeaponOil.Success ||
		!completed.reply.WeaponOil.Guaranteed || completed.reply.WeaponOil.PityUsed != 3001 ||
		!proto.Equal(completed.reply.WeaponOil, duplicate.reply.WeaponOil) || completed.op.ID != duplicate.op.ID ||
		completed.op.WeaponOil == nil || completed.op.WeaponOil.Weapon != bumped || completed.role.Equipped[0].Item != failedWeapon ||
		!reflect.DeepEqual(completed.role.Items, []domain.Item{bumped, bound, remaining}) || completed.role.Wallet != settled.Wallet ||
		completed.role.EconomicSeq != settled.EconomicSeq+1 {
		t.Fatal("同原请求并发时未保证背包武器、保底、钱包及每次只扣一瓶", completed, duplicate)
	}
	role = completed.role
	denies(request(currentQuote), p.CodeWeaponOilQuoteChanged)
	// 已确认失败在旧场景、旧报价期限、新附着和门禁拒绝后，仍只返回原结果。
	failure.SceneEpoch++
	failure.WeaponOilAcceptedAt = time.Now().Add(time.Hour)
	failure.WeaponOilGate = p.CodeWeaponOilUnavailable
	replayed, replayReply, replayOp, err := game.AssetWithReceipt(ctx, auth, failure)
	must(err)
	if replayReply.ErrorCode != 0 || !proto.Equal(replayReply.WeaponOil, failReply.WeaponOil) || !reflect.DeepEqual(replayOp.WeaponOil, failOp.WeaponOil) ||
		replayOp.CommandSeq != failOp.CommandSeq || !reflect.DeepEqual(replayed, role) {
		t.Fatal("新附着重放返回内容或资产与原回执不同", replayReply, replayOp)
	}
	for _, op := range []Operation{failOp, completed.op} {
		var audit struct {
			WeaponOil *WeaponOilResultReceipt `bson:"weapon_oil"`
			Rule      uint64                  `bson:"rule_version"`
			Config    string                  `bson:"config_hash"`
		}
		must(game.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": op.ID, "kind": AssetWeaponOilCommit, "result_code": 0}).Decode(&audit))
		if !reflect.DeepEqual(audit.WeaponOil, op.WeaponOil) || audit.Rule != fixture.RuleVersion || audit.Config != fixture.Hash {
			t.Fatal("武器、原油、实际绑定或保底与同事务审计不一致", audit)
		}
	}
	t.Log("武器祝福油原账本核对通过：第100瓶绑定失败退至3，独立第3001瓶逐档必成至9，同原操作重放仅耗一瓶。")
}
