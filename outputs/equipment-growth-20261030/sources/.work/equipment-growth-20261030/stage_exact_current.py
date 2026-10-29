"""仅按原阶段清单显式暂存；处理已归档.work源被Git忽略的重暂存场景。"""
from pathlib import Path
import hashlib, json, subprocess, sys

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
DEV = [sys.executable, str(ROOT / 'tools/dev')]


def main():
    out = WORK / 'stage-exact-current-03'
    out.mkdir(exist_ok=False)
    command = DEV + ['python3', str(WORK / 'stage_verified.py')]
    plan = subprocess.run(command, cwd=ROOT, capture_output=True)
    (out / 'plan.stdout.txt').write_bytes(plan.stdout)
    (out / 'plan.stderr.txt').write_bytes(plan.stderr)
    assert plan.returncode == 0
    value = json.loads(plan.stdout)
    assert value['applied'] is False and value['extra_paths_allowed'] is False
    stage = json.loads((ROOT / 'docs/基础装备属性与正常穿脱阶段验收.json').read_text(encoding='utf-8'))
    assert stage['status'] == 'limited_passed' and stage['runtime']['rule_version'] == 22
    for name, digest in stage['evidence_sha256'].items():
        assert hashlib.sha256((ROOT / name).read_bytes()).hexdigest() == digest
    # Git状态不会列出被忽略的.log；正式证据清单中的全部原件均须显式暂存。
    paths = sorted(set(value['visible_paths']) | set(value['hidden_archived_work_sources']) | set(stage['evidence_sha256']))
    assert paths and all((ROOT / name).resolve().is_relative_to(ROOT.resolve()) and (ROOT / name).is_file() for name in paths)
    commands = []
    for position in range(0, len(paths), 40):
        argv = DEV + ['git', 'add', '-f', '--', *paths[position:position + 40]]
        result = subprocess.run(argv, cwd=ROOT, capture_output=True)
        prefix = out / str(position // 40 + 1)
        prefix.with_suffix('.stdout.txt').write_bytes(result.stdout)
        prefix.with_suffix('.stderr.txt').write_bytes(result.stderr)
        commands.append({'command': argv, 'exit_code': result.returncode})
        assert result.returncode == 0
    report = {'applied': True, 'exact_paths': paths, 'commands': commands,
              'scope': '仅原stage_verified只读计划批准的明确路径，保留原证据字节；无广泛递归添加',
              'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (out / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'applied': True, 'paths': len(paths), 'explicit_plan_only': True}, ensure_ascii=False))


if __name__ == '__main__':
    main()
