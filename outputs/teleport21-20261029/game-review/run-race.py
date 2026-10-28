"""保存20组21纯game反向回归，不接Mongo。"""
import datetime
import hashlib
import json
import pathlib
import subprocess
import time

directory = pathlib.Path(__file__).resolve().parent
root = directory.parents[2]
paths = sorted(list((root / "server/internal/game").glob("*.go")) + list((root / "server/internal/townnav").glob("*.go")))
def digest():
    return {p.relative_to(root).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}
before = digest()
command = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(root / "tools/dev.ps1"), "go", "test", "-race", "./internal/game", "-run", "^TestTeleport21", "-count=1", "-v"]
started = datetime.datetime.now(datetime.timezone.utc).isoformat()
begin = time.monotonic()
result = subprocess.run(command, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
(directory / "race-final.stdout.txt").write_bytes(result.stdout)
after = digest()
report = {"started_at_utc": started, "finished_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(), "command": command, "exit_code": result.returncode, "wall_seconds": round(time.monotonic() - begin, 3), "input_files": len(before), "changed": [key for key in before if before[key] != after[key]], "capture": "同一subprocess的stdout+stderr原始字节", "before": before, "after": after}
(directory / "race-final.result.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(result.stdout.decode("utf-8", errors="replace"))
print(json.dumps({k: v for k, v in report.items() if k not in ["before", "after"]}, ensure_ascii=False))
raise SystemExit(result.returncode)
