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
    try {
        await page.waitForFunction(name => {
            const current = window.cc?.director?.getScene();
            return current?.name === name && current.getComponentsInChildren('AppRoot').length
                && cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy;
        // 此等待只覆盖触屏冷启动，网络 ACK 仍按服务端期限分别核对。
        }, name, { timeout: name === 'Game' ? 75000 : 30000 });
    } catch (error) {
        const state = await page.evaluate(() => ({
            scene: window.cc?.director?.getScene()?.name,
            visibleText: cc.director.getScene()?.getComponentsInChildren(cc.Label)
                .filter(label => label.node.activeInHierarchy && label.string).map(label => label.string).slice(0, 20),
        })).catch(() => null);
        throw new Error(`等待场景 ${name}：${error.message} ${JSON.stringify(state)}`, { cause: error });
    }
}

// 协议头只有消息、序号及连接绑定；验收仅解码地图入场与 ACK 的必要身份字段。
function bodyOffset(payload) {
    let offset = 4;
    for (let field = 0; field < 2; field++) while (offset < payload.length && payload[offset++] & 0x80) {}
    return offset;
}

async function openedClient(browser, report, job, nickname) {
    const context = await browser.newContext({ viewport, hasTouch: true, isMobile: true, deviceScaleFactor: 1 });
    const page = await context.newPage();
    const frames = { queries: 0, entries: 0, connections: 0, ackAt: [], transitions: [], failures: [], sockets: [] };
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', connection => {
        if (connection.url() !== expected.endpoint) {
            report.errors.push('客户端请求了其它游戏节点');
            return;
        }
        const transport = { connection: ++frames.connections, openedAt: Date.now(), closedAt: null, transitions: [], acknowledgements: [] };
        frames.sockets.push(transport);
        connection.on('close', () => { transport.closedAt = Date.now(); });
        connection.on('framesent', ({ payload }) => {
            if (typeof payload === 'string' || payload.length < 6) return;
            const message = payload.readUInt16BE(2);
            if (message === 4) {
                const ack = pb.EnterWorldAck.decode(payload.subarray(bodyOffset(payload)));
                const sentAt = Date.now();
                transport.acknowledgements.push({ scene: ack.scene_epoch.toString(), baseline: ack.baseline_id.toString(), sentAt });
                frames.ackAt.push(sentAt);
            }
            if (message === 20017) frames.queries++;
            if (message === 20018) frames.entries++;
        });
        connection.on('framereceived', ({ payload }) => {
            if (typeof payload === 'string' || payload.length < 6) return;
            const message = payload.readUInt16BE(2);
            if (!(payload[1] & 1) && message !== 20002) return;
            let offset = bodyOffset(payload);
            if (payload[1] & 1) {
                let code = 0;
                for (let shift = 0; shift <= 28 && offset < payload.length; shift += 7) {
                    const byte = payload[offset++];
                    code += (byte & 0x7f) * 2 ** shift;
                    if (!(byte & 0x80)) break;
                }
                if (frames.failures.length < 20) frames.failures.push({ connection: transport.connection, at: Date.now(), message, code });
                return;
            }
            const entry = pb.EnterLineResult.decode(payload.subarray(offset));
            const transition = { connection: transport.connection, sceneKind: entry.scene_kind, map: entry.room_id.toString(),
                scene: entry.scene_epoch.toString(), baseline: entry.baseline_id.toString(), receivedAt: Date.now() };
            transport.transitions.push(transition);
            frames.transitions.push(transition);
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
            const sceneKind = session.entry?.sceneKind;
            if (sceneKind !== 1 && sceneKind !== 4) return;
            const loadingRoot = cc.find('Canvas/Loading');
            const loading = loadingRoot?.activeInHierarchy ? loadingRoot.getChildByName('Status')?.getComponent(cc.Label)?.string : '';
            const lastError = session.lastError;
            const generation = session.connection.generation, ready = session.ready;
            const previous = steps.at(-1);
            if (previous?.sceneKind === sceneKind && previous.generation === generation && previous.ready === ready
                && previous.loading === loading && previous.lastError === lastError) return;
            if (steps.length === 80) steps.shift();
            steps.push({ at: Date.now(), sceneKind, generation, ready, loading, lastError });
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
    }, { kind, session }, { timeout: kind === 4 ? 75000 : 25000 });
    try {
        const state = await handle.jsonValue();
        assert.equal(state.version, String(expected.version));
        return state;
    } finally { await handle.dispose(); }
}

