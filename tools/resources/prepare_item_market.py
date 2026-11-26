"""从已确认的工作簿行装配道具竞购、直购和双市场佣金。"""


ITEM_NUMBERS = {
    'max_listings_per_account_run': 'max_listings_per_account_run',
    'auction_seconds': 'auction_seconds',
    'buyout_seconds': 'buyout_seconds',
    'bid_raise_bp': 'bid_raise_bp',
    'price_step_ingot_u': 'price_step_yuanbao_units',
    'minimum_raise_ingot_u': 'minimum_raise_yuanbao_units',
    'extend_when_remaining_below_sec': 'extend_when_remaining_below_seconds',
    'extend_to_remaining_sec': 'extend_to_remaining_seconds',
    'seller_commission_bp': 'seller_commission_bp',
}

# 这些已定语义由市场状态机固定执行。源表改变时需要主动审核运行实现。
ITEM_POLICIES = {
    'listing_fee_ingot_u': 0,
    'cancel_with_active_bidder': False,
    'cancellation_fee_ingot_u': 0,
    'allow_negative_balance': False,
    'settlement': 'random_valid_buyer_whole_remaining_lot',
    'bid_reservation': 'full_lot_total',
    'raise_requires_existing_valid_bid': True,
    'max_active_bid_per_account_listing': 1,
    'same_account_trade': False,
    'new_actions_require_active_privilege': True,
    'expired_privilege_preserves_existing_settlement': True,
    'normal_and_fine_equipment_listable': False,
    'list_currency': 'unbound_ingot_only',
}

SHARED_TAX = {
    'game_gold_market_rules.tax_remainder_key': ['account_id', 'origin_server_id'],
    'game_gold_market_rules.tax_shared_between_markets': True,
    'game_gold_market_rules.tax_divisor': 10,
    'game_gold_market_rules.tax_formula': 'tax_u=floor((gross_u+remainder)/10); next_remainder=(gross_u+remainder)%10',
}


def source_rows(tables):
    """保留全部道具规则及所用的共用税余数证据。"""
    return {
        'game_item_market_rules': tables['game_item_market_rules'],
        'game_gold_market_rules': [row for row in tables['game_gold_market_rules'] if row['rule_key'] == 'seller_commission_bp'],
        'game_server_rules': [row for row in tables['game_server_rules'] if row['rule_key'] in SHARED_TAX],
    }


def confirmed_values(rows, types):
    """拒绝缺失、重复、未确认或类型错误的运行配置。"""
    result = {}
    for row in rows:
        key = row['rule_key']
        if key not in types:
            continue
        value = row['rule_value']
        if key in result or row['record_status'] != 'confirmed' or type(value) is not types[key]:
            raise ValueError(f'市场规则须唯一、已确认且类型正确：{key}')
        result[key] = value
    if result.keys() != types.keys():
        raise ValueError(f'缺少市场规则：{sorted(types.keys() - result.keys())}')
    return result


def compile_rules(tables):
    """校验固定交易语义，并从唯一源读取期限、报价和佣金定值。"""
    item_types = dict.fromkeys(ITEM_NUMBERS, int) | {key: type(value) for key, value in ITEM_POLICIES.items()}
    item = confirmed_values(tables['game_item_market_rules'], item_types)
    for key, expected in ITEM_POLICIES.items():
        if item[key] != expected:
            raise ValueError(f'道具市场语义与已装配实现不符：{key}')
    shared_tax = confirmed_values(tables['game_server_rules'], {key: type(value) for key, value in SHARED_TAX.items()})
    if shared_tax != SHARED_TAX:
        raise ValueError('双市场共享税余数的归属及原公式不符')
    gold = confirmed_values(tables['game_gold_market_rules'], {'seller_commission_bp': int})
    divisor = shared_tax['game_gold_market_rules.tax_divisor']
    if gold['seller_commission_bp'] != item['seller_commission_bp'] or item['seller_commission_bp'] * divisor != 10000:
        raise ValueError('道具和金币市场的卖方佣金须采取同一税余数口径')

    result = {field: item[key] for key, field in ITEM_NUMBERS.items()}
    for field, value in result.items():
        exclusive = 2**63 if field.endswith('_yuanbao_units') else 2**32
        if not 0 < value < exclusive:
            raise ValueError(f'道具市场规则超出正整数范围：{field}')
    step = result['price_step_yuanbao_units']
    minimum = result['minimum_raise_yuanbao_units']
    if (result['bid_raise_bp'] > 10000 or minimum < step or minimum % step != 0
            or result['extend_to_remaining_seconds'] < result['extend_when_remaining_below_seconds']):
        raise ValueError('道具市场报价步长、最小加价、比例或延时门槛不一致')
    result['tax_divisor'] = divisor
    return result
