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
           'game_skills','game_buffs','game_effects','game_skill_upgrades','game_shop_prices','game_book_recycle','game_recycle_values','dev_first_loop_profile']

def save(path,value):path.parent.mkdir(parents=True,exist_ok=True);path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
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

SKILL_TARGET_FIELDS='range_cells target_move_tolerance_cells area_radius require_line_of_sight auto_priority auto_min_targets auto_enabled secondary_direction_offsets secondary_distance_cells line_cells line_stop_on_blocked activation_kind'.split()
SKILL_CAST_FIELDS='cooldown_reduction_ms global_spell_ms action_ms delay_base_ms projectile_ms_per_cell secondary_impact_ms charge_window_ms line_step_ms'.split()
SKILL_DAMAGE_FIELDS='accuracy_per_level power_base power_bonus mpower_base mpower_bonus multiplier_base_ppm multiplier_bonus_ppm accuracy_by_level healing_stat_multiplier add_caster_level undead_multiplier_ppm max_attack_bonus_by_level ready_proc_denominator ready_proc_numerators non_undead_divisor'.split()
SKILL_INT_FIELDS='id job_id book_item_id learn_level learned_level max_level mp_base_cost mp_level_cost cooldown_base_ms cooldown_reduction_ms global_spell_ms action_ms range_cells delay_base_ms projectile_ms_per_cell target_move_tolerance_cells accuracy_per_level power_base power_bonus mpower_base mpower_bonus multiplier_base_ppm multiplier_bonus_ppm area_radius healing_stat_multiplier undead_multiplier_ppm auto_priority auto_min_targets secondary_distance_cells secondary_impact_ms charge_window_ms line_cells line_step_ms non_undead_divisor ready_proc_denominator buff_id material_item_id material_quantity'.split()
SKILL_BOOL_FIELDS=['require_line_of_sight','add_caster_level','line_stop_on_blocked','auto_enabled']
SKILL_ARRAY_FIELDS=['accuracy_by_level','max_attack_bonus_by_level','ready_proc_numerators','secondary_direction_offsets']
SKILL_BEHAVIORS=['fencing','spirit_sword','fireball','great_fireball','thunderbolt','firebang','healing','mass_healing','slaying','thrusting','half_moon','flaming_sword','hell_fire','lightning','ice_storm','thunder_storm','magic_shield','soul_shield','blessed_armour']
SKILL_TARGETS=['passive','enemy_entity','enemy_grid','friendly_entity','friendly_grid','attack_mode','attack_charge','enemy_direction','enemy_self','friendly_self']
BUFF_BEHAVIORS={'magic_shield':'magic_shield','soul_shield':'max_magic_defence','blessed_armour':'max_physical_defence'}
BUFF_ATTRIBUTE_FIELDS='kind value_by_level target_level_divisor target_level_add hit_duration_loss_ms_per_hp'.split()
BUFF_DURATION_FIELDS='kind stat_add stat_multiplier seconds_by_level'.split()
COMBAT_TIMING_FIELDS='player_attack_base_ms player_attack_speed_step_ms player_attack_level_step_ms player_attack_level_cap_ms player_attack_min_ms player_attack_action_ms player_attack_impact_ms event_capacity kill_queue_capacity player_push_action_ms monster_push_action_ms_per_step monster_push_move_ms_per_step pushed_step_display_ms'.split()
HEALING_FIELDS='tick_ms base_per_tick pet_base_per_tick levels_per_step queue_fraction_divisor queue_cap'.split()
GROWTH_FIELDS='hp_gain hp_gain_rate mp_gain mp_gain_rate attack_divisor defence_max_divisor magic_divisor tao_divisor magic_defence_min_divisor magic_defence_max_divisor'.split()

def skill_definitions(tables,profile):
    """正式技能表是唯一技能数值源；开发夹具仅选择ID，不保留旧参数副本。"""
    if 'skills' in profile:raise ValueError('技能参数已迁入技能基础定义，请删除开发夹具中的旧skills副本')
    ids=profile.get('skill_ids')
    if not isinstance(ids,list) or not ids or len(set(ids))!=len(ids):raise ValueError('开发技能ID选择缺失或重复')
    result=[]
    for id in ids:
        try:
            row=next(r for r in tables['game_skills'] if r['id']==id)
            if row['resource_costs'].get('items')!=[]:raise ValueError('旧items耗材入口必须显式为空，永久材料只由有类型字段引用')
            refs=row['effect_ids']
            if not isinstance(refs,list) or any(type(v) is not int or v<=0 for v in refs) or len(set(refs))!=len(refs):raise ValueError('技能效果引用须为唯一正整数列表')
            is_buff=row['behavior_id'] in BUFF_BEHAVIORS
            if len(refs)!=(1 if is_buff else 0):raise ValueError('状态技能须有且仅有一个效果引用，旧技能不得挂隐形状态')
            buff_id=0
            if refs:
                effect=next(r for r in tables['game_effects'] if r['id']==refs[0])
                if effect['effect_type']!='defensive_buff' or effect['trigger_phase']!='cast_impact' or effect['target_rule']!={'kind':row['target_rule']['kind']} or effect['proc_limit']!={'per_cast':1} or type(effect['proc_limit']['per_cast']) is not int or effect['recursive_trigger_allowed'] is not False:raise ValueError('状态效果触发管线、目标或递归门禁不一致')
                if not isinstance(effect['parameters'],dict) or set(effect['parameters'])!={'buff_id'}:raise ValueError('状态效果必须唯一引用game_buffs，不内嵌另一份数值')
                buff_id=effect['parameters']['buff_id']
                if type(buff_id) is not int or buff_id<=0:raise ValueError('状态引用缺失或无效')
            result.append({'id':id,'name':row['display_name'],'job_id':row['class_id'],'book_item_id':row['book_item_id'],
                'learn_level':row['learn_level'],'learned_level':row['native_start_level'],'max_level':row['native_max_level'],
                'behavior':row['behavior_id'],'cooldown_base_ms':row['cooldown_ms'],
                'target_kind':row['target_rule']['kind'],
                **{k:row['target_rule'][k] for k in SKILL_TARGET_FIELDS},
                **{k:row['cast_timing'][k] for k in SKILL_CAST_FIELDS},
                'buff_id':buff_id,
                **{k:row['resource_costs'][k] for k in ['mp_base_cost','mp_level_cost','material_item_id','material_quantity']},
                **{k:row['damage_parameters'][k] for k in SKILL_DAMAGE_FIELDS}})
        except (KeyError,StopIteration) as error:raise ValueError('技能基础定义缺少实际执行字段：'+str(id)) from error
    return result

