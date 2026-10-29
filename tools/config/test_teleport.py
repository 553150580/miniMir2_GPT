"""21配置只从唯一表投影；来源静态落点与普通闭门走路保持两种明确语义。"""
import base64
import copy
import hashlib
import json
import unittest

import export_configs as config
from test_source_map_projection import module, ROOT


class TeleportProjectionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tables, errors, _, _ = config.collect(config.load_json(ROOT/'config/schema/catalog.json'))
        assert not errors, errors
        cls.fixture = module.fixture_from_rows(cls.tables)

    @staticmethod
    def row(tables, table, identifier):
        return next(row for row in tables[table] if row['id'] == identifier)

    def skill_projection(self, tables):
        profile = tables['dev_first_loop_profile'][0]['parameters']
        skills = module.skill_definitions(tables, profile)
        module.validate_skills(profile, skills)
        return skills

    def buffs(self, tables):
        profile = tables['dev_first_loop_profile'][0]['parameters']
        return module.buff_definitions(tables, self.skill_projection(tables), module.casting_supply_rules(profile, tables))[0]

    def test_current_21_values_and_initializer_are_explicit_source_candidates(self):
        f = self.fixture
        skill = next(row for row in f['skills'] if row['id'] == 21)
        self.assertEqual(f['rule_version'], 22)
        self.assertEqual((skill['job_id'], skill['book_item_id'], skill['learn_level'], skill['learned_level'], skill['max_level']), (1, 13, 19, 0, 3))
        self.assertEqual((skill['mp_base_cost'], skill['mp_level_cost'], skill['global_spell_ms'], skill['action_ms'], skill['cooldown_base_ms'], skill['delay_base_ms']), (10, 3, 1800, 600, 1800, 200))
        self.assertEqual(skill['teleport'], {'max_attempts': 200, 'axis_rule': 'map_dimension_divided_by_rank_plus_one', 'landing_rule': 'source_static_walk', 'allow_occupied': True, 'allow_accepted_dead': True, 'check_source_map_at_due': True})
        self.assertFalse(skill['auto_enabled'])
        self.assertEqual((skill['material_item_id'], skill['material_quantity']), (0, 0))
        self.assertEqual(f['binding'], {'enabled': False, 'start_points': [], 'safe_zone_enabled': True})
        self.assertEqual(len(f['initial_skill_books']), 3)
        self.assertNotIn(13, [row['item_id'] for row in f['initial_skill_books']])

    def test_teleport_contract_has_no_missing_defaults_or_alternative_axis_rules(self):
        fields = self.row(self.tables, 'game_skills', 21)['target_rule']['teleport']
        for field in fields:
            with self.subTest(missing=field):
                tables = copy.deepcopy(self.tables)
                del self.row(tables, 'game_skills', 21)['target_rule']['teleport'][field]
                with self.assertRaises(ValueError): self.skill_projection(tables)
        changes = [{'max_attempts': value} for value in [False, 0, -1, 1.5, 4097]]
        changes += [{'axis_rule': 'map_dimension_divided_by_rank_plus_one_inclusive'}, {'landing_rule': 'closed_doors_and_dynamic_empty'}, {'extra_default': 1}]
        changes += [{field: value} for field in ['allow_occupied', 'allow_accepted_dead', 'check_source_map_at_due'] for value in [False, 1, None]]
        for change in changes:
            with self.subTest(change=change):
                tables = copy.deepcopy(self.tables)
                self.row(tables, 'game_skills', 21)['target_rule']['teleport'].update(change)
                with self.assertRaises(ValueError): self.skill_projection(tables)

    def test_numeric_changes_are_projected_without_hidden_source_values(self):
        tables = copy.deepcopy(self.tables)
        row = self.row(tables, 'game_skills', 21)
        row['target_rule']['teleport']['max_attempts'] = 197
        row['resource_costs'].update(mp_base_cost=17, mp_level_cost=5)
        row['cast_timing']['delay_base_ms'] = 223
        skill = next(row for row in self.skill_projection(tables) if row['id'] == 21)
        self.assertEqual((skill['teleport']['max_attempts'], skill['mp_base_cost'], skill['mp_level_cost'], skill['delay_base_ms']), (197, 17, 5, 223))
        self.row(tables, 'game_buffs', 21)['duration_ms'] = 31007
        buff = next(row for row in self.buffs(tables) if row['id'] == 21)
        self.assertEqual(buff['duration_ms'], 31007)

    def test_teleport_cannot_be_downgraded_to_old_rule_or_initial_binding_gate(self):
        for version in [13, 14]:
            tables = copy.deepcopy(self.tables)
            tables['dev_first_loop_profile'][0]['parameters']['rule_version'] = version
            with self.subTest(version=version), self.assertRaisesRegex(ValueError, 'rule15'): self.skill_projection(tables)
        tables = copy.deepcopy(self.tables)
        tables['dev_first_loop_profile'][0]['parameters']['binding']['safe_zone_enabled'] = False
        with self.assertRaisesRegex(ValueError, '可信来源安全区绑定'): self.skill_projection(tables)

    def test_teleport_has_explicit_neutral_damage_and_no_material_cost(self):
        row = self.row(self.tables, 'game_skills', 21)
        damage = row['damage_parameters']
        zero = ['accuracy_per_level', 'power_base', 'power_bonus', 'mpower_base', 'mpower_bonus', 'multiplier_bonus_ppm', 'healing_stat_multiplier', 'ready_proc_denominator']
        self.assertTrue(all(damage[key] == 0 for key in zero))
        self.assertEqual((damage['multiplier_base_ppm'], damage['undead_multiplier_ppm'], damage['non_undead_divisor']), (1000000, 1000000, 1))
        self.assertFalse(damage['add_caster_level'])
        for key in ['accuracy_by_level', 'max_attack_bonus_by_level', 'ready_proc_numerators']:
            self.assertEqual(damage[key], [0, 0, 0, 0])
        for change in [{'material_item_id': 187}, {'material_quantity': 1}, {'items': [{'item_id': 187, 'quantity': 1}]}]:
            tables = copy.deepcopy(self.tables)
            self.row(tables, 'game_skills', 21)['resource_costs'].update(change)
            with self.subTest(change=change), self.assertRaises(ValueError): self.skill_projection(tables)

    def test_success_effect_is_separate_from_cast_impact_and_nonrecursive(self):
        row = self.row(self.tables, 'game_skills', 21)
        effect = self.row(self.tables, 'game_effects', row['effect_ids'][0])
        self.assertEqual((effect['effect_type'], effect['trigger_phase'], effect['parameters']), ('teleport_success_buff', 'teleport_success', {'buff_id': 21}))
        changes = [{'effect_type': 'defensive_buff'}, {'trigger_phase': 'cast_impact'}, {'target_rule': {'kind': 'friendly_grid'}}, {'proc_limit': {'per_cast': True}}, {'proc_limit': {'per_cast': 2}}, {'recursive_trigger_allowed': True}, {'parameters': {'buff_id': 21, 'duration_ms': 30000}}]
        for change in changes:
            tables = copy.deepcopy(self.tables)
            self.row(tables, 'game_effects', effect['id']).update(change)
            with self.subTest(change=change), self.assertRaises(ValueError): self.skill_projection(tables)
        for refs in [[], [21, 31]]:
            tables = copy.deepcopy(self.tables)
            self.row(tables, 'game_skills', 21)['effect_ids'] = refs
            with self.subTest(refs=refs), self.assertRaises(ValueError): self.skill_projection(tables)

    def test_flux_fixed_duration_is_single_source_and_survives_death(self):
        buff = next(row for row in self.fixture['buffs'] if row['id'] == 21)
        self.assertEqual((buff['duration_ms'], buff['duration_kind'], buff['duration_seconds_by_level']), (30000, 'fixed_ms', []))
        self.assertEqual(buff['value_by_level'], [30, 30, 30, 30])
        self.assertTrue(buff['allow_dead_targets'])
        self.assertFalse(buff['remove_on_player_death'])
        for value in [None, False, 0, -1, 30000.0, 2**31]:
            tables = copy.deepcopy(self.tables)
            self.row(tables, 'game_buffs', 21)['duration_ms'] = value
            with self.subTest(duration_ms=value), self.assertRaises(ValueError): self.buffs(tables)
        for change in [{'kind': 'mc_source_power'}, {'seconds_by_level': [30, 30, 30, 30]}, {'stat_add': 1}, {'stat_multiplier': 1}, {'duration_ms': 30000}]:
            tables = copy.deepcopy(self.tables)
            self.row(tables, 'game_buffs', 21)['duration_rule'].update(change)
            with self.subTest(change=change), self.assertRaises(ValueError): self.buffs(tables)
        for change in [{'remove_events': ['despawn', 'world_dispose', 'player_death']}, {'remove_events': ['despawn', 'scene_dispose']}, {'immunity_rules': {'allow_dead_targets': False, 'friendly_only': True}}]:
            tables = copy.deepcopy(self.tables)
            self.row(tables, 'game_buffs', 21).update(change)
            with self.subTest(change=change), self.assertRaises(ValueError): self.buffs(tables)

    def test_each_map_requires_one_complete_explicit_no_teleport_policy(self):
        profile = self.tables['dev_first_loop_profile'][0]['parameters']
        for map_id in [1, 2]:
            definition = self.row(self.tables, 'game_maps', map_id)
            policy = next(row for row in self.tables['game_map_regions'] if row['map_id'] == map_id and row['entry_rules'].get('region_kind') == 'map_policy')
            self.assertIs(module.map_no_teleport_policy(self.tables, definition, profile), False)
            tables = copy.deepcopy(self.tables)
            self.row(tables, 'game_map_regions', policy['id'])['entry_rules']['no_teleport'] = True
            self.assertIs(module.map_no_teleport_policy(tables, definition, profile), True)
            for change in ['missing', 'duplicate', 'extra_rule', 'monster', 'missing_bool']:
                tables = copy.deepcopy(self.tables)
                actual = self.row(tables, 'game_map_regions', policy['id'])
                if change == 'missing': tables['game_map_regions'].remove(actual)
                if change == 'duplicate': tables['game_map_regions'].append(copy.deepcopy(actual))
                if change == 'extra_rule': actual['entry_rules']['default_no_teleport'] = False
                if change == 'monster': actual['monster_pool'] = [1]
                if change == 'missing_bool': del actual['entry_rules']['no_teleport']
                with self.subTest(map_id=map_id, change=change), self.assertRaises(ValueError): module.map_no_teleport_policy(tables, definition, profile)
            for value in [None, 0, 1, 'false', [], {}]:
                tables = copy.deepcopy(self.tables)
                self.row(tables, 'game_map_regions', policy['id'])['entry_rules']['no_teleport'] = value
                with self.subTest(map_id=map_id, value=value), self.assertRaises(ValueError): module.map_no_teleport_policy(tables, definition, profile)
            for key in ['min_x', 'min_y', 'max_x', 'max_y']:
                tables = copy.deepcopy(self.tables)
                self.row(tables, 'game_map_regions', policy['id'])['region_cells'][key] += 1
                with self.subTest(map_id=map_id, bounds=key), self.assertRaises(ValueError): module.map_no_teleport_policy(tables, definition, profile)

    def test_map_policies_do_not_change_ordered_five_zone_proof_identity(self):
        town = self.fixture['maps'][0]
        self.assertEqual([zone['id'] for zone in town['safe_zones']], [106, 107, 108, 113, 117])
        self.assertEqual(town['safe_zones'][0]['center'], town['safe_zones'][2]['center'])
        identity = {'MapID': town['id'], 'Source': town['source'], 'Zones': town['safe_zones']}
        digest = lambda value: hashlib.sha256(json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode()).hexdigest()
        # 与永久receipt的Go结构JSON顺序一致；仅原5区参与，不加入地图政策或禁传候选。
        self.assertEqual(digest(identity), '0debd388d93e362336368da1d748096f6df109216f63f6c4ff103db21d0f3535')
        tables = copy.deepcopy(self.tables)
        policies = [row for row in tables['game_map_regions'] if row['entry_rules'].get('region_kind') == 'map_policy']
        for row in policies: row['entry_rules']['no_teleport'] = True
        tables['game_map_regions'] = policies + [row for row in tables['game_map_regions'] if row not in policies]
        maps, _ = module.source_map_catalog(tables, tables['dev_first_loop_profile'][0]['parameters'])
        changed = maps[0]
        self.assertTrue(changed['no_teleport'])
        self.assertEqual(digest({'MapID': changed['id'], 'Source': changed['source'], 'Zones': changed['safe_zones']}), digest(identity))

    def test_locked_source_static_bits_differ_from_closed_door_walk_bits(self):
        town = self.fixture['maps'][0]
        source = base64.b64decode(town['source_static_bits'], validate=True)
        walking = base64.b64decode(town['collision_bits'], validate=True)
        manifest = module.read(ROOT/'client/assets/resources/source-maps/map-2/manifest.json')
        self.assertEqual((len(source), sum(byte.bit_count() for byte in source)), (61250, 162653))
        self.assertEqual((len(walking), sum(byte.bit_count() for byte in walking)), (61250, 162741))
        self.assertEqual(hashlib.sha256(source).hexdigest(), '5882b5b3f393a009e46d9f213ebc9fa8f5c2413cbaee0eca0781b4f06a7562d9')
        self.assertEqual(source, base64.b64decode(manifest['collision']['static_bits'], validate=True))
        self.assertEqual(sum((a ^ b).bit_count() for a, b in zip(source, walking)), 88)
        self.assertTrue(all(a & b == a for a, b in zip(source, walking)))
        self.assertEqual(hashlib.sha256((ROOT/'资料参考/myf/Mir200/Map/0.map').read_bytes()).hexdigest(), town['source']['sha256'])

    def test_public_teleport_and_source_flags_are_strict_display_whitelists(self):
        public = module.public_gameplay(self.fixture)
        skill = next(row for row in public['skills'] if row['id'] == 21)
        self.assertEqual(skill['teleport'], {'max_attempts': 200, 'axis_rule': 'map_dimension_divided_by_rank_plus_one', 'landing_rule': 'source_static_walk'})
        self.assertEqual((skill['source_status'], skill['target_180_verified']), ('candidate', False))
        self.assertFalse(skill['auto_enabled'])
        for field in ['allow_occupied', 'allow_accepted_dead', 'check_source_map_at_due', 'origin_binding_source_seq']:
            self.assertNotIn(field, json.dumps(public))
        for row in public['map_catalog']:
            self.assertIs(type(row['no_teleport']), bool)
            self.assertNotIn('source_static_bits', row)
            self.assertNotIn('collision_bits', row)
            self.assertNotIn('source', row)
        flux = next(row for row in public['statuses'] if row['id'] == 21)
        self.assertEqual(flux['duration_rule'], {'kind': 'fixed_ms', 'stat_add': 0, 'stat_multiplier': 0, 'seconds_by_level': [], 'duration_ms': 30000})
        self.assertFalse(flux['remove_on_player_death'])
        self.assertNotIn('allow_dead_targets', flux)
        self.assertNotIn('source_ref', json.dumps(public))


if __name__ == '__main__': unittest.main()
