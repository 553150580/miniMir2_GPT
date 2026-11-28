#!/usr/bin/env python3
"""从唯一玩法工作簿投影隔离商城，归并现有书页报价并保留待定商品。"""
import argparse
import json
from pathlib import Path

import prepare_forge as forge
import prepare_mining as mining
import prepare_soul_beast as soul


KEY = 'growth_shop'
CURRENCIES = {'ingot': 1, 'bound_ingot': 2}
PRODUCTS = {'oil_ingot': 'oil', 'oil_bound_ingot': 'oil', 'page_ingot': 'page',
            'page_bound_ingot': 'page', 'refine_stone': 'refine'}


def project(server, public, tables, assembly):
    """正式价格只取工作簿。隔离限量用于复验跨币种共用水位。"""
    mining.fields(assembly, {'max_count', 'sample_daily_limits'}, '商城隔离装配')
    mining.fields(assembly['sample_daily_limits'], {'oil', 'page'}, '商城隔离限量')
    maximum = mining.integer(assembly['max_count'], 1000, '商城单笔份数')
    if (server.get('rule_version') != public.get('rule_version') or server.get('rule_version', 0) < 30
            or any(KEY in doc or soul.KEY not in doc or doc.get('development_only') is not True
                   or doc.get('runtime_ready') is not False for doc in (server, public))):
        raise ValueError('商城目录须引用同版本隔离前置')
    item_ids = {'oil': server['luck_oil']['item_id'], 'page': server['skill_economy']['page_item_id'],
                'refine': server['equipment_refine']['stone_item_id']}
    if len(set(item_ids.values())) != len(item_ids):
        raise ValueError('各商城材料必须拥有独立的原物品编号')
    offers, evidence = [], []
    for key, group in PRODUCTS.items():
        row = forge.unique(tables['game_shop_prices'], lambda entry: entry['product_key'] == key, '原商城商品')
        definition = forge.unique(server['items'], lambda entry: entry['id'] == item_ids[group], '商城材料')
        if row['purchased_bound'] is not True or not definition['stackable']:
            raise ValueError('商城必须发放可堆叠的原绑定材料')
        offer = {'id': mining.integer(row['id'], 1_000_000, '商品编号'), 'item_id': definition['id'],
                 'name': row['display_name'], 'currency': 0, 'price_units': 0, 'max_count': 0, 'daily_limit': 0,
                 'item_per_pack': mining.integer(row['quantity'], definition['max_stack'], '每份材料数量')}
        currency = row['payment_currency']
        if group == 'refine':
            if currency != 'ingot_family' or row.get('price_u') is not None or row.get('daily_limit') is not None:
                raise ValueError('洗炼石已出现新价格或条款，请先核对开放口径')
            evidence.append(forge.source(row, 'unverified', 'game_shop_prices'))
        else:
            if currency not in CURRENCIES or key.endswith('_bound_ingot') != (currency == 'bound_ingot'):
                raise ValueError('原商品币种发生变化')
            evidence.append(forge.source(row, 'candidate', 'game_shop_prices'))
            limit = mining.integer(assembly['sample_daily_limits'][group], 1_000_000, '隔离材料日限量')
            if row.get('daily_limit') is not None:
                limit = mining.integer(row['daily_limit'], 1_000_000, '工作簿材料日限量')
            offer.update(currency=CURRENCIES[currency], price_units=mining.integer(row['price_u'], 1_000_000_000, '原元宝价格'),
                         max_count=maximum, daily_limit=limit)
        if group == 'page':
            legacy = [server['skill_shop'], public['shop_offers']]
            legacy.extend(next(shop['offers'] for shop in scope['npc_shops'] if shop['key'] == 'books')
                          for scope in (server, public))
            for directory in legacy:
                prior = forge.unique(directory, lambda entry: entry.get('id', entry.get('offer_id')) == offer['id'], '旧书页报价')
                if (prior['item_id'] != offer['item_id'] or prior.get('currency', prior.get('currency_kind')) != currency
                        or prior['price_units'] != offer['price_units'] or prior['quantity'] != offer['item_per_pack']
                        or prior['output_bound'] is not True):
                    raise ValueError('旧书页售价和绑定必须与唯一工作簿一致')
        offers.append(offer)
    if len({row['id'] for row in offers}) != len(offers):
        raise ValueError('商城工作簿商品编号重复')
    shop_items = set(item_ids.values())
    def keep(rows):
        """同类材料沿唯一商城计数，原技能书继续保留原价和渠道。"""
        return [row for row in rows if row['item_id'] not in shop_items]
    def shops(doc):
        """同时更新客户端与服务端 NPC 目录，避免用旧书页入口绕过日限量。"""
        return [{**row, 'offers': keep(row['offers'])} for row in doc['npc_shops']]
    source = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence, 'assembly': assembly}
    return ({**server, 'skill_shop': keep(server['skill_shop']), 'npc_shops': shops(server),
             KEY: {'offers': sorted(offers, key=lambda offer: offer['id'])},
             '_source_evidence': {**server['_source_evidence'], KEY: source}},
            {**public, 'shop_offers': keep(public['shop_offers']), 'npc_shops': shops(public), KEY: True})


def main():
    """验证前置来源和完整项目内路径，支持按原输入逐字节重现两份产物。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, type=mining.work_path, required=True)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('商城投影输入与输出须使用不同的项目工作路径')
    cfg = mining.config
    tables, errors, _, books = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    base = cfg.load_json(args.base_fixture)
    workbook = cfg.sha256(books['gameplay'])
    evidence = base.get('_source_evidence', {}).get(soul.KEY, {})
    if evidence.get('workbook_sha256') != workbook or evidence.get('compiler_sha256') != cfg.sha256(Path(soul.__file__)):
        raise ValueError('商城所用前置的工作簿或投影器已变化')
    server, public = project(base, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           assembly_sha256=cfg.sha256(args.assembly), base_fixture_sha256=cfg.sha256(args.base_fixture),
                                           base_public_sha256=cfg.sha256(args.base_public))
    for path, value in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'商城隔离结果与输入不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
