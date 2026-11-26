// 用真实 Creator 保存界面触屏验证隔离矿图、矿石纯度、NPC 收购和默认关闭。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');

const directory = safe + 'UIMgr/Panels/TownDirectory';
const details = safe + 'PopupView/Panels/StatusDetails';
const inventory = safe + 'UIMgr/Panels/Inventory';
const itemDetails = safe + 'PopupView/Panels/ItemActions';
const confirm = safe + 'PopupView/Panels/Confirm';
const shop = safe + 'UIMgr/Panels/NpcShop';
const action = safe + 'MapViewport/Actions/Auto';
const hex = bytes => Buffer.from(bytes).toString('hex');
const uiLabel = (page, nodePath) => page.evaluate(nodePath => cc.find(nodePath)?.getComponent(cc.Label)?.string, nodePath);
const originalStyle = require('../../client/assets/resources/development/map-styles/manifest.json').styles.find(row => row.map_id === '7')?.frame;

// 状态报告保留真实 WebSocket 观察，Long 用字符串或受控断言后的小整数表示。
function observation(value) {
    return { scene: value.scene_epoch.toString(), sequence: value.state_sequence.toString(), map: value.map_id.toString(),
        active: value.active, canStart: value.can_start, error: value.error_code,
        attempts: Number(value.attempts.toString()), successes: Number(value.successes.toString()),
        tool: value.tool && { item: value.tool.item_id, instance: hex(value.tool.instance_id), durability: value.tool.durability_u.toString() },
        ore: value.last_ore && { item: value.last_ore.item_id, instance: hex(value.last_ore.instance_id), purity: value.last_ore.ore_purity,
            bound: value.last_ore.bound } };
}

function observe(page, report) {
    page.on('pageerror', error => report.pageErrors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) { report.pageErrors.push('触屏连接未走独立游戏节点'); return; }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 20028) {
                const value = pb.MiningIntent.decode(frame.body);
                report.commands.push({ action: value.action, scene: value.scene_epoch.toString(), tool: hex(value.tool_instance_id) });
            }
            if (frame?.id === 20007) {
                const value = pb.MapTravelPrepare.decode(frame.body);
                report.destinations.push({ map: value.target_map_id.toString(), destination: value.destination });
            }
            if (frame?.id === 40024) {
                const value = pb.VendorQuoteRequest.decode(frame.body);
                report.quoteRequests.push({ request: frame.request, npc: value.npc_id.toString(), action: value.action,
                    instance: hex(value.instance_id), version: value.item_version.toString(), quantity: value.quantity.toString() });
            }
            if (frame?.id === 40026) {
                const value = pb.VendorCommitRequest.decode(frame.body);
                report.saleCommits.push({ request: frame.request, quote: hex(value.quote_id), op: hex(value.operation.op_id), npc: value.npc_id.toString() });
            }
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 20029) report.states.push(observation(pb.MiningState.decode(frame.body)));
            if (frame?.id === 20002) {
                const entry = pb.EnterLineResult.decode(frame.body);
                report.transitions.push({ scene: entry.scene_epoch.toString(), map: entry.room_id.toString(), kind: entry.scene_kind, binding: frame.binding });
            }
            if (frame?.id === 40025) {
                const value = pb.VendorQuoteResult.decode(frame.body);
                report.quotes.push({ request: frame.request, quote: hex(value.quote_id), instance: hex(value.instance_id), item: value.item_id,
                    action: value.action, quantity: value.item_quantity.toString(), bound: value.bound, gold: value.gold.toString(),
                    boundGold: value.bound_gold.toString(), ingot: value.ingot.toString(), boundIngot: value.bound_ingot.toString() });
            }
            if (frame?.id === 40005) {
                const value = pb.OperationResult.decode(frame.body);
                report.operationResults.push({ request: frame.request, op: hex(value.op_id), error: value.error_code, pending: value.pending });
            }
        });
    });
}

