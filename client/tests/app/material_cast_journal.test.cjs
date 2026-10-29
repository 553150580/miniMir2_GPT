const {test}=require('node:test');
const assert=require('node:assert/strict');
const {Long,load,entry}=require('./helpers/session_harness.cjs');
const model=name=>require('../../../.work/ui-tests/app/'+name+'.js');
const models=model('SkillModel');
const {PendingOperations}=load('client/assets/scripts/app/PendingOperations.ts',id=>id==='long'?Long:{});
const connection=load('client/assets/scripts/app/GameConnection.ts',id=>id==='long'?Long:id==='../generated/protobuf/client_pb.js'?require('../../assets/scripts/generated/protobuf/client_pb.js')
 :id==='./InputTrace'?{inputTraceMessage:()=>null,traceInput(){}}:id==='./GameErrors'?{protocolErrorMessage:()=>''}:{});
class Storage{constructor(){this.data=new Map();this.writes=0;this.fail=false;}getItem(k){return this.data.get(k)??null;}setItem(k,v){if(this.fail)throw Error('落盘失败');this.data.set(k,v);this.writes++;}removeItem(k){this.data.delete(k);}}
const decorators=new Proxy({},{get(){return (...args)=>args.length>1?undefined:(...value)=>value.length===1?value[0]:undefined;}});
const flush=()=>new Promise(resolve=>setImmediate(resolve));

