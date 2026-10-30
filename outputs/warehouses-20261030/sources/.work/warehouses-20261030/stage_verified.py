"""仅精确暂存本阶段白名单与formal全部manifest文件；默认dry，无目录递归git add。"""
from pathlib import Path
import argparse
import hashlib
import json
import os
import subprocess

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
BASE = 'd2dab71909d4c389482be9b95d9594db76310eeb'
FORMAL = 'docs/双仓储与五角色并发阶段验收.json'


def need(ok, message):
    if not ok:
        raise ValueError(message)


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT, env={**os.environ, 'GIT_OPTIONAL_LOCKS': '0'})


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(); parser.add_argument('--apply', action='store_true'); parser.add_argument('--plan', type=Path)
    args = parser.parse_args()
    need(git('rev-parse', 'HEAD').strip().decode() == BASE, 'HEAD不是本阶段基线')
    stage = json.loads((ROOT / FORMAL).read_text(encoding='utf-8'))
    need(stage['status'] == 'limited_passed' and stage['runtime']['rule_version'] == 23 and stage['full_p0_completed'] is False, '必须先通过正式限定报告')
    allowed = set(json.loads((WORK / 'source-allowlist.json').read_text(encoding='utf-8'))['allowed_paths'])
    release = stage['runtime']['preview']; need(release.startswith('dev-warehouses-v0.23-') and '/' not in release, 'preview错误')
    allowed |= {'config/releases/' + release + '/' + name for name in ['client.json', 'server.json', 'manifest.json', 'config_types.go', 'config_types.ts']}
    changed = {n.decode('utf-8') for n in git('diff', '--name-only', '-z', BASE).split(b'\0') if n}
    new = {n.decode('utf-8') for n in git('ls-files', '--others', '--exclude-standard', '-z').split(b'\0') if n}
    private = set(json.loads((WORK / 'curate-plan.json').read_text(encoding='utf-8'))['private_sources'])
    paths = set(stage['evidence_sha256']) | {FORMAL} | private | ((changed | new) & allowed)
    for name in sorted(paths):
        p = ROOT / name
        need(not Path(name).is_absolute() and '..' not in Path(name).parts and p.resolve().is_relative_to(ROOT) and p.is_file() and not p.is_symlink() and not p.is_junction(), '路径不是本仓库普通文件：' + name)
        need(name in allowed or name in stage['evidence_sha256'], '未批准路径：' + name)
        if name in stage['evidence_sha256']:
            need(sha(p) == stage['evidence_sha256'][name], '正式归档SHA不同：' + name)
    # ignored *.log/.work不从Git status猜；全部formal.evidence_sha256精确路径强制加入。
    result = {'passed': True, 'applied': False, 'baseline': BASE, 'source_sha256': sha(Path(__file__)),
              'paths': sorted(paths), 'file_sha256': {n: sha(ROOT / n) for n in sorted(paths)},
              'scope': '仅精确路径git add --force；不git add -f .，不提交、不推送，不运行产品检查'}
    if args.plan:
        need(args.plan.resolve().is_relative_to(WORK) and not args.plan.exists(), '计划只新建于本阶段.work')
        args.plan.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    if args.apply:
        names = sorted(paths)
        for offset in range(0, len(names), 24):
            r = subprocess.run(['git', 'add', '--force', '--', *names[offset:offset + 24]], cwd=ROOT, capture_output=True)
            need(r.returncode == 0, '精确暂存失败；不扩大范围或自动重试')
        need(all(sha(ROOT / n) == h for n, h in result['file_sha256'].items()), '暂存时原件变化')
        result['applied'] = True
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, FileNotFoundError) as error:
        print(json.dumps({'passed': False, 'error': str(error)}, ensure_ascii=False))
        raise SystemExit(1)
