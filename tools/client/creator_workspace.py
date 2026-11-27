"""在项目盘复用独立 Creator 工程，按需冻结发布证据。"""

import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import tempfile

import isolated_creator as creator
from creator_work_paths import CREATOR_SLOTS, checked_child, creator_slot_lock, is_reparse_point, workspace_creator_root


ROOT = creator.ROOT
SOURCE_PUBLIC = 'assets/resources/development/gameplay.json'
PUBLIC_META = SOURCE_PUBLIC + '.meta'
WEB_MOBILE = 'build/web-mobile'
FIXTURE = 'config/development/first-loop.json'


def read(path):
    return json.loads(path.read_bytes())


def json_bytes(value):
    return (json.dumps(value, ensure_ascii=False, indent=2) + '\n').encode('utf-8')


def input_file(root, supplied):
    """固定工程配置的原文件，避免借用重解析路径读取其它位置。"""
    resolved = checked_child(root, (root / supplied).absolute())
    if not resolved.is_file():
        raise ValueError(f'缺少 Creator 构建输入：{resolved}')
    return resolved


def project_inputs(root, profile, fixture, public):
    """固定客户端输入和规则。默认槽沿用主工程的公开配置原字节。"""
    source = root / 'client'
    if profile == 'default' and (fixture or public):
        raise ValueError('默认工作槽使用主工程和默认夹具，候选配置请选择 candidate')
    if profile == 'candidate' and not fixture:
        raise ValueError('候选工作槽需要 --fixture')
    config = input_file(root, 'conf/creator-build.json')
    options = read(config)
    if (options.get('platform'), options.get('buildPath'), options.get('outputName')) != ('web-mobile', 'project://build', 'web-mobile'):
        raise ValueError('复用构建仅接受独立工程内的 Web Mobile 默认输出路径')
    fixture = input_file(root, fixture or FIXTURE)
    fixture_bytes = fixture.read_bytes()
    rules = json.loads(fixture_bytes)
    originals = creator.input_snapshot(source)
    projected = {}
    if profile == 'candidate':
        raw = input_file(root, public).read_bytes() if public else json_bytes(creator.public_gameplay(rules))
        projected = {SOURCE_PUBLIC: raw, PUBLIC_META: creator.content_meta_bytes('development/gameplay.json', raw)}
    client_rules = json.loads(projected.get(SOURCE_PUBLIC) or (source / SOURCE_PUBLIC).read_bytes())
    if type(rules.get('rule_version')) is not int or rules['rule_version'] <= 0 or client_rules.get('rule_version') != rules['rule_version']:
        raise ValueError('服务端夹具和客户端公开配置的规则版本不一致')
    expected = {**originals, **{name: hashlib.sha256(raw).hexdigest() for name, raw in projected.items()}}
    return projected, {
        'source_sha256': originals, 'project_sha256': expected,
        'public_sha256': expected[SOURCE_PUBLIC], 'rule_version': rules['rule_version'],
        'fixture': str(fixture), 'fixture_sha256': hashlib.sha256(fixture_bytes).hexdigest(),
        'build_config_sha256': creator.sha(config), 'selection': profile,
        'changed_inputs': sorted(name for name in expected if expected[name] != originals.get(name)),
        'normal_creator_project': True, 'runtime_resource_override': False,
        'only_public_changed': True, 'scene_prefab_preserved': True,
        'excluded_generated_editor_caches': creator.EDITOR_CACHES,
    }


def tree_entries(root):
    """遍历工作槽中的普通目录和文件，清理前拒绝任何重解析点。"""
    if is_reparse_point(root):
        raise ValueError(f'Creator 工作树含有重解析点：{root}')
    yield root
    if root.is_dir():
        for path in root.iterdir():
            yield from tree_entries(path)


def reset_generated(slot, project):
    """只移除当前工作槽的旧输出和旧回执，保留导入缓存。"""
    for path in (project / WEB_MOBILE, slot / 'preview-web-mobile', slot / 'preview-web-mobile.building',
                 slot / 'preview-game-alias.json', slot / 'creator-report.json', slot / 'creator.log'):
        checked_child(slot, path)
        if path.is_dir():
            for _ in tree_entries(path):
                pass
            shutil.rmtree(path)
        else:
            path.unlink(missing_ok=True)


