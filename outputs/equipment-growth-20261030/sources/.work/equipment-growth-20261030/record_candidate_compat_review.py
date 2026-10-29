"""仅记录本轮失败原字节与候选兼容根因；不改配置/Go、不运行测试或服务。"""
from collections import Counter
from datetime import datetime,timezone
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).resolve().parent
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def read(path):return json.loads(path.read_text(encoding='utf-8'))
def save_new(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    with path.open('x',encoding='utf-8',newline='\n') as out:out.write(json.dumps(value,ensure_ascii=False,indent=2)+'\n')

candidate=ROOT/'config/development/summon-candidates.json';old=WORK/'config/before/config/development/summon-candidates.json'
current,baseline=read(candidate),read(old)
for obj in [current,baseline]:obj.pop('source_evidence')
assert current==baseline and current['schema_version']==1 and current['required_rule_version']==17 and current['runtime_enabled'] is False
book=current['books'][0];new_keys={'magic_min_bonus','magic_max_bonus','tao_min_bonus','tao_max_bonus','equipment_stage'}
assert book['id']==25 and book['is_equipment'] is False and not(set(book)&new_keys)
folder=WORK/'go/full-1';report=read(folder/'report.json');wrapped=read(folder/'wrapper.json')
assert report['passed'] is False and report['exit_code']==1 and report['source_sha256']==report['source_sha256_after'] and report['source_changed_during_test'] is False
events=[json.loads(line) for line in (folder/'events.jsonl').read_text(encoding='utf-8').splitlines() if line.strip()]
assert sha(folder/'events.jsonl')==report['events_sha256']
passed=[{'package':row['Package'],'test':row['Test']} for row in events if row.get('Test') and row['Action']=='pass']
failed=[{'package':row['Package'],'test':row.get('Test')} for row in events if row['Action'] in {'fail','build-fail'}]
assert passed==report['passed_tests'] and failed==report['failed_tests']
messages=Counter(row['Package'] for row in events if 'magic_min_bonus' in row.get('Output','') and '缺少字段' in row.get('Output',''))
selectors={}
for name in ['report.json','events.jsonl','wrapper.json','started.json','full-mongo-race.stdout.txt','full-mongo-race.stderr.txt']:
    p=folder/name;assert p.is_file();selectors[p.relative_to(ROOT).as_posix()]={'sha256':sha(p),'bytes':p.stat().st_size}
save_new(WORK/'history/go-full-1-selector.json',{'scope':'实际full-1失败原件，只保存指针和原SHA；最终归档须复制这些原字节/无损gzip，不修改原状态',
    'passed':False,'source_inputs':len(report['source_sha256']),'passed_tests_including_subtests':len(passed),
    'failed_tests_including_subtests':sum(row['test'] is not None for row in failed),'failed_packages':sum(row['test'] is None for row in failed),'files':selectors})
cutover=WORK/'cutover.py';data=cutover.read_bytes();assert data.count(b"go_dir = WORK / 'go/full-2'")==1
old_data=data.replace(b"go_dir = WORK / 'go/full-2'",b"go_dir = WORK / 'go/full-1'")
assert hashlib.sha256(data).hexdigest()=='d8a5230b5bf5f38e0cfd0bbf818e6f1537671c33203d377077f5f66d57c78524'
assert hashlib.sha256(old_data).hexdigest()=='f27f7ab81e7a57909c337ec494f72e22e765b4efadf56fc81769b19006dcf476'
review={'status':'read_only_root_cause_confirmed_minimal_go_compatibility_fix_authorized','recorded_at_utc':datetime.now(timezone.utc).isoformat(),
    'candidate_path':candidate.relative_to(ROOT).as_posix(),'candidate_sha256':sha(candidate),'candidate_payload_unchanged_from_baseline':True,
    'candidate_schema_version':1,'candidate_required_rule_version':17,'runtime_enabled':False,'book_id':25,'book_is_equipment':False,
    'new_equipment_fields_absent_as_legacy_neutral_contract':sorted(new_keys),'unique_item25_is_skill_book':True,'selected_equipment_ids':[218,193],
    'root_cause':{'path':'server/internal/devconfig/pet_runtime.go:123-136','detail':'requirePetFields反射共享Item，将rule22新增无omitempty字段误作旧rule17书的必需字段；JSON生成器及唯一表未丢装备来源数字'},
    'additional_neutral_guard_required':{'path':'server/internal/devconfig/summon_candidates.go:43','detail':'兼容缺失后仍须拒绝候选书新4bonus和stage非零；现Item.Unmarshal的null拒绝与rule22 EquipmentStatsKnown presence保持'},
    'minimal_fix_paths':['server/internal/devconfig/pet_runtime.go','server/internal/devconfig/summon_candidates.go','server/internal/devconfig/equipment_compat_test.go'],
    'config_generator_or_workbook_regeneration_required':False,'python130_scope':'原投影/schema检查通过，不覆盖真实Go旧候选decode；原final-review不修改',
    'configuration_final_review_sha256':sha(WORK/'config/final-review.json'),'full_go1_report_sha256':sha(folder/'report.json'),
    'raw_counts':{'passed_tests_including_subtests':len(passed),'failed_tests_including_subtests':sum(row['test'] is not None for row in failed),'failed_packages':sum(row['test'] is None for row in failed),'matching_error_output_lines_by_package':dict(messages)},
    'read_only_review_ran_go_or_database':False,'full_go_failure_is_actual_not_reconstructed':True,
    'cutover_selector_only_change':{'before_sha256':hashlib.sha256(old_data).hexdigest(),'after_sha256':sha(cutover),'before':'go/full-1','after':'go/full-2','reverse_single_replacement_matches_before_sha':True},
    'limits':'只读根因审查及原全包失败留痕，不代表最小修复或full-2已实测通过；当前主仍21未切换'}
save_new(WORK/'candidate-compat-review.json',review)
print(json.dumps({'report':str(WORK/'candidate-compat-review.json'),'sha256':sha(WORK/'candidate-compat-review.json'),'raw_pass':len(passed),'raw_failed_tests':sum(row['test'] is not None for row in failed),'raw_failed_packages':sum(row['test'] is None for row in failed),'configuration_modified':False},ensure_ascii=False))
