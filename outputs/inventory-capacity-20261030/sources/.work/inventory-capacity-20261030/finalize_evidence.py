"""rule21归档草案；默认只读，全部终态门禁通过且Root显式--apply才新建正式产物。"""
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
OUT = ROOT / 'outputs/inventory-capacity-20261030'
FORMAL = ROOT / 'docs/普通满包与背包触屏阶段验收.json'
BOOK = 'outputs/development-prep-20261026/玩法配置.xlsx'
ARCH = 'outputs/development-prep-20261026/架构配置.xlsx'
FIXTURE = 'config/development/first-loop.json'
PUBLIC = 'client/assets/resources/development/gameplay.json'
GAME = 'client/assets/scenes/Game.scene'
BASE_HEAD = '5b6be439c013d16c4ca0bf0372c294e47b29a141'
COMMANDS = {'models', 'full-ui-app', 'scene', 'strict', 'protocol-check', 'protocol-lint'}
FORBIDDEN = {'client-project', 'library', 'temp', 'build', 'node_modules', '__pycache__', '.git', 'inputs'}
ENGINE = Path('D:/Program Files/CocosCreator/3.8.8/resources/resources/3d/engine/cocos')
ENGINE_FILES = ['input/input.ts', '2d/event/pointer-event-dispatcher.ts', 'scene-graph/node-event-processor.ts',
                'scene-graph/node.ts', 'ui/button.ts', 'core/data/object.ts', 'scene-graph/component.ts',
                'audio/audio-source.ts', 'ui/toggle.ts']
FONTS = [Path('C:/Windows/Fonts/arial.ttf'), Path('C:/Windows/Fonts/msyh.ttc'), Path('C:/Windows/Fonts/simsun.ttc')]
NS = {'s': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}
# 测试名称与工具静态接口已审；实际Go/Creator/GUI终态仍须逐项通过，不以候选字段放行。
OFFLINE_WS_TEST = 'TestFullBagOfflineCombatAndChargingMongoWS'
GUI_CONTRACT_REVIEWED = True


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
    need(OFFLINE_WS_TEST is not None, '真实离线满包WS测试名称尚未审定，不放行归档')
    required = {'TestFullBagContinuousSourceRewardsAndHostingReceiptsMongo', 'TestAutoRecyclePolicyLockAndOnlineOfflineMongoWS', OFFLINE_WS_TEST}
    need(required <= {row['test'] for row in passed if row['package'] == 'minimir/internal/integration'}, '缺离线满包WS/在线WS/Store原事件PASS')
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
        'pb_boundary_cases': codec_result['int64_boundary_cases'], 'pb_protocol_ids': lint['messages']}, protocol_outputs


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
    return value


def configuration_evidence():
    config_dir = WORK / 'config'
    value, candidate, baseline = [read(config_dir / name) for name in ['final-review.json', 'candidate-review.json', 'baseline.json']]
    need(value['status'] == 'configuration_complete_and_frozen' and value['rule_version'] == 21, '配置未冻结到21')
    current(value['current_hashes'], '配置')
    sys.path.insert(0, str(ROOT / 'tools/config'))
    import export_configs as config
    old, new = config_dir / 'before' / BOOK, ROOT / BOOK
    need(sha(old) == candidate['source_before_sha256'] == baseline['files'][BOOK] and sha(new) == candidate['candidate_sha256'], '书前后不等受审候选')
    a, b = config.read_xlsx(old), config.read_xlsx(new)
    diff = [(s, r, c, a.get(s, {}).get(r, {}).get(c), b.get(s, {}).get(r, {}).get(c))
        for s in set(a) | set(b) for r in set(a.get(s, {})) | set(b.get(s, {}))
        for c in set(a.get(s, {}).get(r, {})) | set(b.get(s, {}).get(r, {}))
        if a.get(s, {}).get(r, {}).get(c) != b.get(s, {}).get(r, {}).get(c)]
    need(len(diff) == 1 and diff[0][:3] == ('开发首循环夹具', 4, 3), '唯一书不是只C4')
    before, after = json.loads(diff[0][3]), json.loads(diff[0][4])
    normalized = deepcopy(after); normalized['rule_version'] = 20
    need(before['rule_version'] == 20 and after['rule_version'] == 21 and normalized == before, '除版本外夹带profile变化')
    formulas = []
    with zipfile.ZipFile(old) as left, zipfile.ZipFile(new) as right:
        need(left.namelist() == right.namelist(), 'ZIP成员改变')
        changed = [name for name in left.namelist() if left.read(name) != right.read(name)]
        need(changed == candidate['changed_zip_members'] and len(changed) == 1 and len(left.namelist()) - 1 == 102, '102非编辑ZIP成员变化')
        for archive in [left, right]:
            formulas.append({(name, c.attrib['r']): ET.tostring(c, encoding='unicode') for name in archive.namelist()
                if name.startswith('xl/worksheets/') and name.endswith('.xml') for c in ET.fromstring(archive.read(name)).findall('.//s:c', NS)
                if c.find('s:f', NS) is not None})
    need(formulas[0] == formulas[1] and len(formulas[0]) == 460 and sha(ROOT / ARCH) == baseline['files'][ARCH], '公式或架构书变化')
    for row in value['commands']:
        command = read(config_dir / row['report'])
        need(command['exit_code'] == 0, '配置命令失败')
        for key in ['stdout', 'stderr']:
            need(sha(config_dir / command[key]) == command[key + '_sha256'], '配置原日志变化')
    log = (config_dir / 'python-full.stderr.txt').read_text(encoding='utf-8')
    match = re.search(r'^Ran (\d+) tests? in ', log, re.M)
    need(match and int(match[1]) == value['python_tests'] and re.search(r'^OK\s*$', log, re.M), '配置原Python计数不等')
    fixture, public = read(ROOT / FIXTURE), read(ROOT / PUBLIC)
    need(fixture['rule_version'] == public['rule_version'] == 21 and len(fixture['skills']) == 22 and all(row['id'] != 17 for row in fixture['skills']), '主版本/技能范围变化')
    need(fixture['casting_supply']['enabled'] is False and fixture['pet_runtime']['bootstrap_mode'] == 'legacy_untrained_pet', '主材料或宠训练暗启')
    need(public['equipment_recycling'] == {'mode': 'development_gold_only', 'outputs': []}, 'GoldOnly候选合同变化')
    pointer, sync = read(ROOT / 'config/releases/preview_current.json'), read(ROOT / 'conf/toolchain/config-binding-sync.json')
    need(pointer['version'] == value['preview'] == sync['config_version'] and sync['generated_only'] is True, '预览/类型绑定不是当前21')
    return value, fixture, public


