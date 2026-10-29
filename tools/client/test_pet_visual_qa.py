"""新生GUI准备边界：独立public、精确新库与从未入场资格；不连接真实节点。"""
import copy
import json
import shutil
import tempfile
import types
import unittest
from pathlib import Path
from unittest.mock import patch
import prepare_pet_visual_qa as helper
import isolated_creator as creator

ROOT = helper.ROOT


class PetVisualPreparationTests(unittest.TestCase):
    def setUp(self):
        self.source = json.loads((ROOT / 'config/development/first-loop.json').read_text(encoding='utf-8'))
        self.source['rule_version'] = 18
        self.catalog = json.loads((ROOT / 'config/development/summon-candidates.json').read_text(encoding='utf-8'))

    def test_local_public_comes_from_full_fixture_without_mutating_main(self):
        saved = copy.deepcopy(self.source)
        fixture = helper.build_fixture(self.source, self.catalog)
        public = creator.public_gameplay(fixture)
        self.assertEqual(self.source, saved)
        self.assertEqual(len(fixture['skills']), 23)
        self.assertEqual([s['id'] for s in public['skills'] if s['id'] == 17], [17])
        self.assertEqual(next(i['max_stack'] for i in fixture['items'] if i['id'] == 187), 2)
        self.assertEqual(fixture['pet_runtime']['bootstrap_mode'], 'none')
        self.assertTrue(public['casting_supply']['enabled'])
        self.assertEqual(next(b['item_id'] for b in fixture['initial_skill_books'] if b['job_id'] == 2), 25)
        self.assertEqual(fixture['skill_shop'], self.source['skill_shop'])
        self.assertFalse(any(r['item_id'] in (25, 187) for r in fixture['skill_shop']))
        self.assertEqual(fixture['map'], self.source['map'])
        self.assertEqual(fixture['pet_runtime']['definitions'], self.source['pet_runtime']['definitions'])

    def test_bad_main_or_incomplete_catalog_never_start(self):
        for mutate in [lambda s: s.update(rule_version=17), lambda s: s['casting_supply'].update(enabled=True), lambda s: s['pet_runtime'].update(bootstrap_mode='none')]:
            value = copy.deepcopy(self.source)
            mutate(value)
            with self.assertRaises(ValueError):
                helper.build_fixture(value, self.catalog)
        for key, value in [('runtime_enabled', True), ('required_rule_version', 18), ('upgrades', []), ('books', [])]:
            catalog = copy.deepcopy(self.catalog)
            catalog[key] = value
            with self.assertRaises(ValueError):
                helper.build_fixture(self.source, catalog)

    def test_exact_random_marker_only_and_project_path_guard(self):
        run = 'a' * 32
        name = helper.DB_PREFIX + run
        marker = helper.marker_for(name, run)
        self.assertLessEqual(len(name.encode('utf-8')), 63)
        helper.verify_target(name, run, marker)
        for field, value in [('purpose', 'foreign'), ('project_root', 'foreign'), ('qa_run_id', 'b' * 32), ('application', 'foreign')]:
            with self.assertRaises(ValueError):
                helper.verify_target(name, run, {**marker, field: value})
        with self.assertRaises(ValueError):
            helper.verify_target('minimir_dev_game', run, marker)
        for path in [ROOT, ROOT / '.work', ROOT.parent / 'foreign']:
            with self.assertRaises(ValueError):
                creator.work_path(path)

    def test_invalid_role_id_rejected_before_reading_database(self):
        with tempfile.TemporaryDirectory(dir=ROOT / '.work') as temporary:
            for role_id in ['', '1', 'g' * 32, '0' * 33]:
                with self.assertRaises(ValueError), patch.object(helper.common.database, 'status', side_effect=AssertionError('不能触及DB')):
                    helper.qualify(Path(temporary) / 'runtime.json', role_id)

    def test_creator_refuses_reuse_and_changed_input_before_launch(self):
        fixture = helper.build_fixture(self.source, self.catalog)
        with tempfile.TemporaryDirectory(dir=ROOT / '.work') as temporary:
            directory = Path(temporary)
            (directory / 'client-project').mkdir()
            with self.assertRaises(ValueError):
                creator.prepare_project(directory, fixture)
            with self.assertRaises(RuntimeError), patch.object(creator.subprocess, 'run', side_effect=AssertionError('不能启动Creator')):
                creator.build_project(directory, {'project': str(directory / 'client-project'), 'project_sha256': {'missing': 'sha'}})

    def qualify_sample(self, owned=False, changed=False):
        import pymongo
        run = 'a' * 32
        name = helper.DB_PREFIX + run
        role_id = 'b' * 32
        role = {'_id': role_id, 'job': 2, 'level': 1, 'revision': 1, 'economic_command_seq': 0,
                'source_seq': 0, 'owner_instance': 'active-owner' if owned else '', 'run_epoch': 0,
                'session_epoch': 0, 'learned_skills': [], 'equipped': [], 'items': [
                    {'instance_id': 'c' * 32, 'item_id': 25, 'quantity': 1, 'version': 1, 'source_kind': 'initial_grant'}]}
        captured = []
        with tempfile.TemporaryDirectory(dir=ROOT / '.work') as temporary:
            directory = Path(temporary)
            cfg = json.loads((ROOT / 'conf/dev/server.json').read_text(encoding='utf-8'))
            fixture_path = directory / 'config/development/first-loop.json'
            config_path = directory / 'conf/dev/server.json'
            helper.save(fixture_path, helper.build_fixture(self.source, self.catalog))
            cfg.update(database=name, fixture_file=str(fixture_path))
            helper.save(config_path, cfg)
            binary = directory / 'nbserver.exe'
            binary.write_bytes(b'owned fixture binary')
            meta = {'project_root': str(ROOT), 'database': name, 'run_id': run, 'purpose': helper.PURPOSE,
                    'argv': [str(binary), '-config', str(config_path)], 'binary_sha256': helper.sha(binary),
                    'config_sha256': helper.sha(config_path), 'fixture_sha256': helper.sha(fixture_path),
                    'health_instances': {mode: 'qa-' + mode for mode in helper.PORTS}}
            runtime_path = directory / 'runtime.json'
            helper.save(runtime_path, meta)
            class Roles:
                def find_one(self, query): return copy.deepcopy(role)
                def find_one_and_update(self, query, update, **kwargs):
                    captured.append(copy.deepcopy(query))
                    self_outer.assertEqual(query['owner_instance'], '')
                    self_outer.assertEqual(query['run_epoch'], 0)
                    self_outer.assertEqual(query['session_epoch'], 0)
                    self_outer.assertEqual(query['items'], role['items'])
                    if changed: return None
                    result = copy.deepcopy(role)
                    result.update(update['$set'])
                    result['items'].append(update['$push']['items'])
                    return result
            self_outer = self
            db = types.SimpleNamespace(roles=Roles(), application_meta=types.SimpleNamespace(find_one=lambda _: helper.marker_for(name, run)))
            class Client:
                def __enter__(self): return self
                def __exit__(self, *args): pass
                def __getitem__(self, target):
                    self_outer.assertEqual(target, name)
                    return db
            with patch.object(helper.common, 'windows_owned', return_value=True), patch.object(helper, 'health', return_value={'ok': True}), patch.object(helper.common.database, 'status', return_value={'running': True, 'endpoint_verified': True, 'uri': cfg['mongo_uri_by_platform']['windows']}), patch.object(pymongo, 'MongoClient', return_value=Client()):
                result = helper.qualify(runtime_path, role_id)
            return result, captured

    def test_qualification_cas_contains_empty_owner_and_original_items(self):
        result, captured = self.qualify_sample()
        self.assertEqual(len(captured), 1)
        self.assertEqual(result['explicit_count'], 2)
        self.assertTrue(result['only_before_owner'])
        self.assertFalse(result['skills_seeded'])
        self.assertFalse(result['body_seeded'])

    def test_qualification_never_overwrites_owner_or_changed_aggregate(self):
        with self.assertRaises(ValueError): self.qualify_sample(owned=True)
        with self.assertRaises(RuntimeError): self.qualify_sample(changed=True)

    def test_unknown_static_process_is_preserved_and_not_reported_stopped(self):
        child = types.SimpleNamespace(poll=lambda: None, wait=lambda **_: None)
        handle = {'child': child, 'meta': None, 'node': Path('node.exe')}
        with patch.object(helper.common, 'windows_stop', side_effect=AssertionError('不得停止未知进程')):
            with self.assertRaises(RuntimeError): helper.stop_static(handle)
        self.assertTrue(helper.stop_static({}))
        child.poll = lambda: 0
        self.assertTrue(helper.stop_static(handle))

    def test_build_detects_new_source_file_as_changed_input(self):
        with tempfile.TemporaryDirectory(dir=ROOT / '.work') as temporary:
            directory = Path(temporary)
            project = directory / 'client-project'
            for name in ['assets', 'settings', 'profiles', '.creator', 'node_modules']:
                (project / name).mkdir(parents=True)
            for name in ['package.json', 'package-lock.json', 'tsconfig.json']:
                (project / name).write_text('{}', encoding='utf-8')
            app = directory / 'Creator.exe'
            app.write_bytes(b'fake creator')
            prepared = {'project': str(project), 'project_sha256': creator.files(project), 'public_sha256': 'fixture', 'rule_version': 18}
            def mutate(*_, **__):
                (project / 'assets/late.ts').write_text('late source', encoding='utf-8')
                (project / 'build/web-mobile').mkdir(parents=True)
                (project / 'build/web-mobile/index.html').write_text('built', encoding='utf-8')
                return types.SimpleNamespace(returncode=36)
            with patch.dict(creator.os.environ, {'MINIMIR_CREATOR_EXECUTABLE': str(app)}), patch.object(creator.subprocess, 'run', side_effect=mutate):
                with self.assertRaises(RuntimeError): creator.build_project(directory, prepared)
            report = json.loads((directory / 'creator-report.json').read_text(encoding='utf-8'))
            self.assertFalse(report['passed'])
            self.assertFalse(report['source_stable'])
            self.assertIn('assets/late.ts', report['input_sha256'])

    def reuse_sample(self, mutate=None):
        """假构建只检验复用围栏，不执行Creator，也不冒称真实构建通过。"""
        with tempfile.TemporaryDirectory(dir=ROOT / '.work') as temporary:
            sandbox = Path(temporary)
            source = sandbox / 'client'
            artifact = sandbox / '.work/original'
            project = artifact / 'client-project'
            target = sandbox / '.work/next'
            target.mkdir(parents=True)
            for name in ['assets', 'settings', 'profiles', '.creator', 'node_modules']:
                (source / name).mkdir(parents=True)
            for name in ['package.json', 'package-lock.json', 'tsconfig.json']:
                (source / name).write_text('{}', encoding='utf-8')
            (source / 'assets/body.ts').write_text('original source', encoding='utf-8')
            fixture = helper.build_fixture(self.source, self.catalog)
            creator.save(source / 'assets/resources/development/gameplay.json', creator.public_gameplay(self.source))
            shutil.copytree(source, project)
            creator.save(project / 'assets/resources/development/gameplay.json', creator.public_gameplay(fixture))
            creator.save(artifact / 'project-copy.json', {'project': str(project), 'source_sha256': creator.input_snapshot(source),
                'project_sha256': creator.input_snapshot(project), 'public_sha256': creator.sha(project / 'assets/resources/development/gameplay.json'),
                'only_public_changed': True, 'runtime_resource_override': False, 'excluded_generated_editor_caches': creator.EDITOR_CACHES})
            (project / 'build/web-mobile').mkdir(parents=True)
            (project / 'build/web-mobile/index.html').write_text('test artifact', encoding='utf-8')
            creator.save(artifact / 'creator-report.json', {'creator': '3.8.8', 'platform': 'web-mobile', 'project': str(project),
                'passed': True, 'source_stable': True, 'exit_code': 36, 'rule_version': 18,
                'input_sha256': creator.input_snapshot(project), 'build_sha256': creator.files(project / 'build/web-mobile'),
                'public_sha256': creator.sha(project / 'assets/resources/development/gameplay.json'), 'excluded_generated_editor_caches': creator.EDITOR_CACHES})
            if mutate:
                mutate(source, project, fixture)
            with patch.object(creator, 'ROOT', sandbox), patch.object(creator.subprocess, 'run', side_effect=AssertionError('复用不能启动Creator')):
                copied, built, proof = creator.reuse_project(target, artifact, fixture)
            self.assertEqual((target / 'creator-report.json').read_bytes(), (artifact / 'creator-report.json').read_bytes())
            self.assertEqual((target / 'project-copy.json').read_bytes(), (artifact / 'project-copy.json').read_bytes())
            return proof

    def test_unchanged_real_artifact_contract_reuses_without_rebuilding(self):
        proof = self.reuse_sample()
        for name in ['passed', 'current_source_matches', 'isolated_inputs_match', 'build_outputs_match']:
            self.assertTrue(proof[name])
        self.assertFalse(proof['creator_rerun'])
        self.assertFalse(proof['runtime_resource_override'])

    def test_changed_current_source_or_candidate_public_rejects_reuse(self):
        with self.assertRaises(RuntimeError):
            self.reuse_sample(lambda source, project, fixture: (source / 'assets/body.ts').write_text('changed', encoding='utf-8'))
        with self.assertRaises(RuntimeError):
            self.reuse_sample(lambda source, project, fixture: fixture['casting_supply'].update(enabled=False))

    def test_changed_isolated_input_or_build_output_rejects_reuse(self):
        with self.assertRaises(RuntimeError):
            self.reuse_sample(lambda source, project, fixture: (project / 'assets/body.ts').write_text('changed', encoding='utf-8'))
        with self.assertRaises(RuntimeError):
            self.reuse_sample(lambda source, project, fixture: (project / 'build/web-mobile/new.js').write_text('late output', encoding='utf-8'))

    def test_busy_static_port_fails_before_starting_or_stopping_any_process(self):
        probe = types.SimpleNamespace(__enter__=lambda _: None)
        class BusySocket:
            def __enter__(self): return self
            def __exit__(self, *_): pass
            def settimeout(self, *_): pass
            def connect_ex(self, address):
                self_outer.assertEqual(address, ('127.0.0.1', 18082))
                return 0
        self_outer = self
        with patch.object(helper.socket, 'socket', return_value=BusySocket()), patch.object(helper.subprocess, 'Popen', side_effect=AssertionError('端口忙时不能启动')), patch.object(helper.common, 'windows_stop', side_effect=AssertionError('不能停止已有进程')):
            with self.assertRaises(RuntimeError): helper.launch_static(ROOT / '.work', ROOT / '.work/client-project', 'a' * 32, {})

    def test_only_exact_generated_editor_questionnaire_cache_is_not_runtime_input(self):
        with tempfile.TemporaryDirectory(dir=ROOT / '.work') as temporary:
            project = Path(temporary)
            for name in ['assets', 'settings', 'profiles', '.creator', 'node_modules']:
                (project / name).mkdir()
            for name in ['package.json', 'package-lock.json', 'tsconfig.json']:
                (project / name).write_text('{}', encoding='utf-8')
            cache = project / creator.EDITOR_CACHES[0]
            cache.parent.mkdir(parents=True)
            cache.write_text('{"editor_survey":"old"}', encoding='utf-8')
            runtime = cache.parent / 'engine.json'
            runtime.write_text('{"splash_screen":{"total_time":0}}', encoding='utf-8')
            before = creator.input_snapshot(project)
            cache.write_text('{"editor_survey":"refreshed"}', encoding='utf-8')
            self.assertEqual(creator.input_snapshot(project), before)
            self.assertNotIn(creator.EDITOR_CACHES[0], before)
            runtime.write_text('{"splash_screen":{"total_time":1}}', encoding='utf-8')
            self.assertNotEqual(creator.input_snapshot(project), before)
            self.assertEqual(creator.EDITOR_CACHES, ['settings/v2/packages/information.json'])


if __name__ == '__main__':
    unittest.main()
