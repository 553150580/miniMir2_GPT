"""仓储唯一来源、权限语义和有界分页；不新增经济参数或付费资格。"""
from copy import deepcopy
import unittest

import export_configs as config
from test_source_map_projection import module, ROOT


class WarehouseConfigurationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = config.load_json(ROOT / 'config/schema/catalog.json')
        cls.tables, errors, _, _ = config.collect(cls.schema)
        assert not errors, errors

    @staticmethod
    def profile(tables):
        return tables['dev_first_loop_profile'][0]['parameters']

    @staticmethod
    def row(tables, key):
        return next(row for row in tables['game_warehouse_rules'] if row['rule_key'] == key)

    def compile(self, tables=None):
        tables = self.tables if tables is None else tables
        return module.warehouse_rules(tables, self.profile(tables))

    def test_current_confirmed_contract_and_explicit_technical_window(self):
        self.assertEqual(self.profile(self.tables)['rule_version'], 23)
        self.assertEqual(self.compile(), {'personal_initial_slots': 30, 'personal_expansion_slots': 30,
                         'personal_expansion_yuanbao_units': 30000, 'shared_slots': 50, 'query_page_limit': 30})
        fixture = module.fixture_from_rows(self.tables)
        self.assertEqual(fixture['warehouses'], self.compile())
        self.assertFalse(fixture['_source_evidence']['warehouses']['payment_or_privilege_granted'])
        self.assertIsNone(self.row(self.tables, 'pending_reward_queue_capacity')['rule_value'])
        self.assertEqual(self.row(self.tables, 'pending_reward_queue_capacity')['record_status'], 'unverified')

    def test_capacity_and_price_read_confirmed_rows_without_parallel_defaults(self):
        # 明确隔离改值只核编译器取值；发布仍须通过已定行摘要，不改唯一表。
        tables = deepcopy(self.tables)
        changes = {'personal_warehouse_initial_slots': 60, 'personal_warehouse_page_slots': 30,
                   'personal_warehouse_page_price_ingot_u': 30100, 'shared_warehouse_slots': 53}
        for key, value in changes.items():
            self.row(tables, key)['rule_value'] = value
        self.profile(tables)['warehouses']['query_page_limit'] = 17
        self.assertEqual(self.compile(tables), {'personal_initial_slots': 60, 'personal_expansion_slots': 30,
                         'personal_expansion_yuanbao_units': 30100, 'shared_slots': 53, 'query_page_limit': 17})
        self.assertTrue(config.check_json_contracts(self.schema, tables))

    def test_main_gate_requires_complete_typed_explicit_selection(self):
        for value in ['missing', None, {}, False, {'enabled': False, 'query_page_limit': 30},
                      {'enabled': 1, 'query_page_limit': 30}, {'enabled': True},
                      {'enabled': True, 'query_page_limit': 30, 'free_privilege': True}]:
            tables = deepcopy(self.tables)
            if value == 'missing':
                self.profile(tables).pop('warehouses')
            else:
                self.profile(tables)['warehouses'] = value
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.compile(tables)

    def test_window_rejects_null_bool_fraction_overflow_and_unbounded_work(self):
        for value in [None, False, 0, -1, 1.5, '30', 2**32, 51]:
            tables = deepcopy(self.tables)
            self.profile(tables)['warehouses']['query_page_limit'] = value
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.compile(tables)

    def test_rules_require_single_confirmed_correctly_typed_source(self):
        for change in ['missing', 'duplicate', 'candidate', 'unverified', 'null', 'boolean']:
            tables = deepcopy(self.tables)
            row = self.row(tables, 'shared_warehouse_slots')
            if change == 'missing':
                tables['game_warehouse_rules'].remove(row)
            elif change == 'duplicate':
                tables['game_warehouse_rules'].append(deepcopy(row))
            elif change in ('candidate', 'unverified'):
                row['record_status'] = change
            else:
                row['rule_value'] = None if change == 'null' else True
            with self.subTest(change=change), self.assertRaises(ValueError):
                self.compile(tables)

    def test_capacity_and_cost_cannot_be_negative_free_inexact_or_partial_page(self):
        for key in ['personal_warehouse_initial_slots', 'personal_warehouse_page_slots',
                    'personal_warehouse_page_price_ingot_u', 'shared_warehouse_slots']:
            for value in [0, -1, False, None, '30', 1.5, 2**53]:
                tables = deepcopy(self.tables)
                self.row(tables, key)['rule_value'] = value
                with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                    self.compile(tables)
        tables = deepcopy(self.tables)
        self.row(tables, 'personal_warehouse_initial_slots')['rule_value'] = 31
        with self.assertRaises(ValueError):
            self.compile(tables)

    def test_shared_privilege_binding_currency_and_origin_policies_are_not_optional(self):
        keys = ['shared_warehouse_requires_active_privilege', 'shared_warehouse_origin_isolation_survives_merge',
                'shared_warehouse_modify_items', 'personal_warehouse_stores_currency',
                'personal_warehouse_accepts_bound_items', 'allow_shared_warehouse_normal_fine_unbound_equipment']
        for key in keys:
            tables = deepcopy(self.tables)
            row = self.row(tables, key)
            row['rule_value'] = not row['rule_value']
            with self.subTest(key=key), self.assertRaises(ValueError):
                self.compile(tables)
        for value in [None, ['unbound_items', 'gold', 'ingot', 'bound_ingot'], ['unbound_items'], 'unbound_items']:
            tables = deepcopy(self.tables)
            self.row(tables, 'shared_warehouse_allowed')['rule_value'] = value
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.compile(tables)

    def test_downgrade_cannot_activate_warehouse_contract(self):
        tables = deepcopy(self.tables)
        self.profile(tables)['rule_version'] = 22
        with self.assertRaisesRegex(ValueError, 'rule23'):
            self.compile(tables)
        self.profile(tables).pop('warehouses')
        self.assertIsNone(self.compile(tables))

    def test_public_contains_only_deep_copied_display_contract(self):
        fixture = module.fixture_from_rows(self.tables)
        before = deepcopy(fixture)
        public = module.public_gameplay(fixture)
        self.assertEqual(public['warehouses'], self.compile())
        self.assertNotIn('_source_evidence', public)
        public['warehouses']['personal_expansion_yuanbao_units'] += 1
        self.assertEqual(fixture, before)

    def test_nested_schema_rejects_unknown_keys_nulls_and_wrong_types(self):
        for value in [None, {}, {'enabled': True, 'query_page_limit': None},
                      {'enabled': True, 'query_page_limit': False},
                      {'enabled': True, 'query_page_limit': 30, 'max_pages': 5}]:
            tables = deepcopy(self.tables)
            self.profile(tables)['warehouses'] = value
            with self.subTest(value=value):
                self.assertTrue(config.check_json_contracts(self.schema, tables))


if __name__ == '__main__':
    unittest.main()
