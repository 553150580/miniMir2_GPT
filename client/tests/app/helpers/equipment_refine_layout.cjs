// 历史布局只剥离洗炼阶段明确增加的两个 PopupView 挂接。
const assert = require('node:assert/strict');

module.exports = function normalizeEquipmentRefineLayout(scene) {
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
