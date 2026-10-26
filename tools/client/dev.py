"""管理本任务的回环静态/代理服务；只停止PID和实例标记均匹配的本进程。"""
from pathlib import Path
import json,os,signal,subprocess,sys,time,uuid,urllib.request,urllib.error
ROOT=Path(__file__).resolve().parents[2]
STATE=ROOT/'.work/client-dev/state.json'
def health():
    try:
        with urllib.request.urlopen('http://127.0.0.1:18080/healthz',timeout=1) as response:return json.load(response)
    except urllib.error.HTTPError as response:
        try:return json.load(response)
        except ValueError:return None
    except (OSError,ValueError):return None
def read():
    return json.loads(STATE.read_text()) if STATE.is_file() else None
def owns(state):
    if not state:return False
    result=subprocess.run(['ps','-p',str(state['pid']),'-o','command='],capture_output=True,text=True)
    command=result.stdout.strip()
    return result.returncode==0 and 'tools/client/serve.mjs' in command and '--instance-id '+state['run_id'] in command
def main():
    command=sys.argv[1] if len(sys.argv)>1 else 'status';state=read();current=health()
    if command=='status':print(json.dumps({'running':owns(state),'health':current},ensure_ascii=False));return
    if command=='stop':
        if not owns(state):print('没有可停止的本任务客户端服务');return
        os.kill(state['pid'],signal.SIGTERM)
        for _ in range(30):
            if not owns(state):break
            time.sleep(.1)
        if owns(state):raise RuntimeError('本进程仍未退出，请检查开发日志；未停止其它进程')
        STATE.unlink(missing_ok=True);print('客户端开发服务已停止');return
    if command!='start':raise RuntimeError('只支持start/stop/status')
    if owns(state):print(json.dumps({'running':True,'health':current},ensure_ascii=False));return
    if current is not None:raise RuntimeError('18080已有未登记服务，拒绝接管或停止')
    STATE.parent.mkdir(parents=True,exist_ok=True);run_id=uuid.uuid4().hex
    with (STATE.parent/'server.log').open('a') as output:
        process=subprocess.Popen([str(ROOT/'tools/dev'),'node','tools/client/serve.mjs','--instance-id',run_id],cwd=ROOT,stdout=output,stderr=subprocess.STDOUT,start_new_session=True)
    state={'pid':process.pid,'run_id':run_id};STATE.write_text(json.dumps(state)+'\n')
    for _ in range(50):
        current=health()
        if current and current.get('run_id')==run_id and owns(state):print(json.dumps(current,ensure_ascii=False));return
        if process.poll() is not None:raise RuntimeError('客户端服务启动失败，见.work/client-dev/server.log')
        time.sleep(.1)
    raise RuntimeError('启动就绪检查超时，未停止其它进程')
if __name__=='__main__':
    try:main()
    except (OSError,ValueError,RuntimeError) as error:print(str(error),file=sys.stderr);sys.exit(1)
