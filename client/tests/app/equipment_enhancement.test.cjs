const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, load } = require('./helpers/session_harness.cjs');
const assets = require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const equipment = require('../../../.work/ui-tests/app/EquipmentModel.js');
const ids = load('client/assets/scripts/generated/protocol_ids.ts', () => ({}));
const model = load('client/assets/scripts/app/EquipmentEnhancementModel.ts', path => ({
    long: Long, './SkillAssetsModel': assets, './EquipmentModel': equipment,
})[path]);
const { GameEquipmentEnhancement } = load('client/assets/scripts/app/GameEquipmentEnhancement.ts', path => ({
    '../generated/protocol_ids': ids, './SkillAssetsModel': assets, './EquipmentModel': equipment, './EquipmentEnhancementModel': model,
})[path]);
const { PendingOperations } = load('client/assets/scripts/app/PendingOperations.ts', path => path === 'long' ? Long : {});
const pb = require('../../assets/scripts/generated/protobuf/client_pb.js');
const transport = load('client/assets/scripts/app/GameConnection.ts', path => ({ long: Long, '../generated/protobuf/client_pb.js': pb })[path] ?? {});

const identity = number => new Uint8Array(16).fill(number);
const u = value => Long.fromString(String(value), true);
const signed = value => Long.fromString(String(value));
const epoch = u('9007199254741001'), revision = u('9007199254741013'), version = u('9007199254741021');
const stoneItemId = 900031;
const levels = [1_000_000, 900_000, 800_000].map((success_ppm, current_level) => ({ current_level, stone_cost: 10, success_ppm, bonus_bp: 10 }));
const rules = model.equipmentEnhancementRules({ stone_item_id: stoneItemId, levels });
const rawSlots = Array.from({ length: 10 }, (_, slot) => ({ slot, level: slot === 0 ? 1 : 0 }));
const slots = model.observedEnhancementSlots(rawSlots, rules);
const free = { instanceId: identity(2), itemId: stoneItemId, version, quantity: signed(10), bound: false, sourceKind: 'personal_boss', sourceId: 'a' };
const bound = { ...free, instanceId: identity(3), quantity: signed(6), bound: true, sourceId: 'b' };
const weapon = { ...free, instanceId: identity(4), itemId: 218, quantity: signed(1), bound: false };
const state = { scope: '角色场景', role: 'role-a', sceneEpoch: epoch, revision, level: 3, rules, slots, items: [free, bound],
    equipped: [{ slot: 0, item: weapon }], catalog: [{ item_id: 218, name: '木剑' }], blocked: false };
const uiSession = identity(5), quoteId = identity(6);
const roundTrip = (id, value) => transport.decodePacket(transport.encodePacket(id, 8, Long.UONE, value)).value;

function quotation(request, snapshot = state) {
    const currentLevel = snapshot.slots.get(request.slot);
    return { quoteId, sceneEpoch: request.sceneEpoch, slot: request.slot, currentLevel, targetLevel: currentLevel + 1, currentBonusBp: currentLevel * 10,
        targetBonusBp: (currentLevel + 1) * 10, successPpm: levels[currentLevel].success_ppm, stones: request.stones.map(ref => ({
            item: snapshot.items.find(item => assets.assetId(item.instanceId) === assets.assetId(ref.instanceId)), quantity: ref.quantity,
        })), allowBound: request.allowBound, expiresAtUnixMs: u(2000000000000), roleRevision: snapshot.revision, uiSessionId: request.uiSessionId };
}

function outcome(quote, success) {
    return { quoteId: quote.quoteId, slot: quote.slot, beforeLevel: quote.currentLevel, afterLevel: quote.currentLevel + Number(success), success, stones: quote.stones };
}

