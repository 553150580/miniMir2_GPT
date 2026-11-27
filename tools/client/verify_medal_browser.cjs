// 通过真实触屏、原 WebSocket 及两笔永久回执验收角色勋章。凭据只从环境读取。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const role = safe + 'UIMgr/Panels/Role';
const details = safe + 'PopupView/Panels/ItemActions';
const confirm = safe + 'PopupView/Panels/Confirm';
const gamePath = process.env.MINIMIR_MEDAL_BROWSER_GAME_PATH;
const gameHash = process.env.MINIMIR_MEDAL_BROWSER_GAME_SHA256;
const hex = value => Buffer.from(value ?? []).toString('hex');
const sha = value => crypto.createHash('sha256').update(value).digest('hex');
const fragment = row => ({ id: hex(row.instance_id), version: row.version.toString(), quantity: row.quantity.toString() });

function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) {
            report.errors.push('触屏验收连接了非隔离游戏节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40061 && frame?.id !== 40063) return;
            const base = { id: frame.id, request: frame.request, binding: frame.binding };
            if (frame.id === 40061) {
                const value = pb.MedalQuoteRequest.decode(frame.body);
                report.requests.push({ ...base, scene: value.scene_epoch.toString(), medalVersion: value.medal_version.toString(),
                    allow: value.allow_bound, session: hex(value.ui_session_id), fragments: value.fragments.map(fragment) });
            } else {
                const value = pb.MedalCommitRequest.decode(frame.body);
                report.requests.push({ ...base, quote: hex(value.quote_id), session: hex(value.ui_session_id),
                    op: hex(value.operation.op_id), revision: value.operation.expected_revision.toString() });
            }
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40062) {
                const value = pb.MedalQuote.decode(frame.body);
                report.quotes.push({ request: frame.request, binding: frame.binding, quote: hex(value.quote_id),
                    scene: value.scene_epoch.toString(), level: value.target_level, attack: value.target_main_attack.toString(),
                    defense: value.target_defense.toString(), revision: value.role_revision.toString(), allow: value.allow_bound,
                    session: hex(value.ui_session_id), fragments: value.fragments.map(row => ({ ...fragment(row), bound: row.bound })) });
            } else if (frame?.id === 40005) {
                const value = pb.OperationResult.decode(frame.body), medal = value.medal;
                if (!medal) return;
                report.receipts.push({ request: frame.request, binding: frame.binding, error: value.error_code, pending: value.pending,
                    op: hex(value.op_id), quote: hex(medal.quote_id), beforeLevel: medal.before_level, level: medal.medal.level,
                    version: medal.medal.version.toString(), fragments: medal.fragments.map(row => ({ ...fragment(row), bound: row.bound })) });
            }
        });
    });
}

// 勋章和原物品行都从已保存的 List 读取，点击限定在可见遮罩范围内。
async function listRow(page, context, panel, key, click) {
    let input;
    try {
        for (let attempt = 0; attempt < 8; attempt++) {
            const locate = await wait(page, ({ panel, key }) => {
                const list = cc.find(panel + '/List')?.getComponent('List');
                if (!list?.node.activeInHierarchy) return null;
                const bytes = value => Array.from(value ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
                const index = list.items.findIndex(row => key === 'medal' ? row.medal === true : row.key === key || bytes(row.instanceId) === key);
                if (index < 0) return null;
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
                    if (x >= mask.x && x <= mask.x + mask.width && y >= mask.y + 3 && y <= mask.y + mask.height - 3) {
                        return { point: point(x, y), text: node.getChildByName('Text')?.getComponent(cc.Label)?.string };
                    }
                    earlier = y > mask.y + mask.height;
                }
                const x = mask.x + mask.width / 2;
                return { start: point(x, mask.y + mask.height * (earlier ? .72 : .28)),
                    end: point(x, mask.y + mask.height * (earlier ? .28 : .72)) };
            }, { panel, key });
            if (locate.point) {
                if (click) await page.touchscreen.tap(locate.point.x, locate.point.y);
                return locate.text;
            }
            input ??= await context.newCDPSession(page);
            await input.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [locate.start] });
            for (let step = 1; step <= 5; step++) {
                const y = locate.start.y + (locate.end.y - locate.start.y) * step / 5;
                await input.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: locate.start.x, y }] });
            }
            await input.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
            await page.waitForTimeout(220);
        }
        assert.fail('保存列表没有出现原条目 ' + panel + '/' + key);
    } finally {
        await input?.detach();
    }
}

