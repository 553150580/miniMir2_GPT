const {test}=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');
const models=require('../../../.work/ui-tests/app/CombatStatusModel.js'),{StatusPlayback}=require('../../../.work/ui-tests/app/StatusPlayback.js');
const {captureSupply,selectedCastingSupply,validateSupply}=require('../../../.work/ui-tests/app/CastingSupplyModel.js');
const state=(id=31,remainingMs=10000)=>({statusId:id,sourceSkillId:id,skillLevel:1,remainingMs,maxPhysicalDefenceBonus:'0',maxMagicDefenceBonus:'0',damageReductionPercent:30,casterHandle:1,casterLife:'0'});
const entity=(life='0',statuses=[state()],hp='100')=>({entityHandle:1,entityKind:1,life,hp,statuses});
test('权威状态独立于施法受理：0秒保持、空列表清除、旧场景与旧序号拒绝且复活换life不继承旧受击',()=>{
    const playback=new StatusPlayback();playback.bind('9007199254740993');
    const observe=(sequence,rows,time)=>playback.observe('9007199254740993',sequence,rows,time);
    assert(observe('9007199254740995',[entity()],100));assert.equal(playback.poses(100,600)[0].part,'idle');
    assert(observe('9007199254740996',[entity('0',[state(31,0)])],200));assert.equal(playback.poses(200,600).length,1);
    assert(observe('9007199254740997',[entity('0',[state()], '90')],300));assert.equal(playback.poses(300,600)[0].part,'struck');
    assert.equal(playback.observe('1','99999999999999999',[],400),false);assert.equal(observe('9007199254740996',[],400),false);
    assert(observe('9007199254740998',[entity('1')],400));assert.equal(playback.poses(400,600)[0].life,'1');assert.equal(playback.poses(400,600)[0].part,'idle');
    assert(observe('9007199254740999',[entity('1',[])],500));assert.equal(playback.poses(500,600).length,0);
    playback.bind('9007199254740993');assert(observe('1',[entity('1')],600));assert.equal(playback.poses(600,600)[0].part,'idle');
});
test('倒计时仅展示且时钟回退不增长，状态说明为真实防御上限/减伤，不伪造吸收量',()=>{
    const buff=state(14,1250);buff.maxMagicDefenceBonus='9007199254740993';buff.damageReductionPercent=0;
    assert.equal(models.statusSeconds(buff,100,100),2);assert.equal(models.statusSeconds(buff,100,50),2);assert.equal(models.statusSeconds(buff,100,2000),0);assert.equal(buff.remainingMs,1250);
    const text=models.statusLines(buff,100,2000).join('\n');assert.match(text,/魔法防御上限 \+9007199254740993/);assert.match(text,/等待状态更新/);assert(!text.includes('吸收'));
    const definitions=[{id:31,name:'数据表魔法盾',short_name:'魔盾新'}];assert.equal(models.statusName(31,definitions),'数据表魔法盾');assert.equal(models.statusShortName(31,definitions),'魔盾新');
    assert(models.detailLines(['模式验收战士角色姓名测试与额外很长很长的中文状态说明']).every(line=>Array.from(line).length<=20));
});
function supply(){const item={instanceId:new Uint8Array(16).fill(7),itemId:187,version:'9007199254740995',quantity:'10',bound:true};return {item,snapshot:{ready:true,roleIdentity:'role1',sceneEpoch:'9007199254740993',ruleVersion:'8',playerLife:'0',inventory:{revision:'19',items:[item],equipped:[{slot:100,item}]},config:{enabled:true,slot:100,item_ids:[187]}}};}
test('材料施法固定已装原护符，不自动选背包替代；实例版本/数量/角色/scene/life变化均要求重选',()=>{
    const {item,snapshot}=supply(),skill={material_item_id:187,material_quantity:1},selection=selectedCastingSupply(snapshot,skill);
    assert.equal(selection.version,'9007199254740995');assert.equal(validateSupply(selection,snapshot,true),item);
    snapshot.inventory.revision='20';assert.equal(validateSupply(selection,snapshot,true,false),item,'材料瞄准不受无关经济revision变化影响');snapshot.inventory.revision='19';
    for(const mutate of [s=>s.roleIdentity='other',s=>s.sceneEpoch='2',s=>s.playerLife='1',s=>s.inventory.revision='20',s=>s.inventory.equipped[0].item.version='21',s=>s.inventory.equipped[0].item.quantity='9',s=>s.inventory.equipped[0].item.instanceId=new Uint8Array(16).fill(8)]){
        const row=supply();const preview=selectedCastingSupply(row.snapshot,skill);mutate(row.snapshot);assert.throws(()=>validateSupply(preview,row.snapshot,true),/已变化/);
    }
    snapshot.inventory.equipped=[];assert.throws(()=>selectedCastingSupply(snapshot,skill),/先在背包中装入/);
    snapshot.config.enabled=false;assert.throws(()=>captureSupply(snapshot,item),/暂未开放/);
});
test('状态资产只追加布局：旧988对象/Entity21对象原样保持，滚动覆盖详情与普通层分离',()=>{
    const root=path.resolve(__dirname,'../../..'),read=p=>JSON.parse(fs.readFileSync(path.join(root,p),'utf8')),scene=read('client/assets/scenes/Game.scene');
    const old=structuredClone(scene.slice(0,988));for(const row of old)if(row._children)row._children=row._children.filter(ref=>ref.__id__<988);
    assert.equal(crypto.createHash('sha256').update(JSON.stringify(old)).digest('hex'),'e22c27b81e71cdab6349819ed528c85a495419112e2e5776fbbd705cdd445fb8');
    const entity=read('client/assets/resources/prefabs/Entity.prefab');assert.deepEqual(entity[1]._children.map(ref=>entity[ref.__id__]._name),['Body','AttachedEffects','Name','HP']);const before=structuredClone(entity.slice(0,21));before[1]._children=before[1]._children.filter(ref=>ref.__id__<21);
    assert.equal(crypto.createHash('sha256').update(JSON.stringify(before)).digest('hex'),'628e03206b2ef5370978bed19c5204a32bf268284ab25736e63ec7c33fda7973');
    const named=name=>scene.find(row=>row.__type__==='cc.Node'&&row._name===name),components=n=>n._components.map(ref=>scene[ref.__id__]);
    const bar=named('BuffBar');assert(components(bar).some(row=>row.__type__==='cc.Layout'));assert(components(bar).some(row=>row.__type__==='cc.Widget'));
    for(const name of ['StatusDetails','CombatStatus']){const panel=named(name);assert(panel);assert.equal(panel._active,false);assert(!panel._children.some(ref=>/Close|Cancel/.test(scene[ref.__id__]._name)));}
    assert.equal(scene[named('StatusDetails')._parent.__id__]._name,'Panels');assert.notEqual(named('StatusDetails')._parent.__id__,named('CombatStatus')._parent.__id__);
    const badge=read('client/assets/resources/prefabs/StatusBadge.prefab');assert(badge.some(row=>row.__type__==='cc.Button'));assert(badge.filter(row=>row.__type__==='cc.Label').every(row=>row._fontSize===24));assert(!badge.some(row=>row.__type__==='cc.Graphics'));
});