async function currentMap(page, session, map, kind) {
    await wait(page, ({ session, map, kind }) => {
        const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
        return session.ready && session.entry?.roomId.toString() === map && session.entry?.sceneKind === kind &&
            app?.world?.isMapReady && app.attached?.map.toString() === map && !cc.find('Canvas/Loading')?.activeInHierarchy;
    }, { session, map: String(map), kind }, 55000);
    return originalWorld(page, session);
}

async function openMap(page) {
    await touch(page, safe + 'BottomHUD/Footer/Map');
    await wait(page, target => !!cc.find(target)?.activeInHierarchy, directory);
}

async function backdrop(page, panel) {
    // 市集、地图与物品面板同用保存的背景分层，不以代码销毁面板。
    await page.touchscreen.tap(10, 130);
    await wait(page, target => !cc.find(target)?.activeInHierarchy, panel);
}

async function visibleItem(page, listPath, instance) {
    const point = await wait(page, ({ listPath, instance }) => {
        const list = cc.find(listPath)?.getComponent('List');
        if (!list) return null;
        const hex = item => Array.from(item.instanceId, value => value.toString(16).padStart(2, '0')).join('');
        const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
        for (const [index, row] of list.rows) {
            if (!row.activeInHierarchy || !list.items[index]?.instanceId || hex(list.items[index]) !== instance) continue;
            const rect = row._uiProps.uiTransformComp.getBoundingBoxToWorld();
            return { x: canvas.left + (rect.x + rect.width / 2) * canvas.width / size.width,
                y: canvas.top + (size.height - rect.y - rect.height / 2) * canvas.height / size.height,
                text: row.getChildByName('Text')?.getComponent(cc.Label)?.string };
        }
        return null;
    }, { listPath, instance });
    await page.touchscreen.tap(point.x, point.y);
    return point.text;
}

async function liveMining(page, session, predicate, timeout = 20000) {
    return wait(page, ({ session, predicate }) => {
        const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0], value = app.mining?.model.state;
        if (!value || !session.ready) return null;
        const active = app.mining.active, attempts = Number(value.attempts.toString());
        if (predicate === 'start') return value.canStart && !active;
        if (predicate === 'active') return active;
        if (predicate === 'ore') return active && value.lastOre?.orePurity >= 1 && attempts >= 1;
        if (predicate === 'stopped') return !active && attempts >= 1;
        return null;
    }, { session, predicate }, timeout);
}

