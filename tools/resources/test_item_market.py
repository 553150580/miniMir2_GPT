"""核验道具市场只投影已确认的期限、定点价格和双市场税额。"""

from copy import deepcopy
import unittest

import prepare_first_loop as first_loop
import prepare_item_market as market


class TestItemMarketProjection(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema = first_loop.config.load_json(first_loop.ROOT / 'config/schema/catalog.json')
        tables, errors, _, _ = first_loop.config.collect(schema)
        if errors:
            raise AssertionError(errors)
        cls.tables = market.source_rows(tables)

    def test_confirmed_sheet_projects_server_only(self):
        projected = market.compile_rules(self.tables)
        self.assertEqual(projected, {
            'max_listings_per_account_run': 5,
            'auction_seconds': 7200,
            'buyout_seconds': 86400,
            'bid_raise_bp': 1000,
            'price_step_yuanbao_units': 1,
            'minimum_raise_yuanbao_units': 1,
            'extend_when_remaining_below_seconds': 1200,
            'extend_to_remaining_seconds': 1200,
            'seller_commission_bp': 1000,
            'tax_divisor': 10,
        })
        fixture = first_loop.read(first_loop.DEST)
        self.assertEqual(fixture['item_market'], projected)
        self.assertNotIn('item_market', first_loop.public_gameplay(fixture))
        sources = fixture['_source_evidence']['record_sources']
        for table, rows in self.tables.items():
            self.assertEqual(sources[table], [{key: row[key] for key in ('id', 'source_ref', 'note')} for row in rows])

    def test_rejects_missing_duplicate_or_unconfirmed_row(self):
        table = 'game_item_market_rules'
        first, *remaining = self.tables[table]
        for rows in (remaining, [first, first, *remaining], [{**first, 'record_status': 'candidate'}, *remaining],
                     [{**first, 'rule_value': True}, *remaining]):
            with self.subTest(rows=rows), self.assertRaises(ValueError):
                market.compile_rules({**self.tables, table: rows})

    def test_rejects_unsupported_market_semantics_and_divergent_tax(self):
        for table, key, value in (
            ('game_item_market_rules', 'same_account_trade', True),
            ('game_item_market_rules', 'allow_negative_balance', True),
            ('game_item_market_rules', 'bid_reservation', 'partial'),
            ('game_item_market_rules', 'extend_to_remaining_sec', 1199),
            ('game_item_market_rules', 'price_step_ingot_u', 2),
            ('game_item_market_rules', 'buyout_seconds', 2**32),
            ('game_gold_market_rules', 'seller_commission_bp', 500),
            ('game_server_rules', 'game_gold_market_rules.tax_remainder_key', ['account_id', 'run_server_id']),
            ('game_server_rules', 'game_gold_market_rules.tax_divisor', 20),
        ):
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                changed = deepcopy(self.tables)
                next(row for row in changed[table] if row['rule_key'] == key)['rule_value'] = value
                market.compile_rules(changed)
