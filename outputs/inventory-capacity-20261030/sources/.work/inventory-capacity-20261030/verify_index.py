"""只读核对阶段21暂存原blob、当前受测输入和原资源；不改正式验收结论。"""
from pathlib import Path
import argparse, gzip, hashlib, io, json, os, runpy, subprocess

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
BASE = '5b6be439c013d16c4ca0bf0372c294e47b29a141'
FORMAL = 'docs/普通满包与背包触屏阶段验收.json'
OUT = 'outputs/inventory-capacity-20261030'
ALLOWED = set('''
.gitattributes .gitignore README.md client/README.md config/README.md
client/assets/resources/development/gameplay.json client/assets/scenes/Game.scene
client/assets/scripts/app/AppRoot.ts client/assets/scripts/app/AutoRecycleModel.ts client/assets/scripts/app/GameErrors.ts
client/tests/app/auto_recycle.test.cjs client/tests/app/messages.test.cjs client/tests/app/inventory_layout.test.cjs
client/tests/app/inventory_context.test.cjs
client/tests/app/helpers/auto_recycle_layout.cjs client/tests/app/combat_status.test.cjs client/tests/app/hud_layout.test.cjs
conf/toolchain/config-binding-sync.json config/development/first-loop.json config/development/pet-runtime-candidates.json
config/development/summon-candidates.json config/releases/preview_current.json config/reports/配置校验报告.json
docs/架构适配与规则优先级.md docs/开发阶段与验收清单.md docs/正式开发推进记录.md docs/客户端构建检查.json
docs/普通满包与背包触屏实施边界.md docs/普通满包与背包触屏阶段验收.json
outputs/development-prep-20261026/玩法配置.xlsx
server/internal/devconfig/map_catalog_test.go server/internal/devconfig/pet_runtime_test.go server/internal/devconfig/summon_test.go
server/internal/game/combat.go server/internal/game/runtime.go server/internal/game/hosting_test.go
server/internal/game/map_travel_ack_test.go server/internal/game/full_bag_policy_test.go
server/internal/persistence/settlement.go server/internal/persistence/equipment_recycling_test.go server/internal/persistence/full_bag_policy_test.go
server/internal/integration/auto_recycle_test.go server/internal/integration/full_bag_policy_test.go
server/internal/integration/level_growth_test.go server/internal/integration/summon_pet_test.go server/internal/integration/ownership_test.go
server/internal/integration/full_bag_offline_test.go server/test-results/latest.json server/test-results/latest.events.jsonl
tools/config/test_equipment_recycling.py tools/config/test_level_growth.py tools/config/test_pet_runtime.py
tools/config/test_safe_zone_binding.py tools/config/test_summon_candidates.py tools/config/test_teleport.py
tools/resources/prepare_summon_candidates.py tools/client/verify_inventory_capacity_browser.cjs
'''.split())


def need(value, message):
    if not value:
        raise ValueError(message)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def sha(p):
    return digest(p.read_bytes())


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT, env={**os.environ, 'GIT_OPTIONAL_LOCKS': '0'})


def repo(name):
    p = ROOT / name
    need(isinstance(name, str) and '\\' not in name and ':' not in name and not Path(name).is_absolute() and '..' not in Path(name).parts and p.resolve().is_relative_to(ROOT), '越界路径：' + str(name))
    return p


