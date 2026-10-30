const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {Long,load,entry}=require('./helpers/session_harness.cjs'),{view,List}=require('./helpers/warehouse_harness.cjs');
const compiled=name=>require('../../../.work/ui-tests/app/'+name+'.js'),bytes=n=>new Uint8Array(16).fill(n);
const {PendingOperations}=load('client/assets/scripts/app/PendingOperations.ts',id=>id==='long'?Long:{});
const connection=load('client/assets/scripts/app/GameConnection.ts',id=>id==='long'?Long:id==='../generated/protobuf/client_pb.js'?require('../../assets/scripts/generated/protobuf/client_pb.js'):id==='./InputTrace'?{inputTraceMessage:()=>null,traceInput(){}}:id==='./GameErrors'?compiled('GameErrors'):{});
class Storage{constructor(){this.data=new Map();this.fail=false;}getItem(k){return this.data.get(k)??null;}setItem(k,v){if(this.fail)throw Error('落盘失败');this.data.set(k,v);}removeItem(k){this.data.delete(k);}}
const decorators=new Proxy({},{get(){return(...args)=>args.length>1?undefined:(...v)=>v.length===1?v[0]:undefined;}});
async function fixture(){
 const previous={socket:global.WebSocket,location:global.location},sockets=[];
 class Socket{static OPEN=1;constructor(){this.readyState=0;this.sent=[];sockets.push(this);}send(raw){this.sent.push(raw);this.onPacket?.(connection.decodePacket(raw));}close(){this.readyState=3;this.onclose?.();}}
 global.WebSocket=Socket;global.location={protocol:'http:',host:'127.0.0.1:18080'};
 const h=load('client/assets/scripts/app/GameSession.ts',id=>id==='long'?Long:id==='./GameConnection'?connection:id==='./GameErrors'?compiled('GameErrors'):['./BaselineAssembler','./TeleportModel','./RoleBindingModel','./MapTravelModel','./SkillModel','./PetBornModel'].includes(id)?compiled(id.slice(2)):{}),s=h.session;
 s.role={job:0,key:{originServerId:Long.UONE,roleId:bytes(9)}};const waiting=s.connection.connect('组件测试票'),socket=sockets[0];socket.readyState=1;socket.onopen();await Promise.resolve();
 const deliver=(id,seq,value)=>{const raw=connection.encodePacket(id,seq,Long.UONE,value);socket.onmessage({data:raw.buffer});};
 const e=entry();e.resourceVersion=Long.fromInt(23,true);deliver(20002,connection.decodePacket(socket.sent[0]).seq,e);await waiting;deliver(6,0,e);
 const original={instanceId:bytes(3),itemId:218,quantity:Long.ONE,version:Long.fromString('9007199254740995',true),bound:true,quality:0,locked:true,recycleBlockFlags:1,sourceKind:'',sourceId:''};
 deliver(40003,0,{revision:Long.fromInt(2,true),level:1,items:[original],equipped:[]});deliver(40004,0,{revision:Long.fromInt(2,true),gold:Long.fromInt(10),yuanbaoUnits:Long.fromString('9007199254740995')});
 const storage=new Storage(),key='warehouse.account';s.pending=new PendingOperations(storage,key);
 const cc={_decorator:decorators,Component:class{},Button:{},Toggle:{EventType:{TOGGLE:'toggle'}},EditBox:{EventType:{TEXT_CHANGED:'text-changed'}},Node:{EventType:{TOUCH_END:'touch-end'}}};
 const {AppRoot}=load('client/assets/scripts/app/AppRoot.ts',id=>id==='cc'?cc:id==='long'?Long:id==='./GameSession'?h:id==='./GameConnection'?connection:id==='../input/NavigationIntent'?require('../../../.work/ui-tests/input/NavigationIntent.js'):['./MapTravelModel','./CombatStatusModel'].includes(id)?compiled(id.slice(2)):{}),app=new AppRoot(),handlers=new Map();
 app.world={gameplay:JSON.parse(fs.readFileSync('client/assets/resources/development/gameplay.json','utf8'))};app.find=()=>({active:true,getComponent:()=>({})});app.button=(path,fn)=>handlers.set(path,fn);app.bindGameButtons();app.status=()=>{};app.refreshGame=()=>{};app.popup={confirm:async()=>true};
 const ui=view(()=>app.warehouseInput(),q=>s.connection.request(40020,q));app.warehouse=ui.component;
 let warehouse={role:s.role.key,scope:1,revision:Long.UZERO,capacity:Long.fromNumber(app.world.gameplay.warehouses.personal_initial_slots,true),items:[],gold:Long.ZERO,yuanbaoUnits:Long.ZERO};
 socket.onPacket=p=>{if(p.id===40020)deliver(40021,p.seq,{...warehouse,scope:p.value.scope,capacity:p.value.scope===2?Long.fromNumber(app.world.gameplay.warehouses.shared_slots,true):warehouse.capacity,startSlot:p.value.startSlot,limit:p.value.limit});};
 await ui.component.open(1);ui.bag.node.emit(List.EVENT_ITEM_CLICK,0,s.inventory.items[0]);ui.store.node.emit(List.EVENT_ITEM_CLICK,2,ui.store.items[2]);
 return{h,s,app,ui,socket,storage,key,deliver,original,warehouse,click:path=>handlers.get('UIMgr/Panels/Warehouse/'+path)(),packets:()=>socket.sent.map(connection.decodePacket),setWarehouse(value){warehouse=value;},close(){ui.component.dispose();s.connection.close();global.WebSocket=previous.socket;global.location=previous.location;}};
}
function succeed(f,inspect,{receipt=true}={}){
 const old=f.socket.onPacket;f.socket.onPacket=p=>{if(p.id!==40022){old(p);return;}inspect?.(p);
  const moved={...f.original,version:f.original.version.add(1)};f.setWarehouse({...f.warehouse,revision:Long.UONE,items:[{slot:p.value.warehouseSlot,item:moved}]});
  const revision=p.value.operation.expectedRevision.add(1);f.deliver(40003,0,{revision,level:1,items:[],equipped:[]});
  f.deliver(40005,p.seq,{opId:p.value.operation.opId,commandSeq:p.value.operation.commandSeq,newRevision:revision,pending:false,errorCode:0});
  if(receipt)f.deliver(40023,0,{role:f.s.role.key,roleRevision:revision,opId:p.value.operation.opId,scope:1,action:1,warehouseRevision:Long.UONE,capacity:f.warehouse.capacity,changes:[{slot:p.value.warehouseSlot,after:moved}],gold:Long.ZERO,yuanbaoUnits:Long.ZERO});
 };
}

