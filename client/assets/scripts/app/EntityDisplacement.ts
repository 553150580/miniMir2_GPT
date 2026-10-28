import type { MotionPoint } from './EntityMotion';

type UInt64 = { toString(): string } | bigint | string;
export interface DisplacementEvent {
    readonly sceneEpoch: UInt64;
    readonly entityHandle: number;
    readonly entityLife: UInt64;
    readonly movementRevision: UInt64;
    readonly kind: number;
    readonly origin: MotionPoint;
    readonly path: readonly MotionPoint[];
    readonly facingDirection: number;
    readonly movementDirection: number;
    readonly stepDurationMs: number;
    readonly sourceSkillId: number;
    readonly castId: UInt64;
    readonly attackSequence: number;
    readonly castSequence: number;
}
export type MovementBaselineDecision = 'initial' | 'walk' | 'recover' | 'same' | 'stale';
export interface AcceptedDisplacement { readonly event: DisplacementEvent; readonly recover: boolean; }
const max64 = (1n << 64n) - 1n;
function u64(value: UInt64): bigint {
    if (typeof value === 'number') throw new Error('64位位移身份不能使用Number');
    const text = value?.toString();
    if (!text || !/^\d+$/.test(text)) throw new Error('位移水位无效');
    const parsed = BigInt(text); if (parsed > max64) throw new Error('位移水位溢出');
    return parsed;
}
const point = (value: MotionPoint): boolean => !!value && Number.isSafeInteger(value.x) && Number.isSafeInteger(value.y);
const same = (a: MotionPoint, b: MotionPoint): boolean => a.x === b.x && a.y === b.y;
const uint32 = (value: number): boolean => Number.isInteger(value) && value >= 0 && value <= 0xffffffff;
const direction = (value: number): boolean => Number.isInteger(value) && value >= 0 && value < 8;

// 全量位置负责恢复，瞬时事件只补充尚未被基线消费的真实经过格。
export class EntityDisplacement {
    private life: bigint | null = null;
    private revision = 0n;
    private target: MotionPoint = { x: 0, y: 0 };
    private readonly scene: bigint;
    constructor(sceneEpoch: UInt64, private readonly handle: number) { this.scene = u64(sceneEpoch); }
    get position(): MotionPoint { return { ...this.target }; }

    observe(entityLife: UInt64, movementRevision: UInt64, kind: number, position: MotionPoint): MovementBaselineDecision {
        const life = u64(entityLife), revision = u64(movementRevision);
        if (!point(position)) throw new Error('实体权威落点无效');
        if (this.life === null || this.life !== life) {
            this.life = life; this.revision = revision; this.target = { ...position }; return 'initial';
        }
        if (revision < this.revision) return 'stale';
        if (revision === this.revision && same(position, this.target)) return 'same';
        const previous = this.target, previousRevision = this.revision;
        this.revision = revision; this.target = { ...position };
        return kind === 1 && revision > previousRevision && Math.max(Math.abs(previous.x - position.x), Math.abs(previous.y - position.y)) <= 1
            ? 'walk' : 'recover';
    }

    accept(event: DisplacementEvent): AcceptedDisplacement | null {
        try {
            if (this.life === null || event.entityHandle !== this.handle || u64(event.sceneEpoch) !== this.scene
                || u64(event.entityLife) !== this.life || u64(event.movementRevision) <= this.revision) return null;
            if (!point(event.origin) || !Array.isArray(event.path) || event.path.length > 256
                || !direction(event.facingDirection) || !direction(event.movementDirection)
                || !uint32(event.attackSequence) || !uint32(event.castSequence) || !uint32(event.sourceSkillId)
                || !uint32(event.stepDurationMs) || ![2, 3, 4, 5].includes(event.kind)) return null;
            u64(event.castId);
            if ((event.kind === 4 && event.path.length !== 1) || (event.kind === 5 && event.path.length !== 0)
                || ([2, 3].includes(event.kind) && (!event.path.length || !event.stepDurationMs))) return null;
            let previous = event.origin;
            for (const next of event.path) {
                if (!point(next)) return null;
                const distance = Math.max(Math.abs(previous.x - next.x), Math.abs(previous.y - next.y));
                if ((event.kind === 2 || event.kind === 3) && distance !== 1) return null;
                previous = next;
            }
            const recover = !same(this.target, event.origin);
            this.revision = u64(event.movementRevision); this.target = { ...previous };
            return { event, recover };
        } catch { return null; }
    }
}
