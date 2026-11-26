import type Long from 'long';
import { CodeEquipmentEnhancementInputs, CodeEquipmentEnhancementQuoteChanged, CodeEquipmentEnhancementUnavailable,
    CodeOperationConflict } from '../generated/protocol_ids';
import { equipmentSlotName } from './EquipmentModel';
import { enhancementCommit, enhancementNext, enhancementQuoteLines, enhancementReceiptLines, enhancementReceiptObserved, enhancementSlotLines, enhancementStock,
    enhancementStoneLabel, EquipmentEnhancementSelection, validateEquipmentEnhancementQuote, validateEquipmentEnhancementReceipt } from './EquipmentEnhancementModel';
import type { EquipmentEnhancementCommit, EquipmentEnhancementQuote, EquipmentEnhancementQuoteRequest, EquipmentEnhancementReceipt,
    EquipmentEnhancementSnapshot } from './EquipmentEnhancementModel';
import type { ItemActionView } from './GameItemActions';
import { assetId } from './SkillAssetsModel';

type ReadView = () => ItemActionView | null;

interface EnhancementCommands {
    snapshot(): EquipmentEnhancementSnapshot | null;
    newSession(): Uint8Array;
    open(read: ReadView): void;
    current(read: ReadView): boolean;
    refresh(): void;
    confirm(title: string, message: string, action: string): Promise<boolean>;
    quote(request: EquipmentEnhancementQuoteRequest): Promise<EquipmentEnhancementQuote>;
    submit(request: EquipmentEnhancementCommit, expectedRevision: Long): Promise<void>;
}

interface Review { readonly request: EquipmentEnhancementQuoteRequest; readonly quote: EquipmentEnhancementQuote; }
interface Result { readonly receipt: EquipmentEnhancementReceipt; readonly quote: EquipmentEnhancementQuote; }

interface EnhancementView {
    readonly scope: string;
    readonly role: string;
    readonly sessionId: Uint8Array;
    readonly read: ReadView;
    readonly selection: EquipmentEnhancementSelection;
    slot: number;
    choosingSlot: boolean;
    review?: Review;
    submitted?: Review;
    result?: Result;
    notice: string;
}

// 部位、材料、报价和结算共用保存的物品详情面板，不更动角色列表滚动位置。
export class GameEquipmentEnhancement {
    private view?: EnhancementView;

    constructor(private readonly commands: EnhancementCommands) {}

    open(slot: number): void {
        const snapshot = this.commands.snapshot();
        if (!snapshot?.slots.has(slot)) return;
        const view: EnhancementView = { scope: snapshot.scope, role: snapshot.role, sessionId: this.commands.newSession(),
            read: () => this.render(view), selection: new EquipmentEnhancementSelection(), slot, choosingSlot: false, notice: '' };
        this.view = view;
        this.commands.open(view.read);
    }

    reconcile(request: EquipmentEnhancementCommit, receipt: EquipmentEnhancementReceipt): EquipmentEnhancementReceipt {
        const quote = this.view?.submitted?.quote;
        return validateEquipmentEnhancementReceipt(request, receipt, quote && this.matches(request, quote) ? quote : undefined);
    }

    accept(role: string, request: EquipmentEnhancementCommit, receipt: EquipmentEnhancementReceipt): void {
        const view = this.view, quote = view?.submitted?.quote;
        if (!view || !quote || role !== view.role || !this.matches(request, quote) || !this.commands.current(view.read) ||
            this.commands.snapshot()?.scope !== view.scope) return;
        view.result = { receipt, quote };
        this.clearSelection(view);
        this.commands.refresh();
    }

    private matches(request: EquipmentEnhancementCommit, quote: EquipmentEnhancementQuote): boolean {
        return request.slot === quote.slot && assetId(request.quoteId) === assetId(quote.quoteId) && assetId(request.uiSessionId) === assetId(quote.uiSessionId);
    }

    private active(view: EnhancementView): EquipmentEnhancementSnapshot | undefined {
        const snapshot = this.commands.snapshot();
        if (this.view === view && this.commands.current(view.read) && snapshot?.scope === view.scope && !snapshot.blocked) return snapshot;
        return undefined;
    }

