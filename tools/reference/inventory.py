#!/usr/bin/env python3
"""只读登记已知上级参考资料；不复制资源、不运行原程序、不读取账号与部署配置。"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import struct
from collections import Counter
from pathlib import Path


PROJECT = Path(__file__).resolve().parents[2]
SOURCE_PARENT = PROJECT.parent
SMALL_HASH_LIMIT = 16 * 1024 * 1024
ROOTS = {
    "crystal": "SuprcodeCrystal",
    "myf": "MirServer沉默世界_MYF",
    "client_resources": "客户端资源",
    "client_180_named": "传奇1.80客户端",
    "ui_reference": "UI参考图",
    "historical_architecture": "服务端架构.txt",
}
OUTPUTS = (
    "资料索引/参考文件清单.json",
    "资料索引/技能与资源对照.json",
    "资料索引/UI参考映射.json",
    "资料索引/资料总览.md",
    "docs/技能验收用例.json",
    "docs/资源转换与技能核验.md",
)
ASSET_EXTS = {".wil", ".wix", ".wzl", ".wzx", ".lib", ".wtl", ".map", ".wav", ".mp3", ".ogg", ".bmp", ".png", ".jpg", ".jpeg", ".atr", ".idx"}
KEY_STEMS = {"hum", "hum2", "weapon", "weapon2", "hair", "hair2", "magic", "magic2", "mon1", "tiles", "smtiles", "objects", "prguse", "items", "stateitem", "dnitems", "magicon"}
SENSITIVE_PARTS = {".git", ".svn", "bin", "obj", "logs", "log", "idddb", "iddb", "fdb", "userdata", "guildbase", "countlog", "chatlog", "merchantlog", "trade", "account"}
SENSITIVE_NAME = re.compile(r"password|credential|secret|token|dbsrc|logsrv|setup|serveraddr|addrtable|servertable|send[s]?ms|账号|帐号|密码|凭据|清理|备份|backlist", re.I)
CRYSTAL_PREFIXES = (
    "Server/MirObjects/", "Server/MirDatabase/", "Client/MirObjects/",
    "Client/MirGraphics/", "Client/MirSounds/", "LibraryEditor/Graphics/",
    "LibraryEditor/Functions/", "LibraryViewer/MLibrary.cs", "Shared/Data/",
)
CRYSTAL_EXACT = {
    "Shared/Enums.cs", "Shared/BaseStats.cs", "Shared/Functions/Functions.cs",
    "Server/MirEnvir/Map.cs", "Server/MirEnvir/Envir.cs", "Client/MirControls/MirMapControl.cs",
    "Client/MirScenes/Dialogs/MapControl.cs", "LibraryEditor/LMain.cs",
}
MYF_SCRIPT_PREFIXES = (
    "Mir200/Envir/MonItems/", "Mir200/Envir/SmartMonster/",
    "Mir200/Envir/CustomMagic/", "Mir200/Envir/Npc_Def/",
    "Mir200/Envir/QuestDiary/",
)
MYF_CSV_FILES = {
    "Mud2/DB/ApexM2_Magic.csv": "技能定义候选表",
    "Mud2/DB/ApexM2_Monster.csv": "怪物定义候选表",
    "Mud2/DB/ApexM2_stditems.csv": "物品定义候选表",
    "供需统计.csv": "历史物品货币供需汇总，仅审计参考，不作正式配置",
    "占用变量.csv": "脚本变量占用报告，仅脚本依赖参考，不是运行值或账号记录",
}
SKILL_FILES = (
    "Shared/Enums.cs", "Server/MirObjects/HumanObject.cs",
    "Server/MirObjects/PlayerObject.cs", "Server/MirObjects/SpellObject.cs",
    "Server/MirObjects/MonsterObject.cs", "Client/MirObjects/PlayerObject.cs",
    "Client/MirObjects/SpellObject.cs", "Client/MirObjects/MonsterObject.cs",
)


def digest(path: Path) -> str:
    sha = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            sha.update(block)
    return sha.hexdigest()


def decode_text(path: Path) -> tuple[str, str]:
    raw = path.read_bytes()
    for encoding in ("utf-8-sig", "gb18030"):
        try:
            return raw.decode(encoding), encoding
        except UnicodeDecodeError:
            pass
    raise ValueError(f"文本无法按显式 UTF-8/GB18030 解码：{path.name}")


def classify(source_id: str, relative: str) -> tuple[bool, str]:
    """只接受明确的素材、玩法数据和相关源码；其它内容不打开。"""
    p = Path(relative)
    parts = {part.casefold() for part in p.parts}
    if parts & SENSITIVE_PARTS or SENSITIVE_NAME.search(p.name):
        return False, "sensitive_or_operational"
    if source_id == "historical_architecture":
        return True, "historical_architecture"
    if source_id == "ui_reference":
        return p.suffix.lower() in {".png", ".jpg", ".jpeg"}, "ui_reference"
    if source_id == "crystal":
        allowed = p.suffix.lower() == ".cs" and (relative in CRYSTAL_EXACT or relative.startswith(CRYSTAL_PREFIXES))
        if re.search(r"AccountInfo|CharacterInfo|GuildInfo|Settings|Config|Login|Connection|Network|Market|Trade", p.name, re.I):
            return False, "unrelated_or_operational_source"
        return allowed, "reference_source"
    if source_id == "myf":
        if relative in MYF_CSV_FILES:
            return True, "reference_gameplay_table" if relative.startswith("Mud2/DB/") else "reference_gameplay_report"
        if relative.startswith("Mir200/Map/") and p.suffix.lower() == ".map":
            return True, "reference_map"
        if relative in {"Mir200/Envir/MapInfo.txt", "Mir200/Envir/MonGen.txt", "Mir200/Envir/MonGen (2).txt", "Mir200/Envir/Merchant.txt"}:
            return True, "reference_world_table"
        if relative.startswith(MYF_SCRIPT_PREFIXES) and p.suffix.lower() in {".txt", ".ini"}:
            return True, "reference_gameplay_script"
        return False, "unrelated_or_operational"
    if source_id in {"client_resources", "client_180_named"}:
        allowed = p.parts[0] in {"Data", "Map", "Wav"} and p.suffix.lower() in ASSET_EXTS
        return allowed, "reference_asset"
    return False, "unrecognized"


def file_type(p: Path) -> str:
    return {
        ".cs": "csharp_source", ".csv": "gameplay_csv", ".map": "mir_map_binary",
        ".wil": "wil_image_package", ".wix": "wil_index", ".wzl": "wzl_image_package",
        ".wzx": "wzl_index", ".lib": "crystal_library", ".wtl": "wtl_library",
        ".wav": "audio_wav", ".mp3": "audio_mp3", ".ogg": "audio_ogg",
        ".txt": "reference_text", ".ini": "gameplay_script_ini",
        ".png": "image_png", ".jpg": "image_jpeg", ".jpeg": "image_jpeg",
        ".bmp": "image_bmp", ".atr": "tile_attributes", ".idx": "index_binary",
    }.get(p.suffix.lower(), "reference_binary")


def walk_files(root: Path):
    if root.is_file():
        yield root
        return
    for current, dirs, names in os.walk(root, followlinks=False):
        # 不遍历版本库、构建目录和账号目录；只统计整个被剪枝目录一次，避免列出敏感路径。
        dirs[:] = sorted(d for d in dirs if d.casefold() not in SENSITIVE_PARTS and not (Path(current) / d).is_symlink())
        for name in sorted(names):
            p = Path(current) / name
            if not p.is_symlink() and p.is_file():
                yield p


def inventory(full_hash: bool) -> dict:
    records, sources = [], []
    for source_id, basename in ROOTS.items():
        root = SOURCE_PARENT / basename
        excluded = Counter()
        selected = []
        source = {
            "source_id": source_id, "root": str(root), "exists": root.exists(),
            "access": "read_only", "symlink_traversal": False,
            "role": "历史架构，仅索引不作为当前规则" if source_id == "historical_architecture" else "只读参考，非本项目运行目录",
        }
        if root.exists() and not root.is_symlink():
            for path in walk_files(root):
                relative = path.relative_to(root).as_posix() if root.is_dir() else path.name
                allowed, category = classify(source_id, relative)
                if not allowed:
                    excluded[category] += 1
                    continue
                size = path.stat().st_size
                key = path.stem.casefold() in KEY_STEMS or (path.suffix.lower() == ".map" and path.name.casefold() in {"0.map", "1.map", "2.map", "3.map"})
                do_hash = full_hash or size <= SMALL_HASH_LIMIT or key
                rec = {
                    "reference_id": f"{source_id}:{relative}", "source_id": source_id,
                    "relative_path": relative, "category": category, "file_type": file_type(path),
                    "bytes": size, "sha256": digest(path) if do_hash else None,
                    "hash_status": "sha256_full" if do_hash else "not_hashed_large_binary",
                    "hash_reason": "全部摘要模式" if full_hash else ("首阶段关键包" if key else ("不超过16MiB" if do_hash else "大于16MiB且不在首阶段关键包名单，可用--full-hash补齐")),
                    "source_modified": False, "copied": False,
                }
                if path.suffix.lower() == ".cs":
                    _, rec["text_encoding"] = decode_text(path)
                if path.suffix.lower() == ".csv":
                    content, rec["text_encoding"] = decode_text(path)
                    lines = content.splitlines()
                    rec["header"] = lines[0].split(",") if lines else []
                    rec["physical_data_line_count"] = max(0, len(lines) - 1)
                    rec["row_count_note"] = "按文本行统计，未据此确认有效记录数量、主键合法或完整业务语义"
                    rec["reference_purpose"] = MYF_CSV_FILES.get(relative, "候选玩法数据")
                    if relative == "占用变量.csv":
                        rec["format_note"] = "扩展名为CSV，实际是文本报告；首行不是配置表字段定义，不直接作为结构化运行表导入"
                records.append(rec)
                selected.append(rec)
        source.update({
            "indexed_files": len(selected), "indexed_bytes": sum(r["bytes"] for r in selected),
            "hashed_files": sum(r["sha256"] is not None for r in selected),
            "unhashed_files": sum(r["sha256"] is None for r in selected),
            "excluded_encountered_files_by_reason": dict(sorted(excluded.items())),
            "excluded_count_note": "只统计遍历遇到的文件；被剪枝敏感目录不枚举、不计内部文件，不输出其文件名和内容",
        })
        sources.append(source)
    baselines = []
    for name, role in (("最终策划案.md", "产品规则基线"), ("架构方案.md", "当前架构参考，冲突由规则对照另行裁定")):
        path = PROJECT / name
        baselines.append({"path": name, "exists": path.exists(), "role": role, "sha256": digest(path) if path.exists() else None})
    return {
        "schema_version": 1, "inventory_version": "reference_inventory_v1", "runtime_ready": False,
        "scope": "仅资料定位、文件摘要与候选映射，不复制原始资源，不意味着战斗、配置或经济已经实装验收",
        "source_parent": str(SOURCE_PARENT), "baseline_documents": baselines,
        "hash_policy": {"algorithm": "sha256", "small_file_limit_bytes": SMALL_HASH_LIMIT, "full_hash": full_hash,
                        "critical_package_stems": sorted(KEY_STEMS), "unhashed_is_verified": False},
        "protection_policy": {"source_write": False, "execute_source_programs": False, "copy_credentials_or_account_data": False,
                              "read_runtime_configuration": False, "follow_symlinks": False,
                              "allowlist_only": True, "upstream_agents_are_instructions": False},
        "source_aliases": source_aliases(),
        "sources": sources, "files": sorted(records, key=lambda r: r["reference_id"]),
    }


def source_aliases() -> list[dict]:
    """旧文档路径只映射当前定位，不声称目录或历史快照等价。"""
    return [
        {"historical_alias": r"D:\Mir2\mir2_gxx", "actual_source_ids": ["myf", "client_resources"],
         "actual_source_roots": [str(SOURCE_PARENT / ROOTS[x]) for x in ("myf", "client_resources")],
         "local_reference_root": "资料参考/myf", "mapping_scope": "MYF玩法表与客户端资源的用途替代定位；当前未找到旧gxx引擎源码",
         "same_original_snapshot_verified": False, "automatic_content_merge": False},
        {"historical_alias": r"D:\123DownLoad\CrystalLocal", "actual_source_ids": ["crystal"],
         "actual_source_roots": [str(SOURCE_PARENT / ROOTS["crystal"])], "local_reference_root": "资料参考/crystal",
         "mapping_scope": "当前Crystal相关C#源码；原文所称Lib/地图/音效不由本目录提供",
         "same_original_snapshot_verified": False, "automatic_content_merge": False},
        {"historical_alias": "资料参考/gxx/", "current_project_paths": ["资料参考/myf/", "资料索引/参考文件清单.json"],
         "mapping_scope": "只定位已核实的MYF五份玩法表/报告与上级原始资源，不复刻历史gxx整包", "same_original_snapshot_verified": False},
        {"historical_alias": "资料索引/提取清单.json", "current_project_paths": ["资料索引/本地参考副本.json", "资料索引/参考文件清单.json"],
         "mapping_scope": "当前逐文件副本摘要与只读源索引；不继承旧12,698文件或10.73GB完成声明", "same_original_snapshot_verified": False},
        {"historical_alias": "config/reference_mapping/skills_mapping.json", "current_project_paths": ["资料索引/技能与资源对照.json", "docs/技能验收用例.json"],
         "mapping_scope": "当前67个枚举候选与18类待执行验收契约；不等于旧33技能或132用例已完成", "same_original_snapshot_verified": False},
        {"historical_alias": "assets/imported/", "current_project_paths": ["client/assets/imported/"],
         "mapping_scope": "本任务转换样例归入独立client目录；原始WIL/UI/参考源码位置不变，PNG字节与帧几何已迁移核验，元数据按新路径重生成",
         "verification_path": "conf/toolchain/resource-layout-verification.json", "same_original_snapshot_verified": False},
    ]


def source_ref(records: dict, source_id: str, relative: str, line: int | None = None) -> dict:
    rec = records.get(f"{source_id}:{relative}")
    result = {"reference_id": f"{source_id}:{relative}", "source_id": source_id,
              "relative_path": relative, "line": line, "sha256": rec["sha256"] if rec else None,
              "text_encoding": rec.get("text_encoding") if rec else None}
    if source_id == "crystal":
        result["local_reference_path"] = f"资料参考/crystal/{relative}"
    return result


def extract_skills(manifest: dict) -> dict:
    records = {r["reference_id"]: r for r in manifest["files"]}
    crystal = SOURCE_PARENT / ROOTS["crystal"]
    contents = {}
    for rel in SKILL_FILES:
        if f"crystal:{rel}" in records:
            contents[rel] = decode_text(crystal / rel)[0].splitlines()
    lines = contents.get("Shared/Enums.cs", [])
    in_enum, group, candidates, out_of_scope = False, None, [], Counter()
    jobs = {"Warrior": (0, "战士"), "Wizard": (1, "法师"), "Taoist": (2, "道士")}
    for number, line in enumerate(lines, 1):
        if "public enum Spell : byte" in line:
            in_enum = True
            continue
        if not in_enum:
            continue
        if line.strip() == "}":
            break
        comment = re.match(r"\s*//\s*(.+)", line)
        if comment:
            group = comment.group(1).strip()
        match = re.match(r"\s*(\w+)\s*=\s*(\d+)\s*,?", line)
        if not match or match.group(1) == "None":
            continue
        name, raw_id = match.groups()
        if group not in jobs:
            out_of_scope[group or "unclassified"] += 1
            continue
        refs = []
        for rel, source_lines in contents.items():
            if rel == "Shared/Enums.cs":
                continue
            for no, source_line in enumerate(source_lines, 1):
                if re.search(rf"\bSpell\.{re.escape(name)}\b", source_line):
                    refs.append(source_ref(records, "crystal", rel, no))
        job_id, job_name = jobs[group]
        candidates.append({
            "candidate_id": f"crystal_spell_{raw_id}", "source_spell_id": int(raw_id),
            "source_symbol": name, "source_job_group": group, "project_job_id": job_id,
            "job_name": job_name, "chinese_name": None,
            "target_version": "传奇2 1.80", "target_version_status": "待核验",
            "is_final_skill_list": False, "enabled_for_runtime": False,
            "inclusion_decision": "待来源版本、学习条件、范围与伤害对照后决定；本组包含后续扩展技能候选",
            "enum_evidence": source_ref(records, "crystal", "Shared/Enums.cs", number),
            "code_references": refs, "implementation_reference_found": bool(refs),
            "implementation_search_scope": list(SKILL_FILES),
            "behavior_verified": False, "resource_mapping_status": "待转换验证",
            "parameters": {"learn_level": None, "cast_ms": None, "hit_event_ms": None, "cooldown_ms": None,
                           "mp_cost": None, "range_pattern_id": None, "damage_pipeline_id": None,
                           "animation_mapping_id": None, "target_filters": None},
            "missing_evidence": ["目标版本可靠出处与差异", "中文名及源表ID交叉映射", "学习与基础等级", "八方向命中格/阻挡", "耗蓝、动作与命中时序", "逐技能伤害/治疗/Buff流程", "实际美术帧与音效", "对照用例执行结果"],
            "acceptance_case_ids": ["SK-COMMON-001", "SK-COMMON-002", "SK-COMMON-003", "SK-COMMON-004", "SK-COMMON-005", "SK-COMMON-006", "SK-COMMON-007"],
        })
    def refs_for(pattern):
        regex = re.compile(pattern, re.I)
        return [r["reference_id"] for r in manifest["files"] if r["source_id"] in {"client_resources", "client_180_named"} and regex.search(r["relative_path"])]
    domains = [
        ("actor_body", "职业三形象基础动作", r"^Data/Hum[^/]*\.(wil|wix|wzl|wzx|lib)$", ["站立", "行走", "跑步", "普攻", "施法", "受击", "死亡"], "Client/MirObjects/Frames.cs"),
        ("weapon", "武器与身体叠层", r"^Data/Weapon[^/]*\.(wil|wix|wzl|wzx|lib)$", ["八方向动作对齐", "前后遮挡", "持武器帧"], "Client/MirObjects/PlayerObject.cs"),
        ("hair", "头发叠层", r"^Data/Hair[^/]*\.(wil|wix|wzl|wzx|lib)$", ["固定职业形象", "各动作逐帧对齐"], "Client/MirObjects/PlayerObject.cs"),
        ("pet", "职业宠物", r"^Data/Mon[^/]*\.(wil|wix|wzl|wzx|lib)$", ["跟随", "移动", "攻击", "受击", "死亡", "召唤与变体"], "Client/MirObjects/MonsterObject.cs"),
        ("monster", "怪物", r"^Data/Mon[^/]*\.(wil|wix|wzl|wzx|lib)$", ["形象ID与包号对应", "八方向动作", "攻击事件"], "Client/MirObjects/Frames.cs"),
        ("skill_effect", "技能图标、弹道与效果", r"^Data/(Magic|Effect|Dragon|MagIcon)[^/]*\.(wil|wix|wzl|wzx|lib)$", ["起手", "飞行", "命中", "地面持续", "Buff"], "Client/MirObjects/PlayerObject.cs"),
        ("map", "地图二进制", r"^Map/.*\.map$", ["尺寸", "列优先格序", "碰撞", "图层引用", "16×16裁切"], "Server/MirEnvir/Map.cs"),
        ("tile", "地砖与地图物件", r"^Data/(Tiles|SmTiles|Objects|mmap)[^/]*\.(wil|wix|wzl|wzx|lib)$", ["格子定位", "遮挡", "透明度", "绘制偏移"], "Client/MirObjects/MapCode.cs"),
        ("ui", "旧客户端UI与物品图标", r"^Data/(Prguse|ChrSel|Items|DnItems|StateItem)[^/]*\.(wil|wix|wzl|wzx|lib)$", ["图标对应", "裁边", "缩放", "Creator内置组件适配"], "Client/MirGraphics/MLibrary.cs"),
        ("audio", "技能、怪物与界面音效", r"^Wav/.*\.(wav|mp3|ogg)$", ["编号映射", "触发事件", "时长", "循环与音量"], "Client/MirSounds/SoundList.cs"),
    ]
    assets = []
    for asset_id, name, pattern, required, rel in domains:
        refs = refs_for(pattern)
        assets.append({"asset_domain_id": asset_id, "name": name, "candidate_file_count": len(refs),
                       "candidate_reference_ids": refs, "code_evidence": source_ref(records, "crystal", rel),
                       "required_checks": required, "mapping_status": "待转换验证", "runtime_ready": False,
                       "frame_sequence": None, "direction_stride": None, "draw_offset": None,
                       "action_timing_ms": None, "placeholder_frame_policy": "保留原始索引，不自动居中或删除空帧",
                       "note": "候选文件仅按包名定位；宠物/怪物具体形象ID与技能效果帧尚未确认"})
    return {"schema_version": 1, "runtime_ready": False, "candidate_source": "Crystal Shared/Enums.cs 中三职业注释分组",
            "candidate_count": len(candidates), "candidate_count_is_complete_target_version": False,
            "count_note": "数量仅表示来源枚举候选，不承诺33项或当前数量就是1.80完整清单；后续必须筛选补证。",
            "excluded_enum_groups": dict(out_of_scope),
            "character_visuals": [{"job_id": 0, "gender_id": 0, "name": "男战士"}, {"job_id": 1, "gender_id": 1, "name": "女法师"}, {"job_id": 2, "gender_id": 0, "name": "男道士"}],
            "character_visual_status": "固定产品形象已确定，实际包内衣服/性别/动作帧映射待验证；不拆分装备itemID",
            "skills": candidates, "asset_domains": assets,
            "format_tool_evidence": [source_ref(records, "crystal", p) for p in (
                "LibraryEditor/Graphics/WeMadeLibrary.cs", "LibraryEditor/Graphics/MLibraryV0.cs", "LibraryEditor/Graphics/MLibraryV1.cs", "LibraryEditor/Graphics/MLibraryV2.cs", "LibraryEditor/Graphics/WTLLibrary.cs", "Server/MirEnvir/Map.cs")],
            "material_claim": "目录名传奇1.80客户端和Crystal/MYF项目均非目标版本真实性证明；按来源和行为分别核验。"}


def image_dimensions(path: Path):
    with path.open("rb") as stream:
        start = stream.read(24)
        if start[:8] == b"\x89PNG\r\n\x1a\n":
            return list(struct.unpack(">II", start[16:24]))
        if start[:2] == b"\xff\xd8":
            stream.seek(2)
            while True:
                byte = stream.read(1)
                if not byte:
                    break
                if byte != b"\xff":
                    continue
                marker = stream.read(1)
                while marker == b"\xff":
                    marker = stream.read(1)
                if marker in (b"\xd8", b"\xd9"):
                    continue
                raw_length = stream.read(2)
                if len(raw_length) != 2:
                    break
                length = struct.unpack(">H", raw_length)[0]
                if marker and marker[0] in {0xC0, 0xC1, 0xC2, 0xC3, 0xC5, 0xC6, 0xC7, 0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF}:
                    frame = stream.read(5)
                    height, width = struct.unpack(">HH", frame[1:5])
                    return [width, height]
                stream.seek(length - 2, 1)
    return None


def saved_main_ui_scene() -> dict | None:
    """只读定位本项目已保存的开发场景；不修改场景、不推断视觉验收。"""
    relative = Path("client/assets/scenes/Game.scene")
    path = PROJECT / relative
    if not path.is_file():
        return None
    cursor = path
    while cursor != PROJECT:
        if cursor.is_symlink():
            raise ValueError("开发场景只读登记不接受符号链接")
        cursor = cursor.parent
    return {"path": relative.as_posix(), "sha256": digest(path), "bytes": path.stat().st_size,
            "status": "可编辑场景已保存；视觉/交互与真机验收以独立阶段报告为准",
            "probe_basis": "仅只读确认文件存在及原字节SHA-256，未由索引生成器执行视觉/交互/真机验收"}


def ui_mapping(manifest: dict) -> dict:
    mapping = {
        "01_登录.png": ("登录", "登录页结构参考"), "02_选服.png": ("选服", "入口服与角色导航参考"),
        "03_创角.png": ("创角", "固定职业形象选择参考；不新增性别选择"),
        "主界面参考图.png": ("主界面", "候选布局参考，未证明与指定最高优先JPG相同"),
        "角色预览与背包.jpg": ("角色与背包", "装备部位、预览和背包参考"),
        "技能.png": ("技能", "学习、升级和快捷入口参考"),
        "道具交易行.png": ("道具市场", "竞购/直购列表参考"),
        "道具交易行2.png": ("道具市场", "候选交互补充，差异待人工确认"),
        "金币交易行.png": ("金币市场", "金币撮合列表与下单参考"),
        "我的摊位.png": ("个人挂单", "订单与仓位参考"),
        "我的交易记录.png": ("交易记录", "已成交与流水展示参考"),
    }
    images = []
    for rec in manifest["files"]:
        if rec["source_id"] != "ui_reference":
            continue
        name = Path(rec["relative_path"]).name
        screen, purpose = mapping.get(name, ("待归类", "人工确认"))
        images.append({"reference_id": rec["reference_id"], "relative_path": rec["relative_path"],
                       "source_kind": "upstream_reference_copy",
                       "local_reference_path": f"UI参考图/{rec['relative_path']}",
                       "sha256": rec["sha256"], "dimensions_px": image_dimensions(SOURCE_PARENT / ROOTS["ui_reference"] / rec["relative_path"]),
                       "screen": screen, "purpose": purpose, "mapping_basis": "仅文件名归类，未进行视觉一致性验收",
                       "creator_scene": None, "creator_prefab": None, "implementation_status": "待制作", "source_unchanged": True})
    upstream_count = len(images)
    legacy_required = "主界面UI布局参考.jpg"
    legacy_found = any(Path(r["relative_path"]).name == legacy_required for r in images)
    scene = saved_main_ui_scene()
    registry_path = PROJECT / "资料索引/用户确认参考.json"
    registry = json.loads(registry_path.read_text(encoding="utf-8")) if registry_path.is_file() else {"schema_version": 1, "references": []}
    if registry.get("schema_version") != 1 or not isinstance(registry.get("references"), list):
        raise ValueError("用户确认参考登记格式不支持")
    supplied = []
    ids = {row["reference_id"] for row in images}
    for entry in registry["references"]:
        if entry.get("reference_kind") != "ui_layout_image" or entry.get("confirmation_status") != "user_confirmed":
            raise ValueError("只接受用户明确确认的UI图片登记")
        relative = Path(entry["local_reference_path"])
        if relative.is_absolute() or ".." in relative.parts or relative.parts[:2] != ("UI参考图", "用户确认"):
            raise ValueError("用户参考必须位于本项目UI参考图/用户确认/内")
        path = PROJECT / relative
        cursor = path
        while cursor != PROJECT:
            if cursor.is_symlink():
                raise ValueError("用户参考不接受符号链接")
            cursor = cursor.parent
        if relative.suffix.lower() not in {".png", ".jpg", ".jpeg"} or not path.is_file():
            raise ValueError("用户登记图片不存在或格式未支持")
        actual_hash, actual_bytes, dimensions = digest(path), path.stat().st_size, image_dimensions(path)
        if (actual_hash != entry["expected_sha256"] or actual_bytes != entry["expected_bytes"]
                or dimensions != entry["expected_dimensions_px"]):
            raise ValueError("用户登记图片摘要、字节或尺寸不符；只读核验失败，不覆盖图片")
        if entry["reference_id"] in ids:
            raise ValueError("UI参考ID重复")
        ids.add(entry["reference_id"])
        row = {"reference_id": entry["reference_id"], "source_kind": "project_user_supplement",
               "relative_path": relative.relative_to("UI参考图").as_posix(),
               "local_reference_path": relative.as_posix(), "sha256": actual_hash, "bytes": actual_bytes,
               "dimensions_px": dimensions, "screen": entry["screen"], "layout_role": entry.get("layout_role"),
               "priority": entry.get("priority"), "confirmation_status": "user_confirmed",
               "confirmation_date": entry.get("confirmation_date"), "confirmation_evidence": entry["confirmation_evidence"],
               "purpose": "用户明确确认的当前布局参考", "mapping_basis": "用户附图与明确说明，并核验项目内原字节图片",
               "same_as_legacy_requested_jpg": None, "same_as_upstream_main_ui_png": None,
               "creator_scene": None, "creator_prefab": None, "implementation_status": "待制作",
               "runtime_ready": False, "source_unchanged": True, "copied_by_prepare_local": False}
        if entry.get("layout_role") == "main_ui_layout" and entry.get("priority") == "current_highest" and scene:
            row.update({"creator_scene": scene["path"], "creator_scene_sha256": scene["sha256"],
                        "creator_scene_bytes": scene["bytes"], "creator_scene_probe_basis": scene["probe_basis"],
                        "implementation_status": scene["status"], "status": scene["status"]})
        row["integrity_verified"] = True
        supplied.append(row)
    images.extend(supplied)
    highest = [row for row in supplied if row.get("layout_role") == "main_ui_layout" and row.get("priority") == "current_highest"]
    if len(highest) > 1:
        raise ValueError("当前最高优先主界面参考只能有一项")
    current = highest[0] if highest else None
    return {"schema_version": 2, "runtime_ready": False, "actual_image_count": len(images),
            "upstream_image_count": upstream_count, "user_supplement_image_count": len(supplied),
            "document_claimed_count": 12,
            "count_status": f"上级参考{upstream_count}张、项目内用户补充{len(supplied)}张、当前总计{len(images)}张；不将新增图冒充历史原有12图的一员",
            "user_registry_path": "资料索引/用户确认参考.json", "user_registry_sha256": digest(registry_path) if registry_path.is_file() else None,
            "current_highest_priority_reference": current,
            "current_main_layout_reference_available": bool(current) or legacy_found,
            "current_main_layout_user_confirmed": bool(current),
            "active_main_ui_reference_id": current["reference_id"] if current else None,
            "active_main_ui_reference_path": current["local_reference_path"] if current else None,
            "active_main_ui_reference_available": bool(current),
            "active_main_ui_reference_user_confirmed": bool(current),
            "current_highest_priority_status": current["implementation_status"] if current and current.get("creator_scene") else ("用户已确认当前最高优先布局，可编辑开发场景待登记" if current else "待明确当前布局基线"),
            "legacy_requested_file": legacy_required, "legacy_requested_file_found": legacy_found,
            "highest_priority_requested_file": legacy_required, "highest_priority_file_found": legacy_found,
            "highest_priority_status": "历史指定JPG已找到，仍待核验" if legacy_found else "历史指定JPG仍未发现；当前用户确认布局另见active字段",
            "png_is_same_as_required_jpg": None, "automatic_substitution_allowed": False,
            "resolution_action": ("已保存可编辑场景与预制体的视觉、交互和真机验收以独立阶段报告为准；索引只登记来源和场景摘要，不宣称与历史JPG或PNG相同。" if current.get("creator_scene") else "使用用户已明确确认的PNG作为当前布局基线，制作和验收可编辑.scene/.prefab；不宣称与历史JPG或PNG相同。") if current else "补齐指定JPG或由用户明确确认当前布局；不能宣布主界面布局验收通过。",
            "fixed_ui_policy": "固定层级与布局保存在可编辑.scene/.prefab，代码绑定引用和动态数据；旧截图/工具代码不是Creator资产。",
            "images": images}


def ui_source_summary(ui: dict) -> str:
    summary = (f"上级原有参考保留 **{ui['upstream_image_count']}** 张，项目内用户补充 **{ui['user_supplement_image_count']}** 张，"
               f"当前UI参考总计 **{ui['actual_image_count']}** 张。")
    current = ui["current_highest_priority_reference"]
    if current:
        summary += (f"当前最高优先布局来源为 `{current['local_reference_path']}`，由用户附图并明确说明“主界面ui布局参考”，"
                    f"已只读核验SHA-256、{current['bytes']}字节及{current['dimensions_px'][0]}×{current['dimensions_px'][1]}像素；缺少当前主界面参考的问题已解决。")
    else:
        summary += "当前最高优先主界面参考仍需明确。" if not ui["highest_priority_file_found"] else "历史指定名称的参考已找到，仍需核对布局基线。"
    summary += (f"历史指定 `{ui['legacy_requested_file']}` {'已找到' if ui['legacy_requested_file_found'] else '仍未发现'}；"
                "不声称用户新增PNG与该JPG或上级既有`主界面参考图.png`相同。")
    if current and current.get("creator_scene"):
        summary += (f"已只读登记开发场景 `{current['creator_scene']}` 及SHA-256 `{current['creator_scene_sha256']}`；"
                    "可编辑场景已保存；视觉/交互与真机验收以独立阶段报告为准，runtime_ready仍为false。")
    else:
        summary += "可编辑开发场景尚未登记；图片来源确认不等于布局验收，runtime_ready仍为false。"
    return summary


def ui_next_action(ui: dict) -> str:
    current = ui["current_highest_priority_reference"]
    if current and current.get("creator_scene"):
        return "对照已确认图与可编辑场景的视觉、交互及真机验收以独立阶段报告为准；本索引仅登记来源、场景存在性与摘要。"
    return "下一步以已确认图制作可编辑.scene/.prefab，完成固定布局与交互验收。"


def acceptance_cases(skill_map: dict) -> dict:
    templates = [
        ("SK-COMMON-001", "施放前置与耗蓝", "用有效/无效目标、MP足够/不足、冷却未结束/结束逐项施放。", "失败不扣永久资源；成功的扣耗、动作、冷却和命中事件按核验规则发生。"),
        ("SK-COMMON-002", "八方向与基础命中", "在八方向布置同距离合法/非法目标，记录方向、命中格及结果。", "命中由逻辑格和合法目标决定，与屏幕坐标、特效包围盒无关。"),
        ("SK-COMMON-003", "距离与阻挡", "改变距离与直线/斜角阻挡，比较追击、可施放与命中。", "按来源规则处理可达、射程、视线；不穿越禁止格或靠重连绕过。"),
        ("SK-COMMON-004", "动作与伤害时序", "记录施放、前后摇、弹道/命中与HP变化时间，使用固定输入重放。", "逐技能时序、攻击上下限、命中/防御/幸运扩展接入可解释；不以通用简式代替。"),
        ("SK-COMMON-005", "健康重连与托管", "战斗中断线、经account重新取票接入，跨持续效果与冷却边界。", "保留健康内存现场与事件进度，不刷新HP/CD或重复扣耗发奖；托管执行同一规则。"),
        ("SK-COMMON-006", "真实故障", "施法/持续效果中触发房间或进程故障，完成旧执行停止和换代屏障。", "临时战斗现场不从数据库恢复；只保留已提交永久事实，未提交收益不补造。"),
        ("SK-COMMON-007", "基础与升级扩展分验", "对照基础等级与配置的扩展等级，比较伤害/治疗/时长和命中格。", "只改变经批准的数值字段；不暗增射程、目标数或攻击方式。"),
        ("SK-MELEE-001", "近战贴身与八方向", "八方向相邻格各放敌人，逐个普攻/近战技能并观察宠物占格。", "准确命中允许格；斜向距离、转向与攻击间隔符合核验来源。"),
        ("SK-MELEE-002", "斜角与卡位", "墙角、狭口、玩家宠物与怪物互相卡位时施放近战与位移。", "移动和命中分别遵守来源阻挡规则，无斜角穿墙或越占格。"),
        ("SK-RANGE-001", "直线范围", "八方向直线上布置多个目标、墙后目标及线外目标。", "命中格集、穿透与目标过滤逐项对照，不能全屏伤害。"),
        ("SK-RANGE-002", "扇形范围", "旋转朝向并放置扇形边缘、内角和背后目标。", "相对格偏移与方向旋转一致，边界无多一格/漏一格。"),
        ("SK-RANGE-003", "地面持续区域", "目标进入/离开区域、死亡、刷新区域或重复释放。", "每跳间隔、逐格命中、目标过滤、叠加与结束时点明确。"),
        ("SK-PET-001", "召唤与单宠", "无宠/已有宠物时召唤、重新召唤、切图和死亡。", "按本作1宠约束执行并登记与来源差异；永久资源先扣后生效。"),
        ("SK-PET-002", "宠物移动与攻击", "宠物跟随玩家经过狭口，远程/近战怪物分组测试。", "宠物独立寻路、距离、攻击与转火可解释，无默认一律远程。"),
        ("SK-PET-003", "诱惑与召唤资格", "测试怪物类别、等级、存活状态及控制免疫边界。", "按来源与本作约束判断资格、成功率、持续和解除，不用名称猜行为。"),
        ("SK-BUFF-001", "叠加与解除", "同源/异源重复施加，覆盖刷新、取强、叠层、到期、驱散和死亡。", "Buff叠加规则与计算阶段唯一，无重复乘区、无限叠加或反伤递归。"),
        ("SK-BUFF-002", "防御与治疗流程", "组合护盾、防御、魔御、治疗、毒与本作幸运扩展。", "结算先后、取整、上下限和耗材有来源或批准扩展依据。"),
        ("SK-SYNC-001", "客户端帧与命中一致", "同步录制客户端帧号、绘制偏移、声音与服务端命中事件。", "八方向动作与技能触发可对应；表现差异不能改变服务端命中规则。"),
    ]
    cases = []
    for case_id, title, steps, expected in templates:
        cases.append({"case_id": case_id, "title": title, "status": "待执行", "applicable_skill_candidates": "按技能行为筛选；通用用例适用全部候选",
                      "preconditions": ["目标来源版本和文件摘要已固定", "相关基础参数/命中格/时序预期有可靠证据", "服务端与客户端配置版本相同"],
                      "steps": steps, "expected_contract": expected, "verified_numeric_expectation": None,
                      "source_evidence": [], "execution_artifacts": [], "result": None,
                      "note": "本文件是待执行的验收契约；没有具体来源预期或执行产物时不得填通过。"})
    return {"schema_version": 1, "runtime_ready": False, "case_count": len(cases),
            "candidate_count": skill_map["candidate_count"], "all_status": "待执行",
            "target_version": "传奇2 1.80，基础行为与本作扩展分别验收", "cases": cases}


def overview(manifest: dict, skill_map: dict, ui: dict) -> str:
    rows = []
    for src in manifest["sources"]:
        rows.append(f"| {ROOTS[src['source_id']]} | {src['indexed_files']} | {src['indexed_bytes'] / 1024**3:.3f} GiB | {src['hashed_files']} / {src['unhashed_files']} | {'存在' if src['exists'] else '缺失'} |")
    table = "\n".join(rows)
    return f"""# 只读参考资料总览

