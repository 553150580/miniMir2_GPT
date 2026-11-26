// 在两个真实 Creator 触屏客户端上验收非绑定账户、挂单回执重放和原货币托管领取。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, confirm, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const functions = `${safe}UIMgr/Panels/Functions`;
const entry = `${functions}/Pages/Page2/GoldMarket`;
const market = `${safe}UIMgr/Panels/GoldMarket`;
const place = `${safe}UIMgr/Panels/GoldMarketPlace`;
const detail = `${safe}UIMgr/Panels/GoldMarketDetail`;
const confirmText = `${safe}PopupView/Panels/Confirm/Message`;
const reconnect = 'Canvas/ReconnectDialog';

const hex = value => Buffer.from(value).toString('hex');
const key = frame => `${frame.binding}:${frame.request}`;

function observe(page, errors) {
    const log = { sockets: 0, commands: new Map(), replies: new Map(), states: [], queries: [] };
    page.on('pageerror', error => errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) { errors.push('金币触屏验收连接到了非隔离节点'); return; }
        log.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40039 || frame?.id === 40042) log.queries.push(frame.id);
            if (frame?.id === 40044) log.commands.set(key(frame), { ...frame, value: pb.GoldMarketAction.decode(frame.body) });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40043) log.states.push(pb.GoldMarketState.decode(frame.body));
            if (frame?.id === 40005 && log.commands.has(key(frame))) log.replies.set(key(frame), pb.OperationResult.decode(frame.body));
        });
    });
    return log;
}

function successful(actor, action, index = -1) {
    const [nonce, operation] = [...actor.log.commands].filter(([, frame]) => frame.value.action === action).at(index) ?? [];
    const reply = actor.log.replies.get(nonce);
    assert(operation && reply?.gold_market && reply.error_code === 0 && !reply.pending);
    return { operation, receipt: reply.gold_market };
}

async function client(browser, side, errors) {
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
        deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage(), log = observe(page, errors);
    const asset = page.waitForResponse(response => new URL(response.url()).pathname === '/assets/main/index.js' && response.ok(), { timeout: 65000 });
    void asset.catch(() => {});
    const who = `MINIMIR_GOLD_MARKET_${side.toUpperCase()}`;
    const session = await login(page, side, { account: process.env[who + '_ACCOUNT'], password: process.env[who + '_PASSWORD'] });
    assert.equal(crypto.createHash('sha256').update(await (await asset).body()).digest('hex'), expected.clientBundleSha256);
    const world = await originalWorld(page, session);
    assert.equal(world.rule, String(expected.ruleVersion));
    if (world.auto) {
        await touch(page, `${safe}MapViewport/Actions/Auto`);
        await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
    }
    return { page, session, world, log };
}

async function tradeTab(page) {
    await touch(page, `${safe}BottomHUD/Footer/Functions`);
    await touch(page, `${functions}/Tabs/Tab2`);
}

async function openMarket(page) {
    if (await page.evaluate(path => !!cc.find(path)?.activeInHierarchy, market)) return;
    if (!await page.evaluate(path => !!cc.find(path)?.activeInHierarchy, functions)) await tradeTab(page);
    await touch(page, entry);
    await label(page, `${market}/Title`, '金币交易行');
}

async function select(actor, name) {
    const scope = { Buy: 1, Sell: 2, Mine: 3, Escrow: 4 }[name];
    if (await actor.page.evaluate(scope => cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.goldMarket?.scope === scope, scope)) return;
    await touch(actor.page, `${market}/Tabs/${name}`);
}

async function edit(page, field, value) {
    await touch(page, `${place}/${field}`);
    const input = page.locator('input:visible');
    await input.waitFor({ state: 'visible', timeout: 6000 });
    await input.fill(value);
    await touch(page, `${place}/Title`);
}

async function prepare(actor, side, lots, price, freeze) {
    const { page } = actor;
    await page.bringToFront();
    await openMarket(page);
    await touch(page, `${market}/Place`);
    const target = `${place}/${side}`;
    if (await page.evaluate(path => cc.find(path)?.getComponent(cc.Button)?.interactable, target)) await touch(page, target);
    await label(page, `${place}/Terms`, '限价 8.0～12.0 元宝/手');
    await edit(page, 'Lots', lots);
    await edit(page, 'Price', price);
    await touch(page, `${place}/Submit`);
    const preview = await label(page, confirmText, freeze);
    assert(preview.includes('委托 2分钟') && preview.includes('卖方佣金 10%') && preview.includes(`每手 ${price} 元宝`));
    return preview;
}

