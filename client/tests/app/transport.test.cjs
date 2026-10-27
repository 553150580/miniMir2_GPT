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
test('真实逐格CastStep保留64位场景/施法/生命和原格，空有效格不需要target字段',()=>{
    const large=Long.fromString('9007199254740993',true),step=decodePacket(encodePacket(30012,0,Long.UONE,{sceneEpoch:Long.MAX_UNSIGNED_VALUE,castId:large,skillId:9,casterHandle:1,casterLife:large,grid:{x:4,y:2},direction:2,stepIndex:3,skillLevel:3})).value;
    assert.equal(step.sceneEpoch.toString(),'18446744073709551615');assert.equal(step.castId.toString(),large.toString());assert.equal(step.casterLife.toString(),large.toString());
    assert.deepEqual(step.grid,{x:4,y:2});assert.equal(step.stepIndex,3);assert.equal(step.skillLevel,3);assert.equal(step.targetHandle,undefined);
});

test('战士模式、朝向空挥及受理阶数/生命按新PB往返，模式基线跨分块仍完整恢复',()=>{
    const large=Long.fromString('9007199254740993',true),mode={sceneEpoch:large,revision:large,playerLife:Long.UZERO,serverTimeUnixMs:large,
        states:[{skillId:26,enabled:false,ready:true,remainingMs:10000,cooldownRemainingMs:10000}]};
    const request=decodePacket(encodePacket(30009,1,Long.UONE,{sceneEpoch:large,inputSequence:4294967295,skillId:12,enabled:true})).value;
    assert.equal(request.sceneEpoch.toString(),large.toString());assert.equal(request.enabled,true);assert.equal(request.inputSequence,4294967295);
    const attack=decodePacket(encodePacket(30011,2,Long.UONE,{sceneEpoch:large,inputSequence:3,direction:7,skillId:0})).value;assert.equal(attack.direction,7);assert.equal(attack.skillId,0);
    const state=decodePacket(encodePacket(30010,0,Long.UONE,mode)).value;assert.equal(state.playerLife.toString(),'0');assert.equal(state.revision.toString(),large.toString());assert.equal(state.states[0].ready,true);
    const accepted=decodePacket(encodePacket(30007,0,Long.UONE,{skillId:25,skillLevel:3,casterLife:large})).value;assert.equal(accepted.skillId,25);assert.equal(accepted.skillLevel,3);assert.equal(accepted.casterLife.toString(),large.toString());
    const cast=decodePacket(encodePacket(30006,0,Long.UONE,{skillId:33,skillLevel:2})).value;assert.equal(cast.skillLevel,2);
    const assembler=new BaselineAssembler(),chunk={sceneEpoch:large,baselineId:large,stateSequence:Long.UONE,chunkCount:2};
    const first=decodePacket(encodePacket(5,0,Long.UONE,{...chunk,chunkIndex:0,entities:[{entityHandle:1,attackSkillId:26,attackSkillLevel:3}],skillModeState:mode})).value;
    assert.equal(assembler.accept(first,large,large),null);
    const last=decodePacket(encodePacket(5,0,Long.UONE,{...chunk,chunkIndex:1,entities:[{entityHandle:2}]})).value;
    const baseline=assembler.accept(last,large,large);assert.equal(baseline.skillModeState.revision.toString(),large.toString());assert.equal(baseline.entities[0].attackSkillId,26);assert.equal(baseline.entities[0].attackSkillLevel,3);
});

