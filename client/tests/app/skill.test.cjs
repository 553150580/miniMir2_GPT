const { test } = require('node:test');
const assert = require('node:assert/strict');
const { skillRows, primarySkill, skillAccuracy, createSkillLearnPreview, validateSkillLearnPreview, skillBookInstance, selectSkillTarget, gridDirection } = require('../../../.work/ui-tests/app/SkillModel.js');
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
    assert.equal(selectSkillTarget(learned(), entities, 3).entityHandle, 3);
    assert.equal(selectSkillTarget(learned(), entities, 2).entityHandle, 4);
    assert.equal(selectSkillTarget(learned(), entities, null).entityHandle, 4);
    assert.throws(() => selectSkillTarget(learned(), entities, 5), /已倒下/);
    assert.throws(() => selectSkillTarget({ ...learned(), learned: false }, entities, null), /尚未学会/);
    assert.throws(() => selectSkillTarget(learned({ active: false }), entities, null), /被动/);
    assert.throws(() => selectSkillTarget(learned({ range: 1 }), entities, 3), /超出/);
    entities[0].mp = '2'; assert.throws(() => selectSkillTarget(learned(), entities, null), /魔法不足/);
    entities[0].hp = '0'; assert.throws(() => selectSkillTarget(learned(), entities, null), /角色已倒下/);
});
test('治愈支持自己及当前宠物，敌方选择退回自己，已死亡友方拒绝', () => {
    const entities = [entity(1, 1, 2, 2), entity(2, 2, 3, 3), entity(3, 3, 2, 3)];
    const skill = learned({ kind: 'healing' });
    assert.equal(selectSkillTarget(skill, entities, 2).entityHandle, 2);
    assert.equal(selectSkillTarget(skill, entities, null).entityHandle, 1);
    assert.equal(selectSkillTarget(skill, entities, 3).entityHandle, 1);
    entities[1].hp = '0'; assert.throws(() => selectSkillTarget(skill, entities, 2), /目标已倒下/);
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
test('火球500毫秒开始飞行，与600毫秒动作重叠；服务端delay决定命中而非本地距离', () => {
    const value = playback(); value.accept(event(), definition(), 100);
    assert.deepEqual(effectPoses(value.casts[0], 599).map(p=>p.kind), ['cast']);
    const concurrent = effectPoses(value.casts[0], 650);
    assert.deepEqual(concurrent.map(p=>p.kind), ['cast','projectile']); assert.equal(concurrent[1].fraction, .25); assert.equal(concurrent[1].direction, 4);
    assert.deepEqual(effectPoses(value.casts[0], 799).map(p=>p.kind), ['projectile']);
    assert.deepEqual(effectPoses(value.casts[0], 800).map(p=>p.kind), ['hit']);
    assert.equal(effectPoses(value.casts[0], 800)[0].elapsed, 0);
    value.expire(1399, () => 600); assert.equal(value.casts.length, 1);
    value.expire(1400, () => 600); assert.equal(value.casts.length, 0);
});
test('治愈在服务端500毫秒命中，没有伪造飞弹；后台长暂停后旧特效到期', () => {
    const value = playback(); value.accept(event({ skillId: 2, delayMs: 500 }), definition({ id: 2, kind: 'healing' }), 0);
    assert.deepEqual(effectPoses(value.casts[0], 500).map(p=>p.kind), ['cast','hit']);
    assert.deepEqual(effectPoses(value.casts[0], 600).map(p=>p.kind), ['hit']);
    value.expire(10000, () => 800); assert.equal(value.casts.length, 0);
});
