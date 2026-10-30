"""宠物身体/训练只编译唯一Excel明确值，不能借候选开启17或改免费bootstrap。"""
import copy
import json
import unittest

import export_configs as config
from test_source_map_projection import module, ROOT
import prepare_pet_runtime as pets


class PetRuntimeProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema=config.load_json(ROOT/'config/schema/catalog.json')
        cls.tables,errors,_,_=config.collect(cls.schema)
        assert not errors,errors
        cls.fixture=module.fixture_from_rows(cls.tables)

    def changed(self):
        return copy.deepcopy(self.tables)

    @staticmethod
    def definition(tables):
        return next(row for row in tables['game_pet_templates'] if row['id']==1)

    @staticmethod
    def profile(tables):
        return tables['dev_first_loop_profile'][0]['parameters']

    def test_rule16_selects_shared_definition_without_opening_summon_or_training_bootstrap(self):
        f=self.fixture
        self.assertEqual(f['rule_version'],23)
        self.assertEqual(self.profile(self.tables)['pet_runtime'],{'enabled':True,'bootstrap_mode':'legacy_untrained_pet','definition_ids':[1]})
        self.assertEqual(f['pet_runtime']['bootstrap_mode'],'legacy_untrained_pet')
        self.assertEqual(f['pet_runtime']['definitions'],pets.compile_definitions(self.tables,[1]))
        self.assertEqual(self.definition(self.tables)['summon_skill_id'],17)
        self.assertEqual(len(f['skills']),22)
        self.assertNotIn(17,[skill['id'] for skill in f['skills']])
        self.assertIs(f['casting_supply']['enabled'],False)
        body=f['pet_runtime']['definitions'][0]
        self.assertEqual(body['base'],{key:value for key,value in f['pet'].items() if key!='undead'})
        self.assertEqual(body['undead'],f['pet']['undead'])
        self.assertNotIn('undead',body['base'])

    def test_candidate_is_disabled_and_uses_exact_same_definitions_and_budget(self):
        evidence={'source':'unit-test','sha256':'explicit'}
        catalog=pets.build_catalog(self.tables,evidence)
        self.assertIs(catalog['runtime_enabled'],False)
        self.assertEqual(catalog['schema_version'],2)
        self.assertIs(catalog['development_only'],True)
        self.assertEqual(catalog['definitions'],self.fixture['pet_runtime']['definitions'])
        self.assertEqual(catalog['budgets'],self.fixture['pet_runtime']['budgets'])
        catalog['source_evidence']['source']='mutated'
        catalog['definitions'][0]['training']['initial_level_by_rank'][0]=77
        self.assertEqual(evidence['source'],'unit-test')
        self.assertEqual(pets.compile_definitions(self.tables,[1])[0]['training']['initial_level_by_rank'][0],0)

    def test_budget_is_unique_explicit_integer_and_software_bounded(self):
        self.assertEqual(pets.compile_budgets(self.tables),{'retained_body_capacity_per_owner':128,'full_policy':pets.FULL_POLICY})
        for value in [None,True,0,129,1.5,'128']:
            with self.subTest(value=value):
                tables=self.changed()
                row=next(row for row in tables['arch_runtime_budgets'] if row['code']==pets.CAPACITY_CODE)
                row['value']=value
                with self.assertRaises(ValueError):pets.compile_budgets(tables)
                # 既有外层可空字段合同不改；显式必需性由本编译器拒绝null。
                if value is not None:self.assertTrue(config.check_json_contracts(self.schema,tables))
        for count in [0,2]:
            tables=self.changed()
            row=next(row for row in tables['arch_runtime_budgets'] if row['code']==pets.CAPACITY_CODE)
            tables['arch_runtime_budgets']=[item for item in tables['arch_runtime_budgets'] if item['code']!=pets.CAPACITY_CODE]+[row]*count
            with self.assertRaises(ValueError):pets.compile_budgets(tables)

    def test_base_inherits_monster_and_shared_weight_instead_of_duplicating_numbers(self):
        tables=self.changed()
        template=self.definition(tables)
        monster=next(row for row in tables['game_monsters'] if row['id']==template['monster_template_id'])
        monster['hp']=3456
        monster['attributes']['physical_attack_min']=17
        monster['attributes']['physical_attack_max']=23
        for player in self.profile(tables)['players']:player['magic_resist_weight']=37
        base,undead=pets.compile_base(tables,template)
        self.assertEqual((base['hp'],base['attack_min'],base['attack_max'],base['magic_resist_weight']),(3456,17,23,37))
        self.assertEqual(base['level'],monster['level'])
        self.assertEqual(base['definition_id'],monster['id'])
        self.assertEqual(base['job_id'],template['base_attributes']['owner_job_id'])
        self.assertIs(undead,monster['attributes']['undead'])
        self.profile(tables)['players'][1]['magic_resist_weight']=38
        with self.assertRaisesRegex(ValueError,'全职业相同'):pets.compile_base(tables,template)

    def test_base_descriptor_rejects_unknown_numbers_defaults_and_boolean_identity(self):
        for kind in ['unknown','missing','neutral_nonzero','neutral_bool','source','body_bool','level_zero']:
            with self.subTest(kind=kind):
                tables=self.changed();row=self.definition(tables);rules=row['base_attributes']
                if kind=='unknown':rules['hp']=300
                elif kind=='missing':del rules['neutral_attributes']
                elif kind=='neutral_nonzero':rules['neutral_attributes']['magic_min']=1
                elif kind=='neutral_bool':rules['neutral_attributes']['magic_min']=False
                elif kind=='source':rules['magic_resist_weight_source']='first_player'
                elif kind=='body_bool':row['monster_template_id']=True
                else:next(item for item in tables['game_monsters'] if item['id']==2)['level']=0
                with self.assertRaises(ValueError):pets.compile_definitions(tables,[1])

    def test_source_training_speed_and_lifecycle_are_explicit(self):
        body=pets.compile_definitions(self.tables,[1])[0]
        self.assertEqual(body['source_skill_id'],17)
        self.assertEqual(body['training'],{'initial_level_by_rank':[0,1,2,3],'maximum_level_by_rank':[4,5,6,7],
            'experience_multiplier':3,'owner_range_cells':16,'experience_basis':'owner_actual_granted_experience',
            'grant_phase':'confirmed_owner_experience','eligibility':'alive_same_world_within_owner_range',
            'max_level_ups_per_grant':1,'excess_experience':'keep','at_maximum':'do_not_accumulate','current_hp_on_growth':'keep'})
        self.assertEqual(body['speed'],{'basis':'maximum_training_level','move_reduction_ms_per_level':130,'attack_reduction_ms_per_level':70,
            'minimum_move_interval_ms':400,'minimum_attack_interval_ms':400})
        self.assertEqual(body['lifecycle'],{'corpse_ttl_ms':180000,'spawn_ready_ms':2000,'live_expiry':'no_tame_expiry','cold_enter':'do_not_restore_temporary_pet'})
        self.assertNotIn('no_pets',body)
        self.assertNotIn('minimum_birth_interval_ms',body)

    def test_rule_objects_reject_missing_null_extra_and_wrong_types(self):
        paths=[('follow_and_return',None),('follow_and_return','training'),('follow_and_return','speed'),('lifetime_and_betrayal',None)]
        for parent,child in paths:
            for kind in ['missing','null','extra']:
                with self.subTest(parent=parent,child=child,kind=kind):
                    tables=self.changed();row=self.definition(tables);rules=row[parent] if child is None else row[parent][child]
                    if kind=='missing':del rules[next(iter(rules))]
                    elif kind=='extra':rules['default']=0
                    elif child is None:row[parent]=None
                    else:row[parent][child]=None
                    with self.assertRaises(ValueError):pets.compile_definitions(tables,[1])
        for field,value in [('owner_range_cells',True),('experience_multiplier',1.5),('max_level_ups_per_grant',True),
                            ('grant_phase','kill_before_confirm'),('current_hp_on_growth','heal_to_full')]:
            tables=self.changed();self.definition(tables)['follow_and_return']['training'][field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):pets.compile_definitions(tables,[1])

    def test_training_rank_bounds_and_speed_basis_cannot_be_changed_implicitly(self):
        for key,value in [('initial_level_by_rank',[0,1,2]),('maximum_level_by_rank',[4,5,6,False]),
                          ('initial_level_by_rank',[0,1,2,8]),('maximum_level_by_rank',[4,5,6,256])]:
            tables=self.changed();self.definition(tables)['follow_and_return']['training'][key]=value
            with self.subTest(key=key,value=value),self.assertRaises(ValueError):pets.compile_definitions(tables,[1])
        tables=self.changed();self.definition(tables)['follow_and_return']['speed']['basis']='current_training_level'
        with self.assertRaises(ValueError):pets.compile_definitions(tables,[1])

    def test_growth_is_eight_order_independent_levels_with_terminal_null_only(self):
        tables=self.changed();tables['game_pet_growth'].reverse()
        growth=pets.compile_definitions(tables,[1])[0]['growth']
        self.assertEqual([row['level'] for row in growth],list(range(8)))
        for level,row in enumerate(growth):
            self.assertEqual(row['hp_bonus'],20*level)
            self.assertEqual(row['attack_min_bonus'],level)
            self.assertEqual(row['defence_max_bonus'],2*level)
            self.assertEqual(row['required_experience'],None if level==7 else (level+1)*20000)
        for kind in ['missing','duplicate','gap','null_before_end','non_null_end','boolean','overflow','inverted']:
            tables=self.changed();rows=tables['game_pet_growth']
            if kind=='missing':rows.pop()
            elif kind=='duplicate':rows[1]['growth_level']=0
            elif kind=='gap':rows[1]['growth_level']=9
            elif kind=='null_before_end':rows[1]['required_conditions']['required_experience']=None
            elif kind=='non_null_end':rows[-1]['required_conditions']['required_experience']=160000
            elif kind=='boolean':rows[0]['growth_level']=False
            elif kind=='overflow':rows[0]['attributes']['hp_bonus']=pets.I64
            else:rows[0]['attributes']['attack_min_bonus']=1000
            with self.subTest(kind=kind),self.assertRaises(ValueError):pets.compile_definitions(tables,[1])

    def test_selection_never_fills_missing_ids_or_defaults(self):
        for ids in [None,[],[1,1],[True],[2],[0],[2**32]]:
            with self.subTest(ids=ids),self.assertRaises(ValueError):pets.compile_definitions(self.tables,ids)
        for selected in [None,{}, {'enabled':False,'bootstrap_mode':'legacy_untrained_pet','definition_ids':[1]},
                         {'enabled':True,'bootstrap_mode':'legacy','definition_ids':[1]}]:
            profile=copy.deepcopy(self.profile(self.tables));profile['pet_runtime']=selected
            with self.subTest(selected=selected),self.assertRaises(ValueError):pets.compile_runtime(self.tables,profile)
        profile=copy.deepcopy(self.profile(self.tables));profile['rule_version']=15
        with self.assertRaisesRegex(ValueError,'rule16'):pets.compile_runtime(self.tables,profile)
        del profile['pet_runtime']
        self.assertIsNone(pets.compile_runtime(self.tables,profile))

    def test_legacy_bootstrap_cannot_diverge_from_inherited_fighter(self):
        tables=self.changed();self.definition(tables)['base_attributes']['owner_job_id']=1
        with self.assertRaisesRegex(ValueError,'旧免费bootstrap'):module.fixture_from_rows(tables)

    def test_public_hides_training_body_budgets_and_source_metadata(self):
        public=module.public_gameplay(self.fixture)
        self.assertEqual(public['rule_version'],23)
        self.assertEqual(len(public['pet_profiles']),1)
        serialized=json.dumps(public)
        for field in ['pet_runtime','retained_body_capacity_per_owner','experience_multiplier','source_evidence','corpse_ttl_ms']:
            self.assertNotIn(field,serialized)


if __name__=='__main__':unittest.main()
