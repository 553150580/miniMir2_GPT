const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { EventEmitter } = require('node:events');
const { test } = require('node:test');
const { load, root, Long } = require('./helpers/session_harness.cjs');

const skillAssets = require('../../../.work/ui-tests/app/SkillAssetsModel.js');
const model = load('client/assets/scripts/app/NpcShopModel.ts', id => id === './SkillAssetsModel' ? skillAssets : {});
const scene = JSON.parse(fs.readFileSync(path.join(root, 'client/assets/scenes/Game.scene'), 'utf8'));
const child = (node, name) => node._children.map(ref => scene[ref.__id__]).find(row => row._name === name);
const component = (node, type) => node._components.map(ref => scene[ref.__id__]).find(row => row.__type__ === type);
const panel = ['Canvas', 'Safe', 'UIMgr', 'Panels', 'NpcShop'].reduce(child, scene[scene[0].scene.__id__]);
const detailPanel = ['Canvas', 'Safe', 'PopupView', 'Panels', 'ItemActions'].reduce(child, scene[scene[0].scene.__id__]);

test('NPC商店三种操作与网格保存在场景，列表和按钮互不遮挡', () => {
    assert.equal(panel._active, false);
    const list = child(panel, 'ItemsList'), view = child(list, 'View');
    const content = child(view, 'Content'), template = child(content, 'ItemTemplate');
    const grid = list._components.map(ref => scene[ref.__id__]).find(row => row.templateNode);
    const scroll = component(list, 'cc.ScrollView'), size = component(view, 'cc.UITransform')._contentSize;
    assert.deepEqual([grid.layoutMode, grid.cellWidth * 2 + grid.spacingX], [2, size.width]);
    assert.equal(scene[scroll._content.__id__], content);
    assert.equal(scene[grid.viewport.__id__], view);
    assert.equal(scene[grid.templateNode.__id__], template);
    assert.equal(scroll.vertical, true);
    assert.equal(scroll.horizontal, false);
    assert(component(view, 'cc.Mask'));
    assert(component(template, 'cc.Button'));
    assert(component(child(template, 'Text'), 'cc.Label'));
    const top = list._lpos.y + size.height / 2, bottom = list._lpos.y - size.height / 2;
    const tab = child(panel, 'Tabs');
    assert.deepEqual(tab._children.map(ref => scene[ref.__id__]._name), ['Buy', 'Sell', 'Repair']);
    assert(tab._lpos.y - component(tab, 'cc.UITransform')._contentSize.height / 2 > top);
    for (const name of ['Quantity', 'Preview', 'RepairNormal', 'RepairSpecial']) {
        const row = child(panel, name);
        assert(row._lpos.y + component(row, 'cc.UITransform')._contentSize.height / 2 < bottom);
    }
});

const version = Long.fromString('9007199254740993', true), instance = new Uint8Array(16).fill(7);
const purchase = { key: 'offer:20', offerId: 20, itemId: 88, name: '疗伤药', detail: '金币20', maxQuantity: 6, quantityStep: 2 };
const equipment = { key: 'bag:sword', itemId: 218, name: '木剑', detail: '当前持久1/5', maxQuantity: 1,
    itemVersion: version, instanceId: instance, repairActions: [3] };
const stock = { shopKey: '4:4003', scope: '角色附着1', ready: true, buy: [purchase], sell: [equipment], repair: [equipment] };
const quote = { quoteId: new Uint8Array(16).fill(8), expiresAtUnixMs: '1900000000000', npcId: '4003', action: 1, offerId: 20,
    itemId: 88, price: '40', gold: '20', boundGold: '20', boundIngot: '0', ingot: '0', quantity: 4, bound: true };

test('包量使用实际件数，报价核对原商品、附着及钱包类型', () => {
    assert.equal(model.vendorSelection(stock, 1, purchase, 1, 3n), undefined);
    const buy = model.vendorSelection(stock, 1, purchase, 1, 4n);
    assert.deepEqual([buy.offerId, buy.quantity], [20, 4n]);
    assert.match(model.vendorQuoteText(purchase.name, buy, quote), /购买 疗伤药 ×4\n费用 绑定金币20、金币20\n物品绑定/);
    assert.throws(() => model.vendorQuoteText(purchase.name, buy, { ...quote, offerId: 21 }), /不一致/);
    assert.equal(model.sameVendorSelection(buy, { ...buy, scope: '角色附着2' }), false);
});

