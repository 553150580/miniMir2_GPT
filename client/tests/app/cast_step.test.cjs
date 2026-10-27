const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/Resources/app.asar.unpacked/node_modules/typescript');
const model=require('../../../.work/ui-tests/app/SkillModel.js');
const playback=require('../../../.work/ui-tests/app/CastPlayback.js');
const definition={id:9,name:'地狱火',kind:'hell_fire',active:true,activation_kind:'cast',target_kind:'enemy_direction',line_cells:4,line_step_ms:100,cast_ms:600,impact_base_ms:500};
const event=changes=>({sceneEpoch:'9007199254740993',castId:'9007199254740993',skillId:9,casterHandle:1,casterLife:'0',grid:{x:3,y:2},direction:2,stepIndex:1,skillLevel:3,...changes});
test('地狱火全阶单线4格只按真实Step逐格启动，停顿不预排，重复格和过期格不重播',()=>{
    const value=new playback.CastStepPlayback();value.bind('9007199254740993');
    assert.deepEqual(value.active(1000,500),[]);
    const first=event();assert(value.accept(first,definition,700));first.grid.x=99;
    assert.equal(value.active(700,500)[0].event.grid.x,3);assert.equal(value.active(1199,500).length,1);
    assert.deepEqual(value.active(1200,500),[]);assert(!value.accept(event(),definition,1300));
    assert(value.accept(event({grid:{x:4,y:2},stepIndex:2}),definition,1500));assert.equal(value.active(1500,500).length,1);
    assert(!value.accept(event({stepIndex:5}),definition,1500));assert(!value.accept(event({sceneEpoch:'2',stepIndex:3}),definition,1500));
    for(const level of [0,1,2,3])assert(!value.accept(event({castId:String(9007199254741000n+BigInt(level)),stepIndex:5,skillLevel:level}),definition,1600));
});
test('健康重入不从旧起手补步，仍可展示当前连接随后收到的第3格；有界去重淘汰后旧cast仍拒绝',()=>{
    const value=new playback.CastStepPlayback();value.bind('9007199254740993');
    assert(value.accept(event({stepIndex:3}),definition,0));assert.equal(value.active(0,500)[0].event.stepIndex,3);
    for(let i=1;i<80;i++)value.accept(event({castId:String(9007199254740993n+BigInt(i))}),definition,i);
    assert(!value.accept(event(),definition,100));
    value.bind('9007199254740993');assert.deepEqual(value.active(1000,500),[]);assert(value.accept(event({stepIndex:4}),definition,1200));
});
function load(file,requireModule){const result=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}),module={exports:{}};new Function('require','module','exports',result.outputText)(requireModule,module,module.exports);return module.exports;}
class Vec3{constructor(x=0,y=0,z=0){this.x=x;this.y=y;this.z=z;}}
const source=path.resolve(__dirname,'../../assets/scripts/app');
const {effectFrameKeys}=load(path.join(source,'WorldAssets.ts'),()=>({}));
const {CombatEffects}=load(path.join(source,'CombatEffects.ts'),id=>id==='cc'?{Vec3}:id==='./CastPlayback'?playback:id==='./WorldAssets'?{effectFrameKeys}:id==='./SkillModel'?model:{});
function effects(){
    const grid=(x,y)=>new Vec3(x*48,y*32),drawn=[],beam={directions:Array.from({length:8},()=>['Magic:970']),duration_ms:600};
    const assets={map:{width:16,height:16},gameplay:{skills:[definition,{...definition,id:10,kind:'lightning'}]},frames:new Map(['Magic:920','Magic:930','Magic:970'].map(key=>[key,{}])),
        manifest:{effects:{'9':{cast:{frames:['Magic:920'],duration_ms:600},cast_anchor:'origin',step:{frames:['Magic:930'],duration_ms:500}},'10':{beam,beam_anchor:'origin'}},frames:Object.fromEntries(['Magic:920','Magic:930','Magic:970'].map(key=>[key,{declared_empty:false}]))}};
    const value=new CombatEffects(assets,{},'9007199254740993',grid,()=>new Vec3(999,999),()=> '0');value.draw=(id,key,point)=>drawn.push({id,key,point});
    return {value,drawn,grid};
}
test('真实CombatEffects地狱火起手不生成传播，空格Step独立播放，重复Impact不生成格火焰',()=>{
    const {value,drawn,grid}=effects();
    value.accept({sceneEpoch:'9007199254740993',castId:'9007199254740993',skillId:9,casterHandle:1,casterLife:'0',targetHandle:0,origin:{x:2,y:2},targetGrid:{x:15,y:2},direction:2,delayMs:500},0);
    value.update(500);assert(drawn.every(row=>!row.id.startsWith('step:')));assert.deepEqual(drawn[0].point,grid(2,2));drawn.length=0;
    value.update(700);assert.deepEqual(drawn,[]);
    value.step(event(),700);value.update(700);assert.equal(drawn.length,1);assert.deepEqual(drawn[0].point,grid(3,2));drawn.length=0;
    value.impact({sceneEpoch:'9007199254740993',eventId:'1',castId:'9007199254740993',skillId:9,targetHandle:3,targetLife:'0',outcome:1},800);
    value.update(800);assert.equal(drawn.length,1);drawn.length=0;
    value.step(event({grid:{x:4,y:2},stepIndex:2}),1100);value.update(1100);assert.equal(drawn.length,2);drawn.length=0;
    value.update(1600);assert.deepEqual(drawn,[]);value.step(event(),1700);value.update(1700);assert.deepEqual(drawn,[]);
});
test('真实CombatEffects方向梁固定受理原点，不跟随当前角色；未来Step不依赖本连接曾见CastAccepted',()=>{
    const {value,drawn,grid}=effects();
    value.accept({sceneEpoch:'9007199254740993',castId:'9',skillId:10,casterHandle:1,casterLife:'0',targetHandle:0,origin:{x:2,y:2},targetGrid:{x:15,y:2},direction:2,delayMs:500},0);
    value.update(600);assert.equal(drawn.length,1);assert.deepEqual(drawn[0].point,grid(2,2));assert.notEqual(drawn[0].point.x,999);
    const fresh=effects();fresh.value.step(event({stepIndex:3,grid:{x:5,y:2}}),900);fresh.value.update(900);assert.equal(fresh.drawn.length,1);assert.deepEqual(fresh.drawn[0].point,grid(5,2));
    fresh.drawn.length=0;fresh.value.step(event({castId:'10',casterLife:'1',stepIndex:4}),1400);fresh.value.update(1400);assert.deepEqual(fresh.drawn,[]);
});
