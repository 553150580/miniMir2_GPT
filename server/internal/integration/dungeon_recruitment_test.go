// 本文件在真实账号服、游戏服和独立 MongoDB 上验证讨伐链的招募身份、过滤与掉线行为。
package integration

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/protobuf/proto"
	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
)

// TestDungeonRecruitmentMongoWS 验证同进程两服隔离、三职业席位、招募版本和队长重新附着。
func TestDungeonRecruitmentMongoWS(t *testing.T) {
	store, endpoint, east, west := startDungeonRecruitmentServer(t, true)
	leader := newDungeonRecruit(t, endpoint, east, 0)
	mage := newDungeonRecruit(t, endpoint, east, 1)
	secondMage := newDungeonRecruit(t, endpoint, east, 1)
	taoist := newDungeonRecruit(t, endpoint, east, 2)
	otherRun := newDungeonRecruit(t, endpoint, west, 2)
	captain, guest := leader.attach(t), mage.attach(t)
	duplicate, third, outsider := secondMage.attach(t), taoist.attach(t), otherRun.attach(t)
	initial := dungeonPartyRPC(captain, &pb.DungeonPartyRequest{Action: 1})
	if initial.ErrorCode != 0 || len(initial.Templates) != 1 || initial.Templates[0].TemplateId != 3001 || initial.Templates[0].DurationSeconds != 300 {
		t.Fatal("没有读到由独立配置生成的正式招募字段", initial)
	}
	oldScene := captain.send(p.MsgC2SDungeonPartyRequest, &pb.DungeonPartyRequest{Action: 1, SceneEpoch: captain.entry.SceneEpoch ^ 1})
	travelExpectError(captain, p.MsgC2SDungeonPartyRequest, oldScene, p.CodeLineRecovering)
	id := domain.Bytes(domain.ID())
	create := &pb.DungeonPartyRequest{Action: 2, PartyId: id, TemplateId: 3001, MinimumLevel: 1, AllowedJobsMask: 6}
	formed := requireDungeonParty(t, dungeonPartyRPC(captain, create), id, 1)
	if !bytes.Equal(formed.LeaderRoleId, leader.summary.Key.RoleId) || formed.Members[0].Job != 0 {
		t.Fatal("招募条件应只限制新队友，发起人身份以附着角色为准", formed)
	}
	if retry := requireDungeonParty(t, dungeonPartyRPC(captain, create), id, 1); !proto.Equal(retry, formed) {
		t.Fatal("同一原编号创建重试改变了招募", retry)
	}
	outside := dungeonPartyRPC(outsider, &pb.DungeonPartyRequest{Action: 1})
	if len(outside.Parties) != 0 || outside.OwnParty != nil || dungeonPartyRPC(outsider, &pb.DungeonPartyRequest{Action: 3, PartyId: id, ExpectedRevision: formed.Revision}).ErrorCode == 0 {
		t.Fatal("另一运行服能够观察或加入原招募", outside)
	}
	found := dungeonPartyRPC(guest, &pb.DungeonPartyRequest{Action: 1})
	if len(found.Parties) != 1 || !proto.Equal(found.Parties[0], formed) {
		t.Fatal("同服队友没有读到原名单和修订", found)
	}
	joined := requireDungeonParty(t, dungeonPartyRPC(guest, &pb.DungeonPartyRequest{Action: 3, PartyId: id, ExpectedRevision: formed.Revision}), id, 2)
	if joined.Revision <= formed.Revision || joined.Members[1].Job != 1 {
		t.Fatal("加入后没有产生新的名单修订", joined)
	}
	if blocked := dungeonPartyRPC(duplicate, &pb.DungeonPartyRequest{Action: 3, PartyId: id, ExpectedRevision: joined.Revision}); blocked.ErrorCode == 0 || blocked.OwnParty != nil {
		t.Fatal("重复职业越过招募条件", blocked)
	}
	if stale := dungeonPartyRPC(third, &pb.DungeonPartyRequest{Action: 3, PartyId: id, ExpectedRevision: formed.Revision}); stale.ErrorCode == 0 {
		t.Fatal("旧修订改变了当前招募名单", stale)
	}
	full := requireDungeonParty(t, dungeonPartyRPC(third, &pb.DungeonPartyRequest{Action: 3, PartyId: id, ExpectedRevision: joined.Revision}), id, 3)
	if len(dungeonPartyRPC(duplicate, &pb.DungeonPartyRequest{Action: 1}).Parties) != 0 {
		t.Fatal("已满员的房间仍出现在公开招募")
	}
	if bad := dungeonPartyRPC(guest, &pb.DungeonPartyRequest{Action: 5, PartyId: id, ExpectedRevision: full.Revision, TargetRoleId: taoist.summary.Key.RoleId}); bad.ErrorCode == 0 {
		t.Fatal("普通成员被错误授予踢人权限", bad)
	}
	kicked := requireDungeonParty(t, dungeonPartyRPC(captain, &pb.DungeonPartyRequest{Action: 5, PartyId: id, ExpectedRevision: full.Revision,
		TargetRoleId: taoist.summary.Key.RoleId}), id, 2)
	if dungeonPartyRPC(third, &pb.DungeonPartyRequest{Action: 1}).OwnParty != nil {
		t.Fatal("被队长踢出的角色仍占席")
	}
	updated := requireDungeonParty(t, dungeonPartyRPC(captain, &pb.DungeonPartyRequest{Action: 6, PartyId: id, ExpectedRevision: kicked.Revision,
		MinimumLevel: 1, AllowedJobsMask: 6, AllowSameJob: true}), id, 2)
	requireDungeonParty(t, dungeonPartyRPC(duplicate, &pb.DungeonPartyRequest{Action: 3, PartyId: id, ExpectedRevision: updated.Revision}), id, 3)

	// 先确认队长离线并记下服务端期限，再用同角色新票恢复原房间与三席。
	oldBinding := captain.binding
	captain.close()
	offline := waitDungeonParty(t, guest, func(q *pb.DungeonPartyView) bool { return q != nil && q.LeaderOfflineUntilUnixMs > 0 })
	remaining := time.Until(time.UnixMilli(offline.LeaderOfflineUntilUnixMs))
	if remaining <= 55*time.Second || remaining > time.Minute+time.Second || offline.Members[0].Online {
		t.Fatal("队长断线期限、成员在线状态或原队伍不一致", offline, remaining)
	}
	captain = leader.attach(t)
	recovered := waitDungeonParty(t, captain, func(q *pb.DungeonPartyView) bool { return q != nil && q.LeaderOfflineUntilUnixMs == 0 })
	if captain.binding <= oldBinding || len(recovered.Members) != 3 || !bytes.Equal(recovered.PartyId, id) || !recovered.Members[0].Online {
		t.Fatal("健康新票未恢复队长和原招募", oldBinding, captain.binding, recovered)
	}
	// 再次断线不恢复，维护连接并等待服务端按同一原期限自动解散。
	captain.close()
	offline = waitDungeonParty(t, guest, func(q *pb.DungeonPartyView) bool { return q != nil && q.LeaderOfflineUntilUnixMs > 0 })
	deadline := time.UnixMilli(offline.LeaderOfflineUntilUnixMs).Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state := dungeonPartyRPC(guest, &pb.DungeonPartyRequest{Action: 1})
		if state.OwnParty == nil {
			if time.Now().Before(time.UnixMilli(offline.LeaderOfflineUntilUnixMs)) || len(state.Parties) != 0 {
				t.Fatal("原队伍被提前解散或泄漏到公开目录", state)
			}
			count, err := store.DB.Collection("dungeon_challenges").CountDocuments(t.Context(), bson.M{})
			must(t, err)
			if count != 0 {
				t.Fatal("仅招募及离线解散不应产生永久开战扣次", count)
			}
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("队长断线超过服务端原 60 秒期限后队伍仍存在")
}

// TestDungeonRecruitmentDefaultClosedMongoWS 核对 18080 同源配置未开放无批准模板的入口。
func TestDungeonRecruitmentDefaultClosedMongoWS(t *testing.T) {
	_, endpoint, east, _ := startDungeonRecruitmentServer(t, false)
	c := newDungeonRecruit(t, endpoint, east, 0).attach(t)
	if result := dungeonPartyRPC(c, &pb.DungeonPartyRequest{Action: 1}); result.ErrorCode != p.CodeLineForbidden || len(result.Templates) != 0 {
		t.Fatal("默认规则暴露未审定的讨伐链", result)
	}
}

// dungeonRecruit 保存同一次注册的身份和区服，健康重连重新走签票。
type dungeonRecruit struct {
	endpoint string             // 隔离账号服地址。
	token    string             // 本角色所属账号的原登录权限。
	route    directoryTestEntry // 原永久入口及当前运行服路由。
	summary  *pb.RoleSummary    // 已创建的永久角色。
}

func newDungeonRecruit(t *testing.T, endpoint string, route directoryTestEntry, job uint32) dungeonRecruit {
	t.Helper()
	login, summary := &pb.LoginResult{}, &pb.RoleSummary{}
	rpc(t, endpoint, "", p.MsgC2SLocalRegister, &pb.LocalLoginRequest{Account: "dparty_" + domain.ID()[:8], Password: "test_123456"}, login, 0)
	rpc(t, endpoint, login.AccountToken, p.MsgC2SCreateRole, &pb.CreateRoleRequest{OriginServerId: route.origin.OriginID, Slot: 1, Job: job,
		Name: "招募" + domain.ID()[:6], OpId: domain.Bytes(domain.ID())}, summary, 0)
	return dungeonRecruit{endpoint: endpoint, token: login.AccountToken, route: route, summary: summary}
}

func (player dungeonRecruit) attach(t *testing.T) *client {
	t.Helper()
	ticket := directoryTicket(t, player.endpoint, player.token, player.summary.Key, player.route)
	c := join(t, directoryTicketAddress(t, ticket), ticket.Ticket)
	t.Cleanup(c.close)
	c.auto(false, 1)
	return c
}

// dungeonPartyRPC 读取同场景与同请求序号的正式招募回执，保留 transport 的明确拒绝码。
func dungeonPartyRPC(c *client, q *pb.DungeonPartyRequest) *pb.DungeonPartyState {
	c.t.Helper()
	q.SceneEpoch = c.entry.SceneEpoch
	request := c.send(p.MsgC2SDungeonPartyRequest, q)
	for {
		frame := c.readFrame()
		if frame.Request != request {
			continue
		}
		if frame.Error != 0 {
			return &pb.DungeonPartyState{SceneEpoch: q.SceneEpoch, ErrorCode: frame.Error}
		}
		result := &pb.DungeonPartyState{}
		must(c.t, proto.Unmarshal(frame.Payload, result))
		if frame.ID != p.MsgS2CDungeonPartyState || result.SceneEpoch != q.SceneEpoch {
			c.t.Fatal("招募响应不属于原场景与请求", frame, result)
		}
		return result
	}
}

func requireDungeonParty(t *testing.T, state *pb.DungeonPartyState, id []byte, size int) *pb.DungeonPartyView {
	t.Helper()
	if state.ErrorCode != 0 || state.OwnParty == nil || !bytes.Equal(id, state.OwnParty.PartyId) || len(state.OwnParty.Members) != size {
		t.Fatal("原队伍身份或成员数不一致", state)
	}
	return state.OwnParty
}

func waitDungeonParty(t *testing.T, c *client, ready func(*pb.DungeonPartyView) bool) *pb.DungeonPartyView {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		state := dungeonPartyRPC(c, &pb.DungeonPartyRequest{Action: 1})
		if state.ErrorCode != 0 {
			t.Fatal("查招募状态失败", state)
		}
		if ready(state.OwnParty) {
			return state.OwnParty
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("原队伍没有推进连接状态")
	return nil
}

// startDungeonRecruitmentServer 装配同一账号服和一进程双运行服，仅隔离服安装有四段的测试模板。
func startDungeonRecruitmentServer(t *testing.T, enabled bool, arrange ...func(*devconfig.Config)) (*persistence.Store, string, directoryTestEntry, directoryTestEntry) {
	t.Helper()
	store, endpoint, east, west, _ := startDungeonServer(t, enabled, arrange...)
	return store, endpoint, east, west
}

// startDungeonServer 额外提供同运行服的受控停止入口，用于验证原挑战停止与恢复。
func startDungeonServer(t *testing.T, enabled bool, arrange ...func(*devconfig.Config)) (*persistence.Store, string, directoryTestEntry, directoryTestEntry, func()) {
	t.Helper()
	if os.Getenv("MINIMIR_MONGO_E2E") != "1" || os.Getenv("MINIMIR_INTEGRATION_TEST_MONGO_URI") == "" {
		t.Skip("需要已核验的独立 MongoDB 副本集")
	}
	cfg, err := loadIntegrationConfig()
	must(t, err)
	cfg.Database, cfg.AccountAddress, cfg.GameAddress = "minimir_dev_game_test_dparty_"+domain.ID(), address(t), address(t)
	cfg.OriginID, cfg.RunID, cfg.RunServerIDs = 1, 1, []uint64{1, 2}
	east := makeDirectoryEntry(1, 1, cfg, "dungeon-test", "讨伐联机", "一服", 0, 0)
	west := makeDirectoryEntry(2, 2, cfg, "dungeon-test", "讨伐联机", "二服", 0, 1)
	cfg.OriginRoutes = []devconfig.OriginRouteSeed{east.origin, west.origin}
	cfg.RunServers = []devconfig.RunServerSeed{east.assignment, west.assignment}
	if enabled {
		trial := devconfig.DungeonDefinition{Enabled: true, TemplateID: 3001, DisplayName: "隔离讨伐链", MinimumLevel: 1, DurationMS: 300_000,
			Map: devconfig.MapTemplate{ID: 30101, SceneKind: devconfig.SceneKindDungeon, Width: 16, Height: 6}}
		for offset, kind := range []devconfig.DungeonStageKind{devconfig.DungeonStageMobsFirst, devconfig.DungeonStageMobsSecond, devconfig.DungeonStageElite, devconfig.DungeonStageFinalBoss} {
			x := offset * 4
			trial.Stages = append(trial.Stages, devconfig.DungeonStageDefinition{Kind: kind,
				Area:         devconfig.DungeonArea{Min: devconfig.Point{X: x, Y: 1}, Max: devconfig.Point{X: x + 3, Y: 4}},
				PlayerSpawns: []devconfig.Point{{X: x, Y: 1}, {X: x, Y: 2}, {X: x, Y: 3}},
				Enemies:      []devconfig.DungeonEnemyDefinition{{MonsterID: cfg.Fixture.ChallengeMonsters[0].ID, Spawn: devconfig.Point{X: x + 1, Y: 2}}}})
		}
		cfg.Fixture.DungeonReleases = []devconfig.DungeonDefinition{trial}
	}
	for _, configure := range arrange {
		configure(&cfg)
	}
	must(t, cfg.Fixture.Validate())
	store, err := persistence.Open(t.Context(), cfg)
	must(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		defer store.Close(cleanup)
		must(t, store.DB.Drop(cleanup))
	})
	startDurableWorldBossNode(t, cfg, "account", cfg.AccountAddress)
	stopGame := startDurableWorldBossNode(t, cfg, "game", cfg.GameAddress)
	return store, "http://" + cfg.AccountAddress + "/rpc", east, west, stopGame
}
