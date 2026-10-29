const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const scene = JSON.parse(fs.readFileSync(process.env.MINIMIR_INVENTORY_SCENE_SOURCE || 'client/assets/scenes/Game.scene', 'utf8'));
const byPath = path => {
    let node = scene.find(x => x.__type__ === 'cc.Node' && x._name === 'Canvas');
    for (const name of path.split('/')) node = node._children.map(x => scene[x.__id__]).find(x => x._name === name);
    return node;
};
const component = (node, type) => node._components.map(x => scene[x.__id__]).find(x => x.__type__ === type);
const base = 'Safe/UIMgr/Panels/Inventory';
function rect(path) {
    const node = byPath(base + '/' + path), t = component(node, 'cc.UITransform'); let x = 0, y = 0, n = node;
    while (n !== byPath(base)) { assert.deepEqual(n._lscale, { __type__: 'cc.Vec3', x: 1, y: 1, z: 1 }); x += n._lpos.x; y += n._lpos.y; n = scene[n._parent.__id__]; }
    return { left: x - t._anchorPoint.x * t._contentSize.width, right: x + (1 - t._anchorPoint.x) * t._contentSize.width,
        bottom: y - t._anchorPoint.y * t._contentSize.height, top: y + (1 - t._anchorPoint.y) * t._contentSize.height };
}

test('保存Mask与全部固定按钮不相交，原失败触点位于可见列表外，固定控件原尺寸保持', () => {
    const view = rect('List/View'), fixed = ['ItemLock', 'Equip', 'Recycle'].map(rect);
    assert.equal(view.top, 200); assert.equal(view.bottom - fixed[0].top, 12);
    for (const button of fixed) assert(Math.min(view.right, button.right) <= Math.max(view.left, button.left) || Math.min(view.top, button.top) <= Math.max(view.bottom, button.bottom));
    const oldTouch = { x: 0, y: -233.38889999999992 };
    assert(!(oldTouch.x >= view.left && oldTouch.x <= view.right && oldTouch.y >= view.bottom && oldTouch.y <= view.top));
    assert.deepEqual(component(byPath(base), 'cc.UITransform')._contentSize, { __type__: 'cc.Size', width: 660, height: 820 });
    assert.deepEqual(component(byPath(base + '/List'), 'cc.UITransform')._contentSize, { __type__: 'cc.Size', width: 600, height: 470 });
    assert.deepEqual(component(byPath(base + '/ItemLock'), 'cc.UITransform')._contentSize, { __type__: 'cc.Size', width: 200, height: 54 });
    assert.equal(component(byPath(base + '/List/View/Content/ItemTemplate'), 'cc.UITransform')._contentSize.height, 72);
});

test('保存Widget约束和初始Content顶对齐一致，ScrollView与List共用真View而非宿主高度', () => {
    const host = byPath(base + '/List'), view = byPath(base + '/List/View'), content = byPath(base + '/List/View/Content');
    const widget = component(view, 'cc.Widget'), viewTransform = component(view, 'cc.UITransform'), hostSize = component(host, 'cc.UITransform')._contentSize;
    assert(widget); assert.equal(widget._alignFlags, 45); assert.equal(widget._alignMode, 2); assert.equal(widget._top, 0); assert.equal(widget._bottom, 56);
    assert.equal(viewTransform._contentSize.height, hostSize.height - widget._top - widget._bottom);
    assert.equal(view._lpos.y, (widget._bottom - widget._top) / 2);
    assert.equal(content._lpos.y, viewTransform._contentSize.height / 2); assert.equal(component(content, 'cc.UITransform')._anchorPoint.y, 1);
    assert.equal(component(content, 'cc.Widget'), undefined, '动态内容高度由List管理，不能保存fill Widget覆盖');
    const scroll = component(host, 'cc.ScrollView'), list = host._components.map(x => scene[x.__id__]).find(x => x.viewport && x.rowHeight);
    assert.equal(scene[scroll._content.__id__], content); assert.equal(scene[content._parent.__id__], view); assert.equal(scene[list.viewport.__id__], view);
    assert(component(view, 'cc.Mask')); assert.equal(list.rowHeight, 72); assert.equal(list.overscan, 2);
});

test('历史HUD投影只接受本轮四对象精确变化与唯一Widget，不掩盖未知字段或其它布局修改', () => {
    const { normalizeInventoryViewport } = require('./helpers/auto_recycle_layout.cjs');
    for (const change of [s => { s[270]._lpos.y = 27; }, s => { s[271]._contentSize.width = 599; }, s => { s[273]._lscale.x = 2; },
        s => { s[274]._anchorPoint.y = .5; }, s => { s[1190]._bottom = 55; }, s => { s[270]._components.push({ __id__: 1190 }); },
        s => { s.push(structuredClone(s[1190])); }]) {
        const tampered = structuredClone(scene); change(tampered); assert.throws(() => normalizeInventoryViewport(tampered));
    }
    const projected = normalizeInventoryViewport(structuredClone(scene));
    assert.equal(projected[271]._contentSize.height, 470); assert.equal(scene[271]._contentSize.height, 414, '真实布局不能被历史投影改写');
    const other = structuredClone(scene); other[258]._contentSize.width = 661; normalizeInventoryViewport(other);
    assert.equal(other[258]._contentSize.width, 661, '投影不得还原其它面板字段以绕过原整段哈希');
    const hash = s => require('node:crypto').createHash('sha256').update(JSON.stringify(s.slice(0, 733))).digest('hex');
    assert.notEqual(hash(other), hash(projected));
});
