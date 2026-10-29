"""仅生成两Excel/schema候选与逐格差异，绝不替换唯一源表。"""
from pathlib import Path
from copy import deepcopy
import hashlib
import json
import sys
import zipfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
BASE = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT/'tools/config'))
sys.path.insert(0, str(ROOT/'tools/resources'))
import export_configs as config
import edit_workbook as editor
import prepare_pet_runtime as pets
import prepare_first_loop as loop

SOURCE = '本任务rule16开发候选；资料索引/职业宠物补充来源契约.json；Crystal MonsterObject.cs:811–833,3502–3515；PlayerObject.cs:895–899；Shared/Globals.cs:37'
LIFETIME_SOURCE = '本任务选择BoneFamiliar/AI23行为的跨MYF候选；Crystal MonsterObject.cs:584–599,689–704,734–736,966–1005,1205–1215；MapObject.cs:358–375；不认证MYF Race100等于AI23'


def save(name, value):
    path = BASE/name
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2)+'\n', encoding='utf-8', newline='\n')


def shape(value):
    if value is None: return {'type': 'null'}
    if type(value) is bool: return {'type': 'boolean'}
    if type(value) is int: return {'type': 'int', 'minimum': 0}
    if isinstance(value, str): return {'type': 'string'}
    if isinstance(value, list):
        assert value
        return {'type': 'array', 'items': shape(value[0])}
    return {'type': 'object', 'properties': {key: shape(item) for key, item in value.items()}}


