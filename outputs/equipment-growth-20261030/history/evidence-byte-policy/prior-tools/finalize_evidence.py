"""rule22归档草案；默认只读，全部终态门禁通过且Root显式--apply才新建正式产物。"""
from __future__ import annotations

import argparse
from collections import Counter
from copy import deepcopy
from datetime import datetime, timezone, timedelta
import gzip
import hashlib
import json
from pathlib import Path
import re
import runpy
import shutil
import subprocess
import sys
import xml.etree.ElementTree as ET
import zipfile

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
OUT = ROOT / 'outputs/equipment-growth-20261030'
FORMAL = ROOT / 'docs/基础装备属性与正常穿脱阶段验收.json'
BOOK = 'outputs/development-prep-20261026/玩法配置.xlsx'
ARCH = 'outputs/development-prep-20261026/架构配置.xlsx'
FIXTURE = 'config/development/first-loop.json'
PUBLIC = 'client/assets/resources/development/gameplay.json'
GAME = 'client/assets/scenes/Game.scene'
BASE_HEAD = '2a5616ac883c91336edb227d7bbf87e9cfe64791'
COMMANDS = {'models', 'full-ui-app', 'scene', 'strict', 'protocol-check', 'protocol-lint'}
FORBIDDEN = {'client-project', 'library', 'temp', 'build', 'node_modules', '__pycache__', '.git', 'inputs'}
ENGINE = Path('D:/Program Files/CocosCreator/3.8.8/resources/resources/3d/engine/cocos')
ENGINE_FILES = ['input/input.ts', '2d/event/pointer-event-dispatcher.ts', 'scene-graph/node-event-processor.ts',
                'scene-graph/node.ts', 'ui/button.ts', 'core/data/object.ts', 'scene-graph/component.ts',
                'audio/audio-source.ts', 'ui/toggle.ts']
FONTS = [Path('C:/Windows/Fonts/arial.ttf'), Path('C:/Windows/Fonts/msyh.ttc'), Path('C:/Windows/Fonts/simsun.ttc')]
NS = {'s': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
# 测试名称与工具静态接口已审；实际Go/Creator/GUI终态仍须逐项通过，不以候选字段放行。
GUI_CONTRACT_REVIEWED = True  # 已只读审tool-static-4真实字段；原终态/图审/协议事实仍逐项重核。
PUBLICATION_CONTRACT_REVIEWED = True  # 已静读最终成功/回滚接口；不把失败后恢复或部分切换追认通过。
CUTOVER_SHA256 = 'ed07059fd28023b842466176a6dde3f19eb7706d13a188e0ba488465b4997980'
CONTROLLED_JOB_SHA256 = '3c40395f94b1219f5ba0221cd0b0c2836186b11788e4a6d8482dc22281206df3'
CONTROLLED_PROBE_REVIEW_SHA256 = '38d698a7ff4efbecee787af5a4dc33b6567bfdaa7070b840e808701fa024cbbc'
PUBLIC_SNAPSHOT_TOOL_SHA256 = 'f18298a522696bdb1047c26d0039680f43e02a01a5d8134ecb1b78bcadd1caf1'
MONGO_IDENTITY_FIELDS = ('backend','pid','installation_id','project_key','data_path','uri','port','replica_set','version')


def need(ok, message):
    if not ok:
        raise ValueError(message)


def sha(path):
    value = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            value.update(block)
    return value.hexdigest()


def read(path):
    return json.loads(Path(path).read_text(encoding='utf-8'))


def local(value, base=ROOT):
    value = Path(value)
    path = (value if value.is_absolute() else base / value).resolve()
    need(path.is_relative_to(base.resolve()), '路径越界：' + str(path))
    need(not path.is_symlink() and not getattr(path, 'is_junction', lambda: False)(), '不使用链接：' + str(path))
    return path


def report_file(value,report_path):
    name=Path(value)
    return local(name) if name.is_absolute() or name.parts[0] in {'.work','client','server','tools','config','conf','docs'} else local(name,base=report_path.parent)


def support_command_logs(row,report):
    if {'stdout','stderr'}<=row.keys():
        values=[(report_file(row[key],report),row[key+'_sha256']) for key in ['stdout','stderr']]
    elif {'log','log_sha256'}<=row.keys():
        values=[(report_file(row['log'],report),row['log_sha256'])]
    else:
        # static-4原报告只记固定命令名及双流SHA；路径严格限定本轮4项，不推断其它报告。
        need(report==WORK/'client/tool-static-4/report.json' and row.get('name') in {'syntax','preflight','tool-tests','wrapper-dry-run'}
             and {'stdout_sha256','stderr_sha256'}<=row.keys(), '未知支持报告原日志接口')
        values=[(report.parent/(row['name']+'.'+key+'.log'),row[key+'_sha256']) for key in ['stdout','stderr']]
    for path,digest in values:need(path.is_file() and sha(path)==digest,'支持报告原日志未配对：'+str(path))
    return [path for path,_ in values]


def current(values, label):
    need(isinstance(values, dict) and values, label + '缺少输入集合')
    for name, digest in values.items():
        need(isinstance(digest, str) and re.fullmatch('[0-9a-f]{64}', digest), label + '非法SHA')
        path = local(name)
        need(path.is_file() and sha(path) == digest, label + '当前原字节不同：' + name)


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT)


def snapshot(group):
    if group == 'go':
        module = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='readonly_input_enumeration')
        paths = module['input_files']()
    else:
        module = runpy.run_path(str(WORK / 'stage_checks.py'), run_name='readonly_input_enumeration')
        paths = module['client_inputs']()
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(set(paths))}


def external(report, prefix, allowed):
    before, after = report[prefix + '_before'], report[prefix + '_after']
    need(before == after and {str(Path(n).resolve()) for n in before} == {str(p.resolve()) for p in allowed}, prefix + '集合不完整或前后漂移')
    for name, value in before.items():
        path = Path(name).resolve()
        need(path.is_file() and not path.is_symlink() and sha(path) == value, prefix + '当前字节不同：' + name)
    return len(before)


def logs(report):
    result = {}
    for row in report['commands']:
        need(row['name'] not in result and row['exit_code'] == 0, '命令重复或失败')
        for key in ['stdout', 'stderr']:
            path = local(row[key])
            need(path.is_file() and sha(path) == row[key + '_sha256'], '原始日志漂移：' + str(path))
        result[row['name']] = row
    return result


def wrapper(path, group):
    value = read(path)
    need(value['group'] == group and value['passed'] is True and value['stable'] is True and value['freeze_started_before_commands'] is True,
         group + '包装终态未通过')
    need(value['before'] == value['after'] == snapshot(group) and value['input_count'] == len(value['before']), group + '完整输入前后/current不同')
    commands = logs(value)
    expected = COMMANDS if group == 'client' else {'creator'} if group == 'creator' else {'full-mongo-race'}
    need(set(commands) == expected, group + '检查组缺失')
    if group != 'go':
        external(value, 'external_engine', [ENGINE / name for name in ENGINE_FILES])
        external(value, 'external_font', FONTS)
    return value, commands


def node_counts(path):
    result = {name: int(value) for name, value in re.findall(r'^(?:#|ℹ)\s+(tests|pass|fail|skipped|cancelled|todo)\s+(\d+)\s*$', Path(path).read_text(encoding='utf-8'), re.M)}
    need(set(result) == {'tests', 'pass', 'fail', 'skipped', 'cancelled', 'todo'}, '缺Node完整终态')
    need(result['pass'] == result['tests'] > 0 and all(result[k] == 0 for k in ['fail', 'skipped', 'cancelled', 'todo']), 'Node存在失败/跳过')
    return result['pass']


def json_output(path):
    text = Path(path).read_text(encoding='utf-8')
    for candidate in [text] + list(reversed(text.splitlines())):
        try:
            value = json.loads(candidate)
            if isinstance(value, dict):
                return value
        except json.JSONDecodeError:
            pass
    raise ValueError('缺JSON命令终态：' + str(path))


def go_evidence(report_path, events_path):
    value = read(report_path)
    wrapped, _ = wrapper(report_path.parent / 'wrapper.json', 'go')
    need(value['passed'] is True and value['exit_code'] == 0 and value['mongo_e2e_explicit'] is True and value['race_detector'] is True,
         '全包Go不是实际Mongo/race通过')
    need(value['single_rpc_and_business_deadlines_changed'] is False, '单项RPC/事务/租约/连接业务时限被放宽')
    need(all(arg in value['command'] for arg in ['./...', '-json', '-race']), '未执行完整全包')
    need(value['source_changed_during_test'] is False and value['source_sha256'] == value['source_sha256_after'] == wrapped['before'], 'Go原始输入与wrapper不一致')
    current(value['source_sha256'], 'Go')
    need(sha(events_path) == value['events_sha256'], 'Go raw events SHA不同')
    events = [json.loads(line) for line in events_path.read_text(encoding='utf-8').splitlines() if line.strip()]
    passed = [{'package': e['Package'], 'test': e['Test']} for e in events if e.get('Test') and e['Action'] == 'pass']
    skipped = [{'package': e['Package'], 'test': e['Test']} for e in events if e.get('Test') and e['Action'] == 'skip']
    failed = [{'package': e['Package'], 'test': e.get('Test')} for e in events if e['Action'] in {'fail', 'build-fail'}]
    need(passed == value['passed_tests'] and skipped == value['skipped_tests'] == [] and failed == value['failed_tests'] == [], 'Go报告不等raw终态或存在失败/跳过')
    required = {'TestEquipmentTransactionsVersionsReceiptsMongo','TestEquipmentWearSwapUnequipAndReconnectMongoWS','TestEquipmentNonZeroMagicSpiritRoundtripMongoWS','TestEquipmentLegacyWearCanBeRemovedAndReplacedMongo'}
    need(required <= {row['test'] for row in passed if row['package'] == 'minimir/internal/integration'}, '缺装备原事务/主零属性WS/两职非零隔离WS原PASS')
    need(value['fixture']['sha256'] == sha(ROOT / FIXTURE), 'Go不是当前fixture')
    return value, {'passed': len(passed), 'failed': 0, 'skipped': 0, 'inputs': len(value['source_sha256']), 'required_integration_tests': sorted(required)}


