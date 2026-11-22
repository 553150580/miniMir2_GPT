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
func startDirectoryNode(t *testing.T, mode string, cfg devconfig.Config, address string) {
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
	t.Cleanup(func() {
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
	healthy(t, address)
	t.Logf("%s 节点就绪，PID=%d，地址=%s", mode, child.Process.Pid, address)
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

// TestServerDirectoryTwoGameNodesMongoWS 用真实账号 HTTP 和两个游戏 WebSocket 逐项验证业务闭环。
func TestServerDirectoryTwoGameNodesMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式启用专属 MongoDB 区服验收")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg = directoryMongoTarget(t, cfg)
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	cfg.OriginID, cfg.RunID, cfg.NodeID = 101, 11, "game-east"
	west := cfg
	west.GameAddress, west.OriginID, west.RunID, west.NodeID = address(t), 202, 22, "game-west"
	eastEndpoint, westEndpoint := "ws://"+cfg.GameAddress+"/ws", "ws://"+west.GameAddress+"/ws"
	cfg.OriginRoutes = []devconfig.OriginRouteSeed{
		{OriginID: cfg.OriginID, RunID: cfg.RunID, Name: "东区一服", RegionID: "east", RegionName: "东区", RegionOrder: 0},
		{OriginID: west.OriginID, RunID: west.RunID, Name: "西区一服", RegionID: "west", RegionName: "西区", RegionOrder: 1},
	}
	cfg.RunServers = []devconfig.RunServerSeed{
		{RunID: cfg.RunID, GameNodeID: cfg.NodeID, GameWSURL: eastEndpoint},
		{RunID: west.RunID, GameNodeID: west.NodeID, GameWSURL: westEndpoint},
		{RunID: 33, GameNodeID: cfg.NodeID, GameWSURL: eastEndpoint},
	}
	west.OriginRoutes, west.RunServers = cfg.OriginRoutes, cfg.RunServers

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	inspection, err := persistence.Open(ctx, cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, inspection.DB.Drop(cleanup))
		inspection.Close(cleanup)
	})
	startDirectoryNode(t, "account", cfg, cfg.AccountAddress)
	startDirectoryNode(t, "game", cfg, cfg.GameAddress)
	accountURL := "http://" + cfg.AccountAddress + "/rpc"
	rpc(t, accountURL, "", p.MsgC2SServerDirectory, &pb.ServerDirectoryRequest{}, &pb.ServerDirectoryResult{}, p.CodeUnauthenticated)
	loginRequest := &pb.LocalLoginRequest{Account: "route_" + domain.ID()[:10], Password: "test_123456"}
	login := &pb.LoginResult{}
	rpc(t, accountURL, "", p.MsgC2SLocalRegister, loginRequest, login, 0)
	rpc(t, accountURL, "", p.MsgC2SLocalLogin, loginRequest, login, 0)
	verifyDirectory(t, accountURL, login.AccountToken, false)

	startDirectoryNode(t, "game", west, west.GameAddress)
	verifyDirectory(t, accountURL, login.AccountToken, true)
	eastRole := createDirectoryRole(t, accountURL, login.AccountToken, cfg.OriginID, "东区验收")
	westRole := createDirectoryRole(t, accountURL, login.AccountToken, west.OriginID, "西区验收")
	for _, role := range []*pb.RoleSummary{eastRole, westRole} {
		directory := &pb.RoleDirectoryResult{}
		rpc(t, accountURL, login.AccountToken, p.MsgC2SRoleDirectory, &pb.RoleDirectoryRequest{OriginServerId: role.Key.OriginServerId}, directory, 0)
		if len(directory.Roles) != 1 || !bytes.Equal(role.Key.RoleId, directory.Roles[0].Key.RoleId) {
			t.Fatal("同账号的角色目录混入了其他永久入口服的角色")
		}
	}

	eastTicket := directoryTicket(t, accountURL, login.AccountToken, eastRole.Key, cfg, eastEndpoint)
	westTicket := directoryTicket(t, accountURL, login.AccountToken, westRole.Key, west, westEndpoint)
	verifyDirectoryTicketRejected(t, eastEndpoint, westTicket.Ticket)
	eastClient := join(t, directoryTicketAddress(t, eastTicket), eastTicket.Ticket)
	defer eastClient.close()
	westClient := join(t, directoryTicketAddress(t, westTicket), westTicket.Ticket)
	defer westClient.close()
	t.Log("两个入口均按各自短票地址进入并确认首场景")

	// 同一稳定节点换启动实例申请原 run 或空闲 run 均失败。
	// 不同节点也无法申请已分配给东区节点的运行服。
	registerDirectoryNode(t, cfg.AccountAddress, cfg.NodeID, domain.ID(), cfg.RunID, p.CodeServerBusy)
	registerDirectoryNode(t, cfg.AccountAddress, cfg.NodeID, domain.ID(), 33, p.CodeServerBusy)
	registerDirectoryNode(t, cfg.AccountAddress, west.NodeID, domain.ID(), cfg.RunID, p.CodeUnauthenticated)
	var spare persistence.Run
	must(t, inspection.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": uint64(33)}).Decode(&spare))
	if spare.State != "stopped" || spare.Owner != "" {
		t.Fatal("重复节点的失败登记部分取得了运行服")
	}
	var original persistence.Run
	must(t, inspection.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": cfg.RunID}).Decode(&original))
	registerDirectoryNode(t, cfg.AccountAddress, cfg.NodeID, original.Owner, 33, p.CodeServerBusy)
	retry := registerDirectoryNode(t, cfg.AccountAddress, cfg.NodeID, original.Owner, cfg.RunID, 0)
	if retry.Epoch != original.Epoch || retry.Owner != original.Owner {
		t.Fatal("原启动实例重试意外推进了代次")
	}
	verifyDirectoryRunContender(t, ctx, inspection, original)

	// 在不换游戏进程的情况下推进持久路由版本，单独验证票据的路由绑定。
	stale := directoryTicket(t, accountURL, login.AccountToken, eastRole.Key, cfg, eastEndpoint)
	result, err := inspection.DB.Collection("origin_routes").UpdateOne(ctx,
		bson.M{"_id": cfg.OriginID, "route_version": stale.RouteVersion}, bson.M{"$inc": bson.M{"route_version": 1}})
	must(t, err)
	if result.MatchedCount != 1 {
		t.Fatal("未能将隔离库中的原路由推进到新版本")
	}
	verifyDirectoryTicketRejected(t, eastEndpoint, stale.Ticket)
	fresh := directoryTicket(t, accountURL, login.AccountToken, eastRole.Key, cfg, eastEndpoint)
	if fresh.RouteVersion != stale.RouteVersion+1 {
		t.Fatal("账号节点没有从持久目录读取新路由版本")
	}
	reconnected := join(t, directoryTicketAddress(t, fresh), fresh.Ticket)
	defer reconnected.close()
	t.Log("已拒绝串服票据、重复节点、争抢运行服和旧路由票据，新票正常接管")

	if root := os.Getenv("MINIMIR_SERVER_DIRECTORY_WEB_ROOT"); root != "" {
		verifyDirectoryBrowser(t, cfg.AccountAddress, root, eastEndpoint, westEndpoint)
	}
}

// verifyDirectoryRunContender 模拟待换机的部署分配，验证有效运行租约仍能拒绝另一已分配节点。
func verifyDirectoryRunContender(t *testing.T, ctx context.Context, store *persistence.Store, current persistence.Run) {
	t.Helper()
	const contender = "game-contender"
	assignments := store.DB.Collection("run_servers")
	filter := bson.M{"_id": current.ID, "game_node_id": current.GameNodeID, "assignment_version": current.AssignmentVersion}
	changed, err := assignments.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"game_node_id": contender},
		"$inc": bson.M{"assignment_version": 1}})
	must(t, err)
	if changed.MatchedCount != 1 {
		t.Fatal("隔离库的换机前态未生效")
	}
	registerDirectoryNode(t, store.Cfg.AccountAddress, contender, domain.ID(), current.ID, p.CodeServerBusy)
	var after persistence.Run
	must(t, store.DB.Collection("run_control").FindOne(ctx, bson.M{"_id": current.ID}).Decode(&after))
	if after.Owner != current.Owner || after.Epoch != current.Epoch || after.GameNodeID != current.GameNodeID {
		t.Fatal("未到期的运行服被竞争节点部分接管")
	}
	restored, err := assignments.UpdateOne(ctx, bson.M{"_id": current.ID, "game_node_id": contender},
		bson.M{"$set": bson.M{"game_node_id": current.GameNodeID, "assignment_version": current.AssignmentVersion}})
	must(t, err)
	if restored.MatchedCount != 1 {
		t.Fatal("未能恢复隔离验收的原分配")
	}
}

