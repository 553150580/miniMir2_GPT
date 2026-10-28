const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {createRequire}=require('node:module');
const {TeleportModel,teleportFailureText}=require('../../../.work/ui-tests/app/TeleportModel.js');
const {attachmentKey}=require('../../../.work/ui-tests/app/MapTravelModel.js');
const {root,Long,harness,entry,ready,context,load}=require('./helpers/session_harness.cjs');
const role={originServerId:'9007199254740997',roleId:new Uint8Array(16).fill(9)};
const roleId=role.originServerId+':'+Array.from(role.roleId,v=>v.toString(16).padStart(2,'0')).join('');
const scope=(extra={})=>({connection:7,binding:'9007199254740995',role:roleId,scene:'9007199254740993',baseline:'9007199254740999',map:'1',travelId:'',...extra});
const packet=(extra={})=>({role,fromSceneEpoch:'9007199254740993',sceneEpoch:'9007199254740994',castId:'9007199254740993',playerLife:'2',targetMapId:'2',position:{x:650,y:631},movementRevision:'9007199254740997',outcome:1,transitionId:'ab'.repeat(16),...extra});
const target=(extra={})=>scope({scene:'9007199254740994',baseline:'9007199254741001',map:'2',serverTransitionId:'ab'.repeat(16),...extra});
function model(){const m=new TeleportModel();m.bind(scope());return m;}
function sessionPacket(h,extra={}){return packet({role:h.session.role.key,fromSceneEpoch:h.session.entry.sceneEpoch,sceneEpoch:h.session.entry.sceneEpoch.add(1),playerLife:'0',...extra});}
function baseline(h,e,extra={}){h.session.baseline={sceneEpoch:e.sceneEpoch,baselineId:e.baselineId,stateSequence:Long.UONE,entities:[{entityHandle:7,entityKind:1,hp:Long.ZERO,mp:Long.fromInt(17),life:Long.UZERO,movementRevision:Long.fromString('9007199254740997',true),position:e.position??{x:8,y:8},...extra}]};}

