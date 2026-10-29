"""只读保留无终态检查的实际进程/来源观察，不把句柄缺失解释为测试通过。"""
from pathlib import Path
import datetime, hashlib, json, runpy, subprocess, sys

ROOT = Path(__file__).resolve().parents[2]
WORK = Path(__file__).resolve().parent
OUT = WORK / 'go/full-2'
DEV = [sys.executable, str(ROOT / 'tools/dev')]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read(path):
    return json.loads(path.read_text(encoding='utf-8'))


def main():
    target = OUT / 'interruption.json'
    assert not target.exists() and not (OUT / 'report.json').exists() and not (OUT / 'wrapper.json').exists()
    started = read(OUT / 'started.json')
    script = "Get-CimInstance Win32_Process -Filter \"name='python.exe' OR name='go.exe' OR name='integration.test.exe' OR name='persistence.test.exe' OR name='game.test.exe' OR name='devconfig.test.exe' OR name='nbserver.exe'\" | Where-Object { $_.Name -match '[.]test[.]exe$' -or $_.Name -eq 'nbserver.exe' -or $_.CommandLine -match 'stage_checks[.]py go|tools/server/test[.]py|go[.]exe test -json' } | Select-Object ProcessId,ParentProcessId,CreationDate,Name,CommandLine | ConvertTo-Json -Depth 3"
    commands = [('processes', ['powershell.exe', '-NoProfile', '-Command', script]),
                ('server-owned', DEV + ['python3', 'tools/server/dev.py', 'status']),
                ('mongo-owned', DEV + ['python3', 'tools/environment/mongodb_dev.py', 'status'])]
    rows, values = [], {}
    for name, command in commands:
        output, error = OUT / (name + '.stdout.txt'), OUT / (name + '.stderr.txt')
        with output.open('xb') as stdout, error.open('xb') as stderr:
            result = subprocess.run(command, cwd=ROOT, stdout=stdout, stderr=stderr)
        assert result.returncode == 0
        values[name] = read(output)
        rows.append({'name': name, 'command': command, 'exit_code': result.returncode,
                     'stdout': output.relative_to(ROOT).as_posix(), 'stdout_sha256': sha(output),
                     'stderr': error.relative_to(ROOT).as_posix(), 'stderr_sha256': sha(error)})
    processes = values['processes']
    if isinstance(processes, dict):
        processes = [processes]
    assert processes and all(p['Name'] == 'nbserver.exe' for p in processes), '仍有测试或检查进程，不能重跑'
    assert all(str(ROOT / 'server/bin/nbserver.exe').lower() in p['CommandLine'].lower() for p in processes)
    server, mongo = values['server-owned'], values['mongo-owned']
    assert server['running'] and all(v['status'] == 'ok' and v['rule_version'] == 21 for v in server['health'].values())
    assert mongo['running'] and mongo['endpoint_verified'] and mongo['ownership_verified']
    latest = read(ROOT / 'server/test-results/latest.json')
    assert latest['started_at_utc'] < started['started_at_utc'], 'latest可能属于本轮，须先判断实际终态'
    definition = runpy.run_path(str(ROOT / 'tools/server/test.py'), run_name='interruption_enumeration_only')
    current = {p.relative_to(ROOT).as_posix(): sha(p) for p in definition['input_files']()}
    assert current == started['before']
    value = {'status': 'stopped_without_terminal_report', 'accepted_as_pass': False, 'test_exit_code': None,
             'observed_at_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
             'specific_cause_proven': False,
             'scope': '原执行句柄及实际检查/测试进程均缺失；原终态未写出，不能认作通过或业务失败；未停止任何进程或清理数据库',
             'prior_tool_observation': {'functions_cell': '1650 not found', 'exec_session': '80419 Unknown process id'},
             'commands': rows, 'source_count': len(current), 'source_stable_since_start': True,
             'latest_report_is_older_attempt': True, 'latest_report_sha256': sha(ROOT / 'server/test-results/latest.json'),
             'preserved_original_files': {p.name: sha(p) for p in OUT.iterdir() if p.name in {'started.json', 'full-mongo-race.stdout.txt', 'full-mongo-race.stderr.txt'}},
             'live_main_processes': processes, 'owned_mongo_pid': mongo['pid'], 'script_sha256': sha(Path(__file__))}
    target.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps({'recorded': True, 'accepted_as_pass': False, 'source_count': len(current), 'live_test_processes': 0}, ensure_ascii=False))


if __name__ == '__main__':
    main()
