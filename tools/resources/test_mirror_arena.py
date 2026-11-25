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


if __name__ == '__main__':
    unittest.main()