test('瞬移模型完整uint64与精确Entry配对，不能在首视口ACK前ready；一次到达后接受新场景施法',()=>{
 const m=model(),result=packet(),next=target();assert(m.accept(result,scope(),scope().binding,'2'));assert(m.blocksInput);assert.equal(m.state.castId,'9007199254740993');
 assert.equal(m.ready(next),false);assert.equal(m.acknowledge(next),false);assert.equal(m.matchesEntry({...next,travelId:'ab'.repeat(16)},result.position),false);
 for(const change of [{serverTransitionId:''},{serverTransitionId:'cd'.repeat(16)},{scene:'9007199254740995'},{map:'3'},{connection:8},{binding:'8'},{role:'other'}])assert.equal(m.matchesEntry({...next,...change},result.position),false);
 assert.equal(m.matchesEntry(next,{x:651,y:631}),false);m.attach(next,result.position);assert.equal(m.phase,'attaching');assert(m.acknowledge(next));assert.equal(m.acknowledge(next),false);assert(m.ready(next));assert.equal(m.takeArrival(next,'2',result.movementRevision).transitionId,result.transitionId);assert.equal(m.takeArrival(next,'2',result.movementRevision),null);
 assert(m.accept(packet({fromSceneEpoch:next.scene,sceneEpoch:next.scene,castId:'1',targetMapId:'2',transitionId:''}),next,next.binding,'2'));assert.equal(m.phase,'idle');
});
test('瞬移观察拒绝旧连接/角色/Life/scene及乱序cast；不把Number伪装uint64',()=>{
 for(const change of [{role:{...role,roleId:new Uint8Array(16)}},{fromSceneEpoch:'2'},{playerLife:'1'},{castId:9007199254740992},{sceneEpoch:'9007199254740995'},{transitionId:'AB'.repeat(16)},{targetMapId:'1'},{position:{x:-1,y:0}},{movementRevision:'0'}]){const m=model();assert.equal(m.accept(packet(change),scope(),scope().binding,'2'),false,JSON.stringify(change));assert.equal(m.state,null);}
 const m=model(),same=packet({sceneEpoch:scope().scene,transitionId:'',targetMapId:'1'});assert.equal(m.accept(same,scope(),'8','2'),false);assert(m.accept(same,scope(),scope().binding,'2'));assert.equal(m.accept(same,scope(),scope().binding,'2'),false);assert.equal(m.accept({...same,castId:'1'},scope(),scope().binding,'2'),false);
 assert(m.accept({...same,castId:'1',playerLife:'3'},scope(),scope().binding,'3'));
});
test('瞬移失败终态无迁移；同图结果不自行移动；新Life/新revision或健康Entry不重放到达光',()=>{
 for(let outcome=2;outcome<=6;outcome++){const m=model();assert(m.accept(packet({outcome,sceneEpoch:scope().scene,transitionId:'',position:null,movementRevision:'0'}),scope(),scope().binding,'2'));assert.equal(m.blocksInput,false);assert.equal(m.pendingTransfer,null);assert.match(teleportFailureText(outcome),/不会返还/);}
 for(const [life,revision] of [['3','9007199254740997'],['2','9007199254740998']]){const m=model(),next=target();m.accept(packet(),scope(),scope().binding,'2');m.attach(next,packet().position);m.acknowledge(next);m.ready(next);assert.equal(m.takeArrival(next,life,revision),null);assert.equal(m.takeArrival(next,'2',packet().movementRevision),null);}
 const m=model(),next=target();m.accept(packet(),scope(),scope().binding,'2');m.attach(next,packet().position);m.acknowledge(next);m.ready(next);m.bind({...next,connection:8,serverTransitionId:''});assert.equal(m.takeArrival(next,'2',packet().movementRevision),null);
 assert.notEqual(attachmentKey(next),attachmentKey({...next,serverTransitionId:''}));
});
test('真实Session同Life尸体允许已接受瞬移结果；清旧旅行不碰经济Pending或MP，严格配对后ACK',()=>{
 const h=harness(),s=h.session,e=ready(h,entry());baseline(h,e);s.pending={marker:'经济未知提交'};s.inventory={revision:Long.fromInt(19)};const inv=s.inventory,pending=s.pending,mp=s.baseline.entities[0].mp;
 const c=context(h);s.travel.begin(c,'cd'.repeat(16),1);const result=sessionPacket(h);s.connection.deliver(30014,result);assert.equal(s.ready,false);assert.equal(s.travel.active,false);assert.equal(s.pending,pending);assert.equal(s.inventory,inv);assert.equal(s.baseline.entities[0].mp,mp);
 const next={...entry(e.sceneEpoch.add(1).toString(),'2',2),position:result.position,serverTransitionId:result.transitionId};s.connection.deliver(20002,next);assert.equal(s.entry,next);assert.equal(s.entryTravelId,'');assert.equal(s.entryServerTransitionId,result.transitionId);const identity=h.currentEntryIdentity();s.connection.deliver(6,next);assert.equal(s.ready,false);assert(s.teleport.acknowledge(identity));s.connection.deliver(6,next);assert.equal(s.ready,true);assert.equal(s.teleport.phase,'complete');assert.equal(s.pending,pending);
});
test('真实Session拒绝无前置结果/不匹配的强制Entry，旧消息不能解除迁移屏障；新票普通Entry可恢复',()=>{
 const h=harness(),s=h.session,e=ready(h,entry());baseline(h,e);const result=sessionPacket(h),next={...entry(e.sceneEpoch.add(1).toString(),'2',2),position:result.position,serverTransitionId:result.transitionId};
 s.connection.deliver(20002,next);assert.equal(s.entry,e);s.connection.deliver(30014,result);s.connection.deliver(20002,{...next,serverTransitionId:''});assert.equal(s.entry,e);s.connection.deliver(20002,{...next,position:{x:651,y:631}});assert.equal(s.entry,e);s.connection.deliver(6,e);assert.equal(s.ready,false);
 s.connection.status('断线');s.connection.connected=true;s.connection.binding=Long.fromInt(2,true);const health={...entry('1','2',2),serverTransitionId:''};s.connection.deliver(20002,health);assert.equal(s.entry,health);assert.equal(s.teleport.state,null);assert.equal(s.entryServerTransitionId,'');s.connection.deliver(30014,result);assert.equal(s.teleport.state,null);
});
test('真实Session同图success/failure只记录观察，不改Entry、baseline、MP或经济counter',()=>{
 const h=harness(),s=h.session,e=ready(h,entry());baseline(h,e);const b=s.baseline;s.command=Long.fromInt(20,true);
 for(const [castId,outcome] of [['1',1],['2',3]]){const result=sessionPacket(h,{sceneEpoch:e.sceneEpoch,targetMapId:e.roomId,transitionId:'',castId,outcome});s.connection.deliver(30014,result);assert.equal(s.teleport.state.outcome,outcome);assert.equal(s.entry,e);assert.equal(s.baseline,b);assert.equal(s.ready,true);assert.equal(s.command.toString(),'20');assert.equal(b.entities[0].mp.toString(),'17');assert.deepEqual(b.entities[0].position,{x:8,y:8});}
});
function appHarness(){const file=path.join(root,'client/tests/app/map_travel_runtime.test.cjs'),module={exports:{}};const source=fs.readFileSync(file,'utf8').split('\ntest(')[0];new Function('require','module','exports',source+'\nmodule.exports={appHarness};')(createRequire(file),module,module.exports);return module.exports.appHarness();}
const deferred=()=>{let resolve;const promise=new Promise(r=>resolve=r);return {promise,resolve};};
const flush=()=>new Promise(resolve=>setImmediate(resolve));
test('真实AppRoot跨图先阻旧输入/释放手动预备owner，Entry后按实际落点准备；资源未好不ACK',async()=>{
 const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const old=t.a.world,oldBaseline=s.baseline;const id='cd'.repeat(16);s.travel.begin(context(t.h),id,1);const prepared=t.asset('2',true);let primeReleased=0;t.a.preparedTravel={id,assets:prepared,viewport:{release(){primeReleased++;}}};
 const result=sessionPacket(t.h);s.connection.deliver(30014,result);t.a.handleSessionUpdate();assert.equal(t.a.canSendInput(),false);assert.equal(s.baseline,oldBaseline);assert.equal(t.a.world,old);assert.equal(prepared.disposed,1);assert.equal(primeReleased,1);
 s.connection.sent.length=0;const next={...entry('2','2',2),position:result.position,serverTransitionId:result.transitionId};s.connection.deliver(20002,next);baseline(t.h,next);const gate=deferred();t.setGate(gate);const task=t.a.attachCurrentEntry();await flush();assert.equal(old.disposed,1);assert.equal(s.connection.sent.length,0);assert.deepEqual(t.a.world.position,result.position);s.connection.deliver(6,next);assert.equal(s.ready,false);
 gate.resolve();await task;assert.deepEqual(s.connection.sent.map(p=>p.id),[4]);assert.equal(s.teleport.phase,'awaiting_ready');assert.equal(t.a.canSendInput(),false);s.connection.deliver(6,next);t.a.handleSessionUpdate();assert.equal(t.a.canSendInput(),true);t.a.attachment.dispose();
});
test('真实AppRoot旧Entry资源晚到不覆盖已配对瞬移目标，不发送旧ACK',async()=>{
 const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);const late=deferred();t.pendingLoads.push(late);const stale=t.a.attachCurrentEntry();await flush();const result=sessionPacket(t.h);s.connection.deliver(30014,result);const next={...entry('2','2',2),position:result.position,serverTransitionId:result.transitionId};s.connection.deliver(20002,next);baseline(t.h,next);await t.a.attachCurrentEntry();const current=t.a.world,old=t.asset('1');late.resolve(old);await assert.rejects(stale,/失效|过期/);assert.equal(old.disposed,1);assert.equal(t.a.world,current);assert.deepEqual(s.connection.sent.map(p=>p.value.sceneEpoch.toString()),['2']);t.a.attachment.dispose();
});

