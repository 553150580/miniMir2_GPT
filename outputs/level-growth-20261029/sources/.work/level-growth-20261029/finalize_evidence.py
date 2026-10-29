"""阶段20证据门禁；默认只读--check，Root明确--apply后才新建正式归档。"""
from __future__ import annotations

import argparse
from collections import Counter
from copy import deepcopy
from datetime import datetime, timezone, timedelta
import gzip
import hashlib
import json
import math
from pathlib import Path
import re
import runpy
import shutil
import sys
import xml.etree.ElementTree as ET
import zipfile

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
OUT = ROOT / 'outputs/level-growth-20261029'
FORMAL = ROOT / 'docs/自然成长与待用经验阶段验收.json'
ENGINE = Path('D:/Program Files/CocosCreator/3.8.8/resources/resources/3d/engine/cocos').resolve()
ENGINE_FILES = ['input/input.ts', '2d/event/pointer-event-dispatcher.ts', 'scene-graph/node-event-processor.ts',
                'scene-graph/node.ts', 'ui/button.ts', 'core/data/object.ts', 'scene-graph/component.ts',
                'audio/audio-source.ts', 'ui/toggle.ts']
FONT_FILES = [Path('C:/Windows/Fonts/arial.ttf'), Path('C:/Windows/Fonts/msyh.ttc'), Path('C:/Windows/Fonts/simsun.ttc')]
COMMANDS = {'models', 'full-ui-app', 'scene', 'strict', 'protocol-check', 'protocol-lint'}
FORBIDDEN = {'client-project', 'library', 'temp', 'build', 'node_modules', '__pycache__', '.git', 'inputs'}
BOOK = 'outputs/development-prep-20261026/玩法配置.xlsx'
ARCH = 'outputs/development-prep-20261026/架构配置.xlsx'
FIXTURE = 'config/development/first-loop.json'
PUBLIC = 'client/assets/resources/development/gameplay.json'
NS = {'s': 'http://schemas.openxmlformats.org/spreadsheetml/2006/main'}


def need(ok, message):
    if not ok:
        raise ValueError(message)


def sha(path):
    result = hashlib.sha256()
    with Path(path).open('rb') as stream:
        for part in iter(lambda: stream.read(1024 * 1024), b''):
            result.update(part)
    return result.hexdigest()


def read(path):
    return json.loads(Path(path).read_text(encoding='utf-8'))


def local(value, base=ROOT):
    p = Path(value)
    if not p.is_absolute():
        p = base / p
    p = p.resolve()
    need(p.is_relative_to(base.resolve()), '路径越过受控目录：' + str(p))
    need(not p.is_symlink() and not getattr(p, 'is_junction', lambda: False)(), '不得使用链接：' + str(p))
    return p


def current(values, label):
    need(isinstance(values, dict) and values, label + '缺少输入集合')
    for name, digest in values.items():
        p = local(name)
        need(p.is_file() and sha(p) == digest, label + '当前字节不同：' + name)


def client_snapshot():
    paths = [p for folder in ['client/assets/scripts', 'client/tests'] for p in (ROOT / folder).rglob('*') if p.is_file()]
    paths += [p for pattern in ['*.scene', '*.prefab'] for p in (ROOT / 'client/assets').rglob(pattern)]
    paths += [ROOT / p for p in [PUBLIC, 'client/assets/resources/development/manifest.json', FIXTURE]]
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in sorted(set(paths))}


def external_evidence(report, prefix, allowed):
    before, after, saved = [report[prefix + key] for key in ['_before', '_after', '_current']]
    expected = {str(p.resolve()) for p in allowed}
    need(before == after == saved and {str(Path(p).resolve()) for p in saved} == expected,
         prefix + '不是完整独立白名单或前后不相同')
    for name, digest in saved.items():
        p = Path(name).resolve()
        need(p.is_file() and not p.is_symlink() and sha(p) == digest, prefix + '当前字节不同：' + name)
    return len(saved)


def node_counts(path):
    counts = {k: int(v) for k, v in re.findall(r'^(?:#|ℹ)\s+(tests|pass|fail|skipped|cancelled|todo)\s+(\d+)\s*$',
                                            path.read_text(encoding='utf-8'), re.M)}
    need(set(counts) == {'tests', 'pass', 'fail', 'skipped', 'cancelled', 'todo'}, '缺Node完整终态：' + str(path))
    need(counts['tests'] == counts['pass'] > 0 and all(counts[k] == 0 for k in ['fail', 'skipped', 'cancelled', 'todo']),
         'Node存在未通过节点：' + str(path))
    return counts


def json_output(path):
    text = path.read_text(encoding='utf-8')
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        for line in reversed(text.splitlines()):
            try:
                return json.loads(line)
            except json.JSONDecodeError:
                pass
    raise ValueError('缺命令JSON终态：' + str(path))


def client_evidence(path):
    need(path.is_relative_to(WORK / 'client') and path.name == 'report.json', 'client必须选本阶段明确终态')
    r = read(path)
    need(r['passed'] is True and r['stable'] is True and r['freeze_started_before_commands'] is True, 'client未冻结通过')
    need(type(r['input_count']) is int and r['input_count'] == len(r['before']) == len(r['after']) == len(r['current']) > 0,
         'client声明数量不等于明细集合')
    need(r['before'] == r['after'] == r['current'] == client_snapshot(), 'client前后/当前/完整重扫不同')
    engines = external_evidence(r, 'external_engine', [ENGINE / p for p in ENGINE_FILES])
    fonts = external_evidence(r, 'external_font', FONT_FILES)
    need(len(r['commands']) == len(COMMANDS) and {c['name'] for c in r['commands']} == COMMANDS, 'client检查组缺失或重复')
    logs = {}
    for c in r['commands']:
        p = local(c['log'])
        need(c['exit_code'] == 0 and sha(p) == c['sha256'], 'client命令/日志未通过：' + c['name'])
        logs[c['name']] = p
    app, scene = node_counts(logs['full-ui-app']), node_counts(logs['scene'])
    tasks = [json.loads(line) for line in logs['protocol-check'].read_text(encoding='utf-8').splitlines() if line.strip()]
    codec = [t for t in tasks if 'tools/protocol/verify_client_pb.cjs' in t['command']]
    need(len(codec) == 1 and all(t['exit_code'] == 0 for t in tasks), 'PB包装任务未通过')
    pb = json_output(local(codec[0]['output']))
    protocol = json_output(logs['protocol-lint'])
    need(pb['status'] == 'passed' and protocol['valid'] is True, 'PB边界或协议清单未通过')
    return r, {'app': app['pass'], 'scene': scene['pass'], 'total': app['pass'] + scene['pass'],
               'repo_inputs': r['input_count'], 'engine_inputs': engines, 'font_inputs': fonts,
               'pb_messages': pb['message_count'], 'pb_long_fields': pb['int64_field_count'],
               'pb_boundary_cases': pb['int64_boundary_cases'], 'pb_protocol_ids': protocol['messages']}, [local(t['output']) for t in tasks]


