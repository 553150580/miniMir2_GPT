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
    const travel = async (destination, minimumSteps, requireStraight = false) => {
        // 权威落格比插值早，下一次触摸用已完成移动的显示坐标。
        const settled = await page.waitForFunction(session => {
            const world = cc.director.getScene().getComponentsInChildren('AppRoot')[0].world;
            const player = session.baseline.entities.find(row => row.entityHandle === session.entry.playerHandle);
            return world.playerPosition && Math.abs(world.playerPosition.x - player.position.x) < .001
                && Math.abs(world.playerPosition.y - player.position.y) < .001;
        }, session, { timeout: 4000 });
        await settled.dispose();
        const from = await observePosition();
        // 使用实际保存的视口、世界坐标及 Canvas 尺寸触摸格中心，服务端收到的坐标另行观察。
        const click = await page.evaluate(destination => {
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
                    client: app.nav.state, ready: session.ready, mapWidth: app.world.map.width };
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
            for (const next of route) {
                assert.equal(Math.max(Math.abs(previous.x - next.x), Math.abs(previous.y - next.y)), 1);
                previous = next;
            }
            assert.deepEqual(route.at(-1), destination);
            if (requireStraight) {
                const expectedRoute = Array.from({ length: minimumSteps }, (_, index) => ({
                    x: from.x + (destination.x - from.x) * (index + 1) / minimumSteps,
                    y: from.y + (destination.y - from.y) * (index + 1) / minimumSteps,
                }));
                assert.deepEqual(route, expectedRoute);
            }
            samples.push({ from: { x: from.x, y: from.y }, destination, authoritativeSteps: steps, route,
                viewport: { x: Math.round(click.x), y: Math.round(click.y) } });
        } finally { await ready.dispose(); }
    };
    try {
        const start = await observePosition();
        const origin = { x: start.x, y: start.y };
        if (town) {
            // 三座紧凑城镇的出生格周围两步皆为空地，可以精确比较来回的每一步。
            for (const [dx, dy] of [[0, -2], [2, -2], [2, 0], [2, 2], [0, 2], [-2, 2], [-2, 0], [-2, -2]]) {
                await travel({ x: origin.x + dx, y: origin.y + dy }, 2, true);
                await travel(origin, 2, true);
            }
            return samples;
        }
        // 首个近点正是旧透明矩形吞掉的地格，随后继续向下以便从更远处点击原点。
        await travel({ x: origin.x + 2, y: origin.y + 2 }, 2, true);
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
