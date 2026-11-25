// 在独立真实账号服、游戏服与 Creator 页面上触屏完成挑战待领奖励。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const manifest = path.resolve(process.argv[2] ?? '');
assert(manifest.startsWith(path.resolve('.work') + path.sep));
const { baseUrl, gameURL, region, server, rule, build, sourceScene } = JSON.parse(fs.readFileSync(manifest, 'utf8'));
const base = new URL(baseUrl), game = new URL(gameURL);
assert(base.protocol === 'http:' && base.hostname === '127.0.0.1' && base.port && base.port !== '18080');
assert(game.protocol === 'ws:' && game.hostname === '127.0.0.1' && game.pathname === '/ws' && game.port && !['18080', '18101', '18103'].includes(game.port));
assert(path.resolve(build).startsWith(path.resolve('.work') + path.sep));
const evidence = path.dirname(manifest), boss = 'Canvas/Safe/UIMgr/Panels/BossChallenge';
const bag = 'Canvas/Safe/UIMgr/Panels/Inventory', overlay = 'Canvas/Safe/PopupView/Panels';
const reconnect = 'Canvas/ReconnectDialog';
const hex = value => Buffer.from(value).toString('hex');
const sha = value => crypto.createHash('sha256').update(value).digest('hex');

// 只读取需要验收的原始帧字段，账号密码和票据不会进入报告。
function frame(raw) {
    if (typeof raw === 'string' || raw.length < 6 || raw[0] !== 1) return null;
    let offset = 4;
    const integer = () => {
        let value = 0n;
        for (let shift = 0n; shift < 70n; shift += 7n) {
            const next = raw[offset++];
            if (next === undefined) throw Error('收到不完整WS帧头');
            value |= BigInt(next & 127) << shift;
            if (!(next & 128)) return value.toString();
        }
        throw Error('WS帧头整数溢出');
    };
    return { id: raw.readUInt16BE(2), request: integer(), binding: integer(), error: raw[1] & 1 ? Number(integer()) : 0, body: raw.subarray(offset) };
}

const sceneAsset = JSON.parse(fs.readFileSync(sourceScene, 'utf8'));
const panel = sceneAsset.find(node => node.__type__ === 'cc.Node' && node._name === 'BossChallenge');
const savedReward = panel._children.map(value => sceneAsset[value.__id__]).find(node => node._name === 'Rewards');
assert(savedReward && savedReward._components.some(ref => sceneAsset[ref.__id__].__type__ === 'cc.Button'));

