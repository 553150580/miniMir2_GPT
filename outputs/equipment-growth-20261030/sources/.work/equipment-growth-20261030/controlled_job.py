"""本阶段私有后台执行器：仅固定探针/已冻结切换；没有停止、端口或任意命令接口。"""
from __future__ import annotations

import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import uuid

SELF = Path(__file__).resolve()
WORK = SELF.parent
ROOT = WORK.parents[1]
JOBS = WORK / 'controlled-jobs'
CUTOVER = WORK / 'cutover.py'
CUTOVER_SHA256 = 'ed07059fd28023b842466176a6dde3f19eb7706d13a188e0ba488465b4997980'
sys.path.insert(0, str(ROOT / 'tools'))
from process_control import background_options, windows_identity, windows_owned


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def write(path, value):
    temporary = path.with_name(path.name + '.' + uuid.uuid4().hex + '.tmp')
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    os.replace(temporary, path)


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def folder(task, phase):
    if task not in ('probe', 'cutover') or not re.fullmatch(r'[a-z][a-z0-9-]{0,47}', phase):
        raise ValueError('仅允许固定任务和短阶段目录名')
    if phase != task and not phase.startswith(task + '-'):
        raise ValueError('阶段目录必须以本任务名称开头')
    expected = JOBS / phase
    if expected.resolve() != expected.absolute() or not expected.resolve().is_relative_to(WORK):
        raise ValueError('阶段目录越界或存在路径重定向')
    for path in (WORK, JOBS, expected):
        if path.exists() and (path.is_symlink() or path.is_junction()):
            raise ValueError('执行目录不能是链接或 junction')
    return expected


def snapshot(task):
    files = [SELF, ROOT / 'tools/process_control.py', ROOT / 'tools/dev',
             ROOT / 'tools/dev.ps1', ROOT / 'tools/toolchain.py',
             ROOT / 'conf/toolchain/windows-amd64.lock.json']
    if task == 'cutover':
        if sha(CUTOVER) != CUTOVER_SHA256:
            raise RuntimeError('切换脚本并非授权冻结字节，拒绝运行')
        files.append(CUTOVER)
    return {p.relative_to(ROOT).as_posix(): sha(p) for p in files}


def base_python():
    # dev.ps1 已选择锁定解释器；不用 venv 的短启动器当后台 PID。
    candidate = Path(getattr(sys, '_base_executable', sys.executable)).resolve()
    lock = read(ROOT / 'conf/toolchain/windows-amd64.lock.json')
    prefix = Path(os.path.expandvars(lock['prefix'])).resolve()
    if os.name != 'nt' or not candidate.is_file() or not candidate.is_relative_to(prefix / 'python/managed'):
        raise RuntimeError('此私有执行器只接受 tools/dev 选择的 Windows 锁定 Python')
    if sys.version.split()[0] != lock['python_version']:
        raise RuntimeError('运行 Python 版本不等于 Windows 锁定版本')
    return candidate


def exact_record(out, task, phase):
    launch = read(out / 'launch.json')
    if launch.get('project_root') != str(ROOT) or launch.get('directory') != str(out):
        raise RuntimeError('启动记录不属于本项目精确目录')
    if launch.get('task') != task or launch.get('phase') != phase or not re.fullmatch(r'[0-9a-f]{32}', launch.get('run_id', '')):
        raise RuntimeError('任务、阶段或 run_id 不匹配')
    if launch.get('worker_argv') != [launch['python_executable'], str(SELF), '_worker', task, '--phase', phase, '--run-id', launch['run_id']]:
        raise RuntimeError('启动记录不是固定 worker 命令')
    return launch


def process_matches(meta, launch):
    return bool(meta and meta.get('run_id') == launch['run_id'] and meta.get('task') == launch['task']
                and meta.get('phase') == launch['phase'] and meta.get('project_root') == str(ROOT)
                and meta.get('argv') == launch['worker_argv']
                and windows_owned(meta, launch['python_executable']))