const rows = (page, panel = details) => page.evaluate(panel => cc.find(panel + '/List')?.getComponent('List')?.items ?? [], panel);
const screenshot = (page, name) => page.screenshot({ path: path.join(output, name + '.png') });

async function action(page, index, text) {
    await label(page, details + '/Action' + index + '/Label', text);
    await touch(page, details + '/Action' + index);
}

async function dismiss(page, panel) {
    await page.touchscreen.tap(10, 130);
    await wait(page, panel => !cc.find(panel)?.activeInHierarchy, panel);
}

async function readInventory(page, session, level) {
    return wait(page, ({ session, level }) => {
        const inv = session.inventory, wallet = session.wallet, player = session.baseline?.entities
            .find(row => row.entityHandle === session.entry?.playerHandle);
        if (!inv || !wallet || !player?.combatAttributes || level !== undefined && (inv.medal?.level ?? 0) !== level) return null;
        const id = value => Array.from(value ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
        const item = row => ({ id: id(row.instanceId), itemId: row.itemId, version: row.version.toString(),
            quantity: row.quantity.toString(), bound: row.bound });
        const names = ['physicalAttack', 'magicAttack', 'spiritAttack'];
        const attack = names[session.role.job], fields = [attack + 'Min', attack + 'Max', 'physicalDefenceMin', 'physicalDefenceMax'];
        return { level: inv.level, medal: inv.medal ? { level: inv.medal.level, version: inv.medal.version.toString() } : null,
            equipped: inv.equipped.map(row => ({ slot: row.slot, ...item(row.item) })), capacity: inv.capacity, items: inv.items.map(item),
            wallet: [wallet.boundGold, wallet.gold, wallet.boundYuanbaoUnits, wallet.yuanbaoUnits].map(value => value.toString()),
            attributes: fields.map(key => player.combatAttributes[key].toString()) };
    }, { session, level });
}

async function awaitAttributes(page, session, before, growth, defense) {
    const delta = [growth, growth, defense, defense].map(BigInt);
    const target = before.attributes.map((value, index) => (BigInt(value) + delta[index]).toString());
    await wait(page, ({ session, target }) => {
        const attrs = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle)?.combatAttributes;
        const attack = ['physicalAttack', 'magicAttack', 'spiritAttack'][session.role.job];
        return attrs && [attack + 'Min', attack + 'Max', 'physicalDefenceMin', 'physicalDefenceMax']
            .every((key, index) => attrs[key].toString() === target[index]);
    }, { session, target });
    return target;
}

async function boundPermission(page, approve) {
    await action(page, 2, '允许绑片');
    await label(page, confirm + '/Title', '允许绑定碎片');
    await label(page, confirm + '/ConfirmButton/Label', '继续开启');
    await touch(page, confirm + '/ConfirmButton');
    await label(page, confirm + '/Title', '再次确认绑定投入');
    await label(page, confirm + '/ConfirmButton/Label', '确认开启');
    await touch(page, confirm + (approve ? '/ConfirmButton' : '/CancelButton'));
    await label(page, details + '/Action2/Label', approve ? '关闭绑片' : '允许绑片');
}

