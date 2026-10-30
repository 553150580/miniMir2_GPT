"""旧后端已终止时，受控恢复rule22预览并保留第一次预检失败原件。"""
from pathlib import Path
import datetime
import hashlib
import json
import runpy
import subprocess
import sys

WORK = Path(__file__).resolve().parent
ROOT = WORK.parents[1]
DEV = [sys.executable, str(ROOT / 'tools/dev')]
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
out = WORK / 'recovery-before-publication'
out.mkdir(exist_ok=False)
report = {'passed': False, 'commands': [], 'scope': '只恢复本任务已退出的rule22后端；不修改主静态包、不重启Mongo、不改永久配置',
          'started_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat()}


def command(name, args):
    stdout, stderr = out / (name + '.stdout.txt'), out / (name + '.stderr.txt')
    with stdout.open('xb') as output, stderr.open('xb') as errors:
        result = subprocess.run(DEV + args, cwd=ROOT, stdout=output, stderr=errors)
    report['commands'].append({'name': name, 'command': DEV + args, 'exit_code': result.returncode,
                               'stdout': stdout.relative_to(ROOT).as_posix(), 'stderr': stderr.relative_to(ROOT).as_posix(),
                               'stdout_sha256': sha(stdout), 'stderr_sha256': sha(stderr)})
    assert result.returncode == 0, name
    return read(stdout)


fixture = ROOT / 'config/development/first-loop.json'
original = fixture.read_bytes()
try:
    assert read(fixture)['rule_version'] == 23
    go = read(WORK / 'go/full-2/report.json')
    enumeration = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='recovery_inputs_only')['input_files']
    assert {p.relative_to(ROOT).as_posix(): sha(p) for p in enumeration()} == go['source_sha256_after']
    before = command('server-before', ['python3', 'tools/server/dev.py', 'status'])
    assert before['running'] is False
    mongo = command('mongo-before', ['python3', 'tools/environment/mongodb_dev.py', 'status'])
    report['mongo_before'] = mongo
    baseline = WORK / 'before/config/development/first-loop.json'
    assert read(baseline)['rule_version'] == 22
    expected = read(WORK / 'configuration-baseline.json')['files']['config/development/first-loop.json']
    assert sha(baseline) == expected
    fixture.write_bytes(baseline.read_bytes())
    try:
        started = command('server-start-rule22', ['python3', 'tools/server/dev.py', 'start'])
        assert started['running'] and set(started['health']) == {'account', 'game', 'gm'}
        assert all(q['status'] == 'ok' and q['rule_version'] == 22 and q['fixture_hash'] == expected for q in started['health'].values())
        report['restored_backend'] = started
    finally:
        fixture.write_bytes(original)
        report['disk_rule23_restored'] = fixture.read_bytes() == original
    after = command('mongo-after', ['python3', 'tools/environment/mongodb_dev.py', 'status'])
    keys = ('backend', 'pid', 'installation_id', 'project_key', 'data_path', 'uri', 'port', 'replica_set', 'version')
    assert all(mongo[k] == after[k] for k in keys)
    report['mongo_after'] = after
    source = (WORK / 'runtime-control').resolve()
    target = (WORK / 'runtime-history/preflight-unavailable-1').resolve()
    assert source == WORK / 'runtime-control' and source.is_relative_to(WORK.resolve()) and target.is_relative_to(WORK.resolve())
    assert not source.is_symlink() and not source.is_junction() and not target.exists()
    prior = read(source / 'report.json')
    assert prior['passed'] is False and prior['phase'] == 'preflight' and prior['rollback_attempted'] is False
    assert not (source / 'previous-build').exists() and not (source / 'failed-build').exists()
    target.parent.mkdir(exist_ok=True)
    before_sha = sha(source / 'report.json')
    source.rename(target)
    assert sha(target / 'report.json') == before_sha
    report['preserved_preflight_failure'] = (target / 'report.json').relative_to(ROOT).as_posix()
    report['preserved_preflight_sha256'] = before_sha
    report['passed'] = True
except BaseException as error:
    report['error'] = {'type': type(error).__name__, 'message': str(error)}
    raise
finally:
    if fixture.read_bytes() != original:
        fixture.write_bytes(original)
    report['disk_rule23_restored'] = fixture.read_bytes() == original
    report['finished_at_utc'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    (out / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'passed': report['passed'], 'disk_rule23_restored': report['disk_rule23_restored']}, ensure_ascii=False))
