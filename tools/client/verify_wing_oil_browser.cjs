// 用真实 Web Mobile 触控已保存的培养和角色面板，只读观察原联网报价与回执。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const role = safe + 'UIMgr/Panels/Role';
const details = safe + 'PopupView/Panels/ItemActions';
const status = safe + 'PopupView/Panels/StatusDetails';
const confirm = safe + 'PopupView/Panels/Confirm';
const hex = value => Buffer.from(value ?? []).toString('hex');
const numberText = value => value?.toString() ?? '0';

// 固化真实连接上的独占目标、原油和唯一已确认回执，保留后端对账所需操作号。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href || endpoint.port === '18080') {
            report.errors.push('连接不属于本次隔离游戏节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40049 && frame?.id !== 40051) return;
            const value = (frame.id === 40049 ? pb.WeaponOilQuoteRequest : pb.WeaponOilCommitRequest).decode(frame.body);
            report.requests.push({ id: frame.id, request: frame.request, binding: frame.binding, wingId: hex(value.wing?.wing_id),
                weapon: hex(value.weapon?.instance_id), equipped: value.weapon?.equipped ?? false,
                version: numberText(value.wing?.version ?? value.weapon?.version), scene: numberText(value.scene_epoch), allowed: value.allow_bound,
                session: hex(value.ui_session_id), quote: hex(value.quote_id), op: hex(value.operation?.op_id),
                seq: numberText(value.operation?.command_seq), revision: numberText(value.operation?.expected_revision) });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40050) {
                const q = pb.WeaponOilQuote.decode(frame.body);
                report.quotes.push({ request: frame.request, binding: frame.binding, quote: hex(q.quote_id), session: hex(q.ui_session_id), scene: numberText(q.scene_epoch),
                    wingId: hex(q.wing?.wing_id), weapon: hex(q.weapon?.instance_id), version: numberText(q.wing?.version ?? q.weapon?.version),
                    equipped: q.equipped, allowed: q.allow_bound, oil: hex(q.oil?.instance_id), oilBound: q.oil?.bound,
                    oilVersion: numberText(q.oil?.version), oilQuantity: numberText(q.oil?.quantity), previous: q.current_luck,
                    successLuck: q.success_luck, failureLuck: q.failure_luck, chance: q.success_ppm, pity: q.pity_used,
                    threshold: q.pity_after, guaranteed: q.guaranteed, outputBound: q.output_bound, revision: numberText(q.role_revision) });
            }
            if (frame?.id !== 40005) return;
            const result = pb.OperationResult.decode(frame.body), receipt = result.weapon_oil;
            if (!receipt) return;
            report.receipts.push({ request: frame.request, binding: frame.binding, op: hex(result.op_id), error: result.error_code, pending: result.pending,
                seq: numberText(result.command_seq), revision: numberText(result.new_revision), quote: hex(receipt.quote_id), wingId: hex(receipt.wing?.wing_id),
                weapon: hex(receipt.weapon?.instance_id), version: numberText(receipt.wing?.version), rank: receipt.wing?.rank,
                tier: receipt.wing?.appearance_tier, hidden: receipt.wing?.hidden, luck: receipt.wing?.luck,
                pity: receipt.pity_used, threshold: receipt.pity_after, toSix: receipt.wing?.oil_to_six, toNine: receipt.wing?.oil_to_nine,
                previous: receipt.before_luck, equipped: receipt.equipped, success: receipt.success, guaranteed: receipt.guaranteed,
                oil: hex(receipt.oil?.instance_id), oilBound: receipt.oil?.bound, oilVersion: numberText(receipt.oil?.version),
                oilQuantity: numberText(receipt.oil?.quantity) });
        });
    });
}

