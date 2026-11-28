// 本文件实测普通打怪先赚工具费、离线挖矿和返场续战，沿正常入口验证两层矿区的现金流。
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const miningBattleMonsterSlots = uint32(4) // 共同使用原 4 怪设置，便于比较切图前后的普通战斗。

// miningBattleTrial 保存同一隔离运行服和已装配的真实候选材料。
type miningBattleTrial struct {
	ctx       context.Context    // 节点生存期与只读对账的上下文。
	store     *persistence.Store // 原角色、来源和审计的读取句柄。
	endpoint  string             // 真实账号服入场 RPC。
	route     directoryTestEntry // 放置八个样本角色的同一原区服路由。
	loginGate chan struct{}      // 将建立样本账号的同时注册数控制在现有口令预算内。
	shopGate  chan struct{}      // 共享城镇错开实体走位及报价。
}

// miningBattlePlayer 保存一次隔离建角后的签票身份和当前真实连接。
type miningBattlePlayer struct {
	trial *miningBattleTrial // 所有样本共用的服务端。
	token string             // 只用于测试签发正常区服票据，不写入采样报告。
	key   *pb.RoleKey        // 原建角角色及永久入口服。
	c     *client            // 当前 WebSocket，进矿后会断线并重开。
}

// miningBattleScenario 区分层、特权卡和托管卡的四种组合。
type miningBattleScenario struct {
	Layer     uint32 `json:"layer"`     // 使用原编号的一层或二层。
	Privilege bool   `json:"privilege"` // 活跃特权，决定产出绑定和交易资格。
	Hosting   bool   `json:"hosting"`   // 活跃托管卡，只放大原普通击杀的直接收益。
}

// TestMiningBattleEconomyMongoWS 用原三秒、30% 出矿和原试用工具完整执行两层四卡态组合。
func TestMiningBattleEconomyMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的专属 MongoDB 副本集")
	}
	raw, err := os.ReadFile(os.Getenv("MINIMIR_MINING_TEST_FIXTURE"))
	must(t, err)
	var fixture devconfig.Fixture
	must(t, json.Unmarshal(raw, &fixture))
	fixture.Hash = persistence.Hash(raw)
	must(t, fixture.Validate())
	if fixture.Mining == nil || fixture.Mining.AutoRecycle == nil || fixture.Mining.SuccessPPM != 300_000 ||
		fixture.Mining.IntervalMS != 3000 || len(fixture.Mining.Layers) != 2 || len(fixture.Mining.Tools) != 2 || fixture.ItemMarket == nil {
		t.Fatal("矿战采样需要原两层、三秒、30% 出矿、回收和市场规则")
	}
	ctx, store, endpoint, route := startChallengePlayerDeathServer(t, func(cfg *devconfig.Config) { cfg.Fixture = fixture })
	trial := miningBattleTrial{ctx: ctx, store: store, endpoint: endpoint, route: route,
		loginGate: make(chan struct{}, 2), shopGate: make(chan struct{}, 1)}
	reports := make([]miningBattleReport, 8)
	players := make([]miningBattlePlayer, len(reports))
	if !t.Run("角色分组", func(t *testing.T) {
		for i := range reports {
			scenario := miningBattleScenario{Layer: uint32(i/4 + 1), Privilege: i&1 != 0, Hosting: i&2 != 0}
			t.Run(fmt.Sprintf("%d层_特权%t_托管%t", scenario.Layer, scenario.Privilege, scenario.Hosting), func(t *testing.T) {
				t.Parallel()
				reports[i], players[i] = trial.run(t, scenario)
			})
		}
	}) {
		return
	}
	market := trial.tradeOre(t, players, reports)
	if directory := os.Getenv("MINIMIR_MINING_ECONOMY_REPORT_DIR"); directory != "" {
		result, err := json.MarshalIndent(map[string]any{"fixture_sha256": fixture.Hash, "roles": reports, "market": market}, "", "  ")
		must(t, err)
		must(t, os.WriteFile(filepath.Join(directory, "mining-battle-economy.json"), append(result, '\n'), 0o644))
	}
}

