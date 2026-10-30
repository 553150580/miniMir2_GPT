'use strict';
// 只检验观察器与真实协议形状，不启动浏览器、服务或数据库。
const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path');
const {createRequire}=require('node:module'),root=path.resolve(__dirname,'../..'),ts=require('./creator_paths.cjs').typescript;
const tool=require('./verify_warehouses_browser.cjs'),pb=require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const Long=createRequire(path.join(root,'client/package.json'))('long'),file=path.join(__dirname,'verify_warehouses_browser.cjs'),source=fs.readFileSync(file,'utf8');
function connection(){const input=fs.readFileSync(path.join(root,'client/assets/scripts/app/GameConnection.ts'),'utf8'),code=ts.transpileModule(input,{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,esModuleInterop:true}}).outputText,m={exports:{}};new Function('require','module','exports',code)(id=>id==='long'?Long:id.endsWith('/client_pb.js')?{minimir:{v1:pb}}:{},m,m.exports);return m.exports;}
const encode=connection().encodePacket,hex=x=>Buffer.from(x).toString('hex'),big=Long.fromString('9007199254740995',true),id=new Uint8Array(16).fill(9);
const item=()=>({instance_id:id,item_id:218,quantity:Long.fromString('9223372036854775807'),version:big,bound:true,quality:0,source_kind:'monster_drop',source_id:'原Source:'+big,locked:true,recycle_block_flags:1});
test('实际rule23目录、五项配置、16个PB观察类型及保存仓库节点预检，不启动Chrome',()=>{const p=tool.preflight();assert.equal(p.fixture.rule_version,23);assert.equal(p.gameplay.rule_version,23);assert.equal(p.codecs.length,16);assert.equal(p.gameplay.warehouses.query_page_limit,30);assert.equal(p.gameplay.warehouses.personal_expansion_yuanbao_units,30000);});
test('真实GameConnection编码与PB往返保留warehouse/slot/version/quantity全部Long，原回执变化可读',()=>{const value={scope:1,action:1,warehouseRevision:big,warehouseSlot:big,itemInstanceId:id,itemVersion:big,quantity:Long.MAX_VALUE,currency:0,amount:Long.ZERO,operation:{opId:id,commandSeq:big,expectedRevision:big,expectedRuleVersion:Long.fromInt(23,true)}};
 const p=tool.packet(encode(40022,0xffffffff,Long.MAX_UNSIGNED_VALUE,value)),q=pb.WarehouseAction.decode(p.body);assert.equal(p.request,'4294967295');assert.equal(p.binding,'18446744073709551615');for(const key of['warehouse_revision','warehouse_slot','item_version'])assert.equal(q[key].toString(),big.toString());assert.equal(q.quantity.toString(),'9223372036854775807');assert.equal(hex(q.operation.op_id),hex(id));
 const r=tool.rawResult(pb.WarehouseActionResult.decode(pb.WarehouseActionResult.encode({role:{origin_server_id:big,role_id:id},role_revision:big,op_id:id,scope:1,action:1,warehouse_revision:big,capacity:big,changes:[{slot:big,before:null,after:item()}],gold:Long.MAX_VALUE,yuanbao_units:Long.MAX_VALUE}).finish()));assert.equal(r.role,big+':'+hex(id));assert.equal(r.capacity,big.toString());assert.equal(r.changes[0].slot,big.toString());assert.equal(r.changes[0].before,null);assert.equal(r.changes[0].after.quantity,'9223372036854775807');assert.equal(r.gold,'9223372036854775807');
});
test('Query拒绝观察实际原40020/3504，不能误认正常Page40021或资产40005',()=>{const query=Buffer.from(encode(40020,7,big,{scope:2,startSlot:Long.UZERO,limit:30})),head=tool.packet(query),offset=query.length-head.body.length,variable=value=>{const bytes=[];let n=BigInt(value);do{const b=Number(n&127n);n>>=7n;bytes.push(b|(n?128:0));}while(n);return Buffer.from(bytes);};query[1]=1;const denied=tool.packet(Buffer.concat([query.subarray(0,offset),variable(3504)]));assert.deepEqual({id:denied.id,error:denied.error,request:denied.request,binding:denied.binding},{id:40020,error:3504,request:'7',binding:big.toString()});assert.equal(denied.body.length,0);assert(!source.includes('x.id===40021&&x.code===3504'));assert(source.includes('x.id===40020&&x.code===3504'));});
test('HTTP-only正常10005/10006创角匹配原职业0和2，认证正文/非本端口先过滤，不记录凭据',()=>{for(const job of[0,2]){const request=Buffer.from(encode(10005,3,Long.UZERO,{job,name:'只读测试',originServerId:Long.UONE,slot:0,opId:id})),response=Buffer.from(encode(10006,3,Long.UZERO,{key:{originServerId:big,roleId:id},job,slot:0})),fake={url:()=> 'http://127.0.0.1:18082/account/rpc',method:()=> 'POST',postDataBuffer:()=>request};assert.deepEqual(tool.createdRoleResponse(fake,response,'http://127.0.0.1:18082'),{roleId:hex(id),origin:big.toString(),job,slot:0});}
 const forbidden=()=>{throw Error('不允许读取其它请求正文');};for(const [url,method]of[['http://127.0.0.1:18080/account/rpc','POST'],['http://127.0.0.1:18082/account/rpc?token=not-a-real-credential','POST'],['http://127.0.0.1:18082/account/rpc','GET'],['https://elsewhere.invalid/account/rpc','POST']])assert.equal(tool.createdRoleResponse({url:()=>url,method:()=>method,postDataBuffer:forbidden},null,'http://127.0.0.1:18082'),null);
 const fake={url:()=> 'http://127.0.0.1:18082/account/rpc',method:()=> 'POST',postDataBuffer:()=>Buffer.from([1,0,0x27,0x19,0x80,0x80])};assert.equal(tool.createdRoleResponse(fake,null,'http://127.0.0.1:18082'),null);
});
test('页面与原件观察保留显式零、原来源/锁，部分数量与完整版本分开核验',()=>{const decoded=pb.ItemView.decode(pb.ItemView.encode(item()).finish()),raw=tool.rawItem(decoded),after={...raw,version:(BigInt(raw.version)+1n).toString()};tool.sameOriginal(raw,after);assert.equal(raw.quality,0);assert.equal(raw.locked,true);assert.equal(raw.flags,1);assert.throws(()=>tool.sameOriginal(raw,{...after,sourceId:'另一次来源'}));assert.throws(()=>tool.sameOriginal(raw,{...after,flags:0}));const q=tool.rawPage(pb.WarehousePage.decode(pb.WarehousePage.encode({role:{origin_server_id:big,role_id:id},scope:2,revision:Long.UZERO,capacity:big,start_slot:big,limit:30,items:[{slot:big,item:decoded}],gold:Long.ZERO,yuanbao_units:Long.ZERO}).finish()));assert.equal(q.revision,'0');assert.equal(q.gold,'0');assert.equal(q.items[0].slot,big.toString());});
test('overscan首行虽在复用池但在Mask上方时必须向前回滚，不能拿pool最小index当可见首行',()=>{const view={bottom:-132,top:132},rows=[{index:0,bottom:200,top:290},{index:1,bottom:110,top:200},{index:2,bottom:20,top:110},{index:3,bottom:-70,top:20},{index:4,bottom:-160,top:-70},{index:5,bottom:-250,top:-160}];assert.equal(Math.min(...rows.map(x=>x.index)),0);assert.equal(tool.scrollBackForTarget(0,rows,view),true);assert.equal(tool.scrollBackForTarget(5,rows,view),false);assert.equal(tool.scrollBackForTarget(6,rows,view),false);assert.equal(tool.scrollBackForTarget(0,rows.filter(x=>x.index>0),view),true);assert.throws(()=>tool.scrollBackForTarget(0,[],view));assert(!source.includes('Math.min(...Array.from(list.rows.keys()))'));assert(source.includes('rows.filter(row=>row.top>view.bottom&&row.bottom<view.top)'));assert(source.includes("await tap('BottomHUD/Footer/Map')"));assert(source.includes("s.entry?.roomId.toString()==='2'"));assert(source.includes('report.observations.townTravel='));});
test('所有waitForFunction同步布尔，不把async Promise当就绪；选取等待实际原UID/仓位',()=>{const ast=ts.createSourceFile(file,source,ts.ScriptTarget.Latest,true,ts.ScriptKind.JS),callbacks=[];function visit(n){if(ts.isCallExpression(n)&&ts.isPropertyAccessExpression(n.expression)&&n.expression.name.text==='waitForFunction')callbacks.push(n.arguments[0].getText(ast));ts.forEachChild(n,visit);}visit(ast);assert(callbacks.length>=10);for(const callback of callbacks)assert(!/^async\b/.test(callback));const text=callbacks.find(x=>x.includes("listName==='Inventory'?h(a.selectedItem"));assert(text);const fn=Function('return ('+text+')')(),old=global.cc;global.cc={director:{getScene:()=>({getComponentsInChildren:()=>[{warehouse:{selectedBag:hex(id),selectedSlot:big.toString()},selectedItem:{instanceId:id}}]})}};try{assert.equal(fn({key:hex(id),listName:'BagList'}),true);assert.equal(fn({key:'ffff',listName:'BagList'}),false);assert.equal(fn({key:big.toString(),listName:'WarehouseList'}),true);assert.equal(fn({key:'1',listName:'WarehouseList'}),false);}finally{global.cc=old;}});
test('触屏选择采样在找点之后紧邻touch，业务不私调且完整loaded JsonAsset原字节只按Node语义对比',()=>{const begin=source.indexOf('if(result.point){const beforeTouch=await state();'),touch=source.indexOf('await page.touchscreen.tap(result.point.x,result.point.y);',begin),after=source.indexOf('const after=await state();',touch);assert(begin>=0&&touch>begin&&after>touch);assert(source.includes('after[field]-beforeTouch[field]'));assert(source.includes('after.nav,beforeTouch.nav'));assert(source.includes('view.convertToNodeSpaceAR(w)'));assert(source.includes('fixedButtonOverlap:false'));
 for(const regex of[/\.route\s*\(/,/\.(?:economic|warehouseAction|prepare|reload|refreshGame|committed|setItems|scrollToOffset|setGroundTarget)\s*\(/,/\.selectedBag\s*=(?!=)/,/\.selectedSlot\s*=(?!=)/,/new\s+cc\.Node/])assert(!regex.test(source),String(regex));assert(source.includes("JSON.stringify(asset.json)"));assert(source.includes('assert.deepStrictEqual(JSON.parse(actual),gameplay)'));assert(source.includes("Buffer.from(actual,'utf8')"));assert(!source.includes('canonical'));assert(source.includes('fixtureSha256After'));assert(source.includes('toolSha256After'));assert(source.includes("600000"));assert(source.includes("console.error(redact("));
});

const clone=x=>structuredClone(x),uid=n=>n.toString(16).padStart(32,'0');
const observedItem=(n,quantity='1')=>({id:uid(n),item:900001,quantity,version:'1',bound:true,quality:0,sourceKind:'skill_recycle',sourceId:'original-source',locked:true,flags:1});
const rules={personal_expansion_slots:30,personal_expansion_yuanbao_units:50};
function actionRecord(action,{partial=false}={}){
 const original=observedItem(1,partial?'5':'1'),otherBag=observedItem(2),otherSlot=observedItem(3),amount=partial?'2':'1';
 const before={binding:'9',roleKey:'1:'+uid(99),revision:'5',items:[otherBag],equipped:[],wallet:{gold:'100',boundGold:'7',yuanbaoUnits:'200',boundYuanbaoUnits:'8'},warehouse:{scope:1,revision:'7',capacity:'30',startSlot:'0',limit:30,items:[{slot:'1',item:otherSlot}],gold:'20',yuanbaoUnits:'30'}};
 const after=clone(before);after.revision='6';after.warehouse.revision='8';
 const q={binding:'9',request:'21',opId:uid(9),command:'12',revision:'5',rule:'23',scope:1,action,warehouseRevision:'7',warehouseSlot:'0',itemInstanceId:original.id,itemVersion:'1',quantity:amount,currency:0,amount:'0'};
 const op={binding:q.binding,request:q.request,opId:q.opId,command:q.command,revision:'6',pending:false,error:0},r={binding:q.binding,request:q.request,opId:q.opId,role:before.roleKey,roleRevision:'6',scope:1,action,warehouseRevision:'8',capacity:'30',changes:[],gold:'20',yuanbaoUnits:'30'};
 if(action===1){before.items.unshift(original);if(partial){after.items.unshift({...original,quantity:'3',version:'2'});const child={...original,id:uid(4),quantity:'2'};after.warehouse.items.unshift({slot:'0',item:child});r.changes=[{slot:'0',before:null,after:clone(child)}];}else{const moved={...original,version:'2'};after.warehouse.items.unshift({slot:'0',item:moved});r.changes=[{slot:'0',before:null,after:clone(moved)}];}}
 else if(action===2){before.warehouse.items.unshift({slot:'0',item:original});if(partial){const remainder={...original,quantity:'3',version:'2'},child={...original,id:uid(4),quantity:'2'};after.warehouse.items.unshift({slot:'0',item:remainder});after.items.unshift(child);r.changes=[{slot:'0',before:clone(original),after:clone(remainder)}];}else{after.items.unshift({...original,version:'2'});r.changes=[{slot:'0',before:clone(original),after:null}];}}
 else if(action===3||action===4){q.currency=1;q.amount='5';const sign=action===3?-1n:1n;after.wallet.gold=(100n+sign*5n).toString();after.warehouse.gold=(20n-sign*5n).toString();r.gold=after.warehouse.gold;}
 else if(action===5){after.wallet.yuanbaoUnits='150';after.warehouse.capacity='60';r.capacity='60';}
 return{before,after,request:q,operationResult:op,actionResult:r};
}
test('真实观察合同支持整件/部分存取、货币双向及扩页，全部只核当前背包和已加载页',()=>{
 for(const action of[1,2,3,4,5])for(const partial of(action<=2?[false,true]:[false])){const record=actionRecord(action,{partial});tool.assertWarehouseAction(record,rules);assert.deepEqual(record,clone(record));}
 // 页外其它资产不是本观察器的证据范围；分页窗口不要求覆盖整仓。
 const paged=actionRecord(1);for(const state of[paged.before,paged.after]){state.warehouse.capacity='60';state.warehouse.startSlot='30';state.warehouse.items=state.warehouse.items.map(row=>({...row,slot:(BigInt(row.slot)+30n).toString()}));}paged.request.warehouseSlot='30';paged.actionResult.capacity='60';paged.actionResult.changes[0].slot='30';tool.assertWarehouseAction(paged,rules);
});
test('全取原UID仍留在仓且同时回背包必须拒绝，不能只认原receipt after=null',()=>{
 const record=actionRecord(2);record.after.warehouse.items.unshift(clone(record.before.warehouse.items[0]));assert.throws(()=>tool.assertWarehouseAction(record,rules),/UID重复/);
 const mismatchedAfter=actionRecord(2);mismatchedAfter.after.warehouse.items.unshift({slot:'0',item:observedItem(5)});assert.throws(()=>tool.assertWarehouseAction(mismatchedAfter,rules),/after必须等于/);
});
test('receipt before必须等于真实前页，不能只相信自述原件或缺少的仓位',()=>{
 const forged=actionRecord(2);forged.actionResult.changes[0].before.sourceId='other-source';assert.throws(()=>tool.assertWarehouseAction(forged,rules),/before必须等于/);
 const absent=actionRecord(2);absent.before.warehouse.items.shift();assert.throws(()=>tool.assertWarehouseAction(absent,rules),/before必须等于/);
 const overwrite=actionRecord(1);overwrite.before.warehouse.items.unshift({slot:'0',item:observedItem(5)});assert.throws(()=>tool.assertWarehouseAction(overwrite,rules),/before必须等于/);
});
test('不相干原件、余堆身份与数量、当前UID唯一都属于守恒，不能接受旁路吞物或复制',()=>{
 for(const mutate of[
  r=>{r.after.items[0].sourceId='changed';},
  r=>{r.after.warehouse.items[1].item.quantity='2';},
  r=>{r.after.items.push(observedItem(8));},
  r=>{r.after.warehouse.items.push({slot:'2',item:clone(r.after.items[0])});},
  r=>{r.after.warehouse.items.push(clone(r.after.warehouse.items[0]));},
 ]){const record=actionRecord(1);mutate(record);assert.throws(()=>tool.assertWarehouseAction(record,rules));}
 const partial=actionRecord(1,{partial:true});partial.after.items[0].quantity='4';assert.throws(()=>tool.assertWarehouseAction(partial,rules));
 const recycled=actionRecord(2,{partial:true});recycled.after.items[0].id=uid(2);assert.throws(()=>tool.assertWarehouseAction(recycled,rules),/UID重复/);
});
test('fresh请求与40005/40023必须同request binding op role scope action，角色和仓版本都严格+1',()=>{
 for(const mutate of[
  r=>{r.request.revision='4';},r=>{r.after.revision='7';},r=>{r.operationResult.revision='7';},r=>{r.actionResult.roleRevision='5';},
  r=>{r.actionResult.binding='10';},r=>{r.actionResult.request='22';},r=>{r.operationResult.command='13';},r=>{r.operationResult.pending=true;},
  r=>{r.operationResult.opId=uid(8);},r=>{r.actionResult.role='1:'+uid(98);},r=>{r.actionResult.scope=2;},r=>{r.actionResult.action=2;},r=>{r.actionResult.warehouseRevision='9';},
 ]){const record=actionRecord(1);mutate(record);assert.throws(()=>tool.assertWarehouseAction(record,rules));}
 const large=actionRecord(2);large.before.revision=large.request.revision='9007199254740995';large.after.revision=large.operationResult.revision=large.actionResult.roleRevision='9007199254740996';tool.assertWarehouseAction(large,rules);
});
test('币种/扩页不能混入物品变化、扣绑定钱包、误动其它币种或发生负余额',()=>{
 for(const [action,mutate]of[
  [3,r=>{r.after.wallet.boundGold='6';}],
  [3,r=>{r.after.wallet.yuanbaoUnits='199';}],
  [3,r=>{r.actionResult.changes=[{slot:'0',before:null,after:observedItem(8)}];}],
  [3,r=>{r.after.warehouse.items=[];}],
  [5,r=>{r.after.wallet.gold='99';}],
  [5,r=>{r.after.warehouse.capacity=r.actionResult.capacity='90';}],
 ]){const record=actionRecord(action);mutate(record);assert.throws(()=>tool.assertWarehouseAction(record,rules));}
 const negative=actionRecord(3);negative.request.amount='101';negative.after.wallet.gold='-1';negative.after.warehouse.gold=negative.actionResult.gold='121';assert.throws(()=>tool.assertWarehouseAction(negative,rules));
});
test('方向计算失败前已保留实际rows/view/offset，诊断失败独立保留而不覆盖原错误',()=>{
 const saved=source.indexOf('report.scrolls.push(scroll);'),direction=source.indexOf('const reverse=scrollBackForTarget(',saved);assert(saved>=0&&direction>saved);assert(source.includes('const scroll={listName,key,geometry:result};report.scrolls.push(scroll);scroll.before=await state();'));assert(source.includes('offset:{x:list.scrollView.getScrollOffset().x,y:list.scrollView.getScrollOffset().y}'));assert(source.includes('scroll.reverse=reverse;'));assert(source.includes('report.failureObservationError=redact(observationError.stack||observationError.message)'));assert(source.includes('assertWarehouseAction(record,config)'));
});