// verifyDirectory 检查目录分组、永久入口以及真实节点上线后的可入场状态。
func verifyDirectory(t *testing.T, accountURL, token string, westAvailable bool) {
	t.Helper()
	directory := &pb.ServerDirectoryResult{}
	rpc(t, accountURL, token, p.MsgC2SServerDirectory, &pb.ServerDirectoryRequest{}, directory, 0)
	regions := directory.Regions
	if len(regions) != 2 || regions[0].RegionId != "east" || regions[1].RegionId != "west" ||
		len(regions[0].Servers) != 1 || len(regions[1].Servers) != 1 ||
		regions[0].Servers[0].OriginServerId != 101 || !regions[0].Servers[0].Available ||
		regions[1].Servers[0].OriginServerId != 202 || regions[1].Servers[0].Available != westAvailable {
		t.Fatalf("区服目录分组、顺序或可用状态错误: %v", directory)
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
func directoryTicket(t *testing.T, accountURL, token string, role *pb.RoleKey, cfg devconfig.Config, endpoint string) *pb.EntryTicketResult {
	t.Helper()
	ticket := &pb.EntryTicketResult{}
	rpc(t, accountURL, token, p.MsgC2SEntryTicket, &pb.EntryTicketRequest{Role: role}, ticket, 0)
	if ticket.GameNodeId != cfg.NodeID || ticket.RunServerId != cfg.RunID || ticket.GameWssUrl != endpoint || ticket.RouteVersion == 0 {
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
func registerDirectoryNode(t *testing.T, accountAddress, nodeID, bootID string, runID uint64, expected uint32) persistence.Run {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"game_node_id": nodeID, "boot_id": bootID, "run_server_id": runID})
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
	var run persistence.Run
	if expected == 0 {
		if response.StatusCode != http.StatusOK {
			t.Fatalf("原注册重试返回 HTTP %d", response.StatusCode)
		}
		must(t, json.NewDecoder(response.Body).Decode(&run))
		return run
	}
	var fail struct {
		Code uint32 `json:"error_code"`
	}
	must(t, json.NewDecoder(response.Body).Decode(&fail))
	if response.StatusCode != http.StatusBadRequest || fail.Code != expected {
		t.Fatalf("节点登记拒绝码错误: HTTP %d，业务码 %d，预期 %d", response.StatusCode, fail.Code, expected)
	}
	return run
}

// verifyDirectoryBrowser 在独立浏览器入口提供实际构建和账号代理，让游戏连接按票据直连。
func verifyDirectoryBrowser(t *testing.T, account string, webRoot, eastEndpoint, westEndpoint string) {
	t.Helper()
	repo, err := filepath.Abs("../../..")
	must(t, err)
	build, err := filepath.Abs(webRoot)
	must(t, err)
	work := filepath.Join(repo, ".work")
	relative, err := filepath.Rel(work, build)
	must(t, err)
	if !filepath.IsLocal(relative) || relative == "." {
		t.Fatal("区服浏览器验收只使用工作目录内的独立构建")
	}
	_, err = os.Stat(filepath.Join(build, "index.html"))
	must(t, err)
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
	raw, err := json.Marshal(map[string]any{"baseUrl": web.URL, "entries": []map[string]string{
		{"region": "东区", "server": "东区一服", "origin": "101", "endpoint": eastEndpoint},
		{"region": "西区", "server": "西区一服", "origin": "202", "endpoint": westEndpoint},
	}})
	must(t, err)
	must(t, os.WriteFile(manifest, raw, 0600))
	t.Logf("浏览器验收证据目录 %s", resultDir)
	ctx, stop := context.WithTimeout(context.Background(), 3*time.Minute)
	defer stop()
	command := exec.CommandContext(ctx, "node", filepath.Join(repo, "tools/client/verify_server_directory_browser.cjs"), manifest)
	command.Dir = repo
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("双区服浏览器未完成: %v\n%s", err, output)
	}
}
