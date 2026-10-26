"""使用本机Creator构建；只读保护已保存场景/预制体，退出36才表示成功。"""
from pathlib import Path
import hashlib,json,subprocess,sys,time

ROOT=Path(__file__).resolve().parents[2]
APP=Path('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/MacOS/CocosCreator')
def snapshots():
    return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for suffix in ['*.scene','*.prefab'] for p in (ROOT/'client/assets').rglob(suffix)}
def scripts():
    return {str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for suffix in ['*.ts','*.js'] for p in (ROOT/'client/assets/scripts').rglob(suffix)}
before=snapshots(); source_before=scripts(); started=time.time(); (ROOT/'.work').mkdir(exist_ok=True)
log=ROOT/'.work/creator-cli.log'
with (ROOT/'.work/creator-cli.log').open('w') as output:
    result=subprocess.run([str(APP),'--project',str(ROOT/'client'),'--build','configPath='+str(ROOT/'conf/creator-build.json'),'--log-dest',str(log)],stdout=output,stderr=subprocess.STDOUT,cwd=ROOT)
after=snapshots()
protected=before==after
source_after=scripts();stable=source_before==source_after
success=result.returncode==36 and (ROOT/'client/build/web-mobile/index.html').is_file() and protected and stable
report=dict(creator='3.8.8',platform='web-mobile',exit_code=result.returncode,success=success,scene_prefab_layout_preserved=protected,source_stable_during_build=stable,asset_hashes=after,script_hashes=source_after,seconds=round(time.time()-started,2),log=str(log),physical_mobile_verified=False)
(ROOT/'docs/客户端构建检查.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({k:v for k,v in report.items() if k not in ['asset_hashes','script_hashes']},ensure_ascii=False))
sys.exit(0 if success else 1)
