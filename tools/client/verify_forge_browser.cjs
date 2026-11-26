// 从 Creator 保存界面触屏选择图纸、原矿、绑定投入及真实报价，记录最终成品。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const inventory = safe + 'UIMgr/Panels/Inventory';
const details = safe + 'PopupView/Panels/ItemActions';
const dialog = safe + 'PopupView/Panels/Confirm';
const hex = bytes => Buffer.from(bytes).toString('hex');

function itemView(item) {
    return { instance: hex(item.instance_id), item: item.item_id, bound: item.bound, purity: item.ore_purity,
        version: item.version.toString(), source: item.source_kind, sourceId: item.source_id };
}

// 只观察触屏真正经过原游戏节点的包，不向 HTTP 或游戏接口补发任何操作。
function observe(page, report) {
    page.on('pageerror', error => report.pageErrors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) {
            report.pageErrors.push('打造客户端未连接到独立节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40046) {
                const value = pb.ForgeQuoteRequest.decode(frame.body);
                const ref = item => ({ instance: hex(item.instance_id), version: item.version.toString() });
                report.quoteRequests.push({ request: frame.request, scene: value.scene_epoch.toString(), blueprint: ref(value.blueprint),
                    ores: value.ores.map(ref), allowBound: value.allow_bound });
            }
            if (frame?.id === 40048) {
                const value = pb.ForgeCommitRequest.decode(frame.body);
                report.commits.push({ request: frame.request, quote: hex(value.quote_id), op: hex(value.operation.op_id) });
            }
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40047) {
                const value = pb.ForgeQuote.decode(frame.body);
                report.quotes.push({ request: frame.request, quote: hex(value.quote_id), scene: value.scene_epoch.toString(),
                    blueprint: itemView(value.blueprint), ores: value.ores.map(itemView), allowBound: value.allow_bound,
                    item: value.output_item_id, quality: value.quality, slot: value.slot, requiredLevel: value.required_level,
                    chance: value.success_ppm, gold: value.gold.toString(), boundGold: value.bound_gold.toString(), bound: value.output_bound });
            }
            if (frame?.id === 40005) {
                const value = pb.OperationResult.decode(frame.body), result = value.forge;
                if (result?.output) report.results.push({ request: frame.request, op: hex(value.op_id), quote: hex(result.quote_id),
                    success: result.success, error: value.error_code, pending: value.pending, gold: result.gold.toString(),
                    boundGold: result.bound_gold.toString(), instance: hex(result.output.instance_id), item: result.output.item_id,
                    bound: result.output.bound, quality: result.output.quality, luck: result.output.initial_luck });
            }
        });
    });
}

async function rows(page, listPath) {
    return page.evaluate(listPath => cc.find(listPath)?.getComponent('List')?.items ?? [], listPath);
}

