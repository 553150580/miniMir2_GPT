import { CodeOperationConflict, CodeWingInputs, CodeWingQuoteChanged } from '../generated/protocol_ids';
import { assetId } from './SkillAssetsModel';
import type { ItemActionView } from './GameItemActions';
import { nextWingCost, validateWingQuote, validateWingReceipt, WingFeatherSelection, wingAppearanceAction,
    wingFeatherLabel, wingGrowthAction, wingSpendLines, wingStateLines } from './WingModel';
import type { WingActionRequest, WingQuote, WingQuoteRequest, WingReceipt, WingSnapshot, WingState } from './WingModel';

type WingRead = () => ItemActionView | null;
type Page = 'overview' | 'materials' | 'appearance' | 'receipt';

interface WingCommands {
    snapshot(): WingSnapshot | null;
    newSession(): Uint8Array;
    open(read: WingRead): void;
    current(read: WingRead): boolean;
    refresh(): void;
    confirm(title: string, message: string, action?: string): Promise<boolean>;
    quote(request: WingQuoteRequest): Promise<WingQuote>;
    submit(request: WingActionRequest): Promise<void>;
    openLuck?(wing: WingState): void;
}

interface Review { readonly request: WingQuoteRequest; readonly quote: WingQuote; }
interface Appearance { readonly original: WingState; tier: number; hidden: boolean; }
interface Submission { readonly request: WingActionRequest; readonly expected: WingQuote | WingState; }
interface WingView {
    readonly scope: string;
    readonly role: string;
    readonly sessionId: Uint8Array;
    readonly read: WingRead;
    readonly selected: WingFeatherSelection;
    page: Page;
    review?: Review;
    appearance?: Appearance;
    submitted?: Submission;
    receipt?: WingReceipt;
    notice: string;
}

// 角色翅膀沿用保存的物品详情、滚动列表、数量按钮和确认窗。
export class GameWings {
    private view?: WingView;

    constructor(private readonly commands: WingCommands) {}

    open(): void {
        const snapshot = this.commands.snapshot();
        if (!snapshot) return;
        const view: WingView = { scope: snapshot.scope, role: snapshot.role, sessionId: this.commands.newSession(),
            read: () => this.render(view), selected: new WingFeatherSelection(), page: 'overview', notice: '' };
        this.view = view;
        this.commands.open(view.read);
    }

    reconcile(request: WingActionRequest, receipt: WingReceipt): WingReceipt {
        const submitted = this.view?.submitted;
        return validateWingReceipt(request, receipt, submitted && sameAction(request, submitted.request) ? submitted.expected : undefined);
    }

    accept(role: string, request: WingActionRequest, receipt: WingReceipt): void {
        const view = this.view;
        if (!view || view.role !== role || !view.submitted || !sameAction(request, view.submitted.request) ||
            !this.commands.current(view.read) || this.commands.snapshot()?.scope !== view.scope) return;
        view.submitted = undefined;
        view.review = undefined;
        view.appearance = undefined;
        view.selected.reset();
        view.receipt = receipt;
        view.page = 'receipt';
        view.notice = '';
        this.commands.refresh();
    }

    private render(view: WingView): ItemActionView | null {
        const snapshot = this.commands.snapshot();
        if (!snapshot || snapshot.scope !== view.scope || this.view !== view) return null;
        if (view.page === 'receipt' && view.receipt) return this.renderReceipt(view, snapshot, view.receipt);
        if (view.page === 'appearance' && view.appearance) return this.renderAppearance(view, snapshot, view.appearance);
        if (view.page === 'materials') return view.review ? this.renderQuote(view, snapshot, view.review) : this.renderMaterials(view, snapshot);
        const cost = nextWingCost(snapshot);
        const lines = [...wingStateLines(snapshot.wing), ...(cost ? [
            `${snapshot.wing ? '下一阶' : '激活'}消耗 ${cost}根羽毛`, '激活和升阶必成',
        ] : [(snapshot.wing?.rank ?? 0) >= snapshot.rules.maxRank ? '翅膀已满阶' : '下一阶羽毛费用待开放'])];
        const wing = snapshot.wing;
        return { title: '翅膀', lines, actions: [
            { label: snapshot.wing ? '翅膀升阶' : '激活翅膀', enabled: !!cost && !snapshot.blocked, run: () => this.showPage(view, 'materials') },
            ...(snapshot.wing ? [{ label: '外观设置', enabled: !snapshot.blocked, run: () => this.showPage(view, 'appearance') }] : []),
            ...(wing && snapshot.allowLuckOil && this.commands.openLuck ? [{ label: '祝福油培养', enabled: !snapshot.blocked, run: () => {
                const current = this.active(view)?.wing;
                if (current && sameVersion(wing, current)) this.commands.openLuck?.(current);
            } }] : []),
        ] };
    }

