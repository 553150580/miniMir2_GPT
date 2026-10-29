#!/usr/bin/env python3
"""读取唯一 Excel 数据源，校验并生成带版本的服务端/客户端研发预览。仅依赖 Python 标准库。"""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import re
import shutil
import sys
import tempfile
import xml.etree.ElementTree as ET
import zipfile

ROOT = Path(__file__).resolve().parents[2]
NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
RID = "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"
STATUSES = {"confirmed", "candidate", "unverified", "historical"}


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def canonical(value) -> str:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"), allow_nan=False)


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def read_xlsx(path: Path) -> dict[str, dict[int, dict[int, object]]]:
    """只读取 OpenXML 单元格，不执行宏或外链。数据区公式必须先在表格引擎中计算。"""
    with zipfile.ZipFile(path) as z:
        shared = []
        if "xl/sharedStrings.xml" in z.namelist():
            root = ET.fromstring(z.read("xl/sharedStrings.xml"))
            shared = ["".join(x.itertext()) for x in root.findall("m:si", NS)]
        rels = ET.fromstring(z.read("xl/_rels/workbook.xml.rels"))
        targets = {x.attrib["Id"]: x.attrib["Target"] for x in rels}
        wb = ET.fromstring(z.read("xl/workbook.xml"))
        result = {}
        for s in wb.findall("m:sheets/m:sheet", NS):
            target = targets[s.attrib[RID]]
            if target.startswith("/"):
                target = target.lstrip("/")
            else:
                target = "xl/" + target
            tree = ET.fromstring(z.read(target))
            rows = {}
            for row in tree.findall("m:sheetData/m:row", NS):
                vals = {}
                for c in row.findall("m:c", NS):
                    address = c.attrib.get("r", "A1")
                    letters = re.match(r"[A-Z]+", address).group()
                    col = 0
                    for ch in letters:
                        col = col * 26 + ord(ch) - 64
                    value = c.find("m:v", NS)
                    raw = value.text if value is not None else None
                    typ = c.attrib.get("t", "n")
                    if c.find("m:f", NS) is not None and raw is None:
                        raise ValueError(f"{path.name}/{s.attrib['name']}!{address} 公式没有计算缓存，请先重算保存")
                    if typ == "s":
                        v = shared[int(raw)] if raw is not None else None
                    elif typ == "inlineStr":
                        v = "".join(c.find("m:is", NS).itertext())
                    elif typ == "b":
                        v = raw == "1"
                    elif typ in ("str", "e"):
                        if typ == "e":
                            raise ValueError(f"{s.attrib['name']}!{address} 包含公式错误：{raw}")
                        v = raw
                    elif raw is None:
                        v = None
                    else:
                        f = float(raw)
                        v = int(f) if f.is_integer() else f
                    if v is not None and v != "":
                        vals[col] = v
                if vals:
                    rows[int(row.attrib["r"])] = vals
            result[s.attrib["name"]] = rows
        return result


def parse_value(raw, field):
    if raw is None or raw == "":
        return None
    typ = field["type"]
    if typ == "string":
        if not isinstance(raw, str):
            raise ValueError("必须为文本")
        return raw
    if typ == "bool":
        if isinstance(raw, bool):
            return raw
        if raw in (0, 1):
            return bool(raw)
        if isinstance(raw, str) and raw.lower() in ("true", "false"):
            return raw.lower() == "true"
        raise ValueError("必须为 true/false 布尔值")
    if typ in ("int", "number"):
        if isinstance(raw, bool) or not isinstance(raw, (int, float)) or not math.isfinite(raw):
            raise ValueError("必须为有限数值，不能以文本代替")
        if typ == "int" and int(raw) != raw:
            raise ValueError("必须为整数")
        if typ == "int" and abs(raw) > 2**53 - 1:
            raise ValueError("超过 Excel 精确整数范围，永久ID请使用文本字段")
        return int(raw) if typ == "int" else raw
    if typ == "json":
        return json.loads(raw) if isinstance(raw, str) else raw
    raise ValueError(f"不支持字段类型 {typ}")