def casting_supply_rules(profile,tables):
    rules=profile.get('casting_supply')
    if not isinstance(rules,dict) or set(rules)!={'enabled','slot','item_ids'} or type(rules['enabled']) is not bool or type(rules['slot']) is not int or rules['slot']!=100:raise ValueError('施法材料开关、独立槽与白名单必须显式来自Excel')
    ids=rules['item_ids']
    if not isinstance(ids,list) or not ids or any(type(v) is not int or v<=0 for v in ids) or len(set(ids))!=len(ids):raise ValueError('施法材料白名单缺失或重复')
    for id in ids:
        row=next((r for r in tables['game_items'] if r['id']==id),None)
        if row is None or row['item_type']!='casting_supply':raise ValueError('独立材料槽只能引用施法材料物品')
        if row['max_stack'] is not None and (type(row['max_stack']) is not int or not 0<row['max_stack']<=2**31-1):raise ValueError('已填写的护符数量上限无效')
        if rules['enabled'] and (type(row['max_stack']) is not int or not 0<row['max_stack']<=2**31-1):raise ValueError('未确认正数量单位前不得开启护符')
    return deepcopy(rules)

def buff_definitions(tables,skills,supply):
    """唯一状态表投影为服务端扁平类型；名称另保留为客户端展示白名单。"""
    result=[];names={};seen=set()
    for skill in skills:
        id=skill['buff_id'];kind=BUFF_BEHAVIORS.get(skill['behavior'])
        if not kind:
            if id or skill['material_item_id'] or skill['material_quantity']:raise ValueError('非状态技能不得附加状态或施法材料')
            continue
        row=next((r for r in tables['game_buffs'] if r['id']==id),None)
        if row is None or id in seen:raise ValueError('状态引用不存在或被多个技能复用')
        seen.add(id);attributes=row.get('attribute_effects');duration=row.get('duration_rule');immunity=row.get('immunity_rules');remove=row.get('remove_events')
        if not isinstance(attributes,dict) or set(attributes)!=set(BUFF_ATTRIBUTE_FIELDS) or not isinstance(duration,dict) or set(duration)!=set(BUFF_DURATION_FIELDS):raise ValueError('状态属性/时长规则必须完整且没有并行参数')
        if not isinstance(immunity,dict) or set(immunity)!={'allow_dead_targets','friendly_only'} or type(immunity['allow_dead_targets']) is not bool or immunity['friendly_only'] is not True:raise ValueError('状态目标与尸体策略缺失')
        if not isinstance(remove,list) or any(not isinstance(v,str) for v in remove) or len(set(remove))!=len(remove) or set(remove)!={'despawn','scene_dispose'}|({'player_death'} if kind=='magic_shield' else set()):raise ValueError('状态死亡、离场清理策略与来源不符')
        if type(row.get('duration_ms')) is not int or row['duration_ms']!=0:raise ValueError('状态时长只维护duration_rule，旧duration_ms必须显式中性0')
        if attributes['kind']!=kind:raise ValueError('技能与状态属性类型不一致')
        if any(not isinstance(row.get(k),str) or not row[k] for k in ['display_name','short_name']):raise ValueError('状态中文名称及短名必须来自Excel')
        flat={'id':id,'kind':kind,'value_by_level':deepcopy(attributes['value_by_level']),
              **{k:attributes[k] for k in ['target_level_divisor','target_level_add','hit_duration_loss_ms_per_hp']},
              'duration_kind':duration['kind'],'stat_add':duration['stat_add'],'stat_multiplier':duration['stat_multiplier'],
              'duration_seconds_by_level':deepcopy(duration['seconds_by_level']),'tick_ms':row['tick_ms'],
              'stack_policy':row['stack_policy'],'remove_on_player_death':'player_death' in remove,'allow_dead_targets':immunity['allow_dead_targets']}
        for key in ['value_by_level','duration_seconds_by_level']:
            if not isinstance(flat[key],list) or len(flat[key])!=skill['max_level']+1 or any(type(v) is not int or not 0<=v<=2**31-1 for v in flat[key]):raise ValueError('状态逐阶数值缺失或越界')
        for key in ['target_level_divisor','target_level_add','hit_duration_loss_ms_per_hp','stat_add','stat_multiplier','tick_ms']:
            if type(flat[key]) is not int or not 0<=flat[key]<=2**31-1:raise ValueError('状态数值必须为有界非负整数')
        if not flat['tick_ms']:raise ValueError('状态离散时钟不得为0')
        if kind=='magic_shield':
            if flat['duration_kind']!='mc_source_power' or flat['stack_policy']!='ignore_present_on_impact' or flat['allow_dead_targets'] or not flat['hit_duration_loss_ms_per_hp'] or any(v>100 for v in flat['value_by_level']):raise ValueError('魔法盾重施、减伤或时长规则无效')
            if flat['target_level_divisor'] or flat['target_level_add'] or flat['stat_multiplier'] or any(flat['duration_seconds_by_level']) or skill['material_item_id'] or skill['material_quantity']:raise ValueError('魔法盾不得混入道士属性或永久耗材')
        else:
            if flat['duration_kind']!='sc_rank_linear' or flat['stack_policy']!='refresh_duration_keep_values' or not flat['allow_dead_targets'] or not flat['target_level_divisor'] or flat['hit_duration_loss_ms_per_hp'] or flat['stat_add'] or any(flat['value_by_level']):raise ValueError('道士防御状态必须保留初值与尸体计时规则')
            if skill['material_item_id'] not in supply['item_ids'] or not skill['material_quantity']:raise ValueError('道士状态必须引用永久护符材料')
        result.append(flat);names[str(id)]={'name':row['display_name'],'short_name':row['short_name']}
    return result,names

