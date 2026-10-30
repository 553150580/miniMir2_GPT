"""保留旧选择器原字节，显式选择Go2及修复后的客户端/Creator3。"""
from pathlib import Path
import hashlib
import json

WORK = Path(__file__).resolve().parent
ROOT = WORK.parents[1]


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


cutover = WORK / 'cutover.py'
normal = WORK / 'browser/normal/run-01/run.py'
before = {cutover: cutover.read_bytes(), normal: normal.read_bytes()}
assert digest(before[cutover]) == 'ea67d08a2fd388d24eb9b2f6e83dd5c84574aab770a04c8b8c072fed7d0cd72f'
assert digest(before[normal]) == '00f2da47fa7375e9798c7919b84051804a23937796f6d3565208624b51ac5510'
after = {
    cutover: before[cutover].replace(b'client/full-2', b'client/full-3').replace(b'creator/full-2', b'creator/full-3'),
    normal: before[normal].replace(b'creator/full-2', b'creator/full-3'),
}
original = after[cutover].replace(b'go/full-2', b'go/full-1').replace(b'client/full-3', b'client/full-1').replace(b'creator/full-3', b'creator/full-1')
assert digest(original) == '31ac3e5da97300a139fbb1e0e8148d7023532708de639d7e1f04a77be789d330'
assert after[normal].replace(b'creator/full-3', b'creator/full-2') == before[normal]
history = WORK / 'browser/history/second-isolated-0ce913/selectors-before'
history.mkdir(parents=True, exist_ok=False)
for path, raw in before.items():
    target = history / path.relative_to(WORK)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(raw)
for path, raw in after.items():
    assert path.read_bytes() == before[path]
    path.write_bytes(raw)
report = {
    'read_only': True,
    'reviewed_source_sha256': digest(original),
    'current_source_sha256': digest(after[cutover]),
    'only_changes': ['go/full-1→go/full-2', 'client/full-1→client/full-3', 'creator/full-1→creator/full-3'],
    'selected_attempts': {'go': '2', 'client': '3', 'creator': '3'},
    'all_other_source_bytes_equal': True,
    'script_executed': False,
    'normal_wrapper_sha256': digest(after[normal]),
    'normal_wrapper_only_change': 'Creator selector full-2→full-3',
    'preserved_before': {p.relative_to(ROOT).as_posix(): digest(raw) for p, raw in before.items()},
}
target = WORK / 'cutover-selector-review-3.json'
assert not target.exists()
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps(report, ensure_ascii=False))
