// 本文件核对默认入口隐藏、触屏观看取消、续播和真实服务端托管到账。
'use strict';

const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { writeFileSync } = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const hosting = safe + 'UIMgr/Panels/Hosting';
const entry = hosting + '/RewardedAd';
const panel = safe + 'PopupView/Panels/ItemActions';
const credit = expected.startingSeconds + 3600;

// rowText 读取玩家实际看到的详情文字。
async function rowText(page, text) {
    return wait(page, ({ panel, text }) => cc.find(panel + '/List').getComponent('List').items
        .map(row => typeof row === 'string' ? row : row.label).join(' ').includes(text), { panel, text });
}

// action 必须经过已保存按钮的实际触屏坐标。
async function action(page, text) {
    await label(page, panel + '/Action1/Label', text);
    await touch(page, panel + '/Action1');
}

// connectEmulator 只在验收的 Node 宿主保存令牌，网页只能请求正常播放。
async function connectEmulator(page, report) {
    const emulator = new URL(process.env.MINIMIR_REWARDED_AD_TOUCH_EMULATOR);
    assert(emulator.protocol === 'http:' && emulator.hostname === '127.0.0.1' && emulator.port !== '18080');
    await page.exposeFunction('completeIsolatedVideo', async terms => {
        assert.equal(terms.provider, 'isolated-video');
        assert.equal(terms.placement, 'hosting-hour');
        assert.match(terms.attemptId, /^[a-f0-9]{32}$/);
        assert.match(terms.beneficiary, /^[a-f0-9]{64}$/);
        report.playbacks.push(terms.attemptId);
        if (report.playbacks.length === 1) throw new Error('本次触屏模拟提前关闭');
        const response = await fetch(emulator, {
            method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Isolation-Proof': process.env.MINIMIR_REWARDED_AD_TOUCH_KEY },
            body: JSON.stringify(terms),
        });
        assert.equal(response.status, 204);
    });
    await page.addInitScript(() => {
        globalThis.minimirRewardedAd = { play: terms => globalThis.completeIsolatedVideo(terms) };
    });
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, bundleSha256: expected.bundleSha256,
        fixtureSha256: expected.fixtureSha256, sockets: 0, errors: [], playbacks: [], grantFrames: 0, attempt: '' };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    try {
        const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
            deviceScaleFactor: 1, serviceWorkers: 'block' });
        const page = await context.newPage();
        page.on('pageerror', error => report.errors.push(error.message));
        page.on('websocket', socket => {
            if (socket.url() !== endpoint.href) { report.errors.push('浏览器连接到了验收范围外的游戏节点'); return; }
            report.sockets++;
            socket.on('framereceived', ({ payload }) => {
                const frame = packet(payload);
                if (frame?.id === 30005 && frame.request === '0' && pb.HostingState.decode(frame.body).remain_hosting_sec === credit) {
                    report.grantFrames++;
                }
            });
        });
        if (expected.enabled) await connectEmulator(page, report);
        const bundle = page.waitForResponse(response => response.status() === 200 &&
            response.url() === expected.baseUrl + '/assets/main/index.js', { timeout: 65000 });
        void bundle.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_REWARDED_AD_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_REWARDED_AD_BROWSER_PASSWORD });
        assert.equal(createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.bundleSha256);
        let world = await originalWorld(page, session);
        assert.equal(world.role, expected.role);
        if (world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, model => model.hosting?.autoBattleEnabled === false, session);
            world = await originalWorld(page, session);
        }
        await touch(page, safe + 'BottomHUD/QuickNav/Hosting');
        await wait(page, node => cc.find(node)?.activeInHierarchy, hosting);
        if (expected.enabled) {
            await label(page, entry + '/Label', '激励视频');
            await page.screenshot({ path: path.join(output, 'hosting.png') });
            await touch(page, entry);
            await label(page, panel + '/Title', '激励视频');
            await rowText(page, '每次核验奖励 1 小时');
            await rowText(page, '账号今日剩余 2/2');
            await action(page, '观看视频');
            await rowText(page, '播放尚未确认');
            assert.equal(await session.evaluate(model => model.hosting?.remainHostingSec), expected.startingSeconds);
            assert.equal(report.grantFrames, 0);
            await action(page, '继续播放');
            await rowText(page, '本次到账 1 小时');
            await rowText(page, '账号今日剩余 1/2');
            await rowText(page, '到账后余额 25 小时 5 分钟');
            await wait(page, ({ model, credit }) => model.hosting?.remainHostingSec === credit, { model: session, credit });
            assert.equal(report.playbacks.length, 2);
            assert.equal(report.playbacks[0], report.playbacks[1]);
            [report.attempt] = report.playbacks;
            await page.screenshot({ path: path.join(output, 'reward.png') });
            assert.equal(report.grantFrames, 1);
            assert.deepEqual(await originalWorld(page, session), world);
        } else {
            assert(await page.evaluate(node => cc.find(node)?.activeInHierarchy === false, entry));
            assert.equal(await session.evaluate(model => model.hosting?.remainHostingSec), expected.startingSeconds);
            assert.equal(report.grantFrames, 0);
            report.defaultEntryHidden = true;
            await page.screenshot({ path: path.join(output, 'default.png') });
        }
        assert.deepEqual(report.errors, []);
        assert.equal(report.sockets, 1);
        report.passed = true;
    } finally {
        writeFileSync(path.join(output, 'rewarded-ad-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
