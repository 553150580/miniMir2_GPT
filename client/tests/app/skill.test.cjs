const { test } = require('node:test');
const assert = require('node:assert/strict');
const { skillRows, primarySkill, skillAccuracy, skillQuickLabel, createSkillLearnPreview, validateSkillLearnPreview, skillBookInstance, selectSkillTarget, validateGroundTarget, gridDirection } = require('../../../.work/ui-tests/app/SkillModel.js');
const { CastPlayback, effectPoses } = require('../../../.work/ui-tests/app/CastPlayback.js');
const id = value => new Uint8Array(16).fill(value);
const definition = changes => ({ id: 1, name: '火球术', job_id: 1, book_item_id: 7, learn_level: 7, initial_level: 0,
    kind: 'fireball', active: true, range: 9, base_cost: 3, level_cost: 2, cast_ms: 600, impact_base_ms: 500, projectile_ms_per_cell: 50, accuracy_per_level: 0, ...changes });
function snapshot() {
    return { ready: true, roleIdentity: '1:roleA', job: 1, sceneEpoch: '9007199254740993', ruleVersion: '9',
        definitions: [definition(), definition({ id: 2, name: '治愈术', job_id: 2, kind: 'healing', book_item_id: 21 }),
            definition({ id: 3, name: '基本剑术', job_id: 0, kind: 'fencing', active: false, book_item_id: 1, accuracy_per_level: 3 })],
        inventory: { revision: '9007199254740995', level: 7, learnedSkills: [], items: [
            { instanceId: id(1), itemId: 7, bound: true, quantity: '0', version: '1' },
            { instanceId: id(2), itemId: 7, bound: false, quantity: '9007199254740993', version: '4' },
            { instanceId: id(3), itemId: 7, bound: true, quantity: '3', version: '8' }] } };
}
const entity = (handle, kind, x, y, hp = '100') => ({ entityHandle: handle, entityKind: kind, position: { x, y }, hp, mp: '20', direction: 4 });
const learned = changes => ({ definition: definition(changes), learned: true, level: 0, canLearn: false, cost: 3, status: '' });

