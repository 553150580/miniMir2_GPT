"""在临时工作簿和发布目录验证损坏版本拒绝切主，不接触当前预览。"""
import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import export_configs as exporter


class ReleaseIntegrityTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        scope = patch.object(exporter, 'ROOT', self.root)
        scope.start()
        self.addCleanup(scope.stop)
        (self.root / 'config/schema').mkdir(parents=True)
        self.releases = self.root / 'config/releases'
        self.releases.mkdir()
        fields = [
            {'name': 'id', 'type': 'int', 'required': True, 'target': 'both'},
            {'name': 'value', 'type': 'string', 'required': True, 'target': 'both'},
            {'name': 'record_status', 'type': 'string', 'required': True, 'target': 'none'},
            {'name': 'source_ref', 'type': 'string', 'required': True, 'target': 'none'},
        ]
        schema = {'schema_version': 1, 'source_locks': [], 'blockers': [],
                  'workbooks': [{'group': 'gameplay', 'path': 'book.xlsx'}],
                  'tables': [{'table_id': 'sample', 'sheet_name': '样例', 'workbook_group': 'gameplay',
                              'release_required': True, 'fields': fields}]}
        (self.root / 'config/schema/catalog.json').write_text(json.dumps(schema), encoding='utf-8')
        headers = ''.join(f'<c r="{chr(65+i)}2" t="inlineStr"><is><t>{field["name"]}</t></is></c>'
                          for i, field in enumerate(fields))
        ns = exporter.NS['m']
        with zipfile.ZipFile(self.root / 'book.xlsx', 'w') as book:
            book.writestr('xl/workbook.xml', f'<workbook xmlns="{ns}" '
                          'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
                          '<sheets><sheet name="样例" r:id="rId1"/>'
                          '<sheet name="发布缺口" r:id="rId2"/></sheets></workbook>')
            book.writestr('xl/_rels/workbook.xml.rels', '<Relationships>'
                          '<Relationship Id="rId1" Target="worksheets/sheet1.xml"/>'
                          '<Relationship Id="rId2" Target="worksheets/sheet2.xml"/></Relationships>')
            book.writestr('xl/worksheets/sheet1.xml', f'<worksheet xmlns="{ns}"><sheetData>'
                          f'<row r="2">{headers}</row><row r="4"><c r="A4"><v>1</v></c>'
                          '<c r="B4" t="inlineStr"><is><t>测试内容</t></is></c>'
                          '<c r="C4" t="inlineStr"><is><t>confirmed</t></is></c>'
                          '<c r="D4" t="inlineStr"><is><t>隔离测试</t></is></c>'
                          '</row></sheetData></worksheet>')
            book.writestr('xl/worksheets/sheet2.xml', f'<worksheet xmlns="{ns}"><sheetData/></worksheet>')

    def export(self, version, mode='preview'):
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return exporter.main(['--version', version, '--mode', mode])

    def prepare_old_pointer_and_candidate(self, mode):
        self.assertEqual(self.export(f'{mode}-stable', mode), 0)
        pointer = self.releases / f'{mode}_current.json'
        stable = pointer.read_bytes()
        self.assertEqual(self.export(f'{mode}-candidate', mode), 0)
        candidate = self.releases / json.loads(pointer.read_text(encoding='utf-8'))['path']
        self.assertEqual(self.export(f'{mode}-stable', mode), 0)
        self.assertEqual(pointer.read_bytes(), stable)
        return pointer, stable, candidate

    def test_missing_or_modified_artifact_keeps_previous_pointer_in_both_modes(self):
        for mode in ('preview', 'release'):
            pointer, stable, candidate = self.prepare_old_pointer_and_candidate(mode)
            manifest = json.loads((candidate / 'manifest.json').read_text(encoding='utf-8'))
            for name in manifest['files']:
                artifact = candidate / name
                original = artifact.read_bytes()
                for failure in ('modified', 'missing'):
                    with self.subTest(mode=mode, name=name, failure=failure):
                        try:
                            if failure == 'modified':
                                artifact.write_bytes(b'corrupted')
                            else:
                                artifact.unlink()
                            self.assertEqual(self.export(f'{mode}-candidate', mode), 2)
                            self.assertEqual(pointer.read_bytes(), stable)
                            self.assertEqual(artifact.read_bytes() if artifact.exists() else None,
                                             b'corrupted' if failure == 'modified' else None)
                            self.assertEqual(list(self.releases.glob('.staging-*')), [])
                        finally:
                            artifact.write_bytes(original)

    def test_intact_existing_release_can_switch_without_rewriting_artifacts(self):
        pointer, _, candidate = self.prepare_old_pointer_and_candidate('preview')
        before = {p.name: (p.read_bytes(), p.stat().st_mtime_ns) for p in candidate.iterdir()}
        self.assertEqual(self.export('preview-candidate'), 0)
        self.assertEqual(json.loads(pointer.read_text(encoding='utf-8'))['path'], candidate.name)
        self.assertEqual({p.name: (p.read_bytes(), p.stat().st_mtime_ns) for p in candidate.iterdir()}, before)

    def test_manifest_drift_keeps_previous_pointer(self):
        pointer, stable, candidate = self.prepare_old_pointer_and_candidate('preview')
        manifest_path = candidate / 'manifest.json'
        manifest = json.loads(manifest_path.read_text(encoding='utf-8'))
        manifest['files']['server.json'] = '0' * 64
        manifest_path.write_text(json.dumps(manifest), encoding='utf-8')
        self.assertEqual(self.export('preview-candidate'), 2)
        self.assertEqual(pointer.read_bytes(), stable)


if __name__ == '__main__':
    unittest.main()