def validate_json_shape(value, shape, location):
    """嵌套JSON按初始领域契约检查类型/键，整数不会被JSON字段绕过。"""
    if "any_of" in shape:
        if not shape["any_of"]:
            return []
        if any(not validate_json_shape(value, option, location) for option in shape["any_of"]):
            return []
        return [f"{location} 不符合约定的数组元素类型"]
    typ = shape["type"]
    if typ == "unverified":
        return []
    valid = {
        "null": value is None,
        "boolean": isinstance(value, bool),
        "string": isinstance(value, str),
        "int": isinstance(value, int) and not isinstance(value, bool),
        "number": isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value),
        "array": isinstance(value, list),
        "object": isinstance(value, dict),
    }.get(typ, False)
    if not valid:
        return [f"{location} 必须符合JSON类型 {typ}"]
    errors = []
    if "minimum" in shape and isinstance(value, (int, float)) and value < shape["minimum"]:
        errors.append(f"{location} 不得低于 {shape['minimum']}")
    if "maximum" in shape and isinstance(value, (int, float)) and value > shape["maximum"]:
        errors.append(f"{location} 不得超过 {shape['maximum']}")
    if typ == "object":
        expected = shape["properties"]
        if set(value) != set(expected):
            errors.append(f"{location} 的JSON键与领域契约不一致")
        for key in set(value) & set(expected):
            errors.extend(validate_json_shape(value[key], expected[key], f"{location}.{key}"))
    elif typ == "array":
        for i, item in enumerate(value):
            errors.extend(validate_json_shape(item, shape["items"], f"{location}[{i}]"))
    return errors


def check_json_contracts(schema, data):
    errors = []
    by_key = {(tid, r["id"]): r for tid, rows in data.items() for r in rows}
    registered_rules = set()
    for c in schema.get("json_contracts", []):
        registered_rules.add((c["table_id"], c.get("rule_key")))
        row = by_key.get((c["table_id"], c["id"]))
        location = f"{c['table_id']}/{c['id']}.{c['field']}"
        if row is None:
            if c.get("confirmed_sha256"):
                errors.append(f"{location} 已定规则被删除，须先更新依据和领域契约")
            continue
        if c.get("rule_key") and row.get("rule_key") != c["rule_key"]:
            errors.append(f"{location} 不能复用既有ID更换规则键")
        value = row.get(c["field"])
        if value is None:
            if c.get("confirmed_sha256"):
                errors.append(f"{location} 已定规则不能留空")
            continue
        errors.extend(validate_json_shape(value, c["shape"], location))
        if c.get("confirmed_sha256"):
            actual = hashlib.sha256(canonical(value).encode()).hexdigest()
            if actual != c["confirmed_sha256"]:
                errors.append(f"{location} 已定规则与依据不符；变更须先更新策划、来源锁及领域契约")
    for tid, rows in data.items():
        for row in rows:
            if "rule_key" in row and row.get("record_status") != "historical" and (tid, row["rule_key"]) not in registered_rules:
                errors.append(f"{tid}/{row['id']} 新增通用规则必须先声明JSON领域契约")
    return errors


