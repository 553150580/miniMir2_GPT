export interface GridPoint { readonly x: number; readonly y: number; }
export interface Direction8 { readonly x: -1 | 0 | 1; readonly y: -1 | 0 | 1; }
export interface NavigationState {
    readonly target: GridPoint | null;
    readonly direction: Direction8;
}
export type NavigationReason = 'ground' | 'direction' | 'release' | 'arrived' | 'exit';

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
    private readonly listeners = new Set<(state: NavigationState, reason: NavigationReason) => void>();

    get state(): NavigationState {
        return { target: this.target ? { ...this.target } : null, direction: { ...this.direction } };
    }

    subscribe(listener: (state: NavigationState, reason: NavigationReason) => void): () => void {
        this.listeners.add(listener);
        return () => { this.listeners.delete(listener); };
    }

    setGroundTarget(target: GridPoint): boolean {
        if (!Number.isSafeInteger(target.x) || !Number.isSafeInteger(target.y)) return false;
        this.target = { ...target };
        this.direction = { x: 0, y: 0 };
        this.publish('ground');
        return true;
    }

    setJoystick(value: Direction8): void {
        const valid = (axis: number): boolean => axis === -1 || axis === 0 || axis === 1;
        if (!valid(value.x) || !valid(value.y)) return;
        if (!value.x && !value.y) { this.releaseJoystick(); return; }
        const changed = this.target !== null || this.direction.x !== value.x || this.direction.y !== value.y;
        this.target = null;
        this.direction = { ...value };
        if (changed) this.publish('direction');
    }

    // 失焦、后台、尺寸变化及打开面板只释放摇杆，保留地面目标。
    releaseJoystick(): void {
        if (!this.direction.x && !this.direction.y) return;
        this.direction = { x: 0, y: 0 };
        this.publish('release');
    }

    completeGroundTarget(reached: GridPoint): boolean {
        if (!this.target || reached.x !== this.target.x || reached.y !== this.target.y) return false;
        this.target = null;
        this.publish('arrived');
        return true;
    }

    exitGame(): void {
        this.target = null;
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
