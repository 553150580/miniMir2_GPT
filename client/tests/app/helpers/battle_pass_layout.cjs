// 只去除战令新增的入口引用，原布局仍参加历次摘要检测。
const assert = require('node:assert/strict');

module.exports = function normalizeBattlePassLayout(scene) {
    const parent = scene[407], battlePass = scene[3182];
    assert.equal(parent._name, 'Page1');
    assert.equal(scene[parent._parent.__id__]._name, 'Pages');
    assert.equal(battlePass._name, 'BattlePass');
    assert.deepEqual(battlePass._parent, { __id__: 407 });
    assert.deepEqual(parent._children.filter(ref => ref.__id__ === 3182), [{ __id__: 3182 }]);
    parent._children = parent._children.filter(ref => ref.__id__ !== 3182);
    return scene;
};
