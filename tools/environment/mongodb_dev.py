#!/usr/bin/env python3
"""只管理本项目loopback开发MongoDB；不用系统旧数据、不注册开机服务。"""

from __future__ import annotations

import argparse
import contextlib
import ctypes
import fcntl
import hashlib
import json
import os
import re
import shlex
import signal
import socket
import subprocess
import sys
import time
import uuid
from pathlib import Path


PROJECT = Path(__file__).resolve().parents[2]
LOCK_FILE = PROJECT / "conf/toolchain/mongodb.lock.json"
PROJECT_KEY = hashlib.sha256(str(PROJECT).encode()).hexdigest()[:12]
DEV_ROOT = Path.home() / ".local/share/minimir-toolchain/mongodb/dev" / PROJECT_KEY
OWNER_FILE = DEV_ROOT / "owner.json"
STATE_FILE = DEV_ROOT / "state.json"
REPLICA_SET = f"minimir_dev_{PROJECT_KEY}"
DATABASE = f"minimir_dev_{PROJECT_KEY}"


def atomic_json(path: Path, data: dict) -> None:
    temporary = path.with_name(path.name + ".tmp")
    with temporary.open("w", encoding="utf-8") as stream:
        json.dump(data, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    temporary.chmod(0o600)
    temporary.replace(path)


def sha256_file(path: Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(block)
    return value.hexdigest()


def driver_environment() -> dict:
    # macOS系统Python可能给子进程注入launcher路径；驱动必须使用自己的venv。
    env = os.environ.copy()
    for name in ("PYTHONHOME", "PYTHONPATH", "PYTHONUSERBASE", "__PYVENV_LAUNCHER__", "VIRTUAL_ENV"):
        env.pop(name, None)
    return env


def load_lock() -> dict:
    lock = json.loads(LOCK_FILE.read_text(encoding="utf-8"))
    if lock.get("backend") == "lima_linux_vm":
        lima = Path(lock["lima"]["binary_path"])
        if not lima.is_file() or sha256_file(lima) != lock["lima"]["binary_sha256"]:
            raise ValueError("Lima二进制缺失或与锁不符")
        if lock.get("replica_set") != "minimirDev" or lock.get("host_port") != 27118 or lock.get("guest_port") != 27017:
            raise ValueError("开发端口/副本集配置不符合本任务契约")
        if lock.get("uri") != "mongodb://127.0.0.1:27118/?replicaSet=minimirDev&directConnection=true":
            raise ValueError("拒绝连接本任务loopback之外的Mongo URI")
        if Path(lock["lima"]["lima_home"]).is_symlink() or not Path(lock["driver_python"]).is_file():
            raise ValueError("VM目录或隔离驱动不可用")
        for path_key, hash_key in (("instance_config_path", "instance_config_sha256"), ("owner_marker_path", "owner_marker_sha256")):
            path = Path(lock["lima"][path_key])
            if not path.is_file() or path.is_symlink() or sha256_file(path) != lock["lima"][hash_key]:
                raise ValueError("专用VM配置/所有权标记已变化，不连接或停止Mongo")
        return lock
    binary = Path(lock["binary_path"])
    installation = Path.home() / ".local/share/minimir-toolchain/mongodb"
    if installation not in binary.resolve().parents or binary.is_symlink():
        raise ValueError("锁中二进制不在本任务用户安装目录，拒绝使用")
    if not binary.is_file() or sha256_file(binary) != lock["binary_sha256"]:
        raise ValueError("MongoDB二进制缺失或摘要与锁不符")
    return lock


def read_json(path: Path) -> dict | None:
    if path.is_symlink():
        raise ValueError("控制文件不能是符号链接")
    return json.loads(path.read_text(encoding="utf-8")) if path.is_file() else None


def assert_private_tree() -> None:
    path = DEV_ROOT
    boundary = Path.home()
    while path != boundary:
        if path.is_symlink():
            raise ValueError("开发目录不接受符号链接")
        path = path.parent


def choose_port() -> int:
    for port in range(37170, 37220):
        with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
            try:
                probe.bind(("127.0.0.1", port))
                return port
            except OSError:
                pass
    raise ValueError("开发端口37170～37219均不可用；不会连接或停止现有服务")


def require_port_free(port: int) -> None:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as probe:
        try:
            probe.bind(("127.0.0.1", port))
        except OSError as error:
            raise ValueError("本任务端口已被占用；不会连接、重配或停止占用者") from error


def owner_context(create: bool) -> dict | None:
    assert_private_tree()
    owner = read_json(OWNER_FILE)
    if owner:
        if owner.get("project") != str(PROJECT) or owner.get("project_key") != PROJECT_KEY or owner.get("replica_set") != REPLICA_SET:
            raise ValueError("开发目录所有者与本项目不符")
        if not isinstance(owner.get("port"), int) or not 37170 <= owner["port"] < 37220:
            raise ValueError("开发目录端口不在本工具范围")
        return owner
    if not create:
        return None
    # 只有空目录才可首次认领；绝不把已有数据库目录当成本项目数据。
    if DEV_ROOT.exists() and any(p.name != "control.lock" for p in DEV_ROOT.iterdir()):
        raise ValueError("目录有既有内容但缺少本项目owner标记；拒绝读取或认领")
    DEV_ROOT.mkdir(parents=True, exist_ok=True, mode=0o700)
    owner = {"project": str(PROJECT), "project_key": PROJECT_KEY, "owner_id": str(uuid.uuid4()),
             "port": choose_port(), "replica_set": REPLICA_SET, "database": DATABASE,
             "data_path": str(DEV_ROOT / "data"), "local_development_only": True}
    atomic_json(OWNER_FILE, owner)
    return owner


def process_info(pid: int) -> dict | None:
    if not isinstance(pid, int) or pid <= 1:
        return None
    result = subprocess.run(["/bin/ps", "-p", str(pid), "-o", "uid=", "-o", "lstart=", "-o", "command="],
                            capture_output=True, text=True, shell=False)
    line = result.stdout.strip()
    if result.returncode or not line:
        return None
    match = re.match(r"(\d+)\s+(.{24})\s+(.*)", line)
    if not match:
        return None
    uid, started, command = match.groups()
    # macOS libproc返回实际可执行文件路径，避免仅凭进程名或pid文件授权停止。
    library = ctypes.CDLL("/usr/lib/libproc.dylib")
    buffer = ctypes.create_string_buffer(4096)
    length = library.proc_pidpath(pid, buffer, len(buffer))
    executable = buffer.value.decode() if length > 0 else None
    return {"uid": int(uid), "started": started, "command": command, "executable": executable}


def validate_process(state: dict | None, lock: dict) -> tuple[bool, str]:
    if not state or not state.get("pid"):
        return False, "没有本任务运行PID"
    info = process_info(state["pid"])
    if not info:
        return False, "记录的进程已退出或身份不可读"
    if info["uid"] != os.getuid() or info["executable"] != lock["binary_path"]:
        return False, "PID的用户或二进制不匹配，不管理该进程"
    if state.get("process_started") and info["started"] != state["process_started"]:
        return False, "PID已被复用，不管理该进程"
    try:
        argv = shlex.split(info["command"])
    except ValueError:
        return False, "进程参数无法可靠解析"
    expected_pairs = {"--dbpath": state["data_path"], "--logpath": state["log_path"],
                      "--pidfilepath": state["pid_path"], "--port": str(state["port"]),
                      "--bind_ip": "127.0.0.1", "--replSet": REPLICA_SET}
    for key, value in expected_pairs.items():
        if argv.count(key) != 1 or argv.index(key) + 1 >= len(argv) or argv[argv.index(key) + 1] != value:
            return False, "进程的独立路径/端口/run-id不匹配，不管理该进程"
    if Path(state["pid_path"]).parent.name != state["run_id"] or Path(state["log_path"]).parent.name != state["run_id"]:
        return False, "run-id路径约束不符"
    return True, "已匹配二进制、用户、启动时间、独立路径与run-id"


def verify_listener(pid: int, port: int) -> None:
    result = subprocess.run(["/usr/sbin/lsof", "-nP", "-a", "-p", str(pid), "-iTCP:" + str(port), "-sTCP:LISTEN", "-Fpn"],
                            capture_output=True, text=True, shell=False)
    if result.returncode or f"p{pid}\n" not in result.stdout or f"n127.0.0.1:{port}\n" not in result.stdout:
        raise ValueError("无法确认目标端口由本任务进程仅在loopback监听；不连接")


DRIVER_PROGRAM = r'''
import json,sys,time,uuid
from urllib.parse import urlsplit
from pymongo import MongoClient,WriteConcern
from pymongo.errors import OperationFailure,CollectionInvalid
from pymongo.read_concern import ReadConcern
uri,repl,port,database=sys.argv[1:]
bootstrap_uri='mongodb://127.0.0.1:'+str(urlsplit(uri).port)+'/?directConnection=true'
client=MongoClient(bootstrap_uri,directConnection=True,serverSelectionTimeoutMS=3000,connectTimeoutMS=2000)
try:
    try:
        status=client.admin.command('replSetGetStatus')
        if status['set']!=repl:raise RuntimeError('replica set mismatch')
    except OperationFailure as error:
        if error.code!=94:raise
        client.admin.command('replSetInitiate',{'_id':repl,'members':[{'_id':0,'host':'localhost:'+port}]})
    deadline=time.monotonic()+35
    while time.monotonic()<deadline:
        hello=client.admin.command('hello')
        if hello.get('isWritablePrimary') and hello.get('setName')==repl:break
        time.sleep(0.2)
    else:raise RuntimeError('replica set did not elect primary')
    client.close()
    client=MongoClient(uri,directConnection=True,serverSelectionTimeoutMS=5000,connectTimeoutMS=2000)
    config=client.admin.command('replSetGetConfig')['config']
    if len(config['members'])!=1 or config['members'][0]['host']!='localhost:'+port:raise RuntimeError('unexpected replica members')
    # 本项目新空库，仅建空的环境探测集合，不写账号或玩法数据。
    db=client[database]
    try:db.create_collection('_environment')
    except CollectionInvalid:pass
    if db['_environment'].count_documents({})!=0:raise RuntimeError('environment collection contains unexpected data')
    probe_name='minimir_toolchain_probe_'+uuid.uuid4().hex
    probe=client[probe_name]
    probe.create_collection('txn')
    try:
        with client.start_session() as session:
            with session.start_transaction(read_concern=ReadConcern('snapshot'),write_concern=WriteConcern('majority')):
                probe.txn.insert_one({'_id':'commit','value':1},session=session)
                probe.txn.update_one({'_id':'commit'},{'$inc':{'value':1}},session=session)
        if probe.txn.find_one({'_id':'commit'})['value']!=2:raise RuntimeError('commit check failed')
        with client.start_session() as session:
            session.start_transaction()
            probe.txn.insert_one({'_id':'abort'},session=session)
            session.abort_transaction()
        if probe.txn.find_one({'_id':'abort'}) is not None:raise RuntimeError('abort check failed')
    finally:client.drop_database(probe_name)
    print(json.dumps({'replica_initialized':True,'single_member_primary':True,'transaction_commit_verified':True,'transaction_abort_verified':True,'temporary_probe_database_dropped':True,'empty_database':database,'empty_environment_collection_count':0,'server_version':client.server_info()['version']}))
finally:client.close()
'''


def status(lock: dict) -> dict:
    owner = owner_context(False)
    state = read_json(STATE_FILE)
    valid, reason = validate_process(state, lock)
    return {"running": valid, "identity_reason": reason, "run_id": state.get("run_id") if state else None,
            "pid": state.get("pid") if valid else None, "replica_set": REPLICA_SET,
            "uri": state.get("uri") if valid else None, "planned_port": owner.get("port") if owner else None,
            "last_attempt_status": state.get("status") if state else "never_started",
            "last_attempt_error": state.get("error") if state else None,
            "transaction_smoke": state.get("transaction_smoke") if valid else None,
            "host_supported": lock["host_supported"], "local_development_only": True, "production_ready": False}


def start(lock: dict) -> dict:
    current = status(lock)
    if current["running"]:
        current["action"] = "already_running"
        return current
    owner = owner_context(True)
    old_state = read_json(STATE_FILE)
    if old_state and old_state.get("pid") and process_info(old_state["pid"]):
        raise ValueError("旧PID仍存活但身份未匹配；不会连接或停止它")
    require_port_free(owner["port"])
    data_path = Path(owner["data_path"])
    if data_path.parent != DEV_ROOT:
        raise ValueError("dbpath不属于任务目录")
    data_path.mkdir(mode=0o700, exist_ok=True)
    run_id = str(uuid.uuid4())
    run_path = DEV_ROOT / "runs" / run_id
    run_path.mkdir(parents=True, mode=0o700)
    uri = f"mongodb://127.0.0.1:{owner['port']}/?replicaSet={REPLICA_SET}&directConnection=true"
    state = {"project": str(PROJECT), "owner_id": owner["owner_id"], "run_id": run_id,
             "status": "starting", "pid": None, "port": owner["port"], "data_path": str(data_path),
             "log_path": str(run_path / "mongod.log"), "pid_path": str(run_path / "mongod.pid"),
             "uri": uri, "replica_set": REPLICA_SET, "transaction_smoke": None}
    args = [lock["binary_path"], "--bind_ip", "127.0.0.1", "--port", str(state["port"]),
            "--dbpath", state["data_path"], "--logpath", state["log_path"], "--pidfilepath", state["pid_path"],
            "--replSet", REPLICA_SET, "--fork", "--oplogSize", "128", "--wiredTigerCacheSizeGB", "0.25"]
    state["command_argv"] = args
    atomic_json(STATE_FILE, state)
    started = subprocess.run(args, capture_output=True, text=True, shell=False, timeout=30)
    state.update({"launch_exit_code": started.returncode, "launch_stdout": started.stdout, "launch_stderr": started.stderr})
    if started.returncode:
        state.update({"status": "launch_failed", "error": started.stderr.strip() or started.stdout.strip()})
        atomic_json(STATE_FILE, state)
        return {**status(lock), "action": "start_failed", "launch_exit_code": started.returncode}
    try:
        state["pid"] = int(Path(state["pid_path"]).read_text().strip())
        info = process_info(state["pid"])
        state["process_started"] = info["started"] if info else None
        valid, reason = validate_process(state, lock)
        if not valid:
            raise ValueError(reason)
        verify_listener(state["pid"], state["port"])
        state["status"] = "initializing"
        atomic_json(STATE_FILE, state)
        driver = subprocess.run([lock["driver_python"], "-I", "-c", DRIVER_PROGRAM, uri, REPLICA_SET, str(state["port"]), DATABASE],
                                env=driver_environment(), capture_output=True, text=True, shell=False, timeout=50)
        if driver.returncode:
            raise ValueError("初始化/事务探测失败：" + driver.stderr[-4000:])
        state["transaction_smoke"] = json.loads(driver.stdout.strip().splitlines()[-1])
        state["status"] = "ready"
        atomic_json(STATE_FILE, state)
        return {**status(lock), "action": "started"}
    except Exception as error:
        state.update({"status": "initialization_failed", "error": str(error)})
        atomic_json(STATE_FILE, state)
        valid, _ = validate_process(state, lock)
        if valid:
            os.kill(state["pid"], signal.SIGTERM)
        raise


def stop(lock: dict) -> dict:
    state = read_json(STATE_FILE)
    valid, reason = validate_process(state, lock)
    if not valid:
        return {"action": "nothing_stopped", **status(lock), "reason": reason}
    # 再核对一次身份后只发送SIGTERM，不使用pkill、killall或按端口关库。
    valid, reason = validate_process(state, lock)
    if not valid:
        raise ValueError(reason)
    os.kill(state["pid"], signal.SIGTERM)
    deadline = time.monotonic() + 25
    while time.monotonic() < deadline:
        info = process_info(state["pid"])
        if not info or info["started"] != state["process_started"]:
            state["status"] = "stopped"
            atomic_json(STATE_FILE, state)
            return {"action": "stopped", **status(lock)}
        time.sleep(0.1)
    raise ValueError("本任务进程尚未优雅退出；不自动SIGKILL")


@contextlib.contextmanager
def control_lock(action: str):
    assert_private_tree()
    if action != "start" and not DEV_ROOT.exists():
        yield
        return
    DEV_ROOT.mkdir(parents=True, exist_ok=True, mode=0o700)
    path = DEV_ROOT / "control.lock"
    if path.is_symlink():
        raise ValueError("控制锁不能是符号链接")
    with path.open("a+") as stream:
        fcntl.flock(stream, fcntl.LOCK_EX)
        yield


def vm_command(lock: dict, argv: list[str], timeout: int = 30) -> subprocess.CompletedProcess:
    env = os.environ.copy()
    env["LIMA_HOME"] = lock["lima"]["lima_home"]
    result = subprocess.run([lock["lima"]["binary_path"], *argv], env=env, capture_output=True,
                            text=True, shell=False, timeout=timeout)
    if result.returncode:
        raise ValueError("本任务Lima命令失败：" + result.stderr[-3000:] + result.stdout[-3000:])
    return result


def json_result(output: str) -> dict:
    for line in reversed(output.splitlines()):
        try:
            value = json.loads(line)
            if isinstance(value, dict):
                return value
        except json.JSONDecodeError:
            continue
    raise ValueError("未收到有效控制JSON")


def vm_running(lock: dict) -> bool:
    result = vm_command(lock, ["list", "--json", lock["lima"]["instance"]])
    info = json_result(result.stdout)
    if info.get("name") != lock["lima"]["instance"]:
        raise ValueError("VM名称不匹配")
    return str(info.get("status", "")).casefold() == "running"


def guest_command(lock: dict, action: str) -> dict:
    result = vm_command(lock, ["--tty=false", "shell", "--workdir=/tmp", lock["lima"]["instance"], "--", "python3", lock["guest_controller_path"], action], timeout=45)
    value = json_result(result.stdout)
    if value.get("installation_id") != lock["guest_installation_id"] or value.get("project_key") != PROJECT_KEY:
        raise ValueError("guest控制器身份不匹配，不连接或停止Mongo")
    if value.get("port") != 27017 or value.get("replica_set") != "minimirDev" or value.get("bind_ip") != "127.0.0.1":
        raise ValueError("guest监听或副本集契约不匹配")
    return value


def verify_vm_forwarder(lock: dict) -> bool:
    result = subprocess.run(["/usr/sbin/lsof", "-nP", "-iTCP:27118", "-sTCP:LISTEN", "-Fpn"],
                            capture_output=True, text=True, shell=False)
    if result.returncode:
        return False
    owners = []
    current = None
    for line in result.stdout.splitlines():
        if line.startswith("p"):
            current = {"pid": int(line[1:]), "names": []}
            owners.append(current)
        elif line.startswith("n") and current is not None:
            current["names"].append(line[1:])
    instance_path = str(Path(lock["lima"]["lima_home"]) / lock["lima"]["instance"])
    for owner in owners:
        if owner["names"] != ["127.0.0.1:27118"]:
            return False
        info = process_info(owner["pid"])
        if not info or info["uid"] != os.getuid() or instance_path not in info["command"]:
            return False
        if info["executable"] not in {lock["lima"]["binary_path"], "/usr/bin/ssh"}:
            return False
    return bool(owners)


def vm_status(lock: dict) -> dict:
    base = {"backend": "lima_linux_vm", "version": lock["version"], "host_supported": False,
            "guest_platform_supported": lock["guest_platform_supported"], "replica_set": lock["replica_set"],
            "local_development_only": True, "production_ready": False}
    if not vm_running(lock):
        return {**base, "running": False, "vm_running": False, "uri": None}
    guest = guest_command(lock, "status")
    valid = guest["running"] and guest["listener_verified"]
    forwarded = verify_vm_forwarder(lock) if valid else False
    history = read_json(DEV_ROOT / "vm-client-state.json")
    return {**base, "running": valid, "vm_running": True, "forwarding_verified": forwarded, "guest": guest, "uri": lock["uri"] if valid and forwarded else None,
            "transaction_smoke": history.get("transaction_smoke") if valid and history and history.get("run_id") == guest["run_id"] else None}


def vm_start(lock: dict) -> dict:
    if not vm_running(lock):
        vm_tool = PROJECT / lock["lima"]["controller_path"]
        if not vm_tool.is_file():
            raise ValueError("缺少本任务VM所有权控制工具")
        boot = subprocess.run([sys.executable, str(vm_tool), "start"], capture_output=True, text=True, shell=False, timeout=180)
        if boot.returncode or not vm_running(lock):
            raise ValueError("受控VM启动失败：" + boot.stderr[-2000:] + boot.stdout[-2000:])
    guest = guest_command(lock, "start")
    if not guest["running"] or not guest["listener_verified"]:
        raise ValueError("本任务Mongo尚未安全监听，不连接")
    forwarding_deadline = time.monotonic() + 12
    while not verify_vm_forwarder(lock) and time.monotonic() < forwarding_deadline:
        time.sleep(0.2)
    if not verify_vm_forwarder(lock):
        guest_command(lock, "stop")
        raise ValueError("宿主27118未确认由本任务Lima仅在loopback转发；已停止本任务Mongo，不连接该端口")
    driver = subprocess.run([lock["driver_python"], "-I", "-c", DRIVER_PROGRAM, lock["uri"], lock["replica_set"],
                             str(lock["guest_port"]), DATABASE], env=driver_environment(), capture_output=True, text=True, shell=False, timeout=55)
    if driver.returncode:
        guest_command(lock, "stop")
        raise ValueError("开发副本集/事务探测失败，已停止本任务Mongo：" + driver.stderr[-4000:])
    evidence = json_result(driver.stdout)
    atomic_json(DEV_ROOT / "vm-client-state.json", {"run_id": guest["run_id"], "transaction_smoke": evidence})
    return {"action": "started", **vm_status(lock)}


def vm_stop(lock: dict) -> dict:
    if not vm_running(lock):
        return {"action": "nothing_stopped", **vm_status(lock)}
    result = guest_command(lock, "stop")
    return {"action": result.get("action"), **vm_status(lock)}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("start", "stop", "status"))
    args = parser.parse_args()
    lock = load_lock()
    with control_lock(args.action):
        actions = {"start": vm_start, "stop": vm_stop, "status": vm_status} if lock.get("backend") == "lima_linux_vm" else {"start": start, "stop": stop, "status": status}
        result = actions[args.action](lock)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 2 if result.get("action") == "start_failed" else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, subprocess.SubprocessError, json.JSONDecodeError) as error:
        print(json.dumps({"status": "failed", "error": str(error), "production_ready": False}, ensure_ascii=False), file=sys.stderr)
        raise SystemExit(1)
