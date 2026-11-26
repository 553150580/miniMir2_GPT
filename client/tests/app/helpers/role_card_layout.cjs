// 历史布局测试只剥离本阶段明确新增的两个双月卡引用。
const assert = require('node:assert/strict');

module.exports = function normalizeRoleCardLayout(scene) {
    const attachments = [[339, 'Hosting', 2526], [253, 'Panels', 2533]];
    for (const [parentId, parentName, childId] of attachments) {
        const parent = scene[parentId], child = scene[childId];
        assert.equal(parent._name, parentName);
        assert.equal(child._name, 'RoleCards');
        assert.equal(child.__type__, 'cc.Node');
        assert.deepEqual(child._parent, { __id__: parentId });
        assert.deepEqual(parent._children.filter(ref => ref.__id__ === childId), [{ __id__: childId }]);
        parent._children = parent._children.filter(ref => ref.__id__ !== childId);
    }
    return scene;
};
