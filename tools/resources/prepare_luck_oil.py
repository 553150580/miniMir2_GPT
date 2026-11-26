#!/usr/bin/env python3
"""从唯一玩法工作簿投影已确认的用油规则及隔离物品目录。"""

import argparse
import json
from pathlib import Path

import prepare_combat_luck as combat
import prepare_forge as forge
import prepare_mining as mining


STEP_TABLE = 'game_luck_oil_steps'
SEGMENTS = ('none', '3_to_6', '6_to_9')


def compile_steps(tables):
    """只接受原表逐级成功、原段回退、阈值后的下一瓶必成。"""
    records = combat.indexed_rows(tables[STEP_TABLE], lambda row: row['from_luck'], set(range(9)), '用油档位')
    capped = forge.unique(tables[combat.RULE_TABLE], lambda row: row['rule_key'] == 'oil_at_luck_9_consumption',
                          '幸运满级消耗')
    sources = [forge.source(capped, 'confirmed', combat.RULE_TABLE)]
    if type(capped['rule_value']) is not int or capped['rule_value'] != 0:
        raise ValueError('幸运满级应结束祝福油培养')
    steps = []
    for before, row in sorted(records.items()):
        sources.append(forge.source(row, 'confirmed', STEP_TABLE))
        segment = before // 3
        pity = row['threshold_used_bottles']
        failure = segment * 3 if segment else before
        if (row['target_luck'] != before + 1 or row['failure_luck'] != failure
                or row['pity_segment'] != SEGMENTS[segment] or row['pity_starts_after_threshold'] is not True
                or (segment == 0 and pity is not None)):
            raise ValueError(f'幸运 {before} 的逐级、回退或保底规则须同步实现')
        threshold = mining.integer(pity, 2**32 - 4, '保底累计瓶数') if segment else 0
        steps.append({'success_ppm': mining.integer(row['success_ppm'], 10**6, '用油成功率'),
                      'failure_luck': failure, 'pity_after': threshold})
    probabilities = [row['success_ppm'] for row in steps]
    thresholds = [row['pity_after'] for row in steps]
    if (probabilities[0] != 10**6 or probabilities != sorted(probabilities, reverse=True)
            or thresholds != [0] * 3 + [thresholds[3]] * 3 + [thresholds[6]] * 3
            or not 0 < thresholds[3] < thresholds[6]):
        raise ValueError('用油档位的概率和两段阈值不一致')
    return steps, sources


def project(server, public, tables, assembly):
    """追加隔离祝福油材料，维持默认配置、物品来源和经济渠道。"""
    mining.fields(assembly, {'item_id', 'max_stack'}, '隔离祝福油组装')
    if ('combat_luck' not in server or 'luck_oil' in server or 'luck_oil' in public
            or server.get('rule_version') != public.get('rule_version')
            or not all(base.get('development_only') is True and base.get('runtime_ready') is False for base in (server, public))):
        raise ValueError('祝福油须装配在同版本的隔离战斗幸运规则上')
    item_id = mining.integer(assembly['item_id'], 2**32 - 1, '隔离祝福油编号')
    stack = mining.integer(assembly['max_stack'], 2**31 - 1, '隔离祝福油堆叠')
    seen = {row['id'] for row in server['items']} | {row['item_id'] for row in public['items']}
    if item_id in seen:
        raise ValueError('隔离祝福油编号已被其他物品占用')
    steps, sources = compile_steps(tables)
    material = {'id': item_id, 'name': '祝福油', 'stackable': True, 'max_stack': stack}
    visible = {'item_id': item_id, 'name': material['name'], 'kind': 'luck_oil',
               'allowed_jobs': [0, 1, 2], 'use_enabled': False}
    evidence = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': sources,
                'item_definition_status': 'isolation_test_only', 'acquisition_status': 'not_enabled',
                'unverified_assembly': dict(assembly)}
    return ({**server, 'items': [*server['items'], material], 'luck_oil': {'item_id': item_id, 'steps': steps},
             '_source_evidence': {**server['_source_evidence'], 'luck_oil': evidence}},
            {**public, 'items': [*public['items'], visible], 'luck_oil': {'item_id': item_id}})


def main(argv=None):
    """从工作簿与明确隔离组装构建或复核对应的服务端和客户端文件。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('assembly', 'luck-assembly', 'mining-assembly', 'forge-assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节复核既有候选')
    args = parser.parse_args(argv)
    paths = (args.assembly, args.luck_assembly, args.mining_assembly, args.forge_assembly, args.fixture_out, args.public_out)
    if len(set(paths)) != len(paths):
        parser.error('各份组装和输出路径须互不相同')
    cfg = mining.config
    tables, errors, _, workbooks = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    sources = {'workbook_sha256': cfg.sha256(workbooks['gameplay']), 'base_fixture_sha256': cfg.sha256(mining.BASE_FIXTURE),
               'public_base_sha256': cfg.sha256(mining.BASE_PUBLIC)}
    server, public = combat.compose_predecessors(cfg.load_json(mining.BASE_FIXTURE), cfg.load_json(mining.BASE_PUBLIC),
                                                  tables, sources, args.mining_assembly, args.forge_assembly)
    server, public = combat.project(server, public, tables, cfg.load_json(args.luck_assembly))
    server['_source_evidence']['combat_luck'].update(sources, assembly_sha256=cfg.sha256(args.luck_assembly),
                                                   compiler_sha256=cfg.sha256(Path(combat.__file__)))
    server, public = project(server, public, tables, cfg.load_json(args.assembly))
    server['_source_evidence']['luck_oil'].update(sources, assembly_sha256=cfg.sha256(args.assembly),
                                                compiler_sha256=cfg.sha256(Path(__file__)))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'隔离祝福油快照与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
