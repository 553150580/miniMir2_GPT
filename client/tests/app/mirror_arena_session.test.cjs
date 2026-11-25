const assert = require('node:assert/strict');
const { test } = require('node:test');
const { setTimeout: wait } = require('node:timers/promises');
const { Long, load } = require('./helpers/session_harness.cjs');
const model = require('../../../.work/ui-tests/app/MirrorArenaModel.js');
const travel = require('../../../.work/ui-tests/app/MapTravelModel.js');
const pb = require('../../assets/scripts/generated/protobuf/client_pb.js');
const ids = load('client/assets/scripts/generated/protocol_ids.ts', () => ({}));
const wire = load('client/assets/scripts/app/GameConnection.ts', id => ({
    long: Long, '../generated/protobuf/client_pb.js': pb, './MirrorArenaModel': model,
    './InputTrace': { inputTraceMessage: () => null, traceInput() {} },
}[id] ?? {}));
const { MirrorArenaSession } = load('client/assets/scripts/app/MirrorArenaSession.ts', id => ({
    long: Long, './GameConnection': wire, './MirrorArenaModel': model, './MapTravelModel': travel,
    './GameErrors': { protocolErrorMessage: (_, code) => `反映错误${code}` }, '../generated/protocol_ids': ids,
}[id] ?? {}));

const long = value => Long.fromString(String(value), true);
const bytes = value => new Uint8Array(16).fill(value);
const scene = long('9007199254740997'), battleEpoch = long('9007199254740999');
const rule = long(30), battleMap = long(61), firstSequence = long('9007199254741002'), secondSequence = firstSequence.add(1);
const seasonStart = long('1900000000000'), deadline = seasonStart.add(3600000);
const offerId = bytes(4), challengeId = bytes(7);
const roster = [{ offerId, defenderName: '道士', defenderJob: 2, defenderLevel: 31, defenseVersion: long(3) }];

function record(overrides = {}) {
    return { challengeId, attackerName: '法师', defenderName: '道士', defenseVersion: long(3),
        startedAtUnixMs: seasonStart.add(100), ...overrides };
}
function settled() {
    return record({ outcome: 1, awardedPoints: long(5), finishedAtUnixMs: seasonStart.add(1000) });
}
function lobby(overrides = {}) {
    return { sceneEpoch: scene, ruleVersion: rule, open: true, seasonStartUnixMs: seasonStart,
        seasonEndUnixMs: deadline, dailyLimit: 5, entriesUsed: 1, offers: roster, ...overrides };
}
function battle(phase, overrides = {}) {
    return lobby({ dailyLimit: 0, entriesUsed: 0, offers: [], challenge: phase === 4 ? settled() : record(),
        battleState: phase, battleSceneEpoch: battleEpoch, battleMapId: battleMap, battleDeadlineUnixMs: deadline, ...overrides });
}
function body(handle, overrides = {}) {
    return { entityHandle: handle, entityKind: handle < 3 ? 1 : 2, definitionId: handle === 1 ? 1 : 2,
        position: { x: handle < 3 ? handle : 3, y: 2 }, direction: 0, hp: long(9), hpMax: long(10),
        mp: long(0), mpMax: long(0), life: long(1), ...overrides };
}
function frame(overrides = {}) {
    return { sceneEpoch: scene, challengeId, battleSceneEpoch: battleEpoch, sequence: firstSequence,
        serverUnixMs: seasonStart.add(600), entities: [body(1), body(2), body(102)], observations: [], ...overrides };
}
function observation(messageId, codec, input) {
    return { messageId, payload: pb.minimir.v1[codec].encode(input).finish() };
}

