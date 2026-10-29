"""只读核对两份外观消费报告及原文件；不认证新装备外观、不执行来源程序。"""
from copy import deepcopy
from datetime import datetime,timezone
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
WORK=Path(__file__).resolve().parent
EXPECTED={'appearance-consumer-review.json':'42ed6b1da64a895ceb806686da82fd0809e80920403a3166b72e34c5ef255e75',
    'appearance-consumer-review-v2.json':'42bd8a9a62aca2432c5750ca0e8fb3e1240332bf5172954532feaead39dea430'}
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def read(path):return json.loads(path.read_text(encoding='utf-8'))
def source(path):
    p=Path(path)
    return p if p.is_absolute() else ROOT/p

reports={};counts={}
for name,digest in EXPECTED.items():
    path=WORK/name;assert sha(path)==digest;report=reports[name]=read(path)
    for name,value in report['source_sha256_current'].items():assert sha(source(name))==value,name
    for row in report['input_reports']:assert sha(ROOT/row['path'])==row['sha256'],row['path']
    for row in report['evidence'].values():
        path=source(row['path']);assert sha(path)==row['sha256']
        lines=path.read_text(encoding='utf-8-sig').splitlines()
        assert 1<=row['line']<=row['end_line']<=len(lines),row['path']
    manifest=report['gomir2_reference_actual_consumer']['observed_manifest'];mp=source(manifest['path'])
    assert sha(mp)==manifest['sha256'];actual=read(mp)['families']['classic']
    assert actual['layers']['weapon']==manifest['weapon_shapes'] and set(actual['layers']['weapon'])=={'0','1'}
    assert actual['gender_offsets']==manifest['gender_offsets'] and actual['actions']==manifest['actions']
    for row in actual['layers']['weapon'].values():
        path=(mp.parent/row['file']).resolve();assert path.is_relative_to(mp.parent.resolve()) and sha(path)==row['sha256']
    counts['v1' if len(reports)==1 else 'v2']=len(report['source_sha256_current'])

v1,v2=[reports[n] for n in EXPECTED]
old=deepcopy(v2)
for key in ['revision','supersedes_interpretation_of_v1','art_and_numeric_source_separation']:old.pop(key)
old['recorded_at_utc']=v1['recorded_at_utc']
old['identity_and_class_boundary']['future_mapping_candidate']=v1['identity_and_class_boundary']['future_mapping_candidate']
old['evidence'].pop('client_art_priority');old['source_sha256_current'].pop('client/AGENTS.md')
assert old==v1,'v2除明确范围澄清外有未审变化'
assert v2['supersedes_interpretation_of_v1']['sha256']==EXPECTED['appearance-consumer-review.json']
markers={
    '资料参考/crystal/Client/MirObjects/PlayerObject.cs':['Libraries.CWeapons[Weapon]','WeaponOffSet = Gender == MirGender.Male ? 0 : 416','DrawFrame + WeaponOffSet'],
    '资料参考/crystal/Client/MirObjects/Frames.cs':['get { return Count + Skip; }'],
    'D:/123DownLoad/GoMir2/client/assets/scripts/core/player-art/model.ts':['f.layers[layer][String(shape)]','action.start + pose.direction * (action.count + action.skip) + frame + f.gender_offsets[layer][look.gender]'],
    'D:/123DownLoad/GoMir2/client/assets/scripts/core/player-art/player-stream.ts':['library.data.frames[String(request.index)]']}
for name,values in markers.items():
    text=source(name).read_text(encoding='utf-8-sig')
    assert all(value in text for value in values),name
result={'status':'read_only_support_source_audit_verified_not_runtime_certified','recorded_at_utc':datetime.now(timezone.utc).isoformat(),
    'reports':{name:sha(WORK/name) for name in EXPECTED},'source_files_sha_verified':counts,'referenced_manifests_verified':1,'referenced_weapon_library_json_sha_verified':2,
    'original_source_reports_preserved':v1['input_reports'],'v1_original_bytes_preserved':True,'v2_only_declared_scope_clarification':True,
    'evidence_agreement':'两套可读消费链均按Shape选择独立库，然后在库内按动作/方向/性别索引；不能据此证明raw WIL乘1200、减1或女600公式',
    'scope_clarification':'还原MYF原外观才需其字段/发布客户端/PAK优先级桥接；本作可另按可追溯目标1.80定义到同参考实际库消费映射登记差异，两条路线本轮均未完整认证',
    'runtime220_enabled':False,'main_runtime_or_excel_changed':False,'source_program_or_go_db_browser_executed':False,
    'limits':'额外support来源研究，不作为stage22主运行/完整1.80/220或三职业完整成长通过证据。未重新探帧/导出资源。'}
path=WORK/'appearance-support-peer.json'
with path.open('x',encoding='utf-8',newline='\n') as out:out.write(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
print(json.dumps({'path':str(path),'sha256':sha(path),'source_files_verified':counts,'main_runtime_modified':False},ensure_ascii=False))
