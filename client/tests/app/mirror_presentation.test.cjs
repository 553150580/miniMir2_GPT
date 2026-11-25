const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ts = require('../../../tools/client/creator_paths.cjs').typescript;
const { cc } = require('../support/world_runtime.cjs');

const source = name => path.resolve(__dirname, '../../assets/scripts/app', `${name}.ts`);
function load(name, dependencies) {
    const compilerOptions = { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS };
    const { outputText } = ts.transpileModule(fs.readFileSync(source(name), 'utf8'), { compilerOptions });
    const module = { exports: {} };
    new Function('require', 'module', 'exports', outputText)(id => dependencies[id], module, module.exports);
    return module.exports;
}

function harness() {
    const requests = [], views = [];
    const WorldAssets = { prepare(job, progress, mapId, travelling) {
        return new Promise((resolve, reject) => requests.push({ job, progress, mapId, travelling, resolve, reject }));
    } };
    class GameWorldView {
        constructor(viewport, world, entities, name, job, scene, playerHandle) {
            Object.assign(this, { viewport, world, entities, name, job, scene, playerHandle, map: { width: 13, height: 13 }, calls: [] });
            views.push(this);
        }
        get isMapReady() { return !!this.assets; }
        bindPresentation(identity, current) { Object.assign(this, { identity, current }); }
        prepare(assets) { this.assets = assets; }
        dispose() { this.assets?.dispose(); this.calls.push('dispose'); }
        apply(baseline) { this.baseline = baseline; this.calls.push('state'); }
        acceptDisplacement() { this.calls.push('movement'); }
        acceptCast() { this.calls.push('cast'); }
        acceptAttack() { this.calls.push('attack'); }
        acceptImpact() { this.calls.push('impact'); }
        acceptCastStep() { this.calls.push('step'); }
        update() { this.calls.push('tick'); }
    }
    const { MirrorArenaPresentation } = load('MirrorArenaPresentation', { cc, './WorldAssets': { WorldAssets },
        './GameWorldView': { GameWorldView }, './MapTravelModel': load('MapTravelModel', {}) });
    const safe = new cc.Node('Safe'), ordinary = new cc.Node('OrdinaryWorld'), overlay = new cc.Node('MirrorArenaBattle');
    ordinary.parent = safe; overlay.parent = safe;
    const viewport = new cc.Node('MapViewport'), world = new cc.Node('World');
    viewport.parent = overlay; world.parent = viewport; new cc.Node('Entities').parent = world;
    const renderer = new MirrorArenaPresentation(overlay);
    function complete(index, changes = {}) {
        const mapId = requests[index].mapId;
        const value = { sceneKind: 6, mapId, gameplay: { rule_version: 30, mirror_arena: { enabled: true, map_id: mapId } },
            disposal: 0, dispose() { this.disposal++; }, ...changes };
        requests[index].resolve(value);
        return value;
    }
    return { renderer, requests, views, complete, ordinary, world };
}

const map = '9007199254740995', scene = '9007199254740997';
const identity = { connection: 1, binding: '9', role: '原角色', scene, baseline: scene, map, travelId: '挑战' };

test('镜像进场复用大厅真实预载，仅消费该副流，按位移、快照、特效顺序渲染并独立释放', async () => {
    const { renderer, requests, views, complete, ordinary, world } = harness();
    const progress = [], active = () => true;
    const warming = renderer.preload(map, '30', 2, active, (...value) => progress.push(value));
    requests[0].progress(1, 3);
    requests[0].progress(3, 3);
    const assets = complete(0);
    assert.equal(await warming, true);
    assert.deepEqual(progress, [[1, 3], [3, 3]]);
    assert.equal(await renderer.prepare(identity, '镜像身份', '30', '进攻方', 2, active), true);
    assert.equal(requests.length, 1);
    assert.equal(requests[0].mapId, map);
    assert.equal(views[0].scene, scene);
    assert.equal(views[0].playerHandle, 1);
    assert.equal(world.active, false);

    const frame = { battleKey: '镜像身份', sequence: '9007199254740999', entities: [{ position: { x: 12, y: 12 } }],
        fireWallCells: [{ x: 5, y: 4 }, { x: 13, y: 5 }],
        observations: [30006, 30013, 30007, 30008, 30012].map(messageId => ({ messageId, value: {} })) };
    assert(!renderer.apply({ ...frame, battleKey: '旧身份' }));
    assert(!renderer.apply({ ...frame, entities: [{ position: { x: 13, y: 3 } }] }));
    assert(renderer.apply(frame));
    assert(!renderer.apply(frame));
    assert(!renderer.apply({ ...frame, sequence: '9007199254740998' }));
    renderer.update(1);
    assert.deepEqual(views[0].calls, ['movement', 'state', 'cast', 'attack', 'impact', 'step', 'tick']);
    assert.deepEqual(views[0].baseline.fireWallCells, [{ x: 5, y: 4 }]);
    assert(world.active);
    renderer.reset();
    assert(!world.active);
    assert(!views[0].current());
    assert.equal(assets.disposal, 1);
    assert(ordinary.active);
});

test('镜像换票取消旧加载，迟到资源只释放，新场保留自己的资源，地图规则错误不附着', async () => {
    const { renderer, requests, views, complete } = harness();
    const active = () => true;
    const cancelled = renderer.prepare(identity, '旧票', '30', '进攻方', 2, active);
    renderer.reset();
    const replacement = renderer.prepare({ ...identity, connection: 2 }, '新票', '30', '进攻方', 2, active);
    const late = complete(0);
    assert.equal(await cancelled, false);
    assert.equal(late.disposal, 1);
    const fresh = complete(1);
    assert.equal(await replacement, true);
    assert.equal(views.length, 1);
    assert.equal(views[0].identity.connection, 2);
    const invalid = renderer.preload('33', '30', 2, active);
    const wrong = complete(2, { sceneKind: 1 });
    await assert.rejects(invalid, /地图或资源规则与当前场次不符/);
    assert.equal(wrong.disposal, 1);
    assert.equal(fresh.disposal, 0);
    renderer.dispose();
    assert.equal(fresh.disposal, 1);
    assert.equal(await renderer.preload(map, '30', 2, active), false);
    assert.equal(requests.length, 3);
});
