// 核对打造绑定授权、原矿石版本、权威报价和永久结果，不使用模拟成功率替代服务端。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, load } = require('./helpers/session_harness.cjs');
const ids = load('client/assets/scripts/generated/protocol_ids.ts', () => ({}));
const assets = require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const model = load('client/assets/scripts/app/ForgeModel.ts', id => ({ long: Long, './SkillAssetsModel': assets })[id]);
const { GameForge } = load('client/assets/scripts/app/GameForge.ts', id => ({
    './ForgeModel': model, './SkillAssetsModel': assets, '../generated/protocol_ids': ids,
})[id]);
const pb = require('../../assets/scripts/generated/protobuf/client_pb.js');
const connection = load('client/assets/scripts/app/GameConnection.ts', id => ({
    long: Long, '../generated/protobuf/client_pb.js': pb,
})[id] ?? {});
const u = value => Long.fromString(String(value), true);
const s = value => Long.fromString(String(value));
const bytes = value => new Uint8Array(16).fill(value);
const packageKey = '角色一矿区一';
const material = (instance, bound, purity, extra = {}) => ({ instanceId: bytes(instance), itemId: 910004, quantity: Long.ONE,
    version: u('9007199254741001'), bound, orePurity: purity, sourceKind: 'mining', sourceId: 'a'.repeat(32), ...extra });
const blueprint = { instanceId: bytes(40), itemId: 910021, quantity: Long.ONE, version: u(19), bound: true, sourceKind: 'personal_boss' };
const ores = [material(1, true, 90), material(2, false, 100), material(3, true, 95), material(4, false, 95)];
const recipe = { blueprint_item_id: 910021, output_item_id: 910022, ore_item_id: 910004, ore_count: 4, quality: 2 };
const snapshot = { scope: packageKey, role: '角色一', sceneEpoch: u('9007199254740999'), revision: u('9007199254741002'), blocked: false,
    catalog: [{ item_id: 910021, name: '矿纹项链图纸', kind: 'forge_blueprint' }, { item_id: 910004, name: '黑铁矿', kind: 'mining_ore' },
        { item_id: 910022, name: '矿纹项链', kind: 'equipment', equip_slot: 3, required_level: 20 }], recipes: [recipe],
    items: [blueprint, ...ores, material(5, false, 90, { locked: true }), material(6, false, 51, { recycleBlockFlags: 4 }),
        material(7, false, 101), material(8, false, 90, { sourceId: '' })] };

function quote(rev = snapshot.revision, id = 9) {
    return { quoteId: bytes(id), sceneEpoch: snapshot.sceneEpoch, roleRevision: rev, expiresAtUnixMs: u(1770000000000), blueprint, ores,
        allowBound: true, outputItemId: 910022, quality: 2, requiredLevel: 20, slot: 3, successPpm: 1000000,
        gold: s('9007199254741010'), boundGold: s(8), outputBound: true };
}

test('只选择合法原黑铁矿，切换绑定与原实例变更立即撤销对应选择', () => {
    const selection = new model.ForgeSelection(packageKey, blueprint, recipe);
    assert.equal(model.forgeRecipeFor({ ...snapshot, recipes: [] }, blueprint.itemId), undefined);
    assert.deepEqual(selection.options(snapshot), [ores[1], ores[3]]);
    selection.toggle(snapshot, ores[0]);
    assert.equal(selection.chosen(snapshot).length, 0);
    selection.setAllowBound(snapshot, true);
    for (const ore of ores) selection.toggle(snapshot, ore);
    const request = selection.request(snapshot);
    assert.equal(request.blueprint.version.toString(), '19');
    assert.equal(request.ores[0].version.toString(), '9007199254741001');
    assert.deepEqual(request.ores.map(ore => ore.instanceId[0]), [1, 2, 3, 4]);
    assert.equal(selection.request({ ...snapshot, items: [blueprint, ...ores.slice(1), { ...ores[0], version: u(1) }] }), undefined);
    selection.setAllowBound(snapshot, false);
    assert.deepEqual(selection.chosen(snapshot), [ores[1], ores[3]]);
    assert.equal(selection.request(snapshot), undefined);
    assert.equal(selection.blueprint({ ...snapshot, scope: '新附着' }), undefined);
});

