"""提取真实index reader作窄验证；原index对应Root已移存的378快照，不跑完整index/产品。"""
from pathlib import Path
import ast
from copy import deepcopy
import json
import runpy


WORK = Path(__file__).resolve().parents[1]
ROOT = WORK.parents[1]
OUT = Path(__file__).resolve().parent


def main():
    verifier = WORK / 'verify_index.py'
    module = runpy.run_path(str(verifier), run_name='only_real_index_reader_helpers')
    tree = ast.parse(verifier.read_text(encoding='utf-8'))
    function = next(n for n in tree.body if isinstance(n, ast.FunctionDef) and n.name == 'main')
    archive_reader = next(n for n in ast.walk(function) if isinstance(n, ast.FunctionDef) and n.name == 'archive_blob')
    transition_reader = next(n for n in ast.walk(function) if isinstance(n, ast.FunctionDef) and n.name == 'transition_raw')
    external_loop = next(n for n in ast.walk(function) if isinstance(n, ast.For) and isinstance(n.target, ast.Tuple)
                         and [x.id for x in n.target.elts] == ['prefix', 'external_paths'])
    # 静态核对后段闭包依赖只有各自一次赋值；不再容许paths同时担当map和白名单。
    stores = {}
    for node in ast.walk(function):
        if isinstance(node, ast.Name) and isinstance(node.ctx, ast.Store):
            stores.setdefault(node.id, []).append(node.lineno)
    assert 'paths' not in stores and len(stores['archive_paths']) == len(stores['external_paths']) == 1
    indexed, blobs = module['index'](), module['Blobs']()
    selected = set()
    try:
        stage_raw = blobs.content(indexed[module['FORMAL']][1], collect=True)
        stage = json.loads(stage_raw)
        preserved = WORK / 'history/pre-meta-eol-policy'
        assert module['digest'](stage_raw) == module['sha'](preserved / 'formal-report.json')
        assert len(stage['evidence_sha256']) == 378
        original_repo = module['repo']

        def preserved_repo(name):
            path = original_repo(name)
            if name.startswith(module['OUT'] + '/'):
                # Root已rename旧工作副本；读取其原文件配对实际index原blob，不创建替身证据。
                return preserved / 'archive' / Path(name).relative_to(module['OUT'])
            return path

        scope = {**module, 'indexed': indexed, 'blobs': blobs, 'stage': stage, 'repo': preserved_repo}
        exec(compile(ast.Module(body=[deepcopy(archive_reader)], type_ignores=[]), str(verifier), 'exec'), scope)
        archive_blob = scope['archive_blob']
        scope['archive_paths'] = json.loads(archive_blob(module['OUT'] + '/evidence-paths.json'))
        manifest = json.loads(archive_blob(module['OUT'] + '/evidence-manifest.json'))
        scope['sources'] = {row['source']: row['raw_sha256'] for row in manifest['files']}
        assert len(scope['archive_paths']) == len(scope['sources']) == 375
        original_map = deepcopy(scope['archive_paths'])
        scope['client'] = json.loads((WORK / 'client/full-1/report.json').read_text(encoding='utf-8'))
        scope['client_definition'] = runpy.run_path(str(WORK / 'stage_checks.py'), run_name='only_external_input_whitelist')
        exec(compile(ast.Module(body=[deepcopy(external_loop)], type_ignores=[]), str(verifier), 'exec'), scope)
        assert isinstance(scope['archive_paths'], dict) and scope['archive_paths'] == original_map
        assert isinstance(scope['external_paths'], list) and len(scope['external_paths']) == 3
        exec(compile(ast.Module(body=[deepcopy(transition_reader)], type_ignores=[]), str(verifier), 'exec'), scope)
        actual_reader = scope['transition_raw']

        def recorded_reader(name):
            selected.add(name)
            return actual_reader(name)

        transition = runpy.run_path(str(WORK / 'config/shared_test_transition.py'), run_name='actual_index_raw_transition_check')
        effective = transition['verify'](recorded_reader)
        assert len(effective) == 31 and effective[transition['TEST_PATH']] == transition['NEW_SHA']
        document = transition['DOCUMENT']
        assert document not in indexed and original_map[document]['raw_sha256'] == module['sha'](WORK / 'config/shared-test-transition.json')
        refused = []
        for kind in ['missing_map_entry', 'wrong_raw_sha']:
            scope['archive_paths'] = deepcopy(original_map)
            if kind == 'missing_map_entry':
                scope['archive_paths'].pop(document)
            else:
                scope['archive_paths'][document]['raw_sha256'] = '0' * 64
            try:
                actual_reader(document)
            except ValueError:
                refused.append(kind)
            else:
                raise AssertionError(kind + '未拒绝')
        scope['archive_paths'] = original_map
        assert len(refused) == 2
        report = {'passed': True, 'scope': '仅提取真实reader/外部循环，读取实际旧378暂存原blob及Root已保留的相同工作快照；不是新正式index全通过或产品重测',
                  'verifier_sha256': module['sha'](verifier), 'check_source_sha256': module['sha'](Path(__file__)),
                  'actual_index_formal_sha256': module['digest'](stage_raw), 'archive_map_entries': len(original_map),
                  'external_engine_inputs_checked': 9, 'external_font_inputs_checked': 3,
                  'external_loop_kept_archive_map': True, 'shared_configuration_inputs_verified': len(effective),
                  'shared_test_current_sha256': effective[transition['TEST_PATH']], 'required_go3_raw_passes': 2,
                  'negative_cases_refused': refused,
                  'original_index_oids_used': {name: indexed[name][1].decode() for name in sorted(selected) if name in indexed},
                  'archive_sources_used': {name: original_map[name] for name in sorted(selected) if name not in indexed},
                  'complete_index_or_artifact_tree_scan': False, 'git_index_modified': False, 'product_test_rerun': False}
        with (OUT / 'report-meta-policy.json').open('x', encoding='utf-8', newline='\n') as stream:
            stream.write(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps({'passed': True, 'archive_entries': len(original_map), 'configuration_inputs': len(effective),
                          'raw_go3_passes': 2, 'report_sha256': module['sha'](OUT / 'report-meta-policy.json')}, ensure_ascii=False))
    finally:
        blobs.close()


if __name__ == '__main__':
    main()