test('出售接受原实例版本，普通修理显示恢复量、修后上限和绑定', () => {
    const sell = model.vendorSelection(stock, 2, equipment, 2, 1n);
    assert.notEqual(sell.instanceId, instance);
    assert.equal(sell.itemVersion, 9007199254740993n);
    const sellQuote = { ...quote, action: 2, offerId: 0, itemId: 218, instanceId: instance, quantity: 1, price: '5', boundGold: '0', gold: '5' };
    assert.match(model.vendorQuoteText(equipment.name, sell, sellQuote), /获得 金币5/);
    assert.equal(model.vendorSelection(stock, 3, equipment, 4, 1n), undefined);
    const repair = model.vendorSelection(stock, 3, equipment, 3, 1n);
    const repairQuote = { ...sellQuote, action: 3, durabilityU: '1', maxDurabilityU: '5000', originalDurabilityU: '9000',
        repairedDurabilityU: '4200', repairedMaxDurabilityU: '4200' };
    assert.match(model.vendorQuoteText(equipment.name, repair, repairQuote), /持久 1\/5 → 5\/5\n恢复 5 · 原始上限 9\n当前上限调整为 5 · 绑定状态保持/);
    assert.throws(() => model.vendorQuoteText(equipment.name, repair, { ...repairQuote, instanceId: new Uint8Array(16) }), /不一致/);
});

class SavedNode {
    constructor(data, list) {
        this.name = data._name;
        this.activeInHierarchy = true;
        this.label = { string: '' };
        this.button = { interactable: true };
        this.events = new EventEmitter();
        this.children = data._children.map(ref => new SavedNode(scene[ref.__id__], list));
        this.list = list;
    }
    getChildByName(name) { return this.children.find(row => row.name === name); }
    getChildByPath(name) { return name.split('/').reduce((item, key) => item?.getChildByName(key), this); }
    getComponent(type) { return type === List ? this.list : type === Label ? this.label : this.button; }
    on(event, fn) { this.events.on(event, fn); }
    off(event, fn) { this.events.off(event, fn); }
}
class Label {}
class List {
    static EVENT_ITEM_CLICK = 'list-item-click';
    setItems(items, _renderer, preserve) { this.items = items; this.preserve = preserve; }
}
const Button = { EventType: { CLICK: 'click' } };

test('保存物品详情隔开滚动内容、数量控件和操作按钮', () => {
    assert.equal(detailPanel._active, false);
    const list = child(detailPanel, 'List');
    const topButton = name => {
        const node = child(detailPanel, name), { width, height } = component(node, 'cc.UITransform')._contentSize;
        assert(component(node, 'cc.Button'));
        assert(component(child(node, 'Label'), 'cc.Label'));
        return [node._lpos.x - width / 2, node._lpos.x + width / 2, node._lpos.y + height / 2];
    };
    const bottom = list._lpos.y - component(list, 'cc.UITransform')._contentSize.height / 2;
    assert(bottom > topButton('Minus')[2] && bottom > topButton('Plus')[2]);
    const actions = ['Action1', 'Action2', 'Action3'].map(topButton);
    assert(actions[0][1] < actions[1][0] && actions[1][1] < actions[2][0]);
    const quantity = child(detailPanel, 'Quantity');
    assert(quantity._lpos.y - component(quantity, 'cc.UITransform')._contentSize.height / 2 > actions[0][2]);
    const view = child(list, 'View');
    assert(component(view, 'cc.Mask'));
    assert.equal(scene[component(list, 'cc.ScrollView')._content.__id__], child(view, 'Content'));
});

test('NPC采购只显示通用和本职业，仍能出售旧的其他职业物品', () => {
    const equipmentModel = require('../../../.work/ui-tests/app/EquipmentModel.js');
    const level = require('../../../.work/ui-tests/app/LevelGrowthModel.js');
    const { npcShopSnapshot } = load('client/assets/scripts/app/NpcShopCatalog.ts', id => ({
        './EquipmentModel': equipmentModel, './LevelGrowthModel': level, './SkillAssetsModel': skillAssets,
    })[id] ?? {});
    const definitions = [
        { item_id: 81, kind: 'book', name: '战士书', allowed_jobs: [0] },
        { item_id: 82, kind: 'book', name: '道士书', allowed_jobs: [2] },
        { item_id: 83, kind: 'potion', name: '药品', allowed_jobs: [] },
    ];
    const shop = { key: 'book', roles: ['book'], can_repair: false, offers: [...definitions, { item_id: 84 }].map((row, i) => ({
        id: i+1, item_id: row.item_id, quantity: 1, currency: 'gold', price_units: 8, max_purchase_quantity: 16, required_role_level: 1,
    })), sell_prices: [{ item_id: 82, price_gold: 4 }] };
    const inventory = { level: 12, equipped: [], items: [{ itemId: 82, instanceId: instance, quantity: 1, version, bound: false }] };
    const warrior = npcShopSnapshot(shop, 'entry', inventory, true, definitions, false, 0);
    assert.deepEqual(warrior.buy.map(row => row.itemId), [81, 83]);
    assert.deepEqual(warrior.sell.map(row => row.itemId), [82]);
    assert.deepEqual(npcShopSnapshot(shop, 'entry', inventory, true, definitions, false, 2).buy.map(row => row.itemId), [82, 83]);
});

