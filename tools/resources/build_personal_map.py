"""从项目已保存的地表裁出开阔挂机图，保留17格历史地台和城镇资源。"""

import argparse
from copy import deepcopy
import hashlib
from io import BytesIO
import json
from pathlib import Path
import sys

from PIL import Image, ImageChops, ImageDraw

from build_hud_assets import base_meta, image_meta, stable_uuid
from personal_map_contract import OPEN_COLLISION, collision_basis


ROOT = Path(__file__).resolve().parents[2]
ASSETS = ROOT / 'client/assets/resources/development/maps'
sys.path.insert(0, str(ROOT / 'tools/config'))
import export_configs as config


def crop_floor(image, width, height, half_width, half_height, left, top):
    cropped = image.crop((left, top, left + width, top + height)).convert('RGBA')
    # 提取新菱形内的原像素，清除矩形四角中裁剪范围以外的老地砖。
    scale = 4
    polygon = [(half_width, 0), (width, height - half_height),
               (width - half_width, height), (0, half_height)]
    mask = Image.new('L', (width * scale, height * scale))
    ImageDraw.Draw(mask).polygon([(x * scale, y * scale) for x, y in polygon], fill=255)
    mask = mask.resize((width, height), Image.Resampling.BOX)
    cropped.putalpha(ImageChops.multiply(cropped.getchannel('A'), mask))
    return cropped


def build(tables, source, base_png):
    profile = tables['dev_first_loop_profile'][0]['parameters']
    definition = next(row for row in tables['game_maps'] if row['id'] == profile['map_id'])
    collision = next(row for row in tables['game_map_collisions'] if row['id'] == definition['collision_id'])
    if collision_basis(definition, collision) != OPEN_COLLISION:
        return None
    geometry = definition['visual_geometry']
    width, height = definition['width_cells'], definition['height_cells']
    tile_width, tile_height = geometry['tile_width'], geometry['tile_height']
    base_origin, origin = source['source_origin'], geometry['source_origin']
    offset_x, offset_y = origin['x'] - base_origin['x'], origin['y'] - base_origin['y']
    if ((collision['width_cells'], collision['height_cells']) != (width, height)
            or min(width, height) <= 0 or min(offset_x, offset_y) < 0
            or offset_x + width > source['width'] or offset_y + height > source['height']):
        raise ValueError('挂机图裁剪范围须位于已保存的地表内')
    if (geometry['projection'] != 'isometric_diamond' or source['projection'] != geometry['projection']
            or (tile_width, tile_height) != (source['tile_width'], source['tile_height'])
            or source['background_layers'] != ['back', 'middle'] or source['source_fronts_rendered'] is not False
            or collision['source_hash'] != source['source_sha256']
            or hashlib.sha256(base_png).hexdigest() != source['background_sha256']):
        raise ValueError('已保存地面来源、几何或贴图摘要与唯一Excel不一致')
    half_w, half_h = tile_width // 2, tile_height // 2
    surface_w, surface_h = (width + height) * half_w, (width + height) * half_h
    left = (offset_x - offset_y + source['height'] - height) * half_w
    top = (offset_x + offset_y) * half_h
    with Image.open(BytesIO(base_png)) as image:
        floor = crop_floor(image, surface_w, surface_h, height * half_w, height * half_h, left, top)
    png = BytesIO()
    floor.save(png, format='PNG')

    cells = deepcopy([row[offset_x:offset_x + width] for row in source['cells'][offset_y:offset_y + height]])
    source_blocked = []
    for y, row in enumerate(cells):
        for x, cell in enumerate(row):
            if cell['source_blocked']:
                source_blocked.append({'x': x, 'y': y})
            cell['blocked'] = False
    spawns = [row['cell'] for row in sorted(tables['game_monster_spawns'], key=lambda r: r['slot_index'])
              if row['map_id'] == definition['id']]
    locations = [definition['spawn_cell'], profile['pet_spawn'], *spawns]
    if (len(spawns) != 8 or any(not 0 <= p['x'] < width or not 0 <= p['y'] < height for p in locations)
            or len({(p['x'], p['y']) for p in locations}) != len(locations)):
        raise ValueError('挂机图的玩家、宠物及怪物出生必须各占一个有效格')
    depth = geometry['soil_thickness_px']
    data = {
        'id': definition['id'], 'name': definition['display_name'], 'development_only': True,
        'record_status': 'candidate', 'runtime_ready': False,
        'width': width, 'height': height, **geometry,
        'surface_width': surface_w, 'surface_height': surface_h, 'world_height': surface_h + depth,
        'background_offset_y': depth / 2, 'background_layers': ['back', 'middle'],
        'props': [], 'blocked': [], 'cells': cells, 'source_blocked': source_blocked,
        'source_fronts_rendered': False, 'collision_basis': OPEN_COLLISION, 'connected_walkable_count': width * height,
        'player_spawn': definition['spawn_cell'], 'pet_spawn': profile['pet_spawn'], 'monster_spawns': spawns,
        'source_sha256': source['source_sha256'], 'source_floor': 'maps/first-map.png',
        'source_floor_sha256': source['background_sha256'],
        'background': 'maps/personal-map.png', 'background_sha256': hashlib.sha256(png.getvalue()).hexdigest(),
    }
    return data, png.getvalue()


def write_json(path, data, check):
    if check:
        if config.load_json(path) != data:
            raise ValueError(f'挂机图数据或导入信息过期：{path.name}')
        return
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    schema = config.load_json(ROOT / 'config/schema/catalog.json')
    tables, errors, _, _ = config.collect(schema)
    errors += config.check_json_contracts(schema, tables)
    if errors:
        raise ValueError(errors)
    result = build(tables, config.load_json(ASSETS / 'first-map.json'), (ASSETS / 'first-map.png').read_bytes())
    if result is None:
        print('仍使用历史挂机图资源')
        return
    data, png = result
    path = ASSETS / 'personal-map.png'
    if args.check:
        if path.read_bytes() != png:
            raise ValueError('挂机图地表PNG过期')
    else:
        path.write_bytes(png)
    write_json(ASSETS / 'personal-map.json', data, args.check)
    png_meta = image_meta('personal-map', data['surface_width'], data['surface_height'], 0)
    write_json(ASSETS / 'personal-map.png.meta', png_meta, args.check)
    json_meta = base_meta('json', '2.0.1', stable_uuid('personal-map.json'), ['.json'])
    write_json(ASSETS / 'personal-map.json.meta', json_meta, args.check)
    print(json.dumps({'check': args.check, 'map': path.name, 'cells': data['connected_walkable_count'],
                      'surface': [data['surface_width'], data['surface_height']], 'soil_thickness_px': data['soil_thickness_px']}))


if __name__ == '__main__':
    main()
