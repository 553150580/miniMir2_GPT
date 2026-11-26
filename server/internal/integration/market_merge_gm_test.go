// 本文件通过真实 GM HTTP、账号目录、游戏 WebSocket 和隔离 MongoDB 校验合服前清退。
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/transport"
)

const mergeTestPath = "/api/maintenance/market-merge/" // 本机 GM 独占的真实运维路由。

// mergeTestRole 保留已通过账号创建的原角色和重连所需身份。
type mergeTestRole struct {
	key   *pb.RoleKey
	token string
	route directoryTestEntry
}

// TestItemMarketMergeGMMongoWS 在不同游戏节点用常规市场入口产生债务，经清退后原路领取。
func TestItemMarketMergeGMMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要独立 MongoDB 副本集控制器装配的回环端点")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_merge_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	cfg.OriginID, cfg.RunID, cfg.NodeID = 101, 11, "market-merge-source"
	cfg.RunServerIDs = []uint64{11}
	target := cfg
	target.GameAddress = address(t)
	target.OriginID, target.RunID, target.NodeID = 201, 21, "market-merge-target"
	target.RunServerIDs = []uint64{21}
	sourceRoute := makeDirectoryEntry(101, 11, cfg, "merge", "清退测试", "源服", 0, 0)
	targetRoute := makeDirectoryEntry(201, 21, target, "merge", "清退测试", "目标服", 0, 1)
	cfg.OriginRoutes = append(cfg.OriginRoutes[:0:0], sourceRoute.origin, targetRoute.origin)
	cfg.RunServers = append(cfg.RunServers[:0:0], sourceRoute.assignment, targetRoute.assignment)
	configureGoldMarketWS(t, &cfg)
	target.OriginRoutes, target.RunServers, target.Fixture = cfg.OriginRoutes, cfg.RunServers, cfg.Fixture
	ctx := t.Context()
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer store.Close(cleanup)
		must(t, store.DB.Drop(cleanup))
	})
	gm, key := startMergeTestGM(t, store)
	startDurableWorldBossNode(t, cfg, "account", cfg.AccountAddress)
	stopSource := startDurableWorldBossNode(t, cfg, "game", cfg.GameAddress)
	stopTarget := startDurableWorldBossNode(t, target, "game", target.GameAddress)

	account := "http://" + cfg.AccountAddress + "/rpc"
	item := domain.Item{ID: domain.ID(), ItemID: 35, Quantity: 2, Version: 1, SourceKind: "monster_drop", SourceID: domain.ID()}
	source := seedMergeTestRole(t, store, account, sourceRoute)
	destination := seedMergeTestRole(t, store, account, targetRoute, item)
	attach := func(role mergeTestRole) *client {
		ticket := directoryTicket(t, account, role.token, role.key, role.route)
		connected := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		t.Cleanup(connected.close)
		connected.auto(false, 1)
		return connected
	}
	sourceClient, targetClient := attach(source), attach(destination)
	terms := goldStateWS(sourceClient, 2).Rules
	if terms == nil {
		t.Fatal("隔离测试的源运行服没有开启金币报价")
	}
	sellRequest := goldTermsForTest(terms, persistence.GoldMarketSell, 2, 101)
	sell := goldActionWS(sourceClient, sellRequest, 0)
	if sell.GoldDelta != -20_000 {
		t.Fatal("清退前未从源角色真实冻结非绑金币", sell)
	}
	itemView := item.Proto()
	listing := marketActionWS(targetClient, &pb.ItemMarketAction{Action: marketListAction, ItemInstanceId: itemView.InstanceId,
		ItemVersion: itemView.Version, Quantity: item.Quantity, UnitPriceYuanbaoUnits: 11}, 0)
	if listing.Quantity != item.Quantity {
		t.Fatal("清退前未冻结目标服原物品", listing)
	}
	oldTicket := directoryTicket(t, account, source.token, source.key, source.route)
	planID, requestID := domain.ID(), domain.ID()
	planQuery := "?plan_id=" + planID
	prepare := map[string]any{"plan_id": planID, "source_run_id": cfg.RunID, "target_run_id": target.RunID}
	call := func(method, route string, input any, status int, result any) []byte {
		t.Helper()
		return mergeTestHTTP(t, gm, key, method, route, input, nil, status, result)
	}
	// 运维写入口只存在于 GM，且错误身份、浏览器来源、预检和歧义 JSON 都不触发清退。
	for _, other := range []string{"http://" + cfg.AccountAddress, "http://" + cfg.GameAddress} {
		mergeTestHTTP(t, other, key, http.MethodPost, "prepare", prepare, nil, http.StatusNotFound, nil)
	}
	mergeTestHTTP(t, gm, "", http.MethodPost, "prepare", prepare, nil, http.StatusUnauthorized, nil)
	mergeTestHTTP(t, gm, key, http.MethodPost, "prepare", prepare, map[string]string{"Origin": gm}, http.StatusForbidden, nil)
	mergeTestHTTP(t, gm, key, http.MethodOptions, "prepare", nil, nil, http.StatusMethodNotAllowed, nil)
	call(http.MethodPost, "prepare", json.RawMessage(`{"plan_id":"`+planID+`","source_run_id":11,"source_run_id":21,"target_run_id":21}`), http.StatusBadRequest, nil)
	call(http.MethodGet, "status"+planQuery, nil, http.StatusConflict, nil)

	var plan persistence.MarketMergePlan
	call(http.MethodPost, "prepare", prepare, http.StatusOK, &plan)
	if plan.ID != planID || plan.Version != 1 {
		t.Fatal("计划未以原身份和首版受理", plan)
	}
	call(http.MethodGet, "route-release-proof"+planQuery+"&expected_version=1", nil, http.StatusConflict, nil)
	versioned := func(version int64) map[string]any {
		return map[string]any{"plan_id": planID, "expected_version": version}
	}
	freeze := versioned(plan.Version)
	call(http.MethodPost, "freeze", freeze, http.StatusOK, &plan)
	if plan.Version != 2 {
		t.Fatal("冻结没有确认单次相变", plan)
	}
	frozen := call(http.MethodGet, "status"+planQuery, nil, http.StatusOK, &plan)
	if replay := call(http.MethodPost, "freeze", freeze, http.StatusOK, nil); !bytes.Equal(frozen, replay) {
		t.Fatal("原冻结版本重试改变了冻结回执")
	}
	for _, role := range []mergeTestRole{source, destination} {
		rpc(t, account, role.token, p.MsgC2SEntryTicket, &pb.EntryTicketRequest{Role: role.key}, &pb.EntryTicketResult{}, p.CodeLineRecovering)
	}
	verifyDirectoryTicketRejected(t, source.route.assignment.GameWSURL, oldTicket.Ticket)
	if goldStateWS(sourceClient, 2).CanTrade || marketStateWS(targetClient, 1, 0).CanTrade {
		t.Fatal("已连接角色在冻结后仍获得交易权限")
	}
	if replay := goldActionWS(sourceClient, sellRequest, 0); !bytes.Equal(replay.OrderId, sell.OrderId) || replay.GoldDelta != sell.GoldDelta {
		t.Fatal("冻结改变了已成功挂单的原回执", sell, replay)
	}
	blocked := goldTermsForTest(terms, persistence.GoldMarketSell, 1, 101)
	before := sourceClient.watermark
	goldActionWS(sourceClient, blocked, p.CodeLineRecovering)
	if sourceClient.watermark != before {
		t.Fatal("冻结后拒绝的新命令占用了经济流水", before, sourceClient.watermark)
	}
	// 活租约尚在时拒绝清退，随后由真实游戏进程正常收尾释放租约。
	stepInput := versioned(plan.Version)
	stepInput["request_id"] = requestID
	call(http.MethodPost, "step", stepInput, http.StatusConflict, nil)
	sourceClient.close()
	targetClient.close()
	stopSource()
	stopTarget()
	call(http.MethodPost, "seal", versioned(plan.Version), http.StatusConflict, nil)

	confirmed := map[string]persistence.MarketMergeAssetStep{}
	for attempt := 0; ; attempt++ {
		if attempt >= 5 {
			t.Fatal("两个真实挂单未在有限步骤中排空", confirmed)
		}
		var step persistence.MarketMergeStepReceipt
		raw := call(http.MethodPost, "step", stepInput, http.StatusOK, &step)
		if step.PlanID != planID || step.RequestID != stepInput["request_id"] || step.Version != plan.Version+1 {
			t.Fatal("单步回执缺少原清退身份或新版本", step)
		}
		if replay := call(http.MethodPost, "step", stepInput, http.StatusOK, nil); !bytes.Equal(replay, raw) {
			t.Fatal("原单步请求重试没有返回相同回执")
		}
		stale := versioned(plan.Version)
		stale["request_id"] = domain.ID()
		call(http.MethodPost, "step", stale, http.StatusConflict, nil)
		call(http.MethodGet, "status"+planQuery, nil, http.StatusOK, &plan)
		if plan.Version != step.Version {
			t.Fatal("状态与原单步回执的版本不一致", plan, step)
		}
		if step.Done {
			break
		}
		if step.Asset == nil || step.Asset.ClaimID == "" || confirmed[step.Asset.Kind].SourceID != "" {
			t.Fatal("一次清退改变了资源数量或未返还原托管项", step)
		}
		confirmed[step.Asset.Kind] = *step.Asset
		stepInput = versioned(step.Version)
		stepInput["request_id"] = domain.ID()
	}
	if gold, goods := confirmed["gold_order"], confirmed["item_order"]; len(confirmed) != 2 ||
		gold.RunID != cfg.RunID || gold.SourceID != domain.Hex(sell.OrderId) ||
		goods.RunID != target.RunID || goods.SourceID != domain.Hex(listing.OrderId) {
		t.Fatal("清退回执与原两服订单无法核对", confirmed)
	}
	call(http.MethodPost, "seal", versioned(plan.Version), http.StatusOK, &plan)
	proofQuery := "route-release-proof" + planQuery + "&expected_version=" + strconv.FormatInt(plan.Version, 10)
	sealed := call(http.MethodGet, proofQuery, nil, http.StatusOK, nil)
	var proof map[string]json.RawMessage
	must(t, json.Unmarshal(sealed, &proof))
	if value := string(proof["proof"]); value == "" || value == "null" || value == "\"\"" || value == "{}" {
		t.Fatal("封账后的放行响应没有可验证证明", string(sealed))
	}
	if !bytes.Equal(sealed, call(http.MethodGet, "status"+planQuery, nil, http.StatusOK, nil)) {
		t.Fatal("路由放行只读验证改变了计划或返回了不同结果")
	}
	// 没有发布合服的 HTTP 路径，验证证明不会移动原入口或允许签新票。
	call(http.MethodPost, "apply", versioned(plan.Version), http.StatusNotFound, nil)
	for _, role := range []mergeTestRole{source, destination} {
		rpc(t, account, role.token, p.MsgC2SEntryTicket, &pb.EntryTicketRequest{Role: role.key}, &pb.EntryTicketResult{}, p.CodeLineRecovering)
	}
	call(http.MethodPost, "restore", versioned(plan.Version), http.StatusOK, &plan)
	call(http.MethodGet, proofQuery, nil, http.StatusConflict, nil)
	startDurableWorldBossNode(t, cfg, "game", cfg.GameAddress)
	startDurableWorldBossNode(t, target, "game", target.GameAddress)
	verifyDirectoryTicketRejected(t, source.route.assignment.GameWSURL, oldTicket.Ticket)
	recovered := directoryTicket(t, account, source.token, source.key, source.route)
	if recovered.RouteVersion != oldTicket.RouteVersion {
		t.Fatal("恢复原路由后不应改变永久入口路由版本", oldTicket.RouteVersion, recovered.RouteVersion)
	}
	sourceClient, targetClient = attach(source), attach(destination)
	gold := goldEscrowsWS(sourceClient, sell.OrderId, persistence.GoldMarketClaimGoldReturn, 1)[0]
	goods := marketEscrowWS(targetClient, listing.OrderId, persistence.ItemMarketClaimItem)
	if domain.Hex(gold.EscrowId) != confirmed["gold_order"].ClaimID || domain.Hex(goods.EscrowId) != confirmed["item_order"].ClaimID {
		t.Fatal("真实重连没有找到清退回执中的原角色待领物", gold, goods)
	}
	goldActionWS(sourceClient, goldClaimForTest(gold), 0)
	marketActionWS(targetClient, itemMarketClaimForTest(goods), 0)
	original, err := store.Role(ctx, domain.Hex(source.key.RoleId))
	must(t, err)
	if original.Wallet.Gold != 50_000 || original.Wallet.BoundGold != 88_888 || original.Wallet.Yuanbao != 1_000 || original.Wallet.BoundYuanbao != 9_999 ||
		!marketBagContainsSource(targetClient, item.SourceID, item.Quantity) {
		t.Fatal("回场领取后，源服原币或目标服原物未完整返还")
	}
	t.Logf("两个原运行服清退后按原路登录领取，plan=%s version=%d；管理接口全程无路由写入口", planID, plan.Version)
}

