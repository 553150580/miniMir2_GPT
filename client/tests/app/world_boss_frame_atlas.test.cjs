const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ts = require('../../../tools/client/creator_paths.cjs').typescript;

const file = path.resolve(__dirname, '../../assets/scripts/app/WorldBossFrameAtlas.ts');
const { outputText } = ts.transpileModule(fs.readFileSync(file, 'utf8'), {
    compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS },
});

function fixture(catalog, loadTexture) {
    const releases = [];
    const requests = [];
    const index = { json: catalog, references: 0, addRef() { this.references++; }, decRef() { this.references--; } };
    const texture = {
        width: 20, height: 20, references: 0,
        addRef() { this.references++; },
        decRef() { this.references--; releases.push('纹理'); },
    };
    class JsonAsset {}
    class Texture2D {}
    class Rect {
        constructor(x, y, width, height) { Object.assign(this, { x, y, width, height }); }
    }
    class Size {
        constructor(width, height) { Object.assign(this, { width, height }); }
    }
    class SpriteFrame {
        reset(options) { Object.assign(this, options); }
        destroy() { releases.push('精灵帧'); }
    }
    const resources = {
        getInfoWithPath: () => ({}),
        load(path, type, done) {
            requests.push(path);
            if (type === JsonAsset) done(null, index);
            else if (loadTexture) loadTexture(path, error => done(error, texture));
            else done(null, texture);
        },
    };
    const engine = { JsonAsset, Texture2D, Rect, Size, SpriteFrame, resources };
    const module = { exports: {} };
    new Function('require', 'module', 'exports', outputText)(name => {
        assert.equal(name, 'cc');
        return engine;
    }, module, module.exports);
    return { atlas: new module.exports.WorldBossFrameAtlas(), requests, releases, index, texture };
}

const page = name => ({ name, width: 20, height: 20 });
const originals = {
    'Magic:200': { width: 3, height: 4, png_sha256: '原帧甲' },
    'Magic:201': { width: 3, height: 4, png_sha256: '原帧乙' },
};
const keys = Object.keys(originals);
const rectangles = {
    'Magic:200': [0, 1, 2, 3, 4, '原帧甲'],
    'Magic:201': [0, 6, 2, 3, 4, '原帧乙'],
};

test('同页原帧共享一个纹理引用，各自保留原尺寸和裁切位置', async () => {
    const { atlas, requests, releases, index, texture } = fixture({ schema_version: 1, pages: [page('page-00')], frames: rectangles });
    const target = new Map();
    const progress = [];
    await atlas.populate(keys, originals, target, count => progress.push(count));
    assert.equal(requests.length, 2);
    assert.deepEqual(progress, [2]);
    assert.deepEqual(Array.from(target.values(), frame => [frame.rect.x, frame.originalSize.width, frame.packable]), [[1, 3, false], [6, 3, false]]);
    assert.equal(target.get(keys[0]).texture, target.get(keys[1]).texture);
    assert.equal(index.references, 0);
    assert.equal(texture.references, 1);
    atlas.dispose();
    atlas.dispose();
    assert.equal(texture.references, 0);
    assert.deepEqual(releases, ['精灵帧', '精灵帧', '纹理']);
});

test('过期的原帧索引在请求纹理前失败并释放索引引用', async () => {
    const frames = { ...rectangles, 'Magic:200': [0, 1, 2, 3, 4, '旧原帧'] };
    const { atlas, requests, index } = fixture({ schema_version: 1, pages: [page('page-00')], frames });
    await assert.rejects(atlas.populate(keys, originals, new Map(), () => {}), /来源不符/);
    assert.equal(requests.length, 1);
    assert.equal(index.references, 0);
});

test('其中一页失败仍等并发页结束，离场可清理后到的帧和纹理', async () => {
    const frames = { ...rectangles, 'Magic:201': [1, 6, 2, 3, 4, '原帧乙'] };
    const load = new Map();
    const { atlas, index, texture, releases } = fixture({
        schema_version: 1, pages: [page('page-00'), page('page-01')], frames,
    }, (path, finish) => load.set(path.split('/').at(-2), finish));
    const target = new Map();
    let settled = false;
    const pending = atlas.populate(keys, originals, target, () => {}).finally(() => { settled = true; });
    await new Promise(resolve => setImmediate(resolve));
    load.get('page-00')(new Error('纹理不可用'));
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(settled, false);
    load.get('page-01')(null);
    await assert.rejects(pending, /纹理不可用/);
    assert.equal(target.size, 1);
    atlas.dispose();
    assert.equal(index.references, 0);
    assert.equal(texture.references, 0);
    assert.deepEqual(releases, ['精灵帧', '纹理']);
});
