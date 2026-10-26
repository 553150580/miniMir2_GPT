const { test } = require('node:test');
const assert = require('node:assert/strict');
const path = require('node:path');

const output = path.resolve(__dirname, '../../../.work/scene-tests');
const { CreatorSceneBackend } = require(path.join(output, 'CreatorSceneBackend.js'));
const { SceneLoader } = require(path.join(output, 'SceneLoader.js'));
const { SceneManager } = require(path.join(output, 'SceneManager.js'));
const tick = () => new Promise(resolve => setImmediate(resolve));

// 手动交付异步 SDK 回调，验证边界；不模拟 Creator 节点或宣称真实引擎运行。
function setup() {
    const director = {
        preloads: [], loads: [], acceptLoad: true, preloadError: null, loadError: null,
        preloadScene(name, progress, complete) {
            if (this.preloadError) throw this.preloadError;
            this.preloads.push({ name, progress, complete, argumentCount: arguments.length });
        },
        loadScene(name, complete) {
            if (this.loadError) throw this.loadError;
            this.loads.push({ name, complete, argumentCount: arguments.length });
            return this.acceptLoad;
        },
    };
    const loader = new SceneLoader(new CreatorSceneBackend(director));
    const manager = new SceneManager(loader, ['Login', 'Town', 'Dungeon'], 'Login');
    return { director, loader, manager };
}

test('异步预加载与启动分阶段，只有启动成功才提交当前场景', async () => {
    const { director, manager } = setup();
    const states = [];
    manager.subscribe(state => states.push(state));
    const switched = manager.switchTo('Town');
    assert.equal(director.preloads[0].argumentCount, 3);
    assert.equal(manager.state.phase, 'preloading');
    assert.equal(manager.state.currentScene, 'Login');
    director.preloads[0].complete(null);
    await tick();
    assert.equal(director.loads[0].argumentCount, 2);
    assert.equal(director.loads[0].name, 'Town');
    assert.equal(manager.state.phase, 'loading');
    assert.equal(manager.state.currentScene, 'Login');
    director.loads[0].complete(null);
    assert.equal(await switched, 'Town');
    assert.equal(manager.state.currentScene, 'Town');
    assert.equal(manager.state.phase, 'idle');
    assert.deepEqual(states.map(state => state.phase), ['preloading', 'loading', 'idle']);
});

test('同目标复用同一 Promise，不同目标忙拒绝，白名单限制输入', async () => {
    const { director, manager } = setup();
    const first = manager.switchTo('Town');
    assert.equal(manager.switchTo('Town'), first);
    await assert.rejects(manager.switchTo('Dungeon'), { code: 'BUSY' });
    await assert.rejects(manager.switchTo('unknown'), { code: 'UNKNOWN_SCENE' });
    assert.throws(() => manager.registerScene(' Town '), { code: 'UNKNOWN_SCENE' });
    director.preloads[0].complete(null);
    await tick();
    assert.equal(manager.switchTo('Town'), first);
    director.loads[0].complete(null);
    await first;
    assert.equal(await manager.switchTo('Town'), 'Town');
    assert.equal(director.preloads.length, 1);
    assert.equal(director.loads.length, 1);
});

test('预加载失败保留旧场景并支持重试', async () => {
    const { director, manager } = setup();
    const failure = assert.rejects(manager.switchTo('Town'), { code: 'PRELOAD_FAILED' });
    director.preloads[0].complete(new Error('资源缺失'));
    await failure;
    assert.equal(manager.state.currentScene, 'Login');
    assert.equal(manager.state.phase, 'idle');
    assert.equal(manager.state.lastError.code, 'PRELOAD_FAILED');
    assert.equal(director.loads.length, 0);
    const retry = manager.switchTo('Town');
    director.preloads[1].complete(null);
    await tick();
    director.loads[0].complete(null);
    assert.equal(await retry, 'Town');
    assert.equal(manager.state.lastError, null);
});

test('启动回调失败和拒绝保留旧场景，同步中断将当前场景标为未知', async t => {
    for (const mode of ['callback', 'rejected', 'thrown']) {
        await t.test(mode, async () => {
            const { director, manager } = setup();
            if (mode === 'rejected') director.acceptLoad = false;
            if (mode === 'thrown') director.loadError = new Error('引擎异常');
            const errorCode = mode === 'rejected' ? 'LOAD_REJECTED'
                : mode === 'thrown' ? 'LOAD_INTERRUPTED' : 'LOAD_FAILED';
            const failure = assert.rejects(manager.switchTo('Town'), {
                code: errorCode,
            });
            director.preloads[0].complete(null);
            await tick();
            if (mode === 'callback') director.loads[0].complete(new Error('启动失败'));
            await failure;
            assert.equal(manager.state.currentScene, mode === 'thrown' ? null : 'Login');
            assert.equal(manager.state.targetScene, null);
            assert.equal(manager.state.phase, 'idle');
            assert.equal(manager.state.lastError.code, errorCode);
            director.acceptLoad = true;
            director.loadError = null;
            const retry = manager.switchTo('Dungeon');
            director.preloads[1].complete(null);
            await tick();
            director.loads.at(-1).complete(null);
            assert.equal(await retry, 'Dungeon');
        });
    }
});

test('同步成功回调之后的异常不推翻已知启动结果', async () => {
    const { director, manager } = setup();
    director.loadScene = function (_name, complete) {
        complete(null);
        throw new Error('完成通知之后的异常');
    };
    const switched = manager.switchTo('Town');
    director.preloads[0].complete(null);
    assert.equal(await switched, 'Town');
    assert.equal(manager.state.currentScene, 'Town');
    assert.equal(manager.state.lastError, null);
});

