"""核对工作簿幸运投影、隔离目标和先前候选保真。"""

from copy import deepcopy
import unittest

import prepare_combat_luck as luck
import prepare_mining as mining


class CombatLuckProjectionTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cfg = mining.config
        cls.tables, errors, _, _ = cfg.collect(cfg.load_json(mining.ROOT / 'config/schema/catalog.json'))
        if errors:
            raise RuntimeError(errors)
        cls.directory = mining.ROOT / 'tools/resources/testdata'
        cls.assembly = cfg.load_json(cls.directory / 'combat-luck-isolated.json')
        cls.base = cfg.load_json(mining.BASE_FIXTURE), cfg.load_json(mining.BASE_PUBLIC)

    def test_projects_workbook_luck_and_only_explicit_resistances(self):
        baseline = deepcopy(self.base)
        server, public = luck.project(*self.base, self.tables, self.assembly)
        self.assertEqual(self.base, baseline)
        self.assertEqual(public, baseline[1])
        self.assertNotIn('combat_luck', self.base[0])
        self.assertNotIn('combat_luck', public)
        self.assertEqual(server['combat_luck'], {
            'weapon_wing_ppm': [0, 20000, 30000, 44000, 60000, 80000, 110000, 150000, 210000, 300000],
            'other_ppm': [0, 20000, 30000, 44500],
            'drop_ppm': 10000,
            'initial_weights': [1, 1, 1],
            'monster_resistance_ppm': {'1': 200000},
            'role_resistance_ppm': {'1': 200000},
        })
        evidence = server['_source_evidence']['combat_luck']
        self.assertEqual(evidence['resistance_source_status'], 'isolation_test_only')
        self.assertEqual({row['table_id'] for row in evidence['record_sources']}, {
            'game_luck_contribution', 'game_luck_initial', 'game_luck_rules', 'game_luck_sources',
        })
        self.assertFalse(evidence['default_enabled'])

    def test_real_forge_candidate_is_kept_and_values_follow_the_workbook(self):
        server, public = luck.compose_predecessors(*self.base, self.tables, {},
                                                  self.directory / 'mining-isolated.json', self.directory / 'forge-isolated.json')
        tables = deepcopy(self.tables)
        for row in tables[luck.CONTRIBUTION_TABLE]:
            if row['source_group'] in ('weapon', 'wing') and row['luck'] == 3:
                row['contribution_bp'] = 441
        next(row for row in tables[luck.INITIAL_TABLE] if row['source_key'] == 'equipment_drop')['initial_value_weights']['2'] = 3
        projected, published = luck.project(server, public, tables, self.assembly)
        self.assertEqual(projected['combat_luck']['weapon_wing_ppm'][3], 44100)
        self.assertEqual(projected['combat_luck']['initial_weights'], [1, 3, 1])
        self.assertEqual(projected['forge'], server['forge'])
        self.assertEqual(projected['mining'], server['mining'])
        self.assertEqual(published, public)

    def test_invalid_resistance_and_incompatible_source_changes_are_rejected(self):
        for assembly in ({'other_ppm': [0]}, {'role_resistance_ppm': {'3': 0}}, {'monster_resistance_ppm': {'99999': 1}},
                         {'monster_resistance_ppm': {'01': 1}}, {'monster_resistance_ppm': {'1': 1000001}}, {'role_resistance_ppm': {'1': True}}):
            with self.subTest(assembly=assembly), self.assertRaises(ValueError):
                luck.project(*self.base, self.tables, {**self.assembly, **assembly})
        for table, mutate in (
            (luck.CONTRIBUTION_TABLE, lambda rows: rows.append(rows[0])),
            (luck.CONTRIBUTION_TABLE, lambda rows: next(row for row in rows if row['source_group'] == 'weapon' and row['luck'] == 3).update(contribution_bp=441)),
            (luck.CONTRIBUTION_TABLE, lambda rows: rows.pop()),
            (luck.RULE_TABLE, lambda rows: next(row for row in rows if row['rule_key'] == 'order')['rule_value'].reverse()),
            (luck.SOURCE_TABLE, lambda rows: next(row for row in rows if row['source_key'] == 'necklace').update(oil_allowed=True)),
            (luck.INITIAL_TABLE, lambda rows: next(row for row in rows if row['source_key'] == 'equipment_drop').update(fixed_initial_value=3)),
        ):
            tables = deepcopy(self.tables)
            mutate(tables[table])
            with self.subTest(table=table), self.assertRaises(ValueError):
                luck.project(*self.base, tables, self.assembly)


if __name__ == '__main__':
    unittest.main()
