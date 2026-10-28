#!/usr/bin/env python3
"""仅管理当前项目登记的minimir-dev开发VM；不创建任意VM或启用自动开机。"""
from __future__ import annotations

import os
import sys
if os.name == 'nt' and __name__ == '__main__':
    from lima_windows import main
    raise SystemExit(main())

import argparse
import hashlib
import json
import os
from pathlib import Path
import socket
import subprocess
import sys

PROJECT_ROOT = Path(__file__).resolve().parents[2]
LOCK_PATH = PROJECT_ROOT / "conf/toolchain/lima.lock.json"
VM_NAME = "minimir-dev"


def load_contract():
    lock = json.loads(LOCK_PATH.read_text(encoding="utf-8"))
    vm = lock["vm"]
    binary = Path(lock["binary_path"])
    lima_home = Path(vm["lima_home"])
    expected_home = Path("/Users/srf/.local/share/minimir-toolchain/lima/instances")
    if vm["name"] != VM_NAME or lima_home.resolve() != expected_home.resolve():
        raise RuntimeError("VM名称或独立LIMA_HOME不匹配")
    if binary.resolve() != Path("/Users/srf/.local/share/minimir-toolchain/lima/2.2.1/bin/limactl").resolve():
        raise RuntimeError("非本任务登记的Lima可执行文件")
    if hashlib.sha256(binary.read_bytes()).hexdigest() != lock["binary_sha256"]:
        raise RuntimeError("Lima可执行文件摘要变化，拒绝管理")
    template = PROJECT_ROOT / vm["template_path"]
    if hashlib.sha256(template.read_bytes()).hexdigest() != vm["template_sha256"]:
        raise RuntimeError("VM模板已变更，需先审阅并更新安装锁")
    expected_instance = expected_home / VM_NAME
    if Path(vm["instance_directory"]).resolve() != expected_instance.resolve():
        raise RuntimeError("VM目录不匹配")
    marker_path = Path(vm["ownership_marker_path"])
    if marker_path.resolve() != (expected_instance / ".minimir-owned.json").resolve():
        raise RuntimeError("VM标记路径不匹配")
    marker = json.loads(marker_path.read_text(encoding="utf-8"))
    if marker != vm["ownership_marker"]:
        raise RuntimeError("VM所有权标记不匹配")
    if marker["project_root"] != str(PROJECT_ROOT):
        raise RuntimeError("VM不属于当前项目")
    config = Path(vm["instance_directory"]) / "lima.yaml"
    if hashlib.sha256(config.read_bytes()).hexdigest() != vm["instance_config_sha256"]:
        raise RuntimeError("实际VM配置已变化，拒绝管理")
    env = os.environ.copy()
    env["LIMA_HOME"] = str(lima_home)
    env["PATH"] = str(binary.parent) + os.pathsep + env.get("PATH", "")
    return lock, [str(binary), "--tty=false"], env


def run(command, env, *, capture=False):
    return subprocess.run(command, env=env, cwd=PROJECT_ROOT,
                          text=True, capture_output=capture, check=True)


def current_state(base, env):
    result = run(base + ["list", "--json", VM_NAME], env, capture=True)
    records = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
    if len(records) != 1 or records[0].get("name") != VM_NAME:
        raise RuntimeError("未找到唯一登记VM")
    return records[0]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["start", "stop", "status"])
    args = parser.parse_args()
    lock, base, env = load_contract()
    state = current_state(base, env)
    if args.action == "start" and state.get("status") != "Running":
        # 不覆盖宿主已有服务，也不自动改动本项目固定开发端口。
        with socket.socket() as probe:
            try:
                probe.bind(("127.0.0.1", 27118))
            except OSError as exc:
                raise RuntimeError("宿主127.0.0.1:27118已占用") from exc
        run(base + ["start", "--timeout=5m", VM_NAME], env)
    elif args.action == "stop" and state.get("status") == "Running":
        # 使用Lima正常停止，只作用于登记VM；不发送任意PID信号。
        run(base + ["stop", VM_NAME], env)
    state = current_state(base, env)
    summary_keys = ["name", "status", "dir", "vmType", "arch", "cpus", "memory", "disk", "sshLocalPort"]
    print(json.dumps({key: state.get(key) for key in summary_keys}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.CalledProcessError) as exc:
        print(f"开发VM操作失败：{exc}", file=sys.stderr)
        raise SystemExit(1)
