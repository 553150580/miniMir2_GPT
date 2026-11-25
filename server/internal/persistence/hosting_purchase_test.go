// 本文件核对托管金币补时的当前报价、准入门槛和真实 Mongo 幂等回执。
package persistence

import (
	"context"
	"math"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

func TestHostingPurchaseQuoteAndCommitBounds(t *testing.T) {
	at := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	fixture := devconfig.Fixture{RuleVersion: 30, Hash: "隔离补时配置", HostingTimePrices: []devconfig.HostingTimePriceVersion{
		testHostingTimePrice(73, at.Add(-time.Hour), at.Add(time.Minute), 60),
		testHostingTimePrice(74, at.Add(time.Minute), time.Time{}, 90),
	}}
	role := domain.Role{Level: 19, Revision: 8, HostingSeconds: 86399,
		Wallet: domain.Wallet{BoundGold: 20, Gold: 100000, BoundYuanbao: 4, Yuanbao: 5}}
	quote, code := QuoteHostingTime(role, &fixture, 0, at)
	if code != 0 || quote.QuoteVersion != 73 || quote.BandMinLevel != 1 || quote.BandMaxLevel != 19 || quote.GranularitySeconds != 60 ||
		quote.RequestedSeconds != 60 || quote.ReferenceGoldPerHour != 1801 || quote.GoldPerHour != 3602 || quote.TotalGold != 61 ||
		quote.BeforeSeconds != 86399 || quote.BoundGoldCost != 20 || quote.GoldCost != 41 || quote.Proto().GetExpiresAtUnixMs() != 0 {
		t.Fatal("按配置最小粒度发现时数、整笔向上取整或绑定金币优先错误", quote, code)
	}
	terms := testHostingPurchaseTerms(quote, role, &fixture, 13, at.Add(2*time.Minute))
	changed, receipt, code := applyHostingPurchase(role, &fixture, &terms, 13, at)
	wantWallet := domain.Wallet{Gold: 99959, BoundYuanbao: 4, Yuanbao: 5}
	if code != 0 || receipt == nil || receipt.Quote != terms.Quote || receipt.BalanceSeconds != 86459 || changed.HostingSeconds != 86459 || changed.Wallet != wantWallet {
		t.Fatal("余额少一秒应可购买并可超过24小时，且只扣两个原金币钱包", changed.HostingSeconds, changed.Wallet, receipt, code)
	}
	maximum, code := QuoteHostingTime(role, &fixture, devconfig.MaxHostingTimePurchaseSeconds, at)
	maxTerms := testHostingPurchaseTerms(maximum, role, &fixture, 13, at.Add(time.Minute))
	maxed, _, maxCode := applyHostingPurchase(role, &fixture, &maxTerms, 13, at)
	if code != 0 || maxCode != 0 || maxed.HostingSeconds != 172799 || maximum.TotalGold != 86448 {
		t.Fatal("单笔完整24小时计算错误或被错误封顶", maximum, maxCode, maxed.HostingSeconds)
	}
	for _, bad := range []uint32{59, 61, 86401, math.MaxUint32} {
		if _, code := QuoteHostingTime(role, &fixture, bad, at); code != p.CodeInvalidArgument {
			t.Fatal("无效粒度或超上限未拦截", bad, code)
		}
	}
	capped, poor := role, role
	capped.HostingSeconds = 86400
	poor.Wallet.Gold = 40
	for _, check := range []struct {
		role domain.Role
		want uint32
	}{{capped, p.CodeHostingPurchaseBalance}, {changed, p.CodeHostingPurchaseBalance}, {poor, p.CodeHostingPurchaseFunds}} {
		visible, code := QuoteHostingTime(check.role, &fixture, 60, at)
		snapshot := testHostingPurchaseTerms(visible, check.role, &fixture, 13, at.Add(time.Minute))
		unchanged, denied, buyCode := applyHostingPurchase(check.role, &fixture, &snapshot, 13, at)
		if code != check.want || buyCode != check.want || visible.TotalGold != 61 || denied != nil ||
			unchanged.Wallet != check.role.Wallet || unchanged.HostingSeconds != check.role.HostingSeconds {
			t.Fatal("余额或钱包检查应保留显示报价与原资产", visible, code, buyCode, denied)
		}
	}
	if _, code := QuoteHostingTime(role, nil, 0, at); code != p.CodeHostingPurchaseUnavailable {
		t.Fatal("无批准价表仍出现报价", code)
	}
	if _, _, code := applyHostingPurchase(role, &fixture, nil, 13, at); code != p.CodeHostingPurchaseQuoteStale {
		t.Fatal("未经过服务器签发的请求可进入购买", code)
	}
	for _, check := range []struct {
		name string
		vary func(*domain.Role, *HostingPurchaseQuote, *time.Time, *uint64)
		want uint32
	}{
		{"抬到另一等级档但价格碰巧相同", func(r *domain.Role, _ *HostingPurchaseQuote, _ *time.Time, _ *uint64) { r.Level = 20 }, p.CodeHostingPurchaseQuoteStale},
		{"时刻进入新版本及不同粒度", func(_ *domain.Role, _ *HostingPurchaseQuote, at *time.Time, _ *uint64) { *at = at.Add(time.Minute) }, p.CodeHostingPurchaseQuoteStale},
		{"角色修订变化", func(r *domain.Role, _ *HostingPurchaseQuote, _ *time.Time, _ *uint64) { r.Revision++ }, p.CodeHostingPurchaseQuoteStale},
		{"交易支付拆分变化", func(r *domain.Role, _ *HostingPurchaseQuote, _ *time.Time, _ *uint64) { r.Wallet.BoundGold++ }, p.CodeHostingPurchaseQuoteStale},
		{"场景迁移", func(_ *domain.Role, _ *HostingPurchaseQuote, _ *time.Time, scene *uint64) { *scene++ }, p.CodeHostingPurchaseQuoteStale},
		{"报价精确到期", func(_ *domain.Role, q *HostingPurchaseQuote, at *time.Time, _ *uint64) { q.ExpiresAt = *at }, p.CodeHostingPurchaseQuoteStale},
		{"整笔金额被替换", func(_ *domain.Role, q *HostingPurchaseQuote, _ *time.Time, _ *uint64) { q.TotalGold-- }, p.CodeHostingPurchaseQuoteStale},
		{"零秒只准发现报价", func(_ *domain.Role, q *HostingPurchaseQuote, _ *time.Time, _ *uint64) { q.RequestedSeconds = 0 }, p.CodeInvalidArgument},
	} {
		t.Run(check.name, func(t *testing.T) {
			current, original, now, scene := role, terms, at, uint64(13)
			check.vary(&current, &original.Quote, &now, &scene)
			unchanged, receipt, code := applyHostingPurchase(current, &fixture, &original, scene, now)
			if code != check.want || receipt != nil || unchanged.Wallet != current.Wallet || unchanged.HostingSeconds != current.HostingSeconds {
				t.Fatal("陈旧或不合法报价改变了原资产", code, receipt)
			}
		})
	}
}

func TestHostingPurchaseDurableMongo(t *testing.T) {
	// 随机场景代次的最高位可以置位，必须跨 BSON 持久化与回执重放保持原值。
	const scene = uint64(1)<<63 | 13
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("托管补时并发测试只使用当前任务副本集")
	}
	if !regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`).MatchString(uri) {
		t.Fatal("未提供隔离控制器分配的地址")
	}
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, stop := context.WithTimeout(t.Context(), 30*time.Second)
	defer stop()
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	require(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_buytime_"+domain.ID()[:16]
	now := time.Now().UTC().Truncate(time.Millisecond)
	cfg.Fixture.HostingTimePrices = []devconfig.HostingTimePriceVersion{testHostingTimePrice(73, now.Add(-time.Hour), now.Add(time.Hour), 60)}
	account, err := Open(ctx, cfg)
	require(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer account.Close(cleanup)
		if err := account.DB.Drop(cleanup); err != nil {
			t.Error(err)
		}
	})
	require(account.Singleton(ctx, "account"))
	game, err := Open(ctx, cfg)
	require(err)
	defer game.Close(context.Background())
	lease, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	require(err)
	game.Epoch = lease.Epoch
	login, err := account.Login(ctx, "buytime_"+domain.ID()[:8], "test_123456", true)
	require(err)
	owner, err := account.Authenticate(ctx, login.AccountToken)
	require(err)
	character, err := account.CreateRole(ctx, owner, &pb.CreateRoleRequest{OriginServerId: cfg.OriginID, Slot: 1, Name: "隔离补时角色", OpId: domain.Bytes(domain.ID())})
	require(err)
	ticket, err := account.IssueTicket(ctx, owner, character.Key)
	require(err)
	role, auth, err := game.Enter(ctx, ticket.Ticket)
	require(err)
	_, err = game.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": role.ID}, bson.M{"$set": bson.M{
		"hosting_seconds": int64(86399), "wallet.bound_gold": int64(20), "wallet.gold": int64(100)}})
	require(err)
	role, err = game.Role(ctx, role.ID)
	require(err)
	quote, code := QuoteHostingTime(role, &game.Cfg.Fixture, 60, now)
	if code != 0 || quote.TotalGold != 61 {
		t.Fatal("隔离配置不能报价", quote, code)
	}
	request := func(r domain.Role, terms *HostingPurchaseTerms) AssetRequest {
		id := domain.ID()
		return AssetRequest{Kind: AssetBuyHostingTime, SceneEpoch: scene, Fingerprint: Hash([]byte(id)), HostingPurchase: terms,
			Context: &pb.OperationContext{OpId: domain.Bytes(id), CommandSeq: uint64(r.EconomicSeq + 1),
				ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: game.Cfg.Fixture.RuleVersion}}
	}
	type response struct {
		request AssetRequest
		role    domain.Role
		result  *pb.OperationResult
		receipt Operation
		err     error
	}
	results := make(chan response, 2)
	var workers sync.WaitGroup
	for range 2 {
		terms := testHostingPurchaseTerms(quote, role, &game.Cfg.Fixture, scene, now.Add(time.Minute))
		q := request(role, &terms)
		workers.Go(func() {
			out, result, receipt, err := game.AssetWithReceipt(ctx, auth, q)
			results <- response{q, out, result, receipt, err}
		})
	}
	workers.Wait()
	close(results)
	var winner response
	var success, conflict int
	for claim := range results {
		if Code(claim.err) == p.CodeOperationConflict {
			conflict++
			continue
		}
		require(claim.err)
		if claim.result.ErrorCode != 0 || claim.result.GetHostingPurchase().GetQuote().GetSceneEpoch() != scene {
			t.Fatal("未得到可确认的成交回执", claim.result)
		}
		winner, success = claim, success+1
	}
	if success != 1 || conflict != 1 || winner.role.HostingSeconds != 86459 || winner.role.Wallet.BoundGold != 0 || winner.role.Wallet.Gold != 59 {
		t.Fatal("并发补时未严格检查原余额或只交易一次", success, conflict, winner.role.HostingSeconds, winner.role.Wallet)
	}
	var audited struct {
		Purchase *HostingPurchaseReceipt `bson:"hosting_purchase"`
	}
	require(game.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": winner.receipt.ID, "kind": AssetBuyHostingTime}).Decode(&audited))
	if audited.Purchase == nil || *audited.Purchase != *winner.receipt.HostingPurchase || !proto.Equal(audited.Purchase.Proto(), winner.result.HostingPurchase) {
		t.Fatal("审计、资产回执与协议回执未使用同一笔成交", audited.Purchase, winner.receipt.HostingPurchase)
	}
	// 价格配置撤回且场景临时报价消失后，旧操作继续返回最初的确认报酬。
	game.Cfg.Fixture.HostingTimePrices = nil
	replay := winner.request
	replay.HostingPurchase, replay.SceneEpoch = nil, 99
	current, result, repeated, err := game.AssetWithReceipt(ctx, auth, replay)
	require(err)
	if result.ErrorCode != 0 || current.Revision != winner.role.Revision || !proto.Equal(result.HostingPurchase, winner.result.HostingPurchase) ||
		repeated.HostingPurchase == nil || *repeated.HostingPurchase != *audited.Purchase || current.Wallet != winner.role.Wallet || current.HostingSeconds != 86459 {
		t.Fatal("冷报价重放重新计价或重复扣款", current.HostingSeconds, result)
	}
	sameIDChangedContent := replay
	sameIDChangedContent.Fingerprint = Hash([]byte("篡改的同一请求"))
	_, result, _, err = game.AssetWithReceipt(ctx, auth, sameIDChangedContent)
	if Code(err) != p.CodeOperationConflict || result.HostingPurchase != nil {
		t.Fatal("同一操作号绑定的请求内容可以改变", result, err)
	}
	unchanged, rejected, denial, err := game.AssetWithReceipt(ctx, auth, request(current, nil))
	require(err)
	if rejected.ErrorCode != p.CodeHostingPurchaseUnavailable || denial.HostingPurchase != nil || unchanged.Wallet != current.Wallet || unchanged.HostingSeconds != current.HostingSeconds {
		t.Fatal("无生效价表时出现新扣款或伪造成交回执", rejected, denial)
	}
	count, err := game.DB.Collection("operations").CountDocuments(ctx, bson.M{"role_id": role.ID, "kind": AssetBuyHostingTime, "hosting_purchase": bson.M{"$exists": true}})
	require(err)
	if count != 1 {
		t.Fatal("原角色重复生成补时成交回执", count)
	}
}

func testHostingTimePrice(version uint64, from, until time.Time, step uint32) devconfig.HostingTimePriceVersion {
	return devconfig.HostingTimePriceVersion{Version: version, EffectiveFrom: from, EffectiveUntil: until, PurchaseStepSeconds: step,
		Bands: []devconfig.HostingTimePriceBand{
			{LevelMin: 1, LevelMax: 19, GRefGoldPerHour: 1801}, {LevelMin: 20, LevelMax: 29, GRefGoldPerHour: 1801},
			{LevelMin: 30, LevelMax: 39, GRefGoldPerHour: 1801}, {LevelMin: 40, LevelMax: 49, GRefGoldPerHour: 1801},
			{LevelMin: 50, GRefGoldPerHour: 1801},
		}}
}

func testHostingPurchaseTerms(quote HostingPurchaseQuote, r domain.Role, f *devconfig.Fixture, scene uint64, expires time.Time) HostingPurchaseTerms {
	quote.QuoteID, quote.SceneEpoch, quote.RoleRevision = domain.ID(), int64(scene), uint64(r.Revision)
	quote.RuleVersion, quote.ConfigHash, quote.ExpiresAt = f.RuleVersion, f.Hash, expires
	return HostingPurchaseTerms{Quote: quote}
}
