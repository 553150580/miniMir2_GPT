// 用真实 Creator 手机触屏验证装备洗炼，并保留包摘要、原操作和实发回执。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const args = process.argv.slice(2);
assert.deepEqual([args.length, args[0], args[2]], [4, '--manifest', '--output']);
const manifest = path.resolve(args[1]), output = fs.realpathSync(path.resolve(args[3]));
assert(output.startsWith(fs.realpathSync('.work') + path.sep), '证据只写本轮工作目录');
// 公共触屏工具从第一个位置参数读取同一份已经隔离的清单。
process.argv = [process.argv[0], process.argv[1], manifest];
const { expected, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');
const base = new URL(expected.baseUrl);
const expectedGamePath = process.env.MINIMIR_EQUIPMENT_REFINE_BROWSER_GAME_PATH;
const expectedGameSha = process.env.MINIMIR_EQUIPMENT_REFINE_BROWSER_GAME_SHA256;
if (expectedGamePath || expectedGameSha) {
    assert.match(expectedGamePath ?? '', /^assets\/main\/import\/[a-f0-9]{2}\/[a-f0-9]+\.[a-f0-9]{16}\.json$/u);
    assert.match(expectedGameSha ?? '', /^[a-f0-9]{64}$/u);
}

const inventory = safe + 'UIMgr/Panels/Inventory';
const details = safe + 'PopupView/Panels/ItemActions';
const board = safe + 'PopupView/Panels/EquipmentRefine';
const materials = safe + 'PopupView/Panels/EquipmentRefineMaterials';
const confirm = safe + 'PopupView/Panels/Confirm';
const hex = value => Buffer.from(value ?? []).toString('hex');
const text = value => value?.toString() ?? '0';
const funds = body => [body.gold, body.bound_gold, body.yuanbao_units, body.bound_yuanbao_units].map(text);
const spent = rows => rows.map(row => ({ id: hex(row.item?.instance_id), bound: row.item?.bound, quantity: text(row.quantity),
    available: text(row.item?.quantity), version: text(row.item?.version) }));

function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href || ['18080', '18101', '18103'].includes(endpoint.port)) {
            report.errors.push('触屏连接未走独立游戏节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40058) {
                const body = pb.EquipmentRefineQuoteRequest.decode(frame.body);
                report.requests.push({ id: frame.id, request: frame.request, binding: frame.binding, scene: text(body.scene_epoch),
                    target: hex(body.target?.instance_id), version: text(body.target?.version), equipped: body.target?.equipped,
                    session: hex(body.ui_session_id), allowed: body.allow_bound, locks: body.locked_slots,
                    stones: body.stones.map(row => ({ id: hex(row.instance_id), version: text(row.version), quantity: text(row.quantity) })) });
            }
            if (frame?.id === 40060) {
                const body = pb.EquipmentRefineActionRequest.decode(frame.body);
                report.requests.push({ id: frame.id, request: frame.request, binding: frame.binding, action: body.action,
                    target: hex(body.target?.instance_id), version: text(body.target?.version), equipped: body.target?.equipped,
                    session: hex(body.ui_session_id), allowed: body.allow_bound, quote: hex(body.quote_id), pending: hex(body.pending_id),
                    op: hex(body.operation?.op_id), sequence: text(body.operation?.command_seq), revision: text(body.operation?.expected_revision),
                    at: Date.now(), body: Buffer.from(frame.body).toString('base64') });
            }
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40059) {
                const quote = pb.EquipmentRefineQuote.decode(frame.body);
                report.quotes.push({ request: frame.request, binding: frame.binding, target: hex(quote.target?.instance_id),
                    version: text(quote.target?.version), equipped: quote.equipped, session: hex(quote.ui_session_id),
                    scene: text(quote.scene_epoch), quote: hex(quote.quote_id), allowed: quote.allow_bound, bound: quote.output_bound, locks: quote.locked_slots,
                    revision: text(quote.role_revision), funds: funds(quote), stones: spent(quote.stones),
                    at: Date.now(), body: Buffer.from(frame.body).toString('base64') });
            }
            if (frame?.id !== 40005 || !report.requests.some(request => request.id === 40060 && request.request === frame.request)) return;
            const result = pb.OperationResult.decode(frame.body), reply = result.equipment_refine;
            if (!reply) {
                report.errors.push(`原洗炼回执未确认 code=${result.error_code}`);
                return;
            }
            report.receipts.push({ request: frame.request, binding: frame.binding, op: hex(result.op_id), error: result.error_code,
                waiting: result.pending, revision: text(result.new_revision), action: reply.action, target: hex(reply.target?.instance_id),
                itemVersion: text(reply.target?.version), bound: reply.target?.bound, quote: hex(reply.quote_id), pending: hex(reply.pending_id),
                stones: spent(reply.stones), funds: funds(reply), at: Date.now(), body: Buffer.from(frame.body).toString('base64') });
        });
    });
}