本目录由 `python3 tools/reference/inventory.py` 生成。产品规则以当前项目《最终策划案.md》为依据；当前《架构方案.md》作为架构参考，冲突项须经规则对照裁定。上级《服务端架构.txt》只登记为历史资料，不取代当前文件。所有来源均保持原样，不运行源程序、不复制全量资源。

## 实存与摘要

| 来源 | 已登记文件 | 字节合计 | 已SHA-256 / 未SHA-256 | 状态 |
| --- | ---: | ---: | ---: | --- |
{table}

上级允许来源合计登记 **{len(manifest['files'])}** 个文件，项目内用户补充参考另行登记，不计入该上级统计。这里是允许范围的索引数量，不是源目录全部文件数量。账号库、部署配置、凭据、日志、可执行文件和不相关源码不登记内容；敏感目录直接跳过，不枚举内部文件。`参考文件清单.json` 给出每个允许文件的来源、相对路径、类型、字节、完整摘要或未摘要原因。

默认完整计算不超过16MiB的允许文件以及Hum/Weapon/Hair/Magic/Mon1/Tiles等首阶段关键包摘要；大二进制未计算摘要时明确标记，不能将其视为校验通过。需要补全时运行 `python3 tools/reference/inventory.py --full-hash`。未进行来源前后全面哈希比对，不能声称已完成来源完整性保护验收。

