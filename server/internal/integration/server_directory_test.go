// 使用独立账号和游戏进程验证区服目录、跨节点入场和过期路由隔离。

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.mongodb.org/mongo-driver/v2/bson"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

const directoryChildConfig = "MINIMIR_SERVER_DIRECTORY_CHILD_CONFIG"
const directoryChallengeCodex = "MINIMIR_VERIFY_CHALLENGE_CODEX"

// directoryNodeProcess 记录由父测试传给独立子进程的装配信息和退出控制。
type directoryNodeProcess struct {
	Mode   string
	Config devconfig.Config
}

// TestServerDirectoryNodeProcess 只在父测试显式指定配置时装配一个节点。
func TestServerDirectoryNodeProcess(t *testing.T) {
	path := os.Getenv(directoryChildConfig)
	if path == "" {
		t.Skip("仅由区服验收父测试启动")
	}
	raw, err := os.ReadFile(path)
	must(t, err)
	base, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	node := directoryNodeProcess{Config: base}
	must(t, json.Unmarshal(raw, &node))
	installDirectoryChallengeCodex(t, &node.Config.Fixture)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		// 父测试结束时关闭专用输入管道，触发原有受控停服流程。
		_, _ = io.Copy(io.Discard, os.Stdin)
		stop()
	}()
	must(t, bootstrap.Run(ctx, node.Config, node.Mode))
}

// startDirectoryNode 启动一个真正独立的节点进程，退出后再允许测试删除隔离库。
func startDirectoryNode(t *testing.T, mode string, cfg devconfig.Config, address string) func() {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "directory-node.json")
	raw, err := json.Marshal(directoryNodeProcess{Mode: mode, Config: cfg})
	must(t, err)
	must(t, os.WriteFile(configPath, raw, 0600))
	binary, err := os.Executable()
	must(t, err)
	child := exec.Command(binary, "-test.run=^TestServerDirectoryNodeProcess$")
	child.Env = append(os.Environ(), directoryChildConfig+"="+configPath)
	var log bytes.Buffer
	child.Stdout, child.Stderr = &log, &log
	input, err := child.StdinPipe()
	must(t, err)
	must(t, child.Start())
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()
	stop := sync.OnceFunc(func() {
		_ = input.Close()
		select {
		case err := <-exited:
			if err != nil {
				t.Errorf("%s 节点退出: %v\n%s", mode, err, log.String())
			}
		case <-time.After(15 * time.Second):
			_ = child.Process.Kill()
			<-exited
			t.Errorf("%s 节点未在受控期限内停止\n%s", mode, log.String())
		}
	})
	t.Cleanup(stop)
	healthy(t, address)
	t.Logf("%s 节点就绪，PID=%d，地址=%s", mode, child.Process.Pid, address)
	return stop
}

// directoryMongoTarget 使用项目专属开发库，或由独立副本集控制器核验的本轮回环端点。
func directoryMongoTarget(t *testing.T, cfg devconfig.Config) devconfig.Config {
	t.Helper()
	cfg.Database = "minimir_dev_game_test_dir_" + domain.ID()
	if isolated := os.Getenv("MINIMIR_SERVER_DIRECTORY_TEST_MONGO_URI"); isolated != "" {
		verifiedFormat := regexp.MustCompile(`^mongodb://127\.0\.0\.1:[1-9][0-9]{3,4}/\?replicaSet=minimirLoad_[a-f0-9]{32}&directConnection=true$`)
		if !verifiedFormat.MatchString(isolated) {
			t.Fatal("区服验收只接受本轮独立副本集控制器生成的端点")
		}
		cfg.MongoURI = isolated
	}
	return cfg
}

// directoryTestEntry 关联入口目录、本轮部署和浏览器应收到的公告地址。
type directoryTestEntry struct {
	origin     devconfig.OriginRouteSeed
	assignment devconfig.RunServerSeed
}

