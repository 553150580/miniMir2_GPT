"""只读核对Creator导入后的旧资产保留与真实新meta/cache，不执行构建。"""
import hashlib
import json
from pathlib import Path
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parents[2]
OUT = Path(__file__).resolve().parent


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def load(path):
    return json.loads(path.read_text(encoding='utf-8'))


before = load(OUT / 'before.json')['files']
build_path = ROOT / 'docs/客户端构建检查.json'
build = load(build_path)
assert build['creator'] == '3.8.8' and build['success'] and build['exit_code'] == 36
assert build['scene_prefab_layout_preserved'] and build['source_stable_during_build']
counts = dict(png=0, map_files=0, saved_layout=0, resource_meta=0)
changed = []
for relative, expected in before.items():
    p = ROOT / relative
    current = sha(p)
    if current != expected:
        changed.append({'path': relative, 'before_sha256': expected, 'current_sha256': current})
    categories = []
    if p.suffix == '.png': categories.append('png')
    if '/maps/' in relative or '/source-maps/' in relative: categories.append('map_files')
    if p.suffix in {'.scene', '.prefab'}: categories.append('saved_layout')
    if p.suffix == '.meta': categories.append('resource_meta')
    if categories:
        assert current == expected, '旧资源/布局改变：' + relative
    for category in categories:
        counts[category] += 1
assert counts == {'png': 7998, 'map_files': 13158, 'saved_layout': 10, 'resource_meta': 9972}

resource_root = ROOT / 'client/assets/resources'
added = {p.relative_to(ROOT).as_posix(): p for p in resource_root.rglob('*') if p.is_file() and p.relative_to(ROOT).as_posix() not in before}
pngs = {f'client/assets/resources/development/frames/{package}/frame_{index:06d}.png' for package, first in [('Magic', 1500), ('Mon4', 2860)] for index in range(first, first + 10)}
wavs = {f'client/assets/resources/development/audio/{name}.wav' for name in ['summon_skeleton_cast', 'pet_skeleton_born']}
new_meta = {name + '.meta' for name in pngs | wavs} | {'client/assets/resources/development/audio.meta'}
unknown = sorted(set(added) - pngs - wavs - new_meta)
missing = sorted((pngs | wavs | new_meta) - set(added))
assert not missing
assert not unknown, '发现需单独说明的新资源：' + str(unknown)
manifest_path = ROOT / 'client/assets/resources/development/manifest.json'
manifest = load(manifest_path)
old_manifest = load(OUT / 'manifest-before.json')
assert len(manifest['frames']) == 3352
assert all(manifest['frames'][key] == value for key, value in old_manifest['frames'].items())
assert all(manifest['effects'][key] == value for key, value in old_manifest['effects'].items())
metas = []
for relative in sorted(new_meta):
    p = ROOT / relative
    meta = load(p)
    assert meta['imported'] is True
    record = {'path': relative, 'bytes': p.stat().st_size, 'sha256': sha(p), 'created_utc': datetime.fromtimestamp(p.stat().st_ctime, timezone.utc).isoformat(), 'modified_utc': datetime.fromtimestamp(p.stat().st_mtime, timezone.utc).isoformat(), 'importer': meta['importer'], 'version': meta['ver'], 'uuid': meta['uuid']}
    if relative.endswith('.wav.meta'):
        assert meta['importer'] == 'audio-clip' and meta['ver'] == '1.0.0'
        assert meta['files'] == ['.json', '.wav'] and meta['userData'] == {'downloadMode': 0}
        cache = ROOT / 'client/library' / meta['uuid'][:2]
        imported_json = cache / (meta['uuid'] + '.json')
        native = cache / (meta['uuid'] + '.wav')
        clip = load(imported_json)
        assert clip['__type__'] == 'cc.AudioClip' and clip['_native'] == '.wav'
        assert sha(native) == sha(ROOT / relative.removesuffix('.meta'))
        record.update(imported_asset={'path': imported_json.relative_to(ROOT).as_posix(), 'sha256': sha(imported_json), 'serialized_type': clip['__type__'], 'name': clip['_name'], 'duration_seconds': clip['_duration']}, imported_native={'path': native.relative_to(ROOT).as_posix(), 'sha256': sha(native), 'source_wav_byte_equal': True}, user_data=meta['userData'])
    elif relative.endswith('.png.meta'):
        assert meta['importer'] == 'image' and meta['ver'] == '1.0.27'
        assert any(s['importer'] == 'sprite-frame' and s['imported'] is True for s in meta['subMetas'].values())
        record['sub_importers'] = {key: {'importer': sub['importer'], 'version': sub['ver']} for key, sub in meta['subMetas'].items()}
    else:
        assert meta['importer'] == 'directory'
    metas.append(record)

for entry in manifest['audio'].values():
    original = ROOT / entry['source']
    generated = ROOT / 'client/assets/resources/development' / entry['path']
    assert sha(original) == sha(generated) == entry['sha256']

result = {'scope': 'post-Creator只读SHA及实际导入cache核验；未运行Creator/Go/DB', 'checked_at_utc': datetime.now(timezone.utc).isoformat(), 'old_files_unchanged': counts, 'old_frame_records_unchanged': len(old_manifest['frames']), 'old_effect_records_unchanged': len(old_manifest['effects']), 'current_frame_count': len(manifest['frames']), 'new_pngs': len(pngs), 'new_wavs': len(wavs), 'new_meta_count': len(new_meta), 'new_meta': metas, 'unknown_new_resources': unknown, 'missing_expected_resources': missing, 'added_resource_sha256': {name: sha(p) for name, p in sorted(added.items())}, 'changed_baseline_non_resource_paths': changed, 'manifest_sha256': sha(manifest_path), 'creator_evidence': {'report': build_path.relative_to(ROOT).as_posix(), 'report_sha256': sha(build_path), 'creator': build['creator'], 'exit_code': build['exit_code'], 'success': build['success'], 'seconds': build['seconds'], 'source_stable_during_build': build['source_stable_during_build'], 'scene_prefab_layout_preserved': build['scene_prefab_layout_preserved'], 'log': build['log'], 'log_sha256': sha(Path(build['log']))}, 'meta_provenance': 'Root实际Creator3.8.8导入；before中无这23项，新meta的UUID与实际AudioClip导入cache/原字节native匹配。没有手写meta或将新meta算成旧meta未变。', 'claim_limit': '本次只复核文件/真实导入痕迹，不据此宣称GPU/音频/UI/业务/容量已通过'}
(OUT / 'resource-diff-post-creator.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(json.dumps({'old_unchanged': counts, 'new_pngs': len(pngs), 'new_wavs': len(wavs), 'new_metas': len(new_meta), 'unknown_new': unknown, 'manifest_sha256': result['manifest_sha256'], 'creator_seconds': build['seconds']}, ensure_ascii=False))