function verifyMarginalReceipt(report, prior, level, cost, expectedFragments) {
    const request = report.requests.at(-2), commit = report.requests.at(-1), quote = report.quotes.at(-1), receipt = report.receipts.at(-1);
    assert.deepEqual([request.id, commit.id, request.medalVersion, quote.level, receipt.beforeLevel, receipt.level, receipt.version],
        [40061, 40063, String(prior), level, prior, level, String(level)]);
    assert.equal(request.scene, report.world.scene);
    assert.equal(request.binding, report.world.binding);
    assert.equal(request.request, quote.request);
    assert.equal(commit.request, receipt.request);
    assert.equal(quote.revision, commit.revision);
    for (const key of ['binding', 'session']) assert.equal(request[key], commit[key]);
    assert.deepEqual([quote.quote, commit.quote, receipt.quote], [receipt.quote, receipt.quote, receipt.quote]);
    assert.equal(commit.op, receipt.op);
    assert.equal(receipt.error, 0);
    assert.equal(receipt.pending, false);
    assert.deepEqual(quote.fragments, expectedFragments);
    assert.deepEqual(receipt.fragments, expectedFragments);
    assert.deepEqual(request.fragments, expectedFragments.map(({ bound: _, ...row }) => row));
    assert.equal(quote.fragments.reduce((sum, item) => sum + BigInt(item.quantity), 0n).toString(), cost);
    assert.deepEqual([quote.attack, quote.defense], [String(level), '1']);
}

async function candidate(page, context, session, report, before) {
    assert.deepEqual(expected.levelCosts, ['2', '3', '5']);
    const free = before.items.find(item => item.id === expected.freeFragment.instance_id);
    const bound = before.items.find(item => item.id === expected.boundFragment.instance_id);
    assert.deepEqual([free.itemId, free.quantity, free.bound, bound.itemId, bound.quantity, bound.bound],
        [expected.fragmentItemId, '1', false, expected.fragmentItemId, '10', true]);
    assert.equal(await listRow(page, context, role, 'medal', true), '勋章 · 未激活');
    await label(page, details + '/Title', '角色勋章');
    assert((await rows(page)).includes('下一目标 1级 · 2片'));
    await screenshot(page, '01-medal-overview');
    await action(page, 1, '选择碎片');
    await label(page, details + '/Title', '勋章碎片');
    assert(!(await rows(page)).some(row => row.key === bound.id));
    await boundPermission(page, false);
    assert(!(await rows(page)).some(row => row.key === bound.id));
    assert.deepEqual(report.requests, []);
    report.boundCancel = true;
    await boundPermission(page, true);
    report.boundConfirmed = true;
    await listRow(page, context, details, free.id, true);
    await listRow(page, context, details, bound.id, true);
    assert((await rows(page)).includes('选择 绑定 1片 · 非绑 1片'));
    await action(page, 1, '查看升级报价');
    await label(page, details + '/Title', '勋章升级报价');
    assert((await rows(page)).some(row => typeof row === 'string' && /^1\. 非绑碎片 1片/.test(row)));
    assert((await rows(page)).some(row => typeof row === 'string' && /^2\. 绑定碎片 1片/.test(row)));
    await screenshot(page, '02-medal-mixed-quote');
    await action(page, 1, '确认激活');
    await label(page, details + '/Title', '勋章激活完成');
    const first = await readInventory(page, session, 1);
    const remainder = first.items.find(item => item.id === bound.id);
    assert.equal(remainder.quantity, '9');
    assert(!first.items.some(item => item.id === free.id));
    verifyMarginalReceipt(report, 0, 1, '2', [{ id: free.id, version: free.version, quantity: '1', bound: false },
        { id: bound.id, version: bound.version, quantity: '1', bound: true }]);
    const firstAttributes = await awaitAttributes(page, session, before, 1, 1);
    await screenshot(page, '03-medal-activated');
    await dismiss(page, details);
    assert.equal(await listRow(page, context, role, 'medal', true), '勋章 · 1级');
    await label(page, details + '/Title', '角色勋章');
    assert((await rows(page)).includes('下一目标 2级 · 3片'));
    await action(page, 1, '选择碎片');
    await label(page, details + '/Action2/Label', '允许绑片');
    assert(!(await rows(page)).some(row => row.key === bound.id), '重开勋章后不能保留上一次授权');
    report.boundClosedOnReopen = true;
    await boundPermission(page, true);
    await listRow(page, context, details, bound.id, true);
    await action(page, 1, '查看升级报价');
    await action(page, 1, '确认升级');
    await label(page, details + '/Title', '勋章升级完成');
    const after = await readInventory(page, session, 2);
    assert.equal(after.items.find(item => item.id === bound.id)?.quantity, '6');
    assert.deepEqual([after.equipped, after.wallet, after.capacity], [before.equipped, before.wallet, before.capacity]);
    assert.deepEqual(after.items.filter(item => item.itemId !== expected.fragmentItemId),
        before.items.filter(item => item.itemId !== expected.fragmentItemId));
    verifyMarginalReceipt(report, 1, 2, '3', [{ id: bound.id, version: remainder.version, quantity: '3', bound: true }]);
    assert.notEqual(report.receipts[0].op, report.receipts[1].op);
    const secondAttributes = await awaitAttributes(page, session, before, 2, 1);
    report.attributes = { before: before.attributes, activated: firstAttributes, secondLevel: secondAttributes };
    await screenshot(page, '04-medal-upgraded');
    await action(page, 1, '返回勋章');
    await label(page, details + '/Title', '角色勋章');
    assert((await rows(page)).includes('角色需达 3级'));
    assert((await rows(page)).includes('下一目标 3级 · 5片'));
    assert.equal(await page.evaluate(node => cc.find(node).getComponent(cc.Button).interactable, details + '/Action1'), false);
    report.roleLevelCap = true;
    report.final = after.medal;
    await screenshot(page, '05-medal-role-cap');
}

