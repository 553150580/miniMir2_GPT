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
 const {AppRoot}=load('client/assets/scripts/app/AppRoot.ts',id=>id==='cc'?cc:id==='long'?Long:id==='./GameSession'?h:id==='./SkillAssetsModel'?compiled('SkillAssetsModel'):id==='../input/NavigationIntent'?require('../../../.work/ui-tests/input/NavigationIntent.js'):id==='./MapAttachmentCoordinator'?compiled('MapAttachmentCoordinator'):id==='./MapTravelModel'?compiled('MapTravelModel'):{});
 const {GameAutoRecycle}=load('client/assets/scripts/app/GameAutoRecycle.ts',id=>id==='cc'?cc:id==='long'?Long:{});
 const widgets=new Map(),widget=name=>{if(!widgets.has(name))widgets.set(name,{isChecked:false,string:'',interactable:true,enabled:true,setIsCheckedWithoutNotify(value){this.isChecked=value;}});return widgets.get(name);};
 const panel={getChildByPath(name){return{getComponent(){return widget(name);}};}};
 const app=new AppRoot();app.world={gameplay:JSON.parse(fs.readFileSync('client/assets/resources/development/gameplay.json','utf8'))};app.autoRecycle=new GameAutoRecycle(panel);
 app.autoRecycle.refresh(app.recycleSnapshot(),false,false);const handlers=new Map();app.find=name=>({active:true,getComponent:()=>widget(name)});app.button=(name,fn)=>handlers.set(name,fn);app.bindGameButtons();
 app.status=()=>{};app.refreshGame=()=>app.autoRecycle.refresh(app.recycleSnapshot(),app.economicBusy,app.unresolved);app.popup={confirm:async()=>true};app.selectedItem=s.inventory.items[1];
 return{app,h,s,socket,storage,key,deliver,widgets,widget,click:name=>handlers.get(name)(),close(){s.connection.close();global.WebSocket=previous.socket;global.location=previous.location;}};
}
const buttonPath = 'UIMgr/Panels/Inventory/ItemLock';
function preparePages(f) {
 const item=n=>({instanceId:new Uint8Array(16).fill(n),itemId:900001,quantity:Long.fromInt(5),version:Long.fromString('9007199254740993',true),bound:true,quality:0,sourceKind:'skill_recycle',sourceId:'original-batch',locked:false,recycleBlockFlags:0});
 f.deliver(40003,0,{...f.s.inventory,items:[item(5),item(6)],equipped:[]});
 f.deliver(40004,0,{revision:f.s.inventory.revision,gold:Long.fromInt(100),boundGold:Long.fromInt(200),yuanbaoUnits:Long.ZERO,boundYuanbaoUnits:Long.ZERO});
 f.app.selectedItem=f.s.inventory.items[0]; f.app.skillAssets={locks:[],lock(v){this.locks.push(v);},showQuote:async()=>true};
}
function preview(f,packet) {
 return {previewId:new Uint8Array(16).fill(8),kind:4,ruleVersion:f.s.entry.resourceVersion,configHash:'a'.repeat(64),roleRevision:f.s.inventory.revision,
  expiresAtUnixMs:Long.fromNumber(Date.now()+30000,true),skillId:0,fromLevel:0,toLevel:0,offerId:0,quantity:Long.ZERO,stats:[],
  spend:{revision:f.s.inventory.revision,gold:Long.ZERO,boundGold:Long.ZERO,yuanbaoUnits:Long.ZERO,boundYuanbaoUnits:Long.ZERO},
  materials:packet.value.materials.map(choice=>({item:f.s.inventory.items.find(x=>x.instanceId.toString()===choice.instanceId.toString()),consumeQuantity:choice.quantity})),
  outputs:[{...f.s.inventory.items[0],instanceId:new Uint8Array(),quantity:Long.fromInt(10),version:Long.UONE}]};
}
function respond(f,commit) {
 f.socket.onPacket=packet=>{if(packet.id===40011){assert.equal(packet.value.kind,4);f.deliver(40012,packet.seq,preview(f,packet));}
  if(packet.id===40013){if(commit)commit(packet);else f.deliver(40005,packet.seq,{opId:packet.value.operation.opId,commandSeq:packet.value.operation.commandSeq,pending:false,errorCode:0});}};
}

