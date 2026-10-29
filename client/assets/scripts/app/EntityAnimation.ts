export type EntityAction = 'standing' | 'walking' | 'running' | 'attack' | 'struck' | 'die' | 'casting' | 'appear';
export interface AnimationDefinition { count: number; duration_ms: number; }
export type AnimationDefinitions = Record<Exclude<EntityAction, 'casting' | 'running' | 'appear'>, AnimationDefinition> & Partial<Record<'casting' | 'running' | 'appear', AnimationDefinition>>;
export interface AnimationObservation { direction: number; action: number; attackSequence: number; castSequence?: number; dead: boolean; injured: boolean; }
export interface AnimationMotion { moving: boolean; direction: number; stepProgress: number; movementKind?: number; dashPhase?: number; }
export interface AnimationSample { action: EntityAction; direction: number; frame: number; }

// 怪物及宝宝按原帧播放一步，AI发步间隔只决定下一次目标何时到达。
// 玩家仍沿当前权威步进节奏显示；无外观定义的实体保留既有恢复方式。
export function movementDisplayDurationMs(entityKind: number, walking: AnimationDefinition | undefined, moveIntervalMs: number): number {
    return (entityKind === 2 || entityKind === 3) && walking ? walking.count * walking.duration_ms : moveIntervalMs;
}

// 连续显示时钟独立于快照：重复状态、朝向变化和服务端短暂standing不截断动作。
export class EntityAnimation {
    private observed = false;
    private attackSequence = 0;
    private castSequence = 0;
    private pendingCast = false;
    private castDirection = 4;
    private dead = false;
    private direction = 4;
    private pendingAttack = false;
    private pendingAttackDirection = 4;
    private playingAttackSequence = 0;
    private playingCastSequence = 0;
    private displacedAttackSequence = -1;
    private displacedCastSequence = -1;
    private pendingHurt = false;
    private action: EntityAction = 'standing';
    private started = 0;
    private actionDirection = 4;
    private initialCorpse = false;

    constructor(private readonly definitions: AnimationDefinitions) {}

    // 仅已验证的真实新生事件能调用；baseline首次观察、换模板与健康重入均不会推断出生。
    appear(now: number, elapsedMs: number, direction: number): boolean {
        const definition = this.definitions.appear;
        if (this.dead || !definition || !Number.isFinite(now) || !Number.isFinite(elapsedMs) || elapsedMs < 0
            || elapsedMs >= definition.count * definition.duration_ms || direction !== 5) return false;
        this.begin('appear', now - elapsedMs, direction); return true;
    }
    cancelAppearance(): void { if (this.action === 'appear') this.action = 'standing'; }

    acceptAttack(sequence: number, direction: number): void {
        if (this.dead || sequence <= this.displacedAttackSequence || sequence <= this.attackSequence) return;
        this.attackSequence = sequence; this.pendingAttack = true; this.pendingAttackDirection = direction;
    }
    get currentAttackSequence(): number { return this.playingAttackSequence; }

    // 只丢弃位移水位以内的旧身体动作，新于水位的动作仍在落脚后播放。
    displace(attackSequence: number, castSequence: number): void {
        this.displacedAttackSequence = Math.max(this.displacedAttackSequence, attackSequence);
        this.displacedCastSequence = Math.max(this.displacedCastSequence, castSequence);
        this.pendingAttack = (this.pendingAttack && this.attackSequence > this.displacedAttackSequence)
            || (this.action === 'attack' && this.playingAttackSequence > this.displacedAttackSequence);
        this.pendingCast = (this.pendingCast && this.castSequence > this.displacedCastSequence)
            || (this.action === 'casting' && this.playingCastSequence > this.displacedCastSequence);
        this.attackSequence = Math.max(this.attackSequence, attackSequence);
        this.castSequence = Math.max(this.castSequence, castSequence);
        this.pendingHurt = false; this.action = 'standing'; this.started = 0;
    }

    get playbackUntil(): number {
        const definition = this.definitions[this.action];
        return definition && (this.action === 'attack' || this.action === 'struck' || this.action === 'casting')
            ? this.started + definition.count * definition.duration_ms : 0;
    }

