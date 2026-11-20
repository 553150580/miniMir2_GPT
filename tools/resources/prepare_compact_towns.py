"""校验唯一玩法工作簿内的17×17城镇配置并生成双端投影。"""

from collections import deque
from copy import deepcopy
import re


SIZE = 17
POINT_FIELDS = {'x', 'y'}
TOWN_FIELDS = {'id', 'name', 'player_spawn', 'safe_zone', 'legacy_safe_zone_ids', 'no_pets', 'no_teleport', 'npcs'}
NPC_FIELDS = {'id', 'name', 'role', 'position', 'profile', 'source'}
SOURCE_FIELDS = {'table', 'sha256', 'line', 'map_code', 'position', 'appearance'}


def check_fields(value, fields, description):
    if not isinstance(value, dict) or set(value) != fields:
        raise ValueError(f'{description}字段不完整或包含未知字段')


def positive_id(value):
    return type(value) is int and 0 < value <= 2**32 - 1


def point(value, limit):
    check_fields(value, POINT_FIELDS, '城镇坐标')
    x, y = value['x'], value['y']
    if any(type(axis) is not int or not 0 <= axis < limit for axis in (x, y)):
        raise ValueError('城镇坐标越界')
    return x, y


def validate_reachability(spawn, blocked):
    seen = {spawn}
    pending = deque([spawn])
    while pending:
        x, y = pending.popleft()
        for neighbor in ((x - 1, y), (x + 1, y), (x, y - 1), (x, y + 1)):
            if neighbor not in seen and neighbor not in blocked and all(0 <= axis < SIZE for axis in neighbor):
                seen.add(neighbor)
                pending.append(neighbor)
    if len(seen) != SIZE * SIZE - len(blocked):
        raise ValueError('城镇所有可站立格均须从入口可达')


def compile_npcs(rows, npc_ids):
    if not isinstance(rows, list) or not rows:
        raise ValueError('每个紧凑城镇都必须配置对应NPC')
    npcs, provenance, occupied = [], [], set()
    for row in rows:
        check_fields(row, NPC_FIELDS, '城镇NPC')
        if not positive_id(row['id']) or row['id'] in npc_ids or any(
                not isinstance(row[key], str) or not row[key].strip() for key in ('name', 'role')):
            raise ValueError('城镇NPC编号重复或名称、职责无效')
        if row['profile'] not in ('player_male', 'player_female'):
            raise ValueError('城镇NPC须使用已提供的站立素材')
        position = point(row['position'], SIZE)
        if position in occupied or any(axis in (0, SIZE - 1) for axis in position):
            raise ValueError('城镇NPC位置重复或位于地图边缘')
        source = row['source']
        check_fields(source, SOURCE_FIELDS, '城镇NPC来源')
        if (source['table'] not in ('MerChant.txt', 'Npcs.txt') or not isinstance(source['sha256'], str)
                or not re.fullmatch(r'[a-f0-9]{64}', source['sha256']) or not positive_id(source['line'])
                or not isinstance(source['map_code'], str) or not re.fullmatch(r'[A-Za-z\d]+', source['map_code'])
                or type(source['appearance']) is not int or not 0 <= source['appearance'] <= 65535):
            raise ValueError('城镇NPC必须保留原表、摘要、行号、地图及外观证据')
        point(source['position'], 65536)
        npc_ids.add(row['id'])
        occupied.add(position)
        npcs.append({key: deepcopy(value) for key, value in row.items() if key != 'source'})
        provenance.append({'id': row['id'], **deepcopy(source)})
    return npcs, provenance, occupied


