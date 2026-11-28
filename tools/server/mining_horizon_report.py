"""读取矿战原双角色账本，独立复算两种本金下的真实墙钟、资金与机会差额。"""

from datetime import datetime
from hashlib import file_digest
import json
from pathlib import Path


WALLETS = ('gold', 'bound_gold', 'yuanbao_units', 'bound_yuanbao_units')
CARDS = ('none', 'privilege', 'hosting', 'both')
ZERO = dict.fromkeys(WALLETS, 0)


def read(path):
    """从保留的实际运行目录读取 JSON。"""
    return json.loads(path.read_bytes())


def digest(path):
    """不重新生成原程序、夹具及账本，只校验冻结摘要。"""
    with path.open('rb') as source:
        return file_digest(source, 'sha256').hexdigest()


def plus(a, b, sign=1):
    """保持金币、绑定金币和两类元宝分账。"""
    return {key: a[key] + sign * b[key] for key in WALLETS}


def require(condition, message):
    """在失败时给出原证据问题，允许调用者保留原文件。"""
    if not condition:
        raise ValueError(message)


def at(value):
    """读取服务端或原观察者记录的绝对时刻。"""
    return datetime.fromisoformat(value)


def spent_ms(a, b):
    """对拍报告使用的整毫秒值，不从收益量估算耗时。"""
    span = b - a
    return ((span.days * 86_400 + span.seconds) * 1_000_000 + span.microseconds) // 1000


def actor(profile, start, end):
    """从不同类型的原回执复算一个角色的钱包和窗内统计。"""
    ledger = profile['ledger']
    wallet = profile['before']['wallet']
    inside_net, ore_gold, tool_cost = ZERO.copy(), ZERO.copy(), ZERO.copy()
    kills = experience = mining_ms = attempts = ores = 0
    source_ids = set()
    for row in ledger['battle_rows'] or []:
        moment = at(row['at'])
        require(row['id'] not in source_ids and row.get('hosting_used', 0) == 0, '普通回执重复或在线发生扣时')
        source_ids.add(row['id'])
        gold = dict(ZERO, gold=row.get('recycle_gold', 0), bound_gold=row.get('recycle_bound_gold', 0))
        gold['bound_gold' if row.get('bound') else 'gold'] += row.get('direct_gold', 0)
        wallet = plus(wallet, gold)
        if start <= moment < end:
            inside_net = plus(inside_net, gold)
            kills += int(bool(row.get('monster')))
            experience += row.get('experience', 0)
    for row in ledger['attempts'] or []:
        source, fact = row['id'], row['fact']
        begin, happened = at(fact['StartedAt']), at(fact['At'])
        require(source not in source_ids and fact['HostSeconds'] == 0 and start <= begin < end, '采矿时间、扣费或回执身份不符')
        source_ids.add(source)
        mining_ms += spent_ms(begin, min(end, happened))
        gold = fact['AutoRecycle'] or ZERO
        wallet = plus(wallet, gold)
        if happened < end:
            attempts += 1
            ores += int(fact['Ore'] is not None)
            ore_gold = plus(ore_gold, gold)
    for tool in profile['tools'] or []:
        require(tool['id'] not in source_ids, '买锄回执重复')
        source_ids.add(tool['id'])
        tool_cost = plus(tool_cost, tool['spend'])
    wallet = plus(wallet, tool_cost, -1)
    inside_net = plus(plus(inside_net, ore_gold), tool_cost, -1)
    require(wallet == profile['after']['wallet'] and ledger['asset_net'] == plus(wallet, profile['before']['wallet'], -1), '四钱包原值与到账不守恒')
    require((kills, experience) == (ledger['battle']['kills'], ledger['battle']['experience']), '普通击杀或经验汇总不符')
    require((attempts, ores, mining_ms) == (ledger['window_attempts'], sum(ledger['ore'].values()), ledger['mining_ms']), '原墙钟内矿轮和矿量汇总不符')
    require(ore_gold == ledger['auto_gold'] and tool_cost == ledger['tool_spend'] and inside_net == ledger['window_net'], '回收、购锄或窗内钱包不符')
    require(profile['before']['hosting'] == profile['after']['hosting'], '在线实验出现托管扣时')
    return {'attempts': attempts, 'ores': ores, 'mining_ms': mining_ms, 'tools_bought': len(profile['tools'] or []),
            'tools_exhausted': ledger['exhausted'], 'remaining_tools': ledger['remaining_tools'] or [],
            'stop': profile['stop_reason'], 'battle_active_ms': profile['combat']['online_active_ms'],
            'kills': kills, 'experience': experience, 'ore_gold': ore_gold, 'tool_cost': tool_cost, 'net': inside_net,
            'wear_u': ledger['battle']['wear_u'], 'potion_use': ledger['potion_use'], 'retained_ore': ledger['retained'] or []}