test('NPC按原矿逐块显示纯度和钱包，确认采用服务端报价', () => {
    const { npcShopSnapshot } = load('client/assets/scripts/app/NpcShopCatalog.ts', id => ({
        './EquipmentModel': require('../../../.work/ui-tests/app/EquipmentModel.js'),
        './LevelGrowthModel': require('../../../.work/ui-tests/app/LevelGrowthModel.js'),
        './MiningModel': require('../../../.work/ui-tests/app/MiningModel.js'), './SkillAssetsModel': skillAssets,
    })[id] ?? {});
    const copper = { itemId: 91, quantity: 1, version, bound: true, sourceKind: 'mining', sourceId: 'a'.repeat(32) };
    const ore = (id, purity, fields) => ({ ...copper, orePurity: purity, instanceId: new Uint8Array(16).fill(id), ...fields });
    const inventory = { level: 20, equipped: [], items: [
        ore(1, 3), ore(2, 4, { bound: false }), ore(3, 1), ore(4, 5, { locked: true }),
        ore(5, 101), ore(6, 5, { sourceId: '旧来源' }), ore(7, 5, { quantity: 2 }), ore(8, 5, { itemId: 92 }),
        ore(9, 23, { itemId: 93, bound: false }),
    ] };
    const definitions = [[91, '铜矿'], [92, '黑铁矿'], [93, '金矿']].map(([item_id, name]) => ({ item_id, name, kind: 'mining_ore' }));
    const shop = { key: 'grocery', roles: ['grocery'], can_repair: false, offers: [], sell_prices: [{ item_id: 92, price_gold: 400 }],
        mineral_buyback: [{ item_id: 91, numerator: 1, denominator: 2, rounding: 'floor_per_ore' },
            { item_id: 93, numerator: 2, denominator: 1, rounding: 'exact' }] };
    const stock = npcShopSnapshot(shop, 'entry', inventory, true, definitions, false, 0);
    const [odd, even, zero, locked, gold] = stock.sell;
    assert.equal(stock.sell.length, 5);
    assert.deepEqual(stock.sell.map(row => row.detail), ['纯度 3 · 1绑定金币/块', '纯度 4 · 2金币/块',
        '纯度 1 · 0绑定金币/块', '纯度 5 · 2绑定金币/块', '纯度 23 · 46金币/块']);
    assert.deepEqual(odd.itemDetails, ['绑定物品', '来源 挖矿所得']);
    assert.match(zero.disabledReason, /无收购价/);
    assert.match(locked.disabledReason, /锁定/);
    assert.equal(model.vendorSelection(stock, 2, zero, 2, 1n), undefined);
    assert.equal(model.vendorSelection(stock, 2, even, 2, 2n), undefined);
    assert.equal(npcShopSnapshot({ ...shop, mineral_buyback: undefined }, 'entry', inventory, true, definitions, false, 0).sell.length, 0);

    const selection = model.vendorSelection(stock, 2, odd, 2, 1n);
    const proceeds = { ...quote, action: 2, offerId: 0, itemId: copper.itemId, instanceId: odd.instanceId,
        quantity: 1, price: '7', boundGold: '7', gold: '0' };
    assert.equal(model.vendorQuoteText(odd.name, selection, proceeds), '出售 铜矿 ×1\n获得 绑定金币7\n物品绑定\n纯度 3');
    assert.throws(() => model.vendorQuoteText(odd.name, selection, { ...proceeds, boundGold: '0', gold: '7' }), /钱包类型/);
    assert.equal(model.sameVendorSelection(selection, { ...selection, mineral: { purity: 5, bound: true } }), false);
    const goldSelection = model.vendorSelection(stock, 2, gold, 2, 1n);
    const unbound = { ...proceeds, itemId: gold.itemId, instanceId: gold.instanceId, bound: false, price: '46', boundGold: '0', gold: '46' };
    assert.match(model.vendorQuoteText(gold.name, goldSelection, unbound), /获得 金币46\n物品非绑定\n纯度 23/);
});

