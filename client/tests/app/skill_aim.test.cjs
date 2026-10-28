const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
const root=path.resolve(__dirname,'../../..');
const Long=require('node:module').createRequire(path.join(root,'client/assets/scripts/app/AppRoot.ts'))('long');
const models=require('../../../.work/ui-tests/app/SkillModel.js');
const {NavigationIntent}=require('../../../.work/ui-tests/input/NavigationIntent.js');
const decorator=(...args)=>args.length>1?undefined:(...params)=>params.length===1?params[0]:undefined;
const decorators=new Proxy({},{get(){return decorator;}});
class Vec2{constructor(x=0,y=0){this.x=x;this.y=y;}set(value){this.x=value.x;this.y=value.y;return this;}static distance(a,b){return Math.hypot(a.x-b.x,a.y-b.y);}}
const game={totalTime:100};
function load(file,requireModule){const compiled=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,experimentalDecorators:true,esModuleInterop:true}});const module={exports:{}};new Function('require','module','exports',compiled.outputText)(requireModule,module,module.exports);return module.exports;}
const {MapTouchNavigator}=load(path.join(root,'client/assets/scripts/input/MapTouchNavigator.ts'),id=>id==='cc'?{_decorator:decorators,Component:class{},Vec2,game,warn:(_message,error)=>{throw error;}}:{});
test('真实地图轻触拦截只提交瞄准回调，原地面导航不变；普通轻触仍可更新导航',()=>{
    const nav=new NavigationIntent();nav.setGroundTarget({x:9,y:8});
    const navigator=new MapTouchNavigator();navigator.node={emit(){}};let casts=0,previews=0,aiming=true;
    navigator.bind(nav,()=>({x:4,y:4}),()=>{if(!aiming)return false;casts++;return true;},()=>previews++);
    const event={target:navigator.node,getID:()=>1,getUILocation:()=>new Vec2(20,30),propagationStopped:false};
    navigator.onStart(event);game.totalTime=200;navigator.onEnd(event);
    assert.equal(casts,1);assert.equal(previews,1);assert.deepEqual(nav.state.target,{x:9,y:8});
    aiming=false;navigator.onStart(event);game.totalTime=250;navigator.onEnd(event);assert.deepEqual(nav.state.target,{x:4,y:4});
    nav.setGroundTarget({x:9,y:8});navigator.bind(nav,()=>null);navigator.onStart(event);game.totalTime=300;navigator.onEnd(event);assert.deepEqual(nav.state.target,{x:9,y:8});
});

