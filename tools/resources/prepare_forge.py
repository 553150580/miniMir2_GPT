#!/usr/bin/env python3
"""以唯一玩法工作簿出具图纸打造的隔离服务端夹具和公开目录。"""

import argparse
from copy import deepcopy
import json
from pathlib import Path

import prepare_mining as mining


ROOT = mining.ROOT
CANDIDATE_RULES = {
    'craft_success_probability': 'average_purity/100',
    'craft_failure_consumes': ['black_iron', 'gold'],
    'craft_failure_keeps_blueprint': True,
    'craft_fail_pity': False,
}
CONFIRMED_RULES = {
    't1_crafting_open': False,
    'craft_blueprint_determines': ['quality', 'level', 'part'],
    'craft_success_consumes': ['blueprint', 'black_iron', 'gold'],
}
PRICE_KEYS = {2: 'rare_craft_gold', 3: 'epic_craft_gold', 4: 'legendary_craft_gold'}


def unique(rows, predicate, label):
    """必须找到唯一来源，重复输入不能被覆盖。"""
    matches = [row for row in rows if predicate(row)]
    if len(matches) != 1:
        raise ValueError(f'找不到唯一的{label}')
    return matches[0]


def source(row, status, table):
    """保留来源编号、状态和策划行号，用于复核隔离报价来源。"""
    if row['record_status'] != status or not row['source_ref']:
        raise ValueError(f'{table}缺少预期的来源状态或出处：{row["id"]}')
    return {'table_id': table, **{key: row[key] for key in ('id', 'record_status', 'source_ref')}}


def compile_terms(tables, level, quality, slot):
    """量化参数来自原表，计算时保持整数金币和百万分率。"""
    if tables['game_craft_recipes']:
        raise ValueError('已有正式打造配方条目，须按实际来源更新隔离投影')
    rules = tables[mining.TABLE_RULES]
    evidence = []

    def rule(key, status):
        row = unique(rules, lambda item: item['rule_key'] == key, f'工坊规则 {key}')
        evidence.append(source(row, status, mining.TABLE_RULES))
        return row['rule_value']

    for status, expected in (('confirmed', CONFIRMED_RULES), ('candidate', CANDIDATE_RULES)):
        for key, value in expected.items():
            actual = rule(key, status)
            if type(actual) is not type(value) or actual != value:
                raise ValueError(f'工坊规则已变化，须同步打造逻辑：{key}')
    levels = rule('craft_output_required_levels', 'confirmed')
    if type(levels) is not list or any(type(value) is not int for value in levels) or level not in levels:
        raise ValueError('图纸产出等级不在工作簿开放档内')
    threshold = mining.integer(rule('craft_guaranteed_purity_threshold', 'candidate'), 100, '打造必成纯度')
    price_table = 'game_level_band_economy'
    price_row = unique(tables[price_table], lambda row: row['level_min'] <= level and
                       (row['level_max'] is None or level <= row['level_max']), '打造金币等级档')
    evidence.append(source(price_row, 'candidate', price_table))
    if quality not in PRICE_KEYS:
        raise ValueError('该品质没有可引用的隔离金币候选')
    gold = mining.integer(price_row[PRICE_KEYS[quality]], 2**63 - 1, '单次打造金币')
    if slot == 0:
        multiplier = mining.integer(rule('craft_weapon_gold_multiplier_bp', 'candidate'), 2**32 - 1, '武器金币万分倍率')
        gold, remainder = divmod(gold * multiplier, 10000)
        if remainder:
            raise ValueError('武器打造费用缺少非整比价的取整规则')
        mining.integer(gold, 2**63 - 1, '武器打造金币')
    luck_table = 'game_luck_initial'
    luck = unique(tables[luck_table], lambda row: row['source_key'] == 'craft_success', '打造成功幸运规则')
    evidence.append(source(luck, 'candidate', luck_table))
    if luck['fixed_initial_value'] is not None:
        raise ValueError('打造初始幸运不应为固定值')
    weights = luck['initial_value_weights']
    mining.fields(weights, {'1', '2', '3'}, '打造初始幸运权重')
    result = {'guaranteed_purity': threshold, 'luck_ppm': mining.integer(luck['trigger_ppm'], 10**6, '初始幸运概率'),
              'luck_weights': [mining.integer(weights[str(value)], 2**32 - 1, '初始幸运权重') for value in range(1, 4)]}
    mining.integer(sum(result['luck_weights']), 2**32 - 1, '初始幸运总权重')
    return result, gold, evidence


