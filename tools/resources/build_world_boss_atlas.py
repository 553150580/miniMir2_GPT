"""按世界首领当前可见范围无损打包原帧，只生成本机 Creator 输入。"""

import argparse
import hashlib
import json
from pathlib import Path
import re

from PIL import Image

from build_hud_assets import base_meta, image_meta, stable_uuid


ROOT = Path(__file__).resolve().parents[2]
ATLAS = 'world-boss-atlas'
PAGE_SIZE = 2048
# 原纹理为线性插值，四周各复制一个像素避免采样到邻帧。
GUTTER = 1
FRAME_KEY = re.compile(r'[A-Za-z0-9]+:[0-9]+')


def effect_keys(value):
    """按表现清单收集技能、瞬移和状态使用的帧。"""
    if isinstance(value, str) and FRAME_KEY.fullmatch(value):
        yield value
    elif isinstance(value, (list, dict)):
        for child in value.values() if isinstance(value, dict) else value:
            yield from effect_keys(child)


def load_json(path):
    return json.loads(path.read_text(encoding='utf-8'))


def frame_catalog(source):
    """根据世界首领能看到的角色、宠物和技能编制待打包帧。"""
    manifest = load_json(source / 'manifest.json')
    gameplay = load_json(source / 'gameplay.json')
    profiles, frames = manifest['profiles'], manifest['frames']
    shinsu = source / 'pets/shinsu/profile.json'
    if shinsu.is_file() and 'pet_shinsu' not in profiles:
        local = load_json(shinsu)
        if local['schema_version'] != 1 or local['profile']['id'] != 'pet_shinsu' or frames.keys() & local['frames'].keys():
            raise ValueError('独立神兽帧清单与原资源冲突')
        profiles['pet_shinsu'] = local['profile']
        frames.update(local['frames'])
    visible = {row['profile'] for row in gameplay['jobs']}
    world_map = next((row for row in gameplay.get('challenge_maps', []) if row['scene_kind'] == 4), {})
    world_monsters = world_map.get('monster_definition_ids')
    if world_monsters is None:
        visible.update(row['profile'] for row in gameplay['monster_profiles'] if not row['profile'].startswith('monster_m1_'))
    else:
        original = load_json(source / 'original-monsters/actors.json')
        if original['schema_version'] != 1 or frames.keys() & original['frames'].keys() or profiles.keys() & original['profiles'].keys():
            raise ValueError('原版怪物清单与首领图集冲突')
        profiles.update(original['profiles'])
        frames.update(original['frames'])
        visible.update(original['challenge_profiles'][str(id)] for id in world_monsters)
    visible.update(row['profile'] for row in gameplay['pet_profiles'] if not row.get('appearance_optional') or row['profile'] in profiles)
    keys = set(effect_keys(manifest.get('statuses'))) | set(effect_keys(manifest.get('teleport')))
    for profile in sorted(visible):
        for layer in profiles[profile]['layers']:
            for action in layer['actions'].values():
                for direction in action['directions']:
                    keys.update(direction['frames'])
    for skill in gameplay['skills']:
        if skill['active'] or skill.get('activation_kind') == 'passive_proc':
            keys.update(effect_keys(manifest['effects'].get(str(skill['id']))))
    return {key: frames[key] for key in sorted(keys) if not frames[key].get('declared_empty')}


def pack(frames):
    """按帧高从大到小排版，用现有行的最小高度差减少纹理空隙。"""
    pages = []
    ordered = sorted(frames, key=lambda key: (-frames[key]['height'], -frames[key]['width'], key))
    for key in ordered:
        width, height = (frames[key][field] + 2 * GUTTER for field in ('width', 'height'))
        if not (2 * GUTTER < width <= PAGE_SIZE and 2 * GUTTER < height <= PAGE_SIZE):
            raise ValueError(f'原帧超出图集页尺寸：{key}')
        row_options = [
            (row[2] - height, PAGE_SIZE - row[1] - width, page_index, row_index)
            for page_index, page in enumerate(pages) for row_index, row in enumerate(page['rows'])
            if row[2] >= height and row[1] + width <= PAGE_SIZE
        ]
        if row_options:
            _, _, page_index, row_index = min(row_options)
            page = pages[page_index]
            row = page['rows'][row_index]
        else:
            page = next((item for item in pages if item['height'] + height <= PAGE_SIZE), None)
            if page is None:
                page = {'height': 0, 'width': 0, 'rows': [], 'frames': []}
                pages.append(page)
            row = [page['height'], 0, height]
            page['height'] += height
            page['rows'].append(row)
        y, x, _ = row
        row[1] += width
        page['width'] = max(page['width'], row[1])
        page['frames'].append((key, x + GUTTER, y + GUTTER))
    return pages


