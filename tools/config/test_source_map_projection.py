"""来源整图目录仅接受唯一Excel完整合同，客户端只暴露必要地图观察。"""
import base64
import copy
import importlib.util
import hashlib
from pathlib import Path
import unittest
from unittest.mock import patch

import export_configs as config

ROOT=Path(__file__).resolve().parents[2]
SPEC=importlib.util.spec_from_file_location('prepare_source_map_projection',ROOT/'tools/resources/prepare_first_loop.py')
module=importlib.util.module_from_spec(SPEC);SPEC.loader.exec_module(module)


class SourceMapProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tables,errors,_,_=config.collect(config.load_json(ROOT/'config/schema/catalog.json'))
        assert not errors,errors

    def test_full_town_coordinates_and_runtime_bits_come_from_locked_asset(self):
        before=copy.deepcopy(self.tables)
        fixture=module.fixture_from_rows(self.tables);town=fixture['maps'][0]
        self.assertEqual((town['id'],town['width'],town['height'],town['player_spawn']),(2,700,700,{'x':633,'y':613}))
        self.assertEqual(town['source']['code'],'0')
        bits=base64.b64decode(town['collision_bits'],validate=True)
        self.assertEqual((len(bits),sum(byte.bit_count() for byte in bits)),(61250,162741))
        self.assertEqual([zone['id'] for zone in town['safe_zones']],[106,107,108,113,117])
        self.assertEqual(fixture['binding'],{'enabled':False,'start_points':[],'safe_zone_enabled':True})
        self.assertTrue(any(skill['id']==21 and skill['behavior']=='teleport' for skill in fixture['skills']))
        self.assertEqual(self.tables,before)

    def test_catalog_cannot_alias_personal_map_or_repeat_ids(self):
        for ids in ([1],[2,2],[False]):
            tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['map_catalog_ids']=ids
            with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_missing_runtime_budgets_are_not_filled_by_code(self):
        for field in ['load_concurrency','cache_capacity']:
            tables=copy.deepcopy(self.tables);del tables['dev_first_loop_profile'][0]['parameters']['source_map_streaming'][field]
            with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_native_geometry_is_complete_typed_and_does_not_inherit_old_minimap(self):
        for field in ['projection','tile_width','tile_height','soil_thickness_px','source_origin','chunk_size_cells','source_map_code','minimap']:
            with self.subTest(missing=field):
                tables=copy.deepcopy(self.tables);geometry=next(row for row in tables['game_maps'] if row['id']==2)['visual_geometry']
                del geometry[field]
                with self.assertRaises(ValueError):module.fixture_from_rows(tables)
        for field in ['tile_width','tile_height','soil_thickness_px','chunk_size_cells']:
            with self.subTest(fractional_type=field):
                tables=copy.deepcopy(self.tables);geometry=next(row for row in tables['game_maps'] if row['id']==2)['visual_geometry']
                geometry[field]=float(geometry[field])
                with self.assertRaises(ValueError):module.fixture_from_rows(tables)
        tables=copy.deepcopy(self.tables);next(row for row in tables['game_maps'] if row['id']==2)['visual_geometry']['unused_default']=0
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_inconsistent_region_and_unapproved_binding_are_rejected(self):
        tables=copy.deepcopy(self.tables);tables['dev_first_loop_profile'][0]['parameters']['binding']['enabled']=True
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_safe_zone_source_rows_are_complete_exact_and_keep_duplicates(self):
        tables=copy.deepcopy(self.tables);tables['game_map_regions']=[row for row in tables['game_map_regions'] if row['id']!=107]
        with self.assertRaisesRegex(ValueError,'包括重复行'):module.fixture_from_rows(tables)
        tables=copy.deepcopy(self.tables);next(row for row in tables['game_map_regions'] if row['id']==106)['entry_rules']['source_order']=6.5
        with self.assertRaisesRegex(ValueError,'正整数'):module.fixture_from_rows(tables)
        tables=copy.deepcopy(self.tables);row=next(row for row in tables['game_map_regions'] if row['id']==106)
        row['entry_rules']['center']['x']+=1;row['region_cells']['min_x']+=1;row['region_cells']['max_x']+=1
        with self.assertRaisesRegex(ValueError,'包括重复行'):module.fixture_from_rows(tables)

    def test_source_collision_dimensions_and_region_walkability_are_checked_before_export(self):
        tables=copy.deepcopy(self.tables);next(row for row in tables['game_map_collisions'] if row['id']==2)['width_cells']=17
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)
        tables=copy.deepcopy(self.tables);row=next(row for row in tables['game_map_regions'] if row['id']==106)
        row['entry_rules']['center']['x']=699;row['region_cells'].update(min_x=691,max_x=707)
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)
        tables=copy.deepcopy(self.tables);next(row for row in tables['game_maps'] if row['id']==2)['spawn_cell']={'x':0,'y':0}
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_equal_length_collision_tampering_is_rejected_even_with_rewritten_digest(self):
        manifest_path=ROOT/'client/assets/resources/source-maps/map-2/manifest.json'
        damaged=module.read(manifest_path);bitmap=bytearray(base64.b64decode(damaged['collision']['runtime_bits']));bitmap[0]^=1
        damaged['collision']['runtime_bits']=base64.b64encode(bitmap).decode('ascii')
        damaged['collision']['runtime_sha256']=hashlib.sha256(bitmap).hexdigest()
        original=module.read
        with patch.object(module,'read',side_effect=lambda path:damaged if path.resolve()==manifest_path.resolve() else original(path)):
            with self.assertRaisesRegex(ValueError,'已锁原图'):module.fixture_from_rows(copy.deepcopy(self.tables))
        tables=copy.deepcopy(self.tables);next(row for row in tables['game_map_regions'] if row['id']==106)['region_cells']['min_x']+=1
        with self.assertRaises(ValueError):module.fixture_from_rows(tables)

    def test_public_map_catalog_preserves_id_text_and_hides_raw_source_data(self):
        public=module.public_gameplay(module.fixture_from_rows(self.tables))
        self.assertEqual([row['id'] for row in public['map_catalog']],['1','2'])
        self.assertEqual(public['map_catalog'][1]['resource_key'],'source-maps/map-2/manifest')
        self.assertEqual(public['source_map_streaming'],{'load_concurrency':4,'cache_capacity':49})
        for row in public['map_catalog']:
            self.assertEqual(set(row),{'id','name','resource_key','width','height','scene_kind','safe_zones','no_teleport'})
            for key in ['source_ref','source','collision_bits','source_sha256','manifest_sha256','initial_binding_pool_verified']:
                self.assertNotIn(key,row)


if __name__=='__main__':unittest.main()
