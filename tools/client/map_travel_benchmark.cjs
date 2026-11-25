// 本文件通过原地图菜单、权威线路和场景确认测量连续切图，记录实际挂载的地表。
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const destinations = [
    ['3', '比奇省', '银杏山谷 · 城镇', 17, 6],
    ['4', '比奇省', '边界村 · 城镇', 17, 6],
    ['5', '比奇省', '比奇城 · 城镇', 17, 9],
    ['6', '谷地', '谷地南坡 · 野外', 13, 0],
    ['7', '荒地', '蛛穴外围 · 野外', 13, 0],
];
const home = ['1', '比奇省', '林间战场 · 野外', 13, 0];
const menu = 'UIMgr/Panels/TownDirectory';

// 只观察成功的准备与提交帧编号，不记录任何协议正文、票据或账号数据。
function messageId(payload) {
    return Buffer.isBuffer(payload) && payload.length >= 6 && payload[0] === 1 && payload[1] === 0
        ? payload.readUInt16BE(2) : 0;
}

function observeTravel(page) {
    let sample;
    page.on('websocket', socket => {
        socket.on('framereceived', ({ payload }) => {
            if (sample && messageId(payload) === 20008 && !sample.planAt) sample.planAt = performance.now();
        });
        socket.on('framesent', ({ payload }) => {
            if (sample && messageId(payload) === 20009 && !sample.commitAt) sample.commitAt = performance.now();
        });
    });
    page.on('request', request => {
        if (sample && new URL(request.url()).pathname.startsWith('/assets/resources/')) sample.resourceRequests++;
    });
    return { start() { sample = { resourceRequests: 0 }; return sample; }, end() { sample = undefined; } };
}

async function measureTravel(page, tap, observe, reportDir, checkStyles) {
    const session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
    const results = [];
    const reportFile = path.join(reportDir, 'map-travel-report.json');
    const save = result => fs.writeFileSync(reportFile, JSON.stringify(result, null, 2) + '\n', 'utf8');
    const capture = async ({ id, size, npcs }) => {
        const view = await page.waitForFunction(({ session, id }) => {
            const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0], world = app?.world;
            if (!world?.isMapReady || app.attaching || !session.ready || session.travel.active || session.entry.roomId.toString() !== id) return null;
            const surface = world.world.getChildByPath('Surface').getComponent(cc.Sprite).spriteFrame;
            if (!surface || surface !== world.assets.background) return null;
            return { width: world.map.width, height: world.map.height, depth: world.map.soil_thickness_px,
                npcs: world.map.npcs?.length ?? 0, props: world.map.props?.length ?? 0, collision: world.map.blocked?.length ?? 0,
                frame: surface.uuid ?? surface._uuid, regional: world.assets.regionalFloor ?? null };
        }, { session, id }, { timeout: 35000 });
        try {
            const observed = await view.jsonValue();
            assert.equal(observed.width, size);
            assert.equal(observed.height, size);
            assert.equal(observed.depth, size === 13 ? 24 : 48);
            assert.equal(observed.npcs, npcs);
            assert.equal(observed.collision, npcs);
            assert.equal(observed.props, 0);
            assert.ok(observed.frame, '实际地图地板缺少原 SpriteFrame 身份');
            return observed;
        } finally { await view.dispose(); }
    };
    const travel = async ([id, region, label, size, npcs]) => {
        await tap('BottomHUD/Footer/Map');
        await tap(menu + '/RegionList/View/Content', region);
        const sample = observe.start();
        try {
            const selectedAt = await tap(menu + '/MapList/View/Content', label);
            const view = await capture({ id, size, npcs });
            const readyAt = performance.now();
            assert(selectedAt <= sample.planAt && sample.planAt <= sample.commitAt && sample.commitAt <= readyAt, '切图未观察到正确的计划、提交及就绪顺序');
            return { id, region, label, tapToReadyMs: Math.round(readyAt - selectedAt),
                planToCommitMs: Math.round(sample.commitAt - sample.planAt), resourceRequests: sample.resourceRequests, ...view };
        } catch (error) {
            const observed = await page.evaluate(session => {
                const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0], world = app?.world;
                return { map: session.entry?.roomId?.toString(), world: world?.map?.id, ready: session.ready,
                    viewReady: world?.isMapReady, attaching: app?.attaching, busy: app?.uiActionBusy,
                    travel: session.travel.phase, lastError: session.lastError };
            }, session).catch(() => null);
            save({ passed: false, results, failed: { id, label, ...sample, observed }, error: error.message });
            throw error;
        } finally { observe.end(); }
    };
    try {
        const initial = await capture({ id: home[0], size: home[3], npcs: home[4] });
        for (const destination of destinations) {
            const first = await travel(destination);
            if (checkStyles) {
                assert.match(first.regional, /^development\/map-styles\//);
                await page.screenshot({ path: path.join(reportDir, `map-${first.id}.png`) });
            }
            const back = await travel(home);
            assert.equal(back.frame, initial.frame);
            results.push({ first, back });
        }
        if (checkStyles) assert.equal(new Set([initial.frame, ...results.map(({ first }) => first.frame)]).size, 6);
        const result = { passed: true, initial, results };
        save(result);
        return result;
    } finally { await session.dispose(); }
}

module.exports = { observeTravel, measureTravel };
