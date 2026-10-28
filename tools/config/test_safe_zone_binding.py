"""安全区到访门禁不能隐式打开来源随机新生池或接受缺失配置。"""
import copy
import importlib.util
from pathlib import Path
import unittest
import export_configs as config

ROOT=Path(__file__).resolve().parents[2]
SPEC=importlib.util.spec_from_file_location('prepare_safe_zone_binding',ROOT/'tools/resources/prepare_first_loop.py')
module=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(module)

class SafeZoneBindingProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tables,errors,_,_=config.collect(config.load_json(ROOT/'config/schema/catalog.json'))
        assert not errors,errors

    def test_visit_gate_is_independent_and_teleport_stays_closed(self):
        fixture=module.fixture_from_rows(self.tables)
        self.assertEqual(fixture['rule_version'],14)
        self.assertEqual(fixture['binding'],{'enabled':False,'start_points':[],'safe_zone_enabled':True})
        self.assertFalse(any(skill['id']==21 for skill in fixture['skills']))
        tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['binding']['safe_zone_enabled']=False
        self.assertFalse(module.fixture_from_rows(tables)['binding']['safe_zone_enabled'])

    def test_missing_null_and_untyped_visit_gate_are_rejected(self):
        for value in [None,0,1,'true',[],{}]:
            with self.subTest(value=value):
                tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['binding']['safe_zone_enabled']=value
                with self.assertRaises(ValueError):module.fixture_from_rows(tables)
        tables=copy.deepcopy(self.tables);del tables['dev_first_loop_profile'][0]['parameters']['binding']['safe_zone_enabled']
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_legacy_contract_does_not_silently_enable_new_business(self):
        tables=copy.deepcopy(self.tables);p=tables['dev_first_loop_profile'][0]['parameters'];p['rule_version']=13
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)
        del p['binding']['safe_zone_enabled']
        self.assertEqual(module.fixture_from_rows(tables)['binding'],{'enabled':False,'start_points':[]})

    def test_unknown_fields_and_initialization_pool_stay_rejected(self):
        for change in [{'unknown':True},{'enabled':True},{'start_points':[{'map_id':2,'x':650,'y':631}]}]:
            with self.subTest(change=change):
                tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['binding'].update(change)
                with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_visit_gate_requires_source_town_before_runtime_generation(self):
        p=copy.deepcopy(self.tables['dev_first_loop_profile'][0]['parameters']);p['map_catalog_ids']=[]
        with self.assertRaisesRegex(ValueError,'先接入'):module.source_map_catalog(self.tables,p)

if __name__=='__main__':unittest.main()
