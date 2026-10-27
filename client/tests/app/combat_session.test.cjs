const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {createRequire}=require('node:module');
const ts=require('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/Resources/app.asar.unpacked/node_modules/typescript');
const file=path.resolve(__dirname,'../../assets/scripts/app/GameSession.ts');
const Long=createRequire(file)('long');
class Connection{subscribe(listener){this.listener=listener;}onStatus(listener){this.status=listener;}deliver(id,value){this.listener({id,value,error:0});}}
const compiled=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,esModuleInterop:true}});
const moduleLike={exports:{}};
new Function('require','module','exports',compiled.outputText)(id=>id==='long'?Long:id==='./GameConnection'?{GameConnection:Connection}
    :id==='./BaselineAssembler'?{BaselineAssembler:class{reset(){}accept(value){return value;}}}:id==='./GameErrors'?{protocolErrorMessage:()=>''}:{},moduleLike,moduleLike.exports);
const {session}=moduleLike.exports;
test('瞬时战斗流仅在当前场景Ready转发，初始及重连基线不补播历史动作特效',()=>{
    const seen={casts:0,attacks:0,impacts:0};session.casts.add(()=>seen.casts++);session.attacks.add(()=>seen.attacks++);session.impacts.add(()=>seen.impacts++);
    const entry=sceneEpoch=>({sceneEpoch,baselineId:Long.UONE,nextEconomicCommandSeq:Long.UONE});
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
