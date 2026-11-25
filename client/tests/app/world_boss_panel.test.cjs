const assert = require('node:assert/strict');
const test = require('node:test');
const { Long, load } = require('./helpers/session_harness.cjs');

const epoch = Long.fromString('9007199254740993', true);
const startedAt = 1780000000000;
const dayMs = 24 * 60 * 60 * 1000;
const timestamp = offset => Long.fromNumber(startedAt + offset);
const freshState = (state, opens, expires, sceneEpoch = epoch) => ({
    sceneEpoch, occurrenceId: 'first', templateId: 2, state,
    opensAtUnixMs: timestamp(opens), expiresAtUnixMs: timestamp(expires),
    hp: Long.fromInt(300), hpMax: Long.fromInt(300), ranks: [],
});

function fixture(t) {
    const rows = new Map(), appEvents = new Map(), requests = [];
    const part = name => {
        if (!rows.has(name)) rows.set(name, { string: '', interactable: false });
        return { getComponent: () => rows.get(name) };
    };
    const panel = { activeInHierarchy: true, getChildByPath: part, getChildByName: part };
    const game = {
        on(event, callback, target) { appEvents.set(event, callback.bind(target)); },
        off(event) { appEvents.delete(event); },
    };
    const { GameBoss } = load('client/assets/scripts/app/GameBoss.ts', id => id === 'cc' ? {
        game, Game: { EVENT_HIDE: 'hide', EVENT_SHOW: 'show' },
    } : {});
    let now = 0, nextState = freshState(1, 3000, 12000), listener, answer, prepare, closed = 0;
    t.mock.method(performance, 'now', () => now);
    t.mock.timers.enable({ apis: ['setTimeout'] });
    const current = { sceneEpoch: epoch, sceneKind: 1, ready: true, level: 10,
        worldBoss: { display_name: '林地首领', minimum_level: 1, map_id: '81', template_id: 2 } };
    const connection = {
        generation: 1,
        subscribe(callback) { listener = callback; return () => { listener = undefined; }; },
        request(id, payload) {
            requests.push({ id, payload, time: now });
            return answer ? answer() : Promise.resolve(id === 20017 ? nextState : {});
        },
    };
    const view = new GameBoss(panel, connection, () => current, () => { closed++; }, (target, signal) => prepare(target, signal), {
        claim() { assert.fail('世界首领页不领取个人待领'); }, itemName() {}, report: error => { throw error; },
    });
    const tick = async milliseconds => {
        now += milliseconds;
        t.mock.timers.tick(milliseconds);
        await new Promise(resolve => setImmediate(resolve));
    };
    t.after(() => view.dispose());
    return {
        view, panel, current, connection, rows, requests, tick, appEvents,
        change(state) { nextState = state; },
        respond(custom) { answer = custom; },
        prepare(custom) { prepare = custom; },
        get closed() { return closed; },
        push(id, value, seq = 0) { listener?.({ id, seq, value, error: 0 }); },
    };
}

test('打开世界首领面板后由服务端心跳时钟跨开场和到期，活动中可发现提前击杀', async t => {
    const f = fixture(t);
    f.push(2, { serverUnixMs: timestamp(0) });
    await f.view.show(4);
    assert.equal(f.rows.get('Status').string, '等待开放');
    assert.equal(f.rows.get('Action').interactable, false);
    await f.tick(2999);
    assert.equal(f.requests.length, 1);

    f.change(freshState(3, 3000, 12000));
    await f.tick(1);
    assert.equal(f.rows.get('Status').string, '开放中');
    assert.equal(f.rows.get('Action').interactable, true);
    f.change(freshState(4, 3000, 12000));
    await f.tick(5000);
    assert.equal(f.rows.get('Status').string, '已击杀');
    assert.equal(f.rows.get('Action').interactable, false);

    f.change({ ...freshState(1, dayMs + 3000, dayMs + 12000), occurrenceId: 'next' });
    await f.tick(4000);
    assert.equal(f.rows.get('Status').string, '等待开放');
    assert.deepEqual(f.requests.map(({ id, time }) => [id, time]), [[20017, 0], [20017, 3000], [20017, 8000], [20017, 12000]]);
});

