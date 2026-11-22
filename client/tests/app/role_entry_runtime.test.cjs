const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load, Long } = require('./helpers/session_harness.cjs');
const errors = require('../../../.work/ui-tests/app/GameErrors.js');
const ids = require('../../../.work/ui-tests/generated/protocol_ids.js');

class ProtocolResponseError extends Error {
    constructor(messageId, code) { super(errors.protocolErrorMessage(messageId, code)); this.messageId = messageId; this.code = code; }
}

const roleKey = { originServerId: Long.UONE, roleId: new Uint8Array(16).fill(9) };
const roleEntry = load('client/assets/scripts/app/RoleEntry.ts', module => ({
    './GameConnection': { ProtocolResponseError }, './GameErrors': errors, '../generated/protocol_ids': ids,
})[module]);
const { connectRoleEntry, isRoleAccountAuthenticationError } = roleEntry;

function connection(responses, httpErrors = []) {
    const requests = [], connections = [], closures = [], authRequests = [];
    const socket = { generation: 1, token: 'old-token', disconnectReason: '游戏连接已断开',
        onStatus() { return () => {}; },
        async rpc(id, value, signal) {
            if (id === 10001) {
                authRequests.push({ value, signal });
                return { accountId: new Uint8Array(16).fill(7), accountToken: 'fresh-token' };
            }
            assert.equal(id, 10007);
            requests.push({ role: value.role, signal, token: this.token });
            const error = httpErrors.shift();
            if (error) throw error;
            return { ticket: `短票${requests.length}` };
        },
        async connect(ticket, signal) {
            this.generation++;
            connections.push({ ticket, signal });
            const response = responses.shift();
            if (response instanceof Error) throw response;
            await response;
        },
        close() { closures.push(this.generation); this.generation++; },
    };
    return { socket, requests, connections, closures, authRequests };
}

test('入口短票1002或线路2002各只重取一次同角色并更换票面', async () => {
    for (const code of [1002, 2002]) {
        const state = connection([new ProtocolResponseError(20002, code)]), controller = new AbortController();
        const generation = await connectRoleEntry(state.socket, roleKey, () => {}, controller.signal);
        assert.equal(generation, state.socket.generation);
        assert.deepEqual(state.connections.map(row => row.ticket), ['短票1', '短票2']);
        assert(state.requests.every(row => row.role === roleKey && row.signal === controller.signal));
        assert(state.connections.every(row => row.signal === controller.signal));
        assert.equal(state.closures.length, 1);
    }
});

test('两次短票拒绝停止重试，普通暂态和账号错误只送一次且保留上下文', async () => {
    const error = new ProtocolResponseError(20002, 1002), repeated = connection([error, error]);
    await assert.rejects(connectRoleEntry(repeated.socket, roleKey, () => {}), rejected => rejected === error);
    assert.equal(repeated.requests.length, 2);
    assert.equal(repeated.closures.length, 2);
    for (const failure of [new ProtocolResponseError(10008, 1003), new ProtocolResponseError(10008, 1002)]) {
        const state = connection([], [failure]);
        await assert.rejects(connectRoleEntry(state.socket, roleKey, () => {}), rejected => rejected === failure);
        assert.equal(state.requests.length, 1);
        assert.equal(state.connections.length, 0);
        assert.equal(isRoleAccountAuthenticationError(failure), failure.code === 1002);
    }
    const network = connection([new Error('连接中断')]);
    await assert.rejects(connectRoleEntry(network.socket, roleKey, () => {}), /连接中断/);
    assert.equal(network.requests.length, 1);
    assert.match(errors.protocolErrorMessage(10008, 1002), /登录凭证/);
    assert.match(errors.protocolErrorMessage(20002, 1002), /入场票据/);
    assert.match(errors.protocolErrorMessage(10008, 1003), /暂时繁忙/);
    assert.equal(isRoleAccountAuthenticationError(new ProtocolResponseError(20007, 1002)), false);
});

test('旧短票拒绝晚于切换身份时既不重试也不关闭新的连接代次', async () => {
    let reject;
    const wait = new Promise((_resolve, no) => { reject = no; });
    const state = connection([wait]);
    let current = true;
    const entered = connectRoleEntry(state.socket, roleKey, () => { if (!current) throw Error('旧身份已失效'); });
    await Promise.resolve();
    current = false; state.socket.generation++;
    reject(new ProtocolResponseError(20002, 1002));
    await assert.rejects(entered, /旧身份/);
    assert.equal(state.requests.length, 1);
    assert.deepEqual(state.closures, []);
});

