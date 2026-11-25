"""只读加载用户指定的 79.Lib/80.Lib 到独立 Creator 试玩工程。"""

import argparse
import gzip
import io
import json
from pathlib import Path
import struct

from PIL import Image

from build_hud_assets import base_meta, image_meta, stable_uuid
from pet_birth_assets import parse_frame_set
from prepare_shinsu_assets import DIRECTIONS, PACKAGE, ROOT, sha


# 79 中的常态和出场，与 80 中的战斗动作，使用各自原表而非通用怪物帧序。
SOURCE_ACTIONS = {
    'appear': (79, 'Appear'), 'standing': (79, 'Standing'), 'walking': (79, 'Walking'),
    'attack': (80, 'Attack1'), 'struck': (80, 'Struck'), 'die': (80, 'Die'),
}


def encode_json(value):
    """使用 UTF-8 和稳定换行输出可比较的 Creator 导入清单。"""
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')


def load_lib(source, action_names):
    """读取原图库字节及其动作目录，不修改原路径。"""
    data = source.read_bytes()
    table = parse_frame_set(data, action_names)
    return data, table


def read_frame(data, table, index):
    """取原帧及脚点，从源 BGRA 解码成 Creator 可读的 RGBA。"""
    if not 0 <= index < table['image_count']:
        raise ValueError(f'原帧索引越界 {index}')
    offset = struct.unpack_from('<i', data, 12 + index * 4)[0]
    width, height, x, y, _, _, flags, length = struct.unpack_from('<6hBi', data, offset)
    if not 0 < width <= 2048 or not 0 < height <= 2048 or flags & 128 or length <= 0:
        raise ValueError(f'原帧尺寸或透明层不支持 {index}')
    start, end = offset + 17, offset + 17 + length
    if end > table['frame_seek']:
        raise ValueError(f'原帧压缩区间越界 {index}')
    expected = width * height * 4
    with gzip.GzipFile(fileobj=io.BytesIO(data[start:end])) as stream:
        raw = stream.read(expected + 1)
    if len(raw) != expected:
        raise ValueError(f'原帧解压后的像素长度不一致 {index}')
    visible = sum(pixel != 0 for pixel in raw[3::4])
    if visible == 0:
        raise ValueError(f'原动作引用没有可见像素的帧 {index}')
    image = Image.frombytes('RGBA', (width, height), raw, 'raw', 'BGRA')
    png = io.BytesIO()
    image.save(png, format='PNG')
    return png.getvalue(), {
        'width': width, 'height': height, 'offset_x': x, 'offset_y': y,
        'declared_empty': False, 'visible_alpha_pixels': visible, 'source_bgra_sha256': sha(raw),
    }


def compile_preview(source_dir):
    """将每个原动作按方向及原间隔映射到与骷髅相同的 Creator profile。"""
    enums = (ROOT / '资料参考/crystal/Shared/Enums.cs').read_text(encoding='utf-8-sig')
    block = enums.split('public enum MirAction : byte', 1)[1].split('{', 1)[1].split('}', 1)[0]
    action_names = [name.strip() for name in block.split(',') if name.strip()]
    libraries = {number: load_lib(source_dir / f'{number}.Lib', action_names) for number in (79, 80)}
    actions, frames, files = {}, {}, {}
    for action, (number, source_action) in SOURCE_ACTIONS.items():
        data, table = libraries[number]
        source = table['frames'][source_action]
        count, stride = source['count'], source['count'] + source['skip']
        directions = []
        for direction, direction_name in enumerate(DIRECTIONS):
            keys = []
            for current in range(count):
                progress = count - current - 1 if source['reverse'] else current
                index = source['start'] + direction * stride + progress
                key = f'Shinsu:{len(frames)}'
                name = f'frame_{number}{index:04d}'
                filename = f'frames/{name}.png'
                pixels, metadata = read_frame(data, table, index)
                frames[key] = {
                    **metadata, 'png': f'{PACKAGE}/{filename}', 'png_sha256': sha(pixels),
                    'raw_frame': index, 'source_archive': f'{number}.Lib',
                }
                files[f'{PACKAGE}/{filename}'] = pixels
                sprite_meta = image_meta(f'shinsu-{name}', metadata['width'], metadata['height'], 0)
                for submeta in sprite_meta['subMetas'].values():
                    submeta['displayName'] = name
                files[f'{PACKAGE}/{filename}.meta'] = encode_json(sprite_meta)
                keys.append(key)
            directions.append({'direction': direction, 'name': direction_name, 'frames': keys})
        actions[action] = {
            'count': count, 'duration_ms': source['interval_ms'], 'loop': action in ('standing', 'walking'),
            'blend': 'additive' if source['blend'] else 'normal', 'directions': directions,
        }
    profile = {'id': 'pet_shinsu', 'development_only': True, 'layers': [{'package': 'Shinsu', 'actions': actions}]}
    sources = {f'{number}.Lib': sha(data) for number, (data, _) in libraries.items()}
    files[f'{PACKAGE}/profile.json'] = encode_json({
        'schema_version': 1, 'source_archives': sources, 'profile': profile, 'frames': frames,
    })
    files[f'{PACKAGE}/profile.json.meta'] = encode_json(base_meta('json', '2.0.1', stable_uuid('shinsu-profile'), ['.json']))
    for directory in ('pets', PACKAGE, f'{PACKAGE}/frames'):
        files[f'{directory}.meta'] = encode_json(base_meta('directory', '1.2.0', stable_uuid(f'shinsu-{directory}'), []))
    return files, sources, len(frames)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source_dir', type=Path, help='用户在本机提供的 79.Lib/80.Lib 所在目录')
    parser.add_argument('--output', type=Path, required=True, help='独立试玩 Creator 工程的 assets/resources/development 目录')
    parser.add_argument('--check', action='store_true', help='只读比对导入产物')
    args = parser.parse_args()
    output = args.output.resolve()
    if not output.is_relative_to(ROOT / '.work') or output.parts[-3:] != ('assets', 'resources', 'development'):
        raise ValueError('本机参考试玩产物限项目 .work 内独立 Creator 工程')
    files, sources, count = compile_preview(args.source_dir.resolve())
    for filename, data in files.items():
        path = output / filename
        if path.resolve() != path:
            raise ValueError(f'目标文件路径存在符号链接 {filename}')
        if path.is_file() and path.read_bytes() == data:
            continue
        if args.check:
            raise ValueError(f'原件和试玩导入产物不匹配 {filename}')
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
    print(json.dumps({'frames': count, 'source_archives': sources, 'target': str(output / PACKAGE)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
