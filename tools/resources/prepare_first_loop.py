#!/usr/bin/env python3
"""从唯一Excel导出显式开发夹具；保留已封存的首轮来源提取入口。"""
from __future__ import annotations
import argparse
from copy import deepcopy
import csv
import hashlib
import json
from pathlib import Path
import re
import sys

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'tools/config'))
import export_configs as config
WORK=ROOT/'.work/first-loop'
DEST=ROOT/'config/development/first-loop.json'
CSV_ROOT=ROOT/'资料参考/myf/Mud2/DB'
EXP=ROOT.parent/'MirServer沉默世界_MYF/Mir200/Exps.ini'
MON_DROPS=ROOT.parent/'MirServer沉默世界_MYF/Mir200/Envir/MonItems/稻草人.txt'
ASSETS=ROOT/'client/assets/resources/development'
NOTE='仅显式development配置使用的跨来源候选；未通过目标1.80行为/数值/真机验收，不进入正式发布。'
TABLE_IDS=['game_monsters','game_items','game_equipment','game_maps','game_map_collisions','game_map_regions',
           'game_monster_spawns','game_pet_templates','game_exp_levels','game_drop_pools','game_drop_entries',
           'game_skills','game_recycle_values','dev_first_loop_profile']

def save(path,value):path.parent.mkdir(parents=True,exist_ok=True);path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
def read(path):return json.loads(path.read_text(encoding='utf-8'))
def csv_rows(name):
    with (CSV_ROOT/name).open(encoding='utf-8-sig',newline='') as f:return [(i,row) for i,row in enumerate(csv.DictReader(f),2)]
def find(rows,name):return next((i,r) for i,r in rows if r.get('Name')==name)
def meta(id,source,note=NOTE):return {'id':id,'record_status':'candidate','source_ref':source,'note':note}
def schema_profile():
    """仅供历史首次初始化；当前已维护Excel，不能再次提取覆盖。"""
    fields=[{'name':'id','title':'配置编号','type':'int','required':True,'target':'none','minimum':1},
            {'name':'profile_key','title':'开发配置键','type':'string','required':True,'target':'none'},
            {'name':'parameters','title':'仅开发测试参数','type':'json','required':True,'target':'none'},
            {'name':'record_status','title':'参数状态','type':'string','required':True,'target':'none','enum_values':['confirmed','candidate','unverified','historical']},
            {'name':'source_ref','title':'来源与开发授权','type':'string','required':True,'target':'none'},
            {'name':'note','title':'说明','type':'string','required':False,'target':'none'}]
    return {'table_id':'dev_first_loop_profile','sheet_name':'开发首循环夹具','title':'开发首循环夹具','domain':'开发隔离',
            'source_sections':['用户2026-10-26授权首批正式开发；历史首次初始化结构；当前参数只维护唯一Excel，初始化后禁止--extract'],
            'release_required':False,'min_rows':1,'fields':fields,'workbook_group':'gameplay'}

