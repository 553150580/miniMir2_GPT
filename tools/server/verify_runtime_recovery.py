"""隔离验证受伤现场、健康短冷却和整进程故障期间的托管账本。"""
from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import queue
import subprocess
import sys
import time
import uuid
from pathlib import Path

import verify_startup_recovery as isolation

ROOT = isolation.ROOT
WORK = ROOT / '.work/runtime-recovery-20261101'
PROBE = ROOT / 'tools/server/runtime_recovery_probe.cjs'
DB_PREFIX = 'minimir_dev_game_test_rt_'
PURPOSE = 'isolated_runtime_recovery'


class RuntimeProbe(isolation.Probe):
    def action(self, action, **fields):
        # 自然受伤及原怪物复活有独立等待；不放宽1800ms冷却或业务RPC期限。
        self.handle['child'].stdin.write((json.dumps({'action': action, **fields}) + '\n').encode('utf-8'))
        self.handle['child'].stdin.flush()
        try:
            line = self.lines.get(timeout=60)
        except queue.Empty as error:
            raise RuntimeError('协议探针动作超时：' + action) from error
        if line is None:
            raise RuntimeError('协议探针提前退出')
        result = json.loads(line)
        if result.get('action') != action or not result.get('passed'):
            self.failure = {'action': action, 'error': result.get('error'), 'diagnostic': result.get('diagnostic')}
            raise RuntimeError('协议探针失败：' + action + '；' + str(result.get('error', '无结果')))
        return result


def inputs():
    result = isolation.input_snapshot()
    paths = [Path(__file__), PROBE, ROOT / 'tools/server/verify_startup_recovery.py']
    paths += list((ROOT / 'outputs/development-prep-20261026').glob('*.xlsx'))
    paths += list((ROOT / 'client/assets').rglob('*.scene'))
    paths += list((ROOT / 'client/assets').rglob('*.prefab'))
    result.update({path.relative_to(ROOT).as_posix(): isolation.sha(path) for path in paths})
    return dict(sorted(result.items()))


def preflight():
    prepared = isolation.preflight()
    isolation.checked_command([prepared['node'], '--check', str(PROBE)])
    return prepared


def digest(value):
    # BSON的日期和整数类型用规范扩展JSON参与摘要；不把角色或凭据正文写入报告。
    from bson.json_util import CANONICAL_JSON_OPTIONS, dumps
    raw = dumps(value, json_options=CANONICAL_JSON_OPTIONS, sort_keys=True, separators=(',', ':'))
    return hashlib.sha256(raw.encode('utf-8')).hexdigest()


def role_projection(role):
    # 新票接管只允许以下控制字段换代；全部永久玩法字段仍逐字比较。
    controls = {'owner_instance', 'run_epoch', 'session_epoch', 'revision'}
    return {key: value for key, value in role.items() if key not in controls}