    observe(value: AnimationObservation, now: number): void {
        this.direction = value.direction;
        if (!this.observed) {
            this.observed = true;
            this.attackSequence = value.attackSequence;
            this.castSequence = value.castSequence ?? 0;
            this.dead = value.dead;
            this.initialCorpse = value.dead;
            this.pendingAttack = !value.dead && value.action === 2 && value.attackSequence > this.displacedAttackSequence;
            this.pendingAttackDirection = value.direction;
            this.pendingCast = !value.dead && value.action === 5 && !!this.definitions.casting && (value.castSequence ?? 0) > this.displacedCastSequence;
            this.castDirection = value.direction;
            this.started = now;
            return;
        }
        if (this.dead && !value.dead) {
            this.action = 'standing'; this.started = now;
            this.pendingAttack = false; this.pendingHurt = false; this.pendingCast = false; this.initialCorpse = false;
        }
        if (!value.dead && value.attackSequence > this.attackSequence && value.attackSequence > this.displacedAttackSequence) {
            this.pendingAttack = true; this.pendingAttackDirection = value.direction;
        }
        if (!value.dead && (value.castSequence ?? 0) > this.castSequence && (value.castSequence ?? 0) > this.displacedCastSequence && this.definitions.casting) {
            this.pendingCast = true; this.castDirection = value.direction; this.pendingAttack = false;
        }
        if (!value.dead && value.injured && !this.pendingAttack && !this.pendingCast && this.action !== 'attack' && this.action !== 'casting') this.pendingHurt = true;
        if (value.dead && !this.dead) { this.pendingAttack = false; this.pendingHurt = false; this.pendingCast = false; }
        this.dead = value.dead;
        this.attackSequence = Math.max(this.attackSequence, value.attackSequence);
        this.castSequence = Math.max(this.castSequence, value.castSequence ?? 0);
    }

    sample(now: number, motion: AnimationMotion): AnimationSample {
        if (motion.moving) this.cancelAppearance();
        if (motion.moving && motion.movementKind === 2) return { action: 'walking', direction: motion.direction,
            frame: Math.max(0, this.definitions.walking.count - 1 - 2 * Math.floor(motion.stepProgress * 3)) };
        if (motion.moving && motion.movementKind === 3 && this.definitions.running) return { action: 'running', direction: motion.direction,
            frame: (motion.dashPhase ?? 0) * 3 + Math.min(2, Math.floor(motion.stepProgress * 3)) };
        // 只完成已收到的落脚步，随后倒地；HP和业务终态无需等待表现结束。
        if (this.dead && motion.moving && !this.initialCorpse) return this.walking(motion);
        if (this.dead) {
            if (this.action !== 'die') this.begin('die', now);
            return this.frame('die', this.initialCorpse ? Infinity : now - this.started, this.actionDirection, false);
        }
        if (this.action === 'appear') {
            const definition = this.definitions.appear!;
            if (now - this.started < definition.count * definition.duration_ms) return this.frame('appear', now - this.started, this.actionDirection, false);
            this.action = 'standing';
        }
        if (this.action === 'attack' || this.action === 'struck' || this.action === 'casting') {
            const definition = this.definitions[this.action]!;
            if (now - this.started < definition.count * definition.duration_ms) return this.frame(this.action, now - this.started, this.actionDirection, false);
            this.action = 'standing';
        }
        // 已开始的单次动作完整播放；新的走步由显示运动模型等待playbackUntil。
        if (motion.moving) return this.walking(motion);
        if (this.pendingCast) {
            this.pendingCast = false; this.pendingAttack = false; this.pendingHurt = false;
            this.begin('casting', now, this.castDirection);
            return this.frame('casting', 0, this.actionDirection, false);
        }
        if (this.pendingAttack) {
            this.pendingAttack = false; this.pendingHurt = false; this.begin('attack', now, this.pendingAttackDirection);
            return this.frame('attack', 0, this.actionDirection, false);
        }
        if (this.pendingHurt) {
            this.pendingHurt = false; this.begin('struck', now);
            return this.frame('struck', 0, this.actionDirection, false);
        }
        return this.frame('standing', now, this.direction, true);
    }

    private walking(motion: AnimationMotion): AnimationSample {
        const definition = this.definitions.walking;
        return { action: 'walking', direction: motion.direction,
            frame: Math.min(definition.count - 1, Math.floor(Math.max(0, motion.stepProgress) * definition.count)) };
    }

    private begin(action: EntityAction, now: number, direction = this.direction): void {
        this.action = action; this.started = now; this.actionDirection = direction;
        if (action === 'attack') this.playingAttackSequence = this.attackSequence;
        if (action === 'casting') this.playingCastSequence = this.castSequence;
    }
    private frame(action: EntityAction, elapsed: number, direction: number, loop: boolean): AnimationSample {
        const definition = this.definitions[action]!;
        const index = Math.max(0, Math.floor(elapsed / definition.duration_ms));
        return { action, direction, frame: loop ? index % definition.count : Math.min(definition.count - 1, index) };
    }
}
