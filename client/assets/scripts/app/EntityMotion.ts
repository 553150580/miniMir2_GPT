export interface MotionPoint { readonly x: number; readonly y: number; }
export interface MotionTarget extends MotionPoint {
    readonly direction: number;
    // 调用方传入权威移动间隔；积压时允许缩短显示时长以追上快照。
    readonly durationMs: number;
    readonly snap?: boolean;
}
export interface MotionFrame extends MotionPoint {
    readonly moving: boolean;
    readonly direction: number;
    readonly stepProgress: number;
    readonly stepSequence: number;
}
export interface MotionOptions {
    // 以下仅为显示恢复阈值，不是角色或怪物的玩法速度。
    readonly snapDistance?: number;
    readonly resumeGapMs?: number;
    readonly maxQueuedWaypoints?: number;
}
interface Segment {
    from: MotionPoint;
    readonly to: MotionPoint;
    readonly direction: number;
    readonly sequence: number;
    base: number;
    start: number;
    end: number;
}

const EPSILON = 1e-8;
const same = (a: MotionPoint, b: MotionPoint): boolean => Math.abs(a.x - b.x) < EPSILON && Math.abs(a.y - b.y) < EPSILON;
const directionOf = (a: MotionPoint, b: MotionPoint): number =>
    (Math.round(Math.atan2(b.y - a.y, b.x - a.x) / (Math.PI / 4)) + 10) % 8;

// 只沿已收到的权威目标插值；保留已知拐点，不向尚未收到的位置外推。
export class EntityMotion {
    private readonly snapDistance: number;
    private readonly resumeGapMs: number;
    private readonly maxQueuedWaypoints: number;
    private position: MotionPoint = { x: 0, y: 0 };
    private target: MotionPoint | null = null;
    private segments: Segment[] = [];
    private serverDirection = 0;
    private clock = 0;
    private snapshotAt: number | null = null;
    private changedAt: number | null = null;
    private counter = 0;
    private stepSequence = 0;
    private stepProgress = 0;
    private holdDeadline = 0;

    constructor(options: MotionOptions = {}) {
        // 默认只插值相邻格，跨格丢失路径时不猜测一条可能穿过障碍的直线。
        this.snapDistance = options.snapDistance ?? 1;
        this.resumeGapMs = options.resumeGapMs ?? 1500;
        this.maxQueuedWaypoints = options.maxQueuedWaypoints ?? 32;
        if (!Number.isFinite(this.snapDistance) || this.snapDistance <= 0
            || !Number.isFinite(this.resumeGapMs) || this.resumeGapMs <= 0
            || !Number.isSafeInteger(this.maxQueuedWaypoints) || this.maxQueuedWaypoints < 1) {
            throw new Error('实体插值显示阈值无效');
        }
    }

    push(value: MotionTarget, nowMs: number): MotionFrame {
        if (!Number.isFinite(value.x) || !Number.isFinite(value.y)
            || !Number.isInteger(value.direction) || value.direction < 0 || value.direction > 7
            || !Number.isFinite(value.durationMs) || value.durationMs < 0) throw new Error('实体权威目标无效');
        const now = this.time(nowMs);
        this.advance(now);
        const point = { x: value.x, y: value.y };
        const gap = this.snapshotAt === null ? 0 : now - this.snapshotAt;
        const jump = this.target ? Math.max(Math.abs(point.x - this.target.x), Math.abs(point.y - this.target.y)) : 0;
        this.serverDirection = value.direction;
        this.snapshotAt = now;
        if (!this.target || value.snap || gap > this.resumeGapMs || jump > this.snapDistance + EPSILON) {
            this.snapTo(point);
            return this.frame();
        }
        // 重复坐标只更新站立朝向和收包时间，绝不重新计时或重启步帧。
        if (same(this.target, point)) return this.frame();
        const held = this.holdDeadline > now;
        if (value.durationMs === 0 && !held) { this.snapTo(point); return this.frame(); }
        const oldTarget = this.target;
        const cadence = this.changedAt === null ? value.durationMs : now - this.changedAt;
        this.changedAt = now;
        this.target = point;
        if (this.segments.length && !held) {
            this.segments[0].from = { ...this.position };
            this.segments[0].base = this.stepProgress;
        }
        const start = held ? Math.max(this.holdDeadline, this.segments[this.segments.length - 1]?.end ?? now) : now;
        this.segments.push({ from: oldTarget, to: point, direction: directionOf(oldTarget, point),
            sequence: ++this.counter, base: 0, start, end: held ? start + value.durationMs : start });
        if (this.segments.length > this.maxQueuedWaypoints) {
            this.snapTo(point);
            return this.frame();
        }
        if (held) { this.advance(now); return this.frame(); }
        // 所有已知剩余路段在一个显示窗口内完成，快照加速时不累积永久拖尾。
        const budget = Math.min(value.durationMs, cadence > 0 ? cadence : value.durationMs);
        const weight = this.segments.reduce((sum, segment) => sum + 1 - segment.base, 0);
        let cursor = now;
        for (const segment of this.segments) {
            segment.start = cursor;
            segment.end = cursor + budget * (1 - segment.base) / weight;
            cursor = segment.end;
        }
        this.advance(now);
        return this.frame();
    }

