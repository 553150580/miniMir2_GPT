#!/usr/bin/env python3
"""逐笔复算五个托管等级档的地图样本，并和同一工作簿候选的真实成交对照。"""

import argparse
from collections import defaultdict
from fractions import Fraction
from pathlib import Path
import sys

import ordinary_economy_report as ordinary
import run_integration_batches as matrix
from sample_ordinary_economy import read, safe_directory


def require(condition, message):
    """保证原运行、夹具、角色组合和实际成交能互相核对。"""
    if not condition:
        raise ValueError(message)


def pair(rate):
    """保持每小时观察率的分子、分母，避免提前向上或向下取整。"""
    return [rate.numerator, rate.denominator]


def read_workbook_quotes(batch):
    """对拍已清退隔离库中使用原工作簿五档实际成交的九个角色。"""
    path = batch.resolve(strict=True)
    require(path.parent == (matrix.ROOT / '.work/integration-batches').resolve(), '请选择本项目联机验收的原目录')
    settings = path / 'hosting'
    run = read(settings / 'result.json')
    require(run['passed'] and run['mongo_cleaned'] and 'TestHostingPurchaseWorkbookBandsMongoWS' in run['passing'],
            '实际工作簿隔离报价验收或清退未完成')
    fixture_file = settings / 'hosting-price-fixture.json'
    digest = ordinary.digest(fixture_file)
    fixture = read(fixture_file)
    artifact = read(settings / 'hosting-price-receipts.json')
    prices, = fixture['hosting_time_prices']
    source = fixture['_source_evidence']['hosting_time_prices']
    require(digest == run['hosting_price_fixture_sha256'] == artifact['fixture_sha256'] and prices['version'] == artifact['version'],
            '实际报价的冻结配置或生效版本不符')
    require(source['status'] == 'isolated_candidate' and fixture['runtime_ready'] is False,
            '本批对照须使用原工作簿待审投影')
    required = {level for band in prices['bands'] for level in (band['level_min'], band['level_max']) if level}
    observed = {}
    for row in artifact['receipts']:
        level, receipt = row['level'], row['receipt']
        quote = receipt['Quote']
        band, = (b for b in prices['bands'] if level >= b['level_min'] and (b['level_max'] == 0 or level <= b['level_max']))
        require(level not in observed and level in required, '报价证明存在重复或缺失的档位边界')
        seconds = prices['purchase_step_seconds']
        hourly = 2 * band['g_ref_gold_per_hour']
        cost = (hourly * seconds + 3599) // 3600
        cost_bound = min(row['before']['bound_gold'], cost)
        require((quote['BandMinLevel'], quote['BandMaxLevel'], quote['QuoteVersion'], quote['ConfigHash']) ==
                (band['level_min'], band['level_max'], prices['version'], digest) and quote['RuleVersion'] == fixture['rule_version'],
                '实际原报价档位、版本或配置失配')
        require((quote['RequestedSeconds'], quote['GranularitySeconds'], quote['ReferenceGoldPerHour'], quote['GoldPerHour'],
                 quote['TotalGold'], quote['BoundGoldCost'], quote['GoldCost']) ==
                (seconds, seconds, band['g_ref_gold_per_hour'], hourly, cost, cost_bound, cost - cost_bound), '实际每小时单价或整笔扣费失配')
        require(receipt['BalanceSeconds'] - quote['BeforeSeconds'] == seconds and ordinary.minus(row['before'], row['after']) ==
                ordinary.total_wallet({'gold': cost - cost_bound, 'bound_gold': cost_bound}), '实际钱包、秒数或回执重放失配')
        observed[level] = {'cost': cost, 'bound_gold': cost_bound, 'gold': cost - cost_bound, 'received_seconds': seconds}
    require(observed.keys() == required, '实际报价遗漏等级档的原边界角色')
    return prices, source, digest, observed


def reference_map(rows):
    """一个地图的原时长包含死亡和停战，直金与实际回收分别复算。"""
    exposure = sum(row['seconds'] for row in rows)
    direct = sum(row['direct'] for row in rows)
    recycle = sum(row['recycle'] for row in rows)
    rate = Fraction((direct + recycle) * 3600, exposure)
    jobs = defaultdict(list)
    for row in rows:
        jobs[row['job']].append(row)
    by_job = {}
    for job, entries in sorted(jobs.items()):
        gross = sum(row['direct'] + row['recycle'] for row in entries)
        seconds = sum(row['seconds'] for row in entries)
        by_job[job] = {'samples': len(entries), 'gold_per_hour': pair(Fraction(gross * 3600, seconds))}
    return rate, {
        'samples': len(rows), 'seconds': exposure, 'kills': sum(r['kills'] for r in rows), 'direct_gold': direct, 'recycle_gold': recycle,
        'gold_per_hour': pair(rate), 'confirmed_hosting_seconds': sum(r['hosting'] for r in rows),
        'wear_deaths': sum(r['deaths'] for r in rows), 'crossed_bands': sum(not r['stayed'] for r in rows),
        'jobs': by_job,
    }