def ledger(db, role_id):
    from pymongo.read_concern import ReadConcern
    # 健康离线正持续提交；三类文档必须来自同一快照，不能把跨事务读当成不一致。
    with db.client.start_session() as session:
        with session.start_transaction(read_concern=ReadConcern('snapshot')):
            role = db.roles.find_one({'_id': role_id}, session=session)
            receipts = list(db.operations.find({'role_id': role_id}, session=session).sort('_id', 1))
            audits = list(db.audit_outbox.find({'role_id': role_id}, session=session).sort('_id', 1))
    if role is None:
        raise RuntimeError('本轮角色不存在')
    settlements = [row for row in receipts if row['kind'] == 'settlement']
    events = [row for row in audits if row['kind'] == 'settlement']
    if {row['_id'] for row in settlements} != {row['_id'] for row in events}:
        raise RuntimeError('永久结算回执与审计身份不一致')
    for row in settlements:
        if row.get('settlement') is None or row['_id'] != f"settle:{role_id}:{row['source_seq']}":
            raise RuntimeError('原收益来源缺少typed回执')
        event = next(event for event in events if event['_id'] == row['_id'])
        if row['source_seq'] != event['source_seq'] or row['settlement'] != event['settlement']:
            raise RuntimeError('原收益来源回执与审计不匹配')
    seconds = sum(row['host_seconds'] for row in events)
    if role['hosting_seconds'] != 21600 - seconds:
        raise RuntimeError('原6小时余额与实际托管审计扣时不守恒')
    return {'role': role, 'receipts': receipts, 'audits': audits,
            'summary': {'hosting_seconds': role['hosting_seconds'], 'source_seq': role['source_seq'],
                        'economic_command_seq': role['economic_command_seq'],
                        'report_used_seconds': role['hosting_report']['used_seconds'],
                        'report_kills': role['hosting_report']['kills'],
                        'settlement_count': len(settlements), 'operation_count': len(receipts),
                        'audit_count': len(audits), 'audited_host_seconds': seconds,
                        'audited_kills': sum(event['kills'] for event in events),
                        'role_sha256': digest(role), 'permanent_sha256': digest(role_projection(role)),
                        'receipts_sha256': digest(receipts), 'audits_sha256': digest(audits)}}


def assert_same_ledger(before, after, entered=False):
    if before['receipts'] != after['receipts'] or before['audits'] != after['audits']:
        raise RuntimeError('停止/故障重建窗口生成或修改了资产回执/审计')
    if role_projection(before['role']) != role_projection(after['role']):
        raise RuntimeError('停止/故障重建窗口改变了永久角色事实')
    if not entered and before['role'] != after['role']:
        raise RuntimeError('未附着时角色发生变化')
    if entered:
        for key in ['revision', 'session_epoch']:
            if after['role'][key] != before['role'][key] + 1:
                raise RuntimeError('冷新票没有恰好进行一次角色接管：' + key)
        if after['role']['run_epoch'] <= before['role']['run_epoch']:
            raise RuntimeError('冷新票角色run_epoch没有换代')


def crash_and_restart(db, game, config, argv, binary, directory, handles, label, role_id):
    original = db.run_control.find_one({'_id': config['run_id']})
    isolation.stop_owned(game, force=True)
    stopped = time.monotonic()
    # 强杀瞬间已发往Mongo的事务仍可能完成；先建立稳定的已提交边界，再观测故障窗口。
    prior = ledger(db, role_id)
    stable_since = time.monotonic()
    deadline = stopped + 5
    while time.monotonic() - stable_since < .8:
        time.sleep(.1)
        current = ledger(db, role_id)
        if current != prior:
            prior, stable_since = current, time.monotonic()
        if time.monotonic() > deadline:
            raise RuntimeError('强杀后的已发起事务未形成稳定边界')
    snapshot = prior
    samples, sample_records = 0, []
    deadline = stopped + 25
    while time.monotonic() < deadline:
        assert_same_ledger(snapshot, ledger(db, role_id))
        samples += 1
        sample_records.append({'seconds_after_stop': round(time.monotonic() - stopped, 3),
                               'ledger_sha256': digest(snapshot['summary'])})
        expired = db.run_control.find_one({'_id': config['run_id'], 'owner_instance': original['owner_instance'],
                                          'run_epoch': original['run_epoch'],
                                          '$expr': {'$lte': ['$lease_until', '$$NOW']}})
        if expired and time.monotonic() - stable_since >= 3:
            break
        time.sleep(.2)
    else:
        raise RuntimeError('旧run租约未自然到期，拒绝接管')
    stopped_window = time.monotonic() - stable_since
    game = isolation.launch(argv('game'), binary, directory / f'game-{label}.log', handles)
    isolation.health_ready(config, [game])
    replacement = db.run_control.find_one({'_id': config['run_id']})
    if replacement['run_epoch'] <= original['run_epoch'] or replacement['owner_instance'] == original['owner_instance']:
        raise RuntimeError('新game没有完成所有者换代')
    # 新进程无连接时也不得按停机墙钟补算收益或补扣托管。
    for _ in range(8):
        time.sleep(.2)
        assert_same_ledger(snapshot, ledger(db, role_id))
    return game, snapshot, {'game_force_stop_own_pid': True, 'lease_expired_naturally': True,
                           'old_run_epoch': original['run_epoch'], 'new_run_epoch': replacement['run_epoch'],
                           'stopped_stable_window_seconds': round(stopped_window, 3),
                           'stopped_ledger_samples': samples, 'unattached_restart_samples': 8,
                           'stopped_sample_records': sample_records,
                           'boundary_semantics': '自有PID已终止后，已发起事务收敛的稳定提交边界；不认证kill瞬间未知提交',
                           'stopped_ledger_unchanged': True, 'unattached_restart_ledger_unchanged': True,
                           'committed_boundary': snapshot['summary']}


