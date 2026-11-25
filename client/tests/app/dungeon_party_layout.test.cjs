// 核对讨伐组队面板是独立保存布局，并沿用原弹窗、命中和虚拟列表边界。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');

const scene = JSON.parse(fs.readFileSync(path.resolve(__dirname, '../../assets/scenes/Game.scene'), 'utf8'));
const panels = 'UIMgr/Panels';
const party = panels + '/DungeonParty';
const nodes = new Map();
function walk(index, prefix = '') {
    const value = scene[index], key = prefix + '/' + value._name;
    assert(!nodes.has(key), '保存节点路径重复：' + key);
    nodes.set(key, index);
    for (const ref of value._children) walk(ref.__id__, key);
}
walk(scene[0].scene.__id__);
const safe = Array.from(nodes.keys()).find(key => key.endsWith('/Canvas/Safe'));
function index(relative) {
    const key = safe + '/' + relative;
    assert(nodes.has(key), '缺少保存节点：' + key);
    return nodes.get(key);
}
const node = relative => scene[index(relative)];
const children = value => value._children.map(ref => scene[ref.__id__]);
function component(value, type) {
    const result = value._components.map(ref => scene[ref.__id__]).find(entry => entry.__type__ === type);
    assert(result, value._name + ' 缺少组件 ' + type);
    return result;
}
const size = value => component(value, 'cc.UITransform')._contentSize;
function bounds(value) {
    const { x, y } = value._lpos, { width, height } = size(value);
    return { l: x - width / 2, r: x + width / 2, b: y - height / 2, t: y + height / 2 };
}
const overlap = (a, b) => a.l < b.r && a.r > b.l && a.b < b.t && a.t > b.b;

// 将早期镜像入口和赛季窗口的已知挂载引用剥离后，再逐字段核对原讨伐布局快照。
function beforeMirror(count) {
    const restored = structuredClone(scene.slice(0, count));
    const mounts = [
        ['', 'MirrorArenaBattle'],
        [panels, panels + '/MirrorArena'],
        [panels, panels + '/MirrorSeason'],
        [panels + '/Functions/Pages/Page0', panels + '/Functions/Pages/Page0/MirrorArena'],
    ];
    for (const [parent, child] of mounts) {
        const parentId = parent ? index(parent) : nodes.get(safe), childId = index(child);
        assert(parentId < count && childId >= count);
        assert.deepEqual(scene[childId]._parent, { __id__: parentId });
        const original = restored[parentId];
        assert.deepEqual(original._children.filter(ref => ref.__id__ === childId), [{ __id__: childId }]);
        original._children = original._children.filter(ref => ref.__id__ !== childId);
    }
    return restored;
}

test('只追加原首领的入口与独立普通面板，保留所有先前保存组件与布局', () => {
    const oldCount = 2020;
    const before = beforeMirror(oldCount);
    for (const [parent, child] of [[panels, party], [panels + '/BossChallenge', panels + '/BossChallenge/Dungeon']]) {
        const original = before[index(parent)];
        assert.deepEqual(original._children.filter(ref => ref.__id__ >= oldCount), [{ __id__: index(child) }]);
        original._children = original._children.filter(ref => ref.__id__ < oldCount);
    }
    assert.equal(crypto.createHash('sha256').update(JSON.stringify(before)).digest('hex'),
        'b3fcd906297db659213dad487708b5b8e6b6ce629e5d12b21778f63f1d4eb908');
    const ids = scene.filter(value => value.__type__ === 'cc.Node').map(value => value._id);
    assert.equal(ids.length, new Set(ids).size);
    component(node(party), 'cc.BlockInputEvents');
    component(node(party), 'cc.Sprite');
    assert.equal(node(party)._active, false);
    assert(!children(node(party)).some(value => /close|back/i.test(value._name)));
    component(node('PopupView/Panels/Confirm'), 'cc.BlockInputEvents');
    const dungeon = node(panels + '/BossChallenge/Dungeon');
    component(dungeon, 'cc.Button');
    for (const name of ['Personal', 'World', 'Rewards', 'Status', 'Action', 'Refresh']) {
        assert(!overlap(bounds(dungeon), bounds(node(panels + '/BossChallenge/' + name))), '入口遮挡' + name);
    }
});