def client_evidence(path):
    value, commands = wrapper(path, 'client')
    app, scene = [node_counts(local(commands[name]['stdout'])) for name in ['full-ui-app', 'scene']]
    raw = local(commands['protocol-check']['stdout']).read_text(encoding='utf-8')
    tasks = [json.loads(line) for line in raw.splitlines() if line.strip()]
    need(tasks and all(row['exit_code'] == 0 for row in tasks), '协议子命令未通过')
    codec = [row for row in tasks if 'tools/protocol/verify_client_pb.cjs' in row['command']]
    need(len(codec) == 1, '缺唯一PB编码边界子报告')
    protocol_outputs = [local(row['output']) for row in tasks]
    codec_result = json_output(local(codec[0]['output']))
    lint = json_output(local(commands['protocol-lint']['stdout']))
    need(codec_result['status'] == 'passed' and lint['valid'] is True, '协议检查未通过')
    return value, {'app': app, 'scene': scene, 'total': app + scene, 'inputs': value['input_count'],
        'engine_inputs': len(value['external_engine_before']), 'font_inputs': len(value['external_font_before']),
        'pb_messages': codec_result['message_count'], 'pb_long_fields': codec_result['int64_field_count'],
        'pb_boundary_cases': codec_result['int64_boundary_cases'], 'pb_protocol_ids': lint['messages'], 'pb_error_codes': lint['error_codes']}, protocol_outputs


def creator_evidence(path):
    value = read(path)
    wrapped, _ = wrapper(path.parent / 'wrapper.json', 'creator')
    need(value['creator'] == '3.8.8' and value['platform'] == 'web-mobile' and value['exit_code'] == 36 and value['success'] is True,
         'Creator未真实正常退出36')
    need(value['source_stable_during_build'] is True and value['scene_prefab_layout_preserved'] is True, '构建期间源/10布局变化')
    current(value['asset_hashes'], 'Creator10布局')
    current(value['script_hashes'], 'Creator脚本')
    need(value['asset_hashes'].keys() <= wrapped['before'].keys() and all(wrapped['before'][k] == v for k, v in value['asset_hashes'].items()), 'Creator包装与布局不配对')
    need((path.parent / 'creator.log').is_file(), '缺保存原Creator日志')
    need(value['served_build_preserved'] is True, '候选构建未保护原主预览包')
    before,after=[read(path.parent/name) for name in ['served-build-before.json','served-build-after.json']]
    need(before==after and len(before)==value['served_build_files']>0,'构建时主包原SHA清单未保持')
    need(all(name.startswith('client/build/web-mobile/') and re.fullmatch('[0-9a-f]{64}',digest) for name,digest in before.items()),'受保护主包清单非法')
    build_root=local(value['build_root']);artifact=local(value['artifact_manifest']);config=local(value['build_config'])
    need(artifact==path.parent/'candidate-files.json' and sha(artifact)==value['artifact_manifest_sha256'],'Creator产物manifest未配对')
    files=read(artifact);actual={p.relative_to(ROOT).as_posix():sha(p) for p in sorted(build_root.rglob('*')) if p.is_file()}
    need(actual==files and len(files)==value['artifact_file_count']>0 and (build_root/'index.html').is_file(),'候选全产物原SHA与实际不同')
    need(all(local(name).is_relative_to(build_root) for name in files),'产物清单越界')
    need(sha(config)==value['build_config_sha256'] and sha(ROOT/'conf/creator-build.json')==value['original_build_config_sha256'],'构建配置原SHA不同')
    original,controlled=read(ROOT/'conf/creator-build.json'),read(config)
    original['buildPath']=str(build_root.parent)
    need(original==controlled,'候选构建配置除buildPath外发生变化')
    return value


def protocol_delta():
    manifest=read(ROOT/'shared/proto/protocol_manifest.json')
    old=json.loads(git('show','HEAD:shared/proto/protocol_manifest.json'))
    old_messages={row['value']:row for row in old['messages']};messages={row['value']:row for row in manifest['messages']}
    need(set(old_messages)==set(messages),'本阶段不增减协议消息号')
    for key in messages:
        new_row=deepcopy(messages[key]);old_row=deepcopy(old_messages[key])
        if key==40001:new_row.pop('source_ref');old_row.pop('source_ref')
        need(new_row==old_row,'不相关协议消息变化')
    old_errors={row['value']:row for row in old['error_codes']};errors={row['value']:row for row in manifest['error_codes']}
    need(set(errors)-set(old_errors)=={3401,3402,3403} and all(errors[k]==v for k,v in old_errors.items()),'错误码新增/旧定义不符')
    expected={3401:'CodeEquipmentBagFull',3402:'CodeEquipmentChanged',3403:'CodeEquipmentBlocked'}
    need(all(errors[k]['name']==name for k,name in expected.items()),'装备错误语义不符')
    source=(ROOT/'shared/proto/assets.proto').read_text(encoding='utf-8')
    body=re.search(r'message\s+EquipRequest\s*\{([^}]+)\}',source).group(1)
    fields=re.findall(r'\b(bytes|uint32|uint64|bool|OperationContext)\s+(\w+)\s*=\s*(\d+)\s*;',body)
    need(fields==[('bytes','item_instance_id','1'),('uint32','slot','2'),('OperationContext','operation','3'),('bool','unequip','4'),('uint64','item_version','5')],'EquipRequest原字段/新字段编号变化')
    return {'message_id':40001,'new_fields':{'unequip':4,'item_version':5},'new_errors':expected,'new_message_ids':[]}



def configuration_evidence():
    folder=WORK/'config'; value=read(folder/'final-review.json'); candidate=read(folder/'candidate-review.json')
    need(value['status']=='configuration_complete_and_frozen' and value['rule_version']==22, '配置不是冻结rule22')
    transition=runpy.run_path(str(folder/'shared_test_transition.py'),run_name='exact_shared_test_transition_only')
    current(transition['verify'](lambda name:local(name).read_bytes()),'配置冻结及明确共享测试迁移')
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
    heads={}
    for row in git('ls-tree','-r','-z','HEAD','client/assets').split(b'\0'):
        if row:
            metadata,name=row.split(b'\t',1);mode,kind,oid=metadata.split();need(kind==b'blob','旧资源不是普通blob');heads[name.decode('utf-8')]=oid
    names={name for name in heads if Path(name).suffix in {'.scene','.prefab'}}
    need(len(names)==10 and names==set(build['asset_hashes']),'10保存布局集合变化')
    need(all(git('show','HEAD:'+name)==(ROOT/name).read_bytes() for name in names),'阶段22不得更改任何保存布局')
    protected={name for name in heads if name!=PUBLIC and (not name.startswith('client/assets/scripts/') or name.endswith('.meta'))}
    # 一个cat-file批处理读取原blob，避免为两万份旧资源逐一启动git进程。
    definition=runpy.run_path(str(WORK/'verify_index.py'),run_name='readonly_blob_helpers');blobs=definition['Blobs']()
    try:
        for name in protected:need(blobs.content(heads[name])==sha(ROOT/name),'旧PNG/地图/meta/保存资源原字节被改变：'+name)
    finally:blobs.close()
    return {'saved_layouts_total':10,'saved_layouts_intentionally_changed':0,'saved_layouts_unchanged_against_head':10,
        'layouts_preserved_during_build':10,'previous_tracked_assets_preserved':len(protected)}


