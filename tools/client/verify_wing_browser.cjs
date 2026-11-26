// 本文件通过实际触屏和原 WebSocket 回执验收保存角色栏中的独立翅膀。
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

// 用本次独立 WebSocket 的原请求、报价及已确认回执提供跨端对账依据。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href || endpoint.port === '18080') {
            report.errors.push('触屏验收连接了非隔离节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40052 && frame?.id !== 40054) return;
            const result = { id: frame.id, request: frame.request, binding: frame.binding };
            if (frame.id === 40052) {
                const q = pb.WingQuoteRequest.decode(frame.body);
                Object.assign(result, { allow: q.allow_bound, scene: q.scene_epoch.toString(), wingId: hex(q.wing_id), version: q.wing_version.toString(),
                    session: hex(q.ui_session_id), feathers: q.feathers.map(row => ({ id: hex(row.instance_id),
                        version: row.version.toString(), quantity: Number(row.quantity.toString()) })) });
            } else {
                const q = pb.WingActionRequest.decode(frame.body);
                Object.assign(result, { action: q.action, allow: q.allow_bound, op: hex(q.operation?.op_id), seq: q.operation?.command_seq.toString(),
                    wingId: hex(q.wing_id), version: q.wing_version.toString(), session: hex(q.ui_session_id), quote: hex(q.quote_id),
                    tier: q.appearance_tier, hidden: q.hidden });
            }
            report.requests.push(result);
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40053) {
                const q = pb.WingQuote.decode(frame.body);
                report.quotes.push({ request: frame.request, binding: frame.binding, quote: hex(q.quote_id), scene: q.scene_epoch.toString(),
                    rank: q.target_rank, tier: q.target_appearance_tier, allow: q.allow_bound, session: hex(q.ui_session_id),
                    feathers: q.feathers.map(row => ({ id: hex(row.item.instance_id), version: row.item.version.toString(),
                        bound: row.item.bound, quantity: Number(row.quantity.toString()) })) });
            }
            if (frame?.id !== 40005) return;
            const result = pb.OperationResult.decode(frame.body);
            const receipt = result.wing;
            if (!receipt) return;
            const w = receipt.wing;
            report.results.push({ request: frame.request, binding: frame.binding, op: hex(result.op_id), seq: result.command_seq.toString(),
                pending: result.pending, error: result.error_code, action: receipt.action, quote: hex(receipt.quote_id),
                wingId: hex(w.wing_id), rank: w.rank, tier: w.appearance_tier, luck: w.luck, hidden: w.hidden, version: w.version.toString(),
                feathers: receipt.feathers.map(row => ({ id: hex(row.item.instance_id), version: row.item.version.toString(),
                    bound: row.item.bound, quantity: Number(row.quantity.toString()) })) });
        });
    });
}

// 用保存 List 中被遮罩实际显示的条目，触摸点击或向目标滑动。
async function listRow(page, browser, panel, key, select = true) {
    let input;
    try {
        for (let attempt = 0; attempt < 8; attempt++) {
            const where = await wait(page, ({ panel, key }) => {
                const list = cc.find(panel + '/List')?.getComponent('List');
                if (!list?.node.activeInHierarchy) return null;
                const bytes = row => Array.from(row.instanceId ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
                const index = list.items.findIndex(row => key === 'wing-row' ? row.wing === true : row.key === key || bytes(row) === key);
                if (index < 0) return null;
                // 世界包围盒会合并子节点，滚动内容伸出遮罩时必须用遮罩自身的矩形。
                const rect = node => {
                    const t = node._uiProps.uiTransformComp;
                    const min = t.convertToWorldSpaceAR(new cc.Vec3(-t.anchorX * t.width, -t.anchorY * t.height, 0));
                    const max = t.convertToWorldSpaceAR(new cc.Vec3((1 - t.anchorX) * t.width, (1 - t.anchorY) * t.height, 0));
                    return { x: min.x, y: min.y, width: max.x - min.x, height: max.y - min.y };
                };
                const node = list.rows.get(index), mask = rect(list.viewport);
                const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
                const point = (x, y) => ({ x: canvas.left + x * canvas.width / size.width,
                    y: canvas.top + (size.height - y) * canvas.height / size.height });
                let earlier = index < Math.min(...Array.from(list.rows.keys()));
                if (node?.activeInHierarchy) {
                    const box = rect(node), x = box.x + box.width / 2, y = box.y + box.height / 2;
                    if (x >= mask.x && x <= mask.x + mask.width && y >= mask.y + 3 && y <= mask.y + mask.height - 3)
                        return { point: point(x, y), text: node.getChildByName('Text')?.getComponent(cc.Label)?.string };
                    earlier = y > mask.y + mask.height;
                }
                const x = mask.x + mask.width / 2;
                return { start: point(x, mask.y + mask.height * (earlier ? .72 : .28)),
                    end: point(x, mask.y + mask.height * (earlier ? .28 : .72)) };
            }, { panel, key });
            if (where.point) {
                if (select) await page.touchscreen.tap(where.point.x, where.point.y);
                return where.text;
            }
            input ??= await browser.newCDPSession(page);
            await input.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [where.start] });
            for (let step = 1; step <= 5; step++) {
                const y = where.start.y + (where.end.y - where.start.y) * step / 5;
                await input.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: where.start.x, y }] });
            }
            await input.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
            await page.waitForTimeout(220);
        }
        assert.fail('保存列表未将原条目接入可触区域：' + panel + '/' + key);
    } finally {
        await input?.detach();
    }
}

