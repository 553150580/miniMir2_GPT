#!/usr/bin/env python3
"""从唯一玩法工作簿及前置候选装配隔离的三个元宝炼体系。"""

import argparse
import json
from pathlib import Path

import prepare_body_hits as hits
import prepare_body_training as body
import prepare_forge as forge
import prepare_mining as mining


KEY = 'mastery_combat'
# 每项的工作簿身份和对应的统一属性组。
TRACKS = ((5, 'critical', 'sustained_output', 'critical_cap_bp'),
          (6, 'speed', 'sustained_output', 'attack_speed_cap_bp'),
          (7, 'resistance', 'resistance', 'resistance_cap_bp'))
CONTROL_NAMES = {91: '冰冻', 92: '麻痹', 93: '石化'}


def project(server, public, tables, assembly):
    """沿用前置配置，只追加本阶段系别和隔离控制来源。"""
    mining.fields(assembly, {'critical_extra_bp', 'tracks', 'monster_controls'}, '元宝炼体隔离装配')
    if (server.get('rule_version') != public.get('rule_version') or server.get('rule_version', 0) < 30
            or any(KEY in data or hits.KEY not in data or data.get('development_only') is not True
                   or data.get('runtime_ready') is not False for data in (server, public))):
        raise ValueError('元宝炼体需要同规则的隔离伤害与吸收前置')
    existing = server.get(body.KEY)
    if (not existing or existing != public.get(body.KEY) or [row['id'] for row in existing['tracks']] != [3, 4, 1, 2]
            or not existing['steps'] or tables[body.PIPELINE_TABLE]):
        raise ValueError('前置四系或正式属性结算表已经变化')
    mining.fields(assembly['tracks'], {name for _, name, _, _ in TRACKS}, '隔离能力额度')
    rules = {'critical_extra_bp': mining.integer(assembly['critical_extra_bp'], 5000, '暴击额外伤害基点')}
    tracks, evidence = [], []
    for track_id, name, group, limit_key in TRACKS:
        row = forge.unique(tables[body.TRACK_TABLE], lambda value: value['track_key'] == name, '元宝炼体系别 ' + name)
        if (row['id'] != track_id or row['cost_base_u'] != 200 or row['currency_family'] != 'ingot'
                or row['budget_group'] != group or row['attr_curve_key'] is not None):
            raise ValueError('原费用、币种、属性组或正式系数已变')
        evidence.append(forge.source(row, 'candidate', body.TRACK_TABLE))
        sample = assembly['tracks'][name]
        mining.fields(sample, {'group_cap_bp', 'share_cap_bp', 'bonus_per_rank'}, '元宝炼体属性组')
        limit = mining.integer(sample['group_cap_bp'], 2000, '元宝炼体组上限')
        share = mining.integer(sample['share_cap_bp'], limit, '元宝炼体来源上限')
        bonus = mining.integer(sample['bonus_per_rank'], share // len(existing['steps']), '每级元宝炼体基点')
        tracks.append({'id': track_id, 'name': row['display_name'], 'yuanbao_base_units': row['cost_base_u'],
                       'bonus_per_rank': bonus, 'share_cap_bp': share})
        rules[limit_key] = limit
    # 固定净伤与固定装备下的样本上界，只用来拦住明显超过联合输出目标的装配。
    for group, target in (('output', 3000), ('survival', 4000)):
        row = forge.unique(tables[body.BUDGET_TABLE], lambda value: value['budget_key'] == 'independent_growth_' + group,
                           '独立成长目标 ' + group)
        if row['limit_bp'] != target or row['constraint_kind'] != 'target_increase' or row['stacking'] != 'joint_simulation':
            raise ValueError('全来源联合预算目标已变')
        evidence.append(forge.source(row, 'candidate', body.BUDGET_TABLE))
    damage = next(row['share_cap_bp'] for row in existing['tracks'] if row['id'] == 1)
    if ((10000 + damage) * 10000 + tracks[0]['share_cap_bp'] * rules['critical_extra_bp']
            >= 13000 * (10000 - tracks[1]['share_cap_bp'])):
        raise ValueError('隔离样本已耗尽独立成长持续输出预算')
    controls = assembly['monster_controls']
    if not isinstance(controls, dict) or len(controls) != len(CONTROL_NAMES):
        raise ValueError('隔离样本须覆盖三个不同控制效果')
    enemies = {str(row['id']) for key in ('monsters', 'challenge_monsters') for row in server[key]}
    for key, control in controls.items():
        if key not in enemies:
            raise ValueError('控制来源须使用原隔离怪物模板')
        mining.fields(control, {'status_id', 'duration_ms', 'chance_ppm'}, '怪物控制样本')
        mining.integer(control['chance_ppm'], 1_000_000, '控制触发率')
        if mining.integer(control['duration_ms'], 10_000, '控制时间') < 100:
            raise ValueError('控制样本小于最小推进单位')
    if {control['status_id'] for control in controls.values()} != CONTROL_NAMES.keys():
        raise ValueError('隔离样本的控制来源未覆盖各自状态')
    rules['monster_controls'] = controls
    statuses = public['statuses']
    if {row['id'] for row in statuses} & CONTROL_NAMES.keys():
        raise ValueError('隔离状态编号与现有观察合同冲突')
    training = {**existing, 'tracks': [*existing['tracks'], *tracks]}
    provenance = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence,
                  'budget_policy': 'separate_critical_cadence_and_resistance', 'assembly': assembly}
    return ({**server, body.KEY: training, KEY: rules, '_source_evidence': {**server['_source_evidence'], KEY: provenance}},
            {**public, body.KEY: training, KEY: {key: value for key, value in rules.items() if key != 'monster_controls'},
             'statuses': [*statuses, *({'id': key, 'name': label, 'short_name': label} for key, label in CONTROL_NAMES.items())]})


def main():
    """生成或逐字节校验隔离候选，并记录完整输入摘要。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节核对候选')
    args = parser.parse_args()
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('输入输出须分别保存')
    cfg = mining.config
    tables, errors, _, books = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    workbook = cfg.sha256(books['gameplay'])
    before = cfg.load_json(args.base_fixture)
    previous = before.get('_source_evidence', {}).get(hits.KEY, {})
    if previous.get('workbook_sha256') != workbook or previous.get('compiler_sha256') != cfg.sha256(Path(hits.__file__)):
        raise ValueError('前置候选须由同一玩法工作簿及当前投影器生成')
    server, public = project(before, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           assembly_sha256=cfg.sha256(args.assembly), base_fixture_sha256=cfg.sha256(args.base_fixture),
                                           base_public_sha256=cfg.sha256(args.base_public))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'元宝炼体隔离快照与来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