// 触屏失败时只读取页面与网络证据，避免修改弹窗或代替玩家执行确认。
async function captureFailure(page) {
    return page.evaluate(async () => {
        const controller = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
        const { session } = await System.import('chunks:///_virtual/GameSession.ts');
        const view = controller?.equipmentRefine?.view;
        const key = value => Array.from(value ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
        const bottom = cc.find('Canvas/Safe/Status')?.getComponent(cc.Label)?.string;
        const rows = cc.find('Canvas/Safe/PopupView/Panels/EquipmentRefineMaterials/List')?.getComponent('List')?.items;
        const stored = session.role && session.pending?.get(`${session.role.key.originServerId}:${key(session.role.key.roleId)}`);
        return { scene: cc.director.getScene()?.name, top: controller?.popup?.top?.name, bottom, lastError: session.lastError,
            busy: controller?.economicBusy, unresolved: controller?.unresolved, stored: stored?.id, ready: session.ready,
            materialText: rows?.filter(row => typeof row === 'string'), inventoryRevision: session.inventory?.revision?.toString(),
            view: view ? { role: view.role, scope: view.scope, item: key(view.id), session: key(view.uiSessionId), notice: view.notice,
                working: view.working, equipped: view.equipped, quote: key(view.review?.quote?.quoteId), action: view.action?.action,
                receipt: view.receipt?.action } : null };
    });
}

// 只触摸已经进入保存遮罩的真实列表行。不可见行先用手指滑到可见区。
async function tapRow(page, panel, key) {
    let input;
    try {
        for (let attempt = 0; attempt < 10; attempt++) {
            const visible = await wait(page, ({ panel, key }) => {
                const list = cc.find(panel + '/List')?.getComponent('List');
                if (!list?.node.activeInHierarchy || !list.rows.size) return null;
                const id = value => Array.from(value ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
                const index = list.items.findIndex(row => row?.key === key || id(row?.instanceId) === key);
                if (index < 0) return null;
                const toScreen = (x, y) => {
                    const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
                    return { x: canvas.left + x * canvas.width / size.width, y: canvas.top + (size.height - y) * canvas.height / size.height };
                };
                const row = list.rows.get(index), clip = list.viewport._uiProps.uiTransformComp;
                const bottom = -clip.height * clip.anchorY, top = bottom + clip.height;
                let above = index < Math.min(...Array.from(list.rows.keys()));
                if (row?.activeInHierarchy) {
                    const item = row._uiProps.uiTransformComp;
                    const point = item.convertToWorldSpaceAR(new cc.Vec3(item.width * (.5 - item.anchorX), item.height * (.5 - item.anchorY), 0));
                    const position = clip.convertToNodeSpaceAR(point);
                    if (position.y > bottom + 4 && position.y < top - 4) return { point: toScreen(point.x, point.y) };
                    above = position.y > top - 4;
                }
                const start = clip.convertToWorldSpaceAR(new cc.Vec3(0, bottom + clip.height * (above ? .74 : .26)));
                const end = clip.convertToWorldSpaceAR(new cc.Vec3(0, bottom + clip.height * (above ? .26 : .74)));
                return { start: toScreen(start.x, start.y), end: toScreen(end.x, end.y) };
            }, { panel, key });
            if (visible.point) {
                await page.touchscreen.tap(visible.point.x, visible.point.y);
                return;
            }
            input ??= await page.context().newCDPSession(page);
            await input.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [visible.start] });
            for (let n = 1; n <= 5; ++n) {
                const y = visible.start.y + (visible.end.y - visible.start.y) * n / 5;
                await input.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: visible.start.x, y }] });
            }
            await input.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
            await page.waitForTimeout(220);
        }
        assert.fail('遮罩内未找到实际可点击的原列表项目');
    } finally { await input?.detach(); }
}

