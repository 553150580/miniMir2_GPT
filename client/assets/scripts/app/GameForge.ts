import { CodeForgeInputs, CodeForgeQuoteChanged, CodeOperationConflict, CodeVendorFunds } from '../generated/protocol_ids';
import type { ItemActionView } from './GameItemActions';
import { forgeChance, forgeOreLabel, forgeQuoteLines, forgeReceipt, forgeReceiptLines, forgeRecipeFor, ForgeSelection, validateForgeQuote } from './ForgeModel';
import type { ForgeItem, ForgeQuote, ForgeQuoteRequest, ForgeReceipt, ForgeSnapshot } from './ForgeModel';
import { assetId } from './SkillAssetsModel';

type ForgeRead = () => ItemActionView | null;

interface ForgeCommands {
    snapshot(): ForgeSnapshot | null;
    open(read: ForgeRead): void;
    current(read: ForgeRead): boolean;
    refresh(): void;
    confirm(title: string, message: string, action?: string): Promise<boolean>;
    quote(request: ForgeQuoteRequest): Promise<ForgeQuote>;
    submit(quoteId: Uint8Array): Promise<void>;
}

interface ForgeView {
    readonly role: string;
    readonly selection: ForgeSelection;
    readonly read: ForgeRead;
    quote?: ForgeQuote;
    result?: ForgeReceipt;
    notice: string;
}

// 打造使用保存的详情列表和确认窗。每次打开冻结一个原图纸，新开窗不会继承绑定开关。
export class GameForge {
    private view?: ForgeView;

    constructor(private readonly commands: ForgeCommands) {}

    open(item: ForgeItem): void {
        const snapshot = this.commands.snapshot();
        const recipe = snapshot && forgeRecipeFor(snapshot, item.itemId);
        if (!snapshot || !recipe || snapshot.blocked) throw new Error('当前图纸或角色状态不能打开打造');
        const selection = new ForgeSelection(snapshot.scope, item, recipe);
        if (!selection.blueprint(snapshot)) throw new Error('原图纸已变化或锁定');
        const view: ForgeView = { role: snapshot.role, selection, read: () => this.render(view), notice: '' };
        this.view = view;
        this.commands.open(view.read);
    }

    // 沿永久操作身份重放也核对原报价；当前仍保有原确认窗时进一步对拍钱包和产物。
    reconcile(quoteId: Uint8Array, result: ForgeReceipt): ForgeReceipt {
        const quote = this.view?.quote;
        return forgeReceipt(quoteId, result, quote && assetId(quote.quoteId) === assetId(quoteId) ? quote : undefined);
    }

    accept(role: string, receipt: ForgeReceipt): void {
        const view = this.view;
        if (!view || view.role !== role || !view.quote || assetId(receipt.quoteId) !== assetId(view.quote.quoteId)) return;
        view.result = receipt;
        this.commands.refresh();
    }

    private render(view: ForgeView): ItemActionView | null {
        const snapshot = this.commands.snapshot();
        if (!snapshot || snapshot.scope !== view.selection.scope || this.view !== view) return null;
        const name = (id: number): string => snapshot.catalog.find(item => item.item_id === id)?.name ?? `物品${id}`;
        if (view.result) return { title: view.result.success ? '打造成功' : '打造失败', lines: forgeReceiptLines(view.result, name), actions: [] };
        if (view.quote) return this.renderQuote(view, view.quote, snapshot, name);
        const blueprint = view.selection.blueprint(snapshot);
        const chosen = view.selection.chosen(snapshot);
        const lines = [`图纸 ${name(view.selection.recipe.blueprint_item_id)}`, `产物 ${name(view.selection.recipe.output_item_id)}`,
            `使用绑定资源 ${view.selection.allowBound ? '已开启' : '未开启'}`, `触摸选择黑铁矿 ${chosen.length}/${view.selection.recipe.ore_count}块`];
        if (!blueprint) lines.push('原图纸已变化，请返回背包');
        else if (blueprint.bound && !view.selection.allowBound) lines.push('图纸绑定，需先开启绑定投入');
        if (view.notice) lines.unshift(view.notice);
        return { title: '图纸打造', lines, actions: [
            { label: '查看报价', enabled: !!view.selection.request(snapshot), run: () => this.readQuote(view) },
            { label: view.selection.allowBound ? '关闭绑材' : '开启绑材', enabled: !!blueprint && !snapshot.blocked, run: () => this.useBound(view) },
        ], options: view.selection.options(snapshot).map(ore => {
            const selected = chosen.includes(ore);
            return { key: assetId(ore.instanceId), label: `${selected ? '已选' : '可选'} ${forgeOreLabel(ore)}`,
                enabled: !!blueprint && !snapshot.blocked && (selected || chosen.length < view.selection.recipe.ore_count),
                choose: () => {
                    const current = this.active(view);
                    if (current) { view.selection.toggle(current, ore); view.notice = ''; }
                } };
        }) };
    }

