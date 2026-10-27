const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = path.resolve(__dirname, '../../..');
// Sprite.Type 与 ProgressBar.Mode 是两套枚举，FILLED 的序列化值不同。
const SPRITE_SIMPLE = 0, SPRITE_FILLED = 3, FILL_VERTICAL = 1, PROGRESS_FILLED = 2;
const creator = process.env.MINIMIR_CREATOR_APP ?? '/Applications/Cocos/Creator/3.8.8/CocosCreator.app';
const engine = path.join(creator, 'Contents/Resources/resources/3d/engine/cocos');
const scene = JSON.parse(fs.readFileSync(path.join(root, 'client/assets/scenes/Game.scene'), 'utf8'));
const nodes = new Map();
function indexNode(index, prefix = '') {
    const node = scene[index];
    const name = prefix ? `${prefix}/${node._name}` : node._name;
    assert(!nodes.has(name), `节点路径重复：${name}`);
    nodes.set(name, index);
    for (const child of node._children ?? []) indexNode(child.__id__, name);
}
indexNode(scene[0].scene.__id__);
const safePath = Array.from(nodes.keys()).find(name => name.endsWith('/Canvas/Safe'));
function node(relative = '') {
    const key = relative ? `${safePath}/${relative}` : safePath;
    assert(nodes.has(key), `缺少已保存的节点：${key}`);
    return scene[nodes.get(key)];
}
const components = node => (node._components ?? []).map(reference => scene[reference.__id__]);
function component(node, type) {
    const found = components(node).find(value => value.__type__ === type);
    assert(found, `${node._name}缺少${type}`);
    return found;
}
const transform = node => component(node, 'cc.UITransform');
const children = node => (node._children ?? []).map(reference => scene[reference.__id__]);
function descendants(node) { return children(node).flatMap(child => [child, ...descendants(child)]); }
function compressedUuid(uuid) {
    const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';
    const hex = uuid.replace(/-/g, '');
    let result = hex.slice(0, 5);
    for (let index = 5; index < hex.length; index += 3) {
        const group = parseInt(hex.slice(index, index + 3), 16);
        result += alphabet[group >> 6] + alphabet[group & 63];
    }
    return result;
}
function scriptType(relative) {
    const meta = JSON.parse(fs.readFileSync(path.join(root, 'client/assets/scripts', `${relative}.ts.meta`), 'utf8'));
    return compressedUuid(meta.uuid);
}

test('布局契约中的填充枚举与锁定Creator引擎一致', () => {
    function enumValue(relative, name, member) {
        const source = fs.readFileSync(path.join(engine, relative), 'utf8');
        const body = source.match(new RegExp(`\\benum\\s+${name}\\s*\\{([\\s\\S]*?)\\}`))?.[1];
        assert(body, `引擎缺少枚举${name}`);
        const value = body.match(new RegExp(`\\b${member}\\s*=\\s*(\\d+)\\s*,`))?.[1];
        assert(value !== undefined, `引擎缺少枚举值${name}.${member}`);
        return Number(value);
    }
    assert.equal(SPRITE_SIMPLE, enumValue('2d/components/sprite.ts', 'SpriteType', 'SIMPLE'));
    assert.equal(SPRITE_FILLED, enumValue('2d/components/sprite.ts', 'SpriteType', 'FILLED'));
    assert.equal(FILL_VERTICAL, enumValue('2d/components/sprite.ts', 'FillType', 'VERTICAL'));
    assert.equal(PROGRESS_FILLED, enumValue('ui/progress-bar.ts', 'Mode', 'FILLED'));
});

test('主场景所有内部引用有效，节点父子和组件归属完整且无孤立固定节点', () => {
    function inspect(value) {
        if (!value || typeof value !== 'object') return;
        if ('__id__' in value) assert(Number.isInteger(value.__id__) && scene[value.__id__], `悬空引用${value.__id__}`);
        for (const child of Object.values(value)) inspect(child);
    }
    scene.forEach(inspect);
    const reachable = new Set(nodes.values());
    for (let index = 0; index < scene.length; index++) {
        const value = scene[index];
        if (value.__type__ === 'cc.Node') {
            assert(reachable.has(index), `孤立节点${value._name}`);
            const unique = new Set();
            for (const child of value._children ?? []) {
                assert(!unique.has(child.__id__), `重复子节点${child.__id__}`); unique.add(child.__id__);
                assert.equal(scene[child.__id__]._parent.__id__, index, '父子引用应双向一致');
            }
            for (const reference of value._components ?? []) assert.equal(scene[reference.__id__].node.__id__, index);
            // 禁用的旧渲染组件仍参与Creator组件互斥检查，不能与新Sprite共存。
            const renderers = components(value).filter(item => ['cc.Label', 'cc.Sprite', 'cc.Graphics', 'cc.UIRenderable'].includes(item.__type__));
            assert(renderers.length <= 1, `${value._name}同时保存多个渲染组件：${renderers.map(item => item.__type__).join('、')}`);
        }
    }
});