def go_evidence(path, events_path):
    r = read(path)
    need(r['passed'] is True and r['exit_code'] == 0 and r['mongo_e2e_explicit'] is True and r['race_detector'] is True,
         'Go尚未真实Mongo/race通过')
    need(all(s in r['command'] for s in ['./...', '-json', '-race']), 'Go不是完整全包JSON/race')
    need(r['source_changed_during_test'] is False and r['source_sha256'] == r['source_sha256_after'], 'Go输入前后不同')
    current(r['source_sha256'], 'Go')
    module = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='read_only_input_enumeration')
    need({p.relative_to(ROOT).as_posix() for p in module['input_files']()} == set(r['source_sha256']), 'Go输入集合不完整')
    need(sha(events_path) == r['events_sha256'], 'Go raw events摘要不同')
    events = [json.loads(line) for line in events_path.read_text(encoding='utf-8').splitlines() if line.strip()]
    passed = [{'package': e['Package'], 'test': e['Test']} for e in events if e.get('Action') == 'pass' and e.get('Test')]
    skipped = [{'package': e['Package'], 'test': e['Test']} for e in events if e.get('Action') == 'skip' and e.get('Test')]
    failed = [{'package': e['Package'], 'test': e.get('Test')} for e in events if e.get('Action') in {'fail', 'build-fail'}]
    need(passed == r['passed_tests'] and skipped == r['skipped_tests'] and failed == r['failed_tests'], 'Go报告不等于raw节点')
    need(passed and not skipped and not failed, 'Go存在失败/跳过')
    need(r['fixture']['sha256'] == sha(ROOT / FIXTURE), 'Go不是当前fixture')
    wrapper = read(path.parent / 'wrapper.json')
    need(wrapper['passed'] is True and wrapper['exit_code'] == 0 and wrapper['stable'] is True, 'Go wrapper未通过')
    for name, digest in wrapper['files_sha256'].items():
        need(sha(path.parent / name) == digest, 'Go wrapper保存字节不同：' + name)
    return r, {'passed': len(passed), 'skipped': len(skipped), 'failed': len(failed), 'inputs': len(r['source_sha256']),
               'raw_package_actions': dict(Counter(e['Action'] for e in events if not e.get('Test')))}


def formulas(path):
    with zipfile.ZipFile(path) as book:
        return {name: {c.attrib['r']: ET.tostring(c.find('s:f', NS), encoding='unicode')
                       for c in ET.fromstring(book.read(name)).findall('.//s:c', NS) if c.find('s:f', NS) is not None}
                for name in book.namelist() if name.startswith('xl/worksheets/') and name.endswith('.xml')}


def exp_member(book):
    relationship = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships'
    sheet = next(s for s in ET.fromstring(book.read('xl/workbook.xml')).findall('s:sheets/s:sheet', NS) if s.attrib['name'] == '逐级经验')
    target = next(n.attrib['Target'] for n in ET.fromstring(book.read('xl/_rels/workbook.xml.rels'))
                  if n.attrib['Id'] == sheet.attrib['{' + relationship + '}id'])
    return target.lstrip('/') if target.startswith('/') else 'xl/' + target


