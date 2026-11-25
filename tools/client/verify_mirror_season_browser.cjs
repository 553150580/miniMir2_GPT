// 通过已保存的 Creator 触屏面板验收赛季排名、两种绑定元宝奖励和隔离兑换。
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, lobby, confirm, wait, touch, label, login, openLobby, originalWorld, packet } =
    require('./verify_mirror_arena_browser.cjs');

const season = safe + 'UIMgr/Panels/MirrorSeason';
const confirmMessage = safe + 'PopupView/Panels/Confirm/Message';
const content = season + '/List/View/Content';
const bindingState = 'Canvas/ReconnectDialog/Panel/Reconnect';

const expectedBalance = (BigInt(expected.previousPoints) - BigInt(expected.price)).toString();
const expectedWallet = (BigInt(expected.initialBoundYuanbao) + BigInt(expected.rankReward) + BigInt(expected.participationReward)).toString();

function observe(page, errors) {
    const requests = new Map(), replies = new Map(), states = [];
    let sockets = 0;
    page.on('pageerror', error => errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) { errors.push('触屏验收连到非隔离游戏节点'); return; }
        sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            const codec = frame?.id === 40030 ? pb.ClaimChallengeReward : frame?.id === 40034 ? pb.MirrorArenaExchangeRequest : null;
            if (codec) requests.set(frame.binding + ':' + frame.request, { id: frame.id, data: codec.decode(frame.body) });
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 20026) states.push({ binding: frame.binding, data: pb.MirrorArenaState.decode(frame.body) });
            if (frame?.id !== 40005) return;
            const key = frame.binding + ':' + frame.request;
            if (requests.has(key)) replies.set(key, pb.OperationResult.decode(frame.body));
        });
    });
    return { requests, replies, states, get sockets() { return sockets; } };
}

async function wallet(page, session, amount) {
    return wait(page, ({ session, amount }) => {
        const value = session.wallet?.boundYuanbaoUnits?.toString();
        return value === amount ? value : null;
    }, { session, amount });
}

async function verifyVisibleRanks(page) {
    const rows = await wait(page, ({ content, rival, player }) => {
        const list = cc.find(content);
        const text = list?.children.filter(row => row.activeInHierarchy)
            .map(row => row.getChildByName('Text')?.getComponent(cc.Label)?.string);
        return text?.some(row => row?.includes(rival)) && text.some(row => row?.includes(player)) ? text : null;
    }, { content, rival: expected.rival, player: expected.attacker });
    assert(rows.some(text => text === `第 1 名　${expected.rival}　${expected.currentPoints} 分`));
    assert(rows.some(text => text === `第 2 名　${expected.attacker}　${expected.currentPoints} 分`));
}