// 保证实际列表遮罩内存在目标，使用触屏滑动和点击，互不直接调用界面回调。
async function showRow(page, context, panel, key, kind, tap = false) {
    let input;
    try {
        for (let attempt = 0; attempt < 10; attempt++) {
            const where = await wait(page, ({ panel, key, kind }) => {
                const list = cc.find(panel + '/List')?.getComponent('List');
                if (!list?.node.activeInHierarchy) return null;
                const id = row => Array.from(row.instanceId ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
                const index = list.items.findIndex(row => kind === 'wing' ? row.wing === true : kind === 'weapon' ? id(row) === key :
                    kind === 'text' ? row === key : row.key === key);
                if (index < 0) return null;
                // 世界包围盒会合并列表的子节点，用矩形自身判定裁切边界。
                const rect = node => {
                    const t = node._uiProps.uiTransformComp;
                    const min = t.convertToWorldSpaceAR(new cc.Vec3(-t.anchorX * t.width, -t.anchorY * t.height, 0));
                    const max = t.convertToWorldSpaceAR(new cc.Vec3((1 - t.anchorX) * t.width, (1 - t.anchorY) * t.height, 0));
                    return { x: min.x, y: min.y, width: max.x - min.x, height: max.y - min.y };
                };
                const node = list.rows.get(index), mask = rect(list.viewport);
                const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
                const point = (x, y) => ({ x: canvas.left + x * canvas.width / size.width, y: canvas.top + (size.height - y) * canvas.height / size.height });
                let earlier = index < Math.min(...Array.from(list.rows.keys()));
                if (node?.activeInHierarchy) {
                    const box = rect(node), x = box.x + box.width / 2, y = box.y + box.height / 2;
                    if (x >= mask.x && x <= mask.x + mask.width && y >= mask.y + 4 && y <= mask.y + mask.height - 4)
                        return { point: point(x, y), text: node.getChildByName('Text')?.getComponent(cc.Label)?.string };
                    earlier = y > mask.y + mask.height;
                }
                const x = mask.x + mask.width / 2;
                return { start: point(x, mask.y + mask.height * (earlier ? .72 : .28)), end: point(x, mask.y + mask.height * (earlier ? .28 : .72)) };
            }, { panel, key, kind });
            if (where.point) {
                if (tap) await page.touchscreen.tap(where.point.x, where.point.y);
                return where.text;
            }
            input ??= await context.newCDPSession(page);
            await input.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [where.start] });
            for (let step = 1; step <= 5; step++) {
                const y = where.start.y + (where.end.y - where.start.y) * step / 5;
                await input.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: where.start.x, y }] });
            }
            await input.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
            await page.waitForTimeout(220);
        }
        assert.fail('未能通过手指滑动选到真实原条目：' + key);
    } finally {
        await input?.detach();
    }
}

async function rows(page, panel = details) {
    return page.evaluate(panel => cc.find(panel + '/List').getComponent('List').items, panel);
}

async function action(page, index, title) {
    await label(page, details + '/Action' + index + '/Label', title);
    await touch(page, details + '/Action' + index);
}

async function dismiss(page, panel) {
    await page.touchscreen.tap(10, 130);
    await wait(page, panel => !cc.find(panel)?.activeInHierarchy, panel);
}

async function state(page, session, version) {
    return wait(page, ({ session, version }) => {
        const inv = session.inventory;
        if (!inv || version && inv.wing?.version.toString() !== version) return null;
        const hex = value => Array.from(value ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
        const item = row => ({ id: hex(row.instanceId), luck: row.currentLuck ?? 0, bound: row.bound,
            quantity: row.quantity.toString(), version: row.version.toString(), durability: row.durabilityU?.toString() });
        const wing = inv.wing;
        return { items: inv.items.map(item), equipped: inv.equipped.map(row => ({ slot: row.slot, ...item(row.item) })), capacity: inv.capacity,
            wing: wing ? { wingId: hex(wing.wingId), version: wing.version.toString(), rank: wing.rank, tier: wing.appearanceTier,
                hidden: wing.hidden, luck: wing.luck, toSix: wing.oilToSix, toNine: wing.oilToNine } : null };
    }, { session, version });
}

async function attributes(page, session, chance) {
    await touch(page, role + '/Recycle');
    await label(page, status + '/Title', '战斗属性');
    const result = await wait(page, ({ session, status, chance }) => {
        const player = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle);
        const attrs = player?.combatAttributes, lines = cc.find(status + '/List')?.getComponent('List')?.items;
        if (!attrs || !lines?.length || (attrs.luckAttackPpm ?? 0) !== chance) return null;
        return { chance, enabled: attrs.luckEnabled === true, lines: lines.filter(row => /^幸运一击 /.test(row)) };
    }, { session, status, chance });
    assert.equal(result.enabled, expected.enabled);
    assert.deepEqual(result.lines, expected.enabled ? ['幸运一击 ' + chance / 10000 + '%'] : []);
    await dismiss(page, status);
    return result;
}

async function enterWingOil(page, context) {
    const selected = await showRow(page, context, role, '', 'wing', true);
    assert.equal(selected, '翅膀 · 3阶 · 外观隐藏');
    await label(page, details + '/Title', '翅膀');
    await action(page, 3, '祝福油培养');
    await label(page, details + '/Title', '翅膀祝福油');
}

// 真实开启绑定需要连续两次确认。先取消第二框，不允许提交也不改变原材料。
async function openBindingConfirmation(page) {
    await action(page, 2, '开启绑材');
    await label(page, confirm + '/Title', '开启绑定祝福油');
    await label(page, confirm + '/Message', '开启后优先使用绑定油');
    await label(page, confirm + '/ConfirmButton/Label', '继续开启');
    await touch(page, confirm + '/ConfirmButton');
    await label(page, confirm + '/Title', '再次确认绑定投入');
    await label(page, confirm + '/Message', '翅膀归属不变');
    await label(page, confirm + '/ConfirmButton/Label', '确认开启');
}