def configuration_evidence():
    sys.path.insert(0, str(ROOT / 'tools/config'))
    import export_configs as config
    review, baseline, candidate = [read(WORK / p) for p in ['final-review.json', 'baseline.json', 'candidate-review.json']]
    need(review['status'] == 'configuration_passed' and review['balance_verified'] is False and
         review['natural_level_1_to_50_verified'] is False, '配置口径错误或虚报平衡')
    current(review['current_hashes'], '配置冻结')
    old_path, new_path = WORK / 'before' / BOOK, ROOT / BOOK
    need(sha(old_path) == candidate['source_before_sha256'] == baseline['files'][BOOK] and
         sha(new_path) == candidate['candidate_sha256'], '唯一书前后字节不等')
    old, new = config.read_xlsx(old_path), config.read_xlsx(new_path)
    differences = []
    for sheet in sorted(set(old) | set(new)):
        for row in sorted(set(old.get(sheet, {})) | set(new.get(sheet, {}))):
            a, b = old.get(sheet, {}).get(row, {}), new.get(sheet, {}).get(row, {})
            for column in sorted(set(a) | set(b)):
                if a.get(column) != b.get(column):
                    differences.append({'sheet': sheet, 'row': row, 'column': column, 'before': a.get(column), 'after': b.get(column)})
    need(differences == candidate['differences'], '唯一书独立逐格diff与受审清单不同')
    coords = {(d['sheet'], d['row'], d['column']) for d in differences}
    expected = {('开发首循环夹具', 4, 3)} | {('逐级经验', row, column) for row in range(24, 54) for column in range(1, 9)}
    need(coords == expected and len(differences) == review['changed_cells'] == 241, '改动超出30来源经验行/profile一格')
    change = next(d for d in differences if d['sheet'] == '开发首循环夹具')
    old_profile, profile = json.loads(change['before']), json.loads(change['after'])
    normalized = deepcopy(profile)
    progression_profile = normalized.pop('progression')
    normalized['rule_version'] = 19
    need(old_profile['rule_version'] == 19 and profile['rule_version'] == 20 and normalized == old_profile, 'profile夹带其它业务变化')
    f1, f2 = formulas(old_path), formulas(new_path)
    need(f1 == f2 and sum(map(len, f1.values())) == review['formulas_preserved'] == 460, '460公式变化')
    with zipfile.ZipFile(old_path) as left, zipfile.ZipFile(new_path) as right:
        need(left.namelist() == right.namelist(), 'ZIP成员集合/顺序变化')
        changed = [n for n in left.namelist() if left.read(n) != right.read(n)]
        need(changed == candidate['changed_zip_members'] and len(changed) == 2 and
             len(left.namelist()) - len(changed) == review['untouched_zip_members'] == 101, '非编辑ZIP成员变化')
        a, b = left.read(exp_member(left)), right.read(exp_member(right))
        for row in range(4, 24):
            pattern = rb'<(?:[A-Za-z_][\w.-]*:)?row\b(?=[^>]*\br="' + str(row).encode() + rb'")[^>]*>.*?</(?:[A-Za-z_][\w.-]*:)?row>'
            x, y = re.search(pattern, a, re.S), re.search(pattern, b, re.S)
            need(x and y and x[0] == y[0] and hashlib.sha256(x[0]).hexdigest() == review['first_20_row_raw_sha256'][str(row)],
                 '原20行XML字节变化：' + str(row))
    need(sha(ROOT / ARCH) == baseline['files'][ARCH], '架构书变化')
    schema = config.load_json(ROOT / 'config/schema/catalog.json')
    tables, errors, _, _ = config.collect(schema)
    need(not errors, '当前配置表结构无效：' + str(errors))
    levels = tables['game_exp_levels']
    need([r['level'] for r in levels] == list(range(1, 51)), '经验不是连续1..50')
    source = read(WORK / 'source-audit.json')['source']
    copy = local(source['copy'])
    need(sha(copy) == source['sha256'] and copy.stat().st_nlink == 1, '独立Exp来源字节变化/不独立')
    lines = copy.read_text(encoding='gb18030').splitlines()
    for row in levels[20:]:
        level = row['level']
        need(lines[level + 2] == f"Level{level}={row['next_level_exp']}" and row['record_status'] == 'candidate' and
             row['measurement_version'] == 'myf_source_unbalanced' and f':{level + 3}' in row['source_ref'], '新增经验未精确对应Exp来源：' + str(level))
    fixture, public = read(ROOT / FIXTURE), read(ROOT / PUBLIC)
    need(fixture['progression'] == review['progression'] and
         fixture['progression']['exp_per_level'] == [r['next_level_exp'] for r in levels], 'fixture经验不是唯一表投影')
    need({k: v for k, v in fixture['progression'].items() if k != 'exp_per_level'} == public['progression'] and
         {k: fixture['progression'][k] for k in progression_profile} == progression_profile and len(public['experience']) == 50,
         '公开成长合同不同或平行阈值数组')
    before_fixture, before_public = read(WORK / 'before' / FIXTURE), read(WORK / 'before' / PUBLIC)
    a, b = deepcopy(before_fixture), deepcopy(fixture)
    a.pop('_source_evidence'); b.pop('_source_evidence'); b['rule_version'] = 19; b['progression'] = a['progression']
    need(a == b, 'fixture夹带其它业务值变化')
    b = deepcopy(public); b['rule_version'] = 19; b.pop('progression'); b['experience'] = before_public['experience']
    need(b == before_public, 'public夹带其它业务值变化')
    for name in ['pet-runtime-candidates.json', 'summon-candidates.json']:
        a, b = read(WORK / 'before/config/development' / name), read(ROOT / 'config/development' / name)
        a.pop('source_evidence'); b.pop('source_evidence')
        need(a == b and b['runtime_enabled'] is False, '独立候选夹带业务变化：' + name)
    for name in ['python-full', 'export-check', 'fixture-check', 'pet-check', 'summon-check']:
        command = read(WORK / (name + '.json'))
        need(command['exit_code'] == 0 and command['finished_at_utc'], '配置命令未终态通过：' + name)
    raw = (WORK / 'python-full.stdout.txt').read_text(encoding='utf-8') + (WORK / 'python-full.stderr.txt').read_text(encoding='utf-8')
    matched = re.search(r'^Ran (\d+) tests? in ', raw, re.M)
    need(matched and re.search(r'^OK\s*$', raw, re.M) and int(matched[1]) == review['python_tests'] == 120, 'Python raw终态不同')
    return fixture, public, review, {'changed_cells': len(differences), 'exp_rows_appended': 30, 'formulas': 460,
                                  'untouched_zip_members': 101, 'python_passed': int(matched[1]), 'architecture_preserved': True}


def preservation_evidence(review):
    baseline = read(WORK / 'baseline.json')
    layouts = baseline['saved_layouts']
    actual = {p.relative_to(ROOT).as_posix() for pattern in ['*.scene', '*.prefab'] for p in (ROOT / 'client/assets').rglob(pattern)}
    need(actual == set(layouts) and len(layouts) == 10, '保存布局集合变化')
    current(layouts, '全部10保存布局')
    source = review['previous_stage_asset_baseline']
    p = local(source['path'])
    need(sha(p) == source['sha256'], '旧资源基线字节不同')
    assets = {name: digest for name, digest in read(p)['assets'].items() if Path(name).suffix not in {'.scene', '.prefab'}}
    current(assets, '旧资源/地图/meta')
    need(len(assets) == review['old_resource_files_checked'] and review['old_resource_files_changed'] == [], '旧资源计数不等')
    return {'saved_layouts_preserved': len(layouts), 'old_resource_files_preserved': len(assets), 'layout_changes': 0}