    private renderQuote(view: ForgeView, quote: ForgeQuote, snapshot: ForgeSnapshot, name: (itemId: number) => string): ItemActionView {
        const current = this.validQuote(view, quote, snapshot);
        const lines = forgeQuoteLines(quote, name);
        if (!current) lines.unshift('报价已变化，请重新报价');
        if (view.notice) lines.unshift(view.notice);
        return { title: '打造报价', lines, actions: [
            { label: current ? '确认打造' : '重新报价', enabled: !!view.selection.request(snapshot),
                run: () => current ? this.confirmQuote(view, quote) : this.readQuote(view) },
            { label: '返回选矿', enabled: !snapshot.blocked, run: () => {
                if (this.active(view)) { view.quote = undefined; view.notice = ''; this.commands.refresh(); }
            } },
        ] };
    }

    private active(view: ForgeView): ForgeSnapshot | undefined {
        const snapshot = this.commands.snapshot();
        if (this.view === view && this.commands.current(view.read) && snapshot?.scope === view.selection.scope && !snapshot.blocked) return snapshot;
        return undefined;
    }

    private async useBound(view: ForgeView): Promise<void> {
        let current = this.active(view);
        if (!current || !view.selection.blueprint(current)) return;
        if (!view.selection.allowBound) {
            if (!await this.commands.confirm('使用绑定资源', ['可使用绑定图纸、黑铁矿和金币', '使用任一绑定投入，成功装备归当前角色', '绑定装备无法交易或转入共享仓库'].join('\n'), '继续开启')) return;
            current = this.active(view);
            if (!current || !view.selection.blueprint(current)) return;
            if (!await this.commands.confirm('再次确认绑定投入', ['失败会扣黑铁矿和金币，保留图纸', '本次打开期间不再重复询问', '重新打开时默认关闭'].join('\n'), '确认开启')) return;
            current = this.active(view);
            if (!current || !view.selection.blueprint(current)) return;
        }
        view.selection.setAllowBound(current, !view.selection.allowBound);
        view.notice = '';
        this.commands.refresh();
    }

    private async readQuote(view: ForgeView): Promise<void> {
        const original = this.active(view), request = original && view.selection.request(original);
        view.notice = '';
        if (!request) { view.quote = undefined; this.commands.refresh(); return; }
        try {
            const quote = await this.commands.quote(request);
            const current = this.active(view);
            if (!current) return;
            validateForgeQuote(quote, view.selection, current);
            view.quote = quote;
        } catch (error) {
            if (!this.active(view)) return;
            view.quote = undefined;
            view.notice = error instanceof Error ? error.message : '打造报价未确认，请重试';
        }
        this.commands.refresh();
    }

    private validQuote(view: ForgeView, quote: ForgeQuote, snapshot: ForgeSnapshot): boolean {
        try { validateForgeQuote(quote, view.selection, snapshot); return true; }
        catch { return false; }
    }

    private async confirmQuote(view: ForgeView, quote: ForgeQuote): Promise<void> {
        const original = this.active(view);
        if (!original) return;
        if (!this.validQuote(view, quote, original)) { await this.readQuote(view); return; }
        const summary = [`成功率 ${forgeChance(quote)} · ${quote.outputBound ? '产物绑定' : '产物非绑定'}`, `金币 ${quote.gold} · 绑定金币 ${quote.boundGold}`,
            `消耗已报价的${quote.ores.length}块黑铁矿`, '失败照扣矿石与金币，保留图纸'];
        if (!await this.commands.confirm('确认打造', summary.join('\n'))) return;
        const current = this.active(view);
        if (!current) return;
        if (!this.validQuote(view, quote, current)) { await this.readQuote(view); return; }
        try { await this.commands.submit(new Uint8Array(quote.quoteId)); }
        catch (error) {
            if (!this.active(view)) throw error;
            const code = (error as { code?: number })?.code;
            if (code === CodeForgeQuoteChanged || code === CodeOperationConflict || code === CodeForgeInputs || code === CodeVendorFunds) {
                view.quote = undefined;
                await this.readQuote(view);
                return;
            }
            throw error;
        }
    }
}
