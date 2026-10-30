"""检查真实终态后切换本任务预览；失败保留新旧目录并按受控工具恢复，不操作Mongo。"""
from pathlib import Path
import datetime, hashlib, json, runpy, shutil, subprocess, sys, time, urllib.request

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
DEV = [sys.executable, str(ROOT / 'tools/dev')]
MODES = {18100: 'account', 18101: 'game', 18102: 'gm'}
MONGO_IDENTITY = ('backend', 'pid', 'installation_id', 'project_key', 'data_path', 'uri', 'port', 'replica_set', 'version')


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def exact(path, expected):
    result = path.resolve()
    assert result == expected.absolute() and result.is_relative_to(ROOT.resolve()), '路径不是本任务精确目录：' + str(path)
    return result


def snapshot(folder):
    result = {}
    for path in [folder, *sorted(folder.rglob('*'))]:
        assert not path.is_symlink() and not path.is_junction(), '产物中不能有符号链接或junction'
        if path.is_file():
            result[path.relative_to(folder).as_posix()] = sha(path)
    return result


def stable(values, external=False):
    for name, expected in values.items():
        path = Path(name) if external else ROOT / name
        if not external:
            assert path.resolve().is_relative_to(ROOT.resolve())
        assert sha(path) == expected, '已验输入变化：' + name


def saved_layout_changes(client):
    """按已提交原场景逐对象核对，新增仓库只追加两个指定父节点的children。"""
    base = 'd2dab71909d4c389482be9b95d9594db76310eeb'
    assert subprocess.check_output(DEV + ['git', 'rev-parse', 'HEAD'], cwd=ROOT).strip().decode() == base
    layouts = {name: value for name, value in client['after'].items() if Path(name).suffix in {'.scene', '.prefab'}}
    assert len(layouts) == 10
    scene = 'client/assets/scenes/Game.scene'
    assert scene in layouts
    for name, digest in layouts.items():
        original = subprocess.check_output(DEV + ['git', 'show', base + ':' + name], cwd=ROOT)
        if name != scene:
            assert hashlib.sha256(original).hexdigest() == digest, '未声明的旧布局变化：' + name
            continue
        before, after = json.loads(original), read(ROOT / name)
        assert len(after) > len(before) and sha(ROOT / name) == digest
        for index, old in enumerate(before):
            current = after[index]
            if index in (253, 395):
                assert old['__type__'] == current['__type__'] == 'cc.Node'
                assert old['_name'] == current['_name'] == {253: 'Panels', 395: 'Page0'}[index]
                assert current['_children'][:len(old['_children'])] == old['_children']
                appended = current['_children'][len(old['_children']):]
                assert len(appended) == 1 and appended[0]['__id__'] >= len(before)
                assert after[appended[0]['__id__']]['_name'] == 'Warehouse'
                current = dict(current)
                current['_children'] = old['_children']
            assert old == current, '旧保存对象字段变化：' + str(index)
    return {'unchanged_saved_layouts': 9, 'intentionally_extended_layouts': [scene],
            'original_scene_objects': len(before), 'appended_scene_objects': len(after)-len(before),
            'old_object_fields_preserved_except_declared_child_append': True}


def health():
    result = {}
    for port in [18080, *MODES]:
        with urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz', timeout=5) as response:
            assert response.status == 200
            result[str(port)] = json.load(response)
    return result


def mongo_valid(value, expected_uri):
    assert value['running'] and value['endpoint_verified'] and value['listener_verified'] and value['ownership_verified']
    assert value['uri'] == expected_uri and value['backend'] == 'windows_native' and value['port'] == 27119
    assert all(value.get(key) not in (None, '') for key in MONGO_IDENTITY)


def mongo_same(before, after):
    mongo_valid(after, before['uri'])
    assert all(after[key] == before[key] for key in MONGO_IDENTITY), 'Mongo实际进程/安装/目录身份变化'


def attachment_health(value, backend, static, served, rule, fixture_hash):
    assert backend['running'] and static['running']
    for port, mode in MODES.items():
        observed = value[str(port)]
        assert observed['status'] == 'ok' and observed['mode'] == mode
        assert observed['rule_version'] == rule and observed['fixture_hash'] == fixture_hash
        assert observed['instance'] == backend['health'][mode]['instance']
    node = value['18080']
    state = read(ROOT / '.work/client-dev/state.json')
    assert Path(state['project_root']).resolve() == ROOT.resolve()
    assert node['webMobileBuild'] and Path(node['build_root']).resolve() == served
    assert node['port'] == 18080 and node['account_port'] == 18100 and node['game_port'] == 18101
    assert node['pid'] == state['pid'] and node['run_id'] == state['run_id']
    assert static['health']['pid'] == node['pid'] and static['health']['run_id'] == node['run_id']


