const {test}=require('node:test');
const assert=require('node:assert/strict');
const {Long,load,harness,entry,ready,context,ProtocolResponseError}=require('./helpers/session_harness.cjs');
const {MapAttachmentCoordinator}=require('../../../.work/ui-tests/app/MapAttachmentCoordinator.js');
const travelModel=require('../../../.work/ui-tests/app/MapTravelModel.js');
const townMap=load('client/assets/scripts/app/TownMapModel.ts',()=>travelModel);
const {NavigationIntent}=require('../../../.work/ui-tests/input/NavigationIntent.js');
const {MapViewportPreparation}=load('client/assets/scripts/app/MapViewportPreparation.ts',()=>{});
const deferred=()=>{let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b;});return {promise,resolve,reject};};
const flush=()=>new Promise(r=>setImmediate(r));
function appHarness(options={}){
    const h=harness(options),log=[],views=[],loads=[],pendingLoads=[];let gate,operationSerial=6;
    const decorator=(...a)=>a.length>1?undefined:(...p)=>p.length===1?p[0]:undefined;
    class UITransform{} class MapTouchNavigator{} class TouchJoystick{} class Button{} class Toggle{} class Label{} class List{}
    const navigator={enabled:true,bind(nav){this.nav=nav;}},joystick={enabled:true,bind(nav){this.nav=nav;},release(){this.nav?.releaseJoystick();}};
    const savedWorld={scale:{x:1,y:1},getChildByName:()=>({})};
    const viewport={getChildByName:()=>savedWorld,getComponent:token=>token===UITransform?{width:768,height:512}:navigator};
    const nodes=new Map();const getNode=path=>{if(!nodes.has(path)){const component={interactable:true,setItems(){}};const node={active:false,on(){},getComponent:()=>component};component.node=node;nodes.set(path,node);}return nodes.get(path);};
    class View{constructor(_v,world,_e,_name,_job,scene){this.world=world;this.sceneEpoch=scene;this.isMapReady=false;this.disposed=0;views.push(this);log.push('mount:'+scene);}bindPresentation(identity,current,retire){this.presentationIdentity=identity;this.presentationCurrent=current;this.retirePresentation=retire;}observeEquipment(identity,playerHandle,inventory){this.equipmentObservation={identity,playerHandle,inventory};}prepare(assets){this.assets=assets;this.manifest=assets.manifest;this.gameplay=assets.gameplay;this.map=assets.map;}apply(b){this.baseline=b;}update(){}async prepareViewport(position){this.position=position;log.push('viewport:'+this.sceneEpoch);await gate?.promise;this.isMapReady=true;}dispose(){if(this.disposed++)return;log.push('view.dispose:'+this.sceneEpoch);this.assets?.dispose();}
        presentTeleportDeparture(value){this.teleportDeparture=value;}
        takeAssets(kind, forReturn){
            if(!this.isMapReady || !this.assets || (kind!==undefined && this.assets.sceneKind!==kind) || (forReturn && !this.assets.canRetainAfterTravel))return;
            const assets=this.assets;this.assets=undefined;return assets;
        }
    }
    const asset=(map,source=false,sceneKind=source?2:1)=>({listeners:new Set(),onBeforeInvalidation(listener){this.listeners.add(listener);return ()=>this.listeners.delete(listener);},mapId:map,sceneKind,map:{width:source?700:17,height:source?700:17},gameplay:{rule_version:13,skills:[],items:[],town_travel:{enabled:true}},background:null,statusBadgePrefab:{},sourceMap:source?{model:{manifest:{pixel_width:33600,pixel_height:22400},project:(x,y)=>({x:x*48,y:y*32}),queryChunks:()=>[{id:'owner'}]},acquireChunk(){log.push('prime.acquire');return {ready:Promise.resolve({}),release(){log.push('prime.release');}};}}:undefined,disposed:0,dispose(){this.disposed++;for(const listener of Array.from(this.listeners))listener('dispose');this.listeners.clear();log.push('asset.dispose:'+map);}});
    const WorldAssets={prepare:async(_job,_progress,map)=>{loads.push(map?.toString());const pending=pendingLoads.shift();return pending?pending.promise:asset(map.toString(),map.toString()==='2');}};
    const cc={_decorator:new Proxy({},{get:()=>decorator}),Component:class{},UITransform,Button,Toggle,Label,Game:{EVENT_HIDE:'hide'},game:{totalTime:0,off(){}},sys:{localStorage:options.storage}};
    const {AppRoot}=load('client/assets/scripts/app/AppRoot.ts',id=>id==='cc'?cc:id==='long'?Long:id==='./GameSession'?h:id==='./GameWorldView'?{GameWorldView:View}:id==='./WorldAssets'?{WorldAssets}:id==='./MapAttachmentCoordinator'?{MapAttachmentCoordinator,...require('../../../.work/ui-tests/app/MapAttachmentCoordinator.js')}:id==='./PetBornModel'?require('../../../.work/ui-tests/app/PetBornModel.js'):id==='./MapTravelModel'?travelModel:id==='./TownMapModel'?townMap:id==='./MapViewportPreparation'?{MapViewportPreparation}:id==='./RoleBindingModel'?require('../../../.work/ui-tests/app/RoleBindingModel.js'):id==='../input/NavigationIntent'?{NavigationIntent}:id==='../input/MapTouchNavigator'?{MapTouchNavigator}:id==='../ui/List'?{List}:id==='./GameConnection'?{newOperationId:()=>new Uint8Array(16).fill(++operationSerial),ProtocolResponseError}:id==='./GameCombatStatus'?{GameCombatStatus:class{dispose(){this.disposed=(this.disposed??0)+1;}update(){} }}:{});
    const a=new AppRoot();a.node={getChildByName:()=>({})};a.find=path=>path==='MapViewport'?viewport:getNode(path);a.joystick=joystick;a.attachment=new MapAttachmentCoordinator();a.hud={setMapFrame(){log.push('hud');},refresh(){}};a.ui={};a.popup={};a.showMapLoading=()=>{};const realRefresh=a.refreshGame.bind(a);a.refreshGame=()=>{};const realError=a.error.bind(a);a.error=error=>log.push('error:'+error.message);a.status=()=>{};
    return {h,a,log,views,loads,pendingLoads,asset,getNode,realRefresh,realError,setGate(value){gate=value;},baseline(e){h.session.baseline={sceneEpoch:e.sceneEpoch,baselineId:e.baselineId,stateSequence:Long.UONE,entities:[{entityHandle:7,entityKind:1,hp:Long.fromInt(100),life:Long.UZERO,position:e.roomId.toString()==='2'?{x:633,y:613}:{x:8,y:8}}]};}};
}