test('失败系统消息只观察同角色/scene/Life的确定结果且每次cast只提示一次，健康基线不补播',()=>{
 const {SystemMessageLog}=require('../../../.work/ui-tests/app/SystemMessageLog.js'),log=new SystemMessageLog();const result=packet({sceneEpoch:scope().scene,transitionId:'',outcome:3});
 const snapshot={roleIdentity:roleId,ready:true,entry:{sceneEpoch:scope().scene,baselineId:'1',playerHandle:7},baseline:{entities:[{entityHandle:7,entityKind:1,life:'2',hp:'100'}]}};
 log.observe(snapshot);log.observe({...snapshot,teleportNotice:{...result,role:roleId}});log.observe({...snapshot,teleportNotice:{...result,role:roleId}});assert.equal(log.messages.filter(v=>v.title==='瞬移未完成').length,1);
 for(const extra of [{role:'other'},{fromSceneEpoch:'1'},{playerLife:'3'},{outcome:1}])log.observe({...snapshot,teleportNotice:{...result,role:roleId,castId:'9007199254740994',...extra}});
 assert.equal(log.messages.filter(v=>v.title==='瞬移未完成').length,1);log.observe({...snapshot,teleportNotice:null,entry:{...snapshot.entry,baselineId:'2'}});assert.equal(log.messages.filter(v=>v.title==='瞬移未完成').length,1);
});
test('真实WorldView跨图光只跟匹配结果播放本人入场，不造位移或MP；普通重入不回放',()=>{
 const {makeWorld,state,UITransform}=require('../support/world_runtime.cjs');const f=makeWorld(1);f.assets.mapId='2';const revision=Long.fromString('9007199254740997',true);f.apply([state(1,1,10,8,{hp:Long.ZERO,movementRevision:revision,movementKind:4})]);
 const result={...packet({fromSceneEpoch:'9',sceneEpoch:'1',playerLife:'0',position:{x:10,y:8}}),role:roleId};const player=f.view.visuals.get(1),originalState=player.state,mp=player.state.mp;
 f.view.presentTeleportArrival({...result,playerLife:'1'});f.view.presentTeleportArrival({...result,movementRevision:'9'});assert.equal(f.view.effects.teleports.size,0);f.view.presentTeleportArrival(result);f.update(100);
 const entries=Array.from(f.view.effects.nodes.entries()).filter(([id])=>id.startsWith('arrival:'));assert.equal(entries.length,1);assert(entries[0][0].endsWith(':in'));assert.equal(player.state,originalState);assert.equal(player.state.mp,mp);const record=f.assets.manifest.frames[entries[0][1].sprite.spriteFrame.key];assert.equal(entries[0][1].node.getComponent(UITransform).width,record.width);assert.equal(entries[0][1].node.scale.x,1);f.update(500);assert.equal(f.view.effects.teleports.size,0);f.view.dispose();
 const health=makeWorld(1);health.assets.mapId='2';health.apply([state(1,1,10,8,{movementRevision:revision,movementKind:4})]);health.update(100);assert.equal(health.view.effects.teleports.size,0);health.view.dispose();
});
test('真实AppRoot瞬移CastIntent只有技能与输入身份，没有客户端地图/坐标/目标',()=>{
 const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);t.a.ensureNotLeaving=()=>{};t.a.submitCast({definition:{id:21,kind:'teleport'}},42,{x:999,y:999},7);const sent=s.connection.sent.at(-1);assert.equal(sent.id,30003);assert.deepEqual(Object.keys(sent.value).sort(),['inputSequence','sceneEpoch','skillId']);assert.equal(sent.value.skillId,21);
});

