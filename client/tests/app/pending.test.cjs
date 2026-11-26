const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');
const ts = require('../../../tools/client/creator_paths.cjs').typescript;
const file = path.resolve(__dirname, '../../assets/scripts/app/PendingOperations.ts');
const requireClient = createRequire(file);
const Long = requireClient('long');
const pb = requireClient('../generated/protobuf/client_pb.js').minimir.v1;
const source = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
});
const compiled = { exports: {} };
new Function('require', 'module', 'exports', source.outputText)(requireClient, compiled, compiled.exports);
const { PendingOperations } = compiled.exports;

class MemoryStorage {
    data = new Map();
    getItem(key) { return this.data.get(key) ?? null; }
    setItem(key, value) { this.data.set(key, value); }
    removeItem(key) { this.data.delete(key); }
}
const key = 'test.pending.account-1';
const id = value => new Uint8Array(16).fill(value);
function pending(seed = 1) {
    return { id: 40002, value: { itemInstanceIds: [id(seed)], operation: {
        opId: id(seed + 1), commandSeq: Long.MAX_UNSIGNED_VALUE,
        expectedRevision: Long.fromString('9007199254740993', false), expectedRuleVersion: Long.MIN_VALUE,
    } } };
}

test('金币补时仅保存原报价和整数秒，重连保持同一操作的协议字节', () => {
    const storage = new MemoryStorage(), original = { id: 40007, value: { requestedSeconds: 86400, quoteId: id(8), operation: pending().value.operation } };
    const ledger = new PendingOperations(storage, key);
    ledger.set('roleA', original);
    const restored = new PendingOperations(storage, key).get('roleA');
    const encode = value => pb.BuyHostingTimeRequest.encode({ requested_seconds: value.requestedSeconds, quote_id: value.quoteId,
        operation: { op_id: value.operation.opId, command_seq: value.operation.commandSeq, expected_revision: value.operation.expectedRevision,
            expected_rule_version: value.operation.expectedRuleVersion } }).finish();
    assert.deepEqual(encode(restored.value), encode(original.value));
    assert.throws(() => ledger.set('roleA', { ...original, value: { ...original.value, quoteId: id(7) } }), { code: 'PENDING_EXISTS' });
    for (const changes of [{ requestedSeconds: 0 }, { requestedSeconds: 86401 }, { requestedSeconds: 0.5 },
        { quoteId: new Uint8Array(15) }, { price: 3 }, { quoteVersion: Long.UONE }]) {
        assert.throws(() => new PendingOperations().set('roleA', { ...original, value: { ...original.value, ...changes } }), { code: 'INVALID_OPERATION' });
    }
});

test('购买月卡和特权日领在通知断线后保持原商品、报价、运营日与流水', () => {
    const purchase = { id: 40037, value: { quoteId: id(9), productId: 2, operation: pending().value.operation } };
    const daily = { id: 40038, value: { operatingDay: '2026-11-26', operation: pending().value.operation } };
    const storage = new MemoryStorage(), ledger = new PendingOperations(storage, key);
    ledger.set('roleA', purchase);
    ledger.set('roleB', daily);
    const resumed = new PendingOperations(storage, key);
    const encode = request => {
        const value = request.value, op = value.operation;
        const operation = { op_id: op.opId, command_seq: op.commandSeq, expected_revision: op.expectedRevision,
            expected_rule_version: op.expectedRuleVersion };
        return request.id === 40037 ? pb.BuyRoleCardRequest.encode({ quote_id: value.quoteId, product_id: value.productId, operation }).finish()
            : pb.ClaimRoleCardDailyRequest.encode({ operating_day: value.operatingDay, operation }).finish();
    };
    assert.deepEqual(encode(resumed.get('roleA')), encode(purchase));
    assert.deepEqual(encode(resumed.get('roleB')), encode(daily));
    assert.throws(() => resumed.set('roleB', { ...daily, value: { ...daily.value, operatingDay: '2026-11-27' } }), { code: 'PENDING_EXISTS' });
    for (const [request, changes] of [[purchase, { productId: 0 }], [purchase, { quoteId: new Uint8Array(15) }],
        [purchase, { priceYuanbaoUnits: 30000 }], [daily, { operatingDay: '2026-11-26T12:00:00Z' }]]) {
        assert.throws(() => new PendingOperations().set('roleA', { ...request, value: { ...request.value, ...changes } }), { code: 'INVALID_OPERATION' });
    }
});

