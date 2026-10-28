const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
const source=path.resolve(__dirname,'../../assets/scripts/app');
const {GridProjection}=require('../../../.work/ui-tests/app/GridProjection.js');
const {beamGeometry}=require('../../../.work/ui-tests/app/BeamGeometry.js');
const models=require('../../../.work/ui-tests/app/SkillModel.js'),playback=require('../../../.work/ui-tests/app/CastPlayback.js');
function load(file,requireModule){const output=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS}}),module={exports:{}};new Function('require','module','exports',output.outputText)(requireModule,module,module.exports);return module.exports;}
class Vec3{constructor(x=0,y=0,z=0){this.x=x;this.y=y;this.z=z;}}
class UITransform{constructor(){this.width=1;this.height=1;this.anchorPoint={x:.5,y:.5};}setContentSize(w,h){this.width=w;this.height=h;}setAnchorPoint(x,y){this.anchorPoint={x,y};}convertToNodeSpaceAR(p){return p;}}
class Sprite{static Type={SIMPLE:0};static SizeMode={CUSTOM:0};}
class Label{}class ProgressBar{}class Color{constructor(r,g,b,a){Object.assign(this,{r,g,b,a});}}
class Graphics{constructor(){this.operations=[];}clear(){this.operations=[];}moveTo(x,y){this.operations.push(['move',x,y]);}lineTo(x,y){this.operations.push(['line',x,y]);}close(){this.operations.push(['close']);}fill(){this.operations.push(['fill',this.fillColor]);}stroke(){this.operations.push(['stroke',this.strokeColor]);}}
class Node{
    static EventType={SIZE_CHANGED:'size',TOUCH_END:'touch'};
    constructor(name){this.name=name;this.children=[];this.parts=new Map();this.position=new Vec3();this.scale=new Vec3(1,1,1);this.active=true;this.add(UITransform,new UITransform());}
    add(type,value){value.node=this;this.parts.set(type,value);return value;}getComponent(type){return this.parts.get(type);}
    set parent(value){if(this._parent)this._parent.children=this._parent.children.filter(n=>n!==this);this._parent=value;if(value)value.children.push(this);}get parent(){return this._parent;}
    getChildByName(name){return this.children.find(n=>n.name===name);}getChildByPath(path){return path.split('/').reduce((n,key)=>n?.getChildByName(key),this);}
    setPosition(x,y,z=0){this.position=typeof x==='object'?new Vec3(x.x,x.y,x.z):new Vec3(x,y,z);}setSiblingIndex(i){const a=this.parent.children;a.splice(a.indexOf(this),1);a.splice(i,0,this);}getSiblingIndex(){return this.parent.children.indexOf(this);}
    on(){}off(){}destroy(){this.destroyed=true;}
}
const geometry={width:17,height:17,tile_width:64,tile_height:32,soil_thickness_px:48,projection:'isometric_diamond'},projection=new GridProjection(geometry);
const cc={Node,UITransform,Sprite,Label,ProgressBar,Graphics,Color,Vec3,isValid:()=>true,game:{totalTime:100}};
test('真实资源预载在取票前拒绝新旧地图混包并释放已持有资源，不进入固定场景后才发现错位',async()=>{
    const loaded=[],released=[],map={...geometry,surface_width:1088,surface_height:544,world_height:591,background_offset_y:24};
    const values={'development/manifest':{map:{...geometry}},'development/gameplay':{},'development/maps/first-map':map};
    const resources={load(path,_type,done){loaded.push(path);done(null,{json:values[path],addRef(){},decRef(){released.push(path);}});}};
    const {WorldAssets}=load(path.join(source,'WorldAssets.ts'),id=>id==='cc'?{resources}:id==='./GridProjection'?{GridProjection}:{});
    await assert.rejects(WorldAssets.prepare(0,()=>{}),/地图地板、投影与来源清单不一致/);
    assert.deepEqual(loaded,['development/manifest','development/gameplay','development/maps/first-map']);assert.deepEqual(released,loaded);
});
test('真实地图绘制只使用保存节点：两侧48像素土层、18+18网格、独立Surface与菱形目标',()=>{
    const world=new Node('World');world.add(Sprite,new Sprite());
    for(const [name,type]of [['Soil',Graphics],['Surface',Sprite],['GridLines',Graphics],['TargetMarker',Sprite]]){const n=new Node(name);n.parent=world;n.add(type,new type());}
    const diamond=new Node('Diamond');diamond.parent=world.getChildByName('TargetMarker');diamond.add(Graphics,new Graphics());
    const {prepareMapSurface}=load(path.join(source,'MapSurface.ts'),id=>id==='cc'?cc:{});prepareMapSurface(world,{background:{}},projection);
    assert.equal(world.getComponent(Sprite).enabled,false);assert.equal(world.getComponent(UITransform).height,592);
    const surface=world.getChildByName('Surface');assert.equal(surface.position.y,24);assert.equal(surface.getComponent(UITransform).width,1088);assert.equal(surface.getComponent(UITransform).height,544);
    const operations=world.getChildByName('Soil').getComponent(Graphics).operations;assert.equal(operations.filter(row=>row[0]==='fill').length,4);assert.equal(Math.min(...operations.filter(row=>row[0]==='move'||row[0]==='line').map(row=>row[2])),-296);
    const grid=world.getChildByName('GridLines').getComponent(Graphics).operations;assert.equal(grid.filter(row=>row[0]==='move').length,36);
    assert.equal(world.getChildByName('TargetMarker').getComponent(Sprite).enabled,false);assert(diamond.getComponent(Graphics).operations.some(row=>row[0]==='line'&&row[1]===32&&row[2]===0));
});
test('真实世界渲染将直立物件与角色统一排序，身体武器及刀光样本共同映射视觉方向',()=>{
    const {EntityMotion}=load(path.join(source,'EntityMotion.ts'),()=>({})),{EntityAnimation,movementDisplayDurationMs}=load(path.join(source,'EntityAnimation.ts'),()=>({})),{EntityDisplacement}=load(path.join(source,'EntityDisplacement.ts'),()=>({}));let effects;
    class Effects{constructor(...args){this.sample=args[6];effects=this;}update(){}attack(){}dispose(){}}
    const makeSprite=name=>{const n=new Node(name);n.add(Sprite,new Sprite());return n;};
    cc.instantiate=prefab=>{if(prefab==='prop')return makeSprite('prop');if(prefab instanceof Node)return makeSprite(prefab.name);const n=new Node('entity'),body=makeSprite('Body');body.getComponent(UITransform).setContentSize(64,96);body.parent=n;const name=new Node('Name');name.add(Label,{});name.setPosition(0,78);name.scale=new Vec3(.7,.7,1);name.parent=n;const hp=new Node('HP');hp.add(ProgressBar,{});hp.setPosition(0,58);hp.parent=n;return n;};
    const {GameWorldView}=load(path.join(source,'GameWorldView.ts'),id=>id==='cc'?cc:id==='./GridProjection'?require('../../../.work/ui-tests/app/GridProjection.js'):id==='./EntityHeadAnchor'?require('../../../.work/ui-tests/app/EntityHeadAnchor.js'):id==='./MapSurface'?{prepareMapSurface(){}}:id==='./EntityMotion'?{EntityMotion}:id==='./EntityAnimation'?{EntityAnimation,movementDisplayDurationMs}:id==='./EntityDisplacement'?{EntityDisplacement}:id==='./CombatEffects'?{CombatEffects:Effects}:id==='./StatusEffects'?{StatusEffects:class{observe(){}update(){}dispose(){}}}:{});
    const viewport=new Node('viewport');viewport.getComponent(UITransform).setContentSize(720,808);const world=new Node('world'),entities=new Node('entities');entities.parent=world;new Node('Effects').parent=world;
    const frames=new Map(),definitions={};
    const layer=package=>({package,actions:Object.fromEntries(['standing','walking','attack','struck','die','casting'].map(action=>[action,{count:6,duration_ms:100,directions:Array.from({length:8},(_,direction)=>({frames:Array.from({length:6},(_,i)=>{const key=`${package}:${action}:${direction}:${i}`;frames.set(key,{key});definitions[key]={width:20,height:80,offset_x:0,offset_y:action==='casting'?-400:action==='die'?-900:-40-direction-i,declared_empty:false};return key;})}))}]))});
    const profile={layers:[layer('Hum'),layer('Weapon')],layer_order_by_direction:{1:['Hum','Weapon']}};frames.set('Objects:1',{});definitions['Objects:1']={width:48,height:67,package:'Objects',raw_frame:1127};frames.set('Objects:2',{});definitions['Objects:2']={width:48,height:41,package:'Objects',raw_frame:8898};
    const assets={manifest:{frames:definitions},map:{...geometry,props:[
        {id:'brazier',x:8,y:9,display_x:8,display_y:9,group_id:'brazier_group',part_index:0,frame:'Objects:1',anchor_x:.5,anchor_y:0,pixel_offset_x:0,pixel_offset_y:0},
        {id:'sign',x:9,y:9,display_x:9,display_y:9,group_id:'sign_group',part_index:0,frame:'Objects:2',anchor_x:.5,anchor_y:0,pixel_offset_x:0,pixel_offset_y:0},
    ]},gameplay:{monster_profiles:[{definition_id:1,name:'稻草人'}]},frames,prefab:'entity',propPrefab:'prop',profile:()=>profile};
    const view=new GameWorldView(viewport,world,entities,'角色',0,'1',1);view.prepare(assets,()=>{});
    const hp={isZero:()=>false,lessThan:()=>false,toString:()=> '100'};
    const state=(handle,kind,x,y)=>({entityHandle:handle,entityKind:kind,definitionId:kind===3?1:0,position:{x,y},direction:0,moveIntervalMs:1000,hp,hpMax:hp,life:'0',action:0,attackSequence:0,castSequence:0});
    view.apply({entities:[state(1,1,8,8),state(3,3,7,7)]});view.update(0,0,true);
    const player=view.visuals.get(1);assert.match(player.body[0].spriteFrame.key,/standing:1:/);assert.match(player.body[1].spriteFrame.key,/standing:1:/);
    assert.equal(entities.children[0],view.visuals.get(3).node);assert.equal(entities.children[1],player.node);assert.equal(entities.children[2].name,'brazier');assert.equal(entities.children[3].name,'sign');
    assert.equal(entities.children[2].position.y,8);assert.equal(entities.children[3].position.y,-8);assert.equal(player.node.position.y,24);
    const labelY=player.node.getChildByName('Name').position.y,hpY=player.node.getChildByName('HP').position.y;
    assert.equal(labelY,98);assert.equal(hpY,78);assert.equal(view.visuals.get(3).node.getChildByName('Name').getComponent(Label).string,'稻草人');assert.equal(view.visuals.get(3).node.getChildByName('Name').active,true);
    view.acceptAttack({sceneEpoch:'1',eventId:'1',casterHandle:1,casterLife:'0',attackSequence:1,direction:0});view.update(0,0,true);
    assert.deepEqual(effects.sample(1),{attackSequence:1,frame:0,direction:1});assert.match(player.body[0].spriteFrame.key,/attack:1:0/);
    for(let direction=0;direction<8;direction++){view.apply({entities:[{...state(1,1,8,8),direction,attackSequence:1,castSequence:direction+1,action:5},state(3,3,7,7)]});assert.equal(player.node.getChildByName('Name').position.y,labelY);assert.equal(player.node.getChildByName('HP').position.y,hpY);}
    // 走真正的视图接入，确认两个AI节奏没有被重新作为整步显示时长传入。
    cc.game.totalTime=1000;view.apply({entities:[{...state(3,3,7,7),moveIntervalMs:2000},state(2,2,7,8)]});
    cc.game.totalTime=1200;view.apply({entities:[{...state(3,3,8,7),moveIntervalMs:2000},state(2,2,8,8)]});
    cc.game.totalTime=1500;view.update(0,0,true);
    for(const [handle,y]of [[3,7],[2,8]]){const visual=view.visuals.get(handle),midpoint=projection.project(7.5,y);assert.deepEqual(visual.node.position,new Vec3(midpoint.x,midpoint.y));assert.equal(visual.pose.action,'walking');assert.equal(visual.pose.frame,3);}
    cc.game.totalTime=1800;view.update(0,0,true);
    for(const [handle,y]of [[3,7],[2,8]]){const visual=view.visuals.get(handle),endpoint=projection.project(8,y);assert.deepEqual(visual.node.position,new Vec3(endpoint.x,endpoint.y));assert.equal(visual.pose.action,'standing');}
});
test('真实疾光渲染使用保存Mask和原图尺寸，固定原点的六格端点只裁切不缩放',()=>{
    const vectors=[[0,32],[48,32],[48,0],[48,-32],[0,-32],[-48,-32],[-48,0],[-48,32]],instances=[];
    cc.instantiate=()=>{const root=new Node('Beam'),sprite=new Node('Sprite');sprite.add(Sprite,new Sprite());sprite.parent=root;instances.push(root);return root;};
    const {CombatEffects}=load(path.join(source,'CombatEffects.ts'),id=>id==='cc'?cc:id==='./CastPlayback'?playback:id==='./SkillModel'?models:id==='./BeamGeometry'?{beamGeometry}:id==='./WorldAssets'?{effectFrameKeys:value=>Object.values(value).flatMap(part=>part?.directions?part.directions.flat():[])}:{});
    const keys=Array.from({length:8},(_,i)=>`Magic:${970+i*20}`),native={width:400,height:220,offset_x:-20,offset_y:-160,declared_empty:false};
    const assets={gameplay:{skills:[{id:10,kind:'lightning',active:true,target_kind:'enemy_direction',line_cells:6,cast_ms:600}]},manifest:{effects:{10:{beam:{duration_ms:600,directions:keys.map(key=>[key])},beam_anchor:'origin',beam_native_direction_vectors:vectors}},frames:Object.fromEntries(keys.map(key=>[key,native]))},frames:new Map(keys.map(key=>[key,{}])),beamPrefab:{}};
    const effects=new CombatEffects(assets,new Node('effects'),'1',(x,y)=>{const p=projection.project(x,y);return new Vec3(p.x,p.y);},()=>new Vec3(999,999),()=> '0',()=>null,projection);
    effects.accept({sceneEpoch:'1',castId:'1',skillId:10,casterHandle:1,casterLife:'0',origin:{x:8,y:8},targetGrid:{x:8,y:8},direction:0,delayMs:500},0);effects.update(600);
    const root=instances[0],sprite=root.getChildByName('Sprite');assert.deepEqual(root.position,new Vec3(0,24));assert(Math.abs(root.getComponent(UITransform).width-Math.hypot(192,96))<1e-8);
    assert.equal(sprite.getComponent(UITransform).width,400);assert.equal(sprite.getComponent(UITransform).height,220);assert.deepEqual(root.scale,new Vec3(1,1,1));assert.deepEqual(sprite.scale,new Vec3(1,1,1));
    assert(Math.abs(root.angle+sprite.angle+7.1250163489)<1e-7);
});

