"""核对真实工作簿数值和独立矿区投影不会误入默认发布。"""

import copy
import unittest

import prepare_mining as mining


class TestPrepareMining(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema = mining.config.load_json(mining.ROOT / 'config/schema/catalog.json')
        cls.tables, errors, _, _ = mining.config.collect(schema)
        if errors:
            raise AssertionError(errors)
        cls.server = mining.config.load_json(mining.BASE_FIXTURE)
        cls.client = mining.config.load_json(mining.BASE_PUBLIC)
        cls.assembly = mining.config.load_json(mining.ROOT / 'tools/resources/testdata/mining-isolated.json')

    def test_exact_workbook_numbers_and_isolated_public_projection(self):
        server, client = mining.project(self.server, self.client, self.tables, self.assembly)
        rules = server['mining']
        self.assertEqual((rules['interval_ms'], rules['success_ppm'], rules['tool_wear_u']), (3000, 300000, 1))
        self.assertEqual([r['minimum_level'] for r in rules['layers']], [20, 36])
        self.assertEqual([r['layer_weights'] for r in rules['ores']], [[58, 34], [30, 40], [10, 20], [2, 6]])
        self.assertEqual([(r['min'], r['max'], r['weight_ppm']) for r in rules['purity']],
                         [(1, 20, 450000), (21, 40, 300000), (41, 60, 150000), (61, 80, 70000), (81, 100, 30000)])
        self.assertEqual([(m['id'], m['scene_kind'], m['floor_style_map_id']) for m in client['map_catalog'][-2:]],
                         [('202601', 7, '7'), ('202602', 7, '7')])
        self.assertEqual([item['kind'] for item in client['items'][-6:]], ['mining_ore'] * 4 + ['mining_tool'] * 2)
        old_shop = next(s for s in self.server['npc_shops'] if s['key'] == 'grocery')
        experimental = next(s for s in server['npc_shops'] if s['key'] == 'grocery')
        self.assertEqual(experimental['sell_prices'], old_shop['sell_prices'])
        self.assertEqual([(o['item_id'], o['price_units']) for o in experimental['offers'][-2:]], [(910011, 50), (910012, 80)])
        self.assertEqual([(t['item_id'], t['durability_u']) for t in rules['tools']], [(910011, 12), (910012, 16)])
        self.assertEqual(client['map_catalog'][:-2], self.client['map_catalog'])
        self.assertEqual(server['items'][:-6], self.server['items'])
        self.assertFalse(server['_source_evidence']['mining']['default_enabled'])
        self.assertNotIn('mining', self.server)

    def test_respects_explicit_geometry_and_item_mappings(self):
        for location, key, invalid in ((('map',), 'width', 14), (('map',), 'id', 1), (('map',), 'resource_key', '../../maps'),
                                         ((), 'floor_style_map_id', 3), (('tool',), 'durability_u', True),
                                         (('tool',), 'item_id', 910001), (('tool',), 'shop_key', 'none')):
            with self.subTest(key=key, invalid=invalid):
                assembly = copy.deepcopy(self.assembly)
                row = assembly['layers'][0]
                for part in location:
                    row = row[part]
                row[key] = invalid
                with self.assertRaises(ValueError):
                    mining.project(self.server, self.client, self.tables, assembly)

    def test_refuses_to_override_real_workbook_or_enable_extra_policies(self):
        tables = copy.deepcopy(self.tables)
        tables[mining.TABLE_LAYERS][0]['map_config_id'] = 123
        with self.assertRaises(ValueError):
            mining.project(self.server, self.client, tables, self.assembly)
        tables = copy.deepcopy(self.tables)
        tables[mining.TABLE_PURITY][0]['purity_max'] += 1
        with self.assertRaises(ValueError):
            mining.project(self.server, self.client, tables, self.assembly)
        with self.assertRaises(ValueError):
            mining.project(self.server, self.client, self.tables, {**self.assembly, 'ore_recycle_rounding': 'floor_per_ore'})


if __name__ == '__main__':
    unittest.main()
