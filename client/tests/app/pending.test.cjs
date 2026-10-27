const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { createRequire } = require('node:module');
const ts = require('/Applications/Cocos/Creator/3.8.8/CocosCreator.app/Contents/Resources/app.asar.unpacked/node_modules/typescript');
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