// TestServerDirectoryTwoGameNodesMongoWS 用真实账号 HTTP 和两个独立游戏进程验证四个运行服。
func TestServerDirectoryTwoGameNodesMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式启用专属 MongoDB 区服验收")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg = directoryMongoTarget(t, cfg)
	installDirectoryChallengeCodex(t, &cfg.Fixture)
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	cfg.OriginID, cfg.RunID, cfg.NodeID = 101, 11, "game-east"
	cfg.RunServerIDs = []uint64{11, 12}
	west := cfg
	west.GameAddress, west.OriginID, west.RunID, west.NodeID = address(t), 201, 21, "game-west"
	west.RunServerIDs = []uint64{21, 22}
	entries := []directoryTestEntry{
		makeDirectoryEntry(101, 11, cfg, "east", "东区", "东区一服", 0, 0),
		makeDirectoryEntry(102, 12, cfg, "east", "东区", "东区二服", 0, 1),
		makeDirectoryEntry(201, 21, west, "west", "西区", "西区一服", 1, 0),
		makeDirectoryEntry(202, 22, west, "west", "西区", "西区二服", 1, 1),
	}
	for _, entry := range entries {
		cfg.OriginRoutes = append(cfg.OriginRoutes, entry.origin)
		cfg.RunServers = append(cfg.RunServers, entry.assignment)
	}
	// 保留两个无角色的备用运行服，分别检查节点排他和批量授权回滚。
	for _, id := range []uint64{5, 6} {
		cfg.RunServers = append(cfg.RunServers, devconfig.RunServerSeed{
			RunID: id, GameNodeID: cfg.NodeID, GameWSURL: entries[0].assignment.GameWSURL,
		})
	}
	west.OriginRoutes, west.RunServers = cfg.OriginRoutes, cfg.RunServers

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	// 同一进程中的第二个运行服使用不同代次，错误复用首服资产围栏会直接失败。
	for _, id := range []uint64{12, 22} {
		result, err := inspection.DB.Collection("run_control").UpdateOne(ctx, bson.M{"_id": id, "state": "stopped"},
			bson.M{"$set": bson.M{"run_epoch": int64(id)}})
		must(t, err)
		if result.MatchedCount != 1 {
			t.Fatal("未能设置隔离运行服代次")
		}
	}
	startDirectoryNode(t, "account", cfg, cfg.AccountAddress)
	startDirectoryNode(t, "game", cfg, cfg.GameAddress)
	verifyHostedRunIDs(t, cfg)
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	rpc(t, accountURL, "", p.MsgC2SServerDirectory, &pb.ServerDirectoryRequest{}, &pb.ServerDirectoryResult{}, p.CodeUnauthenticated)
	loginRequest := &pb.LocalLoginRequest{Account: "route_" + domain.ID()[:10], Password: "test_123456"}
	login := &pb.LoginResult{}
	rpc(t, accountURL, "", p.MsgC2SLocalRegister, loginRequest, login, 0)
	rpc(t, accountURL, "", p.MsgC2SLocalLogin, loginRequest, login, 0)
	verifyDirectory(t, accountURL, login.AccountToken, entries, false)

	stopWest := startDirectoryNode(t, "game", west, west.GameAddress)
	verifyHostedRunIDs(t, west)
	verifyDirectory(t, accountURL, login.AccountToken, entries, true)
	roles := make([]*pb.RoleSummary, 0, len(entries))
	connections := make([]*client, 0, len(entries))
	for i, entry := range entries {
		role := createDirectoryRole(t, accountURL, login.AccountToken, entry.origin.OriginID, "路由验收")
		directory := &pb.RoleDirectoryResult{}
		rpc(t, accountURL, login.AccountToken, p.MsgC2SRoleDirectory, &pb.RoleDirectoryRequest{OriginServerId: role.Key.OriginServerId}, directory, 0)
		if len(directory.Roles) != 1 || !bytes.Equal(role.Key.RoleId, directory.Roles[0].Key.RoleId) {
			t.Fatal("同账号的角色目录混入了其他永久入口服的角色")
		}
		ticket := directoryTicket(t, accountURL, login.AccountToken, role.Key, entry)
		if i >= 2 {
			verifyDirectoryTicketRejected(t, entries[0].assignment.GameWSURL, ticket.Ticket)
		}
		connection := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
		defer connection.close()
		roles, connections = append(roles, role), append(connections, connection)
		t.Logf("入口 %d 已完成首场景确认，run=%d node=%s", entry.origin.OriginID, entry.origin.RunID, entry.assignment.GameNodeID)
	}
	verifyDirectoryAssets(t, ctx, inspection, entries, roles, connections)

	// 同一稳定节点换启动实例申请原 run 或空闲 run 均失败。
	registerDirectoryNode(t, cfg.AccountAddress, cfg.NodeID, domain.ID(), cfg.RunServerIDs, p.CodeServerBusy)
	registerDirectoryNode(t, cfg.AccountAddress, cfg.NodeID, domain.ID(), []uint64{5, 6}, p.CodeServerBusy)
	registerDirectoryNode(t, cfg.AccountAddress, west.NodeID, domain.ID(), cfg.RunServerIDs, p.CodeUnauthenticated)
	requireStoppedRuns(t, ctx, inspection, []uint64{5, 6})
	primary := directoryRun(t, ctx, inspection, cfg.RunID)
	secondary := directoryRun(t, ctx, inspection, 12)
	retry := registerDirectoryNode(t, cfg.AccountAddress, cfg.NodeID, primary.Owner, cfg.RunServerIDs, 0)
	if len(retry) != 2 || retry[0].ID != primary.ID || retry[0].Epoch != primary.Epoch ||
		retry[1].ID != secondary.ID || retry[1].Epoch != secondary.Epoch {
		t.Fatal("原启动实例重试改变了运行服授权集合或代次")
	}
	verifyDirectoryRunContender(t, ctx, inspection, secondary)

	// 在同一进程的第二个运行服推进票据路由版本。
	stale := directoryTicket(t, accountURL, login.AccountToken, roles[1].Key, entries[1])
	result, err := inspection.DB.Collection("origin_routes").UpdateOne(ctx,
		bson.M{"_id": entries[1].origin.OriginID, "route_version": stale.RouteVersion}, bson.M{"$inc": bson.M{"route_version": 1}})
	must(t, err)
	if result.MatchedCount != 1 {
		t.Fatal("未能将隔离库中的原路由推进到新版本")
	}
	verifyDirectoryTicketRejected(t, entries[1].assignment.GameWSURL, stale.Ticket)
	fresh := directoryTicket(t, accountURL, login.AccountToken, roles[1].Key, entries[1])
	if fresh.RouteVersion != stale.RouteVersion+1 {
		t.Fatal("账号节点没有从持久目录读取新路由版本")
	}
	reconnected := join(t, directoryTicketAddress(t, fresh), fresh.Ticket)
	defer reconnected.close()
	t.Log("已拒绝错误节点票据、重复节点、争抢运行服和旧路由票据，新票正常接管")

	if root := os.Getenv("MINIMIR_SERVER_DIRECTORY_WEB_ROOT"); root != "" {
		verifyDirectoryBrowser(t, cfg.AccountAddress, root, entries, cfg.Fixture.TasksCodex != nil,
			cfg.Fixture.PersonalBoss != nil && cfg.Fixture.WorldBoss != nil)
	}
	stopWest()
	// 三城触屏会占用单独的浏览器预算，停服后的数据库核对需要从此处单独计时。
	stoppedCtx, cancelStopped := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancelStopped()
	requireStoppedRuns(t, stoppedCtx, inspection, west.RunServerIDs)
	verifyDirectory(t, accountURL, login.AccountToken, entries, false)
	t.Log("西区停服同时释放两个运行服，东区两个运行服维持在线")
}