## 使用分工

- **SuprcodeCrystal**：真实C#源码；用于技能/怪物/宠物行为和WIL/WZL/Lib/地图格式对照。不是本项目Go服务端或Creator实现，也不是官方1.80真实性证明。
- **MirServer沉默世界_MYF**：玩法表、刷怪/掉落/NPC脚本与地图候选；不读取账号库、生产连接配置、部署日志。旧绑定、幸运、货币、经验、掉落与充值口径不得覆盖最终策划。
- **客户端资源 / 传奇1.80客户端**：图像包、索引、地图、音效的原始候选。两套同名包分别保留来源，不互相覆盖；目录命名不证明版本、权利范围或帧映射正确。
- **UI参考图**：{ui_source_summary(ui)}上级11图的轻量副本由 `tools/reference/prepare_local.py` 管理，用户补充图由 `用户确认参考.json` 手工登记并只读校验，不加入上级复制项。

## 技能与资产准备状态

从Crystal三职业枚举提取 **{skill_map['candidate_count']}** 个候选；全部目标1.80状态为“待核验”，不是已确认完整技能清单。候选附枚举/实现引用的路径、行号和文件SHA-256，未知参数保留null。刺客、弓手、自定义及地图事件组不当作本作职业技能导入。

`技能与资源对照.json` 覆盖男战士、女法师、男道士基础动作，以及武器、头发、宠物、怪物、技能、地图、瓦片、UI与音效。文件名定位不代表完整包内帧映射完成；首循环开发子集已有原帧像素与候选动作映射，完整职业资源、目标版本归属及动作/命中时序仍待专项验收。`docs/技能验收用例.json` 是待执行契约，不能作为已经通过的对拍报告。

