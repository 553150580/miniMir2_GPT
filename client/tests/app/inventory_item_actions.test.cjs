const assert = require('node:assert/strict');
const { test } = require('node:test');
const { load, Long } = require('./helpers/session_harness.cjs');

const models = '../../../.work/ui-tests/app/';
const skills = require(models + 'SkillModel.js');
const potions = require(models + 'PotionModel.js');
const assets = require(models + 'SkillAssetsModel.js');
const { inventoryItemActionView } = load('client/assets/scripts/app/InventoryItemActions.ts', path => require(models + path.slice(2) + '.js'));
const unsigned = value => Long.fromString(String(value), true);
const item = (itemId, key, extra = {}) => ({
    instanceId: new Uint8Array(16).fill(key), itemId, quantity: unsigned(2), version: unsigned(1), quality: 0, bound: false, ...extra,
});
const attack = { id: 8, job_id: 0, book_item_id: 82, name: '战斗技能', learn_level: 7, initial_level: 0, max_level: 3, base_cost: 2, level_cost: 1 };
const equipment = { item_id: 80, kind: 'equipment', name: '布衣', required_level: 3, equip_slot: 1, allowed_jobs: [0] };
const potion = { item_id: 81, kind: 'potion', name: '金创药', use_enabled: true, potion: {
    hp: 25, mp: 0, tick_ms: 1000, base_per_tick: 5, levels_per_step: 10, use_delay_ms: 300, queue_cap: 5,
} };
const book = { item_id: 82, kind: 'book', name: '技能书', skill_id: 8 };
const supply = { item_id: 83, kind: 'casting_supply', name: '护符' };
const definitions = [equipment, potion, book, supply];
const role = { ready: true, roleIdentity: '角色1', sceneEpoch: unsigned(123), ruleVersion: unsigned(30) };
const firstBook = item(82, 1), secondBook = item(82, 2, { bound: true, sourceKind: 'monster_drop', sourceId: '12' });
const firstPotion = item(81, 3), secondPotion = item(81, 4, { bound: true });
const armor = item(80, 5, { quantity: unsigned(1), locked: true, recycleBlockFlags: 1 });
const inventory = { revision: unsigned(8), level: 10, learnedSkills: [], items: [firstBook, secondBook, firstPotion, secondPotion, armor] };
const wallet = { revision: inventory.revision, gold: unsigned(0), boundGold: unsigned(0), yuanbaoUnits: unsigned(0), boundYuanbaoUnits: unsigned(0) };
const skillState = { ...role, job: 0, inventory, definitions: [attack] };
const inventoryAssets = { ...role, inventory, wallet, definitions, skills: [attack], offers: [], upgrades: [], recycle: [
    { item_id: 82, book_quality: 0, allowed_source_kinds: ['monster_drop'], page_item_id: 99, pages_per_book: 1 },
] };

test('背包领用选择原药叠与原书叠，消耗不会换为排在前面的同类实例', () => {
    const potionState = { ...role, inventory, definitions, player: { hp: unsigned(5), hpMax: unsigned(30), mp: unsigned(10), mpMax: unsigned(10) } };
    const potionPreview = potions.createPotionPreview(potionState, 81, secondPotion.instanceId);
    const skillPreview = skills.createSkillLearnPreview(skillState, 8, secondBook.instanceId);
    assert.deepEqual(potions.potionInstanceId(potionPreview), secondPotion.instanceId);
    assert.deepEqual(skills.skillBookInstance(skillPreview), secondBook.instanceId);
    assert.match(skillPreview.message, /绑定技能书/);
    const changed = { ...inventory, items: [firstBook, firstPotion, armor] };
    assert.throws(() => potions.validatePotionPreview(potionPreview, { ...potionState, inventory: changed }), /已变化/);
    assert.throws(() => skills.validateSkillLearnPreview(skillPreview, { ...skillState, inventory: changed }), /已变化/);
    const recycle = assets.prepareBookRecycle(inventoryAssets, secondBook);
    assert.equal(recycle.request.kind, 3);
    assert.deepEqual(recycle.request.materials, [{ instanceId: secondBook.instanceId, version: secondBook.version, quantity: 1n }]);
    assert.throws(() => assets.prepareBookRecycle(inventoryAssets, firstBook), /不能拆解/);
});

test('背包详情保留按品类的真实操作和职业、锁定、掉线门槛', async () => {
    const called = [], run = name => async () => { called.push(name); };
    const handlers = { equip: run('equip'), recycle: run('recycle'), lock: run('lock'), usePotion: run('potion'), learn: run('learn'), recycleBook: run('book') };
    const actions = (value, definition, extra = {}) => inventoryItemActionView({ item: value, definition, ready: true, blockedReason: '',
        equipment: { ...role, scope: '角色附着', level: 10, job: 0, definitions, items: inventory.items, equipped: [] },
        assets: inventoryAssets, playerAlive: true, ...extra }, handlers);
    const equipView = actions(armor, equipment);
    assert.deepEqual(equipView.actions.map(({ label, enabled }) => [label, enabled]), [['穿戴装备', true], ['分解装备', false], ['解锁装备', true]]);
    await equipView.actions[2].run();
    const bookView = actions(secondBook, book, { skill: { definition: attack, learned: false, canLearn: true, status: '' } });
    assert.deepEqual(bookView.actions.map(({ label, enabled }) => [label, enabled]), [['学习技能', true], ['拆成书页', true]]);
    await bookView.actions[0].run(); await bookView.actions[1].run();
    assert.deepEqual(actions(secondBook, book).actions.map(({ label }) => label), ['拆成书页']);
    assert.equal(actions(firstPotion, potion, { ready: false, blockedReason: '角色场景尚未就绪' }).actions[0].enabled, false);
    const supplyView = actions(item(83, 6), supply, { supply: { enabled: true, slot: 100, item_ids: [83] } });
    assert.deepEqual(supplyView.actions.map(({ label, enabled }) => [label, enabled]), [['装入耗材', true]]);
    assert.deepEqual(called, ['lock', 'learn', 'book']);
});
