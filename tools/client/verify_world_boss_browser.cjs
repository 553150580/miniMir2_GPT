// 使用两个隔离 Chromium 触屏端验证共享首领的真实开场、宠物表现及重连。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const manifestPath = path.resolve(process.argv[2] ?? '');
const resultDir = path.dirname(manifestPath);
assert(manifestPath.startsWith(path.resolve('.work') + path.sep), '双端验收清单只能位于独立工作目录');
const expected = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
const base = new URL(expected.baseUrl);
const socket = new URL(expected.endpoint);
assert.equal(base.protocol, 'http:');
assert(base.hostname === '127.0.0.1' && base.port !== '18080');
assert(socket.protocol === 'ws:' && socket.hostname === '127.0.0.1' && socket.pathname === '/ws' && !['18101', '18103'].includes(socket.port));

const safe = 'Canvas/Safe/';
const bossRoot = 'UIMgr/Panels/BossChallenge';
const reconnect = 'Canvas/ReconnectDialog';
const viewport = { width: 720, height: 1280 };

// 此等待只读取游戏真实 Cocos 节点，点击继续走 touch 和 Button 的正常处理。
async function touch(page, fullPath, choice) {
    // 单浏览器只在前台完整绘制一页。每次真实触屏前切到该页，开闭窗等待期间保持前台。
    await page.bringToFront();
    const handle = await page.waitForFunction(({ fullPath, choice }) => {
        const root = cc.find(fullPath);
        const node = choice == null ? root : root?.children.find(child => child.activeInHierarchy
            && child.getChildByName('Text')?.getComponent(cc.Label)?.string.replace(/^● /u, '') === choice);
        if (document.hidden || !node?.activeInHierarchy || node.getComponent(cc.Button)?.interactable === false) return null;
        const bounds = node._uiProps?.uiTransformComp?.getBoundingBoxToWorld();
        if (!bounds) return null;
        const size = cc.view.getVisibleSize();
        const rect = document.querySelector('canvas').getBoundingClientRect();
        return { x: rect.x + (bounds.x + bounds.width / 2) / size.width * rect.width,
            y: rect.y + (size.height - bounds.y - bounds.height / 2) / size.height * rect.height };
    }, { fullPath, choice }, { timeout: 20000 });
    try {
        const position = await handle.jsonValue();
        const pressedAt = Date.now();
        await page.touchscreen.tap(position.x, position.y);
        return pressedAt;
    } finally { await handle.dispose(); }
}

async function edit(page, name, value) {
    await touch(page, safe + name);
    const input = page.locator('input:visible');
    await input.waitFor({ state: 'visible', timeout: 5000 });
    await input.fill(value);
}

async function scene(page, name) {
    await page.waitForFunction(name => {
        const current = window.cc?.director?.getScene();
        return current?.name === name && current.getComponentsInChildren('AppRoot').length
            && cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy;
    }, name, { timeout: 30000 });
}

