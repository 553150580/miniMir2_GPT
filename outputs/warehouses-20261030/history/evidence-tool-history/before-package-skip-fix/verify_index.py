"""阶段23只读核Git index原blob/归档解压/受测源及旧资产；不暂存或改报告。"""
from pathlib import Path
import argparse
import gzip
import hashlib
import json
import os
import runpy
import subprocess

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
BASE = 'd2dab71909d4c389482be9b95d9594db76310eeb'
FORMAL = 'docs/双仓储与五角色并发阶段验收.json'
OUT = 'outputs/warehouses-20261030'


def need(ok, message):
    if not ok:
        raise ValueError(message)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def repo(name):
    need(isinstance(name, str) and '\\' not in name and ':' not in name and not Path(name).is_absolute() and '..' not in Path(name).parts, '非法repo路径：' + str(name))
    path = ROOT / name
    need(path.resolve().is_relative_to(ROOT), '越界repo路径')
    return path


def sha(path):
    value = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1048576), b''):
            value.update(block)
    return value.hexdigest()


def git(*args):
    return subprocess.check_output(['git', *args], cwd=ROOT, env={**os.environ, 'GIT_OPTIONAL_LOCKS': '0'})


class Blobs:
    """collect=True始终返原bytes；默认返SHA，不能把digest当原文。"""
    def __init__(self):
        self.process = subprocess.Popen(['git', 'cat-file', '--batch'], cwd=ROOT, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.hashes = {}

    def content(self, oid, collect=False):
        if not collect and oid in self.hashes:
            return self.hashes[oid]
        self.process.stdin.write(oid + b'\n'); self.process.stdin.flush()
        header = self.process.stdout.readline().rstrip(b'\n').split()
        need(len(header) == 3 and header[0] == oid and header[1] == b'blob', '不是所需原blob')
        remaining, hashed, parts = int(header[2]), hashlib.sha256(), []
        while remaining:
            block = self.process.stdout.read(min(1048576, remaining))
            need(block, '原blob截断')
            remaining -= len(block); hashed.update(block)
            if collect:
                parts.append(block)
        need(self.process.stdout.read(1) == b'\n', '原blob缺终止')
        self.hashes[oid] = hashed.hexdigest()
        return b''.join(parts) if collect else self.hashes[oid]

    def close(self):
        self.process.stdin.close()
        try:
            need(self.process.wait(timeout=10) == 0, 'cat-file未正常退出')
        finally:
            self.process.stdout.close(); self.process.stderr.close()


def index():
    result = {}
    for row in git('ls-files', '--stage', '-z').split(b'\0'):
        if not row:
            continue
        metadata, name = row.split(b'\t', 1)
        mode, oid, stage = metadata.split()
        need(stage == b'0' and mode in {b'100644', b'100755'}, '冲突/非普通index项')
        result[name.decode('utf-8')] = (mode, oid)
    return result


def read_archived_source(name, archive_paths, sources, archive_reader, direct_reader=None):
    """完整pathmap必须是独立dict；reader返回原bytes，禁止混入外部字体路径list。"""
    need(isinstance(archive_paths, dict) and isinstance(sources, dict) and name in sources and name in archive_paths, '证据源没有绑定完整index pathmap')
    info = archive_paths[name]
    data = archive_reader(info['path'])
    need(isinstance(data, bytes), '归档reader必须返回原bytes，不能返回digest')
    raw = gzip.decompress(data) if info['encoding'] == 'gzip' else data
    need(digest(raw) == info['raw_sha256'] == sources[name], '归档reader原SHA不同')
    if direct_reader is not None:
        need(direct_reader(name) == raw, '直接暂存与归档原文冲突')
    return raw


def main():
    parser = argparse.ArgumentParser(); parser.add_argument('--report', type=Path)
    args = parser.parse_args()
    need(git('rev-parse', 'HEAD').strip().decode() == BASE, 'HEAD不是已推送rule22基线')
    indexed, heads = index(), {}
    for row in git('ls-tree', '-r', '-z', BASE).split(b'\0'):
        if row:
            meta, name = row.split(b'\t', 1); mode, kind, oid = meta.split()
            need(kind == b'blob', 'HEAD含非blob')
            heads[name.decode('utf-8')] = (mode, oid)
    fields = git('diff', '--cached', '--no-renames', '--name-status', '-z').split(b'\0')
    need(fields[-1] == b'' and len(fields[:-1]) % 2 == 0, '变化列表不完整')
    changes = [(fields[i].decode(), fields[i + 1].decode('utf-8')) for i in range(0, len(fields) - 1, 2)]
    need(changes and all(state in {'A', 'M'} for state, _ in changes), '空stage或包含未批准删除/移动')
    stage = json.loads(repo(FORMAL).read_text(encoding='utf-8'))
    need(stage['status'] == 'limited_passed' and stage['runtime']['rule_version'] == 23 and stage['full_p0_completed'] is False and stage['production_ready'] is False, '阶段范围或终态不符')
    need(stage['verification']['saved_layouts_unchanged_against_base'] == 9 and stage['verification']['saved_layouts_intentionally_changed'] == 1, '9旧布局+1意图增量声明不同')
    release = stage['runtime']['preview']
    need(release.startswith('dev-warehouses-v0.23-') and '/' not in release, '不是本阶段preview')
    allowed = set(json.loads((WORK / 'source-allowlist.json').read_text(encoding='utf-8'))['allowed_paths']) | set(stage['evidence_sha256'])
    allowed |= {'config/releases/' + release + '/' + name for name in ['client.json', 'server.json', 'manifest.json', 'config_types.go', 'config_types.ts']}
    need(not [name for _, name in changes if name not in allowed], '额外stage路径：' + str([name for _, name in changes if name not in allowed]))
    blobs = Blobs()
    try:
        expected = {FORMAL: sha(repo(FORMAL)), **stage['evidence_sha256']}
        base_attributes = blobs.content(heads['.gitattributes'][1], collect=True)
        need(digest(base_attributes) == 'd278504d449285e20319dfb09dbc1d8e0ca6c7a9ec414efa8b475154af285bec' and base_attributes.endswith(b'\n'), 'Git属性BASE不同')
        attributes = base_attributes + b'outputs/warehouses-20261030/** -text\n'
        need(heads['.gitattributes'][0] == indexed['.gitattributes'][0] == b'100644' and repo('.gitattributes').read_bytes() == blobs.content(indexed['.gitattributes'][1], collect=True) == attributes, '只能精确追加本阶段-text行')
        expected['.gitattributes'] = digest(attributes)

        def archive_blob(name):
            need(name in indexed and name in stage['evidence_sha256'], '缺正式清单或index归档：' + name)
            raw = blobs.content(indexed[name][1], collect=True)
            need(digest(raw) == stage['evidence_sha256'][name] == sha(repo(name)), '归档index/file/正式SHA不同：' + name)
            return raw

        manifest = json.loads(archive_blob(OUT + '/evidence-manifest.json'))
        archive_paths = json.loads(archive_blob(OUT + '/evidence-paths.json'))
        sources, targets, gzip_count = {}, set(), 0
        for row in manifest['files']:
            name = OUT + '/' + row['target']
            need(repo(name).resolve().is_relative_to(repo(OUT)) and name not in targets and row['source'] not in sources, '重复/越界归档')
            raw_file = archive_blob(name)
            need(digest(raw_file) == row['file_sha256'] and len(raw_file) == row['file_bytes'] and row['encoding'] in {'identity', 'gzip'}, '包装SHA/bytes/格式不同')
            raw = gzip.decompress(raw_file) if row['encoding'] == 'gzip' else raw_file
            need(digest(raw) == row['raw_sha256'] == sha(repo(row['source'])) and len(raw) == row['raw_bytes'], '原件/解压原SHA或bytes不同')
            need(archive_paths.get(row['source']) == {'path': name, 'encoding': row['encoding'], 'raw_sha256': row['raw_sha256'], 'file_sha256': row['file_sha256']}, 'pathmap没有完整配对')
            sources[row['source']] = row['raw_sha256']; targets.add(name); gzip_count += row['encoding'] == 'gzip'
        need(set(archive_paths) == set(sources) and targets | {OUT + '/' + n for n in ['README.md', 'evidence-manifest.json', 'evidence-paths.json']} == set(stage['evidence_sha256']), '归档/pathmap/正式完整集合不同')

        def source_blob(name):
            """优先核被暂存原文，否则从完整archive_paths解压；不使用外部路径循环变量。"""
            direct = (lambda key: blobs.content(indexed[key][1], collect=True)) if name in indexed else None
            return read_archived_source(name, archive_paths, sources, archive_blob, direct)

        for selected in stage['selectors'].values():
            need(digest(source_blob(selected['path'])) == selected['sha256'], 'selector未绑定index原证据')
        go = json.loads(source_blob(stage['selectors']['go_report']['path']))
        client = json.loads(source_blob(stage['selectors']['client_report']['path']))
        creator = json.loads(source_blob(stage['selectors']['creator_report']['path']))
        need(go['passed'] and go['exit_code'] == 0 and go['mongo_e2e_explicit'] and go['race_detector'] and go['failed_tests'] == go['skipped_tests'] == [], 'Go终态不符')
        need(go['command'][2:] == ['go', 'test', '-json', '-race', '-p', '1', './...', '-count=1', '-timeout=1200s'] and go['package_timeout_seconds'] == 1200 and go['package_parallelism'] == 1 and go['single_rpc_and_business_deadlines_changed'] is False, 'index原Go报告不是完整未过滤原命令')
        events_raw = source_blob(stage['selectors']['go_events']['path'])
        need(digest(events_raw) == go['events_sha256'], 'Go原events未绑定index')
        events = [json.loads(line) for line in events_raw.splitlines() if line.strip()]
        passed = [{'package': e['Package'], 'test': e['Test']} for e in events if e.get('Test') and e['Action'] == 'pass']
        need(passed == go['passed_tests'] and not [e for e in events if e['Action'] in {'fail', 'build-fail', 'skip'}], 'Go原events真实统计不同')
        module = runpy.run_path(str(WORK / 'finalize_evidence.py'), run_name='readonly_index_helpers')
        need(go['source_sha256'] == go['source_sha256_after'] == module['snapshot']('go') and len(go['source_sha256']) == 206, 'Go206当前冻结输入不完整')
        need(client['passed'] and client['stable'] and client['before'] == client['after'] == module['snapshot']('client') and client['input_count'] == len(client['after']), 'client完整枚举/当前冻结输入不同')
        module['external'](client)
        module['layout_evidence'](creator['asset_hashes'])
        need(creator['success'] and creator['exit_code'] == 36 and creator['source_stable_during_build'] and creator['scene_prefab_layout_preserved'], 'Creator未通过')
        for values in [go['source_sha256'], client['after'], creator['asset_hashes'], creator['script_hashes']]:
            for name, value in values.items():
                need(name not in expected or expected[name] == value, '冻结输入SHA交叉冲突：' + name)
                expected[name] = value
        expected['server/test-results/latest.json'] = stage['selectors']['go_report']['sha256']
        expected['server/test-results/latest.events.jsonl'] = stage['selectors']['go_events']['sha256']
        expected['docs/客户端构建检查.json'] = stage['selectors']['creator_report']['sha256']
        protected = {name for name in heads if name.startswith('client/assets/') and name not in {'client/assets/resources/development/gameplay.json', 'client/assets/scenes/Game.scene'} and (not name.startswith('client/assets/scripts/') or name.endswith('.meta'))}
        protected.add('outputs/development-prep-20261026/架构配置.xlsx')
        for name in sorted(protected):
            need(indexed.get(name) == heads[name], '旧资产blob/mode变化：' + name)
            expected[name] = blobs.content(heads[name][1])
        need({name for name in indexed if name.startswith(OUT + '/')} == set(stage['evidence_sha256']), 'index归档遗漏或有额外文件')
        for state, name in changes:
            need(state != 'M' or indexed[name][0] == heads[name][0], '原文件mode变化')
            need(name not in expected or expected[name] == sha(repo(name)), 'current与冻结SHA交叉冲突：' + name)
            expected[name] = sha(repo(name))
        for name, value in expected.items():
            need(name in indexed and blobs.content(indexed[name][1]) == value == sha(repo(name)), '原index blob/current/冻结字节不同：' + name)
        need(index() == indexed and git('rev-parse', 'HEAD').strip().decode() == BASE, '核验期间index或HEAD变化')
        result = {'passed': True, 'read_only': True, 'head': BASE, 'script_sha256': sha(Path(__file__)), 'staged_paths': len(changes),
                  'git_index_blob_sha256_verified': len(expected), 'evidence_files': len(stage['evidence_sha256']), 'gzip_index_file_and_raw_verified': gzip_count,
                  'protected_assets_and_architecture_preserved': len(protected), 'saved_layouts': 10, 'intentional_layout_changes': ['client/assets/scenes/Game.scene'],
                  'go_tests_including_subtests': len(passed), 'go_inputs': 206, 'client_inputs': len(client['after']), 'unexpected_paths': []}
        if args.report:
            need(args.report.resolve().is_relative_to(WORK) and not args.report.exists(), '摘要只新建于本阶段.work')
            args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
        print(json.dumps(result, ensure_ascii=False))
    finally:
        blobs.close()


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, FileNotFoundError) as error:
        print(json.dumps({'passed': False, 'error': str(error)}, ensure_ascii=False))
        raise SystemExit(1)
