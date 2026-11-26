// 使用真实 Web Mobile 触控角色装备槽，按隔离联机原包核对强化投入和回执。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const role = safe + 'UIMgr/Panels/Role';
const board = role + '/EquipmentBoard';
const details = safe + 'PopupView/Panels/ItemActions';
const status = safe + 'PopupView/Panels/StatusDetails';
const confirm = safe + 'PopupView/Panels/Confirm';
const hex = value => Buffer.from(value ?? []).toString('hex');
const text = value => value?.toString() ?? '0';

// 从真实 WebSocket 帧提取原石堆、报价与永久操作号，供独立数据库核对。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href || endpoint.port === '18080') {
            report.errors.push('实际连接未走独立验收游戏节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40055) {
                const value = pb.EquipmentEnhancementQuoteRequest.decode(frame.body);
                report.requests.push({ id: frame.id, request: frame.request, binding: frame.binding, slot: value.slot,
                    scene: text(value.scene_epoch), allowed: value.allow_bound, session: hex(value.ui_session_id),
                    stones: value.stones.map(stone => ({ id: hex(stone.instance_id), version: text(stone.version), quantity: text(stone.quantity) })) });
            }
            if (frame?.id === 40057) {
                const value = pb.EquipmentEnhancementCommitRequest.decode(frame.body);
                report.requests.push({ id: frame.id, request: frame.request, binding: frame.binding, slot: value.slot,
                    quote: hex(value.quote_id), session: hex(value.ui_session_id), op: hex(value.operation?.op_id),
                    sequence: text(value.operation?.command_seq), roleRevision: text(value.operation?.expected_revision) });
            }
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40056) {
                const quote = pb.EquipmentEnhancementQuote.decode(frame.body);
                report.quotes.push({ request: frame.request, binding: frame.binding, quote: hex(quote.quote_id), session: hex(quote.ui_session_id),
                    scene: text(quote.scene_epoch), allowed: quote.allow_bound, slot: quote.slot, beforeLevel: quote.current_level,
                    afterLevel: quote.target_level, currentBonusBp: quote.current_bonus_bp, targetBonusBp: quote.target_bonus_bp,
                    successPpm: quote.success_ppm, roleRevision: text(quote.role_revision), stones: spends(quote.stones) });
            }
            if (frame?.id === 40005) {
                const operation = pb.OperationResult.decode(frame.body), receipt = operation.equipment_enhancement;
                if (!receipt) return;
                report.receipts.push({ request: frame.request, binding: frame.binding, quote: hex(receipt.quote_id), op: hex(operation.op_id),
                    error: operation.error_code, pending: operation.pending, revision: text(operation.new_revision), slot: receipt.slot,
                    beforeLevel: receipt.before_level, afterLevel: receipt.after_level, success: receipt.success, stones: spends(receipt.stones) });
            }
        });
    });
}

function spends(stones) {
    return stones.map(({ item, quantity }) => ({ id: hex(item?.instance_id), bound: item?.bound, version: text(item?.version),
        available: text(item?.quantity), quantity: text(quantity) }));
}

async function action(page, index, name) {
    await label(page, `${details}/Action${index}/Label`, name);
    await touch(page, `${details}/Action${index}`);
}

async function dismiss(page, panel) {
    await page.touchscreen.tap(10, 130);
    await wait(page, panel => !cc.find(panel)?.activeInHierarchy, panel);
}

async function listRows(page, panel = details) {
    return wait(page, panel => {
        const list = cc.find(panel + '/List')?.getComponent('List');
        return list?.node.activeInHierarchy ? list.items : null;
    }, panel);
}

async function line(page, text) {
    return wait(page, ({ details, text }) => {
        const list = cc.find(details + '/List')?.getComponent('List');
        return list?.node.activeInHierarchy && list.items.some(row => typeof row === 'string' && row.includes(text));
    }, { details, text });
}

