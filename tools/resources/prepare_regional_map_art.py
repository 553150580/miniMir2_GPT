"""从本机已核验地表制作小地图表现，输出独立 Creator 资源。"""

from __future__ import annotations

import argparse
from dataclasses import dataclass
from io import BytesIO
import json
from pathlib import Path

from PIL import Image

from build_development_assets import TILE_HEIGHT, TILE_WIDTH, project_floor
from build_hud_assets import base_meta, image_meta, stable_uuid
from build_source_maps import SourceMap, digest
import inspect_wil as wil


ROOT = Path(__file__).resolve().parents[2]
RESOURCES = ROOT / 'client/assets/resources/development'
OUTPUT = RESOURCES / 'map-styles'
BASE_RESOURCE = 'development/map-styles/'
MYF_MAP = ROOT / '资料参考/myf/Mir200/Map/0.map'
LOCKED_MAPS = {
    'myf:0': '2780766485d1e047dc2e8f03578ee83c6a14fd91597fe4562329cb8f24e1d4ec',
    'client:2': '7ef574cd504954a35c8f26571b54a39787086fcb6603931fa76dd336aa2d5d4e',
    'client:3': 'd959f496351dcbb105f2356622890473193b57132e9c7b2e52230f551a2b0e04',
}
LOCKED_TILES = {
    'Tiles': ('6564d75d35f31249ba471874ae74b392bc6863ee33c1092972782e70170e71e4',
              'e11fac073e0229b41812307afbda74a00872e19f41ba30b68803b6bdd4c116e6'),
    'SmTiles': ('f87570676bff54457e9b56630a4b557e1129c3f0a5a359047668be40e44a0605',
                '16d41274a8c68632a1bd6d4a64d065ff70cb81e468c97fd34021fd772267e0ed'),
}


@dataclass(frozen=True)
class FloorStyle:
    """一种场景的美术来源，不定义可走格、角色或实体。"""

    map_id: int  # 当前项目的场景编号。
    slug: str  # 素材名，后缀使用实际 PNG 摘要。
    terrain: str  # 可在浏览器验收中识别的地区风格。
    source: str  # 原地表来源，客户端两个区域只用代表性地砖。
    origin: tuple[int, int]  # 原图中实际裁样的左上格。


STYLES = (
    FloorStyle(1, 'forest-meadow', '林间草地', 'existing', (130, 130)),
    FloorStyle(3, 'ginkgo-village', '银杏村落土径', 'myf:0', (642, 623)),
    FloorStyle(4, 'border-village', '边界村绿地土路', 'myf:0', (289, 605)),
    FloorStyle(5, 'bichon-city', '比奇城夯土广场', 'myf:0', (323, 259)),
    FloorStyle(6, 'southern-valley', '山谷岩土坡地', 'client:2', (428, 556)),
    FloorStyle(7, 'spider-waste', '荒地砂砾地表', 'client:3', (700, 400)),
)


def json_bytes(value) -> bytes:
    """统一生成 UTF-8、LF 和固定缩进。"""
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')


