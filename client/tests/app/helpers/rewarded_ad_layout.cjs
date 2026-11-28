// 去掉本阶段新入口的原父节点引用，旧托管布局继续参加逐对象校验。
const assert = require('node:assert/strict');

module.exports = function normalizeRewardedAdLayout(scene) {
    const hosting = scene[339], entry = scene[3189];
    assert.equal(hosting._name, 'Hosting');
    assert.equal(entry._name, 'RewardedAd');
    assert.deepEqual(entry._parent, { __id__: 339 });
    assert.deepEqual(hosting._children.filter(ref => ref.__id__ === 3189), [{ __id__: 3189 }]);
    hosting._children = hosting._children.filter(ref => ref.__id__ !== 3189);
    return scene;
};
