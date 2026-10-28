const fs=require('node:fs'),path=require('node:path'),{createRequire}=require('node:module');
const ts=require('../../../tools/client/creator_paths.cjs').typescript;
const source=path.resolve(__dirname,'../../assets/scripts/app');
const Long=createRequire(path.join(source,'GameWorldView.ts'))('long');
class Vec3{constructor(x=0,y=0,z=0){Object.assign(this,{x,y,z});}}
class UITransform{constructor(){this.width=64;this.height=96;this.anchorPoint={x:.5,y:.5};}setContentSize(width,height){Object.assign(this,{width,height});}setAnchorPoint(x,y){this.anchorPoint={x,y};}convertToNodeSpaceAR(point){return point;}}
class Sprite{static Type={SIMPLE:0};static SizeMode={CUSTOM:0};}class Label{}class ProgressBar{}
class Node{
 static EventType={SIZE_CHANGED:'size',TOUCH_END:'touch'};
 constructor(name){this.name=name;this.children=[];this.parts=new Map();this.position=new Vec3();this.scale=new Vec3(1,1,1);this.active=true;this.add(UITransform,new UITransform());}
 add(type,value){value.node=this;this.parts.set(type,value);return value;}getComponent(type){return this.parts.get(type);}
 getChildByName(name){return this.children.find(node=>node.name===name);}getChildByPath(name){return name.split('/').reduce((node,key)=>node?.getChildByName(key),this);}
 set parent(value){if(this._parent)this._parent.children=this._parent.children.filter(node=>node!==this);this._parent=value;if(value)value.children.push(this);}get parent(){return this._parent;}
 setPosition(x,y,z=0){this.position=typeof x==='object'?new Vec3(x.x,x.y,x.z):new Vec3(x,y,z);}setSiblingIndex(i){const children=this.parent.children;children.splice(children.indexOf(this),1);children.splice(i,0,this);}getSiblingIndex(){return this.parent.children.indexOf(this);}
 on(){}off(){}destroy(){this.destroyed=true;this.parent=null;}
}
const sprite=name=>{const node=new Node(name);node.add(Sprite,new Sprite());return node;};
const cc={Node,UITransform,Sprite,Label,ProgressBar,Vec3,isValid:node=>!node?.destroyed,game:{totalTime:0},instantiate(prefab){
 if(prefab instanceof Node)return sprite(prefab.name);if(prefab!=='entity')return sprite(prefab);
 const node=new Node('Entity'),body=sprite('Body'),name=new Node('Name'),hp=new Node('HP');body.parent=node;new Node('AttachedEffects').parent=node;
 name.add(Label,{string:''});name.setPosition(0,78);name.scale=new Vec3(.7,.7,1);name.parent=node;hp.add(ProgressBar,{});hp.setPosition(0,58);hp.parent=node;return node;
}};
const cache=new Map();
function load(name){
 if(cache.has(name))return cache.get(name).exports;
 const file=path.join(source,name+'.ts'),result=ts.transpileModule(fs.readFileSync(file,'utf8'),{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,esModuleInterop:true}}),module={exports:{}};cache.set(name,module);
 new Function('require','module','exports',result.outputText)(id=>id==='cc'?cc:id==='./MapSurface'?{prepareMapSurface(){}}:id.startsWith('./')?load(id.slice(2)):createRequire(file)(id),module,module.exports);return module.exports;
}
const {GameWorldView}=load('GameWorldView'),{WorldAssets}=load('WorldAssets');
function makeWorld(ownerHandle){
 cc.game.totalTime=0;const root=path.resolve(__dirname,'../../assets/resources/development'),assets=new WorldAssets();
 assets.manifest=JSON.parse(fs.readFileSync(path.join(root,'manifest.json'),'utf8'));assets.map=JSON.parse(fs.readFileSync(path.join(root,'maps/first-map.json'),'utf8'));assets.gameplay=JSON.parse(fs.readFileSync(path.join(root,'gameplay.json'),'utf8'));
 for(const [key,frame]of Object.entries(assets.manifest.frames))if(!frame.declared_empty)assets.frames.set(key,{key});
 Object.assign(assets,{prefab:'entity',effectPrefab:'effect',propPrefab:'prop',beamPrefab:'beam'});
 assets.profile=state=>assets.manifest.profiles[state.entityKind===1?'player_male':state.entityKind===2?'pet_skeleton':'monster_scarecrow'];
 const viewport=new Node('Viewport'),world=new Node('World'),entities=new Node('Entities');viewport.getComponent(UITransform).setContentSize(720,808);world.getComponent(UITransform).setContentSize(1088,592);entities.parent=world;new Node('Effects').parent=world;
 const view=new GameWorldView(viewport,world,entities,'位移验收',0,Long.UONE,ownerHandle);view.prepare(assets,()=>{});
 let sequence=0;return{view,assets,entities,cc,apply(states,now=cc.game.totalTime){cc.game.totalTime=now;view.apply({stateSequence:Long.fromInt(++sequence,true),entities:states});},update(now){cc.game.totalTime=now;view.update(0,0,true);}};
}
function state(handle=1,kind=1,x=8,y=8,changes={}){return{entityHandle:handle,entityKind:kind,definitionId:kind===3?1:0,position:{x,y},direction:2,moveIntervalMs:kind===1?400:2000,attackSequence:0,castSequence:0,action:0,life:Long.UZERO,movementRevision:Long.UZERO,movementKind:0,hp:Long.fromInt(100),hpMax:Long.fromInt(100),mp:Long.fromInt(100),mpMax:Long.fromInt(100),statuses:[],...changes};}
function event(changes={}){return{sceneEpoch:Long.UONE,entityHandle:1,entityLife:Long.UZERO,movementRevision:Long.UONE,kind:2,origin:{x:8,y:8},path:[{x:9,y:8}],facingDirection:6,movementDirection:2,stepDurationMs:300,sourceSkillId:8,castId:Long.UZERO,attackSequence:0,castSequence:0,...changes};}
module.exports={makeWorld,state,event,Long,Sprite,UITransform,Vec3,cc};