// run 从同一真实角色采集资金不足、战斗筹资、买工具、在线及掉线开采、卖矿和回到普通图。
func (trial *miningBattleTrial) run(t *testing.T, scenario miningBattleScenario) (miningBattleReport, miningBattlePlayer) {
	t.Helper()
	fixture := &trial.store.Cfg.Fixture
	layer, tool := fixture.Mining.Layers[scenario.Layer-1], fixture.Mining.Tools[scenario.Layer-1]
	town, npc, offer := miningVendor(t, fixture, tool.ItemID)
	player := trial.newPlayer(t, layer.MinimumLevel, scenario, 0)
	c := player.c
	keepOpen := sampleHeartbeat(c, time.Now().Add(3*time.Minute))
	defer keepOpen()
	for _, itemID := range []uint32{218, 193} {
		equipPersonalBossAutoFixture(t, c, fixture, itemID)
	}
	// 锁定未穿戴的赠品，筹资仅计原战斗直接金币与实际新掉落的回收。
	spare := naturalGrowthItem(c, 218, "")
	if spare == nil {
		t.Fatal("未找到原新角备用木剑")
	}
	lock := roleCardAssetWS(c, p.MsgC2SSetItemLock, &pb.SetItemLockRequest{ItemInstanceId: spare.InstanceId, ItemVersion: spare.Version, Locked: true, Operation: c.operation()})
	if lock.ErrorCode != 0 {
		t.Fatal("隔离赠品未确认保护", lock)
	}
	policy := &pb.AutoRecyclePolicy{Enabled: true, QualityIds: []uint32{0, 1}}
	recycle := roleCardAssetWS(c, p.MsgC2SSetAutoRecycle, &pb.SetAutoRecycleRequest{Policy: policy, Operation: c.operation()})
	if recycle.ErrorCode != 0 {
		t.Fatal("未通过正常角色操作启用普通及精良装备回收", recycle)
	}
	claim := roleCardAssetWS(c, p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: c.operation()})
	if claim.ErrorCode != 0 || claim.Pending {
		t.Fatal("未通过正常日领取得托管秒数", claim)
	}
	start := player.role(t)
	dailySeconds := fixture.HostingBenefits.FreeDailySeconds
	if scenario.Hosting {
		dailySeconds = fixture.HostingBenefits.CardDailySeconds
	}
	if start.Wallet != (domain.Wallet{}) || start.HostingSeconds != dailySeconds ||
		start.HostingSeconds <= tool.DurabilityU*fixture.Mining.IntervalMS/1000 {
		t.Fatal("初态应无金币和剩余购卡款，且有可验证的日领时长", start.Wallet, start.HostingSeconds)
	}
	trial.atShop(c, town, npc, func() {
		q := &pb.VendorQuoteRequest{SceneEpoch: c.entry.SceneEpoch, NpcId: npc.ID, Action: 1, OfferId: offer.ID, Quantity: 1}
		travelExpectError(c, p.MsgC2SVendorQuote, c.send(p.MsgC2SVendorQuote, q), p.CodeVendorFunds)
	})
	travelToBaseline(c, 3, layer.Map.ID)
	travelAcknowledge(c, false)
	if value := miningStateWS(c, 1, nil); value.Active || value.ErrorCode != p.CodeEquipmentChanged || value.Attempts != 0 {
		t.Fatal("金币不足时未购入工具，矿层却创建了判定", value)
	}
	blocked := player.role(t)
	if blocked.Wallet != start.Wallet || blocked.LastMiningAt != start.LastMiningAt || blocked.HostingSeconds != start.HostingSeconds ||
		!slices.Equal(blocked.Items, start.Items) {
		t.Fatal("购锄失败改变了钱包、时长、原矿时钟或库存", player.key)
	}
	returnMiningBattle(c, fixture.Map.ID)
	beforeBattle := player.role(t)
	afterBattle, firstWindow := player.fight(t, offer.PriceUnits)
	battle := player.battleLedger(t, beforeBattle, afterBattle, scenario, firstWindow)
	if battle.Kills == 0 || afterBattle.Wallet.Gold+afterBattle.Wallet.BoundGold < offer.PriceUnits {
		t.Fatal("未通过普通战斗实获首把工具所需资金", battle)
	}
	var paid miningBattleVendor
	var owned domain.Item
	trial.atShop(c, town, npc, func() {
		before := player.role(t)
		paid = player.vendor(t, npc.ID, &pb.VendorQuoteRequest{Action: 1, OfferId: offer.ID, Quantity: 1})
		wantBound := min(before.Wallet.BoundGold, offer.PriceUnits)
		if paid.Wallet != (domain.Wallet{Gold: offer.PriceUnits - wantBound, BoundGold: wantBound}) || len(paid.Items) != 1 {
			t.Fatal("首把工具实际付款与配置报价、绑金优先或数量不符", paid)
		}
		owned = paid.Items[0]
		if owned.ItemID != tool.ItemID || owned.DurabilityU != tool.DurabilityU || owned.SourceKind != "npc_shop" {
			t.Fatal("首把锄头未按原规格真实买入", owned)
		}
	})
	travelToBaseline(c, 3, layer.Map.ID)
	travelAcknowledge(c, false)
	if c.entry.SceneKind != devconfig.SceneKindMining || travelCount(c.scene, 3) != 0 {
		t.Fatal("矿区不能继续普通战斗", c.entry)
	}
	if !scenario.Privilege {
		// 无特权的两组比较自动回收，持特权的两组留原实物用于 NPC 出售及市场。
		res := roleCardAssetWS(c, p.MsgC2SSetMiningAutoRecycle, &pb.SetMiningAutoRecycleRequest{Enabled: true, Operation: c.operation()})
		if res.ErrorCode != 0 || res.Pending {
			t.Fatal("无法沿正常操作开启矿石回收", res)
		}
	}
	preMine := player.role(t)
	if preMine.Wallet != minusMiningBattleWallet(afterBattle.Wallet, paid.Wallet) ||
		preMine.HostingSeconds != afterBattle.HostingSeconds || preMine.Experience != afterBattle.Experience {
		t.Fatal("走路、换图和回收设置改变了工具费用之外的账", preMine.Wallet, paid)
	}
	scene, binding := c.entry.SceneEpoch, c.binding
	if started := miningStateWS(c, 1, domain.Bytes(owned.ID)); !started.Active || started.AutoRecycleEnabled == scenario.Privilege || started.Tool.GetDurabilityU() != uint64(tool.DurabilityU) {
		t.Fatal("实际买入的工具或原设置未打开开采", started)
	}
	travelUntilWithin(c, 12*time.Second, func(frame wire.Frame) bool {
		if frame.ID != p.MsgS2CMiningState || frame.Request != 0 {
			return false
		}
		var state pb.MiningState
		must(t, proto.Unmarshal(frame.Payload, &state))
		return state.Attempts == 1
	})
	keepOpen()
	c.close()
	deadline := time.Now().Add(time.Duration(tool.DurabilityU/fixture.Mining.ToolWearU*fixture.Mining.IntervalMS)*time.Millisecond + 25*time.Second)
	postMine := player.role(t)
	for postMine.HasItemInstance(owned.ID) {
		if time.Now().After(deadline) {
			t.Fatal("在线一轮之后没有完整托管至原工具耗尽", postMine.LastMiningAt, postMine.HostingSeconds)
		}
		time.Sleep(120 * time.Millisecond)
		postMine = player.role(t)
	}
	c = player.attach(t)
	keepReconnected := sampleHeartbeat(c, time.Now().Add(3*time.Minute))
	defer keepReconnected()
	state := miningStateWS(c, 0, nil)
	if c.entry.RoomId != layer.Map.ID || c.entry.SceneEpoch != scene || c.binding == binding || state.Active || state.Tool != nil {
		t.Fatal("健康重连未接回原矿层和原锄头耗尽终态", c.entry, state)
	}
	mine := player.mineLedger(t, preMine, postMine, scenario, layer, owned)
	if state.Attempts != uint64(len(mine.Attempts)) || state.Successes != uint64(mine.Successes) {
		t.Fatal("原矿况和持久判定或实出矿数量不一致", state, mine.Successes)
	}
	var sold []miningBattleVendor
	if scenario.Privilege && len(mine.Retained) > 1 {
		// 留第一块原矿供市场，其他符合原收购条件的第一块供杂货商实测。
		for _, ore := range mine.Retained[1:] {
			price, allowed := fixture.MiningAutoRecyclePrice(ore.ItemID)
			amount := miningBattleOreGold(price, ore)
			if !allowed || amount == (domain.Wallet{}) {
				continue
			}
			trial.atShop(c, town, npc, func() {
				receipt := player.vendor(t, npc.ID, &pb.VendorQuoteRequest{Action: 2, InstanceId: domain.Bytes(ore.ID), ItemVersion: uint64(ore.Version), Quantity: 1})
				if receipt.Wallet != amount || receipt.Sold == nil || !proto.Equal(receipt.Sold.Proto(), ore.Proto()) {
					t.Fatal("已实采原矿未按单块纯度及绑定进入 NPC 钱包", receipt, ore)
				}
				sold = append(sold, receipt)
			})
			break
		}
	}
	returnMiningBattle(c, fixture.Map.ID)
	resume := player.role(t)
	finish, secondWindow := player.fight(t, 0)
	resumeBattle := player.battleLedger(t, resume, finish, scenario, secondWindow)
	if resumeBattle.Kills == 0 || !mine.Attempts[0].Fact.StartedAt.After(firstWindow.Stop) || !secondWindow.Start.After(postMine.LastMiningAt) {
		t.Fatal("原普通地图没有恢复正常击杀", resumeBattle)
	}
	expected := minusMiningBattleWallet(addMiningBattleWallet(addMiningBattleWallet(battle.Total(), resumeBattle.Total()), mine.AutoGold), paid.Wallet)
	for _, receipt := range sold {
		expected = addMiningBattleWallet(expected, receipt.Wallet)
	}
	if finish.Wallet != expected || finish.HostingSeconds != start.HostingSeconds-mine.HostSeconds ||
		finish.LastMiningAt != postMine.LastMiningAt {
		t.Fatal("普通战斗、原工具、矿石及普通返场后的四钱包和托管时长未对平", finish.Wallet, expected)
	}
	t.Logf("判定 %d 次，实采 %d 块，离线扣 %d 秒，普通两段 %d 杀，实际购锄扣 %d 金币", len(mine.Attempts), mine.Successes, mine.HostSeconds, battle.Kills+resumeBattle.Kills, offer.PriceUnits)
	return miningBattleReport{RoleID: finish.ID, Scenario: scenario, Opening: start.Wallet, FirstBattle: battle, Tool: paid,
		Mining: mine, NPCSales: sold, SecondBattle: resumeBattle, Closing: finish.Wallet,
		HostingBefore: start.HostingSeconds, HostingAfter: finish.HostingSeconds}, player
}

