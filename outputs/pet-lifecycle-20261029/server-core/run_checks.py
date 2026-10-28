import hashlib
import json
import subprocess
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent
OUT = OUT / "final"
OUT.mkdir(exist_ok=True)


def sources():
    return {
        p.relative_to(ROOT).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
        for p in sorted((ROOT / "server/internal/game").glob("*.go"))
    }


for name, extra in [("pet-race", ["-run", "^TestPet"]), ("game-race", [])]:
    args = [
        "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File",
        str(ROOT / "tools/dev.ps1"), "go", "test", "./internal/game",
        "-race", "-count=1", "-json", *extra,
    ]
    before = sources()
    started = time.time()
    with (OUT / (name + ".jsonl")).open("wb") as log:
        result = subprocess.run(args, cwd=ROOT, stdout=log, stderr=subprocess.STDOUT)
    after = sources()
    events = []
    for line in (OUT / (name + ".jsonl")).read_text(encoding="utf-8").splitlines():
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    report = {
        "command_argv": args, "exit_code": result.returncode,
        "wall_seconds": round(time.time() - started, 6),
        "source_scope": "server/internal/game/*.go (production and tests)",
        "source_sha256_before": before, "source_sha256_after": after,
        "sources_stable": before == after,
        "passed_test_nodes": sum(e.get("Action") == "pass" and "Test" in e for e in events),
        "failed_test_nodes": sum(e.get("Action") == "fail" and "Test" in e for e in events),
        "skipped_test_nodes": sum(e.get("Action") == "skip" and "Test" in e for e in events),
        "package_terminal": [e for e in events if e.get("Action") in ["pass", "fail"] and "Test" not in e],
        "raw_output": name + ".jsonl",
        "mongo_invoked": False,
    }
    (OUT / (name + "-result.json")).write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({k: report[k] for k in ["exit_code", "wall_seconds", "sources_stable", "passed_test_nodes", "failed_test_nodes", "skipped_test_nodes"]}), flush=True)
    if result.returncode or before != after:
        raise SystemExit(result.returncode or 2)