def cleanup(handles, client, database, marker, config_path, config_sha, directory, prepared, report):
    errors = []
    for handle in reversed(handles):
        try:
            child = handle.get('child')
            if child is not None and child.stdin is not None and child.poll() is None:
                child.stdin.close()
                try:
                    child.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    # 探针可能仍在等待自然受伤；随后仍走原身份围栏回收自有Node。
                    pass
            isolation.stop_owned(handle)
        except Exception as error:
            errors.append(str(error))
    report['own_processes_stopped'] = not errors
    report['process_exits'] = [{'pid': h['meta']['pid'] if h.get('meta') else None,
                              'exit_code': h['child'].poll(), 'forced_game': h.get('forced', False)}
                             for h in handles if h.get('child') is not None]
    report['race_diagnostics_absent'] = not any(b'WARNING: DATA RACE' in path.read_bytes()
                                               for path in directory.glob('*.log'))
    if client is not None:
        try:
            if marker is not None and not errors:
                if client[database].application_meta.find_one({'_id': marker['_id']}) != marker:
                    raise RuntimeError('本轮库marker变化，拒绝删除')
                client.drop_database(database)
                report['new_database_dropped'] = database not in client.list_database_names()
        except Exception as error:
            errors.append(str(error))
        client.close()
    if config_path is not None and config_sha is not None:
        try:
            if isolation.sha(config_path) != config_sha:
                raise RuntimeError('本轮临时配置变化，拒绝删除')
            config_path.unlink()
            report['temporary_config_removed'] = True
        except Exception as error:
            errors.append(str(error))
    try:
        report['after'] = {'mongo': isolation.mongo_identity(), 'main': isolation.main_identity()}
        report['main_preserved'] = report['after']['main'] == prepared['main']
        report['mongo_preserved'] = report['after']['mongo'] == prepared['mongo']
        report['source_after'] = inputs()
        report['source_stable'] = report['source_before'] == report['source_after']
    except Exception as error:
        errors.append(str(error))
    report['cleanup_errors'] = errors


