"""从唯一玩法工作簿生成城镇商店及装备耐久配置。"""

from copy import deepcopy
import re


PROFILE_KEYS = {'npc_shops', 'npc_shop_prices', 'equipment_durability'}
SHOP_FIELDS = {'key', 'roles', 'item_ids', 'legacy_skill_offers', 'sell_skill_books', 'can_repair'}
PRICE_FIELDS = {'items', 'sell_ratio_numerator', 'sell_ratio_denominator', 'max_purchase_quantity', 'source_ref'}
PRICE_ITEM_FIELDS = {'item_id', 'price_gold', 'source_ref'}
RULE_FIELDS = {'weapon_loss_u', 'armor_hit_loss_u', 'death_loss_u', 'repair_range_cells', 'ordinary', 'special'}


def exact_fields(value, fields, description):
    if not isinstance(value, dict) or set(value) != fields:
        raise ValueError(f'{description}字段不完整或包含未知字段')


def integer(value, minimum=1, maximum=2**32-1):
    return type(value) is int and minimum <= value <= maximum


def nonempty_string(value):
    return isinstance(value, str) and bool(value.strip())


def compile_prices(profile, versions_items):
    if profile['rule_version'] < 25:
        if PROFILE_KEYS & profile.keys():
            raise ValueError('城镇商店及耐久规则须使用rule25')
        return {}
    price_settings = profile.get('npc_shop_prices')
    exact_fields(price_settings, PRICE_FIELDS, '城镇商店价格')
    numerator, denominator = price_settings['sell_ratio_numerator'], price_settings['sell_ratio_denominator']
    if (not integer(denominator) or not integer(numerator, maximum=denominator)
            or not integer(price_settings['max_purchase_quantity']) or not nonempty_string(price_settings['source_ref'])):
        raise ValueError('城镇商店收购比例、批量上限及来源须完整')
    rows = price_settings['items']
    if not isinstance(rows, list) or not rows:
        raise ValueError('城镇商店缺少唯一物品来源价格')
    by_id = {}
    for row in rows:
        exact_fields(row, PRICE_ITEM_FIELDS, '城镇物品来源价格')
        item_id = row['item_id']
        if (not integer(item_id) or item_id in by_id or item_id not in versions_items
                or not integer(row['price_gold'], maximum=2**53-1) or not nonempty_string(row['source_ref'])):
            raise ValueError('城镇物品来源价格须引用已选物品并保留唯一正价及来源')
        by_id[item_id] = row['price_gold']
    return by_id


def equipment_fields(equipment, prices, profile):
    if profile['rule_version'] < 25:
        return {}
    item_id, durability = equipment['item_id'], equipment.get('original_durability_u')
    if not integer(durability, maximum=2**31-1) or item_id not in prices:
        raise ValueError('可修装备须有唯一原始耐久上限和原始价格')
    return {'original_durability_u': durability, 'repair_base_price': prices[item_id]}


def durability_rules(profile, items):
    rules = profile.get('equipment_durability')
    exact_fields(rules, RULE_FIELDS, '装备耐久规则')
    if (any(not integer(rules[key]) for key in ('weapon_loss_u', 'armor_hit_loss_u', 'death_loss_u'))
            or not integer(rules['repair_range_cells'], maximum=8)):
        raise ValueError('装备磨损及修理距离须为有效正整数')
    ordinary, special = rules['ordinary'], rules['special']
    exact_fields(ordinary, {'kind', 'upper_loss_divisor', 'min_max_durability_u'}, '普通修理')
    exact_fields(special, {'kind', 'full_repair_cost_u'}, '特殊修理')
    if (not integer(ordinary['kind'], 1, 1) or not integer(special['kind'], 2, 2)
            or not integer(ordinary['upper_loss_divisor'], minimum=2)
            or not integer(ordinary['min_max_durability_u'], maximum=2**31-1)
            or not integer(special['full_repair_cost_u'], maximum=2**53-1)):
        raise ValueError('普通修理上限磨损、特修满损费用及修理类型必须完整')
    if any(row['original_durability_u'] < ordinary['min_max_durability_u'] for row in items if row['is_equipment']):
        raise ValueError('普通修理后的最低耐久上限不能高于装备原始上限')
    return deepcopy(rules)


