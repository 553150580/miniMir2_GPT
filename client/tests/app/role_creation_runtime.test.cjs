const {test}=require('node:test'),assert=require('node:assert/strict');
const {load,Long}=require('./helpers/session_harness.cjs');
const {PendingRoleCreation}=load('client/assets/scripts/app/PendingRoleCreation.ts',id=>id==='long'?Long:{});
const compiled=name=>require('../../../.work/ui-tests/'+name+'.js');
const {CodeRoleSlotOrNameOccupied}=compiled('generated/protocol_ids');
const errors=compiled('app/GameErrors');
class Storage{constructor(){this.data=new Map();this.failWrite=false;this.failRead=false;}getItem(k){if(this.failRead)throw Error('读失败');return this.data.get(k)??null;}setItem(k,v){if(this.failWrite)throw Error('写失败');this.data.set(k,v);}removeItem(k){if(this.failWrite)throw Error('写失败');this.data.delete(k);}}
class ProtocolResponseError extends Error{constructor(messageId,code){super('明确协议拒绝'+code);this.messageId=messageId;this.code=code;}}
const decorators=new Proxy({},{get(){return(...args)=>args.length>1?undefined:(...v)=>v.length===1?v[0]:undefined;}});
const deferred=()=>{let resolve,reject;const promise=new Promise((yes,no)=>{resolve=yes;reject=no;});return{promise,resolve,reject};};
const hex=bytes=>Buffer.from(bytes).toString('hex');
function fingerprint(q){return JSON.stringify({origin:q.originServerId.toString(),slot:q.slot,job:q.job,name:q.name,op:hex(q.opId)});}

function fixture(storage=new Storage(),key='create.account-a.1'){
 const server=[],receipts=new Map(),requests=[],handlers=new Map(),actions=[],scenes=[];let allocated=0,directoryReads=0,failDirectory=false,disposedAssets=0;
 const account={id:'ab'.repeat(16),name:'account_a'}, ticketCalls=[];
 const session={account,roles:[],connection:{token:'account-a-token',close(){}},roleCreation:new PendingRoleCreation(storage,key),scenes:{async switchTo(name){scenes.push(name);}}};
 session.connection.rpc=async(id,q)=>{
  if(id===10003){directoryReads++;if(failDirectory){failDirectory=false;throw Error('目录响应丢失');}return{roles:server.slice()};}
  if(id===10001){return{accountId:new Uint8Array(16).fill(q.account===account.name?0xab:0xcd),accountToken:'new-token'};}
  if(id===10007){ticketCalls.push(q.role);throw new ProtocolResponseError(10008,1002);}
  assert.equal(id,10005);assert.equal(fingerprint(session.roleCreation.get()),fingerprint(q),'真实RPC发送前原请求已落盘');requests.push(q);
  const action=actions.shift();if(action?.before)await action.before.promise;if(action?.error)throw action.error;
  const idKey=hex(q.opId);let role=receipts.get(idKey);
  if(!role){if(server.some(row=>row.slot===q.slot))throw new ProtocolResponseError(10006,4301);role={key:{originServerId:q.originServerId,roleId:new Uint8Array(16).fill(server.length+1)},slot:q.slot,job:q.job,name:q.name.normalize('NFKC'),level:1};server.push(role);receipts.set(idKey,role);}
  if(action?.drop)throw Error('建角提交后响应丢失');if(action?.directory)failDirectory=true;
  return action?.reply??role;
 };
 const cc={_decorator:decorators,Component:class{},sys:{localStorage:storage}};
 const connection={newOperationId:()=>new Uint8Array(16).fill(++allocated),ProtocolResponseError};
 const lookup=id=>id==='./GameConnection'?connection:id==='./GameErrors'?errors:id==='../generated/protocol_ids'?compiled('generated/protocol_ids'):{};
 const roleEntry=load('client/assets/scripts/app/RoleEntry.ts',lookup);
 const {AppRoot}=load('client/assets/scripts/app/AppRoot.ts',id=>id==='cc'?cc:id==='long'?Long:id==='./GameSession'?{session}:id==='./RoleEntry'?roleEntry:id==='./GameReconnect'?{accountIdentity:hex}:id==='./PendingOperations'?{PendingOperations:class{}}:id==='./PendingRoleCreation'?{PendingRoleCreation}:id==='../input/NavigationIntent'?compiled('input/NavigationIntent'):id==='./WorldAssets'?{WorldAssets:{async prepare(){return{dispose(){disposedAssets++;}};}}}:lookup(id));
 function app(){const result=new AppRoot();result.button=(path,fn)=>handlers.set(path,fn);result.form={Name:{string:'原角色Ａ'},Account:{string:''},Password:{string:''}};result.edit=name=>result.form[name];result.text=()=>{};result.status=message=>{result.message=message;};return result;}
 return{session,server,requests,actions,storage,key,app,scenes,ticketCalls,click:(name='Create')=>handlers.get(name)(),get disposedAssets(){return disposedAssets;},get directoryReads(){return directoryReads;},get allocated(){return allocated;}};
}