def public_snapshot_evidence(path,gui_path,creator_path,client_path,publication_path):
    need(path==WORK/'public-snapshot-probe/report.json','只接受本轮明确的公开JsonAsset只读快照')
    value,gui,creator,client=[read(p) for p in [path,gui_path,creator_path,client_path]]
    tool=path.parent/'probe.cjs';snapshot=path.parent/'loaded-public.json'
    need(value['passed'] is True and value['original_gui_passed'] is True and gui['passed'] is True,'公开JsonAsset快照或原GUI未通过')
    need(local(value['tool_path'])==tool and value['tool_sha256']==value['tool_sha256_after']==sha(tool)==PUBLIC_SNAPSHOT_TOOL_SHA256,'实际公开快照探针源前后/current不符')
    need(local(value['gui_report_path'])==gui_path and value['gui_report_sha256']==value['gui_report_sha256_after']==sha(gui_path),'快照未绑定原run02报告')
    need(value['frozen_qa_tool_sha256']==value['frozen_qa_tool_sha256_after']==gui['toolSha256']==gui['toolSha256After']==sha(ROOT/'tools/client/verify_equipment_browser.cjs'),'快照和原GUI工具不同')
    need(value['current_public_raw_sha256']==value['current_public_raw_sha256_after']==gui['publicSha256']==sha(ROOT/PUBLIC) and value['fixture_sha256']==gui['fixtureSha256']==sha(ROOT/FIXTURE),'快照不是当前public原字节/fixture')
    need(value['creator_report_sha256']==value['creator_report_sha256_after']==gui['creatorReportSha256']==sha(creator_path) and value['creator_build_root']==creator['build_root'],'快照未配对真实Creator')
    need(value['client_report_sha256']==sha(client_path) and value['cutover_report_sha256']==sha(publication_path),'快照未配对所选client/实际切主')
    need(value['source_before']==value['source_after']==client['before']==client['after'],'快照完整项目输入集合/前后变化')
    current(value['source_after'],'公开资源快照项目源')
    core={name:value['source_after'][name] for name in ['client/assets/scripts/app/WorldAssets.ts','client/assets/scripts/app/GameWorldView.ts']}
    need(value['core_source_before']==value['core_source_after']==core,'公开asset.json引用消费源不符')
    need(all(value[key] is True for key in ['no_login','no_database_writes','no_private_business','node_deep_strict_equal_all_public_fields'])
         and value['route_override'] is False and value['touch_input'] is False and value['account_request_count']==value['websocket_count']==0
         and value['page_errors']==value['console_errors']==value['warnings']==[],'公开快照越界或有运行错误')
    need(value['observed_scene']=='Login' and value['resource_key']=='development/gameplay' and value['asset_type']=='cc.JsonAsset','快照不是保存Login公开resources.load JsonAsset')
    need(local(value['snapshot_path'])==snapshot and sha(snapshot)==value['snapshot_sha256']==value['browser_snapshot_sha256']==value['expected_loaded_sha256']==gui['loadedPublicCanonicalSha256'],'实际loaded原串不等于原GUI观测摘要')
    published=read(WORK/'runtime-after.json');expected_health=[{'port':port,**published[str(port)]} for port in [18080,18100,18101,18102]]
    need(value['health_before']==value['health_after']==expected_health,'快照不是本次受控切主的同一4端身份')
    need({row['port']:row for row in gui['healthAfter']}=={row['port']:row for row in value['health_after'] if row['port']!=18080},'快照与原GUI后端实例不同')
    need(value['served_index_before_sha256']==value['served_index_after_sha256']==sha(ROOT/'client/build/web-mobile/index.html'),'快照期间当前主入口字节变化')
    wrapped=read(path.parent/'wrapper.json')
    inputs={p.relative_to(ROOT).as_posix() for p in [tool,path.parent/'run.py',ROOT/'tools/client/verify_equipment_browser.cjs',ROOT/PUBLIC,ROOT/'docs/客户端构建检查.json',ROOT/FIXTURE]}
    need(wrapped['exit_code']==0 and wrapped['stable'] is True and wrapped['before']==wrapped['after'] and set(wrapped['before'])==inputs and wrapped['probe_report_sha256']==sha(path),'快照包装器无真实终态或完整原输入')
    current(wrapped['after'],'公开快照包装器')
    need(wrapped['command'][-2:]==['node',str(tool)] and value['execution_argv'][1:]==[str(tool)],'快照实际命令不同')
    for stream in ['stdout','stderr']:need(sha(path.parent/(stream+'.log'))==wrapped[stream+'_sha256'],'快照原stdout/stderr变化')
    raw=json_output(path.parent/'stdout.log')
    need(raw=={'passed':True,'snapshot_sha256':sha(snapshot),'deep_equal':True,'account_requests':0,'websockets':0,'report_sha256':sha(path)},'快照原stdout终态不符')
    # 直接用锁定Node独立比较全部JSON字段，不在Python模拟ECMA数字格式或对象属性顺序。
    program="const fs=require('node:fs'),a=require('node:assert/strict'),c=require('node:crypto');const raw=JSON.parse(fs.readFileSync(process.argv[1],'utf8')),loaded=JSON.parse(fs.readFileSync(process.argv[2],'utf8'));a.deepStrictEqual(loaded,raw);console.log(JSON.stringify({same:true,node_version:process.version,node_executable:process.execPath,raw_node_stringify_sha256:c.createHash('sha256').update(JSON.stringify(raw)).digest('hex')}));"
    result=subprocess.run([sys.executable,str(ROOT/'tools/dev'),'node','-e',program,str(ROOT/PUBLIC),str(snapshot)],cwd=ROOT,stdin=subprocess.DEVNULL,capture_output=True,timeout=30)
    need(result.returncode==0,'锁定Node独立全字段deepStrictEqual未通过：'+result.stderr.decode('utf-8',errors='replace'))
    compared=json.loads(result.stdout)
    need(compared['same'] is True and compared['node_version']==value['node_version'] and compared['node_executable']==value['execution_argv'][0]
         and compared['raw_node_stringify_sha256']==value['raw_public_node_stringify_sha256'],'独立Node/原probe全字段比较身份不符')
    return {'passed':True,'scope':'公开JsonAsset原串补证关联原6项GUI；没有重做UI、登录或业务写入','snapshot_sha256':sha(snapshot),
        'gui_report_sha256':sha(gui_path),'public_raw_sha256':sha(ROOT/PUBLIC),'node_deep_strict_equal_all_fields':True,'node_version':value['node_version'],
        'raw_public_node_stringify_sha256':compared['raw_node_stringify_sha256'],'probe_report_sha256':sha(path)}


