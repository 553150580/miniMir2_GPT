import Long from 'long';
import { GameConnection, Packet, newOperationId } from './GameConnection';
import { SceneManager } from '../core/scene/SceneManager';
import { BaselineAssembler } from './BaselineAssembler';
import { PendingOperations } from './PendingOperations';
import type { WorldAssets } from './WorldAssets';
import { protocolErrorMessage } from './GameErrors';
import type { AttackEvent, CastEvent, CastStepEvent, ImpactEvent } from './CastPlayback';
import { acceptsSkillModeState, SkillModeState } from './SkillModel';
import type { DisplacementEvent } from './EntityDisplacement';
import { MapTravelModel, travelInteger } from './MapTravelModel';
import type { AttachmentIdentity } from './MapTravelModel';
import { RoleBindingModel } from './RoleBindingModel';
import type { RoleBindingObservation } from './RoleBindingModel';
const assembler = new BaselineAssembler();
const bindingModel = new RoleBindingModel();
let quiescingReady: { entry: any; binding: string; connection: number; ready: boolean } | null = null;
let confirmedCommand = Long.UONE;
let localOperation: { id: string; connection: number; binding: string; role: string; scene: string; baseline: string; next: Long } | null = null;

// 连接与数据跨场景保留，不通过持久UI节点重建固定布局。
export const session = {
    connection: new GameConnection(),
    scenes: null as SceneManager | null,
    role: null as any,
    roles: [] as any[],
    entry: null as any,
    entryBinding: '0',
    entryConnection: 0,
    entryRole: '',
    entryTravelId: '',
    travel: new MapTravelModel(),
    travelState: null as any,
    bindingState: null as RoleBindingObservation | null,
    bindingNotice: null as RoleBindingObservation | null,
    baseline: null as any,
    inventory: null as any,
    wallet: null as any,
    hosting: null as any,
    skillModes: null as SkillModeState | null,
    skillModesReceivedAt: 0,
    report: null as any,
    death: null as any,
    ready: false,
    command: Long.UONE,
    pending: null as PendingOperations | null,
    preparedWorld: null as WorldAssets | null,
    lastError: '',
    update: new Set<() => void>(),
    casts: new Set<(event: CastEvent) => void>(),
    attacks: new Set<(event: AttackEvent) => void>(),
    impacts: new Set<(event: ImpactEvent) => void>(),
    castSteps: new Set<(event: CastStepEvent) => void>(),
    displacements: new Set<(event: DisplacementEvent) => void>(),
    quickSkills: new Map<string, number>(),
};
session.connection.subscribe((packet: Packet) => {
    if (packet.error) { session.lastError = protocolErrorMessage(packet.id, packet.error); }
    else switch (packet.id) {
        case 20002:
            acceptEntry(packet); break;
        case 5:
            if (session.entry) {
                const complete = assembler.accept(packet.value, session.entry.sceneEpoch, session.entry.baselineId);
                if (complete && (!session.baseline || complete.stateSequence.greaterThan(session.baseline.stateSequence))) {
                    session.baseline = complete;
                    if (complete.skillModeState) acceptModes(complete.skillModeState);
                }
            }
            break;
        case 6:
            if (session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch) && packet.value.baselineId.equals(session.entry.baselineId)
                && (!session.travel.blocksInput || (!!session.entryTravelId && session.travel.phase === 'awaiting_ready'))) {
                session.ready = true;
                const next = packet.value.nextEconomicCommandSeq;
                if (next.greaterThan(confirmedCommand)) confirmedCommand = next;
                if (confirmedCommand.greaterThan(session.command)) session.command = confirmedCommand;
                const identity = currentEntryIdentity(); if (identity && session.entryTravelId) session.travel.ready(identity);
            }
            break;
        case 20010:
            if (session.role && session.travel.acceptState(packet.value, { connection: session.connection.generation, binding: packet.binding, role: roleIdentity() })) {
                session.travelState = packet.value;
                if (packet.value.phase === 1) {
                    quiescingReady ??= { entry: session.entry, binding: session.entryBinding, connection: session.entryConnection, ready: session.ready };
                    session.ready = false;
                } else if (packet.value.phase === 2) {
                    if (session.entry?.sceneEpoch.toString() === packet.value.fromSceneEpoch.toString()) session.ready = false;
                    quiescingReady = null;
                } else {
                    if (quiescingReady && session.entry === quiescingReady.entry && session.entryBinding === quiescingReady.binding
                        && session.connection.generation === quiescingReady.connection && session.entry?.sceneEpoch.toString() === packet.value.fromSceneEpoch.toString()) session.ready = quiescingReady.ready;
                    quiescingReady = null;
                }
            }
            break;
        case 40003: session.inventory = packet.value; break;
        case 20011: {
            const identity = currentEntryIdentity();
            if (session.role && identity && isCurrentEntry(identity) && bindingModel.accept(packet.value, identity, packet.binding)) {
                session.bindingState = bindingModel.state; session.bindingNotice = bindingModel.notice;
            }
            break;
        }
        case 40004: session.wallet = packet.value; break;
        case 30005: session.hosting = packet.value; break;
        case 30010: acceptModes(packet.value); break;
        case 30001: session.death = packet.value; break;
        case 30006:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.casts)) listener(packet.value);
            }
            break;
        case 30007:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.attacks)) listener(packet.value);
            }
            break;
        case 30008:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.impacts)) listener(packet.value);
            }
            break;
        case 30012:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.castSteps)) listener(packet.value);
            }
            break;
        case 30013:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.displacements)) listener(packet.value);
            }
            break;
        case 40008: session.report = packet.value; break;
        case 40005: acceptOperationReceipt(packet); break;
    }
    notify();
});
session.connection.onStatus(message => { session.lastError = message; session.ready = false; session.travel.reset(); bindingModel.clear(); session.bindingState = null; session.bindingNotice = null; quiescingReady = null; localOperation = null; notify(); });

