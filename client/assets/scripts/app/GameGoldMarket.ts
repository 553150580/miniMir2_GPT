import { Button, EditBox, isValid, Label, Node } from 'cc';
import Long from 'long';
import { List } from '../ui/List';
import type { PopupView } from '../ui/PopupView';
import type { UIMgr } from '../ui/UIMgr';
import { GoldMarketActionKind as Action, GoldMarketPhase as Phase, GoldMarketScope as Scope, GoldMarketSide as Side,
    type GoldMarketAction, type GoldMarketEscrow, type GoldMarketOrder, type GoldMarketReceipt, type GoldMarketState } from './GoldMarketProtocol';
import { goldMarketDuration, goldMarketTerms, previewGoldMarket } from './GoldMarketModel';
import { marketDeadline, marketMoney } from './ItemMarketPresentation';

interface GoldMarketContext {
    readonly scope: string; // 当前普通场景的原附着身份。
    readonly role: string;
    readonly ruleVersion: Long;
    readonly gold: Long; // 可用非绑定金币。
    readonly yuanbao: Long; // 可用非绑定元宝。
    readonly pending: boolean;
}

interface GoldMarketActions {
    query(scope: Scope, cursor?: Uint8Array): Promise<GoldMarketState>;
    mutate(command: GoldMarketAction): Promise<void>;
    tap(): void;
}

type Row = { readonly order: GoldMarketOrder } | { readonly escrow: GoldMarketEscrow };
const tabs = ['Buy', 'Sell', 'Mine', 'Escrow'];
const actorName = ['', '委托', '撤单', '领取'];
const orderStatus = ['', '活动中', '已成交', '已撤单', '已到期'];
const escrowKinds = ['', '买入金币', '卖方实收', '买单元宝退款', '卖单金币返还'];
const escrowReasons = ['', '成交', '价格改善', '撤单', '到期'];

// 金币市场只绑定已保存控件。实际撮合和托管均由服务端账本推进。
export class GameGoldMarket {
    private attachment = '';
    private scope = Scope.Sell;
    private state?: GoldMarketState;
    private side = Side.Buy;
    private selected?: Row;
    private readonly emptyCursor = new Uint8Array(0);
    private cursors = [this.emptyCursor];
    private page = 0;
    private checked = false;
    private querySerial = 0;
    private loading = false;
    private busy = false;
    private disposed = false;
    private notice = '';
    private readonly list: List;

    constructor(private readonly entry: Node, private readonly panel: Node, private readonly place: Node, private readonly detail: Node,
        private readonly ui: UIMgr, private readonly popup: PopupView, private readonly context: () => GoldMarketContext | undefined,
        private readonly actions: GoldMarketActions) {
        this.entry.active = false;
        this.list = panel.getChildByPath('List')!.getComponent(List)!;
        this.list.node.on(List.EVENT_ITEM_CLICK, this.choose, this);
    }

    discover(): void {
        if (!this.state?.enabled && !this.loading) this.checked = false;
        this.sync();
    }

    async open(): Promise<void> {
        this.sync();
        if (!this.state?.enabled || !this.current()) return;
        this.ui.open('GoldMarket');
        await this.refresh();
    }

    sync(): void {
        if (this.disposed) return;
        const identity = this.context()?.scope ?? '';
        if (identity !== this.attachment) {
            this.attachment = identity; this.state = undefined; this.scope = Scope.Sell;
            this.cursors = [this.emptyCursor]; this.page = 0; this.selected = undefined; this.notice = '';
            this.checked = false; this.loading = false; this.busy = false; this.querySerial++;
            if (this.ui.top === this.detail || this.ui.top === this.place) this.ui.back();
        }
        this.entry.active = !!identity && this.state?.enabled === true;
        if (identity && !this.checked && !this.loading && (this.ui.top?.name === 'Functions' || this.ui.top === this.panel)) {
            void this.read(this.scope, this.page, this.cursors[this.page], false, this.ui.top !== this.panel);
        }
        if (this.ui.top === this.panel) this.render();
        if (this.ui.top === this.place) this.renderPlace();
        if (this.ui.top === this.detail) this.renderDetail();
    }

    async switchScope(scope: Scope): Promise<void> {
        if (this.loading || this.busy || this.scope === scope) return;
        this.scope = scope; this.state = undefined; this.page = 0; this.cursors = [this.emptyCursor]; this.notice = '';
        this.drawRows(false);
        await this.read(scope, 0, this.emptyCursor, false);
    }

    async pageBy(step: number): Promise<void> {
        if (this.loading || this.busy) return;
        const cursor = step < 0 ? this.cursors[this.page - 1] : this.state?.nextCursor;
        if (!cursor || step > 0 && !cursor.length) return;
        await this.read(this.scope, this.page + step, cursor, false);
    }

