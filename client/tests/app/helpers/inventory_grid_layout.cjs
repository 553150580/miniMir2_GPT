const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');

const previous = {
    277: 'a6b1260611c15e3cbd33db325ac6fa7f66d3f5308ee1b47135b65e27c1719f74',
    280: '422fb8a54dee10e2ce4180de833eb82ef88b9f2a499553740ee84a5d64affb5a',
    281: 'a72c808b3d3a5931bebaeb1b423bc7a54bb18b1238b8452c13ab0b1c31be02b4',
    282: '3068d6283cdf98fffcfac986a64f340eea96ecb2412cd07b690bdfbd4a90823e',
    283: 'c918d964967bba0e65bad4046c17f3d8a800b11eeb0b789d4b5469556671f62e',
    820: 'd275c7ce085ee579ccb1a2523a62792c1678673b79f41921dbe8d6cbb9c44a7b',
    824: 'a86cef7e45554ffacb8c0dffad6ad28a0e3f5cd7ffe2657e0c5e58781ae1055a',
    825: 'e661850252980ffc8da904505c9e326bc200cb5d03105705d6253618b3f4ae84',
    826: '97303e990550106771bb91f903cbdb731aeb3a9e22463ef8eaaef41e74803cf4',
};

// 历史布局哈希仅允许还原已批准的九个旧对象字段及背包新增的容量节点引用。
module.exports = function normalizeInventoryGridLayout(scene) {
    require('./role_inventory_presentation.cjs')(scene);
    const restore = (index, target, fields) => {
        for (const [name, [current, original]] of Object.entries(fields)) {
            assert.equal(target[name], current, `对象${index}的${name}偏离保存网格`);
            if (original === undefined) delete target[name];
            else target[name] = original;
        }
    };
    restore(277, scene[277]._contentSize, { width: [192, 600], height: [104, 72] });
    restore(280, scene[280]._lpos, { y: [-52, -36] });
    restore(281, scene[281]._contentSize, { width: [176, 592], height: [96, 68] });
    restore(282, scene[282], { _enableWrapText: [true, false] });
    restore(820, scene[820]._contentSize, { width: [294, 600] });
    restore(824, scene[824]._contentSize, { width: [278, 592] });
    restore(825, scene[825], { _enableWrapText: [true, false] });
    for (const [index, width] of [[283, 192], [826, 294]]) {
        restore(index, scene[index], { layoutMode: [2], cellWidth: [width], cellHeight: [104], spacingX: [12], spacingY: [10] });
    }
    for (const [index, hash] of Object.entries(previous)) {
        assert.equal(createHash('sha256').update(JSON.stringify(scene[index])).digest('hex'), hash, `对象${index}有额外布局变化`);
    }
    const inventory = scene[257], capacity = inventory._children.filter(ref => scene[ref.__id__]._name === 'Capacity');
    assert.equal(capacity.length, 1);
    inventory._children = inventory._children.filter(ref => ref !== capacity[0]);
    return scene;
};
