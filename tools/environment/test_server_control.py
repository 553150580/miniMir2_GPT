"""核对开发服务停止命令传播本轮结清结果。"""
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from server import dev


class ServerStopTest(unittest.TestCase):
    def test_stop_uses_current_supervisor_result(self):
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / 'server.log'
            meta = {'log': str(log), 'run_id': '本轮', 'pid': 100, 'project_root': str(dev.ROOT)}
            status = {'running': False}
            current = {'msg': '装配停止结果', 'mode': 'supervisor', 'run_id': '本轮'}
            outcomes = (
                (dict(current, ok=True), None),
                (dict(current, ok=False, error='game节点退出'), 'game节点退出'),
                (dict(current, ok=True, run_id='旧轮次'), '缺少本轮停服结果'),
            )
            for row, error in outcomes:
                with self.subTest(row=row):
                    log.write_text('旧日志\n' + json.dumps(row, ensure_ascii=False) + '\n', encoding='utf-8')
                    with (
                        patch.object(dev, 'read', return_value=meta),
                        patch.object(dev, 'owned', side_effect=(True, False)),
                        patch.object(dev, 'windows_stop'),
                        patch.object(dev.os, 'kill'),
                        patch.object(dev, 'status', return_value=status),
                    ):
                        if error:
                            with self.assertRaisesRegex(RuntimeError, error):
                                dev.stop()
                        else:
                            self.assertEqual(dev.stop(), status)

    def test_stop_retains_failure_after_supervisor_exits(self):
        with tempfile.TemporaryDirectory() as directory:
            log = Path(directory) / 'server.log'
            meta = {'log': str(log), 'run_id': '本轮', 'pid': 100, 'project_root': str(dev.ROOT)}
            row = {'msg': '装配停止结果', 'mode': 'supervisor', 'run_id': '本轮', 'ok': False, 'error': '结清超时'}
            log.write_text(json.dumps(row, ensure_ascii=False) + '\n', encoding='utf-8')
            with (
                patch.object(dev, 'read', return_value=meta),
                patch.object(dev, 'owned', return_value=False),
                patch.object(dev, 'windows_stop') as windows_stop,
                patch.object(dev.os, 'kill') as kill,
            ):
                with self.assertRaisesRegex(RuntimeError, '结清超时'):
                    dev.stop()
                windows_stop.assert_not_called()
                kill.assert_not_called()
