import { Button, Game, game, Label, Node } from 'cc';
import { newOperationId } from './GameConnection';
import type { GameConnection, Packet } from './GameConnection';
import { protocolErrorMessage } from './GameErrors';

interface BossContext {
    sceneEpoch: { toString(): string };
    sceneKind: number;
    ready: boolean;
    level: number;
    canClaimRewards: boolean;
    worldBoss?: { display_name: string; minimum_level: number; map_id: string; template_id: number };
}

type BossSection = 3 | 4 | 'rewards';

interface RewardItem {
    itemId: number;
    quantity: { toString(): string };
    bound: boolean;
    quality: number;
}

interface PendingChallengeReward {
    rewardId: Uint8Array;
    templateId: number;
    earnedAtUnixMs: { toString(): string };
    items: RewardItem[];
}

interface RewardPage {
    reward: PendingChallengeReward;
    items: RewardItem[];
}

interface ChallengeRewardActions {
    claim(rewardId: Uint8Array): Promise<void>;
    itemName(itemId: number): string | undefined;
    report(error: unknown): void;
}

export interface WorldBossEntryTarget {
    readonly mapId: string;
    readonly templateId: number;
    readonly occurrenceId: string;
}

export interface WorldBossEntryPreparation {
    dispatch(): void;
    release(): void;
}

interface WorldBossObservation {
    sceneEpoch: { toString(): string };
    occurrenceId: string;
    templateId: number;
    state: number;
    opensAtUnixMs: { toString(): string };
    expiresAtUnixMs: { toString(): string };
    hp: { toString(): string };
    hpMax: { toString(): string };
    ranks: { position: number; displayName: string; damage: { toString(): string } }[];
}

const personalState = ['','预约中','正在载入','挑战中','胜利','超时','已离开','战败'];
const worldState = ['','等待开放','正在载入','开放中','已击杀','已超时','已离开','战败'];
const qualifications = ['可入场','尚未开放','等级不足','已有挑战进行中','冷却中','今日次数已用完','请先返回普通挂机地图','背包已满','待领取队列已满'];
const historyOutcomes = ['','胜利','超时','中断','战败'];
const pageSize = 5;
// 心跳校时前短轮询，战斗开放期间定期读取提前击杀，失败最早下一秒重试。
const worldClockFallbackMs = 2000;
const worldActiveRefreshMs = 5000;
const worldQueryMinIntervalMs = 1000;

function unixMs(value: { toString(): string } | undefined): number | undefined {
    const ms = Number(value?.toString());
    return ms > 0 && Number.isSafeInteger(ms) && ms < 8640000000000000 - 28800000 ? ms : undefined;
}

function dateText(value: { toString(): string } | undefined): string {
    const ms = unixMs(value);
    return ms === undefined ? '—' : new Date(ms + 28800000).toISOString().slice(5, 16).replace('T', ' ');
}

// 面板只展示服务端状态，入场、退场和成绩都以当前连接的回执为准。
export class GameBoss {
    private section: BossSection = 3;
    private personal: any;
    private personalQuery?: Promise<void>;
    private personalDirty = false;
    private personalCurrent = false;
    private world?: WorldBossObservation;
    private active: any;
    private page = 0;
    private busy = false;
    private pendingPersonal = '';
    private generation: number;
    private worldScene?: string;
    private serverClock?: { unix: number; monotonic: number };
    private lastWorldQueryAt = -Infinity;
    private worldQuery?: Promise<void>;
    private worldTimer?: ReturnType<typeof setTimeout>;
    private preparingEntry?: AbortController;
    private appHidden = false;
    private disposed = false;
    private readonly detach: () => void;

    constructor(private readonly panel: Node, private readonly connection: GameConnection,
        private readonly context: () => BossContext | null, private readonly closePanel: () => void,
        private readonly prepareWorldEntry: (target: WorldBossEntryTarget, signal: AbortSignal) => Promise<WorldBossEntryPreparation>,
        private readonly rewardActions: ChallengeRewardActions) {
        this.generation = connection.generation;
        this.detach = connection.subscribe(packet => this.observe(packet));
        game.on(Game.EVENT_HIDE, this.onAppHide, this);
        game.on(Game.EVENT_SHOW, this.onAppShow, this);
    }