def collect(schema, workbook_override=None):
    errors, release_issues = [], []
    all_data, sheets_by_group, paths = {}, {}, {}
    if workbook_override and len(schema["workbooks"]) != 1:
        raise ValueError("多个工作簿时请在临时根目录测试，不允许模糊 --workbook 覆盖")
    for w in schema["workbooks"]:
        p = Path(workbook_override) if workbook_override else ROOT / w["path"]
        paths[w["group"]] = p
        sheets_by_group[w["group"]] = read_xlsx(p)
    for table in schema["tables"]:
        tid, fields = table["table_id"], table["fields"]
        sheet = sheets_by_group[table["workbook_group"]].get(table["sheet_name"])
        if sheet is None:
            errors.append(f"{tid}：缺少工作表 {table['sheet_name']}")
            continue
        expected = [f["name"] for f in fields]
        actual = [sheet.get(2, {}).get(i + 1) for i in range(len(fields))]
        extra = [v for k, v in sheet.get(2, {}).items() if k > len(fields)]
        if expected != actual or extra:
            errors.append(f"{tid}：第2行字段与schema不一致，不得自行删除/重排/新增字段")
            continue
        seen, rows = set(), []
        for rn, cells in sorted(sheet.items()):
            if rn < 4:
                continue
            if not any(cells.get(i + 1) is not None for i in range(len(fields))):
                continue
            row = {}
            for i, field in enumerate(fields):
                name = field["name"]
                try:
                    value = parse_value(cells.get(i + 1), field)
                    if value is not None and "enum_values" in field and value not in field["enum_values"]:
                        raise ValueError(f"枚举值不合法，允许 {field['enum_values']}")
                    if value is not None and "minimum" in field and value < field["minimum"]:
                        raise ValueError(f"低于下限 {field['minimum']}")
                    if value is not None and "maximum" in field and value > field["maximum"]:
                        raise ValueError(f"超过上限 {field['maximum']}")
                    row[name] = value
                    if value is None and field.get("required"):
                        release_issues.append(f"{tid} 第{rn}行缺少必填 {name}")
                        if name in ("id", "record_status", "source_ref"):
                            errors.append(f"{tid} 第{rn}行缺少元数据 {name}")
                except (ValueError, TypeError, json.JSONDecodeError) as e:
                    errors.append(f"{tid} 第{rn}行 {name}：{e}")
                    row[name] = None
            if row.get("id") in seen:
                errors.append(f"{tid}：重复ID {row.get('id')}")
            seen.add(row.get("id"))
            if row.get("record_status") not in STATUSES:
                errors.append(f"{tid} 第{rn}行缺少合法记录状态")
            if row.get("record_status") in ("candidate", "unverified") and table.get("release_required"):
                release_issues.append(f"{tid}/{row.get('id')} 尚为 {row.get('record_status')}")
            if row.get("record_status") == "historical" and table.get("release_required"):
                release_issues.append(f"{tid}/{row.get('id')} 为历史样本，不可进入正式运行配置")
            rows.append(row)
        active = [r for r in rows if r.get("record_status") != "historical"]
        if table.get("release_required") and len(active) < table.get("min_rows", 1):
            release_issues.append(f"{tid}：有效记录 {len(active)} 条，至少需要 {table.get('min_rows', 1)} 条")
        all_data[tid] = rows
    # 跨表引用使用实际 Excel 记录，不能用生成时的样本掩盖已删除引用。
    for t in schema["tables"]:
        for f in t["fields"]:
            ref = f.get("ref_table")
            if not ref:
                continue
            if ref not in all_data:
                errors.append(f"schema/{t['table_id']}.{f['name']} 引用不存在的表 {ref}")
                continue
            keys = {r["id"] for r in all_data[ref]}
            for r in all_data.get(t["table_id"], []):
                v = r.get(f["name"])
                if v is not None and v not in keys and not (v == 0 and f.get("allow_zero")):
                    errors.append(f"{t['table_id']}/{r['id']}.{f['name']} 引用缺失 {ref}/{v}")
    # 缺口登记是发布前置条件，解除缺口必须有证据；不能仅删掉缺口行。
    registered = {x["id"]: x for x in schema.get("blockers", [])}
    found = {}
    for group, sheets in sheets_by_group.items():
        if "发布缺口" not in sheets:
            errors.append(f"{group}：缺少发布缺口表")
            continue
        for rn, vals in sheets["发布缺口"].items():
            if rn < 4 or vals.get(1) is None:
                continue
            bid, status, evidence = str(vals.get(1)), vals.get(5), vals.get(6)
            found[bid] = (status, evidence)
            if status not in ("open", "resolved", "not_applicable"):
                errors.append(f"缺口 {bid} 状态不合法")
            if status != "open" and not evidence:
                errors.append(f"缺口 {bid} 关闭时必须填写证据/批准变更引用")
    for bid, b in registered.items():
        if b.get("blocking", True) and (bid not in found or found[bid][0] == "open"):
            release_issues.append(f"{bid}：{b['description']}")
    return all_data, errors, release_issues, paths


