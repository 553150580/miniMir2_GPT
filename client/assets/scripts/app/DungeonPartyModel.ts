import Long from 'long';
import { attachmentKey, travelInteger } from './MapTravelModel';
import type { AttachmentIdentity, TravelInteger } from './MapTravelModel';

// 组队请求的所有身份来自已确认场景、角色以及该场景收到的原队伍视图。
export interface DungeonPartyContext {
    readonly attachment: AttachmentIdentity;
    readonly sceneKind: number;
    readonly ready: boolean;
    readonly job: number;
    readonly level: number;
}

export interface DungeonPartyConditions {
    readonly minimumLevel: number;
    readonly allowedJobsMask: number;
    readonly allowSameJob: boolean;
}

export interface DungeonPartyTemplate {
    readonly templateId: number;
    readonly displayName: string;
    readonly minimumLevel: number;
    readonly durationSeconds: number;
}

export interface DungeonPartyMember {
    readonly roleId: string;
    readonly displayName: string;
    readonly job: number;
    readonly level: number;
    readonly online: boolean;
}

export interface DungeonPartyView extends DungeonPartyConditions {
    readonly partyId: string;
    readonly revision: Long;
    readonly templateId: number;
    readonly leaderRoleId: string;
    readonly members: readonly DungeonPartyMember[];
    readonly leaderOfflineUntilUnixMs: Long;
}

export interface DungeonPartySnapshot {
    readonly attachmentKey: string;
    readonly templates: readonly DungeonPartyTemplate[];
    readonly parties: readonly DungeonPartyView[];
    readonly ownParty: DungeonPartyView | null;
    readonly more: boolean;
    readonly starting: boolean;
    readonly challengeId: string;
    readonly errorCode: number;
}

export type DungeonPartyAction = { readonly kind: 'query' }
    | { readonly kind: 'create'; readonly partyId: Uint8Array; readonly templateId: number; readonly conditions: DungeonPartyConditions }
    | { readonly kind: 'join'; readonly partyId: string }
    | { readonly kind: 'leave'; readonly partyId: string }
    | { readonly kind: 'start'; readonly partyId: string }
    | { readonly kind: 'kick'; readonly partyId: string; readonly targetRoleId: string }
    | { readonly kind: 'update'; readonly partyId: string; readonly conditions: DungeonPartyConditions };

interface DungeonPartyRequest {
    readonly sceneEpoch: Long;
    readonly action: number;
    readonly partyId?: Uint8Array;
    readonly expectedRevision?: Long;
    readonly templateId?: number;
    readonly minimumLevel?: number;
    readonly allowedJobsMask?: number;
    readonly allowSameJob?: boolean;
    readonly targetRoleId?: Uint8Array;
}

export interface PreparedDungeonPartyAction {
    readonly request: DungeonPartyRequest;
    readonly attachment: AttachmentIdentity;
    readonly action: DungeonPartyAction;
}

const actions = { query: 1, create: 2, join: 3, leave: 4, kick: 5, update: 6, start: 7 } as const;
const maxPartyMembers = 3; // 招募协议支持的真人席位数。
const publicPartyLimit = 20; // 服务端每次返回的公开招募上限。
const identifierPattern = /^[a-f0-9]{32}$/;
type WireRecord = Record<string, unknown>;

function record(value: unknown): WireRecord {
    if (typeof value !== 'object' || value === null || Array.isArray(value)) throw new Error('组队响应格式无效');
    return value as WireRecord;
}

function list(value: unknown): readonly unknown[] {
    if (!Array.isArray(value)) throw new Error('组队列表格式无效');
    return value;
}

function uint32(value: unknown, minimum = 0, maximum = 0xffffffff): number {
    if (typeof value !== 'number' || !Number.isInteger(value) || value < minimum || value > maximum) throw new Error('组队数值无效');
    return value;
}

function flag(value: unknown): boolean {
    if (typeof value !== 'boolean') throw new Error('组队标记无效');
    return value;
}

function name(value: unknown): string {
    if (typeof value !== 'string' || !value.trim()) throw new Error('组队名称无效');
    return value;
}

function hex(value: unknown, optional = false): string {
    if (optional && (value == null || value instanceof Uint8Array && value.length === 0)) return '';
    if (!(value instanceof Uint8Array) || value.length !== 16) throw new Error('组队标识必须为16字节');
    return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
}

function bytes(value: string): Uint8Array {
    if (!identifierPattern.test(value)) throw new Error('组队标识无效');
    return Uint8Array.from(value.match(/../g)!, pair => parseInt(pair, 16));
}

