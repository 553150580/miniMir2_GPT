const assert=require('node:assert/strict');
const {load}=require('../../client/tests/app/helpers/session_harness.cjs');
const {GridProjection}=require('../ui-tests/app/GridProjection.js');
class UITransform{constructor(){this.width=128;this.height=128;}setContentSize(w,h){this.width=w;this.height=h;}setAnchorPoint(){}}
class Sprite{static Type={SIMPLE:0};static SizeMode={CUSTOM:0};}
class Graphics{clear(){}moveTo(){}lineTo(){}close(){}fill(){}stroke(){}}
class Node{static EventType={SIZE_CHANGED:'size'};constructor(name){this.name=name;this.children=[];this.parts=new Map([[UITransform,new UITransform()]]);}add(type,value){value.node=this;this.parts.set(type,value);return value;}getComponent(type){return this.parts.get(type);}getChildByName(name){return this.children.find(c=>c.name===name);}getChildByPath(path){return path.split('/').reduce((node,key)=>node?.getChildByName(key),this);}setPosition(){}off(){}on(){}}
const cc={_decorator:{ccclass:()=>type=>type},Component:class{},Node,Sprite,UITransform,Graphics,Color:class{},isValid:()=>true};
const {WorldAssets}=load('client/assets/scripts/app/WorldAssets.ts',name=>name==='cc'?cc:{});
const {prepareMapSurface}=load('client/assets/scripts/app/MapSurface.ts',name=>name==='cc'?cc:{});
const {GameWorldView}=load('client/assets/scripts/app/GameWorldView.ts',name=>name==='cc'?cc:name==='./GridProjection'?{GridProjection}:name==='./MapSurface'?{prepareMapSurface}:name==='./CombatEffects'?{CombatEffects:class{dispose(){}}}:name==='./StatusEffects'?{StatusEffects:class{dispose(){}}}:{});
const {GameHUD}=load('client/assets/scripts/app/GameHUD.ts',name=>name==='cc'?cc:{});
const geometry={width:17,height:17,tile_width:64,tile_height:32,soil_thickness_px:48,projection:'isometric_diamond'};
function scenario(external=false,rebind=false){
    const sprites=[],released=[];
    const frame={refs:0,originalSize:{width:128,height:128},addRef(){this.refs++;},decRef(){this.refs--;if(!this.refs)for(const [label,sprite]of sprites)if(sprite.spriteFrame===this)released.push(label);}};
    const asset=()=>{const value=new WorldAssets();value.background=value.own(frame);value.map={...geometry,props:[]};value.gameplay={};value.manifest={frames:{}};return value;};
    const world=new Node('World');world.add(Sprite,new Sprite());
    for(const [name,type]of [['Soil',Graphics],['GridLines',Graphics],['Surface',Sprite],['TargetMarker',Sprite]]){const child=new Node(name);world.children.push(child);child.add(type,new type());}
    const diamond=new Node('Diamond');world.getChildByName('TargetMarker').children.push(diamond);diamond.add(Graphics,new Graphics());world.children.push(new Node('Effects'));
    const hudNode=new Node('Map'),hudSurface=new Node('Surface');hudNode.add(Sprite,new Sprite());hudSurface.add(Sprite,new Sprite());const hud=new GameHUD();hud.saved=path=>path.endsWith('/Surface')?hudSurface:hudNode;
    sprites.push(['saved primary World/Surface',world.getChildByName('Surface').getComponent(Sprite)],['saved HUD MiniMap/Surface',hudSurface.getComponent(Sprite)]);
    const oldAssets=asset(),oldView=new GameWorldView(new Node('Viewport'),world,new Node('Entities'),'角色',0,'1',1);hud.setMapFrame(frame,oldAssets);oldView.prepare(oldAssets,()=>{});
    if(rebind){const nextAssets=asset(),nextView=new GameWorldView(new Node('Viewport'),world,new Node('Entities'),'角色',0,'2',1);hud.setMapFrame(frame,nextAssets);nextView.prepare(nextAssets,()=>{});oldAssets.dispose();assert.equal(nextView.disposed,false);for(const [,sprite]of sprites)assert.equal(sprite.spriteFrame,frame);nextView.dispose();}
    else if(external)oldAssets.dispose();else oldView.dispose();
    assert.equal(frame.refs,0);assert.deepEqual(released,[]);for(const [,sprite]of sprites)assert.equal(sprite.spriteFrame,null);
    return {external,rebind,released_while_bound:released};
}
console.log(JSON.stringify({scope:'修后真实prepare/WorldAssets生命周期，HUD按新正式合同传assets；覆盖view.dispose、外部assets.dispose、同Frame新owner后旧回调',cases:[scenario(),scenario(true),scenario(true,true)]}));