async function verifyMine(page, session, report) {
    await touch(page, directory + '/RegionList/View/Content', '比奇矿区');
    const rows = await wait(page, ({ directory, minimum, deep }) => {
        const list = cc.find(directory + '/MapList/View/Content');
        const cells = list?.children.filter(row => row.activeInHierarchy).map(row => ({
            text: row.getChildByName('Text')?.getComponent(cc.Label)?.string,
            enabled: row.getComponent(cc.Button)?.interactable,
        })) ?? [];
        return cells.some(row => row.text?.includes(minimum)) && cells.some(row => row.text?.includes(deep)) ? cells : null;
    }, { directory, minimum: String(expected.minLevel) + '级', deep: String(expected.deepLevel) + '级' });
    assert(rows.some(row => row.text.includes(expected.minLevel + '级') && row.enabled));
    assert(rows.some(row => row.text.includes(expected.deepLevel + '级') && !row.enabled));
    report.rows = rows;
    await page.screenshot({ path: path.join(output, '01-mine-directory.png') });
    await touch(page, directory + '/MapList/View/Content', expected.minLevel + '级');
    report.mine = await currentMap(page, session, expected.mineMap, 7);
    report.mine.render = await page.evaluate(session => {
        const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
        return { floor: app.world?.assets?.regionalFloor, width: app.world.map.width, height: app.world.map.height,
            combatEntities: session.baseline.entities.filter(entity => entity.entityKind !== 1).length,
            countHidden: !cc.find('Canvas/Safe/MapViewport/Actions/Count')?.activeInHierarchy };
    }, session);
    assert(originalStyle && report.mine.render.floor === originalStyle && report.mine.render.width === 13 &&
        report.mine.render.height === 13 && report.mine.render.combatEntities === 0 && report.mine.render.countHidden);
    await liveMining(page, session, 'start');
    await label(page, action + '/Label', '开始挖矿');
    await touch(page, action);
    await liveMining(page, session, 'active');
    await label(page, action + '/Label', '停止挖矿');
    await liveMining(page, session, 'ore', 92000);
    assert.match(await uiLabel(page, safe + 'MapViewport/Mission/Text'), /纯度/);
    const emitted = report.states.filter(row => row.map === String(expected.mineMap) && row.ore).at(-1);
    assert(emitted?.ore.bound && emitted.ore.purity <= 100 && emitted.tool.instance === expected.toolInstance);
    assert.equal(BigInt(emitted.tool.durability), BigInt(expected.toolFull) - BigInt(emitted.attempts * expected.toolWear));
    report.ore = emitted.ore;
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await page.screenshot({ path: path.join(output, '02-mining.png') });
    await touch(page, action);
    await liveMining(page, session, 'stopped');
    await label(page, action + '/Label', '开始挖矿');
    await page.waitForTimeout(1200); // 停止前已受理的单轮可能先确认，之后不再新增。
    report.stopped = report.states.filter(row => row.map === String(expected.mineMap) && !row.active).at(-1);
    assert(report.stopped?.attempts >= emitted.attempts);
    await page.waitForTimeout(3800);
    assert.equal(report.states.filter(row => row.map === String(expected.mineMap)).at(-1).attempts, report.stopped.attempts);

    await touch(page, safe + 'MapViewport/Actions/Attack');
    await label(page, details + '/Title', '矿况');
    report.details = await page.evaluate(details => cc.find(details + '/List').getComponent('List').items, details);
    assert(report.details.some(row => row.includes('锄头持久')) && report.details.some(row => row.includes('绑定矿石')) &&
        report.details.some(row => row.includes('纯度')));
    await page.screenshot({ path: path.join(output, '03-mine-details.png') });
    await backdrop(page, details);
    await touch(page, safe + 'BottomHUD/Footer/Inventory');
    await wait(page, target => !!cc.find(target)?.activeInHierarchy, inventory);
    await visibleItem(page, inventory + '/List', report.ore.instance);
    await wait(page, target => !!cc.find(target)?.activeInHierarchy, itemDetails);
    report.item = await page.evaluate(itemDetails => cc.find(itemDetails + '/List').getComponent('List').items, itemDetails);
    assert(report.item.some(row => row.includes('纯度 ' + report.ore.purity)) && report.item.includes('来源 挖矿所得'));
    await page.screenshot({ path: path.join(output, '04-mined-ore.png') });
    await backdrop(page, itemDetails);
    await backdrop(page, inventory);

    await openMap(page);
    const back = await wait(page, ({ directory, map }) => {
        const region = cc.find(directory + '/RegionList')?.getComponent('List')?.items.find(row => row.maps.some(item => item.mapId === map));
        return region?.label;
    }, { directory, map: String(expected.normalMap) });
    await touch(page, directory + '/RegionList/View/Content', back);
    const normal = await wait(page, ({ directory, map }) => cc.find(directory + '/MapList')?.getComponent('List')?.items.find(row => row.mapId === map)?.label,
        { directory, map: String(expected.normalMap) });
    await touch(page, directory + '/MapList/View/Content', normal);
    report.returned = await currentMap(page, session, expected.normalMap, 1);
    await label(page, safe + 'MapViewport/Actions/Attack/Label', '攻击');
    assert.notEqual(report.mine.scene, report.returned.scene);
    assert.equal(report.mine.binding, report.returned.binding);
    assert(report.commands.some(row => row.action === 1) && report.commands.some(row => row.action === 2));
    assert(report.destinations.some(row => row.destination === 3 && row.map === String(expected.mineMap)) &&
        report.destinations.some(row => row.destination === 2 && row.map === String(expected.normalMap)));
    await page.screenshot({ path: path.join(output, '05-returned.png') });
}

