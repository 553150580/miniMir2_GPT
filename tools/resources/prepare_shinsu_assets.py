"""校验有权使用的神兽八向 PNG，并构建独立的 Creator 开发资源包。

输入清单记录 schema_version、provenance 与 actions。provenance.kind 可为
user_original 或 licensed，provenance.reference 记录创作或授权凭据。
actions 为 idle、walk、attack、hurt、death、appear。每项含 interval_ms
和八组 directions，从上方起顺时针排列。每帧填写相对 png、sha256、
offset_x、offset_y。输出保留 PNG 原字节与动作/脚点信息。
"""

import argparse
import hashlib
import io
import json
from pathlib import Path

from PIL import Image


ROOT = Path(__file__).resolve().parents[2]
DEVELOPMENT = ROOT / 'client/assets/resources/development'
PACKAGE = 'pets/shinsu'
DIRECTIONS = ('up', 'up_right', 'right', 'down_right', 'down', 'down_left', 'left', 'up_left')
ACTIONS = {'idle': 'standing', 'walk': 'walking', 'attack': 'attack', 'hurt': 'struck', 'death': 'die', 'appear': 'appear'}


def sha(data):
    """计算原文件摘要，方便独立验收和可重复生成。"""
    return hashlib.sha256(data).hexdigest()


def compile_assets(source):
    """核对动作和图像，再返回外观模型及完整输出文件集合。"""
    source = source.resolve()
    raw = source.read_bytes()
    contract = json.loads(raw.decode('utf-8'))
    provenance = contract.get('provenance', {})
    if contract.get('schema_version') != 1 or provenance.get('kind') not in ('user_original', 'licensed') or not provenance.get('reference'):
        raise ValueError('资源清单缺少版本或可核对的创作、授权凭据')
    if set(contract.get('actions', {})) != set(ACTIONS):
        raise ValueError('需要 idle、walk、attack、hurt、death、appear 六套动作')
    layers, frames, outputs = {}, {}, {}
    for source_action, runtime_action in ACTIONS.items():
        action = contract['actions'][source_action]
        interval, sequences = action['interval_ms'], action['directions']
        if type(interval) is not int or interval <= 0 or len(sequences) != len(DIRECTIONS):
            raise ValueError(f'动作 {source_action} 的时长或八方向序列无效')
        count = len(sequences[0])
        if not count or not all(len(sequence) == count for sequence in sequences):
            raise ValueError(f'动作 {source_action} 各方向需有相同的非零帧数')
        directions = []
        for direction, sequence in enumerate(sequences):
            keys = []
            for frame in sequence:
                path = (source.parent / frame['png']).resolve()
                dx, dy = frame['offset_x'], frame['offset_y']
                if not path.is_relative_to(source.parent) or path.suffix.lower() != '.png' or type(dx) is not int or type(dy) is not int:
                    raise ValueError('帧路径或整数脚点偏移不合法')
                data = path.read_bytes()
                if sha(data) != frame['sha256']:
                    raise ValueError(f'帧摘要不一致 {path.name}')
                with Image.open(io.BytesIO(data)) as image:
                    if image.format != 'PNG' or getattr(image, 'n_frames', 1) != 1:
                        raise ValueError(f'帧须为独立 PNG 图像 {path.name}')
                    width, height = image.size
                    visible_pixels = sum(pixel != 0 for pixel in image.convert('RGBA').getchannel('A').tobytes())
                if not visible_pixels:
                    raise ValueError(f'帧没有可见像素 {path.name}')
                index = len(frames)
                key = f'Shinsu:{index}'
                filename = f'frames/frame_{index:05}.png'
                frames[key] = {
                    'png': f'{PACKAGE}/{filename}', 'png_sha256': frame['sha256'], 'width': width, 'height': height,
                    'offset_x': dx, 'offset_y': dy, 'declared_empty': False, 'visible_alpha_pixels': visible_pixels,
                }
                outputs[filename] = data
                keys.append(key)
            directions.append({'direction': direction, 'name': DIRECTIONS[direction], 'frames': keys})
        layers[runtime_action] = {
            'count': count, 'duration_ms': interval, 'loop': source_action in ('idle', 'walk'), 'directions': directions,
        }
    profile = {'id': 'pet_shinsu', 'development_only': True, 'layers': [{'package': 'Shinsu', 'actions': layers}]}
    manifest = {'schema_version': 1, 'contract_sha256': sha(raw), 'profile': profile, 'frames': frames}
    outputs['profile.json'] = (json.dumps(manifest, ensure_ascii=False, indent=2) + '\n').encode('utf-8')
    return outputs


def check_creator_metadata(root, target, outputs):
    """检查 Creator 实际导入后的目录、清单和 SpriteFrame 元数据。"""
    directories = (root / 'pets.meta', root / 'pets/shinsu.meta', target / 'frames.meta')
    expected = [(path, 'directory') for path in directories]
    for filename in outputs:
        importer = 'image' if filename.endswith('.png') else 'json'
        expected.append((target / (filename + '.meta'), importer))
    for path, importer in expected:
        if not path.is_file():
            raise ValueError(f'请先使用 Creator 3.8.8 导入并保存元数据 {path.name}')
        metadata = json.loads(path.read_text(encoding='utf-8'))
        if metadata.get('importer') != importer or metadata.get('imported') is not True or not metadata.get('uuid'):
            raise ValueError(f'Creator 导入元数据无效 {path.name}')
        if importer == 'image' and not any(part.get('importer') == 'sprite-frame' for part in metadata.get('subMetas', {}).values()):
            raise ValueError(f'Creator PNG 缺少 SpriteFrame 子资源 {path.name}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path, help='有权使用的 PNG 帧清单')
    parser.add_argument('--output', type=Path, default=DEVELOPMENT, help='开发资源根目录，或项目内的独立试验目录')
    parser.add_argument('--check', action='store_true', help='核对输出，无磁盘修改')
    args = parser.parse_args()
    root = args.output.resolve()
    if root != DEVELOPMENT and not any(root.is_relative_to(ROOT / name) for name in ('.work', 'outputs')):
        raise ValueError('只允许写入开发资源根目录或项目独立试验目录')
    target = root / PACKAGE
    outputs = compile_assets(args.source)
    for filename, data in outputs.items():
        path = target / filename
        if path.resolve() != path:
            raise ValueError(f'输出路径必须直接位于独立资源目录 {filename}')
        if path.is_file() and path.read_bytes() == data:
            continue
        if args.check:
            raise ValueError(f'独立资源产物缺失或摘要变化 {filename}')
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
    if args.check and root == DEVELOPMENT:
        check_creator_metadata(root, target, outputs)
    print(f'神兽独立开发资源 {len(outputs) - 1} 帧，目录 {target}')


if __name__ == '__main__':
    main()
