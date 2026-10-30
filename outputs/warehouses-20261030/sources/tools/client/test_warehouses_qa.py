"""仓储GUI隔离的纯安全边界；不连接Mongo、不启动进程/Creator/浏览器。"""
import copy
import datetime as dt
import json
from pathlib import Path
import tempfile
import types
import unittest
from unittest.mock import patch

from bson.int64 import Int64
import prepare_warehouses_qa as qa


def fresh(fixture):
    """明确手造的正常CreateRole形状，仅测试资格门禁。"""
    role_id, account_id, operation_id = 'a' * 32, 'b' * 32, 'c' * 32
    role = {'_id': role_id, 'account_id': account_id, 'origin_server_id': qa.load(qa.ROOT / 'conf/dev/server.json')['origin_id'],
            'slot': 1, 'job': 0, 'gender': 0, 'name': '纯资格样本', 'level': 1, 'experience': Int64(0),
            'revision': Int64(1), 'economic_command_seq': Int64(0), 'source_seq': Int64(0),
            'owner_instance': '', 'run_epoch': Int64(0), 'session_epoch': Int64(0),
            'capacity': fixture['inventory_capacity'], 'equipped': [], 'learned_skills': None,
            'privilege_until': dt.datetime(1, 1, 1), 'hosting_card_until': dt.datetime(1, 1, 1),
            'wallet': {'gold': 0, 'bound_gold': fixture['initial_bound_gold'], 'yuanbao_units': 0, 'bound_yuanbao_units': 0}, 'items': []}
    for n, item_id in enumerate(fixture['initial_inventory']):
        role['items'].append({'instance_id': f'{n + 1:032x}', 'item_id': item_id, 'quantity': 1, 'bound': True, 'version': 1})
    for book in fixture['initial_skill_books']:
        if book['job_id'] == 0:
            role['items'].append({'instance_id': 'd' * 32, 'item_id': book['item_id'], 'quantity': 1, 'bound': True,
                                  'version': 1, 'source_kind': 'initial_grant', 'source_id': operation_id})
    if fixture['binding']['enabled']:
        role['binding'] = copy.deepcopy(fixture['binding']['start_points'][0])
    op = {'_id': f'create:{account_id}:{operation_id}', 'kind': 'create_role', 'role_id': role_id}
    return role, op


class Collection:
    def __init__(self, db, kind): self.db, self.kind = db, kind
    def find_one(self, *args, **kwargs):
        if self.kind == 'role': return copy.deepcopy(self.db.role)
        if self.kind == 'marker': return copy.deepcopy(self.db.marker)
        if self.kind == 'proof': return copy.deepcopy(self.db.proof)
        if self.kind == 'account': return {'_id': self.db.role['account_id']}
    def find(self, *args, **kwargs): return types.SimpleNamespace(limit=lambda _: copy.deepcopy(self.db.operations_))
    def find_one_and_update(self, query, update, **kwargs):
        self.db.cas_queries.append(copy.deepcopy(query))
        if self.db.concurrent:
            self.db.role.update(self.db.concurrent)
        # 真实实现使用Mongo $$ROOT全聚合比较；这里明确只模拟这条围栏的匹配语义。
        if self.db.role != query['$expr']['$eq'][1]['$literal']:
            return None
        self.db.role.update(copy.deepcopy(update['$set']))
        return copy.deepcopy(self.db.role)
    def insert_one(self, proof, **kwargs):
        self.db.proof = copy.deepcopy(proof)


class SampleDB:
    def __init__(self, fixture, meta):
        self.role, op = fresh(fixture)
        self.operations_ = [op]
        self.marker = qa.marker_for(meta['database'], meta['run_id'])
        self.proof = None
        self.concurrent = None
        self.cas_queries = []
        self.roles = Collection(self, 'role')
        self.application_meta = Collection(self, 'marker')
        self.qa_warehouse_qualifications = Collection(self, 'proof')
        self.accounts = Collection(self, 'account')
        self.operations = Collection(self, 'operation')
        self.client = self
    def start_session(self): return self
    def __enter__(self): return self
    def __exit__(self, *args): pass
    def with_transaction(self, callback, **kwargs): return callback(self)


