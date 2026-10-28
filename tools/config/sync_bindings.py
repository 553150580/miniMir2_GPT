#!/usr/bin/env python3
"""核对不可变预览清单后，把双端配置类型同步到各自工程。"""
import hashlib
import json
import os
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]


def main():
    directory=ROOT/'config/releases'
    pointer=json.loads((directory/'preview_current.json').read_text(encoding='utf-8'))
    release=(directory/pointer['path']).resolve()
    if directory.resolve() not in release.parents:raise ValueError('配置发布目录不合法')
    manifest=json.loads((release/'manifest.json').read_text(encoding='utf-8'))
    outputs={'config_types.go':ROOT/'server/internal/configtypes/config_types.go','config_types.ts':ROOT/'client/assets/scripts/generated/config_types.ts'}
    for name,target in outputs.items():
        data=(release/name).read_bytes()
        if hashlib.sha256(data).hexdigest()!=manifest['files'][name]:raise ValueError('配置类型摘要不匹配：'+name)
        target.parent.mkdir(parents=True,exist_ok=True)
        temp=target.with_suffix(target.suffix+'.tmp');temp.write_bytes(data);os.replace(temp,target)
    record={'config_version':manifest['version'],'server':'server/internal/configtypes/config_types.go','client':'client/assets/scripts/generated/config_types.ts','runtime_ready':manifest['runtime_ready'],'generated_only':True}
    (ROOT/'conf/toolchain/config-binding-sync.json').write_text(json.dumps(record,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
    print(json.dumps(record,ensure_ascii=False))


if __name__=='__main__':main()
