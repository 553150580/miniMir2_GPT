import { PotionInteger } from './PotionModel';
import { gridDirection, SkillDefinition } from './SkillModel';

export interface CastEvent {
    sceneEpoch: PotionInteger; castId: PotionInteger; skillId: number; casterHandle: number; targetHandle: number;
    origin: { x: number; y: number }; targetGrid: { x: number; y: number }; direction: number; delayMs: number;
}
export interface PlayingCast { event: CastEvent; definition: SkillDefinition; started: number; }
export interface EffectPose { kind: 'cast' | 'projectile' | 'hit'; key: string; elapsed: number; fraction: number; direction: number; }

// 瞬时表现只属于收到事件时的当前世界；场景变化、过期与销毁均丢弃，不做重连补播。
export class CastPlayback {
    private lastId = 0n;
    private scene = '';
    private values: PlayingCast[] = [];
    readonly capacity = 32;

    bind(scene: PotionInteger): void { this.scene = scene.toString(); this.lastId = 0n; this.values = []; }
    get casts(): readonly PlayingCast[] { return this.values; }
    accept(event: CastEvent, definition: SkillDefinition | undefined, now: number): boolean {
        if (!definition || !definition.active || event.sceneEpoch.toString() !== this.scene || !Number.isFinite(now)
            || !Number.isSafeInteger(event.delayMs) || event.delayMs < 0 || event.delayMs > 30000) return false;
        let id: bigint; try { id = BigInt(event.castId.toString()); } catch { return false; }
        if (id <= this.lastId || ![event.origin.x, event.origin.y, event.targetGrid.x, event.targetGrid.y].every(Number.isFinite)) return false;
        this.lastId = id;
        this.values.push({ event: { ...event, origin: { ...event.origin }, targetGrid: { ...event.targetGrid } }, definition, started: now });
        if (this.values.length > this.capacity) this.values.shift();
        return true;
    }
    expire(now: number, hitDuration: (skillId: number) => number): void {
        this.values = this.values.filter(value => now - value.started < Math.max(value.definition.cast_ms, value.event.delayMs + hitDuration(value.event.skillId)));
    }
    clear(): void { this.values = []; this.lastId = 0n; this.scene = ''; }
}

export function effectPoses(value: PlayingCast, now: number): EffectPose[] {
    const elapsed = now - value.started, definition = value.definition, event = value.event;
    if (elapsed < 0) return [];
    const result: EffectPose[] = [];
    const key = event.castId.toString();
    if (elapsed < definition.cast_ms) result.push({ kind: 'cast', key: key + ':cast', elapsed,
        fraction: elapsed / definition.cast_ms, direction: event.direction });
    if (definition.kind === 'fireball' && elapsed >= definition.impact_base_ms && elapsed < event.delayMs) {
        const duration = event.delayMs - definition.impact_base_ms;
        result.push({ kind: 'projectile', key: key + ':projectile', elapsed: elapsed - definition.impact_base_ms,
            fraction: duration > 0 ? (elapsed - definition.impact_base_ms) / duration : 1,
            direction: gridDirection(event.origin, event.targetGrid, event.direction * 2, 16) });
    }
    if (elapsed >= event.delayMs) result.push({ kind: 'hit', key: key + ':hit', elapsed: elapsed - event.delayMs, fraction: 0, direction: event.direction });
    return result;
}
