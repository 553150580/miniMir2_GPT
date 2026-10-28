"""通过项目锁定工具链留存真实定向race输出，不修改生产输入。"""
from pathlib import Path
import hashlib
import json
import os
import subprocess
import time

root = Path(__file__).resolve().parents[3]
directory = Path(__file__).resolve().parent
checks = [
    ("persistence-race", ["go", "test", "-race", "./internal/persistence", "./internal/domain", "-count=1", "-v"]),
    ("mongo-binding-race", ["go", "test", "-race", "./internal/integration", "-run", "^TestSafeZoneBindingPermanentReceiptColdEnterAndOwnershipMongo$", "-count=1", "-v"]),
]
paths = [
    "server/internal/domain/model.go", "server/internal/persistence/store.go",
    "server/internal/persistence/binding.go", "server/internal/persistence/safe_zone_binding.go",
    "server/internal/persistence/safe_zone_binding_test.go", "server/internal/integration/safe_zone_binding_test.go",
]
before = {path: hashlib.sha256((root / path).read_bytes()).hexdigest() for path in paths}
results = []
for name, arguments in checks:
    environment = os.environ.copy()
    environment["MINIMIR_MONGO_E2E"] = "1"
    command = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(root / "tools/dev.ps1"), *arguments]
    started = time.monotonic()
    result = subprocess.run(command, cwd=root, env=environment, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    output = result.stdout.decode("utf-8", errors="replace")
    (directory / f"{name}.txt").write_text(output, encoding="utf-8")
    results.append({"name": name, "command": command, "environment": {"MINIMIR_MONGO_E2E": "1"}, "exit_code": result.returncode, "elapsed_seconds": round(time.monotonic() - started, 3), "log": f"{name}.txt"})
    print(json.dumps(results[-1], ensure_ascii=False), flush=True)
after = {path: hashlib.sha256((root / path).read_bytes()).hexdigest() for path in paths}
report = {"checks": results, "input_sha256": before, "inputs_stable": before == after, "scope": "永久事务/回执与冷验；原已提交响应丢弃重试，不宣称真实UnknownTransaction故障注入；不作容量验收"}
(directory / "report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
raise SystemExit(0 if before == after and all(item["exit_code"] == 0 for item in results) else 1)
