"""从已经配对的角色细账独立复算普通挂机样本，保持金币和托管时长的整数口径。"""

from collections import Counter
from datetime import datetime, timedelta
from fractions import Fraction
from hashlib import file_digest
from itertools import product
import json


WALLET = ('gold', 'bound_gold', 'yuanbao_units', 'bound_yuanbao_units')
POTIONS = ('35', '36')


def digest(path):
    """计算已冻结证据的文件摘要。"""
    with path.open('rb') as source:
        return file_digest(source, 'sha256').hexdigest()


def total_wallet(*wallets):
    """分别累计四种货币，不使用合计余额替代。"""
    return {key: sum(wallet.get(key, 0) for wallet in wallets) for key in WALLET}


def minus(after, before):
    """计算四种货币各自的实测变化。"""
    return {key: after[key] - before[key] for key in WALLET}


def source_counts(row):
    """从一条确认记录复算事件的已入账原量。"""
    return {
        'kills': int(bool(row.get('monster'))), 'raw_experience': row.get('raw_experience', 0),
        'experience': row.get('experience', 0), 'hosting_bonus_gold': row.get('bonus_gold', 0),
        'hosting_bonus_exp': row.get('bonus_exp', 0), 'hosting_used': row.get('hosting_used', 0),
        'wear_u': row.get('wear_u', 0), 'wear_deaths': int(bool(row.get('wear_death'))), 'discarded': row.get('discarded', 0),
    }


def yields(rows):
    """将原掉落和实际保留库存分开累加，未成交材料仅保留数量。"""
    totals = Counter(source_counts({}))
    direct, recycling, dropped, retained = Counter(), Counter(), Counter(), Counter()
    for row in rows:
        totals.update(source_counts(row))
        direct['bound_gold' if row.get('bound') else 'gold'] += row.get('direct_gold', 0)
        recycling['gold'] += row.get('recycle_gold', 0)
        recycling['bound_gold'] += row.get('recycle_bound_gold', 0)
        if 'drop_key' in row:
            dropped[row['drop_key']] += row['drop_quantity']
            if row.get('retained'):
                retained[row['drop_key']] += row['drop_quantity']
    return {**totals, 'direct': total_wallet(direct), 'recycle': total_wallet(recycling),
            'dropped': dict(dropped) or None, 'retained': dict(retained) or None}


def reconcile_sample(report, source):
    """按原事件时刻划窗，再对拍顺序水位、四钱包、托管余额及药水库存。"""
    start, end = (datetime.fromisoformat(report[key]) for key in ('start', 'end'))
    if end - start != timedelta(seconds=report['scenario']['seconds']):
        raise ValueError('实际采样墙钟范围与原场次不一致')
    before, after = report['start_state'], report['end_state']
    source_seq, command_seq = before['source'], before['command']
    inside, outside, used, seen = [], [], Counter({key: 0 for key in POTIONS}), set()
    with source.open(encoding='utf-8') as lines:
        for line in lines:
            row = json.loads(line)
            if row['id'] in seen or bool(row.get('source')) == bool(row.get('command')):
                raise ValueError('原账本有重复凭据或来源类型不明')
            seen.add(row['id'])
            if row.get('source'):
                source_seq += 1
                if row['source'] != source_seq:
                    raise ValueError('服务端来源水位不连续')
            else:
                command_seq += 1
                if row['command'] != command_seq:
                    raise ValueError('经济命令水位不连续')
            at = datetime.fromisoformat(row['at'])
            (inside if start <= at < end else outside).append(row)
            if 'potion' in row:
                if str(row['potion']) not in POTIONS or row.get('code'):
                    raise ValueError('原扣药记录超出所选药品或未确认成功')
                used[str(row['potion'])] += 1
    if (source_seq, command_seq, source_seq - before['source'], command_seq - before['command']) != (
            after['source'], after['command'], report['source_pairs'], report['command_pairs']):
        raise ValueError('结束时原双水位与细账数不符')
    parts = (yields(inside), yields(outside))
    if parts != (report['gross'], report['boundary']) or dict(used) != report['potion_use']:
        raise ValueError('从细账复算的收益或药耗与角色报告不符')
    total = total_wallet(*(part[key] for part in parts for key in ('direct', 'recycle')))
    if total != minus(after['wallet'], before['wallet']) or total != report['net_window_and_boundary']:
        raise ValueError('绑定金币、非绑定金币或元宝的实际增减失配')
    host = sum(part['hosting_used'] for part in parts)
    if host != before['hosting'] - after['hosting'] or report['scenario']['mode'] == 'online' and host:
        raise ValueError('原窗口存在未解释的托管秒数或在线扣时')
    for item in POTIONS:
        retained_qty = sum(count for part in parts for key, count in (part['retained'] or {}).items()
                           if key.split(':', 1)[0] == item)
        if before['potions'][item] + retained_qty - used[item] != after['potions'][item]:
            raise ValueError('药水库存与掉落、实际消耗不守恒')
    reconcile_maintenance(report)


