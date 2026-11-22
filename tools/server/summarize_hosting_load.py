"""从隔离挂机压测的日志重建各档稳态统计，不连接游戏和数据库。"""

import argparse
import datetime as dt
import hashlib
import json
from pathlib import Path


MIB = 1024 * 1024


def read_rows(path):
    with path.open(encoding='utf-8') as source:
        return [json.loads(line) for line in source if line.strip()]


def stamp(value):
    return dt.datetime.fromisoformat(value)


def sha256(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def process_stats(samples, pid, name, logical_processors):
    readings = [(timestamp, row['processes'].get(name, {})) for timestamp, row in samples]
    readings = [(timestamp, row) for timestamp, row in readings if row.get('pid') == pid]
    cpu = [(timestamp, row['cpu_seconds']) for timestamp, row in readings if 'cpu_seconds' in row]
    seconds = cores = peak_cores = None
    if len(cpu) >= 2:
        seconds = (cpu[-1][0] - cpu[0][0]).total_seconds()
        if seconds > 0:
            cores = (cpu[-1][1] - cpu[0][1]) / seconds
            peak_cores = max((right[1] - left[1]) / (right[0] - left[0]).total_seconds()
                             for left, right in zip(cpu, cpu[1:]) if right[0] > left[0])
    working_set = max((r['working_set_bytes'] / MIB for _, r in readings if 'working_set_bytes' in r),
                      default=None)
    private = max((r['private_bytes'] / MIB for _, r in readings if 'private_bytes' in r), default=None)
    return {
        'pid': pid, 'samples': len(readings), 'cpu_intervals': max(0, len(cpu) - 1),
        'cpu_covered_seconds': seconds, 'cpu_mean_cores': cores, 'cpu_peak_cores': peak_cores,
        'cpu_mean_host_percent': cores * 100 / logical_processors if cores is not None else None,
        'cpu_peak_host_percent': peak_cores * 100 / logical_processors if peak_cores is not None else None,
        'working_set_peak_mib': working_set, 'private_peak_mib': private,
    }


def gc_pause_p99(before, after):
    bounds = after['gc_pause_bounds_sec']
    old, new = before['gc_pause_counts'], after['gc_pause_counts']
    if before['gc_pause_bounds_sec'] != bounds or len(old) != len(new):
        raise ValueError('GC 暂停直方图的区间不匹配')
    # Go 导出时去掉正负无穷，但仍保留最外侧两个桶的计数。
    if len(bounds) == len(new) - 1:
        bounds = [None, *bounds, None]
    if len(bounds) != len(new) + 1:
        raise ValueError('GC 暂停直方图的区间数量无效')
    counts = [b - a for a, b in zip(old, new)]
    if any(count < 0 for count in counts):
        raise ValueError('GC 暂停累加计数倒退')
    total = sum(counts)
    if total == 0:
        return {'events': 0, 'p99_bucket_ms': None}
    remaining = (total * 99 + 99) // 100
    for i, count in enumerate(counts):
        remaining -= count
        if remaining <= 0:
            bucket_ms = [bound * 1000 if bound is not None else None for bound in bounds[i:i+2]]
            return {'events': total, 'p99_bucket_ms': bucket_ms}
    raise ValueError('GC 暂停直方图缺少有效区间')


def gc_stats(before, after, seconds):
    result = gc_pause_p99(before, after)
    cpu = after['gc_cpu_seconds'] - before['gc_cpu_seconds']
    result.update({
        'collections': after['gc_count'] - before['gc_count'],
        'gc_cpu_seconds': cpu,
        'gc_mean_cores': cpu / seconds,
        'gc_pause_total_ms': (after['gc_pause_total_ns'] - before['gc_pause_total_ns']) / 1e6,
        'allocated_mib_per_second': (after['total_alloc_bytes'] - before['total_alloc_bytes']) / MIB / seconds,
        'heap_live_mib': [point['heap_live_bytes'] / MIB for point in (before, after)],
        'heap_alloc_mib': [point['heap_alloc_bytes'] / MIB for point in (before, after)],
        'heap_sys_mib': [point['heap_sys_bytes'] / MIB for point in (before, after)],
        'goroutines': [point['goroutines'] for point in (before, after)],
        'gomaxprocs': after['gomaxprocs'],
    })
    return result


def counter_changes(before, after):
    if before is None or after is None:
        return None
    return {name: after.get(name, 0) - before.get(name, 0)
            for name in sorted(before.keys() | after.keys())
            if after.get(name, 0) != before.get(name, 0)}


def mongo_phase(server, name):
    return next((row for row in server['mongo'] if row['phase'] == name), None)


def queue_stats(before, after):
    return {
        'db_queue_full': after['db_queue_full'] - before.get('db_queue_full', 0),
        'db_queue_full_by_lane': counter_changes(before.get('db_queue_full_by_lane', {}),
                                                after.get('db_queue_full_by_lane')),
        'db_queue_depth_peak_at_tick': after['max_db_queue_depth'],
        'db_queue_depth_peak_by_lane_at_tick': after.get('max_db_queue_depth_by_lane'),
        'db_queue_capacity_by_lane': after.get('db_queue_capacity_by_lane'),
        'worker_tick_execution': after['tick_work'], 'worker_tick_lag': after['tick_lag'],
        'kill_to_receipt': after.get('kill_to_receipt'),
    }


def phase_stats(start, finish, censuses, samples, pids, logical_processors):
    target = finish['target']
    before, after = start['server'], finish['server']
    began, ended = stamp(before['at']), stamp(after['at'])
    seconds = (ended - began).total_seconds()
    if seconds <= 0:
        raise ValueError(f'{target} 人档的观测结束时间早于开始时间')
    window = [(timestamp, row) for timestamp, row in samples if began <= timestamp <= ended]
    mongo = mongo_phase(after, f'hold-{target}')
    if mongo is None:
        raise ValueError(f'缺少 {target} 人档的 MongoDB 事务观测')
    mongo_began, mongo_ended = stamp(mongo['started_at']), stamp(mongo['ended_at'])
    if max(mongo_began, began) >= min(mongo_ended, ended):
        raise ValueError(f'{target} 人档的 MongoDB 与 Go 诊断观测窗口未重叠')

    begin_census, census = start['census'], finish['census']
    scenes = [point['distinct_scenes'] for point in [begin_census, *censuses, census]]
    rooms_match = (
        sum(before['last_tick_rooms']) == begin_census['distinct_scenes']
        and sum(after['last_tick_rooms']) == census['distinct_scenes']
    )
    changed_positions = census['changed_position'] - begin_census['changed_position']
    return {
        'target': target,
        'start': before['at'], 'end': after['at'],
        'seconds': seconds, 'client_elapsed_seconds': finish['elapsed_seconds'],
        'minimum_online': finish['minimum_online'], 'minimum_auto_alive': finish['minimum_auto_alive'],
        'distinct_scenes_start': begin_census['distinct_scenes'], 'distinct_scenes_end': census['distinct_scenes'],
        'minimum_distinct_scenes': min(scenes), 'census_readings': len(scenes),
        'worker_rooms_start': before['last_tick_rooms'], 'worker_rooms_end': after['last_tick_rooms'],
        'room_census_matches_target': rooms_match and min(scenes) == max(scenes) == target,
        'room_census_matches_server': rooms_match,
        'roles_observed_moving': changed_positions, 'roles_observed_moving_since_ramp': census['changed_position'],
        'roles_attacking': finish['attacking_roles'], 'roles_settled': finish['settling_roles'],
        'attacks': finish['new_attacks'], 'settlements': finish['new_settlements'],
        'settlements_per_second': finish['new_settlements'] / seconds,
        'deaths': finish['new_deaths'], 'disconnects': finish['new_disconnects'],
        'client_errors': census['errors'] - begin_census['errors'],
        'client_error_codes': counter_changes(begin_census.get('error_codes', {}), census.get('error_codes', {})),
        'heartbeats': census['heartbeats'] - begin_census['heartbeats'],
        'server_close_reasons': counter_changes(before.get('server_close_reasons'), after.get('server_close_reasons')),
        'processes': {name: process_stats(window, pid, name, logical_processors) for name, pid in pids.items()},
        'gc': gc_stats(before, after, seconds),
        'mongodb': mongo,
        'mongo_window_start_offset_ms': (mongo_began - began).total_seconds() * 1000,
        'mongo_window_end_offset_ms': (mongo_ended - ended).total_seconds() * 1000,
        'queues_and_ticks': queue_stats(before, after),
    }


def admission_stats(row, activation, hold_start, previous):
    target = row['target']
    current = row['current']
    result = {
        **row,
        'missing_online_at_admission': max(0, target - current['online']),
        'missing_entered_at_admission': max(0, target - current['entered']),
        'auto_activation': activation,
    }
    attempts = row['attempt_errors']
    if roles := attempts.get('roles_attempted'):
        result['first_attempt_success_rate'] = attempts['first_success'] / roles
        result['final_success_rate'] = attempts['success'] / roles
    if hold_start is not None:
        server = hold_start['server']
        prior_server = previous['server'] if previous is not None else {}
        close_reasons = prior_server.get('server_close_reasons') if previous is not None else {}
        if previous is None:
            start_label = 'startup'
        else:
            start_label = f"after-{previous['target']}"
        result['until_hold_start'] = {
            'counter_interval_starts_at': start_label,
            'server_close_reasons': counter_changes(close_reasons, server.get('server_close_reasons')),
            **queue_stats(prior_server, server),
            'mongodb': mongo_phase(server, f'ramp-{target}'),
        }
    return result


def fixture_stats(runner):
    fixture = runner['fixture']
    map_path = Path(fixture['path'])
    current_hash = sha256(map_path) if map_path.is_file() else None
    result = {
        'rule_version': fixture['rule_version'], 'sha256': fixture['fixture_hash'],
        'current_file_matches': current_hash == fixture['fixture_hash'], 'personal_map': None,
    }
    if result['current_file_matches']:
        personal = json.loads(map_path.read_text(encoding='utf-8'))['map']
        result['personal_map'] = {name: personal[name] for name in ('id', 'scene_kind', 'width', 'height')}
    return result


def summarize(directory, profile=None):
    runner_path = directory / 'runner.json'
    samples_path = directory / 'process-samples.jsonl'
    runner = json.loads(runner_path.read_text(encoding='utf-8'))
    if profile is None:
        profile = 'prejoin' if (directory / 'clients-prejoin.jsonl').is_file() else 'raised'
    profile_run = runner['profiles'].get(profile, {})
    sources = {runner_path.name: sha256(runner_path)}
    samples = []
    if samples_path.is_file():
        sources[samples_path.name] = sha256(samples_path)
        samples = [(stamp(row['sampled_at']), row) for row in read_rows(samples_path) if row['profile'] == profile]
        samples.sort(key=lambda sample: sample[0])
    result = {
        'run_id': runner['run_id'], 'binary_sha256': runner['binary_sha256'],
        'started_at': runner['started_at'], 'ended_at': runner['ended_at'],
        'profile': profile,
        'logical_processors': runner['logical_processors'], 'requested_roles': runner['roles'],
        'requested_final_hold_seconds': runner['hold_seconds'],
        'error': runner.get('error'), 'cleanup_errors': runner['cleanup_errors'], 'source_sha256': sources,
        'fixture': fixture_stats(runner), 'seed_role': runner.get('seed'),
        'default_admission': None, 'admission': [], 'hold_stages': [],
        'node_health': profile_run.get('health'),
        'mongo_isolation': runner.get('mongo'), 'mongo_cleanup': runner.get('mongo_cleanup'),
        'node_lease_logs': {},
    }
    default_path = directory / 'clients-default.jsonl'
    if default_path.is_file():
        sources[default_path.name] = sha256(default_path)
        result['default_admission'] = next((row for row in read_rows(default_path)
                                            if row['event'] == 'default_admission'), None)

    client_path = directory / f'clients-{profile}.jsonl'
    starts = {}
    if client_path.is_file():
        sources[client_path.name] = sha256(client_path)
        # 失败时 runner 可能未登记分档进程；采样中的 PID 仍随同原始记录保存。
        pids = profile_run.get('pids')
        if not pids and samples:
            pids = {name: row['pid'] for name, row in samples[0][1]['processes'].items()}
        admissions, activations, previous_finish = {}, {}, None
        for row in read_rows(client_path):
            event, target = row['event'], row.get('target')
            if event == 'admission':
                admissions[target] = row
            elif event == 'auto_activation':
                activations[target] = row
            elif event == 'hold_start':
                starts[target] = (row, [])
                result['admission'].append(
                    admission_stats(admissions.pop(target), activations.pop(target, None), row, previous_finish)
                )
            elif event == 'hold_sample':
                starts[target][1].append(row['census'])
            elif event == 'hold_end':
                start, censuses = starts.pop(target)
                result['hold_stages'].append(phase_stats(start, row, censuses, samples, pids or {},
                                                         runner['logical_processors']))
                previous_finish = row
        for target, row in admissions.items():
            result['admission'].append(admission_stats(row, activations.get(target), None, previous_finish))
    result['incomplete_hold_targets'] = sorted(starts)

    for observed_profile in ('default', profile):
        for mode in ('game', 'account'):
            path = directory / f'{mode}-{observed_profile}.stderr.log'
            if path.is_file():
                sources[path.name] = sha256(path)
                content = path.read_text(encoding='utf-8')
                result['node_lease_logs'][f'{observed_profile}-{mode}'] = {
                    'transient': content.count('节点续租遇到暂态错误'),
                    'recovered': content.count('节点续租已恢复'),
                    'expired_or_failed': content.count('节点租约失败'),
                }
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path, help='包含 runner.json 和压测原始日志的目录')
    parser.add_argument('--profile', choices=('raised', 'prejoin'), help='默认按本轮机器人日志自动识别')
    parser.add_argument('--output', type=Path, help='UTF-8 JSON 输出路径，默认写入标准输出')
    args = parser.parse_args()
    report = summarize(args.directory.resolve(strict=True), args.profile)
    output = json.dumps(report, ensure_ascii=False, indent=2, allow_nan=False)
    if args.output is None:
        print(output)
    else:
        with args.output.open('w', encoding='utf-8', newline='\n') as destination:
            destination.write(output + '\n')


if __name__ == '__main__':
    main()
