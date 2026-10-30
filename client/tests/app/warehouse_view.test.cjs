const {test}=require('node:test'),assert=require('node:assert/strict');
const {Long}=require('./helpers/session_harness.cjs'),{view,WidgetNode,List}=require('./helpers/warehouse_harness.cjs');
const config=require('../../assets/resources/development/gameplay.json').warehouses,L=value=>Long.fromString(String(value),true),bytes=n=>new Uint8Array(16).fill(n);
const role={originServerId:L(1),roleId:bytes(9)},roleKey='1:'+Buffer.from(bytes(9)).toString('hex');
const item={instanceId:bytes(3),itemId:218,quantity:Long.ONE,version:L(1),bound:true,quality:0,sourceKind:'',sourceId:'',locked:false,recycleBlockFlags:0};
const input=()=>({ready:true,attachment:'original:connection:binding:scene:baseline',role:roleKey,rule:'23',config,inventory:[{...item}],wallet:{gold:L(20),yuanbaoUnits:L(0)},definitions:[{item_id:218,name:'木剑'}],writeBlocked:false});
const page=(q,extra={})=>({role,scope:q.scope,revision:L(0),capacity:L(q.scope===1?config.personal_initial_slots:config.shared_slots),startSlot:q.startSlot,limit:q.limit,items:[],gold:Long.ZERO,yuanbaoUnits:Long.ZERO,...extra});
const deferred=()=>{let resolve,reject;return{promise:new Promise((yes,no)=>{resolve=yes;reject=no;}),resolve,reject};};

test('真实GameWarehouse A/B同scope查询乱序：旧完成/旧异常/finally都不能清新busy、消息或按钮',async()=>{
 for(const failOld of [false,true]){let current=input();const waiting=[];const v=view(()=>current,q=>{const d=deferred();waiting.push({q,...d});return d.promise;});v.component.refresh(current);
  const a=v.component.open(1),b=v.component.reload();assert.equal(waiting.length,2);assert.equal(v.button('Refresh').interactable,false);
  if(failOld)waiting[0].reject(Error('旧查询失败不应覆盖'));else waiting[0].resolve(page(waiting[0].q));await a;
  assert.equal(v.button('Refresh').interactable,false);assert.match(v.label('Message').string,/正在读取/);assert.equal(v.component.page,null);
  waiting[1].resolve(page(waiting[1].q));await b;assert.equal(v.button('Refresh').interactable,true);assert.equal(v.button('DepositItem').interactable,true);assert.equal(v.component.page.revision.toString(),'0');v.component.dispose();
 }
});
test('新角色/新附着拒旧读取，销毁只退役自身；旧finally不清新界面',async()=>{
 let current=input();const d=deferred(),v=view(()=>current,()=>d.promise);v.component.refresh(current);const p=v.component.open(1);
 current={...current,attachment:'new-role-scope',role:'1:'+Buffer.from(bytes(8)).toString('hex')};v.component.refresh(current);d.resolve(page({scope:1,startSlot:L(0),limit:config.query_page_limit}));await p;assert.equal(v.component.page,null);assert.equal(v.button('DepositItem').interactable,false);
 const x=deferred();v.component.query=()=>x.promise;const pending=v.component.open(1);v.panel.emit(WidgetNode.EventType.NODE_DESTROYED);v.panel.isValid=false;v.bag.node.isValid=false;v.store.node.isValid=false;x.resolve(page({scope:1,startSlot:L(0),limit:config.query_page_limit}));await pending;assert.doesNotThrow(()=>v.component.dispose());
});
test('保存List选择当次更新数量/上下文，不重绑或改变任一offset；未决只禁写不禁查询',async()=>{
 let current=input();const v=view(()=>current,async q=>page(q));v.component.refresh(current);await v.component.open(1);v.bag.offset=17;v.store.offset=93;const before=[v.bag.binds,v.store.binds];
 v.bag.node.emit(List.EVENT_ITEM_CLICK,0,current.inventory[0]);v.store.node.emit(List.EVENT_ITEM_CLICK,2,v.store.items[2]);assert.deepEqual([v.bag.binds,v.store.binds],before);assert.equal(v.bag.offset,17);assert.equal(v.store.offset,93);assert.equal(v.component.prepare(1).request.warehouseSlot.toString(),'2');
 current={...current,writeBlocked:true};v.component.refresh(current);assert.equal(v.button('DepositItem').interactable,false);assert.equal(v.button('Refresh').interactable,true);assert.throws(()=>v.component.prepare(1),/等待/);v.component.dispose();
});
test('共享读权限失效清页和选择，不能继续展示旧物；个人/共享页码及滚动各自保留',async()=>{
 let current=input(),deny=false;const v=view(()=>current,async q=>{if(deny&&q.scope===2){const e=Error('共享读资格过期');e.code=3504;throw e;}return page(q);});v.component.refresh(current);await v.component.open(1);v.store.offset=111;
 await v.component.open(2);v.store.offset=222;await v.component.open(1);assert.equal(v.store.offset,111);await v.component.open(2);assert.equal(v.store.offset,222);
 deny=true;await assert.rejects(v.component.reload(),/过期/);assert.equal(v.component.page,null);assert.equal(v.store.items.length,0);assert.equal(v.button('WithdrawItem').interactable,false);assert.match(v.label('Message').string,/过期/);v.component.dispose();
});
test('类注入反例：共享原动作晚3504或成功只清原scope缓存，个人查询B/选择/消息不会被清；不冒称UI已绕过全局busy',async()=>{
 for(const failure of [false,true]){let current=input(),waiting=null;const v=view(()=>current,q=>{if(q.scope===1){waiting={q,...deferred()};return waiting.promise;}return Promise.resolve(page(q));});v.component.refresh(current);await v.component.open(2);assert(v.component.pager.page(2));
  v.bag.node.emit(List.EVENT_ITEM_CLICK,0,current.inventory[0]);const selected=v.component.selectedBag;assert(selected);const b=v.component.open(1);assert.equal(v.button('Refresh').interactable,false);
  if(failure){const error=Error('原共享资格过期');error.code=3504;v.component.rejected(error,2);}else await v.component.committed(2);
  assert.equal(v.component.pager.page(2),null);assert.equal(v.button('Refresh').interactable,false);assert.match(v.label('Message').string,/正在读取/);assert.equal(v.component.selectedBag,selected);
  waiting.resolve(page(waiting.q));await b;assert.equal(v.component.page.scope,1);assert.equal(v.button('Refresh').interactable,true);assert.equal(v.component.selectedBag,selected);v.component.dispose();
 }
});
