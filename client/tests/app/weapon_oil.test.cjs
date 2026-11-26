const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, load } = require('./helpers/session_harness.cjs');
const assets = require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const equipment = require('../../../.work/ui-tests/app/EquipmentModel.js');
const ids = load('client/assets/scripts/generated/protocol_ids.ts', () => ({}));
const model = load('client/assets/scripts/app/WeaponOilModel.ts', id => ({
    long: Long, './SkillAssetsModel': assets, './EquipmentModel': equipment,
})[id]);
const { GameWeaponOil } = load('client/assets/scripts/app/GameWeaponOil.ts', id => ({
    './WeaponOilModel': model, './EquipmentModel': equipment, './SkillAssetsModel': assets, '../generated/protocol_ids': ids,
})[id]);
const { PendingOperations } = load('client/assets/scripts/app/PendingOperations.ts', id => id === 'long' ? Long : {});
const pb = require('../../assets/scripts/generated/protobuf/client_pb.js');
const transport = load('client/assets/scripts/app/GameConnection.ts', id => (
    { long: Long, '../generated/protobuf/client_pb.js': pb }[id] ?? {}
));

const u = value => Long.fromString(String(value), true);
const bytes = token => new Uint8Array(16).fill(token);
const weapon = { instanceId: bytes(1), itemId: 218, bound: false, version: u('9007199254741013'),
    quantity: Long.ONE, initialLuck: 3, currentLuck: 5, sourceKind: 'monster_drop', sourceId: '128' };
const spare = { ...weapon, instanceId: bytes(2), initialLuck: 0, currentLuck: 0 };
const armor = { ...weapon, instanceId: bytes(3), itemId: 193 };
const oil = (token, bound, extra = {}) => ({ instanceId: bytes(token), itemId: 900021, version: u('9007199254741022'),
    bound, quantity: Long.fromInt(3), sourceKind: 'initial_grant', sourceId: 'test', ...extra });
const freeOil = oil(4, false), boundOil = oil(5, true);
const catalog = [{ item_id: 218, kind: 'equipment', equip_slot: 0, name: '木剑' },
    { item_id: 193, kind: 'equipment', equip_slot: 1, name: '布衣' }, { item_id: 900021, kind: 'luck_oil', name: '祝福油' }];
const snapshot = { scope: '原场景角色面板', role: '角色一', sceneEpoch: u('9007199254741001'), revision: u('9007199254741031'),
    items: [spare, armor, freeOil, boundOil, oil(6, false, { locked: true })],
    equipped: [{ slot: 0, item: { ...weapon, recycleBlockFlags: 8 } }], catalog, oilItemId: 900021, blocked: false };
const target = { instanceId: weapon.instanceId, equipped: true };
const other = { instanceId: spare.instanceId, equipped: false };
const session = bytes(20);
const roundTrip = (id, value) => transport.decodePacket(transport.encodePacket(id, 1, Long.UONE, value)).value;

function quote(request, from = snapshot, more = {}) {
    const current = model.weaponOilTarget(from, request.weapon);
    const luck = equipment.equipmentCurrentLuck(current);
    const spend = request.allowBound ? boundOil : freeOil;
    return { quoteId: bytes(11), uiSessionId: request.uiSessionId, sceneEpoch: from.sceneEpoch, roleRevision: from.revision,
        expiresAtUnixMs: u('1770000000000'), weapon: { ...current, recycleBlockFlags: 0 }, oil: spend,
        equipped: request.weapon.equipped, allowBound: request.allowBound, currentLuck: luck, successLuck: luck + 1,
        failureLuck: luck < 3 ? luck : luck < 6 ? 3 : 6, successPpm: luck ? 200000 : 1000000,
        pityUsed: luck < 3 ? 0 : 99, pityAfter: luck < 3 ? 0 : luck < 6 ? 100 : 3000,
        guaranteed: false, outputBound: current.bound || spend.bound, ...more };
}

