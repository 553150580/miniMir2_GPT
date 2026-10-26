#!/usr/bin/env python3
"""将已校验的Go稳定版安装到用户工具目录，保留系统Go。"""
import argparse
import hashlib
import json
from pathlib import Path
import platform
import shutil
import subprocess
import tarfile
import tempfile
import urllib.request

ROOT=Path(__file__).resolve().parents[2]
PREFIX=Path.home()/'.local/share/minimir-toolchain'


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version',default='go1.27.3')
    parser.add_argument('--metadata',type=Path,default=ROOT/'.work/go-downloads.json')
    args=parser.parse_args()
    if platform.system()!='Darwin' or platform.machine()!='x86_64':raise RuntimeError('当前安装锁仅用于macOS x86_64')
    metadata=json.loads(args.metadata.read_text(encoding='utf-8'))
    release=next(x for x in metadata if x['version']==args.version and x['stable'])
    asset=next(x for x in release['files'] if x['os']=='darwin' and x['arch']=='amd64' and x['kind']=='archive')
    downloads=PREFIX/'go/downloads';downloads.mkdir(parents=True,exist_ok=True)
    archive=downloads/asset['filename']
    expected=asset['sha256']
    sources=['https://go.dev/dl/'+asset['filename'],'https://golang.google.cn/dl/'+asset['filename'],'https://mirrors.ustc.edu.cn/golang/'+asset['filename'],'https://mirrors.aliyun.com/golang/'+asset['filename']]
    selected=None
    for url in sources:
        if archive.exists() and hashlib.sha256(archive.read_bytes()).hexdigest()==expected:
            selected='verified_cache';break
        print('下载Go：'+url,flush=True)
        result=subprocess.run(['curl','--fail','--location','--connect-timeout','10','--max-time','120','--retry','1','--output',str(archive),url],capture_output=True,text=True)
        if result.returncode==0 and hashlib.sha256(archive.read_bytes()).hexdigest()==expected:
            selected=url;break
        print('该来源不可用或摘要不匹配，尝试下一已列来源',flush=True)
    if selected is None:raise RuntimeError('未取得与Go官方摘要一致的安装包')
    destination=PREFIX/'go'/args.version
    if not (destination/'bin/go').exists():
        with tempfile.TemporaryDirectory(dir=str(destination.parent),prefix='.install-') as temp:
            with tarfile.open(archive,'r:gz') as tar:
                for member in tar.getmembers():
                    parts=Path(member.name).parts
                    if not parts or parts[0]!='go' or '..' in parts or member.name.startswith('/') or member.issym() or member.islnk():raise RuntimeError('安装包出现不允许的路径或链接')
                tar.extractall(temp)
            shutil.move(str(Path(temp)/'go'),str(destination))
    version=subprocess.check_output([str(destination/'bin/go'),'version'],text=True).strip()
    if args.version not in version:raise RuntimeError('安装后Go版本与锁不符')
    lock={'version':args.version,'platform':'darwin-amd64','source_metadata':'https://go.dev/dl/?mode=json','official_download':sources[0],'downloaded_from':selected,'archive_sha256':expected,'official_checksum_verified':True,'installation':str(destination),'executable':str(destination/'bin/go'),'version_output':version,'system_go_replaced':False}
    target=ROOT/'conf/toolchain/go.lock.json';target.parent.mkdir(parents=True,exist_ok=True)
    target.write_text(json.dumps(lock,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    print(json.dumps(lock,ensure_ascii=False),flush=True)


if __name__=='__main__':main()
