#!/usr/bin/env python3
"""读取唯一玩法工作簿，生成激励视频的隔离规则和公开入口开关。"""

import argparse
import json
from pathlib import Path
import re

import prepare_battle_pass as battle_pass
import prepare_forge as forge
import prepare_mining as mining


KEY = 'rewarded_ad'
COMPLETION_TABLE = 'game_commercial_candidates'
ENTITLEMENT_TABLE = 'game_entitlement_rules'
IDENTITY_FIELDS = ('provider', 'placement')
LIMITS = {'account_daily': 1000, 'device_daily': 1000, 'ip_daily': 1000,
          'watch_seconds': 900, 'callback_grace_seconds': 86400}


def project(server, public, tables, assembly):
    """读取已登记的原奖励和额度维度，渠道与测试限量只来自隔离装配。"""
    mining.fields(assembly, {*IDENTITY_FIELDS, *LIMITS}, '激励视频隔离装配')
    for field in IDENTITY_FIELDS:
        if not isinstance(assembly[field], str) or not re.fullmatch(r'[a-z0-9_-]{1,64}', assembly[field]):
            raise ValueError(f'无效的隔离渠道标识 {field}')
    for field, limit in LIMITS.items():
        mining.integer(assembly[field], limit, field)
    if (server.get('rule_version') != public.get('rule_version') or server.get('rule_version', 0) < 30
            or any(KEY in doc or battle_pass.KEY not in doc or doc.get('development_only') is not True
                   or doc.get('runtime_ready') is not False for doc in (server, public))):
        raise ValueError('激励视频须使用同版本战令隔离前置')
    evidence = []

    def rule(table, key, status):
        """同时绑定原规则值、状态和策划出处。"""
        row = forge.unique(tables[table], lambda item: item['rule_key'] == key, key)
        evidence.append(forge.source(row, status, table))
        return row['rule_value']

    seconds = mining.integer(rule(ENTITLEMENT_TABLE, 'video_reward_seconds', 'candidate'), 86400, '单次广告奖励')
    unique_proof = rule(COMPLETION_TABLE, 'ad_reward_requires_verified_unique_event', 'confirmed')
    limits = rule(COMPLETION_TABLE, 'ad_limits_scope', 'confirmed')
    if unique_proof is not True or limits != ['account', 'device', 'ip']:
        raise ValueError('激励视频须核验唯一完播，并使用账号、可信设备和 IP 联合额度')
    return ({**server, KEY: {**assembly, 'reward_seconds': seconds},
             '_source_evidence': {**server['_source_evidence'], KEY: {
                 'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence}}},
            {**public, KEY: True})


def main():
    """校验前置来源并重建两侧文件，仅写入项目隔离工作目录。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, type=mining.work_path, required=True)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('各输入和输出须使用独立项目工作路径')
    cfg = mining.config
    tables, errors, _, books = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    base = cfg.load_json(args.base_fixture)
    workbook = cfg.sha256(books['gameplay'])
    source = base.get('_source_evidence', {}).get(battle_pass.KEY, {})
    if source.get('workbook_sha256') != workbook or source.get('compiler_sha256') != cfg.sha256(Path(battle_pass.__file__)):
        raise ValueError('激励视频前置的工作簿或隔离投影已有变化')
    server, public = project(base, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           assembly_sha256=cfg.sha256(args.assembly), base_fixture_sha256=cfg.sha256(args.base_fixture),
                                           base_public_sha256=cfg.sha256(args.base_public))
    for path, value in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'激励视频隔离结果与输入不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'fixture_sha256': cfg.sha256(args.fixture_out), 'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
