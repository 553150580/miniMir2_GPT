"""从已锁定来源合同编译宠物出生资源，时序只从原源码/Lib动作表派生。"""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import re
import struct


CONTRACT = '资料索引/宠物出生表现来源契约.json'


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def parse_frame_set(data: bytes, action_names: list[str]) -> dict:
    """对应已复制MLibrary v3与Frame(BinaryReader)，拒绝截断、重复或未知动作。"""
    if len(data) < 12:
        raise ValueError('宠物Lib头部截断')
    version, count, seek = struct.unpack_from('<iii', data)
    if version != 3 or not 0 < count <= 1_000_000 or not 12 + count * 4 <= seek <= len(data) - 4:
        raise ValueError('宠物Lib版本、图片数或动作表位置不合法')
    indices = struct.unpack_from(f'<{count}i', data, 12)
    if any(not 12 + count * 4 <= offset < seek for offset in indices):
        raise ValueError('宠物Lib图片索引越界')
    frame_count = struct.unpack_from('<i', data, seek)[0]
    if not 0 < frame_count <= len(action_names) or seek + 4 + frame_count * 35 != len(data):
        raise ValueError('宠物Lib动作表截断或含尾部未知数据')
    fields = ['start', 'count', 'skip', 'interval_ms', 'effect_start', 'effect_count', 'effect_skip', 'effect_interval_ms']
    frames = {}
    for i in range(frame_count):
        offset = seek + 4 + i * 35
        action = data[offset]
        if action >= len(action_names) or action_names[action] in frames or data[offset + 33] > 1 or data[offset + 34] > 1:
            raise ValueError('宠物Lib动作重复、未知或布尔值无效')
        frame = dict(zip(fields, struct.unpack_from('<8i', data, offset + 1)))
        frame.update(reverse=bool(data[offset + 33]), blend=bool(data[offset + 34]))
        if frame['start'] < 0 or frame['count'] <= 0 or frame['interval_ms'] <= 0 or frame['count'] + frame['skip'] < 0:
            raise ValueError('宠物Lib动作参数无效')
        frames[action_names[action]] = frame
    return {'version': version, 'image_count': count, 'frame_seek': seek, 'frames': frames}


