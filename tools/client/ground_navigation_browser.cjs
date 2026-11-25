// 通过实际菱形地图触屏和 WebSocket 坐标核对八个方向的地面导航。
const assert = require('node:assert/strict');
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;

function observeGroundIntents(page) {
    const intents = [];
    const positions = [];
    const body = (payload, message) => {
        if (!Buffer.isBuffer(payload) || payload.length < 6 || payload[0] !== 1 ||
            payload[1] !== 0 || payload.readUInt16BE(2) !== message) return null;
        let offset = 4;
        for (let index = 0; index < 2; index++) while (offset < payload.length && payload[offset++] & 0x80) {}
        return payload.subarray(offset);
    };
    page.on('websocket', socket => {
        socket.on('framesent', ({ payload }) => {
            const packet = body(payload, 20006);
            if (!packet) return;
            const move = pb.MoveIntent.decode(packet);
            if (move.destination) intents.push({ input: move.input_sequence, x: move.destination.x, y: move.destination.y });
        });
        socket.on('framereceived', ({ payload }) => {
            const packet = body(payload, 5);
            if (!packet) return;
            const baseline = pb.SceneBaselineChunk.decode(packet);
            const player = baseline.entities.find(row => row.entity_kind === 1);
            const nav = baseline.navigation_state;
            if (!player || !nav?.destination) return;
            positions.push({ input: nav.input_sequence, x: player.position.x, y: player.position.y,
                revision: player.movement_revision.toString() });
        });
    });
    return { intents, positions };
}

// 这些可走格的触点位于对应原画的透明边内，直达路会经过该NPC的阻挡拐角。
const townNpcWitnesses = {
    3: { id: '3005', target: { x: 12, y: 8 } },
    4: { id: '4005', target: { x: 12, y: 8 } },
    5: { id: '5007', target: { x: 10, y: 8 } },
};

async function inspectNpcTap({ witness, body }) {
    const world = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world;
    const npc = world.townNpcs.find(view => view.npc.id === witness.id);
    if (!npc) throw new Error('缺少城镇触摸取样 NPC');
    const foot = world.projection.project(npc.npc.position.x, npc.npc.position.y);
    const point = body ? { x: foot.x, y: foot.y + 34 } : world.projection.project(witness.target.x - .27, witness.target.y - .27);
    const ui = world.world._uiProps.uiTransformComp.convertToWorldSpaceAR(new cc.Vec3(point.x, point.y));
    const local = { x: point.x - foot.x, y: point.y - foot.y };
    const canvas = document.createElement('canvas');
    const pixels = canvas.getContext('2d', { willReadFrequently: true });
    const textures = new Map();
    const alphas = [];
    // 从实际加载的纹理裁片读取原像素，独立于应用的点击命中实现。
    for (const { frames } of npc.layers) {
        for (const key of frames) {
            const frame = world.assets.frames.get(key);
            const definition = world.manifest.frames[key];
            const x = Math.floor(local.x - definition.offset_x + 24);
            const y = Math.floor(16 - definition.offset_y - local.y);
            const { rect, texture } = frame;
            if (x < 1 || y < 1 || x + 1 >= rect.width || y + 1 >= rect.height) {
                throw new Error(`NPC ${witness.id} 触点没有落在站立帧 ${key} 的矩形内部`);
            }
            if (!textures.has(texture)) {
                const asset = texture.image;
                const source = asset.data;
                const drawable = source instanceof HTMLImageElement || source instanceof ImageBitmap || source instanceof HTMLCanvasElement;
                textures.set(texture, drawable ? source : await createImageBitmap(await (await fetch(asset.nativeUrl)).blob()));
            }
            canvas.width = 3;
            canvas.height = 3;
            pixels.drawImage(textures.get(texture), rect.x + x - 1, rect.y + y - 1, 3, 3, 0, 0, 3, 3);
            const rgba = pixels.getImageData(0, 0, 3, 3).data;
            alphas.push(...Array.from(rgba).filter((_, index) => index % 4 === 3));
        }
    }
    const box = document.querySelector('canvas').getBoundingClientRect();
    const visible = cc.view.getVisibleSize();
    return { x: box.left + ui.x * box.width / visible.width,
        y: box.top + (visible.height - ui.y) * box.height / visible.height,
        resolved: world.screenToGrid(ui), name: npc.npc.name, alphaMin: Math.min(...alphas), alphaMax: Math.max(...alphas) };
}