## 项目内轻量参考副本

`python3 tools/reference/prepare_local.py` 可准备上级UI原图、白名单Crystal源码和MYF五份CSV/报告的原字节副本，独立副本统计仍为321文件（305源码、5份CSV/报告、上级11图），不包含用户补充1图。已存在且摘要相同则跳过，任一目标冲突先停止；`--check` 校验来源、目标和副本清单。不会复制地图和素材大包。上级约4GB原始资料仍按本索引定位，转换产物写入本项目 `client/assets/imported/` 等受控目录。`client/assets/imported/samples/` 保留12张有限WIL格式样例；首循环开发批次单独位于 `client/assets/resources/development/`，包含1593张原帧PNG、160组八方向动作和真实16×16地图局部，20页动作联系图已查看。它们是显式development构建的可播放候选，不代表完整资源或目标1.80验收；生成与配置边界见[首循环候选说明](../docs/首循环资源与配置候选.md)。历史Windows来源和旧索引路径见 `source_aliases`，别名仅提供当前定位，不证明历史快照一致。

## 可重复运行

```sh
python3 tools/reference/inventory.py
python3 tools/reference/inventory.py --check
```

扫描根固定为本项目上级六个已知来源，不接受任意扫描目录；不跟随符号链接，不启动exe，不向来源写入。`--check` 在内存中重建预期索引并比较，不修改任何输出。索引只服务研发，不应打入客户端发布包；正式发布需补齐规则/资源验证和配置门禁。
"""


def conversion_doc(ui: dict) -> str:
    return """# 资源转换与技能核验实施说明

