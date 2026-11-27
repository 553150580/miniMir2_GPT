"""固定构建工作槽的复用、换代与证据路径保护测试。"""

from pathlib import Path
import tempfile
import types
import unittest
from unittest.mock import patch

import creator_workspace as workflow
import version_preview_scene_pack as preview
from creator_work_paths import creator_slot_lock


class CreatorWorkspaceTests(unittest.TestCase):
    def setUp(self):
        self.root = Path(self.enterContext(tempfile.TemporaryDirectory(dir=workflow.ROOT / '.work')))
        self.source = self.root / 'client'
        for name in workflow.creator.SELECTED:
            entry = self.source / name
            if name.endswith('.json'):
                entry.parent.mkdir(parents=True, exist_ok=True)
                entry.write_text('{}', encoding='utf-8')
            else:
                entry.mkdir(parents=True)
        workflow.creator.save(self.source / workflow.SOURCE_PUBLIC, {'rule_version': 30})
        (self.source / workflow.PUBLIC_META).write_bytes(b'source-public-meta')
        workflow.creator.save(self.root / workflow.FIXTURE, {'rule_version': 30})
        workflow.creator.save(self.root / 'conf/creator-build.json', {
            'platform': 'web-mobile', 'buildPath': 'project://build', 'outputName': 'web-mobile',
        })
        self.scene = self.source / 'assets/Game.scene'
        self.scene.write_bytes(b'Original Scene')
        self.existing_live = self.source / 'build/web-mobile/index.html'
        self.existing_live.parent.mkdir(parents=True)
        self.existing_live.write_bytes(b'released-package')
        fake_executable = self.root / 'Creator.exe'
        fake_executable.touch()
        self.enterContext(patch.object(workflow.creator, 'ROOT', self.root))
        self.enterContext(patch.dict(workflow.creator.os.environ, {'MINIMIR_CREATOR_EXECUTABLE': str(fake_executable)}))
        self.built = self.enterContext(patch.object(workflow.creator.subprocess, 'run', side_effect=self.fake_build))

    def fake_build(self, command, **options):
        """模拟成功的构建命令和可清理的生成目录。"""
        project = Path(command[command.index('--project') + 1])
        library = project / 'library'
        library.mkdir(exist_ok=True)
        (library / 'reused-data').touch()
        output = project / workflow.WEB_MOBILE
        output.mkdir(parents=True)
        (output / 'index.html').write_bytes((project / 'assets/Game.scene').read_bytes())
        options['stdout'].write(b'test Creator report')
        return types.SimpleNamespace(returncode=36)

    def test_identical_build_is_reused_and_changed_build_overwrites_one_slot(self):
        first = workflow.build_slot(self.root, 'default')
        slot = Path(first['workspace'])
        web = Path(first['web'])
        imported = slot / 'client-project/library/reused-data'
        imported.write_bytes(b'do not delete')
        self.assertFalse(workflow.build_slot(self.root, 'default')['creator_rerun'])
        self.assertEqual(self.built.call_count, 1)
        (slot / 'preview-web-mobile').mkdir()
        (slot / 'preview-web-mobile/stale.html').write_bytes(b'old')
        self.scene.write_bytes(b'Revised Scene')
        second = workflow.build_slot(self.root, 'default')
        self.assertTrue(second['creator_rerun'])
        self.assertEqual(second['workspace'], first['workspace'])
        self.assertEqual((web / 'index.html').read_bytes(), b'Revised Scene')
        self.assertEqual(imported.read_bytes(), b'do not delete')
        self.assertFalse((slot / 'preview-web-mobile').exists())
        self.assertEqual(self.existing_live.read_bytes(), b'released-package')
        self.assertEqual(len(list(slot.parent.iterdir())), 1)

    def test_candidate_and_frozen_evidence_are_independent(self):
        fixture = self.root / '.work/candidate.json'
        public = self.root / '.work/candidate-public.json'
        workflow.creator.save(fixture, {'rule_version': 31})
        workflow.creator.save(public, {'rule_version': 31, 'test_feature': True})
        active = workflow.build_slot(self.root, 'candidate', fixture=fixture, public=public, freeze='m4-candidate')
        frozen = Path(active['frozen_evidence'])
        project = frozen / 'client-project'
        browser_input = project / workflow.SOURCE_PUBLIC
        self.assertTrue(workflow.read(browser_input)['test_feature'])
        self.assertFalse((project / 'library').exists())
        self.assertEqual(workflow.read(frozen / 'creator-report.json')['project'], str(project))
        self.scene.write_bytes(b'New Candidate')
        workflow.build_slot(self.root, 'candidate', fixture=fixture, public=public)
        self.assertEqual((project / workflow.WEB_MOBILE / 'index.html').read_bytes(), b'Original Scene')
        self.assertEqual(workflow.read(self.source / workflow.SOURCE_PUBLIC), {'rule_version': 30})

    def test_existing_slot_is_locked_and_path_escape_is_rejected(self):
        first = workflow.build_slot(self.root, 'default')
        slot = Path(first['workspace'])
        with creator_slot_lock(slot), self.assertRaises(RuntimeError):
            workflow.build_slot(self.root, 'default')
        with self.assertRaises(ValueError):
            workflow.build_slot(self.root, '../outside')
        self.assertEqual(self.built.call_count, 1)

    def test_compatibility_tool_accepts_new_slot_and_keeps_legacy_read_only(self):
        legacy = self.root / 'legacy'
        with patch.object(preview, 'local_creator_work', return_value=legacy):
            active = self.root / '.work/creator-work/default'
            frozen = self.root / '.work/creator-evidence/m4-candidate'
            self.assertEqual(preview.preview_directory(active, self.root), (active, False))
            self.assertEqual(preview.preview_directory(frozen, self.root), (frozen, False))
            self.assertEqual(preview.preview_directory(legacy / 'old', self.root), (legacy / 'old', True))
            with self.assertRaises(ValueError):
                preview.materialize(legacy / 'old', self.root / 'unused', verify_only=True)
            with self.assertRaises(ValueError):
                preview.preview_directory(self.source / 'build/web-mobile', self.root)


if __name__ == '__main__':
    unittest.main()
