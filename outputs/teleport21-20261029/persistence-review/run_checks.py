"""rule15来源传送绑定兼容专项，保留实际命令/退出码；game仍在并行重构。"""
from pathlib import Path
import hashlib
import json
import os
import subprocess
import time

root = Path(__file__).resolve().parents[3]
directory = Path(__file__).resolve().parent
paths = ["server/internal/persistence/safe_zone_binding.go", "server/internal/persistence/safe_zone_binding_test.go", "server/internal/integration/safe_zone_binding_test.go"]
before = {path: hashlib.sha256((root / path).read_bytes()).hexdigest() for path in paths}
checks = [
    ("binding-compat-pure-race", ["go", "test", "-race", "./internal/persistence", "-run", "^Test(SafeZoneBinding|TeleportBinding|BindingProof|BindingSourceDocument|BindingDocument|LegacyBinding|BindingReceipt)", "-count=1", "-v"]),
    ("binding-compat-mongo-race", ["go", "test", "-race", "./internal/integration", "-run", "^TestSafeZoneBindingPermanentReceiptColdEnterAndOwnershipMongo$", "-count=1", "-v"]),
]
results = []
for name, arguments in checks:
    command = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(root / "tools/dev.ps1"), *arguments]
    environment = os.environ.copy()
    environment["MINIMIR_MONGO_E2E"] = "1"
    started = time.monotonic()
    result = subprocess.run(command, cwd=root, env=environment, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    (directory / f"{name}.txt").write_text(result.stdout.decode("utf-8", errors="replace"), encoding="utf-8")
    results.append({"name": name, "command": command, "environment": {"MINIMIR_MONGO_E2E": "1"}, "exit_code": result.returncode, "elapsed_seconds": round(time.monotonic() - started, 3), "log": f"{name}.txt"})
    print(json.dumps(results[-1], ensure_ascii=False), flush=True)
after = {path: hashlib.sha256((root / path).read_bytes()).hexdigest() for path in paths}
report = {"checks": results, "input_sha256": before, "scoped_inputs_stable": before == after, "scope": "仅rule15 Store兼容纯专项与真实Mongo事务；game并行重构，非全包/WS/实际21施放/容量验收；仅提交响应丢弃后原q重试，无真实UnknownTransaction网络注入"}
(directory / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
raise SystemExit(0 if before == after and all(result["exit_code"] == 0 for result in results) else 1)
