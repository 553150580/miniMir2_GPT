"""只读核对阶段20归档和Git index原始blob；不暂存、不提交、不改正式报告。"""
from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import runpy
import subprocess

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
OUT = 'outputs/level-growth-20261029'
FORMAL = 'docs/自然成长与待用经验阶段验收.json'
BOOK = 'outputs/development-prep-20261026/玩法配置.xlsx'
ARCH = 'outputs/development-prep-20261026/架构配置.xlsx'
ENGINE = Path('D:/Program Files/CocosCreator/3.8.8/resources/resources/3d/engine/cocos').resolve()
ENGINE_FILES = ['input/input.ts', '2d/event/pointer-event-dispatcher.ts', 'scene-graph/node-event-processor.ts',
                'scene-graph/node.ts', 'ui/button.ts', 'core/data/object.ts', 'scene-graph/component.ts',
                'audio/audio-source.ts', 'ui/toggle.ts']
FONT_FILES = [Path('C:/Windows/Fonts/arial.ttf'), Path('C:/Windows/Fonts/msyh.ttc'), Path('C:/Windows/Fonts/simsun.ttc')]

# 仅本阶段已授权源码、生成物与滚动说明；证据目录和五个preview文件另按正式清单加入。
ALLOWED = set('''
.gitattributes
.gitignore
README.md
client/README.md
config/README.md
shared/proto/README.md
client/assets/resources/development/gameplay.json
client/assets/scripts/app/AppRoot.ts
client/assets/scripts/app/AutoRecycleModel.ts
client/assets/scripts/app/GameCombatStatus.ts
client/assets/scripts/app/GameConnection.ts
client/assets/scripts/app/GameMessages.ts
client/assets/scripts/app/GameSession.ts
client/assets/scripts/app/GameSkillAssets.ts
client/assets/scripts/app/SkillAssetsModel.ts
client/assets/scripts/app/HudModel.ts
client/assets/scripts/app/SystemMessageLog.ts
client/assets/scripts/app/LevelGrowthModel.ts
client/assets/scripts/app/LevelGrowthModel.ts.meta
client/assets/scripts/generated/config_types.ts
client/assets/scripts/generated/protobuf/client_pb.d.ts
client/assets/scripts/generated/protobuf/client_pb.js
client/assets/scripts/generated/protocol_ids.ts
client/tests/app/auto_recycle_runtime.test.cjs
client/tests/app/combat_session.test.cjs
client/tests/app/combat_status_ui.test.cjs
client/tests/app/helpers/session_harness.cjs
client/tests/app/toggle_lifecycle.test.cjs
client/tests/app/level_growth.test.cjs
client/tests/app/skill_page_merge.test.cjs
client/tests/app/skill_page_merge_runtime.test.cjs
client/tests/ui/compile_models.mjs
conf/toolchain/client-pb-verification.json
conf/toolchain/config-binding-sync.json
conf/toolchain/go-pb-generation.json
config/development/first-loop.json
config/development/pet-runtime-candidates.json
config/development/summon-candidates.json
config/releases/preview_current.json
config/reports/配置校验报告.json
config/schema/catalog.json
docs/客户端构建检查.json
docs/开发阶段与验收清单.md
docs/正式开发推进记录.md
docs/架构适配与规则优先级.md
docs/自然成长与待用经验实施边界.md
docs/自然成长与待用经验阶段验收.json
outputs/development-prep-20261026/玩法配置.xlsx
server/api/generated/minimirv1/assets.pb.go
server/internal/configtypes/config_types.go
server/internal/devconfig/config.go
server/internal/devconfig/map_catalog_test.go
server/internal/devconfig/pet_runtime_test.go
server/internal/devconfig/summon_test.go
server/internal/devconfig/progression.go
server/internal/devconfig/progression_test.go
server/internal/domain/model.go
server/internal/domain/progression.go
server/internal/domain/progression_test.go
server/internal/game/combat.go
server/internal/game/pet_training.go
server/internal/game/runtime.go
server/internal/game/timeline.go
server/internal/game/progression.go
server/internal/game/progression_test.go
server/internal/integration/binding_test.go
server/internal/integration/summon_pet_test.go
server/internal/integration/level_growth_test.go
server/internal/integration/skill_page_merge_test.go
server/internal/persistence/assets.go
server/internal/persistence/auth.go
server/internal/persistence/settlement.go
server/internal/persistence/store.go
server/internal/persistence/progression.go
server/internal/persistence/progression_test.go
server/internal/persistence/skill_assets.go
server/internal/persistence/skill_page_merge_test.go
server/internal/protocol/protocol_ids.go
server/test-results/latest.events.jsonl
server/test-results/latest.json
shared/proto/assets.proto
shared/proto/protocol_manifest.json
tools/client/verify_level_growth_browser.cjs
tools/config/test_equipment_recycling.py
tools/config/test_pet_runtime.py
tools/config/test_safe_zone_binding.py
tools/config/test_summon_candidates.py
tools/config/test_teleport.py
tools/config/test_level_growth.py
tools/resources/prepare_first_loop.py
tools/resources/prepare_summon_candidates.py
'''.split())


