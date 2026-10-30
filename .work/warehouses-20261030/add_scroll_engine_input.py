"""把实际读取的ScrollView纳入第10份外部输入，不变更受控环境行为。"""
from pathlib import Path
import ast
import hashlib
import json
import subprocess
import sys

WORK = Path(__file__).resolve().parent
ROOT = WORK.parents[1]
out = WORK / 'qa-tool-tests-scroll-3'
out.mkdir(exist_ok=False)
paths = [WORK / 'stage_checks.py', ROOT / 'tools/client/prepare_warehouses_qa.py']
needle = b"'audio/audio-source.ts', 'ui/toggle.ts']"
replacement = b"'audio/audio-source.ts', 'ui/toggle.ts', 'ui/scroll-view.ts']"
sha = lambda raw: hashlib.sha256(raw).hexdigest()
before = {p.relative_to(ROOT).as_posix(): p.read_bytes() for p in paths}
assert sha(paths[1].read_bytes()) == '045784638e2bbbd193a4bab840c3ffcd3cd200bdb301113a09322cb6e8e9c053'
for path in paths:
    raw = path.read_bytes()
    assert raw.count(needle) == 1
    changed = raw.replace(needle, replacement)
    assert changed.replace(replacement, needle) == raw
    backup = out / 'before' / path.relative_to(ROOT)
    backup.parent.mkdir(parents=True, exist_ok=True)
    backup.write_bytes(raw)
    ast.parse(changed.decode('utf-8'))
    path.write_bytes(changed)
commands = []
for name, args in [
    ('unit', ['-m', 'unittest', 'discover', '-s', 'tools/client', '-p', 'test_warehouses_qa.py', '-v']),
    ('prepare', ['tools/client/prepare_warehouses_qa.py']),
]:
    with (out / (name + '.stdout.txt')).open('xb') as stdout, (out / (name + '.stderr.txt')).open('xb') as stderr:
        result = subprocess.run([sys.executable, *args], cwd=ROOT, stdout=stdout, stderr=stderr)
    commands.append({'name': name, 'command': [sys.executable, *args], 'exit_code': result.returncode,
                     **{stream: (out / (name + '.' + stream + '.txt')).relative_to(ROOT).as_posix() for stream in ['stdout', 'stderr']},
                     **{stream + '_sha256': sha((out / (name + '.' + stream + '.txt')).read_bytes()) for stream in ['stdout', 'stderr']}})
report = {'passed': all(row['exit_code'] == 0 for row in commands), 'commands': commands,
          'before': {name: sha(raw) for name, raw in before.items()},
          'after': {p.relative_to(ROOT).as_posix(): sha(p.read_bytes()) for p in paths},
          'only_change': "ENGINE_FILES追加ui/scroll-view.ts，其它原字节逐字保持",
          'scope': '外部依赖冻结与14项纯安全边界；无环境、DB、浏览器或产品检查'}
(out / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps(report, ensure_ascii=False))
raise SystemExit(0 if report['passed'] else 1)
