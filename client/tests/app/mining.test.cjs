// 使用真实静态编解码和矿区控制器核对场景、纯度、持久及触摸代表的请求。
const {test}=require('node:test');
const assert=require('node:assert/strict');
const {Long,load,harness,entry,ready,context}=require('./helpers/session_harness.cjs');
const model=require('../../../.work/ui-tests/app/MiningModel.js');
const travel=require('../../../.work/ui-tests/app/MapTravelModel.js');
const pb=require('../../assets/scripts/generated/protobuf/client_pb.js');
const {GameMining}=load('client/assets/scripts/app/GameMining.ts', id=>id==='./MiningModel'?model:
    id==='./GameErrors'?{protocolErrorMessage:()=> '原锄头不可用'}:{});
const conn=load('client/assets/scripts/app/GameConnection.ts',id=>id==='long'?Long:id==='../generated/protobuf/client_pb.js'?pb:{});
const skills=require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const {inventoryItemActionView}=load('client/assets/scripts/app/InventoryItemActions.ts',id=>id==='./MiningModel'?model:
    id==='./SkillAssetsModel'?skills:{});
const {selectWorldMap,miningFloorStyleMap}=load('client/assets/scripts/app/WorldAssets.ts',()=>({}));
const {townDirectoryRegions}=load('client/assets/scripts/app/TownMapModel.ts',id=>id==='./MapTravelModel'?travel:{});
const u=value=>Long.fromString(String(value),true);
const scope={key:'原矿图附着',binding:'9007199254740997',sceneEpoch:u('9007199254740993'),mapId:u(202601),inventoryRevision:u(7)};
const state=(sequence,fields={})=>({sceneEpoch:scope.sceneEpoch,mapId:scope.mapId,stateSequence:u(sequence),
    canStart:true,active:false,errorCode:0,attempts:Long.UZERO,successes:Long.UZERO,...fields});
const mineItem={itemId:910004,instanceId:new Uint8Array(16).fill(21),quantity:Long.ONE,version:u(2),
    bound:true,orePurity:63,sourceKind:'mining'};
const tool={itemId:910011,instanceId:new Uint8Array(16).fill(22),quantity:Long.ONE,version:u(3),
    bound:false,durabilityU:u(9997),maxDurabilityU:u(10000),originalDurabilityU:u(10000)};

test('矿区协议保持5个64位身份/计数和锄头原字节，状态只接收当前附着',()=>{
    const intent=conn.decodePacket(conn.encodePacket(20028,8,u(scope.binding),
        {sceneEpoch:scope.sceneEpoch,action:1,toolInstanceId:tool.instanceId}));
    assert.equal(intent.value.sceneEpoch.toString(),scope.sceneEpoch.toString());
    assert.deepEqual(new Uint8Array(intent.value.toolInstanceId),tool.instanceId);
    const packet=conn.decodePacket(conn.encodePacket(20029,0,u(scope.binding),state('9007199254740995',
        {active:true,attempts:u('9007199254741000'),successes:u(1),tool,lastOre:mineItem})));
    assert(model.isMiningState(packet.value,scope));
    assert.equal(packet.value.stateSequence.toString(),'9007199254740995');
    assert.equal(packet.value.attempts.toString(),'9007199254741000');
    assert.equal(packet.value.lastOre.orePurity,63);
    assert.equal(model.miningDurability(packet.value.tool),'锄头持久 9.997 / 10.000');
    const current=new model.MiningModel();
    current.attach(scope);
    assert(current.observe(scope,packet.value));
    assert.equal(current.observe(scope,state(1)),false);
    assert.equal(current.observe(scope,state('9007199254740996',{lastOre:{...mineItem,orePurity:101}})),false);
    const next={...scope,key:'新矿图附着',mapId:u(202602)};
    current.attach(next);
    assert.equal(current.observe(scope,packet.value),false);
    assert.equal(current.state,null);
    assert(current.observe(next,{...state(1),mapId:next.mapId}));
});