test('真实选角建角：提交后丢响应沿原op/槽/职业/姓名重查，改输入不替换，目录确认才清journal',async()=>{
 const f=fixture(),app=f.app();await app.roleScene();f.actions.push({drop:true});await assert.rejects(f.click(),/响应丢失/);
 const saved=f.session.roleCreation.get();assert(saved);assert.equal(f.server.length,1);assert.equal(f.session.roles.length,0);
 app.selectedJob=2;app.edit=()=>({string:'新输入不应发出'});await f.click();
 assert.equal(f.requests.length,2);assert.equal(fingerprint(f.requests[0]),fingerprint(f.requests[1]));assert.equal(f.allocated,1);
 assert.equal(f.server.length,1);assert.equal(f.session.roles[0].name,'原角色A');assert.equal(app.selectedRole,0);assert.equal(f.session.roleCreation.get(),null);
});

test('建角成功后目录失败保留原请求；组件并发点击共用一个任务，避免双发',async()=>{
 const f=fixture(),app=f.app();await app.roleScene();const gate=deferred();f.actions.push({before:gate,directory:true});
 const first=f.click(),second=f.click();assert.equal(first,second);assert.equal(f.requests.length,1);gate.resolve();
 await assert.rejects(first,/目录响应丢失/);assert.equal(f.requests.length,1);assert(f.session.roleCreation.get());
 await f.click();assert.equal(f.requests.length,2);assert.equal(fingerprint(f.requests[0]),fingerprint(f.requests[1]));assert.equal(f.server.length,1);assert.equal(f.session.roleCreation.get(),null);
});

test('新选角组件读取原持久建角，自动沿原op核对；恢复不要求旧输入仍在场景',async()=>{
 const f=fixture(),old=f.app();await old.roleScene();f.actions.push({drop:true});await assert.rejects(f.click());const original=fingerprint(f.session.roleCreation.get());
 old.disposed=true;f.session.roleCreation=new PendingRoleCreation(f.storage,f.key);const next=f.app();next.edit=()=>{throw Error('重查不读取新表单');};await next.roleScene();
 assert.equal(f.allocated,1);assert.equal(fingerprint(f.requests[1]),original);assert.equal(f.server.length,1);assert.equal(next.selectedRole,0);assert.equal(f.session.roleCreation.get(),null);
});

test('旧建角和目录异步结果不得更新销毁场景、新账号或新ledger，原账号journal保留待核对',async()=>{
 for(const mode of ['destroy','account','ledger']){
  const f=fixture(),app=f.app();await app.roleScene();const gate=deferred();f.actions.push({before:gate});const pending=f.click(),originalLedger=f.session.roleCreation;
  if(mode==='destroy')app.disposed=true;
  if(mode==='account'){f.session.connection.token='account-b-token';f.session.roleCreation=new PendingRoleCreation(f.storage,'create.account-b.1');}
  if(mode==='ledger')f.session.roleCreation=new PendingRoleCreation(f.storage,f.key);
  gate.resolve();
  await pending;assert.equal(f.session.roles.length,0,mode);assert.equal(f.directoryReads,1,mode);assert(originalLedger.get(),mode);
 }
 const f=fixture(),app=f.app(),gate=deferred();f.session.connection.rpc=()=>gate.promise;const pending=app.fetchRoles();f.session.connection.token='other';gate.resolve({roles:[{slot:1,name:'旧账号角色'}]});assert.equal(await pending,false);assert.equal(f.session.roles.length,0);
});

test('明确首次业务拒绝结束原请求；繁忙、未知后的拒绝、坏回执均保持原身份',async()=>{
 const f=fixture(),app=f.app();await app.roleScene();f.actions.push({error:new ProtocolResponseError(10006,4301)});await assert.rejects(f.click());assert.equal(f.session.roleCreation.get(),null);
 f.actions.push({error:new ProtocolResponseError(10006,1003)});await assert.rejects(f.click());const original=fingerprint(f.session.roleCreation.get());
 assert.equal(f.session.connection.token,'account-a-token');assert.deepEqual(f.scenes,[]);
 f.actions.push({error:new ProtocolResponseError(10006,4301)});await assert.rejects(f.click());assert.equal(fingerprint(f.session.roleCreation.get()),original);
 f.actions.push({reply:{slot:4}});await assert.rejects(f.click(),/身份不匹配/);assert.equal(fingerprint(f.session.roleCreation.get()),original);
 await f.click();assert.equal(f.server.length,1);assert.equal(f.session.roleCreation.get(),null);
});

const displayRole=(slot)=>({key:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(slot)},slot,job:0,name:`角色${slot}`,level:1});

