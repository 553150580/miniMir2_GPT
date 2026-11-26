"""保持 Creator 原产物，给试玩的 Game 场景合包生成与历史 URL 兼容的副本。"""

import argparse
from concurrent.futures import ThreadPoolExecutor
from hashlib import sha256
import json
import os
from pathlib import Path, PurePosixPath
import re
import sys


ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
from creator_work_paths import local_creator_work


MAIN_CONFIG = 'assets/main/config.json'
MAIN_BUNDLE = 'assets/main/index.js'
GAME_SCENE = 'db://assets/scenes/Game.scene'
PREVIEW_ROOT = 'preview-web-mobile'
SIDECAR = 'preview-game-alias.json'


def sha(path):
    """计算实际文件字节的摘要。"""
    return sha256(path.read_bytes()).hexdigest()


def json_bytes(value):
    """使用稳定的 UTF-8 JSON 编码，不改变已有的字段顺序。"""
    return json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode('utf-8')


def exact_file(root, name):
    """索引文件必须由隔离目录直接提供。"""
    relative = PurePosixPath(name)
    if relative.is_absolute() or relative.as_posix() != name or '..' in relative.parts or '\\' in name:
        raise ValueError(f'发布索引路径无效：{name}')
    path = root.joinpath(*relative.parts)
    if path.resolve(strict=True) != path or not path.is_file():
        raise ValueError(f'发布文件路径发生重定向：{path}')
    return path


def stage_plan(source_web, creator_file, prior_release, live):
    """从原 Creator 索引和线上旧字节推导可逐项检查的唯一预览索引。"""
    creator = json.loads(creator_file.read_bytes())
    if not (creator['passed'] and creator['source_stable'] and creator['creator'] == '3.8.8'
            and creator['platform'] == 'web-mobile' and creator['exit_code'] == 36
            and Path(creator['project']) == source_web.parents[1]):
        raise ValueError('Game 版本化须使用独立 Creator 3.8.8 Web Mobile 原实构')
    source_files = creator['build_sha256']
    published = json.loads(prior_release.read_bytes())
    if not published['passed'] or published['scene_assets']['Game'] == {}:
        raise ValueError('缺少已发布的原 Game 场景身份')
    old_game = published['scene_assets']['Game']
    historical = exact_file(live, old_game['path']).read_bytes()
    if sha256(historical).hexdigest() != old_game['sha256']:
        raise ValueError('原试玩的 Game 场景字节与已发布摘要不一致')

    raw_config = exact_file(source_web, MAIN_CONFIG).read_bytes()
    config = json.loads(raw_config)
    if sha256(raw_config).hexdigest() != source_files[MAIN_CONFIG] or config['name'] != 'main' or config['importBase'] != 'import':
        raise ValueError('Creator 主包配置与原构建摘要不一致')

    def asset_id(value):
        """读取 Cocos 允许使用字符串或索引的资源 UUID。"""
        return config['uuids'][value] if type(value) is int else value

    game_uuid = asset_id(config['scenes'][GAME_SCENE])
    pack, = [key for key, items in config['packs'].items() if game_uuid in map(asset_id, items)]
    if not re.fullmatch(r'[a-f0-9]{2,}', pack):
        raise ValueError('无法识别 Game 的 Creator JSON 合包')
    source_game = f'assets/main/import/{pack[:2]}/{pack}.json'
    version_entries = config['versions']['import']
    versioned_ids = {asset_id(value) for value in version_entries[::2]}
    if pack in versioned_ids or old_game['path'] != source_game:
        raise ValueError('源 Game 合包已经版本化或与历史 URL 不同，无需本次兼容转换')
    game_file = exact_file(source_web, source_game)
    game_sha = source_files[source_game]
    if sha(game_file) != game_sha or game_sha == old_game['sha256']:
        raise ValueError('当前 Game 合包缺少不同且可校验的 Creator 原字节')
    version = game_sha[:16]
    new_game = f'assets/main/import/{pack[:2]}/{pack}.{version}.json'
    if new_game in source_files:
        raise ValueError('新 Game 版本 URL 已在原实构中占用')
    config['versions']['import'] = [*version_entries, pack, version]
    adjusted_config = json_bytes(config)
    finished = {**source_files, MAIN_CONFIG: sha256(adjusted_config).hexdigest(),
                source_game: old_game['sha256'], new_game: game_sha}
    report = {
        'passed': True,
        'source_web_root': str(source_web),
        'web_root': str(source_web.parents[2] / PREVIEW_ROOT),
        'creator_report_sha256': sha(creator_file),
        'prior_release_sha256': sha(prior_release),
        'raw_main_config_sha256': source_files[MAIN_CONFIG],
        'index_sha256': finished['index.html'],
        'main_bundle_sha256': finished[MAIN_BUNDLE],
        'main_config_sha256': finished[MAIN_CONFIG],
        'raw_game': {'path': source_game, 'sha256': game_sha},
        'old_game': old_game,
        'new_game': {'path': new_game, 'sha256': game_sha},
        'changed_paths': sorted((MAIN_CONFIG, source_game, new_game)),
        'build_sha256': finished,
    }
    return report, {MAIN_CONFIG: adjusted_config, source_game: historical}, {new_game: source_game}