// 请求观察器保留真实 protobuf 编解码顺序，模拟 GameConnection 在同一帧内同步确认回复。
function fixture() {
    const listeners = new Set(), statuses = new Set(), sent = [];
    let connectionNumber = 1, sceneKind = 1, identityRole = 'origin:role', activeScene = scene;
    const connection = {
        generation: connectionNumber, binding: long(connectionNumber),
        request(id, request, apply) {
            const seq = sent.length + 1, binding = this.binding, payload = wire.decodePacket(wire.encodePacket(id, seq, binding, request)).value;
            return new Promise((resolve, reject) => sent.push({ payload, fail: reject, reply: data => {
                const packet = wire.decodePacket(wire.encodePacket(20026, seq, binding, data));
                const accepted = apply(packet.value);
                for (const listener of listeners) listener(packet);
                resolve(accepted);
            } }));
        },
        subscribe(listener) { listeners.add(listener); return () => listeners.delete(listener); },
        onStatus(listener) { statuses.add(listener); return () => statuses.delete(listener); },
    };
    const session = new MirrorArenaSession(connection, () => {
        if (sceneKind !== 1 && sceneKind !== 2) return null;
        return { ruleVersion: rule, identity: { connection: connection.generation, binding: connection.binding,
            role: identityRole, scene: activeScene, baseline: long(2), map: long(1), travelId: '' } };
    }, () => challengeId);
    return {
        session, sent,
        push(id, data, binding = connection.binding, seq = 0) {
            const packet = wire.decodePacket(wire.encodePacket(id, seq, binding, data));
            for (const listener of listeners) listener(packet);
        },
        detach() {
            sceneKind = 0;
            connectionNumber++;
            connection.generation = connectionNumber;
            connection.binding = long(connectionNumber);
            for (const status of statuses) status();
        },
        attach(kind, role = identityRole, nextScene = scene) {
            sceneKind = kind;
            identityRole = role;
            activeScene = nextScene;
            session.syncAttachment();
        },
        async next(index) {
            for (let retry = 0; retry < 150 && !sent[index]; retry++) await wait(10);
            assert(sent[index], `缺少镜像请求 ${index}`);
            return sent[index];
        },
        cleanup: () => session.dispose(),
    };
}

async function openLobby(f) {
    f.session.setEnabled(true);
    const query = f.session.query();
    f.sent[0].reply(lobby());
    await query;
}

test('默认关闭、当前普通附着绑定、服务端关闭与发布保留原版本', async t => {
    const f = fixture(); t.after(f.cleanup);
    await assert.rejects(f.session.query(), /尚未/);
    f.push(20026, lobby());
    assert.equal(f.session.snapshot.lobby, null);
    f.attach(3);
    f.session.setEnabled(true);
    await assert.rejects(f.session.query(), /尚未/);
    assert.equal(f.sent.length, 0);
    f.attach(2);
    let query = f.session.query();
    f.sent[0].reply({ sceneEpoch: scene, ruleVersion: rule });
    await query;
    assert.equal(f.session.snapshot.lobby.open, false);
    query = f.session.query();
    f.sent[1].reply(lobby());
    await query;
    const published = f.session.publish();
    assert.equal(f.sent[2].payload.action, 2);
    f.sent[2].reply(lobby({ publishedDefenseVersion: long('9007199254741090') }));
    await published;
    query = f.session.query();
    f.sent[3].reply(lobby());
    await query;
    assert.equal(f.session.snapshot.lobby.publishedDefenseVersion, '9007199254741090');
});