def layout_evidence(build):
    need(git('rev-parse', 'HEAD').decode().strip() == BASE_HEAD, '阶段起点HEAD变化')
    names = {name.decode() for name in git('ls-tree', '-r', '--name-only', '-z', 'HEAD', 'client/assets').split(b'\0') if name and Path(name.decode()).suffix in {'.scene', '.prefab'}}
    need(len(names) == 10 and names == set(build['asset_hashes']), '10布局集合变化')
    changed = {name for name in names if git('show', 'HEAD:' + name) != (ROOT / name).read_bytes()}
    need(changed == {GAME}, '除Game之外保存布局改变')
    old, actual = json.loads(git('show', 'HEAD:' + GAME)), read(ROOT / GAME)
    need(len(old) == 1190 and len(actual) == 1191, 'Game对象数量不符受审微调')
    planned = deepcopy(old)
    planned[270]['_lpos']['y'] = 28
    planned[270]['_components'].append({'__id__': 1190})
    planned[271]['_contentSize']['height'] = 414
    planned[273]['_lpos']['y'] = 207
    planned[274]['_contentSize']['height'] = 414
    widget = deepcopy(old[255]); widget['node'] = {'__id__': 270}; widget['_bottom'] = 56
    planned.append(widget)
    need(actual == planned, 'Game不等明确4旧对象/1新Widget计划')
    source_diff = [p.decode() for p in git('diff', '--name-only', '-z', 'HEAD', '--', 'client/assets').split(b'\0') if p]
    need(not [p for p in source_diff if p not in {GAME, PUBLIC} and not (p.startswith('client/assets/scripts/') and p.endswith('.ts'))], '旧美术/地图/meta或其它保存资源改变')
    return {'saved_layouts_total': 10, 'saved_layouts_intentionally_changed': 1, 'saved_layouts_unchanged_against_head': 9,
            'layouts_preserved_during_build': 10, 'old_game_objects': 1190, 'new_game_objects': 1191, 'modified_old_object_indices': [270, 271, 273, 274]}


def environment_evidence():
    before, after = [read(WORK / name) for name in ['runtime-before.json', 'runtime-after.json']]
    control = read(WORK / 'runtime-control/report.json')
    need(control['passed'] is True and control['other_ports_touched'] == [] and
         {row['name'] for row in control['commands']} == {'stop', 'start'} and len(control['commands']) == 2, '环境切换不是受控stop/start')
    for row in control['commands']:
        need(row['exit_code'] == 0 and row['command'][-3:] == ['python3', 'tools/server/dev.py', row['name']], '环境命令超出本任务后端')
        for stream in ['stdout', 'stderr']:
            need(sha(local(row[stream])) == row[stream + '_sha256'], '环境切换原日志变化')
    need(before['label'] == 'before' and after['label'] == 'after' and after['loaded_runtime_matches_current_disk_fixture'] is True and
         after['current_disk_fixture_sha256'] == sha(ROOT / FIXTURE) and after['current_disk_public_sha256'] == sha(ROOT / PUBLIC), '切换后运行配置未配对')
    for port in ['18100', '18101', '18102']:
        old, new = before['ports'][port], after['ports'][port]
        need(old['rule_version'] == 20 and new['rule_version'] == 21 and old['instance'] != new['instance'] and
             new['status'] == 'ok' and new['development_only'] is True and new['fixture_hash'] == sha(ROOT / FIXTURE), '切换前后主三节点身份不符')
    need(before['ports']['18080'] == after['ports']['18080'] and after['ports']['18080']['webMobileBuild'] is True,
         '静态预览身份被替换或非实构预览')
    return {'scope': '受控环境支持事实，不作为业务/容量验收', 'commands_passed': 2,
        'backend_rule_before': 20, 'backend_rule_after': 21, 'after_loaded_runtime_matches_disk': True,
        'static_preview_identity_preserved': True,
        'selectors': {name: {'path': (WORK / name).relative_to(ROOT).as_posix(), 'sha256': sha(WORK / name)}
            for name in ['runtime-before.json', 'runtime-after.json', 'runtime-control/report.json']}}


