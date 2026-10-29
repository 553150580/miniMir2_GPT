"""保留第二版归档全部原字节，供仅修私有索引核验器变量影蔽后重建。"""
from pathlib import Path
import hashlib, json

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
OUT = ROOT / 'outputs/equipment-growth-20261030'
FORMAL = ROOT / 'docs/基础装备属性与正常穿脱阶段验收.json'
TARGET = WORK / 'history/pre-pathmap-fix'
assert not TARGET.exists()
assert OUT.resolve() == ROOT / 'outputs/equipment-growth-20261030' and OUT.resolve().is_relative_to(ROOT.resolve())
assert FORMAL.resolve() == ROOT / 'docs/基础装备属性与正常穿脱阶段验收.json'
assert TARGET.resolve() == WORK / 'history/pre-pathmap-fix' and TARGET.resolve().is_relative_to(WORK.resolve())
assert not OUT.is_symlink() and not OUT.is_junction()
raw = FORMAL.read_bytes()
stage = json.loads(raw)
assert stage['status'] == 'limited_passed' and stage['runtime']['rule_version'] == 22 and not stage['full_p0_completed']
expected = stage['evidence_sha256']
assert len(expected) == 368 and {p.relative_to(ROOT).as_posix() for p in OUT.rglob('*') if p.is_file()} == set(expected)
for name, digest in expected.items():
    path = ROOT / name
    assert path.resolve().is_relative_to(OUT.resolve()) and hashlib.sha256(path.read_bytes()).hexdigest() == digest
TARGET.mkdir(parents=True)
FORMAL.rename(TARGET / 'formal-report.json')
OUT.rename(TARGET / 'archive')
record = {'preserved_evidence_files': len(expected), 'formal_sha256': hashlib.sha256(raw).hexdigest(),
          'scope': '仅保留本任务第二版归档和报告；未改任何产品、测试、原日志或Git index',
          'source': str(OUT), 'archive': str(TARGET / 'archive'), 'formal': str(TARGET / 'formal-report.json'),
          'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
(TARGET / 'preservation.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps({'preserved_evidence_files': len(expected), 'product_changed': False}, ensure_ascii=False))