def extract():
    """封存的首次来源提取，不是当前数值源；main在已初始化仓库拒绝此入口。"""
    monsters=csv_rows('ApexM2_Monster.csv');items=csv_rows('ApexM2_stditems.csv');mapping=read(ASSETS/'maps/first-map.json')
    tables={};sources=[]
    def add(t,row):tables.setdefault(t,[]).append(row)
    def source(path):
        relative=str(path.relative_to(ROOT)) if path.is_relative_to(ROOT) else str(path)
        sources.append({'path':relative,'sha256':config.sha256(path)})
    for p in [CSV_ROOT/'ApexM2_Monster.csv',CSV_ROOT/'ApexM2_stditems.csv',EXP,MON_DROPS,
              ROOT.parent/'SuprcodeCrystal/Shared/BaseStats.cs',ROOT.parent/'SuprcodeCrystal/Server/MirObjects/HumanObject.cs',
              ROOT.parent/'SuprcodeCrystal/Server/MirObjects/MapObject.cs',ASSETS/'manifest.json',ASSETS/'maps/first-map.json']:source(p)
    for id,name,key,category,resource in [(1,'稻草人','dev_scarecrow','ordinary','monster_scarecrow'),(2,'变异骷髅','dev_skeleton_pet','pet_template','pet_skeleton')]:
        line,r=find(monsters,name);ref=f'资料参考/myf/Mud2/DB/ApexM2_Monster.csv:{line}；client/assets/resources/development/manifest.json'
        attrs={k:int(r[v]) for k,v in [('mp','MP'),('physical_attack_min','DC'),('physical_attack_max','DCMAX'),('physical_defence_min','AC'),('physical_defence_max','AC'),('magic_defence','MAC'),('accuracy','HIT'),('agility','SPEED')]}
        add('game_monsters',{**meta(id,ref), 'monster_key':key,'display_name':name,'level':int(r['Lvl']),'category':category,
                            'hp':int(r['HP']),'attributes':attrs,'attack_interval_ms':int(r['ATTACK_SPD']),'move_interval_ms':int(r['WALK_SPD']),
                            'attack_behavior_id':'development_range_minus_defence','aggro_range_cells':None,'chase_range_cells':None,
                            'exp':int(r['Exp']),'drop_pool_id':1 if id==1 else None,'resource_key':resource})
    for name,item_type,slot in [('木剑','equipment',0),('布衣(男)','equipment',1),('金创药(小量)','potion',None),('魔法药(小量)','potion',None)]:
        line,r=find(items,name);id=int(r['Idx']);ref=f'资料参考/myf/Mud2/DB/ApexM2_stditems.csv:{line}'
        add('game_items',{**meta(id,ref,NOTE+' 原Looks/Shape仅作来源候选；未映射图标、最大堆叠和药品恢复时序仍留空。'),
                          'item_key':f'dev_source_item_{id}','display_name':'布衣' if id==193 else name,'item_type':item_type,'max_stack':1 if slot is not None else None,
                          'effect_list_id':None,'weight':int(r['Weight']),'quality':0 if slot is not None else None,'description':'来源子集开发候选',
                          'icon_resource_key':None})
        if slot is not None:
            attrs={'physical_attack_min':int(r['Dc']),'physical_attack_max':int(r['Dc2']),
                   'physical_defence_min':int(r['Ac']) if slot==1 else 0,'physical_defence_max':int(r['Ac2']) if slot==1 else 0,
                   'magic_defence_min':int(r['Mac']) if slot==1 else 0,'magic_defence_max':int(r['Mac2']) if slot==1 else 0}
            add('game_equipment',{**meta(id,ref,NOTE+' 品质0、阶段0是开发夹具对接本项目枚举的工程选择；不把来源Color当品质。'),
                'item_id':id,'equipment_type':'weapon' if slot==0 else 'armour','class_restriction':[0,1,2],'required_level':int(r['NeedLevel']),
                'quality':0,'stage':0,'base_attributes':attrs,'original_durability_u':int(r['DuraMax']),'weight':int(r['Weight']),
                'craft_bonus_strengthenable':False})
    for row in tables['game_items']+tables['game_equipment']:
        if row['id']==193:
            row['source_ref']='最终策划案.md:113-115、485-497（4.2、4.20）；资料参考/myf/Mud2/DB/ApexM2_stditems.csv:195'
            row['note']='来源原名布衣(男)。本作无性别穿戴门槛，不新增男女装备分类，同itemID按职业固定外观；允许职业[0,1,2]是目标规则适配。女法师衣服外观映射仍待正式资源验收。仅development候选，未通过目标1.80行为或数值验收。'
    map_ref='client/assets/resources/development/maps/first-map.json；传奇1.80客户端/Map/0.map裁剪原格'
    add('game_maps',{**meta(1,map_ref),'map_key':'dev_map0_crop128_128','display_name':'开发地图0局部','scene_kind':'ordinary',
                    'width_cells':16,'height_cells':16,'required_level':1,'collision_id':1,'spawn_cell':mapping['player_spawn'],
                    'tile_resource_key':'development/maps/first-map','map_version':mapping['source_sha256'][:16]})
    add('game_map_collisions',{**meta(1,map_ref),'map_key':'dev_map0_crop128_128','width_cells':16,'height_cells':16,
        'blocked_cells':mapping['blocked'],'walkable_encoding':'blocked_xy_objects','client_navigation':{'cell_width':48,'cell_height':32,'source_origin':mapping['source_origin']},
        'diagonal_rule':'development_no_corner_cutting','source_hash':mapping['source_sha256']})
    add('game_map_regions',{**meta(1,map_ref),'map_id':1,'region_key':'development_combat','region_cells':{'min_x':0,'min_y':0,'max_x':15,'max_y':15},
                           'npc_id':None,'monster_pool':[{'monster_id':1,'slots':8}],'entry_rules':{'development_only':True}})
    for i,p in enumerate(mapping['monster_spawns'],1):
        add('game_monster_spawns',{**meta(i,map_ref+'；最终策划案.md:普通怪10秒复活'),'map_id':1,'slot_index':i-1,'monster_id':1,'cell':p,
             'respawn_seconds':10,'spawn_kind':'ordinary','slot_resize_policy':'development_fixed_eight_reachable_slots'})
    pet=next(x for x in tables['game_monsters'] if x['id']==2)
    add('game_pet_templates',{**meta(1,pet['source_ref'],NOTE+' 只给道士开发夹具注入宠物，召唤资格/技能耗材未实现，不开放正式召唤。'),
        'pet_key':'dev_skeleton','summon_skill_id':None,'monster_template_id':2,'base_attributes':{'hp':pet['hp'],**pet['attributes']},
        'attack_behavior':'development_melee_adjacent','follow_and_return':None,'lifetime_and_betrayal':None,'resource_key':'pet_skeleton'})
    exp_lines=EXP.read_text(encoding='utf-8-sig').splitlines();exp_values=[]
    for no,line in enumerate(exp_lines,1):
        if no>1 and line.startswith('['):break
        match=re.fullmatch(r'Level(\d+)=(\d+)',line)
        if match and 1<=int(match[1])<=20:
            level,amount=map(int,match.groups());exp_values.append((level,amount))
            add('game_exp_levels',{**meta(level,f'../MirServer沉默世界_MYF/Mir200/Exps.ini:{no}'),
                 'level':level,'next_level_exp':amount,'growth_target':'development_candidate_only','measurement_version':'myf_source_unbalanced'})
    assert len(exp_values)==20
    for id,key,category in [(1,'dev_equipment_subset','equipment'),(2,'dev_potion_subset','item'),(3,'dev_gold_subset','gold')]:
        add('game_drop_pools',{**meta(id,'最终策划案.md:602-614；../MirServer沉默世界_MYF/Mir200/Envir/MonItems/稻草人.txt',NOTE+' 仅采纳来源条目与金币数量，旧独立掉率废弃；类别后子集等权是开发选择。'),
            'pool_key':key,'source_kind':'ordinary_monster','category_key':category,'selection_mode':'development_equal_subset',
            'equipment_stage_range':[0,0] if category=='equipment' else None,'quality_weights':{'0':1} if category=='equipment' else None,'max_random_quality':0 if category=='equipment' else None})
    for id,pool,item in [(1,1,218),(2,1,193),(3,2,35),(4,2,36)]:
        add('game_drop_entries',{**meta(id,'../MirServer沉默世界_MYF/Mir200/Envir/MonItems/稻草人.txt',NOTE+' 子集权重1是开发候选，不沿用来源独立概率。'),
            'pool_id':pool,'item_id':item,'currency_key':None,'weight':1,'quantity_min':1,'quantity_max':1,'binding_policy':'bound_development_only'})
    add('game_drop_entries',{**meta(5,'../MirServer沉默世界_MYF/Mir200/Envir/MonItems/稻草人.txt:6'),
        'pool_id':3,'item_id':None,'currency_key':'bound_gold','weight':1,'quantity_min':30,'quantity_max':30,'binding_policy':'bound_development_only'})
    add('game_skills',{**meta(0,'../SuprcodeCrystal/Server/MirObjects/HumanObject.cs:2974-2981；../SuprcodeCrystal/Server/MirObjects/MapObject.cs:243-259',NOTE+' 普攻0为本项目开发消息标识，非来源技能ID；三职业共用近身普攻，职业专属施法仍禁用。'),
        'skill_key':'development_basic_attack','display_name':'开发普攻','class_id':None,'learn_level':1,'behavior_id':'development_range_minus_defence',
        'target_rule':{'enemy_only':True,'adjacent_eight_directions':True,'class_ids':[0,1,2]},'range_pattern_id':None,'cooldown_ms':None,
        'cast_timing':{'animation':'attack','logic':'server_attack_interval'},'resource_costs':{'mp':0,'items':[]},
        'damage_or_heal_path':'development_range_minus_defence','effect_ids':[],'action_resource_key':'player_male.attack',
        'reference_evidence_id':'dev_physical_basic_v1','target_180_verified':False})
    add('game_recycle_values',{**meta(1,'最终策划案.md:125；用户2026-10-26开发闭环授权',NOTE+' 正式金币分布未知仍空；开发测试仅在独立夹具授权引用来源售价，不能冒充本表已核验回收价。'),
        'equipment_stage':0,'quality':0,'gold_distribution':None,'broken_gold':0,'stone_probability_ppm':None,'broken_byproducts':None,
        'oil_probability_formula':None,'rewards_combinable':None})
    players=[{'job_id':0,'definition_id':0,'hp':18,'mp':14,'attack_min':0,'attack_max':0,'defence':0,'attack_ms':1386,'move_ms':600},
             {'job_id':1,'definition_id':1,'hp':15,'mp':17,'attack_min':0,'attack_max':0,'defence':0,'attack_ms':1386,'move_ms':600},
             {'job_id':2,'definition_id':2,'hp':16,'mp':13,'attack_min':0,'attack_max':0,'defence':0,'attack_ms':1386,'move_ms':600}]
    # 以下仅保留最初rule_version=1的提取记录；当前药品等参数不得在此另设默认值。
    # 已初始化仓库的--extract会被拒绝，后续只读Excel生成最新开发夹具。
    profile={'rule_version':1,'map_id':1,'monster_id':1,'pet_id':1,'players':players,'inventory_capacity':32,
             'initial_inventory':[218,218,193,35,36],'initial_bound_gold':0,'allow_free_dev_revive':True,
             'damage_formula':'development_range_minus_defence','recycle_test_gold_by_item':{'218':50,'193':400,'35':0,'36':0},
             'technical_parameters_notice':'32格/赠送清单/免费主动复活/等权子集是本次开发测试夹具。回收50/400仅借来源Price作绑定金币事务测试，不是正式分解值。',
             'player_stats_source':'../SuprcodeCrystal/Shared/BaseStats.cs:14-68,200-239；level=1逐项Calculate；空手0..0；HumanObject.cs:186,1766-1768 attack=1400-14=1386,move=600',
             'damage_scope_notice':'仅验证无幸运/命中/元素/Buff的源物理区间减固定防御子集。不是完整1.80伤害管线。',
             'revive_notice':'开发测试行为，不是已核验正式死亡/复活规则；不收费、不掉经验、不掉装；健康重连不回血，主动操作幂等恢复。'}
    add('dev_first_loop_profile',{**meta(1,'用户2026-10-26授权1-5开发；'+profile['player_stats_source'],NOTE),
        'profile_key':'first_loop_closed_development','parameters':profile})
    patch={'tables':tables,'new_table':schema_profile(),'source_locks':sources,'source_status':'candidate','development_only':True}
    save(WORK/'config-candidates.json',patch)
    return patch

