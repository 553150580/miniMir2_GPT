"""从唯一玩法工作簿校验普通挂机地图、怪物与互斥掉落的运行投影。"""

from collections import Counter, defaultdict
from copy import deepcopy


def by_id(records, identifier, description):
    """要求运行引用只命中一个工作簿记录。"""
    matches = [record for record in records if record['id'] == identifier]
    if len(matches) != 1 or matches[0]['record_status'] not in ('confirmed', 'candidate'):
        raise ValueError(f'{description}引用不存在或未选入开发候选：{identifier}')
    return matches[0]


def positive(value, limit=2**32-1):
    """限定运行目录使用显式正整数。"""
    return type(value) is int and 0 < value <= limit


def compile_maps(tables, profile, validate_map, policy):
    """普通地图共享同一个13格地板，独立声明坐标、初始名额与槽位怪物。"""
    ids = profile.get('ordinary_map_ids')
    initial_count = profile.get('ordinary_initial_monster_count')
    if (not isinstance(ids, list) or not ids or ids[0] != profile['map_id']
            or any(not positive(id) for id in ids) or len(set(ids)) != len(ids)
            or not positive(initial_count, 8)):
        raise ValueError('普通挂机地图及默认1至8名额须显式配置')
    primary = by_id(tables['game_maps'], ids[0], '普通地图')
    maps, respawns = [], {}
    for map_id in ids:
        definition = by_id(tables['game_maps'], map_id, '普通地图')
        if (definition['scene_kind'] != 'ordinary' or definition['required_level'] != 1
                or (definition['width_cells'], definition['height_cells']) != (13, 13)
                or definition['tile_resource_key'] != primary['tile_resource_key']
                or definition['visual_geometry'] != primary['visual_geometry']):
            raise ValueError(f'普通挂机地图须采用同一开阔13格地板及基础入场资格：{map_id}')
        collision = by_id(tables['game_map_collisions'], definition['collision_id'], '碰撞')
        slots = sorted((row for row in tables['game_monster_spawns'] if row['map_id'] == map_id), key=lambda row: row['slot_index'])
        if [row['slot_index'] for row in slots] != list(range(8)):
            raise ValueError(f'普通地图须显式配置8个连续且唯一的生成槽：{map_id}')
        for slot in slots:
            monster_id = slot['monster_id']
            seconds = slot['respawn_seconds']
            if slot['spawn_kind'] != 'ordinary' or not positive(monster_id) or not positive(seconds, 86400):
                raise ValueError(f'普通怪槽类型、身份或复活时间不合法：{slot["id"]}')
            if respawns.setdefault(monster_id, seconds) != seconds:
                raise ValueError('同一普通模板的复活时钟须在各槽保持一致')
        group = [
            row for row in tables['game_map_regions'] if row['map_id'] == map_id
            and row['entry_rules'] == {'development_only': True}
        ]
        monster_ids = [row['monster_id'] for row in slots]
        if (len(group) != 1 or group[0]['monster_pool'] !=
                [{'monster_id': id, 'slots': count} for id, count in sorted(Counter(monster_ids).items())]):
            raise ValueError(f'全图怪物谱与实际生成槽不一致：{map_id}')
        points = [row['cell'] for row in slots]
        validate_map(definition, collision, profile, points)
        rules = policy(tables, definition, profile)
        maps.append({
            'id': map_id, 'scene_kind': 1, 'display_name': definition['display_name'],
            'resource_key': definition['tile_resource_key'], 'width': definition['width_cells'],
            'height': definition['height_cells'], 'blocked': deepcopy(collision['blocked_cells']),
            'player_spawn': deepcopy(definition['spawn_cell']), 'pet_spawn': deepcopy(profile['pet_spawn']),
            'monster_spawns': deepcopy(points), 'monster_spawn_ids': monster_ids,
            'initial_monster_count': initial_count, 'no_teleport': rules['no_teleport'], 'no_pets': rules['no_pets'],
        })
    return maps, respawns


