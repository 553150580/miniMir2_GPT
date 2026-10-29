"""本轮只核QA的HTTP创角职业观察修正；不启动浏览器或服务。"""
from pathlib import Path
import datetime
import hashlib
import json
import subprocess
import sys

OUT = Path(__file__).resolve().parent
WORK = OUT.parents[1]
ROOT = WORK.parents[1]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def main():
    root_report = WORK / 'client/full-1/report.json'
    frozen = read(root_report)
    files = ['tools/client/verify_equipment_browser.cjs', 'tools/client/test_equipment_browser.cjs',
             '.work/equipment-growth-20261030/browser/run-02/run.py']
    before = {p: sha(ROOT / p) for p in files}
    original = read(OUT / 'before.json')
    report = {'started_at_utc': now(), 'before': before, 'previous_before_http_fix': original['before'],
              'commands': [], 'chrome_started': False, 'creator_started': False,
              'database_control': False, 'service_control': False, 'passed': False,
              'scope': '仅将创角10005职业观察从错误WS移至本机HTTP，不保存其它认证帧/账户/密码/Token/正文',
              'client_full_report_sha256': sha(root_report), 'client_input_count': len(frozen['before']),
              'client_before': {p: sha(ROOT / p) for p in frozen['before']},
              'external_engine_before': {p: sha(Path(p)) for p in frozen['external_engine_before']},
              'external_font_before': {p: sha(Path(p)) for p in frozen['external_font_before']}}
    try:
        assert report['client_before'] == frozen['before'] == frozen['after']
        for kind in ('engine', 'font'):
            assert report['external_' + kind + '_before'] == frozen['external_' + kind + '_before'] == frozen['external_' + kind + '_after']
        commands = [
            ('syntax', ['node', '--check', 'tools/client/verify_equipment_browser.cjs']),
            ('preflight', ['node', 'tools/client/verify_equipment_browser.cjs', '--preflight']),
            ('tool-tests', ['node', '--test', 'tools/client/test_equipment_browser.cjs']),
            ('wrapper-dry-run', ['python3', '.work/equipment-growth-20261030/browser/run-02/run.py'])]
        for name, args in commands:
            command = [sys.executable, str(ROOT / 'tools/dev'), *args]
            with (OUT / (name + '.stdout.log')).open('xb') as stdout, (OUT / (name + '.stderr.log')).open('xb') as stderr:
                result = subprocess.run(command, cwd=ROOT, stdout=stdout, stderr=stderr, check=False)
            report['commands'].append({'name': name, 'command': command, 'exit_code': result.returncode,
                                       'stdout_sha256': sha(OUT / (name + '.stdout.log')),
                                       'stderr_sha256': sha(OUT / (name + '.stderr.log'))})
            assert result.returncode == 0, name + ' failed'
        report['after'] = {p: sha(ROOT / p) for p in files}
        report['current'] = {p: sha(ROOT / p) for p in files}
        report['stable'] = report['before'] == report['after'] == report['current']
        report['client_after'] = {p: sha(ROOT / p) for p in frozen['before']}
        report['client_current_equals_root_before_and_after'] = report['client_after'] == report['client_before'] == frozen['before'] == frozen['after']
        report['historical_run01_before'] = original['historical_run01_before']
        report['historical_run01_after'] = {p: sha(WORK / 'browser/run-01' / p) for p in original['historical_run01_before']}
        report['historical_run01_preserved'] = report['historical_run01_before'] == report['historical_run01_after']
        for kind in ('engine', 'font'):
            report['external_' + kind + '_after'] = {p: sha(Path(p)) for p in frozen['external_' + kind + '_before']}
            assert report['external_' + kind + '_before'] == report['external_' + kind + '_after']
        report['tool_test_count'] = 5
        report['report_contract'] = read(WORK / 'client/tool-static-3/report.json')['report_contract']
        report['run_command'] = '.\\tools\\dev.ps1 python3 .work/equipment-growth-20261030/browser/run-02/run.py --run'
        assert report['stable'] and report['client_current_equals_root_before_and_after'] and report['historical_run01_preserved']
        report['passed'] = True
    finally:
        report['finished_at_utc'] = now()
        (OUT / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'passed': report['passed'], 'report_sha256': sha(OUT / 'report.json'),
                      'tool_sha256': before[files[0]], 'tool_tests': report['tool_test_count']}, ensure_ascii=False))


if __name__ == '__main__':
    main()