// installDirectoryChallengeCodex 用原版已装包的首领测试外图预览，测试资格只存在于本轮独立浏览器环境。
func installDirectoryChallengeCodex(t *testing.T, fixture *devconfig.Fixture) {
	t.Helper()
	if os.Getenv(directoryChallengeCodex) != "1" || os.Getenv("MINIMIR_SERVER_DIRECTORY_WEB_ROOT") == "" {
		return
	}
	boss := fixture.PersonalBoss
	if boss == nil || fixture.TasksCodex == nil {
		t.Fatal("外图图鉴验收缺少原个人首领或任务目录")
	}
	fixture.TasksCodex.Definitions = append(fixture.TasksCodex.Definitions, devconfig.TaskCodexDefinition{
		ID: 80, Kind: devconfig.TaskCodexMonster, Name: boss.DisplayName, Target: 1, Event: devconfig.TaskCodexMonsterKill,
		MonsterID: boss.Monster.ID, MapID: boss.Map.ID,
	})
	must(t, fixture.ValidateTasksCodex())
}

// makeDirectoryEntry 使用同一部署节点为其各运行服公告同一个 WebSocket 路径。
func makeDirectoryEntry(origin, run uint64, cfg devconfig.Config, regionID, region, name string, regionOrder, order int) directoryTestEntry {
	return directoryTestEntry{
		origin: devconfig.OriginRouteSeed{
			OriginID: origin, RunID: run, RegionID: regionID, RegionName: region, Name: name, RegionOrder: regionOrder, Order: order,
		},
		assignment: devconfig.RunServerSeed{RunID: run, GameNodeID: cfg.NodeID, GameWSURL: "ws://" + cfg.GameAddress + "/ws"},
	}
}

