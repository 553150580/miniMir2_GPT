"""核对rule22受控表变更、单一来源派生及命令原始证据；不运行Go或数据库。"""
from copy import deepcopy
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT=Path(__file__).resolve().parents[3]
WORK=Path(__file__).resolve().parent
def load(path):return json.loads(path.read_text(encoding='utf-8'))
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def save(path,value):path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')

baseline,review,applied=[load(WORK/name) for name in ['baseline.json','candidate-review.json','application.json']]
assert baseline['head']=='2a5616ac883c91336edb227d7bbf87e9cfe64791'
assert review['changed_cells']==6 and review['formulas_preserved']==460 and review['untouched_zip_members_verified']==101
assert applied['applied'] and applied['after_sha256']==review['candidate_sha256']
book=next(name for name in baseline['files'] if name.endswith('/玩法配置.xlsx'))
architecture=next(name for name in baseline['files'] if name.endswith('/架构配置.xlsx'))
assert sha(ROOT/book)==review['candidate_sha256'] and sha(ROOT/'config/schema/catalog.json')==review['schema_candidate_sha256']
assert sha(ROOT/architecture)==baseline['files'][architecture]
assert sha(ROOT/'tools/resources/prepare_pet_runtime.py')==baseline['files']['tools/resources/prepare_pet_runtime.py']

fixture=load(ROOT/'config/development/first-loop.json');public=load(ROOT/'client/assets/resources/development/gameplay.json')
assert fixture['rule_version']==public['rule_version']==22
four={'magic_min_bonus','magic_max_bonus','tao_min_bonus','tao_max_bonus'}
old=load(WORK/'before/config/development/first-loop.json');new=deepcopy(fixture)
old.pop('_source_evidence');new.pop('_source_evidence');old['rule_version']=22
for item in new['items']:
    if item['is_equipment']:
        assert item['equipment_stage']==0
        assert all(type(item[key]) is int and item[key]==0 for key in four)
        for key in four|{'equipment_stage'}:item.pop(key)
    else:assert all(item.get(key,0)==0 for key in four)
assert old==new,'除四项显式零与阶段/版本外运行数值发生变化'
new=deepcopy(public)
attrs=set(['physical_attack_min','physical_attack_max','magic_attack_min','magic_attack_max','spirit_attack_min','spirit_attack_max','physical_defence_min','physical_defence_max','magic_defence_min','magic_defence_max','attack_speed'])
for item in new['items']:
    if item['kind']=='equipment':
        assert set(item['base_attributes'])==attrs and item['equipment_stage']==0
        item.pop('base_attributes');item.pop('equipment_stage')
old=load(WORK/'before/client/assets/resources/development/gameplay.json');old['rule_version']=22
assert old==new,'public存在额外运行变化'
for name in ['pet-runtime-candidates.json','summon-candidates.json']:
    old=load(WORK/'before/config/development'/name);new=load(ROOT/'config/development'/name)
    old.pop('source_evidence');new.pop('source_evidence');assert old==new,name
assert len(fixture['skills'])==22 and all(s['id']!=17 for s in fixture['skills'])
assert fixture['casting_supply']['enabled'] is False and fixture['pet_runtime']['bootstrap_mode']=='legacy_untrained_pet'
assert all(i['id']!=220 for i in fixture['items'])
assert [i['recycle_gold'] for i in fixture['items'] if i['is_equipment']]==[50,400]
assert fixture['_source_evidence']['workbook']['sha256']==sha(ROOT/book)
assert fixture['_source_evidence']['exporter_sha256']==sha(ROOT/'tools/resources/prepare_first_loop.py')
assert fixture['_source_evidence']['equipment_foundation']['base_item_ids']==[35,36,193,218]
summon=load(ROOT/'config/development/summon-candidates.json')
assert summon['runtime_enabled'] is False and summon['required_rule_version']==17
assert summon['source_evidence']['generator_sha256']==sha(ROOT/'tools/resources/prepare_summon_candidates.py')
assert summon['source_evidence']['compiler_sha256']==sha(ROOT/'tools/resources/prepare_first_loop.py')