POTION_FIELDS=('tick_ms','base_per_tick','levels_per_step','use_delay_ms','queue_cap')

def potion_rules(profile,items):
    """只校验并复制Excel内的药品参数，不填缺省数值或猜测正式堆叠规则。"""
    settings=profile.get('potions')
    if not isinstance(settings,dict):raise ValueError('开发夹具缺少potions；请维护唯一Excel后重新导出')
    for field in POTION_FIELDS:
        value=settings.get(field)
        if type(value) is not int or value<=0:raise ValueError('药品参数须为正整数：'+field)
    effects=settings.get('effects')
    if not isinstance(effects,list) or not effects:raise ValueError('药品effects必须是非空列表')
    known={item['id']:item for item in items};seen=set()
    for effect in effects:
        if not isinstance(effect,dict):raise ValueError('药品effect必须是对象')
        item_id=effect.get('item_id');hp=effect.get('hp');mp=effect.get('mp')
        if type(item_id) is not int or item_id in seen or item_id not in known or known[item_id]['is_equipment']:
            raise ValueError('药品效果须引用导出夹具中的唯一非装备物品')
        if type(hp) is not int or type(mp) is not int or hp<0 or mp<0 or hp+mp==0:
            raise ValueError('药品hp/mp须为非负整数且至少一项有效')
        seen.add(item_id)
    return deepcopy(settings)

