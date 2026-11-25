"""验证个人与世界首领的独立工作簿投影和公共展示边界。"""

from copy import deepcopy
import unittest

import export_configs as config
from test_source_map_projection import module, ROOT, restore_rule28_skill_selection


class BossCandidateProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema = config.load_json(ROOT / 'config/schema/catalog.json')
        cls.tables, errors, _, _ = config.collect(schema)
        assert not errors, errors

    def test_challenge_maps_do_not_join_normal_catalog_or_rewards(self):
        fixture = module.fixture_from_rows(self.tables)
        client = module.public_gameplay(fixture)
        challenge_maps = client['challenge_maps']
        self.assertEqual([(row['id'], row['scene_kind'], row['floor_map_id']) for row in challenge_maps],
                         [('80', 3, '1'), ('82', 3, '1'), ('83', 3, '1'), ('81', 4, '1'), ('84', 4, '1')])
        for row in challenge_maps:
            self.assertEqual((row['width'], row['height'], row['blocked'], row['resource_key']), (13, 13, [], 'development/maps/personal-map'))
        boss_ids = {entry['id'] for entry in challenge_maps}
        self.assertTrue(boss_ids.isdisjoint(row['id'] for row in client['map_catalog']))
        self.assertTrue(all(boss_ids.isdisjoint(region['map_ids']) for region in client['map_regions']))
        ordinary_ids = [row['id'] for row in fixture['monsters']]
        challenge_ids = [row['id'] for row in fixture['challenge_monsters']]
        self.assertEqual(ordinary_ids, [1, 3, 4, 5])
        self.assertEqual(challenge_ids, [80001, 80002, 80003, 80004, 80005])
        for row in fixture['challenge_monsters']:
            self.assertEqual([row[field] for field in ('respawn_ms', 'exp', 'gold_min', 'gold_max')], [0] * 4)
        self.assertNotIn('kill_reward_gold', fixture['personal_boss'])
        self.assertEqual([(row['template_id'], row['daily_open_offsets_ms']) for row in client['world_bosses']],
                         [(2, [43_200_000]), (5, [72_000_000])])
        self.assertEqual((client['world_boss']['window_ms'], client['world_boss']['return_delay_ms']), (3_600_000, 5_000))
        self.assertEqual({row['definition_id']: row['profile'] for row in client['monster_profiles']
                          if row['definition_id'] in challenge_ids},
                         {80001: 'monster_scarecrow', 80002: 'monster_scarecrow', 80003: 'monster_boss_bone_elite',
                          80004: 'monster_boss_white_boar', 80005: 'monster_boss_zuma_taurus'})

    def test_legacy_selection_keeps_single_template_each_and_original_schedule(self):
        tables = deepcopy(self.tables)
        profile = tables['dev_first_loop_profile'][0]['parameters']
        profile['boss_template_ids'] = [1, 2]
        del profile['world_boss_slot_templates']
        fixture = module.fixture_from_rows(tables)
        client = module.public_gameplay(fixture)
        self.assertEqual(client['world_boss']['daily_open_offsets_ms'], [43_200_000, 72_000_000])
        self.assertEqual([row['id'] for row in client['challenge_maps']], ['80', '81'])
        self.assertNotIn('world_bosses', client)

    def test_rotation_rejects_missing_or_invalid_selection(self):
        for slots in (None, [2], [2, 2], [2, 3], [2, 5, 5]):
            tables = deepcopy(self.tables)
            tables['dev_first_loop_profile'][0]['parameters']['world_boss_slot_templates'] = slots
            with self.subTest(slots=slots), self.assertRaises(ValueError):
                module.fixture_from_rows(tables)

    def test_unsupported_cooldown_map_and_loot_are_rejected(self):
        for key, value in (('map_id', 1), ('monster_id', 1), ('rank_reward_group_id', 1), ('failure_cooldown_seconds', 0)):
            tables = deepcopy(self.tables)
            personal = next(row for row in tables['game_boss_templates'] if row['boss_kind'] == 'personal')
            personal[key] = value
            with self.subTest(field=key), self.assertRaises(ValueError):
                module.fixture_from_rows(tables)

    def test_explicit_development_selection_is_required(self):
        tables = deepcopy(self.tables)
        profile = tables['dev_first_loop_profile'][0]['parameters']
        restore_rule28_skill_selection(profile)
        profile['rule_version'] = 27
        with self.assertRaises(ValueError):
            module.fixture_from_rows(tables)
        del profile['boss_template_ids']
        del profile['world_boss_slot_templates']
        fixture = module.fixture_from_rows(tables)
        self.assertNotIn('personal_boss', fixture)
        self.assertNotIn('challenge_maps', module.public_gameplay(fixture))