// verifyDirectoryRunContender 模拟待换机的部署分配，验证有效运行租约仍能拒绝另一已分配节点。
func verifyDirectoryRunContender(t *testing.T, ctx context.Context, store *persistence.Store, current persistence.Run) {
	t.Helper()
	const contender = "game-contender"
	assignments := store.DB.Collection("run_servers")
	// 先申请更小且空闲的运行服，再竞争已有有效租约的运行服。
	contested := []uint64{5, current.ID}
	filter := bson.M{"_id": bson.M{"$in": contested}, "game_node_id": current.GameNodeID, "assignment_version": current.AssignmentVersion}
	changed, err := assignments.UpdateMany(ctx, filter, bson.M{"$set": bson.M{"game_node_id": contender},
		"$inc": bson.M{"assignment_version": 1}})
	must(t, err)
	if changed.MatchedCount != 2 {
		t.Fatal("隔离库的换机前态未生效")
	}
	registerDirectoryNode(t, store.Cfg.AccountAddress, contender, domain.ID(), contested, p.CodeServerBusy)
	after := directoryRun(t, ctx, store, current.ID)
	if after.Owner != current.Owner || after.Epoch != current.Epoch || after.GameNodeID != current.GameNodeID {
		t.Fatal("未到期的运行服被竞争节点部分接管")
	}
	requireStoppedRuns(t, ctx, store, []uint64{5})
	if count, err := store.DB.Collection("node_leases").CountDocuments(ctx, bson.M{"_id": contender}); err != nil || count != 0 {
		t.Fatalf("批量申请失败后仍留下节点租约，数量=%d 错误=%v", count, err)
	}
	restored, err := assignments.UpdateMany(ctx, bson.M{"_id": bson.M{"$in": contested}, "game_node_id": contender},
		bson.M{"$set": bson.M{"game_node_id": current.GameNodeID, "assignment_version": current.AssignmentVersion}})
	must(t, err)
	if restored.MatchedCount != 2 {
		t.Fatal("未能恢复隔离验收的原分配")
	}
}

// directoryRun 读取隔离库中的当前运行服权属。
func directoryRun(t *testing.T, ctx context.Context, store *persistence.Store, runID uint64) persistence.Run {
	t.Helper()
	var run persistence.Run
	must(t, store.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": runID}).Decode(&run))
	return run
}

// requireStoppedRuns 检查整批租约回滚或退出后，每个运行服都已停止。
func requireStoppedRuns(t *testing.T, ctx context.Context, store *persistence.Store, ids []uint64) {
	t.Helper()
	for _, id := range ids {
		if state := directoryRun(t, ctx, store, id).State; state != "stopped" {
			t.Fatalf("运行服 %d 仍处于 %s", id, state)
		}
	}
}

// verifyHostedRunIDs 核对同一监听端口上的健康信息列出了该进程承载的全部运行服。
func verifyHostedRunIDs(t *testing.T, cfg devconfig.Config) {
	t.Helper()
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Get("http://" + cfg.GameAddress + "/healthz")
	must(t, err)
	defer response.Body.Close()
	var health struct {
		Node string   `json:"game_node_id"`
		Runs []uint64 `json:"run_server_ids"`
	}
	must(t, json.NewDecoder(response.Body).Decode(&health))
	if response.StatusCode != http.StatusOK || health.Node != cfg.NodeID || !slices.Equal(health.Runs, cfg.RunServerIDs) {
		t.Fatalf("节点承载集合不符: HTTP %d，node=%s，runs=%v", response.StatusCode, health.Node, health.Runs)
	}
}

// verifyDirectoryAssets 在每节点第二个运行服真实领取托管时长，并核对同进程其他服的角色资产。
func verifyDirectoryAssets(t *testing.T, ctx context.Context, store *persistence.Store, entries []directoryTestEntry, roles []*pb.RoleSummary, clients []*client) {
	t.Helper()
	for _, index := range []int{1, 3} {
		connection := clients[index]
		result := connection.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: connection.operation()})
		if result.ErrorCode != 0 {
			t.Fatalf("运行服 %d 的资产事务没有走本服围栏，错误码 %d", entries[index].origin.RunID, result.ErrorCode)
		}
	}
	for i, entry := range entries {
		role, err := store.Role(ctx, domain.Hex(roles[i].Key.RoleId))
		must(t, err)
		run := directoryRun(t, ctx, store, entry.origin.RunID)
		if role.Origin != entry.origin.OriginID || role.RunServerID != run.ID || role.Owner != run.Owner || role.RunEpoch != run.Epoch {
			t.Fatalf("入口 %d 的角色绑定不属于本运行服", entry.origin.OriginID)
		}
		var expectedSeconds int64
		if i == 1 || i == 3 {
			expectedSeconds = 21600
		}
		if role.HostingSeconds != expectedSeconds {
			t.Fatalf("入口 %d 的角色托管余额为 %d，预期 %d", role.Origin, role.HostingSeconds, expectedSeconds)
		}
	}
	t.Log("两个次运行服的资产领取成功，同节点其他角色余额没有变化")
}

