"""只在全部单元格和ZIP保留校验通过后应用已授权的rule18单格候选。"""
import hashlib
import json
from pathlib import Path
import shutil
import sys
import zipfile
from xml.etree import ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT / 'tools/config'))
import export_configs as config


def sha(path): return hashlib.sha256(path.read_bytes()).hexdigest()


schema = config.load_json(ROOT / 'config/schema/catalog.json')
source = ROOT / next(w['path'] for w in schema['workbooks'] if w['group'] == 'gameplay')
candidate = OUT / 'gameplay-candidate.xlsx'
plan = json.loads((OUT / 'gameplay-plan.json').read_text(encoding='utf-8'))
assert sha(source) == plan['input_workbook_sha256']
before, after = config.read_xlsx(source), config.read_xlsx(candidate)
assert before.keys() == after.keys()
differences = []
for sheet in before:
    assert before[sheet].keys() == after[sheet].keys()
    for row in before[sheet]:
        assert before[sheet][row].keys() == after[sheet][row].keys()
        for column in before[sheet][row]:
            old, new = before[sheet][row][column], after[sheet][row][column]
            if old != new:
                old_json, new_json = json.loads(old), json.loads(new)
                assert old_json['rule_version'] == 17 and new_json['rule_version'] == 18
                new_json['rule_version'] = 17
                assert old_json == new_json
                differences.append({'sheet': sheet, 'row': row, 'column': column, 'only_json_path': 'rule_version', 'before': 17, 'after': 18})
assert len(differences) == 1
with zipfile.ZipFile(source) as a, zipfile.ZipFile(candidate) as b:
    assert a.namelist() == b.namelist()
    changed = [name for name in a.namelist() if a.read(name) != b.read(name)]
    assert changed == ['xl/worksheets/sheet95.xml']
    formula = lambda z: [(name, ET.tostring(node, encoding='unicode')) for name in z.namelist() if name.startswith('xl/worksheets/') and name.endswith('.xml') for node in ET.fromstring(z.read(name)).iter('{http://schemas.openxmlformats.org/spreadsheetml/2006/main}f')]
    assert formula(a) == formula(b)
    formulas = len(formula(a))
    untouched = len(a.namelist()) - len(changed)
old_sha = sha(source)
shutil.copyfile(candidate, source)
assert sha(source) == sha(candidate)
report = {'input_sha256': old_sha, 'output_sha256': sha(source), 'all_other_cell_values_preserved': True, 'cell_differences': differences, 'formula_count': formulas, 'all_formulas_preserved': True, 'untouched_zip_members': untouched, 'changed_zip_members': changed, 'architecture_workbook_unchanged': True}
(OUT / 'workbook-apply-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps(report, ensure_ascii=False))