test('道具市场六个动作均按原报价版本、目标、数量和流水在重连发送同样字节', () => {
    const op = pending().value.operation;
    const version = Long.fromString('9007199254740993', true);
    const price = Long.fromString('9007199254740995');
    const order = { orderId: id(4), orderVersion: version, quantity: Long.ONE, unitPriceYuanbaoUnits: price, operation: op };
    const requests = [
        { action: 1, itemInstanceId: id(3), itemVersion: version, quantity: Long.ONE, unitPriceYuanbaoUnits: price, operation: op },
        ...[2, 3, 4, 5].map(action => ({ action, ...order })),
        { action: 6, escrowId: id(5), escrowVersion: version, quantity: Long.ZERO, operation: op },
    ];
    for (const value of requests) {
        const storage = new MemoryStorage(), journal = new PendingOperations(storage, key);
        journal.set('roleA', { id: 40041, value });
        const restored = new PendingOperations(storage, key).get('roleA').value;
        const encode = request => pb.ItemMarketAction.encode({ action: request.action, order_id: request.orderId, order_version: request.orderVersion,
            item_instance_id: request.itemInstanceId, item_version: request.itemVersion, quantity: request.quantity,
            unit_price_yuanbao_units: request.unitPriceYuanbaoUnits, escrow_id: request.escrowId, escrow_version: request.escrowVersion,
            operation: { op_id: op.opId, command_seq: op.commandSeq, expected_revision: op.expectedRevision, expected_rule_version: op.expectedRuleVersion } }).finish();
        assert.deepEqual(encode(restored), encode(value));
        assert.deepEqual(restored.operation, op);
    }
    for (const value of [{ ...requests[0], quantity: Long.ZERO }, { ...requests[1], unitPriceYuanbaoUnits: price.toUnsigned() },
        { ...requests[5], quantity: Long.ONE }, { ...requests[5], orderId: id(4) }, { ...requests[2], orderVersion: Long.ONE }]) {
        assert.throws(() => new PendingOperations().set('roleA', { id: 40041, value }), { code: 'INVALID_OPERATION' });
    }
});

test('金币挂单、撤单和领取在重连保留原上限条款、原期限、ID和流水', () => {
    const op = pending().value.operation, version = Long.fromString('9007199254740993', true);
    const original = { action: 1, side: 2, lots: Long.fromInt(2), unitPriceYuanbaoUnits: Long.fromString('9007199254740995'),
        termsHash: '9'.repeat(64), expectedDurationSeconds: 300, operation: op };
    for (const request of [original, { action: 2, orderId: id(4), orderVersion: version, operation: op },
        { action: 3, escrowId: id(5), escrowVersion: version, operation: op }]) {
        const storage = new MemoryStorage(), first = new PendingOperations(storage, key);
        first.set('roleA', { id: 40044, value: request });
        const restored = new PendingOperations(storage, key).get('roleA').value;
        const encode = row => pb.GoldMarketAction.encode({ action: row.action, side: row.side, lots: row.lots,
            unit_price_yuanbao_units: row.unitPriceYuanbaoUnits, terms_hash: row.termsHash, expected_duration_seconds: row.expectedDurationSeconds,
            order_id: row.orderId, order_version: row.orderVersion, escrow_id: row.escrowId, escrow_version: row.escrowVersion,
            operation: { op_id: row.operation.opId, command_seq: row.operation.commandSeq, expected_revision: row.operation.expectedRevision,
                expected_rule_version: row.operation.expectedRuleVersion } }).finish();
        assert.deepEqual(encode(restored), encode(request));
    }
    for (const modifications of [{ side: 0 }, { termsHash: 'short' }, { lots: Long.ONE.toUnsigned() }, { expectedDurationSeconds: 0 }, { escrowId: id(3) }]) {
        assert.throws(() => new PendingOperations().set('roleA', { id: 40044, value: { ...original, ...modifications } }), { code: 'INVALID_OPERATION' });
    }
});

