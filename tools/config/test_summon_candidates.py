"""17独立候选必须闭合typed配置，又不得通过来源登记暗启正常召唤。"""
import copy
import json
import unittest

import export_configs as config
from test_source_map_projection import module, ROOT
import prepare_pet_runtime as pets
import prepare_summon_candidates as summons


class SummonCandidateProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tables,errors,_,_=config.collect(config.load_json(ROOT/'config/schema/catalog.json'))
        assert not errors,errors
        cls.fixture=module.fixture_from_rows(cls.tables)

    def changed(self):return copy.deepcopy(self.tables)

    @staticmethod
    def pet(tables):return next(row for row in tables['game_pet_templates'] if row['id']==1)

    @staticmethod
    def skill(tables):return next(row for row in tables['game_skills'] if row['id']==17)

    @staticmethod
    def profile(tables):return tables['dev_first_loop_profile'][0]['parameters']

    def compile(self,tables):return summons.compile_catalog(tables,{'source':'explicit_test_evidence'})

    def test_candidate_is_complete_closed_and_not_selected_by_main(self):
        c=self.compile(self.tables);f=self.fixture
        self.assertEqual(set(c),{'schema_version','development_only','runtime_enabled','required_rule_version','skills','books','upgrades','pet_definition_ids','source_evidence'})
        self.assertEqual((c['schema_version'],c['development_only'],c['runtime_enabled'],c['required_rule_version']),(1,True,False,17))
        self.assertEqual(c['pet_definition_ids'],[1])
        self.assertEqual([row['id'] for row in c['skills']],[17])
        self.assertEqual([row['id'] for row in c['books']],[25])
        self.assertEqual(len(c['upgrades']),3)
        self.assertEqual(f['rule_version'],17)
        self.assertEqual(len(f['skills']),22)
        self.assertFalse(any(row['id']==17 for row in f['skills']))
        self.assertFalse(any(row['id']==25 for row in f['items']))
        self.assertFalse(any(row['skill_id']==17 for row in f['skill_upgrades']))
        self.assertFalse(any(row['item_id'] in [25,187] for row in f['skill_shop']))
        self.assertFalse(any(row['item_id']==25 for row in f['initial_skill_books']))
        self.assertIs(f['casting_supply']['enabled'],False)
        self.assertEqual(f['pet_runtime']['bootstrap_mode'],'legacy_untrained_pet')

    def test_source_costs_delay_and_birth_clock_have_no_duplicate_numbers(self):
        skill=self.compile(self.tables)['skills'][0]
        self.assertEqual((skill['id'],skill['job_id'],skill['book_item_id'],skill['learn_level'],skill['learned_level'],skill['max_level']),(17,2,25,19,0,3))
        self.assertEqual((skill['mp_base_cost'],skill['mp_level_cost'],skill['cooldown_base_ms'],skill['global_spell_ms'],skill['action_ms'],skill['delay_base_ms']),(12,4,1800,1800,600,500))
        self.assertEqual((skill['material_item_id'],skill['material_quantity']),(187,1))
        self.assertEqual(skill['summon'],{'definition_id':1})
        self.assertEqual(self.fixture['pet_runtime']['definitions'][0]['lifecycle']['spawn_ready_ms'],2000)
        self.assertEqual(self.fixture['pet_runtime']['definitions'][0]['training']['owner_range_cells'],16)
        self.assertNotIn('data_range_cells',self.pet(self.tables)['follow_and_return'])

    def test_unused_combat_fields_are_zero_or_empty_with_strict_types(self):
        skill=self.compile(self.tables)['skills'][0]
        for key in module.SKILL_DAMAGE_FIELDS:
            expected=False if key=='add_caster_level' else [] if key in ['accuracy_by_level','max_attack_bonus_by_level','ready_proc_numerators'] else 0
            self.assertEqual(skill[key],expected,key)
            self.assertIs(type(skill[key]),type(expected),key)
        self.assertEqual((skill['buff_id'],skill['auto_priority'],skill['auto_min_targets']),(0,0,0))
        for key in ['power_base','multiplier_base_ppm','non_undead_divisor','accuracy_by_level','add_caster_level']:
            tables=self.changed()
            self.skill(tables)['damage_parameters'][key]=[0,0,0,0] if key=='accuracy_by_level' else True if key=='add_caster_level' else 1
            with self.subTest(key=key),self.assertRaisesRegex(ValueError,'显式中性'):self.compile(tables)

    def test_recall_name_is_explicit_case_sensitive_and_versioned(self):
        definition=self.fixture['pet_runtime']['definitions'][0]
        self.assertEqual(definition['recall_name'],'BoneFamiliar')
        self.assertEqual(pets.build_catalog(self.tables,{'unit':'test'})['schema_version'],2)
        tables=self.changed();self.pet(tables)['follow_and_return']['recall_name']='bonefamiliar'
        self.assertEqual(pets.compile_definitions(tables,[1])[0]['recall_name'],'bonefamiliar')
        for value in [None,'','  ',False,0,[]]:
            tables=self.changed();self.pet(tables)['follow_and_return']['recall_name']=value
            with self.subTest(value=value),self.assertRaises(ValueError):pets.compile_definitions(tables,[1])
        tables=self.changed();self.profile(tables)['rule_version']=16
        with self.assertRaises(ValueError):pets.compile_definitions(tables,[1])
        del self.pet(tables)['follow_and_return']['recall_name']
        old=pets.build_catalog(tables,{'unit':'legacy16'})
        self.assertEqual(old['schema_version'],1)
        self.assertNotIn('recall_name',old['definitions'][0])

    def test_no_pets_is_independent_explicit_boolean_and_old_contract_rejects_it(self):
        for current in [self.fixture['map']]+self.fixture['maps']:
            self.assertIs(current['no_pets'],False)
            self.assertIs(current['no_teleport'],False)
        public=module.public_gameplay(self.fixture)
        for current in public['map_catalog']:
            self.assertIs(current['no_pets'],False)
        for value in [None,0,1,'false',[],{}]:
            tables=self.changed();policy=next(row for row in tables['game_map_regions'] if row['id']==202)
            policy['entry_rules']['no_pets']=value
            with self.subTest(value=value),self.assertRaises(ValueError):module.fixture_from_rows(tables)
        tables=self.changed();del next(row for row in tables['game_map_regions'] if row['id']==201)['entry_rules']['no_pets']
        with self.assertRaises(ValueError):self.compile(tables)
        tables=self.changed();self.profile(tables)['rule_version']=16
        definition=next(row for row in tables['game_maps'] if row['id']==1)
        with self.assertRaises(ValueError):module.map_policy(tables,definition,self.profile(tables))
        tables=self.changed();next(row for row in tables['game_map_regions'] if row['id']==202)['entry_rules']['no_pets']=True
        f=module.fixture_from_rows(tables)
        self.assertIs(f['maps'][0]['no_pets'],True)
        self.assertIs(f['maps'][0]['no_teleport'],False)
        self.assertEqual(f['maps'][0]['safe_zones'],self.fixture['maps'][0]['safe_zones'])

    def test_candidate_never_opens_runtime_or_changes_original_input(self):
        before=copy.deepcopy(self.tables)
        first=self.compile(self.tables)
        first['skills'][0]['summon']['definition_id']=999
        first['books'][0]['allowed_jobs'].append(0)
        self.assertEqual(self.tables,before)
        self.assertEqual(self.compile(self.tables)['skills'][0]['summon'],{'definition_id':1})
        for kind in ['selected','supply','bootstrap','old_version']:
            tables=self.changed();profile=self.profile(tables)
            if kind=='selected':profile['skill_ids'].append(17)
            elif kind=='supply':profile['casting_supply']['enabled']=True
            elif kind=='bootstrap':profile['pet_runtime']['bootstrap_mode']='none'
            else:profile['rule_version']=16
            with self.subTest(kind=kind),self.assertRaises(ValueError):self.compile(tables)

    def test_definition_link_and_material_cannot_be_guessed(self):
        for kind in ['missing_summon','unknown_key','boolean_id','wrong_id','extra_effect','quantity','unknown_material']:
            tables=self.changed();skill=self.skill(tables)
            if kind=='missing_summon':del skill['target_rule']['summon']
            elif kind=='unknown_key':skill['target_rule']['summon']['delay_ms']=500
            elif kind=='boolean_id':skill['target_rule']['summon']['definition_id']=True
            elif kind=='wrong_id':skill['target_rule']['summon']['definition_id']=2
            elif kind=='extra_effect':skill['effect_ids']=[21]
            elif kind=='quantity':skill['resource_costs']['material_quantity']=2
            else:skill['resource_costs']['material_item_id']=999
            with self.subTest(kind=kind),self.assertRaises(ValueError):self.compile(tables)

    def test_book_is_complete_typed_item_and_does_not_inherit_equipment(self):
        book=self.compile(self.tables)['books'][0]
        self.assertEqual(set(book),{'id','name','equipment_slot','is_equipment','attack_bonus','attack_min_bonus','attack_max_bonus',
            'attack_speed_bonus','defence_bonus','recycle_gold','stackable','allowed_jobs','book_skill_id','is_skill_page','drop_quality',
            'max_stack','defence_min_bonus','defence_max_bonus','magic_defence_min_bonus','magic_defence_max_bonus'})
        self.assertEqual((book['id'],book['book_skill_id'],book['allowed_jobs']),(25,17,[2]))
        self.assertIs(book['is_equipment'],False);self.assertIs(book['is_skill_page'],False)
        tables=self.changed();next(row for row in tables['game_items'] if row['id']==25)['max_stack']=None
        with self.assertRaisesRegex(ValueError,'不补|不能补'):self.compile(tables)

    def test_all_three_upgrades_reuse_source_policy_without_lowering_level(self):
        upgrades=self.compile(self.tables)['upgrades']
        self.assertEqual([row['level'] for row in upgrades],[1,2,3])
        self.assertEqual([row['required_role_level'] for row in upgrades],[19,22,26])
        self.assertEqual([row['page_cost'] for row in upgrades],[5,10,20])
        self.assertEqual([row['book_costs'] for row in upgrades],[[{'item_id':25,'quantity':rank+1}] for rank in [1,2,3]])
        for kind in ['missing','duplicate','books','fractional_level','shape']:
            tables=self.changed();rows=[row for row in tables['game_skill_upgrades'] if row['skill_id']==17]
            if kind=='missing':tables['game_skill_upgrades'].remove(rows[0])
            elif kind=='duplicate':rows[1]['level']=1
            elif kind=='books':rows[0]['book_cost']=[{'item_id':25,'quantity':1}]
            elif kind=='fractional_level':rows[0]['required_role_level']=19.5
            else:rows[0]['preserve_behavior_shape']=False
            with self.subTest(kind=kind),self.assertRaises(ValueError):self.compile(tables)

    def test_numeric_inputs_are_projected_from_their_unique_fields(self):
        tables=self.changed();skill=self.skill(tables)
        skill['resource_costs']['mp_base_cost']=13
        skill['cast_timing']['delay_base_ms']=550
        self.pet(tables)['lifetime_and_betrayal']['spawn_ready_ms']=2500
        out=self.compile(tables)['skills'][0]
        self.assertEqual((out['mp_base_cost'],out['delay_base_ms']),(13,550))
        self.assertEqual(pets.compile_definitions(tables,[1])[0]['lifecycle']['spawn_ready_ms'],2500)


if __name__=='__main__':unittest.main()
