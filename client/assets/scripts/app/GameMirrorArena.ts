import { Button, isValid, Label, Node } from 'cc';
import { List } from '../ui/List';
import type { PopupView } from '../ui/PopupView';
import type { UIMgr } from '../ui/UIMgr';
import { GameConnection, newOperationId } from './GameConnection';
import { protocolErrorMessage } from './GameErrors';
import { attachmentKey, travelInteger } from './MapTravelModel';
import type { AttachmentIdentity, TravelInteger } from './MapTravelModel';
import { MirrorArenaPresentation } from './MirrorArenaPresentation';
import { MirrorArenaSession } from './MirrorArenaSession';

type Snapshot = MirrorArenaSession['snapshot'];
type Battle = NonNullable<Snapshot['battle']>;
type Lobby = NonNullable<Snapshot['lobby']>;
type Offer = Lobby['offers'][number];
type Record = Lobby['attackHistory'][number];
type Row = Offer | Record;
const tabs = ['Offers', 'AttackHistory', 'DefenseHistory'] as const;
type Tab = typeof tabs[number];
const jobs = ['战士', '法师', '道士'] as const;

// 这里只传普通附着的身份和展示资料，镜像帧不写入普通场景。
export interface MirrorArenaHostContext {
    readonly identity: AttachmentIdentity;
    readonly ruleVersion: TravelInteger;
    readonly name: string;
    readonly job: number;
}

interface MirrorArenaActions {
    readonly context: () => MirrorArenaHostContext | null;
    readonly releaseInput: () => void;
    readonly sound: () => void;
}

const seasonFormat = new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: 'numeric', day: 'numeric' });

function seasonDay(value: string): string {
    const time = new Date(Number(value));
    return time.valueOf() > 0 ? seasonFormat.format(time) : '';
}

function outcome(record: Record, defense = false): string {
    if (record.outcome === 3) return '系统中断';
    if (record.outcome === 0) return '尚未结算';
    return (record.outcome === 1) !== defense ? '胜利' : '失败';
}

// 管理编辑器保存的大厅和独立观战层，资源预备完成前不提交开战确认。
export class GameMirrorArena {
    private readonly list: List;
    private readonly transport: MirrorArenaSession;
    private readonly presentation: MirrorArenaPresentation;
    private readonly removeSubscription: () => void;
    private readonly listeners: Array<() => void> = [];
    private tab: Tab = 'Offers';
    private selection = '';
    private paintedRows?: readonly Row[];
    private paintedSelection = '';
    private mapId = '';
    private notice = '';
    private watchingRole = '';
    private loading?: symbol;
    private mounting = '';
    private prepared = '';
    private appliedSequence = '';
    private localError = '';
    private timeText = '';
    private disposed = false;

    constructor(private readonly entryButton: Node, private readonly panel: Node, private readonly overlay: Node,
        private readonly ui: UIMgr, private readonly popup: PopupView, connection: GameConnection, private readonly actions: MirrorArenaActions) {
        this.transport = new MirrorArenaSession(connection, actions.context, newOperationId);
        this.presentation = new MirrorArenaPresentation(overlay);
        const list = this.saved(panel, 'List').getComponent(List);
        if (!list) throw new Error('镜像竞技缺少保存的列表组件');
        this.list = list;
        this.removeSubscription = this.transport.subscribe(state => this.receive(state));
        this.bind(entryButton, () => this.open());
        this.bind(this.saved(panel, 'Refresh'), () => this.request(() => this.transport.query()));
        this.bind(this.saved(panel, 'Publish'), () => this.publish());
        this.bind(this.saved(panel, 'Resume'), () => this.resume());
        this.bind(this.saved(panel, 'Challenge'), () => this.challenge());
        this.bind(this.saved(overlay, 'Footer/Surrender'), () => this.surrender());
        this.bind(this.saved(overlay, 'Footer/Return'), () => this.returnToMap());
        for (const tab of tabs) this.bind(this.saved(panel, `SectionTabs/${tab}`), () => {
            this.tab = tab; this.selection = ''; this.paintedRows = undefined; this.renderLobby(this.transport.snapshot);
        });
        const choose = (_index: number, row: Row): void => {
            if (!this.rows(this.transport.snapshot).includes(row)) return;
            this.actions.sound();
            this.selection = row.id; this.renderLobby(this.transport.snapshot);
        };
        this.list.node.on(List.EVENT_ITEM_CLICK, choose, this);
        this.listeners.push(() => { if (isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, choose, this); });
    }

    get visible(): boolean { return this.overlay.activeInHierarchy; }

