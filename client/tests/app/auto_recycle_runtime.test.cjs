const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {Long,load,entry}=require('./helpers/session_harness.cjs');
const model=require('../../../.work/ui-tests/app/AutoRecycleModel.js');
const {PendingOperations}=load('client/assets/scripts/app/PendingOperations.ts',id=>id==='long'?Long:{});
const connection=load('client/assets/scripts/app/GameConnection.ts',id=>id==='long'?Long:id==='../generated/protobuf/client_pb.js'?require('../../assets/scripts/generated/protobuf/client_pb.js')
 :id==='./InputTrace'?{inputTraceMessage:()=>null,traceInput(){}}:id==='./GameErrors'?{protocolErrorMessage:()=>''}:{});
class Storage{constructor(){this.data=new Map();this.fail=false;}getItem(k){return this.data.get(k)??null;}setItem(k,v){if(this.fail)throw Error('落盘失败');this.data.set(k,v);}removeItem(k){this.data.delete(k);}}
const decorators=new Proxy({},{get(){return (...args)=>args.length>1?undefined:(...values)=>values.length===1?values[0]:undefined;}});
class Toggle{static EventType={TOGGLE:'toggle'};}class EditBox{static EventType={TEXT_CHANGED:'text-changed'};}class Label{}class Button{}
async function fixture(){
 const previous={socket:global.WebSocket,location:global.location};const sockets=[];
 class Socket{static OPEN=1;constructor(){this.readyState=0;this.sent=[];sockets.push(this);}send(bytes){this.sent.push(bytes);this.onPacket?.(connection.decodePacket(bytes));}close(){this.readyState=3;this.onclose?.();}}
 global.WebSocket=Socket;global.location={protocol:'http:',host:'127.0.0.1:18080'};
 const compiled=name=>require('../../../.work/ui-tests/app/'+name+'.js');
 const h=load('client/assets/scripts/app/GameSession.ts',id=>id==='long'?Long:id==='./GameConnection'?connection:id==='./GameErrors'?{protocolErrorMessage:()=>''}
  :['./BaselineAssembler','./TeleportModel','./RoleBindingModel','./MapTravelModel','./SkillModel','./PetBornModel'].includes(id)?compiled(id.slice(2)):{});
 const s=h.session;s.role={job:2,key:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(9)}};
 const connecting=s.connection.connect('纯组件测试票'),socket=sockets[0];socket.readyState=1;socket.onopen();await Promise.resolve();
 const deliver=(id,seq,value)=>{const bytes=connection.encodePacket(id,seq,Long.UONE,value);socket.onmessage({data:bytes.buffer});};
 const e=entry();e.resourceVersion=Long.fromInt(19,true);deliver(20002,connection.decodePacket(socket.sent[0]).seq,e);await connecting;deliver(6,0,e);
 const item=id=>({instanceId:new Uint8Array(16).fill(id),itemId:id===3?193:218,quantity:Long.ONE,version:Long.UONE,quality:0,bound:true,locked:id===1,recycleBlockFlags:id===1?1:0});
 const inventory={revision:Long.fromString('9007199254740994',true),items:[item(1),item(2),item(3)],equipped:[],autoRecycle:model.defaultAutoRecycle()};deliver(40003,0,inventory);
 const storage=new Storage(),key='qa.recycle';s.pending=new PendingOperations(storage,key);
 const cc={_decorator:decorators,Component:class{},Toggle,EditBox,Label,Button,Node:{EventType:{TOUCH_END:'touch-end'}}};
 const {AppRoot}=load('client/assets/scripts/app/AppRoot.ts',id=>id==='cc'?cc:id==='long'?Long:id==='./GameSession'?h:id==='../input/NavigationIntent'?require('../../../.work/ui-tests/input/NavigationIntent.js'):id==='./MapAttachmentCoordinator'?compiled('MapAttachmentCoordinator'):id==='./MapTravelModel'?compiled('MapTravelModel'):{});
 const {GameAutoRecycle}=load('client/assets/scripts/app/GameAutoRecycle.ts',id=>id==='cc'?cc:id==='long'?Long:{});
 const widgets=new Map(),widget=name=>{if(!widgets.has(name))widgets.set(name,{isChecked:false,string:'',interactable:true,enabled:true,setIsCheckedWithoutNotify(value){this.isChecked=value;}});return widgets.get(name);};
 const panel={getChildByPath(name){return{getComponent(){return widget(name);}};}};
 const app=new AppRoot();app.world={gameplay:JSON.parse(fs.readFileSync('client/assets/resources/development/gameplay.json','utf8'))};app.autoRecycle=new GameAutoRecycle(panel);
 app.autoRecycle.refresh(app.recycleSnapshot(),false,false);const handlers=new Map();app.find=name=>({active:true,getComponent:()=>widget(name)});app.button=(name,fn)=>handlers.set(name,fn);app.bindGameButtons();
 app.status=()=>{};app.refreshGame=()=>app.autoRecycle.refresh(app.recycleSnapshot(),app.economicBusy,app.unresolved);app.popup={confirm:async()=>true};app.selectedItem=s.inventory.items[1];
 return{app,h,s,socket,storage,key,deliver,widgets,widget,click:name=>handlers.get(name)(),close(){s.connection.close();global.WebSocket=previous.socket;global.location=previous.location;}};
}
function successful(f,callback){f.socket.onPacket=packet=>{if(![40016,40017,40002].includes(packet.id))return;callback?.(packet);f.deliver(40005,packet.seq,{opId:packet.value.operation.opId,commandSeq:packet.value.operation.commandSeq,pending:false,errorCode:0});};}
const base='UIMgr/Panels/Settings/';
test('真实保存Settings按钮→AppRoot→持久journal→PB40016，打开不提交，开启应用预期跳过锁剑并回收450',async()=>{
 const f=await fixture();try{
  assert.equal(f.widget('AutoRecycleEnabled').isChecked,false);assert.equal(f.socket.sent.map(connection.decodePacket).filter(p=>p.id===40016).length,0);
  f.widget('AutoRecycleEnabled').isChecked=true;f.widget('RecycleQuality0').isChecked=true;let text='';f.app.popup.confirm=async(_kind,message)=>{text=message;return true;};
  successful(f,packet=>{const pending=new PendingOperations(f.storage,f.key).get(f.h.roleIdentity());assert(pending,'真实存储先于网络');assert.equal(packet.id,40016);assert.equal(packet.value.applyExisting,true);assert.deepEqual(packet.value.policy.qualityIds,[0]);assert.equal(packet.value.policy.maxRequiredLevel,0);assert.equal(packet.value.operation.expectedRevision.toString(),'9007199254740994');});
  await f.click(base+'RecycleApply');assert(text.includes('预计回收2件'));assert(text.includes('绑定金币 450'));assert(text.includes('锁定'));assert.equal(f.s.pending.get(f.h.roleIdentity()),null);assert.equal(f.s.inventory.items.length,3,'客户端不预测删除资产');
 }finally{f.close();}
});
test('纯保存关闭策略applyExisting=false；明确取消/存储失败都不能发请求',async()=>{
 const f=await fixture();try{successful(f,packet=>{assert.equal(packet.value.applyExisting,false);assert.equal(packet.value.policy.enabled,false);});await f.click(base+'RecycleSave');
  const before=f.socket.sent.length;f.app.popup.confirm=async()=>false;await f.click(base+'RecycleSave');assert.equal(f.socket.sent.length,before);
  f.app.popup.confirm=async()=>true;f.storage.fail=true;await assert.rejects(f.click(base+'RecycleSave'),/保存|存储|落盘/);assert.equal(f.socket.sent.length,before);
 }finally{f.close();}
});
test('Unknown与重建journal保留原op/策略/Long；原请求重查，不生成新的经济序号',async()=>{
 const f=await fixture();try{f.socket.onPacket=p=>{if(p.id===40016)throw Error('发送结果未知');};await assert.rejects(f.click(base+'RecycleSave'),/未知/);const original=f.s.pending.get(f.h.roleIdentity()),command=f.s.command.toString();assert(original);
  f.s.pending=new PendingOperations(f.storage,f.key);successful(f);await f.app.submitPending();const values=f.socket.sent.map(connection.decodePacket).filter(p=>p.id===40016);
  assert.equal(values.length,2);assert.deepEqual(connection.encodePacket(40016,0,Long.UONE,values[0].value),connection.encodePacket(40016,0,Long.UONE,values[1].value));assert.equal(f.s.command.toString(),command);
 }finally{f.close();}
});
test('锁定按钮持久化40017原instance/version/动作；强保护不能经旧手动分解入口绕过',async()=>{
 const f=await fixture();try{successful(f,p=>{assert.equal(p.id,40017);assert.equal(p.value.locked,true);assert.equal(p.value.itemVersion.toString(),'1');assert.deepEqual(p.value.itemInstanceId,new Uint8Array(16).fill(2));assert(new PendingOperations(f.storage,f.key).get(f.h.roleIdentity()));});
  await f.click('UIMgr/Panels/Inventory/ItemLock');f.app.selectedItem=f.s.inventory.items[0];const before=f.socket.sent.length;await assert.rejects(f.click('UIMgr/Panels/Inventory/Recycle'),/锁定/);assert.equal(f.socket.sent.length,before);
 }finally{f.close();}
});
test('Confirm期间新库存或新附着使原预期失效；旧发送回执不能清新scope/角色journal',async()=>{
 const f=await fixture();try{f.app.popup.confirm=async()=>{f.s.inventory.revision=f.s.inventory.revision.add(1);return true;};const before=f.socket.sent.length;await assert.rejects(f.click(base+'RecycleSave'),/资料已变化/);assert.equal(f.socket.sent.length,before);
  f.app.popup.confirm=async()=>true;let sent;f.socket.onPacket=p=>{if(p.id===40016)sent=p;};const request=f.click(base+'RecycleSave');await new Promise(r=>setImmediate(r));assert(sent);f.s.entry={...f.s.entry,sceneEpoch:f.s.entry.sceneEpoch.add(1)};
  f.deliver(40005,sent.seq,{opId:sent.value.operation.opId,commandSeq:sent.value.operation.commandSeq,pending:false,errorCode:0});await assert.rejects(request,/附着已变化/);assert(f.s.pending.get(f.h.roleIdentity()));
 }finally{f.close();}
});
test('journal严格形状与uint64原样往返；不能持久化额外坐标/价格、重复品质或Number修订',()=>{
 const storage=new Storage(),ledger=new PendingOperations(storage,'strict'),operation={opId:new Uint8Array(16).fill(4),commandSeq:Long.UONE,expectedRevision:Long.MAX_UNSIGNED_VALUE,expectedRuleVersion:Long.fromInt(19,true)},policy=model.defaultAutoRecycle();policy.revision=Long.MAX_UNSIGNED_VALUE;
 const value={policy,applyExisting:false,operation};ledger.set('role',{id:40016,value});assert.equal(new PendingOperations(storage,'strict').get('role').value.policy.revision.toString(),Long.MAX_UNSIGNED_VALUE.toString());
 for(const changed of [{...value,x:1},{...value,policy:{...policy,qualityIds:[0,0]}},{...value,policy:{...policy,revision:1}}])assert.throws(()=>new PendingOperations().set('role',{id:40016,value:changed}));
 assert.throws(()=>new PendingOperations().set('role',{id:40017,value:{itemInstanceId:new Uint8Array(16),itemVersion:Long.UZERO,locked:true,operation}}));
});
