"""应用root审过的候选，并留存唯一输入替换前后摘要；不触碰预览导出。"""
from pathlib import Path
import hashlib
import json
import shutil
import zipfile

ROOT=Path(__file__).resolve().parents[2]
BASE=Path(__file__).resolve().parent
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()

def main():
    schema_path=ROOT/'config/schema/catalog.json'
    schema=json.loads(schema_path.read_text(encoding='utf-8'))
    reviewed=json.loads((BASE/'candidate-review.json').read_text(encoding='utf-8'))
    target_schema=json.loads((BASE/'schema-candidate.json').read_text(encoding='utf-8'))
    assert reviewed['source_books_unchanged'] and reviewed['source_schema_unchanged']
    inputs=[]
    for book in schema['workbooks']:
        group=book['group'];source=ROOT/book['path'];candidate=BASE/(group+'-candidate.xlsx')
        assert sha(source)==reviewed['input_book_sha256'][group],group+'真实源表已变化，须重审'
        expected=reviewed['editor_reviews'][group]
        assert expected['actual_changed_cells']=={'gameplay':72,'architecture':10}[group]
        assert expected['all_unrequested_cell_values_preserved']
        with zipfile.ZipFile(source) as old,zipfile.ZipFile(candidate) as new:
            assert set(old.namelist())==set(new.namelist())
            changed=[name for name in old.namelist() if old.read(name)!=new.read(name)]
            assert all(name.startswith('xl/worksheets/') for name in changed),changed
            preserved=len(old.namelist())-len(changed)
        backup=BASE/(group+'-source-before.xlsx')
        shutil.copyfile(source,backup)
        inputs.append({'group':group,'path':book['path'],'source_before_sha256':sha(source),'candidate_sha256':sha(candidate),
            'changed_cells':expected['actual_changed_cells'],'unchanged_zip_member_count':preserved,
            'changed_zip_members':changed,'all_original_formula_nodes_preserved':expected['all_formula_nodes_preserved']})
    schema_before=sha(schema_path)
    shutil.copyfile(schema_path,BASE/'schema-source-before.json')
    for row in inputs:
        source=ROOT/row['path']
        shutil.copyfile(BASE/(row['group']+'-candidate.xlsx'),source)
        row['source_after_sha256']=sha(source)
        assert row['source_after_sha256']==row['candidate_sha256']
    shutil.copyfile(BASE/'schema-candidate.json',schema_path)
    result={'authorization':'root已审82cells后授权应用；maximum负测14项通过后执行','workbooks':inputs,
        'schema_before_sha256':schema_before,'schema_after_sha256':sha(schema_path),
        'schema_json_contracts_added':reviewed['json_contracts_added'],'schema_json_contracts_replaced':reviewed['json_contracts_replaced'],
        'preview_current_untouched':True,'other_cells_and_formula_proof':'candidate-review.json及两份cell-diff/editor-review'}
    (BASE/'application-report.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps(result,ensure_ascii=False))

if __name__=='__main__':main()
