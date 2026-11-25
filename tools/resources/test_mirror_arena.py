"""验证隔离镜像投影沿用当前玩法 Excel 的原值和状态。"""

from copy import deepcopy
import unittest

import prepare_mirror_arena as arena


class TestMirrorArenaProjection(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema = arena.configuration.load_json(arena.ROOT / 'config/schema/catalog.json')
        cls.tables, errors, _, _ = arena.configuration.collect(schema)
        if errors:
            raise AssertionError(errors)
        cls.fixture = arena.configuration.load_json(arena.BASE_FIXTURE)

    def setUp(self):
        # 日期、网格落点和候选等级差只是显式的本轮隔离输入。
        self.assembly = {'template_id': 1, 'display_name': '隔离镜像', 'map_id': 90001, 'floor_map_id': 1,
                         'season_anchor': '2026-11-25T16:00:00Z', 'match_policy': 'cross_job_level_window',
                         'max_level_gap': 3, 'pet_death_policy': 'owner_death_ends_pet', 'blocked': [{'x': 5, 'y': 5}],
                         'attacker_spawn': {'x': 3, 'y': 5}, 'defender_spawn': {'x': 8, 'y': 5},
                         'attacker_pet_spawn': {'x': 3, 'y': 6}, 'defender_pet_spawn': {'x': 8, 'y': 6}}

    def test_confirmed_rules_and_candidates_project_as_is(self):
        rule, public_map = arena.project(self.tables, self.assembly, self.fixture)
        self.assertNotIn('mirror_arena', self.fixture)
        self.assertEqual(rule['season_anchor'], '2026-11-26T00:00:00+08:00')
        self.assertEqual([rule[name] for name in ('season_days', 'daily_limit', 'opponent_limit', 'duration_ms',
                                                  'win_base_points', 'loss_points', 'attack_history_limit',
                                                  'defense_history_limit')], [15, 5, 3, 60000, 5, 1, 20, 20])
        self.assertEqual(rule['streak_points'], [{'streak': 1, 'points': 5}, {'streak': 3, 'points': 6}, {'streak': 5, 'points': 8}])
        self.assertEqual(rule['timeout_policy'], arena.TIMEOUT)
        self.assertNotIn('rewards', rule)
        self.assertNotIn('feather_exchange_points', rule)
        self.assertNotIn('settlement', rule)
        self.assertNotIn('exchange', rule)
        self.assertEqual(public_map['id'], '90001')
        self.assertEqual(public_map['floor_map_id'], '1')
        self.assertEqual(public_map['scene_kind'], 6)
        self.assertEqual(public_map['monster_definition_ids'], [])
        self.assertEqual(public_map['resource_key'], self.fixture['map']['resource_key'])

    def test_missing_scope_partition_and_non_candidate_rules_are_rejected(self):
        for params in (
            {key: value for key, value in self.assembly.items() if key != 'max_level_gap'},
            {**self.assembly, 'season_anchor': '2026-11-26T00:00:00Z'},
            {**self.assembly, 'season_anchor': '2026-11-26T00:00:00'},
            {**self.assembly, 'match_policy': '按虚构战力匹配'},
            {**self.assembly, 'map_id': 1},
            {**self.assembly, 'map_id': 80},
            {**self.assembly, 'blocked': [{'x': 5, 'y': y} for y in range(13)]},
            {**self.assembly, 'defender_pet_spawn': self.assembly['attacker_spawn']},
        ):
            with self.subTest(params=params), self.assertRaises(ValueError):
                arena.project(self.tables, params, self.fixture)
        tables = deepcopy(self.tables)
        row = next(row for row in tables[arena.RULES] if row['rule_key'] == 'battle_timeout_sec')
        row['record_status'] = 'historical'
        with self.assertRaisesRegex(ValueError, 'battle_timeout_sec'):
            arena.project(tables, self.assembly, self.fixture)

    def test_explicit_isolated_settlement_uses_confirmed_policy_and_optional_candidate_price(self):
        sample = self.season_sample()
        rule, public_map = arena.project(self.tables, self.assembly, self.fixture, sample)
        self.assertFalse(self.tables[arena.REWARDS])
        self.assertEqual(public_map['scene_kind'], 6)
        self.assertEqual(rule['settlement'], {
            'ranking_tiebreak': 'earlier_last_challenge', 'participation_min_challenges': 10,
            'ranks': sample['ranks'], 'participation': sample['participation'],
        })
        self.assertEqual(rule['exchange'], {'item_id': 35, 'price': 250, 'quantity': 1})
        sample['ranks'][0]['reward']['bound_yuanbao_units'] = 45
        self.assertEqual(rule['settlement']['ranks'][0]['reward']['bound_yuanbao_units'], 17)
        del sample['exchange']
        without_exchange, _ = arena.project(self.tables, self.assembly, self.fixture, sample)
        self.assertIn('settlement', without_exchange)
        self.assertNotIn('exchange', without_exchange)
        self.assertNotIn('mirror_arena', self.fixture)

    def test_settlement_rejects_unverified_rank_rules_and_already_authored_workbook_rewards(self):
        for key, value, status in (
            ('ranking_tiebreak', 'later_last_challenge', 'confirmed'),
            ('settlement_sequence', list(reversed(arena.SETTLEMENT_SEQUENCE)), 'confirmed'),
            ('exchange_balance_persists', False, 'confirmed'),
            ('participation_min_challenges', 76, 'confirmed'),
            ('participation_min_challenges', 10, 'candidate'),
            ('feather_exchange_bound', False, 'confirmed'),
            ('feather_exchange_points', 250, 'unverified'),
        ):
            with self.subTest(key=key, status=status):
                tables = deepcopy(self.tables)
                row = next(row for row in tables[arena.RULES] if row['rule_key'] == key)
                row.update(rule_value=value, record_status=status)
                with self.assertRaises(ValueError):
                    arena.project(tables, self.assembly, self.fixture, self.season_sample())
        tables = deepcopy(self.tables)
        tables[arena.REWARDS] = [{'record_status': 'candidate'}]
        with self.assertRaisesRegex(ValueError, '竞技正式奖表已有当前记录'):
            arena.project(tables, self.assembly, self.fixture, self.season_sample())

    def test_settlement_rejects_unknown_assets_overlapping_ranges_and_unverified_exchange_item(self):
        invalid = ({}, {'ranks': []}, {'participation': {'bound_gold': 1}}, {'participation': {'bound_yuanbao_units': True}},
                   {'participation': {'bound_yuanbao_units': arena.MAX_INT64}},
                   {'participation': {'bound_yuanbao_units': 3, 'title_id': 1}},
                   {'exchange': {'item_id': 99999, 'quantity': 1}}, {'exchange': {'item_id': 35, 'quantity': 201}},
                   {'exchange': {'item_id': 35, 'quantity': 1, 'price': 100}})
        for value in invalid:
            with self.subTest(sample=value), self.assertRaises(ValueError):
                arena.project(self.tables, self.assembly, self.fixture, {**self.season_sample(), **value} if value else value)
        sample = self.season_sample()
        sample['ranks'][1]['first_rank'] = 1
        with self.assertRaises(ValueError):
            arena.project(self.tables, self.assembly, self.fixture, sample)

    @staticmethod
    def season_sample():
        """数额和已有药水是隔离样例，未录入正式奖表或羽毛目录。"""
        return {'ranks': [{'first_rank': 1, 'last_rank': 1, 'reward': {'bound_yuanbao_units': 17}},
                          {'first_rank': 2, 'last_rank': 4, 'reward': {'bound_yuanbao_units': 2}}],
                'participation': {'bound_yuanbao_units': 3}, 'exchange': {'item_id': 35, 'quantity': 1}}


if __name__ == '__main__':
    unittest.main()
