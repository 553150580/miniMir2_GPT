const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const { Long, load, root, harness, entry, ready, ProtocolResponseError } = require('./helpers/session_harness.cjs');

const bytes = value => new Uint8Array(16).fill(value);
const integer = value => Long.fromString(value, true);
const longQuantity = Long.fromString('9007199254740999');
const earnedAtUnixMs = Long.fromString('1795586400000');
const saved = JSON.parse(fs.readFileSync(path.join(root, 'client/assets/scenes/Game.scene'), 'utf8'));
const savedNode = (parent, name) => parent._children.map(ref => saved[ref.__id__]).find(node => node._name === name);
const component = (node, type) => node._components.map(ref => saved[ref.__id__]).find(value => value.__type__ === type);

test('待领取保存为同一首领面板的内置按钮，三页签和原翻页控件互不遮挡', () => {
    const panel = saved.find(value => value.__type__ === 'cc.Node' && value._name === 'BossChallenge');
    const tabs = ['Personal', 'Rewards', 'World'].map(name => savedNode(panel, name));
    const bounds = tabs.map(tab => {
        assert(component(tab, 'cc.Button'));
        const size = component(tab, 'cc.UITransform')._contentSize;
        return { left: tab._lpos.x - size.width / 2, right: tab._lpos.x + size.width / 2, y: tab._lpos.y };
    });
    const width = component(panel, 'cc.UITransform')._contentSize.width;
    assert(bounds[0].left >= -width / 2 && bounds[2].right <= width / 2);
    assert(bounds.every((bound, index) => bound.y === bounds[0].y && (index === 0 || bounds[index - 1].right < bound.left)));
    assert.equal(component(savedNode(tabs[1], 'Label'), 'cc.Label')._string, '待领取');
    for (const name of ['Previous', 'Next', 'Refresh', 'Action']) assert(component(savedNode(panel, name), 'cc.Button'));
});

function personal(values = {}) {
    return { canEnter: false, displayName: '林地守卫', eligibilityReason: 7, dailyLimit: 5, successes: 0, templateId: 1,
        history: [], pendingRewards: [], pendingRewardCapacity: 0, ...values };
}

function reward(id, itemCount = 1) {
    return { rewardId: bytes(id), templateId: 1, earnedAtUnixMs,
        items: Array.from({ length: itemCount }, (_, index) => ({ itemId: 200 + index, quantity: longQuantity, bound: index === 0, quality: 1 })) };
}

function panelFixture(t, initial) {
    const nodes = new Map(), requests = [], claims = [], notices = [];
    const named = name => {
        if (!nodes.has(name)) nodes.set(name, { active: false, value: { string: '', interactable: false } });
        const node = nodes.get(name);
        node.getComponent = () => node.value;
        return node;
    };
    const panel = { activeInHierarchy: true, getChildByPath: named, getChildByName: named };
    const scope = { sceneKind: 1, sceneEpoch: integer('9007199254740993'), ready: true, level: 10, canClaimRewards: true };
    let listener, response = initial, query, award;
    const connection = { generation: 1, subscribe(callback) { listener = callback; return () => { listener = undefined; }; },
        request(id, body) { requests.push({ id, body }); return query ? query(id) : Promise.resolve(response); } };
    const { GameBoss } = load('client/assets/scripts/app/GameBoss.ts', id => id === 'cc' ? {
        game: { on() {}, off() {} }, Game: { EVENT_HIDE: 'hide', EVENT_SHOW: 'show' },
    } : id === './GameConnection' ? { newOperationId: () => bytes(9) } : id === './GameErrors' ? {
        protocolErrorMessage: (id, code) => `${id}:${code}`,
    } : {});
    const view = new GameBoss(panel, connection, () => scope, () => {}, () => {}, {
        async claim(id) { claims.push(id); await award?.(id); },
        itemName: id => `奖品${id}`,
        report: error => notices.push(error.message),
    });
    t.after(() => view.dispose());
    return { connection, scope, requests, claims, notices, view, named, panel,
        state(value) { response = value; }, query(value) { query = value; }, award(value) { award = value; },
        label(name) { return named(name).value.string; }, enabled(name) { return named(name).value.interactable; },
        push(value) { listener({ id: 20014, error: 0, value }); } };
}

const flush = () => new Promise(resolve => setImmediate(resolve));

