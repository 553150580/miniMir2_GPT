"""只读审查入口：经项目工具链执行overlay并原样留存返回码及输出。"""
from pathlib import Path
import json
import subprocess
import sys

root = Path(__file__).resolve().parents[3]
directory = Path(__file__).resolve().parent
command = [
    "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
    str(root / "tools/dev.ps1"), "go", "test", "-race", "-overlay",
    "../.work/town-travel-20261029/nav-review/review-overlay.json",
    "./internal/game", "-run", "^TestReview(PendingPause|Quiescing)", "-count=1",
]
result = subprocess.run(command, cwd=root, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
output = result.stdout.decode("utf-8", errors="replace")
label = sys.argv[1] if len(sys.argv) > 1 else "failing"
(directory / f"review-{label}-output.txt").write_text(output, encoding="utf-8")
(directory / (f"review-{label}-result.json" if label != "failing" else "review-result.json")).write_text(json.dumps({
    "command": command, "exit_code": result.returncode,
    "scope": "只读overlay；原反例未改断言；不修改生产文件",
}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(output, end="")
print(f"审查命令返回码：{result.returncode}")
raise SystemExit(result.returncode)
