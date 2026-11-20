"""恢复验收的配置一致性与清理回归，不启动节点或连接Mongo。"""

import hashlib
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import Mock, patch

import verify_startup_recovery as recovery


class RecoveryToolTests(unittest.TestCase):
    def test_selected_fixture_identity_is_frozen_from_the_same_bytes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config_path = root / 'conf/dev/recovery.local.json'
            fixture_path = root / 'selected.json'
            content = b'{"rule_version": 23}\n'
            fixture_path.write_bytes(content)
            expected = {'path': str(fixture_path), 'fixture_hash': hashlib.sha256(content).hexdigest(),
                        'rule_version': 23}
            for name in ['selected.json', str(fixture_path)]:
                with self.subTest(path=name):
                    self.assertEqual(recovery.fixture_identity({'fixture_file': name}, config_path), expected)
            report = {'source_before': {}, 'source_after': {}, 'expected_fixture': expected}
            self.assertTrue(recovery.sources_unchanged(report))
            fixture_path.write_bytes(b'{"rule_version": 23, "changed": true}\n')
            self.assertFalse(recovery.sources_unchanged(report))

    def test_ready_and_restart_validate_each_node_against_frozen_fixture(self):
        modes = ['account', 'game', 'gm']
        config = {f'{mode}_address': f'127.0.0.1:{18110 + index}' for index, mode in enumerate(modes)}
        expected = {'fixture_hash': 'a' * 64, 'rule_version': 23}
        responses = {config[f'{mode}_address']: {'status': 'ok', 'mode': mode, **expected} for mode in modes}
        with patch.object(recovery, 'http_json', side_effect=responses.__getitem__):
            self.assertEqual(set(recovery.health_ready(config, [], expected)), set(modes))
            for mode in modes:
                address = config[f'{mode}_address']
                for field, mismatched, message in [('fixture_hash', 'b' * 64, '配置摘要'),
                                                   ('rule_version', 24, '规则版本'),
                                                   ('rule_version', 23.0, '规则版本')]:
                    with self.subTest(mode=mode, field=field, value=mismatched):
                        responses[address][field] = mismatched
                        try:
                            with self.assertRaisesRegex(RuntimeError, mode + '的' + message):
                                recovery.health_ready(config, [], expected)
                        finally:
                            responses[address][field] = expected[field]
            self.assertEqual(set(recovery.health_ready(config, [], expected)), set(modes))

    def test_probe_exit_timeout_still_uses_owned_cleanup(self):
        child = Mock()
        child.poll.return_value = None
        child.wait.side_effect = subprocess.TimeoutExpired('probe', 5)
        handle = {'child': child}
        with patch.object(recovery, 'stop_owned') as stop:
            self.assertEqual(recovery.cleanup_owned([handle]), [])
        child.stdin.close.assert_called_once_with()
        child.wait.assert_called_once_with(timeout=5)
        stop.assert_called_once_with(handle)

    def test_probe_exit_timeout_preserves_cleanup_ownership_error(self):
        child = Mock()
        child.poll.return_value = None
        child.wait.side_effect = subprocess.TimeoutExpired('probe', 5)
        handle = {'child': child}
        with patch.object(recovery, 'stop_owned', side_effect=RuntimeError('进程身份变化')):
            self.assertEqual(recovery.cleanup_owned([handle]), ['进程身份变化'])


if __name__ == '__main__':
    unittest.main()
