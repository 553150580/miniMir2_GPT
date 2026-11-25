import { Button, isValid, Label, Node } from 'cc';
import Long from 'long';
import { List } from '../ui/List';
import type { PopupView } from '../ui/PopupView';
import type { UIMgr } from '../ui/UIMgr';
import type { MirrorArenaSnapshot, MirrorSeasonLedger, MirrorSeasonReward, MirrorStanding } from './MirrorArenaModel';

type SeasonRow = MirrorSeasonReward | MirrorStanding;
const none: readonly SeasonRow[] = Object.freeze([]);

interface SeasonActions {
    snapshot(): MirrorArenaSnapshot;
    query(): Promise<void>;
    // 附着和角色是确认窗打开时冻结的业务身份。
    scope(): string;
    claim(rewardId: Uint8Array): Promise<void>;
    exchange(value: { packCount: number; expectedPrice: Long; expectedItemId: number; expectedPackQuantity: Long }): Promise<void>;
    itemName(id: number): string | undefined;
    sound(): void;
}

function ingot(units: string): string {
    const value = BigInt(units);
    const whole = value / 10n, decimal = value % 10n;
    return decimal ? `${whole}.${decimal}` : whole.toString();
}

// 展示权威排名和已确认钱包，并把领取或兑换交回原资产事务。
export class GameMirrorSeason {
    private readonly list: List;
    private readonly handlers: Array<() => void> = [];
    private rows: readonly SeasonRow[] = none;
    private rewards = false;
    private selection = '';
    private paintedSelection = '';
    private localNotice = '';
    private busy = false;
    private disposed = false;

    constructor(private readonly entry: Node, private readonly panel: Node, private readonly ui: UIMgr,
        private readonly popup: PopupView, private readonly actions: SeasonActions) {
        const list = this.saved('List').getComponent(List);
        if (!list) throw new Error('镜像赛季缺少已保存的列表');
        this.list = list;
        this.bind(entry, async () => {
            if (!actions.snapshot().lobby?.season || !actions.scope()) return;
            ui.open('MirrorSeason');
            await this.run(() => actions.query());
        });
        this.bind(this.saved('Tabs/Rank'), () => this.tab(false));
        this.bind(this.saved('Tabs/Rewards'), () => this.tab(true));
        this.bind(this.saved('Refresh'), () => this.run(() => actions.query()));
        this.bind(this.saved('Claim'), () => this.claim());
        this.bind(this.saved('Exchange'), () => this.exchange());
        const choose = (_index: number, row: SeasonRow): void => {
            if (!this.rows.includes(row)) return;
            actions.sound();
            this.selection = this.key(row);
            this.refresh(actions.snapshot());
        };
        this.list.node.on(List.EVENT_ITEM_CLICK, choose, this);
        this.handlers.push(() => { if (isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, choose, this); });
    }

    refresh(snapshot: MirrorArenaSnapshot): void {
        if (this.disposed) return;
        const ledger = snapshot.lobby?.season;
        this.entry.active = !!ledger;
        this.enable(this.entry, !!ledger && snapshot.attached && !snapshot.pending);
        if (!this.panel.activeInHierarchy) return;
        const rows = (this.rewards ? ledger?.rewards : ledger?.standings) ?? none;
        if (rows !== this.rows || this.selection !== this.paintedSelection) {
            this.rows = rows;
            this.paintedSelection = this.selection;
            this.list.setItems(rows, (node, value) => {
                const row = value as SeasonRow, label = node.getChildByName('Text')?.getComponent(Label);
                const selected = this.selection === this.key(row) ? '● ' : '';
                if (label) label.string = selected + ('id' in row ? `${row.name}　绑定元宝 ${ingot(row.boundYuanbao)}` :
                    `第 ${row.rank} 名　${row.name}　${row.points} 分`);
            }, true);
        }
        const pending = this.busy || snapshot.pending;
        const ready = !!ledger && snapshot.attached && !pending;
        const picked = rows.find(row => this.key(row) === this.selection);
        const current = ledger?.ownRank ? `本季第 ${ledger.ownRank} 名` : '本季尚未上榜';
        const last = ledger?.last ? `\n最近赛季结转 · 第 ${ledger.last.rank} 名 · ${ledger.last.credit} 分` : '';
        this.text('Headline', current + last);
        this.text('Balance', ledger ? `兑换积分 ${ledger.points}` : '兑换积分 —');
        this.text('Status', this.localNotice || (!snapshot.attached ? '等待角色连接' : pending ? '正在核对' : ''));
        this.text('Detail', picked ? 'id' in picked ? `${picked.rank ? `第 ${picked.rank} 名` : picked.name} · 绑定元宝 ${ingot(picked.boundYuanbao)}`
            : `已完成 ${picked.matches} 场挑战` : this.rewards ? ledger?.moreRewards ? '领取后可查看后续奖励' : '选择待领奖项'
                : rows.length ? '榜单按原赛季竞技分排名' : '当前赛季暂无已确认排名');
        const quote = ledger?.exchange, name = quote && this.actions.itemName(quote.itemId);
        if (this.rewards && !rows.length && !ledger?.moreRewards) this.text('Detail', '暂无待领奖项');
        this.text('Exchange/Label', quote && name ? `${quote.price} 分兑换` : '兑换未开放');
        this.enable(this.saved('Tabs/Rank'), this.rewards);
        this.enable(this.saved('Tabs/Rewards'), !this.rewards);
        this.enable(this.saved('Refresh'), snapshot.attached && !pending);
        this.enable(this.saved('Claim'), ready && !!picked && 'id' in picked);
        this.enable(this.saved('Exchange'), ready && !!quote && !!name && BigInt(ledger!.points) >= BigInt(quote.price));
    }