    refresh(): Promise<void> { return this.read(this.scope, this.page, this.cursors[this.page], true); }

    openPlace(): void {
        if (!this.current() || !this.state?.canTrade || !this.state.rules || this.loading || this.busy) return;
        this.side = this.scope === Scope.Buy ? Side.Sell : Side.Buy;
        this.edit('Lots').string = '1';
        this.edit('Price').string = marketMoney(this.state.rules.referenceUnitPriceYuanbaoUnits);
        this.text(this.place, 'Status', '');
        this.ui.open('GoldMarketPlace'); this.renderPlace();
    }

    selectSide(side: Side): void {
        if (this.busy) return;
        this.side = side; this.renderPlace();
    }

    async previewPlace(): Promise<void> {
        const current = this.current(), rules = this.state?.rules, side = this.side;
        if (!current || !rules || !this.state?.canTrade || !current.ruleVersion.equals(rules.ruleVersion)) return;
        try {
            const preview = previewGoldMarket(rules, side, this.edit('Lots').string, this.edit('Price').string, current);
            await this.act(preview.request, preview.confirmation, this.place, () => {
                const latest = this.current();
                if (!latest || !this.state?.canTrade || this.state.rules?.termsHash !== rules.termsHash || !latest.ruleVersion.equals(rules.ruleVersion)) {
                    throw new Error('金币参考价或委托条款已变化，请刷新后确认');
                }
                if (side === Side.Buy ? latest.yuanbao.lessThan(preview.yuanbao) : latest.gold.lessThan(preview.gold)) throw new Error('原非绑定余额已变化，请重新确认');
            });
        } catch (error) { this.problem(this.place, error); }
    }

    async acceptDetail(): Promise<void> {
        const row = this.selected;
        if (!row) return;
        try {
            if ('escrow' in row) {
                await this.act({ action: Action.Claim, escrowId: row.escrow.escrowId, escrowVersion: row.escrow.version },
                    `领取 ${this.escrowText(row.escrow)}`, this.detail);
            } else if (row.order.owned && row.order.state === Phase.Active && !row.order.remainingLots.isZero()) {
                await this.act({ action: Action.Cancel, orderId: row.order.orderId, orderVersion: row.order.version },
                    `撤销剩余 ${row.order.remainingLots} 手委托\n未成交冻结金额退入交易托管`, this.detail);
            }
        } catch (error) { this.problem(this.detail, error); }
    }

    acceptReceipt(role: string, receipt: GoldMarketReceipt): void {
        if (this.current()?.role !== role) return;
        this.notice = receipt.action === Action.Place ? '委托已受理，等待撮合' : receipt.action === Action.Cancel ? '撤单已确认' : '领取已确认';
        if (this.ui.top === this.place || this.ui.top === this.detail) this.ui.back();
        void this.refresh();
    }

    dispose(): void {
        this.disposed = true;
        if (isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, this.choose, this);
    }

    private current(): GoldMarketContext | undefined {
        const value = this.context();
        return !this.disposed && value?.scope === this.attachment ? value : undefined;
    }

    private async read(scope: Scope, page: number, cursor: Uint8Array, preserve: boolean, quiet = false): Promise<void> {
        const origin = this.current();
        if (!origin || this.loading) return;
        const serial = ++this.querySerial;
        const matches = (): boolean => serial === this.querySerial && this.current()?.scope === origin.scope;
        this.loading = true; this.checked = true;
        if (this.ui.top === this.panel) this.render();
        try {
            const value = await this.actions.query(scope, cursor);
            if (!matches()) return;
            this.state = value; this.scope = scope; this.page = page; this.cursors[page] = cursor;
            this.drawRows(preserve);
        } catch (error) {
            if (matches() && !quiet) this.notice = error instanceof Error ? error.message : String(error);
        } finally {
            if (matches()) { this.loading = false; this.sync(); }
        }
    }

    private async act(request: GoldMarketAction, message: string, panel: Node, validate?: () => void): Promise<void> {
        const origin = this.current();
        if (!origin || origin.pending || this.busy || this.loading || this.ui.top !== panel) return;
        this.busy = true; this.sync();
        try {
            if (!await this.popup.confirm('Confirm', message, `${actorName[request.action]}确认`)) return;
            const now = this.current();
            if (!now || now.scope !== origin.scope || now.pending || this.ui.top !== panel) throw new Error('原角色或页面已变化，请刷新');
            validate?.();
            await this.actions.mutate(request);
        } finally {
            if (this.current()?.scope === origin.scope) { this.busy = false; this.sync(); }
        }
    }

