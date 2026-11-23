// 本文件用隔离数据库和真实 WebSocket 检查多人共享城镇及健康重连。
package integration

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/bootstrap"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestSharedTownMongoWS 验证原地图入口的双角色共享、首帧门禁、跨连接移动、断线和回挂机图。
func TestSharedTownMongoWS(t *testing.T) {
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" {
		t.Skip("需要已核验的专属 MongoDB 副本集")
	}
	cfg, err := devconfig.Load("../../../conf/dev/server.json")
	must(t, err)
	cfg.Database = "minimir_dev_game_test_st_" + domain.ID()
	cfg.AccountAddress, cfg.GameAddress = address(t), address(t)
	ctx, cancel := context.WithCancel(context.Background())
	store, err := persistence.Open(ctx, cfg)
	if err != nil {
		cancel()
		must(t, err)
	}
	var nodes sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		nodes.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer store.Close(cleanup)
		must(t, store.DB.Drop(cleanup))
	})
	for _, mode := range []string{"account", "game"} {
		nodes.Go(func() {
			if err := bootstrap.Run(ctx, cfg, mode); err != nil && ctx.Err() == nil {
				t.Errorf("隔离 %s 节点：%v", mode, err)
			}
		})
	}
	healthy(t, cfg.AccountAddress)
	healthy(t, cfg.GameAddress)
	endpoint := "http://" + cfg.AccountAddress + "/rpc"
	create := func(job uint32) (string, *pb.RoleSummary, *client) {
		t.Helper()
		login, role := &pb.LoginResult{}, &pb.RoleSummary{}
		rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "shared_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
		rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{
			OriginServerId: 1, Slot: 1, Job: job, Name: "同游" + domain.ID()[:6], OpId: domain.Bytes(domain.ID()),
		}, role, 0)
		connection := join(t, cfg.GameAddress, issue(t, endpoint, login.AccountToken, role.Key))
		connection.auto(false, 1)
		return login.AccountToken, role, connection
	}
	_, firstRole, first := create(0)
	defer first.close()
	secondToken, secondRole, second := create(1)
	defer func() { second.close() }()
	if townPeer(first, secondRole.Name) != nil || townPeer(second, firstRole.Name) != nil {
		t.Fatal("普通个人地图错误共享了其他角色")
	}
	targetID := cfg.Fixture.TownTravel.DestinationMapIDs[0]
	town, ok := cfg.Fixture.MapByID(targetID)
	if !ok {
		t.Fatal("缺少待验证的紧凑城镇")
	}
	travelToBaseline(first, 1, targetID)
	travelAcknowledge(first, true)
	travelToBaseline(second, 1, targetID)
	firstView := townPeer(second, firstRole.Name)
	if firstView == nil || !proto.Equal(firstView.Position, first.player().Position) || proto.Equal(second.player().Position, firstView.Position) {
		t.Fatal("共享首帧缺少已确认的角色，或服端分配了重复出生格")
	}
	travelUntilWithin(first, 3*time.Second, func(f wire.Frame) bool { return f.ID == p.MsgS2CSceneBaseline })
	if townPeer(first, secondRole.Name) != nil {
		t.Fatal("等待新基线确认的角色已被发布给他人")
	}
	travelAcknowledge(second, true)
	travelUntilWithin(first, 3*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && townPeer(first, secondRole.Name) != nil
	})
	before := townPeer(first, secondRole.Name)
	step := travelNearbyGround(t, town, second.scene, second.player().Position)
	second.send(p.MsgC2SMoveIntent, &pb.MoveIntent{SceneEpoch: second.entry.SceneEpoch, Destination: step})
	travelUntilWithin(first, 8*time.Second, func(f wire.Frame) bool {
		peer := townPeer(first, secondRole.Name)
		return f.ID == p.MsgS2CSceneBaseline && peer != nil && peer.EntityHandle == before.EntityHandle &&
			peer.MovementRevision > before.MovementRevision && proto.Equal(peer.Position, step)
	})
	lastScene := second.entry.SceneEpoch
	second.close()
	travelUntilWithin(first, 3*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && townPeer(first, secondRole.Name) == nil
	})
	second = join(t, cfg.GameAddress, issue(t, endpoint, secondToken, secondRole.Key))
	if second.entry.SceneEpoch != lastScene || second.entry.RoomId != targetID || !proto.Equal(second.player().Position, step) {
		t.Fatal("健康重连未恢复原城镇场景和位置")
	}
	travelUntilWithin(first, 3*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && townPeer(first, secondRole.Name) != nil
	})
	travelToBaseline(second, 2, cfg.Fixture.Map.ID)
	travelAcknowledge(second, false)
	if townPeer(second, firstRole.Name) != nil {
		t.Fatal("离开城镇后个人地图继续显示其他角色")
	}
	travelUntilWithin(first, 3*time.Second, func(f wire.Frame) bool {
		return f.ID == p.MsgS2CSceneBaseline && townPeer(first, secondRole.Name) == nil
	})
}

// townPeer 从最近一帧读取具有原角色显示名称的远端化身。
func townPeer(c *client, name string) *pb.EntityState {
	for _, body := range c.scene.GetEntities() {
		if body.EntityKind == 1 && body.EntityHandle != c.entry.PlayerHandle && body.DisplayName == name {
			return body
		}
	}
	return nil
}