async function verifyGroundDirections(page, { intents, positions }, town = false) {
    const session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
    const touches = await page.evaluateHandle(() => {
        const node = cc.find('Canvas');
        const observed = [];
        const record = event => observed.push({ type: event.type, target: event.target?.name,
            point: { x: event.getUILocation().x, y: event.getUILocation().y } });
        node.on('touch-start', record, undefined, true);
        node.on('touch-end', record, undefined, true);
        return { observed, release: () => {
            node.off('touch-start', record, undefined, true);
            node.off('touch-end', record, undefined, true);
        } };
    });
    const samples = [];
    const observePosition = () => page.evaluate(session => {
        const player = session.baseline.entities.find(row => row.entityHandle === session.entry.playerHandle);
        return { x: player.position.x, y: player.position.y, revision: player.movementRevision.toString() };
    }, session);
    const waitForSettled = async () => {
        // 权威落格比插值早，下一次触摸用已完成移动的显示坐标。
        const settled = await page.waitForFunction(session => {
            const world = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world;
            const player = session.baseline.entities.find(row => row.entityHandle === session.entry.playerHandle);
            return world.playerPosition && Math.abs(world.playerPosition.x - player.position.x) < .001
                && Math.abs(world.playerPosition.y - player.position.y) < .001;
        }, session, { timeout: 4000 });
        await settled.dispose();
    };
    const travel = async (destination, minimumSteps, options = {}) => {
        await waitForSettled();
        const from = await observePosition();
        // 使用实际保存的视口、世界坐标及 Canvas 尺寸触摸格中心，服务端收到的坐标另行观察。
        const click = options.click ? await options.click() : await page.evaluate(destination => {
            const world = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world;
            const bodies = Array.from(world.visuals.values()).flatMap(value => [value.body[0]?.node, value.placeholder]);
            const npcParts = world.townNpcs.flatMap(view => view.layers.map(layer => layer.sprite.node));
            const boxes = [...bodies, ...npcParts].filter(node => node?.activeInHierarchy)
                .map(node => node._uiProps.uiTransformComp.getBoundingBoxToWorld());
            // 允许在同一地格内避开实际画在前面的身体。
            const candidates = [[0, 0], [.3, .3], [-.3, -.3], [.3, -.3], [-.3, .3]].map(([x, y]) => {
                const local = world.projection.project(destination.x + x, destination.y + y);
                return world.world._uiProps.uiTransformComp.convertToWorldSpaceAR(new cc.Vec3(local.x, local.y));
            });
            const ui = candidates.find(point => !boxes.some(box =>
                point.x >= box.x && point.x <= box.x + box.width && point.y >= box.y && point.y <= box.y + box.height));
            if (!ui) return { blockedByVisibleEntity: true, target: destination, candidates, boxes };
            const box = document.querySelector('canvas').getBoundingClientRect();
            const visible = cc.view.getVisibleSize();
            return { x: box.left + ui.x * box.width / visible.width,
                y: box.top + (visible.height - ui.y) * box.height / visible.height, ui: { x: ui.x, y: ui.y },
                resolved: world.screenToGrid(ui) };
        }, destination);
        assert(!click.blockedByVisibleEntity, JSON.stringify({ from, ...click }));
        assert.deepEqual(click.resolved, destination);
        const before = intents.length;
        const firstPosition = positions.length;
        await page.touchscreen.tap(click.x, click.y);
        const ready = await page.waitForFunction(({ session, destination }) => {
            const nav = session.baseline?.navigationState;
            const player = session.baseline?.entities.find(row => row.entityHandle === session.entry.playerHandle);
            if (nav?.status !== 2 || nav.destination?.x !== destination.x || nav.destination?.y !== destination.y ||
                player?.position.x !== destination.x || player.position.y !== destination.y) return null;
            return { input: nav.inputSequence, revision: player.movementRevision.toString() };
        }, { session, destination }, { timeout: 10000 }).catch(async error => {
            const actual = await page.evaluate(({ touches, session }) => {
                const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0];
                const nav = session.baseline?.navigationState;
                return { touches: touches.observed.slice(-6), nav: nav && { status: nav.status, destination: nav.destination },
                    client: app.nav.state, ready: session.ready, mapWidth: app.world.map.width, panel: app.ui?.top?.name };
            }, { touches, session });
            throw new Error(error.message + ' ' + JSON.stringify({ from, destination, click, intents: intents.slice(before),
                positions: positions.slice(firstPosition).slice(-8), actual }));
        });
        try {
            const result = await ready.jsonValue();
            assert.deepEqual(intents.slice(before), [{ input: result.input, ...destination }]);
            const initialRevision = BigInt(from.revision);
            const steps = Number(BigInt(result.revision) - initialRevision);
            const route = [...new Map(positions.slice(firstPosition).filter(row => row.input === result.input &&
                BigInt(row.revision) > initialRevision).map(row => [row.revision, { x: row.x, y: row.y }])).values()];
            // 后续方向始终保留真实怪物动态占格，验证每步和抵达；首个短程额外断言两步直达。
            assert(steps >= minimumSteps && steps === route.length, JSON.stringify({ from, destination, minimumSteps, steps, route }));
            let previous = from;
            const blocked = options.blocked ?? [];
            const isBlocked = (x, y) => blocked.some(point => point.x === x && point.y === y);
            for (const next of route) {
                const dx = next.x - previous.x;
                const dy = next.y - previous.y;
                assert.equal(Math.max(Math.abs(dx), Math.abs(dy)), 1);
                assert(!isBlocked(next.x, next.y), JSON.stringify({ previous, next, blocked }));
                if (dx && dy) assert(!isBlocked(next.x, previous.y) && !isBlocked(previous.x, next.y), JSON.stringify({ previous, next, blocked }));
                previous = next;
            }
            assert.deepEqual(route.at(-1), destination);
            if (options.balanced) {
                const expectedRoute = Array.from({ length: minimumSteps }, (_, index) => ({
                    x: Math.round(from.x + (destination.x - from.x) * (index + 1) / minimumSteps),
                    y: Math.round(from.y + (destination.y - from.y) * (index + 1) / minimumSteps),
                }));
                assert.deepEqual(route, expectedRoute);
            }
            if (options.exactSteps) assert.equal(steps, minimumSteps);
            samples.push({ from: { x: from.x, y: from.y }, destination, authoritativeSteps: steps, route,
                viewport: { x: Math.round(click.x), y: Math.round(click.y) }, ...(options.name ? { case: options.name } : {}) });
        } finally { await ready.dispose(); }
        return click;
    };
    try {
        const start = await observePosition();
        const origin = { x: start.x, y: start.y };
        if (town) {
            // 三座紧凑城镇的出生格周围两步皆为空地，可以精确比较来回的每一步。
            for (const [dx, dy] of [[0, -2], [2, -2], [2, 0], [2, 2], [0, 2], [-2, 2], [-2, 0], [-2, -2]]) {
                await travel({ x: origin.x + dx, y: origin.y + dy }, 2, { balanced: true });
                await travel(origin, 2, { balanced: true });
            }
            const map = await page.evaluate(() => {
                const { map } = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world;
                return { id: map.id, blocked: map.blocked };
            });
            const mixed = { balanced: true, blocked: map.blocked, name: '无遮挡混合斜率' };
            await travel({ x: origin.x + 3, y: origin.y - 2 }, 3, mixed);
            await travel(origin, 3, mixed);

            const witness = townNpcWitnesses[map.id];
            assert(witness, '城镇缺少NPC透明点击样本');
            const shop = 'Canvas/Safe/UIMgr/Panels/NpcShop';
            await waitForSettled();
            const actor = await page.evaluate(inspectNpcTap, { witness, body: true });
            assert(actor.alphaMin >= 128, JSON.stringify(actor));
            const before = intents.length;
            await page.touchscreen.tap(actor.x, actor.y);
            const opened = await page.waitForFunction(({ shop, name }) => cc.find(shop)?.activeInHierarchy &&
                cc.find(shop + '/Title')?.getComponent(cc.Label)?.string === name, { shop, name: actor.name }, { timeout: 4000 });
            await opened.dispose();
            assert.equal(intents.length, before);
            await page.touchscreen.tap(20, 200);
            const closed = await page.waitForFunction(shop => !cc.find(shop)?.activeInHierarchy, shop, { timeout: 4000 });
            await closed.dispose();
            const clear = await travel(witness.target, 6, { exactSteps: true, blocked: map.blocked, name: 'NPC透明边点地并绕开拐角',
                click: async () => {
                    const ground = await page.evaluate(inspectNpcTap, { witness, body: false });
                    assert.equal(ground.alphaMax, 0, JSON.stringify(ground));
                    return ground;
                } });
            assert(!await page.evaluate(shop => cc.find(shop)?.activeInHierarchy, shop));
            Object.assign(samples.at(-1), { npc: actor.name, bodyAlphaMin: actor.alphaMin, groundAlphaMax: clear.alphaMax });
            return samples;
        }
        // 首个近点正是旧透明矩形吞掉的地格，随后继续向下以便从更远处点击原点。
        await travel({ x: origin.x + 2, y: origin.y + 2 }, 2, { balanced: true });
        await travel({ x: origin.x + 4, y: origin.y + 4 }, 2);
        await travel(origin, 4);
        for (const [dx, dy] of [[3, -3], [-4, -4], [-3, 3], [3, 0], [0, 3], [-3, 0], [0, -3]]) {
            const steps = Math.max(Math.abs(dx), Math.abs(dy));
            await travel({ x: origin.x + dx, y: origin.y + dy }, steps);
            await travel(origin, steps);
        }
        return samples;
    } finally {
        await page.evaluate(touches => touches.release(), touches).catch(() => {});
        await touches.dispose();
        await session.dispose();
    }
}

module.exports = { observeGroundIntents, verifyGroundDirections };