def validate_skills(profile,skills):
    """技能参数必须来自当前Excel，不按技能名或来源表临时填运行数值。"""
    import math
    if not isinstance(skills,list) or not skills:raise ValueError('开发夹具缺少技能定义')
    ids=set();books=set();behaviors=set()
    for skill in skills:
        if not isinstance(skill,dict) or set(skill)!=set(SKILL_INT_FIELDS)|set(SKILL_BOOL_FIELDS)|set(SKILL_ARRAY_FIELDS)|{'name','behavior','target_kind','activation_kind'}:raise ValueError('技能字段不完整')
        if any(type(skill[k]) is not int or skill[k]<0 or skill[k]>2**32-1 for k in SKILL_INT_FIELDS):raise ValueError('技能字段须为有界非负整数')
        if any(type(skill[k]) is not bool for k in SKILL_BOOL_FIELDS):raise ValueError('技能开关必须是明确布尔值')
        if not isinstance(skill['name'],str) or not skill['name'] or skill['behavior'] not in SKILL_BEHAVIORS or skill['target_kind'] not in SKILL_TARGETS:raise ValueError('技能名称/行为/目标类型不合法')
        if not skill['id'] or skill['id'] in ids or not skill['book_item_id'] or skill['book_item_id'] in books or skill['job_id']>2 or not skill['learn_level']:
            raise ValueError('技能身份、职业、学习条件或技能书重复/缺失')
        if skill['max_level']<skill['learned_level'] or skill['max_level']>255:raise ValueError('技能开放等级域不合法')
        if skill['cooldown_reduction_ms']*skill['max_level']>skill['cooldown_base_ms']:raise ValueError('技能最高阶冷却不能为负')
        passive=skill['behavior'] in ['fencing','spirit_sword','slaying']
        if passive!=(skill['target_kind']=='passive'):raise ValueError('被动技能与目标类型不匹配')
        targets={'fencing':'passive','spirit_sword':'passive','slaying':'passive','thrusting':'attack_mode','half_moon':'attack_mode','flaming_sword':'attack_charge','fireball':'enemy_entity','great_fireball':'enemy_entity','thunderbolt':'enemy_entity','firebang':'enemy_grid','healing':'friendly_entity','mass_healing':'friendly_grid','hell_fire':'enemy_direction','lightning':'enemy_direction','ice_storm':'enemy_grid','thunder_storm':'enemy_self','magic_shield':'friendly_self','soul_shield':'friendly_grid','blessed_armour':'friendly_grid'}
        if skill['target_kind']!=targets[skill['behavior']]:raise ValueError('技能行为与合法目标种类不一致')
        activation={'fencing':'passive','spirit_sword':'passive','slaying':'passive_proc','thrusting':'toggle','half_moon':'toggle','flaming_sword':'charge'}.get(skill['behavior'],'cast')
        if skill['activation_kind']!=activation:raise ValueError('技能激活方式与执行管线不一致')
        if activation=='cast':
            if any(skill[k]<=0 for k in ['cooldown_base_ms','global_spell_ms','action_ms','delay_base_ms','auto_min_targets']):raise ValueError('施法时序或自动策略缺失')
            if skill['target_kind'] not in ['enemy_direction','enemy_self','friendly_self'] and skill['range_cells']<=0:raise ValueError('锁定或地面施法缺少射程')
            if skill['target_kind']=='friendly_self' and skill['range_cells']!=0:raise ValueError('自身技能不得附加射程')
        elif activation in ['toggle','charge','passive_proc'] and any(skill[k] for k in ['cooldown_base_ms','global_spell_ms','action_ms','delay_base_ms','projectile_ms_per_cell','auto_priority','auto_min_targets']):raise ValueError('近战模式不得复用施法时钟或自动施法策略')
        if skill['area_radius']>16 or (skill['target_kind'].endswith('_grid') or skill['target_kind']=='enemy_self')!=(skill['area_radius']>0):raise ValueError('区域半径与目标类型不匹配')
        for field in ['accuracy_by_level','max_attack_bonus_by_level','ready_proc_numerators']:
            value=skill[field]
            if not isinstance(value,list) or len(value)!=skill['max_level']+1 or any(type(v) is not int or v<0 or v>2**31-1 for v in value):raise ValueError('逐阶数组缺失或越界：'+field)
        offsets=skill['secondary_direction_offsets']
        if not isinstance(offsets,list) or len(offsets)>8 or any(type(v) is not int or v < -7 or v>7 for v in offsets):raise ValueError('近战旁格朝向无效')
        if len({v%8 for v in offsets})!=len(offsets) or (skill['secondary_distance_cells']==1 and 0 in offsets):raise ValueError('近战旁格重复或覆盖前格')
        if (activation=='toggle')!=bool(offsets) or bool(offsets)!=(skill['secondary_distance_cells']>0):raise ValueError('近战旁格范围与模式不一致')
        if (activation=='charge')!=(skill['charge_window_ms']>0):raise ValueError('充能截止与技能方式不一致')
        if (skill['target_kind']=='enemy_direction')!=(skill['line_cells']>0) or skill['line_cells']>16:raise ValueError('方向技能直线格数无效')
        if skill['behavior']=='hell_fire' and (skill['line_cells']!=4 or not skill['line_stop_on_blocked'] or skill['line_step_ms']<=0):raise ValueError('地狱火必须遵守用户已审定的固定四格与逐格阻挡管线')
        if skill['target_kind']!='enemy_direction' and (skill['line_step_ms'] or skill['line_stop_on_blocked']):raise ValueError('非方向技能不得附加分格传播')
        if skill['non_undead_divisor']<1:raise ValueError('非亡灵除数必须为正整数')
        if activation=='passive_proc':
            if not skill['ready_proc_denominator'] or any(n>skill['ready_proc_denominator'] for n in skill['ready_proc_numerators']):raise ValueError('攻杀准备概率无效')
        elif skill['ready_proc_denominator'] or any(skill['ready_proc_numerators']) or any(skill['max_attack_bonus_by_level']):raise ValueError('非攻杀技能不得附加准备概率或攻击上限')
        ids.add(skill['id']);books.add(skill['book_item_id']);behaviors.add(skill['behavior'])
    if not {'fencing','fireball','healing'}.issubset(behaviors):raise ValueError('首组三职业候选技能不完整')
    timing=profile.get('combat_timing')
    if not isinstance(timing,dict) or set(timing)!=set(COMBAT_TIMING_FIELDS) or any(type(v) is not int or v<=0 or v>2**31-1 for v in timing.values()):raise ValueError('战斗时钟与队列预算缺失或无效')
    if timing['player_attack_min_ms']>timing['player_attack_base_ms']:raise ValueError('攻击最短间隔大于基础间隔')
    if timing['event_capacity']>4096 or timing['kill_queue_capacity']>4096:raise ValueError('事件或逐杀队列超过受控预算上限')
    healing=profile.get('healing')
    if not isinstance(healing,dict) or set(healing)!=set(HEALING_FIELDS) or any(type(healing[k]) is not int or healing[k]<=0 for k in HEALING_FIELDS):raise ValueError('治疗队列参数不完整')
    players=profile.get('players');growth=profile.get('player_growth');initial=profile.get('initial_skill_books')
    if not isinstance(players,list) or {p['job_id'] for p in players}!={0,1,2}:raise ValueError('三职业基础属性缺失')
    if any(type(p.get('can_push')) is not bool for p in players):raise ValueError('玩家can_push必须显式布尔值，不得缺失、null或数字代替')
    if not isinstance(growth,list) or len(growth)!=3 or {g['job_id'] for g in growth}!={0,1,2}:raise ValueError('三职业成长缺失')
    for g in growth:
        if set(g)!=set(GROWTH_FIELDS)|{'job_id','hp_base','mp_base'}:raise ValueError('职业成长字段不完整')
        if any(type(g[k]) not in [int,float] or not math.isfinite(g[k]) or g[k]<0 for k in GROWTH_FIELDS):raise ValueError('职业成长系数不合法')
        if any(type(g[k]) is not int or g[k]<=0 for k in ['hp_base','mp_base']) or g['hp_gain']<=0 or g['mp_gain']<=0:raise ValueError('职业成长基数/除数不合法')
    expected={(s['job_id'],s['book_item_id']) for s in skills}
    if not isinstance(initial,list) or len(initial)!=len(players) or {r['job_id'] for r in initial}!={p['job_id'] for p in players} or any((r['job_id'],r['item_id']) not in expected for r in initial):raise ValueError('每职业仅赠一本所选同职业技能书，不随开放技能增加赠书')

