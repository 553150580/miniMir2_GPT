import { Button, EditBox, isValid, Label, Node } from 'cc';
import Long from 'long';
import type { PopupView } from '../ui/PopupView';
import type { UIMgr } from '../ui/UIMgr';
import { List } from '../ui/List';
import { ItemMarketActionKind as Action, ItemMarketEscrowKind as Escrow, ItemMarketScope as Scope, marketId,
    type ItemMarketAction, type ItemMarketEscrow, type ItemMarketItem, type ItemMarketOrder, type ItemMarketReceipt, type ItemMarketState } from './ItemMarketProtocol';
import { marketAmount, marketBagCandidate, marketDeadline, marketListing, marketMoney, marketOrderAction, marketOrderActions, marketPhase } from './ItemMarketPresentation';

interface ItemMarketContext {
    readonly scope: string; // 当前原角色连接和已经确认的普通场景。
    readonly role: string; // 原永久角色，不跨账号/区服恢复 UI 反馈。
    readonly bag: readonly ItemMarketItem[];
    readonly wallet: Long; // 非绑定元宝余额。
    readonly pending: boolean;
}

interface ItemMarketActions {
    query(scope: Scope, cursor?: Uint8Array): Promise<ItemMarketState>;
    mutate(command: ItemMarketAction): Promise<void>;
    definition(itemID: number): { readonly name: string; readonly kind: string } | undefined;
    tap(): void;
}

type MarketRow = { readonly order: ItemMarketOrder } | { readonly escrow: ItemMarketEscrow };
const actionText = ['', '上架', '预购', '加价', '直购', '撤单', '领取'];
const escrowText = ['', '托管物品', '成交所得', '预购退款'];

// 绑定保存的市场、上架和详情面板。请求、报价和领取结果都经过角色资产命令账本。
export class GameItemMarket {
    private attachment = '';
    private scope = Scope.Public;
    private state?: ItemMarketState;
    private readonly emptyCursor = new Uint8Array(0);
    private cursors = [this.emptyCursor];
    private page = 0;
    private capability = false;
    private checked = false;
    private querySerial = 0;
    private loading = false;
    private takingAction = false;
    private disposed = false;
    private message = '';
    private selected?: MarketRow;
    private selectedBag?: ItemMarketItem;
    private renderedBag?: readonly ItemMarketItem[];
    private readonly list: List;
    private readonly bagList: List;
    private readonly placeholder: Node | null;

    constructor(private readonly entry: Node, private readonly panel: Node, private readonly orderPanel: Node,
        private readonly listing: Node, private readonly ui: UIMgr, private readonly popup: PopupView,
        private readonly context: () => ItemMarketContext | undefined, private readonly actions: ItemMarketActions) {
        this.list = panel.getChildByPath('List')!.getComponent(List)!;
        this.bagList = listing.getChildByPath('List')!.getComponent(List)!;
        this.placeholder = entry.parent?.getChildByName('Text') ?? null;
        this.entry.active = false;
        this.list.node.on(List.EVENT_ITEM_CLICK, this.chooseRow, this);
        this.bagList.node.on(List.EVENT_ITEM_CLICK, this.chooseBag, this);
    }

    async open(): Promise<void> {
        this.sync();
        if (!this.capability || !this.current()) return;
        this.ui.open('ItemMarket');
        await this.load(this.scope, this.page, this.cursors[this.page], true);
    }

    // 用户重开功能面板时可以重新探测被临时拒绝的市场目录，关闭的旧服仍静默。
    discover(): void {
        if (!this.capability && !this.loading) this.checked = false;
        this.sync();
    }