def compile_monsters(tables, fighter, respawns, gold):
    """只把实际使用的普通模板投影到战斗运行配置，并保留公开显示所需的名称与资源键。"""
    result, appearances = [], []
    for monster_id, seconds in respawns.items():
        row = by_id(tables['game_monsters'], monster_id, '普通怪')
        if (row['category'] != 'ordinary' or not positive(row['drop_pool_id'])
                or type(row['attributes'].get('can_push')) is not bool or not row['resource_key']):
            raise ValueError(f'普通怪须显式绑定可用的掉落池及外观：{monster_id}')
        body = fighter(row)
        result.append({
            **body, 'id': monster_id, 'exp': row['exp'], 'gold_min': gold['quantity_min'],
            'gold_max': gold['quantity_max'], 'respawn_ms': seconds * 1000, 'drop_pool_id': row['drop_pool_id'],
        })
        appearances.append({
            'definition_id': body['definition_id'], 'name': row['display_name'], 'profile': row['resource_key'],
            'can_push': body['can_push'],
        })
    return result, appearances


def compile_pools(tables, profile, monsters, items):
    """按怪物池选择装备与道具详情。基础四分类保持独立，不为药材新增一次抽样。"""
    selected = profile.get('ordinary_drop_pool_pairs')
    if not isinstance(selected, list) or not selected:
        raise ValueError('普通怪须配置装备池与道具池配对')
    entries = defaultdict(list)
    for row in tables['game_drop_entries']:
        entries[row['pool_id']].append(row)
    available = {row['id']: row for row in items}
    used, item_pools, result = set(), set(), []
    for pair in selected:
        if not isinstance(pair, dict) or set(pair) != {'id','item_pool_id'} or any(not positive(value) for value in pair.values()):
            raise ValueError('普通怪掉落配对须明确两类工作簿池号')
        equip_id, item_id = pair['id'], pair['item_pool_id']
        if equip_id in used or item_id in item_pools:
            raise ValueError('普通掉落配对不得重复')
        equipment = compile_entries(tables, equip_id, 'equipment', entries, available)
        consumables = compile_entries(tables, item_id, 'item', entries, available)
        result.append({'id': equip_id, 'equipment': equipment, 'items': consumables})
        used.add(equip_id)
        item_pools.add(item_id)
    if used != {row['drop_pool_id'] for row in monsters}:
        raise ValueError('普通怪引用的掉落池与本次投影的池集合不一致')
    return result


def compile_entries(tables, pool_id, category, entries, available):
    """核对同一类别的候选条目权重、物品数量、绑定来源与装备阶段。"""
    pool = by_id(tables['game_drop_pools'], pool_id, '普通掉落池')
    if pool['source_kind'] != 'ordinary_monster' or pool['category_key'] != category or pool['selection_mode'] != 'development_weighted_subset':
        raise ValueError('普通怪子池须明确使用本职业阶段的候选权重')
    equipment = category == 'equipment'
    if equipment:
        stages = pool['equipment_stage_range']
        if (not isinstance(stages, list) or len(stages) != 2 or any(type(stage) is not int for stage in stages)
                or not 0 <= stages[0] <= stages[1] or pool['max_random_quality'] not in (0, 1, 2, 3)
                or pool['quality_weights'] is not None):
            raise ValueError('普通装备池须声明阶段和随机品质上限，具体权重由装备条目维护')
    seen, result = set(), []
    for row in entries[pool_id]:
        id = row['item_id']
        item = available.get(id)
        if (not item or id in seen or item['is_equipment'] != equipment
                or row['currency_key'] is not None or row['binding_policy'] != 'kill_event_privilege'
                or any(not positive(row[key]) for key in ('weight','quantity_min','quantity_max'))
                or row['quantity_min'] > row['quantity_max']):
            raise ValueError(f'普通掉落条目的类别、数量、权重或原绑定策略无效：{row["id"]}')
        if equipment:
            if (row['quantity_max'] != 1 or not stages[0] <= item['equipment_stage'] <= stages[1]
                    or item['drop_quality'] > pool['max_random_quality']):
                raise ValueError('装备超出掉落池的阶段、品质或数量')
        elif row['quantity_max'] > (item.get('max_stack', 1) if item['stackable'] else 1):
            raise ValueError('道具掉落数量不能超过实例堆叠上限')
        seen.add(id)
        result.append({key: row[key] for key in ('item_id','weight','quantity_min','quantity_max')})
    if not result:
        raise ValueError(f'普通掉落子池不得为空：{pool_id}')
    return result