def sync_inputs(root, project, projected, expected):
    """逐文件复用相同输入，删除源已移除的文件并更新发生变化的字节。"""
    source = root / 'client'
    project.mkdir(exist_ok=True)
    old_files, old_dirs = [], []
    for name in creator.SELECTED:
        start = project / name
        if start.exists() or is_reparse_point(start):
            for entry in tree_entries(start):
                (old_dirs if entry.is_dir() else old_files).append(entry)
    for entry in old_files:
        name = entry.relative_to(project).as_posix()
        if name not in expected and name not in creator.EDITOR_CACHES:
            entry.unlink()
    # 源目录改成文件时先移除原空目录，其余目录留给增量导入。
    for entry in reversed(old_dirs):
        name = entry.relative_to(project).as_posix()
        if name in expected or any(parent.relative_to(project).as_posix() in expected for parent in entry.parents if parent.is_relative_to(project)):
            entry.rmdir()
    for name in creator.SELECTED:
        if (source / name).is_dir():
            (project / name).mkdir(exist_ok=True)
    for name, digest in expected.items():
        output = project / name
        if output.is_file() and creator.sha(output) == digest:
            continue
        output.parent.mkdir(parents=True, exist_ok=True)
        if name in projected:
            output.write_bytes(projected[name])
        else:
            shutil.copy2(source / name, output)
    if creator.input_snapshot(project) != expected:
        raise RuntimeError('当前源和 Creator 工作槽输入摘要不一致')


def reuse_report(slot, project, expected):
    """只在已有成功实构且全部原输入和输出未漂移时复用。"""
    prepared = checked_child(slot, slot / 'project-copy.json')
    artifact = checked_child(slot, slot / 'creator-report.json')
    web = checked_child(slot, project / WEB_MOBILE)
    if not (prepared.is_file() and artifact.is_file() and web.is_dir()):
        return None
    try:
        earlier, report = read(prepared), read(artifact)
    except (json.JSONDecodeError, UnicodeDecodeError):
        return None
    if not isinstance(earlier, dict) or not isinstance(report, dict):
        return None
    # 服务端夹具变更但客户端投影不变时可以继续使用原 Creator 包。
    ignored = {'fixture', 'fixture_sha256'}
    if {key: value for key, value in earlier.items() if key not in ignored} != {
            key: value for key, value in expected.items() if key not in ignored}:
        return None
    if (not report.get('passed') or not report.get('source_stable') or report.get('exit_code') != 36
            or report.get('creator') != '3.8.8' or report.get('platform') != 'web-mobile'
            or report.get('project') != str(project) or report.get('rule_version') != expected['rule_version']
            or report.get('public_sha256') != expected['public_sha256']
            or report.get('input_sha256') != expected['project_sha256']
            or not isinstance(report.get('build_sha256'), dict) or 'index.html' not in report['build_sha256']):
        return None
    try:
        if creator.input_snapshot(project) != expected['project_sha256']:
            return None
    except ValueError:
        return None
    actual = {path.relative_to(web).as_posix(): creator.sha(path) for path in tree_entries(web) if path.is_file()}
    if actual == report['build_sha256']:
        return report
    return None