def main():
    out = exact(WORK / 'runtime-control', WORK / 'runtime-control')
    out.mkdir(exist_ok=False)
    report = {'started_at_utc': now(), 'commands': [], 'phase': 'preflight', 'passed': False,
              'scope': '候选真实产物切换及本任务受控起停；Mongo仅status，不接来源服或其它服务',
              'other_ports_touched': [], 'rollback_attempted': False, 'rollback_restored': False}
    write(out / 'report.json', report)
    mutation_started = False
    disk23 = None

    def command(name, arguments, parse_json=True):
        began = time.monotonic()
        with (out / (name + '.stdout.txt')).open('wb') as stdout, (out / (name + '.stderr.txt')).open('wb') as stderr:
            result = subprocess.run(DEV + arguments, cwd=ROOT, stdout=stdout, stderr=stderr)
        row = {'name': name, 'command': DEV + arguments, 'exit_code': result.returncode,
               'seconds': round(time.monotonic() - began, 3), 'parse_json': parse_json}
        for stream in ['stdout', 'stderr']:
            path = out / (name + '.' + stream + '.txt')
            row[stream] = path.relative_to(ROOT).as_posix()
            row[stream + '_sha256'] = sha(path)
        report['commands'].append(row)
        write(out / 'report.json', report)
        print(json.dumps(row, ensure_ascii=False), flush=True)
        assert result.returncode == 0, '受控命令失败：' + name
        return read(out / (name + '.stdout.txt')) if parse_json else None

    def current_go():
        definition = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='cutover_only_enumerate')
        return {path.relative_to(ROOT).as_posix(): sha(path) for path in definition['input_files']()}

    def current_client():
        definition = runpy.run_path(str(WORK / 'stage_checks.py'), run_name='cutover_only_enumerate')
        return {path.relative_to(ROOT).as_posix(): sha(path) for path in definition['client_inputs']()}

    try:
        fixture = exact(ROOT / 'config/development/first-loop.json', ROOT / 'config/development/first-loop.json')
        served = exact(ROOT / 'client/build/web-mobile', ROOT / 'client/build/web-mobile')
        staged = exact(ROOT / 'client/build/warehouses-stage23', ROOT / 'client/build/warehouses-stage23')
        backup = exact(out / 'previous-build', WORK / 'runtime-control/previous-build')
        failed = exact(out / 'failed-build', WORK / 'runtime-control/failed-build')
        # 只消费已终态真实检查。重新枚举还能拒绝测试后新增/删除的输入。
        go_dir = WORK / 'go/full-2'
        go, go_wrapper = read(go_dir / 'report.json'), read(go_dir / 'wrapper.json')
        assert go_wrapper['passed'] and go_wrapper['stable'] and go_wrapper['before'] == go_wrapper['after']
        assert go['passed'] and go['exit_code'] == 0 and not go['failed_tests'] and not go['skipped_tests']
        assert go['mongo_e2e_explicit'] and go['race_detector']
        assert go['command'][2:] == ['go', 'test', '-json', '-race', '-p', '1', './...', '-count=1', '-timeout=1200s'], '必须是本阶段真实全包命令，不能以相同输入枚举的子集替代'
        assert go['package_parallelism'] == 1 and go['package_timeout_seconds'] == 1200 and go['single_rpc_and_business_deadlines_changed'] is False
        assert go['passed_tests'] and sha(go_dir / 'events.jsonl') == go['events_sha256']
        required = {'TestWarehouseTransactionsSplitsLegacyReceiptsAndFencesMongo',
                    'TestWarehousesTwoRolesAtomicTransferAndReconnectMongoWS',
                    'TestWarehouseConcurrentFifthRoleAndSixthRejectedMongoHTTP',
                    'TestWarehousePermanentOriginsShareRunButNotAssetsMongo'}
        assert required <= {row['test'] for row in go['passed_tests'] if row['package'] == 'minimir/internal/integration'}, '全包结果缺少本阶段必要真实业务用例'
        assert current_go() == go['source_sha256'] == go['source_sha256_after'] == go_wrapper['after']
        client = read(WORK / 'client/full-3/report.json')
        assert client['passed'] and client['stable'] and client['input_count'] == len(client['before'])
        assert current_client() == client['before'] == client['after']
        report['saved_layout_changes'] = saved_layout_changes(client)
        creator_wrapper = read(WORK / 'creator/full-3/wrapper.json')
        assert creator_wrapper['passed'] and creator_wrapper['stable']
        assert creator_wrapper['before'] == creator_wrapper['after'] == client['after']
        for prefix in ['external_engine', 'external_font']:
            assert client[prefix + '_before'] == client[prefix + '_after'] == creator_wrapper[prefix + '_before'] == creator_wrapper[prefix + '_after']
            stable(creator_wrapper[prefix + '_after'], external=True)
        build = read(WORK / 'creator/full-3/report.json')
        assert build['success'] and build['exit_code'] == 36 and build['creator'] == '3.8.8' and build['served_build_preserved']
        assert build['scene_prefab_layout_preserved'] and build['source_stable_during_build']
        assert build['asset_hashes'] == {key: value for key, value in client['after'].items() if Path(key).suffix in {'.scene', '.prefab'}}
        assert build['script_hashes'] == {key: value for key, value in client['after'].items() if key.startswith('client/assets/scripts/') and Path(key).suffix in {'.ts', '.js'}}
        stable({**build['asset_hashes'], **build['script_hashes']})
        assert sha(ROOT / 'conf/creator-build.json') == build['original_build_config_sha256']
        controlled_config = exact(Path(build['build_config']), WORK / 'creator/full-3/creator-build-candidate.json')
        assert sha(controlled_config) == build['build_config_sha256']
        candidate = exact(Path(build['build_root']), WORK / 'creator/full-3/candidate/web-mobile')
        manifest_path = exact(Path(build['artifact_manifest']), WORK / 'creator/full-3/candidate-files.json')
        assert sha(manifest_path) == build['artifact_manifest_sha256']
        original_manifest = read(manifest_path)
        expected = {str((ROOT / key).relative_to(candidate)).replace('\\', '/'): value for key, value in original_manifest.items()}
        assert snapshot(candidate) == expected and 'index.html' in expected and len(expected) == build['artifact_file_count']
        old_manifest = {str((ROOT / key).relative_to(served)).replace('\\', '/'): value
                        for key, value in read(WORK / 'creator/full-3/served-build-after.json').items()}
        assert snapshot(served) == old_manifest, '旧预览包自候选构建后已变化'
        config = read(fixture)
        assert config['rule_version'] == 23
        disk23 = fixture.read_bytes()
        fixture_hash = sha(fixture)
        config22 = exact(WORK / 'before/config/development/first-loop.json', WORK / 'before/config/development/first-loop.json')
        baseline_hash = read(WORK / 'configuration-baseline.json')['files']['config/development/first-loop.json']
        assert sha(config22) == baseline_hash and read(config22)['rule_version'] == 22
        configured = read(ROOT / 'conf/dev/server.json')
        expected_uri = configured.get('mongo_uri_by_platform', {}).get('windows', configured['mongo_uri'])
        mongo = command('mongo-before', ['python3', 'tools/environment/mongodb_dev.py', 'status'])
        mongo_valid(mongo, expected_uri)
        backend = command('server-before', ['python3', 'tools/server/dev.py', 'status'])
        static = command('client-before', ['python3', 'tools/client/dev.py', 'status'])
        report['before'] = health()
        attachment_health(report['before'], backend, static, served, 22, baseline_hash)
        write(WORK / 'runtime-before.json', report['before'])
        report['candidate_manifest_sha256'] = build['artifact_manifest_sha256']
        report['mongo_before'] = {key: mongo[key] for key in MONGO_IDENTITY}
        report['phase'] = 'stage_candidate'
        assert not staged.exists() and not backup.exists() and not failed.exists()
        shutil.copytree(candidate, staged)
        assert snapshot(staged) == expected
        report['staged_artifact_files'] = len(expected)
        # 先完成预拷贝，再进入最短的受控停止窗口；任何异常不写PASS。
        report['phase'] = 'switch_owned_services'
        mutation_started = True
        stopped = command('server-stop', ['python3', 'tools/server/dev.py', 'stop'])
        assert not stopped['running']
        command('client-stop', ['python3', 'tools/client/dev.py', 'stop'], parse_json=False)
        assert not command('client-stopped-status', ['python3', 'tools/client/dev.py', 'status'])['running']
        exact(served, ROOT / 'client/build/web-mobile').rename(exact(backup, WORK / 'runtime-control/previous-build'))
        exact(staged, ROOT / 'client/build/warehouses-stage23').rename(exact(served, ROOT / 'client/build/web-mobile'))
        report['phase'] = 'start_candidate'
        backend_after = command('server-start', ['python3', 'tools/server/dev.py', 'start'])
        command('client-start', ['python3', 'tools/client/dev.py', 'start'])
        static_after = command('client-after', ['python3', 'tools/client/dev.py', 'status'])
        report['after'] = health()
        attachment_health(report['after'], backend_after, static_after, served, 23, fixture_hash)
        assert all(report['before'][str(port)]['instance'] != report['after'][str(port)]['instance'] for port in MODES)
        assert report['before']['18080']['run_id'] != report['after']['18080']['run_id']
        assert snapshot(served) == expected
        mongo_after = command('mongo-after', ['python3', 'tools/environment/mongodb_dev.py', 'status'])
        mongo_same(mongo, mongo_after)
        assert current_go() == go['source_sha256_after'] and current_client() == client['after']
        report['served_candidate_exact_match'] = True
        report['mongo_after'] = {key: mongo_after[key] for key in MONGO_IDENTITY}
        report['mongo_service_restarted'] = False
        write(WORK / 'runtime-after.json', report['after'])
        report['phase'] = 'completed'
        report['passed'] = True
    except BaseException as error:
        report['passed'] = False
        report['error'] = {'type': type(error).__name__, 'message': str(error), 'phase': report['phase']}
        if mutation_started:
            report['rollback_attempted'] = True
            report['phase'] = 'rollback'
            try:
                stopped = command('rollback-server-stop', ['python3', 'tools/server/dev.py', 'stop'])
                assert not stopped['running']
                command('rollback-client-stop', ['python3', 'tools/client/dev.py', 'stop'], parse_json=False)
                assert not command('rollback-client-status-stopped', ['python3', 'tools/client/dev.py', 'status'])['running']
                if backup.exists():
                    assert not failed.exists(), '失败包目录已存在，拒绝覆盖'
                    if served.exists():
                        exact(served, ROOT / 'client/build/web-mobile').rename(exact(failed, WORK / 'runtime-control/failed-build'))
                    elif staged.exists():
                        exact(staged, ROOT / 'client/build/warehouses-stage23').rename(exact(failed, WORK / 'runtime-control/failed-build'))
                    exact(backup, WORK / 'runtime-control/previous-build').rename(exact(served, ROOT / 'client/build/web-mobile'))
                assert snapshot(served) == old_manifest, '旧包未恢复，不能声称rollback通过'
                assert current_go() == go['source_sha256_after'], '已测输入变化，拒绝以未测源码编译恢复'
                assert sha(config22) == baseline_hash and all(report['before'][str(port)]['fixture_hash'] == baseline_hash for port in MODES)
                assert fixture.read_bytes() == disk23, '磁盘23已被并发更改，拒绝覆盖其它写入'
                # 仅恢复窗口临时使用已核22原字节；无论启动/健康是否成功，finally恢复磁盘23。
                try:
                    fixture.write_bytes(config22.read_bytes())
                    rollback_backend = command('rollback-server-start', ['python3', 'tools/server/dev.py', 'start'])
                    command('rollback-client-start', ['python3', 'tools/client/dev.py', 'start'])
                    rollback_static = command('rollback-client-after', ['python3', 'tools/client/dev.py', 'status'])
                    rollback_health = health()
                    attachment_health(rollback_health, rollback_backend, rollback_static, served, 22, baseline_hash)
                finally:
                    fixture.write_bytes(disk23)
                    report['rollback_disk_rule23_restored'] = sha(fixture) == fixture_hash
                assert report['rollback_disk_rule23_restored']
                assert snapshot(served) == old_manifest
                rollback_mongo = command('rollback-mongo-status', ['python3', 'tools/environment/mongodb_dev.py', 'status'])
                mongo_same(mongo, rollback_mongo)
                report['rollback_health'] = rollback_health
                report['rollback_mongo'] = {key: rollback_mongo[key] for key in MONGO_IDENTITY}
                report['rollback_restored'] = True
                report['mongo_service_restarted'] = False
            except BaseException as rollback_error:
                report['rollback_error'] = {'type': type(rollback_error).__name__, 'message': str(rollback_error)}
                report['rollback_restored'] = False
                if disk23 is not None:
                    report['rollback_disk_rule23_restored'] = fixture.is_file() and fixture.read_bytes() == disk23
        raise
    finally:
        report['finished_at_utc'] = now()
        write(out / 'report.json', report)
        print(json.dumps({'passed': report['passed'], 'phase': report['phase'], 'rollback_restored': report['rollback_restored']}, ensure_ascii=False), flush=True)


if __name__ == '__main__':
    main()