    private readonly choose = (_index: number, row: Row): void => {
        if (!this.current() || this.loading || this.busy) return;
        this.actions.tap(); this.selected = row;
        this.text(this.detail, 'Status', '');
        this.ui.open('GoldMarketDetail'); this.renderDetail();
    };

    private drawRows(preserve: boolean): void {
        const rows: Row[] = this.state ? [...this.state.orders.map(order => ({ order })), ...this.state.escrows.map(escrow => ({ escrow }))] : [];
        this.list.setItems(rows, (node, data) => {
            const row = data as Row;
            this.text(node, 'Text', 'order' in row ? this.orderText(row.order) : this.escrowText(row.escrow));
        }, preserve);
        if (!preserve) this.list.scrollToIndex(0);
    }

    private render(): void {
        const current = this.current(), value = this.state;
        const available = !!current && !this.loading && !this.busy;
        for (const [index, tab] of tabs.entries()) this.button(this.panel, `Tabs/${tab}`).interactable = available && index + 1 !== this.scope;
        this.button(this.panel, 'Previous').interactable = available && this.page > 0;
        this.button(this.panel, 'Next').interactable = available && !!value?.nextCursor.length;
        this.button(this.panel, 'Refresh').interactable = available;
        this.button(this.panel, 'Place').interactable = available && !!value?.canTrade && !current?.pending;
        this.text(this.panel, 'Balance', current ? `非绑定金币 ${current.gold}　元宝 ${marketMoney(current.yuanbao)}` : '等待余额同步');
        this.text(this.panel, 'Rules', value?.rules ? goldMarketTerms(value.rules) : value?.enabled ? '旧委托可以撤单，托管仍可领取' : '');
        this.text(this.panel, 'Page', `第 ${this.page + 1} 页`);
        const empty = this.scope === Scope.Escrow ? '没有待领取的资产' : '当前没有委托';
        this.text(this.panel, 'Status', this.notice || (this.loading ? '正在查询' : value && !value.orders.length && !value.escrows.length ? empty : ''));
    }

    private renderPlace(): void {
        const rules = this.state?.rules, current = this.current();
        this.text(this.place, 'Terms', rules ? `${goldMarketTerms(rules)}\n每手 ${rules.goldPerLot} 金币 · 委托 ${goldMarketDuration(rules.orderDurationSeconds)}` : '暂无可使用的金币委托条款');
        this.button(this.place, 'Buy').interactable = this.side !== Side.Buy && !this.busy;
        this.button(this.place, 'Sell').interactable = this.side !== Side.Sell && !this.busy;
        this.button(this.place, 'Submit').interactable = !!rules && !!current && !current.pending && !this.busy && this.state?.canTrade === true;
    }

    private renderDetail(): void {
        const row = this.selected;
        if (!row) return;
        const canAct = 'escrow' in row || row.order.owned && row.order.state === Phase.Active && !row.order.remainingLots.isZero();
        const action = this.button(this.detail, 'Action');
        action.node.active = canAct;
        action.interactable = canAct && !this.busy && !this.current()?.pending;
        this.text(this.detail, 'Action/Label', 'escrow' in row ? '领取' : '撤单');
        this.text(this.detail, 'Title', 'escrow' in row ? '交易托管' : '委托详情');
        this.text(this.detail, 'Detail', 'escrow' in row ? `${this.escrowText(row.escrow)}\n${escrowReasons[row.escrow.reason]}` :
            `${this.orderText(row.order)}\n原委托 ${row.order.initialLots} 手\n有效期至 ${marketDeadline(row.order.expiresAtUnixMs)}`);
    }

    private escrowText(value: GoldMarketEscrow): string {
        return `${escrowKinds[value.kind]} ${value.gold.isZero() ? `${marketMoney(value.yuanbaoUnits)} 元宝` : `${value.gold} 金币`}`;
    }

    private orderText(value: GoldMarketOrder): string {
        return `${value.side === Side.Buy ? '买单' : '卖单'} · 剩余 ${value.remainingLots} 手\n${marketMoney(value.unitPriceYuanbaoUnits)} 元宝/手 · ${orderStatus[value.state]}`;
    }

    private problem(panel: Node, error: unknown): void {
        if (this.ui.top === panel && this.current()) this.text(panel, 'Status', error instanceof Error ? error.message : String(error));
    }

    private text(root: Node, path: string, text: string): void { root.getChildByPath(path)!.getComponent(Label)!.string = text; }
    private button(root: Node, path: string): Button { return root.getChildByPath(path)!.getComponent(Button)!; }
    private edit(path: string): EditBox { return this.place.getChildByPath(path)!.getComponent(EditBox)!; }
}
