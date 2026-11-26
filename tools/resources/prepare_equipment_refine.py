#!/usr/bin/env python3
"""由唯一玩法工作簿生成装备洗炼隔离夹具与公开目录。"""

import argparse
from decimal import Decimal
import json
from pathlib import Path
import re

import prepare_combat_luck as combat
import prepare_equipment_enhancement as enhancement
import prepare_forge as forge
import prepare_mining as mining


KEY = 'equipment_refine'
QUALITY_TABLE = 'game_refine_quality'
AFFIX_TABLE = 'game_affixes'
TIER_TABLE = 'game_refine_tiers'
LOCK_TABLE = 'game_refine_lock_costs'
PRICE_TABLE = 'game_level_band_economy'
RECYCLE_TABLE = 'game_recycle_values'
# 当前战斗管线接受的原词条键，按稳定编号核对，未实现的词条留待正式表。
IMPLEMENTED = {1: 'attack', 2: 'defence', 3: 'max_hp', 5: 'max_mp', 12: 'magic', 13: 'tao'}
MILLION = 1_000_000


def compile_prices(tables, sources):
    """保留原五档权重和锁槽单位，金币价按装备需求等级使用。"""
    set_tiers = combat.indexed_rows(tables[TIER_TABLE], lambda row: row['tier'], set(range(1, 6)), '洗炼档位')
    tiers = []
    for tier, row in sorted(set_tiers.items()):
        sources.append(forge.source(row, 'confirmed', TIER_TABLE))
        tiers.append({'tier': tier, 'multiplier_bp': mining.integer(row['multiplier_bp'], 2**32-1, '洗炼档位倍率'),
                      'weight_ppm': mining.integer(row['weight_ppm'], MILLION, '洗炼档位概率')})
    if sum(row['weight_ppm'] for row in tiers) != MILLION or any(
            a['multiplier_bp'] >= b['multiplier_bp'] for a, b in zip(tiers, tiers[1:])):
        raise ValueError('五档倍率或抽样权重不完整')
    set_locks = combat.indexed_rows(tables[LOCK_TABLE], lambda row: row['locked_slots'], set(range(4)), '洗炼锁槽')
    locks = []
    for count, row in sorted(set_locks.items()):
        sources.append(forge.source(row, 'confirmed', LOCK_TABLE))
        locks.append({'locked_slots': count, 'ingot_u': combat.bounded_integer(row['extra_ingot_u_per_attempt'], 0,
                                                                             2**63-1, '洗炼锁槽费用')})
    if locks[0]['ingot_u'] != 0 or any(a['ingot_u'] >= b['ingot_u'] for a, b in zip(locks, locks[1:])):
        raise ValueError('洗炼锁槽费用须从零递增')
    gold_bands, expected_min = [], 1
    bands = sorted(tables[PRICE_TABLE], key=lambda row: row['level_min'])
    if not bands:
        raise ValueError('缺少当前洗炼金币等级费用')
    for index, row in enumerate(bands):
        sources.append(forge.source(row, 'candidate', PRICE_TABLE))
        maximum = row['level_max']
        if (row['level_min'] != expected_min or (index == len(bands)-1) != (maximum is None)
                or maximum is not None and not expected_min <= maximum < 2**32-1):
            raise ValueError('按装备需求等级定价须连续且末档无上限')
        gold_bands.append({'min_level': expected_min, 'max_level': maximum or 0,
                           'gold': mining.integer(row['refine_gold_per_attempt'], 2**63-1, '洗炼金币费用')})
        expected_min = (maximum or 0) + 1
    return tiers, locks, gold_bands