// 使用权威首领进场和返回帧驱动真实附着逻辑，资源本体仍替换成可数引用的测试资源。
async function enterBossWithPersonalAssets(t, occurrence) {
    const {h,a}=t,s=h.session,personal=ready(h,entry('1'));
    t.baseline(personal);
    const home=t.asset('1'),boss=t.asset('81',false,4);
    s.preparedWorld=home;
    await a.attachCurrentEntry();
    const target={...entry('2','81',4),serverTransitionId:occurrence};
    t.pendingLoads.push({promise:Promise.resolve(boss)});
    s.connection.deliver(20020,{occurrenceId:occurrence,state:2,sceneEpoch:target.sceneEpoch});
    s.connection.deliver(20002,target);t.baseline(target);
    await a.attachCurrentEntry();
    s.connection.deliver(6,target);
    return {home,boss};
}

test('首领掉线重接和主动返回交接个人图及首领图的同一资源，并各自只释放一次',async test=>{
    const t=appHarness(),s=t.h.session,a=t.a,occurrence='13'.repeat(16);
    test.after(()=>{a.releaseBossReconnect();a.releaseBossReturn();a.attachment.dispose();});
    const {home,boss}=await enterBossWithPersonalAssets(t,occurrence);
    assert.deepEqual(t.loads,['81']);
    assert.equal(t.views[0].disposed,1);
    assert.equal(home.disposed,0);
    assert.equal(a.bossReturn.assets,home);

    s.connection.status('掉线');
    a.handleSessionUpdate();
    s.connection.binding=Long.fromInt(2,true);s.connection.connected=true;
    const ordinary=entry('3');
    s.connection.deliver(20002,ordinary);t.baseline(ordinary);
    await a.attachCurrentEntry();
    s.connection.deliver(6,ordinary);
    assert.equal(a.world.assets,home);
    assert.equal(a.bossReconnect.assets,boss);
    assert.equal(boss.disposed,0);

    const resumed={...entry('4','81',4),serverTransitionId:occurrence};
    s.connection.deliver(20020,{occurrenceId:occurrence,state:2,sceneEpoch:resumed.sceneEpoch});
    s.connection.deliver(20002,resumed);t.baseline(resumed);
    await a.attachCurrentEntry();
    s.connection.deliver(6,resumed);
    assert.equal(a.world.assets,boss);
    assert.equal(a.bossReconnect,undefined);

    const returned={...entry('5'),serverTransitionId:occurrence};
    s.connection.deliver(20020,{occurrenceId:occurrence,state:4,sceneEpoch:resumed.sceneEpoch});
    s.connection.deliver(20002,returned);t.baseline(returned);
    await a.attachCurrentEntry();
    assert.deepEqual(t.loads,['81'],'两次重连、返回均复用已经持有的地图资源');
    assert.equal(a.world.assets,home);
    assert.equal(a.bossReturn,undefined);
    assert.equal(boss.disposed,1);
    assert.equal(home.disposed,0);
    a.attachment.dispose();
    assert.equal(home.disposed,1);
});

test('首领返回缓存只交给原角色、原地图与原规则，不匹配时独立释放',async()=>{
    for(const mismatch of ['角色','地图','规则']){
        const t=appHarness(),s=t.h.session,a=t.a;
        const {home}=await enterBossWithPersonalAssets(t,'17'.repeat(16));
        const captured=a.attached;
        const returnTo={...captured,map:mismatch==='地图'?Long.fromInt(7,true):Long.UONE,
            role:mismatch==='角色'?'其它角色':captured.role};
        s.entry=entry('3');
        assert.equal(a.takeBossReturn(returnTo,mismatch==='规则'?'29':'13'),undefined);
        assert.equal(a.bossReturn,undefined);
        assert.equal(home.disposed,1);
        a.releaseBossReturn();
        a.attachment.dispose();
        assert.equal(home.disposed,1);
    }
});

