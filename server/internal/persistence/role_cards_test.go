// 本文件验证角色卡期、原报价、上海日领取与永久回执。
package persistence

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	p "minimir/internal/protocol"
)

// roleCardTestConfig 使用工作簿确认的两个元宝月卡与特权奖励。
func roleCardTestConfig() *devconfig.RoleCardConfig {
	return &devconfig.RoleCardConfig{DailyPrivilegeUnits: 2000, Products: []devconfig.RoleCardProduct{
		{ID: 7, Kind: devconfig.RoleCardKindPrivilege, Days: 30, PriceYuanbaoUnits: 30000},
		{ID: 8, Kind: devconfig.RoleCardKindHosting, Days: 30, PriceYuanbaoUnits: 45000},
	}}
}

// authorizeRoleCardQuote 模拟原房间通过角色与场景附着签发报价。
func authorizeRoleCardQuote(t *testing.T, r domain.Role, f *devconfig.Fixture, id uint32, scene uint64, at time.Time) RoleCardQuote {
	t.Helper()
	quote, code := QuoteRoleCard(r, f, id, at)
	if code != 0 {
		t.Fatalf("无法获得测试原报价：%d", code)
	}
	quote.QuoteID, quote.SceneEpoch, quote.ExpiresAt = domain.ID(), int64(scene), at.Add(time.Minute)
	return quote
}

// TestRoleCardQuoteFreezesTerms 覆盖元宝整额、到期边界与续期起点。
func TestRoleCardQuoteFreezesTerms(t *testing.T) {
	at := time.Date(2026, 11, 27, 15, 59, 59, 0, time.UTC)
	f := devconfig.Fixture{RoleCards: roleCardTestConfig(), Hash: Hash([]byte("原整卡配置")), RuleVersion: 30}
	r := domain.Role{ID: domain.ID(), Origin: 1, Revision: 2, Wallet: domain.Wallet{Yuanbao: 45000, BoundYuanbao: 90000},
		PrivilegeSince: at.Add(-time.Hour), PrivilegeUntil: at.Add(time.Second)}
	q := authorizeRoleCardQuote(t, r, &f, 7, 9, at)
	next, receipt, code := applyRoleCardPurchase(r, &f, &q, 9, at)
	if code != 0 || next.Wallet.Yuanbao != 15000 || next.Wallet.BoundYuanbao != 90000 ||
		!receipt.EffectiveAt.Equal(at) || !next.PrivilegeSince.Equal(r.PrivilegeSince) ||
		!next.PrivilegeUntil.Equal(r.PrivilegeUntil.Add(30*24*time.Hour)) || !next.HostingCardUntil.IsZero() {
		t.Fatal("续期未延长原卡或动了绑定钱包", next.Wallet, receipt, code)
	}
	for _, later := range []time.Time{r.PrivilegeUntil, q.ExpiresAt} {
		if _, _, code := applyRoleCardPurchase(r, &f, &q, 9, later); code != p.CodeOperationConflict {
			t.Fatalf("确认时卡期形态或报价窗口已变化仍成交：%d", code)
		}
	}
	mutated := f
	mutated.Hash = Hash([]byte("另一发布"))
	if _, _, code := applyRoleCardPurchase(r, &mutated, &q, 9, at); code != p.CodeOperationConflict {
		t.Fatalf("原报价未核对当前配置：%d", code)
	}
	r.Wallet.Yuanbao = q.PriceYuanbaoUnits - 1
	if _, _, code := applyRoleCardPurchase(r, &f, &q, 9, at); code != p.CodeVendorFunds {
		t.Fatalf("绑定元宝意外补足缺额：%d", code)
	}
	if RoleCardOperatingDay(at) != "2026-11-27" || RoleCardOperatingDay(at.Add(time.Second)) != "2026-11-28" {
		t.Fatal("角色卡未沿用上海运营日切换")
	}
}

