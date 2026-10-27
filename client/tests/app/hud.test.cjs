const { test } = require('node:test');
const assert = require('node:assert/strict');
const { hudInteger, hudAmount, hudRatio, miniMapPoint, createHudState } = require('../../../.work/ui-tests/app/HudModel.js');
const value = text => ({ toString: () => text });

test('四钱包保留64位精度，元宝按十分单位显示，缩写不先转Number', () => {
    assert.equal(hudAmount(value('9223372036854775807')), '9,223,372,036,854,775,807');
    assert.equal(hudAmount(value('9223372036854775807'), 1), '922,337,203,685,477,580.7');
    assert.equal(hudAmount(value('1130'), 1), '113.0');
    assert.equal(hudAmount(value('16648496'), 0, true), '1664.8万');
    assert.equal(hudAmount(value('100005'), 1, true), '1万');
    assert.equal(hudAmount(value('99999'), 1, true), '9,999.9');
    assert.equal(hudAmount(undefined), '—');
    assert.throws(() => hudInteger(Number.MAX_SAFE_INTEGER + 1));
});

test('血蓝经验条采用大整数计算，空数据、零上限和越界状态均有界', () => {
    assert.equal(hudRatio(value('4611686018427387903'), value('9223372036854775807')), .4999);
    assert.equal(hudRatio(1, 3), .3333);
    for (const [current, total, expected] of [[0, 0, 0], [10, 0, 0], [-1, 10, 0], [20, 10, 1], [null, 10, 0]]) {
        assert.equal(hudRatio(current, total), expected);
    }
});

function sample() {
    return {
        role: { name: '测试道士', job: 2, level: 1 },
        wallet: { gold: value('123'), boundGold: value('456'), yuanbaoUnits: value('27'), boundYuanbaoUnits: value('38') },
        inventory: { level: 3, experience: value('12'), items: [
            { itemId: 35, quantity: value('2') }, { itemId: 35, quantity: value('3') },
            { itemId: 36, quantity: value('4') }, { itemId: 218, quantity: value('99') },
        ] },
        baseline: { entities: [{ entityKind: 1, position: { x: 8, y: 7 }, hp: value('14'), hpMax: value('16'), mp: value('10'), mpMax: value('20') }] },
        ready: true, auto: true, count: 4,
        gameplay: { jobs: [{ job_id: 2, name: '道士' }], items: [{ item_id: 35, kind: 'potion' }, { item_id: 36, kind: 'potion' }], experience: [{ level: 3, next_level_exp: 30 }] },
        map: { name: '开发地图0局部', width: 16, height: 16 },
    };
}

test('HUD使用同步等级、实际背包药品总数和四钱包独立余额', () => {
    const state = createHudState(sample());
    assert.equal(state.level, '3级');
    assert.equal(state.job, '道士');
    assert.equal(state.gold, '绑金 456'); assert.equal(state.yuanbao, '绑元 3.8');
    assert.equal(state.walletDetails, '金币 123\n绑定金币 456\n元宝 2.7\n绑定元宝 3.8');
    assert.equal(state.redPotionCount, '5'); assert.equal(state.bluePotionCount, '4');
    assert.equal(state.experienceText, '12 / 30'); assert.equal(state.experience, .4);
    assert.equal(state.hpText, '14/16'); assert.equal(state.mp, .5);
    assert.equal(state.objective, '自动战斗 · 4只怪物');
});

test('没有配置或同步数据时显示待同步，缺经验下一档不假称满级', () => {
    const input = sample(); input.inventory = null; input.wallet = null; input.baseline = null; input.ready = false;
    const state = createHudState(input);
    assert.equal(state.redPotionCount, '—'); assert.equal(state.gold, '绑金 —');
    assert.equal(state.hpText, '—/—'); assert.equal(state.experienceText, '经验 —');
    assert.equal(state.objective, '场景同步中');
    input.inventory = { level: 99, experience: value('100'), items: [] };
    assert.equal(createHudState(input).experienceText, '经验 100');
});

test('手动与死亡展示服从当前状态，不把目标区伪装成已接入任务', () => {
    const input = sample(); input.auto = false;
    assert.equal(createHudState(input).objective, '手动战斗 · 4只怪物');
    input.baseline.entities[0].hp = value('0');
    const state = createHudState(input);
    assert.equal(state.attack, '复活'); assert.equal(state.hp, 0);
    assert.equal(state.objective, '角色已倒下 · 点击复活');
});

test('小地图按实际贴图矩形映射格子中心，Y轴翻转且适配锚点', () => {
    const map = { width: 16, height: 16 };
    assert.deepEqual(miniMapPoint({ x: 0, y: 0 }, map, 136, 96), { x: -63.75, y: 45 });
    assert.deepEqual(miniMapPoint({ x: 15, y: 15 }, map, 136, 96), { x: 63.75, y: -45 });
    assert.deepEqual(miniMapPoint({ x: 8, y: 7 }, map, 136, 96), { x: 4.25, y: 3 });
    assert.deepEqual(miniMapPoint({ x: 0, y: 0 }, map, 136, 96, 0, 1), { x: 4.25, y: -3 });
    assert.equal(miniMapPoint(null, map, 136, 96), null);
    assert.equal(miniMapPoint({ x: NaN, y: 1 }, map, 136, 96), null);
    assert.equal(miniMapPoint({ x: 0, y: 0 }, map, 0, 96), null);
});
