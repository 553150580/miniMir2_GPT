"""核验托管权益从当前已定工作簿投影为服务端布尔值。"""

import unittest

import prepare_first_loop as first_loop
from prepare_hosting_benefits import compile_benefits


class TestHostingBenefits(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema = first_loop.config.load_json(first_loop.ROOT / 'config/schema/catalog.json')
        tables, errors, _, _ = first_loop.config.collect(schema)
        if errors:
            raise AssertionError(errors)
        cls.rows = tables['game_entitlement_rules']
        cls.auto_rule = next(row for row in cls.rows if row['rule_key'] == 'hosting_auto_personal_boss')
        cls.other_rows = [row for row in cls.rows if row is not cls.auto_rule]

    def test_confirmed_personal_boss_projects_only_to_server_fixture(self):
        self.assertIs(compile_benefits(self.rows)['auto_personal_boss'], True)
        fixture = first_loop.read(first_loop.DEST)
        self.assertEqual(fixture['hosting_benefits'], compile_benefits(self.rows))
        self.assertNotIn('hosting_benefits', first_loop.public_gameplay(fixture))

    def test_requires_a_single_confirmed_boolean_value(self):
        false_rule = {**self.auto_rule, 'rule_value': False}
        self.assertIs(compile_benefits([*self.other_rows, false_rule])['auto_personal_boss'], False)
        invalid_rows = [[], [self.auto_rule, self.auto_rule], [{**self.auto_rule, 'record_status': 'candidate'}]]
        invalid_rows.extend([{**self.auto_rule, 'rule_value': value}] for value in ('true', 1, None))
        for rules in invalid_rows:
            with self.subTest(rules=rules), self.assertRaises(ValueError):
                compile_benefits([*self.other_rows, *rules])
