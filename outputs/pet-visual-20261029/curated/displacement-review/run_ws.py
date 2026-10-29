"""只跑位移与召唤两条真实WS；先核所有权、停自有main，finally恢复main。"""
import datetime
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[3]
OUT = Path(__file__).resolve().parent

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def save(name, value):
    (OUT / name).write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')

def control(script, action, name):
    command = [sys.executable, str(ROOT / 'tools/dev'), 'python3', script, action]
    result = subprocess.run(command, cwd=ROOT, capture_output=True, encoding='utf-8', errors='replace', timeout=180)
    (OUT / (name + '.stdout.txt')).write_text(result.stdout, encoding='utf-8')
    (OUT / (name + '.stderr.txt')).write_text(result.stderr, encoding='utf-8')
    if result.returncode:
        raise RuntimeError(f'{name} exit {result.returncode}: {result.stderr[-1000:]}')
    value = json.loads(result.stdout)
    save(name + '.json', value)
    return value

spec = importlib.util.spec_from_file_location('controlled_server_tests', ROOT / 'tools/server/test.py')
test_tool = importlib.util.module_from_spec(spec)
spec.loader.exec_module(test_tool)

def hashes():
    return {path.relative_to(ROOT).as_posix(): sha(path) for path in test_tool.input_files()}

report = {'started_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
          'mongo_e2e_explicit': True, 'race_detector': True,
          'single_rpc_and_business_deadlines_changed': False, 'production_ready': False}
result_code = 1
must_restore = False
try:
    mongo = control('tools/environment/mongodb_dev.py', 'status', 'mongodb-before')
    config = json.loads((ROOT / 'conf/dev/server.json').read_text(encoding='utf-8'))
    expected_uri = config.get('mongo_uri_by_platform', {}).get('windows', config['mongo_uri'])
    if not all(mongo.get(k) for k in ['running', 'ownership_verified', 'listener_verified', 'endpoint_verified']) or mongo.get('uri') != expected_uri:
        raise RuntimeError('拒绝未核验的专属Mongo端点')
    report['mongo_ownership'] = mongo
    main_before = control('tools/server/dev.py', 'status', 'main-before')
    report['main_before'] = main_before
    state = json.loads((ROOT / 'server/.dev/runtime.json').read_text(encoding='utf-8'))
    save('main-process-before.json', state)
    if not main_before.get('running') or main_before.get('pid') != state.get('pid') or state.get('project_root') != str(ROOT):
        raise RuntimeError('原main非核验通过的当前项目进程')
    marker = json.loads((ROOT / '.work/pet-visual-20261029/server/go-frozen.json').read_text(encoding='utf-8'))
    if not marker.get('frozen') or any(sha(ROOT / name) != digest for name, digest in marker['go_sha256'].items()):
        raise RuntimeError('Go冻结标记不符')
    must_restore = True
    stopped = control('tools/server/dev.py', 'stop', 'main-stopped')
    if stopped.get('running'):
        raise RuntimeError('自有main未受控停止')
    before = hashes()
    report['source_sha256'] = before
    save('source-before.json', before)
    expected_tests = {'TestDisplacementSkillsLearningUpgradeCastingAndReconnectMongoWS', 'TestSummonPetMaterialRecallTrainingNoPetsAndColdMongoWS'}
    pattern = '^(' + '|'.join(sorted(expected_tests)) + ')$'
    command = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'test', '-json', '-race', '-p', '1', './internal/integration', '-count=1', '-timeout=300s', '-run', pattern]
    report['command'] = command
    save('running.json', {'command': command, 'started_at_utc': report['started_at_utc'], 'source_inputs': len(before)})
    print(json.dumps({'running': True, 'command': command, 'source_inputs': len(before)}, ensure_ascii=False), flush=True)
    start = time.monotonic()
    result = subprocess.run(command, cwd=ROOT, env=dict(os.environ, MINIMIR_MONGO_E2E='1'), capture_output=True, encoding='utf-8', errors='replace', timeout=420)
    report['test_seconds'] = round(time.monotonic() - start, 3)
    report['exit_code'] = result.returncode
    (OUT / 'focused.events.jsonl').write_text(result.stdout, encoding='utf-8')
    (OUT / 'focused.stderr.txt').write_text(result.stderr, encoding='utf-8')
    events = []
    for line in result.stdout.splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            pass
    report['passed_tests'] = [e['Test'] for e in events if e.get('Action') == 'pass' and e.get('Test')]
    report['failed_tests'] = [e.get('Test', e.get('Package')) for e in events if e.get('Action') in ['fail', 'build-fail']]
    report['skipped_tests'] = [e['Test'] for e in events if e.get('Action') == 'skip' and e.get('Test')]
    after = hashes()
    report['source_sha256_after'] = after
    report['source_stable'] = before == after
    save('source-after.json', after)
    result_code = result.returncode or (2 if before != after else 0)
    if set(report['passed_tests']) != expected_tests or report['skipped_tests']:
        result_code = result_code or 4
except Exception as error:
    report['error'] = str(error)
finally:
    if must_restore:
        try:
            restored = control('tools/server/dev.py', 'start', 'main-restored')
            report['main_after'] = restored
            fixture = json.loads((ROOT / 'config/development/first-loop.json').read_text(encoding='utf-8'))
            expected_hash = sha(ROOT / 'config/development/first-loop.json')
            healthy = restored.get('running') and set(restored.get('health', {})) == {'account', 'game', 'gm'} and all(
                h.get('status') == 'ok' and h.get('rule_version') == fixture['rule_version'] == 18 and h.get('fixture_hash') == expected_hash
                for h in restored['health'].values())
            report['main_restored_verified'] = bool(healthy)
            if not healthy:
                result_code = 3
        except Exception as error:
            report['main_restore_error'] = str(error)
            result_code = 3
    report['finished_at_utc'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    report['passed'] = result_code == 0
    save('focused-report.json', report)
    print(json.dumps({key: report.get(key) for key in ['passed', 'exit_code', 'test_seconds', 'source_stable', 'passed_tests', 'failed_tests', 'skipped_tests', 'main_restored_verified', 'error', 'main_restore_error']}, ensure_ascii=False), flush=True)
raise SystemExit(result_code)
