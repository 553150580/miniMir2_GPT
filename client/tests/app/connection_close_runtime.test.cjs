const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, load } = require('./helpers/session_harness.cjs');

const { GameConnection, encodePacket, decodePacket } = load('client/assets/scripts/app/GameConnection.ts', id => {
    if (id === 'long') return Long;
    if (id === '../generated/protobuf/client_pb.js') return require('../../assets/scripts/generated/protobuf/client_pb.js');
    if (id === './InputTrace') return { traceInput() {}, inputTraceMessage() {} };
    return {};
});

function sockets(t) {
    const original = global.WebSocket, location = global.location;
    const instances = [];
    class Socket {
        static OPEN = 1;
        constructor() { this.readyState = 0; this.sent = []; instances.push(this); }
        send(bytes) { this.sent.push(decodePacket(bytes)); }
        close() { this.readyState = 3; this.onclose?.(); }
        open() { this.readyState = Socket.OPEN; this.onopen?.(); }
        deliver(id, seq, binding, value) { this.onmessage?.({ data: encodePacket(id, seq, binding, value).buffer }); }
    }
    global.WebSocket = Socket;
    global.location = { protocol: 'http:', host: '127.0.0.1:18080' };
    const connection = new GameConnection(), status = [];
    connection.onStatus(message => status.push(message));
    t.after(() => { connection.close(); global.WebSocket = original; global.location = location; });
    async function enter(ticket) {
        const joining = connection.connect(ticket), socket = instances.at(-1);
        socket.open(); await Promise.resolve();
        const request = socket.sent.at(-1);
        assert.equal(request.value.accountTicket, ticket);
        socket.deliver(20002, request.seq, Long.UONE, { sceneEpoch: Long.UONE, baselineId: Long.UONE });
        await joining;
        return socket;
    }
    return { connection, status, instances, enter };
}

test('网络关闭立即退役当前连接、拒绝未完成请求并且只通知一次，迟到旧帧不进入替换连接', async t => {
    const { connection, status, enter } = sockets(t);
    const old = await enter('第一次');
    const firstGeneration = connection.generation, incoming = [];
    connection.subscribe(packet => incoming.push(packet.id));
    const pending = connection.request(30004, { enabled: true, monsterCount: 1 });
    const rejected = assert.rejects(pending, /关闭|断开/);
    old.close(); await rejected;
    assert.equal(connection.connected, false);
    assert.ok(connection.generation > firstGeneration);
    assert.match(connection.disconnectReason, /断开/);
    assert.deepEqual(status, [connection.disconnectReason]);
    assert.equal(connection.pending.size, 0);
    old.close();
    assert.equal(status.length, 1);

    const current = await enter('第二次');
    const newerGeneration = connection.generation;
    old.close(); old.deliver(30005, 0, Long.UONE, { autoBattleEnabled: true });
    assert.equal(connection.generation, newerGeneration);
    assert.deepEqual(incoming, [20002]);
    assert.equal(connection.connected, true);
    assert.equal(connection.disconnectReason, '');
    current.deliver(30005, 0, Long.UONE, { autoBattleEnabled: false });
    assert.deepEqual(incoming, [20002, 30005]);
});

test('主动关闭清理未完成请求，不把切角或主动退出当作掉线', async t => {
    const { connection, status, enter } = sockets(t);
    const socket = await enter('退出测试');
    const pending = connection.request(30004, { enabled: false, monsterCount: 1 });
    const rejected = assert.rejects(pending, /关闭|失效/);
    connection.close(); await rejected;
    assert.equal(connection.connected, false);
    assert.equal(connection.disconnectReason, '');
    assert.equal(connection.pending.size, 0);
    assert.deepEqual(status, []);
    socket.close();
    assert.deepEqual(status, []);
});

test('握手尚未open时网络断开也立即结束，并在状态中留下重连原因', async t => {
    const { connection, status, instances } = sockets(t);
    const joining = connection.connect('断开测试');
    const rejected = assert.rejects(joining, /断开|关闭/);
    const socket = instances.at(-1);
    socket.close(); await rejected;
    assert.equal(socket.sent.length, 0);
    assert.equal(connection.connected, false);
    assert.match(connection.disconnectReason, /断开/);
    assert.deepEqual(status, [connection.disconnectReason]);
});

function clock(t) {
    t.mock.timers.enable({ apis: ['setTimeout'] });
    return async milliseconds => {
        t.mock.timers.tick(milliseconds);
        await Promise.resolve();
    };
}