function mirrorExchange() {
    return { id: 40034, value: {
        packCount: 7, expectedPrice: Long.fromString('9007199254740993', false),
        expectedItemId: 0x80000002, expectedPackQuantity: Long.fromString('9007199254740995', false),
        operation: { opId: id(18), commandSeq: Long.MAX_UNSIGNED_VALUE,
            expectedRevision: Long.fromString('9007199254741001', true), expectedRuleVersion: Long.fromInt(30, true) },
    } };
}

test('镜像兑换重载原报价、物品每包数量和操作，未知提交后保持 40034 的原协议字节', () => {
    const storage = new MemoryStorage(), original = mirrorExchange(), first = new PendingOperations(storage, key);
    first.set('roleA', original);
    const persisted = storage.getItem(key), saved = JSON.parse(persisted).operations[0].value;
    assert.deepEqual(saved.expectedPrice, { $type: 'Long', value: '9007199254740993', unsigned: false });
    assert.deepEqual(saved.expectedPackQuantity, { $type: 'Long', value: '9007199254740995', unsigned: false });
    const reattached = new PendingOperations(storage, key);
    const restored = reattached.get('roleA');
    assert.deepEqual(restored, original);
    assert.equal(restored.id, 40034);
    assert.equal(restored.value.expectedPrice.unsigned, false);
    assert.equal(restored.value.expectedPackQuantity.unsigned, false);
    assert.equal(restored.value.operation.commandSeq.unsigned, true);
    const encode = value => pb.MirrorArenaExchangeRequest.encode({
        pack_count: value.packCount, expected_price: value.expectedPrice, expected_item_id: value.expectedItemId,
        expected_pack_quantity: value.expectedPackQuantity, operation: {
            op_id: value.operation.opId, command_seq: value.operation.commandSeq,
            expected_revision: value.operation.expectedRevision, expected_rule_version: value.operation.expectedRuleVersion,
        },
    }).finish();
    const originalBytes = encode(original.value);
    assert.deepEqual(encode(restored.value), originalBytes);
    reattached.set('roleA', original);
    for (const changed of [
        { packCount: 8 }, { expectedItemId: 35 }, { expectedPrice: original.value.expectedPrice.add(1) },
        { expectedPackQuantity: original.value.expectedPackQuantity.add(1) },
    ]) {
        assert.throws(() => reattached.set('roleA', { ...original, value: { ...original.value, ...changed } }), { code: 'PENDING_EXISTS' });
        assert.deepEqual(encode(reattached.get('roleA').value), originalBytes);
        assert.equal(storage.getItem(key), persisted);
    }
});

test('镜像兑换拒绝半套报价、非法数量和损坏的本地 Long，禁止生成替代请求', () => {
    const original = mirrorExchange(), invalid = { code: 'INVALID_OPERATION' };
    for (const change of [
        { packCount: 0 }, { packCount: 101 }, { packCount: 0.5 }, { expectedItemId: 0 }, { expectedItemId: 0x100000000 },
        { expectedPrice: undefined }, { expectedItemId: undefined }, { expectedPackQuantity: undefined },
        { expectedPrice: 9007199254740992 }, { expectedPrice: Long.NEG_ONE }, { expectedPrice: Long.ONE.toUnsigned() },
        { expectedPackQuantity: Long.ZERO }, { expectedPackQuantity: Long.ONE.toUnsigned() }, { addedPoints: Long.ONE },
    ]) {
        const journal = new PendingOperations();
        assert.throws(() => journal.set('roleA', { ...original, value: { ...original.value, ...change } }), invalid);
        assert.equal(journal.get('roleA'), null);
    }
    const storage = new MemoryStorage(), journal = new PendingOperations(storage, key);
    journal.set('roleA', original);
    const corrupt = JSON.parse(storage.getItem(key));
    corrupt.operations[0].value.expectedPackQuantity.unsigned = true;
    const raw = JSON.stringify(corrupt);
    storage.setItem(key, raw);
    assert.throws(() => new PendingOperations(storage, key), { code: 'CORRUPT_STORAGE' });
    assert.throws(() => journal.set('roleA', original), { code: 'CORRUPT_STORAGE' });
    assert.equal(storage.getItem(key), raw);
});

