const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ts = require('../../../tools/client/creator_paths.cjs').typescript;
const { UITransform, Vec3 } = require('../support/world_runtime.cjs');

const source = path.resolve(__dirname, '../../assets/scripts/app/TownTouchHit.ts');
const code = ts.transpileModule(fs.readFileSync(source, 'utf8'), {
    compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS },
}).outputText;

test('城镇身体根据当前原图、裁边和合图矩形过滤透明地面，保留引擎外框及遮罩检查', () => {
    class Texture2D { constructor(data) { this.image = { data }; } }
    const sampled = [];
    const document = { createElement() { return { getContext() { return {
        clearRect() {}, drawImage(source, x, y) { sampled.push({ source, x, y }); },
        getImageData() { const { source, x, y } = sampled.at(-1); return { data: [0, 0, 0, source.opaque(x, y) ? 255 : 0] }; },
    }; } }; } };
    const module = { exports: {} };
    const view = { getViewportRect: () => ({ x: 100, y: 50 }), getScaleX: () => 2, getScaleY: () => 4 };
    new Function('require', 'module', 'exports', 'document', code)(
        () => ({ Texture2D, UITransform, Vec3, view }), module, module.exports, document);
    const transform = new UITransform();
    transform.setAnchorPoint(0, 1);
    transform.setContentSize(8, 8);
    transform.hitTest = (_point, windowId) => windowId === 3;
    const original = { opaque: (x, y) => x === 14 && y === 22 };
    const atlas = { opaque: (x, y) => x === 104 && y === 203 };
    const body = { node: { getComponent: () => transform }, spriteFrame: {
        originalSize: { width: 8, height: 8 }, offset: { x: 0, y: 1 }, rect: { x: 100, y: 200, width: 6, height: 4 },
        texture: new Texture2D(atlas), original: { _x: 10, _y: 20, _texture: new Texture2D(original) }, rotated: false,
    } };
    const screen = (x, y) => ({ x: 100 + 2 * x, y: 50 - 4 * y });
    new module.exports.TownActorTouch().bind(body);
    assert.equal(transform.hitTest(screen(5.1, 3.1), 0), false);
    assert.equal(transform.hitTest(screen(0.2, 0.5), 3), false);
    assert.equal(transform.hitTest(screen(8, 8), 3), false);
    assert.equal(sampled.length, 0);
    assert.equal(transform.hitTest(screen(5.1, 3.1), 3), true);
    assert.deepEqual(sampled.at(-1), { source: original, x: 14, y: 22 });
    assert.equal(transform.hitTest(screen(4.7, 3.1), 3), false);
    body.spriteFrame = { ...body.spriteFrame, original: null };
    assert.equal(transform.hitTest(screen(5.1, 3.1), 3), false);
    assert.equal(transform.hitTest(screen(5.1, 4.1), 3), true);
    assert.deepEqual(sampled.at(-1), { source: atlas, x: 104, y: 203 });
    body.spriteFrame = null;
    assert.equal(transform.hitTest(screen(5.1, 4.1), 3), false);
});
