"""独立核对本阶段没有改写来源地图/美术及已保存固定布局。"""
from pathlib import Path
import hashlib,json,subprocess
ROOT=Path(__file__).resolve().parents[2];BASE=Path(__file__).parent
sha=lambda b:hashlib.sha256(b).hexdigest()
previous=json.loads((ROOT/'outputs/source-town-foundation-20261029/resources/artifact-inventory.json').read_text(encoding='utf-8'))
for file,record in previous.items():
 assert sha((ROOT/file).read_bytes())==record['sha256'],file
layouts=sorted([*ROOT.glob('client/assets/**/*.scene'),*ROOT.glob('client/assets/**/*.prefab')])
layout_hashes={}
for path in layouts:
 relative=path.relative_to(ROOT).as_posix()
 old=subprocess.run(['git','show','9b7a4d5bc86d956d3b3bdac7410e1b28efdf3f81:'+relative],cwd=ROOT,stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True).stdout
 assert path.read_bytes()==old,relative
 layout_hashes[relative]=sha(old)
assert len(layout_hashes)==10
report={'passed':True,'source_artifact_files':len(previous),'source_artifact_bytes':sum(record['bytes'] for record in previous.values()),'source_artifacts_compared_against':'outputs/source-town-foundation-20261029/resources/artifact-inventory.json','all_source_bytes_unchanged':True,'layouts_compared_against':'9b7a4d5bc86d956d3b3bdac7410e1b28efdf3f81','saved_layouts':layout_hashes,'all_saved_layout_bytes_unchanged':True}
(BASE/'assets-unchanged.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps({'passed':True,'source_files':len(previous),'saved_layouts':len(layout_hashes)}))
