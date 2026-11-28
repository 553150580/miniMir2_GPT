#!/usr/bin/env python3
"""核对魂兽的唯一玩法工作簿，投影显式隔离的逐星、附体与同名首领样本。"""
import argparse
import json
from pathlib import Path

import prepare_body_mastery as mastery
import prepare_forge as forge
import prepare_mining as mining


KEY = 'soul_beasts'
POINTS = {1, 2, 3, 8, 9, 18}
BASIS = {7: 200, 15: 150, 19: 150}
COLLECTION_RULES = {
    'soul_possession_limit': 1,
    'soul_external_render_or_combat': False,
    'soul_duplicate_to_common_fragments': True,
    'soul_pools_selected_each': 1,
    'soul_affix_switch_cost': 0,
    'soul_fragment_antiprofit': 'D(type,s+1)<=D(type,s)+C(type,s); synthesize_1star_cost>=D(type,1)',
}


def project(server, public, tables, assembly):
    """使用前置战斗属性组和原首领结算，角色碎片与物品表无关。"""
    mining.fields(assembly, {'duplicate', 'synthesis', 'stars', 'defaults', 'affixes', 'personal', 'world',
                             'world_minimum_damage'}, '魂兽隔离装配')
    if (server.get('rule_version') != public.get('rule_version') or server.get('rule_version', 0) < 30
            or any(KEY in data or mastery.KEY not in data or data.get('development_only') is not True
                   or data.get('runtime_ready') is not False for data in (server, public))):
        raise ValueError('魂兽依赖同版本隔离战斗配置')
    if tables['game_soul_stars'] or tables['game_attribute_pipeline']:
        raise ValueError('正式逐星或属性表已出现，须先对齐正式配置')
    evidence = []
    for key, value in COLLECTION_RULES.items():
        row = forge.unique(tables['game_collection_rules'], lambda row: row['rule_key'] == key, '魂兽收藏规则')
        if type(row['rule_value']) is not type(value) or row['rule_value'] != value:
            raise ValueError('正式魂兽收藏规则已变化：' + key)
        evidence.append(forge.source(row, 'confirmed', 'game_collection_rules'))
    types = []
    for rank, star in enumerate(assembly['stars'], 1):
        mining.fields(star, {'salvage', 'upgrade'}, '逐星碎片额度')
        salvage = mining.integer(star['salvage'], 1_000_000_000, '分解回收')
        if rank == 1:
            if type(star['upgrade']) is not int or star['upgrade'] != 0 or assembly['synthesis'] < salvage:
                raise ValueError('一星合成须覆盖分解，且一星无升星费')
        else:
            expense = mining.integer(star['upgrade'], 1_000_000_000, '逐星费用')
            if not previous <= salvage <= previous + expense:
                raise ValueError('魂兽升星与分解额度不守恒')
        previous = salvage
    if len(assembly['stars']) != 7:
        raise ValueError('须覆盖七个星级')
    for id, row in enumerate(sorted(tables['game_soul_types'], key=lambda row: row['id']), 1):
        if row['id'] != id or row['max_owned'] != 1 or row['max_star'] != 7 or row['tradeable_record'] is not False:
            raise ValueError('魂兽原名录或角色归属规则已变化')
        evidence.append(forge.source(row, 'confirmed', 'game_soul_types'))
        types.append({'id': id, 'name': row['display_name'], 'stars': assembly['stars'],
                      'duplicate': mining.integer(assembly['duplicate'], 1_000_000_000, '重复折片'),
                      'synthesis': mining.integer(assembly['synthesis'], 1_000_000_000, '未持有合成')})
    if len(types) != 13:
        raise ValueError('须覆盖策划十三种魂兽')
    affixes = []
    for key, values in assembly['affixes'].items():
        id = int(key)
        if id not in POINTS and id not in BASIS:
            raise ValueError('词条尚未接入原战斗阶段')
        row = forge.unique(tables['game_soul_affixes'], lambda value: value['id'] == id, '魂兽词条')
        if row['pool_key'] != ('damage', 'defence', 'effect')[(id-1)//7]:
            raise ValueError('魂兽词条所属池发生变化')
        evidence.append(forge.source(row, 'unverified', 'game_soul_affixes'))
        cap = 2 if id in POINTS else BASIS[id]
        if len(values) != 7 or sorted(values) != values:
            raise ValueError('隔离词条须覆盖递增的七星')
        affixes.append({'id': id, 'name': row['display_name'], 'unit': 'points' if id in POINTS else 'bp',
                        'values': [mining.integer(value, cap, '魂兽词条额度') for value in values]})
    if len(assembly['defaults']) != 3 or any(id not in {row['id'] for row in affixes} or (id-1)//7 != pool
                                            for pool, id in enumerate(assembly['defaults'])):
        raise ValueError('必须各选一个原池选项')
    patch = {}
    for source in ('personal', 'world'):
        if not assembly[source]:
            raise ValueError('隔离样本须有实际个人和世界首领来源')
        main_key, variant_key = source + '_boss', source + '_boss_variants'
        bosses = [server[main_key], *server.get(variant_key, [])]
        for template, drop in assembly[source].items():
            mining.fields(drop, {'type_id', 'chance_ppm'}, '同名首领魂兽来源')
            id = mining.integer(drop['type_id'], len(types), '原魂兽编号')
            mining.integer(drop['chance_ppm'], 1_000_000, '隔离掉落比例')
            row = forge.unique(tables['game_boss_templates'], lambda value: value['id'] == int(template), '首领模板')
            boss = forge.unique(bosses, lambda value: value['template_id'] == int(template), '当前实际首领')
            if row['boss_kind'] != source or row['monster_id'] != boss['monster_id'] or boss['display_name'] != types[id-1]['name']:
                raise ValueError('实际首领身份与工作簿同名收藏不符')
            evidence.append(forge.source(row, 'candidate', 'game_boss_templates'))
            if source == 'world' and not boss.get('rewards'):
                bosses[bosses.index(boss)] = {**boss, 'rewards': {'minimum_damage': mining.integer(
                    assembly['world_minimum_damage'], 100_000, '隔离的最低有效扣血')}}
        patch.update({main_key: bosses[0], variant_key: bosses[1:]})
    rules = {'types': types, 'affixes': affixes, 'defaults': assembly['defaults'],
             'personal': assembly['personal'], 'world': assembly['world']}
    source = {'status': 'isolated_candidate', 'default_enabled': False, 'record_sources': evidence, 'assembly': assembly}
    return ({**server, **patch, KEY: rules, '_source_evidence': {**server['_source_evidence'], KEY: source}},
            {**public, KEY: {key: rules[key] for key in ('types', 'affixes', 'defaults')}})


def main():
    """生成和逐字节核对隔离服务端与公开配置。"""
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ('base-fixture', 'base-public', 'assembly', 'fixture-out', 'public-out'):
        parser.add_argument('--' + name, type=mining.work_path, required=True)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    if len({args.base_fixture, args.base_public, args.assembly, args.fixture_out, args.public_out}) != 5:
        parser.error('魂兽投影输入输出须相互独立')
    cfg = mining.config
    tables, errors, _, books = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError(errors)
    before = cfg.load_json(args.base_fixture)
    workbook = cfg.sha256(books['gameplay'])
    evidence = before.get('_source_evidence', {}).get(mastery.KEY, {})
    if evidence.get('workbook_sha256') != workbook or evidence.get('compiler_sha256') != cfg.sha256(Path(mastery.__file__)):
        raise ValueError('前置候选来源未对齐')
    server, public = project(before, cfg.load_json(args.base_public), tables, cfg.load_json(args.assembly))
    server['_source_evidence'][KEY].update(workbook_sha256=workbook, compiler_sha256=cfg.sha256(Path(__file__)),
                                           assembly_sha256=cfg.sha256(args.assembly), base_fixture_sha256=cfg.sha256(args.base_fixture),
                                           base_public_sha256=cfg.sha256(args.base_public))
    for path, value in ((args.fixture_out, server), (args.public_out, public)):
        expected = (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
        if args.check:
            if path.read_bytes() != expected:
                raise ValueError(f'魂兽隔离输出与输入不一致：{path.name}')
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(expected)
    print(json.dumps({'checked': args.check, 'fixture_sha256': cfg.sha256(args.fixture_out),
                      'public_sha256': cfg.sha256(args.public_out)}))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
