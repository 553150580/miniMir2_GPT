import { PotionInteger } from './PotionModel';
import { gridDirection, isGroundSkill, SkillDefinition } from './SkillModel';

export interface CastEvent {
    sceneEpoch: PotionInteger; castId: PotionInteger; skillId: number; casterHandle: number; targetHandle: number;
    origin: { x: number; y: number }; targetGrid: { x: number; y: number }; direction: number; delayMs: number;
    casterLife?: PotionInteger; targetLife?: PotionInteger;
}
export interface AttackEvent {
    sceneEpoch: PotionInteger; eventId: PotionInteger; casterHandle: number; targetHandle: number; origin: { x: number; y: number };
    targetGrid: { x: number; y: number }; direction: number; delayMs: number; attackSequence: number; inputSequence: number;
}
export interface ImpactEvent {
    sceneEpoch: PotionInteger; eventId: PotionInteger; skillId: number; casterHandle: number; targetHandle: number;
    targetLife: PotionInteger; targetGrid: { x: number; y: number }; amount: PotionInteger; outcome: number; healing: boolean;
    castId: PotionInteger; attackSequence: number;
}
export interface PlayingCast { event: CastEvent; definition: SkillDefinition; started: number; resolvedAt?: number; hitAt?: number; }
export interface EffectPose { kind: 'cast' | 'projectile' | 'hit'; key: string; elapsed: number; fraction: number; direction: number; }

// 瞬时表现只属于收到事件时的当前世界；场景变化、过期与销毁均丢弃，不做重连补播。
export class CastPlayback {
    private lastId = 0n;
    private lastImpactId = 0n;
    private scene = '';
    private values: PlayingCast[] = [];
    readonly capacity = 32;

    bind(scene: PotionInteger): void { this.scene = scene.toString(); this.lastId = 0n; this.lastImpactId = 0n; this.values = []; }
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
    impact(event: ImpactEvent, now: number): boolean {
        if (event.sceneEpoch.toString() !== this.scene || !Number.isFinite(now)) return false;
        let id: bigint; try { id = BigInt(event.eventId.toString()); } catch { return false; }
        if (id <= this.lastImpactId) return false; this.lastImpactId = id;
        const cast = this.values.find(value => value.event.castId.toString() === event.castId.toString() && value.event.skillId === event.skillId);
        // 地表范围特效只由一次受理驱动，不能按每个受影响实体重复播放。
        if (!cast || isGroundSkill(cast.definition) || cast.resolvedAt !== undefined || cast.event.targetHandle !== event.targetHandle
            || (cast.event.targetLife && cast.event.targetLife.toString() !== event.targetLife.toString())) return false;
        cast.resolvedAt = now;
        if (event.outcome === 1 || event.outcome === 4) cast.hitAt = now;
        return true;
    }
    expire(now: number, hitDuration: (skillId: number) => number, castDuration?: (skillId: number) => number): void {
        this.values = this.values.filter(value => {
            const casterEnd = value.started + (castDuration?.(value.event.skillId) ?? value.definition.cast_ms);
            const end = isGroundSkill(value.definition) ? value.started + value.event.delayMs + hitDuration(value.event.skillId)
                : value.hitAt !== undefined ? value.hitAt + hitDuration(value.event.skillId)
                : value.resolvedAt ?? value.started + value.event.delayMs + 5000;
            return now < Math.max(casterEnd, end);
        });
    }
    clear(): void { this.values = []; this.lastId = 0n; this.lastImpactId = 0n; this.scene = ''; }
}

export function effectPoses(value: PlayingCast, now: number, casterDuration = value.definition.cast_ms): EffectPose[] {
    const elapsed = now - value.started, definition = value.definition, event = value.event;
    if (elapsed < 0) return [];
    const result: EffectPose[] = [];
    const key = event.castId.toString();
    if (elapsed < casterDuration) result.push({ kind: 'cast', key: key + ':cast', elapsed,
        fraction: elapsed / casterDuration, direction: event.direction });
    if ((definition.kind === 'fireball' || definition.kind === 'great_fireball') && value.resolvedAt === undefined && elapsed >= definition.impact_base_ms && elapsed < event.delayMs) {
        const duration = event.delayMs - definition.impact_base_ms;
        result.push({ kind: 'projectile', key: key + ':projectile', elapsed: elapsed - definition.impact_base_ms,
            fraction: duration > 0 ? (elapsed - definition.impact_base_ms) / duration : 1,
            direction: gridDirection(event.origin, event.targetGrid, event.direction * 2, 16) });
    }
    const hitAt = isGroundSkill(definition) ? value.started + event.delayMs : value.hitAt;
    if (hitAt !== undefined && now >= hitAt) result.push({ kind: 'hit', key: key + ':hit', elapsed: now - hitAt, fraction: 0, direction: event.direction });
    return result;
}
