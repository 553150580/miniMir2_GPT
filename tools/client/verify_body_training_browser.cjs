// 使用 Creator 实包触屏读取炼体的权威报价、回执和生效的战斗属性。
const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const { expected, output, endpoint, safe, wait, touch, label, login, originalWorld, packet } = require('./verify_mirror_arena_browser.cjs');
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

const role = safe + 'UIMgr/Panels/Role';
const panel = safe + 'PopupView/Panels/ItemActions';
const attributes = safe + 'PopupView/Panels/StatusDetails';
const toHex = value => Buffer.from(value).toString('hex');
const screenshot = (page, name) => page.screenshot({ path: path.join(output, name + '.png') });

// 只记录本轮隔离角色的原经济响应。
function observe(page, report) {
    page.on('pageerror', error => report.errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) {
            report.errors.push('浏览器连接了隔离范围之外的节点');
            return;
        }
        report.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 40067 || frame?.id === 40069) report.requests.push(frame.id);
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id !== 40005) return;
            const result = pb.OperationResult.decode(frame.body), receipt = result.body_training;
            if (!receipt) return;
            report.receipts.push({ operation: toHex(result.op_id), quote: toHex(receipt.quote_id), error: result.error_code,
                pending: result.pending, track: receipt.terms.track, after: receipt.after_level, success: receipt.success,
                gold: receipt.terms.gold.toString(), boundGold: receipt.terms.bound_gold.toString(),
                yuanbao: receipt.terms.yuanbao_units.toString(), boundYuanbao: receipt.terms.bound_yuanbao_units.toString() });
        });
    });
}

async function observeRole(page, session, health, mana, hp, mp) {
    return wait(page, ({ session, health, mana, hp, mp }) => {
        const inventory = session.inventory, wallet = session.wallet;
        const player = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle);
        if (!inventory?.bodyTraining || !wallet || wallet.revision.lessThan(inventory.revision) || !player ||
            inventory.bodyTraining.healthLevel !== health || inventory.bodyTraining.manaLevel !== mana ||
            hp !== undefined && player.hpMax.toString() !== hp || mp !== undefined && player.mpMax.toString() !== mp) return null;
        return { health, mana, hp: player.hpMax.toString(), mp: player.mpMax.toString(),
            gold: wallet.gold.toString(), boundGold: wallet.boundGold.toString() };
    }, { session, health, mana, hp, mp });
}

// 从实际库存和角色基线核对两系百分比，以及独立保存的四个等级。
async function observeHits(page, session, damage, absorption) {
    return wait(page, ({ session, damage, absorption }) => {
        const inventory = session.inventory, wallet = session.wallet;
        const player = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle);
        const levels = inventory?.bodyTraining, stats = player?.combatAttributes;
        if (!levels || !stats || !wallet || wallet.revision.lessThan(inventory.revision) ||
            levels.damageLevel !== damage || levels.absorptionLevel !== absorption || levels.healthLevel !== 0 || levels.manaLevel !== 0 ||
            stats.expandedDamageBp !== damage * 100 || stats.expandedAbsorptionBp !== absorption * 100) return null;
        return { damage, absorption, hp: player.hpMax.toString(), mp: player.mpMax.toString(),
            gold: wallet.gold.toString(), boundGold: wallet.boundGold.toString() };
    }, { session, damage, absorption });
}

async function action(page, index, text) {
    await label(page, panel + '/Action' + index + '/Label', text);
    await touch(page, panel + '/Action' + index);
}

// 在手机列表选择系别，通过原报价和确认按钮提交。
async function firstRank(page, index, name, bound, ordinary, options = {}) {
    if (options.select) {
        const row = name + ' 0 /';
        await touch(page, panel + '/List/View/Content', row);
        await wait(page, ({ panel, row }) => cc.find(panel + '/List').getComponent('List').items
            .some(item => item.label?.includes('✓ ' + row)), { panel, row });
    }
    await action(page, index, options.select ? '查看报价' : name + '炼体');
    await label(page, panel + '/Title', '炼体报价');
    const lines = await page.evaluate(base => cc.find(base + '/List').getComponent('List').items, panel);
    const display = lines.join(' ');
    const currency = options.yuanbao ? '元宝' : '金币';
    for (const line of [name + ' 0 → 1级', '成功率 100%', '绑定' + currency + ' ' + bound, '普通' + currency + ' ' + ordinary]) {
        assert(display.includes(line), display);
    }
    if (options.bonus) assert(display.includes('属性额度 0% → ' + options.bonus), display);
    await screenshot(page, 'quote-' + name);
    await action(page, 1, '确认炼体');
    await label(page, panel + '/Title', '炼体成功');
}