def attach_blueprint(server, template_id, pending_limit, blueprint_id):
    """将候选图纸交给原个人首领奖励，已有胜利附件不能被覆盖。"""
    primary = server.get('personal_boss')
    variants = server.get('personal_boss_variants', [])
    bosses = ([primary] if primary else []) + variants
    original = unique(bosses, lambda row: row.get('template_id') == template_id and row.get('enabled') is True, '图纸的个人首领模板')
    if original.get('item_reward') is not None:
        raise ValueError('候选个人首领已有胜利附件，不能覆盖原奖励')
    modified = {**original, 'item_reward': {'pending_limit': pending_limit, 'items': [{'item_id': blueprint_id, 'quantity': 1}]}}
    if original is primary:
        return {'personal_boss': modified}
    return {'personal_boss_variants': [modified if row is original else row for row in variants]}


def project(server, public, tables, assembly):
    """在已装配矿区上增加一条明确的图纸自然来源和一份隔离配方。"""
    mining.fields(assembly, {'blueprint_item_id', 'blueprint_name', 'output_item_id', 'output_name', 'output_template_item_id',
                            'required_level', 'quality', 'ore_count', 'personal_boss_template_id', 'reward_pending_limit'}, '隔离打造组装')
    if (not all(base.get('development_only') is True and base.get('runtime_ready') is False for base in (server, public)) or
            server.get('rule_version') != public.get('rule_version') or 'mining' not in server or 'forge' in server or 'forge_recipes' in public):
        raise ValueError('隔离打造须源于同规则的完整矿区装配与公开目录')
    blueprint_id = mining.integer(assembly['blueprint_item_id'], 2**32 - 1, '图纸编号')
    output_id = mining.integer(assembly['output_item_id'], 2**32 - 1, '成品编号')
    occupied = {row['id'] for row in server['items']} | {row['item_id'] for row in public['items']}
    if blueprint_id == output_id or {blueprint_id, output_id} & occupied:
        raise ValueError('图纸与成品编号重复或覆盖了原目录')
    blueprint_name, output_name = assembly['blueprint_name'], assembly['output_name']
    if any(type(value) is not str or not value.strip() for value in (blueprint_name, output_name)):
        raise ValueError('图纸和成品须有可显示名称')
    template_id = mining.integer(assembly['output_template_item_id'], 2**32 - 1, '隔离成品基础模板')
    template = unique(server['items'], lambda row: row['id'] == template_id and row.get('is_equipment') is True, '基础装备样本')
    public_template = unique(public['items'], lambda row: row['item_id'] == template_id and row['kind'] == 'equipment', '公开基础装备样本')
    slot = template['equipment_slot']
    if (slot, template['allowed_jobs']) != (public_template['equip_slot'], public_template['allowed_jobs']):
        raise ValueError('隔离成品的基础部位或职业与公开装备样本不一致')
    level = mining.integer(assembly['required_level'], 50, '隔离成品等级')
    quality = mining.integer(assembly['quality'], 4, '隔离成品品质')
    stage = (level - 1) // 10
    terms, gold, evidence = compile_terms(tables, level, quality, slot)
    ore = unique(server['mining']['ores'], lambda row: row.get('key') == 'black_iron', '当前黑铁矿模板')
    ore_id = mining.integer(ore['item_id'], 2**32 - 1, '黑铁矿编号')
    unique(public['items'], lambda row: row['item_id'] == ore_id and row['kind'] == 'mining_ore', '公开黑铁矿模板')
    ore_count = mining.integer(assembly['ore_count'], 8, '隔离耗用黑铁块数')
    recipe = {'blueprint_item_id': blueprint_id, 'output_item_id': output_id, 'ore_count': ore_count, 'quality': quality}
    boss_id = mining.integer(assembly['personal_boss_template_id'], 2**32 - 1, '图纸的个人首领模板编号')
    pending_limit = mining.integer(assembly['reward_pending_limit'], 20, '隔离首领待领取容量')
    boss_updates = attach_blueprint(server, boss_id, pending_limit, blueprint_id)

    server_blueprint = {'id': blueprint_id, 'name': blueprint_name, 'max_stack': 1, 'drop_quality': quality}
    server_output = {**template, 'id': output_id, 'name': output_name, 'required_level': level, 'drop_quality': quality,
                     'equipment_stage': stage, 'max_stack': 1, 'recycle_gold': 0}
    public_blueprint = {'item_id': blueprint_id, 'name': blueprint_name, 'kind': 'forge_blueprint', 'quality': quality,
                        'allowed_jobs': [0, 1, 2], 'use_enabled': False}
    public_output = {**deepcopy(public_template), 'item_id': output_id, 'name': output_name, 'required_level': level,
                     'quality': quality, 'equipment_stage': stage, 'recycle_gold': 0}
    provenance = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence,
                  'recipe_table_status': 'no_rows', 'ore_count_status': 'historical', 'ore_count_source': '最终策划案.md:869-871；5.12',
                  'unverified_assembly': dict(assembly)}
    return ({**server, **boss_updates, 'items': [*server['items'], server_blueprint, server_output],
             'forge': {**terms, 'ore_item_id': ore_id, 'recipes': [{**recipe, 'gold': gold}]},
             '_source_evidence': {**server['_source_evidence'], 'forge': provenance}},
            {**public, 'items': [*public['items'], public_blueprint, public_output], 'forge_recipes': [{**recipe, 'ore_item_id': ore_id}]})