def gui_evidence(path,creator_path,creator,fixture,public,static_path,public_snapshot):
    need(GUI_CONTRACT_REVIEWED,'装备正常GUI合同尚未完成真实工具字段审查；不生成正式通过')
    value=read(path);tool=ROOT/'tools/client/verify_equipment_browser.cjs'
    static=read(static_path)
    need(static['passed'] is True and static['stable'] is True and static['before']==static['after']==static['current'],'GUI静态报告前后/current不符')
    current(static['current'],'GUI静态输入')
    need(static['current'][tool.relative_to(ROOT).as_posix()]==sha(tool) and static['chrome_started'] is False and static['creator_started'] is False,'最终工具未静态冻结或把静态检查当GUI')
    for command in static['commands']:
        need(command['exit_code']==0,'GUI静态命令失败')
        support_command_logs(command,static_path)
    if static_path==WORK/'client/tool-static-4/report.json':
        need([row['name'] for row in static['commands']]==['syntax','preflight','tool-tests','wrapper-dry-run'], 'static-4固定4命令集合/顺序不同')
        need(static['tool_test_count']==node_counts(static_path.parent/'tool-tests.stdout.log'), 'static-4原5工具测试终态计数不符')
    need(value['passed'] is True and value['expectedRuleVersion']==22 and value['checks'] and value['errors']==[],'正常GUI未真实终态通过')
    need(value['touchInput'] is True and all(value[key] is False for key in ['routeOverride','bodySeeded','seeded','privateBusinessInvoked','physicalMobileVerified','fullBagVerifiedByThisGui','positiveMcScVerifiedByThisGui']),'GUI路线越界或夸称隔离/容量范围')
    need(value['creatorReportSha256']==sha(creator_path) and value['creatorBuildRoot']==creator['build_root'],'GUI与真实候选构建不配对')
    frozen={**creator['asset_hashes'],**creator['script_hashes']}
    need(value['sourceBefore']==value['sourceAfter']==frozen,'GUI构建源前后集合不同')
    current(frozen,'GUI输入');need(value['toolSha256']==value['toolSha256After']==sha(tool),'GUI工具前后/current变化')
    need(value['fixtureSha256']==sha(ROOT/FIXTURE) and value['publicSha256']==sha(ROOT/PUBLIC),'GUI不是当前fixture/public')
    need(public_snapshot['passed'] is True and public_snapshot['node_deep_strict_equal_all_fields'] is True
         and public_snapshot['snapshot_sha256']==value['loadedPublicCanonicalSha256'] and public_snapshot['gui_report_sha256']==sha(path)
         and public_snapshot['public_raw_sha256']==value['publicSha256'],'原GUI loaded摘要未与实际公开快照/全部字段/当前原文件配对')
    health={}
    for key in ['healthBefore','healthAfter']:
        rows=value[key];need(isinstance(rows,list) and len(rows)==3 and {r['port'] for r in rows}=={18100,18101,18102},'GUI health三节点不完整')
        health[key]={r['port']:r for r in rows}
        for row in rows:need(row['status']=='ok' and row['development_only'] is True and row['rule_version']==22 and row['fixture_hash']==value['fixtureSha256'],'GUI三节点并非当前开发22')
    need(all(health['healthBefore'][p]['instance']==health['healthAfter'][p]['instance'] for p in health['healthBefore']),'GUI期间实例变化')
    published=read(WORK/'runtime-after.json')
    need(all(health['healthBefore'][p]['instance']==published[str(p)]['instance'] for p in health['healthBefore']),'GUI不是本次受控切主实例')
    wrapped,start=read(path.parent/'wrapper-result.json'),read(path.parent/'wrapper-start.json')
    expected_inputs={tool.relative_to(ROOT).as_posix(),'docs/客户端构建检查.json',FIXTURE,PUBLIC}
    need(wrapped['exit_code']==0 and wrapped['inputs_stable'] is True and wrapped['run_requested'] is True and wrapped['browser_report_sha256']==sha(path),'浏览器包装器未真实通过')
    need(wrapped['inputs']==wrapped['inputs_after']==start['inputs'] and set(wrapped['inputs'])==expected_inputs,'浏览器包装器输入集合或前后变化')
    current(wrapped['inputs'],'浏览器包装器');need(start['wrapper_sha256']==sha(path.parent/'run.py'),'浏览器包装器源变化')
    need(sha(path.parent/'tool-at-start.cjs')==value['toolSha256'],'运行时实际工具原字节未保留')
    for key in ['product_source_modified','service_control','database_seeding']:need(wrapped[key] is False,'GUI包装器越界')
    for stream in ['stdout','stderr']:need(sha(path.parent/(stream+'.log'))==wrapped[stream+'_sha256'],'GUI原stdout/stderr变化')
    raw=json_output(path.parent/'stdout.log')
    need(raw['passed'] is True and raw['checks']==len(value['checks']) and raw['errors']==len(value['errors']) and raw['warnings']==len(value['warnings']),'GUI原stdout终态计数不同')
    observations=value['observations'];initial=observations['initial'];before_reentry=observations['beforeReentry'];reentry=observations['reentry']
    need(value['createdJobs']==[0] and initial['job']==0 and initial['rule']=='22' and initial['auto'] is False and initial['hostingAuto'] is False,'不是正常新战士明确关闭Auto')
    swords=[row for row in initial['items'] if row['item']==218];cloth=[row for row in initial['items'] if row['item']==193]
    need(len(swords)==2 and len(cloth)==1 and not any(row['slot']==0 for row in initial['equipped']),'不是原两木剑/布衣与初始空武器槽')
    a,b=[row['id'] for row in swords];cloth_id=cloth[0]['id'];original_ids={a,b,cloth_id}
    def weapon_visible(state):
        rows=[row for row in state['weapon']['layers'] if row['package']=='Weapon'];need(len(rows)==1 and type(rows[0]['active']) is bool,'缺权威own候选Weapon层观察')
        return rows[0]['active']
    def locate(state,id,worn):
        rows=[row for row in state['equipped' if worn else 'items'] if row['id']==id];need(len(rows)==1,'原物品不存在或多重归属')
        return rows[0]
    def identity(old,new,worn):
        keys=['id','item','quantity','bound','quality','sourceKind','sourceId','locked']
        need(all(old[k]==new[k] for k in keys),'迁移改了原物品身份/来源/绑定/品质/锁')
        need(int(new['version'])==int(old['version'])+1 and 0<int(old['version'])<2**63-1,'原件迁移版本未恰+1')
        need(type(old['flags']) is int and type(new['flags']) is int and old['flags']&6==0 and new['flags']==(old['flags']|8 if worn else old['flags']&~8) and bool(new['flags']&1)==new['locked'],'穿脱flags除8外变化或6非零')
    def state_guard(old,new):
        need(all(old[k]==new[k] for k in ['roleKey','binding','scene','life','map']),'穿脱跨原角色/票/场景/生命')
        for key in ['maxHp','maxMp']:need(old[key]==new[key],'穿脱更改资源上限')
        need(0<int(new['hp'])<=int(old['hp']) and 0<=int(new['mp'])<=int(old['mp']),'穿脱恢复资源/复活或角色已死')
        need(all(old['weapon'][k]==new['weapon'][k] for k in ['visualToken','animationToken','motionToken','headY','profile']),'穿脱重建表现/动作/位移对象')
        if old['weapon']['pose']['action'] in ['attack','casting','struck'] and old['weapon']['pose']['action']==new['weapon']['pose']['action'] and old['attackSequence']==new['attackSequence'] and old['castSequence']==new['castSequence']:
            need(old['weapon']['started']==new['weapon']['started'],'同一身体动作被穿脱重启')
        need(new['auto'] is False and new['hostingAuto'] is False and not new['error'],'穿脱代改Auto或产生错误')
        for id in original_ids:need(sum(row['id']==id for row in new['items']+new['equipped'])==1,'原三件装备吞物/重复归属')
    attr_pairs=[('physicalAttackMin','physical_attack_min'),('physicalAttackMax','physical_attack_max'),('magicAttackMin','magic_attack_min'),('magicAttackMax','magic_attack_max'),('spiritAttackMin','spirit_attack_min'),('spiritAttackMax','spirit_attack_max'),('physicalDefenceMin','physical_defence_min'),('physicalDefenceMax','physical_defence_max'),('magicDefenceMin','magic_defence_min'),('magicDefenceMax','magic_defence_max')]
    labels=[('02-cloth',cloth_id,False,193,1),('03-first-weapon',a,False,218,1),('04-replace',b,False,218,0),('05-unequip',b,True,218,-1),('06-restore',b,False,218,1)]
    operations=[row for row in value['operations'] if row['id']==40001]
    need(len(operations)==len(labels) and len({row['opId'] for row in operations})==len(labels),'原5次穿脱缺失/重复或健康重播')
    for label,id,unequip,item_id,sign in labels:
        row=observations[label];old,new=row['afterDetailsReturn'],row['after'];request,result=row['request'],row['result'];item=locate(old,id,unequip)
        need(row['touch']['id']==id and row['sameEvent']['id']==id and row['sameEvent']['before']['nav']==row['sameEvent']['after']['nav'] and row['sameEvent']['before']['offset']==row['sameEvent']['after']['offset'],'不是原真实触点或同事件清导航/偏移')
        offset='roleOffset' if unequip else 'inventoryOffset';need(old[offset]==row['open'][offset],'详情返回改下层列表偏移')
        need(row['confirmation']['message'] and row['lines'] and row['scroll'] and row['scroll'][-1]['offset']>=row['scroll'][-1]['max']-2,'未普通滚读完整详情/确认窗')
        need(request in operations and request['itemInstanceId']==id and request['itemVersion']==item['version'] and request['unequip'] is unequip and request['rule']=='22' and request['binding']==new['binding'],'原40001请求未绑定原实例/版本/方向/票')
        definition=next(d for d in public['items'] if d['item_id']==item_id);need(request['slot']==definition['equip_slot'],'请求槽不同唯一装备定义')
        need(result in value['results'] and result['opId']==request['opId'] and result['binding']==request['binding'] and result['pending'] is False and result['error']==0,'缺原op可信提交结果')
        need(int(request['revision'])<int(result['revision'])<=int(new['revision']),'原提交修订水位不合法')
        identity(item,locate(new,id,not unequip),not unequip);state_guard(old,new)
        for camel,key in attr_pairs:need(int(new['attributes'][camel])==int(old['attributes'][camel])+definition['base_attributes'][key]*sign,'权威装备属性增减不同来源')
    lock=observations['lockOriginalSword'];unlocked=locate(lock['before'],a,False);locked=locate(lock['after'],a,False)
    need(unlocked['locked'] is False and locked['locked'] is True and locked['flags']==unlocked['flags']|1 and int(locked['version'])==int(unlocked['version'])+1,'不是实际锁A且只推进原锁位/版本')
    need(locate(observations['03-first-weapon']['after'],a,True)['locked'] is True,'为了穿戴擅自解锁原A')
    identity(locate(observations['03-first-weapon']['after'],a,True),locate(observations['04-replace']['after'],a,False),False)
    need(observations['03-first-weapon']['sameEvent']['before']['nav']['target']==observations['groundNavigation']['touch']['target'],'真实地面目标未在同次选择保留')
    visible={'initial':False,'02-cloth':False,'03-first-weapon':True,'04-replace':True,'05-unequip':False,'06-restore':True}
    for key,expected in visible.items():need(weapon_visible(observations[key] if key=='initial' else observations[key]['after']) is expected,'权威空槽/穿入未控制已有Weapon层')
    on,off=observations['emptySlotAutoOn'],observations['emptySlotAutoOff']
    need(on['auto'] is True and on['hostingAuto'] is True and off['auto'] is False and off['hostingAuto'] is False and not weapon_visible(on) and not weapon_visible(off),'空槽错误代改用户Auto')
    need(all(before_reentry[k]==reentry[k] for k in ['roleKey','life','scene','map','auto','hostingAuto','attackSequence','castSequence']) and before_reentry['binding']!=reentry['binding'],'不是同原生命现场的健康新票')
    need(all(before_reentry[k]==reentry[k] for k in ['items','equipped','attributes','maxHp','maxMp']) and int(reentry['hp'])<=int(before_reentry['hp']) and int(reentry['mp'])<=int(before_reentry['mp']),'健康新票改变装备/属性或补满资源')
    need(not any(r['binding']==reentry['binding'] for r in value['results']) and all(op['binding']!=reentry['binding'] for op in operations),'健康新票重播旧经济结果/穿脱')
    if before_reentry['weapon']['pose']['action']=='standing':need(reentry['weapon']['pose']['action'] not in ['attack','casting','appear'],'健康新票重播身体动作')
    need(weapon_visible(reentry),'健康新票丢失原武器层')
    shots=[local(name,base=path.parent) for name in value['screenshots']]
    need(shots and len({p.name for p in shots})==len(shots) and all(p.is_file() and p.suffix.lower()=='.png' for p in shots),'实际GPU截图集合不完整')
    visual=read(path.parent/'visual-review.json')
    need(visual['visual_passed'] is True and visual['report_sha256']==sha(path) and set(visual['screenshots'])=={p.name for p in shots},'缺实际逐图复核或不是本报告')
    need(visual['screenshot_sha256']=={p.name:sha(p) for p in shots},'实际看图字节与当前PNG不同')
    return value,shots,{'created_job':0,'equipment_operations':len(operations),'locked_a_stayed_locked':True,'original_equipment_items':3,
        'positive_mc_sc_by_gui':False,'full_bag_by_gui':False,'same_scope_life_visuals_during_moves':True,'health_new_binding_without_receipt_replay':True,'screenshots':len(shots)}


