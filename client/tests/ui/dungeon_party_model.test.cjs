const assert = require('node:assert/strict');
const test = require('node:test');
const Long = require('../../node_modules/long');
const { load } = require('../app/helpers/session_harness.cjs');
const { prepareDungeonPartyAction, validateDungeonPartyAction, observeDungeonPartyState } = require('../../../.work/ui-tests/app/DungeonPartyModel.js');

const binary = octet => new Uint8Array(16).fill(octet);
const id = octet => Buffer.from(binary(octet)).toString('hex');
const leader = { roleId: binary(17), displayName: '战士', job: 0, level: 41, online: true };
const guest = { roleId: binary(34), displayName: '法师', job: 1, level: 39, online: true };
const context = { attachment: { connection: 3, binding: '8800000000000000000', role: `1:${id(17)}`,
    scene: '9999999999999999000', baseline: '9999999999999999001', map: '1', travelId: '' }, sceneKind: 1, ready: true, job: 0, level: 41 };
const applicant = { ...context, attachment: { ...context.attachment, role: `1:${id(34)}` }, job: 1, level: 39 };
const template = { templateId: 1, displayName: '隔离讨伐', minimumLevel: 30, durationSeconds: 300 };
const revision = Long.fromString('18446744073709551613', true);
const conditions = { minimumLevel: 31, allowedJobsMask: 6, allowSameJob: false };
const partyId = id(51);
const party = { partyId: binary(51), revision, templateId: 1, ...conditions, leaderRoleId: leader.roleId, members: [leader, guest] };

function wire(ctx, override = {}) {
    return { sceneEpoch: Long.fromString(ctx.attachment.scene.toString(), true), templates: [template], parties: [], ...override };
}

function snapshot(ctx, override = {}) {
    const query = prepareDungeonPartyAction(ctx, { kind: 'query' });
    const parsed = observeDungeonPartyState(wire(ctx, override), query, ctx);
    assert.ok(parsed, '测试名单须与协议匹配');
    return parsed;
}

test('查询、创建和加入使用对应协议字段，保留精准场景与可重复提交的创建编号', () => {
    const directory = snapshot(context);
    const query = prepareDungeonPartyAction(context, { kind: 'query' });
    assert.deepEqual(Object.keys(query.request), ['sceneEpoch', 'action']);
    assert.equal(query.request.sceneEpoch.toString(), '9999999999999999000');
    assert.equal(query.request.action, 1);

    const originalId = binary(51), filters = { ...conditions };
    const create = prepareDungeonPartyAction(context, { kind: 'create', partyId: originalId, templateId: 1, conditions: filters }, directory);
    originalId[0] = 1;
    filters.minimumLevel = 80;
    assert.equal(create.request.action, 2);
    assert.equal(create.request.expectedRevision, undefined);
    assert.equal(create.request.minimumLevel, 31);
    assert.deepEqual(create.request.partyId, party.partyId);
    assert.ok(validateDungeonPartyAction(create, context, snapshot(context, { ownParty: party })));
    assert.equal(observeDungeonPartyState(wire(context, { ownParty: party }), create, context).ownParty.partyId, partyId);

    const first = { ...party, members: [leader] };
    const join = { kind: 'join', partyId };
    const available = snapshot(applicant, { parties: [first] });
    const prepared = prepareDungeonPartyAction(applicant, join, available);
    assert.equal(prepared.request.action, 3);
    assert.equal(prepared.request.expectedRevision.toString(), '18446744073709551613');
    assert.equal(prepared.request.templateId, undefined);
    assert.equal(observeDungeonPartyState(wire(applicant, { ownParty: party }), prepared, applicant).ownParty.members[1].roleId, id(34));
    assert.throws(() => prepareDungeonPartyAction({ ...applicant, level: 30 }, join, available), /等级或职业/);
    assert.throws(() => prepareDungeonPartyAction({ ...applicant, job: 0 }, join, available), /等级或职业/);
    assert.throws(() => prepareDungeonPartyAction(applicant, join, snapshot(applicant, { parties: [party] })), /等级或职业/);
    assert.equal(prepareDungeonPartyAction(applicant, join, snapshot(applicant, { ownParty: party })).request.action, 3);
});

