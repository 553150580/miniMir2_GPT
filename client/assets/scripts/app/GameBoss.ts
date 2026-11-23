import { Button, Label, Node } from 'cc';
import { newOperationId } from './GameConnection';
import type { GameConnection, Packet } from './GameConnection';
import { protocolErrorMessage } from './GameErrors';

interface BossContext {
    sceneEpoch: { toString(): string };
    sceneKind: number;
    ready: boolean;
    level: number;
    worldBoss?: { display_name: string; minimum_level: number };
}

const personalState = ['','预约中','正在载入','挑战中','胜利','超时','已离开','战败'];
const worldState = ['','等待开放','正在载入','开放中','已击杀','已超时','已离开','战败'];
const qualifications = ['可入场','尚未开放','等级不足','已有挑战进行中','冷却中','今日次数已用完','请先返回普通挂机地图'];
const historyOutcomes = ['','胜利','超时','中断','战败'];
const pageSize = 5;

function dateText(value: { toString(): string } | undefined): string {
    const ms = Number(value?.toString() ?? 0);
    return ms > 0 && Number.isSafeInteger(ms) && ms < 8640000000000000 - 28800000
        ? new Date(ms + 28800000).toISOString().slice(5, 16).replace('T', ' ') : '—';
}

// 面板只展示服务端状态，入场、退场和成绩都以当前连接的回执为准。
export class GameBoss {
    private section: 3 | 4 = 3;
    private personal: any;
    private world: any;
    private active: any;
    private page = 0;
    private busy = false;
    private pendingPersonal = '';
    private generation: number;
    private readonly detach: () => void;

    constructor(private readonly panel: Node, private readonly connection: GameConnection,
        private readonly context: () => BossContext | null, private readonly closePanel: () => void) {
        this.generation = connection.generation;
        this.detach = connection.subscribe(packet => this.observe(packet));
    }

    dispose(): void { this.detach(); }

    async open(): Promise<void> {
        const kind = this.context()?.sceneKind;
        if (kind === 3 || kind === 4) this.section = kind;
        await this.show(this.section);
    }

    async show(section: 3 | 4): Promise<void> {
        this.section = section; this.page = 0;
        await this.reload();
    }

    turn(delta: number): void { this.page += delta; this.render(); }

    async reload(): Promise<void> {
        this.render();
        const context = this.context();
        if (!context?.ready) return;
        const section = this.section;
        if (section === 4 && !context.worldBoss) return;
        await this.connection.request(section === 3 ? 20015 : 20017, section === 3 ? {} : { sceneEpoch: context.sceneEpoch });
    }

    async action(): Promise<void> {
        const context = this.context();
        if (!context?.ready || this.busy) return;
        const { sceneEpoch, sceneKind } = context;
        this.busy = true; this.render();
        try {
            if (this.section === 3 && sceneKind === 3 && this.active?.state === 3) {
                this.connection.send(20013, { sceneEpoch, challengeId: this.active.challengeId });
            } else if (this.section === 4 && sceneKind === 4 && this.world?.state === 3) {
                this.connection.send(20019, { sceneEpoch, occurrenceId: this.world.occurrenceId });
            } else if (this.section === 3 && sceneKind === 1 && this.personal?.canEnter) {
                this.pendingPersonal ||= Array.from(newOperationId(), byte => byte.toString(16).padStart(2, '0')).join('');
                await this.connection.request(20012, { sceneEpoch, challengeId: this.pendingPersonal, templateId: this.personal.templateId });
            } else if (this.section === 4 && this.canEnterWorld(context)) {
                await this.connection.request(20018, { sceneEpoch, templateId: this.world.templateId, occurrenceId: this.world.occurrenceId });
            } else return;
            this.closePanel();
        } finally {
            this.busy = false; this.render();
        }
    }

    render(): void {
        if (this.generation !== this.connection.generation) {
            this.personal = this.world = this.active = null;
            this.pendingPersonal = ''; this.generation = this.connection.generation;
        }
        const context = this.context(), local = this.section === 3;
        const value = this.personal, world = this.world;
        const inside = context?.sceneKind === this.section;
        const rows: string[] = local ? (value?.history ?? []).map((row: any) =>
            `${dateText(row.finishedAtUnixMs)}  ${historyOutcomes[row.outcome] ?? '已结算'}  伤害 ${row.damage}${row.rewardGold && row.rewardGold.toString() !== '0' ? `  金币 +${row.rewardGold}` : ''}`)
            : (world?.ranks ?? []).map((row: any) => `${row.position}. ${row.displayName}  伤害 ${row.damage}`);
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
            this.label('Status', context?.worldBoss ? worldState[world?.state] ?? '等待查询' : '尚未开放');
            this.label('Details', world ? `上海时间 ${dateText(world.opensAtUnixMs)} 开场 / ${dateText(world.expiresAtUnixMs)} 结束\n首领生命 ${world.hp}/${world.hpMax}` : '');
        }
        this.label('Results', rows.slice(this.page * pageSize, (this.page + 1) * pageSize).join('\n') || (local ? '暂无挑战记录' : '暂无伤害排行'));
        this.label('Page', `${this.page + 1}/${pages}`);
        this.label('Action/Label', inside ? '离开挑战' : '进入挑战');
        this.enable('Action', !!context?.ready && !this.busy && (inside ? (local ? this.active?.state : world?.state) === 3
            : local ? context.sceneKind === 1 && value?.canEnter === true && !(this.active?.state > 0 && this.active.state < 4) : this.canEnterWorld(context)));
        this.enable('Previous', this.page > 0);
        this.enable('Next', this.page + 1 < pages);
    }

    private canEnterWorld(context: BossContext): boolean {
        return context.sceneKind === 1 && !!context.worldBoss && context.level >= context.worldBoss.minimum_level && this.world?.state === 3;
    }

    private observe(packet: Packet): void {
        if (packet.error) return;
        const value = packet.value;
        if (packet.id === 20016) this.personal = value;
        else if (packet.id === 20014) {
            this.active = value;
            if (value.state >= 4) this.pendingPersonal = '';
        } else if (packet.id === 20020) this.world = value;
        else return;
        this.render();
    }

    private label(name: string, text: string): void { this.panel.getChildByPath(name)!.getComponent(Label)!.string = text; }
    private enable(name: string, enabled: boolean): void { this.panel.getChildByName(name)!.getComponent(Button)!.interactable = enabled; }
}
