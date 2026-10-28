#!/usr/bin/env python3
"""使用已锁定的protoc与protoc-gen-go生成真实服务端PB代码。"""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess

ROOT=Path(__file__).resolve().parents[2]


def main():
    for tool in ('protoc','protoc-gen-go','go'):
        if not shutil.which(tool):raise SystemExit('请通过 ./tools/dev python3 tools/protocol/generate_go_pb.py 执行；缺少 '+tool)
    sources=sorted((ROOT/'shared/proto').glob('*.proto'))
    output=ROOT/'server/api/generated/minimirv1';output.mkdir(parents=True,exist_ok=True)
    args=['protoc','--proto_path='+str(ROOT/'shared/proto'),'--go_out='+str(ROOT/'server'),'--go_opt=module=minimir']+[str(p) for p in sources]
    subprocess.run(args,cwd=ROOT,check=True)
    result={'protoc':subprocess.check_output(['protoc','--version'],text=True).strip(),'protoc_gen_go':subprocess.check_output(['protoc-gen-go','--version'],text=True).strip(),'sources':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sources},'outputs':{p.relative_to(ROOT).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in output.glob('*.pb.go')},'business_implemented':False}
    (ROOT/'conf/toolchain/go-pb-generation.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8', newline='\n')
    print(json.dumps({'go_pb_files':len(result['outputs']),'protoc':result['protoc'],'protoc_gen_go':result['protoc_gen_go']},ensure_ascii=False))


if __name__=='__main__':main()