async function getAssets(page, session, revision) {
    return wait(page, ({ session, revision }) => {
        const inv = session.inventory, wallet = session.wallet;
        if (!inv || !wallet || revision && (BigInt(inv.revision.toString()) < BigInt(revision) ||
            BigInt(wallet.revision.toString()) < BigInt(revision))) return null;
        const hex = id => Array.from(id ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
        const affixes = rows => (rows ?? []).map(row => ({ slot: row.slot, attribute: row.attributeId, tier: row.tier, value: row.valueUnits.toString() }));
        const item = row => ({ id: hex(row.instanceId), itemId: row.itemId, bound: row.bound, version: row.version.toString(),
            quantity: row.quantity.toString(), durability: row.durabilityU?.toString(), sourceId: row.sourceId, sourceKind: row.sourceKind,
            affixes: affixes(row.refineAffixes), pending: row.refinePending ? { id: hex(row.refinePending.pendingId),
                affixes: affixes(row.refinePending.affixes), locks: row.refinePending.lockedSlots } : null });
        return { revision: inv.revision.toString(), level: inv.level, items: inv.items.map(item),
            equipped: inv.equipped.map(row => ({ slot: row.slot, ...item(row.item) })),
            wallet: Object.fromEntries(['gold', 'boundGold', 'yuanbaoUnits', 'boundYuanbaoUnits'].map(key => [key, wallet[key].toString()])) };
    }, { session, revision });
}

async function detailLine(page, panel, content) {
    return wait(page, ({ panel, content }) => {
        const list = cc.find(panel + '/List')?.getComponent('List');
        return list?.node.activeInHierarchy && list.items.some(row => typeof row === 'string' && row.includes(content));
    }, { panel, content });
}

async function bindingPrompt(page, accept) {
    await touch(page, board + '/BoundToggle');
    await label(page, confirm + '/Title', '开启绑定资源');
    await label(page, confirm + '/ConfirmButton/Label', '继续开启');
    await touch(page, confirm + '/ConfirmButton');
    await label(page, confirm + '/Title', '确认装备归属');
    await label(page, confirm + '/Message', '装备立即绑定当前角色');
    await touch(page, confirm + (accept ? '/ConfirmButton' : '/CancelButton'));
    await label(page, board + '/BoundToggle/Label', '绑定资源：' + (accept ? '开' : '关'));
}

async function openRefine(page) {
    await tapRow(page, details, 'equipment-refine');
    await label(page, board + '/Title', '装备洗炼');
    await label(page, board + '/Equipment', expected.weaponName);
}

async function quoteOnTouch(page, report, originalStones, bound, wallet) {
    await touch(page, board + '/MaterialButton');
    await label(page, materials + '/Title', '选择原洗炼石');
    for (const stone of originalStones) await tapRow(page, materials, stone.instance_id);
    await detailLine(page, materials, '已选 2/2枚');
    await label(page, materials + '/Action1/Label', '查看报价');
    await touch(page, materials + '/Action1');
    await label(page, materials + '/Title', '确认洗炼报价');
    await detailLine(page, materials, '结算后绑定本角色');
    const request = report.requests.filter(row => row.id === 40058).at(-1);
    const quote = report.quotes.find(row => row.request === request?.request);
    assert(quote, '真实触屏操作缺少原报价');
    assert.deepEqual([request.target, request.scene, request.binding, request.allowed, request.equipped],
        [expected.target.instance_id, report.world.scene, report.world.binding, bound, false]);
    assert.deepEqual([quote.target, quote.scene, quote.binding, quote.allowed, quote.session, quote.bound],
        [request.target, request.scene, request.binding, request.allowed, request.session, true]);
    assert.deepEqual(quote.funds, wallet);
    assert.equal(quote.stones.length, 2);
    assert.deepEqual(quote.stones.map(row => [row.id, row.bound, row.quantity]).sort(),
        originalStones.map(row => [row.instance_id, bound, '1']).sort());
    return quote;
}

async function actionResult(page, report, afterCount, action, quote, pending, fundsPaid) {
    await label(page, board + '/Title', '装备洗炼');
    const sends = report.requests.filter(row => row.id === 40060);
    assert.equal(sends.length, afterCount);
    const sent = sends.at(-1), reply = report.receipts.find(row => row.request === sent.request);
    assert(reply, '真实触屏操作缺少原结算');
    assert.match(sent.op, /^[a-f0-9]{32}$/u);
    assert.deepEqual([sent.action, sent.target, sent.quote, sent.pending, reply.action, reply.target, reply.quote, reply.op, reply.error, reply.waiting],
        [action, expected.target.instance_id, quote?.quote ?? '', pending, action, expected.target.instance_id, quote?.quote ?? '', sent.op, 0, false]);
    assert.deepEqual(reply.funds, fundsPaid);
    if (quote) {
        assert.match(reply.pending, /^[a-f0-9]{32}$/u);
        assert.equal(sent.revision, quote.revision);
        assert.deepEqual(reply.stones, quote.stones);
    } else {
        assert.equal(reply.pending, pending);
        assert.deepEqual(reply.stones, []);
    }
    report.operations.push({ op: sent.op, action, quote: sent.quote, pending: sent.pending, resultPending: reply.pending, revision: sent.revision });
    return reply;
}

async function decide(page, apply) {
    const name = apply ? '采纳候选' : '放弃候选';
    await touch(page, board + (apply ? '/ApplyButton' : '/DiscardButton'));
    await label(page, confirm + '/Title', name);
    await touch(page, confirm + '/ConfirmButton');
}

async function candidate(page, session, report, before) {
    const target = state => state.items.find(item => item.id === expected.target.instance_id);
    const cost = expected.goldCost, zero = ['0', '0', '0', '0'];
    await openRefine(page);
    await label(page, board + '/Current/Row1/Value', '空词条');
    await label(page, board + '/Candidate/Row1/Value', '待生成');
    report.leftEmpty = true;
    await page.screenshot({ path: path.join(output, '01-refine-before.png') });
    await bindingPrompt(page, false);
    report.bindingCancelled = true;
    await bindingPrompt(page, true);
    report.bindingGranted = true;

    const first = await quoteOnTouch(page, report, expected.boundStones, true, ['0', cost, '0', '0']);
    await page.screenshot({ path: path.join(output, '02-refine-bound-quote.png') });
    await touch(page, materials + '/Action1');
    await label(page, board + '/Current/Row1/Value', '空词条');
    await label(page, board + '/Candidate/Row1/Value', '档');
    report.rightVisible = true;
    const one = await actionResult(page, report, 1, 1, first, '', ['0', cost, '0', '0']);
    const afterOne = await getAssets(page, session, one.revision);
    const paid = target(afterOne);
    report.immediateBound = paid?.bound === true && paid.pending?.id === one.pending && paid.affixes.length === 0;
    assert(report.immediateBound);
    assert.equal(afterOne.wallet.gold, before.wallet.gold);
    assert.equal(BigInt(before.wallet.boundGold) - BigInt(afterOne.wallet.boundGold), BigInt(cost));
    assert(expected.boundStones.every(stone => !afterOne.items.some(item => item.id === stone.instance_id)));
    await page.screenshot({ path: path.join(output, '03-refine-generated.png') });

    await decide(page, false);
    await label(page, board + '/Candidate/Row1/Value', '待生成');
    await label(page, board + '/Current/Row1/Value', '空词条');
    const two = await actionResult(page, report, 2, 3, null, one.pending, zero);
    const afterTwo = await getAssets(page, session, two.revision);
    assert.deepEqual(afterTwo.wallet, afterOne.wallet);
    report.discardPreserved = target(afterTwo).bound && !target(afterTwo).pending && target(afterTwo).affixes.length === 0;
    assert(report.discardPreserved);
    await page.screenshot({ path: path.join(output, '04-refine-discarded.png') });

    // 返回原详情，再进入同一原件核对新面板会话默认关闭绑定。
    await page.touchscreen.tap(10, 130);
    await label(page, details + '/Title', expected.weaponName);
    await openRefine(page);
    await label(page, board + '/BoundToggle/Label', '绑定资源：关');
    report.reopenBoundOff = true;
    const second = await quoteOnTouch(page, report, expected.freeStones, false, [cost, '0', '0', '0']);
    report.secondQuoteNonbound = true;
    assert.notEqual(second.session, first.session);
    await page.screenshot({ path: path.join(output, '05-refine-free-quote.png') });
    await touch(page, materials + '/Action1');
    const preview = await label(page, board + '/Candidate/Row1/Value', '档');
    const three = await actionResult(page, report, 3, 1, second, '', [cost, '0', '0', '0']);
    assert.notEqual(three.pending, one.pending);
    const afterThree = await getAssets(page, session, three.revision);
    const affixes = target(afterThree).pending?.affixes;
    assert.equal(affixes?.length, 1);
    await decide(page, true);
    await label(page, board + '/Current/Row1/Value', preview);
    await label(page, board + '/Candidate/Row1/Value', '待生成');
    const four = await actionResult(page, report, 4, 2, null, three.pending, zero);
    const afterFour = await getAssets(page, session, four.revision), final = target(afterFour);
    assert.deepEqual(afterFour.wallet, afterThree.wallet);
    assert.equal(BigInt(before.wallet.gold) - BigInt(afterFour.wallet.gold), BigInt(cost));
    assert.equal(afterFour.wallet.yuanbaoUnits, before.wallet.yuanbaoUnits);
    assert.equal(afterFour.wallet.boundYuanbaoUnits, before.wallet.boundYuanbaoUnits);
    assert(expected.freeStones.every(stone => !afterFour.items.some(item => item.id === stone.instance_id)));
    assert.deepEqual([final.itemId, final.id, final.durability, final.quantity, final.sourceId, final.sourceKind],
        [target(before).itemId, target(before).id, target(before).durability, target(before).quantity, target(before).sourceId, target(before).sourceKind]);
    assert.deepEqual(final.affixes, affixes);
    assert(final.bound && !final.pending);
    report.adopted = true;
    await page.screenshot({ path: path.join(output, '06-refine-adopted.png') });
}

async function main() {
    assert(typeof expected.enabled === 'boolean' && /^[a-f0-9]{64}$/u.test(expected.clientBundleSha256));
    const report = { passed: false, mode: expected.mode, enabled: expected.enabled, fixtureSha256: expected.fixtureSha256,
        clientBundleSha256: expected.clientBundleSha256, viewport: { width: 390, height: 844 }, sockets: 0,
        errors: [], requests: [], quotes: [], receipts: [], operations: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: report.viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const bundle = page.waitForResponse(response => response.ok() && new URL(response.url()).pathname === '/assets/main/index.js', { timeout: 65_000 });
        void bundle.catch(() => {});
        // 在冷启动前监听，核对本次触屏实际下载的版本化 Game 场景合包。
        const game = expectedGamePath ? page.waitForResponse(response => {
            const url = new URL(response.url());
            return response.status() === 200 && url.origin === base.origin && url.pathname === '/' + expectedGamePath;
        }, { timeout: 65_000 }) : undefined;
        void game?.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_EQUIPMENT_REFINE_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_EQUIPMENT_REFINE_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.clientBundleSha256);
        if (game) {
            const response = await game;
            const sha256 = crypto.createHash('sha256').update(await response.body()).digest('hex');
            assert.equal(sha256, expectedGameSha, '浏览器实际收到的 Game 场景合包校验值不同');
            report.gamePack = { path: new URL(response.url()).pathname.slice(1), sha256 };
        }
        report.world = await originalWorld(page, session);
        assert.deepEqual([report.world.map, report.world.role, report.world.rule], [String(expected.normalMap), expected.role, String(expected.ruleVersion)]);
        if (report.world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        const before = await getAssets(page, session), weapon = before.items.find(row => row.id === expected.target.instance_id);
        assert.deepEqual([before.level, before.wallet, weapon?.itemId, weapon?.bound, weapon?.affixes, weapon?.pending],
            [expected.roleLevel, expected.wallet, expected.target.item_id, false, [], null]);
        await touch(page, safe + 'BottomHUD/Footer/Inventory');
        await tapRow(page, inventory, expected.target.instance_id);
        await label(page, details + '/Title', expected.weaponName);
        report.oldEquipmentVisible = true;
        if (expected.enabled) await candidate(page, session, report, before);
        else {
            assert.equal(await page.evaluate(({ details, board }) => cc.find(details + '/List').getComponent('List').items
                .some(row => row?.key === 'equipment-refine') || cc.find(board).activeInHierarchy, { details, board }), false);
            assert.deepEqual(await getAssets(page, session), before);
            assert.deepEqual(report.requests, []);
            report.defaultRefineHidden = true;
            await page.screenshot({ path: path.join(output, '01-default-equipment.png') });
        }
        const after = await originalWorld(page, session);
        for (const key of ['map', 'role', 'binding', 'scene', 'life']) assert.equal(after[key], report.world[key], key);
        assert.equal(report.sockets, 1);
        assert.deepEqual(report.errors, []);
        report.passed = true;
    } catch (error) {
        report.error = error.stack || String(error);
        report.clientFailure = await captureFailure(page).catch(error => ({ error: error.message }));
        await page.screenshot({ path: path.join(output, 'failure.png') }).catch(() => {});
        throw error;
    } finally {
        fs.writeFileSync(path.join(output, 'equipment-refine-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, operations: report.operations, screenshots: output }));
}

main().catch(error => { console.error(error.message); process.exitCode = 1; });
