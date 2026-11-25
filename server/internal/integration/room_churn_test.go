// 模拟角色持续换人入场、到期逐出和健康重连，观察游戏房间配额与恢复边界。

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestDormantRoomChurnAndHealthyReconnectMongoWS 制造足量入场、离开与重连，验证过期房间回收且仍有效房间保持原场景。
// t 提供断言失败报告、子用例和清理注册。
func TestDormantRoomChurnAndHealthyReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启独立 MongoDB 验证房间回收和真实持续换人")
	}
	// cfg 保存由加载取得的服务配置，err 接收加载过程中可能产生的错误，供后续失败分支处理。
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database = "minimir_dev_game_test_churn_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	// 冻结隔离怪物的攻击与移动，便于独立断言在线和托管房间保留。
	// i 标记遍历怪物集合时访问的下标。
	for i := range cfg.Fixture.Monsters {
		// monster 保存当前处理所用的怪物。
		monster := &cfg.Fixture.Monsters[i]
		monster.HP, monster.MoveMS, monster.AttackMin, monster.AttackMax = 1000, 120000, 0, 0
	}
	must(t, cfg.Fixture.Validate())
	// ctx 传递从调用方派生的上下文，供取消或控制截止时间，cancel 取消对应上下文或停止计时，释放其关联资源。
	ctx, cancel := context.WithTimeout(t.Context(), 115*time.Second)
	defer cancel()
	// store 保存由打开取得的持久层存储。
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	// nodes 记录节点集合，使用协程汇合计数器表示。
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		// cleanup 传递从调用方派生的上下文，供取消或控制截止时间，stop 取消对应上下文或停止计时，释放其关联资源。
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	// 每名测试角色独享账号与票据，并映射到同一分片，命中默认256间房的准入上限。
	const capacity = 256
	// roles 保存由预置数据房间“churn”角色集合取得的角色集合。
	roles := seedRoomChurnRoles(t, ctx, store, capacity+2)
	// mode 接收遍历文本列表时当前的元素。
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			// err 接收运行批次过程中可能产生的错误，供后续失败分支处理。
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("换人验收%s节点：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	// endpoint 保存根据当前数据与账号地址的计算结果与当前数据计算的端点。
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	// enter 封装入场对应的后续处理，i 记录当前位置的下标，使用数值表示。
	enter := func(i int) *client {
		return join(t, cfg.GameAddress, issue(t, endpoint, roles[i].token, roles[i].key))
	}

	// dormant 保存由入场取得的“dormant”。
	dormant := enter(0)
	dormant.auto(false, 1)
	// claim 保存由资产取得的领取。
	claim := dormant.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: dormant.operation()})
	if claim.ErrorCode != 0 {
		t.Fatalf("预置角色领取长期余额失败：%v", claim)
	}
	dormant.inventoryAt(claim.NewRevision)
	// persisted 保存由角色取得的“persisted”。
	persisted, err := store.Role(ctx, domain.Hex(roles[0].key.RoleId))
	must(t, err)
	if persisted.HostingSeconds <= 0 {
		t.Fatal("托管日领未形成永久余额")
	}
	// spawn 按目标类型读取当前接口值，得到生成或出生点。
	spawn := proto.Clone(dormant.player().Position).(*pb.GridPoint)
	// goal 保存当前处理所用的目标格。
	goal := &pb.GridPoint{X: spawn.X + 1, Y: spawn.Y}
	dormant.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: dormant.entry.SceneEpoch, Destination: goal})
	// f 记录协议帧。
	travelUntilWithin(dormant, 4*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && proto.Equal(dormant.player().Position, goal)
	})
	// dormantScene 保存场景代次，用于当前步骤中的“dormant”场景，dormantLife 保存生命代次，用于当前步骤中的“dormant”生命代次。
	dormantScene, dormantLife := dormant.entry.SceneEpoch, dormant.player().Life
	// dormantHP 保存生命值，用于当前步骤中的“dormant”生命值。
	dormantHP := dormant.player().Hp
	dormant.close()
	dormant = enter(0)
	if dormant.entry.SceneEpoch != dormantScene || dormant.player().Life != dormantLife ||
		dormant.player().Hp != dormantHP || !proto.Equal(dormant.player().Position, goal) {
		t.Fatal("健康窗口内重连未保留原场景、生命或坐标")
	}
	dormant.close()

	// hosted 保存由入场取得的已网络主机或托管。
	hosted := enter(1)
	hosted.auto(true, 1)
	// hostedScene 保存场景代次，用于当前步骤中的已网络主机或托管场景。
	hostedScene := hosted.entry.SceneEpoch
	hosted.close()
	// online 保存由入场取得的在线。
	online := enter(2)
	defer online.close()
	// heartbeatRequest 规定“heartbeat”请求的初始取值。
	const heartbeatRequest = 9999
	// heartbeat 创建用于维护“heartbeat”的容器或通道。
	heartbeat := make(chan error, 1)
	go func() {
		for {
			// raw 保存由读取取得的原始值，err 接收读取过程中可能产生的错误，供后续失败分支处理。
			_, raw, err := online.ws.Read(ctx)
			if err != nil {
				heartbeat <- err
				return
			}
			// frame 保存由解码取得的协议帧。
			frame, err := wire.Decode(raw)
			if err != nil || frame.ID == p.MsgS2CHeartbeat && frame.Request == heartbeatRequest && frame.Error == 0 {
				heartbeat <- err
				return
			}
		}
	}()
	// i 标记循环或列表中的当前位置。
	for i := 3; i < capacity; i++ {
		enter(i).close()
	}
	// ticket 保存由签发取得的入场票据。
	ticket := issue(t, endpoint, roles[capacity].token, roles[capacity].key)
	assertEntryRejectionFrame(t, cfg.GameAddress, ticket, p.CodeServerBusy)
	t.Log("同一分片满额且回收窗口未到，真实新角色收到结构化容量拒绝")

	// 等待真实 worker 时钟越过最后一个空闲房间的回收窗口。
	// timer 保存由创建定时器取得的定时器。
	timer := time.NewTimer(time.Minute + time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("等待真实回收窗口截止失败")
	}
	// i 保存当前处理所用的当前位置的下标。
	for i := capacity; i < len(roles); i++ {
		enter(i).close()
	}
	// cold 保存由入场取得的“cold”。
	cold := enter(0)
	defer cold.close()
	if cold.entry.SceneEpoch == dormantScene || !proto.Equal(cold.player().Position, spawn) ||
		int64(cold.hosting.RemainHostingSec) != persisted.HostingSeconds {
		t.Fatal("空闲到期再入场未建立初始场景，或永久托管余额发生变化")
	}
	hosted = enter(1)
	defer hosted.close()
	if hosted.entry.SceneEpoch != hostedScene || hosted.hosting.RemainHostingSec <= 0 || hosted.hosting.RemainHostingSec >= 180 {
		t.Fatal("有效离线托管应保留原场景并按实际运行扣时")
	}
	must(t, online.ws.Write(ctx, websocket.MessageBinary, wire.Encode(wire.Message(p.MsgC2SHeartbeat, heartbeatRequest, online.binding, &pb.Heartbeat{}))))
	select {
	// err 保存当前操作的失败原因，供调用方检查或返回。
	case err := <-heartbeat:
		must(t, err)
	case <-ctx.Done():
		t.Fatal("持续在线的原连接缺少正常心跳")
	}
	t.Logf("同一分片累计准入%d个独立角色；离线超期让出新槽位，健康重连、托管余额与在线连接通过", len(roles))
}