test('传送耗蓝取本人权威最大百分比后整数增加，其它技能与0秒尚未移除状态保持正确',()=>{
 const {skillCost,skillRows,validateCaster,skillDescriptionLines}=require('../../../.work/ui-tests/app/SkillModel.js');const {statusLines}=require('../../../.work/ui-tests/app/CombatStatusModel.js');
 const definition={id:21,name:'瞬息移动',kind:'teleport',job_id:1,initial_level:0,active:true,base_cost:10,level_cost:3,target_kind:'friendly_self',buff_id:21,learn_level:19};
 assert.deepEqual([0,1,2,3].map(level=>skillCost(definition,level,30)),[13,16,20,24]);assert.equal(skillCost({...definition,kind:'fireball'},1,30),13);assert.throws(()=>skillCost(definition,1,NaN),/状态/);
 const status={statusId:21,sourceSkillId:21,skillLevel:1,remainingMs:0,teleportManaPenaltyPercent:30,maxPhysicalDefenceBonus:'0',maxMagicDefenceBonus:'0',damageReductionPercent:0,casterHandle:7,casterLife:'2'};
 const snapshot={job:1,ready:true,definitions:[definition],inventory:{level:19,learnedSkills:[{skillId:21,level:1}],items:[]},playerStatuses:[status,{teleportManaPenaltyPercent:20}],mapNoTeleport:true,statusDefinitions:[{id:21,duration_rule:{kind:'fixed_ms',duration_ms:30000},attribute_effects:{value_by_level:[30,30,30,30]}}]};
 const row=skillRows(snapshot)[0];assert.equal(row.cost,16);assert.throws(()=>validateCaster(row,[{entityHandle:7,hp:'100',mp:'15'}],7),/魔法不足/);assert.throws(()=>validateCaster(row,[{entityHandle:7,hp:'0',mp:'100'}],7),/已倒下/);
 const lines=skillDescriptionLines(row,snapshot).join('\n');assert.match(lines,/30秒/);assert.match(lines,/当前地图禁止/);assert.doesNotMatch(lines,/射程 0/);const statusText=statusLines(status,0,1000,snapshot.statusDefinitions).join('\n');assert.match(statusText,/传送技能魔法消耗 \+30%/);assert.doesNotMatch(statusText,/伤害减免/);
 assert.equal(skillRows({...snapshot,playerStatuses:[]})[0].cost,13);
});
test('真实AppRoot新Entry仅在ACK/SyncReady后消费一次跨图到达表现，后续刷新及健康入场不重放',async()=>{
 const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);const result=sessionPacket(t.h);s.connection.deliver(30014,result);const next={...entry('2','2',2),position:result.position,serverTransitionId:result.transitionId};s.connection.deliver(20002,next);baseline(t.h,next);await t.a.attachCurrentEntry();const arrivals=[];t.a.world.presentTeleportArrival=value=>arrivals.push(value);t.a.quickSkill=()=>null;t.a.skillSnapshot=()=>({});t.a.skillAssetsSnapshot=()=>({});t.a.combatStatus.refresh=()=>{};
 t.realRefresh();assert.equal(arrivals.length,0);s.connection.deliver(6,next);t.a.world.isMapReady=false;t.realRefresh();assert.equal(arrivals.length,0);t.a.world.isMapReady=true;t.realRefresh();t.realRefresh();assert.equal(arrivals.length,1);assert.equal(arrivals[0].transitionId,result.transitionId);const health={...entry('3','2',2),serverTransitionId:''};s.connection.deliver(20002,health);baseline(t.h,health);await t.a.attachCurrentEntry();t.a.world.presentTeleportArrival=value=>arrivals.push(value);t.a.combatStatus.refresh=()=>{};s.connection.deliver(6,health);t.realRefresh();assert.equal(arrivals.length,1);t.a.attachment.dispose();
});

