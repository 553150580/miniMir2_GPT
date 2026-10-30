"""只测私有证据reader/EOL/gzip边界；不执行阶段门禁、暂存或产品检查。"""
from pathlib import Path
import gzip
import copy
import importlib.util
import json
import tempfile
import unittest
from unittest.mock import patch

WORK = Path(__file__).resolve().parent


def module(name):
    spec = importlib.util.spec_from_file_location('stage23_private_' + name, WORK / (name + '.py'))
    value = importlib.util.module_from_spec(spec); spec.loader.exec_module(value)
    return value


FINAL, INDEX = module('finalize_evidence'), module('verify_index')


class EvidenceSafetyTests(unittest.TestCase):
    def successful_recorded_actions(self):
        # 真实已失败整run中的已成功片段；只作为本validator正例，不追认整GUI通过。
        path = WORK / 'isolated-gui/run-a885081a3a0f4af794f180c7db9334f3/browser/report.json'
        report = json.loads(path.read_text(encoding='utf-8'))
        self.assertFalse(report['passed'])
        return [r for r in report['actions'] if not r['cancelled']]

    def test_actual_index_blob_collect_is_bytes_default_is_digest(self):
        # 只读一个既有小源文件，不运行完整index扫描，不写Git对象或index。
        name = 'tools/client/prepare_pet_runtime_qa.py'
        oid = INDEX.git('rev-parse', ':' + name).strip()
        expected = INDEX.git('show', ':' + name)
        reader = INDEX.Blobs()
        try:
            self.assertEqual(reader.content(oid), INDEX.digest(expected))
            self.assertEqual(reader.content(oid, collect=True), expected)
            self.assertIsInstance(reader.content(oid, collect=True), bytes)
        finally:
            reader.close()

    def test_pathmap_identity_gzip_and_direct_index_preserve_original_crlf(self):
        original = b'first\r\nsecond\r\n'
        sha = INDEX.digest(original)
        for encoding in ['identity', 'gzip']:
            raw = gzip.compress(original, mtime=0) if encoding == 'gzip' else original
            paths = {'original.log': {'path': 'archive.log', 'encoding': encoding, 'raw_sha256': sha}}
            self.assertEqual(INDEX.read_archived_source('original.log', paths, {'original.log': sha}, lambda _: raw, lambda _: original), original)
            with self.assertRaises(ValueError):
                INDEX.read_archived_source('original.log', paths, {'original.log': sha}, lambda _: raw, lambda _: original.replace(b'\r\n', b'\n'))
            with self.assertRaises(ValueError):
                INDEX.read_archived_source('original.log', paths, {'original.log': sha}, lambda _: sha)
            with self.assertRaises(ValueError):
                INDEX.read_archived_source('original.log', ['font1', 'font2', 'font3'], {'original.log': sha}, lambda _: raw)

    def test_archive_applies_only_to_patched_temporary_target_and_roundtrips_raw(self):
        formal_before = FINAL.FORMAL.read_bytes() if FINAL.FORMAL.exists() else None
        with tempfile.TemporaryDirectory(dir=WORK) as temporary:
            folder = Path(temporary)
            source = folder / 'original.jsonl'
            raw = (b'{"source":"raw"}\r\n') * 10000
            source.write_bytes(raw)
            archive = FINAL.Archive()
            archive.add(source, 'current/original.jsonl', 'synthetic_private_roundtrip_only')
            with patch.object(FINAL, 'OUT', folder / 'archive-only'):
                rows = archive.apply()
                actual = (FINAL.OUT / rows[0]['target']).read_bytes()
            self.assertEqual(len(rows), 1)
            self.assertEqual(rows[0]['encoding'], 'gzip')
            self.assertEqual(gzip.decompress(actual), raw)
            self.assertEqual(rows[0]['raw_sha256'], FINAL.digest(raw))
            self.assertEqual(rows[0]['file_sha256'], FINAL.digest(actual))
            self.assertEqual(FINAL.FORMAL.read_bytes() if FINAL.FORMAL.exists() else None, formal_before)

    def test_unreviewed_gui_contract_cannot_pass_even_if_reports_absent(self):
        with patch.object(FINAL, 'GUI_CONTRACT_REVIEWED', False):
            with self.assertRaisesRegex(ValueError, 'PENDING'):
                FINAL.gui_evidence(WORK / 'never-created/report.json', 'normal', WORK / 'never-created/creator.json')

    def test_client_protocol_report_path_matches_actual_producer_argv_exactly(self):
        report_path = WORK / 'client/full-2/report.json'
        report = json.loads(report_path.read_text(encoding='utf-8'))
        expected = FINAL.expected_client_commands(report_path)
        for row in report['commands']:
            self.assertEqual(row['command'][2:], expected[row['name']])
        # 不把其它argv归一化；只有原producer的Path字符串形式构造这一个参数。
        self.assertEqual(expected['protocol-check'][-1], str(report_path.parent.relative_to(FINAL.ROOT) / 'protocol'))

    def test_actual_recorded_successful_fragments_pass_item_and_fresh_revision_gates(self):
        actions = self.successful_recorded_actions()
        self.assertGreaterEqual(len(actions), 2)
        for row in actions:
            FINAL.validate_item_action(row['request'], row['actionResult'], row['before'], row['after'])
            FINAL.validate_fresh_revision(row['request'], row['operationResult'], row['actionResult'], row['before'], row['after'])

    def test_withdraw_cannot_leave_same_uid_in_loaded_warehouse_and_bag(self):
        row = self.successful_recorded_actions()[-1]
        before = copy.deepcopy(row['after'])
        slot = row['request']['warehouseSlot']
        item = next(r['item'] for r in before['warehouse']['items'] if r['slot'] == slot)
        q = {**row['request'], 'action': 2, 'warehouseRevision': before['warehouse']['revision'], 'itemInstanceId': item['id'], 'itemVersion': item['version'], 'quantity': item['quantity']}
        moved = copy.deepcopy(item); moved['version'] = str(int(item['version']) + 1)
        after = copy.deepcopy(before); after['items'].append(moved)
        receipt = {**row['actionResult'], 'action': 2, 'changes': [{'slot': slot, 'before': item, 'after': None}]}
        with self.assertRaises(ValueError):
            FINAL.validate_item_action(q, receipt, before, after)
        # 正常全取必须真实查询页无该槽，不能只信receipt.after=null。
        after['warehouse']['items'] = [r for r in after['warehouse']['items'] if r['slot'] != slot]
        FINAL.validate_item_action(q, receipt, before, after)
        changed = copy.deepcopy(row['actionResult']); changed['roleRevision'] = str(int(row['operationResult']['revision']) - 1)
        with self.assertRaises(ValueError):
            FINAL.validate_fresh_revision(row['request'], row['operationResult'], changed, row['before'], row['after'])


if __name__ == '__main__':
    unittest.main()