def creator_evidence(path):
    r = read(path)
    need(r['creator'] == '3.8.8' and r['platform'] == 'web-mobile' and r['exit_code'] == 36 and r['success'] is True,
         'Creator尚未真实退出36通过')
    need(r['source_stable_during_build'] is True and r['scene_prefab_layout_preserved'] is True and
         type(r['seconds']) in (int, float) and r['seconds'] > 0, 'Creator输入/布局/耗时无效')
    current(r['asset_hashes'], 'Creator布局'); current(r['script_hashes'], 'Creator脚本')
    expected = {p.relative_to(ROOT).as_posix() for pattern in ['*.ts', '*.js'] for p in (ROOT / 'client/assets/scripts').rglob(pattern)}
    need(set(r['script_hashes']) == expected, 'Creator脚本集合不完整')
    saved_log = path.parent / 'creator.log'
    need(saved_log.is_file() and sha(saved_log) == sha(local(r['log'])), 'Creator日志未独立保留或不同')
    return r


def gui_evidence(path, build, fixture):
    need(path.is_relative_to(WORK) and path.name == 'report.json', 'GUI必须明确选择本阶段终态')
    r = read(path)
    need(r['passed'] is True and r['checks'] and r['errors'] == [], 'GUI未终态通过')
    need(r['seeded'] is False and r['privateBusinessInvoked'] is False and r['physicalMobileVerified'] is False,
         'GUI存在预置/私有业务或超出真机口径')
    frozen = {**build['asset_hashes'], **build['script_hashes']}
    need(r['sourceBefore'] == r['sourceAfter'] == frozen, 'GUI来源前后不同或不属于当前Creator布局/脚本')
    current(r['sourceBefore'], 'GUI来源')
    need(r['fixtureSha256'] == sha(ROOT / FIXTURE) and r['publicSha256'] == sha(ROOT / PUBLIC) and
         r['toolSha256'] == r['toolSha256After'] == sha(ROOT / 'tools/client/verify_level_growth_browser.cjs'), 'GUI工具前后/fixture/public不是当前')
    need({h['port'] for h in r['health']} == {18100, 18101, 18102} and len(r['health']) == 3 and
         all(h['status'] == 'ok' and h['development_only'] is True and h['rule_version'] == 20 and h['fixture_hash'] == r['fixtureSha256'] for h in r['health']),
         'GUI不是本项目3实例/当前fixture')
    need({h['port'] for h in r['healthAfter']} == {18100, 18101, 18102} and len(r['healthAfter']) == 3 and
         all(h['status'] == 'ok' and h['development_only'] is True and h['rule_version'] == 20 and h['fixture_hash'] == r['fixtureSha256'] for h in r['healthAfter']) and
         {h['port']: h['instance'] for h in r['healthAfter']} == {h['port']: h['instance'] for h in r['health']},
         'GUI结束health不是同3实例/当前fixture')
    observations, receipts = r['observations'], r['receipts']
    first, learned, final, before_reentry, reentry = [observations[k] for k in ['initial', 'learned', 'final', 'beforeReentry', 'reentry']]
    need(first['level'] == 1 and first['experience'] == '0' and first['growth']['format'] == 1 and
         first['growth']['catalog'] == first['growth']['cap'] == 50 and
         first['growth']['pool'] == first['growth']['legacy'] == '0' and first['growth']['rebirth'] == 0, 'GUI不是正常1级新角色')
    detail, scroll = observations['detail']['detail'], observations['detailScroll']
    need(isinstance(detail, list) and all(isinstance(line, str) for line in detail) and
         all(text in '\n'.join(detail) for text in ['当前上限 50', '待用经验', '转生尚未开放']), 'GUI保存成长详情缺上限/待用池/转生关闭说明')
    need(all(type(scroll[k]) in (int, float) and math.isfinite(scroll[k]) for k in ['before', 'after']) and
         scroll['after'] > scroll['before'] + 10, 'GUI真实拖动未推进保存ScrollView')
    need(any(s['id'] == 2 for s in learned['skills']) and final['level'] >= 22 and receipts and
         any(x['before'] == 21 and x['after'] >= 22 for x in receipts), '未实际学初始技能或跨越旧21断点')
    for row in receipts:
        need(int(row['raw']) == int(row['accepted']) + int(row['rejected']) and 0 <= int(row['pooled']) <= int(row['accepted']) and
             row['rejected'] == row['pooled'] == '0' and row['migrated'] is False and row['curve'] == fixture['progression']['curve_version'], 'GUI原奖励分账或迁移口径不符')
    ids = {(x['revision'], x['source']) for x in receipts}
    need(len(ids) == len(receipts), 'GUI通知重复')
    # final采样之后仍可有真实结算；只对该Revision已观测的奖励核final余额。
    earned_receipts = [x for x in receipts if int(x['revision']) <= int(final['revision'])]
    earned = sum(int(x['accepted']) for x in earned_receipts)
    need(earned == int(observations['earned']) == sum(fixture['progression']['exp_per_level'][:final['level'] - 1]) + int(final['experience']), 'GUI累计自然EXP不守恒')
    need(int(before_reentry['hp']) > 0 and reentry['binding'] != before_reentry['binding'] and reentry['life'] == before_reentry['life'] and
         reentry['level'] == before_reentry['level'] and reentry['experience'] == before_reentry['experience'] and
         reentry['growth'] == before_reentry['growth'] and reentry['notice'] is None, '健康重入改变成长或重播奖励')
    shots = sorted(path.parent.glob('*.png'))
    need(all((path.parent / name).is_file() for name in ['01-natural-level7.png', '02-normal-book-learn.png', '03-growth-details.png',
         '04-growth-details-scrolled.png', '05-natural-cross21.png', '06-level22-details.png', '07-healthy-reentry.png']), 'GUI缺正常路线截图')
    need(r['startedAt'] and r['finishedAt'], 'GUI缺实际起止')
    merge_stats = page_merge_evidence(r, fixture)
    return r, shots, {'receipts': len(receipts), 'receipts_at_final_revision': len(earned_receipts),
                      'final_revision': final['revision'], 'accepted_experience': earned, 'final_level': final['level'],
                      'final_experience': final['experience'], 'healthy_reentry': True, 'screenshots': len(shots), **merge_stats}


