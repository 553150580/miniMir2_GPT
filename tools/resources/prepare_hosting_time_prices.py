#!/usr/bin/env python3
"""将唯一工作簿的补时锚点投影到项目内隔离试玩，用于验证真实报价与扣款。"""

import argparse
from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
import sys


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools/config'))
import export_configs as config


BANDS = {1: 19, 20: 29, 30: 39, 40: 49, 50: None}
PRICE_TABLE = 'game_level_band_economy'
MAX_REFERENCE_GOLD = (2**63 - 1) // 48


def compile_bands(rows):
    """按正式分档和原工作簿毛金币形成测试输入，保留每行的待审状态和来源。"""
    bands, source = [], []
    pending = BANDS.copy()
    for row in rows:
        lower, upper = row['level_min'], row['level_max']
        g_ref = row['g_ref_gold_per_hour']
        if type(lower) is not int or lower not in pending or pending[lower] != upper or (upper is not None and type(upper) is not int):
            raise ValueError('托管锚点存在不符或重复的等级档')
        del pending[lower]
        if row['record_status'] not in ('candidate', 'confirmed') or type(g_ref) is not int or not 0 < g_ref <= MAX_REFERENCE_GOLD:
            raise ValueError(f'{lower} 级档缺少有效的工作簿锚点')
        if type(row['hosting_gold_per_hour']) is not int or row['hosting_gold_per_hour'] != 2 * g_ref:
            raise ValueError(f'{lower} 级档的托管单价必须为锚点的两倍')
        bands.append({'level_min': lower, 'level_max': upper or 0, 'g_ref_gold_per_hour': g_ref})
        source.append({key: row[key] for key in ('id', 'level_min', 'level_max', 'record_status', 'source_ref')})
    if pending:
        raise ValueError(f'托管锚点缺少等级档 {sorted(pending)}')
    bands.sort(key=lambda row: row['level_min'])
    source.sort(key=lambda row: row['level_min'])
    return bands, source


def output_in_work(value):
    """候选只写到项目工作目录，用于隔离实例加载。"""
    path = Path(value).resolve()
    if not path.is_relative_to((ROOT / '.work').resolve()) or path.suffix != '.json':
        raise argparse.ArgumentTypeError('隔离投影须在项目 .work 目录内并使用 JSON')
    return path


def versioned_fixture(version, step, effective, lifetime):
    """读取既有开发配置和唯一经济工作簿，生成可冻结的绝对报价窗口。"""
    if not 0 < version <= 2**63 - 1 or not 0 < step <= 86400 or not 0 < lifetime <= 24:
        raise ValueError('隔离价格版本、购买粒度或生效窗口无效')
    if effective.utcoffset() is None:
        raise ValueError('生效时刻必须含绝对时区')
    effective = effective.astimezone(timezone.utc)
    tables, errors, _, paths = config.collect(config.load_json(ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    bands, rows = compile_bands(tables[PRICE_TABLE])
    committed = ROOT / 'config/development/first-loop.json'
    raw = config.load_json(committed)
    if raw.get('development_only') is not True or raw.get('runtime_ready') is not False or raw.get('hosting_time_prices'):
        raise ValueError('隔离锚点投影需要关闭正式价表的原开发配置')
    raw['hosting_time_prices'] = [{'version': version, 'effective_from': effective.isoformat(),
                                  'effective_until': (effective + timedelta(hours=lifetime)).isoformat(),
                                  'purchase_step_seconds': step, 'bands': bands}]
    raw.setdefault('_source_evidence', {})['hosting_time_prices'] = {
        'status': 'isolated_candidate', 'source_table': PRICE_TABLE, 'source_rows': rows,
        'workbook_sha256': config.sha256(paths['gameplay']), 'base_fixture_sha256': config.sha256(committed),
    }
    return raw


def main():
    """要求显式提供隔离版本、报价单位和窗口，支持原配置逐字节复核。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--fixture-out', required=True, type=output_in_work)
    parser.add_argument('--version', required=True, type=int)
    parser.add_argument('--step-seconds', required=True, type=int)
    parser.add_argument('--effective-from', required=True, type=datetime.fromisoformat)
    parser.add_argument('--lifetime-hours', type=int, default=3)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    try:
        fixture = versioned_fixture(args.version, args.step_seconds, args.effective_from, args.lifetime_hours)
        body = (json.dumps(fixture, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if args.fixture_out.read_bytes() != body:
                raise ValueError('隔离版价格夹具与原来源不符')
        else:
            args.fixture_out.parent.mkdir(parents=True, exist_ok=True)
            args.fixture_out.write_bytes(body)
    except (OSError, ValueError, KeyError) as error:
        print(error, file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
