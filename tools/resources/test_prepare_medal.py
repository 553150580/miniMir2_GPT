"""校验勋章隔离装配和世界首领原奖励的增量。"""

from copy import deepcopy
import unittest

import prepare_equipment_refine as refine
import prepare_medal as medal
import prepare_mining as mining
import test_prepare_equipment_refine as previous


class MedalProjectionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        origin = previous.EquipmentRefineProjectionTest
        origin.setUpClass()
        cls.tables, cls.default_server, cls.default_public = origin.tables, origin.default_server, origin.default_public
        cls.server, cls.public = refine.project(origin.server, origin.public, origin.tables, origin.assembly)
        cls.assembly = mining.config.load_json(mining.ROOT / 'tools/resources/testdata/medal-isolated.json')

    def test_projection_changes_only_selected_reward_and_new_catalog(self):
        originals = deepcopy((self.server, self.public))
        server, public = medal.project(self.server, self.public, self.tables, self.assembly)
        self.assertEqual((self.server, self.public), originals)
        self.assertNotIn(medal.KEY, self.default_server)
        self.assertNotIn(medal.KEY, self.default_public)
        self.assertEqual({key: server[key] for key in self.server if key not in ('items', 'world_boss', '_source_evidence')},
                         {key: self.server[key] for key in self.server if key not in ('items', 'world_boss', '_source_evidence')})
        self.assertEqual(public['items'][:-1], self.public['items'])
        self.assertEqual(server['items'][:-1], self.server['items'])
        self.assertEqual(server['items'][-1]['id'], public['items'][-1]['item_id'])
        self.assertEqual(public['items'][-1]['kind'], 'medal_fragment')
        self.assertFalse(public['items'][-1]['use_enabled'])
        target, original = server['world_boss'], self.server['world_boss']
        self.assertEqual({key: value for key, value in target.items() if key != 'rewards'},
                         {key: value for key, value in original.items() if key != 'rewards'})
        rewards = target['rewards']
        self.assertEqual(rewards, {**original['rewards'], 'participation': [
            *original['rewards']['participation'], {'item_id': 900061, 'quantity': 6},
        ]})
        self.assertEqual(server[medal.KEY], public[medal.KEY])
        self.assertEqual([row['fragment_cost'] for row in server[medal.KEY]['levels']], [2, 3, 5])
        self.assertEqual(server['_source_evidence'][medal.KEY]['binding'], 'original_world_boss_kill_time_privilege')

    def test_rejects_predecessor_or_workbook_scope_drift(self):
        for changed in ({'fragment_item_id': 900031}, {'world_boss_template_id': 1}, {'levels': []},
                        {'world_boss_fragments': 100}):
            with self.subTest(changed=changed), self.assertRaises(ValueError):
                medal.project(self.server, self.public, self.tables, {**self.assembly, **changed})
        changed = deepcopy(self.tables)
        changed[medal.LEVELS_TABLE].append({'id': 1})
        with self.assertRaises(ValueError):
            medal.project(self.server, self.public, changed, self.assembly)
        changed = deepcopy(self.tables)
        boss = next(row for row in changed[medal.BOSS_TABLE] if row['id'] == 2)
        boss['participation_reward_group_id'] = 1
        with self.assertRaises(ValueError):
            medal.project(self.server, self.public, changed, self.assembly)


if __name__ == '__main__':
    unittest.main()
