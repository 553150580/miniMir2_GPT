// 使用 Creator 实包触屏读取两系的权威报价、原回执和战斗上限。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const role = safe + 'UIMgr/Panels/Role';
const panel = safe + 'PopupView/Panels/ItemActions';
const toHex = value => Buffer.from(value).toString('hex');
const screenshot = (page, name) => page.screenshot({ path: path.join(output, name + '.png') });

// 只记录本轮隔离角色的原经济响应。
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
            if (frame?.id === 40067 || frame?.id === 40069) report.requests.push(frame.id);
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40005) return;
            const result = pb.OperationResult.decode(frame.body), receipt = result.body_training;
            if (!receipt) return;
            report.receipts.push({ operation: toHex(result.op_id), quote: toHex(receipt.quote_id), error: result.error_code,
                pending: result.pending, track: receipt.terms.track, after: receipt.after_level, success: receipt.success,
                gold: receipt.terms.gold.toString(), boundGold: receipt.terms.bound_gold.toString() });
        });
    });
}

async function observeRole(page, session, health, mana, hp, mp) {
    return wait(page, ({ session, health, mana, hp, mp }) => {
        const inventory = session.inventory, wallet = session.wallet;
        const player = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle);
        if (!inventory?.bodyTraining || !wallet || wallet.revision.lessThan(inventory.revision) || !player ||
            inventory.bodyTraining.healthLevel !== health || inventory.bodyTraining.manaLevel !== mana ||
            hp !== undefined && player.hpMax.toString() !== hp || mp !== undefined && player.mpMax.toString() !== mp) return null;
        return { health, mana, hp: player.hpMax.toString(), mp: player.mpMax.toString(),
            gold: wallet.gold.toString(), boundGold: wallet.boundGold.toString() };
    }, { session, health, mana, hp, mp });
}

async function action(page, index, text) {
    await label(page, panel + '/Action' + index + '/Label', text);
    await touch(page, panel + '/Action' + index);
}

async function firstRank(page, index, name, bound, gold) {
    await action(page, index, name + '炼体');
    await label(page, panel + '/Title', '炼体报价');
    const lines = await page.evaluate(base => cc.find(base + '/List').getComponent('List').items, panel);
    const display = lines.join(' ');
    for (const line of [name + ' 0 → 1级', '成功率 100%', '绑定金币 ' + bound, '普通金币 ' + gold]) {
        assert(display.includes(line), display);
    }
    await screenshot(page, 'quote-' + index);
    await action(page, 1, '确认炼体');
    await label(page, panel + '/Title', '炼体成功');
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, bundleSha256: expected.bundleSha256,
        fixtureSha256: expected.fixtureSha256, sockets: 0, errors: [], requests: [], receipts: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
        deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const bundle = page.waitForResponse(response => response.status() === 200 &&
            response.url() === expected.baseUrl + '/assets/main/index.js', { timeout: 65000 });
        void bundle.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_BODY_TRAINING_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_BODY_TRAINING_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.bundleSha256);
        const world = await originalWorld(page, session);
        assert.equal(world.role, expected.role);
        if (world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, model => model.hosting?.autoBattleEnabled === false, session);
        }
        await touch(page, safe + 'BottomHUD/QuickNav/Role');
        await touch(page, role + '/Message');
        if (expected.enabled) {
            await label(page, panel + '/Title', '炼体');
            report.before = await observeRole(page, session, 0, 0);
            const hp = (BigInt(report.before.hp) + 2n).toString(), mp = (BigInt(report.before.mp) + 1n).toString();
            await firstRank(page, 1, '生命上限', '1000', '500');
            report.health = await observeRole(page, session, 1, 0, hp, report.before.mp);
            await action(page, 1, '返回炼体');
            await firstRank(page, 2, '魔法上限', '0', '1500');
            report.after = await observeRole(page, session, 1, 1, hp, mp);
            assert.deepEqual([report.after.gold, report.after.boundGold], ['3000', '0']);
            assert.deepEqual(report.requests, [40067, 40069, 40067, 40069]);
            assert.deepEqual(report.receipts.map(r => [r.track, r.success, r.after, r.error, r.pending]), [[3, true, 1, 0, false], [4, true, 1, 0, false]]);
            await screenshot(page, 'trained');
            await action(page, 1, '返回炼体');
            await action(page, 3, '查看转生');
            await label(page, panel + '/Title', '角色成长');
        } else {
            await wait(page, node => cc.find(node)?.activeInHierarchy, safe + 'PopupView/Panels/StatusDetails');
            assert.equal(await page.evaluate(node => cc.find(node)?.activeInHierarchy, panel), false);
            assert.deepEqual(report.requests, []);
            await screenshot(page, 'default-growth');
        }
        assert.deepEqual(report.errors, []);
        assert(report.sockets >= 1);
        report.passed = true;
    } finally {
        fs.writeFileSync(path.join(output, 'body-training-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