    dispose(): void {
        this.disposed = true;
        this.preparingEntry?.abort();
        this.cancelWorldTimer();
        this.detach();
        game.off(Game.EVENT_HIDE, this.onAppHide, this);
        game.off(Game.EVENT_SHOW, this.onAppShow, this);
    }

    // 面板栈切换时立即收回定时器。重新可见时，以最新连接和场景重新安排查询。
    visibilityChanged(): void {
        this.cancelWorldTimer();
        if (!this.panel.activeInHierarchy) this.preparingEntry?.abort();
        if (!this.disposed && this.panel.activeInHierarchy) this.render();
    }

    private onAppHide(): void { this.appHidden = true; this.cancelWorldTimer(); this.preparingEntry?.abort(); }
    private onAppShow(): void {
        this.appHidden = false;
        // 有些设备休眠时会暂停单调时钟，回前台先重新查询，后续心跳再校时。
        this.serverClock = undefined;
        this.visibilityChanged();
    }

    async open(): Promise<void> {
        const kind = this.context()?.sceneKind;
        if (kind === 3 || kind === 4) this.section = kind;
        await this.show(this.section);
    }

    async show(section: BossSection): Promise<void> {
        if (section !== 4) this.preparingEntry?.abort();
        this.section = section; this.page = 0;
        await this.reload();
    }

    turn(delta: number): void { this.page += delta; this.render(); }

    async reload(): Promise<void> {
        this.render();
        const context = this.context();
        if (!context?.ready) return;
        if (this.section !== 4) {
            if (context.sceneKind !== 4) await this.queryPersonal(context);
        } else if (context.worldBoss) await this.queryWorld(context);
    }

    // 持久终态和原领取操作恢复都要求重新读取队列，旧快照不能再触发领取。
    refreshPersonal(): void {
        if (this.disposed) return;
        this.personalDirty = true;
        this.personalCurrent = false;
        if (this.panel.activeInHierarchy) this.render();
    }

    async action(): Promise<void> {
        const context = this.context();
        if (!context?.ready || this.busy) return;
        const { sceneEpoch, sceneKind } = context;
        this.busy = true; this.render();
        try {
            if (this.section === 'rewards') {
                const reward = this.rewardPages()[this.page]?.reward;
                if (!this.canClaimReward(context, reward)) return;
                await this.rewardActions.claim(new Uint8Array(reward.rewardId));
                this.refreshPersonal();
                return;
            } else if (this.section === 3 && sceneKind === 3 && this.active?.state === 3) {
                this.connection.send(20013, { sceneEpoch, challengeId: this.active.challengeId });
            } else if (this.section === 4 && sceneKind === 4 && this.world?.state === 3) {
                this.connection.send(20019, { sceneEpoch, occurrenceId: this.world.occurrenceId });
            } else if (this.section === 3 && sceneKind === 1 && this.personal?.canEnter) {
                this.pendingPersonal ||= Array.from(newOperationId(), byte => byte.toString(16).padStart(2, '0')).join('');
                const response = await this.connection.request(20012, { sceneEpoch, challengeId: this.pendingPersonal, templateId: this.personal.templateId });
                if (response?.errorCode) return;
            } else if (this.section === 4 && this.world && context.worldBoss && this.canEnterWorld(context)) {
                if (!await this.enterWorld(context, this.world)) return;
            } else return;
            this.closePanel();
        } finally {
            this.busy = false; this.render();
        }
    }