test('防御升级与技能中文说明区分减伤、时长系数和基础秒数，耗符不冒充升级材料或最终伤害',()=>{
    const {skillQuoteLines}=require('../../../.work/ui-tests/app/SkillAssetsModel.js'),{skillDescriptionLines}=require('../../../.work/ui-tests/app/SkillModel.js');
    const quote={kind:2,fromLevel:0,toLevel:1,stats:[['damage_reduction_percent',20,30],['duration_rank_factor',1,2],['base_duration_seconds',50,100],['material_quantity',1,1]].map(([key,before,after])=>({key,beforeMin:String(before),beforeMax:String(before),afterMin:String(after),afterMax:String(after)})),spend:{gold:'0',boundGold:'0',yuanbaoUnits:'0',boundYuanbaoUnits:'0'},materials:[],outputs:[]};
    const lines=skillQuoteLines(quote,[]).join('\n');assert.match(lines,/伤害减免（%）/);assert.match(lines,/阶数系数（非秒数）/);assert.match(lines,/基础持续时间/);assert.match(lines,/每次施法护符用量/);assert(!lines.includes('技能基础威力'));
    const snapshot={statusDefinitions:[{id:31,attribute_effects:{value_by_level:[20,30,40,50]}},{id:14,attribute_effects:{value_by_level:[0,0,0,0],target_level_divisor:7,target_level_add:4},duration_rule:{stat_multiplier:4,seconds_by_level:[50,100,150,200]}}],castingSupplyEnabled:true,castingSupply:{name:'护身符',quantity:'99'}};
    const shield=skillDescriptionLines({definition:{kind:'magic_shield',buff_id:31,learn_level:31,active:true},status:'已学',level:1,cost:40},snapshot).join('\n');assert.match(shield,/减免 30%/);assert.match(shield,/仅对自己/);assert.match(shield,/仍消耗魔法，但不刷新/);
    const ghost=skillDescriptionLines({definition:{kind:'soul_shield',buff_id:14,area_radius:3,material_quantity:1,learn_level:22,active:true},status:'已学',level:1,cost:4},snapshot).join('\n');assert.match(ghost,/半径3格/);assert.match(ghost,/等级÷7取整＋4/);assert.match(ghost,/道术值×4＋100秒/);assert.match(ghost,/护身符 · 余99/);assert.match(ghost,/倒下时仍可受益/);assert.match(ghost,/无主宠物尸体不属于友方/);
});
