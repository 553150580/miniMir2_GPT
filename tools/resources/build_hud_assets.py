#!/usr/bin/env python3
"""从只读1.80来源提取主界面原帧，仅写ui180资源和对应目录meta。"""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import uuid

from PIL import Image
import inspect_wil as wil


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / 'client/assets/resources/ui180'
DIRECTORY_META = OUTPUT.with_suffix('.meta')

# 这些映射只规定界面图像来源，不新增技能、物品数值或玩法状态。
# 名称、来源包、原始帧号、九宫格边距、用途。
ASSETS = [
    ('portrait-warrior-male', 'Prguse3', 365, 0, '战士男头像'),
    ('portrait-wizard-male', 'Prguse3', 366, 0, '法师男头像'),
    ('portrait-taoist-male', 'Prguse3', 367, 0, '道士男头像'),
    ('portrait-warrior-female', 'Prguse3', 368, 0, '战士女头像'),
    ('portrait-wizard-female', 'Prguse3', 369, 0, '法师女头像'),
    ('portrait-taoist-female', 'Prguse3', 370, 0, '道士女头像'),
    ('button-normal', 'Prguse3', 230, 4, '无字按钮常态'),
    ('button-pressed', 'Prguse3', 231, 4, '无字按钮按下态'),
    ('button-small-normal', 'Prguse3', 245, 3, '无字小按钮常态'),
    ('button-small-pressed', 'Prguse3', 246, 3, '无字小按钮按下态'),
    ('slot', 'Prguse3', 240, 3, '无字方形物品槽'),
    ('slot-small', 'Prguse3', 242, 3, '无字小方槽'),
    ('slot-highlight', 'Prguse3', 600, 0, '原始白色方框，仅作为选择高亮图像'),
    ('panel', 'Prguse3', 235, 8, '无文字和独立关闭按钮的黑底边框面板'),
    ('icon-gold', 'Prguse', 28, 0, '金币入口图标'),
    ('icon-yuanbao', 'Prguse3', 81, 0, '元宝入口图标'),
    ('orb-neutral', 'Prguse', 5, 0, '灰色高光球，允许场景Sprite着色'),
    ('orb-red', 'Prguse', 6, 0, '红色球原帧'),
    ('orb-red-blue', 'Prguse', 4, 0, '红蓝半球原帧'),
    ('bar-red', 'Prguse3', 386, 0, '红色状态条原帧'),
    ('bar-blue', 'Prguse3', 387, 0, '蓝色状态条原帧'),
    ('bar-green', 'Prguse3', 388, 0, '绿色状态条原帧'),
    ('icon-red-potion', 'Items', 3, 0, '通用红药入口图标，物品绑定为候选视觉映射'),
    ('icon-blue-potion', 'Items', 4, 0, '通用蓝药入口图标，物品绑定为候选视觉映射'),
    ('icon-attack', 'Prguse', 74, 0, '剑形普攻入口图标，不表示新增技能'),
    ('icon-settings', 'Prguse', 77, 0, '工具形设置入口图标'),
    ('icon-skills', 'MagIcon', 2, 0, '技能入口图标，不表示火球技能已实现'),
    ('icon-pet', 'MagIcon', 30, 0, '骷髅形宠物入口图标，不表示新增召唤技能'),
    ('icon-basic-skill', 'MagIcon', 0, 0, '剑形技能入口图标，不表示新增技能'),
    ('checkmark', 'Prguse3', 353, 0, '红色勾选标记'),
]


def stable_uuid(name: str) -> str:
    return str(uuid.uuid5(uuid.NAMESPACE_URL, 'minimir2://ui180/' + name))


def canonical(value) -> str:
    return json.dumps(value, ensure_ascii=False, indent=2) + '\n'


def allowed_path(path: Path) -> None:
    """限制工具输出范围，禁止触及既有布局和其它生成目录。"""
    if path != DIRECTORY_META and OUTPUT not in path.parents:
        raise ValueError('输出不在ui180范围：' + str(path))
    cursor = path
    while cursor != ROOT:
        if cursor.is_symlink():
            raise ValueError('输出路径不接受符号链接：' + str(cursor))
        cursor = cursor.parent


