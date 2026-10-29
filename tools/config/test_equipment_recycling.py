"""开发自动回收只投影已选装备的唯一等级与既有金币候选。"""
import copy
import unittest

import export_configs as config
from test_source_map_projection import module, ROOT


class EquipmentRecyclingProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema=config.load_json(ROOT/'config/schema/catalog.json')
        cls.tables,errors,_,_=config.collect(cls.schema)
        assert not errors,errors
        cls.fixture=module.fixture_from_rows(cls.tables)

    def changed(self):
        return copy.deepcopy(self.tables)

    @staticmethod
    def profile(tables):
        return tables['dev_first_loop_profile'][0]['parameters']

    def test_current_development_contract_preserves_prices_and_other_stage_gates(self):
        f=self.fixture
        self.assertEqual(f['rule_version'],19)
        self.assertEqual(f['equipment_recycling'],{'mode':'development_gold_only','outputs':[]})
        self.assertEqual([(r['id'],r['required_level'],r['drop_quality'],r['recycle_gold']) for r in f['items'] if r['is_equipment']],[(218,1,0,50),(193,1,0,400)])
        self.assertEqual(len(f['skills']),22)
        self.assertNotIn(17,[r['id'] for r in f['skills']])
        self.assertIs(f['casting_supply']['enabled'],False)
        self.assertEqual(f['pet_runtime']['bootstrap_mode'],'legacy_untrained_pet')
        evidence=f['_source_evidence']['equipment_recycling']
        self.assertIs(evidence['formal_recipe_published'],False)
        self.assertIs(evidence['formal_recycle_values_consumed'],False)
        self.assertIs(evidence['durability_model_implemented'],False)

    def test_required_level_and_gold_follow_their_existing_unique_cells(self):
        tables=self.changed();before=copy.deepcopy(tables)
        next(r for r in tables['game_equipment'] if r['item_id']==218)['required_level']=37
        self.profile(tables)['recycle_test_gold_by_item']['218']=73
        f=module.fixture_from_rows(tables)
        item=next(r for r in f['items'] if r['id']==218)
        self.assertEqual((item['required_level'],item['recycle_gold']),(37,73))
        self.assertEqual(before['game_recycle_values'],tables['game_recycle_values'])

    def test_quality_is_explicit_and_equipment_and_item_must_agree(self):
        for value in [None,False,-1,5,1.5,'0','missing','mismatch']:
            tables=self.changed();row=next(r for r in tables['game_equipment'] if r['item_id']==218)
            item=next(r for r in tables['game_items'] if r['id']==218)
            if value=='missing':del row['quality']
            elif value=='mismatch':item['quality']=1
            else:row['quality']=value
            with self.subTest(value=value),self.assertRaises(ValueError):module.fixture_from_rows(tables)
        tables=self.changed()
        for key in ['game_equipment','game_items']:
            next(r for r in tables[key] if r['id']==218)['quality']=3
        item=next(r for r in module.fixture_from_rows(tables)['items'] if r['id']==218)
        self.assertEqual(item['drop_quality'],3)

    def test_missing_duplicate_or_invalid_equipment_level_is_not_defaulted(self):
        for kind in ['missing_definition','duplicate_definition','missing_level',None,False,0,-1,1.5,'1',2**32]:
            tables=self.changed();row=next(r for r in tables['game_equipment'] if r['item_id']==218)
            if kind=='missing_definition':tables['game_equipment'].remove(row)
            elif kind=='duplicate_definition':tables['game_equipment'].append(copy.deepcopy(row))
            elif kind=='missing_level':del row['required_level']
            else:row['required_level']=kind
            with self.subTest(kind=kind),self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_gold_is_explicit_nonnegative_int64_and_never_source_price_fallback(self):
        for value in [None,False,-1,1.5,'50',2**63]:
            items=copy.deepcopy(self.fixture['items'])
            next(r for r in items if r['is_equipment'])['recycle_gold']=value
            with self.subTest(value=value),self.assertRaises(ValueError):
                module.equipment_recycling_rules(self.profile(self.tables),items)
        tables=self.changed();del self.profile(tables)['recycle_test_gold_by_item']['218']
        with self.assertRaises(KeyError):module.fixture_from_rows(tables)

    def test_contract_rejects_null_unknown_mode_missing_and_nonempty_outputs(self):
        bad=[None,{}, {'mode':'official','outputs':[]},{'mode':'development_gold_only'},
             {'mode':'development_gold_only','outputs':None},{'mode':'development_gold_only','outputs':False},
             {'mode':'development_gold_only','outputs':{}},{'mode':'development_gold_only','outputs':[{}]},
             {'mode':'development_gold_only','outputs':[],'gold':50}]
        for rules in bad:
            profile=copy.deepcopy(self.profile(self.tables));profile['equipment_recycling']=rules
            with self.subTest(rules=rules),self.assertRaises(ValueError):module.equipment_recycling_rules(profile,self.fixture['items'])
        profile=copy.deepcopy(self.profile(self.tables));del profile['equipment_recycling']
        with self.assertRaises(ValueError):module.equipment_recycling_rules(profile,self.fixture['items'])

    def test_schema_requires_explicit_array_and_exact_development_contract_keys(self):
        for value in [None,{}, {'mode':'development_gold_only','outputs':None},
                      {'mode':'development_gold_only','outputs':[],'unexpected':False}]:
            tables=self.changed();self.profile(tables)['equipment_recycling']=value
            with self.subTest(value=value):self.assertTrue(config.check_json_contracts(self.schema,tables))

    def test_historical_contract_cannot_hide_new_recycling_fields(self):
        profile=copy.deepcopy(self.profile(self.tables));profile['rule_version']=18
        with self.assertRaisesRegex(ValueError,'rule19'):module.equipment_recycling_rules(profile,self.fixture['items'])
        del profile['equipment_recycling']
        self.assertIsNone(module.equipment_recycling_rules(profile,self.fixture['items']))

    def test_formal_recipe_nulls_are_preserved_and_not_interpreted_as_zero(self):
        row=self.tables['game_recycle_values'][0]
        for key in ['gold_distribution','stone_probability_ppm','broken_byproducts','oil_probability_formula','rewards_combinable']:
            self.assertIsNone(row[key])
        tables=self.changed();tables['game_recycle_values']=[]
        f=module.fixture_from_rows(tables)
        self.assertEqual(f['equipment_recycling'],self.fixture['equipment_recycling'])
        self.assertEqual(f['items'],self.fixture['items'])

    def test_public_only_equipment_has_authoritative_level_and_candidate_gold(self):
        f=copy.deepcopy(self.fixture);before=copy.deepcopy(f);public=module.public_gameplay(f)
        self.assertEqual(public['equipment_recycling'],f['equipment_recycling'])
        for item in public['items']:
            if item['kind']=='equipment':
                source=next(row for row in f['items'] if row['id']==item['item_id'])
                self.assertEqual((item['required_level'],item['recycle_gold'],item['quality']),(source['required_level'],source['recycle_gold'],source['drop_quality']))
            else:
                self.assertNotIn('required_level',item);self.assertNotIn('recycle_gold',item)
            self.assertNotIn('original_durability_u',item)
        public['equipment_recycling']['outputs'].append({'not_runtime':True})
        self.assertEqual(f,before)


if __name__=='__main__':unittest.main()
