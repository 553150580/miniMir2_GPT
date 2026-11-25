#!/usr/bin/env python3
"""从唯一玩法工作簿和显式装配生成隔离镜像竞技规则，不修改默认试玩。"""

from __future__ import annotations

import argparse
from collections import deque
from copy import deepcopy
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import re
import sys


ROOT = Path(__file__).resolve().parents[2]
WORK = ROOT / '.work'
BASE_FIXTURE = ROOT / 'config/development/first-loop.json'
BASE_PUBLIC = ROOT / 'client/assets/resources/development/gameplay.json'
sys.path.insert(0, str(ROOT / 'tools/config'))
import export_configs as configuration


RULES = 'game_arena_rules'
STREAKS = 'game_arena_streak_points'
REWARDS = 'game_arena_rewards'
SHANGHAI = timezone(timedelta(hours=8))
SCENE_KIND = 6
MAX_UINT32 = (1 << 32) - 1
MAX_INT64 = (1 << 63) - 1
TIMEOUT = 'effective_surviving_hp_ratio_defender_wins_tie'
RANKING_TIEBREAK = 'earlier_last_challenge'
SETTLEMENT_SEQUENCE = ['rank_rewards', 'score_to_exchange_1_to_1', 'clear_season_score']
SETTLEMENT_RULES = ('ranking_tiebreak', 'settlement_sequence', 'exchange_balance_persists', 'participation_min_challenges')
MATCH_POLICIES = {'same_job_level_window', 'cross_job_level_window'}
PET_POLICIES = {'owner_death_ends_pet', 'surviving_pet_continues'}
SPAWN_FIELDS = ('attacker_spawn', 'defender_spawn', 'attacker_pet_spawn', 'defender_pet_spawn')
ASSEMBLY_FIELDS = {'template_id', 'display_name', 'map_id', 'floor_map_id', 'season_anchor',
                   'match_policy', 'max_level_gap', 'pet_death_policy', 'blocked', *SPAWN_FIELDS}


def require_integer(value, minimum, maximum, name):
    """整数保持原值，避免布尔值、浮点数或越界数字进入权威规则。"""
    if type(value) is not int or not minimum <= value <= maximum:
        raise ValueError(f'镜像竞技 {name} 需要范围内的整数')
    return value


def require_rule(tables, name, status):
    """只读取当前唯一且经过登记的指定状态规则。"""
    matches = [row for row in tables[RULES] if row['rule_key'] == name and row['record_status'] == status]
    if len(matches) != 1:
        raise ValueError(f'镜像竞技缺少唯一的 {status} 来源规则 {name}')
    return matches[0]['rule_value']


def read_anchor(value):
    """归一化显式的运营零点，缺失时不从系统日期猜测赛季。"""
    if not isinstance(value, str) or not re.fullmatch(r'\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:Z|[+-]\d{2}:\d{2})', value):
        raise ValueError('镜像竞技赛季零点须明确写入带时区的 RFC 3339 时刻')
    try:
        at = datetime.fromisoformat(value).astimezone(SHANGHAI)
    except (OverflowError, ValueError) as error:
        raise ValueError('镜像竞技赛季起点日期无效') from error
    if at < datetime(1970, 1, 1, tzinfo=timezone.utc) or (at.hour, at.minute, at.second) != (0, 0, 0):
        raise ValueError('镜像竞技赛季起点必须是 Unix 纪元后的上海零点')
    return at.isoformat(timespec='seconds')


