const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, load, harness, ready, entry } = require('./helpers/session_harness.cjs');
const model = require('../../../.work/ui-tests/app/WingModel.js');
const assets = require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const ids = load('client/assets/scripts/generated/protocol_ids.ts', () => ({}));
const { GameWings } = load('client/assets/scripts/app/GameWings.ts', path => ({
    '../generated/protocol_ids': ids, './SkillAssetsModel': assets, './WingModel': model,
})[path]);
const { PendingOperations } = load('client/assets/scripts/app/PendingOperations.ts', path => path === 'long' ? Long : {});
const pb = require('../../assets/scripts/generated/protobuf/client_pb.js');
const transport = load('client/assets/scripts/app/GameConnection.ts', path => ({
    long: Long, '../generated/protobuf/client_pb.js': pb,
})[path] ?? {});

const u = value => Long.fromString(String(value), true);
const s = value => Long.fromString(String(value));
const bytes = byte => new Uint8Array(16).fill(byte);
const pack = (id, value) => transport.encodePacket(id, 5, Long.UONE, value);
const roundTrip = (id, value) => transport.decodePacket(pack(id, value)).value;
const source = { feather_item_id: 900032, max_rank: 10, appearance_tiers: [1, 3, 5, 7, 10],
    ranks: [{ rank: 1, feathers: 3 }, { rank: 2, feathers: 4 }, { rank: 3, feathers: 6 }] };
const rules = model.wingRules(source);
const feather = (id, quantity, bound, more = {}) => ({ instanceId: bytes(id), itemId: source.feather_item_id,
    quantity: s(quantity), version: u('9007199254741009'), sourceKind: 'world_boss', sourceId: '原首领场次', bound, ...more });
const free = feather(1, 8, false), bound = feather(2, 2, true);
const initial = { scope: '原角色场景', role: 'role-one', sceneEpoch: u('9007199254741001'), revision: u('9007199254741003'),
    rules, wing: null, inventory: [free, bound, feather(3, 99, false, { locked: true }), feather(4, 99, false, { sourceId: '' })],
    blocked: false };
const session = bytes(15);
const wing = (more = {}) => ({ wingId: bytes(12), rank: 1, appearanceTier: 1,
    hidden: false, version: u(1), luck: 0, oilToSix: 0, oilToNine: 0, ...more });

function quote(snapshot, request, id = 11) {
    const targetRank = (snapshot.wing?.rank ?? 0) + 1;
    return { quoteId: bytes(id), uiSessionId: request.uiSessionId, sceneEpoch: snapshot.sceneEpoch, roleRevision: snapshot.revision,
        expiresAtUnixMs: u(2000000000000), allowBound: request.allowBound, wing: snapshot.wing, targetRank,
        targetAppearanceTier: snapshot.rules.appearanceTiers.includes(targetRank) ? targetRank : snapshot.wing.appearanceTier,
        feathers: request.feathers.map(ref => ({ item: snapshot.inventory.find(item => assets.assetId(item.instanceId) === assets.assetId(ref.instanceId)),
            quantity: ref.quantity })) };
}

function growthReceipt(review) {
    const before = review.wing ?? wing();
    return { action: 1, quoteId: review.quoteId, feathers: review.feathers, wing: { ...before,
        version: review.wing ? before.version.add(1) : before.version, rank: review.targetRank, appearanceTier: review.targetAppearanceTier } };
}

test('候选读取连续羽毛费用及独立外观阶，未知后续费用不会显示已满阶或零价', () => {
    assert.equal(model.wingRules(null), undefined);
    assert.equal(model.wingRules({ ...source, ranks: [{ rank: 2, feathers: 3 }] }), undefined);
    assert.equal(model.wingRules({ ...source, appearance_tiers: [1, 3, 3, 10] }), undefined);
    assert.equal(model.wingRules({ ...source, ranks: [{ rank: 1, feathers: Number.MAX_SAFE_INTEGER + 1 }] }), undefined);
    assert.equal(model.nextWingCost(initial), 3n);
    assert.equal(model.nextWingCost({ ...initial, wing: wing({ rank: 3, appearanceTier: 3 }) }), 0n);
    assert.equal(model.wingSlotLabel(), '翅膀 · 未激活');
    assert.match(model.wingSlotLabel(wing({ rank: 3, hidden: true })), /3阶 · 外观隐藏/);
    assert.throws(() => model.observedWingState(wing({ oilToSix: 1 })), /状态无效/);
});

