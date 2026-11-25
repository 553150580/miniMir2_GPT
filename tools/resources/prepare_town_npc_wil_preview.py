"""只读导入当前紧凑城镇使用的原 NPC 帧，产物仅进入本机隔离 Creator 工程。"""

import argparse
import hashlib
import io
import json
from pathlib import Path

from PIL import Image

from build_hud_assets import base_meta, image_meta, stable_uuid
from inspect_wil import decode_frame, probe, sha256_file


ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / '资料索引/城镇NPC原版外观来源.json'
FIXTURE = ROOT / 'config/development/first-loop.json'
PACKAGE = 'town-npcs'


def json_bytes(value):
    """保留稳定的 UTF-8 清单，便于只读复验。"""
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')


def source_identities(fixture, starts):
    """以已登记外观号确定原站姿，并核对现行城镇所有 NPC 均有对应来源。"""
    towns = {row['id']: row for row in fixture['maps'] if row.get('compact_town')}
    references = fixture['_source_evidence']['compact_towns']
    if set(towns) != {row['map_id'] for row in references}:
        raise ValueError('紧凑城镇与来源记录的地图编号不一致')
    appearances = {}
    for town in references:
        if {npc['id'] for npc in towns[town['map_id']]['npcs']} != {npc['id'] for npc in town['npcs']}:
            raise ValueError(f'NPC 运行身份与原外观记录不一致：{town["map_id"]}')
        for npc in town['npcs']:
            identity, shape = str(npc['id']), str(npc['appearance'])
            if identity in appearances or shape not in starts:
                raise ValueError(f'NPC 编号重复或缺少原外观记录：{identity}')
            appearances[identity] = shape
    return appearances


def encode_frame(wil, source, palette):
    """保留原透明度、尺寸与脚点，不裁切可见区域。"""
    raw = decode_frame(wil, source, palette)
    width, height = source['width'], source['height']
    visible = sum(alpha != 0 for alpha in raw[3::4])
    if width <= 1 or height <= 1 or visible <= 4:
        raise ValueError(f'NPC 原站立帧空白：{source["frame_index"]}')
    output = io.BytesIO()
    with Image.frombytes('RGBA', (width, height), raw) as image:
        image.save(output, format='PNG')
    return output.getvalue(), visible


def compile_preview(source, contract, fixture):
    """根据已登记外观规则输出本机所用的三向 NPC 站立序列。"""
    wil, wix = source / 'npc.wil', source / 'npc.WIX'
    if any(path.is_symlink() or path.is_junction() for path in (wil, wix)):
        raise ValueError('NPC 来源包不能通过文件链接读取')
    archive_hashes = {path.name: sha256_file(path) for path in (wil, wix)}
    if archive_hashes != contract['archives']:
        raise ValueError('NPC 原始 WIL/WIX 与登记摘要不符')
    table = probe(wil, wix)
    if table['frame_count'] != contract['frame_count']:
        raise ValueError('NPC 原始图库帧数与登记不符')
    starts, action = contract['appearance_frame_starts'], contract['standing']
    appearances = source_identities(fixture, starts)
    profiles, frames, files = {}, {}, {}
    npc_profiles = {identity: f'npc_original_{shape}' for identity, shape in appearances.items()}
    for shape in sorted(set(appearances.values()), key=int):
        profile_id = f'npc_original_{shape}'
        directions = []
        for direction in range(action['directions']):
            start = starts[shape] + direction * action['direction_stride']
            keys = []
            for index in range(start, start + action['count']):
                row = table['frames'][index]
                png, visible = encode_frame(wil, row, table['palette'])
                stem, key = f'frame_{index:04d}', f'Npc:{index}'
                path = f'{PACKAGE}/frames/{stem}.png'
                files[path] = png
                metadata = image_meta(f'{PACKAGE}-{stem}', row['width'], row['height'], 0)
                for submeta in metadata['subMetas'].values():
                    submeta['displayName'] = stem
                files[path + '.meta'] = json_bytes(metadata)
                frames[key] = {field: row[field] for field in ('width', 'height', 'offset_x', 'offset_y')}
                frames[key].update(png=path, png_sha256=hashlib.sha256(png).hexdigest(), declared_empty=False,
                                   visible_alpha_pixels=visible, raw_frame=index)
                keys.append(key)
            directions.append({'direction': direction, 'frames': keys})
        standing = {'count': action['count'], 'duration_ms': action['duration_ms'], 'directions': directions}
        profiles[profile_id] = {'id': profile_id, 'layers': [{'package': 'Npc', 'actions': {'standing': standing}}]}
    manifest = {'schema_version': 1, 'npc_profiles': npc_profiles, 'profiles': profiles, 'frames': frames,
                'source_archives': archive_hashes, 'source_reference': contract['reference']}
    files[f'{PACKAGE}/manifest.json'] = json_bytes(manifest)
    files[f'{PACKAGE}/manifest.json.meta'] = json_bytes(base_meta('json', '2.0.1', stable_uuid(f'{PACKAGE}-manifest'), ['.json']))
    for name in (PACKAGE, f'{PACKAGE}/frames'):
        files[name + '.meta'] = json_bytes(base_meta('directory', '1.2.0', stable_uuid(name), []))
    return manifest, files


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path, help='本机已登记的 npc.wil 与 npc.WIX 所在 Data 目录')
    parser.add_argument('--output', type=Path, required=True, help='隔离 Creator 工程的 assets/resources/development')
    parser.add_argument('--check', action='store_true', help='仅核对已生成的本机文件')
    args = parser.parse_args()
    source, output = args.source.resolve(strict=True), args.output.resolve()
    if not output.is_relative_to(ROOT / '.work') or output.parts[-3:] != ('assets', 'resources', 'development'):
        raise ValueError('NPC 原画只能生成到项目 .work 内的隔离 Creator 项目')
    contract = json.loads(CONTRACT.read_text(encoding='utf-8'))
    fixture = json.loads(FIXTURE.read_text(encoding='utf-8'))
    manifest, files = compile_preview(source, contract, fixture)
    for name, contents in files.items():
        path = output / name
        if path.resolve() != path:
            raise ValueError(f'NPC 原画文件路径含链接：{name}')
        if path.is_file() and path.read_bytes() == contents:
            continue
        if args.check:
            raise ValueError(f'NPC 原画与本机导入结果不一致：{name}')
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(contents)
    print(json.dumps({key: len(manifest[key]) for key in ('npc_profiles', 'profiles', 'frames')}, ensure_ascii=False))


if __name__ == '__main__':
    main()