async function fixture(skillId){
 const oldSocket=global.WebSocket,oldLocation=global.location,sockets=[];
 class Socket{static OPEN=1;constructor(){this.readyState=0;this.sent=[];sockets.push(this);}send(bytes){this.sent.push(bytes);this.onPacket?.(connection.decodePacket(bytes),bytes);}close(){this.readyState=3;this.onclose?.();}}
 global.WebSocket=Socket;global.location={protocol:'http:',host:'127.0.0.1:18080'};
 const h=load('client/assets/scripts/app/GameSession.ts',id=>id==='long'?Long:id==='./GameConnection'?connection:id==='./GameErrors'?{protocolErrorMessage:()=>''}
  :['./BaselineAssembler','./TeleportModel','./RoleBindingModel','./MapTravelModel','./SkillModel','./PetBornModel'].includes(id)?model(id.slice(2)):{});
 const s=h.session;s.role={job:2,key:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(9)}};
 const connecting=s.connection.connect('纯组件测试票'),socket=sockets[0];socket.readyState=1;socket.onopen();await Promise.resolve();
 const deliver=(id,seq,value)=>{const bytes=connection.encodePacket(id,seq,Long.UONE,value);socket.onmessage({data:bytes.buffer});};
 const e=entry('9007199254740993','1');e.resourceVersion=Long.fromInt(18,true);deliver(20002,connection.decodePacket(socket.sent[0]).seq,e);await connecting;deliver(6,0,e);
 const definition={id:skillId,name:'材料技能',job_id:2,book_item_id:25,learn_level:19,initial_level:0,max_level:3,kind:skillId===17?'summon_pet':skillId===14?'soul_shield':'blessed_armour',target_kind:skillId===17?'friendly_self':'friendly_grid',active:true,activation_kind:'cast',range:9,base_cost:12,level_cost:4,cast_ms:600,material_item_id:187,material_quantity:1,area_radius:3};
 const supply={instanceId:new Uint8Array(16).fill(7),itemId:187,version:Long.fromString('9007199254740995',true),quantity:Long.fromInt(2),bound:true};
 s.inventory={revision:Long.fromString('9007199254740994',true),level:19,items:[],equipped:[{slot:100,item:supply}],learnedSkills:[{skillId,level:0}]};
 s.baseline={entities:[{entityHandle:e.playerHandle,entityKind:1,life:Long.MAX_UNSIGNED_VALUE,hp:Long.fromInt(100),mp:Long.fromInt(100),position:{x:8,y:8},direction:5}],ownedPet:null};
 const storage=new Storage(),key='qa.material.account';s.pending=new PendingOperations(storage,key);
 const {AppRoot}=load('client/assets/scripts/app/AppRoot.ts',id=>id==='cc'?{_decorator:decorators,Component:class{}}:id==='long'?Long:id==='./GameSession'?h:id==='./SkillModel'?models:id==='./CastingSupplyModel'?model('CastingSupplyModel'):id==='../input/NavigationIntent'?require('../../../.work/ui-tests/input/NavigationIntent.js'):id==='./MapAttachmentCoordinator'?model('MapAttachmentCoordinator'):{});
 const app=new AppRoot();app.world={gameplay:{skills:[definition],items:[],casting_supply:{enabled:true,slot:100,item_ids:[187]},map_catalog:[{id:'1',no_pets:false}]},map:{width:17,height:17},showAim(){}};
 app.find=()=>({active:false,getComponent:()=>({string:''})});app.ui={top:null};app.status=()=>{};app.error=error=>{throw error;};
 return {h,s,app,storage,key,socket,deliver,supply,row:()=>models.skillRows(app.skillSnapshot())[0],close(){s.connection.close();global.WebSocket=oldSocket;global.location=oldLocation;}};
}
test('真实Session→AppRoot新召17→持久未决→生产GameConnection发40014，不补客户端坐标（socket替身，非真实WS）',async()=>{
 const f=await fixture(17);
 try{
  let stored;
  f.socket.onPacket=packet=>{if(packet.id!==40014)return;stored=new PendingOperations(f.storage,f.key).get(f.h.roleIdentity());assert(stored,'发送之前必须已经落盘');assert(!('targetHandle'in stored.value));assert(!('targetGrid'in stored.value));assert.equal(stored.value.sceneEpoch.toString(),'9007199254740993');assert.equal(stored.value.casterLife.toString(),Long.MAX_UNSIGNED_VALUE.toString());
   assert.equal(packet.value.targetHandle,0);assert.equal(packet.value.targetGrid,null);assert.equal(packet.value.direction,5);assert.equal(packet.value.materialVersion.toString(),'9007199254740995');
   f.deliver(40005,packet.seq,{opId:packet.value.operation.opId,pending:false,errorCode:0,commandSeq:packet.value.operation.commandSeq});};
  await f.app.castSkill(f.row());assert(stored);assert.equal(f.socket.sent.map(connection.decodePacket).filter(p=>p.id===40014).length,1);assert.equal(f.s.pending.get(f.h.roleIdentity()),null);assert.equal(f.supply.quantity.toString(),'2');
 }finally{f.close();}
});
test('17未知发送保留无坐标原请求；重建持久日志后只重发原op/Long/材料，不新分经济序号',async()=>{
 const f=await fixture(17);
 try{
  f.socket.onPacket=packet=>{if(packet.id===40014)throw Error('模拟发送结果未知');};await assert.rejects(f.app.castSkill(f.row()),/未知/);
  const original=f.s.pending.get(f.h.roleIdentity());assert(original);assert(!('targetGrid'in original.value));const command=f.s.command.toString();f.s.pending=new PendingOperations(f.storage,f.key);
  f.socket.onPacket=packet=>{if(packet.id===40014)f.deliver(40005,packet.seq,{opId:packet.value.operation.opId,pending:false,errorCode:0,commandSeq:packet.value.operation.commandSeq});};await f.app.submitPending();
  const packets=f.socket.sent.map(connection.decodePacket).filter(p=>p.id===40014);assert.equal(packets.length,2);
  assert.deepEqual(connection.encodePacket(40014,0,Long.UONE,packets[0].value),connection.encodePacket(40014,0,Long.UONE,packets[1].value),'忽略传输request号后，含原经济op的完整PB请求必须逐字一致');
  assert.deepEqual(packets[1].value.operation.opId,original.value.operation.opId);assert.equal(f.s.command.toString(),command);assert.equal(f.s.pending.get(f.h.roleIdentity()),null);
 }finally{f.close();}
});
for(const skill of [14,15])test(`旧${skill}仍从材料瞄准经真实日志和编码发送原grid，不因17分支放松字段`,async()=>{
 const f=await fixture(skill);
 try{
  f.socket.onPacket=packet=>{if(packet.id!==40014)return;const stored=new PendingOperations(f.storage,f.key).get(f.h.roleIdentity());assert.deepEqual(stored.value.targetGrid,{x:9,y:8});assert.equal(stored.value.targetHandle,0);assert.deepEqual(packet.value.targetGrid,{x:9,y:8});assert.equal(packet.value.skillId,skill);f.deliver(40005,packet.seq,{opId:packet.value.operation.opId,pending:false,errorCode:0,commandSeq:packet.value.operation.commandSeq});};
  f.app.castSkill(f.row());assert(f.app.aim);assert.equal(f.app.consumeGround({x:9,y:8}),true);await flush();assert.equal(f.socket.sent.map(connection.decodePacket).filter(p=>p.id===40014).length,1);assert.equal(f.s.pending.get(f.h.roleIdentity()),null);
 }finally{f.close();}
});
test('17实际持久存储失败仍禁止发送，命令推测回退；不绕过原Pending',async()=>{
 const f=await fixture(17);
 try{f.storage.fail=true;const before=f.s.command.toString();await assert.rejects(f.app.castSkill(f.row()),/保存未决操作失败/);assert.equal(f.s.command.toString(),before);assert.equal(f.socket.sent.map(connection.decodePacket).filter(p=>p.id===40014).length,0);assert.equal(f.s.pending.get(f.h.roleIdentity()),null);}finally{f.close();}
});