// TestRoleCardsDurableMongo 验证角色独立购买、时间查证、绑定日领及原回执重放。
func TestRoleCardsDurableMongo(t *testing.T) {
	uri := os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI")
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || uri == "" {
		t.Skip("需要独立 MongoDB 副本集")
	}
	require := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	require(err)
	cfg.MongoURI, cfg.Database = uri, "minimir_dev_game_test_role_cards_"+domain.ID()[:16]
	cfg.Fixture.RoleCards = roleCardTestConfig()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	account, err := Open(ctx, cfg)
	require(err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		defer account.Close(cleanup)
		require(account.DB.Drop(cleanup))
	})
	require(account.Singleton(ctx, "account"))
	game, err := Open(ctx, cfg)
	require(err)
	defer game.Close(context.Background())
	lease, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	require(err)
	game.Epoch = lease.Epoch
	login, err := account.Login(ctx, "card_"+domain.ID()[:8], "test_123456", true)
	require(err)
	accountAuth, err := account.Authenticate(ctx, login.AccountToken)
	require(err)
	create := func(slot uint32, name string) (domain.Role, Auth) {
		t.Helper()
		summary, err := account.CreateRole(ctx, accountAuth, &pb.CreateRoleRequest{
			OriginServerId: cfg.OriginID, Slot: slot, Name: name, OpId: domain.Bytes(domain.ID()),
		})
		require(err)
		ticket, err := account.IssueTicket(ctx, accountAuth, summary.Key)
		require(err)
		role, auth, err := game.Enter(ctx, ticket.Ticket)
		require(err)
		return role, auth
	}
	r, auth := create(1, "月卡原角色")
	other, otherAuth := create(2, "月卡同账号")
	// 真实场景代次可使用 uint64 高半区，报价在 Mongo 中以有符号整数保存同一组位。
	const scene = uint64(1<<63) + 9
	seed := func(r domain.Role, fields bson.M) domain.Role {
		t.Helper()
		_, err := game.DB.Collection("roles").UpdateOne(ctx, bson.M{"_id": r.ID}, bson.M{"$set": fields})
		require(err)
		r, err = game.Role(ctx, r.ID)
		require(err)
		return r
	}
	request := func(r domain.Role, kind string) AssetRequest {
		id := domain.ID()
		return AssetRequest{Kind: kind, Fingerprint: Hash([]byte(id)), SceneEpoch: scene,
			Context: &pb.OperationContext{OpId: domain.Bytes(id), CommandSeq: uint64(r.EconomicSeq + 1),
				ExpectedRevision: uint64(r.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
	}
	submit := func(auth Auth, q AssetRequest, code uint32) (domain.Role, Operation) {
		t.Helper()
		role, result, op, err := game.AssetWithReceipt(ctx, auth, q)
		require(err)
		if result.ErrorCode != code || !reflect.DeepEqual(result.RoleCardPurchase, op.CardPurchase.Proto()) ||
			!reflect.DeepEqual(result.RoleCardDaily, op.CardDaily.Proto()) {
			t.Fatalf("错误码或回执非预期：期待 %d，实际 %+v", code, result)
		}
		return role, op
	}
	prior := bossTime(time.Now().Add(-2 * time.Hour))
	r = seed(r, bson.M{"privilege_until": prior, "wallet.yuanbao_units": 105000, "wallet.bound_yuanbao_units": 17})
	other = seed(other, bson.M{"wallet.bound_yuanbao_units": 100000})
	quote := authorizeRoleCardQuote(t, r, &cfg.Fixture, 7, scene, time.Now())
	foreign := request(other, AssetBuyRoleCard)
	foreign.CardPurchase = &quote
	submit(otherAuth, foreign, p.CodeOperationConflict)
	if _, code := QuoteRoleCard(other, &cfg.Fixture, 7, time.Now()); code != p.CodeVendorFunds {
		t.Fatal("同账号绑定钱包意外支付月卡")
	}

	buy := request(r, AssetBuyRoleCard)
	buy.CardPurchase = &quote
	r, original := submit(auth, buy, 0)
	if original.CardPurchase == nil || r.Wallet.Yuanbao != 75000 || r.Wallet.BoundYuanbao != 17 ||
		!r.PrivilegeSince.Equal(original.CardPurchase.EffectiveAt) || !r.PrivilegeUntil.Equal(r.PrivilegeSince.Add(30*24*time.Hour)) ||
		!r.HostingCardUntil.IsZero() {
		t.Fatal("原购买没有原子扣款或创建独立特权", original)
	}
	buy.CardPurchase = nil
	replayRole, replay := submit(auth, buy, 0)
	if !reflect.DeepEqual(replay.CardPurchase, original.CardPurchase) || replayRole.Wallet != r.Wallet || replayRole.Revision != r.Revision {
		t.Fatal("无新报价的重试改变了已确认扣款或卡期")
	}
	previous, err := game.RolePrivilegeUntilAt(ctx, r, prior.Add(-time.Millisecond))
	require(err)
	gap, err := game.RolePrivilegeUntilAt(ctx, r, original.CardPurchase.EffectiveAt.Add(-time.Millisecond))
	require(err)
	active, err := game.RolePrivilegeUntilAt(ctx, r, original.CardPurchase.EffectiveAt)
	require(err)
	if !previous.Equal(prior) || !gap.IsZero() || !active.Equal(r.PrivilegeUntil) {
		t.Fatalf("旧角色权益、过期空窗或本次生效边界错误：%v %v %v", previous, gap, active)
	}

	late := request(r, AssetClaimRoleCardDaily)
	late.CardAcceptedAt = original.CardPurchase.EffectiveAt.Add(-time.Millisecond)
	late.CardDay = RoleCardOperatingDay(late.CardAcceptedAt)
	r, _ = submit(auth, late, p.CodeLineForbidden)
	daily := request(r, AssetClaimRoleCardDaily)
	daily.CardAcceptedAt = time.Now()
	daily.CardDay = RoleCardOperatingDay(daily.CardAcceptedAt)
	r, allowance := submit(auth, daily, 0)
	if allowance.CardDaily == nil || allowance.CardDaily.GrantedBoundYuanbaoUnits != 2000 || r.Wallet.BoundYuanbao != 2017 ||
		r.Wallet.Yuanbao != 75000 || r.PrivilegeClaimDay != daily.CardDay || r.ClaimDay != "" || r.ClaimedSeconds != 0 {
		t.Fatal("特权每日奖励未独立记账", allowance)
	}
	host := request(r, AssetBuyRoleCard)
	hostQuote := authorizeRoleCardQuote(t, r, &cfg.Fixture, 8, scene, time.Now())
	host.CardPurchase = &hostQuote
	r, hosted := submit(auth, host, 0)
	if hosted.CardPurchase == nil || !r.HostingCardActiveAt(hosted.CardPurchase.EffectiveAt) || r.HostingSeconds != 0 || r.Wallet.Yuanbao != 30000 {
		t.Fatal("托管购卡多发时长或扣错金额", hosted)
	}
	stale := request(r, AssetBuyRoleCard)
	stale.CardPurchase = &quote
	r, _ = submit(auth, stale, p.CodeOperationConflict)
	renew := request(r, AssetBuyRoleCard)
	renewalQuote := authorizeRoleCardQuote(t, r, &cfg.Fixture, 7, scene, time.Now())
	renew.CardPurchase = &renewalQuote
	r, renewed := submit(auth, renew, 0)
	if renewed.CardPurchase == nil || !r.PrivilegeSince.Equal(original.CardPurchase.ValidSince) ||
		!r.PrivilegeUntil.Equal(original.CardPurchase.ValidUntil.Add(30*24*time.Hour)) || r.Wallet.Yuanbao != 0 {
		t.Fatal("有效期内续购未按原到期顺延", renewed)
	}
	repeat := request(r, AssetClaimRoleCardDaily)
	repeat.CardDay, repeat.CardAcceptedAt = daily.CardDay, daily.CardAcceptedAt
	r, _ = submit(auth, repeat, p.CodeOperationConflict)
	replayRole, replay = submit(auth, daily, 0)
	if !reflect.DeepEqual(replay.CardDaily, allowance.CardDaily) || replayRole.Revision != r.Revision || replayRole.Wallet.BoundYuanbao != 2017 {
		t.Fatal("重复领取或续购改变了原每日回执")
	}
	other, err = game.Role(ctx, other.ID)
	require(err)
	if other.PrivilegeActiveAt(time.Now()) || other.HostingCardActiveAt(time.Now()) || other.Wallet.BoundYuanbao != 100000 {
		t.Fatal("同账号另一个角色继承了权益或被扣款")
	}
	periods, err := game.DB.Collection(roleCardPeriodCollection).CountDocuments(ctx, bson.M{"role_id": r.ID})
	require(err)
	audits, err := game.DB.Collection("audit_outbox").CountDocuments(ctx, bson.M{"role_id": r.ID, "kind": AssetBuyRoleCard, "result_code": 0})
	require(err)
	if periods != 3 || audits != 3 {
		t.Fatalf("权益变更和扣款未同时只入账一次：%d %d", periods, audits)
	}
}