class Blobs:
    def __init__(self):
        self.process = subprocess.Popen(['git', 'cat-file', '--batch'], cwd=ROOT, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.hashes = {}

    def content(self, oid, collect=False):
        if not collect and oid in self.hashes:
            return self.hashes[oid]
        self.process.stdin.write(oid + b'\n')
        self.process.stdin.flush()
        header = self.process.stdout.readline().rstrip(b'\n').split()
        need(len(header) == 3 and header[0] == oid and header[1] == b'blob', '不是所需原blob')
        remaining, value, pieces = int(header[2]), hashlib.sha256(), []
        while remaining:
            block = self.process.stdout.read(min(1048576, remaining))
            need(block, '截断blob')
            remaining -= len(block)
            value.update(block)
            if collect:
                pieces.append(block)
        need(self.process.stdout.read(1) == b'\n', 'blob缺终止')
        self.hashes[oid] = value.hexdigest()
        return b''.join(pieces) if collect else self.hashes[oid]

    def close(self):
        self.process.stdin.close()
        need(self.process.wait(timeout=10) == 0, 'cat-file未正常结束')


def index():
    result = {}
    for row in git('ls-files', '--stage', '-z').split(b'\0'):
        if not row:
            continue
        metadata, name = row.split(b'\t', 1)
        mode, oid, stage = metadata.split()
        need(stage == b'0' and mode in {b'100644', b'100755'}, '冲突或非普通index项')
        result[name.decode('utf-8')] = (mode, oid)
    return result


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--report', type=Path)
    args = parser.parse_args()
    need(git('rev-parse', 'HEAD').strip().decode() == BASE, 'HEAD不是已推送rule20基线')
    indexed = index()
    heads = {}
    for row in git('ls-tree', '-r', '-z', 'HEAD').split(b'\0'):
        if row:
            metadata, name = row.split(b'\t', 1)
            mode, kind, oid = metadata.split()
            need(kind == b'blob', '只接受普通HEAD文件')
            heads[name.decode('utf-8')] = (mode, oid)
    parts = git('diff', '--cached', '--no-renames', '--name-status', '-z').split(b'\0')
    need(parts[-1] == b'' and len(parts[:-1]) % 2 == 0, 'Git变化列表不完整')
    changes = [(parts[i].decode(), parts[i + 1].decode('utf-8')) for i in range(0, len(parts) - 1, 2)]
    need(changes and all(state in {'A', 'M'} for state, _ in changes), '无变化或有额外删除/移动')
    stage = json.loads(repo(FORMAL).read_text(encoding='utf-8'))
    need(stage['status'] == 'limited_passed', '尚未通过本阶段限定验收')
    release = stage['runtime']['preview']
    need(release.startswith('dev-inventory-capacity-v0.21-') and '/' not in release, '快照错误')
    allowed = ALLOWED | set(stage['evidence_sha256']) | {'config/releases/' + release + '/' + n for n in ['client.json', 'server.json', 'manifest.json', 'config_types.go', 'config_types.ts']}
    need(not [name for _, name in changes if name not in allowed], '额外staged路径：' + str([name for _, name in changes if name not in allowed]))
    blobs = Blobs()
    try:
        expected = {FORMAL: sha(repo(FORMAL)), **stage['evidence_sha256']}
        def archive_blob(name):
            need(name in indexed and name in stage['evidence_sha256'], '归档未列入正式清单或index：' + name)
            raw = blobs.content(indexed[name][1], collect=True)
            need(digest(raw) == stage['evidence_sha256'][name] == sha(repo(name)), '归档index/file/正式SHA不同：' + name)
            return raw
        manifest = json.loads(archive_blob(OUT + '/evidence-manifest.json'))
        paths = json.loads(archive_blob(OUT + '/evidence-paths.json'))
        sources, targets, gzip_count = {}, set(), 0
        for row in manifest['files']:
            name = OUT + '/' + row['target']
            need(repo(name).resolve().is_relative_to(repo(OUT)) and name not in targets and row['source'] not in sources, '归档重复或目标越界')
            data = archive_blob(name)
            need(len(data) == row['file_bytes'] and digest(data) == row['file_sha256'], '归档包装SHA/bytes不符：' + name)
            need(row['encoding'] in {'identity', 'gzip'}, '未知归档格式')
            raw = gzip.decompress(data) if row['encoding'] == 'gzip' else data
            need(len(raw) == row['raw_bytes'] and digest(raw) == row['raw_sha256'] == sha(repo(row['source'])), 'index解压原SHA/bytes与源不符：' + name)
            need(paths.get(row['source']) == {'path': name, 'encoding': row['encoding'], 'raw_sha256': row['raw_sha256'], 'file_sha256': row['file_sha256']}, '证据path映射不同')
            sources[row['source']] = row['raw_sha256']
            targets.add(name)
            gzip_count += row['encoding'] == 'gzip'
        need(set(paths) == set(sources) and targets | {OUT + '/' + n for n in ['README.md', 'evidence-manifest.json', 'evidence-paths.json']} == set(stage['evidence_sha256']), '归档集合不完整')
        def add(name, value):
            repo(name)
            need(name not in expected or expected[name] == value, 'SHA交叉冲突：' + name)
            expected[name] = value
        for selector in stage['selectors'].values():
            need(sha(repo(selector['path'])) == selector['sha256'], 'selector当前原字节不匹配')
            need(sources.get(selector['path']) == selector['sha256'], 'selector与index归档原字节未绑定')
        go = json.loads(repo(stage['selectors']['go_report']['path']).read_text(encoding='utf-8'))
        client = json.loads(repo(stage['selectors']['client_report']['path']).read_text(encoding='utf-8'))
        creator = json.loads(repo(stage['selectors']['creator_report']['path']).read_text(encoding='utf-8'))
        need(go['passed'] and go['mongo_e2e_explicit'] and go['race_detector'] and go['exit_code'] == 0 and not go['failed_tests'] and not go['skipped_tests'], 'Go门禁未通过')
        need(go['source_sha256'] == go['source_sha256_after'], 'Go受测源变化')
        definition = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='enumeration_only')
        need(set(go['source_sha256']) == {p.relative_to(ROOT).as_posix() for p in definition['input_files']()}, 'Go输入枚举不全')
        client_definition = runpy.run_path(str(WORK / 'stage_checks.py'), run_name='enumeration_only')
        need(client['passed'] and client['stable'] and client['before'] == client['after'] == client_definition['snapshot'](client_definition['client_inputs']()), 'client当前冻结不匹配')
        for prefix, paths in [('external_engine', [client_definition['ENGINE'] / name for name in client_definition['ENGINE_FILES']]), ('external_font', client_definition['FONTS'])]:
            before, after = client[prefix + '_before'], client[prefix + '_after']
            need(before == after and set(before) == {str(p) for p in paths}, '外部输入白名单/前后态变化：' + prefix)
            need(all(sha(Path(name)) == value for name, value in after.items()), '外部输入当前字节变化：' + prefix)
        need(creator['success'] and creator['source_stable_during_build'] and creator['scene_prefab_layout_preserved'] and creator['exit_code'] == 36, 'Creator未通过')
        for values in [go['source_sha256'], client['after'], creator['asset_hashes'], creator['script_hashes']]:
            for name, value in values.items():
                add(name, value)
        add('server/test-results/latest.json', stage['selectors']['go_report']['sha256'])
        add('server/test-results/latest.events.jsonl', stage['selectors']['go_events']['sha256'])
        add('docs/客户端构建检查.json', stage['selectors']['creator_report']['sha256'])
        for name, value in json.loads((WORK / 'config/final-review.json').read_text(encoding='utf-8'))['current_hashes'].items():
            add(name, value)
        protected = {n for n in heads if n.startswith('client/assets/') and n not in {'client/assets/scenes/Game.scene', 'client/assets/resources/development/gameplay.json'} and (not n.startswith('client/assets/scripts/') or n.endswith('.meta'))}
        protected.add('outputs/development-prep-20261026/架构配置.xlsx')
        for name in sorted(protected):
            need(indexed.get(name) == heads[name], '原资产blob/mode变更：' + name)
            add(name, blobs.content(heads[name][1]))
        need(len(creator['asset_hashes']) == 10, '布局不是10份')
        for name in creator['asset_hashes']:
            if name != 'client/assets/scenes/Game.scene':
                need(indexed[name] == heads[name], '其它9布局变更：' + name)
        need({n for n in indexed if n.startswith(OUT + '/')} == set(stage['evidence_sha256']), 'index归档不完整或有额外文件')
        for state, name in changes:
            add(name, sha(repo(name)))
            if state == 'M':
                need(indexed[name][0] == heads[name][0], '文件mode变更：' + name)
        for name, value in expected.items():
            need(name in indexed and blobs.content(indexed[name][1]) == value and sha(repo(name)) == value, '暂存/当前/冻结原字节不符：' + name)
        need(index() == indexed and git('rev-parse', 'HEAD').strip().decode() == BASE, '核对期间HEAD/index变化')
        report = {'passed': True, 'read_only': True, 'head': BASE, 'script_sha256': sha(Path(__file__)), 'staged_paths': len(changes),
                  'git_index_blob_sha256_verified': len(expected), 'evidence_files': len(stage['evidence_sha256']),
                  'gzip_index_file_and_raw_verified': gzip_count,
                  'protected_assets_and_architecture_preserved': len(protected), 'saved_layouts': 10, 'intentional_layout_changes': ['client/assets/scenes/Game.scene'],
                  'go_tests_including_subtests': len(go['passed_tests']), 'go_inputs': len(go['source_sha256']), 'client_inputs': len(client['before']), 'unexpected_paths': []}
        if args.report:
            need(args.report.resolve().is_relative_to(WORK) and not args.report.exists(), '摘要只新建于本阶段.work')
            args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
        print(json.dumps(report, ensure_ascii=False))
    finally:
        blobs.close()


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, FileNotFoundError) as error:
        print(json.dumps({'passed': False, 'error': str(error)}, ensure_ascii=False))
        raise SystemExit(1)
