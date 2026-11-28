// 用 Creator 实包和手机触屏验证魂兽名录、三池选择、确认页及有符号的原回执。
'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const role = safe + 'UIMgr/Panels/Role';
const panel = safe + 'PopupView/Panels/ItemActions';
const listContent = panel + '/List/View/Content';

// 记录实包发送和原服务器回复的少量验收字段，丢弃票据和口令。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) {
            report.errors.push('连接了隔离范围之外的节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40070) return;
            const action = pb.SoulActionRequest.decode(frame.body);
            report.requests.push([action.action, action.type_id, action.affix_id, action.fragment_delta.toString()]);
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40003 && !report.wireInventory) {
                const inventory = pb.InventoryResult.decode(frame.body), collection = inventory.soul_collection;
                report.wireInventory = { revision: inventory.revision.toString(), soul: collection && {
                    revision: collection.revision.toString(), fragments: collection.fragments.toString(), total: collection.beasts.length } };
            }
            if (frame?.id !== 40005) return;
            const reply = pb.OperationResult.decode(frame.body), receipt = reply.soul;
            if (!receipt) return;
            report.receipts.push({ operation: Buffer.from(reply.op_id).toString('hex'), action: receipt.action,
                delta: receipt.fragment_delta.toString(), fragments: receipt.collection.fragments.toString(),
                revision: receipt.collection.revision.toString(), error: reply.error_code, pending: reply.pending });
        });
    });
}

// 使用保存的详情按钮进行真实触屏操作。
async function action(page, index, text) {
    await label(page, panel + '/Action' + index + '/Label', text);
    await touch(page, panel + '/Action' + index);
}

// 核对服务端库存与场景基线，而非只观察按钮的字样。
async function snapshot(page, session, rev, fragments, star, attached, damage) {
    return wait(page, ({ session, rev, fragments, star, attached, damage }) => {
        const s = session.inventory?.soulCollection;
        const player = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle);
        const stats = player?.combatAttributes;
        if (!s || !stats || s.revision.toString() !== rev || s.fragments.toString() !== fragments ||
            s.attachedTypeId !== attached || s.beasts.length !== Number(star > 0) || (s.beasts[0]?.star ?? 0) !== star ||
            stats.expandedDamageBp !== damage) return null;
        return { revision: rev, fragments, star, attached, affixes: s.beasts[0]?.affixIds ?? [],
            damage, physicalAttack: stats.physicalAttackMax.toString() };
    }, { session, rev: String(rev), fragments: String(fragments), star, attached, damage });
}

// 核对确认页的碎片变更方向、数额和标题。
async function terms(page, title, delta) {
    await label(page, panel + '/Title', title);
    const lines = await page.evaluate(node => cc.find(node + '/List').getComponent('List').items, panel);
    assert(lines.join(' ').includes('通用碎片 ' + delta), lines.join(' '));
}

async function main() {
    const report = { passed: false, fixtureSha256: expected.fixtureSha256, bundleSha256: expected.bundleSha256,
        requests: [], receipts: [], errors: [], sockets: 0 };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    try {
        const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
            deviceScaleFactor: 1, serviceWorkers: 'block' });
        const page = await context.newPage();
        const screenshot = name => page.screenshot({ path: path.join(output, name + '.png') });
        observe(page, report);
        const bundle = page.waitForResponse(response => response.status() === 200 &&
            response.url() === expected.baseUrl + '/assets/main/index.js', { timeout: 65000 });
        void bundle.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_SOUL_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_SOUL_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.bundleSha256);
        assert.equal((await originalWorld(page, session)).role, expected.role);
        report.observed = await page.evaluate(session => {
            const s = session.inventory?.soulCollection;
            const player = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle);
            return { haveCollection: !!s, revision: s?.revision?.toString(), fragments: s?.fragments?.toString(), attached: s?.attachedTypeId,
                total: s?.beasts?.length, player: !!player, damage: player?.combatAttributes?.expandedDamageBp };
        }, session);
        report.before = await snapshot(page, session, 0, 30, 0, 0, 0);
        await touch(page, safe + 'BottomHUD/QuickNav/Role');
        await touch(page, role + '/Message');
        await label(page, panel + '/Title', '炼体');
        await action(page, 3, '魂兽');
        await label(page, panel + '/Title', '魂兽名录');
        report.catalogSize = await page.evaluate(node => cc.find(node + '/List').getComponent('List').items
            .filter(row => row.label?.includes('未收录')).length, panel);
        assert.equal(report.catalogSize, 13);
        await screenshot('catalog');
        await touch(page, listContent, '骷髅精灵');
        await action(page, 1, '合成');
        await terms(page, '确认合成', '消耗 12');
        assert.deepEqual(report.requests, []);
        await action(page, 1, '确认');
        await snapshot(page, session, 1, 18, 1, 0, 0);
        await action(page, 1, '附体');
        report.attached = await snapshot(page, session, 2, 18, 1, 1, 25);
        await action(page, 3, '配置与分解');
        await label(page, panel + '/Title', '配置魂兽');
        assert.equal(await page.evaluate(node => cc.find(node + '/Action2').getComponent(cc.Button).interactable, panel), false);
        await touch(page, listContent, '伤害 · 攻击');
        await action(page, 1, '保存词条');
        report.selected = await snapshot(page, session, 3, 18, 1, 1, 0);
        assert.deepEqual(report.selected.affixes, [1, 8, 18]);
        assert.equal(BigInt(report.selected.physicalAttack), BigInt(report.before.physicalAttack) + 1n);
        await screenshot('affixes');
        await action(page, 3, '返回魂兽');
        await action(page, 2, '升星');
        await terms(page, '确认升星', '消耗 6');
        await screenshot('upgrade');
        await action(page, 1, '确认');
        await snapshot(page, session, 4, 12, 2, 1, 0);
        await action(page, 1, '解除附体');
        await snapshot(page, session, 5, 12, 2, 0, 0);
        await action(page, 3, '配置与分解');
        await action(page, 2, '分解');
        await terms(page, '确认分解', '获得 7');
        await screenshot('salvage');
        await action(page, 1, '确认');
        report.after = await snapshot(page, session, 6, 19, 0, 0, 0);
        assert.equal(report.after.physicalAttack, report.before.physicalAttack);
        await label(page, panel + '/Title', '魂兽名录');
        assert.deepEqual(report.requests, [[6, 1, 0, '-12'], [1, 1, 0, '0'], [3, 1, 1, '0'],
            [4, 1, 0, '-6'], [2, 1, 0, '0'], [5, 1, 0, '7']]);
        assert.deepEqual(report.receipts.map(r => [r.action, r.delta, r.fragments, r.revision, r.error, r.pending]),
            [[6, '-12', '18', '1', 0, false], [1, '0', '18', '2', 0, false], [3, '0', '18', '3', 0, false],
                [4, '-6', '12', '4', 0, false], [2, '0', '12', '5', 0, false], [5, '7', '19', '6', 0, false]]);
        assert.deepEqual(report.errors, []);
        assert.equal(report.sockets, 1);
        report.passed = true;
    } finally {
        fs.writeFileSync(path.join(output, 'soul-beast-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