// 城镇接近和打开商店都派发真实触摸，只读取地形、NPC 原画像素与可视窗口来计算触点。
async function townTouchPoint(page, town, mode) {
    return wait(page, ({ town, mode }) => {
        const world = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world;
        const npc = world?.townNpcs.find(view => view.npc.id === String(town.npc));
        if (!npc || !world.isMapReady) return null;
        const box = document.querySelector('canvas').getBoundingClientRect(), size = cc.view.getVisibleSize();
        const bounds = world.viewport._uiProps.uiTransformComp.getBoundingBoxToWorld();
        const entityRects = Array.from(world.visuals.values()).flatMap(visual => visual.body.map(sprite =>
            sprite.node._uiProps.uiTransformComp?.getBoundingBoxToWorld()).filter(Boolean));
        const inside = (point, rect, margin = 0) => point.x >= rect.x + margin && point.x <= rect.x + rect.width - margin
            && point.y >= rect.y + margin && point.y <= rect.y + rect.height - margin;
        const css = point => ({ x: box.left + point.x * box.width / size.width, y: box.top + (size.height - point.y) * box.height / size.height });
        const groundTransform = world.world._uiProps.uiTransformComp;
        const opacity = (layer, x, y) => layer.frames.every(key => {
            const definition = world.manifest.frames[key];
            const left = definition.offset_x - 24, top = 16 - definition.offset_y;
            const mask = world.manifest.townNpcHitMasks?.[key];
            const column = Math.floor(x - left), row = Math.floor(top - y);
            if (column < 1 || row < 1 || column >= definition.width - 1 || row >= definition.height - 1) return false;
            if (!mask) return true;
            for (let dy = -1; dy <= 1; dy++) for (let dx = -1; dx <= 1; dx++) {
                const pixel = (row + dy) * definition.width + column + dx;
                if (!(mask[pixel >> 3] & (1 << (pixel & 7)))) return false;
            }
            return true;
        });
        if (mode === 'npc') {
            for (let y = 24; y <= 54; y += 2) for (const x of [0, 2, -2, 5, -5, 8, -8]) {
                if (!npc.layers.some(layer => opacity(layer, x, y))) continue;
                const foot = world.projection.project(npc.npc.position.x, npc.npc.position.y);
                const point = groundTransform.convertToWorldSpaceAR(new cc.Vec3(foot.x + x, foot.y + y));
                if (inside(point, bounds, 8) && !entityRects.some(rect => inside(point, rect))) return css(point);
            }
            return null;
        }
        const npcRects = world.townNpcs.flatMap(view => view.layers.map(layer =>
            layer.sprite.node._uiProps.uiTransformComp.getBoundingBoxToWorld()));
        for (const target of town.approach) for (const [dx, dy] of [[0, 0], [-.3, -.3], [.3, -.3], [-.3, .3], [.3, .3]]) {
            const local = world.projection.project(target.x + dx, target.y + dy);
            const point = groundTransform.convertToWorldSpaceAR(new cc.Vec3(local.x, local.y));
            const tile = world.screenToGrid(point);
            if (tile?.x === target.x && tile.y === target.y && inside(point, bounds, 8) &&
                !entityRects.concat(npcRects).some(rect => inside(point, rect))) return { ...css(point), target };
        }
        return null;
    }, { town, mode }).catch(error => { throw new Error('等待城镇触摸点 ' + mode + '：' + error.message); });
}

async function observedWallet(page, session) {
    return wait(page, session => {
        const { wallet, inventory } = session;
        if (!wallet || !inventory || !wallet.revision.equals(inventory.revision)) return null;
        return { gold: wallet.gold.toString(), boundGold: wallet.boundGold.toString(),
            ingot: wallet.yuanbaoUnits.toString(), boundIngot: wallet.boundYuanbaoUnits.toString() };
    }, session);
}