test('预加载同步异常被转换为可恢复的失败', async () => {
    const { director, manager } = setup();
    director.preloadError = new Error('同步异常');
    await assert.rejects(manager.switchTo('Town'), { code: 'PRELOAD_FAILED' });
    assert.equal(manager.state.phase, 'idle');
    assert.equal(manager.state.currentScene, 'Login');
});

test('取消预加载后立即切换，过期进度和完成回调不能污染新任务', async () => {
    const { director, manager } = setup();
    const cancelled = assert.rejects(manager.switchTo('Town'), { code: 'CANCELLED' });
    const old = director.preloads[0];
    assert.equal(manager.cancelPreload(), true);
    await cancelled;
    const next = manager.switchTo('Dungeon');
    old.progress(1, 1, '过期资源');
    old.complete(null);
    old.complete(new Error('过期失败'));
    await tick();
    assert.equal(manager.state.targetScene, 'Dungeon');
    assert.equal(manager.state.progress, null);
    assert.equal(director.loads.length, 0);
    director.preloads[1].complete(null);
    await tick();
    director.loads[0].complete(null);
    assert.equal(await next, 'Dungeon');
});

test('预加载完成但微任务未启动加载时仍可逻辑取消', async () => {
    const { director, manager } = setup();
    const cancelled = assert.rejects(manager.switchTo('Town'), { code: 'CANCELLED' });
    director.preloads[0].complete(null);
    assert.equal(manager.cancelPreload(), true);
    await cancelled;
    await tick();
    assert.equal(director.loads.length, 0);
    assert.equal(manager.state.currentScene, 'Login');
});

test('引擎加载开始后不允许取消，dispose 等待真实结果且不通知旧监听', async () => {
    const { director, manager } = setup();
    let notifications = 0;
    manager.subscribe(() => { ++notifications; });
    const switched = manager.switchTo('Town');
    director.preloads[0].complete(null);
    await tick();
    assert.equal(manager.cancelPreload(), false);
    const disposal = manager.dispose();
    assert.equal(manager.dispose(), disposal);
    const before = notifications;
    assert.equal(manager.state.phase, 'loading');
    assert.equal(manager.state.disposed, true);
    await assert.rejects(manager.switchTo('Dungeon'), { code: 'DISPOSED' });
    let disposed = false;
    disposal.then(() => { disposed = true; });
    await tick();
    assert.equal(disposed, false);
    director.loads[0].complete(null);
    await switched;
    await disposal;
    assert.equal(manager.state.currentScene, 'Town');
    assert.equal(manager.state.phase, 'disposed');
    assert.equal(notifications, before);
});

test('dispose 只取消预加载逻辑，不接受过期回调或新增监听', async () => {
    const { director, manager } = setup();
    const cancelled = assert.rejects(manager.switchTo('Town'), { code: 'CANCELLED' });
    await manager.dispose();
    await cancelled;
    director.preloads[0].complete(null);
    await tick();
    assert.equal(director.loads.length, 0);
    assert.equal(manager.state.phase, 'disposed');
    assert.throws(() => manager.subscribe(() => {}), { code: 'DISPOSED' });
});

test('进度只来自 SDK，未知总数不除零，加载期间不伪造百分比', async () => {
    const { director, manager } = setup();
    const switched = manager.switchTo('Town');
    const request = director.preloads[0];
    for (const [completed, total, ratio] of [[0, 0, null], [3, 0, null], [1, 4, 0.25],
        [3, 4, 0.75], [1, 4, 0.25], [5, 4, 1], [-1, 4, 0],
        [Infinity, 4, null], [NaN, 4, null], [1, NaN, null], [1, -1, null]]) {
        request.progress(completed, total, '资源');
        assert.equal(manager.state.progress.ratio, ratio);
        assert.equal(manager.state.progress.completed, Number.isFinite(completed) ? Math.max(0, completed) : 0);
        assert.equal(manager.state.progress.total, Number.isFinite(total) ? Math.max(0, total) : 0);
    }
    request.complete(null);
    await tick();
    request.progress(4, 4, '迟到进度');
    assert.equal(manager.state.progress, null);
    assert.equal(manager.state.phase, 'loading');
    director.loads[0].complete(null);
    await switched;
});

test('重复完成回调与观察者异常不重复加载或破坏状态', async () => {
    const { director, manager } = setup();
    const unsubscribe = manager.subscribe(() => { throw new Error('界面错误'); });
    const switched = manager.switchTo('Town');
    director.preloads[0].progress(1, 2, null);
    director.preloads[0].complete(null);
    director.preloads[0].complete(new Error('重复完成'));
    await tick();
    assert.equal(director.loads.length, 1);
    director.loads[0].complete(null);
    director.loads[0].complete(new Error('重复启动回调'));
    await switched;
    unsubscribe();
    assert.equal(manager.state.currentScene, 'Town');
    assert.equal(manager.state.lastError, null);
});

test('同步预加载进度触发取消时仍阻断后续加载', async () => {
    const { director, manager } = setup();
    director.preloadScene = function (_name, progress, complete) {
        progress(1, 2, null);
        complete(null);
    };
    manager.subscribe(state => {
        if (state.progress) manager.cancelPreload();
    });
    await assert.rejects(manager.switchTo('Town'), { code: 'CANCELLED' });
    await tick();
    assert.equal(director.loads.length, 0);
    assert.equal(manager.state.currentScene, 'Login');
});