// verifyDirectory 检查目录分组、永久入口以及真实节点上线后的可入场状态。
func verifyDirectory(t *testing.T, accountURL, token string, entries []directoryTestEntry, westAvailable bool) {
	t.Helper()
	directory := &pb.ServerDirectoryResult{}
	rpc(t, accountURL, token, p.MsgC2SServerDirectory, &pb.ServerDirectoryRequest{}, directory, 0)
	regions := directory.Regions
	if len(regions) != 2 || len(regions[0].Servers) != 2 || len(regions[1].Servers) != 2 {
		t.Fatalf("区服目录分组、顺序或可用状态错误: %v", directory)
	}
	for i, entry := range entries {
		region := regions[i/2]
		server := region.Servers[i%2]
		if region.RegionId != entry.origin.RegionID || server.OriginServerId != entry.origin.OriginID ||
			server.Name != entry.origin.Name || server.Available != (i < 2 || westAvailable) {
			t.Fatalf("入口 %d 的目录内容有误: %v", entry.origin.OriginID, directory)
		}
	}
}

// createDirectoryRole 在选中的永久入口服下创建测试角色。
func createDirectoryRole(t *testing.T, accountURL, token string, origin uint64, prefix string) *pb.RoleSummary {
	t.Helper()
	role := &pb.RoleSummary{}
	rpc(t, accountURL, token, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
		OriginServerId: origin, Slot: 1, Job: 0, Name: prefix + domain.ID()[:5], OpId: domain.Bytes(domain.ID()),
	}, role, 0)
	return role
}

// directoryTicket 核对票据携带持久目录解析出的目标运行服、节点身份和连接地址。
func directoryTicket(t *testing.T, accountURL, token string, role *pb.RoleKey, entry directoryTestEntry) *pb.EntryTicketResult {
	t.Helper()
	ticket := &pb.EntryTicketResult{}
	rpc(t, accountURL, token, p.MsgC2SEntryTicket, &pb.EntryTicketRequest{Role: role}, ticket, 0)
	if ticket.GameNodeId != entry.assignment.GameNodeID || ticket.RunServerId != entry.origin.RunID ||
		ticket.GameWssUrl != entry.assignment.GameWSURL || ticket.RouteVersion == 0 {
		t.Fatalf("票据路由错误: 节点=%s 运行服=%d 地址=%s 版本=%d", ticket.GameNodeId, ticket.RunServerId, ticket.GameWssUrl, ticket.RouteVersion)
	}
	return ticket
}

// directoryTicketAddress 从票据公告地址取出通用入场探针接受的回环监听地址。
func directoryTicketAddress(t *testing.T, ticket *pb.EntryTicketResult) string {
	t.Helper()
	endpoint, err := url.Parse(ticket.GameWssUrl)
	must(t, err)
	return endpoint.Host
}

