const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {createRequire}=require('node:module');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
const file=path.resolve(__dirname,'../../assets/scripts/app/GameSession.ts');
const Long=createRequire(file)('long');
// 这些历史用例每次Enter都表示重新建立观察连接；真实同连接换图另有专项。
class Connection{constructor(){this.generation=0;this.binding=Long.UONE;}subscribe(listener){this.listener=listener;}onStatus(listener){this.status=message=>{this.generation++;listener(message);};}deliver(id,value){if(id===20002){this.generation++;this.binding=this.binding.add(1);value={roomId:Long.UONE,sceneKind:1,resourceVersion:Long.UONE,...value};}this.listener({id,value,error:0,binding:this.binding});}}
const compiled=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,esModuleInterop:true}});
const moduleLike={exports:{}};
new Function('require','module','exports',compiled.outputText)(id=>id==='long'?Long:id==='./GameConnection'?{GameConnection:Connection}
    :id==='./BaselineAssembler'?{BaselineAssembler:class{reset(){}accept(value){return value;}}}:id==='./GameErrors'?{protocolErrorMessage:()=>''}
    :id==='./WarehouseModel'?require('../../../.work/ui-tests/app/WarehouseModel.js'):id==='./SkillModel'?require('../../../.work/ui-tests/app/SkillModel.js'):id==='./TeleportModel'?require('../../../.work/ui-tests/app/TeleportModel.js'):id==='./RoleBindingModel'?require('../../../.work/ui-tests/app/RoleBindingModel.js'):id==='./PetBornModel'?require('../../../.work/ui-tests/app/PetBornModel.js'):id==='./AutoRecycleModel'?require('../../../.work/ui-tests/app/AutoRecycleModel.js'):id==='./LevelGrowthModel'?require('../../../.work/ui-tests/app/LevelGrowthModel.js'):id==='./MapTravelModel'?require('../../../.work/ui-tests/app/MapTravelModel.js'):{},moduleLike,moduleLike.exports);