本文是资料准备与后续实施说明，不构成另一份策划案。原始来源只读；当前《最终策划案.md》决定产品规则，当前《架构方案.md》作为架构参考，冲突由规则对照处理，上级旧架构仅历史。

## 当前产物

`资料索引/参考文件清单.json` 定位上级允许参考的源码、玩法表、图像包/索引、地图、音效和UI图；`技能与资源对照.json` 提供三职业技能枚举候选及源码位置/摘要；`UI参考映射.json` 根据当前__UI_COUNT__张图记录界面归类、尺寸、摘要与最高参考状态，其中上级原有__UPSTREAM_UI_COUNT__张，项目用户补充__USER_UI_COUNT__张。用户补充图由 `资料索引/用户确认参考.json` 单独登记，不计入上级6998项索引或321项复制统计。所有候选均未作为正式运行配置启用，首批仅可由显式development构建载入。本次未全量复制或转换资源，未运行原始Windows程序，未执行技能对拍。

项目内轻量副本由 `python3 tools/reference/prepare_local.py` 准备：UI原图写入 `UI参考图/`，白名单Crystal源码写入 `资料参考/crystal/`，MYF五份CSV/报告写入 `资料参考/myf/` 并保留相对路径。全部原字节复制，双端SHA-256与复制理由见 `资料索引/本地参考副本.json`；已存在且摘要相同跳过，冲突停止。供需汇总与变量占用报告只作历史审计/脚本参考，不能当正式数值表。副本中出现后续职业、英雄或攻城依赖源码，不表示纳入本作范围。

