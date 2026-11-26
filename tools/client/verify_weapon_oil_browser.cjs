// 用真实手机尺寸触屏点击 Creator 中已保存的场景，只观察列表、基线与实际 WebSocket 数据。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const profile = safe + 'UIMgr/Panels/Role';
const inventory = safe + 'UIMgr/Panels/Inventory';
const details = safe + 'PopupView/Panels/ItemActions';
const status = safe + 'PopupView/Panels/StatusDetails';
const confirm = safe + 'PopupView/Panels/Confirm';
const hex = value => Buffer.from(value ?? []).toString('hex');
const numberText = value => value?.toString() ?? '0';

// 按请求、连接代次和永久操作号捕获真实报价与结算，避免把本地展示认作已提交事实。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href || endpoint.port === '18080') {
            report.errors.push('实际客户端没有连接本次隔离游戏节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40049 && frame?.id !== 40051) return;
            const value = (frame.id === 40049 ? pb.WeaponOilQuoteRequest : pb.WeaponOilCommitRequest).decode(frame.body);
            report.requests.push({ id: frame.id, request: frame.request, binding: frame.binding, instance: hex(value.weapon.instance_id),
                version: numberText(value.weapon.version), equipped: value.weapon.equipped, allowBound: value.allow_bound,
                session: hex(value.ui_session_id), quote: hex(value.quote_id), op: hex(value.operation?.op_id),
                revision: numberText(value.operation?.expected_revision), scene: numberText(value.scene_epoch) });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40050) {
                const value = pb.WeaponOilQuote.decode(frame.body);
                report.quotes.push({ request: frame.request, binding: frame.binding, quote: hex(value.quote_id), session: hex(value.ui_session_id),
                    scene: numberText(value.scene_epoch), weapon: hex(value.weapon.instance_id), oil: hex(value.oil.instance_id),
                    oilBound: value.oil.bound, equipped: value.equipped, allowBound: value.allow_bound, luck: value.current_luck,
                    successLuck: value.success_luck, failureLuck: value.failure_luck, chance: value.success_ppm,
                    guaranteed: value.guaranteed, pityUsed: value.pity_used, pityAfter: value.pity_after, outputBound: value.output_bound,
                    revision: numberText(value.role_revision) });
            } else if (frame?.id === 40005) {
                const value = pb.OperationResult.decode(frame.body);
                const receipt = value.weapon_oil;
                report.results.push({ request: frame.request, binding: frame.binding, op: hex(value.op_id), error: value.error_code, pending: value.pending,
                    ...(receipt ? { quote: hex(receipt.quote_id), weapon: hex(receipt.weapon.instance_id), oil: hex(receipt.oil.instance_id),
                        bound: receipt.weapon.bound, oilBound: receipt.oil.bound, version: numberText(receipt.weapon.version),
                        luck: receipt.weapon.current_luck, beforeLuck: receipt.before_luck, equipped: receipt.equipped, success: receipt.success,
                        guaranteed: receipt.guaranteed, pityUsed: receipt.pity_used, pityAfter: receipt.pity_after } : {}) });
            }
        });
    });
}

