"""仅重跑本次来源序号不变量修正涉及的持久层纯测试。"""
from pathlib import Path
import hashlib
import json
import subprocess
import time

root = Path(__file__).resolve().parents[3]
directory = Path(__file__).resolve().parent
paths = ["server/internal/persistence/safe_zone_binding.go", "server/internal/persistence/safe_zone_binding_test.go"]
before = {path: hashlib.sha256((root / path).read_bytes()).hexdigest() for path in paths}
command = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(root / "tools/dev.ps1"), "go", "test", "-race", "./internal/persistence", "-run", "^Test(SafeZoneBinding|LegacyBinding|BindingSourceDocument|BindingProof|BindingReceipt)", "-count=1", "-v"]
started = time.monotonic()
result = subprocess.run(command, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
output = result.stdout.decode("utf-8", errors="replace")
(directory / "source-revision-invariant-pure-race.txt").write_text(output, encoding="utf-8")
after = {path: hashlib.sha256((root / path).read_bytes()).hexdigest() for path in paths}
old = directory / "report.json"
previous = directory / "report-before-source-revision-invariant.json"
if not previous.exists():
    previous.write_bytes(old.read_bytes())
report = {
    "status": "修正后的纯专项已执行；最终全包以root最终full记录为准",
    "earlier_checks": {"report": previous.name, "note": "此前persistence整包/race与真实Mongo记录均为来源序号不变量修正前，原始日志保留；没有将其冒称修正后全包证据"},
    "correction": "op.SourceSeq<=op.Revision；纯fixture改为SourceSeq8/EconomicSeq4/Revision15，ExpectedRevision从Role派生；新增不可达证明反例",
    "check": {"command": command, "exit_code": result.returncode, "elapsed_seconds": round(time.monotonic() - started, 3), "log": "source-revision-invariant-pure-race.txt"},
    "input_sha256": before, "inputs_stable": before == after,
    "scope": "本次只运行有变化的persistence纯专项；未重跑Mongo或全包；21文档仍是只读研究计划",
}
old.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(report, ensure_ascii=False), flush=True)
raise SystemExit(0 if result.returncode == 0 and before == after else 1)
