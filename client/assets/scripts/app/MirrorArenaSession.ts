import Long from 'long';
import { decodeMirrorCombatPayload } from './GameConnection';
import type { GameConnection, Packet } from './GameConnection';
import { protocolErrorMessage } from './GameErrors';
import { attachmentKey, travelInteger } from './MapTravelModel';
import type { AttachmentIdentity, TravelInteger } from './MapTravelModel';
import { mirrorID, MirrorArenaPhase, MirrorFrameLifetimes, readMirrorFrame, readMirrorState } from './MirrorArenaModel';
import type { MirrorArenaFrame, MirrorArenaLobby, MirrorArenaSnapshot, MirrorBattle } from './MirrorArenaModel';
import { CodeLineRecovering, CodeOperationPending, CodeServerBusy } from '../generated/protocol_ids';

const requestID = 20025, stateID = 20026, frameID = 20027;
const action = { Query: 1, Publish: 2, Challenge: 3, Ready: 4, Surrender: 5, Observe: 6 } as const;
type MirrorAction = typeof action[keyof typeof action];
type MirrorState = NonNullable<ReturnType<typeof readMirrorState>>;
type MirrorConnection = Pick<GameConnection, 'generation' | 'binding' | 'subscribe' | 'onStatus' | 'request'>;

// 只由已完整确认的普通城镇/野外附着提供；特殊挑战和切图中的原附着返回空。
export interface MirrorArenaScope { readonly identity: AttachmentIdentity; readonly ruleVersion: TravelInteger; }

interface BoundScope {
    readonly key: string; readonly role: string; readonly generation: number; readonly binding: string;
    readonly scene: string; readonly rule: string;
}
interface RequestTarget { readonly id: string; readonly challengeId: Uint8Array; readonly epoch?: string; readonly offerId?: Uint8Array; }
interface Pending {
    readonly action: MirrorAction; readonly target?: RequestTarget;
    readonly finished: Promise<void>; readonly resolve: () => void;
}
interface Accepted { readonly state: MirrorState; readonly applied: boolean; }

export class MirrorArenaRequestError extends Error {
    constructor(readonly code: number) { super(protocolErrorMessage(requestID, code)); }
}

// 镜像插件自己订阅传输。主游戏的场景、背包、导航、托管及经济上下文从未由这里写入。
export class MirrorArenaSession {
    private enabled = false;
    private scope: BoundScope | null = null;
    private role = '';
    private rule = '';
    private lobby: MirrorArenaLobby | null = null;
    private battle: MirrorBattle | null = null;
    private frame: MirrorArenaFrame | null = null;
    private original: MirrorBattle | null = null; // 同角色换票时保留的身份和高水位，不作为新票的授权。
    private lastSequence = '0';
    private readonly lifetimes = new MirrorFrameLifetimes(); // 同场换票沿用身体与出生水位。
    private proposal: RequestTarget | null = null;
    private recovering = ''; // 大厅读到永久活动记录，等待原连接执行权接回。
    private retired = new Set<string>();
    private pending: Pending | null = null;
    private errorCode = 0;
    private recoveryTimer?: ReturnType<typeof setTimeout>;
    private queried = false;
    private disposed = false;
    private readonly listeners = new Set<(snapshot: MirrorArenaSnapshot) => void>();
    private readonly stopPackets: () => void;
    private readonly stopStatus: () => void;

    constructor(private readonly connection: MirrorConnection, private readonly getScope: () => MirrorArenaScope | null,
        private readonly makeOperationId: () => Uint8Array) {
        this.stopPackets = connection.subscribe(packet => this.acceptPacket(packet));
        this.stopStatus = connection.onStatus(() => this.bind(null));
    }

    get snapshot(): MirrorArenaSnapshot {
        return { enabled: this.enabled, attached: !!this.scope, pending: !!this.pending, errorCode: this.errorCode,
            lobby: this.lobby, battle: this.battle, frame: this.frame };
    }

    subscribe(listener: (snapshot: MirrorArenaSnapshot) => void): () => void {
        this.listeners.add(listener);
        return () => this.listeners.delete(listener);
    }

    // 公开资源无镜像配置时保持关闭。启用也只表示可以查询服务端的赛季开关。
    setEnabled(enabled: boolean): void {
        if (this.disposed || this.enabled === enabled) return;
        this.enabled = enabled;
        if (!enabled) this.clearRole();
        if (!this.bind(enabled ? this.readScope() : null)) this.emit();
    }