    private render(view: EnhancementView): ItemActionView | null {
        const snapshot = this.commands.snapshot();
        if (!snapshot || this.view !== view || view.scope !== snapshot.scope || !snapshot.slots.has(view.slot)) return null;
        if (view.choosingSlot) return this.renderSlots(view, snapshot);
        if (view.result) return this.renderResult(view, snapshot, view.result);
        if (view.review) return this.renderReview(view, snapshot, view.review);
        return this.renderMaterials(view, snapshot);
    }

    private renderSlots(view: EnhancementView, snapshot: EquipmentEnhancementSnapshot): ItemActionView {
        return { title: '选择强化部位', lines: ['强化等级保存在角色的装备部位'], optionsFirst: true, options:
            Array.from(snapshot.slots).sort(([left], [right]) => left - right).map(([slot, level]) => ({ key: `部位${slot}`, label: `${equipmentSlotName(slot)} +${level}`,
                enabled: !snapshot.blocked, choose: () => this.chooseSlot(view, slot) })),
            actions: [{ label: '返回强化', enabled: !snapshot.blocked, run: () => this.chooseSlot(view, view.slot) }] };
    }

    private renderMaterials(view: EnhancementView, snapshot: EquipmentEnhancementSnapshot): ItemActionView {
        const selection = view.selection, focus = selection.focused(snapshot), cost = enhancementNext(snapshot, view.slot)?.stoneCost;
        const stock = enhancementStock(snapshot), total = selection.total(snapshot), changed = selection.changed(snapshot);
        const lines = [...enhancementSlotLines(snapshot, view.slot), `非绑 ${stock.free}颗 · 绑定 ${stock.bound}颗`,
            `使用绑定强化石 ${selection.allowBound ? '已开启' : '未开启'}`];
        if (cost) lines.push(`已选强化石 ${total}/${cost}颗`, focus ? '用加减调整当前石堆' : '轻触列表选石堆');
        if (changed) lines.unshift('原强化石已变化，请重新选择');
        if (view.notice) lines.unshift(view.notice);
        const count = focus ? selection.count(focus) : 0n;
        return { title: `${equipmentSlotName(view.slot)}强化`, lines, options: cost ? selection.options(snapshot).map(item => {
            const key = assetId(item.instanceId), selected = selection.count(item);
            return { key, label: `${focus && assetId(focus.instanceId) === key ? '● ' : ''}${enhancementStoneLabel(item)}${selected ? ` 取${selected}` : ''}`,
                enabled: !snapshot.blocked && !changed, choose: () => {
                    const current = this.active(view);
                    if (current) { selection.pick(current, view.slot, item); view.notice = ''; }
                } };
        }) : undefined, quantity: focus && !changed ? { value: count.toString(),
            minus: count && !snapshot.blocked ? () => this.adjust(view, -1n) : undefined,
            plus: cost && total < cost && count < BigInt(focus.quantity.toString()) && !snapshot.blocked ? () => this.adjust(view, 1n) : undefined,
        } : undefined, actions: [
            { label: changed ? '重选材料' : '查看报价', enabled: changed ? !snapshot.blocked : !!selection.request(snapshot, view.slot, view.sessionId),
                run: () => changed ? this.resetMaterials(view) : this.readQuote(view) },
            { label: selection.allowBound ? '关闭绑材' : '开启绑材', enabled: !!cost && !snapshot.blocked, run: () => this.changeBound(view) },
            { label: '更换部位', enabled: !snapshot.blocked, run: () => this.selectSlot(view) },
        ] };
    }

    private renderReview(view: EnhancementView, snapshot: EquipmentEnhancementSnapshot, review: Review): ItemActionView {
        const valid = this.validReview(review, snapshot);
        return { title: '强化报价', lines: [...(valid ? [] : ['原报价已变化，请重新选择']), ...enhancementQuoteLines(review.quote)], actions: [
            { label: '确认强化', enabled: valid && !view.submitted, run: () => this.commit(view, review) },
            { label: '重新选石', enabled: !snapshot.blocked && !view.submitted, run: () => this.resetMaterials(view) },
            { label: '更换部位', enabled: !snapshot.blocked && !view.submitted, run: () => this.selectSlot(view) },
        ] };
    }

