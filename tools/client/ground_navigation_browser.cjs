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

function inspectTownAdjacentTap(destination) {
    const world = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world;
    const body = world.visuals.get(world.playerHandle).body[0].node;
    const bounds = body._uiProps.uiTransformComp.getBoundingBoxToWorld();
    const local = world.projection.project(destination.x, destination.y);
    const ui = world.world._uiProps.uiTransformComp.convertToWorldSpaceAR(new cc.Vec3(local.x, local.y));
    const box = document.querySelector('canvas').getBoundingClientRect();
    const visible = cc.view.getVisibleSize();
    return { x: box.left + ui.x * box.width / visible.width,
        y: box.top + (visible.height - ui.y) * box.height / visible.height,
        resolved: world.screenToGrid(ui),
        overlapsOwnBody: ui.x >= bounds.x && ui.x <= bounds.x + bounds.width && ui.y >= bounds.y && ui.y <= bounds.y + bounds.height };
}

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

// 同一运行服的另一个真实角色提供站姿纹理。逐帧寻找人体外框内的全透明和不透明触点。
async function inspectPeerTaps(name) {
    const app = cc.director.getScene().getComponentsInChildren('AppRoot')[0], world = app.world;
    const peer = Array.from(world.visuals.values()).find(visual => visual.state.entityKind === 1 && visual.state.displayName === name);
    const sprite = peer.body[0], transform = sprite.node._uiProps.uiTransformComp;
    const foot = peer.node.position, exact = peer.state.position;
    const rectangles = world.townNpcs.flatMap(npc => npc.layers.map(layer => layer.sprite.node._uiProps.uiTransformComp.getBoundingBoxToWorld()));
    const canvas = document.createElement('canvas'), pixels = canvas.getContext('2d', { willReadFrequently: true });
    const rasters = [];
    const keys = peer.profile.layers[0].actions.standing.directions[peer.orderDirection].frames;
    for (const key of new Set(keys)) {
        const frame = world.assets.frames.get(key), definition = world.manifest.frames[key];
        if (!frame || definition.declared_empty) throw new Error('城镇对照角色缺少站姿原画');
        const original = frame.original, rect = frame.rect, asset = (original?._texture ?? frame.texture).image;
        let bitmap = asset.data;
        const temporary = ![HTMLImageElement, ImageBitmap, HTMLCanvasElement].some(type => bitmap instanceof type);
        if (temporary) bitmap = await createImageBitmap(await (await fetch(asset.nativeUrl)).blob());
        canvas.width = rect.width; canvas.height = rect.height;
        pixels.drawImage(bitmap, original?._x ?? rect.x, original?._y ?? rect.y, rect.width, rect.height, 0, 0, rect.width, rect.height);
        if (temporary) bitmap.close();
        rasters.push({ left: definition.offset_x - 24 + (definition.width - rect.width) / 2 + frame.offset.x,
            top: 16 - definition.offset_y - (definition.height - rect.height) / 2 + frame.offset.y,
            width: rect.width, height: rect.height, data: pixels.getImageData(0, 0, rect.width, rect.height).data });
    }
    const box = document.querySelector('canvas').getBoundingClientRect(), visible = cc.view.getVisibleSize();
    const viewport = world.viewport._uiProps.uiTransformComp.getBoundingBoxToWorld();
    const inside = (point, rect, edge = 0) => point.x >= rect.x + edge && point.x <= rect.x + rect.width - edge
        && point.y >= rect.y + edge && point.y <= rect.y + rect.height - edge;
    const actorBox = transform.getBoundingBoxToWorld();
    let clear, body;
    for (let y = 2; y < transform.height - 2 && !(clear && body); y += 2) {
        for (let x = 2; x < transform.width - 2 && !(clear && body); x += 2) {
            const localX = sprite.node.position.x + x + .5, localY = sprite.node.position.y - y - .5;
            const ui = world.world._uiProps.uiTransformComp.convertToWorldSpaceAR(new cc.Vec3(foot.x + localX, foot.y + localY));
            if (!inside(ui, viewport, 12) || !inside(ui, actorBox, 2) || rectangles.some(rect => inside(ui, rect))) continue;
            const alpha = [];
            for (const raster of rasters) {
                const column = Math.floor(localX - raster.left), row = Math.floor(raster.top - localY);
                if (column < 1 || row < 1 || column >= raster.width - 1 || row >= raster.height - 1) break;
                for (let dy = -1; dy <= 1; dy++) for (let dx = -1; dx <= 1; dx++) {
                    alpha.push(raster.data[((row + dy) * raster.width + column + dx) * 4 + 3]);
                }
            }
            if (alpha.length !== rasters.length * 9) continue;
            const point = { x: box.x + ui.x * box.width / visible.width,
                y: box.y + (visible.height - ui.y) * box.height / visible.height,
                alphaMin: Math.min(...alpha), alphaMax: Math.max(...alpha) };
            if (!body && point.alphaMin === 255) body = point;
            if (!clear && point.alphaMax === 0) {
                const target = world.screenToGrid(ui), own = world.visuals.get(world.playerHandle).state.position;
                if (target && (target.x !== exact.x || target.y !== exact.y) && (target.x !== own.x || target.y !== own.y)) {
                    clear = { ...point, target };
                }
            }
        }
    }
    return { handle: peer.state.entityHandle, body, clear };
}

