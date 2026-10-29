"""只从独立副本[Exp]生成21–50候选，逐格审证后才允许替换唯一源书。"""
from copy import deepcopy
import hashlib
import json
from pathlib import Path
import re
import shutil
import sys
import xml.etree.ElementTree as ET
import zipfile

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'tools/config'))
import export_configs as config
import edit_workbook

POLICY={'curve_version':'myf_exp_candidate_1_50_v1','operating_level_cap':50,'catalog_max_level':50,
        'pending_pool_policy':'one_next_level','new_overflow_policy':'reject_new_with_receipt','legacy_overflow_policy':'preserve_frozen',
        'level_up_resources':'source_full_if_alive_original_birth','rebirth_enabled':False}


def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def save(p,v):p.parent.mkdir(parents=True,exist_ok=True);p.write_text(json.dumps(v,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')


def formulas(path):
    ns={'s':'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
    with zipfile.ZipFile(path) as z:
        return {name:{c.attrib['r']:ET.tostring(c.find('s:f',ns),encoding='unicode') for c in ET.fromstring(z.read(name)).findall('.//s:c',ns) if c.find('s:f',ns) is not None}
                for name in z.namelist() if name.startswith('xl/worksheets/') and name.endswith('.xml')}


def main():
    import argparse
    parser=argparse.ArgumentParser();parser.add_argument('--apply',action='store_true');args=parser.parse_args()
    schema=config.load_json(ROOT/'config/schema/catalog.json')
    book=ROOT/next(w['path'] for w in schema['workbooks'] if w['group']=='gameplay')
    architecture=ROOT/next(w['path'] for w in schema['workbooks'] if w['group']=='architecture')
    before=WORK/'before';before.mkdir(exist_ok=True)
    baseline_path=WORK/'baseline.json'
    if not baseline_path.exists():
        paths=[book,architecture,ROOT/'config/schema/catalog.json',ROOT/'tools/resources/prepare_first_loop.py',ROOT/'tools/resources/prepare_summon_candidates.py',
               ROOT/'config/development/first-loop.json',ROOT/'client/assets/resources/development/gameplay.json',ROOT/'config/development/pet-runtime-candidates.json',ROOT/'config/development/summon-candidates.json',ROOT/'config/releases/preview_current.json']
        for p in paths:
            target=before/p.relative_to(ROOT);target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(p,target)
        layouts={p.relative_to(ROOT).as_posix():sha(p) for pattern in ['*.scene','*.prefab'] for p in (ROOT/'client/assets').rglob(pattern)}
        save(baseline_path,{'files':{p.relative_to(ROOT).as_posix():sha(p) for p in paths},'saved_layouts':layouts})
    baseline=config.load_json(baseline_path);original=before/book.relative_to(ROOT)
    tables,errors,_,_=config.collect(schema);assert not errors,errors
    old_profile=deepcopy(tables['dev_first_loop_profile'][0]['parameters'])
    if args.apply:
        # --apply之前源书仍必须完全等于受审前态。
        assert sha(book)==baseline['files'][book.relative_to(ROOT).as_posix()]
    assert old_profile['rule_version']==19 and 'progression' not in old_profile
    updated=deepcopy(old_profile);updated['rule_version']=20;updated['progression']=deepcopy(POLICY)
    source=ROOT/'资料参考/myf/Mir200/Exps.ini';data=source.read_bytes()
    locks=[x for x in schema['source_locks'] if x['path'].replace('\\','/').endswith('/Exps.ini')]
    assert len(locks)==1;locked=locks[0]['sha256']
    assert sha(source)==locked
    section='';values={}
    for line,text in enumerate(data.decode('gb18030').splitlines(),1):
        h=re.fullmatch(r'\s*\[(.+)\]\s*',text)
        if h:section=h[1]
        m=re.fullmatch(r'Level(\d+)=(\d+)',text.strip())
        if section=='Exp' and m:
            level=int(m[1]);assert level not in values;values[level]=(line,int(m[2]))
    assert len(tables['game_exp_levels'])==20 and all(r['next_level_exp']==values[r['level']][1] for r in tables['game_exp_levels'])
    operations=[{'kind':'update','table_id':'dev_first_loop_profile','id':1,'values':{'parameters':updated}}]
    for level in range(21,51):
        line,value=values[level];assert line==level+3 and value>0
        row={'id':level,'level':level,'next_level_exp':value,'growth_target':'development_candidate_only','measurement_version':'myf_source_unbalanced','record_status':'candidate',
             'source_ref':f'资料参考/myf/Mir200/Exps.ini:{line}；[Exp].Level{level}；独立副本SHA256={locked}',
             'note':'仅来源数值开发候选，未验证自然1→50或长期节奏；当前仅15EXP普通怪，不沿旧235天表。INI其它经验处理开关未自动移植；50级下级门槛仅供待用池及未来转生，显式cap禁止自动51。'}
        operations.append({'kind':'append','table_id':'game_exp_levels','id':level,'values':row})
    plan={'schema_version':1,'group':'gameplay','workbook':book.relative_to(ROOT).as_posix(),'input_workbook_sha256':baseline['files'][book.relative_to(ROOT).as_posix()],'operations':operations}
    save(WORK/'edit-plan.json',plan);candidate=WORK/'玩法配置.candidate.xlsx'
    if not candidate.exists():save(WORK/'editor-review.json',edit_workbook.apply(WORK/'edit-plan.json',candidate))
    a,b=config.read_xlsx(original),config.read_xlsx(candidate);diff=[]
    for sheet in set(a)|set(b):
        for row in set(a.get(sheet,{}))|set(b.get(sheet,{})):
            left,right=a.get(sheet,{}).get(row,{}),b.get(sheet,{}).get(row,{})
            for column in set(left)|set(right):
                if left.get(column)!=right.get(column):diff.append({'sheet':sheet,'row':row,'column':column,'before':left.get(column),'after':right.get(column)})
    profile_diffs=[d for d in diff if d['sheet']=='开发首循环夹具'];exp_diffs=[d for d in diff if d['sheet']=='逐级经验']
    assert len(diff)==241 and len(profile_diffs)==1 and profile_diffs[0]['row']==4 and profile_diffs[0]['column']==3
    assert len(exp_diffs)==240 and all(d['before'] is None and d['row']>=24 for d in exp_diffs)
    profile=json.loads(profile_diffs[0]['after']);assert profile.pop('progression')==POLICY;profile['rule_version']=19;assert profile==old_profile
    fa,fb=formulas(original),formulas(candidate);assert fa==fb and sum(map(len,fa.values()))==460
    with zipfile.ZipFile(original) as left,zipfile.ZipFile(candidate) as right:
        assert left.namelist()==right.namelist();changed=[n for n in left.namelist() if left.read(n)!=right.read(n)];assert len(changed)==2
        unchanged=len(left.namelist())-len(changed)
    assert sha(architecture)==baseline['files'][architecture.relative_to(ROOT).as_posix()]
    review={'status':'candidate_reviewed_not_applied' if not args.apply else 'candidate_applied','source_before_sha256':sha(original),'candidate_sha256':sha(candidate),
            'changed_cells':len(diff),'appended_exp_rows':30,'profile_cells_changed':1,'exp_cells_appended':240,'differences':diff,
            'first_20_exp_rows_unchanged':True,'all_other_cells_unchanged':True,'all_460_formulas_preserved':True,
            'changed_zip_members':changed,'untouched_zip_members_verified':unchanged,'architecture_unchanged':True,'source_ini_unchanged':sha(source)==locked}
    save(WORK/'candidate-review.json',review)
    if args.apply:
        shutil.copyfile(candidate,book);assert sha(book)==sha(candidate)
        save(WORK/'application.json',{'applied':True,'before_sha256':sha(original),'after_sha256':sha(book),'candidate_review':'candidate-review.json','only_reviewed_cells_applied':True})
    print(json.dumps({k:review[k] for k in ['status','changed_cells','appended_exp_rows','profile_cells_changed','untouched_zip_members_verified','candidate_sha256']},ensure_ascii=False))


if __name__=='__main__':main()
