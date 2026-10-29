"""保留本阶段首次归档原件，补阶段专属Git原字节策略；不修改产品或原日志。"""
from pathlib import Path
import datetime, hashlib, json, subprocess, sys

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
OUT = ROOT / 'outputs/equipment-growth-20261030'
FORMAL = ROOT / 'docs/基础装备属性与正常穿脱阶段验收.json'
BACKUP = WORK / 'history/pre-eol-policy'
DEV = [sys.executable, str(ROOT / 'tools/dev'), 'git']
POLICY = b'outputs/equipment-growth-20261030/** -text\n'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    assert subprocess.check_output(DEV + ['rev-parse', 'HEAD'], cwd=ROOT).decode().strip() == '2a5616ac883c91336edb227d7bbf87e9cfe64791'
    assert not BACKUP.exists()
    assert OUT.resolve() == ROOT / 'outputs/equipment-growth-20261030' and OUT.resolve().is_relative_to(ROOT.resolve())
    assert FORMAL.resolve() == ROOT / 'docs/基础装备属性与正常穿脱阶段验收.json' and FORMAL.resolve().is_relative_to(ROOT.resolve())
    assert BACKUP.resolve() == WORK / 'history/pre-eol-policy' and BACKUP.resolve().is_relative_to(WORK.resolve())
    assert not OUT.is_symlink() and not OUT.is_junction()
    original_report = FORMAL.read_bytes()
    stage = json.loads(original_report)
    assert stage['status'] == 'limited_passed' and stage['runtime']['rule_version'] == 22 and not stage['full_p0_completed']
    expected = stage['evidence_sha256']
    actual = {p.relative_to(ROOT).as_posix() for p in OUT.rglob('*') if p.is_file()}
    assert actual == set(expected) and len(actual) == 355
    for name, digest in expected.items():
        path = ROOT / name
        assert path.resolve().is_relative_to(OUT.resolve()) and sha(path.read_bytes()) == digest
    attributes = ROOT / '.gitattributes'
    base = subprocess.check_output(DEV + ['show', 'HEAD:.gitattributes'], cwd=ROOT)
    assert attributes.read_bytes() == base and base.endswith(b'\n') and POLICY not in base
    name = 'outputs/equipment-growth-20261030/current/client/protocol-check.stdout.txt'
    working = (ROOT / name).read_bytes()
    indexed = subprocess.check_output(DEV + ['show', ':' + name], cwd=ROOT)
    assert working != indexed and working.replace(b'\r\n', b'\n') == indexed
    BACKUP.mkdir(parents=True)
    (BACKUP / 'normalized-index-sample.txt').write_bytes(indexed)
    FORMAL.rename(BACKUP / 'formal-report.json')
    OUT.rename(BACKUP / 'archive')
    attributes.write_bytes(base + POLICY)
    record = {'recorded_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'scope': '原355归档及正式报告原字节保留；仅追加本阶段Git -text规则，未改任何产品、数值或原日志',
              'original_formal_sha256': sha(original_report), 'original_evidence_files': 355,
              'backup_archive': str(BACKUP / 'archive'), 'backup_formal': str(BACKUP / 'formal-report.json'),
              'gitattributes_before_sha256': sha(base), 'gitattributes_after_sha256': sha(attributes.read_bytes()),
              'exact_append_utf8': POLICY.decode(), 'sample': {'path': name, 'working_bytes': len(working), 'index_bytes': len(indexed),
                  'working_sha256': sha(working), 'normalized_index_sha256': sha(indexed), 'only_crlf_normalization': True},
              'script_sha256': sha(Path(__file__).read_bytes()), 'git_index_modified_by_this_script': False}
    (BACKUP / 'preservation.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'preserved_original_evidence_files': 355, 'exact_policy_appended': True, 'products_or_logs_changed': False}, ensure_ascii=False))


if __name__ == '__main__':
    main()