SKILL_INT_FIELDS='id job_id book_item_id learn_level learned_level mp_base_cost mp_level_cost cooldown_base_ms cooldown_reduction_ms global_spell_ms action_ms range_cells delay_base_ms projectile_ms_per_cell target_move_tolerance_cells accuracy_per_level power_base power_bonus mpower_base mpower_bonus multiplier_base_ppm multiplier_bonus_ppm'.split()
HEALING_FIELDS='tick_ms base_per_tick pet_base_per_tick levels_per_step queue_fraction_divisor queue_cap'.split()
GROWTH_FIELDS='hp_gain hp_gain_rate mp_gain mp_gain_rate attack_divisor defence_max_divisor magic_divisor tao_divisor magic_defence_min_divisor magic_defence_max_divisor'.split()

def validate_skills(profile):
    """技能参数必须来自当前Excel，不按技能名或来源表临时填运行数值。"""
    import math
    skills=profile.get('skills')
    if not isinstance(skills,list) or not skills:raise ValueError('开发夹具缺少技能定义')
    ids=set();books=set();behaviors=set()
    for skill in skills:
        if not isinstance(skill,dict) or set(skill)!=set(SKILL_INT_FIELDS)|{'name','behavior'}:raise ValueError('技能字段不完整')
        if any(type(skill[k]) is not int or skill[k]<0 or skill[k]>2**32-1 for k in SKILL_INT_FIELDS):raise ValueError('技能字段须为有界非负整数')
        if not isinstance(skill['name'],str) or not skill['name'] or skill['behavior'] not in ['fencing','fireball','healing']:raise ValueError('技能名称/行为不合法')
        if not skill['id'] or skill['id'] in ids or not skill['book_item_id'] or skill['book_item_id'] in books or skill['job_id']>2 or not skill['learn_level']:
            raise ValueError('技能身份、职业、学习条件或技能书重复/缺失')
        if skill['behavior']!='fencing' and any(skill[k]<=0 for k in ['cooldown_base_ms','global_spell_ms','action_ms','range_cells','delay_base_ms']):raise ValueError('主动技能时序与射程必须完整')
        ids.add(skill['id']);books.add(skill['book_item_id']);behaviors.add(skill['behavior'])
    if behaviors!={'fencing','fireball','healing'}:raise ValueError('首组三职业候选技能不完整')
    healing=profile.get('healing')
    if not isinstance(healing,dict) or set(healing)!=set(HEALING_FIELDS) or any(type(healing[k]) is not int or healing[k]<=0 for k in HEALING_FIELDS):raise ValueError('治疗队列参数不完整')
    players=profile.get('players');growth=profile.get('player_growth');initial=profile.get('initial_skill_books')
    if not isinstance(players,list) or {p['job_id'] for p in players}!={0,1,2}:raise ValueError('三职业基础属性缺失')
    if not isinstance(growth,list) or len(growth)!=3 or {g['job_id'] for g in growth}!={0,1,2}:raise ValueError('三职业成长缺失')
    for g in growth:
        if set(g)!=set(GROWTH_FIELDS)|{'job_id','hp_base','mp_base'}:raise ValueError('职业成长字段不完整')
        if any(type(g[k]) not in [int,float] or not math.isfinite(g[k]) or g[k]<0 for k in GROWTH_FIELDS):raise ValueError('职业成长系数不合法')
        if any(type(g[k]) is not int or g[k]<=0 for k in ['hp_base','mp_base']) or g['hp_gain']<=0 or g['mp_gain']<=0:raise ValueError('职业成长基数/除数不合法')
    expected={(s['job_id'],s['book_item_id']) for s in skills}
    if not isinstance(initial,list) or len(initial)!=len(expected) or {(r['job_id'],r['item_id']) for r in initial}!=expected:raise ValueError('初始书必须逐项引用当前职业技能书')