function roleId(context: DungeonPartyContext): string {
    const match = /^([1-9]\d*):([a-f0-9]{32})$/.exec(context.attachment.role);
    if (!match) throw new Error('当前角色身份无效');
    travelInteger(match[1], true);
    return match[2];
}

function uint64(value: unknown, positive = false): Long {
    return Long.fromString(travelInteger(value as TravelInteger, positive), true);
}

function conditions(value: DungeonPartyConditions): DungeonPartyConditions {
    return { minimumLevel: uint32(value.minimumLevel), allowedJobsMask: uint32(value.allowedJobsMask, 0, 7), allowSameJob: flag(value.allowSameJob) };
}

function checkedAttachment(context: DungeonPartyContext): AttachmentIdentity {
    if (!context.ready) throw new Error('场景尚未确认');
    const value = context.attachment;
    attachmentKey(value);
    roleId(context);
    return Object.freeze({ ...value, binding: travelInteger(value.binding, true), scene: travelInteger(value.scene, true),
        baseline: travelInteger(value.baseline, true), map: travelInteger(value.map, true) });
}

// 构造协议允许的最小字段。确认操作沿用这里冻结的条件、角色与队伍修订。
export function prepareDungeonPartyAction(context: DungeonPartyContext, action: DungeonPartyAction,
    snapshot?: DungeonPartySnapshot | null): PreparedDungeonPartyAction {
    const attachment = checkedAttachment(context);
    const request = { sceneEpoch: uint64(attachment.scene, true), action: actions[action.kind] };
    if (action.kind === 'query') return { request, attachment, action: { kind: 'query' } };
    if (context.sceneKind !== 1 && context.sceneKind !== 2) throw new Error('请在普通野外或城镇调整队伍');
    if (!snapshot || snapshot.attachmentKey !== attachmentKey(attachment)) throw new Error('请先刷新当前角色的招募');
    if (snapshot.starting) throw new Error('队伍正在开战，请等待确认');

    const own = snapshot.ownParty;
    const current = roleId(context);
    if (action.kind === 'create') {
        const partyId = hex(action.partyId);
        const template = snapshot.templates.find(value => value.templateId === action.templateId);
        if (!template || context.level < template.minimumLevel) throw new Error('未达到所选讨伐的入口要求');
        if (own && (own.partyId !== partyId || own.templateId !== template.templateId || own.leaderRoleId !== current)) throw new Error('当前角色已有队伍');
        const filters = conditions(action.conditions);
        return { request: { ...request, partyId: bytes(partyId), templateId: template.templateId, ...filters }, attachment,
            action: { ...action, partyId: bytes(partyId), conditions: filters } };
    }

    const party = own?.partyId === action.partyId ? own : action.kind === 'join' ? snapshot.parties.find(row => row.partyId === action.partyId) : null;
    if (!party) throw new Error('原队伍已变化，请刷新招募');
    const selected = { ...request, partyId: bytes(party.partyId), expectedRevision: uint64(party.revision, true) };
    if (action.kind === 'join') {
        if (own && own !== party) throw new Error('请先离开当前队伍');
        const job = uint32(context.job, 0, 2);
        if (!own && (context.level < party.minimumLevel || !(party.allowedJobsMask & (1 << job)) ||
            !party.allowSameJob && party.members.some(member => member.job === job))) throw new Error('未满足所选队伍的等级或职业条件');
        return { request: selected, attachment, action: { ...action } };
    }
    if (action.kind === 'leave') return { request: selected, attachment, action: { ...action } };
    if (party.leaderRoleId !== current) throw new Error('只有队长才能执行此操作');
    if (action.kind === 'start') {
        if (context.sceneKind !== 1 || party.members.some(member => !member.online)) throw new Error('请在普通野外开战，并等待队员上线');
        return { request: selected, attachment, action: { ...action } };
    }
    if (action.kind === 'kick') {
        if (action.targetRoleId === current || !party.members.some(member => member.roleId === action.targetRoleId)) throw new Error('请选择队内其他角色');
        return { request: { ...selected, targetRoleId: bytes(action.targetRoleId) }, attachment, action: { ...action } };
    }
    const filters = conditions(action.conditions);
    return { request: { ...selected, ...filters }, attachment, action: { ...action, conditions: filters } };
}

