const assert=require('node:assert/strict');
const {load}=require('../../client/tests/app/helpers/session_harness.cjs');
class UITransform{constructor(){this.width=128;this.height=128;}setContentSize(w,h){this.width=w;this.height=h;}}
class Sprite{static Type={SIMPLE:0};static SizeMode={CUSTOM:0};}
class Graphics{clear(){}moveTo(){}lineTo(){}close(){}fill(){}stroke(){}}
class Node{static EventType={SIZE_CHANGED:'size'};constructor(name){this.name=name;this.children=[];this.parts=new Map([[UITransform,new UITransform()]]);}add(type,value){value.node=this;this.parts.set(type,value);return value;}getComponent(type){return this.parts.get(type);}getChildByName(name){return this.children.find(c=>c.name===name);}getChildByPath(path){return path.split('/').reduce((node,key)=>node?.getChildByName(key),this);}setPosition(){}off(){}}
const decorator=()=>type=>type;
const cc={_decorator:{ccclass:decorator},Component:class{},Node,Sprite,UITransform,Graphics,Color:class{},isValid:()=>true};
const {WorldAssets}=load('client/assets/scripts/app/WorldAssets.ts',name=>name==='cc'?cc:{});
const {GameWorldView}=load('client/assets/scripts/app/GameWorldView.ts',name=>name==='cc'?cc:{});
const {GameHUD}=load('client/assets/scripts/app/GameHUD.ts',name=>name==='cc'?cc:{});
const {prepareMapSurface}=load('client/assets/scripts/app/MapSurface.ts',name=>name==='cc'?cc:{});
const sprites=[],releasedWhileBound=[];
const frame={refCount:0,originalSize:{width:128,height:128},addRef(){this.refCount++;return this;},decRef(){this.refCount--;if(this.refCount===0){for(const [label,sprite]of sprites)if(sprite.spriteFrame===this)releasedWhileBound.push(label);}}};
const assets=new WorldAssets();assets.background=assets.own(frame);
const world=new Node('World');world.add(Sprite,new Sprite());
for(const [name,type]of [['Soil',Graphics],['GridLines',Graphics],['Surface',Sprite],['TargetMarker',Sprite]]){const child=new Node(name);world.children.push(child);child.add(type,new type());}
const diamond=new Node('Diamond');world.getChildByName('TargetMarker').children.push(diamond);diamond.add(Graphics,new Graphics());
const projection={surfaceWidth:1088,worldHeight:592,surfaceHeight:544,floorCenterY:24,geometry:{width:17,height:17,tile_width:64,tile_height:32,soil_thickness_px:48},corners:()=>[{x:0,y:0},{x:1,y:0},{x:1,y:1},{x:0,y:1}],project:()=>({x:0,y:0})};
prepareMapSurface(world,assets,projection);
sprites.push(['saved primary World/Surface',world.getChildByName('Surface').getComponent(Sprite)]);
const hudNode=new Node('Map'),hudSurface=new Node('Surface');hudNode.add(Sprite,new Sprite());hudSurface.add(Sprite,new Sprite());
const hud=new GameHUD();hud.saved=path=>path.endsWith('/Surface')?hudSurface:hudNode;hud.setMapFrame(frame);
sprites.push(['saved HUD MiniMap/Surface',hudSurface.getComponent(Sprite)]);
const old=Object.create(GameWorldView.prototype);Object.assign(old,{disposed:false,mapReady:true,viewport:new Node('Viewport'),world,visuals:new Map(),props:[],assets});
old.dispose();
assert.equal(frame.refCount,0);assert.deepEqual(releasedWhileBound,['saved primary World/Surface','saved HUD MiniMap/Surface']);
console.log(JSON.stringify({scope:'真实prepareMapSurface/GameHUD.setMapFrame/GameWorldView.dispose/WorldAssets.dispose，CC节点与refcount轻量stub',frame_ref_count:frame.refCount,released_while_bound:releasedWhileBound}));
