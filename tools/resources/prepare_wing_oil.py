#!/usr/bin/env python3
"""在隔离翅膀候选上启用与武器同源的祝福油规则。"""

import argparse
import json
from pathlib import Path

import prepare_combat_luck as combat
import prepare_forge as forge
import prepare_luck_oil as oil
import prepare_mining as mining
import prepare_wings as wings


def project(server, public, tables):
    """确认原实例、同一用油分档与贡献表后，只开放已激活的翅膀。"""
    original = server.get('wings')
    if (not original or original != public.get('wings') or original.get('allow_luck_oil')
            or server.get('rule_version') != public.get('rule_version')
            or not all(base.get('development_only') is True and base.get('runtime_ready') is False for base in (server, public))):
        raise ValueError('翅膀用油必须继承同版本、未开启该能力的隔离成长配置')
    source = forge.unique(tables[combat.SOURCE_TABLE], lambda row: row['source_key'] == 'wing', '翅膀幸运来源')
    if (source['oil_allowed'] is not True or source['activation_requirement'] != 'unique_active_wing_id'
            or source['contribution_group'] != 'wing'):
        raise ValueError('翅膀用油需明确引用已经激活的唯一翅膀')
    steps, oil_sources = oil.compile_steps(tables)
    oil_rules = server.get('luck_oil', {})
    if steps != oil_rules.get('steps') or public.get('luck_oil') != {'item_id': oil_rules.get('item_id')}:
        raise ValueError('翅膀与武器须共享原九档用油、回退及硬保底')
    contribution = combat.indexed_rows(
        [row for row in tables[combat.CONTRIBUTION_TABLE] if row['source_group'] == 'wing'],
        lambda row: row['luck'], set(range(source['maximum_luck'] + 1)), '翅膀幸运贡献')
    ppm = [combat.bounded_integer(row['contribution_bp'], 0, 10_000, '翅膀幸运贡献') * 100
           for _, row in sorted(contribution.items())]
    if len(ppm) != len(steps) + 1 or ppm != server.get('combat_luck', {}).get('weapon_wing_ppm'):
        raise ValueError('翅膀面板须显示与服务端战斗相同的原贡献表')
    enabled = {**original, 'allow_luck_oil': True}
    evidence = [forge.source(source, 'confirmed', combat.SOURCE_TABLE), *oil_sources,
                *(forge.source(row, 'confirmed', combat.CONTRIBUTION_TABLE) for row in contribution.values())]
    provenance = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence}
    return ({**server, 'wings': enabled, '_source_evidence': {**server['_source_evidence'], 'wing_oil': provenance}},
            {**public, 'wings': {**enabled, 'luck_contribution_ppm': ppm}})


def main(argv=None):
    """绑定现有翅膀装配、唯一工作簿与新增公开能力的摘要。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('wing-fixture', 'wing-public', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节核对现有投影')
    args = parser.parse_args(argv)
    if len({args.wing_fixture, args.wing_public, args.fixture_out, args.public_out}) != 4:
        parser.error('输入与输出文件须各自独立')
    cfg = mining.config
    tables, errors, _, workbooks = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    workbook = cfg.sha256(workbooks['gameplay'])
    predecessor = cfg.load_json(args.wing_fixture)
    prior = predecessor.get('_source_evidence', {}).get('wings', {})
    if prior.get('workbook_sha256') != workbook or prior.get('compiler_sha256') != cfg.sha256(Path(wings.__file__)):
        raise ValueError('翅膀前置候选须由同一工作簿和现行导出器生成')
    server, public = project(predecessor, cfg.load_json(args.wing_public), tables)
    server['_source_evidence']['wing_oil'].update(
        workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
        wing_fixture_sha256=cfg.sha256(args.wing_fixture), wing_public_sha256=cfg.sha256(args.wing_public))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'翅膀用油快照与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