def fixture_from_rows(tables):
    one=lambda table,id:next(r for r in tables[table] if r['id']==id)
    profile=one('dev_first_loop_profile',1)['parameters'];m=one('game_maps',1);collision=one('game_map_collisions',m['collision_id'])
    geo=read(ASSETS/'maps/first-map.json');monster=one('game_monsters',profile['monster_id']);pet_def=one('game_pet_templates',profile['pet_id']);pet=one('game_monsters',pet_def['monster_template_id'])
    validate_skills(profile)
    skills=deepcopy(profile['skills']);healing=deepcopy(profile['healing']);growth=deepcopy(profile['player_growth'])
    resist_weights={row['magic_resist_weight'] for row in profile['players']}
    if len(resist_weights)!=1:raise ValueError('当前开发子集使用同一有来源的魔法闪避权重')
    resist_weight=next(iter(resist_weights))
    fighter=lambda r:{'job_id':2 if r['id']==2 else 0,'definition_id':r['id'],'hp':r['hp'],'mp':r['attributes']['mp'],
        'attack_min':r['attributes']['physical_attack_min'],'attack_max':r['attributes']['physical_attack_max'],
        'defence':r['attributes']['physical_defence_min'],'defence_min':r['attributes']['physical_defence_min'],
        'defence_max':r['attributes']['physical_defence_max'],'magic_defence_min':r['attributes']['magic_defence'],
        'magic_defence_max':r['attributes']['magic_defence'],'magic_min':0,'magic_max':0,'tao_min':0,'tao_max':0,
        'accuracy':r['attributes']['accuracy'],'agility':r['attributes']['agility'],
        'magic_resist':0,'magic_resist_weight':resist_weight,
        'attack_ms':r['attack_interval_ms'],'move_ms':r['move_interval_ms']}
    items=[]
    for r in tables['game_items']:
        if r['id'] not in [35,36,193,218]:continue
        eq=next((x for x in tables['game_equipment'] if x['item_id']==r['id']),None);attrs=eq['base_attributes'] if eq else {}
        items.append({'id':r['id'],'name':r['display_name'],'equipment_slot':0 if eq and eq['equipment_type']=='weapon' else 1 if eq else 0,
                      'is_equipment':eq is not None,'attack_bonus':0,'attack_min_bonus':attrs.get('physical_attack_min',0),
                      'attack_max_bonus':attrs.get('physical_attack_max',0),'defence_bonus':attrs.get('physical_defence_min',0),
                      'defence_min_bonus':attrs.get('physical_defence_min',0),'defence_max_bonus':attrs.get('physical_defence_max',0),
                      'magic_defence_min_bonus':attrs.get('magic_defence_min',0),'magic_defence_max_bonus':attrs.get('magic_defence_max',0),
                      'recycle_gold':profile['recycle_test_gold_by_item'][str(r['id'])],'stackable':eq is None,
                      'allowed_jobs':eq['class_restriction'] if eq else [0,1,2]})
    for skill in skills:
        if any(item['id']==skill['book_item_id'] for item in items):raise ValueError('技能书ID与已有物品冲突')
        items.append({'id':skill['book_item_id'],'name':'《'+skill['name']+'》','equipment_slot':0,'is_equipment':False,
                      'book_skill_id':skill['id'],'attack_bonus':0,'attack_min_bonus':0,'attack_max_bonus':0,'defence_bonus':0,
                      'defence_min_bonus':0,'defence_max_bonus':0,'magic_defence_min_bonus':0,'magic_defence_max_bonus':0,
                      'recycle_gold':0,'stackable':False,'allowed_jobs':[skill['job_id']]})
    categories={r['category_key']:r['prob_ppm'] for r in tables['game_drop_categories']}
    drop_gold=one('game_drop_entries',5)
    potions=potion_rules(profile,items)
    result={'schema_version':1,'rule_version':profile['rule_version'],'development_only':True,'runtime_ready':False,'source_status':'candidate',
        'damage_formula':profile['damage_formula'],'allow_free_dev_revive':profile['allow_free_dev_revive'],
        'potions':potions,'skills':skills,'healing':healing,'player_growth':growth,'initial_skill_books':deepcopy(profile['initial_skill_books']),
        'map':{'id':m['id'],'width':m['width_cells'],'height':m['height_cells'],'blocked':collision['blocked_cells'],
               'player_spawn':m['spawn_cell'],'pet_spawn':geo['pet_spawn'],'monster_spawns':[r['cell'] for r in tables['game_monster_spawns'] if r['map_id']==m['id']]},
        'players':profile['players'],'pet':fighter(pet),'monsters':[{**fighter(monster),'id':monster['id'],'exp':monster['exp'],
                    'gold_min':drop_gold['quantity_min'],'gold_max':drop_gold['quantity_max'],'respawn_ms':10000}],
        'items':items,'drops':{'equipment_item_ids':[r['item_id'] for r in tables['game_drop_entries'] if r['pool_id']==1],
                             'item_ids':[r['item_id'] for r in tables['game_drop_entries'] if r['pool_id']==2],
                             **{key+'_ppm':categories[key] for key in ['empty','gold','item','equipment']},'equipment_guarantee':20},
        'progression':{'exp_per_level':[r['next_level_exp'] for r in sorted(tables['game_exp_levels'],key=lambda r:r['level'])]},
        'initial_inventory':profile['initial_inventory'],'initial_bound_gold':profile['initial_bound_gold'],'inventory_capacity':profile['inventory_capacity'],
        '_source_evidence':{'status':'candidate','official_180_restoration_verified':False,
                            'record_sources':{tid:[{'id':r['id'],'source_ref':r['source_ref'],'note':r['note']} for r in rows] for tid,rows in tables.items()},
                            'player_stats':profile['player_stats_source'],'technical_parameters':profile['technical_parameters_notice'],
                            'damage_scope':profile['damage_scope_notice'],'revive':profile['revive_notice'],
                            'potion_effects':{
                                'status':'candidate','target_180_verified':False,
                                'parameters_source':'唯一玩法Excel/开发首循环夹具/parameters.potions；本工具不补默认数值',
                                'reference_document':'docs/药品与技能实施边界.md',
                                'amount_source':'资料参考/myf/Mud2/DB/ApexM2_stditems.csv:37-38记录Ac20/Mac30；未取得MYF字段语义实现，仅作跨来源开发恢复测试量，不是已核验原服药效。',
                                'recovery_source':'资料参考/crystal/Server/MirObjects/HumanObject.cs:186,550-648；PlayerObject.cs:5820-5837,6330-6337；普通药分HP/MP累计队列，满值仍扣并清对应剩余，死亡暂停保留。',
                                'use_delay_adaptation':'../SuprcodeCrystal/Client/MirControls/MirItemCell.cs:541-543,625仅有客户端共享300ms节流；本作提升为服务端共享节流属于明确development适配。',
                                'full_resource_use':'仍消费，客户端提交前明示并确认；恢复满值后清除对应剩余队列',
                                'death_policy':'死亡拒绝新使用，已提交剩余效果暂停保留，不因重新连接重发',
                                'automatic_use_enabled':False,
                                'formal_scope':'正式game_items堆叠上限及effect_list_id仍待核验；未开放完整正式效果表。'},
                            'skills':{'status':'candidate','target_180_verified':False,
                                'reference_document':'docs/技能来源核验与实施清单.md',
                                'parameters_source':'唯一玩法Excel/开发首循环夹具；技能、职业成长、独立治疗时钟及初始书清单',
                                'scope':'仅基本剑术被动、火球单体延迟命中、治愈自身/自有宠物；技能升级/书页/其余完整技能仍关闭',
                                'learning_adaptation':'7级学习资格、新角色按职业1本绑定书属于development候选；原生初学等级0，未学由列表缺项表示',
                                'attribute_scope':'三职默认成长与DC/MC/SC/AC/MAC、准确敏捷；不含元素/幸运/完整Buff，来源默认不是源服实库',
                                'monster_resist_scope':'本轮开发怪物/宠物不配置魔法闪避加成（0）；权重从唯一Excel中共同的源码默认候选权重取得',
                                'formal_tables_populated':False}}}
    return result

