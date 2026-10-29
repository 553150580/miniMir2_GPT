"""归档rule16限定验收；保留隔离样本、原始失败及正常业务边界。"""
from pathlib import Path
import datetime,hashlib,json,re,shutil

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).parent
OUT=ROOT/'outputs/pet-runtime-20261029'
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()

def write(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')

fixture=read(ROOT/'config/development/first-loop.json')
go=read(ROOT/'server/test-results/latest.json')
build=read(ROOT/'docs/客户端构建检查.json')
client=read(WORK/'client-review/report.json')
assert fixture['rule_version']==16 and len(fixture['skills'])==22 and not any(s['id']==17 for s in fixture['skills']) and not fixture['casting_supply']['enabled']
assert fixture['pet_runtime']['enabled'] and fixture['pet_runtime']['bootstrap_mode']=='legacy_untrained_pet'
assert go['passed'] and go['race_detector'] and go['mongo_e2e_explicit'] and not go['skipped_tests'] and not go['failed_tests']
assert not go['source_changed_during_test'] and go['source_sha256']==go['source_sha256_after']
for required in ['config/development/pet-runtime-candidates.json','tools/server/test.py','tools/dev','tools/toolchain.py','conf/toolchain/windows-amd64.lock.json','资料参考/myf/Mir200/Map/0.map','server/cmd/toolchain-smoke/main.go']:
    assert required in go['source_sha256'],required
for file,digest in go['source_sha256'].items():assert sha(ROOT/file)==digest,file
assert build['success'] and build['exit_code']==36 and build['scene_prefab_layout_preserved'] and build['source_stable_during_build']
for file,digest in {**build['asset_hashes'],**build['script_hashes']}.items():assert sha(ROOT/file)==digest,file
assert client['stable'] and all(c['exit_code']==0 for c in client['commands'])
assert client['before']==client['after']==client['current']==client['current_frozen']
for file,digest in client['current'].items():assert sha(ROOT/file)==digest,file
python=read(WORK/'checks/summary.json')
assert python['inputs_stable'] and all(c['exit_code']==0 for c in python['commands'])
for file,digest in python['input_sha256'].items():assert sha(ROOT/file)==digest,file
assert 'Ran 90 tests' in (WORK/'checks/python-configuration.txt').read_text(encoding='utf-8')
for name in ['references','pet-sources','resources','fixture','scene-compile','scene','go','main-status','mongodb-status','client-status']:
    d=read(WORK/'checks'/f'{name}.json');assert d['exit_code']==0,name
    assert sha(ROOT/d['output'])==d['output_sha256'],name
def selected_browser(name):
    selected=read(WORK/f'{name}-selection.json')
    path=WORK/selected['report_directory']/'report.json'
    assert path.resolve().is_relative_to(WORK.resolve())
    assert sha(path)==selected['report_sha256']
    return read(path)
town=selected_browser('browser-town');ui=selected_browser('browser-ui')
assert town['passed'] and town['bindingVerified'] and len(town['checks'])==7 and not town['errors'] and not town['warnings']
assert ui['passed'] and len(ui['checks'])==6 and not ui['errors']
selection=read(WORK/'gui-review/final-run.json')
gui_path=WORK/'gui-review'/selection['run_directory']/'report.json'
assert gui_path.resolve().is_relative_to((WORK/'gui-review').resolve())
assert sha(gui_path)==selection['report_sha256']
gui=read(gui_path);assert gui['passed']
qa_tools_path=WORK/'gui-review'/selection['tools_report_directory']/'report.json'
assert qa_tools_path.resolve().is_relative_to((WORK/'gui-review').resolve())
qa_tools=read(qa_tools_path)
assert qa_tools['passed'] and qa_tools['stable'] and qa_tools['before']==qa_tools['after']==qa_tools['current']
for file,digest in qa_tools['current'].items():assert sha(ROOT/file)==digest,file
assert gui['main_restored'] and gui['qa_database_dropped'] and gui['qa_stopped'] and gui['protected_unchanged'] and not gui['cleanup_errors']
assert gui['browser']['passed'] and gui['browser']['checks']==5 and gui['browser']['errors']==0
main=read(WORK/'checks/main-status.txt');mongo=read(WORK/'checks/mongodb-status.txt');static=read(WORK/'checks/client-status.txt')
assert main['running'] and len(main['health'])==3 and all(d['status']=='ok' and d['rule_version']==16 and d['fixture_hash']==sha(ROOT/'config/development/first-loop.json') for d in main['health'].values())
assert mongo['running'] and mongo['endpoint_verified'] and mongo['port']==27119 and mongo['version']=='8.0.34' and static['running']
assets=read(WORK/'assets-unchanged.json');assert assets['all_saved_layout_bytes_unchanged'] and assets['all_source_bytes_unchanged']
scene=int(re.search(r'ℹ pass (\d+)',(WORK/'checks/scene.txt').read_text(encoding='utf-8')).group(1));assert scene==16
mapping={}
def copy(source,target):
    destination=OUT/target;destination.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(source,destination)
    mapping[source.relative_to(ROOT).as_posix()]=destination.relative_to(ROOT).as_posix()
for p in sorted(WORK.rglob('*')):
    if p.is_file() and p.suffix in {'.json','.jsonl','.txt','.log','.md','.png','.xlsx','.py','.cjs','.go'} and '__pycache__' not in p.parts:
        copy(p,p.relative_to(WORK))
for source,target in [('server/test-results/latest.json','server/latest.json'),('server/test-results/latest.events.jsonl','server/latest.events.jsonl'),('docs/客户端构建检查.json','creator/report.json'),('.work/creator-cli.log','creator/creator-cli.txt'),('.work/reference-copies-check.json','references.json')]:copy(ROOT/source,target)
write(OUT/'evidence-paths.json',mapping)
(OUT/'README.md').write_text('''# 宠物尸体与训练引擎限定证据

rule16主profile保留原免费legacy开发宠，明确不训练；22项技能不含17，护符与材料商店关闭。训练factory/逐阶数据/一次确认奖励引擎已实现，没有公开新生指令；此阶段不等于正常购符、学习、500ms召唤及Recall通过。

server/latest与checks/go是最终全包Go race（开启真实Mongo/WS，数量含各层单元及子测试）；pre-manifest-go保留首次852项通过但遗漏候选目录摘要的旧冻结清单，second-input-manifest-go保留补两项后的139输入通过记录。最终清单同时覆盖候选JSON、来源原图、全部cmd及受控工具选择链。独立ws-runtime使用局部HP1/怪物20/10秒TTL，通过真实怪击、健康新票、原期限及冷重建；其原始Enter revision比较错误保留first-real-enter-baseline。server-core保留同批训练奖励到上限后忽略极值加法的修复前失败，以及最终纯game race。

gui-review通过独立marker随机库、独立进程身份和本地配置，用正常保存UI观察HP1活宠被真实怪击、原对象尸体详情、无活宠面板、健康新票和原15秒截止移除。没有预置尸体、运行时造body、注入技能或改主配置；每轮finally停止自己节点、清理精确自库并恢复main，失败截图也保留。它不证明原180秒墙钟、自然成长、17或128容量。browser-town与browser-ui是主配置正常业务回归，Creator是真实119.5秒Web Mobile构建。

所有数值由唯一Excel和同源编译器派生；candidate-review/application-report/两书cell-diff/编辑计划记录82个授权单元格。空表首行编辑ValueError及JSON maximum未生效的反例均保留。client-review中的组件替身、纯模型和PB边界分别记录，不能冒充真机或容量验收。evidence-paths映射.work原路径，不改写原始报告内部摘要。
''',encoding='utf-8',newline='\n')
preview=read(ROOT/'config/releases/preview_current.json')
report={'schema_version':1,'date':'2026-10-29','recorded_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),
 'stage':'宠物原对象尸体、共享身体预算、逐阶训练与完整基线传输引擎',
 'status':'限定开发验收通过；完整17召唤与购符业务继续实施',
 'runtime':{'rule_version':16,'preview':preview['version'],'fixture_sha256':sha(ROOT/'config/development/first-loop.json'),'skills':22,'statuses':4,'summon17_enabled':False,'casting_supply_enabled':False,'bootstrap_mode':'legacy_untrained_pet','legacy_pet_trains':False,'candidate_file_runtime_enabled':False,'corpse_ttl_ms':180000,'retained_bodies_per_owner':128,'runtime_ready':False,'production_ready':False},
 'implementation':{'death_archives_original_object_handle_life_world':True,'corpse_is_not_friendly_after_master_cleared':True,'inactive_world_cleanup_uses_original_absolute_deadline':True,'pending_does_not_extend_deadline':True,'shared_budget_counts_active_corpses_reservations':True,'full_rejects_birth_never_death_or_discards_unexpired_corpses':True,'monotonic_handle_and_life_within_retained_execution_no_reuse':True,'new_birth_immutable_typed_definition_fighter_and_training':True,'legacy_bootstrap_life_zero_and_static_attributes_kept':True,'training_captures_original_birth_and_qualification_at_kill':True,'training_only_once_after_confirmed_owner_experience':True,'single_level_per_grant_excess_kept_no_free_healing':True,'at_cap_ignored_grant_never_performs_overflowing_addition':True,'temporary_pet_state_not_persisted_or_cold_restored':True,'uint64_training_experience_with_checked_arithmetic_and_client_Long':True,'real_encoded_baseline_bytes_checked_before_any_chunk_sent':True,'complete_chunked_exit_ack_fenced_to_original_request_and_attachment':True,'saved_pet_overview_and_entity_detail_observe_current_complete_baseline':True},
 'verification':{'go_full_mongo_ws_race_passed_including_subtests':len(go['passed_tests']),'go_skipped':0,'go_inputs':len(go['source_sha256']),'go_source_stable':True,'configuration_python_passed':90,'configuration_inputs':len(python['input_sha256']),'javascript_ui_app_passed':client['full_ui_app_actual_counts']['pass'],'javascript_scene_passed':scene,'javascript_total_passed':client['full_ui_app_actual_counts']['pass']+scene,'client_inputs':len(client['current']),'pb_messages':68,'pb_long_fields':136,'pb_boundary_cases':444,'new_protocol_ids':0,'creator_seconds':build['seconds'],'creator_exit_code':36,'saved_layouts_preserved':10,'source_town_artifact_files_unchanged':assets['source_artifact_files'],'isolated_corpse_gui_run_id':gui['run_id'],'isolated_corpse_gui_checks':5,'isolated_corpse_gui_ttl_ms':gui['corpse_ttl_ms'],'isolated_corpse_gui_errors':gui['browser']['errors'],'isolated_corpse_gui_warnings':gui['browser']['warnings'],'normal_touch_town_checks':7,'normal_touch_town_errors':town['errors'],'normal_touch_town_warnings':town['warnings'],'normal_ui_checks':6,'normal_ui_errors':ui['errors'],'normal_ui_warnings':ui['warnings'],'physical_mobile_verified':False,'capacity_verified':False},
 'adaptations':['训练在永久角色EXP成功确认后一次应用；资格与birth在击杀事实冻结，非来源逐对象即时GainExp调度全等','训练经验拓宽uint64并拒绝溢出，不模拟来源uint32环绕；一次grant只升一级及excess保留不变','单活动宠是本作策划规则；128含所有保留World活动体/尸体/预留，属于技术候选，不是容量认证','MYF模板与Crystal BoneFamiliar/AI23及180000ms尸体期限为明确跨来源候选，未认证原服实际配置'],
 'limits':['护符Count包单位与售价仍待用户答复，未按DuraMax猜张数；17未加入skill_ids，未开放材料商店','主profile仍免费legacy不训练；新生训练factory只在受控模型/隔离测试调用，不冒称正常召唤或自然训练已通过','NoPets地图政策、学习/购符/主动17、新生500ms原图/fallback、Recall及死亡再召仍待独立业务接线和验收','真实WS采用10秒局部TTL、GUI采用15秒；主Excel180秒由纯game绝对时钟和输入验证覆盖，不称180秒墙钟浏览器已测','128身体和64KiB分块边界已组件验证，不等于128身体真实GUI、并发容量或手机真机','完整三职业、自然1→19/25供给/自动回收、经济闭环、完整P0及目标1.80认证继续待实现或验收']}
report['evidence_sha256']={p.relative_to(ROOT).as_posix():sha(p) for p in sorted(OUT.rglob('*')) if p.is_file()}
write(ROOT/'docs/宠物尸体与训练引擎阶段验收.json',report)
print(json.dumps({'passed':True,'go':len(go['passed_tests']),'go_inputs':len(go['source_sha256']),'js':report['verification']['javascript_total_passed'],'creator':build['seconds'],'evidence_files':len(report['evidence_sha256'])},ensure_ascii=False))
