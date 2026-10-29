"""保留阶段21真实检查的原始输出和输入前后态；不覆盖既有终态。"""
from pathlib import Path
import argparse, datetime, hashlib, json, runpy, shutil, subprocess, sys, time

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
DEV = [sys.executable, str(ROOT / 'tools/dev')]
ENGINE = Path('D:/Program Files/CocosCreator/3.8.8/resources/resources/3d/engine/cocos')
ENGINE_FILES = ['input/input.ts', '2d/event/pointer-event-dispatcher.ts', 'scene-graph/node-event-processor.ts',
                'scene-graph/node.ts', 'ui/button.ts', 'core/data/object.ts', 'scene-graph/component.ts',
                'audio/audio-source.ts', 'ui/toggle.ts']
FONTS = [Path('C:/Windows/Fonts/arial.ttf'), Path('C:/Windows/Fonts/msyh.ttc'), Path('C:/Windows/Fonts/simsun.ttc')]


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def write(p, value):
    p.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def client_inputs():
    paths = [p for folder in ['client/assets/scripts', 'client/tests'] for p in (ROOT / folder).rglob('*') if p.is_file()]
    paths += [p for pattern in ['*.scene', '*.prefab'] for p in (ROOT / 'client/assets').rglob(pattern)]
    paths += [ROOT / p for p in ['client/assets/resources/development/gameplay.json', 'client/assets/resources/development/manifest.json', 'config/development/first-loop.json']]
    return sorted(set(paths))


def snapshot(paths):
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in paths}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('group', choices=['go', 'client', 'creator'])
    parser.add_argument('--attempt', default='1')
    args = parser.parse_args()
    assert args.attempt.isalnum()
    out = WORK / args.group / ('full-' + args.attempt)
    out.mkdir(parents=True, exist_ok=False)
    if args.group == 'go':
        definition = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='only_enumerate_inputs')
        enumerate_inputs = definition['input_files']
        # 新增真实在线/离线满包场景后只增加测试包总观察预算；业务时限保持原配置。
        checks = [('full-mongo-race', ['python3', 'tools/server/test.py', '--mongo', '--race', '--timeout-seconds', '900'])]
    else:
        enumerate_inputs = client_inputs
        checks = [('creator', ['python3', 'tools/client/build.py'])] if args.group == 'creator' else [
            ('models', ['node', 'client/tests/ui/compile_models.mjs']),
            ('full-ui-app', ['node', '--test', 'client/tests/ui/*.test.cjs', 'client/tests/app/*.test.cjs']),
            ('scene', ['node', '--test', 'client/tests/scene/*.test.cjs']),
            ('strict', ['node', 'tools/client/typecheck.mjs']),
            ('protocol-check', ['task', 'protocol-check', '--report-dir', str(out.relative_to(ROOT) / 'protocol')]),
            ('protocol-lint', ['python3', 'tools/config/generate_protocol.py', '--check'])]
    report = {'group': args.group, 'started_at_utc': now(), 'freeze_started_before_commands': True,
              'before': snapshot(enumerate_inputs()), 'commands': [], 'scope': '本阶段实际执行范围；不代替正常GUI、真机或完整容量验收'}
    if args.group != 'go':
        report['external_engine_before'] = {str(ENGINE / n): sha(ENGINE / n) for n in ENGINE_FILES}
        report['external_font_before'] = {str(p): sha(p) for p in FONTS}
    write(out / 'started.json', report)
    for name, command in checks:
        begin = time.monotonic()
        with (out / (name + '.stdout.txt')).open('wb') as stdout, (out / (name + '.stderr.txt')).open('wb') as stderr:
            result = subprocess.run(DEV + command, cwd=ROOT, stdout=stdout, stderr=stderr)
        row = {'name': name, 'command': DEV + command, 'exit_code': result.returncode, 'seconds': round(time.monotonic() - begin, 3),
               'stdout': (out / (name + '.stdout.txt')).relative_to(ROOT).as_posix(),
               'stderr': (out / (name + '.stderr.txt')).relative_to(ROOT).as_posix(),
               'stdout_sha256': sha(out / (name + '.stdout.txt')), 'stderr_sha256': sha(out / (name + '.stderr.txt'))}
        report['commands'].append(row)
        print(json.dumps(row, ensure_ascii=False), flush=True)
        if result.returncode:
            break
    report['after'] = snapshot(enumerate_inputs())
    report['input_count'] = len(report['before'])
    report['stable'] = report['before'] == report['after']
    if args.group != 'go':
        report['external_engine_after'] = {str(ENGINE / n): sha(ENGINE / n) for n in ENGINE_FILES}
        report['external_font_after'] = {str(p): sha(p) for p in FONTS}
        report['stable'] = report['stable'] and report['external_engine_before'] == report['external_engine_after'] and report['external_font_before'] == report['external_font_after']
    for original, target in ([('server/test-results/latest.json', 'report.json'), ('server/test-results/latest.events.jsonl', 'events.jsonl')] if args.group == 'go' else [('docs/客户端构建检查.json', 'report.json'), ('.work/creator-cli.log', 'creator.log')] if args.group == 'creator' else []):
        shutil.copyfile(ROOT / original, out / target)
    report['finished_at_utc'] = now()
    report['passed'] = report['stable'] and len(report['commands']) == len(checks) and all(c['exit_code'] == 0 for c in report['commands'])
    write(out / ('wrapper.json' if args.group in {'go', 'creator'} else 'report.json'), report)
    print(json.dumps({'passed': report['passed'], 'stable': report['stable'], 'inputs': report['input_count'], 'output': str(out)}, ensure_ascii=False))
    raise SystemExit(0 if report['passed'] else 1)


if __name__ == '__main__':
    main()