test('技能学习未决保存原书与原经济身份，拒绝补造等级、数量、错误ID或凭据', () => {
    const storage = new MemoryStorage(), original = { id: 40010, value: { itemInstanceId: id(7), skillId: 1, operation: pending().value.operation } };
    new PendingOperations(storage,key).set('roleA', original);
    const restored = new PendingOperations(storage,key).get('roleA');
    const encode = value => pb.LearnSkillRequest.encode({ item_instance_id:value.itemInstanceId,skill_id:value.skillId,
        operation:{ op_id:value.operation.opId,command_seq:value.operation.commandSeq,expected_revision:value.operation.expectedRevision,expected_rule_version:value.operation.expectedRuleVersion } }).finish();
    assert.deepEqual(encode(restored.value),encode(original.value));
    for(const mutate of [v=>{v.itemInstanceId=new Uint8Array(15);},v=>{v.skillId=0;},v=>{v.skillId=1.5;},v=>{v.skillId=4294967296;},
        v=>{v.level=3;},v=>{v.quantity=2;},v=>{v.token='secret';}]){
        const value={...original.value};mutate(value);const store=new PendingOperations();
        assert.throws(()=>store.set('roleA',{id:40010,value}),{code:'INVALID_OPERATION'});assert.equal(store.get('roleA'),null);
    }
});

test('新旧技能槽都按原比较范围重放，重连不能修改观察到的四槽', () => {
    const storage = new MemoryStorage();
    const operation = pending().value.operation;
    const encode = value => pb.SetSkillSlotsRequest.encode({ skill_slot_ids: value.skillSlotIds,
        expected_skill_slot_ids: value.expectedSkillSlotIds, operation: { op_id: value.operation.opId,
            command_seq: value.operation.commandSeq, expected_revision: value.operation.expectedRevision,
            expected_rule_version: value.operation.expectedRuleVersion } }).finish();
    for (const [role, original] of [
        ['roleA', { skillSlotIds: [17, 0, 0, 0], operation }],
        ['roleB', { skillSlotIds: [17, 30, 0, 0], expectedSkillSlotIds: [17, 0, 0, 0], operation }],
    ]) {
        new PendingOperations(storage, key).set(role, { id: 40031, value: original });
        const restored = new PendingOperations(storage, key).get(role).value;
        assert.deepEqual(encode(restored), encode(original));
        assert.deepEqual(Object.keys(restored).sort(), Object.keys(original).sort());
    }
    for (const expectedSkillSlotIds of [[], [17, 17, 0, 0], [17, -1, 0, 0], [17, 0, 0, 0, 0]]) {
        assert.throws(() => new PendingOperations().set('roleA', { id: 40031,
            value: { skillSlotIds: [17, 30, 0, 0], expectedSkillSlotIds, operation } }), { code: 'INVALID_OPERATION' });
    }
});

test('技能资产提交仅保存原previewId与经济身份，重连不能改报价或夹带价格材料',()=>{
    const storage=new MemoryStorage(),original={id:40013,value:{previewId:id(9),operation:pending().value.operation}};
    const store=new PendingOperations(storage,key);store.set('roleA',original);
    const restored=new PendingOperations(storage,key).get('roleA');
    const encode=value=>pb.SkillAssetCommitRequest.encode({preview_id:value.previewId,operation:{op_id:value.operation.opId,command_seq:value.operation.commandSeq,expected_revision:value.operation.expectedRevision,expected_rule_version:value.operation.expectedRuleVersion}}).finish();
    assert.deepEqual(encode(restored.value),encode(original.value));
    assert.throws(()=>store.set('roleA',{id:40013,value:{...original.value,previewId:id(8)}}),{code:'PENDING_EXISTS'});
    for(const mutate of [v=>{v.previewId=new Uint8Array(15);},v=>{v.price=1;},v=>{v.materials=[];},v=>{v.bound=false;},v=>{v.token='secret';}]){
        const value={...original.value};mutate(value);assert.throws(()=>new PendingOperations().set('roleA',{id:40013,value}),{code:'INVALID_OPERATION'});
    }
});
function wire(value) {
    return pb.RecycleRequest.encode({ item_instance_ids: value.itemInstanceIds, operation: {
        op_id: value.operation.opId, command_seq: value.operation.commandSeq,
        expected_revision: value.operation.expectedRevision, expected_rule_version: value.operation.expectedRuleVersion,
    } }).finish();
}