def skill_economy(tables,profile,skills,items):
    """书价、页价、分解页数与逐技能消耗分别读取其唯一表，不回填测试缺省。"""
    economy=deepcopy(profile.get('skill_economy'))
    if not isinstance(economy,dict) or set(economy)!={'page_item_id','preview_ttl_ms','preview_material_limit'} or any(type(v) is not int or v<=0 for v in economy.values()):raise ValueError('技能经济技术参数缺失')
    by_item={r['id']:r for r in items};by_skill={r['id']:r for r in skills}
    if economy['page_item_id'] not in by_item or not by_item[economy['page_item_id']].get('is_skill_page'):raise ValueError('书页物品引用不符')
    upgrades=[]
    for row in tables['game_skill_upgrades']:
        if row['skill_id'] not in by_skill:continue
        skill=by_skill[row['skill_id']];rank=row['level'];costs=row['book_cost']
        if type(rank) is not int or not skill['learned_level']<rank<=skill['max_level']:raise ValueError('升级目标阶越界')
        if costs!=[{'item_id':skill['book_item_id'],'quantity':rank+1}]:raise ValueError('每阶技能书必须等于目标阶+1，且只消耗对应技能书')
        if row['preserve_behavior_shape'] is not True or row['allowed_numeric_changes']!={'native_rank':rank}:raise ValueError('升级只能改变已支持的原生数值阶，不能覆盖射程/目标/行为')
        if type(row['page_cost']) is not int or row['page_cost']<0 or row['page_item_id']!=economy['page_item_id'] or type(row['required_role_level']) is not int or row['required_role_level']<=0:raise ValueError('升级材料或等级条件缺失')
        upgrades.append({'skill_id':skill['id'],'level':rank,'required_role_level':row['required_role_level'],'book_costs':deepcopy(costs),'page_item_id':row['page_item_id'],'page_cost':row['page_cost']})
    for skill in skills:
        rows=[r for r in upgrades if r['skill_id']==skill['id']]
        if sorted(r['level'] for r in rows)!=list(range(skill['learned_level']+1,skill['max_level']+1)):raise ValueError('逐技能升级行重复或不连续')
    offers=[];seen=set()
    for choice in profile['skill_shop_entries']:
        try:row=next(r for r in tables['game_shop_prices'] if r['id']==choice['offer_id'])
        except StopIteration as error:raise ValueError('技能商品未引用商城表') from error
        currency=row['payment_currency'];price=row['price_gold'] if currency=='gold' else row['price_u']
        if choice['offer_id'] in seen or choice['item_id'] not in by_item or currency not in ['gold','ingot','bound_ingot'] or type(price) is not int or price<=0 or row['daily_limit'] is not None:raise ValueError('技能商品身份/价格/限购未受支持')
        if type(row['quantity']) is not int or row['quantity']<=0 or type(choice['max_purchase_quantity']) is not int or choice['max_purchase_quantity']<=0:raise ValueError('技能商品数量须为正整数')
        seen.add(choice['offer_id'])
        offers.append({'id':choice['offer_id'],'item_id':choice['item_id'],'quantity':row['quantity'],'currency':currency,'price_units':price,'required_role_level':choice['required_role_level'],'output_bound':row['purchased_bound'],'max_purchase_quantity':choice['max_purchase_quantity']})
    sources=profile['recycle_source_kinds'];books=profile['recyclable_book_ids']
    if not sources or set(sources)!={'monster_drop'} or len(set(books))!=len(books) or any(id not in by_item or not by_item[id].get('book_skill_id') for id in books):raise ValueError('可拆页书本或来源白名单不符；商店/赠书/未知来源不得启用')
    recycle=[]
    for item_id in books:
        for row in tables['game_book_recycle']:
            if row['npc_basic_book_pages']!=0 or type(row['pages']) is not int or row['pages']<=0:raise ValueError('NPC书不得生成书页，品质页数必须有效')
            recycle.append({'item_id':item_id,'book_quality':row['book_quality'],'page_item_id':economy['page_item_id'],'pages_per_book':row['pages'],'allowed_source_kinds':deepcopy(sources)})
    return {'skill_economy':economy,'skill_upgrades':upgrades,'skill_shop':offers,'skill_book_recycle':recycle}

