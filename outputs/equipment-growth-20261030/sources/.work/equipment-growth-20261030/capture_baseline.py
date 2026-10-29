"""修改前冻结阶段22输入；只复制少量原配置，来源目录保持只读。"""
from pathlib import Path
import hashlib, json, shutil, subprocess, sys

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
DEV = [sys.executable, str(ROOT / 'tools/dev'), 'git']
HEAD = '2a5616ac883c91336edb227d7bbf87e9cfe64791'
assert subprocess.check_output(DEV + ['rev-parse', 'HEAD'], cwd=ROOT).strip().decode() == HEAD
assert not subprocess.check_output(DEV + ['status', '--porcelain=v1'], cwd=ROOT).strip(), '先确认已有改动归属'
target = WORK / 'baseline.json'
assert not target.exists()
paths = ['outputs/development-prep-20261026/玩法配置.xlsx', 'outputs/development-prep-20261026/架构配置.xlsx',
         'config/schema/catalog.json', 'tools/resources/prepare_first_loop.py', 'tools/resources/prepare_summon_candidates.py',
         'config/development/first-loop.json', 'client/assets/resources/development/gameplay.json',
         'config/development/pet-runtime-candidates.json', 'config/development/summon-candidates.json', 'config/releases/preview_current.json',
         'shared/proto/assets.proto', 'shared/proto/protocol_manifest.json',
         'conf/toolchain/client-pb-verification.json', 'conf/toolchain/go-pb-generation.json', 'conf/toolchain/config-binding-sync.json']
hashes = {}
for name in paths:
    source = ROOT / name
    hashes[name] = hashlib.sha256(source.read_bytes()).hexdigest()
    backup = WORK / 'before' / name
    backup.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, backup)
layouts = {p.relative_to(ROOT).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest() for pattern in ['*.scene', '*.prefab'] for p in (ROOT / 'client/assets').rglob(pattern)}
assert len(layouts) == 10
report = {'head': HEAD, 'files': hashes, 'saved_layouts': layouts, 'scope': '阶段22修改前原态；不将阶段21通过追认为新阶段验收'}
target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps({'baseline_created': True, 'head': HEAD, 'copied_files': len(paths), 'saved_layouts': len(layouts)}, ensure_ascii=False))
