"""核对三城、原来源绑定和大地图入口在同一规则投影下保持一致。"""

from copy import deepcopy
import unittest

import export_configs as config
from test_source_map_projection import ROOT, module, restore_rule28_skill_selection


class CompactTownTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        schema = config.load_json(ROOT / 'config/schema/catalog.json')
        cls.tables, errors, _, _ = config.collect(schema)
        errors += config.check_json_contracts(schema, cls.tables)
        if errors:
            raise ValueError(errors)
        cls.fixture = module.fixture_from_rows(cls.tables)

    def profile(self):
        return deepcopy(self.tables['dev_first_loop_profile'][0]['parameters'])

    def test_compact_towns_and_menu_share_public_design_without_source_data(self):
        public = module.public_gameplay(self.fixture)
        towns = [row for row in public['map_catalog'] if row.get('compact_town')]
        self.assertEqual(public['map_regions'], [{'id': '0', 'name': '比奇省', 'map_ids': ['1', '3', '4', '5']}])
        self.assertEqual(public['town_travel']['destination_map_ids'], [town['id'] for town in towns])
        self.assertEqual([(town['name'], len(town['npcs'])) for town in towns],
                         [('银杏山谷', 6), ('边界村', 6), ('比奇城', 9)])
        for town in towns:
            self.assertEqual((town['width'], town['height'], town['scene_kind']), (17, 17, 2))
            self.assertEqual(town['blocked'], [npc['position'] for npc in town['npcs']])
            self.assertTrue(all(set(npc) == {'id', 'name', 'role', 'position', 'profile'} for npc in town['npcs']))

    def test_rule23_retains_original_legacy_town_and_travel_shape(self):
        tables = deepcopy(self.tables)
        profile = tables['dev_first_loop_profile'][0]['parameters']
        restore_rule28_skill_selection(profile)
        profile['rule_version'] = 23
        del profile['boss_template_ids']
        del profile['compact_towns'], profile['map_regions']
        for key in module.npc_shops.PROFILE_KEYS:
            del profile[key]
        profile['base_item_ids'] = [35, 36, 193, 218]
        prior = module.fixture_from_rows(tables)
        self.assertEqual(prior['maps'], self.fixture['maps'][:1])
        self.assertEqual(module.public_gameplay(prior)['town_travel'], {'enabled': True, 'town_map_id': '2'})
        self.assertNotIn('map_regions', module.public_gameplay(prior))

    def test_collision_and_legacy_aliases_refuse_invalid_workbook_records(self):
        legacy = self.fixture['maps'][:1]
        personal = self.fixture['map']
        changes = [
            lambda towns: towns[0]['npcs'][0].update(position=towns[0]['player_spawn']),
            lambda towns: towns[0]['npcs'][0].update(position=towns[0]['npcs'][1]['position']),
            lambda towns: towns[0]['npcs'][0]['source'].update(line=0),
            lambda towns: towns[1].update(legacy_safe_zone_ids=towns[0]['legacy_safe_zone_ids']),
            lambda towns: towns[1].update(legacy_safe_zone_ids=[106, 113]),
            lambda towns: towns[0].update(legacy_safe_zone_ids=[106]),
        ]
        for index, change in enumerate(changes):
            with self.subTest(index=index):
                profile = self.profile()
                change(profile['compact_towns'])
                with self.assertRaises(ValueError):
                    module.compact_towns.compile_compact_towns(profile, legacy, personal)

    def test_regions_allow_just_wilderness_but_require_each_map_exactly_once(self):
        profile = self.profile()
        available = [self.fixture['map'], *self.fixture['maps'][1:], {'id': 9, 'scene_kind': 1}]
        standalone = {'id': 'wilderness', 'name': '测试野外', 'map_ids': [9]}
        profile['map_regions'].append(standalone)
        self.assertEqual(module.compact_towns.compile_map_regions(profile, available)[-1], standalone)
        for invalid in [[3], [1], [9, 9], [9, 2]]:
            with self.subTest(members=invalid):
                profile['map_regions'][-1]['map_ids'] = invalid
                with self.assertRaises(ValueError):
                    module.compact_towns.compile_map_regions(profile, available)


if __name__ == '__main__':
    unittest.main()