class GroundSource:
    """只读打开原地图、地砖库和索引，缓存已读取的有限原帧。"""

    def __init__(self, client_root: Path):
        self.client_root = client_root
        self.maps = {}
        self.packages = {}
        self.frames = {}
        wil.SOURCE_ROOTS['regional_ground'] = client_root / 'Data'

    def source_map(self, source: str) -> SourceMap:
        """严格按锁定的原地图记录加载，不引用完整地图作为游戏关卡。"""
        if source not in self.maps:
            path = MYF_MAP if source == 'myf:0' else self.client_root / 'Map' / f'{source.split(":", 1)[1]}.map'
            source_root = ROOT / '资料参考' if source == 'myf:0' else self.client_root
            data = wil.readonly_path(path, source_root).read_bytes()
            if digest(data) != LOCKED_MAPS[source]:
                raise ValueError(f'原地表地图原件摘要不符：{source}')
            self.maps[source] = SourceMap(data)
        return self.maps[source]

    def frame(self, package: str, raw: int) -> tuple[Image.Image, str]:
        """仅解码被样本引用的非空原生平面地砖。"""
        key = (package, raw)
        if key not in self.frames:
            if package not in self.packages:
                path, index = wil.locate_pair('regional_ground', package)
                if (wil.sha256_file(path), wil.sha256_file(index)) != LOCKED_TILES[package]:
                    raise ValueError(f'原地表库或索引摘要不符：{package}')
                self.packages[package] = (path, wil.probe(path, index))
            path, data = self.packages[package]
            if not 0 <= raw < data['frame_count']:
                raise ValueError(f'原地砖编号超出范围：{package}:{raw}')
            frame = data['frames'][raw]
            width, height = frame['width'], frame['height']
            maximum = (2 * TILE_WIDTH, 2 * TILE_HEIGHT) if package == 'Tiles' else (TILE_WIDTH, TILE_HEIGHT)
            if frame['declared_empty'] or not 0 < width <= maximum[0] or not 0 < height <= maximum[1]:
                raise ValueError(f'原地砖缺失或为立体贴图：{package}:{raw} {width}×{height}')
            rgba = wil.decode_frame(path, frame, data['palette'])
            self.frames[key] = (Image.frombytes('RGBA', (width, height), rgba), digest(rgba))
        return self.frames[key]


def render_floor(source: GroundSource, style: FloorStyle, width: int, height: int) -> tuple[bytes, dict]:
    """从原 back/middle 绘制地面并换为本项目的 64×32 菱形。"""
    level = source.source_map(style.source)
    x0, y0 = style.origin
    x1, y1 = x0 + width, y0 + height
    if min(x0, y0) < 0 or x1 > level.width or y1 > level.height:
        raise ValueError(f'地图{style.map_id}的地表采样越界')
    canvas = Image.new('RGBA', (width * TILE_WIDTH, height * TILE_HEIGHT))
    frames = {}
    # 原背景大瓦片在偶数格定位，奇数起点可能需要裁剪左侧和上侧的交叠。
    for y in range(y0 - y0 % 2, y1, 2):
        for x in range(x0 - x0 % 2, x1, 2):
            number = level.cell(x, y).back & 0x7fff
            if number:
                image, sha = source.frame('Tiles', number - 1)
                canvas.alpha_composite(image, ((x - x0) * TILE_WIDTH, (y - y0) * TILE_HEIGHT))
                frames[f'Tiles:{number - 1}'] = sha
    for y in range(y0, y1):
        for x in range(x0, x1):
            number = level.cell(x, y).middle & 0x7fff
            if number:
                image, sha = source.frame('SmTiles', number - 1)
                canvas.alpha_composite(image, ((x - x0) * TILE_WIDTH, (y - y0) * TILE_HEIGHT))
                frames[f'SmTiles:{number - 1}'] = sha
    if canvas.getchannel('A').getextrema() != (255, 255):
        raise ValueError(f'地图{style.map_id}的平面地表存在空洞')
    floor = project_floor(canvas, width, height, 64, 32)
    output = BytesIO()
    floor.save(output, format='PNG')
    record = {'source_map': style.source, 'source_map_sha256': LOCKED_MAPS[style.source],
              'source_origin': {'x': x0, 'y': y0}, 'source_tile_rgba_sha256': dict(sorted(frames.items()))}
    return output.getvalue(), record


def existing_floor(style: FloorStyle) -> tuple[bytes, dict]:
    """首张林地图保留当前已经验收的原地表。"""
    relative = 'maps/personal-map.png'
    path = RESOURCES / relative
    payload = path.read_bytes()
    source = json.loads(path.with_suffix('.json').read_bytes())
    if digest(payload) != source['background_sha256'] or source['source_origin'] != dict(zip(('x', 'y'), style.origin, strict=True)):
        raise ValueError('现有林间地表与原记录不一致')
    return payload, {'source_map': 'existing:personal-map', 'source_map_sha256': source['source_sha256'],
                     'source_origin': source['source_origin'], 'source_floor': relative}


