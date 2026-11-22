const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createServer } = require('node:http');
const { once } = require('node:events');

async function listen(t, handler) {
    const server = createServer(handler);
    t.after(async () => {
        if (!server.listening) return;
        const closed = new Promise(resolve => server.close(resolve));
        server.closeAllConnections();
        await closed;
    });
    server.listen(0, '127.0.0.1');
    await once(server, 'listening');
    return server.address().port;
}

async function proxy(t, upstream, timeoutMs) {
    const { proxyAccountRpc } = await import('../../../tools/client/account_proxy.mjs');
    const upstreamPort = await listen(t, upstream);
    const port = await listen(t, (request, response) => proxyAccountRpc(request, response, upstreamPort, timeoutMs));
    return `http://127.0.0.1:${port}/account/rpc`;
}

test('账号代理上游一直不回响应时按总时限返回504，重试可以通过', { timeout: 5000 }, async t => {
    let calls = 0;
    const url = await proxy(t, (request, response) => {
        assert.equal(request.url, '/rpc');
        request.resume();
        if (++calls === 2) response.end('重新认证成功');
    }, 150);
    const first = await fetch(url, { method: 'POST', body: '第一次' });
    assert.equal(first.status, 504);
    assert.match(await first.text(), /认证服务响应超时/);
    const retry = await fetch(url, { method: 'POST', body: '第二次' });
    assert.equal(retry.status, 200);
    assert.equal(await retry.text(), '重新认证成功');
    assert.equal(calls, 2);
});

test('账号代理也限制只收到部分响应体的请求', { timeout: 5000 }, async t => {
    let closed;
    const url = await proxy(t, (request, response) => {
        request.resume();
        closed = once(response, 'close');
        response.write('响应体暂未结束');
    }, 180);
    const first = await fetch(url, { method: 'POST', body: '第一次' });
    await assert.rejects(first.text());
    await closed;
});

test('账号代理在自身截止前释放已断开的客户端对应上游', { timeout: 5000 }, async t => {
    let closed;
    const url = await proxy(t, (request, response) => {
        request.resume();
        closed = once(response, 'close', { signal: AbortSignal.timeout(1500) });
        response.write('响应体暂未结束');
    }, 4000);
    const controller = new AbortController();
    const next = await fetch(url, { method: 'POST', body: '请求', signal: controller.signal });
    controller.abort();
    await assert.rejects(next.text());
    await closed;
});
