"""核对单字段UI修复的真实检查，归档必要原件并更新三份阶段说明。"""
from pathlib import Path
import datetime
import gzip
import hashlib
import json
import runpy
import shutil
import subprocess

WORK = Path(__file__).resolve().parent
ROOT = WORK.parents[1]
OUT = ROOT / 'outputs/map-confirm-wrap-20261030'
FORMAL = ROOT / 'docs/地图确认文字换行修复.json'
BASE = '103384e87ed65e9f96364974a718838cd1fa85e4'
read = lambda p: json.loads(p.read_text(encoding='utf-8'))
sha = lambda p: hashlib.sha256(p.read_bytes()).hexdigest()
write = lambda p, v: p.write_text(json.dumps(v, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
assert subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip() == BASE
assert not OUT.exists() and not FORMAL.exists()
client = read(WORK / 'client/full-2/report.json')
creator = read(WORK / 'creator/full-1/report.json')
runtime = read(WORK / 'runtime/report.json')
browser = read(WORK / 'browser/run-03/report.json')
assert client['passed'] and client['stable'] and all(r['exit_code'] == 0 for r in client['commands'])
assert client['before'] == client['after']
for name, digest in client['after'].items():
    assert sha(ROOT / name) == digest, name
for prefix in ['external_engine', 'external_font']:
    assert client[prefix + '_before'] == client[prefix + '_after']
    assert all(sha(Path(p)) == digest for p, digest in client[prefix + '_after'].items())
assert creator['success'] and creator['exit_code'] == 36 and creator['creator'] == '3.8.8'
assert creator['scene_prefab_layout_preserved'] and creator['source_stable_during_build'] and creator['served_build_preserved']
assert creator['asset_hashes'] == {p: d for p, d in client['after'].items() if p.endswith(('.scene', '.prefab'))}
assert runtime['passed'] and runtime['served_candidate_exact_match'] and not runtime['rollback_attempted']
assert runtime['backend_before'] == runtime['backend_after'] and not runtime['backend_restarted'] and not runtime['mongo_restarted']
assert browser['passed'] and browser['errors'] == [] and len(browser['checks']) == 3 and browser['warehouseWrites'] == 0
assert browser['warnings'] == ['Unexpected screen orientation: 13'] * 6
assert browser['healthBefore'] == browser['healthAfter']
assert browser['toolSha256'] == sha(ROOT / 'tools/client/verify_confirm_wrap_browser.cjs')
assert browser['creatorReportSha256'] == sha(WORK / 'creator/full-1/report.json')
assert browser['sourceBefore'] == browser['sourceAfter'] == {**creator['asset_hashes'], **creator['script_hashes']}
assert len(browser['screenshots']) == 7 and len(browser['confirmations']) == 9
for c in browser['confirmations']:
    assert c['wrap'] and c['fontSize'] == c['actualFontSize'] == 24 and c['width'] == 530 and c['height'] == 280
    assert max(c['lineWidths']) <= 531 and len(c['lines']) * c['lineHeight'] <= 280
    assert ''.join(c['lines']).replace(' ', '') == c['text'].replace('\n', '').replace(' ', '')
assert [len(c['lines']) for c in browser['confirmations']] == [2, 2, 2, 2, 1, 1, 1, 1, 7]
old = json.loads(subprocess.check_output(['git', 'show', BASE + ':client/assets/scenes/Game.scene'], cwd=ROOT))
new = read(ROOT / 'client/assets/scenes/Game.scene')
old[483]['_enableWrapText'] = True
assert new == old
go = read(ROOT / '.work/warehouses-20261030/go/full-2/report.json')
enumerate_go = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='only_check_previous_go_inputs')['input_files']
assert go['source_sha256_after'] == {p.relative_to(ROOT).as_posix(): sha(p) for p in enumerate_go()}

OUT.mkdir(parents=True)
records = []
def add(p, relative):
    assert p.is_file() and p.resolve().is_relative_to(ROOT)
    target = OUT / relative
    compressed = p.suffix in {'.json', '.txt', '.log', '.scene'} and p.stat().st_size > 131072
    if compressed:
        target = target.with_name(target.name + '.gz')
    target.parent.mkdir(parents=True, exist_ok=True)
    raw = p.read_bytes()
    target.write_bytes(gzip.compress(raw, mtime=0) if compressed else raw)
    assert (gzip.decompress(target.read_bytes()) if compressed else target.read_bytes()) == raw
    records.append({'source': p.relative_to(ROOT).as_posix(), 'target': target.relative_to(ROOT).as_posix(),
                    'encoding': 'gzip' if compressed else 'identity', 'raw_sha256': hashlib.sha256(raw).hexdigest(), 'file_sha256': sha(target)})

for directory in ['client/full-1', 'client/full-2', 'creator/full-1', 'runtime']:
    folder = WORK / directory
    for p in sorted(folder.iterdir()):
        if p.is_file():
            add(p, ('history/' if directory == 'client/full-1' else 'current/') + directory + '/' + p.name)
for run in ['run-01', 'run-02', 'run-03']:
    folder = WORK / 'browser' / run
    names = ['report.json', 'tool-at-run.cjs']
    if run == 'run-03':
        names += browser['screenshots']
    for name in names:
        p = folder / name
        if p.exists():
            add(p, ('current/' if run == 'run-03' else 'history/') + 'browser/' + run + '/' + name)
for name in ['edit-report.json', 'edit_saved_label.py', 'stage_checks.py', 'build_candidate.py', 'publish_static.py', 'accept.py']:
    add(WORK / name, 'sources/' + name)
add(ROOT / 'tools/client/verify_confirm_wrap_browser.cjs', 'sources/verify_confirm_wrap_browser.cjs')
manifest = OUT / 'evidence-manifest.json'
write(manifest, {'baseline': BASE, 'files': records})
evidence = {row['target']: row['file_sha256'] for row in records}
evidence[manifest.relative_to(ROOT).as_posix()] = sha(manifest)
result = {'status': 'limited_passed', 'baseline': BASE, 'recorded_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
          'scope': '通用保存Confirm正文换行修复与三种视口触屏；不是新玩法、经济校准、真机或容量验收',
          'only_product_change': {'file': 'client/assets/scenes/Game.scene', 'object': 483, 'field': '_enableWrapText', 'before': False, 'after': True},
          'runtime_rule_version': 23, 'gameplay_workbooks_changed': False, 'server_changed': False,
          'javascript_tests': 496, 'client_inputs': 235, 'creator_seconds': creator['seconds'], 'creator_exit_code': 36,
          'browser_checks': 3, 'browser_errors': [], 'browser_warnings': browser['warnings'],
          'actual_view_image_count': 7, 'visual_reviewer': 'Root', 'visual_passed': True,
          'viewport_sizes': [{'width': 720, 'height': 1280}, {'width': 1024, 'height': 1280}, {'width': 720, 'height': 1600}],
          'actual_line_counts': [len(c['lines']) for c in browser['confirmations']],
          'raw_check_description_note': '浏览器第三条check短句写八行是描述笔误；实际仓储确认截面和截图为7行，210像素，按真实数值验收。',
          'warning_scope': '六条方向警告来自锁定引擎render-window.ts:263重置窗口分支；实际文字/尺寸/触屏通过，保留原警告，真机旋转待验。',
          'previous_go_reused_only_if_exact_source_equal': {'tests': 1269, 'source_inputs': 206, 'new_go_test_run': False},
          'artifact_transfer': runtime['artifact_transfer'], 'backend_restarted': False, 'mongo_restarted': False,
          'full_p0_completed': False, 'production_ready': False, 'evidence_sha256': evidence,
          'history': ['client/full-1为旧历史布局哈希与禁换行断言的6红，准确单字段归一化后全496绿',
                      'browser/run-01为QA全局UITransform取空；run-02为QA误要求能完整放下一行的短句必须多行；原失败终态保留，不追认通过']}
write(FORMAL, result)
summary = ('2026-10-30：已修复地图往返确认文字截断，通用Confirm仅在保存Game.scene中启用正文自动换行，字号24与530×280尺寸保持。'
           '496项客户端检查与Creator3.8.8实构通过；三种窗口尺寸触屏、六次取消、两次往返及仓储确认取消通过，7张截图已复核。'
           '原回收文本像素度量及历史布局保护保留，只放行该开关；后端、唯一玩法表与rule23保持。浏览器0错误，6条引擎方向警告原样登记，真机旋转待验。')
for name in ['README.md', 'client/README.md', 'docs/正式开发推进记录.md']:
    p = ROOT / name
    text = p.read_text(encoding='utf-8')
    assert summary not in text
    first, rest = text.split('\n', 1)
    link = 'docs/地图确认文字换行修复.json' if name == 'README.md' else '../docs/地图确认文字换行修复.json' if name == 'client/README.md' else '地图确认文字换行修复.json'
    p.write_text(first + '\n\n' + summary + '见[修复验收](' + link + ')。\n' + rest, encoding='utf-8', newline='\n')
print(json.dumps({'passed': True, 'formal': FORMAL.relative_to(ROOT).as_posix(), 'evidence_files': len(evidence)}, ensure_ascii=False))