async function wallet(actor, gold, yuanbao, oldBinding = '') {
    const { page, session } = actor;
    return wait(page, ({ session, gold, yuanbao, oldBinding }) => {
        if (!session.ready || !session.wallet || session.entry?.sceneKind !== 1) return null;
        const binding = session.connection.binding.toString();
        if (oldBinding && BigInt(binding) <= BigInt(oldBinding)) return null;
        const actual = { gold: session.wallet.gold.toString(), yuanbao: session.wallet.yuanbaoUnits.toString(), binding,
            boundGold: session.wallet.boundGold.toString(), boundYuanbao: session.wallet.boundYuanbaoUnits.toString() };
        return actual.gold === gold && actual.yuanbao === yuanbao ? actual : null;
    }, { session, gold, yuanbao, oldBinding });
}

async function observeRow(actor, scope, matcher, text) {
    await actor.page.bringToFront();
    await openMarket(actor.page);
    await select(actor, scope);
    const deadline = Date.now() + 32000;
    do {
        await touch(actor.page, `${market}/Refresh`);
        await wait(actor.page, () => !cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.goldMarket?.loading);
        const state = actor.log.states.at(-1);
        const item = state && matcher(state);
        if (item) {
            await touch(actor.page, `${market}/List/View/Content`, text);
            return item;
        }
        await actor.page.waitForTimeout(450);
    } while (Date.now() < deadline);
    throw new Error('同步后仍看不到委托或托管：' + text);
}