test('部位强化只接受十个原部位，空槽、换装和等级上限可按权威状态显示', () => {
    assert.equal(model.equipmentEnhancementRules(undefined), undefined);
    assert.equal(model.equipmentEnhancementRules({ stone_item_id: stoneItemId, levels: [levels[1]] }), undefined);
    assert.equal(model.observedEnhancementSlots([...rawSlots.slice(0, -1), { slot: 100, level: 0 }], rules), undefined);
    assert.equal(model.observedEnhancementSlots([...rawSlots.slice(0, -1), rawSlots[0]], rules), undefined);
    assert.match(model.enhancementSlotLines(state, 0).join('\n'), /\+0\.1%.*\n当前穿戴 木剑/);
    assert.match(model.enhancementSlotLines({ ...state, equipped: [] }, 0).join('\n'), /武器强化 \+1.*\n.*\n当前为空部位，穿戴后生效/);
    assert.equal(model.enhancementNext({ ...state, level: 1 }, 0), undefined);
    assert.equal(model.enhancementNext(state, 100), undefined);
    assert.equal(model.enhancementNext(state, 1).successPpm, 1_000_000);
});

test('只报价选择的原石堆，保留绑定权限、64 位版本与当次实际配比', () => {
    assert.deepEqual(model.enhancementStock(state), { free: 10n, bound: 6n });
    const selection = new model.EquipmentEnhancementSelection();
    assert.deepEqual(selection.options(state), [free]);
    selection.pick(state, 0, bound);
    assert.equal(selection.request(state, 0, uiSession), undefined);
    selection.allowBound = true;
    selection.pick(state, 0, bound);
    selection.pick(state, 0, free);
    const request = roundTrip(40055, selection.request(state, 0, uiSession)), quote = roundTrip(40056, quotation(request));
    assert.deepEqual(request.stones.map(stone => stone.quantity.toString()), ['6', '4']);
    assert.ok(quote.roleRevision.equals(revision));
    assert.ok(request.stones[0].version.equals(version));
    assert.doesNotThrow(() => model.validateEquipmentEnhancementQuote(quote, request, { ...state, revision: revision.add(1) }));
    const wrong = { ...quote, successPpm: quote.successPpm + 1 };
    assert.throws(() => model.validateEquipmentEnhancementQuote(wrong, request, state), /重新报价/);
    assert.throws(() => model.validateEquipmentEnhancementQuote(quote, { ...request, allowBound: false }, state), /重新报价/);
    const changed = { ...state, items: [{ ...free, version: version.add(1) }, bound] };
    assert.equal(selection.changed(changed), true);
    assert.throws(() => model.validateEquipmentEnhancementQuote(quote, request, changed), /重新报价/);
    assert.match(model.enhancementQuoteLines(quote).join('\n'), /基础属性加成 \+0\.1% → \+0\.2%/);
});

test('强化回执保持原报价号，失败保留等级且结算同步需要原材料实际变化', () => {
    const selection = new model.EquipmentEnhancementSelection();
    selection.pick(state, 0, free);
    const quote = quotation(selection.request(state, 0, uiSession));
    const request = model.enhancementCommit(quote);
    const failed = roundTrip(40005, { equipmentEnhancement: outcome(quote, false) }).equipmentEnhancement;
    assert.equal(model.validateEquipmentEnhancementReceipt(request, failed, quote).success, false);
    assert.equal(model.validateEquipmentEnhancementReceipt(request, failed).afterLevel, 1);
    assert.equal(model.enhancementReceiptObserved({ ...state, revision: revision.add(1) }, failed, revision), false);
    assert.equal(model.enhancementReceiptObserved({ ...state, revision: revision.add(1), items: [bound] }, failed, revision), true);
    assert.throws(() => model.validateEquipmentEnhancementReceipt(request, { ...failed, afterLevel: 0 }, quote), /强化回执/);
    assert.throws(() => model.validateEquipmentEnhancementReceipt(request, { ...failed, quoteId: identity(9) }, quote), /强化回执/);
    assert.throws(() => model.validateEquipmentEnhancementReceipt(request, { ...failed, stones: [...failed.stones, ...failed.stones] }, quote), /强化回执/);
    assert.throws(() => model.validateEquipmentEnhancementReceipt(request, failed, { ...quote, successPpm: 1_000_000 }), /强化回执/);
});