    sync(): void {
        if (this.disposed) return;
        const current = this.context();
        if ((current?.scope ?? '') !== this.attachment) {
            this.attachment = current?.scope ?? '';
            this.scope = Scope.Public; this.state = undefined; this.cursors = [this.emptyCursor]; this.page = 0;
            this.message = ''; this.querySerial++; this.checked = false; this.capability = false; this.loading = false; this.takingAction = false;
            this.selected = undefined; this.selectedBag = undefined; this.renderedBag = undefined;
            if (this.ui.top === this.orderPanel || this.ui.top === this.listing) this.ui.back();
        }
        this.entry.active = !!current && this.capability;
        if (this.placeholder) this.placeholder.active = !this.entry.active;
        if (current && !this.checked && !this.loading && (this.ui.top?.name === 'Functions' || this.ui.top === this.panel)) {
            void this.load(Scope.Public, 0, this.emptyCursor, false, this.ui.top !== this.panel);
        }
        if (this.ui.top === this.panel) this.renderControls();
        if (this.ui.top === this.orderPanel) this.renderOrder();
        if (this.ui.top === this.listing) this.renderListing();
    }

    async switchScope(scope: Scope): Promise<void> {
        if (!this.current() || this.loading || this.takingAction || this.scope === scope && this.state) return;
        this.scope = scope; this.state = undefined; this.cursors = [this.emptyCursor]; this.page = 0; this.message = '';
        this.renderRows(false);
        await this.load(scope, 0, this.emptyCursor, false);
    }

    async movePage(offset: number): Promise<void> {
        if (this.loading || this.takingAction || !this.state) return;
        const cursor = offset < 0 ? this.cursors[this.page - 1] : this.state.nextCursor;
        if (!cursor || offset > 0 && !cursor.length) return;
        await this.load(this.scope, this.page + offset, cursor, false);
    }

    refresh(): Promise<void> { return this.load(this.scope, this.page, this.cursors[this.page], true); }

    openListing(): void {
        if (!this.current() || !this.state?.canTrade || this.takingAction) return;
        this.selectedBag = undefined; this.renderedBag = undefined; this.message = '';
        this.edit(this.listing, 'Quantity').string = '1'; this.edit(this.listing, 'Price').string = '';
        this.text(this.listing, 'Status', '');
        this.ui.open('ItemMarketListing'); this.renderListing();
    }

    async submitListing(): Promise<void> {
        const current = this.current();
        const selected = this.selectedBag;
        if (!current || !selected || !this.state?.canTrade) return;
        const item = current.bag.find(row => marketId(row.instanceId) === marketId(selected.instanceId));
        if (!item || !item.version.equals(selected.version) || !this.listable(item)) throw new Error('原物品已经变化，请重新选择');
        const command = marketListing(item, this.edit(this.listing, 'Quantity').string, this.edit(this.listing, 'Price').string);
        if (command.action !== Action.List) return;
        const amount = marketAmount(command.unitPriceYuanbaoUnits, command.quantity);
        await this.act(command, `${this.itemName(item.itemId)} ×${command.quantity} · ${marketMoney(command.unitPriceYuanbaoUnits)} 元宝/件\n整单 ${marketMoney(amount)} 元宝\n上架免费，成交后由卖方支付佣金`, this.listing);
    }

    async orderAction(kind?: Action): Promise<void> {
        const row = this.selected;
        if (!row || !this.state) return;
        if ('escrow' in row) {
            await this.act({ action: Action.Claim, escrowId: row.escrow.escrowId, escrowVersion: row.escrow.version, quantity: Long.ZERO },
                `领取 ${this.escrowDescription(row.escrow)}`, this.orderPanel);
            return;
        }
        const order = row.order;
        const action = kind ?? marketOrderActions(order, this.state.canTrade)[0];
        const command = marketOrderAction(order, action, this.state.canTrade, this.edit(this.orderPanel, 'Quantity').string);
        if (command.action === Action.List || command.action === Action.Claim) return;
        const title = `${this.itemName(order.item.itemId)} ×${command.quantity}`;
        const price = marketMoney(command.unitPriceYuanbaoUnits);
        let detail = '';
        if (action === Action.Cancel) detail = '免费撤单，余货退入交易托管仓';
        else if (action === Action.Raise) {
            const next = order.nextUnitPriceYuanbaoUnits;
            const difference = marketAmount(next.subtract(order.unitPriceYuanbaoUnits), command.quantity);
            detail = `单价 ${price} → ${marketMoney(next)} 元宝\n追加冻结 ${marketMoney(difference)} 非绑定元宝`;
        } else {
            const total = marketAmount(order.unitPriceYuanbaoUnits, command.quantity);
            detail = `${price} 元宝/件\n${action === Action.Bid ? '冻结' : '支付'} ${marketMoney(total)} 非绑定元宝`;
        }
        await this.act(command, `${title}\n${detail}`, this.orderPanel);
    }

