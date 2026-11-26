#!/usr/bin/env python3
"""从唯一玩法工作簿投影翅膀结构和明确隔离的羽毛成长路径。"""

import argparse
import json
from pathlib import Path

import prepare_combat_luck as combat
import prepare_forge as forge
import prepare_mining as mining
import prepare_recycle_oil as recycle


RULES_TABLE = 'game_collection_rules'
RANKS_TABLE = 'game_wing_ranks'
WING_RULES = {
    'wing_unique_per_role': True,
    'wing_initial_luck': 0,
    'wing_max_luck': 9,
    'wing_trade_unequip_recycle_or_shared_storage': False,
    'wing_consumes_backpack_slot': False,
    'wing_strengthenable': False,
    'wing_rank_up_success_ppm': 10**6,
    'wing_id_survives_rank_or_skin_changes': True,
    'wing_visual_hidden_keeps_stats': True,
}


def compile_structure(tables):
    """校验唯一实例、独立位置、成长必成和正式逐阶数字的来源状态。"""
    rules = {key: forge.unique(tables[RULES_TABLE], lambda row: row['rule_key'] == key, key)
             for key in (*WING_RULES, 'wing_max_rank', 'wing_visual_evolution_ranks')}
    evidence = [forge.source(row, 'confirmed', RULES_TABLE) for row in rules.values()]
    for key, value in WING_RULES.items():
        published = rules[key]['rule_value']
        if type(published) is not type(value) or published != value:
            raise ValueError(f'翅膀固定规则须同步实现：{key}')
    limit = mining.integer(rules['wing_max_rank']['rule_value'], 2**32 - 1, '翅膀最高阶')
    tiers = rules['wing_visual_evolution_ranks']['rule_value']
    if (type(tiers) is not list or not tiers or tiers[0] != 1 or sorted(set(tiers)) != tiers
            or any(type(tier) is not int or not 1 <= tier <= limit for tier in tiers)):
        raise ValueError('翅膀外观须按已解锁的原阶数排序')
    ranks = combat.indexed_rows(tables[RANKS_TABLE], lambda row: row['rank'], set(range(1, limit + 1)), '翅膀逐阶表')
    for rank, row in ranks.items():
        evidence.append(forge.source(row, 'unverified', RANKS_TABLE))
        if row['appearance_evolves'] is not (rank in tiers) or row['feather_cost'] is not None or row['attribute_effects'] is not None:
            raise ValueError('正式翅膀逐阶表已有数值或外观变更，须重新核对')
    return limit, tiers, evidence


def project(server, public, tables, assembly):
    """把羽毛发给原世界首领有实际贡献的角色，保留原掉落时特权判绑。"""
    mining.fields(assembly, {'feather_item_id', 'feather_max_stack', 'rank_feathers', 'world_boss_template_id',
                            'world_boss_minimum_damage', 'world_boss_feathers'}, '隔离翅膀装配')
    if ('equipment_recycle_oil' not in server or 'wings' in server or 'wings' in public
            or server.get('rule_version') != public.get('rule_version')
            or not all(base.get('development_only') is True and base.get('runtime_ready') is False for base in (server, public))):
        raise ValueError('翅膀须从同一版本的已验收前置成长配置装配')
    limit, tiers, evidence = compile_structure(tables)
    item_id = mining.integer(assembly['feather_item_id'], 2**32 - 1, '原羽毛候选编号')
    max_stack = mining.integer(assembly['feather_max_stack'], 2**31 - 1, '原羽毛堆叠上限')
    occupied = {row['id'] for row in server['items']} | {row['item_id'] for row in public['items']}
    if item_id in occupied:
        raise ValueError('翅膀羽毛候选编号与原目录冲突')
    costs = assembly['rank_feathers']
    if type(costs) is not list or not 1 <= len(costs) <= limit:
        raise ValueError('隔离翅膀费用须从激活起提供连续阶段')
    costs = [mining.integer(cost, 2**63 - 1, '隔离逐阶羽毛') for cost in costs]
    wings = {'feather_item_id': item_id, 'max_rank': limit, 'appearance_tiers': tiers,
             'ranks': [{'rank': rank, 'feathers': cost} for rank, cost in enumerate(costs, 1)]}
    template = mining.integer(assembly['world_boss_template_id'], 2**32 - 1, '原世界首领模板')
    primary = server.get('world_boss')
    variants = server.get('world_boss_variants', [])
    target = forge.unique(([primary] if primary else []) + variants,
                          lambda row: row.get('template_id') == template and row.get('enabled') is True, '世界首领先验模板')
    if target.get('rewards') is not None:
        raise ValueError('该世界首领已有奖表，不能覆盖原配置')
    damage = mining.integer(assembly['world_boss_minimum_damage'], 2**63 - 1, '世界首领奖励最低有效伤害')
    quantity = mining.integer(assembly['world_boss_feathers'], max_stack, '隔离世界首领羽毛数量')
    reward = {'minimum_damage': damage, 'participation': [{'item_id': item_id, 'quantity': quantity}]}
    updated = {**target, 'rewards': reward}
    bosses = {'world_boss': updated} if target is primary else {
        'world_boss_variants': [updated if row is target else row for row in variants]}
    material = {'id': item_id, 'name': '羽毛', 'stackable': True, 'max_stack': max_stack}
    visible = {'item_id': item_id, 'name': material['name'], 'kind': 'wing_feather', 'allowed_jobs': [0, 1, 2], 'use_enabled': False}
    provenance = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence,
                  'rank_costs_status': 'isolation_test_only', 'attributes_status': 'unresolved',
                  'feather_supply_status': 'isolation_test_only', 'unverified_assembly': dict(assembly)}
    return ({**server, **bosses, 'items': [*server['items'], material], 'wings': wings,
             '_source_evidence': {**server['_source_evidence'], 'wings': provenance}},
            {**public, 'items': [*public['items'], visible], 'wings': wings})


def main(argv=None):
    """生成服务端及公开候选，按实际工作簿和原前置规则登记摘要。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('assembly', 'recycle-fixture', 'recycle-public', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节校验候选')
    args = parser.parse_args(argv)
    if len({args.assembly, args.recycle_fixture, args.recycle_public, args.fixture_out, args.public_out}) != 5:
        parser.error('本轮输入和输出文件须各自独立')
    cfg = mining.config
    tables, errors, _, workbooks = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    workbook = cfg.sha256(workbooks['gameplay'])
    server, public = cfg.load_json(args.recycle_fixture), cfg.load_json(args.recycle_public)
    prior = server.get('_source_evidence', {}).get('equipment_recycle_oil', {})
    if prior.get('workbook_sha256') != workbook or prior.get('compiler_sha256') != cfg.sha256(Path(recycle.__file__)):
        raise ValueError('翅膀前置回收候选必须使用同一工作簿及现行导出器')
    server, public = project(server, public, tables, cfg.load_json(args.assembly))
    server['_source_evidence']['wings'].update(
        workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)), assembly_sha256=cfg.sha256(args.assembly),
        recycle_fixture_sha256=cfg.sha256(args.recycle_fixture), recycle_public_sha256=cfg.sha256(args.recycle_public))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'翅膀快照与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