async function openedClient(browser, report, job, nickname) {
    const context = await browser.newContext({ viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1 });
    const page = await context.newPage();
    const frames = { queries: 0, entries: 0, connections: 0, ackAt: [], transitions: [], failures: [] };
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', connection => {
        if (connection.url() !== expected.endpoint) {
            report.errors.push('客户端请求了其它游戏节点');
            return;
        }
        frames.connections++;
        connection.on('framesent', ({ payload }) => {
            if (typeof payload === 'string' || payload.length < 6) return;
            const message = payload.readUInt16BE(2);
            if (message === 4) frames.ackAt.push(Date.now());
            if (message === 20017) frames.queries++;
            if (message === 20018) frames.entries++;
        });
        connection.on('framereceived', ({ payload }) => {
            if (typeof payload === 'string' || payload.length < 6) return;
            const message = payload.readUInt16BE(2);
            if (!(payload[1] & 1) && message !== 20002) return;
            let offset = 4;
            for (let field = 0; field < 2; field++) while (offset < payload.length && payload[offset++] & 0x80) {}
            if (payload[1] & 1) {
                let code = 0;
                for (let shift = 0; shift <= 28 && offset < payload.length; shift += 7) {
                    const byte = payload[offset++];
                    code += (byte & 0x7f) * 2 ** shift;
                    if (!(byte & 0x80)) break;
                }
                if (frames.failures.length < 20) frames.failures.push({ message, code });
                return;
            }
            const entry = pb.EnterLineResult.decode(payload.subarray(offset));
            frames.transitions.push({ sceneKind: entry.scene_kind, map: entry.room_id.toString(), receivedAt: Date.now() });
        });
    });
    await page.goto(base.href);
    await scene(page, 'Login');
    await edit(page, 'Account', 'wb_' + crypto.randomBytes(6).toString('hex'));
    await edit(page, 'Password', crypto.randomBytes(8).toString('hex'));
    await touch(page, safe + 'Register');
    await scene(page, 'ServerSelect');
    await touch(page, safe + 'RegionList/View/Content', expected.region);
    await touch(page, safe + 'ServerList/View/Content', expected.server);
    await scene(page, 'RoleSelect');
    await page.waitForFunction(() => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string === '空角色槽');
    await touch(page, safe + 'JobTabs/Job' + job);
    const roleName = nickname + crypto.randomBytes(3).toString('hex');
    await edit(page, 'Name', roleName);
    await touch(page, safe + 'Create');
    await page.waitForFunction(name => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string.includes(name), roleName);
    await touch(page, safe + 'Enter');
    await scene(page, 'Game');
    const session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
    const progress = await page.evaluateHandle(session => {
        const steps = [];
        const timer = setInterval(() => {
            if (session.entry?.sceneKind !== 4) return;
            const loading = cc.find('Canvas/Loading/Status')?.getComponent(cc.Label)?.string;
            const lastError = session.lastError;
            const previous = steps.at(-1);
            if (previous?.loading === loading && previous?.lastError === lastError) return;
            if (steps.length === 80) steps.shift();
            steps.push({ at: Date.now(), loading, lastError });
        }, 200);
        return { steps, stop: () => clearInterval(timer) };
    }, session);
    const client = { page, session, progress, frames, roleName };
    await map(client, 1);
    assert.equal(await page.evaluate(session => session.role.job, session), job);
    // 使用正常自动战斗开关，避免首领申请与两个新角色的普通挂机结算争抢同一事务时点。
    await page.waitForFunction(session => !!session.hosting, session, { timeout: 5000 });
    if (await page.evaluate(session => session.hosting.autoBattleEnabled, session)) {
        await touch(page, safe + 'MapViewport/Actions/Auto');
        await page.waitForFunction(session => session.hosting?.autoBattleEnabled === false, session, { timeout: 10000 });
    }
    return client;
}

async function map(client, kind) {
    const { page, session } = client;
    const handle = await page.waitForFunction(({ kind, session }) => {
        const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
        const entry = session.entry;
        if (!session.ready || entry?.sceneKind !== kind || !app?.world?.isMapReady || app.attaching) return null;
        return { map: entry.roomId.toString(), version: entry.resourceVersion.toString(), occurrence: entry.serverTransitionId ?? '',
            generation: session.connection.generation, roleHandle: entry.playerHandle };
    }, { kind, session }, { timeout: kind === 4 ? 50000 : 25000 });
    try {
        const state = await handle.jsonValue();
        assert.equal(state.version, String(expected.version));
        return state;
    } finally { await handle.dispose(); }
}

// 发起参战前允许准备资源，收到实际进场帧后仍须按服务端线上的十秒期限确认。
function worldAckMs(client) {
    const entry = client.frames.transitions.findLast(row => row.sceneKind === 4);
    const acceptedAt = client.frames.ackAt.find(at => at >= entry?.receivedAt);
    assert(entry && acceptedAt !== undefined, '缺少共享首领进场或确认帧');
    const elapsed = acceptedAt - entry.receivedAt;
    assert(elapsed < 10000, '共享首领首基线确认超过十秒');
    return elapsed;
}

async function bossPanel(client, status, enabled, timeout = 18000) {
    const { page, session } = client;
    await page.waitForFunction(({ status, enabled, session }) => {
        const panel = cc.find('Canvas/Safe/UIMgr/Panels/BossChallenge');
        return session.ready && panel?.activeInHierarchy
            && panel.getChildByName('Status')?.getComponent(cc.Label)?.string === status
            && (enabled === undefined || panel.getChildByName('Action')?.getComponent(cc.Button)?.interactable === enabled);
    }, { status, enabled, session }, { timeout });
}

async function pet(client, corpse, handle = 0, timeout = 15000) {
    const { page, session } = client;
    const result = await page.waitForFunction(({ session, corpse, handle, definition }) => {
        if (!session.ready || session.entry?.sceneKind !== 4) return null;
        const world = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]?.world;
        const entity = session.baseline?.entities.find(candidate => candidate.entityKind === 2 && candidate.definitionId === definition
            && (!handle || candidate.entityHandle === handle) && Boolean(candidate.isPetCorpse) === corpse && candidate.hp.isZero() === corpse);
        if (!entity || !world) return null;
        const visual = world.visuals.get(entity.entityHandle);
        const name = visual?.node.getChildByName('Name');
        const label = name?.getComponent(cc.Label)?.string;
        const drawn = visual?.body.some(layer => layer.node.activeInHierarchy && layer.spriteFrame);
        if (!name?.activeInHierarchy || !label || label.endsWith('·尸体') !== corpse || !drawn) return null;
        const floor = visual.node.parent !== world.entities && visual.node.getSiblingIndex() < world.entities.getSiblingIndex();
        if (corpse !== floor) return null;
        return { handle: entity.entityHandle, life: entity.life.toString(), hp: entity.hp.toString(),
            position: { x: entity.position.x, y: entity.position.y }, name: label, floor };
    }, { session, corpse, handle, definition: expected.petDefinition }, { timeout });
    try { return await result.jsonValue(); } finally { await result.dispose(); }
}