test('保存原名单，确认首帧后才接收独立表现，不重播旧包或回退终态', async t => {
    const f = fixture(); t.after(f.cleanup);
    await openLobby(f);
    await assert.rejects(f.session.challenge(bytes(3)), /候选已变化/);
    const started = f.session.challenge(offerId);
    assert.deepEqual(f.sent[1].payload.offerId, offerId);
    assert.deepEqual(f.sent[1].payload.challengeId, challengeId);
    assert(f.sent[1].payload.sceneEpoch.equals(scene));
    f.push(20026, battle(1, { challenge: record({ challengeId: bytes(8) }) }));
    assert.equal(f.session.snapshot.battle, null);

    // 模拟已缓存地图的界面在同步回执中立即确认加载，不能被上一请求的忙态拦截。
    let ready;
    const remove = f.session.subscribe(snapshot => {
        if (snapshot.battle?.phase === 1 && !snapshot.pending && !ready) {
            remove();
            ready = f.session.ready(snapshot.battle.key);
        }
    });
    f.sent[1].reply(battle(1));
    await started;
    const preparing = f.session.snapshot.battle, readyWire = f.sent[2];
    assert.equal(f.session.snapshot.lobby.dailyLimit, 5);
    assert.equal(f.session.snapshot.lobby.offers[0].defenderName, '道士');
    assert.equal(readyWire.payload.action, 4);
    assert(readyWire.payload.battleSceneEpoch.equals(battleEpoch));
    f.push(20027, frame());
    assert.equal(f.session.snapshot.frame, null);
    readyWire.reply(battle(2));

    const cast = observation(30006, 'CastAccepted', { scene_epoch: battleEpoch, cast_id: long('9007199254741111'),
        caster_handle: 2, target_handle: 1, skill_id: 1, origin: { x: 2, y: 2 }, target_grid: { x: 1, y: 2 }, direction: 7 });
    const wrong = observation(30008, 'CombatImpact', { scene_epoch: scene, event_id: long(8), caster_handle: 2, target_handle: 1 });
    f.push(20027, frame({ observations: [cast, wrong, { messageId: 40005, payload: new Uint8Array([8, 1]) }] }));
    await ready;
    assert.equal(f.session.snapshot.frame.battleKey, preparing.key);
    assert.equal(f.session.snapshot.frame.sequence, firstSequence.toString());
    assert.equal(f.session.snapshot.frame.entities[0].hpMax.toString(), '10');
    const [event] = f.session.snapshot.frame.observations;
    assert.equal(f.session.snapshot.frame.observations.length, 1);
    assert.equal(event.messageId, 30006);
    assert(event.value.castId.equals(long('9007199254741111')));
    const previous = f.session.snapshot.frame;
    for (const bad of [frame({ sequence: firstSequence }), frame({ sequence: secondSequence, challengeId: bytes(6) }),
        frame({ sequence: secondSequence, battleSceneEpoch: long(9) }), frame({ sequence: secondSequence, sceneEpoch: long(8) }),
        frame({ sequence: secondSequence, entities: [body(1), body(1)] })]) f.push(20027, bad);
    f.push(20027, frame({ sequence: secondSequence }), long(2));
    f.push(20027, frame({ sequence: secondSequence }), undefined, 9);
    assert.equal(f.session.snapshot.frame, previous);

    const surrender = f.session.surrender();
    assert.equal(f.sent[3].payload.action, 5);
    f.sent[3].reply(battle(3));
    await surrender;
    f.push(20026, battle(2, { seasonPoints: long(1) }));
    assert.equal(f.session.snapshot.battle.phase, 3);
    f.push(20027, frame({ sequence: secondSequence }));
    assert.equal(f.session.snapshot.frame.sequence, secondSequence.toString());
    f.push(20026, battle(4, { seasonPoints: long(5) }));
    f.push(20027, frame({ sequence: secondSequence.add(1) }));
    assert.equal(f.session.snapshot.frame.sequence, secondSequence.toString());
    assert.equal(f.session.snapshot.battle.challenge.awardedPoints, '5');
    f.session.dismiss();
    f.push(20026, battle(4));
    assert.equal(f.session.snapshot.battle, null);
    assert.equal(f.session.snapshot.frame, null);
    const history = f.session.query();
    f.sent[4].reply(lobby({ attackHistory: [settled()] }));
    await history;
    assert.equal(f.session.snapshot.lobby.attackHistory[0].outcome, 1);
});