test('真实常驻盾只用保存AttachedEffects与原图，完整基线恢复常驻而不补起手，空状态清除',()=>{
    const {StatusEffects}=load(path.join(source,'StatusEffects.ts'),id=>id==='cc'?cc:id==='./StatusPlayback'?require('../../../.work/ui-tests/app/StatusPlayback.js'):{});
    const instances=[];cc.instantiate=()=>{const node=new Node('Effect');node.add(Sprite,new Sprite());instances.push(node);return node;};
    const parent=new Node('AttachedEffects'),native={width:92,height:110,offset_x:-21,offset_y:-78,declared_empty:false};
    const keys=['Magic:3890','Magic:3891','Magic:3892','Magic:3900','Magic:3901','Magic:3902'];
    const assets={effectPrefab:{},manifest:{statuses:{31:{idle:{frames:keys.slice(0,3),duration_ms:600},struck:{frames:keys.slice(3),duration_ms:600}}},frames:Object.fromEntries(keys.map(key=>[key,native]))},frames:new Map(keys.map(key=>[key,{key}]))};
    const effects=new StatusEffects(assets,'1',(_id,life)=>life==='0'?parent:null),status={statusId:31,remainingMs:0};
    const entity=(statuses,hp='100')=>({entityHandle:1,entityKind:1,life:'0',hp,statuses});
    effects.observe('1',[entity([status])],0);effects.update(0);assert.equal(instances.length,1);const n=instances[0];assert.equal(n.parent,parent);assert.equal(n.getComponent(Sprite).spriteFrame.key,'Magic:3890');assert.deepEqual(n.position,new Vec3(-45,94));assert.equal(n.getComponent(UITransform).width,92);assert.deepEqual(n.scale,new Vec3(1,1,1));
    effects.update(200);assert.equal(n.getComponent(Sprite).spriteFrame.key,'Magic:3891');effects.observe('2',[entity([status],'90')],250);effects.update(250);assert.equal(n.getComponent(Sprite).spriteFrame.key,'Magic:3900');effects.update(850);assert.match(n.getComponent(Sprite).spriteFrame.key,/389/);
    effects.observe('3',[entity([],'90')],900);effects.update(900);assert.equal(n.destroyed,true);assert.equal(effects.nodes.size,0);
    effects.dispose();
});

