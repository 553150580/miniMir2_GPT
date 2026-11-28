// 本文件用可控上海时钟验证历史资格、一次奖水位和旧支付摘要。
package persistence

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/payment"
)

// TestBattlePassCalendarAndLegacyProof 同时覆盖旧支付 JSON 摘要和零点后允许原单晚到账。
func TestBattlePassCalendarAndLegacyProof(t *testing.T) {
	const start = "2026-01-01"
	rules := &devconfig.BattlePassRules{AnchorDay: start}
	boundary := time.Date(2026, 1, 31, 0, 0, 0, 0, shanghaiOperatingZone)
	previous := boundary.Add(-time.Millisecond)
	if battlePassWindow(rules, previous) != start || battlePassWindow(rules, boundary) != "2026-01-31" ||
		battlePassDay(start, shanghaiOperatingDay(previous)) != 30 || battlePassDay(start, shanghaiOperatingDay(boundary)) != 0 {
		t.Fatal("30 日窗口未在上海零点切换")
	}
	order := PaymentOrder{Quote: PaymentOrderQuote{BattlePass: BattlePassOrderTerms{Start: start}}, CreatedAt: previous.Add(-time.Second), VerifiedAt: boundary}
	if !order.acceptsBattlePassPaidAt(previous) || order.acceptsBattlePassPaidAt(boundary) || order.acceptsBattlePassPaidAt(order.CreatedAt.Add(-time.Millisecond)) {
		t.Fatal("原单应按付款时刻而非核验时刻判定周期")
	}
	oldQuote := PaymentOrderQuote{Kind: payment.KindPrivilegeCard, ProductID: 2, AmountFen: 7800, CardDays: 90,
		RuleVersion: 30, ConfigHash: strings.Repeat("a", 64), ProviderID: "provider", MerchantID: "merchant"}
	bytes, err := json.Marshal(oldQuote)
	if err != nil || string(bytes) != `{"Kind":"privilege_card","ProductID":2,"AmountFen":7800,"YuanbaoUnits":0,"CardDays":90,"RuleVersion":30,"ConfigHash":"`+strings.Repeat("a", 64)+`","ProviderID":"provider","MerchantID":"merchant"}` {
		t.Fatal("已有支付证明的原报价编码发生变化", string(bytes), err)
	}
}

// TestBattlePassHistoryMongo 用同一个拥有真实租约的角色跨期保存资格，并从实际资产事务补领。
func TestBattlePassHistoryMongo(t *testing.T) {
	ctx, _, store, role, auth := startIsolatedMiningStoreTest(t, "MINIMIR_BATTLE_PASS_TEST_FIXTURE")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	fixture := &store.Cfg.Fixture
	start, ok := battlePassDate(fixture.BattlePass.AnchorDay)
	if !ok {
		t.Fatal("没有隔离战令日历")
	}
	middle := time.Date(start.Year(), start.Month(), start.Day(), 23, 59, 59, 0, shanghaiOperatingZone)
	last := middle.AddDate(0, 0, 29)
	newStart := last.Add(time.Second)
	assertQualification := func(at time.Time) {
		t.Helper()
		// 单独固定原日活已确认前态，普通挂机与六个日常真实领取由 WebSocket 验收。
		source := role
		source.TasksCodex = &domain.TaskCodexProgress{Daily: domain.TaskCodexDailyProgress{Day: shanghaiOperatingDay(at),
			Activity: fixture.TasksCodex.DailyActivityTarget}}
		must(store.tx(ctx, func(tc context.Context) error { return store.recordBattlePassActivity(tc, source, at) }))
	}
	assertQualification(middle)
	assertQualification(last)
	assertQualification(last)
	assertQualification(newStart)

	// 以更新后的候选档金额核对旧账按原奖表，当前配置仍服务后续周期。
	fixture.BattlePass.Tiers = slices.Clone(fixture.BattlePass.Tiers)
	fixture.BattlePass.Tiers[0].Daily.Items = slices.Clone(fixture.BattlePass.Tiers[0].Daily.Items)
	fixture.BattlePass.Tiers[0].Daily.Items[0].Quantity = 1
	old := fixture.BattlePass.AnchorDay
	board, err := store.ReadBattlePassState(ctx, role, 15, old, "", newStart)
	must(err)
	if board.SeasonStart != old || board.SeasonEnd != shanghaiOperatingDay(newStart) || !board.Days[0].Qualified ||
		!board.Days[29].Qualified || board.Tiers[1].UpgradeCashFen != 0 || board.Tiers[0].Daily.Materials[0].Quantity != 2 || len(board.History) != 2 {
		t.Fatal("新周期覆盖旧日资格或原奖励", board)
	}
	state, err := store.ReadBattlePassState(ctx, role, 15, "", board.History[0], newStart)
	must(err)
	if len(state.History) != 1 || state.History[0] != old || !state.Days[0].Qualified || state.Days[29].Qualified {
		t.Fatal("历史分页或新周期资格错误", state)
	}

	op := &pb.OperationContext{OpId: domain.Bytes(domain.ID()), CommandSeq: uint64(role.EconomicSeq + 1),
		ExpectedRevision: uint64(role.Revision), ExpectedRuleVersion: fixture.RuleVersion}
	request := &pb.BattlePassClaimRequest{Operation: op, SeasonStart: old, Day: 30, Tier: 1}
	payload, err := proto.Marshal(request)
	must(err)
	asset := AssetRequest{Kind: AssetBattlePassClaim, Context: op, BattlePass: request, Fingerprint: Hash(payload)}
	next, result, first, err := store.AssetWithReceipt(ctx, auth, asset)
	must(err)
	if result.ErrorCode != 0 || first.BattlePass == nil || next.Wallet != role.Wallet || result.BattlePass.GetItems()[0].Quantity != 2 {
		t.Fatal("跨期漏领没有按原奖表补发", result)
	}
	again, retry, _, err := store.AssetWithReceipt(ctx, auth, asset)
	must(err)
	if again.Revision != next.Revision || retry.ErrorCode != 0 || !proto.Equal(retry.BattlePass, result.BattlePass) {
		t.Fatal("旧周期原领取回执重复发货或被新日替换")
	}
	board, err = store.ReadBattlePassState(ctx, next, 15, old, "", newStart)
	must(err)
	if board.Days[0].ClaimedTier != 0 || board.Days[29].ClaimedTier != 1 || result.BattlePass.GetTier() != 1 {
		t.Fatal("补领不应改变其他资格日期", board)
	}
}
