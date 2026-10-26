#!/usr/bin/env python3
"""准备可核验的项目内轻量参考副本；来源只读，目标冲突时停止。"""

from __future__ import annotations

import argparse
import csv
import io
import json
import re
import sys
from collections import Counter
from pathlib import Path

sys.dont_write_bytecode = True
import inventory as source_inventory


PROJECT = source_inventory.PROJECT
SOURCE_PARENT = source_inventory.SOURCE_PARENT
MANIFEST_PATH = PROJECT / "资料索引/参考文件清单.json"
LOCAL_MANIFEST = PROJECT / "资料索引/本地参考副本.json"
MYF_REVIEW = {
    "Mud2/DB/ApexM2_Magic.csv": {
        "header_prefix": ["MagID", "MagName", "EffectType"],
        "reason": "技能定义候选，供目标版本筛选、来源ID和数值字段对照；不直接作为正式技能配置",
        "content_kind": "gameplay_definition",
    },
    "Mud2/DB/ApexM2_Monster.csv": {
        "header_prefix": ["Name", "Race", "RaceImg"],
        "reason": "怪物定义候选，供形象ID、属性和AI字段对照；不沿用旧平衡参数",
        "content_kind": "gameplay_definition",
    },
    "Mud2/DB/ApexM2_stditems.csv": {
        "header_prefix": ["Idx", "Name", "Stdmode"],
        "reason": "物品定义候选，供装备/道具ID、外观和属性字段对照；绑定交易规则按最终策划重建",
        "content_kind": "gameplay_definition",
    },
    "供需统计.csv": {
        "header_prefix": ["物品货币", "检测合计", "扣除合计"],
        "reason": "物品货币的历史汇总统计，不含账号/角色维度；仅审计参考，不作正式数值或收益承诺",
        "content_kind": "historical_gameplay_summary",
    },
    "占用变量.csv": {
        "report_markers": ["生成时间:", "扫描路径:", "正常变量"],
        "reason": "脚本变量占用报告，记录变量符号与引用，不是变量运行值、账号记录或正式配置表",
        "content_kind": "script_variable_report",
    },
}


def safe_relative(value: str) -> Path:
    relative = Path(value)
    if relative.is_absolute() or ".." in relative.parts or not relative.parts:
        raise ValueError("拒绝不安全的相对路径")
    return relative


def reject_symlinks(path: Path, boundary: Path) -> None:
    if path != boundary and boundary not in path.parents:
        raise ValueError("路径超出固定目录边界")
    current = path
    while True:
        if current.is_symlink():
            raise ValueError("拒绝符号链接，防止越界或修改源文件")
        if current == boundary:
            break
        current = current.parent


def review_myf_csv(source: Path, relative: str) -> dict:
    """仅审阅固定五份静态玩法资料，不扫描或打开账号库和实际运行配置。"""
    definition = MYF_REVIEW[relative]
    text, encoding = source_inventory.decode_text(source)
    if "header_prefix" in definition:
        header = next(csv.reader(io.StringIO(text)), [])
        if header[:len(definition["header_prefix"])] != definition["header_prefix"]:
            raise ValueError(f"玩法表表头改变，停止复制：{relative}")
    elif not all(marker in text for marker in definition["report_markers"]):
        raise ValueError(f"变量报告结构改变，停止复制：{relative}")
    forbidden = (
        r"(?:password|passwd|secret|token|api_key)\s*[:=]",
        r"\b(?:\d{1,3}\.){3}\d{1,3}:\d{2,5}\b",
        r"(?:account_id|username|password|账号列表|帐号列表)",
    )
    if any(re.search(pattern, text, re.I) for pattern in forbidden):
        raise ValueError(f"检测到需人工复核的账号/凭据/端点字段，停止复制且不输出内容：{relative}")
    return {
        "review_status": "固定玩法资料结构与禁用字段检查通过",
        "review_scope": "物品/技能/怪物定义、无账号维度的供需汇总、脚本变量占用报告",
        "content_kind": definition["content_kind"], "source_text_encoding": encoding,
        "review_is_target_version_or_balance_verification": False,
    }


