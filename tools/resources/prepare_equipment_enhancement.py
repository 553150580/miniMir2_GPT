#!/usr/bin/env python3
"""由唯一玩法工作簿和隔离奖励装配生成部位强化的双端目录。"""

import argparse
import json
from pathlib import Path

import prepare_combat_luck as combat
import prepare_forge as forge
import prepare_mining as mining
import prepare_wing_oil as wing_oil


KEY = 'equipment_enhancement'
SLOT_TABLE = 'game_equipment_slots'
LEVEL_TABLE = 'game_enhancement_levels'
RUNTIME_SLOTS = ('weapon', 'armor', 'helmet', 'necklace', 'bracelet_left', 'bracelet_right',
                 'ring_left', 'ring_right', 'belt', 'boots')
MAX_LEVEL = 50
MAX_BONUS_BP = 1_500


def compile_rules(tables):
    """按运行槽位键匹配原部位，概率、费用和收益只读取原候选表。"""
    slots = combat.indexed_rows(tables[SLOT_TABLE], lambda row: row['slot_key'], set(RUNTIME_SLOTS), '装备部位')
    steps = combat.indexed_rows(tables[LEVEL_TABLE], lambda row: row['from_level'], set(range(MAX_LEVEL)), '部位强化等级')
    sources, runtime_slot_ids, levels = [], {}, []
    for slot, key in enumerate(RUNTIME_SLOTS):
        row = slots[key]
        if row['strengthenable'] is not True:
            raise ValueError(f'原装备部位 {key} 未开放强化')
        sources.append(forge.source(row, 'confirmed', SLOT_TABLE))
        runtime_slot_ids[str(slot)] = row['id']
    for current in range(MAX_LEVEL):
        row = steps[current]
        sources.append(forge.source(row, 'candidate', LEVEL_TABLE))
        if (type(row['from_level']) is not int or row['to_level'] != current + 1
                or row['stage'] != current // 10 + 1):
            raise ValueError(f'部位强化 {current} 级的进阶或阶段不连续')
        levels.append({'current_level': current,
                       'stone_cost': mining.integer(row['stone_cost'], 2**31 - 1, '强化石费用'),
                       'success_ppm': mining.integer(row['success_ppm'], 10**6, '强化成功率'),
                       'bonus_bp': mining.integer(row['base_attr_increment_bp'], MAX_BONUS_BP, '基础属性增量')})
    if sum(row['bonus_bp'] for row in levels) > MAX_BONUS_BP:
        raise ValueError('部位强化累计超出当前阶段候选预算')
    recycle = forge.unique(tables[mining.TABLE_RULES], lambda row: row['rule_key'] == 'equipment_recycle_enhance_stone',
                           '强化石分解规则')
    if recycle['rule_value'] is not False:
        raise ValueError('原装备分解规则不能产出强化石')
    sources.append(forge.source(recycle, 'confirmed', mining.TABLE_RULES))
    return levels, sources, runtime_slot_ids


def attach_reward(server, template_id, pending_limit, item_id, quantity):
    """接入已有的个人首领胜利附件，保留其他场次与已有奖励。"""
    primary = server.get('personal_boss')
    variants = server.get('personal_boss_variants', [])
    original = forge.unique(([primary] if primary else []) + variants,
                            lambda row: row.get('template_id') == template_id and row.get('enabled') is True,
                            '强化石的个人首领模板')
    if original.get('item_reward') is not None:
        raise ValueError('隔离强化不能覆盖个人首领原胜利附件')
    changed = {**original, 'item_reward': {'pending_limit': pending_limit,
                                          'items': [{'item_id': item_id, 'quantity': quantity}]}}
    if original is primary:
        return {'personal_boss': changed}
    return {'personal_boss_variants': [changed if row is original else row for row in variants]}


def project(server, public, tables, assembly):
    """保留现有资源和其他首领奖励，供隔离玩家通过原结算取得强化石。"""
    mining.fields(assembly, {'item_id', 'max_stack', 'personal_boss_template_id', 'reward_quantity', 'reward_pending_limit'},
                  '隔离部位强化装配')
    if (KEY in server or KEY in public or server.get('rule_version') != public.get('rule_version')
            or not all(base.get('development_only') is True and base.get('runtime_ready') is False
                       and base.get('wings', {}).get('allow_luck_oil') is True for base in (server, public))):
        raise ValueError('部位强化必须加在已有同规则隔离养成的双端目录上')
    item_id = mining.integer(assembly['item_id'], 2**32 - 1, '隔离强化石编号')
    stack = mining.integer(assembly['max_stack'], 2**31 - 1, '隔离强化石堆叠上限')
    quantity = mining.integer(assembly['reward_quantity'], stack, '个人首领强化石数量')
    pending = mining.integer(assembly['reward_pending_limit'], 20, '个人首领奖励待领容量')
    boss_id = mining.integer(assembly['personal_boss_template_id'], 2**32 - 1, '个人首领模板')
    occupied = {row['id'] for row in server['items']} | {row['item_id'] for row in public['items']}
    if item_id in occupied or any(row['id'] == item_id or row['item_key'] == 'enhance_stone' for row in tables['game_items']):
        raise ValueError('隔离强化石不能覆盖原目录物品或唯一工作簿的正式目录身份')
    boss = forge.unique(tables['game_boss_templates'], lambda row: row['id'] == boss_id and row['boss_kind'] == 'personal',
                        '工作簿个人首领模板')
    levels, sources, slot_ids = compile_rules(tables)
    rewards = attach_reward(server, boss_id, pending, item_id, quantity)
    rules = {'stone_item_id': item_id, 'levels': levels}
    material = {'id': item_id, 'name': '强化石', 'stackable': True, 'max_stack': stack}
    display = {'item_id': item_id, 'name': material['name'], 'kind': 'enhancement_stone', 'allowed_jobs': [0, 1, 2],
               'use_enabled': False}
    evidence = {'status': 'isolated_candidate', 'default_enabled': False,
                'record_sources': [*sources, forge.source(boss, 'candidate', 'game_boss_templates')],
                'runtime_slot_source_ids': slot_ids, 'unverified_assembly': dict(assembly),
                'bonus_arithmetic': 'sum_basis_points_then_truncate_once', 'craft_extra_base_included': False}
    return ({**server, **rewards, 'items': [*server['items'], material], KEY: rules,
             '_source_evidence': {**server['_source_evidence'], KEY: evidence}},
            {**public, 'items': [*public['items'], display], KEY: rules})


def main(argv=None):
    """从已核对的前置目录和唯一工作簿生成或逐字节检查双端候选。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节检查现有隔离目录')
    args = parser.parse_args(argv)
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('输入与输出路径须互相独立')
    cfg = mining.config
    tables, errors, _, workbooks = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    workbook = cfg.sha256(workbooks['gameplay'])
    source_server = cfg.load_json(args.base_fixture)
    predecessor = source_server.get('_source_evidence', {}).get('wing_oil', {})
    if predecessor.get('workbook_sha256') != workbook or predecessor.get('compiler_sha256') != cfg.sha256(Path(wing_oil.__file__)):
        raise ValueError('已验收的翅膀祝福油前置须来自同一玩法工作簿及现行导出器')
    server, public = project(source_server, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           base_fixture_sha256=cfg.sha256(args.base_fixture),
                                           base_public_sha256=cfg.sha256(args.base_public), assembly_sha256=cfg.sha256(args.assembly))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'部位强化快照与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