def controlled_job_evidence(task,phase):
    # 只读原进程元数据与真实退出日志；没有terminal时不以PID消失或环境健康代替成功。
    need((task,phase) in {('probe','probe-01'),('cutover','cutover-01')}, '只核本轮明确的固定受控任务')
    directory=local(WORK/'controlled-jobs'/phase)
    need(directory.is_dir() and sha(WORK/'controlled_job.py')==CONTROLLED_JOB_SHA256, '受控执行器未冻结或任务目录缺失')
    names=['launch.json','spawn.json','worker.json','approved.json','task.json','terminal.json','stdout.log','stderr.log']
    need(all((directory/name).is_file() for name in names), '受控任务缺原登记、批准、子任务或终态/日志')
    launch,spawn,worker,approved,child,terminal=[read(directory/name) for name in names[:6]]
    run_id=launch['run_id']; python=Path(launch['python_executable'])
    need(re.fullmatch('[0-9a-f]{32}',run_id) and launch['task']==task and launch['phase']==phase and launch['project_root']==str(ROOT) and launch['directory']==str(directory), '受控任务附着不同')
    need(python==Path(getattr(sys,'_base_executable',sys.executable)).resolve() and sha(python)==launch['python_sha256'], '受控任务锁定Python原字节不同')
    argv=[str(python),str(WORK/'controlled_job.py'),'_worker',task,'--phase',phase,'--run-id',run_id]
    need(launch['worker_argv']==worker['argv']==argv and worker['run_id']==run_id and worker['task']==task and worker['phase']==phase and worker['project_root']==str(ROOT), '受控worker原argv/run_id不同')
    need(worker['executable']==str(python) and type(worker['pid']) is int and worker['pid']>0 and type(worker['start_ticks']) is int and worker['start_ticks']>0, '受控worker映像/PID/创建时钟缺失')
    need(spawn['run_id']==approved['run_id']==child['run_id']==terminal['run_id']==run_id and spawn['pid']==worker['pid'] and approved['worker']==child['worker']==terminal['worker']==worker, '受控spawn/批准/任务/终态未配对原worker')
    need(terminal['task']==task and terminal['phase']==phase and terminal['passed'] is True and terminal['exit_code']==terminal['worker_exit_code']==0 and terminal['source_stable'] is True, '受控任务不是实际子任务/worker退出0且源码稳定')
    need(launch['automatic_restart'] is False and launch['stop_api'] is False and terminal['automatic_restart'] is False and not ({'error','source_after_error'} & set(terminal)), '受控任务重启、停止API或失败不可接受')
    sources=[WORK/'controlled_job.py',ROOT/'tools/process_control.py',ROOT/'tools/dev',ROOT/'tools/dev.ps1',ROOT/'tools/toolchain.py',ROOT/'conf/toolchain/windows-amd64.lock.json']
    if task=='cutover':sources.append(WORK/'cutover.py')
    expected={p.relative_to(ROOT).as_posix():sha(p) for p in sources}
    need(launch['source_before']==terminal['source_before']==terminal['source_after']==expected, '受控任务完整源before/after/current不同')
    current(expected,'受控任务源')
    if task=='cutover':
        need(expected[(WORK/'cutover.py').relative_to(ROOT).as_posix()]==CUTOVER_SHA256, '受控任务不是已审ed070切换原字节')
        command=[str(python),str(WORK/'cutover.py')]
        need('probe_duration_seconds' not in terminal, '实际切主不能用probe代替')
    else:
        command=[str(python),'-c','import time; print("fixed probe started", flush=True); time.sleep(2); print("fixed probe completed", flush=True)']
        need(terminal['probe_duration_seconds']==2 and terminal['seconds']>=2, 'probe不是固定2秒真实子任务')
    need(child['command']==terminal['task_command']==command, '受控任务命令不是固定已审任务')
    identity=child['child_identity']
    need(identity and type(identity['pid']) is int and identity['pid']>0 and identity['pid']==child['pid'] and identity['pid']!=worker['pid'] and identity['executable']==str(python) and type(identity['start_ticks']) is int and identity['start_ticks']>worker['start_ticks'], '实际子进程PID/映像/创建时钟未与原worker配对')
    for stream in ['stdout','stderr']:need(sha(directory/(stream+'.log'))==terminal[stream+'_sha256'], '受控任务终态后原日志变化')
    parse=lambda text:datetime.fromisoformat(text.replace('Z','+00:00'))
    created,spawned,approved_at,child_at,started,finished=[parse(value) for value in [launch['created_at_utc'],spawn['spawned_at_utc'],approved['approved_at_utc'],child['started_at_utc'],terminal['started_at_utc'],terminal['finished_at_utc']]]
    need(created<=spawned<=approved_at<=child_at<=finished and created<=started<=child_at and terminal['seconds']>0, '受控任务时序不一致')
    marker=json.loads((directory/'stdout.log').read_text(encoding='utf-8').splitlines()[0])
    need(set(marker)=={'run_id','task','phase','task_started_at_utc'} and marker['run_id']==run_id and marker['task']==task and marker['phase']==phase and approved_at<=parse(marker['task_started_at_utc'])<=child_at, '受控任务原stdout起始身份不一致')
    if task=='probe':
        need((directory/'stdout.log').read_text(encoding='utf-8').splitlines()[1:]==['fixed probe started','fixed probe completed'], '固定probe原子任务日志不同')
    return {'passed':True,'task':task,'phase':phase,'run_id':run_id,'worker_pid':worker['pid'],'worker_start_ticks':worker['start_ticks'],
        'task_pid':identity['pid'],'task_start_ticks':identity['start_ticks'],'exit_code':0,'worker_exit_code':0,'source_stable':True,
        'automatic_restart':False,'terminal_sha256':sha(directory/'terminal.json'),'seconds':terminal['seconds'],'source_inputs':len(expected)}


def controlled_probe_evidence():
    path=WORK/'controlled-job-review/report.json'
    need(sha(path)==CONTROLLED_PROBE_REVIEW_SHA256, '原probe审查报告字节不同')
    review=read(path); probe=controlled_job_evidence('probe','probe-01')
    expected={p.relative_to(ROOT).as_posix():sha(p) for p in [WORK/'controlled_job.py',ROOT/'tools/process_control.py',ROOT/'tools/dev',ROOT/'tools/dev.ps1',ROOT/'tools/toolchain.py',ROOT/'conf/toolchain/windows-amd64.lock.json',WORK/'cutover.py']}
    need(review['passed'] is True and review['source_before']==review['source_after']==expected, '原probe报告完整源不同')
    for row in review['commands']:
        need(row['exit_code']==0 and row['command'][:2]==[review['start']['worker']['executable'],str(WORK/'controlled_job.py')] and row['command'][2:] in [['start','probe','--phase','probe-01'],['status','probe','--phase','probe-01']], 'probe审查不是原固定start/status')
        for stream in ['stdout','stderr']:need(sha(local(row[stream]))==row[stream+'_sha256'], 'probe审查原命令日志不同')
    outputs=[read(local(row['stdout'])) for row in review['commands']]
    need(outputs[0]==review['start'] and outputs[1]==review['live'] and outputs[-1]==review['completed'], 'probe报告未配对原start/live/completed日志')
    need(review['start']['worker']==review['live']['worker']==review['completed']['worker']==read(WORK/'controlled-jobs/probe-01/worker.json') and review['live']['live'] is True and review['completed']['live'] is False, 'probe不是同一实际worker从live到终态')
    need(review['completed']['terminal']==read(WORK/'controlled-jobs/probe-01/terminal.json') and review['completed']['terminal_sha256']==probe['terminal_sha256'], 'probe实际终态未配对')
    return probe


