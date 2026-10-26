#!/usr/bin/env python3
"""Linux开发VM内的Mongo进程管理器；仅管理带本任务run-id的独立数据目录。"""

from __future__ import annotations

import argparse
import fcntl
import hashlib
import json
import os
import signal
import socket
import subprocess
import time
import uuid
from pathlib import Path


ROOT = Path.home() / ".local/share/minimir/mongodb"
INSTALLATION = ROOT / "installation.json"


def read(path: Path):
    if path.is_symlink():
        raise ValueError("拒绝控制文件符号链接")
    return json.loads(path.read_text()) if path.is_file() else None


def write(path: Path, value: dict):
    temp = path.with_name(path.name + ".tmp")
    temp.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    temp.chmod(0o600)
    temp.replace(path)


def load():
    conf = read(INSTALLATION)
    if not conf or conf.get("purpose") != "minimir_owned_development_vm":
        raise ValueError("缺少本任务安装标记")
    binary = Path(conf["binary_path"])
    if ROOT not in binary.resolve().parents or binary.is_symlink() or not binary.is_file():
        raise ValueError("二进制不在本任务目录")
    if hashlib.sha256(binary.read_bytes()).hexdigest() != conf["binary_sha256"]:
        raise ValueError("Mongo二进制与安装摘要不符")
    base = ROOT / "dev" / conf["project_key"]
    for item in [ROOT, base, base / "data"]:
        if item.is_symlink():
            raise ValueError("不允许数据目录符号链接")
    return conf, base


def proc(pid):
    if not isinstance(pid, int) or pid <= 1:
        return None
    path = Path("/proc") / str(pid)
    try:
        stat = (path / "stat").read_text()
        after_name = stat[stat.rfind(")") + 2:].split()
        return {"uid": path.stat().st_uid, "start_ticks": after_name[19],
                "executable": str((path / "exe").resolve(strict=True)),
                "argv": [x.decode() for x in (path / "cmdline").read_bytes().split(b"\0") if x]}
    except (OSError, ValueError, IndexError):
        return None


def identity(state, conf, base):
    if not state or state.get("installation_id") != conf["installation_id"] or not state.get("pid"):
        return False
    info = proc(state["pid"])
    if not info or info["uid"] != os.getuid() or info["executable"] != conf["binary_path"] or info["start_ticks"] != state.get("start_ticks"):
        return False
    pairs = {"--bind_ip": "127.0.0.1", "--port": str(conf["port"]), "--replSet": conf["replica_set"],
             "--dbpath": str(base / "data"), "--pidfilepath": state["pid_path"], "--logpath": state["log_path"]}
    for key, expected in pairs.items():
        argv = info["argv"]
        if argv.count(key) != 1 or argv.index(key) + 1 >= len(argv) or argv[argv.index(key) + 1] != expected:
            return False
    return Path(state["pid_path"]).parent == base / "runs" / state["run_id"] and Path(state["log_path"]).parent == base / "runs" / state["run_id"]


def owned_listener(pid, port):
    try:
        inodes = set()
        for descriptor in (Path("/proc") / str(pid) / "fd").iterdir():
            try:
                target = os.readlink(descriptor)
            except OSError:
                continue
            if target.startswith("socket:["):
                inodes.add(target[8:-1])
        sockets = [line.split() for line in Path("/proc/net/tcp").read_text().splitlines()[1:]]
        matching = [row for row in sockets if row[1].endswith(f":{port:04X}") and row[3] == "0A" and row[9] in inodes]
        return bool(matching) and all(row[1] == f"0100007F:{port:04X}" for row in matching)
    except OSError:
        return False


def status(conf, base):
    state = read(base / "state.json")
    valid = identity(state, conf, base)
    listener = owned_listener(state["pid"], conf["port"]) if valid else False
    return {"running": valid, "listener_verified": listener, "pid": state["pid"] if valid else None,
            "run_id": state.get("run_id") if state else None, "status": state.get("status") if state else "never_started",
            "installation_id": conf["installation_id"], "project_key": conf["project_key"],
            "replica_set": conf["replica_set"], "port": conf["port"], "bind_ip": "127.0.0.1",
            "data_path": str(base / "data"), "error": state.get("error") if state else None,
            "local_development_only": True, "production_ready": False}


