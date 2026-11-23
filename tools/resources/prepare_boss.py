"""从唯一玩法表的显式候选投影个人首领和世界首领的独立运行配置。"""

from copy import deepcopy
import re


TABLES = ('game_boss_templates', 'game_challenge_rules', 'game_entitlement_rules')
KINDS = {'personal': 3, 'world': 4}
DAY_MS = 24 * 60 * 60 * 1000


def integer(value, minimum, maximum, field):
    """检验整数边界，防止布尔值或超宽数值进入服务端 JSON。"""
    if type(value) is not int or not minimum <= value <= maximum:
        raise ValueError(f'首领配置 {field} 超出整数范围')
    return value


def selected_row(tables, table, identifier):
    """仅允许显式选择当前已登记的配置行。"""
    row = next((row for row in tables[table] if row['id'] == identifier), None)
    if row is None or row['record_status'] not in ('candidate', 'confirmed'):
        raise ValueError(f'首领缺少候选来源 {table}/{identifier}')
    return row


def confirmed(tables, table, rule):
    """运营制度直接引用策划已经确认的规则行。"""
    rows = [row for row in tables[table] if row['rule_key'] == rule and row['record_status'] == 'confirmed']
    if len(rows) != 1:
        raise ValueError(f'缺少唯一确认的首领规则 {rule}')
    return rows[0]['rule_value']


def point(value, width, height, field):
    """将来源坐标核对为网格内的明确整数点。"""
    if not isinstance(value, dict) or set(value) != {'x', 'y'}:
        raise ValueError(f'首领配置 {field} 坐标格式不合法')
    return {'x': integer(value['x'], 0, width - 1, field), 'y': integer(value['y'], 0, height - 1, field)}


