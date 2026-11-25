"""只生成本机切图纹理页。原帧按需要它的地图和职业分组，避免提前加载无关画面。"""

import argparse
from collections import defaultdict
import json
from pathlib import Path

from build_world_boss_atlas import ROOT, build_atlas, effect_keys, load_json


ATLAS = 'map-travel-atlas'


def merge_original_catalogs(source, manifest):
    """补充已校验来源独立导入的 NPC、首领、普通怪及神兽帧。"""
    profiles, frames = dict(manifest['profiles']), dict(manifest['frames'])
    extras = {}
    paths = {'monsters': 'original-monsters/actors.json', 'npcs': 'town-npcs/manifest.json',
             'shinsu': 'pets/shinsu/profile.json'}
    for kind, relative in paths.items():
        path = source / relative
        if not path.is_file() or kind == 'shinsu' and 'pet_shinsu' in profiles:
            continue
        extra = load_json(path)
        actors = {'pet_shinsu': extra['profile']} if kind == 'shinsu' else extra['profiles']
        if extra['schema_version'] != 1 or profiles.keys() & actors.keys() or frames.keys() & extra['frames'].keys():
            raise ValueError(f'切图图集的原始外观清单冲突：{relative}')
        profiles.update(actors)
        frames.update(extra['frames'])
        extras[kind] = extra
    return profiles, frames, extras


def map_frames(selected, job, gameplay, manifest, profiles, extras, floor):
    """对应 WorldAssets 的完整动作、状态、技能、原物件和 NPC 可见范围。"""
    kind = selected['scene_kind']
    visible = {row['profile'] for row in gameplay['jobs'] if kind == 2 or row['job_id'] == job}
    monster_ids = selected.get('monster_definition_ids')
    for row in gameplay['monster_profiles']:
        profile = row['profile']
        if monster_ids is None and profile.startswith('monster_m1_') or monster_ids is not None and row['definition_id'] not in monster_ids:
            continue
        if kind >= 3 and monster_ids is not None:
            profile = extras['monsters']['challenge_profiles'].get(str(row['definition_id']), profile)
        visible.add(profile)
    visible.update(row['profile'] for row in gameplay['pet_profiles'] if row['job_id'] == job
                   and (not manifest.get('development_only') or not row.get('appearance_optional') or row['profile'] in profiles))
    keys = set(effect_keys(manifest.get('statuses'))) | set(effect_keys(manifest.get('teleport')))
    for profile in visible:
        for layer in profiles[profile]['layers']:
            for action in layer['actions'].values():
                for direction in action['directions']:
                    keys.update(direction['frames'])
    for skill in gameplay['skills']:
        if (kind == 2 or skill['job_id'] == job) and (skill['active'] or skill.get('activation_kind') == 'passive_proc'):
            keys.update(effect_keys(manifest['effects'].get(str(skill['id']))))
    # 共用地板的战斗图没有自身的装饰。城镇装饰用目录配置，历史图使用原地板数据。
    compact_town = selected.get('compact_town')
    layout = selected if compact_town else {} if kind >= 3 or selected.get('floor_map_id') is not None else floor
    keys.update(prop['frame'] for prop in layout.get('props', []))
    for npc in layout.get('npcs', []):
        original = extras['npcs']['npc_profiles'][npc['id']] if compact_town and 'npcs' in extras else None
        for layer in profiles[original or npc['profile']]['layers']:
            if layer['package'] == 'Weapon':
                continue
            direction = layer['actions']['standing']['directions'][0 if original else 4]['frames']
            keys.update(direction if original else direction[:1])
    return keys


def build(source):
    """页面只包含被同一组目的地与职业需要的帧，可以共用且没有其他图的纹理占用。"""
    manifest, gameplay = (load_json(source / name) for name in ('manifest.json', 'gameplay.json'))
    catalog = gameplay.get('map_catalog')
    if not catalog:
        return None  # 历史开发包不使用地图目录图集。
    profiles, frames, extras = merge_original_catalogs(source, manifest)
    maps = [*catalog, *gameplay.get('challenge_maps', [])]
    usage = defaultdict(list)
    required = {}
    for selected in maps:
        if selected['scene_kind'] == 4 or selected['scene_kind'] == 2 and not selected.get('compact_town'):
            continue  # 世界首领与来源地图使用各自原资源加载器。
        floor = load_json(source.parent / f"{selected['resource_key']}.json")
        for job in gameplay['jobs']:
            group = (selected['id'], job['job_id'])
            keys = map_frames(selected, job['job_id'], gameplay, manifest, profiles, extras, floor)
            required[group] = {key for key in keys if not frames[key].get('declared_empty')}
            for key in required[group]:
                usage[key].append(group)
    by_usage = defaultdict(dict)
    for key, destinations in sorted(usage.items()):
        by_usage[tuple(sorted(destinations))][key] = frames[key]
    index = build_atlas(source, ATLAS, [by_usage[group] for group in sorted(by_usage)])
    for (map_id, job), keys in required.items():
        page_ids = {index['frames'][key][0] for key in keys}
        pages = [index['pages'][page_id] for page_id in sorted(page_ids)]
        print(json.dumps({'map_id': map_id, 'job': job, 'frames': len(keys), 'pages': len(pages),
                          'texture_bytes_rgba': sum(page['width'] * page['height'] * 4 for page in pages)}))
    return index


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--resources', type=Path, default=ROOT / 'client/assets/resources/development')
    build(parser.parse_args().resources.resolve(strict=True))