async function main() {
    assert(typeof expected.enabled === 'boolean' && /^[a-f0-9]{64}$/.test(expected.clientBundleSha256));
    assert((!gamePath && !gameHash) || (/^assets\/main\/import\/[\da-f]{2}\/[\w.-]+\.json$/.test(gamePath) && /^[\da-f]{64}$/.test(gameHash)));
    const report = { passed: false, mode: expected.mode, enabled: expected.enabled, fixtureSha256: expected.fixtureSha256,
        clientBundleSha256: expected.clientBundleSha256, viewport: { width: 390, height: 844 }, sockets: 0, errors: [],
        requests: [], quotes: [], receipts: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: report.viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const base = new URL(expected.baseUrl);
        const asset = name => page.waitForResponse(response => {
            const url = new URL(response.url());
            return response.status() === 200 && url.origin === base.origin && url.pathname === '/' + name;
        }, { timeout: 65_000 });
        const bundle = asset('assets/main/index.js'), game = gamePath ? asset(gamePath) : undefined;
        void bundle.catch(() => {});
        void game?.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_MEDAL_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_MEDAL_BROWSER_PASSWORD });
        assert.equal(sha(await (await bundle).body()), expected.clientBundleSha256);
        if (game) {
            const response = await game, sha256 = sha(await response.body());
            assert.equal(sha256, gameHash);
            report.gamePack = { path: gamePath, sha256 };
        }
        report.world = await originalWorld(page, session);
        assert.deepEqual([report.world.map, report.world.role, report.world.rule],
            [String(expected.normalMap), expected.role, String(expected.ruleVersion)]);
        if (report.world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        const before = await readInventory(page, session, 0);
        assert.equal(before.level, expected.roleLevel);
        await touch(page, safe + 'BottomHUD/QuickNav/Role');
        assert((await listRow(page, context, role, expected.weapon.instance_id, false)).includes(expected.weaponName));
        report.oldEquipmentVisible = true;
        if (expected.enabled) await candidate(page, context, session, report, before);
        else {
            assert(!(await rows(page, role)).some(row => row.medal === true));
            assert.deepEqual(await readInventory(page, session, 0), before);
            report.defaultEntryHidden = true;
            await screenshot(page, '01-default-role');
        }
        assert.deepEqual([report.requests.length, report.quotes.length, report.receipts.length], expected.enabled ? [4, 2, 2] : [0, 0, 0]);
        const after = await originalWorld(page, session);
        for (const key of ['map', 'role', 'binding', 'scene', 'life']) assert.equal(after[key], report.world[key], key);
        assert.equal(report.sockets, 1);
        assert.deepEqual(report.errors, []);
        report.passed = true;
    } catch (error) {
        report.error = error.stack || String(error);
        await screenshot(page, 'failure').catch(() => {});
        throw error;
    } finally {
        fs.writeFileSync(path.join(output, 'medal-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, operations: report.receipts.length, output }));
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
