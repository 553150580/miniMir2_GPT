#!/usr/bin/env python3
"""从唯一工作簿核对炼体公式，投影隔离的生命与魔法两系。"""

import argparse
import json
from pathlib import Path

import prepare_forge as forge
import prepare_mining as mining
import prepare_rebirth as rebirth


KEY = 'body_training'
TRACK_TABLE = 'game_body_tracks'
STEP_TABLE = 'game_body_level_steps'
BUDGET_TABLE = 'game_attribute_budgets'
PIPELINE_TABLE = 'game_attribute_pipeline'


def project(server, public, tables, assembly):
    """保留原工作簿费用和概率，样本额度仅出现在独立候选内。"""
    mining.fields(assembly, {'max_rank', 'health_bonus_per_rank', 'mana_bonus_per_rank',
                             'health_cap_bp', 'mana_cap_bp'}, '炼体隔离样本')
    if (server.get('rule_version') != public.get('rule_version') or server.get('rule_version', 0) < 30
            or any(KEY in base or rebirth.KEY not in base or base.get('development_only') is not True
                   or base.get('runtime_ready') is not False for base in (server, public))):
        raise ValueError('炼体使用已验收的同规则首转候选')
    if tables[PIPELINE_TABLE]:
        raise ValueError('已有正式属性结算表，须重新核对炼体的接入与正式上限')
    cap = mining.integer(assembly['max_rank'], min(50, server['progression']['catalog_max_level']), '样本开放档位')
    budget = forge.unique(tables[BUDGET_TABLE], lambda row: row['budget_key'] == 'independent_growth_survival', '独立养成生存目标')
    evidence = [forge.source(budget, 'candidate', BUDGET_TABLE)]
    if budget['limit_bp'] != 4000 or budget['constraint_kind'] != 'target_increase' or budget['stacking'] != 'joint_simulation':
        raise ValueError('独立生存预算的原约束已经变化')
    hp_cap = mining.integer(assembly['health_cap_bp'], budget['limit_bp'] - 1, '生命样本上限')
    mp_cap = mining.integer(assembly['mana_cap_bp'], 10_000, '魔法样本上限')
    tracks = []
    for track_id, name, source_name, group in ((3, 'hp', 'health', 'effective_survival'), (4, 'mp', 'mana', 'resource')):
        row = forge.unique(tables[TRACK_TABLE], lambda value: value['track_key'] == name, '炼体系别 ' + name)
        evidence.append(forge.source(row, 'candidate', TRACK_TABLE))
        if row['id'] != track_id or row['currency_family'] != 'gold' or row['cost_base_u'] != 1500 or row['budget_group'] != group:
            raise ValueError('样本仅使用工作簿既有金币费和原系别')
        if row['attr_curve_key'] is not None:
            raise ValueError('炼体已有正式属性曲线，须重新审核样本')
        amount = mining.integer(assembly[source_name + '_bonus_per_rank'], 2**31 - 1, '炼体每级额度')
        tracks.append({'id': track_id, 'name': row['display_name'], 'gold_base': row['cost_base_u'], 'bonus_per_rank': amount})
    steps = []
    for level in range(50):
        row = forge.unique(tables[STEP_TABLE], lambda value: value['from_level'] == level, f'炼体第{level}级')
        if row['to_level'] != level + 1 or row['cost_multiplier'] != level//10 + 1 or row['success_ppm'] != 1_000_000 - level % 10 * 100_000:
            raise ValueError('炼体工作簿与首测公式不一致')
        if level < cap:
            evidence.append(forge.source(row, 'candidate', STEP_TABLE))
            steps.append({'from': level, 'cost_multiplier': row['cost_multiplier'], 'success_ppm': row['success_ppm']})
    rules = {'tracks': tracks, 'steps': steps, 'health_cap_bp': hp_cap, 'mana_cap_bp': mp_cap}
    provenance = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence,
                  'budget_policy': 'equipment_base_then_add_once_and_cap', 'assembly': dict(assembly)}
    return ({**server, KEY: rules, '_source_evidence': {**server['_source_evidence'], KEY: provenance}},
            {**public, KEY: rules})


def main():
    """生成或逐字节核对项目独立候选。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节核对候选')
    args = parser.parse_args()
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('输入输出路径须互相独立')
    cfg = mining.config
    tables, errors, _, books = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    workbook = cfg.sha256(books['gameplay'])
    before = cfg.load_json(args.base_fixture)
    previous = before.get('_source_evidence', {}).get(rebirth.KEY, {})
    if previous.get('workbook_sha256') != workbook or previous.get('compiler_sha256') != cfg.sha256(Path(rebirth.__file__)):
        raise ValueError('前置转生候选须由同一玩法工作簿和当前投影器生成')
    server, public = project(before, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           assembly_sha256=cfg.sha256(args.assembly), base_fixture_sha256=cfg.sha256(args.base_fixture),
                                           base_public_sha256=cfg.sha256(args.base_public))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'炼体隔离快照与来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