function acceptEntry(packet: Packet): void {
    const value = packet.value;
    try {
        if (!Number.isInteger(value.playerHandle) || value.playerHandle < 1 || value.playerHandle > 0xffffffff || ![1, 2].includes(value.sceneKind)) throw new Error('入场缺少有效玩家身份或地图类型');
        travelInteger(value.sceneEpoch, true); travelInteger(value.baselineId, true); travelInteger(value.roomId, true); travelInteger(value.resourceVersion, true);
        const sameConnection = session.entry && session.entryBinding === packet.binding.toString() && session.entryConnection === session.connection.generation;
        if (sameConnection && BigInt(value.sceneEpoch.toString()) <= BigInt(session.entry.sceneEpoch.toString())) {
            // 同连接已附着的场景不能由重复或迟到Enter回执退回；也不撤销新场景Ready。
            return;
        }
        const identity: AttachmentIdentity = { connection: session.connection.generation, binding: packet.binding, role: roleIdentity(), scene: value.sceneEpoch, baseline: value.baselineId, map: value.roomId, travelId: session.travel.id };
        const travelling = session.travel.acceptsEntry(identity);
        if (sameConnection && session.travel.active && !travelling) throw new Error('新地图入场不匹配旅行提交身份');
        if (travelling && value.sceneKind !== session.travel.plan?.targetSceneKind) throw new Error('新地图入场类型不匹配旅行计划');
        if (travelling) session.travel.attach(identity); else session.travel.reset();
        assembler.reset(); session.entry = value; session.baseline = null; session.ready = false; quiescingReady = null;
        session.entryBinding = packet.binding.toString(); session.entryConnection = session.connection.generation; session.entryRole = identity.role; session.entryTravelId = travelling ? identity.travelId : '';
        bindingModel.bind(currentEntryIdentity()!); session.bindingState = null; session.bindingNotice = null;
        session.inventory = null; session.wallet = null; session.hosting = null; session.report = null; session.death = null; session.skillModes = null; session.skillModesReceivedAt = 0;
        confirmedCommand = value.nextEconomicCommandSeq ?? Long.UONE; session.command = confirmedCommand; localOperation = null;
    } catch (error) { session.ready = false; session.lastError = error instanceof Error ? error.message : String(error); }
}
export function currentEntryIdentity(): AttachmentIdentity | null {
    const entry = session.entry; if (!entry) return null;
    return { connection: session.entryConnection, binding: session.entryBinding, role: session.entryRole,
        scene: entry.sceneEpoch, baseline: entry.baselineId, map: entry.roomId, travelId: session.entryTravelId };
}
export function isCurrentEntry(identity: AttachmentIdentity): boolean {
    const current = currentEntryIdentity();
    return !!current && session.connection.generation === identity.connection && session.connection.binding.toString() === identity.binding.toString()
        && roleIdentity() === identity.role && current.connection === identity.connection && current.binding.toString() === identity.binding.toString()
        && current.scene.toString() === identity.scene.toString() && current.baseline.toString() === identity.baseline.toString()
        && current.map.toString() === identity.map.toString() && current.travelId === identity.travelId;
}

