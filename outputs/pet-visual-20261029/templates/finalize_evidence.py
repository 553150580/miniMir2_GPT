"""只在当前全部证据通过后按显式白名单归档；本脚本须经Root审阅后运行。"""
from __future__ import annotations

import argparse
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import runpy
import shutil
import struct
import sys

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
OUT = ROOT / 'outputs/pet-visual-20261029'
FORMAL = ROOT / 'docs/宠物出生表现与隔离客户端阶段验收.json'
FORBIDDEN_PARTS = {'client-project', 'library', 'temp', 'build', 'node_modules', '__pycache__', '.git'}
EDITOR_CACHES = ['settings/v2/packages/information.json']
CREATOR_INPUTS = ['assets', 'settings', 'profiles', '.creator', 'node_modules', 'package.json', 'package-lock.json', 'tsconfig.json']
ENGINE_ROOT = Path('D:/Program Files/CocosCreator/3.8.8/resources/resources/3d/engine/cocos').resolve()
ENGINE_FILES = ['input/input.ts', '2d/event/pointer-event-dispatcher.ts', 'scene-graph/node-event-processor.ts', 'scene-graph/node.ts', 'ui/button.ts', 'core/data/object.ts', 'scene-graph/component.ts', 'audio/audio-source.ts']


def need(condition, message):
    if not condition:
        raise ValueError(message)


def sha(path):
    digest = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            digest.update(block)
    return digest.hexdigest()


def read(path):
    return json.loads(Path(path).read_text(encoding='utf-8'))


def write_new(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('x', encoding='utf-8', newline='\n') as stream:
        stream.write(json.dumps(value, ensure_ascii=False, indent=2) + '\n')


def project_path(value, base=ROOT):
    path = Path(value)
    if not path.is_absolute():
        path = base / path
    path = path.resolve()
    need(path.is_relative_to(base.resolve()), '路径越过受控根目录：' + str(path))
    return path


def current(expected, base=ROOT, label='输入'):
    need(isinstance(expected, dict) and expected, label + '缺少完整摘要')
    for relative, digest in expected.items():
        path = project_path(relative, base)
        need(path.is_file() and not path.is_symlink() and sha(path) == digest, label + '已漂移：' + relative)


def client_snapshot():
    paths = [p for folder in ['client/assets/scripts', 'client/tests'] for p in (ROOT / folder).rglob('*') if p.is_file()]
    paths += [p for pattern in ['*.scene', '*.prefab'] for p in (ROOT / 'client/assets').rglob(pattern)]
    paths += [ROOT / p for p in ['client/assets/resources/development/gameplay.json', 'client/assets/resources/development/manifest.json', 'config/development/first-loop.json']]
    need(all(not p.is_symlink() and not getattr(p, 'is_junction', lambda: False)() for p in paths), 'client输入不得经链接跳到外部')
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(set(paths))}


def engine_evidence(report):
    before, after, recorded = [report[key] for key in ['external_engine_before', 'external_engine_after', 'external_engine_current']]
    need(isinstance(before, dict) and before and before == after == recorded, '独立引擎源码前后不等')
    expected = {str((ENGINE_ROOT / name).resolve()) for name in ENGINE_FILES}
    actual = {str(Path(name).resolve()) for name in recorded}
    need(actual == expected and len(actual) == len(recorded), '独立引擎源不是受控Creator3.8.8精确白名单')
    for name, digest in recorded.items():
        p = Path(name).resolve()
        need(p.is_relative_to(ENGINE_ROOT) and p.is_file() and not p.is_symlink() and sha(p) == digest, '独立引擎输入变化：' + name)
    return len(recorded)


def creator_snapshot(project):
    """独立重扫所有受控树，只排唯一已审Editor问卷缓存；build目录不使用此排除。"""
    result = {}
    for name in CREATOR_INPUTS:
        path = project / name
        need(path.exists(), 'Creator缺少完整受控树：' + name)
        candidates = [path] if path.is_file() else [path, *path.rglob('*')]
        for p in candidates:
            relative = p.relative_to(project).as_posix()
            if relative in EDITOR_CACHES:
                continue
            need(not p.is_symlink() and not getattr(p, 'is_junction', lambda: False)(), 'Creator输入不得链接外部：' + str(p))
            if p.is_file():
                result[relative] = sha(p)
    return result


def complete_tree(directory):
    paths = [directory, *directory.rglob('*')]
    need(all(not p.is_symlink() and not getattr(p, 'is_junction', lambda: False)() for p in paths), '构建输出不可经链接逃逸')
    return {p.relative_to(directory).as_posix(): sha(p) for p in paths if p.is_file()}


def node_counts(path):
    text = Path(path).read_text(encoding='utf-8')
    values = {key: int(value) for key, value in re.findall(r'^(?:ℹ|#)\s+(tests|pass|fail|skipped|cancelled|todo)\s+(\d+)\s*$', text, re.M)}
    need(set(values) == {'tests', 'pass', 'fail', 'skipped', 'cancelled', 'todo'}, '缺少真实Node终态计数：' + str(path))
    need(values['tests'] == values['pass'] > 0 and all(values[key] == 0 for key in ['fail', 'skipped', 'cancelled', 'todo']), 'Node未全通过：' + str(path))
    return values


def command_check(name):
    path = WORK / 'checks' / (name + '.json')
    value = read(path)
    need(value['exit_code'] == 0, '检查未通过：' + name)
    output = project_path(value['output'])
    need(sha(output) == value['output_sha256'], '检查原始输出摘要不同：' + name)
    return value, output


def qa_tools_evidence():
    report = read(WORK / 'qa-tools-final/report.json')
    need(report['passed'] and report['inputs_stable'] and report['before'] == report['after'] and len(report['after']) == 11, '最新QA工具不是11输入稳定终态')
    current(report['after'], label='最新QA工具')
    need({item['name'] for item in report['commands']} == {'python', 'node'}, 'QA工具纯测缺失')
    counts = {}
    for item in report['commands']:
        need(item['exit_code'] == 0, '最新QA工具纯测失败')
        output = project_path(item['output'])
        need(sha(output) == item['output_sha256'], 'QA工具纯测原始日志不同')
        if item['name'] == 'node':
            counts['node'] = node_counts(output)['pass']
        else:
            text = output.read_text(encoding='utf-8')
            matched = re.search(r'^Ran (\d+) tests? in ', text, re.M)
            need(matched and re.search(r'^OK\s*$', text, re.M) and not re.search(r'^(?:FAILED|ERROR|FAIL:)', text, re.M), 'QA工具Python未成功')
            counts['python'] = int(matched.group(1))
    return report, counts


def go_evidence():
    report_path = ROOT / 'server/test-results/latest.json'
    events_path = ROOT / 'server/test-results/latest.events.jsonl'
    report = read(report_path)
    need(report['passed'] and report['mongo_e2e_explicit'] and report['race_detector'] and report['exit_code'] == 0, '最终Go未真实Mongo/race通过')
    need(report['source_changed_during_test'] is False and report['source_sha256'] == report['source_sha256_after'], 'Go运行中输入变化')
    need(len(report['source_sha256']) == 164, 'Go不属于本轮164输入冻结合同')
    current(report['source_sha256'], label='Go')
    # 只调用已锁定runner的文件枚举函数，不运行其main、Go或数据库检查。
    definition = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='read_only_input_enumeration')
    actual_paths = {p.relative_to(ROOT).as_posix() for p in definition['input_files']()}
    need(actual_paths == set(report['source_sha256']), 'Go冻结后新增或删除了受控输入')
    need(sha(events_path) == report['events_sha256'], 'Go raw events摘要不同')
    events = [json.loads(line) for line in events_path.read_text(encoding='utf-8').splitlines() if line.strip()]
    passed = [{'package': e['Package'], 'test': e['Test']} for e in events if e.get('Action') == 'pass' and e.get('Test')]
    skipped = [{'package': e['Package'], 'test': e['Test']} for e in events if e.get('Action') == 'skip' and e.get('Test')]
    failed = [{'package': e['Package'], 'test': e.get('Test')} for e in events if e.get('Action') in {'fail', 'build-fail'}]
    need(passed == report['passed_tests'] and skipped == report['skipped_tests'] and failed == report['failed_tests'], 'Go原始events与报告计数不一致')
    need(passed and not skipped and not failed, 'Go存在失败或test skip')
    need(report['fixture']['sha256'] == sha(ROOT / 'config/development/first-loop.json'), 'Go不是当前fixture')
    command_check('go')
    return report, {'passed': len(passed), 'failed': len(failed), 'skipped': len(skipped), 'inputs': len(report['source_sha256']), 'raw_package_actions': dict(Counter(e['Action'] for e in events if not e.get('Test')))}


