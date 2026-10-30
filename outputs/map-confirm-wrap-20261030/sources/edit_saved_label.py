"""仅修改已保存Confirm正文的换行开关，保留其它原字节。"""
from pathlib import Path
import hashlib
import json
import subprocess

WORK = Path(__file__).resolve().parent
ROOT = WORK.parents[1]
path = ROOT / 'client/assets/scenes/Game.scene'
raw = path.read_bytes()
assert subprocess.check_output(['git', 'show', 'HEAD:client/assets/scenes/Game.scene'], cwd=ROOT) == raw
text = raw.decode('utf-8')
decoder = json.JSONDecoder()
cursor, spans, items = text.index('[') + 1, [], []
while True:
    while text[cursor].isspace() or text[cursor] == ',':
        cursor += 1
    if text[cursor] == ']':
        break
    start = cursor
    item, cursor = decoder.raw_decode(text, cursor)
    spans.append((start, cursor))
    items.append(item)
node = items[items[0]['scene']['__id__']]
for name in ['Canvas', 'Safe', 'PopupView', 'Panels', 'Confirm', 'Message']:
    ref = next(r['__id__'] for r in node['_children'] if items[r['__id__']].get('_name') == name)
    node = items[ref]
label_id = next(r['__id__'] for r in node['_components'] if items[r['__id__']]['__type__'] == 'cc.Label')
assert label_id == 483 and items[label_id]['_enableWrapText'] is False and items[label_id]['_fontSize'] == 24
start, end = spans[label_id]
old = text[start:end]
needle = '"_enableWrapText": false'
assert old.count(needle) == 1
updated = text[:start] + old.replace(needle, '"_enableWrapText": true') + text[end:]
after = json.loads(updated)
expected = json.loads(text)
expected[label_id]['_enableWrapText'] = True
assert after == expected
before = WORK / 'before/Game.scene'
before.parent.mkdir(exist_ok=False)
before.write_bytes(raw)
path.write_bytes(updated.encode('utf-8'))
report = {'base': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
          'path': path.relative_to(ROOT).as_posix(), 'node_path': 'Canvas/Safe/PopupView/Panels/Confirm/Message',
          'object_index': label_id, 'only_field': '_enableWrapText', 'before': False, 'after': True,
          'before_sha256': hashlib.sha256(raw).hexdigest(), 'after_sha256': hashlib.sha256(path.read_bytes()).hexdigest(),
          'other_object_fields_and_raw_text_preserved': True, 'font_size': 24, 'logic_changed': False}
(WORK / 'edit-report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
for name in ['stage_checks.py', 'build_candidate.py']:
    source = ROOT / '.work/warehouses-20261030' / name
    content = source.read_bytes()
    if name == 'stage_checks.py':
        content = content.replace('保留阶段23真实检查'.encode(), '保留确认窗换行修复真实检查'.encode())
    (WORK / name).write_bytes(content)
print(json.dumps(report, ensure_ascii=False))