def render_styles(client_root: Path) -> tuple[dict, dict]:
    """用现行地图目录核对每种地表的大小及场景类型。"""
    fixture = json.loads((ROOT / 'config/development/first-loop.json').read_bytes())
    maps = {row['id']: row for row in [fixture['map'], *fixture['maps']]}
    source = GroundSource(client_root)
    rows, artifacts = [], {}
    for style in STYLES:
        current = maps[style.map_id]
        width, height, kind = (current[key] for key in ('width', 'height', 'scene_kind'))
        if (kind, width, height) not in ((1, 13, 13), (2, 17, 17)):
            raise ValueError(f'地图{style.map_id}的玩法网格不适用于区域地表')
        payload, provenance = existing_floor(style) if style.source == 'existing' else render_floor(source, style, width, height)
        sha = digest(payload)
        name = f'{style.slug}-{sha[:12]}'
        image_width, image_height = (width + height) * 32, (width + height) * 16
        with Image.open(BytesIO(payload)) as image:
            if image.size != (image_width, image_height) or image.mode != 'RGBA':
                raise ValueError(f'地图{style.map_id}的导出尺寸或透明边缘格式不正确')
        artifacts[f'{name}.png'] = payload
        artifacts[f'{name}.png.meta'] = json_bytes(image_meta(name, image_width, image_height, 0))
        rows.append({'map_id': str(style.map_id), 'scene_kind': kind, 'width': width, 'height': height, 'frame': BASE_RESOURCE + name,
                     'terrain': style.terrain, 'png_sha256': sha, 'source_layers': ['back', 'middle'], **provenance})
    if len({row['png_sha256'] for row in rows}) != len(rows):
        raise ValueError('地区地表仍有相同贴图')
    manifest = {'schema_version': 1, 'styles': rows}
    artifacts['manifest.json'] = data = json_bytes(manifest)
    artifacts['manifest.json.meta'] = json_bytes(base_meta('json', '2.0.1', stable_uuid(f'regional-style-manifest-{digest(data)}'), ['.json']))
    return manifest, artifacts


def write_checked(path: Path, desired: bytes, check: bool) -> None:
    """只写独立表现目录，在复验时逐项核对。"""
    if path.resolve() != path or path.is_symlink() or path.is_junction():
        raise ValueError(f'区域地表目标不是项目内的稳定路径：{path}')
    if check:
        actual = path.read_bytes()
        same = json.loads(actual) == json.loads(desired) if path.suffix == '.meta' else actual == desired
        if not same:
            raise ValueError(f'区域地表的已保存字节不一致：{path.name}')
    elif not path.is_file() or path.read_bytes() != desired:
        path.write_bytes(desired)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source-root', required=True, type=Path, help='只读原客户端根，需包含 Map 和 Data')
    parser.add_argument('--check', action='store_true', help='按原件重新计算并只读核对已有产物')
    args = parser.parse_args()
    manifest, files = render_styles(args.source_root.resolve(strict=True))
    if OUTPUT.resolve() != OUTPUT or OUTPUT.is_symlink() or OUTPUT.is_junction():
        raise ValueError('区域地表目录存在重定向')
    if not args.check:
        OUTPUT.mkdir(exist_ok=True)
    write_checked(OUTPUT.with_suffix('.meta'), json_bytes(base_meta('directory', '1.2.0', stable_uuid('regional-style-directory'), [])), args.check)
    for filename, data in files.items():
        write_checked(OUTPUT / filename, data, args.check)
    print(json.dumps({'maps': len(manifest['styles']), 'files': len(files) + 1,
                      'status': 'checked' if args.check else 'generated'}, ensure_ascii=False))


if __name__ == '__main__':
    main()