    syncAttachment(): void {
        if (this.enabled && !this.disposed) this.bind(this.readScope());
    }

    dispose(): void {
        if (this.disposed) return;
        this.disposed = true;
        this.enabled = false;
        this.stopPackets();
        this.stopStatus();
        this.listeners.clear();
        this.clearRole();
        this.bind(null);
    }

    async query(): Promise<void> {
        this.queried = true;
        await this.request(action.Query);
    }

    async publish(): Promise<void> {
        await this.request(action.Publish);
    }

    async challenge(offerId: Uint8Array): Promise<void> {
        this.requireScope();
        if (this.pending) throw new Error('镜像竞技正在处理上一项操作');
        if (this.battle || this.original || this.recovering) throw new Error('请先查看原镜像场次');
        const id = mirrorID(offerId), offered = this.lobby?.offers.find(row => row.id === id);
        if (!this.lobby?.open || !offered) throw new Error('候选已变化，请刷新镜像对手');
        if (this.proposal && mirrorID(this.proposal.offerId) !== id) throw new Error('上一场挑战尚待确认，请核对原挑战');
        if (!this.proposal) {
            const challengeId = this.makeOperationId(), challenge = mirrorID(challengeId);
            if (!challenge || this.retired.has(challenge)) throw new Error('无法生成镜像挑战身份');
            this.proposal = { id: challenge, challengeId: new Uint8Array(challengeId), offerId: new Uint8Array(offered.offerId) };
        }
        const proposal = this.proposal;
        try {
            await this.request(action.Challenge, proposal);
        } catch (error) {
            // 传输超时和暂忙均沿原候选与挑战身份核对，明确业务拒绝才放弃原请求。
            if (this.proposal === proposal && error instanceof MirrorArenaRequestError && !this.retryable(error.code)) this.proposal = null;
            throw error;
        }
    }

    // 必须由独立场景加载器在当次视口准备完成后调用。旧附着的迟到资源回调不会发包。
    async ready(battleKey: string): Promise<void> {
        const previous = this.requireBattle();
        if (previous.key !== battleKey) throw new Error('镜像战斗附着已变化');
        if (this.pending?.action === action.Query) await this.pending.finished;
        const battle = this.requireBattle();
        if (battle.key !== battleKey) throw new Error('镜像战斗附着已变化');
        if (battle.phase !== MirrorArenaPhase.Preparing) return;
        await this.request(action.Ready, this.target(battle));
    }

    async surrender(): Promise<void> {
        const battle = this.requireBattle();
        if (battle.phase !== MirrorArenaPhase.Fighting) throw new Error('当前镜像战斗无法认输');
        await this.request(action.Surrender, this.target(battle));
    }

    async observe(): Promise<void> {
        const key = this.requireBattle().key;
        if (this.pending?.action === action.Query) await this.pending.finished;
        const battle = this.requireBattle();
        if (battle.key !== key) throw new Error('镜像战斗附着已变化');
        if (battle.phase === MirrorArenaPhase.Finished) return;
        await this.request(action.Observe, this.target(battle));
    }

    // 已确认结果只在玩家离开时丢弃本地视口，后续刷新仍可显示原战报。
    dismiss(): void {
        if (this.battle?.phase !== MirrorArenaPhase.Finished) return;
        this.retire(this.battle.challenge.id);
        this.emit();
    }

    private retire(id: string): void {
        this.retired.add(id);
        if (this.retired.size > 32) this.retired.delete(this.retired.values().next().value!);
        this.original = this.battle = this.frame = null;
        this.lastSequence = '0';
        this.lifetimes.clear();
    }

    private readScope(): BoundScope | null {
        try {
            const source = this.getScope();
            if (!source) return null;
            const { identity, ruleVersion } = source, binding = travelInteger(identity.binding, true);
            if (identity.connection !== this.connection.generation || binding !== travelInteger(this.connection.binding, true)) return null;
            const rule = travelInteger(ruleVersion, true);
            return { key: JSON.stringify([attachmentKey(identity), rule]), generation: identity.connection, binding,
                role: identity.role, scene: travelInteger(identity.scene, true), rule };
        } catch { return null; }
    }

