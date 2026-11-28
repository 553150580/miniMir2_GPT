// 在当前场景中剥离后续入口，再还原洗炼阶段之前的两个挂接位置。
const assert = require('node:assert/strict');

module.exports = function normalizeEquipmentRefineLayout(scene) {
    require('./growth_shop_layout.cjs')(scene);
    const parent = scene[450];
    assert.equal(parent._name, 'Panels');
    assert.equal(scene[parent._parent.__id__]._name, 'PopupView');
    for (const [id, name] of [[2944, 'EquipmentRefine'], [3110, 'EquipmentRefineMaterials']]) {
        const child = scene[id];
        assert.equal(child._name, name);
        assert.deepEqual(child._parent, { __id__: 450 });
        assert.deepEqual(parent._children.filter(ref => ref.__id__ === id), [{ __id__: id }]);
        parent._children = parent._children.filter(ref => ref.__id__ !== id);
    }
    return scene;
};
