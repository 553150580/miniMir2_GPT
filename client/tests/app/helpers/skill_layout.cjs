const assert = require('node:assert/strict');

// 历史布局只还原新增技能控件和留出控件所需的列表几何。
module.exports = function normalizeSkillControls(scene) {
    const restore = (object, key, current, previous) => {
        assert.equal(object[key], current);
        object[key] = previous;
    };
    const move = (id, key, current, previous) => restore(scene[id]._lpos, key, current, previous);
    const resize = (id, key, current, previous) => restore(scene[id]._contentSize, key, current, previous);
    const slots = ['AttackSlot1', 'AttackSlot2', 'AttackSlot3', 'AttackSlot4', 'ClearSlots', 'SectionTabs'];
    const additionsNow = scene[733]._children.filter(ref => ref.__id__ >= 1929).map(ref => scene[ref.__id__]._name);
    if (additionsNow.length > 0) {
        assert.deepEqual(additionsNow, slots);
        move(737, 'x', -52, 0); move(737, 'y', 368, 355);
        resize(738, 'width', 400, 600); resize(738, 'height', 60, 64);
        move(740, 'y', 285, 250); resize(741, 'width', 604, 600); resize(741, 'height', 94, 110);
        move(743, 'y', 51, -44); resize(744, 'width', 604, 600); resize(744, 'height', 345, 312);
        for (const [node, size] of [[760, 761], [767, 768], [938, 939], [963, 964]]) {
            move(node, 'y', -215, -325);
            resize(size, 'height', 52, 60);
        }
        for (const [node, size] of [[779, 780], [786, 787], [793, 794]]) {
            move(node, 'y', -155, -250);
            resize(size, 'height', 52, 60);
        }
        move(774, 'x', -79, 0); move(774, 'y', -332, -380);
        resize(775, 'width', 438, 600); resize(775, 'height', 54, 40);
        restore(scene[776], '_string', '选技能后点槽位，同一技能可移除。', '自动施法随战斗开启 · 手动快捷可单独设置');
        for (const id of [1416, 1423]) restore(scene[id], '_active', false, true);
        move(1430, 'x', 238, 231); move(1430, 'y', 368, 153); resize(1431, 'height', 45, 52);
    }
    // 发布换用这一份已知的新场景 UUID，仍校验场景对象的其它历史字段。
    if (scene[1]._id === 'ca7d8bf1-9100-59e3-b59b-5ba330141897') scene[1]._id = '0f1590d8-ab8f-56fb-a232-db0f38f5cbd3';
    const additions = [[74, ['TargetDetails']], [733, ['AutoCast', 'SkillAuto', 'Details', ...additionsNow]]];
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
