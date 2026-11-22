"""统计混合挂机长测的分窗变化和进程资源趋势。"""

import datetime as dt
from statistics import fmean, median


MIB = 1024 * 1024


def trend(readings, duration):
    # 以首尾各最多五分钟的中位数与全窗口线性斜率同时反映变化。
    if not readings:
        return None
    offsets, values = zip(*readings)
    edge = min(300, duration / 4)
    first = [value for offset, value in readings if offset <= edge]
    last = [value for offset, value in readings if offset >= duration - edge]
    early = median(first) if first else None
    late = median(last) if last else None
    mean_offset, mean_value = fmean(offsets), fmean(values)
    spread = sum((offset - mean_offset) ** 2 for offset in offsets)
    slope = (sum((offset - mean_offset) * (value - mean_value)
                 for offset, value in readings) * 3600 / spread) if spread else None
    return {
        'samples': len(readings),
        'first': values[0], 'last': values[-1], 'min': min(values), 'max': max(values),
        'edge_window_seconds': edge, 'early_median': early, 'late_median': late,
        'late_minus_early': late - early if early is not None and late is not None else None,
        'linear_change_per_hour': slope,
    }


def hold_trends(began, ended, samples, pids, diagnostics):
    seconds = (ended - began).total_seconds()
    result = {}
    for name, pid in pids.items():
        series = [(timestamp, row['processes'].get(name, {})) for timestamp, row in samples
                  if began <= timestamp <= ended]
        result[name] = {
            metric: trend(
                [((timestamp - began).total_seconds(), reading[field] / MIB)
                 for timestamp, reading in series if reading.get('pid') == pid and field in reading],
                seconds,
            ) for metric, field in (('working_set_mib', 'working_set_bytes'),
                                    ('private_mib', 'private_bytes'))
        }
    game = result.setdefault('game', {})
    parsed = [(dt.datetime.fromisoformat(point['at']), point) for point in diagnostics]
    game['heap_live_mib'] = trend(
        [((timestamp - began).total_seconds(), point['heap_live_bytes'] / MIB)
         for timestamp, point in parsed if began <= timestamp <= ended], seconds,
    )
    return result


def window_counts(before, after, seconds):
    counts = {field: after[field] - before[field]
              for field in ('settlements', 'attacks', 'deaths', 'disconnects', 'errors', 'heartbeats')}
    counts['settlements_per_second'] = counts['settlements'] / seconds
    return counts


def operation_totals(rows, category):
    attempts = [row[category] for row in rows if row.get(category) is not None]
    if not attempts:
        return None
    keys = ('roles_attempted', 'first_success', 'success', 'failures', 'recovered',
            'ticket_failures', 'ticket_timeouts', 'ticket_write_concern_errors')
    totals = {key: sum(row.get(key, 0) for row in attempts) for key in keys}
    count = totals['roles_attempted']
    if count:
        totals['first_success_rate'] = totals['first_success'] / count
        totals['final_success_rate'] = totals['success'] / count
    return totals


def reconnect_totals(rows, category):
    observations = [row[category] for row in rows if row.get(category) is not None]
    if not observations:
        return None
    return {key: sum(row.get(key, 0) for row in observations) for key in
            ('previously_online', 'same_scene', 'same_life', 'same_position',
             'new_binding', 'restored', 'hosting_debited')}