test('退出、踢人、更新和开战保留原队修订，身份或修订变化后撤销旧确认', () => {
    const own = snapshot(context, { ownParty: party });
    const leave = prepareDungeonPartyAction(context, { kind: 'leave', partyId }, own);
    const kick = prepareDungeonPartyAction(context, { kind: 'kick', partyId, targetRoleId: id(34) }, own);
    const update = prepareDungeonPartyAction(context, { kind: 'update', partyId, conditions }, own);
    const start = prepareDungeonPartyAction(context, { kind: 'start', partyId }, own);
    assert.deepEqual([leave, kick, update, start].map(value => value.request.action), [4, 5, 6, 7]);
    assert.deepEqual(kick.request.targetRoleId, guest.roleId);
    assert.deepEqual([update.request.minimumLevel, update.request.allowedJobsMask, update.request.allowSameJob], [31, 6, false]);
    assert.equal(start.request.targetRoleId, undefined);
    assert.ok([leave, kick, update, start].every(value => value.request.expectedRevision.equals(revision)));
    assert.ok(validateDungeonPartyAction(start, context, own));

    for (const fields of [{ connection: 4 }, { binding: '8800000000000000001' }, { scene: '9999999999999999002' }, { baseline: '9999999999999999002' },
        { role: applicant.attachment.role }, { map: '2' }]) {
        assert.equal(validateDungeonPartyAction(start, { ...context, attachment: { ...context.attachment, ...fields } }, own), false);
    }
    const newer = snapshot(context, { ownParty: { ...party, revision: revision.add(1) } });
    assert.equal(validateDungeonPartyAction(start, context, newer), false);
    const guestSnapshot = snapshot(applicant, { ownParty: party });
    for (const action of [{ kind: 'start', partyId }, { kind: 'kick', partyId, targetRoleId: id(17) }, { kind: 'update', partyId, conditions }]) {
        assert.throws(() => prepareDungeonPartyAction(applicant, action, guestSnapshot), /只有队长/);
    }
    assert.throws(() => prepareDungeonPartyAction({ ...context, sceneKind: 2 }, start.action, own), /普通野外/);
    assert.throws(() => prepareDungeonPartyAction(context, start.action, snapshot(context, { ownParty: party, starting: true })), /正在开战/);
});

test('20022只接收原场景和原本角色，错误结果仍可更新原队伍，场次编号只属于成功开战', () => {
    const query = prepareDungeonPartyAction(context, { kind: 'query' });
    const own = snapshot(context, { ownParty: party });
    const start = prepareDungeonPartyAction(context, { kind: 'start', partyId }, own);
    const challengeId = binary(68);
    const confirmed = wire(context, { ownParty: party, challengeId });
    assert.equal(observeDungeonPartyState(confirmed, start, context).challengeId, id(68));
    assert.equal(observeDungeonPartyState(confirmed, query, context), null);
    assert.equal(observeDungeonPartyState({ ...confirmed, starting: true }, start, context), null);
    assert.equal(observeDungeonPartyState({ ...confirmed, sceneEpoch: confirmed.sceneEpoch.add(1) }, start, context), null);
    assert.equal(observeDungeonPartyState(confirmed, start, { ...context, attachment: { ...context.attachment, connection: 4 } }), null);
    assert.equal(observeDungeonPartyState({ ...confirmed, ownParty: { ...party, revision: revision.sub(1) } }, start, context), null);
    assert.equal(observeDungeonPartyState({ ...confirmed, ownParty: { ...party, members: [guest], leaderRoleId: guest.roleId } }, start, context), null);
    assert.equal(observeDungeonPartyState({ ...confirmed, ownParty: { ...party, revision: 9007199254740994 } }, start, context), null);

    const stale = observeDungeonPartyState(wire(context, { errorCode: 4301, ownParty: { ...party, revision: revision.add(1) } }), start, context);
    assert.equal(stale.errorCode, 4301);
    assert.equal(stale.challengeId, '');
    assert.equal(stale.ownParty.revision.toString(), '18446744073709551614');
});

test('生成协议的默认字段与完整 64 位组队消息可直接往返', () => {
    const { encodePacket, decodePacket } = load('client/assets/scripts/app/GameConnection.ts', id =>
        id === 'long' ? Long : id === '../generated/protobuf/client_pb.js' ? require('../../assets/scripts/generated/protobuf/client_pb.js') : {});
    const query = prepareDungeonPartyAction(context, { kind: 'query' });
    const payload = { sceneEpoch: query.request.sceneEpoch, templates: [template], ownParty: { ...party, members: [{ ...leader, job: undefined }] } };
    const binding = Long.fromString(context.attachment.binding, true);
    const message = decodePacket(encodePacket(20022, 1, binding, payload));
    const parsed = observeDungeonPartyState(message.value, query, context);
    assert.ok(parsed);
    assert.equal(parsed.errorCode, 0);
    assert.equal(parsed.ownParty.members[0].job, 0);
    assert.ok(parsed.ownParty.leaderOfflineUntilUnixMs.isZero());
    const start = prepareDungeonPartyAction(context, { kind: 'start', partyId }, parsed);
    const decoded = decodePacket(encodePacket(20021, 2, binding, start.request)).value;
    assert.equal(decoded.sceneEpoch.toString(), '9999999999999999000');
    assert.equal(decoded.expectedRevision.toString(), revision.toString());
});