原始约4GB地图、图像与音效资料按索引定位，未全量复制。12张格式样例保留在 `client/assets/imported/samples/`；首循环开发批次单独导入 `client/assets/resources/development/`，包含1593张原帧PNG和一张真实16×16源地图局部，供显式development构建使用。历史Windows路径和旧 `资料参考/gxx/` 等名称通过索引 `source_aliases` 指向当前真实来源或轻量副本；映射只是用途定位，不声称两者是同一历史发布快照。

## 已实现的有限WIL转换起点

`tools/resources/inspect_wil.py` 对照已复制的Crystal `LibraryEditor/Graphics/WeMadeLibrary.cs`，仅支持本地实际存在的一种严格子格式。它读取来源，不运行原始exe，不修改包/索引。所用Python依赖仅标准库与已锁定Pillow13.0.0。

| 格式/能力 | 当前支持 | 验证与拒绝条件 |
| --- | --- | --- |
| WIL/WIX v1.0、内嵌256色、version=0 | 支持探测和少量PNG导出 | 固定签名、声明/索引帧数一致、WIX 48字节头、8字节帧头、色板和像素边界检查 |
| 像素与透明度 | 8位索引、原包色板、原始bottom-up行序 | 按Crystal读取规则保留索引0透明，其余色板项不猜颜色；拒绝未知高字节色板语义 |
| 原索引与偏移 | 全部帧号、尺寸、偏移逐项记录 | 不删除小帧、不居中、不裁边、不缩放；零索引/零尺寸仅记录元数据，不伪造替代PNG |
| 明确帧号选择 | `--package Hum --frames 0,1` | 单包最多16个唯一合法索引，输出名保留原帧号 |
| WZL/WZX、MIZ/MIX、Lib、WTL | 不支持 | 显式拒绝，不尝试套用WIL结构 |
| 压缩WIL、16/32位、非零版本、未知色板 | 不支持 | 显式拒绝，不猜测压缩方式或颜色 |
| 八方向、动作/技能时序与外观ID映射 | 未完成 | `direction_stride/action_mapping/timing_ms`保持null，不能将样例当正式动画 |