def validate_map_inputs(definition,collision,profile,spawns):
    """地图/碰撞/宠物出生只读唯一Excel，表现几何不改变逻辑通行规则。"""
    width,height=definition['width_cells'],definition['height_cells']
    if any(type(v) is not int or v<=0 for v in [width,height]) or [width,height]!=[collision.get('width_cells'),collision.get('height_cells')]:raise ValueError('地图与碰撞尺寸缺失或不一致')
    geometry=definition.get('visual_geometry')
    if not isinstance(geometry,dict) or set(geometry)!={'projection','tile_width','tile_height','soil_thickness_px','source_origin'}:raise ValueError('地图表现几何必须完整来自Excel')
    if geometry['projection']!='isometric_diamond' or any(type(geometry[k]) is not int or geometry[k]<=0 for k in ['tile_width','tile_height','soil_thickness_px']):raise ValueError('菱形投影或土层尺寸无效')
    if geometry['tile_width']!=2*geometry['tile_height']:raise ValueError('当前菱形地图采用2:1地格比例')
    point=lambda p:isinstance(p,dict) and set(p)=={'x','y'} and all(type(v) is int and v>=0 for v in p.values())
    if not point(geometry['source_origin']):raise ValueError('地图来源裁剪原点无效')
    if collision.get('client_navigation')!={'geometry_reference':f'game_maps/{definition["id"]}.visual_geometry','collision_basis':'authored_edge_decorations'}:raise ValueError('客户端地图几何须引用唯一基础表，碰撞须明确为本作场边摆放')
    cells=collision['blocked_cells']
    if not isinstance(cells,list) or any(not point(p) or p['x']>=width or p['y']>=height for p in cells):raise ValueError('阻挡格坐标越界或格式错误')
    blocked={(p['x'],p['y']) for p in cells}
    if len(blocked)!=len(cells):raise ValueError('阻挡格重复')
    decorations=definition.get('decorations')
    if not isinstance(decorations,list) or len(decorations)!=len(cells):raise ValueError('场边装饰必须逐项引用唯一碰撞位置')
    if any(not isinstance(p,dict) or set(p)!={'id','frame','collision_cell_index'} or not isinstance(p['id'],str) or not p['id']
           or not isinstance(p['frame'],str) or not p['frame'].startswith('Objects:') or not p['frame'][8:].isdigit()
           or type(p['collision_cell_index']) is not int for p in decorations):raise ValueError('场边装饰身份/原帧/位置引用无效')
    if len({p['id'] for p in decorations})!=len(decorations) or sorted(p['collision_cell_index'] for p in decorations)!=list(range(len(cells))):raise ValueError('装饰身份或位置引用重复/缺失')
    if any(min(p['x'],p['y'],width-1-p['x'],height-1-p['y'])>1 for p in cells):raise ValueError('装饰仅放四周，中央保持开阔')
    locations=[definition['spawn_cell'],profile.get('pet_spawn'),*spawns]
    if any(not point(p) or p['x']>=width or p['y']>=height or (p['x'],p['y']) in blocked for p in locations):raise ValueError('玩家/宠物/怪物出生必须在地图可走格内')
    keys=[(p['x'],p['y']) for p in locations]
    if len(set(keys))!=len(keys):raise ValueError('静态出生槽不能重叠')
    seen={keys[0]};queue=[keys[0]]
    for x,y in queue:
        for dx,dy in [(0,-1),(1,-1),(1,0),(1,1),(0,1),(-1,1),(-1,0),(-1,-1)]:
            q=(x+dx,y+dy)
            if q in seen or q in blocked or not(0<=q[0]<width and 0<=q[1]<height):continue
            if dx and dy and ((x+dx,y) in blocked or (x,y+dy) in blocked):continue
            seen.add(q);queue.append(q)
    if any(p not in seen for p in keys):raise ValueError('出生槽存在静态不可达位置')
    return deepcopy(geometry)

