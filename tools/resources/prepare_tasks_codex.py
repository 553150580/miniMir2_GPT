"""把唯一玩法表中的已选任务、成就、图鉴与绑定奖励导出为开发运行配置。"""

from collections import defaultdict


TABLES = ('game_quests', 'game_daily_tasks', 'game_achievements', 'game_codex', 'game_reward_groups', 'game_reward_entries')
EVENTS = {'monster_kill', 'item_recycle', 'skill_learn', 'level_reached'}
RUNNABLE = {'confirmed', 'candidate'}
MAX_INT64 = (1 << 63) - 1
MAX_UINT32 = (1 << 32) - 1
DAILY_RULE_FIELDS = {
    'daily_pool_size': 'daily_task_pool_size',
    'daily_claim_limit': 'daily_task_selected_count',
    'daily_activity_target': 'daily_activity_reward_threshold',
}


def project(tables):
    """保持表间稳定编号，奖励只接受明确的绑定确定值。"""
    if not all(table in tables for table in TABLES):
        return None

    def rows(table):
        return [row for row in tables[table] if row.get('record_status') in RUNNABLE]

    definitions = []
    reward_groups = {row['id']: row for row in rows('game_reward_groups')}
    entries = defaultdict(list)
    for entry in rows('game_reward_entries'):
        entries[entry['reward_group_id']].append(entry)

    def reward(group_id):
        group = reward_groups.get(group_id)
        if not group or group['binding_policy'] != 'bound' or not entries[group_id]:
            raise ValueError(f'任务奖励组 {group_id} 缺少开放的绑定奖励')
        result = {}
        for row in entries[group_id]:
            if row.get('binding_override') not in (None, 'bound') or row['weight_or_probability'] != {'certain': True}:
                raise ValueError(f'任务奖励条目 {row["id"]} 必须为确定绑定奖励')
            distribution = row['quantity_distribution']
            quantity = distribution.get('fixed') if isinstance(distribution, dict) else None
            if type(quantity) is not int or not 0 < quantity <= MAX_INT64 or set(distribution) != {'fixed'}:
                raise ValueError(f'奖励条目 {row["id"]} 必须填写正整数数量')
            asset = row['asset_key']
            if asset in ('bound_gold', 'bound_yuanbao_units'):
                result[asset] = result.get(asset, 0) + quantity
                if result[asset] > MAX_INT64:
                    raise ValueError(f'奖励组 {group_id} 的绑定货币总量超出整数范围')
            elif asset.startswith('item:') and asset[5:].isascii() and asset[5:].isdigit() and 0 < int(asset[5:]) <= MAX_UINT32:
                result.setdefault('items', []).append({'item_id': int(asset[5:]), 'quantity': quantity})
            else:
                raise ValueError(f'任务奖励资产 {asset} 不受支持')
        return result

    def add(row, kind, name, target, event='', **gate):
        identity = row['id']
        if (not isinstance(name, str) or not name.strip() or type(target) is not int or not 0 < target <= MAX_INT64
                or type(identity) is not int or not 0 < identity <= MAX_UINT32):
            raise ValueError(f'任务或图鉴 {row["id"]} 缺少名称或有效目标')
        if kind != 5 and event not in EVENTS:
            raise ValueError(f'任务 {row["id"]} 缺少服务端可确认事件')
        for key, value in gate.items():
            maximum = MAX_INT64 if key == 'map_id' else MAX_UINT32
            if value is not None and (type(value) is not int or not 0 <= value <= maximum):
                raise ValueError(f'任务 {identity} 的 {key} 超出整数范围')
        group_id = row['reward_group_id']
        definitions.append({'id': row['id'], 'kind': kind, 'name': name, 'target': target,
                            **({'event': event} if event else {}), **{k: v for k, v in gate.items() if v},
                            'reward': reward(group_id)})

    for row in rows('game_quests'):
        goal, gates = row['objective_events'], row['prerequisites']
        add(row, 1, goal.get('name'), goal.get('target'), goal.get('event'),
            prerequisite_id=gates.get('quest_id'), minimum_level=gates.get('level'),
            monster_id=goal.get('monster_id'), map_id=goal.get('map_id'))
    for row in rows('game_daily_tasks'):
        gates = row['unlock_conditions']
        add(row, 2, gates.get('name'), row['target_count'], row['objective_event'],
            activity=row['activity_points'], minimum_level=gates.get('level'))
    for row in rows('game_achievements'):
        goal = row['objective_events']
        add(row, 3, goal.get('name'), row['required_progress'], goal.get('event'), monster_id=goal.get('monster_id'))
    monsters = {row['id']: row['display_name'] for row in tables['game_monsters']}
    for row in rows('game_codex'):
        if row['qualification_rule'] != 'confirmed_kill' or row.get('budgeted_attributes'):
            raise ValueError(f'图鉴 {row["id"]} 只接受已确认击杀和已配置的绑定奖励')
        add(row, 4, monsters[row['monster_id']], row['required_kills'], 'monster_kill',
            monster_id=row['monster_id'], map_id=row['map_group_id'])
    daily = {}
    if any(row['kind'] == 2 for row in definitions):
        rules = {row['rule_key']: row['rule_value'] for row in rows('game_progression_rules')}
        daily = {field: rules.get(key) for field, key in DAILY_RULE_FIELDS.items()}
        if any(type(value) is not int or not 0 < value <= MAX_UINT32 for value in daily.values()):
            raise ValueError('每日任务池、领取上限及活跃目标缺少唯一玩法表规则')
    for group in reward_groups.values():
        if group['source_kind'] == 'daily_activity':
            gate = group['qualification']
            target = daily.get('daily_activity_target')
            if gate.get('target', target) != target:
                raise ValueError('每日活跃奖励门槛与玩法规则不一致')
            add({'id': 1, 'reward_group_id': group['id']}, 5, gate.get('name'), target)
    return {'definitions': definitions, **daily} if definitions else None
