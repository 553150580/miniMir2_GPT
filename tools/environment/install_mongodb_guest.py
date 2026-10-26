#!/usr/bin/env python3
"""在专用Ubuntu24.04开发VM内安装已核验的Mongo官方归档，不运行系统服务。"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import shutil
import subprocess
import tarfile
import uuid
from pathlib import Path


ARCHIVE_SHA = "3e55cdcf7013fbc805d29d8195230df4b23c7688733ee15ad58ac6bc2a416611"
ROOT = Path.home() / ".local/share/minimir/mongodb"


def digest(path):
    sha = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            sha.update(block)
    return sha.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", type=Path, required=True)
    parser.add_argument("--controller", type=Path, required=True)
    parser.add_argument("--controller-sha256", required=True)
    parser.add_argument("--project-key", required=True)
    args = parser.parse_args()
    if platform.system() != "Linux" or platform.machine() != "x86_64":
        raise ValueError("只允许专用Linux x86_64开发VM")
    release = Path("/etc/os-release").read_text()
    if 'ID=ubuntu' not in release or 'VERSION_ID="24.04"' not in release:
        raise ValueError("归档只针对Ubuntu24.04，不在其它系统猜测安装")
    if digest(args.archive) != ARCHIVE_SHA or digest(args.controller) != args.controller_sha256:
        raise ValueError("转入guest的官方归档或控制器摘要不符")
    if ROOT.is_symlink():
        raise ValueError("安装目录不接受符号链接")
    ROOT.mkdir(parents=True, exist_ok=True, mode=0o700)
    destination = ROOT / "8.0.34"
    destination.mkdir(exist_ok=True)
    with tarfile.open(args.archive, "r:gz") as archive:
        for member in archive.getmembers():
            parts = Path(member.name).parts
            if not parts or parts[0] != "mongodb-linux-x86_64-ubuntu2404-8.0.34" or ".." in parts or member.issym() or member.islnk():
                raise ValueError("官方归档出现未支持路径/链接")
            target = destination.joinpath(*parts[1:])
            if target.is_symlink():
                raise ValueError("目标存在符号链接")
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
            elif member.isfile():
                target.parent.mkdir(parents=True, exist_ok=True)
                with archive.extractfile(member) as source:
                    content = source.read()
                if target.exists() and target.read_bytes() != content:
                    raise ValueError("已有安装内容与同版本归档冲突，不覆盖")
                if not target.exists():
                    target.write_bytes(content)
                    target.chmod(member.mode & 0o777)
            else:
                raise ValueError("归档成员类型不支持")
    binary = destination / "bin/mongod"
    dependencies = subprocess.run(["ldd", str(binary)], capture_output=True, text=True, check=True)
    if "not found" in dependencies.stdout:
        raise ValueError("系统依赖缺失：" + dependencies.stdout)
    version = subprocess.run([str(binary), "--version"], capture_output=True, text=True, check=True)
    if "v8.0.34" not in version.stdout:
        raise ValueError("实际二进制版本不符")
    installation_path = ROOT / "installation.json"
    previous = json.loads(installation_path.read_text()) if installation_path.is_file() else None
    if previous and (previous.get("project_key") != args.project_key or previous.get("version") != "8.0.34"):
        raise ValueError("该guest安装目录属于其它项目或版本")
    controller = ROOT / "mongodb_guest.py"
    shutil.copyfile(args.controller, controller)
    controller.chmod(0o700)
    evidence = {"purpose": "minimir_owned_development_vm", "installation_id": previous["installation_id"] if previous else str(uuid.uuid4()),
                "project_key": args.project_key, "version": "8.0.34", "binary_path": str(binary), "binary_sha256": digest(binary),
                "archive_sha256": ARCHIVE_SHA, "controller_path": str(controller), "controller_sha256": digest(controller),
                "replica_set": "minimirDev", "port": 27017, "bind_ip": "127.0.0.1", "guest_os": "Ubuntu24.04", "guest_arch": "x86_64",
                "database_state": "未读取其它数据库；新专用dbpath由控制器首次创建", "local_development_only": True, "production_ready": False,
                "auto_start": False, "version_output": version.stdout, "ldd_output": dependencies.stdout}
    installation_path.write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + "\n")
    installation_path.chmod(0o600)
    print(json.dumps(evidence, ensure_ascii=False))


if __name__ == "__main__":
    main()