// 触摸滑动列表并点击实际进入遮罩的原材料行，不直接调用界面回调。
async function tapOption(page, key) {
    let input;
    try {
        for (let attempt = 0; attempt < 10; attempt++) {
            const move = await wait(page, ({ details, key }) => {
                const list = cc.find(details + '/List')?.getComponent('List');
                if (!list?.node.activeInHierarchy) return null;
                const index = list.items.findIndex(row => row?.key === key);
                if (index < 0 || !list.rows.size) return null;
                const row = list.rows.get(index), clip = list.viewport._uiProps.uiTransformComp;
                const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
                const screen = (x, y) => ({ x: canvas.left + x * canvas.width / size.width, y: canvas.top + (size.height - y) * canvas.height / size.height });
                const bottom = -clip.height * clip.anchorY, top = bottom + clip.height;
                let earlier = index < Math.min(...Array.from(list.rows.keys()));
                if (row?.activeInHierarchy) {
                    const box = row._uiProps.uiTransformComp.getBoundingBoxToWorld();
                    const x = box.x + box.width / 2, y = box.y + box.height / 2;
                    const local = clip.convertToNodeSpaceAR(new cc.Vec3(x, y));
                    if (local.y >= bottom + 4 && local.y <= top - 4) return { point: screen(x, y) };
                    earlier = local.y > top - 4;
                }
                const from = clip.convertToWorldSpaceAR(new cc.Vec3(0, bottom + clip.height * (earlier ? .74 : .26)));
                const to = clip.convertToWorldSpaceAR(new cc.Vec3(0, bottom + clip.height * (earlier ? .26 : .74)));
                return { from: screen(from.x, from.y), to: screen(to.x, to.y) };
            }, { details, key });
            if (move.point) {
                await page.touchscreen.tap(move.point.x, move.point.y);
                return;
            }
            input ??= await page.context().newCDPSession(page);
            await input.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [move.from] });
            for (let step = 1; step <= 5; step++) {
                const y = move.from.y + (move.to.y - move.from.y) * step / 5;
                await input.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: move.from.x, y }] });
            }
            await input.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
            await page.waitForTimeout(220);
        }
        assert.fail(`手指滑动后仍未找到材料或部位 ${key}`);
    } finally {
        await input?.detach();
    }
}

async function assets(page, session, afterRevision) {
    return wait(page, ({ session, afterRevision }) => {
        const inv = session.inventory, wallet = session.wallet;
        if (!inv || !wallet || afterRevision && BigInt(inv.revision.toString()) < BigInt(afterRevision)) return null;
        const item = row => ({ id: Array.from(row.instanceId, value => value.toString(16).padStart(2, '0')).join(''),
            itemId: row.itemId, version: row.version.toString(), quantity: row.quantity.toString(), bound: row.bound,
            durability: row.durabilityU?.toString() });
        return { revision: inv.revision.toString(), level: inv.level, slots: inv.equipmentEnhancementSlots,
            equipped: inv.equipped.map(worn => ({ slot: worn.slot, ...item(worn.item) })), items: inv.items.map(item),
            wallet: ['gold', 'boundGold', 'yuanbaoUnits', 'boundYuanbaoUnits'].map(key => wallet[key].toString()) };
    }, { session, afterRevision });
}

async function bindingPrompts(page) {
    await action(page, 2, '开启绑材');
    await label(page, confirm + '/Title', '开启绑定强化石');
    await label(page, confirm + '/ConfirmButton/Label', '继续开启');
    await touch(page, confirm + '/ConfirmButton');
    await label(page, confirm + '/Title', '再次确认绑定投入');
    await label(page, confirm + '/Message', '失败仍扣除所选强化石');
    await label(page, confirm + '/ConfirmButton/Label', '确认开启');
}

async function openWeapon(page) {
    await touch(page, board + '/Slot0');
    await label(page, details + '/Title', expected.weaponName);
    await action(page, 1, '部位强化');
    await label(page, details + '/Title', '武器强化');
}

