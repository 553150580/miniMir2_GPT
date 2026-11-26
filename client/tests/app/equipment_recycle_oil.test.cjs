const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long } = require('./helpers/session_harness.cjs');
const model = require('../../../.work/ui-tests/app/EquipmentRecycleOilModel.js');
const { sourceName } = require('../../../.work/ui-tests/app/SkillAssetsModel.js');

const id = (byte) => new Uint8Array(16).fill(byte);
const rule = { denominator: 30, oilItemId: 900021 };
const sword = { instanceId: id(1), itemId: 218, quantity: Long.ONE, version: Long.UONE, bound: true,
    initialLuck: 3, currentLuck: 8, durabilityU: Long.UONE, maxDurabilityU: Long.fromInt(1000, true),
    originalDurabilityU: Long.fromInt(1000, true) };
const oil = { instanceId: id(2), itemId: rule.oilItemId, quantity: Long.ONE, version: Long.UONE, bound: true,
    sourceKind: 'equipment_recycle', sourceId: '01'.repeat(16) };

test('分解油比例保留精确分数与近似标记，培养后的当前幸运参与分解', () => {
    assert.deepEqual(model.equipmentRecycleOilRule({ denominator: 30 }, { item_id: 900021 }), rule);
    assert.equal(model.equipmentRecycleOilRule(undefined, { item_id: 900021 }), undefined);
    assert.equal(model.equipmentRecycleOilRule({ denominator: 0 }, { item_id: 900021 }), undefined);
    assert.deepEqual([1, 2, 3, 8, 9, 31].map(luck => model.recycleOilPercent(luck, 30)),
        ['约3.33%', '约6.67%', '10%', '约26.67%', '30%', '100%']);
    assert.deepEqual(model.equipmentRecycleOilLines(rule, sword), ['分解出油概率 约26.67%', '命中得1瓶绑定祝福油']);
    const confirm = model.equipmentRecycleOilConfirmation('木剑', { ...sword, currentLuck: 3, bound: false }, rule);
    assert.match(confirm, /当前幸运\+3 · 非绑定\n分解出油概率 10%\n命中得1瓶非绑定祝福油/);
    assert.match(confirm, /分解不可撤销/);
    assert.equal(sourceName('equipment_recycle'), '装备分解');
});

test('破损有效耐久的分解金币为0且无油，未知耐久与受保护装备不会冒充可抽样', () => {
    assert.deepEqual(model.equipmentRecycleOilLines(rule, { ...sword, durabilityU: Long.UZERO }),
        ['破损分解金币 0', '破损装备不判定祝福油']);
    assert.match(model.equipmentRecycleOilLines(rule, { ...sword, originalDurabilityU: Long.UZERO })[0], /持久未确认/);
    assert.match(model.equipmentRecycleOilLines(rule, sword, '装备已锁定')[0], /当前不能分解出油/);
    assert.match(model.equipmentRecycleOilLines(rule, { ...sword, initialLuck: 0, currentLuck: 0 })[0], /幸运\+0/);
});

test('只显示原件真实有资格尝试的结果，拒绝串单、重复产物、掉包和其他来源', () => {
    const miss = { equipmentInstanceId: id(3), luck: 1, oil: null };
    const hit = { equipmentInstanceId: sword.instanceId, luck: 8, oil };
    assert.deepEqual(model.validateRecycleOilReceipt([id(1)], undefined), []);
    const rows = model.validateRecycleOilReceipt([id(1), id(3)], { rolls: [hit, miss] }, rule.oilItemId);
    assert.equal(model.recycleOilResultSummary(rows), '分解获得1瓶祝福油');
    assert.equal(model.recycleOilResultSummary([miss]), '本次未获得祝福油');
    assert.match(model.recycleOilResultLines(rows).join('\n'), /获得1瓶绑定祝福油\n来源 装备分解/);
    assert.throws(() => model.validateRecycleOilReceipt([id(1)], { rolls: [miss] }), /不匹配/);
    assert.throws(() => model.validateRecycleOilReceipt([id(1), id(3)], { rolls: [hit, hit] }), /不匹配/);
    assert.throws(() => model.validateRecycleOilReceipt([id(1), id(2)], { rolls: [hit] }), /不匹配/);
    for (const badOil of [{ ...oil, instanceId: sword.instanceId }, { ...oil, sourceId: '03'.repeat(16) },
        { ...oil, sourceKind: 'initial_grant' }, { ...oil, quantity: Long.fromInt(2) }, { ...oil, itemId: 333 }]) {
        assert.throws(() => model.validateRecycleOilReceipt([id(1)], { rolls: [{ ...hit, oil: badOil }] }, rule.oilItemId), /不匹配/);
    }
});