    private renderMaterials(view: WingView, snapshot: WingSnapshot): ItemActionView {
        const selected = view.selected, cost = nextWingCost(snapshot), total = selected.total(snapshot);
        const stock = selected.totals(snapshot), focus = selected.focused(snapshot), count = focus ? selected.count(snapshot, focus) : 0n;
        const lines = [`目标 ${(snapshot.wing?.rank ?? 0) + 1}阶 · 角色专属`, `已选羽毛 ${total}/${cost}根`,
            `非绑 ${stock.free}根 · 绑定 ${stock.bound}根`, `使用绑定资源 ${selected.allowBound ? '已开启' : '未开启'}`,
            ...(focus ? ['用下方加减调整当前堆'] : ['轻触羽毛堆选择原料'])];
        if (view.notice) lines.unshift(view.notice);
        return { title: '选择羽毛', lines, actions: [
            { label: '查看报价', enabled: !!selected.request(snapshot, view.sessionId), run: () => this.readQuote(view) },
            { label: selected.allowBound ? '关闭绑材' : '开启绑材', enabled: !snapshot.blocked, run: () => this.changeBound(view) },
            { label: '返回翅膀', enabled: !snapshot.blocked, run: () => this.showPage(view, 'overview') },
        ], options: selected.options(snapshot).map(item => ({
            key: assetId(item.instanceId),
            label: `${focus && assetId(focus.instanceId) === assetId(item.instanceId) ? '● ' : ''}${wingFeatherLabel(item)}${selected.count(snapshot, item) ? ` 取${selected.count(snapshot, item)}` : ''}`,
            enabled: !snapshot.blocked, choose: () => {
                const current = this.active(view);
                if (current) { selected.pick(current, item); view.notice = ''; }
            },
        })), quantity: focus ? {
            value: count.toString(),
            minus: !snapshot.blocked && count > 0n ? () => this.adjust(view, -1n) : undefined,
            plus: !snapshot.blocked && total < cost && count < BigInt(focus.quantity.toString()) ? () => this.adjust(view, 1n) : undefined,
        } : undefined };
    }

    private renderQuote(view: WingView, snapshot: WingSnapshot, review: Review): ItemActionView {
        const { quote } = review, valid = this.validReview(review, snapshot);
        const lines = [`翅膀 ${quote.wing?.rank ?? 0} → ${quote.targetRank}阶`, `结算外观 ${quote.targetAppearanceTier}阶`,
            '成功率 100% · 角色专属', quote.wing ? '幸运、累计用油与显隐保留' : '初始幸运+0', ...wingSpendLines(quote.feathers)];
        if (!valid) lines.unshift('报价已变化，请重新报价');
        return { title: '翅膀成长报价', lines, actions: [
            { label: valid ? quote.wing ? '确认升阶' : '确认激活' : '重新报价',
                enabled: !!view.selected.request(snapshot, view.sessionId),
                run: () => valid ? this.confirmGrowth(view, review) : this.readQuote(view) },
            { label: view.selected.allowBound ? '关闭绑材' : '开启绑材', enabled: !snapshot.blocked, run: () => this.changeBound(view) },
            { label: '返回选羽毛', enabled: !snapshot.blocked, run: () => this.showPage(view, 'materials') },
        ] };
    }

    private renderAppearance(view: WingView, snapshot: WingSnapshot, appearance: Appearance): ItemActionView {
        const wing = snapshot.wing, fresh = !!wing && sameVersion(appearance.original, wing);
        const canSave = fresh && !!wingAppearanceAction(snapshot, appearance.tier, appearance.hidden);
        const lines = [...wingStateLines(wing), `选中外观 ${appearance.tier}阶 · ${appearance.hidden ? '隐藏' : '显示'}`,
            '外观设置不消耗羽毛', '隐藏外观仍保留属性'];
        if (!fresh) lines.unshift('原翅膀已变化，请返回重选');
        if (view.notice) lines.unshift(view.notice);
        return { title: '翅膀外观', lines, actions: [
            { label: '保存外观', enabled: canSave, run: () => this.saveAppearance(view, appearance) },
            { label: appearance.hidden ? '改为显示' : '改为隐藏', enabled: fresh && !snapshot.blocked,
                run: () => { if (this.active(view)) { appearance.hidden = !appearance.hidden; this.commands.refresh(); } } },
            { label: '返回翅膀', enabled: !snapshot.blocked, run: () => this.showPage(view, 'overview') },
        ], options: snapshot.rules.appearanceTiers.filter(tier => !!wing && tier <= wing.rank).map(tier => ({
            key: `外观:${tier}`, label: `${appearance.tier === tier ? '已选' : '选择'} ${tier}阶外观`,
            enabled: fresh && !snapshot.blocked && appearance.tier !== tier,
            choose: () => { if (this.active(view)) appearance.tier = tier; },
        })) };
    }

    private renderReceipt(view: WingView, snapshot: WingSnapshot, receipt: WingReceipt): ItemActionView {
        const authoritative = !!snapshot.wing && assetId(snapshot.wing.wingId) === assetId(receipt.wing.wingId)
            && snapshot.wing.version.greaterThanOrEqual(receipt.wing.version);
        const more = receipt.action === 1 && authoritative && !!nextWingCost(snapshot);
        const lines = [...wingStateLines(receipt.wing), ...wingSpendLines(receipt.feathers)];
        if (!authoritative) lines.unshift('等待角色原翅膀状态同步');
        const title = receipt.action === 2 ? '翅膀外观已保存' : receipt.wing.rank === 1 ? '翅膀激活完成' : '翅膀升阶完成';
        return { title, lines, actions: [
            { label: more ? '继续升阶' : '返回翅膀', enabled: authoritative && !snapshot.blocked,
                run: () => this.showPage(view, more ? 'materials' : 'overview') },
            ...(more ? [{ label: '返回翅膀', enabled: !snapshot.blocked, run: () => this.showPage(view, 'overview') }] : []),
        ] };
    }

