"""为已搬走的首次预检原日志登记显式路径映射，不重写原报告或放宽原SHA。"""
from pathlib import Path
import hashlib
import json

WORK = Path(__file__).resolve().parent
ROOT = WORK.parents[1]
folder = WORK / 'runtime-history/preflight-unavailable-1'
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
write = lambda p, v: p.write_text(json.dumps(v, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
report_path = folder / 'report.json'
value = read(report_path)
assert value['passed'] is False and value['phase'] == 'preflight' and value['rollback_attempted'] is False
assert [r['name'] for r in value['commands']] == ['mongo-before', 'server-before', 'client-before']
entries = []
for row in value['commands']:
    for stream in ['stdout', 'stderr']:
        original = Path(row[stream])
        assert original.parent.as_posix() == '.work/warehouses-20261030/runtime-control'
        actual = folder / original.name
        assert sha(actual) == row[stream + '_sha256']
        entries.append({'original_path': original.as_posix(), 'preserved_path': actual.relative_to(ROOT).as_posix(),
                        'raw_sha256': sha(actual)})
mapping = folder / 'relocation.json'
assert not mapping.exists()
write(mapping, {'original_report_path': '.work/warehouses-20261030/runtime-control/report.json',
                'preserved_report_path': report_path.relative_to(ROOT).as_posix(),
                'original_report_raw_sha256': sha(report_path), 'original_report_unchanged': True,
                'reason': '原后端已退出导致第一次preflight拒绝；原目录完整搬入history后才允许第二次同源切换。旧报告引用按本映射解析，不能误读重试的新同名日志。',
                'logs': entries})
plan_path, allowed_path = WORK / 'curate-plan.json', WORK / 'source-allowlist.json'
plan, allowed = read(plan_path), read(allowed_path)
name = report_path.relative_to(ROOT).as_posix()
matches = [r for r in plan['history_reports'] if r['path'] == name]
assert len(matches) == 1
plan['history_reports'] = [r for r in plan['history_reports'] if r['path'] != name]
for path in [report_path, mapping, *[ROOT / r['preserved_path'] for r in entries]]:
    assert path.is_file() and path.resolve().is_relative_to(folder.resolve())
    name = path.relative_to(ROOT).as_posix()
    assert not any(r['path'] == name for r in plan['history_files'])
    plan['history_files'].append({'path': name, 'scope': 'actual_failed_preflight_relocated_original_raw_bytes_explicit_relocation_map_not_pass'})
script = Path(__file__).resolve().relative_to(ROOT).as_posix()
assert script not in plan['private_sources'] and script not in allowed['allowed_paths']
plan['private_sources'].append(script)
allowed['allowed_paths'].append(script)
write(plan_path, plan)
write(allowed_path, allowed)
print(json.dumps({'preserved_report_sha256': sha(report_path), 'mapped_raw_logs': len(entries), 'old_raw_report_rewritten': False}))