// roomChurnRole 汇集反复入场回收测试使用的角色与账号凭据。
type roomChurnRole struct {
	// token 记录令牌，使用文本表示。
	token string
	// key 记录键，使用角色键表示。
	key *pb.RoleKey
}

// 从正常建角结果复制基础字段，仅在测试库批量预置不同的账号、角色与授权令牌。
// seedRoomChurnRoles 在专属测试库生成换人场景需要的账号、角色和令牌前态。
// t 提供断言失败报告、子用例和清理注册，ctx 向下传递当前请求的截止时间和取消信号，store 记录持久层存储，count 记录数量，使用数值表示。
func seedRoomChurnRoles(t *testing.T, ctx context.Context, store *persistence.Store, count int) []roomChurnRole {
	t.Helper()
	// login 接收登录供后续步骤使用，err 接收登录过程中可能产生的错误，供后续失败分支处理。
	login, err := store.Login(ctx, "churn_"+domain.ID()[:10], "test_123456", true)
	must(t, err)
	// auth 保存由“authenticate”取得的授权。
	auth, err := store.Authenticate(ctx, login.AccountToken)
	must(t, err)
	// seed 保存由创建角色取得的预置数据。
	seed, err := store.CreateRole(ctx, auth, &pb.CreateRoleRequest{
		OriginServerId: store.Cfg.OriginID, Slot: 1, Job: 0, Name: "换人模板", OpId: domain.Bytes(domain.ID()),
	})
	must(t, err)
	// template 保存由角色取得的模板。
	template, err := store.Role(ctx, domain.Hex(seed.Key.RoleId))
	must(t, err)
	template.Items, template.Equipped = nil, nil
	// entries 创建用于维护“entries”的容器或通道。
	entries := make([]roomChurnRole, 0, count)
	// accounts 创建用于维护账号集合的容器或通道。
	accounts := make([]domain.Account, 0, count)
	// tokens 创建用于维护令牌集合的容器或通道。
	tokens := make([]persistence.Token, 0, count)
	// roles 创建用于维护角色集合的容器或通道。
	roles := make([]domain.Role, 0, count)
	// expires 保存由新增取得的过期集合。
	expires := time.Now().Add(10 * time.Minute)
	for len(entries) < count {
		// id 区分当前步骤指向的目标对象。
		id := domain.ID()
		// hash 保存由累加值“256”取得的摘要。
		hash := sha256.Sum256(domain.Bytes(id))
		if binary.BigEndian.Uint64(hash[:8])%8 != 0 {
			continue
		}
		// role 保存当前处理所用的角色。
		role := template
		role.ID, role.AccountID = id, domain.ID()
		role.Name = fmt.Sprintf("换人%03d", len(entries))
		role.NameKey = role.Name
		if len(entries) == 1 {
			role.HostingSeconds = 180
		}
		// token 保存根据标识与标识计算的令牌。
		token := domain.ID() + domain.ID()
		roles = append(roles, role)
		accounts = append(accounts, domain.Account{
			ID: role.AccountID, Name: fmt.Sprintf("churn_%03d", len(entries)), Active: true, AuthVersion: 1,
		})
		tokens = append(tokens, persistence.Token{ID: persistence.Hash([]byte(token)), AccountID: role.AccountID, AuthVersion: 1, Expires: expires})
		entries = append(entries, roomChurnRole{token: token, key: role.Summary().Key})
	}
	_, err = store.DB.Collection("accounts").InsertMany(ctx, accounts)
	must(t, err)
	_, err = store.DB.Collection("account_tokens").InsertMany(ctx, tokens)
	must(t, err)
	_, err = store.DB.Collection("roles").InsertMany(ctx, roles)
	must(t, err)
	return entries
}
