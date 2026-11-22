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
