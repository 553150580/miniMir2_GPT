"""只在隔离压测构建中替换接入配额并采样，仓库运行路径保持原样。"""
import json
import argparse
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
SERVER = ROOT / "server"
SOURCE = Path(__file__).resolve().parent
WORK = ROOT / ".work" / "hosting-load"
replace = {}


def change(path, edits):
    original = SERVER / path
    source = original.read_text(encoding="utf-8")
    for old, new in edits:
        if source.count(old) != 1:
            raise RuntimeError(f"工作区与压测注入锚点不匹配 {original.name}")
        source = source.replace(old, new)
    replace[original] = source


change("internal/transport/ws.go", [
    ("func (c *connection) DeliverAndClose(f wire.Frame, _ string) {\n\tif c.Alive() && c.ending.CompareAndSwap(false, true) {", """var HostingLoadObserveTerminal func(string)

func (c *connection) DeliverAndClose(f wire.Frame, reason string) {
	if c.Alive() && c.ending.CompareAndSwap(false, true) {
		if HostingLoadObserveTerminal != nil { HostingLoadObserveTerminal(reason) }"""),
    ("func (c *connection) Close(_ string) {\n\tif c.closed.CompareAndSwap(false, true) {", """var HostingLoadObserveClose func(string)

func (c *connection) Close(reason string) {
	if c.closed.CompareAndSwap(false, true) {
		if HostingLoadObserveClose != nil { HostingLoadObserveClose(reason) }"""),
    ("func (s *Server) ws(", "var HostingLoadConnectionLimit int64 = 128\n\nfunc (s *Server) ws("),
    ("if s.active.Add(1) > 128 {", "if s.active.Add(1) > HostingLoadConnectionLimit {"),
])
change("internal/game/runtime.go", [
    ("const maxRoomsPerWorker = 256", "var HostingLoadRoomLimit = 256"),
    ("workerCount*maxRoomsPerWorker", "workerCount*HostingLoadRoomLimit"),
    ("func New(ctx context.Context,", """type HostingLoadQueueDepth struct {
	Business int `json:"business"`
	Settlement int `json:"settlement"`
	Ticket int `json:"ticket"`
	Entry int `json:"entry"`
}

type HostingLoadRoomState struct {
	Total int `json:"total"`
	Online int `json:"online"`
	OnlineAutoAlive int `json:"online_auto_alive"`
	Managed int `json:"managed"`
	Inactive int `json:"inactive"`
	Joining int `json:"joining"`
	Isolated int `json:"isolated"`
	Pending int `json:"pending"`
}

var HostingLoadObserveTick func(shard int, at time.Time, rooms HostingLoadRoomState, roomsSampled bool, queued, capacity HostingLoadQueueDepth, elapsed, lag time.Duration)
var HostingLoadQueueFull func(lane string)
var HostingLoadRoomCreated func()
var HostingLoadRoomEvicted func()

func New(ctx context.Context,"""),
    ("type worker struct {", "type worker struct {\n\thostingLoadShard int\n\thostingLoadRoomSampleAt time.Time"),
    ("w := &worker{e: e, commands:", "w := &worker{hostingLoadShard: i, e: e, commands:"),
    ("if len(w.rooms) >= maxRoomsPerWorker {", "if len(w.rooms) >= HostingLoadRoomLimit {"),
    ("w.rooms[c.role] = r", "w.rooms[c.role] = r\n\t\t\tif HostingLoadRoomCreated != nil { HostingLoadRoomCreated() }"),
    ("if !offerDatabaseJob(e.admissionTickets, j) {", 'if !auditDatabaseOffer(e.admissionTickets, j, "ticket") {'),
    ("if !c.tick.IsZero() {\n\t\t\t\tw.begin", """if !c.tick.IsZero() {
				started := time.Now()
				var rooms HostingLoadRoomState
				sampleRooms := HostingLoadObserveTick != nil && c.tick.Sub(w.hostingLoadRoomSampleAt) >= time.Second
				if sampleRooms {
					w.hostingLoadRoomSampleAt = c.tick
				}
				w.begin"""),
    ("w.expireIdleRoom(roleID, r, c.tick)", """if !w.expireIdleRoom(roleID, r, c.tick) && sampleRooms {
						w.observeLoadRoom(&rooms, r)
					}"""),
    ("w.endTownNavigationTick()\n\t\t\t\tcontinue", """w.endTownNavigationTick()
				if HostingLoadObserveTick != nil {
					e := w.e
					queued := HostingLoadQueueDepth{len(e.jobs), len(e.settlementJobs), len(e.admissionTickets), len(e.admissionEntries)}
					capacity := HostingLoadQueueDepth{cap(e.jobs), cap(e.settlementJobs), cap(e.admissionTickets), cap(e.admissionEntries)}
					HostingLoadObserveTick(w.hostingLoadShard, c.tick, rooms, sampleRooms, queued, capacity, time.Since(started), started.Sub(c.tick))
				}
				continue"""),
])
change("internal/game/room_lifecycle.go", [
    ("delete(w.rooms, roleID)\n\treturn true", """delete(w.rooms, roleID)
	if HostingLoadRoomEvicted != nil {
		HostingLoadRoomEvicted()
	}
	return true"""),
    ("func (w *worker) retainRoom(", """// 压测从同一个房间 worker 采样生命周期，避免读取正在变更的地图或连接。
func (w *worker) observeLoadRoom(count *HostingLoadRoomState, r *room) {
	count.Total++
	if r.Pending || r.PendingRetry != nil || r.PendingOperationID != "" {
		count.Pending++
	}
	switch {
	case r.Isolated:
		count.Isolated++
	case r.Conn != nil && r.Conn.Alive() && r.Ready:
		count.Online++
		if r.Auto && r.Player.HP > 0 {
			count.OnlineAutoAlive++
		}
	case r.Scene == 0 || r.Conn != nil:
		count.Joining++
	case r.Auto && r.Player.HP > 0 && r.Role.HostingSeconds > 0:
		if m, ok := w.currentMap(r); ok && m.SceneKindValue() == devconfig.SceneKindPersonal {
			count.Managed++
			return
		}
		count.Inactive++
	default:
		count.Inactive++
	}
}

func (w *worker) retainRoom("""),
])
change("internal/game/database_jobs.go", [
    ("func (e *Engine) submitEntry(j job) bool {", """func auditDatabaseOffer(queue chan<- job, j job, lane string) bool {
	ok := offerDatabaseJob(queue, j)
	if !ok && HostingLoadQueueFull != nil {
		HostingLoadQueueFull(lane)
	}
	return ok
}

func (e *Engine) submitEntry(j job) bool {"""),
    ("return offerDatabaseJob(queue, j)\n}\n\nfunc (w *worker) submit", 'return auditDatabaseOffer(queue, j, "entry")\n}\n\nfunc (w *worker) submit'),
    ("queue = w.e.settlementJobs\n\t}\n\treturn offerDatabaseJob(queue, j)", """return auditDatabaseOffer(w.e.settlementJobs, j, "settlement")
	}
	return auditDatabaseOffer(queue, j, "business")"""),
])
change("internal/game/combat.go", [
    ("func (w *worker) settleInWorldReceipt(", """var HostingLoadObserveReward func(time.Duration)

func (w *worker) settleInWorldReceipt("""),
    ("r.SettlementGeneration++\n", "started := time.Now()\n\tr.SettlementGeneration++\n"),
    ("w.finishSettlementReceipt(binding, q, role, receipt, err, &j, finished)", """if HostingLoadObserveReward != nil && q.Kills > 0 && err == nil {
				HostingLoadObserveReward(time.Since(started))
			}
			w.finishSettlementReceipt(binding, q, role, receipt, err, &j, finished)"""),
])
change("internal/persistence/store.go", [
    ('"go.mongodb.org/mongo-driver/v2/bson"', '"go.mongodb.org/mongo-driver/v2/bson"\n\t"go.mongodb.org/mongo-driver/v2/event"'),
    ("func Open(ctx context.Context, cfg devconfig.Config)", """var HostingLoadCommandMonitor *event.CommandMonitor
var HostingLoadObserveTx func(time.Duration,error)

func Open(ctx context.Context, cfg devconfig.Config)"""),
    (".SetServerSelectionTimeout", ".SetMonitor(HostingLoadCommandMonitor).SetServerSelectionTimeout"),
    (".SetMaxPoolSize(3))", ".SetMaxPoolSize(3).SetMonitor(HostingLoadCommandMonitor))"),
    ("func (s *Store) tx(ctx context.Context, fn func(context.Context) error) error {", """func (s *Store) tx(ctx context.Context, fn func(context.Context) error) (result error) {
	var observeError error
	if HostingLoadObserveTx != nil {
		started := time.Now()
		defer func() {
			if observeError == nil { observeError = result }
			HostingLoadObserveTx(time.Since(started), observeError)
		}()
	}"""),
    ("options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))", "options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()))\n\tobserveError = e"),
])