def run():
    prepared = preflight()
    attempt = uuid.uuid4().hex
    directory = WORK / (dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ-') + attempt[:8])
    if not directory.resolve().is_relative_to(WORK.resolve()):
        raise RuntimeError('本轮工作目录越界')
    directory.mkdir(parents=True)
    report = {'schema_version': 1, 'attempt': attempt, 'passed': False,
              'started_at_utc': dt.datetime.now(dt.timezone.utc).isoformat(),
              'scope': '健康受伤与短冷却、整进程强杀冷首基线、已提交托管故障窗口',
              'before': prepared, 'source_before': inputs(), 'checks': [],
              'production_ready': False, 'runtime_ready': False, 'full_p0_completed': False,
              'full_fault_matrix_verified': False, 'capacity_verified': False, 'physical_mobile_verified': False,
              'room_panic_verified': False, 'network_partition_verified': False,
              'pending_commit_fault_verified': False, 'credentials_written': False,
              'lease_modified_by_probe': False}
    handles, client, marker, config_path, config_sha, probe = [], None, None, None, None, None
    database = DB_PREFIX + attempt
    report['database'] = database
    try:
        from pymongo import MongoClient
        from pymongo.write_concern import WriteConcern
        client = MongoClient(prepared['mongo']['uri'], serverSelectionTimeoutMS=5000)
        if database in client.list_database_names() or len(database.encode()) > 63:
            raise RuntimeError('独立随机测试库身份不符或已存在')
        db = client.get_database(database, write_concern=WriteConcern('majority'))
        candidate = {'_id': 'minimir_development', 'application': 'minimir_first_loop', 'schema_version': 1,
                     'recovery_attempt': attempt, 'project_root': str(ROOT), 'purpose': PURPOSE, 'database': database}
        db.application_meta.insert_one(candidate)
        marker = candidate
        config = isolation.load(ROOT / 'conf/dev/server.json')
        config.update(database=database, node_id='runtime-' + attempt[:12])
        config.update({mode + '_address': f'127.0.0.1:{port}' for mode, port in
                       zip(['account', 'game', 'gm'], isolation.free_ports())})
        config_path = ROOT / 'conf/dev' / ('runtime-' + attempt + '.local.json')
        with config_path.open('x', encoding='utf-8', newline='\n') as output:
            output.write(json.dumps(config, ensure_ascii=False, indent=2) + '\n')
        config_sha = isolation.sha(config_path)
        isolation.save(directory / 'isolated-config.json', config)
        binary = directory / 'nbserver.exe'
        command = [sys.executable, str(ROOT / 'tools/dev'), 'go', 'build', '-race', '-o', str(binary), './cmd/nbserver']
        result = subprocess.run(command, cwd=ROOT, capture_output=True, timeout=180)
        (directory / 'build.stdout.log').write_bytes(result.stdout)
        (directory / 'build.stderr.log').write_bytes(result.stderr)
        if result.returncode:
            raise RuntimeError('独立race服务构建失败')
        report.update(binary_sha256=isolation.sha(binary), race_build=True)
        argv = lambda mode: [str(binary), '-config', str(config_path), '-mode', mode]
        account = isolation.launch(argv('account'), binary, directory / 'account.log', handles)
        game = isolation.launch(argv('game'), binary, directory / 'game-1.log', handles)
        gm = isolation.launch(argv('gm'), binary, directory / 'gm.log', handles)
        health = isolation.health_ready(config, [account, game, gm])
        report['runtime_rule_version'] = health['game']['rule_version']
        probe_handle = isolation.launch([prepared['node'], str(PROBE)], prepared['node'],
                                        directory / 'probe.stderr.log', handles, interactive=True)
        probe = RuntimeProbe(probe_handle)
        created = probe.action('initialize', account='http://' + config['account_address'],
                               game='ws://' + config['game_address'] + '/ws',
                               rule_version=health['game']['rule_version'])
        report['checks'].append(created)
        role_id = created['result']['role_id']
        # 仅首次附着前的等级门槛是隔离资格；受伤、学习、耗蓝、扣时全部走真实业务。
        before = db.roles.find_one({'_id': role_id})
        if before is None or before['level'] != 1 or before['session_epoch'] != 0:
            raise RuntimeError('角色不处于本轮未附着的新建前态')
        changed = db.roles.update_one({'_id': role_id, 'level': 1, 'session_epoch': 0}, {'$set': {'level': 7}})
        after = db.roles.find_one({'_id': role_id})
        if changed.modified_count != 1 or after != {**before, 'level': 7}:
            raise RuntimeError('隔离资格没有精确只改变等级')
        report['qualification'] = {'seeded': True, 'before_first_attachment': True,
                                   'changes': [{'field': 'level', 'before': 1, 'after': 7}],
                                   'other_role_fields_unchanged': True, 'natural_growth_verified': False}
        report['checks'].append(probe.action('prepare_join'))
        report['checks'].append(probe.action('healthy_reconnect'))
        report['checks'].append(probe.action('prepare_crash'))
        game, snapshot, check = crash_and_restart(db, game, config, argv, binary, directory, handles, '2', role_id)
        report['checks'].append({'online_crash': check})
        report['checks'].append(probe.action('cold_rejoin'))
        assert_same_ledger(snapshot, ledger(db, role_id), entered=True)
        report['checks'].append({'online_cold_permanent_and_original_receipts_preserved': True})
        report['checks'].append(probe.action('activate_cold'))
        assert_same_ledger(snapshot, ledger(db, role_id), entered=True)
        online = ledger(db, role_id)
        if online['summary']['audited_host_seconds'] != 0 or online['summary']['hosting_seconds'] != 21600:
            raise RuntimeError('在线受伤、施法或健康重连期间误扣托管时间')
        report['checks'].append({'online_combat_never_billed': True, 'ledger': online['summary']})
        report['checks'].append(probe.action('offline_start'))
        deadline = time.monotonic() + 15
        while time.monotonic() < deadline:
            hosted = ledger(db, role_id)
            if hosted['summary']['audited_host_seconds'] >= online['summary']['audited_host_seconds'] + 2:
                break
            time.sleep(.1)
        else:
            raise RuntimeError('真实离线托管未形成至少两秒已提交扣时')
        if hosted['summary']['report_used_seconds'] < 2:
            raise RuntimeError('离线战报未记录原真实扣时')
        report['checks'].append({'healthy_offline_billing': True, 'before': online['summary'], 'after': hosted['summary']})
        game, snapshot, check = crash_and_restart(db, game, config, argv, binary, directory, handles, '3', role_id)
        report['checks'].append({'offline_crash': check})
        report['checks'].append(probe.action('offline_rejoin'))
        assert_same_ledger(snapshot, ledger(db, role_id), entered=True)
        report['checks'].append({'offline_cold_permanent_and_original_receipts_preserved': True})
        report['checks'].append(probe.action('activate_cold'))
        assert_same_ledger(snapshot, ledger(db, role_id), entered=True)
        report['checks'].append(probe.action('finish'))
        probe_handle['child'].stdin.close()
        probe_handle['child'].wait(timeout=5)
        report.update(passed=True, injured_hp_reset_verified=True, healthy_injured_body_verified=True,
                      skill_cooldown_verified=True, managed_fault_billing_verified=True)
    except Exception as error:
        report['error'] = str(error)
        if probe is not None and hasattr(probe, 'failure'):
            report['failed_probe_action'] = probe.failure
    finally:
        cleanup(handles, client, database, marker, config_path, config_sha, directory, prepared, report)
        report['finished_at_utc'] = dt.datetime.now(dt.timezone.utc).isoformat()
        report['passed'] = report['passed'] and not report['cleanup_errors'] and all(report.get(key) is True for key in
                            ['own_processes_stopped', 'new_database_dropped', 'temporary_config_removed',
                             'main_preserved', 'mongo_preserved', 'source_stable', 'race_diagnostics_absent'])
        isolation.save(directory / 'report.json', report)
    print(json.dumps({'passed': report['passed'], 'report': str(directory / 'report.json'),
                      'error': report.get('error'), 'production_ready': False}, ensure_ascii=False))
    return 0 if report['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    action = parser.add_mutually_exclusive_group(required=True)
    action.add_argument('--check', action='store_true', help='只读核验依赖、专属Mongo和主预览身份')
    action.add_argument('--run', action='store_true', help='执行独立测试，结束清理本轮自有资源')
    args = parser.parse_args()
    try:
        if args.check:
            print(json.dumps(preflight(), ensure_ascii=False))
            return 0
        return run()
    except Exception as error:
        print(json.dumps({'passed': False, 'run_started': False, 'error': str(error)}, ensure_ascii=False))
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
