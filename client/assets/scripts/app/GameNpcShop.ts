import { Button, isValid, Label, Node } from 'cc';
import { List } from '../ui/List';
import { PopupView } from '../ui/PopupView';
import { assetInteger } from './SkillAssetsModel';
import { sameVendorSelection, vendorActionName, vendorQuoteText, vendorRows, vendorSelection } from './NpcShopModel';
import type { VendorAction, VendorMode, VendorQuote, VendorRow, VendorSelection, VendorSnapshot } from './NpcShopModel';
import type { GameItemActions, ItemActionView } from './GameItemActions';

interface VendorHandlers {
    quote(selection: VendorSelection): Promise<VendorQuote>;
    commit(selection: VendorSelection, quote: VendorQuote): Promise<unknown>;
    report(error: unknown): void;
}

const tabs = [['Buy', 1, '购买'], ['Sell', 2, '出售'], ['Repair', 3, '修理']] as const;

// 商品与原实例由上层读取权威数据；此视图只绑定保存控件、管理选择并展现报价。
export class GameNpcShop {
    readonly panel: Node;
    private readonly list: List;
    private readonly buttons: Array<{ node: Node; handler: () => void }> = [];
    private snapshot?: VendorSnapshot;
    private mode: VendorMode = 1;
    private shopKey = '';
    private selected = '';
    private quantity = 1n;
    private renderKey = '';
    private epoch = 0;
    private busy = false;
    private notice = '';
    private disposed = false;

    constructor(safe: Node, private readonly popup: PopupView, private readonly handlers: VendorHandlers,
        private readonly itemActions?: GameItemActions) {
        const panel = safe.getChildByPath('UIMgr/Panels/NpcShop');
        const list = panel?.getChildByName('ItemsList')?.getComponent(List);
        if (!panel || !list) throw new Error('NPC商店面板尚未完整保存在场景');
        this.panel = panel;
        this.list = list;
        this.list.node.on(List.EVENT_ITEM_CLICK, this.choose, this);
        for (const [path, mode] of tabs) this.bind(`Tabs/${path}`, () => this.changeMode(mode));
        this.bind('Quantity/Minus', () => this.adjust(-1n));
        this.bind('Quantity/Plus', () => this.adjust(1n));
        this.bind('Preview', () => this.runQuote(this.mode === 2 ? 2 : 1));
        this.bind('RepairNormal', () => this.runQuote(3));
        this.bind('RepairSpecial', () => this.runQuote(4));
    }

    open(title: string, shopKey: string): void {
        if (!shopKey) throw new Error('未识别当前NPC商店');
        this.epoch++;
        this.shopKey = shopKey;
        this.mode = 1;
        this.clearSelection();
        this.snapshot = undefined;
        this.busy = false;
        this.notice = '';
        this.text('Title', title);
        this.render(false);
    }

    refresh(snapshot: VendorSnapshot | null): void {
        if (this.disposed || !this.shopKey) return;
        const current = snapshot?.shopKey === this.shopKey && snapshot.scope ? snapshot : undefined;
        if (this.snapshot?.scope !== current?.scope) {
            this.epoch++;
            this.busy = false;
            this.clearSelection();
        }
        this.snapshot = current;
        const row = this.currentRow();
        if (!row) this.clearSelection();
        else if (this.quantity > assetInteger(row.maxQuantity) || this.quantity % this.step(row) !== 0n) this.quantity = this.step(row);
        this.render(true);
    }

    dispose(): void {
        if (this.disposed) return;
        this.disposed = true;
        this.epoch++;
        if (isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, this.choose, this);
        for (const { node, handler } of this.buttons) if (isValid(node, true)) node.off(Button.EventType.CLICK, handler, this);
        this.buttons.length = 0;
    }

    private bind(path: string, action: () => void | Promise<void>): void {
        const node = this.saved(path), handler = (): void => {
            if (!this.panel.activeInHierarchy || this.busy) return;
            const epoch = this.epoch;
            const report = (error: unknown): void => {
                if (this.disposed || this.epoch !== epoch || !this.panel.activeInHierarchy) return;
                this.notice = error instanceof Error ? error.message : String(error);
                this.render(true);
                this.handlers.report(error);
            };
            try { void Promise.resolve(action()).catch(report); }
            catch (error) { report(error); }
        };
        node.on(Button.EventType.CLICK, handler, this);
        this.buttons.push({ node, handler });
    }

    private clearSelection(): void { this.selected = ''; this.quantity = 1n; }
    private step(row: VendorRow): bigint {
        const step = this.mode === 3 ? 1n : assetInteger(row.quantityStep ?? 1);
        return step > 0n ? step : 1n;
    }
    private currentRow(): VendorRow | undefined { return vendorRows(this.snapshot, this.mode).find(row => row.key === this.selected); }
    private changeMode(mode: VendorMode): void {
        if (mode === this.mode) return;
        this.mode = mode;
        this.notice = '';
        this.clearSelection();
        this.render(false);
    }
    private choose = (_index: number, choice: VendorRow): void => {
        if (this.busy || !this.panel.activeInHierarchy) return;
        const row = vendorRows(this.snapshot, this.mode).find(value => value.key === choice.key);
        if (!row) return;
        if (this.selected !== row.key) this.quantity = this.step(row);
        this.selected = row.key;
        this.notice = '';
        this.render(true);
        const epoch = this.epoch, mode = this.mode;
        this.itemActions?.open(() => this.itemDetails(epoch, mode, row.key));
    };

