"""来源地图生成的独立小样本验收，不写或重建既有场景布局。"""
import base64
import hashlib
import json
from pathlib import Path
import struct
import tempfile
import unittest

import build_source_maps as maps


def source_map(width, height, cells=None):
    header = bytearray(52)
    struct.pack_into('<HH', header, 0, width, height)
    header[4:18] = b'\rLegend of mir'
    cells = cells or {}
    return maps.SourceMap(bytes(header) + b''.join(struct.pack('<HHHBBBBBB', *cells.get((x, y), (0,) * 9))
                                                  for x in range(width) for y in range(height)))


def frame(package, index, width, height, offset_x=0, offset_y=0):
    return {'package': package, 'raw_frame': index, 'width': width, 'height': height,
            'offset_x': offset_x, 'offset_y': offset_y, 'declared_empty': not width or not height}


class FixtureFrames:
    def __init__(self, definitions):
        self.frames = definitions

    def frame(self, name, index):
        key = f'{name}:{index}'
        if key not in self.frames:
            raise ValueError('样本未提供帧：' + key)
        return key


def write_one_frame_wil(directory, name):
    directory.mkdir(parents=True, exist_ok=True)
    header = bytearray(60)
    signature = b'#ILIB v1.0-WEMADE Entertainment inc.'
    header[:len(signature)] = signature
    struct.pack_into('<IIII', header, 44, 1, 256, 1024, 0)
    palette = bytearray(1020)
    struct.pack_into('<I', palette, 0, 0x030201)
    library = bytes(header) + bytes(palette) + struct.pack('<hhhh', 1, 1, 7, 9) + bytes([1])
    index_header = bytearray(48)
    index_signature = b'#INDX v1.0-WEMADE Entertainment inc.'
    index_header[:len(index_signature)] = index_signature
    struct.pack_into('<I', index_header, 44, 1)
    index = bytes(index_header) + struct.pack('<I', 1080)
    (directory / f'{name}.wil').write_bytes(library)
    (directory / f'{name}.wix').write_bytes(index)
    return library, index