// 发起参战前允许准备资源，收到实际进场帧后仍须按服务端线上的十秒期限确认。
function worldAckMs(client) {
    const entry = client.frames.transitions.findLast(row => row.sceneKind === 4);
    return entryAckMs(client.frames.sockets.find(row => row.connection === entry?.connection), entry, '共享首领');
}

// 必须由同一条连接确认同一场景和基线，旧普通图或其它连接的 ACK 不得充当重连验证。
function entryAckMs(transport, entry, label) {
    const acceptedAt = transport?.acknowledgements.find(row => row.scene === entry?.scene && row.baseline === entry?.baseline)?.sentAt;
    assert(entry && acceptedAt !== undefined, `缺少${label}入场或对应确认帧`);
    const elapsed = acceptedAt - entry.receivedAt;
    assert(elapsed >= 0 && elapsed < 10000, `${label}首基线确认超过十秒`);
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

// Chrome 切换到另一端时首个触屏事件可能只恢复前台。核对真实页签选中后再继续。
async function selectWorldTab(client) {
    const { page } = client;
    const selected = () => cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]?.boss?.section === 4;
    for (let attempt = 0; attempt < 3; attempt++) {
        if (await page.evaluate(selected)) return;
        await touch(page, safe + bossRoot + '/World');
        try {
            await page.waitForFunction(selected, null, { timeout: 2000 });
            return;
        } catch (error) {
            if (attempt === 2 || error.name !== 'TimeoutError') throw error;
        }
    }
}

// 使用真实账号初始发放的治愈术书，经技能学习交易进入共享场，不注入客户端表现数据。
async function learnObserverSpell(caster) {
    await caster.page.waitForFunction(session => session.inventory?.items.some(row => row.itemId === 21), caster.session,
        { timeout: 10000 });
    await caster.page.evaluate(async session => {
        const { operation } = await System.import('chunks:///_virtual/GameSession.ts');
        const book = session.inventory.items.find(row => row.itemId === 21);
        const result = await session.connection.request(40010, { itemInstanceId: book.instanceId, skillId: 2, operation: operation() });
        if (result.errorCode) throw new Error('治愈术真实学习失败：' + result.errorCode);
    }, caster.session);
}

// 持有旁观端真实资源，让两端确认入场时只差一次触屏操作。正常入场仍负责获取和挂载资源。
async function warmObserverAssets(observer) {
    await observer.page.bringToFront();
    return observer.page.evaluateHandle(async ({ session, map }) => {
        const { WorldAssets } = await System.import('chunks:///_virtual/WorldAssets.ts');
        return WorldAssets.prepare(session.role.job, () => {}, map);
    }, { session: observer.session, map: String(expected.map) });
}