def client_evidence(report_path):
    report_path = project_path(report_path)
    need(report_path.is_relative_to(WORK / 'client') and report_path.name == 'report.json', '必须显式选择本轮client验证报告')
    report = read(report_path)
    need(report['passed'] and report['stable'] and report['freeze_started_before_commands'], '所选client未冻结通过')
    need(type(report['input_count']) is int and report['input_count'] > 0 and report['input_count'] == len(report['before']) == len(report['after']) == len(report['current']), 'client声明count与实际集合不同')
    need(report['before'] == report['after'] == report['current'], 'client前后摘要不等')
    current(report['current'], label='client')
    need(client_snapshot() == report['current'], 'client独立全范围重扫不等，不能只验任意子集')
    engine_count = engine_evidence(report)
    need({c['name'] for c in report['commands']} == {'models', 'full-ui-app', 'strict', 'protocol-check', 'protocol-lint'}, 'client五命令缺失')
    for item in report['commands']:
        need(item['exit_code'] == 0, 'client命令失败：' + item['name'])
        need(sha(project_path(item['log'])) == item['sha256'], 'client日志摘要不同')
    app = node_counts(project_path(next(c['log'] for c in report['commands'] if c['name'] == 'full-ui-app')))
    _, scene_path = command_check('scene')
    scene = node_counts(scene_path)
    _, python_path = command_check('python')
    raw_python = python_path.read_text(encoding='utf-8')
    matched = re.search(r'^Ran (\d+) tests? in ', raw_python, re.M)
    need(matched and re.search(r'^OK\s*$', raw_python, re.M) and not re.search(r'^(?:FAILED|ERROR|FAIL:)', raw_python, re.M), 'Python不是完整成功终态')
    _, pb_path = command_check('protobuf')
    pb = read(pb_path)
    need(pb['status'] == 'passed', 'PB codec未通过')
    _, protocol_path = command_check('protocol')
    protocol = read(protocol_path)
    need(protocol['valid'] is True, '协议清单未通过')
    return report_path, report, app, scene, int(matched.group(1)), pb, protocol, engine_count


def main_creator():
    path = ROOT / 'docs/客户端构建检查.json'
    report = read(path)
    need(report['creator'] == '3.8.8' and report['success'] and report['exit_code'] == 36, '主Creator未真实通过')
    need(report['source_stable_during_build'] and report['scene_prefab_layout_preserved'], '主Creator源码/布局不稳定')
    current(report['script_hashes'], label='主Creator脚本')
    current(report['asset_hashes'], label='主Creator保存布局')
    need(Path(report['log']).is_file(), '主Creator缺实际日志')
    return report


def assets_evidence():
    before = read(WORK / 'before.json')['files']
    counts = dict(png=0, map_files=0, saved_layout=0, resource_meta=0, source_town_files=0)
    for relative, digest in before.items():
        path = ROOT / relative
        categories = []
        if path.suffix == '.png': categories.append('png')
        if '/maps/' in relative or '/source-maps/' in relative: categories.append('map_files')
        if path.suffix in {'.scene', '.prefab'}: categories.append('saved_layout')
        if path.suffix == '.meta': categories.append('resource_meta')
        if relative.startswith('client/assets/resources/source-maps/'): categories.append('source_town_files')
        if categories:
            need(sha(path) == digest, '旧资源/保存布局变化：' + relative)
        for category in categories:
            counts[category] += 1
    post = read(WORK / 'resource-diff-post-creator.json')
    need({k: counts[k] for k in post['old_files_unchanged']} == post['old_files_unchanged'], 'post-Creator保留计数不同')
    current(post['added_resource_sha256'], label='新增原帧/WAV/真实meta')
    added_now = {p.relative_to(ROOT).as_posix() for p in (ROOT / 'client/assets/resources').rglob('*') if p.is_file() and p.relative_to(ROOT).as_posix() not in before}
    need(added_now == set(post['added_resource_sha256']), 'post-Creator之后出现未解释的新资源')
    need(not post['unknown_new_resources'] and not post['missing_expected_resources'], '资源有未解释增量')
    manifest = read(ROOT / 'client/assets/resources/development/manifest.json')
    need(sha(ROOT / 'client/assets/resources/development/manifest.json') == post['manifest_sha256'], '正式资源manifest变化')
    old_manifest = read(WORK / 'manifest-before.json')
    need(all(manifest['frames'][k] == v for k, v in old_manifest['frames'].items()), '旧frame元数据变化')
    need(all(manifest['effects'][k] == v for k, v in old_manifest['effects'].items()), '旧effect变化')
    profiles = json.loads(json.dumps(manifest['profiles']))
    profiles['pet_skeleton']['layers'][0]['actions'].pop('appear')
    need(profiles == old_manifest['profiles'], '其它profile或原宠物动作变化')
    source = manifest['pet_birth_source']
    need(sha(ROOT / source['contract']) == source['contract_sha256'] and sha(ROOT / source['compiler']) == source['compiler_sha256'], '资源子编译器/来源锁变化')
    need(sha(ROOT / manifest['converter']) == manifest['converter_sha256'], '主converter变化')
    sys.path.insert(0, str(ROOT / 'tools/resources'))
    import pet_birth_assets
    compiled = pet_birth_assets.compile_contract(ROOT)
    caster, birth = compiled['cast'], compiled['birth']
    new_frames = {f"{item['package']}:{item['raw_start'] + index}" for item in [caster, birth] for index in range(item['count'])}
    need(set(manifest['frames']) - set(old_manifest['frames']) == new_frames, '新增帧不是来源合同派生集合')
    for item in manifest['audio'].values():
        need(item['status'] == 'candidate' and item['development_only'] is True, '声音未保持候选')
        need(sha(ROOT / item['source']) == sha(ROOT / 'client/assets/resources/development' / item['path']) == item['sha256'], '原声被改写')
    apply = read(WORK / 'workbook-apply-report.json')
    need(apply['all_other_cell_values_preserved'] and apply['all_formulas_preserved'], '唯一Excel有未解释变化')
    need(len(apply['cell_differences']) == 1 and apply['cell_differences'][0]['only_json_path'] == 'rule_version' and apply['cell_differences'][0]['before'] == 17 and apply['cell_differences'][0]['after'] == 18, '不是单格rule17→18')
    need(sha(ROOT / 'outputs/development-prep-20261026/玩法配置.xlsx') == apply['output_sha256'], '唯一玩法book已变化')
    original = read(WORK / 'resource-diff.json')['current_hashes']
    need(sha(ROOT / 'outputs/development-prep-20261026/架构配置.xlsx') == original['outputs/development-prep-20261026/架构配置.xlsx'], '架构book已变化')
    return counts, post, manifest, apply