test('规则24地图目录选中城镇或野外即带原Long目标进入真实旅行，不再弹旧确认', async () => {
    const t=appHarness(),s=t.h.session,rule=Long.fromInt(24,true);
    const gameplay={rule_version:24, skills:[],items:[],town_travel:{enabled:true,destination_map_ids:['3','4']},
        map_catalog:[{id:'1',name:'林间战场',scene_kind:1},{id:'3',name:'山谷村',scene_kind:2,compact_town:true},{id:'4',name:'边界村',scene_kind:2,compact_town:true}],
        map_regions:[{id:'0',name:'比奇省',map_ids:['1','3','4']}]};
    const initial={...entry('1'),resourceVersion:rule};ready(t.h,initial);t.baseline(initial);
    s.preparedWorld=Object.assign(t.asset('1'),{gameplay});await t.a.attachCurrentEntry();
    const panel={};let shown;
    t.a.townDirectory={panel,show(regions){shown=regions;}};
    t.a.ui={top:null,open(name){assert.equal(name,'TownDirectory');this.top=panel;},back(){this.top=null;}};
    t.a.popup={confirm(){assert.fail('选地图已确认，不弹旧往返窗');}};
    const requests=[];
    let prepared;
    s.connection.request=async(code,value)=>{
        requests.push({code,value});
        if(code===20007){
            assert(Long.isLong(value.targetMapId));prepared=value;
            return {travelId:value.travelId,fromSceneEpoch:value.sceneEpoch,targetMapId:value.targetMapId,targetSceneKind:value.destination===1?2:1,
                position:{x:8,y:8},expiresAtUnixMs:String(Date.now()+60000)};
        }
        assert.equal(code,20009);assert.equal(value.travelId,prepared.travelId);assert.equal(value.cancel,false);
        const scene=value.sceneEpoch.add(1),committed={travelId:value.travelId,fromSceneEpoch:value.sceneEpoch,sceneEpoch:scene,phase:2,targetMapId:prepared.targetMapId};
        s.connection.deliver(20010,committed);
        const next={...entry(scene.toString(),prepared.targetMapId.toString(),prepared.destination===1?2:1),resourceVersion:rule};
        s.connection.deliver(20002,next);t.baseline(next);await t.a.attachCurrentEntry();s.connection.deliver(6,next);
        return committed;
    };
    for(const [map,kind] of [['4',2],['1',1]]){
        t.a.openMapDirectory();assert.equal(t.a.ui.top,panel);
        const rows=shown[0].maps,chosen=rows.find(row=>row.mapId===map);
        assert.equal(rows.find(row=>row.current).mapId,s.entry.roomId.toString());assert.equal(chosen.current,false);
        t.pendingLoads.push({promise:Promise.resolve(Object.assign(t.asset(map,false,kind),{gameplay}))});
        await t.a.travelMap(chosen);
        assert.equal(s.entry.roomId.toString(),map);assert.equal(t.a.world.map.width,17);assert.equal(t.a.ui.top,null);
    }
    assert.deepEqual(requests.filter(request=>request.code===20007).map(({value})=>[value.destination,value.targetMapId.toString()]),[[1,'4'],[2,'1']]);
    t.a.attachment.dispose();
});
test('真实AppRoot初次Town重入使用Entry.roomId替换默认primary，首视口完成前绝不ACK',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1','2',2)),warm=t.asset('1');t.baseline(e);s.preparedWorld=warm;const gate=deferred();t.setGate(gate);
    const task=t.a.attachCurrentEntry();await flush();assert.deepEqual(t.loads,['2']);assert.equal(warm.disposed,1);assert.equal(t.views.length,1);assert.equal(t.views[0].map.width,700);assert.equal(s.connection.sent.length,0);assert.equal(t.a.canSendInput(),false);
    gate.resolve();await task;assert.deepEqual(s.connection.sent.map(p=>p.id),[4]);assert.equal(s.connection.sent[0].value.sceneEpoch.toString(),'1');assert.deepEqual(t.views[0].position,{x:633,y:613});assert.equal(t.a.canSendInput(),true);t.a.attachment.dispose();
});
test('快速返回沿用原地图资源，但仍等权威计划、最新基线和首视口才发送ACK',async test=>{
    const t=appHarness(),s=t.h.session,a=t.a,initial=ready(t.h,entry('1'));
    const home=Object.assign(t.asset('1'),{canRetainAfterTravel:true});
    const town=Object.assign(t.asset('2',false,2),{canRetainAfterTravel:true});
    s.preparedWorld=home;t.baseline(initial);await a.attachCurrentEntry();
    test.after(()=>{a.attachment.dispose();a.releaseRetainedMap();});
    t.pendingLoads.push({promise:Promise.resolve(town)});
    a.popup={confirm:async()=>true};
    const requests=[];let plan;
    s.connection.request=async(code,value)=>{
        requests.push(code);
        if(code===20007){
            plan={travelId:value.travelId,fromSceneEpoch:value.sceneEpoch,targetMapId:Long.fromInt(value.destination===1?2:1,true),
                targetSceneKind:value.destination===1?2:1,position:{x:8,y:8},expiresAtUnixMs:String(Date.now()+60000)};
            return plan;
        }
        assert.equal(code,20009);assert.equal(value.travelId,plan.travelId);
        const scene=plan.fromSceneEpoch.add(1),committed={...plan,sceneEpoch:scene,phase:2};
        s.connection.deliver(20010,committed);
        const next=entry(scene.toString(),plan.targetMapId.toString(),plan.targetSceneKind);
        s.connection.deliver(20002,next);t.baseline(next);await a.attachCurrentEntry();s.connection.deliver(6,next);
        return committed;
    };
    await a.travelMap();
    assert.equal(a.retainedMap.assets,home);assert.equal(home.disposed,0);
    assert.equal(a.takeRetainedMap('其他角色','13','1',1),undefined);
    assert.equal(a.takeRetainedMap(t.h.roleIdentity(),'14','1',1),undefined);
    const gate=deferred();t.setGate(gate);
    const returned=a.travelMap();await flush();
    assert.deepEqual(t.loads,['2']);assert.deepEqual(requests,[20007,20009,20007,20009]);
    assert.equal(a.canSendInput(),false);assert.deepEqual(s.connection.sent.map(frame=>frame.value.sceneEpoch.toString()),['1','2']);
    gate.resolve();await returned;
    assert.equal(a.world.assets,home);assert.equal(a.retainedMap.assets,town);
    assert.deepEqual(s.connection.sent.map(frame=>frame.value.sceneEpoch.toString()),['1','2','3']);
    a.releaseRetainedMap();assert.equal(town.disposed,1);assert.equal(home.disposed,0);
});
test('地图资源准备后的真实ACK只携当前角色已保存的自动施法偏好',async()=>{
    const {saveSkillPreference}=require('../../../.work/ui-tests/app/SkillPreferences.js');
    const values=new Map(),storage={getItem:key=>values.get(key)??null,setItem:(key,value)=>values.set(key,value)};
    const t=appHarness({storage}),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');
    const first=t.h.roleIdentity(),saved={enabled:false,disabledSkillIds:[1,11]};
    saveSkillPreference(storage,first,'auto',saved);
    await t.a.attachCurrentEntry();
    assert.deepEqual(s.connection.sent.at(-1),{id:4,value:{sceneEpoch:e.sceneEpoch,baselineId:e.baselineId,autoSkillSettings:saved}});
    s.role={...s.role,key:{...s.role.key,roleId:new Uint8Array(16).fill(3)}};
    const next=ready(t.h,entry('2'));t.baseline(next);s.preparedWorld=t.asset('1');
    await t.a.attachCurrentEntry();
    assert.deepEqual(s.connection.sent.at(-1),{id:4,value:{sceneEpoch:next.sceneEpoch,baselineId:next.baselineId}});
    t.a.attachment.dispose();
});
test('真实AppRoot先apply完整基线才消费出生，重复refresh/visual重建不重播，旧owner解绑不清新scope',async()=>{
 const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();
 t.a.quickSkill=()=>null;t.a.skillSnapshot=()=>({});t.a.skillAssetsSnapshot=()=>({});t.a.combatStatus.refresh=()=>{};
 const view=t.views[0],calls=[];view.manifest={profiles:{pet_skeleton:{layers:[{actions:{appear:{count:10,duration_ms:100}}}]}}};
 const apply=view.apply.bind(view);view.apply=baseline=>{calls.push('apply');apply(baseline);};view.presentPetBorn=value=>{assert.equal(view.baseline,s.baseline);calls.push('born:'+value.event.entityLife);};
 s.presentationNow=()=>100;s.baseline={...s.baseline,stateSequence:Long.fromInt(2,true),entities:[...s.baseline.entities,{entityHandle:3,entityKind:2,definitionId:1,life:Long.UONE,hp:Long.UONE,movementRevision:Long.UZERO,direction:5}]};
 const identity=t.h.currentEntryIdentity(),event={role:s.role.key,sceneEpoch:e.sceneEpoch,baselineId:e.baselineId,stateSequence:Long.fromInt(2,true),entityHandle:3,entityLife:Long.UONE,definitionId:1,mapId:e.roomId,sourceCastId:Long.UONE,sourceSkillId:17,bornAtUnixMs:Long.UONE,movementRevision:Long.UZERO,direction:5,elapsedMs:200};
 assert(s.petBorn.accept(event,identity,s.connection.binding,100));t.realRefresh();assert.deepEqual(calls,['apply','born:1']);t.realRefresh();assert.deepEqual(calls,['apply','born:1']);
 assert.equal(s.petBorn.accept(event,identity,s.connection.binding,100),false);s.role=null;assert.equal(view.presentationCurrent(),false);assert.doesNotThrow(()=>view.retirePresentation());t.a.attachment.dispose();
});
test('真实Session完整Assembler到AppRoot：quiescing仍观察原World出生，取消不重播且收束期间禁用输入',async()=>{
 const t=appHarness({realBaselines:true}),s=t.h.session,e=entry('1');e.resourceVersion=Long.fromInt(18,true);ready(t.h,e);t.baseline(e);const initial=t.asset('1');initial.gameplay.rule_version=18;s.preparedWorld=initial;await t.a.attachCurrentEntry();
 t.a.quickSkill=()=>null;t.a.skillSnapshot=()=>({});t.a.skillAssetsSnapshot=()=>({});t.a.combatStatus.refresh=()=>{};
 const view=t.views[0],shown=[];view.manifest={profiles:{pet_skeleton:{layers:[{actions:{appear:{count:10,duration_ms:100}}}]}}};view.presentPetBorn=value=>shown.push(value);
 const identity=t.h.currentEntryIdentity(),c=context(t.h),id='ab'.repeat(16),now=Date.now();s.travel.begin(c,id,1);
 s.travel.acceptPlan({travelId:id,fromSceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:String(now+60000)},c,now);s.travel.prepared(c,now);s.travel.commit(c,now);
 const state={travelId:id,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,phase:1,targetMapId:Long.fromInt(2,true)};s.connection.deliver(20010,state);assert.equal(s.ready,false);assert.equal(s.travel.phase,'quiescing');assert.equal(t.a.canSendInput(),false);
 const pet={entityHandle:3,entityKind:2,definitionId:1,life:Long.UONE,hp:Long.fromInt(10),hpMax:Long.fromInt(10),movementRevision:Long.UZERO,direction:5,position:{x:8,y:8}},sequence=Long.fromInt(2,true);
 const chunk={sceneEpoch:e.sceneEpoch,baselineId:e.baselineId,stateSequence:sequence,chunkCount:2,chunkIndex:0,entities:[pet]};s.connection.deliver(5,chunk);assert.equal(s.baseline.stateSequence.toString(),'1');
 s.connection.deliver(5,{...chunk,chunkIndex:1,entities:[]});assert.equal(s.baseline.entities[0].entityHandle,3);
 s.presentationNow=()=>100;const born={role:s.role.key,sceneEpoch:e.sceneEpoch,baselineId:e.baselineId,stateSequence:sequence,entityHandle:3,entityLife:Long.UONE,definitionId:1,mapId:e.roomId,sourceCastId:Long.UONE,sourceSkillId:17,bornAtUnixMs:Long.UONE,movementRevision:Long.UZERO,direction:5,elapsedMs:0};
 s.connection.deliver(30015,born,{binding:Long.fromInt(2,true)});assert.equal(s.petBorn.pendingCount,0,'旧票不能借quiescing通过');
 s.connection.deliver(30015,born);assert.equal(s.petBorn.pendingCount,1);t.realRefresh();assert.equal(shown.length,1);assert.equal(shown[0].event.entityLife,'1');assert.equal(view.baseline,s.baseline);assert.equal(t.a.canSendInput(),false);assert.equal(s.ready,false);assert.deepEqual(s.connection.sent.map(p=>p.id),[4]);
 s.connection.deliver(20010,{...state,phase:3});assert.equal(s.ready,true);assert.equal(s.travel.phase,'cancelled');assert.equal(t.h.currentEntryIdentity().scene.toString(),identity.scene.toString());t.realRefresh();assert.equal(shown.length,1);assert.equal(s.petBorn.pendingCount,0);assert.equal(t.a.canSendInput(),true);
 s.connection.deliver(30015,born);t.realRefresh();assert.equal(shown.length,1,'取消后同birth不重播');
 const next=entry('2','2',2);next.resourceVersion=Long.fromInt(18,true);s.connection.deliver(20002,next);s.connection.deliver(30015,born);assert.equal(s.petBorn.pendingCount,0,'新Entry不能接收原scene出生');t.realRefresh();assert.equal(shown.length,1);t.a.attachment.dispose();
});
test('真实AppRoot同Game场景旅行重挂新world，预备租约持续到真实新视口完成，旧摇杆不能发送',async()=>{
    const t=appHarness(),s=t.h.session,old=ready(t.h,entry('1'));t.baseline(old);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const original=t.views[0];s.connection.sent.length=0;
    const c=context(t.h),id='07'.repeat(16),plan={travelId:id,fromSceneEpoch:old.sceneEpoch,targetMapId:Long.fromInt(2,true),targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:'2000'};
    s.travel.begin(c,id,1);s.travel.acceptPlan(plan,c,1000);const target=t.asset('2',true),prime=new MapViewportPreparation(target);await prime.prepare(plan.position,768,512);t.a.preparedTravel={id,assets:target,viewport:prime};s.travel.prepared(c,1100);s.travel.commit(c,1200);
    s.connection.deliver(20010,{travelId:id,fromSceneEpoch:old.sceneEpoch,sceneEpoch:Long.fromInt(2,true),phase:2,targetMapId:Long.fromInt(2,true)});const next=entry('2','2',2);s.connection.deliver(20002,next);t.baseline(next);const gate=deferred();t.setGate(gate);const task=t.a.attachCurrentEntry();await flush();
    assert.equal(t.views.length,2);assert.equal(original.disposed,1);assert.notEqual(t.views[1],original);assert.equal(t.a.world.sceneEpoch.toString(),'2');assert.equal(t.log.includes('prime.release'),false);assert.equal(t.a.canSendInput(),false);t.a.nav.setJoystick({x:1,y:0});t.a.update(.2);assert.equal(s.connection.sent.length,0);
    gate.resolve();await task;assert.equal(t.log.at(-1),'prime.release');assert.deepEqual(s.connection.sent.map(p=>p.id),[4]);assert.equal(s.connection.sent[0].value.sceneEpoch.toString(),'2');assert.equal(s.travel.phase,'awaiting_ready');assert.equal(t.a.canSendInput(),false);
    s.connection.deliver(6,next);t.a.handleSessionUpdate();assert.equal(s.travel.phase,'complete');assert.equal(t.a.canSendInput(),true);t.a.attachment.dispose();
});
test('真实AppRoot旧加载晚于新scene完成时只释放自身资源，不ACK旧scene或覆盖新world',async()=>{
    const t=appHarness(),s=t.h.session,old=ready(t.h,entry('1'));t.baseline(old);const late=deferred();t.pendingLoads.push(late);const one=t.a.attachCurrentEntry();await flush();
    const next=entry('2','2',2);s.connection.deliver(20002,next);t.baseline(next);await t.a.attachCurrentEntry();const current=t.a.world,stale=t.asset('1');late.resolve(stale);await assert.rejects(one,/失效|过期/);
    assert.equal(stale.disposed,1);assert.equal(t.a.world,current);assert.deepEqual(s.connection.sent.map(p=>p.value.sceneEpoch.toString()),['2']);t.a.attachment.dispose();
});
test('真实AppRoot旅行目标准备失败只cancel原计划；原world、HP、导航目标与经济Pending保持',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const original=t.a.world,baseline=s.baseline,ledger={get:()=>null};s.pending=ledger;
    t.a.nav.setGroundTarget({x:9,y:9});const planId='07'.repeat(16),target=deferred();t.pendingLoads.push(target);const requests=[];
    s.connection.request=async(id,value)=>{requests.push({id,value});if(id===20007)return {travelId:planId,fromSceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:String(Date.now()+60000)};const result={travelId:planId,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,phase:3,targetMapId:Long.fromInt(2,true)};s.connection.deliver(20010,result);return result;};
    const task=t.a.travelMap();await flush();assert.equal(t.a.world,original);assert.equal(t.a.canSendInput(),true);target.reject(Error('资源失败'));await assert.rejects(task,/资源失败/);
    assert.deepEqual(requests.map(r=>r.id),[20007,20009]);assert.equal(requests[1].value.travelId,planId);assert.equal(requests[1].value.cancel,true);assert.equal(requests[1].value.sceneEpoch.toString(),'1');assert.equal(t.a.world,original);assert.equal(original.disposed,0);assert.equal(s.baseline,baseline);assert.equal(s.pending,ledger);assert.deepEqual(t.a.nav.state.target,{x:9,y:9});assert.equal(t.a.canSendInput(),true);t.a.attachment.dispose();
});
test('真实AppRoot提交响应未知时保持输入屏障与目标资源，核对使用原id；服务端取消后恢复原world',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const original=t.a.world,id='07'.repeat(16),requests=[];t.a.popup={confirm:async()=>true};
    s.connection.request=async(code,value)=>{requests.push({code,value});if(code===20007)return {travelId:id,fromSceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:String(Date.now()+60000)};throw Error('提交结果未知');};
    await assert.rejects(t.a.travelMap(),/未知/);assert.equal(s.travel.phase,'uncertain');assert.equal(t.a.canSendInput(),false);assert(t.a.preparedTravel);assert.equal(original.disposed,0);
    s.connection.request=async(code,value)=>{requests.push({code,value});const terminal={travelId:id,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),phase:4};s.connection.deliver(20010,terminal);return terminal;};
    await assert.rejects(t.a.travelMap(),/终止/);assert.deepEqual(requests.map(r=>r.code),[20007,20009,20009]);assert.equal(requests[2].value.travelId,id);assert.equal(requests[2].value.cancel,false);assert.equal(t.a.preparedTravel,undefined);assert.equal(t.a.world,original);assert.equal(t.a.canSendInput(),true);t.a.attachment.dispose();
});

