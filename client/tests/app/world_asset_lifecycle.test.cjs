const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs'),path=require('node:path');
const {load,Long,root}=require('./helpers/session_harness.cjs');
const model=name=>require(path.join(root,'.work/ui-tests/app',name+'.js'));
const sprites=[];
class UITransform{constructor(){this.width=128;this.height=128;this.anchorPoint={x:.5,y:.5};}setContentSize(width,height){Object.assign(this,{width,height});}setAnchorPoint(x,y){this.anchorPoint={x,y};}}
class Sprite{static Type={SIMPLE:0};static SizeMode={CUSTOM:0};constructor(){sprites.push(this);}}
class Graphics{clear(){}moveTo(){}lineTo(){}close(){}fill(){}stroke(){}}
class Label{}class ProgressBar{}
class Vec3{constructor(x=0,y=0,z=0){Object.assign(this,{x,y,z});}}
class Node{static EventType={SIZE_CHANGED:'size',TOUCH_END:'touch'};constructor(name){this.name=name;this.children=[];this.position=new Vec3();this.scale=new Vec3(1,1,1);this.parts=new Map();this.add(UITransform,new UITransform());}
 add(type,value){value.node=this;this.parts.set(type,value);return value;}getComponent(type){return this.parts.get(type);}getComponentsInChildren(type){return [this.parts.get(type),...this.children.flatMap(n=>n.getComponentsInChildren(type))].filter(Boolean);}
 getChildByName(name){return this.children.find(c=>c.name===name);}getChildByPath(p){return p.split('/').reduce((n,k)=>n?.getChildByName(k),this);}
 set parent(value){if(this._parent)this._parent.children=this._parent.children.filter(n=>n!==this);this._parent=value;if(value)value.children.push(this);}get parent(){return this._parent;}
 setPosition(x,y,z=0){this.position=typeof x==='object'?x:new Vec3(x,y,z);}setScale(x,y,z){this.scale=new Vec3(x,y,z);}on(){}off(){}destroy(){this.destroyRequested=true;}getSiblingIndex(){return this.parent?.children.indexOf(this)??0;}setSiblingIndex(){}
}
function spriteNode(name){const node=new Node(name);node.add(Sprite,new Sprite());return node;}
const cc={_decorator:{ccclass:()=>value=>value},Component:class{},Node,Sprite,UITransform,Graphics,Label,ProgressBar,Vec3,Color:class{},game:{totalTime:0},isValid:(value,strict)=>!!value&&!value.destroyed&&!(strict&&value.destroyRequested),instantiate(prefab){
 if(prefab!=='entity')return spriteNode('Effect');const node=new Node('Entity'),body=spriteNode('Body'),name=new Node('Name'),hp=new Node('HP');body.parent=node;name.add(Label,{});name.parent=node;hp.add(ProgressBar,{});hp.parent=node;new Node('AttachedEffects').parent=node;return node;
}};
const worldAssets=load('client/assets/scripts/app/WorldAssets.ts',id=>id==='cc'?cc:id==='./GridProjection'?model('GridProjection'):{}),{WorldAssets}=worldAssets;
const {CombatEffects}=load('client/assets/scripts/app/CombatEffects.ts',id=>id==='cc'?cc:id==='./WorldAssets'?worldAssets:id==='./CastPlayback'?model('CastPlayback'):id==='./SkillModel'?model('SkillModel'):id==='./BeamGeometry'?model('BeamGeometry'):{});
const {StatusEffects}=load('client/assets/scripts/app/StatusEffects.ts',id=>id==='cc'?cc:id==='./StatusPlayback'?model('StatusPlayback'):{});
const {prepareMapSurface}=load('client/assets/scripts/app/MapSurface.ts',id=>id==='cc'?cc:{});
const {GameWorldView}=load('client/assets/scripts/app/GameWorldView.ts',id=>id==='cc'?cc:id==='./GridProjection'?model('GridProjection'):id==='./MapSurface'?{prepareMapSurface}:id==='./CombatEffects'?{CombatEffects}:id==='./StatusEffects'?{StatusEffects}:id==='./EntityMotion'?load('client/assets/scripts/app/EntityMotion.ts',()=>({})):id==='./EntityDisplacement'?load('client/assets/scripts/app/EntityDisplacement.ts',()=>({})):id==='./EntityAnimation'?load('client/assets/scripts/app/EntityAnimation.ts',()=>({})):id==='./EntityHeadAnchor'?model('EntityHeadAnchor'):{});
const {GameHUD}=load('client/assets/scripts/app/GameHUD.ts',id=>id==='cc'?cc:{});
function frame(name){return{name,refs:0,violations:[],originalSize:{width:1088,height:544},addRef(){this.refs++;},decRef(){assert(this.refs>0);if(!--this.refs)for(const sprite of sprites)if(sprite.spriteFrame===this)this.violations.push(sprite.node.name);}};}
function assets(background=frame('primary')){const a=new WorldAssets();a.background=a.own(background);a.map={...JSON.parse(fs.readFileSync(path.join(root,'client/assets/resources/development/maps/first-map.json'),'utf8')),props:[]};a.manifest={frames:{effect:{width:32,height:32,offset_x:0,offset_y:0},status:{width:32,height:32,offset_x:0,offset_y:0}},effects:{},statuses:{31:{loop:{frames:['status'],duration_ms:1000}}}};a.gameplay={skills:[],items:[]};a.prefab='entity';a.effectPrefab='effect';a.profile=()=>null;return a;}
function savedWorld(){const viewport=new Node('Viewport'),world=spriteNode('World'),entities=new Node('Entities');entities.parent=world;world.parent=viewport;
 for(const [name,type]of [['Soil',Graphics],['GridLines',Graphics],['Surface',Sprite],['TargetMarker',Sprite]]){const child=new Node(name);child.add(type,new type());child.parent=world;}const diamond=new Node('Diamond');diamond.add(Graphics,new Graphics());diamond.parent=world.getChildByName('TargetMarker');new Node('Effects').parent=world;return{viewport,world,entities};}
