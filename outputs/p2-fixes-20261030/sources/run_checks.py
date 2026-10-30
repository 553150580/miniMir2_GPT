"""冻结本轮修复输入，保存真实命令和候选构建证据；不覆盖历史验收。"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import runpy
import shutil
import subprocess
import sys
import threading
import time

ROOT = next(p for p in Path(__file__).resolve().parents if (p / 'tools/dev').is_file())
WORK = ROOT / '.work/p2-fixes-20261030'
DEV = [sys.executable, str(ROOT / 'tools/dev')]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def tree(folder):
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(folder.rglob('*')) if p.is_file()}


def client_inputs():
    paths = [p for name in ['client/assets/scripts', 'client/tests'] for p in (ROOT / name).rglob('*') if p.is_file()]
    paths += [p for p in (ROOT / 'client/assets').rglob('*') if p.suffix in {'.scene', '.prefab'}]
    paths += [ROOT / name for name in ['client/assets/resources/development/gameplay.json', 'config/development/first-loop.json']]
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(set(paths))}


def checked_command(command, directory, name, timeout):
    start = time.monotonic()
    with (directory / (name + '.stdout.txt')).open('wb') as stdout, (directory / (name + '.stderr.txt')).open('wb') as stderr:
        result = subprocess.run(DEV + command, cwd=ROOT, stdout=stdout, stderr=stderr, timeout=timeout)
    row = {'command': command, 'exit_code': result.returncode, 'seconds': round(time.monotonic() - start, 3),
           'stdout': str((directory / (name + '.stdout.txt')).relative_to(ROOT)),
           'stderr': str((directory / (name + '.stderr.txt')).relative_to(ROOT))}
    print(json.dumps(row, ensure_ascii=False), flush=True)
    return row


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('group', choices=['server', 'client', 'creator'])
    parser.add_argument('--attempt', required=True)
    args = parser.parse_args()
    if not args.attempt.isalnum():
        parser.error('attempt仅允许字母数字')
    out = WORK / args.group / ('full-' + args.attempt)
    out.mkdir(parents=True, exist_ok=False)
    enumerate_server = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='inputs_only')['input_files']
    snapshot = (lambda: {p.relative_to(ROOT).as_posix(): sha(p) for p in enumerate_server()}) if args.group == 'server' else client_inputs
    report = {'group': args.group, 'started_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'before': snapshot(), 'commands': [], 'production_ready': False, 'physical_mobile_verified': False}
    save(out / 'started.json', report)
    if args.group == 'server':
        history = out / 'prior-latest'
        history.mkdir()
        for name in ['latest.json', 'latest.events.jsonl']:
            source = ROOT / 'server/test-results' / name
            if source.is_file():
                shutil.copyfile(source, history / name)
        commands = [('full-mongo-race', ['python3', 'tools/server/test.py', '--mongo', '--race', '--timeout-seconds', '1200'], 1400),
                    ('vet', ['go', 'vet', './...'], 120)]
    elif args.group == 'client':
        commands = [('models', ['node', 'client/tests/ui/compile_models.mjs'], 120),
                    ('ui-app', ['node', '--test', 'client/tests/ui/*.test.cjs', 'client/tests/app/*.test.cjs'], 180),
                    ('scene-compile', ['node', 'tools/client/compile_scene_tests.mjs'], 90),
                    ('scene', ['node', '--test', 'client/tests/scene/*.test.cjs'], 90),
                    ('strict', ['node', 'tools/client/typecheck.mjs'], 120),
                    ('pb', ['node', 'tools/protocol/verify_client_pb.cjs'], 120),
                    ('protocol-ids', ['python3', 'tools/config/generate_protocol.py', '--check'], 60)]
    else:
        config = json.loads((ROOT / 'conf/creator-build.json').read_text(encoding='utf-8'))
        config['buildPath'] = str(out / 'candidate')
        save(out / 'creator-build.json', config)
        layouts = {p.relative_to(ROOT).as_posix(): sha(p) for p in (ROOT / 'client/assets').rglob('*') if p.suffix in {'.scene', '.prefab'}}
        served = tree(ROOT / 'client/build/web-mobile')
        save(out / 'served-build-before.json', served)
        app = Path(os.environ['MINIMIR_CREATOR_EXECUTABLE'])
        command = [str(app)] + (['--no-sandbox', '--disable-gpu'] if os.name == 'nt' else [])
        command += ['--project', str(ROOT / 'client'), '--build', 'configPath=' + str(out / 'creator-build.json')]
        started = time.monotonic()
        with (out / 'creator.txt').open('wb') as stdout:
            result = subprocess.run(command, cwd=ROOT, stdout=stdout, stderr=subprocess.STDOUT, timeout=1200)
        save(out / 'creator-exit.json', {'exit_code': result.returncode,
             'finished_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat()})
        current = snapshot()
        after_layouts = {p.relative_to(ROOT).as_posix(): sha(p) for p in (ROOT / 'client/assets').rglob('*') if p.suffix in {'.scene', '.prefab'}}
        artifact = tree(out / 'candidate/web-mobile')
        save(out / 'candidate-files.json', artifact)
        served_after = tree(ROOT / 'client/build/web-mobile')
        save(out / 'served-build-after.json', served_after)
        report.update(creator='3.8.8', platform='web-mobile', exit_code=result.returncode,
                      seconds=round(time.monotonic() - started, 3), asset_hashes=after_layouts,
                      script_hashes={p: digest for p, digest in current.items() if p.startswith('client/assets/scripts/') and p.endswith(('.ts', '.js'))},
                      scene_prefab_layout_preserved=layouts == after_layouts,
                      served_build_preserved=served == served_after, build_root=str(out / 'candidate/web-mobile'),
                      artifact_count=len(artifact), commands=[{'command': command, 'exit_code': result.returncode}])
        report['after'] = current
        report['source_stable_during_build'] = report['before'] == current
        report['success'] = (result.returncode == 36 and (out / 'candidate/web-mobile/index.html').is_file()
                             and report['scene_prefab_layout_preserved'] and report['served_build_preserved']
                             and report['source_stable_during_build'])
        report['passed'] = report['success']
        save(out / 'report.json', report)
        print(json.dumps({k: report[k] for k in ['passed', 'exit_code', 'seconds', 'artifact_count', 'build_root']}, ensure_ascii=False))
        return 0 if report['passed'] else 1
    for name, command, timeout in commands:
        row = checked_command(command, out, name, timeout)
        report['commands'].append(row)
        if row['exit_code']:
            break
    report['after'] = snapshot()
    report['stable'] = report['before'] == report['after']
    report['passed'] = report['stable'] and len(report['commands']) == len(commands) and all(row['exit_code'] == 0 for row in report['commands'])
    if args.group == 'server':
        for name in ['latest.json', 'latest.events.jsonl']:
            shutil.copyfile(ROOT / 'server/test-results' / name, out / name)
        result = json.loads((out / 'latest.json').read_text(encoding='utf-8'))
        report['passed'] = report['passed'] and result['passed'] and not result['skipped_tests']
        report['passed_test_count'] = len(result['passed_tests'])
    report['finished_at_utc'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    save(out / 'report.json', report)
    print(json.dumps({k: report[k] for k in ['passed', 'stable', 'finished_at_utc']}, ensure_ascii=False))
    return 0 if report['passed'] else 1


if __name__ == '__main__':
    heartbeat_stop = threading.Event()
    def heartbeat():
        while not heartbeat_stop.wait(20):
            print(json.dumps({'status': 'running', 'at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat()}), flush=True)
    threading.Thread(target=heartbeat, daemon=True).start()
    try:
        raise SystemExit(main())
    finally:
        heartbeat_stop.set()
