"""核对金币市场的来源状态、显式隔离装配和主配置保留。"""

from argparse import ArgumentTypeError
from copy import deepcopy
import unittest

import prepare_gold_market as market


class TestGoldMarketProjection(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema = market.config.load_json(market.ROOT / 'config/schema/catalog.json')
        cls.tables, errors, _, _ = market.config.collect(schema)
        if errors:
            raise AssertionError(errors)
        cls.base = market.config.load_json(market.BASE_FIXTURE)
        cls.assembly = {'order_expiry_seconds': 120, 'reference_price_yuanbao_units': 100}

    def test_scope_and_workbook_statuses(self):
        base = deepcopy(self.base)
        candidate = market.project_fixture(base, self.tables, self.assembly)
        self.assertNotIn('gold_market', base)
        self.assertEqual(base, self.base)
        self.assertEqual(candidate.pop('gold_market'), {
            'gold_per_lot': 10000, 'price_step_yuanbao_units': 1,
            'max_buy_orders_per_account_run': 5, 'max_sell_orders_per_account_run': 5,
            'tax_divisor': 10, 'order_expiry_seconds': 120, 'reference_price_yuanbao_units': 100,
            'price_window_lots': 5000, 'price_limit_bp': 2000,
        })
        evidence = candidate['_source_evidence'].pop('gold_market')
        self.assertEqual(candidate, base)
        self.assertFalse(evidence['default_enabled'])
        self.assertEqual(evidence['explicit_test_inputs'], self.assembly)
        gold_rows = evidence['workbook_rules'][market.GOLD_RULES]
        self.assertEqual([row['record_status'] for row in gold_rows], ['confirmed'] * 13 + ['unverified'])
        server_rows = evidence['workbook_rules'][market.SERVER_RULES]
        self.assertEqual([row['record_status'] for row in server_rows], ['candidate'] * 2 + ['unverified'] + ['confirmed'] * 4)

    def test_explicit_values_and_integer_bounds(self):
        for assembly in ({}, {'order_expiry_seconds': 120}, {**self.assembly, 'gold_per_lot': 1},
                         {**self.assembly, 'order_expiry_seconds': 0}, {**self.assembly, 'order_expiry_seconds': 2**32},
                         {**self.assembly, 'reference_price_yuanbao_units': True},
                         {**self.assembly, 'reference_price_yuanbao_units': 100.0},
                         {**self.assembly, 'reference_price_yuanbao_units': 2**63 - 1}):
            with self.subTest(assembly=assembly), self.assertRaises(ValueError):
                market.compile_rules(self.tables, assembly)
        with self.assertRaises(ArgumentTypeError):
            market.work_path(market.BASE_FIXTURE)

    def test_rejects_unreviewed_workbook_changes(self):
        for table, key, fields in (
            (market.GOLD_RULES, 'gold_per_lot', {'record_status': 'candidate'}),
            (market.GOLD_RULES, 'order_expiry_seconds', {'rule_value': 120}),
            (market.GOLD_RULES, 'same_account_trade', {'rule_value': True}),
            (market.SERVER_RULES, market.PRICE_PREFIX + 'reference_limit_bp', {'record_status': 'confirmed'}),
            (market.SERVER_RULES, market.PRICE_PREFIX + 'cold_start_price_ingot_u', {'rule_value': 100}),
            (market.SERVER_RULES, market.PRICE_PREFIX + 'tax_divisor', {'rule_value': 20}),
        ):
            with self.subTest(key=key), self.assertRaises(ValueError):
                tables = deepcopy(self.tables)
                next(row for row in tables[table] if row['rule_key'] == key).update(fields)
                market.compile_rules(tables, self.assembly)
