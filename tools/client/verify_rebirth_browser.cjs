// 用 Creator Web Mobile 实包和触屏操作验收角色成长入口、报价、确认与库存刷新。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const role = safe + 'UIMgr/Panels/Role';
const sheet = safe + 'PopupView/Panels/ItemActions';
const hex = value => Buffer.from(value).toString('hex');

// 记录同一账号在原 WebSocket 上收到的真实报价和操作结果。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) {
            report.errors.push('触屏连接超出本次隔离游戏节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40064) report.requests.push({ id: frame.id, request: frame.request });
            if (frame?.id === 40066) {
                const value = pb.RebirthCommitRequest.decode(frame.body);
                report.requests.push({ id: frame.id, request: frame.request, op: hex(value.operation.op_id), quoteId: hex(value.quote_id),
                    revision: value.operation.expected_revision.toString() });
            }
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40065) {
                const value = pb.RebirthQuote.decode(frame.body);
                report.quote = { request: frame.request, quoteId: hex(value.preview.quote_id), revision: value.role_revision.toString() };
            }
            if (frame?.id === 40005) {
                const value = pb.OperationResult.decode(frame.body);
                if (!value.rebirth) return;
                report.receipt = { request: frame.request, op: hex(value.op_id), quoteId: hex(value.rebirth.quote_id),
                    challengeId: hex(value.rebirth.challenge_id), cap: value.rebirth.transition.new_cap,
                    level: value.rebirth.transition.new_level, experience: value.rebirth.transition.released_experience.toString(),
                    gold: value.rebirth.gold.toString(), boundGold: value.rebirth.bound_gold.toString(),
                    pending: value.pending, error: value.error_code };
            }
        });
    });
}

const lines = page => page.evaluate(panel => cc.find(panel + '/List')?.getComponent('List')?.items ?? [], sheet);
const screenshot = (page, name) => page.screenshot({ path: path.join(output, name + '.png') });

// 确认新等级、经验池、材料、凭证和钱包同时更新。
async function candidate(page, session, report) {
    await label(page, sheet + '/Title', '角色成长');
    assert((await lines(page)).some(line => String(line).includes('林地守卫')));
    await label(page, sheet + '/Action1/Label', '查看转生报价');
    await touch(page, sheet + '/Action1');
    await label(page, sheet + '/Title', '转生报价');
    const displayed = (await lines(page)).join(' ');
    for (const value of ['50 → 51', '75000000', '绑定金币 600', '金币 400', '绑定材料 2']) assert(displayed.includes(value), displayed);
    await screenshot(page, '01-rebirth-quote');
    await label(page, sheet + '/Action1/Label', '确认转生');
    await touch(page, sheet + '/Action1');
    await label(page, sheet + '/Title', '转生完成');
    report.inventory = await wait(page, ({ session, proof, material }) => {
        const inv = session.inventory, wallet = session.wallet;
        if (!inv || !wallet || wallet.revision.lessThan(inv.revision) || inv.progression?.rebirthCount !== 1) return null;
        const row = inv.items.find(item => item.itemId === material);
        return { count: inv.progression.rebirthCount, level: inv.level, cap: inv.progression.levelCap,
            pending: inv.progression.pendingExperience.toString(), gold: wallet.gold.toString(), boundGold: wallet.boundGold.toString(),
            proof: inv.items.some(item => item.itemId === proof), material: row?.quantity.toString() };
    }, { session, proof: expected.credentialItemId, material: expected.materialItemId });
    assert.deepEqual(report.inventory, { count: 1, level: 51, cap: 51, pending: '0', gold: '800', boundGold: '0', proof: false, material: '1' });
    const [quoted, committed] = report.requests;
    assert.equal(report.requests.length, 2);
    assert.equal(quoted.request, report.quote.request);
    assert.deepEqual([report.receipt.request, report.receipt.op, report.receipt.quoteId, report.receipt.challengeId],
        [committed.request, committed.op, report.quote.quoteId, expected.challengeId]);
    assert.deepEqual([report.receipt.cap, report.receipt.level, report.receipt.experience, report.receipt.gold, report.receipt.boundGold,
        report.receipt.error, report.receipt.pending], [51, 51, '75000000', '400', '600', 0, false]);
    await screenshot(page, '02-rebirth-receipt');
    await label(page, sheet + '/Action1/Label', '查看当前成长');
    await touch(page, sheet + '/Action1');
    assert((await lines(page)).includes('已完成当前开放档位'));
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, fixtureSha256: expected.fixtureSha256,
        clientBundleSha256: expected.clientBundleSha256, requests: [], errors: [], sockets: 0 };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
        deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const bundle = page.waitForResponse(response => response.status() === 200 &&
            response.url() === expected.baseUrl + '/assets/main/index.js', { timeout: 65_000 });
        void bundle.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_REBIRTH_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_REBIRTH_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.clientBundleSha256);
        const world = await originalWorld(page, session);
        assert.equal(world.role, expected.role);
        if (world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, model => model.hosting?.autoBattleEnabled === false, session);
        }
        await touch(page, safe + 'BottomHUD/QuickNav/Role');
        await touch(page, role + '/Message');
        if (expected.enabled) await candidate(page, session, report);
        else {
            const detail = safe + 'PopupView/Panels/StatusDetails';
            await wait(page, node => cc.find(node)?.activeInHierarchy, detail);
            assert.equal(await page.evaluate(node => cc.find(node)?.activeInHierarchy, sheet), false);
            assert.deepEqual(report.requests, []);
            await screenshot(page, '01-rebirth-closed');
        }
        assert.deepEqual(report.errors, []);
        assert(report.sockets >= 1);
        report.passed = true;
    } finally {
        fs.writeFileSync(path.join(output, 'rebirth-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