const {session}=moduleLike.exports;
session.role={job:0,key:{originServerId:Long.UONE,roleId:new Uint8Array(16).fill(1)}};
test('入场必须显式提供正玩家句柄，缺失或越界不能沿kind或ID1恢复Ready',()=>{
    const entry={sceneEpoch:Long.UONE,baselineId:Long.UONE,nextEconomicCommandSeq:Long.UONE};
    for(const playerHandle of [undefined,0,-1,4294967296]){session.connection.deliver(20002,{...entry,playerHandle});session.connection.deliver(6,entry);assert.equal(session.entry,null);assert.equal(session.ready,false);assert.match(session.lastError,/玩家身份/);}
    session.connection.deliver(20002,{...entry,playerHandle:7});session.connection.deliver(6,entry);assert.equal(session.entry.playerHandle,7);assert.equal(session.ready,true);
});
test('显式位移只在当前Ready连接转发，初始/重连基线不补播旧路径',()=>{
    let count=0;const listener=()=>count++;session.displacements.add(listener);
    const entry={sceneEpoch:Long.UONE,baselineId:Long.UONE,nextEconomicCommandSeq:Long.UONE,playerHandle:1};
    session.connection.deliver(20002,entry);session.connection.deliver(30013,{sceneEpoch:Long.UONE});assert.equal(count,0);
    session.connection.deliver(5,{stateSequence:Long.UONE,entities:[{entityHandle:1,movementRevision:Long.fromInt(9),movementKind:2}]});assert.equal(count,0);
    session.connection.deliver(6,entry);session.connection.deliver(30013,{sceneEpoch:Long.fromInt(2)});assert.equal(count,0);
    session.connection.deliver(30013,{sceneEpoch:Long.UONE});assert.equal(count,1);
    session.connection.status('测试断线');session.connection.deliver(30013,{sceneEpoch:Long.UONE});assert.equal(count,1);
    session.connection.deliver(20002,entry);session.connection.deliver(5,{stateSequence:Long.UONE,entities:[{entityHandle:1,movementRevision:Long.fromInt(9),movementKind:2}]});assert.equal(count,1);
    session.displacements.delete(listener);
});
test('瞬时战斗流仅在当前场景Ready转发，初始及重连基线不补播历史动作特效',()=>{
    const seen={casts:0,attacks:0,impacts:0};session.casts.add(()=>seen.casts++);session.attacks.add(()=>seen.attacks++);session.impacts.add(()=>seen.impacts++);
    const entry=sceneEpoch=>({sceneEpoch,baselineId:Long.UONE,nextEconomicCommandSeq:Long.UONE,playerHandle:1});
    session.connection.deliver(20002,entry(Long.UONE));
    session.connection.deliver(5,{stateSequence:Long.UONE,entities:[{castSequence:99,attackSequence:99}]});
    for(const id of [30006,30007,30008])session.connection.deliver(id,{sceneEpoch:Long.UONE});assert.deepEqual(seen,{casts:0,attacks:0,impacts:0});
    session.connection.deliver(6,entry(Long.UONE));for(const id of [30006,30007,30008])session.connection.deliver(id,{sceneEpoch:Long.UONE});
    assert.deepEqual(seen,{casts:1,attacks:1,impacts:1});
    session.connection.deliver(5,{stateSequence:Long.fromInt(2,true),entities:[{castSequence:100,attackSequence:100}]});
    for(const id of [30006,30007,30008])session.connection.deliver(id,{sceneEpoch:Long.fromInt(2,true)});assert.deepEqual(seen,{casts:1,attacks:1,impacts:1});
    session.connection.status('断线');for(const id of [30006,30007,30008])session.connection.deliver(id,{sceneEpoch:Long.UONE});assert.deepEqual(seen,{casts:1,attacks:1,impacts:1});
    session.connection.deliver(20002,entry(Long.fromInt(2,true)));session.connection.deliver(6,entry(Long.fromInt(2,true)));
    for(const id of [30006,30007,30008])session.connection.deliver(id,{sceneEpoch:Long.UONE});assert.deepEqual(seen,{casts:1,attacks:1,impacts:1});
});
test('模式基线可在Ready前恢复，推送按原场景与修订隔离，重入清缓存且不触发旧刀光',()=>{
    const epoch=Long.fromString('9007199254740993',true),entry={sceneEpoch:epoch,baselineId:Long.UONE,nextEconomicCommandSeq:Long.UONE,playerHandle:1};
    session.connection.deliver(20002,entry);
    const mode={sceneEpoch:epoch,revision:Long.fromString('9007199254740993',true),playerLife:Long.UZERO,serverTimeUnixMs:Long.fromString('1900000000000',true),states:[{skillId:26,enabled:false,ready:true,remainingMs:5000,cooldownRemainingMs:5000}]};
    session.connection.deliver(5,{stateSequence:Long.UONE,entities:[],skillModeState:mode});assert.equal(session.ready,false);assert.equal(session.skillModes.states[0].ready,true);
    session.connection.deliver(30010,{...mode,sceneEpoch:Long.UONE,revision:mode.revision.add(10),states:[]});assert.equal(session.skillModes,mode);
    session.connection.deliver(30010,{...mode,states:[]});assert.equal(session.skillModes,mode);
    const newer={...mode,revision:mode.revision.add(1),states:[]};session.connection.deliver(30010,newer);assert.equal(session.skillModes,newer);
    session.connection.deliver(20002,entry);assert.equal(session.skillModes,null);
});
test('CastStep只转发当前Ready连接的真实未来步，重入基线不补播旧起手和已处理格',()=>{
    let received=0;const listener=()=>received++;session.castSteps.add(listener);
    const entry={sceneEpoch:Long.fromInt(3,true),baselineId:Long.UONE,nextEconomicCommandSeq:Long.UONE,playerHandle:1};
    session.connection.deliver(20002,entry);session.connection.deliver(30012,{sceneEpoch:entry.sceneEpoch,stepIndex:1});assert.equal(received,0);
    session.connection.deliver(5,{stateSequence:Long.UONE,entities:[{castSequence:99,castSkillId:9}]});assert.equal(received,0);
    session.connection.deliver(6,entry);session.connection.deliver(30012,{sceneEpoch:Long.UONE,stepIndex:2});assert.equal(received,0);
    session.connection.deliver(30012,{sceneEpoch:entry.sceneEpoch,stepIndex:3});assert.equal(received,1);
    session.connection.status('断线');session.connection.deliver(30012,{sceneEpoch:entry.sceneEpoch,stepIndex:4});assert.equal(received,1);session.castSteps.delete(listener);
});
test('同连接复活用新life与更高修订恢复模式操作，迟到旧life状态和旧基线不能覆盖',()=>{
    const models=require('../../../.work/ui-tests/app/SkillModel.js');
    const decorator=(...args)=>args.length>1?undefined:(...params)=>params.length===1?params[0]:undefined;
    const appFile=path.resolve(__dirname,'../../assets/scripts/app/AppRoot.ts');
    const appCode=ts.transpileModule(fs.readFileSync(appFile,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,experimentalDecorators:true,esModuleInterop:true}});
    const appModule={exports:{}};
    new Function('require','module','exports',appCode.outputText)(id=>id==='cc'?{_decorator:new Proxy({},{get:()=>decorator}),Component:class{}}
        :id==='long'?Long:id==='./GameSession'?{session,roleIdentity:()=> '复活回归角色'}:id==='./SkillModel'?models:{},appModule,appModule.exports);
    // 使用真实入口的快照与施放逻辑；不创建场景节点或真实连接。
    const app=Object.create(appModule.exports.AppRoot.prototype),sent=[],connection=session.connection;
    app.inputSequence=0;connection.send=(id,value)=>sent.push({id,value});
    const definition={id:12,name:'刺杀剑术',job_id:0,book_item_id:3,learn_level:1,initial_level:0,max_level:3,kind:'thrusting',active:true,
        activation_kind:'toggle',target_kind:'attack_mode',range:2,base_cost:0,level_cost:0};
    app.world={gameplay:{skills:[definition]}};
    const epoch=Long.fromString('9007199254740993',true),revision=Long.fromString('9007199254740995',true);
    const entry={sceneEpoch:epoch,resourceVersion:Long.fromInt(6,true),baselineId:Long.UONE,nextEconomicCommandSeq:Long.UONE,playerHandle:1};
    session.connection.deliver(20002,entry);session.role={job:0};
    session.inventory={level:40,items:[],learnedSkills:[{skillId:12,level:0}]};
    const oldMode={sceneEpoch:epoch,revision,playerLife:Long.UZERO,serverTimeUnixMs:Long.fromString('1900000000000',true),
        states:[{skillId:12,enabled:true,ready:false,remainingMs:0,cooldownRemainingMs:0}]};
    const player=(life,hp)=>({entityHandle:1,entityKind:1,life,hp:Long.fromInt(hp),mp:Long.fromInt(10),position:{x:2,y:2},direction:4});
    session.connection.deliver(5,{stateSequence:Long.UONE,entities:[player(Long.UZERO,0)],skillModeState:oldMode});
    session.connection.deliver(6,entry);
    const revivedMode={...oldMode,revision:revision.add(1),playerLife:Long.UONE};
    session.connection.deliver(5,{stateSequence:Long.fromInt(2,true),entities:[player(Long.UONE,100)],skillModeState:revivedMode});
    assert.equal(session.connection,connection);assert.equal(session.ready,true);
    assert.equal(app.skillSnapshot().modes,revivedMode);
    assert.equal(models.skillModeStatus(app.skillSnapshot(),definition),'已开启');
    const row=models.skillRows(app.skillSnapshot())[0];app.castSkill(row,false);
    assert.equal(sent.length,1);assert.equal(sent[0].id,30009);assert.equal(sent[0].value.enabled,false);
    assert.equal(sent[0].value.sceneEpoch.toString(),epoch.toString());
    // 旧出生代次的已发送状态修订不可能高于复活递增后的版本。
    session.connection.deliver(30010,oldMode);
    session.connection.deliver(5,{stateSequence:Long.UONE,entities:[player(Long.UZERO,0)],skillModeState:oldMode});
    assert.equal(app.skillSnapshot().modes,revivedMode);assert.equal(session.baseline.entities[0].life.toString(),'1');
    assert.equal(models.skillModeStatus(app.skillSnapshot(),definition),'已开启');
    delete connection.send;
});
