// 本文件用隔离 MongoDB 验证上海运营日、托管卡到期和领取回执重放。
package integration

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestFirstLoopHostingDailyClaimsMongo 使用真实账号、角色和游戏所有者事务。
func TestFirstLoopHostingDailyClaimsMongo(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("需要隔离 MongoDB 副本集")
	}
	if os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Fatal("托管领取验收须由隔离联机运行器提供数据库")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_hostclaim_" + domain.ID()[:16]
	// ctx 只约束实际数据库操作；业务时刻由隔离测试明确设定。
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	account, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer account.Close(cleanup)
		must(t, account.DB.Drop(cleanup))
	})
	must(t, account.Singleton(ctx, "account"))
	game, err := persistence.Open(ctx, cfg)
	must(t, err)
	defer game.Close(context.Background())
	lease, err := account.ApproveGame(ctx, cfg.NodeID, game.Instance, cfg.RunID)
	must(t, err)
	game.Epoch = lease.Epoch

	login, err := account.Login(ctx, "hostclaim_"+domain.ID()[:8], "test_123456", true)
	must(t, err)
	accountAuth, err := account.Authenticate(ctx, login.AccountToken)
	must(t, err)
	summary, err := account.CreateRole(ctx, accountAuth, &pb.CreateRoleRequest{
		OriginServerId: cfg.OriginID, Slot: 1, Name: "托管日领" + domain.ID()[:6], OpId: domain.Bytes(domain.ID()),
	})
	must(t, err)
	ticket, err := account.IssueTicket(ctx, accountAuth, summary.Key)
	must(t, err)
	role, auth, err := game.Enter(ctx, ticket.Ticket)
	must(t, err)
	start := role

	// newClaim 固定客户端操作身份；测试时刻属于服务端输入，不进入请求摘要。
	newClaim := func(at time.Time) persistence.AssetRequest {
		t.Helper()
		request := persistence.AssetRequest{Kind: persistence.AssetClaimHostingTime, HostingClaimAt: at,
			Context: &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
				ExpectedRevision: uint64(role.Revision), ExpectedRuleVersion: cfg.Fixture.RuleVersion}}
		body, err := json.Marshal(request)
		must(t, err)
		request.Fingerprint = persistence.Hash(body)
		return request
	}
	// submit 提交真实事务，并要求经济操作可以推进经济水位但不使用战斗来源流水。
	submit := func(request persistence.AssetRequest, code uint32, balance int64) persistence.Operation {
		t.Helper()
		must(t, account.Renew(ctx, "account"))
		must(t, game.Renew(ctx, "game"))
		next, result, receipt, err := game.AssetWithReceipt(ctx, auth, request)
		must(t, err)
		if result.ErrorCode != code || receipt.Code != code || next.HostingSeconds != balance ||
			next.SourceSeq != start.SourceSeq || next.Wallet != start.Wallet ||
			result.CommandSeq != uint64(next.EconomicSeq) || result.NewRevision != uint64(next.Revision) {
			t.Fatalf("领取结果或水位错误：code=%d/%d，余额=%d，领取回执=%+v", result.ErrorCode, receipt.Code, next.HostingSeconds, receipt.HostingClaim)
		}
		role = next
		return receipt
	}
	// checkClaim 对照原事务回执、永久审计和角色当日额度。
	checkClaim := func(op persistence.Operation, at time.Time, day string, active bool, grant, total, balance int64) {
		t.Helper()
		claim := op.HostingClaim
		if claim == nil || !claim.AcceptedAt.Equal(at) || claim.OperatingDay != day ||
			claim.CardActive != active || claim.GrantedSeconds != grant || claim.ClaimedSeconds != total ||
			claim.DailyLimitSeconds != total || claim.BalanceSeconds != balance ||
			role.ClaimDay != day || role.ClaimedSeconds != total {
			t.Fatalf("领取回执与原运营日、权益或金额不一致：%+v", claim)
		}
		var row struct {
			Claim *persistence.HostingClaimReceipt `bson:"hosting_claim"` // 与领取在同一事务落盘的原明细。
		}
		must(t, account.DB.Collection("audit_outbox").FindOne(ctx, bson.M{"_id": op.ID}).Decode(&row))
		if !reflect.DeepEqual(row.Claim, claim) {
			t.Fatalf("原领取与审计明细不一致：%+v / %+v", claim, row.Claim)
		}
	}
	// setExpiry 仅模拟已发生的卡事实；本用例不依赖充值或续卡入口。
	setExpiry := func(until time.Time) {
		t.Helper()
		result, err := account.DB.Collection("roles").UpdateOne(ctx,
			bson.M{"_id": role.ID, "revision": role.Revision}, bson.M{"$set": bson.M{"hosting_card_until": until}})
		must(t, err)
		if result.MatchedCount != 1 {
			t.Fatal("测试角色已改变")
		}
	}

	const free = int64(6 * 3600)
	const card = int64(12 * 3600)
	// midnight 是上海午夜，仍是 UTC 的前一天；一秒内依次领取、补差和到期。
	midnight := time.Date(2026, 11, 25, 16, 0, 0, 0, time.UTC)
	freeAt, supplementAt := midnight.Add(-time.Second), midnight.Add(-500*time.Millisecond)
	first := newClaim(freeAt)
	firstReceipt := submit(first, 0, free)
	checkClaim(firstReceipt, freeAt, "2026-11-25", false, free, free, free)

	setExpiry(midnight)
	supplement := newClaim(supplementAt)
	supplementReceipt := submit(supplement, 0, card)
	checkClaim(supplementReceipt, supplementAt, "2026-11-25", true, free, card, card)
	var durableSupplement persistence.Operation // 原事务落盘后保留的完整回执。
	must(t, account.DB.Collection("operations").FindOne(ctx, bson.M{"_id": supplementReceipt.ID}).Decode(&durableSupplement))
	// 当日续卡后已领总额仍是十二小时；续卡事实之后再恢复到用于到期等号的隔离前态。
	setExpiry(midnight.Add(time.Hour))
	rejected := submit(newClaim(midnight.Add(-time.Millisecond)), p.CodeOperationConflict, card)
	if rejected.HostingClaim != nil || role.ClaimedSeconds != card {
		t.Fatal("同日续卡重复授予托管时长")
	}
	setExpiry(midnight)

	nextDay := submit(newClaim(midnight), 0, card+free)
	checkClaim(nextDay, midnight, "2026-11-26", false, free, free, card+free)
	stale := submit(newClaim(midnight.Add(-time.Millisecond)), p.CodeOperationConflict, card+free)
	if stale.HostingClaim != nil || role.ClaimDay != "2026-11-26" || role.ClaimedSeconds != free {
		t.Fatal("过时受理时刻把已确认运营日倒拨")
	}
	watermark := role.EconomicSeq
	// 即使客户端在次日重放且两张卡状态都已改变，也只返回原成功和当前水位。
	supplement.HostingClaimAt = midnight.Add(12 * time.Hour)
	replayed := submit(supplement, 0, card+free)
	if !reflect.DeepEqual(replayed, durableSupplement) || role.EconomicSeq != watermark {
		t.Fatal("旧操作回执或当前命令水位被重放改写")
	}

	// 旧数据中只有原成功码，没有类型化领取明细。
	_, err = account.DB.Collection("operations").UpdateOne(ctx, bson.M{"_id": firstReceipt.ID},
		bson.M{"$unset": bson.M{"hosting_claim": ""}})
	must(t, err)
	first.HostingClaimAt = midnight.Add(13 * time.Hour)
	legacy := submit(first, 0, card+free)
	if legacy.ID != firstReceipt.ID || legacy.HostingClaim != nil || role.EconomicSeq != watermark {
		t.Fatal("旧格式领取回执重放触发了补领或改写")
	}
	for i := 1; i <= 2; i++ {
		at := midnight.AddDate(0, 0, i)
		out := submit(newClaim(at), 0, card+free*int64(1+i))
		checkClaim(out, at, at.Add(8*time.Hour).Format("2006-01-02"), false, free, free, card+free*int64(1+i))
	}
}
