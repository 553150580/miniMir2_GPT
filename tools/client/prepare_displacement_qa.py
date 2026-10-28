"""仅为刚由浏览器新建且尚未入场的隔离QA角色准备等级与余额，不写运行配置或技能。"""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(ROOT/'tools/environment'))
import mongodb_windows as database


def main():
    if os.name!='nt':raise RuntimeError('本工具只使用已登记的Windows专属QA数据库')
    request=json.load(sys.stdin)
    account=request['account_name'];names=request['role_names']
    if not re.fullmatch(r'dqa[0-9a-f]{10}',account) or len(names)!=2 or len(set(names))!=2 or not all(re.fullmatch(r'D[MW][0-9a-f]{8}',name) for name in names):raise ValueError('只接受本次浏览器新建的随机QA账号和角色')
    result=subprocess.run([sys.executable,str(ROOT/'tools/dev'),'python3','tools/server/dev.py','status'],capture_output=True,encoding='utf-8',check=True)
    if json.loads(result.stdout)['running']:raise RuntimeError('必须先正常停止本项目服务，不能修改活动owner数据')
    endpoint=database.status()
    if not endpoint['running'] or not endpoint['endpoint_verified']:raise RuntimeError('专属数据库端点未核验，不连接')
    config=json.loads((ROOT/'conf/dev/server.json').read_text(encoding='utf-8'))
    if config['database']!='minimir_dev_game' or config['mongo_uri_by_platform']['windows']!=endpoint['uri']:raise ValueError('QA数据库不是本项目专属默认开发库')
    from pymongo import MongoClient
    with MongoClient(endpoint['uri'],serverSelectionTimeoutMS=3000) as client:
        db=client[config['database']]
        owner=db.accounts.find_one({'name':account})
        if not owner:raise ValueError('没有刚刚正常注册的QA账号')
        roles=list(db.roles.find({'account_id':owner['_id'],'name':{'$in':names}}))
        if len(roles)!=2 or any(role.get('level')!=1 or role.get('learned_skills') or role.get('economic_command_seq')!=0 or role.get('revision')!=1 for role in roles):raise ValueError('角色必须是新建未学习、未发生经济操作的前态')
        evidence=[]
        for role in roles:
            result=db.roles.update_one({'_id':role['_id'],'revision':1,'level':1,'economic_command_seq':0},
                                      {'$set':{'level':40,'wallet':{'gold':1000000,'bound_gold':1000000,'yuanbao_units':1000000,'bound_yuanbao_units':1000000}}})
            if result.modified_count!=1:raise RuntimeError('QA角色前态变化，未覆盖')
            evidence.append(dict(name=role['name'],role_id_sha256=hashlib.sha256(str(role['_id']).encode()).hexdigest(),initial_level=1,qa_level=40,
                                 learned_skills_seeded=False,books_seeded=False,items_seeded=False))
    print(json.dumps(dict(passed=True,isolated_qa_preconditions=True,natural_progression_verified=False,roles=evidence,
                         scope='只为本次新角色提供等级及余额，后续购书、原书学习和施放均走正常UI/owner链路'),ensure_ascii=False))


if __name__=='__main__':main()