test('持久化重载保持原PB字节、Long64位有符号状态和16字节ID', () => {
    const storage = new MemoryStorage();
    const original = pending();
    new PendingOperations(storage, key).set('origin1:roleA', original);
    const raw = storage.getItem(key);
    assert.match(raw, /18446744073709551615/);
    assert.match(raw, /"unsigned":false/);
    const restored = new PendingOperations(storage, key).get('origin1:roleA');
    assert.equal(restored.id, 40002);
    assert.deepEqual(wire(restored.value), wire(original.value));
    assert.equal(restored.value.operation.commandSeq.unsigned, true);
    assert.equal(restored.value.operation.expectedRevision.unsigned, false);
    assert.equal(restored.value.operation.expectedRuleVersion.toString(), '-9223372036854775808');
    assert.ok(restored.value.operation.opId instanceof Uint8Array);
    assert.deepEqual(restored.value.operation.opId, original.value.operation.opId);
});

test('两个角色隔离，clear仅清当前角色，不为另一角色返回原请求', () => {
    const storage = new MemoryStorage();
    const store = new PendingOperations(storage, key);
    store.set('roleA', pending(1));
    store.set('roleB', pending(3));
    assert.equal(store.get('roleC'), null);
    assert.equal(store.get('roleA').value.operation.opId[0], 2);
    assert.equal(store.get('roleB').value.operation.opId[0], 4);
    store.clear('roleA');
    assert.equal(new PendingOperations(storage, key).get('roleA'), null);
    assert.equal(new PendingOperations(storage, key).get('roleB').value.operation.opId[0], 4);
    store.clear('roleB');
    assert.equal(storage.getItem(key), null);
});

test('未决请求不可被新操作覆盖，原样set幂等且读写值防外部修改', () => {
    const store = new PendingOperations();
    const input = pending(1);
    store.set('roleA', input);
    input.value.operation.opId[0] = 99;
    assert.equal(store.get('roleA').value.operation.opId[0], 2);
    const snapshot = store.get('roleA');
    snapshot.value.operation.commandSeq.low = 0;
    snapshot.value.itemInstanceIds[0][0] = 99;
    assert.deepEqual(wire(store.get('roleA').value), wire(pending(1).value));
    store.set('roleA', pending(1));
    assert.throws(() => store.set('roleA', pending(2)), { code: 'PENDING_EXISTS' });
});

test('仅白名单协议字段可保存，密码token、普通Number64和非法ID被拒绝', () => {
    const store = new PendingOperations();
    assert.throws(() => store.set('roleA', { id: 10001, value: { account: 'a', password: 'secret' } }), { code: 'INVALID_OPERATION' });
    const secret = pending(); secret.value.token = 'secret';
    assert.throws(() => store.set('roleA', secret), { code: 'INVALID_OPERATION' });
    const unsafe = pending(); unsafe.value.operation.commandSeq = 9007199254740992;
    assert.throws(() => store.set('roleA', unsafe), { code: 'INVALID_OPERATION' });
    const bytes = pending(); bytes.value.operation.opId = new Uint8Array(15);
    assert.throws(() => store.set('roleA', bytes), { code: 'INVALID_OPERATION' });
    assert.throws(() => store.set('', pending()), { code: 'INVALID_ROLE' });
    assert.equal(store.get('roleA'), null);
});

test('损坏、标记篡改、Long溢出、重复角色和原始JSON超限全部拒绝', () => {
    const storage = new MemoryStorage();
    new PendingOperations(storage, key).set('roleA', pending());
    const clean = storage.getItem(key);
    for (const mutate of [
        () => '{',
        raw => { const data = JSON.parse(raw); data.operations[0].id = 10001; return JSON.stringify(data); },
        raw => raw.replace('18446744073709551615', '18446744073709551616'),
        raw => raw.replace('"$type":"Bytes16"', '"$type":"Unknown"'),
        raw => { const data = JSON.parse(raw); data.operations.push(data.operations[0]); return JSON.stringify(data); },
        raw => { const data = JSON.parse(raw); data.token = 'secret'; return JSON.stringify(data); },
        raw => raw + ' '.repeat(65536),
        () => JSON.stringify({ version: 1, operations: [], unused: '中'.repeat(22000) }),
    ]) {
        storage.setItem(key, mutate(clean));
        assert.throws(() => new PendingOperations(storage, key), { code: 'CORRUPT_STORAGE' });
    }
    storage.setItem(key, clean);
    const existing = new PendingOperations(storage, key);
    storage.setItem(key, '{');
    assert.throws(() => existing.set('roleB', pending(3)), { code: 'CORRUPT_STORAGE' });
    assert.equal(storage.getItem(key), '{');
});

