#!/usr/bin/env python3
"""检查锁定工具、生成证据和专用开发数据库；不安装软件、启停服务或修改数据库。"""
import argparse
from datetime import datetime, timedelta, timezone
import glob
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
LOCKS = ROOT / "conf/toolchain"
EXPECTED_URI = "mongodb://127.0.0.1:27118/?replicaSet=minimirDev&directConnection=true"


def read_json(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def local_path(path):
    value = Path(path)
    return value if value.is_absolute() else ROOT / value


def sha256(path):
    value = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            value.update(block)
    return value.hexdigest()


def matches_hash(path, expected):
    try:
        return bool(path and expected and local_path(path).is_file()
                    and sha256(local_path(path)) == expected)
    except OSError:
        return False


def invoke(executable, arguments, timeout=15, env=None):
    if not executable or not Path(executable).is_file():
        return {"path": executable, "return_code": None, "stdout": "", "error": "可执行文件不存在"}
    try:
        child_env = (os.environ if env is None else env).copy()
        for key in ("__PYVENV_LAUNCHER__", "PYTHONHOME", "PYTHONPATH", "PYTHONUSERBASE"):
            child_env.pop(key, None)
        result = subprocess.run([str(executable)] + list(arguments), cwd=ROOT,
                                env=child_env, capture_output=True, text=True, timeout=timeout)
        return {"path": str(executable), "return_code": result.returncode,
                "stdout": result.stdout.strip(), "stderr": result.stderr.strip()[:3000]}
    except (OSError, subprocess.TimeoutExpired) as error:
        return {"path": str(executable), "return_code": None, "stdout": "",
                "error": type(error).__name__ + ": " + str(error)[:500]}


def version_matches(result, expected):
    return bool(expected and result.get("return_code") == 0
                and re.search(r"(?<![\w.+-])" + re.escape(expected) + r"(?![\w.+-])", result.get("stdout", "")))


def generation_files_match(records):
    return bool(records) and all(matches_hash(path, digest) for path, digest in records)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--strict", action="store_true", help="工具链或开发数据库未就绪时返回2")
    parser.add_argument("--creator-path", help="指定Creator.app；仍须为3.8.8")
    parser.add_argument("--skip-database", action="store_true", help="只检查工具；开发数据库状态记为未检查")
    parser.add_argument("--report", default="docs/开发环境检查.json", help="检查报告路径")
    args = parser.parse_args()
    checks, tools, locks = [], {}, {}

    def check(code, passed, message, scope="toolchain", evidence=None):
        item = {"code": code, "status": "passed" if passed else "failed",
                "scope": scope, "message": message}
        if evidence is not None:
            item["evidence"] = evidence
        checks.append(item)

    for name in ["go", "protoc", "go-modules", "python", "node-tools", "lima", "mongodb"]:
        path = LOCKS / (name + ".lock.json")
        try:
            data = read_json(path)
            if not isinstance(data, dict):
                raise ValueError("锁的根必须是对象")
            locks[name] = data
            check("lock_" + name, True, "读取工具链锁：" + path.name)
        except (OSError, ValueError) as error:
            locks[name] = {}
            check("lock_" + name, False, "缺失或无效的锁：" + path.name, evidence=str(error))

    # 选用目标只来自项目锁；已有的系统旧版本仅作信息记录。
    go = locks["go"]
    tools["go"] = invoke(go.get("executable"), ["version"])
    check("go_version", version_matches(tools["go"], go.get("version")), "Go运行版本与安装锁匹配")
    check("go_archive_provenance", go.get("official_checksum_verified") is True
          and bool(go.get("archive_sha256")), "Go官方归档摘要已在安装时核验")

    protoc = locks["protoc"]
    tools["protoc"] = invoke(protoc.get("binary_path"), ["--version"])
    check("protoc_version", version_matches(tools["protoc"], protoc.get("version_output")), "protoc运行版本匹配")
    check("protoc_binary_hash", matches_hash(protoc.get("binary_path"), protoc.get("binary_sha256")), "protoc二进制摘要匹配")

    modules = locks["go-modules"]
    plugin = modules.get("protoc_gen_go", {})
    tools["protoc-gen-go"] = invoke(plugin.get("path"), ["--version"])
    check("go_pb_plugin", version_matches(tools["protoc-gen-go"], plugin.get("version"))
          and matches_hash(plugin.get("path"), plugin.get("sha256")), "Go PB插件版本与摘要匹配")
    check("go_module_lock", modules.get("verification") == "all modules verified"
          and matches_hash("server/go.mod", modules.get("go_mod_sha256"))
          and matches_hash("server/go.sum", modules.get("go_sum_sha256")),
          "server/go.mod与go.sum匹配已完成的Go模块校验", evidence="conf/toolchain/go-modules.lock.json")

    python = locks["python"]
    python_spec, uv, venv = python.get("python", {}), python.get("uv", {}), python.get("tools_venv", {})
    python_executable = venv.get("python")
    python_program = ("import sys,json,platform,PIL; "
                      "print(json.dumps({'executable':sys.executable,'venv_prefix':sys.prefix,'python_version':platform.python_version(),"
                      "'pillow_version':PIL.__version__}))")
    tools["python3"] = invoke(python_executable, ["-I", "-c", python_program])
    try:
        python_runtime = json.loads(tools["python3"]["stdout"])
    except (ValueError, KeyError):
        python_runtime = {}
    pillow = next((p for p in venv.get("packages", []) if p.get("name", "").lower() == "pillow"), {})
    check("python_runtime", tools["python3"].get("return_code") == 0
          and python_runtime.get("python_version") == python_spec.get("version")
          and Path(python_runtime.get("venv_prefix", "")).resolve() == Path(venv.get("path", "__missing__")).resolve(),
          "隔离Python运行版本匹配", evidence=python_runtime)
    check("python_binary_hash", matches_hash(python_spec.get("executable"), python_spec.get("executable_sha256")), "Python基础解释器摘要匹配")
    check("pillow_runtime", bool(pillow) and python_runtime.get("pillow_version") == pillow.get("version")
          and matches_hash(pillow.get("installed_imaging_extension"), pillow.get("installed_imaging_extension_sha256")),
          "Pillow版本与本机图像扩展摘要匹配")
    requirements = venv.get("requirements", {})
    check("python_dependency_lock", venv.get("all_installed_packages_hash_locked") is True
          and matches_hash(requirements.get("path"), requirements.get("sha256")), "Python工具依赖哈希锁匹配")
    tools["uv"] = invoke(uv.get("executable"), ["--version"])
    check("uv_runtime", version_matches(tools["uv"], "uv " + str(uv.get("version", "")))
          and matches_hash(uv.get("executable"), uv.get("executable_sha256")), "uv版本和二进制摘要匹配")

    node = locks["node-tools"]
    node_spec = node.get("node", {})
    tools["node"] = invoke(node_spec.get("executable"), ["--version"])
    check("node_version", version_matches(tools["node"], node_spec.get("version")), "Node运行版本与锁匹配")
    packages = []
    for package in node.get("packages", []):
        try:
            actual = read_json(Path(package["installed_path"]) / "package.json").get("version")
        except (OSError, ValueError, KeyError):
            actual = None
        packages.append({"name": package.get("name"), "expected_version": package.get("version"),
                         "actual_version": actual, "path": package.get("installed_path"),
                         "passed": actual == package.get("version") and actual is not None
                         and package.get("registry_lock_integrity_match") is True})
    tools["node_packages"] = packages
    check("node_packages", {"protobufjs", "protobufjs-cli", "long"}.issubset({p["name"] for p in packages})
          and all(p["passed"] for p in packages), "protobufjs/CLI/Long实际安装版本与registry完整性锁匹配")
    for name in ["package_manifest", "package_lock"]:
        spec = node.get(name, {})
        check("node_" + name, matches_hash(spec.get("path"), spec.get("sha256")), "Node " + name + "摘要匹配")
    check("pbjs_pbts_executables", all(Path(node.get("binaries", {}).get(name, "__missing__")).is_file()
          for name in ["pbjs", "pbts"]), "pbjs与pbts生成入口存在")

    paths = {p for pattern in ["/Applications/CocosCreator*.app",
             "/Applications/Cocos/Creator/*/CocosCreator.app",
             str(Path.home() / "Applications/CocosCreator*.app")] for p in glob.glob(pattern)}
    if args.creator_path:
        paths.add(args.creator_path)
    candidates = []
    for app in sorted(paths):
        try:
            with (Path(app) / "Contents/Info.plist").open("rb") as stream:
                version = plistlib.load(stream).get("CFBundleShortVersionString")
        except (OSError, ValueError):
            version = None
        candidates.append({"path": app, "version": version, "matches_required": version == "3.8.8"})
    tools["cocos_creator_candidates"] = candidates
    creator = next((p for p in candidates if p["matches_required"]), None)
    check("creator_version", creator is not None, "发现指定Cocos Creator 3.8.8")
    tsc = Path(creator["path"]) / "Contents/Resources/app.asar.unpacked/node_modules/typescript/bin/tsc" if creator else None
    tools["typescript"] = invoke(node_spec.get("executable"), [str(tsc), "--version"] if tsc else ["--version"])
    tools["typescript"]["compiler_path"] = str(tsc) if tsc else None
    check("creator_typescript", tsc is not None and tsc.is_file()
          and version_matches(tools["typescript"], "Version 5.8.2"),
          "Creator所选编辑器TypeScript5.8.2可运行；不替换引擎内部编译链")

    lima = locks["lima"]
    tools["lima"] = invoke(lima.get("binary_path"), ["--version"])
    check("lima_runtime", version_matches(tools["lima"], lima.get("version_output"))
          and matches_hash(lima.get("binary_path"), lima.get("binary_sha256")), "Lima版本与二进制摘要匹配")

    # 当前生成锁是验证入口；protoc首次历史命令中的旧目录不作为当前路径。
    try:
        go_pb = read_json(LOCKS / "go-pb-generation.json")
        sources = [(str(Path("shared/proto") / Path(name).name), digest) for name, digest in go_pb.get("sources", {}).items()]
        outputs = list(go_pb.get("outputs", {}).items())
        go_pb_ok = ({Path(p).name for p, _ in sources} == {p.name for p in (ROOT / "shared/proto").glob("*.proto")}
                    and generation_files_match(sources) and generation_files_match(outputs)
                    and go_pb.get("protoc") == protoc.get("version_output")
                    and go_pb.get("protoc_gen_go") == plugin.get("version"))
    except (OSError, ValueError, TypeError):
        go_pb_ok = False
    check("go_pb_generation", go_pb_ok, "Go PB生成证据匹配shared/proto与server输出", evidence="conf/toolchain/go-pb-generation.json")
    try:
        client_pb = read_json(LOCKS / "client-pb-verification.json")
        source_records = [(x["path"], x["sha256"]) for x in client_pb.get("proto_sources", [])]
        output_records = [(x["path"], x["sha256"]) for x in client_pb.get("generated_files", [])]
        client_pb_ok = (client_pb.get("status") == "passed" and client_pb.get("runtime_long_loaded") is True
                        and generation_files_match(source_records) and generation_files_match(output_records))
    except (OSError, ValueError, KeyError):
        client_pb_ok = False
    check("client_pb_generation", client_pb_ok, "JS/DTS及Long编解码证据匹配当前源与输出", evidence="conf/toolchain/client-pb-verification.json")

    mongo = locks["mongodb"]
    historical = mongo.get("verification", {})
    evidence_flags = ["replica_set_initialized", "single_member_primary", "transaction_commit", "transaction_abort",
                      "temporary_probe_database_dropped", "guest_loopback_listener_verified",
                      "host_loopback_forwarder_verified", "start_stop_restart_verified"]
    history_ok = historical.get("status") == "passed_development_capability" and all(historical.get(k) is True for k in evidence_flags)
    check("mongo_installation_evidence", mongo.get("backend") == "lima_linux_vm"
          and mongo.get("guest_platform_supported") is True
          and mongo.get("actual_sha256") == mongo.get("official_sha256")
          and bool(mongo.get("actual_sha256")) and history_ok,
          "Linux开发Mongo安装、回环、副本集和事务证据已锁定", evidence=mongo.get("verification_evidence_path"))
    check("mongo_controller_hash", matches_hash("tools/environment/mongodb_dev.py", mongo.get("controller_sha256")),
          "专用Mongo身份控制器摘要匹配安装验证")
    try:
        go_smoke = read_json(LOCKS / "go-smoke-mongodb.json")
        go_smoke_ok = (go_smoke.get("mongodb_transaction") == "snapshot_majority_commit_and_duplicate_rollback_passed"
                       and go_smoke.get("mongodb_server_version") == mongo.get("version")
                       and go_smoke.get("protobuf_uint64_roundtrip") == "passed")
    except (OSError, ValueError):
        go_smoke_ok = False
    check("go_driver_mongo_evidence", go_smoke_ok, "锁定Go驱动已完成开发Mongo事务与uint64冒烟", evidence="conf/toolchain/go-smoke-mongodb.json")

    database = {"backend": mongo.get("backend"), "selected_version": mongo.get("version"), "uri": mongo.get("uri"),
                "replica_set": mongo.get("replica_set"), "historical_transaction_evidence_passed": history_ok, "current_probe": None}
    if args.skip_database:
        check("mongo_current_runtime", False, "已按参数跳过开发数据库实时检查", scope="database")
    else:
        result = invoke(python_executable, [str(ROOT / "tools/environment/mongodb_dev.py"), "status"], timeout=55)
        try:
            state = json.loads(result["stdout"])
        except (ValueError, KeyError):
            state = {}
        database["managed_status"] = state
        smoke = state.get("transaction_smoke") or {}
        current_owned = (result.get("return_code") == 0 and state.get("running") is True
                         and state.get("forwarding_verified") is True and state.get("uri") == EXPECTED_URI
                         and mongo.get("uri") == EXPECTED_URI)
        current_txn = all(smoke.get(k) is True for k in ["replica_initialized", "single_member_primary",
                          "transaction_commit_verified", "transaction_abort_verified", "temporary_probe_database_dropped"])
        check("mongo_owned_runtime", current_owned, "当前Mongo与回环转发由本项目身份控制器验证", scope="database",
              evidence={"controller_return_code": result.get("return_code"), "error": result.get("error") or result.get("stderr")})
        check("mongo_current_run_transaction_evidence", current_txn, "当前Mongo run-id对应已验证并清理的事务探测", scope="database")
        if current_owned:
            # 只读服务元数据，不访问玩家集合、不初始化副本集、不写事务探测数据。
            code = ("import json,sys; from pymongo import MongoClient; "
                    "c=MongoClient(sys.argv[1],directConnection=True,serverSelectionTimeoutMS=3000,connectTimeoutMS=2000); "
                    "h=c.admin.command('hello'); b=c.admin.command('buildInfo'); r=c.admin.command('replSetGetConfig')['config']; "
                    "print(json.dumps({'version':b['version'],'set_name':h.get('setName'),'primary':h.get('isWritablePrimary',False),"
                    "'members':[x['host'] for x in r['members']]})); c.close()")
            env = os.environ.copy()
            for name in ["PYTHONHOME", "PYTHONPATH", "PYTHONUSERBASE", "__PYVENV_LAUNCHER__", "VIRTUAL_ENV"]:
                env.pop(name, None)
            result = invoke(mongo.get("driver_python"), ["-I", "-c", code, EXPECTED_URI], timeout=12, env=env)
            try:
                live = json.loads(result["stdout"])
            except (ValueError, KeyError):
                live = {}
            database["current_probe"] = live
            check("mongo_readonly_metadata", result.get("return_code") == 0 and live.get("version") == mongo.get("version")
                  and live.get("set_name") == "minimirDev" and live.get("primary") is True
                  and live.get("members") == ["localhost:27017"],
                  "当前版本、可写PRIMARY与单成员配置只读检查通过", scope="database", evidence=live if live else result)

    tool_missing = [c["message"] for c in checks if c["scope"] == "toolchain" and c["status"] == "failed"]
    db_missing = [c["message"] for c in checks if c["scope"] == "database" and c["status"] == "failed"]
    tool_ready = not tool_missing
    db_ready = not args.skip_database and not db_missing and database.get("current_probe") is not None
    now = datetime.now(timezone(timedelta(hours=8)))
    report = {"schema_version": 2, "checked_on": now.date().isoformat(), "checked_at": now.isoformat(),
              "user_timezone": "Asia/Shanghai", "required_creator_version": "3.8.8",
              "toolchain_entry": "./tools/dev", "wrapper_active": os.environ.get("MINIMIR_TOOLCHAIN_ACTIVE") == "1",
              "checker_python": sys.executable, "tools": tools, "checks": checks, "database": database,
              "toolchain_ready": tool_ready, "development_database_ready": db_ready, "development_ready": tool_ready and db_ready,
              "game_runtime_ready": False, "missing": tool_missing + db_missing,
              "informational_system_tools": {"mongod": {**mongo.get("system_existing_install_preserved", {}), "selected": False},
                                             "docker": {"path": shutil.which("docker"), "required": False}},
              "note": "就绪仅指固定工具链与本机单节点开发数据库；不代表游戏运行、业务迁移、Creator构建、HA或容量已验收。检查不启停或安装服务，不修改数据库。"}
    destination = local_path(args.report)
    destination.parent.mkdir(parents=True, exist_ok=True)
    destination.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"toolchain_ready": tool_ready, "development_database_ready": db_ready,
                      "development_ready": report["development_ready"], "game_runtime_ready": False,
                      "missing": report["missing"], "report": str(destination)}, ensure_ascii=False))
    return 2 if args.strict and not report["development_ready"] else 0


if __name__ == "__main__":
    sys.exit(main())
