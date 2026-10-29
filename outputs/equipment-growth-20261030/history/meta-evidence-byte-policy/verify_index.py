"""只读核对阶段22暂存原blob、当前受测输入和原资源；不改正式验收结论。"""
from pathlib import Path
import argparse, gzip, hashlib, io, json, os, runpy, subprocess

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
BASE = '2a5616ac883c91336edb227d7bbf87e9cfe64791'
FORMAL = 'docs/基础装备属性与正常穿脱阶段验收.json'
OUT = 'outputs/equipment-growth-20261030'
ALLOWED = set(json.loads((WORK / 'source-allowlist.json').read_text(encoding='utf-8'))['allowed_paths'])
ATTRIBUTES_BASE_SHA256 = 'da5a722d187c456b15b31bc6150040d88b812bac11345c484b128f8be2b0f54f'
ATTRIBUTES_AFTER_SHA256 = 'f138d9711b185e4ab0aa02513bbeb1ee074c6243eb89dda6e4d21b3b4f32559d'
EVIDENCE_APPEND = b'outputs/equipment-growth-20261030/** -text\n'


def need(value, message):
    if not value:
        raise ValueError(message)


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def sha(p):
    return digest(p.read_bytes())


def expected_evidence_attributes(base):
    need(digest(base)==ATTRIBUTES_BASE_SHA256 and base.endswith(b'\n') and EVIDENCE_APPEND not in base,
         'Git证据策略BASE原字节不同')
    expected=base+EVIDENCE_APPEND
    need(digest(expected)==ATTRIBUTES_AFTER_SHA256, 'Git证据策略不是唯一43B追加')
    return expected