    // 原规则没有候选装配时入口保持隐藏。切图、断线和新票都以原场附着重新核验。
    refresh(config?: { enabled?: boolean; map_id?: TravelInteger }): void {
        if (this.disposed) return;
        let mapId = '';
        if (config?.enabled && config.map_id !== undefined) {
            try { mapId = travelInteger(config.map_id, true); } catch { /* 配置损坏时保留关闭入口。 */ }
        }
        const context = this.actions.context();
        if (context && this.watchingRole && this.watchingRole !== context.identity.role) this.watchingRole = '';
        if (mapId !== this.mapId) {
            this.mapId = mapId;
            this.entryButton.active = !!mapId;
            this.transport.setEnabled(!!mapId);
            if (!mapId) { this.watchingRole = ''; if (this.ui.top === this.panel) this.ui.back(); }
        }
        this.enabled(this.entryButton, !!mapId && !!context);
        this.transport.syncAttachment();
    }

    update(dt: number): void {
        if (!this.visible) return;
        this.presentation.update(dt);
        const battle = this.transport.snapshot.battle;
        const left = battle?.phase === 2 ? Math.min(600, Math.max(0, Math.ceil((Number(battle.deadlineUnixMs) - Date.now()) / 1000))) : 0;
        const text = battle?.phase === 2 ? `剩余 ${left} 秒` : '';
        if (text !== this.timeText) { this.timeText = text; this.label(this.overlay, 'Header/Time', text); }
    }

    dispose(): void {
        this.disposed = true;
        this.removeSubscription();
        this.transport.dispose(); this.presentation.dispose();
        for (const remove of this.listeners) remove();
        this.listeners.length = 0;
    }

    private open(): void {
        if (!this.mapId || !this.actions.context()) return;
        this.ui.open('MirrorArena');
        this.notice = '';
        this.renderLobby(this.transport.snapshot);
        void this.request(() => this.transport.query());
    }

    private async publish(): Promise<void> {
        const context = this.actions.context();
        if (!context || !this.transport.snapshot.lobby?.open || this.transport.snapshot.pending) return;
        const identity = attachmentKey(context.identity);
        if (!await this.popup.confirm('Confirm', '将当前装备、技能与职业宠物发布为本赛季防守快照。已发给对手的原版本仍有效。', '发布防守快照')) return;
        if (!this.samePanel(identity)) return;
        await this.request(() => this.transport.publish(), '已发布防守快照');
    }

    private async challenge(): Promise<void> {
        const context = this.actions.context(), offer = this.selectedOffer(this.transport.snapshot);
        if (!context || !offer || !this.canChallenge(this.transport.snapshot)) return;
        const identity = attachmentKey(context.identity), map = this.mapId;
        if (!await this.popup.confirm('Confirm', `挑战 ${offer.defenderName} 的防守快照？战斗由双方自动进行，开始后计入本日次数。`, '发起镜像挑战')) return;
        if (!this.samePanel(identity) || this.selectedOffer(this.transport.snapshot)?.id !== offer.id) return;
        const loading = Symbol();
        this.loading = loading;
        this.notice = '正在准备战斗资源';
        this.renderLobby(this.transport.snapshot);
        const current = (): boolean => this.loading === loading && this.mapId === map && this.samePanel(identity);
        try {
            const prepared = await this.presentation.preload(map, context.ruleVersion, context.job, current,
                (done, total) => { if (current()) this.label(this.panel, 'Status', `准备战斗资源 ${done}/${total}`); });
            if (!prepared || !current() || this.selectedOffer(this.transport.snapshot)?.id !== offer.id) return;
            this.watchingRole = context.identity.role;
            if (this.transport.snapshot.battle?.phase === 4) this.transport.dismiss();
            await this.request(() => this.transport.challenge(offer.offerId));
        } catch (error) { if (current()) this.notice = this.message(error); }
        finally {
            if (this.loading === loading) { this.loading = undefined; this.renderLobby(this.transport.snapshot); }
        }
    }

    private resume(): void {
        const context = this.actions.context();
        if (!context || !this.transport.snapshot.battle) return;
        this.watchingRole = context.identity.role;
        this.localError = '';
        this.receive(this.transport.snapshot);
    }

    private async surrender(): Promise<void> {
        const battle = this.transport.snapshot.battle;
        if (!battle || battle.phase !== 2 || this.transport.snapshot.pending) return;
        if (!await this.popup.confirm('Confirm', '认输按本场失败结算，消耗本日挑战次数。确定认输？', '镜像竞技')) return;
        if (this.visible && this.transport.snapshot.battle?.key === battle.key && this.transport.snapshot.battle.phase === 2)
            await this.request(() => this.transport.surrender());
    }