test('传送起手只用服务端原格展示，空目标不产生异常或本地目的地，快捷槽为保存宽度提供短名',()=>{
 const {CastPlayback,effectPoses}=require('../../../.work/ui-tests/app/CastPlayback.js');const {skillQuickLabel}=require('../../../.work/ui-tests/app/SkillModel.js');const p=new CastPlayback();p.bind('1');const definition={id:21,kind:'teleport',active:true,target_kind:'friendly_self',cast_ms:600},event={sceneEpoch:'1',castId:'1',skillId:21,origin:{x:8,y:8},targetGrid:null,delayMs:200};
 assert(p.accept(event,definition,0));assert.deepEqual(p.casts[0].event.targetGrid,event.origin);assert.equal(event.targetGrid,null);assert.deepEqual(effectPoses(p.casts[0],100).map(v=>v.kind),['cast']);assert.equal(skillQuickLabel({...definition,name:'瞬息移动'}),'瞬移');const q=new CastPlayback();q.bind('1');assert.equal(q.accept({...event,skillId:1},{...definition,id:1,kind:'fireball'},0),false);
});

test('真实Session角色已清除的迟到30014静默丢弃，不抛身份异常或留下迁移屏障',()=>{
 const h=harness(),e=ready(h,entry());baseline(h,e);const result=sessionPacket(h);h.session.role=null;assert.doesNotThrow(()=>h.session.connection.deliver(30014,result));assert.equal(h.session.teleport.state,null);assert.equal(h.session.teleport.blocksInput,false);
});