async function absent(client, otherRole, petHandle = 0) {
    await client.page.waitForFunction(({ session, otherRole, petHandle }) => {
        const entities = session.baseline?.entities;
        return session.ready && session.entry?.sceneKind === 4 && entities && !entities.some(entity =>
            entity.displayName === otherRole || (petHandle && entity.entityHandle === petHandle));
    }, { session: client.session, otherRole, petHandle }, { timeout: 10000 });
}

// 走常规客户端导航及真实 WebSocket，直接指定地格，避免点中宠物时变成查看实体。
async function traversePetCell(client, position, expectedStatus) {
    const { page, session } = client;
    const inputSequence = await page.evaluate(position => {
        const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
        if (!app?.canSendInput()) return 0;
        return app.nav.setGroundTarget(position) ? app.inputSequence : 0;
    }, position);
    assert(inputSequence > 0, '共享首领场景未接收导航输入');
    await page.waitForFunction(({ session, position, inputSequence, expectedStatus }) => {
        const navigation = session.baseline?.navigationState;
        const player = session.baseline?.entities.find(entity => entity.entityHandle === session.entry.playerHandle);
        if (!player || navigation?.inputSequence !== inputSequence || navigation.status !== expectedStatus) return false;
        const arrived = player.position.x === position.x && player.position.y === position.y;
        return arrived === (expectedStatus === 2);
    }, { session, position, inputSequence, expectedStatus }, { timeout: 5000 });
}

// 失败时只保存地图状态和错误，不记录账号、票据、密码或完整协议帧。
async function diagnose(client) {
    if (!client) return undefined;
    const runtime = await client.page.evaluate(session => {
        const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
        const panel = cc.find('Canvas/Safe/UIMgr/Panels/BossChallenge');
        return { sceneKind: session.entry?.sceneKind, ready: session.ready, lastError: session.lastError,
            attaching: app?.attaching, mapReady: app?.world?.isMapReady,
            baselineScene: session.baseline?.sceneEpoch.toString(), entryScene: session.entry?.sceneEpoch.toString(),
            panelStatus: panel?.getChildByName('Status')?.getComponent(cc.Label)?.string,
            reconnectMessage: cc.find('Canvas/ReconnectDialog/Panel/Message')?.getComponent(cc.Label)?.string };
    }, client.session).catch(() => null);
    const progress = await client.page.evaluate(data => data.steps, client.progress).catch(() => []);
    return { runtime, progress, frames: client.frames };
}

