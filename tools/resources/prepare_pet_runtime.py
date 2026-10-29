#!/usr/bin/env python3
"""两本唯一Excel共同编译宠物定义；独立候选保持关闭，profile显式选择后才生成运行规则。"""
from __future__ import annotations

import argparse
from copy import deepcopy
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT/'tools/config'))
import export_configs as config

DEST = ROOT/'config/development/pet-runtime-candidates.json'
CAPACITY_CODE = 'retained_pet_body_capacity_per_owner'
FULL_POLICY = 'reject_birth_without_discarding_corpses'
I64 = 2**63-1
U64 = 2**64-1
NEUTRAL_FIELDS = {'magic_min', 'magic_max', 'tao_min', 'tao_max', 'magic_resist', 'attack_speed'}
GROWTH_FIELDS = {'hp_bonus', 'attack_min_bonus', 'attack_max_bonus', 'defence_min_bonus', 'defence_max_bonus', 'magic_defence_min_bonus', 'magic_defence_max_bonus'}
TRAINING_FIELDS = {'initial_level_by_rank', 'maximum_level_by_rank', 'experience_multiplier', 'owner_range_cells', 'experience_basis', 'grant_phase', 'eligibility', 'max_level_ups_per_grant', 'excess_experience', 'at_maximum', 'current_hp_on_growth'}
SPEED_FIELDS = {'basis', 'move_reduction_ms_per_level', 'attack_reduction_ms_per_level', 'minimum_move_interval_ms', 'minimum_attack_interval_ms'}
LIFECYCLE_FIELDS = {'corpse_ttl_ms', 'spawn_ready_ms', 'live_expiry', 'cold_enter'}


def exact(value, fields, label):
    if not isinstance(value, dict) or set(value) != set(fields):
        raise ValueError(label+'字段必须完整且不得有额外键')
    return value


def integer(value, label, minimum=0, maximum=I64):
    if type(value) is not int or not minimum <= value <= maximum:
        raise ValueError(label+'须为有界整数，不能补默认或截断')
    return value


def one(tables, table, identifier):
    values = [row for row in tables[table] if row['id'] == identifier and row['record_status'] != 'historical']
    if len(values) != 1:
        raise ValueError(table+'来源身份缺失或重复')
    return values[0]


def compile_budgets(tables):
    rows = [row for row in tables['arch_runtime_budgets'] if row['code'] == CAPACITY_CODE and row['record_status'] != 'historical']
    if len(rows) != 1 or rows[0]['value_unit'] != '身体/owner' or rows[0]['adaptation_status'] != 'adapt':
        raise ValueError('宠物保留身体预算须在唯一架构Excel显式维护且不得重复')
    capacity = integer(rows[0]['value'], '宠物保留身体容量', 1, 128)
    return {'retained_body_capacity_per_owner': capacity, 'full_policy': FULL_POLICY}


def compile_base(tables, definition):
    rules = exact(definition['base_attributes'], {'kind', 'owner_job_id', 'neutral_attributes', 'magic_resist_weight_source'}, '宠物基础继承')
    if rules['kind'] != 'inherit_monster_template' or rules['magic_resist_weight_source'] != 'profile_player_shared':
        raise ValueError('宠物基础属性须唯一继承怪物模板及既有公共魔闪权重')
    job = integer(rules['owner_job_id'], '宠物职业', 0, 2)
    neutral = exact(rules['neutral_attributes'], NEUTRAL_FIELDS, '宠物中性属性')
    if any(type(value) is not int or value != 0 for value in neutral.values()):
        raise ValueError('当前来源宠物不使用的MC/SC/魔闪/攻速须显式中性0')
    monster_id = integer(definition['monster_template_id'], '宠物来源怪物身份', 1, 2**32-1)
    monster = one(tables, 'game_monsters', monster_id)
    attrs = monster['attributes']
    profile = one(tables, 'dev_first_loop_profile', 1)['parameters']
    weights = [row['magic_resist_weight'] for row in profile['players']]
    if len(weights) != 3 or any(type(value) is not int or value <= 0 for value in weights) or len(set(weights)) != 1:
        raise ValueError('宠物魔闪权重须来自全职业相同的既有公共值')
    for field in ['undead', 'can_push']:
        if type(attrs[field]) is not bool:
            raise ValueError('宠物来源'+field+'必须显式布尔')
    base = {'job_id': job, 'definition_id': monster['id'], 'level': monster['level'], 'hp': monster['hp'],
            'mp': attrs['mp'], 'attack_min': attrs['physical_attack_min'], 'attack_max': attrs['physical_attack_max'],
            'defence': attrs['physical_defence_min'], 'defence_min': attrs['physical_defence_min'],
            'defence_max': attrs['physical_defence_max'], 'magic_defence_min': attrs['magic_defence'],
            'magic_defence_max': attrs['magic_defence'], 'accuracy': attrs['accuracy'], 'agility': attrs['agility'],
            'attack_ms': monster['attack_interval_ms'], 'move_ms': monster['move_interval_ms'],
            'magic_resist_weight': weights[0], 'can_push': attrs['can_push'], **deepcopy(neutral)}
    for key, value in base.items():
        if key == 'can_push':
            continue
        integer(value, '宠物基础'+key, 1 if key in ['level', 'hp', 'attack_ms', 'move_ms', 'definition_id'] else 0)
    if base['attack_max'] < base['attack_min'] or base['defence_max'] < base['defence_min']:
        raise ValueError('宠物基础上下限倒置')
    if any(base[key] > 2**32-1 for key in ['level', 'definition_id', 'attack_ms', 'move_ms']):
        raise ValueError('宠物基础身份或动作间隔超过Go类型')
    return base, attrs['undead']


