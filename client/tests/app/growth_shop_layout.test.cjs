// 验证商城沿用现有详情，只增加功能页上的已保存触屏入口。
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const normalize = require('./helpers/growth_shop_layout.cjs');

const scene = JSON.parse(readFileSync('client/assets/scenes/Game.scene', 'utf8'));
const component = (node, type) => node._components.map(ref => scene[ref.__id__]).find(row => row.__type__ === type);
const height = node => component(node, 'cc.UITransform')._contentSize.height;

test('成长商城只追加一个默认隐藏的功能入口，原 3175 个对象保持原摘要', () => {
    const before = normalize(structuredClone(scene)).slice(0, 3175);
    assert.equal(createHash('sha256').update(JSON.stringify(before)).digest('hex'),
        '4709efe4f51876668a690978837d084c2e0ad20836a2c314ce765b6ce184b5d2');
    const shop = scene[3175], label = scene[shop._children[0].__id__];
    assert.equal(shop._active, false);
    assert.equal(component(label, 'cc.Label')._string, '成长商城');
    assert.deepEqual(component(shop, 'cc.Button').clickEvents, []);
    for (const neighbor of scene[419]._children.map(ref => scene[ref.__id__]).filter(node => node !== shop)) {
        assert(shop._lpos.y - height(shop) / 2 > neighbor._lpos.y + height(neighbor) / 2);
    }
});