function receipt(quoted, success) {
    return { quoteId: quoted.quoteId, equipped: quoted.equipped, oil: quoted.oil,
        weapon: { ...quoted.weapon, version: quoted.weapon.version.add(1), bound: quoted.outputBound,
            currentLuck: success ? quoted.successLuck : quoted.failureLuck },
        beforeLuck: quoted.currentLuck, success, guaranteed: quoted.guaranteed, pityUsed: quoted.pityUsed + Number(quoted.pityAfter > 0),
        pityAfter: quoted.pityAfter };
}

test('候选目录仅提供原背包和穿戴武器，装备详情仍兼容旧初始幸运', () => {
    assert.equal(model.weaponOilItemId({ item_id: 900021 }), 900021);
    assert.equal(model.weaponOilItemId({ itemId: 900021 }), 0);
    assert.equal(model.weaponOilItemId(undefined), 0);
    assert.equal(equipment.equipmentCurrentLuck({ ...weapon, currentLuck: undefined }), 3);
    assert.equal(equipment.equipmentCurrentLuck(weapon), 5);
    assert.ok(equipment.equipmentDetailLines(catalog[0], weapon, 1, 0, true).includes('幸运+5'));
    assert.deepEqual(model.weaponOilTargets(snapshot).map(row => [row.item.instanceId[0], row.target.equipped]), [[1, true], [2, false]]);
    assert.deepEqual(model.weaponOilAvailable(snapshot), { free: 3n, bound: 3n });
    assert.match(model.weaponOilBlock(snapshot, { instanceId: armor.instanceId, equipped: false }, false), /原武器/);
    const capped = { ...snapshot, equipped: [{ slot: 0, item: { ...weapon, currentLuck: 9 } }] };
    assert.equal(model.weaponOilRequest(capped, target, false, session), undefined);
    const onlyBound = { ...snapshot, items: [spare, boundOil] };
    assert.match(model.weaponOilBlock(onlyBound, target, false), /开启绑材/);
    assert.ok(model.weaponOilRequest(onlyBound, target, true, session));
});

test('报价核对原武器、油堆、绑材优先和保底；阈值当瓶仍可失败至+3', () => {
    const request = roundTrip(40049, model.weaponOilRequest(snapshot, target, true, session));
    assert.equal(request.weapon.version.toString(), weapon.version.toString());
    const original = roundTrip(40050, quote(request));
    model.validateWeaponOilQuote(original, request, snapshot);
    assert.match(model.weaponOilQuoteLines(original, () => '物品').join('\n'), /失败回退至幸运\+3.*\n本次所属阶段 3→6/);
    const commit = model.weaponOilCommit(original);
    const result = roundTrip(40005, { weaponOil: receipt(original, false) }).weaponOil;
    assert.equal(model.weaponOilReceipt(commit, result, original).weapon.currentLuck, 3);
    assert.match(model.weaponOilReceiptLines(result, () => '物品').join('\n'), /累计耗油 100\/100瓶\n下一瓶起本段逐步必成/);
    assert.equal(model.weaponOilReceipt(commit, result).weapon.bound, true);
    for (const changed of [{ ...original, uiSessionId: bytes(21) }, { ...original, outputBound: false },
        { ...original, pityUsed: 100 }, { ...original, oil: freeOil }, { ...original, roleRevision: u(2) }]) {
        assert.throws(() => model.validateWeaponOilQuote(changed, request, snapshot), /重新报价/);
    }
    for (const changed of [{ ...result, weapon: { ...result.weapon, bound: false } }, { ...result, pityUsed: 101 },
        { ...result, weapon: { ...result.weapon, version: weapon.version } }, { ...result, equipped: false }]) {
        assert.throws(() => model.weaponOilReceipt(commit, changed), /原操作不符/);
    }
});

test('第二阶段达到阈值后的每瓶保证增一级，回执仍记录旧阶段耗油', () => {
    const stage = { ...snapshot, equipped: [{ slot: 0, item: { ...weapon, currentLuck: 8 } }] };
    const request = model.weaponOilRequest(stage, target, false, session);
    const fixed = quote(request, stage, { pityUsed: 3000, successPpm: 1000000, guaranteed: true });
    model.validateWeaponOilQuote(fixed, request, stage);
    const result = receipt(fixed, true);
    assert.equal(model.weaponOilReceipt(model.weaponOilCommit(fixed), result, fixed).pityUsed, 3001);
    assert.equal(result.weapon.currentLuck, 9);
    assert.throws(() => model.weaponOilReceipt(model.weaponOilCommit(fixed), receipt(fixed, false)), /原操作不符/);
});