test('羽毛只按原堆、绑定与版本选择精确根数，真实报价及回执保留大整数身份', () => {
    const selected = new model.WingFeatherSelection();
    assert.deepEqual(selected.options(initial), [free]);
    selected.pick(initial, bound);
    assert.equal(selected.total(initial), 0n);
    selected.setAllowBound(initial, true);
    assert.deepEqual(selected.options(initial), [bound, free]);
    selected.pick(initial, bound);
    selected.adjust(initial, -1n);
    selected.pick(initial, free);
    const request = roundTrip(40052, selected.request(initial, session));
    assert.equal(request.wingId.length, 0);
    assert.equal(request.wingVersion.toString(), '0');
    assert.deepEqual(request.feathers.map(ref => [ref.instanceId[0], ref.quantity.toString()]), [[2, '1'], [1, '2']]);
    assert.equal(request.feathers[0].version.toString(), '9007199254741009');
    const review = roundTrip(40053, quote(initial, request));
    model.validateWingQuote(review, request, initial);
    assert.match(model.wingSpendLines(review.feathers).join('\n'), /世界首领击杀奖励/);
    for (const changed of [{ ...review, allowBound: false }, { ...review, uiSessionId: bytes(1) },
        { ...review, feathers: [review.feathers[0], review.feathers[0]] }, { ...review, roleRevision: u(3) }]) {
        assert.throws(() => model.validateWingQuote(changed, request, initial), /重新报价/);
    }
    const changed = { ...initial, inventory: [...initial.inventory.slice(1), { ...free, version: free.version.add(1) }] };
    assert.equal(selected.request(changed, session), undefined);
    const action = model.wingGrowthAction(review);
    const result = roundTrip(40005, { wing: growthReceipt(review) }).wing;
    assert.equal(model.validateWingReceipt(action, result, review).wing.rank, 1);
    assert.throws(() => model.validateWingReceipt(action, { ...result, wing: { ...result.wing, rank: 2 } }), /不匹配/);
    selected.setAllowBound(initial, false);
    assert.equal(selected.total(initial), 2n);
    assert.equal(selected.request(initial, session), undefined);
});

test('跨阶外观演进及显隐使用原实例，幸运与前后段累计不改变', () => {
    const before = wing({ rank: 2, hidden: true, version: u('9007199254741003'), luck: 6, oilToSix: 101, oilToNine: 4 });
    const snapshot = { ...initial, wing: before };
    const selection = new model.WingFeatherSelection();
    selection.pick(snapshot, free);
    const request = selection.request(snapshot, session), review = quote(snapshot, request);
    assert.equal(review.targetAppearanceTier, 3);
    model.validateWingQuote(review, request, snapshot);
    const outcome = growthReceipt(review);
    const action = model.wingGrowthAction(review);
    assert.equal(model.validateWingReceipt(action, outcome, review).wing.oilToNine, 4);
    assert.deepEqual(outcome.wing.wingId, before.wingId);
    assert.equal(outcome.wing.hidden, true);
    assert.throws(() => model.validateWingReceipt(action, { ...outcome, wing: { ...outcome.wing, oilToNine: 0 } }, review), /不匹配/);
    assert.equal(model.wingAppearanceAction({ ...snapshot, wing: outcome.wing }, 5, false), undefined);
    const appearance = model.wingAppearanceAction({ ...snapshot, wing: outcome.wing }, 1, false);
    assert.deepEqual(appearance, { action: 2, wingId: before.wingId, wingVersion: outcome.wing.version, appearanceTier: 1, hidden: false });
    const style = roundTrip(40005, { wing: { action: 2, feathers: [], wing: { ...outcome.wing, version: outcome.wing.version.add(1), appearanceTier: 1, hidden: false } } }).wing;
    assert.equal(model.validateWingReceipt(appearance, style, outcome.wing).wing.rank, 3);
    assert.throws(() => model.validateWingReceipt(appearance, { ...style, feathers: review.feathers }), /不匹配/);
    assert.match(model.wingStateLines(style.wing).join('\n'), /当前幸运\+6\n3→6段累计用油 101瓶\n6→9段累计用油 4瓶/);
});

