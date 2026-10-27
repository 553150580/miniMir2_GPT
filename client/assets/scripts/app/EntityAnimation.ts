export type EntityAction = 'standing' | 'walking' | 'attack' | 'struck' | 'die' | 'casting';
export interface AnimationDefinition { count: number; duration_ms: number; }
export type AnimationDefinitions = Record<Exclude<EntityAction, 'casting'>, AnimationDefinition> & Partial<Record<'casting', AnimationDefinition>>;
export interface AnimationObservation { direction: number; action: number; attackSequence: number; castSequence?: number; dead: boolean; injured: boolean; }
export interface AnimationMotion { moving: boolean; direction: number; stepProgress: number; }
export interface AnimationSample { action: EntityAction; direction: number; frame: number; }

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
    private pendingHurt = false;
    private action: EntityAction = 'standing';
    private started = 0;
    private actionDirection = 4;
    private initialCorpse = false;

    constructor(private readonly definitions: AnimationDefinitions) {}

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
            this.pendingAttack = !value.dead && value.action === 2;
            this.pendingAttackDirection = value.direction;
            this.pendingCast = !value.dead && value.action === 5 && !!this.definitions.casting;
            this.castDirection = value.direction;
            this.started = now;
            return;
        }
        if (this.dead && !value.dead) {
            this.action = 'standing'; this.started = now;
            this.pendingAttack = false; this.pendingHurt = false; this.pendingCast = false; this.initialCorpse = false;
        }
        if (!value.dead && value.attackSequence !== this.attackSequence) {
            this.pendingAttack = true; this.pendingAttackDirection = value.direction;
        }
        if (!value.dead && (value.castSequence ?? 0) !== this.castSequence && this.definitions.casting) {
            this.pendingCast = true; this.castDirection = value.direction; this.pendingAttack = false;
        }
        if (!value.dead && value.injured && !this.pendingAttack && !this.pendingCast && this.action !== 'attack' && this.action !== 'casting') this.pendingHurt = true;
        if (value.dead && !this.dead) { this.pendingAttack = false; this.pendingHurt = false; this.pendingCast = false; }
        this.dead = value.dead;
        this.attackSequence = value.attackSequence;
        this.castSequence = value.castSequence ?? 0;
    }

    sample(now: number, motion: AnimationMotion): AnimationSample {
        // 只完成已收到的落脚步，随后倒地；HP和业务终态无需等待表现结束。
        if (this.dead && motion.moving && !this.initialCorpse) return this.walking(motion);
        if (this.dead) {
            if (this.action !== 'die') this.begin('die', now);
            return this.frame('die', this.initialCorpse ? Infinity : now - this.started, this.actionDirection, false);
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

    private begin(action: EntityAction, now: number, direction = this.direction): void { this.action = action; this.started = now; this.actionDirection = direction; }
    private frame(action: EntityAction, elapsed: number, direction: number, loop: boolean): AnimationSample {
        const definition = this.definitions[action]!;
        const index = Math.max(0, Math.floor(elapsed / definition.duration_ms));
        return { action, direction, frame: loop ? index % definition.count : Math.min(definition.count - 1, index) };
    }
}
