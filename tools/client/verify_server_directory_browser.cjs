// 在独立试玩入口逐服触屏选服、建角和入场，核对票据目标与首场景确认。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { observeTravel, measureTravel } = require('./map_travel_benchmark.cjs');

const manifestPath = path.resolve(process.argv[2] ?? '');
const work = path.resolve('.work');
assert(manifestPath.startsWith(work + path.sep), '选服验收清单必须在独立工作目录');
const { baseUrl, entries, tasksCodex, bossChallenges } = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
const base = new URL(baseUrl);
assert(base.protocol === 'http:' && base.hostname === '127.0.0.1' && base.port !== '18080', '仅允许独立回环试玩入口');
assert.equal(entries.length, 4);
assert.equal(new Set(entries.map(entry => entry.origin)).size, 4);
assert.equal(new Set(entries.map(entry => entry.run)).size, 4);
const endpoints = new Map();
for (const entry of entries) {
    const target = new URL(entry.endpoint);
    assert(target.protocol === 'ws:' && target.hostname === '127.0.0.1' && target.pathname === '/ws');
    const assigned = endpoints.get(entry.node) ?? [];
    assigned.push(entry.endpoint);
    endpoints.set(entry.node, assigned);
}
assert.equal(endpoints.size, 2);
for (const urls of endpoints.values()) {
    assert.equal(urls.length, 2);
    assert.equal(urls[0], urls[1]);
}
assert.equal(new Set(Array.from(endpoints.values(), urls => urls[0])).size, 2);

// 账号协议帧先跳过请求序号和连接代次，余下部分供对应的 protobuf 类型解码。
function accountPayload(raw, message) {
    if (!raw || raw.length < 6 || raw[0] !== 1 || raw[1] !== 0 || raw.readUInt16BE(2) !== message) return null;
    let offset = 4;
    for (let field = 0; field < 2; field++) {
        while (offset < raw.length && raw[offset++] & 0x80) {}
    }
    return raw.subarray(offset);
}