test('待领独立于战报和零上限显示，完整附件分页且按原奖包编号领取', async t => {
    const f = panelFixture(t, personal());
    await f.view.open();
    assert.equal(f.named('Rewards').active, false);
    assert.match(f.label('Status'), /背包已满/);
    f.state(personal({ eligibilityReason: 8, pendingRewards: [reward(4, 6), reward(5)] }));
    await f.view.reload();
    assert.equal(f.named('Rewards').active, true);
    assert.match(f.label('Status'), /待领取队列已满/);
    await f.view.show('rewards');
    assert.equal(f.label('Status'), '待领取 2');
    assert.equal(f.label('Action/Label'), '领取');
    assert.match(f.label('Results'), /奖品200 ×9007199254740999.*绑定/);
    assert.equal(f.label('Results').split('\n').length, 5);
    f.view.turn(1);
    assert.match(f.label('Results'), /奖品205.*非绑定/);
    assert.equal(f.label('Page'), '2/3');
    f.award(() => f.state(personal({ pendingRewardCapacity: 8, pendingRewards: [reward(5)] })));
    await f.view.action();
    await flush();
    assert.deepEqual(f.claims, [bytes(4)]);
    assert.equal(f.label('Status'), '待领取 1/8');
    assert.equal(f.label('Page'), '1/1');
});

test('满包拒绝和未决领取后，原奖励可见且禁止在挑战或经济核对中发新领取', async t => {
    const f = panelFixture(t, personal({ pendingRewardCapacity: 8, pendingRewards: [reward(7)] }));
    await f.view.show('rewards');
    f.award(() => { throw new Error('背包已满，奖励仍在待领取'); });
    await assert.rejects(f.view.action(), /奖励仍在待领取/);
    assert.equal(f.label('Status'), '待领取 1/8');
    assert.equal(f.enabled('Action'), true);
    f.scope.canClaimRewards = false;
    f.view.render();
    assert.equal(f.enabled('Action'), false);
    await f.view.action();
    f.scope.canClaimRewards = true;
    f.scope.sceneKind = 3;
    f.view.render();
    await f.view.action();
    assert.deepEqual(f.claims, [bytes(7)]);
});

test('世界击杀与超时奖励各保留来源、原排名和领取编号，独立显示待领数量', async t => {
    const ranked = { ...reward(11), sourceKind: 'world_boss', sourceRank: 2 };
    const timeout = { ...reward(12), sourceKind: 'world_boss_timeout', sourceRank: 0 };
    const f = panelFixture(t, personal({ pendingRewardCapacity: 8, pendingRewards: [reward(3)],
        worldPendingRewards: [ranked, timeout], worldPendingRewardsMore: true }));
    await f.view.show('rewards');
    assert.equal(f.label('Status'), '个人 1/8 · 世界 2+');
    assert.match(f.label('Details'), /首领 1/);
    f.view.turn(1);
    assert.match(f.label('Details'), /世界首领 · 第2名/);
    f.view.turn(1);
    assert.match(f.label('Details'), /世界首领 · 超时奖励/);
    await f.view.action();
    assert.deepEqual(f.claims, [timeout.rewardId]);
});

test('掉线后原场景的迟到查询不能覆盖新队列，终态后自动刷新可领取状态', async t => {
    const f = panelFixture(t, personal());
    const replies = [];
    f.query(() => new Promise(resolve => replies.push(resolve)));
    const original = f.view.show('rewards');
    f.connection.generation++;
    f.scope.sceneEpoch = integer('9007199254740994');
    f.view.render();
    replies[1](personal({ pendingRewardCapacity: 8, pendingRewards: [reward(8)] }));
    await flush();
    replies[0](personal({ pendingRewardCapacity: 8, pendingRewards: [reward(9), reward(10)] }));
    await original;
    assert.equal(f.label('Status'), '待领取 1/8');
    f.scope.sceneKind = 3;
    f.scope.ready = false;
    f.push({ state: 4 });
    assert.equal(replies.length, 2);
    f.scope.sceneKind = 1;
    f.scope.ready = true;
    f.view.render();
    assert.equal(f.enabled('Action'), false);
    replies[2](personal({ pendingRewardCapacity: 8, pendingRewards: [reward(8), reward(11)] }));
    await flush();
    assert.equal(f.label('Status'), '待领取 2/8');
    f.push({ state: 6, errorCode: 3401 });
    assert.deepEqual(f.notices, ['20012:3401']);
    replies[3](personal({ eligibilityReason: 7, pendingRewardCapacity: 8, pendingRewards: [reward(8), reward(11)] }));
    await flush();
});