pointer=load(ROOT/'config/releases/preview_current.json');version=pointer['version']
assert version.startswith('dev-equipment-foundation-v0.22-')
release=ROOT/'config/releases'/pointer['path'];manifest=load(release/'manifest.json')
assert manifest['version']==version
for name,value in manifest['files'].items():assert sha(release/name)==value,name
sync=load(ROOT/'conf/toolchain/config-binding-sync.json');assert sync['config_version']==version and sync['generated_only'] is True
generated_types=['server/internal/configtypes/config_types.go','client/assets/scripts/generated/config_types.ts']
for name in generated_types:assert subprocess.check_output(['git','show','HEAD:'+name],cwd=ROOT)==(ROOT/name).read_bytes(),name
adaptations=load(WORK/'current-test-adaptations.json')
for row in adaptations:
    path=ROOT/row['path'];assert sha(path)==row['after_sha256'],str(path)
    if row['path'].endswith('.go'):
        before=(WORK/'before'/row['path']).read_bytes()
        assert before.replace(b'.RuleVersion != 21',b'.RuleVersion != 22')==path.read_bytes(),str(path)
layouts=load(ROOT/'.work/equipment-growth-20261030/baseline.json')['saved_layouts']
for name,value in layouts.items():assert sha(ROOT/name)==value,name

commands=[]
for name in ['candidate-3','apply','export-check','equipment-narrow','pet-generate','summon-generate','fixture-generate','export','pet-check','summon-check','fixture-check','python-full','sync-bindings','verify-bindings']:
    command=load(WORK/(name+'.json'));assert command['exit_code']==0,name
    for field in ['stdout','stderr']:assert sha(WORK/command[field])==command[field+'_sha256'],name
    commands.append({'name':name,'report':name+'.json','exit_code':0,'seconds':command['seconds']})
text=(WORK/'python-full.stderr.txt').read_text(encoding='utf-8')
match=re.search(r'^Ran (\d+) tests? in ',text,re.M);assert match and re.search(r'^OK\s*$',text,re.M)
count=int(match[1]);assert count==130
paths=set(baseline['files'])|{'tools/config/test_equipment_foundation.py','conf/toolchain/config-binding-sync.json',*generated_types}
paths|={p.relative_to(ROOT).as_posix() for p in release.iterdir() if p.is_file()}
final={'status':'configuration_complete_and_frozen','recorded_at_utc':datetime.now(timezone.utc).isoformat(),
    'rule_version':22,'preview':version,'python_tests':count,'changed_workbook_cells':6,'formulas_preserved':460,
    'untouched_zip_members_verified':101,'architecture_workbook_preserved':True,'saved_layouts_preserved':len(layouts),
    'only_reviewed_profile_and_equipment_changes':True,'original_7_equipment_attributes_preserved':True,
    'all_other_runtime_values_unchanged':True,'independent_candidate_payloads_unchanged':True,'go_ts_generated_type_bytes_unchanged':True,
    'sync_record_matches_new_release':True,'main_skill_count':22,'summon17_enabled':False,'casting_supply_enabled':False,
    'bootstrap_mode':'legacy_untrained_pet','summon_candidate_required_rule_version':17,'summon_candidate_runtime_enabled':False,
    'source220_enabled':False,'new_recycle_prices_added':False,'new_appearance_mapping_added':False,
    'current_version_go_guard_replacements':sum(row.get('current_guard_replacements',0) for row in adaptations),
    'history':[{'report':'candidate.json','scope':'作者工具误用多工作簿override接口，未应用源；改为显式临时schema路径'},
        {'report':'candidate-2.json','scope':'作者工具手写列号断言错误；改为按schema字段推导；非产品失败'}],
    'commands':commands,'current_hashes':{name:sha(ROOT/name) for name in sorted(paths)},
    'scope':'唯一表/schema/导出/Python/绑定一致性，不表示装备实际收益、穿脱、Mongo/WS、Creator、GUI、外观或完整三职业成长已验收'}
save(WORK/'final-review.json',final)
print(json.dumps({k:final[k] for k in ['status','rule_version','preview','python_tests','changed_workbook_cells','formulas_preserved','untouched_zip_members_verified']},ensure_ascii=False))
print(json.dumps({key:final['current_hashes'][key] for key in ['config/development/first-loop.json','client/assets/resources/development/gameplay.json',book]},ensure_ascii=False))
