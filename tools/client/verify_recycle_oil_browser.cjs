// 使用真实手机触屏打开固定背包、确认分解与查看祝福油，并只读记录 WebSocket 和界面可见状态。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const inventory = safe + 'UIMgr/Panels/Inventory';
const details = safe + 'PopupView/Panels/ItemActions';
const confirmation = safe + 'PopupView/Panels/Confirm';
const hex = value => Buffer.from(value ?? []).toString('hex');

// 保留原分解实例、操作和连接，按真实服务器回执解码原油及绑定。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href || endpoint.port === '18080') {
            report.errors.push('触屏回归没有连接本次隔离节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40002) return;
            const value = pb.RecycleRequest.decode(frame.body);
            report.requests.push({ request: frame.request, binding: frame.binding, op: hex(value.operation?.op_id),
                seq: value.operation?.command_seq.toString(), instances: value.item_instance_ids.map(hex) });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40005) return;
            const value = pb.OperationResult.decode(frame.body);
            report.results.push({ request: frame.request, binding: frame.binding, op: hex(value.op_id), pending: value.pending,
                error: value.error_code, revision: value.new_revision.toString(), seq: value.command_seq.toString(),
                rolls: (value.recycle_oil?.rolls ?? []).map(roll => ({ instance: hex(roll.equipment_instance_id), luck: roll.luck,
                    ...(roll.oil ? { oil: hex(roll.oil.instance_id), bound: roll.oil.bound, itemId: roll.oil.item_id, source: roll.oil.source_kind,
                        sourceId: roll.oil.source_id, quantity: roll.oil.quantity.toString(), version: roll.oil.version.toString() } : {}) })) });
        });
    });
}

