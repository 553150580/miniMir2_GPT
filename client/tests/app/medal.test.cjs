const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, harness, ready, entry, load } = require('./helpers/session_harness.cjs');
const model = require('../../../.work/ui-tests/app/MedalModel.js');
const assets = require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const ids = load('client/assets/scripts/generated/protocol_ids.ts', () => ({}));
const { GameMedal } = load('client/assets/scripts/app/GameMedal.ts', name => ({
    '../generated/protocol_ids': ids, './MedalModel': model, './SkillAssetsModel': assets,
})[name]);
const { PendingOperations } = load('client/assets/scripts/app/PendingOperations.ts', name => name === 'long' ? Long : {});
const pb = require('../../assets/scripts/generated/protobuf/client_pb.js');
const { encodePacket, decodePacket } = load('client/assets/scripts/app/GameConnection.ts', name => ({
    long: Long, '../generated/protobuf/client_pb.js': pb,
})[name] ?? {});

const u = value => Long.fromString(String(value), true);
const s = value => Long.fromString(String(value));
const bytes = value => new Uint8Array(16).fill(value);
const rulesInput = { fragment_item_id: 900061, levels: [
    { level: 1, fragment_cost: 2, main_attack: 1, defense: 1 },
    { level: 2, fragment_cost: 3, main_attack: 2, defense: 1 },
    { level: 3, fragment_cost: 5, main_attack: 3, defense: 2 },
] };
const rules = model.medalRules(rulesInput);
const version = u('9007199254740997');
const free = { instanceId: bytes(1), version, itemId: rules.fragmentItemId,
    quantity: s(1), bound: false, sourceKind: 'world_boss', sourceId: '2' };
const bound = { ...free, instanceId: bytes(2), quantity: s(10), bound: true };
const snapshot = { scope: 'original-scene', role: 'original-role', roleLevel: 2, sceneEpoch: u(70),
    revision: version, rules, inventory: [free, bound], blocked: false };
const sessionId = bytes(8);
const quoteId = bytes(9);
const transport = (id, value) => decodePacket(encodePacket(id, 1, u(11), value)).value;
const quoteFor = (request, current = snapshot) => {
    const next = model.nextMedalLevel(current);
    return { quoteId, sceneEpoch: request.sceneEpoch, medal: current.medal,
        targetLevel: next.level, targetMainAttack: s(next.mainAttack), targetDefense: s(next.defense), allowBound: request.allowBound,
        expiresAtUnixMs: u('9223372036854775807'), roleRevision: current.revision, uiSessionId: request.uiSessionId,
        fragments: request.fragments.map(row => ({ ...row, bound: current.inventory.find(item =>
            model.sameMedalId(item.instanceId, row.instanceId)).bound })) };
};
const growth = quote => ({ quoteId: quote.quoteId, beforeLevel: quote.medal?.level ?? 0,
    medal: { level: quote.targetLevel, version: (quote.medal?.version ?? Long.UZERO).add(Long.UONE) }, fragments: quote.fragments });

test('勋章配置需要完整档位和安全整数，实际角色等级限制可用目标', () => {
    assert.equal(model.medalRules(undefined), undefined);
    assert.equal(model.medalRules({ ...rulesInput, levels: [rulesInput.levels[1]] }), undefined);
    assert.equal(model.medalRules({ ...rulesInput, levels: [{ ...rulesInput.levels[0], fragment_cost: Number.MAX_VALUE }] }), undefined);
    assert.equal(model.medalSlotLabel(), '勋章 · 未激活');
    assert.equal(model.nextMedalLevel(snapshot).cost, 2n);
    assert.equal(model.nextMedalLevel({ ...snapshot, medal: { level: 2, version: u(2) } }), undefined);
    assert.equal(model.nextMedalLevel({ ...snapshot, roleLevel: 4, medal: { level: 3, version: u(3) } }), undefined);
});

test('选择原碎片、绑定授权和实例版本，校验相同顺序的真实大整数报价和回执', () => {
    const select = new model.MedalFragmentSelection();
    assert.deepEqual(select.options(snapshot), [free]);
    assert.deepEqual(select.inventoryTotals(snapshot), { free: 1n, bound: 10n });
    select.pick(snapshot, bound);
    select.pick(snapshot, free);
    assert.equal(select.request(snapshot, sessionId), undefined);
    select.setAllowBound(true);
    select.pick(snapshot, bound);
    const request = transport(40061, select.request(snapshot, sessionId));
    assert.equal(request.medalVersion.toString(), '0');
    assert.deepEqual(request.fragments.map(row => [row.instanceId[0], row.version.toString(), row.quantity.toString()]),
        [[1, '9007199254740997', '1'], [2, '9007199254740997', '1']]);
    const quote = transport(40062, quoteFor(request));
    assert.doesNotThrow(() => model.validateMedalQuote(quote, request, { ...snapshot, revision: version.add(1) }));
    assert.throws(() => model.validateMedalQuote({ ...quote, fragments: quote.fragments.slice().reverse() }, request, snapshot), /重新报价/);
    assert.throws(() => model.validateMedalQuote({ ...quote, targetDefense: u(1) }, request, snapshot), /重新报价/);
    const changed = { ...snapshot, inventory: [free, { ...bound, version: version.add(1) }] };
    assert.equal(select.request(changed, sessionId), undefined);
    assert.throws(() => model.validateMedalQuote(quote, request, changed), /重新报价/);
    select.setAllowBound(false);
    assert.deepEqual(select.selectedTotals(snapshot), { free: 1n, bound: 0n });
    const original = { quoteId, uiSessionId: sessionId };
    const receipt = transport(40005, { medal: growth(quote) }).medal;
    assert.equal(model.validateMedalReceipt(original, receipt, quote).medal.level, 1);
    assert.equal(model.validateMedalReceipt(original, receipt).fragments.length, 2);
    assert.throws(() => model.validateMedalReceipt(original, { ...receipt, fragments: [...receipt.fragments, receipt.fragments[0]] }), /回执不匹配/);
    assert.throws(() => model.validateMedalReceipt(original, { ...receipt, quoteId: bytes(7) }), /回执不匹配/);
});

