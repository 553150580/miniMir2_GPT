// 使用实际触屏、持久未决请求和新票验收金币补时，页面与接口均指向本轮隔离服务。
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const manifest = path.resolve(process.argv[2] ?? '');
assert(manifest.startsWith(path.resolve('.work') + path.sep), '补时验收清单必须位于独立工作目录');
const expected = JSON.parse(fs.readFileSync(manifest, 'utf8')), output = path.dirname(manifest);
const base = new URL(expected.baseUrl), endpoint = new URL(expected.endpoint);
assert(base.protocol === 'http:' && base.hostname === '127.0.0.1' && base.port !== '18080');
assert(endpoint.protocol === 'ws:' && endpoint.hostname === '127.0.0.1' && !['18101', '18103'].includes(endpoint.port));
assert.equal(typeof expected.enabled, 'boolean');

const safe = 'Canvas/Safe/', panel = `${safe}UIMgr/Panels/Hosting`, confirm = `${safe}PopupView/Panels/Confirm`, reconnect = 'Canvas/ReconnectDialog';

async function scene(page, name) {
    await page.waitForFunction(name => {
        const current = window.cc?.director?.getScene();
        return current?.name === name && current.getComponentsInChildren('AppRoot').length &&
            cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy;
    }, name, { timeout: 55000 });
}

// 按钮坐标取已保存的 Cocos 控件，所有业务动作使用 Chromium 触屏。
async function touch(page, fullPath, choice) {
    const handle = await page.waitForFunction(({ fullPath, choice }) => {
        const root = cc.find(fullPath);
        const node = choice === undefined ? root : root?.children.find(child => child.activeInHierarchy &&
            child.getChildByName('Text')?.getComponent(cc.Label)?.string.replace(/^● /u, '') === choice);
        if (!node?.activeInHierarchy || node.getComponent(cc.Button)?.interactable === false) return null;
        const bounds = node._uiProps?.uiTransformComp?.getBoundingBoxToWorld();
        if (!bounds) return null;
        const size = cc.view.getVisibleSize(), layout = document.querySelector('canvas').getBoundingClientRect();
        return { x: layout.x + (bounds.x + bounds.width / 2) / size.width * layout.width,
            y: layout.y + (size.height - bounds.y - bounds.height / 2) / size.height * layout.height };
    }, { fullPath, choice }, { timeout: 20000 });
    try { const point = await handle.jsonValue(); await page.touchscreen.tap(point.x, point.y); }
    finally { await handle.dispose(); }
}

async function edit(page, field, value) {
    assert(value, '独立补时账号认证配置缺失');
    await touch(page, safe + field);
    const input = page.locator('input:visible');
    await input.waitFor({ state: 'visible', timeout: 5000 });
    await input.fill(value);
}

// 绑定与请求号配对真实帧。除原报价和经济请求外不记录任何鉴权消息。
function header(payload) {
    let offset = 4;
    const next = () => {
        let value = 0n, shift = 0n;
        do {
            const byte = payload[offset++];
            value |= BigInt(byte & 0x7f) << shift;
            if (!(byte & 0x80)) return value.toString();
            shift += 7n;
        } while (offset < payload.length);
        throw new Error('WebSocket帧头意外结束');
    };
    const request = next(), binding = next();
    return { key: `${binding}:${request}`, binding, offset };
}

const hex = value => Buffer.from(value).toString('hex');

async function state(page, session, fromBinding = '', target) {
    const handle = await page.waitForFunction(({ session, fromBinding, target }) => {
        const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0], { inventory, wallet, hosting } = session;
        if (!session.ready || session.entry?.sceneKind !== 1 || !app?.world?.isMapReady || !inventory || !wallet || !hosting) return null;
        const binding = session.connection.binding.toString(), boundGold = wallet.boundGold.toString(), gold = wallet.gold.toString();
        if (fromBinding && BigInt(binding) <= BigInt(fromBinding) || target && (boundGold !== target.boundGold || gold !== target.gold ||
            hosting.remainHostingSec !== target.balance)) return null;
        return { binding, role: session.entryRole, version: session.entry.resourceVersion.toString(), level: inventory.level,
            revision: inventory.revision.toString(), boundGold, gold, balance: hosting.remainHostingSec };
    }, { session, fromBinding, target }, { timeout: 20000 });
    try { return await handle.jsonValue(); } finally { await handle.dispose(); }
}