test('激活空身份和原外观版本断线后按原请求字节恢复', () => {
    const data = new Map(), storage = { getItem: key => data.get(key) ?? null, setItem: (key, text) => data.set(key, text), removeItem: key => data.delete(key) };
    const operation = { opId: bytes(24), commandSeq: u(17), expectedRevision: initial.revision, expectedRuleVersion: u(30) };
    const first = { action: 1, quoteId: bytes(11), uiSessionId: session, allowBound: false, wingId: new Uint8Array(), wingVersion: u(0) };
    const style = { action: 2, wingId: bytes(12), wingVersion: u('9007199254741003'), appearanceTier: 1, hidden: false };
    const ledger = new PendingOperations(storage, 'wings');
    ledger.set('role-one', { id: 40054, value: { ...first, operation } });
    ledger.set('role-two', { id: 40054, value: { ...style, operation } });
    const restored = new PendingOperations(storage, 'wings');
    assert.deepEqual(pack(40054, restored.get('role-one').value), pack(40054, { ...first, operation }));
    assert.deepEqual(pack(40054, restored.get('role-two').value), pack(40054, { ...style, operation }));
    assert.equal(roundTrip(40054, restored.get('role-two').value).hidden, false);
    for (const wrong of [{ ...first, wingVersion: u(2) }, { ...first, allowBound: undefined }, { ...style, wingId: new Uint8Array(), wingVersion: u(0) }, { ...style, allowBound: true }]) {
        assert.throws(() => new PendingOperations().set('other', { id: 40054, value: { ...wrong, operation } }), { code: 'INVALID_OPERATION' });
    }
});

test('保存翅膀弹窗二次授权、真实报价、顺接下一阶及切换已解锁外观', async () => {
    let snapshot = initial, read, sequence = 10, latest;
    const prompts = [], answers = [true, false, true, true], requests = [], sent = [];
    const view = new GameWings({ snapshot: () => snapshot, newSession: () => bytes(++sequence),
        open: value => { read = value; }, current: value => read === value, refresh() {},
        confirm: async (_title, _text, action) => { prompts.push(action); return answers.shift(); },
        quote: async request => { requests.push(request); latest = quote(snapshot, request, requests.length); return latest; },
        submit: async request => { sent.push(request); const receipt = request.action === 1 ? growthReceipt(latest) : {
            action: 2, feathers: [], wing: { ...snapshot.wing, appearanceTier: request.appearanceTier, hidden: request.hidden, version: snapshot.wing.version.add(1) } };
            view.accept(snapshot.role, request, view.reconcile(request, receipt));
        } });
    const action = label => read().actions.find(row => row.label === label);
    view.open();
    assert.equal(read().title, '翅膀');
    action('激活翅膀').run();
    await action('开启绑材').run();
    assert.ok(read().lines.includes('使用绑定资源 未开启'));
    await action('开启绑材').run();
    assert.deepEqual(prompts, ['继续开启', '确认开启', '继续开启', '确认开启']);
    read().options[0].choose();
    read().quantity.minus();
    read().options[1].choose();
    await action('查看报价').run();
    snapshot = { ...initial, revision: initial.revision.add(1) };
    await action('重新报价').run();
    assert.equal(sent.length, 0);
    await action('确认激活').run();
    assert.equal(read().title, '翅膀激活完成');
    assert.ok(read().lines.includes('等待角色原翅膀状态同步'));
    assert.equal(action('返回翅膀').enabled, false);
    const created = growthReceipt(latest).wing;
    snapshot = { ...snapshot, revision: snapshot.revision.add(1), wing: created, inventory: [
        { ...bound, quantity: s(1), version: bound.version.add(1) }, { ...free, quantity: s(6), version: free.version.add(1) },
    ] };
    action('继续升阶').run();
    assert.ok(read().lines.includes('使用绑定资源 已开启'));
    read().options[0].choose();
    read().options[1].choose();
    await action('查看报价').run();
    assert.deepEqual(requests[2].wingId, created.wingId);
    assert.deepEqual(requests[2].uiSessionId, requests[0].uiSessionId);
    assert.equal(requests[2].allowBound, true);
    await action('确认升阶').run();
    const second = growthReceipt(latest).wing;
    snapshot = { ...snapshot, revision: snapshot.revision.add(1), wing: { ...second, rank: 3, appearanceTier: 3 } };
    action('返回翅膀').run();
    assert.ok(read().lines.includes('下一阶羽毛费用待开放'));
    action('外观设置').run();
    assert.deepEqual(read().options.map(option => option.label), ['选择 1阶外观', '已选 3阶外观']);
    read().options[0].choose();
    action('改为隐藏').run();
    await action('保存外观').run();
    assert.equal(sent[2].action, 2);
    assert.equal(sent[2].hidden, true);
    assert.deepEqual(sent[2].wingId, created.wingId);
    assert.equal(read().title, '翅膀外观已保存');
    snapshot = initial;
    view.open();
    action('激活翅膀').run();
    assert.ok(read().lines.includes('使用绑定资源 未开启'));
});