async function verifySharedTownGround(page, evidence, name) {
    const session = await page.evaluateHandle(async () => (await System.import('chunks:///_virtual/GameSession.ts')).session);
    try {
        const prior = evidence.intents.length;
        const observed = await page.waitForFunction(name => {
            const world = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world;
            return Array.from(world.visuals.values()).some(visual => visual.state.entityKind === 1 && visual.state.displayName === name
                && visual.pose?.action === 'standing' && visual.body[0]?.spriteFrame);
        }, name, { timeout: 7500 });
        await observed.dispose();
        const taps = await page.evaluate(inspectPeerTaps, name);
        assert(taps.body && taps.clear, '无法找到共享城镇角色的实图触点：' + JSON.stringify(taps));
        await page.touchscreen.tap(taps.body.x, taps.body.y);
        const selected = await page.waitForFunction(handle =>
            cc.director.getScene().getComponentsInChildren('AppRoot')[0].selectedTarget?.handle === handle,
        taps.handle, { timeout: 4500 });
        await selected.dispose();
        assert.equal(evidence.intents.length, prior);
        await page.touchscreen.tap(taps.clear.x, taps.clear.y);
        const arrived = await page.waitForFunction(({ session, target }) => {
            const nav = session.baseline?.navigationState, player = session.baseline?.entities.find(row => row.entityHandle === session.entry.playerHandle);
            return nav?.status === 2 && nav.destination?.x === target.x && nav.destination?.y === target.y
                && player?.position.x === target.x && player.position.y === target.y && nav.inputSequence;
        }, { session, target: taps.clear.target }, { timeout: 10000 }).catch(error => {
            throw new Error(error.message + ' ' + JSON.stringify({ taps, intents: evidence.intents.slice(prior) }));
        });
        try { assert.deepEqual(evidence.intents.slice(prior), [{ input: await arrived.jsonValue(), ...taps.clear.target }]); }
        finally { await arrived.dispose(); }
        return { bodyAlphaMin: taps.body.alphaMin, groundAlphaMax: taps.clear.alphaMax, destination: taps.clear.target, arrived: true };
    } finally { await session.dispose(); }
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
            const lineDeviation = route.reduce((total, point) => total + Math.abs(
                (destination.x - from.x) * (point.y - from.y) - (destination.y - from.y) * (point.x - from.x)), 0);
            if (options.lineDeviation !== undefined) {
                assert.equal(lineDeviation, options.lineDeviation, JSON.stringify({ from, destination, route, lineDeviation }));
            }
            samples.push({ from: { x: from.x, y: from.y }, destination, authoritativeSteps: steps, route,
                viewport: { x: Math.round(click.x), y: Math.round(click.y) }, ...(options.name ? { case: options.name } : {}),
                ...(options.lineDeviation !== undefined ? { lineDeviation } : {}) });
        } finally { await ready.dispose(); }
        return click;
    };
    try {
        const start = await observePosition();
        const origin = { x: start.x, y: start.y };
        if (town) {
            // 角色站在镜头中央时，正上方相邻格中心落在本人的身体矩形内。
            // 这里直接触摸格中心，避免原八方向用例主动绕开身体而掩盖近距离走位问题。
            const behind = { x: origin.x - 1, y: origin.y - 1 };
            await travel(behind, 1, { name: '跨过自己身体的一步点地', exactSteps: true, click: async () => {
                const point = await page.evaluate(inspectTownAdjacentTap, behind);
                assert(point.overlapsOwnBody, JSON.stringify(point));
                return point;
            } });
            await travel(origin, 1, { exactSteps: true });
            // 三座紧凑城镇的出生格周围两步皆为空地，可以精确比较来回的每一步。
            for (const [dx, dy] of [[0, -2], [2, -2], [2, 0], [2, 2], [0, 2], [-2, 2], [-2, 0], [-2, -2]]) {
                await travel({ x: origin.x + dx, y: origin.y + dy }, 2, { balanced: true });
                await travel(origin, 2, { balanced: true });
            }
            const map = await page.evaluate(() => {
                const { map } = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world;
                return { id: map.id, blocked: map.blocked };
            });
            // 旧服从出生格点仓库左上侧地面时会先走过目标列，再折返一格。
            // 同为六步的较直绕行保留原碰撞并降低全程离点击直线的偏差。
            assert.deepEqual(origin, { x: 8, y: 13 });
            const aroundWarehouse = { exactSteps: true, blocked: map.blocked, lineDeviation: 12, name: '仓库绕行不中途折返' };
            await travel({ x: 6, y: 8 }, 6, aroundWarehouse);
            await travel(origin, 6, aroundWarehouse);

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

module.exports = { observeGroundIntents, verifyGroundDirections, verifySharedTownGround };