function notify(): void { for (const update of Array.from(session.update)) { try { update(); } catch (error) { session.lastError = error instanceof Error ? error.message : String(error); } } }
function acceptModes(value: SkillModeState): void {
    if (session.entry && acceptsSkillModeState(session.skillModes, value, session.entry.sceneEpoch)) {
        session.skillModes = value; session.skillModesReceivedAt = Date.now();
    }
}

export function roleIdentity(): string {
    if (!session.role) throw new Error('尚未选择角色');
    return session.role.key.originServerId.toString() + ':' + Array.from(session.role.key.roleId as Uint8Array, value => value.toString(16).padStart(2, '0')).join('');
}

export function operation(): any {
    if (!session.ready) throw new Error('场景基线尚未确认');
    // 绑定只观察永久修订；新经济操作仍等完整Inventory，不能拿自动SourceSeq或绑定修订改写旧Pending。
    if (session.bindingState && (!session.inventory || BigInt(session.bindingState.roleRevision) > BigInt(session.inventory.revision.toString()))) throw new Error('角色资料正在同步，请稍后操作');
    const result = { opId: newOperationId(), commandSeq: session.command,
        expectedRevision: session.inventory?.revision ?? Long.UZERO,
        expectedRuleVersion: session.entry?.resourceVersion ?? Long.UONE };
    session.command = session.command.add(1);
    localOperation = { id: bytesIdentity(result.opId), connection: session.connection.generation, binding: session.connection.binding.toString(),
        role: roleIdentity(), scene: session.entry.sceneEpoch.toString(), baseline: session.entry.baselineId.toString(), next: session.command };
    return result;
}

function bytesIdentity(value: Uint8Array): string { return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join(''); }
function acceptOperationReceipt(packet: Packet): void {
    const value = packet.value; if (value.pending) return;
    const next = value.commandSeq.add(1); if (next.greaterThan(confirmedCommand)) confirmedCommand = next;
    const current = localOperation && value.opId instanceof Uint8Array && bytesIdentity(value.opId) === localOperation.id
        && session.connection.generation === localOperation.connection && packet.binding.toString() === localOperation.binding
        && session.entryRole === localOperation.role && session.entry?.sceneEpoch.toString() === localOperation.scene
        && session.entry?.baselineId.toString() === localOperation.baseline && session.command.equals(localOperation.next);
    // 当前op无副作用拒绝可以撤回本地推测；其它回执不能把新场景/新op水位拉低。
    if (current) { session.command = confirmedCommand; localOperation = null; }
    else if (confirmedCommand.greaterThan(session.command)) session.command = confirmedCommand;
}
