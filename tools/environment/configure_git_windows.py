"""仅修正当前仓库复制来的Mac配置；不修改全局Git、身份或保存任何凭据。"""
import json
import os
import subprocess


def main():
    if os.name != 'nt':
        raise SystemExit('此工具仅用于Windows仓库迁移')
    subprocess.run(['git', 'config', '--local', 'core.filemode', 'false'], check=True)
    key = 'credential.https://github.com.helper'
    current = subprocess.run(['git', 'config', '--local', '--get-all', key], capture_output=True, text=True)
    legacy = '/Users/' in current.stdout
    if legacy:
        subprocess.run(['git', 'config', '--local', '--replace-all', key, ''], check=True)
        subprocess.run(['git', 'config', '--local', '--add', key, 'manager'], check=True)
    print(json.dumps(dict(repository_only=True, filemode_tracking=False, copied_mac_credential_helper_replaced=legacy)))


if __name__ == '__main__':
    main()