test('新票只接回同一挑战的原场与更高整帧水位，并有界重查暂忙目录', async t => {
    const f = fixture(); t.after(f.cleanup);
    await openLobby(f);
    const start = f.session.challenge(offerId);
    f.sent[1].reply(battle(1));
    await start;
    const old = f.session.snapshot.battle;
    const ready = f.session.ready(old.key);
    f.sent[2].reply(battle(2));
    await ready;
    f.push(20027, frame());
    f.detach();
    assert.equal(f.session.snapshot.attached, false);
    assert.equal(f.session.snapshot.frame, null);
    f.attach(4);
    await assert.rejects(f.session.observe(), /尚未/);
    f.attach(1);
    await assert.rejects(f.session.ready(old.key), /尚未/);
    const recovering = await f.next(3);
    assert.equal(recovering.payload.action, 1);
    recovering.reply({ sceneEpoch: scene, ruleVersion: rule, errorCode: ids.CodeLineRecovering });
    const retry = await f.next(4);
    retry.reply(lobby({ challenge: record() }));
    assert.equal(f.session.snapshot.battle, null);
    f.push(20026, battle(2), long(1));
    f.push(20026, battle(2, { battleSceneEpoch: battleEpoch.add(1) }));
    assert.equal(f.session.snapshot.battle, null);
    f.push(20026, battle(2));
    const recovered = f.session.snapshot.battle;
    assert(recovered && recovered.key !== old.key);
    assert.equal(recovered.epoch, old.epoch);
    f.push(20027, frame({ sequence: secondSequence }), long(1));
    f.push(20027, frame());
    assert.equal(f.session.snapshot.frame, null);
    const observing = f.session.observe();
    assert.equal(f.sent[5].payload.action, 6);
    f.sent[5].reply(battle(2));
    f.push(20027, frame({ sequence: secondSequence }));
    await observing;
    assert.equal(f.session.snapshot.frame.battleKey, recovered.key);
    assert.equal(f.session.snapshot.frame.sequence, secondSequence.toString());
    f.detach();
    f.attach(1, 'origin:other-role');
    f.push(20026, battle(2));
    assert.equal(f.session.snapshot.lobby, null);
    assert.equal(f.session.snapshot.battle, null);
});

test('预约响应未知时只重投原候选与挑战号，过期附着回执不修改新角色', async t => {
    const f = fixture(); t.after(f.cleanup);
    await openLobby(f);
    const start = f.session.challenge(offerId);
    f.sent[1].fail(new Error('原预约回执未知'));
    await assert.rejects(start, /原预约回执未知/);
    const retry = f.session.challenge(offerId);
    assert.deepEqual(f.sent[1].payload, f.sent[2].payload);
    f.detach();
    f.attach(1, 'origin:other-role');
    f.sent[2].reply(battle(1));
    await assert.rejects(retry, /附着已变化/);
    assert.equal(f.session.snapshot.battle, null);
    assert.equal(f.session.snapshot.lobby, null);
});

test('换票时原场推送早于目录回执，已缓存的新视口能等待同一查询再确认首帧', async t => {
    const f = fixture(); t.after(f.cleanup);
    await openLobby(f);
    const start = f.session.challenge(offerId);
    f.sent[1].reply(battle(1));
    await start;
    f.detach();
    f.attach(1);
    const read = await f.next(2);
    f.push(20026, battle(1));
    const prep = f.session.snapshot.battle;
    assert(prep);
    const ready = f.session.ready(prep.key);
    assert.equal(f.sent.length, 3);
    read.reply(battle(1));
    const confirmation = await f.next(3);
    assert.equal(confirmation.payload.action, 4);
    assert.equal(confirmation.payload.battleSceneEpoch.toString(), prep.epoch);
    confirmation.reply(battle(2));
    f.push(20027, frame());
    await ready;
    assert.equal(f.session.snapshot.frame.battleKey, prep.key);
});

test('镜像火墙只允许合法独立格；重复或越界整帧丢弃，旧数据无火墙字段时显示为空', () => {
    const current = { key: 'only', challenge: { id: model.mirrorID(challengeId) }, epoch: battleEpoch.toString(), phase: 2 };
    const parse = value => model.readMirrorFrame(value, scene.toString(), current, '0', wire.decodeMirrorCombatPayload);
    assert.deepEqual(parse(frame()).fireWallCells, []);
    assert.deepEqual(parse(frame({ fireWallCells: [{ x: 0, y: 16 }, { x: 8, y: 2 }] })).fireWallCells, [{ x: 0, y: 16 }, { x: 8, y: 2 }]);
    for (const invalid of [[{ x: 1, y: 0 }, { x: 1, y: 0 }], [{ x: 17, y: 0 }], [{ x: 0, y: .5 }], new Array(290).fill({ x: 0, y: 0 })]) {
        assert.equal(parse(frame({ fireWallCells: invalid })), null);
    }
});