def compile_quotas(tables, pools, items):
    """只投影当前普通怪池用到的阶段与品质，不把未选中的配额视为已启用。"""
    equipment = {row['id']: (row['equipment_stage'], row['drop_quality']) for row in items if row['is_equipment']}
    selected = [(pool['id'], {equipment[row['item_id']] for row in pool['equipment']}) for pool in pools]
    needed = {key for _, entries in selected for key in entries if key[1] > 1}
    quotas = {}
    for row in tables.get('game_drop_quotas', []):
        key = row['stage'], row['quality']
        if (row['source_kind'] != 'ordinary_monster' or key not in needed
                or row['record_status'] not in ('confirmed', 'candidate')):
            continue
        if key in quotas:
            raise ValueError(f'阶段与品质存在多份普通怪配额：{key}')
        validate_quota(row)
        quotas[key] = {field: deepcopy(row[field]) for field in (
            'id', 'stage', 'quality', 'source_kind', 'window_seconds', 'quota_units',
            'refill_schedule', 'exhausted_downgrade_weights')}
    if missing := sorted(needed - quotas.keys()):
        raise ValueError(f'普通装备缺少显式高品质额度：{missing}')
    for pool_id, entries in selected:
        for stage, quality in entries & needed:
            for target in quotas[stage, quality]['exhausted_downgrade_weights']:
                if (stage, int(target)) not in entries:
                    raise ValueError(f'掉落池{pool_id}缺少阶段{stage}的降档品质{target}装备')
    return [quotas[key] for key in sorted(quotas)]


def validate_quota(row):
    """校验工作簿的分时补充与降档权重，禁止把历史经济样本补作默认值。"""
    window, capacity = row['window_seconds'], row['quota_units']
    schedule, weights = row['refill_schedule'], row['exhausted_downgrade_weights']
    if (not positive(window, (2**63-1) // 10**9) or not positive(capacity, 2**63-1)
            or not isinstance(schedule, list) or not schedule):
        raise ValueError(f'高品质额度{row["id"]}需明确正窗口、额度和释放计划')
    previous, total = -1, 0
    for refill in schedule:
        if not isinstance(refill, dict) or refill.keys() != {'offset_seconds', 'units'}:
            raise ValueError(f'高品质额度{row["id"]}需明确释放偏移与件数')
        offset, units = refill['offset_seconds'], refill['units']
        if type(offset) is not int or not previous < offset < window or not positive(units, capacity - total):
            raise ValueError(f'高品质额度{row["id"]}的释放偏移或件数无效')
        previous, total = offset, total + units
    if total != capacity or previous == 0:
        raise ValueError(f'高品质额度{row["id"]}须完整分时释放，窗首不能一次给足')
    possible = {str(quality) for quality in range(row['quality'])}
    if (not isinstance(weights, dict) or not weights or not weights.keys() <= possible
            or any(not positive(weight, 2**63-1) for weight in weights.values()) or sum(weights.values()) > 2**63-1):
        raise ValueError(f'高品质额度{row["id"]}的降档权重无效')


def public_fields(row, floor_id, definitions):
    """公开地图身份、静态地板引用及该图可观察的怪物外观数量。"""
    if 'monster_spawn_ids' not in row:
        return {}
    fields = {
        'monster_definition_ids': list(dict.fromkeys(definitions[id] for id in row['monster_spawn_ids'])),
        'initial_monster_count': row['initial_monster_count'],
    }
    if row['id'] != floor_id:
        fields.update(floor_map_id=str(floor_id), player_spawn=deepcopy(row['player_spawn']), blocked=deepcopy(row['blocked']))
    return fields