function app(){
    const definition=(id,kind,target_kind)=>({id,name:`技能${id}`,job_id:1,book_item_id:7,learn_level:1,initial_level:0,kind,target_kind,active:true,range:9,
        base_cost:3,level_cost:2,cast_ms:600,impact_base_ms:500,projectile_ms_per_cell:0,accuracy_per_level:0});
    const definitions=[definition(1,'fireball','enemy_entity'),definition(11,'thunderbolt','enemy_entity'),definition(23,'firebang','enemy_grid')];
    const sent=[],shown=[];
    const session={ready:true,role:{job:1},entry:{sceneEpoch:'1',resourceVersion:'5',playerHandle:1},inventory:{revision:'1',level:7,items:[],learnedSkills:definitions.map(row=>({skillId:row.id,level:0}))},quickSkills:new Map(),
        baseline:{entities:[{entityHandle:1,entityKind:1,hp:'100',mp:'100',position:{x:2,y:2},direction:4},{entityHandle:3,entityKind:3,hp:'100',position:{x:4,y:4},direction:0}]},connection:{send:(id,value)=>sent.push({id,value})}};
    const {AppRoot}=load(path.join(root,'client/assets/scripts/app/AppRoot.ts'),id=>id==='cc'?{_decorator:decorators,Component:class{}}
        :id==='../input/NavigationIntent'?{NavigationIntent}:id==='long'?Long:id==='./GameSession'?{session,roleIdentity:()=> 'roleA',operation:()=>({opId:new Uint8Array(16).fill(9),commandSeq:Long.UONE,expectedRevision:Long.UONE,expectedRuleVersion:Long.UONE})}:id==='./SkillModel'?models:id==='./CastingSupplyModel'?require('../../../.work/ui-tests/app/CastingSupplyModel.js'):{});
    const instance=new AppRoot(),saved={active:false,getComponent:()=>({string:''})};
    instance.world={gameplay:{skills:definitions,items:[]},map:{width:16,height:16},showAim:value=>shown.push(value)};instance.ui={top:null};instance.find=()=>saved;instance.error=error=>{throw error;};
    const row=id=>models.skillRows(instance.skillSnapshot()).find(row=>row.definition.id===id);
    return {instance,session,sent,shown,row,definitions,definition};
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
    const {GameWorldView}=load(path.join(root,'client/assets/scripts/app/GameWorldView.ts'),id=>id==='cc'?{Vec3,UITransform:class{},isValid:()=>false}:id==='./GridProjection'?require('../../../.work/ui-tests/app/GridProjection.js'):{});
    const world=new GameWorldView({}, {getComponent:()=>({convertToNodeSpaceAR:point=>point})}, {}, '',1,'1',1);
    world.map={width:4,height:4,tile_width:64,tile_height:32,soil_thickness_px:48,projection:'isometric_diamond',blocked:Array.from({length:4},(_,y)=>Array.from({length:4},(_,x)=>({x,y}))).flat(),cells:Array.from({length:4},()=>Array.from({length:4},()=>({blocked:true})))};
    const point=world.grid(1,1);assert.equal(world.screenToGrid(point),null);assert.deepEqual(world.screenToGrid(point,false),{x:1,y:1});
    const outside=world.grid(4,1);assert.equal(world.screenToGrid(outside),null);assert.equal(world.screenToGrid(outside,false),null);
    world.map.blocked=[];assert.deepEqual(world.screenToGrid(point),{x:1,y:1},'旧来源cells位不覆盖新的运行阻挡');
});
test('真实AppRoot方向瞄准不选替代怪、不限制点击到近端、拒绝原地，原地雷光直接提交且导航保持',()=>{
    const {instance,session,sent,row,definitions,definition}=app(),errors=[];
    definitions.push({...definition(10,'lightning','enemy_direction'),activation_kind:'cast'}, {...definition(24,'thunder_storm','enemy_self'),activation_kind:'cast'});
    session.inventory.learnedSkills.push({skillId:10,level:0},{skillId:24,level:0});instance.error=error=>errors.push(error.message);
    instance.nav.setGroundTarget({x:9,y:8});instance.castSkill(row(10));assert.equal(sent.length,0);
    instance.consumeGround({x:2,y:2});assert.match(errors[0],/选择方向/);assert(instance.aim);assert.equal(sent.length,0);
    instance.consumeGround({x:15,y:2});assert.equal(sent.length,1);assert.equal(sent[0].value.skillId,10);assert.equal(sent[0].value.direction,2);assert.equal(sent[0].value.targetHandle,0);
    assert.deepEqual(instance.nav.state.target,{x:9,y:8});
    instance.castSkill(row(24));assert.equal(sent.length,2);assert.equal(sent[1].value.skillId,24);assert.equal(sent[1].value.targetHandle,0);assert.deepEqual(sent[1].value.targetGrid,{x:2,y:2});
    assert.deepEqual(instance.nav.state.target,{x:9,y:8});
});
test('真实AppRoot模式与烈火走专用请求，关闭不扣本地魔法，未确认不翻状态、错误出生代次不提交',()=>{
    const {instance,session,sent,row,definitions,definition}=app();
    definitions.push({...definition(12,'thrusting','attack_mode'),activation_kind:'toggle'}, {...definition(26,'flaming_sword','attack_charge'),activation_kind:'charge'});
    session.inventory.learnedSkills.push({skillId:12,level:0},{skillId:26,level:0});
    const player=session.baseline.entities[0];player.life='0';player.mp='0';
    session.skillModes={sceneEpoch:'1',revision:'1',playerLife:'0',serverTimeUnixMs:'1900000000000',states:[{skillId:12,enabled:true,ready:false,remainingMs:0,cooldownRemainingMs:0},{skillId:26,enabled:false,ready:false,remainingMs:0,cooldownRemainingMs:0}]};
    instance.nav.setGroundTarget({x:9,y:8});instance.castSkill(row(12));assert.equal(sent[0].id,30009);assert.equal(sent[0].value.enabled,false);
    assert.equal(session.skillModes.states[0].enabled,true);assert.equal(player.mp,'0');assert.deepEqual(instance.nav.state.target,{x:9,y:8});
    player.mp='100';instance.castSkill(row(26));assert.equal(sent[1].id,30009);assert.equal(sent[1].value.skillId,26);assert.equal(sent[1].value.enabled,true);
    assert.equal(session.skillModes.states[1].ready,false);
    session.skillModes.states[1].ready=true;assert.throws(()=>instance.castSkill(row(26)),/已就绪/);assert.equal(sent.length,2);
    player.life='1';assert.throws(()=>instance.castSkill(row(12)),/尚未同步/);assert.equal(sent.length,2);
});
test('真实AppRoot普通攻击只发送朝向AttackIntent，空挥同样可操作且不造Cast或清导航',async()=>{
    const {instance,session,sent}=app();for(const entity of session.baseline.entities)entity.hp={isZero:()=>false,toString:()=> '100'};
    instance.nav.setGroundTarget({x:9,y:8});await instance.basicAttack();assert.equal(sent[0].id,30011);assert.equal(sent[0].value.skillId,0);assert.equal(sent[0].value.direction,3);
    session.baseline.entities.splice(1);await instance.basicAttack();assert.equal(sent[1].id,30011);assert.equal(sent[1].value.direction,4);assert.deepEqual(instance.nav.state.target,{x:9,y:8});
});
test('真实AppRoot将屏幕摇杆八方向转成逻辑方向发送，原生摇杆向量不被地图旋转修改',()=>{
    const {instance,session,sent}=app(),{GridProjection}=require('../../../.work/ui-tests/app/GridProjection.js');
    const projection=new GridProjection({width:17,height:17,tile_width:64,tile_height:32,soil_thickness_px:48});
    instance.world.logicalDirection=(x,y)=>projection.logicalDirectionFromScreen(x,y);instance.world.update=()=>{};
    const screens=[[0,1],[1,1],[1,0],[1,-1],[0,-1],[-1,-1],[-1,0],[-1,1]];
    screens.forEach(([x,y],index)=>{instance.nav.setJoystick({x,y});instance.update(.2);assert.equal(sent[index].id,20006);assert.equal(sent[index].value.direction,(index+7)%8);assert.deepEqual(instance.nav.state.direction,{x,y});});
});

