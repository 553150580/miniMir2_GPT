// 通过真实 Creator 移动触屏读取并领取两个已确认的世界首领奖，WebSocket 只观察原请求和返回。
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const manifest = path.resolve(process.argv[2] ?? ''), output = path.dirname(manifest);
assert(manifest.startsWith(path.resolve('.work') + path.sep), '触屏领奖清单必须在独立工作目录');
const expected = JSON.parse(fs.readFileSync(manifest, 'utf8'));
const base = new URL(expected.baseUrl), socket = new URL(expected.endpoint);
assert(base.protocol === 'http:' && base.hostname === '127.0.0.1' && base.port !== '18080');
assert(socket.protocol === 'ws:' && socket.hostname === '127.0.0.1' && socket.pathname === '/ws' && !['18101', '18103'].includes(socket.port));
assert.deepEqual(expected.players.map(player => player.rank), [1, 2]);

const safe = 'Canvas/Safe/';
const board = `${safe}UIMgr/Panels/BossChallenge`;
const hex = bytes => Buffer.from(bytes).toString('hex');

// 按实际保存节点的布局转换坐标，所有点击都从 Chromium 触摸输入经过 Cocos 的 Button。
async function touch(page, nodePath, choice) {
    const handle = await page.waitForFunction(({ nodePath, choice }) => {
        const root = cc.find(nodePath);
        const target = choice === undefined ? root : root?.children.find(child => child.activeInHierarchy &&
            child.getChildByName('Text')?.getComponent(cc.Label)?.string.replace(/^● /u, '') === choice);
        if (!target?.activeInHierarchy || target.getComponent(cc.Button)?.interactable === false) return null;
        const box = target._uiProps?.uiTransformComp?.getBoundingBoxToWorld();
        if (!box) return null;
        const size = cc.view.getVisibleSize(), canvas = document.querySelector('canvas').getBoundingClientRect();
        return { x: canvas.x + (box.x + box.width / 2) / size.width * canvas.width,
            y: canvas.y + (size.height - box.y - box.height / 2) / size.height * canvas.height };
    }, { nodePath, choice }, { timeout: 20000 });
    try { const point = await handle.jsonValue(); await page.touchscreen.tap(point.x, point.y); }
    finally { await handle.dispose(); }
}

async function scene(page, name) {
    await page.waitForFunction(name => {
        const current = window.cc?.director?.getScene();
        return current?.name === name && current.getComponentsInChildren('AppRoot').length &&
            cc.find('Canvas/Safe')?.activeInHierarchy && !cc.find('Canvas/Loading')?.activeInHierarchy;
    }, name, { timeout: name === 'Game' ? 60000 : 30000 });
}

async function edit(page, field, value) {
    assert(value, '缺少本轮独立角色凭据');
    await touch(page, safe + field);
    const input = page.locator('input:visible');
    await input.waitFor({ state: 'visible', timeout: 5000 });
    await input.fill(value);
}

// 帧头两处原 varint 分别是请求序号和连接绑定。证据仅保存与领奖直接相关的消息。
function header(payload) {
    let offset = 4;
    const readUnsigned = () => {
        let value = 0n, shift = 0n;
        while (offset < payload.length) {
            const byte = payload[offset++];
            value |= BigInt(byte & 0x7f) << shift;
            if (!(byte & 0x80)) return value.toString();
            shift += 7n;
        }
        throw new Error('领奖帧头缺失');
    };
    const request = readUnsigned(), binding = readUnsigned();
    return { key: `${binding}:${request}`, offset };
}

function wireItem(item) {
    return { instanceId: hex(item.instance_id), itemId: item.item_id, quantity: item.quantity.toString(), bound: item.bound,
        sourceKind: item.source_kind, sourceId: item.source_id };
}

