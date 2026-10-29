"""只读汇总既有证据，不执行测试或修改冻结输入。"""
from pathlib import Path
import datetime
import hashlib
import json
import re

ROOT=Path(__file__).resolve().parents[2]
BASE=Path(__file__).resolve().parent
sha=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
load=lambda path:json.loads(path.read_text(encoding='utf-8'))

def current_drift(manifest):
    return [name for name,digest in manifest.items() if not (ROOT/name).exists() or sha(ROOT/name)!=digest]

def evidence(path):
    path=ROOT/path
    return {'path':path.relative_to(ROOT).as_posix(),'sha256':sha(path)}

def selected_browser(name, checks, warnings):
    selection_path=BASE/(name+'-selection.json')
    selection=load(selection_path)
    report_path=BASE/selection['report_directory']/'report.json'
    assert report_path.resolve().is_relative_to(BASE.resolve())
    assert sha(report_path)==selection['report_sha256']
    report=load(report_path)
    assert report['passed'] and len(report['checks'])==checks and not report['errors'] and len(report['warnings'])==warnings
    if name=='browser-town':assert report['bindingVerified']
    return {'pass':True,'checks':checks,'errors':0,'warnings':warnings,'report_directory':selection['report_directory'],
        'selection':evidence(selection_path),'selected_report':evidence(report_path)}

