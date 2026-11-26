#!/usr/bin/env python3
"""从唯一玩法表与显式隔离映射生成矿区服务端夹具及公开目录。"""

import argparse
import json
from pathlib import Path
import re
import sys


ROOT = Path(__file__).resolve().parents[2]
WORK = ROOT / '.work'
BASE_FIXTURE = ROOT / 'config/development/first-loop.json'
RESOURCES = ROOT / 'client/assets/resources'
BASE_PUBLIC = RESOURCES / 'development/gameplay.json'
TABLE_LAYERS = 'game_mine_layers'
TABLE_ORES = 'game_ore_types'
TABLE_PURITY = 'game_ore_purity'
TABLE_TOOLS = 'game_pickaxes'
TABLE_RULES = 'game_workshop_rules'
ORE_KEYS = {'copper', 'iron', 'gold', 'black_iron'}
CONFIRMED = {'mining_binding_time': 'successful_ore_event', 'mining_only_damages_pickaxe': True,
             'hosting_card_mining_gold_bonus': False, 'pickaxe_break_action': 'consumable_may_disappear'}
TIMING = {'mining_interval_sec': 'interval_ms', 'mining_success_ppm': 'success_ppm',
          'tool_durability_cost_per_attempt': 'tool_wear_u'}

sys.path.insert(0, str(ROOT / 'tools/config'))
import export_configs as config


def integer(value, maximum, label):
    """拒绝布尔、浮点和服务端可表示范围外的数值。"""
    if type(value) is not int or not 0 < value <= maximum:
        raise ValueError(f'{label}须为范围内的正整数')
    return value


def fields(row, expected, label):
    if type(row) is not dict or row.keys() != expected:
        raise ValueError(f'{label}字段不完整或包含额外业务参数')


def source_rows(rows, expected):
    """规则证据保留原身份与状态，缺项或变更状态时拒绝静默装配。"""
    for row in rows:
        if row['record_status'] != expected or not row['source_ref']:
            raise ValueError(f'挖矿来源条目须保留状态 {expected}：{row["id"]}')
    return rows


