#!/usr/bin/env python3
"""仅管理本项目本机开发装配；Mongo由tools/environment/mongodb_dev.py独立管理。"""
from __future__ import annotations
import argparse, hashlib, json, os, signal, socket, subprocess, sys, time, urllib.request, uuid
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'tools'))
from process_control import background_options, windows_identity, windows_owned, windows_stop
STATE=ROOT/'server/.dev/runtime.json'
BINARY=ROOT/('server/bin/nbserver.exe' if os.name=='nt' else 'server/bin/nbserver')
CONFIG=ROOT/'conf/dev/server.json'

def process(pid):
    p=subprocess.run(['ps','-p',str(pid),'-o','command='],capture_output=True,text=True)
    return p.stdout.strip() if p.returncode==0 else ''
def owned(meta):
    if os.name=='nt':return bool(meta and meta.get('project_root')==str(ROOT) and windows_owned(meta,BINARY))
    command=process(meta.get('pid',0))
    expected=[str(BINARY),'-config',str(CONFIG),'-mode','supervisor','-run-id',meta.get('run_id','')]
    return bool(command and meta.get('run_id') and command==' '.join(expected))
def read():
    if not STATE.exists(): return None
    if STATE.is_symlink(): raise RuntimeError('拒绝运行状态符号链接')
    data=json.loads(STATE.read_text())
    if data.get('project_root')!=str(ROOT) and os.name!='nt':raise RuntimeError('运行状态不属于本项目')
    return data
def status():
    meta=read(); running=bool(meta and owned(meta)); health={}
    if running:
        for mode,port in [('account',18100),('game',18101),('gm',18102)]:
            try:
                with urllib.request.urlopen(f'http://127.0.0.1:{port}/healthz',timeout=1) as r: health[mode]=json.load(r)
            except Exception as e: health[mode]={'status':'unavailable','error':str(e)}
    return {'running':running,'pid':meta.get('pid') if running else None,'run_id':meta.get('run_id') if meta else None,'health':health,'development_only':True,'production_ready':False}
def start():
    meta=read()
    if meta and meta.get('project_root')!=str(ROOT):
        # 只在显式启动时归档复制来的状态；status/stop不改旧状态，也不使用其中的PID。
        archive=STATE.parent/'imported';archive.mkdir(exist_ok=True)
        STATE.rename(archive/(str(uuid.uuid4())+'-runtime.json'));meta=None
    if meta and owned(meta): return status()
    for port in [18100,18101,18102]:
        with socket.socket() as s:
            # 与Go net.Listen一致：允许已关闭连接的TIME_WAIT，仍拒绝活动监听。
            s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
            try: s.bind(('127.0.0.1',port))
            except OSError: raise RuntimeError(f'端口{port}已占用；不会接管或停止未知进程')
    subprocess.run([sys.executable,str(ROOT/'tools/dev'),'go','build','-o',str(BINARY),'./cmd/nbserver'],cwd=ROOT,check=True)
    run=str(uuid.uuid4());directory=ROOT/'server/.dev/runs'/run;directory.mkdir(parents=True,mode=0o700)
    with (directory/'server.log').open('ab') as log:
        child=subprocess.Popen([str(BINARY),'-config',str(CONFIG),'-mode','supervisor','-run-id',run],cwd=ROOT,stdin=subprocess.DEVNULL,stdout=log,stderr=subprocess.STDOUT,close_fds=True,**background_options())
    meta={'schema_version':1,'project_root':str(ROOT),'run_id':run,'pid':child.pid,'binary_sha256':hashlib.sha256(BINARY.read_bytes()).hexdigest(),'log':str(directory/'server.log'),'started_at_unix':int(time.time())}
    if os.name=='nt':meta.update(windows_identity(child.pid) or {})
    STATE.parent.mkdir(parents=True,exist_ok=True)
    tmp=STATE.with_suffix('.tmp');tmp.write_text(json.dumps(meta,ensure_ascii=False,indent=2)+'\n');tmp.chmod(0o600);tmp.replace(STATE)
    for _ in range(100):
        result=status()
        if not result['running']: raise RuntimeError(f'装配启动失败，请查 {meta["log"]}')
        if len(result['health'])==3 and all(h.get('status')=='ok' for h in result['health'].values()): return result
        time.sleep(.1)
    raise RuntimeError(f'装配未在10秒内就绪，请查 {meta["log"]}')
def stop():
    meta=read()
    if not meta or not owned(meta): return status()
    if os.name=='nt':windows_stop(meta,BINARY)
    else:os.kill(meta['pid'],signal.SIGTERM)
    for _ in range(150):
        if not owned(meta): return status()
        time.sleep(.1)
    # 不发SIGKILL：不能在未知提交边界直接停止数据库资产任务。
    raise RuntimeError('受控节点未完成优雅停止；保留原进程与日志以核查，不强杀')
def main():
    parser=argparse.ArgumentParser();parser.add_argument('action',choices=['start','stop','status']);args=parser.parse_args()
    try: print(json.dumps(globals()[args.action](),ensure_ascii=False,indent=2))
    except Exception as e: print(json.dumps({'error':str(e)},ensure_ascii=False),file=sys.stderr);raise SystemExit(1)
if __name__=='__main__':main()