test('五角色和64KiB写入上限不会丢失原记录', () => {
    const storage = new MemoryStorage();
    const store = new PendingOperations(storage, key);
    for (let i = 0; i < 5; ++i) store.set(`role${i}`, pending(i));
    const original = storage.getItem(key);
    assert.throws(() => store.set('role5', pending(8)), { code: 'CAPACITY' });
    assert.equal(storage.getItem(key), original);
    const largeStore = new PendingOperations();
    const large = pending(); large.value.itemInstanceIds = Array.from({ length: 2000 }, () => id(9));
    assert.throws(() => largeStore.set('roleA', large), { code: 'LIMIT' });
    assert.equal(largeStore.get('roleA'), null);
});

test('存储失败明确报错，清除失败保留原未决操作', () => {
    const storage = new MemoryStorage();
    const store = new PendingOperations(storage, key);
    store.set('roleA', pending());
    storage.removeItem = () => { throw new Error('存储不可用'); };
    assert.throws(() => store.clear('roleA'), { code: 'STORAGE_FAILURE' });
    assert.ok(new PendingOperations(storage, key).get('roleA'));
    storage.setItem = () => { throw new Error('配额不足'); };
    assert.throws(() => store.set('roleB', pending(3)), { code: 'STORAGE_FAILURE' });
    assert.equal(store.get('roleB'), null);
    storage.getItem = () => { throw new Error('禁止读取'); };
    assert.throws(() => store.get('roleA'), { code: 'STORAGE_FAILURE' });
});

test('其它白名单经济消息可往返，账号存储命名空间互不干扰', () => {
    const storage = new MemoryStorage();
    const context = pending().value.operation;
    const values = [
        { id: 40001, value: { itemInstanceId: id(1), slot: 0, operation: context } },
        { id: 40006, value: { benefitKind: 0, operation: context } },
        { id: 30002, value: { deathId: id(2), action: 1, operation: context } },
    ];
    for (const value of values) {
        const store = new PendingOperations(storage, key); store.set('roleA', value);
        assert.equal(new PendingOperations(storage, key).get('roleA').id, value.id);
        assert.equal(new PendingOperations(storage, 'account-2').get('roleA'), null);
        store.clear('roleA');
    }
});

test('使用药品的未决记录保存原实例、代次与操作身份，重载后PB字节不变', () => {
    const storage = new MemoryStorage();
    const original = { id: 40009, value: { itemInstanceId: id(7), itemId: 35,
        sceneEpoch: Long.MAX_UNSIGNED_VALUE, operation: pending().value.operation } };
    const encode = value => pb.UseItemRequest.encode({ item_instance_id: value.itemInstanceId, item_id: value.itemId,
        scene_epoch: value.sceneEpoch, operation: { op_id: value.operation.opId, command_seq: value.operation.commandSeq,
            expected_revision: value.operation.expectedRevision, expected_rule_version: value.operation.expectedRuleVersion } }).finish();
    const store = new PendingOperations(storage, key);
    store.set('roleA', original);
    const restored = new PendingOperations(storage, key).get('roleA');
    assert.equal(restored.id, 40009);
    assert.equal(restored.value.sceneEpoch.toString(), '18446744073709551615');
    assert.deepEqual(encode(restored.value), encode(original.value));
    assert.throws(() => store.set('roleA', { ...original, value: { ...original.value, itemInstanceId: id(8) } }), { code: 'PENDING_EXISTS' });
    assert.deepEqual(store.get('roleA').value.itemInstanceId, id(7));
});

test('药品未决请求拒绝不安全代次、无效实例及额外字段，不能夹带新数量或凭据', () => {
    for (const mutate of [
        value => { value.sceneEpoch = 9007199254740992; },
        value => { value.sceneEpoch = Long.UZERO; },
        value => { value.sceneEpoch = Long.NEG_ONE; },
        value => { value.itemId = 0; }, value => { value.itemId = -1; },
        value => { value.itemInstanceId = new Uint8Array(15); },
        value => { value.quantity = 100; }, value => { value.token = 'secret'; },
    ]) {
        const store = new PendingOperations();
        const value = { itemInstanceId: id(7), itemId: 35, sceneEpoch: Long.UONE, operation: pending().value.operation };
        mutate(value);
        assert.throws(() => store.set('roleA', { id: 40009, value }), { code: 'INVALID_OPERATION' });
        assert.equal(store.get('roleA'), null);
    }
});