默认对“传奇1.80客户端/Data”的Hum、Weapon、Hair、Mon1、Tiles、Magic各导出2个可见帧，共12张PNG。六份 `client/assets/imported/samples/client_180_named/<包名>/frames.json` 保留全部80,008个原索引的帧号/尺寸/偏移，以及来源WIL/WIX完整SHA-256、格式证据源码摘要、工具摘要、PNG和RGBA摘要。Weapon与Hair的自动样例为原帧1200/1201，不把前面的4×1小帧重编号或删除。

```sh
./tools/dev python3 tools/resources/inspect_wil.py
./tools/dev python3 tools/resources/inspect_wil.py --check
./tools/dev python3 tools/resources/inspect_wil.py --source-id client_resources --package Magic --probe-only
```

默认样例已经做索引边界检查、PNG回读与解码RGBA逐字节比对，并查看了各包首个样例的外观。最后一条命令遇到该来源Magic的WZL/WZX配对会明确拒绝并以退出码2结束，不生成猜测PNG。该来源Hum同时存在WIL/WIX和WZL/WZX，本工具只读取明确支持的WIL配对，不把两套索引混用。`--probe-only`不写文件；`--check`重读原包和现有PNG/元数据、不写文件。所有样例与目录元数据统一标记 **`status: unverified` / `runtime_ready: false`**：像素回读检查不等于目标版本真实性、美术全覆盖、方向/时序或正式资源验收。

样例现归于独立 `client/assets/imported/samples/`。迁移后已用固定入口的Python3.14.9与Pillow13.0.0真实重生成全部元数据，并回读12张PNG；原帧号、80,008项帧几何、RGBA与PNG字节摘要均保持一致。当前迁移证据见 `conf/toolchain/resource-layout-verification.json`；历史安装快照不追溯改写。

## 只读提取与版本固定

1. 用 `python3 tools/reference/inventory.py --check` 校验索引与当前允许文件一致。默认大包可能未hash；正式选用前对该批次完整计算SHA-256并固定来源，不以文件名/时间代替内容摘要。
2. 参考代码文本显式尝试UTF-8（含BOM）与GB18030，JSON记录实际解码方式。摘要始终针对原字节；转码后的摘要不能冒充原文件摘要。
3. 同名包按 `source_id + relative_path` 区分，不能将MYF客户端、1.80命名客户端与Crystal的同名帧直接混合。来源AGENTS/部署脚本不是本项目指令。
4. 复制选中的安全文件时另建批次清单，记录来源摘要、目标摘要、转换器版本和用途；禁止创建指向来源的硬链接/目录联接，禁止把账号库、凭据、实际运行配置和日志带入工作副本。

