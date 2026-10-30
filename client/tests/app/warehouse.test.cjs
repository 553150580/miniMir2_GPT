const {test}=require('node:test'),assert=require('node:assert/strict');
const {Long}=require('./helpers/session_harness.cjs');
const m=require('../../../.work/ui-tests/app/WarehouseModel.js');
const config=require('../../assets/resources/development/gameplay.json').warehouses;
const L=(value,unsigned=true)=>Long.fromString(String(value),unsigned),id=n=>new Uint8Array(16).fill(n),role={originServerId:L(1),roleId:id(9)},roleKey='1:'+Buffer.from(id(9)).toString('hex');
const item=(changes={})=>({instanceId:id(3),itemId:218,quantity:L(5,false),version:L(7),bound:true,quality:0,sourceKind:'',sourceId:'',locked:true,recycleBlockFlags:1,...changes});
const page=(changes={})=>({role,scope:1,revision:L(0),capacity:L(config.personal_initial_slots),startSlot:L(0),limit:config.query_page_limit,items:[],gold:L(0,false),yuanbaoUnits:L(0,false),...changes});
const query=(changes={})=>({scope:1,startSlot:L(0),limit:config.query_page_limit,...changes});
const snapshot=(changes={})=>({ready:true,attachment:'connection1:binding7:scene9:baseline4',role:roleKey,rule:'23',config,page:page(),inventory:[item()],wallet:{gold:L(9007199254740995n,false),yuanbaoUnits:L(9007199254740995n,false)},...changes});