test('技能列表仅投影本职，已学0级保留初学及按等级计算的MP，剑术不设主动入口', () => {
    const input = snapshot(); input.inventory.learnedSkills = [{ skillId: 1, level: 0 }];
    const rows = skillRows(input); assert.equal(rows.length, 1); assert.equal(rows[0].learned, true);
    assert.match(rows[0].status, /初学（等级0）/); assert.equal(rows[0].canLearn, false); assert.equal(rows[0].cost, 3);
    input.inventory.learnedSkills[0].level = 2; assert.equal(primarySkill(input).cost, 7);
    input.job = 0; assert.equal(primarySkill(input), null); assert.equal(skillRows(input)[0].definition.active, false);
    input.inventory.learnedSkills=[{skillId:3,level:0}];assert.equal(skillAccuracy(skillRows(input)[0]),0);
    input.inventory.learnedSkills[0].level=1;assert.equal(skillAccuracy(skillRows(input)[0]),3);
});
test('学习需到7级并有对应原书，按库存首个正数量选实例且保留绑定、Long数量和版本', () => {
    const input = snapshot(), preview = createSkillLearnPreview(input, 1);
    assert.equal(preview.instanceHex, '02'.repeat(16)); assert.equal(preview.quantity, '9007199254740993');
    assert.match(preview.message, /非绑定/); assert.match(preview.message, /等级0/);
    assert.deepEqual(skillBookInstance(preview), id(2)); validateSkillLearnPreview(preview, input);
    input.inventory.level = 6; assert.equal(skillRows(input)[0].canLearn, false);
    assert.throws(() => createSkillLearnPreview(input, 1), /达到7级/);
    assert.throws(() => createSkillLearnPreview(snapshot(), 2), /当前职业/);
    input.inventory.level = 7; input.inventory.items = []; assert.throws(() => createSkillLearnPreview(input, 1), /没有对应技能书/);
});
test('学习确认前的角色、场景、规则、背包和原书变化均须重新预览，不能换另一本书', () => {
    for (const mutate of [
        v => { v.roleIdentity = '1:roleB'; }, v => { v.sceneEpoch = '2'; }, v => { v.ruleVersion = '10'; },
        v => { v.inventory.revision = '9007199254740996'; }, v => { v.inventory.items[1].version = '5'; },
        v => { v.inventory.items[1].quantity = '2'; }, v => { v.inventory.items.splice(1, 1); },
        v => { v.inventory.learnedSkills = [{ skillId: 1, level: 0 }]; }, v => { v.ready = false; },
    ]) {
        const input = snapshot(), preview = createSkillLearnPreview(input, 1); mutate(input);
        assert.throws(() => validateSkillLearnPreview(preview, input), /请重新预览/);
    }
});
test('火球优先合法所选怪物，否则取最近活怪；资源、死亡、被动与距离拒绝均明确', () => {
    const entities = [entity(1, 1, 2, 2), entity(2, 2, 2, 3), entity(3, 3, 5, 2), entity(4, 3, 3, 2), entity(5, 3, 2, 2, '0')];
    assert.equal(selectSkillTarget(learned(), entities, 3,1).entityHandle, 3);
    assert.equal(selectSkillTarget(learned(), entities, 2,1).entityHandle, 4);
    assert.equal(selectSkillTarget(learned(), entities, null,1).entityHandle, 4);
    assert.throws(() => selectSkillTarget(learned(), entities, 5,1), /已倒下/);
    assert.throws(() => selectSkillTarget({ ...learned(), learned: false }, entities, null,1), /尚未学会/);
    assert.throws(() => selectSkillTarget(learned({ active: false }), entities, null,1), /被动/);
    assert.throws(() => selectSkillTarget(learned({ range: 1 }), entities, 3,1), /超出/);
    entities[0].mp = '2'; assert.throws(() => selectSkillTarget(learned(), entities, null,1), /魔法不足/);
    entities[0].hp = '0'; assert.throws(() => selectSkillTarget(learned(), entities, null,1), /角色已倒下/);
});
test('施法者和治疗默认自身显式使用owner，未实现的其它玩家不冒充友方自身',()=>{
    const entities=[entity(2,1,16,16,'0'),entity(7,1,2,2),entity(3,3,3,2)];
    assert.equal(selectSkillTarget(learned(),entities,null,7).entityHandle,3);
    assert.equal(selectSkillTarget(learned({kind:'healing',target_kind:'friendly_entity'}),entities,2,7).entityHandle,7);
    entities[0].hp='100';entities[1].hp='0';assert.throws(()=>selectSkillTarget(learned(),entities,null,7),/角色已倒下/);
});
test('治愈支持自己及当前宠物，敌方选择退回自己，已死亡友方拒绝', () => {
    const entities = [entity(1, 1, 2, 2), entity(2, 2, 3, 3), entity(3, 3, 2, 3)];
    const skill = learned({ kind: 'healing' });
    assert.equal(selectSkillTarget(skill, entities, 2,1).entityHandle, 2);
    assert.equal(selectSkillTarget(skill, entities, null,1).entityHandle, 1);
    assert.equal(selectSkillTarget(skill, entities, 3,1).entityHandle, 1);
    entities[1].hp = '0'; assert.throws(() => selectSkillTarget(skill, entities, 2,1), /目标已倒下/);
});
test('八方向身体与十六方向飞行按地图上方为0、顺时针排列，原地保留方向', () => {
    const origin = { x: 5, y: 5 }, deltas = [[0,-1],[1,-1],[1,0],[1,1],[0,1],[-1,1],[-1,0],[-1,-1]];
    deltas.forEach(([x,y], index) => {
        assert.equal(gridDirection(origin, { x: 5+x, y: 5+y }), index);
        assert.equal(gridDirection(origin, { x: 5+x, y: 5+y }, 4, 16), index*2);
    });
    assert.equal(gridDirection(origin, origin, 6), 6);
    assert.equal(gridDirection(origin, { x: 6, y: 3 }, 4, 16), 1);
});
const event = changes => ({ sceneEpoch: '9007199254740993', castId: '9007199254740995', skillId: 1, casterHandle: 1, targetHandle: 3,
    origin: { x: 2, y: 2 }, targetGrid: { x: 6, y: 2 }, direction: 2, delayMs: 700, ...changes });
