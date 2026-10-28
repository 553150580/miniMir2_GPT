"""城镇旅行与导航合同不继承代码默认，也不从旧生成物补数。"""
import copy
import unittest
from test_source_map_projection import module,ROOT
import export_configs as config

class TownRuntimeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tables,errors,_,_=config.collect(config.load_json(ROOT/'config/schema/catalog.json'));assert not errors,errors

    def test_runtime_budgets_and_public_gate_are_projected_only_from_unique_workbook(self):
        fixture=module.fixture_from_rows(self.tables);profile=self.tables['dev_first_loop_profile'][0]['parameters']
        self.assertEqual(fixture['town_travel'],profile['town_travel']);self.assertEqual(fixture['town_navigation'],profile['town_navigation'])
        self.assertEqual(module.public_gameplay(fixture)['town_travel'],{'enabled':True,'town_map_id':'2'})
        self.assertFalse(fixture['binding']['enabled']);self.assertTrue(any(s['id']==21 for s in fixture['skills']))

    def test_missing_fractional_and_boolean_budgets_are_rejected(self):
        for group in ['town_travel','town_navigation']:
            for field in self.tables['dev_first_loop_profile'][0]['parameters'][group]:
                tables=copy.deepcopy(self.tables);del tables['dev_first_loop_profile'][0]['parameters'][group][field]
                with self.subTest(group=group,missing=field),self.assertRaises(ValueError):module.fixture_from_rows(tables)
        for value in [1024.0,True,0]:
            tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['town_navigation']['work_per_task']=value
            with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_memory_and_worker_total_budgets_are_not_per_room_defaults(self):
        for change in [{'max_memory_bytes_per_worker':1},{'workspaces_per_worker':16},{'work_per_worker_tick':1},{'max_route_cells':490001}]:
            tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['town_navigation'].update(change)
            with self.subTest(change=change),self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_only_known_town_and_enabled_bounded_navigation_allow_travel(self):
        for map_id in [1,99]:
            tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['town_travel']['town_map_id']=map_id
            with self.assertRaises(ValueError):module.fixture_from_rows(tables)
        tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['town_navigation']['enabled']=False
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)

if __name__=='__main__':unittest.main()
