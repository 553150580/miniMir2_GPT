"""仅给正常新建、尚未入场的隔离法师设置学习门槛等级及余额；不写书、技能、绑定或位置。"""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools/environment'))
import mongodb_windows as database


def main():
    if os.name != 'nt':
        raise RuntimeError('仅支持已登记的Windows项目专属开发库')
    request = json.load(sys.stdin)
    account, name = request['account_name'], request['role_name']
    if not re.fullmatch(r'tpq[0-9a-f]{10}', account) or not re.fullmatch(r'TM[0-9a-f]{8}', name):
        raise ValueError('只接受本次正常注册的新隔离账号及法师')
    status = subprocess.run([sys.executable, str(ROOT / 'tools/dev'), 'python3', 'tools/server/dev.py', 'status'], capture_output=True, encoding='utf-8', check=True)
    if json.loads(status.stdout)['running']:
        raise RuntimeError('须先正常停止本项目服务，不能写活动owner')
    endpoint = database.status()
    if not endpoint['running'] or not endpoint['endpoint_verified']:
        raise RuntimeError('项目专属数据库端点未核验')
    config = json.loads((ROOT / 'conf/dev/server.json').read_text(encoding='utf-8'))
    if config['database'] != 'minimir_dev_game' or config['mongo_uri_by_platform']['windows'] != endpoint['uri']:
        raise ValueError('不是本项目专属开发库')
    gameplay = json.loads((ROOT / 'client/assets/resources/development/gameplay.json').read_text(encoding='utf-8'))
    skill = next(row for row in gameplay['skills'] if row['id'] == 21)
    if gameplay['rule_version'] < 15 or skill['job_id'] != 1:
        raise ValueError('当前public未启用受检21合同')
    qa_level = skill['learn_level']
    from pymongo import MongoClient
    with MongoClient(endpoint['uri'], serverSelectionTimeoutMS=3000) as client:
        db = client[config['database']]
        owner = db.accounts.find_one({'name': account})
        if not owner:
            raise ValueError('没有刚正常注册的隔离账号')
        roles = list(db.roles.find({'account_id': owner['_id'], 'name': name}))
        if len(roles) != 1:
            raise ValueError('角色必须唯一')
        role = roles[0]
        if role.get('job') != 1 or role.get('level') != 1 or role.get('learned_skills') or role.get('economic_command_seq') != 0 or role.get('revision') != 1 or role.get('binding') is not None or role.get('binding_source_seq', 0) != 0:
            raise ValueError('角色须为未入场、未学习、未绑定、未经济操作的新法师')
        untouched = {key: value for key, value in role.items() if key not in ('level', 'wallet')}
        changed = db.roles.update_one({'_id': role['_id'], 'revision': 1, 'level': 1, 'economic_command_seq': 0, 'binding': None},
                                     {'$set': {'level': qa_level, 'wallet': {'gold': 1000000, 'bound_gold': 1000000, 'yuanbao_units': 1000000, 'bound_yuanbao_units': 1000000}}})
        if changed.modified_count != 1:
            raise RuntimeError('原角色前态变化，未覆盖')
        after = db.roles.find_one({'_id': role['_id']})
        if untouched != {key: value for key, value in after.items() if key not in ('level', 'wallet')}:
            raise RuntimeError('除等级与余额外的角色字段发生变化')
        evidence = {'role_id_sha256': hashlib.sha256(str(role['_id']).encode()).hexdigest(), 'initial_level': 1, 'qa_level': qa_level,
                    'books_seeded': False, 'skills_seeded': False, 'binding_seeded': False, 'position_seeded': False, 'other_role_fields_preserved': True}
    print(json.dumps({'passed': True, 'isolated_qa_preconditions': True, 'natural_progression_verified': False, 'role': evidence,
                      'scope': '仅隔离角色等级/余额前态；购书、原书学习、Walk绑定和21施放均走正常保存UI与owner链路'}, ensure_ascii=False))


if __name__ == '__main__':
    main()