def gui_evidence(path, creator_path, creator, fixture, public):
    need(path.is_relative_to(WORK), 'GUI不是本阶段显式报告')
    value = read(path)
    need(value['passed'] is True and value['errors'] == [] and value['seeded'] is False and value['privateBusinessInvoked'] is False, 'GUI未正常UI通过')
    need(value['fixtureSha256'] == sha(ROOT / FIXTURE) and value['publicSha256'] == sha(ROOT / PUBLIC), 'GUI不是当前运行配置')
    tool = ROOT / 'tools/client/verify_inventory_capacity_browser.cjs'
    need(value['toolSha256'] == value['toolSha256After'] == sha(tool), 'GUI工具中途变化')
    static = read(WORK / 'tool-static/report.json')
    need(static['status'] == 'qa_tool_prepared_not_run' and static['actual_gui_run'] is False and
         static['tool_sha256_before'] == static['tool_sha256_after'] == static['tool_sha256_current'] == sha(tool) and
         static['preflight_checks'] == 5 and set(static['report_core_fields']) <= value.keys(), 'GUI工具不等已审静态接口')
    need(len(static['commands']) == 2 and {row['name'] for row in static['commands']} == {'syntax', 'preflight'}, 'GUI工具静态检查缺组')
    for row in static['commands']:
        need(row['exit_code'] == 0 and sha(local(row['log'])) == row['log_sha256'], 'GUI工具静态日志不同')
    need(value['sourceBefore'] == value['sourceAfter'] and value['sourceBefore'], 'GUI输入前后变化')
    need(value['sourceBefore'] == {**creator['asset_hashes'], **creator['script_hashes']} and
         value['creatorReportSha256'] == sha(creator_path) == sha(ROOT / 'docs/客户端构建检查.json'), 'GUI不是所选当前实构')
    current(value['sourceBefore'], 'GUI')
    need(value['checks'] and isinstance(value['warnings'], list), 'GUI缺检查或警告列表')
    need(GUI_CONTRACT_REVIEWED, 'GUI实际observations/健康身份/触点契约尚未冻结；当前草案拒正式通过')
    need(value['touchInput'] is True and value['routeOverride'] is False and value['bodySeeded'] is False and
         value['offlineChargingVerifiedByThisGui'] is False and value['physicalMobileVerified'] is False, 'GUI范围或实际输入标记不符')
    need(all(row['type'] == 'warning' for row in value['warnings']), 'GUI有console error不能降格成警告')
    need(len(value['health']) == len(value['healthAfter']) == 3 and {row['port'] for row in value['health']} == {18100, 18101, 18102}, '缺主三节点身份')
    before_health = {row['port']: row for row in value['health']}
    for row in value['healthAfter']:
        old = before_health[row['port']]
        need(row['status'] == old['status'] == 'ok' and row['instance'] == old['instance'] and
             row['rule_version'] == old['rule_version'] == 21 and row['fixture_hash'] == old['fixture_hash'] == sha(ROOT / FIXTURE) and
             old['development_only'] is True, 'GUI主服务身份/fixture变化')
    observed = value['observations']
    initial = observed['initial']
    need(initial['job'] == 2 and initial['level'] == 1 and initial['experience'] == '0' and initial['hosting']['remain'] == '0' and
         initial['autoRecycleEnabled'] is False, 'GUI不是正常新道士/零托管资格')
    role = initial['roleKey']
    for name in ['firstBagFull', 'afterFullContinued', 'autoOn', 'beforeReentry', 'reentry', 'reentryContinued']:
        state = observed[name]
        need(state['roleKey'] == role and state['rule'] == '21' and state['auto'] is True and state['hosting']['auto'] is True and
             len(state['items']) == 32 and int(state['hp']) > 0, 'GUI非同角色存活满包Auto：' + name)
    need(any(row['id'] == 2 for row in observed['learned']['skills']) and observed['learned']['level'] >= 7, '未正常学原职业书')
    receipts, recycles = value['receipts'], value['recycleResults']
    need(receipts and recycles, '缺真实奖励和弃物协议')
    for row in receipts:
        need(row['role'] == role and row['rule'] == '21' and row['curve'] == fixture['progression']['curve_version'] and
             int(row['source']) > 0 and int(row['raw']) == int(row['accepted']) + int(row['rejected']) and
             0 <= int(row['pooled']) <= int(row['accepted']), '40019原奖励/入账/拒收不守恒')
    for row in recycles:
        need(row['role'] == role and all(int(row[key]) >= 0 for key in ['revision', 'source', 'policyRevision', 'count', 'gold', 'boundGold', 'discarded']), '40018角色/数值不符')
    first = observed['firstDiscard']
    need(first in recycles and first['binding'] == initial['binding'] and first['role'] == role and first['trigger'] == 1 and
         first['reason'] == 'bag_full' and int(first['source']) > 0 and int(first['revision']) > int(initial['revision']) and
         first['op'] == '' and int(first['discarded']) > 0 and int(first['count']) == 0, '首满包并非真实原来源弃新增物')
    proof = observed['afterFullProof']
    later = proof['laterDiscards']
    need(len(later) >= 3 and len({int(row['source']) for row in later}) >= 3 and
         all(row in recycles and row['binding'] == initial['binding'] and row['role'] == role and row['trigger'] == 1 and
             row['reason'] == 'bag_full' and int(row['source']) > int(first['source']) and int(row['revision']) > int(first['revision']) and
             row['op'] == '' and int(row['discarded']) > 0 for row in later), '首满包后没有三个不同原Source的合法弃物')
    start, end = proof['receiptsBefore'], proof['receiptsAfter']
    need(type(start) is int and type(end) is int and 0 <= start < end <= len(receipts) and
         sum(int(row['accepted']) for row in receipts[:start]) == int(proof['acceptedBefore']) and
         sum(int(row['accepted']) for row in receipts[:end]) == int(proof['acceptedAfter']) > int(proof['acceptedBefore']), '满包后真实EXP接收未推进')
    for name, size in [('initial', (720, 1280)), ('tall', (720, 1600)), ('wide', (960, 1280)), ('restored', (720, 1280))]:
        shape = observed['geometry'][name]
        need((shape['window']['width'], shape['window']['height']) == size and shape['panel'] == {'width': 660, 'height': 820} and
             shape['view']['width'] == 600 and shape['view']['height'] == 414 and shape['rowHeight'] == 72 and
             abs(shape['view']['bottom'] - shape['itemLock']['top'] - 12) < .1, '实际GPU几何不等保存微调：' + name)
        view = shape['view']
        for key in ['itemLock', 'equip', 'recycle']:
            button = shape[key]
            need(min(view['right'], button['right']) <= max(view['left'], button['left']) or
                 min(view['top'], button['top']) <= max(view['bottom'], button['bottom']), '实际列表仍与固定按钮相交')
    for key in ['lastRowSelection', 'pageSelection']:
        selection = observed[key]
        event = selection['sameEvent']
        need(selection['touches'] and event['id'] == event['selectedId'] == selection['item']['id'] and
             0 <= event['time'] - event['touchBefore']['time'] < 1000 and abs(event['offset'] - event['touchBefore']['offset']) < .1 and
             abs(selection['offsetAfter'] - event['offset']) < 1, '缺同次真实触屏选择或滚动偏移改变')
    need(observed['pageSelection']['sameEvent']['label'] == '合并书页' and observed['pageSelection']['sameEvent']['interactable'] is True, '页上下文未当次更新')
    probe, cancelled = observed['bookProbe'], observed['mergeQuoteCancelled']
    need(probe['quote']['kind'] == 3 and len(probe['quote']['materials']) == 2 and cancelled['quote']['kind'] == 4 and
         cancelled['confirmationText']['title'] == '合并确认' and len(cancelled['after']['items']) == 32 and
         not [row for row in value['operations'] if row['id'] == 40013 and row['preview'] == cancelled['quote']['preview']], '合并报价/正常取消并非真实UI路径')
    need(observed['autoOff']['auto'] is False and observed['autoOff']['hosting']['auto'] is False, '没有实际关闭Auto')
    before, reentry = observed['beforeReentry'], observed['reentry']
    need(before['binding'] != reentry['binding'] and all(before[key] == reentry[key] for key in ['roleKey', 'life', 'scene', 'map']), '健康票没有换binding或重建现场')
    inventory = observed['reentryInitialInventory']
    need(inventory in value['inventoryBaselines'] and inventory['binding'] == reentry['binding'], '缺新票首Inventory水位')
    watermark = int(inventory['revision'])
    new_events = [row for row in [*receipts, *recycles] if row['binding'] == reentry['binding']]
    need(new_events and all(int(row['revision']) > watermark for row in new_events), '新票补播首Inventory水位内旧通知')
    need(any(row['binding'] == reentry['binding'] and int(row['revision']) > watermark and int(row['accepted']) > 0 for row in receipts) and
         any(row['binding'] == reentry['binding'] and int(row['revision']) > watermark and row['reason'] == 'bag_full' and int(row['discarded']) > 0 for row in recycles), '健康票Auto没有继续真正新奖励/弃物')
    def total(state):
        return sum(fixture['progression']['exp_per_level'][:state['level'] - 1]) + int(state['experience'])
    after = observed['reentryContinued']
    need(before['life'] == after['life'] and int(observed['reentryProgress']['before']) == total(before) and
         int(observed['reentryProgress']['after']) == total(after) >= total(before), '健康票成长倒退或新Life')
    need(observed['cleanup']['auto'] is False and observed['cleanup']['hosting']['auto'] is False, '结束未通过正常UI关Auto')
    shot_names = ['01-equipped-normal-auto', '02-normal-book-learn', '03-first-bag-full', '04-full-bag-continued',
                  '05-last-row-details-return', '06-tall-inventory', '06-wide-inventory', '07-book-quote', '08-immediate-merge-quote', '09-healthy-auto-on']
    need(static['screenshots'] == [name + '.png' for name in shot_names], 'GUI截图接口不等静态冻结清单')
    shots = [path.parent / (name + '.png') for name in shot_names]
    need(all(p.is_file() and p.read_bytes().startswith(b'\x89PNG\r\n\x1a\n') for p in shots), '缺正常流程原截图')
    wrapped = read(path.parent / 'wrapper-result.json')
    need(wrapped['exit_code'] == 0 and wrapped['inputs_stable'] is True and wrapped['inputs'] == wrapped['inputs_after'] and
         wrapped['browser_report_sha256'] == sha(path) and wrapped['product_source_modified'] is False and
         wrapped['service_control'] is False and wrapped['database_seeding'] is False, 'GUI包装终态或范围不符')
    current(wrapped['inputs'], 'GUI包装')
    for stream in ['stdout', 'stderr']:
        need(sha(path.parent / (stream + '.log')) == wrapped[stream + '_sha256'], 'GUI包装原输出变化')
    need(sha(path.parent / 'tool-at-start.cjs') == value['toolSha256'], '启动时工具副本不配对')
    visual, handoff = [read(path.parent / name) for name in ['visual-review.json', 'handoff.json']]
    need(visual['status'] == 'passed_with_scope' and visual['browser_report_sha256'] == sha(path) and
         visual['wrapper_result_sha256'] == sha(path.parent / 'wrapper-result.json') and
         visual['raw_report_unchanged'] is True and visual['product_or_tool_source_modified'] is False and
         {row['path']: row['sha256'] for row in visual['images']} == {p.name: sha(p) for p in shots}, '人工看图记录不等这10张原PNG')
    need(handoff['status'] == 'passed' and handoff['exec_terminal_exit_code'] == 0 and handoff['report_sha256'] == sha(path) and
         handoff['visual_review_sha256'] == sha(path.parent / 'visual-review.json') and
         handoff['wrapper_result_sha256'] == sha(path.parent / 'wrapper-result.json'), 'GUI终态handoff不配对')
    return value, shots, {'bag_full_protocol_events': len([row for row in recycles if row['reason'] == 'bag_full']),
        'experience_receipts': len(receipts), 'healthy_reentry': True, 'same_scene_and_life': True,
        'reentry_inventory_revision': inventory['revision'], 'screenshots': len(shots), 'gui_offline_charging_verified': False}


