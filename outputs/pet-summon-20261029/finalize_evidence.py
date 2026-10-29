"""召唤与跨图规则的限定证据；主profile闭门、隔离前态与正常GUI分开记录。"""
from pathlib import Path
import datetime,hashlib,json,re,shutil
ROOT=Path(__file__).resolve().parents[2];WORK=Path(__file__).parent;OUT=ROOT/'outputs/pet-summon-20261029'
read=lambda p:json.loads(p.read_text(encoding='utf-8'))
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def write(p,value):
 p.parent.mkdir(parents=True,exist_ok=True);p.write_text(json.dumps(value,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
f=read(ROOT/'config/development/first-loop.json');go=read(ROOT/'server/test-results/latest.json');build=read(ROOT/'docs/客户端构建检查.json');client=read(WORK/'client-review/report.json')
assert f['rule_version']==17 and len(f['skills'])==22 and not any(s['id']==17 for s in f['skills']) and not f['casting_supply']['enabled'] and f['pet_runtime']['bootstrap_mode']=='legacy_untrained_pet'
assert not read(ROOT/'config/development/summon-candidates.json')['runtime_enabled'] and read(ROOT/'config/development/pet-runtime-candidates.json')['schema_version']==2
assert go['passed'] and go['race_detector'] and go['mongo_e2e_explicit'] and not go['skipped_tests'] and not go['failed_tests'] and not go['source_changed_during_test']
assert go['source_sha256']==go['source_sha256_after'] and 'config/development/summon-candidates.json' in go['source_sha256']
for path,digest in go['source_sha256'].items():assert sha(ROOT/path)==digest,path
assert client['stable'] and client['before']==client['after']==client['current']==client['current_frozen']
assert all(c['exit_code']==0 for c in client['commands'])
for path,digest in client['current'].items():assert sha(ROOT/path)==digest,path
assert build['success'] and build['exit_code']==36 and build['scene_prefab_layout_preserved'] and build['source_stable_during_build']
for path,digest in {**build['script_hashes'],**build['asset_hashes']}.items():assert sha(ROOT/path)==digest,path
py=read(WORK/'checks/final/summary.json');assert py['passed'] and py['inputs_stable'] and py['input_count']==14 and all(c['exit_code']==0 for c in py['commands'])
for path,digest in py['inputs'].items():assert sha(ROOT/path)==digest,path
assert 'Ran 100 tests' in (WORK/'checks/final/python.txt').read_text(encoding='utf-8')
for task in ['references','pet-sources','resources','fixture','config','protocol','protobuf','bindings','scene-compile','scene','host','go','main-status','mongodb-status','client-status']:
 d=read(WORK/'checks'/f'{task}.json');assert d['exit_code']==0,task
 assert sha(ROOT/d['output'])==d['output_sha256'],task
ws=read(WORK/'ws-runtime/run-011627/report.json');assert ws['passed'] and ws['real_mongo_ws'] and ws['inputs_stable'] and ws['input_sha256']==ws['input_sha256_after']
for path,digest in ws['input_sha256_after'].items():assert sha(ROOT/path)==digest,path
def selected(name):
 s=read(WORK/f'{name}-selection.json');p=WORK/s['report_directory']/'report.json'
 assert p.resolve().is_relative_to(WORK.resolve()) and sha(p)==s['report_sha256']
 return read(p)
town=selected('browser-town');ui=selected('browser-ui')
assert town['passed'] and town['bindingVerified'] and len(town['checks'])==7 and not town['errors'] and not town['warnings']
assert ui['passed'] and len(ui['checks'])==6 and not ui['errors']
selection=read(WORK/'gui-review/final-run.json');p=WORK/'gui-review'/selection['run_directory']/'report.json'
assert p.resolve().is_relative_to((WORK/'gui-review').resolve()) and sha(p)==selection['report_sha256']
gui=read(p);assert gui['passed'] and gui['main_restored'] and gui['qa_stopped'] and gui['qa_database_dropped'] and gui['protected_unchanged'] and not gui['cleanup_errors']
assert gui['browser']['passed'] and gui['browser']['checks']==5 and gui['browser']['errors']==0
main=read(WORK/'checks/main-status.txt');mongo=read(WORK/'checks/mongodb-status.txt');static=read(WORK/'checks/client-status.txt')
assert main['running'] and len(main['health'])==3 and all(h['status']=='ok' and h['rule_version']==17 and h['fixture_hash']==sha(ROOT/'config/development/first-loop.json') for h in main['health'].values())
assert mongo['running'] and mongo['endpoint_verified'] and mongo['port']==27119 and mongo['version']=='8.0.34' and static['running']
assets=read(WORK/'assets-unchanged.json');assert assets['all_saved_layout_bytes_unchanged'] and assets['all_source_bytes_unchanged']
scene=int(re.search(r'ℹ pass (\d+)',(WORK/'checks/scene.txt').read_text(encoding='utf-8')).group(1));assert scene==16
mapping={}
def copy(source,target):
 dest=OUT/target;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(source,dest)
 mapping[source.relative_to(ROOT).as_posix()]=dest.relative_to(ROOT).as_posix()
for p in sorted(WORK.rglob('*')):
 if p.is_file() and p.suffix in {'.json','.jsonl','.txt','.log','.md','.png','.xlsx','.py','.cjs','.go','.patch'} and '__pycache__' not in p.parts:
  copy(p,p.relative_to(WORK))
for source,target in [('server/test-results/latest.json','server/latest.json'),('server/test-results/latest.events.jsonl','server/latest.events.jsonl'),('docs/客户端构建检查.json','creator/report.json'),('.work/creator-cli.log','creator/creator-cli.txt'),('.work/reference-copies-check.json','references.json')]:copy(ROOT/source,target)
write(OUT/'evidence-paths.json',mapping)
(OUT/'README.md').write_text('''# 召唤与跨图宠物规则限定证据

主rule17仍22技能、原初始书/商店/升级选择、legacy免费未训宠与关闭护符。独立summon-candidates的17/书25/三阶升级完整但runtime_enabled=false。主NoPets两图显式false属保持旧旅行行为的技术候选，不是认证原服地图政策。

server/latest与checks/go为最终开启Mongo/WS的全包Go race，数量包括单元及子测试；ws-runtime/run-011627是真实协议隔离链路，run-011126保留NoPets断言未先等通用CD的测试前态失败。样本19级、书25、升级2书5页、护符两Count/容量2、怪HP1/伤害0/敏捷0/长动作及禁宠Town均明确为局部cfg和随机库前态。随后真实学习/装卸/扣耗/原Front叠格出生/玩家击杀确认EXP训练/升级预览提交/无装符Recall/旧回执/异图Owned/健康与冷建走正常协议，不是自然购书购符、主包规格或17开放。完整全包再次执行该WS。

server-core保留完整game探索中的死亡重复baseline失败及修正，四个peer边界各自修前红/修后绿：同图NoPets过度入图冻结、ScheduleID回绕、Frozen异图恢复多跑本帧AI、初始NoPets免费宠漏冻结。config-peer保留旧RecallName空串混旧schema以及少付升级书两条真实反例、修后overlay通过。类型/Store/来源peer是只读审查，不冒称执行了WS。

client-review含完整224输入冻结和358 UI/app；69PB/140个Long/458边界、16场景与真实Creator分别记录。before-preview-fix及before-rule17-tools是旧通过检查点，不能取代最新源码摘要。gui-review为主17语义下的隔离HP1/15秒尸体5项，随机marker自库清理并恢复main；browser-town/ui为主22技能正常业务。没有以这些旧宠GUI冒称17正常施法或新生视觉验收。两书82格/460公式及不变的架构书由逐格/ZIP检查证明，所有运行值仍只维护唯一Excel。evidence-paths映射原.work路径，原始报告内部路径和SHA不改写。
''',encoding='utf-8',newline='\n')
config=read(ROOT/'config/reports/配置校验报告.json');preview=read(ROOT/'config/releases/preview_current.json')
report={'schema_version':1,'date':'2026-10-29','recorded_at_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),
 'stage':'召唤与跨图宠物规则、Owned观察及完整原子材料链路',
 'status':'隔离业务与观察底座限定验收通过；主17/购符仍关闭',
 'runtime':{'rule_version':17,'preview':preview['version'],'fixture_sha256':sha(ROOT/'config/development/first-loop.json'),'skills':22,'summon17_enabled':False,'casting_supply_enabled':False,'bootstrap_mode':'legacy_untrained_pet','legacy_pet_trains':False,'summon_candidate_runtime_enabled':False,'pet_candidate_schema_version':2,'record_count':config['record_count'],'release_blockers':len(config['release_blockers']),'runtime_ready':False,'production_ready':False},
 'implementation':{'explicit_map_no_pets_separate_from_no_teleport':True,'case_sensitive_frozen_recall_name_not_display_or_definition_id':True,'recall_cast_intent_no_material_and_fresh_atomic_material_rpc':True,'stale_mode_failure_receipt_never_consumes_amulet_or_forges_success':True,'original_job_reservation_preserved_through_unknown_commit':True,'fresh_original_front_static_walk_then_current_foot_fallback':True,'fresh_checks_no_pets_at_acceptance_not_again_at_due':True,'accepted_request_direction_and_source_newborn_down_left':True,'recall_tracks_original_birth_across_world_moves':True,'spawn_lock_only_blocks_normal_walk_attack_not_auto_recall':True,'recall_preserves_original_pet_clocks_life_training':True,'frozen_return_skips_only_pet_ai_for_that_step':True,'initial_and_actual_cross_map_entry_apply_policy_once':True,'same_map_transition_does_not_apply_entry_freeze':True,'master_death_archives_resident_pet_with_one_terminal_baseline':True,'shared_schedule_counter_never_wraps_and_checks_total_allocation_before_fee':True,'owned_snapshot_excludes_corpses_reservations_and_other_world_coordinates':True,'all_baseline_chunks_and_cancel_ack_include_identical_owned_metadata':True,'upgrade_preview_describes_future_birth_not_damage_or_current_pet_mutation':True,'old_json_contract_rejects_new_keys_even_empty_or_null':True,'upgrade_requires_target_rank_plus_one_canonical_books':True},
 'verification':{'go_full_mongo_ws_race_passed_including_subtests':len(go['passed_tests']),'go_skipped':0,'go_failed':0,'go_inputs':len(go['source_sha256']),'go_source_stable':True,'configuration_python_passed':100,'configuration_inputs':14,'javascript_ui_app_passed':client['full_ui_app_actual_counts']['pass'],'javascript_scene_passed':scene,'javascript_total_passed':client['full_ui_app_actual_counts']['pass']+scene,'client_inputs':len(client['current']),'pb_messages':69,'pb_long_fields':140,'pb_boundary_cases':458,'new_protocol_ids':0,'creator_seconds':build['seconds'],'creator_exit_code':36,'saved_layouts_preserved':10,'source_town_artifact_files_unchanged':assets['source_artifact_files'],'isolated_summon_ws_seconds':ws['seconds'],'isolated_summon_ws_inputs':len(ws['input_sha256']),'isolated_corpse_gui_run_id':gui['run_id'],'isolated_corpse_gui_checks':5,'isolated_corpse_gui_ttl_ms':gui['corpse_ttl_ms'],'isolated_corpse_gui_errors':gui['browser']['errors'],'isolated_corpse_gui_warnings':gui['browser']['warnings'],'normal_touch_town_checks':7,'normal_touch_town_errors':town['errors'],'normal_touch_town_warnings':town['warnings'],'normal_ui_checks':6,'normal_ui_errors':ui['errors'],'normal_ui_warnings':ui['warnings'],'physical_mobile_verified':False,'capacity_verified':False},
 'adaptations':['Fresh/Recall以不同协议维护经济操作身份，客户端旧观察遇模式变化明确拒绝，保留原资源；来源无此乐观协议层','训练仍采用永久角色EXP确认后原birth一次应用，非来源对象即时GainExp调度全等；经验uint64检查而非uint32环绕','单活动宠、共128身体预算与普通健康旅行沿已登记本作规则；NoPets原图驻留、同图不额外Teleport','MYF模板/护符Shape5与Crystal BoneFamiliar/Shape0为明确跨来源候选；等级选择Crystal19/22/26，MYF19/23/26差异保留'],
 'limits':['护符包Count单位/售价仍待用户答复，主187.max_stack仍0，17/购符/初始赠书和商店选择不开放','真实WS依赖明确隔离等级/原书/升级输入/两Count，不能声称自然养成、正常购书购符或主pack已认证','新生表现/召唤音效等尚未进行正常17触屏业务验收；本轮GUI只回归原免费宠与Owned观察，不冒称已从UI召唤','候选三阶完整数据与真实0→1升级通过不代表自然1→19/22/26供给或全部阶正常获取渠道完成','128身体/64KiB/最大uint64序号为组件边界，不等于真实并发容量；手机真机、完整P0、完整三职业及目标1.80继续待实现/验收']}
report['evidence_sha256']={p.relative_to(ROOT).as_posix():sha(p) for p in sorted(OUT.rglob('*')) if p.is_file()}
write(ROOT/'docs/召唤与跨图宠物规则阶段验收.json',report)
print(json.dumps({'passed':True,'go':len(go['passed_tests']),'go_inputs':len(go['source_sha256']),'js':report['verification']['javascript_total_passed'],'creator':build['seconds'],'evidence_files':len(report['evidence_sha256'])},ensure_ascii=False))