    acceptReceipt(role: string, receipt: ItemMarketReceipt): void {
        if (this.context()?.role !== role || this.disposed) return;
        this.message = `${actionText[receipt.action]}已确认`;
        if (this.ui.top === this.orderPanel || this.ui.top === this.listing) this.ui.back();
        void this.refresh();
    }

    dispose(): void {
        this.disposed = true;
        if (isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, this.chooseRow, this);
        if (isValid(this.bagList.node, true)) this.bagList.node.off(List.EVENT_ITEM_CLICK, this.chooseBag, this);
    }

    private current(): ItemMarketContext | undefined {
        const current = this.context();
        return !this.disposed && current?.scope === this.attachment ? current : undefined;
    }

    private async load(scope: Scope, page: number, cursor: Uint8Array, preserve: boolean, quiet = false): Promise<void> {
        const original = this.current();
        if (!original || this.loading) return;
        const query = ++this.querySerial;
        this.loading = true; this.checked = true; this.renderControls();
        try {
            const state = await this.actions.query(scope, cursor);
            if (query !== this.querySerial || this.current()?.scope !== original.scope) return;
            this.capability = state.enabled;
            this.state = state; this.scope = scope; this.page = page; this.cursors[page] = cursor;
            this.renderRows(preserve);
        } catch (error) {
            if (query === this.querySerial && this.current()?.scope === original.scope && !quiet) this.message = error instanceof Error ? error.message : String(error);
        } finally {
            if (query === this.querySerial && this.current()?.scope === original.scope) { this.loading = false; this.sync(); }
        }
    }

    private async act(command: ItemMarketAction, message: string, panel: Node): Promise<void> {
        const original = this.current();
        if (!original || original.pending || this.takingAction || this.loading || this.ui.top !== panel) return;
        this.takingAction = true;
        try {
            if (!await this.popup.confirm('Confirm', message, `道具${actionText[command.action]}确认`)) return;
            const current = this.current();
            if (!current || current.scope !== original.scope || current.pending || this.ui.top !== panel) throw new Error('原角色或市场页面已变化，请重新查看');
            await this.actions.mutate(command);
        } catch (error) {
            if (this.current()?.scope === original.scope && this.ui.top === panel) this.text(panel, 'Status', error instanceof Error ? error.message : String(error));
            throw error;
        } finally {
            if (this.current()?.scope === original.scope) { this.takingAction = false; this.sync(); }
        }
    }

    private readonly chooseRow = (_index: number, row: MarketRow): void => {
        if (this.loading || this.takingAction || !this.current()) return;
        this.actions.tap(); this.selected = row;
        this.edit(this.orderPanel, 'Quantity').string = '1';
        this.text(this.orderPanel, 'Status', '');
        this.ui.open('ItemMarketOrder'); this.renderOrder();
    };

    private readonly chooseBag = (_index: number, item: ItemMarketItem): void => {
        if (this.takingAction) return;
        this.actions.tap(); this.selectedBag = item;
        this.edit(this.listing, 'Quantity').string = '1'; this.edit(this.listing, 'Price').string = '';
        this.renderListing();
    };

    private renderRows(preserve: boolean): void {
        const rows: MarketRow[] = this.state ? [...this.state.orders.map(order => ({ order })), ...this.state.escrows.map(escrow => ({ escrow }))] : [];
        this.list.setItems(rows, (node, data) => {
            const row = data as MarketRow;
            this.text(node, 'Text', 'order' in row ? `${this.itemName(row.order.item.itemId)} ×${row.order.remainingQuantity}\n${marketMoney(row.order.unitPriceYuanbaoUnits)} 元宝/件 · ${marketPhase(row.order.state)}` : this.escrowDescription(row.escrow));
        }, preserve);
        if (!preserve) this.list.scrollToIndex(0);
    }

