const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
const file=path.resolve(__dirname,'../../assets/scripts/app/InputTrace.ts');
const output=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}});
const loaded={exports:{}};new Function('module','exports',output.outputText)(loaded,loaded.exports);
const {inputTraceMessage,traceInput}=loaded.exports;

test('输入追踪只投影指定消息及字段，Long施法ID保持原精度并剔除凭据',()=>{
    const secrets={token:'secret',account:'private',password:'secret',operation:{opId:new Uint8Array(16)}};
    const cases=[
        ['send',30003,{...secrets,skillId:1,inputSequence:99},{skill_id:1,input_sequence:99}],
        ['send',30004,{...secrets,enabled:false},{enabled:false}],
        ['send',40009,{...secrets,itemId:36},{item_id:36}],
        ['send',40010,{...secrets,skillId:2},{skill_id:2}],
        ['send',40014,{...secrets,skillId:14,inputSequence:8,materialInstanceId:new Uint8Array(16)},{skill_id:14,input_sequence:8}],
        ['send',40015,{...secrets,unequip:true,itemInstanceId:new Uint8Array(16)},{unequip:true}],
        ['receive',30005,{...secrets,autoBattleEnabled:true},{enabled:true}],
        ['receive',30006,{...secrets,skillId:1,inputSequence:99,castId:{toString:()=> '9007199254740993'}},{skill_id:1,input_sequence:99,cast_id:'9007199254740993'}],
        ['receive',30007,{...secrets,attackSequence:9,eventId:{toString:()=> '9007199254740994'}},{attack_sequence:9,event_id:'9007199254740994'}],
        ['receive',30012,{...secrets,skillId:9,stepIndex:3,castId:{toString:()=> '9007199254740993'}},{skill_id:9,step_index:3,cast_id:'9007199254740993'}],
        ['receive',30008,{...secrets,skillId:11,attackSequence:0,outcome:1,eventId:{toString:()=> '9007199254740995'},castId:{toString:()=> '9007199254740993'}},
            {attack_sequence:0,skill_id:11,outcome:1,event_id:'9007199254740995',cast_id:'9007199254740993'}],
    ];
    for(const [direction,id,value,fields]of cases)assert.deepEqual(inputTraceMessage(direction,id,8,value),{event:direction,message_id:id,packet_seq:8,...fields});
    assert.equal(inputTraceMessage('send',10001,1,secrets),null);
    assert.equal(inputTraceMessage('receive',40003,1,secrets),null);
});
test('只有显式renderQA=1写入结构化日志，其他网址和空记录保持静默',()=>{
    const originalLocation=global.location,originalInfo=console.info,captured=[];
    console.info=(...args)=>captured.push(args);
    try{
        for(const query of ['', '?renderQA=0', '?renderQA=10', '?other=1']){
            global.location={search:query};traceInput({event:'button',path:'BottomHUD/Skills/BluePotion',blocked:false});
        }
        assert.equal(captured.length,0);
        global.location={search:'?renderQA=1'};traceInput(null);assert.equal(captured.length,0);
        traceInput({event:'button',path:'BottomHUD/Skills/BasicAttack',blocked:true});
        assert.equal(captured.length,1);assert.equal(captured[0][0],'输入验收');
        const value=JSON.parse(captured[0][1]);assert.equal(value.blocked,true);assert.equal(value.path,'BottomHUD/Skills/BasicAttack');assert(Number.isSafeInteger(value.at_ms));
    }finally{console.info=originalInfo;if(originalLocation===undefined)delete global.location;else global.location=originalLocation;}
});