async function observeMastery(page, session, expectedLevels) {
    return wait(page, ({ session, expectedLevels }) => {
        const inventory = session.inventory, wallet = session.wallet;
        const player = session.baseline?.entities.find(row => row.entityHandle === session.entry?.playerHandle);
        const levels = inventory?.bodyTraining, stats = player?.combatAttributes;
        if (!levels || !stats || !wallet || wallet.revision.lessThan(inventory.revision) ||
            [levels.criticalLevel, levels.speedLevel, levels.resistanceLevel].some((value, i) => value !== expectedLevels[i]) ||
            [levels.healthLevel, levels.manaLevel, levels.damageLevel, levels.absorptionLevel].some(value => value !== 0) ||
            [stats.criticalBp, stats.attackIntervalReductionBp, stats.allResistanceBp].some((value, i) => value !== expectedLevels[i] * [100, 50, 50][i])) return null;
        return { gold: wallet.gold.toString(), boundGold: wallet.boundGold.toString(),
            yuanbao: wallet.yuanbaoUnits.toString(), boundYuanbao: wallet.boundYuanbaoUnits.toString(), interval: player.attackIntervalMs };
    }, { session, expectedLevels });
}

// 从实际打开的战斗详情列表核对显示数值。
async function combatDetails(page, cases) {
    await page.touchscreen.tap(10, 130);
    await wait(page, node => !cc.find(node)?.activeInHierarchy, panel);
    await touch(page, role + '/Recycle');
    await label(page, attributes + '/Title', '战斗属性');
    return wait(page, ({ node, expected }) => {
        const rows = cc.find(node + '/List')?.getComponent('List').items;
        return rows && expected.every(text => rows.includes(text)) ? expected : null;
    }, { node: attributes, expected: cases });
}

// 用实包观察层回放两条数值事件，确认节点复用时恢复预制体原色。
async function damageStyleSample(page, session) {
    const style = await wait(page, model => {
        const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
        const numbers = app?.world?.damageNumbers;
        const character = model.baseline?.entities.find(row => row.entityHandle === model.entry?.playerHandle);
        if (!numbers || !character || !numbers.head(character.entityHandle)) return null;
        const at = cc.game.totalTime;
        const rgb = color => [color.r, color.g, color.b];
        const emit = critical => {
            const eventId = (numbers.lastEventId + 1n).toString();
            numbers.accept({ eventId, sceneEpoch: model.entry.sceneEpoch, targetHandle: character.entityHandle,
                targetLife: character.life, amount: critical ? '125' : '100', outcome: 1, healing: false, critical }, at);
            return numbers.active.find(row => row.eventId === eventId);
        };
        const critical = emit(true);
        if (!critical) throw new Error('暴击飘字实包样本未进入节点');
        const marked = rgb(critical.label.color);
        const original = rgb(critical.normalColor);
        numbers.recycle(numbers.active.indexOf(critical));
        const normal = emit(false);
        if (!normal) throw new Error('普通飘字实包样本未进入节点');
        return { marked, original, afterReuse: rgb(normal.label.color), reused: normal.node === critical.node };
    }, session);
    assert.deepEqual(style.marked, [255, 230, 140]);
    assert.notDeepEqual(style.marked, style.original);
    assert(style.reused);
    assert.deepEqual(style.afterReuse, style.original);
    return style;
}