def reconcile_maintenance(report):
    """窗口外的真实补药与修理以原成交费用核算，保留失败前态。"""
    care = report.get('post_window_maintenance')
    if not care:
        return
    actions = care['transactions'] or []
    rows = {row['id']: row for row in actions}
    if len(rows) != len(actions) or any(row['action'] not in (1, 3) for row in actions):
        raise ValueError('维护成交出现重复编号或非维护操作')
    potion = total_wallet(*(row['spend'] for row in actions if row['action'] == 1))
    repair = total_wallet(*(row['spend'] for row in actions if row['action'] == 3))
    start, end, after = report['start_state'], report['end_state'], care['after']
    if (potion, repair) != (care['potion_cost'], care['repair_cost']) or minus(end['wallet'], after['wallet']) != total_wallet(potion, repair):
        raise ValueError('原 NPC 实付费用与维修后余额不一致')
    travel = care.get('travel_sources') or []
    if (minus(after['wallet'], start['wallet']) != care['net_with_maintenance']
            or end['hosting'] != after['hosting'] or after['source'] - end['source'] != len(set(travel))):
        raise ValueError('补给后的净金币或托管余额与资产不一致')


def summarize(directory, manifest):
    """校验所有选定角色都已完成，保留每个样本的精确毛金币小时率。"""
    selection = manifest['selection']
    expanded = 'levels' in selection and 'map_ids' in selection
    levels = selection['levels'] if expanded else [selection['level']]
    maps = selection['map_ids'] if expanded else [selection['map_id']]
    cases = product(levels, maps, selection['jobs'], selection['monsters'], selection['kits'], selection['cards'],
                    range(1, selection['repeat'] + 1))
    expected = {}
    for level, map_id, job, slots, kit, cards, count in cases:
        key = f'j{job}-m{slots}-{kit}-{cards}-r{count}'
        if expanded:
            key = f'l{level}-map{map_id}-{key}'
        expected[key] = (level, map_id, job, slots, kit, cards, count)
    found = {p.stem: p for p in directory.glob('*.json')}
    if found.keys() != expected.keys():
        raise ValueError(f'应完成 {len(expected)} 个组合，实际有 {len(found)} 个')
    results = []
    for key, path in sorted(found.items()):
        row = json.loads(path.read_bytes())
        case = row['scenario']
        if tuple(case[field] for field in ('level', 'map_id', 'job', 'slots', 'kit', 'cards', 'repeat')) != expected[key]:
            raise ValueError(f'样本身份误配 {key}')
        if (case['mode'], case['seconds'], row['config_hash'], row['start_state']['level']) != (
                selection['mode'], selection['seconds'], manifest['fixture_sha256'], expected[key][0]):
            raise ValueError(f'样本规则、地图或时长变化 {key}')
        if row['source_log'] != key + '.jsonl':
            raise ValueError(f'样本引用了另一份细账 {key}')
        audit = directory / row['source_log']
        reconcile_sample(row, audit)
        gross = sum(row['gross'][kind][wallet] for kind in ('direct', 'recycle') for wallet in WALLET[:2])
        rate = Fraction(gross * 3600, case['seconds'])
        care = row.get('post_window_maintenance')
        results.append({'sample': key, 'conditions': case, 'starting_level': row['start_state']['level'],
                        'ending_level': row['end_state']['level'],
                        'starting_equipment': [item['item']['item_id'] for item in row['start_state']['equipment']],
                        'starting_skills': row['start_state']['skills'],
                        'kills': row['gross']['kills'], 'experience': row['gross']['experience'],
                        'observed_online_deaths': row['observed_online_deaths'], 'wear_deaths': row['gross']['wear_deaths'],
                        'gross_gold': gross, 'gross_gold_per_hour': [rate.numerator, rate.denominator],
                        'gold_retained_after_care': None if not care or care['status'] != 'completed'
                        else sum(care['net_with_maintenance'][bucket] for bucket in WALLET[:2]),
                        'sources': row['source_pairs'], 'commands': row['command_pairs'],
                        'hosting_used': row['gross']['hosting_used'], 'price': row['hosting_price'],
                        'sample_sha256': digest(path), 'source_sha256': digest(audit)})
    return {'verified_samples': len(results), 'window_seconds': selection['seconds'],
            'real_6_12_24_hour_window': selection['seconds'] in (21600, 43200, 86400),
            'weighted_level_band_price_computed': False, 'samples': results}
