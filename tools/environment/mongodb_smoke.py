"""复用 Linux/Mac 既有的开发副本集及事务探测，不接入其它数据库。"""
DRIVER_PROGRAM = r'''
import json,sys,time,uuid
from urllib.parse import urlsplit
from pymongo import MongoClient,WriteConcern
from pymongo.errors import OperationFailure,CollectionInvalid
from pymongo.read_concern import ReadConcern
uri,repl,port,database=sys.argv[1:]
bootstrap_uri='mongodb://127.0.0.1:'+str(urlsplit(uri).port)+'/?directConnection=true'
client=MongoClient(bootstrap_uri,directConnection=True,serverSelectionTimeoutMS=3000,connectTimeoutMS=2000)
try:
    try:
        status=client.admin.command('replSetGetStatus')
        if status['set']!=repl:raise RuntimeError('replica set mismatch')
    except OperationFailure as error:
        if error.code!=94:raise
        client.admin.command('replSetInitiate',{'_id':repl,'members':[{'_id':0,'host':'localhost:'+port}]})
    deadline=time.monotonic()+35
    while time.monotonic()<deadline:
        hello=client.admin.command('hello')
        if hello.get('isWritablePrimary') and hello.get('setName')==repl:break
        time.sleep(0.2)
    else:raise RuntimeError('replica set did not elect primary')
    client.close()
    client=MongoClient(uri,directConnection=True,serverSelectionTimeoutMS=5000,connectTimeoutMS=2000)
    config=client.admin.command('replSetGetConfig')['config']
    if len(config['members'])!=1 or config['members'][0]['host']!='localhost:'+port:raise RuntimeError('unexpected replica members')
    # 本项目新空库，仅建空的环境探测集合，不写账号或玩法数据。
    db=client[database]
    try:db.create_collection('_environment')
    except CollectionInvalid:pass
    if db['_environment'].count_documents({})!=0:raise RuntimeError('environment collection contains unexpected data')
    probe_name='minimir_toolchain_probe_'+uuid.uuid4().hex
    probe=client[probe_name]
    probe.create_collection('txn')
    try:
        with client.start_session() as session:
            with session.start_transaction(read_concern=ReadConcern('snapshot'),write_concern=WriteConcern('majority')):
                probe.txn.insert_one({'_id':'commit','value':1},session=session)
                probe.txn.update_one({'_id':'commit'},{'$inc':{'value':1}},session=session)
        if probe.txn.find_one({'_id':'commit'})['value']!=2:raise RuntimeError('commit check failed')
        with client.start_session() as session:
            session.start_transaction()
            probe.txn.insert_one({'_id':'abort'},session=session)
            session.abort_transaction()
        if probe.txn.find_one({'_id':'abort'}) is not None:raise RuntimeError('abort check failed')
    finally:client.drop_database(probe_name)
    print(json.dumps({'replica_initialized':True,'single_member_primary':True,'transaction_commit_verified':True,'transaction_abort_verified':True,'temporary_probe_database_dropped':True,'empty_database':database,'empty_environment_collection_count':0,'server_version':client.server_info()['version']}))
finally:client.close()
'''
