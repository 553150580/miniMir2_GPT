"""以已完成的原始报告更新当前构建记录，并核对数值输入未变。"""
import importlib.util
import hashlib
import json
from pathlib import Path
import shutil
import subprocess

ROOT = Path(__file__).resolve().parents[2]
WORK = ROOT / '.work/p2-fixes-20261030'
spec = importlib.util.spec_from_file_location('records', WORK / 'finalize.py')
records = importlib.util.module_from_spec(spec)
spec.loader.exec_module(records)
destination = ROOT / 'outputs/p2-fixes-20261030'
records.archive_file(ROOT / 'docs/客户端构建检查.json', destination / 'history/prior-client-build-report.json')
current = records.load(WORK / 'creator/full-2/report.json')
current['original_build_root'] = current['build_root']
current['build_root'] = str(ROOT / 'client/build/web-mobile')
current['artifact_transfer_report'] = 'outputs/p2-fixes-20261030/publish/run-2/report.json'
current['seconds_scope'] = 'Creator执行及后段源码/布局/原包/产物SHA核对合计，不是单独引擎builder任务时长'
records.save(ROOT / 'docs/客户端构建检查.json', current)
report_path = ROOT / 'docs/六项P2修复阶段验收.json'
report = records.load(report_path)
report['creator_and_post_sha_guard_seconds'] = report.pop('creator_seconds')
protected = ['outputs/development-prep-20261026/玩法配置.xlsx', 'outputs/development-prep-20261026/架构配置.xlsx',
             'config/schema/catalog.json', 'config/development/first-loop.json',
             'client/assets/resources/development/gameplay.json', 'config/releases/preview_current.json']
for name in protected:
    baseline = subprocess.check_output(['git', 'show', report['baseline'] + ':' + name], cwd=ROOT)
    assert hashlib.sha256(baseline).hexdigest() == records.sha(ROOT / name), name
report['protected_inputs_unchanged_from_baseline'] = {p: records.sha(ROOT / p) for p in protected}
old = f'Creator3.8.8实构exit36、{report["creator_and_post_sha_guard_seconds"]:.2f}秒'
new = f'Creator3.8.8构建及后段SHA核对{report["creator_and_post_sha_guard_seconds"]:.2f}秒、exit36'
for name in ['README.md', 'docs/正式开发推进记录.md', 'docs/架构适配与规则优先级.md', 'docs/开发阶段与验收清单.md']:
    path = ROOT / name
    raw = path.read_bytes()
    assert old.encode() in raw
    path.write_bytes(raw.replace(old.encode(), new.encode(), 1))
for source in [WORK / 'finalize.py', Path(__file__)]:
    shutil.copyfile(source, destination / 'sources' / source.name)
report['evidence_sha256'] = records.manifest(destination)
records.save(report_path, report)
print(json.dumps({'passed': True, 'protected_inputs': len(protected), 'evidence_files': len(report['evidence_sha256'])}))