def validate_map_asset(tables):
    """生成地图只作派生一致性证据，不再反向提供宠物出生或地格数值。"""
    profile=tables['dev_first_loop_profile'][0]['parameters'];definition=next(x for x in tables['game_maps'] if x['id']==profile['map_id'])
    collision=next(x for x in tables['game_map_collisions'] if x['id']==definition['collision_id']);geometry=definition['visual_geometry']
    asset=read(ASSETS/'maps/first-map.json')
    if (asset['width'],asset['height'])!=(definition['width_cells'],definition['height_cells']):raise ValueError('生成地图与唯一Excel尺寸不一致')
    if any(asset.get(k)!=v for k,v in geometry.items()):raise ValueError('生成地图投影与唯一Excel不一致')
    if asset.get('source_sha256')!=collision['source_hash']:raise ValueError('地图来源文件摘要与唯一碰撞表不一致')
    if asset.get('blocked')!=collision['blocked_cells'] or asset.get('pet_spawn')!=profile['pet_spawn'] or asset.get('player_spawn')!=definition['spawn_cell']:raise ValueError('地图派生碰撞或出生与唯一Excel不一致')
    spawns=[r['cell'] for r in tables['game_monster_spawns'] if r['map_id']==definition['id']]
    if asset.get('monster_spawns')!=spawns:raise ValueError('地图派生怪物槽位与唯一Excel不一致')
    cells=asset.get('cells')
    if not isinstance(cells,list) or len(cells)!=asset['height'] or any(not isinstance(row,list) or len(row)!=asset['width'] for row in cells):raise ValueError('地图必须包含完整格子，不能靠负表缺省扩大')
    actual={(x,y) for y,row in enumerate(cells) for x,cell in enumerate(row) if cell['blocked']}
    if actual!={(p['x'],p['y']) for p in collision['blocked_cells']}:raise ValueError('本作运行格碰撞与唯一Excel不一致')
    if asset.get('collision_basis')!='authored_edge_decorations' or any(type(c.get(k)) is not bool for row in cells for c in row for k in ['source_blocked','blocked']):raise ValueError('原始来源碰撞与本作运行碰撞必须分别记录')
    source=[{'x':x,'y':y} for y,row in enumerate(cells) for x,c in enumerate(row) if c['source_blocked']]
    if asset.get('source_blocked')!=source:raise ValueError('原始来源碰撞证据不完整')
    props=asset.get('props')
    expected=[{'id':p['id'],'frame':p['frame'],**collision['blocked_cells'][p['collision_cell_index']]} for p in definition['decorations']]
    if not isinstance(props,list) or [{k:p.get(k) for k in ['id','frame','x','y']} for p in props]!=expected:raise ValueError('运行装饰不得混入未配置的大树或其它物件')
    return asset