def main():
    config=load(BASE/'checks/summary.json')
    python=(BASE/'checks/python-configuration.txt').read_text(encoding='utf-8')
    assert re.search(r'Ran 90 tests in [\d.]+s\s+OK\s*$',python)
    ws=load(BASE/'ws-runtime/real-mongo-ws-result.json')
    ws_before=load(BASE/'ws-runtime/real-mongo-ws-before.json')
    ws_after=load(BASE/'ws-runtime/real-mongo-ws-after.json')
    ws_text=(BASE/'ws-runtime/real-mongo-ws.txt').read_text(encoding='utf-8')
    assert ws['exit_code']==0 and ws_before==ws_after and '--- PASS: TestPetRuntimeCorpseHealthyExpiryAndColdEntryMongoWS' in ws_text
    go=load(ROOT/'server/test-results/latest.json')
    go_meta=load(BASE/'checks/go.json')
    go_output=load(BASE/'checks/go.txt')
    events=[json.loads(line) for line in (ROOT/'server/test-results/latest.events.jsonl').read_text(encoding='utf-8').splitlines() if line.strip()]
    passed=[item for item in events if item.get('Action')=='pass' and item.get('Test')]
    failed=[item for item in events if item.get('Action') in ['fail','build-fail']]
    skipped=[item for item in events if item.get('Action')=='skip' and item.get('Test')]
    assert go['exit_code']==0 and go['passed'] and len(passed)==len(go['passed_tests'])==len(go_output['tests'])
    assert len(failed)==len(go['failed_tests'])==0 and len(skipped)==len(go['skipped_tests'])==0
    assert {(e['Package'],e['Test']) for e in passed}=={(e['package'],e['test']) for e in go['passed_tests']}=={(e['package'],e['test']) for e in go_output['tests']}
    assert sha(BASE/'checks/go.txt')==go_meta['output_sha256']
    assert sha(ROOT/'server/test-results/latest.events.jsonl')==go['events_sha256']
    ws_in_full=[{'test':item['Test'],'elapsed_seconds':item.get('Elapsed'),'package':item['Package']} for item in passed if item['Test']=='TestPetRuntimeCorpseHealthyExpiryAndColdEntryMongoWS']
    assert len(ws_in_full)==1
    config_drift=current_drift(config['input_sha256'])
    artifacts_drift=current_drift(config['artifact_sha256'])
    go_drift=current_drift(go['source_sha256_after'])
    assert not config_drift and not artifacts_drift and not go_drift
    catalog=load(ROOT/'config/development/pet-runtime-candidates.json')
    fixture=load(ROOT/'config/development/first-loop.json')
    assert catalog['definitions']==fixture['pet_runtime']['definitions'] and catalog['budgets']==fixture['pet_runtime']['budgets']
    missing_full=[name for name in ['config/development/pet-runtime-candidates.json','tools/server/test.py'] if name not in go['source_sha256']]
    assert len(go['source_sha256'])==153 and len(passed)==852 and not missing_full
    assert go['source_sha256']==go['source_sha256_after'] and go['race_detector'] and go['mongo_e2e_explicit']
    client=load(BASE/'client-review/report.json')
    assert client['stable'] and client['before']==client['after']==client['current']==client['current_frozen']
    assert client['input_files']==len(client['current'])==220 and not current_drift(client['current'])
    assert all(row['exit_code']==0 for row in client['commands'])
    assert client['full_ui_app_actual_counts']=={'tests':348,'pass':348,'fail':0,'skipped':0}
    assert client['pb']=={'message_count':68,'int64_field_count':136,'int64_boundary_cases':444}
    client_raw=(BASE/'client-review/full-ui-app.txt').read_text(encoding='utf-8')
    for key,value in [('tests',348),('pass',348),('fail',0),('skipped',0)]:
        assert re.search(r'(?:#|ℹ) '+key+r' '+str(value)+r'(?:\r?\n|$)',client_raw),(key,value)
    town=selected_browser('browser-town',7,0)
    ui=selected_browser('browser-ui',6,1)
    gui_selection_path=BASE/'gui-review/final-run.json'
    gui_selection=load(gui_selection_path)
    gui_path=BASE/'gui-review'/gui_selection['run_directory']/'report.json'
    assert gui_path.resolve().is_relative_to((BASE/'gui-review').resolve())
    assert sha(gui_path)==gui_selection['report_sha256']
    gui=load(gui_path)
    assert gui['passed'] and gui['corpse_ttl_ms']==15000
    assert all(gui[key] for key in ['main_restored','qa_database_dropped','qa_stopped','protected_unchanged']) and not gui['cleanup_errors']
    assert gui['browser']['passed'] and (gui['browser']['checks'],gui['browser']['errors'],gui['browser']['warnings'])==(5,0,0)
    gui_raw_path=Path(gui['browser']['report'])
    assert gui_raw_path.resolve().is_relative_to(gui_path.parent.resolve())
    gui_raw=load(gui_raw_path)
    assert gui_raw['passed'] and len(gui_raw['checks'])==5 and not gui_raw['errors'] and not gui_raw['warnings']
    tools_path=BASE/'gui-review'/gui_selection['tools_report_directory']/'report.json'
    assert tools_path.resolve().is_relative_to((BASE/'gui-review').resolve())
    tools=load(tools_path)
    assert tools['passed'] and tools['stable'] and tools['before']==tools['after']==tools['current']
    assert len(tools['current'])==4 and not current_drift(tools['current'])
    doc='docs/职业宠物生命周期与召唤实施边界.md'
    doc_text=(ROOT/doc).read_text(encoding='utf-8')
    clarifications=[]
    for text,suggestion in [('本阶段拟将当前道士入场免费骷髅','改为后续17阶段目标，避免与本轮rule16底座混读'),
                            ('选入运行仍须唯一Excel候选','可明确DataRange16已落PetDefs定义，本轮legacy不训练')]:
        if text in doc_text:
            clarifications.append({'line':next(i+1 for i,line in enumerate(doc_text.splitlines()) if text in line),'text':text,'suggestion':suggestion})
    result={'schema_version':1,'audited_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),
        'mode':'只读既有日志/JSON/当前摘要；未运行DB、Go或Python测试；未改冻结输入',
        'scope':'rule16尸体生命周期、定义/训练引擎与完整传输；17/购符/正常新生入口未开放',
        'python_configuration':{'test_methods':90,'exit_code':0,'input_count':len(config['input_sha256']),
            'before_after_stable':config['inputs_stable'],'current_drift':config_drift,
            'evidence':[evidence('.work/pet-runtime-20261029/checks/'+name) for name in ['summary.json','commands.json','python-configuration.txt','inputs-before.json','inputs-after.json','candidate-check.txt','fixture-check.txt']]},
        'standalone_real_mongo_ws':{'test_methods':1,'pass':1,'skip':0,'exit_code':ws['exit_code'],
            'race':True,'isolated_corpse_ttl_ms':10000,'case_elapsed_seconds':14.85,'package_elapsed_seconds':20.338,
            'wrapper_elapsed_seconds':ws['elapsed_seconds'],'input_count':len(ws_before),'before_after_stable':ws_before==ws_after,
            'current_drift':current_drift(ws_after),'drift_scope_note':'执行期间稳定；其后其它集成测试的变更由最终全Go重新覆盖，不能把此清单的137项说成全部当前字节',
            'evidence':[evidence('.work/pet-runtime-20261029/ws-runtime/'+name) for name in ['real-mongo-ws-result.json','real-mongo-ws.txt','real-mongo-ws-before.json','real-mongo-ws-after.json','review.md']]},
        'latest_full_go':{'passed_test_nodes':len(passed),'failed_test_nodes':len(failed),'skipped_test_nodes':len(skipped),
            'exit_code':go['exit_code'],'real_mongo':go['mongo_e2e_explicit'],'race':go['race_detector'],
            'source_count':len(go['source_sha256']),'before_after_stable':go['source_sha256']==go['source_sha256_after'],
            'current_drift':go_drift,'fixture_sha256':go['fixture']['sha256'],'events_sha256':go['events_sha256'],
            'started_at_utc':go['started_at_utc'],'finished_at_utc':go['finished_at_utc'],
            'command':go['command'],'pet_ws_reexecuted':ws_in_full,'missing_manifest_items':missing_full,
            'evidence':[evidence(name) for name in ['.work/pet-runtime-20261029/checks/go.json','.work/pet-runtime-20261029/checks/go.txt','server/test-results/latest.json','server/test-results/latest.events.jsonl']]},
        'client':{'input_count':220,'before_after_current_frozen_equal':True,'current_drift':[],
            'ui_app_counts':client['full_ui_app_actual_counts'],'pb':client['pb'],
            'fixture_sha256':client['fixture_sha256'],'evidence':[evidence(BASE/'client-review/report.json'),evidence(BASE/'client-review/full-ui-app.txt')]},
        'selected_browser_evidence':{'town':town,'ui':ui,'corpse_gui':{'pass':True,'checks':5,'errors':0,'warnings':0,
            'run_id':gui['run_id'],'corpse_ttl_ms':15000,'main_restored':True,'qa_database_dropped':True,'qa_stopped':True,'protected_unchanged':True,
            'selection':evidence(gui_selection_path),'selected_report':evidence(gui_path),'raw_browser_report':evidence(gui_raw_path),
            'tools_report_directory':gui_selection['tools_report_directory'],'tools_report':evidence(tools_path),
            'tools_input_count':4,'tools_before_after_current_equal':True,'tools_current_drift':[]}},
        'configuration':{'runtime_rule_version':fixture['rule_version'],'candidate_runtime_enabled':catalog['runtime_enabled'],
            'candidate_runtime_same_definitions_and_budgets':True,'legacy_untrained':fixture['pet_runtime']['bootstrap_mode'],
            'skill17_selected':any(skill['id']==17 for skill in fixture['skills']),'material_enabled':fixture['casting_supply']['enabled'],
            'main_corpse_ttl_ms':catalog['definitions'][0]['lifecycle']['corpse_ttl_ms'],'body_budget':catalog['budgets'],
            'final_artifact_sha256':config['artifact_sha256']},
        'document_review':{'document':evidence(doc),'product_blockers':[],'suggested_clarifications':clarifications},
        'evidence_limitations':['独立WS验证局部HP1/宠攻击0/怪伤20与命中255/长怪动作/TTL10秒，不冒称主180秒或17自然业务验收',
            'NoMaster友方grid过滤由game专项覆盖，WS未施放材料关闭的14/15',
            '原始before反例与中间失败不得与最终成功合并计数；Enter版本基线错误不是产品缺陷',
            '容量、手机真机、护符单位、正常17/Recall、自然训练成长与完整三职业未因本轮通过而完成'],
        'evidence_findings':([{'kind':'manifest_scope','severity':'证据口径','message':'最新全Go报告未含候选JSON/测试驱动脚本，当前驱动代码已包含；旧137报告只能声称其实际列明范围稳定，不能当成新清单通过'}] if missing_full else [])}
    (BASE/'final-audit.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps({'python':90,'ws_inputs':len(ws_before),'full_go':len(passed),'full_go_inputs':len(go['source_sha256']),
        'client_inputs':220,'gui':[5,0,0],'town':[7,0,0],'ui':[6,0,1],'static_tools_current':4,
        'full_go_missing_items':missing_full,'standalone_ws_current_drift':result['standalone_real_mongo_ws']['current_drift'],
        'artifact_current_drift':artifacts_drift,'document_product_blockers':0},ensure_ascii=False))

if __name__=='__main__':main()