// 仅点击当前列表实际可见的原实例行或动作行，不直接调用游戏中的按钮回调。
async function touchListRow(page, panel, key) {
    const point = await wait(page, ({ panel, key }) => {
        const list = cc.find(panel + '/List')?.getComponent('List');
        if (!list?.node.activeInHierarchy) return null;
        const id = row => Array.from(row.instanceId ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
        const index = list.items.findIndex(row => row.key === key || id(row) === key);
        const node = list.rows.get(index);
        if (!node?.activeInHierarchy) return null;
        const row = node._uiProps.uiTransformComp.getBoundingBoxToWorld(), mask = list.viewport._uiProps.uiTransformComp.getBoundingBoxToWorld();
        const x = row.x + row.width / 2, y = row.y + row.height / 2;
        if (x < mask.x || x > mask.x + mask.width || y < mask.y || y > mask.y + mask.height) return null;
        const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
        return { x: canvas.left + x * canvas.width / size.width, y: canvas.top + (size.height - y) * canvas.height / size.height };
    }, { panel, key });
    await page.touchscreen.tap(point.x, point.y);
}

async function dismiss(page, node) {
    if (!await page.evaluate(node => !!cc.find(node)?.activeInHierarchy, node)) return;
    await page.touchscreen.tap(10, 130);
    await wait(page, node => !cc.find(node)?.activeInHierarchy, node);
}

async function detailRows(page, panel = details) {
    return page.evaluate(panel => cc.find(panel + '/List').getComponent('List').items, panel);
}

// 用真实滑动把关键值带入已保存列表的遮罩，保证截图表现与观测一致。
async function showRows(page, context, panel, wanted) {
    let input;
    for (let attempt = 0; attempt < 8; attempt++) {
        const where = await page.evaluate(({ panel, wanted }) => {
            const list = cc.find(panel + '/List').getComponent('List');
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
            return { visible, start: point(mask.y + mask.height * .33), end: point(mask.y + mask.height * .62) };
        }, { panel, wanted });
        if (where.visible) return;
        input ??= await context.newCDPSession(page);
        await input.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [where.start] });
        for (let step = 1; step <= 5; step++) {
            const y = where.start.y + (where.end.y - where.start.y) * step / 5;
            await input.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: where.start.x, y }] });
        }
        await input.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
        await page.waitForTimeout(230);
    }
    assert.fail('关键详情行没有实际滚入遮罩：' + wanted.join('、'));
}

async function checkBag(page, report) {
    await touch(page, safe + 'BottomHUD/Footer/Inventory');
    await touchListRow(page, inventory, expected.bag.instance_id);
    await label(page, details + '/Title', expected.weaponName);
    await label(page, details + '/Action1/Label', '穿戴装备');
    const choices = await detailRows(page);
    const entry = choices.some(row => row.key === 'weapon-oil' && row.label === '祝福油培养');
    assert.equal(entry, expected.enabled, '正常背包装备详情的培养入口显示错误');
    if (expected.enabled) {
        await touchListRow(page, details, 'weapon-oil');
        await label(page, details + '/Title', '武器祝福油');
        assert((await detailRows(page)).includes('背包 · 幸运+0'), '正常背包入口没有选择原背包武器');
    } else {
        report.hidden.bag = true;
        await page.screenshot({ path: path.join(output, '01-default-bag.png') });
    }
    await dismiss(page, details);
    await dismiss(page, inventory);
}

async function openWorn(page) {
    await touch(page, profile + '/EquipmentBoard/Slot0');
    if (expected.enabled) {
        await label(page, details + '/Title', expected.weaponName);
        await label(page, details + '/Action1/Label', '祝福油培养');
        await touch(page, details + '/Action1');
        await label(page, details + '/Title', '武器祝福油');
    } else {
        await label(page, status + '/Title', expected.weaponName);
        const actionPanelVisible = await page.evaluate(details => cc.find(details)?.activeInHierarchy, details);
        assert(!actionPanelVisible, '默认包已穿武器出现培养动作详情');
        assert(!(await detailRows(page, status)).some(row => String(row).includes('祝福油培养')));
    }
}

async function checkStatus(page, session, attackPPM, labelAttack) {
    await touch(page, profile + '/Recycle');
    await label(page, status + '/Title', '战斗属性');
    return wait(page, ({ session, status, attackPPM, labelAttack }) => {
        const attrs = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle)?.combatAttributes;
        const lines = cc.find(status + '/List')?.getComponent('List')?.items;
        if (!attrs?.luckEnabled || attrs.luckAttackPpm !== attackPPM || attrs.luckResistancePpm !== 200000 ||
            !lines?.includes(labelAttack) || !lines.includes('幸运抵抗 20%')) return null;
        return { attackPPM: attrs.luckAttackPpm, resistancePPM: attrs.luckResistancePpm, lines: lines.filter(line => /^幸运(?:一击|抵抗) /.test(line)) };
    }, { session, status, attackPPM, labelAttack });
}

async function hasQuoteButton(page, enabled) {
    await label(page, details + '/Action1/Label', '查看报价');
    await wait(page, ({ details, enabled }) => cc.find(details + '/Action1')?.getComponent(cc.Button)?.interactable === enabled, { details, enabled });
}