test('四个可滚动列表各用保存的遮罩与点击模板，默认只从副本页开始', () => {
    const listNames = ['TemplateList', 'RoomList', 'MemberList', 'HireList'];
    for (const [offset, name] of listNames.entries()) {
        const prefix = party + '/' + name, list = node(prefix), view = node(prefix + '/View');
        const scroll = component(list, 'cc.ScrollView');
        const virtual = list._components.map(ref => scene[ref.__id__]).find(value => value.rowHeight !== undefined);
        assert(virtual, '缺少编辑器保存的 List');
        assert.equal(list._active, offset === 0);
        assert.equal(scroll.horizontal, false);
        assert.equal(scroll.vertical, true);
        assert.equal(scroll._content.__id__, index(prefix + '/View/Content'));
        assert.equal(virtual.viewport.__id__, index(prefix + '/View'));
        assert.equal(virtual.scrollView.__id__, list._components.find(ref => scene[ref.__id__] === scroll).__id__);
        assert.equal(virtual.layoutMode, 0);
        assert.equal(virtual.cellHeight, 72);
        assert.equal(virtual.spacingY, 6);
        component(view, 'cc.Mask');
        component(view, 'cc.Widget');
        const rowPath = prefix + '/View/Content/ItemTemplate', row = node(rowPath);
        assert.equal(virtual.templateNode.__id__, index(rowPath));
        assert.equal(row._active, false);
        assert.equal(size(row).width, virtual.cellWidth);
        assert.equal(size(row).height, virtual.cellHeight);
        assert(size(row).width <= size(view).width);
        component(row, 'cc.Button');
        assert.equal(component(node(rowPath + '/Text'), 'cc.Label')._fontSize, 24);
    }
    assert.deepEqual(bounds(node(party + '/TemplateList')), bounds(node(party + '/RoomList')));
    assert.deepEqual(bounds(node(party + '/RoomList')), bounds(node(party + '/MemberList')));
    assert.deepEqual(bounds(node(party + '/MemberList')), bounds(node(party + '/HireList')));
});

test('原生操作与招募条件各有独立触屏空间，列表与固定按钮命中区互不遮挡', () => {
    const buttons = ['TemplatesTab', 'RoomsTab', 'MembersTab', 'HiresTab', 'LevelDown', 'LevelUp', 'Create', 'Join', 'Leave', 'Refresh', 'Kick', 'Update', 'Start'];
    const toggles = ['JobWarrior', 'JobMage', 'JobTaoist', 'AllowSameJob'];
    const labels = ['Headline', 'Status', 'Selection', 'MinimumLevel', 'Conditions', 'Message'];
    const footprint = bounds(node(party)), names = [...buttons, ...toggles, ...labels, 'TemplateList'];
    for (const [offset, name] of names.entries()) {
        const current = node(party + '/' + name), box = bounds(current);
        assert(box.l >= footprint.l && box.r <= footprint.r && box.b >= footprint.b && box.t <= footprint.t, '控件超出面板：' + name);
        for (const other of names.slice(offset + 1)) assert(!overlap(box, bounds(node(party + '/' + other))), name + '遮挡' + other);
        if (labels.includes(name)) assert(component(current, 'cc.Label')._fontSize >= 24);
        if (buttons.includes(name) || toggles.includes(name)) {
            const type = toggles.includes(name) ? 'cc.Toggle' : 'cc.Button';
            const control = component(current, type);
            assert(size(current).width >= 44 && size(current).height >= 44);
            assert.deepEqual(control.clickEvents, []);
            assert(component(node(party + '/' + name + '/Label'), 'cc.Label')._fontSize >= 24);
            if (type === 'cc.Toggle') {
                assert.equal(current._active, true, '招募条件必须在组队面板开启后可见并可触摸：' + name);
                assert.equal(scene[control._checkMark.__id__].__type__, 'cc.Sprite');
                assert.equal(control._isChecked, name !== 'AllowSameJob');
                assert.deepEqual(control.checkEvents, []);
            }
        }
    }
});

test('佣兵页只改变原面板的页签排列，新增按钮复用当前页的条件区域', () => {
    const originalCount = 2227;
    const restored = beforeMirror(originalCount);
    const panel = restored[index(party)];
    assert.deepEqual(panel._children.filter(ref => ref.__id__ >= originalCount).map(ref => scene[ref.__id__]._name),
        ['HiresTab', 'HireList', 'HireAdd']);
    panel._children = panel._children.filter(ref => ref.__id__ < originalCount);
    for (const [offset, tab] of ['TemplatesTab', 'RoomsTab', 'MembersTab'].entries()) {
        const button = restored[index(party + '/' + tab)];
        const label = restored[index(party + '/' + tab + '/Label')];
        button._lpos.x = (offset - 1) * 206;
        restored[button._components[0].__id__]._contentSize.width = 186;
        restored[label._components[0].__id__]._contentSize.width = 178;
    }
    assert.equal(crypto.createHash('sha256').update(JSON.stringify(restored)).digest('hex'),
        'c4f637ed852c49f6125b2071c58bdc8b1c5517e58ccdd2ddcd294eee810a5dd4');
    assert.equal(node(party + '/HiresTab')._active, false);
    const add = node(party + '/HireAdd');
    component(add, 'cc.Button');
    assert.equal(add._active, false);
    const replaced = node(party + '/MinimumLevel');
    assert.deepEqual(add._lpos, replaced._lpos);
    assert.equal(size(add).width, size(replaced).width);
    assert.equal(component(node(party + '/HireAdd/Label'), 'cc.Label')._string, '加入佣兵');
});
