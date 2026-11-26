#!/usr/bin/env python3
"""从唯一玩法工作簿投影手动装备分解的祝福油渠道。"""

import argparse
import json
from pathlib import Path
import re

import prepare_combat_luck as combat
import prepare_forge as forge
import prepare_luck_oil as oil
import prepare_mining as mining


RECYCLE_TABLE = 'game_recycle_values'


def compile_recycle_terms(tables):
    """提取精确分母并核对每件至多一瓶、原绑定、破损零金币。"""
    sources = []
    values = {}
    for key in ('oil_output_quantity', 'oil_recycle_probability_formula', 'oil_recycle_inherits_binding'):
        row = forge.unique(tables[combat.RULE_TABLE], lambda candidate: candidate['rule_key'] == key, key)
        sources.append(forge.source(row, 'confirmed', combat.RULE_TABLE))
        values[key] = row['rule_value']
    formula = values['oil_recycle_probability_formula']
    match = re.fullmatch(r'min\(1,luck/([1-9][0-9]*)\)', formula) if isinstance(formula, str) else None
    if (not match or type(values['oil_output_quantity']) is not int or values['oil_output_quantity'] != 1
            or values['oil_recycle_inherits_binding'] is not True):
        raise ValueError('分解祝福油的概率、单瓶数量或绑定规则已变化')
    denominator = mining.integer(int(match.group(1)), 2**32 - 1, '祝福油抽样分母')
    known = tables[RECYCLE_TABLE]
    if not known or any(type(row['broken_gold']) is not int or row['broken_gold'] != 0
                        for row in known):
        raise ValueError('装备分解表缺少破损零金币规则')
    if any(row['broken_byproducts'] is not None or row['rewards_combinable'] is not None for row in known):
        raise ValueError('装备分解副产物已有明细，须按新的正式规则复核投影')
    sources.extend(forge.source(row, 'candidate', RECYCLE_TABLE) for row in known)
    return {'denominator': denominator}, sources


def project(server, public, tables):
    """继承已验证的祝福油定义和原手动分解价，只开放正耐久原装备产油。"""
    oil_rule, oil_ui = server.get('luck_oil'), public.get('luck_oil')
    if (not oil_rule or not oil_ui or oil_rule['item_id'] != oil_ui['item_id']
            or not all(base.get('development_only') is True and base.get('runtime_ready') is False for base in (server, public))
            or server.get('rule_version') != public.get('rule_version') or 'equipment_recycle_oil' in server
            or 'equipment_recycle_oil' in public):
        raise ValueError('装备分解出油须继承同版本且尚未开放此渠道的祝福油候选')
    item_id = oil_rule['item_id']
    item = forge.unique(server['items'], lambda row: row['id'] == item_id, '原祝福油定义')
    forge.unique(public['items'], lambda row: row['item_id'] == item_id and row['kind'] == 'luck_oil', '公开祝福油定义')
    if item.get('equipment_recycle_material') or item.get('is_equipment') or item.get('stackable') is not True:
        raise ValueError('原祝福油缺少独立、可堆叠的材料身份')
    terms, sources = compile_recycle_terms(tables)
    items = [{**row, 'equipment_recycle_material': True} if row is item else row for row in server['items']]
    prior = server['_source_evidence']
    evidence = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': sources,
                'operation_scope': 'manual_durable_equipment', 'broken_byproducts_status': 'unresolved',
                'reward_combination_status': 'unresolved', 'gold_distribution_status': 'original_development_candidate'}
    return ({**server, 'items': items, 'equipment_recycle_oil': terms,
             '_source_evidence': {**prior, 'luck_oil': {**prior['luck_oil'], 'acquisition_status': 'manual_recycle_candidate'},
                                  'equipment_recycle_oil': evidence}},
            {**public, 'equipment_recycle_oil': terms})


def main(argv=None):
    """把工作簿和通过核对的前置候选固定为本轮两个可复验文件。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for field in ('oil-fixture', 'oil-public', 'fixture-out', 'public-out'):
        parser.add_argument('--' + field, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节核对既有输出')
    args = parser.parse_args(argv)
    if len({args.oil_fixture, args.oil_public, args.fixture_out, args.public_out}) != 4:
        parser.error('前置和输出文件必须互不相同')
    cfg = mining.config
    tables, errors, _, workbooks = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    workbook = cfg.sha256(workbooks['gameplay'])
    server, public = cfg.load_json(args.oil_fixture), cfg.load_json(args.oil_public)
    origin = server.get('_source_evidence', {}).get('luck_oil', {})
    if origin.get('workbook_sha256') != workbook or origin.get('compiler_sha256') != cfg.sha256(Path(oil.__file__)):
        raise ValueError('前置祝福油候选和当前工作簿或导出器不同')
    server, public = project(server, public, tables)
    server['_source_evidence']['equipment_recycle_oil'].update(
        workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
        oil_fixture_sha256=cfg.sha256(args.oil_fixture), oil_public_sha256=cfg.sha256(args.oil_public))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'分解出油快照与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