def grid_points(assembly, width, height):
    """独立装配的阻挡与四个落点须各自唯一，而且能经同一网格抵达。"""
    def point(value, field):
        if not isinstance(value, dict) or set(value) != {'x', 'y'}:
            raise ValueError(f'镜像竞技 {field} 坐标格式不合法')
        return (require_integer(value['x'], 0, width - 1, field),
                require_integer(value['y'], 0, height - 1, field))

    if not isinstance(assembly['blocked'], list):
        raise ValueError('镜像竞技阻挡格须显式给出数组')
    blocked = {point(cell, '阻挡格') for cell in assembly['blocked']}
    spawns = {field: point(assembly[field], field) for field in SPAWN_FIELDS}
    if len(blocked) != len(assembly['blocked']) or len(set(spawns.values())) != len(spawns) or blocked.intersection(spawns.values()):
        raise ValueError('镜像竞技阻挡或座位格重复、冲突')
    start = spawns['attacker_spawn']
    pending, visited = deque([start]), {start}
    while pending:
        x, y = pending.popleft()
        for next_x, next_y in ((x - 1, y), (x + 1, y), (x, y - 1), (x, y + 1)):
            node = (next_x, next_y)
            if 0 <= next_x < width and 0 <= next_y < height and node not in visited and node not in blocked:
                pending.append(node)
                visited.add(node)
    if not visited.issuperset(spawns.values()):
        raise ValueError('镜像竞技座位之间被阻挡地形隔开')
    return {field: {'x': x, 'y': y} for field, (x, y) in spawns.items()}


def streak_points(tables, base):
    """保持工作簿原始档位，拒绝未经确认的单行或相互矛盾的升档。"""
    rows = [row for row in tables[STREAKS] if row['record_status'] != 'historical']
    if not 0 < len(rows) <= 32 or any(row['record_status'] != 'confirmed' for row in rows):
        raise ValueError('镜像竞技连续胜利档位须全部来自已确认数据')
    tiers = sorted((require_integer(row['streak_wins'], 1, MAX_UINT32, '连胜场数'),
                    require_integer(row['win_points'], 1, MAX_INT64, '连胜积分')) for row in rows)
    if tiers[0] != (1, base) or any(first[0] >= later[0] or first[1] > later[1] for first, later in zip(tiers, tiers[1:])):
        raise ValueError('镜像竞技起始连胜积分、连胜档位或积分顺序矛盾')
    return [{'streak': wins, 'points': points} for wins, points in tiers]


def project_settlement(tables, sample, base_fixture, arena_rule):
    """验收用奖励量只能显式装配；结算顺序、参与门槛与兑换候选原值继续取唯一工作簿。"""
    if any(row['record_status'] != 'historical' for row in tables[REWARDS]):
        raise ValueError('竞技正式奖表已有当前记录，须按奖表核对并投影，不能用隔离奖励覆盖')
    if not isinstance(sample, dict) or set(sample) not in ({'ranks', 'participation'}, {'ranks', 'participation', 'exchange'}):
        raise ValueError('镜像赛季隔离装配需显式指定排名档、参与奖和可选的绑定兑换')
    confirmed = lambda name: require_rule(tables, name, 'confirmed')
    if (confirmed('ranking_tiebreak') != RANKING_TIEBREAK or confirmed('settlement_sequence') != SETTLEMENT_SEQUENCE
            or confirmed('exchange_balance_persists') is not True):
        raise ValueError('镜像赛季排名与结转规则和当前已支持的工作簿规则不同')
    threshold = require_integer(confirmed('participation_min_challenges'), 1,
                                min(MAX_UINT32, arena_rule['season_days'] * arena_rule['daily_limit']), '赛季参与场数')

    def reward(grant):
        if not isinstance(grant, dict) or set(grant) != {'bound_yuanbao_units'}:
            raise ValueError('镜像赛季隔离奖仅允许已支持的绑定元宝最小整数单位')
        return {'bound_yuanbao_units': require_integer(grant['bound_yuanbao_units'], 1, MAX_INT64, '赛季绑定元宝')}

    participation = reward(sample['participation'])
    ranks = sample['ranks']
    if not isinstance(ranks, list) or not 0 < len(ranks) <= 32:
        raise ValueError('镜像赛季隔离排名档数量不合法')
    last = 0
    validated = []
    for rank in ranks:
        if not isinstance(rank, dict) or set(rank) != {'first_rank', 'last_rank', 'reward'}:
            raise ValueError('镜像赛季隔离排名档缺少开始名次、结束名次或奖励')
        first_rank = require_integer(rank['first_rank'], last + 1, MAX_UINT32, '排名档起始名次')
        last = require_integer(rank['last_rank'], first_rank, MAX_UINT32, '排名档末名')
        grant = reward(rank['reward'])
        if grant['bound_yuanbao_units'] + participation['bound_yuanbao_units'] > MAX_INT64:
            raise ValueError('镜像赛季名次奖与参与奖汇总超过整数上限')
        validated.append({'first_rank': first_rank, 'last_rank': last, 'reward': grant})
    result = {'settlement': {'ranking_tiebreak': RANKING_TIEBREAK, 'participation_min_challenges': threshold,
                             'ranks': validated, 'participation': participation}}

    if 'exchange' in sample:
        exchange = sample['exchange']
        if not isinstance(exchange, dict) or set(exchange) != {'item_id', 'quantity'}:
            raise ValueError('镜像兑换隔离输入只允许指定既有物品编号与单份数量')
        if confirmed('feather_exchange_bound') is not True:
            raise ValueError('镜像兑换不允许改变工作簿已定的来源绑定')
        identity = require_integer(exchange['item_id'], 1, MAX_UINT32, '本轮隔离兑换物品编号')
        item = next((row for row in base_fixture['items'] if row['id'] == identity), None)
        if not item or item.get('is_equipment') is not False or item.get('stackable') is not True:
            raise ValueError('镜像兑换须引用当前目录中已存在的可堆叠非装备物品')
        quantity = require_integer(exchange['quantity'], 1, item['max_stack'], '单份兑换数量')
        price = require_integer(require_rule(tables, 'feather_exchange_points', 'candidate'), 1, MAX_INT64, '候选兑换积分')
        result['exchange'] = {'item_id': identity, 'price': price, 'quantity': quantity}
    return result