// 点中原背包格真正位于可视遮罩内的渲染节点，不调用游戏按钮回调。
async function openItem(page, item) {
    const point = await wait(page, ({ inventory, instance }) => {
        const list = cc.find(inventory + '/List')?.getComponent('List');
        if (!list?.node.activeInHierarchy) return null;
        const id = row => Array.from(row.instanceId ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
        const index = list.items.findIndex(row => id(row) === instance);
        const node = list.rows.get(index);
        if (!node?.activeInHierarchy) return null;
        const row = node._uiProps.uiTransformComp.getBoundingBoxToWorld(), mask = list.viewport._uiProps.uiTransformComp.getBoundingBoxToWorld();
        const x = row.x + row.width / 2, y = row.y + row.height / 2;
        if (x < mask.x || x > mask.x + mask.width || y < mask.y || y > mask.y + mask.height) return null;
        const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
        return { x: canvas.left + x * canvas.width / size.width, y: canvas.top + (size.height - y) * canvas.height / size.height };
    }, { inventory, instance: item.instance_id });
    await page.touchscreen.tap(point.x, point.y);
    await label(page, details + '/Title', expected.weaponName);
    await label(page, details + '/Action2/Label', '分解装备');
}

async function rows(page) {
    return page.evaluate(details => cc.find(details + '/List').getComponent('List').items, details);
}

async function dismiss(page) {
    await page.touchscreen.tap(10, 130);
    await wait(page, details => !cc.find(details)?.activeInHierarchy, details);
}

async function confirmRecycle(page) {
    await touch(page, details + '/Action2');
    await label(page, confirmation + '/Title', '确认');
    return label(page, confirmation + '/Message', '分解' + expected.weaponName);
}

async function balance(page, session) {
    return page.evaluate(session => {
        const ids = session.inventory.items.map(item => Array.from(item.instanceId, byte => byte.toString(16).padStart(2, '0')).join(''));
        return { ids, capacity: session.inventory.capacity, rev: session.inventory.revision.toString(),
            gold: session.wallet?.gold?.toString(), boundGold: session.wallet?.boundGold?.toString() };
    }, session);
}

// 先看 +3 绑定原件并取消。默认规则也必须保有原分解动作及取消语义。
async function previewAndCancel(page, session, report) {
    report.original = await balance(page, session);
    assert.equal(report.original.ids[0], expected.preview.instance_id);
    assert.equal(report.original.capacity, report.original.ids.length, '原隔离背包应满格');
    await openItem(page, expected.preview);
    const preview = await rows(page);
    const displaysOil = preview.some(row => /^分解出油概率 /.test(String(row)));
    assert.equal(displaysOil, expected.enabled);
    report.previewLines = preview;
    if (expected.enabled) {
        assert(preview.includes('分解出油概率 10%') && preview.includes('命中得1瓶绑定祝福油'));
        await page.screenshot({ path: path.join(output, '01-preview.png') });
    }
    const message = await confirmRecycle(page);
    report.confirmation = message;
    if (expected.enabled) {
        assert(message.includes('当前幸运+3 · 绑定') && message.includes('分解出油概率 10%') &&
            message.includes('命中得1瓶绑定祝福油') && message.includes('产物继承原装备绑定'));
        await page.screenshot({ path: path.join(output, '02-confirm.png') });
    } else {
        assert(message.includes('分解所得继承装备绑定状态') && !message.includes('祝福油'));
        report.defaultPreviewHidden = true;
        await page.screenshot({ path: path.join(output, '01-default-confirm.png') });
    }
    await touch(page, confirmation + '/CancelButton');
    await label(page, details + '/Title', expected.weaponName);
    assert.deepEqual(await balance(page, session), report.original, '取消预览后不应改原库存和钱包');
    assert.deepEqual(report.requests, [], '取消前后都不能提交 40002');
    report.cancelled = true;
    await dismiss(page);
}

// 只按真实每件 30% 提交，未中时返回背包，首次命中后点开同一瓶已入包的油。
async function recycleToFirstAward(page, session, report) {
    assert.equal(expected.attempts.length, 24);
    for (const candidate of expected.attempts) {
        await openItem(page, candidate);
        assert((await rows(page)).includes('分解出油概率 30%'));
        const message = await confirmRecycle(page);
        assert(message.includes('当前幸运+9 · 绑定') && message.includes('分解出油概率 30%'));
        const index = report.attempts.length;
        assert.equal(report.requests.length, index, '确认前不应发送原分解');
        await touch(page, confirmation + '/ConfirmButton');
        await label(page, details + '/Title', '装备分解结果');
        assert.equal(report.requests.length, index + 1);
        const request = report.requests[index];
        const result = report.results.find(reply => reply.request === request.request && reply.op === request.op);
        assert(result, '缺少与本次真实触屏分解对应的已确认回执');
        assert.deepEqual([request.instances, request.binding, result.binding, result.error, result.pending, result.seq],
            [[candidate.instance_id], report.world.binding, request.binding, 0, false, request.seq]);
        assert.match(request.op, /^[0-9a-f]{32}$/);
        assert.equal(result.rolls.length, 1);
        const [roll] = result.rolls, resultLines = await rows(page);
        assert.deepEqual([roll.instance, roll.luck], [candidate.instance_id, 9]);
        assert(resultLines.includes('原装备尾号 ' + candidate.instance_id.slice(-8)) && resultLines.includes('分解幸运+9'));
        await wait(page, ({ session, original, revision }) => session.inventory.revision.toString() === revision &&
            !session.inventory.items.some(item => Array.from(item.instanceId, byte => byte.toString(16).padStart(2, '0')).join('') === original),
        { session, original: candidate.instance_id, revision: result.revision });
        report.attempts.push({ op: request.op, instance: roll.instance, luck: roll.luck, oil: roll.oil ?? '', resultLines });
        if (!roll.oil) {
            assert(resultLines.includes('本件未获得祝福油'));
            await dismiss(page);
            continue;
        }
        assert(resultLines.includes('获得1瓶绑定祝福油') && resultLines.includes('来源 装备分解'));
        assert.deepEqual([roll.bound, roll.source, roll.sourceId, roll.quantity, roll.version], [true, 'equipment_recycle', candidate.instance_id, '1', '1']);
        await label(page, details + '/Action1/Label', '查看祝福油');
        await page.screenshot({ path: path.join(output, '03-result.png') });
        await touch(page, details + '/Action1');
        await label(page, details + '/Title', '祝福油');
        const oilDetail = await rows(page);
        assert(oilDetail.includes('数量 1') && oilDetail.includes('绑定物品') && oilDetail.includes('来源 装备分解'));
        const state = await balance(page, session);
        const remaining = [expected.preview.instance_id, ...expected.attempts.slice(index + 1).map(item => item.instance_id), roll.oil];
        assert.deepEqual(state.ids, remaining);
        assert.equal(report.requests.length, report.attempts.length, '查看原油只读，不应发送分解');
        report.openedOil = { instance: roll.oil, source: roll.sourceId, detail: oilDetail };
        report.remaining = state;
        await page.screenshot({ path: path.join(output, '04-oil.png') });
        return;
    }
    assert.fail('真实分母30独立尝试24次均未命中，已保留每次实际结果');
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, fixtureSha256: expected.fixtureSha256, clientBundleSha256: expected.clientBundleSha256,
        viewport: { width: 390, height: 844 }, sockets: 0, errors: [], attempts: [], requests: [], results: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: report.viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const asset = page.waitForResponse(response => response.ok() && new URL(response.url()).pathname === '/assets/main/index.js', { timeout: 65000 });
        void asset.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_RECYCLE_BROWSER_ACCOUNT, password: process.env.MINIMIR_RECYCLE_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await asset).body()).digest('hex'), expected.clientBundleSha256);
        report.world = await originalWorld(page, session);
        assert.equal(report.world.map, String(expected.normalMap));
        assert.equal(report.world.rule, String(expected.ruleVersion));
        if (report.world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        await touch(page, safe + 'BottomHUD/Footer/Inventory');
        await previewAndCancel(page, session, report);
        if (expected.enabled) await recycleToFirstAward(page, session, report);
        assert.equal(report.sockets, 1);
        assert.deepEqual(report.errors, []);
        report.passed = true;
    } catch (error) {
        report.error = error.stack || String(error);
        await page.screenshot({ path: path.join(output, 'failure.png') }).catch(() => {});
        throw error;
    } finally {
        fs.writeFileSync(path.join(output, 'recycle-oil-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, cancelled: report.cancelled,
        defaultPreviewHidden: report.defaultPreviewHidden, attempts: report.attempts.length, remaining: report.remaining?.ids.length, screenshots: output }));
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