test('提交跨重连保留原界面令牌、武器位置和明确关闭绑材，协议字节不变', () => {
    let text;
    const storage = { getItem: () => text ?? null, setItem: (_key, value) => { text = value; }, removeItem: () => { text = undefined; } };
    const request = model.weaponOilCommit(quote(model.weaponOilRequest(snapshot, other, false, session)));
    const original = { id: 40051, value: { ...request, operation: { opId: bytes(14), commandSeq: u(15), expectedRevision: snapshot.revision,
        expectedRuleVersion: u(30) } } };
    new PendingOperations(storage, 'test.oil').set('role-one', original);
    const retrieved = new PendingOperations(storage, 'test.oil').get('role-one');
    assert.deepEqual(transport.encodePacket(original.id, 7, Long.UONE, retrieved.value), transport.encodePacket(original.id, 7, Long.UONE, original.value));
    assert.equal(roundTrip(40051, retrieved.value).weapon.equipped, false);
    assert.equal(roundTrip(40051, retrieved.value).allowBound, false);
    assert.equal(retrieved.value.weapon.version.toString(), spare.version.toString());
    for (const patch of [{ allowBound: undefined }, { uiSessionId: bytes(2).slice(1) }, { weapon: { ...request.weapon, equipped: undefined } }]) {
        assert.throws(() => new PendingOperations().set('other', { ...original, value: { ...original.value, ...patch } }), { code: 'INVALID_OPERATION' });
    }
});

test('两次绑定确认和换选共用本次界面；报价修订变动和关闭授权都阻止旧提交', async () => {
    let current = snapshot, read, sequence = 20, reviewed;
    const prompts = [], answers = [false, true, false, true, true], requests = [], commits = [];
    const controller = new GameWeaponOil({ snapshot: () => current, newSession: () => bytes(++sequence),
        open: next => { read = next; }, current: next => read === next, refresh() {},
        confirm: async (title, _message, action) => { prompts.push([title, action]); return answers.shift(); },
        quote: async request => { requests.push(request); reviewed = quote(request, current, { quoteId: bytes(10 + requests.length) }); return reviewed; },
        submit: async request => { commits.push(request); controller.accept(current.role, request, controller.reconcile(request, receipt(reviewed, true))); },
    });
    const run = label => read().actions.find(action => action.label === label).run();
    controller.open(target);
    await run('开启绑材');
    await run('开启绑材');
    assert.ok(read().lines.includes('使用绑定资源 未开启'));
    await run('开启绑材');
    assert.deepEqual(prompts.map(row => row[1]), ['继续开启', '继续开启', '确认开启', '继续开启', '确认开启']);
    read().options.find(option => option.enabled).choose();
    await run('查看报价');
    assert.equal(requests[0].allowBound, true);
    assert.equal(requests[0].weapon.equipped, false);
    assert.deepEqual(requests[0].weapon.instanceId, spare.instanceId);
    current = { ...snapshot, revision: snapshot.revision.add(1) };
    await run('重新报价');
    assert.equal(commits.length, 0);
    assert.deepEqual(requests[1].uiSessionId, requests[0].uiSessionId);
    await run('关闭绑材');
    assert.equal(read().title, '武器祝福油');
    await run('查看报价');
    await run('使用祝福油');
    assert.equal(commits.length, 1);
    assert.equal(commits[0].allowBound, false);
    assert.match(read().lines.join('\n'), /结算后装备非绑定/);
    assert.equal(read().actions[0].label, '查看下一瓶');
    controller.open(target);
    assert.ok(read().lines.includes('使用绑定资源 未开启'));
    await run('查看报价');
    assert.notDeepEqual(requests[3].uiSessionId, requests[0].uiSessionId);
});
