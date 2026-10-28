import { attachmentKey, travelInteger } from './MapTravelModel';
import type { AttachmentIdentity, TravelInteger } from './MapTravelModel';

export interface TeleportPacket {
    readonly role?: { readonly originServerId: TravelInteger; readonly roleId: Uint8Array };
    readonly fromSceneEpoch: TravelInteger; readonly sceneEpoch: TravelInteger; readonly castId: TravelInteger;
    readonly playerLife: TravelInteger; readonly targetMapId: TravelInteger; readonly position?: { readonly x: number; readonly y: number } | null;
    readonly movementRevision: TravelInteger; readonly outcome: number; readonly transitionId: string;
}
export interface TeleportObservation {
    readonly role: string; readonly fromSceneEpoch: string; readonly sceneEpoch: string; readonly castId: string;
    readonly playerLife: string; readonly targetMapId: string; readonly position: { readonly x: number; readonly y: number } | null;
    readonly movementRevision: string; readonly outcome: number; readonly transitionId: string;
}
export type TeleportPhase = 'idle' | 'pending_entry' | 'attaching' | 'awaiting_ready' | 'complete';
function roleIdentity(value: TeleportPacket['role']): string {
    if (!value || !(value.roleId instanceof Uint8Array) || value.roleId.length !== 16) throw new Error('瞬移结果缺少完整角色身份');
    return travelInteger(value.originServerId, true) + ':' + Array.from(value.roleId, byte => byte.toString(16).padStart(2, '0')).join('');
}
function normalize(value: TeleportPacket): TeleportObservation {
    if (!Number.isInteger(value.outcome) || value.outcome < 1 || value.outcome > 6 || typeof value.transitionId !== 'string') throw new Error('瞬移结果类型无效');
    let position: TeleportObservation['position'] = null;
    if (value.position != null) {
        if (![value.position.x, value.position.y].every(n => Number.isSafeInteger(n) && n >= 0 && n <= 2147483647)) throw new Error('瞬移结果坐标无效');
        position = Object.freeze({ ...value.position });
    }
    return Object.freeze({ role: roleIdentity(value.role), fromSceneEpoch: travelInteger(value.fromSceneEpoch, true), sceneEpoch: travelInteger(value.sceneEpoch, true),
        castId: travelInteger(value.castId, true), playerLife: travelInteger(value.playerLife), targetMapId: travelInteger(value.targetMapId), position,
        movementRevision: travelInteger(value.movementRevision), outcome: value.outcome, transitionId: value.transitionId });
}

// 服务端先在200ms时点决定结果；本模型只隔离观察与附着，不随机选格、改MP或等待资源后再授权迁移。
export class TeleportModel {
    private scope = '';
    private source?: AttachmentIdentity;
    private latestCast = 0n;
    private latestLife = '';
    private observed: TeleportObservation | null = null;
    private transfer: TeleportObservation | null = null;
    private attached = '';
    private arrivalTaken = false;
    private phaseValue: TeleportPhase = 'idle';
    get state(): TeleportObservation | null { return this.observed; }
    get pendingTransfer(): TeleportObservation | null { return this.transfer; }
    get phase(): TeleportPhase { return this.phaseValue; }
    get blocksInput(): boolean { return ['pending_entry', 'attaching', 'awaiting_ready'].includes(this.phaseValue); }
    bind(identity: AttachmentIdentity): void { this.clear(); this.source = { ...identity }; this.scope = attachmentKey(identity); }
    clear(): void { this.scope = ''; this.source = undefined; this.latestCast = 0n; this.latestLife = ''; this.observed = null; this.transfer = null; this.attached = ''; this.arrivalTaken = false; this.phaseValue = 'idle'; }
    accept(value: TeleportPacket, current: AttachmentIdentity, transportBinding: TravelInteger, playerLife: TravelInteger): boolean {
        try {
            if (!this.scope || this.blocksInput || this.scope !== attachmentKey(current) || travelInteger(transportBinding) !== current.binding.toString()) return false;
            const result = normalize(value);
            if (result.role !== current.role || result.fromSceneEpoch !== current.scene.toString() || result.playerLife !== travelInteger(playerLife) || (this.latestLife === result.playerLife && BigInt(result.castId) <= this.latestCast)) return false;
            const cross = result.sceneEpoch !== result.fromSceneEpoch;
            if (result.outcome === 1) {
                if (!result.position || result.targetMapId === '0' || result.movementRevision === '0') return false;
                if (cross) {
                    if (result.targetMapId === current.map.toString() || BigInt(result.sceneEpoch) !== BigInt(result.fromSceneEpoch) + 1n || !/^[0-9a-f]{32}$/.test(result.transitionId)) return false;
                } else if (result.transitionId || result.targetMapId !== current.map.toString()) return false;
            } else if (cross || result.transitionId) return false;
            this.latestLife = result.playerLife; this.latestCast = BigInt(result.castId); this.observed = result;
            this.transfer = null; this.attached = ''; this.arrivalTaken = false; this.phaseValue = 'idle';
            if (cross) { this.transfer = result; this.phaseValue = 'pending_entry'; }
            return true;
        } catch { return false; }
    }
    matchesEntry(identity: AttachmentIdentity, position: { readonly x: number; readonly y: number } | null | undefined): boolean {
        const result = this.transfer;
        return !!result && !!this.source && identity.connection === this.source.connection && identity.binding.toString() === this.source.binding.toString()
            && identity.role === result.role && identity.scene.toString() === result.sceneEpoch && identity.map.toString() === result.targetMapId
            && !identity.travelId && identity.serverTransitionId === result.transitionId && position?.x === result.position!.x && position?.y === result.position!.y;
    }
    attach(identity: AttachmentIdentity, position: { readonly x: number; readonly y: number }): void {
        if (this.phaseValue !== 'pending_entry' || !this.matchesEntry(identity, position)) throw new Error('新入场不匹配服务端瞬移结果');
        this.attached = attachmentKey(identity); this.phaseValue = 'attaching';
    }
    acknowledge(identity: AttachmentIdentity): boolean {
        if (this.phaseValue !== 'attaching' || attachmentKey(identity) !== this.attached) return false;
        this.phaseValue = 'awaiting_ready'; return true;
    }
    ready(identity: AttachmentIdentity): boolean {
        if (this.phaseValue !== 'awaiting_ready' || attachmentKey(identity) !== this.attached) return false;
        this.source = { ...identity }; this.scope = attachmentKey(identity); this.latestLife = ''; this.latestCast = 0n;
        this.phaseValue = 'complete'; return true;
    }
    takeArrival(identity: AttachmentIdentity, playerLife: TravelInteger, movementRevision: TravelInteger): TeleportObservation | null {
        if (this.phaseValue !== 'complete' || this.arrivalTaken || attachmentKey(identity) !== this.attached) return null;
        this.arrivalTaken = true;
        return this.transfer?.playerLife === travelInteger(playerLife) && this.transfer.movementRevision === travelInteger(movementRevision) ? this.transfer : null;
    }
}

export function teleportFailureText(outcome: number): string {
    const reason = ({ 2: '当前地图禁止本次瞬移', 3: '本次未找到可传送位置', 4: '绑定点暂不具备传送资格', 5: '本次施法的角色身份已失效', 6: '本次地图迁移未能完成' } as Record<number, string>)[outcome];
    return reason ? reason + '；已消耗的魔法不会返还' : '';
}