class Archive:
    def __init__(self):
        self.jobs = {}

    def add(self, source, target, scope):
        source = local(source)
        target = Path(target)
        need(source.is_file() and source.suffix.lower() not in {'.exe', '.dll', '.lib', '.wil', '.wix', '.ttf', '.ttc', '.wav'}, '不归档二进制包/字体/声音原包')
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
        names = ['events.jsonl', 'wrapper.json', 'started.json', 'creator.log', 'stderr.txt', 'tool.stdout.txt', 'tool.stderr.txt',
                 'ownership.stdout.json', 'ownership.stderr.txt', 'stdout.log', 'stderr.log', 'wrapper-start.json', 'wrapper-result.json',
                 'visual-review.json', 'run.py', 'tool-at-start.cjs', 'handoff.json', 'progress.json']
        for name in names:
            path = report.parent / name
            if path.is_file():
                self.add(path, Path(target) / name, scope)
        for row in value['commands'] if 'commands' in value else []:
            if {'stdout', 'stderr'} <= row.keys():
                for key in ['stdout', 'stderr']:
                    self.add(local(row[key]), Path(target) / Path(row[key]).name, scope)
            else:
                need('log' in row and 'log_sha256' in row, '未知支持报告日志接口')
                path = local(row['log'])
                need(sha(path) == row['log_sha256'], '支持报告原log不同')
                self.add(path, Path(target) / path.name, scope)
        if 'log_sha256' in value:
            path = report.parent / 'actual.log'
            need(path.is_file() and sha(path) == value['log_sha256'], '单命令原actual.log未配对')
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