def need(value, message):
    if not value:
        raise ValueError(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def sha(path):
    result = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b''):
            result.update(block)
    return result.hexdigest()


def repo(name):
    need(isinstance(name, str) and '\\' not in name, '必须是POSIX仓库相对路径：' + str(name))
    rel = PurePosixPath(name)
    need(name and not rel.is_absolute() and '..' not in rel.parts and ':' not in name, '路径越界：' + name)
    path = ROOT.joinpath(*rel.parts)
    need(path.resolve().is_relative_to(ROOT), '路径越界：' + name)
    for ancestor in [path, *path.parents]:
        if ancestor == ROOT:
            break
        need(not ancestor.is_symlink() and not getattr(ancestor, 'is_junction', lambda: False)(), '拒绝链接：' + name)
    return path


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT, env={**os.environ, 'GIT_OPTIONAL_LOCKS': '0'})


def index_entries():
    values = {}
    for row in git('ls-files', '--stage', '-z').split(b'\0'):
        if not row:
            continue
        metadata, name = row.split(b'\t', 1)
        mode, oid, stage = metadata.split()
        name = name.decode('utf-8')
        need(stage == b'0' and mode in {b'100644', b'100755'} and name not in values, 'index有冲突/非普通/重复文件：' + name)
        values[name] = (mode, oid)
    return values


def head_entries():
    values = {}
    for row in git('ls-tree', '-r', '-z', 'HEAD').split(b'\0'):
        if row:
            metadata, name = row.split(b'\t', 1)
            mode, kind, oid = metadata.split()
            if kind == b'blob':
                values[name.decode('utf-8')] = (mode, oid)
    return values


def staged_paths():
    parts = git('diff', '--cached', '--no-renames', '--name-status', '-z').split(b'\0')
    need(parts[-1] == b'' and len(parts[:-1]) % 2 == 0, 'Git状态缺NUL/形状错误')
    return [(parts[i].decode('ascii'), parts[i + 1].decode('utf-8')) for i in range(0, len(parts) - 1, 2)]


class Blobs:
    """直接读取原blob；逐个流式散列，避免把整批图像/历史日志拼入内存。"""
    def __init__(self):
        self.process = subprocess.Popen(['git', 'cat-file', '--batch'], cwd=ROOT, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                        stderr=subprocess.PIPE, env={**os.environ, 'GIT_OPTIONAL_LOCKS': '0'})
        self.hashes = {}

    def content(self, oid, collect=True):
        if not collect and oid in self.hashes:
            return self.hashes[oid]
        self.process.stdin.write(oid + b'\n'); self.process.stdin.flush()
        header = self.process.stdout.readline().rstrip(b'\n').split()
        need(len(header) == 3 and header[0] == oid and header[1] == b'blob', 'Git未返回预期原blob')
        remaining = int(header[2]); result = hashlib.sha256(); pieces = []
        while remaining:
            block = self.process.stdout.read(min(1024 * 1024, remaining))
            need(block, 'Git blob截断')
            remaining -= len(block); result.update(block)
            if collect:
                pieces.append(block)
        need(self.process.stdout.read(1) == b'\n', 'Git blob终结错误')
        self.hashes[oid] = result.hexdigest()
        return b''.join(pieces) if collect else self.hashes[oid]

    def close(self):
        self.process.stdin.close()
        need(self.process.wait(timeout=10) == 0, 'git cat-file未正常退出')