async function main() {
    const report = { passed: false, enabled: expected.enabled, hits: expected.hits ?? false, mastery: expected.mastery ?? false, bundleSha256: expected.bundleSha256,
        fixtureSha256: expected.fixtureSha256, sockets: 0, errors: [], requests: [], receipts: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const context = await browser.newContext({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true,
        deviceScaleFactor: 1, serviceWorkers: 'block' });
    const page = await context.newPage();
    observe(page, report);
    try {
        const bundle = page.waitForResponse(response => response.status() === 200 &&
            response.url() === expected.baseUrl + '/assets/main/index.js', { timeout: 65000 });
        void bundle.catch(() => {});
        const session = await login(page, 'role', { account: process.env.MINIMIR_BODY_TRAINING_BROWSER_ACCOUNT,
            password: process.env.MINIMIR_BODY_TRAINING_BROWSER_PASSWORD });
        assert.equal(crypto.createHash('sha256').update(await (await bundle).body()).digest('hex'), expected.bundleSha256);
        const world = await originalWorld(page, session);
        assert.equal(world.role, expected.role);
        if (world.auto) {
            await touch(page, safe + 'MapViewport/Actions/Auto');
            await wait(page, model => model.hosting?.autoBattleEnabled === false, session);
        }
        await touch(page, safe + 'BottomHUD/QuickNav/Role');
        await touch(page, role + '/Message');
        if (expected.mastery) {
            await label(page, panel + '/Title', '炼体');
            report.before = await observeMastery(page, session, [0, 0, 0]);
            const trials = [['暴击', '15.0', '5.0', '1%'], ['攻速', '0.0', '20.0', '0.5%'], ['全抗', '0.0', '20.0', '0.5%']];
            for (const [name, bound, ordinary, bonus] of trials) {
                await firstRank(page, 1, name, bound, ordinary, { select: true, yuanbao: true, bonus });
                await action(page, 1, '返回炼体');
            }
            report.after = await observeMastery(page, session, [1, 1, 1]);
            assert.deepEqual([report.after.gold, report.after.boundGold, report.after.yuanbao, report.after.boundYuanbao],
                ['5000', '1000', '550', '0']);
            assert.equal(report.after.interval, report.before.interval - Math.floor(report.before.interval / 200));
            assert.deepEqual(report.requests, [40067, 40069, 40067, 40069, 40067, 40069]);
            assert.deepEqual(report.receipts.map(r => [r.track, r.success, r.after, r.gold, r.boundGold, r.yuanbao, r.boundYuanbao, r.error, r.pending]),
                [[5, true, 1, '0', '0', '50', '150', 0, false], [6, true, 1, '0', '0', '200', '0', 0, false],
                    [7, true, 1, '0', '0', '200', '0', 0, false]]);
            await screenshot(page, 'trained-mastery');
            report.attributes = await combatDetails(page, ['暴击率 1%', '普攻间隔缩减 0.5%', '全抗 0.5%', '幸运抵抗 0.5%']);
            await screenshot(page, 'combat-mastery');
            report.damageStyleSample = await damageStyleSample(page, session);
        } else if (expected.hits) {
            await label(page, panel + '/Title', '炼体');
            report.before = await observeHits(page, session, 0, 0);
            await firstRank(page, 1, '伤害吸收', '1000', '500', { select: true, bonus: '1%' });
            report.absorption = await observeHits(page, session, 0, 1);
            await action(page, 1, '返回炼体');
            await firstRank(page, 1, '伤害', '0', '3000', { select: true, bonus: '1%' });
            report.after = await observeHits(page, session, 1, 1);
            assert.deepEqual([report.after.gold, report.after.boundGold, report.after.hp, report.after.mp],
                ['2500', '0', report.before.hp, report.before.mp]);
            assert.deepEqual(report.requests, [40067, 40069, 40067, 40069]);
            assert.deepEqual(report.receipts.map(r => [r.track, r.success, r.after, r.error, r.pending]), [[2, true, 1, 0, false], [1, true, 1, 0, false]]);
            await action(page, 1, '返回炼体');
            await screenshot(page, 'trained-hits');
            await action(page, 2, '查看转生');
            await label(page, panel + '/Title', '角色成长');
            report.attributes = await combatDetails(page, ['伤害加成 1%', '伤害吸收 1%']);
            await screenshot(page, 'combat-hits');
        } else if (expected.enabled) {
            await label(page, panel + '/Title', '炼体');
            report.before = await observeRole(page, session, 0, 0);
            const hp = (BigInt(report.before.hp) + 2n).toString(), mp = (BigInt(report.before.mp) + 1n).toString();
            await firstRank(page, 1, '生命上限', '1000', '500');
            report.health = await observeRole(page, session, 1, 0, hp, report.before.mp);
            await action(page, 1, '返回炼体');
            await firstRank(page, 2, '魔法上限', '0', '1500');
            report.after = await observeRole(page, session, 1, 1, hp, mp);
            assert.deepEqual([report.after.gold, report.after.boundGold], ['3000', '0']);
            assert.deepEqual(report.requests, [40067, 40069, 40067, 40069]);
            assert.deepEqual(report.receipts.map(r => [r.track, r.success, r.after, r.error, r.pending]), [[3, true, 1, 0, false], [4, true, 1, 0, false]]);
            await screenshot(page, 'trained');
            await action(page, 1, '返回炼体');
            await action(page, 3, '查看转生');
            await label(page, panel + '/Title', '角色成长');
        } else {
            await wait(page, node => cc.find(node)?.activeInHierarchy, attributes);
            assert.equal(await page.evaluate(node => cc.find(node)?.activeInHierarchy, panel), false);
            assert.deepEqual(report.requests, []);
            await screenshot(page, 'default-growth');
        }
        assert.deepEqual(report.errors, []);
        assert(report.sockets >= 1);
        report.passed = true;
    } finally {
        fs.writeFileSync(path.join(output, 'body-training-browser-report.json'), JSON.stringify(report, null, 2) + '\n');
        await browser.close();
    }
}

main().catch(error => { console.error(error); process.exitCode = 1; });
