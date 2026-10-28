const {test}=require('node:test'),assert=require('node:assert/strict');
const {RoleBindingModel,roleBindingText}=require('../../../.work/ui-tests/app/RoleBindingModel.js');
const identity={connection:1,binding:'9007199254740993',role:'1:'+'09'.repeat(16),scene:'9007199254740994',baseline:'9007199254740995',map:'2',travelId:''};
const role={originServerId:'1',roleId:new Uint8Array(16).fill(9)};
const packet=(overrides={})=>({role,sceneEpoch:identity.scene,roleRevision:'9007199254740996',sourceSeq:'9007199254740993',binding:{mapId:'2',center:{x:650,y:631}},cause:1,safeZoneId:'106',trusted:true,...overrides});
function model(){const m=new RoleBindingModel();m.bind(identity);return m;}
const accept=(m,p,i=identity,b=identity.binding)=>m.accept(p,i,b);
test('绑定模型独立保存完整64位身份、原中心和receipt SourceSeq，初次快照与nil绑定均静默',()=>{
 const m=model();assert.equal(roleBindingText(m.state),'绑定信息同步中');assert.equal(accept(m,packet({sourceSeq:'0',binding:null,trusted:false,safeZoneId:'0'})),true);assert.equal(roleBindingText(m.state),'尚未绑定');assert.equal(m.notice,null);
 const n=model();assert.equal(accept(n,packet()),true);assert.equal(n.state.sourceSeq,'9007199254740993');assert.equal(n.state.roleRevision,'9007199254740996');assert.equal(n.notice,null);assert.equal(roleBindingText(n.state,[{id:'2',name:'比奇省'}]),'绑定：比奇省 (650,631)');
 assert.equal(accept(n,packet({roleRevision:'18446744073709551615'})),true);assert.equal(n.state.roleRevision,'18446744073709551615');
});
test('历史坐标无证明只显示待确认；同中心第一次可信提交提示一次，同中心重复receipt不刷',()=>{
 const m=model();assert.equal(accept(m,packet({sourceSeq:'0',trusted:false,safeZoneId:'0'})),true);assert.match(roleBindingText(m.state),/历史记录待服务端确认/);assert.equal(m.notice,null);
 assert.equal(accept(m,packet({roleRevision:'9007199254740997',cause:2})),true);const first=m.notice;assert(first&&first.trusted);assert.equal(accept(m,packet({roleRevision:'9007199254740997',cause:2})),false);assert.equal(m.notice,first);
 assert.equal(accept(m,packet({roleRevision:'9007199254740998',sourceSeq:'9007199254740994',cause:2})),true);assert.equal(m.notice,first);
 assert.equal(accept(m,packet({roleRevision:'9007199254740999',sourceSeq:'9007199254740995',binding:{mapId:'2',center:{x:330,y:330}},cause:2})),true);assert.deepEqual(m.notice.binding.center,{x:330,y:330});
});
test('旧connection/binding/role/scene/baseline不能覆盖当前绑定；个人图仍可观察已提交Town绑定',()=>{
 for(const key of ['connection','binding','role','scene','baseline','map','travelId']){const m=model(),changed={...identity,[key]:key==='connection'?2:key==='role'?'other':key==='travelId'?'old':'3'};assert.equal(accept(m,packet(),changed),false);assert.equal(m.state,null);}
 const m=model();assert.equal(accept(m,packet(),identity,'8'),false);assert.equal(accept(m,packet({sceneEpoch:'1'})),false);assert.equal(accept(m,packet({role:{...role,roleId:new Uint8Array(16).fill(8)}})),false);
 const personal={...identity,map:'1'};m.bind(personal);assert.equal(accept(m,packet(),personal),true);assert.equal(m.state.binding.mapId,'2');
});
test('绑定观察拒绝Number窄化、矛盾证明、负格、旧修订/receipt和同水位不同中心',()=>{
 const bad=[{roleRevision:1},{roleRevision:'0'},{roleRevision:'9007199254740992'},{sourceSeq:1},{safeZoneId:106},{safeZoneId:'0'},{trusted:false,sourceSeq:'0',safeZoneId:'106'},{sceneEpoch:1},{sourceSeq:'0'},{trusted:false},{cause:2,trusted:false,sourceSeq:'0'},{binding:null},{binding:{mapId:2,center:{x:650,y:631}}},{binding:{mapId:'2',center:{x:-1,y:0}}},{role:{...role,roleId:new Uint8Array(15)}}];
 for(const value of bad){const m=model();assert.equal(accept(m,packet(value)),false);assert.equal(m.state,null);}
 const m=model();accept(m,packet());const original=m.state;assert.equal(accept(m,packet({roleRevision:'9007199254740995'})),false);assert.equal(accept(m,packet({sourceSeq:'9007199254740992'})),false);assert.equal(accept(m,packet({binding:{mapId:'2',center:{x:1,y:1}}})),false);assert.equal(m.state,original);
});
test('健康重入与冷附着快照静默恢复，清scope后迟到旧事件不能恢复状态',()=>{
 const m=model();accept(m,packet({cause:2}));assert(m.notice);const next={...identity,connection:2,binding:'9007199254740999',scene:'1',baseline:'1'};m.bind(next);assert.equal(m.state,null);assert.equal(m.notice,null);
 assert.equal(m.accept(packet({sceneEpoch:'1'}),next,next.binding),true);assert.equal(m.notice,null);m.clear();assert.equal(m.accept(packet({sceneEpoch:'1',cause:2}),next,next.binding),false);assert.equal(m.state,null);
});

