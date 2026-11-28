// 触屏核对真实战令实包、原现金差额和原绑定奖项。
'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const functions = safe + 'UIMgr/Panels/Functions';
const entry = functions + '/Pages/Page1/BattlePass';
const panel = safe + 'PopupView/Panels/ItemActions';
const content = panel + '/List/View/Content';

// observe 记录真正走到游戏服的原领取请求和返回的绑定差额。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) {
            report.errors.push('页面连接了验收范围外的节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40076) return;
            const claim = pb.BattlePassClaimRequest.decode(frame.body);
            report.claims.push({ operation: Buffer.from(claim.operation.op_id).toString('hex'), day: claim.day,
                tier: claim.tier, previous: claim.claimed_tier });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40005) return;
            const operation = pb.OperationResult.decode(frame.body), result = operation.battle_pass;
            if (!result) return;
            report.operation = Buffer.from(operation.op_id).toString('hex');
            report.receipt = { day: result.day, tier: result.tier, previous: result.previous_tier,
                gold: result.bound_gold.toString(), yuanbao: result.bound_yuanbao_units.toString(),
                items: result.items.map(item => [item.item_id, item.quantity.toString(), item.bound, item.source_kind]) };
        });
    });
}

// action 使用 Creator 保存的原按钮执行触屏操作。
async function action(page, i, text) {
    await label(page, panel + `/Action${i}/Label`, text);
    await touch(page, panel + `/Action${i}`);
}

// rowText 读取列表当前显示的原价格、材料和状态。
async function rowText(page, text) {
    return wait(page, ({ panel, text }) => cc.find(panel + '/List').getComponent('List').items
        .map(row => typeof row === 'string' ? row : row.label).join(' ').includes(text), { panel, text });
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, bundleSha256: expected.bundleSha256, fixtureSha256: expected.fixtureSha256,
        sockets: 0, errors: [], claims: [], operation: '' };
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
        const session = await login(page, 'role', { account: process.env.MINIMIR_BATTLE_PASS_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_BATTLE_PASS_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.bundleSha256);
        const world = await originalWorld(page, session);
        assert.equal(world.role, expected.role);
        if (world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, model => model.hosting?.autoBattleEnabled === false, session);
        }
        await touch(page, safe + 'BottomHUD/Footer/Functions');
        await touch(page, functions + '/Tabs/Tab1');
        if (expected.enabled) {
            await label(page, entry + '/Label', '战令');
            await touch(page, entry);
            await rowText(page, '当前28 元档');
            await action(page, 2, '档位与奖励');
            await touch(page, content, '98 元档');
            await label(page, panel + '/Title', '98 元档');
            await rowText(page, '当前升档差额 70.00 元');
            await page.screenshot({ path: path.join(output, 'upgrade.png') });
            await action(page, 1, '返回档位');
            await action(page, 1, '返回战令');
            await touch(page, content, '今日 · 可补差额');
            await label(page, panel + '/Title', `第 ${expected.day} 天奖励`);
            await rowText(page, '本次领取差额');
            await rowText(page, '绑定金币 3000');
            await page.screenshot({ path: path.join(output, 'claim.png') });
            assert.deepEqual(report.claims, []);
            await action(page, 1, '领取奖励');
            await rowText(page, '今日 · 已领');
            assert.deepEqual(report.claims, [{ operation: report.operation, day: expected.day, tier: 2, previous: 1 }]);
            assert.deepEqual(report.receipt, { day: expected.day, tier: 2, previous: 1, gold: '3000', yuanbao: '1000',
                items: [[900001, '8', true, 'battle_pass'], [900021, '1', true, 'battle_pass'], [900041, '3', true, 'battle_pass'],
                    [900051, '5', true, 'battle_pass']] });
            await action(page, 3, '历史周期');
            await label(page, panel + '/Title', '历史战令');
            await page.screenshot({ path: path.join(output, 'history.png') });
        } else {
            await wait(page, node => cc.find(node)?.parent?.activeInHierarchy, entry);
            report.defaultEntryHidden = await page.evaluate(node => cc.find(node)?.activeInHierarchy === false, entry);
            assert(report.defaultEntryHidden);
            assert.deepEqual(report.claims, []);
            await page.screenshot({ path: path.join(output, 'default.png') });
        }
        assert.deepEqual(report.errors, []);
        assert.equal(report.sockets, 1);
        report.passed = true;
    } finally {
        fs.writeFileSync(path.join(output, 'battle-pass-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
