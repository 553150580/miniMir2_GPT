const { test } = require('node:test');
const assert = require('node:assert/strict');
const m = require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const id = n => new Uint8Array(16).fill(n);
const page = (n, changes = {}) => ({ instanceId: id(n), itemId: 900001, version: '9007199254740993', quantity: '5', bound: true, quality: 0,
    sourceKind: 'skill_recycle', sourceId: 'original-source', locked: false, recycleBlockFlags: 0, ...changes });
function input() { return { ready: true, roleIdentity: '1:role', sceneEpoch: '9007199254740995', ruleVersion: '20',
    inventory: { revision: '9007199254740997', level: 7, items: [page(1), page(2)], learnedSkills: [] },
    wallet: { revision: '9007199254740997', gold: '100', boundGold: '200', yuanbaoUnits: '0', boundYuanbaoUnits: '0' },
    definitions: [{ item_id: 900001, name: '书页', kind: 'skill_page' }], skills: [], offers: [], upgrades: [], recycle: [] }; }
function quote(s) { return { previewId: id(9), kind: 4, ruleVersion: s.ruleVersion, configHash: 'a'.repeat(64), roleRevision: s.inventory.revision,
    expiresAtUnixMs: '20000', skillId: 0, fromLevel: 0, toLevel: 0, offerId: 0, quantity: '0', stats: [],
    spend: { revision: s.inventory.revision, gold: '0', boundGold: '0', yuanbaoUnits: '0', boundYuanbaoUnits: '0' },
    materials: s.inventory.items.map(item => ({ item: { ...item }, consumeQuantity: item.quantity })), outputs: [page(0, { instanceId: new Uint8Array(), quantity: '10' })] }; }

test('报价捕获不能忽略物品配置与保护状态改变，即使数量版本revision未变', () => {
    for (const changed of [{ itemId: 900002 }, { locked: true }, { recycleBlockFlags: 4 }]) {
        const s = input(), choices = s.inventory.items.map(x => ({ instanceId: x.instanceId, version: x.version, quantity: 5n }));
        const context = m.captureSkillAssetContext(s, choices), q = quote(s);
        Object.assign(s.inventory.items[0], changed);
        assert.throws(() => m.validateSkillAssetContext(context, q, s, 10000), /重新预览/);
    }
});

test('同源书页只选完整原实例，Long数量和版本不窄化，不猜MaxStack', () => {
    const s = input(); s.inventory.items[0].quantity = '9007199254740993';
    const prepared = m.preparePageMerge(s, s.inventory.items[0]);
    assert.equal(prepared.request.kind, 4); assert.equal(prepared.request.materials.length, 2);
    assert.equal(prepared.request.materials[0].quantity, 9007199254740993n);
    assert.equal(prepared.request.materials[0].version, '9007199254740993');
    assert.notEqual(prepared.request.materials[0].instanceId, s.inventory.items[0].instanceId);
    assert.equal(prepared.context.sceneEpoch, '9007199254740995');
    assert.equal(prepared.context.revision, '9007199254740997');
});

test('不同来源ID或绑定品质不混组，保护未知及重复身份不能合并', () => {
    for (const change of [{ itemId: 900002 }, { bound: false }, { quality: 1 }, { sourceKind: 'monster_drop' }, { sourceId: 'another-source' },
        { sourceKind: 'initial_grant' }, { sourceKind: 'legacy' }, { sourceKind: '' }, { sourceId: '' }, { locked: true },
        { recycleBlockFlags: 1 }, { recycleBlockFlags: 2 }, { recycleBlockFlags: 4 }, { recycleBlockFlags: 8 }, { recycleBlockFlags: 16 },
        { locked: undefined }, { quality: undefined }, { bound: undefined }, { quantity: '0' }, { version: '9223372036854775807' }, { version: '18446744073709551615' }]) {
        const s = input(); Object.assign(s.inventory.items[1], change);
        assert(m.pageMergeGroup(s.inventory.items[0], s.inventory.items, s.definitions).reason, JSON.stringify(change));
        assert.throws(() => m.preparePageMerge(s, s.inventory.items[0]));
    }
    const s = input(); s.inventory.items[1].instanceId = id(1);
    assert.throws(() => m.preparePageMerge(s, s.inventory.items[0]), /重复|实例/);
    for (const sourceKind of ['skill_recycle', 'npc_shop', 'monster_drop']) {
        const s = input(); s.inventory.items.forEach(x => x.sourceKind = sourceKind);
        assert.equal(m.pageMergeGroup(s.inventory.items[0], s.inventory.items, s.definitions).reason, null);
    }
});

test('合并只显示真实输出，可多叠但必须减格且守恒，不接受换源收费或半堆材料', () => {
    const s = input(), prepared = m.preparePageMerge(s, s.inventory.items[0]), q = quote(s);
    m.validatePageMergeQuote(q, prepared.request.materials, s);
    for (const mutate of [q => { q.outputs.push(page(0, { quantity: '1' })); }, q => { q.outputs[0].quantity = '9'; },
        q => { q.outputs[0].sourceId = 'new-source'; }, q => { q.outputs[0].bound = false; }, q => { q.outputs[0].quality = 1; },
        q => { q.outputs[0].locked = true; }, q => { q.spend.gold = '1'; }, q => { q.kind = 3; },
        q => { q.materials[0].consumeQuantity = '4'; }, q => { q.materials[0].item.itemId = 900002; }]) {
        const changed = quote(s); mutate(changed);
        assert.throws(() => m.validatePageMergeQuote(changed, prepared.request.materials, s));
    }
    s.inventory.items.push(page(3)); const three = m.preparePageMerge(s, s.inventory.items[0]), multi = quote(s);
    multi.outputs = [page(0, { quantity: '8' }), page(0, { quantity: '7' })];
    m.validatePageMergeQuote(multi, three.request.materials, s);
    const lines = m.skillQuoteLines(multi, s.definitions).join('\n');
    assert.match(lines, /原有3叠.*报价2叠/); assert.match(lines, /来源.*保持/); assert.doesNotMatch(lines, /必.*1叠|65535/);
});
