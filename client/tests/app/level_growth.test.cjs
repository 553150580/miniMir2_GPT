const {test}=require('node:test'),assert=require('node:assert/strict');
const {Long,load,harness,entry,ready}=require('./helpers/session_harness.cjs');
const growth=require('../../../.work/ui-tests/app/LevelGrowthModel.js'),{createHudState}=require('../../../.work/ui-tests/app/HudModel.js'),{SystemMessageLog}=require('../../../.work/ui-tests/app/SystemMessageLog.js');
const number=value=>Long.fromString(String(value));
function state(extra={}){return{formatVersion:1,curveVersion:'myf-candidate20',levelCap:50,rebirthCount:0,nextLevelExperience:number(100),pendingExperience:Long.ZERO,pendingExperienceLimit:number(100),legacyExperienceOverflow:Long.ZERO,atCap:false,migrationRequired:false,rebirthEnabled:false,catalogMaxLevel:50,...extra};}
const role='1:'+('09'.repeat(16));
function grant(extra={}){return{role:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(9)},roleRevision:Long.fromInt(11,true),sourceSeq:Long.UONE,curveVersion:'myf-candidate20',rawExperience:number(150),acceptedExperience:number(150),pooledExperience:number(50),rejectedExperience:Long.ZERO,levelBefore:49,levelAfter:50,state:state({atCap:true,pendingExperience:number(50)}),migrated:false,legacyLevelBefore:0,legacyLevelAfter:0,ruleVersion:Long.fromInt(20,true),...extra};}
test('权威50cap/next/pool与历史封存保持Long，未知目录或待整理不给假比例',()=>{
 const s=state();assert.equal(growth.progressionMeter(49,number(25),s).ratio,.25);
 const capped=state({atCap:true,pendingExperience:number(50)});assert.equal(growth.progressionMeter(50,Long.ZERO,capped).ratio,.5);assert(growth.progressionMeter(50,Long.ZERO,capped).text.includes('待用'));
 for(const unknown of [null,state({nextLevelExperience:Long.ZERO}),state({migrationRequired:true,legacyExperienceOverflow:Long.MAX_VALUE,catalogMaxLevel:20})])assert.equal(growth.progressionMeter(70,number(99999),unknown).ratio,0);
 const lines=growth.progressionDetailLines(70,Long.ZERO,state({migrationRequired:true,legacyExperienceOverflow:Long.MAX_VALUE})).join('\n');assert(lines.includes(Long.MAX_VALUE.toString()));assert(lines.includes('保持原等级'));assert(lines.includes('不推定已转生'));assert(lines.includes('不提供假费用'));
 assert.throws(()=>growth.observedProgression(state({pendingExperience:number(101)})),/池上限/);assert.throws(()=>growth.observedProgression(state({pendingExperience:Long.NEG_ONE})),/非负Long/);
 const frozen=state({migrationRequired:true,pendingExperience:number(90),pendingExperienceLimit:Long.ZERO,legacyExperienceOverflow:Long.MAX_VALUE});assert.doesNotThrow(()=>growth.observedProgression(frozen));assert(growth.progressionDetailLines(70,Long.ZERO,frozen).some(line=>line.includes('待用历史余额 90')));assert.equal(growth.progressionMeter(70,Long.ZERO,frozen).ratio,0);
 assert.throws(()=>growth.observedProgression(state({legacyExperienceOverflow:Long.MAX_UNSIGNED_VALUE})),/int64/);
});
test('rule20 HUD只用权威成长状态，不能用旧public阈值掩盖缺失；旧rule19显示兼容',()=>{
 const input={playerHandle:0,role:{job:0,level:49},inventory:{level:49,experience:number(25)},wallet:null,baseline:null,ready:true,auto:false,count:1,map:null,gameplay:{rule_version:20,progression:{curve_version:'myf-candidate20'},experience:[{level:49,next_level_exp:1}]}};
 assert.equal(createHudState(input).experience,0);assert.match(createHudState(input).experienceText,/同步/);
 input.inventory.progression=state();assert.equal(createHudState(input).experience,.25);input.gameplay.progression.curve_version='other';assert.equal(createHudState(input).experience,0);input.gameplay.rule_version=19;assert.equal(createHudState(input).experience,1);
});
test('授予分账raw=accepted+rejected与pooled<=accepted；角色/curve/规则/负数/旧修订不污染水位',()=>{
 const m=new growth.ExperienceGrantModel();m.attach(role);m.synchronize(Long.fromInt(10,true));
 const invalid=[{rawExperience:number(149)},{pooledExperience:number(151)},{rejectedExperience:Long.NEG_ONE},{curveVersion:'old'},{ruleVersion:Long.fromInt(19,true)},{role:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(8)}},{levelAfter:48}];
 for(const extra of invalid)assert.equal(m.accept(grant(extra),role,Long.fromInt(20,true),'myf-candidate20'),false);
 assert(m.accept(grant(),role,Long.fromInt(20,true),'myf-candidate20'));assert.equal(m.notice.raw,'150');assert.equal(m.notice.pooled,'50');assert(!m.accept(grant(),role,Long.fromInt(20,true),'myf-candidate20'));
});
test('真实Session Inventory先到不吞同revision 40019；旧binding/健康重入历史被拒且事件不修改库存经验',()=>{
 const h=harness(),e=entry();e.resourceVersion=Long.fromInt(20,true);ready(h,e);const s=h.session,c=s.connection;s.progressionCurve='myf-candidate20';
 c.deliver(40003,{revision:Long.fromInt(10,true),level:49,experience:number(50),progression:state()});c.deliver(40003,{revision:Long.fromInt(11,true),level:50,experience:Long.ZERO,progression:state({atCap:true,pendingExperience:number(50)})});s.progressionCurve='old-catalog';c.deliver(40019,grant());assert.equal(s.experienceNotice,null);s.progressionCurve='myf-candidate20';c.deliver(40019,grant());assert.equal(s.experienceNotice.accepted,'150');assert.equal(s.inventory.experience.toString(),'0');
 c.deliver(40019,grant({roleRevision:Long.fromInt(12,true)}),{binding:Long.fromInt(2,true)});assert.equal(s.experienceNotice.revision,'11');
 const next=entry('9007199254740994');next.resourceVersion=Long.fromInt(20,true);ready(h,next);s.progressionCurve='myf-candidate20';c.deliver(40003,{revision:Long.fromInt(12,true),level:50,experience:Long.ZERO,progression:state({atCap:true,pendingExperience:number(100)})});c.deliver(40019,grant({roleRevision:Long.fromInt(12,true)}));assert.equal(s.experienceNotice,null);
 c.deliver(40019,grant({roleRevision:Long.fromInt(13,true),rawExperience:number(15),acceptedExperience:Long.ZERO,pooledExperience:Long.ZERO,rejectedExperience:number(15),levelBefore:50,levelAfter:50,state:state({atCap:true,pendingExperience:number(100)})}));assert.equal(s.experienceNotice.rejected,'15');assert.equal(s.experienceNotice.poolFull,true);
});
test('rule20系统消息不从经验/等级差值猜奖励，原授予才分账/升级，旧连接及重复不补播',()=>{
 const log=new SystemMessageLog(),base={roleIdentity:role,ready:true,role:{level:49},entry:{sceneEpoch:'1',baselineId:'1'},inventory:{level:49,experience:number(50),items:[]},gameplay:{rule_version:20}};
 log.observe(base);log.observe({...base,inventory:{level:50,experience:Long.ZERO,items:[]}});assert(!log.messages.some(row=>row.title==='角色升级'||row.title==='经验获得'));
 const m=new growth.ExperienceGrantModel();m.attach(role);m.synchronize(Long.fromInt(10,true));assert(m.accept(grant(),role,Long.fromInt(20,true),'myf-candidate20'));
 log.observe({...base,experienceNotice:m.notice});log.observe({...base,experienceNotice:m.notice});assert.equal(log.messages.filter(row=>row.title==='角色升级').length,1);const summary=log.messages.find(row=>row.title==='经验结算');assert(summary.message.includes('原经验 150'));assert(summary.message.includes('计入待用池 50'));assert(summary.preview.includes('查看分账'));
 m.detach();m.attach(role);m.synchronize(Long.fromInt(11,true));assert(!m.accept(grant(),role,Long.fromInt(20,true),'myf-candidate20'));
});
test('满池只对原事件未接收部分提示，legacy整理不冒充本次奖励升级',()=>{
 const m=new growth.ExperienceGrantModel();m.attach(role);m.synchronize(Long.fromInt(10,true));const full=grant({rawExperience:number(20),acceptedExperience:number(5),pooledExperience:number(5),rejectedExperience:number(15),levelBefore:50,levelAfter:50,migrated:true,legacyLevelBefore:75,legacyLevelAfter:75,state:state({atCap:true,pendingExperience:number(100),legacyExperienceOverflow:number(999)})});
 assert(m.accept(full,role,Long.fromInt(20,true),'myf-candidate20'));const log=new SystemMessageLog();log.observe({roleIdentity:role,ready:true,gameplay:{rule_version:20},experienceNotice:m.notice});assert(log.messages.some(row=>row.title==='待用经验池已满'));assert(log.messages.some(row=>row.title==='历史成长整理'));assert(!log.messages.some(row=>row.title==='角色升级'));
});
test('装备需求只读真实定义：未满足仍可查看，缺字段不猜一级，职业与等级均满足才可穿戴',()=>{
 const d={kind:'equipment',name:'名字里的99级无权威性',required_level:7,allowed_jobs:[0]};assert.match(growth.equipmentLevelLines(d,6,0).join('\n'),/需求等级 7级/);assert.match(growth.equipmentLevelReason(d,6,0),/需7级/);assert.equal(growth.equipmentLevelReason(d,7,0),'');assert.match(growth.equipmentLevelReason({...d,required_level:undefined},99,0),/尚未同步/);assert.match(growth.equipmentLevelReason(d,7,1),/职业/);
});
test('真实AppRoot原穿戴按钮未达需求不发40001，已达只提交原实例；保存Role消息入口读取当前成长并拒旧附着provider',async()=>{
 const h=harness(),e=entry();e.resourceVersion=Long.fromInt(20,true);ready(h,e);const s=h.session;s.inventory={level:6,experience:number(50),revision:Long.UONE,items:[],progression:state()};s.pending={get:()=>null};
 const decorators=new Proxy({},{get(){return(...args)=>args.length>1?undefined:(...v)=>v.length===1?v[0]:undefined;}}),cc={_decorator:decorators,Component:class{},Button:{EventType:{CLICK:'click'}},Toggle:{EventType:{TOGGLE:'toggle'}},EditBox:{EventType:{TEXT_CHANGED:'text-changed'}},Node:{EventType:{TOUCH_END:'touch-end'}}};
 const {AppRoot}=load('client/assets/scripts/app/AppRoot.ts',id=>id==='cc'?cc:id==='long'?Long:id==='./GameSession'?h:id==='./MapTravelModel'?require('../../../.work/ui-tests/app/MapTravelModel.js'):id==='../input/NavigationIntent'?require('../../../.work/ui-tests/input/NavigationIntent.js'):{}),app=new AppRoot(),handlers=new Map(),sent=[];
 app.world={gameplay:{items:[{item_id:777,name:'高阶候选',kind:'equipment',required_level:7,equip_slot:0,allowed_jobs:[0]}]}};app.find=()=>({active:true});app.button=(name,fn)=>handlers.set(name,fn);app.bindGameButtons();app.selectedItem={instanceId:new Uint8Array(16).fill(3),itemId:777,quantity:Long.ONE,version:Long.UONE};s.inventory.items=[app.selectedItem];s.inventory.equipped=[];app.popup={confirm:async()=>true};app.economic=(id,value)=>sent.push({id,value});
 await assert.rejects(handlers.get('UIMgr/Panels/Inventory/Equip')(),/需7级/);assert.equal(sent.length,0);s.inventory.level=7;await handlers.get('UIMgr/Panels/Inventory/Equip')();assert.equal(sent[0].id,40001);assert.deepEqual(sent[0].value.itemInstanceId,app.selectedItem.instanceId);assert.equal(sent[0].value.unequip,false);assert.equal(sent[0].value.itemVersion.toString(),'1');
 let read;app.combatStatus={showLiveDetails(_title,provider){read=provider;}};handlers.get('UIMgr/Panels/Role/Message')();assert(read().some(line=>line.includes('当前等级 7')));s.entry={...s.entry,sceneEpoch:s.entry.sceneEpoch.add(1)};assert.equal(read(),null);
});
test('托管报告显示实际/原/入池/未接收分账，旧报告缺字段不伪造原EXP',()=>{
 const {autoRecycleReportLines}=require('../../../.work/ui-tests/app/AutoRecycleModel.js');const report={usedSeconds:Long.UONE,kills:Long.UONE,experience:number(5),rawExperience:number(20),pooledExperience:number(5),rejectedExperience:number(15)};
 const text=autoRecycleReportLines(report).join('\n');assert(text.includes('原经验 20'));assert(text.includes('实际接收 5'));assert(text.includes('未接收 15'));assert(autoRecycleReportLines({...report,rawExperience:Long.ZERO}).join('\n').includes('旧战报未提供完整经验分账'));
});
