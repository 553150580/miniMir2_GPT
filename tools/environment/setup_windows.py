"""在已引导的用户目录内安装固定生成器、驱动及可选浏览器测试库。"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
DEV = [sys.executable, str(ROOT / 'tools/dev')]


def run(arguments):
    subprocess.run(DEV + arguments, cwd=ROOT, check=True)


def node_tools(prefix, name, package, lock):
    directory = prefix / name
    directory.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(ROOT / package, directory / 'package.json')
    shutil.copyfile(ROOT / lock, directory / 'package-lock.json')
    run(['npm', 'ci', '--prefix', str(directory), '--ignore-scripts', '--no-audit', '--no-fund', '--registry=https://registry.npmjs.org'])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--browser', action='store_true')
    args = parser.parse_args()
    if os.name != 'nt' or os.environ.get('MINIMIR_TOOLCHAIN_ACTIVE') != '1':
        raise SystemExit('请通过Windows统一入口执行')
    prefix = Path(os.environ['MINIMIR_TOOLCHAIN_ROOT'])
    node_tools(prefix, 'node-tools', 'conf/toolchain/node-tools.package.json', 'conf/toolchain/node-tools.package-lock.json')
    run(['go', 'install', 'google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12'])
    run(['uv', 'pip', 'install', '--python', sys.executable, '--require-hashes', '--only-binary', ':all:', '-r', 'conf/toolchain/mongodb-driver.windows.requirements.lock'])
    if not (ROOT / 'client/node_modules/protobufjs/package.json').is_file():
        run(['--cwd', 'client', 'npm', 'ci', '--ignore-scripts', '--no-audit', '--no-fund', '--registry=https://registry.npmjs.org'])
    if args.browser:
        node_tools(prefix, 'browser-tools', 'conf/toolchain/browser-tools.package.json', 'conf/toolchain/browser-tools.package-lock.json')
    run(['python3', 'tools/environment/configure_git_windows.py'])
    print(json.dumps(dict(initialized=True, user_directory_only=True, windows_features_changed=False, vm_created=False)))


if __name__ == '__main__':
    main()