    private renderControls(): void {
        const current = this.current();
        const available = !!current && !this.loading && !this.takingAction;
        const selected = this.scope === Scope.Public ? 'Public' : this.scope === Scope.Mine ? 'Mine' : 'Escrow';
        for (const tab of ['Public', 'Mine', 'Escrow']) this.button(this.panel, `Tabs/${tab}`).interactable = tab !== selected && available;
        this.button(this.panel, 'Previous').interactable = available && this.page > 0;
        this.button(this.panel, 'Next').interactable = available && !!this.state?.nextCursor.length;
        this.button(this.panel, 'Refresh').interactable = available;
        this.button(this.panel, 'ListItem').interactable = available && this.state?.canTrade === true && !current?.pending;
        this.text(this.panel, 'Page', `第 ${this.page + 1} 页`);
        const empty = this.scope === Scope.Escrow ? '没有待领取的托管资产' : '当前没有订单';
        this.text(this.panel, 'Status', this.message || (this.loading ? '正在查询' : this.state && !this.state.orders.length && !this.state.escrows.length ? empty : ''));
        this.text(this.panel, 'Balance', current ? `非绑定元宝 ${marketMoney(current.wallet)}${this.state?.canTrade === false ? ' · 特权未生效' : ''}` : '等待角色同步');
    }

    private renderOrder(): void {
        const row = this.selected;
        if (!row || !this.state) return;
        const active = !!this.current() && !this.current()?.pending && !this.takingAction;
        const choices = 'order' in row ? marketOrderActions(row.order, this.state.canTrade) : [Action.Claim];
        const kind = choices[0];
        this.button(this.orderPanel, 'Action').node.active = !!kind;
        this.button(this.orderPanel, 'Action').interactable = active;
        this.text(this.orderPanel, 'Action/Label', actionText[kind] ?? '');
        this.edit(this.orderPanel, 'Quantity').node.active = kind === Action.Direct;
        this.orderPanel.getChildByPath('QuantityLabel')!.active = kind === Action.Direct;
        if ('order' in row) {
            const order = row.order;
            this.text(this.orderPanel, 'Title', this.itemName(order.item.itemId));
            this.text(this.orderPanel, 'Detail', `${marketPhase(order.state)} · 剩余 ${order.remainingQuantity} 件\n单价 ${marketMoney(order.unitPriceYuanbaoUnits)} 元宝\n有效预购 ${order.activeBidCount} · ${marketDeadline(order.endAtUnixMs)}`);
        } else {
            this.text(this.orderPanel, 'Title', '交易托管');
            this.text(this.orderPanel, 'Detail', this.escrowDescription(row.escrow));
        }
    }

    private renderListing(): void {
        const current = this.current();
        if (!current) return;
        if (this.renderedBag !== current.bag) {
            this.renderedBag = current.bag;
            this.bagList.setItems(current.bag.filter(item => this.listable(item)), (node, data) => {
                const item = data as ItemMarketItem;
                this.text(node, 'Text', `${this.itemName(item.itemId)} ×${item.quantity}`);
            });
        }
        this.text(this.listing, 'Selection', this.selectedBag ? `已选 ${this.itemName(this.selectedBag.itemId)} ×${this.selectedBag.quantity}` : '选择可交易的背包物品');
        this.button(this.listing, 'Submit').interactable = !!this.selectedBag && !current.pending && !this.takingAction;
    }

    private listable(item: ItemMarketItem): boolean {
        return marketBagCandidate(item, this.actions.definition(item.itemId)?.kind === 'equipment');
    }

    private escrowDescription(row: ItemMarketEscrow): string {
        return row.kind === Escrow.Item && row.item ? `${this.itemName(row.item.itemId)} ×${row.item.quantity}` : `${escrowText[row.kind]} ${marketMoney(row.yuanbaoUnits)} 元宝`;
    }

    private itemName(id: number): string { return this.actions.definition(id)?.name ?? `道具 ${id}`; }
    private text(root: Node, path: string, text: string): void { root.getChildByPath(path)!.getComponent(Label)!.string = text; }
    private button(root: Node, path: string): Button { return root.getChildByPath(path)!.getComponent(Button)!; }
    private edit(root: Node, path: string): EditBox { return root.getChildByPath(path)!.getComponent(EditBox)!; }
}
