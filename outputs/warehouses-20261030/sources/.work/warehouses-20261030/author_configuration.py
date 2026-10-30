"""仅升级唯一表开发profile；仓库既定容量、价格及其状态保持原值。"""
from copy import deepcopy
from datetime import datetime, timezone
from pathlib import Path
import argparse
import hashlib
import json
import shutil
import subprocess
import sys
import xml.etree.ElementTree as ET
import zipfile

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
BASE = 'd2dab71909d4c389482be9b95d9594db76310eeb'
sys.path.insert(0, str(ROOT / 'tools/config'))
import export_configs as config
import edit_workbook

NOTICE = ('；本任务rule23双仓储：容量与个人扩页价格只读取角色与仓库规则的已定原行；'
          'query_page_limit=30仅为有界查询技术候选，不限制个人总页数；'
          '共享按本角色当前特权及永久account+origin校验；不赠特权或新增非绑元宝')


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def formulas(path):
    ns = {'m': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
    result = {}
    with zipfile.ZipFile(path) as archive:
        for name in archive.namelist():
            if name.startswith('xl/worksheets/') and name.endswith('.xml'):
                for cell in ET.fromstring(archive.read(name)).findall('.//m:c', ns):
                    if cell.find('m:f', ns) is not None:
                        result[(name, cell.attrib['r'])] = ET.tostring(cell, encoding='unicode')
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    assert subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip() == BASE
    schema_path = ROOT / 'config/schema/catalog.json'
    schema = config.load_json(schema_path)
    books = {row['group']: ROOT / row['path'] for row in schema['workbooks']}
    book = books['gameplay']
    baseline_path = WORK / 'configuration-baseline.json'
    assert not baseline_path.exists(), '拒绝覆盖既有配置编辑记录'
    names = [path.relative_to(ROOT).as_posix() for path in books.values()]
    names += ['config/schema/catalog.json', 'tools/resources/prepare_first_loop.py',
              'tools/resources/prepare_summon_candidates.py', 'config/development/first-loop.json',
              'client/assets/resources/development/gameplay.json', 'config/releases/preview_current.json', '.gitattributes']
    before = {}
    for name in names:
        target = WORK / 'before' / name
        target.parent.mkdir(parents=True, exist_ok=True)
        raw = subprocess.check_output(['git', 'show', BASE + ':' + name], cwd=ROOT)
        # 原始工作簿及schema必须仍与阶段22完全一致；并行作者尚未改本任务配置输入。
        assert (ROOT / name).read_bytes() == raw, name
        target.write_bytes(raw)
        before[name] = sha(target)
    save(baseline_path, {'head': BASE, 'recorded_at_utc': datetime.now(timezone.utc).isoformat(), 'files': before})
    tables, errors, _, _ = config.collect(schema)
    assert not errors, errors
    original = tables['dev_first_loop_profile'][0]
    profile = deepcopy(original['parameters'])
    assert profile['rule_version'] == 22 and 'warehouses' not in profile
    profile.update(rule_version=23, warehouses={'enabled': True, 'query_page_limit': 30})
    source_ref = original['source_ref'] + NOTICE
    operation = {'kind': 'update', 'table_id': 'dev_first_loop_profile', 'id': 1,
                 'values': {'parameters': profile, 'source_ref': source_ref}}
    plan = {'schema_version': 1, 'group': 'gameplay', 'workbook': book.relative_to(ROOT).as_posix(),
            'input_workbook_sha256': sha(book), 'operations': [operation]}
    save(WORK / 'configuration-edit-plan.json', plan)
    candidate = WORK / '玩法配置.candidate.xlsx'
    assert not candidate.exists()
    save(WORK / 'configuration-editor-report.json', edit_workbook.apply(WORK / 'configuration-edit-plan.json', candidate))
    updated_schema = deepcopy(schema)
    contract = next(row for row in updated_schema['json_contracts']
                    if row['table_id'] == 'dev_first_loop_profile' and row['field'] == 'parameters')
    contract['shape']['properties']['warehouses'] = {'type': 'object', 'properties': {
        'enabled': {'type': 'boolean'},
        'query_page_limit': {'type': 'int', 'minimum': 1, 'maximum': 2**32-1}}}
    contract['source_ref'] += NOTICE
    save(WORK / 'catalog.candidate.json', updated_schema)
    candidate_schema = deepcopy(updated_schema)
    next(row for row in candidate_schema['workbooks'] if row['group'] == 'gameplay')['path'] = candidate.relative_to(ROOT).as_posix()
    updated, errors, _, _ = config.collect(candidate_schema)
    errors += config.check_json_contracts(updated_schema, updated) + config.semantic_checks(updated)
    assert not errors, errors
    for table_id, rows in tables.items():
        expected = deepcopy(rows)
        if table_id == 'dev_first_loop_profile':
            expected[0].update(operation['values'])
        assert updated[table_id] == expected, table_id
    old_cells, new_cells = config.read_xlsx(book), config.read_xlsx(candidate)
    differences = []
    for sheet in set(old_cells) | set(new_cells):
        for row in set(old_cells.get(sheet, {})) | set(new_cells.get(sheet, {})):
            left, right = old_cells.get(sheet, {}).get(row, {}), new_cells.get(sheet, {}).get(row, {})
            for column in set(left) | set(right):
                if left.get(column) != right.get(column):
                    differences.append({'sheet': sheet, 'row': row, 'column': column,
                                        'before': left.get(column), 'after': right.get(column)})
    assert {(row['sheet'], row['row'], row['column']) for row in differences} == {('开发首循环夹具', 4, 3), ('开发首循环夹具', 4, 5)}
    old_formulas = formulas(book)
    assert old_formulas == formulas(candidate) and len(old_formulas) == 460
    with zipfile.ZipFile(book) as original_zip, zipfile.ZipFile(candidate) as candidate_zip:
        assert original_zip.namelist() == candidate_zip.namelist()
        changed = [name for name in original_zip.namelist() if original_zip.read(name) != candidate_zip.read(name)]
        assert len(changed) == 1 and changed[0].startswith('xl/worksheets/')
        unchanged_members = len(original_zip.namelist()) - len(changed)
    report = {'status': 'reviewed', 'head': BASE, 'changed_cells': differences, 'formulas_preserved': len(old_formulas),
              'changed_zip_members': changed, 'unchanged_zip_members': unchanged_members,
              'existing_warehouse_rules_preserved': True, 'architecture_workbook_preserved': True,
              'candidate_sha256': sha(candidate), 'schema_candidate_sha256': sha(WORK / 'catalog.candidate.json'),
              'applied': args.apply}
    if args.apply:
        shutil.copyfile(candidate, book)
        shutil.copyfile(WORK / 'catalog.candidate.json', schema_path)
        assert sha(book) == report['candidate_sha256'] and sha(schema_path) == report['schema_candidate_sha256']
        assert sha(books['architecture']) == before[books['architecture'].relative_to(ROOT).as_posix()]
    save(WORK / 'configuration-review.json', report)
    print(json.dumps({key: report[key] for key in ['status', 'applied', 'formulas_preserved', 'unchanged_zip_members', 'existing_warehouse_rules_preserved']}, ensure_ascii=False))


if __name__ == '__main__':
    main()