function playback() { const value = new CastPlayback(); value.bind('9007199254740993'); return value; }
test('施法瞬时事件在本场景防重和有界，忽略旧场景、旧ID，清理后不补播历史', () => {
    const value = playback(), source = event(); assert.equal(value.accept(source, definition(), 100), true);
    source.targetGrid.x = 99; assert.equal(value.casts[0].event.targetGrid.x, 6);
    assert.equal(value.accept(event(), definition(), 200), false);
    assert.equal(value.accept(event({ castId: '9007199254740994' }), definition(), 200), false);
    assert.equal(value.accept(event({ sceneEpoch: '2', castId: '9007199254740996' }), definition(), 200), false);
    assert.equal(value.accept(event({ delayMs: 30001 }), definition(), 200), false);
    for (let i = 0; i < 40; i++) value.accept(event({ castId: String(9007199254740996n+BigInt(i)) }), definition(), 200+i);
    assert.equal(value.casts.length, value.capacity);
    value.expire(10000, () => 600); assert.equal(value.casts.length, 0);
    value.clear(); assert.equal(value.accept(event(), definition(), 10100), false);
    value.bind('2'); assert.equal(value.casts.length, 0);
});
const impact=changes=>({sceneEpoch:'9007199254740993',eventId:'9007199254740996',skillId:1,casterHandle:1,targetHandle:3,targetLife:'1',targetGrid:{x:6,y:2},amount:'5',outcome:1,healing:false,castId:'9007199254740995',attackSequence:0,...changes});
test('火球500毫秒开始飞行，与600毫秒动作重叠；仅真实Impact开始命中效果', () => {
    const value = playback(); value.accept(event(), definition(), 100);
    assert.deepEqual(effectPoses(value.casts[0], 599).map(p=>p.kind), ['cast']);
    const concurrent = effectPoses(value.casts[0], 650);
    assert.deepEqual(concurrent.map(p=>p.kind), ['cast','projectile']); assert.equal(concurrent[1].fraction, .25); assert.equal(concurrent[1].direction, 4);
    assert.deepEqual(effectPoses(value.casts[0], 799).map(p=>p.kind), ['projectile']);
    assert.deepEqual(effectPoses(value.casts[0], 800).map(p=>p.kind), []);
    value.impact(impact(),800);
    assert.deepEqual(effectPoses(value.casts[0], 800).map(p=>p.kind), ['hit']);
    assert.equal(effectPoses(value.casts[0], 800)[0].elapsed, 0);
    value.expire(1399, () => 600); assert.equal(value.casts.length, 1);
    value.expire(1400, () => 600); assert.equal(value.casts.length, 0);
});
test('治愈只在服务端确认加入治疗队列后显示命中特效，未声称立即加血', () => {
    const value = playback(); value.accept(event({ skillId: 2, delayMs: 500 }), definition({ id: 2, kind: 'healing' }), 0);
    assert.deepEqual(effectPoses(value.casts[0], 500).map(p=>p.kind), ['cast']);
    value.impact(impact({skillId:2,outcome:4,healing:true}),500);
    assert.deepEqual(effectPoses(value.casts[0], 500).map(p=>p.kind), ['cast','hit']);
    assert.deepEqual(effectPoses(value.casts[0], 600).map(p=>p.kind), ['hit']);
    value.expire(10000, () => 800); assert.equal(value.casts.length, 0);
});