async function rows(page, panel = details) {
    return page.evaluate(panel => cc.find(panel + '/List').getComponent('List').items, panel);
}

async function openAction(page, slot, text) {
    await label(page, details + '/Action' + slot + '/Label', text);
    await touch(page, details + '/Action' + slot);
}

async function dismiss(page, panel) {
    await page.touchscreen.tap(10, 130);
    await wait(page, panel => !cc.find(panel)?.activeInHierarchy, panel);
}

async function balance(page, session, version) {
    return wait(page, ({ session, version }) => {
        const inv = session.inventory;
        if (!inv || version && inv.wing?.version.toString() !== version) return null;
        const id = value => Array.from(value ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
        const item = row => ({ id: id(row.instanceId), version: row.version.toString(), quantity: Number(row.quantity.toString()), bound: row.bound });
        const w = inv.wing;
        return { capacity: inv.capacity, items: inv.items.map(item), equipped: inv.equipped.map(row => ({ slot: row.slot, ...item(row.item) })),
            wing: w ? { wingId: id(w.wingId), rank: w.rank, tier: w.appearanceTier, luck: w.luck, hidden: w.hidden, version: w.version.toString() } : null };
    }, { session, version });
}

// 在普通角色栏真实打开战斗属性，同时检查权威快照和显示的百分数。
async function attributes(page, session) {
    await touch(page, role + '/Recycle');
    await label(page, status + '/Title', '战斗属性');
    const value = await wait(page, ({ session, status }) => {
        const attrs = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle)?.combatAttributes;
        const lines = cc.find(status + '/List')?.getComponent('List')?.items;
        return attrs && lines?.length ? { enabled: attrs.luckEnabled === true, attack: attrs.luckAttackPpm ?? 0,
            lines: lines.filter(text => /^幸运一击 /.test(text)) } : null;
    }, { session, status });
    assert.equal(value.enabled, expected.enabled);
    assert.equal(value.attack, expected.expectedLuckPPM);
    assert.deepEqual(value.lines, expected.enabled ? ['幸运一击 ' + expected.expectedLuckPPM / 10000 + '%'] : []);
    await dismiss(page, status);
    return value;
}

// 先在第二步取消，核对原绑定材料仍不可选；再由真实两次确认开启当前面板的授权。
async function confirmBinding(page, session, report) {
    const start = async () => {
        await openAction(page, 2, '开启绑材');
        await label(page, confirm + '/Title', '允许绑定羽毛');
        await label(page, confirm + '/Message', '允许选择并消耗绑定羽毛');
        await label(page, confirm + '/ConfirmButton/Label', '继续开启');
        await touch(page, confirm + '/ConfirmButton');
        await label(page, confirm + '/Title', '再次确认绑定投入');
        await label(page, confirm + '/ConfirmButton/Label', '确认开启');
    };
    const before = await balance(page, session);
    assert(!(await rows(page)).some(row => row.key === expected.feather.instance_id), '未授权时不应显示原绑羽毛堆');
    await start();
    await page.screenshot({ path: path.join(output, '02-binding-confirm.png') });
    await touch(page, confirm + '/CancelButton');
    await label(page, details + '/Action2/Label', '开启绑材');
    assert((await rows(page)).includes('使用绑定资源 未开启'));
    assert.deepEqual(await balance(page, session), before);
    assert.deepEqual(report.requests, []);
    report.boundCancel = true;
    await start();
    await touch(page, confirm + '/ConfirmButton');
    await label(page, details + '/Action2/Label', '关闭绑材');
    assert((await rows(page)).includes('使用绑定资源 已开启'));
    assert.deepEqual(await balance(page, session), before);
    assert.deepEqual(report.requests, []);
    report.boundConfirmed = true;
}

