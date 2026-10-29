"""审证rule22最小唯一表候选；仅应用显式请求的六个单元格和JSON契约。"""
from copy import deepcopy
from datetime import datetime, timezone
import argparse
import csv
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import xml.etree.ElementTree as ET
import zipfile

ROOT=Path(__file__).resolve().parents[3]
WORK=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'tools/config'))
import export_configs as config
import edit_workbook

NEW_ATTRIBUTES={'magic_attack_min':'Mc','magic_attack_max':'Mc2','spirit_attack_min':'Sc','spirit_attack_max':'Sc2'}
SOURCE_NOTE='；rule22基础装备属性：原表Mc/Mc2/Sc/Sc2逐列均为0，显式投影魔法攻击/道术攻击上下限；不证明完整词条/外观/持久系统'
PROFILE_NOTE='；本任务rule22装备基础合同：base_item_ids只选择原4物品；MC/SC来自唯一装备行；装备阶段按floor((required_level−1)/10)校验；不选入乌木剑220，不新增回收价或外观'

def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def save(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')

def cells_and_formulas(path):
    ns={'s':'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
    cells,formulas={},{}
    with zipfile.ZipFile(path) as archive:
        for name in archive.namelist():
            if name.startswith('xl/worksheets/') and name.endswith('.xml'):
                for cell in ET.fromstring(archive.read(name)).findall('.//s:c',ns):
                    key=(name,cell.attrib['r']);cells[key]=ET.tostring(cell,encoding='unicode')
                    if cell.find('s:f',ns) is not None:formulas[key]=cells[key]
    return cells,formulas

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--apply',action='store_true');args=parser.parse_args()
    schema_path=ROOT/'config/schema/catalog.json';schema=config.load_json(schema_path)
    books={w['group']:ROOT/w['path'] for w in schema['workbooks']};book=books['gameplay'];architecture=books['architecture']
    baseline_path=WORK/'baseline.json'
    if not baseline_path.exists():
        names=[w['path'] for w in schema['workbooks']]+['config/schema/catalog.json','tools/resources/prepare_first_loop.py','tools/resources/prepare_summon_candidates.py','tools/resources/prepare_pet_runtime.py',
            'config/development/first-loop.json','client/assets/resources/development/gameplay.json','config/development/pet-runtime-candidates.json','config/development/summon-candidates.json','config/releases/preview_current.json']
        names += ['tools/config/'+n for n in ['test_level_growth.py','test_equipment_recycling.py','test_safe_zone_binding.py','test_pet_runtime.py','test_summon_candidates.py','test_teleport.py']]
        names += ['server/internal/devconfig/'+n for n in ['map_catalog_test.go','pet_runtime_test.go','summon_test.go']]
        names += ['server/internal/integration/'+n for n in ['level_growth_test.go','summon_pet_test.go']]
        for name in names:
            target=WORK/'before'/name;target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(ROOT/name,target)
        save(baseline_path,{'head':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),
            'recorded_at_utc':datetime.now(timezone.utc).isoformat(),'files':{name:sha(ROOT/name) for name in names}})
    baseline=config.load_json(baseline_path)
    assert baseline['head']=='2a5616ac883c91336edb227d7bbf87e9cfe64791'
    for path in [book,architecture,schema_path]:assert sha(path)==baseline['files'][path.relative_to(ROOT).as_posix()],str(path)
    tables,errors,_,_=config.collect(schema);assert not errors,errors
    old_profile=deepcopy(tables['dev_first_loop_profile'][0]);profile=deepcopy(old_profile['parameters'])
    assert profile['rule_version']==21 and 'base_item_ids' not in profile
    profile.update(rule_version=22,base_item_ids=[35,36,193,218])
    source=ROOT/'资料参考/myf/Mud2/DB/ApexM2_stditems.csv'
    with source.open(encoding='utf-8-sig',newline='') as stream:source_rows={int(row['Idx']):(line,row) for line,row in enumerate(csv.DictReader(stream),2)}
    operations=[];new_refs={}
    for row in tables['game_equipment']:
        assert row['id'] in (218,193)
        line,raw=source_rows[row['item_id']];assert f':{line}' in row['source_ref']
        attrs=deepcopy(row['base_attributes']);assert not (set(attrs)&set(NEW_ATTRIBUTES))
        for key,column in NEW_ATTRIBUTES.items():
            assert raw[column]=='0';attrs[key]=int(raw[column])
        ref=row['source_ref']+SOURCE_NOTE;new_refs[row['id']]=ref
        operations.append({'kind':'update','table_id':'game_equipment','id':row['id'],'values':{'base_attributes':attrs,'source_ref':ref}})
    profile_ref=old_profile['source_ref']+PROFILE_NOTE
    operations.append({'kind':'update','table_id':'dev_first_loop_profile','id':1,'values':{'parameters':profile,'source_ref':profile_ref}})
    plan={'schema_version':1,'group':'gameplay','workbook':book.relative_to(ROOT).as_posix(),'input_workbook_sha256':sha(book),'operations':operations}
    save(WORK/'edit-plan.json',plan)
    candidate=WORK/'玩法配置.candidate.xlsx'
    if not candidate.exists():save(WORK/'editor-review.json',edit_workbook.apply(WORK/'edit-plan.json',candidate))
    candidate_schema=deepcopy(schema)
    for table in candidate_schema['tables']:
        if table['table_id']=='game_equipment':
            next(field for field in table['fields'] if field['name']=='equipment_type')['enum_values']=['weapon','armour']
    for contract in candidate_schema['json_contracts']:
        if contract['table_id']=='game_equipment' and contract['id'] in new_refs:
            contract['source_ref']=new_refs[contract['id']]
            if contract['field']=='base_attributes':
                for key in NEW_ATTRIBUTES:contract['shape']['properties'][key]={'type':'int','minimum':0,'maximum':2**53-1}
        if contract['table_id']=='dev_first_loop_profile' and contract['field']=='parameters':
            contract['shape']['properties']['base_item_ids']={'type':'array','items':{'type':'int','minimum':1,'maximum':2**53-1}}
            contract['source_ref']+=PROFILE_NOTE
    save(WORK/'catalog.candidate.json',candidate_schema)
    check_schema=deepcopy(candidate_schema)
    next(w for w in check_schema['workbooks'] if w['group']=='gameplay')['path']=candidate.relative_to(ROOT).as_posix()
    selected,errors,_,_=config.collect(check_schema)
    errors+=config.check_json_contracts(candidate_schema,selected)+config.semantic_checks(selected)
    assert not errors,errors
    before,after=config.read_xlsx(book),config.read_xlsx(candidate);diff=[]
    for sheet in sorted(set(before)|set(after)):
        for row in sorted(set(before.get(sheet,{}))|set(after.get(sheet,{}))):
            left,right=before.get(sheet,{}).get(row,{}),after.get(sheet,{}).get(row,{})
            for col in sorted(set(left)|set(right)):
                if left.get(col)!=right.get(col):diff.append({'sheet':sheet,'row':row,'column':col,'before':left.get(col),'after':right.get(col)})
    assert len(diff)==6
    expected_cells=set()
    for op in operations:
        table=next(t for t in schema['tables'] if t['table_id']==op['table_id'])
        row_number=next(n for n,values in before[table['sheet_name']].items() if n>=4 and values.get(1)==op['id'])
        expected_cells.update((table['sheet_name'],row_number,next(i+1 for i,f in enumerate(table['fields']) if f['name']==key)) for key in op['values'])
    assert {(d['sheet'],d['row'],d['column']) for d in diff}==expected_cells
    for tid in tables:
        for old,new in zip(tables[tid],selected[tid]):
            expected=deepcopy(old)
            operation=next((op for op in operations if op['table_id']==tid and op['id']==old['id']),None)
            if operation:expected.update(operation['values'])
            assert new==expected,(tid,old['id'])
        assert len(tables[tid])==len(selected[tid]),tid
    a,fa=cells_and_formulas(book);b,fb=cells_and_formulas(candidate)
    assert fa==fb and len(fa)==460
    rawdiff=[key for key in set(a)|set(b) if a.get(key)!=b.get(key)];assert len(rawdiff)==6
    with zipfile.ZipFile(book) as original,zipfile.ZipFile(candidate) as updated:
        assert original.namelist()==updated.namelist()
        changed=[name for name in original.namelist() if original.read(name)!=updated.read(name)]
        assert len(changed)==2 and set(changed)=={key[0] for key in rawdiff}
        untouched=len(original.namelist())-len(changed);assert untouched==101
    review={'status':'candidate_reviewed_not_applied','source_before_sha256':sha(book),'candidate_sha256':sha(candidate),
        'schema_before_sha256':sha(schema_path),'schema_candidate_sha256':sha(WORK/'catalog.candidate.json'),
        'changed_cells':len(diff),'differences':diff,'formulas_preserved':len(fa),'changed_zip_members':changed,
        'untouched_zip_members_verified':untouched,'architecture_workbook_preserved':True,'all_other_cells_styles_and_records_preserved':True,
        'profile_changes':['rule_version21_to22','base_item_ids_original_four','source_ref'],'equipment_changes':['218/193 four explicit zero MC/SC fields','source_ref precise original Mc/Mc2/Sc/Sc2'],
        'source_csv_sha256':sha(source),'all_original_equipment_attributes_preserved':True,'source_220_not_selected':True,
        'no_new_recycle_prices_or_appearance':True}
    save(WORK/'candidate-review.json',review)
    if args.apply:
        shutil.copyfile(candidate,book);shutil.copyfile(WORK/'catalog.candidate.json',schema_path)
        assert sha(book)==review['candidate_sha256'] and sha(schema_path)==review['schema_candidate_sha256']
        save(WORK/'application.json',{'applied':True,'before_sha256':review['source_before_sha256'],'after_sha256':sha(book),
            'schema_before_sha256':review['schema_before_sha256'],'schema_after_sha256':sha(schema_path),'only_reviewed_cells_and_schema_applied':True})
    print(json.dumps({k:review[k] for k in ['status','changed_cells','formulas_preserved','untouched_zip_members_verified','candidate_sha256']},ensure_ascii=False))
    print(json.dumps({'applied':args.apply},ensure_ascii=False))

if __name__=='__main__':main()