def main(argv=None):
    """从两个隔离组装文件生成同规则服务端与公开快照。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assembly', required=True, type=mining.work_path)
    parser.add_argument('--mining-assembly', required=True, type=mining.work_path)
    parser.add_argument('--fixture-out', required=True, type=mining.work_path)
    parser.add_argument('--public-out', required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节复核已经生成的隔离快照')
    args = parser.parse_args(argv)
    if len({args.assembly, args.mining_assembly, args.fixture_out, args.public_out}) != 4:
        parser.error('两份组装和两个输出须使用不同文件')
    cfg = mining.config
    tables, errors, _, workbooks = cfg.collect(cfg.load_json(ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    server, public = mining.project(cfg.load_json(mining.BASE_FIXTURE), cfg.load_json(mining.BASE_PUBLIC), tables, cfg.load_json(args.mining_assembly))
    sources = {'workbook_sha256': cfg.sha256(workbooks['gameplay']), 'base_fixture_sha256': cfg.sha256(mining.BASE_FIXTURE),
               'public_base_sha256': cfg.sha256(mining.BASE_PUBLIC)}
    mining_evidence = {'assembly_sha256': cfg.sha256(args.mining_assembly), 'compiler_sha256': cfg.sha256(Path(mining.__file__))}
    server['_source_evidence']['mining'].update(sources, **mining_evidence)
    server, public = project(server, public, tables, cfg.load_json(args.assembly))
    server['_source_evidence']['forge'].update(sources, assembly_sha256=cfg.sha256(args.assembly), compiler_sha256=cfg.sha256(Path(__file__)),
                                               mining_assembly_sha256=mining_evidence['assembly_sha256'], mining_compiler_sha256=mining_evidence['compiler_sha256'])
    for path, output in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(output, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'隔离打造快照与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