test('点击进入遇到10008认证拒绝，重新登录同账号恢复原角色身份，不自动继续入场',async()=>{
 const f=fixture(),app=f.app();f.server.push(displayRole(1),displayRole(2));await app.roleScene();f.click('Role1');
 const pendingKey=`minimir.pending.v1.${f.session.account.id}.1`;f.storage.setItem(pendingKey,'原经济记录');
 await f.click('Enter');assert.deepEqual(f.scenes,['Login']);assert.equal(f.disposedAssets,1);
 assert.equal(f.session.connection.token,'');assert.equal(f.session.ready,false);assert.equal(f.session.role,null);
 assert.equal(f.storage.getItem(pendingKey),'原经济记录');assert.equal(f.ticketCalls.length,1);assert.equal(f.ticketCalls[0],f.server[1].key);
 const login=f.app();login.loginScene();assert.equal(login.form.Account.string,'account_a');assert.equal(login.form.Password.string,'');assert.match(login.message,/重新登录/);
 login.form.Password.string='history_pass';await f.click('Login');assert.equal(login.form.Password.string,'');assert.deepEqual(f.scenes,['Login','RoleSelect']);
 f.server.reverse();const resumed=f.app();await resumed.roleScene();assert.equal(resumed.selectedRole,0);assert.equal(f.session.roles[resumed.selectedRole].slot,2);assert.equal(f.ticketCalls.length,1);
});

test('目录及建角认证拒绝返回登录，并按原样保存未决建角记录',async()=>{
 const directory=fixture(),first=directory.app();directory.session.connection.rpc=async()=>{throw new ProtocolResponseError(10004,1002);};
 await first.roleScene();assert.deepEqual(directory.scenes,['Login']);assert.equal(directory.session.connection.token,'');
 const create=fixture(),second=create.app();await second.roleScene();create.actions.push({error:new ProtocolResponseError(10006,1002)});
 await create.click();assert.deepEqual(create.scenes,['Login']);assert.equal(create.session.connection.token,'');
 const saved=new PendingRoleCreation(create.storage,create.key).get();assert(saved);assert.equal(fingerprint(saved),fingerprint(create.requests[0]));
});

test('旧凭证的迟到1002不可退出新登录，切换账号也不沿用原账号角色选择',async()=>{
 const f=fixture(),app=f.app();f.server.push(displayRole(1),displayRole(2));await app.roleScene();f.click('Role1');
 const gate=deferred();f.session.connection.rpc=()=>gate.promise;const stale=f.click('Enter');
 await Promise.resolve();const account=f.session.account;f.session.account={id:'another',name:'another_name'};
 gate.reject(new ProtocolResponseError(10008,1002));await stale;
 assert.equal(f.session.connection.token,'account-a-token');assert.deepEqual(f.scenes,[]);assert.equal(f.disposedAssets,1);
 const fresh=fixture(),old=fresh.app();fresh.server.push(displayRole(1),displayRole(2));await old.roleScene();fresh.click('Role1');await fresh.click('Enter');
 const login=fresh.app();login.loginScene();login.form.Account.string='account_b';login.form.Password.string='new_pass';await fresh.click('Login');
 const roles=fresh.app();await roles.roleScene();assert.equal(roles.selectedRole,0);assert.notEqual(fresh.session.account.id,account.id);
});

test('重入后收到原建角确定占用回执，解除旧记录并允许按新输入建角',async()=>{
 const f=fixture(),old=f.app();await old.roleScene();
 f.actions.push({error:Error('原建角响应丢失')});await assert.rejects(f.click(),/响应丢失/);
 const original=fingerprint(f.session.roleCreation.get());
 f.actions.push({error:new ProtocolResponseError(10008,CodeRoleSlotOrNameOccupied)});
 await assert.rejects(f.click());assert.equal(fingerprint(f.session.roleCreation.get()),original);
 old.disposed=true;
 const next=f.app();next.edit=()=>({string:'新的角色名'});
 f.actions.push({error:new ProtocolResponseError(10006,CodeRoleSlotOrNameOccupied)});
 await assert.rejects(next.roleScene(),error=>error.code===CodeRoleSlotOrNameOccupied);
 assert.equal(fingerprint(f.requests[2]),original);assert.equal(f.session.roleCreation.get(),null);assert.equal(f.allocated,1);
 await f.click();assert.equal(f.allocated,2);assert.equal(f.server[0].name,'新的角色名');assert.equal(f.session.roleCreation.get(),null);
});

test('建角journal按账号隔离、严格校验且先落盘；写失败/损坏不得发送或覆盖原记录',async()=>{
 const f=fixture(),app=f.app();await app.roleScene();f.storage.failWrite=true;await assert.rejects(f.click(),/尚未发送/);assert.equal(f.requests.length,0);
 f.storage.failWrite=false;const request={originServerId:Long.UONE,slot:1,job:0,name:'角色',opId:new Uint8Array(16).fill(7)};f.session.roleCreation.set(request);
 request.opId.fill(9);assert.equal(hex(f.session.roleCreation.get().opId),'07'.repeat(16));assert.throws(()=>f.session.roleCreation.set(request),/不能更换/);
 assert.equal(new PendingRoleCreation(f.storage,'create.account-b.1').get(),null);assert.throws(()=>f.session.roleCreation.clear(request.opId),/其它操作/);
 f.storage.data.set(f.key,'{"version":1}');await assert.rejects(f.click(),/记录损坏/);assert.equal(f.requests.length,0);assert.equal(f.storage.getItem(f.key),'{"version":1}');
});