def publication_evidence(path,creator):
    need(PUBLICATION_CONTRACT_REVIEWED,'候选产物受控切主报告合同待审，不把候选构建等同当前主包')
    need(sha(WORK/'cutover.py')==CUTOVER_SHA256,'受审切主工具原字节变化')
    controlled_probe_evidence()
    controlled=controlled_job_evidence('cutover','cutover-01')
    value=read(path)
    expected_report_keys={'started_at_utc','commands','phase','passed','scope','other_ports_touched','rollback_attempted','rollback_restored','before','candidate_manifest_sha256','mongo_before','staged_artifact_files','after','served_candidate_exact_match','mongo_after','mongo_service_restarted','finished_at_utc'}
    need(set(value)==expected_report_keys,'成功切主报告字段不同或残留failure/rollback信息')
    need(path==WORK/'runtime-control/report.json' and value['passed'] is True and value['other_ports_touched']==[] and value['mongo_service_restarted'] is False,'受控切主未通过或范围扩大')
    need(value['phase']=='completed' and value['rollback_attempted'] is False and value['rollback_restored'] is False,'切主不是无回滚成功终态')
    need('error' not in value and not any(key.startswith('rollback_') and key not in {'rollback_attempted','rollback_restored'} for key in value),'失败/回滚不能追认为本次成功切主')
    started,finished=[datetime.fromisoformat(value[key].replace('Z','+00:00')) for key in ['started_at_utc','finished_at_utc']]
    task=read(WORK/'controlled-jobs/cutover-01/task.json'); terminal=read(WORK/'controlled-jobs/cutover-01/terminal.json')
    output=(WORK/'controlled-jobs/cutover-01/stdout.log').read_text(encoding='utf-8').splitlines()
    marker=json.loads(output[0]); child_born=datetime(1601,1,1,tzinfo=timezone.utc)+timedelta(microseconds=task['child_identity']['start_ticks']//10)
    need(datetime.fromisoformat(marker['task_started_at_utc'])<=child_born<=started<=finished<=datetime.fromisoformat(terminal['finished_at_utc']), '切主报告不在原受控子任务执行窗口')
    summary=json.loads(output[-1])
    need(summary=={'passed':True,'phase':'completed','rollback_restored':False}, '受控子任务原stdout不是本次切主成功终态')
    need(started.tzinfo is not None and finished.tzinfo is not None and started<=finished,'切主缺完整终态时钟')
    need(value['served_candidate_exact_match'] is True and value['candidate_manifest_sha256']==creator['artifact_manifest_sha256'],'切主报告不匹配候选')
    commands=logs(value)
    # 8个主命令外，最终脚本明确增加2个client status核验；按原顺序保留10条，不丢弃额外日志。
    expected={'mongo-before':('tools/environment/mongodb_dev.py','status'),'server-before':('tools/server/dev.py','status'),'client-before':('tools/client/dev.py','status'),
        'server-stop':('tools/server/dev.py','stop'),'client-stop':('tools/client/dev.py','stop'),'client-stopped-status':('tools/client/dev.py','status'),
        'server-start':('tools/server/dev.py','start'),'client-start':('tools/client/dev.py','start'),'client-after':('tools/client/dev.py','status'),'mongo-after':('tools/environment/mongodb_dev.py','status')}
    need(list(commands)==list(expected),'受控环境命令集合/顺序不同或混入恢复动作')
    parsed={}
    for name,(tool,action) in expected.items():
        need(commands[name]['command'][-3:]==['python3',tool,action],'受控命令内容不同：'+name)
        need(commands[name]['parse_json'] is (name!='client-stop'),'client-stop纯文本/其它JSON读取合同不同')
        if name!='client-stop':parsed[name]=json_output(local(commands[name]['stdout']))
    need(parsed['server-stop']['running'] is False and parsed['client-stopped-status']['running'] is False,'未核实原服务已停止')
    before,after=read(WORK/'runtime-before.json'),read(WORK/'runtime-after.json')
    need(before==value['before'] and after==value['after'] and set(before)==set(after)=={'18080','18100','18101','18102'},'直接health快照未与切主报告绑定')
    old_fixture=read(WORK/'baseline.json')['files'][FIXTURE]
    backend_before,backend_after=parsed['server-before'],parsed['server-start']
    static_before,static_after=parsed['client-before'],parsed['client-after']
    need(all(row['running'] is True for row in [backend_before,backend_after,static_before,static_after]),'前后主服务status未运行')
    for port,mode in [('18100','account'),('18101','game'),('18102','gm')]:
        need(before[port]['rule_version']==21 and before[port]['fixture_hash']==old_fixture,'切主前不是受保护rule21')
        need(after[port]['rule_version']==22 and after[port]['fixture_hash']==sha(ROOT/FIXTURE) and after[port]['status']=='ok' and after[port]['development_only'] is True and before[port]['instance']!=after[port]['instance'],'切主后主节点身份/fixture不符')
        need(before[port]['mode']==after[port]['mode']==mode and before[port]['instance']==backend_before['health'][mode]['instance'] and after[port]['instance']==backend_after['health'][mode]['instance'],'直接health未绑定原backend status身份')
    need(before['18080']['run_id']!=after['18080']['run_id'],'静态服务未切换受控新身份')
    main=ROOT/'client/build/web-mobile'
    for health,status in [(before['18080'],static_before),(after['18080'],static_after)]:
        need(health['webMobileBuild'] is True and Path(health['build_root']).resolve()==main.resolve() and health['port']==18080 and health['account_port']==18100 and health['game_port']==18101,'静态主包路径/代理端口不是本任务')
        need(health['pid']==status['health']['pid'] and health['run_id']==status['health']['run_id'],'静态直接health与status身份不同')
    state=read(ROOT/'.work/client-dev/state.json')
    need(Path(state['project_root']).resolve()==ROOT.resolve() and state['pid']==after['18080']['pid'] and state['run_id']==after['18080']['run_id'],'当前静态state不是切主后的同项目身份')
    build_root=local(creator['build_root']);files=read(local(creator['artifact_manifest']))
    expected_files={(ROOT/name).relative_to(build_root).as_posix():digest for name,digest in files.items()}
    actual={}
    for node in [main,*sorted(main.rglob('*'))]:
        need(not node.is_symlink() and not getattr(node,'is_junction',lambda:False)(),'当前主包含链接/junction')
        if node.is_file():actual[node.relative_to(main).as_posix()]=sha(node)
    need(expected_files==actual and len(actual)==value['staged_artifact_files'],'当前主包未与真实候选逐文件相等')
    mongo_before,mongo_after=parsed['mongo-before'],parsed['mongo-after']
    configured=read(ROOT/'conf/dev/server.json');expected_uri=configured.get('mongo_uri_by_platform',{}).get('windows',configured['mongo_uri'])
    for row in [mongo_before,mongo_after]:
        need(all(row[key] is True for key in ['running','endpoint_verified','listener_verified','ownership_verified']),'Mongo不满足运行/端点/监听/所有权核验')
        need(row['backend']=='windows_native' and type(row['port']) is int and row['port']==27119 and row['uri']==expected_uri,'不是配置选中的本任务Mongo实例')
        need(all(type(row[key]) is str and row[key] for key in MONGO_IDENTITY_FIELDS if key not in {'pid','port'}),'Mongo完整身份元数据缺失或类型错误')
        need(type(row['pid']) is int and row['pid']>0,'Mongo PID无效')
    mongo_identity={key:mongo_before[key] for key in MONGO_IDENTITY_FIELDS}
    need(mongo_identity=={key:mongo_after[key] for key in MONGO_IDENTITY_FIELDS}==value['mongo_before']==value['mongo_after'],'Mongo PID/安装/项目/数据目录等原身份变化')
    return {'passed':True,'scope':'受控环境/实际候选切主支持事实，不代替业务验收','candidate_manifest_sha256':creator['artifact_manifest_sha256'],
        'served_candidate_exact_match':True,'artifact_files':len(actual),'backend_rule_before':21,'backend_rule_after':22,'mongo_service_restarted':False,
        'happy_path_core_commands':8,'additional_status_checks':2,'total_commands':len(commands),'rollback_attempted':False,'rollback_restored':False,
        'mongo_identity_fields_verified':list(MONGO_IDENTITY_FIELDS),'mongo_identity_equal':True,'cutover_source_sha256':CUTOVER_SHA256,
        'runtime_before_sha256':sha(WORK/'runtime-before.json'),'runtime_after_sha256':sha(WORK/'runtime-after.json'),'controlled_runner':controlled}


class Archive:
    def __init__(self):
        self.jobs = {}

    def add(self, source, target, scope):
        source = local(source)
        target = Path(target)
        need(source.is_file() and source.suffix.lower() not in {'.exe','.dll','.lib','.wil','.wix','.wzl','.wzx','.pak','.rar','.zip','.7z','.ttf','.ttc','.wav','.mp3','.ogg','.bin'}, '不归档二进制包/字体/声音原包')
        need(not target.is_absolute() and '..' not in target.parts and not (set(target.parts) & FORBIDDEN), '归档目标越界/缓存')
        relative = source.relative_to(ROOT).as_posix()
        if any(job['source'] == relative for job in self.jobs.values()):
            return
        compressed = source.suffix in {'.jsonl', '.txt', '.log', '.json'} and source.stat().st_size >= 128 * 1024
        if source.suffix == '.jsonl':
            compressed = True
        if compressed:
            target = target.with_name(target.name + '.gz')
        need(target.as_posix() not in self.jobs, '归档目标重复')
        self.jobs[target.as_posix()] = {'source': relative, 'target': target.as_posix(), 'scope': scope,
            'encoding': 'gzip' if compressed else 'identity', 'raw_sha256': sha(source), 'raw_bytes': source.stat().st_size}

    def bundle(self, report, target, scope):
        report = local(report); value = read(report)
        self.add(report, Path(target) / report.name, scope)
        names = ['events.jsonl','command.json','mongo-status.json','wrapper.json', 'started.json', 'creator.log', 'stderr.txt', 'tool.stdout.txt', 'tool.stderr.txt',
                 'ownership.stdout.json', 'ownership.stderr.txt', 'stdout.log', 'stderr.log', 'wrapper-start.json', 'wrapper-result.json',
                  'visual-review.json','run.py','tool-at-start.cjs','handoff.json','progress.json',
                  'candidate-files.json','creator-build-candidate.json','served-build-before.json','served-build-after.json']
        for name in names:
            path = report.parent / name
            if path.is_file():
                self.add(path, Path(target) / name, scope)
        for row in value['commands'] if 'commands' in value else []:
            for path in support_command_logs(row,report):self.add(path,Path(target)/path.name,scope)
        if 'log_sha256' in value:
            path = report.parent / 'actual.log'
            need(path.is_file() and sha(path) == value['log_sha256'], '单命令原actual.log未配对')
            self.add(path, Path(target) / 'actual.log', scope)
        if 'actual_log_sha256' in value:
            path = report.parent / 'actual.log'
            need(path.is_file() and sha(path) == value['actual_log_sha256'], '原选择反例actual.log未配对')
            self.add(path, Path(target) / 'actual.log', scope)
        wrapped = report.parent / 'wrapper.json'
        if wrapped.is_file():
            for row in read(wrapped)['commands']:
                for key in ['stdout', 'stderr']:
                    self.add(local(row[key]), Path(target) / Path(row[key]).name, scope)
        if 'events_sha256' in value:
            need(sha(report.parent / 'events.jsonl') == value['events_sha256'], '历史Go raw不同')
        return value

    def apply(self):
        OUT.mkdir(parents=True, exist_ok=False)
        records = []
        for job in self.jobs.values():
            source, target = ROOT / job['source'], OUT / job['target']
            need(sha(source) == job['raw_sha256'], '归档前原字节漂移')
            target.parent.mkdir(parents=True, exist_ok=True)
            if job['encoding'] == 'gzip':
                with source.open('rb') as src, target.open('xb') as dst, gzip.GzipFile(fileobj=dst, filename='', mode='wb', mtime=0) as z:
                    shutil.copyfileobj(src, z)
                need(hashlib.sha256(gzip.decompress(target.read_bytes())).hexdigest() == job['raw_sha256'], 'gzip原字节不等')
            else:
                shutil.copyfile(source, target)
                need(sha(target) == job['raw_sha256'], '复制原字节不等')
            records.append({**job, 'file_sha256': sha(target), 'file_bytes': target.stat().st_size})
        return records


def write_new(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    with path.open('x',encoding='utf-8',newline='\n') as stream:stream.write(json.dumps(value,ensure_ascii=False,indent=2)+'\n')


def interrupted_go_history(archive):
    # 此次没有测试终态；只核原始观察和文件，不补造report/wrapper或推断业务失败。
    directory=WORK/'go/full-2'; path=directory/'interruption.json'; value=read(path)
    need(value['status']=='stopped_without_terminal_report' and value['accepted_as_pass'] is False and value['test_exit_code'] is None,
         'full-2中断历史不能作为终态通过/业务失败')
    need(value['specific_cause_proven'] is False and value['source_stable_since_start'] is True and value['latest_report_is_older_attempt'] is True,
         'full-2中断观察合同不同')
    need(not (directory/'report.json').exists() and not (directory/'wrapper.json').exists(), 'full-2已出现终态，须另行审阅原始状态')
    started=read(directory/'started.json')
    need(started['group']=='go' and started['freeze_started_before_commands'] is True and len(started['before'])==value['source_count']==197,
         'full-2原始启动输入不完整')
    expected_originals={'started.json','full-mongo-race.stdout.txt','full-mongo-race.stderr.txt'}
    need(set(value['preserved_original_files'])==expected_originals, 'full-2原文件白名单不同')
    files={path,*(directory/name for name in expected_originals)}
    for name,digest in value['preserved_original_files'].items():
        need(sha(directory/name)==digest, 'full-2原启动/空日志字节变化')
    need(all((directory/name).stat().st_size==0 for name in expected_originals if name.endswith('.txt')), 'full-2原空日志不再为空')
    commands=value['commands']
    need([row['name'] for row in commands]==['processes','server-owned','mongo-owned'] and all(row['exit_code']==0 for row in commands),
         'full-2缺原三项只读probe')
    for row in commands:
        for key in ['stdout','stderr']:
            raw=local(row[key])
            need(raw==directory/(row['name']+'.'+key+'.txt') and sha(raw)==row[key+'_sha256'], 'full-2 probe原字节不同')
            files.add(raw)
    processes=read(directory/'processes.stdout.txt')
    need(processes==value['live_main_processes'] and processes and all(row['Name']=='nbserver.exe' for row in processes),
         'full-2 probe并非只保留原main且无测试链')
    server,mongo=[read(directory/(name+'.stdout.txt')) for name in ['server-owned','mongo-owned']]
    need(server['running'] is True and all(row['status']=='ok' and row['rule_version']==21 for row in server['health'].values()),
         'full-2原main健康观察不一致')
    need(mongo['running'] is True and mongo['endpoint_verified'] is True and mongo['ownership_verified'] is True and mongo['pid']==value['owned_mongo_pid'],
         'full-2原Mongo所有权观察不一致')
    need(sha(WORK/'record_interruption.py')==value['script_sha256'], 'full-2中断记录工具原字节变化')
    for raw in sorted(files):archive.add(raw,Path('history/go/full-2')/raw.name,'interrupted_without_terminal_unknown_cause_not_test_pass_or_business_failure')
    return {'path':path.relative_to(ROOT).as_posix(),'sha256':sha(path),'status_at_that_time':value['status'],
        'passed_at_that_time':None,'exit_code_at_that_time':None,'accepted_as_pass':False,'specific_cause_proven':False,
        'scope':'原终态缺失且原因未知；三项只读probe及原启动/空日志保留，不计测试通过或业务失败'}


def curate(args,protocol_outputs,shots):
    archive,history=Archive(),[]
    for key,target in [('go_report','go'),('client_report','client'),('creator_report','creator'),('gui_report','gui')]:
        archive.bundle(getattr(args,key),'current/'+target,'current_final_selected_scope')
    archive.bundle(args.publication_report,'support/publication','controlled_build_publication_not_business_acceptance')
    archive.bundle(args.gui_static_report,'support/gui-static','actual_static_tool_contract_not_browser')
    for name in ['probe.cjs','run.py','report.json','wrapper.json','stdout.log','stderr.log','loaded-public.json']:
        archive.add(args.public_probe_report.parent/name,Path('support/public-loaded-snapshot')/name,'read_only_public_jsonasset_original_string_snapshot_matches_original_gui_hash_and_all_current_public_fields_not_new_ui_acceptance')
    archive.bundle(WORK/'controlled-job-review/report.json','support/controlled-runner/probe-review','fixed_two_second_probe_actual_start_live_terminal_not_business_or_publication')
    archive.add(WORK/'controlled-job-peer-review.json','support/controlled-runner/peer-review.json','read_only_runner_source_and_probe_metadata_review_not_cutover_execution')
    for phase in ['probe-01','cutover-01']:
        for name in ['launch.json','spawn.json','worker.json','approved.json','task.json','terminal.json','stdout.log','stderr.log']:
            archive.add(WORK/'controlled-jobs'/phase/name,Path('support/controlled-runner')/phase/name,'fixed_task_original_identity_source_and_exit_metadata_not_independent_business_acceptance')
    for name in ['runtime-before.json','runtime-after.json','cutover.py']:
        archive.add(WORK/name,Path('support/publication')/name,'controlled_environment_support_not_business_acceptance')
    archive.add(WORK/'cutover-review.json','support/publication/cutover-review.json','static_ast_review_not_actual_execution')
    archive.add(WORK/'cutover-before-review.py','history/publication/cutover-before-review.py','historical_unexecuted_script_before_stop_parser_and_rollback_review')
    for path in protocol_outputs:archive.add(path,Path('current/client/protocol')/path.name,'current_protocol_raw')
    for path in shots:archive.add(path,Path('current/gui')/path.name,'current_actual_gpu_screenshot_not_generated_art')
    cfg=read(WORK/'config/final-review.json')
    names=['baseline.json','candidate-review.json','editor-review.json','edit-plan.json','application.json','final-review.json','author_configuration.py','verify_configuration.py','run_checks.py','update_current_tests.py','current-test-adaptations.json']
    for row in cfg['commands']:names += [row['name']+suffix for suffix in ['.json','.stdout.txt','.stderr.txt']]
    for name in sorted(set(names)):archive.add(WORK/'config'/name,Path('current/config')/name,'configuration_six_cells_original_byteproof')
    archive.add(WORK/'config/shared-test-transition.json','support/config/shared-test-transition.json','exact_one_shared_test_three_itemversion_insertions_go3_proof_not_python130_reexecution')
    archive.add(WORK/'config/shared_test_transition.py',Path('sources')/WORK.relative_to(ROOT)/'config/shared_test_transition.py','shared_original_byte_transition_verifier_used_by_finalizer_and_index')
    archive.add(WORK/'finalizer-diagnostics/check-01.json','history/finalizer/check-01.json','actual_initial_tool_preflight_shared_test_hash_mismatch_not_product_failure')
    archive.add(WORK/'finalizer-diagnostics/check-02.json','history/finalizer/check-02.json','derived_structured_tool_terminal_observation_not_independent_raw_stdout_or_product_failure')
    for name in ['finalizer-before-shared-test-transition.py','verify-index-before-shared-test-transition.py']:
        archive.add(WORK/'history'/name,Path('history/finalizer')/name,'original_frozen_archiver_before_explicit_shared_test_transition')
    archive.add(WORK/'history/finalizer-before-public-snapshot.py','history/finalizer/finalizer-before-public-snapshot.py','original_archiver_with_rejected_python_stringification_assumption_not_gui_product_failure')
    for name in [BOOK,'config/schema/catalog.json','tools/resources/prepare_first_loop.py','tools/resources/prepare_summon_candidates.py']:
        archive.add(WORK/'config/before'/name,Path('before')/name,'before_exact_source_not_current')
    for attempt in ['candidate','candidate-2']:
        for suffix in ['.json','.stdout.txt','.stderr.txt']:archive.add(WORK/'config'/(attempt+suffix),Path('history/config')/(attempt+suffix),'historical_author_tool_failure_not_gameplay')
    for name in ['baseline.json','source-audit.json','same-server-client-audit.json','client-audit.json','server-audit.json','peer-review.json']:
        archive.add(WORK/name,Path('support/source-audits')/name,'read_only_audit_scope_as_recorded_not_runtime_certification')
    for name in ['appearance-consumer-review.json','appearance-consumer-review-v2.json','appearance-support-peer.json']:
        archive.add(WORK/name,Path('support/source-audits')/name,'support_next_art_source_research_not_stage22_or_target180_or_item220_certification')
    archive.add(WORK/'candidate-compat-review.json','history/go/full-1/candidate-compat-review.json','read_only_root_cause_legacy_candidate_decode_not_python_or_main_data_failure')
    archive.add(WORK/'history/go-full-1-selector.json','history/go/full-1/original-selector.json','original_failed_full_go_raw_sha_pointers_no_status_rewrite')
    history.append(interrupted_go_history(archive))
    for folder in ['source-samples','same-server-samples']:
        for path in sorted((WORK/folder).glob('*.png')):archive.add(path,Path('support/source-audits')/folder/path.name,'read_only_native_pixels_not_runtime_selection_or_shape_mapping')
    for name in ['audit_sources.py','audit_same_server_client.py','capture_baseline.py','collect_client_scope.py','stage_checks.py','build_candidate.py','finalize_evidence.py','verify_index.py','stage_verified.py','update_docs.py','source-allowlist.json','curate-plan.json','prepare_evidence_tools.py','record_candidate_compat_review.py','record_interruption.py','verify_appearance_support.py','controlled_job.py','controlled_job_probe_review.py']:
        archive.add(WORK/name,Path('sources')/WORK.relative_to(ROOT)/name,'source_at_archive_for_reproduction')
    archive.add(ROOT/'tools/client/verify_equipment_browser.cjs','sources/tools/client/verify_equipment_browser.cjs','actual_gui_tool_source_at_archive')
    archive.add(ROOT/'tools/client/test_equipment_browser.cjs','sources/tools/client/test_equipment_browser.cjs','actual_pb_header_long_and_saved_nodes_static_tool_tests_not_browser')
    selected={getattr(args,key).resolve() for key in ['go_report','client_report','creator_report','gui_report']}
    candidates=set(args.history_report)
    candidates|={WORK/'client/tool-static/report.json',WORK/'client/tool-static-2/report.json',WORK/'client/tool-static-3/report.json'}
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
    for name,default in [('go-report','go/full-3/report.json'),('client-report','client/full-1/report.json'),('creator-report','creator/full-1/report.json')]:parser.add_argument('--'+name,type=Path,default=WORK/default)
    parser.add_argument('--go-events',type=Path);parser.add_argument('--gui-report',type=Path,required=True)
    parser.add_argument('--publication-report',type=Path,required=True,help='Root最终受控候选包切主的实际报告')
    parser.add_argument('--public-probe-report',type=Path,required=True,help='与原GUI loaded摘要及当前public全字段配对的实际只读JsonAsset快照')
    parser.add_argument('--gui-static-report',type=Path,default=WORK/'client/tool-static-4/report.json')
    parser.add_argument('--history-report',type=Path,action='append',default=[]);parser.add_argument('--support-report',type=Path,action='append',default=[])
    mode=parser.add_mutually_exclusive_group();mode.add_argument('--check',action='store_true',help='默认只读门禁，不写正式docs/outputs');mode.add_argument('--apply',action='store_true',help='仅Root最终授权后使用，不覆盖旧正式证据')
    args=parser.parse_args()
    for key in ['go_report','client_report','creator_report','gui_report','publication_report','gui_static_report','public_probe_report']:setattr(args,key,local(getattr(args,key)))
    args.go_events=local(args.go_events) if args.go_events else args.go_report.parent/'events.jsonl'
    args.history_report=[local(p) for p in args.history_report];args.support_report=[local(p) for p in args.support_report]
    # 先查轻量GUI/快照元数据，失败即停；随后原Go/client/Creator/布局/发布全门禁仍逐项执行。
    creator_before=read(args.creator_report)
    public_snapshot=public_snapshot_evidence(args.public_probe_report,args.gui_report,args.creator_report,args.client_report,args.publication_report)
    gui,shots,observations=gui_evidence(args.gui_report,args.creator_report,creator_before,read(ROOT/FIXTURE),read(ROOT/PUBLIC),args.gui_static_report,public_snapshot)
    config,fixture,public=configuration_evidence()
    go,gc=go_evidence(args.go_report,args.go_events)
    client,cc,protocol_outputs=client_evidence(args.client_report)
    creator=creator_evidence(args.creator_report);need(creator==creator_before,'轻量GUI核对后Creator原报告变化');layout=layout_evidence(creator)
    publication=publication_evidence(args.publication_report,creator);protocol=protocol_delta()
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
            'client_external_engine_inputs':cc['engine_inputs'],'client_external_font_inputs':cc['font_inputs'],**{key:cc[key] for key in ['pb_messages','pb_long_fields','pb_boundary_cases','pb_protocol_ids','pb_error_codes']},
            'creator_seconds':creator['seconds'],'creator_exit_code':36,'gameplay_excel_changed_cells':6,'formulas_preserved':460,'untouched_zip_members':101,'architecture_workbook_preserved':True,**layout},
        'main_creator':{'passed':True,'seconds':creator['seconds'],'exit_code':36,'source_stable':True,'served_build_preserved':True,'artifact_file_count':creator['artifact_file_count'],
            'timing_scope':'creator_process_and_postbuild_hash_validation','wrapper_command_seconds':read(args.creator_report.parent/'wrapper.json')['commands'][0]['seconds']},
        'protocol_delta':protocol,'controlled_publication':publication,'public_loaded_snapshot':public_snapshot,
        'gui':{'passed':True,'checks':gui['checks'],'errors':gui['errors'],'warnings':gui['warnings'],'observed':observations,'seeded':False,'privateBusinessInvoked':False},
        'raw_go_counts':gc,'historical_attempts':history,'selectors':{key:{'path':getattr(args,key).relative_to(ROOT).as_posix(),'sha256':sha(getattr(args,key))} for key in ['go_report','go_events','client_report','creator_report','gui_report','publication_report','gui_static_report','public_probe_report']},
        'limits':['主218/193四项MC/SC来源值均为0；非零MC/SC及等级资格仅隔离测试，不当主服装备成长或自然供给',
            '乌木剑220仍未选入；Price3000不是本任务回收/商店价；原body/Shape/Looks跨引擎编号和PAK覆盖未认证',
            '还原MYF原外观才需其实际字段/客户端/补丁桥接；可另以可追溯目标1.80定义到同参考实际库消费映射登记美术差异，不把复刻MYF魔改Shape34当唯一前提；两条路线本轮均未完整认证',
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
    with (OUT/'README.md').open('x',encoding='utf-8',newline='\n') as stream:stream.write('# 基础装备属性与正常穿脱限定证据\n\ncurrent仅取终态通过且输入当前匹配的实证；support保留作者窄测/隔离资格的明确范围；history保留原失败及无终态中断，不追认通过。Go full-2原因未知、无终态，只保留三项只读probe与原启动/空日志，不计业务失败。10份保存布局及旧资源全保持。\n\n主仅木剑/布衣，四项MC/SC为原来源零值。非零两职业测试不是主服自然供给或完整成长。220/身体外观/正式回收与修理未认证；50/400仅开发GoldOnly，洗炼石与祝福油未开放。整体容量、真机、完整P0及全1.80继续待验。\n\n外观消费v1/v2及peer只作下一内容来源研究：MYF原外观还原与目标1.80可追溯差异美术是两条路线，不能把MYF魔改Shape34当唯一前置；可解码、同名/相邻帧或跨引擎公式仍不认证任一路线。\n')
    stage['evidence_sha256']={(OUT/r['target']).relative_to(ROOT).as_posix():r['file_sha256'] for r in records}
    for name in ['README.md','evidence-manifest.json','evidence-paths.json']:stage['evidence_sha256'][(OUT/name).relative_to(ROOT).as_posix()]=sha(OUT/name)
    stage['evidence_file_count']=len(stage['evidence_sha256'])
    current(go['source_sha256'],'归档后Go');need(snapshot('client')==client['before'],'归档后client漂移')
    write_new(FORMAL,stage);print(json.dumps({**summary,'archive_executed':True,'formal_report_written':True},ensure_ascii=False))


if __name__=='__main__':main()
