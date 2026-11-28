// 本文件通过真实任务、WebSocket 和支付原单对拍三档战令的绑定差额。
package integration

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/payment"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestBattlePassRewardsAndPaymentsMongoWS 真实完成每日任务，再用原渠道支付逐级升档和补发。
func TestBattlePassRewardsAndPaymentsMongoWS(t *testing.T) {
	trial := startBattlePassTrial(t, true)
	player := trial.create(t, "战令联机", time.Now().Add(time.Hour), false)
	c := trial.attach(t, &player, true)
	state := queryBattlePass(t, c, "")
	if state.Tier != 1 || state.Activity != 0 || len(state.History) != 0 || state.Tiers[2].UpgradeCashFen != 9800 {
		t.Fatal("新角色免费档或完整现金价错误", state)
	}
	day := currentBattlePassIndex(t, state)
	claimBattlePassWS(t, c, &pb.BattlePassClaimRequest{SeasonStart: state.SeasonStart, Day: day, Tier: 1}, p.CodeBattlePassUnavailable)
	state = qualifyBattlePass(t, trial, player, c)
	free := claimBattlePassWS(t, c, &pb.BattlePassClaimRequest{SeasonStart: state.SeasonStart, Day: day, Tier: 1}, 0)
	assertBattlePassMaterials(t, free.BattlePass, map[uint32]int64{900001: 2, 900041: 2, 900051: 5}, 0, 0)

	account, err := trial.store.Authenticate(trial.ctx, player.token)
	must(t, err)
	account.RoleID = domain.Hex(player.summary.Key.RoleId)
	second := &pb.RoleSummary{}
	rpc(t, trial.endpoint, player.token, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: trial.route.origin.OriginID,
		Slot: 2, Job: 0, Name: "战令同账" + domain.ID()[:4], OpId: domain.Bytes(domain.ID())}, second, 0)
	other := trial.attach(t, &worldBossRewardPlayer{token: player.token, summary: second}, false)
	readRole := func() domain.Role {
		t.Helper()
		r, err := trial.store.Role(trial.ctx, account.RoleID)
		must(t, err)
		return r
	}
	before := readRole() // 从此停止挂机，只核对后续升档和绑定资产的实际差额。
	const provider, merchant = "isolated-battle-pass", "isolated-cash-only"
	getQuote := func(tier uint32, cost int64) persistence.PaymentOrderQuote {
		t.Helper()
		quote, err := trial.store.QuoteBattlePassPayment(trial.ctx, account, state.SeasonStart, tier, provider, merchant)
		must(t, err)
		if quote.ProductID != tier || quote.AmountFen != cost || quote.Kind != payment.KindBattlePass || quote.YuanbaoUnits != 0 || quote.CardDays != 0 {
			t.Fatal("原现金升档应付与当前档位不符", quote)
		}
		return quote
	}
	create := func(quote persistence.PaymentOrderQuote, request string) persistence.PaymentOrder {
		t.Helper()
		order, err := trial.store.CreatePaymentOrder(trial.ctx, account, request, quote)
		must(t, err)
		return order
	}
	bind := func(order persistence.PaymentOrder) persistence.PaymentOrder {
		t.Helper()
		bound, err := trial.store.BindPaymentOrder(trial.ctx, order.ID, provider, merchant, domain.ID())
		must(t, err)
		return bound
	}
	quote := getQuote(2, 2800)
	// 两张候选单并发竞争原步骤，最多一张会进入渠道绑定。
	type createdOrder struct {
		order persistence.PaymentOrder
		err   error
	}
	results := make(chan createdOrder, 2)
	for _, offered := range []persistence.PaymentOrderQuote{quote, getQuote(3, 9800)} {
		go func() {
			order, err := trial.store.CreatePaymentOrder(trial.ctx, account, domain.ID(), offered)
			results <- createdOrder{order, err}
		}()
	}
	var winner persistence.PaymentOrder
	var rejected int
	for range 2 {
		result := <-results
		if result.err == nil {
			winner = result.order
		} else if persistence.Code(result.err) == p.CodeBattlePassPaymentPending {
			rejected++
		} else {
			t.Fatal("并发原单失败原因错误", result.err)
		}
	}
	if winner.ID == "" || rejected != 1 {
		t.Fatal("同一原档位步骤没有保留唯一可支付原单", winner.ID, rejected)
	}
	canceled := bind(winner)
	verifyBattlePassPayment(t, trial, canceled, payment.ProviderClosed, payment.ErrClosed, 0)
	verifyBattlePassPayment(t, trial, canceled, payment.ProviderPaid, payment.ErrOrderMismatch, 0)
	request := domain.ID()
	paid28 := bind(create(quote, request))
	if replay := create(quote, request); replay.ID != paid28.ID || replay.ExternalOrderID != paid28.ExternalOrderID {
		t.Fatal("原创建请求重试生成了不同付款单")
	}
	verifyBattlePassPayment(t, trial, paid28, payment.ProviderPaid, payment.ErrOrderMismatch, 1)
	verifier := verifyBattlePassPayment(t, trial, paid28, payment.ProviderPaid, nil, 0)
	waitBattlePassPayment(t, trial, paid28)
	checked, err := trial.store.ReconcilePaymentOrder(trial.ctx, verifier, paid28.ID)
	must(t, err)
	if checked.State != persistence.PaymentOrderFulfilled || checked.AssetOperationRef != "payment:"+paid28.ID {
		t.Fatal("重复核验产生了新的履约结果", checked.State)
	}
	state = queryBattlePass(t, c, "")
	if state.Tier != 2 || state.Tiers[2].UpgradeCashFen != 7000 || readRole().Wallet != before.Wallet {
		t.Fatal("现金 28 档未实际生效或增加了原钱包", state.Tier, state.Tiers[2], readRole().Wallet)
	}
	difference := claimBattlePassWS(t, c, &pb.BattlePassClaimRequest{SeasonStart: state.SeasonStart, Day: day, Tier: 2, ClaimedTier: 1}, 0)
	assertBattlePassMaterials(t, difference.BattlePass, map[uint32]int64{900001: 8, 900021: 1, 900041: 3, 900051: 5}, 3000, 1000)

	paid98 := bind(create(getQuote(3, 7000), domain.ID()))
	verifyBattlePassPayment(t, trial, paid98, payment.ProviderPaid, nil, 0)
	waitBattlePassPayment(t, trial, paid98)
	state = queryBattlePass(t, c, "")
	if state.Tier != 3 || state.Days[day-1].ClaimedTier != 2 || queryBattlePass(t, other, "").Tier != 1 {
		t.Fatal("原角色升档没保留已领水位，或同账号其他角色继承了付费档", state)
	}
	claim := &pb.BattlePassClaimRequest{SeasonStart: state.SeasonStart, Day: day, Tier: 3, ClaimedTier: 2}
	full := readRole()
	_, err = trial.store.DB.Collection("roles").UpdateByID(trial.ctx, account.RoleID, bson.M{"$set": bson.M{"capacity": len(full.Items)}})
	must(t, err)
	claimBattlePassWS(t, c, claim, p.CodeEquipmentBagFull)
	if readRole().Wallet != full.Wallet || queryBattlePass(t, c, "").Days[day-1].ClaimedTier != 2 {
		t.Fatal("满包导致部分发奖或推进原日水位")
	}
	_, err = trial.store.DB.Collection("roles").UpdateByID(trial.ctx, account.RoleID, bson.M{"$set": bson.M{"capacity": full.Capacity}})
	must(t, err)
	claim = proto.Clone(claim).(*pb.BattlePassClaimRequest)
	top := claimBattlePassWS(t, c, claim, 0)
	assertBattlePassMaterials(t, top.BattlePass, map[uint32]int64{900001: 30, 900021: 2, 900041: 15, 900051: 20}, 7000, 2000)
	previousBinding := c.binding
	c.close()
	c = trial.attach(t, &player, false)
	replay := c.asset(p.MsgC2SBattlePassClaim, claim)
	if c.binding == previousBinding || replay.ErrorCode != 0 || !proto.Equal(top.BattlePass, replay.BattlePass) {
		t.Fatal("新票未沿原请求取回首个实际差额")
	}
	once := &pb.BattlePassClaimRequest{SeasonStart: state.SeasonStart, Tier: 3}
	award := claimBattlePassWS(t, c, once, 0)
	assertBattlePassMaterials(t, award.BattlePass, map[uint32]int64{900031: 1}, 0, 0)
	claimBattlePassWS(t, c, proto.Clone(once).(*pb.BattlePassClaimRequest), p.CodeBattlePassChanged)
	got := readRole()
	want := before.Wallet
	want.BoundGold += 10000
	want.BoundYuanbao += 3000
	if got.Wallet != want {
		t.Fatal("原三档只累积到 98 档总奖，钱包仍有差异", got.Wallet, want)
	}
	count, err := trial.store.DB.Collection("audit_outbox").CountDocuments(trial.ctx, bson.M{"role_id": account.RoleID, "battle_pass": bson.M{"$exists": true}})
	must(t, err)
	if count != 4 || len(queryBattlePass(t, other, "").History) != 0 {
		t.Fatal("原重复、失败或其他角色使用了已确认的战令奖励", count)
	}
	count, err = trial.store.DB.Collection("audit_outbox").CountDocuments(trial.ctx,
		bson.M{"role_id": account.RoleID, "payment_grant.battle_pass": bson.M{"$exists": true}})
	must(t, err)
	if count != 2 {
		t.Fatal("已关闭、错款或重查的战令支付新增了履约", count)
	}
	t.Log("实际达成 100 日活跃，28 元和 70 元两张已核验原单，三档每日差额相加等于最高档，羽毛按周期只到账一次")
}