def fixture_from_rows(tables):
    one=lambda table,id:next(r for r in tables[table] if r['id']==id)
    profile=one('dev_first_loop_profile',1)['parameters'];m=one('game_maps',1);collision=one('game_map_collisions',m['collision_id'])
    spawns=[r['cell'] for r in tables['game_monster_spawns'] if r['map_id']==m['id']]
    validate_map_inputs(m,collision,profile,spawns)
    monster=one('game_monsters',profile['monster_id']);pet_def=one('game_pet_templates',profile['pet_id']);pet=one('game_monsters',pet_def['monster_template_id'])
    if any(type(r['attributes'].get('can_push')) is not bool for r in [monster,pet]):raise ValueError('已选怪物及宠物来源can_push必须显式布尔值')
    skills=skill_definitions(tables,profile);validate_skills(profile,skills)
    supply=casting_supply_rules(profile,tables);buffs,buff_names=buff_definitions(tables,skills,supply)
    if profile['combat_timing']['kill_queue_capacity']<sum(r['map_id']==m['id'] for r in tables['game_monster_spawns'])+1:raise ValueError('逐杀队列必须覆盖本地图全部怪物槽及后续结算边界')
    healing=deepcopy(profile['healing']);growth=deepcopy(profile['player_growth'])
    resist_weights={row['magic_resist_weight'] for row in profile['players']}
    if len(resist_weights)!=1:raise ValueError('当前开发子集使用同一有来源的魔法闪避权重')
    resist_weight=next(iter(resist_weights))
    fighter=lambda r:{'job_id':2 if r['id']==2 else 0,'definition_id':r['id'],'level':r['level'],'hp':r['hp'],'mp':r['attributes']['mp'],
        'attack_min':r['attributes']['physical_attack_min'],'attack_max':r['attributes']['physical_attack_max'],
        'defence':r['attributes']['physical_defence_min'],'defence_min':r['attributes']['physical_defence_min'],
        'defence_max':r['attributes']['physical_defence_max'],'magic_defence_min':r['attributes']['magic_defence'],
        'magic_defence_max':r['attributes']['magic_defence'],'magic_min':0,'magic_max':0,'tao_min':0,'tao_max':0,
        'accuracy':r['attributes']['accuracy'],'agility':r['attributes']['agility'],
        'magic_resist':0,'magic_resist_weight':resist_weight,
        'attack_speed':0,'attack_ms':r['attack_interval_ms'],'move_ms':r['move_interval_ms'],'undead':r['attributes']['undead'],'can_push':r['attributes']['can_push']}
    items=[]
    for r in tables['game_items']:
        if r['id'] not in [35,36,193,218]:continue
        eq=next((x for x in tables['game_equipment'] if x['item_id']==r['id']),None);attrs=eq['base_attributes'] if eq else {}
        items.append({'id':r['id'],'name':r['display_name'],'equipment_slot':0 if eq and eq['equipment_type']=='weapon' else 1 if eq else 0,
                      'is_equipment':eq is not None,'attack_bonus':0,'attack_min_bonus':attrs.get('physical_attack_min',0),
                      'attack_max_bonus':attrs.get('physical_attack_max',0),'defence_bonus':attrs.get('physical_defence_min',0),
                      'defence_min_bonus':attrs.get('physical_defence_min',0),'defence_max_bonus':attrs.get('physical_defence_max',0),
                      'magic_defence_min_bonus':attrs.get('magic_defence_min',0),'magic_defence_max_bonus':attrs.get('magic_defence_max',0),
                      'attack_speed_bonus':attrs['attack_speed'] if eq else 0,
                      'recycle_gold':profile['recycle_test_gold_by_item'][str(r['id'])],'stackable':eq is None,
                      'allowed_jobs':eq['class_restriction'] if eq else [0,1,2]})
    for skill in skills:
        if any(item['id']==skill['book_item_id'] for item in items):raise ValueError('技能书ID与已有物品冲突')
        source=one('game_items',skill['book_item_id'])
        items.append({'id':skill['book_item_id'],'name':source['display_name'],'equipment_slot':0,'is_equipment':False,
                      'book_skill_id':skill['id'],'attack_bonus':0,'attack_min_bonus':0,'attack_max_bonus':0,'defence_bonus':0,
                      'defence_min_bonus':0,'defence_max_bonus':0,'magic_defence_min_bonus':0,'magic_defence_max_bonus':0,
                      'recycle_gold':0,'stackable':False,'max_stack':source['max_stack'],'drop_quality':profile['skill_book_drop_quality'],'allowed_jobs':[skill['job_id']]})
    source=one('game_items',profile['skill_economy']['page_item_id'])
    items.append({'id':source['id'],'name':source['display_name'],'equipment_slot':0,'is_equipment':False,'is_skill_page':True,
                  'max_stack':source['max_stack'],'recycle_gold':0,'stackable':True,'allowed_jobs':[0,1,2]})
    for id in supply['item_ids']:
        source=one('game_items',id)
        if any(item['id']==id for item in items):raise ValueError('施法材料不能同时充当装备、技能书或药品')
        items.append({'id':id,'name':source['display_name'],'kind':'casting_supply','equipment_slot':supply['slot'],
                      'is_equipment':False,'stackable':True,'max_stack':source['max_stack'] if supply['enabled'] else 0,
                      'recycle_gold':0,'allowed_jobs':[0,1,2]})
    economy=skill_economy(tables,profile,skills,items)
    if not supply['enabled'] and any(r['item_id'] in supply['item_ids'] for r in economy['skill_shop']):raise ValueError('护符单位/价格未授权，不得开启护符商品')
    categories={r['category_key']:r['prob_ppm'] for r in tables['game_drop_categories']}
    drop_gold=one('game_drop_entries',5)
    potions=potion_rules(profile,items)
    result={'schema_version':1,'rule_version':profile['rule_version'],'development_only':True,'runtime_ready':False,'source_status':'candidate',
        'damage_formula':profile['damage_formula'],'allow_free_dev_revive':profile['allow_free_dev_revive'],
        'potions':potions,'skills':skills,'buffs':buffs,'casting_supply':supply,'combat_timing':deepcopy(profile['combat_timing']),'healing':healing,'player_growth':growth,'initial_skill_books':deepcopy(profile['initial_skill_books']),**economy,
        'map':{'id':m['id'],'width':m['width_cells'],'height':m['height_cells'],'blocked':deepcopy(collision['blocked_cells']),
               'player_spawn':deepcopy(m['spawn_cell']),'pet_spawn':deepcopy(profile['pet_spawn']),'monster_spawns':deepcopy(spawns)},
        'players':profile['players'],'pet':fighter(pet),'monsters':[{**fighter(monster),'id':monster['id'],'exp':monster['exp'],
                    'gold_min':drop_gold['quantity_min'],'gold_max':drop_gold['quantity_max'],'respawn_ms':10000}],
        'items':items,'drops':{'equipment_item_ids':[r['item_id'] for r in tables['game_drop_entries'] if r['pool_id']==1],
                             'item_ids':[r['item_id'] for r in tables['game_drop_entries'] if r['pool_id']==2],
                             **{key+'_ppm':categories[key] for key in ['empty','gold','item','equipment']},'equipment_guarantee':20},
        'progression':{'exp_per_level':[r['next_level_exp'] for r in sorted(tables['game_exp_levels'],key=lambda r:r['level'])]},
        'initial_inventory':profile['initial_inventory'],'initial_bound_gold':profile['initial_bound_gold'],'inventory_capacity':profile['inventory_capacity'],
        '_source_evidence':{'status':'candidate','official_180_restoration_verified':False,
                            'buff_names':buff_names,
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
                                'parameters_source':'唯一玩法Excel：技能基础定义、技能逐级扩展、商城商品候选、技能书分解候选；开发夹具仅引用ID与技术预算',
                                'scope':'已登记开发技能及三状态来源候选；范围、状态与材料按唯一表投影，转换成功不代表本阶段业务或完整目标版本通过',
                                'learning_adaptation':'学习取各自Crystal Level1为本作开发适配；新角色仍按首组三职业1本绑定书，原生初学0，未学由列表缺项表示',
                                'attribute_scope':'三职默认成长、DC/MC/SC/AC/MAC与准确敏捷，新增三防御状态候选；不含元素/幸运/完整Buff，来源默认不是源服实库',
                                'monster_resist_scope':'本轮开发怪物/宠物不配置魔法闪避加成（0）；权重从唯一Excel中共同的源码默认候选权重取得',
                                'formal_skill_and_upgrade_tables_populated':True,
                                'economy_reference':'docs/技能获取与升级实施边界.md；书本=目标阶+1用户确定，页与来源渠道候选'}}}
    return result