    dispose(): void {
        this.disposed = true;
        for (const unbind of this.handlers) unbind();
        this.handlers.length = 0;
    }

    private tab(showRewards: boolean): void {
        this.rewards = showRewards; this.selection = ''; this.localNotice = '';
        this.refresh(this.actions.snapshot());
    }

    private async claim(): Promise<void> {
        const scope = this.actions.scope();
        const reward = this.ledger()?.rewards.find(row => row.id === this.selection);
        if (!scope || !reward || this.busy) return;
        if (!await this.popup.confirm('Confirm', `领取${reward.name}的 ${ingot(reward.boundYuanbao)} 绑定元宝？`, '镜像赛季')) return;
        if (!this.current(scope) || !this.ledger()?.rewards.some(row => row.id === reward.id)) return;
        await this.run(() => this.actions.claim(reward.rewardId), '已领取赛季奖励');
    }

    private async exchange(): Promise<void> {
        const scope = this.actions.scope(), quote = this.ledger()?.exchange;
        const name = quote && this.actions.itemName(quote.itemId);
        if (!scope || !quote || !name || this.busy) return;
        if (!await this.popup.confirm('Confirm', `花费 ${quote.price} 兑换积分，换取绑定${name} ×${quote.quantity}？`, '镜像兑换')) return;
        const next = this.ledger()?.exchange;
        if (!this.current(scope) || next?.itemId !== quote.itemId || next.price !== quote.price || next.quantity !== quote.quantity) return;
        await this.run(() => this.actions.exchange({ packCount: 1, expectedPrice: Long.fromString(quote.price), expectedItemId: quote.itemId,
            expectedPackQuantity: Long.fromString(quote.quantity) }), '兑换已确认');
    }

    private async run(action: () => Promise<void>, success = ''): Promise<void> {
        if (this.disposed || this.busy || !this.actions.scope()) return;
        this.busy = true; this.localNotice = '';
        this.refresh(this.actions.snapshot());
        try {
            await action();
            this.localNotice = success;
            if (success) {
                try { await this.actions.query(); }
                catch { this.localNotice = `${success}，请刷新查看余额和待领`; }
            }
        }
        catch (error) { this.localNotice = error instanceof Error ? error.message : String(error); }
        finally { this.busy = false; this.refresh(this.actions.snapshot()); }
    }

    private current(scope: string): boolean {
        return !this.disposed && this.ui.top === this.panel && this.actions.scope() === scope;
    }

    private ledger(): MirrorSeasonLedger | null | undefined { return this.actions.snapshot().lobby?.season; }

    private key(row: SeasonRow): string { return 'id' in row ? row.id : String(row.rank); }

    private bind(node: Node, action: () => void | Promise<void>): void {
        const click = (): void => { if (!this.disposed) { this.actions.sound(); void action(); } };
        node.on(Button.EventType.CLICK, click, this);
        this.handlers.push(() => { if (isValid(node, true)) node.off(Button.EventType.CLICK, click, this); });
    }

    private saved(path: string): Node {
        const node = this.panel.getChildByPath(path);
        if (!node) throw new Error(`镜像赛季缺少保存节点 ${path}`);
        return node;
    }

    private text(path: string, value: string): void {
        const label = this.saved(path).getComponent(Label);
        if (label && label.string !== value) label.string = value;
    }

    private enable(node: Node, value: boolean): void {
        const button = node.getComponent(Button);
        if (button && button.interactable !== value) button.interactable = value;
    }
}
