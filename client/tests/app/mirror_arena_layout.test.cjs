const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const scene = JSON.parse(fs.readFileSync(path.join(__dirname, '../../assets/scenes/Game.scene'), 'utf8'));
const canvas = scene.find(value => value.__type__ === 'cc.Node' && value._name === 'Canvas');
const children = (value) => value._children.map(reference => scene[reference.__id__]);
const nodeAt = (root, route) => route.split('/').reduce((parent, name) => {
    const node = children(parent).find(child => child._name === name);
    assert(node, '已保存节点缺失：' + route);
    return node;
}, root);
const component = (value, type) => {
    const found = value._components.map(reference => scene[reference.__id__]).find(item => item.__type__ === type);
    assert(found, value._name + ' 缺少 ' + type);
    return found;
};
const safe = nodeAt(canvas, 'Safe');
const bounds = node => {
    const { width, height } = component(node, 'cc.UITransform')._contentSize;
    return { left: node._lpos.x - width / 2, right: node._lpos.x + width / 2,
        bottom: node._lpos.y - height / 2, top: node._lpos.y + height / 2 };
};
const overlap = (one, other) => one.left < other.right && one.right > other.left
    && one.bottom < other.top && one.top > other.bottom;

test('镜像入口保存在功能页并默认关闭，点击热区和原入口、面板边界相互独立', () => {
    const functions = nodeAt(safe, 'UIMgr/Panels/Functions');
    const pages = nodeAt(functions, 'Pages'), page = nodeAt(pages, 'Page0');
    const entry = nodeAt(page, 'MirrorArena'), hit = bounds(entry);
    assert.equal(entry._active, false);
    component(entry, 'cc.Button');
    assert.equal(component(nodeAt(entry, 'Label'), 'cc.Label')._string, '镜像竞技');
    for (const child of children(page).filter(node => node !== entry)) {
        assert(!overlap(hit, bounds(child)), child._name);
    }
    const frame = bounds(functions);
    assert(hit.bottom + pages._lpos.y > frame.bottom && hit.left > frame.left && hit.right < frame.right);
});

test('镜像普通面板使用已保存的战报列表、遮罩、模板及所有触屏操作', () => {
    const panel = nodeAt(safe, 'UIMgr/Panels/MirrorArena');
    assert.equal(panel._active, false);
    component(panel, 'cc.BlockInputEvents');
    component(panel, 'cc.Sprite');
    assert(!children(panel).some(node => /^(?:Close|Back)$/.test(node._name)));
    const captions = ['Title', 'Season', 'Status', 'Attempts', 'Score', 'Defense', 'Detail'];
    for (const name of captions) assert(component(nodeAt(panel, name), 'cc.Label')._fontSize >= 24);
    const actions = ['Challenge', 'Resume', 'Refresh', 'Publish'];
    const tabs = ['Offers', 'AttackHistory', 'DefenseHistory'].map(name => 'SectionTabs/' + name);
    for (const name of [...actions, ...tabs]) {
        const button = nodeAt(panel, name);
        component(button, 'cc.Button');
        assert(component(button, 'cc.UITransform')._contentSize.height >= 48);
        assert(component(nodeAt(button, 'Label'), 'cc.Label')._fontSize >= 24);
    }
    const list = nodeAt(panel, 'List'), view = nodeAt(list, 'View');
    const content = nodeAt(view, 'Content'), template = nodeAt(content, 'ItemTemplate');
    const reusable = component(list, '39a82802rlT3Z9OWj3G8DOj');
    assert.equal(scene[component(list, 'cc.ScrollView')._content.__id__], content);
    component(view, 'cc.Mask');
    assert.equal(scene[reusable.viewport.__id__], view);
    assert.equal(scene[reusable.templateNode.__id__], template);
    assert(3 * reusable.cellHeight + 2 * reusable.spacingY <= component(view, 'cc.UITransform')._contentSize.height);
    assert.equal(template._active, false);
    component(template, 'cc.Button');
    assert(component(nodeAt(template, 'Text'), 'cc.Label')._fontSize >= 24);
    for (const name of [...actions, 'Detail', 'SectionTabs']) assert(!overlap(bounds(list), bounds(nodeAt(panel, name))), name);
});

test('镜像战斗复用独立保存的菱形世界，被普通弹窗、顶层确认和重连窗口覆盖', () => {
    const battle = nodeAt(safe, 'MirrorArenaBattle');
    assert.equal(battle._active, false);
    const layerOrder = children(safe).map(node => node._name);
    assert(layerOrder.indexOf('BottomHUD') < layerOrder.indexOf('MirrorArenaBattle'));
    assert(layerOrder.indexOf('MirrorArenaBattle') < layerOrder.indexOf('UIMgr'));
    assert(layerOrder.indexOf('UIMgr') < layerOrder.indexOf('PopupView'));
    const canvasOrder = children(canvas).map(node => node._name);
    assert(canvasOrder.indexOf('Safe') < canvasOrder.indexOf('ReconnectDialog'));

    for (const name of ['Title', 'Status', 'Time', 'Attack', 'Defense']) {
        assert(component(nodeAt(battle, 'Header/' + name), 'cc.Label')._fontSize >= 24);
    }
    for (const name of ['Surrender', 'Return']) component(nodeAt(battle, 'Footer/' + name), 'cc.Button');
    assert.equal(component(nodeAt(battle, 'Header'), 'cc.Widget')._top, 0);
    assert.equal(component(nodeAt(battle, 'Footer'), 'cc.Widget')._bottom, 0);
    component(battle, 'cc.BlockInputEvents');
    const viewport = nodeAt(battle, 'MapViewport'), stretch = component(viewport, 'cc.Widget');
    component(viewport, 'cc.Mask');
    assert.equal(stretch._top, component(nodeAt(battle, 'Header'), 'cc.UITransform')._contentSize.height);
    assert.equal(stretch._bottom, component(nodeAt(battle, 'Footer'), 'cc.UITransform')._contentSize.height);
    assert.equal(stretch._alignFlags, 45);
    assert.deepEqual(children(viewport).map(node => node._name), ['World', 'Loading']);
    const world = nodeAt(viewport, 'World');
    assert.notEqual(world, nodeAt(safe, 'MapViewport/World'));
    assert.deepEqual(children(world).map(node => node._name),
        ['Soil', 'Surface', 'GridLines', 'GroundEffects', 'TargetMarker', 'Corpses', 'Entities', 'Effects']);
    assert.equal(nodeAt(world, 'TargetMarker')._active, false);
    assert.equal(nodeAt(viewport, 'Loading')._active, false);
    component(nodeAt(viewport, 'Loading/Status'), 'cc.Label');
});