def compile_contract(root: Path) -> dict:
    path = root / CONTRACT
    contract = json.loads(path.read_text(encoding='utf-8'))
    if contract['schema_version'] != 1 or contract['status'] != 'candidate' or contract['official_180_restoration_verified'] is not False:
        raise ValueError('出生视觉合同必须保持显式来源候选')
    for item in contract['copies']:
        local = root / item['local_reference_path']
        if local.is_symlink() or not local.is_file() or local.stat().st_nlink != 1 or digest(local) != item['sha256']:
            raise ValueError('出生视觉来源副本摘要或独立性不符')
    code = {}
    for item in contract['source_code']:
        source = root / item['path']
        if digest(source) != item['sha256']:
            raise ValueError('出生视觉依据源码摘要变化')
        code[Path(item['path']).name] = source.read_text(encoding='utf-8-sig')
    enum_body = code['Enums.cs'].split('public enum MirAction : byte', 1)[1].split('{', 1)[1].split('}', 1)[0]
    names = [name.strip() for name in enum_body.split(',') if name.strip()]
    if any(not re.fullmatch(r'[A-Za-z][A-Za-z0-9_]*', name) for name in names):
        raise ValueError('MirAction枚举格式变化，需审视资源读取器')
    visual = contract['visual_mapping']
    cast = visual['cast']
    if type(cast['skill_id']) is not int or cast['skill_id'] <= 0 or cast['package'] != 'Magic':
        raise ValueError('召唤起手映射无效')
    expression = re.search(r'case Spell\.' + re.escape(cast['source_spell']) + r':\s*Effects\.Add\(new Effect\(Libraries\.Magic,\s*(\d+),\s*(\d+),\s*Frame\.Count\s*\*\s*FrameInterval,\s*this\)\);', code['PlayerObject.cs'])
    spell = re.search(r'Player\.Add\(MirAction\.Spell, new Frame\(\s*\d+,\s*(\d+),\s*[-\d]+,\s*(\d+)', code['Frames.cs'])
    if not expression or not spell or 'public bool Blend = true;' not in code['Effect.cs']:
        raise ValueError('召唤起手或默认混合源码格式变化')
    cast_start, cast_count = map(int, expression.groups())
    spell_count, interval = map(int, spell.groups())
    if not cast_count or not spell_count or not interval:
        raise ValueError('召唤起手帧数或时长为空')
    birth = visual['birth']
    if type(birth['raw_start']) is not int or birth['raw_start'] < 0:
        raise ValueError('出生WIL原索引必须显式非负整数')
    library = parse_frame_set((root / birth['source_library']).read_bytes(), names)
    frame = library['frames'][birth['source_action']]
    if frame['reverse'] or not frame['blend'] or frame['count'] + frame['skip'] != 0 or any(frame[key] for key in ['effect_start', 'effect_count', 'effect_skip', 'effect_interval_ms']):
        raise ValueError('出生Lib动作不再符合单序列加色合同')
    audio = {}
    copies = {item['local_reference_path']: item for item in contract['copies']}
    for key, item in contract['audio_mapping'].items():
        output = Path(item['output'])
        if output.is_absolute() or '..' in output.parts or output.parts[0] != 'audio' or output.suffix != '.wav':
            raise ValueError('声音输出必须位于独立audio目录')
        audio[key] = {**item, 'sha256': copies[item['source']]['sha256']}
    return {'contract': contract, 'cast': {'skill_id': cast['skill_id'], 'package': cast['package'], 'raw_start': cast_start, 'count': cast_count, 'duration_ms': spell_count * interval, 'blend': 'additive'},
            'birth': {**birth, 'count': frame['count'], 'direction_stride': frame['count'] + frame['skip'], 'duration_ms': frame['interval_ms'], 'blend': 'additive'}, 'audio': audio,
            'evidence': {'contract': CONTRACT, 'contract_sha256': digest(path), 'compiler': 'tools/resources/pet_birth_assets.py', 'compiler_sha256': digest(Path(__file__)), 'status': 'candidate'}}


def append_assets(root, importer, profiles, effects, directions):
    compiled = compile_contract(root)
    # 包摘要锁定在来源契约；不同包不得仅凭相同帧号混入。
    for item in compiled['contract']['wil_sources']:
        wil_path, wix_path, _ = importer.package(item['package'])
        if digest(wil_path) != item['wil_sha256'] or digest(wix_path) != item['wix_sha256']:
            raise ValueError('出生WIL/WIX版本与来源合同不符')
    cast, birth = compiled['cast'], compiled['birth']
    cast_frames = [importer.frame(cast['package'], cast['raw_start'] + i) for i in range(cast['count'])]
    birth_frames = [importer.frame(birth['package'], birth['raw_start'] + i) for i in range(birth['count'])]
    if any(importer.frames[key]['visible_alpha_pixels'] <= 0 for key in cast_frames + birth_frames):
        raise ValueError('召唤起手或出生映射引用空原格')
    effects[str(cast['skill_id'])] = {'kind': 'summon_pet', 'cast': {'frames': cast_frames, 'duration_ms': cast['duration_ms'], 'blend': cast['blend']}, 'cast_sound': 'summon_skeleton_cast'}
    layer = next(layer for layer in profiles[birth['profile']]['layers'] if layer['package'] == birth['package'])
    layer['actions']['appear'] = {key: birth[key] for key in ['raw_start', 'count', 'direction_stride', 'duration_ms', 'blend']} | {'loop': False, 'sound': 'pet_skeleton_born', 'directions': [{'direction': index, 'name': name, 'frames': list(birth_frames)} for index, name in enumerate(directions)]}
    audio = {}
    for key, item in compiled['audio'].items():
        data = (root / item['source']).read_bytes()
        importer.write_bytes(item['output'], data)
        audio[key] = {'path': item['output'], 'sha256': item['sha256'], 'status': 'candidate', 'development_only': True, 'source': item['source'], 'trigger': item['trigger']}
    return audio, compiled['evidence']