async function observeQuote(page, report, source, allowed) {
    const start = report.requests.length;
    await tapOption(page, source);
    await line(page, '已选强化石 10/10颗');
    await action(page, 1, '查看报价');
    await label(page, details + '/Title', '强化报价');
    await line(page, '基础属性加成 +0% → +0.1%');
    assert.equal(report.requests.length, start + 1);
    const request = report.requests[start], quote = report.quotes.find(value => value.request === request.request);
    assert(quote, '缺少当前触屏对应的原报价');
    assert.match(quote.quote, /^[a-f0-9]{32}$/);
    assert.match(request.session, /^[a-f0-9]{32}$/);
    assert.deepEqual([request.id, request.binding, request.scene, request.slot, request.allowed],
        [40055, report.world.binding, report.world.scene, 0, allowed]);
    assert.deepEqual([quote.binding, quote.scene, quote.slot, quote.allowed, quote.session, quote.beforeLevel, quote.afterLevel,
        quote.currentBonusBp, quote.targetBonusBp, quote.successPpm],
    [request.binding, request.scene, 0, allowed, request.session, 0, 1, 0, 10, 1_000_000]);
    assert.equal(quote.stones.length, 1);
    const stone = quote.stones[0];
    assert.deepEqual([stone.id, stone.bound, stone.quantity], [source, allowed, '10']);
    assert.deepEqual(request.stones, [{ id: source, version: stone.version, quantity: '10' }]);
    return quote;
}

async function candidate(page, session, report, before) {
    const free = expected.freeStone.instance_id, bound = expected.boundStone.instance_id;
    assert.deepEqual(before.slots.map(row => row.level), Array(10).fill(0));
    await touch(page, board + '/Slot1');
    await label(page, details + '/Title', '衣服强化');
    await line(page, '当前为空部位，穿戴后生效');
    report.emptySlotVisible = true;
    await page.screenshot({ path: path.join(output, '01-empty-slot.png') });
    await dismiss(page, details);
    await openWeapon(page);
    assert.deepEqual((await listRows(page)).filter(row => row?.key).map(row => row.key), [free]);
    const unboundQuote = await observeQuote(page, report, free, false);
    report.unboundQuoteVerified = true;
    await action(page, 2, '重新选石');

    await bindingPrompts(page);
    await touch(page, confirm + '/CancelButton');
    await line(page, '使用绑定强化石 未开启');
    assert.deepEqual((await listRows(page)).filter(row => row?.key).map(row => row.key), [free]);
    assert.equal(report.receipts.length, 0);
    report.boundCancelled = true;
    await bindingPrompts(page);
    await page.screenshot({ path: path.join(output, '02-binding-confirm.png') });
    await touch(page, confirm + '/ConfirmButton');
    await line(page, '使用绑定强化石 已开启');
    report.boundGranted = true;

    await action(page, 3, '更换部位');
    await label(page, details + '/Title', '选择强化部位');
    await tapOption(page, '部位1');
    await label(page, details + '/Title', '衣服强化');
    await line(page, '使用绑定强化石 已开启');
    await action(page, 3, '更换部位');
    await tapOption(page, '部位0');
    await label(page, details + '/Title', '武器强化');
    await line(page, '使用绑定强化石 已开启');
    report.permissionPreserved = true;
    const quote = await observeQuote(page, report, bound, true);
    assert.equal(quote.session, unboundQuote.session);
    assert.notEqual(quote.quote, unboundQuote.quote);
    await page.screenshot({ path: path.join(output, '03-enhancement-quote.png') });
    await action(page, 1, '确认强化');
    await label(page, details + '/Title', '强化结算');
    await line(page, '武器强化成功');
    await line(page, '部位强化 +0 → +1');
    const committed = report.requests.filter(request => request.id === 40057);
    assert.equal(committed.length, 1);
    assert.equal(report.receipts.length, 1);
    const [commit] = committed, [receipt] = report.receipts;
    assert.deepEqual([commit.quote, commit.session, commit.slot, commit.roleRevision], [quote.quote, quote.session, 0, quote.roleRevision]);
    assert.deepEqual([receipt.quote, receipt.op, receipt.slot, receipt.request, receipt.binding, receipt.error, receipt.pending, receipt.success,
        receipt.beforeLevel, receipt.afterLevel], [quote.quote, commit.op, 0, commit.request, commit.binding, 0, false, true, 0, 1]);
    assert.deepEqual(receipt.stones, quote.stones);
    report.operation = { op: receipt.op, quote: receipt.quote, slot: receipt.slot, beforeLevel: receipt.beforeLevel, afterLevel: receipt.afterLevel,
        roleRevision: commit.roleRevision, stoneId: bound, stoneBound: true, quantity: '10' };
    const after = await assets(page, session, receipt.revision);
    assert.deepEqual(after.equipped, before.equipped);
    assert.deepEqual(after.wallet, before.wallet);
    report.weaponEqual = true;
    report.walletEqual = true;
    const oldStone = before.items.find(item => item.id === bound), newStone = after.items.find(item => item.id === bound);
    assert(oldStone?.bound);
    assert.equal(BigInt(oldStone.quantity) - BigInt(newStone?.quantity ?? '0'), 10n);
    if (newStone) assert(BigInt(newStone.version) > BigInt(oldStone.version));
    assert.deepEqual(after.items.filter(item => item.id !== bound), before.items.filter(item => item.id !== bound));
    assert.deepEqual(after.slots.map(row => row.level), [1, ...Array(9).fill(0)]);
    await page.screenshot({ path: path.join(output, '04-enhancement-receipt.png') });
    await dismiss(page, details);
    await label(page, board + '/Slot0/Label', '+1');
    await openWeapon(page);
    await line(page, '基础属性加成 +0.1%');
    await line(page, '使用绑定强化石 未开启');
    assert.deepEqual((await listRows(page)).filter(row => row?.key).map(row => row.key), [free]);
    report.reopenedOff = true;
    await page.screenshot({ path: path.join(output, '05-enhancement-slot.png') });
    await dismiss(page, details);
}