def verify_evidence_attributes(base,working,indexed_raw,base_mode,index_mode):
    expected=expected_evidence_attributes(base)
    need(base_mode==index_mode==b'100644', 'Git证据策略mode不能变化')
    need(working==indexed_raw==expected, 'Git证据策略current/index必须严格等于BASE+唯一阶段-text行')
    return digest(expected)


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
    need(git('rev-parse', 'HEAD').strip().decode() == BASE, 'HEAD不是已推送rule21基线')
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
    need(stage['status']=='limited_passed' and stage['runtime']['rule_version']==22 and stage['full_p0_completed'] is False,'尚未通过本阶段22限定验收或夸称完整P0')
    need(stage['runtime']['source220_enabled'] is False and stage['verification']['saved_layouts_unchanged_against_head']==10 and stage['verification']['saved_layouts_intentionally_changed']==0,'范围或10布局声明不符')
    release = stage['runtime']['preview']
    need(release.startswith('dev-equipment-foundation-v0.22-') and '/' not in release, '快照错误')
    allowed = ALLOWED | set(stage['evidence_sha256']) | {'config/releases/' + release + '/' + n for n in ['client.json', 'server.json', 'manifest.json', 'config_types.go', 'config_types.ts']}
    need(not [name for _, name in changes if name not in allowed], '额外staged路径：' + str([name for _, name in changes if name not in allowed]))
    blobs = Blobs()
    try:
        expected = {FORMAL: sha(repo(FORMAL)), **stage['evidence_sha256']}
        need('.gitattributes' in heads and '.gitattributes' in indexed, '缺原或暂存Git证据字节策略')
        expected['.gitattributes']=verify_evidence_attributes(blobs.content(heads['.gitattributes'][1],collect=True),
            repo('.gitattributes').read_bytes(),blobs.content(indexed['.gitattributes'][1],collect=True),
            heads['.gitattributes'][0],indexed['.gitattributes'][0])
        def archive_blob(name):
            need(name in indexed and name in stage['evidence_sha256'], '归档未列入正式清单或index：' + name)
            raw = blobs.content(indexed[name][1], collect=True)
            need(digest(raw) == stage['evidence_sha256'][name] == sha(repo(name)), '归档index/file/正式SHA不同：' + name)
            return raw
        manifest = json.loads(archive_blob(OUT + '/evidence-manifest.json'))
        archive_paths = json.loads(archive_blob(OUT + '/evidence-paths.json'))
        sources, targets, gzip_count = {}, set(), 0
        for row in manifest['files']:
            name = OUT + '/' + row['target']
            need(repo(name).resolve().is_relative_to(repo(OUT)) and name not in targets and row['source'] not in sources, '归档重复或目标越界')
            data = archive_blob(name)
            need(len(data) == row['file_bytes'] and digest(data) == row['file_sha256'], '归档包装SHA/bytes不符：' + name)
            need(row['encoding'] in {'identity', 'gzip'}, '未知归档格式')
            raw = gzip.decompress(data) if row['encoding'] == 'gzip' else data
            need(len(raw) == row['raw_bytes'] and digest(raw) == row['raw_sha256'] == sha(repo(row['source'])), 'index解压原SHA/bytes与源不符：' + name)
            need(archive_paths.get(row['source']) == {'path': name, 'encoding': row['encoding'], 'raw_sha256': row['raw_sha256'], 'file_sha256': row['file_sha256']}, '证据path映射不同')
            sources[row['source']] = row['raw_sha256']
            targets.add(name)
            gzip_count += row['encoding'] == 'gzip'
        need(set(archive_paths) == set(sources) and targets | {OUT + '/' + n for n in ['README.md', 'evidence-manifest.json', 'evidence-paths.json']} == set(stage['evidence_sha256']), '归档集合不完整')
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
        publication=json.loads(repo(stage['selectors']['publication_report']['path']).read_text(encoding='utf-8'))
        need(go['passed'] and go['mongo_e2e_explicit'] and go['race_detector'] and go['exit_code'] == 0 and not go['failed_tests'] and not go['skipped_tests'], 'Go门禁未通过')
        need(go['source_sha256'] == go['source_sha256_after'], 'Go受测源变化')
        definition = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='enumeration_only')
        need(set(go['source_sha256']) == {p.relative_to(ROOT).as_posix() for p in definition['input_files']()}, 'Go输入枚举不全')
        client_definition = runpy.run_path(str(WORK / 'stage_checks.py'), run_name='enumeration_only')
        need(client['passed'] and client['stable'] and client['before'] == client['after'] == client_definition['snapshot'](client_definition['client_inputs']()), 'client当前冻结不匹配')
        for prefix, external_paths in [('external_engine', [client_definition['ENGINE'] / name for name in client_definition['ENGINE_FILES']]), ('external_font', client_definition['FONTS'])]:
            before, after = client[prefix + '_before'], client[prefix + '_after']
            need(before == after and set(before) == {str(p) for p in external_paths}, '外部输入白名单/前后态变化：' + prefix)
            need(all(sha(Path(name)) == value for name, value in after.items()), '外部输入当前字节变化：' + prefix)
        need(creator['success'] and creator['source_stable_during_build'] and creator['scene_prefab_layout_preserved'] and creator['exit_code'] == 36, 'Creator未通过')
        need(creator['served_build_preserved'] and publication['passed'] and publication['served_candidate_exact_match'] and publication['mongo_service_restarted'] is False,'候选构建/切主事实未通过')
        manifest_path=Path(creator['artifact_manifest']).resolve()
        need(manifest_path.is_relative_to(ROOT) and sha(manifest_path)==creator['artifact_manifest_sha256']==publication['candidate_manifest_sha256'],'候选产物清单SHA未配对')
        need(sources.get(manifest_path.relative_to(ROOT).as_posix())==creator['artifact_manifest_sha256'],'候选manifest未绑定实际归档raw字节')
        artifact=json.loads(manifest_path.read_text(encoding='utf-8'));build_root=Path(creator['build_root']).resolve()
        need(build_root.is_relative_to(WORK),'候选build_root不属于本阶段')
        actual={p.relative_to(ROOT).as_posix():sha(p) for p in build_root.rglob('*') if p.is_file()}
        need(actual==artifact and len(actual)==creator['artifact_file_count'],'候选当前产物不符原manifest')
        served=ROOT/'client/build/web-mobile';expected_served={(ROOT/name).relative_to(build_root).as_posix():value for name,value in artifact.items()}
        need({p.relative_to(served).as_posix():sha(p) for p in served.rglob('*') if p.is_file()}==expected_served,'当前主预览包不是原真实候选')
        for values in [go['source_sha256'], client['after'], creator['asset_hashes'], creator['script_hashes']]:
            for name, value in values.items():
                add(name, value)
        add('server/test-results/latest.json', stage['selectors']['go_report']['sha256'])
        add('server/test-results/latest.events.jsonl', stage['selectors']['go_events']['sha256'])
        add('docs/客户端构建检查.json', stage['selectors']['creator_report']['sha256'])
        transition_module=WORK/'config/shared_test_transition.py'
        transition=runpy.run_path(str(transition_module),run_name='indexed_exact_shared_test_transition_only')
        need(sources.get(transition_module.relative_to(ROOT).as_posix())==sha(transition_module),'共享迁移校验源未绑定实际归档index')
        def transition_raw(name):
            if name in indexed:
                return blobs.content(indexed[name][1],collect=True)
            need(name in archive_paths and name in sources,'共享迁移证据未绑定index原blob：'+name)
            record=archive_paths[name]
            data=archive_blob(record['path'])
            raw=gzip.decompress(data) if record['encoding']=='gzip' else data
            need(digest(raw)==record['raw_sha256']==sources[name], '共享迁移index解压原字节不符：'+name)
            return raw
        for name, value in transition['verify'](transition_raw).items():
            add(name, value)
        protected = {n for n in heads if n.startswith('client/assets/') and n != 'client/assets/resources/development/gameplay.json' and (not n.startswith('client/assets/scripts/') or n.endswith('.meta'))}
        protected.add('outputs/development-prep-20261026/架构配置.xlsx')
        for name in sorted(protected):
            need(indexed.get(name) == heads[name], '原资产blob/mode变更：' + name)
            add(name, blobs.content(heads[name][1]))
        need(len(creator['asset_hashes']) == 10, '布局不是10份')
        for name in creator['asset_hashes']:
            need(indexed[name] == heads[name], '阶段22的10布局不得改变：' + name)
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
                  'protected_assets_and_architecture_preserved': len(protected), 'saved_layouts': 10, 'intentional_layout_changes': [],
                  'go_tests_including_subtests': len(go['passed_tests']), 'go_inputs': len(go['source_sha256']), 'client_inputs': len(client['before']), 'unexpected_paths': []}
        report['evidence_git_attributes']={'base_sha256':ATTRIBUTES_BASE_SHA256,'current_and_index_sha256':expected['.gitattributes'],
            'only_append':EVIDENCE_APPEND.decode('utf-8'),'mode':'100644'}
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