async function claim(actor, kind, text) {
    const row = await observeRow(actor, 'Escrow', view => view.escrows.find(escrow => escrow.kind === kind), text);
    await touch(actor.page, `${detail}/Action`);
    await label(actor.page, confirmText, text);
    await touch(actor.page, confirm);
    await label(actor.page, `${market}/Status`, '领取已确认');
    const result = successful(actor, 3);
    assert.equal(hex(result.receipt.escrow_id), hex(row.escrow_id));
    return result.receipt;
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, errors: [], feature: { entryVisible: false },
        clientBundleSha256: expected.clientBundleSha256, fixtureSha256: expected.fixtureSha256, orders: {} };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    let seller, buyer;
    try {
        report.step = '正常选服、角色和公开交易功能入口';
        seller = await client(browser, 'seller', report.errors);
        await tradeTab(seller.page);
        await wait(seller.page, () => {
            const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
            return app?.goldMarket?.checked && !app.goldMarket.loading && app?.itemMarket?.checked && !app.itemMarket.loading;
        });
        report.feature.itemMarketPreserved = await seller.page.evaluate(path => !!cc.find(path)?.activeInHierarchy, `${functions}/Pages/Page2/ItemMarket`);
        assert.equal(report.feature.itemMarketPreserved, true);
        assert.deepEqual(seller.log.queries.slice(0, 2).sort(), [40039, 40042]);
        if (!expected.enabled) {
            assert(seller.log.states.at(-1)?.enabled === false && !seller.log.states.at(-1)?.rules);
            assert(!await seller.page.evaluate(path => !!cc.find(path)?.activeInHierarchy, entry));
            assert.equal(seller.log.commands.size, 0);
            await seller.page.screenshot({ path: path.join(output, 'gold-market-disabled.png') });
        } else {
            report.feature.entryVisible = true;
            await openMarket(seller.page);
            buyer = await client(browser, 'buyer', report.errors);
            await openMarket(buyer.page);

            report.step = '发布原两手卖单并模拟已提交后未接首回执';
            const sellText = await prepare(seller, 'Sell', '2', '9.0', '冻结 20000 非绑定金币');
            await seller.page.screenshot({ path: path.join(output, 'gold-market-sell-confirmation.png') });
            await seller.page.evaluate(session => {
                const socket = session.connection.socket, receive = socket.onmessage;
                socket.onmessage = function (event) {
                    const bytes = new Uint8Array(event.data);
                    if (bytes[0] === 1 && bytes[1] === 0 && bytes[2] * 256 + bytes[3] === 40005) { this.close(4001, '隔离金币委托首回执'); return; }
                    return receive.call(this, event);
                };
            }, seller.session);
            await touch(seller.page, confirm);
            await wait(seller.page, path => !!cc.find(path)?.activeInHierarchy, reconnect);
            const first = successful(seller, 1, 0);
            assert.equal(first.receipt.state, 1);
            assert.equal(first.receipt.filled_lots.toString(), '0');
            assert.equal(first.receipt.gold_delta.toString(), '-20000');
            assert.equal(first.operation.value.expected_duration_seconds, 120);
            assert.equal(first.operation.value.unit_price_yuanbao_units.toString(), '90');
            report.orders.seller = hex(first.receipt.order_id);
            report.sellerOp = hex(first.operation.value.operation.op_id);
            assert.deepEqual(await seller.page.evaluate(session => {
                const row = session.pending?.get(session.entryRole);
                return row ? { id: row.id, op: Array.from(row.value.operation.opId, b => b.toString(16).padStart(2, '0')).join('') } : null;
            }, seller.session), { id: 40044, op: report.sellerOp });
            await touch(seller.page, `${reconnect}/Panel/Reconnect`);
            const resumedBalance = await wallet(seller, '30000', '0', seller.world.binding);
            await wait(seller.page, session => !!session.ready && !session.pending?.get(session.entryRole), seller.session);
            const resumed = await originalWorld(seller.page, seller.session), replay = successful(seller, 1);
            assert.deepEqual([resumed.map, resumed.scene, resumed.role], [seller.world.map, seller.world.scene, seller.world.role]);
            assert.notEqual(replay.operation.binding, first.operation.binding);
            assert.deepEqual(replay.operation.body, first.operation.body);
            assert.deepEqual(pb.GoldMarketReceipt.encode(replay.receipt).finish(), pb.GoldMarketReceipt.encode(first.receipt).finish());
            report.reconnect = { sameRequest: true, sameReceipt: true, sameOrdinaryScene: true, oldBinding: seller.world.binding, newBinding: resumedBalance.binding };
            report.feature.sellerPlaced = true;

            report.step = '发布较高限价的一手买单，服务端按在簿卖价撮合';
            const buyText = await prepare(buyer, 'Buy', '1', '11.0', '冻结 11.0 非绑定元宝');
            await touch(buyer.page, confirm);
            await label(buyer.page, `${market}/Status`, '委托已受理，等待撮合');
            await wallet(buyer, '0', '890');
            const buy = successful(buyer, 1);
            assert.equal(buy.receipt.filled_lots.toString(), '0');
            assert.equal(buy.receipt.yuanbao_delta_units.toString(), '-110');
            report.orders.buyer = hex(buy.receipt.order_id);
            report.feature.buyerPlaced = true;
            report.previews = { sell: sellText, buy: buyText };

            report.step = '卖方撤销只剩一手的部分成交委托';
            await observeRow(seller, 'Mine', view => view.orders.find(row => hex(row.order_id) === report.orders.seller && row.owned && row.remaining_lots.toString() === '1'), '剩余 1 手');
            await touch(seller.page, `${detail}/Action`);
            await label(seller.page, confirmText, '撤销剩余 1 手');
            await touch(seller.page, confirm);
            await label(seller.page, `${market}/Status`, '撤单已确认');
            assert.equal(successful(seller, 2).receipt.state, 3);
            report.feature.cancelRemaining = true;

            report.step = '双方分别领取成交、价格改善退款和撤单返还';
            assert.equal((await claim(seller, 2, '卖方实收 8.1 元宝')).yuanbao_delta_units.toString(), '81');
            assert.equal((await claim(seller, 4, '卖单金币返还 10000 金币')).gold_delta.toString(), '10000');
            assert.equal((await claim(buyer, 1, '买入金币 10000 金币')).gold_delta.toString(), '10000');
            assert.equal((await claim(buyer, 3, '买单元宝退款 2.0 元宝')).yuanbao_delta_units.toString(), '20');
            report.sellerWallet = await wallet(seller, '40000', '81');
            report.buyerWallet = await wallet(buyer, '10000', '910');
            assert.deepEqual([report.sellerWallet.boundGold, report.sellerWallet.boundYuanbao], ['70000', '77']);
            assert.deepEqual([report.buyerWallet.boundGold, report.buyerWallet.boundYuanbao], ['88000', '113']);
            report.feature.claimedGoldAndYuanbao = true;
            await seller.page.screenshot({ path: path.join(output, 'gold-market-seller-final.png') });
            await buyer.page.screenshot({ path: path.join(output, 'gold-market-buyer-final.png') });
        }
        assert.equal(seller.log.sockets, expected.enabled ? 2 : 1);
        if (buyer) assert.equal(buyer.log.sockets, 1);
        assert.deepEqual(report.errors, []);
        report.passed = true;
        delete report.step;
    } finally {
        if (!report.passed) for (const [name, actor] of [['seller', seller], ['buyer', buyer]]) {
            if (actor) await actor.page.screenshot({ path: path.join(output, `gold-market-${name}-failure.png`), timeout: 3000 }).catch(() => {});
        }
        for (const actor of [seller, buyer]) await actor?.session.dispose();
        await browser.close();
        fs.writeFileSync(path.join(output, 'gold-market-browser-report.json'), `${JSON.stringify(report, null, 2)}\n`, 'utf8');
        if (!report.passed) console.error('金币触屏验收步骤：' + report.step);
    }
    console.log(`金币市场真实触屏 ${expected.enabled ? '挂单双边撮合、断线重放、撤单及原角色领取' : '缺省关闭且道具功能保留'}通过`);
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
