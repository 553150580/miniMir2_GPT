"""Windows Lima 专属 VM 边界；只管理本项目登记的目录、镜像和二进制。"""
import contextlib
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]
PROFILE = ROOT / 'conf/toolchain/windows-amd64.lock.json'


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def settings():
    profile = json.loads(PROFILE.read_text(encoding='utf-8'))
    prefix = Path(os.path.expandvars(profile['prefix']))
    home = Path(os.path.expandvars(profile['lima_home']))
    binary = prefix / profile['tools']['lima']['executable']
    name = profile['virtual_machine']['name']
    return profile, prefix, home, binary, name


@contextlib.contextmanager
def control_lock():
    import msvcrt
    path = ROOT / '.work/environment/windows-control.lock'
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.is_symlink():
        raise RuntimeError('控制锁不能是符号链接')
    with path.open('a+b') as stream:
        if stream.tell() == 0:
            stream.write(b'0')
            stream.flush()
        stream.seek(0)
        try:
            msvcrt.locking(stream.fileno(), msvcrt.LK_NBLCK, 1)
        except OSError as error:
            raise RuntimeError('本项目已有 VM/数据库控制操作正在执行') from error
        try:
            yield
        finally:
            stream.seek(0)
            msvcrt.locking(stream.fileno(), msvcrt.LK_UNLCK, 1)


def create():
    profile, prefix, home, binary, name = settings()
    marker = home / '_config' / f'{name}.owner.json'
    if marker.exists():
        return load()
    directory = home / name
    if directory.exists():
        raise RuntimeError('目标 VM 目录已存在但没有本项目标记，拒绝接管')
    spec = profile['virtual_machine']
    image = prefix / 'lima/images/ubuntu-24.04-minimal-cloudimg-amd64.img'
    image.parent.mkdir(parents=True, exist_ok=True)
    if not image.is_file():
        temporary = image.with_suffix('.partial')
        urllib.request.urlretrieve(spec['image_url'], temporary)
        if sha(temporary) != spec['image_sha256']:
            raise RuntimeError('Ubuntu 官方镜像摘要不符')
        temporary.replace(image)
    if image.is_symlink() or sha(image) != spec['image_sha256']:
        raise RuntimeError('Ubuntu 镜像缓存不符')
    template = ROOT / spec['template']
    rendered = ROOT / '.work/environment/minimir-dev.windows.lima.yaml'
    rendered.parent.mkdir(parents=True, exist_ok=True)
    rendered.write_text(template.read_text(encoding='utf-8').replace('@VERIFIED_IMAGE@', image.as_posix()), encoding='utf-8')
    owner = dict(project_root=str(ROOT), name=name, directory=str(directory), lima_binary_sha256=sha(binary),
                 template_sha256=sha(template), image_sha256=sha(image), creation_id=uuid.uuid4().hex)
    marker.parent.mkdir(parents=True, exist_ok=True)
    # 先登记创建意图，异常后保留现场；不会把已有且未登记的 VM 当成本项目实例。
    with marker.open('x', encoding='utf-8') as stream:
        json.dump(owner, stream, ensure_ascii=False, indent=2)
    try:
        run(['start', '--name=' + name, '--tty=false', '--timeout=900s', str(rendered)], timeout=960)
    finally:
        config = directory / 'lima.yaml'
        if config.is_file():
            owner['vm_config_sha256'] = sha(config)
            marker.write_text(json.dumps(owner, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    return load()


def load():
    profile, prefix, home, binary, name = settings()
    marker = home / '_config' / f'{name}.owner.json'
    if marker.is_symlink() or not marker.is_file():
        raise RuntimeError('缺少本项目 Windows VM 所有权标记；不会接管既有 VM')
    owner = json.loads(marker.read_text(encoding='utf-8'))
    if owner['project_root'] != str(ROOT) or owner['name'] != name or Path(owner['directory']).resolve() != (home / name).resolve():
        raise RuntimeError('Windows VM 所有权与当前项目不一致')
    if binary.is_symlink() or sha(binary) != owner['lima_binary_sha256'] or sha(home / name / 'lima.yaml') != owner['vm_config_sha256']:
        raise RuntimeError('VM 配置或 Lima 二进制变化，拒绝管理')
    return owner


def run(arguments, timeout=180, check=True):
    _, _, _, binary, _ = settings()
    result = subprocess.run([str(binary)] + list(arguments), cwd=ROOT, capture_output=True,
                            encoding='utf-8', errors='replace', timeout=timeout)
    if check and result.returncode:
        raise RuntimeError('Lima 命令失败：' + result.stderr[-2500:] + result.stdout[-1500:])
    return result


def json_output(text):
    for line in reversed(text.splitlines()):
        try:
            return json.loads(line)
        except ValueError:
            pass
    raise RuntimeError('控制命令没有返回有效 JSON')


def info():
    owner = load()
    result = json_output(run(['list', '--json', owner['name']]).stdout)
    if result['name'] != owner['name'] or Path(result['dir']).resolve() != Path(owner['directory']).resolve():
        raise RuntimeError('Lima 返回了其它 VM，拒绝继续')
    return result


def guest(arguments, timeout=180):
    owner = load()
    return run(['--tty=false', 'shell', '--workdir=/tmp', owner['name'], '--'] + list(arguments), timeout=timeout)


def copy_to_guest(source, destination):
    current = info()
    config = Path(current['sshConfigFile'])
    if config.is_symlink() or config.resolve() != (Path(current['dir']) / 'ssh.config').resolve():
        raise RuntimeError('VM SSH 配置目录不符')
    # Lima copy 在 Windows 把盘符改成 /c，和系统 OpenSSH 不兼容；沿同一已核验 SSH 配置直接调用 scp。
    executable = Path(os.environ['SystemRoot']) / 'System32/OpenSSH/scp.exe'
    result = subprocess.run([str(executable), '-F', str(config), str(Path(source).resolve()),
                             f'lima-{current["name"]}:{destination}'], capture_output=True, timeout=300)
    if result.returncode:
        raise RuntimeError('专属 guest 文件传输失败：' + result.stderr.decode('utf-8', errors='replace')[-1800:])


def start():
    owner = load()
    if info()['status'].lower() != 'running':
        run(['start', '--tty=false', '--timeout=900s', owner['name']], timeout=960)
    guest(['true'])
    return dict(running=True, backend='lima_linux_vm', acceleration='tcg', capacity_verified=False)


def stop():
    if info()['status'].lower() != 'running':
        return dict(running=False)
    # Windows 2.2.1 的 hostagent SIGINT 可能返回参数错误；通过已核验 guest 的正常关机路径停止。
    shutdown_error = None
    try:
        guest(['sudo', 'poweroff'], timeout=180)
    except (RuntimeError, subprocess.TimeoutExpired) as error:
        # 正常关机会关闭SSH；命令断开不代表失败，仍以登记VM的最终状态为准。
        shutdown_error = str(error)
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        if info()['status'].lower() != 'running':
            return dict(running=False, graceful_shutdown=True)
        time.sleep(1)
    raise RuntimeError('专属 VM 尚未完成正常关机，未强杀：' + (shutdown_error or '等待状态超时'))


def main():
    action = sys.argv[1] if len(sys.argv) > 1 else 'status'
    with control_lock():
        result = {'init': create, 'start': start, 'stop': stop, 'status': info}[action]()
    print(json.dumps(result, ensure_ascii=False))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