test('真实预载会为无该职业技能的角色加载常驻状态六帧与保存徽标，退场统一释放原引用',async()=>{
    class Asset{constructor(json){this.json=json;this.refs=0;}addRef(){this.refs++;}decRef(){this.refs--;}}
    class JsonAsset extends Asset{}class Prefab extends Asset{}class SpriteFrame extends Asset{}
    const map={...geometry,surface_width:1088,surface_height:544,world_height:592,background_offset_y:24,props:[]};
    const keys=[3890,3891,3892,3900,3901,3902].map(id=>`Magic:${id}`),manifest={map:{...map},profiles:{player:{layers:[]}},effects:{},statuses:{31:{idle:{frames:keys.slice(0,3)},struck:{frames:keys.slice(3)}}}},frames={};
    const values={'development/manifest':new JsonAsset(manifest),'development/maps/first-map':new JsonAsset(map),'development/gameplay':new JsonAsset({jobs:[{job_id:0,profile:'player'}],monster_profiles:[],pet_profiles:[],skills:[]})};
    for(const key of keys){const id=key.split(':')[1],png=`frames/Magic/frame_${id.padStart(6,'0')}.png`;frames[key]={png,declared_empty:false};values['development/'+png.replace('.png','')+'/spriteFrame']=new SpriteFrame();}manifest.frames=frames;
    for(const name of ['Entity','Effect','MapProp','Beam','StatusBadge'])values['prefabs/'+name]=new Prefab();values['development/maps/first-map/spriteFrame']=new SpriteFrame();
    const requested=[],ccAssets={Asset,JsonAsset,Prefab,SpriteFrame,resources:{load(key,type,callback){requested.push(key);queueMicrotask(()=>callback(values[key]?null:new Error(key),values[key]));}}};
    const {WorldAssets}=load(path.join(source,'WorldAssets.ts'),id=>id==='cc'?ccAssets:id==='./GridProjection'?{GridProjection}:{});
    const assets=await WorldAssets.prepare(0,()=>{});assert.equal(assets.frames.size,6);assert(requested.includes('prefabs/StatusBadge'));assert(keys.every(key=>assets.frames.has(key)));
    assert(Object.values(values).every(asset=>asset.refs===1));assets.dispose();assert(Object.values(values).every(asset=>asset.refs===0));assert.equal(assets.frames.size,0);assets.dispose();assert(Object.values(values).every(asset=>asset.refs===0));
});