async function main() {
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const report = { baseUrl, reached: [], errors: [] };
    let page;
    try {
        const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1 });
        page = await context.newPage();
        if (process.env.MINIMIR_BENCHMARK_MAP_TRAVEL === '1') {
            const network = await context.newCDPSession(page);
            await network.send('Network.enable');
            await network.send('Network.setCacheDisabled', { cacheDisabled: true });
        }
        const travelBenchmark = process.env.MINIMIR_BENCHMARK_MAP_TRAVEL === '1' ? observeTravel(page) : undefined;
        page.on('pageerror', error => report.errors.push(error.message));
        const connections = [];
        const createdOrigins = [];
        const ticketResponses = [];
        page.on('request', request => {
            if (new URL(request.url()).pathname !== '/account/rpc') return;
            const payload = accountPayload(request.postDataBuffer(), 10005);
            if (payload) createdOrigins.push(pb.CreateRoleRequest.decode(payload).origin_server_id.toString());
        });
        page.on('response', response => {
            const request = response.request();
            if (new URL(request.url()).pathname !== '/account/rpc' || !accountPayload(request.postDataBuffer(), 10007)) return;
            // 只保留目标字段，票据原文不写进报告。
            const route = response.body().then(raw => {
                const payload = accountPayload(raw, 10008);
                if (!payload) return null;
                const ticket = pb.EntryTicketResult.decode(payload);
                return { run: ticket.run_server_id.toString(), node: ticket.game_node_id, endpoint: ticket.game_wss_url };
            }).catch(() => null);
            ticketResponses.push(route);
        });
        page.on('websocket', socket => {
            const state = { endpoint: socket.url(), ready: false };
            connections.push(state);
            socket.on('framereceived', ({ payload }) => {
                // 消息 6 是首基线确认后的 SyncReady。观察不保存账号、口令、网络票据或资产正文。
                if (typeof payload !== 'string' && payload.length >= 6 && payload[0] === 1 && payload[1] === 0
                    && payload[2] * 256 + payload[3] === 6) state.ready = true;
            });
        });
        const scene = async name => {
            report.step = '等待场景 ' + name;
            await page.waitForFunction(name => {
                const current = window.cc?.director?.getScene();
                return current?.name === name && current.getComponentsInChildren('AppRoot').length
                    && cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy;
            }, name, { timeout: 60000 });
            await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
        };
        const tap = async (suffix, label) => {
            report.step = `触屏 ${suffix}${label === undefined ? '' : ' · ' + label}`;
            const handle = await page.waitForFunction(({ suffix, label }) => {
                const node = cc.find('Canvas/Safe/' + suffix);
                const target = label == null ? node : node?.children.find(child => {
                    const value = child.getChildByName('Text')?.getComponent(cc.Label)?.string;
                    return value?.replace(/^● /u, '') === label;
                });
                if (!target?.activeInHierarchy || label != null && !target.getComponent(cc.Button)?.interactable) return null;
                const transform = target._uiProps?.uiTransformComp;
                if (!transform) return null;
                const box = transform.getBoundingBoxToWorld();
                const visible = cc.view.getVisibleSize(), rect = document.querySelector('canvas').getBoundingClientRect();
                return { x: rect.x + (box.x + box.width / 2) / visible.width * rect.width,
                    y: rect.y + (visible.height - box.y - box.height / 2) / visible.height * rect.height };
            }, { suffix, label }, { timeout: 30000 });
            const point = await handle.jsonValue();
            await handle.dispose();
            const tappedAt = performance.now();
            await page.touchscreen.tap(point.x, point.y);
            return tappedAt;
        };
        const edit = async (name, value) => {
            await tap(name);
            const widget = page.locator('input:visible');
            await widget.waitFor({ state: 'visible', timeout: 5000 });
            await widget.fill(value);
        };
        const verifyTasks = async () => {
            const panel = 'UIMgr/Panels/TasksCodex';
            const bag = 'UIMgr/Panels/Inventory';
            await tap('BottomHUD/QuickNav/Inventory');
            report.step = '选择新角色背包里的原木剑';
            const weapon = await page.waitForFunction(path => {
                const list = cc.find('Canvas/Safe/' + path + '/List')?.getComponent('List');
                const match = Array.from(list?.rows ?? []).find(([index, node]) => node.activeInHierarchy && list.items[index]?.itemId === 218);
                if (!match) return null;
                const box = match[1]._uiProps.uiTransformComp.getBoundingBoxToWorld();
                const visible = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
                return { x: canvas.x + (box.x + box.width / 2) / visible.width * canvas.width,
                    y: canvas.y + (visible.height - box.y - box.height / 2) / visible.height * canvas.height };
            }, bag, { timeout: 10000 });
            const point = await weapon.jsonValue();
            await weapon.dispose();
            await page.touchscreen.tap(point.x, point.y);
            await page.waitForFunction(() => cc.find('Canvas/Safe/PopupView/Panels/StatusDetails')?.activeInHierarchy);
            await page.touchscreen.tap(10, 200);
            await page.waitForFunction(() => !cc.find('Canvas/Safe/PopupView/Panels/StatusDetails')?.activeInHierarchy);
            await tap(bag + '/Equip');
            await tap('PopupView/Panels/Confirm/ConfirmButton');
            report.step = '等待权威装备回执';
            const session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
            try {
                const equipped = await page.waitForFunction(session => session.inventory?.equipped?.some(
                    entry => entry.slot === 0 && entry.item.itemId === 218), session, { timeout: 15000 });
                await equipped.dispose();
            } finally { await session.dispose(); }
            await page.touchscreen.tap(10, 200);
            await page.waitForFunction(path => !cc.find('Canvas/Safe/' + path)?.activeInHierarchy, bag);
            report.step = '开启自动战斗并观察任务进度';
            const alreadyAuto = await page.evaluate(() => cc.find('Canvas/Safe/MapViewport/Actions/Auto')?.getComponent(cc.Toggle)?.isChecked);
            if (!alreadyAuto) await tap('MapViewport/Actions/Auto');
            await tap('BottomHUD/QuickNav/Quest');
            report.step = '等待主线目标可领取';
            await page.waitForFunction(path => {
                const root = cc.find('Canvas/Safe/' + path);
                return root?.activeInHierarchy && root.getChildByName('Claim')?.getComponent(cc.Button)?.interactable
                    && root.getChildByName('Detail')?.getComponent(cc.Label)?.string.includes('初次击杀');
            }, panel, { timeout: 45000 });
            await tap(panel + '/Claim');
            report.step = '等待领取回执刷新主线任务';
            await page.waitForFunction(path => cc.find('Canvas/Safe/' + path + '/RecordList/View/Content')?.children.some(
                node => node.getChildByName('Text')?.getComponent(cc.Label)?.string?.includes('初次击杀  已领')),
            panel, { timeout: 15000 });
            await tap(panel + '/CategoryList/View/Content', '图鉴');
            report.step = '核对击杀已计入图鉴';
            await page.waitForFunction(path => {
                const root = cc.find('Canvas/Safe/' + path);
                return /稻草人\s+[1-5]\/5/.test(root?.getChildByName('Detail')?.getComponent(cc.Label)?.string ?? '');
            }, panel, { timeout: 15000 });
            report.tasksCodex = { mainClaimed: true, killInCodex: true };
            const hasPreview = await page.evaluate(path => !!cc.find('Canvas/Safe/' + path + '/CodexPreview'), panel);
            if (hasPreview) {
                await page.waitForFunction(path => {
                    const root = cc.find('Canvas/Safe/' + path);
                    const list = root?.getChildByPath('RecordList')?.getComponent('List');
                    const frame = root?.getChildByName('CodexPreview')?.getComponent(cc.Sprite)?.spriteFrame;
                    return frame && list?.items[0]?.heading && list.items[1]?.name === '稻草人';
                }, panel, { timeout: 12000 });
                report.tasksCodex.mapGroupAndMonsterPreview = true;
            }
            // 左上角的背景位于保存布局的任务面板外。
            await page.touchscreen.tap(20, 200);
            await page.waitForFunction(path => !cc.find('Canvas/Safe/' + path)?.activeInHierarchy, panel);
        };
        const verifyBossChallenges = async () => {
            const panel = 'UIMgr/Panels/BossChallenge';
            const boss = 'BottomHUD/QuickNav/Boss';
            const sceneKind = async kind => {
                report.step = '等待首领附着 ' + kind;
                const session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
                try {
                    const result = await page.waitForFunction(({ kind, session }) => {
                        const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
                        return session.ready && session.entry.sceneKind === kind && app?.world?.isMapReady && !app?.attaching
                            && { map: session.entry.roomId.toString(), version: session.entry.resourceVersion.toString() };
                    }, { kind, session }, { timeout: 25000 });
                    try { return await result.jsonValue(); } finally { await result.dispose(); }
                } finally { await session.dispose(); }
            };
            const expectLabel = async (node, substring) => {
                report.step = '核对首领面板 ' + node + ' ' + substring;
                await page.waitForFunction(({ node, substring }) => cc.find('Canvas/Safe/' + node)?.getComponent(cc.Label)?.string.includes(substring),
                    { node: panel + '/' + node, substring }, { timeout: 15000 });
            };
            const home = await sceneKind(1);
            await tap(boss);
            await expectLabel('Status', '可入场');
            await tap(panel + '/Action');
            const personal = await sceneKind(3);
            assert.notEqual(personal.map, home.map);
            assert.equal(personal.version, home.version);
            if (process.env.MINIMIR_QA_PERSONAL_BOSS_PROFILE) {
                report.step = '核对个人首领的真实原版外观';
                const appearance = await page.waitForFunction(profile => {
                    const world = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]?.world;
                    const guard = Array.from(world?.visuals?.values() ?? []).find(visual =>
                        visual.state.entityKind === 3 && visual.profile?.id === profile);
                    const frame = guard?.keys[0];
                    if (!/^Hunt:18[0-9]{3}$/.test(frame ?? '') || !guard.body[0]?.spriteFrame
                        || world.assets.frames.get(frame) !== guard.body[0].spriteFrame) return null;
                    return { profile, frame, archive: world.manifest.frames[frame].source_archive };
                }, process.env.MINIMIR_QA_PERSONAL_BOSS_PROFILE, { timeout: 6000 });
                try { report.personalBossAppearance = await appearance.jsonValue(); } finally { await appearance.dispose(); }
                assert.equal(report.personalBossAppearance.archive, '18.Lib');
            }
            await tap(boss);
            await expectLabel('Status', '挑战中');
            await tap(panel + '/Action');
            assert.deepEqual(await sceneKind(1), home);
            await tap(boss);
            await expectLabel('Results', '中断');
            await tap(panel + '/World');
            await expectLabel('Details', '上海时间');
            const worldStatus = await page.evaluate(path => cc.find('Canvas/Safe/' + path + '/Status')?.getComponent(cc.Label)?.string, panel);
            assert.match(worldStatus, /等待开放|开放中|已击杀|已超时/);
            await page.touchscreen.tap(20, 200);
            await page.waitForFunction(path => !cc.find('Canvas/Safe/' + path)?.activeInHierarchy, panel);
            report.bossChallenges = { home, personal, leaveAndHistory: true, worldStatus };
        };
        const verifyOrdinaryMap = async () => {
            const version = await page.evaluate(async () => {
                const { session } = await System.import('chunks:///_virtual/GameSession.ts');
                return Number(session.entry?.resourceVersion.toString());
            });
            if (version < 30) return;
            const list = 'UIMgr/Panels/TownDirectory';
            await tap('BottomHUD/Footer/Map');
            await tap(list + '/RegionList/View/Content', '谷地');
            await tap(list + '/MapList/View/Content', '谷地南坡 · 野外');
            report.step = '核对规则30普通地图真实怪物外观';
            // 模块只加载一次，让页面轮询实际等待地图附着和怪物贴图完成。
            const session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
            try {
                const view = await page.waitForFunction(session => {
                    const world = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0]?.world;
                    if (!world?.isMapReady || !session.ready || session.entry.roomId.toString() !== '6') return null;
                    const monster = Array.from(world.visuals.values()).find(visual => visual.state.entityKind === 3 && visual.state.definitionId === 3);
                    if (!monster?.body[0]?.spriteFrame || !/^Hunt:\d+$/.test(monster.keys[0])) return null;
                    return { map: world.map.id, rows: world.map.height, columns: world.map.width, profile: monster.profile.id, frame: monster.keys[0] };
                }, session, { timeout: 35000 });
                try { report.ordinaryMap = await view.jsonValue(); } finally { await view.dispose(); }
                assert.equal(String(report.ordinaryMap.map), '6');
                assert.equal(report.ordinaryMap.profile, 'monster_m1_flower');
                assert.equal(report.ordinaryMap.rows, 13);
                assert.equal(report.ordinaryMap.columns, 13);
            } finally { await session.dispose(); }
        };
        await page.goto(base.href);
        await scene('Login');
        await edit('Account', 'scope_' + crypto.randomBytes(5).toString('hex'));
        await edit('Password', crypto.randomBytes(8).toString('hex'));
        await tap('Register');
        for (const [index, entry] of entries.entries()) {
            await scene('ServerSelect');
            await tap('RegionList/View/Content', entry.region);
            await tap('ServerList/View/Content', entry.server);
            await scene('RoleSelect');
            // 场景标题先更新，角色目录回包后才用服务端数据替换场景占位文本。
            report.step = '等待角色目录 · ' + entry.server;
            await page.waitForFunction(name => {
                const title = cc.find('Canvas/Safe/Title')?.getComponent(cc.Label)?.string;
                const slot = cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string;
                return title?.startsWith(name + ' · ') && slot === '空角色槽';
            }, entry.server, { timeout: 30000 });
            const role = '路由' + crypto.randomBytes(2).toString('hex');
            await edit('Name', role);
            await tap('Create');
            await page.waitForFunction(name => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string.includes(name), role);
            assert.equal(createdOrigins[index], entry.origin);
            const before = connections.length;
            await tap('Enter');
            await scene('Game');
            await page.waitForFunction(() => cc.director.getScene().getComponentsInChildren('AppRoot')[0]?.reconciliationStarted,
                null, { timeout: 30000 });
            const opened = connections.slice(before);
            assert.deepEqual(opened, [{ endpoint: entry.endpoint, ready: true }]);
            assert.equal(ticketResponses.length, index + 1);
            const route = await ticketResponses[index];
            assert.deepEqual(route, { run: entry.run, node: entry.node, endpoint: entry.endpoint });
            report.reached.push({ region: entry.region, server: entry.server, origin: createdOrigins[index], ...route, ready: true });
            if (index === 0 && travelBenchmark) {
                report.step = '测量五处目标地图及原路返回';
                report.travelBenchmark = await measureTravel(page, tap, travelBenchmark, path.dirname(manifestPath),
                    process.env.MINIMIR_EXPECT_REGIONAL_MAP_STYLES === '1');
            }
            if (bossChallenges && index === 0) await verifyBossChallenges();
            if (tasksCodex && index === 0) await verifyTasks();
            if (index === 0) await verifyOrdinaryMap();
            if (entry !== entries.at(-1)) {
                await tap('BottomHUD/Footer/ChangeRole');
                await scene('RoleSelect');
                await tap('BackServers');
            }
        }
        assert.deepEqual(createdOrigins, entries.map(entry => entry.origin));
        assert.deepEqual(report.errors, []);
        delete report.step;
        report.passed = true;
    } catch (error) {
        if (page) report.observed = await page.evaluate(() => {
            const scene = window.cc?.director?.getScene()?.name;
            const rows = name => cc.find('Canvas/Safe/' + name + '/View/Content')?.children.map(
                node => node.getChildByName('Text')?.getComponent(cc.Label)?.string).filter(Boolean);
            const tasks = cc.find('Canvas/Safe/UIMgr/Panels/TasksCodex');
            return { scene, status: cc.find('Canvas/Safe/Status')?.getComponent(cc.Label)?.string,
                title: cc.find('Canvas/Safe/Title')?.getComponent(cc.Label)?.string,
                firstSlot: cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string,
                tasksVisible: tasks?.activeInHierarchy, taskDetail: tasks?.getChildByName('Detail')?.getComponent(cc.Label)?.string,
                regions: rows('RegionList'), servers: rows('ServerList') };
        }).catch(() => null);
        throw new Error(`${report.step}: ${error.message}${report.observed ? ' ' + JSON.stringify(report.observed) : ''}`);
    } finally {
        await browser.close();
        fs.writeFileSync(path.join(path.dirname(manifestPath), 'server-directory-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
    }
}

main().catch(error => { console.error(error.message); process.exitCode = 1; });