    private returnToMap(): void {
        this.watchingRole = '';
        this.hideBattle();
        if (this.transport.snapshot.battle?.phase === 4) this.transport.dismiss();
        this.ui.closeAll();
    }

    private receive(state: Snapshot): void {
        if (this.disposed) return;
        if (this.panel.activeInHierarchy) this.renderLobby(state);
        const context = this.actions.context(), battle = state.battle;
        if (!context || !battle || this.watchingRole !== context.identity.role) {
            if (this.visible || this.mounting) this.hideBattle();
            return;
        }
        if (!this.overlay.active) {
            this.actions.releaseInput();
            this.ui.closeAll();
            this.overlay.active = true;
        }
        if (!this.localError && this.prepared !== battle.key && this.mounting !== battle.key && battle.phase < 4) {
            this.mounting = battle.key;
            void this.mount(battle, context);
        }
        const frame = state.frame;
        if (frame && this.prepared === battle.key && frame.sequence !== this.appliedSequence && this.presentation.apply(frame)) {
            this.appliedSequence = frame.sequence;
        }
        this.renderBattle(state, battle);
    }

    private async mount(battle: Battle, context: MirrorArenaHostContext): Promise<void> {
        const original = context.identity, key = attachmentKey(original);
        const current = (): boolean => {
            const host = this.actions.context();
            return !this.disposed && this.watchingRole === original.role && this.transport.snapshot.battle?.key === battle.key &&
                !!host && attachmentKey(host.identity) === key;
        };
        const identity: AttachmentIdentity = { ...original, scene: battle.epoch, baseline: battle.epoch, map: battle.mapId, travelId: battle.challenge.id };
        try {
            const prepared = await this.presentation.prepare(identity, battle.key, context.ruleVersion, context.name, context.job, current,
                (done, total) => { if (current()) this.label(this.overlay, 'Header/Status', `准备战斗资源 ${done}/${total}`); });
            if (!prepared || !current()) return;
            this.prepared = battle.key;
            this.appliedSequence = '';
            const phase = this.transport.snapshot.battle?.phase;
            if (phase === 1) await this.transport.ready(battle.key);
            else if (phase === 2 && !this.transport.snapshot.frame) await this.transport.observe();
        } catch (error) {
            if (current()) this.localError = this.message(error);
        } finally {
            if (this.mounting === battle.key) {
                this.mounting = '';
                this.receive(this.transport.snapshot);
            }
        }
    }

    private hideBattle(): void {
        this.overlay.active = false;
        this.mounting = ''; this.prepared = ''; this.appliedSequence = ''; this.localError = '';
        this.presentation.reset();
    }

    private rows(state: Snapshot): readonly Row[] {
        const lobby = state.lobby;
        return (this.tab === 'Offers' ? lobby?.offers : this.tab === 'AttackHistory' ? lobby?.attackHistory : lobby?.defenseHistory) ?? noRows;
    }

    private selectedOffer(state: Snapshot): Offer | undefined {
        return this.tab === 'Offers' ? state.lobby?.offers.find(row => row.id === this.selection) : undefined;
    }

    private canChallenge(state: Snapshot): boolean {
        return !!state.attached && !!state.lobby?.open && state.lobby.entriesUsed < state.lobby.dailyLimit &&
            (!state.battle || state.battle.phase === 4) && !this.loading && !state.pending;
    }

