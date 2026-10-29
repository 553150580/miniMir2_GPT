const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const root = process.cwd();
const {Long, load, entry} = require(path.join(root, 'client/tests/app/helpers/session_harness.cjs'));
const modelCache = new Map();
function model(name) {
    if (modelCache.has(name)) return modelCache.get(name);
    const value = load(`client/assets/scripts/app/${name}.ts`, id => id === 'long' ? Long : id.startsWith('./') ? model(id.slice(2)) : {});
    modelCache.set(name, value); return value;
}
class Connection {
    constructor() { this.generation = 1; this.binding = Long.UONE; }
    subscribe(listener) { this.listener = listener; }
    onStatus(listener) { this.status = listener; }
    deliver(id, value) { this.listener({id, value, binding: this.binding, error: 0}); }
}
function sessionFixture() {
    const h = load('client/assets/scripts/app/GameSession.ts', id => id === 'long' ? Long : id === './GameConnection' ? {GameConnection: Connection}
        : id === './GameErrors' ? {protocolErrorMessage: () => ''}
        : ['./BaselineAssembler', './TeleportModel', './RoleBindingModel', './MapTravelModel', './SkillModel', './PetBornModel'].includes(id) ? model(id.slice(2)) : {});
    h.session.role = {key: {originServerId: Long.UONE, roleId: new Uint8Array(16).fill(9)}, job: 2};
    h.session.presentationNow = () => 100;
    const e = entry('1', '1'); e.resourceVersion = Long.fromInt(18, true);
    h.session.connection.deliver(20002, e);
    h.session.connection.deliver(6, {...e, nextEconomicCommandSeq: Long.UONE});
    const body = {entityHandle: 3, entityKind: 2, definitionId: 1, life: Long.UONE,
        hp: Long.fromInt(10), hpMax: Long.fromInt(10), movementRevision: Long.UZERO, direction: 5, position: {x: 8, y: 8}};
    const baseline = {sceneEpoch: Long.UONE, baselineId: Long.UONE, stateSequence: Long.fromInt(2, true), chunkIndex: 0, chunkCount: 1, entities: [body]};
    const born = {sceneEpoch: Long.UONE, baselineId: Long.UONE, stateSequence: baseline.stateSequence, entityHandle: 3, entityLife: Long.UONE,
        definitionId: 1, mapId: Long.UONE, sourceCastId: Long.UONE, sourceSkillId: 17, bornAtUnixMs: Long.UONE,
        movementRevision: Long.UZERO, direction: 5, role: h.session.role.key, elapsedMs: 0};
    return {h, e, baseline, born};
}
const ordinary = sessionFixture();
ordinary.h.session.connection.deliver(5, ordinary.baseline);
ordinary.h.session.connection.deliver(30015, ordinary.born);
assert.equal(ordinary.h.session.petBorn.pendingCount, 1);
assert(ordinary.h.session.petBorn.take(ordinary.h.currentEntryIdentity(), ordinary.h.session.baseline, 1000, 100));

const q = sessionFixture(), h = q.h, identity = h.currentEntryIdentity(), id = 'ab'.repeat(16), now = Date.now();
const context = {...identity, life: Long.UZERO, rule: q.e.resourceVersion};
h.session.travel.begin(context, id, 1);
h.session.travel.acceptPlan({travelId: id, fromSceneEpoch: Long.UONE, targetMapId: Long.fromInt(2, true), targetSceneKind: 2,
    position: {x: 633, y: 613}, expiresAtUnixMs: String(now + 60000)}, context, now);
h.session.travel.prepared(context, now); h.session.travel.commit(context, now);
h.session.connection.deliver(20010, {travelId: id, fromSceneEpoch: Long.UONE, sceneEpoch: Long.UONE, phase: 1, targetMapId: Long.fromInt(2, true)});
assert.equal(h.session.travel.phase, 'quiescing'); assert.equal(h.session.ready, false);
let oldCastEvents = 0; h.session.casts.add(() => oldCastEvents++);
h.session.connection.deliver(30006, {sceneEpoch: Long.UONE});
h.session.connection.deliver(5, q.baseline);
assert.equal(h.session.baseline.entities[0].life.toString(), '1');
h.session.connection.deliver(30015, q.born);
const pendingDuring = h.session.petBorn.pendingCount;
const playbackDuring = h.session.petBorn.take(h.currentEntryIdentity(), h.session.baseline, 1000, 100);
assert(playbackDuring);
// 仍是原attachment；旅行取消恢复ready，而不会另发出生事实。
h.session.connection.deliver(20010, {travelId: id, fromSceneEpoch: Long.UONE, sceneEpoch: Long.UONE, phase: 3, targetMapId: Long.fromInt(2, true)});
const playbackAfterCancel = h.session.petBorn.take(h.currentEntryIdentity(), h.session.baseline, 1000, 100);
assert.equal(h.session.ready, true); assert.equal(oldCastEvents, 1);
assert.equal(pendingDuring, 1); assert.equal(playbackAfterCancel, null);
const files = ['GameSession.ts', 'MapTravelModel.ts', 'PetBornModel.ts', 'AppRoot.ts'];
const report = {kind: '只读轻量真实Session/Travel/PetBorn/Assembler重放，连接是内存替身，未运行网络/GPU/音频',
    control_ready_birth_accepted: true, quiescing_old_cast_events: oldCastEvents,
    quiescing_full_baseline_live_birth: true, quiescing_pending_births: pendingDuring,
    quiescing_playback_consumed: !!playbackDuring,
    cancelled_same_attachment_ready: h.session.ready, cancelled_same_attachment_birth_playback: playbackAfterCancel,
    finding: '原漏出生已修：quiescing仍可消费同attachment完整baseline真实出生；取消不重播',
    source_sha256: Object.fromEntries(files.map(file => [`client/assets/scripts/app/${file}`, crypto.createHash('sha256').update(fs.readFileSync(path.join(root, 'client/assets/scripts/app', file))).digest('hex')]))};
console.log(JSON.stringify(report, null, 2));
