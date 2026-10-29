const {test}=require('node:test');
const assert=require('node:assert/strict');
const {PetBornModel,petAppearDuration}=require('../../../.work/ui-tests/app/PetBornModel.js');
const {Long,harness,entry,ready,load}=require('./helpers/session_harness.cjs');
const role={originServerId:Long.UONE,roleId:new Uint8Array(16).fill(9)},roleKey='1:'+ '09'.repeat(16);
const identity=(changes={})=>({connection:1,binding:'1',role:roleKey,scene:'9007199254740993',baseline:'9007199254740994',map:'2',travelId:'',...changes});
const event=(changes={})=>({role,sceneEpoch:'9007199254740993',baselineId:'9007199254740994',stateSequence:'18446744073709551614',entityHandle:3,entityLife:'9007199254740995',definitionId:1,mapId:'2',sourceCastId:'18446744073709551615',sourceSkillId:17,bornAtUnixMs:'18446744073709551615',movementRevision:'0',direction:5,elapsedMs:0,...changes});
const body=(changes={})=>({entityHandle:3,entityKind:2,definitionId:1,life:'9007199254740995',hp:'10',movementRevision:'0',direction:5,...changes});
const baseline=(changes={})=>({sceneEpoch:'9007199254740993',baselineId:'9007199254740994',stateSequence:'18446744073709551614',entities:[body()],...changes});
function model(){const m=new PetBornModel();m.bind(identity());return m;}
test('首次基线不是新生；高64位真实出生匹配完整状态后单次消费',()=>{
 const m=model();assert.equal(m.take(identity(),baseline(),1000,100),null);assert(m.accept(event(),identity(),'1',100));
 const value=m.take(identity(),baseline(),1000,250);assert.equal(value.elapsedMs,150);assert.equal(value.event.bornAtUnixMs,'18446744073709551615');
 assert.equal(m.accept(event(),identity(),'1',300),false);assert.equal(m.take(identity(),baseline(),1000,300),null);
});
test('事件先于完整基线只保留一个，旧序号等待并按server elapsed加单调时间裁剪',()=>{
 const m=model();assert(m.accept(event({elapsedMs:200}),identity(),'1',100));assert.equal(m.pendingCount,1);
 assert.equal(m.take(identity(),baseline({stateSequence:'10'}),1000,120),null);assert.equal(m.pendingCount,1);
 assert.equal(m.take(identity(),baseline(),1000,350).elapsedMs,450);
});
test('原动画期限到达即过期，不用服务器Unix与客户端墙钟相减',()=>{
 const m=model();assert(m.accept(event({elapsedMs:999,bornAtUnixMs:'1'}),identity(),'1',100));
 assert.equal(m.take(identity(),baseline(),1000,101),null);assert.equal(m.pendingCount,0);assert.equal(m.accept(event(),identity(),'1',101),false);
 assert.equal(petAppearDuration({profiles:{pet_skeleton:{layers:[{actions:{appear:{count:10,duration_ms:100}}}]}}}),1000);
 assert.equal(petAppearDuration({}),0);
});
test('connection/binding/role/scene/baseline/map隔离，旧票不改变当前水位',()=>{
 for(const [key,value]of Object.entries({connection:2,binding:'2',role:'other',scene:'3',baseline:'4',map:'3'})){const m=model();assert.equal(m.accept(event(),identity({[key]:value}),'1',100),false);assert(m.accept(event(),identity(),'1',100));}
 for(const change of [{role:{...role,roleId:new Uint8Array(16)}},{sceneEpoch:'1'},{baselineId:'1'},{mapId:'1'},{entityLife:1},{direction:4},{sourceSkillId:18},{elapsedMs:-1}])assert.equal(model().accept(event(change),identity(),'1',100),false);
});
test('尸体/新Life/换模板/Recall新位移/新方向/已移除body都丢弃且不产生身体',()=>{
 for(const change of [{hp:'0'},{entityKind:3},{isPetCorpse:true},{life:'9007199254740996'},{definitionId:2},{movementRevision:'1'},{direction:2}]){
  const m=model();assert(m.accept(event(),identity(),'1',0));assert.equal(m.take(identity(),baseline({entities:[body(change)]}),1000,10),null);assert.equal(m.pendingCount,0);
 }
 const m=model();assert(m.accept(event(),identity(),'1',0));assert.equal(m.take(identity(),baseline({entities:[]}),1000,10),null);
});
test('资源失效丢pending但保消费高水位；更新birth替换旧pending有界为1',()=>{
 const m=model();assert(m.accept(event(),identity(),'1',0));m.discardPending();assert.equal(m.pendingCount,0);assert.equal(m.accept(event(),identity(),'1',1),false);
 assert(m.accept(event({entityLife:'9007199254740996'}),identity(),'1',10));assert.equal(m.pendingCount,1);
 assert(m.accept(event({entityLife:'9007199254740997'}),identity(),'1',20));assert.equal(m.pendingCount,1);
 assert.equal(m.take(identity(),baseline({entities:[body({life:'9007199254740997'})]}),1000,30).event.entityLife,'9007199254740997');
});
test('真实Session只接受rule18 ready当前票，换票/关闭清scope，不改变经济counter',()=>{
 const h=harness();h.session.role={key:role};h.session.presentationNow=()=>100;const e=entry('9007199254740993','2');e.baselineId=Long.fromString('9007199254740994',true);e.resourceVersion=Long.fromInt(18,true);
 h.session.connection.deliver(20002,e);h.session.connection.deliver(30015,event());assert.equal(h.session.petBorn.pendingCount,0);ready(h,{...e,sceneEpoch:Long.fromString('9007199254740994',true)});
 // 上面的ready是另一真实入场，使用其当前scene/baseline，不能把旧scene事件混入。
 const i=h.currentEntryIdentity(),next=event({sceneEpoch:i.scene,baselineId:i.baseline});const before=h.session.command.toString();h.session.connection.deliver(30015,next);assert.equal(h.session.petBorn.pendingCount,1);assert.equal(h.session.command.toString(),before);
 h.session.connection.status('断开');assert.equal(h.session.petBorn.pendingCount,0);
});
test('30015实际PB→完整两块Assembler→真实Session，只在完整含body状态可消费且保持Long（非网络验收）',()=>{
 const model=name=>require('../../../.work/ui-tests/app/'+name+'.js');
 const codecs=load('client/assets/scripts/app/GameConnection.ts',id=>id==='long'?Long:id==='../generated/protobuf/client_pb.js'?require('../../../client/assets/scripts/generated/protobuf/client_pb.js'):{});
 class Connection{constructor(){this.generation=1;this.binding=Long.UONE;}subscribe(fn){this.listener=fn;}onStatus(fn){this.status=fn;}deliver(id,value){this.listener(codecs.decodePacket(codecs.encodePacket(id,0,this.binding,value)));}}
 const h=load('client/assets/scripts/app/GameSession.ts',id=>id==='long'?Long:id==='./GameConnection'?{GameConnection:Connection}:id==='./GameErrors'?{protocolErrorMessage:()=>''}:['./BaselineAssembler','./TeleportModel','./RoleBindingModel','./MapTravelModel','./SkillModel','./PetBornModel'].includes(id)?model(id.slice(2)):{});
 h.session.role={key:role};h.session.presentationNow=()=>100;const e=entry();e.baselineId=Long.fromString('9007199254740994',true);e.roomId=Long.fromString('2',true);e.resourceVersion=Long.fromInt(18,true);h.session.connection.deliver(20002,e);h.session.connection.deliver(6,e);
 h.session.connection.deliver(30015,event());assert.equal(h.session.petBorn.pendingCount,1);
 const chunk={sceneEpoch:e.sceneEpoch,baselineId:e.baselineId,stateSequence:Long.fromString(event().stateSequence,true),chunkCount:2,chunkIndex:0,entities:[]};
 h.session.connection.deliver(5,chunk);assert.equal(h.session.baseline,null);assert.equal(h.session.petBorn.take(h.currentEntryIdentity(),h.session.baseline,1000,100),null);
 h.session.connection.deliver(5,{...chunk,chunkIndex:1,entities:[body({life:Long.fromString(body().life,true),hp:Long.fromInt(10),hpMax:Long.fromInt(10),movementRevision:Long.UZERO})]});
 assert.equal(h.session.baseline.entities.length,1);assert(Long.isLong(h.session.baseline.entities[0].life));assert.equal(h.session.petBorn.take(h.currentEntryIdentity(),h.session.baseline,1000,150).elapsedMs,50);
 h.session.connection.deliver(30015,event());assert.equal(h.session.petBorn.pendingCount,0);
});