def gui_evidence(directory, main_fixture):
    directory = project_path(directory, WORK)
    need(directory.is_dir(), '--gui-run必须是本轮明确run目录')
    parent = read(directory / 'report.json')
    runtime = read(directory / 'runtime.json')
    child = read(directory / 'browser/report.json')
    need(parent['passed'] and parent['main_restored'] and parent['qa_stopped'] and parent['static_stopped'] and parent['qa_database_dropped'], 'GUI父终态/恢复/清理未通过')
    need(not parent['cleanup_errors'] and parent['protected_unchanged'] and parent['tools_stable'] and parent['credentials_written'] is False, 'GUI保护输入/工具或清理失败')
    need(parent['browser_exit_code'] == 0 and parent['browser'] == {key: child[key] for key in ['passed', 'checks', 'errors', 'warnings']}, '父子GUI结果不一致')
    need(child['passed'] and isinstance(child['checks'], list) and child['checks'] and isinstance(child['warnings'], list) and child['errors'] == [], '子GUI未真实通过')
    for key in ['privateCastInvoked', 'routeOverride', 'bodySeeded', 'credentialsWritten']:
        need(child[key] is False, 'GUI存在禁止的状态替代：' + key)
    need(child['mainProtectedInputsStable'] is True, 'GUI主输入漂移')
    need(parent['protected_before'] == parent['protected_after'], 'GUI父保护输入前后不等')
    need(parent['tools_before'] == parent['tools_after'], 'GUI父工具前后不等')
    current(parent['protected_after'], label='GUI主保护输入')
    current(parent['tools_after'], label='GUI工具')
    run = runtime['run_id']
    need(re.fullmatch(r'[0-9a-f]{32}', run) and parent['run_id'] == run and child['runId'] == run and directory.name == 'run-' + run, 'GUI run身份不同')
    need(runtime['project_root'] == str(ROOT) and runtime['purpose'] == 'isolated_pet_visual_gui' and runtime['rule_version'] == 18, 'GUI runtime身份不合法')
    name = 'minimir_dev_game_test_pet_viz_' + run
    need(len(name.encode('utf-8')) <= 63 and parent['database'] == runtime['database'] == name, 'GUI清理目标不是精确当前62字节库')
    config_path = directory / 'conf/dev/server.json'
    fixture_path = directory / 'config/development/first-loop.json'
    config = read(config_path)
    fixture = read(fixture_path)
    need(config['database'] == name and Path(config['fixture_file']).resolve() == fixture_path, 'GUI配置与精确库/fixture不配对')
    need(sha(config_path) == runtime['config_sha256'] and sha(fixture_path) == runtime['fixture_sha256'] == child['fixtureHash'], 'GUI配置/fixture摘要不同')
    need(fixture['rule_version'] == 18 and fixture['casting_supply']['enabled'] and fixture['pet_runtime']['bootstrap_mode'] == 'none', '不是显式17隔离前态')
    need(sha(fixture_path) != sha(ROOT / 'config/development/first-loop.json'), '把主关闭配置冒充隔离配置')
    need(runtime['ports'] == {'account': 18110, 'game': 18111, 'gm': 18112}, 'QA业务端口不合法')
    need(parent['main_after']['running'] is True and parent['main_after']['pid'] != runtime['pid'], '主服务没有独立恢复')
    need(set(parent['main_after']['health']) == {'account', 'game', 'gm'}, '主恢复缺节点')
    for mode, health in parent['main_after']['health'].items():
        need(health['mode'] == mode and health['status'] == 'ok' and health['rule_version'] == 18 and health['fixture_hash'] == sha(ROOT / 'config/development/first-loop.json'), '恢复的主节点不是当前主fixture')
        need(health['instance'] != runtime['health_instances'][mode], '主节点仍是临时QA实例')
    artifact = project_path(runtime['artifact_directory'], WORK)
    project = artifact / 'client-project'
    need(Path(runtime['project']).resolve() == project and Path(runtime['creator_report']).resolve() == directory / 'creator-report.json', '复用工程/新run报告路径不合法')
    copied = read(directory / 'project-copy.json')
    creator = read(directory / 'creator-report.json')
    need(Path(copied['project']).resolve() == project and Path(creator['project']).resolve() == project, '真实Creator指向不同artifact')
    need(creator['passed'] and creator['source_stable'] and creator['creator'] == '3.8.8' and creator['exit_code'] == 36 and creator['platform'] == 'web-mobile' and creator['rule_version'] == 18, '独立Creator未实际通过')
    need(copied['only_public_changed'] and copied['scene_prefab_preserved'] and copied['normal_creator_project'] and copied['runtime_resource_override'] is False, '独立工程不是受控复制')
    need(copied['excluded_generated_editor_caches'] == creator['excluded_generated_editor_caches'] == EDITOR_CACHES, '缺少精确单项Editor缓存合同；旧包不能复用')
    generated_caches = creator['generated_editor_cache_sha256']
    need(isinstance(generated_caches, dict) and set(generated_caches).issubset(EDITOR_CACHES) and all(isinstance(value, str) and re.fullmatch(r'[0-9a-f]{64}', value) for value in generated_caches.values()), 'Editor缓存原始摘要声明不合法')
    # 只调用输入枚举和纯public投影，不调用构建、启动或数据库函数。
    sys.path.insert(0, str(ROOT / 'tools/client'))
    import isolated_creator as creator_tools
    need(creator_tools.EDITOR_CACHES == EDITOR_CACHES and creator_tools.SELECTED == CREATOR_INPUTS, '工具输入树或Editor排除范围与已审合同不同')
    need(creator_snapshot(ROOT / 'client') == copied['source_sha256'], 'artifact与当前主工程完整输入树不同')
    projected = (json.dumps(creator_tools.public_gameplay(fixture), ensure_ascii=False, indent=2) + '\n').encode('utf-8')
    public_sha = hashlib.sha256(projected).hexdigest()
    expected = {**copied['source_sha256'], 'assets/resources/development/gameplay.json': public_sha}
    need(expected == copied['project_sha256'] == creator['input_sha256'] == creator_snapshot(project), '同fixture public/独立完整输入树不同')
    need(creator['public_sha256'] == copied['public_sha256'] == child['publicHash'] == public_sha, 'GUI public编译值不同')
    need(complete_tree(project / 'build/web-mobile') == creator['build_sha256'] and 'index.html' in creator['build_sha256'], '独立compiled全树产物漂移')
    need(parent['artifact_reused'] == runtime['artifact_reused'] and Path(parent['artifact_directory']).resolve() == artifact, '父/runtime复用状态不同')
    if runtime['artifact_reused']:
        proof_path = directory / 'reused-artifact.json'
        need(Path(runtime['artifact_proof']).resolve() == proof_path and artifact != directory, '复用proof路径不合法')
        proof = read(proof_path)
        for key in ['passed', 'current_source_matches', 'isolated_inputs_match', 'build_outputs_match']:
            need(proof[key] is True, '构建复用proof未通过：' + key)
        need(proof['run_id'] == run and proof['fixture_sha256'] == sha(fixture_path), '复用proof不属于本次run/fixture')
        need(Path(proof['artifact_directory']).resolve() == artifact and Path(proof['project']).resolve() == project, '复用proof artifact不匹配')
        need(proof['creator_report_sha256'] == sha(artifact / 'creator-report.json') == sha(directory / 'creator-report.json') and proof['project_copy_sha256'] == sha(artifact / 'project-copy.json') == sha(directory / 'project-copy.json'), '复制的原构建报告字节不同')
        need(proof['public_sha256'] == public_sha and proof['runtime_resource_override'] is False and proof['creator_rerun'] is False, '复用proof混入资源覆盖/伪重新构建')
        need(proof['source_files'] == len(copied['source_sha256']) and proof['build_files'] == len(creator['build_sha256']), '复用proof文件计数不等')
    else:
        need(artifact == directory and runtime['artifact_proof'] is None, '非复用run不得借用另一工程')
    static_path = directory / 'static-runtime.json'
    need(Path(runtime['static_runtime']).resolve() == static_path, '静态runtime路径不配对')
    static = read(static_path)
    need(static['run_id'] == run and static['pid'] == runtime['static_pid'] and static['project_root'] == str(ROOT), '静态PID/run身份不同')
    need(runtime['static_port'] == parent['static_port'] == 18082, '本次隔离测试静态端口不明确')
    argv = static['argv']
    need(argv[argv.index('--port') + 1] == str(runtime['static_port']) and argv[argv.index('--instance-id') + 1] == run and Path(argv[argv.index('--build-root') + 1]).resolve() == project / 'build/web-mobile', '静态argv未锁定当前artifact/端口/run')
    need(argv[argv.index('--account-port') + 1] == str(runtime['ports']['account']) and argv[argv.index('--game-port') + 1] == str(runtime['ports']['game']), '隔离静态服务上游不是当前QA端口')
    need(runtime['static_pid'] not in {runtime['pid'], parent['main_after']['pid']}, '静态/QA/主进程身份混用')
    qualification = read(directory / 'qualification.json')
    need(qualification == child['qualification'] and qualification['passed'] and qualification['only_before_owner'] and qualification['book_from_normal_create'] and qualification['explicit_count'] == 2, '不是正常创角后未Enter资格')
    for key in ['skills_seeded', 'binding_seeded', 'position_seeded', 'body_seeded', 'pack_size_or_price_inferred']:
        need(qualification[key] is False, '资格前态越界：' + key)
    manifest = read(project / 'assets/resources/development/manifest.json')
    need(child['manifestHash'] == sha(project / 'assets/resources/development/manifest.json') == sha(ROOT / 'client/assets/resources/development/manifest.json'), 'GUI不是当前来源资源')
    appearance = manifest['profiles']['pet_skeleton']['layers'][0]['actions']['appear']
    wanted = {0, (appearance['count'] - 1) // 2, appearance['count'] - 1}
    need({g['frame'] for g in child['gpu']} == wanted and len(child['gpu']) == len(wanted), '缺实际GPU首中末帧')
    png_paths = []
    for capture in child['gpu']:
        image = project_path(capture['file'], directory / 'browser')
        data = image.read_bytes()
        need(sha(image) == capture['sha256'] and data.startswith(b'\x89PNG\r\n\x1a\n') and len(data) > 1024, 'GPU PNG摘要/内容无效')
        width, height = struct.unpack_from('>II', data, 16)
        need(width > 0 and height > 0 and capture['action'] == 'appear' and capture['phase'] == 'fresh', 'GPU截图不是实际fresh出场')
        need(capture['frames'][0] == appearance['directions'][capture['direction']]['frames'][capture['frame']], 'GPU帧没有匹配来源原格')
        need(capture['blend'][0]['source'] == capture['blendConstants']['source'] and capture['blend'][0]['destination'] == capture['blendConstants']['destination'], 'GPU材质blend未匹配公开枚举')
        png_paths.append(image)
    audio = child['freshObservation']['audio']
    for index, cue in enumerate([manifest['effects']['17']['cast_sound'], appearance['sound']]):
        need(any(s['gestureUnlocked'] and s['voices'][index]['playing'] and s['voices'][index]['clipBound'] and s['voices'][index]['playCalls'] > 0 and s['voices'][index]['cue'] == cue for s in audio), '未观察到真实AudioSource播放：' + cue)
    need(len(child['born']) == 1 and child['born'][0]['skill'] == 17 and child['born'][0]['cast'] in {c['cast'] for c in child['casts'] if c['skill'] == 17}, '出生事实不唯一或未配施法')
    need(not any(s['phase'] in {'recall', 'healthy'} for s in child['finalObservation']['samples']), 'Recall/健康重入重播出生')
    need(child['protocolErrors'] == [], 'GUI有协议错误')
    states = {state['name']: state for state in child['states']}
    initial, fresh, recall, healthy = [states[key] for key in ['01-normal-new-role', '03-fresh-pet', '04-unequipped-recall', '05-healthy-reentry']]
    need(initial['pets'] == [] and initial['books'] == ['1'] and initial['materialBag'] == ['2'] and initial['learned'] == [], '初始角色存在预置宠/技能或非两Count')
    need(initial['level'] == qualification['level'] and fresh['materialEquipped'] == ['1'] and recall['materialBag'] == ['1'], '学习/新召/Recall材料前态不同')
    identity = (fresh['pets'][0]['handle'], fresh['pets'][0]['life'])
    need(identity == (recall['pets'][0]['handle'], recall['pets'][0]['life']) == (healthy['pets'][0]['handle'], healthy['pets'][0]['life']), 'Recall/健康重入替换birth')
    need(recall['audio']['voices'][1]['playCalls'] == fresh['audio']['voices'][1]['playCalls'] and recall['audio']['voices'][0]['playCalls'] > fresh['audio']['voices'][0]['playCalls'], 'Recall声音语义错误')
    need(all(v['playCalls'] == 0 for v in healthy['audio']['voices']) and all(s['action'] != 'appear' for s in healthy['appearance']), '健康重观播放声音或出生')
    need(healthy['binding'] != recall['binding'], '所谓健康重入没有新票绑定')
    for name in states:
        image = directory / 'browser' / (name + '.png')
        need(image.is_file(), '缺正常UI状态截图：' + name)
        png_paths.append(image)
    need(parent['creator'] == {key: creator[key] for key in ['passed', 'seconds', 'exit_code', 'source_stable', 'public_sha256']}, '父记录的原真实构建摘要不同')
    return parent, runtime, child, creator, artifact, png_paths


def later_failed_runs():
    specs = [
        ('a2e19b4968b142078cd7c78f3ab7829d', 'actual_audio_unlock_failure'),
        ('422a443fc2e84cd4a240ed9e2a19b13d', 'actual_lifecycle_cleanup_failure'),
        ('425e92f75bf946afb173ef23be024c21', 'editor_questionnaire_cache_raw_guard_failure'),
    ]
    summaries = []
    for run_id, scope in specs:
        directory = WORK / 'gui' / ('run-' + run_id)
        parent, creator = read(directory / 'report.json'), read(directory / 'creator-report.json')
        need(parent['passed'] is False and parent['main_restored'] is True, '后续历史失败被改写成成功或未恢复')
        summary = {'run_id': run_id, 'scope': scope, 'parent_passed': parent['passed'], 'creator_seconds': creator['seconds'], 'creator_exit_code': creator['exit_code'], 'creator_source_stable': creator['source_stable'], 'raw_error': parent['error']}
        if scope == 'editor_questionnaire_cache_raw_guard_failure':
            copied = read(directory / 'project-copy.json')
            before, after = copied['project_sha256'], creator['input_sha256']
            changed = [name for name in before if before[name] != after[name]]
            need(set(before) == set(after) and changed == EDITOR_CACHES and creator['exit_code'] == 36 and creator['source_stable'] is False and creator['passed'] is False, '旧Editor缓存失败不再是原始raw guard边界')
            need(not (directory / 'browser/report.json').exists() and not (directory / 'runtime.json').exists(), 'Editor guard失败历史不能冒称运行过GUI')
            summary.update(changed_raw_input_keys=changed, cache_before_sha256={name: before[name] for name in changed}, cache_after_sha256={name: after[name] for name in changed}, gui_executed=False, note='旧raw稳定失败原样保留；新单项缓存合同不追溯改写它')
        else:
            child = read(directory / 'browser/report.json')
            need(child['passed'] is False and parent['qa_stopped'] and parent['static_stopped'] and parent['qa_database_dropped'], '历史真实产品失败或清理记录不同')
            need(len(child['born']) == 1 and len(child['gpu']) == 3, '历史局部出生/GPU通过证据不同')
            summary.update(partial_checks=child['checks'], errors=child['errors'], warnings=child['warnings'], gpu_frames=child['gpu'], born_count=len(child['born']), cast_count=len(child['casts']), qa_stopped=parent['qa_stopped'], static_stopped=parent['static_stopped'], qa_database_dropped=parent['qa_database_dropped'])
            if scope == 'actual_audio_unlock_failure':
                audio = child['failureState']['audio']
                need(audio['gestureUnlocked'] is False and all(voice['playCalls'] == 0 for voice in audio['voices']), '未解锁历史不再匹配实际声道记录')
                summary.update(observed_appear_frames=sorted({sample['frame'] for sample in child['failureObservation']['samples']}), audio_gesture_unlocked=audio['gestureUnlocked'], voice_play_calls=[voice['playCalls'] for voice in audio['voices']], note='原始GPU通过，真实音频未解锁；不能按完整出生阶段通过')
            else:
                joined = '\n'.join(child['errors'])
                need('off' in joined or '_eventProcessor' in joined, '换角销毁历史不匹配原异常')
                audio = child['freshObservation']['audio']
                played = {voice['cue'] for snapshot in audio for voice in snapshot['voices'] if voice['playing'] and voice['playCalls'] > 0}
                need({'summon_skeleton_cast', 'pet_skeleton_born'}.issubset(played), '换角失败前的双声道证据缺失')
                summary.update(actual_played_cues=sorted(played), note='新生/双声道/Recall局部通过；换角销毁抛错，不能称健康重入通过')
        summaries.append(summary)
    return summaries


def normal_browser(path, kind, main_build_path):
    path = project_path(path)
    need(path.is_relative_to(ROOT / '.work'), '正常浏览器报告必须在本项目.work')
    value = read(path)
    need(value['passed'] and value['errors'] == [] and isinstance(value['checks'], list) and value['checks'] and isinstance(value['warnings'], list), kind + '正常浏览器未通过')
    need(path.stat().st_mtime_ns >= main_build_path.stat().st_mtime_ns, kind + '报告早于当前主Creator')
    if kind == 'town':
        need(value['expectedRuleVersion'] == '18' and value['bindingVerified'] is True and value['physicalMobileVerified'] is False, 'Town不是本轮正常主rule18触屏')
        need(value['readyRuleChecks'] and all(str(item['resourceVersion']) == '18' for item in value['readyRuleChecks']), 'Town有旧规则入场')
    else:
        need(value['ruleVersion'] == 18 and value['touchInput'] is True and value['physicalMobileVerified'] is False, 'UI不是本轮正常主rule18触屏')
    return path, value


def normal_selector(town_path, ui_path, gui_dir):
    base = town_path.parent.parent
    need(ui_path.parent.parent == base, '主Town/UI不是同一次明确正常回归')
    selector = read(base / 'selector.json')
    summary = read(base / 'report.json')
    need(project_path(selector['directory']) == base and project_path(selector['summary']) == base / 'report.json', '主回归selector路径不同')
    need(project_path(selector['town_binding']['report']) == town_path and project_path(selector['general_ui']['report']) == ui_path, '主回归selector不是所选两个报告')
    need(summary['passed'] and summary['main_unchanged'] and summary['preview_unchanged'] and summary['protected_inputs_unchanged'], '主浏览器回归状态不稳定')
    need(summary['main_process_or_configuration_modified'] is False and summary['direct_database_access'] is False and summary['credentials_written'] is False, '主回归包含非正常前态操作')
    need(summary['fixture_sha256'] == sha(ROOT / 'config/development/first-loop.json'), '主回归不是当前主fixture')
    need(project_path(summary['gui_terminal']['report']) == gui_dir / 'report.json' and summary['gui_terminal']['sha256'] == sha(gui_dir / 'report.json'), '正常主回归没有配对本次GUI终态')
    before, after = read(base / 'inputs-before.json'), read(base / 'inputs-after.json')
    need(before == after, '正常主回归受保护输入前后不同')
    current(after, label='主浏览器回归')
    preview_before, preview_after = read(base / 'preview-before.json'), read(base / 'preview-after.json')
    need(preview_before == summary['preview_before'] and preview_before == preview_after, '主回归静态预览原始前后状态不同')
    need({item['name'] for item in summary['steps']} == {'town_binding', 'general_ui'}, '主回归步骤不全')
    for item in summary['steps']:
        need(item['passed'] and item['exit_code'] == 0 and item['errors'] == [], '主浏览器步骤未成功')
        report_path = project_path(item['report'])
        need(sha(report_path) == item['report_sha256'], '主浏览器子报告SHA不等')
        child = read(report_path)
        need(item['checks'] == len(child['checks']) and item['warnings'] == child['warnings'], '主回归步骤计数不同')
    return base, summary


def final_service_snapshots(normal_summary, normal_base):
    values = {}
    for name in ['main-status', 'mongodb-status', 'client-status']:
        _, output = command_check(name)
        values[name] = read(output)
    main, mongo, preview = [values[name] for name in ['main-status', 'mongodb-status', 'client-status']]
    need(main['running'] and main == normal_summary['main_after'], '最终main状态不等于正常主回归后的同实例状态')
    observed_preview = read(normal_base / 'preview-after.json')
    need(preview['running'] and all(observed_preview[key] == value for key, value in preview['health'].items()), '最终静态预览不等于正常主回归后的同实例身份')
    need(set(observed_preview) == set(preview['health']) | {'index_sha256'} and sha(ROOT / 'client/build/web-mobile/index.html') == observed_preview['index_sha256'], '主回归记录的实际index产物变化')
    need(mongo['running'] and mongo['endpoint_verified'] and mongo['ownership_verified'] and mongo['listener_verified'] and mongo['production_ready'] is False, '最终Mongo专属所有权状态未通过')
    config = read(ROOT / 'conf/dev/server.json')
    need(mongo['uri'] == config['mongo_uri_by_platform']['windows'], '最终Mongo不是当前开发专属端点')
    return values


def curate_jobs(gui_dir, artifact, gpu, town_path, ui_path, client_path, normal_base):
    jobs, skipped, references = {}, [], []

    def add(source, target, scope):
        source = project_path(source)
        relative = source.relative_to(ROOT)
        need(not FORBIDDEN_PARTS.intersection(relative.parts) and source.suffix.lower() not in {'.exe', '.dll', '.xlsx', '.lib', '.wav'}, '禁止归档工程缓存/二进制/原包：' + str(source))
        need(not ('go-full-attempt1' in relative.parts and 'inputs' in relative.parts), '禁止整套旧inputs')
        need(source.is_file() and not source.is_symlink(), '归档源文件不存在/非独立文件：' + str(source))
        target = Path(target)
        need(not target.is_absolute() and '..' not in target.parts, '归档目标越界')
        key = target.as_posix()
        record = {'source': relative.as_posix(), 'target': key, 'sha256': sha(source), 'scope': scope}
        need(key not in jobs or jobs[key] == record, '归档目标冲突：' + key)
        jobs[key] = record

    def add_small_tree(directory, target, scope):
        """仅指定的红绿/历史证据小目录；不扩展为整个.work或工程树。"""
        need(directory.is_dir(), '缺必要历史目录：' + str(directory))
        files = [p for p in directory.rglob('*') if p.is_file() and not FORBIDDEN_PARTS.intersection(p.relative_to(directory).parts)]
        need(len(files) <= 1000 and sum(p.stat().st_size for p in files) <= 100 * 1024 * 1024, '历史小目录异常扩大，需Root重新审白名单')
        allowed = {'.json', '.jsonl', '.txt', '.log', '.md', '.png', '.py', '.cjs', '.js', '.ts', '.patch', '.before'}
        for source in files:
            if source.suffix.lower() not in allowed:
                skipped.append({'path': source.relative_to(ROOT).as_posix(), 'reason': '不是已审小证据文件类型，不归档原包/二进制'})
                continue
            add(source, target / source.relative_to(directory), scope)

    plan = read(WORK / 'curate_plan.json')
    for category in plan['categories']:
        for item in category['files']:
            source = project_path(item['path'])
            if item['mode'] == 'reference_hash_only':
                references.append({'path': item['path'], 'current_sha256': sha(source), 'plan_sha256': item['sha256']})
                continue
            if not source.is_file() or sha(source) != item['sha256']:
                skipped.append({'path': item['path'], 'reason': '已不等于计划快照，不能套用其历史分类；当前检查另列'})
                continue
            scope = category['classification']
            if category['id'] == 'client_validation2' and client_path != WORK / 'client/validation-2/report.json':
                scope = 'historical_client_before_material_wiring_fix'
            add(source, Path('curated') / source.relative_to(WORK), scope)
    for name in ['references', 'pet-sources', 'resources', 'fixture', 'config', 'candidate', 'python', 'go', 'build', 'protocol', 'protobuf', 'bindings', 'scene-compile', 'scene', 'host', 'client', 'models', 'typecheck', 'qa-tools', 'browser-tools', 'birth-assets', 'main-status', 'mongodb-status', 'client-status']:
        check, output = command_check(name)
        add(WORK / 'checks' / (name + '.json'), Path('current/checks') / (name + '.json'), 'current_check')
        add(output, Path('current/checks') / output.name, 'current_check')
    for source, target in [('server/test-results/latest.json', 'current/server/latest.json'), ('server/test-results/latest.events.jsonl', 'current/server/latest.events.jsonl'), ('docs/客户端构建检查.json', 'current/main-creator/report.json')]:
        add(ROOT / source, target, 'current_verified')
    add(Path(read(ROOT / 'docs/客户端构建检查.json')['log']), 'current/main-creator/creator.log', 'current_verified')
    for name in ['report.json', 'runtime.json', 'static-runtime.json', 'creator-report.json', 'project-copy.json', 'qualification.json', 'browser-output.txt', 'main-restore.txt', 'browser/report.json']:
        add(gui_dir / name, Path('current/gui') / name, 'current_live_gui_verified')
    add(artifact / 'creator.log', 'current/gui/original-creator.log', 'current_reused_or_fresh_build')
    if (gui_dir / 'reused-artifact.json').is_file():
        add(gui_dir / 'reused-artifact.json', 'current/gui/reused-artifact.json', 'current_strict_build_reuse')
    for source in gpu:
        add(source, Path('current/gui/browser') / source.name, 'actual_gpu_or_normal_ui_screenshot')
    if artifact != gui_dir:
        add(artifact / 'report.json', 'history/reused-artifact-source-parent/report.json', 'original_build_pass_but_prior_gui_attempt_not_current_gui')
    for kind, report_path, names in [('town', town_path, ['01-personal.png', '02-town-entry.png', '03-safe-zone.png', '03a-role-binding.png', '04-town-reentry.png', '05-personal-return.png']), ('ui', ui_path, ['02-game.png', '03-navigation-panel.png', '04-taller-viewport.png', '05-healthy-reentry.png', 'navigation-trace.json'])]:
        add(report_path, Path('current/browser-' + kind) / 'report.json', 'current_normal_main_browser')
        for name in names:
            add(report_path.parent / name, Path('current/browser-' + kind) / name, 'normal_touch_screenshot_or_navigation_observation')
    for name in ['finalize_evidence.py', 'update_docs.py']:
        add(WORK / name, Path('templates') / name, 'reviewed_phase_template')
    # DB65失败只读不存在证明是独立后续证据，原失败report不改写。
    add(WORK / 'gui-invalid-name-followup.json', 'history/db65-readonly-followup.json', 'historical_failure_readonly_followup')
    add(WORK / 'verify_invalid_db_name.py', 'templates/verify_invalid_db_name.py', 'historical_readonly_probe_source')
    for name in ['report.json', 'python.txt', 'node.txt']:
        add(WORK / 'qa-tools-final' / name, Path('current/qa-tools-final') / name, 'current_11_input_qa_tool_pure_checks')
    add(WORK / 'update_docs-requirements.json', 'templates/update_docs-requirements.json', 'reviewed_report_document_contract')
    add(client_path, 'current/client/report.json', 'current_explicitly_selected_client_validation')
    for item in read(client_path)['commands']:
        source = project_path(item['log'])
        add(source, Path('current/client') / source.name, 'current_explicitly_selected_client_validation')
    for name in ['validation-3', 'validation-4']:
        old = WORK / 'client' / name
        add_small_tree(old, Path('history/client') / name, 'historical_before_audio_or_destroy_fix_not_current')
    for name in ['material-journal-fix', 'audio-input-fix', 'audio-destroy-fix']:
        add_small_tree(WORK / 'client' / name, Path('history/client') / name, 'actual_product_regression_red_green_not_gpu_acceptance')
    for name in ['manual-diagnostics-tests', 'editor-cache-tests', 'editor-information']:
        add_small_tree(WORK / 'client-peer' / name, Path('history/client-peer') / name, 'tool_or_editor_contract_evidence')
    old_creator = WORK / 'creator-before-material-journal'
    if old_creator.is_dir():
        add_small_tree(old_creator, Path('history/creator-before-material-journal'), 'historical_creator_before_material_fix')
    for run_id, scope in [('a2e19b4968b142078cd7c78f3ab7829d', 'actual_audio_unlock_failure'), ('422a443fc2e84cd4a240ed9e2a19b13d', 'actual_lifecycle_cleanup_failure'), ('425e92f75bf946afb173ef23be024c21', 'editor_questionnaire_cache_raw_guard_failure')]:
        directory = WORK / 'gui' / ('run-' + run_id)
        for name in ['report.json', 'creator-report.json', 'project-copy.json', 'creator.log', 'main-restore.txt']:
            add(directory / name, Path('history/gui') / ('run-' + run_id) / name, scope)
        if (directory / 'browser').is_dir():
            for name in ['runtime.json', 'static-runtime.json', 'qualification.json', 'browser-output.txt']:
                add(directory / name, Path('history/gui') / ('run-' + run_id) / name, scope)
            add_small_tree(directory / 'browser', Path('history/gui') / ('run-' + run_id) / 'browser', scope)
    quarantine = ROOT / '.work/quarantine/creator-information-20261029/information.json'
    add(quarantine, 'history/editor-cache-quarantine/information.json', 'original_generated_editor_form_metadata_not_authorization_or_product_configuration')
    for source in normal_base.iterdir():
        if source.is_file() and source.suffix in {'.json', '.txt'}:
            add(source, Path('current/normal-browsers') / source.name, 'current_main_normal_touch_selector_and_identity')
    for report_path in [town_path, ui_path]:
        for name in ['stdout.txt', 'stderr.txt']:
            add(report_path.parent / name, Path('current/normal-browsers') / report_path.parent.name / name, 'current_main_normal_touch_raw_logs')
    # 这次正常UI材料接线失败不能被后续成功抹去，也不能降成单纯工具超时。
    material_failure = WORK / 'gui/run-ca7a3ea669684bfbbcb55edbda6d8033'
    for name in ['report.json', 'runtime.json', 'static-runtime.json', 'qualification.json', 'reused-artifact.json', 'browser-output.txt', 'main-restore.txt', 'browser/report.json', 'browser/01-normal-new-role.png', 'browser/failure.png']:
        add(material_failure / name, Path('history/material-wiring-failure') / name, 'actual_product_material_pending_payload_wiring_failure')
    return jobs, skipped, references


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--gui-run', required=True, type=Path)
    parser.add_argument('--client-report', required=True, type=Path, help='显式选择修复后当前client验证；旧validation-2不得冒充新源码')
    parser.add_argument('--town-report', required=True, type=Path)
    parser.add_argument('--ui-report', required=True, type=Path)
    parser.add_argument('--check', action='store_true', help='只完成全部preflight，不创建outputs或正式docs报告')
    args = parser.parse_args()
    # 写任何outputs/docs之前完成全部终态、当前摘要和归档白名单门禁。
    need(not OUT.exists() and not FORMAL.exists(), '正式证据或report已存在，拒绝覆盖；由Root先审视已有产物')
    fixture = read(ROOT / 'config/development/first-loop.json')
    candidate = read(ROOT / 'config/development/summon-candidates.json')
    pets = read(ROOT / 'config/development/pet-runtime-candidates.json')
    need(fixture['rule_version'] == 18 and len(fixture['skills']) == 22 and all(skill['id'] != 17 for skill in fixture['skills']), '主profile不是rule18/22技能/17关闭')
    need(fixture['casting_supply']['enabled'] is False and fixture['pet_runtime']['bootstrap_mode'] == 'legacy_untrained_pet', '主材料/legacy政策变化')
    need(candidate['runtime_enabled'] is False and candidate['required_rule_version'] == 17 and pets['runtime_enabled'] is False, '独立17目录被激活或最低合同改变')
    need(all(row['item_id'] != 25 for row in fixture['initial_skill_books']) and next(item for item in fixture['items'] if item['id'] == 187)['max_stack'] == 0, '主赠书或未定符包规格被改变')
    go, gc = go_evidence()
    client_path, client, app, scene, py_count, pb, protocol, engine_count = client_evidence(args.client_report)
    qa_tools, qa_counts = qa_tools_evidence()
    build = main_creator()
    counts, post, manifest, apply = assets_evidence()
    gui_dir = project_path(args.gui_run)
    need(gui_dir.is_relative_to(WORK), '--gui-run必须指本轮.work阶段目录')
    parent, runtime, gui, isolated, artifact, gpu = gui_evidence(gui_dir, fixture)
    town_path, town = normal_browser(args.town_report, 'town', ROOT / 'docs/客户端构建检查.json')
    ui_path, ui = normal_browser(args.ui_report, 'ui', ROOT / 'docs/客户端构建检查.json')
    normal_base, normal_summary = normal_selector(town_path, ui_path, gui_dir)
    service_snapshots = final_service_snapshots(normal_summary, normal_base)
    absence = read(WORK / 'gui-invalid-name-followup.json')
    need(absence['passed'] and absence['read_only'] and absence['database_exists'] is False and absence['database_dropped'] is False and absence['database_or_user_data_modified'] is False and absence['original_failure_report_preserved'], 'DB65失败缺只读不存在后续证明')
    need(sha(project_path(absence['original_failed_report'])) == absence['original_report_sha256'], '原DB65失败报告被改写')
    previous_go = read(WORK / 'go-full-attempt1/latest.json')
    directed = read(WORK / 'displacement-review/focused-report.json')
    previous_main = read(WORK / 'creator-before-quiescing/report.json')
    previous_gui = read(WORK / 'gui/run-6d49a00ee9514431b6ffa6bf30846cd8/report.json')
    material_failure_parent = read(WORK / 'gui/run-ca7a3ea669684bfbbcb55edbda6d8033/report.json')
    material_failure_child = read(WORK / 'gui/run-ca7a3ea669684bfbbcb55edbda6d8033/browser/report.json')
    later_failures = later_failed_runs()
    need(previous_go['passed'] is False and previous_go['exit_code'] != 0, '首轮Go历史失败被改写')
    need(directed['passed'] and directed['mongo_e2e_explicit'] and directed['race_detector'] and directed['source_stable'] and directed['exit_code'] == 0, '真实因果专项未通过')
    need(previous_gui['passed'] is False and previous_gui['creator']['passed'] is True, '首轮独立构建/无GUI失败边界被改写')
    need(material_failure_parent['passed'] is False and material_failure_child['passed'] is False and material_failure_parent['browser_exit_code'] != 0, '实际材料接线失败被改写')
    need(material_failure_child['casts'] == [] and material_failure_child['born'] == [] and not any(item['id'] == 40014 for item in material_failure_child['sent']), '材料接线失败不再是原始发送前边界')
    config = read(ROOT / 'config/reports/配置校验报告.json')
    preview = read(ROOT / 'config/releases/preview_current.json')
    need(config['structure_valid'] and config['errors'] == [] and config['runtime_ready'] is False and preview['runtime_ready'] is False, '当前配置预览未通过或错误宣称runtime ready')
    schema = read(ROOT / 'config/schema/catalog.json')
    for workbook in schema['workbooks']:
        need(config['workbook_hashes'][workbook['group']] == sha(ROOT / workbook['path']), '配置报告不是当前唯一Excel')
    release = project_path(preview['path'], ROOT / 'config/releases')
    release_manifest = read(release / 'manifest.json')
    need(release_manifest['version'] == preview['version'] and release_manifest['content_sha256'] == preview['content_sha256'] and release_manifest['schema_sha256'] == sha(ROOT / 'config/schema/catalog.json'), 'preview/schema身份不同')
    current(release_manifest['files'], release, '当前preview文件')
    need(fixture['_source_evidence']['source_locks'] == schema['source_locks'] and fixture['_source_evidence']['workbook']['sha256'] == config['workbook_hashes']['gameplay'], 'fixture source evidence不是当前schema/book')
    jobs, skipped, references = curate_jobs(gui_dir, artifact, gpu, town_path, ui_path, client_path, normal_base)
    report = {'schema_version': 1, 'date': '2026-10-29', 'recorded_at_utc': datetime.now(timezone.utc).isoformat(), 'stage': '宠物新生原帧/原声与隔离正常触屏表现', 'status': 'limited_passed',
        'scope': '主rule18仍22技能/legacy不训练/材料关闭/17closed；只验隔离资格下正常学习、装符、新召、Recall和健康重观；不代表正常购符包或完整P0',
        'runtime': {'rule_version': fixture['rule_version'], 'preview': preview['version'], 'fixture_sha256': sha(ROOT / 'config/development/first-loop.json'), 'skills': len(fixture['skills']), 'summon17_enabled': False, 'casting_supply_enabled': fixture['casting_supply']['enabled'], 'bootstrap_mode': fixture['pet_runtime']['bootstrap_mode'], 'legacy_pet_trains': False, 'summon_candidate_runtime_enabled': candidate['runtime_enabled'], 'summon_candidate_required_rule_version': candidate['required_rule_version'], 'pet_candidate_schema_version': pets['schema_version'], 'table_count': config['table_count'], 'record_count': config['record_count'], 'release_blockers': len(config['release_blockers']), 'runtime_ready': False, 'production_ready': False},
        'verification': {'go_full_mongo_ws_race_passed_including_subtests': gc['passed'], 'go_skipped': gc['skipped'], 'go_failed': gc['failed'], 'go_inputs': gc['inputs'], 'go_source_stable': True, 'configuration_python_passed': py_count, 'javascript_ui_app_passed': app['pass'], 'javascript_scene_passed': scene['pass'], 'javascript_total_passed': app['pass'] + scene['pass'], 'client_inputs': client['input_count'], 'client_external_engine_inputs': engine_count, 'pb_messages': pb['message_count'], 'pb_long_fields': pb['int64_field_count'], 'pb_boundary_cases': pb['int64_boundary_cases'], 'pb_protocol_ids': protocol['messages'], 'creator_seconds': build['seconds'], 'creator_exit_code': build['exit_code'], 'saved_layouts_preserved': counts['saved_layout'], 'source_town_artifact_files_unchanged': counts['source_town_files'], 'source_added_png_frames': post['new_pngs'], 'source_added_wav_files': post['new_wavs'], 'source_creator_imported_meta_files': post['new_meta_count'], 'previous_development_assets_preserved': True, 'gameplay_excel_changed_cells': len(apply['cell_differences']), 'only_profile_rule_17_to_18': True, 'architecture_workbook_preserved': True, 'qa_tool_python_passed': qa_counts['python'], 'qa_tool_node_passed': qa_counts['node'], 'qa_tool_inputs': len(qa_tools['after'])},
        'main_creator': {'passed': build['success'], 'seconds': build['seconds'], 'exit_code': build['exit_code'], 'source_stable': build['source_stable_during_build'], 'layout_preserved': build['scene_prefab_layout_preserved']},
        'isolated_creator': {'passed': isolated['passed'], 'seconds': isolated['seconds'], 'exit_code': isolated['exit_code'], 'source_stable': isolated['source_stable'], 'artifact_directory': str(artifact), 'reused_for_gui': runtime['artifact_reused'], 'seconds_scope': '原真实Creator构建耗时，不是本次GUI运行耗时', 'excluded_generated_editor_caches': isolated['excluded_generated_editor_caches'], 'generated_editor_cache_sha256': isolated['generated_editor_cache_sha256']},
        'gui': {key: gui[key] for key in ['passed', 'checks', 'errors', 'warnings', 'privateCastInvoked', 'routeOverride', 'bodySeeded']} | {'run_id': runtime['run_id'], 'qa_database': runtime['database'], 'main_restored': parent['main_restored'], 'qa_stopped': parent['qa_stopped'], 'static_stopped': parent['static_stopped'], 'qa_database_dropped': parent['qa_database_dropped'], 'gpu_screenshots': len(gui['gpu']), 'scope': gui['scope']},
        'normal_main_browsers': {'town': {'passed': town['passed'], 'checks': len(town['checks']), 'errors': len(town['errors']), 'warnings': len(town['warnings']), 'source_report': town_path.relative_to(ROOT).as_posix(), 'sha256': sha(town_path)}, 'ui': {'passed': ui['passed'], 'checks': len(ui['checks']), 'errors': len(ui['errors']), 'warnings': len(ui['warnings']), 'source_report': ui_path.relative_to(ROOT).as_posix(), 'sha256': sha(ui_path)}},
        'artifact_preservation': counts | {'new_pngs': post['new_pngs'], 'new_wavs': post['new_wavs'], 'new_creator_meta': post['new_meta_count']},
        'historical_scopes': ['旧整包与抗拒8前态失败和当前Go通过分开；旧日志不能追溯证明伤害来源，新定向样本才有普通挥刀及施法前后HP配对因果', 'client validation1/validation2与quiescing红绿保留原时点，所选修复后client五命令输入另列', '旧主Creator与早期独立构建成功/无GUI失败不当current GUI；被严格复用的当前实际构建单列原耗时', '超限库名原失败不改写，另有只读不存在核验，不执行不存在库删除', '正常Create/qualify/learn/equip之后，Quick材料未决payload严格验证挡住40014的实际产品接线失败单独保留；子工具终态虽为等GPU超时，不能把实际接线缺陷降为工具超时'],
        'historical_verification': {'full_go_attempt1': {'passed': previous_go['passed'], 'test_passed': len(previous_go['passed_tests']), 'failed_test_or_package_records': len(previous_go['failed_tests']), 'test_skipped': len(previous_go['skipped_tests']), 'skill8_old_damage_attribution': '不能由旧断言追溯证明，保留待证边界'}, 'directed_causal_mongo_race': {'passed': directed['passed'], 'seconds': directed['test_seconds'], 'methods_passed': len(directed['passed_tests']), 'scope': '实际新样本，不逆推旧失败'}, 'old_main_creator': {'seconds': previous_main['seconds'], 'success': previous_main['success'], 'scope': 'quiescing修复前历史包'}, 'old_independent_creator_without_gui': {'seconds': previous_gui['creator']['seconds'], 'creator_passed': previous_gui['creator']['passed'], 'parent_passed': previous_gui['passed'], 'scope': '超限库名insert marker失败，未运行GUI'}, 'invalid_database_readonly_followup': absence},
        'client_validation_selector': {'report': client_path.relative_to(ROOT).as_posix(), 'sha256': sha(client_path)},
        'later_failed_gui_attempts': later_failures,
        'normal_main_selector': {'directory': normal_base.relative_to(ROOT).as_posix(), 'protected_input_count': len(read(normal_base / 'inputs-after.json')), 'passed': normal_summary['passed']},
        'final_service_snapshots': {'scope': '只核已有受控状态命令的原始输出；归档器不查询数据库或改变服务', 'main_pid': service_snapshots['main-status']['pid'], 'main_run_id': service_snapshots['main-status']['run_id'], 'mongodb_pid': service_snapshots['mongodb-status']['pid'], 'mongodb_version': service_snapshots['mongodb-status']['version'], 'mongodb_port': service_snapshots['mongodb-status']['port'], 'static_preview_running': service_snapshots['client-status']['running']},
        'editor_cache_contract': {'excluded_generated_editor_caches': EDITOR_CACHES, 'scope': '仅生成的编辑器问卷服务URL缓存；非授权旁路，实际splash和其它settings仍全树严核；旧raw失败不改写'},
        'limits': ['主17/购符仍关闭，护符Count包规格与售价未定；隔离19级/原书25/两Count不能称自然供给', '真实AudioSource.playing及GPU原帧不等于硬件听感或手机真机', '不承诺128容量、尸体重召完整UI、自然养成、完整三职业/P0或全1.80认证'], 'physical_mobile_verified': False, 'hardware_audio_perception_verified': False, 'capacity_verified': False, 'production_ready': False}
    # schema与文档更新器使用同一必填字段清单；缺项不补零或空数组。
    requirements = read(WORK / 'update_docs-requirements.json')
    need(sha(WORK / 'update_docs.py') == requirements['script_sha256'], '文档更新器与已审字段合同不同')
    # 只核已存在的分组清单，清单结构由Root/rules审阅；业务字段本身在上面逐项显式构造。
    need(report['status'] == requirements['status'], '正式report状态与文档合同不同')
    for group, requested in requirements['required_fields'].items():
        values = report if group == 'root' else report[group]
        need(all(key in values and values[key] is not None for key in requested), '正式report缺文档必需字段：' + group)
    # 最后一次校验所有将复制的源字节；至此之前没有创建outputs或正式report。
    for item in jobs.values(): need(sha(ROOT / item['source']) == item['sha256'], '归档前源文件变化')
    current(go['source_sha256'], label='写入前Go')
    current(client['current'], label='写入前client')
    need(client_snapshot() == client['current'], '写入前client源码集合变化')
    engine_evidence(client)
    if args.check:
        print(json.dumps({'preflight': 'passed', 'archive_executed': False, 'formal_report_written': False, 'status_when_applied': report['status'], 'go': gc['passed'], 'go_inputs': gc['inputs'], 'javascript': app['pass'] + scene['pass'], 'client_repo_inputs': client['input_count'], 'client_external_engine_inputs': engine_count, 'python': py_count, 'gui_checks': len(gui['checks']), 'gui_warnings': len(gui['warnings']), 'explicit_archive_jobs': len(jobs), 'gui_run': runtime['run_id']}, ensure_ascii=False))
        return
    OUT.mkdir(parents=True, exist_ok=False)
    mapping = {}
    for item in jobs.values():
        target = OUT / item['target']
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(ROOT / item['source'], target)
        need(sha(target) == item['sha256'], '归档副本不等于原字节')
        mapping[item['source']] = target.relative_to(ROOT).as_posix()
    write_new(OUT / 'evidence-paths.json', mapping)
    write_new(OUT / 'source-reference-hashes.json', references)
    write_new(OUT / 'evidence-manifest.json', {'scope': '显式白名单；原报告内部路径/SHA不改写', 'files': list(jobs.values()), 'plan_snapshot_not_reused_as_current': skipped, 'go_raw_counts': gc})
    with (OUT / 'README.md').open('x', encoding='utf-8', newline='\n') as stream:
        stream.write('# 宠物出生表现限定证据\n\n主rule18仍22技能、legacy不训练、材料关闭、17未开放；当前隔离GUI与正常主Town/UI分别记录。current只选终态通过且当前SHA匹配的证据；curated保留历史前态、失败、来源研究和红绿因果。\n\n未复制client-project、library、temp、build、node_modules、nbserver.exe、整套旧inputs或原图/音频包。GPU截图来自实际AFTER_DRAW，原解码接触表单独归类。原Creator构建耗时与复用后的QA运行分开；没有以生成成功代替业务验收，也不承诺听感、真机、容量、完整P0或全1.80。\n\n产品项目输入与独立安装引擎源码分成两组冻结，各自before/after/current和当前SHA严格相等。Creator受控输入树只排settings/v2/packages/information.json生成问卷缓存，其新原始SHA另记；实际splash和其它settings全严核，build目录无排除。旧raw稳定guard失败原样保留，不因新缓存合同追溯改成成功。\n')
    archive_files = [OUT / item['target'] for item in jobs.values()] + [OUT / name for name in ['evidence-paths.json', 'source-reference-hashes.json', 'evidence-manifest.json', 'README.md']]
    report['evidence_sha256'] = {path.relative_to(ROOT).as_posix(): sha(path) for path in archive_files}
    report['evidence_file_count'] = len(archive_files)
    # 复制只读证据后仍不得沿漂移输入写成功；保留partial outputs供Root检查，不自动删除。
    current(go['source_sha256'], label='写入后Go')
    current(client['current'], label='写入后client')
    need(client_snapshot() == client['current'], '写入后client源码集合变化')
    engine_evidence(client)
    current(parent['tools_after'], label='写入后GUI工具')
    current(qa_tools['after'], label='写入后QA纯测工具')
    write_new(FORMAL, report)
    print(json.dumps({'status': report['status'], 'go': gc['passed'], 'go_inputs': gc['inputs'], 'javascript': app['pass'] + scene['pass'], 'python': py_count, 'gui_checks': len(gui['checks']), 'evidence_files': len(archive_files), 'report': str(FORMAL)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