def page_merge_evidence(report, fixture):
    """独立复算真实报价和库存分组，不把工具自己的completed布尔当成业务证明。"""
    definitions = {row['id']: row for row in fixture['items']}
    records = report.get('pageMerges')
    need(isinstance(records, list) and records, 'GUI未实际完成正常同源书页合并')
    def key(item):
        return (item['item'], item['bound'], item['quality'], item['sourceKind'], item['sourceId'])
    def totals(items):
        result = {}
        for item in items:
            if definitions.get(item['item'], {}).get('is_skill_page'):
                group = json.dumps(key(item), ensure_ascii=False, separators=(',', ':'))
                result[group] = result.get(group, 0) + int(item['quantity'])
        return {group: str(value) for group, value in sorted(result.items())}
    freed, previews = 0, set()
    for record in records:
        need(record.get('completed') is True and record['quote']['kind'] == 4 and record['quote']['rule'] == '20', 'GUI合并未实际完成或不是kind4')
        before, after, quote = record['before'], record['after'], record['quote']
        need(quote['preview'] not in previews, '同一GUI合并报价重复计数'); previews.add(quote['preview'])
        need(all(before[name] == after[name] for name in ['binding', 'roleKey', 'scene', 'baseline', 'map', 'rule']) and
             int(after['revision']) > int(before['revision']), 'GUI合并角色/场景变化或未提交修订')
        old = {row['id']: row for row in before['items']}; new = {row['id']: row for row in after['items']}
        mats = quote['materials']; ids = [row['id'] for row in mats]
        need(len(ids) >= 2 and len(set(ids)) == len(ids) and set(ids) <= set(old), 'GUI合并原材料身份无效')
        source = key(mats[0]); need(source[3] in {'skill_recycle', 'npc_shop', 'monster_drop'} and source[4], 'GUI合并未知来源')
        for material in mats:
            original = old[material['id']]; definition = definitions.get(material['item'], {})
            need(definition.get('is_skill_page') is True and definition.get('stackable') is True and definition.get('max_stack', 0) > 1,
                 'GUI合并不是唯一配置允许的书页')
            need(key(original) == key(material) == source and material['version'] == original['version'] and
                 int(material['consume']) == int(original['quantity']) > 0 and original['locked'] is False and original['flags'] == 0,
                 'GUI合并混来源/绑定/品质、非全叠或保护物品')
        removed, added = set(old) - set(new), set(new) - set(old)
        need(removed == set(ids) == set(record['removedIds']) and added == set(record['newIds']), 'GUI合并实际增删与原材料不符')
        for item_id in set(old) & set(new):
            need(old[item_id] == new[item_id], 'GUI合并修改了其它原实例')
        actual = [new[item_id] for item_id in added]
        need(0 < len(actual) < len(ids) and len(actual) == len(quote['outputs']), 'GUI合并未减少实际格位')
        signature = lambda rows: sorted((key(row), int(row['quantity'])) for row in rows)
        need(signature(actual) == signature(quote['outputs']) == signature(record['actualOutputs']), 'GUI合并实际产物与报价不符')
        need(sum(int(row['quantity']) for row in actual) == sum(int(row['consume']) for row in mats), 'GUI合并数量不守恒')
        for output in actual:
            need(key(output) == source and output['locked'] is False and output['flags'] == 0 and
                 int(output['version']) == 1 and 0 < int(output['quantity']) <= definitions[output['item']]['max_stack'], 'GUI合并产物来源或容量不符')
        need(totals(before['items']) == totals(after['items']) == record['sourceTotalsBefore'] == record['sourceTotalsAfter'], 'GUI合并完整来源分组数量不守恒')
        need(all(before['wallet'][field] == after['wallet'][field] and quote['spend'][field] == '0'
                 for field in ['gold', 'boundGold', 'yuanbaoUnits', 'boundYuanbaoUnits']), 'GUI合并改变了货币')
        need(record['confirmationText']['title'] == '合并确认' and record['quoteLines'] and record['quoteScroll'] and
             record['quoteScroll'][-1]['offset'] >= record['quoteScroll'][-1]['max'] - 2, 'GUI合并未真实显示及滚读确认报价')
        slots = len(old) - len(new)
        need(slots == len(mats) - len(actual) == record['slotDifference'] > 0, 'GUI实际减格统计不符'); freed += slots
        need(record['cumulativeSlotDifference'] == freed, 'GUI累计减格不符')
    need(report['totalMergedSlotsFreed'] == freed, 'GUI合并释放总格数不符')
    return {'same_source_page_merges': len(records), 'merged_slots_freed': freed, 'page_source_totals_preserved': True}


def sync_evidence(preview):
    pointer = read(ROOT / 'config/releases/preview_current.json')
    need(pointer['version'] == preview, 'preview版本不匹配')
    sync = read(ROOT / 'conf/toolchain/config-binding-sync.json')
    need(sync['config_version'] == preview and sync['generated_only'] is True, '类型绑定未同步当前preview')
    release = ROOT / 'config/releases' / preview
    for generated, name in [(sync['server'], 'config_types.go'), (sync['client'], 'config_types.ts')]:
        need(sha(local(generated)) == sha(release / name), '生成类型未对应preview：' + generated)
    return {'version': preview, 'generated_only': True, 'fixture_hash_not_part_of_sync_record': True,
            'fixture_sha256_checked_separately': sha(ROOT / FIXTURE), 'public_sha256_checked_separately': sha(ROOT / PUBLIC)}