test('物品来源品质与技能报价字段精确往返，未知旧来源保持空值，报价提交只携原ID',()=>{
    const item={instanceId:new Uint8Array(16).fill(3),itemId:7,quantity:Long.MAX_VALUE,bound:false,version:Long.MAX_UNSIGNED_VALUE,sourceKind:'monster_drop',sourceId:'dev-drop-1',quality:1};
    const legacy=decodePacket(encodePacket(40003,1,Long.UONE,{items:[{itemId:7,quantity:Long.ONE}]})).value.items[0];
    assert.equal(legacy.sourceKind,'');assert.equal(legacy.sourceId,'');assert.equal(legacy.quality,0);
    const request=decodePacket(encodePacket(40011,19,Long.UONE,{kind:2,skillId:1,materials:[{instanceId:item.instanceId,version:item.version,quantity:Long.fromInt(2)}]})).value;
    assert.equal(request.materials[0].version.toString(),'18446744073709551615');assert.equal(request.materials[0].quantity.toString(),'2');
    const previewId=new Uint8Array(16).fill(9),preview=decodePacket(encodePacket(40012,19,Long.UONE,{previewId,kind:2,ruleVersion:Long.MAX_UNSIGNED_VALUE,configHash:'a'.repeat(64),roleRevision:Long.fromString('9007199254740993',true),expiresAtUnixMs:Long.fromString('1900000000000',true),skillId:1,fromLevel:0,toLevel:1,
        spend:{gold:Long.fromInt(200),boundGold:Long.fromInt(300)},materials:[{item,consumeQuantity:Long.fromInt(2)}],outputs:[{...item,itemId:900001,bound:true}],stats:[{key:'accuracy_bonus',beforeMin:Long.ZERO,beforeMax:Long.ZERO,afterMin:Long.fromInt(3),afterMax:Long.fromInt(3)}]})).value;
    assert.equal(preview.materials[0].item.sourceKind,'monster_drop');assert.equal(preview.materials[0].item.quality,1);assert.equal(preview.materials[0].item.quantity.toString(),'9223372036854775807');
    assert.equal(preview.roleRevision.toString(),'9007199254740993');assert.equal(preview.expiresAtUnixMs.toString(),'1900000000000');assert.equal(preview.stats[0].beforeMin.toString(),'0');
    const commit=decodePacket(encodePacket(40013,20,Long.UONE,{previewId,operation:{opId:new Uint8Array(16).fill(8),commandSeq:Long.UONE,expectedRevision:preview.roleRevision,expectedRuleVersion:preview.ruleVersion}})).value;
    assert.deepEqual(commit.previewId,previewId);assert.equal(commit.operation.expectedRevision.toString(),'9007199254740993');
});
test('攻击受理、命中与出生代次保留Long精度，治疗入队不被改成直接回血数值',()=>{
    const large=Long.fromString('9007199254740993',true);
    const cast=decodePacket(encodePacket(30006,0,Long.UONE,{sceneEpoch:Long.UONE,castId:large,skillId:11,casterLife:large,targetLife:large})).value;
    assert.equal(cast.casterLife.toString(),large.toString());assert.equal(cast.targetLife.toString(),large.toString());
    const attack=decodePacket(encodePacket(30007,0,Long.UONE,{sceneEpoch:Long.UONE,eventId:large,casterHandle:1,targetHandle:3,attackSequence:4294967295,inputSequence:9,delayMs:500})).value;
    assert.equal(attack.eventId.toString(),large.toString());assert.equal(attack.attackSequence,4294967295);
    const impact=decodePacket(encodePacket(30008,0,Long.UONE,{sceneEpoch:Long.UONE,eventId:large,castId:large,skillId:29,targetHandle:1,targetLife:large,amount:Long.MAX_VALUE,outcome:4,healing:true})).value;
    assert.equal(impact.castId.toString(),large.toString());assert.equal(impact.targetLife.toString(),large.toString());assert.equal(impact.amount.toString(),'9223372036854775807');assert.equal(impact.outcome,4);assert.equal(impact.healing,true);
    const entity=decodePacket(encodePacket(5,0,Long.UONE,{entities:[{entityHandle:1,life:large,attackIntervalMs:1386}]})).value.entities[0];
    assert.equal(entity.life.toString(),large.toString());assert.equal(entity.attackIntervalMs,1386);
});

