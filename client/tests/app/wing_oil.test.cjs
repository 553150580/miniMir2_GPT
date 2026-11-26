const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, load } = require('./helpers/session_harness.cjs');
const assets = require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const equipment = require('../../../.work/ui-tests/app/EquipmentModel.js');
const wings = require('../../../.work/ui-tests/app/WingModel.js');
const ids = load('client/assets/scripts/generated/protocol_ids.ts', () => ({}));
const model = load('client/assets/scripts/app/WeaponOilModel.ts', path => ({
    long: Long, './SkillAssetsModel': assets, './EquipmentModel': equipment, './WingModel': wings,
})[path]);
const { GameWeaponOil } = load('client/assets/scripts/app/GameWeaponOil.ts', path => ({
    './WeaponOilModel': model, './EquipmentModel': equipment, './SkillAssetsModel': assets, '../generated/protocol_ids': ids,
})[path]);
const { GameWings } = load('client/assets/scripts/app/GameWings.ts', path => ({
    './WingModel': wings, './SkillAssetsModel': assets, '../generated/protocol_ids': ids,
})[path]);
const { PendingOperations } = load('client/assets/scripts/app/PendingOperations.ts', path => path === 'long' ? Long : {});
const pb = require('../../assets/scripts/generated/protobuf/client_pb.js');
const transport = load('client/assets/scripts/app/GameConnection.ts', path => ({
    long: Long, '../generated/protobuf/client_pb.js': pb,
})[path] ?? {});

const u = value => Long.fromString(String(value), true);
const bytes = byte => new Uint8Array(16).fill(byte);
const scene = u('9007199254741001'), revision = u('9007199254741013');
const version = u('9007199254741021'), oilId = 900021;
const contributionPpm = [0, 20000, 30000, 44000, 60000, 80000, 110000, 150000, 210000, 300000];
const wing = { wingId: bytes(8), rank: 3, appearanceTier: 1, hidden: true, luck: 5, oilToSix: 99, oilToNine: 0, version };
const weapon = { instanceId: bytes(4), itemId: 218, quantity: Long.ONE, bound: false, initialLuck: 0, currentLuck: 0, version };
const free = { instanceId: bytes(5), itemId: oilId, quantity: Long.ONE, bound: false, version, sourceKind: 'equipment_recycle', sourceId: '02'.repeat(16) };
const bound = { ...free, instanceId: bytes(6), quantity: Long.fromInt(2), bound: true };
const snapshot = { scope: '角色场景一', role: 'role-one', sceneEpoch: scene, revision, items: [weapon, bound, free], equipped: [],
    catalog: [{ item_id: 218, kind: 'equipment', equip_slot: 0, name: '木剑' }, { item_id: oilId, kind: 'luck_oil', name: '祝福油' }],
    oilItemId: oilId, wing, wingContributionPpm: contributionPpm, blocked: false };
const wingTarget = { wingId: wing.wingId }, weaponTarget = { instanceId: weapon.instanceId, equipped: false };
const uiSession = bytes(12);
const pack = (id, value) => transport.encodePacket(id, 5, Long.UONE, value);
const roundTrip = (id, value) => transport.decodePacket(pack(id, value)).value;

function quote(request, from = snapshot) {
    const target = request.wing ? { wing: from.wing } : { weapon };
    const luck = request.wing ? from.wing.luck : weapon.currentLuck;
    const pityUsed = request.wing ? (luck < 3 ? 0 : luck < 6 ? from.wing.oilToSix : from.wing.oilToNine) : 0;
    const pityAfter = luck < 3 ? 0 : luck < 6 ? 100 : 3000;
    const guaranteed = pityAfter > 0 && pityUsed >= pityAfter;
    const oil = request.allowBound ? from.items.find(item => item.itemId === oilId && item.bound) ?? free : free;
    return { ...target, quoteId: bytes(13), sceneEpoch: from.sceneEpoch, roleRevision: from.revision, equipped: false,
        uiSessionId: request.uiSessionId, allowBound: request.allowBound, outputBound: !!request.wing || oil.bound, oil,
        currentLuck: luck, successLuck: luck + 1, failureLuck: luck < 3 ? luck : luck < 6 ? 3 : 6,
        successPpm: guaranteed || !luck ? 1000000 : 200000, pityUsed, pityAfter, guaranteed, expiresAtUnixMs: u(2000000000000) };
}