// 读取 Creator 已绘制的两行、栅格字形边界及世界坐标，以验证保存布局和显示结果。
async function verifySeasonLayout(page, report, state, balance, status = '') {
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    const layout = await page.evaluate(path => {
        const panel = cc.find(path);
        if (!panel?.activeInHierarchy) throw new Error('赛季面板没有显示');
        const bounds = node => {
            // UITransform 整棵子树的包围盒会吞下溢出的子节点，必须单独计算节点自己的矩形。
            const t = node._uiProps.uiTransformComp;
            if (!t) throw new Error(`赛季节点 ${node.name} 缺少已保存的矩形`);
            const corners = [0, 1].flatMap(x => [0, 1].map(y =>
                t.convertToWorldSpaceAR(new cc.Vec3((x - t.anchorX) * t.width, (y - t.anchorY) * t.height))));
            return { left: Math.min(...corners.map(p => p.x)), right: Math.max(...corners.map(p => p.x)),
                bottom: Math.min(...corners.map(p => p.y)), top: Math.max(...corners.map(p => p.y)) };
        };
        const labels = Object.fromEntries(['Headline', 'Balance', 'Status'].map(name => {
            const node = panel.getChildByName(name), label = node.getComponent(cc.Label), t = node._uiProps.uiTransformComp;
            const data = label.textLayoutData, context = label.assemblerData?.context;
            const lines = Array.from(data.parsedString), scale = label.actualFontSize / label.fontSize;
            const glyphs = label.string && context ? lines.map((line, index) => {
                const glyph = context.measureText(line), x = data.startPosition.x;
                const y = data.startPosition.y + index * label.lineHeight * scale;
                return { left: x - glyph.actualBoundingBoxLeft, right: x + glyph.actualBoundingBoxRight,
                    top: y - glyph.actualBoundingBoxAscent, bottom: y + glyph.actualBoundingBoxDescent };
            }) : [];
            return [name, { bounds: bounds(node), visible: node.activeInHierarchy && label.enabledInHierarchy,
                width: t.width, height: t.height, lineHeight: label.lineHeight, fontSize: label.fontSize, lines, glyphs,
                canvas: { width: data.canvasSize.width, height: data.canvasSize.height } }];
        }));
        const origin = cc.view.getVisibleOrigin(), visible = cc.view.getVisibleSize();
        return { viewport: { width: innerWidth, height: innerHeight }, panel: bounds(panel), tabs: bounds(panel.getChildByName('Tabs')),
            screen: { left: origin.x, right: origin.x + visible.width, bottom: origin.y, top: origin.y + visible.height }, ...labels };
    }, season);
    report.layouts.push({ state, ...layout });
    const { Headline, Balance, Status, panel, screen, tabs } = layout;
    const fits = (inner, outer) => inner.left >= outer.left - 1 && inner.right <= outer.right + 1 &&
        inner.bottom >= outer.bottom - 1 && inner.top <= outer.top + 1;
    assert(fits(panel, screen), '赛季面板应该完整显示于游戏可见区域');
    assert(Headline.height >= 2 * Headline.lineHeight, '两行赛季概要需要完整行高');
    assert(Headline.bounds.bottom > Balance.bounds.top && Balance.bounds.bottom > Status.bounds.top &&
        Status.bounds.bottom > tabs.top, '赛季概要、余额、状态和页签之间需要留出间隔');
    const lines = [[Headline, ['本季第 2 名', `最近赛季结转 · 第 1 名 · ${expected.previousPoints} 分`]],
        [Balance, [`兑换积分 ${balance}`]], [Status, status ? [status] : []]];
    for (const [node, expectedLines] of lines) {
        assert(node.visible && fits(node.bounds, panel));
        if (!expectedLines.length) continue;
        assert.deepEqual(node.lines, expectedLines);
        assert(node.fontSize >= 24 && node.glyphs.length === expectedLines.length);
        for (const glyph of node.glyphs) {
            assert(glyph.right > glyph.left && glyph.bottom > glyph.top && glyph.left >= -1 && glyph.top >= -1 &&
                glyph.right <= node.canvas.width + 1 && glyph.bottom <= node.canvas.height + 1,
            `赛季文字字形超出画布：${expectedLines.join(' / ')}`);
        }
    }
}

async function openSeason(page) {
    const already = await page.evaluate(path => cc.find(path)?.activeInHierarchy === true, lobby);
    if (!already) await openLobby(page);
    await touch(page, lobby + '/SeasonLedger');
    await label(page, season + '/Title', '镜像赛季');
}

function verifyStandings(state) {
    assert(state?.season_settlement_enabled && state.own_rank === 2);
    assert.equal(state.season_points.toString(), expected.currentPoints);
    assert.deepEqual(state.standings.map(row => [row.rank, row.name, row.points.toString()]), [
        [1, expected.rival, expected.currentPoints], [2, expected.attacker, expected.currentPoints],
    ]);
    assert(BigInt(state.standings[0].last_match_at_unix_ms.toString()) < BigInt(state.standings[1].last_match_at_unix_ms.toString()));
    assert.equal(state.last_settlement.rank, 1);
    assert.equal(state.last_settlement.exchange_credit.toString(), expected.previousPoints);
    assert.equal(state.last_settlement.points.toString(), expected.previousPoints);
}

