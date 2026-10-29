"""首次脚手架原件；后续已按真实字段适配，禁止重跑覆盖最终工作脚本。"""
from pathlib import Path
import json
import subprocess

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).resolve().parent
OLD=ROOT/'.work/inventory-capacity-20261030'
if any((WORK/name).exists() for name in ['finalize_evidence.py','verify_index.py','source-allowlist.json']):
    raise SystemExit('首次脚手架已生成；禁止覆盖后续受审适配，直接使用最终工作脚本。')
base=(OLD/'finalize_evidence.py').read_text(encoding='utf-8')
core=base[:base.index('def configuration_evidence():')]
core=core.replace('rule21归档草案','rule22归档草案').replace('outputs/inventory-capacity-20261030','outputs/equipment-growth-20261030').replace('docs/普通满包与背包触屏阶段验收.json','docs/基础装备属性与正常穿脱阶段验收.json').replace("BASE_HEAD = '5b6be439c013d16c4ca0bf0372c294e47b29a141'","BASE_HEAD = '2a5616ac883c91336edb227d7bbf87e9cfe64791'")
core=core.replace("OFFLINE_WS_TEST = 'TestFullBagOfflineCombatAndChargingMongoWS'\nGUI_CONTRACT_REVIEWED = True","GUI_CONTRACT_REVIEWED = False  # 最终正常UI工具落定后，只读审查真实字段才可改为True。")
core=core.replace("    need(OFFLINE_WS_TEST is not None, '真实离线满包WS测试名称尚未审定，不放行归档')\n    required = {'TestFullBagContinuousSourceRewardsAndHostingReceiptsMongo', 'TestAutoRecyclePolicyLockAndOnlineOfflineMongoWS', OFFLINE_WS_TEST}","    required = {'TestEquipmentTransactionsVersionsReceiptsMongo', 'TestEquipmentWearSwapUnequipAndReconnectMongoWS', 'TestEquipmentNonZeroMagicSpiritRoundtripMongoWS'}")
core=core.replace("'缺离线满包WS/在线WS/Store原事件PASS'","'缺装备原事务/主零属性WS/两职非零隔离WS原PASS'")
archive=base[base.index('class Archive:'):base.index('def curate(')]
tail=r'''
def configuration_evidence():
    folder=WORK/'config'; value=read(folder/'final-review.json'); candidate=read(folder/'candidate-review.json')
    need(value['status']=='configuration_complete_and_frozen' and value['rule_version']==22, '配置不是冻结rule22')
    current(value['current_hashes'],'配置冻结')
    need(candidate['changed_cells']==value['changed_workbook_cells']==6 and candidate['formulas_preserved']==value['formulas_preserved']==460, '配置格数/公式与候选不符')
    need(candidate['untouched_zip_members_verified']==value['untouched_zip_members_verified']==101 and value['architecture_workbook_preserved'], '未编辑ZIP/架构书不符')
    before=folder/'before'/BOOK; after=ROOT/BOOK
    need(sha(before)==candidate['source_before_sha256'] and sha(after)==candidate['candidate_sha256'], '源书前后原SHA不同')
    with zipfile.ZipFile(before) as left, zipfile.ZipFile(after) as right:
        need(left.namelist()==right.namelist(),'工作簿ZIP成员变化')
        changed=[name for name in left.namelist() if left.read(name)!=right.read(name)]
        need(changed==candidate['changed_zip_members'] and len(changed)==2 and len(left.namelist())-len(changed)==101,'实际ZIP差异不同')
        formulas=[]
        for archive in [left,right]:
            formulas.append({(name,cell.attrib['r']):ET.tostring(cell) for name in archive.namelist() if name.startswith('xl/worksheets/') and name.endswith('.xml')
                for cell in ET.fromstring(archive.read(name)).findall('.//s:c',NS) if cell.find('s:f',NS) is not None})
        need(formulas[0]==formulas[1] and len(formulas[0])==460,'实际460公式变化')
    for row in value['commands']:
        command=read(folder/row['report']); need(command['exit_code']==row['exit_code']==0,'配置命令失败')
        for key in ['stdout','stderr']:need(sha(folder/command[key])==command[key+'_sha256'],'配置原命令日志漂移')
    raw=(folder/'python-full.stderr.txt').read_text(encoding='utf-8'); match=re.search(r'^Ran (\d+) tests? in ',raw,re.M)
    need(match and int(match[1])==value['python_tests'] and re.search(r'^OK\s*$',raw,re.M),'配置原Python计数不同')
    fixture,public=read(ROOT/FIXTURE),read(ROOT/PUBLIC)
    need(fixture['rule_version']==public['rule_version']==22 and len(fixture['skills'])==22 and all(s['id']!=17 for s in fixture['skills']),'主配置/技能选择变化')
    need(not fixture['casting_supply']['enabled'] and fixture['pet_runtime']['bootstrap_mode']=='legacy_untrained_pet','主护符或训练暗启')
    need(all(i['id']!=220 for i in fixture['items']),'220不属于本阶段运行选择')
    need(public['equipment_recycling']=={'mode':'development_gold_only','outputs':[]},'开发GoldOnly合同变化')
    eq=[i for i in fixture['items'] if i['is_equipment']]
    need([(i['id'],i['recycle_gold'],i['equipment_stage']) for i in eq]==[(218,50,0),(193,400,0)],'主装备范围/原回收候选变化')
    for item in eq:need(all(type(item[k]) is int and item[k]==0 for k in ['magic_min_bonus','magic_max_bonus','tao_min_bonus','tao_max_bonus']),'主零MC/SC被替换为隔离数值')
    summon=read(ROOT/'config/development/summon-candidates.json')
    need(summon['runtime_enabled'] is False and summon['required_rule_version']==17,'召唤候选最低版本/关闭状态变化')
    pointer,sync=read(ROOT/'config/releases/preview_current.json'),read(ROOT/'conf/toolchain/config-binding-sync.json')
    need(pointer['version']==value['preview']==sync['config_version'] and pointer['version'].startswith('dev-equipment-foundation-v0.22-') and sync['generated_only'],'预览/绑定不是当前22')
    return value,fixture,public


def layout_evidence(build):
    need(git('rev-parse','HEAD').decode().strip()==BASE_HEAD,'不是阶段22基线HEAD')
    names={p.decode() for p in git('ls-tree','-r','--name-only','-z','HEAD','client/assets').split(b'\0') if p and Path(p.decode()).suffix in {'.scene','.prefab'}}
    need(len(names)==10 and names==set(build['asset_hashes']),'10保存布局集合变化')
    need(all(git('show','HEAD:'+name)==(ROOT/name).read_bytes() for name in names),'阶段22不得更改任何保存布局')
    old={p.decode() for p in git('ls-tree','-r','--name-only','-z','HEAD','client/assets').split(b'\0') if p}
    protected={name for name in old if name!=PUBLIC and (not name.startswith('client/assets/scripts/') or name.endswith('.meta'))}
    need(all(git('show','HEAD:'+name)==(ROOT/name).read_bytes() for name in protected),'旧PNG/地图/meta/保存资源被改变')
    return {'saved_layouts_total':10,'saved_layouts_intentionally_changed':0,'saved_layouts_unchanged_against_head':10,
        'layouts_preserved_during_build':10,'previous_tracked_assets_preserved':len(protected)}


def gui_evidence(path,creator_path,creator,fixture,public):
    # 故意阻断：只能根据最终freeze的实际工具/报告字段补充业务断言，不能用通用passed冒充正常穿脱验收。
    need(GUI_CONTRACT_REVIEWED,'装备正常GUI合同尚未完成真实工具字段审查；不生成正式通过')
    raise ValueError('待接最终正常UI report：必须核原票/原物品/穿脱版本/背包守恒/HPMP不恢复/健康重入与来源工具前后/current')


def write_new(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    with path.open('x',encoding='utf-8',newline='\n') as stream:stream.write(json.dumps(value,ensure_ascii=False,indent=2)+'\n')


def curate(args,protocol_outputs,shots):
    archive,history=Archive(),[]
    for key,target in [('go_report','go'),('client_report','client'),('creator_report','creator'),('gui_report','gui')]:
        archive.bundle(getattr(args,key),'current/'+target,'current_final_selected_scope')
    for path in protocol_outputs:archive.add(path,Path('current/client/protocol')/path.name,'current_protocol_raw')
    for path in shots:archive.add(path,Path('current/gui')/path.name,'current_actual_gpu_screenshot_not_generated_art')
    cfg=read(WORK/'config/final-review.json')
    names=['baseline.json','candidate-review.json','editor-review.json','edit-plan.json','application.json','final-review.json','author_configuration.py','verify_configuration.py','run_checks.py','update_current_tests.py','current-test-adaptations.json']
    for row in cfg['commands']:names += [row['name']+suffix for suffix in ['.json','.stdout.txt','.stderr.txt']]
    for name in sorted(set(names)):archive.add(WORK/'config'/name,Path('current/config')/name,'configuration_six_cells_original_byteproof')
    for name in [BOOK,'config/schema/catalog.json','tools/resources/prepare_first_loop.py','tools/resources/prepare_summon_candidates.py']:
        archive.add(WORK/'config/before'/name,Path('before')/name,'before_exact_source_not_current')
    for attempt in ['candidate','candidate-2']:
        for suffix in ['.json','.stdout.txt','.stderr.txt']:archive.add(WORK/'config'/(attempt+suffix),Path('history/config')/(attempt+suffix),'historical_author_tool_failure_not_gameplay')
    for name in ['baseline.json','source-audit.json','same-server-client-audit.json','client-audit.json','server-audit.json','peer-review.json']:
        archive.add(WORK/name,Path('support/source-audits')/name,'read_only_audit_scope_as_recorded_not_runtime_certification')
    for folder in ['source-samples','same-server-samples']:
        for path in sorted((WORK/folder).glob('*.png')):archive.add(path,Path('support/source-audits')/folder/path.name,'read_only_native_pixels_not_runtime_selection_or_shape_mapping')
    for name in ['audit_sources.py','audit_same_server_client.py','capture_baseline.py','collect_client_scope.py','stage_checks.py','finalize_evidence.py','verify_index.py','stage_verified.py','update_docs.py','source-allowlist.json','curate-plan.json','prepare_evidence_tools.py']:
        archive.add(WORK/name,Path('sources')/WORK.relative_to(ROOT)/name,'source_at_archive_for_reproduction')
    archive.add(ROOT/'tools/client/verify_equipment_browser.cjs','sources/tools/client/verify_equipment_browser.cjs','actual_gui_tool_source_at_archive')
    selected={getattr(args,key).resolve() for key in ['go_report','client_report','creator_report','gui_report']}
    candidates=set(args.history_report)
    for group in ['go','client','creator']:
        candidates|={p.resolve() for p in (WORK/group).glob('full-*/report.json') if p.resolve() not in selected}
    for path in sorted(candidates):
        need(path.is_relative_to(WORK),'历史报告越界')
        report=archive.bundle(path,Path('history')/path.parent.relative_to(WORK),'historical_exact_state_not_current_pass')
        history.append({'path':path.relative_to(ROOT).as_posix(),'sha256':sha(path),'passed_at_that_time':report.get('passed'),'exit_code_at_that_time':report.get('exit_code'),
            'scope':'historical_exact_raw_scope_no_reclassification_as_final_pass'})
        for image in sorted(path.parent.glob('*.png')):archive.add(image,Path('history')/path.parent.relative_to(WORK)/image.name,'historical_partial_screenshot_not_final')
    for path in args.support_report:
        need(path.is_relative_to(WORK),'支持报告越界')
        archive.bundle(path,Path('support')/path.parent.relative_to(WORK),'owner_narrow_or_isolated_scope_as_recorded_not_final_full')
    return archive,history


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    for name,default in [('go-report','go/full-1/report.json'),('client-report','client/full-1/report.json'),('creator-report','creator/full-1/report.json')]:parser.add_argument('--'+name,type=Path,default=WORK/default)
    parser.add_argument('--go-events',type=Path);parser.add_argument('--gui-report',type=Path,required=True)
    parser.add_argument('--history-report',type=Path,action='append',default=[]);parser.add_argument('--support-report',type=Path,action='append',default=[])
    mode=parser.add_mutually_exclusive_group();mode.add_argument('--check',action='store_true',help='默认只读门禁，不写正式docs/outputs');mode.add_argument('--apply',action='store_true',help='仅Root最终授权后使用，不覆盖旧正式证据')
    args=parser.parse_args()
    for key in ['go_report','client_report','creator_report','gui_report']:setattr(args,key,local(getattr(args,key)))
    args.go_events=local(args.go_events) if args.go_events else args.go_report.parent/'events.jsonl'
    args.history_report=[local(p) for p in args.history_report];args.support_report=[local(p) for p in args.support_report]
    config,fixture,public=configuration_evidence()
    go,gc=go_evidence(args.go_report,args.go_events)
    client,cc,protocol_outputs=client_evidence(args.client_report)
    creator=creator_evidence(args.creator_report);layout=layout_evidence(creator)
    gui,shots,observations=gui_evidence(args.gui_report,args.creator_report,creator,fixture,public)
    archive,history=curate(args,protocol_outputs,shots)
    validation=read(ROOT/'config/reports/配置校验报告.json')
    need(validation['structure_valid'] and validation['runtime_ready'] is False and validation['release_blockers'],'正式发布阻断不可隐藏')
    stage={'schema_version':1,'started_date':'2026-10-30','date':datetime.now(timezone.utc).astimezone(timezone(timedelta(hours=8))).date().isoformat(),'recorded_at_utc':datetime.now(timezone.utc).isoformat(),
        'stage':'基础装备属性与正常穿脱','status':'limited_passed','scope':'现有木剑/布衣的装备属性与明确穿脱开发基础；不是新增装备内容或三职业完整成长验收',
        'runtime':{'rule_version':22,'preview':config['preview'],'fixture_sha256':sha(ROOT/FIXTURE),'public_sha256':sha(ROOT/PUBLIC),'skills':len(fixture['skills']),
            'summon17_enabled':False,'casting_supply_enabled':False,'bootstrap_mode':'legacy_untrained_pet','legacy_pet_trains':False,'table_count':validation['table_count'],'record_count':validation['record_count'],
            'release_blockers':len(validation['release_blockers']),'runtime_ready':False,'production_ready':False,'equipment_item_ids':[218,193],'source220_enabled':False},
        'verification':{'go_full_mongo_ws_race_passed_including_subtests':gc['passed'],'go_inputs':gc['inputs'],'go_failed':0,'go_skipped':0,'go_source_stable':True,
            'configuration_python_passed':config['python_tests'],'javascript_ui_app_passed':cc['app'],'javascript_scene_passed':cc['scene'],'javascript_total_passed':cc['total'],'client_inputs':cc['inputs'],
            'client_external_engine_inputs':cc['engine_inputs'],'client_external_font_inputs':cc['font_inputs'],**{key:cc[key] for key in ['pb_messages','pb_long_fields','pb_boundary_cases','pb_protocol_ids']},
            'creator_seconds':creator['seconds'],'creator_exit_code':36,'gameplay_excel_changed_cells':6,'formulas_preserved':460,'untouched_zip_members':101,'architecture_workbook_preserved':True,**layout},
        'main_creator':{'passed':True,'seconds':creator['seconds'],'exit_code':36,'source_stable':True},
        'gui':{'passed':True,'checks':gui['checks'],'errors':gui['errors'],'warnings':gui['warnings'],'observed':observations,'seeded':False,'privateBusinessInvoked':False},
        'raw_go_counts':gc,'historical_attempts':history,'selectors':{key:{'path':getattr(args,key).relative_to(ROOT).as_posix(),'sha256':sha(getattr(args,key))} for key in ['go_report','go_events','client_report','creator_report','gui_report']},
        'limits':['主218/193四项MC/SC来源值均为0；非零MC/SC及等级资格仅隔离测试，不当主服装备成长或自然供给',
            '乌木剑220仍未选入；Price3000不是本任务回收/商店价；原body/Shape/Looks跨引擎编号和PAK覆盖未认证',
            '50/400仅development_gold_only候选、outputs=[]；正式洗炼石与祝福油、损坏回收、持久及修理未验收',
            '主仍22技能、legacy免费未训宠、护符关闭、17独立目录runtime_enabled=false，不新增初始赠书',
            'T031仅完成已验基础子范围；真机、整体容量、完整P0、目标1.80及三职业完整装备成长仍未完成'],
        'capacity_verified':False,'physical_mobile_verified':False,'production_ready':False,'full_p0_completed':False}
    current(go['source_sha256'],'归档前Go');need(snapshot('client')==client['before'],'归档前client漂移')
    for job in archive.jobs.values():need(sha(ROOT/job['source'])==job['raw_sha256'],'证据原件漂移')
    summary={'preflight':'passed','archive_executed':False,'formal_report_written':False,'go':gc['passed'],'javascript':cc['total'],'python':config['python_tests'],'gui_checks':len(gui['checks']),'archive_files_planned':len(archive.jobs)}
    if not args.apply:print(json.dumps(summary,ensure_ascii=False));return
    need(not OUT.exists() and not FORMAL.exists(),'不能覆盖正式报告/归档')
    records=archive.apply()
    write_new(OUT/'evidence-manifest.json',{'schema_version':1,'scope':'显式白名单，原字节或无损gzip；不复制引擎/工程缓存/整输入树/二进制/凭证','files':records})
    write_new(OUT/'evidence-paths.json',{r['source']:{'path':(OUT/r['target']).relative_to(ROOT).as_posix(),'encoding':r['encoding'],'raw_sha256':r['raw_sha256'],'file_sha256':r['file_sha256']} for r in records})
    with (OUT/'README.md').open('x',encoding='utf-8',newline='\n') as stream:stream.write('# 基础装备属性与正常穿脱限定证据\n\ncurrent仅取终态通过且输入当前匹配的实证；support保留作者窄测/隔离资格的明确范围；history保留原失败不追认通过。10份保存布局及旧资源全保持。\n\n主仅木剑/布衣，四项MC/SC为原来源零值。非零两职业测试不是主服自然供给或完整成长。220/身体外观/正式回收与修理未认证；50/400仅开发GoldOnly，洗炼石与祝福油未开放。整体容量、真机、完整P0及全1.80继续待验。\n')
    stage['evidence_sha256']={(OUT/r['target']).relative_to(ROOT).as_posix():r['file_sha256'] for r in records}
    for name in ['README.md','evidence-manifest.json','evidence-paths.json']:stage['evidence_sha256'][(OUT/name).relative_to(ROOT).as_posix()]=sha(OUT/name)
    stage['evidence_file_count']=len(stage['evidence_sha256'])
    current(go['source_sha256'],'归档后Go');need(snapshot('client')==client['before'],'归档后client漂移')
    write_new(FORMAL,stage);print(json.dumps({**summary,'archive_executed':True,'formal_report_written':True},ensure_ascii=False))


if __name__=='__main__':main()
'''
(WORK/'finalize_evidence.py').write_text(core+tail[:tail.index('def write_new')]+archive+tail[tail.index('def write_new'):],encoding='utf-8',newline='\n')