// startMergeTestGM 开真实回环 HTTP 监听，专用临时凭据只存在于本用例。
func startMergeTestGM(t *testing.T, inspection *persistence.Store) (string, string) {
	t.Helper()
	var handler http.Handler
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}))
	cfg := inspection.Cfg
	cfg.GMAddress = server.Listener.Addr().String()
	store, err := persistence.Open(t.Context(), cfg)
	must(t, err)
	key := domain.ID() + domain.ID()
	must(t, store.Singleton(t.Context(), "gm"))
	handler = (&transport.Server{Store: store, Mode: "gm", NodeKey: key}).Handler()
	server.Start()
	var renewer sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	renewer.Go(func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := store.Renew(ctx, "gm"); err != nil && ctx.Err() == nil {
					t.Errorf("隔离 GM 租约续期失败：%v", err)
					return
				}
			}
		}
	})
	t.Cleanup(func() {
		cancel()
		renewer.Wait()
		server.Close()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer store.Close(cleanup)
		must(t, store.Release(cleanup, "gm"))
	})
	return server.URL, key
}

// seedMergeTestRole 仅在首次入场前准备隔离卡、非绑和绑定钱包以及真实来源物品。
func seedMergeTestRole(t *testing.T, store *persistence.Store, rpcURL string, route directoryTestEntry, items ...domain.Item) mergeTestRole {
	t.Helper()
	login := &pb.LoginResult{}
	rpc(t, rpcURL, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "merge_" + domain.ID()[:9], Password: "test_123456"}, login, 0)
	role := createDirectoryRole(t, rpcURL, login.AccountToken, route.origin.OriginID, "清退")
	id := domain.Hex(role.Key.RoleId)
	stored, err := store.Role(t.Context(), id)
	must(t, err)
	changed, err := store.DB.Collection("roles").UpdateByID(t.Context(), id, bson.M{"$set": bson.M{
		"wallet":          domain.Wallet{Gold: 50_000, BoundGold: 88_888, Yuanbao: 1_000, BoundYuanbao: 9_999},
		"privilege_until": time.Now().Add(time.Hour), "items": append(stored.Items, items...),
	}})
	must(t, err)
	if changed.ModifiedCount != 1 {
		t.Fatal("未找到隔离清退角色", role.Name)
	}
	return mergeTestRole{key: role.Key, token: login.AccountToken, route: route}
}

// mergeTestHTTP 通过真实 HTTP 核对状态和无跨域响应，并按需要检查稳定回执。
func mergeTestHTTP(t *testing.T, endpoint, key, method, action string, input any, headers map[string]string, status int, result any) []byte {
	t.Helper()
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		must(t, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, method, endpoint+mergeTestPath+action, bytes.NewReader(body))
	must(t, err)
	if key != "" {
		request.Header.Set("X-Dev-Node-Key", key)
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	must(t, err)
	defer response.Body.Close()
	var raw bytes.Buffer
	_, err = raw.ReadFrom(response.Body)
	must(t, err)
	if response.StatusCode != status || response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("%s %s 响应 %d，预期 %d：%s", method, action, response.StatusCode, status, raw.String())
	}
	if result != nil {
		must(t, json.Unmarshal(raw.Bytes(), result))
	}
	return raw.Bytes()
}
