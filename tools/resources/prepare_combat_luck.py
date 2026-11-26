#!/usr/bin/env python3
"""从唯一玩法工作簿生成幸运战斗候选，保留默认规则和原公开目录。"""

import argparse
import json
from pathlib import Path

import prepare_forge as forge
import prepare_mining as mining


CONTRIBUTION_TABLE = 'game_luck_contribution'
SOURCE_TABLE = 'game_luck_sources'
RULE_TABLE = 'game_luck_rules'
INITIAL_TABLE = 'game_luck_initial'
OTHER_PARTS = {'helmet', 'necklace', 'armor', 'bracelet_left', 'bracelet_right', 'ring_left', 'ring_right', 'belt', 'boots'}
RULES = {
    'attack_probability_cap_bp': 10_000,
    'order': ['sum_valid_contributions', 'clamp_to_100_percent', 'subtract_target_resistance', 'clamp_to_0_100_percent'],
    'proc_result': 'use_maximum_of_original_attack_interval',
    'affects_drop_rate': False,
    'bypasses_hit_or_defence': False,
    'adds_extra_damage_event': False,
    'medal_contributes_luck': False,
}


def bounded_integer(value, lower, upper, label):
    """拒绝字段缺失、布尔冒充数值和定点范围外的数值。"""
    if type(value) is not int or not lower <= value <= upper:
        raise ValueError(f'{label}必须是 {lower} 至 {upper} 的整数')
    return value


def indexed_rows(rows, key, expected, label):
    """明确核对完整性和重复行，防止同档参数覆盖旧值。"""
    result = {}
    for row in rows:
        index = key(row)
        if index not in expected or index in result:
            raise ValueError(f'{label}含未知或重复档位：{index}')
        result[index] = row
    if result.keys() != expected:
        raise ValueError(f'{label}档位不完整')
    return result


def compile_combat_rules(tables):
    """投影当前已实现的分档、叠加顺序与装备掉落初始幸运。"""
    evidence = []
    for key, expected in RULES.items():
        row = forge.unique(tables[RULE_TABLE], lambda item: item['rule_key'] == key, f'幸运规则 {key}')
        evidence.append(forge.source(row, 'confirmed', RULE_TABLE))
        if type(row['rule_value']) is not type(expected) or row['rule_value'] != expected:
            raise ValueError(f'幸运规则 {key} 已变化，须同步战斗实现')

    sources = indexed_rows(tables[SOURCE_TABLE], lambda row: row['source_key'], OTHER_PARTS | {'weapon', 'wing'}, '幸运贡献位')
    for key, row in sources.items():
        evidence.append(forge.source(row, 'confirmed', SOURCE_TABLE))
        oil_allowed = key in ('weapon', 'wing')
        group = key if oil_allowed else 'other_equipment'
        activation = 'unique_active_wing_id' if key == 'wing' else 'valid_equipped_item'
        if ((row['maximum_luck'], row['contribution_group'], row['activation_requirement']) != (9 if oil_allowed else 3, group, activation)
                or row['oil_allowed'] is not oil_allowed):
            raise ValueError(f'幸运贡献位 {key} 已变化，须同步属性来源')

    sizes = {'weapon': 10, 'wing': 10, 'other_equipment': 4}
    tiers = indexed_rows(tables[CONTRIBUTION_TABLE], lambda row: (row['source_group'], row['luck']),
                         {(group, luck) for group, size in sizes.items() for luck in range(size)}, '幸运贡献')
    contributions = {}
    for group, size in sizes.items():
        values = []
        for luck in range(size):
            row = tiers[group, luck]
            evidence.append(forge.source(row, 'confirmed', CONTRIBUTION_TABLE))
            values.append(bounded_integer(row['contribution_bp'], 0, 10_000, '单件幸运贡献') * 100)
        if values[0] != 0 or values != sorted(values):
            raise ValueError(f'{group} 幸运贡献须从零开始并单调不降')
        contributions[group] = values
    if contributions['weapon'] != contributions['wing']:
        raise ValueError('武器和翅膀幸运档位已分开，须同步采用各自表格')

    drop = forge.unique(tables[INITIAL_TABLE], lambda row: row['source_key'] == 'equipment_drop', '装备掉落初始幸运')
    evidence.append(forge.source(drop, 'candidate', INITIAL_TABLE))
    if drop['fixed_initial_value'] is not None:
        raise ValueError('装备掉落初始幸运不应为固定值')
    weights = drop['initial_value_weights']
    mining.fields(weights, {'1', '2', '3'}, '装备掉落初始幸运权重')
    values = [mining.integer(weights[str(index)], 2**32 - 1, '初始幸运权重') for index in range(1, 4)]
    mining.integer(sum(values), 2**32 - 1, '初始幸运总权重')
    return {'weapon_wing_ppm': contributions['weapon'], 'other_ppm': contributions['other_equipment'],
            'drop_ppm': bounded_integer(drop['trigger_ppm'], 0, 10**6, '装备掉落初始幸运概率'), 'initial_weights': values}, evidence