test('真实会话只保留有效翅膀库存观察，拒绝旧库存和损坏的新状态', () => {
    const h = harness(), e = ready(h, entry()), socket = h.session.connection;
    const observation = wing({ rank: 3, appearanceTier: 3, hidden: true, version: u(4) });
    socket.deliver(40003, { items: [], revision: u(10), wing: observation });
    assert.equal(h.session.inventory.wing, observation);
    socket.deliver(40003, { items: [], revision: u(9), wing: null });
    assert.equal(h.session.inventory.wing, observation);
    socket.deliver(40003, { items: [], revision: u(11), wing: { ...observation, rank: 1 } });
    assert.equal(h.session.inventory.wing, observation);
    socket.deliver(40003, { items: [], revision: u(11), wing: { ...observation, wingId: bytes(15), version: u(5) } });
    assert.equal(h.session.inventory.wing, observation);
    socket.deliver(40003, { items: [], revision: u(11), wing: null });
    assert.equal(h.session.inventory.wing, observation);
    assert.equal(e.playerHandle, h.session.entry.playerHandle);
});

test('真实 AppRoot 翅膀提交在掉回包后落盘只重查原请求，匹配回执才清账', async () => {
    const h = harness(), state = h.session;
    ready(h, entry());
    state.connection.deliver(40003, { revision: initial.revision, items: initial.inventory });
    const data = new Map(), storage = { getItem: key => data.get(key) ?? null, setItem: (key, text) => data.set(key, text), removeItem: key => data.delete(key) };
    state.pending = new PendingOperations(storage, '翼未决');
    const decorator = (...args) => args.length > 1 ? undefined : (...values) => values.length === 1 ? values[0] : undefined;
    const { AppRoot } = load('client/assets/scripts/app/AppRoot.ts', id => id === './GameSession' ? h :
        id === 'cc' ? { _decorator: new Proxy({}, { get: () => decorator }), Component: class {} } :
        id === 'long' ? Long : id === '../input/NavigationIntent' ? require('../../../.work/ui-tests/input/NavigationIntent.js') :
        id === './MapAttachmentCoordinator' ? require('../../../.work/ui-tests/app/MapAttachmentCoordinator.js') : {});
    const selection = new model.WingFeatherSelection();
    selection.pick(initial, free);
    const review = quote(initial, selection.request(initial, session)), request = model.wingGrowthAction(review);
    const outbound = [];
    state.connection.request = async (message, value) => {
        assert.equal(message, 40054);
        assert.ok(new PendingOperations(storage, '翼未决').get(h.roleIdentity()));
        outbound.push(pack(message, value));
        if (outbound.length === 1) throw new Error('已发送但响应丢失');
        const outcome = growthReceipt(review);
        return roundTrip(40005, { opId: value.operation.opId, commandSeq: value.operation.commandSeq,
            wing: outbound.length === 2 ? { ...outcome, quoteId: bytes(99) } : outcome });
    };
    const first = new AppRoot();
    await assert.rejects(first.economic(40054, request), /响应丢失/);
    assert.ok(state.pending.get(h.roleIdentity()));
    ready(h, entry('9007199254741031'));
    state.pending = new PendingOperations(storage, '翼未决');
    const resumed = new AppRoot(), statuses = [];
    resumed.status = message => statuses.push(message);
    await assert.rejects(resumed.submitPending(true), /保留原请求/);
    await resumed.submitPending(true);
    assert.deepEqual(outbound[1], outbound[0]);
    assert.deepEqual(outbound[2], outbound[0]);
    assert.equal(state.pending.get(h.roleIdentity()), null);
    assert.deepEqual(statuses, ['翅膀已达 1阶']);
});