def compile_definitions(tables, definition_ids):
    if not isinstance(definition_ids, list) or not definition_ids or len(definition_ids) > 256 or any(type(value) is not int or not 0 < value <= 2**32-1 for value in definition_ids) or len(set(definition_ids)) != len(definition_ids):
        raise ValueError('宠物定义选择必须显式、唯一且有界')
    output, body_ids = [], set()
    for identifier in definition_ids:
        definition = one(tables, 'game_pet_templates', identifier)
        follow = exact(definition['follow_and_return'], {'source_skill_id', 'training', 'speed'}, '宠物公共规则')
        training = exact(follow['training'], TRAINING_FIELDS, '宠物训练')
        speed = exact(follow['speed'], SPEED_FIELDS, '宠物速度')
        lifecycle = exact(definition['lifetime_and_betrayal'], LIFECYCLE_FIELDS, '宠物生命期限')
        source_skill = integer(follow['source_skill_id'], '宠物来源技能身份', 1, 2**32-1)
        if definition['summon_skill_id'] is not None and definition['summon_skill_id'] != source_skill:
            raise ValueError('宠物正式召唤引用与来源候选身份不同')
        base, undead = compile_base(tables, definition)
        if base['definition_id'] in body_ids:
            raise ValueError('多个宠物定义不能复用同一body身份')
        body_ids.add(base['definition_id'])
        resource = definition['resource_key']
        if not isinstance(resource, str) or not resource or '..' in resource:
            raise ValueError('宠物资源身份缺失或越界')
        for key in ['initial_level_by_rank', 'maximum_level_by_rank']:
            values = training[key]
            if not isinstance(values, list) or len(values) != 4:
                raise ValueError('宠物初级与最大训练级须显式覆盖四阶')
            for value in values:
                integer(value, key, 0, 255)
        if any(start > end for start, end in zip(training['initial_level_by_rank'], training['maximum_level_by_rank'])):
            raise ValueError('宠物初始训练级超过该出生上限')
        integer(training['experience_multiplier'], '宠物经验倍率', 1, U64)
        integer(training['owner_range_cells'], '宠物主人范围', 0, 65535)
        enums = {'experience_basis': 'owner_actual_granted_experience', 'grant_phase': 'confirmed_owner_experience',
                 'eligibility': 'alive_same_world_within_owner_range', 'excess_experience': 'keep',
                 'at_maximum': 'do_not_accumulate', 'current_hp_on_growth': 'keep'}
        if any(training[key] != value for key, value in enums.items()) or type(training['max_level_ups_per_grant']) is not int or training['max_level_ups_per_grant'] != 1:
            raise ValueError('宠物训练资格、确认阶段或刷新规则不同')
        if speed['basis'] != 'maximum_training_level':
            raise ValueError('宠物速度减量须按出生最大训练级')
        for key in SPEED_FIELDS-{'basis'}:
            integer(speed[key], key, 1 if key.startswith('minimum_') else 0, 2**32-1 if key.startswith('minimum_') else I64)
        integer(lifecycle['corpse_ttl_ms'], '宠物尸体期限', 1, 86400000)
        integer(lifecycle['spawn_ready_ms'], '宠物出生动作期限', 0, 86400000)
        if lifecycle['live_expiry'] != 'no_tame_expiry' or lifecycle['cold_enter'] != 'do_not_restore_temporary_pet':
            raise ValueError('召唤宠不能混入诱惑叛变或临时快照恢复')
        growth_rows = [row for row in tables['game_pet_growth'] if row['pet_id'] == identifier and row['record_status'] != 'historical']
        for row in growth_rows:
            integer(row['growth_level'], '宠物成长等级', 0, 255)
        growth_rows.sort(key=lambda row: row['growth_level'])
        maximum = max(training['maximum_level_by_rank'])
        if len(growth_rows) != maximum+1:
            raise ValueError('宠物逐级成长须完整覆盖所有出生上限')
        growth = []
        mapping = {'hp_bonus': 'hp', 'attack_min_bonus': 'attack_min', 'attack_max_bonus': 'attack_max',
                   'defence_min_bonus': 'defence_min', 'defence_max_bonus': 'defence_max',
                   'magic_defence_min_bonus': 'magic_defence_min', 'magic_defence_max_bonus': 'magic_defence_max'}
        for level, row in enumerate(growth_rows):
            if type(row['growth_level']) is not int or row['growth_level'] != level:
                raise ValueError('宠物训练等级必须连续唯一且从0开始')
            attrs = exact(row['attributes'], GROWTH_FIELDS, '宠物逐级增量')
            required = exact(row['required_conditions'], {'required_experience'}, '宠物逐级阈值')['required_experience']
            if level == maximum:
                if required is not None:
                    raise ValueError('宠物成长终态阈值必须显式null')
            else:
                integer(required, '宠物下一层经验', 1, U64)
            for key, value in attrs.items():
                integer(value, key, 0, I64-base[mapping[key]])
            for low, high in [('attack_min', 'attack_max'), ('defence_min', 'defence_max'), ('magic_defence_min', 'magic_defence_max')]:
                if base[low]+attrs[low+'_bonus'] > base[high]+attrs[high+'_bonus']:
                    raise ValueError('宠物成长属性上下限倒置')
            growth.append({'level': level, **deepcopy(attrs), 'required_experience': required})
        output.append({'id': identifier, 'source_skill_id': source_skill, 'monster_template_id': definition['monster_template_id'],
                       'owner_job_id': base['job_id'], 'resource_key': resource, 'base': base, 'undead': undead,
                       'training': deepcopy(training), 'speed': deepcopy(speed), 'lifecycle': deepcopy(lifecycle), 'growth': growth})
    return output


