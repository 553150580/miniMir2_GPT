"""规则25商店、耐久和既有技能书目录只投影显式Excel配置。"""

from copy import deepcopy
import unittest

import export_configs as config
from test_source_map_projection import ROOT, module


class NPCShopProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = config.load_json(ROOT / 'config/schema/catalog.json')
        cls.tables, errors, _, _ = config.collect(cls.schema)
        errors += config.check_json_contracts(cls.schema, cls.tables)
        if errors:
            raise ValueError(errors)
        cls.fixture = module.fixture_from_rows(cls.tables)

    def test_repair_defaults_and_prices_are_public_only_on_equipment(self):
        public = module.public_gameplay(self.fixture)
        self.assertEqual(public['equipment_durability'], self.fixture['equipment_durability'])
        for catalog, keys in ((self.fixture['items'], ('id', 'equipment_slot')),
                              (public['items'], ('item_id', 'equip_slot'))):
            expected = {218: (0, 4000, 50), 193: (1, 10000, 400), 259: (3, 8000, 500)}
            actual = {item[keys[0]]: (item[keys[1]], item['original_durability_u'], item['repair_base_price'])
                      for item in catalog if 'original_durability_u' in item}
            self.assertEqual(actual, expected)
        ordinary = {item['item_id']: item for item in public['items'] if item['kind'] in ('meat', 'misc')}
        self.assertEqual({item_id: item['kind'] for item_id, item in ordinary.items()}, {92: 'meat', 594: 'misc'})
        self.assertTrue(all(not item['use_enabled'] for item in ordinary.values()))

    def test_source_npcs_sell_only_eligible_goods_and_reuse_book_catalogue(self):
        fixture = self.fixture
        shops = {shop['key']: shop for shop in fixture['npc_shops']}
        self.assertEqual({shop['key'] for shop in shops.values() if shop['can_repair']}, {'weapon', 'clothing', 'jewelry'})
        self.assertEqual({price['item_id']: price['price_gold'] for shop in shops.values() if shop['key'] != 'books'
                          for price in shop['sell_prices']}, {218: 25, 193: 200, 259: 250, 92: 100, 35: 5, 36: 5, 594: 5})
        books = shops['books']
        self.assertEqual(books['offers'], fixture['skill_shop'])
        expected = {offer['item_id']: offer['price_units'] // (2 * offer['quantity']) for offer in books['offers']
                    if offer['currency'] == 'gold' and offer['item_id'] != fixture['skill_economy']['page_item_id']}
        self.assertEqual(len(expected), 22)
        self.assertEqual({price['item_id']: price['price_gold'] for price in books['sell_prices']}, expected)
        sources = next(town for town in fixture['_source_evidence']['compact_towns'] if town['map_id'] == 5)
        self.assertEqual({row['id']: row['line'] for row in sources['npcs'] if row['id'] in (5008, 5009)}, {5008: 78, 5009: 79})

    def test_refuses_invented_roles_missing_book_authorization_and_invalid_repair(self):
        edits = [
            lambda profile: profile['npc_shops'][0].update(roles=['不存在的商人']),
            lambda profile: profile['npc_shops'][1].update(roles=profile['npc_shops'][0]['roles']),
            lambda profile: profile['npc_shops'][0].update(sell_skill_books=True),
            lambda profile: profile['npc_shop_prices'].update(sell_ratio_denominator=0),
            lambda profile: profile['equipment_durability']['ordinary'].update(min_max_durability_u=4001),
            lambda profile: profile['equipment_durability']['special'].update(full_repair_cost_u=0),
        ]
        for index, edit in enumerate(edits):
            tables = deepcopy(self.tables)
            edit(tables['dev_first_loop_profile'][0]['parameters'])
            with self.subTest(index=index), self.assertRaises(ValueError):
                module.fixture_from_rows(tables)


if __name__ == '__main__':
    unittest.main()
