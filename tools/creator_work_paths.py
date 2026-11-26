"""定位项目专用的 Creator 临时工程与只读验收产物目录。"""

import hashlib
import os
from pathlib import Path


def local_creator_work(project_root):
    """Windows 可把大型构建放在按项目绝对路径区分的本机应用数据目录。"""
    directory = os.environ.get('LOCALAPPDATA')
    if os.name != 'nt' or not directory:
        return None
    local_data = Path(directory).resolve(strict=True)
    project = os.path.normcase(str(Path(project_root).resolve(strict=True)))
    project_id = hashlib.sha256(project.encode('utf-8')).hexdigest()[:16]
    owned = local_data / 'minimir-creator-work' / project_id
    if owned.resolve() != owned:
        raise ValueError('本机 Creator 工作目录重定向到项目隔离区之外')
    return owned


def creator_work_path(value, project_root, *, strict=False):
    """只允许项目 .work 或本机应用数据中这个项目的 Creator 子目录。"""
    path = Path(value).resolve(strict=strict)
    roots = [(Path(project_root) / '.work').resolve()]
    owned = local_creator_work(project_root)
    if owned is not None:
        roots.append(owned)
    if all(path == root or not path.is_relative_to(root) for root in roots):
        raise ValueError('独立 Creator 工程和验收产物须位于本项目的工作子目录')
    return path
