"""通过项目锁定Python运行实际脚本，保存stdout/stderr、耗时和退出码。"""
import contextlib
from datetime import datetime, timezone
import io
import json
from pathlib import Path
import runpy
import sys
import time
import traceback

OUT = Path(__file__).resolve().parent / 'checks'
OUT.mkdir(exist_ok=True)
tag, script, *args = sys.argv[1:]
start = datetime.now(timezone.utc).isoformat()
clock = time.monotonic()
output = io.StringIO()
code = 0
sys.argv = [script, *args]
sys.path.insert(0, str(Path(script).resolve().parent))
with contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
    try:
        runpy.run_path(script, run_name='__main__')
    except SystemExit as error:
        code = error.code if isinstance(error.code, int) else 1
    except BaseException:
        traceback.print_exc()
        code = 1
raw = output.getvalue()
(OUT / f'{tag}.log').write_text(raw, encoding='utf-8', newline='\n')
record = {'command': ['.\\tools\\dev.ps1', 'python3', str(Path(__file__)), tag, script, *args], 'actual_python_argv': [script, *args], 'started_at': start, 'finished_at': datetime.now(timezone.utc).isoformat(), 'elapsed_seconds': time.monotonic() - clock, 'exit_code': code, 'log': f'checks/{tag}.log'}
(OUT / f'{tag}.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
print(raw, end='')
print(json.dumps(record, ensure_ascii=False))
raise SystemExit(code)