test('矿区先查询再开始与停止；状态晚到不重放，工具行发送原16字节',async()=>{
    let current=null, receive, renders=0;
    const requests=[], waiting=[];
    const game=new GameMining({subscribe(listener){receive=listener;return ()=>{receive=null;};},
        request(id,value){requests.push({id,value});return new Promise((resolve,reject)=>waiting.push({resolve,reject}));}},
    ()=>current,()=>{renders++;},id=>id===mineItem.itemId?'黑铁矿':id===tool.itemId?'一层锄头':undefined);
    game.refresh();
    assert.equal(requests.length,0,'无矿区的正式包不探测协议');
    current=scope; game.refresh(); game.refresh();
    assert.deepEqual(requests[0],{id:20028,value:{sceneEpoch:scope.sceneEpoch,action:0}});
    waiting.shift().resolve(state(1,{tool})); await Promise.resolve();
    assert.equal(game.actionText,'开始挖矿');
    const start=game.useTool(tool.instanceId);
    assert.deepEqual(requests[1].value.toolInstanceId,tool.instanceId);
    assert.equal(requests[1].value.action,1);
    waiting.shift().resolve(state(2,{active:true,tool})); await start;
    assert.equal(game.actionText,'停止挖矿');
    assert.equal(game.toolBlock,'请先停止当前挖矿');
    receive({id:20029,binding:u(scope.binding),error:0,value:state(3,{active:true,attempts:u(1),successes:u(1),tool,lastOre:mineItem})});
    assert.match(game.objective,/黑铁矿 纯度 63/);
    assert(game.details().includes('绑定矿石'));
    const stop=game.toggle();
    assert.equal(requests[2].value.action,2);
    waiting.shift().resolve(state(4,{attempts:u(1),successes:u(1),tool,lastOre:mineItem}));
    await stop;
    assert.equal(game.active,false);
    receive({id:20029,binding:u(1),error:0,value:state(25,{active:true})});
    assert.equal(game.active,false);
    const oldStart=game.toggle();
    current={...scope,key:'重连原矿图',binding:'2'}; game.refresh();
    const old=waiting.shift(), recent=waiting.shift();
    recent.resolve(state(1,{tool})); old.resolve(state(26,{active:true}));
    await oldStart;
    assert.equal(game.active,false);
    assert(renders>0);
    game.dispose(); assert.equal(receive,null);
});

test('矿石回收只确认原附着的明确开关，到账显示跟随原矿绑态和 Long',async()=>{
    const copper={...mineItem,itemId:910001};
    const ids=[copper.itemId], saved=[];
    const receipt=Long.fromString('9007199254740997');
    let current=scope, receive, confirm, writable=true;
    const game=new GameMining({subscribe(listener){receive=listener;return ()=>{};}},()=>current,()=>{},
        id=>id===copper.itemId?'铜矿':undefined,{canSave:()=>writable,confirmEnable:names=>{
            assert.deepEqual(Array.from(names),['铜矿']);
            return new Promise(resolve=>{confirm=resolve;});
        },save:async enabled=>{saved.push(enabled);}});
    const deliver=(sequence,fields={})=>receive({id:20029,binding:u(current.binding),error:0,value:state(sequence,fields)});
    deliver(1);
    assert.equal(game.recycleDetails(),null);
    deliver(2,{recycleItemIds:ids,autoRecycleEnabled:false});
    const read=game.recycleDetails();
    assert(read().lines.includes('矿石自动回收 已关闭'));
    assert.equal(read().actions[0].label,'开启回收');
    const canceled=read().actions[0].run(); confirm(false); await canceled;
    assert.deepEqual(saved,[]);
    const enabled=read().actions[0].run(); confirm(true); await enabled;
    assert.deepEqual(saved,[true]);
    assert.equal(read().actions[0].label,'开启回收','服务端观察前不乐观改设定');
    deliver(3,{recycleItemIds:ids,autoRecycleEnabled:true,lastOre:copper,lastRecycleBoundGold:receipt});
    assert.match(game.objective,/已回收/);
    assert(read().lines.includes(`本次回收 ${receipt}绑定金币`));
    writable=false;
    assert.equal(read().actions[0].enabled,false);
    writable=true;
    await read().actions[0].run();
    assert.deepEqual(saved,[true,false]);
    deliver(4,{recycleItemIds:ids,autoRecycleEnabled:false,lastOre:copper,lastRecycleBoundGold:receipt});
    const stale=read().actions[0].run();
    current={...scope,key:'新附着',binding:'3'};
    deliver(1,{recycleItemIds:ids,autoRecycleEnabled:false});
    confirm(true); await stale;
    assert.equal(read(),null);
    assert.deepEqual(saved,[true,false]);
    assert.equal(game.recycleDetails()().actions[0].label,'开启回收');
    assert(!model.isMiningState(state(1,{recycleItemIds:ids,autoRecycleEnabled:true,lastOre:copper,lastRecycleGold:receipt}),scope));
    assert(!model.isMiningState(state(1,{recycleItemIds:[...ids,...ids],autoRecycleEnabled:true}),scope));
});