    private bind(scope: BoundScope | null): boolean {
        if (scope?.key === this.scope?.key) return false;
        clearTimeout(this.recoveryTimer);
        if (this.pending) this.finish(this.pending);
        if (this.original?.phase === MirrorArenaPhase.Finished) this.retire(this.original.challenge.id);
        this.battle = this.frame = null;
        this.errorCode = 0;
        this.scope = scope;
        if (scope) {
            if (scope.role !== this.role || scope.rule !== this.rule) this.clearRole();
            this.role = scope.role;
            this.rule = scope.rule;
            if (this.queried || this.proposal || this.original) this.scheduleRecovery(scope, 0);
        }
        this.emit();
        return true;
    }

    private clearRole(): void {
        this.role = this.rule = this.recovering = '';
        this.lobby = this.original = this.proposal = null;
        this.retired.clear();
        this.queried = false;
        this.lastSequence = '0';
        this.lifetimes.clear();
    }

    private requireScope(): BoundScope {
        this.syncAttachment();
        if (!this.enabled || this.disposed || !this.scope) throw new Error('镜像竞技尚未在当前场景就绪');
        return this.scope;
    }

    private requireBattle(): MirrorBattle {
        this.requireScope();
        if (!this.battle) throw new Error('原镜像战斗尚未确认');
        return this.battle;
    }

    private target(battle: MirrorBattle): RequestTarget {
        return { id: battle.challenge.id, challengeId: battle.challenge.challengeId, epoch: battle.epoch };
    }

    private current(scope: BoundScope): boolean {
        return this.enabled && !this.disposed && scope === this.scope && this.readScope()?.key === scope.key;
    }

    // 健康新票完成普通入场后重新读原场。暂忙或活动记录尚未交接时只做有界查询。
    private scheduleRecovery(scope: BoundScope, attempt: number): void {
        this.recoveryTimer = setTimeout(async () => {
            if (!this.current(scope)) return;
            let retry = !!this.pending;
            if (!retry) {
                try {
                    await this.query();
                    retry = !!this.recovering && !this.battle;
                } catch (error) { retry = error instanceof MirrorArenaRequestError && this.retryable(error.code); }
            }
            if (attempt < 3 && retry && this.current(scope)) this.scheduleRecovery(scope, attempt + 1);
        }, attempt === 0 ? 0 : attempt * 350);
    }

    private retryable(code: number): boolean {
        return code === CodeLineRecovering || code === CodeOperationPending || code === CodeServerBusy;
    }

    private async request(type: MirrorAction, target?: RequestTarget): Promise<void> {
        const scope = this.requireScope();
        if (this.pending) throw new Error('镜像竞技正在处理上一项操作');
        let resolve!: () => void;
        const finished = new Promise<void>(complete => { resolve = complete; });
        const pending = { action: type, target, finished, resolve };
        this.pending = pending;
        this.errorCode = 0;
        this.emit();
        const payload = { action: type, sceneEpoch: Long.fromString(scope.scene, true),
            ...(target ? { challengeId: new Uint8Array(target.challengeId) } : {}),
            ...(target?.epoch ? { battleSceneEpoch: Long.fromString(target.epoch, true) } : {}),
            ...(target?.offerId ? { offerId: new Uint8Array(target.offerId) } : {}) };
        let result: Accepted | null | undefined;
        const accept = (value: unknown): unknown => {
            // 此回调从 WebSocket 同步执行，让随后抵达的 20027 能命中刚授权的新场。
            if (this.pending === pending && this.current(scope)) {
                // 先释放已回执的请求。缓存已就绪的监听者可立即发送原场首帧确认。
                this.finish(pending);
                result = this.applyState(value, scope, pending);
                if (!result?.applied) this.emit();
            }
            return value;
        };
        try {
            const value: unknown = await this.connection.request(requestID, payload, accept);
            if (!this.current(scope)) throw new Error('镜像竞技的原附着已变化');
            if (result === undefined) accept(value);
            if (!result?.applied) throw new Error('镜像竞技回包与原请求不一致');
            if (result.state.errorCode) throw new MirrorArenaRequestError(result.state.errorCode);
        } finally {
            if (this.pending === pending) {
                this.finish(pending);
                this.emit();
            }
        }
    }

    private finish(pending: Pending): void {
        this.pending = null;
        pending.resolve();
    }

