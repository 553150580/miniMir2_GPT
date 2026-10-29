const {test}=require('node:test');
const assert=require('node:assert/strict');
const {makeWorld,state,event,Long,Sprite,UITransform,cc}=require('../support/world_runtime.cjs');
// 使用真实GameWorldView/WorldAssets/动画/位移/状态实现；仅Cocos节点是轻量替身。
cc.Node.prototype.on=function(type,callback){(this.listeners??=new Map()).set(type,callback);};
function fixture(){
 const f=makeWorld(1),skeleton=f.assets.manifest.profiles.pet_skeleton,second=f.assets.manifest.profiles.player_female;
 let current=skeleton;f.assets.profile=value=>value.entityKind===1?f.assets.manifest.profiles.player_male:current;
 f.assets.gameplay.pet_profiles.push({definition_id:3,job_id:0,name:'新宠物候选'},{definition_id:2,job_id:0,name:'骷髅候选'});
 return Object.assign(f,{skeleton,second,setProfile(value){current=value;},pet(changes={}){return state(2,2,8,8,{definitionId:2,...changes});}});
}
function assertRetired(visual){assert.equal(visual.node.destroyed,true);assert.equal(visual.node.parent,null);assert.equal(visual.node.active,false);for(const body of visual.body){assert.equal(body.spriteFrame,null);assert.equal(body.customMaterial,null);}}

test('同handle定义变化即使映射同profile也退役旧visual，权威位置与原导航标记保留',()=>{
 const f=fixture();f.view.showAim=()=>{throw Error('外观变化不得接管导航');};f.apply([f.pet()]);const previous=f.view.visuals.get(2),surface=f.view.world.children.slice();
 f.apply([f.pet({definitionId:3,position:{x:9,y:8},movementRevision:Long.UONE,movementKind:1})],100);
 const current=f.view.visuals.get(2);assert.notEqual(current,previous);assert.equal(current.profile,f.skeleton);assertRetired(previous);
 assert.deepEqual(current.state.position,{x:9,y:8});assert.equal(current.node.getChildByName('Name').getComponent([...current.node.getChildByName('Name').parts.keys()].find(type=>type.name==='Label')).string,'新宠物候选');
 assert.deepEqual(f.view.world.children,surface);assert.equal(f.entities.children.filter(node=>node.name==='Entity').length,1);f.view.dispose();
});

test('同handle实际profile变化重建层数头顶与新动作，不沿用旧骷髅帧',()=>{
 const f=fixture();f.apply([f.pet()]);const previous=f.view.visuals.get(2),oldName=previous.node.getChildByName('Name').position.y;
 f.setProfile(f.second);f.apply([f.pet({attackSequence:1,action:2})],100);f.update(100);const current=f.view.visuals.get(2);
 assert.notEqual(current,previous);assert.equal(current.profile,f.second);assert.equal(current.body.length,3);assert.notEqual(current.node.getChildByName('Name').position.y,oldName);assertRetired(previous);
 assert.equal(current.pose.action,'attack');for(const [index,sprite]of current.body.entries()){const layer=f.second.layers[index];assert(layer.actions.attack.directions.some(direction=>direction.frames.includes(sprite.spriteFrame.key)));}
 f.view.dispose();
});

test('新Life且新profile从新落点恢复，旧Life攻击/位移不能进入新visual',()=>{
 const f=fixture();f.apply([f.pet({action:2,attackSequence:8})]);const previous=f.view.visuals.get(2);f.setProfile(f.second);
 f.apply([f.pet({definitionId:3,life:Long.UONE,position:{x:11,y:9},attackSequence:0,movementRevision:Long.UZERO})],100);const current=f.view.visuals.get(2),position={...current.node.position};
 assert.notEqual(current,previous);assert.equal(current.profile,f.second);assert.equal(current.pose.action,'standing');assertRetired(previous);
 f.view.acceptAttack({sceneEpoch:Long.UONE,eventId:Long.UONE,casterHandle:2,casterLife:Long.UZERO,attackSequence:99,direction:6});
 f.view.acceptDisplacement(event({entityHandle:2,entityLife:Long.UZERO,origin:{x:11,y:9},path:[{x:12,y:9}],movementRevision:Long.fromInt(99,true)}));f.update(200);
 assert.equal(current.pose.action,'standing');assert.deepEqual({...current.node.position},position);assert.deepEqual(current.state.position,{x:11,y:9});f.view.dispose();
});

test('同profile新Life沿用节点但重置旧倒地与运动，保持既有复生语义',()=>{
 const f=fixture();f.apply([f.pet({hp:Long.UZERO})]);const previous=f.view.visuals.get(2),animation=previous.animation;assert.equal(previous.pose.action,'die');
 f.apply([f.pet({life:Long.UONE,position:{x:10,y:10}})],100);const current=f.view.visuals.get(2);assert.equal(current,previous);assert.notEqual(current.animation,animation);assert.equal(current.pose.action,'standing');
 assert.equal(current.node.destroyed,undefined);assert.deepEqual(current.motion.sample(100).x,10);assert.deepEqual(current.motion.sample(100).y,10);f.view.dispose();
});