test('实际保存存入按钮：GameWarehouse→确认→原journal持久化→真实40022编码；原40005清未决，40023只观测再查页',async()=>{
 const f=await fixture();try{succeed(f,p=>{const stored=new PendingOperations(f.storage,f.key).get(f.h.roleIdentity());assert(stored);assert.equal(stored.id,40022);assert.equal(stored.value.itemVersion.toString(),'9007199254740995');assert.equal(p.value.warehouseRevision.toString(),'0');assert.equal(p.value.warehouseSlot.toString(),'2');assert.equal(p.value.quantity.toString(),'1');assert.equal(p.value.itemVersion.toString(),'9007199254740995');assert.deepEqual(p.value.itemInstanceId,bytes(3));assert.equal(f.s.inventory.items.length,1,'回执之前不能本地搬物');});
  await f.click('DepositItem');assert.equal(f.packets().filter(x=>x.id===40022).length,1);assert.equal(f.s.pending.get(f.h.roleIdentity()),null);assert.equal(f.s.inventory.items.length,0);assert.equal(f.ui.component.page.revision.toString(),'1');assert.equal(f.s.warehouseResult.warehouseRevision.toString(),'1');assert.equal(f.s.warehouseNotice.action,1);
  f.deliver(40023,0,{...f.s.warehouseResult,warehouseRevision:Long.fromInt(999,true)});assert.equal(f.ui.component.page.revision.toString(),'1');
 }finally{f.close();}
});
test('确认取消/改版本/换仓修订/新附着/存储失败均在发40022前拒绝；无关EXP可推进当前operation修订',async()=>{
 for(const mode of ['cancel','version','page','scope','storage','xp']){const f=await fixture();try{f.app.popup.confirm=async()=>{if(mode==='cancel')return false;if(mode==='version')f.s.inventory.items[0].version=f.s.inventory.items[0].version.add(1);if(mode==='page')f.ui.component.page.revision=Long.UONE;if(mode==='scope')f.s.connection.close();if(mode==='storage')f.storage.fail=true;if(mode==='xp')f.s.inventory={...f.s.inventory,revision:Long.fromInt(3,true),experience:Long.fromInt(999)};return true;};succeed(f,p=>assert.equal(p.value.operation.expectedRevision.toString(),'3'));
   if(mode==='cancel'||mode==='xp')await f.click('DepositItem');else await assert.rejects(f.click('DepositItem'));assert.equal(f.packets().filter(x=>x.id===40022).length,mode==='xp'?1:0,mode);
  }finally{f.close();}}
});
test('真实journal故障保留原40022，恢复后完整Long/空Bytes/币种/原op不重造；读取请求禁止写journal',()=>{
 const storage=new Storage(),ledger=new PendingOperations(storage,'warehouse'),operation={opId:bytes(8),commandSeq:Long.UONE,expectedRevision:Long.UONE,expectedRuleVersion:Long.fromInt(23,true)};
 const original={scope:2,action:3,warehouseRevision:Long.fromString('9007199254740995',true),warehouseSlot:Long.UZERO,itemInstanceId:new Uint8Array(0),itemVersion:Long.UZERO,quantity:Long.ZERO,currency:2,amount:Long.fromString('9007199254740995'),operation};ledger.set('1:role',{id:40022,value:original});const replay=new PendingOperations(storage,'warehouse').get('1:role');
 assert.equal(replay.value.itemInstanceId.length,0);assert.equal(replay.value.amount.toString(),original.amount.toString());assert.deepEqual(replay.value.operation.opId,operation.opId);assert.equal(connection.decodePacket(connection.encodePacket(replay.id,8,Long.UONE,replay.value)).value.itemInstanceId.length,0);
 for(const change of [{amount:Number.MAX_SAFE_INTEGER},{warehouseSlot:Long.UONE},{itemInstanceId:bytes(1)},{quantity:Long.ONE},{currency:0},{scope:1},{unexpected:1}])assert.throws(()=>new PendingOperations().set('1:role',{id:40022,value:{...original,...change}}));assert.throws(()=>new PendingOperations().set('1:role',{id:40020,value:{scope:1,startSlot:Long.UZERO,limit:30,operation}}));
});
test('原OperationResult成功即解除未决，不依赖40023；旧附着/其它角色结果不能造当前仓库提示',async()=>{
 const f=await fixture();try{succeed(f,null,{receipt:false});await f.click('DepositItem');assert.equal(f.s.pending.get(f.h.roleIdentity()),null);assert.equal(f.s.warehouseResult,null);
  const stale={role:f.s.role.key,opId:bytes(77),scope:1,action:1,roleRevision:Long.fromInt(3,true),warehouseRevision:Long.UONE,capacity:f.warehouse.capacity,changes:[],gold:Long.ZERO,yuanbaoUnits:Long.ZERO};f.deliver(40023,0,stale);assert.equal(f.s.warehouseResult,null);
  const other={...stale,role:{...f.s.role.key,roleId:bytes(10)}};f.deliver(40023,0,other);assert.equal(f.s.warehouseNotice,null);
 }finally{f.close();}
});
test('共享到期原op重查仅返真实40005终态即可清journal；后续3504清页，不等待或虚造40023',async()=>{
 const f=await fixture();try{await f.ui.component.open(2);f.ui.component.chooseCurrency(1);f.ui.edit('Amount').string='1';let committed=false;const old=f.socket.onPacket;
  f.socket.onPacket=p=>{if(p.id===40022){assert.equal(p.value.scope,2);assert.equal(p.value.action,3);assert(new PendingOperations(f.storage,f.key).get(f.h.roleIdentity()));committed=true;f.deliver(40005,p.seq,{opId:p.value.operation.opId,commandSeq:p.value.operation.commandSeq,newRevision:Long.fromInt(3,true),errorCode:0,pending:false});return;}if(p.id===40020&&committed){const number=value=>{let x=BigInt(value),out=[];do{const b=Number(x&127n);x>>=7n;out.push(b|(x?128:0));}while(x);return out;};const raw=new Uint8Array([1,1,40021>>8,40021&255,...number(p.seq),1,...number(3504)]);f.socket.onmessage({data:raw.buffer});return;}old(p);};
  await assert.rejects(f.click('DepositMoney'),/特权/);assert.equal(f.s.pending.get(f.h.roleIdentity()),null);assert.equal(f.app.unresolved,false);assert.equal(f.s.warehouseResult,null);assert.equal(f.ui.component.page,null);assert.equal(f.ui.store.items.length,0);
 }finally{f.close();}
});
test('实际连接在40022发送后关闭：journal原op/指纹保留，新客户端沿原40022重查而不重新分配操作',async()=>{
 const first=await fixture();let storage,key,original;
 try{first.socket.onPacket=p=>{if(p.id===40022){original=p.value;first.s.connection.close();}};await assert.rejects(first.click('DepositItem'));storage=first.storage;key=first.key;assert.equal(first.app.unresolved,true);assert(first.s.pending.get(first.h.roleIdentity()));assert.equal(first.packets().filter(p=>p.id===40022).length,1);}finally{first.close();}
 const next=await fixture();try{next.s.pending=new PendingOperations(storage,key);succeed(next,p=>{for(const name of ['warehouseRevision','warehouseSlot','itemVersion','quantity','amount'])assert.equal(p.value[name].toString(),original[name].toString());assert.deepEqual(p.value.itemInstanceId,original.itemInstanceId);assert.deepEqual(p.value.operation.opId,original.operation.opId);assert.equal(p.value.operation.commandSeq.toString(),original.operation.commandSeq.toString());assert.equal(p.value.operation.expectedRevision.toString(),original.operation.expectedRevision.toString());});await next.app.submitPending();assert.equal(next.s.pending.get(next.h.roleIdentity()),null);assert.equal(next.packets().filter(p=>p.id===40022).length,1);}finally{next.close();}
});
