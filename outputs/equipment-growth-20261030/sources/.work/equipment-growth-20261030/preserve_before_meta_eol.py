"""保留第三版归档，并仅为已验Creator元数据追加原字节Git策略。"""
from pathlib import Path
import hashlib, json, subprocess, sys

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
OUT = ROOT / 'outputs/equipment-growth-20261030'
FORMAL = ROOT / 'docs/基础装备属性与正常穿脱阶段验收.json'
TARGET = WORK / 'history/pre-meta-eol-policy'
META = 'client/assets/scripts/app/EquipmentModel.ts.meta'
FIRST = b'outputs/equipment-growth-20261030/** -text\n'
APPEND = b'client/assets/scripts/app/EquipmentModel.ts.meta -text\n'
DEV = [sys.executable, str(ROOT / 'tools/dev'), 'git']
assert not TARGET.exists()
assert OUT.resolve() == ROOT / 'outputs/equipment-growth-20261030' and OUT.resolve().is_relative_to(ROOT.resolve())
assert FORMAL.resolve() == ROOT / 'docs/基础装备属性与正常穿脱阶段验收.json'
assert TARGET.resolve() == WORK / 'history/pre-meta-eol-policy' and TARGET.resolve().is_relative_to(WORK.resolve())
assert not OUT.is_symlink() and not OUT.is_junction()
raw = FORMAL.read_bytes()
stage = json.loads(raw)
assert stage['status'] == 'limited_passed' and stage['runtime']['rule_version'] == 22 and not stage['full_p0_completed']
expected = stage['evidence_sha256']
assert len(expected) == 378 and {p.relative_to(ROOT).as_posix() for p in OUT.rglob('*') if p.is_file()} == set(expected)
for name, digest in expected.items():
    path = ROOT / name
    assert path.resolve().is_relative_to(OUT.resolve()) and hashlib.sha256(path.read_bytes()).hexdigest() == digest
base = subprocess.check_output(DEV + ['show', 'HEAD:.gitattributes'], cwd=ROOT)
attributes = ROOT / '.gitattributes'
assert attributes.read_bytes() == base + FIRST
working = (ROOT / META).read_bytes()
indexed = subprocess.check_output(DEV + ['show', ':' + META], cwd=ROOT)
assert hashlib.sha256(working).hexdigest() == '25d0876c535d2e431d995c1e873c0292c8af1073ce1cd5f33351700ded661e3e'
assert working.replace(b'\r\n', b'\n') == indexed and json.loads(working) == json.loads(indexed)
TARGET.mkdir(parents=True)
(TARGET / 'normalized-meta-index.bin').write_bytes(indexed)
FORMAL.rename(TARGET / 'formal-report.json')
OUT.rename(TARGET / 'archive')
attributes.write_bytes(base + FIRST + APPEND)
record = {'preserved_evidence_files': len(expected), 'formal_sha256': hashlib.sha256(raw).hexdigest(),
          'scope': '只追加精确Creator已验元数据-text例外；产品/元数据/原日志/既有档案不变',
          'scan': str(WORK / 'index-differences.json'), 'files_scanned': 835, 'only_difference': META,
          'meta_working_sha256': hashlib.sha256(working).hexdigest(), 'normalized_index_sha256': hashlib.sha256(indexed).hexdigest(),
          'metadata_json_values_equal': True, 'exact_second_append': APPEND.decode('utf-8'),
          'attributes_after_sha256': hashlib.sha256(attributes.read_bytes()).hexdigest(),
          'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
(TARGET / 'preservation.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps({'preserved_files': len(expected), 'meta_unchanged': True, 'attributes_sha256': record['attributes_after_sha256']}, ensure_ascii=False))