function reconnectFixture(state, exitGame = async () => {}) {
    const account = { id: '07'.repeat(16), name: 'account_a' }, role = { key: roleKey };
    const session = { account, role, connection: state.socket, ready: true, update: new Set(), lastError: '' };
    const status = { string: '' }, password = { string: '', node: { active: false } }, label = { string: '' };
    let click, attached = 0;
    const listeners = new Map();
    const button = { interactable: true, node: {
        getChildByName() { return { getComponent() { return label; } }; }, on(_event, listener) { click = listener; }, off() {},
    } };
    const parts = { 'Panel/Message': status, 'Panel/Password': password, 'Panel/Reconnect': button };
    const root = { active: false, getChildByPath(name) { return { getComponent() { return parts[name]; } }; },
        on(event, listener) { listeners.set(event, listener); }, off(event) { listeners.delete(event); } };
    const { GameReconnect } = load('client/assets/scripts/app/GameReconnect.ts', module => ({
        cc: { Button: { EventType: { CLICK: 'click' } }, Node: { EventType: { TOUCH_END: 'touch-end' } }, isValid: () => true }, './RoleEntry': roleEntry,
        './GameSession': { session, roleIdentity: () => '角色', isCurrentEntry: value => value.connection === state.socket.generation,
            currentEntryIdentity: () => ({ connection: state.socket.generation }) },
    })[module]);
    const dialog = new GameReconnect(root, () => true, () => {}, async () => { attached++; }, exitGame, () => {});
    return { dialog, root, status, password, button, click, get attached() { return attached; }, touch(target = root) {
        const event = { target, propagationStopped: false };
        listeners.get('touch-end')?.(event);
        return event;
    }, async retry() {
        click();
        for (let i = 0; dialog.attempt && i < 20; i++) await new Promise(resolve => setImmediate(resolve));
        assert.equal(dialog.attempt, undefined);
    } };
}

test('重连新票成功直接恢复，同一账号手工凭证只在连续拒绝后请求', async () => {
    const first = connection([new ProtocolResponseError(20002, 2002)]), recovered = reconnectFixture(first);
    await recovered.retry();
    assert.equal(recovered.root.active, false);
    assert.equal(recovered.attached, 1);
    assert.equal(first.authRequests.length, 0);
    recovered.dialog.dispose();

    const error = new ProtocolResponseError(20002, 1002), second = connection([error, error]), failed = reconnectFixture(second);
    await failed.retry();
    assert.equal(failed.root.active, true);
    assert.equal(failed.password.node.active, true);
    assert.match(failed.status.string, /取票后仍未能进入/);
    failed.password.string = 'secret_word';
    await failed.retry();
    assert.equal(failed.root.active, false);
    assert.equal(failed.password.string, '');
    assert.equal(second.authRequests.length, 1);
    assert.equal(second.authRequests[0].value.account, 'account_a');
    assert.equal(second.requests.at(-1).token, 'fresh-token');
    failed.dialog.dispose();
});

test('重连的账号1002提示当前账号口令，1003保持普通重试且不强制认证', async () => {
    for (const code of [1002, 1003]) {
        const state = connection([], [new ProtocolResponseError(10008, code)]), view = reconnectFixture(state);
        await view.retry();
        assert.equal(view.root.active, true);
        assert.equal(view.password.node.active, code === 1002);
        assert.match(view.status.string, code === 1002 ? /当前账号/ : /暂时繁忙/);
        assert.equal(state.socket.token, 'old-token');
        assert.equal(state.authRequests.length, 0);
        view.dialog.dispose();
    }
});

test('重连弹窗只有背景手势退出，取消在途重连并屏蔽迟到的成功回调', async () => {
    let entered;
    const state = connection([new Promise(resolve => { entered = resolve; })]);
    let exits = 0;
    const view = reconnectFixture(state, async () => { exits++; state.socket.close(); });
    view.click();
    await Promise.resolve();
    const attempt = state.connections[0].signal;
    assert.equal(view.touch(view.button.node).propagationStopped, false);
    assert.equal(attempt.aborted, false);
    assert.equal(view.touch().propagationStopped, true);
    assert.equal(attempt.aborted, true);
    assert.equal(view.button.interactable, false);
    view.touch(); view.click();
    entered();
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(exits, 1);
    assert.equal(state.requests.length, 1);
    assert.equal(view.attached, 0);
    assert.equal(view.dialog.attempt, undefined);
    view.dialog.dispose();
    assert.equal(view.touch().propagationStopped, false);
});

test('背景返回失败时保持遮罩并恢复重连操作', async () => {
    const state = connection([]), view = reconnectFixture(state, async () => { throw Error('加载选角失败'); });
    view.touch();
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(view.root.active, true);
    assert.equal(view.button.interactable, true);
    assert.match(view.status.string, /加载选角失败.*\n.*背景/);
    await view.retry();
    assert.equal(view.root.active, false);
    view.dialog.dispose();
});