// 对拍每次按钮确认的协议操作与同请求、同绑定、同角色序号的真实回执。
async function actionResult(page, session, report, rank, tier, hidden, action, spent, quote) {
    const request = report.requests.at(-1);
    const receipt = report.results.find(result => result.request === request?.request);
    assert(receipt, '缺少与真实触屏动作对应的原回执');
    assert.deepEqual([request.id, request.action, request.binding, receipt.error, receipt.pending, receipt.binding, receipt.op, receipt.seq],
        [40054, action, report.world.binding, 0, false, request.binding, request.op, request.seq]);
    assert.match(request.op, /^[a-f0-9]{32}$/);
    assert.match(receipt.wingId, /^[a-f0-9]{32}$/);
    assert.deepEqual([receipt.action, receipt.rank, receipt.tier, receipt.hidden, receipt.luck, receipt.version, receipt.quote],
        [action, rank, tier, hidden, 0, String(report.operations.length + 1), quote?.quote ?? '']);
    assert.deepEqual(receipt.feathers, quote?.feathers ?? []);
    if (quote) {
        assert.equal(request.allow, true);
        assert.equal(request.session, quote.session);
        assert.equal(request.quote, quote.quote);
    } else {
        assert.deepEqual([request.allow, request.quote, request.tier, request.hidden], [false, '', tier, hidden]);
    }
    const previous = report.operations.at(-1);
    assert.deepEqual([request.wingId, request.version], [previous?.wingId ?? '', previous?.version ?? '0']);
    if (previous) assert.equal(receipt.wingId, previous.wingId);
    const state = await balance(page, session, receipt.version);
    const { wingId, luck, version } = receipt;
    assert.deepEqual(state.wing, { wingId, rank, tier, luck, hidden, version });
    report.operations.push({ op: request.op, action, rank, tier, hidden, luck, version, wingId, spent });
    return state;
}

async function growWing(page, browser, session, report, rank, tier, before) {
    await label(page, details + '/Title', '选择羽毛');
    const expectedStack = before.items.find(item => item.id === expected.feather.instance_id);
    assert(expectedStack && expectedStack.bound);
    const text = await listRow(page, browser, details, expectedStack.id);
    assert(text.includes('绑定羽毛') && text.includes(expectedStack.id.slice(-6)));
    assert((await rows(page)).includes('已选羽毛 ' + rank + '/' + rank + '根'));
    const opCount = report.requests.length;
    await openAction(page, 1, '查看报价');
    await label(page, details + '/Title', '翅膀成长报价');
    const preview = await rows(page);
    assert(preview.includes('成功率 100% · 角色专属') && preview.includes('结算外观 ' + tier + '阶'));
    assert.equal(report.requests.length, opCount + 1);
    const request = report.requests.at(-1), quote = report.quotes.at(-1);
    assert.match(quote.quote, /^[a-f0-9]{32}$/);
    assert.match(request.session, /^[a-f0-9]{32}$/);
    assert.deepEqual([request.id, request.scene, request.binding, request.allow, request.wingId, request.version],
        [40052, report.world.scene, report.world.binding, true, before.wing?.wingId ?? '', before.wing?.version ?? '0']);
    assert.deepEqual([quote.request, quote.binding, quote.session, quote.scene, quote.rank, quote.tier, quote.allow],
        [request.request, request.binding, request.session, request.scene, rank, tier, true]);
    const original = { id: expectedStack.id, version: expectedStack.version, quantity: rank };
    assert.deepEqual(request.feathers, [original]);
    assert.deepEqual(quote.feathers, [{ ...original, bound: true }]);
    assert.deepEqual(await balance(page, session), before, '报价不应先消耗绑定材料');
    await openAction(page, 1, rank === 1 ? '确认激活' : '确认升阶');
    await label(page, details + '/Title', rank === 1 ? '翅膀激活完成' : '翅膀升阶完成');
    const after = await actionResult(page, session, report, rank, tier, false, 1, rank, quote);
    const remaining = expectedStack.quantity - rank;
    const entries = before.items.flatMap(item => item.id !== expectedStack.id ? [item] : remaining ? [{
        ...item, quantity: remaining, version: (BigInt(item.version) + 1n).toString()
    }] : []);
    assert.deepEqual(after.items, entries);
    assert.deepEqual([after.capacity, after.equipped], [before.capacity, before.equipped]);
    return after;
}