def freeze_build(root, name, slot, prepared, built):
    """按需冻结受控输入和产物，将导入缓存与工作槽留作下次复用。"""
    if not re.fullmatch('[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}', name) or name.endswith('.'):
        raise ValueError('发布证据名须为最多 64 个常规文件名字符')
    archives = workspace_creator_root(root, evidence=True)
    destination = checked_child(archives, archives / name)
    if destination.exists():
        raise ValueError(f'发布证据目录已存在：{destination}')
    archives.mkdir(parents=True, exist_ok=True)
    source = slot / 'client-project'
    with tempfile.TemporaryDirectory(dir=archives, prefix='.freeze-') as scratch:
        pending = checked_child(archives, Path(scratch) / 'artifact')
        project = pending / 'client-project'
        project.mkdir(parents=True)
        for directory in creator.SELECTED:
            if (source / directory).is_dir():
                (project / directory).mkdir()
        for name in (*prepared['project_sha256'], *(f'{WEB_MOBILE}/{name}' for name in built['build_sha256'])):
            target = project / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(source / name, target)
        if creator.input_snapshot(project) != prepared['project_sha256'] or creator.files(project / WEB_MOBILE) != built['build_sha256']:
            raise RuntimeError('冻结证据与原 Creator 工程或原产物不一致')
        source_log = checked_child(slot, slot / 'creator.log')
        shutil.copyfile(source_log, pending / 'creator.log')
        final_project = destination / 'client-project'
        creator.save(pending / 'project-copy.json', {**prepared, 'project': str(final_project)})
        creator.save(pending / 'creator-report.json', {**built, 'project': str(final_project), 'log': str(destination / 'creator.log')})
        creator.save(pending / 'frozen-from.json', {
            'workspace_creator_report_sha256': creator.sha(slot / 'creator-report.json'),
            'workspace_project_copy_sha256': creator.sha(slot / 'project-copy.json'),
            'creator_report_sha256': creator.sha(pending / 'creator-report.json'),
            'project_copy_sha256': creator.sha(pending / 'project-copy.json'),
        })
        checked_child(archives, pending).rename(checked_child(archives, destination))
    return destination


def build_slot(root, profile, *, fixture=None, public=None, force=False, freeze=None):
    """每种配置使用固定槽位，缺失或变更时在同一位置重建。"""
    if profile not in CREATOR_SLOTS:
        raise ValueError('未知 Creator 工作槽')
    root = Path(root).resolve(strict=True)
    slots = workspace_creator_root(root)
    slot = checked_child(slots, slots / profile)
    slot.mkdir(parents=True, exist_ok=True)
    project = checked_child(slot, slot / 'client-project')
    with creator_slot_lock(slot):
        owner = checked_child(slot, slot / '.creator-slot.json')
        identity = {'workspace': str(root), 'selection': profile}
        if owner.exists():
            if read(owner) != identity:
                raise ValueError(f'Creator 工作槽不属于当前项目：{slot}')
        else:
            if any(path.name != '.creator-slot.lock' for path in slot.iterdir()):
                raise ValueError(f'Creator 工作槽已被其它内容占用：{slot}')
            creator.save(owner, identity)
        projected, prepared = project_inputs(root, profile, fixture, public)
        prepared['project'] = str(project)
        built = None if force else reuse_report(slot, project, prepared)
        reused = built is not None
        if not reused:
            sync_inputs(root, project, projected, prepared['project_sha256'])
            reset_generated(slot, project)
            creator.save(slot / 'project-copy.json', prepared)
            built = creator.build_project(slot, prepared, keep_import_cache=True)
            if creator.input_snapshot(root / 'client') != prepared['source_sha256']:
                checked_child(slot, slot / 'creator-report.json').unlink()
                raise RuntimeError('实构期间原项目输入已变更，请重新运行当前工作槽')
        elif read(slot / 'project-copy.json') != prepared:
            creator.save(slot / 'project-copy.json', prepared)
        snapshot = freeze_build(root, freeze, slot, prepared, built) if freeze else None
        return {'workspace': str(slot), 'web': str(project / WEB_MOBILE),
                'creator_rerun': not reused, 'frozen_evidence': str(snapshot) if snapshot else None}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('profile', choices=CREATOR_SLOTS)
    parser.add_argument('--fixture', type=Path, help='候选服务端夹具')
    parser.add_argument('--public', type=Path, help='候选编译器已经导出的客户端公开配置')
    parser.add_argument('--force', action='store_true', help='在相同工作槽重新执行实构')
    parser.add_argument('--freeze', help='用单独目录冻结本次构建的受控输入、输出和报告')
    args = parser.parse_args()
    try:
        result = build_slot(ROOT, args.profile, fixture=args.fixture, public=args.public, force=args.force, freeze=args.freeze)
    except (OSError, ValueError, RuntimeError) as exc:
        parser.exit(1, f'{exc}\n')
    print(json.dumps(result, ensure_ascii=False), flush=True)


if __name__ == '__main__':
    main()