    render(): void {
        if (this.disposed) return;
        const context = this.context();
        this.syncWorldScene(context);
        if (this.personalDirty && !this.personalQuery && this.panel.activeInHierarchy && this.section !== 4
            && context?.ready && (context.sceneKind === 1 || context.sceneKind === 2)) {
            void this.queryPersonal(context).catch(this.rewardActions.report);
        }
        const pending = this.pendingRewards;
        const showRewards = !this.personal || !!(this.personal.pendingRewardCapacity || pending.length);
        this.panel.getChildByName('Rewards')!.active = showRewards;
        if (this.section === 'rewards') {
            if (this.personal && !showRewards) { this.section = 3; this.page = 0; }
            else { this.renderRewards(context); this.cancelWorldTimer(); return; }
        }
        const local = this.section === 3;
        const value = this.personal, world = this.world;
        const inside = context?.sceneKind === this.section;
        const rows: string[] = local ? (value?.history ?? []).map((row: any) =>
            `${dateText(row.finishedAtUnixMs)}  ${historyOutcomes[row.outcome] ?? '已结算'}  伤害 ${row.damage}${row.rewardGold && row.rewardGold.toString() !== '0' ? `  金币 +${row.rewardGold}` : ''}`)
            : (world?.ranks ?? []).map(row => `${row.position}. ${row.displayName}  伤害 ${row.damage}`);
        const pages = Math.max(1, Math.ceil(rows.length / pageSize));
        this.page = Math.max(0, Math.min(this.page, pages - 1));
        if (local) {
            const state = this.active?.state;
            this.label('Headline', value?.displayName || '个人首领');
            this.label('Status', inside && state ? personalState[state] : value
                ? `${qualifications[value.eligibilityReason] ?? '资格待确认'}  今日 ${value.successes}/${value.dailyLimit}` : '等待查询');
            this.label('Details', value?.open ? `等级 ${value.minimumLevel}   ${inside ? '结束' : '可挑战时刻'} ${dateText(inside ? this.active?.expiresAtUnixMs : value.availableAtUnixMs)}` : '');
        } else {
            this.label('Headline', context?.worldBoss?.display_name || '世界首领');
            this.label('Status', context?.worldBoss ? worldState[world?.state ?? 0] || '等待查询' : '尚未开放');
            this.label('Details', world ? `上海时间 ${dateText(world.opensAtUnixMs)} 开场 / ${dateText(world.expiresAtUnixMs)} 结束\n首领生命 ${world.hp}/${world.hpMax}` : '');
        }
        this.label('Results', rows.slice(this.page * pageSize, (this.page + 1) * pageSize).join('\n') || (local ? '暂无挑战记录' : '暂无伤害排行'));
        this.label('Page', `${this.page + 1}/${pages}`);
        this.label('Action/Label', inside ? '离开挑战' : '进入挑战');
        this.enable('Action', !!context?.ready && !this.busy && (inside ? (local ? this.active?.state : world?.state) === 3
            : local ? context.sceneKind === 1 && value?.canEnter === true && !(this.active?.state > 0 && this.active.state < 4) : this.canEnterWorld(context)));
        this.enable('Previous', this.page > 0);
        this.enable('Next', this.page + 1 < pages);
        this.scheduleWorldRefresh(context);
    }

    private get pendingRewards(): PendingChallengeReward[] { return this.personal?.pendingRewards ?? []; }

    // 一个可领取奖包的附件超过原保存结果区时继续翻页，每页仍领取整个奖包。
    private rewardPages(): RewardPage[] {
        return this.pendingRewards.flatMap(reward => {
            const items = reward.items ?? [];
            return Array.from({ length: Math.max(1, Math.ceil(items.length / pageSize)) }, (_, page) =>
                ({ reward, items: items.slice(page * pageSize, (page + 1) * pageSize) }));
        });
    }

    private canClaimReward(context: BossContext | null, reward: PendingChallengeReward | undefined): reward is PendingChallengeReward {
        return !!context?.ready && context.canClaimRewards && (context.sceneKind === 1 || context.sceneKind === 2)
            && !(this.active?.state > 0 && this.active.state < 4) && this.personalCurrent && !this.personalQuery
            && reward?.rewardId instanceof Uint8Array && reward.rewardId.length === 16;
    }

    private renderRewards(context: BossContext | null): void {
        const pages = this.rewardPages();
        this.page = Math.max(0, Math.min(this.page, pages.length - 1));
        const page = pages[this.page], capacity = this.personal?.pendingRewardCapacity;
        this.label('Headline', '挑战奖励');
        this.label('Status', this.personal ? `待领取 ${this.pendingRewards.length}${capacity ? '/' + capacity : ''}` : '等待查询');
        if (page) {
            const source = this.personal?.templateId === page.reward.templateId && this.personal.displayName
                ? this.personal.displayName : `首领 ${page.reward.templateId}`;
            this.label('Details', `${source} · ${dateText(page.reward.earnedAtUnixMs)}\n${page.reward.items?.length ?? 0} 项物品`);
        } else this.label('Details', context && context.sceneKind >= 3 ? '请返回原场景后领取' : '');
        const lines = page?.items.map(item => `${this.rewardActions.itemName(item.itemId) ?? `物品 ${item.itemId}`} ×${item.quantity}  ${item.bound ? '绑定' : '非绑定'}`
            + (item.quality ? `  品质 ${item.quality}` : ''));
        this.label('Results', lines?.join('\n') || (this.personal ? '暂无待领取奖励' : '尚未同步待领取奖励'));
        this.label('Page', `${this.page + 1}/${Math.max(1, pages.length)}`);
        this.label('Action/Label', '领取');
        this.enable('Action', !this.busy && this.canClaimReward(context, page?.reward));
        this.enable('Previous', this.page > 0);
        this.enable('Next', this.page + 1 < pages.length);
    }