test('学习0级状态、原书学习请求及施法事件大ID按新PB定义精确往返',()=>{
    const instance=new Uint8Array(16).fill(7),opId=new Uint8Array(16).fill(8);
    const request=decodePacket(encodePacket(40010,19,Long.UONE,{itemInstanceId:instance,skillId:1,operation:{opId,
        commandSeq:Long.MAX_UNSIGNED_VALUE,expectedRevision:Long.UONE,expectedRuleVersion:Long.UONE}})).value;
    assert.deepEqual(request.itemInstanceId,instance);assert.equal(request.skillId,1);assert.equal(request.operation.commandSeq.toString(),'18446744073709551615');
    const inventory=decodePacket(encodePacket(40003,0,Long.UONE,{learnedSkills:[{skillId:1,level:0}]})).value;
    assert.deepEqual(inventory.learnedSkills,[{skillId:1,level:0}]);
    const event=decodePacket(encodePacket(30006,0,Long.UONE,{sceneEpoch:Long.MAX_UNSIGNED_VALUE,castId:Long.fromString('9007199254740993',true),skillId:1,
        casterHandle:1,targetHandle:3,origin:{x:2,y:3},targetGrid:{x:5,y:3},direction:2,delayMs:650,inputSequence:99})).value;
    assert.equal(event.sceneEpoch.toString(),'18446744073709551615');assert.equal(event.castId.toString(),'9007199254740993');
    assert.equal(event.delayMs,650);assert.equal(event.inputSequence,99);assert.deepEqual(event.targetGrid,{x:5,y:3});
    const baseline=decodePacket(encodePacket(5,0,Long.UONE,{entities:[{entityHandle:1,action:5,castSequence:4294967295,castSkillId:1}]})).value;
    assert.equal(baseline.entities[0].castSequence,4294967295);assert.equal(baseline.entities[0].castSkillId,1);
});

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
        let quoted=false;const preview=connection.request(40011,{kind:1,offerId:1,quantity:Long.ONE}).then(value=>{quoted=true;return value;});
        const previewSeq=decodePacket(socket.sent.at(-1)).seq;
        deliver(40003,previewSeq,Long.fromInt(5,true),{items:[],revision:Long.UONE});await Promise.resolve();assert.equal(quoted,false);
        deliver(40012,previewSeq,Long.fromInt(5,true),{previewId:new Uint8Array(16).fill(5),kind:1,offerId:1,quantity:Long.ONE});assert.equal((await preview).offerId,1);
    }finally{connection.close();global.WebSocket=original;delete global.location;}
});

test('防御状态/属性及材料施法/装卸精确保留64位，完整空状态不会成为保留指令',()=>{
    const large=Long.fromString('9007199254740993',true),operation={opId:new Uint8Array(16).fill(7),commandSeq:large,expectedRevision:large,expectedRuleVersion:large};
    const value=decodePacket(encodePacket(40014,4,Long.UONE,{sceneEpoch:large,inputSequence:9,skillId:14,targetHandle:0,targetGrid:{x:8,y:9},direction:4,materialInstanceId:new Uint8Array(16).fill(5),materialVersion:large,casterLife:Long.UZERO,operation})).value;
    assert.equal(value.materialVersion.toString(),large.toString());assert.equal(value.casterLife.toString(),'0');assert.equal(value.operation.commandSeq.toString(),large.toString());assert.deepEqual(value.targetGrid,{x:8,y:9});
    const supply=decodePacket(encodePacket(40015,5,Long.UONE,{itemInstanceId:new Uint8Array(16).fill(5),itemVersion:large,unequip:true,operation})).value;assert.equal(supply.itemVersion.toString(),large.toString());assert.equal(supply.unequip,true);
    const baseline=decodePacket(encodePacket(5,0,Long.UONE,{entities:[{entityHandle:1,life:Long.UZERO,statuses:[{statusId:31,sourceSkillId:31,skillLevel:0,remainingMs:0,maxPhysicalDefenceBonus:Long.MAX_VALUE,maxMagicDefenceBonus:Long.MAX_VALUE,damageReductionPercent:20,casterHandle:1,casterLife:large}],combatAttributes:{physicalAttackMin:Long.MAX_VALUE,magicDefenceMax:Long.MAX_VALUE,damageReductionPercent:20}},{entityHandle:2,statuses:[]}]})).value;
    assert.equal(baseline.entities[0].statuses[0].casterLife.toString(),large.toString());assert.equal(baseline.entities[0].statuses[0].maxMagicDefenceBonus.toString(),Long.MAX_VALUE.toString());assert.equal(baseline.entities[0].statuses[0].remainingMs,0);
    assert.equal(baseline.entities[0].combatAttributes.physicalAttackMin.toString(),Long.MAX_VALUE.toString());assert.deepEqual(baseline.entities[1].statuses,[]);
});
