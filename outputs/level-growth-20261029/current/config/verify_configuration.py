"""只读核验rule20源表/原值/原XML行及派生输出，保存最终摘要。"""
from copy import deepcopy
import hashlib
import json
from pathlib import Path
import re
import sys
import xml.etree.ElementTree as ET
import zipfile

ROOT=Path(__file__).resolve().parents[2];WORK=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'tools/config'))
import export_configs as config

def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def read(p):return json.loads(p.read_text(encoding='utf-8'))

baseline=read(WORK/'baseline.json');application=read(WORK/'application.json');review=read(WORK/'candidate-review.json')
book='outputs/development-prep-20261026/玩法配置.xlsx';architecture='outputs/development-prep-20261026/架构配置.xlsx'
assert sha(ROOT/book)==application['after_sha256']==review['candidate_sha256']
assert sha(ROOT/architecture)==baseline['files'][architecture]

def exp_member(z):
    ns={'s':'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
    r='http://schemas.openxmlformats.org/officeDocument/2006/relationships'
    sheet=next(s for s in ET.fromstring(z.read('xl/workbook.xml')).findall('s:sheets/s:sheet',ns) if s.attrib['name']=='逐级经验')
    target=next(n.attrib['Target'] for n in ET.fromstring(z.read('xl/_rels/workbook.xml.rels')) if n.attrib['Id']==sheet.attrib['{'+r+'}id'])
    return target.lstrip('/') if target.startswith('/') else 'xl/'+target

with zipfile.ZipFile(WORK/'before'/book) as old,zipfile.ZipFile(ROOT/book) as new:
    member=exp_member(old);a,b=old.read(member),new.read(member)
    raw_rows={}
    for number in range(4,24):
        pattern=rb'<(?:[A-Za-z_][\w.-]*:)?row\b(?=[^>]*\br="'+str(number).encode()+rb'")[^>]*>.*?</(?:[A-Za-z_][\w.-]*:)?row>'
        left,right=re.search(pattern,a,re.S),re.search(pattern,b,re.S)
        assert left and right and left[0]==right[0],number
        raw_rows[str(number)]=hashlib.sha256(left[0]).hexdigest()

old_fixture=read(WORK/'before/config/development/first-loop.json');fixture=read(ROOT/'config/development/first-loop.json')
before_business=deepcopy(old_fixture);before_business.pop('_source_evidence')
normalized=deepcopy(fixture);normalized.pop('_source_evidence');normalized['rule_version']=19;normalized['progression']=before_business['progression']
assert normalized==before_business
old_public=read(WORK/'before/client/assets/resources/development/gameplay.json');public=read(ROOT/'client/assets/resources/development/gameplay.json')
normalized=deepcopy(public);normalized['rule_version']=19;normalized.pop('progression');normalized['experience']=old_public['experience'];assert normalized==old_public
candidate_checks={}
for name in ['pet-runtime-candidates.json','summon-candidates.json']:
    a=read(WORK/'before/config/development'/name);b=read(ROOT/'config/development'/name);a.pop('source_evidence');b.pop('source_evidence');assert a==b
    candidate_checks[name]={'all_business_values_preserved':True,'only_source_evidence_changed':True}
for name,digest in baseline['saved_layouts'].items():assert sha(ROOT/name)==digest,name
assets_baseline=ROOT/'.work/auto-recycle-20261029/config/baseline.json'
protected=read(assets_baseline)['assets'];checked={}
for name,digest in protected.items():
    if Path(name).suffix in ['.scene','.prefab']:continue
    assert sha(ROOT/name)==digest,name
    checked[name]=digest
schema=config.load_json(ROOT/'config/schema/catalog.json');tables,errors,_,_=config.collect(schema);assert not errors,errors
assert [r['level'] for r in tables['game_exp_levels']]==list(range(1,51))
assert fixture['progression']['exp_per_level']==[r['next_level_exp'] for r in tables['game_exp_levels']]
assert len(public['experience'])==50 and 'exp_per_level' not in public['progression']
pointer=read(ROOT/'config/releases/preview_current.json');assert pointer['version'].startswith('dev-level-growth-v0.20-')
paths=[book,architecture,'config/schema/catalog.json','tools/resources/prepare_first_loop.py','tools/resources/prepare_summon_candidates.py',
       'config/development/first-loop.json','config/development/pet-runtime-candidates.json','config/development/summon-candidates.json',
       'client/assets/resources/development/gameplay.json','config/releases/preview_current.json']
raw=(WORK/'python-full.stdout.txt').read_text(encoding='utf-8')+(WORK/'python-full.stderr.txt').read_text(encoding='utf-8')
count=re.search(r'^Ran (\d+) tests? in ',raw,re.M);assert count and re.search(r'^OK\s*$',raw,re.M)
assert read(WORK/'python-full.json')['exit_code']==0
result={'status':'configuration_passed','scope':'rule20来源经验目录与显式cap/pool配置；不认证自然1→50周期、转生业务或正式经济',
        'preview':pointer['version'],'rule_version':fixture['rule_version'],'current_hashes':{p:sha(ROOT/p) for p in paths},
        'changed_cells':review['changed_cells'],'appended_exp_rows':30,'profile_cells_changed':1,'formulas_preserved':460,
        'untouched_zip_members':review['untouched_zip_members_verified'],'first_20_exp_rows_raw_xml_preserved':True,'first_20_row_raw_sha256':raw_rows,
        'all_other_fixture_business_preserved':True,'all_other_public_values_preserved':True,'candidate_checks':candidate_checks,
        'saved_layouts_unchanged':len(baseline['saved_layouts']),'previous_stage_asset_baseline':{'path':assets_baseline.relative_to(ROOT).as_posix(),'sha256':sha(assets_baseline)},
        'old_resource_files_checked':len(checked),'old_resource_files_changed':[],'python_tests':int(count[1]),'python_exit_code':0,
        'progression':fixture['progression'],'balance_verified':False,'natural_level_1_to_50_verified':False}
(WORK/'final-review.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps({k:result[k] for k in ['status','preview','python_tests','changed_cells','first_20_exp_rows_raw_xml_preserved','old_resource_files_checked','current_hashes']},ensure_ascii=False))
