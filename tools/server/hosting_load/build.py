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
    ("func (c *connection) Close(_ string) {\n\tif c.closed.CompareAndSwap(false, true) {", """var HostingLoadObserveClose func(string)

func (c *connection) Close(reason string) {
	if c.closed.CompareAndSwap(false, true) {
		if HostingLoadObserveClose != nil { HostingLoadObserveClose(reason) }"""),
    ("func (s *Server) ws(", "var HostingLoadConnectionLimit int64 = 128\n\nfunc (s *Server) ws("),
    ("if s.active.Add(1) > 128 {", "if s.active.Add(1) > HostingLoadConnectionLimit {"),
])
change("internal/game/runtime.go", [
    ("func New(ctx context.Context,", """var HostingLoadRoomLimit = 256
var HostingLoadObserveTick func(shard, rooms, queue int, elapsed, lag time.Duration)
var HostingLoadQueueFull func()

func New(ctx context.Context,"""),
    ("type worker struct {", "type worker struct {\n\thostingLoadShard int"),
    ("w := &worker{e: e, commands:", "w := &worker{hostingLoadShard: i, e: e, commands:"),
    ("if len(w.rooms) >= 256 {", "if len(w.rooms) >= HostingLoadRoomLimit {"),
    ("if !c.tick.IsZero() {\n\t\t\t\tw.begin", "if !c.tick.IsZero() {\n\t\t\t\tstarted := time.Now()\n\t\t\t\tw.begin"),
    ("w.endTownNavigationTick()\n\t\t\t\tcontinue", """w.endTownNavigationTick()
				if HostingLoadObserveTick != nil {
					HostingLoadObserveTick(w.hostingLoadShard, len(w.rooms), len(w.e.jobs), time.Since(started), started.Sub(c.tick))
				}
				continue"""),
    ("case w.e.jobs <- j:\n\t\treturn true\n\tdefault:\n\t\treturn false", """case w.e.jobs <- j:
		return true
	default:
		if HostingLoadQueueFull != nil { HostingLoadQueueFull() }
		return false"""),
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
    (".SetMaxPoolSize(16).SetServerSelectionTimeout", ".SetMaxPoolSize(16).SetMonitor(HostingLoadCommandMonitor).SetServerSelectionTimeout"),
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
    parser.add_argument("--name", choices=("hosting-load", "hosting-load-prejoin"), default="hosting-load")
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
