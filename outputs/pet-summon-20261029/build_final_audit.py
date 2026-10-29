"""只读复核root选定终态日志、GUI指针与当前摘要，不重新执行业务。"""
from pathlib import Path
import datetime
import hashlib
import json
import re

ROOT=Path(__file__).resolve().parents[2]
BASE=Path(__file__).resolve().parent
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()

def evidence(path):
    path=ROOT/path
    return {'path':path.relative_to(ROOT).as_posix(),'sha256':sha(path)}

def drift(values):
    return [name for name,digest in values.items() if not (ROOT/name).exists() or sha(ROOT/name)!=digest]

def selected_browser(name,expected_sha,checks,warnings):
    selection_path=BASE/(name+'-selection.json')
    selection=read(selection_path)
    path=BASE/selection['report_directory']/'report.json'
    assert path.resolve().is_relative_to(BASE.resolve()) and selection['report_sha256']==sha(path)==expected_sha
    data=read(path)
    assert data['passed'] and len(data['checks'])==checks and not data['errors'] and len(data['warnings'])==warnings
    if name=='browser-town':assert data['bindingVerified']
    return {'passed':True,'checks':checks,'errors':0,'warnings':warnings,'selection':evidence(selection_path),'report':evidence(path)}

def main():
    p=read(BASE/'checks/final/summary.json')
    assert p['passed'] and p['inputs_stable'] and len(p['inputs'])==14 and not drift(p['inputs']) and not drift(p['artifact_sha256'])
    assert read(BASE/'checks/final/before.json')==read(BASE/'checks/final/after.json')==p['inputs']
    assert re.search(r'Ran 100 tests in [\d.]+s\s+OK\s*$',(BASE/'checks/final/python.txt').read_text(encoding='utf-8'))
    assert all(row['exit_code']==0 for row in p['commands'])
    g=read(ROOT/'server/test-results/latest.json')
    go_meta=read(BASE/'checks/go.json');go_output=read(BASE/'checks/go.txt')
    events=[json.loads(line) for line in (ROOT/'server/test-results/latest.events.jsonl').read_text(encoding='utf-8').splitlines() if line.strip()]
    passes=[e for e in events if e.get('Action')=='pass' and e.get('Test')]
    failures=[e for e in events if e.get('Action') in ['fail','build-fail']]
    skips=[e for e in events if e.get('Action')=='skip' and e.get('Test')]
    assert g['passed'] and g['exit_code']==0 and g['race_detector'] and g['mongo_e2e_explicit']
    assert len(passes)==len(g['passed_tests'])==944 and not failures and not skips and not g['failed_tests'] and not g['skipped_tests']
    assert {(e['Package'],e['Test']) for e in passes}=={(e['package'],e['test']) for e in g['passed_tests']}=={(e['package'],e['test']) for e in go_output['tests']}
    assert g['source_sha256']==g['source_sha256_after'] and len(g['source_sha256'])==162 and not drift(g['source_sha256'])
    assert sha(ROOT/'server/test-results/latest.events.jsonl')==g['events_sha256']
    assert sha(BASE/'checks/go.txt')==go_meta['output_sha256']
    integrated_ws=[{'test':e['Test'],'elapsed_seconds':e.get('Elapsed')} for e in passes if e['Test']=='TestSummonPetMaterialRecallTrainingNoPetsAndColdMongoWS']
    assert len(integrated_ws)==1
    ws=read(BASE/'ws-runtime/run-011627/report.json')
    ws_old=read(BASE/'ws-runtime/run-011126/report.json')
    for name,record in [('run-011627',ws),('run-011126',ws_old)]:
        assert record['input_sha256']==record['input_sha256_after'] and record['inputs_stable'] and len(record['input_sha256'])==162
        assert sha(BASE/'ws-runtime'/name/'output.txt')==record['output_sha256']
    assert ws['passed'] and ws['exit_code']==0 and ws['seconds']==22.426 and not drift(ws['input_sha256_after'])
    assert ws_old['exit_code']==1 and not ws_old['passed']
    ws_old_drift=drift(ws_old['input_sha256_after'])
    assert ws_old_drift==['server/internal/integration/summon_pet_test.go']
    preview=read(ROOT/'config/releases/preview_current.json')
    release=ROOT/'config/releases'/preview['path'];manifest=read(release/'manifest.json')
    assert preview['version']==manifest['version']=='dev-pet-summon-v0.17-9cfc98f544bf'
    assert preview['content_sha256']==manifest['content_sha256'] and preview['runtime_ready'] is False
    for name,digest in manifest['files'].items():assert sha(release/name)==digest,name
    f=read(ROOT/'config/development/first-loop.json');pet=read(ROOT/'config/development/pet-runtime-candidates.json');summon=read(ROOT/'config/development/summon-candidates.json')
    s=summon['source_evidence']
    for book in s['workbooks']:assert sha(ROOT/book['path'])==book['sha256']
    for key,name in [('schema_sha256','config/schema/catalog.json'),('compiler_sha256','tools/resources/prepare_first_loop.py'),
        ('pet_compiler_sha256','tools/resources/prepare_pet_runtime.py'),('generator_sha256','tools/resources/prepare_summon_candidates.py'),
        ('source_contract_sha256','资料索引/职业宠物补充来源契约.json')]:assert sha(ROOT/name)==s[key]
    assert manifest['schema_sha256']==s['schema_sha256']
    assert pet['definitions']==f['pet_runtime']['definitions'] and pet['budgets']==f['pet_runtime']['budgets']
    assert len(f['skills'])==22 and not any(row['id']==17 for row in f['skills']) and f['casting_supply']['enabled'] is False
    assert f['pet_runtime']['bootstrap_mode']=='legacy_untrained_pet' and pet['runtime_enabled'] is False and summon['runtime_enabled'] is False
    build=read(ROOT/'docs/客户端构建检查.json')
    assert build['creator']=='3.8.8' and build['success'] and build['exit_code']==36 and build['seconds']==122.71
    assert build['scene_prefab_layout_preserved'] and build['source_stable_during_build']
    assert len(build['asset_hashes'])==10 and not drift(build['asset_hashes']) and not drift(build['script_hashes'])
    client_path=BASE/'client-review/report.json'
    assert sha(client_path)=='cb6d2656e7efc914c4addcc9dba6df47e78733d5d3473a89a61af4e41e7a4582'
    client=read(client_path)
    assert client['stable'] and client['before']==client['after']==client['current']==client['current_frozen']
    assert client['input_files']==len(client['current'])==224 and not drift(client['current'])
    assert len(client['commands'])==5 and all(row['exit_code']==0 for row in client['commands'])
    assert client['full_ui_app_actual_counts']=={'tests':358,'pass':358,'fail':0,'skipped':0}
    assert client['pb']=={'message_count':69,'int64_field_count':140,'int64_boundary_cases':458}
    assert client['fixture_sha256']==sha(ROOT/'config/development/first-loop.json')
    assert client['public_sha256']==sha(ROOT/'client/assets/resources/development/gameplay.json')
    assert client['summon_candidate_sha256']==sha(ROOT/'config/development/summon-candidates.json')
    client_raw=(BASE/'client-review/full-ui-app.txt').read_text(encoding='utf-8')
    for key,value in [('tests',358),('pass',358),('fail',0),('skipped',0)]:
        assert re.search(r'(?:#|ℹ) '+key+' '+str(value)+r'(?:\r?\n|$)',client_raw),(key,value)
    scene_meta=read(BASE/'checks/scene.json')
    assert scene_meta['exit_code']==0 and sha(ROOT/scene_meta['output'])==scene_meta['output_sha256']
    scene_text=(ROOT/scene_meta['output']).read_text(encoding='utf-8')
    assert re.search(r'(?:#|ℹ) pass 16(?:\r?\n|$)',scene_text)
    assert re.search(r'(?:#|ℹ) skipped 0(?:\r?\n|$)',scene_text)
    town=selected_browser('browser-town','d7c0e32ab494af26ed65c72ae00307ad7c37a4494188788fbeaa4d5b7bc47123',7,0)
    ui=selected_browser('browser-ui','bf2c507f6280062bba389b527e3f7a8820b3f2f85e30dfecc9b877bc8c90da4a',6,1)
    gui_selection_path=BASE/'gui-review/final-run.json';selection=read(gui_selection_path)
    gui_path=BASE/'gui-review'/selection['run_directory']/'report.json'
    assert selection['run_directory']=='run-1e6ecba6ad354480b145fb28221d9b45'
    assert gui_path.resolve().is_relative_to((BASE/'gui-review').resolve())
    assert selection['report_sha256']==sha(gui_path)=='04d8d6b27051b4c824646a08fc6e657b1c9daea91ac8c10f25f32da39df43225'
    gui=read(gui_path)
    assert gui['passed'] and gui['corpse_ttl_ms']==15000
    assert all(gui[key] for key in ['main_restored','qa_database_dropped','qa_stopped','protected_unchanged']) and not gui['cleanup_errors']
    assert gui['browser']['passed'] and (gui['browser']['checks'],gui['browser']['errors'],gui['browser']['warnings'])==(5,0,0)
    gui_raw_path=Path(gui['browser']['report'])
    assert gui_raw_path.resolve().is_relative_to(gui_path.parent.resolve())
    gui_raw=read(gui_raw_path)
    assert gui_raw['passed'] and len(gui_raw['checks'])==5 and not gui_raw['errors'] and not gui_raw['warnings']
    tools_path=BASE/'gui-review'/selection['tools_report_directory']/'report.json'
    assert selection['tools_report_directory']=='static-rule17-tools' and tools_path.resolve().is_relative_to((BASE/'gui-review').resolve())
    tools=read(tools_path)
    assert tools['passed'] and tools['stable'] and tools['before']==tools['after']==tools['current']
    assert len(tools['current'])==4 and not drift(tools['current'])
    result={'schema_version':1,'audited_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'status':'root选定的本阶段终态证据已完整只读核对；无未补查项',
        'audit_only':True,'new_test_or_db_execution':False,'frozen_inputs_modified':False,
        'python':{'passed_methods':100,'failed':0,'input_count':14,'before_after_current_equal':True,
            'evidence':[evidence(BASE/'checks/final'/n) for n in ['summary.json','commands.json','python.txt','before.json','after.json']]},
        'go':{'passed_test_nodes':944,'failed_test_nodes':0,'skipped_test_nodes':0,'real_mongo':True,'race':True,'input_count':162,
            'before_after_current_equal':True,'finished_at_utc':g['finished_at_utc'],'events_sha256':g['events_sha256'],
            'command':g['command'],'summon_ws_reexecuted':integrated_ws,
            'evidence':[evidence(path) for path in ['server/test-results/latest.json','server/test-results/latest.events.jsonl',BASE/'checks/go.json',BASE/'checks/go.txt']]},
        'ws':{'selected_run':'run-011627','passed':True,'exit_code':0,'wrapper_seconds':22.426,'case_seconds':10.54,'package_seconds':15.962,
            'input_count':162,'before_after_current_equal':True,'isolated':{'role_level':19,'initial_book_id':25,'explicit_count_units':2,'monster_hp':1,'monster_damage':0,'no_pets_town':True,'bootstrap_mode':'none'},
            'not_verified':['正常购符/包规格/售价','自然1→19','主profile17开放'],
            'evidence':[evidence(BASE/'ws-runtime/run-011627'/n) for n in ['report.json','output.txt']],
            'first_failed_premise':{'run':'run-011126','wrapper_seconds':ws_old['seconds'],'exit_code':1,'actual_error':3203,'expected_error':3202,
                'reason':'仍在施法Cooldown前态，不能把冷却拒绝当作NoPets判定失败；后续只调整测试等待前态',
                'current_drift':ws_old_drift,'only_tests_changed':True,'final_full_go_reexecuted_same_ws':True,
                'evidence':[evidence(BASE/'ws-runtime/run-011126'/n) for n in ['report.json','output.txt']]}},
        'configuration':{'rule_version':17,'main_skill_count':22,'main_summon17_selected':False,'main_material_enabled':False,'bootstrap_mode':'legacy_untrained_pet',
            'pet_catalog_schema':pet['schema_version'],'summon_catalog_schema':summon['schema_version'],'summon_required_rule':summon['required_rule_version'],
            'preview':preview,'preview_files_all_match':True,'source_evidence_current':True,'artifact_sha256':p['artifact_sha256']},
        'creator':{'version':'3.8.8','platform':build['platform'],'seconds':122.71,'exit_code':36,'success':True,'layout_count':10,
            'script_count':len(build['script_hashes']),'all_current_match':True,'evidence':[evidence('docs/客户端构建检查.json'),evidence(Path(build['log']))]},
        'client':{'status':'verified_selected_final','passed_ui_app':358,'scene_passed':16,'javascript_total':374,
            'input_count':224,'before_after_current_frozen_equal':True,'current_drift':[],'five_commands_exit_zero':True,
            'pb':client['pb'],'new_protocol_ids':client['new_protocol_ids'],'saved_layouts_preserved':client['saved_layouts_unchanged'],
            'evidence':[evidence(client_path),evidence(BASE/'client-review/full-ui-app.txt'),evidence(BASE/'checks/scene.json'),evidence(BASE/'checks/scene.txt')]},
        'gui':{'status':'verified_selected_final','run_id':gui['run_id'],'checks':5,'errors':0,'warnings':0,'corpse_ttl_ms':15000,
            'main_restored':True,'qa_stopped':True,'qa_database_dropped':True,'protected_unchanged':True,
            'tools_input_count':4,'tools_before_after_current_equal':True,'tools_current_drift':[],
            'evidence':[evidence(gui_selection_path),evidence(gui_path),evidence(gui_raw_path),evidence(tools_path)]},
        'town':town,'ui':ui,
        'limits':['Count2为明确隔离数量，不是护符购买包或DuraMax换算','主仍22技能/legacy不训练/材料关闭；候选编译不等于主17正常开放',
            '手机真机、容量、自然养成与完整三职业不因这些限定证据通过而完成']}
    (BASE/'final-audit.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps({'python':100,'python_inputs':14,'go':944,'go_inputs':162,'ws_wrapper_seconds':22.426,'creator_seconds':122.71,
        'client':358,'client_inputs':224,'scene':16,'pb':[69,140,458],'gui':[5,0,0],'town':[7,0,0],'ui':[6,0,1],'tools_inputs':4},ensure_ascii=False))

if __name__=='__main__':main()