test('真实AppRoot准备中丢失角色/连接后迟到资源仍释放，不升级授权取消、不重挂原world',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const original=t.a.world,pending=deferred();t.pendingLoads.push(pending);
    const requests=[];s.connection.request=async(code,value)=>{requests.push(code);return {travelId:value.travelId,fromSceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:String(Date.now()+60000)};};
    const task=t.a.travelMap();await flush();s.connection.status('断线');s.role=null;const late=t.asset('2',true);pending.resolve(late);await assert.rejects(task,/失效/);assert.equal(late.disposed,1);assert.deepEqual(requests,[20007]);assert.equal(t.a.world,original);assert.equal(original.disposed,0);t.a.attachment.dispose();
});
test('真实AppRoot导航status5仅当前目标产生一次系统消息，不发送新移动或改写最后错误',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const notices=[];t.a.messages={navigationUnreachable:id=>notices.push(id)};
    t.a.nav.setGroundTarget({x:9,y:9});const sequence=t.a.inputSequence;const navigation={inputSequence:sequence,destination:{x:9,y:9},status:5,revision:'9007199254740993'};s.baseline.navigationState=navigation;
    const sends=s.connection.sent.length;s.lastError='保留旧提示';t.a.nav.observeAuthority(navigation);t.a.nav.observeAuthority(navigation);assert.equal(notices.length,1);assert.match(notices[0],/9007199254740993/);assert.equal(s.connection.sent.length,sends);assert.equal(s.lastError,'保留旧提示');assert.equal(t.a.nav.state.target,null);t.a.attachment.dispose();
});
test('真实AppRoot Town刷新不发送AutoBattle，按钮禁用；回个人按真实baseline恢复自动状态和怪数',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1','2',2));t.baseline(e);s.preparedWorld=t.asset('2',true);await t.a.attachCurrentEntry();t.a.quickSkill=()=>null;t.a.skillSnapshot=()=>({});t.a.skillAssetsSnapshot=()=>({});t.a.combatStatus.refresh=()=>{};
    const requests=[];s.connection.request=async(code,value)=>requests.push({code,value});s.hosting={autoBattleEnabled:true};t.realRefresh();assert.equal(t.a.auto,false);assert.equal(t.a.count,1);assert.equal(t.getNode('MapViewport/Actions/Auto').getComponent().interactable,false);assert.equal(t.getNode('MapViewport/Actions/Count').getComponent().interactable,false);await assert.rejects(t.a.autoState(),/城镇不刷怪/);assert.equal(requests.length,0);
    // 当前入口身份改成个人后，沿服务端快照恢复，刷新本身不造初始化请求。
    e.sceneKind=1;s.baseline.entities.push({entityHandle:8,entityKind:3});s.hosting={autoBattleEnabled:true};t.realRefresh();assert.equal(t.a.auto,true);assert.equal(t.a.count,1);assert.equal(t.getNode('MapViewport/Actions/Auto').getComponent().interactable,true);assert.equal(requests.length,0);t.a.attachment.dispose();
});
test('真实AppRoot销毁立即移除基线/旅行等待者，不保留旧ACK监听',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1')),identity=t.h.currentEntryIdentity();s.baseline=null;
    const c=context(t.h),id='07'.repeat(16);s.travel.begin(c,id,1);const baseline=t.a.waitBaseline(identity),travel=t.a.waitTravel(id,c);assert.equal(s.update.size,2);
    const checks=[assert.rejects(baseline,/取消/),assert.rejects(travel,/取消/)];t.a.onDestroy();await Promise.all(checks);assert.equal(s.update.size,0);assert.equal(t.a.mapWaiters.size,0);
});

