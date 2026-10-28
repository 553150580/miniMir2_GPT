"""保留Windows Lima诊断控制器；原生数据库启用后不作为默认入口。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.request
import uuid

import lima_windows as vm
from mongodb_smoke import DRIVER_PROGRAM

ROOT = vm.ROOT
KEY = hashlib.sha256(str(ROOT).encode()).hexdigest()[:12]
STATE = ROOT / '.work/environment/windows-mongodb.json'
URI = 'mongodb://127.0.0.1:27118/?replicaSet=minimirDev&directConnection=true'


def save(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    temporary.replace(path)


def installation():
    vm.load()
    if STATE.is_symlink() or not STATE.is_file():
        raise RuntimeError('尚未初始化本项目开发数据库，请先执行 mongodb_dev.py init')
    record = json.loads(STATE.read_text(encoding='utf-8'))
    if record['project_key'] != KEY or record['version'] != '8.0.34':
        raise RuntimeError('开发数据库安装标记不匹配')
    if record['controller_sha256'] != vm.sha(ROOT / 'tools/environment/mongodb_guest.py'):
        raise RuntimeError('guest 控制器已变化，需重新审阅初始化')
    return record


def control(action):
    record = installation()
    reply = vm.json_output(vm.guest(['python3', record['controller_path'], action]).stdout)
    if reply.get('project_key') != KEY or reply.get('installation_id') != record['installation_id']:
        raise RuntimeError('guest 数据库身份不符，不连接或停止宿主端口')
    return reply


def forwarded():
    info = vm.info()
    command = 'Get-NetTCPConnection -State Listen -LocalPort 27118 -ErrorAction SilentlyContinue | Select-Object LocalAddress,OwningProcess | ConvertTo-Json -Compress'
    result = subprocess.run(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', command], capture_output=True, encoding='utf-8', errors='replace')
    if not result.stdout.strip():
        return False
    records = json.loads(result.stdout)
    records = records if isinstance(records, list) else [records]
    return bool(records) and all(item['LocalAddress'] == '127.0.0.1' and item['OwningProcess'] == info['hostAgentPID'] for item in records)


def initialize():
    vm.create()
    vm.start()
    if STATE.is_file():
        record = installation()
        return dict(action='already_initialized', installation_id=record['installation_id'], version=record['version'], production_ready=False)
    _, prefix, _, _, name = vm.settings()
    lock = json.loads((ROOT / 'conf/toolchain/mongodb.lock.json').read_text(encoding='utf-8'))
    archive = prefix / 'downloads/mongodb-linux-x86_64-ubuntu2404-8.0.34.tgz'
    if not archive.is_file():
        temporary = archive.with_suffix('.partial')
        urllib.request.urlretrieve(lock['archive_url'], temporary)
        if vm.sha(temporary) != lock['official_sha256']:
            raise RuntimeError('Mongo 官方归档摘要不符')
        temporary.replace(archive)
    if vm.sha(archive) != lock['official_sha256']:
        raise RuntimeError('Mongo 缓存归档摘要不符')
    staging = '/tmp/minimir-bootstrap-' + uuid.uuid4().hex
    vm.guest(['mkdir', '-m', '700', staging])
    sources = [archive, ROOT / 'tools/environment/install_mongodb_guest.py', ROOT / 'tools/environment/mongodb_guest.py']
    for source in sources:
        vm.copy_to_guest(source, f'{staging}/{source.name}')
    controller = ROOT / 'tools/environment/mongodb_guest.py'
    record = vm.json_output(vm.guest(['python3', staging + '/install_mongodb_guest.py', '--archive', staging + '/' + archive.name,
                                     '--controller', staging + '/mongodb_guest.py', '--controller-sha256', vm.sha(controller), '--project-key', KEY], timeout=300).stdout)
    if record.get('project_key') != KEY or record.get('archive_sha256') != lock['official_sha256']:
        raise RuntimeError('guest 安装回执不匹配')
    save(STATE, record)
    return dict(action='initialized', installation_id=record['installation_id'], version=record['version'], production_ready=False)


def status():
    info = vm.info()
    base = dict(backend='lima_linux_vm', version='8.0.34', vm_running=info['status'].lower() == 'running', production_ready=False)
    if not base['vm_running']:
        return dict(base, running=False)
    guest = control('status')
    valid = bool(guest.get('running') and guest.get('listener_verified'))
    forward = forwarded() if valid else False
    return dict(base, running=valid and forward, guest=guest, forwarding_verified=forward, uri=URI if valid and forward else None)


def start():
    vm.start()
    guest = control('start')
    if not guest.get('running') or not guest.get('listener_verified'):
        raise RuntimeError('guest Mongo 尚未通过进程及回环监听校验')
    for _ in range(30):
        if forwarded():
            break
        time.sleep(1)
    else:
        control('stop')
        raise RuntimeError('宿主端口未确认属于本项目 Lima，未连接')
    result = subprocess.run([sys.executable, '-I', '-c', DRIVER_PROGRAM, URI, 'minimirDev', '27017', 'minimir_dev_' + KEY],
                            capture_output=True, encoding='utf-8', timeout=180)
    if result.returncode:
        control('stop')
        raise RuntimeError('副本集与事务探测未通过：' + result.stderr[-2500:])
    smoke = vm.json_output(result.stdout)
    save(ROOT / '.work/environment/windows-mongodb-smoke.json', smoke)
    return dict(status(), transaction_smoke=smoke)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['init', 'start', 'stop', 'status'])
    action = parser.parse_args().action
    try:
        with vm.control_lock():
            result = {'init': initialize, 'start': start, 'stop': lambda: control('stop'), 'status': status}[action]()
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0
    except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
        print(json.dumps(dict(error=str(error), production_ready=False), ensure_ascii=False), file=sys.stderr)
        return 1
