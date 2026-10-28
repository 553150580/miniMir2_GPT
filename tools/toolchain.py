"""Windows 工具分发：版本来自平台锁，安装和缓存只在用户目录。"""
from pathlib import Path
import json
import os
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]


def windows_paths():
    lock = json.loads((ROOT / 'conf/toolchain/windows-amd64.lock.json').read_text(encoding='utf-8'))
    prefix = Path(os.path.expandvars(lock['prefix']))
    return lock, prefix


def creator_app():
    explicit = os.environ.get('MINIMIR_CREATOR_APP')
    candidates = [Path(explicit)] if explicit else [
        Path('D:/Program Files/CocosCreator/3.8.8'),
        Path(os.environ.get('ProgramFiles', 'C:/Program Files')) / 'CocosCreator/3.8.8',
        Path(os.environ.get('ProgramFiles', 'C:/Program Files')) / 'Cocos/Creator/3.8.8',
    ]
    for path in candidates:
        if (path / 'CocosCreator.exe').is_file():
            return path.resolve()
    raise RuntimeError('未找到 Creator 3.8.8；请将 MINIMIR_CREATOR_APP 指向安装目录')


def windows_main(arguments):
    lock, prefix = windows_paths()
    python = prefix / 'python/tools-venv/Scripts/python.exe'
    if not python.is_file():
        raise SystemExit('请先运行 tools/environment/bootstrap_windows.ps1 初始化锁定工具')
    env = os.environ.copy()
    for key in ('__PYVENV_LAUNCHER__', 'PYTHONHOME', 'PYTHONPATH', 'PYTHONUSERBASE'):
        env.pop(key, None)
    executables = {name: prefix / spec['executable'] for name, spec in lock['tools'].items()}
    executables.update({name: Path(os.path.expandvars(spec['executable'])) for name, spec in lock.get('host_tools', {}).items()})
    executables['python3'] = executables['python'] = python
    executables['limactl'] = executables.pop('lima')
    executables['protoc-gen-go'] = prefix / 'go-tools/bin/protoc-gen-go.exe'
    executables['gofmt'] = executables['go'].parent / 'gofmt.exe'
    executables['qemu-system-x86_64'] = executables.pop('qemu')
    directories = [python.parent, prefix / 'go-tools/bin', prefix / 'node-tools/node_modules/.bin']
    directories.extend(path.parent for path in executables.values())
    env['PATH'] = os.pathsep.join(dict.fromkeys(map(str, directories))) + os.pathsep + env.get('PATH', '')
    env.update({
        'MINIMIR_TOOLCHAIN_ACTIVE': '1', 'MINIMIR_PYTHON': str(python),
        'MINIMIR_TOOLCHAIN_ROOT': str(prefix), 'PYTHONUTF8': '1', 'PYTHONIOENCODING': 'utf-8',
        'MINIMIR_NODE_TOOLS_ROOT': str(prefix / 'node-tools'),
        'MINIMIR_NODE_TOOLS': str(prefix / 'node-tools'),
        'GOMODCACHE': str(prefix / 'cache/go-mod'), 'GOCACHE': str(prefix / 'cache/go-build'),
        'GOPATH': str(prefix / 'cache/go-path'), 'GOBIN': str(prefix / 'go-tools/bin'),
        'GOTOOLCHAIN': 'local', 'GOPROXY': 'https://goproxy.cn,https://proxy.golang.org,direct',
        'CC': str(executables['gcc']),
        'GOSUMDB': 'sum.golang.org https://goproxy.cn/sumdb/sum.golang.org',
        'LIMA_HOME': os.path.expandvars(lock['lima_home']),
    })
    env.update(DOTNET_ROOT=str(prefix / 'dotnet/10.0.403'),
               DOTNET_CLI_HOME=str(prefix / 'cache/dotnet-cli'),
               DOTNET_CLI_TELEMETRY_OPTOUT='1', DOTNET_SKIP_FIRST_TIME_EXPERIENCE='1',
               DOTNET_NOLOGO='1', DOTNET_ADD_GLOBAL_TOOLS_TO_PATH='false',
               DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE='true',
               NUGET_PACKAGES=str(prefix / 'cache/nuget'))
    # Lima 2.2.1 在 Windows 默认选择 WHPX；本机未启用该系统功能，使用其公开的 QEMU 参数入口选 TCG。
    env['QEMU_SYSTEM_X86_64'] = '"' + executables['qemu-system-x86_64'].as_posix() + '" -machine q35,accel=tcg -cpu max'
    try:
        app = creator_app()
        env.update(MINIMIR_CREATOR_APP=str(app), COCOS_CREATOR_PATH=str(app),
                   MINIMIR_CREATOR_EXECUTABLE=str(app / 'CocosCreator.exe'))
    except RuntimeError:
        pass
    args = list(arguments)
    if not args:
        print('Windows 用法：./tools/dev.ps1 <go|python3|node|protoc|task|其他命令> [参数...]')
        return 0
    cwd = ROOT / 'server' if args[0] == 'go' else ROOT
    if args[0] == '--cwd':
        if len(args) < 3:
            raise SystemExit('--cwd需要项目内目录和命令')
        cwd = (ROOT / args[1]).resolve()
        if not cwd.is_relative_to(ROOT):
            raise SystemExit('--cwd必须在本项目内')
        args = args[2:]
    command = args[0]
    if command == 'task':
        args = [str(python), str(ROOT / 'tools/tasks.py')] + args[1:]
    elif command in ('npm', 'npx'):
        node = executables['node']
        args = [str(node), str(node.parent / f'node_modules/npm/bin/{command}-cli.js')] + args[1:]
    elif command in ('pbjs', 'pbts'):
        args = [str(executables['node']), str(prefix / f'node-tools/node_modules/protobufjs-cli/bin/{command}')] + args[1:]
    elif command in executables:
        executable = executables[command]
        if not executable.is_file():
            raise SystemExit(f'锁定工具未安装：{command} ({executable})')
        args[0] = str(executable)
    result = subprocess.run(args, cwd=cwd, env=env)
    return result.returncode