async function main() {
    const report = { passed: false, rule, bundleSha256: sha(fs.readFileSync(path.join(build, 'assets/main/index.js'))),
        sceneSha256: sha(fs.readFileSync(sourceScene)), savedRewardButton: true, connections: [], settlements: [], claims: [], errors: [], httpErrors: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--no-sandbox', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true,
        deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    let session;
    const observe = fn => event => {
        try { const packet = frame(event.payload); if (packet) fn(packet); }
        catch (error) { report.errors.push(error.message); }
    };
    const operations = new Map();
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('response', response => {
        if (response.status() >= 400 && new URL(response.url()).pathname !== '/favicon.ico') report.httpErrors.push({ url: response.url(), status: response.status() });
    });
    page.on('requestfailed', request => report.httpErrors.push({ url: request.url(), error: request.failure()?.errorText }));
    page.on('websocket', socket => {
        report.connections.push(socket.url());
        socket.on('framesent', observe(packet => {
            if (packet.id !== 40030) return;
            const request = pb.ClaimChallengeReward.decode(packet.body);
            const claim = { reward: hex(request.reward_id), operation: hex(request.operation.op_id), binding: packet.binding };
            report.claims.push(claim);
            operations.set(`${packet.binding}/${packet.request}`, claim);
        }));
        socket.on('framereceived', observe(packet => {
            if (packet.error || packet.id !== 20014 && packet.id !== 40005) return;
            if (packet.id === 20014) {
                const result = pb.PersonalBossState.decode(packet.body);
                if (result.state >= 4) report.settlements.push({ id: result.challenge_id, state: result.state, error: result.error_code });
                return;
            }
            const claim = operations.get(`${packet.binding}/${packet.request}`);
            if (!claim) return;
            const result = pb.OperationResult.decode(packet.body);
            assert.equal(hex(result.op_id), claim.operation);
            claim.error = result.error_code;
            claim.pending = result.pending;
        }));
    });

    const step = name => { report.step = name; };
    const scene = name => page.waitForFunction(name => {
        const current = window.cc?.director?.getScene();
        return current?.name === name && current.getComponentsInChildren('AppRoot').length && cc.find('Canvas/Safe')?.activeInHierarchy
            && !cc.find('Canvas/Loading')?.activeInHierarchy;
    }, name, { timeout: 45_000 });
    const touch = async (key, label) => {
        const value = await page.waitForFunction(({ key, label }) => {
            const root = window.cc?.find(key);
            const node = label === undefined ? root : root?.children.find(child => child.getChildByName('Text')?.getComponent(cc.Label)?.string?.replace(/^● /u, '') === label);
            if (!node?.activeInHierarchy || node.getComponent(cc.Button)?.interactable === false) return null;
            const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
            if (key.includes('/BossChallenge/') && app?.uiActionBusy) return null;
            const b = node._uiProps.uiTransformComp.getBoundingBoxToWorld();
            const size = cc.view.getVisibleSize(), rect = document.querySelector('canvas').getBoundingClientRect();
            return { x: rect.x + (b.x + b.width / 2) / size.width * rect.width,
                y: rect.y + (size.height - b.y - b.height / 2) / size.height * rect.height };
        }, { key, label }, { timeout: 12_000 });
        const point = await value.jsonValue();
        await value.dispose();
        await page.touchscreen.tap(point.x, point.y);
    };
    const safe = (suffix, label) => touch('Canvas/Safe/' + suffix, label);
    const edit = async (name, value) => {
        await safe(name);
        const widget = page.locator('input:visible');
        await widget.waitFor({ timeout: 5000 });
        await widget.fill(value);
    };
    const hide = async node => {
        await page.touchscreen.tap(12, 200);
        await page.waitForFunction(node => !cc.find(node)?.activeInHierarchy, node, { timeout: 4000 });
    };
    const waitReady = async kind => {
        const signal = await page.waitForFunction(({ session, kind }) => {
            const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
            return session.ready && session.entry?.sceneKind === kind && !app?.attaching && app?.world?.isMapReady && app.canSendInput();
        }, { session, kind }, { timeout: 40_000 });
        await signal.dispose();
    };
    const bagCount = async (items, equipped) => {
        await page.waitForFunction(({ session, items, equipped }) => session.inventory?.capacity === 3 &&
            session.inventory.items?.length === items && session.inventory.equipped?.length === equipped,
        { session, items, equipped }, { timeout: 10_000 });
    };
    const bossStatus = text => page.waitForFunction(({ boss, text }) => cc.find(boss + '/Status')?.getComponent(cc.Label)?.string.includes(text),
        { boss, text }, { timeout: 9000 });
    const openRewards = async () => {
        await safe('BottomHUD/QuickNav/Boss');
        await touch(boss + '/Rewards');
        await bossStatus('待领取 1/1');
        return page.evaluate(boss => {
            const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
            const pending = app.boss.personal.pendingRewards[0];
            const toHex = value => Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
            const label = node => cc.find(boss + '/' + node).getComponent(cc.Label).string;
            return { reward: toHex(pending.rewardId), earned: pending.earnedAtUnixMs.toString(),
                items: pending.items.map(item => ({ instance: toHex(item.instanceId), id: item.itemId, bound: item.bound, quantity: item.quantity.toString() })),
                headline: label('Headline'), status: label('Status'), details: label('Details'), body: label('Results') };
        }, boss);
    };
    const chooseSword = async action => {
        await safe('BottomHUD/QuickNav/Inventory');
        const cell = await page.waitForFunction(bag => {
            const list = cc.find(bag + '/List')?.getComponent('List');
            const match = Array.from(list?.rows ?? []).find(([index, node]) => node.activeInHierarchy && list.items[index]?.itemId === 218);
            if (!match) return null;
            const b = match[1]._uiProps.uiTransformComp.getBoundingBoxToWorld();
            const v = cc.view.getVisibleSize(), rect = document.querySelector('canvas').getBoundingClientRect();
            return { x: rect.x + (b.x + b.width / 2) / v.width * rect.width, y: rect.y + (v.height - b.y - b.height / 2) / v.height * rect.height };
        }, bag, { timeout: 8000 });
        const point = await cell.jsonValue();
        await cell.dispose();
        await page.touchscreen.tap(point.x, point.y);
        await page.waitForFunction(overlay => cc.find(overlay + '/StatusDetails')?.activeInHierarchy, overlay, { timeout: 5000 });
        await hide(overlay + '/StatusDetails');
        await touch(bag + '/' + action);
        await touch(overlay + '/Confirm/ConfirmButton');
    };

    try {
        step('新账号、选服、建角并等待普通图');
        const bundle = await context.request.get(new URL('/assets/main/index.js', base).href);
        assert.equal(bundle.status(), 200);
        assert.equal(sha(await bundle.body()), report.bundleSha256);
        await page.goto(base.href);
        await scene('Login');
        await edit('Account', 'reward_' + crypto.randomBytes(4).toString('hex'));
        await edit('Password', crypto.randomBytes(8).toString('hex'));
        await safe('Register');
        await scene('ServerSelect');
        await safe('RegionList/View/Content', region);
        await safe('ServerList/View/Content', server);
        await scene('RoleSelect');
        await page.waitForFunction(() => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string === '空角色槽');
        const role = '奖包' + crypto.randomBytes(3).toString('hex');
        await edit('Name', role);
        await safe('Create');
        await page.waitForFunction(name => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string.includes(name), role);
        await safe('Enter');
        await scene('Game');
        session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
        await waitReady(1);
        await bagCount(3, 0);
        report.roleId = await page.evaluate(session => Array.from(session.role.key.roleId, x => x.toString(16).padStart(2, '0')).join(''), session);
        assert.equal(await page.evaluate(session => session.entry.resourceVersion.toString(), session), rule);
        const isAuto = await page.evaluate(session => session.hosting?.autoBattleEnabled, session);
        if (isAuto) {
            await safe('MapViewport/Actions/Auto');
            await page.waitForFunction(session => !session.hosting.autoBattleEnabled, session);
        }

        step('真实满包阻断入场，穿戴木剑只空一格');
        await safe('BottomHUD/QuickNav/Boss');
        await bossStatus('背包已满');
        assert.equal(await page.evaluate(boss => cc.find(boss + '/Action').getComponent(cc.Button).interactable, boss), false);
        report.admission = { initialBag: '3/3', blocked: '背包已满', afterEquip: '2/3' };
        await hide(boss);
        await chooseSword('Equip');
        await bagCount(2, 1);
        await hide(bag);

        step('真实触屏入场、普攻击杀、返回原图');
        await safe('BottomHUD/QuickNav/Boss');
        await bossStatus('可入场');
        await touch(boss + '/Action');
        await waitReady(3);
        await safe('MapViewport/Actions/Attack');
        await waitReady(1);
        await bagCount(2, 1);
        assert.equal(report.settlements.length, 1);
        assert.equal(report.settlements[0].state, 4);
        report.challenge = report.settlements[0].id;

        step('保存的待领取页显示时间与整笔两份物品');
        report.pending = await openRewards();
        assert.equal(report.pending.headline, '挑战奖励');
        assert.match(report.pending.details, /\d\d-\d\d \d\d:\d\d/);
        assert.match(report.pending.body, /木剑 ×1.*绑定/);
        assert.match(report.pending.body, /金创药\(小量\) ×2.*绑定/);
        assert.deepEqual(report.pending.items.map(item => [item.id, item.quantity, item.bound]), [[218, '1', true], [35, '2', true]]);
        report.rewardId = report.pending.reward;
        report.itemIds = report.pending.items.map(item => item.instance);
        await page.screenshot({ path: path.join(evidence, 'pending.png') });
        await touch(boss + '/Action');
        await page.waitForFunction(session => session.lastError?.includes('背包已满，奖励仍在待领取'), session, { timeout: 10_000 });
        report.fullMessage = await page.evaluate(session => session.lastError, session);
        await bagCount(2, 1);
        report.slotsOnRejectedClaim = 1;
        await bossStatus('待领取 1/1');
        assert.equal(report.claims[0].error, 3401);

        step('真实断线弹窗重新签票，由同角色读回原奖包');
        const originalBinding = await page.evaluate(session => session.connection.binding.toString(), session);
        await hide(boss);
        await page.evaluate(session => { session.connection.socket.close(); }, session);
        await touch(reconnect + '/Panel/Reconnect');
        await waitReady(1);
        await page.waitForFunction(path => !cc.find(path)?.activeInHierarchy, reconnect, { timeout: 10_000 });
        report.reconnected = await page.evaluate(session => ({ binding: session.connection.binding.toString(),
            role: Array.from(session.role.key.roleId, x => x.toString(16).padStart(2, '0')).join('') }), session);
        assert.notEqual(report.reconnected.binding, originalBinding);
        assert.equal(report.reconnected.role, report.roleId);
        assert.deepEqual(await openRewards(), report.pending);
        await hide(boss);

        step('真实分解让出空间后用新操作领取原奖包');
        await chooseSword('Recycle');
        await bagCount(1, 1);
        await hide(bag);
        assert.deepEqual(await openRewards(), report.pending);
        await touch(boss + '/Action');
        await bagCount(3, 1);
        await bossStatus('待领取 0/1');
        report.final = { bag: '3/3', equipped: 1, pending: '0/1' };
        assert.equal(await page.evaluate(boss => cc.find(boss + '/Action').getComponent(cc.Button).interactable, boss), false);
        const final = await page.evaluate(session => session.inventory.items.map(item => Array.from(item.instanceId, x => x.toString(16).padStart(2, '0')).join('')), session);
        for (const item of report.itemIds) assert.equal(final.filter(id => id === item).length, 1);
        assert.deepEqual(report.connections, [gameURL, gameURL]);
        assert.deepEqual(report.claims.map(value => [value.reward, value.error, value.pending]), [[report.rewardId, 3401, false], [report.rewardId, 0, false]]);
        assert.notEqual(report.claims[0].operation, report.claims[1].operation);
        await page.screenshot({ path: path.join(evidence, 'claimed.png') });
        assert.deepEqual(report.errors, []);
        assert.deepEqual(report.httpErrors, []);
        delete report.step;
        report.passed = true;
    } catch (error) {
        report.failure = error.stack;
        await page.screenshot({ path: path.join(evidence, 'failure.png') }).catch(() => {});
        console.error(error.message);
        process.exitCode = 1;
    } finally {
        fs.writeFileSync(path.join(evidence, 'browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await session?.dispose().catch(() => {});
        await browser.close();
    }
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
