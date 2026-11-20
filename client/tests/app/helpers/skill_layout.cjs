const assert = require('node:assert/strict');

// 历史布局只还原新增技能控件和留出控件所需的列表几何。
module.exports = function normalizeSkillControls(scene) {
    const additions = [[74, ['TargetDetails']], [733, ['AutoCast', 'SkillAuto', 'Details']]];
    for (const [parent, names] of additions) {
        const children = scene[parent]._children;
        const added = children.filter(ref => ref.__id__ >= 1416);
        assert.deepEqual(added.map(ref => scene[ref.__id__]._name), names);
        scene[parent]._children = children.filter(ref => ref.__id__ < 1416);
    }
    assert.equal(scene[743]._lpos.y, -44);
    scene[743]._lpos.y = -20;
    for (const index of [744, 747, 750]) {
        assert.equal(scene[index]._contentSize.height, 312);
        scene[index]._contentSize.height = 360;
    }
    assert.equal(scene[749]._lpos.y, 156);
    scene[749]._lpos.y = 180;
    assert.equal(scene[776]._string, '自动施法随战斗开启 · 手动快捷可单独设置');
    scene[776]._string = '开发候选 · 各技能独立升级';
    return scene;
};
