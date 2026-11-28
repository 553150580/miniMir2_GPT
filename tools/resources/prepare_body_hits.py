#!/usr/bin/env python3
"""核对唯一工作簿与前置炼体快照，投影隔离的伤害及吸收两系。"""

import argparse
import json
from pathlib import Path

import prepare_body_training as body
import prepare_forge as forge
import prepare_mining as mining


KEY = 'expanded_hit_budget'


def project(server, public, tables, assembly):
    """保存同一来源的费率与作用范围，只补足当前隔离样本的额度。"""
    mining.fields(assembly, {'damage_bonus_per_rank', 'damage_share_cap_bp',
                             'absorption_bonus_per_rank', 'absorption_share_cap_bp'}, '炼体命中样本')
    if (server.get('rule_version') != public.get('rule_version') or server.get('rule_version', 0) < 30
            or any(KEY in data or data.get('development_only') is not True or data.get('runtime_ready') is not False
                   for data in (server, public))):
        raise ValueError('炼体命中使用同规则的前置隔离候选')
    existing = server.get(body.KEY)
    if (not existing or existing != public.get(body.KEY) or [row['id'] for row in existing['tracks']] != [3, 4]
            or not existing['steps'] or tables[body.PIPELINE_TABLE]):
        raise ValueError('生命及魔法前置候选或正式属性结算表已经变化')
    tracks, budgets, evidence = [], {}, []
    for track_id, track_key, gold in ((1, 'damage', 3000), (2, 'absorption', 1500)):
        group = 'expanded_' + track_key
        track = forge.unique(tables[body.TRACK_TABLE], lambda row: row['track_key'] == track_key, '炼体系别 ' + track_key)
        budget = forge.unique(tables[body.BUDGET_TABLE], lambda row: row['budget_key'] == group, '扩展属性组 ' + group)
        if (track['id'] != track_id or track['cost_base_u'] != gold or track['currency_family'] != 'gold'
                or track['budget_group'] != group or track['attr_curve_key'] is not None):
            raise ValueError('现有炼体系别、费率或正式曲线已经变化')
        if budget['limit_bp'] != 2000 or budget['constraint_kind'] != 'hard_cap_candidate' or budget['stacking'] != 'additive':
            raise ValueError('扩展命中两组的候选加算上限已经变化')
        evidence.extend((forge.source(track, 'candidate', body.TRACK_TABLE), forge.source(budget, 'candidate', body.BUDGET_TABLE)))
        share = mining.integer(assembly[track_key + '_share_cap_bp'], budget['limit_bp'], '炼体来源上限')
        bonus = mining.integer(assembly[track_key + '_bonus_per_rank'], share // len(existing['steps']), '炼体每级万分比')
        tracks.append({'id': track_id, 'name': track['display_name'], 'gold_base': gold, 'bonus_per_rank': bonus, 'share_cap_bp': share})
        budgets[track_key + '_cap_bp'] = budget['limit_bp']
    rules = {**existing, 'tracks': [*existing['tracks'], *tracks]}
    provenance = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence,
                  'budget_policy': 'original_net_then_capped_damage_and_absorption', 'assembly': dict(assembly)}
    return ({**server, body.KEY: rules, KEY: budgets, '_source_evidence': {**server['_source_evidence'], KEY: provenance}},
            {**public, body.KEY: rules, KEY: budgets})


def main():
    """生成或逐字节核对隔离文件，关联原工作簿和前置投影。"""
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
    previous = before.get('_source_evidence', {}).get(body.KEY, {})
    if previous.get('workbook_sha256') != workbook or previous.get('compiler_sha256') != cfg.sha256(Path(body.__file__)):
        raise ValueError('前置炼体候选须由同一玩法工作簿和当前投影器生成')
    server, public = project(before, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           assembly_sha256=cfg.sha256(args.assembly), base_fixture_sha256=cfg.sha256(args.base_fixture),
                                           base_public_sha256=cfg.sha256(args.base_public))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'炼体伤害与吸收隔离快照与来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
