"""药品开发投影：唯一配置传递、客户端裁剪与历史提取保护。"""

from copy import deepcopy
import importlib.util
import json
from pathlib import Path
import sys
import unittest
from unittest.mock import patch


ROOT=Path(__file__).resolve().parents[2]
SPEC=importlib.util.spec_from_file_location('prepare_first_loop',ROOT/'tools/resources/prepare_first_loop.py')
first_loop=importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(first_loop)


def source_row(id,**values):
    return {'id':id,'source_ref':'测试来源','note':'仅测试夹具',**values}


def candidate_tables():
    # 故意使用区别于当前Excel的参数，验证导出器没有补入第二套数值。
    players=[{'job_id':i,'magic_resist_weight':17} for i in range(3)]
    profile={'rule_version':9,'map_id':1,'monster_id':1,'pet_id':1,'players':players,
             'inventory_capacity':4,'initial_inventory':[35,36],'initial_bound_gold':0,
             'allow_free_dev_revive':True,'damage_formula':'测试结算路径',
             'recycle_test_gold_by_item':{'35':0,'36':0,'193':0,'218':0},
             'player_stats_source':'测试角色来源','technical_parameters_notice':'测试夹具说明',
             'damage_scope_notice':'测试结算边界','revive_notice':'测试复活边界',
             'potions':{'tick_ms':173,'base_per_tick':9,'levels_per_step':7,'use_delay_ms':347,
                        'queue_cap':1234,'effects':[{'item_id':35,'hp':17,'mp':0},{'item_id':36,'hp':0,'mp':23}]}}
    native=[]
    for i,kind in enumerate(['fencing','fireball','healing']):
        skill={field:1 for field in first_loop.SKILL_INT_FIELDS}
        skill.update(id=100+i,job_id=i,book_item_id=1000+i,name='测试'+kind,behavior=kind,
                     learned_level=0,max_level=3,learn_level=8,mp_base_cost=6,mp_level_cost=4,range_cells=2,
                     delay_base_ms=451,projectile_ms_per_cell=37)
        native.append(skill)
    profile['healing']={k:13 for k in first_loop.HEALING_FIELDS}
    profile['player_growth']=[{**{k:3.25 for k in first_loop.GROWTH_FIELDS},'job_id':i,'hp_base':11,'mp_base':12} for i in range(3)]
    profile['initial_skill_books']=[{'job_id':s['job_id'],'item_id':s['book_item_id']} for s in native]
    profile.update(skill_ids=[r['id'] for r in native],skill_economy={'page_item_id':900001,'preview_ttl_ms':12345,'preview_material_limit':7},
                   skill_shop_entries=[{'offer_id':301,'item_id':1000,'required_role_level':2,'max_purchase_quantity':17},
                                       {'offer_id':302,'item_id':900001,'required_role_level':1,'max_purchase_quantity':19}],
                   recyclable_book_ids=[r['book_item_id'] for r in native],recycle_source_kinds=['monster_drop'],skill_book_drop_quality=2)
    fighter=source_row(1,hp=18,attributes={'mp':14,'physical_attack_min':1,'physical_attack_max':2,'physical_defence_min':0,
                                        'physical_defence_max':3,'magic_defence':4,'accuracy':5,'agility':6},
                       attack_interval_ms=700,move_interval_ms=500,exp=3)
    tables={
        'dev_first_loop_profile':[source_row(1,parameters=profile)],
        'game_maps':[source_row(1,collision_id=1,width_cells=2,height_cells=2,spawn_cell={'x':0,'y':0})],
        'game_map_collisions':[source_row(1,blocked_cells=[])],
        'game_monsters':[fighter],
        'game_pet_templates':[source_row(1,monster_template_id=1)],
        'game_items':[source_row(id,display_name=name,item_type=kind,max_stack=None,effect_list_id=None)
                      for id,name,kind in [(35,'测试红药','potion'),(36,'测试蓝药','potion'),(193,'测试衣服','equipment'),(218,'测试剑','equipment')]],
        'game_equipment':[source_row(193,item_id=193,equipment_type='armour',base_attributes={},class_restriction=[0,1,2]),
                          source_row(218,item_id=218,equipment_type='weapon',base_attributes={},class_restriction=[0,1,2])],
        'game_drop_categories':[source_row(i,category_key=key,prob_ppm=value)
                                for i,(key,value) in enumerate([('empty',650000),('gold',200000),('item',100000),('equipment',50000)],1)],
        'game_drop_entries':[source_row(1,pool_id=1,item_id=218),source_row(2,pool_id=2,item_id=35),
                             source_row(5,pool_id=3,quantity_min=1,quantity_max=1)],
        'game_monster_spawns':[source_row(1,map_id=1,cell={'x':1,'y':1})],
        'game_exp_levels':[source_row(1,level=1,next_level_exp=12)],
    }
    tables['game_items'] += [source_row(r['book_item_id'],display_name=r['name'],item_type='skill_book',max_stack=1) for r in native]
    tables['game_items'].append(source_row(900001,display_name='测试书页',item_type='skill_page',max_stack=99))
    tables['game_skills']=[];tables['game_skill_upgrades']=[]
    for r in native:
        tables['game_skills'].append(source_row(r['id'],display_name=r['name'],class_id=r['job_id'],book_item_id=r['book_item_id'],
          learn_level=r['learn_level'],native_start_level=r['learned_level'],native_max_level=r['max_level'],behavior_id=r['behavior'],cooldown_ms=r['cooldown_base_ms'],
          target_rule={k:r[k] for k in ['range_cells','target_move_tolerance_cells']},
          cast_timing={k:r[k] for k in ['cooldown_reduction_ms','global_spell_ms','action_ms','delay_base_ms','projectile_ms_per_cell']},
          resource_costs={k:r[k] for k in ['mp_base_cost','mp_level_cost']},
          damage_parameters={k:r[k] for k in ['accuracy_per_level','power_base','power_bonus','mpower_base','mpower_bonus','multiplier_base_ppm','multiplier_bonus_ppm']}))
        for rank in range(1,r['max_level']+1):
            tables['game_skill_upgrades'].append(source_row(r['id']*10+rank,skill_id=r['id'],level=rank,book_cost=[{'item_id':r['book_item_id'],'quantity':rank+1}],
              page_cost=13+rank,page_item_id=900001,allowed_numeric_changes={'native_rank':rank},preserve_behavior_shape=True,required_role_level=8+rank))
    tables['game_shop_prices']=[source_row(301,payment_currency='gold',price_gold=71,price_u=None,quantity=1,purchased_bound=True,daily_limit=None),
                                source_row(302,payment_currency='bound_ingot',price_u=89,quantity=2,purchased_bound=True,daily_limit=None)]
    tables['game_book_recycle']=[source_row(1,book_quality=2,pages=7,npc_basic_book_pages=0)]
    return tables


