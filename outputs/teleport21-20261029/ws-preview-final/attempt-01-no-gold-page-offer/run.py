"""显式专属Mongo/WS定向作业：单会话直接保存真实输出，不重跑已通过用例。"""
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
(directory / "mongo-ownership.stdout.json").write_bytes(owner.stdout)
owned = json.loads(owner.stdout.decode("utf-8-sig")) if owner.returncode == 0 else {}
if not all(owned.get(key) for key in ["running", "listener_verified", "ownership_verified", "endpoint_verified"]):
    raise SystemExit("专属Mongo所有权未通过，不启动真实测试")

paths = sorted(set(list((root / "server/internal/game").glob("*.go")) + list((root / "server/internal/townnav").glob("*.go")) + [root / "server/internal/persistence/safe_zone_binding.go", root / "server/internal/integration/teleport21_test.go", root / "server/api/generated/minimirv1/world.pb.go", root / "config/development/first-loop.json"]))
def digests():
    return {path.relative_to(root).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest() for path in paths}
before = digests()
command = prefix + ["go", "test", "-race", "-count=1", "./internal/integration", "-run", "^TestTeleport21LearningBindingAndHealthyColdEntryMongoWS$", "-v"]
started = datetime.datetime.now(datetime.timezone.utc).isoformat()
(directory / "inputs-before.json").write_text(json.dumps({"recorded_at_utc": started, "files": before}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"phase": "test_start", "command": command, "ownership_pid": owned["pid"], "started_at_utc": started}), flush=True)
environment = os.environ.copy()
environment["MINIMIR_MONGO_E2E"] = "1"
begin = time.monotonic()
result = subprocess.run(command, cwd=root, env=environment, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
(directory / "mongo-ws-race.stdout.txt").write_bytes(result.stdout)
after = digests()
report = {"started_at_utc": started, "finished_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(), "command": command, "environment": {"MINIMIR_MONGO_E2E": "1"}, "exit_code": result.returncode, "wall_seconds": round(time.monotonic() - begin, 3), "input_files": len(before), "source_changed": [key for key in before if before[key] != after[key]], "ownership_exit_code": owner.returncode, "capture": "subprocess保存同一作业stdout+stderr原始字节；非PowerShell Tee", "scope": "定向真实注册/购书/Walk绑定/21/ACK/冷热入场；最终全包以root统一冻结manifest为准"}
(directory / "inputs-after.json").write_text(json.dumps(after, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
(directory / "result.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(result.stdout.decode("utf-8", errors="replace"), flush=True)
print(json.dumps(report, ensure_ascii=False), flush=True)
raise SystemExit(result.returncode)
