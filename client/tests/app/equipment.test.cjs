const {test}=require('node:test'),assert=require('node:assert/strict');
const {Long}=require('./helpers/session_harness.cjs');
const model=require('../../../.work/ui-tests/app/EquipmentModel.js');
const bytes=n=>new Uint8Array(16).fill(n);
const base={physical_attack_min:2,physical_attack_max:5,magic_attack_min:0,magic_attack_max:1,spirit_attack_min:0,spirit_attack_max:3,physical_defence_min:0,physical_defence_max:0,magic_defence_min:0,magic_defence_max:0,attack_speed:0};
const definition={item_id:218,name:'木剑',kind:'equipment',equip_slot:0,required_level:1,allowed_jobs:[0,1,2],equipment_stage:0,base_attributes:base};
const item=()=>({instanceId:bytes(1),itemId:218,version:Long.fromString('9007199254740995',true),quantity:Long.ONE,bound:true,locked:false,recycleBlockFlags:0});
function snapshot(){return{ready:true,scope:'connection/binding/role/scene/baseline',roleIdentity:'owner',ruleVersion:Long.fromInt(22,true),level:7,job:2,items:[item()],equipped:[],definitions:[definition]};}

test('单件公开MC/SC上下限和显式0分别显示，缺失/null/危险整数不补假0，非装备不展示基础属性',()=>{
 const lines=model.equipmentDetailLines(definition,item(),7,2,false).join('\n');assert.match(lines,/魔法攻击 0–1/);assert.match(lines,/道术攻击 0–3/);assert.match(lines,/攻速加成 0/);assert.match(lines,/装备阶段 T1/);assert.match(lines,/当前总属性/);assert.match(lines,/数量 1/);assert.match(lines,/来源/);
 for(const value of [null,{...base,spirit_attack_max:null},{...base,attack_speed:-1},{...base,attack_speed:Number.MAX_SAFE_INTEGER+1},Object.fromEntries(Object.entries(base).filter(([key])=>key!=='magic_attack_min'))])assert.deepEqual(model.equipmentBaseAttributeLines({...definition,base_attributes:value}),['基础属性尚未同步']);
 assert.deepEqual(model.equipmentBaseAttributeLines({...definition,kind:'book'}),[]);assert.match(model.equipmentDetailLines({...definition,equipment_stage:1},item(),7,2,false).join('\n'),/阶段尚未同步/);
});

test('确认绑定原instance/itemID/version/容器/slot，版本超过2^53仍保持Long；无关经验/修订推进不失效',()=>{
 const s=snapshot(),preview=model.createEquipmentPreview(s,s.items[0],false);assert.equal(preview.itemVersion.toString(),'9007199254740995');s.level=8;s.revision=Long.fromInt(999,true);assert.doesNotThrow(()=>model.validateEquipmentPreview(preview,s));
 for(const change of [v=>v.items[0].version=v.items[0].version.add(1),v=>v.items[0].itemId=193,v=>{v.equipped=[{slot:0,item:v.items.shift()}];},v=>v.items[0].recycleBlockFlags=4,v=>v.scope='new-binding',v=>v.roleIdentity='other-role']){const next=snapshot(),p=model.createEquipmentPreview(next,next.items[0],false);change(next);assert.throws(()=>model.validateEquipmentPreview(p,next),/变化|处理/);}
 assert.deepEqual(preview.itemInstanceId,bytes(1));
 for(const bad of [Long.MAX_VALUE,Long.MAX_UNSIGNED_VALUE,1]){const next=snapshot();next.items[0].version=bad;assert.throws(()=>model.createEquipmentPreview(next,next.items[0],false),/版本/);}
});

test('卸下只查原已穿戴slot，不拿背包同ID替代；现等级不足/职业已变仍可卸下，锁定不阻穿脱',()=>{
 const s=snapshot(),worn=s.items.shift();worn.locked=true;worn.recycleBlockFlags=9;s.equipped=[{slot:0,item:worn}];s.level=1;s.job=1;s.definitions=[{...definition,required_level:99,allowed_jobs:[0]}];
 assert.equal(model.equipmentActionReason(s,worn,true,0),'');const preview=model.createEquipmentPreview(s,worn,true,0);assert.equal(preview.unequip,true);assert.doesNotThrow(()=>model.validateEquipmentPreview(preview,s));
 s.equipped=[];s.items=[worn];assert.throws(()=>model.validateEquipmentPreview(preview,s),/原装备/);assert.throws(()=>model.createEquipmentPreview(s,worn,true,0),/所在位置/);
});

test('穿戴保留需求门槛与强保护，确认不承诺丢装或自动清包',()=>{
 const s=snapshot();s.definitions=[{...definition,required_level:8}];assert.match(model.equipmentActionReason(s,s.items[0],false),/需8级/);s.level=8;s.items[0].locked=true;s.items[0].recycleBlockFlags=1;assert.equal(model.equipmentActionReason(s,s.items[0],false),'');
 for(const flags of [2,4,8,16]){s.items[0].recycleBlockFlags=flags;assert.notEqual(model.equipmentActionReason(s,s.items[0],false),'');}s.items[0].recycleBlockFlags=0;
 assert.match(model.equipmentConfirmation(model.createEquipmentPreview(s,s.items[0],false),'木剑'),/空间不足时不会丢弃/);assert.equal(s.items.length,1);
});