test('保存背包上下文原入口→真实40011完整Long材料→报价确认→先journal后PB40013',async()=>{
 const f=await fixture();try{preparePages(f);let displayed;
  f.app.skillAssets.showQuote=async quote=>{displayed=quote;return true;};
  respond(f,packet=>{const persisted=new PendingOperations(f.storage,f.key).get(f.h.roleIdentity());assert(persisted);assert.equal(persisted.id,40013);
   assert.deepEqual(persisted.value.previewId,new Uint8Array(16).fill(8));assert.deepEqual(persisted.value.operation.opId,packet.value.operation.opId);
   f.deliver(40005,packet.seq,{opId:packet.value.operation.opId,commandSeq:packet.value.operation.commandSeq,pending:false,errorCode:0});});
  await f.click(buttonPath);const sent=f.socket.sent.map(connection.decodePacket).filter(x=>[40011,40013,40017].includes(x.id));
  assert.deepEqual(sent.map(x=>x.id),[40011,40013]);assert.equal(sent[0].value.materials[0].version.toString(),'9007199254740993');
  assert.deepEqual(sent[0].value.materials.map(x=>x.quantity.toString()),['5','5']);assert.equal(displayed.kind,4);
  assert.equal(f.s.pending.get(f.h.roleIdentity()),null);assert.equal(f.s.inventory.items.length,2,'客户端不预测合并或自行增删物品');
  assert.deepEqual(f.app.skillAssets.locks,[true,false]);
 }finally{f.close();}
});

test('取消合并与真实journal落盘失败都不能发送40013或改发装备锁',async()=>{
 const f=await fixture();try{preparePages(f);respond(f);f.app.skillAssets.showQuote=async()=>false;await f.click(buttonPath);
  assert.equal(f.socket.sent.map(connection.decodePacket).filter(x=>[40013,40017].includes(x.id)).length,0);
  f.app.skillAssets.showQuote=async()=>true;f.storage.fail=true;const command=f.s.command.toString();await assert.rejects(f.click(buttonPath),/落盘|存储|保存/);
  assert.equal(f.s.command.toString(),command);assert.equal(f.socket.sent.map(connection.decodePacket).filter(x=>[40013,40017].includes(x.id)).length,0);
 }finally{f.close();}
});

test('合并确认期间换票或换scene、保护/原来源改变，旧报价不能提交',async()=>{
 for(const mutate of [f=>{f.s.connection.binding=f.s.connection.binding.add(1);},f=>{f.s.entry={...f.s.entry,sceneEpoch:f.s.entry.sceneEpoch.add(1)};},
  f=>{f.s.inventory.items[0].locked=true;},f=>{f.s.inventory.items[0].sourceId='different-batch';},f=>{delete f.s.inventory.items[0].locked;}]){
  const f=await fixture();try{preparePages(f);respond(f);f.app.skillAssets.showQuote=async()=>{mutate(f);return true;};
   await assert.rejects(f.click(buttonPath),/变化|重新预览/);assert.equal(f.socket.sent.map(connection.decodePacket).filter(x=>x.id===40013).length,0);
  }finally{f.close();}
 }
});

test('合并提交结果Unknown后持久重建只重放原previewId/op，不重新报价换身份',async()=>{
 const f=await fixture();try{preparePages(f);respond(f,()=>{throw Error('发送结果未知');});await assert.rejects(f.click(buttonPath),/未知/);
  const original=f.s.pending.get(f.h.roleIdentity()),command=f.s.command.toString();assert(original);f.s.pending=new PendingOperations(f.storage,f.key);
  respond(f);await f.app.submitPending();const sent=f.socket.sent.map(connection.decodePacket),commits=sent.filter(x=>x.id===40013);
  assert.equal(sent.filter(x=>x.id===40011).length,1);assert.equal(commits.length,2);assert.deepEqual(connection.encodePacket(40013,0,Long.UONE,commits[0].value),connection.encodePacket(40013,0,Long.UONE,commits[1].value));
  assert.equal(f.s.command.toString(),command);assert.equal(f.s.pending.get(f.h.roleIdentity()),null);
 }finally{f.close();}
});

test('真实GameSkillAssets复用滚动报价列表和合并确认标题，不构造固定节点',async()=>{
 const {GameSkillAssets}=load('client/assets/scripts/app/GameSkillAssets.ts',id=>id==='cc'?{}:id==='./SkillAssetsModel'?require('../../../.work/ui-tests/app/SkillAssetsModel.js'):{});
 const view=Object.create(GameSkillAssets.prototype),shown=[];view.input={definitions:[{item_id:900001,name:'书页',kind:'skill_page'}]};
 view.quoteList={setItems(lines,_render,preserve){shown.push({lines,preserve});}};view.popup={confirm:async(kind,message,title)=>{shown.push({kind,message,title});return false;}};
 const f=await fixture();try{preparePages(f);const q=preview(f,{value:{materials:f.s.inventory.items.map(x=>({instanceId:x.instanceId,quantity:x.quantity}))}});
  assert.equal(await view.showQuote(q),false);assert.equal(shown[0].preserve,false);assert(shown[0].lines.some(x=>x.includes('原有2叠')));assert.equal(shown[1].kind,'SkillQuote');assert.equal(shown[1].title,'合并确认');
 }finally{f.close();}
});