test('心跳前有界查询，切换页签、关闭、后台和销毁后取消自动查询', async t => {
    const f = fixture(t);
    await f.view.show(4);
    await f.tick(1999);
    assert.equal(f.requests.length, 1);
    f.change(freshState(3, 2000, 12000));
    await f.tick(1);
    assert.equal(f.rows.get('Action').interactable, true);

    f.appEvents.get('hide')();
    await f.tick(10000);
    assert.equal(f.requests.length, 2);
    f.change(freshState(1, dayMs, dayMs + 10000));
    f.appEvents.get('show')();
    await f.tick(0);
    assert.equal(f.requests.length, 3);
    assert.equal(f.rows.get('Action').interactable, false);

    f.panel.activeInHierarchy = false;
    f.view.visibilityChanged();
    await f.tick(20000);
    assert.equal(f.requests.length, 3);
    f.panel.activeInHierarchy = true;
    await f.view.show(3);
    await f.tick(20000);
    assert.deepEqual(f.requests.map(({ id }) => id), [20017, 20017, 20017, 20015]);
    await f.view.show(4);
    f.view.dispose();
    await f.tick(20000);
    assert.equal(f.requests.length, 5);
    assert.equal(f.appEvents.size, 0);
});

test('刷新在途时合并查询，旧场景和旧连接均不能覆盖新场次', async t => {
    const f = fixture(t), resolve = [];
    f.respond(() => new Promise(accept => resolve.push(accept)));
    const first = f.view.show(4), duplicate = f.view.reload();
    await f.tick(5000);
    assert.equal(f.requests.length, 1);

    const next = Long.fromString('9007199254740994', true);
    f.current.sceneEpoch = next;
    f.connection.generation++;
    f.view.render();
    await f.tick(0);
    assert.equal(f.requests.length, 2);
    resolve[1](freshState(1, 20000, 30000, next));
    await f.tick(0);
    assert.equal(f.rows.get('Status').string, '等待开放');
    resolve[0](freshState(3, 0, 12000));
    await Promise.all([first, duplicate]);
    f.push(20020, freshState(3, 0, 12000));
    assert.equal(f.rows.get('Status').string, '等待开放');
    assert.equal(f.rows.get('Action').interactable, false);
});

test('触屏入场先准备目录指定地图再重新查询，确认原场才派送席位请求', async t => {
    const f = fixture(t);
    f.change(freshState(3, 0, 15000));
    await f.view.show(4);
    const template = { mapId: '81', occurrenceId: 'first', templateId: 2 };
    let finish, dispatched = 0, released = 0;
    f.prepare((target, signal) => {
        assert.deepEqual(target, template);
        assert.equal(signal.aborted, false);
        return new Promise(resolve => { finish = resolve; });
    });
    const action = f.view.action();
    assert.deepEqual(f.requests.map(request => request.id), [20017]);
    finish({
        dispatch() {
            assert.deepEqual(f.requests.map(request => request.id), [20017, 20017]);
            dispatched++;
        },
        release() { released++; },
    });
    await action;
    assert.deepEqual(f.requests.map(request => request.id), [20017, 20017, 20018]);
    assert.deepEqual(f.requests.at(-1).payload, { sceneEpoch: epoch, templateId: 2, occurrenceId: 'first' });
    assert.equal(dispatched, 1);
    assert.equal(released, 0);
    assert.equal(f.closed, 1);
});

for (const cause of ['关闭面板', '开场身份变化']) {
    test(`${cause}时原世界场准备仅释放，不提交入场`, async t => {
        const f = fixture(t);
        f.change(freshState(3, 0, 15000));
        await f.view.show(4);
        let releaseCount = 0;
        f.prepare(async (_target, signal) => {
            if (cause === '关闭面板') {
                f.panel.activeInHierarchy = false;
                f.view.visibilityChanged();
                assert.equal(signal.aborted, true);
            } else f.change({ ...freshState(3, 30000, 45000), occurrenceId: 'next' });
            return {
                dispatch() { assert.fail('不应派送已取消或过期的准备'); },
                release() { releaseCount++; },
            };
        });
        if (cause === '关闭面板') await f.view.action();
        else await assert.rejects(f.view.action(), /场次已变化/);
        assert.equal(f.requests.some(request => request.id === 20018), false);
        assert.equal(f.closed, 0);
        assert.equal(releaseCount, 1);
    });
}
