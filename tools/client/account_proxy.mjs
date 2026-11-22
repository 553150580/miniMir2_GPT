import http from 'node:http';

const proxyDeadlineMs = 15000;

// 有界代理覆盖上传、响应头和响应体。浏览器先断开时立即释放账号上游。
export function proxyAccountRpc(request, response, accountPort, timeoutMs = proxyDeadlineMs) {
    const fail = (status, message) => {
        if (response.destroyed || response.writableEnded) return;
        if (response.headersSent) {
            response.destroy();
        } else {
            response.writeHead(status, { 'Content-Type': 'text/plain; charset=utf-8' });
            response.end(message);
        }
    };
    const unavailable = () => fail(503, '认证服务不可用');
    const upstream = http.request({
        hostname: '127.0.0.1',
        port: accountPort,
        path: '/rpc',
        method: request.method,
        headers: { ...request.headers, host: '127.0.0.1:' + accountPort },
    }, result => {
        result.on('error', unavailable);
        if (response.destroyed || response.writableEnded) {
            result.destroy();
            return;
        }
        response.writeHead(result.statusCode, result.headers);
        result.pipe(response);
    });
    const deadline = setTimeout(() => {
        fail(504, '认证服务响应超时');
        upstream.destroy();
    }, timeoutMs);
    response.once('close', () => {
        clearTimeout(deadline);
        if (!response.writableFinished) upstream.destroy();
    });
    upstream.on('error', unavailable);
    request.pipe(upstream);
}
