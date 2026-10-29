"""默认只列暂存计划；Root最终明确--apply才按本阶段白名单暂存。"""
from pathlib import Path
import argparse
import hashlib
import json
import runpy
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).resolve().parent
DEV=[sys.executable,str(ROOT/'tools/dev'),'git']

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--apply',action='store_true');args=parser.parse_args()
    definition=runpy.run_path(str(WORK/'verify_index.py'),run_name='only_read_allowlist')
    stage=json.loads((ROOT/definition['FORMAL']).read_text(encoding='utf-8'))
    assert stage['status']=='limited_passed' and stage['runtime']['rule_version']==22 and stage['full_p0_completed'] is False
    assert subprocess.check_output(DEV+['rev-parse','HEAD'],cwd=ROOT).decode().strip()==definition['BASE']
    allowed=definition['ALLOWED']|set(stage['evidence_sha256'])
    release=ROOT/'config/releases'/stage['runtime']['preview'];files=sorted(release.iterdir())
    assert len(files)==5 and all(p.is_file() for p in files)
    allowed|={p.relative_to(ROOT).as_posix() for p in files}
    raw=subprocess.check_output(DEV+['status','--porcelain=v1','--untracked-files=all','-z'],cwd=ROOT);paths=[]
    for item in raw.split(b'\0'):
        if not item:continue
        state,name=item[:2].decode('ascii'),item[3:].decode('utf-8')
        assert all(letter in ' AM?' for letter in state),'拒绝删除/移动/冲突：'+name
        assert name in allowed,'阶段白名单外：'+name
        paths.append(name)
    for name,digest in stage['evidence_sha256'].items():assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==digest,name
    hidden=[name for name in stage['evidence_sha256'] if '/sources/.work/' in name]
    assert hidden and all('/sources/.work/equipment-growth-20261030/' in name and Path(name).suffix in {'.py','.json'} for name in hidden)
    if args.apply:
        for offset in range(0,len(paths),40):subprocess.run(DEV+['add','--',*paths[offset:offset+40]],cwd=ROOT,check=True)
        subprocess.run(DEV+['add','-f','--',*hidden],cwd=ROOT,check=True)
    print(json.dumps({'applied':args.apply,'visible_paths':paths,'hidden_archived_work_sources':hidden,'extra_paths_allowed':False},ensure_ascii=False))

if __name__=='__main__':main()
