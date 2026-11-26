// 用真实 Creator 保存界面触屏验证隔离矿图、开停、矿石纯度和默认关闭。
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
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 20029) report.states.push(observation(pb.MiningState.decode(frame.body)));
            if (frame?.id === 20002) {
                const entry = pb.EnterLineResult.decode(frame.body);
                report.transitions.push({ scene: entry.scene_epoch.toString(), map: entry.room_id.toString(), kind: entry.scene_kind, binding: frame.binding });
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

async function visibleInventoryItem(page, instance) {
    const point = await wait(page, ({ inventory, instance }) => {
        const list = cc.find(inventory + '/List')?.getComponent('List');
        if (!list) return null;
        const hex = item => Array.from(item.instanceId, value => value.toString(16).padStart(2, '0')).join('');
        const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
        for (const [index, row] of list.rows) {
            if (!row.activeInHierarchy || !list.items[index] || hex(list.items[index]) !== instance) continue;
            const rect = row._uiProps.uiTransformComp.getBoundingBoxToWorld();
            return { x: canvas.left + (rect.x + rect.width / 2) * canvas.width / size.width,
                y: canvas.top + (size.height - rect.y - rect.height / 2) * canvas.height / size.height };
        }
        return null;
    }, { inventory, instance });
    await page.touchscreen.tap(point.x, point.y);
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
    await visibleInventoryItem(page, report.ore.instance);
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

async function main() {
    const report = { passed: false, enabled: expected.enabled, clientBundleSha256: expected.clientBundleSha256,
        fixtureSha256: expected.fixtureSha256, viewport: { width: 390, height: 844 }, sockets: 0,
        pageErrors: [], commands: [], destinations: [], states: [], transitions: [] };
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
        if (expected.enabled) await verifyMine(page, session, report);
        else {
            report.regions = await page.evaluate(directory => cc.find(directory + '/RegionList').getComponent('List').items.map(row => row.label), directory);
            assert(report.regions.length > 0 && !report.regions.some(row => row.includes('矿区')));
            assert.equal(report.commands.length, 0);
            assert.equal(report.states.length, 0);
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
