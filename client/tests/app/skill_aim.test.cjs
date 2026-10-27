const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/Resources/app.asar.unpacked/node_modules/typescript');
const root=path.resolve(__dirname,'../../..');
const models=require('../../../.work/ui-tests/app/SkillModel.js');
const {NavigationIntent}=require('../../../.work/ui-tests/input/NavigationIntent.js');
const decorator=(...args)=>args.length>1?undefined:(...params)=>params.length===1?params[0]:undefined;
const decorators=new Proxy({},{get(){return decorator;}});
class Vec2{constructor(x=0,y=0){this.x=x;this.y=y;}set(value){this.x=value.x;this.y=value.y;return this;}static distance(a,b){return Math.hypot(a.x-b.x,a.y-b.y);}}
const game={totalTime:100};
function load(file,requireModule){const compiled=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,experimentalDecorators:true}});const module={exports:{}};new Function('require','module','exports',compiled.outputText)(requireModule,module,module.exports);return module.exports;}
const {MapTouchNavigator}=load(path.join(root,'client/assets/scripts/input/MapTouchNavigator.ts'),id=>id==='cc'?{_decorator:decorators,Component:class{},Vec2,game,warn:(_message,error)=>{throw error;}}:{});
test('真实地图轻触拦截只提交瞄准回调，原地面导航不变；普通轻触仍可更新导航',()=>{
    const nav=new NavigationIntent();nav.setGroundTarget({x:9,y:8});
    const navigator=new MapTouchNavigator();navigator.node={emit(){}};let casts=0,previews=0,aiming=true;
    navigator.bind(nav,()=>({x:4,y:4}),()=>{if(!aiming)return false;casts++;return true;},()=>previews++);
    const event={target:navigator.node,getID:()=>1,getUILocation:()=>new Vec2(20,30),propagationStopped:false};
    navigator.onStart(event);game.totalTime=200;navigator.onEnd(event);
    assert.equal(casts,1);assert.equal(previews,1);assert.deepEqual(nav.state.target,{x:9,y:8});
    aiming=false;navigator.onStart(event);game.totalTime=250;navigator.onEnd(event);assert.deepEqual(nav.state.target,{x:4,y:4});
});

function app(){
    const definition=(id,kind,target_kind)=>({id,name:`技能${id}`,job_id:1,book_item_id:7,learn_level:1,initial_level:0,kind,target_kind,active:true,range:9,
        base_cost:3,level_cost:2,cast_ms:600,impact_base_ms:500,projectile_ms_per_cell:0,accuracy_per_level:0});
    const definitions=[definition(1,'fireball','enemy_entity'),definition(11,'thunderbolt','enemy_entity'),definition(23,'firebang','enemy_grid')];
    const sent=[],shown=[];
    const session={ready:true,role:{job:1},entry:{sceneEpoch:'1',resourceVersion:'5'},inventory:{revision:'1',level:7,items:[],learnedSkills:definitions.map(row=>({skillId:row.id,level:0}))},quickSkills:new Map(),
        baseline:{entities:[{entityHandle:1,entityKind:1,hp:'100',mp:'100',position:{x:2,y:2},direction:4},{entityHandle:3,entityKind:3,hp:'100',position:{x:4,y:4},direction:0}]},connection:{send:(id,value)=>sent.push({id,value})}};
    const {AppRoot}=load(path.join(root,'client/assets/scripts/app/AppRoot.ts'),id=>id==='cc'?{_decorator:decorators,Component:class{}}
        :id==='../input/NavigationIntent'?{NavigationIntent}:id==='./GameSession'?{session,roleIdentity:()=> 'roleA'}:id==='./SkillModel'?models:{});
    const instance=new AppRoot(),saved={active:false,getComponent:()=>({string:''})};
    instance.world={gameplay:{skills:definitions,items:[]},map:{width:16,height:16},showAim:value=>shown.push(value)};instance.ui={top:null};instance.find=()=>saved;instance.error=error=>{throw error;};
    const row=id=>models.skillRows(instance.skillSnapshot()).find(row=>row.definition.id===id);
    return {instance,session,sent,shown,row};
}
test('真实AppRoot可以施放列表第二主动技能，不再强制第一个技能，快捷选择按角色隔离',()=>{
    const {instance,session,sent,row}=app();instance.castSkill(row(11));assert.equal(sent.length,1);assert.equal(sent[0].value.skillId,11);assert.equal(sent[0].value.targetHandle,3);
    session.quickSkills.set('roleA',11);assert.equal(instance.quickSkill().definition.id,11);
    session.role.job=2;assert.equal(instance.quickSkill(),null);
});
test('真实AppRoot地面瞄准、取消与提交均不改原导航；提交只含原技能及明确地面格',()=>{
    const {instance,sent,row}=app();instance.nav.setGroundTarget({x:9,y:8});
    instance.castSkill(row(23));assert.equal(sent.length,0);assert.deepEqual(instance.nav.state.target,{x:9,y:8});
    instance.cancelAim();assert.deepEqual(instance.nav.state.target,{x:9,y:8});assert.equal(sent.length,0);
    instance.castSkill(row(23));assert.equal(instance.consumeGround({x:4,y:4}),true);
    assert.equal(sent.length,1);assert.equal(sent[0].value.skillId,23);assert.equal(sent[0].value.targetHandle,0);assert.deepEqual(sent[0].value.targetGrid,{x:4,y:4});
    assert.deepEqual(instance.nav.state.target,{x:9,y:8});assert.equal(instance.aim,null);
});
test('真实AppRoot可提交blocked center，越界但仍在射程内也必须拒绝，导航始终保留',()=>{
    const {instance,sent,row}=app(),errors=[];
    instance.world.map={width:4,height:4,cells:Array.from({length:4},()=>Array.from({length:4},()=>({blocked:true})))};
    instance.error=error=>errors.push(error.message);instance.nav.setGroundTarget({x:3,y:2});
    instance.castSkill(row(23));instance.consumeGround({x:4,y:1});
    assert.equal(sent.length,0);assert.match(errors[0],/地图边界/);assert(instance.aim);assert.deepEqual(instance.nav.state.target,{x:3,y:2});
    instance.consumeGround({x:1,y:1});assert.equal(sent.length,1);assert.deepEqual(sent[0].value.targetGrid,{x:1,y:1});assert.deepEqual(instance.nav.state.target,{x:3,y:2});
});
test('真实世界坐标转换：普通导航拒绝阻挡中心，瞄准允许同格；地图外两种模式均拒绝',()=>{
    class Vec3{constructor(x=0,y=0,z=0){this.x=x;this.y=y;this.z=z;}}
    const {GameWorldView}=load(path.join(root,'client/assets/scripts/app/GameWorldView.ts'),id=>id==='cc'?{Vec3,UITransform:class{},isValid:()=>false}:{});
    const world=new GameWorldView({}, {getComponent:()=>({convertToNodeSpaceAR:point=>point})}, {}, '',1,'1');
    world.map={width:4,height:4,cells:Array.from({length:4},()=>Array.from({length:4},()=>({blocked:true})))};
    const point=world.grid(1,1);assert.equal(world.screenToGrid(point),null);assert.deepEqual(world.screenToGrid(point,false),{x:1,y:1});
    const outside=world.grid(4,1);assert.equal(world.screenToGrid(outside),null);assert.equal(world.screenToGrid(outside,false),null);
});