async function sellOre(page, session, report, ore, supplied) {
    const rate = expected.town.rates.find(row => row.item_id === ore.item);
    const amount = rate && BigInt(ore.purity) * BigInt(rate.numerator) / BigInt(rate.denominator);
    assert(amount > 0n, '测试矿石须在本店收购范围且可获得正收入');
    const before = await observedWallet(page, session);
    const shownCurrency = ore.bound ? '绑定金币' : '金币';
    const row = await visibleItem(page, shop + '/ItemsList', ore.instance);
    assert(row.includes(`纯度 ${ore.purity}`) && row.includes(`${amount}${shownCurrency}/块`));
    await label(page, itemDetails + '/Action1/Label', '出售');
    await touch(page, itemDetails + '/Action1');
    await label(page, confirm + '/Title', '出售确认');
    const message = await uiLabel(page, confirm + '/Message');
    assert(message.includes(`获得 ${shownCurrency}${amount}`) && message.includes(`纯度 ${ore.purity}`));
    const request = report.quoteRequests.find(row => row.instance === ore.instance);
    const quote = report.quotes.find(row => row.request === request?.request);
    assert(request?.action === 2 && request.npc === String(expected.town.npc) && request.quantity === '1' && request.version === '1');
    assert(quote?.instance === ore.instance && quote.item === ore.item && quote.action === 2 && quote.quantity === '1' && quote.bound === ore.bound);
    assert.equal(quote.gold, ore.bound ? '0' : amount.toString());
    assert.equal(quote.boundGold, ore.bound ? amount.toString() : '0');
    assert(quote.ingot === '0' && quote.boundIngot === '0');
    await page.screenshot({ path: path.join(output, ore.bound ? '06-bound-ore-quote.png' : '07-unbound-ore-quote.png') });
    await touch(page, confirm + '/ConfirmButton');
    const wanted = { ...before, [ore.bound ? 'boundGold' : 'gold']: (BigInt(before[ore.bound ? 'boundGold' : 'gold']) + amount).toString() };
    await wait(page, ({ session, instance, wanted }) => {
        const { wallet, inventory } = session;
        const hex = bytes => Array.from(bytes, value => value.toString(16).padStart(2, '0')).join('');
        return wallet && inventory && wallet.revision.equals(inventory.revision) && !inventory.items.some(item => hex(item.instanceId) === instance) &&
            wallet.gold.toString() === wanted.gold && wallet.boundGold.toString() === wanted.boundGold &&
            wallet.yuanbaoUnits.toString() === wanted.ingot && wallet.boundYuanbaoUnits.toString() === wanted.boundIngot;
    }, { session, instance: ore.instance, wanted });
    const commit = report.saleCommits.find(row => row.quote === quote.quote);
    const result = report.operationResults.find(row => row.request === commit?.request);
    assert(commit?.npc === String(expected.town.npc) && result?.op === commit.op && result.error === 0 && !result.pending);
    report.sales.push({ ...ore, supplied, price: amount.toString(), quote: quote.quote, op: commit.op, row, message, before, after: wanted });
    if (await page.evaluate(panel => !!cc.find(panel)?.activeInHierarchy, itemDetails)) await backdrop(page, itemDetails);
}

