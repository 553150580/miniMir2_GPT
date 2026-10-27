const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createPotionPreview, potionInstanceId, validatePotionPreview } = require('../../../.work/ui-tests/app/PotionModel.js');
const { protocolErrorMessage } = require('../../../.work/ui-tests/app/GameErrors.js');
const id = value => new Uint8Array(16).fill(value);
const count = value => ({ toString: () => value });

function snapshot() {
    return {
        ready: true, roleIdentity: '1:roleA', sceneEpoch: count('9007199254740993'), ruleVersion: '7',
        inventory: { revision: '9007199254740995', level: 23, items: [
            { instanceId: id(1), itemId: 35, quantity: '0', bound: true, version: '2' },
            { instanceId: id(2), itemId: 35, quantity: count('9007199254740993'), bound: false, version: '4' },
            { instanceId: id(3), itemId: 35, quantity: '3', bound: true, version: '6' },
            { instanceId: id(4), itemId: 36, quantity: '5', bound: true, version: '1' },
        ] },
        definitions: [
            { item_id: 35, kind: 'potion', name: '金创药(小量)', use_enabled: true,
                potion: { hp: 20, mp: 0, tick_ms: 100, base_per_tick: 3, levels_per_step: 10, use_delay_ms: 150, queue_cap: 500 } },
            { item_id: 36, kind: 'potion', name: '魔法药(小量)', use_enabled: true,
                potion: { hp: 0, mp: 30, tick_ms: 100, base_per_tick: 3, levels_per_step: 10, use_delay_ms: 150, queue_cap: 500 } },
        ],
        player: { hp: '100', hpMax: '100', mp: '50', mpMax: '50' },
    };
}

test('只选择库存顺序中的首个正数量实例，不另设绑定优先级，Long数量保持精确', () => {
    const input = snapshot(), preview = createPotionPreview(input, 35);
    assert.equal(preview.instanceHex, '02'.repeat(16));
    assert.equal(preview.quantity, '9007199254740993');
    assert.equal(preview.sceneEpoch, '9007199254740993');
    assert.equal(preview.inventoryRevision, '9007199254740995');
    assert.match(preview.message, /非绑定/);
    assert.match(preview.message, /所选实例数量：9007199254740993，消耗1个/);
});

test('预览显示总恢复与等级分步值，资源已满仍明确消耗，不承诺固定恢复时间', () => {
    const red = createPotionPreview(snapshot(), 35), blue = createPotionPreview(snapshot(), 36);
    assert.match(red.message, /生命\+20 \/ 魔法\+0/);
    assert.match(red.message, /每100毫秒分步恢复/);
    assert.match(red.message, /每步各最多5点/);
    assert.match(red.message, /当前生命已满，仍会消耗1个/);
    assert.match(blue.message, /当前魔法已满，仍会消耗1个/);
    assert.equal(red.requiresConfirmation, true); assert.equal(blue.requiresConfirmation, true);
    assert.doesNotMatch(red.message, /(?:\d+秒后|\d+毫秒后).*(?:回满|恢复完成)/);
});

test('普通缺血缺蓝直接使用，只有全部有效恢复通道已满才要求确认', () => {
    const input = snapshot();
    input.player.hp = '99'; input.player.mp = '49';
    assert.equal(createPotionPreview(input, 35).requiresConfirmation, false);
    assert.equal(createPotionPreview(input, 36).requiresConfirmation, false);
    input.definitions[0].potion.mp = 10;
    input.player.hp = '100';
    assert.equal(createPotionPreview(input, 35).requiresConfirmation, false, '红满蓝缺的双通道药仍直接使用');
    input.player.hp = '99'; input.player.mp = '50';
    assert.equal(createPotionPreview(input, 35).requiresConfirmation, false, '蓝满红缺的双通道药仍直接使用');
    input.player.hp = '100';
    assert.equal(createPotionPreview(input, 35).requiresConfirmation, true);
});

test('未同步、死亡、禁用或缺失规则与缺药均阻止预览', () => {
    for (const mutate of [
        input => { input.ready = false; }, input => { input.inventory = null; },
        input => { input.player = null; }, input => { input.player.hp = '0'; },
        input => { input.definitions[0].use_enabled = false; },
        input => { delete input.definitions[0].potion; },
        input => { delete input.definitions[0].potion.tick_ms; },
        input => { input.definitions[0].potion.levels_per_step = 0; },
        input => { input.definitions[0].potion.hp = 0; },
        input => { input.inventory.items = input.inventory.items.filter(item => item.itemId !== 35); },
        input => { input.inventory.items[1].quantity = Number.MAX_SAFE_INTEGER + 1; },
    ]) {
        const input = snapshot(); mutate(input);
        assert.throws(() => createPotionPreview(input, 35));
    }
});

test('确认前角色、场景、配置、库存版本、实例版本及数量变化必须重新预览', () => {
    for (const mutate of [
        input => { input.ready = false; }, input => { input.roleIdentity = '1:roleB'; },
        input => { input.sceneEpoch = '9007199254740994'; }, input => { input.ruleVersion = '8'; },
        input => { input.inventory.revision = '9007199254740996'; },
        input => { input.inventory.level++; }, input => { input.inventory.items[1].version = '5'; },
        input => { input.inventory.items[1].quantity = '9007199254740992'; },
        input => { input.inventory.items[1].itemId = 36; },
        input => { input.inventory.items.splice(1, 1); },
        input => { input.definitions[0].use_enabled = false; },
    ]) {
        const input = snapshot(), preview = createPotionPreview(input, 35); mutate(input);
        assert.throws(() => validatePotionPreview(preview, input), /重新点击药品预览/);
    }
});

test('已预览的实例仍存在时允许库存顺序改变，绝不改用另一个绑定实例', () => {
    const input = snapshot(), preview = createPotionPreview(input, 35);
    input.inventory.items.reverse();
    validatePotionPreview(preview, input);
    assert.deepEqual(potionInstanceId(preview), id(2));
    input.inventory.items = input.inventory.items.filter(item => item.instanceId[0] !== 2);
    assert.throws(() => validatePotionPreview(preview, input), /重新点击药品预览/);
});

test('确认时死亡拒绝，正常预览与校验不乐观改写库存或血蓝', () => {
    const input = snapshot(), before = JSON.stringify(input), preview = createPotionPreview(input, 35);
    validatePotionPreview(preview, input);
    assert.equal(JSON.stringify(input), before);
    input.player.hp = '0';
    assert.throws(() => validatePotionPreview(preview, input), /角色已倒下/);
});

test('预览实例身份与外部Uint8Array隔离，请求每次返回独立字节', () => {
    const input = snapshot(), preview = createPotionPreview(input, 35);
    input.inventory.items[1].instanceId.fill(9);
    assert.deepEqual(potionInstanceId(preview), id(2));
    const request = potionInstanceId(preview); request.fill(8);
    assert.deepEqual(potionInstanceId(preview), id(2));
});

test('药品拒绝回执和经济结果使用相同友好说明', () => {
    assert.equal(protocolErrorMessage(40009, 3101), '角色倒下或当前状态不能使用药品');
    assert.equal(protocolErrorMessage(40005, 3101), protocolErrorMessage(40009, 3101));
    assert.equal(protocolErrorMessage(40009, 3102), '药品使用过快，请稍后再试');
    assert.equal(protocolErrorMessage(40005, 3102), protocolErrorMessage(40009, 3102));
    assert.equal(protocolErrorMessage(40009, 9999), '操作失败，错误码9999');
});
