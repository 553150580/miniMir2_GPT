"""只读快照探针包装器，保留实际stdout/stderr/退出码，不接管服务。"""
from pathlib import Path
import datetime
import hashlib
import json
import subprocess
import sys

OUT = Path(__file__).resolve().parent
ROOT = OUT.parents[2]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    tool = OUT / 'probe.cjs'
    command = [sys.executable, str(ROOT / 'tools/dev'), 'node', str(tool)]
    files = [tool, OUT / 'run.py', ROOT / 'tools/client/verify_equipment_browser.cjs',
             ROOT / 'client/assets/resources/development/gameplay.json', ROOT / 'docs/客户端构建检查.json',
             ROOT / 'config/development/first-loop.json']
    before = {p.relative_to(ROOT).as_posix(): sha(p) for p in files}
    record = {'command': command, 'started_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'before': before}
    with (OUT / 'stdout.log').open('xb') as stdout, (OUT / 'stderr.log').open('xb') as stderr:
        result = subprocess.run(command, cwd=ROOT, stdout=stdout, stderr=stderr, check=False)
    record.update(exit_code=result.returncode, finished_at_utc=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                  after={p.relative_to(ROOT).as_posix(): sha(p) for p in files}, stdout_sha256=sha(OUT / 'stdout.log'), stderr_sha256=sha(OUT / 'stderr.log'))
    record['stable'] = record['before'] == record['after']
    record['probe_report_sha256'] = sha(OUT / 'report.json') if (OUT / 'report.json').is_file() else None
    (OUT / 'wrapper.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'exit_code': result.returncode, 'stable': record['stable'], 'report_sha256': record['probe_report_sha256'], 'wrapper_sha256': sha(OUT / 'wrapper.json')}, ensure_ascii=False))
    return result.returncode if record['stable'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