test('触击商店条目仅打开详情，报价确认和真实提交仍使用原商品', async () => {
    const list = new List(), saved = new SavedNode(panel, list), detailList = new List(), detail = new SavedNode(detailPanel, detailList);
    list.node = saved.getChildByName('ItemsList'); detailList.node = detail.getChildByName('List');
    const safe = { getChildByPath: path => path.endsWith('/ItemActions') ? detail : saved };
    const popupNode = new EventEmitter(), estimates = [], commits = [], errors = [];
    const PopupView = { EVENT_CHANGED: 'popup-change' };
    let accept;
    const popup = { node: popupNode, show() { this.top = detail; this.node.emit(PopupView.EVENT_CHANGED); }, confirm() {
        this.top = { name: 'Confirm' }; this.node.emit(PopupView.EVENT_CHANGED);
        return new Promise(resolve => { accept = () => { this.top = detail; this.node.emit(PopupView.EVENT_CHANGED); resolve(true); }; });
    } };
    const cc = { Label, Button, isValid: Boolean };
    const { GameItemActions } = load('client/assets/scripts/app/GameItemActions.ts', id => ({
        cc, '../ui/List': { List }, '../ui/PopupView': { PopupView }, './CombatStatusModel': { detailLines: lines => lines },
    })[id] ?? {});
    const { GameNpcShop } = load('client/assets/scripts/app/GameNpcShop.ts', id => ({
        cc, '../ui/List': { List }, './NpcShopModel': model, './SkillAssetsModel': skillAssets,
    })[id] ?? {});
    const actions = new GameItemActions(safe, popup, error => errors.push(error));
    const shop = new GameNpcShop(safe, popup, {
        quote: async selected => { estimates.push(selected); return quote; },
        commit: async selected => { commits.push(selected); },
        report: error => errors.push(error),
    }, actions);
    shop.open('药店', stock.shopKey);
    shop.refresh(stock);
    list.node.events.emit(List.EVENT_ITEM_CLICK, 0, list.items[0]);
    assert.equal(popup.top, detail);
    assert.equal(detail.getChildByName('Title').label.string, '疗伤药');
    assert.equal(detail.getChildByPath('Action1/Label').label.string, '购买');
    assert.equal(detail.getChildByName('Action2').active, false);
    assert.deepEqual(estimates, []);
    detail.getChildByName('Plus').events.emit('click');
    assert.equal(detail.getChildByName('Quantity').label.string, '数量 4');
    detail.getChildByName('Action1').events.emit('click');
    detail.getChildByName('Action1').events.emit('click');
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(estimates.length, 1);
    assert.deepEqual(commits, []);
    accept();
    await new Promise(resolve => setImmediate(resolve));
    assert.deepEqual(commits.map(entry => [entry.scope, entry.offerId, entry.quantity]), [['角色附着1', 20, 4n]]);
    shop.refresh({ ...stock, scope: '角色附着2' });
    actions.refresh();
    assert.equal(detail.getChildByName('Action1').active, false);
    detail.getChildByName('Action1').events.emit('click');
    assert.equal(estimates.length, 1);
    assert.deepEqual(errors, []);
    actions.dispose();
    shop.dispose();
});

test('真实视图报价后须手动确认，换附着期间旧报价不能提交', async () => {
    const list = new List(), saved = new SavedNode(panel, list);
    list.node = saved.getChildByName('ItemsList');
    const safe = { getChildByPath: () => saved }, failures = [], commits = [], estimates = [];
    let resolveConfirm;
    const popup = { confirm: () => new Promise(resolve => { resolveConfirm = resolve; }) };
    const { GameNpcShop } = load('client/assets/scripts/app/GameNpcShop.ts', id => ({
        cc: { Label, Button, isValid: Boolean }, '../ui/List': { List }, './NpcShopModel': model, './SkillAssetsModel': skillAssets,
    })[id] ?? {});
    const shop = new GameNpcShop(safe, popup, {
        quote: async selected => { estimates.push(selected); return quote; },
        commit: async selected => { commits.push(selected); },
        report: error => failures.push(error),
    });
    const tick = () => new Promise(resolve => setImmediate(resolve));
    const click = path => saved.getChildByPath(path).events.emit('click');
    const selectAndEstimate = () => {
        list.node.events.emit(List.EVENT_ITEM_CLICK, 0, list.items[0]);
        click('Quantity/Plus');
        assert.equal(saved.getChildByPath('Quantity/Value').label.string, '4');
        click('Preview');
        click('Preview');
    };
    shop.open('边界铁匠铺', stock.shopKey);
    shop.refresh(stock);
    selectAndEstimate();
    await tick();
    assert.equal(estimates.length, 1);
    assert.equal(commits.length, 0);
    shop.refresh({ ...stock, scope: '角色附着2' });
    resolveConfirm(true);
    await tick();
    assert.equal(commits.length, 0);
    selectAndEstimate();
    await tick();
    resolveConfirm(true);
    await tick();
    assert.deepEqual(commits.map(entry => [entry.scope, entry.offerId, entry.quantity]), [['角色附着2', 20, 4n]]);
    assert.deepEqual(failures, []);
    shop.dispose();
    assert.equal(list.node.events.listenerCount(List.EVENT_ITEM_CLICK), 0);
});
