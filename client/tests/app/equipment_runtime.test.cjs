const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs');
const {Long,load,entry}=require('./helpers/session_harness.cjs');
const {PendingOperations}=load('client/assets/scripts/app/PendingOperations.ts',id=>id==='long'?Long:{});
const compiled=name=>require('../../../.work/ui-tests/app/'+name+'.js');
const connection=load('client/assets/scripts/app/GameConnection.ts',id=>id==='long'?Long:id==='../generated/protobuf/client_pb.js'?require('../../assets/scripts/generated/protobuf/client_pb.js'):id==='./InputTrace'?{inputTraceMessage:()=>null,traceInput(){}}:id==='./GameErrors'?{protocolErrorMessage:()=>''}:{});
class Storage{constructor(){this.data=new Map();this.fail=false;}getItem(k){return this.data.get(k)??null;}setItem(k,v){if(this.fail)throw Error('落盘失败');this.data.set(k,v);}removeItem(k){this.data.delete(k);}}
const bytes=n=>new Uint8Array(16).fill(n),decorators=new Proxy({},{get(){return(...args)=>args.length>1?undefined:(...v)=>v.length===1?v[0]:undefined;}});
async function fixture(){
 const previous={socket:global.WebSocket,location:global.location},sockets=[];
 class Socket{static OPEN=1;constructor(){this.readyState=0;this.sent=[];sockets.push(this);}send(value){this.sent.push(value);this.onPacket?.(connection.decodePacket(value));}close(){this.readyState=3;this.onclose?.();}}
 global.WebSocket=Socket;global.location={protocol:'http:',host:'127.0.0.1:18080'};
 const h=load('client/assets/scripts/app/GameSession.ts',id=>id==='long'?Long:id==='./GameConnection'?connection:id==='./GameErrors'?{protocolErrorMessage:()=>''}:['./BaselineAssembler','./TeleportModel','./RoleBindingModel','./MapTravelModel','./SkillModel','./PetBornModel'].includes(id)?compiled(id.slice(2)):{}),s=h.session;
 s.role={job:2,key:{originServerId:Long.UONE,roleId:bytes(9)}};const waiting=s.connection.connect('组件测试票'),socket=sockets[0];socket.readyState=1;socket.onopen();await Promise.resolve();
 const deliver=(id,seq,value)=>{const data=connection.encodePacket(id,seq,Long.UONE,value);socket.onmessage({data:data.buffer});};
 const e=entry();e.resourceVersion=Long.fromInt(22,true);deliver(20002,connection.decodePacket(socket.sent[0]).seq,e);await waiting;deliver(6,0,e);
 const original={instanceId:bytes(3),itemId:218,quantity:Long.ONE,version:Long.fromString('9007199254740995',true),bound:true,quality:0,locked:false,recycleBlockFlags:0};
 deliver(40003,0,{revision:Long.fromString('9007199254740994',true),level:7,items:[original],equipped:[]});
 const storage=new Storage(),key='equipment.account';s.pending=new PendingOperations(storage,key);
 const cc={_decorator:decorators,Component:class{},Button:{},Toggle:{EventType:{TOGGLE:'toggle'}},EditBox:{EventType:{TEXT_CHANGED:'text-changed'}},Node:{EventType:{TOUCH_END:'touch-end'}}};
 const {AppRoot}=load(process.env.MINIMIR_EQUIPMENT_RUNTIME_SOURCE||'client/assets/scripts/app/AppRoot.ts',id=>id==='cc'?cc:id==='long'?Long:id==='./GameSession'?h:id==='../input/NavigationIntent'?require('../../../.work/ui-tests/input/NavigationIntent.js'):['./MapTravelModel','./CombatStatusModel'].includes(id)?compiled(id.slice(2)):{}),app=new AppRoot(),handlers=new Map();
 app.world={gameplay:JSON.parse(fs.readFileSync('client/assets/resources/development/gameplay.json','utf8'))};app.find=()=>({active:true,getComponent:()=>({})});app.button=(path,fn)=>handlers.set(path,fn);app.bindGameButtons();app.status=()=>{};app.refreshGame=()=>{};app.popup={confirm:async()=>true};app.selectedItem=s.inventory.items[0];
 return{h,s,app,socket,storage,key,deliver,click:path=>handlers.get(path)(),packets:()=>socket.sent.map(connection.decodePacket).filter(p=>p.id===40001),close(){s.connection.close();global.WebSocket=previous.socket;global.location=previous.location;}};
}
function succeed(f,inspect){f.socket.onPacket=p=>{if(p.id!==40001)return;inspect?.(p);f.deliver(40005,p.seq,{opId:p.value.operation.opId,commandSeq:p.value.operation.commandSeq,pending:false,errorCode:0});};}
const equip='UIMgr/Panels/Inventory/Equip',unequip='UIMgr/Panels/Role/Equip';