def command_line_matches(pid, expected):
    # 在已经核对映像/创建时钟后，只读核对真实进程命令行中的 run_id；不停止进程。
    query = (f"$p=Get-CimInstance Win32_Process -Filter 'ProcessId={int(pid)}'; "
             "if($null -ne $p){$p.CommandLine | ConvertTo-Json -Compress}")
    result = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', query],
                            stdin=subprocess.DEVNULL, capture_output=True, text=True, encoding='utf-8',
                            errors='strict', timeout=10, creationflags=subprocess.CREATE_NO_WINDOW)
    if result.returncode != 0:
        raise RuntimeError('只读进程命令行核验失败：' + result.stderr.strip())
    return bool(result.stdout.strip() and json.loads(result.stdout) == subprocess.list2cmdline(expected))


def status(task, phase):
    out = folder(task, phase)
    launch = exact_record(out, task, phase)
    meta = read(out / 'worker.json') if (out / 'worker.json').is_file() else None
    terminal_path = out / 'terminal.json'
    observed = windows_identity(meta['pid']) if meta else None
    owned = process_matches(meta, launch)
    if owned:
        owned = command_line_matches(meta['pid'], launch['worker_argv'])
        # CIM 查询期间进程也可能结束，必须再次检查原创建时钟。
        owned = owned and process_matches(meta, launch)
    result = {'task': task, 'phase': phase, 'run_id': launch['run_id'], 'directory': str(out),
              'observed_at_utc': now(), 'live': bool(owned), 'worker': meta,
              'actual_identity': observed, 'terminal_present': terminal_path.is_file()}
    if terminal_path.is_file():
        terminal = read(terminal_path)
        if terminal.get('run_id') != launch['run_id'] or terminal.get('worker') != meta:
            raise RuntimeError('终态不属于原 worker/run_id')
        for stream in ('stdout', 'stderr'):
            if sha(out / (stream + '.log')) != terminal[stream + '_sha256']:
                raise RuntimeError('终态后原日志字节变化')
        result.update(status='completed' if terminal['worker_exit_code'] == 0 else 'failed',
                      passed=terminal['passed'], terminal=terminal, terminal_sha256=sha(terminal_path))
    elif owned:
        result.update(status='live', passed=False)
    else:
        result.update(status='stopped_without_terminal', passed=False,
                      reason='没有原 PID 身份或原 run_id 进程，且没有实际终态；不会自动重启')
    return result


def start(task, phase):
    out = folder(task, phase)
    before = snapshot(task)
    python = base_python()
    JOBS.mkdir(exist_ok=True)
    out.mkdir(exist_ok=False)
    run_id = uuid.uuid4().hex
    argv = [str(python), str(SELF), '_worker', task, '--phase', phase, '--run-id', run_id]
    launch = {'task': task, 'phase': phase, 'run_id': run_id, 'project_root': str(ROOT),
              'directory': str(out), 'created_at_utc': now(), 'python_executable': str(python),
              'python_sha256': sha(python), 'worker_argv': argv, 'source_before': before,
              'background': 'tools.process_control.background_options / CREATE_NEW_CONSOLE hidden',
              'automatic_restart': False, 'stop_api': False}
    write(out / 'launch.json', launch)
    with (out / 'stdout.log').open('xb') as stdout, (out / 'stderr.log').open('xb') as stderr:
        child = subprocess.Popen(argv, cwd=ROOT, stdin=subprocess.DEVNULL, stdout=stdout,
                                 stderr=stderr, close_fds=True, **background_options())
    write(out / 'spawn.json', {'run_id': run_id, 'pid': child.pid, 'spawned_at_utc': now()})
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        if (out / 'worker.json').is_file():
            meta = read(out / 'worker.json')
            if meta['pid'] != child.pid or not process_matches(meta, launch):
                raise RuntimeError('后台真实 PID/创建时钟/映像不匹配，未授权执行任务；不停止未知进程')
            # 单次批准绑定原 PID 和 run_id，worker 等到它才运行固定任务。
            write(out / 'approved.json', {'run_id': run_id, 'worker': meta, 'approved_at_utc': now()})
            return {'status': 'started', 'live': True, 'task': task, 'phase': phase, 'run_id': run_id,
                    'worker': meta, 'directory': str(out), 'source_before': before}
        if child.poll() is not None:
            break
        time.sleep(0.02)
    return status(task, phase)