def main():
    if os.name != "nt" or os.environ.get("MINIMIR_TOOLCHAIN_ACTIVE") != "1":
        raise RuntimeError("请通过 ./tools/dev.ps1 在 Windows 锁定工具环境下构建")
    WORK.mkdir(parents=True, exist_ok=True)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--name", choices=("hosting-load", "hosting-load-prejoin", "hosting-load-queue", "hosting-load-soak"), default="hosting-load")
    args = parser.parse_args()
    target = WORK / (args.name + ".exe")
    server_dev = SERVER / ".dev"
    server_dev.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="hosting-load-build-", dir=server_dev) as temporary:
        directory = Path(temporary)
        for path in SOURCE.glob("*.go"):
            shutil.copyfile(path, directory / path.name)
        overlay = directory / "overlay"
        overlay.mkdir()
        replacements = {}
        for original, source in replace.items():
            path = overlay / original.name
            path.write_text(source, encoding="utf-8")
            replacements[str(original)] = str(path)
        overlay_json = overlay / "overlay.json"
        overlay_json.write_text(json.dumps({"Replace": replacements}, indent=2), encoding="utf-8")
        command = [sys.executable, str(ROOT / "tools/dev"), "go"]
        package = "./" + directory.relative_to(SERVER).as_posix()
        subprocess.run(command + ["vet", "-overlay", str(overlay_json), package], check=True, cwd=ROOT)
        subprocess.run(command + ["build", "-overlay", str(overlay_json),
                                   "-o", str(target), package], check=True, cwd=ROOT)
    print(target)


if __name__ == "__main__":
    main()