def resistances(entered, known, label):
    """隔离抵抗只允许显式指定已安装的稳定目标编号。"""
    if type(entered) is not dict or not entered.keys() <= {str(value) for value in known}:
        raise ValueError(f'{label}引用了未安装的目标')
    return {target: bounded_integer(value, 0, 10**6, label) for target, value in entered.items()}


def project(server, public, tables, assembly):
    """增加服务端规则；客户端从权威战斗属性接收概率，公开目录原样保留。"""
    mining.fields(assembly, {'monster_resistance_ppm', 'role_resistance_ppm'}, '隔离幸运组装')
    if (not all(base.get('development_only') is True and base.get('runtime_ready') is False for base in (server, public)) or
            server.get('rule_version') != public.get('rule_version') or 'combat_luck' in server or 'combat_luck' in public):
        raise ValueError('隔离幸运须源自同版本且未开放该功能的开发规则')
    terms, sources = compile_combat_rules(tables)
    terms['monster_resistance_ppm'] = resistances(assembly['monster_resistance_ppm'],
                                                  {m['id'] for m in server['monsters'] + server.get('challenge_monsters', [])}, '怪物幸运抵抗')
    terms['role_resistance_ppm'] = resistances(assembly['role_resistance_ppm'], {m['job_id'] for m in server['players']}, '角色幸运抵抗')
    provenance = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': sources,
                  'resistance_source_status': 'isolation_test_only', 'unverified_assembly': assembly}
    return {**server, 'combat_luck': terms, '_source_evidence': {**server['_source_evidence'], 'combat_luck': provenance}}, public


def compose_predecessors(server, public, tables, sources, mining_path, forge_path):
    """沿用已验证的矿区和打造输入，使同一候选可测试原打造装备的幸运。"""
    if mining_path is None:
        return server, public
    cfg = mining.config
    mining_evidence = {'assembly_sha256': cfg.sha256(mining_path), 'compiler_sha256': cfg.sha256(Path(mining.__file__))}
    server, public = mining.project(server, public, tables, cfg.load_json(mining_path))
    server['_source_evidence']['mining'].update(sources, **mining_evidence)
    server, public = forge.project(server, public, tables, cfg.load_json(forge_path))
    server['_source_evidence']['forge'].update(sources, assembly_sha256=cfg.sha256(forge_path), compiler_sha256=cfg.sha256(Path(forge.__file__)),
                                               mining_assembly_sha256=mining_evidence['assembly_sha256'], mining_compiler_sha256=mining_evidence['compiler_sha256'])
    return server, public


def main(argv=None):
    """保存可逐字节复核的正式配置之外的服务端和公开候选。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assembly', required=True, type=mining.work_path)
    parser.add_argument('--mining-assembly', type=mining.work_path)
    parser.add_argument('--forge-assembly', type=mining.work_path)
    parser.add_argument('--fixture-out', required=True, type=mining.work_path)
    parser.add_argument('--public-out', required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节核对已生成的候选')
    args = parser.parse_args(argv)
    if (args.mining_assembly is None) != (args.forge_assembly is None):
        parser.error('矿区和打造组装必须同时提供，或者同时省略')
    paths = [value for value in (args.assembly, args.mining_assembly, args.forge_assembly, args.fixture_out, args.public_out) if value is not None]
    if len(set(paths)) != len(paths):
        parser.error('各组装及输出文件必须互不相同')
    cfg = mining.config
    tables, errors, _, workbooks = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    sources = {'workbook_sha256': cfg.sha256(workbooks['gameplay']), 'base_fixture_sha256': cfg.sha256(mining.BASE_FIXTURE),
               'public_base_sha256': cfg.sha256(mining.BASE_PUBLIC)}
    server, public = compose_predecessors(cfg.load_json(mining.BASE_FIXTURE), cfg.load_json(mining.BASE_PUBLIC), tables, sources,
                                          args.mining_assembly, args.forge_assembly)
    server, public = project(server, public, tables, cfg.load_json(args.assembly))
    evidence = dict(sources, assembly_sha256=cfg.sha256(args.assembly), compiler_sha256=cfg.sha256(Path(__file__)))
    if args.forge_assembly is not None:
        evidence.update(forge_assembly_sha256=cfg.sha256(args.forge_assembly), mining_assembly_sha256=cfg.sha256(args.mining_assembly))
    server['_source_evidence']['combat_luck'].update(evidence)
    fixture_bytes = (json.dumps(server, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
    public_bytes = mining.BASE_PUBLIC.read_bytes() if args.forge_assembly is None else (json.dumps(public, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
    for path, expected in ((args.fixture_out, fixture_bytes), (args.public_out, public_bytes)):
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'隔离幸运候选与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
