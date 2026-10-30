"""仅切换本任务静态预览，候选同卷搬入主目录；不停止后端或Mongo。"""
from pathlib import Path
import datetime
import hashlib
import json
import subprocess
import sys

WORK = Path(__file__).resolve().parent
ROOT = WORK.parents[1]
DEV = [sys.executable, str(ROOT / 'tools/dev')]
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
out = WORK / 'runtime'
out.mkdir(exist_ok=False)
report = {'passed': False, 'commands': [], 'started_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
          'backend_restarted': False, 'mongo_restarted': False, 'rollback_attempted': False}
main = ROOT / 'client/build/web-mobile'
candidate = WORK / 'creator/full-1/candidate/web-mobile'
backup, failed = out / 'previous-build', out / 'failed-build'
changed = False


def snapshot(folder):
    result = {}
    for p in folder.rglob('*'):
        assert not p.is_symlink() and not p.is_junction()
        if p.is_file():
            result[p.relative_to(folder).as_posix()] = sha(p)
    return result


def command(name, args, parse=True):
    stdout, stderr = out / (name + '.stdout.txt'), out / (name + '.stderr.txt')
    with stdout.open('xb') as a, stderr.open('xb') as b:
        result = subprocess.run(DEV + args, cwd=ROOT, stdout=a, stderr=b)
    report['commands'].append({'name': name, 'command': DEV + args, 'exit_code': result.returncode,
                               'stdout': stdout.relative_to(ROOT).as_posix(), 'stderr': stderr.relative_to(ROOT).as_posix(),
                               'stdout_sha256': sha(stdout), 'stderr_sha256': sha(stderr)})
    assert result.returncode == 0, name
    return read(stdout) if parse else None


try:
    for p in [main, candidate, backup, failed]:
        assert p.resolve().is_relative_to(ROOT.resolve()) and not p.is_symlink() and not p.is_junction()
    assert main.resolve() == ROOT / 'client/build/web-mobile' and candidate.resolve().is_relative_to(WORK.resolve())
    assert not backup.exists() and not failed.exists()
    build = read(WORK / 'creator/full-1/report.json')
    wrapper = read(WORK / 'creator/full-1/wrapper.json')
    client = read(WORK / 'client/full-2/report.json')
    assert wrapper['passed'] and client['passed'] and wrapper['before'] == wrapper['after'] == client['after']
    assert build['success'] and build['exit_code'] == 36 and build['served_build_preserved']
    for name, digest in client['after'].items():
        assert sha(ROOT / name) == digest
    assert Path(build['build_root']) == candidate
    manifest = read(Path(build['artifact_manifest']))
    expected = {(ROOT / p).relative_to(candidate).as_posix(): s for p, s in manifest.items()}
    previous = {(ROOT / p).relative_to(main).as_posix(): s for p, s in read(WORK / 'creator/full-1/served-build-after.json').items()}
    assert snapshot(main) == previous
    backend = command('backend-before', ['python3', 'tools/server/dev.py', 'status'])
    static = command('static-before', ['python3', 'tools/client/dev.py', 'status'])
    assert backend['running'] and all(v['status'] == 'ok' and v['rule_version'] == 23 for v in backend['health'].values())
    assert static['running'] and static['health']['port'] == 18080
    report['backend_before'], report['static_before'] = backend, static
    command('static-stop', ['python3', 'tools/client/dev.py', 'stop'], False)
    changed = True
    main.rename(backup)
    candidate.rename(main)
    command('static-start', ['python3', 'tools/client/dev.py', 'start'])
    after = command('static-after', ['python3', 'tools/client/dev.py', 'status'])
    assert after['running'] and after['health']['run_id'] != static['health']['run_id']
    assert snapshot(main) == expected
    backend_after = command('backend-after', ['python3', 'tools/server/dev.py', 'status'])
    assert backend_after == backend
    report.update(passed=True, served_candidate_exact_match=True, artifact_files=len(expected),
                  candidate_manifest_sha256=build['artifact_manifest_sha256'], backend_after=backend_after, static_after=after,
                  artifact_transfer='同卷rename，历史Creator build_root已搬入client/build/web-mobile，原路径不再存在')
except BaseException as error:
    report['error'] = {'type': type(error).__name__, 'message': str(error)}
    if changed:
        report['rollback_attempted'] = True
        try:
            command('rollback-static-stop', ['python3', 'tools/client/dev.py', 'stop'], False)
            if main.exists() and backup.exists():
                main.rename(failed)
            if backup.exists():
                backup.rename(main)
            command('rollback-static-start', ['python3', 'tools/client/dev.py', 'start'])
            report['rollback_restored'] = snapshot(main) == previous
        except BaseException as restore:
            report['rollback_error'] = str(restore)
    raise
finally:
    report['finished_at_utc'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    (out / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({k: report.get(k) for k in ['passed', 'served_candidate_exact_match', 'backend_restarted', 'mongo_restarted', 'rollback_attempted']}, ensure_ascii=False))
