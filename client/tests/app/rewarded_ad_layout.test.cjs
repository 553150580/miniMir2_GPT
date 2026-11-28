// 核对保存的托管视频入口高度和旧控件的实际触屏空间。
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { readFileSync } = require('node:fs');
const { test } = require('node:test');
const normalize = require('./helpers/rewarded_ad_layout.cjs');

test('激励视频保存为默认隐藏的托管入口，不遮挡现有控件', () => {
    const scene = JSON.parse(readFileSync('client/assets/scenes/Game.scene', 'utf8'));
    const before = normalize(structuredClone(scene)).slice(0, 3189);
    assert.equal(createHash('sha256').update(JSON.stringify(before)).digest('hex'),
        '9c6515628563ac6c67199258d88efac3efaf21e9eccfd27f394c6aab43bd1606');
    const button = scene[3189], label = scene[3193], hosting = scene[339], receipt = scene[2017];
    const component = (node, type) => node._components.map(ref => scene[ref.__id__]).find(row => row.__type__ === type);
    const size = node => component(node, 'cc.UITransform')._contentSize;
    assert.equal(button._active, false);
    assert.equal(component(label, 'cc.Label')._string, '激励视频');
    assert.equal(size(button).height, 48);
    assert(button._lpos.y - size(button).height / 2 > -size(hosting).height / 2);
    assert(button._lpos.y + size(button).height / 2 < receipt._lpos.y - size(receipt).height / 2);
    assert.deepEqual(component(button, 'cc.Button').clickEvents, []);
});