def build_plan(manifest: dict) -> list[dict]:
    plans = []
    seen_myf = set()
    for rec in manifest["files"]:
        source_id = rec["source_id"]
        relative = safe_relative(rec["relative_path"])
        relative_string = relative.as_posix()
        if source_id not in source_inventory.ROOTS:
            raise ValueError("来源ID不在固定白名单")
        allowed, _ = source_inventory.classify(source_id, relative_string)
        if not allowed:
            raise ValueError("源索引含白名单外条目，停止准备")
        if source_id == "crystal" and relative.suffix.lower() == ".cs":
            target_relative = Path("资料参考/crystal") / relative
            reason = "三职业战斗、怪物/宠物、地图、帧/音效和资源格式相关静态C#源码及必要交叉依赖；仅作参考，不启用来源玩法"
        elif source_id == "ui_reference":
            target_relative = Path("UI参考图") / relative
            reason = "保留原名与原字节的界面参考图，用于Creator场景/预制体制作及视觉核对"
        elif source_id == "myf" and relative_string in MYF_REVIEW:
            target_relative = Path("资料参考/myf") / relative
            reason = MYF_REVIEW[relative_string]["reason"]
            seen_myf.add(relative_string)
        else:
            continue
        source_root = SOURCE_PARENT / source_inventory.ROOTS[source_id]
        source = source_root / relative
        target = PROJECT / target_relative
        reject_symlinks(source, source_root)
        reject_symlinks(target, PROJECT)
        if not source.is_file():
            raise ValueError(f"允许来源不存在：{source_id}:{relative_string}")
        source_sha = source_inventory.digest(source)
        if not rec.get("sha256") or source_sha != rec["sha256"]:
            raise ValueError(f"来源摘要与只读索引不符，请先更新并核验源索引：{source_id}:{relative_string}")
        if target.exists() and (not target.is_file() or source_inventory.digest(target) != source_sha):
            raise ValueError(f"目标已存在且内容冲突，停止且不覆盖：{target_relative.as_posix()}")
        item = {
            "reference_id": rec["reference_id"], "source_id": source_id,
            "source_root": str(source_root), "source_relative_path": relative_string,
            "target_relative_path": target_relative.as_posix(), "bytes": source.stat().st_size,
            "source_sha256": source_sha, "target_sha256": source_sha,
            "copy_mode": "byte_preserving_independent_file", "text_transcoding": False,
            "reason": reason, "runtime_enabled": False,
        }
        if source_id == "myf":
            item["content_review"] = review_myf_csv(source, relative_string)
        if source_id == "crystal":
            item["source_text_encoding"] = rec.get("text_encoding")
            item["source_contains_out_of_scope_dependencies"] = True
            item["dependency_note"] = "静态参考代码可能含英雄/额外职业/攻城等依赖；这些内容不自动纳入当前制作范围"
        plans.append(item)
    if seen_myf != set(MYF_REVIEW):
        raise ValueError("源索引未包含全部五份允许MYF玩法资料；先运行inventory.py")
    if not any(x["source_id"] == "ui_reference" for x in plans):
        raise ValueError("未找到允许UI原图")
    if not any(x["source_id"] == "crystal" for x in plans):
        raise ValueError("未找到允许Crystal参考源码")
    return sorted(plans, key=lambda x: x["target_relative_path"])


def copy_independent(item: dict) -> bool:
    source = Path(item["source_root"]) / item["source_relative_path"]
    target = PROJECT / item["target_relative_path"]
    reject_symlinks(target, PROJECT)
    if target.exists():
        if target.is_file() and source_inventory.digest(target) == item["source_sha256"]:
            return False
        raise ValueError(f"复制前目标冲突，停止：{item['target_relative_path']}")
    target.parent.mkdir(parents=True, exist_ok=True)
    created = False
    try:
        # 独占创建，避免并发运行时覆盖用户或另一进程创建的目标。
        with source.open("rb") as source_stream, target.open("xb") as target_stream:
            created = True
            for block in iter(lambda: source_stream.read(1024 * 1024), b""):
                target_stream.write(block)
        if source_inventory.digest(target) != item["source_sha256"] or source_inventory.digest(source) != item["source_sha256"]:
            raise ValueError(f"复制期间内容变化或摘要不符：{item['target_relative_path']}")
    except Exception:
        if created:
            target.unlink(missing_ok=True)
        raise
    return True


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="只校验来源/目标摘要与副本清单，不创建或修改文件")
    args = parser.parse_args()
    if not MANIFEST_PATH.is_file():
        raise ValueError("缺少只读源索引；先运行tools/reference/inventory.py")
    manifest = json.loads(MANIFEST_PATH.read_text(encoding="utf-8"))
    # 先检查全部来源和目标，任一冲突时尚未复制任何文件。
    plans = build_plan(manifest)
    counts = dict(sorted(Counter(x["source_id"] for x in plans).items()))
    local_manifest = {
        "schema_version": 1, "copy_manifest_version": "local_reference_v1", "runtime_ready": False,
        "scope": "轻量原字节参考副本；不复制地图、图像包、音效大包、账号、凭据或部署配置",
        "source_manifest_path": "资料索引/参考文件清单.json",
        "source_manifest_sha256": source_inventory.digest(MANIFEST_PATH),
        "copied_file_count": len(plans), "copied_bytes": sum(x["bytes"] for x in plans), "counts_by_source": counts,
        "policy": {"modify_source": False, "run_source_programs": False, "follow_symlinks": False,
                   "overwrite_conflict": False, "existing_same_hash": "skip", "hardlink_or_junction": False,
                   "copy_large_resource_packages": False},
        "large_resource_policy": "约4GB原始资源按参考文件清单定位；转换工具与产物写入本项目；少量samples研发样例不代表完整批次或方向映射完成。",
        "source_aliases": source_inventory.source_aliases(), "files": plans,
    }
    expected = json.dumps(local_manifest, ensure_ascii=False, indent=2) + "\n"
    if args.check:
        missing = [x["target_relative_path"] for x in plans if not (PROJECT / x["target_relative_path"]).is_file()]
        if missing:
            raise ValueError(f"缺少{len(missing)}份目标副本；运行prepare_local.py准备")
        if not LOCAL_MANIFEST.is_file() or LOCAL_MANIFEST.read_text(encoding="utf-8") != expected:
            raise ValueError("本地参考副本清单缺失或过期；先准备副本并更新清单")
        copied = 0
    else:
        copied = sum(copy_independent(item) for item in plans)
        LOCAL_MANIFEST.parent.mkdir(parents=True, exist_ok=True)
        LOCAL_MANIFEST.write_text(expected, encoding="utf-8")
    print(json.dumps({"status": "checked" if args.check else "prepared", "files": len(plans),
                      "counts_by_source": counts, "newly_copied": copied, "bytes": local_manifest["copied_bytes"],
                      "source_unchanged": True, "runtime_ready": False}, ensure_ascii=False))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as error:
        print(f"准备失败：{error}", file=sys.stderr)
        raise SystemExit(1)
