// 历史布局只剥离明确登记的四个市场挂接，原节点仍逐项参加旧摘要校验。
const assert = require('node:assert/strict');

module.exports = function normalizeItemMarketLayout(scene) {
    for (const [parentId, parentName, childId, childName] of [
        [419, 'Page2', 2573, 'ItemMarket'], [253, 'Panels', 2580, 'ItemMarket'],
        [253, 'Panels', 2665, 'ItemMarketOrder'], [253, 'Panels', 2698, 'ItemMarketListing'],
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
