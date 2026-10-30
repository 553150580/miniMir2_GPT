import { Button, EditBox, isValid, Label, Node } from 'cc';
import Long from 'long';
import { List } from '../ui/List';
import { CodeWarehousePrivilegeRequired } from '../generated/protocol_ids';
import { sourceName } from './SkillAssetsModel';
import { createWarehousePreview, warehouseConfig, warehouseConfirmation, warehouseId, warehouseInteger, warehouseItemReason, warehouseMoneyText,
    warehouseSlots, WarehouseItem, WarehousePage, WarehousePager, WarehousePreview, WarehouseQuery, WarehouseScope, WarehouseSlot, WarehouseSnapshot, WarehouseTicket } from './WarehouseModel';

export type WarehouseInput = Omit<WarehouseSnapshot, 'page'> & { definitions: readonly { item_id: number; name: string }[]; writeBlocked: boolean };

// 固定控件全部来自保存场景；只查询一页、复用条目并保留两类仓库各自的页码/滚动。
export class GameWarehouse {
    private readonly panel: Node;
    private readonly bag: List;
    private readonly store: List;
    private readonly pager = new WarehousePager();
    private readonly offsets = new Map<WarehouseScope, number>();
    private input: WarehouseInput | null = null;
    private scope: WarehouseScope = 1;
    private selectedBag = '';
    private selectedSlot = '';
    private selectedFrom: 'bag' | 'store' = 'bag';
    private currency = 1;
    private busy = false;
    private failed = false;
    private disposed = false;
    private retired = false;
    private notice = '';
    private bagKey = '';
    private storeKey = '';
    private reading: WarehouseTicket | null = null;
    constructor(safe: Node, private readonly current: () => WarehouseInput,
        private readonly query: (request: WarehouseQuery) => Promise<WarehousePage>) {
        const panel = safe.getChildByPath('UIMgr/Panels/Warehouse');
        if (!panel) throw new Error('仓库面板尚未保存');
        this.panel = panel;
        this.bag = this.saved('BagList').getComponent(List)!;
        this.store = this.saved('WarehouseList').getComponent(List)!;
        if (!this.bag || !this.store) throw new Error('仓库列表尚未保存');
        this.bag.node.on(List.EVENT_ITEM_CLICK, this.selectBag, this);
        this.store.node.on(List.EVENT_ITEM_CLICK, this.selectSlot, this);
        this.panel.on(Node.EventType.NODE_DESTROYED, this.retire, this);
    }
    get page(): WarehousePage | null { return this.pager.page(this.scope); }
    get activeScope(): WarehouseScope { return this.scope; }
    get snapshot(): WarehouseSnapshot {
        const input = this.current();
        return { ...input, ready: input.ready && !input.writeBlocked && !this.busy && !this.failed, page: this.page };
    }
    refresh(input: WarehouseInput): void {
        if (this.disposed || this.retired) return;
        if (this.input?.attachment !== input.attachment) {
            this.pager.bind(input.attachment); this.selectedBag = ''; this.selectedSlot = ''; this.offsets.clear();
            this.scope = 1; this.busy = false; this.failed = false; this.notice = ''; this.bagKey = ''; this.storeKey = '';
            this.reading = null;
            this.edit('Quantity').string = '1'; this.edit('Amount').string = '';
        }
        this.input = input;
        if (this.selectedBag && !input.inventory.some(item => warehouseId(item.instanceId) === this.selectedBag)) this.selectedBag = '';
        const key = input.inventory.map(item => `${warehouseId(item.instanceId)}:${item.version}:${item.quantity}`).join('|');
        if (key !== this.bagKey) { this.bagKey = key; this.bag.setItems(input.inventory, this.renderBag, true); }
        const page = this.page, storeKey = page ? `${page.scope}:${page.revision}:${page.startSlot}:${page.capacity}` : '';
        if (storeKey !== this.storeKey) {
            this.storeKey = storeKey; this.store.setItems(page ? warehouseSlots(page) : [], this.renderSlot, true);
        }
        this.refreshControls();
    }
    async open(scope: WarehouseScope): Promise<void> {
        if (this.disposed) return;
        this.refresh(this.current());
        this.rememberOffset(); this.scope = scope; this.selectedSlot = ''; this.failed = false; this.notice = '';
        await this.load(this.pager.page(scope)?.startSlot ?? Long.UZERO, true);
    }
    async reload(): Promise<void> { await this.load(this.page?.startSlot ?? Long.UZERO, false); }
    async turnPage(direction: -1 | 1): Promise<void> {
        const page = this.page; if (!page) throw new Error('请先查询仓库');
        const start = warehouseInteger(page.startSlot), limit = BigInt(page.limit), capacity = warehouseInteger(page.capacity);
        const next = direction > 0 ? start + limit : start > limit ? start - limit : 0n;
        if (next >= capacity || next === start) return;
        this.rememberOffset(); this.selectedSlot = ''; this.offsets.set(this.scope, 0);
        await this.load(Long.fromString(next.toString(), true), true);
    }
    chooseCurrency(currency: 1 | 2): void { if (this.busy) return; this.currency = currency; this.edit('Amount').string = ''; this.refreshControls(); }
    prepare(action: number): WarehousePreview {
        const slot = this.selectedSlot ? Long.fromString(this.selectedSlot, true) : Long.UZERO;
        const item = action === 1 ? this.input?.inventory.find(row => warehouseId(row.instanceId) === this.selectedBag)
            : action === 2 ? this.page?.items.find(row => row.slot.equals(slot))?.item : null;
        if (action <= 2 && !this.selectedSlot) throw new Error('请先选择当前仓位');
        return createWarehousePreview(this.snapshot, action, slot, item ?? null,
            this.edit(action <= 2 ? 'Quantity' : 'Amount').string.trim(), action === 3 || action === 4 ? this.currency : 0);
    }
    confirmation(preview: WarehousePreview, definitions: readonly { item_id: number; name: string }[]): string {
        const id = preview.request.itemInstanceId, source = preview.request.action === 1 ? this.input?.inventory : this.page?.items.map(row => row.item);
        const item = id.length ? source?.find(row => warehouseId(row.instanceId) === warehouseId(id)) : null;
        return warehouseConfirmation(preview, warehouseConfig(this.input!.config), item ? definitions.find(row => row.item_id === item.itemId)?.name ?? '物品资料待同步' : '');
    }
    detail(definitions: readonly { item_id: number; name: string }[]): { title: string; lines: string[] } {
        const item = this.selectedFrom === 'bag' ? this.input?.inventory.find(row => warehouseId(row.instanceId) === this.selectedBag)
            : this.page?.items.find(row => row.slot.toString() === this.selectedSlot)?.item;
        if (!item) throw new Error('先选择要查看的原物品');
        return { title: definitions.find(row => row.item_id === item.itemId)?.name ?? '物品资料待同步',
            lines: [this.selectedFrom === 'bag' ? '背包原物品' : '仓库原物品', `数量 ${item.quantity}`, item.bound ? '绑定物品' : '非绑定物品',
                `品质 ${item.quality}`, `来源 ${sourceName(item.sourceKind)}`, item.locked ? '已锁定；允许存取，回收受保护' : '未锁定',
                warehouseItemReason(item, this.scope) || '可按明确数量存取，不自动合并'] };
    }
    async committed(sourceScope: WarehouseScope = this.scope): Promise<void> {
        if (this.disposed) return;
        const start = this.pager.page(sourceScope)?.startSlot ?? Long.UZERO;
        if (sourceScope === this.scope) this.rememberOffset();
        this.pager.invalidate(sourceScope);
        if (sourceScope !== this.scope) return;
        this.selectedBag = ''; this.selectedSlot = ''; this.notice = '原操作已确认，正在重新查询';
        await this.load(start, true);
    }
    rejected(error: unknown, sourceScope: WarehouseScope = this.scope): void {
        if (this.disposed) return;
        if ((error as { code?: number }).code === CodeWarehousePrivilegeRequired) {
            this.pager.invalidate(2);
            if (this.scope === 2) { this.selectedSlot = ''; this.storeKey = ''; this.store.setItems([], this.renderSlot, false); }
        }
        if (sourceScope !== this.scope) return;
        this.failed = true; this.notice = error instanceof Error ? error.message : '仓库操作失败，请刷新';
        this.refreshControls();
    }
    private async load(startSlot: Long, restore: boolean): Promise<void> {
        const input = this.current(); this.refresh(input);
        if (!input.ready || !input.attachment) throw new Error('当前场景尚未就绪');
        const config = warehouseConfig(input.config), scope = this.scope;
        const ticket = this.pager.begin({ scope, startSlot, limit: config.query_page_limit });
        this.reading = ticket;
        this.busy = true; this.failed = false; this.refreshControls();
        try {
            const value = await this.query(ticket.query), current = this.current();
            if (!this.isReading(ticket) || current.attachment !== ticket.attachment) return;
            if (this.pager.accept(ticket, value, current.role, config)) {
                this.busy = false; this.failed = false; this.notice = ''; this.refresh(current);
                if (restore) this.store.restoreScrollOffset(this.offsets.get(scope) ?? 0);
            }
        } catch (error) {
            if (!this.isReading(ticket)) return;
            if (!this.pager.fail(ticket)) throw error;
            this.failed = true;
            if ((error as { code?: number }).code === CodeWarehousePrivilegeRequired && scope === 2) {
                this.pager.invalidate(2); this.selectedSlot = ''; this.storeKey = ''; this.store.setItems([], this.renderSlot, false);
            }
            this.notice = error instanceof Error ? error.message : '仓库查询失败';
            throw error;
        } finally {
            if (this.isReading(ticket)) { this.reading = null; this.busy = false; this.refreshControls(); }
        }
    }
    private renderBag = (node: Node, value: unknown): void => {
        const item = value as WarehouseItem;
        this.rowText(node, `${this.selectedBag === warehouseId(item.instanceId) ? '● ' : ''}${this.name(item)}\n×${item.quantity}\n${item.bound ? '绑定' : '非绑定'}${item.locked ? ' · 已锁定' : ''}`);
    };
    private renderSlot = (node: Node, value: unknown): void => {
        const row = value as WarehouseSlot, number = (warehouseInteger(row.slot) + 1n).toString();
        this.rowText(node, `${this.selectedSlot === row.slot.toString() ? '● ' : ''}仓位 ${number}\n${row.item ? this.name(row.item) : '空位'}${row.item ? `\n×${row.item.quantity}` : '\n可选择存入'}`);
    };
    private name(item: WarehouseItem): string {
        const value = this.input?.definitions.find(row => row.item_id === item.itemId)?.name ?? '物品资料待同步';
        const characters = Array.from(value); return characters.length > 9 ? characters.slice(0, 8).join('') + '…' : value;
    }
    private selectBag = (_index: number, value: WarehouseItem): void => {
        if (this.busy) return;
        const current = this.current().inventory.find(row => warehouseId(row.instanceId) === warehouseId(value.instanceId));
        this.selectedBag = current ? warehouseId(current.instanceId) : ''; this.selectedFrom = 'bag';
        this.edit('Quantity').string = '1'; this.bag.setRenderer(this.renderBag); this.refreshControls();
    };
    private selectSlot = (_index: number, value: WarehouseSlot): void => {
        if (this.busy) return;
        const row = this.page && warehouseSlots(this.page).find(row => row.slot.equals(value.slot));
        this.selectedSlot = row ? row.slot.toString() : ''; this.selectedFrom = 'store';
        this.edit('Quantity').string = '1'; this.store.setRenderer(this.renderSlot); this.refreshControls();
    };
    private refreshControls(): void {
        if (this.disposed || this.retired || !this.input) return;
        const page = this.page, ready = this.input.ready && !this.input.writeBlocked && !this.busy && !this.failed && !!page;
        this.text('Personal/Label', this.scope === 1 ? '● 个人仓库' : '个人仓库'); this.text('Shared/Label', this.scope === 2 ? '● 共享仓库' : '共享仓库');
        this.text('Message', this.busy ? '正在读取当前角色仓库' : this.notice || !page ? this.notice || '请查询当前仓库'
            : this.scope === 1 ? `个人容量 ${page.capacity} 格\n不存货币 · 不与其它堆叠自动合并` : `金币 ${page.gold}\n元宝 ${warehouseMoneyText(page.yuanbaoUnits)}`);
        const pageNumber = page ? (warehouseInteger(page.startSlot) / BigInt(page.limit) + 1n).toString() : '';
        this.text('Page', page ? `第${pageNumber.length > 12 ? pageNumber.slice(0, 12) + '\n' + pageNumber.slice(12) : pageNumber}页` : '等待分页');
        this.text('Gold/Label', this.currency === 1 ? '● 金币' : '金币'); this.text('Yuanbao/Label', this.currency === 2 ? '● 元宝' : '元宝');
        for (const name of ['Gold', 'Yuanbao', 'AmountLabel', 'Amount', 'DepositMoney', 'WithdrawMoney']) this.saved(name).active = this.scope === 2;
        for (const name of ['PersonalHint', 'Expand']) this.saved(name).active = this.scope === 1;
        for (const name of ['Personal', 'Shared', 'Refresh']) this.saved(name).getComponent(Button)!.interactable = this.input.ready && !this.busy;
        this.saved('Previous').getComponent(Button)!.interactable = ready && !page!.startSlot.isZero();
        this.saved('Next').getComponent(Button)!.interactable = ready && warehouseInteger(page!.startSlot) + BigInt(page!.limit) < warehouseInteger(page!.capacity);
        for (const name of ['DepositItem', 'WithdrawItem', 'DepositMoney', 'WithdrawMoney', 'Expand', 'Details']) this.saved(name).getComponent(Button)!.interactable = ready;
        this.edit('Quantity').enabled = ready; this.edit('Amount').enabled = ready;
    }
    private rememberOffset(): void { if (this.input && isValid(this.store, true)) this.offsets.set(this.scope, this.store.scrollView?.getScrollOffset().y ?? 0); }
    private isReading(ticket: WarehouseTicket): boolean {
        return !this.disposed && this.reading === ticket && this.current().attachment === ticket.attachment && this.scope === ticket.query.scope;
    }
    private saved(path: string): Node { const node = this.panel.getChildByPath(path); if (!node) throw new Error('仓库缺少保存控件：' + path); return node; }
    private text(path: string, value: string): void { this.saved(path).getComponent(Label)!.string = value; }
    private edit(path: string): EditBox { return this.saved(path).getComponent(EditBox)!; }
    private rowText(node: Node, text: string): void { node.getChildByName('Text')!.getComponent(Label)!.string = text; }
    private retire = (): void => { this.retired = true; this.disposed = true; this.reading = null; this.pager.clear(); this.input = null; };
    dispose(): void {
        this.disposed = true; this.reading = null; this.pager.clear(); this.input = null;
        if (!this.retired) {
            if (isValid(this.bag.node, true)) this.bag.node.off(List.EVENT_ITEM_CLICK, this.selectBag, this);
            if (isValid(this.store.node, true)) this.store.node.off(List.EVENT_ITEM_CLICK, this.selectSlot, this);
            if (isValid(this.panel, true)) this.panel.off(Node.EventType.NODE_DESTROYED, this.retire, this);
        }
        this.offsets.clear();
    }
}

