const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Long, load } = require('./helpers/session_harness.cjs');

const { GameConnection, ProtocolResponseError, encodePacket, decodePacket } = load('client/assets/scripts/app/GameConnection.ts', id => {
    if (id === 'long') return Long;
    if (id === '../generated/protobuf/client_pb.js') return require('../../assets/scripts/generated/protobuf/client_pb.js');
    if (id === './GameErrors') return { protocolErrorMessage: () => '协议拒绝' };
    return {};
});

function setup(t, fetch) {
    const originalFetch = global.fetch, originalLocation = global.location;
    global.fetch = fetch;
    global.location = { origin: 'http://127.0.0.1:18081' };
    t.after(() => { global.fetch = originalFetch; global.location = originalLocation; });
    t.mock.timers.enable({ apis: ['setTimeout'] });
    return new GameConnection();
}

const pendingUntilAbort = signal => new Promise((_, reject) => signal.addEventListener('abort', () => reject(new Error('请求已停止')), { once: true }));
const success = (id, seq, value) => ({ ok: true, status: 200, arrayBuffer: async () => encodePacket(id, seq, Long.UZERO, value).buffer });

for (const phase of ['响应头', '响应体']) {
    test(`账号 RPC 等待${phase}超时后可正常重试，并释放原计时器`, async t => {
        const signals = [];
        const connection = setup(t, async (url, request) => {
            assert.equal(url, 'http://127.0.0.1:18081/account/rpc');
            signals.push(request.signal);
            if (signals.length === 1) {
                return phase === '响应头' ? pendingUntilAbort(request.signal)
                    : { ok: true, status: 200, arrayBuffer: () => pendingUntilAbort(request.signal) };
            }
            return success(10002, decodePacket(request.body).seq, { accountToken: '第二次认证', accountId: new Uint8Array(16) });
        });
        const first = connection.rpc(10001, { account: 'test_user', password: 'test_password' });
        const checked = assert.rejects(first, /账号服务响应超时/);
        await Promise.resolve();
        t.mock.timers.tick(9999);
        assert.equal(signals[0].aborted, false);
        t.mock.timers.tick(1);
        await checked;
        assert.equal(signals[0].aborted, true);

        const result = await connection.rpc(10001, { account: 'test_user', password: 'test_password' });
        assert.equal(result.accountToken, '第二次认证');
        assert.notEqual(signals[1], signals[0]);
        t.mock.timers.tick(10000);
        assert.equal(signals[1].aborted, false);
    });
}

test('账号 RPC 支持现有重连取消，并保持明确协议拒绝的错误类型', async t => {
    const signals = [];
    const connection = setup(t, async (_url, request) => {
        signals.push(request.signal);
        if (signals.length === 1) return pendingUntilAbort(request.signal);
        const { seq } = decodePacket(request.body);
        return { ok: true, status: 200, arrayBuffer: async () => Uint8Array.of(1, 1, 10008 >> 8, 10008 & 255, seq, 0, 234, 7).buffer };
    });
    const upper = new AbortController();
    const role = { originServerId: Long.UONE, roleId: new Uint8Array(16) };
    const first = connection.rpc(10007, { role }, upper.signal);
    const checked = assert.rejects(first, /账号请求已取消/);
    upper.abort();
    await checked;
    assert.equal(signals[0].aborted, true);
    await assert.rejects(connection.rpc(10007, { role }, upper.signal), /账号请求已取消/);
    assert.equal(signals.length, 1);

    const current = new AbortController();
    await assert.rejects(connection.rpc(10007, { role }, current.signal),
        error => error instanceof ProtocolResponseError && error.messageId === 10008 && error.code === 1002);
    current.abort();
    assert.equal(signals[1].aborted, false);
});