def main():
    schema = config.load_json(ROOT/'config/schema/catalog.json')
    before = {w['group']: config.sha256(ROOT/w['path']) for w in schema['workbooks']}
    tables, errors, _, _ = config.collect(schema)
    assert not errors, errors
    assert tables['dev_first_loop_profile'][0]['parameters']['rule_version'] == 15
    assert not tables['game_pet_growth']
    old_fixture = loop.fixture_from_rows(tables)
    after = deepcopy(tables)
    template = after['game_pet_templates'][0]
    assert template['id'] == 1 and template['summon_skill_id'] is None and template['monster_template_id'] == 2
    template['base_attributes'] = {'kind': 'inherit_monster_template', 'owner_job_id': 2,
        'neutral_attributes': {key: 0 for key in sorted(pets.NEUTRAL_FIELDS)}, 'magic_resist_weight_source': 'profile_player_shared'}
    template['follow_and_return'] = {'source_skill_id': 17, 'training': {
        'initial_level_by_rank': [0, 1, 2, 3], 'maximum_level_by_rank': [4, 5, 6, 7],
        'experience_multiplier': 3, 'owner_range_cells': 16, 'experience_basis': 'owner_actual_granted_experience',
        'grant_phase': 'confirmed_owner_experience', 'eligibility': 'alive_same_world_within_owner_range',
        'max_level_ups_per_grant': 1, 'excess_experience': 'keep', 'at_maximum': 'do_not_accumulate', 'current_hp_on_growth': 'keep'},
        'speed': {'basis': 'maximum_training_level', 'move_reduction_ms_per_level': 130, 'attack_reduction_ms_per_level': 70,
                  'minimum_move_interval_ms': 400, 'minimum_attack_interval_ms': 400}}
    template['lifetime_and_betrayal'] = {'corpse_ttl_ms': 180000, 'spawn_ready_ms': 2000,
        'live_expiry': 'no_tame_expiry', 'cold_enter': 'do_not_restore_temporary_pet'}
    template['source_ref'] = SOURCE+'；'+LIFETIME_SOURCE+'；MYF Monster.csv:381基础模板由monster_template_id唯一引用'
    template['note'] = 'rule16仅开启World身体/尸体生命周期；legacy_untrained_pet保留原免费bootstrap属性且不训练。SourceSkillID17仅来源身份，summon_skill_id仍空，17未选入、护符未开；新生或隔离训练另按typed规则。'
    gameplay_ops = [{'kind': 'update', 'table_id': 'game_pet_templates', 'id': 1,
                     'values': {key: template[key] for key in ['base_attributes', 'follow_and_return', 'lifetime_and_betrayal', 'source_ref', 'note']}}]
    for level in range(8):
        growth = {'id': level+1, 'pet_id': 1, 'growth_level': level,
            'attributes': {'hp_bonus': level*20, 'attack_min_bonus': level, 'attack_max_bonus': level,
                           'defence_min_bonus': level*2, 'defence_max_bonus': level*2,
                           'magic_defence_min_bonus': level*2, 'magic_defence_max_bonus': level*2},
            'required_conditions': {'required_experience': None if level == 7 else (level+1)*20000},
            'record_status': 'candidate', 'source_ref': SOURCE,
            'note': '属性只保存相对monster基础的增量；阈值只在本表维护，倍率及资格只在PetDefs维护；仅终态7阈值明确null，不从模板Level15推断训练等级。'}
        after['game_pet_growth'].append(growth)
        gameplay_ops.append({'kind': 'append', 'table_id': 'game_pet_growth', 'id': growth['id'], 'values': growth})
    profile = after['dev_first_loop_profile'][0]
    profile['parameters']['rule_version'] = 16
    profile['parameters']['pet_runtime'] = {'enabled': True, 'bootstrap_mode': 'legacy_untrained_pet', 'definition_ids': [1]}
    profile['parameters']['technical_parameters_notice'] += '；rule16同owner保留身体/尸体有界；独立训练定义来自PetDefs/Growth，原免费bootstrap不训练，17与护符仍关闭，不认证容量。'
    profile['source_ref'] += '；本任务rule16宠物驻留/身体/训练候选，数值由PetDefs/Growth及架构预算共同维护'
    profile['note'] += ' rule16仅旧免费bootstrap的原对象死亡/尸体生命周期，正常17材料召唤仍待用户护符单位决定；不新增初始书。'
    gameplay_ops.append({'kind': 'update', 'table_id': 'dev_first_loop_profile', 'id': 1, 'values': {key: profile[key] for key in ['parameters', 'source_ref', 'note']}})
    budget = {'id': 40, 'record_status': 'candidate', 'source_ref': '本任务rule16技术候选；docs/职业宠物生命周期与召唤实施边界.md',
        'note': '包括同owner全部World的活动宠、尸体及待新生reservation；满额只拒新生不拒死亡，不丢弃未过期原尸体。101仅180s/1800ms典型源路径估计，不证明通用上界或整机容量。',
        'code': pets.CAPACITY_CODE, 'value': 128, 'value_unit': '身体/owner',
        'applicability': '同owner全部保留World的宠物身体与新生预留', 'adaptation_status': 'adapt', 'verification_status': 'pending'}
    assert all(row['id'] != 40 and row['code'] != pets.CAPACITY_CODE for row in tables['arch_runtime_budgets'])
    after['arch_runtime_budgets'].append(budget)
    operations = {'gameplay': gameplay_ops, 'architecture': [{'kind': 'append', 'table_id': 'arch_runtime_budgets', 'id': 40, 'values': budget}]}
    candidate_schema = deepcopy(schema)
    replaced, added = [], []
    candidate_schema['json_contracts'].append({'table_id': 'arch_runtime_budgets', 'id': 40, 'field': 'value', 'rule_key': None,
        'shape': {'type': 'int', 'minimum': 1, 'maximum': 128}, 'source_ref': 'rule16软件支持上限；数据仅由架构Excel预算40维护'})
    added.append(('arch_runtime_budgets', 40, 'value'))
    for field in ['base_attributes', 'follow_and_return', 'lifetime_and_betrayal']:
        key = ('game_pet_templates', 1, field)
        contract = {'table_id': key[0], 'id': key[1], 'field': field, 'rule_key': None, 'shape': shape(template[field]), 'source_ref': template['source_ref']}
        index = next((i for i, row in enumerate(candidate_schema['json_contracts']) if (row['table_id'], row['id'], row['field']) == key), None)
        if index is None:
            candidate_schema['json_contracts'].append(contract); added.append(key)
        else:
            candidate_schema['json_contracts'][index] = contract; replaced.append(key)
    for row in after['game_pet_growth']:
        for field in ['attributes', 'required_conditions']:
            contract = {'table_id': 'game_pet_growth', 'id': row['id'], 'field': field, 'rule_key': None, 'shape': shape(row[field]), 'source_ref': SOURCE}
            candidate_schema['json_contracts'].append(contract); added.append(('game_pet_growth', row['id'], field))
    contract = next(row for row in candidate_schema['json_contracts'] if row['table_id'] == 'dev_first_loop_profile' and row['id'] == 1 and row['field'] == 'parameters')
    contract['shape']['properties']['pet_runtime'] = shape(profile['parameters']['pet_runtime'])
    contract['source_ref'] += '；本任务rule16仅选择PetRuntime定义及bootstrap模式，训练/尸体数值不进profile'
    replaced.append(('dev_first_loop_profile', 1, 'parameters'))
    errors = config.check_json_contracts(candidate_schema, after)+config.semantic_checks(after)
    assert not errors, errors
    output_paths, review = {}, {}
    for book in schema['workbooks']:
        group = book['group']
        plan = {'workbook': book['path'], 'group': group, 'input_workbook_sha256': before[group], 'operations': operations[group]}
        save(group+'-plan.json', plan)
        output = BASE/(group+'-candidate.xlsx')
        review[group] = editor.apply(BASE/(group+'-plan.json'), output)
        save(group+'-editor-review.json', review[group])
        output_paths[group] = output
        # 对全部实际单元格值做独立diff，只允许编辑器列明的格变化；公式缓存之外原公式字符串另由XML核对。
        original = config.read_xlsx(ROOT/book['path']); staged = config.read_xlsx(output)
        allowed=set()
        table_by_id={row['table_id']:row for row in schema['tables']}
        for op in operations[group]:
            table=table_by_id[op['table_id']];sheet=table['sheet_name']
            number=next(n for n,cells in staged[sheet].items() if n>=4 and cells.get(1)==op['id'])
            for key in op['values']:
                allowed.add((sheet,number,next(i+1 for i,field in enumerate(table['fields']) if field['name']==key)))
        all_cell_changes=[]
        for sheet in set(original)|set(staged):
            for number in set(original.get(sheet,{}))|set(staged.get(sheet,{})):
                old_cells=original.get(sheet,{}).get(number,{});new_cells=staged.get(sheet,{}).get(number,{})
                for column in set(old_cells)|set(new_cells):
                    if old_cells.get(column)!=new_cells.get(column):
                        assert (sheet,number,column) in allowed,(sheet,number,column)
                        all_cell_changes.append((sheet,number,column))
        def formulas(path):
            found={}
            with zipfile.ZipFile(path) as archive:
                for name in archive.namelist():
                    if name.startswith('xl/worksheets/') and name.endswith('.xml'):
                        for cell in ET.fromstring(archive.read(name)).iter('{'+editor.MAIN+'}c'):
                            formula=cell.find('{'+editor.MAIN+'}f')
                            if formula is not None:found[(name,cell.attrib['r'])]=ET.tostring(formula,encoding='unicode')
            return found
        assert formulas(ROOT/book['path'])==formulas(output),'原公式文本/属性发生变化'
        review[group]['all_unrequested_cell_values_preserved']=True
        review[group]['all_formula_nodes_preserved']=len(formulas(output))
        review[group]['full_workbook_changed_cell_count']=len(all_cell_changes)
        changes = []
        requested = {(op['table_id'], op['id'], key) for op in operations[group] for key in op['values']}
        for table in [row for row in schema['tables'] if row['workbook_group'] == group]:
            sheet = table['sheet_name']
            old = {cells.get(1): cells for n, cells in original[sheet].items() if n >= 4 and cells.get(1) is not None}
            new = {cells.get(1): cells for n, cells in staged[sheet].items() if n >= 4 and cells.get(1) is not None}
            for identifier, cells in new.items():
                for column, field in enumerate(table['fields'], 1):
                    a, b = old.get(identifier, {}).get(column), cells.get(column)
                    if a != b:
                        assert (table['table_id'], identifier, field['name']) in requested, (table['table_id'], identifier, field['name'])
                        changes.append({'table_id': table['table_id'], 'id': identifier, 'field': field['name'], 'before': a, 'after': b})
            assert set(old).issubset(new), '不能删除其它记录'
        save(group+'-cell-diff.json', changes)
        review[group]['actual_changed_cells'] = len(changes)
    save('schema-candidate.json', candidate_schema)
    schema_for_staged = deepcopy(candidate_schema)
    for book in schema_for_staged['workbooks']:
        book['path'] = output_paths[book['group']].relative_to(ROOT).as_posix()
    staged_tables, errors, _, _ = config.collect(schema_for_staged)
    errors += config.check_json_contracts(candidate_schema, staged_tables)+config.semantic_checks(staged_tables)
    assert not errors, errors
    catalog = pets.build_catalog(staged_tables, {'stage': 'review_candidate_only', 'workbooks': {group: config.sha256(path) for group, path in output_paths.items()}})
    fixture = loop.fixture_from_rows(staged_tables)
    save('pet-runtime-candidates.json', catalog)
    save('first-loop-candidate.json', fixture)
    assert fixture['pet'] == old_fixture['pet'], '不能改变旧免费bootstrap属性'
    assert fixture['skills'] == old_fixture['skills'], '不能开放17或改变既有技能'
    assert fixture['casting_supply'] == old_fixture['casting_supply']
    assert fixture['initial_skill_books'] == old_fixture['initial_skill_books']
    stable = all(config.sha256(ROOT/book['path']) == before[book['group']] for book in schema['workbooks'])
    assert stable and config.load_json(ROOT/'config/schema/catalog.json') == schema
    save('candidate-review.json', {'source_books_unchanged': stable, 'source_schema_unchanged': True,
        'input_book_sha256': before, 'editor_reviews': review, 'json_contracts_added': added, 'json_contracts_replaced': replaced,
        'stage_semantic_checks': 'PASS', 'runtime_enabled_in_independent_catalog': False, 'fixture_rule_version': fixture['rule_version'],
        'legacy_bootstrap_fighter_unchanged': True, 'skills_22_and_supply_unchanged': True,
        'pending': 'root逐格审阅后才应用唯一book/schema；CLI最终生成与--check待应用后执行'})
    print(json.dumps({'candidate_only': True, 'source_inputs_unchanged': stable, 'actual_changed_cells': {key: value['actual_changed_cells'] for key, value in review.items()}, 'rule_version': fixture['rule_version']}))


if __name__ == '__main__':
    main()