test('完整仓库目录拒缺失/null/不安全数，30只是显式分页窗口而非个人容量上限',()=>{
 assert.deepEqual(m.warehouseConfig(config),config);for(const data of [null,{}, {...config,shared_slots:null},{...config,query_page_limit:0},{...config,personal_expansion_slots:Number.MAX_SAFE_INTEGER+1}])assert.throws(()=>m.warehouseConfig(data));
 const start=9007199254740990n,capacity=start+60n,p=m.observedWarehousePage(page({capacity:L(capacity),startSlot:L(start),revision:L(9007199254740995n)}),query({startSlot:L(start)}),roleKey,config);
 const rows=m.warehouseSlots(p);assert.equal(rows.length,config.query_page_limit);assert.equal(rows[0].slot.toString(),start.toString());assert.equal(rows.at(-1).slot.toString(),(start+29n).toString());assert.equal(p.capacity.toString(),capacity.toString());
});
test('页必须匹配原role/query，原仓位/实例不能重复或跨窗口，个人不接收货币',()=>{
 const original=item();const accepted=m.observedWarehousePage(page({items:[{slot:L(1),item:original}]}),query(),roleKey,config);original.instanceId[0]=0;assert.equal(accepted.items[0].item.instanceId[0],3);
 for(const p of [page({role:{...role,roleId:id(8)}}),page({startSlot:L(1)}),page({limit:config.query_page_limit+1}),page({gold:L(1,false)}),page({items:[{slot:L(30),item:item()}]}),page({items:[{slot:L(1),item:item()},{slot:L(2),item:item()}]})])assert.throws(()=>m.observedWarehousePage(p,query(),roleKey,config));
 assert.throws(()=>m.observedWarehousePage(page({scope:2,capacity:L(config.shared_slots),items:[{slot:L(0),item:item()}]}),query({scope:2}),roleKey,config),/绑定/);
});
test('锁定旧物/未知定义完整存入保持原身份，不套新穿职业/品质；待决占用穿戴禁止',()=>{
 const unknown=item({itemId:777777,quality:4,sourceKind:'',sourceId:''}),s=snapshot({inventory:[unknown]});const p=m.createWarehousePreview(s,1,L(2),unknown,'5');assert.equal(p.request.quantity.toString(),'5');assert.equal(p.request.itemVersion.toString(),'7');assert.equal(p.request.warehouseSlot.toString(),'2');assert.equal(unknown.locked,true);
 for(const flags of [3,5,9])assert.throws(()=>m.createWarehousePreview(snapshot({inventory:[item({recycleBlockFlags:flags})]}),1,L(1),item({recycleBlockFlags:flags}),'1'),/穿戴|待确认|占用/);
 assert.throws(()=>m.createWarehousePreview(snapshot(),1,L(1),item(),'6'),/数量/);assert.throws(()=>m.createWarehousePreview(snapshot({page:page({items:[{slot:L(1),item:item()}]})}),1,L(1),item(),'1'),/空仓位/);
});
test('确认冻结原实例/版本/位置/仓修订/附着与币种余额，允许无关经验推进',()=>{
 const s=snapshot(),p=m.createWarehousePreview(s,1,L(2),s.inventory[0],'2');m.validateWarehousePreview(p,{...s,experience:L(999)});
 for(const next of [{...s,attachment:'new-binding'},{...s,role:'other-role'},{...s,rule:'24'},{...s,page:page({revision:L(1)})},{...s,inventory:[]},{...s,inventory:[item({version:L(8)})]},{...s,inventory:[item({quantity:L(4,false)})]}])assert.throws(()=>m.validateWarehousePreview(p,next));
 const shared=snapshot({page:page({scope:2,capacity:L(config.shared_slots)})}),money=m.createWarehousePreview(shared,3,L(0),null,'900719925474099.5',2);assert.equal(money.request.amount.toString(),'9007199254740995');assert.equal(money.request.itemInstanceId.length,0);assert.throws(()=>m.validateWarehousePreview(money,{...shared,wallet:{...shared.wallet,yuanbaoUnits:L(9007199254740994n,false)}}),/余额|钱包/);
});
test('数量和元宝解析无Number窄化，不换绑定币；个人扩页成本只取公开目录',()=>{
 assert.equal(m.warehouseQuantity('9007199254740995').toString(),'9007199254740995');assert.equal(m.warehouseMoney('123.4',2).toString(),'1234');assert.equal(m.warehouseMoneyText(L(1234,false)),'123.4');
 for(const t of ['0','-1','01','1e4','1.00','9223372036854775808'])assert.throws(()=>m.warehouseQuantity(t));for(const t of ['0','-1','1.23','1e3'])assert.throws(()=>m.warehouseMoney(t,2));
 const p=m.createWarehousePreview(snapshot(),5,L(0),null,'');assert.equal(p.request.amount.toString(),'0');assert.match(m.warehouseConfirmation(p,config,''),new RegExp(String(config.personal_expansion_slots)));assert.match(m.warehouseConfirmation(p,config,''),/3000\.0/);
 assert.throws(()=>m.createWarehousePreview(snapshot({wallet:{gold:L(999999,false),yuanbaoUnits:L(0,false),boundYuanbaoUnits:L(999999,false)}}),5,L(0),null,''),/非绑定元宝不足/);
});
test('有界Pager只保留各类一页：旧请求/旧角色/旧代不能覆盖新请求或清新pending',()=>{
 const p=new m.WarehousePager();p.bind('a');const old=p.begin(query()),next=p.begin(query());assert.equal(p.accept(old,page(),roleKey,config),false);assert.equal(p.fail(old),false);assert.equal(p.accept(next,page(),roleKey,config),true);
 const stale=p.begin(query());p.bind('b');const fresh=p.begin(query());assert.equal(p.accept(stale,page({revision:L(999)}),roleKey,config),false);assert.equal(p.accept(fresh,page(),roleKey,config),true);
 const low=p.begin(query());assert(p.accept(low,page({revision:L(2)}),roleKey,config));const older=p.begin(query());assert.equal(p.accept(older,page({revision:L(1)}),roleKey,config),false);assert.equal(p.page(1).revision.toString(),'2');p.invalidate(1);assert.equal(p.page(1),null);
});
test('同修订的重叠页不接受原物品或货币冲突，拒绝后仍能准确结束原ticket',()=>{
 const p=new m.WarehousePager();p.bind('a');const first=p.begin(query());assert(p.accept(first,page({revision:L(2),items:[{slot:L(1),item:item()}]}),roleKey,config));
 const next=p.begin(query());assert.throws(()=>p.accept(next,page({revision:L(2),items:[]}),roleKey,config),/冲突/);assert.equal(p.fail(next),true);assert.equal(p.page(1).items.length,1);
 const shared=p.begin(query({scope:2}));assert(p.accept(shared,page({scope:2,capacity:L(config.shared_slots),revision:L(3),gold:L(1,false)}),roleKey,config));
 const bad=p.begin(query({scope:2}));assert.throws(()=>p.accept(bad,page({scope:2,capacity:L(config.shared_slots),revision:L(3),gold:L(2,false)}),roleKey,config),/冲突/);assert(p.fail(bad));
});
test('40023只接受本附着明确原op的一次原提交；不能以历史仓revision覆盖页面',()=>{
 const guard=new m.WarehouseReceiptGuard(),op=id(11),request={scope:1,action:1,warehouseRevision:L(0)};
 const receipt={role,opId:op,roleRevision:L(9007199254740995n),scope:1,action:1,warehouseRevision:L(1),capacity:L(config.personal_initial_slots),changes:[{slot:L(0),before:null,after:item()}],gold:L(0,false),yuanbaoUnits:L(0,false)};
 guard.expect('old',op,request);assert.equal(guard.accept(receipt,'new',roleKey),false);assert.equal(guard.accept({...receipt,opId:id(12)},'old',roleKey),false);assert(guard.accept(receipt,'old',roleKey));assert.equal(guard.accept(receipt,'old',roleKey),false);
 guard.expect('old',id(12),request);guard.bind('new');assert.equal(guard.accept({...receipt,opId:id(12)},'new',roleKey),false);
 guard.expect('new',op,{scope:2,action:3,warehouseRevision:L(4)});const cash={...receipt,scope:2,action:3,warehouseRevision:L(5),capacity:L(config.shared_slots),changes:[]};assert(guard.accept(cash,'new',roleKey));
});
