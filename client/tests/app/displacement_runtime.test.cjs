const {test}=require('node:test');const assert=require('node:assert/strict');
const {makeWorld,state,event,Long,Sprite,UITransform}=require('../support/world_runtime.cjs');
const near=(a,b)=>assert(Math.abs(a-b)<1e-8,`${a} != ${b}`);
function bodyKey(visual){return visual.body[0].spriteFrame.key;}

test('真实View消费Pushed原倒序帧和独立面向，同水位HP/状态基线不重启路径',()=>{
 const f=makeWorld(1);f.apply([state()]);const visual=f.view.visuals.get(1),nameY=visual.node.getChildByName('Name').position.y,hpY=visual.node.getChildByName('HP').position.y;
 f.view.acceptDisplacement(event());const first=visual.motion.sample(0);assert.equal(visual.pose.frame,5);assert.equal(visual.pose.direction,6);
 f.apply([state(1,1,9,8,{movementRevision:Long.UONE,movementKind:2,direction:6,hp:Long.fromInt(80),statuses:[{statusId:31,remainingMs:1000}]})],100);
 near(visual.motion.sample(100).x,8+1/3);assert.equal(visual.pose.frame,3);assert.equal(visual.state.hp.toString(),'80');assert.equal(visual.motion.sample(100).stepSequence,first.stepSequence);assert.equal(f.view.statusEffects.nodes.size,1);
 const direction=f.view.projection.visualDirection(6),layer=visual.profile.layers[0];assert.equal(bodyKey(visual),layer.actions.walking.directions[direction].frames[3]);
 f.update(200);assert.equal(visual.pose.frame,1);f.update(300);assert.equal(visual.pose.action,'struck');assert.equal(visual.motion.sample(300).x,9);
 f.update(600);assert.equal(visual.pose.action,'standing');
 assert.equal(visual.node.getChildByName('Name').position.y,nameY);assert.equal(visual.node.getChildByName('HP').position.y,hpY);assert.equal(visual.node.getChildByName('Name').scale.x,.7);f.view.dispose();
});
test('真实View强制抢占旧攻击hold，迟到旧身体消息不重锁，较新攻击在落脚后播放',()=>{
 const f=makeWorld(1);f.apply([state()]);const visual=f.view.visuals.get(1);
 const attack=(id,sequence)=>({sceneEpoch:Long.UONE,eventId:Long.fromInt(id),casterHandle:1,casterLife:Long.UZERO,attackSequence:sequence,direction:4,skillId:0});
 f.view.acceptAttack(attack(1,5));f.update(0);assert.equal(visual.pose.action,'attack');assert.equal(visual.animation.playbackUntil,600);
 f.cc.game.totalTime=100;f.view.acceptDisplacement(event({attackSequence:5,castSequence:6}));assert.equal(visual.animation.playbackUntil,0);
 f.apply([state(1,1,9,8,{movementRevision:Long.UONE,movementKind:2,direction:6,attackSequence:5,castSequence:6,action:2})],200);
 f.view.acceptAttack(attack(2,5));f.update(250);near(visual.motion.sample(250).x,8.5);assert.equal(visual.pose.action,'walking');
 f.view.acceptAttack(attack(3,6));f.update(399);assert.equal(visual.pose.action,'walking');f.update(400);assert.equal(visual.pose.action,'attack');assert.equal(visual.animation.currentAttackSequence,6);assert.equal(visual.pose.direction,4);f.view.dispose();
});
test('真实View基线先到/重入/丢事件只恢复，原地传送立即校准而不变普通步',()=>{
 const f=makeWorld(1);f.apply([state()]);f.apply([state(1,1,9,8,{movementRevision:Long.UONE,movementKind:2})],100);f.view.acceptDisplacement(event());assert.equal(f.view.visuals.get(1).motion.sample(100).moving,false);
 const rejoined=makeWorld(1);rejoined.apply([state(1,1,9,8,{movementRevision:Long.UONE,movementKind:3})]);assert.equal(rejoined.view.visuals.get(1).pose.action,'standing');rejoined.view.acceptDisplacement(event());assert.equal(rejoined.view.visuals.get(1).motion.sample(0).moving,false);
 rejoined.view.acceptDisplacement(event({movementRevision:Long.fromInt(2),kind:4,origin:{x:9,y:8},path:[{x:9,y:8}],stepDurationMs:0}));assert.equal(rejoined.view.visuals.get(1).motion.sample(0).moving,false);assert.equal(rejoined.view.effects.teleports.size,1);
 f.view.dispose();rejoined.view.dispose();
});
test('真实Running三层使用原WIL尺寸及偏移，连续force转角后接普通600ms怪物步',()=>{
 const f=makeWorld(1);f.apply([state(),state(3,3,8,10)]);
 f.view.acceptDisplacement(event({kind:3,path:[{x:9,y:8},{x:9,y:9}],facingDirection:4}));f.update(400);
 const visual=f.view.visuals.get(1);assert.equal(visual.pose.action,'running');assert.equal(visual.pose.frame,4);
 const direction=f.view.projection.visualDirection(4);
 for(const [index,layer]of visual.profile.layers.entries()){const key=layer.actions.running.directions[direction].frames[4],frame=f.assets.manifest.frames[key],sprite=visual.body[index];assert.equal(sprite.spriteFrame.key,key);assert.equal(sprite.node.getComponent(UITransform).width,frame.width);assert.equal(sprite.node.getComponent(UITransform).height,frame.height);assert.equal(sprite.node.position.x,frame.offset_x-24);assert.equal(sprite.node.position.y,16-frame.offset_y);assert.equal(sprite.node.scale.x,1);}
 f.cc.game.totalTime=450;f.view.acceptDisplacement(event({entityHandle:3,movementRevision:Long.UONE,origin:{x:8,y:10},path:[{x:9,y:10}]}));
 f.apply([state(1,1,9,9,{movementRevision:Long.UONE,movementKind:3}),state(3,3,10,10,{movementRevision:Long.fromInt(2),movementKind:1})],500);
 const monster=f.view.visuals.get(3);f.update(750);near(monster.motion.sample(750).x,9);f.update(1050);near(monster.motion.sample(1050).x,9.5);f.update(1350);assert.equal(monster.motion.sample(1350).x,10);assert.equal(monster.pose.action,'standing');f.view.dispose();
});
test('真实通用Teleport本人只有入场，旁观者出入场独立锚点，旧PNG不拉伸',()=>{
 for(const owner of [true,false]){const f=makeWorld(1),handle=owner?1:3,kind=owner?1:3;f.apply([state(handle,kind)]);
  f.view.acceptDisplacement(event({entityHandle:handle,kind:4,path:[{x:10,y:8}],stepDurationMs:0}));f.update(100);
  const entries=Array.from(f.view.effects.nodes.entries()).filter(([key])=>key.startsWith('teleport:'));assert.equal(entries.length,owner?1:2);
  for(const [id,visual]of entries){const key=visual.sprite.spriteFrame.key;assert.match(key,/^Magic:2[56]\d$/);const record=f.assets.manifest.frames[key];assert.equal(visual.node.getComponent(UITransform).width,record.width);assert.equal(visual.node.getComponent(UITransform).height,record.height);const point=id.endsWith(':out')?f.view.grid(8,8):f.view.grid(10,8);assert.equal(visual.node.position.x,point.x+record.offset_x-24);assert.equal(visual.node.position.y,point.y+16-record.offset_y);}
  assert.equal(entries.some(([key])=>key.endsWith(':out')),!owner);f.update(500);assert.equal(f.view.effects.teleports.size,0);assert.equal(Array.from(f.view.effects.nodes.keys()).filter(key=>key.startsWith('teleport:')).length,0);f.view.dispose();
 }
});
test('显式ownerHandle决定同为kind1的Dash首相、传送出入场和镜头，不能猜ID1',()=>{
 for(const handle of [1,2]){
  const f=makeWorld(2);f.apply([state(1,1,8,8),state(2,1,5,5)]);assert.equal(f.view.playerPosition.x,5);
  const origin=handle===1?{x:8,y:8}:{x:5,y:5},next={x:origin.x+1,y:origin.y};
  f.view.acceptDisplacement(event({entityHandle:handle,kind:3,origin,path:[next]}));f.update(100);
  f.cc.game.totalTime=100;f.view.acceptDisplacement(event({entityHandle:handle,movementRevision:Long.fromInt(2),kind:3,origin:next,path:[{x:next.x+1,y:next.y}]}));f.update(300);
  assert.equal(f.view.visuals.get(handle).motion.sample(300).dashPhase,handle===2?1:0);if(handle===1)assert.equal(f.view.playerPosition.x,5);
  f.cc.game.totalTime=350;f.view.acceptDisplacement(event({entityHandle:handle,movementRevision:Long.fromInt(3),kind:4,origin:{x:next.x+1,y:next.y},path:[{x:10,y:10}],stepDurationMs:0}));
  const entries=Array.from(f.view.effects.nodes.keys()).filter(key=>key.startsWith('teleport:'));assert.equal(entries.some(key=>key.endsWith(':out')),handle!==2);
  if(handle===2)assert.equal(f.view.playerPosition.x,10);else assert.equal(f.view.playerPosition.x,5);f.view.dispose();
 }
});
test('强制位移只抢占身体水位，已释放投射/权威命中保留；新life拒旧轨迹',()=>{
 const f=makeWorld(1);f.apply([state(),state(3,3,10,8)]);
 f.view.acceptCast({sceneEpoch:Long.UONE,castId:Long.UONE,skillId:1,casterHandle:1,casterLife:Long.UZERO,targetHandle:3,targetLife:Long.UZERO,origin:{x:8,y:8},targetGrid:{x:10,y:8},direction:2,delayMs:800,inputSequence:1,skillLevel:0});
 f.cc.game.totalTime=550;f.view.acceptDisplacement(event({attackSequence:2,castSequence:1}));assert.equal(f.view.effects.playback.casts.length,1);assert(Array.from(f.view.effects.nodes.keys()).some(key=>key.endsWith(':projectile')));
 f.view.acceptImpact({sceneEpoch:Long.UONE,eventId:Long.UONE,skillId:1,casterHandle:1,targetHandle:3,targetLife:Long.UZERO,targetGrid:{x:10,y:8},amount:Long.ONE,outcome:1,healing:false,castId:Long.UONE,attackSequence:0});f.update(800);assert(Array.from(f.view.effects.nodes.keys()).some(key=>key.endsWith(':hit')));
 f.apply([state(1,1,5,5,{life:Long.UONE}),state(3,3,10,8)],900);f.view.acceptDisplacement(event({movementRevision:Long.fromInt(2)}));assert.equal(f.view.visuals.get(1).motion.sample(900).x,5);assert.equal(f.view.visuals.get(1).motion.sample(900).moving,false);f.view.dispose();
});
