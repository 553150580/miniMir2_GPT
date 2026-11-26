"""核验真实工作簿现金卡的分币、双卡期限和隔离投影。"""

import unittest

import prepare_payment_cash_cards as payment


class TestPaymentCashCards(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema = payment.config.load_json(payment.ROOT / 'config/schema/catalog.json')
        cls.tables, errors, _, _ = payment.config.collect(schema)
        if errors:
            raise AssertionError(errors)
        cls.products = cls.tables[payment.PRODUCTS_TABLE]
        cls.base = payment.config.load_json(payment.BASE_FIXTURE)

    def test_six_workbook_products_and_source_are_exact_and_do_not_alter_primary_rules(self):
        result = payment.project_fixture(self.base, self.tables)
        cash = result.pop('payment')['cash_card_products']
        self.assertEqual([(p['id'], p['kind'], p['days'], p['price_fen']) for p in cash], [
            (1, 1, 30, 3000), (2, 1, 90, 7800), (3, 1, 365, 29800),
            (4, 2, 30, 4500), (5, 2, 90, 11800), (6, 2, 365, 46800),
        ])
        evidence = result['_source_evidence'].pop('payment')
        sources = evidence['record_sources'][payment.PRODUCTS_TABLE]
        self.assertFalse(evidence['default_enabled'])
        self.assertEqual([(r['id'], r['payment_key']) for r in sources], [(i, 'cash') for i in range(1, 7)])
        self.assertEqual([p['id'] for p in result['role_cards']['products']], [7, 8])
        self.assertEqual(result, self.base)
        self.assertNotIn('payment', self.base)

    def test_rejects_mixed_amounts_unconfirmed_products_and_ambiguous_terms(self):
        original, suffix = self.products[0], self.products[1:]
        mutations = ({'record_status': 'candidate'}, {'role_scoped': False}, {'payment_key': 'bound_ingot'},
                     {'source_ref': ''}, {'ingot_u': 1}, {'cash_fen': 0}, {'cash_fen': True},
                     {'cash_fen': 3000.0}, {'cash_fen': 2**63}, {'duration_days': 30.0},
                     {'duration_days': 90}, {'id': 7}, {'id': True}, {'card_key': 'unknown'})
        for changed in mutations:
            with self.subTest(changed=changed), self.assertRaises(ValueError):
                payment.compile_cash_cards([{**original, **changed}, *suffix])
        with self.assertRaises(ValueError):
            payment.compile_cash_cards(suffix)

    def test_rejects_nonmatching_yuanbao_base_and_existing_payment_configuration(self):
        for base in ({**self.base, 'role_cards': None}, {**self.base, 'payment': {}}, {**self.base, 'runtime_ready': True}):
            with self.subTest(keys=sorted(base)), self.assertRaises(ValueError):
                payment.project_fixture(base, self.tables)


if __name__ == '__main__':
    unittest.main()