function wingReceipt(review, success) {
    const before = review.wing, stage = review.currentLuck;
    return { quoteId: review.quoteId, equipped: false, oil: review.oil, beforeLuck: stage, success, guaranteed: review.guaranteed,
        pityAfter: review.pityAfter, pityUsed: review.pityUsed + Number(review.pityAfter > 0),
        wing: { ...before, version: before.version.add(1), luck: success ? review.successLuck : review.failureLuck,
            oilToSix: before.oilToSix + Number(stage >= 3 && stage < 6), oilToNine: before.oilToNine + Number(stage >= 6) } };
}

test('翼候选开关与自身实例同时满足才提供翼用油，原武器路径保持原形状', () => {
    assert.deepEqual(model.wingOilContributions({ allow_luck_oil: true, luck_contribution_ppm: contributionPpm }), contributionPpm);
    assert.equal(model.wingOilContributions({ luck_contribution_ppm: contributionPpm }), undefined);
    assert.equal(model.wingOilContributions({ allow_luck_oil: true, luck_contribution_ppm: [0] }), undefined);
    for (const current of [{ ...snapshot, wing: null }, { ...snapshot, wingContributionPpm: undefined }]) {
        assert.equal(model.weaponOilRequest(current, wingTarget, true, uiSession), undefined);
        assert.equal(model.weaponOilRequest(current, weaponTarget, false, uiSession).weapon.equipped, false);
    }
    assert.equal(model.weaponOilRequest(snapshot, { wingId: bytes(9) }, true, uiSession), undefined);
    const request = roundTrip(40049, model.weaponOilRequest(snapshot, wingTarget, false, uiSession));
    assert.equal(request.weapon, null);
    assert.equal(request.wing.version.toString(), version.toString());
    const preview = roundTrip(40050, quote(request));
    model.validateWeaponOilQuote(preview, request, snapshot);
    assert.equal(preview.weapon, null);
    assert.match(model.weaponOilQuoteLines(preview, () => '祝福油', contributionPpm).join('\n'), /角色专属（不变）/);
    assert.match(model.weaponOilQuoteLines(preview, () => '祝福油', contributionPpm).join('\n'), /幸运一击贡献 8%/);
    for (const invalid of [{ ...preview, weapon }, { ...preview, wing: { ...wing, hidden: false } },
        { ...preview, equipped: true }, { ...preview, outputBound: false }, { ...preview, pityUsed: 100 }]) {
        assert.throws(() => model.validateWeaponOilQuote(invalid, request, snapshot), /变化|原身份/);
    }
});

test('翼阈值当瓶可回退；两段计数与可见、阶数、外观分别按原回执保持', () => {
    const review = quote(model.weaponOilRequest(snapshot, wingTarget, true, uiSession));
    const action = model.weaponOilCommit(review);
    const missed = roundTrip(40005, { weaponOil: wingReceipt(review, false) }).weaponOil;
    assert.equal(model.weaponOilReceipt(action, missed, review).wing.oilToSix, 100);
    assert.equal(model.weaponOilReceiptLuck(missed), 3);
    assert.equal(missed.wing.hidden, true);
    assert.match(model.weaponOilReceiptLines(missed, () => '祝福油', contributionPpm).join('\n'), /100\/100瓶\n下一瓶起本段逐步必成/);
    for (const invalid of [{ ...missed, weapon }, { ...missed, wing: { ...missed.wing, wingId: bytes(7) } },
        { ...missed, wing: { ...missed.wing, appearanceTier: 3 } }, { ...missed, wing: { ...missed.wing, oilToSix: 101 } }]) {
        assert.throws(() => model.weaponOilReceipt(action, invalid, review), /原操作不符|原身份/);
    }
    const second = { ...snapshot, wing: { ...wing, luck: 8, oilToSix: 103, oilToNine: 3000 } };
    const guaranteed = quote(model.weaponOilRequest(second, wingTarget, false, uiSession), second);
    model.validateWeaponOilQuote(guaranteed, model.weaponOilRequest(second, wingTarget, false, uiSession), second);
    assert.equal(guaranteed.successPpm, 1000000);
    const hit = model.weaponOilReceipt(model.weaponOilCommit(guaranteed), wingReceipt(guaranteed, true), guaranteed);
    assert.deepEqual([hit.wing.luck, hit.wing.oilToSix, hit.wing.oilToNine], [9, 103, 3001]);
});