def compile_compact_towns(profile, legacy_maps, personal_map):
    if profile['rule_version'] < 24:
        return [], []
    rows = profile.get('compact_towns')
    if not isinstance(rows, list) or not rows:
        raise ValueError('rule24必须由唯一玩法工作簿提供城镇清单')
    travel = profile.get('town_travel')
    if not isinstance(travel, dict) or not positive_id(travel.get('town_map_id')):
        raise ValueError('紧凑城镇必须配置历史绑定地图编号')
    legacy_id = travel['town_map_id']
    legacy_map = next((row for row in legacy_maps if row['id'] == legacy_id), None)
    if legacy_map is None or not legacy_map['safe_zones']:
        raise ValueError('紧凑城镇缺少用于历史绑定核验的来源地图')
    legacy_zones = {zone['id']: zone for zone in legacy_map['safe_zones']}
    map_ids = {personal_map['id'], *(row['id'] for row in legacy_maps)}
    zone_ids, npc_ids, names, consumed_zones = set(legacy_zones), set(), set(), set()
    maps, evidence = [], []
    for row in rows:
        check_fields(row, TOWN_FIELDS, '紧凑城镇')
        town_id, name = row['id'], row['name']
        if (not positive_id(town_id) or town_id in map_ids or not isinstance(name, str)
                or not name.strip() or name in names):
            raise ValueError('紧凑城镇地图号重复或名称无效')
        if any(type(row[key]) is not bool for key in ('no_pets', 'no_teleport')):
            raise ValueError('紧凑城镇须显式声明宠物和瞬移规则')
        mapping = row['legacy_safe_zone_ids']
        if (not isinstance(mapping, list) or not mapping or any(not positive_id(value) for value in mapping)
                or len(set(mapping)) != len(mapping) or any(value not in legacy_zones or value in consumed_zones for value in mapping)):
            raise ValueError('历史安全区须准确且唯一对应一个紧凑城镇')
        if any(legacy_zones[value]['center'] != legacy_zones[mapping[0]]['center'] for value in mapping):
            raise ValueError('同一城镇的历史安全区必须共享来源中心')
        zone = row['safe_zone']
        check_fields(zone, {'id', 'center', 'radius'}, '紧凑城镇安全区')
        center = point(zone['center'], SIZE)
        radius = zone['radius']
        if (not positive_id(zone['id']) or zone['id'] in zone_ids or type(radius) is not int or radius <= 0
                or any(axis - radius < 0 or axis + radius >= SIZE for axis in center)):
            raise ValueError('紧凑城镇安全区编号重复或范围越界')
        spawn = point(row['player_spawn'], SIZE)
        npcs, provenance, blocked = compile_npcs(row['npcs'], npc_ids)
        if spawn in blocked or center in blocked:
            raise ValueError('城镇入口和安全区中心须为可行走格')
        validate_reachability(spawn, blocked)
        map_ids.add(town_id)
        zone_ids.add(zone['id'])
        names.add(name)
        consumed_zones.update(mapping)
        maps.append({'id': town_id, 'display_name': name, 'resource_key': personal_map['resource_key'],
                     'scene_kind': 2, 'compact_town': True, 'width': SIZE, 'height': SIZE,
                     'player_spawn': deepcopy(row['player_spawn']),
                     'blocked': [deepcopy(npc['position']) for npc in npcs], 'safe_zones': [deepcopy(zone)],
                     'legacy_safe_zone_ids': list(mapping), 'no_pets': row['no_pets'], 'no_teleport': row['no_teleport'],
                     'npcs': npcs})
        evidence.append({'map_id': town_id, 'source_map_id': legacy_id,
                         'legacy_safe_zones': [deepcopy(legacy_zones[value]) for value in mapping], 'npcs': provenance})
    if consumed_zones != set(legacy_zones):
        raise ValueError('历史安全区尚有未对应的紧凑城镇')
    return maps, evidence


def public_fields(row):
    if not row.get('compact_town'):
        return {}
    return {'compact_town': True, 'player_spawn': deepcopy(row['player_spawn']), 'blocked': deepcopy(row['blocked']),
            'npcs': [{**deepcopy(npc), 'id': str(npc['id'])} for npc in row['npcs']]}


def compile_map_regions(profile, available_maps):
    rows = profile.get('map_regions')
    if not isinstance(rows, list) or not rows:
        raise ValueError('地图菜单须由唯一玩法工作簿提供区域清单')
    eligible = {row['id']: row.get('scene_kind', 1) for row in available_maps}
    region_ids, region_names, used_maps = set(), set(), set()
    result = []
    for row in rows:
        check_fields(row, {'id', 'name', 'map_ids'}, '地图菜单区域')
        region_id, name, members = row['id'], row['name'], row['map_ids']
        if (not isinstance(region_id, str) or not re.fullmatch(r'[A-Za-z0-9_-]{1,32}', region_id)
                or region_id in region_ids or not isinstance(name, str) or not name.strip() or name in region_names):
            raise ValueError('地图菜单的区域编号或名称重复或无效')
        if (not isinstance(members, list) or not members or any(not positive_id(value) for value in members)
                or len(set(members)) != len(members)
                or any(value in used_maps or eligible.get(value) not in (1, 2) for value in members)):
            raise ValueError('地图菜单的成员重复或不允许旅行')
        if not any(eligible[value] == 1 for value in members):
            raise ValueError('每个地图区域至少需要一个野外挂机地图')
        result.append(deepcopy(row))
        region_ids.add(region_id)
        region_names.add(name)
        used_maps.update(members)
    if used_maps != set(eligible):
        raise ValueError('地图菜单必须覆盖全部旅行地图')
    return result