// newPlayer 只准备矿层等级、初次购卡的来源资金和单独市场买家的本金。
func (trial *miningBattleTrial) newPlayer(t *testing.T, level uint32, scenario miningBattleScenario, buyerFunds int64) miningBattlePlayer {
	t.Helper()
	var products []devconfig.RoleCardProduct
	yuanbao := buyerFunds
	for _, product := range trial.store.Cfg.Fixture.RoleCards.Products {
		if scenario.Privilege && product.Kind == devconfig.RoleCardKindPrivilege || scenario.Hosting && product.Kind == devconfig.RoleCardKindHosting {
			products = append(products, product)
			yuanbao += product.PriceYuanbaoUnits
		}
	}
	account := &pb.LoginResult{}
	func() {
		trial.loginGate <- struct{}{}
		defer func() { <-trial.loginGate }()
		rpc(t, trial.endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "mineecon_" + domain.ID()[:10], Password: domain.ID()[:16]}, account, 0)
	}()
	summary := createDirectoryRole(t, trial.endpoint, account.AccountToken, trial.route.origin.OriginID, "矿战")
	player := miningBattlePlayer{trial: trial, token: account.AccountToken, key: summary.Key}
	initial, err := trial.store.DB.Collection("roles").UpdateByID(trial.ctx, domain.Hex(summary.Key.RoleId), bson.M{"$set": bson.M{"level": level, "wallet.yuanbao_units": yuanbao}})
	must(t, err)
	if initial.MatchedCount != 1 {
		t.Fatal("隔离角色初始态未保存", summary.Name)
	}
	c := player.attach(t)
	c.auto(false, miningBattleMonsterSlots)
	for _, product := range products {
		quote := roleCardStateWS(c, product.ID).GetQuote()
		if len(quote.GetQuoteId()) != 16 || quote.GetProduct().GetPriceYuanbaoUnits() != product.PriceYuanbaoUnits {
			t.Fatal("无法获得原角色月卡报价", product.ID, quote)
		}
		bought := roleCardAssetWS(c, p.MsgC2SBuyRoleCard, &pb.BuyRoleCardRequest{QuoteId: quote.QuoteId, ProductId: product.ID, Operation: c.operation()})
		if bought.ErrorCode != 0 || !proto.Equal(bought.GetRoleCardPurchase().GetQuote(), quote) {
			t.Fatal("原卡未通过正式游戏内流程到账", bought)
		}
	}
	return player
}