async function visibleText(page, container, name, prefix) {
    const handle = await page.waitForFunction(({ container, name, prefix }) => {
        const root = cc.find(container), child = root?.getChildByName(name);
        const label = child?.getComponent(cc.Label)?.string;
        return root?.activeInHierarchy && child?.activeInHierarchy && label?.includes(prefix) ? label : null;
    }, { container, name, prefix }, { timeout: 10000 });
    try { return await handle.jsonValue(); } finally { await handle.dispose(); }
}

// 只中断首个实际成功回执送入客户端的瞬间。服务器已发送结果，浏览器仍须用原操作对账。
async function loseFirstReceipt(page, session) {
    await page.evaluate(session => {
        const socket = session.connection.socket, receive = socket.onmessage;
        socket.onmessage = function (event) {
            const payload = new Uint8Array(event.data);
            if (payload.length >= 6 && payload[2] * 256 + payload[3] === 40005 && !(payload[1] & 1)) {
                this.close(4001, '隔离补时：首回执未送达客户端');
                return;
            }
            return receive.call(this, event);
        };
    }, session);
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, errors: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage(), quoteRequests = new Map(), quotes = new Map(), buys = new Map(), replies = new Map();
    page.on('pageerror', error => report.errors.push(error.message));
    let connections = 0;
    page.on('websocket', connection => {
        if (connection.url() !== expected.endpoint) { report.errors.push('客户端连接越出本轮隔离节点'); return; }
        connections++;
        connection.on('framesent', ({ payload }) => {
            if (typeof payload === 'string' || payload.length < 6) return;
            const message = payload.readUInt16BE(2);
            if (message !== 40032 && message !== 40007) return;
            const { key, binding, offset } = header(payload);
            if (message === 40032) quoteRequests.set(key, pb.HostingQuoteRequest.decode(payload.subarray(offset)).requested_seconds);
            else buys.set(key, { binding, wire: payload.subarray(offset), value: pb.BuyHostingTimeRequest.decode(payload.subarray(offset)) });
        });
        connection.on('framereceived', ({ payload }) => {
            if (typeof payload === 'string' || payload.length < 6 || payload[1] & 1) return;
            const message = payload.readUInt16BE(2);
            if (message !== 40033 && message !== 40005) return;
            const { key, offset } = header(payload);
            if (message === 40033 && quoteRequests.has(key)) quotes.set(key, pb.HostingQuoteResult.decode(payload.subarray(offset)));
            if (message === 40005 && buys.has(key)) replies.set(key, pb.OperationResult.decode(payload.subarray(offset)));
        });
    });

    let session;
    try {
        report.step = '触屏登录并选择本轮隔离区服和角色';
        await page.goto(base.href);
        await scene(page, 'Login');
        await edit(page, 'Account', process.env.MINIMIR_HOSTING_BROWSER_ACCOUNT);
        await edit(page, 'Password', process.env.MINIMIR_HOSTING_BROWSER_PASSWORD);
        await touch(page, safe + 'Login');
        await scene(page, 'ServerSelect');
        await touch(page, safe + 'RegionList/View/Content', expected.region);
        await touch(page, safe + 'ServerList/View/Content', expected.server);
        await scene(page, 'RoleSelect');
        await page.waitForFunction(name => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string.includes(name), expected.roleName);
        await touch(page, safe + 'Enter');
        await scene(page, 'Game');
        session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
        const firstState = await state(page, session);
        assert.equal(firstState.version, String(expected.ruleVersion));
        if (await page.evaluate(session => session.hosting.autoBattleEnabled, session)) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await page.waitForFunction(session => session.hosting?.autoBattleEnabled === false, session, { timeout: 10000 });
        }
        const before = await state(page, session);
        await touch(page, safe + 'BottomHUD/QuickNav/Hosting');
        if (!expected.enabled) {
            report.step = '当前规则仅显示免费领时和原托管入口';
            await visibleText(page, panel, 'Message', '当前规则未开放金币补时');
            const present = await page.evaluate(path => {
                const panel = cc.find(path);
                const active = name => !!panel.getChildByName(name)?.activeInHierarchy;
                return { purchase: ['BuyMinus', 'BuyQuantity', 'BuyPlus', 'BuyQuote'].some(active), claim: active('Claim'), leave: active('Leave') };
            }, panel);
            assert.deepEqual(present, { purchase: false, claim: true, leave: true });
            assert([...quotes].some(([key, row]) => quoteRequests.get(key) === 0 && row.error_code === 3801));
            assert.equal(buys.size, 0);
            report.unavailable = { discoveryCode: 3801, purchaseHidden: true, originalActionsVisible: true };
            await page.screenshot({ path: path.join(output, 'hosting-price-unavailable.png') });
        } else {
            report.step = '读取有效档位，调整数量并取消第一次确认';
            await touch(page, `${panel}/BuyPlus`);
            const discovery = [...quotes].find(([key, value]) => quoteRequests.get(key) === 0 && value.error_code === 0)?.[1];
            assert(discovery && discovery.quote_id.length === 0 && discovery.requested_seconds === 0);
            const seconds = 2 * discovery.granularity_seconds;
            await touch(page, `${panel}/BuyQuote`);
            const message = await visibleText(page, confirm, 'Message', '标准毛金币 G_ref');
            const firstQuote = [...quotes].filter(([key]) => quoteRequests.get(key) === seconds).at(-1)?.[1];
            assert(firstQuote && firstQuote.quote_id.length === 16);
            await touch(page, `${confirm}/CancelButton`);
            await page.waitForFunction(path => !cc.find(path)?.activeInHierarchy, confirm);
            assert.equal(buys.size, 0, '取消确认不能产生资产请求');
            report.cancelledWithoutPurchase = true;

            report.step = '确认相同数量的有效原报价并拦截首笔真实回执';
            await touch(page, `${panel}/BuyQuote`);
            const locked = await visibleText(page, confirm, 'Message', '标准毛金币 G_ref');
            const quote = [...quotes].filter(([key]) => quoteRequests.get(key) === seconds).at(-1)?.[1];
            const rate = BigInt(quote.gold_per_hour.toString()), reference = BigInt(quote.reference_gold_per_hour.toString());
            const total = BigInt(quote.total_gold.toString()), bound = BigInt(quote.bound_gold_cost.toString()), gold = BigInt(quote.gold_cost.toString());
            const band = quote.band_max_level ? `${quote.band_min_level}–${quote.band_max_level}级` : `${quote.band_min_level}级及以上`;
            assert.equal(locked, message);
            assert(locked.includes(`${band} · 报价版本 ${quote.quote_version}`) && locked.includes(`G_ref：${reference}`)
                && locked.includes(`每小时售价（2倍）：${rate}`) && locked.includes(`合计 ${total}金币`)
                && locked.includes(`先扣绑定金币 ${bound}，再扣金币 ${gold}`));
            assert(rate === reference * 2n && total === (rate * BigInt(seconds) + 3599n) / 3600n);
            assert(bound > 0n && gold > 0n && bound + gold === total && bound === BigInt(before.boundGold));
            assert.equal(quote.before_seconds.toString(), String(before.balance));
            await page.screenshot({ path: path.join(output, 'hosting-purchase-confirmation.png') });
            await loseFirstReceipt(page, session);
            await touch(page, `${confirm}/ConfirmButton`);
            await page.waitForFunction(path => cc.find(path)?.activeInHierarchy, reconnect, { timeout: 15000 });
            assert.equal(buys.size, 1);
            const [originalKey, original] = buys.entries().next().value, firstReply = replies.get(originalKey);
            assert(firstReply?.hosting_purchase && firstReply.error_code === 0 && !firstReply.pending);
            assert.equal(hex(original.value.quote_id), hex(quote.quote_id));
            assert.equal(original.value.requested_seconds, seconds);
            assert.equal(original.value.operation.expected_revision.toString(), quote.role_revision.toString());
            const originalOperation = hex(original.value.operation.op_id);
            const retained = await page.evaluate(session => {
                const operation = session.pending?.get(session.entryRole);
                return operation ? { id: operation.id, op: Array.from(operation.value.operation.opId, byte => byte.toString(16).padStart(2, '0')).join('') } : null;
            }, session);
            assert.deepEqual(retained, { id: 40007, op: originalOperation });

            report.step = '实际重连接回后，核对原请求和只加一次的 typed 回执';
            await touch(page, `${reconnect}/Panel/Reconnect`);
            const expectedAssets = { balance: before.balance + seconds, boundGold: (BigInt(before.boundGold) - bound).toString(),
                gold: (BigInt(before.gold) - gold).toString() };
            const after = await state(page, session, before.binding, expectedAssets);
            assert.equal(after.role, before.role);
            await page.waitForFunction(path => !cc.find(path)?.activeInHierarchy, reconnect, { timeout: 15000 });
            if (!await page.evaluate(path => !!cc.find(path)?.activeInHierarchy, panel)) await touch(page, safe + 'BottomHUD/QuickNav/Hosting');
            const receipt = await visibleText(page, panel, 'BuyReceipt', '已购');
            assert(receipt.includes(`扣绑定金币 ${bound}、金币 ${gold}`) && receipt.includes(`报价版本 ${quote.quote_version}`));
            assert.equal(buys.size, 2);
            assert.equal(replies.size, 2);
            const [replayKey, replay] = [...buys].at(-1), replayReply = replies.get(replayKey);
            assert(replay.binding !== original.binding && replayReply?.hosting_purchase && replayReply.error_code === 0);
            assert.deepEqual(replay.wire, original.wire, '新票回执查询沿用相同40007原体');
            assert.deepEqual(pb.HostingPurchaseReceipt.encode(replayReply.hosting_purchase).finish(), pb.HostingPurchaseReceipt.encode(firstReply.hosting_purchase).finish());
            assert.equal(replayReply.hosting_purchase.balance_seconds.toString(), String(expectedAssets.balance));
            assert.equal(await page.evaluate(session => session.pending?.get(session.entryRole) ?? null, session), null);
            report.quote = { seconds, version: quote.quote_version.toString(), reference: reference.toString(), rate: rate.toString(),
                total: total.toString(), bound: bound.toString(), gold: gold.toString(), text: locked };
            report.purchase = { opId: originalOperation, quoteId: hex(quote.quote_id), requestedSeconds: seconds, boundGoldCost: bound.toString(),
                goldCost: gold.toString(), beforeSeconds: before.balance, balanceSeconds: after.balance, previousBinding: before.binding,
                sameRequest: true, sameReceipt: true, receiptText: receipt };
            await page.screenshot({ path: path.join(output, 'hosting-purchase-recovered.png') });
        }
        report.connectedSockets = connections;
        assert.equal(connections, expected.enabled ? 2 : 1);
        assert.deepEqual(report.errors, []);
        report.passed = true;
        delete report.step;
    } finally {
        await session?.dispose();
        await browser.close();
        fs.writeFileSync(path.join(output, 'hosting-purchase-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
        if (!report.passed) console.error('补时浏览器验收步骤 ' + report.step);
    }
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
