"""实际检查通过后切换本任务预览；所有目录、进程和数据库均核对所有权。"""
from pathlib import Path
import datetime, hashlib, json, shutil, subprocess, sys, time, urllib.request

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
DEV = [sys.executable, str(ROOT / 'tools/dev')]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def snapshot(folder):
    result = {}
    for path in sorted(folder.rglob('*')):
        assert not path.is_symlink(), '产物中不能有符号链接'
        if path.is_file():
            result[path.relative_to(folder).as_posix()] = sha(path)
    return result


def stable(values):
    for name, expected in values.items():
        assert sha(ROOT / name) == expected, '已验输入变化：' + name


def health():
    result = {}
    for port in [18080, 18100, 18101, 18102]:
        with urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz', timeout=5) as response:
            assert response.status == 200
            result[str(port)] = json.load(response)
    return result


def main():
    go_dir = WORK / 'go/full-1'
    go, go_wrapper = read(go_dir / 'report.json'), read(go_dir / 'wrapper.json')
    assert go_wrapper['passed'] and go_wrapper['stable']
    assert go['passed'] and go['exit_code'] == 0 and not go['failed_tests'] and not go['skipped_tests']
    assert go['mongo_e2e_explicit'] and go['race_detector'] and go['source_sha256'] == go['source_sha256_after']
    stable(go['source_sha256'])
    client = read(WORK / 'client/full-1/report.json')
    assert client['passed'] and client['stable'] and client['before'] == client['after']
    stable(client['before'])
    build_path = WORK / 'creator/full-1/report.json'
    build = read(build_path)
    assert build['success'] and build['exit_code'] == 36 and build['served_build_preserved']
    assert build['scene_prefab_layout_preserved'] and build['source_stable_during_build']
    stable({**build['asset_hashes'], **build['script_hashes']})
    candidate = Path(build['build_root']).resolve()
    assert candidate == (WORK / 'creator/full-1/candidate/web-mobile').resolve()
    original_manifest = read(Path(build['artifact_manifest']))
    assert sha(Path(build['artifact_manifest'])) == build['artifact_manifest_sha256']
    expected = {str((ROOT / key).relative_to(candidate)).replace('\\', '/'): value for key, value in original_manifest.items()}
    assert snapshot(candidate) == expected and 'index.html' in expected
    served = (ROOT / 'client/build/web-mobile').resolve()
    assert served.is_relative_to(ROOT.resolve()) and served == ROOT / 'client/build/web-mobile'
    current_served = {str((ROOT / key).relative_to(served)).replace('\\', '/'): value
                      for key, value in read(WORK / 'creator/full-1/served-build-after.json').items()}
    assert snapshot(served) == current_served, '旧预览包自候选构建后已变化'
    config = read(ROOT / 'config/development/first-loop.json')
    assert config['rule_version'] == 22
    fixture_hash = sha(ROOT / 'config/development/first-loop.json')
    out = WORK / 'runtime-control'
    out.mkdir(exist_ok=False)
    report = {'started_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'commands': [],
              'scope': '候选真实产物切换及本任务受控起停；不操作来源服、其它服务或已有数据库',
              'passed': False, 'other_ports_touched': [], 'candidate_manifest_sha256': build['artifact_manifest_sha256']}

    def command(name, arguments):
        began = time.monotonic()
        with (out / (name + '.stdout.txt')).open('wb') as stdout, (out / (name + '.stderr.txt')).open('wb') as stderr:
            result = subprocess.run(DEV + arguments, cwd=ROOT, stdout=stdout, stderr=stderr)
        row = {'name': name, 'command': DEV + arguments, 'exit_code': result.returncode,
               'seconds': round(time.monotonic() - began, 3)}
        for stream in ['stdout', 'stderr']:
            path = out / (name + '.' + stream + '.txt')
            row[stream] = path.relative_to(ROOT).as_posix()
            row[stream + '_sha256'] = sha(path)
        report['commands'].append(row)
        write(out / 'report.json', report)
        print(json.dumps(row, ensure_ascii=False), flush=True)
        assert result.returncode == 0, '受控命令失败，保留原输出和目录以核查：' + name
        return read(out / (name + '.stdout.txt'))

    mongo = command('mongo-before', ['python3', 'tools/environment/mongodb_dev.py', 'status'])
    assert mongo['running'] and mongo['endpoint_verified'] and '27119' in mongo['uri']
    backend = command('server-before', ['python3', 'tools/server/dev.py', 'status'])
    static = command('client-before', ['python3', 'tools/client/dev.py', 'status'])
    assert backend['running'] and static['running']
    report['before'] = health()
    assert all(report['before'][str(port)]['rule_version'] == 21 for port in [18100, 18101, 18102])
    write(WORK / 'runtime-before.json', report['before'])
    staged = (ROOT / 'client/build/equipment-stage22').resolve()
    backup = (out / 'previous-build').resolve()
    assert staged.is_relative_to(ROOT.resolve()) and backup.is_relative_to(WORK.resolve())
    assert not staged.exists() and not backup.exists()
    shutil.copytree(candidate, staged)
    assert snapshot(staged) == expected
    report['staged_artifact_files'] = len(expected)
    command('server-stop', ['python3', 'tools/server/dev.py', 'stop'])
    command('client-stop', ['python3', 'tools/client/dev.py', 'stop'])
    assert served == ROOT / 'client/build/web-mobile' and backup.parent == out.resolve()
    served.rename(backup)
    staged.rename(served)
    command('server-start', ['python3', 'tools/server/dev.py', 'start'])
    command('client-start', ['python3', 'tools/client/dev.py', 'start'])
    report['after'] = health()
    assert all(report['after'][str(port)]['rule_version'] == 22 and report['after'][str(port)]['fixture_hash'] == fixture_hash
               for port in [18100, 18101, 18102])
    assert all(report['before'][str(port)]['instance'] != report['after'][str(port)]['instance'] for port in [18100, 18101, 18102])
    assert report['before']['18080']['run_id'] != report['after']['18080']['run_id']
    assert snapshot(served) == expected
    mongo_after = command('mongo-after', ['python3', 'tools/environment/mongodb_dev.py', 'status'])
    assert mongo_after['running'] and mongo_after['endpoint_verified'] and mongo_after['uri'] == mongo['uri']
    report['served_candidate_exact_match'] = True
    report['mongo_service_restarted'] = False
    report['passed'] = True
    report['finished_at_utc'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    write(WORK / 'runtime-after.json', report['after'])
    write(out / 'report.json', report)
    print(json.dumps({'passed': True, 'rule_version': 22, 'artifact_files': len(expected)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
