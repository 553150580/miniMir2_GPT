"""验证部位强化隔离投影的唯一来源与原目录保真。"""

from copy import deepcopy
import unittest

import prepare_combat_luck as combat
import prepare_equipment_enhancement as enhancement
import prepare_luck_oil as oil
import prepare_mining as mining
import prepare_recycle_oil as recycle
import prepare_wing_oil as wing_oil
import prepare_wings as wings


class EquipmentEnhancementProjectionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        """从默认快照和原隔离装配构建前置成长数据。"""
        cfg = mining.config
        cls.tables, errors, _, _ = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
        if errors:
            raise RuntimeError(errors)
        folder = mining.ROOT / 'tools/resources/testdata'
        cls.assembly = cfg.load_json(folder / 'equipment-enhancement-isolated.json')
        cls.default_server, cls.default_public = cfg.load_json(mining.BASE_FIXTURE), cfg.load_json(mining.BASE_PUBLIC)
        server, public = combat.compose_predecessors(cls.default_server, cls.default_public, cls.tables, {},
                                                   folder / 'mining-isolated.json', folder / 'forge-isolated.json')
        server, public = combat.project(server, public, cls.tables, cfg.load_json(folder / 'combat-luck-isolated.json'))
        server, public = oil.project(server, public, cls.tables, cfg.load_json(folder / 'luck-oil-isolated.json'))
        server, public = recycle.project(server, public, cls.tables)
        server, public = wings.project(server, public, cls.tables, cfg.load_json(folder / 'wings-isolated.json'))
        cls.server, cls.public = wing_oil.project(server, public, cls.tables)

    def test_workbook_rules_and_boss_reward_preserve_default_and_predecessors(self):
        """核对数据一致、运行槽映射和一份新附件的精确增量。"""
        originals = deepcopy((self.server, self.public))
        server, public = enhancement.project(self.server, self.public, self.tables, self.assembly)
        self.assertEqual((self.server, self.public), originals)
        self.assertNotIn(enhancement.KEY, self.default_server)
        self.assertNotIn(enhancement.KEY, self.default_public)
        self.assertEqual(server[enhancement.KEY], public[enhancement.KEY])
        expected_steps = [{'current_level': row['from_level'], 'stone_cost': row['stone_cost'],
                           'success_ppm': row['success_ppm'], 'bonus_bp': row['base_attr_increment_bp']}
                          for row in self.tables[enhancement.LEVEL_TABLE]]
        self.assertEqual(server[enhancement.KEY]['levels'], expected_steps)
        self.assertEqual(sum(row['bonus_bp'] for row in expected_steps), 1_500)
        source = server['_source_evidence'][enhancement.KEY]
        self.assertEqual([source['runtime_slot_source_ids'][str(slot)] for slot in range(4)], [1, 4, 2, 3])
        self.assertFalse(source['craft_extra_base_included'])
        self.assertEqual(server['personal_boss']['item_reward'], {'pending_limit': 2, 'items': [{'item_id': 900041, 'quantity': 10}]})
        self.assertEqual(server['personal_boss_variants'], self.server['personal_boss_variants'])
        self.assertEqual(server['world_boss'], self.server['world_boss'])
        self.assertEqual(server['world_boss_variants'], self.server['world_boss_variants'])
        self.assertEqual(server['items'][:-1], self.server['items'])
        self.assertEqual(public['items'][:-1], self.public['items'])

    def test_refuses_existing_material_reward_and_invalid_rule_data(self):
        """已有首领、材料身份或唯一表规则发生冲突时拒绝投影。"""
        for edit in ({'stone_cost': 1}, {'item_id': 900031}, {'personal_boss_template_id': 3}, {'reward_quantity': 100}):
            with self.subTest(edit=edit), self.assertRaises(ValueError):
                enhancement.project(self.server, self.public, self.tables, {**self.assembly, **edit})
        tables = deepcopy(self.tables)
        tables[enhancement.SLOT_TABLE][0]['strengthenable'] = False
        with self.assertRaises(ValueError):
            enhancement.project(self.server, self.public, tables, self.assembly)
        tables = deepcopy(self.tables)
        tables[enhancement.LEVEL_TABLE][0]['success_ppm'] = 1_000_001
        with self.assertRaises(ValueError):
            enhancement.project(self.server, self.public, tables, self.assembly)


if __name__ == '__main__':
    unittest.main()
