"""从唯一玩法工作簿装配元宝月卡与特权卡每日领取。"""


def compile_role_cards(product_rows, entitlement_rows):
    """只投影已确认且角色独立的两类非绑定元宝月卡。"""
    kinds = {'privilege': 1, 'hosting': 2}
    products, product_ids, product_kinds = [], set(), set()
    for row in product_rows:
        if row['payment_key'] == 'cash':
            continue
        product_id = row['id']
        kind = kinds.get(row['card_key'])
        days = row['duration_days']
        price = row['ingot_u']
        if (row['payment_key'] != 'ingot' or row['record_status'] != 'confirmed' or row['role_scoped'] is not True
                or row['cash_fen'] is not None or type(product_id) is not int or not 0 < product_id < 2**32
                or kind is None or type(days) is not int or days != 30 or type(price) is not int or not 0 < price < 2**63):
            raise ValueError(f'元宝月卡商品须为已确认的角色级30天商品：{product_id}')
        if product_id in product_ids or kind in product_kinds:
            raise ValueError(f'元宝月卡商品编号及卡种须唯一：{product_id}')
        products.append({'id': product_id, 'kind': kind, 'days': days, 'price_yuanbao_units': price})
        product_ids.add(product_id)
        product_kinds.add(kind)
    if product_kinds != set(kinds.values()):
        raise ValueError('元宝月卡必须完整配置特权卡和托管卡')

    claims = [row for row in entitlement_rows if row['rule_key'] == 'privilege_daily_bound_ingot_u']
    if len(claims) != 1 or claims[0]['record_status'] != 'confirmed':
        raise ValueError('特权卡每日绑定元宝领取须为唯一的已确认规则')
    units = claims[0]['rule_value']
    if type(units) is not int or not 0 < units < 2**63:
        raise ValueError('特权卡每日绑定元宝领取须为正整数定点单位')
    return {'products': sorted(products, key=lambda product: product['id']), 'daily_privilege_units': units}
