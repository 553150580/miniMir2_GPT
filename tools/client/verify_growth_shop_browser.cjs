// 用 Creator Web Mobile 实包触屏验证保存入口、原币种确认与绑定到账。
'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const functions = safe + 'UIMgr/Panels/Functions';
const entry = functions + '/Pages/Page2/GrowthShop';
const panel = safe + 'PopupView/Panels/ItemActions';
const content = panel + '/List/View/Content';
const hex = value => Buffer.from(value).toString('hex');

// 只记录本轮角色的商城操作编号和确切成交明细。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) {
            report.errors.push('浏览器连接了隔离范围之外的节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40073) return;
            const q = pb.GrowthShopCommitRequest.decode(frame.body);
            report.requests.push({ product: q.product_id, count: q.count, quote: hex(q.quote_id) });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40005) return;
            const reply = pb.OperationResult.decode(frame.body), result = reply.growth_shop;
            if (!result) return;
            report.receipts.push({ operation: hex(reply.op_id), quote: hex(result.quote_id), product: result.terms.product_id,
                currency: result.terms.currency, count: result.terms.count, total: result.terms.total_price_units.toString(),
                balance: result.balance_units.toString(), error: reply.error_code, pending: reply.pending,
                items: result.items.map(item => ({ item: item.item_id, quantity: item.quantity.toString(), bound: item.bound, source: item.source_kind })) });
        });
    });
}

// action 通过场景保存的三个固定按钮进行真实触屏。
async function action(page, index, text) {
    await label(page, panel + '/Action' + index + '/Label', text);
    await touch(page, panel + '/Action' + index);
}

// waitForPurchase 核对角色收到的资产快照和两个钱包，确认页文字不作为唯一证据。
async function waitForPurchase(page, session, amount, bound, itemCount) {
    return wait(page, ({ session, amount, bound, itemCount }) => {
        const { wallet, inventory } = session;
        const items = inventory?.items.filter(item => item.sourceKind === 'growth_shop');
        if (!wallet || wallet.yuanbaoUnits.toString() !== amount || wallet.boundYuanbaoUnits.toString() !== bound ||
            !items || items.length !== itemCount || items.some(item => !item.bound)) return null;
        return items.map(item => [item.itemId, item.quantity.toString(), item.bound]);
    }, { session, amount, bound, itemCount });
}

// buy 先确认显示的确切货币金额和份数，再触摸提交。
async function buy(page, report, text, count, spend, name) {
    const before = report.requests.length;
    await touch(page, content, text);
    for (let i = 1; i < count; i++) await touch(page, panel + '/Plus');
    await action(page, 1, '查看报价');
    await label(page, panel + '/Title', '确认购买');
    const terms = await page.evaluate(panel => cc.find(panel + '/List').getComponent('List').items.join(' '), panel);
    assert(terms.includes(`×${count}`) && terms.includes('到账为绑定材料') && terms.includes(spend), terms);
    assert.equal(report.requests.length, before);
    await page.screenshot({ path: path.join(output, name + '.png') });
    await action(page, 1, '确认购买');
    await label(page, panel + '/Title', '成长商城');
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, bundleSha256: expected.bundleSha256, fixtureSha256: expected.fixtureSha256,
        sockets: 0, errors: [], requests: [], receipts: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    try {
        const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
            deviceScaleFactor: 1, serviceWorkers: 'block' });
        const page = await context.newPage();
        observe(page, report);
        const bundle = page.waitForResponse(response => response.status() === 200 &&
            response.url() === expected.baseUrl + '/assets/main/index.js', { timeout: 65000 });
        void bundle.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_GROWTH_SHOP_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_GROWTH_SHOP_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.bundleSha256);
        const world = await originalWorld(page, session);
        assert.equal(world.role, expected.role);
        if (world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, model => model.hosting?.autoBattleEnabled === false, session);
        }
        await touch(page, safe + 'BottomHUD/Footer/Functions');
        await touch(page, functions + '/Tabs/Tab2');
        if (expected.enabled) {
            await label(page, entry + '/Label', '成长商城');
            await touch(page, entry);
            await label(page, panel + '/Title', '成长商城');
            report.catalog = await wait(page, node => {
                const rows = cc.find(node + '/List').getComponent('List').items.filter(row => row.key);
                return rows.length === 5 ? rows.map(row => row.label) : null;
            }, panel);
            assert(report.catalog.includes('洗炼石 · 价格待定'));
            await page.screenshot({ path: path.join(output, 'catalog.png') });
            await buy(page, report, '祝福油 · 100.0元宝', 1, '支付 100.0 元宝', 'ordinary');
            await waitForPurchase(page, session, '300', '500', 1);
            await buy(page, report, '书页 · 10.0绑定元宝', 2, '支付 20.0 绑定元宝', 'bound');
            report.inventory = await waitForPurchase(page, session, '300', '300', 2);
            assert.deepEqual(report.inventory, [[900021, '1', true], [900001, '2', true]]);
            assert.deepEqual(report.receipts.map(r => [r.product, r.currency, r.count, r.total, r.balance, r.error, r.pending]),
                [[1, 1, 1, '1000', '300', 0, false], [4, 2, 2, '200', '300', 0, false]]);
            assert.deepEqual(report.requests, report.receipts.map(r => ({ product: r.product, count: r.count, quote: r.quote })));
        } else {
            await wait(page, node => cc.find(node)?.parent?.activeInHierarchy, entry);
            report.defaultEntryHidden = await page.evaluate(node => cc.find(node)?.activeInHierarchy === false, entry);
            assert(report.defaultEntryHidden);
            assert.deepEqual(report.requests, []);
            await page.screenshot({ path: path.join(output, 'default.png') });
        }
        assert.deepEqual(report.errors, []);
        assert.equal(report.sockets, 1);
        report.passed = true;
    } finally {
        fs.writeFileSync(path.join(output, 'growth-shop-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
