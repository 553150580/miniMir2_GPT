// 本文件以隔离 MongoDB 核对专属翅膀用油、两段保底、同事务扣瓶及原回执重放。
package persistence

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// TestWingOilSettlementMongo 让两个原保底门槛和正式资产幂等事务在实际副本集上一起运行。
func TestWingOilSettlementMongo(t *testing.T) {
	ctx, account, game, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_WING_OIL_TEST_FIXTURE")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fixture := &game.Cfg.Fixture
	if !wingOilEnabled(fixture) || len(fixture.LuckOil.Steps) != 9 ||
		fixture.LuckOil.Steps[5].PityAfter != 100 || fixture.LuckOil.Steps[5].SuccessPPM != 200_000 ||
		fixture.LuckOil.Steps[8].PityAfter != 3000 || fixture.LuckOil.Steps[8].SuccessPPM != 50_000 {
		t.Fatal("隔离实库必须载入正式投影的两段翅膀用油候选规则")
	}
	// 两种绑定油堆与原穿戴武器始终独立。只在隔离库准备已有培养历史。
	worn := InitializeItemDurability(domain.Item{ID: domain.ID(), ItemID: 218, Quantity: 1, Version: 6, CurrentLuck: 7,
		InitialLuck: 3, LuckOilToSix: 103, LuckOilToNine: 31, SourceKind: "monster_drop", SourceID: "wing_oil_test"}, fixture)
	free := domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 4, Version: 2, SourceKind: "task_reward", SourceID: "wing_oil_test"}
	bound := domain.Item{ID: domain.ID(), ItemID: fixture.LuckOil.ItemID, Quantity: 6, Version: 3, Bound: true, SourceKind: "npc_shop", SourceID: "wing_oil_test"}
	wing := domain.WingState{OwnerRoleID: role.ID, ID: domain.ID(), Version: 9, Rank: 3, AppearanceTier: 1, Hidden: true,
		Luck: 5, OilToSix: 99}
	bag := []domain.Item{free, bound}
	roles, wings := game.DB.Collection("roles"), game.DB.Collection(roleWingCollection)
	_, err := roles.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"items": bag, "equipped": []domain.Equipped{{Slot: 0, Item: worn}}}})
	must(err)
	if _, err = game.PreviewWeaponOil(ctx, auth, wingOilQuoteRequest(wing, false)); Code(err) != p.CodeWeaponOilInputs {
		t.Fatal("未激活自己的翅膀不能得到报价", err)
	}
	_, err = wings.InsertOne(ctx, wing)
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	preview := func(allowBound bool, expectedOil domain.Item, pity, after, odds uint32, guaranteed bool) *pb.WeaponOilQuote {
		t.Helper()
		req := wingOilQuoteRequest(*role.Wing, allowBound)
		q, err := game.PreviewWeaponOil(ctx, auth, req)
		must(err)
		if q.Weapon != nil || !proto.Equal(q.Wing, role.Wing.Proto()) || q.Equipped || !q.OutputBound ||
			!proto.Equal(q.Oil, expectedOil.Proto()) || q.AllowBound != allowBound || q.PityUsed != pity || q.PityAfter != after ||
			q.SuccessPpm != odds || q.Guaranteed != guaranteed || q.SuccessLuck != role.Wing.Luck+1 {
			t.Fatal("原翅膀报价字段、原油堆或首次抽样规则与实际状态不一致", q, role.Wing)
		}
		return q
	}
	commit := func(quote *pb.WeaponOilQuote) AssetRequest {
		command := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
			ExpectedRevision: quote.RoleRevision, ExpectedRuleVersion: fixture.RuleVersion}
		return AssetRequest{Kind: AssetWeaponOilCommit, Context: command, SceneEpoch: quote.SceneEpoch,
			WeaponOilAcceptedAt: time.Now(), WeaponOilCommit: &pb.WeaponOilCommitRequest{Operation: command,
				QuoteId: quote.QuoteId, UiSessionId: quote.UiSessionId, AllowBound: quote.AllowBound,
				Wing: &pb.WingOilRef{WingId: quote.Wing.WingId, Version: quote.Wing.Version}}}
	}
	fingerprint := func(q AssetRequest) AssetRequest {
		t.Helper()
		raw, err := proto.Marshal(q.WeaponOilCommit)
		must(err)
		q.Fingerprint = Hash(raw)
		return q
	}
	deny := func(q AssetRequest, expected uint32) {
		t.Helper()
		out, result, op, err := game.AssetWithReceipt(ctx, auth, fingerprint(q))
		must(err)
		if result.ErrorCode != expected || result.WeaponOil != nil || op.WeaponOil != nil || out.Wallet != role.Wallet ||
			!reflect.DeepEqual(out.Items, role.Items) || !reflect.DeepEqual(out.Equipped, role.Equipped) || !reflect.DeepEqual(out.Wing, role.Wing) {
			t.Fatal("拒绝的首次操作改变了原翼、油堆或武器", result, out.Wing, out.Items)
		}
		role = out
	}
	forceRoll := func(quote *pb.WeaponOilQuote, roll uint32) {
		t.Helper()
		changed, err := game.DB.Collection(weaponOilQuoteCollection).UpdateOne(ctx, bson.M{"_id": role.ID, "quote_id": domain.Hex(quote.QuoteId)},
			bson.M{"$set": bson.M{"roll": roll}})
		must(err)
		if changed.MatchedCount != 1 {
			t.Fatal("原隔离报价的私有抽样未找到")
		}
	}
	type confirmation struct {
		role  domain.Role         // 提交后的原角色当前状态。
		reply *pb.OperationResult // 本瓶确认回执。
		op    Operation           // 同事务存档的原回执。
		err   error               // 独立 MongoDB 或外围围栏错误。
	}
	execute := func(q AssetRequest) confirmation {
		out, reply, op, err := game.AssetWithReceipt(ctx, auth, q)
		return confirmation{out, reply, op, err}
	}
	verify := func(confirmed confirmation, oil *domain.Item, luck, firstUsed, secondUsed uint32, success, guaranteed bool) {
		t.Helper()
		must(confirmed.err)
		before := *role.Wing
		want := before
		want.Luck, want.OilToSix, want.OilToNine, want.Version = luck, firstUsed, secondUsed, before.Version+1
		result, persisted := confirmed.reply.WeaponOil, confirmed.op.WeaponOil
		if confirmed.reply.ErrorCode != 0 || result == nil || result.Weapon != nil || result.Equipped || result.Success != success ||
			result.BeforeLuck != before.Luck || result.Guaranteed != guaranteed || !proto.Equal(result.Wing, want.Proto()) ||
			!proto.Equal(result.Oil, oil.Proto()) || persisted == nil || persisted.OilConsumed != 1 || persisted.Plan.Oil != *oil ||
			!persisted.Plan.OutputBound || !reflect.DeepEqual(persisted.Plan.Wing, &before) || !reflect.DeepEqual(persisted.Wing, &want) {
			t.Fatal("扣瓶或原翼前后确认条款与实际操作不一致", confirmed.reply, confirmed.op.WeaponOil)
		}
		used := firstUsed
		if before.Luck >= 6 {
			used = secondUsed
		}
		if result.PityUsed != used || confirmed.role.Wallet != role.Wallet || confirmed.role.Equipped[0].Item != worn ||
			!reflect.DeepEqual(confirmed.role.Wing, &want) || confirmed.role.EconomicSeq != role.EconomicSeq+1 {
			t.Fatal("跨阶段用油改动了穿戴武器、钱包或原翅膀独立状态", confirmed.role, result)
		}
		oil.Quantity--
		oil.Version++
		var expected []domain.Item
		for _, item := range []domain.Item{free, bound} {
			if item.Quantity > 0 {
				expected = append(expected, item)
			}
		}
		if !slices.Equal(confirmed.role.Items, expected) || oil.Quantity == 0 && persisted.OilRemainder != nil ||
			oil.Quantity > 0 && !reflect.DeepEqual(persisted.OilRemainder, oil) {
			t.Fatal("原油堆部分扣减或耗尽没有按原实例更新", confirmed.role.Items, expected)
		}
		var audit struct {
			Oil    *WeaponOilResultReceipt `bson:"weapon_oil"`   // 同笔原确认数据。
			Rule   uint64                  `bson:"rule_version"` // 实际结算的规则版本。
			Config string                  `bson:"config_hash"`  // 实际结算的配置摘要。
		}
		must(game.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": confirmed.op.ID, "kind": AssetWeaponOilCommit}).Decode(&audit))
		if !reflect.DeepEqual(audit.Oil, persisted) || audit.Rule != fixture.RuleVersion || audit.Config != fixture.Hash {
			t.Fatal("翅膀用油永久回执和同事务审计不一致", audit)
		}
		role = confirmed.role
	}
	quote := preview(false, free, 99, 100, 200_000, false)
	q := commit(quote)
	q.WeaponOilCommit.Weapon = weaponOilQuoteRequest(worn, true, false).Weapon
	deny(q, p.CodeInvalidArgument)
	q = commit(quote)
	q.WeaponOilCommit.Wing = nil
	q.WeaponOilCommit.Weapon = weaponOilQuoteRequest(worn, true, false).Weapon
	deny(q, p.CodeWeaponOilQuoteChanged)
	q = commit(quote)
	q.WeaponOilCommit.Wing.Version++
	deny(q, p.CodeWeaponOilQuoteChanged)
	q = commit(quote)
	q.WeaponOilCommit.AllowBound = true
	deny(q, p.CodeWeaponOilQuoteChanged)
	q = commit(quote)
	q.WeaponOilAcceptedAt = time.UnixMilli(int64(quote.ExpiresAtUnixMs))
	deny(q, p.CodeWeaponOilQuoteChanged)
	forceRoll(quote, weaponOilPPM-1)
	original := fingerprint(commit(quote))
	firstFailure := execute(original)
	verify(firstFailure, &free, 3, 100, 0, false, false)
	obsolete := preview(true, bound, 100, 100, weaponOilPPM, true)
	ticket, err := account.IssueTicket(ctx, auth, role.Summary().Key)
	must(err)
	oldEpoch := auth.SessionEpoch
	role, auth, err = game.Enter(ctx, ticket.Ticket)
	must(err)
	if auth.SessionEpoch == oldEpoch || role.Inventory().Wing.GetLuck() != 3 {
		t.Fatal("重连没有恢复独立原翅膀状态", role.Wing)
	}
	deny(commit(obsolete), p.CodeWeaponOilQuoteChanged)
	quote = preview(true, bound, 100, 100, weaponOilPPM, true)
	forceRoll(quote, weaponOilPPM-1)
	firstPity := fingerprint(commit(quote))
	role, err = game.Settle(ctx, auth, Settlement{SourceSeq: role.SourceSeq + 1, Gold: 1})
	must(err)
	// 最后一步审计故障时，已扣油、原翼更新、原报价与永久回执必须一并回滚。
	auditKey := bson.M{"_id": "asset:" + role.ID + ":" + domain.Hex(firstPity.Context.OpId)}
	audits := game.DB.Collection("audit_outbox")
	_, err = audits.InsertOne(ctx, auditKey)
	must(err)
	if attempt := execute(firstPity); !mongo.IsDuplicateKeyError(attempt.err) {
		t.Fatal("隔离故障没有阻断最后的审计写入", attempt.err)
	}
	rolledBack, err := game.Role(ctx, role.ID)
	must(err)
	missing, err := game.DB.Collection("operations").CountDocuments(ctx, auditKey)
	must(err)
	if !reflect.DeepEqual(rolledBack, role) || missing != 0 {
		t.Fatal("最后一步失败仍然改变了原翅膀状态、油堆或永久回执", rolledBack.Wing, rolledBack.Items, missing)
	}
	_, err = audits.DeleteOne(ctx, auditKey)
	must(err)
	concurrent := make(chan confirmation, 2)
	for range 2 {
		go func() { concurrent <- execute(firstPity) }()
	}
	once, replay := <-concurrent, <-concurrent
	verify(once, &bound, 4, 101, 0, true, true)
	must(replay.err)
	if replay.reply.ErrorCode != 0 || !proto.Equal(replay.reply.WeaponOil, once.reply.WeaponOil) || replay.op.ID != once.op.ID || !reflect.DeepEqual(replay.role, role) {
		t.Fatal("两个相同请求未只扣同一个原油、确认一件原翅膀", replay.reply, once.reply)
	}
	apply := func(allowBound bool, oil *domain.Item, roll, before, limit, odds, luck, firstUsed, secondUsed uint32, success, guaranteed bool) {
		t.Helper()
		quote := preview(allowBound, *oil, before, limit, odds, guaranteed)
		forceRoll(quote, roll)
		verify(execute(fingerprint(commit(quote))), oil, luck, firstUsed, secondUsed, success, guaranteed)
	}
	apply(true, &bound, weaponOilPPM-1, 101, 100, weaponOilPPM, 5, 102, 0, true, true)
	apply(true, &bound, weaponOilPPM-1, 102, 100, weaponOilPPM, 6, 103, 0, true, true)
	apply(false, &free, 0, 0, 3000, fixture.LuckOil.Steps[6].SuccessPPM, 7, 103, 1, true, false)
	apply(false, &free, 0, 1, 3000, fixture.LuckOil.Steps[7].SuccessPPM, 8, 103, 2, true, false)
	_, err = wings.UpdateByID(ctx, role.ID, bson.M{"$set": bson.M{"oil_to_nine": 2999}, "$inc": bson.M{"version": 1}})
	must(err)
	role, err = game.Role(ctx, role.ID)
	must(err)
	apply(false, &free, weaponOilPPM-1, 2999, 3000, 50_000, 6, 103, 3000, false, false)
	apply(true, &bound, weaponOilPPM-1, 3000, 3000, weaponOilPPM, 7, 103, 3001, true, true)
	apply(true, &bound, weaponOilPPM-1, 3001, 3000, weaponOilPPM, 8, 103, 3002, true, true)
	apply(true, &bound, weaponOilPPM-1, 3002, 3000, weaponOilPPM, 9, 103, 3003, true, true)
	if _, err = game.PreviewWeaponOil(ctx, auth, wingOilQuoteRequest(*role.Wing, false)); Code(err) != p.CodeWeaponOilInputs {
		t.Fatal("幸运满档后仍能报价", err)
	}
	original.SceneEpoch++
	original.WeaponOilAcceptedAt = time.Now().Add(time.Hour)
	original.WeaponOilGate = p.CodeWeaponOilUnavailable
	old := execute(original)
	must(old.err)
	if old.reply.ErrorCode != 0 || !proto.Equal(old.reply.WeaponOil, firstFailure.reply.WeaponOil) ||
		!reflect.DeepEqual(old.op.WeaponOil, firstFailure.op.WeaponOil) || !reflect.DeepEqual(old.role, role) {
		t.Fatal("原操作重放必须返回首次失败回执，同时保留当前满幸运资产", old.reply, old.role)
	}
	var durable domain.WingState
	must(wings.FindOne(ctx, bson.M{"_id": role.ID}).Decode(&durable))
	if durable != *role.Wing || durable.ID != wing.ID || durable.Rank != wing.Rank || durable.AppearanceTier != wing.AppearanceTier || !durable.Hidden {
		t.Fatal("用油未把独立的同一原翼保存到永久表", durable)
	}
	t.Log("翅膀用油原账核对通过：第100和第3000瓶原概率失败回退，下一瓶起逐档必成；原翅膀、外观、隐藏和另一段累计均保留，同原操作交易只扣一瓶。")
}