// attach 从同一账号、入口服与角色签票，延长测试客户端本地读取期限。
func (player *miningBattlePlayer) attach(t *testing.T) *client {
	t.Helper()
	ticket := directoryTicket(t, player.trial.endpoint, player.token, player.key, player.trial.route)
	c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
	c.stop()
	c.ctx, c.stop = context.WithTimeout(player.trial.ctx, 4*time.Minute)
	t.Cleanup(c.close)
	player.c = c
	return c
}

// role 从 MongoDB 读取已确认资产，客户端浮字和预览不计入账本。
func (player miningBattlePlayer) role(t *testing.T) domain.Role {
	t.Helper()
	role, err := player.trial.store.Role(player.trial.ctx, domain.Hex(player.key.RoleId))
	must(t, err)
	return role
}

// fight 在角色原普通地图自动战斗，按确认的工具资金或首杀停止并等耐久来源收束。
func (player miningBattlePlayer) fight(t *testing.T, price int64) (domain.Role, miningBattleWindow) {
	t.Helper()
	c := player.c
	started := time.Now().UTC()
	c.auto(true, miningBattleMonsterSlots)
	limit := 30 * time.Second
	if price > 0 {
		limit = 150 * time.Second // 本次筹资只随真实掉落和回收继续，不补测试金币。
	}
	deadline := time.Now().Add(limit)
	travelUntilWithin(c, limit+8*time.Second, func(frame wire.Frame) bool {
		if time.Now().After(deadline) {
			return true // 先主动停战，再统一查看此段实际结算。
		}
		if frame.ID != p.MsgS2CExperienceGrantResult {
			return false
		}
		if price == 0 {
			return true
		}
		wallet := player.role(t).Wallet
		return wallet.Gold+wallet.BoundGold >= price
	})
	c.auto(false, miningBattleMonsterSlots)
	window := miningBattleWindow{Start: started, Stop: time.Now().UTC()}
	last := player.role(t)
	still, timeout := time.Now(), time.Now().Add(12*time.Second)
	for time.Since(still) < 4*time.Second {
		time.Sleep(180 * time.Millisecond)
		if next := player.role(t); next.Revision != last.Revision {
			last, still = next, time.Now()
		}
		if time.Now().After(timeout) {
			t.Fatal("停止战斗后在途收益与磨损未收束", last.SourceSeq)
		}
	}
	return last, window
}

// atShop 顺序复用共享城镇的相邻格和原商人入口，报完价后立即离开操作临界段。
func (trial *miningBattleTrial) atShop(c *client, town *devconfig.MapTemplate, npc devconfig.TownNPC, trade func()) {
	c.t.Helper()
	trial.shopGate <- struct{}{}
	defer func() { <-trial.shopGate }()
	reachTownVendor(c, town, npc)
	trade()
}

// returnMiningBattle 只从普通地图目录返回，保持原怪槽配置和完整入场确认。
func returnMiningBattle(c *client, mapID uint64) {
	c.t.Helper()
	travelToBaseline(c, 2, mapID)
	travelAcknowledge(c, false)
	if c.entry.SceneKind != devconfig.SceneKindPersonal || travelCount(c.scene, 3) != int(miningBattleMonsterSlots) {
		c.t.Fatal("返回时没有恢复原普通地图及四个怪槽", c.entry)
	}
}
