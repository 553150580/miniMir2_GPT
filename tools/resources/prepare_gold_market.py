#!/usr/bin/env python3
"""从唯一工作簿和显式期限、参考价生成隔离金币市场夹具。"""

import argparse
import json
from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[2]
WORK = ROOT / '.work'
BASE_FIXTURE = ROOT / 'config/development/first-loop.json'
sys.path.insert(0, str(ROOT / 'tools/config'))
import export_configs as config
from prepare_item_market import SHARED_TAX, confirmed_values


GOLD_NUMBERS = {
    'gold_per_lot': 'gold_per_lot',
    'price_step_ingot_u': 'price_step_yuanbao_units',
    'max_buy_orders_per_account_run': 'max_buy_orders_per_account_run',
    'max_sell_orders_per_account_run': 'max_sell_orders_per_account_run',
}

# 这些已确认的业务语义必须与撮合实现一致，禁止随隔离装配覆盖。
GOLD_POLICIES = {
    'quantity_type': 'positive_integer_lots',
    'seller_commission_bp': 1000,
    'listing_fee_ingot_u': 0,
    'cancel_fee_ingot_u': 0,
    'partial_fill': True,
    'priority': ['price', 'accepted_time'],
    'fill_price': 'resting_order_price',
    'same_account_trade': False,
    'sell_bound_gold': False,
}

GOLD_RULES = 'game_gold_market_rules'
SERVER_RULES = 'game_server_rules'
PRICE_PREFIX = 'game_gold_market_rules.'
ASSEMBLY_FIELDS = {'order_expiry_seconds', 'reference_price_yuanbao_units'}
MAX_UINT32 = 2**32 - 1
MAX_INT64 = 2**63 - 1


def required_rule(rows, key, status):
    """明确核对条目状态和唯一性，未核验条目保持空值。"""
    found = [row for row in rows if row['rule_key'] == key]
    if len(found) != 1 or found[0]['record_status'] != status:
        raise ValueError(f'金币市场规则须唯一且保留来源状态 {status}：{key}')
    if status == 'unverified' and found[0]['rule_value'] is not None:
        raise ValueError(f'金币市场未核条目已有值，须核对原表后重新审定装配：{key}')
    return found[0]['rule_value']


def positive_integer(value, limit, label):
    """拒绝布尔、浮点和超出服务端表示范围的金额或期限。"""
    if type(value) is not int or not 0 < value <= limit:
        raise ValueError(f'金币市场 {label} 须为范围内的正整数')
    return value


def compile_rules(tables, assembly):
    """从确认规则投影账本参数，从候选规则投影限幅，由隔离装配明确给出缺项。"""
    if type(assembly) is not dict or assembly.keys() != ASSEMBLY_FIELDS:
        raise ValueError('隔离金币撮合仅接受显式订单期限与冷启动参考价')
    gold, server = tables[GOLD_RULES], tables[SERVER_RULES]
    types = dict.fromkeys(GOLD_NUMBERS, int) | {key: type(value) for key, value in GOLD_POLICIES.items()}
    fixed = confirmed_values(gold, types)
    for key, expected in GOLD_POLICIES.items():
        if fixed[key] != expected:
            raise ValueError(f'金币撮合已确认规则与运行实现不符：{key}')
    item = confirmed_values(tables['game_item_market_rules'], {'seller_commission_bp': int})
    tax = confirmed_values(server, {key: type(value) for key, value in SHARED_TAX.items()})
    if tax != SHARED_TAX or item['seller_commission_bp'] != fixed['seller_commission_bp']:
        raise ValueError('两市场卖方佣金、累计余数键或原公式不一致')
    required_rule(gold, 'order_expiry_seconds', 'unverified')
    required_rule(server, PRICE_PREFIX + 'cold_start_price_ingot_u', 'unverified')

    result = {}
    for key, field in GOLD_NUMBERS.items():
        maximum = MAX_UINT32 if field.startswith('max_') else MAX_INT64
        result[field] = positive_integer(fixed[key], maximum, key)
    reference = positive_integer(assembly['reference_price_yuanbao_units'], MAX_INT64, '隔离参考价')
    limit = positive_integer(required_rule(server, PRICE_PREFIX + 'reference_limit_bp', 'candidate'), 9999, '候选限幅')
    # Python 的整数中间量不溢出，最终发布到 Go 的上下限必须仍可表示。
    if reference % result['price_step_yuanbao_units'] != 0 or reference + reference * limit // 10000 > MAX_INT64:
        raise ValueError('金币市场参考价未对齐步长或报价上界超出整数范围')
    result.update(
        tax_divisor=tax[PRICE_PREFIX + 'tax_divisor'],
        order_expiry_seconds=positive_integer(assembly['order_expiry_seconds'], MAX_UINT32, '隔离挂单期限'),
        reference_price_yuanbao_units=reference,
        price_window_lots=positive_integer(required_rule(server, PRICE_PREFIX + 'reference_vwap_window_lots', 'candidate'),
                                           MAX_INT64, '候选成交窗口'),
        price_limit_bp=limit,
    )
    return result