test('重连终止信号会立即关闭未完成的WS握手或入场，并清理原计时器且不误报掉线', async t => {
    const tick = clock(t), { connection, status, instances } = sockets(t);
    const beforeOpen = new AbortController();
    const opening = connection.connect('等待握手', beforeOpen.signal);
    const rejectedOpen = assert.rejects(opening, /取消/);
    const first = instances.at(-1);
    beforeOpen.abort(); await rejectedOpen;
    assert.equal(first.readyState, 3);
    first.open(); await tick(10000);
    assert.deepEqual(first.sent, []);
    assert.deepEqual(status, []);

    const beforeEntry = new AbortController();
    const joining = connection.connect('等待入场', beforeEntry.signal);
    const rejectedEntry = assert.rejects(joining, /取消|关闭/);
    const second = instances.at(-1);
    second.open(); await Promise.resolve();
    const request = second.sent.at(-1);
    assert.equal(request.id, 20001);
    assert.equal(connection.pending.size, 1);
    beforeEntry.abort(); await rejectedEntry;
    assert.equal(second.readyState, 3);
    assert.equal(connection.pending.size, 0);
    second.deliver(20002, request.seq, Long.UONE, { sceneEpoch: Long.UONE, baselineId: Long.UONE });
    await tick(20000);
    assert.equal(connection.connected, false);
    assert.equal(connection.binding.toString(), '0');
    assert.deepEqual(status, []);
});

test('绑定后心跳使用当前会话和原请求序号，正常回执继续检查，主动关闭停止计时', async t => {
    const tick = clock(t), { connection, status, instances } = sockets(t);
    const signal = new AbortController();
    const joining = connection.connect('登录票据', signal.signal);
    const socket = instances.at(-1);
    socket.open(); await Promise.resolve();
    await tick(9000);
    assert.deepEqual(socket.sent.map(packet => packet.id), [20001]);
    socket.deliver(20002, socket.sent[0].seq, Long.UONE, { sceneEpoch: Long.UONE, baselineId: Long.UONE });
    await joining;
    signal.abort();
    assert.equal(connection.connected, true);
    await tick(9999);
    assert.deepEqual(socket.sent.map(packet => packet.id), [20001]);
    await tick(1);
    const heartbeat = socket.sent.at(-1);
    assert.equal(heartbeat.id, 1);
    assert.equal(heartbeat.binding.toString(), '1');
    assert.ok(Long.isLong(heartbeat.value.clientMonotonicMs));
    socket.deliver(2, heartbeat.seq + 1, Long.UONE, { serverUnixMs: Long.UONE });
    socket.deliver(2, heartbeat.seq, Long.fromInt(2, true), { serverUnixMs: Long.UONE });
    assert.equal(connection.pending.size, 1);
    socket.deliver(2, heartbeat.seq, Long.UONE, { serverUnixMs: Long.UONE });
    await tick(0);
    assert.equal(connection.pending.size, 0);
    await tick(10000);
    assert.deepEqual(socket.sent.map(packet => packet.id), [20001, 1, 1]);
    connection.close();
    await tick(30000);
    assert.equal(connection.pending.size, 0);
    assert.deepEqual(socket.sent.map(packet => packet.id), [20001, 1, 1]);
    assert.deepEqual(status, []);
});

test('后台不发送心跳也不以后台超时判掉线，回前台后持续半开才通知重连', async t => {
    const tick = clock(t), { connection, status, enter } = sockets(t);
    const previous = global.document;
    global.document = { hidden: true };
    t.after(() => { global.document = previous; });
    const socket = await enter('后台观测');
    await tick(10000);
    assert.deepEqual(socket.sent.map(packet => packet.id), [20001]);
    global.document.hidden = false;
    await tick(10000);
    assert.deepEqual(socket.sent.map(packet => packet.id), [20001, 1]);
    global.document.hidden = true;
    await tick(10000);
    assert.equal(connection.pending.size, 0);
    assert.equal(connection.connected, true);
    assert.deepEqual(status, []);

    global.document.hidden = false;
    await tick(10000);
    assert.deepEqual(socket.sent.map(packet => packet.id), [20001, 1, 1]);
    await tick(10000);
    assert.equal(connection.connected, false);
    assert.equal(connection.pending.size, 0);
    assert.match(connection.disconnectReason, /超时/);
    assert.deepEqual(status, [connection.disconnectReason]);
    await tick(30000);
    assert.equal(status.length, 1);
});
