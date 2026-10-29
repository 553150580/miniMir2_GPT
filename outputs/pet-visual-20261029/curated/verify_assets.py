"""核对受控资源增量和旧PNG/地图/布局原字节保留。"""
import copy
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).resolve().parent
sha = lambda path: hashlib.sha256(path.read_bytes()).hexdigest()
before = json.loads((OUT / 'before.json').read_text(encoding='utf-8'))
old = json.loads((OUT / 'manifest-before.json').read_text(encoding='utf-8'))
path = ROOT / 'client/assets/resources/development/manifest.json'
new = json.loads(path.read_text(encoding='utf-8'))
assert set(new['frames']) - set(old['frames']) == {f'Magic:{i}' for i in range(1500, 1510)} | {f'Mon4:{i}' for i in range(2860, 2870)}
assert all(new['frames'][key] == value for key, value in old['frames'].items())
assert all(new['effects'][key] == value for key, value in old['effects'].items())
assert set(new['effects']) - set(old['effects']) == {'17'}
profiles = copy.deepcopy(new['profiles'])
assert 'appear' not in old['profiles']['pet_skeleton']['layers'][0]['actions']
profiles['pet_skeleton']['layers'][0]['actions'].pop('appear')
assert profiles == old['profiles']
for key in old:
    if key not in {'converter_sha256', 'profiles', 'effects', 'frames'}:
        assert new[key] == old[key], key
unchanged = {'png': 0, 'map_files': 0, 'saved_layout': 0, 'resource_meta': 0}
changed = []
for relative, expected in before['files'].items():
    source = ROOT / relative
    if sha(source) != expected:
        changed.append(relative)
    if source.suffix == '.png' or '/maps/' in relative or '/source-maps/' in relative or source.suffix in {'.scene', '.prefab', '.meta'}:
        assert sha(source) == expected, relative
        unchanged['png'] += source.suffix == '.png'
        unchanged['map_files'] += '/maps/' in relative or '/source-maps/' in relative
        unchanged['saved_layout'] += source.suffix in {'.scene', '.prefab'}
        unchanged['resource_meta'] += source.suffix == '.meta'
added = sorted(p.relative_to(ROOT).as_posix() for p in (ROOT / 'client/assets/resources').rglob('*') if p.is_file() and p.relative_to(ROOT).as_posix() not in before['files'])
assert len([p for p in added if p.endswith('.png')]) == 20
assert len([p for p in added if p.endswith('.wav')]) == 2
audio = new['audio']
for entry in audio.values():
    source = ROOT / entry['source']
    generated = ROOT / 'client/assets/resources/development' / entry['path']
    assert source.read_bytes() == generated.read_bytes()
    assert sha(generated) == entry['sha256']
final = ['tools/resources/build_development_assets.py', 'tools/resources/pet_birth_assets.py', 'tools/resources/test_pet_birth_assets.py', '资料索引/宠物出生表现来源契约.json', 'outputs/development-prep-20261026/玩法配置.xlsx', 'outputs/development-prep-20261026/架构配置.xlsx', 'config/schema/catalog.json', 'client/assets/resources/development/manifest.json']
final += [p.relative_to(ROOT).as_posix() for p in (ROOT / '资料参考/crystal-supplement/assets').rglob('*') if p.is_file()]
result = {'old_frame_records_preserved': len(old['frames']), 'new_frame_records': len(new['frames']) - len(old['frames']), 'frame_count': len(new['frames']), 'new_audio': len(audio), 'old_effects_preserved': len(old['effects']), 'other_profiles_and_original_pet_actions_unchanged': True, 'unchanged_old_files': unchanged, 'added_resources': added, 'changed_tracked_baseline_paths': changed, 'current_hashes': {name: sha(ROOT / name) for name in final}, 'creator_import_meta': '未猜造meta；待Root统一Creator3.8.8真实导入生成后另验', 'claims': '仅来源/生成/字节保留核验；无Creator/GPU/音频播放/UI/业务/容量完成声明'}
(OUT / 'resource-diff.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps(result, ensure_ascii=False))
