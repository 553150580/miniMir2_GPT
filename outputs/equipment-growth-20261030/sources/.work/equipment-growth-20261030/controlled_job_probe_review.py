"""只验证固定无业务探针；绝不启动切换任务。"""
from pathlib import Path
import ast
import hashlib
import json
import runpy
import subprocess
import sys
import time
from unittest import mock

WORK = Path(__file__).resolve().parent
ROOT = WORK.parents[1]
TOOL = WORK / 'controlled_job.py'
OUT = WORK / 'controlled-job-review'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    OUT.mkdir(exist_ok=False)
    definition = runpy.run_path(str(TOOL), run_name='controlled_probe_review')
    ast.parse(TOOL.read_text(encoding='utf-8'))
    before = definition['snapshot']('cutover')
    report = {'passed': False, 'scope': '真实固定2秒无业务probe；未执行cutover/服务/网络/数据库操作',
              'source_before': before, 'commands': [], 'checks': []}
    python = str(definition['base_python']())

    def call(action, label):
        command = [python, str(TOOL), action, 'probe', '--phase', 'probe-01']
        result = subprocess.run(command, cwd=ROOT, capture_output=True)
        stdout, stderr = OUT / (label + '.stdout.log'), OUT / (label + '.stderr.log')
        stdout.write_bytes(result.stdout)
        stderr.write_bytes(result.stderr)
        report['commands'].append({'command': command, 'exit_code': result.returncode,
                                   'stdout': stdout.relative_to(ROOT).as_posix(), 'stdout_sha256': sha(stdout),
                                   'stderr': stderr.relative_to(ROOT).as_posix(), 'stderr_sha256': sha(stderr)})
        if result.returncode != 0:
            raise RuntimeError(result.stderr.decode('utf-8'))
        return json.loads(result.stdout)

    try:
        started = call('start', '01-start')
        assert started['status'] == 'started' and started['live']
        report['start'] = started
        report['checks'].append('start实际登记原PID/映像/创建时钟/run_id，并返回live')
        live = call('status', '02-live')
        assert live['status'] == 'live' and live['live'] and not live['terminal_present']
        report['live'] = live
        report['checks'].append('status实际CIM命令行+创建时钟核对同一worker，2秒probe尚活跃')
        deadline = time.monotonic() + 15
        count = 0
        while time.monotonic() < deadline:
            time.sleep(0.15)
            count += 1
            observed = call('status', '03-poll-' + str(count))
            if observed['terminal_present'] and not observed['live']:
                break
        else:
            raise RuntimeError('探针观察超时；保留原进程/证据，绝不自动重启或停止')
        terminal = observed['terminal']
        assert observed['status'] == 'completed' and observed['passed']
        assert terminal['exit_code'] == 0 and terminal['worker_exit_code'] == 0
        assert terminal['source_stable'] and terminal['probe_duration_seconds'] == 2 and terminal['seconds'] >= 2
        assert observed['worker']['pid'] == started['worker']['pid']
        report['completed'] = observed
        report['checks'].append('同一worker真正退出且terminal为子任务exit0；原日志/源码前后相同')
        # 这是纯分支替身，不伪造真实进程终止或业务结果。
        fake_out = definition['folder']('probe', 'probe-missing-terminal')
        fake = {**definition['read'](Path(started['directory']) / 'launch.json'),
                'phase': 'probe-missing-terminal', 'directory': str(fake_out)}
        fake['worker_argv'] = [python, str(TOOL), '_worker', 'probe', '--phase', fake['phase'], '--run-id', fake['run_id']]
        with mock.patch.dict(definition['status'].__globals__, {'read': lambda path: fake}):
            missing = definition['status']('probe', fake['phase'])
        assert missing['status'] == 'stopped_without_terminal' and not missing['passed'] and not missing['live']
        report['missing_terminal_pure_branch'] = missing
        report['checks'].append('纯分支：缺原PID且无终态只能stopped_without_terminal，不冒称成功/重启')
        rejected = []
        for task, phase in [('other', 'probe-02'), ('probe', '../escape'), ('cutover', 'probe-02')]:
            try:
                definition['folder'](task, phase)
            except ValueError:
                rejected.append([task, phase])
        assert len(rejected) == 3
        report['rejected_arguments'] = rejected
        report['checks'].append('纯分支：任意任务、越界目录和任务/阶段不一致均拒绝')
        try:
            definition['start']('probe', 'probe-01')
        except FileExistsError:
            report['checks'].append('已存在精确phase目录拒绝重用，不覆盖原日志或再启动')
        else:
            raise RuntimeError('重复phase没有拒绝')
        report['source_after'] = definition['snapshot']('cutover')
        assert report['source_after'] == before
        report['passed'] = True
    finally:
        (OUT / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'passed': report['passed'], 'checks': len(report['checks']),
                      'report': str(OUT / 'report.json'), 'report_sha256': sha(OUT / 'report.json'),
                      'tool_sha256': sha(TOOL)}, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
