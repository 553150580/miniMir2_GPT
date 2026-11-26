// 通过真实触屏、上架原物品实例和操作重连覆盖候选道具市场。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, confirm, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const functions = `${safe}UIMgr/Panels/Functions`;
const entry = `${functions}/Pages/Page2/ItemMarket`;
const market = `${safe}UIMgr/Panels/ItemMarket`;
const listing = `${safe}UIMgr/Panels/ItemMarketListing`;
const order = `${safe}UIMgr/Panels/ItemMarketOrder`;
const confirmMessage = `${safe}PopupView/Panels/Confirm/Message`;
const reconnect = 'Canvas/ReconnectDialog';

const hex = value => Buffer.from(value).toString('hex');
const key = packet => `${packet.binding}:${packet.request}`;

function observe(page, errors) {
    const result = { sockets: 0, commands: new Map(), responses: new Map(), states: [] };
    page.on('pageerror', error => errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) { errors.push('浏览器接到了非隔离市场节点'); return; }
        result.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40041) result.commands.set(key(frame), { ...frame, value: pb.ItemMarketAction.decode(frame.body) });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40040) result.states.push(pb.ItemMarketState.decode(frame.body));
            if (frame?.id === 40005 && result.commands.has(key(frame))) result.responses.set(key(frame), pb.OperationResult.decode(frame.body));
        });
    });
    return result;
}

function commands(watch, action) {
    return [...watch.commands].filter(([, command]) => command.value.action === action)
        .map(([nonce, command]) => ({ command, response: watch.responses.get(nonce) }));
}

function confirmed(watch, action, index = -1) {
    const row = commands(watch, action).at(index);
    assert(row?.response?.item_market && row.response.error_code === 0 && !row.response.pending);
    return { ...row, receipt: row.response.item_market };
}

async function client(browser, side, errors) {
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
        deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage(), watch = observe(page, errors);
    const runtimeBundle = page.waitForResponse(response => new URL(response.url()).pathname === '/assets/main/index.js' && response.ok(), { timeout: 65000 });
    void runtimeBundle.catch(() => {});
    const session = await login(page, side, { account: process.env[`MINIMIR_MARKET_${side.toUpperCase()}_ACCOUNT`],
        password: process.env[`MINIMIR_MARKET_${side.toUpperCase()}_PASSWORD`] });
    const bundle = await (await runtimeBundle).body();
    assert.equal(crypto.createHash('sha256').update(bundle).digest('hex'), expected.clientBundleSha256);
    const world = await originalWorld(page, session);
    assert.equal(world.rule, String(expected.ruleVersion));
    if (world.auto) {
        await touch(page, `${safe}MapViewport/Actions/Auto`);
        await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
    }
    return { page, session, world, watch };
}

async function openFunctions(page) {
    await touch(page, `${safe}BottomHUD/Footer/Functions`);
    await touch(page, `${functions}/Tabs/Tab2`);
}

async function openMarket(page) {
    if (await page.evaluate(path => !!cc.find(path)?.activeInHierarchy, market)) return;
    if (!await page.evaluate(path => !!cc.find(path)?.activeInHierarchy, functions)) await openFunctions(page);
    await touch(page, entry);
    await label(page, `${market}/Title`, '道具交易行');
}

async function selectTab(page, name) {
    const path = `${market}/Tabs/${name}`;
    const scope = { Public: 1, Mine: 2, Escrow: 3 }[name];
    if (await page.evaluate(scope => cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.itemMarket?.scope === scope, scope)) return;
    await touch(page, path);
}

// 使用现存 Cocos EditBox 的 DOM 输入桥和触屏失焦，业务输入不经 JS 修改节点。
async function edit(page, panel, field, value) {
    await touch(page, `${panel}/${field}`);
    const input = page.locator('input:visible');
    await input.waitFor({ state: 'visible', timeout: 6000 });
    await input.fill(value);
    await touch(page, `${panel}/Title`);
}

async function sell(seller, quantity, price) {
    const { page, watch } = seller;
    await page.bringToFront();
    await openMarket(page);
    await touch(page, `${market}/ListItem`);
    await touch(page, `${listing}/List/View/Content`, expected.itemName);
    await edit(page, listing, 'Quantity', quantity);
    await edit(page, listing, 'Price', price);
    await touch(page, `${listing}/Submit`);
    await label(page, confirmMessage, `${price} 元宝/件`);
    await touch(page, confirm);
    await label(page, `${market}/Status`, '上架已确认');
    return hex(confirmed(watch, 1).receipt.order_id);
}