def curate(args, client, protocol_outputs, shots):
    archive, history = Archive(), []
    for key, target in [('go_report', 'go'), ('client_report', 'client'), ('creator_report', 'creator'), ('gui_report', 'gui')]:
        path = getattr(args, key)
        archive.bundle(path, 'current/' + target, 'current')
        wrapped = path.parent / 'wrapper.json'
        if wrapped.is_file():
            for row in read(wrapped)['commands']:
                for stream in ['stdout', 'stderr']:
                    archive.add(local(row[stream]), Path('current') / target / Path(row[stream]).name, 'current_raw_wrapper_output')
    for path in protocol_outputs:
        archive.add(path, Path('current/client/protocol') / path.name, 'current_protocol_artifact')
    for path in shots:
        archive.add(path, Path('current/gui') / path.name, 'current_gui_screenshot')
    config_names = ['baseline.json', 'candidate-review.json', 'editor-review.json', 'edit-plan.json', 'application.json', 'final-review.json', 'author_configuration.py', 'verify_configuration.py', 'run_checks.py']
    for row in read(WORK / 'config/final-review.json')['commands']:
        config_names += [row['name'] + suffix for suffix in ['.json', '.stdout.txt', '.stderr.txt']]
    for name in config_names:
        archive.add(WORK / 'config' / name, Path('current/config') / name, 'configuration_and_single_cell_beforeproof')
    archive.add(WORK / 'config/before' / BOOK, 'before/玩法配置.xlsx', 'before_unique_workbook')
    for name in ['client/before.json', 'client/layout-diff.json', 'server-peer/review.json']:
        archive.add(WORK / name, Path('current') / name, 'peer_or_saved_layout_scope_as_recorded')
    for name in ['server/go-frozen.json', 'server-peer/refresh_review.py']:
        archive.add(WORK / name, Path('support') / name, 'narrow_freeze_and_read_only_review_source_as_recorded')
    for name in ['mongo-store-2', 'mongo-auto-ws-1', 'mongo-offline-full-ws-2']:
        archive.bundle(WORK / 'server' / name / 'report.json', Path('support/server') / name,
                       'owner_narrow_not_substituted_for_final_full_go')
    archive.bundle(WORK / 'client/production-freeze.json', 'support/client/production-freeze',
                   'narrow_production_freeze_before_final_full_client_scope_as_recorded')
    archive.bundle(WORK / 'client/narrow/report.json', 'support/client/narrow',
                   'owner_narrow_not_substituted_for_final_full_client')
    archive.bundle(WORK / 'tool-static/report.json', 'support/tool-static', 'actual_syntax_and_pure_tool_contract_not_gui')
    archive.add(WORK / 'tool-static/preflight.cjs', 'support/tool-static/preflight.cjs', 'pure_tool_contract_source')
    archive.bundle(WORK / 'runtime-control/report.json', 'support/runtime-control', 'controlled_environment_only_not_business_acceptance')
    for name in ['runtime-before.json', 'runtime-after.json', 'cutover.py', 'record_runtime.py']:
        archive.add(WORK / name, Path('support/environment') / name, 'controlled_environment_only_not_business_acceptance')
    for path in [Path(__file__), WORK / 'stage_checks.py', WORK / 'verify_index.py', WORK / 'update_docs.py', WORK / 'curate-plan.json', WORK / 'partial_preflight.py',
                 ROOT / 'tools/client/verify_inventory_capacity_browser.cjs']:
        archive.add(path, Path('sources') / path.relative_to(ROOT), 'source_at_archive')
    # 格式修复失败与Store失败都是实际历史；缺任一原日志时拒绝遗漏式归档。
    apply_dir = WORK / 'server/apply'
    for name in ['report.json', 'patch-format-proof.json', 'original-d932.patch', 'git-compatible.patch'] + [str(i) + suffix for i in range(4) for suffix in ['.stdout.txt', '.stderr.txt']]:
        archive.add(apply_dir / name, Path('history/server/apply') / name, 'historical_patch_format_failure_not_gameplay')
    legacy_peer = WORK / 'history/server-peer/before-offline-ws/review.json'
    archive.add(legacy_peer, 'history/server-peer/before-offline-ws/review.json', 'historical_static_review_not_reproduced_red_green')
    adapted = WORK / 'server/full-1-test-adaptations'
    for name in ['before-sha256.json', 'go-frozen-before.json', 'residual-search-command.json', 'residual-search.stderr.txt', 'residual-search.txt']:
        archive.add(adapted / name, Path('history/server/full-1-test-adaptations') / name,
                    'historical_test_contract_adaptation_not_production_change')
    for name in ['server/internal/integration/level_growth_test.go', 'server/internal/integration/summon_pet_test.go',
                 'server/internal/integration/ownership_test.go']:
        archive.add(adapted / 'before' / name, Path('history/server/full-1-test-adaptations/before') / name,
                    'historical_original_test_bytes')
    red_path = WORK / 'client/selection-and-layout-original-red/result.json'
    red = archive.bundle(red_path, 'history/client/selection-and-layout-original-red',
                         'baseline5b_original_source_replay_not_full1_four_layout_hash_failures')
    need(red['exit_code'] == 1, '真实原源码反例状态不可追认通过')
    old_sources = {}
    for name, field in [('client/assets/scripts/app/AppRoot.ts', 'app_sha256'), (GAME, 'scene_sha256')]:
        original = WORK / 'client/original' / Path(name).name
        need(sha(original) == red[field] and original.read_bytes() == git('show', BASE_HEAD + ':' + name), '原源码反例没有绑定5b6基线：' + name)
        old_sources[name] = red[field]
        archive.add(original, Path('history/client/selection-and-layout-original-red/source') / original.name,
                    'baseline5b_original_source_bytes')
    history.append({'path': red_path.relative_to(ROOT).as_posix(), 'sha256': sha(red_path), 'exit_code_at_that_time': 1,
        'baseline_commit': BASE_HEAD, 'source_sha256': old_sources,
        'scope': 'saved_original_AppRoot_and_Game_replay_not_claimed_live_before_run_not_full1_four_layout_hash_failures'})
    candidates = set(args.history_report)
    candidates.add(WORK / 'server/mongo-store-1/report.json')
    candidates.add(WORK / 'server/mongo-offline-full-ws-1/report.json')
    for group, selected in [('go', args.go_report), ('client', args.client_report), ('creator', args.creator_report)]:
        candidates |= {p.resolve() for p in (WORK / group).glob('full-*/report.json') if p.resolve() != selected}
    for path in sorted(candidates):
        need(path.is_relative_to(WORK), '历史超出本阶段')
        report = archive.bundle(path, Path('history') / path.parent.relative_to(WORK), 'historical_not_current')
        history.append({'path': path.relative_to(ROOT).as_posix(), 'sha256': sha(path), 'passed_at_that_time': report.get('passed'),
            'exit_code_at_that_time': report.get('exit_code'), 'scope': 'historical_not_current_status_unchanged',
            **({'failure_scope': 'pre_business_test_account_length', 'production_defect_claimed': False,
                'detail': '原测试注册账号21字节违反既有6至20合同，10009被1001拒绝，未进入满包业务；原失败不改写'}
               if path == WORK / 'server/mongo-offline-full-ws-1/report.json' else {})})
        for image in sorted(path.parent.glob('*.png')):
            archive.add(image, Path('history') / path.parent.relative_to(WORK) / image.name, 'historical_partial_or_failure_screenshot')
    for path in args.support_report:
        need(path.is_relative_to(WORK), '支持报告超出本阶段')
        archive.bundle(path, Path('support') / path.parent.relative_to(WORK), 'author_narrow_scope_as_recorded_not_full_current')
    return archive, history


