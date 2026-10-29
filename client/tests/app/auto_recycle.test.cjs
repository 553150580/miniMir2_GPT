const {test}=require('node:test'),assert=require('node:assert/strict');
const {Long,harness,entry,ready}=require('./helpers/session_harness.cjs');
const model=require('../../../.work/ui-tests/app/AutoRecycleModel.js');
const policy=(extra={})=>({...model.defaultAutoRecycle(),enabled:true,qualityIds:[0],revision:Long.fromInt(3,true),...extra});
const item=(id,extra={})=>({instanceId:new Uint8Array(16).fill(id),itemId:id===3?193:218,quantity:Long.ONE,bound:true,version:Long.UONE,quality:0,...extra});
function snapshot(extra={}){return{ready:true,role:'1:'+('09'.repeat(16)),scope:'ticket1',rule:Long.fromInt(19,true),revision:Long.fromInt(10,true),policy:policy(),
 items:[item(1,{locked:true,recycleBlockFlags:1}),item(2),item(3)],equipped:[],recycling:{mode:'development_gold_only',outputs:[]},
 definitions:[{item_id:218,name:'木剑',kind:'equipment',required_level:1,quality:0,recycle_gold:50},{item_id:193,name:'布衣',kind:'equipment',required_level:1,quality:0,recycle_gold:400}],...extra};}
test('默认策略关闭；仅保存不估算/消耗当前背包，应用需明确开启',()=>{
 assert.equal(model.defaultAutoRecycle().enabled,false);const s=snapshot();const p=model.createAutoRecyclePreview(s,policy({enabled:false}),false);assert.equal(p.count,0);assert.equal(p.boundGold,0n);assert.equal(s.items.length,3);
 assert.throws(()=>model.createAutoRecyclePreview(s,policy({enabled:false}),true),/明确开启/);
});
test('应用背包按实例品质与原绑定计算，锁定木剑保留，其余预期450绑定金币',()=>{
 const s=snapshot(),p=model.createAutoRecyclePreview(s,policy(),true);assert.equal(p.count,2);assert.equal(p.gold,0n);assert.equal(p.boundGold,450n);assert(p.skipped.some(x=>x.includes('锁定')));assert.equal(s.items[0].locked,true);
 const unbound=snapshot({items:[item(2,{bound:false})]});assert.equal(model.createAutoRecyclePreview(unbound,policy(),true).gold,50n);
});
test('品质是装备0–4；等级0不限，类型/绑定保护与所有强保护不能被自动或手动绕过',()=>{
 const s=snapshot();for(const flags of [1,2,4,8,16]){const x=item(2,{recycleBlockFlags:flags});assert(model.manualRecycleBlockReason(x));assert(!model.autoRecycleMatches(policy(),x,s.definitions[0]));}
 assert(!model.autoRecycleMatches(policy({protectBound:true}),item(2),s.definitions[0]));assert(!model.autoRecycleMatches(policy({protectUnbound:true}),item(2,{bound:false}),s.definitions[0]));
 assert(!model.autoRecycleMatches(policy({protectedItemIds:[218]}),item(2),s.definitions[0]));assert(!model.autoRecycleMatches(policy({maxRequiredLevel:1}),item(2),{...s.definitions[0],required_level:2}));assert(model.autoRecycleMatches(policy({maxRequiredLevel:0}),item(2),{...s.definitions[0],required_level:99}));
 assert.throws(()=>model.observedAutoRecycle(policy({qualityIds:[5]})));assert.throws(()=>model.observedAutoRecycle(policy({qualityIds:[0,0]})));
});
test('未知recipe/需求等级及伪造稀有实例保留并说明，不阻塞其它合法装备',()=>{
 const s=snapshot({items:[item(2,{quality:2}),item(3),item(4,{itemId:777})]});const p=model.createAutoRecyclePreview(s,policy({qualityIds:[0,2]}),true);assert.equal(p.count,1);assert.equal(p.boundGold,400n);assert(p.skipped.some(x=>x.includes('品质分解')));assert(p.skipped.some(x=>x.includes('需求等级未知')));
 assert.throws(()=>model.createAutoRecyclePreview({...s,recycling:undefined},policy(),true),/配置/);
});
test('保护类型使用唯一装备名称，未知/同名歧义/重复均拒绝；恢复为名称',()=>{
 const s=snapshot();assert.deepEqual(model.parseProtectedEquipmentNames('木剑，布衣',s.definitions),[193,218]);assert.equal(model.protectedEquipmentNames([218,193],s.definitions),'木剑，布衣');
 assert.throws(()=>model.parseProtectedEquipmentNames('218',s.definitions),/未知装备/);assert.throws(()=>model.parseProtectedEquipmentNames('木剑',s.definitions.concat({...s.definitions[0],item_id:999})),/歧义/);assert.throws(()=>model.parseProtectedEquipmentNames('木剑,木剑',s.definitions),/重复/);
});
test('确认捕获原角色/附着/库存/策略/规则水位，期间换票或库存变化必须重新确认',()=>{
 const s=snapshot(),p=model.createAutoRecyclePreview(s,policy(),true);model.validateAutoRecyclePreview(p,s);
 for(const change of [{scope:'new-ticket'},{role:'other-role'},{revision:Long.fromInt(11,true)},{policy:policy({revision:Long.fromInt(4,true)})},{rule:Long.fromInt(20,true)}])assert.throws(()=>model.validateAutoRecyclePreview(p,{...s,...change}),/资料已变化/);
});
function result(revision=11,extra={}){return{role:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(9)},roleRevision:Long.fromInt(revision,true),sourceSeq:Long.UONE,opId:new Uint8Array(),policyRevision:Long.UONE,
 recycledCount:Long.UONE,gold:Long.ZERO,boundGold:Long.fromInt(50),discardedCount:Long.UZERO,discardReason:'',trigger:1,...extra};}
