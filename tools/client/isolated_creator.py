"""复制独立Creator工程并真实构建；局部public来自同一配置编译器，不替换运行时资源。"""
from __future__ import annotations
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[2]
SELECTED = ['assets', 'settings', 'profiles', '.creator', 'node_modules', 'package.json', 'package-lock.json', 'tsconfig.json']
EDITOR_CACHES = ['settings/v2/packages/information.json']
sys.path.insert(0, str(ROOT / 'tools/resources'))
from prepare_first_loop import public_gameplay


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def save(path, value):
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')


def work_path(value):
    path = Path(value).resolve()
    if not path.is_relative_to((ROOT / '.work').resolve()) or path == (ROOT / '.work').resolve():
        raise ValueError('独立工程必须位于本项目.work子目录')
    return path


def files(directory):
    directory = Path(directory)
    return {p.relative_to(directory).as_posix(): sha(p) for p in sorted(directory.rglob('*')) if p.is_file()}


def input_snapshot(project):
    project = Path(project)
    snapshot = {}
    for name in SELECTED:
        path = project / name
        if not path.exists():
            raise ValueError('Creator工程缺少受控输入：' + name)
        candidates = [path] if path.is_file() else [path, *path.rglob('*')]
        for p in candidates:
            if p.relative_to(project).as_posix() in EDITOR_CACHES:
                continue
            if p.is_symlink() or getattr(p, 'is_junction', lambda: False)():
                raise ValueError('受控输入不能通过链接指向外部资源：' + str(p))
            if p.is_file():
                snapshot[p.relative_to(project).as_posix()] = sha(p)
    return snapshot


def prepare_project(directory, fixture):
    directory = work_path(directory)
    project = directory / 'client-project'
    if project.exists():
        raise ValueError('保留旧工程与构建，必须使用新目录')
    source = ROOT / 'client'
    snapshot = input_snapshot(source)
    project.mkdir(parents=True)
    for name in SELECTED:
        path = source / name
        if path.is_dir():
            def omit_editor_cache(parent, children):
                return [child for child in children if (Path(parent) / child).relative_to(source).as_posix() in EDITOR_CACHES]
            shutil.copytree(path, project / name, ignore=omit_editor_cache)
        elif path.is_file():
            shutil.copy2(path, project / name)
        else:
            raise ValueError('Creator工程缺少受控输入：' + name)
    if files(project) != snapshot:
        raise RuntimeError('独立工程复制结果与输入字节不一致')
    public = public_gameplay(fixture)
    public_path = project / 'assets/resources/development/gameplay.json'
    save(public_path, public)
    expected = dict(snapshot)
    expected['assets/resources/development/gameplay.json'] = sha(public_path)
    if files(project) != expected:
        raise RuntimeError('除编译器生成public外的工程输入发生变化')
    report = {'project': str(project), 'source_sha256': snapshot, 'project_sha256': expected,
              'public_sha256': sha(public_path), 'rule_version': fixture['rule_version'],
              'normal_creator_project': True, 'runtime_resource_override': False,
              'only_public_changed': True, 'scene_prefab_preserved': True,
              'excluded_generated_editor_caches': EDITOR_CACHES}
    save(directory / 'project-copy.json', report)
    return report


