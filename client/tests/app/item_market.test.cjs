const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, load, harness, entry, ready } = require('./helpers/session_harness.cjs');

const protocol = load('client/assets/scripts/app/ItemMarketProtocol.ts', id => id === 'long' ? Long : {});
const model = load('client/assets/scripts/app/ItemMarketPresentation.ts', id => id === 'long' ? Long : id === './ItemMarketProtocol' ? protocol : {});
const { ItemMarketActionKind: Action, ItemMarketScope: Scope } = protocol;
const qty = Long.fromInt(3), price = Long.fromInt(15), id = byte => new Uint8Array(16).fill(byte);
const order = { orderId: id(4), version: Long.fromString('9007199254741005', true), state: 1, item: { itemId: 35 }, remainingQuantity: qty,
    unitPriceYuanbaoUnits: price, endAtUnixMs: Long.fromString('1795651201000'), activeBidCount: 0, owned: false, sameAccount: false,
    myActiveBid: false, nextUnitPriceYuanbaoUnits: Long.fromInt(17) };
const state = (sceneEpoch, scope = Scope.Public) => ({ sceneEpoch, scope, enabled: true, canTrade: true,
    orders: scope === Scope.Escrow ? [] : [order], escrows: [], nextCursor: new Uint8Array([2, 9]), errorCode: 0, roleRevision: Long.UONE });

test('公开订单、我的订单和托管以原场景和不透明分页令牌绑定在同一角色', async () => {
    const h = harness(), first = ready(h, entry());
    const cursor = new Uint8Array([255, 10, 77]);
    h.session.connection.request = async (id, value) => {
        assert.equal(id, 40039);
        assert.deepEqual(value, { sceneEpoch: first.sceneEpoch, scope: Scope.Mine, pageSize: 20, cursor });
        return state(first.sceneEpoch, Scope.Mine);
    };
    assert.equal((await h.queryItemMarket(Scope.Mine, cursor)).orders[0].version.toString(), '9007199254741005');
    let respond;
    h.session.connection.request = () => new Promise(resolve => { respond = resolve; });
    const detached = h.queryItemMarket(Scope.Escrow);
    ready(h, entry('9007199254740995', '2'));
    respond(state(first.sceneEpoch, Scope.Escrow));
    await assert.rejects(detached, /地图已切换/);
    assert.throws(() => protocol.validateItemMarketState({ ...state(first.sceneEpoch, Scope.Escrow), orders: [order] }, first.sceneEpoch, Scope.Escrow), /未确认/);
});

test('候选人须已有本人有效预购才可加价，卡过期的卖家仍可撤无预购订单', () => {
    assert.deepEqual(model.marketOrderActions(order, true), [Action.Bid]);
    assert.deepEqual(model.marketOrderActions({ ...order, myActiveBid: true, activeBidCount: 1 }, true), [Action.Raise]);
    assert.deepEqual(model.marketOrderActions({ ...order, sameAccount: true }, true), []);
    assert.deepEqual(model.marketOrderActions({ ...order, owned: true }, false), [Action.Cancel]);
    assert.deepEqual(model.marketOrderActions({ ...order, owned: true, activeBidCount: 1 }, true), []);
    assert.deepEqual(model.marketOrderActions({ ...order, state: 2 }, true), [Action.Direct]);
    assert.deepEqual(model.marketOrderActions({ ...order, state: 5 }, true), []);
});

test('上架与直购锁定原身份、单价和金额，禁止溢出或多位小数', () => {
    const item = { instanceId: id(3), itemId: 35, version: Long.UONE, quantity: qty, bound: false, locked: false, sourceKind: 'monster_drop', sourceId: 'fixture', recycleBlockFlags: 0, quality: 1 };
    assert.equal(model.marketBagCandidate(item, false), true);
    assert.equal(model.marketBagCandidate(item, true), false);
    assert.equal(model.marketBagCandidate({ ...item, quality: 2 }, true), true);
    assert.equal(model.marketBagCandidate({ ...item, bound: true }, false), false);
    const listing = model.marketListing(item, '2', '1.5');
    assert.equal(listing.unitPriceYuanbaoUnits.toString(), '15');
    assert.deepEqual(listing.itemInstanceId, id(3));
    assert.throws(() => model.marketListing(item, '1', '0.15'), /一位小数/);
    assert.throws(() => model.marketListing(item, '4', '1.5'), /超出背包/);
    assert.throws(() => model.marketAmount(Long.MAX_VALUE, qty), /超出范围/);
    const raising = model.marketOrderAction({ ...order, myActiveBid: true }, Action.Raise, true);
    assert.equal(raising.unitPriceYuanbaoUnits.toString(), '15');
    assert.equal(raising.quantity.toString(), '3');
    const direct = model.marketOrderAction({ ...order, state: 2 }, Action.Direct, true, '2');
    assert.equal(direct.quantity.toString(), '2');
    assert.throws(() => model.marketOrderAction({ ...order, state: 2 }, Action.Direct, true, '4'), /超过/);
});