test('同Life换profile保留位移高水位、附体节点和播放时钟，不重播旧攻击',()=>{
 const f=fixture();f.assets.manifest.statuses['31']={idle:{frames:['retained-status'],duration_ms:1000}};f.assets.manifest.frames['retained-status']={width:32,height:32,offset_x:0,offset_y:0};f.assets.frames.set('retained-status',{key:'retained-status'});
 const initial=f.pet({attackSequence:9,action:2,movementRevision:Long.fromInt(5,true),statuses:[{statusId:31}]});f.apply([initial]);f.update(900);const previous=f.view.visuals.get(2),movement=previous.movement,status=Array.from(f.view.statusEffects.nodes.values())[0],started=Array.from(f.view.statusEffects.playback.values.values())[0].started;
 assert.equal(previous.pose.action,'standing');f.setProfile(f.second);f.apply([initial],1000);const current=f.view.visuals.get(2);
 assert.notEqual(current,previous);assert.equal(current.movement,movement);assert.equal(current.pose.action,'standing');assertRetired(previous);
 assert.equal(Array.from(f.view.statusEffects.nodes.values())[0],status);assert.equal(status.node.parent,current.node.getChildByName('AttachedEffects'));assert.equal(status.node.destroyed,undefined);assert.equal(status.sprite.spriteFrame.key,'retained-status');assert.equal(Array.from(f.view.statusEffects.playback.values.values())[0].started,started);
 f.apply([f.pet({attackSequence:9,movementRevision:Long.fromInt(4,true),position:{x:1,y:1},statuses:[{statusId:31}]})],1100);assert.deepEqual(current.state.position,{x:8,y:8});f.view.dispose();assert.equal(status.sprite.spriteFrame,null);
});

test('替换新Life在旧父节点destroy前同步解绑附体帧，旧触屏回调不能选中新visual',()=>{
 const f=fixture(),selected=[];f.view.selected=value=>selected.push(value);f.assets.manifest.statuses['31']={idle:{frames:['retired-status'],duration_ms:1000}};f.assets.manifest.frames['retired-status']={width:32,height:32,offset_x:0,offset_y:0};f.assets.frames.set('retired-status',{key:'retired-status'});
 f.apply([f.pet({statuses:[{statusId:31}]})]);const previous=f.view.visuals.get(2),status=Array.from(f.view.statusEffects.nodes.values())[0],destroy=previous.node.destroy.bind(previous.node);let frameAtDestroy='not observed';previous.node.destroy=()=>{frameAtDestroy=status.sprite.spriteFrame;destroy();};
 f.setProfile(f.second);f.apply([f.pet({definitionId:3,life:Long.UONE})],100);const current=f.view.visuals.get(2);assert.equal(frameAtDestroy,null);assertRetired(previous);
 previous.node.listeners.get(cc.Node.EventType.TOUCH_END)({});assert.equal(selected.length,0);current.node.listeners.get(cc.Node.EventType.TOUCH_END)({});assert.equal(selected.length,1);assert.equal(selected[0],current.state);f.view.dispose();
});

test('真实WorldAssets共享帧仅由世界owner归还，旧外观与新外观在ref0前均已解绑',()=>{
 const f=fixture();f.apply([f.pet()]);const previous=f.view.visuals.get(2),key=previous.body[0].spriteFrame.key,watched=[];
 function owned(name){const value={key:name,refs:0,violations:[],addRef(){this.refs++;},decRef(){assert(this.refs>0);if(!--this.refs)for(const sprite of watched)if(sprite.spriteFrame===this)this.violations.push(sprite.node.name);}};f.assets.own(value);return value;}
 const oldFrame=owned(key);f.assets.frames.set(key,oldFrame);previous.body[0].spriteFrame=oldFrame;watched.push(...previous.body);
 f.setProfile(f.second);f.apply([f.pet({definitionId:3})],100);const current=f.view.visuals.get(2),newKey=current.body[0].spriteFrame.key,newFrame=owned(newKey);f.assets.frames.set(newKey,newFrame);current.body[0].spriteFrame=newFrame;watched.push(...current.body);
 assert.equal(oldFrame.refs,1);assert.equal(newFrame.refs,1);assert.equal(previous.body[0].spriteFrame,null);f.view.dispose();for(const frame of [oldFrame,newFrame]){assert.equal(frame.refs,0);assert.deepEqual(frame.violations,[]);}
});


test('128具不同handle的NoMaster尸体与新活宠真实视图共存，初见尸体终帧且逐个退役同步清帧',()=>{
 const f=fixture(),corpses=Array.from({length:128},(_,i)=>f.pet({entityHandle:100+i,hp:Long.ZERO,isPetCorpse:true,life:Long.fromInt(i+1,true)})),live=f.pet({entityHandle:300,life:Long.fromInt(129,true)});f.apply([...corpses,live]);assert.equal(f.view.visuals.size,129);
 for(const corpse of corpses){const visual=f.view.visuals.get(corpse.entityHandle);assert.equal(visual.pose.action,'die');assert.equal(visual.pose.frame,f.skeleton.layers[0].actions.die.count-1);}const previous=f.view.visuals.get(100),active=f.view.visuals.get(300);assert.equal(active.pose.action,'standing');
 f.apply([live],100);assert.equal(f.view.visuals.size,1);assert.equal(f.view.visuals.get(300),active);assertRetired(previous);f.view.dispose();assertRetired(active);
});