def verify_inventory(directory, expected):
    """逐个核验静态树原字节，并拒绝未被索引的文件或路径重定向。"""
    observed = set()
    for path in directory.rglob('*'):
        if path.is_symlink() or path.is_junction():
            raise ValueError(f'静态路径含链接目录或符号链接：{path}')
        if path.is_file():
            observed.add(path.relative_to(directory).as_posix())
    if observed != expected.keys():
        raise ValueError(f'静态索引文件集合不一致：{directory}')

    def verify(name):
        if sha(exact_file(directory, name)) != expected[name]:
            raise ValueError(f'静态文件字节与索引不一致：{name}')

    with ThreadPoolExecutor(max_workers=8) as worker:
        list(worker.map(verify, sorted(observed)))


def materialize(creator_dir, prior_release):
    """新建兼容副本，仅两份新内容实写，所有同字节文件均从原实构链接。"""
    source = creator_dir / 'client-project/build/web-mobile'
    creator_file = creator_dir / 'creator-report.json'
    output, metadata = creator_dir / PREVIEW_ROOT, creator_dir / SIDECAR
    report, independent, renamed = stage_plan(source, creator_file, prior_release, ROOT / 'client/build/web-mobile')
    source_files = json.loads(creator_file.read_bytes())['build_sha256']
    verify_inventory(source, source_files)
    if metadata.exists() and (not output.is_dir() or json.loads(metadata.read_bytes()) != report):
        raise ValueError('已冻结的 Game 兼容报告与本次预览不一致')
    if not output.exists():
        pending = creator_dir / (PREVIEW_ROOT + '.building')
        pending.mkdir()
        for name in report['build_sha256']:
            target = pending / name
            target.parent.mkdir(parents=True, exist_ok=True)
            if name in independent:
                with target.open('xb') as stream:
                    stream.write(independent[name])
            else:
                os.link(exact_file(source, renamed.get(name, name)), target)
        verify_inventory(pending, report['build_sha256'])
        pending.rename(output)
    else:
        verify_inventory(output, report['build_sha256'])
    if not metadata.exists():
        with metadata.open('xb') as stream:
            stream.write(json_bytes(report) + b'\n')
    print(metadata, flush=True)


def main():
    """限定 Windows 工具链、本项目 C 盘隔离区和发布原证据。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--creator-dir', type=Path, required=True)
    parser.add_argument('--prior-release', type=Path, required=True)
    args = parser.parse_args()
    owned = local_creator_work(ROOT)
    creator, previous = args.creator_dir.absolute(), args.prior_release.absolute()
    workspace = ROOT / '.work'
    if (os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1' or owned is None
            or creator == owned or not creator.is_relative_to(owned)
            or not previous.is_relative_to(workspace) or previous.name != 'published.json'
            or creator.resolve(strict=True) != creator or previous.resolve(strict=True) != previous):
        parser.error('须使用锁定工具链、本项目独立 Creator 目录与原发布记录')
    for relative in ('client-project/build/web-mobile', 'creator-report.json'):
        path = creator / relative
        if path.resolve(strict=True) != path:
            parser.error('拒绝经重定向的 Creator 原构建')
    for relative in (PREVIEW_ROOT, PREVIEW_ROOT + '.building', SIDECAR):
        path = creator / relative
        if path.is_symlink() or path.is_junction() or path.resolve() != path:
            parser.error('拒绝经重定向的兼容副本路径')
    materialize(creator, previous)


if __name__ == '__main__':
    main()