async function main() {
    const report = { passed: false, errors: [], layouts: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true,
        deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    const wire = observe(page, report.errors);
    let session;
    try {
        report.step = '角色登录选择原入口，查看同分排序与旧赛季结转';
        session = await login(page, 'attacker');
        const original = await originalWorld(page, session);
        assert.equal(original.map, expected.normalMap);
        assert.equal(original.rule, expected.ruleVersion);
        const initialWallet = await wallet(page, session, expected.initialBoundYuanbao);
        assert.equal(await page.evaluate(session => session.inventory?.items.some(item => item.sourceKind === 'mirror_arena_exchange'), session),
            false, '测试角色入场时应无兑换来源物品');
        await openSeason(page);
        await label(page, season + '/Headline', '本季第 2 名');
        await label(page, season + '/Headline', `最近赛季结转 · 第 1 名 · ${expected.previousPoints} 分`);
        await label(page, season + '/Balance', `兑换积分 ${expected.previousPoints}`);
        await verifySeasonLayout(page, report, '结转', expected.previousPoints);
        await verifyVisibleRanks(page);
        const initial = wire.states.at(-1)?.data;
        verifyStandings(initial);
        assert.equal(initial.exchange_price.toString(), expected.price);
        assert.equal(initial.exchange_pack_quantity.toString(), expected.quantity);
        assert.equal(initial.exchange_item_id, expected.itemId);
        const pending = initial.season_pending_rewards;
        assert.deepEqual(new Map(pending.map(row => [row.source_kind, row.bound_yuanbao_units.toString()])),
            new Map([['mirror_season_rank', expected.rankReward], ['mirror_season_participation', expected.participationReward]]));
        report.ranking = { currentRank: initial.own_rank, previousRank: initial.last_settlement.rank, settledPoints: expected.previousPoints };
        await page.screenshot({ path: path.join(output, 'season-standings.png') });

        report.step = '触屏分别确认排名奖、参与奖，核对两个原附件编号';
        await touch(page, season + '/Tabs/Rewards');
        await page.screenshot({ path: path.join(output, 'season-rewards.png') });
        for (const name of ['赛季排名奖', '赛季参与奖']) {
            await touch(page, content, name);
            await label(page, season + '/Detail', name === '赛季参与奖' ? '赛季参与奖 · 绑定元宝' : '第 1 名 · 绑定元宝');
            await touch(page, season + '/Claim');
            await label(page, confirmMessage, '领取' + name);
            await touch(page, confirm);
            await label(page, season + '/Status', '已领取赛季奖励');
        }
        await label(page, season + '/Detail', '暂无待领奖项');
        await wallet(page, session, expectedWallet);
        const claims = Array.from(wire.requests).filter(([, row]) => row.id === 40030);
        assert.equal(claims.length, 2);
        const claimed = [];
        for (const [key, { data }] of claims) {
            const reply = wire.replies.get(key);
            assert(reply && reply.error_code === 0 && Buffer.from(reply.op_id).equals(Buffer.from(data.operation.op_id)));
            claimed.push(Buffer.from(data.reward_id).toString('hex'));
        }
        assert.deepEqual(claimed.sort(), pending.map(row => Buffer.from(row.reward_id).toString('hex')).sort());
        report.claim = { sent: claims.length, confirmed: true, initialWallet, wallet: expectedWallet };

        report.step = '按实际物品名称和权威报价触屏兑换，确认绑定实例';
        await touch(page, season + '/Exchange');
        await label(page, confirmMessage, `花费 ${expected.price} 兑换积分，换取绑定${expected.itemName} ×${expected.quantity}`);
        await touch(page, confirm);
        await label(page, season + '/Status', '兑换已确认');
        await label(page, season + '/Balance', `兑换积分 ${expectedBalance}`);
        const trades = Array.from(wire.requests).filter(([, row]) => row.id === 40034);
        assert.equal(trades.length, 1);
        const [requestKey, { data: request }] = trades[0];
        const result = wire.replies.get(requestKey);
        assert(result && result.error_code === 0 && result.mirror_arena_exchange);
        assert(Buffer.from(result.op_id).equals(Buffer.from(request.operation.op_id)));
        assert.deepEqual([request.pack_count, request.expected_item_id, request.expected_price.toString(), request.expected_pack_quantity.toString()],
            [1, expected.itemId, expected.price, expected.quantity]);
        const receipt = result.mirror_arena_exchange;
        assert.deepEqual([receipt.item_id, receipt.quantity.toString(), receipt.spent_points.toString(), receipt.remaining_points.toString()],
            [expected.itemId, expected.quantity, expected.price, expectedBalance]);
        const instance = Array.from(receipt.item_instance_id);
        assert.equal(instance.length, 16);
        const inventory = () => wait(page, ({ session, instance, expected }) => session.inventory?.items.some(item =>
            item.itemId === expected.itemId && item.bound && item.quantity.toString() === expected.quantity &&
            item.sourceKind === 'mirror_arena_exchange' && item.instanceId.length === instance.length &&
            Array.from(item.instanceId).every((byte, index) => byte === instance[index])
        ), { session, instance, expected });
        await inventory();
        report.exchange = { item: expected.itemName, price: expected.price, quantity: expected.quantity, balance: expectedBalance, bound: true };
        await verifySeasonLayout(page, report, '结算反馈', expectedBalance, '兑换已确认');
        await page.screenshot({ path: path.join(output, 'season-exchange-complete.png') });

        report.step = '窄屏显示完整两行概要、兑换余额与结果反馈';
        await page.setViewportSize({ width: 390, height: 844 });
        await verifySeasonLayout(page, report, '窄屏', expectedBalance, '兑换已确认');
        await page.screenshot({ path: path.join(output, 'season-narrow.png') });
        await page.setViewportSize({ width: 720, height: 1280 });

        report.step = '掉线新票后刷新赛季，核对原排名、钱包和绑定实例';
        await page.evaluate(session => session.connection.socket.close(4001, '隔离赛季触屏重连'), session);
        await touch(page, bindingState);
        const reconnected = await wait(page, ({ session, old }) => {
            const binding = session.connection.binding.toString();
            return session.ready && BigInt(binding) > BigInt(old) ? binding : null;
        }, { session, old: original.binding }, 26000);
        if (!await page.evaluate(path => cc.find(path)?.activeInHierarchy === true, season)) await openSeason(page);
        await touch(page, season + '/Refresh');
        await label(page, season + '/Balance', `兑换积分 ${expectedBalance}`);
        await label(page, season + '/Headline', `最近赛季结转 · 第 1 名 · ${expected.previousPoints} 分`);
        await wallet(page, session, expectedWallet);
        await inventory();
        const final = wire.states.findLast(row => row.binding === reconnected && row.data.season_settlement_enabled)?.data;
        verifyStandings(final);
        assert.equal(final.exchange_points.toString(), expectedBalance);
        assert.equal(final.season_pending_rewards.length, 0);
        assert.equal(wire.sockets, 2);
        assert.equal(wire.requests.size, 3);
        assert.deepEqual(report.errors, []);
        report.reconnect = { sameRole: (await originalWorld(page, session)).role === original.role, newBinding: true,
            sameIngot: true, confirmedBoundItem: true, balance: expectedBalance };
        assert(report.reconnect.sameRole);
        report.passed = true;
        delete report.step;
    } catch (error) {
        report.error = error.message;
        await page.screenshot({ path: path.join(output, 'season-failed.png') }).catch(() => {});
        report.observed = await page.evaluate(({ lobby, season }) => {
            const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0], value = sub =>
                cc.find(season + '/' + sub)?.getComponent(cc.Label)?.string;
            return { scene: cc.director.getScene()?.name, menu: app?.ui?.top?.name, lobby: cc.find(lobby)?.activeInHierarchy,
                ledger: cc.find(season)?.activeInHierarchy, headline: value('Headline'), balance: value('Balance'), status: value('Status') };
        }, { lobby, season }).catch(() => null);
        report.wire = { requests: Array.from(wire.requests.values(), row => row.id), responseCodes: Array.from(wire.replies.values(), result => result.error_code),
            seasonResponses: wire.states.filter(row => row.data.season_settlement_enabled).length, sockets: wire.sockets };
        throw new Error(report.step + '：' + error.message + ' ' + JSON.stringify(report.observed));
    } finally {
        await session?.dispose();
        await browser.close();
        fs.writeFileSync(path.join(output, 'mirror-season-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
    }
    console.log('触屏核对两行结转与窄屏、两种原绑定元宝附件和配置绑定兑换');
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
