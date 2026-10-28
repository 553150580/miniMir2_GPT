const assert=require('node:assert/strict');
const {load}=require('../../client/tests/app/helpers/session_harness.cjs');
class UITransform{setContentSize(){}setAnchorPoint(){}}
class Sprite{static Type={SIMPLE:0};static SizeMode={CUSTOM:0};}
class Node{static EventType={SIZE_CHANGED:'size'};constructor(name){this.name=name;this.destroyRequested=false;this.parts=new Map([[UITransform,new UITransform()]]);}getComponent(type){return this.parts.get(type);}off(){}setPosition(){}destroy(){this.destroyRequested=true;}addSprite(){const s=new Sprite();s.node=this;this.parts.set(Sprite,s);return s;}}
class Vec3{constructor(x=0,y=0,z=0){Object.assign(this,{x,y,z});}}
const cc={Node,Sprite,UITransform,Vec3,game:{totalTime:0},isValid:()=>true,instantiate:()=>{const n=new Node('Effect');n.addSprite();return n;}};
const {WorldAssets}=load('client/assets/scripts/app/WorldAssets.ts',id=>id==='cc'?cc:{});
const {GameWorldView}=load('client/assets/scripts/app/GameWorldView.ts',id=>id==='cc'?cc:{});
const {CombatEffects}=load('client/assets/scripts/app/CombatEffects.ts',id=>id==='cc'?cc:id==='./CastPlayback'?require('../../.work/ui-tests/app/CastPlayback.js'):{});
const {StatusEffects}=load('client/assets/scripts/app/StatusEffects.ts',id=>id==='cc'?cc:id==='./StatusPlayback'?require('../../.work/ui-tests/app/StatusPlayback.js'):{});
const assets=new WorldAssets(),sprites=[],released=[];
function frame(name){const f={name,refs:0,addRef(){this.refs++;},decRef(){this.refs--;if(!this.refs)for(const [label,sprite]of sprites)if(sprite.spriteFrame===this)released.push(label);}};assets.own(f);return f;}
function visual(name){const node=new Node(name),sprite=node.addSprite();sprite.spriteFrame=frame(name);sprites.push([name,sprite]);return {node,sprite};}
assets.manifest={effects:{},frames:{effect:{width:32,height:32,offset_x:0,offset_y:0},status:{width:32,height:32,offset_x:0,offset_y:0}},statuses:{31:{loop:{frames:['status'],duration_ms:1000}}}};
assets.gameplay={skills:[]};assets.frames.set('effect',frame('skill effect'));assets.frames.set('status',frame('attached status'));
const effects=new CombatEffects(assets,new Node('Effects'),'1',()=>new Vec3(),()=>null,()=>null);
effects.draw('active','effect',new Vec3());sprites.push(['active skill Sprite',effects.nodes.get('active').sprite]);
const statuses=new StatusEffects(assets,'1',()=>new Node('AttachedEffects'));
statuses.playback.poses=()=>[{entityHandle:1,life:'0',part:'loop',elapsed:0,key:'status'}];statuses.update(0);sprites.push(['active attached status Sprite',statuses.nodes.get('status').sprite]);
const body=visual('active actor appearance'),prop=visual('active prop'),retired=visual('actor retired earlier this frame');
const world=Object.create(GameWorldView.prototype);Object.assign(world,{disposed:false,mapReady:true,viewport:new Node('Viewport'),visuals:new Map([[2,{node:retired.node,body:[retired.sprite]}]]),props:[{node:prop.node}],assets,update(){}});
world.apply({entities:[]});assert.equal(world.visuals.size,0);assert.equal(retired.node.destroyRequested,true);
world.visuals.set(1,{node:body.node,body:[body.sprite]});world.effects=effects;world.statusEffects=statuses;
world.dispose();
assert.equal(body.sprite.spriteFrame,null);assert.equal(prop.sprite.spriteFrame,null);
assert.deepEqual(released,['active skill Sprite','active attached status Sprite','actor retired earlier this frame']);
console.log(JSON.stringify({scope:'真实CombatEffects.draw/dispose、StatusEffects.update/dispose、GameWorldView.dispose、WorldAssets.dispose；Node.destroy按Creator延迟销毁保留Sprite字段',released_while_bound:released,positive_controls:['active actor appearance解绑','active prop解绑']}));
