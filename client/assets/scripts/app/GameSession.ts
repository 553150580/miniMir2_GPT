import Long from 'long';
import { GameConnection, Packet, newOperationId } from './GameConnection';
import { SceneManager } from '../core/scene/SceneManager';
import { BaselineAssembler } from './BaselineAssembler';
import { PendingOperations } from './PendingOperations';
import type { WorldAssets } from './WorldAssets';
import { protocolErrorMessage } from './GameErrors';
const assembler = new BaselineAssembler();

// 连接与数据跨场景保留，不通过持久UI节点重建固定布局。
export const session = {
    connection: new GameConnection(),
    scenes: null as SceneManager | null,
    role: null as any,
    roles: [] as any[],
    entry: null as any,
    baseline: null as any,
    inventory: null as any,
    wallet: null as any,
    hosting: null as any,
    report: null as any,
    death: null as any,
    ready: false,
    command: Long.UONE,
    pending: null as PendingOperations | null,
    preparedWorld: null as WorldAssets | null,
    lastError: '',
    update: new Set<() => void>(),
};
session.connection.subscribe((packet: Packet) => {
    if (packet.error) { session.lastError = protocolErrorMessage(packet.id, packet.error); }
    else switch (packet.id) {
        case 20002:
            assembler.reset(); session.entry = packet.value; session.baseline = null; session.ready = false;
            session.inventory = null; session.wallet = null; session.hosting = null; session.report = null; session.death = null;
            session.command = packet.value.nextEconomicCommandSeq ?? Long.UONE; break;
        case 5:
            if (session.entry) {
                const complete = assembler.accept(packet.value, session.entry.sceneEpoch, session.entry.baselineId);
                if (complete && (!session.baseline || complete.stateSequence.greaterThan(session.baseline.stateSequence))) session.baseline = complete;
            }
            break;
        case 6:
            if (session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch) && packet.value.baselineId.equals(session.entry.baselineId)) {
                session.ready = true; session.command = packet.value.nextEconomicCommandSeq;
            }
            break;
        case 40003: session.inventory = packet.value; break;
        case 40004: session.wallet = packet.value; break;
        case 30005: session.hosting = packet.value; break;
        case 30001: session.death = packet.value; break;
        case 40008: session.report = packet.value; break;
        case 40005: if (!packet.value.pending) session.command = packet.value.commandSeq.add(1); break;
    }
    notify();
});
session.connection.onStatus(message => { session.lastError = message; session.ready = false; notify(); });

function notify(): void { for (const update of Array.from(session.update)) { try { update(); } catch (error) { session.lastError = error instanceof Error ? error.message : String(error); } } }

export function roleIdentity(): string {
    if (!session.role) throw new Error('尚未选择角色');
    return session.role.key.originServerId.toString() + ':' + Array.from(session.role.key.roleId as Uint8Array, value => value.toString(16).padStart(2, '0')).join('');
}

export function operation(): any {
    if (!session.ready) throw new Error('场景基线尚未确认');
    const result = { opId: newOperationId(), commandSeq: session.command,
        expectedRevision: session.inventory?.revision ?? Long.UZERO,
        expectedRuleVersion: session.entry?.resourceVersion ?? Long.UONE };
    session.command = session.command.add(1);
    return result;
}
