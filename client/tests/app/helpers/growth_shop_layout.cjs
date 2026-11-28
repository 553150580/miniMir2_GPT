// 将已登记的商城入口从历史场景摘要中剥离。
const assert = require('node:assert/strict');

module.exports = function normalizeGrowthShopLayout(scene) {
    require('./battle_pass_layout.cjs')(scene);
    const parent = scene[419], shop = scene[3175];
    assert.equal(parent._name, 'Page2');
    assert.equal(scene[parent._parent.__id__]._name, 'Pages');
    assert.equal(shop._name, 'GrowthShop');
    assert.deepEqual(shop._parent, { __id__: 419 });
    assert.deepEqual(parent._children.filter(ref => ref.__id__ === 3175), [{ __id__: 3175 }]);
    parent._children = parent._children.filter(ref => ref.__id__ !== 3175);
    return scene;
};
