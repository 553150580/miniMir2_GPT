"""将备注纠正后的最终受控差异与最初唯一源前态重新独立核对。"""
from pathlib import Path
import json
import sys
import zipfile
import xml.etree.ElementTree as ET

ROOT=Path(__file__).resolve().parents[2]
BASE=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'tools/config'))
import export_configs as c
import edit_workbook as editor

before=BASE/'gameplay-source-before.xlsx'
source=ROOT/'outputs/development-prep-20261026/玩法配置.xlsx'
allowed={(row['sheet'],row['row'],row['column']) for row in c.load_json(BASE/'gameplay-cell-diff.json')}
old=c.read_xlsx(before);new=c.read_xlsx(source)
diff=[]
for sheet in set(old)|set(new):
    for row in set(old.get(sheet,{}))|set(new.get(sheet,{})):
        a=old.get(sheet,{}).get(row,{});b=new.get(sheet,{}).get(row,{})
        for column in set(a)|set(b):
            if a.get(column)!=b.get(column):
                assert (sheet,row,column) in allowed
                diff.append({'sheet':sheet,'row':row,'column':column,'before':a.get(column),'after':b.get(column)})
assert len(diff)==82
def formulas(path):
    result={}
    with zipfile.ZipFile(path) as archive:
        for name in archive.namelist():
            if name.startswith('xl/worksheets/') and name.endswith('.xml'):
                for cell in ET.fromstring(archive.read(name)).iter('{'+editor.MAIN+'}c'):
                    value=cell.find('{'+editor.MAIN+'}f')
                    if value is not None:result[(name,cell.attrib['r'])]=ET.tostring(value,encoding='unicode')
    return result
assert formulas(before)==formulas(source)
with zipfile.ZipFile(before) as a,zipfile.ZipFile(source) as b:
    assert set(a.namelist())==set(b.namelist())
    changed=[name for name in a.namelist() if a.read(name)!=b.read(name)]
    assert len(changed)==6 and all(name.startswith('xl/worksheets/') for name in changed)
    untouched=len(a.namelist())-len(changed)
architecture=ROOT/'outputs/development-prep-20261026/架构配置.xlsx'
assert c.sha256(architecture)==c.sha256(BASE/'architecture-source-before.xlsx')
(BASE/'final-gameplay-cell-diff.json').write_text(json.dumps(diff,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
result={'final_book_sha256':c.sha256(source),'original_book_sha256':c.sha256(before),'actual_changed_cells':82,
    'all_unrequested_values_preserved':True,'all_formula_nodes_preserved':len(formulas(source)),
    'changed_zip_members':changed,'unchanged_zip_member_count':untouched,'architecture_unchanged':True,
    'metadata_correction':'metadata-correction-report.json；仅修正旧备注，不改候选数值或范围'}
(BASE/'final-source-review.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps(result,ensure_ascii=False))