test('实际保存穿戴按钮确认后沿Session→真实journal→PB40001发送显式false与原Long版本，XP推进可用最新revision',async()=>{
 const f=await fixture();try{let confirm='';f.app.popup.confirm=async(_name,text)=>{confirm=text;f.s.inventory={...f.s.inventory,revision:f.s.inventory.revision.add(1),level:8};return true;};
  succeed(f,p=>{const stored=new PendingOperations(f.storage,f.key).get(f.h.roleIdentity());assert(stored,'必须先落盘再发送');assert.equal(stored.value.unequip,false);assert.equal(p.value.unequip,false);assert.equal(p.value.itemVersion.toString(),'9007199254740995');assert.equal(p.value.operation.expectedRevision.toString(),'9007199254740995');assert.deepEqual(p.value.itemInstanceId,bytes(3));});
  await f.click(equip);assert.match(confirm,/穿戴木剑/);assert.equal(f.packets().length,1);assert.equal(f.s.pending.get(f.h.roleIdentity()),null);assert.equal(f.s.inventory.items.length,1,'不本地预测搬装备');
 }finally{f.close();}
});

test('实际Role已穿戴选择明确卸下原slot与原版本，背包同类不会被取代发送',async()=>{
 const f=await fixture();try{const worn={...f.s.inventory.items[0],instanceId:bytes(5),recycleBlockFlags:8};f.s.inventory.equipped=[{slot:0,item:worn}];f.app.selectedEquipped={slot:0,item:worn};
  succeed(f,p=>{assert.equal(p.value.unequip,true);assert.equal(p.value.slot,0);assert.deepEqual(p.value.itemInstanceId,bytes(5));assert.equal(p.value.itemVersion.toString(),worn.version.toString());});await f.click(unequip);assert.equal(f.packets().length,1);assert.deepEqual(f.app.selectedItem.instanceId,bytes(3));
 }finally{f.close();}
});

test('确认取消、同ID改版本、移出容器、换slot和新附着均不能发40001，持久化失败也不发',async()=>{
 for(const mode of ['cancel','version','container','slot','scope','storage']){const f=await fixture();try{if(mode==='slot'){const worn={...f.s.inventory.items[0],recycleBlockFlags:8};f.s.inventory.equipped=[{slot:0,item:worn}];f.app.selectedEquipped={slot:0,item:worn};}f.app.popup.confirm=async()=>{if(mode==='cancel')return false;if(mode==='version')f.s.inventory.items[0].version=f.s.inventory.items[0].version.add(1);if(mode==='container'){f.s.inventory.equipped=[{slot:0,item:f.s.inventory.items[0]}];f.s.inventory.items=[];}if(mode==='slot')f.s.inventory.equipped[0].slot=1;if(mode==='scope')f.s.entry={...f.s.entry,sceneEpoch:f.s.entry.sceneEpoch.add(1)};if(mode==='storage')f.storage.fail=true;return true;};
  if(mode==='cancel')await f.click(equip);else await assert.rejects(f.click(mode==='slot'?unequip:equip),/变化|位置|保存未决操作/);assert.equal(f.packets().length,0,mode);
 }finally{f.close();}}
});

test('新40001 Unknown重建真实journal仍沿同op/版本/动作原字节重发，不分新经济序号',async()=>{
 const f=await fixture();try{f.socket.onPacket=p=>{if(p.id===40001)throw Error('发送结果未知');};await assert.rejects(f.click(equip),/未知/);const original=f.s.pending.get(f.h.roleIdentity()),command=f.s.command.toString();assert(original);f.s.pending=new PendingOperations(f.storage,f.key);succeed(f);await f.app.submitPending();const all=f.packets();assert.equal(all.length,2);assert.deepEqual(connection.encodePacket(40001,0,Long.UONE,all[0].value),connection.encodePacket(40001,0,Long.UONE,all[1].value));assert.equal(f.s.command.toString(),command);
 }finally{f.close();}
});

test('旧21三字段journal原shape恢复并经真实编码重发，不补新版本、不把原穿戴改为卸下',async()=>{
 const f=await fixture();try{const operation={...f.h.operation(),expectedRuleVersion:Long.fromInt(21,true)};f.s.pending.set(f.h.roleIdentity(),{id:40001,value:{itemInstanceId:bytes(3),slot:0,operation}});f.s.pending=new PendingOperations(f.storage,f.key);const old=f.s.pending.get(f.h.roleIdentity());assert.deepEqual(Object.keys(old.value).sort(),['itemInstanceId','operation','slot']);succeed(f,p=>{assert.equal(p.value.unequip,false);assert.equal(p.value.itemVersion.toString(),'0');assert.equal(p.value.operation.expectedRuleVersion.toString(),'21');assert.deepEqual(p.value.operation.opId,operation.opId);const stored=new PendingOperations(f.storage,f.key).get(f.h.roleIdentity());assert(!('unequip'in stored.value));assert(!('itemVersion'in stored.value));});await f.app.submitPending();assert.equal(f.packets().length,1);
 }finally{f.close();}
});