async function defaultRole(page, session, report, before) {
    assert.equal(before.slots.length, 0);
    await touch(page, board + '/Slot0');
    await label(page, status + '/Title', expected.weaponName);
    assert(!(await listRows(page, status)).some(row => String(row).includes('强化')));
    assert(!await page.evaluate(details => cc.find(details)?.activeInHierarchy, details));
    await page.screenshot({ path: path.join(output, '01-default-weapon.png') });
    await dismiss(page, status);
    assert.equal(await page.evaluate(node => cc.find(node).getComponent(cc.Button).interactable, board + '/Slot1'), false);
    assert.deepEqual(await assets(page, session), before);
    assert.deepEqual(report.requests, []);
    report.defaultEnhancementHidden = true;
    report.walletEqual = true;
    report.weaponEqual = true;
}

async function main() {
    assert(typeof expected.enabled === 'boolean' && /^[a-f0-9]{64}$/.test(expected.clientBundleSha256));
    const report = { passed: false, enabled: expected.enabled, fixtureSha256: expected.fixtureSha256, clientBundleSha256: expected.clientBundleSha256,
        viewport: { width: 390, height: 844 }, sockets: 0, errors: [], requests: [], quotes: [], receipts: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: report.viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const bundle = page.waitForResponse(response => response.ok() && new URL(response.url()).pathname === '/assets/main/index.js', { timeout: 65_000 });
        void bundle.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_EQUIPMENT_ENHANCEMENT_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_EQUIPMENT_ENHANCEMENT_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.clientBundleSha256);
        report.world = await originalWorld(page, session);
        assert.equal(report.world.map, String(expected.normalMap));
        assert.equal(report.world.rule, String(expected.ruleVersion));
        assert.equal(report.world.role, expected.role);
        if (report.world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        await touch(page, safe + 'BottomHUD/QuickNav/Role');
        await label(page, board + '/Slot0/Label', expected.weaponName);
        const before = await assets(page, session);
        assert.equal(before.level, 2);
        assert.deepEqual(before.equipped.map(worn => [worn.slot, worn.id, worn.itemId, worn.bound]), [[0, expected.weapon.instance_id, 218, false]]);
        report.oldEquipmentVisible = true;
        if (expected.enabled) await candidate(page, session, report, before);
        else await defaultRole(page, session, report, before);
        const worldAfter = await originalWorld(page, session);
        for (const key of ['map', 'role', 'binding', 'scene', 'life']) assert.equal(worldAfter[key], report.world[key], key);
        assert.equal(report.sockets, 1);
        assert.deepEqual(report.errors, []);
        report.passed = true;
    } catch (error) {
        report.error = error.stack || String(error);
        await page.screenshot({ path: path.join(output, 'failure.png') }).catch(() => {});
        throw error;
    } finally {
        fs.writeFileSync(path.join(output, 'equipment-enhancement-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, operation: report.operation, screenshots: output }));
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