    private acceptPacket(packet: Packet): void {
        if (!this.enabled || this.disposed || (packet.id !== stateID && packet.id !== frameID)) return;
        this.syncAttachment();
        const scope = this.scope;
        if (!scope || !this.current(scope) || packet.seq !== 0 || packet.error || packet.binding.toString() !== scope.binding) return;
        if (packet.id === stateID) {
            this.applyState(packet.value, scope);
        } else if (this.battle) {
            const frame = readMirrorFrame(packet.value, scope.scene, this.battle, this.lastSequence, decodeMirrorCombatPayload,
                this.lifetimes, performance.now());
            if (frame) {
                this.frame = frame;
                this.lastSequence = frame.sequence;
                this.emit();
            }
        }
    }

    private applyState(raw: unknown, scope: BoundScope, reply?: Pending): Accepted | null {
        const value = readMirrorState(raw, scope.scene, scope.rule);
        if (!value) return null;
        if (value.errorCode) {
            this.errorCode = value.errorCode;
            this.emit();
            return { state: value, applied: true };
        }
        const target = reply?.target;
        const authorityRead = reply?.action === action.Query || reply?.action === action.Publish;
        if (target && (!value.phase || value.challenge?.id !== target.id || target.epoch && target.epoch !== value.epoch)) return null;
        if (!value.phase) {
            if (!authorityRead) return null;
            this.applyLobby(value);
        } else {
            const challenge = value.challenge!;
            if (this.retired.has(challenge.id)) return { state: value, applied: !!authorityRead };
            const original = this.original, sameChallenge = original?.challenge.id === challenge.id;
            if (sameChallenge && (original.epoch !== value.epoch || original.mapId !== value.mapId ||
                original.challenge.startedAtUnixMs !== challenge.startedAtUnixMs || original.challenge.defenseVersion !== challenge.defenseVersion ||
                original.challenge.attackerName !== challenge.attackerName || original.challenge.defenderName !== challenge.defenderName)) return null;
            if (!sameChallenge && original && original.phase !== MirrorArenaPhase.Finished) return null;
            if (!sameChallenge && !authorityRead && this.proposal?.id !== challenge.id && this.recovering !== challenge.id) return null;
            if (sameChallenge && value.phase < original.phase) return { state: value, applied: !!reply };
            if (sameChallenge && original.phase === MirrorArenaPhase.Finished &&
                (original.challenge.outcome !== challenge.outcome || original.challenge.awardedPoints !== challenge.awardedPoints ||
                    original.challenge.finishedAtUnixMs !== challenge.finishedAtUnixMs)) return null;
            if (value.lobby.dailyLimit > 0) this.applyLobby(value);
            else this.mergeScore(value.lobby);
            if (!sameChallenge) { this.lastSequence = '0'; this.frame = null; this.lifetimes.clear(); }
            const key = JSON.stringify([scope.key, challenge.id, value.epoch]);
            this.battle = this.original = { key, challenge, epoch: value.epoch, mapId: value.mapId,
                phase: value.phase, deadlineUnixMs: value.deadlineUnixMs };
            if (this.proposal?.id === challenge.id) this.proposal = null;
            this.recovering = '';
        }
        this.errorCode = 0;
        this.emit();
        return { state: value, applied: true };
    }

    private applyLobby(value: MirrorState): void {
        const version = value.lobby.publishedDefenseVersion;
        const current = this.lobby?.seasonStartUnixMs === value.lobby.seasonStartUnixMs ? this.lobby.publishedDefenseVersion : '0';
        this.lobby = { ...value.lobby, publishedDefenseVersion: version === '0' ? current : version };
        this.recovering = value.challenge?.id ?? '';
        const original = this.original;
        const archived = value.lobby.attackHistory.find(record => record.id === original?.challenge.id || record.id === this.proposal?.id);
        if (archived && archived.id === this.proposal?.id) this.proposal = null;
        if (archived && original && archived.id === original.challenge.id && original.phase !== MirrorArenaPhase.Finished) {
            const key = JSON.stringify([this.scope!.key, archived.id, original.epoch]);
            this.battle = this.original = { ...original, key, challenge: archived, phase: MirrorArenaPhase.Finished };
        }
    }

    private mergeScore(update: MirrorArenaLobby): void {
        const lobby = this.lobby;
        if (!lobby || lobby.seasonStartUnixMs !== update.seasonStartUnixMs) return;
        this.lobby = { ...lobby, seasonPoints: update.seasonPoints, streak: update.streak };
    }

    private emit(): void {
        const snapshot = this.snapshot;
        for (const listener of Array.from(this.listeners)) listener(snapshot);
    }
}
