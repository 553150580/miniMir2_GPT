"""只读生产文件并补记冻结输入复核，不触发测试。"""
import datetime
import hashlib
import json
import pathlib

directory = pathlib.Path(__file__).resolve().parent
root = directory.parents[2]
before = json.loads((directory / "frozen-inputs.json").read_text(encoding="utf-8"))
changed = []
for entry in before["files"]:
    path = root / entry["path"]
    actual = hashlib.sha256(path.read_bytes()).hexdigest()
    if actual != entry["sha256"]:
        changed.append({"path": entry["path"], "expected": entry["sha256"], "actual": actual})
out = {"checked_at_utc": datetime.datetime.now(datetime.timezone.utc).isoformat(), "files_checked": len(before["files"]), "changed": changed, "stable": not changed, "scope": "归档manifest所列game全部Go及关键Store/domain/PB/fixture输入；root全包manifest为阶段最终权威"}
(directory / "frozen-inputs-recheck.json").write_text(json.dumps(out, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(json.dumps(out, ensure_ascii=False))
raise SystemExit(bool(changed))
