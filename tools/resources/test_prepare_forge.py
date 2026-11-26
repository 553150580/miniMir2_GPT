"""核对隔离打造的工作簿来源与原目录保真。"""

from copy import deepcopy
import unittest

import prepare_forge as forge
import prepare_mining as mining


class ForgeProjectionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cfg = mining.config
        cls.tables, errors, _, _ = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
        if errors:
            raise RuntimeError(errors)
        directory = mining.ROOT / 'tools/resources/testdata'
        cls.assembly = cfg.load_json(directory / 'forge-isolated.json')
        cls.base_server = cfg.load_json(mining.BASE_FIXTURE)
        cls.base_public = cfg.load_json(mining.BASE_PUBLIC)
        cls.server, cls.public = mining.project(cls.base_server, cls.base_public, cls.tables, cfg.load_json(directory / 'mining-isolated.json'))

    def test_preserves_original_catalogs_and_adds_real_boss_attachment(self):
        original = deepcopy((self.server, self.public))
        server, public = forge.project(self.server, self.public, self.tables, self.assembly)
        self.assertEqual((self.server, self.public), original)
        self.assertEqual(server['mining'], self.server['mining'])
        self.assertEqual(server['personal_boss'], self.server['personal_boss'])
        self.assertEqual(server['items'][:-2], self.server['items'])
        self.assertEqual(public['items'][:-2], self.public['items'])
        self.assertNotIn('forge', self.base_server)
        self.assertNotIn('forge_recipes', self.base_public)
        self.assertEqual(server['forge'], {'ore_item_id': 910004, 'guaranteed_purity': 95, 'luck_ppm': 150000,
                                           'luck_weights': [1, 1, 1], 'recipes': [
                                               {'blueprint_item_id': 910021, 'output_item_id': 910022, 'ore_count': 4, 'quality': 2, 'gold': 30000}]})
        self.assertEqual(server['items'][-2]['id'], 910021)
        self.assertEqual(public['items'][-2]['kind'], 'forge_blueprint')
        self.assertEqual(public['items'][-1]['equip_slot'], 3)
        self.assertEqual((public['items'][-1]['required_level'], public['items'][-1]['quality'], public['items'][-1]['equipment_stage']), (20, 2, 1))
        self.assertEqual(public['forge_recipes'], [{'blueprint_item_id': 910021, 'output_item_id': 910022, 'ore_item_id': 910004,
                                                    'ore_count': 4, 'quality': 2}])
        original_boss = next(row for row in self.server['personal_boss_variants'] if row['template_id'] == 3)
        altered_boss = next(row for row in server['personal_boss_variants'] if row['template_id'] == 3)
        self.assertEqual({key: value for key, value in altered_boss.items() if key != 'item_reward'}, original_boss)
        self.assertEqual(altered_boss['item_reward'], {'pending_limit': 2, 'items': [{'item_id': 910021, 'quantity': 1}]})
        proof = server['_source_evidence']['forge']
        self.assertEqual(proof['ore_count_status'], 'historical')
        self.assertEqual(proof['recipe_table_status'], 'no_rows')
        self.assertFalse(proof['default_enabled'])

    def test_price_and_luck_are_recompiled_from_catalog(self):
        tables = deepcopy(self.tables)
        next(row for row in tables['game_level_band_economy'] if row['level_min'] == 20)['rare_craft_gold'] = 30700
        luck = next(row for row in tables['game_luck_initial'] if row['source_key'] == 'craft_success')
        luck['initial_value_weights']['2'] = 5
        server, _ = forge.project(self.server, self.public, tables, self.assembly)
        self.assertEqual(server['forge']['recipes'][0]['gold'], 30700)
        self.assertEqual(server['forge']['luck_weights'], [1, 5, 1])

    def test_refuses_overrides_missing_source_and_collisions(self):
        for edit in ({'gold': 1}, {'required_level': 11}, {'blueprint_item_id': 910004},
                     {'output_item_id': 261}, {'personal_boss_template_id': 500},
                     {'output_template_item_id': 910004}, {'ore_count': 9}):
            with self.subTest(edit=edit), self.assertRaises(ValueError):
                forge.project(self.server, self.public, self.tables, {**self.assembly, **edit})
        server = deepcopy(self.server)
        next(row for row in server['personal_boss_variants'] if row['template_id'] == 3)['item_reward'] = {'items': []}
        with self.assertRaises(ValueError):
            forge.project(server, self.public, self.tables, self.assembly)
        tables = deepcopy(self.tables)
        tables['game_craft_recipes'].append({'id': 1})
        with self.assertRaises(ValueError):
            forge.project(self.server, self.public, tables, self.assembly)


if __name__ == '__main__':
    unittest.main()