    private canEnterWorld(context: BossContext): boolean {
        if (context.sceneKind !== 1 || !context.worldBoss || context.level < context.worldBoss.minimum_level || this.world?.state !== 3) return false;
        const deadline = this.worldBoundary();
        return deadline === undefined || performance.now() < deadline;
    }

    // 首领地图在服务器保留席位前完成准备，从准备到派送均须保持原场次与角色附着。
    private async enterWorld(context: BossContext, offered: WorldBossObservation): Promise<boolean> {
        const offer = context.worldBoss!;
        if (offered.templateId !== offer.template_id) throw new Error('世界首领场次与当前模板不一致');
        const target = { mapId: offer.map_id, templateId: offered.templateId, occurrenceId: offered.occurrenceId };
        const generation = this.connection.generation;
        const attempt = new AbortController();
        this.preparingEntry = attempt;
        let prepared: WorldBossEntryPreparation | undefined;
        let accepted = false;
        const current = (): BossContext | null => {
            const next = this.context();
            return !attempt.signal.aborted && generation === this.connection.generation && this.canRefreshWorld(next)
                && next.sceneKind === 1 && next.sceneEpoch.toString() === context.sceneEpoch.toString()
                && next.worldBoss?.map_id === target.mapId && next.worldBoss.template_id === target.templateId ? next : null;
        };
        try {
            prepared = await this.prepareWorldEntry(target, attempt.signal);
            const next = current();
            if (!next) return false;
            // 准备资源可能跨过场次边界，以再次查询到的场次决定是否发入场请求。
            await this.queryWorld(next);
            const latest = current();
            if (!latest) return false;
            if (this.world?.occurrenceId !== target.occurrenceId || this.world.templateId !== target.templateId || !this.canEnterWorld(latest))
                throw new Error('世界首领场次已变化，请刷新后重试');
            prepared.dispatch();
            this.preparingEntry = undefined;
            await this.connection.request(20018, { sceneEpoch: latest.sceneEpoch, templateId: target.templateId, occurrenceId: target.occurrenceId });
            accepted = true;
            return true;
        } catch (error) {
            if (attempt.signal.aborted) return false;
            throw error;
        } finally {
            if (this.preparingEntry === attempt) this.preparingEntry = undefined;
            if (!accepted) prepared?.release();
        }
    }

    // 不沿用换线、换场景或重连前的场次快照及查询回调。
    private syncWorldScene(context: BossContext | null): void {
        if (this.generation !== this.connection.generation) {
            this.personal = this.active = null;
            this.personalQuery = undefined;
            this.personalDirty = true;
            this.personalCurrent = false;
            this.pendingPersonal = '';
            this.generation = this.connection.generation;
            this.serverClock = undefined;
            this.worldScene = undefined;
        }
        const scene = context?.sceneEpoch.toString() ?? '';
        if (this.worldScene === scene) return;
        this.worldScene = scene;
        if (this.personalQuery) this.personalDirty = true;
        this.personalQuery = undefined;
        this.world = undefined;
        this.worldQuery = undefined;
        this.lastWorldQueryAt = -Infinity;
        this.cancelWorldTimer();
    }

    private queryPersonal(context: BossContext): Promise<void> {
        if (this.personalQuery) return this.personalQuery;
        this.personalDirty = false;
        this.personalCurrent = false;
        const generation = this.generation, scene = context.sceneEpoch.toString();
        const query = this.connection.request(20015, {}).then(value => {
            if (this.disposed || this.connection.generation !== generation || this.context()?.sceneEpoch.toString() !== scene) return;
            if (value.errorCode) throw new Error(protocolErrorMessage(20015, value.errorCode));
            this.personal = value;
            this.personalCurrent = !this.personalDirty;
        }).finally(() => {
            if (this.personalQuery !== query) return;
            this.personalQuery = undefined;
            this.render();
        });
        this.personalQuery = query;
        return query;
    }