// 在第二个确认框取消，再重新明确两次授权。授权本身不发送任何祝福油协议。
async function authorize(page, report) {
    const openFirst = async () => {
        await touch(page, details + '/Action2');
        await label(page, confirm + '/Title', '开启绑定祝福油');
        await label(page, confirm + '/ConfirmButton/Label', '继续开启');
        await touch(page, confirm + '/ConfirmButton');
        await label(page, confirm + '/Title', '再次确认绑定投入');
        await label(page, confirm + '/ConfirmButton/Label', '确认开启');
    };
    await hasQuoteButton(page, false);
    await openFirst();
    await touch(page, confirm + '/CancelButton');
    await label(page, details + '/Action2/Label', '开启绑材');
    await hasQuoteButton(page, false);
    report.cancelled = (await detailRows(page)).includes('使用绑定资源 未开启');
    assert(report.cancelled);
    assert.deepEqual(report.requests, []);
    await openFirst();
    await touch(page, confirm + '/ConfirmButton');
    await label(page, details + '/Action2/Label', '关闭绑材');
    await hasQuoteButton(page, true);
    report.authorized = (await detailRows(page)).includes('使用绑定资源 已开启');
    assert(report.authorized);
    assert.deepEqual(report.requests, []);
}

async function useOnce(page, session, context, report) {
    await touch(page, details + '/Action1');
    await label(page, details + '/Title', '祝福油报价');
    await label(page, details + '/Action1/Label', '使用祝福油');
    const quoteLines = await detailRows(page);
    assert(quoteLines.includes('成功率 100%') && quoteLines.includes('累计耗油 100/100瓶') && quoteLines.includes('本瓶保底，逐瓶升一级') &&
        quoteLines.includes('装备 非绑定 → 绑定') && quoteLines.includes('消耗1瓶绑定' + expected.oilName));
    await showRows(page, context, details, ['成功率 100%', '累计耗油 100/100瓶']);
    await page.screenshot({ path: path.join(output, '02-quote.png') });
    assert.deepEqual(report.requests.map(value => value.id), [40049]);
    assert.equal(report.quotes.length, 1);
    const request = report.requests[0], quote = report.quotes[0];
    assert.deepEqual([request.instance, request.version, request.equipped, request.allowBound, request.binding, request.scene],
        [expected.worn.instance_id, String(expected.worn.version), true, true, report.world.binding, report.world.scene]);
    assert.match(request.session, /^[0-9a-f]{32}$/);
    assert.match(quote.quote, /^[0-9a-f]{32}$/);
    assert.deepEqual([quote.request, quote.binding, quote.scene, quote.session, quote.weapon, quote.oil, quote.oilBound, quote.equipped, quote.allowBound],
        [request.request, request.binding, request.scene, request.session, expected.worn.instance_id, expected.oil.instance_id, true, true, true]);
    assert.deepEqual([quote.luck, quote.successLuck, quote.failureLuck, quote.chance, quote.pityUsed, quote.pityAfter, quote.guaranteed, quote.outputBound],
        [3, 4, 3, 1000000, 100, 100, true, true]);
    await touch(page, details + '/Action1');
    await label(page, details + '/Title', '祝福油结算');
    const lines = await detailRows(page);
    assert(lines.includes('成功 · 幸运+3 → +4') && lines.includes('已扣1瓶绑定' + expected.oilName) && lines.includes('累计耗油 101/100瓶') &&
        lines.includes('结算后装备绑定'));
    assert.deepEqual(report.requests.map(value => value.id), [40049, 40051]);
    const commit = report.requests[1];
    assert.match(commit.op, /^[0-9a-f]{32}$/);
    assert.deepEqual([commit.quote, commit.binding, commit.instance, commit.version, commit.equipped, commit.allowBound, commit.session, commit.revision],
        [quote.quote, quote.binding, request.instance, request.version, true, true, request.session, quote.revision]);
    const result = report.results.find(value => value.op === commit.op);
    assert(result, '缺少同一笔祝福油永久操作结果');
    assert.deepEqual([result.request, result.binding, result.error, result.pending, result.quote, result.weapon, result.oil, result.version],
        [commit.request, commit.binding, 0, false, quote.quote, expected.worn.instance_id, expected.oil.instance_id, String(expected.worn.version + 1)]);
    assert.deepEqual([result.bound, result.oilBound, result.beforeLuck, result.luck, result.equipped, result.success, result.guaranteed, result.pityUsed, result.pityAfter],
        [true, true, 3, 4, true, true, true, 101, 100]);
    report.inventory = await wait(page, ({ session, expected }) => {
        const id = item => Array.from(item?.instanceId ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
        const worn = session.inventory?.equipped.find(row => row.slot === 0)?.item;
        const oil = session.inventory?.items.find(item => id(item) === expected.oil.instance_id);
        if (!worn?.bound || id(worn) !== expected.worn.instance_id || worn.currentLuck !== 4 || worn.initialLuck !== 3 || !oil?.bound || oil.quantity.toString() !== '1') return null;
        return { weapon: id(worn), oil: id(oil), remaining: oil.quantity.toString(), initialLuck: worn.initialLuck, currentLuck: worn.currentLuck };
    }, { session, expected });
    report.commit = commit;
    report.result = result;
    report.quoteLines = quoteLines;
    report.resultLines = lines;
    await page.screenshot({ path: path.join(output, '03-result.png') });
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, fixtureSha256: expected.fixtureSha256, clientBundleSha256: expected.clientBundleSha256,
        viewport: { width: 390, height: 844 }, sockets: 0, errors: [], requests: [], quotes: [], results: [], hidden: {} };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: report.viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const asset = page.waitForResponse(response => response.ok() && new URL(response.url()).pathname === '/assets/main/index.js', { timeout: 65000 });
        void asset.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_OIL_BROWSER_ACCOUNT, password: process.env.MINIMIR_OIL_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await asset).body()).digest('hex'), expected.clientBundleSha256);
        report.world = await originalWorld(page, session);
        assert.equal(report.world.map, String(expected.normalMap));
        assert.equal(report.world.rule, String(expected.ruleVersion));
        assert.equal(await page.evaluate(session => session.role.job, session), 1);
        if (report.world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        await checkBag(page, report);
        await touch(page, safe + 'BottomHUD/QuickNav/Role');
        if (expected.enabled) {
            report.before = await checkStatus(page, session, 44000, '幸运一击 4.4%');
            await dismiss(page, status);
        }
        await openWorn(page);
        if (!expected.enabled) {
            report.hidden.worn = true;
            await page.screenshot({ path: path.join(output, '02-default-worn.png') });
            assert.deepEqual(report.requests, []);
            assert.deepEqual(report.quotes, []);
        } else {
            assert((await detailRows(page)).includes('穿戴中 · 幸运+3'));
            await showRows(page, context, details, ['使用绑定资源 未开启']);
            await page.screenshot({ path: path.join(output, '01-bound-off.png') });
            await authorize(page, report);
            await useOnce(page, session, context, report);
            await dismiss(page, details);
            report.after = await checkStatus(page, session, 60000, '幸运一击 6%');
            await showRows(page, context, status, report.after.lines);
            await page.screenshot({ path: path.join(output, '04-stats.png') });
            await dismiss(page, status);
            await openWorn(page);
            await hasQuoteButton(page, false);
            const reopened = await detailRows(page);
            report.reset = reopened.includes('使用绑定资源 未开启') && reopened.includes('穿戴中 · 幸运+4') && reopened.includes('绑定油 1瓶');
            assert(report.reset, '关闭重开没有重置绑定开关或同步结算后的原资产');
            await showRows(page, context, details, ['使用绑定资源 未开启']);
            await page.screenshot({ path: path.join(output, '05-reopen.png') });
            assert.deepEqual(report.requests.map(value => value.id), [40049, 40051]);
        }
        assert.equal(report.sockets, 1);
        assert.deepEqual(report.errors, []);
        report.passed = true;
    } catch (error) {
        report.error = error.stack || String(error);
        await page.screenshot({ path: path.join(output, 'failure.png') }).catch(() => {});
        throw error;
    } finally {
        fs.writeFileSync(path.join(output, 'weapon-oil-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, hidden: report.hidden, cancelled: report.cancelled,
        authorized: report.authorized, reset: report.reset, before: report.before?.lines, after: report.after?.lines, screenshots: output }));
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