def start(conf, base):
    current = status(conf, base)
    if current["running"]:
        if not current["listener_verified"]:
            raise ValueError("本任务进程存在但loopback监听未通过，拒绝重开")
        return {"action": "already_running", **current}
    previous = read(base / "state.json")
    if previous and previous.get("status") != "stopped" and previous.get("pid") and proc(previous["pid"]):
        raise ValueError("旧PID已复用或身份不符，不管理该进程")
    with socket.socket() as port_check:
        try:
            port_check.bind(("127.0.0.1", conf["port"]))
        except OSError as error:
            raise ValueError("目标端口已有其它进程；不连接或停止占用者") from error
    owner_path = base / "owner.json"
    owner = read(owner_path)
    if owner and owner != {"installation_id": conf["installation_id"], "project_key": conf["project_key"]}:
        raise ValueError("已有数据目录属于其它安装")
    data = base / "data"
    if not owner and data.exists() and any(data.iterdir()):
        raise ValueError("已有非空数据目录没有所有者标记，拒绝读取")
    data.mkdir(mode=0o700, exist_ok=True)
    write(owner_path, {"installation_id": conf["installation_id"], "project_key": conf["project_key"]})
    run_id = str(uuid.uuid4())
    run = base / "runs" / run_id
    run.mkdir(parents=True, mode=0o700)
    state = {"installation_id": conf["installation_id"], "project_key": conf["project_key"], "run_id": run_id,
             "pid": None, "status": "starting", "pid_path": str(run / "mongod.pid"), "log_path": str(run / "mongod.log")}
    argv = [conf["binary_path"], "--bind_ip", "127.0.0.1", "--port", str(conf["port"]), "--replSet", conf["replica_set"],
            "--dbpath", str(data), "--pidfilepath", state["pid_path"], "--logpath", state["log_path"],
            "--fork", "--oplogSize", "128", "--wiredTigerCacheSizeGB", "0.25"]
    state["command_argv"] = argv
    write(base / "state.json", state)
    result = subprocess.run(argv, capture_output=True, text=True, shell=False, timeout=30)
    if result.returncode:
        state.update({"status": "launch_failed", "error": result.stderr or result.stdout, "exit_code": result.returncode})
        write(base / "state.json", state)
        return {"action": "start_failed", **status(conf, base)}
    state["pid"] = int(Path(state["pid_path"]).read_text().strip())
    info = proc(state["pid"])
    state["start_ticks"] = info["start_ticks"] if info else None
    if not identity(state, conf, base) or not owned_listener(state["pid"], conf["port"]):
        state.update({"status": "identity_failed", "error": "新进程身份或loopback监听不符"})
        write(base / "state.json", state)
        raise ValueError(state["error"])
    state["status"] = "running"
    write(base / "state.json", state)
    return {"action": "started", **status(conf, base)}


def stop(conf, base):
    state = read(base / "state.json")
    if not identity(state, conf, base):
        return {"action": "nothing_stopped", **status(conf, base)}
    # Linux pidfd绑定进程实例；重新核对身份后发SIGTERM，避免PID复用误伤。
    descriptor = os.pidfd_open(state["pid"])
    try:
        if not identity(state, conf, base):
            raise ValueError("停止前进程身份变化，不发信号")
        signal.pidfd_send_signal(descriptor, signal.SIGTERM)
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            if not identity(state, conf, base):
                state["status"] = "stopped"
                write(base / "state.json", state)
                return {"action": "stopped", **status(conf, base)}
            time.sleep(0.1)
        raise ValueError("进程未在25秒内优雅退出；不自动SIGKILL")
    finally:
        os.close(descriptor)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("start", "stop", "status"))
    args = parser.parse_args()
    conf, base = load()
    base.mkdir(parents=True, exist_ok=True, mode=0o700)
    lock = base / "control.lock"
    if lock.is_symlink():
        raise ValueError("控制锁不能为符号链接")
    with lock.open("a+") as stream:
        fcntl.flock(stream, fcntl.LOCK_EX)
        result = {"start": start, "stop": stop, "status": status}[args.action](conf, base)
    print(json.dumps(result, ensure_ascii=False))
    return 2 if result.get("action") == "start_failed" else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        print(json.dumps({"status": "failed", "error": str(error)}, ensure_ascii=False))
        raise SystemExit(1)
