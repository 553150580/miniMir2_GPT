"""应用root已批字段/升级方案，保留本阶段源前态与候选逐格证明。"""
from pathlib import Path
import json
import shutil
import hashlib

ROOT=Path(__file__).resolve().parents[2]
BASE=Path(__file__).resolve().parent
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
before=json.loads((BASE/'source-before.json').read_text(encoding='utf-8'))
schema_path=ROOT/'config/schema/catalog.json'
schema=json.loads(schema_path.read_text(encoding='utf-8'))
review=json.loads((BASE/'candidate-review.json').read_text(encoding='utf-8'))
assert review['changed_cells']=={'gameplay':82,'architecture':0}
assert review['gameplay_editor_review']['all_unrequested_cell_values_preserved']
assert review['gameplay_editor_review']['all_formula_nodes_preserved']==460
assert sha(schema_path)==before['config/schema/catalog.json']
books=[]
for row in schema['workbooks']:
    path=ROOT/row['path']
    assert sha(path)==before[row['path']]
    shutil.copyfile(path,BASE/(row['group']+'-source-before.xlsx'))
    books.append({'group':row['group'],'path':row['path'],'sha256_before':sha(path)})
shutil.copyfile(schema_path,BASE/'schema-source-before.json')
for row in books:
    if row['group']=='gameplay':shutil.copyfile(BASE/'gameplay-candidate.xlsx',ROOT/row['path'])
    row['sha256_after']=sha(ROOT/row['path'])
    row['changed_cells']=review['changed_cells'][row['group']]
    if row['group']=='architecture':assert row['sha256_after']==row['sha256_before']
shutil.copyfile(BASE/'schema-candidate.json',schema_path)
report={'authorization':'root批准最小唯一字段/两图NoPets/关闭17及171–173升级候选；未启17/护符/商店/赠书',
    'books':books,'schema_before':before['config/schema/catalog.json'],'schema_after':sha(schema_path),
    'candidate_review':'candidate-review.json','cell_diff':'gameplay-cell-diff.json','formulas_preserved':460,
    'preview_sync_not_performed':True}
(BASE/'application-report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps(report,ensure_ascii=False))