def worker(task, phase, run_id):
    out = folder(task, phase)
    launch = exact_record(out, task, phase)
    if launch['run_id'] != run_id or str(Path(sys.executable).resolve()) != launch['python_executable']:
        raise RuntimeError('worker 的原 run_id/锁定解释器不符')
    identity = windows_identity(os.getpid())
    if not identity:
        raise RuntimeError('无法取得 worker 真实 Windows 进程身份')
    meta = {**identity, 'run_id': run_id, 'task': task, 'phase': phase,
            'project_root': str(ROOT), 'argv': launch['worker_argv']}
    # 写入前不允许覆盖其它 worker 的登记。
    if (out / 'worker.json').exists() or (out / 'terminal.json').exists():
        raise RuntimeError('worker 证据已存在，拒绝重用目录')
    write(out / 'worker.json', meta)
    terminal = {'run_id': run_id, 'task': task, 'phase': phase, 'worker': meta,
                'started_at_utc': now(), 'source_before': launch['source_before'],
                'passed': False, 'exit_code': None, 'automatic_restart': False}
    began = time.monotonic()
    code = 1
    try:
        deadline = time.monotonic() + 10
        while not (out / 'approved.json').is_file():
            if time.monotonic() >= deadline:
                raise RuntimeError('启动批准未到；固定任务未运行')
            time.sleep(0.02)
        approved = read(out / 'approved.json')
        if approved['run_id'] != run_id or approved['worker'] != meta:
            raise RuntimeError('批准不属于本 worker')
        if snapshot(task) != launch['source_before'] or sha(Path(launch['python_executable'])) != launch['python_sha256']:
            raise RuntimeError('批准后执行源码或锁定解释器变化，拒绝任务')
        if task == 'probe':
            # 固定 2 秒，无业务、网络、端口或服务调用；真实子进程退出码写入终态。
            program = 'import time; print("fixed probe started", flush=True); time.sleep(2); print("fixed probe completed", flush=True)'
            command = [launch['python_executable'], '-c', program]
            terminal['probe_duration_seconds'] = 2
        else:
            # 不能传递任何新参数；所有 Go3/Creator/current 安全门禁仍由原冻结脚本执行。
            if sha(CUTOVER) != CUTOVER_SHA256:
                raise RuntimeError('切换脚本冻结摘要变化')
            command = [launch['python_executable'], str(CUTOVER)]
        terminal['task_command'] = command
        print(json.dumps({'run_id': run_id, 'task': task, 'phase': phase, 'task_started_at_utc': now()}, ensure_ascii=False), flush=True)
        process = subprocess.Popen(command, cwd=ROOT, stdin=subprocess.DEVNULL, close_fds=True)
        task_identity = windows_identity(process.pid)
        write(out / 'task.json', {'run_id': run_id, 'worker': meta, 'command': command,
                                  'child_identity': task_identity, 'pid': process.pid, 'started_at_utc': now()})
        code = process.wait()
        terminal['exit_code'] = code
    except BaseException as error:
        terminal['error'] = {'type': type(error).__name__, 'message': str(error)}
        print(json.dumps(terminal['error'], ensure_ascii=False), file=sys.stderr, flush=True)
    finally:
        try:
            terminal['source_after'] = snapshot(task)
            terminal['source_stable'] = terminal['source_after'] == terminal['source_before']
        except BaseException as error:
            terminal['source_stable'] = False
            terminal['source_after_error'] = {'type': type(error).__name__, 'message': str(error)}
        terminal['finished_at_utc'] = now()
        terminal['seconds'] = round(time.monotonic() - began, 3)
        terminal['passed'] = terminal['exit_code'] == 0 and terminal['source_stable']
        terminal['worker_exit_code'] = code if terminal['passed'] else 1
        sys.stdout.flush()
        sys.stderr.flush()
        terminal['stdout_sha256'] = sha(out / 'stdout.log')
        terminal['stderr_sha256'] = sha(out / 'stderr.log')
        write(out / 'terminal.json', terminal)
    return code if terminal['passed'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['start', 'status', '_worker'])
    parser.add_argument('task', choices=['probe', 'cutover'])
    parser.add_argument('--phase', required=True)
    parser.add_argument('--run-id')
    args = parser.parse_args()
    if args.action == '_worker':
        if not args.run_id:
            parser.error('worker 必须携带原 run_id')
        return worker(args.task, args.phase, args.run_id)
    if args.run_id:
        parser.error('start/status 不接受外部 run_id')
    result = start(args.task, args.phase) if args.action == 'start' else status(args.task, args.phase)
    print(json.dumps(result, ensure_ascii=False, indent=2), flush=True)
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