class SourceMapTests(unittest.TestCase):
    def test_original_column_major_and_geometry_validation(self):
        source = source_map(2, 3, {(1, 0): (2, 0, 0, 0, 0, 0, 0, 0, 0)})
        self.assertEqual(source.cell(1, 0).back, 2)
        self.assertEqual(source.cell(0, 1).back, 0)
        with self.assertRaises(ValueError):
            source.raw(2, 0)
        with self.assertRaises(ValueError):
            maps.SourceMap(source.data[:-1])
        geometry = {'projection': 'source_rectangular', 'tile_width': 48, 'tile_height': 32,
                    'soil_thickness_px': 0, 'source_origin': {'x': 0, 'y': 0},
                    'chunk_size_cells': 1, 'source_map_code': '0'}
        definition = {'width_cells': 2, 'height_cells': 3, 'collision_id': 7, 'visual_geometry': geometry}
        collision = {'id': 7, 'width_cells': 2, 'height_cells': 3, 'source_hash': 'fixture',
                     'walkable_encoding': 'source_map_v0_closed_doors',
                     'blocked_cells': {'source_file': 'myf:Mir200/Map/0.map', 'door_policy': 'closed'}}
        self.assertEqual(maps.validate_definition(definition, collision, source, 'fixture'), geometry)
        with self.assertRaises(ValueError):
            maps.validate_definition(definition, collision, source, 'wrong')
        geometry.pop('chunk_size_cells')
        with self.assertRaises(ValueError):
            maps.validate_definition(definition, collision, source, 'fixture')

    def test_row_major_low_bit_first_collision_separates_original_and_closed_doors(self):
        source = source_map(3, 3, {
            (2, 0): (0x8000, 0, 0, 0, 0, 0, 0, 0, 0),
            (0, 1): (0, 0x8000, 0, 0, 0, 0, 0, 0, 0),
            (2, 2): (0, 0, 0, 0x81, 0, 0, 0, 0, 0),
        })
        result = maps.collision_data(source)
        self.assertEqual(base64.b64decode(result['static_bits']), bytes([0b00001100, 0]))
        self.assertEqual(base64.b64decode(result['closed_door_bits']), bytes([0, 1]))
        self.assertEqual(base64.b64decode(result['runtime_bits']), bytes([0b00001100, 1]))
        self.assertEqual((result['static_count'], result['closed_door_count'], result['runtime_count']), (2, 1, 3))

    def test_layers_preserve_absolute_parity_closed_door_and_native_anchors(self):
        frames = FixtureFrames({
            'Tiles:0': frame('Tiles', 0, 96, 64, 900, 900),
            'SmTiles:0': frame('SmTiles', 0, 48, 32, 800, 800),
            'Objects:0': frame('Objects', 0, 48, 96, 700, 700),
        })
        cell = maps.Cell(1, 1, 1, 0x83, 9, 0, 0, 0, 0)
        floor, objects = maps.cell_draws(2, 4, cell, frames)
        self.assertEqual([record[2] for record in floor], [0, 1])
        self.assertEqual(objects, [[2, 4, 4, 'Objects:0', 0, 0, 0, 0, 3, 9]])
        self.assertEqual(maps.pixel_rect(2, 4, 0, frames.frames['Tiles:0']), [96, 128, 192, 192])
        self.assertEqual(maps.object_rect(objects[0], frames), [96, 64, 144, 160])
        self.assertEqual([record[2] for record in maps.cell_draws(3, 4, cell, frames)[0]], [1])

    def test_flat_front_draws_on_floor_and_unsupported_library_fails(self):
        frames = FixtureFrames({'Objects:0': frame('Objects', 0, 48, 32)})
        floor, objects = maps.cell_draws(0, 0, maps.Cell(0, 0, 1, 0, 0, 0, 0, 0, 0), frames)
        self.assertEqual(floor, [[0, 0, 2, 'Objects:0']])
        self.assertEqual(objects, [])
        with self.assertRaises(ValueError):
            maps.cell_draws(0, 0, maps.Cell(0, 0, 1, 0, 0, 0, 0, 1, 0), frames)

    def test_torch_uses_verified_wil_offset_and_animation_union(self):
        frames = FixtureFrames({f'Objects:{i}': frame('Objects', i, 100, 100, -51, -113) for i in range(2723, 2733)})
        floor, objects = maps.cell_draws(10, 10, maps.Cell(0, 0, 2724, 0, 0, 0x8a, 0, 0, 0), frames)
        self.assertEqual(floor, [])
        self.assertEqual(objects[0][4:8], [10, 0, 1, 1])
        self.assertEqual(maps.object_rect(objects[0], frames), [429, 139, 529, 239])
        frames.frames['Objects:2732']['height'] = 300
        self.assertEqual(maps.object_rect(objects[0], frames), [429, -61, 529, 239])
        with self.assertRaises(ValueError):
            maps.cell_draws(10, 10, maps.Cell(0, 0, 2724, 0, 0, 0x8a, 1, 0, 0), frames)

    def test_spatial_index_finds_tall_slice_from_another_owner_chunk(self):
        source = source_map(3, 4, {(2, 3): (0, 0, 1, 0, 0, 0, 0, 0, 0)})
        frames = FixtureFrames({'Objects:0': frame('Objects', 0, 48, 128)})
        with tempfile.TemporaryDirectory() as directory:
            artifacts = maps.Artifacts(Path(directory))
            chunks, spatial, bounds, totals = maps.build_chunks(source, {'chunk_size_cells': 2}, frames, artifacts)
            self.assertEqual(len(chunks), 4)
            self.assertEqual(bounds, [96, 0, 144, 128])
            self.assertEqual(spatial['buckets']['1,0'], ['1_1'])
            self.assertEqual(spatial['buckets']['1,1'], ['1_1'])
            chunk = json.loads((Path(directory) / 'chunks/chunk_1_1.json').read_bytes())
            raw = base64.b64decode(chunk['cells_base64'])
            self.assertEqual(raw, source.raw(2, 2) + source.raw(2, 3))
            self.assertEqual(totals['object_draws'], 1)
        self.assertEqual(maps.bucket_keys([-1, -1, 48, 32], 48, 32), [(-1, -1), (0, -1), (-1, 0), (0, 0)])

    def test_artifacts_preserve_png_bytes_and_check_without_writes(self):
        rgba = bytes([7, 8, 9, 255])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            artifacts = maps.Artifacts(root)
            expected = artifacts.write_png('frames/f.png', 1, 1, rgba)
            path = root / 'frames/f.png'
            before = path.read_bytes()
            self.assertEqual(artifacts.write_png('frames/f.png', 1, 1, rgba), expected)
            self.assertEqual(path.read_bytes(), before)
            artifacts.write_json('manifest.json', {'f': expected})
            checked = maps.Artifacts(root, check=True)
            checked.write_png('frames/f.png', 1, 1, rgba)
            checked.write_json('manifest.json', {'f': expected})
            checked.verify_inventory()
            with self.assertRaises(ValueError):
                checked.write_json('manifest.json', {'f': 'tampered'})
            self.assertEqual(hashlib.sha256(path.read_bytes()).hexdigest(), expected)
            with self.assertRaises(ValueError):
                artifacts.path('../outside.json')

    def test_complete_fixture_import_and_readonly_check_preserve_sources(self):
        source = source_map(2, 2, {(0, 0): (0, 0, 1, 0, 0, 0, 0, 0, 0)})
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            client = root / 'client'
            data = client / 'Data'
            library, index = write_one_frame_wil(data, 'Objects')
            path = root / '0.map'
            path.write_bytes(source.data)
            geometry = {'projection': 'source_rectangular', 'tile_width': 48, 'tile_height': 32,
                        'soil_thickness_px': 0, 'source_origin': {'x': 0, 'y': 0},
                        'chunk_size_cells': 1, 'source_map_code': '0'}
            definition = {'id': 2, 'display_name': '样本', 'width_cells': 2, 'height_cells': 2,
                          'collision_id': 2, 'visual_geometry': geometry}
            collision = {'id': 2, 'width_cells': 2, 'height_cells': 2, 'source_hash': maps.digest(source.data),
                         'walkable_encoding': 'source_map_v0_closed_doors',
                         'blocked_cells': {'source_file': 'myf:Mir200/Map/0.map', 'door_policy': 'closed'}}
            output = root / 'generated'
            generated = maps.build(definition, collision, path, client, output)
            before = {p.relative_to(output).as_posix(): p.read_bytes() for p in output.rglob('*') if p.is_file()}
            checked = maps.build(definition, collision, path, client, output, check=True)
            after = {p.relative_to(output).as_posix(): p.read_bytes() for p in output.rglob('*') if p.is_file()}
            self.assertEqual(before, after)
            self.assertEqual(generated, checked)
            self.assertEqual(generated['id'], '2')
            self.assertTrue(generated['source_frozen_before_after'])
            self.assertEqual(generated['statistics']['pngs'], 1)
            self.assertEqual(path.read_bytes(), source.data)
            self.assertEqual((data / 'Objects.wil').read_bytes(), library)
            self.assertEqual((data / 'Objects.wix').read_bytes(), index)
            with (output / 'frames/Objects/frame_000000.png').open('rb') as stream:
                with maps.Image.open(stream) as image:
                    self.assertEqual(image.tobytes(), bytes([3, 2, 1, 255]))
            frozen = maps.Frames(client, maps.Artifacts(root / 'frozen'))
            frozen.frame('Objects', 0)
            (data / 'Objects.wil').write_bytes(library[:-1] + bytes([2]))
            with self.assertRaisesRegex(ValueError, '转换期间来源WIL/WIX改变'):
                frozen.verify_frozen()

    def test_optional_minimap_requires_explicit_frame_and_records_mapping_without_default(self):
        source = source_map(1, 1)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            path = root / 'Mir200/Map/0.map'
            path.parent.mkdir(parents=True)
            path.write_bytes(source.data)
            mapping = root / 'Mir200/Envir/MiniMap.txt'
            mapping.parent.mkdir(parents=True)
            mapping_bytes = ';样本小地图映射\n0 1\n'.encode('gb18030')
            mapping.write_bytes(mapping_bytes)
            client = root / 'client'
            write_one_frame_wil(client / 'Data', 'mmap')
            geometry = {'projection': 'source_rectangular', 'tile_width': 48, 'tile_height': 32,
                        'soil_thickness_px': 0, 'source_origin': {'x': 0, 'y': 0},
                        'chunk_size_cells': 1, 'source_map_code': '0'}
            definition = {'id': 2, 'display_name': '样本', 'width_cells': 1, 'height_cells': 1,
                          'collision_id': 2, 'visual_geometry': geometry}
            collision = {'id': 2, 'width_cells': 1, 'height_cells': 1, 'source_hash': maps.digest(source.data),
                         'walkable_encoding': 'source_map_v0_closed_doors',
                         'blocked_cells': {'source_file': 'myf:Mir200/Map/0.map', 'door_policy': 'closed'}}
            absent = maps.build(definition, collision, path, client, root / 'absent')
            self.assertNotIn('minimap', absent)
            self.assertNotIn('minimap_source', absent)
            self.assertEqual(absent['frames'], {})
            for bad in [None, {}, {'package': 'mmap'}, {'package': 'mmap', 'raw_frame': False},
                        {'package': 'mmap', 'raw_frame': -1}, {'package': '../mmap', 'raw_frame': 0},
                        {'package': 'mmap', 'raw_frame': 0, 'default': True}]:
                geometry['minimap'] = bad
                with self.assertRaises(ValueError):
                    maps.validate_definition(definition, collision, source, maps.digest(source.data))
            geometry['minimap'] = {'package': 'mmap', 'raw_frame': 0}
            present = maps.build(definition, collision, path, client, root / 'present')
            self.assertEqual(present['minimap'], {'package': 'mmap', 'raw_frame': 0, 'frame': 'mmap:0'})
            self.assertEqual(present['statistics']['pngs'], 1)
            self.assertEqual(present['minimap_source']['source_line'], 2)
            self.assertEqual(present['minimap_source']['mapped_image_number'], 1)
            self.assertEqual(present['minimap_source']['sha256'], maps.digest(mapping_bytes))
            self.assertFalse(present['minimap_source']['source_engine_index_base_verified'])
            self.assertEqual(present, maps.build(definition, collision, path, client, root / 'present', check=True))
            self.assertEqual(mapping.read_bytes(), mapping_bytes)


if __name__ == '__main__':
    unittest.main()