    private active(view: WingView): WingSnapshot | undefined {
        const snapshot = this.commands.snapshot();
        if (this.view === view && this.commands.current(view.read) && snapshot?.scope === view.scope && !snapshot.blocked) return snapshot;
        return undefined;
    }

    private showPage(view: WingView, page: 'overview' | 'materials' | 'appearance'): void {
        const snapshot = this.active(view);
        if (!snapshot || page === 'appearance' && !snapshot.wing) return;
        view.page = page;
        view.review = undefined;
        view.receipt = undefined;
        view.notice = '';
        const wing = snapshot.wing;
        view.appearance = page === 'appearance' && wing ? { original: wing, tier: wing.appearanceTier, hidden: wing.hidden } : undefined;
        this.commands.refresh();
    }

    private adjust(view: WingView, amount: 1n | -1n): void {
        const snapshot = this.active(view);
        if (snapshot) { view.selected.adjust(snapshot, amount); view.notice = ''; }
    }

    private async changeBound(view: WingView): Promise<void> {
        let snapshot = this.active(view);
        if (!snapshot) return;
        if (!view.selected.allowBound) {
            if (!await this.commands.confirm('允许绑定羽毛',
                '允许选择并消耗绑定羽毛\n翅膀始终归当前角色专属', '继续开启') || !this.active(view)) return;
            if (!await this.commands.confirm('再次确认绑定投入',
                '请核对每叠实际投入\n本次打开期间保留开启状态\n重新打开时默认关闭', '确认开启')) return;
            snapshot = this.active(view);
            if (!snapshot) return;
        }
        view.selected.setAllowBound(snapshot, !view.selected.allowBound);
        view.review = undefined;
        view.notice = '';
        this.commands.refresh();
    }

    private async readQuote(view: WingView): Promise<void> {
        const snapshot = this.active(view), request = snapshot && view.selected.request(snapshot, view.sessionId);
        if (!request) return;
        view.review = undefined;
        view.notice = '';
        try {
            const quote = await this.commands.quote(request);
            const current = this.active(view);
            if (!current || view.page !== 'materials' || view.selected.allowBound !== request.allowBound) return;
            validateWingQuote(quote, request, current);
            view.review = { request, quote };
        } catch (error) {
            if (!this.active(view)) return;
            view.notice = error instanceof Error ? error.message : '翅膀报价未确认，请重试';
        }
        this.commands.refresh();
    }

    private validReview(review: Review, snapshot: WingSnapshot): boolean {
        try { validateWingQuote(review.quote, review.request, snapshot); return true; }
        catch { return false; }
    }

    private async confirmGrowth(view: WingView, review: Review): Promise<void> {
        const snapshot = this.active(view);
        if (!snapshot || view.review !== review || !this.validReview(review, snapshot)) return;
        await this.submit(view, { request: wingGrowthAction(review.quote), expected: review.quote });
    }

    private async saveAppearance(view: WingView, appearance: Appearance): Promise<void> {
        const snapshot = this.active(view), wing = snapshot?.wing;
        if (!snapshot || !wing || !sameVersion(appearance.original, wing)) return;
        const request = wingAppearanceAction(snapshot, appearance.tier, appearance.hidden);
        if (request) await this.submit(view, { request, expected: wing });
    }

    private async submit(view: WingView, submitted: Submission): Promise<void> {
        view.submitted = submitted;
        try { await this.commands.submit(submitted.request); }
        catch (error) {
            const code = (error as { code?: number })?.code;
            if (code !== CodeWingInputs && code !== CodeWingQuoteChanged && code !== CodeOperationConflict) throw error;
            if (!this.active(view) || view.submitted !== submitted) return;
            view.submitted = undefined;
            view.review = undefined;
            view.notice = '原翅膀或羽毛已变化，请重新确认';
            this.commands.refresh();
        }
    }
}

function sameVersion(left: WingState, right: WingState): boolean {
    return assetId(left.wingId) === assetId(right.wingId) && left.version.equals(right.version);
}

function sameAction(left: WingActionRequest, right: WingActionRequest): boolean {
    if (left.action !== right.action || !left.wingVersion.equals(right.wingVersion)) return false;
    if (left.action === 1 && right.action === 1) return assetId(left.quoteId) === assetId(right.quoteId) &&
        assetId(left.uiSessionId) === assetId(right.uiSessionId) && left.allowBound === right.allowBound;
    return left.action === 2 && right.action === 2 && assetId(left.wingId) === assetId(right.wingId) &&
        left.appearanceTier === right.appearanceTier && left.hidden === right.hidden;
}