// startBattlePassTrial 从工作簿实包启动隔离节点，战斗缩短等待而仍使用正常结算和任务。
func startBattlePassTrial(t *testing.T, enabled bool) worldBossRewardTrial {
	t.Helper()
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) {
		if !enabled {
			return
		}
		path := os.Getenv("MINIMIR_BATTLE_PASS_TEST_FIXTURE")
		raw, err := os.ReadFile(path)
		must(t, err)
		var f devconfig.Fixture
		must(t, json.Unmarshal(raw, &f))
		must(t, f.Validate())
		if f.BattlePass == nil {
			t.Fatal("隔离配置缺少战令")
		}
		for i := range f.Monsters {
			m := &f.Monsters[i]
			m.HP, m.AttackMin, m.AttackMax, m.Defence, m.DefenceMin, m.DefenceMax = 1, 0, 0, 0, 0, 0
			m.MoveMS, m.Agility = 200, 0
		}
		f.Hash = persistence.Hash(raw)
		cfg.Fixture, cfg.FixtureFile = f, path
	})
	return worldBossRewardTrial{ctx: ctx, store: store, endpoint: endpoint, route: route}
}

// queryBattlePass 返回当前附着收到的权威目录。
func queryBattlePass(t *testing.T, c *client, start string) *pb.BattlePassState {
	t.Helper()
	id := c.send(p.MsgC2SBattlePassQuery, &pb.BattlePassQuery{SceneEpoch: c.entry.SceneEpoch, SeasonStart: start})
	frame := travelUntilWithin(c, 6*time.Second, func(f wire.Frame) bool { return f.Request == id })
	if frame.ID != p.MsgS2CBattlePassState || frame.Error != 0 {
		t.Fatal("战令目录拒绝原角色", frame.ID, frame.Error)
	}
	var state pb.BattlePassState
	must(t, proto.Unmarshal(frame.Payload, &state))
	return &state
}