test('材料施法与护符装卸持久保留原实例、Long版本、生命代次及op；坏载荷不得落盘',()=>{
    const large=Long.fromString('9007199254740993',true),operation=pending().value.operation;
    const cast={sceneEpoch:large,inputSequence:7,skillId:14,targetHandle:0,targetGrid:{x:8,y:9},direction:4,materialInstanceId:id(7),materialVersion:large,casterLife:Long.UZERO,operation};
    const equip={itemInstanceId:id(7),itemVersion:large,unequip:true,operation};
    for(const [message,value,codec]of [[40014,cast,pb.CastWithMaterialRequest],[40015,equip,pb.SetCastingSupplyRequest]]){
        const storage=new MemoryStorage(),ledger=new PendingOperations(storage,key);ledger.set('roleA',{id:message,value});const restored=new PendingOperations(storage,key).get('roleA');
        assert.equal(restored.id,message);assert.deepEqual(restored.value.operation.opId,operation.opId);
        const snake=v=>v==null||Long.isLong(v)||v instanceof Uint8Array?v:Array.isArray(v)?v.map(snake):typeof v==='object'?Object.fromEntries(Object.entries(v).map(([k,v])=>[k.replace(/[A-Z]/g,x=>'_'+x.toLowerCase()),snake(v)])):v;
        assert.deepEqual(codec.encode(snake(restored.value)).finish(),codec.encode(snake(value)).finish());
    }
    for(const mutate of [v=>v.materialVersion=9007199254740992,v=>v.casterLife=Long.fromInt(-1),v=>v.direction=8,v=>v.inputSequence=0,v=>v.targetGrid={x:1,y:-1},v=>v.targetGrid={x:1,y:2,extra:3},v=>v.materialInstanceId=id(3).slice(1),v=>v.skillId=0,v=>v.token='secret']){
        const value={...cast,targetGrid:{...cast.targetGrid}};mutate(value);assert.throws(()=>new PendingOperations().set('roleA',{id:40014,value}),{code:'INVALID_OPERATION'});
    }
    assert.throws(()=>new PendingOperations().set('roleA',{id:40015,value:{...equip,slot:100}}),{code:'INVALID_OPERATION'});
    assert.throws(()=>new PendingOperations().set('roleA',{id:40015,value:{...equip,unequip:1}}),{code:'INVALID_OPERATION'});
});
test('17无目标材料日志重载仍省略坐标，禁止追加target/map；旧14/15不能借此省略原目标',()=>{
 const large=Long.MAX_UNSIGNED_VALUE,cast={sceneEpoch:large,inputSequence:9,skillId:17,direction:5,materialInstanceId:id(7),materialVersion:large,casterLife:large,operation:pending().value.operation};
 const storage=new MemoryStorage(),ledger=new PendingOperations(storage,key);ledger.set('roleA',{id:40014,value:cast});const restored=new PendingOperations(storage,key).get('roleA');
 assert(!('targetHandle'in restored.value));assert(!('targetGrid'in restored.value));assert.equal(restored.value.casterLife.toString(),large.toString());assert.equal(restored.value.materialVersion.toString(),large.toString());
 for(const extra of [{targetHandle:0},{targetGrid:{x:0,y:0}},{mapId:Long.UONE},{targetHandle:0,targetGrid:{x:0,y:0}},{token:'secret'}])assert.throws(()=>new PendingOperations().set('roleA',{id:40014,value:{...cast,...extra}}),{code:'INVALID_OPERATION'});
 for(const skillId of [14,15]){
  assert.throws(()=>new PendingOperations().set('roleA',{id:40014,value:{...cast,skillId}}),{code:'INVALID_OPERATION'});
  const valid={...cast,skillId,targetHandle:0,targetGrid:{x:8,y:9}};new PendingOperations().set('roleA',{id:40014,value:valid});
 }
});