    private queryWorld(context: BossContext): Promise<void> {
        if (this.worldQuery) return this.worldQuery;
        this.cancelWorldTimer();
        this.lastWorldQueryAt = performance.now();
        const generation = this.generation, scene = this.worldScene;
        const query = this.connection.request(20017, { sceneEpoch: context.sceneEpoch }).then((value: WorldBossObservation) => {
            if (!this.disposed && this.connection.generation === generation
                && this.context()?.sceneEpoch.toString() === scene && value.sceneEpoch.toString() === scene) this.world = value;
        }).finally(() => {
            if (this.worldQuery !== query) return;
            this.worldQuery = undefined;
            this.render();
        });
        this.worldQuery = query;
        return query;
    }

    // 将服务端秒表和场次时间换算到浏览器单调时钟，电脑时区或手调时间不改判定。
    private worldBoundary(): number | undefined {
        const clock = this.serverClock;
        const boundary = unixMs(this.world?.state === 1 ? this.world.opensAtUnixMs : this.world?.expiresAtUnixMs);
        if (clock && boundary !== undefined) return clock.monotonic + boundary - clock.unix;
    }

    private cancelWorldTimer(): void {
        clearTimeout(this.worldTimer);
        this.worldTimer = undefined;
    }

    private canRefreshWorld(context: BossContext | null): context is BossContext {
        return !this.disposed && !this.appHidden && (typeof document === 'undefined' || !document.hidden)
            && this.panel.activeInHierarchy && this.section === 4 && !!context?.ready && !!context.worldBoss
            && (context.sceneKind === 1 || context.sceneKind === 2);
    }

    private scheduleWorldRefresh(context: BossContext | null): void {
        if (!this.canRefreshWorld(context)) { this.cancelWorldTimer(); return; }
        if (this.worldTimer !== undefined || this.worldQuery) return;
        const boundary = this.worldBoundary();
        const waiting = this.world?.state === 1 || this.world?.state === 4;
        const polling = this.lastWorldQueryAt + (this.serverClock ? worldActiveRefreshMs : worldClockFallbackMs);
        const next = boundary === undefined ? polling : waiting ? boundary : Math.min(boundary, polling);
        const due = Math.max(this.lastWorldQueryAt + worldQueryMinIntervalMs, next);
        this.worldTimer = setTimeout(() => {
            this.worldTimer = undefined;
            const current = this.context();
            this.syncWorldScene(current);
            if (this.canRefreshWorld(current)) {
                // 入场时仍由服务端验证，访问窗口结束时先关闭旧场次按钮。
                this.enable('Action', !this.busy && this.canEnterWorld(current));
                void this.queryWorld(current).catch(() => { /* 暂时失败保留上次回执，定时器按下限重试。 */ });
            }
        }, Math.max(0, due - performance.now()));
    }

    private observe(packet: Packet): void {
        const { id, value } = packet;
        if (packet.error || this.disposed || (id !== 2 && id !== 20014 && id !== 20020)) return;
        const context = this.context();
        this.syncWorldScene(context);
        if (id === 2) {
            const sample = unixMs(value.serverUnixMs);
            if (sample !== undefined) {
                this.serverClock = { unix: sample, monotonic: performance.now() };
                this.cancelWorldTimer();
                this.scheduleWorldRefresh(context);
            }
            return;
        }
        if (id === 20014) {
            this.active = value;
            if (value.state >= 4) {
                this.pendingPersonal = '';
                if (value.errorCode) this.rewardActions.report(new Error(protocolErrorMessage(20012, value.errorCode)));
                this.personalDirty = true;
                this.personalCurrent = false;
            }
        } else if (id === 20020 && packet.seq === 0 && value.sceneEpoch.toString() === this.worldScene) {
            this.world = value;
            this.cancelWorldTimer();
        }
        else return;
        this.render();
    }

    private label(name: string, text: string): void { this.panel.getChildByPath(name)!.getComponent(Label)!.string = text; }
    private enable(name: string, enabled: boolean): void { this.panel.getChildByName(name)!.getComponent(Button)!.interactable = enabled; }
}