def client_snapshot():
    files = [p for folder in ['client/assets/scripts', 'client/tests'] for p in (ROOT / folder).rglob('*') if p.is_file()]
    files += [p for pattern in ['*.scene', '*.prefab'] for p in (ROOT / 'client/assets').rglob(pattern)]
    files += [ROOT / p for p in ['client/assets/resources/development/gameplay.json', 'client/assets/resources/development/manifest.json', 'config/development/first-loop.json']]
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(set(files))}


def external(client, prefix, allowed):
    before, after, saved = [client[prefix + k] for k in ['_before', '_after', '_current']]
    normalized = lambda values: {os.path.normcase(str(Path(p).resolve())): h for p, h in values.items()}
    need(before == after == saved and set(normalized(saved)) == {os.path.normcase(str(p.resolve())) for p in allowed}, prefix + '冻结/白名单不相同')
    for name, value in saved.items():
        path = Path(name).resolve()
        need(not path.is_relative_to(ROOT) and path.is_file() and not path.is_symlink() and sha(path) == value, prefix + '本地字节不同：' + name)
    return len(saved)


def node_count(raw):
    values = {k: int(v) for k, v in re.findall(r'^(?:#|ℹ)\s+(tests|pass|fail|skipped|cancelled|todo)\s+(\d+)\s*$', raw.decode('utf-8'), re.M)}
    need(set(values) == {'tests', 'pass', 'fail', 'skipped', 'cancelled', 'todo'} and values['tests'] == values['pass'] > 0 and
         all(values[k] == 0 for k in ['fail', 'skipped', 'cancelled', 'todo']), 'Node raw终态不是全通过')
    return values['pass']


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--report', type=Path, help='可选：只把核验摘要新写入本阶段.work；默认仅stdout')
    args = parser.parse_args()
    head = git('rev-parse', 'HEAD').strip()
    indexed, heads, changes = index_entries(), head_entries(), staged_paths()
    need(changes, '阶段尚未暂存；不提前宣称index验收')
    blobs = Blobs()
    try:
        expected = {}

        def add(name, value):
            repo(name)
            need(isinstance(value, str) and re.fullmatch('[0-9a-f]{64}', value), '非法SHA：' + name)
            need(name not in expected or expected[name] == value, '摘要集合冲突：' + name)
            expected[name] = value

        def index_bytes(name):
            repo(name); need(name in indexed, '应核验文件尚未进index：' + name)
            return blobs.content(indexed[name][1])

        stage_bytes = index_bytes(FORMAL); stage = json.loads(stage_bytes); add(FORMAL, digest(stage_bytes))
        need(stage['status'] == 'limited_passed' and stage['runtime']['rule_version'] == 20 and stage['production_ready'] is False, '不是阶段20限定验收')
        for name, value in stage['evidence_sha256'].items():
            need(name.startswith(OUT + '/'), '正式证据越过本阶段目录：' + name); add(name, value)
        need(stage['evidence_file_count'] == len(stage['evidence_sha256']), '正式证据数量不同')
        manifest = json.loads(index_bytes(OUT + '/evidence-manifest.json'))
        paths = json.loads(index_bytes(OUT + '/evidence-paths.json'))
        records = {}; targets = set(); gzip_count = 0

        def archive_raw(record):
            data = index_bytes(OUT + '/' + record['target'])
            need(len(data) == record['file_bytes'] and digest(data) == record['file_sha256'], 'index归档file SHA/bytes不同：' + record['target'])
            need(record['encoding'] in {'gzip', 'identity'}, '未知归档编码')
            if record['encoding'] == 'gzip':
                # 从已读取的index压缩blob解码，工作树gzip不能替代该证明。
                with gzip.GzipFile(fileobj=io.BytesIO(data), mode='rb') as stream:
                    raw = stream.read()
            else:
                raw = data
            need(len(raw) == record['raw_bytes'] and digest(raw) == record['raw_sha256'], 'index归档raw SHA/bytes不同：' + record['target'])
            return raw

        for record in manifest['files']:
            name = OUT + '/' + record['target']; repo(name); repo(record['source'])
            need(name not in targets and record['source'] not in records, '归档源/目标重复')
            need(stage['evidence_sha256'].get(name) == record['file_sha256'], '正式证据与manifest不同')
            archive_raw(record)
            original = repo(record['source'])
            need(original.is_file() and sha(original) == record['raw_sha256'], '归档原源字节变化：' + record['source'])
            need(paths.get(record['source']) == {'path': name, 'encoding': record['encoding'], 'raw_sha256': record['raw_sha256'], 'file_sha256': record['file_sha256']}, 'paths映射不匹配')
            targets.add(name); records[record['source']] = record; gzip_count += record['encoding'] == 'gzip'
        extras = {OUT + '/' + n for n in ['README.md', 'evidence-manifest.json', 'evidence-paths.json']}
        need(set(stage['evidence_sha256']) == targets | extras and set(paths) == set(records), '证据manifest不是完整集合')
        need({n for n in indexed if n.startswith(OUT + '/')} == targets | extras, 'index证据目录存在多余/缺失文件')
        need({p.relative_to(ROOT).as_posix() for p in repo(OUT).rglob('*') if p.is_file()} == targets | extras, '工作树证据目录存在多余/缺失文件')

        def source_raw(name):
            need(name in records, '最终原始证据未被白名单归档：' + name)
            return archive_raw(records[name])

        def selected(key):
            selector = stage['selectors'][key]
            raw = source_raw(selector['path'])
            need(digest(raw) == selector['sha256'], 'selector与index归档原字节不同：' + key)
            return raw

        go_raw, events_raw, client_raw, build_raw = [selected(k) for k in ['go_report', 'go_events', 'client_report', 'creator_report']]
        go, client, build = map(json.loads, [go_raw, client_raw, build_raw])
        for name, raw in [('server/test-results/latest.json', go_raw), ('server/test-results/latest.events.jsonl', events_raw), ('docs/客户端构建检查.json', build_raw)]:
            add(name, digest(raw))
        need(go['passed'] is True and go['mongo_e2e_explicit'] is True and go['race_detector'] is True and go['exit_code'] == 0 and
             go['source_changed_during_test'] is False and go['source_sha256'] == go['source_sha256_after'], 'Go未真实Mongo/race稳定通过')
        need(all(a in go['command'] for a in ['./...', '-json', '-race']), 'Go不是完整全包JSON/race')
        definition = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='readonly_input_enumeration')
        need({p.relative_to(ROOT).as_posix() for p in definition['input_files']()} == set(go['source_sha256']), 'Go输入集合不完整')
        need(digest(events_raw) == go['events_sha256'], 'Go raw事件不匹配')
        events = [json.loads(line) for line in events_raw.splitlines() if line.strip()]
        passed = [{'package': e['Package'], 'test': e['Test']} for e in events if e.get('Action') == 'pass' and e.get('Test')]
        skipped = [{'package': e['Package'], 'test': e['Test']} for e in events if e.get('Action') == 'skip' and e.get('Test')]
        failed = [{'package': e['Package'], 'test': e.get('Test')} for e in events if e.get('Action') in {'fail', 'build-fail'}]
        need(passed == go['passed_tests'] and skipped == go['skipped_tests'] == [] and failed == go['failed_tests'] == [], 'Go raw节点与终态不同')
        v = stage['verification']
        need(len(passed) == v['go_full_mongo_ws_race_passed_including_subtests'] and len(go['source_sha256']) == v['go_inputs'], '正式Go计数不匹配')
        need(client['passed'] is True and client['stable'] is True and client['freeze_started_before_commands'] is True and
             client['before'] == client['after'] == client['current'] == client_snapshot() and client['input_count'] == len(client['current']), 'client完整当前冻结不匹配')
        commands = {c['name']: c for c in client['commands']}
        need(len(commands) == len(client['commands']) == 6 and set(commands) == {'models', 'full-ui-app', 'scene', 'strict', 'protocol-check', 'protocol-lint'}, 'client组别不同')
        for c in commands.values():
            need(c['exit_code'] == 0 and digest(source_raw(c['log'])) == c['sha256'], 'client原始命令/日志不同')
        app, scene = [node_count(source_raw(commands[n]['log'])) for n in ['full-ui-app', 'scene']]
        need(app == v['javascript_ui_app_passed'] and scene == v['javascript_scene_passed'] and app + scene == v['javascript_total_passed'] and
             client['input_count'] == v['client_inputs'], 'client raw计数不同')
        engine_count = external(client, 'external_engine', [ENGINE / p for p in ENGINE_FILES])
        font_count = external(client, 'external_font', FONT_FILES)
        need(engine_count == v['client_external_engine_inputs'] and font_count == v['client_external_font_inputs'], '外部源计数不同')
        need(build['creator'] == '3.8.8' and build['platform'] == 'web-mobile' and build['exit_code'] == 36 and build['success'] is True and
             build['source_stable_during_build'] is True and build['scene_prefab_layout_preserved'] is True and build['seconds'] == stage['main_creator']['seconds'], 'Creator不是本阶段稳定真构建')
        for values in [go['source_sha256'], client['current'], build['asset_hashes'], build['script_hashes']]:
            for name, value in values.items():
                add(name, value)

        config = json.loads(source_raw('.work/level-growth-20261029/final-review.json'))
        baseline = json.loads(source_raw('.work/level-growth-20261029/baseline.json'))
        for name, value in config['current_hashes'].items():
            add(name, value)
        need(config['rule_version'] == 20 and config['python_tests'] == v['configuration_python_passed'] == 120 and
             config['balance_verified'] is False and config['natural_level_1_to_50_verified'] is False, '配置阶段/平衡口径不同')
        old_book = source_raw('.work/level-growth-20261029/before/' + BOOK)
        need(digest(old_book) == baseline['files'][BOOK] == blobs.content(heads[BOOK][1], collect=False), '受审旧Excel二进制不是HEAD源')
        add(ARCH, baseline['files'][ARCH])
        need(blobs.content(heads[ARCH][1], collect=False) == baseline['files'][ARCH], '架构书相对HEAD变化')

        # 本阶段所有旧tracked美术、地图、资源JSON、meta与十份布局均不应改变。
        # 玩法public是唯一明确例外；脚本源由测试/Creator集合证明，原meta仍保护。
        protected = {n for n in heads if n.startswith('client/assets/') and n != 'client/assets/resources/development/gameplay.json' and
                     (not n.startswith('client/assets/scripts/') or n.endswith('.meta'))}
        protected.add(ARCH)
        source = json.loads(source_raw('.work/level-growth-20261029/source-audit.json'))['source']
        protected.add(source['copy']); add(source['copy'], source['sha256'])
        layouts = {n for n in heads if n.startswith('client/assets/') and Path(n).suffix in {'.scene', '.prefab'}}
        need(layouts == set(baseline['saved_layouts']) == set(build['asset_hashes']) and len(layouts) == 10, '10布局集合不同')
        for name in layouts:
            need(build['asset_hashes'][name] == baseline['saved_layouts'][name], '布局不符合阶段前态：' + name)
        need(protected <= set(indexed), '原tracked资产被删除：' + str(sorted(protected - set(indexed))))
        for name in sorted(protected):
            need(indexed[name] == heads[name], '原tracked资产原blob或mode改变：' + name)
            add(name, blobs.content(heads[name][1], collect=False))

        preview = stage['runtime']['preview']
        need(preview.startswith('dev-level-growth-v0.20-') and '/' not in preview and '\\' not in preview, 'preview不属于本阶段')
        release_files = {'config/releases/' + preview + '/' + n for n in ['client.json', 'server.json', 'config_types.go', 'config_types.ts', 'manifest.json']}
        allowed = ALLOWED | set(stage['evidence_sha256']) | release_files
        need(all(status in {'A', 'M'} for status, name in changes), '本阶段不允许删除/重命名/类型变化：' + str(changes))
        need(not [n for _, n in changes if n not in allowed], '出现未经本阶段覆盖的staged路径：' + str([n for _, n in changes if n not in allowed]))
        for status, name in changes:
            need(repo(name).is_file(), 'staged工作树文件缺失：' + name)
            if status == 'M':
                need(indexed[name][0] == heads[name][0], 'mode变化：' + name)
            add(name, sha(repo(name)))
        need(set(expected) <= set(indexed), '冻结输入/证据尚未全部进index：' + str(sorted(set(expected) - set(indexed))))
        for name, value in expected.items():
            need(blobs.content(indexed[name][1], collect=False) == value, 'index原blob SHA与冻结值不同（含行尾）：' + name)
            need(repo(name).is_file() and sha(repo(name)) == value, '工作树与index/冻结值不同（含行尾）：' + name)
        fixture_name = 'config/development/first-loop.json'
        need(expected[fixture_name] == go['fixture']['sha256'] == stage['runtime']['fixture_sha256'], 'fixture不配对')
        need(expected['client/assets/resources/development/gameplay.json'] == stage['runtime']['public_sha256'], 'public不配对')
        for key, selector in stage['selectors'].items():
            need(digest(source_raw(selector['path'])) == selector['sha256'], 'selector与index归档不匹配：' + key)
        need(index_entries() == indexed and git('rev-parse', 'HEAD').strip() == head, '核验期间index/HEAD变化')
        summary = {'passed': True, 'read_only': True, 'head': head.decode(), 'script_sha256': sha(Path(__file__)),
                   'git_index_blob_sha256_verified': len(expected), 'unique_blob_objects_read': len(blobs.hashes),
                   'staged_paths': len(changes), 'deleted_paths': [], 'unexpected_staged_paths': [],
                   'evidence_files': len(stage['evidence_sha256']), 'gzip_index_file_and_raw_verified': gzip_count,
                   'go_full_real_mongo_race_passed': len(passed), 'go_inputs': len(go['source_sha256']), 'go_failed': 0, 'go_skipped': 0,
                   'client_inputs': client['input_count'], 'client_javascript_passed': app + scene,
                   'external_engine_local_only': engine_count, 'external_fonts_local_only': font_count,
                   'old_tracked_assets_preserved_against_head': len(protected) - 2, 'saved_layouts_preserved': len(layouts),
                   'gameplay_workbook_binary_verified': True, 'architecture_workbook_preserved': True,
                   'stage': FORMAL, 'go_report': stage['selectors']['go_report']['path'], 'client_report': stage['selectors']['client_report']['path'],
                   'index_manifest_sha256': digest(json.dumps({n: [m.decode(), o.decode()] for n, (m, o) in sorted(indexed.items())}, sort_keys=True).encode())}
        if args.report:
            p = args.report.resolve(); need(p.is_relative_to(WORK) and p.suffix == '.json' and not p.exists(), '摘要只能新建于本阶段.work/*.json')
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(json.dumps(summary, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
        print(json.dumps(summary, ensure_ascii=False))
    finally:
        blobs.close()


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, FileNotFoundError, subprocess.CalledProcessError) as error:
        print(json.dumps({'passed': False, 'read_only': True, 'error': str(error)}, ensure_ascii=False))
        raise SystemExit(1)