async function runWing(page, browser, session, report, initial) {
    assert.equal(await listRow(page, browser, role, 'wing-row'), '翅膀 · 未激活');
    await label(page, details + '/Title', '翅膀');
    await label(page, details + '/Action1/Label', '激活翅膀');
    await page.screenshot({ path: path.join(output, '01-wing-inactive.png') });
    await openAction(page, 1, '激活翅膀');
    await label(page, details + '/Title', '选择羽毛');
    await confirmBinding(page, session, report);
    let state = initial;
    for (const [rank, tier] of [[1, 1], [2, 1], [3, 3]]) {
        state = await growWing(page, browser, session, report, rank, tier, state);
        if (rank < 3) await openAction(page, 1, '继续升阶');
    }
    assert((await rows(page)).includes('翅膀 3阶 · 角色专属'));
    await page.screenshot({ path: path.join(output, '03-wing-rank3.png') });
    await openAction(page, 1, '返回翅膀');
    await openAction(page, 2, '外观设置');
    await label(page, details + '/Title', '翅膀外观');
    await listRow(page, browser, details, '外观:1');
    assert((await rows(page)).includes('选中外观 1阶 · 显示'));
    await openAction(page, 1, '保存外观');
    await label(page, details + '/Title', '翅膀外观已保存');
    const selected = await actionResult(page, session, report, 3, 1, false, 2, 0);
    await openAction(page, 1, '返回翅膀');
    await openAction(page, 2, '外观设置');
    await label(page, details + '/Title', '翅膀外观');
    await openAction(page, 2, '改为隐藏');
    assert((await rows(page)).includes('选中外观 1阶 · 隐藏'));
    await openAction(page, 1, '保存外观');
    await label(page, details + '/Title', '翅膀外观已保存');
    const hidden = await actionResult(page, session, report, 3, 1, true, 2, 0);
    assert.deepEqual([hidden.items, hidden.equipped, hidden.capacity], [selected.items, selected.equipped, selected.capacity]);
    assert.equal(hidden.items.length, expected.bagCount - 1);
    assert((await rows(page)).includes('外观 1阶 · 已隐藏'));
    assert((await rows(page)).includes('原翅膀尾号 ' + hidden.wing.wingId.slice(-8)));
    await page.screenshot({ path: path.join(output, '04-wing-hidden.png') });
    report.final = hidden.wing;
    await dismiss(page, details);
    assert.equal(await listRow(page, browser, role, 'wing-row', false), '翅膀 · 3阶 · 外观隐藏');
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, fixtureSha256: expected.fixtureSha256, clientBundleSha256: expected.clientBundleSha256,
        viewport: { width: 390, height: 844 }, sockets: 0, errors: [], requests: [], quotes: [], results: [], operations: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: report.viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const asset = page.waitForResponse(response => response.ok() && new URL(response.url()).pathname === '/assets/main/index.js', { timeout: 65000 });
        void asset.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_WING_BROWSER_ACCOUNT, password: process.env.MINIMIR_WING_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await asset).body()).digest('hex'), expected.clientBundleSha256);
        report.world = await originalWorld(page, session);
        assert.equal(report.world.map, String(expected.normalMap));
        assert.equal(report.world.rule, String(expected.ruleVersion));
        if (report.world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        const before = await balance(page, session);
        assert.deepEqual([before.wing, before.capacity, before.items.length, before.equipped.length], [null, expected.capacity, expected.bagCount, 1]);
        await touch(page, safe + 'BottomHUD/QuickNav/Role');
        const sword = await listRow(page, context, role, expected.weapon.instance_id, false);
        assert(sword.includes(expected.weaponName), '保存角色栏应显示旧原木剑');
        report.oldEquipmentVisible = true;
        const first = await attributes(page, session);
        if (expected.enabled) {
            await runWing(page, context, session, report, before);
            assert.equal(report.requests.length, 8);
        } else {
            assert(!(await rows(page, role)).some(row => row.wing === true), '默认角色栏出现翅膀入口');
            report.defaultEntryHidden = true;
            await page.screenshot({ path: path.join(output, '01-default-role.png') });
            assert.deepEqual(await balance(page, session), before);
            assert.deepEqual(report.requests, []);
        }
        const last = await attributes(page, session);
        report.attributes = { before: first.attack, after: last.attack, enabled: first.enabled };
        assert.deepEqual(last, first, '升阶和隐藏不能重置原装备或权威幸运');
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
        fs.writeFileSync(path.join(output, 'wing-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, defaultEntryHidden: report.defaultEntryHidden,
        boundCancel: report.boundCancel, boundConfirmed: report.boundConfirmed, operations: report.operations.length,
        final: report.final, attributes: report.attributes, screenshots: output }));
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
