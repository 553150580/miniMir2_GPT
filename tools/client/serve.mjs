// 仅本机开发：静态Web Mobile包与二进制RPC/WS同源代理，不处理账号或游戏结算。
import http from 'node:http';
import net from 'node:net';
import { createReadStream, existsSync, statSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(fileURLToPath(new URL('../../client/build/web-mobile/', import.meta.url)));
const runIndex=process.argv.indexOf('--instance-id');
const runId=runIndex>=0?process.argv[runIndex+1]:'';
const sockets=new Set();
const mime = { '.html':'text/html; charset=utf-8', '.js':'text/javascript', '.json':'application/json', '.css':'text/css', '.png':'image/png', '.wasm':'application/wasm', '.ttf':'font/ttf', '.woff':'font/woff', '.svg':'image/svg+xml' };
const proxy = (request, response) => {
    const upstream = http.request({ hostname:'127.0.0.1', port:18100, path:'/rpc', method:request.method, headers:{ ...request.headers, host:'127.0.0.1:18100' } }, result => { response.writeHead(result.statusCode, result.headers); result.pipe(response); });
    upstream.on('error', () => { if (!response.headersSent) response.writeHead(503); response.end('认证服务未启动'); });
    request.pipe(upstream);
};
const server = http.createServer((request,response) => {
    if (request.url === '/favicon.ico') { response.writeHead(204); response.end(); return; }
    if (request.url === '/account/rpc') { proxy(request,response); return; }
    if (request.url === '/healthz') { response.writeHead(existsSync(path.join(root,'index.html')) ? 200 : 503, {'Content-Type':'application/json'}); response.end(JSON.stringify({webMobileBuild:existsSync(path.join(root,'index.html')),developmentOnly:true,pid:process.pid,run_id:runId})); return; }
    let target;
    try { target = path.resolve(root, '.' + decodeURIComponent(new URL(request.url,'http://localhost').pathname)); } catch { response.writeHead(400);response.end();return; }
    if (target !== root && !target.startsWith(root+path.sep)) { response.writeHead(403); response.end(); return; }
    if (existsSync(target) && statSync(target).isDirectory()) target=path.join(target,'index.html');
    if (!existsSync(target) || !statSync(target).isFile()) { response.writeHead(404); response.end('请先构建Web Mobile'); return; }
    response.writeHead(200,{'Content-Type':mime[path.extname(target)]??'application/octet-stream','Cache-Control':'no-store'}); createReadStream(target).pipe(response);
});
server.on('upgrade',(request,socket,head) => {
    if (request.url !== '/game/ws') {socket.destroy();return;}
    const upstream=net.connect(18101,'127.0.0.1',()=>{
        const headers=Object.entries({...request.headers,host:'127.0.0.1:18101'}).map(([key,value])=>`${key}: ${value}`).join('\r\n');
        upstream.write(`GET /ws HTTP/1.1\r\n${headers}\r\n\r\n`);if(head.length)upstream.write(head);socket.pipe(upstream);upstream.pipe(socket);
    });
    sockets.add(socket);sockets.add(upstream);socket.on('close',()=>sockets.delete(socket));upstream.on('close',()=>sockets.delete(upstream));
    upstream.on('error',()=>socket.destroy());socket.on('error',()=>upstream.destroy());socket.on('close',()=>upstream.destroy());
});
server.listen(18080,'127.0.0.1',()=>console.log('本机开发客户端：http://127.0.0.1:18080（仅回环监听）'));
for(const signal of (process.platform==='win32'?['SIGINT','SIGTERM','SIGBREAK']:['SIGINT','SIGTERM']))process.on(signal,()=>{for(const socket of sockets)socket.destroy();server.close(()=>process.exit(0));});