// currentBattlePassIndex 将原服务端运营日映射到周期内日期。
func currentBattlePassIndex(t *testing.T, state *pb.BattlePassState) uint32 {
	t.Helper()
	start, err := time.Parse(time.DateOnly, state.SeasonStart)
	must(t, err)
	today, err := time.Parse(time.DateOnly, state.OperatingDay)
	must(t, err)
	return uint32(today.Sub(start)/(24*time.Hour)) + 1
}

// qualifyBattlePass 让实际普通击杀满足前六个日常任务并逐项线上领取。
func qualifyBattlePass(t *testing.T, trial worldBossRewardTrial, player worldBossRewardPlayer, c *client) *pb.BattlePassState {
	t.Helper()
	c.auto(true, 8)
	deadline := time.Now().Add(120 * time.Second)
	for {
		r, err := trial.store.Role(trial.ctx, domain.Hex(player.summary.Key.RoleId))
		must(t, err)
		if r.TasksCodex != nil && r.TasksCodex.Daily.Counters["2:6"].Count == 18 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("真实普通挂机未完成 18 次日常击杀", r.TasksCodex)
		}
		time.Sleep(200 * time.Millisecond)
	}
	c.auto(false, 8)
	tasks := queryWorldBossTaskProgress(t, c)
	if tasks.Activity != 0 || tasks.DailyClaimed != 0 {
		t.Fatal("战令等待实际日常领取增加活跃", tasks.Activity)
	}
	for id := uint32(1); id <= 6; id++ {
		var result *pb.OperationResult
		for attempt := 0; attempt < 3; attempt++ {
			result = c.asset(p.MsgC2STaskCodexClaim, &pb.TaskCodexClaimRequest{Operation: c.operation(), Kind: devconfig.TaskCodexDaily, Id: id, Day: tasks.Day})
			if result.ErrorCode != p.CodeOperationConflict {
				break
			}
			c.inventoryAt(result.NewRevision)
		}
		if result.ErrorCode != 0 {
			t.Fatal("实际日常任务领取失败", id, result)
		}
		c.inventoryAt(result.NewRevision)
	}
	state := queryBattlePass(t, c, "")
	if state.Activity != 100 || !state.Days[currentBattlePassIndex(t, state)-1].Qualified {
		t.Fatal("实际领取的日常活跃未取得战令运营日资格", state.Activity)
	}
	return state
}

