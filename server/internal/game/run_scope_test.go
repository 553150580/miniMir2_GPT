// 本文件验证多个运行服共享同一引擎资源，且入场和房间不能借用其他运行服视图。
package game

import (
	"context"
	"testing"

	"minimir/internal/devconfig"
	"minimir/internal/persistence"
	p "minimir/internal/protocol"
	"minimir/internal/wire"
)

// TestRunEngineSharesResourcesAndRoutesTicket 检查同进程票据的运行服、节点和授权代次。
// t 汇报测试断言与子用例结果。
func TestRunEngineSharesResourcesAndRoutesTicket(t *testing.T) {
	// primary 和 secondary 代表同一游戏进程获得的两个运行服授权。
	primary := &persistence.Store{Cfg: devconfig.Config{RunID: 1, NodeID: "game"}, Instance: "same-process", Epoch: 8}
	secondary := &persistence.Store{Cfg: devconfig.Config{RunID: 2, NodeID: "game"}, Instance: "same-process", Epoch: 3}
	// e 是两个运行服共用的引擎，测试结束时没有网络连接需要排空。
	e := NewRunEngine(context.Background(), []*persistence.Store{primary, secondary})
	t.Cleanup(func() {
		e.cancel()
		e.wg.Wait()
	})
	if len(e.workers) != workerCount || cap(e.settlementJobs) != workerCount*maxRoomsPerWorker || len(e.runStores) != 2 {
		t.Fatal("每个运行服应使用同一套分片、数据库队列与房间预算")
	}
	// ticket 应选中第二个运行服的授权，即使预查入口使用第一个视图。
	ticket := persistence.Ticket{Run: 2, Owner: secondary.Instance, GameNodeID: secondary.Cfg.NodeID, Epoch: secondary.Epoch}
	if e.Store != primary || e.storeForTicket(ticket) != secondary {
		t.Fatal("未按票据选择指定运行服的视图")
	}
	// bad 依次提供未承载、错代次、错实例、错节点的已读票据。
	for _, bad := range []persistence.Ticket{
		{Run: 3, Owner: ticket.Owner, GameNodeID: ticket.GameNodeID, Epoch: ticket.Epoch},
		{Run: ticket.Run, Owner: ticket.Owner, GameNodeID: ticket.GameNodeID, Epoch: ticket.Epoch + 1},
		{Run: ticket.Run, Owner: "another-process", GameNodeID: ticket.GameNodeID, Epoch: ticket.Epoch},
		{Run: ticket.Run, Owner: ticket.Owner, GameNodeID: "another-node", Epoch: ticket.Epoch},
	} {
		if e.storeForTicket(bad) != nil {
			t.Fatalf("错误范围的票据获得了运行服视图：%+v", bad)
		}
	}
}

// TestEntryCannotReuseRoomAcrossRunViews 验证换运行服的票据不能接管或排队使用旧房间。
// t 汇报测试结果。
func TestEntryCannotReuseRoomAcrossRunViews(t *testing.T) {
	// oldRun 和 otherRun 是同一进程承载的不同运行服。
	oldRun := &persistence.Store{Cfg: devconfig.Config{RunID: 1}}
	otherRun := &persistence.Store{Cfg: devconfig.Config{RunID: 2}}
	// e 提供独立入场队列；w 是手工装配的分片，直接处理测试命令。
	e := &Engine{Store: oldRun, admissionEntries: make(chan job, 1)}
	w := &worker{e: e, rooms: make(map[string]*room)}
	// old 保留一个已有活动场景的角色房间，正在等待原运行服的结算确认。
	old := &room{runStore: oldRun, Scene: 1, Pending: true}
	w.rooms["role"] = old
	// conn 是另一运行服短票对应的新连接，frames 捕获返回的拒绝。
	conn := &travelConnection{alive: true}
	w.command(command{
		role: "role", conn: conn, runStore: otherRun, ticket: &persistence.Ticket{Run: 2},
		frame: wire.Frame{Request: 19},
	})
	if w.rooms["role"] != old || w.roomStore(old) != oldRun || len(old.Waiting) != 0 || len(e.admissionEntries) != 0 {
		t.Fatal("其他运行服票据影响了原房间或等待队列")
	}
	if len(conn.frames) != 1 || conn.frames[0].ID != p.MsgS2CEnterLine ||
		conn.frames[0].Request != 19 || conn.frames[0].Error != p.CodeRouteChanged || conn.Alive() {
		t.Fatalf("未给新连接返回明确的路由失效结果：%+v", conn.frames)
	}
}

// TestEntryDatabaseJobUsesRoomRun 验证入场写任务采用新房间的运行服视图。
// t 汇报测试结果。
func TestEntryDatabaseJobUsesRoomRun(t *testing.T) {
	// primary 是默认预查入口，target 是票据所在的另一个运行服。
	primary := &persistence.Store{Cfg: devconfig.Config{RunID: 1, NodeID: "game"}, Instance: "same-process", Epoch: 8}
	target := &persistence.Store{Cfg: devconfig.Config{RunID: 2, NodeID: "game"}, Instance: primary.Instance, Epoch: 3}
	// e 提供待执行队列，w 可依次运行分片接收、数据库调用及确认回调。
	e := &Engine{Store: primary, ctx: context.Background(), admissionEntries: make(chan job, 1)}
	w := &worker{e: e, rooms: make(map[string]*room)}
	// conn 捕获新房间的业务响应，ticket 保留正确的运行服边界但故意缺少可提交编号。
	conn := &travelConnection{alive: true}
	ticket := &persistence.Ticket{Run: 2, Owner: target.Instance, GameNodeID: target.Cfg.NodeID, Epoch: target.Epoch}
	w.command(command{role: "new-role", conn: conn, ticket: ticket, runStore: target})
	// created 是待确认的冷房间，应当固定第二个视图直到房间释放。
	created := w.rooms["new-role"]
	if created == nil || w.roomStore(created) != target || len(e.admissionEntries) != 1 {
		t.Fatal("入场未绑定票据选择的运行服或未创建待确认任务")
	}
	// entry 是原准入任务。正确视图会通过运行服校验，再因编号缺失停止，此路径无需连接数据库。
	entry := <-e.admissionEntries
	entry.run()(w)
	if len(conn.frames) != 1 || conn.frames[0].Error != p.CodeInvalidArgument || w.rooms["new-role"] != nil {
		t.Fatalf("入场事务未使用选中的视图或未清理拒绝的冷房间：%+v", conn.frames)
	}
}
