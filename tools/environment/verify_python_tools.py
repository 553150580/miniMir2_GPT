#!/usr/bin/env python3
"""检查用户级Python工具链；可显式备份并重建本任务自己的WIL派生样例。"""
from __future__ import annotations

import argparse
import hashlib
import importlib.metadata
import importlib.util
import json
import platform
from pathlib import Path
import shutil
import subprocess
import sys
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parents[2]
SAMPLES = ROOT / "client/assets/imported/samples"
BACKUP = ROOT / ".work/pre-python-upgrade-samples"


def digest(path: Path) -> str:
    hasher = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1 << 20), b""):
            hasher.update(chunk)
    return hasher.hexdigest()


def resource_module():
    path = ROOT / "tools/resources/inspect_wil.py"
    spec = importlib.util.spec_from_file_location("minimir_wil_python_verification", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def pixels_match_current_decoder(directory: Path) -> list[dict]:
    from PIL import Image
    wil = resource_module()
    results = []
    for metadata in sorted((directory / "client_180_named").glob("*/frames.json")):
        old = json.loads(metadata.read_text(encoding="utf-8"))
        source, _ = wil.locate_pair(old["source_id"], metadata.parent.name)
        original_wil = Path(old["wil_path"])
        original_wix = Path(old["wix_path"])
        if digest(original_wil) != old["wil_sha256"] or digest(original_wix) != old["wix_sha256"]:
            raise ValueError(f"来源包或索引发生变化：{metadata.parent.name}")
        data = wil.probe(original_wil, original_wix)
        for sample in old["samples"]:
            if sample["png_path"] is None:
                continue
            relative = Path(sample["png_path"]).relative_to("client/assets/imported/samples")
            image_path = directory / relative
            frame = data["frames"][sample["frame_index"]]
            rgba = wil.decode_frame(source, frame, data["palette"])
            with Image.open(image_path) as image:
                if image.mode != "RGBA" or image.size != (frame["width"], frame["height"]) or image.tobytes() != rgba:
                    raise ValueError(f"新Pillow回读与源RGBA不一致，停止重建：{image_path}")
            rgba_sha = hashlib.sha256(rgba).hexdigest()
            if rgba_sha != sample["decoded_rgba_sha256"]:
                raise ValueError(f"源RGBA摘要与原转换记录不同：{image_path}")
            results.append({"package": metadata.parent.name, "frame_index": frame["frame_index"],
                            "png": str(relative), "png_sha256": digest(image_path), "rgba_sha256": rgba_sha,
                            "previous_pillow_version": old["pillow_version"], "pixel_bytes_equal": True})
    if len(results) != 12:
        raise ValueError(f"预期12张本任务样例，实际{len(results)}张；拒绝扩大重建范围")
    return results


def run(command: list[str]) -> dict:
    result = subprocess.run(command, cwd=ROOT, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError(f"命令失败{result.returncode}: {command}\n{result.stdout}\n{result.stderr}")
    return {"command": command, "exit_code": result.returncode, "stdout": result.stdout, "stderr": result.stderr}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--refresh-samples", action="store_true", help="先备份并确认12帧旧PNG像素一致，再真实重建PNG和frames元数据")
    parser.add_argument("--report", type=Path, default=ROOT / ".work/python-tools-latest-check.json",
                        help="默认只写本地检查结果；安装时conf/toolchain中的快照报告保持不变")
    args = parser.parse_args()
    pillow = importlib.metadata.version("Pillow")
    before = pixels_match_current_decoder(SAMPLES)
    refreshed = None
    if args.refresh_samples:
        if BACKUP.exists():
            raise ValueError("已存在升级前备份；保留它并拒绝覆盖，请先人工核对本轮重建状态")
        BACKUP.parent.mkdir(parents=True, exist_ok=True)
        shutil.copytree(SAMPLES, BACKUP)
        if pixels_match_current_decoder(BACKUP) != before:
            raise ValueError("备份检查与重建前记录不一致")
        # 只删除已验证且已备份的派生PNG和frames元数据，原WIL/WIX和来源索引不动。
        expected_files = {SAMPLES / x["png"] for x in before}
        expected_files.update((SAMPLES / "client_180_named").glob("*/frames.json"))
        for path in expected_files:
            path.unlink()
        refreshed = run([sys.executable, str(ROOT / "tools/resources/inspect_wil.py")])
    resource_check = run([sys.executable, str(ROOT / "tools/resources/inspect_wil.py"), "--check"])
    resource_json = json.loads(resource_check["stdout"])
    if resource_json["status"] != "checked" or resource_json["failures"]:
        raise ValueError("资源check未通过")
    after = pixels_match_current_decoder(SAMPLES)
    # PNG压缩字节可以随依赖版本变化；原始RGBA必须逐字节相同。
    comparable = lambda records: {(x["package"], x["frame_index"]): x["rgba_sha256"] for x in records}
    if comparable(before) != comparable(after):
        raise ValueError("重建后源像素发生变化，升级前样例仍保存在备份目录")
    tests = run([sys.executable, "-m", "unittest", "discover", "-s", "tools/config", "-p", "test_*.py"])
    report = {
        "schema_version": 1, "checked_at_utc": datetime.now(timezone.utc).isoformat(), "status": "passed",
        "python_executable": sys.executable, "python_version": platform.python_version(), "pillow_version": pillow,
        "stdlib_configuration_tests": tests,
        "resource_check": {"command": resource_check["command"], "exit_code": 0, "result": resource_json},
        "sample_refresh": {"requested": args.refresh_samples, "performed": bool(refreshed),
                           "backup_path": str(BACKUP) if args.refresh_samples else None,
                           "source_packages_modified": False, "source_indices_modified": False,
                           "previous_pixels_verified_before_removal": True, "rgba_preserved": True,
                           "before": before, "after": after,
                           "generator_command": refreshed["command"] if refreshed else None},
        "business_or_full_art_validation_passed": False,
        "limitations": ["配置11项回归与12张PNG像素检查不等于业务实现、1.80行为核验或完整美术验收。"],
    }
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"status": "passed", "python": report["python_version"], "pillow": pillow,
                      "configuration_tests": tests["stderr"].strip(), "resource_packages": len(resource_json["results"]),
                      "png_roundtrips": len(after), "rgba_preserved": True, "refreshed_samples": bool(refreshed)}, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