test('打造协议按原矿石和 Long 往返，拒绝改绑、重矿、改价或旧角色修订', () => {
    const selection = new model.ForgeSelection(packageKey, blueprint, recipe);
    selection.setAllowBound(snapshot, true);
    for (const ore of ores) selection.toggle(snapshot, ore);
    const req = connection.decodePacket(connection.encodePacket(40046, 1, Long.UONE, selection.request(snapshot))).value;
    assert.equal(req.ores[0].version.toString(), ores[0].version.toString());
    assert.deepEqual(new Uint8Array(req.ores[0].instanceId), ores[0].instanceId);
    const originalQuote = quote();
    const result = connection.decodePacket(connection.encodePacket(40047, 1, Long.UONE, originalQuote)).value;
    model.validateForgeQuote(result, selection, snapshot);
    assert.match(model.forgeQuoteLines(result, () => '矿纹项链').join('\n'), /成功率 100% · 产物绑定/);
    assert.match(model.forgeQuoteLines(result, () => '矿纹项链').join('\n'), /金币 9007199254741010/);
    for (const changed of [quote(u(1)), { ...quote(), outputBound: false }, { ...quote(), ores: [ores[0], ores[0], ores[2], ores[3]] },
        { ...quote(), ores: [{ ...ores[0], orePurity: 91 }, ...ores.slice(1)] }, { ...quote(), allowBound: false }]) {
        assert.throws(() => model.validateForgeQuote(changed, selection, snapshot), /重新报价/);
    }
    const failed = { quoteId: bytes(9), success: false, gold: originalQuote.gold, boundGold: originalQuote.boundGold };
    assert.match(model.forgeReceiptLines(model.forgeReceipt(bytes(9), failed, originalQuote), () => '').join('\n'), /原图纸保留/);
    assert.throws(() => model.forgeReceipt(bytes(8), failed), /原订单/);
    assert.throws(() => model.forgeReceipt(bytes(9), { ...failed, gold: s(2) }, originalQuote), /报价不同/);
    const output = { instanceId: bytes(56), itemId: 910022, bound: true, quality: 2, initialLuck: 3 };
    const success = { ...failed, success: true, output };
    assert.match(model.forgeReceiptLines(model.forgeReceipt(bytes(9), success, originalQuote), () => '矿纹项链').join('\n'), /幸运\+3/);
});

test('绑定投入需两次确认，拒绝旧报价提交，重开默认关闭', async () => {
    let current = snapshot, read, controller, confirmationHook = () => true;
    const confirmCalls = [], requests = [], spends = [];
    controller = new GameForge({ snapshot: () => current, open: next => { read = next; }, current: next => read === next,
        refresh() {}, confirm: async (title, message, label) => { confirmCalls.push({ title, message, label }); return confirmationHook(); },
        quote: async value => { requests.push(value); return quote(current.revision, 8 + requests.length); },
        submit: async quoteId => {
            spends.push(quoteId);
            const output = { instanceId: bytes(56), itemId: 910022, bound: true, quality: 2, initialLuck: 3 };
            controller.accept(snapshot.role, controller.reconcile(quoteId, { quoteId, success: true, output,
                gold: s('9007199254741010'), boundGold: s(8) }));
        } });
    controller.open(blueprint);
    assert.deepEqual(read().options.map(row => row.key.slice(-2)), ['02', '04']);
    confirmationHook = () => confirmCalls.length === 1;
    await read().actions[1].run();
    assert.deepEqual(confirmCalls.map(call => call.label), ['继续开启', '确认开启']);
    assert.deepEqual(read().options.map(row => row.key.slice(-2)), ['02', '04']);
    assert.equal(requests.length, 0);
    confirmationHook = () => true;
    await read().actions[1].run();
    for (const option of read().options) option.choose();
    await read().actions[0].run();
    assert.equal(requests.length, 1);
    assert.match(read().lines.join('\n'), /失败保留图纸/);
    assert.equal(spends.length, 0);
    confirmationHook = () => { current = { ...snapshot, revision: snapshot.revision.add(1) }; return true; };
    await read().actions[0].run();
    assert.equal(requests.length, 2);
    assert.equal(spends.length, 0);
    confirmationHook = () => true;
    await read().actions[0].run();
    assert.deepEqual(spends, [bytes(10)]);
    assert.equal(read().title, '打造成功');
    controller.open(blueprint);
    assert.match(read().lines.join('\n'), /使用绑定资源 未开启/);
    assert.deepEqual(read().options.map(row => row.key.slice(-2)), ['02', '04']);
});
