// 核对战令仅增加一个默认隐藏的已保存按钮及其触屏位置。
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const normalize = require('./helpers/battle_pass_layout.cjs');

const scene = JSON.parse(readFileSync('client/assets/scenes/Game.scene', 'utf8'));
const component = (node, type) => node._components.map(ref => scene[ref.__id__]).find(row => row.__type__ === type);

test('战令保存为成长页的按钮，先前 3182 个序列化对象参与原摘要', () => {
    const before = normalize(structuredClone(scene)).slice(0, 3182);
    assert.equal(createHash('sha256').update(JSON.stringify(before)).digest('hex'),
        'faae34c761185778e20035bdfca265999567d2bcbee6c515ba46d0c0c4153b31');
    const button = scene[3182], label = scene[button._children[0].__id__];
    assert.equal(button._active, false);
    assert.equal(component(label, 'cc.Label')._string, '战令');
    assert.equal(component(label, 'cc.Label')._fontSize, 30);
    assert.deepEqual(component(button, 'cc.Button').clickEvents, []);
    const area = component(button, 'cc.UITransform')._contentSize;
    const page = component(scene[407], 'cc.UITransform')._contentSize;
    assert(area.width <= page.width && area.height <= page.height);
    assert.deepEqual([button._lpos.x, button._lpos.y], [0, 0]);
});
