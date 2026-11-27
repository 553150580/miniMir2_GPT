"""定位项目专用的 Creator 临时工程与只读验收产物目录。"""

import hashlib
import os
from pathlib import Path
import stat
from contextlib import contextmanager


CREATOR_SLOTS = ('default', 'candidate')


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
    workspace = (Path(project_root) / '.work').resolve()
    if path != workspace and path.is_relative_to(workspace):
        return path
    owned = local_creator_work(project_root)
    if owned is not None and path != owned and path.is_relative_to(owned):
        return path
    raise ValueError('独立 Creator 工程和验收产物须位于本项目的工作子目录')


def workspace_creator_root(project_root, *, evidence=False):
    """以项目工作目录为根，保持活跃构建与需留存的证据分开。"""
    workspace = Path(project_root).resolve(strict=True) / '.work'
    root = workspace / ('creator-evidence' if evidence else 'creator-work')
    if root.resolve() != root or is_reparse_point(workspace) or is_reparse_point(root):
        raise ValueError('项目的 Creator 工作目录含有重解析路径')
    return root


def is_reparse_point(path):
    """识别 Windows 重解析点及其它平台的符号链接。"""
    try:
        info = path.lstat()
    except FileNotFoundError:
        return False
    return stat.S_ISLNK(info.st_mode) or bool(getattr(info, 'st_file_attributes', 0) & stat.FILE_ATTRIBUTE_REPARSE_POINT)


def checked_child(root, path):
    """在读写或清理前固定实际父路径，并拒绝重解析路径和越界。"""
    root, path = Path(root), Path(path)
    if not root.is_absolute() or path == root or not path.is_relative_to(root) or '..' in path.parts:
        raise ValueError(f'Creator 工作路径越界：{path}')
    for parent in (path, *path.parents):
        if is_reparse_point(parent):
            raise ValueError(f'Creator 工作路径含有重解析点：{parent}')
        if parent == root:
            break
    if path.resolve() != path:
        raise ValueError(f'Creator 工作路径解析后不一致：{path}')
    return path


@contextmanager
def creator_slot_lock(slot):
    """同槽构建与封装共用一个系统文件锁，进程退出时自动释放。"""
    slot = Path(slot)
    lock = checked_child(slot, slot / '.creator-slot.lock')
    with lock.open('a+b', buffering=0) as handle:
        if os.name == 'nt':
            import msvcrt
            # 锁文件始终保留，不可 unlink 后另开同名文件绕过当前持有者。
            if handle.seek(0, os.SEEK_END) == 0:
                handle.write(b'\0')
            handle.seek(0)
            try:
                msvcrt.locking(handle.fileno(), msvcrt.LK_NBLCK, 1)
            except OSError as exc:
                raise RuntimeError(f'Creator 工作槽正被其它操作使用：{slot}') from exc
        else:
            import fcntl
            try:
                fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError as exc:
                raise RuntimeError(f'Creator 工作槽正被其它操作使用：{slot}') from exc
        yield
