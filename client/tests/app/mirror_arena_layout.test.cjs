const { test } = require('node:test');
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
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
const fits = (inner, outer) => inner.left >= outer.left && inner.right <= outer.right
    && inner.bottom >= outer.bottom && inner.top <= outer.top;

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

    const header = nodeAt(battle, 'Header'), status = bounds(nodeAt(header, 'Status'));
    for (const name of ['Title', 'Status', 'Time', 'Attack', 'Defense']) {
        assert(component(nodeAt(battle, 'Header/' + name), 'cc.Label')._fontSize >= 24);
    }
    for (const name of ['Attack', 'Defense']) {
        const fighter = bounds(nodeAt(header, name)), margin = component(header, 'cc.UITransform')._contentSize.height / 2;
        assert(fighter.bottom >= -margin + 16 && fighter.top <= status.bottom, '生命行应避开分隔线和场次状态');
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

test('赛季入口默认关闭，旧镜像仅变更已验证的入口引用和标题、生命行座标', () => {
    const oldCount = 2445, original = structuredClone(scene.slice(0, oldCount));
    const panels = nodeAt(safe, 'UIMgr/Panels'), lobby = nodeAt(panels, 'MirrorArena');
    const button = nodeAt(lobby, 'SeasonLedger'), season = nodeAt(panels, 'MirrorSeason'), hit = bounds(button);
    assert.equal(button._active, false);
    component(button, 'cc.Button');
    assert(fits(hit, bounds(lobby)));
    for (const existing of children(lobby).filter(node => node !== button)) {
        assert(!overlap(hit, bounds(existing)), '赛季入口遮挡 ' + existing._name);
    }
    for (const [parent, added] of [[panels, season], [lobby, button]]) {
        const parentIndex = scene.indexOf(parent), childIndex = scene.indexOf(added);
        assert(parentIndex < oldCount && childIndex >= oldCount);
        assert.deepEqual(added._parent, { __id__: parentIndex });
        const node = original[parentIndex];
        assert.deepEqual(node._children.filter(ref => ref.__id__ >= oldCount), [{ __id__: childIndex }]);
        node._children = node._children.filter(ref => ref.__id__ !== childIndex);
    }
    for (const [name, previousWidth] of [['Title', 610], ['Season', 606]]) {
        const saved = nodeAt(lobby, name);
        original[scene.indexOf(saved)]._lpos.x = 0;
        original[scene.indexOf(component(saved, 'cc.UITransform'))]._contentSize.width = previousWidth;
    }
    for (const name of ['Attack', 'Defense']) {
        original[scene.indexOf(nodeAt(safe, `MirrorArenaBattle/Header/${name}`))]._lpos.y = -46;
    }
    assert.equal(crypto.createHash('sha256').update(JSON.stringify(original)).digest('hex'),
        '4301eb5a5710d160a4afa527649eeb8af0ee8c75071071ab7cce3efcd7545d01');
});

test('赛季排行榜和领奖窗口沿统一普通弹窗，窄屏按钮、条目及多行详情有独立空间', () => {
    const panels = nodeAt(safe, 'UIMgr/Panels'), season = nodeAt(panels, 'MirrorSeason');
    const ledger = nodeAt(panels, 'MirrorArena/SeasonLedger');
    assert.equal(season._active, false);
    component(season, 'cc.Sprite');
    component(season, 'cc.BlockInputEvents');
    const labels = ['Title', 'Headline', 'Balance', 'Status', 'Detail'];
    const actions = ['Refresh', 'Claim', 'Exchange'];
    assert.deepEqual(children(season).map(node => node._name),
        ['Title', 'Headline', 'Balance', 'Status', 'Tabs', 'List', 'Detail', ...actions]);
    const box = bounds(season);
    for (const [offset, node] of children(season).entries()) {
        const hit = bounds(node);
        assert(fits(hit, box), '超出赛季面板 ' + node._name);
        for (const peer of children(season).slice(offset + 1))
            assert(!overlap(hit, bounds(peer)), node._name + ' 遮挡 ' + peer._name);
    }
    for (const name of labels) assert(component(nodeAt(season, name), 'cc.Label')._fontSize >= 24);
    const headline = nodeAt(season, 'Headline'), headlineBox = bounds(headline);
    assert(headlineBox.top - headlineBox.bottom >= 2 * component(headline, 'cc.Label')._lineHeight,
        '当季排名与最近赛季结转各需一整行');
    const summaryOrder = ['Title', 'Headline', 'Balance', 'Status', 'Tabs'].map(name => bounds(nodeAt(season, name)));
    for (let index = 1; index < summaryOrder.length; index++) {
        assert(summaryOrder[index - 1].bottom > summaryOrder[index].top, '摘要、状态和页签之间应保留间距');
    }
    for (const name of ['Status', 'Detail']) {
        const text = nodeAt(season, name), label = component(text, 'cc.Label');
        assert(label._enableWrapText && bounds(text).top - bounds(text).bottom >= label._lineHeight * 2);
    }
    const tabRoot = nodeAt(season, 'Tabs'), rank = nodeAt(tabRoot, 'Rank'), rewards = nodeAt(tabRoot, 'Rewards');
    assert(!overlap(bounds(rank), bounds(rewards)));
    const list = nodeAt(season, 'List'), view = nodeAt(list, 'View'), content = nodeAt(view, 'Content');
    const row = nodeAt(content, 'ItemTemplate'), savedList = component(list, '39a82802rlT3Z9OWj3G8DOj');
    component(view, 'cc.Mask');
    assert.equal(scene[component(list, 'cc.ScrollView')._content.__id__], content);
    assert.equal(scene[savedList.viewport.__id__], view);
    assert.equal(scene[savedList.templateNode.__id__], row);
    assert.equal(row._active, false);
    assert.equal(component(row, 'cc.UITransform')._contentSize.height, savedList.cellHeight);
    assert(component(nodeAt(row, 'Text'), 'cc.Label')._fontSize >= 24);
    assert(component(nodeAt(row, 'Text'), 'cc.Label')._enableWrapText);
    assert(3 * savedList.cellHeight + 2 * savedList.spacingY <= component(view, 'cc.UITransform')._contentSize.height);
    for (const button of [ledger, rank, rewards, row, ...actions.map(name => nodeAt(season, name))]) {
        const size = component(button, 'cc.UITransform')._contentSize;
        assert(Math.min(size.width, size.height) * 390 / 720 >= 44, button._name + ' 的窄屏点击范围不足');
        assert.deepEqual(component(button, 'cc.Button').clickEvents, []);
    }
});
