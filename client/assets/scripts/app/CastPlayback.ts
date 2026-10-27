import { PotionInteger } from './PotionModel';
import { gridDirection, isAreaEffect, isGroundSkill, SkillDefinition } from './SkillModel';

export interface CastEvent {
    sceneEpoch: PotionInteger; castId: PotionInteger; skillId: number; casterHandle: number; targetHandle: number;
    origin: { x: number; y: number }; targetGrid: { x: number; y: number }; direction: number; delayMs: number;
    casterLife?: PotionInteger; targetLife?: PotionInteger; skillLevel?: number;
}
export interface AttackEvent {
    sceneEpoch: PotionInteger; eventId: PotionInteger; casterHandle: number; targetHandle: number; origin: { x: number; y: number };
    targetGrid: { x: number; y: number }; direction: number; delayMs: number; attackSequence: number; inputSequence: number;
    skillId?: number; skillLevel?: number; casterLife?: PotionInteger;
}
export interface CastStepEvent {
    sceneEpoch: PotionInteger; castId: PotionInteger; skillId: number; casterHandle: number; casterLife: PotionInteger;
    grid: { x: number; y: number }; direction: number; stepIndex: number; skillLevel: number;
}
export interface ImpactEvent {
    sceneEpoch: PotionInteger; eventId: PotionInteger; skillId: number; casterHandle: number; targetHandle: number;
    targetLife: PotionInteger; targetGrid: { x: number; y: number }; amount: PotionInteger; outcome: number; healing: boolean;
    castId: PotionInteger; attackSequence: number;
}
export interface PlayingCast { event: CastEvent; definition: SkillDefinition; started: number; resolvedAt?: number; hitAt?: number; }
export interface EffectPose { kind: 'cast' | 'projectile' | 'hit' | 'beam'; key: string; elapsed: number; fraction: number; direction: number; }

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
        if (!cast || isAreaEffect(cast.definition) || cast.resolvedAt !== undefined || cast.event.targetHandle !== event.targetHandle
            || (cast.event.targetLife != null && cast.event.targetLife.toString() !== event.targetLife.toString())) return false;
        cast.resolvedAt = now;
        if (event.outcome === 1 || event.outcome === 4) cast.hitAt = now;
        return true;
    }
    expire(now: number, hitDuration: (skillId: number) => number, castDuration?: (skillId: number) => number, beamDuration?: (skillId: number) => number): void {
        this.values = this.values.filter(value => {
            const casterEnd = value.started + (castDuration?.(value.event.skillId) ?? value.definition.cast_ms);
            const end = isAreaEffect(value.definition) ? value.started + value.event.delayMs + hitDuration(value.event.skillId)
                : value.hitAt !== undefined ? value.hitAt + hitDuration(value.event.skillId)
                : value.resolvedAt ?? value.started + value.event.delayMs + 5000;
            const beamEnd = beamDuration?.(value.event.skillId) ? value.started + value.definition.cast_ms + beamDuration(value.event.skillId) : 0;
            return now < Math.max(casterEnd, end, beamEnd);
        });
    }
    clear(): void { this.values = []; this.lastId = 0n; this.lastImpactId = 0n; this.scene = ''; }
}

export function effectPoses(value: PlayingCast, now: number, casterDuration = value.definition.cast_ms, beamDuration = 0): EffectPose[] {
    const elapsed = now - value.started, definition = value.definition, event = value.event;
    if (elapsed < 0) return [];
    const result: EffectPose[] = [];
    const key = event.castId.toString();
    if (elapsed < casterDuration) result.push({ kind: 'cast', key: key + ':cast', elapsed,
        fraction: elapsed / casterDuration, direction: event.direction });
    if (definition.kind === 'lightning' && elapsed >= definition.cast_ms && elapsed < definition.cast_ms + beamDuration) {
        result.push({ kind: 'beam', key: key + ':beam', elapsed: elapsed - definition.cast_ms, fraction: 0, direction: event.direction });
    }
    if (['fireball', 'great_fireball', 'soul_shield', 'blessed_armour'].includes(definition.kind) && value.resolvedAt === undefined && elapsed >= definition.impact_base_ms && elapsed < event.delayMs) {
        const duration = event.delayMs - definition.impact_base_ms;
        result.push({ kind: 'projectile', key: key + ':projectile', elapsed: elapsed - definition.impact_base_ms,
            fraction: duration > 0 ? (elapsed - definition.impact_base_ms) / duration : 1,
            direction: gridDirection(event.origin, event.targetGrid, event.direction * 2, 16) });
    }
    const hitAt = isGroundSkill(definition) ? value.started + event.delayMs : value.hitAt;
    if (hitAt !== undefined && now >= hitAt) result.push({ kind: 'hit', key: key + ':hit', elapsed: now - hitAt, fraction: 0, direction: event.direction });
    return result;
}