async function verifyOreBuyback(page, session, report) {
    const { town } = expected;
    await openMap(page);
    const region = await wait(page, ({ directory, map }) => cc.find(directory + '/RegionList')?.getComponent('List')?.items
        .find(row => row.maps.some(item => item.mapId === map))?.label, { directory, map: String(town.map) });
    await touch(page, directory + '/RegionList/View/Content', region);
    const destination = await wait(page, ({ directory, map }) => cc.find(directory + '/MapList')?.getComponent('List')?.items
        .find(row => row.mapId === map)?.label, { directory, map: String(town.map) });
    await touch(page, directory + '/MapList/View/Content', destination);
    report.town = await currentMap(page, session, town.map, 2);
    assert.equal(report.town.binding, report.mine.binding);
    // 绕开角色和 NPC 身体，先点击可通行的邻格，再从当前原画身体打开商店。
    const approach = await townTouchPoint(page, town, 'ground');
    report.town.approach = approach;
    await page.touchscreen.tap(approach.x, approach.y);
    await wait(page, ({ session, target }) => {
        const player = session.baseline?.entities.find(entity => entity.entityHandle === session.entry.playerHandle);
        return player?.position.x === target.x && player.position.y === target.y && session.baseline.navigationState?.status === 2;
    }, { session, target: approach.target });
    const npc = await townTouchPoint(page, town, 'npc');
    report.town.touch = npc;
    await page.touchscreen.tap(npc.x, npc.y);
    await label(page, shop + '/Title', town.name);
    await touch(page, shop + '/Tabs/Sell');
    // 正常优先出售实际挖出的原件。若碰到黑铁或零价铜，使用明确隔离前态的绑定铜完成窗口覆盖。
    const rate = town.rates.find(row => row.item_id === report.ore.item);
    const sellMined = rate && report.ore.purity * rate.numerator >= rate.denominator;
    await sellOre(page, session, report, sellMined ? report.ore : town.fallback, !sellMined);
    await sellOre(page, session, report, town.unbound, true);
    assert.equal(report.quotes.length, 2);
    assert.equal(report.saleCommits.length, 2);
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, clientBundleSha256: expected.clientBundleSha256,
        fixtureSha256: expected.fixtureSha256, viewport: { width: 390, height: 844 }, sockets: 0,
        pageErrors: [], commands: [], destinations: [], states: [], transitions: [],
        quotes: [], quoteRequests: [], saleCommits: [], operationResults: [], sales: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: report.viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const asset = page.waitForResponse(response => response.ok() && new URL(response.url()).pathname === '/assets/main/index.js', { timeout: 65000 });
        void asset.catch(() => {});
        const session = await login(page, 'miner', { account: process.env.MINIMIR_MINING_BROWSER_ACCOUNT, password: process.env.MINIMIR_MINING_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await asset).body()).digest('hex'), expected.clientBundleSha256);
        report.normal = await currentMap(page, session, expected.normalMap, 1);
        if (report.normal.auto) {
            await touch(page, action);
            await wait(page, session => session.hosting?.autoBattleEnabled === false, session);
        }
        await openMap(page);
        if (expected.enabled) {
            await verifyMine(page, session, report);
            await verifyOreBuyback(page, session, report);
        } else {
            report.regions = await page.evaluate(directory => cc.find(directory + '/RegionList').getComponent('List').items.map(row => row.label), directory);
            assert(report.regions.length > 0 && !report.regions.some(row => row.includes('矿区')));
            assert.equal(report.commands.length, 0);
            assert.equal(report.states.length, 0);
            assert.deepEqual(report.quoteRequests, []);
            await page.screenshot({ path: path.join(output, '01-default-no-mine.png') });
        }
        assert.equal(report.sockets, 1);
        assert.deepEqual(report.pageErrors, []);
        report.passed = true;
    } catch (error) {
        report.error = error.stack || String(error);
        report.clientFailure = await page.evaluate(async () => {
            const { session } = await System.import('chunks:///_virtual/GameSession.ts');
            const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
            return { scene: session.entry?.sceneEpoch?.toString(), map: session.entry?.roomId?.toString(),
                ready: session.ready, lastError: session.lastError, travel: session.travel.phase, attached: app?.attached?.map?.toString() };
        }).catch(() => null);
        await page.screenshot({ path: path.join(output, 'failure.png') }).catch(() => {});
        throw error;
    } finally {
        fs.writeFileSync(path.join(output, 'mining-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
    console.log(JSON.stringify({ passed: report.passed, enabled: report.enabled, states: report.states.length, screenshots: output }));
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
