const {test}=require('node:test'),assert=require('node:assert/strict');
const {actualList,warehouse,method,Vec2,Button,Long,List}=require('./helpers/list_restore_harness.cjs');
const cfg=require('../../assets/resources/development/gameplay.json').warehouses,L=x=>Long.fromString(String(x),true),bytes=n=>new Uint8Array(16).fill(n);
const role={originServerId:L(1),roleId:bytes(9)},roleKey='1:'+Buffer.from(bytes(9)).toString('hex');
const item={instanceId:bytes(3),itemId:218,quantity:Long.ONE,version:L(2),bound:true,quality:0,sourceKind:'',sourceId:'',locked:true,recycleBlockFlags:1};
const input=()=>({ready:true,attachment:'original:binding:scene:baseline',role:roleKey,rule:'23',config:cfg,inventory:[item],wallet:{gold:L(0),yuanbaoUnits:L(0)},definitions:[{item_id:218,name:'木剑'}],writeBlocked:false});
const page=(q,revision,items=[])=>({role,scope:q.scope,revision:L(revision),capacity:L(q.scope===1?cfg.personal_initial_slots:cfg.shared_slots),startSlot:q.startSlot,limit:q.limit,items,gold:Long.ZERO,yuanbaoUnits:Long.ZERO});
test('引擎0时长不派滚动事件：真实List清页后直接恢复底层offset会空白，公开恢复API须同步重建末行池',()=>{
 assert(!method('_doScroll').includes('_dispatchEvent'));assert(!method('_moveContent').includes('_dispatchEvent'));
 const v=actualList();v.list.setItems(Array.from({length:30},(_,n)=>n));v.userScroll(2420);assert(v.visible().includes(29));
 v.list.setItems([]);v.list.setItems(Array.from({length:30},(_,n)=>n));const emitted=v.scroll.emitted;v.scroll.scrollToOffset(new Vec2(0,2420),0);assert.equal(v.scroll.emitted,emitted);assert.deepEqual(v.visible(),[],'真实空白反例前提');
 v.list.restoreScrollOffset(2420);assert(v.visible().includes(29));assert.equal(v.scroll.getScrollOffset().y,2420);assert(v.list.visibleNodeCount<=10);
});
test('真实GameWarehouse提交后清页/查询/恢复2420：末仓位29立即可见且点击绑定原物，不等待额外拖动事件',async()=>{
 let current=input(),revision=0;const v=warehouse(()=>current,async q=>page(q,revision,revision?[{slot:L(29),item}]:[]));v.component.refresh(current);await v.component.open(1);v.store.userScroll(2420);assert(v.store.visible().includes(29));
 current={...current,inventory:[]};revision=1;await v.component.committed(1);assert.equal(v.store.scroll.getScrollOffset().y,2420);assert(v.store.visible().includes(29),'新查询后原末仓位不能空白');
 const row=v.store.list.rows.get(29);assert(row);row.emit(Button.EventType.CLICK);assert.equal(v.component.prepare(2).request.warehouseSlot.toString(),'29');assert.equal(v.component.prepare(2).request.itemVersion.toString(),'2');assert.equal(v.store.scroll.getScrollOffset().y,2420);v.component.dispose();
});
test('真实两仓切换恢复各自末页滚动后都保有Mask内条目，恢复不改变选择/renderer或超出池预算',async()=>{
 const current=input(),v=warehouse(()=>current,async q=>page(q,0));v.component.refresh(current);await v.component.open(1);v.store.userScroll(2420);await v.component.open(2);v.store.userScroll(1800);await v.component.open(1);assert.equal(v.store.scroll.getScrollOffset().y,2420);assert(v.store.visible().includes(29));await v.component.open(2);assert.equal(v.store.scroll.getScrollOffset().y,1800);assert(v.store.visible().length>0);assert(v.store.list.visibleNodeCount<=10);v.component.dispose();
});
test('恢复偏移按当前内容界限钳制，拒非有限值，退休节点不再写偏移或碰旧行',()=>{
 const v=actualList();v.list.setItems(Array.from({length:20},(_,n)=>n));v.list.restoreScrollOffset(99999);assert.equal(v.scroll.getScrollOffset().y,1520);assert(v.visible().includes(19));v.list.restoreScrollOffset(-5);assert.equal(v.scroll.getScrollOffset().y,0);assert(v.visible().includes(0));const writes=v.scroll.writes.length;v.list.restoreScrollOffset(NaN);assert.equal(v.scroll.writes.length,writes);v.node.isValid=false;v.list.restoreScrollOffset(400);assert.equal(v.scroll.writes.length,writes);
});