test('精神力读取独立准确阶表，等级2为5；快捷选择可绑定第二主动技能且角色间不串技能',()=>{
    const input=snapshot();input.definitions.push(definition({id:11,name:'雷电术',kind:'thunderbolt',target_kind:'enemy_entity'}));
    input.inventory.learnedSkills=[{skillId:1,level:0},{skillId:11,level:0}];assert.equal(primarySkill(input,11).definition.id,11);
    assert.equal(primarySkill(input,999).definition.id,1);input.job=0;assert.equal(primarySkill(input,11),null);
    const spirit={...learned({kind:'spirit_sword',active:false,accuracy_by_level:[0,3,5,8]}),level:2};
    assert.equal(skillAccuracy(spirit),5);spirit.level=0;assert.equal(skillAccuracy(spirit),0);
    assert.throws(()=>skillAccuracy({...spirit,level:4}),/准确加成/);
});
test('地面技能必须明确选择合法范围格，不误用实体选取，也不修改实体或导航输入',()=>{
    const entities=[entity(1,1,2,2),entity(3,3,4,4)],grid={x:4,y:4};
    const skill=learned({kind:'firebang',target_kind:'enemy_grid',area_radius:1,range:9});
    const bounds={width:16,height:16};
    const before=JSON.stringify(entities);assert.deepEqual(validateGroundTarget(skill,entities,grid,bounds,1),grid);assert.equal(JSON.stringify(entities),before);
    assert.throws(()=>selectSkillTarget(skill,entities,3,1),/地面位置/);
    assert.throws(()=>validateGroundTarget(skill,entities,{x:12,y:4},bounds,1),/超出技能射程/);assert.throws(()=>validateGroundTarget(skill,entities,{x:-1,y:4},bounds,1),/无效/);
    assert.throws(()=>validateGroundTarget(learned(),entities,grid,bounds,1),/实体目标/);
    assert.deepEqual(validateGroundTarget(learned({kind:'mass_healing',target_kind:'friendly_grid'}),entities,grid,bounds,1),grid);
});
test('爆裂和群疗只按逻辑地图边界与射程验中心，不检查center是否可走',()=>{
    const entities=[entity(1,1,2,2)];
    const bounds={width:4,height:4,cells:Array.from({length:4},()=>Array.from({length:4},()=>({blocked:true})))};
    for(const [kind,target_kind]of [['firebang','enemy_grid'],['mass_healing','friendly_grid']]){
        const skill=learned({kind,target_kind,range:9});
        assert.deepEqual(validateGroundTarget(skill,entities,{x:1,y:1},bounds,1),{x:1,y:1});
        assert.throws(()=>validateGroundTarget(skill,entities,{x:4,y:1},bounds,1),/地图边界/);
        assert.throws(()=>validateGroundTarget(skill,entities,{x:1,y:4},bounds,1),/地图边界/);
    }
});
test('雷电300ms起手结束后不保持末帧，命中闪光由权威结果驱动；抵抗和失效不伪造命中',()=>{
    for(const outcome of [1,2,3]){
        const value=playback();value.accept(event({skillId:11,delayMs:500,targetLife:'2'}),definition({id:11,kind:'thunderbolt',target_kind:'enemy_entity'}),0);
        assert.deepEqual(effectPoses(value.casts[0],299,300).map(p=>p.kind),['cast']);assert.deepEqual(effectPoses(value.casts[0],300,300),[]);
        assert.equal(value.impact(impact({skillId:11,targetLife:'1',outcome}),500),false);
        assert.equal(value.impact(impact({skillId:11,targetLife:'2',eventId:'9007199254740997',outcome}),500),true);
        assert.deepEqual(effectPoses(value.casts[0],500,300).map(p=>p.kind),outcome===1?['hit']:[]);
        assert.equal(value.impact(impact({skillId:11,targetLife:'2',eventId:'9007199254740997',outcome}),501),false);
    }
});
test('爆裂与群疗一次受理只在原地面播放一次范围效果，不按各目标Impact重播',()=>{
    for(const kind of ['firebang','mass_healing']){
        const value=playback();value.accept(event({skillId:23,targetHandle:0,delayMs:500}),definition({id:23,kind,target_kind:kind==='firebang'?'enemy_grid':'friendly_grid'}),0);
        assert.equal(value.impact(impact({skillId:23,targetHandle:3}),500),false);
        assert.equal(value.impact(impact({skillId:23,targetHandle:4,eventId:'9007199254740997'}),510),false);
        const poses=effectPoses(value.casts[0],600,600);assert.deepEqual(poses.map(p=>p.kind),['hit']);assert.equal(poses[0].elapsed,100);
        value.expire(1500,()=>1000,()=>600);assert.equal(value.casts.length,0);
    }
});
test('大火球沿16方向飞行，未收到结算只清理超时表现而不制造命中',()=>{
    const value=playback();value.accept(event({skillId:5,delayMs:700}),definition({id:5,kind:'great_fireball',target_kind:'enemy_entity'}),0);
    assert.equal(effectPoses(value.casts[0],550,600).find(p=>p.kind==='projectile').direction,4);
    assert.deepEqual(effectPoses(value.casts[0],800,600),[]);
    value.expire(5700,()=>600,()=>600);assert.equal(value.casts.length,0);
});
test('固定74像素快捷槽使用明确短标签，保留原字号、完整技能名与其他全名',()=>{
    const bang=definition({kind:'firebang',name:'爆裂火焰'}),mass=definition({kind:'mass_healing',name:'群体治疗术'});
    assert.equal(skillQuickLabel(bang),'爆裂');assert.equal(skillQuickLabel(mass),'群疗');assert.equal(bang.name,'爆裂火焰');assert.equal(mass.name,'群体治疗术');
    for(const [kind,name]of [['fireball','火球术'],['healing','治愈术'],['great_fireball','大火球'],['thunderbolt','雷电术']]){
        assert.equal(skillQuickLabel(definition({kind,name})),name);assert(Array.from(skillQuickLabel(definition({kind,name}))).length<=3);
    }
});
