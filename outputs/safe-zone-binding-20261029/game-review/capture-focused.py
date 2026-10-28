"""只保存本轮窄专项的真实子进程输出；不修改生产输入。"""
import datetime
import json
import pathlib
import subprocess
import time

root = pathlib.Path(__file__).resolve().parents[3]
out = pathlib.Path(__file__).resolve().parent
command = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(root / "tools/dev.ps1"), "go", "test", "-race", "./internal/game", "-run", "Test(SafeZoneBinding|ShoulderDashSourceSafeZone|SourcePushedCanEnter)", "-count=1"]
started = datetime.datetime.now(datetime.timezone.utc).isoformat()
begin = time.monotonic()
result = subprocess.run(command, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
(out / "focused-final.stdout.txt").write_bytes(result.stdout)
summary = {"started_at_utc": started, "command": command, "exit_code": result.returncode, "wall_seconds": round(time.monotonic() - begin, 3), "capture": "subprocess stdout+stderr bytes", "production_mutated": False}
(out / "focused-final.result.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(summary, ensure_ascii=False))
print(result.stdout.decode("utf-8", errors="replace"))
raise SystemExit(result.returncode)
