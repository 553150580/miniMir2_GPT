const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const {createRequire}=require('node:module');
const ts=require('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/Resources/app.asar.unpacked/node_modules/typescript');
const source=path.resolve(__dirname,'../../assets/scripts/app');
const loadedModules=new Map();
function loadSource(file){
    if(loadedModules.has(file))return loadedModules.get(file).exports;
    const nativeRequire=createRequire(file);
    const requireSource=specifier=>{
        if(specifier.startsWith('./')||specifier.startsWith('../')){
            const resolved=path.resolve(path.dirname(file),specifier);
            const candidates=path.extname(resolved)==='.ts'?[resolved]
                :path.extname(resolved)===''?[resolved+'.ts',path.join(resolved,'index.ts')]:[];
            const dependency=candidates.find(candidate=>fs.existsSync(candidate));
            if(dependency)return loadSource(dependency);
        }
        // 生成的 protobuf.js 和 long 等依赖沿用 Node 原生解析。
        return nativeRequire(specifier);
    };
    const result=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,esModuleInterop:true}});
    const module={exports:{}};loadedModules.set(file,module);
    try{new Function('require','module','exports',result.outputText)(requireSource,module,module.exports);}
    catch(error){loadedModules.delete(file);throw error;}
    return module.exports;
}
function load(name){return loadSource(path.join(source,name+'.ts'));}
const {encodePacket,decodePacket,GameConnection}=load('GameConnection');
const {viewportGeometry}=load('ViewportGeometry');
const {BaselineAssembler}=load('BaselineAssembler');
const Long=createRequire(path.join(source,'GameConnection.ts'))('long');

test('药品使用帧保留原实例、最大64位场景代次与经济操作身份',()=>{
    const instance=new Uint8Array(16).fill(5),operationId=new Uint8Array(16).fill(9);
    const encoded=encodePacket(40009,19,Long.UONE,{itemInstanceId:instance,itemId:35,sceneEpoch:Long.MAX_UNSIGNED_VALUE,
        operation:{opId:operationId,commandSeq:Long.fromString('9007199254740993',true),expectedRevision:Long.UONE,expectedRuleVersion:Long.UONE}});
    const decoded=decodePacket(encoded);
    assert.equal(decoded.id,40009);assert.equal(decoded.value.itemId,35);
    assert.deepEqual(decoded.value.itemInstanceId,instance);
    assert.equal(decoded.value.sceneEpoch.toString(),'18446744073709551615');
    assert.deepEqual(decoded.value.operation.opId,operationId);
    assert.equal(decoded.value.operation.commandSeq.toString(),'9007199254740993');
});

