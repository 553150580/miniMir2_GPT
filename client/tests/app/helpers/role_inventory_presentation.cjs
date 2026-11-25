const assert = require('node:assert/strict');

// 历史整段哈希只还原已明确调整的角色和背包位置，未知字段继续由原哈希保护。
module.exports = function normalizeRoleInventoryPresentation(scene) {
    const refs = [257, 298].map(index => (scene[index]?._children ?? []).map(ref => scene[ref.__id__]?._name));
    if (!refs.some(names => names.includes('SectionTabs'))) return scene;
    const restore = (value, key, current, previous) => {
        assert.equal(value[key], current);
        value[key] = previous;
    };
    const pos = (index, axis, current, previous) => restore(scene[index]._lpos, axis, current, previous);
    const extent = (index, axis, current, previous) => restore(scene[index]._contentSize, axis, current, previous);
    const prune = (index, added) => {
        assert.deepEqual(scene[index]._children.filter(ref => added.includes(scene[ref.__id__]._name)).map(ref => scene[ref.__id__]._name), added);
        scene[index]._children = scene[index]._children.filter(ref => !added.includes(scene[ref.__id__]._name));
    };
    prune(257, ['CapacityBadge', 'SectionTabs']);
    prune(298, ['SectionTabs', 'EquipmentBoard', 'EquipmentListTitle']);
    pos(261, 'x', -202, 0); extent(262, 'width', 175, 600); extent(262, 'height', 60, 64);
    pos(264, 'y', 227, 250); extent(265, 'height', 46, 110);
    pos(305, 'y', 216, 250); extent(306, 'width', 592, 600); extent(306, 'height', 79, 110);
    pos(308, 'y', -177, -35); pos(314, 'y', 77.5, 235);
    for (const index of [309, 312, 315]) extent(index, 'height', 155, 470);
    pos(325, 'x', -149, -145); pos(332, 'x', 149, 145);
    for (const index of [325, 332]) pos(index, 'y', -303, -325);
    for (const index of [326, 333]) { extent(index, 'width', 247, 260); extent(index, 'height', 56, 60); }
    for (const index of [330, 337]) { extent(index, 'width', 239, 252); extent(index, 'height', 51, 56); }
    for (const index of [263, 304]) {
        const color = scene[index]._color;
        restore(color, 'r', 241, 223); restore(color, 'g', 206, 193); restore(color, 'b', 153, 119);
    }
    return scene;
};
