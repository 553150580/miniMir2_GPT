// 用两个隔离账号和实际 Creator 资源核对镜像双方出场与健康重连。
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require(path.join(process.env.MINIMIR_TOOLCHAIN_ROOT, 'browser-tools/node_modules/playwright-core'));
const pb = require('../../client/assets/scripts/generated/protobuf/client_pb.js').minimir.v1;
const { expected, output, endpoint, safe, lobby, confirm, wait, touch, label, login, openLobby, originalWorld, packet } =
    require('./verify_mirror_arena_browser.cjs');

const battle = safe + 'MirrorArenaBattle', reconnect = 'Canvas/ReconnectDialog/Panel/Reconnect';
const petDefinitions = [expected.skeletonDefinition, expected.shinsuDefinition];
assert(petDefinitions.every(id => Number.isInteger(id) && id > 0) && petDefinitions[0] !== petDefinitions[1]);

// 解码原生 WebSocket 的权威完整帧，同时保留关联出生原始身份。
function observe(page, errors) {
    const wire = { sockets: 0, requests: [], states: [], frames: [], births: [] };
    page.on('pageerror', error => errors.push(error.message));
    page.on('websocket', socket => {
        if (socket.url() !== endpoint.href) { errors.push('补召验收连接未走隔离游戏节点'); return; }
        wire.sockets++;
        socket.on('framesent', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 20025) wire.requests.push(pb.MirrorArenaRequest.decode(frame.body));
        });
        socket.on('framereceived', ({ payload }) => {
            const frame = packet(payload);
            if (frame?.id === 20026) wire.states.push(pb.MirrorArenaState.decode(frame.body));
            if (frame?.id !== 20027) return;
            const state = pb.MirrorArenaCombatFrame.decode(frame.body);
            const scope = { binding: frame.binding, epoch: state.battle_scene_epoch.toString(), sequence: state.sequence.toString(),
                challenge: Buffer.from(state.challenge_id).toString('hex'), serverTime: state.server_unix_ms.toString() };
            const pets = state.entities.filter(entity => entity.entity_kind === 2).map(pet => ({
                handle: pet.entity_handle, life: pet.life.toString(), definition: pet.definition_id, dead: pet.hp.isZero(),
                corpse: pet.is_pet_corpse, revision: pet.movement_revision.toString(), direction: pet.direction,
            }));
            wire.frames.push({ ...scope, pets });
            for (const observation of state.observations) {
                if (observation.message_id !== 30015) continue;
                const value = pb.PetBorn.decode(observation.payload);
                wire.births.push({ ...scope, value, body: pets.find(pet => pet.handle === value.entity_handle) });
            }
        });
    });
    return wire;
}

// 只记录实际出场函数返回后的原始帧和音效请求，仍由同一渲染器按原处理流程播放。
async function observePresentation(page) {
    await page.evaluate(async () => {
        const { GameWorldView } = await System.import('chunks:///_virtual/GameWorldView.ts');
        const present = GameWorldView.prototype.presentPetBorn;
        window.mirrorBirthPresentation = [];
        GameWorldView.prototype.presentPetBorn = function (playback) {
            if (this.assets?.sceneKind !== 6) return present.call(this, playback);
            const event = playback.event, audio = this.audio, play = audio?.play;
            const cues = [];
            if (audio && play) audio.play = function (index, cue, ...args) {
                cues.push({ index, cue });
                return play.call(this, index, cue, ...args);
            };
            try {
                const accepted = present.call(this, playback), visual = this.visuals.get(event.entityHandle);
                const action = visual?.profile?.layers[0].actions.appear;
                const key = visual?.keys[0], direction = this.projection?.visualDirection(event.direction);
                const originalFrame = action?.directions[direction]?.frames.includes(key);
                window.mirrorBirthPresentation.push({ handle: event.entityHandle, life: event.entityLife, sequence: event.stateSequence,
                    definition: event.definitionId, accepted, action: visual?.pose?.action, key, originalFrame,
                    sprite: !!visual?.body[0]?.spriteFrame, cues, expectedCue: action?.sound ?? null });
                return accepted;
            } finally { if (audio && play) audio.play = play; }
        };
    });
}