def semantic_checks(data):
    """按明确的业务字段检查通用概率和精度；未知模型留给对应核验用例。"""
    errors = []
    for tid, rows in data.items():
        for row in rows:
            if row.get("record_status") == "historical":
                continue
            for key, v in row.items():
                if key.endswith("_bp") and any(x in key for x in ("chance", "prob", "success", "contribution", "resist")) and v is not None and isinstance(v, (int, float)):
                    if not 0 <= v <= 10000:
                        errors.append(f"{tid}/{row['id']}.{key} 超出万分比范围0～10000")
                if key.endswith("_ppm") and v is not None and isinstance(v, (int, float)):
                    if not 0 <= v <= 1000000:
                        errors.append(f"{tid}/{row['id']}.{key} 超出百万分率范围0～1000000")
                if any(key.endswith(s) for s in ("_yuanbao_units", "_gold", "_seconds")) and isinstance(v, (int, float)):
                    if int(v) != v or v < 0:
                        errors.append(f"{tid}/{row['id']}.{key} 必须为非负整数")
    # 互斥掉落主分类必须完整并且概率恰好守恒；未填内容表由正式门禁负责。
    drops = data.get("game_drop_categories", [])
    if drops:
        expected = {"empty", "gold", "item", "equipment"}
        if {r.get("category_key") for r in drops} != expected or len(drops) != 4:
            errors.append("game_drop_categories 必须且只能包含空/金币/道具/装备四类")
        if any(r.get("prob_ppm") is None for r in drops) or sum(r.get("prob_ppm") or 0 for r in drops) != 1000000:
            errors.append("game_drop_categories 互斥分类概率合计必须为1000000ppm")
    for r in data.get("game_level_band_economy", []):
        g, price = r.get("g_ref_gold_per_hour"), r.get("hosting_gold_per_hour")
        if g is not None and price is not None and price != 2*g:
            errors.append(f"game_level_band_economy/{r['id']} 托管单价必须为标准毛金币的2倍")
    currencies = data.get("game_currencies", [])
    for r in currencies:
        key = r.get("currency_key")
        expected_scale = 10 if key in ("ingot", "bound_ingot") else 1
        if r.get("units_per_display") != expected_scale:
            errors.append(f"game_currencies/{r['id']} 金额精度与最终策划不符")
        if key == "ingot" and r.get("mint_sources") != ["verified_recharge_only"]:
            errors.append("非绑定元宝唯一新增来源必须为已核验充值")
    ores = data.get("game_ore_types", [])
    for r in ores:
        num, den = r.get("price_gold_numerator"), r.get("price_gold_denominator")
        if not isinstance(num, int) or not isinstance(den, int) or num < 0 or den <= 0:
            errors.append(f"game_ore_types/{r['id']} 回收单价必须为非负整数分子与正整数分母")
    if ores:
        for layer in (1, 2):
            if sum(r.get(f"layer_{layer}_weight") or 0 for r in ores) != 100:
                errors.append(f"game_ore_types 第{layer}层矿种权重合计必须为100")
    return errors


def make_projection(schema, data, target):
    result = {}
    states = {}
    for table in schema["tables"]:
        tid = table["table_id"]
        fields = [f["name"] for f in table["fields"] if f["target"] in (target, "both")]
        if not fields:
            continue
        rows = [r for r in data.get(tid, []) if r.get("record_status") != "historical"]
        result[tid] = [{f: r.get(f) for f in fields} for r in rows]
        states[tid] = {str(r["id"]): r["record_status"] for r in rows}
    return result, states


