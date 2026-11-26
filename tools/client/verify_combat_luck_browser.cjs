// 在独立实包中触屏打开战斗属性和穿戴原项链，只读观察场景与真实 WebSocket 消息。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const profile = safe + 'UIMgr/Panels/Role';
const inventory = safe + 'UIMgr/Panels/Inventory';
const status = safe + 'PopupView/Panels/StatusDetails';
const details = safe + 'PopupView/Panels/ItemActions';
const confirm = safe + 'PopupView/Panels/Confirm';
const hex = value => Buffer.from(value ?? []).toString('hex');

// 记录原游戏节点发回的正式操作回执，按请求和绑定验明同一笔操作。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) {
            report.errors.push('幸运客户端未连接到隔离游戏节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40001) return;
            const value = pb.EquipRequest.decode(frame.body);
            report.requests.push({ id: frame.id, request: frame.request, binding: frame.binding, instance: hex(value.item_instance_id),
                slot: value.slot, version: value.item_version.toString(), unequip: value.unequip, op: hex(value.operation.op_id) });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40005) return;
            const value = pb.OperationResult.decode(frame.body);
            report.results.push({ id: frame.id, request: frame.request, binding: frame.binding, op: hex(value.op_id),
                revision: value.new_revision.toString(), error: value.error_code, pending: value.pending });
        });
    });
}

// 从保存的角色面板进入详情，并等待常规场景同步的新属性及对应文本。
async function readStatus(page, session, attackPPM) {
    const enabled = attackPPM !== null;
    await touch(page, safe + 'BottomHUD/QuickNav/Role');
    await touch(page, profile + '/Recycle');
    await label(page, status + '/Title', '战斗属性');
    const result = await wait(page, ({ session, status, attackPPM, enabled }) => {
        const attrs = session.baseline?.entities.find(entity => entity.entityHandle === session.entry?.playerHandle)?.combatAttributes;
        const lines = cc.find(status + '/List')?.getComponent('List')?.items;
        if (!attrs || !lines?.some(line => line.startsWith('物理攻击 '))) return null;
        const current = { enabled: attrs.luckEnabled === true, attackPPM: attrs.luckAttackPpm ?? 0, resistancePPM: attrs.luckResistancePpm ?? 0 };
        if (enabled && (current.enabled !== true || current.attackPPM !== attackPPM || current.resistancePPM !== 200000)) return null;
        if (enabled && !lines.includes(attackPPM ? '幸运一击 4.45%' : '幸运一击 0%')) return null;
        return { ...current, allLines: lines, lines: lines.filter(line => /^幸运(?:一击|抵抗) /.test(line)) };
    }, { session, status, attackPPM, enabled });
    assert.equal(result.enabled, enabled);
    assert.deepEqual(result.lines, enabled ? [attackPPM ? '幸运一击 4.45%' : '幸运一击 0%', '幸运抵抗 20%'] : []);
    if (!enabled) assert.deepEqual([result.attackPPM, result.resistancePPM], [0, 0]);
    return result;
}

// 将幸运行实际滚入保存列表的遮罩，确保截图展示生成后的可见文字。
async function showLuckRows(page, context, wanted) {
    let touchInput;
    for (let attempt = 0; attempt < 5; attempt++) {
        const where = await page.evaluate(({ status, wanted }) => {
            const list = cc.find(status + '/List').getComponent('List');
            const mask = list.viewport._uiProps.uiTransformComp.getBoundingBoxToWorld();
            const visible = wanted.every(text => {
                const node = list.rows.get(list.items.indexOf(text));
                if (!node?.activeInHierarchy || node.getChildByName('Text')?.getComponent(cc.Label)?.string !== text) return false;
                const box = node._uiProps.uiTransformComp.getBoundingBoxToWorld();
                return box.y >= mask.y - 1 && box.y + box.height <= mask.y + mask.height + 1;
            });
            const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
            const point = y => ({ x: canvas.left + (mask.x + mask.width / 2) * canvas.width / size.width,
                y: canvas.top + (size.height - y) * canvas.height / size.height });
            return { visible, start: point(mask.y + mask.height * .32), end: point(mask.y + mask.height * .63) };
        }, { status, wanted });
        if (where.visible) return;
        touchInput ??= await context.newCDPSession(page);
        await touchInput.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [where.start] });
        for (let step = 1; step <= 5; step++) {
            const y = where.start.y + (where.end.y - where.start.y) * step / 5;
            await touchInput.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: where.start.x, y }] });
        }
        await touchInput.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
        await page.waitForTimeout(240);
    }
    assert.fail('战斗属性的幸运行未进入可见遮罩');
}

