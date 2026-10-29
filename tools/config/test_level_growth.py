"""连续来源经验与显式等级/待用池合同，不把第50门槛当自动51许可。"""
from copy import deepcopy
import re
import unittest

import export_configs as config
from test_source_map_projection import module, ROOT


class LevelGrowthProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema=config.load_json(ROOT/'config/schema/catalog.json')
        cls.tables,errors,_,_=config.collect(cls.schema)
        assert not errors,errors
        cls.profile=cls.tables['dev_first_loop_profile'][0]['parameters']
        cls.fixture=module.fixture_from_rows(cls.tables)

    def test_current_curve_matches_only_registered_exp_section_with_explicit_rule_sources(self):
        source={};section=''
        for line in (ROOT/'资料参考/myf/Mir200/Exps.ini').read_bytes().decode('gb18030').splitlines():
            h=re.fullmatch(r'\s*\[(.+)\]\s*',line)
            if h:section=h[1]
            m=re.fullmatch(r'Level(\d+)=(\d+)',line.strip())
            if section=='Exp' and m:source[int(m[1])]=int(m[2])
        r=self.fixture['progression']
        self.assertEqual(self.fixture['rule_version'],21)
        self.assertEqual(r['exp_per_level'],[source[n] for n in range(1,51)])
        self.assertEqual(len(r['exp_per_level']),50)
        for key in ['initial_level_cap','rebirth_level_cap_increment','rebirth_reset_progression']:
            self.assertEqual(r[key],next(row['rule_value'] for row in self.tables['game_progression_rules'] if row['rule_key']==key))
            self.assertNotIn(key,self.profile['progression'])
        self.assertEqual((r['operating_level_cap'],r['catalog_max_level']),(50,50))
        self.assertIs(r['rebirth_enabled'],False)
        evidence=self.fixture['_source_evidence']['progression']
        self.assertIs(evidence['balance_verified'],False);self.assertIs(evidence['natural_level_1_to_50_verified'],False)

    def test_reordering_source_rows_does_not_relabel_levels(self):
        tables=deepcopy(self.tables);tables['game_exp_levels'].reverse()
        self.assertEqual(module.progression_rules(tables,self.profile),self.fixture['progression'])

    def test_missing_duplicate_extra_or_invalid_level_cannot_be_compressed_to_array(self):
        for value in ['missing_row','duplicate','extra','missing_field',None,False,0,-1,1.5,'21',2**32]:
            tables=deepcopy(self.tables);rows=tables['game_exp_levels']
            if value=='missing_row':rows.pop(20)
            elif value=='duplicate':rows[20]['level']=20
            elif value=='extra':rows.append({**deepcopy(rows[-1]),'id':51,'level':51})
            elif value=='missing_field':del rows[20]['level']
            else:rows[20]['level']=value
            with self.subTest(value=value),self.assertRaises(ValueError):module.progression_rules(tables,self.profile)

    def test_threshold_requires_explicit_positive_excel_exact_integer(self):
        for value in ['missing',None,False,0,-1,1.5,'1100',2**53,2**63]:
            tables=deepcopy(self.tables)
            if value=='missing':del tables['game_exp_levels'][20]['next_level_exp']
            else:tables['game_exp_levels'][20]['next_level_exp']=value
            with self.subTest(value=value),self.assertRaises(ValueError):module.progression_rules(tables,self.profile)

    def test_current_profile_cannot_default_any_policy_or_enable_rebirth(self):
        for key in module.PROGRESSION_FIELDS:
            profile=deepcopy(self.profile);del profile['progression'][key]
            with self.subTest(missing=key),self.assertRaises(ValueError):module.progression_rules(self.tables,profile)
        for kind in ['missing','null','unknown','curve','pool','new_overflow','legacy_overflow','resources','rebirth','cap_null','cap_bool','cap_zero','cap_float','cap_overflow','catalog_mismatch','operating_mismatch']:
            p=deepcopy(self.profile);r=p['progression']
            if kind=='missing':del p['progression']
            elif kind=='null':p['progression']=None
            elif kind=='unknown':r['extra']=0
            elif kind=='curve':r['curve_version']='other'
            elif kind=='pool':r['pending_pool_policy']='unlimited'
            elif kind=='new_overflow':r['new_overflow_policy']='discard_silently'
            elif kind=='legacy_overflow':r['legacy_overflow_policy']='spend_existing'
            elif kind=='resources':r['level_up_resources']='always_heal'
            elif kind=='rebirth':r['rebirth_enabled']=True
            elif kind=='cap_null':r['operating_level_cap']=None
            elif kind=='cap_bool':r['operating_level_cap']=False
            elif kind=='cap_zero':r['operating_level_cap']=0
            elif kind=='cap_float':r['operating_level_cap']=50.5
            elif kind=='cap_overflow':r['operating_level_cap']=2**32
            elif kind=='catalog_mismatch':r['catalog_max_level']=51
            else:r['operating_level_cap']=49
            with self.subTest(kind=kind),self.assertRaises(ValueError):module.progression_rules(self.tables,p)

    def test_confirmed_rules_are_unique_required_and_typed_not_profile_defaults(self):
        for kind in ['missing','duplicate','candidate','null','boolean_cap','zero_increment','reset_true','reset_string']:
            tables=deepcopy(self.tables);rows=tables['game_progression_rules'];row=next(r for r in rows if r['rule_key']=='initial_level_cap')
            if kind=='missing':rows.remove(row)
            elif kind=='duplicate':rows.append({**deepcopy(row),'id':999})
            elif kind=='candidate':row['record_status']='candidate'
            elif kind=='null':row['rule_value']=None
            elif kind=='boolean_cap':row['rule_value']=True
            elif kind=='zero_increment':next(r for r in rows if r['rule_key']=='rebirth_level_cap_increment')['rule_value']=0
            else:next(r for r in rows if r['rule_key']=='rebirth_reset_progression')['rule_value']=True if kind=='reset_true' else 'false'
            with self.subTest(kind=kind),self.assertRaises(ValueError):module.progression_rules(tables,self.profile)

    def test_schema_rejects_missing_null_bool_and_unknown_nested_profile_fields(self):
        for key,value in [('progression',None),('operating_level_cap',False),('catalog_max_level',0),('new_overflow_policy',None),('rebirth_enabled',0)]:
            tables=deepcopy(self.tables);p=tables['dev_first_loop_profile'][0]['parameters']
            if key=='progression':p[key]=value
            else:p['progression'][key]=value
            with self.subTest(key=key):self.assertTrue(config.check_json_contracts(self.schema,tables))
        tables=deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['progression']['extra']=0
        self.assertTrue(config.check_json_contracts(self.schema,tables))

    def test_public_has_one_continuous_experience_catalog_and_no_source_or_second_array(self):
        public=module.public_gameplay(self.fixture);r=public['progression']
        self.assertEqual(set(r),set(self.fixture['progression'])-{'exp_per_level'})
        self.assertEqual(public['experience'],[{'level':row['level'],'next_level_exp':row['next_level_exp']} for row in self.tables['game_exp_levels']])
        self.assertNotIn('exp_per_level',r);self.assertNotIn('source_ref',r);self.assertNotIn('balance_verified',r)
        public['progression']['curve_version']='changed'
        self.assertEqual(self.fixture['progression']['curve_version'],self.profile['progression']['curve_version'])

    def test_legacy_version_cannot_mix_new_fields_and_keeps_historical_catalog(self):
        profile=deepcopy(self.profile);profile['rule_version']=19
        with self.assertRaisesRegex(ValueError,'旧版本'):module.progression_rules(self.tables,profile)
        del profile['progression'];tables=deepcopy(self.tables);tables['game_exp_levels']=tables['game_exp_levels'][:20]
        legacy=module.progression_rules(tables,profile)
        self.assertEqual(set(legacy),{'exp_per_level'});self.assertEqual(len(legacy['exp_per_level']),20)


if __name__=='__main__':unittest.main()
