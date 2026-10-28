"""仅白名单静态源码核对；独立复制Globals并登记技术片段，不读任何来源运行配置。"""
from pathlib import Path
import difflib
import hashlib
import json
import shutil

ROOT = Path(__file__).resolve().parents[2]
BASE = Path(__file__).resolve().parent
LATEST = Path('D:/123DownLoad/CrystalLatest')
LOCAL = Path('D:/123DownLoad/CrystalLocal')
RELS = ['Server/MirObjects/HumanObject.cs', 'Server/MirObjects/PlayerObject.cs',
        'Server/MirObjects/MonsterObject.cs', 'Server/MirObjects/MapObject.cs',
        'Server/MirEnvir/Map.cs', 'Server/MirEnvir/Envir.cs',
        'Server/MirDatabase/MagicInfo.cs', 'Server/MirDatabase/BuffInfo.cs',
        'Server/MirObjects/Monsters/BoneFamiliar.cs']

def sha(data):
    return hashlib.sha256(data).hexdigest()

def save(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')

def lines(path):
    return path.read_text(encoding='utf-8-sig').splitlines()

def member(path, signature):
    source = lines(path)
    start = next(i for i, text in enumerate(source) if signature in text)
    depth, opened = 0, False
    for end in range(start, len(source)):
        line = source[end]
        if '{' in line:
            opened = True
        depth += line.count('{') - line.count('}')
        if opened and depth == 0:
            return start + 1, end + 1, '\n'.join(source[start:end+1])
    raise ValueError(signature)

def main():
    roots = {'windows_latest': LATEST, 'windows_local': LOCAL}
    frozen_paths = [ROOT/'资料参考/crystal'/rel for rel in RELS]
    frozen_paths += [root/rel for root in roots.values() for rel in RELS]
    frozen_paths += [root/rel for root in roots.values() for rel in ['Shared/Globals.cs', 'Server/Settings.cs']]
    frozen = {str(path): sha(path.read_bytes()) for path in frozen_paths}
    comparisons = []
    for name, root in roots.items():
        for rel in RELS:
            source, reference = root/rel, ROOT/'资料参考/crystal'/rel
            a, b = source.read_bytes(), reference.read_bytes()
            comparisons.append({'root': name, 'relative_path': rel, 'source_sha256': sha(a),
                                'reference_sha256': sha(b), 'byte_equal': a == b,
                                'bom_eol_normalized_equal': '\n'.join(lines(source)) == '\n'.join(lines(reference))})
    selected = [('Server/MirObjects/HumanObject.cs', 'private void SummonSkeleton('),
                ('Server/MirObjects/HumanObject.cs', 'private void SummonShinsu('),
                ('Server/MirObjects/PlayerObject.cs', 'public override void GainExp('),
                ('Server/MirObjects/MonsterObject.cs', 'public void PetExp('),
                ('Server/MirObjects/MonsterObject.cs', 'public int DeadDelay'),
                ('Server/MirObjects/MonsterObject.cs', 'public void PetRecall('),
                ('Server/MirObjects/MonsterObject.cs', 'public override void Die('),
                ('Server/MirObjects/MonsterObject.cs', 'public virtual void RefreshAll('),
                ('Server/MirObjects/MonsterObject.cs', 'public bool Spawn(Map temp,'),
                ('Server/MirObjects/MonsterObject.cs', 'public override void Spawned(')]
    sections, diffs = [], []
    for rel, signature in selected:
        original = member(ROOT/'资料参考/crystal'/rel, signature)
        for name, root in roots.items():
            actual = member(root/rel, signature)
            same = actual[2] == original[2]
            sections.append({'root': name, 'relative_path': rel, 'signature': signature,
                             'source_lines': actual[:2], 'reference_lines': original[:2],
                             'section_text_equal_after_bom_eol_normalization': same})
            if not same:
                diffs.extend(difflib.unified_diff(original[2].splitlines(), actual[2].splitlines(),
                             fromfile='copied/'+rel, tofile=name+'/'+rel, lineterm=''))
    slices = BASE/'source-slices'
    slices.mkdir(exist_ok=True)
    (slices/'windows-local-related-methods.diff.txt').write_text('\n'.join(diffs)+'\n', encoding='utf-8')
    constants = []
    for name, root in roots.items():
        for rel, tokens in [('Shared/Globals.cs', ['DataRange =']), ('Server/Settings.cs', ['SkeletonName = "', 'ShinsuName = "'])]:
            path = root/rel
            constants.append({'root': name, 'source_path': str(path), 'source_sha256': sha(path.read_bytes()),
                              'technical_lines_only': [{'line': n+1, 'text': text.strip()} for n, text in enumerate(lines(path)) if any(token in text for token in tokens)]})
    save(slices/'windows-pet-technical-constants.json', constants)
    source = LATEST/'Shared/Globals.cs'
    target = ROOT/'资料参考/crystal-supplement/Shared/Globals.cs'
    target.parent.mkdir(parents=True, exist_ok=True)
    assert source.resolve() != target.resolve()
    if target.exists():
        assert target.read_bytes() == source.read_bytes(), '已有补充副本不同，不覆盖'
    else:
        shutil.copyfile(source, target)
    assert target.stat().st_nlink == 1
    assert target.read_bytes() == source.read_bytes()
    settings = LATEST/'Server/Settings.cs'
    historical = json.loads((ROOT/'资料索引/三职业技能来源映射.json').read_text(encoding='utf-8'))['source_files']['crystal_settings']['sha256']
    assert sha(settings.read_bytes()) == historical
    settings_slice = ROOT/'资料参考/crystal-supplement/source-slices/Settings.pet-defaults.json'
    save(settings_slice, {'schema_version': 1, 'is_runtime_configuration': False,
                         'source_path': str(settings), 'source_sha256': sha(settings.read_bytes()),
                         'scope': '仅两个静态宠物默认名；不复制或读取来源运行配置',
                         'technical_lines': [{'line': n+1, 'text': text.strip()} for n, text in enumerate(lines(settings))
                                             if 'SkeletonName = "' in text or 'ShinsuName = "' in text]})
    stable = all(sha(Path(path).read_bytes()) == digest for path, digest in frozen.items())
    assert stable, '来源审查期间变化'
    contract = {
        'schema_version': 1, 'date': '2026-10-29',
        'scope': '职业宠物静态技术源码补充；不是运行数值源或完整1.80认证',
        'is_runtime_configuration': False, 'target_180_verified': False,
        'unique_runtime_numeric_source': 'outputs/development-prep-20261026/两本唯一Excel',
        'authorization': 'root明确授权白名单读取两个Windows技术源码目录；独立复制Latest Globals，Settings只摘录默认宠物名',
        'source_inputs_read_only': True, 'source_inputs_hashes_stable': stable,
        'source_programs_executed': False, 'operating_configuration_or_account_db_read': False,
        'globals': {'source_path': str(source), 'source_sha256': sha(source.read_bytes()),
                    'local_reference_path': target.relative_to(ROOT).as_posix(),
                    'target_sha256': sha(target.read_bytes()), 'target_st_nlink': target.stat().st_nlink,
                    'copy_mode': 'independent_byte_copy', 'data_range_line': 37, 'data_range_cells': 16,
                    'field_status': '已匹配核心源码的静态常量；选入运行仍须唯一Excel候选'},
        'settings': {'source_path': str(settings), 'source_sha256': sha(settings.read_bytes()),
                     'matches_previously_registered_settings_sha256': True,
                     'historical_registered_sha256': historical, 'whole_file_copied': False,
                     'local_technical_slice': settings_slice.relative_to(ROOT).as_posix(),
                     'technical_slice_sha256': sha(settings_slice.read_bytes()),
                     'technical_default_literals': {'SkeletonName': {'value': 'BoneFamiliar', 'line': 162}, 'ShinsuName': {'value': 'Shinsu', 'line': 163}},
                     'field_status': '仅技术默认名称；可被Game配置覆盖，未读取运营配置，不认证原服实际模板选择'},
        'core_file_comparison': comparisons, 'related_method_comparison': sections,
        'proposed_candidate_composition': {
            'skeleton': 'MYF Monster.csv:381变异骷髅基础模板 + Crystal BoneFamiliar召唤/训练行为，显式跨来源开发候选',
            'ai_23_mapping': 'MonsterObject.GetMonster:65–67的AI23对应BoneFamiliar；不将MYF Race100直接等同AI23',
            'corpse_ttl_ms': {'value': 180000, 'source': 'MonsterObject.DeadDelay:584–599的default分支', 'status': '若本作选择BoneFamiliar/AI23行为时的明确开发候选，非MYF实服配置认证'}},
        'technical_slices': (slices/'windows-pet-technical-constants.json').relative_to(ROOT).as_posix(),
        'related_differences': (slices/'windows-local-related-methods.diff.txt').relative_to(ROOT).as_posix(),
        'boundary': 'Latest逐文件相同只证明列出的技术输入匹配，不声明两个目录所有文件/提交或实际部署配置相同；Local不同版本单独保留，不混为既有快照'
    }
    save(ROOT/'资料索引/职业宠物补充来源契约.json', contract)
    save(BASE/'windows-source-comparison.json', contract)
    print(json.dumps({'latest_byte_matches': sum(row['byte_equal'] for row in comparisons if row['root']=='windows_latest'),
                      'compared_files_per_root': len(RELS), 'source_inputs_stable': stable,
                      'globals_target_st_nlink': target.stat().st_nlink,
                      'contract': '资料索引/职业宠物补充来源契约.json'}, ensure_ascii=False))

if __name__ == '__main__':
    main()