def public_gameplay(fixture):
    """客户端只接收显式展示白名单，来源、夹具说明和服务端内部字段留在源侧。"""
    potions=fixture['potions'];effects={effect['item_id']:effect for effect in potions['effects']}
    shared={field:potions[field] for field in POTION_FIELDS}
    items=[]
    for row in fixture['items']:
        effect=effects.get(row['id']) if not row['is_equipment'] else None
        item={'item_id':row['id'],'name':row['name'],'kind':'equipment' if row['is_equipment'] else 'book' if row.get('book_skill_id') else 'potion',
              **({'equip_slot':row['equipment_slot']} if row['is_equipment'] else {}),
              'allowed_jobs':deepcopy(row['allowed_jobs']),'use_enabled':effect is not None}
        if effect is not None:item['potion']={**shared,'hp':effect['hp'],'mp':effect['mp']}
        if row.get('book_skill_id'):item['skill_id']=row['book_skill_id']
        items.append(item)
    skills=[{'id':s['id'],'name':s['name'],'job_id':s['job_id'],'book_item_id':s['book_item_id'],
             'learn_level':s['learn_level'],'initial_level':s['learned_level'],'kind':s['behavior'],'active':s['behavior']!='fencing',
             'range':s['range_cells'],'base_cost':s['mp_base_cost'],'level_cost':s['mp_level_cost'],
             'cast_ms':s['action_ms'],'delay_base_ms':s['cooldown_base_ms'],'delay_reduction_ms':s['cooldown_reduction_ms'],
             'impact_base_ms':s['delay_base_ms'],'projectile_ms_per_cell':s['projectile_ms_per_cell'],
             'accuracy_per_level':s['accuracy_per_level'],
             'source_status':'candidate','target_180_verified':False} for s in fixture['skills']]
    return {'schema_version':1,'rule_version':fixture['rule_version'],'development_only':True,'runtime_ready':False,'status':'candidate',
            'skills':skills,
            'items':items,
            'jobs':[{'job_id':0,'name':'战士','profile':'player_male'},{'job_id':1,'name':'法师','profile':'player_female'},{'job_id':2,'name':'道士','profile':'player_male'}],
            'experience':[{'level':i+1,'next_level_exp':n} for i,n in enumerate(fixture['progression']['exp_per_level'])],
            'monster_profiles':[{'definition_id':1,'name':'稻草人','profile':'monster_scarecrow'}],
            'pet_profiles':[{'definition_id':2,'name':'变异骷髅','profile':'pet_skeleton','job_id':2}]}

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--extract',action='store_true',help='历史首次初始化；已建立开发表后禁止重跑，不能覆盖唯一Excel');p.add_argument('--check',action='store_true');args=p.parse_args()
    if args.extract:
        if any(t['table_id']=='dev_first_loop_profile' for t in config.load_json(ROOT/'config/schema/catalog.json')['tables']):
            raise ValueError('开发表已初始化；后续只编辑唯一Excel，再运行本工具导出，不用来源提取覆盖已维护数据。')
        patch=extract();fixture=fixture_from_rows(patch['tables']|{'game_drop_categories':config.collect(config.load_json(ROOT/'config/schema/catalog.json'))[0]['game_drop_categories']})
        fixture['_source_evidence']['workbook_sync']='pending_initial_extraction'
    else:
        schema=config.load_json(ROOT/'config/schema/catalog.json');tables,errors,_,_=config.collect(schema)
        if errors:raise ValueError(errors)
        selected={tid:tables[tid] for tid in TABLE_IDS}
        selected['game_drop_categories']=tables['game_drop_categories'];fixture=fixture_from_rows(selected)
        fixture['_source_evidence']['workbook_sync']='exported_from_current_unique_excel'
        fixture['_source_evidence']['source_locks']=schema['source_locks']
        book=ROOT/next(w['path'] for w in schema['workbooks'] if w['group']=='gameplay')
        fixture['_source_evidence']['workbook']={'path':str(book.relative_to(ROOT)),'sha256':config.sha256(book)}
        fixture['_source_evidence']['exporter_sha256']=config.sha256(Path(__file__))
    if args.check:
        if read(DEST)!=fixture:raise ValueError('development fixture outdated; export it from Excel')
    else:save(DEST,fixture)
    gameplay=public_gameplay(fixture)
    public_path=ASSETS/'gameplay.json'
    if args.check:
        if read(public_path)!=gameplay:raise ValueError('public development gameplay metadata outdated')
    else:save(public_path,gameplay)
    print(json.dumps({'status':'checked' if args.check else 'generated','path':str(DEST),'sha256':config.sha256(DEST),'development_only':True,'runtime_ready':False},ensure_ascii=False))
if __name__=='__main__':raise SystemExit(main())