    private itemDetails(epoch: number, mode: VendorMode, key: string): ItemActionView | null {
        const row = this.currentRow();
        if (this.disposed || !this.panel.activeInHierarchy || epoch !== this.epoch || mode !== this.mode || !row || row.key !== key) return null;
        const actions: readonly VendorAction[] = mode === 3 ? row.repairActions ?? [] : [mode === 2 ? 2 : 1];
        const usable = !this.busy && !row.disabledReason && this.snapshot?.ready;
        return {
            title: row.name,
            lines: [row.detail, ...[row.disabledReason || (!this.snapshot?.ready && this.snapshot?.reason), this.notice].filter((line): line is string => !!line),
                ...row.itemDetails ?? []],
            actions: actions.map(action => ({ label: vendorActionName(action), enabled: !this.busy && !!vendorSelection(this.snapshot, mode, row, action, this.quantity),
                run: () => this.runQuote(action) })),
            quantity: mode === 3 ? undefined : {
                value: this.quantity.toString(),
                minus: usable && this.quantity > this.step(row) ? () => this.adjust(-1n) : undefined,
                plus: usable && this.quantity + this.step(row) <= assetInteger(row.maxQuantity) ? () => this.adjust(1n) : undefined,
            },
        };
    }

    private adjust(direction: bigint): void {
        const row = this.currentRow();
        if (!row || this.mode === 3 || row.disabledReason) return;
        const amount = this.quantity + direction * this.step(row);
        if (amount < 1n || amount > assetInteger(row.maxQuantity)) return;
        this.quantity = amount;
        this.render(true);
    }

    private async runQuote(action: VendorAction): Promise<void> {
        const row = this.currentRow(), selection = vendorSelection(this.snapshot, this.mode, row, action, this.quantity);
        if (!selection || !row) return;
        const epoch = this.epoch;
        const stillSelected = (): boolean => this.epoch === epoch && this.panel.activeInHierarchy
            && sameVendorSelection(selection, vendorSelection(this.snapshot, this.mode, this.currentRow(), action, this.quantity));
        this.busy = true;
        this.notice = '';
        this.render(true);
        try {
            const quote = await this.handlers.quote(selection);
            if (!stillSelected()) return;
            if (await this.popup.confirm('Confirm', vendorQuoteText(row.name, selection, quote), `${vendorActionName(action)}确认`) && stillSelected()) {
                await this.handlers.commit(selection, quote);
                this.notice = '操作已确认';
            }
        } finally {
            if (!this.disposed && this.epoch === epoch) { this.busy = false; this.render(true); }
        }
    }

    private render(preserveOffset: boolean): void {
        const rows = vendorRows(this.snapshot, this.mode), row = this.currentRow();
        const key = JSON.stringify([this.mode, this.snapshot?.scope, this.snapshot?.ready, this.snapshot?.reason, this.snapshot?.allowsRepair,
            this.busy, this.notice, this.selected, this.quantity.toString(),
            rows.map(value => [value.key, value.itemId, value.name, value.detail, value.disabledReason,
                value.maxQuantity.toString(), value.quantityStep?.toString(), value.itemVersion?.toString(), value.repairActions])]);
        if (preserveOffset && this.renderKey === key) return;
        this.renderKey = key;
        this.list.setItems(rows, (node, value) => {
            const item = value as VendorRow;
            node.getChildByName('Text')!.getComponent(Label)!.string = `${item.key === this.selected ? '● ' : ''}${item.name}\n${item.detail}`;
        }, preserveOffset);
        for (const [path, mode, text] of tabs) {
            if (mode === 3) this.saved(`Tabs/${path}`).active = this.snapshot?.allowsRepair !== false;
            this.text(`Tabs/${path}/Label`, mode === this.mode ? `● ${text}` : text);
            this.enable(`Tabs/${path}`, !this.busy && mode !== this.mode);
        }
        const repair = this.mode === 3;
        this.saved('Quantity').active = !this.itemActions && !repair;
        this.saved('Preview').active = !this.itemActions && !repair;
        this.saved('RepairNormal').active = !this.itemActions && repair;
        this.saved('RepairSpecial').active = !this.itemActions && repair;
        this.text('Quantity/Value', this.quantity.toString());
        const enabled = !!(row && !row.disabledReason && this.snapshot?.ready && !this.busy);
        this.enable('Quantity/Minus', enabled && !!row && this.quantity > this.step(row));
        this.enable('Quantity/Plus', enabled && !!row && this.quantity + this.step(row) <= assetInteger(row.maxQuantity));
        const actionable = (action: VendorAction): boolean => !this.busy && !!vendorSelection(this.snapshot, this.mode, row, action, this.quantity);
        const action = this.mode === 2 ? 2 : 1;
        this.text('Preview/Label', `${vendorActionName(action)}报价`);
        this.enable('Preview', actionable(action));
        this.enable('RepairNormal', actionable(3));
        this.enable('RepairSpecial', actionable(4));
        this.text('Message', !this.snapshot?.ready ? this.snapshot?.reason || '正在同步商店' : row ? row.disabledReason || row.name
            : rows.length ? repair ? '选择要修理的装备' : this.mode === 2 ? '选择要出售的物品' : '选择要购买的商品'
                : repair ? '当前没有可修理的装备' : this.mode === 2 ? '当前没有可出售的物品' : '当前没有上架商品');
        this.text('Status', this.busy ? '正在核对本次操作' : this.notice);
    }

    private saved(path: string): Node {
        const node = this.panel.getChildByPath(path);
        if (!node) throw new Error(`NPC商店缺少保存节点 ${path}`);
        return node;
    }
    private enable(path: string, enabled: boolean): void { this.saved(path).getComponent(Button)!.interactable = enabled; }
    private text(path: string, value: string): void { this.saved(path).getComponent(Label)!.string = value; }
}
