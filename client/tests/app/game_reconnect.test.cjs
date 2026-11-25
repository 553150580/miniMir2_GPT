// 使用真实 GameReconnect、GameSession 与首领迁移模型，控制每次地图挂接完成的先后顺序。
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { load, harness, entry, Long } = require('./helpers/session_harness.cjs');
const { StaleMapAttachmentError } = require('../../../.work/ui-tests/app/MapAttachmentCoordinator.js');
const occurrence = 'ab'.repeat(16);

class Label { string = ''; }
class EditBox { string = ''; node = { active: false }; }
class Node {
    static EventType = { TOUCH_END: 'touch-end' };
    active = true;
    children = new Map();
    parts = new Map();
    handlers = new Map();
    on(event, handler, owner) { this.handlers.set(event, handler.bind(owner)); }
    off(event) { this.handlers.delete(event); }
    getComponent(type) { return this.parts.get(type); }
    getChildByName(name) { return this.children.get(name); }
    getChildByPath(path) { return path.split('/').reduce((node, name) => node.getChildByName(name), this); }
    child(name) { const child = new Node(); this.children.set(name, child); return child; }
}
class Button {
    static EventType = { CLICK: 'click' };
    interactable = true;
    constructor(node) { this.node = node; }
}

// 等待 async 接续和重连成功回调；避免依赖真浏览器时钟和秒级睡眠。
const flush = () => new Promise(resolve => setImmediate(resolve));

function setup(t) {
    const game = harness();
    const { session } = game;
    const conn = session.connection;
    session.account = { name: 'test', id: 'test' };
    session.originServer = { originServerId: Long.UONE };
    conn.token = 'test';
    conn.closeCount = 0;
    conn.close = () => { conn.closeCount++; conn.generation++; conn.connected = false; };
    const subscribe = conn.onStatus.bind(conn);
    conn.onStatus = listener => { subscribe(listener); return () => {}; };
    const initial = entry('9007199254740993', '1', 1);
    const boss = { ...entry('9007199254740994', '81', 4), serverTransitionId: occurrence };
    const cc = { Label, EditBox, Button, Node, isValid: () => true };
    const base = 'client/assets/scripts/app/';
    const { GameReconnect } = load(`${base}GameReconnect.ts`, id => {
        if (id === 'cc') return cc;
        if (id === './GameSession') return game;
        if (id === './MapAttachmentCoordinator') return { StaleMapAttachmentError };
        if (id === './MapTravelModel') return require('../../../.work/ui-tests/app/MapTravelModel.js');
        if (id === './RoleEntry') return {
            isEntryTicketError: () => false,
            isRoleAccountAuthenticationError: () => false,
            async connectRoleEntry(connection, key, requireScope) {
                assert.equal(key, session.role.key);
                requireScope();
                connection.generation++;
                connection.binding = Long.fromInt(7, true);
                connection.connected = true;
                connection.deliver(20002, initial);
                return connection.generation;
            },
        };
        return {};
    });
    const root = new Node(), dialog = root.child('Panel'), status = new Label();
    const retry = dialog.child('Reconnect');
    dialog.child('Message').parts.set(Label, status);
    dialog.child('Password').parts.set(EditBox, new EditBox());
    retry.parts.set(Button, new Button(retry));
    retry.child('Label').parts.set(Label, new Label());
    const attachments = [];
    let mountedScene = '';
    const controller = new GameReconnect(root, () => mountedScene === session.entry?.sceneEpoch.toString(), () => {}, () => {
        const requestedScene = session.entry.sceneEpoch.toString();
        return new Promise((resolve, reject) => attachments.push({ requestedScene, mount: () => { mountedScene = requestedScene; resolve(); }, reject }));
    }, async () => {}, () => {});
    t.after(() => controller.dispose());
    return { session, conn, initial, boss, root, status, attachments, retry: () => retry.handlers.get('click')() };
}

function deliverBoss(h) {
    h.conn.deliver(20020, { occurrenceId: occurrence, sceneEpoch: h.boss.sceneEpoch, state: 2 });
    h.conn.deliver(20002, h.boss);
    assert.equal(h.session.entryBossTransition, true, '必须是 GameSession 接受的权威首领切换');
}

test('重连普通图任务退役后跟随同一新连接的权威首领图，仅在原场次挂接并确认后关闭弹窗', async t => {
    const h = setup(t);
    h.retry();
    await flush();
    assert.deepEqual(h.attachments.map(task => task.requestedScene), [h.initial.sceneEpoch.toString()]);

    // 服务端 ACK 普通图后立即恢复原场次，旧地图异步挂接任务此时尚未返回。
    h.conn.deliver(6, h.initial);
    deliverBoss(h);
    await flush();
    assert.deepEqual(h.attachments.map(task => task.requestedScene), [h.initial.sceneEpoch.toString(), h.boss.sceneEpoch.toString()]);
    h.attachments[0].reject(new StaleMapAttachmentError('普通图的旧挂接已退役'));
    await flush();
    assert.equal(h.root.active, true);
    assert.equal(h.conn.closeCount, 0);
    h.attachments[1].mount();
    await flush();
    assert.equal(h.root.active, true, '仅挂接还缺少首领图的服务端确认');
    h.conn.deliver(6, h.boss);
    await flush();
    assert.equal(h.root.active, false);
    assert.equal(h.conn.closeCount, 0);
    assert.equal(h.session.lastError, '');
});

test('只恢复普通地图时，重连仍需匹配当前挂接和服务端确认', async t => {
    const h = setup(t);
    h.retry();
    await flush();
    h.attachments[0].mount();
    await flush();
    assert.equal(h.root.active, true);
    h.conn.deliver(6, h.initial);
    await flush();
    assert.equal(h.root.active, false);
    assert.equal(h.conn.closeCount, 0);
});

test('同一角色的新连接取代重连代次，旧挂接成功也不能关闭弹窗或关闭新连接', async t => {
    const h = setup(t);
    h.retry();
    await flush();
    h.conn.generation++;
    h.conn.binding = Long.fromInt(8, true);
    h.conn.deliver(20002, entry('9007199254740995', '1', 1));
    h.attachments[0].mount();
    await flush();
    assert.equal(h.root.active, true);
    assert.equal(h.conn.closeCount, 0);
    assert.equal(h.attachments.length, 1);
});