test('实际 40030 编码与 AppRoot 经济账本在新绑定重放原奖包、原操作及大整数版本', async () => {
    const h = harness(), session = h.session;
    ready(h, entry());
    session.inventory = { revision: integer('9007199254740997') };
    const { PendingOperations } = load('client/assets/scripts/app/PendingOperations.ts', id => id === 'long' ? Long : {});
    const storage = { saved: null, getItem() { return this.saved; }, setItem(_key, value) { this.saved = value; }, removeItem() { this.saved = null; } };
    session.pending = new PendingOperations(storage, 'challenge-reward');
    const decorators = new Proxy({}, { get() { return (...args) => args.length > 1 ? undefined : (...value) => value.length === 1 ? value[0] : undefined; } });
    const cc = { _decorator: decorators, Component: class {} };
    const { AppRoot } = load('client/assets/scripts/app/AppRoot.ts', id => id === 'cc' ? cc : id === 'long' ? Long
        : id === './GameSession' ? h : id === './GameConnection' ? { ProtocolResponseError }
        : id === '../input/NavigationIntent' ? require('../../../.work/ui-tests/input/NavigationIntent.js') : {});
    const { encodePacket, decodePacket } = load('client/assets/scripts/app/GameConnection.ts', id => id === 'long' ? Long
        : id === '../generated/protobuf/client_pb.js' ? require('../../assets/scripts/generated/protobuf/client_pb.js') : {});
    const app = new AppRoot();
    app.status = () => {};
    let rewarded = 0;
    app.boss = { refreshPersonal() { rewarded++; } };
    let original;
    session.connection.request = async (id, payload) => {
        assert.equal(id, 40030);
        assert.deepEqual(session.pending.get(h.roleIdentity()).value.rewardId, bytes(2));
        original = encodePacket(id, 1, Long.UONE, payload);
        assert.deepEqual(decodePacket(original).value.rewardId, bytes(2));
        throw Error('连接结果未知');
    };
    await assert.rejects(app.economic(40030, { rewardId: bytes(2) }), /结果未知/);
    const originalPending = session.pending.get(h.roleIdentity());
    assert.equal(originalPending.value.operation.expectedRevision.toString(), '9007199254740997');
    session.connection.status('连接已断开');
    session.connection.binding = Long.fromInt(2, true);
    ready(h, entry('9007199254740994', '1', 1, '2'));
    session.inventory = { revision: integer('9007199254740998') };
    session.pending = new PendingOperations(storage, 'challenge-reward');
    session.connection.request = async (id, payload) => {
        assert.deepEqual(encodePacket(id, 1, Long.UONE, payload), original);
        return { opId: payload.operation.opId, errorCode: 0 };
    };
    await app.submitPending(true);
    assert.equal(rewarded, 1);
    assert.equal(session.pending.get(h.roleIdentity()), null);
    const data = originalPending.value;
    assert.throws(() => session.pending.set(h.roleIdentity(), { id: 40030, value: { ...data, rewardId: new Uint8Array(15) } }));
    assert.throws(() => session.pending.set(h.roleIdentity(), { id: 40030, value: { ...data, sceneEpoch: Long.UONE } }));

    let rejectedId;
    session.connection.request = async (_id, payload) => {
        rejectedId = payload.operation.opId;
        return { opId: rejectedId, errorCode: 3401 };
    };
    await assert.rejects(app.economic(40030, { rewardId: bytes(3) }), error => error.code === 3401);
    assert.equal(session.pending.get(h.roleIdentity()), null);
    session.connection.request = async (_id, payload) => {
        assert.deepEqual(payload.rewardId, bytes(3));
        assert.notDeepEqual(payload.operation.opId, rejectedId);
        return { opId: payload.operation.opId, errorCode: 0 };
    };
    await app.economic(40030, { rewardId: bytes(3) });

    const ids = load('client/assets/scripts/generated/protocol_ids.ts', () => ({}));
    const { protocolErrorMessage } = load('client/assets/scripts/app/GameErrors.ts', id => id.endsWith('/protocol_ids') ? ids : {});
    assert.equal(protocolErrorMessage(40030, 3401), '背包已满，奖励仍在待领取');
    assert.equal(protocolErrorMessage(20012, 3401), '背包或待领取队列已满，请查看个人首领面板');
    assert.equal(protocolErrorMessage(40001, 3401), '背包空间不足，装备保持原位');
});
