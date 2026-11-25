"""为本机 Creator 工程准备完整的原版 NPC、普通怪、首领和神兽资源。"""

import argparse
import json
from pathlib import Path

import prepare_original_monster_assets as monsters
import prepare_shinsu_lib_preview as shinsu
import prepare_town_npc_wil_preview as npcs


def write_files(output, files, check):
    """重复导入时核对原字节，避免修改已有的等价输入。"""
    for relative, content in files.items():
        path = output / relative
        if path.resolve() != path:
            raise ValueError(f'外观输出路径含重定向：{path.name}')
        if path.is_file() and path.read_bytes() == content:
            continue
        if check:
            raise ValueError(f'缺少原版外观或文件摘要不一致：{relative}')
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(content)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('monster_libs', type=Path, help='用户本机的原版 Monster Lib 目录')
    parser.add_argument('npc_data', type=Path, help='用户本机的原版 npc.wil 与 npc.WIX 所在 Data 目录')
    parser.add_argument('--output', type=Path, default=monsters.ROOT / 'client/assets/resources/development')
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    destination = args.output.resolve()
    if not destination.is_relative_to(monsters.ROOT) or destination.parts[-3:] != ('assets', 'resources', 'development'):
        raise ValueError('外观产物仅写入本项目的 Creator 开发资源目录')
    libraries = args.monster_libs.resolve(strict=True)
    monster_manifest, monster_files = monsters.compile_art(libraries)
    shinsu_files, _, shinsu_count = shinsu.compile_preview(libraries)
    contract = json.loads(npcs.CONTRACT.read_text(encoding='utf-8'))
    fixture = json.loads(npcs.FIXTURE.read_text(encoding='utf-8'))
    npc_manifest, npc_files = npcs.compile_preview(args.npc_data.resolve(strict=True), contract, fixture)
    for files in (monster_files, shinsu_files, npc_files):
        write_files(destination, files, args.check)
    print(json.dumps({'怪物及首领帧': len(monster_manifest['frames']), '神兽帧': shinsu_count,
                      'NPC帧': len(npc_manifest['frames']), 'NPC映射': len(npc_manifest['npc_profiles'])}, ensure_ascii=False))


if __name__ == '__main__':
    main()
