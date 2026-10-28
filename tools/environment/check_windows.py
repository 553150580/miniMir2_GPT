"""检查Windows用户级锁定工具和本任务原生数据库；不安装或改变服务状态。"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
from toolchain import creator_app, windows_paths


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--strict', action='store_true')
    parser.add_argument('--skip-database', action='store_true')
    parser.add_argument('--include-linux-vm', action='store_true')
    parser.add_argument('--report', default='docs/开发环境检查.json')
    parser.add_argument('--creator-path')
    args = parser.parse_args()
    lock, prefix = windows_paths()
    checks, tools = [], {}

    def check(name, passed, evidence):
        checks.append(dict(code=name, status='passed' if passed else 'failed', evidence=evidence))

    for name, spec in lock['tools'].items():
        if spec.get('optional_linux_vm') and not args.include_linux_vm:
            continue
        binary = prefix / spec['executable']
        arguments = ['version'] if name == 'go' else ['--version']
        if name == 'qemu':
            arguments = ['--version']
        if binary.is_file():
            result = subprocess.run([str(binary)] + arguments, capture_output=True, encoding='utf-8', errors='replace', timeout=30)
            output = result.stdout.strip()
            tools[name] = dict(path=str(binary), version=output, exit_code=result.returncode,
                               binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest())
            check(name + '_version', result.returncode == 0 and spec['version'] in output, tools[name])
        else:
            check(name + '_version', False, str(binary))
        archive = prefix / 'downloads' / spec['url'].rsplit('/', 1)[1]
        algorithm, expected = ('sha512', spec['sha512']) if 'sha512' in spec else ('sha256', spec['sha256'])
        digest = None
        if archive.is_file():
            with archive.open('rb') as stream:
                digest = hashlib.file_digest(stream, algorithm).hexdigest()
        check(name + '_archive', digest == expected, dict(expected=expected, actual=digest))
    for name, spec in lock.get('host_tools', {}).items():
        binary = Path(os.path.expandvars(spec['executable']))
        result = subprocess.run([str(binary), '--version'], capture_output=True, encoding='utf-8') if binary.is_file() else None
        digest = hashlib.sha256(binary.read_bytes()).hexdigest() if binary.is_file() else None
        check(name + '_host_tool', bool(result and result.returncode == 0 and spec['version'] in result.stdout and digest == spec['sha256']),
              dict(path=str(binary), sha256=digest, expected_version=spec['version']))
    runtime = dict(python=sys.version.split()[0], executable=sys.executable)
    import PIL
    runtime['pillow'] = PIL.__version__
    check('isolated_python', runtime['python'] == lock['python_version'] and runtime['pillow'] == lock['pillow_version']
          and Path(sys.executable).resolve() == (prefix / 'python/tools-venv/Scripts/python.exe').resolve(), runtime)
    app = creator_app()
    literal = str(app / 'CocosCreator.exe').replace("'", "''")
    version = subprocess.check_output(['powershell.exe', '-NoProfile', '-NonInteractive', '-Command', f"(Get-Item -LiteralPath '{literal}').VersionInfo.ProductVersion"], encoding='utf-8').strip()
    check('creator_version', version == lock['creator_version'], dict(app=str(app), version=version))
    node = prefix / lock['tools']['node']['executable']
    package_roots = {'client': ROOT / 'client/node_modules', 'generator': prefix / 'node-tools/node_modules'}
    for scope, directory in package_roots.items():
        expected = {'protobufjs': '8.8.1', 'long': '5.3.2'}
        if scope == 'generator':
            expected['protobufjs-cli'] = '2.7.1'
        for name, selected in expected.items():
            metadata = directory / name / 'package.json'
            actual = json.loads(metadata.read_text(encoding='utf-8')).get('version') if metadata.is_file() else None
            check(f'{scope}_{name}', actual == selected, dict(version=actual, expected=selected))
    plugin = prefix / 'go-tools/bin/protoc-gen-go.exe'
    result = subprocess.run([str(plugin), '--version'], capture_output=True, encoding='utf-8') if plugin.is_file() else None
    check('protoc_gen_go', bool(result and result.returncode == 0 and 'v1.36.12' in result.stdout), str(plugin))
    database = dict(status='not_checked')
    if not args.skip_database:
        import mongodb_windows
        try:
            database = mongodb_windows.status()
            check('owned_database', database['running'] and database['ownership_verified'] and database['listener_verified'], database)
        except (OSError, RuntimeError, ValueError, subprocess.SubprocessError) as error:
            database = dict(error=str(error))
            check('owned_database', False, database)
    passed = all(item['status'] == 'passed' for item in checks)
    report = dict(schema_version=3, checked_at_utc=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                  platform='windows-amd64', toolchain_entry='./tools/dev.ps1', wrapper_active=os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') == '1',
                  tools=tools, python=runtime, checks=checks, database=database,
                  checked_scope_passed=passed, database_checked=not args.skip_database, production_ready=False)
    target = (ROOT / args.report).resolve()
    if not target.is_relative_to(ROOT):
        raise RuntimeError('检查报告必须位于本项目内')
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n', encoding='utf-8', newline='\n')
    print(json.dumps(dict(passed=passed, failed=[item['code'] for item in checks if item['status'] != 'passed'], report=str(target)), ensure_ascii=False))
    return 2 if args.strict and not passed else 0


if __name__ == '__main__':
    raise SystemExit(main())