    private renderResult(view: EnhancementView, snapshot: EquipmentEnhancementSnapshot, result: Result): ItemActionView {
        const synced = enhancementReceiptObserved(snapshot, result.receipt, result.quote.roleRevision);
        return { title: '强化结算', lines: [...enhancementReceiptLines(result.receipt), ...(synced ? [] : ['正在同步原部位和背包'])], actions: [
            { label: '返回部位', enabled: synced && !snapshot.blocked, run: () => this.resetMaterials(view) },
            { label: '更换部位', enabled: synced && !snapshot.blocked, run: () => this.selectSlot(view) },
        ] };
    }

    private clearSelection(view: EnhancementView): void {
        view.selection.clear();
        view.review = undefined;
        view.submitted = undefined;
        view.notice = '';
    }

    private resetMaterials(view: EnhancementView): void {
        if (!this.active(view)) return;
        this.clearSelection(view);
        view.result = undefined;
        this.commands.refresh();
    }

    private selectSlot(view: EnhancementView): void {
        if (!this.active(view)) return;
        this.clearSelection(view);
        view.result = undefined;
        view.choosingSlot = true;
        this.commands.refresh();
    }

    private chooseSlot(view: EnhancementView, slot: number): void {
        if (!this.active(view)?.slots.has(slot)) return;
        view.slot = slot;
        view.choosingSlot = false;
        this.commands.refresh();
    }

    private adjust(view: EnhancementView, delta: -1n | 1n): void {
        const snapshot = this.active(view);
        if (snapshot) view.selection.adjust(snapshot, view.slot, delta);
    }

    private async changeBound(view: EnhancementView): Promise<void> {
        if (!this.active(view)) return;
        if (!view.selection.allowBound) {
            if (!await this.commands.confirm('开启绑定强化石', '开启后可选择绑定强化石\n强化只增加角色部位等级', '继续开启') || !this.active(view)) return;
            if (!await this.commands.confirm('再次确认绑定投入', '失败仍扣除所选强化石\n穿戴装备的绑定保持原样\n重开本面板时默认关闭', '确认开启') || !this.active(view)) return;
        }
        view.selection.allowBound = !view.selection.allowBound;
        this.clearSelection(view);
        this.commands.refresh();
    }

    private async readQuote(view: EnhancementView): Promise<void> {
        const snapshot = this.active(view), request = snapshot && view.selection.request(snapshot, view.slot, view.sessionId);
        if (!request) return;
        view.notice = '';
        try {
            const quote = await this.commands.quote(request);
            const current = this.active(view);
            if (!current || view.slot !== request.slot || view.selection.allowBound !== request.allowBound) return;
            validateEquipmentEnhancementQuote(quote, request, current);
            view.review = { request, quote };
        } catch (error) {
            if (!this.active(view)) return;
            view.notice = error instanceof Error ? error.message : '强化报价未确认，请重试';
        }
        this.commands.refresh();
    }

    private validReview(review: Review, snapshot: EquipmentEnhancementSnapshot): boolean {
        try { validateEquipmentEnhancementQuote(review.quote, review.request, snapshot); return true; }
        catch { return false; }
    }

    private async commit(view: EnhancementView, review: Review): Promise<void> {
        const snapshot = this.active(view);
        if (!snapshot || view.review !== review || view.submitted || !this.validReview(review, snapshot)) return;
        view.submitted = review;
        try {
            await this.commands.submit(enhancementCommit(review.quote), review.quote.roleRevision);
        } catch (error) {
            const code = (error as { code?: number })?.code;
            if (code !== CodeEquipmentEnhancementInputs && code !== CodeEquipmentEnhancementQuoteChanged &&
                code !== CodeEquipmentEnhancementUnavailable && code !== CodeOperationConflict) throw error;
            if (!this.active(view)) return;
            this.clearSelection(view);
            view.notice = error instanceof Error ? error.message : '强化条件已变化，请重新报价';
            this.commands.refresh();
        }
    }
}
