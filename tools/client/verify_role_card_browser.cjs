// 用真实 Creator 触屏验收原双月卡面板、断线未决重放、服务端钱包和同场景返回。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, confirm, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const hosting = safe + 'UIMgr/Panels/Hosting';
const cards = safe + 'UIMgr/Panels/RoleCards';
const confirmPanel = safe + 'PopupView/Panels/Confirm';
const reconnect = 'Canvas/ReconnectDialog';
const amounts = { privilege: BigInt(expected.privilegeUnits), hosting: BigInt(expected.hostingUnits), daily: BigInt(expected.dailyUnits) };
const initial = { yuanbao: BigInt(expected.initialYuanbao), bound: BigInt(expected.initialBoundYuanbao) };

const hex = value => Buffer.from(value).toString('hex');
const key = frame => frame.binding + ':' + frame.request;
const priceText = value => value % 10n === 0n ? (value / 10n).toString() : `${value / 10n}.${value % 10n}`;

// observe 将原帧严格按绑定和请求号配对，忽略鉴权和角色入场凭据。
function observe(page, errors) {
    const seen = { sockets: 0, requests: new Map(), states: new Map(), replies: new Map() };
    const clientTypes = { 40035: pb.RoleCardQuery, 40037: pb.BuyRoleCardRequest, 40038: pb.ClaimRoleCardDailyRequest };
    page.on('pageerror', error => errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) { errors.push('触屏客户端连到了非隔离节点'); return; }
        seen.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload), codec = clientTypes[frame?.id];
            if (codec) seen.requests.set(key(frame), { ...frame, value: codec.decode(frame.body) });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload), request = seen.requests.get(frame && key(frame));
            if (frame?.id === 40036 && request?.id === 40035) seen.states.set(key(frame), pb.RoleCardState.decode(frame.body));
            if (frame?.id === 40005 && [40037, 40038].includes(request?.id)) seen.replies.set(key(frame), pb.OperationResult.decode(frame.body));
        });
    });
    return seen;
}

function quoted(seen, productId) {
    return [...seen.states].filter(([id, state]) => seen.requests.get(id).value.product_id === productId && !state.error_code).at(-1)?.[1];
}

function executions(seen, id) {
    return [...seen.requests].filter(([, request]) => request.id === id)
        .map(([operation, request]) => ({ request, reply: seen.replies.get(operation) }));
}

async function openCards(page) {
    if (await page.evaluate(path => !!cc.find(path)?.activeInHierarchy, cards)) return;
    if (!await page.evaluate(path => !!cc.find(path)?.activeInHierarchy, hosting)) await touch(page, safe + 'BottomHUD/QuickNav/Hosting');
    await touch(page, hosting + '/RoleCards');
    await label(page, cards + '/Title', '双月卡');
}

async function wallet(page, session, yuanbao, bound, newerBinding = '') {
    return wait(page, ({ session, yuanbao, bound, newerBinding }) => {
        if (!session.ready || !session.wallet || session.entry?.sceneKind !== 1) return null;
        const binding = session.connection.binding.toString();
        if (newerBinding && BigInt(binding) <= BigInt(newerBinding)) return null;
        const current = { yuanbaoUnits: session.wallet.yuanbaoUnits.toString(), boundYuanbaoUnits: session.wallet.boundYuanbaoUnits.toString() };
        return current.yuanbaoUnits === yuanbao && current.boundYuanbaoUnits === bound ? current : null;
    }, { session, yuanbao: yuanbao.toString(), bound: bound.toString(), newerBinding });
}

async function nextQuote(page, seen, kind) {
    const productId = expected[kind + 'Product'];
    const button = kind === 'privilege' ? 'BuyPrivilege' : 'BuyHosting';
    const name = kind === 'privilege' ? '特权月卡' : '托管月卡';
    await label(page, `${cards}/${button}/Label`, `${priceText(amounts[kind])} 元宝`);
    await touch(page, `${cards}/${button}`);
    const text = await label(page, `${confirmPanel}/Message`, `${name} 30 天`);
    const state = quoted(seen, productId), quote = state?.quote;
    assert(quote?.quote_id.length === 16 && quote.product.product_id === productId && quote.product.days === 30);
    assert.equal(quote.product.price_yuanbao_units.toString(), amounts[kind].toString());
    assert(text.includes(`${priceText(amounts[kind])} 非绑定元宝`));
    return quote;
}

function confirmed(operation, quote, expectedBalance) {
    const row = operation.reply?.role_card_purchase;
    assert(row && !operation.reply.pending && operation.reply.error_code === 0);
    assert.equal(hex(operation.request.value.quote_id), hex(quote.quote_id));
    assert.equal(operation.request.value.product_id, quote.product.product_id);
    assert.equal(row.yuanbao_balance_units.toString(), expectedBalance.toString());
    assert.equal(BigInt(row.new_until_unix_ms.toString()) - BigInt(row.effective_at_unix_ms.toString()), 30n * 86400000n);
    return { opId: hex(operation.request.value.operation.op_id), quoteId: hex(quote.quote_id),
        expiration: row.new_until_unix_ms.toString(), priceUnits: row.quote.product.price_yuanbao_units.toString() };
}