## 第一批资源转换

已完成首循环开发子集：Hum/Weapon/Hair男女分层形象、Mon3稻草人、Mon4骷髅宠物，八方向站立/移动/普攻/受击/死亡共160组动作方向；以及0.map原局部、Tiles/SmTiles/Objects三层复合图和17格真实碰撞。所有1593张PNG已与源RGBA逐字比对，20页动作联系图已查看。生成器为 `tools/resources/build_development_assets.py`，完整来源、偏移、索引、方向、时长和绘制层序在 `client/assets/resources/development/manifest.json`。帧映射仍为跨来源候选，不能称已通过目标1.80验收。三职业专属施法、音效与其余宠物/普通怪尚待补齐。

| 环节 | 输入与证据 | 生成及通过要求 |
| --- | --- | --- |
| 格式识别 | Crystal `WeMadeLibrary`、`MLibraryV0/V1/V2`、`WTLLibrary`及实存包头 | 识别WIL/WIX、WZL/WZX、Lib等具体版本；拒绝索引越界和未知结构 |
| 帧导出 | 图像包与配套索引 | PNG及逐帧元数据；保留原始帧号、占位帧、透明度、绘制偏移与尺寸，不自动居中 |
| 动作映射 | `Client/MirObjects/Frames.cs`、玩家/怪物表现代码与实际帧 | 八方向站立、移动、普攻、施法、受击、死亡；每方向起帧、数量、步幅、时长与空帧明确 |
| 分层外观 | Hum、Weapon、Hair候选包 | 男战士/女法师/男道士逐帧对齐；前后遮挡正确；外观不改变同itemID身份或属性 |
| 宠物与怪物 | Mon包、`Appr/Shape`候选数据和源码 | 明确包号/形象ID/动作，宠物攻击类型经行为核验，不默认一律远程 |
| 技能与音效 | Magic/Effect/MagIcon包、SoundList及施法表现代码 | 起手、弹道、命中、持续、Buff与声音事件独立映射，命中由服务端逻辑格决定 |
| 地图 | `.map`、Crystal `Map.cs/MapCode.cs`、Tiles/SmTiles/Objects | 识别尺寸和原格序，保留列优先语义、阻挡与图层；裁成16×16后验证可走区/出生/刷怪点 |
| Creator导入 | 已验证PNG/元数据/音效 | 可编辑.scene/.prefab；固定UI层级使用内置组件，原图和C#界面代码不等于Creator资产 |
| 回读验证 | 转换产物与原始参考 | 对照像素、偏移、方向、帧序、地图引用与声音；生成预览并人工/自动检查后标记通过 |

每个完整资产批次须有 `source_id/source_path/source_sha256/converter_version/frame_index/direction/start/count/stride/duration_ms/offset_x/offset_y/width/height/alpha/placeholder` 等明确元数据；未知项保留null并阻止正式发布。有限WIL样例的方向字段仍保持null；开发批次提供可播放候选映射，二者不混用。生成、检查与配置边界见[首循环候选说明](首循环资源与配置候选.md)。

## 技能核验流程

1. 从三职业候选逐项筛选目标版本；记录可靠出处、版本/提交号或摘要、函数和行号。Crystal与MYF均含后续/自制内容，不自动证明官方1.80；不把33或当前候选数量写成完整目标清单。
2. 固定学习条件、基础等级、目标过滤、射程/方向/命中格、碰撞、耗蓝/道具、前后摇、冷却、动作与命中时点、伤害/治疗、召唤/位移、Buff叠加与解除。
3. 基础规则与本作扩展分别记录。本作单宠、武器/翅膀幸运和获准的数值升级按最终策划执行；不得把这些差异混称原版完全还原。
4. 先建立可信预期，再执行 `docs/技能验收用例.json` 的适用用例。覆盖八方向近战/斜角、直线/扇形/地面持续、阻挡、耗蓝、宠物、Buff、健康重连、真实故障与升级前后。
5. 保存输入配置/版本、固定测试随机种子（仅测试环境）、服务端事件与结果、客户端录屏/帧号及差异。映射清单完整或脚本检查通过都不能代替技能对拍通过。

正式技能表只启用已经明确来源、参数和验收证据的条目；缺少信息时关闭相应功能入口，保留待核验状态，不猜造技能伤害公式或收费配置。完整P0仍需补齐最终三职业技能范围。

## 当前UI参考与发布边界

__UI_SOURCE_SUMMARY__用户补充登记项包含路径、角色、优先级、明确确认依据、原字节摘要与尺寸；生成器不修改任何图片或场景。__UI_NEXT_ACTION__

资料和候选索引是研发输入，不应随客户端投影下发源码、来源全文、服务端账本字段或历史经济结论。资源转换、逐技能对拍、全来源属性预算、经济复算、真机和容量均分别验收；本次资料准备不能替代这些门槛。
""".replace("__UI_COUNT__", str(ui["actual_image_count"])).replace("__UPSTREAM_UI_COUNT__", str(ui["upstream_image_count"])).replace("__USER_UI_COUNT__", str(ui["user_supplement_image_count"])).replace("__UI_SOURCE_SUMMARY__", ui_source_summary(ui)).replace("__UI_NEXT_ACTION__", ui_next_action(ui))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--full-hash", action="store_true", help="为全部允许文件完整计算SHA-256，不读取被排除内容")
    parser.add_argument("--check", action="store_true", help="在内存生成并比较，不写入文件")
    args = parser.parse_args()
    manifest = inventory(args.full_hash)
    skills = extract_skills(manifest)
    ui = ui_mapping(manifest)
    outputs = {
        OUTPUTS[0]: manifest, OUTPUTS[1]: skills, OUTPUTS[2]: ui,
        OUTPUTS[3]: overview(manifest, skills, ui), OUTPUTS[4]: acceptance_cases(skills), OUTPUTS[5]: conversion_doc(ui),
    }
    differing = []
    for relative, value in outputs.items():
        text = value if isinstance(value, str) else json.dumps(value, ensure_ascii=False, indent=2) + "\n"
        path = PROJECT / relative
        if args.check:
            if not path.exists() or path.read_text(encoding="utf-8") != text:
                differing.append(relative)
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text, encoding="utf-8")
    if differing:
        print(json.dumps({"status": "stale_or_missing", "files": differing}, ensure_ascii=False))
        raise SystemExit(1)
    print(json.dumps({"status": "checked" if args.check else "generated", "indexed_files": len(manifest["files"]),
                      "candidate_skills": skills["candidate_count"], "ui_images": ui["actual_image_count"],
                      "upstream_ui_images": ui["upstream_image_count"], "user_supplement_ui_images": ui["user_supplement_image_count"],
                      "all_skills_status": "待核验", "all_acceptance_status": "待执行", "runtime_ready": False}, ensure_ascii=False))


if __name__ == "__main__":
    main()
