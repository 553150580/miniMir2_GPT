export interface GridPoint { readonly x: number; readonly y: number; }
export interface Direction8 { readonly x: -1 | 0 | 1; readonly y: -1 | 0 | 1; }
export interface NavigationState {
    readonly target: GridPoint | null;
    readonly direction: Direction8;
}
export type NavigationReason = 'ground' | 'direction' | 'release' | 'arrived' | 'cancelled' | 'unreachable' | 'exit' | 'restore';
export interface AuthoritativeNavigation { inputSequence: number; destination?: GridPoint; status: number; revision: { toString(): string } | bigint | string; }

export function direction8(x: number, y: number, deadZone = 0.18): Direction8 {
    const threshold = Number.isFinite(deadZone) ? Math.max(0, deadZone) : 0.18;
    if (!Number.isFinite(x) || !Number.isFinite(y) || Math.hypot(x, y) <= threshold) {
        return { x: 0, y: 0 };
    }
    const angle = Math.round(Math.atan2(y, x) / (Math.PI / 4)) * Math.PI / 4;
    return { x: (Math.round(Math.cos(angle)) || 0) as Direction8['x'],
        y: (Math.round(Math.sin(angle)) || 0) as Direction8['y'] };
}

// 导航意图与地图绘制、面板、网络连接分别维护；本对象不持有节点。
export class NavigationIntent {
    private target: GridPoint | null = null;
    private direction: Direction8 = { x: 0, y: 0 };
    private groundSequence: number | null = null;
    private localIntent = false;
    private seenRevision = 0n;
    private groundLowerBound = 0n;
    private groundRevision: bigint | null = null;
    private inputBlocked = false;
    private readonly listeners = new Set<(state: NavigationState, reason: NavigationReason) => void>();

    get state(): NavigationState {
        return { target: this.target ? { ...this.target } : null, direction: { ...this.direction } };
    }

    subscribe(listener: (state: NavigationState, reason: NavigationReason) => void): () => void {
        this.listeners.add(listener);
        return () => { this.listeners.delete(listener); };
    }
    setInputBlocked(blocked: boolean): void { this.inputBlocked = blocked; }

    setGroundTarget(target: GridPoint): boolean {
        if (this.inputBlocked || !Number.isSafeInteger(target.x) || !Number.isSafeInteger(target.y)) return false;
        this.target = { ...target };
        this.groundSequence = null; this.groundRevision = null; this.groundLowerBound = this.seenRevision; this.localIntent = true;
        this.direction = { x: 0, y: 0 };
        this.publish('ground');
        return true;
    }

    setJoystick(value: Direction8): void {
        if (this.inputBlocked) return;
        const valid = (axis: number): boolean => axis === -1 || axis === 0 || axis === 1;
        if (!valid(value.x) || !valid(value.y)) return;
        if (!value.x && !value.y) { this.releaseJoystick(); return; }
        const changed = this.target !== null || this.direction.x !== value.x || this.direction.y !== value.y;
        this.target = null;
        this.groundSequence = null; this.groundRevision = null; this.localIntent = true;
        this.direction = { ...value };
        if (changed) this.publish('direction');
    }

    // 失焦、后台、尺寸变化及打开面板只释放摇杆，保留地面目标。
    releaseJoystick(): void {
        if (!this.direction.x && !this.direction.y) return;
        this.direction = { x: 0, y: 0 };
        this.publish('release');
    }

    bindGroundInput(sequence: number): void {
        if (this.target && Number.isInteger(sequence) && sequence >= 0 && sequence <= 0xffffffff) {
            this.groundSequence = sequence; this.groundLowerBound = this.seenRevision;
        }
    }

    // 强制路径即使经过目标也不等于寻路到达，只接受当前输入身份的权威结果。
    observeAuthority(value: AuthoritativeNavigation | null | undefined): boolean {
        if (!value || !Number.isInteger(value.inputSequence) || value.inputSequence < 0 || value.inputSequence > 0xffffffff) return false;
        let revision: bigint;
        try {
            if (typeof value.revision === 'number' || !/^\d+$/.test(value.revision?.toString() ?? '')) return false;
            revision = BigInt(value.revision.toString()); if (revision > (1n << 64n) - 1n || revision < this.seenRevision) return false;
        } catch { return false; }
        this.seenRevision = revision;
        const matching = this.groundSequence === value.inputSequence
            && (this.groundRevision === null ? revision > this.groundLowerBound : revision === this.groundRevision);
        if (value.status === 2 || value.status === 4 || value.status === 5) {
            if (!this.target || !matching || !value.destination
                || value.destination.x !== this.target.x || value.destination.y !== this.target.y) return false;
            this.target = null; this.groundSequence = null; this.groundRevision = null; this.localIntent = true;
            this.direction = { x: 0, y: 0 }; this.publish(value.status === 2 ? 'arrived' : value.status === 4 ? 'cancelled' : 'unreachable'); return true;
        }
        if (value.status === 3 && !this.localIntent) {
            this.target = null; this.groundSequence = null; this.groundRevision = null; this.localIntent = true; this.direction = { x: 0, y: 0 };
            this.publish('restore'); return true;
        }
        if (value.status !== 1 || !value.destination || !Number.isSafeInteger(value.destination.x) || !Number.isSafeInteger(value.destination.y)
            || (this.localIntent && !matching)) return false;
        if (this.groundSequence === value.inputSequence && this.groundRevision === revision && this.target?.x === value.destination.x && this.target?.y === value.destination.y) return false;
        this.target = { ...value.destination }; this.direction = { x: 0, y: 0 }; this.groundSequence = value.inputSequence; this.groundRevision = revision;
        this.publish('restore'); return true;
    }

    exitGame(): void {
        this.target = null;
        this.groundSequence = null; this.groundRevision = null; this.localIntent = true;
        this.direction = { x: 0, y: 0 };
        this.publish('exit');
    }

    private publish(reason: NavigationReason): void {
        for (const listener of Array.from(this.listeners)) {
            if (!this.listeners.has(listener)) continue;
            try { listener(this.state, reason); } catch { /* 隔离观察者，保留输入状态 */ }
        }
    }
}