// 依原实例定位已在遮罩内的复用行，离屏时使用硬件触摸事件滑动真实列表。
async function select(page, context, listPath, instance, selectedOre = false) {
    const attempts = [];
    for (let attempt = 0; attempt < 12; attempt++) {
        const where = await page.evaluate(({ listPath, instance, selectedOre }) => {
            const list = cc.find(listPath)?.getComponent('List');
            if (!list?.node.activeInHierarchy) return null;
            const id = row => row.key ?? (row.instanceId && Array.from(row.instanceId, byte => byte.toString(16).padStart(2, '0')).join(''));
            const index = list.items.findIndex(row => typeof row === 'object' && row && id(row) === instance);
            if (index < 0) return null;
            const mask = list.viewport._uiProps.uiTransformComp.getBoundingBoxToWorld();
            const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
            const point = (x, y) => ({ x: canvas.left + x * canvas.width / size.width, y: canvas.top + (size.height - y) * canvas.height / size.height });
            const visible = [];
            let target;
            for (const [i, row] of list.rows) {
                if (!row.activeInHierarchy) continue;
                const bounds = row._uiProps.uiTransformComp.getBoundingBoxToWorld();
                const center = bounds.y + bounds.height / 2;
                if (i === index) target = { bounds, center };
                if (center < mask.y + 3 || center > mask.y + mask.height - 3) continue;
                visible.push(i);
            }
            const x = mask.x + mask.width / 2;
            // 保存详情的外层裁剪会盖住极下缘，手势从列表内部可见的行开始。
            const start = point(x, mask.y + mask.height * 0.39), finish = point(x, mask.y + mask.height * 0.85);
            const reverse = target ? target.center > mask.y + mask.height / 2 : index < Math.min(...visible);
            const movement = reverse ? { start: finish, finish: start } : { start, finish };
            let at;
            if (target && visible.includes(index)) {
                const low = Math.max(target.bounds.y, mask.y), high = Math.min(target.bounds.y + target.bounds.height, mask.y + mask.height);
                if (high - low >= target.bounds.height * (selectedOre ? 0.78 : 0.45)) {
                    at = point(target.bounds.x + target.bounds.width / 2, (low + high) / 2);
                }
            }
            return { ...movement, at, index, visible, offset: list.scrollView.getScrollOffset().y };
        }, { listPath, instance, selectedOre });
        attempts.push(where);
        if (where?.at) {
            await page.touchscreen.tap(where.at.x, where.at.y);
            if (!selectedOre) return;
            const verified = await wait(page, ({ target, id }) => cc.find(target)?.getComponent('List')?.items
                .find(row => typeof row === 'object' && row.key === id && row.label.startsWith('已选')),
            { target: listPath, id: instance }, 750).catch(() => null);
            if (verified) return verified;
        }
        if (!where) throw Error('原实例不在可用界面的列表中：' + instance);
        const cdp = await context.newCDPSession(page);
        try {
            await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [where.start] });
            for (let step = 1; step <= 8; step++) {
                const fraction = step / 8;
                const y = where.start.y + (where.finish.y - where.start.y) * fraction;
                await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: where.start.x, y }] });
                await page.waitForTimeout(35);
            }
            await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
        } finally { await cdp.detach(); }
        await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        await wait(page, target => {
            const scroll = cc.find(target)?.getComponent('List')?.scrollView;
            return scroll && !scroll.isScrolling() && !scroll.isAutoScrolling();
        }, listPath, 10000);
    }
    throw Error('真实触屏无法滚到原实例：' + instance + '，列表位置 ' + JSON.stringify(attempts));
}

async function openForge(page, context) {
    await select(page, context, inventory + '/List', expected.blueprint.instance_id);
    await label(page, details + '/Action1/Label', '打造');
    await touch(page, details + '/Action1');
    await label(page, details + '/Title', '图纸打造');
    await label(page, details + '/Action2/Label', '开启绑材');
}

async function firstDialog(page) {
    await touch(page, details + '/Action2');
    await label(page, dialog + '/Title', '使用绑定资源');
    return label(page, dialog + '/Message', '绑定图纸、黑铁矿和金币');
}

async function secondDialog(page) {
    await label(page, dialog + '/ConfirmButton/Label', '继续开启');
    await touch(page, dialog + '/ConfirmButton');
    await label(page, dialog + '/Title', '再次确认绑定投入');
    return label(page, dialog + '/Message', '失败会扣黑铁矿和金币，保留图纸');
}

async function dialogBack(page, report, name) {
    await touch(page, dialog + '/CancelButton');
    await label(page, details + '/Action2/Label', '开启绑材');
    assert.equal(report.quoteRequests.length + report.commits.length, 0);
    report.canceled.push(name);
}

async function authorize(page, capture = false) {
    const first = await firstDialog(page), second = await secondDialog(page);
    await label(page, dialog + '/ConfirmButton/Label', '确认开启');
    if (capture) await page.screenshot({ path: path.join(output, '01-forge-bind-confirm.png') });
    await touch(page, dialog + '/ConfirmButton');
    await label(page, details + '/Action2/Label', '关闭绑材');
    return [first, second];
}

