"""仅审证并应用唯一开发profile的rule20→21；不改变其它玩法数值。"""
from copy import deepcopy
from datetime import datetime, timezone
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import xml.etree.ElementTree as ET
import zipfile

ROOT = Path(__file__).resolve().parents[3]
WORK = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT / 'tools/config'))
import export_configs as config
import edit_workbook


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def cells_and_formulas(path):
    ns = {'s': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
    cells, formulas = {}, {}
    with zipfile.ZipFile(path) as archive:
        for name in archive.namelist():
            if name.startswith('xl/worksheets/') and name.endswith('.xml'):
                for cell in ET.fromstring(archive.read(name)).findall('.//s:c', ns):
                    key = (name, cell.attrib['r'])
                    cells[key] = ET.tostring(cell, encoding='unicode')
                    if cell.find('s:f', ns) is not None:
                        formulas[key] = cells[key]
    return cells, formulas


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--apply', action='store_true')
    args = parser.parse_args()
    schema = config.load_json(ROOT / 'config/schema/catalog.json')
    books = {w['group']: ROOT / w['path'] for w in schema['workbooks']}
    book, architecture = books['gameplay'], books['architecture']
    baseline_path = WORK / 'baseline.json'
    if not baseline_path.exists():
        files = [book, architecture, ROOT / 'config/schema/catalog.json',
                 ROOT / 'tools/resources/prepare_summon_candidates.py', ROOT / 'tools/resources/prepare_first_loop.py',
                 ROOT / 'tools/resources/prepare_pet_runtime.py', ROOT / 'config/development/first-loop.json',
                 ROOT / 'client/assets/resources/development/gameplay.json', ROOT / 'config/development/pet-runtime-candidates.json',
                 ROOT / 'config/development/summon-candidates.json', ROOT / 'config/releases/preview_current.json']
        files += [ROOT / 'tools/config' / name for name in ['test_level_growth.py', 'test_equipment_recycling.py',
                  'test_safe_zone_binding.py', 'test_pet_runtime.py', 'test_summon_candidates.py', 'test_teleport.py']]
        files += [ROOT / 'server/internal/devconfig' / name for name in ['map_catalog_test.go', 'pet_runtime_test.go', 'summon_test.go']]
        for path in files:
            target = WORK / 'before' / path.relative_to(ROOT)
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(path, target)
        save(baseline_path, {'head': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
             'recorded_at_utc': datetime.now(timezone.utc).isoformat(),
             'files': {p.relative_to(ROOT).as_posix(): sha(p) for p in files}})
    baseline = config.load_json(baseline_path)
    original = WORK / 'before' / book.relative_to(ROOT)
    assert sha(book) == baseline['files'][book.relative_to(ROOT).as_posix()], '唯一书已漂移，禁止重复套用'
    tables, errors, _, _ = config.collect(schema)
    assert not errors, errors
    old_profile = deepcopy(tables['dev_first_loop_profile'][0]['parameters'])
    assert old_profile['rule_version'] == 20
    updated = deepcopy(old_profile)
    updated['rule_version'] = 21
    plan = {'schema_version': 1, 'group': 'gameplay', 'workbook': book.relative_to(ROOT).as_posix(),
            'input_workbook_sha256': sha(book), 'operations': [{'kind': 'update', 'table_id': 'dev_first_loop_profile',
            'id': 1, 'values': {'parameters': updated}}]}
    save(WORK / 'edit-plan.json', plan)
    candidate = WORK / '玩法配置.candidate.xlsx'
    if not candidate.exists():
        save(WORK / 'editor-review.json', edit_workbook.apply(WORK / 'edit-plan.json', candidate))
    before, after = config.read_xlsx(original), config.read_xlsx(candidate)
    diff = []
    for sheet in set(before) | set(after):
        for row in set(before.get(sheet, {})) | set(after.get(sheet, {})):
            left, right = before.get(sheet, {}).get(row, {}), after.get(sheet, {}).get(row, {})
            for col in set(left) | set(right):
                if left.get(col) != right.get(col):
                    diff.append({'sheet': sheet, 'row': row, 'column': col, 'before': left.get(col), 'after': right.get(col)})
    assert len(diff) == 1 and (diff[0]['sheet'], diff[0]['row'], diff[0]['column']) == ('开发首循环夹具', 4, 3)
    candidate_profile = json.loads(diff[0]['after'])
    assert candidate_profile == updated
    candidate_profile['rule_version'] = 20
    assert candidate_profile == old_profile, '除rule_version外不得改变profile'
    a, fa = cells_and_formulas(original)
    b, fb = cells_and_formulas(candidate)
    assert fa == fb and len(fa) == 460
    raw_cell_diffs = [key for key in set(a) | set(b) if a.get(key) != b.get(key)]
    assert len(raw_cell_diffs) == 1 and raw_cell_diffs[0][1] == 'C4'
    with zipfile.ZipFile(original) as left, zipfile.ZipFile(candidate) as right:
        assert left.namelist() == right.namelist()
        changed = [name for name in left.namelist() if left.read(name) != right.read(name)]
        assert len(changed) == 1 and changed[0] == raw_cell_diffs[0][0]
        unchanged = len(left.namelist()) - len(changed)
        assert unchanged == 102
    assert sha(architecture) == baseline['files'][architecture.relative_to(ROOT).as_posix()]
    review = {'status': 'candidate_reviewed_not_applied', 'source_before_sha256': sha(original),
              'candidate_sha256': sha(candidate), 'changed_cells': 1, 'differences': diff,
              'only_profile_rule_20_to_21': True, 'all_other_cells_and_styles_unchanged': True,
              'formulas_preserved': len(fa), 'changed_zip_members': changed, 'untouched_zip_members_verified': unchanged,
              'architecture_workbook_preserved': True, 'schema_unchanged': sha(ROOT / 'config/schema/catalog.json') == baseline['files']['config/schema/catalog.json']}
    save(WORK / 'candidate-review.json', review)
    if args.apply:
        shutil.copyfile(candidate, book)
        assert sha(book) == sha(candidate)
        save(WORK / 'application.json', {'applied': True, 'before_sha256': sha(original), 'after_sha256': sha(book),
             'candidate_review': 'candidate-review.json', 'only_reviewed_cell_applied': True})
    print(json.dumps({**{k: review[k] for k in ['status', 'changed_cells', 'formulas_preserved', 'untouched_zip_members_verified', 'candidate_sha256']}, 'applied': args.apply}, ensure_ascii=False))


if __name__ == '__main__':
    main()
