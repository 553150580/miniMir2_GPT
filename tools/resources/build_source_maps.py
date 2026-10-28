#!/usr/bin/env python3
"""从唯一工作簿及只读来源原图生成完整城镇地图；不改个人地图或保存布局。"""
from __future__ import annotations

import argparse
import base64
from collections import Counter, defaultdict
from dataclasses import dataclass
import hashlib
import json
import math
from pathlib import Path
import re
import struct
import sys

from PIL import Image, __version__ as PILLOW_VERSION

import inspect_wil as wil


ROOT = Path(__file__).resolve().parents[2]
GEOMETRY_FIELDS = {
    'projection', 'tile_width', 'tile_height', 'soil_thickness_px', 'source_origin',
    'chunk_size_cells', 'source_map_code',
}
FRONT_LIBRARIES = {2: 'Objects', 5: 'Objects4', 6: 'Objects5', 7: 'Objects6'}
FLOOR_FIELDS = ['x', 'y', 'layer', 'frame']
OBJECT_FIELDS = ['x', 'y', 'layer', 'frame', 'animation_count', 'animation_tick',
                 'blend', 'offset_mode', 'door_id', 'door_offset']


def digest(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def json_bytes(value, compact=False) -> bytes:
    options = {'separators': (',', ':')} if compact else {'indent': 2}
    return (json.dumps(value, ensure_ascii=False, **options) + '\n').encode('utf-8')


@dataclass(frozen=True)
class Cell:
    back: int
    middle: int
    front: int
    door_raw: int
    door_offset: int
    animation: int
    tick: int
    library: int
    light: int

    @property
    def blocked(self):
        return bool((self.back | self.middle | self.front) & 0x8000)

    @property
    def door(self):
        return self.door_raw & 0x7f


class SourceMap:
    """保留列优先原字节，所有对外格坐标仍为完整原图坐标。"""
    def __init__(self, data: bytes):
        if len(data) < 52:
            raise ValueError('来源地图头截断')
        self.width, self.height = struct.unpack_from('<HH', data)
        if not self.width or not self.height or data[4:18] != b'\rLegend of mir':
            raise ValueError('只支持已核验的Mir v0来源地图签名')
        if len(data) != 52 + self.width * self.height * 12:
            raise ValueError('来源地图须为52字节头加12字节列优先原格')
        self.data = data

    def raw(self, x: int, y: int) -> bytes:
        if not 0 <= x < self.width or not 0 <= y < self.height:
            raise ValueError('来源地图坐标越界')
        offset = 52 + (x * self.height + y) * 12
        return self.data[offset:offset + 12]

    def cell(self, x: int, y: int) -> Cell:
        return Cell(*struct.unpack('<HHHBBBBBB', self.raw(x, y)))


def validate_definition(definition: dict, collision: dict, source: SourceMap, source_hash: str):
    geometry = definition.get('visual_geometry')
    if not isinstance(geometry, dict) or not GEOMETRY_FIELDS.issubset(geometry) or set(geometry) - GEOMETRY_FIELDS - {'minimap'}:
        raise ValueError('来源地图visual_geometry必须由唯一Excel显式提供七项必需字段，仅允许额外可选minimap')
    if geometry['projection'] != 'source_rectangular' or any(type(geometry[key]) is not int for key in ('tile_width', 'tile_height')) or geometry['tile_width'] != 48 or geometry['tile_height'] != 32:
        raise ValueError('本工具只支持已核验的source_rectangular原生48×32投影')
    if type(geometry['soil_thickness_px']) is not int or geometry['soil_thickness_px'] != 0 or geometry['source_origin'] != {'x': 0, 'y': 0} or any(type(value) is not int for value in geometry['source_origin'].values()):
        raise ValueError('完整来源地图不能添加土层或改变来源坐标原点')
    size = geometry['chunk_size_cells']
    if type(size) is not int or not 1 <= size <= 64:
        raise ValueError('chunk_size_cells须由唯一Excel指定1至64的整数')
    if not isinstance(geometry['source_map_code'], str) or not re.fullmatch(r'[A-Za-z0-9_-]+', geometry['source_map_code']):
        raise ValueError('来源地图号必须是无路径的显式字符串')
    if 'minimap' in geometry:
        minimap = geometry['minimap']
        if not isinstance(minimap, dict) or set(minimap) != {'package', 'raw_frame'}:
            raise ValueError('可选minimap必须显式提供package/raw_frame，不补默认帧')
        if not isinstance(minimap['package'], str) or not re.fullmatch(r'[A-Za-z0-9_]+', minimap['package']):
            raise ValueError('小地图来源包名无效')
        if type(minimap['raw_frame']) is not int or minimap['raw_frame'] < 0:
            raise ValueError('小地图必须提供显式非负整数原帧')
    dimensions = (source.width, source.height)
    if (definition.get('width_cells'), definition.get('height_cells')) != dimensions:
        raise ValueError('唯一Excel地图尺寸与完整来源地图不一致')
    if definition.get('collision_id') != collision.get('id') or (collision.get('width_cells'), collision.get('height_cells')) != dimensions:
        raise ValueError('唯一Excel地图与碰撞引用或尺寸不一致')
    if collision.get('source_hash') != source_hash:
        raise ValueError('来源原图SHA256与唯一Excel碰撞source_hash不同')
    if collision.get('walkable_encoding') != 'source_map_v0_closed_doors':
        raise ValueError('唯一Excel须显式选择source_map_v0_closed_doors')
    expected = {'source_file': f'myf:Mir200/Map/{geometry["source_map_code"]}.map', 'door_policy': 'closed'}
    if collision.get('blocked_cells') != expected:
        raise ValueError('唯一Excel须显式引用MYF来源原图并选择关闭门开发候选')
    return geometry


def workbook_definition(map_id: int):
    sys.path.insert(0, str(ROOT / 'tools/config'))
    import export_configs as config
    tables, errors, _, _ = config.collect(config.load_json(ROOT / 'config/schema/catalog.json'))
    if errors:
        raise ValueError('唯一Excel结构校验失败：' + '；'.join(errors))
    definitions = [row for row in tables['game_maps'] if row['id'] == map_id]
    if len(definitions) != 1:
        raise ValueError(f'唯一Excel中地图{map_id}不存在或不唯一')
    definition = definitions[0]
    collisions = [row for row in tables['game_map_collisions'] if row['id'] == definition['collision_id']]
    if len(collisions) != 1:
        raise ValueError('唯一Excel地图关联碰撞不存在或不唯一')
    return definition, collisions[0]


def minimap_source_record(source_path: Path, map_code: str, selected: dict):
    """只读取相同Mir200下的玩法映射副本，记录事实；帧选择始终来自唯一Excel。"""
    mir_root = source_path.parent.parent
    path = wil.readonly_path(mir_root / 'Envir/MiniMap.txt', mir_root)
    raw = path.read_bytes()
    matches = []
    for line_number, line in enumerate(raw.decode('gb18030').splitlines(), 1):
        fields = line.split(';', 1)[0].split()
        if not fields or fields[0] != map_code:
            continue
        if len(fields) != 2 or not re.fullmatch(r'\d+', fields[1]):
            raise ValueError('小地图来源映射行结构尚未核验：' + str(line_number))
        matches.append((line_number, int(fields[1])))
    if len(matches) != 1:
        raise ValueError('配置小地图时，来源MiniMap.txt必须包含唯一地图号映射：' + map_code)
    line, image_number = matches[0]
    return {
        'path': str(path), 'sha256': digest(raw), 'encoding': 'gb18030',
        'source_line': line, 'map_code': map_code, 'mapped_image_number': image_number,
        'configured_package': selected['package'], 'configured_raw_frame': selected['raw_frame'],
        'frame_selection_basis': 'unique_workbook.visual_geometry.minimap',
        'mapping_status': 'cross_source_candidate',
        'index_base_contract': '来源编号减一只作跨来源候选记录，不由此推导或覆盖唯一Excel原帧',
        'one_based_correspondence_observed': image_number == selected['raw_frame'] + 1,
        'source_engine_index_base_verified': False,
    }


class Artifacts:
    """只写独立生成目录；已有同像素PNG保留原字节与Creator引用。"""
    def __init__(self, root: Path, check=False):
        self.root, self.check = root, check
        self.expected = set()

    def path(self, relative: str):
        path = self.root / relative
        if not path.is_relative_to(self.root) or any(part == '..' for part in Path(relative).parts):
            raise ValueError('生成物路径越出目标目录')
        cursor = path
        while cursor != self.root.parent:
            if cursor.is_symlink():
                raise ValueError('生成目录或文件不能是符号链接')
            cursor = cursor.parent
        self.expected.add(relative)
        return path

    def write_json(self, relative: str, value, compact=False):
        path, data = self.path(relative), json_bytes(value, compact)
        if self.check:
            if not path.is_file() or path.read_bytes() != data:
                raise ValueError('生成元数据缺失或过期：' + relative)
        elif not path.is_file() or path.read_bytes() != data:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
        return digest(data)

    def write_png(self, relative: str, width: int, height: int, rgba: bytes):
        path = self.path(relative)
        same = False
        if path.is_file():
            with Image.open(path) as old:
                same = old.mode == 'RGBA' and old.size == (width, height) and old.tobytes() == rgba
        if not same:
            if self.check:
                raise ValueError('来源帧PNG缺失或像素不同：' + relative)
            path.parent.mkdir(parents=True, exist_ok=True)
            Image.frombytes('RGBA', (width, height), rgba).save(path, format='PNG')
        with Image.open(path) as image:
            if image.mode != 'RGBA' or image.size != (width, height) or image.tobytes() != rgba:
                raise ValueError('来源PNG回读像素不同：' + relative)
        return wil.sha256_file(path)

    def verify_inventory(self):
        if not self.root.exists():
            raise ValueError('生成目录不存在')
        actual = {path.relative_to(self.root).as_posix() for path in self.root.rglob('*')
                  if path.is_file() and path.suffix.lower() in ('.json', '.png')}
        extra = actual - self.expected
        if extra:
            raise ValueError('生成目录含旧版本额外产物，请使用独立输出目录复验：' + ','.join(sorted(extra)[:8]))


class Frames:
    def __init__(self, client_root: Path, artifacts: Artifacts):
        self.artifacts = artifacts
        self.packages = {}
        self.frames = {}
        self.sources = {}
        wil.SOURCE_ROOTS['source_map_client'] = client_root / 'Data'

    def package(self, name):
        if name not in self.packages:
            path, index = wil.locate_pair('source_map_client', name)
            wil_hash, wix_hash = wil.sha256_file(path), wil.sha256_file(index)
            data = wil.probe(path, index)
            self.packages[name] = (path, index, data)
            self.sources[name] = {
                'package': name, 'wil_path': str(path), 'wix_path': str(index),
                'wil_sha256': wil_hash, 'wix_sha256': wix_hash,
                'format': data['format_id'], 'frame_count': data['frame_count'],
                'palette_sha256': data['palette_sha256'],
            }
        return self.packages[name]

    def frame(self, name: str, index: int):
        key = f'{name}:{index}'
        if key not in self.frames:
            path, _, data = self.package(name)
            if not 0 <= index < data['frame_count']:
                raise ValueError(f'来源地图引用越界帧：{key}')
            frame = data['frames'][index]
            rgba = wil.decode_frame(path, frame, data['palette'])
            relative = f'frames/{name}/frame_{index:06d}.png'
            png_sha = None if frame['declared_empty'] else self.artifacts.write_png(relative, frame['width'], frame['height'], rgba)
            self.frames[key] = {
                'package': name, 'raw_frame': index,
                'png': None if frame['declared_empty'] else relative, 'png_sha256': png_sha,
                'rgba_sha256': digest(rgba), 'width': frame['width'], 'height': frame['height'],
                'offset_x': frame['offset_x'], 'offset_y': frame['offset_y'],
                'source_offset': frame['source_offset'], 'declared_empty': frame['declared_empty'],
                'png_roundtrip_verified': not frame['declared_empty'],
            }
        return key

    def verify_frozen(self):
        for name, record in self.sources.items():
            path, index, _ = self.packages[name]
            if wil.sha256_file(path) != record['wil_sha256'] or wil.sha256_file(index) != record['wix_sha256']:
                raise ValueError('转换期间来源WIL/WIX改变：' + name)


def set_bit(bits: bytearray, index: int):
    bits[index >> 3] |= 1 << (index & 7)


def collision_data(source: SourceMap):
    length = (source.width * source.height + 7) // 8
    static, doors = bytearray(length), bytearray(length)
    for y in range(source.height):
        for x in range(source.width):
            cell, index = source.cell(x, y), y * source.width + x
            if cell.blocked:
                set_bit(static, index)
            if cell.door:
                set_bit(doors, index)
    runtime = bytes(a | b for a, b in zip(static, doors))
    result = {'encoding': 'row_major_lsb0_blocked_bitset_base64', 'index': 'y*width+x',
              'one_means': 'blocked', 'bit_count': source.width * source.height,
              'door_policy': 'closed_development_candidate',
              'static_basis': '(back_raw|middle_raw|front_raw)&0x8000',
              'door_basis': 'cell_byte_6&0x7f', 'runtime_basis': 'static_bits OR closed_door_bits'}
    for name, bits in [('static', bytes(static)), ('closed_door', bytes(doors)), ('runtime', runtime)]:
        result[name + '_bits'] = base64.b64encode(bits).decode('ascii')
        result[name + '_count'] = sum(value.bit_count() for value in bits)
        result[name + '_sha256'] = digest(bits)
    return result


def flat_frame(frame):
    return (frame['width'], frame['height']) in ((48, 32), (96, 64))


def pixel_rect(x: int, y: int, layer: int, frame: dict, offset_mode=0):
    """坐标向下；普通原图忽略WIL偏移，仅来源火把特例应用偏移。"""
    if frame['declared_empty']:
        return None
    left = x * 48
    top = y * 32 if layer <= 2 else (y + 1) * 32 - frame['height']
    if offset_mode:
        left += frame['offset_x']
        top += frame['offset_y']
    return [left, top, left + frame['width'], top + frame['height']]


def union_rect(a, b):
    if a is None:
        return b
    if b is None:
        return a
    return [min(a[0], b[0]), min(a[1], b[1]), max(a[2], b[2]), max(a[3], b[3])]


def bucket_keys(rect, bucket_width, bucket_height):
    if rect is None or rect[2] <= rect[0] or rect[3] <= rect[1]:
        return []
    return [(x, y) for y in range(math.floor(rect[1] / bucket_height), math.ceil(rect[3] / bucket_height))
            for x in range(math.floor(rect[0] / bucket_width), math.ceil(rect[2] / bucket_width))]


def cell_draws(x: int, y: int, cell: Cell, frames):
    floor, objects = [], []
    if x % 2 == 0 and y % 2 == 0 and cell.back & 0x7fff:
        floor.append([x, y, 0, frames.frame('Tiles', (cell.back & 0x7fff) - 1)])
    # 来源客户端MiddleImage为有符号short；高位墙字段保留碰撞，不绘成猜测图片。
    if 0 < cell.middle < 0x8000:
        key = frames.frame('SmTiles', cell.middle - 1)
        if flat_frame(frames.frames[key]):
            floor.append([x, y, 1, key])
        elif cell.middle - 1 > 0:
            objects.append([x, y, 3, key, 0, 0, 0, 0, 0, 0])
    if cell.front & 0x7fff:
        library = cell.library + 2
        if library not in FRONT_LIBRARIES:
            raise ValueError(f'来源前景库尚未核验：({x},{y}) library={library}')
        package, index = FRONT_LIBRARIES[library], (cell.front & 0x7fff) - 1
        key = frames.frame(package, index)
        count, blend = cell.animation & 0x7f, bool(cell.animation & 0x80)
        if cell.animation and not (package == 'Objects' and index == 2723 and cell.animation == 0x8a and cell.tick == 0):
            raise ValueError(f'来源动画尚未核验：({x},{y}) {key} animation={cell.animation} tick={cell.tick}')
        # 来源DrawFloor仍先绘制原帧尺寸属于地板的前景，再绘制非地板或动画前景。
        if flat_frame(frames.frames[key]):
            floor.append([x, y, 2, key])
        if count or not flat_frame(frames.frames[key]):
            for offset in range(count):
                frames.frame(package, index + offset)
            offset_mode = int(blend and 2723 <= index <= 2732)
            objects.append([x, y, 4, key, count, cell.tick, int(blend), offset_mode, cell.door, cell.door_offset])
    elif cell.animation:
        raise ValueError(f'无前景图像却带动画：({x},{y})')
    return floor, objects


def object_rect(record, frames):
    x, y, layer, key, count, _, _, offset_mode, _, _ = record
    first = frames.frames[key]
    result = None
    for offset in range(max(count, 1)):
        frame = frames.frames[f'{first["package"]}:{first["raw_frame"] + offset}']
        result = union_rect(result, pixel_rect(x, y, layer, frame, offset_mode))
    return result


def build_chunks(source: SourceMap, geometry: dict, frames, artifacts: Artifacts):
    size = geometry['chunk_size_cells']
    bucket_width, bucket_height = size * 48, size * 32
    spatial = defaultdict(set)
    chunks = []
    render_bounds = None
    totals = Counter()
    door_ids = set()
    for cy in range(math.ceil(source.height / size)):
        for cx in range(math.ceil(source.width / size)):
            x0, y0 = cx * size, cy * size
            width, height = min(size, source.width - x0), min(size, source.height - y0)
            chunk_id = f'{cx}_{cy}'
            floor, objects, raw_cells = [], [], bytearray()
            rect = None
            for y in range(y0, y0 + height):
                for x in range(x0, x0 + width):
                    raw_cells.extend(source.raw(x, y))
                    cell = source.cell(x, y)
                    if cell.door:
                        door_ids.add(cell.door)
                    totals['animated_cells'] += bool(cell.animation)
                    totals['light_cells'] += bool(cell.light)
                    cell_floor, cell_objects = cell_draws(x, y, cell, frames)
                    floor.extend(cell_floor)
                    objects.extend(cell_objects)
                    for record in cell_floor:
                        bounds = pixel_rect(record[0], record[1], record[2], frames.frames[record[3]])
                        rect = union_rect(rect, bounds)
                        for bucket in bucket_keys(bounds, bucket_width, bucket_height):
                            spatial[bucket].add(chunk_id)
                    for record in cell_objects:
                        bounds = object_rect(record, frames)
                        rect = union_rect(rect, bounds)
                        for bucket in bucket_keys(bounds, bucket_width, bucket_height):
                            spatial[bucket].add(chunk_id)
            totals['floor_draws'] += len(floor)
            totals['object_draws'] += len(objects)
            data = {'schema_version': 1, 'id': chunk_id, 'x': cx, 'y': cy,
                    'cell_bounds': [x0, y0, width, height],
                    'cells_base64': base64.b64encode(raw_cells).decode('ascii'), 'floor': floor, 'objects': objects}
            relative = f'chunks/chunk_{cx}_{cy}.json'
            chunk_sha = artifacts.write_json(relative, data, compact=True)
            chunks.append({'id': chunk_id, 'x': cx, 'y': cy, 'path': relative,
                           'cell_bounds': data['cell_bounds'], 'draw_bounds': rect, 'sha256': chunk_sha})
            render_bounds = union_rect(render_bounds, rect)
    spatial_index = {'bucket_width_px': bucket_width, 'bucket_height_px': bucket_height,
                     'coordinates': 'source_pixels_y_down_half_open',
                     'buckets': {f'{x},{y}': sorted(spatial[(x, y)]) for x, y in sorted(spatial, key=lambda p: (p[1], p[0]))}}
    totals['door_groups'] = len(door_ids)
    totals['chunks'] = len(chunks)
    return chunks, spatial_index, render_bounds, dict(totals)


def source_evidence():
    records = [
        ('资料参考/crystal/Client/MirObjects/MapCode.cs', [226, 259], 'v0列优先原格与cell+6门、cell+8动画字段'),
        ('资料参考/crystal/Client/MirScenes/GameScene.cs', [10658, 10759], '来源地板奇偶与48×32/96×64平面层分类'),
        ('资料参考/crystal/Client/MirScenes/GameScene.cs', [10810, 10945], '逐行前景后实体、前景底边锚点及火把偏移特例'),
        ('资料参考/crystal/Client/MirScenes/GameScene.cs', [1188, 1193], '地图动画100ms基础时钟'),
        ('资料参考/crystal/Client/MirGraphics/MLibrary.cs', [640, 709], '普通Draw忽略WIL偏移，DrawBlend按显式参数应用偏移'),
    ]
    return [{'path': path, 'sha256': wil.sha256_file(ROOT / path), 'lines': lines, 'role': role}
            for path, lines, role in records]


def build(definition, collision, source_path: Path, client_root: Path, output: Path, check=False):
    source_bytes = source_path.read_bytes()
    source_hash = digest(source_bytes)
    source = SourceMap(source_bytes)
    geometry = validate_definition(definition, collision, source, source_hash)
    if source_path.name.casefold() != (geometry['source_map_code'] + '.map').casefold():
        raise ValueError('指定来源文件名与唯一Excel来源地图号不同')
    artifacts = Artifacts(output, check)
    frames = Frames(client_root, artifacts)
    minimap = None
    minimap_source = None
    if 'minimap' in geometry:
        selected = geometry['minimap']
        minimap_source = minimap_source_record(source_path, geometry['source_map_code'], selected)
        minimap = {**selected, 'frame': frames.frame(selected['package'], selected['raw_frame'])}
        if frames.frames[minimap['frame']]['declared_empty']:
            raise ValueError('唯一Excel选择的小地图原帧为空，不能伪造替代图')
    bitmap = collision_data(source)
    chunks, spatial, bounds, totals = build_chunks(source, geometry, frames, artifacts)
    frames.verify_frozen()
    if wil.sha256_file(source_path) != source_hash:
        raise ValueError('转换期间来源原图改变')
    if minimap_source and wil.sha256_file(Path(minimap_source['path'])) != minimap_source['sha256']:
        raise ValueError('转换期间来源小地图映射改变')
    manifest = {
        'schema_version': 1, 'id': str(definition['id']), 'name': definition['display_name'],
        'record_status': 'candidate', 'development_only': True, 'runtime_ready': False,
        'official_180_map_acceptance': False,
        'width': source.width, 'height': source.height,
        **geometry, 'pixel_width': source.width * 48, 'pixel_height': source.height * 32,
        'source': {'path': str(source_path), 'sha256': source_hash, 'header_bytes': 52,
                   'cell_bytes': 12, 'cell_order': 'column_major', 'full_source_map': True},
        'frame_sources': [frames.sources[name] for name in sorted(frames.sources)],
        'source_frozen_before_after': True,
        'collision': bitmap,
        'draw_contract': {
            'coordinates': 'full_source_cells_and_pixels_y_down',
            'source_row_policy': 'render_all_source_rows_including_y0',
            'floor_fields': FLOOR_FIELDS, 'object_fields': OBJECT_FIELDS,
            'layers': {'0': 'back_floor', '1': 'middle_floor', '2': 'front_floor',
                       '3': 'middle_upright', '4': 'front_upright'},
            'floor_anchor': 'top_left=(x*48,y*32)',
            'object_anchor': 'top_left=(x*48,(y+1)*32-frame.height)',
            'offset_modes': {'0': 'ignore_wil_offset', '1': 'add_wil_offset_after_bottom_alignment'},
            'floor_order': ['y', 'x', 'layer'],
            'world_order': '每个来源行先按x/layer画全部立体物件，再画该行实体；下一行随后绘制',
            'blend_modes': {'0': 'normal', '1': 'additive'},
            'animation_clock_ms': 100,
            'animation_frame_formula': 'floor((floor(now_ms/100)%(count*(1+tick)))/(1+tick))',
            'animation_frame_key': 'package:(base_raw_frame+animation_frame)',
            'animation_scope': '已核验的Objects:2723..2732十帧加色火把；WIL偏移(-51,-113)',
            'chunk_cells_encoding': 'base64_row_major_12_byte_original_cells_little_endian',
            'spatial_scope': '每个绘制实例全部动画帧精确像素包围盒到owner chunk，半开区间；负bucket合法',
        },
        'door_contract': {
            'policy': 'closed_development_candidate', 'field_byte_offset': 6, 'id_mask': 127,
            'closed_image': 'base_raw_frame', 'offset_retained_only': True,
            'opening_implemented': False, 'interior_transfer_implemented': False,
            'original_server_behavior_accepted': False,
            'cross_source_difference': 'Crystal服务端v0门解析检查cell+8，与客户端cell+6不同；本作明确采用客户端字段并保持关闭候选',
        },
        'chunks': chunks, 'spatial_index': spatial, 'render_bounds': bounds,
        'frames': {key: frames.frames[key] for key in sorted(frames.frames)},
        'statistics': {**totals, 'frames': len(frames.frames),
                       'pngs': sum(not frame['declared_empty'] for frame in frames.frames.values()),
                       'decoded_rgba_bytes': sum(frame['width'] * frame['height'] * 4 for frame in frames.frames.values())},
        'evidence': source_evidence(),
        'tool_sha256': wil.sha256_file(Path(__file__)),
        'decoder_sha256': wil.sha256_file(ROOT / 'tools/resources/inspect_wil.py'),
        'pillow_version': PILLOW_VERSION,
        'limitations': [
            '完整MYF服务端0.map统一提供渲染格及碰撞；原客户端地图版本不混入运行原格',
            '关闭门是明确开发候选；门开关、室内传送及原服完整行为尚未实现或验收',
            '未恢复来源地图动态灯光、天气、音效或NPC业务；原始light字段保留在chunk原格中',
            '本作完整原图保留并绘制第0行；未套用Crystal参考渲染y<=0跳过边界行的限制',
            '图像/结构与来源摘要复验不等于Creator构建、真机、业务、容量或目标1.80完整认证',
        ],
    }
    if minimap:
        manifest['minimap'] = minimap
        manifest['minimap_source'] = minimap_source
        manifest['limitations'].append('小地图采用唯一Excel显式选择原帧；MYF映射编号减一仍属跨来源候选，未认证来源引擎编号规则')
    artifacts.write_json('manifest.json', manifest)
    artifacts.verify_inventory()
    return manifest


def output_path(map_id: int, requested: Path | None):
    default = ROOT / f'client/assets/resources/source-maps/map-{map_id}'
    candidate = requested.absolute() if requested else default
    if requested and not any(candidate.resolve().is_relative_to(ROOT / name) for name in ('.work', 'outputs')):
        raise ValueError('自定义输出仅允许本项目.work或outputs内的独立目录')
    cursor = candidate
    while cursor != ROOT.parent:
        if cursor.is_symlink():
            raise ValueError('输出目录或上级不能是符号链接')
        if cursor.parent == cursor:
            raise ValueError('输出目录不在项目范围')
        cursor = cursor.parent
    return candidate.resolve()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--map-id', required=True, type=int)
    parser.add_argument('--source-map', required=True, type=Path, help='只读MYF服务端完整原图；须匹配唯一Excel source_hash')
    parser.add_argument('--source-root', required=True, type=Path, help='只读原客户端根目录；只读取Data内WIL/WIX')
    parser.add_argument('--output', type=Path, help='独立再生成目录，仅允许项目内.work或outputs')
    parser.add_argument('--check', action='store_true', help='只读复验现有全部PNG、元数据和来源摘要')
    args = parser.parse_args()
    if args.map_id <= 0:
        parser.error('map-id须为正整数')
    definition, collision = workbook_definition(args.map_id)
    target = output_path(args.map_id, args.output)
    manifest = build(definition, collision, args.source_map.resolve(), args.source_root.resolve(), target, args.check)
    print(json.dumps({'status': 'checked' if args.check else 'generated', 'map_id': args.map_id,
                      'output': str(target), 'source_sha256': manifest['source']['sha256'],
                      'statistics': manifest['statistics'],
                      'collision': {key: manifest['collision'][key] for key in ('static_count', 'closed_door_count', 'runtime_count')},
                      'manifest_sha256': wil.sha256_file(target / 'manifest.json'), 'runtime_ready': False}, ensure_ascii=False))


if __name__ == '__main__':
    try:
        main()
    except (OSError, ValueError, KeyError, struct.error, wil.UnsupportedFormat) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(2)
