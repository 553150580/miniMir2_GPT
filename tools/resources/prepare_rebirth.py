#!/usr/bin/env python3
"""用唯一玩法工作簿和已核验的前置目录装配隔离转生。"""

import argparse
import json
from pathlib import Path

import prepare_forge as forge
import prepare_medal as medal
import prepare_mining as mining


KEY = 'rebirth'
RULES_TABLE = 'game_progression_rules'
COSTS_TABLE = 'game_rebirth_costs'
RULES = {'initial_level_cap': 50, 'rebirth_level_cap_increment': 1, 'rebirth_reset_progression': False}


def project(server, public, tables, assembly):
    """保留50级前的经验，向原个人首领追加固定绑定的隔离附件。"""
    fields = {'credential_item_id', 'material_item_id', 'material_stack', 'personal_boss_template_id',
              'material_reward', 'material_cost', 'gold_cost', 'level_51_experience'}
    mining.fields(assembly, fields, '转生隔离装配')
    if (server.get('rule_version') != public.get('rule_version') or server.get('rule_version', 0) < 30
            or any(KEY in base or medal.KEY not in base or base.get('development_only') is not True
                   or base.get('runtime_ready') is not False for base in (server, public))):
        raise ValueError('转生须从当前规则的已验收隔离勋章目录装配')
    if tables[COSTS_TABLE]:
        raise ValueError('正式转生费用表已有记录，须重新审定正式费用和供给')
    evidence = []
    for name, expected in RULES.items():
        row = forge.unique(tables[RULES_TABLE], lambda value: value['rule_key'] == name, name)
        evidence.append(forge.source(row, 'confirmed', RULES_TABLE))
        if type(row['rule_value']) is not type(expected) or row['rule_value'] != expected:
            raise ValueError(f'已确认的转生规则已变化：{name}')
    prior = server['progression']
    previous_exp = prior['exp_per_level']
    if (any(prior[name] != value for name, value in RULES.items()) or prior['rebirth_enabled'] is not False
            or prior['operating_level_cap'] != 50 or prior['catalog_max_level'] != 50 or len(previous_exp) != 50
            or public['progression'] != {name: val for name, val in prior.items() if name != 'exp_per_level'}
            or public['experience'] != [{'level': i, 'next_level_exp': amount} for i, amount in enumerate(previous_exp, 1)]
            or any(row.get('level', 0) >= 51 for row in tables['game_exp_levels'])):
        raise ValueError('原双端等级数据不一致或已有正式的51级经验')
    ids = {name: mining.integer(assembly[name], 2**32 - 1, name)
           for name in ('credential_item_id', 'material_item_id', 'personal_boss_template_id')}
    proof, material, template = ids.values()
    occupied = ({row['id'] for row in server['items']} | {row['item_id'] for row in public['items']}
                | {row['id'] for row in tables['game_items']})
    if proof == material or proof in occupied or material in occupied:
        raise ValueError('转生凭证和材料须使用独立目录编号')
    stack = mining.integer(assembly['material_stack'], 2**31 - 1, '绑定材料堆叠上限')
    quantity = mining.integer(assembly['material_reward'], stack, '个人首领绑定材料')
    cost = mining.integer(assembly['material_cost'], 2**31 - 1, '绑定材料费用')
    gold = mining.integer(assembly['gold_cost'], 2**53 - 1, '金币费用')
    next_exp = mining.integer(assembly['level_51_experience'], 2**53 - 1, '51级经验门槛')
    if next_exp < previous_exp[-1]:
        raise ValueError('隔离经验尾部须衔接原已验收门槛')
    source = forge.unique(tables['game_boss_templates'],
                          lambda row: row['id'] == template and row['boss_kind'] == 'personal', '原个人首领模板')
    evidence.append(forge.source(source, 'candidate', 'game_boss_templates'))
    if source['participation_reward_group_id'] is not None or source['rank_reward_group_id'] is not None:
        raise ValueError('个人首领奖励已有正式来源，请使用已核定的供给')
    main, variants = server['personal_boss'], server.get('personal_boss_variants', [])
    boss = forge.unique([main, *variants], lambda row: row['template_id'] == template and row['enabled'] is True,
                        '已启用的个人首领')
    reward = boss.get('item_reward')
    if not reward or len(reward.get('items', [])) + len(reward.get('bound_items', [])) > 6:
        raise ValueError('原首领奖励未验收或无法容纳两格隔离附件')
    entries = [{'item_id': proof, 'quantity': 1}, {'item_id': material, 'quantity': quantity}]
    updated = {**boss, 'item_reward': {**reward, 'bound_items': [*reward.get('bound_items', []), *entries]}}
    bosses = {'personal_boss': updated} if boss is main else {
        'personal_boss_variants': [updated if row is boss else row for row in variants]}
    items = [{'id': proof, 'name': '转生凭证', 'stackable': True, 'max_stack': 1},
             {'id': material, 'name': '转生材料', 'stackable': True, 'max_stack': stack}]
    display = [{'item_id': item['id'], 'name': item['name'], 'kind': kind,
                'allowed_jobs': [0, 1, 2], 'use_enabled': False}
               for item, kind in zip(items, ('rebirth_credential', 'rebirth_material'), strict=True)]
    stage = {'required_level': 50, 'challenge_template_id': template, 'credential_item_id': proof,
             'material_item_id': material, 'material_cost': cost, 'gold_cost': gold}
    progression = {**public['progression'], 'operating_level_cap': 51, 'catalog_max_level': 51, 'rebirth_enabled': True}
    provenance = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence,
                  'fees_rewards_and_tail_status': 'isolation_test_only', 'unverified_assembly': dict(assembly)}
    return ({**server, **bosses, KEY: {'stages': [stage]}, 'items': [*server['items'], *items],
             'progression': {**progression, 'exp_per_level': [*previous_exp, next_exp]},
             '_source_evidence': {**server['_source_evidence'], KEY: provenance}},
            {**public, KEY: {'stages': [{**stage, 'challenge_name': boss['display_name']}]},
             'progression': progression, 'experience': [*public['experience'], {'level': 51, 'next_level_exp': next_exp}],
             'items': [*public['items'], *display]})


def main():
    """生成或逐字节检查两份候选，只接受项目工作目录内隔离路径。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, required=True, type=mining.work_path)
    parser.add_argument('--check', action='store_true', help='逐字节核对双端候选')
    args = parser.parse_args()
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('输入输出必须使用独立路径')
    cfg = mining.config
    tables, errors, _, books = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    workbook = cfg.sha256(books['gameplay'])
    base = cfg.load_json(args.base_fixture)
    previous = base.get('_source_evidence', {}).get(medal.KEY, {})
    if previous.get('workbook_sha256') != workbook or previous.get('compiler_sha256') != cfg.sha256(Path(medal.__file__)):
        raise ValueError('前置勋章须由同一玩法工作簿与现行导出器生成')
    server, public = project(base, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           assembly_sha256=cfg.sha256(args.assembly),
                                           base_fixture_sha256=cfg.sha256(args.base_fixture),
                                           base_public_sha256=cfg.sha256(args.base_public))
    for path, data in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(data, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'转生候选产物已变化：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
