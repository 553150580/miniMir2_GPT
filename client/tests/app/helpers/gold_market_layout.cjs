// 历史发布检查仅剥离金币市场的四个明确挂接，并保留其它节点参加原摘要。
const assert = require('node:assert/strict');

module.exports = function normalizeGoldMarketLayout(scene) {
    for (const [parentId, parentName, childId, childName] of [
        [419, 'Page2', 2762, 'GoldMarket'], [253, 'Panels', 2769, 'GoldMarket'],
        [253, 'Panels', 2864, 'GoldMarketPlace'], [253, 'Panels', 2924, 'GoldMarketDetail'],
    ]) {
        const parent = scene[parentId], child = scene[childId];
        assert.equal(parent._name, parentName);
        assert.equal(child._name, childName);
        assert.deepEqual(child._parent, { __id__: parentId });
        assert.deepEqual(parent._children.filter(ref => ref.__id__ === childId), [{ __id__: childId }]);
        parent._children = parent._children.filter(ref => ref.__id__ !== childId);
    }
    return scene;
};