function verifyBorn(wire, rendered, challenge, epoch, map, initialBinding) {
    const births = wire.births.filter(row => row.challenge === challenge && row.binding === initialBinding);
    assert.equal(births.length, 2, '存活原宠的两侧各只出场一次');
    return petDefinitions.map((definition, side) => {
        const row = births.find(birth => birth.value.definition_id === definition);
        assert(row && row.epoch === epoch && row.body && !row.body.dead && !row.body.corpse);
        const { value, body } = row;
        assert.deepEqual([value.entity_handle, value.source_skill_id, value.definition_id], [103 + side, side === 0 ? 17 : 30, definition]);
        assert.deepEqual([value.scene_epoch.toString(), value.baseline_id.toString(), value.state_sequence.toString(), value.map_id.toString()],
            [epoch, epoch, row.sequence, map]);
        assert.equal(value.entity_life.toString(), body.life);
        assert.equal(value.movement_revision.toString(), body.revision);
        assert.equal(value.direction, body.direction);
        assert(!value.role && !value.source_cast_id.isZero() && BigInt(value.born_at_unix_ms.toString()) <= BigInt(row.serverTime));
        const view = rendered.find(pet => pet.handle === body.handle && pet.life === body.life && pet.sequence === row.sequence);
        assert(view?.accepted && view.sprite && view.originalFrame);
        assert.equal(view.action, 'appear');
        if (view.expectedCue) assert(view.cues.some(sound => sound.index === 1 && sound.cue === view.expectedCue));
        return { handle: body.handle, life: body.life, definition, skill: value.source_skill_id, originalFrame: view.key, sound: view.expectedCue };
    });
}