def compile_pool(tables, entered, enabled, target_stages, sources):
    """只取已接战斗的词条；权重、阶段基值和禁止重复均注明为隔离前态。"""
    rows = combat.indexed_rows(tables[AFFIX_TABLE], lambda row: row['id'], set(range(1, 17)), '洗炼原词条')
    by_quality = combat.indexed_rows(tables[QUALITY_TABLE], lambda row: row['quality'], set(range(5)), '洗炼品质')
    if tables['game_refine_values'] or tables['game_attribute_pipeline'] or any(
            row['sample_weight'] is not None or row['value_curve_key'] is not None for row in rows.values()):
        raise ValueError('词条权重、阶段基值或属性管线已有正式记录，须重新核对候选来源')
    for row in rows.values():
        sources.append(forge.source(row, 'unverified', AFFIX_TABLE))
    qualities, nominal_pools = [], {}
    for quality, row in sorted(by_quality.items()):
        sources.append(forge.source(row, 'unverified', QUALITY_TABLE))
        slots, stone_cost = row['slot_count'], row['stones_per_attempt']
        original_pool = sum(value['minimum_quality'] <= quality for value in rows.values())
        if (row['allow_duplicate_attr'] is not None or original_pool != row['pool_size']
                or slots != quality or stone_cost != (1 << quality if quality else 0)):
            raise ValueError('原词条池、槽数、材料数量或正式重复规则已变化')
        nominal_pools[str(quality)] = original_pool
        if quality in enabled:
            qualities.append({'quality': quality, 'slot_count': slots, 'stone_cost': stone_cost, 'allow_duplicate_attr': False})
    mining.fields(entered, {str(index) for index in IMPLEMENTED}, '隔离词条装配')
    affixes = []
    for index, key in sorted(IMPLEMENTED.items()):
        row = rows[index]
        if row['attr_key'] != key:
            raise ValueError('原词条身份不再对应当前战斗管线')
        supplied = entered[str(index)]
        stages = {str(stage) for quality, stage in target_stages if quality >= row['minimum_quality']}
        mining.fields(supplied, {'weight', 'base_values_u'}, '词条候选参数')
        base = supplied['base_values_u']
        mining.fields(base, stages, '按原装备阶段声明的词条基值')
        affixes.append({'id': index, 'key': key, 'name': row['display_name'], 'min_quality': row['minimum_quality'],
                        'weight': mining.integer(supplied['weight'], 2**32-1, '候选词条权重'),
                        'base_values_u': {stage: mining.integer(base[stage], 2**31-1, '候选词条固定基值')
                                          for stage in sorted(stages, key=int)}})
    if sum(row['weight'] for row in affixes) > 2**32-1:
        raise ValueError('候选词条总权重超出抽样范围')
    return qualities, affixes, nominal_pools


def compile_recycle(tables, server, item, sources):
    """使用已查证的候选石率，只接受仍存在的普通怪原装备池。"""
    values = {}
    for key, status in (('refine_stone_recycle_probability', 'candidate'), ('refine_stone_recycle_quantity', 'candidate'),
                        ('ordinary_quality_recycle_stone', 'confirmed')):
        row = forge.unique(tables[mining.TABLE_RULES], lambda entry: entry['rule_key'] == key, key)
        sources.append(forge.source(row, status, mining.TABLE_RULES))
        values[key] = row['rule_value']
    formula = values['refine_stone_recycle_probability']
    match = re.fullmatch(r'min\(1,(0\.[0-9]+)\*quality\*\(stage\+1\)\)', formula) if isinstance(formula, str) else None
    quantity = values['refine_stone_recycle_quantity']
    if not match or type(quantity) is not int or quantity != 1 or values['ordinary_quality_recycle_stone'] is not False:
        raise ValueError('原分解洗炼石公式、产量或品质资格已改变')
    source_rows = tables[RECYCLE_TABLE]
    if not source_rows or any(row['broken_gold'] != 0 or row['stone_probability_ppm'] is not None
                              or row['broken_byproducts'] is not None or row['rewards_combinable'] is not None for row in source_rows):
        raise ValueError('副产物或组合规则已有正式数据，须重审隔离分解')
    sources.extend(forge.source(row, 'candidate', RECYCLE_TABLE) for row in source_rows)
    item_id, quality, stage = item['id'], item['drop_quality'], item['equipment_stage']
    ordinary = {monster['drop_pool_id'] for monster in server['monsters']}
    if not any(row['item_id'] == item_id and row['quantity_min'] == row['quantity_max'] == 1
               for pool in server['ordinary_drop_pools'] if pool['id'] in ordinary for row in pool['equipment']):
        raise ValueError('隔离可产石装备没有实际普通怪物单件掉落来源')
    amount = min(Decimal(1), Decimal(match[1]) * quality * (stage + 1)) * MILLION
    if not 0 < amount <= MILLION or amount != int(amount):
        raise ValueError('隔离分解概率不能用百万分整数无损表示')
    return {'item_id': item_id, 'quality': quality, 'stage': stage, 'source_kind': 'monster_drop',
            'probability_ppm': int(amount), 'quantity': quantity}


