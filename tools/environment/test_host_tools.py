"""真实验证 Windows 入口参数、失败状态和开发进程的优雅退出边界。"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
from process_control import background_options, windows_identity, windows_owned, windows_stop


@unittest.skipUnless(os.name == 'nt', 'Windows 专属进程及 PowerShell 入口')
class WindowsHostTools(unittest.TestCase):
    def launch(self, arguments):
        return subprocess.run([sys.executable, str(ROOT / 'tools/dev')] + arguments, cwd=ROOT,
                              capture_output=True, encoding='utf-8', timeout=30)

    def test_failure_code_and_outside_directory_rejected(self):
        self.assertEqual(self.launch(['python3', '-c', 'raise SystemExit(7)']).returncode, 7)
        self.assertNotEqual(self.launch(['--cwd', '..', 'python3', '-c', 'print("invalid")']).returncode, 0)

    def test_go_uses_server_and_locked_version(self):
        result = self.launch(['go', 'env', 'GOMOD', 'GOVERSION'])
        self.assertEqual(result.returncode, 0, result.stderr)
        lines = result.stdout.splitlines()
        self.assertEqual(Path(lines[0]).resolve(), ROOT / 'server/go.mod')
        self.assertEqual(lines[1], 'go1.27.3')

    def test_powershell_preserves_native_arguments(self):
        values = ['中文 空格', 'literal"quote', '', 'C:\\path with space\\']
        literal = lambda text: "'" + text.replace("'", "''") + "'"
        command = '& ' + literal(str(ROOT / 'tools/dev.ps1')) + ' python3 -c ' + literal('import json,sys; print(json.dumps(sys.argv[1:],ensure_ascii=False))') + ' ' + ' '.join(map(literal, values))
        with tempfile.TemporaryDirectory(dir=ROOT / '.work') as directory:
            script = Path(directory) / 'arguments.ps1'
            script.write_text(command, encoding='utf-8-sig')
            result = subprocess.run(['powershell.exe', '-NoProfile', '-File', str(script)], capture_output=True, encoding='utf-8', timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), values)

    def test_owned_console_stops_gracefully_and_rejects_stale_pid(self):
        program = 'import signal,sys,time; signal.signal(signal.SIGBREAK,lambda *_:sys.exit(0)); print("ready",flush=True)\nwhile True: time.sleep(0.05)'
        binary = sys._base_executable
        child = subprocess.Popen([binary, '-c', program], stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                 encoding='utf-8', **background_options())
        try:
            self.assertEqual(child.stdout.readline().strip(), 'ready')
            owned = windows_identity(child.pid)
            self.assertTrue(windows_owned(owned, binary))
            stale = dict(owned, start_ticks=owned['start_ticks'] + 1)
            self.assertFalse(windows_owned(stale, binary))
            with self.assertRaises(RuntimeError):
                windows_stop(stale, binary)
            self.assertIsNone(child.poll())
            windows_stop(owned, binary)
            self.assertEqual(child.wait(timeout=10), 0)
        finally:
            if child.poll() is None:
                windows_stop(windows_identity(child.pid), binary)
                child.wait(timeout=10)
            child.stdout.close()
            child.stderr.close()


if __name__ == '__main__':
    unittest.main()