def pair(path, capital, layer, cards, duration, fixture_sha):
    """核对两端报告确实来自同一个配对条件和完整计划窗口。"""
    report = read(path)
    settings = report['selection']
    start, end = at(report['window']['start']), at(report['window']['end'])
    require(report['fixture_sha256'] == fixture_sha and report['capital'] == capital and report['layer'] == layer
            and settings['cards'] == cards and settings['seconds'] == duration and spent_ms(start, end) == duration * 1000,
            f'配对时长、矿层和条件不一致 {path.name}')
    mixed, control = report['mixed'], report['control']
    before = mixed['before']
    for name in ('wallet', 'experience', 'level', 'potions', 'skills', 'hosting'):
        require(before[name] == control['before'][name], f'配对初态的 {name} 不同')
    try:
        active, pure = actor(mixed, start, end), actor(control, start, end)
    except ValueError as error:
        raise ValueError(f'{path.name} {error}') from error
    require(pure['attempts'] == pure['tools_bought'] == 0 and pure['kills'] > 0 and active['tools_bought'] > 0, '纯战斗混入开采或矿战没有实际付费')
    require(active['battle_active_ms'] + active['mining_ms'] <= duration * 1000 and pure['battle_active_ms'] <= duration * 1000,
            '同一角色的普通战斗和采矿时间重叠')
    return {'capital': capital, 'layer': layer, 'cards': cards, 'seconds': duration, 'initial_wallet': before['wallet'],
            'data_sha256': digest(path), 'mixed': active, 'ordinary': pure, 'role_ids': [mixed['role_id'], control['role_id']],
            'difference': {'kills': pure['kills'] - active['kills'], 'experience': pure['experience'] - active['experience'],
                           'net': plus(active['net'], pure['net'], -1)}}


def summarize(directory: Path):
    """在受控副本集清退后复算原账本，并保存逐对与分别汇总的报告。"""
    manifest = read(directory / 'manifest.json')
    run = read(directory / 'result.json')
    require(run['passed'] and manifest['seconds'] == run['seconds'], '本批真实联机尚未完成')
    require(manifest['binary_sha256'] == digest(directory / 'mining-horizon.test.exe'), '原采样程序的摘要变化')
    results = []
    summary = {}
    for capital in manifest['capital']:
        group = directory / capital / 'mining'
        fixture_sha = digest(group / 'mining-fixture.json')
        require(run['capital'][capital]['mongo_cleaned'] and fixture_sha == run['capital'][capital]['mining_fixture_sha256'], '原夹具或数据库清退未通过')
        rows = [pair(group / f'layer{layer}-{cards}.json', capital, layer, cards, manifest['seconds'], fixture_sha)
                for layer in (1, 2) for cards in CARDS]
        results.extend(rows)
        summary[capital] = {
            'pairs': len(rows), 'funds_stops': sum(x['mixed']['stop'] == 'funds' for x in rows),
            'tools_bought': sum(x['mixed']['tools_bought'] for x in rows),
            'attempts': sum(x['mixed']['attempts'] for x in rows), 'ores': sum(x['mixed']['ores'] for x in rows),
            'mining_ms': sum(x['mixed']['mining_ms'] for x in rows),
            'mixed_kills': sum(x['mixed']['kills'] for x in rows), 'ordinary_kills': sum(x['ordinary']['kills'] for x in rows),
            'mixed_experience': sum(x['mixed']['experience'] for x in rows), 'ordinary_experience': sum(x['ordinary']['experience'] for x in rows),
        }
        for side in ('mixed', 'ordinary'):
            for currency in ('tool_cost', 'ore_gold', 'net'):
                total = ZERO.copy()
                for row in rows:
                    total = plus(total, row[side][currency])
                summary[capital][side + '_' + currency] = total
    ids = [role for row in results for role in row['role_ids']]
    require(len(set(ids)) == len(ids), '不同配对之间角色重复')
    output = {'passed': True, 'seconds': run['seconds'], 'pairs': len(results), 'roles': len(ids), 'capital': summary, 'samples': results}
    (directory / 'summary.json').write_text(json.dumps(output, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    return output