// 一个页面完整执行重新选角、检查待领、点击领取和核对入包。
async function claimAs(browser, player, credentials, report) {
    const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, isMobile: true, hasTouch: true,
        deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    const requests = [], queries = [], responses = new Map();
    let connections = 0, gameSession;
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', ws => {
        if (ws.url() !== expected.endpoint) { report.errors.push('页面连接到本轮以外的游戏节点'); return; }
        connections++;
        ws.on('framesent', ({ payload }) => {
            if (!Buffer.isBuffer(payload) || payload.length < 6 || payload.readUInt16BE(2) !== 40030) return;
            const { key, offset } = header(payload);
            requests.push({ key, body: pb.ClaimChallengeReward.decode(payload.subarray(offset)) });
        });
        ws.on('framereceived', ({ payload }) => {
            if (!Buffer.isBuffer(payload) || payload.length < 6 || payload[1] & 1) return;
            const id = payload.readUInt16BE(2);
            if (id !== 20016 && id !== 40005) return;
            const { key, offset } = header(payload);
            if (id === 20016) queries.push(pb.PersonalBossInfo.decode(payload.subarray(offset)));
            else responses.set(key, pb.OperationResult.decode(payload.subarray(offset)));
        });
    });
    try {
        await page.goto(base.href);
        await scene(page, 'Login');
        await edit(page, 'Account', credentials.account);
        await edit(page, 'Password', credentials.password);
        await touch(page, `${safe}Login`);
        await scene(page, 'ServerSelect');
        await touch(page, `${safe}RegionList/View/Content`, expected.region);
        await touch(page, `${safe}ServerList/View/Content`, expected.server);
        await scene(page, 'RoleSelect');
        await page.waitForFunction(name => cc.find('Canvas/Safe/Role0/Label')?.getComponent(cc.Label)?.string.includes(name), player.name);
        await touch(page, `${safe}Enter`);
        await scene(page, 'Game');
        gameSession = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
        const attached = await page.waitForFunction(session => {
            const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
            if (!session.ready || !session.inventory || !session.hosting || !app?.world?.isMapReady || session.entry?.sceneKind !== 1) return null;
            return { identity: session.entryRole, binding: session.connection.binding.toString(), version: session.entry.resourceVersion.toString(),
                auto: session.hosting.autoBattleEnabled };
        }, gameSession, { timeout: 20000 });
        let attachment;
        try { attachment = await attached.jsonValue(); } finally { await attached.dispose(); }
        assert.equal(attachment.identity, player.identity);
        assert(BigInt(attachment.binding) > BigInt(player.binding));
        assert.equal(attachment.version, String(expected.version));
        if (attachment.auto) {
            await touch(page, `${safe}MapViewport/Actions/Auto`);
            await page.waitForFunction(session => session.hosting?.autoBattleEnabled === false, gameSession);
        }
        await touch(page, `${safe}BottomHUD/QuickNav/Boss`);
        await touch(page, `${board}/Rewards`);
        const displayed = await page.waitForFunction(({ board, headline }) => {
            const panel = cc.find(board);
            if (!panel?.activeInHierarchy) return null;
            const label = name => panel.getChildByPath(name)?.getComponent(cc.Label)?.string;
            const details = label('Details'), prize = label('Results'), action = panel.getChildByName('Action');
            return details?.includes(headline) && prize && action?.getComponent(cc.Button)?.interactable === true && label('Action/Label') === '领取'
                ? { details, prize, status: label('Status') } : null;
        }, { board, headline: `${expected.bossName} · 第${player.rank}名` }, { timeout: 15000 });
        let before;
        try { before = await displayed.jsonValue(); } finally { await displayed.dispose(); }
        assert(before.status.includes('世界 1'));
        for (const item of player.items) {
            const pattern = `${item.name} ×${item.quantity}  ${item.bound ? '绑定' : '非绑定'}`;
            assert(before.prize.split('\n').some(line => line === pattern || line.startsWith(pattern + '  品质 ')), `缺少界面物品 ${pattern}`);
        }
        const pending = queries.flatMap(query => query.world_pending_rewards).find(reward => hex(reward.reward_id) === player.rewardId);
        assert(pending);
        assert.equal(pending.source_kind, 'world_boss');
        assert.equal(pending.source_rank, player.rank);
        assert.equal(pending.template_id, player.template);
        assert.equal(pending.earned_at_unix_ms.toString(), player.earnedAt);
        assert.deepEqual(pending.items.map(wireItem), player.items.map(({ name, ...item }) =>
            ({ ...item, sourceKind: 'world_boss', sourceId: expected.occurrence })));
        await page.screenshot({ path: path.join(output, `world-boss-rank-${player.rank}-pending.png`) });
        assert.equal(requests.length, 0);
        await touch(page, `${board}/Action`);
        // 原奖从可领列表移除，恰好相同的实例、数量、绑定进入原角色的真实背包快照。
        await page.waitForFunction(({ session, board, items, occurrence }) => {
            const panel = cc.find(board), app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
            const pending = app?.boss?.personal?.worldPendingRewards;
            if (pending?.length || app?.boss?.busy) return false;
            const rewardTab = panel?.getChildByName('Rewards');
            const empty = rewardTab?.active === false || panel?.getChildByName('Results')?.getComponent(cc.Label)?.string === '暂无待领取奖励';
            const allReceived = items.every(original => (session.inventory?.items ?? []).filter(item => {
                const id = Array.from(item.instanceId, value => value.toString(16).padStart(2, '0')).join('');
                return original.instanceId === id && original.itemId === item.itemId && original.quantity === item.quantity.toString() &&
                    original.bound === item.bound && item.sourceKind === 'world_boss' && item.sourceId === occurrence;
            }).length === 1);
            return empty && allReceived;
        }, { session: gameSession, board, items: player.items, occurrence: expected.occurrence }, { timeout: 15000 });
        assert(queries.some(query => query.world_pending_rewards.length === 0 && !query.error_code), '领取后未从真实服务器读到空列表');
        assert.equal(connections, 1);
        assert.equal(requests.length, 1);
        const [{ key, body }] = requests, reply = responses.get(key);
        const opId = hex(body.operation.op_id);
        assert.equal(hex(body.reward_id), player.rewardId);
        assert.match(opId, /^[0-9a-f]{32}$/u);
        assert(reply && reply.error_code === 0 && !reply.pending && hex(reply.op_id) === opId);
        await page.screenshot({ path: path.join(output, `world-boss-rank-${player.rank}-claimed.png`) });
        return { identity: attachment.identity, binding: attachment.binding, rank: player.rank, rewardId: player.rewardId, opId,
            source: pending.source_kind, earnedAt: pending.earned_at_unix_ms.toString(), awardText: before.prize, attachment: before.details,
            items: pending.items.map((item, index) => ({ ...wireItem(item), name: player.items[index].name })) };
    } catch (error) {
        await page.screenshot({ path: path.join(output, `world-boss-rank-${player.rank}-failure.png`) }).catch(() => {});
        report.failedRank = player.rank;
        throw error;
    } finally {
        await gameSession?.dispose();
        await context.close();
    }
}

async function main() {
    const report = { passed: false, errors: [], occurrence: expected.occurrence, players: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    try {
        for (const [index, name] of ['WINNER', 'PARTNER'].entries()) {
            const team = `MINIMIR_WORLD_BOSS_REWARD_${name}`;
            const credentials = { account: process.env[`${team}_ACCOUNT`], password: process.env[`${team}_PASSWORD`] };
            report.players.push(await claimAs(browser, expected.players[index], credentials, report));
        }
        assert.deepEqual(report.errors, []);
        report.passed = true;
    } catch (error) {
        report.errors.push(error.message);
        throw error;
    } finally {
        fs.writeFileSync(path.join(output, 'world-boss-reward-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
        await browser.close();
    }
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