test('真实AppRoot自身魔法盾不误选宠物；友方材料瞄准固定原护符，经Pending超时保留原op重试，不发30003或改导航',async()=>{
    const {instance,session,sent,row,definitions,definition}=app(),errors=[];
    definitions.push({...definition(31,'magic_shield','friendly_self')},{...definition(14,'soul_shield','friendly_grid'),area_radius:3,material_item_id:187,material_quantity:1});
    session.inventory.learnedSkills.push({skillId:31,level:0},{skillId:14,level:0});session.baseline.entities[0].life=Long.UZERO;session.entry.sceneEpoch=Long.UONE;
    session.baseline.entities.push({entityHandle:2,entityKind:2,hp:'100',position:{x:3,y:2}});instance.selectedTarget=2;instance.nav.setGroundTarget({x:9,y:8});
    instance.castSkill(row(31));assert.equal(sent.length,1);assert.equal(sent[0].id,30003);assert.equal(sent[0].value.targetHandle,0);assert.deepEqual(sent[0].value.targetGrid,{x:2,y:2});
    const item={instanceId:new Uint8Array(16).fill(7),itemId:187,version:Long.fromString('9007199254740993',true),quantity:Long.fromInt(5),bound:true};
    session.inventory.items=[];session.inventory.equipped=[{slot:100,item}];instance.world.gameplay.casting_supply={enabled:true,slot:100,item_ids:[187]};
    const {PendingOperations}=load(path.join(root,'client/assets/scripts/app/PendingOperations.ts'),id=>id==='long'?Long:{});session.pending=new PendingOperations();
    const requests=[];session.connection.request=(id,value)=>{requests.push({id,value});return Promise.reject(new Error('提交结果未知'));};instance.error=e=>errors.push(e.message);instance.status=()=>{};
    instance.castSkill(row(14));assert.equal(sent.length,1);assert(instance.aim.supply);instance.consumeGround({x:4,y:4});await new Promise(resolve=>setImmediate(resolve));
    assert.equal(requests.length,1);assert.equal(requests[0].id,40014);assert.equal(requests[0].value.materialVersion.toString(),'9007199254740993');assert.equal(requests[0].value.casterLife.toString(),'0');
    assert.equal(sent.filter(r=>r.value.skillId===14).length,0);assert.deepEqual(instance.nav.state.target,{x:9,y:8});assert.equal(item.quantity.toString(),'5');assert.equal(session.baseline.entities[0].mp,'100');assert.match(errors[0],/未知/);
    const pending=session.pending.get('roleA');assert(pending);assert.equal(pending.id,40014);const original=pending.value.operation.opId;
    session.entry.sceneEpoch=Long.fromInt(2,true);session.connection.request=(id,value)=>{requests.push({id,value});return Promise.resolve({opId:original,pending:false,errorCode:0});};
    await instance.submitPending();assert.equal(requests[1].value.sceneEpoch.toString(),'1');assert.deepEqual(requests[1].value.operation.opId,original);assert.equal(session.pending.get('roleA'),null);
});
test('真实AppRoot材料瞄准期间原护符变化拒绝，不换用背包；装入确认跨life变化不提交',async()=>{
    const {instance,session,row,definitions,definition}=app(),errors=[],calls=[];
    definitions.push({...definition(14,'soul_shield','friendly_grid'),material_item_id:187,material_quantity:1});session.inventory.learnedSkills.push({skillId:14,level:0});session.baseline.entities[0].life='0';
    const item={instanceId:new Uint8Array(16).fill(7),itemId:187,version:'3',quantity:'5',bound:true};session.inventory.equipped=[{slot:100,item}];session.inventory.items=[{...item,instanceId:new Uint8Array(16).fill(8)}];
    instance.world.gameplay.casting_supply={enabled:true,slot:100,item_ids:[187]};instance.world.gameplay.items=[{item_id:187,name:'护身符',kind:'casting_supply'}];
    instance.nav.setGroundTarget({x:9,y:8});instance.error=e=>errors.push(e.message);instance.economic=(id,value)=>{calls.push({id,value});return Promise.resolve();};
    instance.castSkill(row(14));item.version='4';instance.consumeGround({x:3,y:3});assert.match(errors[0],/原护符/);assert.equal(calls.length,0);assert.deepEqual(instance.nav.state.target,{x:9,y:8});
    instance.selectedItem=session.inventory.items[0];instance.popup={confirm:async()=>{session.baseline.entities[0].life='1';return true;}};
    await assert.rejects(instance.setCastingSupply(false),/已变化/);assert.equal(calls.length,0);
});

test('实际disabled护符门禁在Use/快捷共用施法入口和装卸入口前拒绝，不进瞄准或发送任何材料请求',async()=>{
    const {instance,session,row,definitions,definition,sent}=app();definitions.push({...definition(14,'soul_shield','friendly_grid'),material_item_id:187,material_quantity:1});session.inventory.learnedSkills.push({skillId:14,level:0});
    instance.world.gameplay.casting_supply={enabled:false,slot:100,item_ids:[187]};instance.nav.setGroundTarget({x:9,y:8});instance.economic=()=>{throw new Error('禁止创建经济请求');};
    assert.throws(()=>instance.castSkill(row(14)),/施法护符暂未开放/);assert.equal(instance.aim,null);assert.equal(sent.length,0);assert.deepEqual(instance.nav.state.target,{x:9,y:8});
    assert.equal(models.skillActionLabel(instance.skillSnapshot(),row(14)),'护符未开放');assert.equal(models.skillQuickStateLabel(instance.skillSnapshot(),row(14)),'未开放');
    await assert.rejects(instance.setCastingSupply(false),/施法护符暂未开放/);await assert.rejects(instance.setCastingSupply(true),/施法护符暂未开放/);assert.equal(sent.length,0);
});