// claimBattlePassWS 只提交玩家所见的原待领取条款，保留请求用来重放。
func claimBattlePassWS(t *testing.T, c *client, input *pb.BattlePassClaimRequest, code uint32) *pb.OperationResult {
	t.Helper()
	input.Operation = c.operation()
	result := c.asset(p.MsgC2SBattlePassClaim, input)
	if result.ErrorCode != code || result.Pending || (result.BattlePass != nil) != (code == 0) {
		t.Fatal("战令领取结果与原资格不符", code, result)
	}
	c.inventoryAt(result.NewRevision)
	return result
}

// assertBattlePassMaterials 对拍每个绑定模板和实际两种绑定币种的到账。
func assertBattlePassMaterials(t *testing.T, receipt *pb.BattlePassReceipt, want map[uint32]int64, gold, yuanbao int64) {
	t.Helper()
	got := make(map[uint32]int64)
	for _, item := range receipt.GetItems() {
		if !item.Bound || item.SourceKind != "battle_pass" || len(item.InstanceId) != 16 {
			t.Fatal("战令资产没有绑定至原来源", item)
		}
		got[item.ItemId] += item.Quantity
	}
	if !maps.Equal(got, want) || receipt.BoundGold != gold || receipt.BoundYuanbaoUnits != yuanbao {
		t.Fatal("档位差额与工作簿不一致", got, receipt)
	}
}

// verifyBattlePassPayment 只在隔离测试使用模拟渠道，通过真实主动查单校验金额与原订单。
func verifyBattlePassPayment(t *testing.T, trial worldBossRewardTrial, order persistence.PaymentOrder, state payment.ProviderState, want error, extraFen int64) *payment.Verifier {
	t.Helper()
	provider := paymentTestProvider{actual: payment.ProviderOrder{OrderID: order.ID, MerchantID: order.Quote.MerchantID,
		ExternalOrderID: order.ExternalOrderID, PurchaseKind: order.Quote.Kind, ProductID: order.Quote.ProductID,
		AmountFen: order.Quote.AmountFen + extraFen, Currency: payment.CurrencyCNY, BeneficiaryReference: order.Expected().BeneficiaryReference(),
		State: state, TransactionID: domain.ID(), PaidAt: time.Now()}}
	verifier, err := payment.NewVerifier(order.Quote.ProviderID, order.Quote.MerchantID, provider)
	must(t, err)
	_, err = trial.store.ReconcilePaymentOrder(trial.ctx, verifier, order.ID)
	if !errors.Is(err, want) {
		t.Fatal("模拟渠道原单核验结果错误", err, want)
	}
	return verifier
}

// waitBattlePassPayment 等实际游戏节点后台履约，并核对原操作账本没有新增支付赠币。
func waitBattlePassPayment(t *testing.T, trial worldBossRewardTrial, original persistence.PaymentOrder) {
	t.Helper()
	deadline := time.Now().Add(28 * time.Second)
	for {
		order, err := trial.store.GetPaymentOrder(trial.ctx, original.ID)
		must(t, err)
		if order.State == persistence.PaymentOrderFulfilled {
			var receipt persistence.Operation
			must(t, trial.store.DB.Collection("operations").FindOne(trial.ctx, bson.M{"_id": order.AssetOperationRef}).Decode(&receipt))
			grant := receipt.PaymentGrant
			if order.BattlePassReserved || order.AssetOperationRef != "payment:"+original.ID || grant == nil || grant.BattlePass == nil ||
				grant.BattlePass.Terms != original.Quote.BattlePass || grant.BattlePass.Tier != original.Quote.ProductID ||
				grant.YuanbaoAdded != 0 || grant.WalletBefore != grant.WalletAfter {
				t.Fatal("现金履约的原角色周期、到账回执或钱包错误", grant)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("实际游戏节点未完成战令原单履约", order.State)
		}
		time.Sleep(40 * time.Millisecond)
	}
}
