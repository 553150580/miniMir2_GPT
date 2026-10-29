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

    def test_visit_gate_is_independent_of_initializer_and_required_by_teleport(self):
        fixture=module.fixture_from_rows(self.tables)
        self.assertEqual(fixture['rule_version'],21)
        self.assertEqual(fixture['binding'],{'enabled':False,'start_points':[],'safe_zone_enabled':True})
        self.assertTrue(any(skill['id']==21 for skill in fixture['skills']))
        tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['binding']['safe_zone_enabled']=False
        with self.assertRaisesRegex(ValueError,'可信来源安全区绑定'):module.fixture_from_rows(tables)

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
        # 仅删除新gate也不能把当前21/NoTeleport配置伪装成旧版本合法输入。
        with self.assertRaisesRegex(ValueError,'rule15'):module.fixture_from_rows(tables)
        # 独立旧地图投影只验证历史二字段binding合同；明确不投影未来技能和地图政策。
        legacy=copy.deepcopy(self.tables)
        legacy['game_map_regions']=[row for row in legacy['game_map_regions'] if row['entry_rules'].get('region_kind')!='map_policy']
        maps,_=module.source_map_catalog(legacy,p)
        self.assertEqual(len(maps),1)
        self.assertNotIn('no_teleport',maps[0]);self.assertNotIn('source_static_bits',maps[0])

    def test_unknown_fields_and_initialization_pool_stay_rejected(self):
        for change in [{'unknown':True},{'enabled':True},{'start_points':[{'map_id':2,'x':650,'y':631}]}]:
            with self.subTest(change=change):
                tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['binding'].update(change)
                with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_visit_gate_requires_source_town_before_runtime_generation(self):
        p=copy.deepcopy(self.tables['dev_first_loop_profile'][0]['parameters']);p['map_catalog_ids']=[]
        with self.assertRaisesRegex(ValueError,'先接入'):module.source_map_catalog(self.tables,p)

if __name__=='__main__':unittest.main()
