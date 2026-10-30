"""在隔离协议目录验证清单和双端编号生成物一致，检查模式不得改写文件。"""
import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import generate_protocol as generator


class ProtocolConstantTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        scope = patch.object(generator, 'ROOT', self.root)
        scope.start()
        self.addCleanup(scope.stop)
        protocol = self.root / 'shared/proto'
        protocol.mkdir(parents=True)
        (protocol / 'sample.proto').write_text('syntax = "proto3";\nmessage Sample { uint64 value = 1; }\n',
                                             encoding='utf-8')
        self.manifest_path = protocol / 'protocol_manifest.json'
        self.manifest = {'messages': [{'name': 'MsgC2SSample', 'value': 1, 'proto_message': 'Sample'}],
                         'error_codes': [{'name': 'CodeSample', 'value': 3001}]}
        self.save_manifest()
        self.run_generator([])
        self.outputs = [self.root / 'server/internal/protocol/protocol_ids.go',
                        self.root / 'client/assets/scripts/generated/protocol_ids.ts']

    def save_manifest(self):
        self.manifest_path.write_text(json.dumps(self.manifest), encoding='utf-8')

    def run_generator(self, arguments):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            generator.main(arguments)
        return json.loads(output.getvalue())

    def snapshot(self):
        return {p.relative_to(self.root).as_posix(): p.read_bytes() for p in self.root.rglob('*') if p.is_file()}

    def test_check_accepts_generated_constants_without_writing(self):
        before = self.snapshot()
        self.assertTrue(self.run_generator(['--check'])['valid'])
        self.assertEqual(self.snapshot(), before)

    def test_manifest_message_and_error_code_changes_require_regeneration(self):
        for family in ('messages', 'error_codes'):
            with self.subTest(family=family):
                self.manifest[family][0]['value'] += 1
                self.save_manifest()
                before = self.snapshot()
                with self.assertRaisesRegex(ValueError, '生成协议常量与清单不一致'):
                    self.run_generator(['--check'])
                self.assertEqual(self.snapshot(), before)
                self.run_generator([])
                self.assertTrue(self.run_generator(['--check'])['valid'])

    def test_missing_or_modified_constant_file_on_each_side_is_rejected(self):
        for output in self.outputs:
            original = output.read_bytes()
            for failure in ('modified', 'missing'):
                with self.subTest(side=output.suffix, failure=failure):
                    try:
                        if failure == 'modified':
                            output.write_bytes(original.replace(b' = 1', b' = 9'))
                        else:
                            output.unlink()
                        before = self.snapshot()
                        with self.assertRaisesRegex(ValueError, '生成协议常量与清单不一致'):
                            self.run_generator(['--check'])
                        self.assertEqual(self.snapshot(), before)
                    finally:
                        output.write_bytes(original)

    def test_go_pb_record_does_not_replace_constant_validation(self):
        source = self.root / 'shared/proto/sample.proto'
        record = {'sources': {'sample.proto': generator.hashlib.sha256(source.read_bytes()).hexdigest()},
                  'outputs': {}}
        generation = self.root / 'conf/toolchain/go-pb-generation.json'
        generation.parent.mkdir(parents=True)
        generation.write_text(json.dumps(record), encoding='utf-8')
        self.assertEqual(self.run_generator(['--check'])['protoc_compile'], 'verified_generated_sources_current')
        self.manifest['messages'][0]['value'] = 60000
        self.save_manifest()
        with self.assertRaisesRegex(ValueError, '生成协议常量与清单不一致'):
            self.run_generator(['--check'])


if __name__ == '__main__':
    unittest.main()