    sample(nowMs: number): MotionFrame {
        this.advance(this.time(nowMs));
        return this.frame();
    }

    // 绝对时间门槛只延后未起步段；重复传同一截止时间不会累计等待。
    holdUntil(deadlineMs: number): void {
        if (!Number.isFinite(deadlineMs) || deadlineMs < 0) throw new Error('实体起步门槛时间无效');
        if (deadlineMs <= this.clock || deadlineMs <= this.holdDeadline) return;
        this.holdDeadline = deadlineMs;
        let cursor = this.clock;
        for (const segment of this.segments) {
            if (segment.start < this.clock || segment.base > 0) { cursor = segment.end; continue; }
            const duration = segment.end - segment.start;
            segment.start = Math.max(segment.start, deadlineMs, cursor);
            segment.end = segment.start + duration;
            cursor = segment.end;
        }
        this.advance(this.clock);
    }

    // 死亡等状态取消剩余起步等待；已开始运动不回退，未来路段保持几何及各自时长。
    releaseHold(nowMs: number): MotionFrame {
        const now = this.time(nowMs);
        this.advance(now);
        const remaining = Math.max(0, this.holdDeadline - now);
        this.holdDeadline = 0;
        if (!remaining) return this.frame();
        let cursor = now;
        for (const segment of this.segments) {
            if (segment.start < now || segment.base > 0) { cursor = segment.end; continue; }
            const duration = segment.end - segment.start;
            segment.start = Math.max(now, cursor, segment.start - remaining);
            segment.end = segment.start + duration;
            cursor = segment.end;
        }
        this.advance(now);
        return this.frame();
    }

    reset(): void {
        this.position = { x: 0, y: 0 };
        this.target = null;
        this.segments.length = 0;
        this.serverDirection = 0;
        this.clock = 0;
        this.snapshotAt = null;
        this.changedAt = null;
        this.counter = 0;
        this.stepSequence = 0;
        this.stepProgress = 0;
        this.holdDeadline = 0;
    }

    private time(value: number): number {
        if (!Number.isFinite(value) || value < 0) throw new Error('实体插值需要有效的单调毫秒时间');
        this.clock = Math.max(this.clock, value);
        return this.clock;
    }

    private snapTo(point: MotionPoint): void {
        this.position = { ...point };
        this.target = { ...point };
        this.segments.length = 0;
        this.changedAt = null;
        this.holdDeadline = 0;
        this.stepSequence = ++this.counter;
        this.stepProgress = 0;
    }

    private advance(now: number): void {
        while (this.segments.length && now >= this.segments[0].end) {
            const done = this.segments.shift()!;
            this.position = { ...done.to };
            this.stepSequence = done.sequence;
            this.stepProgress = 1;
        }
        const segment = this.segments[0];
        if (!segment) return;
        const ratio = Math.max(0, Math.min(1, (now - segment.start) / (segment.end - segment.start)));
        this.position = { x: segment.from.x + (segment.to.x - segment.from.x) * ratio,
            y: segment.from.y + (segment.to.y - segment.from.y) * ratio };
        this.stepSequence = segment.sequence;
        this.stepProgress = segment.base + (1 - segment.base) * ratio;
    }

    private frame(): MotionFrame {
        return { ...this.position, moving: this.segments.length > 0 && this.clock >= this.segments[0].start,
            direction: this.segments[0]?.direction ?? this.serverDirection,
            stepSequence: this.stepSequence, stepProgress: this.stepProgress };
    }
}