test('HUD和消息控制器绑定Safe，头像、小地图、药品及状态均有固定编辑器节点', () => {
    component(node(), 'cc.SafeArea');
    component(node(), scriptType('app/GameHUD'));
    component(node(), scriptType('app/GameMessages'));
    for (const name of ['RoleName', 'Level', 'Job', 'Gold', 'Yuanbao', 'MapName', 'Position']) component(node(`TopHUD/${name}`), 'cc.Label');
    for (let job = 0; job < 3; job++) component(node(`TopHUD/Avatar/Portrait${job}`), 'cc.Sprite');
    component(node('TopHUD/MiniMap/Map'), 'cc.Sprite');
    assert.equal(node('TopHUD/MiniMap/Map/PlayerDot')._parent.__id__, nodes.get(`${safePath}/TopHUD/MiniMap/Map`));
    const map = transform(node('TopHUD/MiniMap/Map'))._contentSize;
    const world = transform(node('MapViewport/World'))._contentSize;
    assert(Math.abs(map.width / map.height - world.width / world.height) < 1e-8, '小地图必须保留真实地图贴图比例');
    for (const color of ['Red', 'Blue']) component(node(`BottomHUD/Skills/${color}Potion/Count`), 'cc.Label');
    component(node('MapViewport/Mission/Text'), 'cc.Label');
});

test('安全区上下HUD保持固定高度，新增屏幕高度只进入地图视口', () => {
    const top = node('TopHUD'), bottom = node('BottomHUD'), map = node('MapViewport');
    const topWidget = component(top, 'cc.Widget'), bottomWidget = component(bottom, 'cc.Widget'), mapWidget = component(map, 'cc.Widget');
    const TOP = 1, BOTTOM = 4, LEFT = 8, RIGHT = 32;
    assert.equal(topWidget._alignFlags & (TOP | BOTTOM), TOP);
    assert.equal(bottomWidget._alignFlags & (TOP | BOTTOM), BOTTOM);
    assert.equal(mapWidget._alignFlags & (TOP | BOTTOM | LEFT | RIGHT), TOP | BOTTOM | LEFT | RIGHT);
    assert.equal(topWidget._top, 0); assert.equal(bottomWidget._bottom, 0);
    assert.equal(mapWidget._top, transform(top)._contentSize.height);
    assert.equal(mapWidget._bottom, transform(bottom)._contentSize.height);
    for (const fixed of [top, bottom, ...descendants(top), ...descendants(bottom)]) {
        const widget = components(fixed).find(value => value.__type__ === 'cc.Widget');
        if (widget) assert.notEqual(widget._alignFlags & (TOP | BOTTOM), TOP | BOTTOM, `${fixed._name}不应吸收新增屏幕高度`);
    }
    const height = safeHeight => safeHeight - mapWidget._top - mapWidget._bottom;
    assert.equal(height(1600) - height(1280), 320);
    assert.equal(height(1280), transform(map)._contentSize.height);
    for (const value of scene.filter(value => value.__type__ === 'cc.Widget')) assert.equal(value._alignMode, 2, 'Widget应按窗口变化对齐');
    for (const fixed of [node('MapViewport/World'), ...descendants(node('MapViewport/World'))]) {
        assert(!components(fixed).some(value => value.__type__ === 'cc.Widget'), '地图与实体位置不能由Widget管理');
    }
});

test('字体至少24且禁止自动缩小，UI节点使用等比缩放', () => {
    for (const value of scene) {
        if (value.__type__ === 'cc.Label') {
            assert(value._fontSize >= 24, `字体低于24：${scene[value.node.__id__]._name}`);
            assert.notEqual(value._overflow, 2, 'SHRINK会让实际字号低于约定');
        }
        if (value.__type__ === 'cc.EditBox') {
            assert(value._fontSize >= 24); assert(value._placeholderFontSize >= 24);
        }
        if (value.__type__ === 'cc.Node') assert.equal(value._lscale.x, value._lscale.y, `${value._name}使用非等比缩放`);
    }
});