def generate_bindings(schema):
    go_types = {"int": "int64", "number": "float64", "bool": "bool", "string": "string", "json": "json.RawMessage"}
    ts_types = {"int": "number", "number": "number", "bool": "boolean", "string": "string", "json": "unknown"}
    go = ['// 由配置schema生成。字段使用指针区分缺失值与0。', 'package configtypes', '', 'import "encoding/json"', '', 'var _ json.RawMessage', '']
    ts = ['// 由配置schema生成。研发预览允许null，正式发布须通过完整校验。', '']
    def pascal(s):
        return "".join(x[:1].upper() + x[1:] for x in s.split("_"))
    for table in schema["tables"]:
        name = pascal(table["table_id"])
        sf = [f for f in table["fields"] if f["target"] in ("server", "both")]
        cf = [f for f in table["fields"] if f["target"] in ("client", "both")]
        if sf:
            go.append(f"type {name} struct {{")
            for f in sf:
                typ = go_types[f["type"]]
                if f["type"] != "json":
                    typ = "*" + typ
                go.append(f'    {pascal(f["name"])} {typ} `json:"{f["name"]}"`')
            go.extend(['}', ''])
        if cf:
            ts.append(f"export interface {name} {{")
            for f in cf:
                ts.append(f'  {f["name"]}: {ts_types[f["type"]]} | null;')
            ts.extend(['}', ''])
    return "\n".join(go), "\n".join(ts)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true", help="只校验，不生成配置")
    parser.add_argument("--mode", choices=("preview", "release"), default="preview")
    parser.add_argument("--version", default="dev-prep-v0.1")
    parser.add_argument("--schema", type=Path, default=ROOT / "config/schema/catalog.json")
    parser.add_argument("--report", type=Path, default=ROOT / "config/reports/配置校验报告.json")
    args = parser.parse_args(argv)
    try:
        schema = load_json(args.schema)
        data, errors, release_issues, paths = collect(schema)
        errors += semantic_checks(data)
        errors += check_json_contracts(schema, data)
        release_issues.extend(f"{c['table_id']}/{c['id']}.{c['field']} 的JSON领域结构仍待核验" for c in schema.get("json_contracts", []) if c["shape"].get("type") == "unverified")
        from source_locations import resolve_locked_source
        for ref in schema["source_locks"]:
            p = resolve_locked_source(ROOT, ref)
            if not p.exists() or sha256(p) != ref["sha256"]:
                errors.append(f"依据文件发生变化：{ref['path']}，须先重新对齐schema和配置，再更新锁")
        if not re.fullmatch(r"[A-Za-z0-9._-]{1,80}", args.version):
            errors.append("版本号只能使用1～80位英文字母、数字、点、下划线和连字符")
        errors = list(dict.fromkeys(errors))
        release_issues = list(dict.fromkeys(release_issues))
        report = {
            "schema_version": schema["schema_version"], "mode": args.mode,
            "structure_valid": not errors, "runtime_ready": not errors and not release_issues,
            "table_count": len(schema["tables"]), "record_count": sum(map(len, data.values())),
            "status_counts": {s: sum(r.get("record_status") == s for rows in data.values() for r in rows) for s in sorted(STATUSES)},
            "errors": errors, "release_blockers": release_issues,
            "workbook_hashes": {g: sha256(p) for g, p in paths.items()},
        }
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
        if errors or (args.mode == "release" and release_issues):
            print(json.dumps({"structure_valid": not errors, "runtime_ready": False, "error_count": len(errors), "release_blocker_count": len(release_issues), "report": str(args.report)}, ensure_ascii=False))
            return 2
        if args.check:
            print(json.dumps({k: report[k] for k in ("structure_valid", "runtime_ready", "table_count", "record_count", "status_counts")}, ensure_ascii=False))
            return 0
        payloads, all_states = {}, {}
        for target in ("server", "client"):
            tables, states = make_projection(schema, data, target)
            payloads[target] = {"runtime_ready": report["runtime_ready"], "tables": tables}
            all_states[target] = states
        digest = hashlib.sha256(canonical({"payloads": payloads, "states": all_states, "schema_sha256": sha256(args.schema)}).encode()).hexdigest()
        release_name = f"{args.version}-{digest[:12]}"
        for payload in payloads.values():
            payload["config_version"] = release_name
        dest = ROOT / "config/releases" / release_name
        temp = Path(tempfile.mkdtemp(prefix=".staging-", dir=dest.parent))
        try:
            files = {}
            for target, payload in payloads.items():
                p = temp / f"{target}.json"
                p.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
                files[p.name] = sha256(p)
            go, ts = generate_bindings(schema)
            (temp / "config_types.go").write_text(go + "\n", encoding="utf-8", newline="\n")
            (temp / "config_types.ts").write_text(ts + "\n", encoding="utf-8", newline="\n")
            for name in ("config_types.go", "config_types.ts"):
                files[name] = sha256(temp / name)
            manifest = {"version": release_name, "requested_version": args.version, "content_sha256": digest, "runtime_ready": report["runtime_ready"], "mode": args.mode, "files": files, "source_locks": schema["source_locks"], "workbooks": report["workbook_hashes"], "schema_sha256": sha256(args.schema), "record_states": all_states}
            (temp / "manifest.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
            if dest.exists():
                old = load_json(dest / "manifest.json")
                if old != manifest:
                    raise ValueError("相同内容版本已存在但来源/清单不同，请使用新版本号")
            else:
                os.replace(temp, dest)
            pointer = dest.parent / f"{args.mode}_current.json"
            pointer_tmp = pointer.with_suffix(".tmp")
            pointer_tmp.write_text(json.dumps({"path": release_name, "version": release_name, "content_sha256": digest, "runtime_ready": report["runtime_ready"]}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8", newline="\n")
            os.replace(pointer_tmp, pointer)
        finally:
            if temp.exists():
                shutil.rmtree(temp)
        print(json.dumps({"output": str(dest), "runtime_ready": report["runtime_ready"], "release_blocker_count": len(release_issues)}, ensure_ascii=False))
        return 0
    except (OSError, ValueError, KeyError, zipfile.BadZipFile, ET.ParseError) as e:
        print(f"配置处理失败：{e}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