test('真实AppRoot Prepare明确拒绝解除本地锁，网络未知只沿原id重查后取消，不生成新旅行',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const requests=[];
    s.connection.request=async(code,value)=>{requests.push({code,value});throw new ProtocolResponseError(20008,1001);};await assert.rejects(t.a.travelMap(),ProtocolResponseError);assert.equal(s.travel.active,false);assert.equal(t.a.canSendInput(),true);assert.deepEqual(requests.map(r=>r.code),[20007]);
    s.connection.request=async(code,value)=>{requests.push({code,value});throw Error(code===20007?'Prepare响应未知':'未知计划Conflict');};await assert.rejects(t.a.travelMap(),/Prepare响应未知/);assert.equal(s.travel.phase,'uncertain');assert.equal(s.travel.blocksInput,false);assert.equal(t.a.canSendInput(),true);const id=s.travel.id;
    s.connection.request=async(code,value)=>{requests.push({code,value});if(code===20007)return {travelId:id,fromSceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:String(Date.now()+60000)};const terminal={travelId:id,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),phase:3};s.connection.deliver(20010,terminal);return terminal;};
    await t.a.travelMap();assert.equal(s.travel.phase,'cancelled');assert.deepEqual(requests.slice(-2).map(r=>[r.code,r.value.travelId]),[[20007,id],[20009,id]]);assert.equal(requests.at(-1).value.cancel,true);assert.equal(t.a.canSendInput(),true);t.a.attachment.dispose();
});
test('真实AppRoot Prepare重查直接返回phase4终态时不再补cancel或陷入回执超时',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const id='07'.repeat(16),source=context(t.h);s.travel.begin(source,id,1);s.travel.uncertain();t.a.travelSource=source;t.a.travelDestination=1;
    const requests=[];s.connection.request=async(code,value)=>{requests.push({code,value});const terminal={travelId:id,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),phase:4};s.connection.deliver(20010,terminal);return terminal;};await t.a.travelMap();assert.deepEqual(requests.map(r=>r.code),[20007]);assert.equal(s.travel.active,false);assert.equal(t.a.canSendInput(),true);t.a.attachment.dispose();
});