test('只在公开候选显示两层矿区并锁等级，复用原尺寸地表与原目的3',()=>{
    const regular={id:'1',name:'普通挂机',resource_key:'development/maps/personal-map',scene_kind:1,width:13,height:13};
    const cave={...regular,id:'7',name:'原洞穴'};
    const base={town_travel:{destination_map_ids:[]},map_catalog:[regular,cave],map_regions:[{id:'normal',name:'原区域',map_ids:['1','7']}]};
    assert.equal(townDirectoryRegions(base,'1',1).flatMap(row=>row.maps).some(row=>row.destination===3),false);
    const mining={...regular,id:'202601',name:'比奇矿区一层',scene_kind:7,minimum_level:20,floor_style_map_id:'7'};
    const candidate={...base,map_catalog:[...base.map_catalog,mining],map_regions:[...base.map_regions,{id:'mining',name:'比奇矿区',map_ids:['202601']}]};
    assert.equal(townDirectoryRegions(candidate,'1',19)[1].maps[0].levelLocked,true);
    const map=townDirectoryRegions(candidate,'1',20)[1].maps[0];
    assert.equal(map.levelLocked,false); assert.equal(map.destination,3); assert.equal(map.minimumLevel,20);
    assert.equal(miningFloorStyleMap(candidate,selectWorldMap(candidate,map.mapId)).id,'7');
    assert.throws(()=>miningFloorStyleMap(candidate,{...mining,width:17}),/地表样式/);
    const t=new travel.MapTravelModel(), source={connection:1,binding:'1',role:'甲',scene:'2',baseline:'3',life:'1',rule:'30'};
    const id='01'.repeat(16), plan={travelId:id,fromSceneEpoch:'2',targetMapId:mining.id,targetSceneKind:7,position:{x:1,y:2},expiresAtUnixMs:'8000'};
    assert.throws(()=>t.begin(source,id,3)); t.begin(source,id,3,map.mapId);
    assert.throws(()=>t.acceptPlan({...plan,targetSceneKind:1},source,1));
    assert.throws(()=>t.acceptPlan({...plan,targetMapId:'202602'},source,1));
    t.acceptPlan(plan,source,1);
    assert.equal(t.plan.targetMapId,mining.id);
});

test('背包详情使用当前实例矿石纯度和工具状态',()=>{
    const ore=inventoryItemActionView({item:mineItem,definition:{kind:'mining_ore',name:'黑铁矿'},ready:true},{});
    assert(ore.lines.includes('纯度 63')); assert(ore.lines.includes('绑定物品')); assert(ore.lines.includes('来源 挖矿所得'));
    const choose=()=>Promise.resolve();
    const pick=inventoryItemActionView({item:tool,definition:{kind:'mining_tool',name:'锄头'},ready:true,miningReason:'请先停止当前挖矿'},{mine:choose});
    assert(pick.lines.includes('锄头持久 9.997 / 10.000'));
    assert.equal(pick.actions[0].enabled,false);
    assert.equal(pick.actions[0].run,choose);
});

test('真实Session接受匹配的矿区旅行和矿区健康重入，拒绝同连接无旅行的新矿图区',()=>{
    const h=harness(),s=h.session, id='03'.repeat(16), target=u(202601);
    const before=ready(h,entry('18','1',1)), src=context(h), next=entry('19','202601',7);
    s.connection.deliver(20002,next);
    assert.equal(s.entry,before);
    assert.match(s.lastError,/矿区入场不匹配/);
    s.ready=true;
    s.travel.begin(src,id,3,target);
    s.travel.acceptPlan({travelId:id,fromSceneEpoch:src.scene,targetMapId:target,targetSceneKind:7,position:{x:2,y:3},expiresAtUnixMs:'8000'},src,1000);
    s.travel.prepared(src,2000); s.travel.commit(src,3000);
    s.connection.deliver(20010,{travelId:id,fromSceneEpoch:before.sceneEpoch,sceneEpoch:next.sceneEpoch,phase:2,targetMapId:target});
    s.connection.deliver(20002,next);
    assert.equal(s.entry,next); assert.equal(s.entryTravelId,id);
    assert(s.travel.acknowledge(h.currentEntryIdentity()));
    s.connection.deliver(6,next);
    assert.equal(s.ready,true);
    s.connection.generation++; s.connection.binding=u(2);
    s.connection.deliver(20002,next); s.connection.deliver(6,next);
    assert.equal(s.entryBinding,'2'); assert.equal(s.entryTravelId,''); assert.equal(s.ready,true);
});

test('矿区显示时长不足一轮，普通买卖仍显示原钱包余额不足',()=>{
    const ids=load('client/assets/scripts/generated/protocol_ids.ts',()=>({}));
    const {protocolErrorMessage}=load('client/assets/scripts/app/GameErrors.ts',id=>id==='../generated/protocol_ids'?ids:{});
    assert.equal(protocolErrorMessage(ids.MsgC2SMiningIntent,ids.CodeVendorFunds),'托管时长不足一轮，已停止挖矿');
    assert.equal(protocolErrorMessage(ids.MsgS2CMiningState,ids.CodeVendorFunds),'托管时长不足一轮，已停止挖矿');
    assert.equal(protocolErrorMessage(ids.MsgC2SVendorCommit,ids.CodeVendorFunds),'当前钱包余额不足');
});
