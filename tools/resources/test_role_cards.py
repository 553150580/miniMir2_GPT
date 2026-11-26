"""核验当前工作簿的角色月卡投影、金额单位与失败门禁。"""

import unittest

import prepare_first_loop as first_loop
from prepare_role_cards import compile_role_cards


class TestRoleCards(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema = first_loop.config.load_json(first_loop.ROOT / 'config/schema/catalog.json')
        tables, errors, _, _ = first_loop.config.collect(schema)
        if errors:
            raise AssertionError(errors)
        cls.products = tables['game_card_products']
        cls.privilege, cls.hosting = [row for row in cls.products if row['payment_key'] == 'ingot']
        cls.claim = next(row for row in tables['game_entitlement_rules']
                         if row['rule_key'] == 'privilege_daily_bound_ingot_u')

    def test_confirmed_role_products_and_daily_units_project_server_only(self):
        projected = compile_role_cards(self.products, [self.claim])
        self.assertEqual(projected, {'products': [
            {'id': 7, 'kind': 1, 'days': 30, 'price_yuanbao_units': 30000},
            {'id': 8, 'kind': 2, 'days': 30, 'price_yuanbao_units': 45000},
        ], 'daily_privilege_units': 2000})
        fixture = first_loop.read(first_loop.DEST)
        self.assertEqual(fixture['role_cards'], projected)
        self.assertNotIn('role_cards', first_loop.public_gameplay(fixture))
        sources = fixture['_source_evidence']['record_sources']['game_card_products']
        self.assertEqual([row['id'] for row in sources], [7, 8])

    def test_rejects_non_role_mixed_payment_and_invalid_product_terms(self):
        bad_fields = ({'record_status': 'candidate'}, {'role_scoped': False}, {'payment_key': 'bound_ingot'},
                      {'cash_fen': 1}, {'duration_days': 90}, {'duration_days': 30.0},
                      {'ingot_u': 0}, {'ingot_u': 2**63}, {'ingot_u': True}, {'id': self.hosting['id']},
                      {'card_key': self.hosting['card_key']}, {'card_key': 'other'})
        for fields in bad_fields:
            with self.subTest(fields=fields), self.assertRaises(ValueError):
                compile_role_cards([{**self.privilege, **fields}, self.hosting], [self.claim])
        with self.assertRaises(ValueError):
            compile_role_cards([self.privilege], [self.claim])

    def test_requires_unique_confirmed_daily_units(self):
        for claims in ([], [self.claim, self.claim], [{**self.claim, 'record_status': 'candidate'}],
                       [{**self.claim, 'rule_value': 0}], [{**self.claim, 'rule_value': True}]):
            with self.subTest(claims=claims), self.assertRaises(ValueError):
                compile_role_cards(self.products, claims)