test('通知首库存设历史水位，Inventory先于同提交40018仍接受；重复/旧角色/互斥身份/负金额拒绝',()=>{
 const m=new model.EquipmentRecycleModel(),role=snapshot().role;m.attach(role);m.synchronize(Long.fromInt(10,true));assert(m.accept(result(),role));assert(!m.accept(result(),role));
 for(const change of [{opId:new Uint8Array(16)},{gold:Long.NEG_ONE},{role:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(8)}},{trigger:2,opId:new Uint8Array(16),sourceSeq:Long.UONE}])assert(!m.accept(result(12,change),role));
 m.detach();m.attach(role);m.synchronize(Long.fromInt(12,true));assert(!m.accept(result(12),role));assert(m.accept(result(13),role));
});
test('真实Session Inventory后同revision回收消息不误去重；旧binding/不Ready与健康重入历史拒绝',()=>{
 const h=harness(),e=ready(h,entry());const s=h.session,c=s.connection;c.deliver(40003,{revision:Long.fromInt(10,true),items:[]});c.deliver(40003,{revision:Long.fromInt(11,true),items:[]});c.deliver(40018,result());assert.equal(s.recycleNotice.revision,'11');
 c.deliver(40018,result(12),{binding:Long.fromInt(9,true)});assert.equal(s.recycleNotice.revision,'11');s.ready=false;c.deliver(40018,result(12));assert.equal(s.recycleNotice.revision,'11');
 ready(h,entry('9007199254740994'));c.deliver(40003,{revision:Long.fromInt(12,true),items:[]});c.deliver(40018,result(12));assert.equal(s.recycleNotice,null);c.deliver(40018,result(13));assert.equal(s.recycleNotice.revision,'13');
});
test('战报保留Long完整值、原绑定分账与总掉落，100条仅样本，死亡/满包原因不混用',()=>{
 const r={usedSeconds:Long.UONE,kills:Long.UONE,experience:Long.ONE,recycleCount:Long.MAX_UNSIGNED_VALUE,recycleGold:Long.fromInt(50),recycleBoundGold:Long.fromInt(400),discardCount:Long.UONE,lastDiscardReason:'bag_full',totalDropCount:Long.fromInt(1000,true),stopReason:2,drops:Array(100).fill({})};
 const text=model.autoRecycleReportLines(r).join('\n');assert(text.includes(Long.MAX_UNSIGNED_VALUE.toString()));assert(text.includes('总掉落 1000'));assert(text.includes('仅为样本'));assert(text.includes('角色倒下'));assert(text.includes('自动回收后仍无空间'));
 assert(!model.recycleDiscardText('future_reason').includes('背包已满'));
});
test('系统回收提示只来自权威40018观察，不从钱包变多或物品消失推测，重复仅一条',()=>{
 const {SystemMessageLog}=require('../../../.work/ui-tests/app/SystemMessageLog.js'),log=new SystemMessageLog(),role=snapshot().role;
 const base={roleIdentity:role,ready:true,role:{name:'角色'},entry:{sceneEpoch:'1',baselineId:'1'},inventory:{level:1,experience:Long.ZERO,items:[]},wallet:{boundGold:Long.ZERO}};
 log.observe(base);log.observe({...base,wallet:{boundGold:Long.fromInt(50)}});assert.equal(log.messages.filter(x=>x.title==='自动回收').length,0);
 const m=new model.EquipmentRecycleModel();m.attach(role);m.synchronize(Long.fromInt(10,true));assert(m.accept(result(),role));log.observe({...base,recycleNotice:m.notice});log.observe({...base,recycleNotice:m.notice});
 assert.equal(log.messages.filter(x=>x.title==='自动回收').length,1);assert(log.messages.find(x=>x.title==='自动回收').message.includes('绑定金币50'));
});
test('确认文字按保存Label真实尺寸与本机字体度量覆盖32格主配置边界，安全原因完整且不截行',{skip:process.platform!=='win32'},()=>{
 const fs=require('node:fs'),path=require('node:path'),{spawnSync}=require('node:child_process');
 const scene=JSON.parse(fs.readFileSync('client/assets/scenes/Game.scene','utf8')),deref=ref=>scene[ref.__id__];
 let node=scene.find(value=>value.__type__==='cc.Node'&&value._name==='Safe');for(const name of ['PopupView','Panels','Confirm','Message'])node=node._children.map(deref).find(value=>value._name===name);
 const label=node._components.map(deref).find(value=>value.__type__==='cc.Label'),size=node._components.map(deref).find(value=>value.__type__==='cc.UITransform')._contentSize;
 assert.equal(label._enableWrapText,false);assert.equal(label._isSystemFontUsed,true);assert.equal(label._fontSize,24);assert.equal(label._lineHeight,30);
 const gameplay=JSON.parse(fs.readFileSync('client/assets/resources/development/gameplay.json','utf8')),definitions=gameplay.items.filter(row=>row.kind==='equipment');
 assert.equal(definitions.length,2);assert(definitions.every(row=>row.quality===0&&row.required_level===1));
 const samples=[model.createAutoRecyclePreview(snapshot(),policy(),true)];
 for(const binding of [false,true])samples.push(model.createAutoRecyclePreview(snapshot({items:Array.from({length:32},(_,i)=>item(i+1,{itemId:193,bound:binding})),definitions}),policy(),true));
 for(const protectedItemIds of [[],[218],[193],[218,193]])for(const protectBound of [false,true])for(const protectUnbound of [false,true]){
  const items=Array.from({length:32},(_,i)=>item(i+1,{itemId:i%2?193:218,bound:i%3===0,locked:i%5===1,recycleBlockFlags:[0,1,2,4,8][i%5]}));
  samples.push(model.createAutoRecyclePreview(snapshot({items,definitions}),policy({protectedItemIds,protectBound,protectUnbound}),true));
 }
 const allReasons=['装备已锁定','装备有未确认预览','装备正在被占用','装备正在穿戴','装备保护状态尚未识别','按类型保护','按绑定状态保护','需求等级未知','缺少当前品质分解定义','未命中品质或等级筛选','装备数量状态不支持回收'];
 samples.push({...samples[0],skipped:Array.from({length:32},(_,i)=>'木剑：'+allReasons[i%allReasons.length])});
 const texts=samples.map(model.autoRecycleConfirmation);texts.push(model.autoRecycleConfirmation(model.createAutoRecyclePreview(snapshot(),policy({enabled:false}),false)));
 for(const text of texts){const lines=text.split('\n');assert(lines.length<=8,`真实主配置边界不能超8行：${text}`);assert(lines.length*label._lineHeight<=size.height);assert(lines.every(line=>Array.from(line).length<=21));assert(text.includes('未确认预览'));assert(text.includes('回收不可撤销'));assert(text.includes('服务器提交结果为准'));}
 const summary=texts[texts.length-2];for(const reason of ['锁定','未确认预览','占用','穿戴','保护未知','类型保护','绑定保护','等级未知','品质缺配方','筛选不符','数量异常'])assert(summary.replace(/\n/g,'').includes(reason),reason);
 assert.throws(()=>model.autoRecycleConfirmation({...samples[0],skipped:['木剑：'+('未知动态保护原因'.repeat(30))]}),/超出确认窗口/);
 const dir=path.resolve('.work/auto-recycle-20261029/client'),input=path.join(dir,`font-input-${process.pid}.json`),output=path.join(dir,`confirmation-metrics-${process.pid}.json`);
 fs.mkdirSync(dir,{recursive:true});const lines=Array.from(new Set(texts.flatMap(text=>text.split('\n'))));fs.writeFileSync(input,JSON.stringify({lines,font:label._fontFamily,size:label._fontSize}));
 // Windows GDI+执行实际Arial及中文字体度量；不启动Chrome/Creator，不用同一字符串长度算法当作像素宽度。
 const script=`Add-Type -AssemblyName System.Drawing\n$inputData = Get-Content -Encoding UTF8 -Raw -LiteralPath '${input.replace(/'/g,"''")}' | ConvertFrom-Json\n$bitmap = [System.Drawing.Bitmap]::new(1,1)\n$graphics = [System.Drawing.Graphics]::FromImage($bitmap)\n$format = [System.Drawing.StringFormat]::GenericTypographic.Clone()\n$format.FormatFlags = $format.FormatFlags -bor [System.Drawing.StringFormatFlags]::MeasureTrailingSpaces\n$result = @()\ntry { foreach ($family in @($inputData.font, 'Microsoft YaHei', 'SimSun')) { $font = [System.Drawing.Font]::new($family,[single]$inputData.size,[System.Drawing.FontStyle]::Regular,[System.Drawing.GraphicsUnit]::Pixel); try { if ($font.Name -ne $family) { throw 'Missing requested font' }; foreach ($line in $inputData.lines) { $width = $graphics.MeasureString([string]$line,$font,10000,$format).Width; $result += [PSCustomObject]@{font=$font.Name;line=$line;width=[double]$width} } } finally { $font.Dispose() } } } finally { $format.Dispose();$graphics.Dispose();$bitmap.Dispose() }\n$result | ConvertTo-Json -Depth 4 -Compress`;
 const measured=spawnSync('powershell',['-NoProfile','-NonInteractive','-EncodedCommand',Buffer.from(script,'utf16le').toString('base64')],{encoding:'utf8',maxBuffer:4*1024*1024});assert.equal(measured.status,0,measured.stderr);
 assert(measured.stdout.trim(),JSON.stringify({status:measured.status,stderr:measured.stderr,error:measured.error}));
 const metrics=JSON.parse(measured.stdout);assert.equal(metrics.length,lines.length*3);assert(metrics.every(row=>row.width<=size.width),JSON.stringify(metrics.filter(row=>row.width>size.width)));
 fs.writeFileSync(output,JSON.stringify({scope:'保存Confirm真实Label尺寸+本机GDI字体度量；非Creator实构/GPU截图',label:{width:size.width,height:size.height,font:label._fontFamily,fontSize:label._fontSize,lineHeight:label._lineHeight},samples:texts,metrics,maxWidth:Math.max(...metrics.map(row=>row.width)),maxLines:Math.max(...texts.map(text=>text.split('\n').length))},null,2)+'\n');
 console.log('确认字体度量证据：'+path.relative(process.cwd(),output));
});