async function waitMarketRow(client, targetID, phase, rowContains) {
    const { page, watch } = client;
    await page.bringToFront();
    await openMarket(page);
    const deadline = Date.now() + 42000;
    do {
        await touch(page, `${market}/Refresh`);
        await wait(page, () => !cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.itemMarket?.loading);
        const latest = watch.states.at(-1)?.orders.find(row => hex(row.order_id) === targetID && row.state === phase);
        if (latest) { await touch(page, `${market}/List/View/Content`, rowContains); return latest; }
        await page.waitForTimeout(750);
    } while (Date.now() < deadline);
    throw new Error(`正式页面一直没有取得目标订单的阶段 ${phase}`);
}

async function pay(client, amountText) {
    const { page } = client;
    await touch(page, `${order}/Action`);
    await label(page, confirmMessage, amountText);
    await touch(page, confirm);
}

async function accountWallet(client, amount, bound, beforeBinding = '') {
    const { page, session } = client;
    return wait(page, ({ session, amount, bound, beforeBinding }) => {
        if (!session.ready || !session.wallet || session.entry?.sceneKind !== 1) return null;
        const binding = session.connection.binding.toString();
        if (beforeBinding && BigInt(binding) <= BigInt(beforeBinding)) return null;
        const yuanbao = session.wallet.yuanbaoUnits.toString(), boundYuanbao = session.wallet.boundYuanbaoUnits.toString();
        return yuanbao === amount && boundYuanbao === bound ? { yuanbao, boundYuanbao, binding } : null;
    }, { session, amount, bound, beforeBinding });
}