# 原blob索引校验器保持成熟解析/解压算法，只调整本阶段名字、保护范围与精确路径白名单。
index=(OLD/'verify_index.py').read_text(encoding='utf-8')
index=index.replace('阶段21','阶段22').replace("BASE = '5b6be439c013d16c4ca0bf0372c294e47b29a141'","BASE = '2a5616ac883c91336edb227d7bbf87e9cfe64791'")
index=index.replace('docs/普通满包与背包触屏阶段验收.json','docs/基础装备属性与正常穿脱阶段验收.json').replace('outputs/inventory-capacity-20261030','outputs/equipment-growth-20261030').replace('dev-inventory-capacity-v0.21-','dev-equipment-foundation-v0.22-').replace('已推送rule20基线','已推送rule21基线')
start=index.index("ALLOWED = set('''");end=index.index("'''.split())",start)+len("'''.split())")
index=index[:start]+"ALLOWED = set(json.loads((WORK / 'source-allowlist.json').read_text(encoding='utf-8'))['allowed_paths'])"+index[end:]
index=index.replace("n not in {'client/assets/scenes/Game.scene', 'client/assets/resources/development/gameplay.json'}","n != 'client/assets/resources/development/gameplay.json'")
index=index.replace("        for name in creator['asset_hashes']:\n            if name != 'client/assets/scenes/Game.scene':\n                need(indexed[name] == heads[name], '其它9布局变更：' + name)","        for name in creator['asset_hashes']:\n            need(indexed[name] == heads[name], '阶段22的10布局不得改变：' + name)")
index=index.replace("'intentional_layout_changes': ['client/assets/scenes/Game.scene']","'intentional_layout_changes': []")
(WORK/'verify_index.py').write_text(index,encoding='utf-8',newline='\n')

raw=subprocess.check_output(['git','status','--porcelain=v1','--untracked-files=all','-z'],cwd=ROOT)
allowed={item[3:].decode('utf-8') for item in raw.split(b'\0') if item}
allowed={name for name in allowed if not name.startswith('config/releases/dev-')}
allowed|={'README.md','client/README.md','config/README.md','docs/开发阶段与验收清单.md','docs/正式开发推进记录.md','docs/架构适配与规则优先级.md','docs/客户端构建检查.json','docs/装备基础与明确穿脱实施边界.md','docs/基础装备属性与正常穿脱阶段验收.json','server/test-results/latest.json','server/test-results/latest.events.jsonl'}
(WORK/'source-allowlist.json').write_text(json.dumps({'schema_version':1,'baseline':'2a5616ac883c91336edb227d7bbf87e9cfe64791','status':'draft_explicit_paths_for_root_final_review','scope':'只列明确生产/测试/配置/协议/说明路径；最终Go修复若新增文件须逐路径审阅，不能通配放行','allowed_paths':sorted(allowed)},ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps({'prepared':['finalize_evidence.py','verify_index.py','source-allowlist.json'],'formal_written':False,'archive_executed':False,'index_or_production_modified':False},ensure_ascii=False))
