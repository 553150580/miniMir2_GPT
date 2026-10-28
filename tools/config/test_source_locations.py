"""迁移来源不得用错误副本替代已变更的原文件或越界读取。"""
import hashlib
import json
from pathlib import Path
import tempfile
import unittest

from source_locations import resolve_locked_source


class SourceLocationsTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        (self.root / '资料索引').mkdir()
        (self.root / 'copy').write_bytes(b'original')
        self.digest = hashlib.sha256(b'original').hexdigest()
        self.reference = dict(path='missing', sha256=self.digest)
        self.record = dict(target_relative_path='copy', source_sha256=self.digest, target_sha256=self.digest)
        self.manifest()

    def manifest(self):
        (self.root / '资料索引/本地参考副本.json').write_text(json.dumps(dict(files=[self.record])), encoding='utf-8')

    def test_registered_original_bytes_are_reusable(self):
        self.assertEqual(resolve_locked_source(self.root, self.reference), self.root / 'copy')

    def test_changed_original_is_not_hidden_by_old_copy(self):
        (self.root / 'missing').write_bytes(b'changed')
        self.assertEqual(resolve_locked_source(self.root, self.reference), self.root / 'missing')

    def test_changed_copy_and_false_digest_are_rejected(self):
        (self.root / 'copy').write_bytes(b'changed')
        self.assertEqual(resolve_locked_source(self.root, self.reference), self.root / 'missing')
        (self.root / 'copy').write_bytes(b'original')
        self.record['source_sha256'] = '0' * 64
        self.manifest()
        self.assertEqual(resolve_locked_source(self.root, self.reference), self.root / 'missing')

    def test_copy_manifest_cannot_escape_project(self):
        self.record['target_relative_path'] = '../outside'
        self.manifest()
        self.assertEqual(resolve_locked_source(self.root, self.reference), self.root / 'missing')
