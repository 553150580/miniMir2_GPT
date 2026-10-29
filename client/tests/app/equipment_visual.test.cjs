const {test}=require('node:test'),assert=require('node:assert/strict');
const {makeWorld,state,Long}=require('../support/world_runtime.cjs');
const identity={connection:1,binding:'1',role:'owner',scene:'1',baseline:'1',map:'1',travelId:''};
function weapon(visual){return visual.body[visual.profile.layers.findIndex(layer=>layer.package==='Weapon')];}
function fixture(){const f=makeWorld(7);let current=true;f.view.bindPresentation(identity,()=>current);f.apply([state(2,1),state(7,1)]);return Object.assign(f,{retire(){current=false;}});}

test('真实View未知Inventory沿用候选武器，明确当前own空slot0只隐藏own层，其他Kind1不受影响',()=>{
 const f=fixture(),own=f.view.visuals.get(7),other=f.view.visuals.get(2);assert.equal(weapon(own).node.active,true);assert.equal(f.view.observeEquipment(identity,7,null),false);f.update(100);assert.equal(weapon(own).node.active,true);
 assert.equal(f.view.observeEquipment(identity,7,{equipped:[]}),true);f.update(150);assert.equal(weapon(own).node.active,false);assert.equal(weapon(other).node.active,true);f.view.dispose();
});

test('当前武器穿回只恢复固定原层，不重建Life/动作/位移/头顶，也不宣称item专属外观',()=>{
 const f=fixture();f.apply([state(7,1,8,8,{attackSequence:1,action:2})],100);const v=f.view.visuals.get(7),animation=v.animation,motion=v.motion,head=v.node.getChildByName('Name').position.y;
 f.view.observeEquipment(identity,7,{equipped:[]});f.update(200);const before={...v.pose};f.view.observeEquipment(identity,7,{equipped:[{slot:0,item:{itemId:218}}]});f.update(250);
 assert.equal(f.view.visuals.get(7),v);assert.equal(v.animation,animation);assert.equal(v.motion,motion);assert.equal(v.state.life.toString(),'0');assert.equal(v.pose.action,before.action);assert(v.pose.frame>=before.frame);assert.equal(v.node.getChildByName('Name').position.y,head);assert.equal(weapon(v).node.active,true);assert.equal(v.profile.id,'player_male');f.view.dispose();
});

test('旧binding/不同角色/新scene/错playerHandle/已退休附着拒绝更新，不把未知slot0定义当已卸下',()=>{
 const f=fixture(),v=f.view.visuals.get(7);for(const [scope,handle]of [[{...identity,binding:'2'},7],[{...identity,role:'other'},7],[{...identity,scene:'2'},7],[identity,2]])assert.equal(f.view.observeEquipment(scope,handle,{equipped:[]}),false);
 assert.equal(f.view.observeEquipment(identity,7,{equipped:[{slot:0,item:{itemId:99999}}]}),false);f.update(100);assert.equal(weapon(v).node.active,true);f.retire();assert.equal(f.view.observeEquipment(identity,7,{equipped:[]}),false);f.update(200);assert.equal(weapon(v).node.active,true);f.view.dispose();
});