// 由道士连接主动施法，在战士所属视图核对同一远端施法事件及实际 SpriteFrame。
async function observeOtherSpell(observer, caster, casterHandle) {
    await observer.page.bringToFront();
    const samples = await observer.page.evaluateHandle(({ session, casterHandle }) => {
        const events = [];
        const observe = event => {
            if (event.skillId === 2 && event.casterHandle === casterHandle)
                events.push({ cast: event.castId.toString(), scene: event.sceneEpoch.toString() });
        };
        session.casts.add(observe);
        return { events, stop: () => session.casts.delete(observe) };
    }, { session: observer.session, casterHandle });
    try {
        await caster.page.evaluate(session => {
            const player = session.baseline.entities.find(entity => entity.entityHandle === session.entry.playerHandle);
            const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
            session.connection.send(30003, { sceneEpoch: session.entry.sceneEpoch, inputSequence: ++app.inputSequence, skillId: 2,
                targetHandle: player.entityHandle, targetGrid: player.position, direction: player.direction });
        }, caster.session);
        const visible = await observer.page.waitForFunction(({ samples, session }) => {
            const world = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]?.world;
            for (const event of samples.events) {
                if (event.scene !== session.entry.sceneEpoch.toString()) continue;
                const effect = world?.effects?.nodes.get(event.cast + ':cast');
                if (effect?.sprite?.spriteFrame && /^Magic:20[0-9]$/.test(effect.frame))
                    return { cast: event.cast, frame: effect.frame };
            }
            return null;
        }, { samples, session: observer.session }, { timeout: 8000 });
        try { return await visible.jsonValue(); } finally { await visible.dispose(); }
    } finally {
        await observer.page.evaluate(samples => samples.stop(), samples);
        await samples.dispose();
    }
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
    let warrior, tao, warmObserver;
    try {
        report.step = '走正常触屏注册、选服和建角';
        // 保持两个独立客户端存活。开闭验收端在等待期间置于前台。
        warrior = await openedClient(browser, report, 0, '验武');
        const taoPending = openedClient(browser, report, 2, '验道');
        void taoPending.catch(() => {});
        await touch(warrior.page, safe + 'BottomHUD/QuickNav/Boss');
        await selectWorldTab(warrior);
        await warrior.page.bringToFront();
        report.step = '开放前保持面板，等待服务端自动更新';
        await bossPanel(warrior, '等待开放', false);
        const initialQueries = warrior.frames.queries;
        report.closedStateBeforeOpen = { observedAt: Date.now(), queries: initialQueries };
        assert(report.closedStateBeforeOpen.observedAt < expected.opensAt, '战士面板必须真实观测到开场前状态');
        tao = await taoPending;
        report.jobs = { warrior: 0, tao: 2 };
        await warrior.page.bringToFront();
        // 战士已在开场前打开面板。道士完成冷加载的时刻与面板自动刷新没有依赖关系。
        await bossPanel(warrior, '开放中', true, 65000);
        assert(warrior.frames.queries > initialQueries && warrior.frames.entries === 0);
        report.autoOpened = true;

        if (expected.version >= 30) {
            report.step = '预热战士端世界首领资源并由道士学习治疗术';
            await learnObserverSpell(tao);
            warmObserver = await warmObserverAssets(warrior);
        }
        report.step = '先由道士带着宠物入场，再让战士加入同一场次';
        await touch(tao.page, safe + 'BottomHUD/QuickNav/Boss');
        await selectWorldTab(tao);
        await bossPanel(tao, '开放中', true);
        const taoPressedAt = await touch(tao.page, safe + bossRoot + '/Action');
        report.step = '等待道士确认共享首领场景';
        const taoRoom = await map(tao, 4);
        const warriorPressedAt = await touch(warrior.page, safe + bossRoot + '/Action');
        report.step = '等待战士确认共享首领场景';
        const warriorRoom = await map(warrior, 4);
        if (warmObserver) {
            await warrior.page.evaluate(assets => assets.dispose(), warmObserver);
            await warmObserver.dispose();
            warmObserver = undefined;
        }
        report.tapToEntryMs = {
            warrior: warrior.frames.transitions.findLast(row => row.sceneKind === 4).receivedAt - warriorPressedAt,
            tao: tao.frames.transitions.findLast(row => row.sceneKind === 4).receivedAt - taoPressedAt,
        };
        report.entryAckMs = { warrior: worldAckMs(warrior), tao: worldAckMs(tao) };
        assert.equal(warriorRoom.map, String(expected.map));
        assert.equal(warriorRoom.map, taoRoom.map);
        assert(warriorRoom.occurrence.length > 0 && warriorRoom.occurrence === taoRoom.occurrence);
        if (expected.version >= 30) {
            report.step = '战士观察真实道士治疗特效';
            report.otherPlayerSpell = await observeOtherSpell(warrior, tao, taoRoom.roleHandle);
        }
        report.step = '核对战士可见的道士活宠';
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
        const [original, rejoined] = warrior.frames.sockets;
        assert(original.closedAt !== null && rejoined.closedAt === null, '必须替换断开的原连接，并保持当前重连活动');
        const [personalEntry, bossEntry] = rejoined.transitions;
        assert.equal(rejoined.transitions.length, 2, '重连只需依次确认原个人地图和共享首领图');
        assert.equal(personalEntry.sceneKind, 1);
        assert.equal(personalEntry.map, '1');
        assert.equal(bossEntry.sceneKind, 4);
        assert.equal(bossEntry.map, String(expected.map));
        report.reconnected = true;
        report.reconnectPersonalAckMs = entryAckMs(rejoined, personalEntry, '重连普通地图');
        report.reconnectAckMs = entryAckMs(rejoined, bossEntry, '重连共享首领');

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
        await bossPanel(warrior, '等待开放', false, Math.max(15000, expected.expiresAt - Date.now() + 15000));
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
        if (warmObserver) await warrior.page.evaluate(assets => assets.dispose(), warmObserver).catch(() => {});
        await Promise.allSettled([warrior, tao].filter(Boolean).map(client =>
            client.page.evaluate(data => data.stop(), client.progress)));
        await browser.close();
        fs.writeFileSync(path.join(resultDir, 'world-boss-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
        if (!report.passed) console.error('验收步骤 ' + report.step);
    }
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