async function claim(client, phrase) {
    const { page } = client;
    await page.bringToFront();
    await openMarket(page);
    await selectTab(page, 'Escrow');
    await touch(page, `${market}/Refresh`);
    await touch(page, `${market}/List/View/Content`, phrase);
    await pay(client, phrase);
    await label(page, `${market}/Status`, '领取已确认');
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, errors: [], feature: { entryVisible: false }, clientBundleSha256: expected.clientBundleSha256,
        fixtureSha256: expected.fixtureSha256, orderIDs: {} };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    let seller, buyer;
    try {
        report.step = '持卡角色正常触屏登录并打开原功能交易页';
        seller = await client(browser, 'seller', report.errors);
        await openFunctions(seller.page);
        if (!expected.enabled) {
            await wait(seller.page, () => {
                const market = cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.itemMarket;
                return market?.checked && !market.loading;
            });
            assert(!await seller.page.evaluate(path => !!cc.find(path)?.activeInHierarchy, entry));
            assert(seller.watch.states.at(-1)?.enabled === false && !seller.watch.commands.size);
            await seller.page.screenshot({ path: path.join(output, 'item-market-disabled.png') });
        } else {
            await openMarket(seller.page);
            report.feature.entryVisible = true;
            buyer = await client(browser, 'buyer', report.errors);
            await openMarket(buyer.page);

            report.step = '卖家上架，买家冻结整单并按服务端价格加价';
            report.orderIDs.auction = await sell(seller, '2', '1.5');
            const auction = await waitMarketRow(buyer, report.orderIDs.auction, 1, '1.5 元宝/件');
            assert.equal(auction.remaining_quantity.toString(), '2');
            await pay(buyer, '冻结 3.0 非绑定元宝');
            await label(buyer.page, `${market}/Status`, '预购已确认');
            await accountWallet(buyer, '970', '113');
            await selectTab(buyer.page, 'Mine');
            const bid = await waitMarketRow(buyer, report.orderIDs.auction, 1, '1.5 元宝/件');
            assert(bid.my_active_bid && bid.active_bid_count === 1 && bid.next_unit_price_yuanbao_units.toString() === '17');
            await pay(buyer, '单价 1.5 → 1.7 元宝');
            await label(buyer.page, `${market}/Status`, '加价已确认');
            await accountWallet(buyer, '966', '113');
            report.feature.bidAndRaise = true;

            report.step = '另行上架无人预购及可主动撤销的订单';
            report.orderIDs.direct = await sell(seller, '2', '2.0');
            report.orderIDs.cancel = await sell(seller, '1', '4.0');
            await selectTab(seller.page, 'Mine');
            await waitMarketRow(seller, report.orderIDs.cancel, 1, '4.0 元宝/件');
            await pay(seller, '余货退入交易托管仓');
            await label(seller.page, `${market}/Status`, '撤单已确认');
            report.feature.cancelledOrder = true;

            report.step = '无人预购后执行部分直购，首包丢失时保留原未决操作';
            await selectTab(buyer.page, 'Public');
            const direct = await waitMarketRow(buyer, report.orderIDs.direct, 2, '2.0 元宝/件');
            assert.equal(direct.remaining_quantity.toString(), '2');
            await buyer.page.evaluate(session => {
                const socket = session.connection.socket, receive = socket.onmessage;
                socket.onmessage = function (event) {
                    const msg = new Uint8Array(event.data);
                    if (msg[0] === 1 && msg[1] === 0 && msg[2] * 256 + msg[3] === 40005) { this.close(4001, '隔离市场首返回'); return; }
                    return receive.call(this, event);
                };
            }, buyer.session);
            await pay(buyer, '支付 2.0 非绑定元宝');
            await wait(buyer.page, path => !!cc.find(path)?.activeInHierarchy, reconnect);
            const first = confirmed(buyer.watch, 4);
            report.directOp = hex(first.command.value.operation.op_id);
            assert.deepEqual(await buyer.page.evaluate(session => {
                const pending = session.pending?.get(session.entryRole);
                return pending ? { id: pending.id, op: Array.from(pending.value.operation.opId, byte => byte.toString(16).padStart(2, '0')).join('') } : null;
            }, buyer.session), { id: 40041, op: report.directOp });
            await touch(buyer.page, `${reconnect}/Panel/Reconnect`);
            report.buyerWallet = await accountWallet(buyer, '946', '113', buyer.world.binding);
            await wait(buyer.page, session => !!session.ready && !session.pending?.get(session.entryRole), buyer.session);
            const resumed = await originalWorld(buyer.page, buyer.session);
            assert.deepEqual([resumed.map, resumed.scene, resumed.role], [buyer.world.map, buyer.world.scene, buyer.world.role]);
            const again = confirmed(buyer.watch, 4);
            assert.equal(commands(buyer.watch, 4).length, 2);
            assert.notEqual(first.command.binding, again.command.binding);
            assert.deepEqual(first.command.body, again.command.body);
            assert.deepEqual(pb.ItemMarketReceipt.encode(first.receipt).finish(), pb.ItemMarketReceipt.encode(again.receipt).finish());
            report.reconnect = { sameRequest: true, sameReceipt: true, originalMap: resumed.map, afterBinding: resumed.binding };
            report.feature.partialDirect = true;

            report.step = '从已保存托管页领取一件退货、两笔实收和竞购赢家原物品';
            await claim(seller, `${expected.itemName} ×1`);
            await claim(seller, '成交所得 3.1 元宝');
            await claim(seller, '成交所得 1.8 元宝');
            report.sellerWallet = await accountWallet(seller, '49', '77');
            await claim(buyer, `${expected.itemName} ×2`);
            await accountWallet(buyer, '946', '113');
            report.feature.claimedEscrow = true;
            await seller.page.screenshot({ path: path.join(output, 'item-market-seller.png') });
            await buyer.page.screenshot({ path: path.join(output, 'item-market-buyer.png') });
        }
        assert.equal(seller.watch.sockets, 1);
        if (buyer) assert.equal(buyer.watch.sockets, 2);
        assert.deepEqual(report.errors, []);
        report.passed = true;
        delete report.step;
    } finally {
        if (!report.passed) {
            if (seller) await seller.page.screenshot({ path: path.join(output, 'item-market-seller-failure.png'), timeout: 3000 }).catch(() => {});
            if (buyer) await buyer.page.screenshot({ path: path.join(output, 'item-market-buyer-failure.png'), timeout: 3000 }).catch(() => {});
        }
        for (const actor of [seller, buyer]) await actor?.session.dispose();
        await browser.close();
        fs.writeFileSync(path.join(output, 'item-market-browser-report.json'), `${JSON.stringify(report, null, 2)}\n`, 'utf8');
        if (!report.passed) console.error('道具市场触屏验收步骤：' + report.step);
    }
    console.log(`道具市场 ${expected.enabled ? '上架、预购、加价、撤单、部分直购、托管及重连' : '缺省关闭'}通过`);
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
