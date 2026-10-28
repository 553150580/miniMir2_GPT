"""仅运行合法rule12夹具的既有绑定真实用例；保存同一进程原始输出。"""
import datetime
import hashlib
import json
import os
import pathlib
import subprocess
import time

directory = pathlib.Path(__file__).resolve().parent
root = directory.parents[2]
prefix = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(root / "tools/dev.ps1")]
owner = subprocess.run(prefix + ["python", "tools/environment/mongodb_windows.py", "status"], cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
(directory / "mongo-ownership.json").write_bytes(owner.stdout)
owned = json.loads(owner.stdout.decode("utf-8-sig")) if owner.returncode == 0 else {}
if not all(owned.get(key) for key in ["running", "listener_verified", "ownership_verified", "endpoint_verified"]):
    raise SystemExit("专属Mongo所有权未通过，不启动真实测试")
paths = sorted(list((root / "server/internal/devconfig").glob("*.go")) + [root / "server/internal/integration/binding_test.go", root / "server/internal/persistence/auth.go", root / "server/internal/persistence/binding.go", root / "config/development/first-loop.json"])
def digest():
    return {p.relative_to(root).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}
before = digest()
started = datetime.datetime.now(datetime.timezone.utc).isoformat()
command = prefix + ["go", "test", "-race", "-count=1", "./internal/integration", "-run", "^TestRoleBindingInitializationBackfillAndHealthyReconnectMongoWS$", "-v"]
print(json.dumps({"phase": "test_start", "started_at_utc": started, "command": command, "ownership_pid": owned["pid"]}), flush=True)
environment = os.environ.copy()
environment["MINIMIR_MONGO_E2E"] = "1"
begin = time.monotonic()
result = subprocess.run(command, cwd=root, env=environment, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
(directory / "after.stdout.txt").write_bytes(result.stdout)
after = digest()
report = {"started_at_utc": started, "finished_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(), "command": command, "environment": {"MINIMIR_MONGO_E2E": "1"}, "exit_code": result.returncode, "wall_seconds": round(time.monotonic() - begin, 3), "changed": [key for key in before if before[key] != after[key]], "before": before, "after": after, "ownership_exit_code": owner.returncode, "capture": "同一subprocess stdout+stderr原始字节"}
(directory / "result.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(result.stdout.decode("utf-8", errors="replace"))
print(json.dumps({key: value for key, value in report.items() if key not in ["before", "after"]}, ensure_ascii=False))
raise SystemExit(result.returncode)