test('勋章未决账本重开保存原报价和原操作字节，不接收额外材料字段', () => {
    const data = new Map();
    const storage = { getItem: key => data.get(key) ?? null, setItem: (key, val) => data.set(key, val), removeItem: key => data.delete(key) };
    const operation = { opId: bytes(3), commandSeq: u(4), expectedRevision: version, expectedRuleVersion: u(30) };
    const value = { quoteId, uiSessionId: sessionId, operation };
    const ledger = new PendingOperations(storage, '勋章未决');
    ledger.set(snapshot.role, { id: 40063, value });
    const restored = new PendingOperations(storage, '勋章未决').get(snapshot.role);
    assert.deepEqual(encodePacket(40063, 1, u(11), restored.value), encodePacket(40063, 1, u(11), value));
    assert.throws(() => new PendingOperations().set(snapshot.role, { id: 40063, value: { ...value, allowBound: false } }),
        { code: 'INVALID_OPERATION' });
});

test('保存物品弹窗支持绑定二次授权、原报价提交、回执后更新下一档和上限', async () => {
    let live = snapshot, read, submit, receipt;
    const approvals = [true, false, true, true], confirmations = [];
    const game = new GameMedal({ snapshot: () => live, open: value => { read = value; }, current: value => value === read,
        newSession: () => sessionId, refresh() {}, confirm: async title => { confirmations.push(title); return approvals.shift(); },
        quote: async request => quoteFor(request, live),
        submit: async (request, revision) => {
            assert.equal(revision, live.revision);
            submit = request;
            live = { ...live, medal: receipt.medal };
            game.accept(live.role, request, game.reconcile(request, receipt));
        } });
    game.open();
    assert.equal(read().title, '角色勋章');
    assert(read().lines.includes('下一目标 1级 · 2片'));
    await read().actions[0].run();
    assert.equal(read().title, '勋章碎片');
    await read().actions[1].run();
    assert.equal(read().options.length, 1);
    await read().actions[1].run();
    assert.equal(read().options.length, 2);
    assert.deepEqual(confirmations, ['允许绑定碎片', '再次确认绑定投入', '允许绑定碎片', '再次确认绑定投入']);
    read().options[0].choose();
    read().options[1].choose();
    await read().actions[0].run();
    assert.equal(read().title, '勋章升级报价');
    assert(read().lines.some(line => /^1\. 非绑碎片 1片/.test(line)));
    assert(read().lines.some(line => /^2\. 绑定碎片 1片/.test(line)));
    receipt = { quoteId, beforeLevel: 0, medal: { level: 1, version: u(1) },
        fragments: [{ instanceId: free.instanceId, version, quantity: s(1), bound: false },
            { instanceId: bound.instanceId, version, quantity: s(1), bound: true }] };
    await read().actions[0].run();
    assert.deepEqual(submit, { quoteId, uiSessionId: sessionId });
    assert.equal(read().title, '勋章激活完成');
    assert.equal(read().actions[0].label, '继续升级');
    game.open();
    assert(read().lines.includes('下一目标 2级 · 3片'));
    await read().actions[0].run();
    assert.equal(read().options.length, 1, '重新打开默认关闭绑定投入');
    live = { ...live, medal: { level: 2, version: u(2) } };
    game.open();
    assert(read().lines.includes('角色需达 3级'));
    assert.equal(read().actions[0].enabled, false);
});

test('库存保持最新角色勋章，已经激活后拒绝异代等级和空状态', () => {
    const h = harness();
    ready(h, entry());
    const socket = h.session.connection, medal = { level: 2, version: u(2) };
    socket.deliver(40003, { items: [], revision: u(10), medal });
    assert.equal(h.session.inventory.medal, medal);
    for (const invalid of [null, { level: 1, version: u(2) }, { level: 2, version: u(5) }, { level: 3, version: u(1) }]) {
        socket.deliver(40003, { items: [], revision: u(11), medal: invalid });
        assert.equal(h.session.inventory.medal, medal);
    }
});