async function main() {
    const report = { passed: false, errors: [], ruleVersion: expected.version, opensAt: expected.opensAt, expiresAt: expected.expiresAt };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    let warrior, tao;
    try {
        report.step = '走正常触屏注册、选服和建角';
        // 保持两个独立客户端存活。开闭验收端在等待期间置于前台。
        warrior = await openedClient(browser, report, 0, '验武');
        const taoPending = openedClient(browser, report, 2, '验道');
        void taoPending.catch(() => {});
        await touch(warrior.page, safe + 'BottomHUD/QuickNav/Boss');
        await touch(warrior.page, safe + bossRoot + '/World');
        await warrior.page.bringToFront();
        report.step = '开放前保持面板，等待服务端自动更新';
        await bossPanel(warrior, '等待开放', false);
        const initialQueries = warrior.frames.queries;
        report.closedStateBeforeOpen = { observedAt: Date.now(), queries: initialQueries };
        tao = await taoPending;
        report.jobs = { warrior: 0, tao: 2 };
        await warrior.page.bringToFront();
        assert(Date.now() < expected.opensAt, '开场前必须完成双端准备并保持战士面板');
        await bossPanel(warrior, '开放中', true, 65000);
        assert(warrior.frames.queries > initialQueries && warrior.frames.entries === 0);
        report.autoOpened = true;

        report.step = '先由道士带着宠物入场，再让战士加入同一场次';
        await touch(tao.page, safe + 'BottomHUD/QuickNav/Boss');
        await touch(tao.page, safe + bossRoot + '/World');
        await bossPanel(tao, '开放中', true);
        const taoPressedAt = await touch(tao.page, safe + bossRoot + '/Action');
        report.step = '等待道士确认共享首领场景';
        const taoRoom = await map(tao, 4);
        const warriorPressedAt = await touch(warrior.page, safe + bossRoot + '/Action');
        report.step = '等待战士确认共享首领场景';
        const warriorRoom = await map(warrior, 4);
        report.tapToEntryMs = {
            warrior: warrior.frames.transitions.findLast(row => row.sceneKind === 4).receivedAt - warriorPressedAt,
            tao: tao.frames.transitions.findLast(row => row.sceneKind === 4).receivedAt - taoPressedAt,
        };
        report.entryAckMs = { warrior: worldAckMs(warrior), tao: worldAckMs(tao) };
        assert.equal(warriorRoom.map, String(expected.map));
        assert.equal(warriorRoom.map, taoRoom.map);
        assert(warriorRoom.occurrence.length > 0 && warriorRoom.occurrence === taoRoom.occurrence);
        const alive = await pet(warrior, false);
        const own = await pet(tao, false, alive.handle);
        assert.equal(alive.life, own.life);
        assert.equal(alive.name, own.name);
        assert.deepEqual(alive.position, own.position);
        assert.notEqual(warriorRoom.roleHandle, taoRoom.roleHandle);
        report.alivePet = alive;
        await warrior.page.screenshot({ path: path.join(resultDir, 'warrior-observes-taoist-pet.png') });
        report.step = '战士导航到活宠所在格被权威占位阻挡';
        await traversePetCell(warrior, alive.position, 5);
        report.occupiedPetTileBlocked = true;

        report.step = '战士掉线，道士观察席位消失，战士用原角色重新接回';
        await warrior.page.evaluate(session => session.connection.socket.close(4001, 'isolated browser transport interruption'), warrior.session);
        await absent(tao, warrior.roleName);
        await touch(warrior.page, reconnect + '/Panel/Reconnect');
        await warrior.page.waitForFunction(() => !cc.find('Canvas/ReconnectDialog')?.activeInHierarchy, null, { timeout: 30000 });
        const restored = await map(warrior, 4);
        assert.equal(restored.occurrence, warriorRoom.occurrence);
        assert.equal(restored.roleHandle, warriorRoom.roleHandle);
        assert(restored.generation > warriorRoom.generation && warrior.frames.connections === 2 && warrior.frames.entries === 1);
        report.reconnected = true;
        report.reconnectAckMs = worldAckMs(warrior);

        report.step = '首领真实击杀，道士原出生的宠物尸体在战士地板层可见';
        const corpse = await pet(warrior, true, alive.handle, 65000);
        assert.equal(corpse.life, alive.life);
        assert.equal(corpse.name, alive.name + '·尸体');
        assert.deepEqual(corpse.position, alive.position);
        report.petCorpse = corpse;
        await warrior.page.screenshot({ path: path.join(resultDir, 'warrior-observes-taoist-pet-corpse.png') });
        report.step = '战士可进入同一宠物尸体的地格';
        await traversePetCell(warrior, corpse.position, 2);
        report.corpseTileWalkable = true;

        report.step = '主动离开挑战并确认双方返回普通场景';
        await touch(tao.page, safe + 'BottomHUD/QuickNav/Boss');
        await bossPanel(tao, '开放中', true);
        await touch(tao.page, safe + bossRoot + '/Action');
        assert.equal((await map(tao, 1)).map, '1');
        await absent(warrior, tao.roleName, corpse.handle);
        await touch(warrior.page, safe + 'BottomHUD/QuickNav/Boss');
        await bossPanel(warrior, '开放中', true);
        await touch(warrior.page, safe + bossRoot + '/Action');
        assert.equal((await map(warrior, 1)).map, '1');
        report.returned = true;

        report.step = '持续保持普通场面板，从开放自动更新到下次等待';
        await touch(warrior.page, safe + 'BottomHUD/QuickNav/Boss');
        await warrior.page.bringToFront();
        await bossPanel(warrior, '开放中');
        assert(Date.now() < expected.expiresAt, '关闭前必须在普通地图保持战士面板');
        const beforeCloseQueries = warrior.frames.queries;
        await bossPanel(warrior, '等待开放', false, 75000);
        assert(warrior.frames.queries > beforeCloseQueries && warrior.frames.entries === 1);
        report.autoClosed = true;
        report.clientCounts = { warrior: warrior.frames, tao: tao.frames };
        assert.deepEqual(report.errors, []);
        delete report.step;
        report.passed = true;
        console.log('双客户端世界首领自动开闭、跨职业活宠与尸体、原席重连和返回通过。');
    } catch (error) {
        report.observations = { warrior: await diagnose(warrior), tao: await diagnose(tao) };
        throw error;
    } finally {
        await Promise.allSettled([warrior, tao].filter(Boolean).map(client =>
            client.page.evaluate(data => data.stop(), client.progress)));
        await browser.close();
        fs.writeFileSync(path.join(resultDir, 'world-boss-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
        if (!report.passed) console.error('验收步骤 ' + report.step);
    }
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