class Archive:
    """按明确文件归档；大文本gzip，不复制工程/缓存/字体/整套输入树。"""
    def __init__(self):
        self.jobs = {}

    def add(self, source, target, scope, core=False):
        source = local(source)
        need(source.is_file() and source.suffix.lower() not in {'.exe', '.dll', '.lib', '.wil', '.wix', '.ttf', '.ttc'}, '拒绝缺文件/原包/字体二进制')
        destination = Path(target)
        need(not destination.is_absolute() and '..' not in destination.parts and not set(destination.parts) & FORBIDDEN, '归档目标越界')
        name = source.relative_to(ROOT).as_posix()
        if any(job['source'] == name for job in self.jobs.values()):
            return
        compress = not core and source.suffix.lower() in {'.json', '.jsonl', '.txt', '.log', '.py', '.cjs'} and source.stat().st_size >= 128 * 1024
        if source.suffix == '.jsonl':
            compress = True
        if compress:
            destination = destination.with_name(destination.name + '.gz')
        target = destination.as_posix()
        need(target not in self.jobs, '归档目标重复')
        self.jobs[target] = {'source': name, 'target': target, 'scope': scope, 'encoding': 'gzip' if compress else 'identity',
                             'raw_sha256': sha(source), 'raw_bytes': source.stat().st_size}

    def shallow(self, folder, target, scope, names):
        for name in names:
            p = folder / name
            if p.is_file():
                self.add(p, Path(target) / name, scope, core=name == 'report.json')

    def apply(self):
        OUT.mkdir(parents=True, exist_ok=False)
        records = []
        for job in self.jobs.values():
            source = ROOT / job['source']; need(sha(source) == job['raw_sha256'], '归档前源变化')
            target = OUT / job['target']; target.parent.mkdir(parents=True, exist_ok=True)
            if job['encoding'] == 'gzip':
                with source.open('rb') as src, target.open('xb') as dst, gzip.GzipFile(filename='', mode='wb', fileobj=dst, mtime=0) as stream:
                    shutil.copyfileobj(src, stream)
                check = hashlib.sha256()
                with gzip.open(target, 'rb') as stream:
                    for block in iter(lambda: stream.read(1024 * 1024), b''):
                        check.update(block)
                need(check.hexdigest() == job['raw_sha256'], 'gzip原字节不等')
            else:
                shutil.copyfile(source, target); need(sha(target) == job['raw_sha256'], '归档字节不同')
            records.append({**job, 'file_sha256': sha(target), 'file_bytes': target.stat().st_size})
        return records


def curate(args, client, protocol_outputs, shots):
    archive, history = Archive(), []
    for source, target in [(args.go_report, 'go/report.json'), (args.go_events, 'go/events.jsonl'),
                           (args.client_report, 'client/report.json'), (args.creator_report, 'creator/report.json'),
                           (args.creator_report.parent / 'creator.log', 'creator/creator.log'), (args.gui_report, 'gui/report.json')]:
        archive.add(source, 'current/' + target, 'current', core=target.endswith('report.json'))
    archive.shallow(args.go_report.parent, 'current/go', 'current', ['wrapper.json', 'tool.stdout.txt', 'tool.stderr.txt'])
    archive.shallow(args.gui_report.parent, 'current/gui', 'current', ['stdout.log', 'stderr.log', 'wrapper-start.json', 'wrapper-result.json', 'run.py', 'visual-review.json'])
    for c in client['commands']:
        archive.add(local(c['log']), 'current/client/' + Path(c['log']).name, 'current')
    for p in protocol_outputs:
        archive.add(p, 'current/client/' + p.name, 'current_protocol_artifact')
    for p in shots:
        archive.add(p, 'current/gui/' + p.name, 'current')
    names = ['source-audit.json', 'baseline.json', 'edit-plan.json', 'candidate-review.json', 'editor-review.json', 'application.json',
             'final-review.json', 'same-source-page-merge-review.json', 'qa-mask-geometry-source.json', 'author_configuration.py', 'update_schema.py', 'verify_configuration.py', 'run_checks.py']
    for prefix in ['narrow', 'python-full', 'export', 'export-check', 'fixture-generate', 'fixture-check', 'pet-generate', 'pet-check', 'summon-generate', 'summon-check']:
        names += [prefix + suffix for suffix in ['.json', '.stdout.txt', '.stderr.txt']]
    archive.shallow(WORK, 'current/config', 'configuration_or_before_source_audit', names)
    archive.add(WORK / 'before' / BOOK, 'before/玩法配置.xlsx', 'before_unique_workbook')
    for folder in ['config-peer', 'server-peer']:
        archive.shallow(WORK / folder, 'current/' + folder, 'peer_review_scope_as_recorded', ['review.json', 'review.md'])
    older_peer = WORK / 'server-peer/review-before-merge.json'
    if older_peer.is_file():
        archive.add(older_peer, 'history/server-peer/review-before-merge.json', 'historical_peer_before_merge', core=True)
    archive.shallow(WORK / 'client/validation-3/protocol-artifacts', 'history/client/validation-3/protocol-artifacts',
                    'historical_task_artifacts_saved_before_new_validation', ['protocol-check-0.txt', 'protocol-check-1.txt', 'saved-before-validation4.json'])
    archive.shallow(WORK / 'page-merge', 'current/page-merge', 'author_narrow_scope_as_recorded', ['report.json', 'go-frozen.json'])
    candidates = set(args.history_report)
    candidates |= {p.resolve() for p in (WORK / 'server').glob('full-*/report.json') if p.resolve() != args.go_report}
    candidates |= {p.resolve() for p in (WORK / 'client').glob('*/report.json') if p.resolve() != args.client_report}
    candidates |= {p.resolve() for p in (WORK / 'browser').glob('run-*/report.json') if p.resolve() != args.gui_report}
    candidates |= {p.resolve() for p in WORK.glob('creator*/report.json') if p.resolve() != args.creator_report}
    for p in sorted(candidates):
        need(p.is_relative_to(WORK), '历史必须明确选择本阶段路径')
        r = read(p)
        history.append({'path': p.relative_to(ROOT).as_posix(), 'sha256': sha(p), 'passed_at_that_time': r.get('passed'), 'scope': 'historical_not_current'})
        prefix = Path('history') / p.parent.relative_to(WORK)
        archive.add(p, prefix / p.name, 'historical_not_current', core=True)
        if p.parent.parent == WORK / 'server':
            if 'events_sha256' in r:
                need(sha(p.parent / 'events.jsonl') == r['events_sha256'], '历史Go raw不同')
            archive.shallow(p.parent, prefix, 'historical_go', ['events.jsonl', 'wrapper.json', 'tool.stdout.txt', 'tool.stderr.txt'])
        if p.parent.parent == WORK / 'browser':
            archive.shallow(p.parent, prefix, 'historical_browser_attempt', ['stdout.log', 'stderr.log', 'wrapper-start.json', 'wrapper-result.json', 'run.py', 'progress.json', 'tool-at-start.cjs', 'stop-owned-chrome.ps1', 'stop-owned-processes-before.json', 'controlled-stop.json'])
        if r.get('creator') == '3.8.8' and r.get('platform') == 'web-mobile':
            archive.shallow(p.parent, prefix, 'historical_creator', ['creator.log'])
        for c in r.get('commands', []):
            if 'log' in c:
                old_log = local(c['log'])
            else:
                digest = c.get('log_sha256')
                matches = [candidate for candidate in p.parent.iterdir() if candidate.is_file()
                           and candidate.suffix in {'.txt', '.log'} and digest and sha(candidate) == digest]
                need(len(matches) == 1, '历史探索日志未能按原摘要唯一定位：' + str(p))
                old_log = matches[0]
            archive.add(old_log, prefix / old_log.name, 'historical_client')
        for image in sorted(p.parent.glob('*.png')):
            archive.add(image, prefix / image.name, 'historical_screenshot')
    for p in [ROOT / 'shared/proto/assets.proto', ROOT / 'shared/proto/protocol_manifest.json',
              ROOT / 'tools/client/verify_level_growth_browser.cjs', Path(__file__), WORK / 'update_docs.py', WORK / 'verify_index.py']:
        archive.add(p, Path('sources') / p.relative_to(ROOT), 'source_at_archive')
    return archive, history