class WarehousesQATests(unittest.TestCase):
    def setUp(self):
        self.fixture = qa.fixture_contract(qa.load(qa.FIXTURE))
        self.run = 'e' * 32
        self.meta = {'run_id': self.run, 'database': qa.DB_PREFIX + self.run, 'fixture_sha256': qa.sha(qa.FIXTURE)}

    def test_only_exact_new_random_database_and_marker_are_eligible(self):
        self.assertEqual(len(self.meta['database'].encode()), 60)
        self.assertLessEqual(len(self.meta['database'].encode()), 63)
        marker = qa.marker_for(self.meta['database'], self.run)
        qa.verify_target(self.meta['database'], self.run, marker)
        for name, run, value in [('minimir_dev_game', self.run, marker), (self.meta['database'], 'F' * 32, marker),
                                 (self.meta['database'], self.run, {**marker, 'purpose': 'pet'}),
                                 (self.meta['database'], self.run, {**marker, 'project_root': 'D:/other'}),
                                 (self.meta['database'], self.run, {**marker, 'extra': True})]:
            with self.subTest(name=name, marker=value), self.assertRaises(ValueError): qa.verify_target(name, run, value)

    def test_output_path_cannot_target_main_config_or_other_stage(self):
        for path in ['.', '.work', 'conf/dev/server.json', 'client/build/web-mobile', '.work/pet-visual-20261029/runtime.json', '.work/warehouses-20261030/../foreign/runtime.json']:
            with self.subTest(path=path), self.assertRaises(ValueError): qa.work_path(path)
        self.assertEqual(qa.work_path('.work/warehouses-20261030/qa/run-' + self.run), qa.WORK / 'qa' / ('run-' + self.run))

    def test_fixture_never_changes_values_or_opens_summon_material(self):
        before = copy.deepcopy(self.fixture)
        qa.fixture_contract(self.fixture)
        self.assertEqual(self.fixture, before)
        for change in [lambda f: f.update(rule_version=22), lambda f: f['warehouses'].update(personal_expansion_yuanbao_units=0),
                       lambda f: f['casting_supply'].update(enabled=True), lambda f: f['skills'].append({'id': 17}),
                       lambda f: next(i for i in f['items'] if i['id'] == 900001).update(max_stack=4)]:
            value = copy.deepcopy(self.fixture)
            change(value)
            with self.assertRaises(ValueError): qa.fixture_contract(value)

    def test_normal_create_bson_int64_and_original_binding_are_accepted(self):
        role, op = fresh(self.fixture)
        qa.initial_role(role, self.fixture, op)
        self.assertNotIn('binding', role)
        future = copy.deepcopy(self.fixture)
        future['binding'].update(enabled=True, start_points=[{'map_id': 2, 'x': 633, 'y': 613}])
        role, op = fresh(future)
        qa.initial_role(role, future, op)
        role['binding']['x'] += 1
        with self.assertRaises(ValueError): qa.initial_role(role, future, op)

    def test_role_used_owned_changed_initial_items_or_source_are_rejected(self):
        changes = [{'revision': 2}, {'owner_instance': 'owner'}, {'session_epoch': 1}, {'economic_command_seq': 1}, {'source_seq': 1},
                   {'binding': {'map_id': 2, 'x': 1, 'y': 1}}, {'learned_skills': [{'skill_id': 3, 'level': 0}]}, {'level': 19}, {'revision': True}]
        for changes_ in changes:
            db = SampleDB(self.fixture, self.meta)
            db.role.update(changes_)
            with self.subTest(changes=changes_), self.assertRaises(ValueError): qa.qualification_transaction(db, db.role['_id'], self.fixture, self.meta, dt.datetime(2026, 10, 30))
            self.assertEqual(db.cas_queries, [])
        for change in [lambda r: r['items'][0].update(bound=False), lambda r: r['items'][0].update(quality=4),
                       lambda r: r['items'][-1].update(source_id='changed'), lambda r: r['wallet'].update(gold=1)]:
            db = SampleDB(self.fixture, self.meta)
            change(db.role)
            with self.assertRaises(ValueError): qa.qualification_transaction(db, db.role['_id'], self.fixture, self.meta, dt.datetime(2026, 10, 30))

    def test_qualification_preserves_all_original_fields_and_is_idempotent_before_enter(self):
        db = SampleDB(self.fixture, self.meta)
        before = copy.deepcopy(db.role)
        proof = qa.qualification_transaction(db, before['_id'], self.fixture, self.meta, dt.datetime(2026, 10, 30, microsecond=123456))
        after = proof['after']
        self.assertEqual(set(k for k in before if before[k] != after[k]), {'items', 'wallet', 'privilege_until'})
        self.assertEqual(after['items'][:len(before['items'])], before['items'])
        self.assertEqual([(i['item_id'], i['quantity'], i['bound'], i['source_kind']) for i in after['items'][len(before['items']):]], [(218, 1, False, 'development_fixture'), (900001, 5, False, 'development_fixture')])
        self.assertEqual(after['wallet']['yuanbao_units'], self.fixture['warehouses']['personal_expansion_yuanbao_units'] + 1000)
        self.assertEqual(after['wallet']['bound_gold'], before['wallet']['bound_gold'])
        self.assertEqual(after['revision'], 1)
        self.assertEqual(after['privilege_until'].microsecond, 123000)
        self.assertEqual(db.cas_queries[0]['$expr']['$eq'], ['$$ROOT', {'$literal': before}])
        replay = qa.qualification_transaction(db, before['_id'], self.fixture, self.meta, dt.datetime(2026, 11, 1))
        self.assertEqual(replay, proof)
        self.assertEqual(len(db.cas_queries), 1)
        db.role.update(owner_instance='entered', revision=2)
        with self.assertRaises(ValueError): qa.qualification_transaction(db, before['_id'], self.fixture, self.meta, dt.datetime(2026, 11, 1))

    def test_foreign_marker_extra_operation_and_concurrent_enter_fail_closed(self):
        db = SampleDB(self.fixture, self.meta)
        db.marker['purpose'] = 'main'
        with self.assertRaises(ValueError): qa.qualification_transaction(db, db.role['_id'], self.fixture, self.meta, dt.datetime(2026, 10, 30))
        self.assertEqual(db.cas_queries, [])
        db = SampleDB(self.fixture, self.meta)
        db.operations_.append({'kind': 'asset'})
        with self.assertRaises(ValueError): qa.qualification_transaction(db, db.role['_id'], self.fixture, self.meta, dt.datetime(2026, 10, 30))
        db = SampleDB(self.fixture, self.meta)
        db.concurrent = {'owner_instance': 'new-owner', 'revision': 2, 'session_epoch': 1}
        with self.assertRaises(RuntimeError): qa.qualification_transaction(db, db.role['_id'], self.fixture, self.meta, dt.datetime(2026, 10, 30))
        self.assertEqual(db.role['wallet']['gold'], 0)
        self.assertEqual(len(db.role['items']), len(self.fixture['initial_inventory']) + 1)
        self.assertIsNone(db.proof)

    def test_forged_or_reused_pid_is_never_stopped(self):
        handle = {'binary': Path('qa.exe'), 'meta': {'pid': 123, 'start_ticks': 456, 'argv': ['qa.exe']}, 'argv': ['qa.exe']}
        with patch.object(qa, 'windows_owned', return_value=False), patch.object(qa, 'windows_identity', return_value={'pid': 123, 'start_ticks': 999}), patch.object(qa, 'windows_stop', side_effect=AssertionError('不能停止伪身份')):
            with self.assertRaises(RuntimeError): qa.stop_owned(handle)
        with patch.object(qa, 'windows_owned', return_value=True), patch.object(qa.database, 'command_line', return_value='other.exe'):
            self.assertFalse(qa.process_matches(handle['meta'], Path('qa.exe'), ['qa.exe']))
        self.assertTrue(qa.stop_owned({}))

    def test_qualification_rejects_forged_process_before_database_access(self):
        with tempfile.TemporaryDirectory(dir=qa.WORK) as temporary:
            directory = Path(temporary) / ('run-' + self.run)
            directory.mkdir()
            runtime = directory / 'runtime.json'
            qa.save(runtime, {**self.meta, 'project_root': str(qa.ROOT), 'purpose': qa.PURPOSE, 'ports': qa.PORTS,
                              'static_port': qa.STATIC_PORT, 'argv': [], 'pid': 123, 'start_ticks': 456})
            with patch.object(qa, 'windows_owned', return_value=False), patch.object(qa, 'mongo_identity', side_effect=AssertionError('伪进程不能触数据库')), patch.object(qa, 'qa_health', side_effect=AssertionError('伪进程不能回退健康身份')):
                with self.assertRaises(RuntimeError): qa.validate_runtime(runtime)
        with patch.object(qa, 'validate_runtime', side_effect=AssertionError('畸形ID不能查运行身份')):
            for value in ['a', 'A' * 32, '../main', None]:
                with self.assertRaises(ValueError): qa.qualify('unused', value)

    def test_busy_static_port_does_not_start_or_stop_any_process(self):
        class Busy:
            def __enter__(self): return self
            def __exit__(self, *_): pass
            def settimeout(self, *_): pass
            def connect_ex(self, address): return 0 if address[1] == 18082 else 1
            def setsockopt(self, *_): pass
            def bind(self, *_): pass
        with patch.object(qa.socket, 'socket', return_value=Busy()), patch.object(qa.subprocess, 'Popen', side_effect=AssertionError('不能启动')), patch.object(qa, 'windows_stop', side_effect=AssertionError('不能接管')):
            with self.assertRaises(RuntimeError): qa.ensure_ports_free()
        self.assertNotIn(18081, [*qa.PORTS.values(), qa.STATIC_PORT])

    def test_exited_supervisor_does_not_prove_qa_ports_are_closed(self):
        child = types.SimpleNamespace(poll=lambda: 0)
        handle = {'child': child, 'binary': Path('qa.exe'), 'meta': None, 'argv': []}
        class Listening:
            def __enter__(self): return self
            def __exit__(self, *_): pass
            def settimeout(self, *_): pass
            def connect_ex(self, address): return 0 if address[1] == 18111 else 1
        with patch.object(qa.socket, 'socket', return_value=Listening()):
            stopped = qa.stop_owned(handle) and qa.ports_closed(qa.PORTS.values())
        self.assertFalse(stopped)

    def artifact(self, mutate=None):
        with tempfile.TemporaryDirectory(dir=qa.WORK) as temporary:
            folder = Path(temporary)
            source = folder / 'source.ts'
            source.write_text('saved source', encoding='utf-8')
            files = [source, qa.FIXTURE, qa.PUBLIC, qa.MANIFEST]
            candidate = folder / 'candidate/web-mobile'
            candidate.mkdir(parents=True)
            (candidate / 'index.html').write_text('real-artifact-test-placeholder', encoding='utf-8')
            manifest = folder / 'candidate-files.json'
            qa.save(manifest, qa.tree(candidate))
            external = {'external_engine': {'locked-engine': 'a' * 64}, 'external_font': {'locked-font': 'b' * 64}}
            qa.save(folder / 'report.json', {'creator': '3.8.8', 'platform': 'web-mobile', 'exit_code': 36, 'success': True,
                'scene_prefab_layout_preserved': True, 'source_stable_during_build': True, 'served_build_preserved': True,
                'script_hashes': qa.snapshot(p for p in (qa.ROOT / 'client/assets/scripts').rglob('*') if p.is_file() and p.suffix in {'.ts', '.js'}),
                'asset_hashes': qa.snapshot(p for p in (qa.ROOT / 'client/assets').rglob('*') if p.is_file() and p.suffix in {'.scene', '.prefab'}),
                'build_root': str(candidate), 'artifact_manifest': str(manifest), 'artifact_manifest_sha256': qa.sha(manifest), 'artifact_file_count': 1})
            wrapper = {'group': 'creator', 'passed': True, 'stable': True, 'before': qa.snapshot(files), 'after': qa.snapshot(files), 'commands': [{'exit_code': 0}]}
            wrapper.update({key + suffix: value for key, value in external.items() for suffix in ['_before', '_after']})
            qa.save(folder / 'wrapper.json', wrapper)
            if mutate: mutate(folder, source, candidate)
            with patch.object(qa, 'client_inputs', return_value=files), patch.object(qa, 'external_sources', return_value=external), patch.object(qa.subprocess, 'Popen', side_effect=AssertionError('静态验证不构建/启动')):
                return qa.verify_artifact(folder / 'report.json')

    def test_candidate_requires_exact_current_source_and_complete_built_output(self):
        proof = self.artifact()
        self.assertEqual(proof['public_sha256'], qa.sha(qa.PUBLIC))
        with self.assertRaises(ValueError): self.artifact(lambda folder, source, build: source.write_text('late source', encoding='utf-8'))
        with self.assertRaises(ValueError): self.artifact(lambda folder, source, build: (build / 'late.js').write_text('late output', encoding='utf-8'))
        def wrong_report(folder, *_):
            report = qa.load(folder / 'report.json')
            report['success'] = False
            qa.save(folder / 'report.json', report)
        with self.assertRaises(ValueError): self.artifact(wrong_report)
        def changed_engine(folder, *_):
            wrapper = qa.load(folder / 'wrapper.json')
            wrapper['external_engine_after']['locked-engine'] = 'c' * 64
            qa.save(folder / 'wrapper.json', wrapper)
        with self.assertRaises(ValueError): self.artifact(changed_engine)
        def missing_fonts(folder, *_):
            wrapper = qa.load(folder / 'wrapper.json')
            del wrapper['external_font_before']
            qa.save(folder / 'wrapper.json', wrapper)
        with self.assertRaises(ValueError): self.artifact(missing_fonts)

    def test_qualification_rechecks_original_go_and_creator_before_write(self):
        with tempfile.TemporaryDirectory(dir=qa.WORK) as temporary:
            path = Path(temporary) / 'go-report.json'
            path.write_text('{}', encoding='utf-8')
            meta = {'go_report': str(path), 'go_report_sha256': qa.sha(path), 'go_source_sha256': {'source.go': 'a' * 64},
                    'creator_report': 'original-report', 'artifact_manifest_sha256': 'b' * 64}
            with patch.object(qa, 'verify_go', return_value=meta['go_source_sha256']), patch.object(qa, 'verify_artifact', return_value={'artifact_manifest_sha256': 'b' * 64}):
                qa.verify_frozen_runtime_sources(meta)
            with patch.object(qa, 'verify_go', return_value={'source.go': 'changed'}), patch.object(qa, 'verify_artifact', side_effect=AssertionError('Go漂移先拒')):
                with self.assertRaises(RuntimeError): qa.verify_frozen_runtime_sources(meta)
            with patch.object(qa, 'verify_go', return_value=meta['go_source_sha256']), patch.object(qa, 'verify_artifact', return_value={'artifact_manifest_sha256': 'changed'}):
                with self.assertRaises(RuntimeError): qa.verify_frozen_runtime_sources(meta)

    def test_go_gate_requires_full_current_real_mongo_race_evidence(self):
        with tempfile.TemporaryDirectory(dir=qa.WORK) as temporary:
            directory = Path(temporary)
            source = directory / 'source.go'
            source.write_text('explicit synthetic input', encoding='utf-8')
            report = {'passed': True, 'exit_code': 0, 'mongo_e2e_explicit': True, 'race_detector': True,
                      'source_changed_during_test': False, 'skipped_tests': [], 'failed_tests': [],
                      'package_timeout_seconds': 1200, 'package_parallelism': 1, 'single_rpc_and_business_deadlines_changed': False,
                      'command': [qa.sys.executable, str(qa.ROOT / 'tools/dev'), 'go', 'test', '-json', '-race', '-p', '1', './...', '-count=1', '-timeout=1200s'],
                      'source_sha256': qa.snapshot([source]), 'source_sha256_after': qa.snapshot([source])}
            path = directory / 'report.json'
            qa.save(path, report)
            with patch.object(qa.runpy, 'run_path', return_value={'input_files': lambda: [source]}):
                self.assertEqual(qa.verify_go(path), qa.snapshot([source]))
                for key, value in [('passed', False), ('race_detector', False), ('mongo_e2e_explicit', False), ('skipped_tests', ['not-run']), ('failed_tests', ['failed']),
                                   ('command', [s.replace('./...', './internal/integration') for s in report['command']]),
                                   ('command', report['command'] + ['-run', 'TestOne']), ('command', report['command'] + ['-skip=TestOne'])]:
                    qa.save(path, {**report, key: value})
                    with self.assertRaises(ValueError): qa.verify_go(path)
                qa.save(path, report)
                source.write_text('changed after test', encoding='utf-8')
                with self.assertRaises(ValueError): qa.verify_go(path)


if __name__ == '__main__':
    unittest.main()
