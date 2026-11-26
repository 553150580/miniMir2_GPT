const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const normalizeItemMarketLayout = require('./helpers/item_market_layout.cjs');

const scene = JSON.parse(readFileSync('client/assets/scenes/Game.scene', 'utf8'));
const child = (parent, name) => scene[parent._children.find(ref => scene[ref.__id__]._name === name)?.__id__];
const component = (node, type) => scene[node._components.find(ref => scene[ref.__id__].__type__ === type)?.__id__];

test('市场只增加已登记的四个挂接，历史 2573 个场景对象全部参与原摘要', () => {
    const baseline = normalizeItemMarketLayout(structuredClone(scene)).slice(0, 2573);
    assert.equal(createHash('sha256').update(JSON.stringify(baseline)).digest('hex'),
        'df147f19f8bd5a0e90271c8f9310172ce03602038983c1c023c881f18eaed950');
});

test('交易页默认关闭，三页及两个输入面板使用保存的列表、按钮与原生 EditBox', () => {
    const entry = child(scene[419], 'ItemMarket');
    assert(component(entry, 'cc.Button'));
    assert.equal(entry._active, false);
    const [market, detail, listing] = ['ItemMarket', 'ItemMarketOrder', 'ItemMarketListing'].map(name => child(scene[253], name));
    for (const panel of [market, detail, listing]) {
        assert.equal(panel._active, false);
        assert(component(panel, 'cc.BlockInputEvents'));
    }
    assert.deepEqual(child(market, 'Tabs')._children.map(ref => scene[ref.__id__]._name), ['Public', 'Mine', 'Escrow']);
    for (const panel of [market, listing]) {
        const list = child(panel, 'List'), view = child(list, 'View'), content = child(view, 'Content');
        assert(component(list, 'cc.ScrollView'));
        assert(component(view, 'cc.Mask'));
        assert.equal(child(content, 'ItemTemplate')._active, false);
    }
    for (const input of [child(detail, 'Quantity'), child(listing, 'Quantity'), child(listing, 'Price')]) {
        const edit = component(input, 'cc.EditBox');
        assert(component(child(input, 'TEXT_LABEL'), 'cc.Label'));
        assert(component(child(input, 'PLACEHOLDER_LABEL'), 'cc.Label'));
        assert(edit._textLabel && edit._placeholderLabel);
    }
});