def world_schedule(tables):
    """把上海小时分刻度转换成日内毫秒，同时拒绝跨场重叠。"""
    rules = 'game_challenge_rules'
    times = confirmed(tables, rules, 'world_boss_daily_start_local')
    if not isinstance(times, list) or not times or any(not isinstance(value, str) or not re.fullmatch(r'(?:[01][0-9]|2[0-3]):[0-5][0-9]', value) for value in times):
        raise ValueError('世界首领上海日程须使用有效的 HH:MM')
    offsets = [(int(value[:2]) * 60 + int(value[3:])) * 60000 for value in times]
    window = integer(confirmed(tables, rules, 'world_boss_window_sec'), 1, DAY_MS // 1000 - 1, '世界窗口') * 1000
    recovery = integer(confirmed(tables, rules, 'world_boss_return_after_kill_sec'), 0, window // 1000 - 1, '返回等待') * 1000
    if any(later - prior < window + recovery for prior, later in zip(offsets, offsets[1:] + [offsets[0] + DAY_MS])):
        raise ValueError('世界首领日程重叠或未严格排序')
    return {'daily_open_offsets_ms': offsets, 'window_ms': window, 'return_delay_ms': recovery}


def project(tables, profile, ordinary_map, generic_map_ids, fighter):
    """从模板、地图、碰撞、出生点和怪物独立记录编译受控开发候选。"""
    selection = profile.get('boss_template_ids')
    if selection is None:
        return {}, {}
    if profile['rule_version'] < 28 or not isinstance(selection, list) or len(selection) != len(KINDS) or any(type(identity) is not int for identity in selection) or len(set(selection)) != len(KINDS):
        raise ValueError('须在规则28起明确选择个人和世界首领模板')
    result = {'challenge_monsters': []}
    evidence = {'monster_profiles': [], 'floor_map_ids': {}}
    seen_maps = set(generic_map_ids)
    seen_monsters = {profile['monster_id'], *(row['monster_template_id'] for row in tables['game_pet_templates'])}
    visual = selected_row(tables, 'game_monsters', profile['monster_id'])['resource_key']
    for identity in selection:
        integer(identity, 1, (1 << 32) - 1, '模板编号')
        row = selected_row(tables, 'game_boss_templates', identity)
        kind = row['boss_kind']
        if kind not in KINDS or kind + '_boss' in result:
            raise ValueError('仅允许一个个人和一个世界首领模板')
        source_map = selected_row(tables, 'game_maps', row['map_id'])
        source_monster = selected_row(tables, 'game_monsters', row['monster_id'])
        if source_monster['resource_key'] != visual or any(type(source_monster['attributes'].get(field)) is not bool for field in ('undead', 'can_push')):
            raise ValueError('候选首领需要当前已登记的可用形象及明确实体规则')
        collision = selected_row(tables, 'game_map_collisions', source_map['collision_id'])
        map_id = integer(source_map['id'], 1, (1 << 63) - 1, '地图编号')
        monster_id = integer(source_monster['id'], 1, (1 << 32) - 1, '怪物编号')
        if map_id in seen_maps or monster_id in seen_monsters or source_map['scene_kind'] != kind + '_boss' or source_monster['category'] != kind + '_boss':
            raise ValueError('首领地图或怪物不能与其他玩法共用身份')
        seen_maps.add(map_id)
        seen_monsters.add(monster_id)
        width = integer(source_map['width_cells'], 1, 17, '地图宽度')
        height = integer(source_map['height_cells'], 1, 17, '地图高度')
        floor = source_map['visual_geometry'].get('floor_map_id')
        if (width, height) != (ordinary_map['width'], ordinary_map['height']) or floor != ordinary_map['id'] or source_map['tile_resource_key'] != ordinary_map['resource_key'] or source_map['decorations'] != []:
            raise ValueError('当前首领候选须引用既有同尺寸空地板')
        if (collision['width_cells'], collision['height_cells'], collision['map_key']) != (width, height, source_map['map_key']) or collision['walkable_encoding'] != 'blocked_xy_objects':
            raise ValueError('首领地图碰撞来源不符')
        blocked = [point(cell, width, height, '阻挡') for cell in collision['blocked_cells']]
        if len({(cell['x'], cell['y']) for cell in blocked}) != len(blocked):
            raise ValueError('首领地图包含重复阻挡')
        spawn = [candidate for candidate in tables['game_monster_spawns'] if candidate['map_id'] == map_id]
        if (len(spawn) != 1 or spawn[0]['record_status'] not in ('candidate', 'confirmed') or spawn[0]['monster_id'] != monster_id
                or spawn[0]['spawn_kind'] != kind + '_boss' or spawn[0]['respawn_seconds'] != 0):
            raise ValueError('独立首领地图需要唯一首领出生格和零普通复活时间')
        player = point(source_map['spawn_cell'], width, height, '玩家出生')
        pet = point(profile['pet_spawn'], width, height, '宠物出生')
        target = point(spawn[0]['cell'], width, height, '首领出生')
        if any(cell in blocked for cell in (player, pet, target)) or player == target or pet == target:
            raise ValueError('首领、玩家或宠物出生格冲突')
        min_level = integer(row['required_level'], 1, (1 << 32) - 1, '入场等级')
        participation = row['participation_rule']
        expected = 'duration_seconds' if kind == 'personal' else 'participant_limit'
        if (source_map['required_level'] != min_level or row.get('rank_reward_group_id') is not None or row.get('participation_reward_group_id') is not None
                or source_monster['exp'] != 0 or source_monster['drop_pool_id'] not in (None, 0) or set(participation) != {'development_enabled', expected}
                or participation['development_enabled'] is not True):
            raise ValueError('首领候选必须明确开放，且不能套用怪物经验或普通与正式奖励组')
        grid = {'id': map_id, 'display_name': source_map['display_name'], 'scene_kind': KINDS[kind],
                'resource_key': source_map['tile_resource_key'], 'width': width, 'height': height, 'blocked': blocked,
                'no_teleport': True, 'no_pets': False, 'player_spawn': player, 'pet_spawn': pet, 'monster_spawns': [target]}
        entry = {'enabled': True, 'template_id': identity, 'display_name': source_monster['display_name'],
                 'map': grid, 'monster_id': monster_id, 'minimum_level': min_level}
        if kind == 'personal':
            rules = 'game_challenge_rules'
            if confirmed(tables, rules, 'personal_boss_failed_consumes_success') is not False:
                raise ValueError('当前个人首领仅支持成功计次')
            entry.update(duration_ms=integer(participation[expected], 1, DAY_MS // 1000, '挑战时限') * 1000,
                         success_cooldown_ms=integer(row['cooldown_seconds'], 1, DAY_MS // 1000, '成功冷却') * 1000,
                         retry_cooldown_ms=integer(row['failure_cooldown_seconds'], 1, DAY_MS // 1000, '失败冷却') * 1000,
                         daily_success_limit=integer(confirmed(tables, rules, 'personal_boss_daily_success_base'), 1, (1 << 32) - 1, '成功次数'),
                         privilege_success_bonus=integer(confirmed(tables, 'game_entitlement_rules', 'privilege_personal_boss_success_bonus'), 0, (1 << 32) - 1, '特权次数'),
                         history_limit=integer(confirmed(tables, rules, 'personal_boss_history_visible_limit'), 1, 100, '显示历史'))
        else:
            if row['cooldown_seconds'] != 0 or row['failure_cooldown_seconds'] != 0:
                raise ValueError('世界首领由每天的确认场次控制，不启用个人冷却')
            entry.update(world_schedule(tables), participant_limit=integer(participation[expected], 2, width * height - len(blocked) - 1, '共享人数'))
        result[kind + '_boss'] = entry
        result['challenge_monsters'].append({**fighter(source_monster), 'id': monster_id, 'exp': 0, 'gold_min': 0, 'gold_max': 0, 'respawn_ms': 0})
        evidence['floor_map_ids'][str(map_id)] = str(floor)
        evidence['monster_profiles'].append({'definition_id': monster_id, 'name': source_monster['display_name'],
                                             'profile': source_monster['resource_key'], 'can_push': source_monster['attributes']['can_push']})
    return result, evidence


def public_fields(fixture):
    """客户端只取得挑战地图、模型和日程展示字段，不拥有场次或奖励规则。"""
    evidence = fixture.get('_source_evidence', {}).get('boss_challenges')
    if evidence is None:
        return {}, []
    maps = []
    for kind in KINDS:
        grid = fixture[kind + '_boss']['map']
        identity = str(grid['id'])
        maps.append({'id': identity, 'name': grid['display_name'], 'resource_key': grid['resource_key'],
                     'floor_map_id': evidence['floor_map_ids'][identity],
                     **{field: deepcopy(grid[field]) for field in ('scene_kind', 'width', 'height', 'blocked', 'player_spawn')}})
    world = fixture['world_boss']
    return {'challenge_maps': maps, 'world_boss': {'map_id': str(world['map']['id']),
            **{field: deepcopy(world[field]) for field in ('enabled', 'template_id', 'display_name', 'monster_id', 'minimum_level', 'daily_open_offsets_ms', 'window_ms', 'return_delay_ms')}}}, deepcopy(evidence['monster_profiles'])
