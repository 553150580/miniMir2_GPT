"""从用户本机原图库导入普通怪物和首领外观，不改原件或战斗配置。"""

import argparse
import json
from pathlib import Path

from build_hud_assets import base_meta, image_meta, stable_uuid
from prepare_shinsu_assets import DIRECTIONS, ROOT, sha
from prepare_shinsu_lib_preview import encode_json, load_lib, read_frame


PACKAGE = 'original-monsters'
ART = {
    # 图库编号和名称对应独立参考源码 Enums.cs 的 Monster 枚举。
    'monster_m1_cat': (6, 'HookingCat', '2639b11bce1437d7902c6546c037d62704302618fe01fb757ba01e6db7d28ede'),
    'monster_m1_flower': (10, 'CannibalPlant', '410cad8d0781f99c39244a803b575d42cbf6706784b6be5324f5cd0249481752'),
    'monster_m1_spider': (12, 'SpittingSpider', 'b7b55455723dbbbc1a39c236f4a05abc422f61f15a0b7e9c2007bff3ffd86d27'),
    'monster_boss_guard': (18, 'OmaWarrior', '03a14b21056c66baea72e92285e9115df72ec3324e1a2376e45df8270094dcd1'),
    'monster_boss_lord': (34, 'WoomaTaurus', '5bfa3816f89a09e3b763ae9eff7e4776240ad4092b5a40dca90c93fd32d96bad'),
}
CHALLENGES = {'80001': 'monster_boss_guard', '80002': 'monster_boss_lord'}
ACTIONS = {'standing': 'Standing', 'walking': 'Walking', 'attack': 'Attack1', 'struck': 'Struck', 'die': 'Die'}


def enum_body(text, marker):
    return text.split(marker, 1)[1].split('{', 1)[1].split('}', 1)[0]


def compile_art(source):
    """按库自带动作表保留原索引、脚点、八向顺序和动作间隔。"""
    import re

    enums = (ROOT / '资料参考/crystal/Shared/Enums.cs').read_text(encoding='utf-8-sig')
    action_names = [item.strip() for item in enum_body(enums, 'public enum MirAction : byte').split(',') if item.strip()]
    monster_ids = dict(re.findall(r'^\s*(\w+)\s*=\s*(\d+)', enum_body(enums, 'public enum Monster : ushort'), re.MULTILINE))
    catalog = {'schema_version': 1, 'sources': {}, 'profiles': {}, 'frames': {}, 'challenge_profiles': CHALLENGES}
    files = {}
    for profile_id, (number, enum_name, digest) in ART.items():
        if int(monster_ids[enum_name]) != number:
            raise ValueError(f'怪物枚举和原库编号不一致：{enum_name}')
        filename = f'{number}.Lib'
        data, table = load_lib(source / filename, action_names)
        if sha(data) != digest:
            raise ValueError(f'原图库摘要变化：{filename}')
        catalog['sources'][filename] = {'name': enum_name, 'sha256': digest}
        actions = {}
        for action, source_action in ACTIONS.items():
            # 原食人花没有行走动作。保留玩法移动时只播放它原来的待机序列。
            if number == 10 and action == 'walking' and source_action not in table['frames']:
                source_action = 'Standing'
            row = table['frames'][source_action]
            count, stride = row['count'], row['count'] + row['skip']
            directions = []
            for direction, direction_name in enumerate(DIRECTIONS):
                keys = []
                for step in range(count):
                    index = row['start'] + direction * stride + (count - step - 1 if row['reverse'] else step)
                    key = f'Hunt:{number * 1000 + index}'
                    keys.append(key)
                    if key in catalog['frames']:
                        continue
                    image, metadata = read_frame(data, table, index)
                    name = f'frame_{number:02d}_{index:04d}'
                    target = f'{PACKAGE}/frames/{name}.png'
                    catalog['frames'][key] = {**metadata, 'png': target, 'png_sha256': sha(image),
                                               'raw_frame': index, 'source_archive': filename}
                    metadata = image_meta(f'original-monster-{name}', metadata['width'], metadata['height'], 0)
                    for part in metadata['subMetas'].values():
                        part['displayName'] = name
                    files[target] = image
                    files[target + '.meta'] = encode_json(metadata)
                directions.append({'direction': direction, 'name': direction_name, 'frames': keys})
            actions[action] = {'count': count, 'duration_ms': row['interval_ms'], 'loop': action == 'standing',
                               'blend': 'additive' if row['blend'] else 'normal', 'directions': directions}
        catalog['profiles'][profile_id] = {'id': profile_id, 'layers': [{'package': 'Hunt', 'actions': actions}]}
    files[f'{PACKAGE}/actors.json'] = encode_json(catalog)
    files[f'{PACKAGE}/actors.json.meta'] = encode_json(base_meta('json', '2.0.1', stable_uuid(f'{PACKAGE}-manifest'), ['.json']))
    for directory in (PACKAGE, f'{PACKAGE}/frames'):
        files[directory + '.meta'] = encode_json(base_meta('directory', '1.2.0', stable_uuid(directory), []))
    return catalog, files


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path, help='本机用户提供的原版 Monster Lib 目录')
    parser.add_argument('--output', type=Path, default=ROOT / 'client/assets/resources/development')
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    target = args.output.resolve()
    if not target.is_relative_to(ROOT) or target.parts[-3:] != ('assets', 'resources', 'development'):
        raise ValueError('只允许输出到项目内的 Creator 开发资源目录')
    catalog, files = compile_art(args.source.resolve(strict=True))
    for relative, raw in files.items():
        path = target / relative
        if path.resolve() != path:
            raise ValueError(f'输出路径存在重定向：{path.name}')
        if path.is_file() and path.read_bytes() == raw:
            continue
        if args.check:
            raise ValueError(f'原版怪物资源未生成或已变化：{relative}')
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(raw)
    print(json.dumps({'profiles': len(catalog['profiles']), 'frames': len(catalog['frames']),
                      'source_libraries': catalog['sources']}, ensure_ascii=False))


if __name__ == '__main__':
    main()