def compile_runtime(tables, profile):
    if profile['rule_version'] < 16:
        if 'pet_runtime' in profile:
            raise ValueError('宠物生命周期选择须使用rule16及后续合同')
        return None
    selected = exact(profile.get('pet_runtime'), {'enabled', 'bootstrap_mode', 'definition_ids'}, 'rule16宠物选择')
    if selected['enabled'] is not True or selected['bootstrap_mode'] not in ['legacy_untrained_pet', 'none']:
        raise ValueError('rule16宠物合同须显式启用并选择bootstrap模式')
    definitions = compile_definitions(tables, selected['definition_ids'])
    if selected['bootstrap_mode'] == 'legacy_untrained_pet' and profile['pet_id'] not in selected['definition_ids']:
        raise ValueError('旧bootstrap必须引用已选同源宠物定义')
    return {'enabled': True, 'bootstrap_mode': selected['bootstrap_mode'], 'budgets': compile_budgets(tables), 'definitions': definitions}


def build_catalog(tables, source_evidence):
    ids = [row['id'] for row in tables['game_pet_templates'] if row['record_status'] != 'historical']
    return {'schema_version': 1, 'development_only': True, 'runtime_enabled': False,
            'budgets': compile_budgets(tables), 'definitions': compile_definitions(tables, ids),
            'source_evidence': deepcopy(source_evidence)}


def source_evidence(schema, workbook_paths):
    sys.path.insert(0, str(ROOT/'tools/reference'))
    import verify_pet_sources
    verify_pet_sources.verify()
    return {'workbooks': [{'group': group, 'path': path.relative_to(ROOT).as_posix(), 'sha256': config.sha256(path)} for group, path in workbook_paths.items()],
            'schema_sha256': config.sha256(ROOT/'config/schema/catalog.json'),
            'source_contract_sha256': config.sha256(ROOT/'资料索引/职业宠物补充来源契约.json'),
            'generator_sha256': config.sha256(Path(__file__)), 'target_180_verified': False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    schema = config.load_json(ROOT/'config/schema/catalog.json')
    tables, errors, _, paths = config.collect(schema)
    errors += config.check_json_contracts(schema, tables)+config.semantic_checks(tables)
    if errors:
        raise ValueError(errors)
    catalog = build_catalog(tables, source_evidence(schema, paths))
    if args.check:
        if json.loads(DEST.read_text(encoding='utf-8')) != catalog:
            raise ValueError('宠物候选过期，须从两本唯一Excel重新生成')
    else:
        DEST.parent.mkdir(parents=True, exist_ok=True)
        DEST.write_text(json.dumps(catalog, ensure_ascii=False, indent=2)+'\n', encoding='utf-8', newline='\n')
    print(json.dumps({'check': args.check, 'runtime_enabled': False, 'definition_count': len(catalog['definitions'])}))


if __name__ == '__main__':
    main()