def public_gameplay(fixture):
    """客户端只接收显式展示白名单，来源、夹具说明和服务端内部字段留在源侧。"""
    potions=fixture['potions'];effects={effect['item_id']:effect for effect in potions['effects']}
    shared={field:potions[field] for field in POTION_FIELDS}
    items=[]
    for row in fixture['items']:
        effect=effects.get(row['id']) if not row['is_equipment'] else None
        item={'item_id':row['id'],'name':row['name'],'kind':'casting_supply' if row.get('kind')=='casting_supply' else 'equipment' if row['is_equipment'] else 'book' if row.get('book_skill_id') else 'skill_page' if row.get('is_skill_page') else 'potion',
              **({'equip_slot':row['equipment_slot']} if row['is_equipment'] or row.get('kind')=='casting_supply' else {}),
              'allowed_jobs':deepcopy(row['allowed_jobs']),'use_enabled':effect is not None}
        if effect is not None:item['potion']={**shared,'hp':effect['hp'],'mp':effect['mp']}
        if row.get('book_skill_id'):item['skill_id']=row['book_skill_id']
        items.append(item)
    skills=[{'id':s['id'],'name':s['name'],'job_id':s['job_id'],'book_item_id':s['book_item_id'],
             'learn_level':s['learn_level'],'initial_level':s['learned_level'],'max_level':s['max_level'],'kind':s['behavior'],'active':s['activation_kind'] not in ['passive','passive_proc'],
             'activation_kind':s['activation_kind'],'auto_enabled':s['auto_enabled'],
             'buff_id':s['buff_id'],'material_item_id':s['material_item_id'],'material_quantity':s['material_quantity'],
             'target_kind':s['target_kind'],'area_radius':s['area_radius'],'require_line_of_sight':s['require_line_of_sight'],
             'range':s['range_cells'],'base_cost':s['mp_base_cost'],'level_cost':s['mp_level_cost'],
             'cast_ms':s['action_ms'],'delay_base_ms':s['cooldown_base_ms'],'delay_reduction_ms':s['cooldown_reduction_ms'],
             'impact_base_ms':s['delay_base_ms'],'projectile_ms_per_cell':s['projectile_ms_per_cell'],
             'accuracy_per_level':s['accuracy_per_level'],'accuracy_by_level':deepcopy(s['accuracy_by_level']),
             'max_attack_bonus_by_level':deepcopy(s['max_attack_bonus_by_level']),
             'secondary_direction_offsets':deepcopy(s['secondary_direction_offsets']),'secondary_distance_cells':s['secondary_distance_cells'],
             'line_cells':s['line_cells'],'line_step_ms':s['line_step_ms'],'charge_window_ms':s['charge_window_ms'],
             'source_status':'candidate','target_180_verified':False} for s in fixture['skills']]
    statuses=[]
    for buff in fixture['buffs']:
        names=fixture['_source_evidence']['buff_names'][str(buff['id'])]
        statuses.append({'id':buff['id'],'name':names['name'],'short_name':names['short_name'],'kind':buff['kind'],
            'stack_policy':buff['stack_policy'],'tick_ms':buff['tick_ms'],'remove_on_player_death':buff['remove_on_player_death'],
            'duration_rule':{'kind':buff['duration_kind'],'stat_add':buff['stat_add'],'stat_multiplier':buff['stat_multiplier'],'seconds_by_level':deepcopy(buff['duration_seconds_by_level'])},
            'attribute_effects':{'kind':buff['kind'],'value_by_level':deepcopy(buff['value_by_level']),
                **{key:buff[key] for key in ['target_level_divisor','target_level_add','hit_duration_loss_ms_per_hp']}}})
    return {'schema_version':1,'rule_version':fixture['rule_version'],'development_only':True,'runtime_ready':False,'status':'candidate',
            'skills':skills,'statuses':statuses,'casting_supply':deepcopy(fixture['casting_supply']),
            'skill_upgrades':deepcopy(fixture['skill_upgrades']),'skill_book_recycle':deepcopy(fixture['skill_book_recycle']),
            'shop_offers':[{'offer_id':r['id'],'item_id':r['item_id'],'name':next(i['name'] for i in fixture['items'] if i['id']==r['item_id']),
                            'quantity':r['quantity'],'currency_kind':r['currency'],'price_units':r['price_units'],'required_role_level':r['required_role_level'],
                            'output_bound':r['output_bound'],'max_purchase_quantity':r['max_purchase_quantity'],'source_status':'candidate'} for r in fixture['skill_shop']],
            'items':items,
            'jobs':[{'job_id':p['job_id'],'name':['战士','法师','道士'][p['job_id']],'profile':'player_female' if p['job_id']==1 else 'player_male','can_push':p['can_push']} for p in fixture['players']],
            'experience':[{'level':i+1,'next_level_exp':n} for i,n in enumerate(fixture['progression']['exp_per_level'])],
            'monster_profiles':[{'definition_id':m['definition_id'],'name':'稻草人','profile':'monster_scarecrow','can_push':m['can_push']} for m in fixture['monsters']],
            'pet_profiles':[{'definition_id':fixture['pet']['definition_id'],'name':'变异骷髅','profile':'pet_skeleton','job_id':2,'can_push':fixture['pet']['can_push']}]}

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
        validate_map_asset(tables)
        selected={tid:tables[tid] for tid in TABLE_IDS}
        selected['game_drop_categories']=tables['game_drop_categories'];fixture=fixture_from_rows(selected)
        fixture['_source_evidence']['workbook_sync']='exported_from_current_unique_excel'
        fixture['_source_evidence']['source_locks']=schema['source_locks']
        book=ROOT/next(w['path'] for w in schema['workbooks'] if w['group']=='gameplay')
        fixture['_source_evidence']['workbook']={'path':book.relative_to(ROOT).as_posix(),'sha256':config.sha256(book)}
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
