const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const normalize = require('./helpers/gold_market_layout.cjs');

const scene = JSON.parse(readFileSync('client/assets/scenes/Game.scene', 'utf8'));
const child = (node, name) => scene[node._children.find(ref => scene[ref.__id__]._name === name)?.__id__];
const component = (node, type) => scene[node._components.find(ref => scene[ref.__id__].__type__ === type)?.__id__];

test('金币交易只追加保存的四个入口及面板，原 2762 对象仍通过历史摘要', () => {
    const before = normalize(structuredClone(scene)).slice(0, 2762);
    assert.equal(createHash('sha256').update(JSON.stringify(before)).digest('hex'), '3af11e911b72c0768aebc9f7503d6b5ef952f41d4225edff024d885dfd6b79e9');
});

test('金币市场默认隐藏，四个交易页签及委托 EditBox 均为独立保存组件', () => {
    const entry = child(scene[419], 'GoldMarket'), item = child(scene[419], 'ItemMarket');
    assert.equal(entry._active, false);
    assert(component(entry, 'cc.Button'));
    const height = node => component(node, 'cc.UITransform')._contentSize.height;
    assert(entry._lpos.y + height(entry) / 2 < item._lpos.y - height(item) / 2);
    const [market, place, detail] = ['GoldMarket', 'GoldMarketPlace', 'GoldMarketDetail'].map(name => child(scene[253], name));
    for (const panel of [market, place, detail]) {
        assert.equal(panel._active, false);
        assert(component(panel, 'cc.BlockInputEvents'));
    }
    assert.deepEqual(child(market, 'Tabs')._children.map(ref => scene[ref.__id__]._name), ['Buy', 'Sell', 'Mine', 'Escrow']);
    const list = child(market, 'List'), view = child(list, 'View');
    assert(component(list, 'cc.ScrollView'));
    assert(component(view, 'cc.Mask'));
    assert.equal(child(child(view, 'Content'), 'ItemTemplate')._active, false);
    for (const name of ['Lots', 'Price']) {
        const input = child(place, name), edit = component(input, 'cc.EditBox');
        assert(edit._textLabel && edit._placeholderLabel);
        assert(component(child(input, 'TEXT_LABEL'), 'cc.Label'));
        assert(component(child(input, 'PLACEHOLDER_LABEL'), 'cc.Label'));
    }
});