// 确认窗关闭后重新核对最新视图，不能把旧操作换成最新名单后再提交。
export function validateDungeonPartyAction(prepared: PreparedDungeonPartyAction, current: DungeonPartyContext,
    snapshot?: DungeonPartySnapshot | null): boolean {
    try {
        const replay = prepareDungeonPartyAction(current, prepared.action, snapshot);
        const original = prepared.request, actual = replay.request;
        return attachmentKey(prepared.attachment) === attachmentKey(replay.attachment) &&
            original.sceneEpoch.equals(actual.sceneEpoch) && original.action === actual.action &&
            original.expectedRevision?.toString() === actual.expectedRevision?.toString() &&
            hex(original.partyId, true) === hex(actual.partyId, true) && hex(original.targetRoleId, true) === hex(actual.targetRoleId, true) &&
            original.templateId === actual.templateId && original.minimumLevel === actual.minimumLevel &&
            original.allowedJobsMask === actual.allowedJobsMask && original.allowSameJob === actual.allowSameJob;
    } catch { return false; }
}

function readTemplate(value: unknown): DungeonPartyTemplate {
    const row = record(value);
    return { templateId: uint32(row.templateId, 1), displayName: name(row.displayName), minimumLevel: uint32(row.minimumLevel, 1),
        durationSeconds: uint32(row.durationSeconds, 1) };
}

function readMember(value: unknown): DungeonPartyMember {
    const row = record(value);
    return { roleId: hex(row.roleId), displayName: name(row.displayName), job: uint32(row.job ?? 0, 0, 2),
        level: uint32(row.level, 1), online: flag(row.online ?? false) };
}

function readParty(value: unknown, templates: ReadonlyMap<number, DungeonPartyTemplate>): DungeonPartyView {
    const row = record(value);
    const templateId = uint32(row.templateId, 1), template = templates.get(templateId);
    const minimumLevel = uint32(row.minimumLevel, template?.minimumLevel ?? 1);
    const members = list(row.members).map(readMember), leaderRoleId = hex(row.leaderRoleId);
    const deadline = travelInteger((row.leaderOfflineUntilUnixMs ?? Long.ZERO) as TravelInteger);
    if (!template || members.length < 1 || members.length > maxPartyMembers || members[0].roleId !== leaderRoleId ||
        new Set(members.map(member => member.roleId)).size !== members.length || BigInt(deadline) > 9223372036854775807n) throw new Error('组队名单无效');
    return { partyId: hex(row.partyId), revision: uint64(row.revision, true), templateId, minimumLevel,
        allowedJobsMask: uint32(row.allowedJobsMask, 1, 7), allowSameJob: flag(row.allowSameJob ?? false), leaderRoleId,
        members, leaderOfflineUntilUnixMs: Long.fromString(deadline, false) };
}

// WebSocket 已按原请求号和绑定配对。再检查此响应是否仍属于界面持有的场景、角色与开战动作。
export function observeDungeonPartyState(raw: unknown, prepared: PreparedDungeonPartyAction,
    current: DungeonPartyContext): DungeonPartySnapshot | null {
    try {
        const key = attachmentKey(checkedAttachment(current));
        const row = record(raw);
        if (key !== attachmentKey(prepared.attachment) || travelInteger(row.sceneEpoch as TravelInteger, true) !== prepared.request.sceneEpoch.toString()) return null;
        const templates = list(row.templates ?? []).map(readTemplate);
        const templateMap = new Map(templates.map(template => [template.templateId, template]));
        if (templateMap.size !== templates.length) return null;
        const parties = list(row.parties ?? []).map(value => readParty(value, templateMap));
        if (parties.length > publicPartyLimit || new Set(parties.map(party => party.partyId)).size !== parties.length ||
            parties.some(party => party.members.length >= maxPartyMembers || !party.members[0].online)) return null;
        const ownParty = row.ownParty == null ? null : readParty(row.ownParty, templateMap);
        const member = roleId(current);
        if (ownParty && !ownParty.members.some(value => value.roleId === member)) return null;
        const more = flag(row.more ?? false), starting = flag(row.starting ?? false);
        if (starting && !ownParty) return null;
        const errorCode = uint32(row.errorCode ?? 0), challengeId = hex(row.challengeId, true);
        const kind = prepared.action.kind;
        const successStart = errorCode === 0 && kind === 'start';
        if (!!challengeId !== successStart || successStart && starting) return null;
        if (errorCode === 0 && kind !== 'query') {
            if (kind === 'leave' ? ownParty !== null : ownParty?.partyId !== hex(prepared.request.partyId)) return null;
            if (kind === 'create' && (ownParty?.leaderRoleId !== member || ownParty.templateId !== prepared.request.templateId)) return null;
            if (ownParty && prepared.request.expectedRevision && ownParty.revision.lessThan(prepared.request.expectedRevision)) return null;
            if (kind === 'kick' && ownParty?.members.some(value => value.roleId === hex(prepared.request.targetRoleId))) return null;
        }
        return { attachmentKey: key, templates, parties, ownParty, more, starting, challengeId, errorCode };
    } catch { return null; }
}