async function main() {
    const report = { passed: false, errors: [] };
    const browser = await chromium.launch({ channel: 'chrome', headless: true,
        args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] });
    const options = { viewport: { width: 720, height: 1280 }, hasTouch: true, isMobile: true, deviceScaleFactor: 1, serviceWorkers: 'block' };
    const guardContext = await browser.newContext(options), attackContext = await browser.newContext(options);
    const defender = await guardContext.newPage(), attacker = await attackContext.newPage();
    const guardWire = observe(defender, report.errors), wire = observe(attacker, report.errors);
    let guardSession, session;
    try {
        report.step = '道士从普通地图发布防守并离线';
        guardSession = await login(defender, 'defender');
        assert.equal((await originalWorld(defender, guardSession)).role, expected.defender);
        await openLobby(defender);
        await touch(defender, lobby + '/Publish');
        await touch(defender, confirm);
        await label(defender, lobby + '/Defense', '防守快照 v1');
        assert(guardWire.requests.some(request => request.action === 2));
        await defender.evaluate(session => session.connection.socket.close(4001, '防守发布后离线'), guardSession);
        await wait(defender, session => !session.ready, guardSession, 6000);
        await guardSession.dispose();
        guardSession = undefined;
        await guardContext.close();

        report.step = '进攻道士选择原对手并确认独立场次';
        session = await login(attacker, 'attacker');
        const original = await originalWorld(attacker, session);
        assert.equal(original.role, expected.attacker);
        assert.equal(original.map, expected.normalMap);
        assert.equal(original.rule, expected.ruleVersion);
        await observePresentation(attacker);
        await openLobby(attacker);
        const offer = wire.states.flatMap(state => state.offers).find(candidate => candidate.defender_name === expected.defender);
        assert(offer && offer.defender_job === 2);
        await touch(attacker, lobby + '/List/View/Content', expected.defender);
        await touch(attacker, lobby + '/Challenge');
        await touch(attacker, confirm);
        await label(attacker, battle + '/Header/Status', '双方自动战斗', 42000);
        const prepared = wire.states.find(state => state.battle_state === 1 && state.challenge);
        assert(prepared);
        const map = prepared.battle_map_id.toString(), epoch = prepared.battle_scene_epoch.toString();
        assert.equal(map, String(expected.battleMap ?? expected.map));
        const challenge = Buffer.from(prepared.challenge.challenge_id).toString('hex');
        assert(wire.requests.some(request => request.action === 4 && request.battle_scene_epoch.toString() === epoch));

        report.step = '验证两侧空席先后真实出生与原资源播放';
        const rendered = await wait(attacker, () => window.mirrorBirthPresentation.length >= 2 ? window.mirrorBirthPresentation : null, undefined, 25000);
        const first = wire.frames.find(frame => frame.challenge === challenge);
        assert(first && first.pets.length === 0, '首个完整画面确认时两侧均没有继承宠物');
        report.births = verifyBorn(wire, rendered, challenge, epoch, map, original.binding);
        assert.equal(rendered.length, 2);
        await attacker.screenshot({ path: path.join(output, 'mirror-summon.png') });

        report.step = '健康换票后接回同场身体且不补播已消费事件';
        const prior = wire.frames.at(-1);
        await attacker.evaluate(session => session.connection.socket.close(4001, '隔离补召健康换票'), session);
        await touch(attacker, reconnect);
        const recovered = await wait(attacker, ({ session, epoch, binding, sequence, pets }) => {
            const app = cc.director.getScene()?.getComponentsInChildren('AppRoot')[0];
            const active = app?.mirrorArena?.presentation?.active, view = active?.view;
            const current = session.connection.binding.toString();
            if (!session.ready || BigInt(current) <= BigInt(binding) || !view?.world?.activeInHierarchy ||
                view.sceneEpoch.toString() !== epoch || active.sequence <= BigInt(sequence)) return null;
            for (const pet of pets) {
                const body = view.visuals.get(pet.handle);
                if (!body?.body[0]?.spriteFrame || body.state.life.toString() !== pet.life || body.state.definitionId !== pet.definition ||
                    body.state.isPetCorpse || body.state.hp.isZero() || body.pose?.action === 'appear') return null;
            }
            return { binding: current, sequence: active.sequence.toString(), births: window.mirrorBirthPresentation.length };
        }, { session, epoch, binding: original.binding, sequence: prior.sequence, pets: report.births }, 26000);
        assert.equal(recovered.births, rendered.length);
        assert(wire.frames.some(frame => frame.binding === recovered.binding && frame.challenge === challenge && frame.sequence === recovered.sequence));
        assert(!wire.births.some(row => row.binding === recovered.binding));
        assert.equal(wire.sockets, 2);
        const ordinary = await originalWorld(attacker, session);
        assert.equal(ordinary.role, original.role);
        assert.equal(ordinary.map, original.map);
        report.reconnect = { sameChallenge: true, sameBodies: true, noReplay: true, newBinding: true, sequence: recovered.sequence };
        await attacker.screenshot({ path: path.join(output, 'mirror-summon-reconnected.png') });

        report.step = '触屏认输后回原普通地图，确认只有原挑战结算';
        await touch(attacker, battle + '/Footer/Surrender');
        await touch(attacker, confirm);
        await label(attacker, battle + '/Header/Status', '失败 · 积分 +1');
        const ended = wire.states.find(state => state.battle_state === 4 && state.challenge &&
            Buffer.from(state.challenge.challenge_id).toString('hex') === challenge);
        assert(ended && ended.challenge.outcome === 2 && ended.challenge.awarded_points.toString() === '1');
        report.terminal = { challenge: Buffer.from(ended.challenge.challenge_id).toString('hex'), outcome: ended.challenge.outcome,
            points: ended.challenge.awarded_points.toString() };
        assert.equal(wire.requests.filter(request => request.action === 5).length, 1);
        await touch(attacker, battle + '/Footer/Return');
        await wait(attacker, path => !cc.find(path)?.activeInHierarchy, battle);
        const returned = await originalWorld(attacker, session);
        assert(returned.ready && returned.role === original.role && returned.map === original.map);
        report.surrender = { once: true, points: '1', ordinaryMap: returned.map };
        assert.deepEqual(report.errors, []);
        report.passed = true;
        delete report.step;
    } catch (error) {
        report.error = error.message;
        report.observed = { frames: wire.frames.length, births: wire.births.map(row => ({ body: row.body, sequence: row.sequence })),
            rendered: await attacker.evaluate(() => window.mirrorBirthPresentation ?? []).catch(() => []), sockets: wire.sockets };
        await attacker.screenshot({ path: path.join(output, 'mirror-summon-failed.png') }).catch(() => {});
        throw error;
    } finally {
        await guardSession?.dispose();
        await session?.dispose();
        await browser.close();
        fs.writeFileSync(path.join(output, 'mirror-summoning-browser-report.json'), JSON.stringify(report, null, 2) + '\n', 'utf8');
    }
    console.log('镜像两侧从空席出生，使用原宠物帧与音效，新票保留同一可见身体');
}

main().catch(error => { console.error(error.stack); process.exitCode = 1; });