test('成功回执保留原操作目标，拒绝把其他托管条目或订单当成本笔', () => {
    const request = model.marketOrderAction({ ...order, state: 2 }, Action.Direct, true, '2');
    const receipt = { action: Action.Direct, orderId: order.orderId, orderVersion: order.version.add(1), itemId: 35,
        quantity: Long.fromInt(2), unitPriceYuanbaoUnits: price, yuanbaoDeltaUnits: Long.fromInt(-30), taxYuanbaoUnits: Long.fromInt(3) };
    assert.equal(protocol.validateItemMarketReceipt(receipt, request).yuanbaoDeltaUnits.toString(), '-30');
    assert.throws(() => protocol.validateItemMarketReceipt({ ...receipt, orderId: id(8) }, request), /原道具订单/);
    assert.throws(() => protocol.validateItemMarketReceipt({ ...receipt, quantity: qty }, request), /原道具数量/);
    const claim = { action: Action.Claim, escrowId: id(5), escrowVersion: Long.UONE, quantity: Long.ZERO };
    assert.throws(() => protocol.validateItemMarketReceipt({ ...receipt, action: Action.Claim, escrowId: id(6) }, claim), /原托管项目/);
});

test('入口暂忙后重新打开功能可重试，市场页签在上一读请求确认前不抢占读取槽', async () => {
    class Button { interactable = true; }
    class Label { string = ''; }
    class Node {
        active = true;
        children = new Map();
        getChildByName(name) { return this.getChildByPath(name); }
        getChildByPath(path) { if (!this.children.has(path)) this.children.set(path, new Node()); return this.children.get(path); }
        getComponent(type) {
            if (type === List) return this.list ??= new List(this);
            if (type === Label) return this.label ??= new Label();
            return this.button ??= new Button();
        }
        on() {}
        off() {}
    }
    class List {
        static EVENT_ITEM_CLICK = 'click';
        constructor(node) { this.node = node; }
        setItems() {}
        scrollToIndex() {}
    }
    const { GameItemMarket } = load('client/assets/scripts/app/GameItemMarket.ts', id => {
        if (id === 'long') return Long;
        if (id === './ItemMarketProtocol') return protocol;
        if (id === './ItemMarketPresentation') return model;
        if (id === '../ui/List') return { List };
        return { Button, Label, Node, isValid: () => true };
    });
    const market = new Node(), entry = new Node(), details = new Node(), listing = new Node();
    const ui = { top: { name: 'Functions' }, open() { this.top = market; } };
    const actor = { scope: 'ready', role: 'seller', bag: [], wallet: Long.ZERO, pending: false };
    const queries = [];
    const controller = new GameItemMarket(entry, market, details, listing, ui, {}, () => actor,
        { query: scope => new Promise((resolve, reject) => queries.push({ scope, resolve, reject })) });
    controller.discover(); controller.discover();
    assert.equal(queries.length, 1);
    queries[0].reject(new Error('共享只读槽暂忙'));
    await Promise.resolve();
    assert.equal(entry.active, false);
    assert.equal(queries.length, 1);
    controller.discover();
    assert.equal(queries.length, 2);
    queries[1].resolve(state(Long.UONE));
    await Promise.resolve();
    assert.equal(entry.active, true);
    const opening = controller.open();
    const mine = market.getChildByPath('Tabs/Mine').getComponent(Button);
    assert.equal(mine.interactable, false);
    await controller.switchScope(Scope.Mine);
    assert.equal(queries.length, 3);
    queries[2].resolve(state(Long.UONE));
    await opening;
    assert.equal(mine.interactable, true);
    const switching = controller.switchScope(Scope.Mine);
    await controller.switchScope(Scope.Escrow);
    assert.equal(queries.length, 4);
    assert.equal(queries[3].scope, Scope.Mine);
    queries[3].resolve(state(Long.UONE, Scope.Mine));
    await switching;
    controller.dispose();
});