test('原始头像、图标及填充血球保留宽高比，容器九宫格单独适配', () => {
    const assets = JSON.parse(fs.readFileSync(path.join(root, 'client/assets/resources/ui180/sprite-frames.json'), 'utf8')).assets;
    const byUuid = new Map(Object.entries(assets).map(([name, value]) => [value.sprite_frame_uuid, { name, ...value }]));
    for (const sprite of scene.filter(value => value.__type__ === 'cc.Sprite' && [SPRITE_SIMPLE, SPRITE_FILLED].includes(value._type))) {
        const image = byUuid.get(sprite._spriteFrame?.__uuid__);
        if (!image) continue;
        const owner = scene[sprite.node.__id__], size = transform(owner)._contentSize;
        const relativeError = Math.abs((size.width / size.height) / (image.width / image.height) - 1);
        assert(relativeError < 1e-5, `${owner._name}/${image.name}图像被拉伸：${size.width}×${size.height}，源${image.width}×${image.height}`);
    }
});

test('自动开关和血蓝使用内置Toggle与FILLED进度组件，引用指向保存的图像', () => {
    const toggle = component(node('MapViewport/Actions/Auto'), 'cc.Toggle');
    const check = scene[toggle._checkMark.__id__];
    assert.equal(check.__type__, 'cc.Sprite');
    assert.equal(scene[check.node.__id__]._parent.__id__, nodes.get(`${safePath}/MapViewport/Actions/Auto`));
    for (const channel of ['HP', 'MP']) {
        const progress = component(node(`BottomHUD/Footer/${channel}`), 'cc.ProgressBar');
        const sprite = scene[progress._barSprite.__id__];
        assert.equal(progress._mode, PROGRESS_FILLED); assert.equal(progress._totalLength, 1);
        assert.equal(sprite.__type__, 'cc.Sprite'); assert.equal(sprite._type, SPRITE_FILLED); assert.equal(sprite._fillType, FILL_VERTICAL);
        assert.equal(scene[sprite.node.__id__]._parent.__id__, nodes.get(`${safePath}/BottomHUD/Footer/${channel}`));
    }
    for (const value of scene.filter(value => ['cc.Button', 'cc.Toggle'].includes(value.__type__))) {
        assert.deepEqual(value.clickEvents ?? [], []); assert.deepEqual(value.checkEvents ?? [], []);
    }
});

test('消息列表使用ScrollView与Mask，输入框保留默认文本树及组件引用', () => {
    const listNode = node('UIMgr/Panels/Messages/List');
    const scroll = component(listNode, 'cc.ScrollView'), list = component(listNode, scriptType('ui/List'));
    const content = scene[scroll._content.__id__], view = scene[content._parent.__id__], template = scene[list.templateNode.__id__];
    component(view, 'cc.Mask');
    assert.equal(list.scrollView.__id__, scene.indexOf(scroll)); assert.equal(list.viewport.__id__, scene.indexOf(view));
    assert.equal(template._parent.__id__, scene.indexOf(content)); assert.equal(template._active, false);
    component(template, 'cc.Button'); component(children(template).find(child => child._name === 'Text'), 'cc.Label');
    const search = node('UIMgr/Panels/Messages/Search'), edit = component(search, 'cc.EditBox');
    assert.deepEqual(children(search).map(child => child._name).sort(), ['PLACEHOLDER_LABEL', 'TEXT_LABEL']);
    for (const reference of [edit._textLabel, edit._placeholderLabel]) {
        const label = scene[reference.__id__]; assert.equal(label.__type__, 'cc.Label');
        const child = scene[label.node.__id__]; assert.equal(child._parent.__id__, scene.indexOf(search));
        assert(!components(child).some(value => value.__type__ === 'cc.Widget'));
    }
});

test('普通弹窗和覆盖弹窗位于全部HUD之上，各自保存独立遮罩与面板', () => {
    const order = children(node()).map(value => value._name);
    const ordinary = order.indexOf('UIMgr'), popup = order.indexOf('PopupView');
    assert(ordinary >= 0 && popup > ordinary);
    for (const name of ['MapViewport', 'TopHUD', 'BottomHUD', 'Status']) assert(order.indexOf(name) < ordinary, `${name}不能绘制在弹窗之上`);
    for (const name of ['UIMgr', 'PopupView']) {
        component(node(`${name}/Backdrop`), 'cc.BlockInputEvents');
        for (const panel of children(node(`${name}/Panels`))) {
            component(panel, 'cc.Sprite'); component(panel, 'cc.BlockInputEvents');
            assert(!descendants(panel).some(child => /^(?:Close|CloseButton)$/i.test(child._name)), '不设置独立关闭按钮');
        }
    }
});
