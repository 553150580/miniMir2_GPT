// 旅行计划与客户端附着只保存内存身份；不替代服务端移交，也不修改永久经济Pending。
export type TravelInteger = string | bigint | { toString(): string };
export interface TravelContext {
    readonly connection: number; readonly binding: TravelInteger; readonly role: string;
    readonly scene: TravelInteger; readonly baseline: TravelInteger; readonly life: TravelInteger; readonly rule: TravelInteger;
}
export interface TravelPlan {
    readonly travelId: string; readonly fromSceneEpoch: TravelInteger; readonly targetMapId: TravelInteger;
    readonly targetSceneKind: number; readonly position: { readonly x: number; readonly y: number }; readonly expiresAtUnixMs: TravelInteger;
}
export interface TravelState {
    readonly travelId: string; readonly fromSceneEpoch: TravelInteger; readonly sceneEpoch: TravelInteger;
    readonly phase: number; readonly targetMapId: TravelInteger;
}
export interface AttachmentIdentity {
    readonly connection: number; readonly binding: TravelInteger; readonly role: string;
    readonly scene: TravelInteger; readonly baseline: TravelInteger; readonly map: TravelInteger; readonly travelId: string;
    readonly serverTransitionId?: string;
}
export type TravelPhase = 'idle' | 'planning' | 'preparing' | 'confirming' | 'commit_sent' | 'quiescing' | 'committed' | 'attaching' | 'awaiting_ready' | 'complete' | 'cancelling' | 'cancelled' | 'expired' | 'uncertain';
export function travelInteger(value: TravelInteger, positive = false): string {
    if (typeof value === 'number' || value == null) throw new Error('旅行64位身份不能使用Number');
    const text = value.toString();
    if (!/^(0|[1-9]\d*)$/.test(text) || BigInt(text) > 18446744073709551615n || (positive && text === '0')) throw new Error('旅行64位身份无效');
    return text;
}
export function attachmentKey(value: AttachmentIdentity): string {
    if (!Number.isSafeInteger(value.connection) || value.connection < 0 || !value.role) throw new Error('地图附着连接身份无效');
    return JSON.stringify([value.connection, travelInteger(value.binding, true), value.role, travelInteger(value.scene, true), travelInteger(value.baseline, true), travelInteger(value.map, true), value.travelId, value.serverTransitionId ?? '']);
}
function context(value: TravelContext): TravelContext {
    if (!Number.isSafeInteger(value.connection) || value.connection < 0 || !value.role) throw new Error('旅行连接身份无效');
    return Object.freeze({ ...value, binding: travelInteger(value.binding, true), scene: travelInteger(value.scene, true), baseline: travelInteger(value.baseline, true), life: travelInteger(value.life), rule: travelInteger(value.rule, true) });
}
export class MapTravelModel {
    phase: TravelPhase = 'idle';
    private source?: TravelContext;
    private sequence = 0;
    private identifier = '';
    private destination = 0;
    private preparedPlan?: TravelPlan;
    private committedScene?: string;
    private attached?: string;
    private commitDispatched = false;
    get generation(): number { return this.sequence; }
    get id(): string { return this.identifier; }
    get plan(): TravelPlan | undefined { return this.preparedPlan; }
    get active(): boolean { return !['idle', 'complete', 'cancelled', 'expired'].includes(this.phase); }
    get blocksInput(): boolean { return this.commitDispatched && this.active; }
    begin(value: TravelContext, id: string, destination: number): number {
        if (this.active || !/^[a-f0-9]{32}$/.test(id) || ![1, 2].includes(destination)) throw new Error('旅行请求正在处理中或身份无效');
        this.source = context(value); this.identifier = id; this.destination = destination; this.preparedPlan = undefined; this.committedScene = undefined; this.attached = undefined;
        this.commitDispatched = false;
        this.phase = 'planning'; return ++this.sequence;
    }
    matches(value: TravelContext): boolean {
        if (!this.source) return false;
        try { const candidate = context(value); return ['connection', 'binding', 'role', 'scene', 'baseline', 'life', 'rule'].every(key => (candidate as any)[key] === (this.source as any)[key]); } catch { return false; }
    }
    sameConnection(value: Pick<TravelContext, 'connection' | 'binding' | 'role'>): boolean {
        return !!this.source && value.connection === this.source.connection && value.role === this.source.role && travelInteger(value.binding) === this.source.binding;
    }
    acceptPlan(plan: TravelPlan, current: TravelContext, nowMs: number): void {
        if (this.phase !== 'planning' || !this.matches(current) || plan.travelId !== this.identifier || travelInteger(plan.fromSceneEpoch) !== this.source!.scene
            || plan.targetSceneKind !== (this.destination === 1 ? 2 : 1) || !plan.position || ![plan.position.x, plan.position.y].every(n => Number.isSafeInteger(n) && n >= 0)) throw new Error('旅行计划与原请求身份不匹配');
        const expires = travelInteger(plan.expiresAtUnixMs, true);
        if (BigInt(expires) > 9223372036854775807n || !Number.isSafeInteger(nowMs) || nowMs < 0 || BigInt(expires) <= BigInt(nowMs)) throw new Error('旅行计划已经过期');
        this.preparedPlan = Object.freeze({ ...plan, fromSceneEpoch: travelInteger(plan.fromSceneEpoch), targetMapId: travelInteger(plan.targetMapId, true), expiresAtUnixMs: expires, position: Object.freeze({ ...plan.position }) });
        this.phase = 'preparing';
    }
    prepared(current: TravelContext, nowMs: number): void { this.checkPlan(current, nowMs); if (this.phase !== 'preparing') throw new Error('旅行资源准备代次已变化'); this.phase = 'confirming'; }
    commit(current: TravelContext, nowMs: number): void { this.checkPlan(current, nowMs); if (this.phase !== 'confirming') throw new Error('旅行确认状态已变化'); this.commitDispatched = true; this.phase = 'commit_sent'; }
    cancelling(): void { if (this.committedScene || !this.active) throw new Error('旅行已提交，不能本地回滚'); this.phase = 'cancelling'; }
    uncertain(): void { if (this.active && !this.committedScene) this.phase = 'uncertain'; }
    acceptState(value: TravelState, current: Pick<TravelContext, 'connection' | 'binding' | 'role'>): boolean {
        if (!this.source || !this.sameConnection(current) || value.travelId !== this.identifier || travelInteger(value.fromSceneEpoch) !== this.source.scene || ![1, 2, 3, 4].includes(value.phase)) return false;
        if (this.preparedPlan && travelInteger(value.targetMapId) !== this.preparedPlan.targetMapId) return false;
        if (this.committedScene) return value.phase === 2 && travelInteger(value.sceneEpoch) === this.committedScene;
        if (['cancelled', 'expired', 'complete'].includes(this.phase)) return false;
        if (value.phase === 1) {
            if (!this.commitDispatched || travelInteger(value.sceneEpoch) !== this.source.scene || !['commit_sent', 'quiescing', 'uncertain'].includes(this.phase)) return false;
            this.phase = 'quiescing'; return true;
        }
        if (value.phase === 2) {
            if (!this.commitDispatched || !['commit_sent', 'quiescing', 'uncertain'].includes(this.phase) || !this.preparedPlan) return false;
            const scene = travelInteger(value.sceneEpoch, true); if (BigInt(scene) !== BigInt(this.source.scene.toString()) + 1n) return false;
            this.committedScene = scene; this.phase = 'committed'; return true;
        }
        if (travelInteger(value.sceneEpoch) !== this.source.scene) return false;
        this.phase = value.phase === 3 ? 'cancelled' : 'expired'; return true;
    }
    acceptsEntry(value: AttachmentIdentity): boolean {
        return !value.serverTransitionId && !!this.committedScene && !!this.preparedPlan && this.sameConnection(value) && value.travelId === this.identifier
            && travelInteger(value.scene) === this.committedScene && travelInteger(value.map) === this.preparedPlan.targetMapId;
    }
    attach(value: AttachmentIdentity): void { if (!this.acceptsEntry(value)) throw new Error('新地图附着不属于已提交旅行'); this.attached = attachmentKey(value); this.phase = 'attaching'; }
    acknowledge(value: AttachmentIdentity): boolean {
        if (this.phase !== 'attaching' || this.attached !== attachmentKey(value)) return false;
        this.phase = 'awaiting_ready'; return true;
    }
    ready(value: AttachmentIdentity): boolean { if (this.phase !== 'awaiting_ready' || this.attached !== attachmentKey(value)) return false; this.phase = 'complete'; return true; }
    reset(): void { this.sequence++; this.phase = 'idle'; this.source = undefined; this.identifier = ''; this.preparedPlan = undefined; this.committedScene = undefined; this.attached = undefined; this.commitDispatched = false; }
    private checkPlan(current: TravelContext, nowMs: number): void {
        if (!this.preparedPlan || !this.matches(current) || !Number.isSafeInteger(nowMs) || nowMs < 0 || BigInt(this.preparedPlan.expiresAtUnixMs.toString()) <= BigInt(nowMs)) throw new Error('旅行原连接、角色状态或计划期限已变化');
    }
}