test('真实AppRoot状态资源失效回调捕获旧controller，迟到旧通知不清新地图controller',async()=>{
    const t=appHarness(),s=t.h.session,old=ready(t.h,entry('1'));t.baseline(old);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const oldStatus=t.a.combatStatus,callbacks=Array.from(t.a.world.assets.listeners);
    const next=entry('2','2',2);s.connection.deliver(20002,next);t.baseline(next);await t.a.attachCurrentEntry();const current=t.a.combatStatus;assert.notEqual(current,oldStatus);for(const callback of callbacks)callback('dispose');assert.equal(t.a.combatStatus,current);assert.equal(current.disposed,undefined);assert(oldStatus.disposed>=1);t.a.attachment.dispose();
});

test('真实AppRoot私有API并发：旧资源/确认迟到的finally只处理原owner，不清新计划资源、Loading或输入',async()=>{
    // 正常保存按钮仍由uiActionBusy串行化；这里直接重入API检验未来调用与异步收尾的归属。
    for(const stage of ['resource','confirm']){
        const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const feedback=[];
        t.a.showMapLoading=(...args)=>feedback.push(['loading',...args]);const barrier=t.a.setInputBarrier.bind(t.a);t.a.setInputBarrier=value=>{feedback.push(['input',value]);barrier(value);};t.a.refreshGame=()=>feedback.push(['refresh']);
        const late=deferred(),oldConfirm=deferred(),newConfirm=deferred();if(stage==='resource')t.pendingLoads.push(late);let confirmations=0;t.a.popup={confirm:()=>stage==='confirm'&&confirmations++===0?oldConfirm.promise:newConfirm.promise};
        s.connection.request=async(code,value)=>{if(code===20007)return {travelId:value.travelId,fromSceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:String(Date.now()+60000)};const result={travelId:value.travelId,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),phase:3};s.connection.deliver(20010,result);return result;};
        const old=t.a.travelMap(),oldDone=assert.rejects(old,error=>error.staleMapAttachment===true);await flush();const oldID=s.travel.id;
        s.connection.deliver(20010,{travelId:oldID,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),phase:4});t.a.handleSessionUpdate();
        const current=t.a.travelMap();await flush();const newID=s.travel.id,owner=t.a.preparedTravel;assert.notEqual(newID,oldID);assert.equal(s.travel.phase,'confirming');assert(owner);
        t.a.showMapLoading(true,'新计划自己的加载状态');const before=feedback.length;s.lastError='新计划保留提示';const retired=t.asset('2',true);
        if(stage==='resource')late.resolve(retired);else oldConfirm.resolve(false);await oldDone;
        assert.equal(t.a.preparedTravel,owner);assert.equal(owner.assets.disposed,0);assert.equal(s.travel.id,newID);assert.equal(s.travel.phase,'confirming');assert.equal(feedback.length,before);
        // 外层Button的统一错误入口同样忽略已标记的过期回调。
        try{await old;}catch(error){t.realError(error);}assert.equal(s.lastError,'新计划保留提示');if(stage==='resource')assert.equal(retired.disposed,1);
        newConfirm.resolve(false);await current;assert.equal(owner.assets.disposed,1);t.a.attachment.dispose();
    }
});
test('真实AppRoot旧reconcile明确错误迟到不reset新计划，finally不清新owner或隐藏新Loading',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();const source=context(t.h),id='01'.repeat(16),late=deferred(),confirm=deferred(),feedback=[];
    s.travel.begin(source,id,1);s.travel.uncertain();t.a.travelSource=source;t.a.travelDestination=1;t.a.popup={confirm:()=>confirm.promise};t.a.showMapLoading=(...args)=>feedback.push(args);
    s.connection.request=async(code,value)=>{if(code===20007&&value.travelId===id)return late.promise;if(code===20007)return {travelId:value.travelId,fromSceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),targetSceneKind:2,position:{x:633,y:613},expiresAtUnixMs:String(Date.now()+60000)};const result={travelId:value.travelId,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),phase:3};s.connection.deliver(20010,result);return result;};
    const old=t.a.travelMap(),oldDone=assert.rejects(old,error=>error.staleMapAttachment===true);await flush();s.connection.deliver(20010,{travelId:id,fromSceneEpoch:e.sceneEpoch,sceneEpoch:e.sceneEpoch,targetMapId:Long.fromInt(2,true),phase:4});t.a.handleSessionUpdate();
    const current=t.a.travelMap();await flush();const owner=t.a.preparedTravel,newID=s.travel.id;assert(owner);t.a.showMapLoading(true,'新计划加载');const before=feedback.length;
    late.reject(new ProtocolResponseError(20008,1001));await oldDone;assert.equal(s.travel.id,newID);assert.equal(s.travel.phase,'confirming');assert.equal(t.a.preparedTravel,owner);assert.equal(owner.assets.disposed,0);assert.equal(feedback.length,before);
    confirm.resolve(false);await current;assert.equal(owner.assets.disposed,1);t.a.attachment.dispose();
});