def compile_rules(tables):
    """时间、概率、矿层门槛和候选抽样只从原表读取。"""
    rules = tables[TABLE_RULES]
    values = {}
    for key in CONFIRMED.keys() | TIMING.keys():
        rows = [row for row in rules if row['rule_key'] == key]
        expected = 'confirmed' if key in CONFIRMED else 'candidate'
        if len(rows) != 1:
            raise ValueError(f'挖矿规则须唯一：{key}')
        values[key] = source_rows(rows, expected)[0]['rule_value']
    if any(type(values[key]) is not type(value) or values[key] != value for key, value in CONFIRMED.items()):
        raise ValueError('已确认的出矿绑定、工具扣减或托管收益规则与实现不符')
    if tables[TABLE_TOOLS]:
        raise ValueError('锄头工作簿已有配置，须按新来源修改隔离投影')
    compiled = {'interval_ms': 1000 * integer(values['mining_interval_sec'], (2**63 - 1) // 10**9, '挖矿判定秒数'),
                'success_ppm': integer(values['mining_success_ppm'], 10**6, '出矿概率'),
                'tool_wear_u': integer(values['tool_durability_cost_per_attempt'], 2**63 - 1, '锄头每次损耗')}
    layers = {}
    for row in source_rows(tables[TABLE_LAYERS], 'candidate'):
        layer = integer(row['layer'], 2, '候选矿层')
        if layer in layers or row['map_config_id'] is not None:
            raise ValueError('原矿层重复或已设置正式地图，隔离映射不能覆盖它')
        layers[layer] = integer(row['minimum_level'], 2**32 - 1, '矿层等级要求')
    if layers.keys() != {1, 2}:
        raise ValueError('工作簿必须提供比奇矿区的两个候选楼层')
    bands = []
    for row in source_rows(tables[TABLE_PURITY], 'candidate'):
        if row['within_band'] != 'uniform_integer':
            raise ValueError('本次纯度只支持已登记的档内整数均匀抽样')
        bands.append({'min': integer(row['purity_min'], 100, '纯度下界'), 'max': integer(row['purity_max'], 100, '纯度上界'),
                      'weight_ppm': integer(row['weight_ppm'], 10**6, '纯度档权重')})
    bands.sort(key=lambda row: row['min'])
    edge = 1
    for band in bands:
        if band['min'] != edge or band['max'] < edge:
            raise ValueError('矿石纯度档须不重叠地完整覆盖1至100')
        edge = band['max'] + 1
    if edge != 101 or sum(band['weight_ppm'] for band in bands) != 10**6:
        raise ValueError('纯度档缺失或总权重不等于1000000')
    return compiled, layers, bands


def compile_ores(rows, mappings):
    """矿种权重和名称来自原表，隔离映射只指定可持久化物品编号。"""
    fields(mappings, ORE_KEYS, '隔离矿石编号')
    ores, items = [], []
    for row in sorted(source_rows(rows, 'candidate'), key=lambda row: row['id']):
        key = row['ore_key']
        if key not in mappings or not row['display_name']:
            raise ValueError('矿种缺少唯一已登记的隔离物品映射')
        item = integer(mappings[key], 2**32 - 1, '矿石物品编号')
        weights = [integer(row[f'layer_{i}_weight'], 100, '矿类抽样权重') for i in (1, 2)]
        ores.append({'key': key, 'item_id': item, 'layer_weights': weights})
        items.append((item, row['display_name'], 'mining_ore'))
    if len(ores) != 4 or {row['key'] for row in ores} != ORE_KEYS or any(sum(values) != 100 for values in zip(*(r['layer_weights'] for r in ores))):
        raise ValueError('四矿种的两层权重必须独立合计100')
    return ores, items


def compile_mineral_buyback(rows, item_ids, selection, shop_keys):
    """装配只选收购商店和矿种，价格与整数取整策略始终读取候选原表。"""
    if type(selection) is not dict or not selection.keys() <= shop_keys:
        raise ValueError('矿石收购只能装配到已有 NPC 商店')
    source_by_key = {row['ore_key']: row for row in rows}
    buyback = {}
    for shop, selected in selection.items():
        if (type(selected) is not list or not selected or any(type(key) is not str or key not in item_ids for key in selected)
                or len(set(selected)) != len(selected)):
            raise ValueError('每个收购商店须明确选择至少一种已装配且不重复的矿石')
        prices = []
        for key in selected:
            source = source_by_key[key]
            numerator = integer(source['price_gold_numerator'], 2**32 - 1, '矿石收购价分子')
            denominator = integer(source['price_gold_denominator'], 2**32 - 1, '矿石收购价分母')
            rounding = source['integer_rounding_rule']
            if rounding not in ('floor_per_ore', 'exact') or (rounding == 'exact' and denominator != 1):
                raise ValueError('矿石收购价须逐块向下取整，或使用分母为1的精确整数')
            prices.append({'item_id': item_ids[key], 'numerator': numerator, 'denominator': denominator, 'rounding': rounding})
        buyback[shop] = prices
    return buyback


def compile_mining_auto_recycle(selection, item_ids, buyback):
    """自动回收只选已定价矿种，沿用同一商店的逐块计价，不复制价格数值。"""
    fields(selection, {'shop_key', 'ore_keys'}, '矿石自动回收')
    shop, keys = selection['shop_key'], selection['ore_keys']
    if type(shop) is not str or shop not in buyback or type(keys) is not list or not keys:
        raise ValueError('矿石自动回收须选择已配置收购价的商店与矿种')
    priced = {row['item_id'] for row in buyback[shop]}
    if any(type(key) is not str or item_ids.get(key) not in priced for key in keys) or len(set(keys)) != len(keys):
        raise ValueError('矿石自动回收须使用本店有价且不重复的矿种')
    return {'shop_key': shop, 'item_ids': [item_ids[key] for key in keys]}


def assemble_floor(row, public, styles):
    """校验实际地板清单和原风格引用，同时固定隔离矿区的独立坐标。"""
    geo = row['map']
    fields(geo, {'id', 'display_name', 'resource_key', 'width', 'height', 'player_spawn', 'blocked'}, '隔离矿图')
    width = integer(geo['width'], 17, '矿图宽度')
    height = integer(geo['height'], 17, '矿图高度')
    integer(geo['id'], 2**63 - 1, '矿图编号')
    if type(geo['display_name']) is not str or not geo['display_name'].strip():
        raise ValueError('矿图缺少可显示名称')
    key = geo['resource_key']
    if type(key) is not str or not re.fullmatch(r'(?:[A-Za-z0-9_-]+/)*[A-Za-z0-9_-]+', key):
        raise ValueError('隔离矿图的相对地板资源键无效')
    floor_id = integer(row['floor_map_id'], 2**63 - 1, '原地板编号')
    floor = config.load_json(RESOURCES / (key + '.json'))
    if (floor.get('id'), floor.get('width'), floor.get('height')) != (floor_id, width, height):
        raise ValueError('隔离矿图尺寸与原地板清单不符')
    style_id = str(integer(row['floor_style_map_id'], 2**63 - 1, '原区域地表编号'))
    source = next((m for m in public['map_catalog'] if m['id'] == style_id), None)
    if (source is None or (source['scene_kind'], source['resource_key'], source['width'], source['height']) != (1, key, width, height)
            or not any(s['map_id'] == style_id and (s['scene_kind'], s['width'], s['height']) == (1, width, height) for s in styles)):
        raise ValueError('原区域地表必须是具备同尺寸和地板资源的普通地图风格')

    def point(value):
        fields(value, {'x', 'y'}, '矿图坐标')
        x, y = value['x'], value['y']
        if type(x) is not int or type(y) is not int or not 0 <= x < width or not 0 <= y < height:
            raise ValueError('矿图出生或阻挡坐标越界')
        return x, y

    spawn = point(geo['player_spawn'])
    if type(geo['blocked']) is not list:
        raise ValueError('矿图须明确提供阻挡数组')
    barriers = {point(value) for value in geo['blocked']}
    if spawn in barriers or len(barriers) != len(geo['blocked']):
        raise ValueError('矿图出生点及阻挡配置冲突')
    return {**geo, 'scene_kind': 7}, str(floor_id), style_id


def project(fixture, public, tables, assembly):
    """沿原图目录及商店附加显式隔离输入，未装配的规则和资源字节不受影响。"""
    if (not all(base.get('development_only') is True and base.get('runtime_ready') is False for base in (fixture, public))
            or fixture.get('rule_version') != public.get('rule_version') or 'mining' in fixture or 'mining' in public):
        raise ValueError('矿区候选须源于同版本且缺省关闭的开发夹具与公开资源')
    required_assembly = {'layers', 'ore_item_ids'}
    optional_assembly = {'mineral_buyback', 'auto_recycle'}
    if type(assembly) is not dict or not required_assembly <= assembly.keys() <= required_assembly | optional_assembly:
        raise ValueError('隔离挖矿组装字段不完整或包含额外业务参数')
    timing, levels, bands = compile_rules(tables)
    ores, items = compile_ores(tables[TABLE_ORES], assembly['ore_item_ids'])
    if type(assembly['layers']) is not list or not 1 <= len(assembly['layers']) <= 2:
        raise ValueError('隔离组装须明确启用一层或两层')
    layers, tools, map_entries, selected, offers = [], [], [], set(), {}
    styles = config.load_json(RESOURCES / 'development/map-styles/manifest.json')['styles']
    occupied_maps = {str(r['id']) for r in public['map_catalog'] + public.get('challenge_maps', [])}
    occupied_maps.update(str(m['id']) for m in [fixture['map'], *fixture.get('maps', [])])
    for row in assembly['layers']:
        fields(row, {'layer', 'map', 'floor_map_id', 'floor_style_map_id', 'tool'}, '隔离矿层')
        layer = integer(row['layer'], 2, '矿层')
        if layer in selected:
            raise ValueError('隔离矿层重复')
        selected.add(layer)
        geo, floor_id, style_id = assemble_floor(row, public, styles)
        map_id = str(geo['id'])
        if map_id in occupied_maps:
            raise ValueError('矿图编号与现有场景重叠')
        occupied_maps.add(map_id)
        levels_for_map = {'layer': layer, 'minimum_level': levels[layer]}
        layers.append({**levels_for_map, 'map': geo})
        map_entries.append({**geo, 'id': map_id, 'name': geo['display_name'], 'floor_map_id': floor_id, 'floor_style_map_id': style_id,
                            'monster_definition_ids': [], 'initial_monster_count': 0, 'minimum_level': levels[layer], 'mining_layer': layer})
        tool = row['tool']
        fields(tool, {'item_id', 'name', 'durability_u', 'shop_key', 'offer_id', 'gold_price'}, '隔离锄头')
        tool_id = integer(tool['item_id'], 2**32 - 1, '锄头物品编号')
        durability = integer(tool['durability_u'], 2**63 - 1, '锄头耐久')
        if durability < timing['tool_wear_u'] or type(tool['name']) is not str or not tool['name'].strip():
            raise ValueError('隔离锄头须有名称及足以执行一次判定的耐久')
        offers.setdefault(tool['shop_key'], []).append({'id': integer(tool['offer_id'], 2**32 - 1, '锄头商品编号'), 'item_id': tool_id,
            'quantity': 1, 'currency': 'gold', 'price_units': integer(tool['gold_price'], 2**63 - 1, '隔离锄头价格'),
            'required_role_level': levels[layer], 'output_bound': False, 'max_purchase_quantity': 1})
        tools.append({'item_id': tool_id, 'layer': layer, 'durability_u': durability})
        items.append((tool_id, tool['name'], 'mining_tool'))
    new_ids = {item_id for item_id, _, _ in items}
    if len(new_ids) != len(items) or new_ids & {i['id'] for i in fixture['items']} or new_ids & {i['item_id'] for i in public['items']}:
        raise ValueError('隔离工具或矿石与已有物品编号重叠')
    shop_keys = {shop['key'] for shop in fixture['npc_shops']}
    if (fixture['npc_shops'] != public['npc_shops'] or not offers.keys() <= shop_keys
            or any('mineral_buyback' in shop for shop in fixture['npc_shops'])):
        raise ValueError('找不到隔离锄头商店或原服务端与公开商店配置不一致')
    buyback = compile_mineral_buyback(tables[TABLE_ORES], assembly['ore_item_ids'], assembly.get('mineral_buyback', {}), shop_keys)
    shops = []
    for shop in fixture['npc_shops']:
        key = shop['key']
        combined = [*shop['offers'], *offers.get(key, [])]
        if len({o['id'] for o in combined}) != len(combined):
            raise ValueError('隔离锄头的商品编号与原商店重复')
        entry = {**shop, 'offers': combined}
        if key in buyback:
            entry['mineral_buyback'] = buyback[key]
        shops.append(entry)
    mining = {**timing, 'layers': sorted(layers, key=lambda r: r['layer']), 'ores': ores, 'purity': bands,
              'tools': sorted(tools, key=lambda r: r['layer'])}
    if 'auto_recycle' in assembly:
        mining['auto_recycle'] = compile_mining_auto_recycle(assembly['auto_recycle'], assembly['ore_item_ids'], buyback)
    server = {**fixture, 'mining': mining, 'npc_shops': shops,
              'items': [*fixture['items'], *({'id': i, 'name': n, 'max_stack': 1} for i, n, _ in items)]}
    if any(row['id'] == 'mining' for row in public['map_regions']):
        raise ValueError('隔离矿区目录编号已被使用')
    map_entries.sort(key=lambda entry: entry['mining_layer'])
    for entry in map_entries:
        del entry['display_name']
    client = {**public, 'map_catalog': [*public['map_catalog'], *map_entries], 'npc_shops': shops,
              'map_regions': [*public['map_regions'], {'id': 'mining', 'name': '比奇矿区', 'map_ids': [m['id'] for m in map_entries]}],
              'items': [*public['items'], *({'item_id': i, 'name': n, 'kind': k, 'allowed_jobs': [0, 1, 2], 'use_enabled': False}
                                          for i, n, k in items)]}
    sources = {table: [{key: row[key] for key in ('id', 'record_status', 'source_ref')}
                       for row in tables[table] if table != TABLE_RULES or row['rule_key'] in CONFIRMED.keys() | TIMING.keys()]
               for table in (TABLE_RULES, TABLE_LAYERS, TABLE_ORES, TABLE_PURITY, TABLE_TOOLS)}
    server['_source_evidence'] = {**fixture['_source_evidence'], 'mining': {'status': 'isolated_candidate', 'default_enabled': False,
        'record_sources': sources, 'unverified_assembly': assembly}}
    return server, client


def work_path(value):
    path, parent = Path(value).resolve(), WORK.resolve()
    if path == parent or not path.is_relative_to(parent):
        raise argparse.ArgumentTypeError('矿区组装与候选输出须位于本项目 .work 子目录')
    return path


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assembly', required=True, type=work_path)
    parser.add_argument('--fixture-out', required=True, type=work_path)
    parser.add_argument('--public-out', required=True, type=work_path)
    parser.add_argument('--check', action='store_true', help='校验既有隔离投影的完整字节')
    args = parser.parse_args(argv)
    if len({args.assembly, args.fixture_out, args.public_out}) != 3:
        parser.error('矿区组装、服务端和客户端输出应当是不同文件')
    schema = config.load_json(ROOT / 'config/schema/catalog.json')
    tables, errors, _, workbooks = config.collect(schema)
    if errors:
        raise ValueError(errors)
    server, client = project(config.load_json(BASE_FIXTURE), config.load_json(BASE_PUBLIC), tables, config.load_json(args.assembly))
    server['_source_evidence']['mining'].update(workbook_sha256=config.sha256(workbooks['gameplay']),
        base_fixture_sha256=config.sha256(BASE_FIXTURE), public_base_sha256=config.sha256(BASE_PUBLIC),
        assembly_sha256=config.sha256(args.assembly), compiler_sha256=config.sha256(Path(__file__)))
    for path, output in ((args.fixture_out, server), (args.public_out, client)):
        expected = (json.dumps(output, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'矿区隔离输出与当前来源不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'default_enabled': False,
                      'fixture_sha256': config.sha256(args.fixture_out), 'public_sha256': config.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
