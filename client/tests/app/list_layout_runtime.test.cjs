const { test } = require('node:test');
const assert = require('node:assert/strict');
const { actualList, Button, List, cc } = require('./helpers/list_restore_harness.cjs');

const million = Array.from({ length: 1_000_000 }, (_, index) => index);
const render = (node, value, index) => { node.bound = { value, index }; };

test('横向百万项列表沿真实Creator负X读取，滚动、复用和点击都使用原线性索引', () => {
    const { list, node, scroll, content, userScroll } = actualList('横向', { templateWidth: 64,
        layout: { layoutMode: 1, cellHeight: 52, spacingX: 10, spacingY: 8 } });
    try {
        assert.equal(scroll.horizontal, true); assert.equal(scroll.vertical, false);
        list.setItems(million, render);
        assert.equal(content.getComponent(cc.UITransform).width, 73_999_990);
        assert.equal(content.getComponent(cc.UITransform).height, 280);
        assert.equal(list.rows.get(0).getComponent(cc.UITransform).width, 64);
        const initial = new Set(list.rows.values());
        userScroll(7400);
        assert.equal(scroll.getScrollOffset().x, -7400);
        assert.ok(!list.rows.has(0));
        const row = list.rows.get(100);
        assert.ok(initial.has(row));
        const selected = [];
        node.on(List.EVENT_ITEM_CLICK, (...args) => selected.push(args));
        row.emit(Button.EventType.CLICK);
        assert.deepEqual(selected, [[100, 100, row]]);
        const stops = scroll.stops;
        list.setItems(million, render);
        assert.equal(scroll.stops, stops);
        assert.equal(scroll.getScrollOffset().x, -7400);
        list.scrollToIndex(million.length - 1);
        assert.equal(scroll.getScrollOffset().x, -73_999_700);
        assert.ok(list.rows.has(million.length - 1));
        assert.ok(list.visibleNodeCount + list.spare.length <= 10);
        list.restoreScrollOffset(145);
        assert.equal(scroll.getScrollOffset().x, -145);
        list.setItems([100, 99, 98], render);
        assert.equal(scroll.getScrollOffset().x, 0);
        row.emit(Button.EventType.CLICK);
        const rebound = list.indices.get(row);
        if (rebound === undefined) assert.equal(selected.length, 1);
        else assert.deepEqual(selected[1], [rebound, [100, 99, 98][rebound], row]);
    } finally { list.onDisable(); list.onDestroy(); }
});

test('网格百万项按行换列，视口变窄保留首个可见条目并重新定位池中的条目', () => {
    const { list, node, scroll, content, resize } = actualList('网格', {
        layout: { layoutMode: 2, cellWidth: 64, cellHeight: 52, spacingX: 10, spacingY: 8 } });
    try {
        assert.equal(scroll.horizontal, false); assert.equal(scroll.vertical, true);
        list.setItems(million, render);
        const size = content.getComponent(cc.UITransform);
        assert.equal(size.width, 290); assert.equal(size.height, 14_999_992);
        assert.deepEqual(list.rows.get(0).position, { x: -113, y: 0, z: 0 });
        assert.deepEqual(list.rows.get(1).position, { x: -39, y: 0, z: 0 });
        assert.deepEqual(list.rows.get(4).position, { x: -113, y: -60, z: 0 });

        list.scrollToIndex(44);
        assert.equal(scroll.getScrollOffset().y, 660);
        const retained = list.rows.get(44);
        assert.ok(retained);
        resize(215, 280);
        assert.equal(scroll.getScrollOffset().y, 840);
        assert.equal(scroll.getScrollOffset().x, 0);
        assert.equal(list.rows.get(44), retained);
        assert.deepEqual(retained.position, { x: 72.5, y: -840, z: 0 });
        const clicks = [];
        node.on(List.EVENT_ITEM_CLICK, (index, value) => clicks.push({ index, value }));
        retained.emit(Button.EventType.CLICK);
        assert.deepEqual(clicks, [{ index: 44, value: 44 }]);
        assert.ok(list.visibleNodeCount + list.spare.length <= 33);

        list.setItems([4, 3, 2, 1, 0], render, false);
        assert.equal(scroll.getScrollOffset().y, 0);
        assert.deepEqual(list.rows.get(4).bound, { index: 4, value: 0 });
        assert.equal(list.visibleNodeCount + list.spare.length, 5);
    } finally { list.onDisable(); list.onDestroy(); }
});

test('原垂直模板仅有rowHeight时保持原位置和模板尺寸，现代垂直布局可使用格高和间隔', () => {
    const legacy = actualList('旧垂直');
    const modern = actualList('垂直配置', { layout: { cellWidth: 140, cellHeight: 40, spacingY: 10 } });
    try {
        const entries = Array.from({ length: 20 }, (_, index) => index);
        legacy.list.setItems(entries, render);
        modern.list.setItems(entries, render);
        assert.deepEqual(legacy.list.rows.get(2).position, { x: 0, y: -180, z: 0 });
        assert.equal(legacy.list.rows.get(2).getComponent(cc.UITransform).width, 290);
        assert.equal(modern.content.getComponent(cc.UITransform).height, 990);
        assert.deepEqual(modern.list.rows.get(2).position, { x: -75, y: -100, z: 0 });
        assert.equal(modern.list.rows.get(2).getComponent(cc.UITransform).height, 40);
        modern.list.scrollToIndex(7);
        assert.equal(modern.scroll.getScrollOffset().y, 350);
        assert.ok(modern.list.rows.has(7));
    } finally {
        for (const { list } of [legacy, modern]) { list.onDisable(); list.onDestroy(); }
    }
});