async function verifyForge(page, context, session, report) {
    await openForge(page, context);
    await firstDialog(page);
    await dialogBack(page, report, 'first');
    await firstDialog(page);
    await secondDialog(page);
    await dialogBack(page, report, 'second');
    await authorize(page);
    await page.touchscreen.tap(10, 130);
    await wait(page, target => !cc.find(target)?.activeInHierarchy, details);
    await openForge(page, context);
    report.reopenedDisabled = (await rows(page, details + '/List')).some(row => typeof row === 'string' && row.includes('使用绑定资源 未开启'));
    assert(report.reopenedDisabled, '重开图纸时必须重新关闭绑定投入');
    report.bindingDialogs = await authorize(page, true);
    for (const ore of expected.ores) {
        const id = ore.instance_id;
        const selected = await select(page, context, details + '/List', id, true);
        assert(selected.label.includes(`纯度${ore.ore_purity}`) && selected.label.includes('绑定'));
        report.selected.push(id);
    }
    assert.deepEqual(report.quoteRequests, []);
    await label(page, details + '/Action1/Label', '查看报价');
    await touch(page, details + '/Action1');
    await label(page, details + '/Title', '打造报价');
    const ref = item => ({ instance: item.instance_id, version: String(item.version) });
    assert.equal(report.quoteRequests.length, 1);
    const request = report.quoteRequests[0];
    assert.deepEqual(request.blueprint, ref(expected.blueprint));
    assert.deepEqual(request.ores, expected.ores.map(ref));
    assert(request.allowBound && request.scene === report.normal.scene);
    assert.equal(report.quotes.length, 1);
    const quote = report.quotes[0];
    assert.equal(quote.request, request.request);
    assert.equal(quote.scene, request.scene);
    assert.equal(quote.blueprint.instance, expected.blueprint.instance_id);
    assert.deepEqual(quote.ores.map(item => item.instance), report.selected);
    assert.deepEqual(quote.ores.map(item => item.purity), expected.ores.map(item => item.ore_purity));
    assert(quote.allowBound && quote.bound && quote.chance === 1_000_000);
    assert.equal(quote.item, expected.output.itemId);
    assert.equal(quote.quality, expected.output.quality);
    assert.equal(quote.slot, expected.output.slot);
    assert.equal(quote.requiredLevel, expected.output.requiredLevel);
    assert.deepEqual({ gold: quote.gold, boundGold: quote.boundGold }, expected.price);
    report.quoteLines = (await rows(page, details + '/List')).join('\n');
    assert.match(report.quoteLines, /100%/);
    assert.match(report.quoteLines, /20级 稀有/);
    assert.match(report.quoteLines, /失败保留图纸/);
    assert.match(report.quoteLines, /失败照扣黑铁矿和金币/);
    await page.screenshot({ path: path.join(output, '02-forge-quote.png') });

    await label(page, details + '/Action1/Label', '确认打造');
    await touch(page, details + '/Action1');
    await label(page, dialog + '/Title', '确认打造');
    report.finalConfirm = await label(page, dialog + '/Message', '失败照扣矿石与金币，保留图纸');
    assert.match(report.finalConfirm, /100%.*产物绑定/);
    assert.equal(report.commits.length, 0);
    await touch(page, dialog + '/ConfirmButton');
    await label(page, details + '/Title', '打造成功');
    assert.equal(report.commits.length, 1);
    report.commit = report.commits[0];
    assert.equal(report.commit.quote, quote.quote);
    assert.equal(report.results.length, 1);
    const result = report.results[0];
    assert(result.success && !result.pending && result.error === 0 && result.bound && result.luck <= 3);
    assert.equal(result.request, report.commit.request);
    assert.equal(result.op, report.commit.op);
    assert.equal(result.quote, quote.quote);
    assert.equal(result.item, expected.output.itemId);
    assert.equal(result.quality, expected.output.quality);
    assert.deepEqual({ gold: result.gold, boundGold: result.boundGold }, expected.price);
    report.resultLines = (await rows(page, details + '/List')).join('\n');
    assert(report.resultLines.includes(expected.output.name) && report.resultLines.includes('装备绑定'));
    if (result.luck > 0) assert(report.resultLines.includes(`幸运+${result.luck}`));
    await page.screenshot({ path: path.join(output, '03-forge-result.png') });
    const assets = await wait(page, ({ session, instances }) => {
        const id = item => Array.from(item.instanceId, byte => byte.toString(16).padStart(2, '0')).join('');
        const wallet = session.wallet, items = session.inventory?.items;
        if (!wallet || !items) return null;
        const output = items.find(item => id(item) === instances.output), extra = items.find(item => id(item) === instances.extra);
        if (!output || !extra || items.some(item => instances.spent.includes(id(item)))) return null;
        return { source: output.sourceKind, sourceId: output.sourceId, durability: output.durabilityU.toString(),
            wallet: { gold: wallet.gold.toString(), boundGold: wallet.boundGold.toString(), yuanbao: wallet.yuanbaoUnits.toString(),
                boundYuanbao: wallet.boundYuanbaoUnits.toString() }, retained: id(extra) };
    }, { session, instances: { output: result.instance, extra: expected.extra.instance_id, spent: [expected.blueprint.instance_id, ...report.selected] } });
    assert(assets.source === 'forge_craft' && assets.sourceId === quote.quote && BigInt(assets.durability) > 0n);
    assert.deepEqual(assets.wallet, expected.walletAfter);
    report.result = { ...result, source: assets.source, sourceId: assets.sourceId, durability: assets.durability };
    report.walletAfter = assets.wallet;
    report.retained = assets.retained;
    await page.touchscreen.tap(10, 130);
    await wait(page, target => !cc.find(target)?.activeInHierarchy, details);
    await select(page, context, inventory + '/List', result.instance);
    await label(page, details + '/Title', expected.output.name);
    report.bagLines = (await rows(page, details + '/List')).join('\n');
    assert(report.bagLines.includes('来源 图纸打造') && report.bagLines.includes('绑定物品') && report.bagLines.includes('持久 7/7'));
    if (result.luck > 0) assert(report.bagLines.includes(`幸运+${result.luck}`));
    await page.screenshot({ path: path.join(output, '04-forge-bag-result.png') });
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, clientBundleSha256: expected.clientBundleSha256,
        fixtureSha256: expected.fixtureSha256, viewport: { width: 390, height: 844 }, sockets: 0, pageErrors: [], canceled: [], selected: [],
        quoteRequests: [], quotes: [], commits: [], results: [], commit: null, result: null };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: report.viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const asset = page.waitForResponse(response => response.ok() && new URL(response.url()).pathname === '/assets/main/index.js', { timeout: 65000 });
        void asset.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_FORGE_BROWSER_ACCOUNT, password: process.env.MINIMIR_FORGE_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await asset).body()).digest('hex'), expected.clientBundleSha256);
        report.normal = await originalWorld(page, session);
        assert.equal(report.normal.map, String(expected.normalMap));
        if (report.normal.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        const catalog = await page.evaluate(() => {
            const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
            return { recipes: app.world.gameplay.forge_recipes ?? [], blueprints: app.world.gameplay.items.filter(item => item.kind === 'forge_blueprint').map(item => item.item_id) };
        });
        await touch(page, safe + 'BottomHUD/Footer/Inventory');
        await wait(page, target => !!cc.find(target)?.activeInHierarchy, inventory);
        if (expected.enabled) {
            assert.equal(catalog.recipes.length, 1);
            assert.deepEqual(catalog.blueprints, [expected.blueprint.item_id]);
            await verifyForge(page, context, session, report);
        } else {
            assert.deepEqual(catalog, { recipes: [], blueprints: [] });
            assert.deepEqual(report.quoteRequests, []);
            assert.deepEqual(report.commits, []);
            await page.screenshot({ path: path.join(output, '01-default-forge-absent.png') });
        }
        assert.equal(report.sockets, 1);
        assert.deepEqual(report.pageErrors, []);
        report.passed = true;
    } catch (error) {
        report.error = error.stack || String(error);
        await page.screenshot({ path: path.join(output, 'failure.png') }).catch(() => {});
        throw error;
    } finally {
        fs.writeFileSync(path.join(output, 'forge-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, commits: report.commits.length, screenshots: output }));
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
