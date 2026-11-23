// 本文件核验共享城镇的作用域、入场发布、权威占格以及断连回收。
package game

import (
	"sync"
	"testing"

	pb "minimir/api/generated/minimirv1"
	"minimir/internal/devconfig"
	"minimir/internal/domain"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestSharedTownSameRunConfirmationAndIsolation 走正常入场确认命令验证角色只向同服同镇发布。
func TestSharedTownSameRunConfirmationAndIsolation(t *testing.T) {
	firstWorker, first, firstConn, _ := mapTravelWorker(t)
	engine := firstWorker.e
	engine.Store.Cfg.RunID = 1
	fixture := &engine.Store.Cfg.Fixture
	town, ok := fixture.MapByID(fixture.TownTravel.DestinationMapIDs[0])
	if !ok {
		t.Fatal("缺少正式紧凑城镇")
	}
	first.Role.Name = "第一位旅人"
	second, secondConn := sharedTownTestRoom(engine.Store, town, "second", "第二位旅人")
	thirdStore := &persistence.Store{Cfg: engine.Store.Cfg}
	thirdStore.Cfg.RunID = 2
	third, _ := sharedTownTestRoom(thirdStore, town, "other-run", "不同运行服")
	secondWorker := &worker{e: engine, rooms: map[string]*room{second.Role.ID: second, third.Role.ID: third}}

	firstTown := firstWorker.ensureTownWorld(first, town.ID)
	petLanding, petLandingOK := travelPetPoint(town, town.PlayerSpawn, nil, nil, 0)
	if !petLandingOK || !firstWorker.activateWorldAt(first, firstTown, town.PlayerSpawn, &petLanding, true) {
		t.Fatal("商旅前往紧凑城镇失败")
	}
	first.Ready = false
	if !secondWorker.attachSharedTown(second, town) || !secondWorker.attachSharedTown(third, town) {
		t.Fatal("其他角色入镇失败")
	}
	if first.Player.Pos == second.Player.Pos || first.Player.Pos != third.Player.Pos {
		t.Fatal("城镇初始占格没有按运行服隔离")
	}
	if peers := sharedTownTestPeers(t, firstWorker, first); len(peers) != 0 {
		t.Fatal("未确认入场的角色过早发布", peers)
	}
	secondWorker.command(command{role: second.Role.ID, conn: secondConn, frame: wire.Message(p.MsgC2SEnterWorldAck, 1, secondConn.Binding(),
		&pb.EnterWorldAck{SceneEpoch: second.Scene, BaselineId: second.Baseline + 1})})
	if second.Ready {
		t.Fatal("错误基线确认通过")
	}
	secondWorker.command(command{role: second.Role.ID, conn: secondConn, frame: wire.Message(p.MsgC2SEnterWorldAck, 2, secondConn.Binding(),
		&pb.EnterWorldAck{SceneEpoch: second.Scene, BaselineId: second.Baseline})})
	if peers := sharedTownTestPeers(t, firstWorker, first); len(peers) != 1 || peers[0].DisplayName != second.Role.Name || peers[0].DefinitionId != second.Role.Job {
		t.Fatal("同服同镇未发布已确认角色及其职业、名字", peers)
	}
	if peers := sharedTownTestPeers(t, secondWorker, third); len(peers) != 0 {
		t.Fatal("多个运行服的共享城镇串线", peers)
	}
	secondWorker.command(command{role: second.Role.ID, conn: secondConn, disconnect: true})
	if peers := sharedTownTestPeers(t, firstWorker, first); len(peers) != 0 || !firstWorker.townCellFree(first, &first.Player, second.Player.Pos) {
		t.Fatal("断线后共享席位或占格没有释放", peers)
	}
	firstWorker.command(command{role: first.Role.ID, conn: firstConn, disconnect: true})
	if len(engine.sharedTowns.towns) != 1 {
		t.Fatal("清空的运行服城镇未回收")
	}
}

// TestSharedTownConcurrentMoveCommitsOneWinner 验证两个 worker 同时走向同一格时只能成功一次。
func TestSharedTownConcurrentMoveCommitsOneWinner(t *testing.T) {
	firstWorker, _, _, now := mapTravelWorker(t)
	engine := firstWorker.e
	engine.Store.Cfg.RunID = 1
	f := &engine.Store.Cfg.Fixture
	town, ok := f.MapByID(f.TownTravel.DestinationMapIDs[0])
	if !ok {
		t.Fatal("缺少正式紧凑城镇")
	}
	first, _ := sharedTownTestRoom(engine.Store, town, "first", "先到者")
	second, _ := sharedTownTestRoom(engine.Store, town, "second", "后来者")
	secondWorker := &worker{e: engine}
	if !firstWorker.attachSharedTown(first, town) || !secondWorker.attachSharedTown(second, town) {
		t.Fatal("测试角色入镇失败")
	}
	target := town.PlayerSpawn
	for _, offset := range directions {
		point := devconfig.Point{X: target.X + offset.X, Y: target.Y + offset.Y}
		if point != second.Player.Pos && town.Walkable(point) {
			target = point
			break
		}
	}
	if target == first.Player.Pos || target == second.Player.Pos {
		t.Fatal("测试需要一个未占格")
	}
	if zone, inside := town.SafeZoneAt(target); inside {
		// 绑定安全区已经持久确认，本次并发测试只验证实际移动提交。
		binding := domain.RoleBinding{MapID: town.ID, X: int32(zone.Center.X), Y: int32(zone.Center.Y)}
		for _, r := range []*room{first, second} {
			r.Role.Binding, r.Role.BindingSourceSeq, r.Role.BindingSafeZoneID = &binding, 1, zone.ID
		}
	}
	// 两个独立房间的同时提交共享同一权威格表。
	start, results := make(chan struct{}), make(chan bool, 2)
	var group sync.WaitGroup
	for w, r := range map[*worker]*room{firstWorker: first, secondWorker: second} {
		group.Go(func() {
			<-start
			results <- w.commitWalk(r, &r.Player, target, 0, now, 300)
		})
	}
	close(start)
	group.Wait()
	if a, b := <-results, <-results; a == b || (first.Player.Pos == target) == (second.Player.Pos == target) {
		t.Fatal("格占用未在实际提交时原子仲裁")
	}
}

// sharedTownTestRoom 产生相互独立的执行代次和已绑定的测试连接。
func sharedTownTestRoom(store *persistence.Store, town *devconfig.MapTemplate, id, name string) (*room, *travelConnection) {
	conn := &travelConnection{alive: true, binding: 1}
	r := &room{runStore: store, Role: domain.Role{ID: id, Name: name, Job: 1, Level: 1}, Conn: conn,
		Auth: persistence.Auth{RoleID: id, SessionEpoch: 1}, Scene: 10, Baseline: 11,
		worldState: &worldState{MapID: town.ID}, Player: entity{Handle: 1, Kind: 1, Def: 1, Pos: town.PlayerSpawn, HP: 20, MaxHP: 20, Life: 1}}
	return r, conn
}

// sharedTownTestPeers 只读基线中用于观察远端角色的句柄。
func sharedTownTestPeers(t *testing.T, w *worker, r *room) []*pb.EntityState {
	t.Helper()
	chunks, err := w.buildBaseline(r, 0)
	if err != nil {
		t.Fatal(err)
	}
	var peers []*pb.EntityState
	for _, chunk := range chunks {
		for _, item := range chunk.Entities {
			if item.EntityHandle >= sharedTownHandleFirst {
				peers = append(peers, item)
			}
		}
	}
	return peers
}