def project_fixture(base, tables, assembly):
    """保留主夹具的全部玩法数据，只附加隔离金币撮合及其来源。"""
    if (base['rule_version'] != 30 or base['development_only'] is not True or base['runtime_ready'] is not False
            or 'gold_market' in base):
        raise ValueError('金币撮合须基于缺省关闭的规则 30 开发夹具')
    market = compile_rules(tables, assembly)
    if item := base.get('item_market'):
        if item['tax_divisor'] != market['tax_divisor'] or item['seller_commission_bp'] != GOLD_POLICIES['seller_commission_bp']:
            raise ValueError('主夹具的道具市场不采用原共享佣金')
    source = {}
    for table, predicate in (
        (GOLD_RULES, lambda row: row['rule_key'] in (GOLD_NUMBERS.keys() | GOLD_POLICIES.keys() | {'order_expiry_seconds'})),
        (SERVER_RULES, lambda row: row['rule_key'].startswith(PRICE_PREFIX)),
        ('game_item_market_rules', lambda row: row['rule_key'] == 'seller_commission_bp'),
    ):
        source[table] = [{key: row[key] for key in ('id', 'rule_key', 'record_status', 'source_ref')}
                         for row in tables[table] if predicate(row)]
    metadata = {'status': 'isolated_candidate', 'default_enabled': False,
                'workbook_rules': source, 'explicit_test_inputs': dict(assembly)}
    return {**base, 'gold_market': market, '_source_evidence': {**base['_source_evidence'], 'gold_market': metadata}}


def work_path(value):
    """隔离配置及装配输入只使用本项目的工作目录，禁止覆盖主配置。"""
    path = Path(value).resolve()
    parent = WORK.resolve()
    if path == parent or not path.is_relative_to(parent):
        raise argparse.ArgumentTypeError('金币市场候选路径须位于本项目 .work 子目录内')
    return path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assembly', required=True, type=work_path, help='显式挂单期限与参考价 JSON')
    parser.add_argument('--fixture-out', required=True, type=work_path, help='隔离服务端夹具输出')
    parser.add_argument('--check', action='store_true', help='只校验已生成夹具')
    args = parser.parse_args()
    if args.assembly == args.fixture_out:
        parser.error('夹具输出须与隔离装配输入分开')
    schema = config.load_json(ROOT / 'config/schema/catalog.json')
    tables, errors, _, _ = config.collect(schema)
    if errors:
        raise ValueError(errors)
    base, assembly = config.load_json(BASE_FIXTURE), config.load_json(args.assembly)
    fixture = project_fixture(base, tables, assembly)
    fixture['_source_evidence']['gold_market'].update(base_fixture_sha256=config.sha256(BASE_FIXTURE),
                                                    compiler_sha256=config.sha256(Path(__file__)))
    expected = (json.dumps(fixture, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
    if args.check:
        if args.fixture_out.read_bytes() != expected:
            raise ValueError('金币市场隔离夹具与当前来源不一致')
    else:
        args.fixture_out.parent.mkdir(parents=True, exist_ok=True)
        args.fixture_out.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'development_only': True, 'runtime_ready': False,
                      'default_enabled': False, 'path': str(args.fixture_out), 'sha256': config.sha256(args.fixture_out)},
                     ensure_ascii=False))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
