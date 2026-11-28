#!/usr/bin/env python3
"""从唯一工作簿导出隔离战令，复用已经配置的日常活跃与成长材料。"""
import argparse
from datetime import date
import json
from pathlib import Path

import prepare_forge as forge
import prepare_growth_shop as shop
import prepare_mining as mining


KEY = 'battle_pass'
TIERS = ('free', 'paid_28', 'paid_98')
PERIODS = {'daily': 'daily', 'once_per_season': 'season'}
CURRENCIES = {'bound_gold': 'bound_gold', 'bound_ingot': 'bound_yuanbao_units'}


def rewards_for(key, rows, items, evidence):
    """沿工作簿原类型和整数单位导出绑定奖励，不把不同档位相加。"""
    rewards = {'daily': {}, 'season': {}}
    seen = set()
    for row in rows:
        if row['tier_key'] != key:
            continue
        period, asset = row['period_key'], row['reward_key']
        if row['bound'] is not True or period not in PERIODS or (period, asset) in seen:
            raise ValueError('战令奖励须固定绑定，且同档同周期内每种奖励唯一')
        seen.add((period, asset))
        evidence.append(forge.source(row, 'candidate', 'game_battlepass_rewards'))
        target = rewards[PERIODS[period]]
        quantity = mining.integer(row['quantity_u'], 1_000_000_000, '绑定奖励单位')
        if asset in CURRENCIES:
            target[CURRENCIES[asset]] = quantity
        else:
            target.setdefault('items', []).append({'item_id': items[asset], 'quantity': quantity})
    for reward in rewards.values():
        if 'items' in reward:
            reward['items'].sort(key=lambda item: item['item_id'])
    return rewards


def project(server, public, tables, assembly):
    """保留开发门禁，初测日历使用独立装配，活动门槛来自既有任务。"""
    mining.fields(assembly, {'anchor_day'}, '战令隔离日历')
    anchor = date.fromisoformat(assembly['anchor_day'])
    if not 2020 <= anchor.year <= 2099 or anchor.isoformat() != assembly['anchor_day']:
        raise ValueError('隔离战令的上海运营日起点不合法')
    if (server.get('rule_version') != public.get('rule_version') or server.get('rule_version', 0) < 30
            or any(KEY in doc or shop.KEY not in doc or doc.get('development_only') is not True
                   or doc.get('runtime_ready') is not False for doc in (server, public))):
        raise ValueError('战令须使用同版本成长商城隔离前置')
    if not server.get('tasks_codex', {}).get('daily_activity_target'):
        raise ValueError('战令须引用现有的每日任务活跃门槛')
    items = {'enhance_stone': server['equipment_enhancement']['stone_item_id'],
             'refine_stone': server['equipment_refine']['stone_item_id'],
             'skill_page': server['skill_economy']['page_item_id'],
             'blessing_oil': server['luck_oil']['item_id'], 'feather': server['wings']['feather_item_id']}
    if len(set(items.values())) != len(items):
        raise ValueError('战令成长材料模板重复')
    for item in items.values():
        definition = forge.unique(server['items'], lambda entry: entry['id'] == item, '战令绑定材料')
        if not definition['stackable']:
            raise ValueError('战令成长材料须为可堆叠正式模板')
    tiers, evidence = [], []
    reward_rows = tables['game_battlepass_rewards']
    if any(row['tier_key'] not in TIERS for row in reward_rows) or len(tables['game_battlepass_tiers']) != len(TIERS):
        raise ValueError('战令原表出现未登记的档位或奖励')
    for number, key in enumerate(TIERS, start=1):
        row = forge.unique(tables['game_battlepass_tiers'], lambda entry: entry['tier_key'] == key, '战令原档位')
        if (row['id'] != number or row['duration_days'] != 30 or row['daily_total_not_stack'] is not True
                or row['cash_only'] != (number > 1)):
            raise ValueError('战令原档位须采用三档、30 天、不叠加的现金规则')
        evidence.append(forge.source(row, 'candidate', 'game_battlepass_tiers'))
        price = mining.integer(row['cash_fen'], 1_000_000_000, '战令现金分价') if number > 1 else row['cash_fen']
        if number == 1 and price != 0:
            raise ValueError('免费原档的价格须为零')
        tiers.append({'id': number, 'cash_fen': price, **rewards_for(key, reward_rows, items, evidence)})
    return ({**server, KEY: {'anchor_day': anchor.isoformat(), 'tiers': tiers},
             '_source_evidence': {**server['_source_evidence'], KEY: {
                 'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence, 'assembly': assembly}}},
            {**public, KEY: True})


def main():
    """校验前置输入，并可逐字节重建客户端开关和服务端原奖表。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, type=mining.work_path, required=True)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('战令投影各输入与输出须使用独立项目工作路径')
    cfg = mining.config
    tables, errors, _, books = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    base = cfg.load_json(args.base_fixture)
    workbook = cfg.sha256(books['gameplay'])
    source = base.get('_source_evidence', {}).get(shop.KEY, {})
    if source.get('workbook_sha256') != workbook or source.get('compiler_sha256') != cfg.sha256(Path(shop.__file__)):
        raise ValueError('战令前置的工作簿或隔离投影已有变化')
    server, public = project(base, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           assembly_sha256=cfg.sha256(args.assembly), base_fixture_sha256=cfg.sha256(args.base_fixture),
                                           base_public_sha256=cfg.sha256(args.base_public))
    for path, value in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'战令隔离结果与输入不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'fixture_sha256': cfg.sha256(args.fixture_out), 'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