def project(tables, assembly, base_fixture, settlement_rewards=None):
    """编译显式隔离场景。工作簿提供制度，装配只提供本轮场景和待验证的候选选择。"""
    if not isinstance(assembly, dict) or set(assembly) != ASSEMBLY_FIELDS:
        raise ValueError('镜像竞技隔离装配需完整声明模板、场景、四个座位、零点、匹配和宠物规则')
    policy, pet = assembly['match_policy'], assembly['pet_death_policy']
    if not isinstance(policy, str) or policy not in MATCH_POLICIES or not isinstance(pet, str) or pet not in PET_POLICIES:
        raise ValueError('镜像竞技仅允许显式选择已实现的隔离匹配与宠物规则')
    # 首发匹配策略仍未确认。隔离样本选择不代表这条工作簿已获得确认。
    if require_rule(tables, 'match_same_or_cross_job', 'unverified') is not None:
        raise ValueError('镜像竞技匹配规则已有来源变化，须重新核对隔离政策')
    if (require_rule(tables, 'loss_resets_attack_streak', 'confirmed') is not True
            or require_rule(tables, 'defender_loses_points', 'confirmed') is not False):
        raise ValueError('镜像竞技积分原规则与当前玩法支持范围不符')
    timeout = require_rule(tables, 'timeout_resolution', 'candidate')
    if timeout != TIMEOUT:
        raise ValueError('镜像竞技超时比较规则缺少已实现的候选来源')

    identity = require_integer(assembly['map_id'], 1, MAX_INT64, '地图编号')
    floor_id = require_integer(assembly['floor_map_id'], 1, MAX_INT64, '地板地图编号')
    template_id = require_integer(assembly['template_id'], 1, MAX_UINT32, '模板编号')
    display_name = assembly['display_name']
    if not isinstance(display_name, str) or not display_name.strip():
        raise ValueError('镜像竞技装配缺少显示名称')
    ordinary = [base_fixture['map'], *base_fixture.get('maps', [])]
    existing = {entry['id'] for entry in ordinary}
    for release in ('personal_boss', 'world_boss'):
        if primary := base_fixture.get(release):
            existing.add(primary['map']['id'])
        existing.update(row['map']['id'] for row in base_fixture.get(release + '_variants', []))
    existing.update(row['map']['id'] for row in base_fixture.get('dungeons', []))
    if identity in existing:
        raise ValueError('镜像竞技地图编号占用现有普通或特殊场景')
    floor = next((row for row in ordinary if row['id'] == floor_id and row.get('scene_kind', 1) == 1), None)
    if floor is None:
        raise ValueError('镜像竞技装配必须明确选择既有个人图地板')
    width = require_integer(floor['width'], 1, 17, '地图宽度')
    height = require_integer(floor['height'], 1, 17, '地图高度')
    key = floor['resource_key']
    if not isinstance(key, str) or not re.fullmatch(r'development/maps/[A-Za-z0-9_-]+(?:/[A-Za-z0-9_-]+)*', key):
        raise ValueError('镜像竞技地板尚无可用的开发原画位置')
    spawns = grid_points(assembly, width, height)
    grid = {'id': identity, 'display_name': display_name.strip(), 'resource_key': key, 'scene_kind': SCENE_KIND,
            'width': width, 'height': height, 'blocked': deepcopy(assembly['blocked']), 'no_teleport': True,
            'no_pets': False, 'player_spawn': spawns['attacker_spawn'], 'pet_spawn': spawns['attacker_pet_spawn']}
    confirmed = lambda name: require_rule(tables, name, 'confirmed')
    candidate = lambda name: require_rule(tables, name, 'candidate')
    win_base = require_integer(confirmed('win_points_base'), 1, MAX_INT64, '基础胜利积分')
    rule = {'enabled': True, 'template_id': template_id, 'display_name': grid['display_name'], 'map': grid, **spawns,
            'season_anchor': read_anchor(assembly['season_anchor']),
            'season_days': require_integer(confirmed('season_days'), 1, 366, '赛季天数'),
            'daily_limit': require_integer(confirmed('daily_challenge_max'), 1, MAX_UINT32, '每日挑战上限'),
            'opponent_limit': require_integer(candidate('daily_opponents'), 1, 100, '候选数量'),
            'attack_history_limit': require_integer(confirmed('challenge_records_visible_limit'), 1, 100, '进攻战报数量'),
            'defense_history_limit': require_integer(confirmed('defended_records_visible_limit'), 1, 100, '防守战报数量'),
            'duration_ms': require_integer(candidate('battle_timeout_sec'), 1, 3600, '战斗时限') * 1000,
            'win_base_points': win_base, 'loss_points': require_integer(confirmed('loss_points'), 0, MAX_INT64, '进攻失败积分'),
            'streak_points': streak_points(tables, win_base), 'timeout_policy': timeout, 'match_policy': policy,
            'max_level_gap': require_integer(assembly['max_level_gap'], 0, MAX_UINT32, '候选等级差'), 'pet_death_policy': pet}
    if settlement_rewards is not None:
        rule.update(project_settlement(tables, settlement_rewards, base_fixture, rule))
    public_map = {'id': str(identity), 'name': grid['display_name'], 'resource_key': key, 'floor_map_id': str(floor_id),
                  'scene_kind': SCENE_KIND, 'width': width, 'height': height, 'blocked': deepcopy(grid['blocked']),
                  'player_spawn': dict(spawns['attacker_spawn']), 'monster_definition_ids': []}
    return rule, public_map