def write_new(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open('x', encoding='utf-8', newline='\n') as stream:
        stream.write(json.dumps(value, ensure_ascii=False, indent=2) + '\n')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go-report', type=Path, default=WORK / 'server/full-3/report.json')
    parser.add_argument('--go-events', type=Path, default=WORK / 'server/full-3/events.jsonl')
    parser.add_argument('--client-report', type=Path, default=WORK / 'client/validation-4/report.json')
    parser.add_argument('--creator-report', type=Path, default=WORK / 'creator-merge/report.json')
    parser.add_argument('--gui-report', type=Path, required=True)
    parser.add_argument('--history-report', type=Path, action='append', default=[])
    mode = parser.add_mutually_exclusive_group()
    mode.add_argument('--check', action='store_true', help='默认模式：完成所有门禁但不写docs/outputs')
    mode.add_argument('--apply', action='store_true', help='仅Root审阅终态后明确使用；不可覆盖既有正式产物')
    args = parser.parse_args()
    for name in ['go_report', 'go_events', 'client_report', 'creator_report', 'gui_report']:
        setattr(args, name, local(getattr(args, name)))
    args.history_report = [local(p) for p in args.history_report]
    fixture, public, config, cc = configuration_evidence()
    need(fixture['rule_version'] == 20 and len(fixture['skills']) == 22 and all(s['id'] != 17 for s in fixture['skills']) and
         fixture['casting_supply']['enabled'] is False and fixture['pet_runtime']['bootstrap_mode'] == 'legacy_untrained_pet', '主范围门禁变化')
    go, gc = go_evidence(args.go_report, args.go_events)
    client, js, protocols = client_evidence(args.client_report)
    build = creator_evidence(args.creator_report)
    preservation = preservation_evidence(config)
    gui, shots, observation = gui_evidence(args.gui_report, build, fixture)
    sync = sync_evidence(config['preview'])
    archive, history = curate(args, client, protocols, shots)
    validation = read(ROOT / 'config/reports/配置校验报告.json')
    need(validation['structure_valid'] is True and validation['runtime_ready'] is False and validation['release_blockers'], '正式发布门禁被移除')
    report = {'schema_version': 1, 'started_date': '2026-10-29', 'date': datetime.now(timezone(timedelta(hours=8))).date().isoformat(), 'recorded_at_utc': datetime.now(timezone.utc).isoformat(),
              'stage': '自然成长、待用经验与同源书页合并', 'status': 'limited_passed',
              'scope': '来源MYF未平衡经验候选与永久入账/旧余额迁移/原身体升级恢复；正常主UI学习、同源书页报价合并清包及自然跨21至22，不认证自然1至50节奏或转生业务',
              'runtime': {'rule_version': fixture['rule_version'], 'preview': config['preview'], 'fixture_sha256': sha(ROOT / FIXTURE), 'public_sha256': sha(ROOT / PUBLIC),
                          'skills': len(fixture['skills']), 'summon17_enabled': False, 'casting_supply_enabled': False,
                          'bootstrap_mode': fixture['pet_runtime']['bootstrap_mode'], 'legacy_pet_trains': False, 'progression': public['progression'],
                          'experience_catalog_rows': len(public['experience']), 'rebirth_enabled': False, 'table_count': validation['table_count'],
                          'record_count': validation['record_count'], 'release_blockers': len(validation['release_blockers']), 'runtime_ready': False, 'production_ready': False},
              'verification': {'go_full_mongo_ws_race_passed_including_subtests': gc['passed'], 'go_failed': gc['failed'], 'go_skipped': gc['skipped'], 'go_inputs': gc['inputs'], 'go_source_stable': True,
                               'configuration_python_passed': cc['python_passed'], 'javascript_ui_app_passed': js['app'], 'javascript_scene_passed': js['scene'], 'javascript_total_passed': js['total'],
                               'client_inputs': js['repo_inputs'], 'client_external_engine_inputs': js['engine_inputs'], 'client_external_font_inputs': js['font_inputs'],
                               **{k: js[k] for k in ['pb_messages', 'pb_long_fields', 'pb_boundary_cases', 'pb_protocol_ids']},
                               'creator_seconds': build['seconds'], 'creator_exit_code': build['exit_code'], 'gameplay_excel_changed_cells': cc['changed_cells'],
                               'exp_rows_appended': cc['exp_rows_appended'], 'formulas_preserved': cc['formulas'], 'untouched_zip_members': cc['untouched_zip_members'],
                               'architecture_workbook_preserved': True, **preservation},
              'main_creator': {'passed': build['success'], 'seconds': build['seconds'], 'exit_code': build['exit_code'], 'source_stable': build['source_stable_during_build']},
              'gui': {'passed': gui['passed'], 'checks': gui['checks'], 'errors': gui['errors'], 'warnings': gui['warnings'], 'observed': observation,
                      'seeded': False, 'privateBusinessInvoked': False, 'scope': gui['scope']},
              'type_binding_sync': sync, 'raw_go_counts': gc, 'historical_attempts': history,
              'browser_report': args.gui_report.relative_to(ROOT).as_posix(),
              'go_report': args.go_report.relative_to(ROOT).as_posix(),
              'client_report': args.client_report.relative_to(ROOT).as_posix(),
              'creator_report': args.creator_report.relative_to(ROOT).as_posix(),
              'selectors': {k: {'path': getattr(args, k).relative_to(ROOT).as_posix(), 'sha256': sha(getattr(args, k))}
                            for k in ['go_report', 'go_events', 'client_report', 'creator_report', 'gui_report']},
              'limits': ['MYF1至50阈值是未平衡来源候选，现怪物仅15EXP；约794日仅算术诊断，不是新版升级周期验收',
                         '50级阈值只作为one_next_level待用池上限，不自动开放51或转生条件/费用',
                         '迁移与新奖励分账，曲线不符冻结旧余额并拒收新EXP；迁移本身不补原身体HP/MP',
                         '主22技能/legacy不训练/材料false/17关闭保持；护符包Count及售价仍未审定',
                         '50/400仍是development_gold_only，outputs=[]；正式洗炼石与祝福油、损坏回收未验收',
                         'kind4仅手动合并同一来源ID及完整属性等价的已配置书页，不合并不同来源、不可叠书/装备或未知数量护符；65535为既有开发计数候选，非MYF包规格',
                         '现有普通满包还会额外停自动战斗/托管，尚未完全对齐4.2只停止拾取或弃新增掉落；本阶段使用普通清包验证成长，不认证完整满包挂机策略，下一阶段优先修复',
                         '响应丢弃后原ID重试不等于真实UnknownTransaction网络故障注入；未验手机、容量、完整P0或全1.80'],
              'balance_verified': False, 'natural_level_1_to_50_verified': False, 'ordinary_full_bag_policy_fully_aligned': False, 'physical_mobile_verified': False, 'capacity_verified': False, 'production_ready': False}
    current(go['source_sha256'], '结束前Go'); need(client_snapshot() == client['current'], '结束前client变化')
    external_evidence(client, 'external_engine', [ENGINE / p for p in ENGINE_FILES]); external_evidence(client, 'external_font', FONT_FILES)
    for job in archive.jobs.values():
        need(sha(ROOT / job['source']) == job['raw_sha256'], '结束前归档来源变化')
    summary = {'preflight': 'passed', 'archive_executed': False, 'formal_report_written': False, 'go': gc['passed'], 'go_inputs': gc['inputs'],
               'javascript': js['total'], 'client_inputs': js['repo_inputs'], 'engine_inputs': js['engine_inputs'], 'font_inputs': js['font_inputs'],
               'python': cc['python_passed'], 'gui_checks': len(gui['checks']), 'archive_files_planned': len(archive.jobs)}
    if not args.apply:
        print(json.dumps(summary, ensure_ascii=False)); return
    need(not OUT.exists() and not FORMAL.exists(), '正式产物已存在，拒绝覆盖')
    records = archive.apply()
    write_new(OUT / 'evidence-manifest.json', {'schema_version': 1, 'scope': '明确文件白名单，gzip保存原字节；不递归工程/缓存/整套inputs', 'files': records})
    write_new(OUT / 'evidence-paths.json', {r['source']: {'path': (OUT / r['target']).relative_to(ROOT).as_posix(), 'encoding': r['encoding'],
                                                        'raw_sha256': r['raw_sha256'], 'file_sha256': r['file_sha256']} for r in records})
    with (OUT / 'README.md').open('x', encoding='utf-8', newline='\n') as stream:
        stream.write('# 等级成长限定证据\n\ncurrent只收终态通过且当前输入吻合的完整Mongo/WS race、分组client、Creator与正常新角色UI证据。history保留当时原status，不改失败或追认通过。大日志gzip无损归档，manifest分别记raw/file SHA与字节数，不复制工程、缓存、字体、可执行文件或整套输入。\n\nMYF1至50阈值仍为未平衡来源候选；正常UI只证明本报告实际跨21至22。约794日算术不等于新版周期；51/转生、正式洗炼石与祝福油、损坏回收、手机、容量、完整P0与全1.80均未验收。主22技能/legacy不训练/护符false/17关闭保持。\n')
    report['evidence_sha256'] = {(OUT / r['target']).relative_to(ROOT).as_posix(): r['file_sha256'] for r in records}
    for name in ['README.md', 'evidence-manifest.json', 'evidence-paths.json']:
        report['evidence_sha256'][(OUT / name).relative_to(ROOT).as_posix()] = sha(OUT / name)
    report['evidence_file_count'] = len(report['evidence_sha256'])
    current(go['source_sha256'], '归档后Go'); need(client_snapshot() == client['current'], '归档后client变化')
    write_new(FORMAL, report)
    print(json.dumps({**summary, 'archive_executed': True, 'formal_report_written': True, 'formal_report': str(FORMAL)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
