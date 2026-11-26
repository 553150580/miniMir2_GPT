"""验证洗炼隔离投影的唯一来源、精确概率及原目录保真。"""

from copy import deepcopy
import unittest

import prepare_equipment_enhancement as enhancement
import prepare_equipment_refine as refine
import prepare_mining as mining
import test_prepare_equipment_enhancement as previous


class EquipmentRefineProjectionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        origin = previous.EquipmentEnhancementProjectionTest
        origin.setUpClass()
        cls.tables, cls.default_server, cls.default_public = origin.tables, origin.default_server, origin.default_public
        cls.server, cls.public = enhancement.project(origin.server, origin.public, origin.tables, origin.assembly)
        cls.assembly = mining.config.load_json(mining.ROOT / 'tools/resources/testdata/equipment-refine-isolated.json')

    def test_original_catalog_and_exact_probability_are_preserved(self):
        originals = deepcopy((self.server, self.public))
        server, public = refine.project(self.server, self.public, self.tables, self.assembly)
        self.assertEqual((self.server, self.public), originals)
        self.assertNotIn(refine.KEY, self.default_server)
        self.assertNotIn(refine.KEY, self.default_public)
        self.assertEqual({key: server[key] for key in self.server if key not in ('items', '_source_evidence')},
                         {key: value for key, value in self.server.items() if key not in ('items', '_source_evidence')})
        self.assertEqual({key: public[key] for key in self.public if key != 'items'},
                         {key: value for key, value in self.public.items() if key != 'items'})
        self.assertEqual(server['items'][:-1], self.server['items'])
        self.assertEqual(public['items'][:-1], self.public['items'])
        self.assertEqual(server['items'][-1]['id'], public['items'][-1]['item_id'])
        self.assertTrue(server['items'][-1]['equipment_refine_stone'])
        self.assertNotIn('equipment_recycle_material', server['items'][-1])
        rules = server[refine.KEY]
        self.assertEqual(rules, public[refine.KEY])
        self.assertEqual(rules['recycle'], {'item_id': 220, 'quality': 1, 'stage': 0, 'source_kind': 'monster_drop',
                                           'probability_ppm': 50_000, 'quantity': 1})
        self.assertEqual([row['id'] for row in rules['affixes'] if row['min_quality'] == 1], [2, 3, 5])
        self.assertEqual([row['id'] for row in rules['affixes'] if row['min_quality'] == 2], [1, 12, 13])
        self.assertEqual([row['gold'] for row in rules['gold_bands']], [1_000, 3_000, 12_000, 64_000, 160_000])
        self.assertEqual([row['weight_ppm'] for row in rules['tiers']], [450_000, 300_000, 150_000, 70_000, 30_000])
        self.assertEqual([row['ingot_u'] for row in rules['lock_costs']], [0, 2_000, 5_000, 9_000])

    def test_rejects_item_alias_and_source_drift(self):
        for change in ({'stone_item_id': 900041}, {'recycle_item_id': 910022},
                       {'recycle_item_id': 218}, {'target_item_ids': [220, 220]}):
            with self.subTest(change=change), self.assertRaises(ValueError):
                refine.project(self.server, self.public, self.tables, {**self.assembly, **change})
        conflicting = deepcopy(self.tables)
        conflicting[refine.RECYCLE_TABLE][0]['rewards_combinable'] = True
        with self.assertRaises(ValueError):
            refine.project(self.server, self.public, conflicting, self.assembly)
        conflicting = deepcopy(self.tables)
        conflicting[refine.AFFIX_TABLE][0]['sample_weight'] = 1
        with self.assertRaises(ValueError):
            refine.project(self.server, self.public, conflicting, self.assembly)


if __name__ == '__main__':
    unittest.main()