test('真实AppRoot复用保存Role Message展示未知/nil/legacy/trusted，提交观察透传消息区而不改布局或地图',async()=>{
    const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1','2',2));t.baseline(e);s.preparedWorld=t.asset('2',true);await t.a.attachCurrentEntry();t.a.quickSkill=()=>null;t.a.skillSnapshot=()=>({});t.a.skillAssetsSnapshot=()=>({});t.a.combatStatus.refresh=()=>{};t.a.world.gameplay.map_catalog=[{id:'2',name:'比奇省'}];let presented;t.a.messages={refresh:value=>presented=value};
    t.realRefresh();assert.match(t.getNode('UIMgr/Panels/Role/Message').getComponent().string,/绑定信息同步中/);
    const observation={role:s.role.key,sceneEpoch:e.sceneEpoch,roleRevision:Long.fromInt(10,true),sourceSeq:Long.UZERO,binding:null,cause:1,safeZoneId:Long.UZERO,trusted:false};
    s.connection.deliver(20011,observation);t.realRefresh();assert.match(t.getNode('UIMgr/Panels/Role/Message').getComponent().string,/尚未绑定/);
    // 新连接读到历史坐标；历史字段不构成可信receipt。
    s.connection.status('新连接');s.connection.binding=Long.fromInt(2,true);s.connection.connected=true;const reentry=entry('1','2',2);s.connection.deliver(20002,reentry);t.baseline(reentry);await t.a.attachCurrentEntry();s.connection.deliver(6,reentry);t.a.combatStatus.refresh=()=>{};t.a.world.gameplay.map_catalog=[{id:'2',name:'比奇省'}];
    s.connection.deliver(20011,{...observation,sceneEpoch:reentry.sceneEpoch,binding:{mapId:Long.fromInt(2,true),center:{x:650,y:631}}});t.realRefresh();assert.match(t.getNode('UIMgr/Panels/Role/Message').getComponent().string,/历史记录待服务端确认/);
    s.connection.deliver(40003,{revision:Long.fromInt(11,true),items:[],equipped:[]});s.connection.deliver(20011,{...observation,sceneEpoch:reentry.sceneEpoch,roleRevision:Long.fromInt(11,true),sourceSeq:Long.fromInt(9,true),binding:{mapId:Long.fromInt(2,true),center:{x:650,y:631}},cause:2,safeZoneId:Long.fromInt(106,true),trusted:true});t.realRefresh();
    const text=t.getNode('UIMgr/Panels/Role/Message').getComponent().string;assert.match(text,/绑定：比奇省 \(650,631\)/);assert(!text.includes('历史记录'));assert.equal(presented.bindingState,s.bindingState);assert.equal(presented.bindingNotice,s.bindingNotice);assert.equal(t.a.world.map.width,700);t.a.attachment.dispose();
});


