import { attachmentKey, travelInteger } from './MapTravelModel';
import type { AttachmentIdentity, TravelInteger } from './MapTravelModel';

export interface PetBornEvent {
    sceneEpoch: TravelInteger; baselineId: TravelInteger; stateSequence: TravelInteger;
    entityHandle: number; entityLife: TravelInteger; definitionId: number; mapId: TravelInteger;
    sourceCastId: TravelInteger; sourceSkillId: number; bornAtUnixMs: TravelInteger; movementRevision: TravelInteger;
    direction: number; role?: { originServerId: TravelInteger; roleId: Uint8Array }; elapsedMs: number;
}
export interface PetBornObservation {
    readonly sceneEpoch: string; readonly baselineId: string; readonly stateSequence: string;
    readonly entityHandle: number; readonly entityLife: string; readonly definitionId: number; readonly mapId: string;
    readonly sourceCastId: string; readonly sourceSkillId: number; readonly bornAtUnixMs: string;
    readonly movementRevision: string; readonly direction: number; readonly role: string; readonly elapsedMs: number;
}
export interface PetBornPlayback { readonly event: PetBornObservation; readonly elapsedMs: number; }
// 每个新生事件按身体模板解析出场时长。旧资源包沿用骷髅默认值。
export function petAppearDuration(manifest: any, definitionId?: number, gameplay?: any): number {
    const profiles = gameplay?.pet_profiles;
    const key = profiles && definitionId !== undefined
        ? profiles.find((row: { definition_id: number }) => row.definition_id === definitionId)?.profile : 'pet_skeleton';
    const value = key && manifest?.profiles?.[key]?.layers?.[0]?.actions?.appear;
    if (!value || !Number.isSafeInteger(value.count) || value.count <= 0 || !Number.isSafeInteger(value.duration_ms) || value.duration_ms <= 0
        || !Number.isSafeInteger(value.count * value.duration_ms)) return 0;
    return value.count * value.duration_ms;
}

// 一次附着仅保留一个尚待完整基线的出生通知；消费水位属于Session，不属于可重建的visual。
export class PetBornModel {
    private scope = '';
    private life = 0n;
    private pending: { event: PetBornObservation; received: number } | null = null;
    get pendingCount(): number { return this.pending ? 1 : 0; }
    bind(identity: AttachmentIdentity): void { this.clear(); this.scope = attachmentKey(identity); }
    clear(): void { this.scope = ''; this.life = 0n; this.pending = null; }
    discardPending(): void { this.pending = null; }
    accept(value: PetBornEvent, identity: AttachmentIdentity, binding: TravelInteger, now: number): boolean {
        try {
            if (!this.scope || this.scope !== attachmentKey(identity) || travelInteger(binding, true) !== identity.binding.toString() || !Number.isFinite(now)) return false;
            if (!value.role || !(value.role.roleId instanceof Uint8Array) || value.role.roleId.length !== 16) return false;
            const role = travelInteger(value.role.originServerId, true) + ':' + Array.from(value.role.roleId, n => n.toString(16).padStart(2, '0')).join('');
            const event: PetBornObservation = Object.freeze({ ...value, role,
                sceneEpoch: travelInteger(value.sceneEpoch, true), baselineId: travelInteger(value.baselineId, true), stateSequence: travelInteger(value.stateSequence, true),
                entityLife: travelInteger(value.entityLife, true), mapId: travelInteger(value.mapId, true), sourceCastId: travelInteger(value.sourceCastId, true),
                bornAtUnixMs: travelInteger(value.bornAtUnixMs, true), movementRevision: travelInteger(value.movementRevision) });
            if (event.role !== identity.role || event.sceneEpoch !== identity.scene.toString() || event.baselineId !== identity.baseline.toString() || event.mapId !== identity.map.toString()
                || ![event.entityHandle, event.definitionId].every(n => Number.isSafeInteger(n) && n > 0 && n <= 0xffffffff)
                || ![17, 30].includes(event.sourceSkillId) || !Number.isInteger(event.direction) || event.direction < 0 || event.direction > 7
                || event.sourceSkillId === 17 && event.direction !== 5 || !Number.isSafeInteger(event.elapsedMs) || event.elapsedMs < 0 || event.elapsedMs > 0xffffffff
                || BigInt(event.entityLife) <= this.life) return false;
            this.life = BigInt(event.entityLife); this.pending = { event, received: now }; return true;
        } catch { return false; }
    }
    take(identity: AttachmentIdentity, baseline: any, duration: number | ((event: PetBornObservation) => number), now: number): PetBornPlayback | null {
        const pending = this.pending;
        if (!pending) return null;
        try {
            const durationMs = typeof duration === 'function' ? duration(pending.event) : duration;
            if (this.scope !== attachmentKey(identity) || !Number.isFinite(now) || now < pending.received || !Number.isSafeInteger(durationMs) || durationMs <= 0) {
                this.pending = null; return null;
            }
            const event = pending.event, elapsedMs = event.elapsedMs + now - pending.received;
            if (elapsedMs >= durationMs) { this.pending = null; return null; }
            if (!baseline) return null;
            if (baseline.sceneEpoch?.toString() !== event.sceneEpoch || baseline.baselineId?.toString() !== event.baselineId) { this.pending = null; return null; }
            if (BigInt(travelInteger(baseline.stateSequence, true)) < BigInt(event.stateSequence)) return null;
            this.pending = null;
            const body = baseline.entities?.find((row: any) => row.entityHandle === event.entityHandle);
            if (!body || body.entityKind !== 2 || body.isPetCorpse || body.life?.toString() !== event.entityLife || body.definitionId !== event.definitionId
                || BigInt(body.hp.toString()) <= 0n || travelInteger(body.movementRevision) !== event.movementRevision || body.direction !== event.direction) return null;
            return Object.freeze({ event, elapsedMs });
        } catch { this.pending = null; return null; }
    }
}