export interface MeleeSample { attackSequence: number; direction: number; frame: number; }
export interface PlayingMelee { event: AttackEvent; receivedAt: number; }
export interface PlayingCastStep { event: CastStepEvent; started: number; }

// 每格仅由服务端真正处理后的事件启动；不从首格时间预排后续，也不要求本连接见过旧起手。
export class CastStepPlayback {
    private scene = '';
    private retiredThrough = 0n;
    private readonly seen = new Map<string, { id: bigint; mask: number }>();
    private values: PlayingCastStep[] = [];
    bind(scene: PotionInteger): void { this.clear(); this.scene = scene.toString(); }
    accept(event: CastStepEvent, definition: SkillDefinition | undefined, now: number): boolean {
        const count = definition?.line_cells ?? 0;
        if (event.sceneEpoch.toString() !== this.scene || definition?.kind !== 'hell_fire' || definition.id !== event.skillId || !Number.isFinite(now)
            || !Number.isSafeInteger(count) || count < 1 || count > 32 || !Number.isSafeInteger(event.stepIndex) || event.stepIndex < 1 || event.stepIndex > count
            || !Number.isSafeInteger(event.casterHandle) || event.casterHandle < 1 || !Number.isSafeInteger(event.skillLevel) || event.skillLevel < 0 || event.skillLevel > 3
            || !Number.isSafeInteger(event.direction) || event.direction < 0 || event.direction > 7
            || ![event.grid?.x, event.grid?.y].every(value => Number.isSafeInteger(value) && value >= 0)) return false;
        let id: bigint, life: bigint;
        try { id = BigInt(event.castId.toString()); life = BigInt(event.casterLife.toString()); } catch { return false; }
        if (id <= this.retiredThrough || life < 0n) return false;
        const key = `${event.casterHandle}:${life}:${id}`, bit = 1 << (event.stepIndex - 1), record = this.seen.get(key);
        if (record && (record.mask & bit)) return false;
        this.seen.set(key, { id, mask: (record?.mask ?? 0) | bit });
        if (this.seen.size > 64) {
            const oldest = Array.from(this.seen).sort((a, b) => a[1].id < b[1].id ? -1 : a[1].id > b[1].id ? 1 : 0)[0];
            this.seen.delete(oldest[0]); this.retiredThrough = oldest[1].id > this.retiredThrough ? oldest[1].id : this.retiredThrough;
        }
        this.values.push({ event: { ...event, grid: { ...event.grid } }, started: now });
        if (this.values.length > 128) this.values.shift();
        return true;
    }
    active(now: number, durationMs: number): readonly PlayingCastStep[] {
        this.values = this.values.filter(value => now >= value.started && now - value.started < durationMs); return this.values;
    }
    clear(): void { this.scene = ''; this.retiredThrough = 0n; this.seen.clear(); this.values = []; }
}
// 刀光跟随已受理的那一次身体帧；快照仅改变身体，不凭历史序号补造刀光。
export class MeleePlayback {
    private scene = '';
    private lastId = 0n;
    private values: PlayingMelee[] = [];
    bind(scene: PotionInteger): void { this.scene = scene.toString(); this.lastId = 0n; this.values = []; }
    accept(event: AttackEvent, now: number): boolean {
        if (event.sceneEpoch.toString() !== this.scene || !Number.isFinite(now) || !Number.isSafeInteger(event.skillId) || !event.skillId
            || !Number.isSafeInteger(event.skillLevel) || event.skillLevel! < 0 || event.skillLevel! > 3
            || !Number.isSafeInteger(event.direction) || event.direction < 0 || event.direction > 7) return false;
        let id: bigint; try { id = BigInt(event.eventId.toString()); } catch { return false; }
        if (id <= this.lastId) return false; this.lastId = id;
        this.values.push({ event: { ...event }, receivedAt: now }); if (this.values.length > 32) this.values.shift();
        return true;
    }
    poses(now: number, sample: (handle: number) => MeleeSample | null): Array<{ event: AttackEvent; key: string; frame: number; direction: number }> {
        this.values = this.values.filter(value => now - value.receivedAt < 5000);
        return this.values.flatMap(value => {
            const pose = sample(value.event.casterHandle);
            return pose && pose.attackSequence === value.event.attackSequence
                ? [{ event: value.event, key: `attack:${value.event.eventId}`, frame: pose.frame, direction: pose.direction }] : [];
        });
    }
    clear(): void { this.values = []; this.lastId = 0n; this.scene = ''; }
}