function hud(a){const saved=spriteNode('MiniMap'),surface=spriteNode('MiniSurface');surface.parent=saved;const result=new GameHUD();result.saved=p=>p.endsWith('/Surface')?surface:saved;result.setMapFrame(a.background,a);return{hud:result,sprite:surface.getComponent(Sprite)};}
function view(a,saved=savedWorld()){const v=new GameWorldView(saved.viewport,saved.world,saved.entities,'本人',0,'1',1);v.prepare(a,()=>{});return{view:v,...saved,sprite:saved.world.getChildByName('Surface').getComponent(Sprite)};}
function actor(){return{entityHandle:1,entityKind:1,definitionId:0,position:{x:8,y:8},direction:4,action:0,attackSequence:0,castSequence:0,life:Long.UZERO,hp:Long.fromInt(100),hpMax:Long.fromInt(100),moveIntervalMs:400,movementRevision:Long.UZERO,movementKind:0,statuses:[]};}
function owned(a,key){const f=frame(key);a.own(f);a.frames.set(key,f);return f;}

test('实际primary WorldView.prepare + HUD完整owner + WorldView.dispose：保存双Surface在背景ref0前解绑',()=>{
 const a=assets(),background=a.background,w=view(a),h=hud(a),savedSurface=w.world.getChildByName('Surface');assert.equal(w.sprite.spriteFrame,background);assert.equal(h.sprite.spriteFrame,background);
 w.view.dispose();assert.equal(background.refs,0);assert.deepEqual(background.violations,[]);assert.equal(w.sprite.spriteFrame,null);assert.equal(h.sprite.spriteFrame,null);assert.equal(w.world.getChildByName('Surface'),savedSurface);assert.equal(savedSurface.destroyRequested,undefined);h.hud.onDestroy();
});
test('实际WorldAssets直接释放先解绑保存地板/HUD、活body/skill/status，延迟destroy仍无绑定ref0',()=>{
 const a=assets(),background=a.background,w=view(a),h=hud(a),body=owned(a,'body'),effect=owned(a,'effect'),status=owned(a,'status');w.view.apply({stateSequence:Long.UONE,entities:[actor()]});w.view.visuals.get(1).body[0].spriteFrame=body;
 w.view.effects.draw('skill','effect',new Vec3());w.view.statusEffects.playback.poses=()=>[{entityHandle:1,life:'0',part:'loop',elapsed:0,key:'status'}];w.view.statusEffects.update(0);
 const bound=sprites.filter(s=>[body,effect,status].includes(s.spriteFrame));assert.equal(bound.length,3);a.dispose();assert.equal(w.view.isMapReady,false);assert.equal(w.view.disposed,true);
 for(const f of [background,body,effect,status]){assert.equal(f.refs,0);assert.deepEqual(f.violations,[]);}for(const s of bound){assert.equal(s.spriteFrame,null);assert(s.node.destroyRequested||s.node.parent?.destroyRequested);}assert.equal(h.sprite.spriteFrame,null);w.view.dispose();
});
test('实际apply实体移除、skill/status update过期先同步清帧，已从索引删除的待销毁节点不会漏',()=>{
 const a=assets(),w=view(a),body=owned(a,'body'),effect=owned(a,'effect'),status=owned(a,'status');w.view.apply({stateSequence:Long.UONE,entities:[actor()]});const retired=w.view.visuals.get(1);retired.body[0].spriteFrame=body;
 w.view.effects.draw('skill','effect',new Vec3());const skill=w.view.effects.nodes.get('skill');w.view.effects.update(0);assert.equal(skill.sprite.spriteFrame,null);assert.equal(skill.node.destroyRequested,true);
 w.view.statusEffects.playback.poses=()=>[{entityHandle:1,life:'0',part:'loop',elapsed:0,key:'status'}];w.view.statusEffects.update(0);const attached=w.view.statusEffects.nodes.get('status');w.view.statusEffects.playback.poses=()=>[];w.view.statusEffects.update(1);assert.equal(attached.sprite.spriteFrame,null);
 w.view.apply({stateSequence:Long.fromInt(2,true),entities:[]});assert.equal(retired.body[0].spriteFrame,null);assert.equal(retired.node.destroyRequested,true);assert.equal(w.view.visuals.size,0);w.view.dispose();for(const f of [body,effect,status])assert.deepEqual(f.violations,[]);
});
test('共享同一Frame的新owner在相同保存节点挂接后，旧Surface/HUD失效回调不能清新owner',()=>{
 const shared=frame('shared'),a=assets(shared),b=assets(shared),saved=savedWorld();view(a,saved);const h=hud(a);
 const late=Array.from(a.beforeInvalidation),current=view(b,saved);h.hud.setMapFrame(shared,b);for(const notify of late)notify('dispose');assert.equal(current.sprite.spriteFrame,shared);assert.equal(h.sprite.spriteFrame,shared);
 a.dispose();assert.equal(shared.refs,1);assert.equal(current.sprite.spriteFrame,shared);assert.equal(h.sprite.spriteFrame,shared);current.view.dispose();assert.equal(shared.refs,0);assert.deepEqual(shared.violations,[]);assert.equal(h.sprite.spriteFrame,null);
});
test('失效通知即使某个消费者失败仍通知其它人且保留资源；重入释放拒绝，移除故障后可重试',()=>{
 const a=assets(),background=a.background,seen=[];const fail=a.onBeforeInvalidation(()=>{seen.push('fail');throw Error('解绑失败');});a.onBeforeInvalidation(()=>seen.push('next'));
 assert.throws(()=>a.dispose(),/解绑失败/);assert.equal(background.refs,1);assert.deepEqual(seen,['fail','next']);fail();const reentrant=a.onBeforeInvalidation(()=>a.dispose());assert.throws(()=>a.dispose(),/不能重入/);assert.equal(background.refs,1);reentrant();a.dispose();assert.equal(background.refs,0);
});
test('HUD先销毁会同步解绑再退订，不依赖下一帧或世界owner通知',()=>{
 const a=assets(),background=a.background,h=hud(a);h.hud.onDestroy();assert.equal(h.sprite.spriteFrame,null);a.dispose();assert.equal(background.refs,0);assert.deepEqual(background.violations,[]);
});

test('实际GameCombatStatus退役徽标先清根及子Sprite，WorldAssets归还保存prefab依赖前无残留绑定',()=>{
 const {GameCombatStatus}=load('client/assets/scripts/app/GameCombatStatus.ts',id=>id==='cc'?{...cc,Button:{EventType:{CLICK:'click'}}}:{}),a=assets(),texture=owned(a,'badge'),badge=spriteNode('StatusBadge'),child=spriteNode('BadgeIcon');child.parent=badge;badge.getComponent(Sprite).spriteFrame=texture;child.getComponent(Sprite).spriteFrame=texture;
 const controller=Object.create(GameCombatStatus.prototype);controller.badges=new Map([[31,{node:badge,handler:()=>{}}]]);a.onBeforeInvalidation(()=>controller.dispose());a.dispose();assert.equal(texture.refs,0);assert.deepEqual(texture.violations,[]);assert.equal(badge.destroyRequested,true);assert.equal(child.getComponent(Sprite).spriteFrame,null);
});