const {Long,load,harness,entry,ready}=require('./helpers/session_harness.cjs');
function serverBinding(h,e,changes={}){return packet({role:h.session.role.key,sceneEpoch:e.sceneEpoch,roleRevision:Long.fromInt(11,true),sourceSeq:Long.fromInt(9,true),safeZoneId:Long.fromInt(106,true),binding:{mapId:Long.fromInt(2,true),center:{x:650,y:631}},...changes});}
test('真实Session入场ACK前可接已提交快照，但BindingState不授予Ready；未绑定与未知状态分开',()=>{
 const h=harness(),s=h.session,e=entry('1','2',2);s.connection.deliver(20002,e);assert.equal(s.bindingState,null);assert.equal(s.ready,false);
 s.connection.deliver(20011,serverBinding(h,e,{binding:null,sourceSeq:Long.UZERO,safeZoneId:Long.UZERO,trusted:false}));assert(s.bindingState);assert.equal(s.bindingState.binding,null);assert.equal(s.ready,false);assert.equal(s.bindingNotice,null);
 s.connection.deliver(6,e);assert.equal(s.ready,true);assert.equal(s.bindingState.binding,null);
});
test('真实Session绑定自动提交只观察receipt，SourceSeq不动经济command、不清改原Pending revision',()=>{
 const h=harness(),s=h.session,e=ready(h,entry('1','2',2,'5'));s.inventory={revision:Long.fromInt(10,true)};const operation=h.operation();
 const {PendingOperations}=load('client/assets/scripts/app/PendingOperations.ts',id=>id==='long'?Long:{});s.pending=new PendingOperations();s.pending.set(h.roleIdentity(),{id:40006,value:{benefitKind:0,operation}});const before=s.pending.get(h.roleIdentity());
 s.connection.deliver(40003,{revision:Long.fromString('9007199254740994',true)});s.connection.deliver(20011,serverBinding(h,e,{cause:2,sourceSeq:Long.fromString('9007199254740993',true),roleRevision:Long.fromString('9007199254740994',true)}));
 assert.equal(s.bindingState.sourceSeq,'9007199254740993');assert.equal(s.bindingNotice.cause,2);assert.equal(s.command.toString(),'6');assert.deepEqual(s.pending.get(h.roleIdentity()),before);assert.equal(before.value.operation.expectedRevision.toString(),'10');
 // 不存在Pending时新operation仍采用完整Inventory，而非SourceSeq或任意绑定修订。
 s.pending.clear(h.roleIdentity());const next=h.operation();assert.equal(next.expectedRevision.toString(),'9007199254740994');assert.equal(next.commandSeq.toString(),'6');
});
test('真实Session绑定观察领先Inventory时新op等待完整资料，旧Pending不变；较新Inventory不遮掉较晚绑定',()=>{
 const h=harness(),s=h.session,e=ready(h,entry('1','2',2,'5'));s.inventory={revision:Long.fromInt(10,true)};s.connection.deliver(20011,serverBinding(h,e,{cause:2}));assert.throws(()=>h.operation(),/角色资料正在同步/);assert.equal(s.command.toString(),'5');
 s.connection.deliver(40003,{revision:Long.fromInt(20,true)});assert.equal(h.operation().expectedRevision.toString(),'20');
 s.connection.deliver(20011,serverBinding(h,e,{roleRevision:Long.fromInt(12,true),sourceSeq:Long.fromInt(10,true),binding:{mapId:Long.fromInt(2,true),center:{x:330,y:330}},cause:2}));assert.deepEqual(s.bindingState.binding.center,{x:330,y:330});assert.equal(h.operation().expectedRevision.toString(),'20');
});
test('真实Session旧scene/不同角色/旧transport binding隔离，健康新连接和冷scene1静默恢复receipt',()=>{
 const h=harness(),s=h.session,e=ready(h,entry('9007199254740993','2',2));s.connection.deliver(20011,serverBinding(h,e));const original=s.bindingState;
 s.connection.deliver(20011,serverBinding(h,e,{sceneEpoch:Long.UONE,cause:2}));s.connection.deliver(20011,serverBinding(h,e,{role:{...s.role.key,roleId:new Uint8Array(16).fill(8)},cause:2}));s.connection.deliver(20011,serverBinding(h,e,{cause:2}),{binding:Long.fromInt(8,true)});assert.equal(s.bindingState,original);
 s.connection.status('断线');assert.equal(s.bindingState,null);s.connection.binding=Long.fromInt(2,true);const next=entry('1','1',1);s.connection.deliver(20002,next);s.connection.deliver(20011,serverBinding(h,next));assert.equal(s.bindingState.binding.mapId,'2');assert.equal(s.bindingNotice,null);assert.equal(s.ready,false);
});
