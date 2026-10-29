"""只读核实首轮超长库名没有创建数据库；保留原失败报告，不执行清理写入。"""
from pathlib import Path
import hashlib,json,sys
ROOT=Path(__file__).resolve().parents[2];WORK=Path(__file__).parent
sys.path.insert(0,str(ROOT/'tools/environment'))
import mongodb_windows
from pymongo import MongoClient

failed=WORK/'gui/run-6d49a00ee9514431b6ffa6bf30846cd8/report.json'
before=failed.read_bytes();report=json.loads(before)
name=report['database']
assert not report['passed'] and len(name.encode())==65 and name=='minimir_dev_game_test_pet_visual_'+report['run_id']
state=mongodb_windows.status()
cfg=json.loads((ROOT/'conf/dev/server.json').read_text(encoding='utf-8'))
assert state['running'] and state['endpoint_verified'] and state['uri']==cfg['mongo_uri_by_platform']['windows']
with MongoClient(state['uri'],serverSelectionTimeoutMS=3000) as client:
    exists=name in client.list_database_names()
assert not exists
assert failed.read_bytes()==before
result={'passed':True,'read_only':True,'target_database':name,'name_bytes':65,'maximum_name_bytes':63,
        'database_exists':False,'database_dropped':False,'database_or_user_data_modified':False,
        'original_failed_report':failed.relative_to(ROOT).as_posix(),'original_report_sha256':hashlib.sha256(before).hexdigest(),
        'original_failure_report_preserved':True,'scope':'仅证明被Mongo命名校验拒绝的65字节名称当前不存在；不改写原QA失败/清理错误，不将其改成GUI成功'}
(WORK/'gui-invalid-name-followup.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n',encoding='utf-8',newline='\n')
print(json.dumps(result,ensure_ascii=False))