// verifyDirectoryTicketRejected 向指定游戏节点递交错误票据，要求在场景同步前收到路由拒绝。
func verifyDirectoryTicketRejected(t *testing.T, endpoint, ticket string) {
	t.Helper()
	ctx, stop := context.WithTimeout(context.Background(), 8*time.Second)
	defer stop()
	ws, _, err := websocket.Dial(ctx, endpoint, nil)
	must(t, err)
	defer ws.CloseNow()
	must(t, ws.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(
		p.MsgC2SEnterLine, 1, 0, &pb.EnterLineRequest{AccountTicket: ticket},
	))))
	_, data, err := ws.Read(ctx)
	must(t, err)
	frame, err := wire.Decode(data)
	must(t, err)
	if frame.ID != p.MsgS2CEnterLine || frame.Request != 1 || frame.Error != p.CodeRouteChanged {
		t.Fatalf("错误路由的票据未按原请求拒绝: 消息=%d 请求=%d 错误=%d", frame.ID, frame.Request, frame.Error)
	}
}

// registerDirectoryNode 通过账号节点的内部注册入口检验持久租约仲裁结果。
func registerDirectoryNode(t *testing.T, accountAddress, nodeID, bootID string, runIDs []uint64, expected uint32) []persistence.Run {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"game_node_id": nodeID, "boot_id": bootID, "run_server_ids": runIDs})
	must(t, err)
	request, err := http.NewRequest("POST", "http://"+accountAddress+"/internal/register-game", bytes.NewReader(payload))
	must(t, err)
	key, err := bootstrap.NodeKey()
	must(t, err)
	request.Header.Set("X-Dev-Node-Key", key)
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	must(t, err)
	defer response.Body.Close()
	if expected == 0 {
		if response.StatusCode != http.StatusOK {
			t.Fatalf("原注册重试返回 HTTP %d", response.StatusCode)
		}
		var grant struct {
			Runs []persistence.Run `json:"runs"`
		}
		must(t, json.NewDecoder(response.Body).Decode(&grant))
		return grant.Runs
	}
	var fail struct {
		Code uint32 `json:"error_code"`
	}
	must(t, json.NewDecoder(response.Body).Decode(&fail))
	if response.StatusCode != http.StatusBadRequest || fail.Code != expected {
		t.Fatalf("节点登记拒绝码错误: HTTP %d，业务码 %d，预期 %d", response.StatusCode, fail.Code, expected)
	}
	return nil
}

// verifyDirectoryBrowser 在独立浏览器入口提供实际构建和账号代理，让游戏连接按票据直连。
func verifyDirectoryBrowser(t *testing.T, account string, webRoot string, entries []directoryTestEntry, tasksCodex, bossChallenges bool) {
	t.Helper()
	repo, err := filepath.Abs("../../..")
	must(t, err)
	build, err := creatorBrowserBuildPath(repo, webRoot)
	must(t, err)
	work := filepath.Join(repo, ".work")
	upstream, err := url.Parse("http://" + account)
	must(t, err)
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	mux := http.NewServeMux()
	mux.Handle("/account/rpc", http.StripPrefix("/account", proxy))
	mux.Handle("/", http.FileServer(http.Dir(build)))
	web := httptest.NewServer(mux)
	defer web.Close()

	resultDir, err := os.MkdirTemp(work, "server-directory-browser-")
	must(t, err)
	manifest := filepath.Join(resultDir, "manifest.json")
	browserEntries := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		browserEntries = append(browserEntries, map[string]string{
			"region": entry.origin.RegionName, "server": entry.origin.Name,
			"origin": strconv.FormatUint(entry.origin.OriginID, 10), "run": strconv.FormatUint(entry.origin.RunID, 10),
			"node": entry.assignment.GameNodeID, "endpoint": entry.assignment.GameWSURL,
		})
	}
	raw, err := json.Marshal(map[string]any{"baseUrl": web.URL, "entries": browserEntries,
		"tasksCodex": tasksCodex, "bossChallenges": bossChallenges})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("浏览器验收证据目录 %s", resultDir)
	browserTimeout := 3 * time.Minute
	if os.Getenv("MINIMIR_VERIFY_TOWN_GROUND_NAVIGATION") == "1" {
		// 三座城镇逐步播放后再点击下一格，完整八向回归有独立时间预算。
		browserTimeout = 4 * time.Minute
	}
	ctx, stop := context.WithTimeout(context.Background(), browserTimeout)
	defer stop()
	command := exec.CommandContext(ctx, "node", filepath.Join(repo, "tools/client/verify_server_directory_browser.cjs"), manifest)
	command.Dir = repo
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("四服浏览器验收未完成: %v\n%s", err, output)
	}
}