def write_json(path: Path, value, check: bool) -> None:
    allowed_path(path)
    expected = canonical(value)
    if check:
        # Creator可能规范化JSON排版或整数表达；校验全部字段语义，不把排版变化视为资源变更。
        if json.loads(path.read_text(encoding='utf-8')) != value:
            raise ValueError('元数据不一致：' + str(path.relative_to(ROOT)))
    else:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(expected, encoding='utf-8')


def base_meta(importer: str, version: str, asset_uuid: str, files: list[str]) -> dict:
    return {'ver': version, 'importer': importer, 'imported': True,
            'uuid': asset_uuid, 'files': files, 'subMetas': {}, 'userData': {}}


def image_meta(name: str, width: int, height: int, border: int) -> dict:
    asset_uuid = stable_uuid(name + '.png')
    texture = base_meta('texture', '1.0.22', asset_uuid + '@6c48a', ['.json'])
    texture.update({'displayName': name, 'id': '6c48a', 'name': 'texture'})
    texture['userData'] = {
        'wrapModeS': 'clamp-to-edge', 'wrapModeT': 'clamp-to-edge',
        'imageUuidOrDatabaseUri': asset_uuid, 'isUuid': True, 'visible': False,
        'minfilter': 'linear', 'magfilter': 'linear', 'mipfilter': 'none', 'anisotropy': 0,
    }
    sprite = base_meta('sprite-frame', '1.0.12', asset_uuid + '@f9941', ['.json'])
    sprite.update({'displayName': name, 'id': 'f9941', 'name': 'spriteFrame'})
    sprite['userData'] = {
        'trimType': 'none', 'trimThreshold': 1, 'rotated': False,
        'offsetX': 0, 'offsetY': 0, 'trimX': 0, 'trimY': 0,
        'width': width, 'height': height, 'rawWidth': width, 'rawHeight': height,
        'borderTop': border, 'borderBottom': border, 'borderLeft': border, 'borderRight': border,
        'packable': False, 'pixelsToUnit': 100, 'pivotX': 0.5, 'pivotY': 0.5, 'meshType': 0,
        'vertices': {
            'rawPosition': [-width/2, -height/2, 0, width/2, -height/2, 0,
                            -width/2, height/2, 0, width/2, height/2, 0],
            'indexes': [0, 1, 2, 2, 1, 3],
            'uv': [0, height, width, height, 0, 0, width, 0],
            'nuv': [0, 0, 1, 0, 0, 1, 1, 1],
            'minPos': [-width/2, -height/2, 0], 'maxPos': [width/2, height/2, 0],
        },
        'isUuid': True, 'imageUuidOrDatabaseUri': asset_uuid + '@6c48a', 'atlasUuid': '',
    }
    result = base_meta('image', '1.0.27', asset_uuid, ['.json', '.png'])
    result['subMetas'] = {'6c48a': texture, 'f9941': sprite}
    result['userData'] = {'type': 'sprite-frame', 'fixAlphaTransparencyArtifacts': False,
                          'hasAlpha': True, 'redirect': asset_uuid + '@6c48a'}
    return result


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    packages, sources, assets, frames = {}, {}, {}, []
    for name, package, index, border, purpose in ASSETS:
        if package not in packages:
            source, source_index = wil.locate_pair('client_180_named', package)
            data = wil.probe(source, source_index)
            packages[package] = (source, source_index, data)
            sources[package] = {
                'source_id': 'client_180_named', 'wil_path': str(source), 'wix_path': str(source_index),
                'wil_sha256': wil.sha256_file(source), 'wix_sha256': wil.sha256_file(source_index),
                'format': data['format_id'], 'frame_count': data['frame_count'],
            }
        source, _, data = packages[package]
        frame = data['frames'][index]
        if frame['declared_empty']:
            raise ValueError(f'不能导出空帧：{package}:{index}')
        rgba = wil.decode_frame(source, frame, data['palette'])
        visible = sum(alpha > 0 for alpha in rgba[3::4])
        if not visible:
            raise ValueError(f'不能导出全透明帧：{package}:{index}')
        image = Image.frombytes('RGBA', (frame['width'], frame['height']), rgba)
        path = OUTPUT / (name + '.png')
        allowed_path(path)
        if not args.check:
            path.parent.mkdir(parents=True, exist_ok=True)
            image.save(path, format='PNG')
        with Image.open(path) as reopened:
            if reopened.mode != 'RGBA' or reopened.size != image.size or reopened.tobytes() != rgba:
                raise ValueError('PNG回读与源帧不一致：' + name)
        meta = image_meta(name, image.width, image.height, border)
        write_json(path.with_suffix('.png.meta'), meta, args.check)
        asset_uuid = meta['uuid']
        assets[name] = {
            'path': path.relative_to(ROOT).as_posix(), 'resource_path': 'ui180/' + name + '/spriteFrame',
            'width': image.width, 'height': image.height, 'image_uuid': asset_uuid,
            'texture_uuid': asset_uuid + '@6c48a', 'sprite_frame_uuid': asset_uuid + '@f9941',
            'borders': {'top': border, 'bottom': border, 'left': border, 'right': border},
        }
        frames.append({
            'name': name, 'package': package, 'raw_frame': index, 'purpose': purpose,
            'path': path.relative_to(ROOT).as_posix(), 'source_offset': frame['source_offset'],
            'width': image.width, 'height': image.height,
            'offset_x': frame['offset_x'], 'offset_y': frame['offset_y'],
            'png_sha256': wil.sha256_file(path), 'rgba_sha256': hashlib.sha256(rgba).hexdigest(),
            'visible_alpha_pixels': visible, 'png_roundtrip_verified': True,
            'image_changed': False, 'sprite_frame_uuid': asset_uuid + '@f9941',
        })
    # MYF物品表的Looks不直接等同于当前1.80客户端中的可用图像，记录而不猜配。
    item_table = ROOT / '资料参考/myf/Mud2/DB/ApexM2_stditems.csv'
    manifest = {
        'schema_version': 1, 'purpose': '主界面原始图像提取与来源追踪',
        'source_images_readonly': True, 'pixel_transforms': [],
        'runtime_gameplay_ready': False, 'skill_implementation_claimed': False,
        'sources': sources, 'frames': frames,
        'mapping_notes': [
            '头像、按钮、槽、边框及图标经原帧联系表目视选择，选择记录不构成技能或完整1.80玩法还原证明。',
            '全部PNG保留源帧尺寸、像素、透明度；不裁切、不改色。场景可使用Sprite着色和元数据九宫格。',
            'MYF金创药(小量)/魔法药(小量)的Looks为317/316，但本1.80 Items对应帧不可用；Items3/4只作为通用红蓝药入口的候选视觉映射，不修改或声明该物品图号已核验。',
            'MagIcon0/2/30及Prguse74/77只作为入口图标，不表示技能已实现或物品已拥有。',
        ],
        'item_mapping_evidence': {'path': item_table.relative_to(ROOT).as_posix(),
                                  'sha256': wil.sha256_file(item_table),
                                  'rows': [{'item_id': 35, 'source_looks': 317}, {'item_id': 36, 'source_looks': 316}],
                                  'mapping_status': 'candidate_visual_icon_only'},
    }
    for package, (source, source_index, _) in packages.items():
        if wil.sha256_file(source) != sources[package]['wil_sha256'] or wil.sha256_file(source_index) != sources[package]['wix_sha256']:
            raise ValueError('提取期间源文件发生变化：' + package)
    for name, value in [('manifest.json', manifest), ('sprite-frames.json', {'schema_version': 1, 'assets': assets})]:
        write_json(OUTPUT / name, value, args.check)
        write_json(OUTPUT / (name + '.meta'), base_meta('json', '2.0.1', stable_uuid(name), ['.json']), args.check)
    write_json(DIRECTORY_META, base_meta('directory', '1.2.0', stable_uuid('directory'), []), args.check)
    print(canonical({'checked' if args.check else 'exported': len(frames), 'sources': len(sources),
                     'source_pixels_preserved': True, 'output': OUTPUT.relative_to(ROOT).as_posix()}).strip())


if __name__ == '__main__':
    main()