test('应用帧与snake_case协议往返：大ID、64位水位和bytes保持精确',()=>{
    const binding=Long.fromString('18446744073709551615',true);
    const bytes=encodePacket(20002,4294967295,binding,{sceneEpoch:Long.fromString('9007199254740993',true),baselineId:Long.UONE,nextEconomicCommandSeq:Long.fromString('9007199254740995',true),position:{x:-7,y:8}});
    const result=decodePacket(bytes);
    assert.equal(result.seq,4294967295);assert(result.binding.equals(binding));
    assert.equal(result.value.sceneEpoch.toString(),'9007199254740993');assert.equal(result.value.nextEconomicCommandSeq.toString(),'9007199254740995');
    assert.deepEqual(result.value.position,{x:-7,y:8});
    const id=new Uint8Array(16).fill(245);
    assert.deepEqual(decodePacket(encodePacket(40002,1,Long.UONE,{itemInstanceIds:[id],operation:{opId:id,commandSeq:Long.UONE,expectedRevision:Long.UONE,expectedRuleVersion:Long.UONE}})).value.itemInstanceIds[0],id);
});
test('MP、默认0和嵌套装备使用同一PB数据定义',()=>{
    const state=decodePacket(encodePacket(5,1,Long.UONE,{sceneEpoch:Long.UONE,baselineId:Long.UONE,entities:[{entityHandle:1,entityKind:1,position:{x:0,y:0},mp:Long.fromInt(12),mpMax:Long.fromInt(14),moveIntervalMs:600,attackSequence:4294967295}]})).value;
    assert.equal(state.entities[0].mp.toString(),'12');assert.equal(state.entities[0].direction,0);assert.equal(state.entities[0].position.x,0);
    assert.equal(state.entities[0].moveIntervalMs,600);assert.equal(state.entities[0].attackSequence,4294967295);
    const inventory=decodePacket(encodePacket(40003,2,Long.UONE,{equipped:[{slot:0,item:{itemId:218,quantity:Long.ONE,bound:true}}],level:1,experience:Long.ZERO})).value;
    assert.equal(inventory.equipped[0].item.itemId,218);assert.equal(inventory.experience.toString(),'0');
});
test('截断、超长、未知版本和变长整数溢出被拒绝',()=>{
    for(const data of [new Uint8Array(5),new Uint8Array([2,0,0,2,1,0]),new Uint8Array([1,2,0,2,1,0]),new Uint8Array([1,0,0,2,255,255,255,255,16,0]),new Uint8Array(65537)])assert.throws(()=>decodePacket(data));
});
test('同宽更高只增加逻辑视口高度，同高更宽只增加宽度',()=>{
    const base=viewportGeometry(720,1280),tall=viewportGeometry(720,1600),wide=viewportGeometry(960,1280);
    assert.deepEqual(base,{width:720,height:1280,scale:1});
    assert.equal(tall.scale,base.scale);assert.equal(tall.width,base.width);assert.equal(tall.height-base.height,320);
    assert.equal(wide.scale,base.scale);assert.equal(wide.height,base.height);assert.equal(wide.width-base.width,240);
    const small=viewportGeometry(360,800);assert.equal(small.width,720);assert.equal(small.height,1600);assert.equal(small.scale,.5);
    for(const [w,h]of[[0,1],[1,-1],[Infinity,2],[NaN,2]])assert.throws(()=>viewportGeometry(w,h));
});
test('乱序基线必须全部到齐，过期代次和序列不能触发就绪',()=>{
    const assembler=new BaselineAssembler();
    const chunk=(index,sequence=1)=>({sceneEpoch:Long.UONE,baselineId:Long.UONE,chunkCount:2,chunkIndex:index,stateSequence:Long.fromInt(sequence,true),entities:[{entityHandle:index+1}]});
    assert.equal(assembler.accept(chunk(1),Long.UONE,Long.UONE),null);
    const complete=assembler.accept(chunk(0),Long.UONE,Long.UONE);assert.deepEqual(complete.entities.map(x=>x.entityHandle),[1,2]);
    assert.equal(assembler.accept(chunk(0,2),Long.UONE,Long.UONE),null);
    assert.equal(assembler.accept(chunk(1,1),Long.UONE,Long.UONE),null);
    assert.equal(assembler.accept(chunk(1,2),Long.fromInt(2,true),Long.UONE),null);
    assert.equal(assembler.accept(chunk(1,2),Long.UONE,Long.UONE).stateSequence.toString(),'2');
    assembler.reset();assert.throws(()=>assembler.accept({...chunk(0),chunkCount:65},Long.UONE,Long.UONE));
    assembler.reset();assembler.accept(chunk(0),Long.UONE,Long.UONE);
    assert.throws(()=>assembler.accept({...chunk(1),entities:[{entityHandle:1}]},Long.UONE,Long.UONE));
});
test('同序号的快照不能抢经济回执，旧绑定被隔离，自动状态等待对应响应',async()=>{
    const original=global.WebSocket;let socket;
    class FakeSocket{
        static OPEN=1;
        constructor(){socket=this;this.readyState=0;this.sent=[];}
        send(bytes){this.sent.push(bytes);}
        close(){this.readyState=3;this.onclose?.();}
    }
    global.WebSocket=FakeSocket;global.location={protocol:'http:',host:'127.0.0.1:18080'};
    const connection=new GameConnection();
    const deliver=(id,seq,binding,value)=>{const bytes=encodePacket(id,seq,binding,value);socket.onmessage({data:bytes.buffer});};
    try{
        const connecting=connection.connect('单次测试票');socket.readyState=1;socket.onopen();await Promise.resolve();
        const enter=decodePacket(socket.sent.at(-1));deliver(20002,enter.seq,Long.fromInt(5,true),{sceneEpoch:Long.UONE,baselineId:Long.UONE});await connecting;
        const operation={opId:new Uint8Array(16).fill(9),commandSeq:Long.UONE,expectedRevision:Long.UONE,expectedRuleVersion:Long.UONE};
        let completed=false;const pending=connection.request(40001,{itemInstanceId:new Uint8Array(16).fill(8),slot:0,operation}).then(value=>{completed=true;return value;});
        const seq=decodePacket(socket.sent.at(-1)).seq;
        deliver(40003,seq,Long.fromInt(5,true),{items:[],revision:Long.UONE});await Promise.resolve();assert.equal(completed,false);
        deliver(40005,seq,Long.fromInt(4,true),{opId:operation.opId,commandSeq:Long.UONE});await Promise.resolve();assert.equal(completed,false);
        deliver(40005,seq,Long.fromInt(5,true),{opId:operation.opId,commandSeq:Long.UONE});assert.equal((await pending).commandSeq.toString(),'1');
        const auto=connection.request(30004,{sceneEpoch:Long.UONE,enabled:false,monsterCount:4});
        const autoSeq=decodePacket(socket.sent.at(-1)).seq;
        deliver(30005,autoSeq,Long.fromInt(5,true),{autoBattleEnabled:false,remainHostingSec:21600});assert.equal((await auto).remainHostingSec,21600);
    }finally{connection.close();global.WebSocket=original;delete global.location;}
});