def output_file(value):
    """候选输出只能落到临时工作目录，不改规则 30 默认入口。"""
    target = Path(value).resolve()
    if not target.is_relative_to(WORK.resolve()) or target == WORK.resolve() or target.suffix != '.json':
        raise ValueError('镜像竞技隔离投影只输出 .work 下的 JSON 文件')
    return target


def write_projection(path, value, check):
    """统一编码和换行，支持在隔离验收中逐字节核对相同输入。"""
    raw = (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
    if check:
        if not path.is_file() or path.read_bytes() != raw:
            raise ValueError(f'镜像竞技隔离投影已经过期 {path}')
        return
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(raw)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--assembly', required=True, type=Path, help='本轮显式场景装配 JSON')
    parser.add_argument('--settlement-rewards', type=output_file, help='可选的 .work 隔离结算奖量与兑换测试物品装配 JSON')
    parser.add_argument('--fixture-out', required=True, type=output_file, help='独立运行规则输出文件')
    parser.add_argument('--public-out', type=output_file, help='可选的客户端公开目录输出文件')
    parser.add_argument('--check', action='store_true', help='只核对已经生成的投影')
    arguments = parser.parse_args()
    targets = [path for path in (arguments.fixture_out, arguments.public_out) if path is not None]
    sources = {arguments.assembly.resolve(), arguments.settlement_rewards} - {None}
    if len(set(targets)) != len(targets) or sources.intersection(targets) or arguments.settlement_rewards == arguments.assembly.resolve():
        raise ValueError('镜像竞技隔离装配、服务端和客户端输出应使用不同文件')
    schema = configuration.load_json(ROOT / 'config/schema/catalog.json')
    tables, errors, _, _ = configuration.collect(schema)
    if errors:
        raise ValueError(errors)
    fixture = configuration.load_json(BASE_FIXTURE)
    workbook = ROOT / next(row['path'] for row in schema['workbooks'] if row['group'] == 'gameplay')
    if not fixture['development_only'] or fixture['runtime_ready'] or 'mirror_arena' in fixture:
        raise ValueError('镜像竞技隔离投影必须基于未启用镜像竞技的开发底稿')
    if fixture['_source_evidence']['workbook']['sha256'] != configuration.sha256(workbook):
        raise ValueError('默认开发夹具与当前唯一玩法工作簿版本不一致')
    assembly = configuration.load_json(arguments.assembly)
    rewards = configuration.load_json(arguments.settlement_rewards) if arguments.settlement_rewards else None
    if arguments.settlement_rewards and rewards is None:
        raise ValueError('显式镜像赛季奖励装配不能写成空值')
    rule, public_map = project(tables, assembly, fixture, rewards)
    fixture['mirror_arena'] = rule
    fixture['_source_evidence']['mirror_arena'] = {
        'tables': [RULES, STREAKS], 'workbook_sha256': configuration.sha256(workbook),
        'compiler_sha256': configuration.sha256(Path(__file__)),
        'assembly_sha256': configuration.sha256(arguments.assembly),
        'scope': '隔离镜像候选，赛季日期、地图、匹配范围和宠物阵亡策略为本轮测试选择。',
        'candidate_rules': ['daily_opponents', 'battle_timeout_sec', 'timeout_resolution'],
    }
    if arguments.settlement_rewards:
        evidence = fixture['_source_evidence']['mirror_arena']
        evidence['tables'].append(REWARDS)
        evidence['season_settlement'] = {
            'confirmed_rules': list(SETTLEMENT_RULES),
            'reward_assembly_sha256': configuration.sha256(arguments.settlement_rewards),
            'scope': '仅隔离验证绑定元宝派奖和结转。兑换物品来自现有目录的测试选择。',
        }
        if 'exchange' in rule:
            evidence['candidate_rules'].append('feather_exchange_points')
            evidence['season_settlement']['confirmed_rules'].append('feather_exchange_bound')
    write_projection(arguments.fixture_out, fixture, arguments.check)
    if arguments.public_out:
        public = configuration.load_json(BASE_PUBLIC)
        if public['rule_version'] != fixture['rule_version'] or public['runtime_ready']:
            raise ValueError('镜像竞技公开目录与服务端开发规则版本不一致')
        public.setdefault('challenge_maps', []).append(public_map)
        public['mirror_arena'] = {'enabled': True, 'template_id': rule['template_id'],
                                  'display_name': rule['display_name'], 'map_id': public_map['id']}
        write_projection(arguments.public_out, public, arguments.check)
    print(json.dumps({'checked': arguments.check, 'development_only': True, 'runtime_ready': False,
                      'rule_version': fixture['rule_version'], 'fixture_sha256': configuration.sha256(arguments.fixture_out)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