async function geometry(page) {
    return page.evaluate(path => {
        const root = cc.find(path), names = ['Title', 'Wallet', 'PrivilegeInfo', 'BuyPrivilege', 'HostingInfo', 'BuyHosting', 'Claim', 'Status'];
        const bounds = names.map(name => {
            const node = root.getChildByName(name), box = node._uiProps.uiTransformComp.getBoundingBoxToWorld();
            return { name, top: box.yMax, bottom: box.yMin, left: box.xMin, right: box.xMax };
        });
        const panel = root._uiProps.uiTransformComp.getBoundingBoxToWorld();
        return { bounds, nonOverlapping: bounds.every((row, index) => !index || row.top < bounds[index - 1].bottom),
            fits: bounds.every(row => row.top <= panel.yMax && row.bottom >= panel.yMin && row.left >= panel.xMin && row.right <= panel.xMax) };
    }, cards);
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, errors: [], feature: { entryVisible: false },
        clientBundleSha256: expected.clientBundleSha256, fixtureSha256: expected.fixtureSha256 };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
        deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage(), seen = observe(page, report.errors);
    let session;
    try {
        report.step = '真实触屏登录、选区、选服和选角色';
        const runtimeBundle = page.waitForResponse(response => new URL(response.url()).pathname === '/assets/main/index.js' && response.ok(), { timeout: 65000 });
        void runtimeBundle.catch(() => {});
        session = await login(page, 'roleName', { account: process.env.MINIMIR_ROLE_CARD_BROWSER_ACCOUNT, password: process.env.MINIMIR_ROLE_CARD_BROWSER_PASSWORD });
        const bundle = await (await runtimeBundle).body();
        assert.equal(crypto.createHash('sha256').update(bundle).digest('hex'), report.clientBundleSha256);
        const before = await originalWorld(page, session);
        assert.equal(before.rule, String(expected.ruleVersion));
        if (before.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        await wallet(page, session, initial.yuanbao, initial.bound);
        await touch(page, safe + 'BottomHUD/QuickNav/Hosting');
        if (!expected.enabled) {
            report.step = '确认已加载新协议但无目录的服务仍保留原托管入口';
            await wait(page, () => {
                const root = cc.find('Canvas/Safe/UIMgr/Panels/Hosting');
                const ui = cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.roleCards;
                return root?.activeInHierarchy && ui?.checked && !ui.checking && !!ui.state;
            });
            const discovery = quoted(seen, 0);
            assert(discovery && discovery.products.length === 0 && !discovery.quote);
            assert.deepEqual(await page.evaluate(path => {
                const root = cc.find(path);
                return ['RoleCards', 'Claim', 'Leave'].map(name => root.getChildByName(name).activeInHierarchy);
            }, hosting), [false, true, true]);
            assert.equal(executions(seen, 40037).length + executions(seen, 40038).length, 0);
            report.feature.originalHostingActionsVisible = true;
            await page.screenshot({ path: path.join(output, 'role-card-disabled.png') });
        } else {
            report.step = '打开已保存双月卡面板并取消第一次购买确认';
            await openCards(page);
            assert.deepEqual(quoted(seen, 0).products.map(item => item.kind).sort(), [1, 2]);
            report.feature.entryVisible = true;
            report.geometry = await geometry(page);
            assert(report.geometry.nonOverlapping && report.geometry.fits, JSON.stringify(report.geometry));
            await page.screenshot({ path: path.join(output, 'role-card-initial.png') });
            await nextQuote(page, seen, 'privilege');
            await touch(page, `${confirmPanel}/CancelButton`);
            await wait(page, path => !cc.find(path)?.activeInHierarchy, confirmPanel);
            assert.equal(executions(seen, 40037).length, 0);
            report.feature.openingQuoteCancelled = true;

            report.step = '确认特权月卡，拦下首个真实回执触发原经济操作恢复';
            const privilege = await nextQuote(page, seen, 'privilege');
            await page.evaluate(session => {
                const socket = session.connection.socket, listener = socket.onmessage;
                socket.onmessage = function (event) {
                    const bytes = new Uint8Array(event.data);
                    if (bytes[0] === 1 && bytes[1] === 0 && bytes[2] * 256 + bytes[3] === 40005) { this.close(4001, '隔离复核首次购卡回执'); return; }
                    return listener.call(this, event);
                };
            }, session);
            await touch(page, confirm);
            await wait(page, path => !!cc.find(path)?.activeInHierarchy, reconnect);
            const original = executions(seen, 40037)[0], afterPrivilege = initial.yuanbao - amounts.privilege;
            const first = confirmed(original, privilege, afterPrivilege);
            assert.deepEqual(await page.evaluate(session => {
                const pending = session.pending?.get(session.entryRole);
                return pending ? { id: pending.id, op: Array.from(pending.value.operation.opId, b => b.toString(16).padStart(2, '0')).join('') } : null;
            }, session), { id: 40037, op: first.opId });

            report.step = '新票回原地图并复核同一请求、回执和新余额';
            await touch(page, `${reconnect}/Panel/Reconnect`);
            await wallet(page, session, afterPrivilege, initial.bound, before.binding);
            await wait(page, path => !cc.find(path)?.activeInHierarchy, reconnect);
            await openCards(page);
            await label(page, cards + '/Status', '特权月卡购买成功');
            const resumed = await originalWorld(page, session);
            assert.deepEqual([resumed.role, resumed.map, resumed.scene, resumed.life], [before.role, before.map, before.scene, before.life]);
            const replay = executions(seen, 40037)[1];
            assert.equal(executions(seen, 40037).length, 2);
            assert.equal(replay.request.binding, resumed.binding);
            assert.deepEqual(replay.request.body, original.request.body);
            assert.deepEqual(pb.RoleCardPurchaseReceipt.encode(replay.reply.role_card_purchase).finish(), pb.RoleCardPurchaseReceipt.encode(original.reply.role_card_purchase).finish());
            assert.equal(await page.evaluate(session => session.pending?.get(session.entryRole) ?? null, session), null);
            report.feature.privilegePurchased = true;
            report.reconnect = { sameRequest: true, sameReceipt: true, oldBinding: before.binding, newBinding: resumed.binding, sameOrdinaryScene: true };
            report.purchase = { privilege: first };

            report.step = '领取原上海运营日的特权绑定元宝';
            await label(page, cards + '/Claim/Label', `领取 ${priceText(amounts.daily)} 绑定元宝`);
            await touch(page, cards + '/Claim');
            const afterDaily = initial.bound + amounts.daily;
            await wallet(page, session, afterPrivilege, afterDaily);
            await label(page, cards + '/Status', `已领取 ${priceText(amounts.daily)} 绑定元宝`);
            const daily = executions(seen, 40038)[0], receipt = daily.reply?.role_card_daily;
            assert(receipt && !daily.reply.pending && daily.reply.error_code === 0);
            assert.equal(receipt.operating_day, daily.request.value.operating_day);
            assert.equal(receipt.granted_units.toString(), amounts.daily.toString());
            assert.equal(receipt.bound_yuanbao_balance_units.toString(), afterDaily.toString());
            assert.equal(executions(seen, 40038).length, 1);
            report.feature.dailyClaimed = true;
            report.daily = { opId: hex(daily.request.value.operation.op_id), operatingDay: receipt.operating_day, grantedUnits: receipt.granted_units.toString() };

            report.step = '确认托管月卡，已领取状态仍属于原角色';
            const hostingQuote = await nextQuote(page, seen, 'hosting');
            await touch(page, confirm);
            const afterHosting = afterPrivilege - amounts.hosting;
            report.wallet = await wallet(page, session, afterHosting, afterDaily);
            await label(page, cards + '/Status', '托管月卡购买成功');
            await label(page, cards + '/Claim/Label', '今日特权奖励不可领');
            assert.equal(executions(seen, 40037).length, 3);
            report.purchase.hosting = confirmed(executions(seen, 40037)[2], hostingQuote, afterHosting);
            report.feature.hostingPurchased = true;
            assert(await page.evaluate(path => !cc.find(path).getComponent(cc.Button).interactable, cards + '/Claim'));
            await page.screenshot({ path: path.join(output, 'role-card-purchased.png') });
            await page.touchscreen.tap(195, 90);
            await wait(page, path => !!cc.find(path)?.activeInHierarchy, hosting);
            assert(!await page.evaluate(path => !!cc.find(path)?.activeInHierarchy, cards));
            report.feature.backToHosting = true;
        }
        assert.equal(seen.sockets, expected.enabled ? 2 : 1);
        assert.deepEqual(report.errors, []);
        report.passed = true;
        delete report.step;
    } finally {
        if (!report.passed) await page.screenshot({ path: path.join(output, 'role-card-failure.png'), timeout: 5000 }).catch(() => {});
        await session?.dispose();
        await browser.close();
        fs.writeFileSync(path.join(output, 'role-card-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
        if (!report.passed) console.error('双月卡浏览器验收步骤：' + report.step);
    }
    console.log(`双月卡真实触屏 ${expected.enabled ? '购买、日领与重连' : '缺省拒绝'} 验收通过`);
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