test('旧附着迟到回执保留原pending，不能清新角色journal或改新选择',async()=>{
 const f=await fixture();try{let sent;f.socket.onPacket=p=>{if(p.id===40001)sent=p;};const task=f.click(equip);await new Promise(r=>setImmediate(r));assert(sent);const oldRole=f.h.roleIdentity();f.s.role={...f.s.role,key:{originServerId:Long.UONE,roleId:bytes(8)}};f.s.entry={...f.s.entry,sceneEpoch:f.s.entry.sceneEpoch.add(1)};const newRole=f.h.roleIdentity(),other={id:40001,value:{itemInstanceId:bytes(6),slot:1,unequip:true,itemVersion:Long.UONE,operation:{...sent.value.operation,opId:bytes(10)}}};f.s.pending.set(newRole,other);const selected={instanceId:bytes(11)};f.app.selectedItem=selected;
  f.deliver(40005,sent.seq,{opId:sent.value.operation.opId,commandSeq:sent.value.operation.commandSeq,pending:false,errorCode:0});await assert.rejects(task,/附着已变化/);assert(f.s.pending.get(oldRole));assert.deepEqual(f.s.pending.get(newRole).value.itemInstanceId,bytes(6));assert.equal(f.app.selectedItem,selected);
 }finally{f.close();}
});

test('新旧40001 strict shape不允许半套新增字段、Number版本或多余坐标；专用错误保留中文边界',()=>{
 const operation={opId:bytes(1),commandSeq:Long.UONE,expectedRevision:Long.UONE,expectedRuleVersion:Long.fromInt(22,true)},value={itemInstanceId:bytes(3),slot:0,unequip:false,itemVersion:Long.MAX_UNSIGNED_VALUE,operation};const storage=new Storage();new PendingOperations(storage,'max').set('role',{id:40001,value});assert.equal(new PendingOperations(storage,'max').get('role').value.itemVersion.toString(),Long.MAX_UNSIGNED_VALUE.toString());
 for(const changed of [{...value,itemVersion:1},{...value,itemVersion:Long.UZERO},{...value,unequip:'false'},{...value,x:1},{itemInstanceId:bytes(3),slot:0,unequip:false,operation}])assert.throws(()=>new PendingOperations().set('role',{id:40001,value:changed}));
 const ids=load('client/assets/scripts/generated/protocol_ids.ts',()=>({})),errors=load('client/assets/scripts/app/GameErrors.ts',id=>id==='../generated/protocol_ids'?ids:{});assert.match(errors.protocolErrorMessage(40001,3401),/背包空间不足.*保持原位/);assert.match(errors.protocolErrorMessage(40001,3402),/原装备.*变化/);assert.match(errors.protocolErrorMessage(40001,3403),/职业、等级/);
});

test('实际Role战斗属性按钮读取当前baseline的MC/SC Long，刷新和新Entry范围不靠本地装备差值',async()=>{
 const f=await fixture();try{const attrs={physicalAttackMin:Long.ONE,physicalAttackMax:Long.fromInt(5),physicalDefenceMin:Long.ZERO,physicalDefenceMax:Long.ONE,magicDefenceMin:Long.ZERO,magicDefenceMax:Long.ONE,magicAttackMin:Long.ZERO,magicAttackMax:Long.fromString('9007199254740995'),spiritAttackMin:Long.ONE,spiritAttackMax:Long.fromInt(3),accuracy:Long.ONE,agility:Long.ONE,damageReductionPercent:0};f.s.baseline={entities:[{entityHandle:f.s.entry.playerHandle,combatAttributes:attrs}]};let read;f.app.combatStatus={showLiveDetails(title,provider){assert.equal(title,'战斗属性');read=provider;}};
  await f.click('UIMgr/Panels/Role/Recycle');assert(read().includes('魔法攻击 0–9007199254740995'));assert(read().includes('道术攻击 1–3'));f.s.baseline.entities[0].combatAttributes={...attrs,spiritAttackMax:Long.fromInt(7)};assert(read().includes('道术攻击 1–7'));f.s.entry={...f.s.entry,sceneEpoch:f.s.entry.sceneEpoch.add(1)};assert.equal(read(),null);assert.equal(f.packets().length,0);
 }finally{f.close();}
});