test('原提交能通过 protobuf 和本地未决恢复，不存报价旁路或替代槽位', () => {
    const content = new Map(), storage = { getItem: key => content.get(key) ?? null, setItem: (key, value) => content.set(key, value),
        removeItem: key => content.delete(key) };
    const operation = { opId: identity(7), commandSeq: u(25), expectedRevision: revision, expectedRuleVersion: u(30) };
    const value = { quoteId, uiSessionId: uiSession, slot: 0, operation };
    const pending = new PendingOperations(storage);
    pending.set('role-a', { id: 40057, value });
    const recovered = new PendingOperations(storage).get('role-a');
    assert.deepEqual(recovered, { id: 40057, value });
    assert.ok(roundTrip(40057, recovered.value).operation.expectedRevision.equals(revision));
    assert.equal(roundTrip(40057, recovered.value).slot, 0);
    assert.throws(() => pending.set('role-b', { id: 40057, value: { ...value, slot: 100 } }), /普通装备槽/);
    assert.throws(() => pending.set('role-b', { id: 40057, value: { ...value, allowBound: true } }), /未允许/);
    const decodedSlots = roundTrip(40003, { equipmentEnhancementSlots: rawSlots }).equipmentEnhancementSlots;
    assert.deepEqual(Array.from(model.observedEnhancementSlots(decodedSlots, rules)), Array.from(slots));
});

function harness() {
    let read, snapshot = state;
    const confirmations = [], confirmationsRead = [], commits = [];
    const game = new GameEquipmentEnhancement({ snapshot: () => snapshot, newSession: () => uiSession,
        open: next => { read = next; }, current: target => read === target, refresh: () => {},
        confirm: async (title, text) => { confirmationsRead.push({ title, text }); return confirmations.shift() ?? false; },
        quote: async request => quotation(request, snapshot), submit: async (request, expectedRevision) => { commits.push({ request, expectedRevision }); } });
    return { game, confirmations, confirmationsRead, commits, read: () => read(), update: next => { snapshot = next; } };
}

test('保存的强化面板走两次绑定授权，换槽保留，重新打开重置', async () => {
    const ui = harness();
    ui.game.open(0);
    assert.equal(ui.read().title, '武器强化');
    assert.equal(ui.read().options.length, 1);
    ui.confirmations.push(true, false);
    await ui.read().actions[1].run();
    assert.equal(ui.read().options.length, 1);
    ui.confirmations.push(true, true);
    await ui.read().actions[1].run();
    assert.equal(ui.read().options.length, 2);
    assert.match(ui.confirmationsRead[3].text, /失败仍扣除/);
    await ui.read().actions[2].run();
    assert.equal(ui.read().options.length, 10);
    ui.read().options[1].choose();
    assert.equal(ui.read().title, '衣服强化');
    assert.equal(ui.read().options.length, 2);
    ui.game.open(2);
    assert.equal(ui.read().title, '头盔强化');
    assert.equal(ui.read().options.length, 1);
});

test('界面按原报价修订提交，同一角色的原扣石同步后才可再操作', async () => {
    const ui = harness();
    ui.game.open(0);
    ui.read().options[0].choose();
    await ui.read().actions[0].run();
    assert.equal(ui.read().title, '强化报价');
    ui.update({ ...state, revision: revision.add(1) });
    assert.equal(ui.read().actions[0].enabled, true);
    await ui.read().actions[0].run();
    const commit = ui.commits[0];
    assert.ok(commit.expectedRevision.equals(revision));
    const selection = new model.EquipmentEnhancementSelection();
    selection.pick(state, 0, free);
    const receipt = outcome(quotation(selection.request(state, 0, uiSession)), false);
    ui.game.accept(state.role, commit.request, ui.game.reconcile(commit.request, receipt));
    assert.equal(ui.read().title, '强化结算');
    assert.equal(ui.read().actions[0].enabled, false);
    ui.update({ ...state, revision: revision.add(2), items: [bound] });
    assert.equal(ui.read().actions[0].enabled, true);
    await ui.read().actions[0].run();
    assert.equal(ui.read().title, '武器强化');
    assert.equal(ui.read().actions[0].enabled, false);
});
