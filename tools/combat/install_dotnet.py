#!/usr/bin/env python3
"""从微软官方发布清单安装独立.NET稳定SDK，不修改系统工具。"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
PREFIX = Path.home() / '.local/share/minimir-toolchain'
MANIFEST = 'https://builds.dotnet.microsoft.com/dotnet/release-metadata/10.0/releases.json'


def open_official(url):
    for attempt in range(3):
        try:
            return urllib.request.urlopen(url, timeout=30)
        except (urllib.error.URLError, TimeoutError):
            if attempt == 2:
                raise
            time.sleep(attempt + 1)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--upgrade', action='store_true', help='显式升级至官方最新稳定SDK；默认复现已有锁')
    args = parser.parse_args()
    if platform.system() != 'Darwin':
        raise SystemExit('本安装入口仅核验macOS SDK')
    machine = platform.machine()
    arm = subprocess.run(['sysctl', '-n', 'hw.optional.arm64'], capture_output=True, text=True)
    rid = 'osx-arm64' if machine == 'arm64' or arm.stdout.strip() == '1' else 'osx-x64'
    lock_path = ROOT / 'conf/toolchain/dotnet.lock.json'
    locked = json.loads(lock_path.read_text()) if lock_path.exists() else None
    raw = open_official(MANIFEST).read()
    metadata = json.loads(raw)
    version = metadata['latest-sdk'] if args.upgrade or locked is None else locked['version']
    if '-' in version or metadata['support-phase'] not in ['active', 'maintenance']:
        raise SystemExit('拒绝非稳定或已停止支持的SDK')
    sdks = [s for release in metadata['releases'] for s in [release.get('sdk', {})] + release.get('sdks', [])]
    sdk = next(s for s in sdks if s.get('version') == version)
    package = next(f for f in sdk['files'] if f['rid'] == rid and f['url'].endswith('.tar.gz'))
    if not package['url'].startswith('https://builds.dotnet.microsoft.com/dotnet/Sdk/'):
        raise SystemExit('SDK地址不属于官方主源')
    if locked is not None and not args.upgrade:
        if locked['platform'] != rid or locked['source_metadata'] != MANIFEST or locked['official_download'] != package['url'] or locked['archive_sha512'].lower() != package['hash'].lower():
            raise SystemExit('当前锁与本机/官方清单不一致；拒绝静默改锁，请核查后显式--upgrade')
    evidence = ROOT / '.work/combat-modes/oracle'
    evidence.mkdir(parents=True, exist_ok=True)
    (evidence / 'dotnet-releases10.0.json').write_bytes(raw)
    cache = PREFIX / 'cache/dotnet'
    cache.mkdir(parents=True, exist_ok=True)
    archive = cache / Path(package['url']).name
    if not archive.exists():
        pending = archive.with_suffix('.download')
        print('下载官方.NET ' + version + ' ' + rid, flush=True)
        with open_official(package['url']) as source, pending.open('wb') as dest:
            shutil.copyfileobj(source, dest, length=1024 * 1024)
        pending.replace(archive)
    with archive.open('rb') as source:
        digest = hashlib.file_digest(source, 'sha512').hexdigest()
    if digest.lower() != package['hash'].lower():
        raise SystemExit('官方SHA512不匹配，拒绝解包或执行')
    installation = PREFIX / 'dotnet' / version
    if not installation.exists():
        staging = installation.with_name(version + '.installing')
        if staging.exists():
            raise SystemExit('存在未完成安装目录，请先核查：' + str(staging))
        staging.mkdir(parents=True)
        with tarfile.open(archive, 'r:gz') as bundle:
            bundle.extractall(staging, filter='data')
        staging.replace(installation)
    executable = installation / 'dotnet'
    with tarfile.open(archive, 'r:gz') as bundle:
        member = next(m for m in bundle.getmembers() if m.name.removeprefix('./') == 'dotnet' and m.isfile())
        with bundle.extractfile(member) as source, executable.open('rb') as installed:
            expected_executable = hashlib.file_digest(source, 'sha256').hexdigest()
            if hashlib.file_digest(installed, 'sha256').hexdigest() != expected_executable:
                raise SystemExit('已安装dotnet可执行文件与官方包不符，拒绝执行')
    env = os.environ | {'DOTNET_ROOT': str(installation), 'DOTNET_MULTILEVEL_LOOKUP': '0',
        'DOTNET_CLI_TELEMETRY_OPTOUT': '1', 'DOTNET_SKIP_FIRST_TIME_EXPERIENCE': '1', 'DOTNET_NOLOGO': '1',
        'DOTNET_CLI_HOME': str(PREFIX / 'cache/dotnet-cli'), 'DOTNET_GENERATE_ASPNET_CERTIFICATE': 'false',
        'DOTNET_ADD_GLOBAL_TOOLS_TO_PATH': 'false', 'DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE': 'true'}
    actual = subprocess.check_output([str(executable), '--version'], env=env, text=True).strip()
    if actual != version:
        raise SystemExit('SDK实际执行版本不匹配')
    lock = {'version': version, 'platform': rid, 'detected_process_machine': machine,
        'hardware_arm64_sysctl': arm.stdout.strip() or None, 'source_metadata': MANIFEST,
        'source_metadata_sha256': hashlib.sha256(raw).hexdigest(), 'official_download': package['url'],
        'archive_sha512': digest, 'official_checksum_verified': True, 'installation': str(installation),
        'executable': str(executable), 'executable_sha256': expected_executable,
        'version_output': actual, 'system_dotnet_replaced': False}
    lock_path.write_text(json.dumps(lock, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(json.dumps({'version': actual, 'platform': rid, 'sha512_verified': True, 'installation': str(installation)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
