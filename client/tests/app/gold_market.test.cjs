const assert = require('node:assert/strict');
const { test } = require('node:test');
const { Long, load, harness, entry, ready } = require('./helpers/session_harness.cjs');

const item = load('client/assets/scripts/app/ItemMarketProtocol.ts', id => id === 'long' ? Long : {});
const itemPresentation = load('client/assets/scripts/app/ItemMarketPresentation.ts', id => id === 'long' ? Long : id === './ItemMarketProtocol' ? item : {});
const protocol = load('client/assets/scripts/app/GoldMarketProtocol.ts', id => id === 'long' ? Long : id === './ItemMarketProtocol' ? item : {});
const model = load('client/assets/scripts/app/GoldMarketModel.ts', id => id === 'long' ? Long : id === './GoldMarketProtocol' ? protocol : itemPresentation);
const { GoldMarketScope: Scope, GoldMarketSide: Side, GoldMarketActionKind: Action } = protocol;
const units = value => Long.fromInt(value);
const id = value => new Uint8Array(16).fill(value);
const time = Long.fromString('1795651201000');
const rules = { ruleVersion: Long.fromInt(13, true), configHash: '1'.repeat(64), termsHash: '2'.repeat(64),
    referenceUnitPriceYuanbaoUnits: units(100), minUnitPriceYuanbaoUnits: units(80), maxUnitPriceYuanbaoUnits: units(120),
    priceStepYuanbaoUnits: Long.ONE, goldPerLot: units(10000), orderDurationSeconds: 3661, sellerCommissionBp: 1000, maxOrdersPerSide: 5 };
const wallet = { gold: units(50000), yuanbao: units(1000) };
const empty = (sceneEpoch, scope) => ({ sceneEpoch, scope, enabled: false, canTrade: false, orders: [], escrows: [],
    nextCursor: new Uint8Array(0), errorCode: 0, roleRevision: Long.UONE });

test('买单只预留非绑定元宝、卖单只预留非绑定金币，确认保留原限价与准确期限', () => {
    const buy = model.previewGoldMarket(rules, Side.Buy, '2', '10.1', wallet);
    assert.equal(buy.yuanbao.toString(), '202');
    assert.equal(buy.request.termsHash, rules.termsHash);
    assert.equal(buy.request.expectedDurationSeconds, 3661);
    assert.match(buy.confirmation, /冻结 20.2 非绑定元宝，委托 1小时1分钟1秒/);
    assert.match(buy.confirmation, /卖方佣金 10%/);
    const sell = model.previewGoldMarket(rules, Side.Sell, '1', '8.0', wallet);
    assert.match(sell.confirmation, /冻结 10000 非绑定金币/);
    assert.throws(() => model.previewGoldMarket(rules, Side.Sell, '1', '7.9', wallet), /限价/);
    assert.throws(() => model.previewGoldMarket(rules, Side.Buy, '1', '10.01', wallet), /一位小数/);
    assert.throws(() => model.previewGoldMarket(rules, Side.Buy, '1', '10.0', { ...wallet, yuanbao: Long.ZERO }), /非绑定元宝/);
    assert.throws(() => model.previewGoldMarket(rules, Side.Sell, '1', '10.0', { ...wallet, gold: Long.ZERO }), /非绑定金币/);
});

test('两个市场在一个附着共用只读槽，排队后换地图的原读不会再发出', async () => {
    const h = harness(), original = ready(h, entry()), observed = [];
    h.session.connection.request = (message, request) => new Promise(resolve => observed.push({ message, request, resolve }));
    const oldItems = h.queryItemMarket(item.ItemMarketScope.Public);
    const gold = h.queryGoldMarket(Scope.Sell);
    assert.deepEqual(observed.map(row => row.message), [40039]);
    observed[0].resolve(empty(original.sceneEpoch, item.ItemMarketScope.Public));
    await oldItems;
    assert.deepEqual(observed.map(row => row.message), [40039, 40042]);
    assert.equal(observed[1].request.sceneEpoch.toString(), original.sceneEpoch.toString());
    observed[1].resolve(empty(original.sceneEpoch, Scope.Sell));
    assert.equal((await gold).enabled, false);

    const active = h.queryItemMarket(item.ItemMarketScope.Public), stale = h.queryGoldMarket(Scope.Escrow);
    const stops = [assert.rejects(active, /地图已切换/), assert.rejects(stale, /地图已切换/)];
    ready(h, entry('9007199254740995', '4'));
    observed[2].resolve(empty(original.sceneEpoch, item.ItemMarketScope.Public));
    await Promise.all(stops);
    assert.equal(observed.length, 3);
});

test('旧托管可以没有新单条款，挂单回执不能偷换价量或伪称已撮合', () => {
    const escrow = { escrowId: id(3), orderId: id(4), version: Long.UONE, kind: 1, gold: units(10000), yuanbaoUnits: Long.ZERO,
        reason: 1, createdAtUnixMs: time };
    const state = { ...empty(Long.UONE, Scope.Escrow), enabled: true, escrows: [escrow] };
    assert.deepEqual(protocol.validateGoldMarketState(state, Long.UONE, Scope.Escrow).escrows, [escrow]);
    assert.throws(() => protocol.validateGoldMarketState({ ...state, canTrade: true }, Long.UONE, Scope.Escrow), /未确认/);
    const request = model.previewGoldMarket(rules, Side.Buy, '2', '10.0', wallet).request;
    const receipt = { action: Action.Place, orderId: id(4), orderVersion: Long.UONE, side: Side.Buy, state: 1, quantityLots: units(2),
        filledLots: Long.ZERO, remainingLots: units(2), unitPriceYuanbaoUnits: units(100), goldDelta: Long.ZERO, yuanbaoDeltaUnits: units(-200), expiresAtUnixMs: time };
    assert.equal(protocol.validateGoldMarketReceipt(receipt, request).yuanbaoDeltaUnits.toString(), '-200');
    assert.throws(() => protocol.validateGoldMarketReceipt({ ...receipt, filledLots: Long.ONE }, request), /原价量/);
    assert.throws(() => protocol.validateGoldMarketReceipt({ ...receipt, unitPriceYuanbaoUnits: units(101) }, request), /原价量/);
    assert.throws(() => protocol.validateGoldMarketReceipt({ ...receipt, action: Action.Claim, escrowId: id(8) },
        { action: Action.Claim, escrowId: escrow.escrowId, escrowVersion: escrow.version }), /原托管/);
});
