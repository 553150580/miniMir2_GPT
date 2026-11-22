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
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

func TestDormantRoomChurnAndHealthyReconnectMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("显式开启独立 MongoDB 验证房间回收和真实持续换人")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_churn_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	// 冻结隔离怪物的攻击与移动，便于独立断言在线和托管房间保留。
	for i := range cfg.Fixture.Monsters {
		monster := &cfg.Fixture.Monsters[i]
		monster.HP, monster.MoveMS, monster.AttackMin, monster.AttackMax = 1000, 120000, 0, 0
	}
	must(t, cfg.Fixture.Validate())
	ctx, cancel := context.WithTimeout(t.Context(), 115*time.Second)
	defer cancel()
	store, err := persistence.Open(ctx, cfg)
	must(t, err)
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		must(t, store.DB.Drop(cleanup))
		store.Close(cleanup)
	})
	// 每名测试角色独享账号与票据，并映射到同一分片，命中默认256间房的准入上限。
	const capacity = 256
	roles := seedRoomChurnRoles(t, ctx, store, capacity+2)
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("换人验收%s节点：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	enter := func(i int) *client {
		return join(t, cfg.GameAddress, issue(t, endpoint, roles[i].token, roles[i].key))
	}

	dormant := enter(0)
	dormant.auto(false, 1)
	claim := dormant.asset(p.MsgC2SClaimHostingTime, &pb.ClaimHostingTimeRequest{Operation: dormant.operation()})
	if claim.ErrorCode != 0 {
		t.Fatalf("预置角色领取长期余额失败：%v", claim)
	}
	dormant.inventoryAt(claim.NewRevision)
	persisted, err := store.Role(ctx, domain.Hex(roles[0].key.RoleId))
	must(t, err)
	if persisted.HostingSeconds <= 0 {
		t.Fatal("托管日领未形成永久余额")
	}
	spawn := proto.Clone(dormant.player().Position).(*pb.GridPoint)
	goal := &pb.GridPoint{X: spawn.X + 1, Y: spawn.Y}
	dormant.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: dormant.entry.SceneEpoch, Destination: goal})
	travelUntilWithin(dormant, 4*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && proto.Equal(dormant.player().Position, goal)
	})
	dormantScene, dormantLife := dormant.entry.SceneEpoch, dormant.player().Life
	dormantHP := dormant.player().Hp
	dormant.close()
	dormant = enter(0)
	if dormant.entry.SceneEpoch != dormantScene || dormant.player().Life != dormantLife ||
		dormant.player().Hp != dormantHP || !proto.Equal(dormant.player().Position, goal) {
		t.Fatal("健康窗口内重连未保留原场景、生命或坐标")
	}
	dormant.close()

	hosted := enter(1)
	hosted.auto(true, 1)
	hostedScene := hosted.entry.SceneEpoch
	hosted.close()
	online := enter(2)
	defer online.close()
	const heartbeatRequest = 9999
	heartbeat := make(chan error, 1)
	go func() {
		for {
			_, raw, err := online.ws.Read(ctx)
			if err != nil {
				heartbeat <- err
				return
			}
			frame, err := wire.Decode(raw)
			if err != nil || frame.ID == p.MsgS2CHeartbeat && frame.Request == heartbeatRequest && frame.Error == 0 {
				heartbeat <- err
				return
			}
		}
	}()
	for i := 3; i < capacity; i++ {
		enter(i).close()
	}
	ticket := issue(t, endpoint, roles[capacity].token, roles[capacity].key)
	assertEntryRejectionFrame(t, cfg.GameAddress, ticket, p.CodeServerBusy)
	t.Log("同一分片满额且回收窗口未到，真实新角色收到结构化容量拒绝")

	// 等待真实 worker 时钟越过最后一个空闲房间的回收窗口。
	timer := time.NewTimer(time.Minute + time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("等待真实回收窗口截止失败")
	}
	for i := capacity; i < len(roles); i++ {
		enter(i).close()
	}
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
	case err := <-heartbeat:
		must(t, err)
	case <-ctx.Done():
		t.Fatal("持续在线的原连接缺少正常心跳")
	}
	t.Logf("同一分片累计准入%d个独立角色；离线超期让出新槽位，健康重连、托管余额与在线连接通过", len(roles))
}

type roomChurnRole struct {
	token string
	key   *pb.RoleKey
}

// 从正常建角结果复制基础字段，仅在测试库批量预置不同的账号、角色与授权令牌。
func seedRoomChurnRoles(t *testing.T, ctx context.Context, store *persistence.Store, count int) []roomChurnRole {
	t.Helper()
	login, err := store.Login(ctx, "churn_"+domain.ID()[:10], "test_123456", true)
	must(t, err)
	auth, err := store.Authenticate(ctx, login.AccountToken)
	must(t, err)
	seed, err := store.CreateRole(ctx, auth, &pb.CreateRoleRequest{
		OriginServerId: store.Cfg.OriginID, Slot: 1, Job: 0, Name: "换人模板", OpId: domain.Bytes(domain.ID()),
	})
	must(t, err)
	template, err := store.Role(ctx, domain.Hex(seed.Key.RoleId))
	must(t, err)
	template.Items, template.Equipped = nil, nil
	entries := make([]roomChurnRole, 0, count)
	accounts := make([]domain.Account, 0, count)
	tokens := make([]persistence.Token, 0, count)
	roles := make([]domain.Role, 0, count)
	expires := time.Now().Add(10 * time.Minute)
	for len(entries) < count {
		id := domain.ID()
		hash := sha256.Sum256(domain.Bytes(id))
		if binary.BigEndian.Uint64(hash[:8])%8 != 0 {
			continue
		}
		role := template
		role.ID, role.AccountID = id, domain.ID()
		role.Name = fmt.Sprintf("换人%03d", len(entries))
		role.NameKey = role.Name
		if len(entries) == 1 {
			role.HostingSeconds = 180
		}
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