def place_frame(canvas, frame, x, y):
    """按原像素复制纹理和外沿，保持透明像素、色值和动作脚点。"""
    width, height = frame.size
    canvas.paste(frame, (x, y))
    canvas.paste(frame.crop((0, 0, width, 1)), (x, y - 1))
    canvas.paste(frame.crop((0, height - 1, width, height)), (x, y + height))
    canvas.paste(frame.crop((0, 0, 1, height)), (x - 1, y))
    canvas.paste(frame.crop((width - 1, 0, width, height)), (x + width, y))
    for from_x, to_x in ((0, x - 1), (width - 1, x + width)):
        for from_y, to_y in ((0, y - 1), (height - 1, y + height)):
            canvas.putpixel((to_x, to_y), frame.getpixel((from_x, from_y)))


def write_json(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, separators=(',', ':')) + '\n', encoding='utf-8', newline='\n')


def build(source):
    """生成 Creator 原生纹理页与帧索引，并复核每页裁出的像素。"""
    frames = frame_catalog(source)
    pages = pack(frames)
    destination = source / ATLAS
    destination.mkdir(exist_ok=True)
    write_json(destination.with_suffix('.meta'), base_meta('directory', '1.2.0', stable_uuid(ATLAS), []))
    index = {'schema_version': 1, 'pages': [], 'frames': {}}
    for page_id, page in enumerate(pages):
        name = f'page-{page_id:02d}'
        width, height = page['width'], page['height']
        canvas = Image.new('RGBA', (width, height))
        original_pixels = {}
        for key, x, y in page['frames']:
            frame = frames[key]
            image_path = (source / frame['png']).resolve(strict=True)
            if not image_path.is_relative_to(source) or image_path.suffix != '.png':
                raise ValueError(f'无效原帧路径：{key}')
            digest = hashlib.sha256(image_path.read_bytes()).hexdigest()
            if digest != frame['png_sha256']:
                raise ValueError(f'原帧摘要不匹配：{key}')
            with Image.open(image_path) as image:
                rgba = image.convert('RGBA')
            if rgba.size != (frame['width'], frame['height']):
                raise ValueError(f'原帧尺寸不匹配：{key}')
            place_frame(canvas, rgba, x, y)
            original_pixels[key] = hashlib.sha256(rgba.tobytes()).digest()
            index['frames'][key] = [page_id, x, y, *rgba.size, digest]
        output = destination / f'{name}.png'
        canvas.save(output, format='PNG', compress_level=4)
        with Image.open(output) as replay:
            for key, x, y in page['frames']:
                frame = frames[key]
                crop = replay.crop((x, y, x + frame['width'], y + frame['height']))
                if hashlib.sha256(crop.tobytes()).digest() != original_pixels[key]:
                    raise ValueError(f'图集导出未保留原像素：{key}')
        # 旧页面可能仍持有上一版索引。图像字节变化后，所有原生纹理和纹理元数据都用新 UUID。
        page_digest = hashlib.sha256(output.read_bytes()).hexdigest()
        metadata = image_meta(f'{ATLAS}-{name}-{page_digest}', width, height, 0)
        texture = metadata['subMetas']['6c48a']
        texture['displayName'] = name
        metadata['subMetas'] = {'6c48a': texture}
        metadata['userData']['type'] = 'texture'
        write_json(output.with_suffix('.png.meta'), metadata)
        index['pages'].append({'name': name, 'width': width, 'height': height, 'sha256': page_digest})
    # 只清除同一生成器上次留下且新索引没有引用的页。
    names = {page['name'] for page in index['pages']}
    for path in destination.iterdir():
        match = re.fullmatch(r'(page-[0-9]+)\.png(?:\.meta)?', path.name)
        if match and match[1] not in names:
            path.unlink()
    index_path = destination / 'index.json'
    write_json(index_path, index)
    digest = hashlib.sha256(index_path.read_bytes()).hexdigest()
    write_json(destination / 'index.json.meta', base_meta('json', '2.0.1', stable_uuid(f'{ATLAS}-index-{digest}'), ['.json']))
    print(json.dumps({'world_boss_atlas_pages': len(pages), 'source_frames': len(frames),
                      'page_bytes': sum((destination / f'{page["name"]}.png').stat().st_size for page in index['pages']),
                      'texture_bytes_rgba': sum(page['width'] * page['height'] * 4 for page in index['pages'])}))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--resources', type=Path, default=ROOT / 'client/assets/resources/development')
    build(parser.parse_args().resources.resolve(strict=True))