    private renderLobby(state: Snapshot): void {
        const lobby = state.lobby, battle = state.battle, rows = this.rows(state);
        if (rows !== this.paintedRows || this.paintedSelection !== this.selection) {
            const preserveScroll = this.paintedRows !== undefined;
            this.paintedRows = rows; this.paintedSelection = this.selection;
            this.list.setItems(rows, (node, value) => {
                const row = value as Row;
                this.label(node, 'Text', (row.id === this.selection ? '● ' : '') + ('offerId' in row ?
                    `${row.defenderName}　${jobs[row.defenderJob]} ${row.defenderLevel}级` :
                    `${this.tab === 'DefenseHistory' ? row.attackerName : row.defenderName}　${outcome(row, this.tab === 'DefenseHistory')}`));
            }, preserveScroll);
        }
        for (const name of tabs) this.enabled(this.saved(this.panel, `SectionTabs/${name}`), this.tab !== name);
        this.label(this.panel, 'Season', lobby?.open ? `赛季 ${seasonDay(lobby.seasonStartUnixMs)} – ${seasonDay(lobby.seasonEndUnixMs)}` : '');
        this.label(this.panel, 'Attempts', lobby?.open ? `今日已用 ${lobby.entriesUsed}/${lobby.dailyLimit}` : '今日次数 —');
        this.label(this.panel, 'Score', lobby?.open ? `积分 ${lobby.seasonPoints}　连胜 ${lobby.streak}` : '赛季积分 —');
        this.label(this.panel, 'Defense', lobby?.publishedDefenseVersion && lobby.publishedDefenseVersion !== '0'
            ? `防守快照 v${lobby.publishedDefenseVersion}` : '尚未发布防守快照');
        this.label(this.panel, 'Status', this.notice || (state.errorCode ? protocolErrorMessage(20025, state.errorCode) :
            state.pending ? '正在核对' : !state.attached ? '等待角色连接' : !lobby ? '正在查询赛季' : lobby.open ? '' : '镜像竞技未开放'));
        const picked = rows.find(row => row.id === this.selection);
        const detail = picked ? ('offerId' in picked ? `防守快照 v${picked.defenseVersion}` :
            `${picked.attackerName} 对 ${picked.defenderName}\n快照 v${picked.defenseVersion}${this.tab === 'AttackHistory' ? `　积分 +${picked.awardedPoints}` : ''}`)
            : this.tab === 'Offers' ? (rows.length ? '选择系统分配的对手' : '暂无可挑战的对手') : '暂无战报';
        this.label(this.panel, 'Detail', detail);
        this.enabled(this.saved(this.panel, 'Challenge'), this.canChallenge(state) && !!this.selectedOffer(state));
        this.enabled(this.saved(this.panel, 'Refresh'), !!state.attached && !state.pending && !this.loading);
        this.enabled(this.saved(this.panel, 'Publish'), !!lobby?.open && !!state.attached && !state.pending && !this.loading &&
            (!battle || battle.phase === 4));
        this.enabled(this.saved(this.panel, 'Resume'), !!battle && !!state.attached && !this.loading);
        this.label(this.panel, 'Resume/Label', battle?.phase === 4 ? '查看结果' : '继续观战');
    }

    private renderBattle(state: Snapshot, battle: Battle): void {
        const { challenge, phase } = battle;
        const result = challenge.outcome === 3 ? '系统中断 · 未消耗次数' : `${outcome(challenge)} · 积分 +${challenge.awardedPoints}`;
        const currentFrame = state.frame;
        const fighter = (handle: number): string => {
            const entity = currentFrame?.entities.find(value => value.entityHandle === handle);
            return entity ? `\n生命 ${entity.hp}/${entity.hpMax}` : '';
        };
        this.label(this.overlay, 'Header/Attack', challenge.attackerName + fighter(1));
        this.label(this.overlay, 'Header/Defense', challenge.defenderName + fighter(2));
        const status = phase === 4 ? result : state.errorCode ? protocolErrorMessage(20025, state.errorCode) :
            this.localError || (phase === 3 ? '等待服务端确认结果' : this.prepared !== battle.key ? '准备战斗资源' :
                phase === 2 ? '双方自动战斗' : '等待服务端确认首帧');
        this.label(this.overlay, 'Header/Status', status);
        this.enabled(this.saved(this.overlay, 'Footer/Surrender'), phase === 2 && !state.pending);
        this.enabled(this.saved(this.overlay, 'Footer/Return'), phase >= 2 || !!this.localError);
    }

    private async request(action: () => Promise<void>, success = ''): Promise<void> {
        if (this.disposed) return;
        this.notice = '';
        try { await action(); if (!this.disposed) this.notice = success; }
        catch (error) {
            if (this.disposed) return;
            if (this.visible) this.localError = this.message(error);
            else this.notice = this.message(error);
        }
        this.receive(this.transport.snapshot);
    }

    private samePanel(identity: string): boolean {
        const context = this.actions.context();
        return !this.disposed && this.ui.top === this.panel && !!context && attachmentKey(context.identity) === identity;
    }

    private message(error: unknown): string { return error instanceof Error ? error.message : String(error); }

    private bind(node: Node, action: () => void | Promise<void>): void {
        const click = (): void => {
            if (this.disposed) return;
            this.actions.sound();
            void action();
        };
        node.on(Button.EventType.CLICK, click, this);
        this.listeners.push(() => { if (isValid(node, true)) node.off(Button.EventType.CLICK, click, this); });
    }

    private saved(root: Node, path: string): Node {
        const node = root.getChildByPath(path);
        if (!node) throw new Error(`镜像竞技缺少保存节点 ${path}`);
        return node;
    }

    private label(root: Node, path: string, value: string): void {
        const label = this.saved(root, path).getComponent(Label);
        if (label && label.string !== value) label.string = value;
    }

    private enabled(node: Node, value: boolean): void {
        const button = node.getComponent(Button);
        if (button && button.interactable !== value) button.interactable = value;
    }
}

const noRows: readonly Row[] = Object.freeze([]);