test('真实AppRoot退出只接受完整取消观察；等待期间新Entry使旧完成不能关闭新附着',async()=>{
 for(const replacement of [false,true]){
  const t=appHarness(),s=t.h.session,e=ready(t.h,entry('1'));t.baseline(e);s.preparedWorld=t.asset('1');await t.a.attachCurrentEntry();let closed=0,changed=0;s.connection.close=()=>{closed++;};s.scenes={switchTo:async()=>{changed++;}};const pending=deferred();s.connection.requestNavigationCancel=(scene,baseline,input)=>{assert.equal(scene.toString(),'1');assert.equal(baseline.toString(),'1');assert(input>0);return pending.promise;};
  const task=t.a.leave(false),check=replacement?assert.rejects(task,/退出期间/):task;await flush();assert.equal(closed,0);assert.equal(changed,0);
  if(replacement){s.connection.deliver(20002,entry('2'));t.baseline(s.entry);}
  pending.resolve({sceneEpoch:e.sceneEpoch,baselineId:e.baselineId,stateSequence:Long.fromInt(2,true),chunkCount:2,chunkIndex:1,assembledNavigationCancel:true,navigationState:{status:4,revision:Long.UONE},entities:[]});await check;assert.equal(closed,replacement?0:1);assert.equal(changed,replacement?0:1);t.a.attachment.dispose();
 }
});

test('离线托管只接受服务端已确认的普通地图、存活角色、开启自动战斗和可用余额', async () => {
    const cases = [
        { host: null, error: /尚未同步/ },
        { host: { autoBattleEnabled: true, remainHostingSec: 0 }, error: /领取免费托管时长/ },
        { host: { autoBattleEnabled: false, remainHostingSec: 60 }, error: /开启自动战斗/ },
        { dead: true, error: /角色存活/ },
        { town: true, error: /普通挂机地图/ },
    ];
    for (const fixture of cases) {
        const t = appHarness(), s = t.h.session;
        const e = ready(t.h, fixture.town ? entry('1', '2', 2) : entry('1'));
        t.baseline(e);
        s.preparedWorld = t.asset(e.roomId.toString(), !!fixture.town);
        await t.a.attachCurrentEntry();
        if (fixture.dead) s.baseline.entities[0].hp = Long.ZERO;
        s.hosting = Object.hasOwn(fixture, 'host') ? fixture.host : { autoBattleEnabled: true, remainHostingSec: 60 };
        t.a.auto = true;
        s.connection.close = () => assert.fail('托管不可用时应保留连接');
        s.connection.requestNavigationCancel = () => assert.fail('托管不可用时应保留导航');
        await assert.rejects(t.a.leave(true), fixture.error);
        assert.equal(s.ready, true);
        assert.equal(t.a.canSendInput(), true);
        t.a.attachment.dispose();
    }
});

test('离线托管在导航取消回执后复核，死亡不退出，仍可托管时才返回登录', async () => {
    for (const remaining of [true, false]) {
        const t = appHarness(), s = t.h.session, e = ready(t.h, entry('1'));
        t.baseline(e);
        s.preparedWorld = t.asset('1');
        await t.a.attachCurrentEntry();
        s.connection.token = 'test-token';
        s.account = { id: 'account', name: 'account_name' };
        s.hosting = { autoBattleEnabled: true, remainHostingSec: 60 };
        const pending = deferred(), transitions = [];
        s.connection.requestNavigationCancel = () => pending.promise;
        s.connection.close = () => transitions.push('关闭连接');
        s.scenes = { switchTo: async scene => transitions.push(scene) };
        const result = t.a.leave(true);
        const checked = remaining ? result : assert.rejects(result, /角色存活/);
        assert.deepEqual(transitions, []);
        pending.resolve({ ...s.baseline, stateSequence: Long.fromInt(2, true), assembledNavigationCancel: true,
            navigationState: { status: 4, revision: Long.UONE },
            entities: [{ ...s.baseline.entities[0], hp: remaining ? Long.UONE : Long.ZERO }] });
        await checked;
        assert.deepEqual(transitions, remaining ? ['关闭连接', 'Login'] : []);
        assert.equal(s.ready, !remaining);
        assert.equal(s.connection.token, remaining ? '' : 'test-token');
        t.a.attachment.dispose();
    }
});

test('掉线返回只退出本地，保留原未决账本，并按登录凭证选择登录或选角', async () => {
    for (const token of ['', 'test-token']) {
        const t = appHarness(), s = t.h.session;
        const ledger = { get() { return { id: 40026 }; } };
        s.connection.token = token;
        s.pending = ledger;
        const transitions = [];
        s.connection.requestNavigationCancel = () => assert.fail('掉线返回不能发送原连接上的请求');
        s.connection.close = () => transitions.push('关闭连接');
        s.scenes = { switchTo: async scene => transitions.push(scene) };
        await t.a.leave(false, true);
        assert.deepEqual(transitions, ['关闭连接', token ? 'RoleSelect' : 'Login']);
        assert.equal(s.pending, ledger);
        t.a.attachment.dispose();
    }
});