def project(server, public, tables, assembly):
    """只增加隔离洗炼和原石身份，保持原商店、掉落、图纸及油投影。"""
    mining.fields(assembly, {'stone_item_id', 'max_stack', 'recycle_item_id', 'target_item_ids', 'affix_candidates'}, '隔离洗炼装配')
    if (KEY in server or KEY in public or server.get('rule_version') != public.get('rule_version')
            or not all(base.get('development_only') is True and base.get('runtime_ready') is False
                       for base in (server, public)) or enhancement.KEY not in server
            or server[enhancement.KEY] != public.get(enhancement.KEY)):
        raise ValueError('洗炼必须加在同规则且已核对的部位强化双端目录')
    stone = mining.integer(assembly['stone_item_id'], 2**32-1, '隔离洗炼石编号')
    stack = mining.integer(assembly['max_stack'], 2**31-1, '隔离洗炼石堆叠上限')
    occupied = {row['id'] for row in server['items']} | {row['item_id'] for row in public['items']}
    if stone in occupied or any(row['id'] == stone or row['item_key'] == 'refine_stone' for row in tables['game_items']):
        raise ValueError('隔离洗炼石不能覆盖原目录物品或正式表的新身份')
    target_ids = assembly['target_item_ids']
    if type(target_ids) is not list or not target_ids or any(type(value) is not int for value in target_ids) or len(set(target_ids)) != len(target_ids):
        raise ValueError('隔离洗炼须列出唯一原装备模板')
    recycle_id = mining.integer(assembly['recycle_item_id'], 2**32-1, '隔离分解原装备编号')
    input_items = {item_id: forge.unique(server['items'], lambda row: row['id'] == item_id, '原洗炼装备模板') for item_id in target_ids}
    if recycle_id not in input_items or any(not item.get('is_equipment') or not item.get('original_durability_u')
                                           or not item.get('drop_quality') for item in input_items.values()):
        raise ValueError('隔离洗炼和分解须使用原有耐久的非普通装备')
    sources = []
    targets, stages = [], set()
    for item_id, item in sorted(input_items.items()):
        quality, stage = item['drop_quality'], item['equipment_stage']
        forge.unique(public['items'], lambda row: row['item_id'] == item_id and row['kind'] == 'equipment'
                     and row['quality'] == quality and row.get('equipment_stage', 0) == stage, '原公开洗炼装备')
        targets.append({'item_id': item_id, 'quality': quality})
        stages.add((quality, stage))
    recycle_item = forge.unique(tables['game_items'], lambda row: row['id'] == recycle_id
                                and row['quality'] == input_items[recycle_id]['drop_quality'], '工作簿分解原装备')
    sources.append(forge.source(recycle_item, 'candidate', 'game_items'))
    tiers, costs, prices = compile_prices(tables, sources)
    qualities, affixes, pools = compile_pool(tables, assembly['affix_candidates'], {row['quality'] for row in targets}, stages, sources)
    recycle = compile_recycle(tables, server, input_items[recycle_id], sources)
    rules = {'stone_item_id': stone, 'targets': targets, 'qualities': qualities, 'affixes': affixes,
             'tiers': tiers, 'gold_bands': prices, 'lock_costs': costs, 'recycle': recycle}
    material = {'id': stone, 'name': '洗炼石', 'stackable': True, 'max_stack': stack, 'equipment_refine_stone': True}
    display = {'item_id': stone, 'name': material['name'], 'kind': 'refine_stone', 'allowed_jobs': [0, 1, 2], 'use_enabled': False}
    evidence = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': sources,
                'nominal_affix_pool_sizes': pools, 'unverified_assembly': assembly,
                'combat_affix_scope': 'fixed_tenths', 'duplicate_scope': 'isolated_sampling_no_duplicate',
                'recycle_scope': 'manual_durable_ordinary_drop_zero_current_luck',
                'reward_combination_status': 'unresolved', 'broken_byproducts_status': 'unresolved'}
    return ({**server, KEY: rules, 'items': [*server['items'], material],
             '_source_evidence': {**server['_source_evidence'], KEY: evidence}},
            {**public, KEY: rules, 'items': [*public['items'], display]})


def main(argv=None):
    """对照唯一工作簿与已验收前置投影生成或逐字节核对双端文件。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节核对现有隔离候选')
    args = parser.parse_args(argv)
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('输入与输出路径须互相独立')
    cfg = mining.config
    tables, errors, _, workbooks = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    workbook = cfg.sha256(workbooks['gameplay'])
    prior = cfg.load_json(args.base_fixture)
    source = prior.get('_source_evidence', {}).get(enhancement.KEY, {})
    if source.get('workbook_sha256') != workbook or source.get('compiler_sha256') != cfg.sha256(Path(enhancement.__file__)):
        raise ValueError('保留的装备部位强化前置须由同一玩法工作簿与现行导出器生成')
    server, public = project(prior, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           base_fixture_sha256=cfg.sha256(args.base_fixture), base_public_sha256=cfg.sha256(args.base_public),
                                           assembly_sha256=cfg.sha256(args.assembly))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'装备洗炼快照与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