// 报价必须匹配原视图会话、原目标和本次真实绑定授权。
async function quoted(page, report, wing, chosen, version, allowed) {
    const index = report.requests.length;
    await action(page, 1, '查看报价');
    await label(page, details + '/Title', '祝福油报价');
    assert.equal(report.requests.length, index + 1);
    const request = report.requests[index], q = report.quotes.find(item => item.request === request.request);
    assert(q, '缺原请求报价');
    const wingId = wing ? expected.wing.wing_id : '', weapon = wing ? '' : expected.weapon.instance_id;
    assert.match(request.session, /^[a-f0-9]{32}$/);
    assert.match(q.quote, /^[a-f0-9]{32}$/);
    assert.deepEqual([request.id, request.binding, request.scene, request.wingId, request.weapon, request.equipped, request.version, request.allowed],
        [40049, report.world.binding, report.world.scene, wingId, weapon, !wing, version, allowed]);
    assert.deepEqual([q.binding, q.scene, q.wingId, q.weapon, q.equipped, q.version, q.session, q.allowed, q.oil, q.oilBound],
        [request.binding, request.scene, wingId, weapon, !wing, version, request.session, allowed, chosen.instance_id, chosen.bound]);
    return q;
}

async function runOil(page, context, session, report, before) {
    await enterWingOil(page, context);
    const first = await rows(page);
    assert(first.includes('角色专属（不变）') && first.includes('当前幸运+3') && first.includes('使用绑定资源 未开启'));
    await page.screenshot({ path: path.join(output, '01-wing-oil-entry.png') });
    await openBindingConfirmation(page);
    await page.screenshot({ path: path.join(output, '02-binding-confirm.png') });
    await touch(page, confirm + '/CancelButton');
    await label(page, details + '/Action2/Label', '开启绑材');
    assert((await rows(page)).includes('使用绑定资源 未开启'));
    assert.deepEqual(await state(page, session), before);
    assert.deepEqual(report.requests, []);
    const firstQuote = await quoted(page, report, true, expected.freeOil, before.wing.version, false);
    assert((await rows(page)).includes('消耗1瓶非绑定祝福油'));
    assert.deepEqual(await state(page, session), before);
    report.boundCancelled = true;
    await action(page, 3, '返回选对象');
    await label(page, details + '/Title', '翅膀祝福油');
    await openBindingConfirmation(page);
    await touch(page, confirm + '/ConfirmButton');
    await label(page, details + '/Action2/Label', '关闭绑材');
    assert((await rows(page)).includes('使用绑定资源 已开启'));
    report.boundGranted = true;

    const weaponKey = '穿戴:' + expected.weapon.instance_id;
    const weaponLabel = await showRow(page, context, details, weaponKey, 'option', true);
    assert.equal(weaponLabel, '选择 穿戴 ' + expected.weaponName + ' +3');
    await label(page, details + '/Title', '武器祝福油');
    await label(page, details + '/Action2/Label', '关闭绑材');
    const weaponQuote = await quoted(page, report, false, expected.boundOil, before.equipped[0].version, true);
    assert.equal(weaponQuote.session, firstQuote.session);
    assert.deepEqual(await state(page, session), before);
    await action(page, 3, '返回选对象');
    assert.equal(await showRow(page, context, details, '翅膀:' + before.wing.wingId, 'option', true), '选择 翅膀 3阶 +3');
    await label(page, details + '/Title', '翅膀祝福油');
    await label(page, details + '/Action2/Label', '关闭绑材');
    const q = await quoted(page, report, true, expected.boundOil, before.wing.version, true);
    assert.equal(q.session, weaponQuote.session);
    report.permissionPreserved = true;
    const preview = await rows(page);
    assert(preview.includes('成功率 100%') && preview.includes('累计耗油 100/100瓶') && preview.includes('本瓶保底，逐瓶升一级') &&
        preview.includes('消耗1瓶绑定祝福油') && preview.includes('失败也扣油，翅膀归属不变'));
    assert.deepEqual([q.previous, q.successLuck, q.failureLuck, q.chance, q.pity, q.threshold, q.guaranteed, q.outputBound],
        [3, 4, 3, 1_000_000, 100, 100, true, true]);
    await showRow(page, context, details, '累计耗油 100/100瓶', 'text');
    await page.screenshot({ path: path.join(output, '03-wing-oil-quote.png') });
    await action(page, 1, '使用祝福油');
    await label(page, details + '/Title', '祝福油结算');
    const request = report.requests.at(-1), receipt = report.receipts.find(item => item.request === request.request);
    assert(receipt, '缺真实独立用油回执');
    assert.match(request.op, /^[a-f0-9]{32}$/);
    assert.deepEqual([request.id, request.binding, request.wingId, request.weapon, request.equipped, request.allowed, request.quote,
        request.session, request.revision, request.version], [40051, q.binding, q.wingId, '', false, true, q.quote, q.session, q.revision, q.version]);
    assert.deepEqual([receipt.op, receipt.binding, receipt.error, receipt.pending, receipt.seq, receipt.quote, receipt.wingId, receipt.weapon,
        receipt.equipped, receipt.previous, receipt.luck, receipt.guaranteed, receipt.success, receipt.pity, receipt.threshold,
        receipt.oil, receipt.oilBound, receipt.oilVersion, receipt.oilQuantity],
        [request.op, q.binding, 0, false, request.seq, q.quote, q.wingId, '', false, 3, 4, true, true, 101, 100,
            q.oil, true, q.oilVersion, q.oilQuantity]);
    const after = await state(page, session, receipt.version);
    assert.deepEqual(after.wing, { ...before.wing, version: String(BigInt(before.wing.version) + 1n), luck: 4, toSix: 101 });
    assert.deepEqual([after.capacity, after.equipped], [before.capacity, before.equipped]);
    assert.deepEqual(after.items, before.items.map(item => item.id === q.oil ? { ...item, quantity: '1', version: '2' } : item));
    report.operation = { op: request.op, wingId: receipt.wingId, version: receipt.version, rank: receipt.rank, tier: receipt.tier,
        hidden: receipt.hidden, luck: receipt.luck, pity: receipt.pity, oil: receipt.oil, oilBound: receipt.oilBound };
    const lines = await rows(page);
    assert(lines.includes('角色专属（不变）') && lines.includes('当前幸运+4') && lines.includes('幸运一击贡献 6%') &&
        lines.includes('3→6段累计用油 101瓶') && lines.includes('已扣1瓶绑定祝福油'));
    await showRow(page, context, details, '已扣1瓶绑定祝福油', 'text');
    await page.screenshot({ path: path.join(output, '04-wing-oil-result.png') });
    await dismiss(page, details);
    await enterWingOil(page, context);
    await label(page, details + '/Action2/Label', '开启绑材');
    const reopened = await quoted(page, report, true, expected.freeOil, after.wing.version, false);
    assert.notEqual(reopened.session, q.session);
    assert.deepEqual(await state(page, session), after);
    assert.equal(report.requests.filter(item => item.id === 40051).length, 1);
    assert.equal(report.receipts.length, 1);
    report.reopenedOff = true;
    await dismiss(page, details);
}