def fixture(tables):
    with patch.object(first_loop,'read',return_value={'pet_spawn':{'x':1,'y':0}}):
        return first_loop.fixture_from_rows(tables)


class PotionProjectionTests(unittest.TestCase):
    def test_excel_parameters_are_preserved_and_detached(self):
        tables=candidate_tables();before=deepcopy(tables)
        result=fixture(tables)
        self.assertEqual(result['potions'],before['dev_first_loop_profile'][0]['parameters']['potions'])
        self.assertEqual(result['rule_version'],9)
        self.assertEqual(tables,before)
        tables['dev_first_loop_profile'][0]['parameters']['potions']['effects'][0]['hp']=999
        self.assertEqual(result['potions']['effects'][0]['hp'],17)
        result['potions']['effects'][1]['mp']=111
        self.assertEqual(tables['dev_first_loop_profile'][0]['parameters']['potions']['effects'][1]['mp'],23)
        self.assertTrue(all(row['max_stack'] is None and row['effect_list_id'] is None for row in tables['game_items'][:4]))

    def test_public_items_only_expose_approved_display_fields(self):
        tables=candidate_tables();settings=tables['dev_first_loop_profile'][0]['parameters']['potions']
        settings['_private_source']='不能下发的来源说明'
        settings['effects'][0]['owner_internal']='不能下发的owner字段'
        current=fixture(tables);public=first_loop.public_gameplay(current)
        exported={row['item_id']:row for row in public['items']}
        for id,hp,mp in [(35,17,0),(36,0,23)]:
            row=exported[id]
            self.assertTrue(row['use_enabled'])
            self.assertEqual(row['potion'],{field:settings[field] for field in first_loop.POTION_FIELDS}|{'hp':hp,'mp':mp})
        for id in [193,218]:
            self.assertFalse(exported[id]['use_enabled']);self.assertNotIn('potion',exported[id])
        encoded=json.dumps(public,ensure_ascii=False)
        for private in ['_private_source','owner_internal','不能下发','_source_evidence','record_sources','source_ref','recycle_gold']:
            self.assertNotIn(private,encoded)
        self.assertIn('_private_source',current['potions'])
        exported[35]['allowed_jobs'].append(99)
        self.assertNotIn(99,current['items'][0]['allowed_jobs'])

    def test_missing_or_invalid_parameters_are_rejected_without_defaults(self):
        baseline=candidate_tables();profile=baseline['dev_first_loop_profile'][0]['parameters']
        invalid=[]
        missing=deepcopy(profile);missing.pop('potions');invalid.append(missing)
        for field in first_loop.POTION_FIELDS:
            for bad in [None,0,-1,True,1.5,'200']:
                value=deepcopy(profile);value['potions'][field]=bad;invalid.append(value)
        for effects in [[],[{'item_id':35,'hp':0,'mp':0}],[{'item_id':35,'hp':-1,'mp':1}],
                        [{'item_id':35,'hp':True,'mp':0}],[{'item_id':193,'hp':1,'mp':0}],
                        [{'item_id':999,'hp':1,'mp':0}],[{'item_id':35,'hp':1,'mp':0}]*2]:
            value=deepcopy(profile);value['potions']['effects']=effects;invalid.append(value)
        for index,value in enumerate(invalid):
            with self.subTest(index=index):
                rows=deepcopy(baseline);rows['dev_first_loop_profile'][0]['parameters']=value
                with self.assertRaises(ValueError):fixture(rows)

    def test_evidence_retains_candidate_boundaries(self):
        result=fixture(candidate_tables());evidence=result['_source_evidence']['potion_effects']
        self.assertEqual(evidence['status'],'candidate')
        self.assertFalse(evidence['target_180_verified'])
        self.assertFalse(evidence['automatic_use_enabled'])
        self.assertEqual(evidence['reference_document'],'docs/药品与技能实施边界.md')
        self.assertIn('Ac20/Mac30',evidence['amount_source'])
        self.assertIn('死亡拒绝新使用',evidence['death_policy'])
        self.assertIn('仍消费',evidence['full_resource_use'])

    def test_skill_and_growth_projection_uses_only_excel_values(self):
        tables=candidate_tables();before=deepcopy(tables);result=fixture(tables)
        profile=before['dev_first_loop_profile'][0]['parameters']
        self.assertEqual(result['skills'],first_loop.skill_definitions(before,profile))
        self.assertNotIn('skills',profile)
        for key in ['healing','player_growth','initial_skill_books']:
            self.assertEqual(result[key],profile[key])
            self.assertIsNot(result[key],tables['dev_first_loop_profile'][0]['parameters'][key])
        public=first_loop.public_gameplay(result)
        row=public['skills'][1];source=result['skills'][1]
        self.assertEqual(row['impact_base_ms'],451);self.assertEqual(row['projectile_ms_per_cell'],37)
        self.assertEqual(row['base_cost'],6);self.assertEqual(row['level_cost'],4)
        self.assertEqual(row['initial_level'],0);self.assertFalse(row['target_180_verified'])
        book=next(x for x in public['items'] if x['item_id']==source['book_item_id'])
        self.assertEqual(book['kind'],'book');self.assertEqual(book['skill_id'],source['id'])
        self.assertFalse(book['use_enabled']);self.assertNotIn('potion',book)
        self.assertEqual(result['monsters'][0]['magic_defence_max'],4)
        self.assertEqual(result['monsters'][0]['magic_resist_weight'],17)
        self.assertNotIn('power_base',json.dumps(public))
        self.assertEqual(tables,before)

    def test_missing_duplicated_or_fractional_skill_contract_is_rejected(self):
        baseline=candidate_tables();bad=[]
        for field in ['skill_ids','healing','player_growth','initial_skill_books']:
            rows=deepcopy(baseline);rows['dev_first_loop_profile'][0]['parameters'].pop(field);bad.append(rows)
        for field,value in [('book_item_id',1000),('native_max_level',-1)]:
            rows=deepcopy(baseline);rows['game_skills'][1][field]=value;bad.append(rows)
        rows=deepcopy(baseline);rows['game_skills'][1]['resource_costs']['mp_base_cost']=0.5;bad.append(rows)
        rows=deepcopy(baseline);rows['game_skills'][1]['cast_timing']['delay_base_ms']=0;bad.append(rows)
        rows=deepcopy(baseline);rows['dev_first_loop_profile'][0]['parameters']['skills']=[];bad.append(rows)
        rows=deepcopy(baseline);rows['dev_first_loop_profile'][0]['parameters']['healing']['pet_base_per_tick']=0;bad.append(rows)
        rows=deepcopy(baseline);rows['dev_first_loop_profile'][0]['parameters']['player_growth'][0]['hp_gain']=float('nan');bad.append(rows)
        rows=deepcopy(baseline);rows['dev_first_loop_profile'][0]['parameters']['initial_skill_books'][0]['item_id']=9999;bad.append(rows)
        for rows in bad:
            with self.subTest(rows=rows),self.assertRaises(ValueError):fixture(rows)

    def test_skill_economy_preserves_unique_table_values_and_wallet_units(self):
        result=fixture(candidate_tables());public=first_loop.public_gameplay(result)
        self.assertEqual([r['book_costs'][0]['quantity'] for r in result['skill_upgrades'][:3]],[2,3,4])
        self.assertEqual([r['page_cost'] for r in result['skill_upgrades'][:3]],[14,15,16])
        self.assertEqual(result['skill_book_recycle'][0]['pages_per_book'],7)
        self.assertEqual(result['skill_shop'][0]['price_units'],71)
        self.assertEqual(result['skill_shop'][1]['price_units'],89)
        self.assertEqual(public['shop_offers'][0]['currency_kind'],'gold')
        self.assertEqual(public['shop_offers'][1]['currency_kind'],'bound_ingot')
        self.assertEqual(next(r for r in result['items'] if r['id']==1000)['drop_quality'],2)
        self.assertEqual(next(r for r in public['items'] if r['item_id']==900001)['kind'],'skill_page')

    def test_skill_upgrade_rejects_cost_drift_shape_changes_or_gaps(self):
        baseline=candidate_tables();bad=[]
        rows=deepcopy(baseline);rows['game_skill_upgrades'][0]['book_cost'][0]['quantity']=1;bad.append(rows)
        rows=deepcopy(baseline);rows['game_skill_upgrades'][0]['allowed_numeric_changes']['range_cells']=99;bad.append(rows)
        rows=deepcopy(baseline);rows['game_skill_upgrades'].pop(1);bad.append(rows)
        rows=deepcopy(baseline);rows['game_book_recycle'][0]['npc_basic_book_pages']=1;bad.append(rows)
        rows=deepcopy(baseline);rows['dev_first_loop_profile'][0]['parameters']['recycle_source_kinds']=['npc_shop'];bad.append(rows)
        for rows in bad:
            with self.subTest(rows=rows),self.assertRaises(ValueError):fixture(rows)

    def test_initialized_project_cannot_reextract_or_save(self):
        schema={'tables':[{'table_id':'dev_first_loop_profile'}]}
        for flags in [['--extract'],['--extract','--check']]:
            with self.subTest(flags=flags),patch.object(sys,'argv',['prepare_first_loop.py',*flags]),\
                 patch.object(first_loop.config,'load_json',return_value=schema),\
                 patch.object(first_loop,'extract') as extract,patch.object(first_loop,'save') as save:
                with self.assertRaisesRegex(ValueError,'开发表已初始化'):first_loop.main()
                extract.assert_not_called();save.assert_not_called()


if __name__=='__main__':unittest.main()