def build_project(directory, prepared):
    directory = work_path(directory)
    project = work_path(prepared['project'])
    if project.parent != directory or files(project) != prepared['project_sha256']:
        raise RuntimeError('独立工程输入已变化，拒绝构建旧资格证据')
    app = Path(os.environ.get('MINIMIR_CREATOR_EXECUTABLE', ''))
    if not app.is_file():
        raise ValueError('统一工具链未提供Creator3.8.8')
    log = directory / 'creator.log'
    command = [str(app)] + (['--no-sandbox', '--disable-gpu'] if os.name == 'nt' else [])
    command += ['--project', str(project), '--build', 'configPath=' + str(ROOT / 'conf/creator-build.json')]
    started = time.monotonic()
    with log.open('wb') as output:
        result = subprocess.run(command, cwd=ROOT, stdout=output, stderr=subprocess.STDOUT)
    after = input_snapshot(project)
    # Creator允许自己的library/temp/build输出；受控工程输入必须保持。
    stable = after == prepared['project_sha256']
    report = {'creator': '3.8.8', 'platform': 'web-mobile', 'project': str(project),
              'exit_code': result.returncode, 'seconds': round(time.monotonic() - started, 2),
              'source_stable': stable, 'input_sha256': after, 'public_sha256': prepared['public_sha256'],
              'rule_version': prepared['rule_version'], 'log': str(log),
              'excluded_generated_editor_caches': EDITOR_CACHES,
              'generated_editor_cache_sha256': {name: sha(project / name) for name in EDITOR_CACHES if (project / name).is_file()},
              'physical_mobile_verified': False}
    report['passed'] = result.returncode == 36 and stable and (project / 'build/web-mobile/index.html').is_file()
    if report['passed']:
        report['build_sha256'] = files(project / 'build/web-mobile')
    save(directory / 'creator-report.json', report)
    if not report['passed']:
        raise RuntimeError('独立Creator实构失败，保留creator.log及报告')
    return report


def reuse_project(directory, artifact_directory, fixture):
    """只复用本轮真实构建的不变产物；任何源、public、工程或产物漂移均拒绝。"""
    directory, artifact_directory = work_path(directory), work_path(artifact_directory)
    project = artifact_directory / 'client-project'
    if directory == artifact_directory or not project.is_dir():
        raise ValueError('复用必须指定另一次真实构建的独立工程')
    original_copy = artifact_directory / 'project-copy.json'
    original_build = artifact_directory / 'creator-report.json'
    prepared = json.loads(original_copy.read_text(encoding='utf-8'))
    built = json.loads(original_build.read_text(encoding='utf-8'))
    if Path(prepared['project']).resolve() != project or Path(built['project']).resolve() != project or not prepared.get('only_public_changed') or prepared.get('runtime_resource_override') is not False or not built.get('passed') or not built.get('source_stable') or built.get('exit_code') != 36 or built.get('creator') != '3.8.8' or built.get('platform') != 'web-mobile' or built.get('rule_version') != fixture['rule_version']:
        raise RuntimeError('复用来源不是具有完整输入证据的真实Creator包')
    if prepared.get('excluded_generated_editor_caches') != EDITOR_CACHES or built.get('excluded_generated_editor_caches') != EDITOR_CACHES:
        raise RuntimeError('复用来源没有明确相同的编辑器缓存边界')
    source = input_snapshot(ROOT / 'client')
    if source != prepared['source_sha256']:
        raise RuntimeError('主工程输入已变化，必须重新真实构建')
    public = (json.dumps(public_gameplay(fixture), ensure_ascii=False, indent=2) + '\n').encode('utf-8')
    public_sha = hashlib.sha256(public).hexdigest()
    expected = {**source, 'assets/resources/development/gameplay.json': public_sha}
    if expected != prepared['project_sha256'] or expected != built['input_sha256'] or public_sha != built['public_sha256'] or public_sha != prepared['public_sha256'] or input_snapshot(project) != expected:
        raise RuntimeError('候选public或独立工程输入已变化，不能复用构建')
    if not built.get('build_sha256') or 'index.html' not in built['build_sha256'] or files(project / 'build/web-mobile') != built['build_sha256']:
        raise RuntimeError('独立构建产物缺失或已变化，不能复用')
    proof = {'passed': True, 'artifact_directory': str(artifact_directory), 'project': str(project),
             'creator_report_sha256': sha(original_build), 'project_copy_sha256': sha(original_copy),
             'public_sha256': public_sha, 'current_source_matches': True, 'isolated_inputs_match': True,
             'build_outputs_match': True, 'source_files': len(source), 'build_files': len(built['build_sha256']),
             'runtime_resource_override': False, 'creator_rerun': False}
    shutil.copyfile(original_copy, directory / 'project-copy.json')
    shutil.copyfile(original_build, directory / 'creator-report.json')
    save(directory / 'reused-artifact.json', proof)
    return prepared, built, proof