test('翼用油重连保存原 quote、界面、翼 ID 和 64 位版本，不能混入武器或穿戴位', () => {
    let text;
    const storage = { getItem: () => text ?? null, setItem: (_key, value) => { text = value; }, removeItem: () => { text = undefined; } };
    const request = model.weaponOilCommit(quote(model.weaponOilRequest(snapshot, wingTarget, false, uiSession)));
    const operation = { opId: bytes(21), commandSeq: u(50), expectedRevision: revision, expectedRuleVersion: u(30) };
    const original = { id: 40051, value: { ...request, operation } };
    new PendingOperations(storage, 'wing-oil').set('role-one', original);
    const replay = new PendingOperations(storage, 'wing-oil').get('role-one');
    assert.deepEqual(pack(40051, replay.value), pack(40051, original.value));
    const wire = roundTrip(40051, replay.value);
    assert.equal(wire.weapon, null);
    assert.equal(wire.wing.version.toString(), version.toString());
    assert.equal(wire.allowBound, false);
    for (const extra of [{ weapon: { ...weaponTarget, version } }, { wing: { ...request.wing, equipped: false } }, { wing: { ...request.wing, version: u(0) } }]) {
        assert.throws(() => new PendingOperations().set('role-one', { ...original, value: { ...original.value, ...extra } }), { code: 'INVALID_OPERATION' });
    }
});

test('同一保存面板换翼、武器保留两次确认的授权；回执后等待翼库存，重开清授权', async () => {
    let live = snapshot, read, latest, id = 10;
    const prompts = [], requests = [], spent = [];
    const view = new GameWeaponOil({ snapshot: () => live, newSession: () => bytes(++id), open: reader => { read = reader; },
        current: reader => read === reader, refresh() {}, confirm: async (_title, message, action) => { prompts.push({ message, action }); return true; },
        quote: async request => { requests.push(request); latest = quote(request, live); return latest; },
        submit: async request => { spent.push(request); view.accept(live.role, request, view.reconcile(request, wingReceipt(latest, false))); },
    });
    const action = label => read().actions.find(row => row.label === label);
    view.open(weaponTarget);
    await action('开启绑材').run();
    read().options.find(option => option.label.includes('翅膀')).choose();
    assert.equal(read().title, '翅膀祝福油');
    assert.ok(read().lines.includes('使用绑定资源 已开启'));
    assert.deepEqual(prompts.map(prompt => prompt.action), ['继续开启', '确认开启']);
    assert.match(prompts[0].message, /翅膀始终归当前角色专属/);
    await action('查看报价').run();
    await action('使用祝福油').run();
    assert.equal(spent[0].weapon, undefined);
    assert.equal(spent[0].allowBound, true);
    assert.equal(action('查看下一瓶').enabled, false);
    assert.ok(read().lines.includes('等待原翅膀状态同步'));
    live = { ...live, revision: live.revision.add(1), wing: wingReceipt(latest, false).wing };
    await action('返回选对象').run();
    read().options.find(option => option.label.includes('木剑')).choose();
    await action('查看报价').run();
    assert.deepEqual(requests[1].uiSessionId, requests[0].uiSessionId);
    assert.equal(requests[1].allowBound, true);
    assert.equal(requests[1].wing, undefined);
    view.open(wingTarget);
    assert.ok(read().lines.includes('使用绑定资源 未开启'));
    await action('查看报价').run();
    assert.notDeepEqual(requests[2].uiSessionId, requests[0].uiSessionId);
    assert.equal(requests[2].allowBound, false);
});

test('已激活且开放时从原翅膀概览进入共用培养；缺省和未激活保持原按钮', () => {
    const rules = wings.wingRules({ feather_item_id: 900032, max_rank: 10, appearance_tiers: [1, 3], ranks: [{ rank: 1, feathers: 2 }] });
    let live = { ...snapshot, rules, inventory: [], allowLuckOil: false }, read, opened;
    const control = new GameWings({ snapshot: () => live, newSession: () => uiSession, open: reader => { read = reader; },
        current: reader => read === reader, refresh() {}, openLuck: original => { opened = original; } });
    control.open();
    assert.equal(read().actions.some(action => action.label === '祝福油培养'), false);
    live = { ...live, allowLuckOil: true, wing: null };
    assert.equal(read().actions.some(action => action.label === '祝福油培养'), false);
    live = { ...live, wing };
    read().actions.find(action => action.label === '祝福油培养').run();
    assert.equal(opened, wing);
});
