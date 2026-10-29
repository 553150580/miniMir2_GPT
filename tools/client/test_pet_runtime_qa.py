"""宠物GUI工具静态/所有权反例；全部节点、DB和浏览器均替身，不启动真实进程。"""
import copy
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import types
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('pet_gui_helper', ROOT / 'tools/client/prepare_pet_runtime_qa.py')
helper = importlib.util.module_from_spec(spec)
spec.loader.exec_module(helper)


class PetGUIStaticTests(unittest.TestCase):
    def test_fixture_copy_changes_only_explicit_isolated_qa_values(self):
        original = json.loads((ROOT / 'config/development/first-loop.json').read_text(encoding='utf-8'))
        saved = copy.deepcopy(original)
        for version in (16, 17):
            probe = copy.deepcopy(original)
            probe['rule_version'] = version  # 纯门禁样本；不会写盘或把主17配置改成16。
            self.assertEqual(helper.build_fixture(probe, 15000)['rule_version'], version)
        value = helper.build_fixture(original, 15000)
        self.assertEqual(value['rule_version'], original['rule_version'])
        self.assertEqual(original, saved)
        self.assertEqual(value['pet']['hp'], 1)
        self.assertEqual(value['pet']['attack_max'], 0)
        self.assertEqual(value['map']['pet_spawn'], {'x': 8, 'y': 6})
        self.assertEqual(value['map']['monster_spawns'][0], {'x': 8, 'y': 5})
        self.assertFalse(value['casting_supply']['enabled'])
        self.assertFalse(any(s['id'] == 17 for s in value['skills']))
        for row in value['pet_runtime']['definitions']:
            if row['base']['definition_id'] == value['pet']['definition_id']:
                self.assertEqual(row['base']['hp'], 1)
                self.assertEqual(row['base']['attack_max'], 0)
                row['base'].update({k: original['pet'][k] for k in ['hp', 'attack_min', 'attack_max']})
                row['lifecycle']['corpse_ttl_ms'] = 180000
        value['pet'] = original['pet']
        value['map']['pet_spawn'] = original['map']['pet_spawn']
        value['map']['monster_spawns'][0] = original['map']['monster_spawns'][0]
        value['monsters'] = original['monsters']
        self.assertEqual(value, original)

    def test_unsupported_ttl_or_open_skill_rejected(self):
        original = json.loads((ROOT / 'config/development/first-loop.json').read_text(encoding='utf-8'))
        for ttl in [0, 11999, 15001, 180000]:
            with self.assertRaises(ValueError):
                helper.build_fixture(original, ttl)
        opened = copy.deepcopy(original)
        opened['skills'].append({'id': 17})
        with self.assertRaises(ValueError):
            helper.build_fixture(opened, 15000)
        for change in [lambda value: value.update(rule_version=15), lambda value: value.update(rule_version=18), lambda value: value['casting_supply'].update(enabled=True), lambda value: value['pet_runtime'].update(bootstrap_mode='none')]:
            probe = copy.deepcopy(original)
            change(probe)
            with self.assertRaises(ValueError):
                helper.build_fixture(probe, 15000)
        for version in (16, 17):
            def reply(url, timeout):
                mode = next(mode for mode, port in helper.PORTS.items() if ':' + str(port) + '/' in url)
                return io.StringIO(json.dumps({'status': 'ok', 'mode': mode, 'fixture_hash': 'owned', 'rule_version': version, 'instance': 'this-' + mode}))
            with patch.object(helper.urllib.request, 'urlopen', side_effect=reply):
                self.assertEqual(len(helper.health('owned', version)), 3)
                self.assertIsNone(helper.health('owned', 33 - version), '同hash旧规则也不能当当前节点')
                self.assertIsNone(helper.health('foreign', version))

    def test_drop_requires_exact_random_name_root_purpose_and_marker(self):
        run = 'a' * 32
        name = helper.DB_PREFIX + run
        marker = {'_id': 'minimir_development', 'application': 'minimir_first_loop', 'schema_version': 1, 'qa_run_id': run, 'database': name, 'project_root': str(ROOT), 'purpose': helper.PURPOSE}
        helper.verify_drop_target(name, run, marker)
        for altered in [None, {**marker, 'project_root': 'foreign'}, {**marker, 'purpose': 'foreign'}, {**marker, '_id': 'b' * 32}]:
            with self.assertRaises(ValueError):
                helper.verify_drop_target(name, run, altered)
        with self.assertRaises(ValueError):
            helper.verify_drop_target('minimir_dev_game', run, marker)
        with self.assertRaises(ValueError):
            helper.workspace_path('../foreign')

    def controlled_run(self, wrong_marker=False, lost_identity=False, write_unknown=False, write_missing=False):
        import pymongo
        events = []
        with tempfile.TemporaryDirectory(dir=ROOT / '.work') as temp:
            folder = Path(temp)
            class Child:
                pid = 991
                code = None
                def poll(self): return self.code
                def wait(self, timeout): return self.code
            child = Child()
            state = {'marker': None, 'name': None, 'browser_done': False}
            class Collection:
                def insert_one(self, marker):
                    events.append('insert_marker')
                    if write_missing:
                        raise TimeoutError('marker创建响应未知')
                    assert marker['_id'] == 'minimir_development' and marker['application'] == 'minimir_first_loop' and marker['qa_run_id']
                    state.update(marker=marker.copy(), name=marker['database'])
                    if write_unknown:
                        raise TimeoutError('marker创建响应未知')
                def find_one(self, key):
                    return {**state['marker'], 'purpose': 'foreign'} if wrong_marker else state['marker']
            class Client:
                def __getitem__(self, name): return self if name.startswith(helper.DB_PREFIX) else Collection()
                def list_database_names(self): return [state['name']] if state['marker'] else []
                def drop_database(self, name):
                    events.append('drop')
                    self.assert_target = name
                    state['marker'] = None
                def close(self): events.append('close_db')
            client = Client()
            def dev(args, log=None, timeout=60):
                if args[0] == 'go':
                    Path(args[3]).write_bytes(b'fake-owned-binary')
                    return None
                if args[-1] == 'start':
                    events.append('restore_main')
                    return {'running': True, 'health': {key: {'status': 'ok'} for key in helper.PORTS}}
                return {'running': False}
            def launch(*args, **kwargs):
                events.append('launch')
                return child
            def browser(cmd, **kwargs):
                events.append('browser')
                state['browser_done'] = True
                out = Path(cmd[cmd.index('--report-dir') + 1])
                helper.save(out / 'report.json', {'passed': True, 'checks': [1], 'errors': [], 'warnings': []})
                return types.SimpleNamespace(returncode=0, stdout=b'', stderr=b'')
            def stop(meta, executable):
                events.append('stop_qa')
                child.code = 0
            with patch.object(helper, 'protected_inputs', return_value={'guard': 'same'}), patch.object(helper.database, 'status', return_value={'running': True, 'endpoint_verified': True, 'uri': 'mongodb://127.0.0.1:27119/?replicaSet=minimirWinDev&directConnection=true'}), patch.object(helper, 'dev', side_effect=dev), patch.object(helper, 'assert_main_identity'), patch.object(helper, 'ports_free', side_effect=lambda: events.append('ports_free')), patch.object(pymongo, 'MongoClient', return_value=client), patch.object(helper.subprocess, 'Popen', side_effect=launch), patch.object(helper.subprocess, 'run', side_effect=browser), patch.object(helper, 'windows_identity', return_value={'pid': 991, 'start_ticks': 123}), patch.object(helper, 'windows_owned', side_effect=lambda *_: not (lost_identity and state['browser_done'])), patch.object(helper, 'windows_stop', side_effect=stop), patch.object(helper, 'health', return_value={mode: {'instance': 'qa-' + mode} for mode in helper.PORTS}), patch('sys.stdout', new_callable=io.StringIO):
                code = helper.run(types.SimpleNamespace(report_dir=str(folder), ttl_ms=15000))
            report = json.loads(next(folder.glob('run-*/report.json')).read_text(encoding='utf-8'))
            return code, report, events

    @unittest.skipUnless(helper.os.name == 'nt', '仅Windows所有权控制流')
    def test_real_helper_control_flow_stops_then_drops_only_marker_db_then_restores_main(self):
        code, report, events = self.controlled_run()
        self.assertEqual(code, 0)
        self.assertTrue(report['passed'])
        self.assertLess(events.index('stop_qa'), events.index('drop'))
        self.assertLess(events.index('drop'), events.index('restore_main'))

    @unittest.skipUnless(helper.os.name == 'nt', '仅Windows所有权控制流')
    def test_foreign_database_marker_is_preserved_but_owned_nodes_still_stop_and_main_restores(self):
        code, report, events = self.controlled_run(wrong_marker=True)
        self.assertEqual(code, 1)
        self.assertFalse(report['qa_database_dropped'])
        self.assertNotIn('drop', events)
        self.assertIn('stop_qa', events)
        self.assertIn('restore_main', events)

    @unittest.skipUnless(helper.os.name == 'nt', '仅Windows所有权控制流')
    def test_lost_process_identity_never_stops_drops_or_replaces_foreign_listener(self):
        code, report, events = self.controlled_run(lost_identity=True)
        self.assertEqual(code, 1)
        self.assertFalse(report['qa_stopped'])
        self.assertNotIn('stop_qa', events)
        self.assertNotIn('drop', events)
        self.assertNotIn('restore_main', events)

    @unittest.skipUnless(helper.os.name == 'nt', '仅Windows所有权控制流')
    def test_unknown_marker_write_is_cleaned_only_after_exact_ownership_readback(self):
        code, report, events = self.controlled_run(write_unknown=True)
        self.assertEqual(code, 1)
        self.assertTrue(report['database_creation_attempted'])
        self.assertFalse(report['database_creation_acknowledged'])
        self.assertTrue(report['qa_database_dropped'])
        self.assertIn('drop', events)
        self.assertNotIn('launch', events)
        self.assertIn('restore_main', events)

    @unittest.skipUnless(helper.os.name == 'nt', '仅Windows所有权控制流')
    def test_unknown_marker_without_exact_readback_is_never_dropped(self):
        code, report, events = self.controlled_run(write_missing=True)
        self.assertEqual(code, 1)
        self.assertTrue(report['database_creation_attempted'])
        self.assertFalse(report['qa_database_dropped'])
        self.assertNotIn('drop', events)
        self.assertTrue(report['cleanup_errors'])
        self.assertIn('restore_main', events)


if __name__ == '__main__':
    unittest.main()
