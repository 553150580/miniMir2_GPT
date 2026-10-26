"""针对配置输入错误、裁剪和正式发布门槛的回归检查。"""
from __future__ import annotations
import copy
import json
from pathlib import Path
import shutil
import tempfile
import unittest
import xml.etree.ElementTree as ET
import zipfile

import export_configs as exporter


class ConfigurationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root = exporter.ROOT
        cls.schema = exporter.load_json(cls.root / "config/schema/catalog.json")
        cls.data, cls.errors, cls.release_issues, _ = exporter.collect(cls.schema)

    def test_actual_workbooks_have_valid_structure(self):
        self.assertEqual(self.errors, [])
        self.assertEqual(exporter.semantic_checks(self.data), [])
        self.assertEqual(exporter.check_json_contracts(self.schema, self.data), [])
        self.assertGreater(len(self.data), 30)

    def test_unverified_values_and_implementation_block_release(self):
        self.assertTrue(self.release_issues)
        self.assertTrue(any("candidate" in x or "unverified" in x for x in self.release_issues))
        self.assertTrue(any("有效记录 0" in x for x in self.release_issues))

    def test_missing_is_not_zero_and_bool_is_not_amount(self):
        f = {"type": "int"}
        self.assertIsNone(exporter.parse_value(None, f))
        self.assertEqual(exporter.parse_value(0, f), 0)
        for invalid in (True, 0.1, "100", float("inf")):
            with self.assertRaises(ValueError):
                exporter.parse_value(invalid, f)

    def test_client_never_receives_server_or_source_columns(self):
        payload, _ = exporter.make_projection(self.schema, self.data, "client")
        for table in self.schema["tables"]:
            forbidden = {f["name"] for f in table["fields"] if f["target"] in ("server", "none")}
            for row in payload.get(table["table_id"], []):
                self.assertFalse(forbidden & row.keys())
                self.assertNotIn("source_ref", row)
                self.assertNotIn("note", row)
        self.assertNotIn("history_economy_inputs", payload)
        self.assertNotIn("game_server_rules", payload)

    def test_historical_records_are_excluded_on_both_sides(self):
        for target in ("server", "client"):
            _, states = exporter.make_projection(self.schema, self.data, target)
            self.assertTrue(all(v != "historical" for rows in states.values() for v in rows.values()))

    def test_probability_conservation_and_quote_formula(self):
        bad = copy.deepcopy(self.data)
        bad["game_drop_categories"][0]["prob_ppm"] -= 1
        self.assertTrue(any("概率合计" in x for x in exporter.semantic_checks(bad)))
        bad = copy.deepcopy(self.data)
        bad["game_level_band_economy"][0]["hosting_gold_per_hour"] += 1
        self.assertTrue(any("2倍" in x for x in exporter.semantic_checks(bad)))

    def test_unbound_yuanbao_mint_and_precision(self):
        bad = copy.deepcopy(self.data)
        for r in bad["game_currencies"]:
            if r["currency_key"] == "ingot":
                r["units_per_display"] = 100
                r["mint_sources"] = ["gm_grant"]
        messages = exporter.semantic_checks(bad)
        self.assertTrue(any("精度" in x for x in messages))
        self.assertTrue(any("唯一新增来源" in x for x in messages))

    def test_nested_json_cannot_bypass_confirmed_economy_rules(self):
        bad = copy.deepcopy(self.data)
        tax = next(r for r in bad['game_item_market_rules'] if r['rule_key']=='seller_commission_bp')
        tax['rule_value'] = 500
        self.assertTrue(any('已定规则' in x for x in exporter.check_json_contracts(self.schema,bad)))
        bad = copy.deepcopy(self.data)
        initial = next(r for r in bad['game_luck_initial'] if r.get('initial_value_weights'))
        initial['initial_value_weights'] = {'1':-10,'2':0,'3':1}
        self.assertTrue(exporter.check_json_contracts(self.schema,bad))

    def test_nested_integer_rules_reject_fractional_seconds(self):
        contract = {'type':'object','properties':{'seconds':{'type':'int','minimum':0}}}
        self.assertTrue(exporter.validate_json_shape({'seconds':1.5},contract,'purchase'))
        self.assertTrue(exporter.validate_json_shape({'seconds':-1},contract,'purchase'))
        self.assertEqual(exporter.validate_json_shape({'seconds':0},contract,'purchase'),[])

    def test_ore_price_uses_exact_rational_integers(self):
        copper=next(r for r in self.data['game_ore_types'] if r['ore_key']=='copper')
        self.assertEqual((copper['price_gold_numerator'],copper['price_gold_denominator']),(1,2))
        self.assertEqual(copper['integer_rounding_rule'],'floor_per_ore')
        self.assertEqual(copper['record_status'],'candidate')
        self.assertNotIn('gold_per_purity',copper)
        bad=copy.deepcopy(self.data)
        bad['game_ore_types'][0]['price_gold_denominator']=0
        self.assertTrue(any('整数分子' in e for e in exporter.semantic_checks(bad)))

    def test_excel_edit_is_read_and_duplicate_id_is_rejected(self):
        # 只在临时副本修改OpenXML，证明导出读取实际Excel，而不是读取初始种子。
        with tempfile.TemporaryDirectory() as directory:
            temp = Path(directory)
            for w in self.schema["workbooks"]:
                dest = temp / w["path"]
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(self.root / w["path"], dest)
            table = next(t for t in self.schema["tables"] if t["table_id"] == "game_currencies")
            w = next(w for w in self.schema["workbooks"] if w["group"] == table["workbook_group"])
            workbook = temp / w["path"]
            with zipfile.ZipFile(workbook) as z:
                entries = {n: z.read(n) for n in z.namelist()}
            wb = ET.fromstring(entries["xl/workbook.xml"])
            rels = ET.fromstring(entries["xl/_rels/workbook.xml.rels"])
            targets = {r.attrib["Id"]: r.attrib["Target"] for r in rels}
            sheet = next(s for s in wb.findall("m:sheets/m:sheet", exporter.NS) if s.attrib["name"] == table["sheet_name"])
            target = targets[sheet.attrib[exporter.RID]]
            target = target.lstrip("/") if target.startswith("/") else "xl/" + target
            tree = ET.fromstring(entries[target])
            c = next(c for c in tree.findall("m:sheetData/m:row/m:c", exporter.NS) if c.attrib["r"] == "A5")
            c.find("m:v", exporter.NS).text = "1"
            entries[target] = ET.tostring(tree, encoding="utf-8", xml_declaration=True)
            with zipfile.ZipFile(workbook, "w", zipfile.ZIP_DEFLATED) as z:
                for n, body in entries.items():
                    z.writestr(n, body)
            try:
                exporter.ROOT = temp
                _, errors, _, _ = exporter.collect(self.schema)
            finally:
                exporter.ROOT = self.root
            self.assertTrue(any("重复ID" in e for e in errors), errors)


if __name__ == "__main__":
    unittest.main()
