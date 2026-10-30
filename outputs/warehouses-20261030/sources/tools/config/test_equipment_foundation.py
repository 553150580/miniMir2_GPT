"""装备11字段、唯一选择与阶段公式；不把来源未知值默认为已定。"""
from copy import deepcopy
import unittest

import export_configs as config
from test_source_map_projection import module, ROOT


class EquipmentFoundationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema=config.load_json(ROOT/'config/schema/catalog.json')
        cls.tables,errors,_,_=config.collect(cls.schema)
        assert not errors,errors
        cls.fixture=module.fixture_from_rows(cls.tables)

    @staticmethod
    def profile(tables):return tables['dev_first_loop_profile'][0]['parameters']

    @staticmethod
    def equipment(tables,id=218):return next(row for row in tables['game_equipment'] if row['item_id']==id)

    def test_current_source_selection_and_four_explicit_zeros(self):
        self.assertEqual(self.fixture['rule_version'],23)
        self.assertEqual(self.profile(self.tables)['base_item_ids'],[35,36,193,218])
        self.assertNotIn(220,[item['id'] for item in self.fixture['items']])
        for item in self.fixture['items']:
            for field in ['magic_min_bonus','magic_max_bonus','tao_min_bonus','tao_max_bonus']:
                if item['is_equipment']:self.assertIn(field,item)
                self.assertEqual(item.get(field,0),0)
        self.assertEqual([(i['id'],i['equipment_stage'],i['recycle_gold']) for i in self.fixture['items'] if i['is_equipment']],[(218,0,50),(193,0,400)])
        self.assertFalse(self.fixture['casting_supply']['enabled'])
        self.assertNotIn(17,[skill['id'] for skill in self.fixture['skills']])

    def test_mc_sc_projection_uses_unique_equipment_values_and_public_whitelist(self):
        tables=deepcopy(self.tables);row=self.equipment(tables)
        row['base_attributes'].update(magic_attack_min=3,magic_attack_max=17,spirit_attack_min=4,spirit_attack_max=19)
        fixture=module.fixture_from_rows(tables);before=deepcopy(fixture);public=module.public_gameplay(fixture)
        item=next(i for i in fixture['items'] if i['id']==218)
        self.assertEqual([item[key] for key in ['magic_min_bonus','magic_max_bonus','tao_min_bonus','tao_max_bonus']],[3,17,4,19])
        shown=next(i for i in public['items'] if i['item_id']==218)
        self.assertEqual(shown['base_attributes'],row['base_attributes'])
        self.assertEqual(set(shown['base_attributes']),set(module.EQUIPMENT_ATTRIBUTE_BONUSES))
        self.assertEqual(len(shown['base_attributes']),11)
        shown['base_attributes']['magic_attack_max']=99
        self.assertEqual(fixture,before)
        for item in public['items']:
            if item['kind']!='equipment':
                self.assertNotIn('base_attributes',item);self.assertNotIn('equipment_stage',item)

    def test_selection_comes_from_profile_not_fixed_four_or_all_equipment(self):
        tables=deepcopy(self.tables);self.profile(tables)['base_item_ids']=[35,36,193]
        self.assertNotIn(218,[row['id'] for row in module.fixture_from_rows(tables)['items']])
        self.assertIn(218,[row['id'] for row in tables['game_equipment']])

    def test_selection_is_required_unique_known_typed_and_not_another_item_pipeline(self):
        for value in ['missing',None,[],{},[35,35],[True],[0],[-1],[1.5],['35'],[2**53],[999999],[25],[900001],[187]]:
            tables=deepcopy(self.tables);profile=self.profile(tables)
            if value=='missing':del profile['base_item_ids']
            else:profile['base_item_ids']=value
            with self.subTest(value=value),self.assertRaises(ValueError):module.fixture_from_rows(tables)
        tables=deepcopy(self.tables);tables['game_items'].append(deepcopy(next(r for r in tables['game_items'] if r['id']==35)))
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_rule22_attributes_never_fill_missing_or_accept_untyped_unknown_values(self):
        for key in module.EQUIPMENT_ATTRIBUTE_BONUSES:
            for value in ['missing',None,True,-1,1.5,'0',2**53]:
                tables=deepcopy(self.tables);attrs=self.equipment(tables)['base_attributes']
                if value=='missing':del attrs[key]
                else:attrs[key]=value
                with self.subTest(key=key,value=value),self.assertRaises(ValueError):module.fixture_from_rows(tables)
        tables=deepcopy(self.tables);self.equipment(tables)['base_attributes']['source_price']=3000
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_all_five_min_max_ranges_are_ordered(self):
        for prefix in ['physical_attack','magic_attack','spirit_attack','physical_defence','magic_defence']:
            tables=deepcopy(self.tables);self.equipment(tables)['base_attributes'].update({prefix+'_min':2,prefix+'_max':1})
            with self.subTest(prefix=prefix),self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_unknown_equipment_type_is_rejected_including_legacy(self):
        for version in [21,22]:
            for value in [None,'boots','Weapon','',False,0]:
                tables=deepcopy(self.tables);profile=self.profile(tables);profile['rule_version']=version
                if version<22:profile.pop('base_item_ids')
                profile.pop('warehouses')
                self.equipment(tables)['equipment_type']=value
                with self.subTest(version=version,value=value),self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_stage_uses_row_and_exact_requirement_formula_at_boundaries(self):
        for level in [1,10,11,20,21,50,2**32-1]:
            tables=deepcopy(self.tables);stage=(level-1)//10;self.equipment(tables).update(required_level=level,stage=stage)
            fixture=module.fixture_from_rows(tables);item=next(i for i in fixture['items'] if i['id']==218)
            self.assertEqual(item['equipment_stage'],stage)
            public=next(i for i in module.public_gameplay(fixture)['items'] if i['item_id']==218)
            self.assertEqual((public['equipment_stage'],public['required_level']),(stage,level))
        for value in ['missing',None,False,-1,1.5,'0',1]:
            tables=deepcopy(self.tables);row=self.equipment(tables)
            if value=='missing':del row['stage']
            else:row['stage']=value
            with self.subTest(value=value),self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_schema_requires_each_new_key_and_profile_selection_without_null_or_extras(self):
        for key in ['magic_attack_min','magic_attack_max','spirit_attack_min','spirit_attack_max']:
            for value in ['missing',None,False,-1,2**53]:
                tables=deepcopy(self.tables);attrs=self.equipment(tables)['base_attributes']
                if value=='missing':del attrs[key]
                else:attrs[key]=value
                with self.subTest(key=key,value=value):self.assertTrue(config.check_json_contracts(self.schema,tables))
        for value in ['missing',None,[False],[0],[2**53]]:
            tables=deepcopy(self.tables);profile=self.profile(tables)
            if value=='missing':del profile['base_item_ids']
            else:profile['base_item_ids']=value
            with self.subTest(value=value):self.assertTrue(config.check_json_contracts(self.schema,tables))

    def test_legacy_is_explicit_and_cannot_activate_new_attributes_by_downgrade(self):
        tables=deepcopy(self.tables);profile=self.profile(tables);profile['rule_version']=21;profile.pop('warehouses')
        with self.assertRaisesRegex(ValueError,'rule22'):module.fixture_from_rows(tables)
        profile.pop('base_item_ids');fixture=module.fixture_from_rows(tables)
        self.assertTrue(all('equipment_stage' not in item for item in fixture['items']))
        self.equipment(tables)['base_attributes']['magic_attack_max']=1
        with self.assertRaisesRegex(ValueError,'rule22'):module.fixture_from_rows(tables)


if __name__=='__main__':unittest.main()
