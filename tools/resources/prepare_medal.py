#!/usr/bin/env python3
"""从唯一玩法工作簿校验固定规则，生成明确隔离的勋章和碎片来源。"""

import argparse
import json
from pathlib import Path

import prepare_combat_luck as combat
import prepare_equipment_refine as refine
import prepare_forge as forge
import prepare_mining as mining


KEY = 'medal'
RULES_TABLE = 'game_collection_rules'
LEVELS_TABLE = 'game_medal_levels'
BOSS_TABLE = 'game_boss_templates'
RULES = {
    'medal_unique': True,
    'medal_level_cap': 'actual_role_level',
    'medal_upgrade_success_ppm': 1_000_000,
    'medal_unequip_or_recycle': False,
    'medal_has_durability_or_weight': False,
}


def compile_rules(tables):
    """核对已定规则和逐级表缺口，出现正式费用后停止沿用隔离数值。"""
    evidence = []
    for name, expected in RULES.items():
        row = forge.unique(tables[RULES_TABLE], lambda value: value['rule_key'] == name, f'勋章规则 {name}')
        evidence.append(forge.source(row, 'confirmed', RULES_TABLE))
        value = row['rule_value']
        if type(value) is not type(expected) or value != expected:
            raise ValueError(f'勋章固定规则已变化：{name}')
    if tables[LEVELS_TABLE]:
        raise ValueError('正式勋章逐级表已有记录，须重新审定费用和属性来源')
    return evidence


def compile_levels(rows):
    """保留显式隔离的正费用和整数累计属性，按目标等级连续投影。"""
    if type(rows) is not list or not rows:
        raise ValueError('隔离勋章须从激活起配置连续等级')
    levels = []
    for index, row in enumerate(rows, 1):
        mining.fields(row, {'fragment_cost', 'main_attack', 'defense'}, '隔离勋章等级')
        cost = mining.integer(row['fragment_cost'], 2**31 - 1, '勋章碎片费用')
        main = combat.bounded_integer(row['main_attack'], 0, 2**31 - 1, '勋章累计职业主攻')
        defense = combat.bounded_integer(row['defense'], 0, 2**31 - 1, '勋章累计物防')
        if levels and (main < levels[-1]['main_attack'] or defense < levels[-1]['defense']):
            raise ValueError('勋章隔离档位的累计属性不得降低')
        levels.append({'level': index, 'fragment_cost': cost, 'main_attack': main, 'defense': defense})
    return levels


def append_world_boss_reward(server, tables, template, item_id, quantity, evidence):
    """在已验收的原合格参与奖中追加碎片，保留资格、排名和同场羽毛。"""
    source = forge.unique(tables[BOSS_TABLE], lambda row: row['id'] == template and row['boss_kind'] == 'world',
                          '世界首领工作簿模板')
    evidence.append(forge.source(source, 'candidate', BOSS_TABLE))
    if source['participation_reward_group_id'] is not None or source['rank_reward_group_id'] is not None:
        raise ValueError('首领已有正式奖表，须按实际来源配置勋章碎片')
    main, variants = server.get('world_boss'), server.get('world_boss_variants', [])
    target = forge.unique(([main] if main else []) + variants,
                          lambda row: row.get('template_id') == template and row.get('enabled') is True,
                          '已启用的世界首领模板')
    rewards = target.get('rewards')
    if type(rewards) is not dict or type(rewards.get('participation')) is not list or not rewards['participation']:
        raise ValueError('勋章隔离奖须继承已有的真实参与奖励')
    participation = rewards['participation']
    if any(row['item_id'] == item_id for row in participation):
        raise ValueError('世界首领原参与奖重复使用勋章碎片')
    updated = {**target, 'rewards': {**rewards, 'participation': [*participation, {'item_id': item_id, 'quantity': quantity}]}}
    if target is main:
        return {'world_boss': updated}
    return {'world_boss_variants': [updated if row is target else row for row in variants]}


def project(server, public, tables, assembly):
    """只增加隔离勋章目录和原世界首领参与奖，不修改两份前置输入。"""
    mining.fields(assembly, {'fragment_item_id', 'fragment_max_stack', 'world_boss_template_id', 'world_boss_fragments',
                            'levels'}, '勋章隔离装配')
    if (refine.KEY not in server or refine.KEY not in public or KEY in server or KEY in public
            or server.get('rule_version') != public.get('rule_version') or server.get('rule_version', 0) < 30
            or not all(base.get('development_only') is True and base.get('runtime_ready') is False for base in (server, public))):
        raise ValueError('勋章须从同规则下已验收的洗炼前置候选装配')
    evidence = compile_rules(tables)
    fragment = mining.integer(assembly['fragment_item_id'], 2**32 - 1, '隔离勋章碎片编号')
    max_stack = mining.integer(assembly['fragment_max_stack'], 2**31 - 1, '勋章碎片堆叠上限')
    if fragment in ({row['id'] for row in server['items']} | {row['item_id'] for row in public['items']}):
        raise ValueError('勋章碎片编号与原材料或装备冲突')
    quantity = mining.integer(assembly['world_boss_fragments'], max_stack, '世界首领参与碎片数量')
    template = mining.integer(assembly['world_boss_template_id'], 2**32 - 1, '原世界首领模板')
    bosses = append_world_boss_reward(server, tables, template, fragment, quantity, evidence)
    rules = {'fragment_item_id': fragment, 'levels': compile_levels(assembly['levels'])}
    material = {'id': fragment, 'name': '勋章碎片', 'stackable': True, 'max_stack': max_stack}
    visible = {'item_id': fragment, 'name': material['name'], 'kind': 'medal_fragment', 'allowed_jobs': [0, 1, 2], 'use_enabled': False}
    provenance = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence,
                  'level_costs_and_attributes_status': 'isolation_test_only', 'world_boss_supply_status': 'isolation_test_only',
                  'binding': 'original_world_boss_kill_time_privilege', 'unverified_assembly': dict(assembly)}
    return ({**server, **bosses, 'items': [*server['items'], material], KEY: rules,
             '_source_evidence': {**server['_source_evidence'], KEY: provenance}},
            {**public, 'items': [*public['items'], visible], KEY: rules})


def main(argv=None):
    """读取现行唯一工作簿与清晰的前置摘要，生成或核对双端隔离配置。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节核对双端隔离产物')
    args = parser.parse_args(argv)
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('输入与输出路径须互相独立')
    cfg = mining.config
    tables, errors, _, books = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    workbook = cfg.sha256(books['gameplay'])
    previous = cfg.load_json(args.base_fixture)
    previous_source = previous.get('_source_evidence', {}).get(refine.KEY, {})
    if previous_source.get('workbook_sha256') != workbook or previous_source.get('compiler_sha256') != cfg.sha256(Path(refine.__file__)):
        raise ValueError('勋章前置洗炼候选须由同一工作簿和现行导出器生成')
    server, public = project(previous, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           assembly_sha256=cfg.sha256(args.assembly),
                                           base_fixture_sha256=cfg.sha256(args.base_fixture), base_public_sha256=cfg.sha256(args.base_public))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'勋章快照与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