def summarize(study, quotes):
    """保留五档各图及各职业的已验证观察率，并登记候选锚点在样本范围内的位置。"""
    folder = safe_directory(study)
    manifest = read(folder / 'manifest.json')
    result = read(folder / 'ordinary-economy/result.json')
    binary_hash, fixture_hash = ordinary.digest(folder / 'ordinary-economy.test.exe'), ordinary.digest(folder / 'fixture.json')
    require(result['passed'] and result['mongo_cleaned'] and binary_hash == manifest['test_binary_sha256']
            and fixture_hash == manifest['fixture_sha256'], '真实角色采样、原程序或独立库清退未完成')
    prices, source, price_fixture, paid = read_workbook_quotes(quotes)
    require(fixture_hash == source['base_fixture_sha256'], '价格隔离实验与普通图样本须从同一规则基线开始')
    selection = manifest['selection']
    require(selection['mode'] == 'hosting' and selection['cards'] == ['none'] and sorted(selection['jobs']) == [0, 1, 2]
            and len(selection['kits']) == len(selection['monsters']) == 1, '锚点对照须固定离线的三职业无卡配装和怪槽')
    directory = folder / 'samples'
    verified = ordinary.summarize(directory, manifest)
    bands, setups = {}, {}
    source_rows = {row['level_min']: row for row in source['source_rows']}
    for band in prices['bands']:
        lower, upper = band['level_min'], band['level_max']
        rows = defaultdict(list)
        start_level = set()
        for sample in verified['samples']:
            level = sample['starting_level']
            if level < lower or upper and level > upper:
                continue
            start_level.add(level)
            raw = read(directory / (sample['sample'] + '.json'))
            require(sample['sample_sha256'] == ordinary.digest(directory / (sample['sample'] + '.json')) and not sample['price']['version'],
                    '角色样本发生变化或混入生效补时价格')
            condition = sample['conditions']
            setup = {'equipment': sample['starting_equipment'], 'skills': sample['starting_skills']}
            require(setups.setdefault(condition['job'], setup) == setup, '同职业对照的实际起始配装或技能变化')
            gross = raw['gross']
            final_level = sample['ending_level']
            rows[condition['map_id']].append({
                'job': condition['job'], 'seconds': condition['seconds'], 'kills': sample['kills'],
                'direct': sum(gross['direct'][key] for key in ordinary.WALLET[:2]),
                'recycle': sum(gross['recycle'][key] for key in ordinary.WALLET[:2]),
                'hosting': sample['hosting_used'], 'deaths': sample['wear_deaths'],
                'stayed': final_level >= lower and (upper == 0 or final_level <= upper),
            })
        samples_per_map = len(selection['jobs']) * selection['repeat']
        require(len(start_level) == 1 and set(rows) == set(selection['map_ids']) and all(len(r) == samples_per_map for r in rows.values()),
                f'{lower} 档缺少同口径的实际地图样本')
        maps, rates = {}, []
        for map_id, observations in sorted(rows.items()):
            rate, metrics = reference_map(observations)
            rates.append(rate)
            maps[map_id] = metrics
        low, high = min(rates), max(rates)
        candidate = band['g_ref_gold_per_hour']
        comparison = 'below' if candidate < low else 'above' if candidate > high else 'within'
        bands[lower] = {'starting_level': min(start_level), 'level_min': lower, 'level_max': upper,
                        'source_ref': source_rows[lower]['source_ref'], 'record_status': source_rows[lower]['record_status'],
                        'workbook_g_ref': candidate, 'workbook_hourly_price': 2 * candidate,
                        'tested_purchase': paid[lower],
                        'observed_map_range_per_hour': [pair(low), pair(high)], 'candidate_relative_to_observations': comparison,
                        'maps': maps}
    output = {'passed': True, 'verified_samples': verified['verified_samples'],
              'confirmed_kills': sum(row['kills'] for row in verified['samples']),
              'confirmed_source_pairs': sum(row['sources'] for row in verified['samples']),
              'verified_quote_levels': sorted(paid), 'window_seconds': selection['seconds'],
              'source_fixture_sha256': fixture_hash, 'workbook_sha256': source['workbook_sha256'],
              'price_fixture_sha256': price_fixture, 'mode': selection['mode'], 'map_ids': selection['map_ids'],
              'test_quote_version': prices['version'], 'monster_slots': selection['monsters'][0], 'setups': setups,
              'bands': bands}
    matrix.save(folder / 'hosting-reference-rates.json', output)
    return output


def main():
    """接收留存的真实普通图账本和隔离报价结果，重新计算五档观察值。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--study', required=True, type=Path)
    parser.add_argument('--pricing-batch', required=True, type=Path)
    args = parser.parse_args()
    try:
        report = summarize(args.study, args.pricing_batch)
    except (OSError, ValueError, KeyError, RuntimeError) as error:
        print(error, file=sys.stderr)
        return 1
    print(f'{report["verified_samples"]} 个普通图样本和 {len(report["verified_quote_levels"])} 个报价边界已复算')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
