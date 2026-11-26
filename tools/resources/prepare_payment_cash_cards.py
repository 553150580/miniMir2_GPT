#!/usr/bin/env python3
"""从唯一玩法工作簿投影六款角色现金卡，生成独立支付验收夹具。"""

import argparse
import json
from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[2]
WORK = ROOT / '.work'
BASE_FIXTURE = ROOT / 'config/development/first-loop.json'
PRODUCTS_TABLE = 'game_card_products'
ENTITLEMENTS_TABLE = 'game_entitlement_rules'
CARD_KINDS = {'privilege': 1, 'hosting': 2}
DURATIONS = {30, 90, 365}

sys.path.insert(0, str(ROOT / 'tools/config'))
import export_configs as config
from prepare_role_cards import compile_role_cards


def compile_cash_cards(products):
    """仅收取已确认的角色现金卡，校验原人民币分且拒绝混合支付。"""
    projected, ids, terms = [], set(), set()
    expected = {(kind, days) for kind in CARD_KINDS.values() for days in DURATIONS}
    for row in products:
        payment, product_id = row['payment_key'], row['id']
        if type(product_id) is not int or not 0 < product_id < 2**32 or product_id in ids:
            raise ValueError(f'双卡商品编号须为当前工作簿的唯一正整数：{product_id}')
        ids.add(product_id)
        if payment == 'ingot':
            continue
        kind = CARD_KINDS.get(row['card_key'])
        days, fen = row['duration_days'], row['cash_fen']
        if (payment != 'cash' or row['record_status'] != 'confirmed' or row['role_scoped'] is not True
                or row['ingot_u'] is not None or not row['source_ref'] or kind is None
                or type(days) is not int or days not in DURATIONS or type(fen) is not int or not 0 < fen < 2**63):
            raise ValueError(f'现金卡须为已确认的角色级独立商品且只使用人民币整数分：{product_id}')
        key = kind, days
        if key in terms:
            raise ValueError(f'现金卡的卡种和期限组合重复：{product_id}')
        terms.add(key)
        projected.append({'id': product_id, 'kind': kind, 'days': days, 'price_fen': fen})
    if terms != expected:
        raise ValueError('现金卡须完整提供两种角色卡的月卡、季卡和年卡')
    return {'cash_card_products': sorted(projected, key=lambda row: row['id'])}


def project_fixture(base, tables):
    """保留现有游戏规则和元宝月卡，只增加隔离现金卡的可追溯目录。"""
    if (type(base.get('rule_version')) is not int or base['rule_version'] <= 0 or base.get('development_only') is not True
            or base.get('runtime_ready') is not False or 'payment' in base):
        raise ValueError('现金卡须从未装配支付的开发夹具生成')
    products = tables[PRODUCTS_TABLE]
    payments = compile_cash_cards(products)
    if base.get('role_cards') != compile_role_cards(products, tables[ENTITLEMENTS_TABLE]):
        raise ValueError('主夹具元宝月卡与唯一玩法工作簿不一致，请先按原投影校验')
    cash_sources = [
        {key: row[key] for key in ('id', 'card_key', 'payment_key', 'record_status', 'source_ref')}
        for row in sorted(products, key=lambda row: row['id']) if row['payment_key'] == 'cash'
    ]
    evidence = {'status': 'isolated_catalog', 'default_enabled': False, 'record_sources': {PRODUCTS_TABLE: cash_sources}}
    return {**base, 'payment': payments, '_source_evidence': {**base['_source_evidence'], 'payment': evidence}}


def work_path(value):
    """拒绝把隔离生成物写回主配置或工作簿路径。"""
    path, parent = Path(value).resolve(), WORK.resolve()
    if path == parent or not path.is_relative_to(parent):
        raise argparse.ArgumentTypeError('现金卡候选路径须位于本项目 .work 子目录')
    return path


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--fixture-out', required=True, type=work_path, help='独立现金卡目录夹具输出')
    parser.add_argument('--check', action='store_true', help='逐字节校验现有生成文件')
    args = parser.parse_args(argv)
    schema = config.load_json(ROOT / 'config/schema/catalog.json')
    tables, errors, _, workbooks = config.collect(schema)
    if errors:
        raise ValueError(errors)
    fixture = project_fixture(config.load_json(BASE_FIXTURE), tables)
    table = next(table for table in schema['tables'] if table['table_id'] == PRODUCTS_TABLE)
    workbook = workbooks[table['workbook_group']]
    fixture['_source_evidence']['payment'].update(
        workbook=workbook.relative_to(ROOT).as_posix(), workbook_sha256=config.sha256(workbook), sheet=table['sheet_name'],
        base_fixture_sha256=config.sha256(BASE_FIXTURE), compiler_sha256=config.sha256(Path(__file__)),
    )
    expected = (json.dumps(fixture, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
    if args.check:
        if args.fixture_out.read_bytes() != expected:
            raise ValueError('现金卡隔离夹具与当前工作簿投影不一致')
    else:
        args.fixture_out.parent.mkdir(parents=True, exist_ok=True)
        args.fixture_out.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'cash_cards': len(fixture['payment']['cash_card_products']),
                      'default_enabled': False, 'path': str(args.fixture_out), 'sha256': config.sha256(args.fixture_out)},
                     ensure_ascii=False))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