// 点中背包内保存的原实例行，按详情及原确认入口穿戴。
async function wearNecklace(page, session, report) {
    const original = expected.item.item;
    await touch(page, safe + 'BottomHUD/Footer/Inventory');
    const point = await wait(page, ({ inventory, instance }) => {
        const list = cc.find(inventory + '/List')?.getComponent('List');
        if (!list?.node.activeInHierarchy) return null;
        const index = list.items.findIndex(row => Array.from(row.instanceId ?? [], byte => byte.toString(16).padStart(2, '0')).join('') === instance);
        const node = list.rows.get(index);
        if (!node?.activeInHierarchy) return null;
        const row = node._uiProps.uiTransformComp.getBoundingBoxToWorld(), mask = list.viewport._uiProps.uiTransformComp.getBoundingBoxToWorld();
        const x = row.x + row.width / 2, y = row.y + row.height / 2;
        if (x < mask.x || x > mask.x + mask.width || y < mask.y || y > mask.y + mask.height) return null;
        const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
        return { x: canvas.left + x * canvas.width / size.width, y: canvas.top + (size.height - y) * canvas.height / size.height };
    }, { inventory, instance: original.instance_id });
    await page.touchscreen.tap(point.x, point.y);
    await label(page, details + '/Title', '矿纹项链');
    await label(page, details + '/Action1/Label', '穿戴装备');
    const itemLines = await page.evaluate(details => cc.find(details + '/List').getComponent('List').items, details);
    assert(itemLines.some(line => line.includes('幸运+3')));
    await touch(page, details + '/Action1');
    await label(page, confirm + '/Title', '穿戴装备');
    assert.deepEqual(report.requests, [], '确认前不能提交装备操作');
    await touch(page, confirm + '/ConfirmButton');
    await wait(page, ({ session, instance, slot }) => session.inventory.equipped.some(row => row.slot === slot &&
        Array.from(row.item.instanceId, byte => byte.toString(16).padStart(2, '0')).join('') === instance), { session, instance: original.instance_id, slot: expected.item.slot });
    assert.equal(report.requests.length, 1);
    const request = report.requests[0];
    assert.equal(request.instance, original.instance_id);
    assert.equal(request.slot, expected.item.slot);
    assert.equal(request.version, String(original.version));
    assert.equal(request.unequip, false);
    assert.equal(request.binding, report.world.binding);
    assert.match(request.op, /^[0-9a-f]{32}$/);
    const receipt = report.results.find(value => value.op === request.op && value.binding === request.binding);
    assert(receipt && !receipt.pending && receipt.error === 0, '缺少原操作的成功回执');
    report.equip = request;
    report.receipt = receipt;
    report.itemLines = itemLines;
}

async function dismiss(page, node) {
    const active = await page.evaluate(node => !!cc.find(node)?.activeInHierarchy, node);
    if (!active) return;
    await page.touchscreen.tap(10, 130);
    await wait(page, node => !cc.find(node)?.activeInHierarchy, node);
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, clientBundleSha256: expected.clientBundleSha256, fixtureSha256: expected.fixtureSha256,
        viewport: { width: 390, height: 844 }, sockets: 0, errors: [], requests: [], results: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: report.viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const asset = page.waitForResponse(response => response.ok() && new URL(response.url()).pathname === '/assets/main/index.js', { timeout: 65000 });
        void asset.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_LUCK_BROWSER_ACCOUNT, password: process.env.MINIMIR_LUCK_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await asset).body()).digest('hex'), expected.clientBundleSha256);
        report.world = await originalWorld(page, session);
        assert.equal(report.world.map, String(expected.normalMap));
        assert.equal(report.world.rule, String(expected.ruleVersion));
        assert.equal(await page.evaluate(session => session.role.job, session), 1);
        if (report.world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        report.before = await readStatus(page, session, expected.enabled ? 0 : null);
        if (expected.enabled) {
            await showLuckRows(page, context, report.before.lines);
            await page.screenshot({ path: path.join(output, '01-before.png') });
            await dismiss(page, status);
            await dismiss(page, profile);
            await wearNecklace(page, session, report);
            await dismiss(page, details);
            await dismiss(page, inventory);
            report.after = await readStatus(page, session, 44500);
            await showLuckRows(page, context, report.after.lines);
            await page.screenshot({ path: path.join(output, '02-equipped.png') });
        } else {
            assert.deepEqual(report.requests, []);
            await page.screenshot({ path: path.join(output, '01-default.png') });
        }
        assert.equal(report.sockets, 1);
        assert.deepEqual(report.errors, []);
        report.passed = true;
    } catch (error) {
        report.error = error.stack || String(error);
        await page.screenshot({ path: path.join(output, 'failure.png') }).catch(() => {});
        throw error;
    } finally {
        fs.writeFileSync(path.join(output, 'luck-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, before: report.before.lines, after: report.after?.lines, screenshots: output }));
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