async function main() {
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
        const session = await login(page, 'role', { account: process.env.MINIMIR_WING_OIL_BROWSER_ACCOUNT, password: process.env.MINIMIR_WING_OIL_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.clientBundleSha256);
        report.world = await originalWorld(page, session);
        assert.equal(report.world.map, String(expected.normalMap));
        assert.equal(report.world.rule, String(expected.ruleVersion));
        if (report.world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        await touch(page, safe + 'BottomHUD/QuickNav/Role');
        assert((await showRow(page, context, role, expected.weapon.instance_id, 'weapon')).includes(expected.weaponName));
        report.oldEquipmentVisible = true;
        const before = await state(page, session);
        assert.deepEqual(before.equipped.map(item => item.id), [expected.weapon.instance_id]);
        const originalAttributes = await attributes(page, session, expected.attributesBefore);
        if (expected.enabled) {
            assert(before.wing.hidden && before.wing.luck === 3 && before.wing.toSix === 100);
            await runOil(page, context, session, report, before);
        } else {
            assert(!(await rows(page, role)).some(row => row.wing === true));
            report.defaultWingHidden = true;
            await touch(page, role + '/EquipmentBoard/Slot0');
            await label(page, status + '/Title', expected.weaponName);
            assert(!await page.evaluate(details => cc.find(details)?.activeInHierarchy, details));
            assert(!(await rows(page, status)).some(row => String(row).includes('祝福油')));
            report.defaultWeaponHidden = true;
            await page.screenshot({ path: path.join(output, '01-default-weapon.png') });
            await dismiss(page, status);
            assert.deepEqual(await state(page, session), before);
            assert.deepEqual(report.requests, []);
        }
        const latestAttributes = await attributes(page, session, expected.attributesAfter);
        report.attributes = { before: originalAttributes.chance, after: latestAttributes.chance, enabled: latestAttributes.enabled };
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
        fs.writeFileSync(path.join(output, 'wing-oil-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, boundCancelled: report.boundCancelled,
        permissionPreserved: report.permissionPreserved, reopenedOff: report.reopenedOff, operation: report.operation, attributes: report.attributes, screenshots: output }));
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
