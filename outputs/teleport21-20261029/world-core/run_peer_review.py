"""只读核心审查：临时overlay探针，不改生产或生成配置。"""
from pathlib import Path
import hashlib
import json
import subprocess
import time

root = Path(__file__).resolve().parents[3]
base = Path(__file__).resolve().parent
assert base == root / ".work/teleport21-20261029/world-core"
production = ["combat.go", "timeline.go", "safe_zone_binding.go", "world_context.go"]

def hashes():
    return {name: hashlib.sha256((root / "server/internal/game" / name).read_bytes()).hexdigest() for name in production}

overlay = {"Replace": {str(root / "server/internal/game/world_peer_review_test.go"): str(base / "peer_review_test.go")}}
(base / "peer-review-overlay.json").write_text(json.dumps(overlay, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
command = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", str(root / "tools/dev.ps1"), "go", "test", "-race", "-overlay", str(base / "peer-review-overlay.json"), "./internal/game", "-run", "^Test(PeerWorld|World)", "-count=1", "-v"]
before = hashes()
started = time.time()
result = subprocess.run(command, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, encoding="utf-8", errors="replace")
elapsed = time.time() - started
(base / "peer-review-race.txt").write_text(json.dumps(command, ensure_ascii=False) + "\n\n" + result.stdout + f"\nexit_code={result.returncode}\nelapsed_seconds={elapsed:.3f}\n", encoding="utf-8")
after = hashes()
report = {"command": command, "exit_code": result.returncode, "elapsed_seconds": elapsed, "inputs_before": before, "inputs_after": after, "reviewed_core_inputs_stable": before == after, "scope": "永久回调原World/unknown原job/SourceCandidate与两侧kill的顺序；临时overlay及既有World纯测试", "boundary": "unknown由回调错误模拟，不是真实网络或UnknownTransaction故障注入；不代表21实际业务、WS或容量通过"}
(base / "peer-review-report.json").write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps({"exit_code": result.returncode, "reviewed_core_inputs_stable": before == after, "elapsed_seconds": elapsed, "log": str(base / "peer-review-race.txt")}, ensure_ascii=False))
raise SystemExit(result.returncode)
