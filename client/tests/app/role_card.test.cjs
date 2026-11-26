const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, load, harness, entry, ready, ProtocolResponseError } = require('./helpers/session_harness.cjs');

const protocol = load('client/assets/scripts/app/RoleCardProtocol.ts', id => id === 'long' ? Long : {});
const large = Long.fromString('9007199254740993');
const timestamp = Long.fromString('1795651201000');
const quoteId = new Uint8Array(16).fill(5);
const privilege = { productId: 1, kind: 1, days: 30, priceYuanbaoUnits: Long.fromInt(30000) };
const hosting = { productId: 2, kind: 2, days: 30, priceYuanbaoUnits: Long.fromInt(45000) };
const quote = { quoteId, product: privilege, ruleVersion: Long.fromInt(13, true), configHash: 'a'.repeat(64),
    expiresAtUnixMs: timestamp, previousUntilUnixMs: Long.ZERO };
const dates = { privilegeUntilUnixMs: Long.ZERO, hostingUntilUnixMs: Long.ZERO, operatingDay: '2026-11-26' };
const state = sceneEpoch => ({ sceneEpoch, products: [privilege, hosting], roleRevision: large.toUnsigned(),
    ...dates, dailyPrivilegeUnits: Long.fromInt(2000), privilegeClaimable: false, errorCode: 0 });

test('月卡的角色查询在原附着确认两商品，并拒绝切换角色或地图后的旧报价', async () => {
    const h = harness();
    const first = ready(h, entry());
    h.session.connection.request = async (id, value) => {
        assert.equal(id, 40035);
        assert.deepEqual(value, { sceneEpoch: first.sceneEpoch, productId: 0 });
        return state(first.sceneEpoch);
    };
    const current = await h.queryRoleCard();
    assert.equal(current.roleRevision.toString(), '9007199254740993');
    assert.deepEqual(current.products.map(item => item.kind), [1, 2]);
    let respond;
    h.session.connection.request = (id, value) => new Promise(resolve => { assert.equal(value.productId, 1); respond = resolve; });
    const old = h.queryRoleCard(1);
    ready(h, entry('9007199254740995', '2'));
    respond({ ...state(first.sceneEpoch), quote });
    await assert.rejects(old, /地图已切换/);
});

test('关闭月卡的角色查询保留明确拒绝，过期与错场报价不当做真实状态', async () => {
    const h = harness();
    const current = ready(h, entry());
    h.session.connection.request = async () => ({ errorCode: 2103 });
    await assert.rejects(h.queryRoleCard(), ProtocolResponseError);
    h.session.connection.request = async () => ({ ...state(current.sceneEpoch), quote: { ...quote, product: hosting } });
    await assert.rejects(h.queryRoleCard(1), /报价与当前目录不一致/);
    h.session.connection.request = async () => state(current.sceneEpoch.add(1));
    await assert.rejects(h.queryRoleCard(), /月卡状态未同步/);
});

test('购卡与日领验证原回执身份，元宝余额和事件时间不丢失 64 位精度', () => {
    const originalBuy = { quoteId, productId: privilege.productId };
    const purchase = { quote, effectiveAtUnixMs: timestamp.sub(1000), newUntilUnixMs: timestamp.add(30 * 86400000), yuanbaoBalanceUnits: large };
    assert.equal(protocol.validateRoleCardPurchaseReceipt(purchase, originalBuy).yuanbaoBalanceUnits.toString(), large.toString());
    assert.throws(() => protocol.validateRoleCardPurchaseReceipt(purchase, { ...originalBuy, productId: 2 }), /原报价/);
    assert.throws(() => protocol.validateRoleCardPurchaseReceipt({ ...purchase, yuanbaoBalanceUnits: large.toNumber() }, originalBuy), /原报价/);
    assert.throws(() => protocol.validateRoleCardPurchaseReceipt({ ...purchase, quote: { ...quote, quoteId: new Uint8Array(16).fill(8) } }, originalBuy), /原报价/);
    const daily = { operatingDay: dates.operatingDay, acceptedAtUnixMs: timestamp, grantedUnits: Long.fromInt(2000), boundYuanbaoBalanceUnits: large };
    assert.equal(protocol.validateRoleCardDailyReceipt(daily, dates).boundYuanbaoBalanceUnits.toString(), large.toString());
    assert.throws(() => protocol.validateRoleCardDailyReceipt(daily, { operatingDay: '2026-11-27' }), /原运营日/);
    assert.throws(() => protocol.validateRoleCardDailyReceipt({ ...daily, boundYuanbaoBalanceUnits: Long.ONE }, dates), /原运营日/);
});

test('双月卡入口等待当前附着的能力协商，丢弃上个角色的报价目录', async () => {
    class Node {
        constructor(parent) { this.parent = parent; this.active = true; this.children = new Map(); }
        get activeInHierarchy() { return this.active && (!this.parent || this.parent.activeInHierarchy); }
        child(name) { const node = new Node(this); this.children.set(name, node); return node; }
        getChildByName(name) { return this.children.get(name); }
        getChildByPath(name) { return this.getChildByName(name); }
        getComponent(kind) { return kind === Button ? this.button : this.label; }
    }
    class Button {}
    class Label {}
    const { GameRoleCards } = load('client/assets/scripts/app/GameRoleCards.ts', id => id === 'long' ? Long : { Button, Label, Node });
    const parent = new Node(), entry = parent.child('RoleCards'), view = new Node();
    parent.active = false; view.active = false;
    for (const name of ['Wallet', 'PrivilegeInfo', 'HostingInfo', 'Status']) view.child(name).label = new Label();
    for (const name of ['BuyPrivilege', 'BuyHosting', 'Claim']) {
        const node = view.child(name);
        node.child('Label').label = new Label();
        node.button = new Button(); node.button.node = node;
    }
    const contexts = [1, 2].map(slot => ({ scope: `attach-${slot}`, role: `role-${slot}`, balance: large, ruleVersion: quote.ruleVersion, pending: false }));
    let context = contexts[0];
    const queue = [];
    const controller = new GameRoleCards(entry, view, {}, () => context, { query: () => new Promise(resolve => queue.push(resolve)) });
    controller.sync();
    assert.equal(queue.length, 0);
    assert.equal(controller.available, false);
    parent.active = true;
    controller.discover(); controller.sync();
    assert.equal(queue.length, 1);
    context = contexts[1];
    controller.sync();
    assert.equal(queue.length, 2);
    queue[0](state(Long.UONE));
    await Promise.resolve();
    assert.equal(controller.available, false);
    queue[1](state(Long.UONE));
    await Promise.resolve();
    assert.equal(controller.available, true);
    context = undefined;
    controller.sync();
    assert.equal(controller.available, false);
});