def resale_price(price, quantity, settings):
    return price * settings['sell_ratio_numerator'] // (quantity * settings['sell_ratio_denominator'])


def skill_book_sell_prices(skill_offers, book_ids, settings):
    prices = {}
    for offer in skill_offers:
        item_id = offer['item_id']
        if offer['currency'] == 'gold' and item_id in book_ids:
            price = resale_price(offer['price_units'], offer['quantity'], settings)
            prices[item_id] = min(price, prices.get(item_id, price))
    if not prices or min(prices.values()) <= 0:
        raise ValueError('书店金币收购须有正价技能书商品，不能用书页或其他币种兑换')
    return [{'item_id': item_id, 'price_gold': price} for item_id, price in prices.items()]


def compile_shops(profile, items, maps, skill_offers, prices):
    book_ids = {row['id'] for row in items if row.get('book_skill_id')}
    town_roles = {npc['role'] for row in maps for npc in row.get('npcs', [])}
    settings = profile['npc_shop_prices']
    rows = profile.get('npc_shops')
    if not isinstance(rows, list) or not rows:
        raise ValueError('rule25须配置城镇商店')
    keys, roles, selected_prices, shops = set(), set(), set(), []
    legacy_count = 0
    for row in rows:
        exact_fields(row, SHOP_FIELDS, '城镇商店')
        key, shop_roles, item_ids = row['key'], row['roles'], row['item_ids']
        if (not isinstance(key, str) or not re.fullmatch(r'[a-z][a-z0-9_]*', key) or key in keys
                or not isinstance(shop_roles, list) or not shop_roles
                or any(not nonempty_string(role) or role not in town_roles or role in roles for role in shop_roles)
                or len(shop_roles) != len(set(shop_roles))):
            raise ValueError('城镇商店标识须唯一，职责须与真实城镇NPC唯一对应')
        if (not isinstance(item_ids, list) or any(not integer(item_id) or item_id not in prices for item_id in item_ids)
                or len(item_ids) != len(set(item_ids))
                or any(type(row[field]) is not bool for field in ('legacy_skill_offers', 'sell_skill_books', 'can_repair'))
                or row['sell_skill_books'] and not row['legacy_skill_offers']):
            raise ValueError('城镇商店商品、书店来源及修理权限必须显式配置')
        offers, sell_prices = [], []
        for item_id in item_ids:
            price = prices[item_id]
            sell_price = resale_price(price, 1, settings)
            if sell_price <= 0:
                raise ValueError('城镇商店收购单价须为正整数')
            offers.append({'id': item_id, 'item_id': item_id, 'quantity': 1, 'currency': 'gold',
                           'price_units': price, 'required_role_level': 1, 'output_bound': False,
                           'max_purchase_quantity': settings['max_purchase_quantity']})
            sell_prices.append({'item_id': item_id, 'price_gold': sell_price})
        if row['legacy_skill_offers']:
            legacy_count += 1
            offers.extend(deepcopy(skill_offers))
        if row['sell_skill_books']:
            sell_prices.extend(skill_book_sell_prices(skill_offers, book_ids, settings))
        if not offers and not row['can_repair']:
            raise ValueError('城镇商店须提供商品或修理')
        if len({offer['id'] for offer in offers}) != len(offers):
            raise ValueError('同一城镇商店商品编号须唯一')
        keys.add(key)
        roles.update(shop_roles)
        selected_prices.update(item_ids)
        shops.append({'key': key, 'roles': deepcopy(shop_roles), 'offers': offers,
                      'sell_prices': sell_prices, 'can_repair': row['can_repair']})
    if legacy_count != 1 or selected_prices != prices.keys():
        raise ValueError('书店须恰好复用一次既有目录，来源价格均须对应已选商品')
    return shops