def write_new(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('x', encoding='utf-8', newline='\n') as stream:
        stream.write(json.dumps(value, ensure_ascii=False, indent=2) + '\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go-report', type=Path, default=WORK / 'go/full-1/report.json')
    parser.add_argument('--go-events', type=Path)
    parser.add_argument('--client-report', type=Path, default=WORK / 'client/full-1/report.json')
    parser.add_argument('--creator-report', type=Path, default=WORK / 'creator/full-1/report.json')
    parser.add_argument('--gui-report', type=Path, required=True)
    parser.add_argument('--history-report', type=Path, action='append', default=[])
    parser.add_argument('--support-report', type=Path, action='append', default=[])
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--check', action='store_true', help='默认：只读完整门禁，不写docs/outputs')
    mode.add_argument('--apply', action='store_true', help='仅Root最终明确授权执行；不可覆盖旧正式产物')
    args = parser.parse_args()
    for key in ['go_report', 'client_report', 'creator_report', 'gui_report']:
        setattr(args, key, local(getattr(args, key)))
    args.go_events = local(args.go_events) if args.go_events else args.go_report.parent / 'events.jsonl'
    args.history_report = [local(path) for path in args.history_report]
    args.support_report = [local(path) for path in args.support_report]
    config, fixture, public = configuration_evidence()
    go, gc = go_evidence(args.go_report, args.go_events)
    client, cc, protocol_outputs = client_evidence(args.client_report)
    creator = creator_evidence(args.creator_report)
    layout = layout_evidence(creator)
    environment = environment_evidence()
    gui, shots, observations = gui_evidence(args.gui_report, args.creator_report, creator, fixture, public)
    archive, history = curate(args, client, protocol_outputs, shots)
    validation = read(ROOT / 'config/reports/配置校验报告.json')
    need(validation['structure_valid'] is True and validation['runtime_ready'] is False and validation['release_blockers'], '正式发布门禁改变')
    peer = read(WORK / 'server-peer/review.json')
    current(peer['source_sha256'], '最终peer')
    need(not any(f['id'] == 'offline_full_bag_ws_coverage' for f in peer['findings']), '最终peer仍未核离线WS新证据')
    stage = {'schema_version': 1, 'started_date': '2026-10-30',
        'date': datetime.now(timezone.utc).astimezone(timezone(timedelta(hours=8))).date().isoformat(),
        'recorded_at_utc': datetime.now(timezone.utc).isoformat(),
        'stage': '普通满包与背包触屏', 'status': 'limited_passed',
        'scope': '普通满包仍进行真实战斗与原永久结算，仅按策略弃新增物；托管原扣时与终态、保存背包视口及触屏选择限定验证',
        'runtime': {'rule_version': 21, 'preview': config['preview'], 'fixture_sha256': sha(ROOT / FIXTURE), 'public_sha256': sha(ROOT / PUBLIC),
            'skills': 22, 'summon17_enabled': False, 'casting_supply_enabled': False, 'bootstrap_mode': 'legacy_untrained_pet',
            'legacy_pet_trains': False, 'table_count': validation['table_count'], 'record_count': validation['record_count'],
            'release_blockers': len(validation['release_blockers']), 'runtime_ready': False, 'production_ready': False},
        'verification': {'go_full_mongo_ws_race_passed_including_subtests': gc['passed'], 'go_inputs': gc['inputs'], 'go_failed': 0, 'go_skipped': 0,
            'go_source_stable': True, 'configuration_python_passed': config['python_tests'], 'javascript_ui_app_passed': cc['app'],
            'javascript_scene_passed': cc['scene'], 'javascript_total_passed': cc['total'], 'client_inputs': cc['inputs'],
            'client_external_engine_inputs': cc['engine_inputs'], 'client_external_font_inputs': cc['font_inputs'],
            **{name: cc[name] for name in ['pb_messages', 'pb_long_fields', 'pb_boundary_cases', 'pb_protocol_ids']},
            'creator_seconds': creator['seconds'], 'creator_exit_code': 36, 'gameplay_excel_changed_cells': 1, 'formulas_preserved': 460,
            'untouched_zip_members': 102, 'architecture_workbook_preserved': True, **layout},
        'main_creator': {'passed': True, 'seconds': creator['seconds'], 'exit_code': 36, 'source_stable': True},
        'runtime_control_support': environment,
        'gui': {'passed': True, 'checks': gui['checks'], 'errors': gui['errors'], 'warnings': gui['warnings'], 'observed': observations,
            'seeded': False, 'privateBusinessInvoked': False}, 'raw_go_counts': gc, 'historical_attempts': history,
        'selectors': {key: {'path': getattr(args, key).relative_to(ROOT).as_posix(), 'sha256': sha(getattr(args, key))}
            for key in ['go_report', 'go_events', 'client_report', 'creator_report', 'gui_report']},
        'limits': ['50/400为development_gold_only候选、outputs=[]；正式洗炼石与祝福油和损坏回收未验收',
            '主仍22技能、legacy免费未训宠、护符关闭、17仅独立候选，未新增赠书或购符规格',
            '原静态drain顺序finding遗漏致死立即snapshot，未运行反例，不算生产实证红绿；四步收束只是防护增强',
            '隔离低HP/已满背包/短托管余额不代表自然效率或经济平衡；手机、整体容量、完整P0及全1.80未验收'],
        'capacity_verified': False, 'physical_mobile_verified': False, 'production_ready': False}
    current(go['source_sha256'], '归档前Go')
    need(snapshot('client') == client['before'], '归档前client变化')
    for job in archive.jobs.values():
        need(sha(ROOT / job['source']) == job['raw_sha256'], '归档前证据漂移')
    summary = {'preflight': 'passed', 'archive_executed': False, 'formal_report_written': False, 'go': gc['passed'],
        'javascript': cc['total'], 'python': config['python_tests'], 'gui_checks': len(gui['checks']), 'archive_files_planned': len(archive.jobs)}
    if not args.apply:
        print(json.dumps(summary, ensure_ascii=False)); return
    need(not OUT.exists() and not FORMAL.exists(), '正式目标已存在，禁止覆盖')
    records = archive.apply()
    write_new(OUT / 'evidence-manifest.json', {'schema_version': 1, 'scope': '显式白名单；原字节或无损gzip，不复制工程/输入树/二进制/凭证', 'files': records})
    write_new(OUT / 'evidence-paths.json', {r['source']: {'path': (OUT / r['target']).relative_to(ROOT).as_posix(), 'encoding': r['encoding'],
        'raw_sha256': r['raw_sha256'], 'file_sha256': r['file_sha256']} for r in records})
    with (OUT / 'README.md').open('x', encoding='utf-8', newline='\n') as stream:
        stream.write('# 普通满包与背包触屏限定证据\n\ncurrent只对应通过且当前输入吻合的终态；history保留原失败/原状态，不追认通过。Game.scene四处旧对象和一Widget有意调整，其余9布局相对阶段前保持，构建期间10布局保持。\n\n50/400仅开发GoldOnly候选，outputs=[]；正式洗炼石与祝福油、损坏回收、手机、整体容量和完整P0未验收。原静态finding不是实证生产红绿。\n')
    stage['evidence_sha256'] = {(OUT / r['target']).relative_to(ROOT).as_posix(): r['file_sha256'] for r in records}
    for name in ['README.md', 'evidence-manifest.json', 'evidence-paths.json']:
        stage['evidence_sha256'][(OUT / name).relative_to(ROOT).as_posix()] = sha(OUT / name)
    stage['evidence_file_count'] = len(stage['evidence_sha256'])
    current(go['source_sha256'], '归档后Go'); need(snapshot('client') == client['before'], '归档后client变化')
    write_new(FORMAL, stage)
    print(json.dumps({**summary, 'archive_executed': True, 'formal_report_written': True}, ensure_ascii=False))


if __name__ == '__main__':
    main()
