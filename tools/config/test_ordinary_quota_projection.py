"""验证普通装备池与唯一工作簿额度相互引用。"""

from copy import deepcopy
import unittest

import export_configs as config
from test_source_map_projection import module


class OrdinaryQuotaProjectionTests(unittest.TestCase):
    def setUp(self):
        self.compiler = module.ordinary_hunt
        self.items = [
            {'id': item, 'is_equipment': True, 'equipment_stage': stage, 'drop_quality': quality}
            for item, stage, quality in [(1, 1, 0), (2, 1, 2), (3, 1, 3), (4, 2, 0)]
        ]
        self.pools = [{'id': 11, 'equipment': [{'item_id': item} for item in (1, 2, 3, 4)]}]
        self.rare = {'id': 1, 'source_kind': 'ordinary_monster', 'record_status': 'candidate', 'stage': 1,
                     'quality': 2, 'quota_units': 3, 'window_seconds': 7200,
                     'refill_schedule': [{'offset_seconds': 0, 'units': 1}, {'offset_seconds': 19, 'units': 2}],
                     'exhausted_downgrade_weights': {'0': 4}}
        self.epic = dict(self.rare, id=2, quality=3, exhausted_downgrade_weights={'0': 2, '2': 3})

    def test_exports_only_high_quality_limits_used_by_ordinary_pools(self):
        world = dict(self.rare, id=3, source_kind='world_boss')
        unused = dict(self.rare, id=4, stage=2)
        result = self.compiler.compile_quotas({'game_drop_quotas': [world, unused, self.epic, self.rare]}, self.pools, self.items)
        self.assertEqual([row['id'] for row in result], [1, 2])
        self.assertEqual(result[0]['refill_schedule'], self.rare['refill_schedule'])
        self.assertEqual(result[1]['exhausted_downgrade_weights'], self.epic['exhausted_downgrade_weights'])
        white_pools = [{'id': 11, 'equipment': [{'item_id': 1}]}]
        self.assertEqual(self.compiler.compile_quotas({'game_drop_quotas': [self.rare]}, white_pools, self.items), [])

    def test_rejects_missing_release_or_cross_stage_downgrade(self):
        for invalid in ([], [dict(self.rare, record_status='historical')],
                        [dict(self.rare, refill_schedule=[{'offset_seconds': 0, 'units': 3}])]):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                self.compiler.compile_quotas({'game_drop_quotas': [self.epic, *invalid]}, self.pools, self.items)
        later_stage_only = deepcopy(self.pools)
        later_stage_only[0]['equipment'].pop(0)
        with self.assertRaisesRegex(ValueError, '缺少阶段1'):
            self.compiler.compile_quotas({'game_drop_quotas': [self.rare, self.epic]}, later_stage_only, self.items)

    def test_full_workbook_projection_keeps_quota_server_only(self):
        schema = config.load_json(config.ROOT / 'config/schema/catalog.json')
        tables, errors, _, _ = config.collect(schema)
        self.assertEqual(errors, [])
        baseline = module.fixture_from_rows(tables)
        self.assertNotIn('drop_quotas', baseline)
        # 只在测试副本提升一个同时存在于两个同阶段池的模板，保留可用的普通装备回退。
        for table, key in [('game_equipment', 'item_id'), ('game_items', 'id')]:
            next(row for row in tables[table] if row[key] == 220)['quality'] = 2
        for pool in tables['game_drop_pools']:
            if pool['id'] in (13, 15):
                pool['max_random_quality'] = 2
        tables['game_drop_quotas'] = [dict(self.rare, stage=0, source_ref='仅测试工作簿副本', note='不发布')]
        fixture = module.fixture_from_rows(tables)
        self.assertEqual(fixture['drop_quotas'][0]['stage'], 0)
        self.assertEqual(fixture['drop_quotas'][0]['quality'], 2)
        self.assertEqual(fixture['drop_quotas'][0]['refill_schedule'], self.rare['refill_schedule'])
        public = module.public_gameplay(fixture)
        self.assertNotIn('drop_quotas', public)
        self.assertEqual(next(row for row in public['items'] if row['item_id'] == 220)['quality'], 2)
