import { CodeMedalInputs, CodeMedalQuoteChanged, CodeOperationConflict } from '../generated/protocol_ids';
import { assetId } from './SkillAssetsModel';
import type { ItemActionView } from './GameItemActions';
import { MedalFragmentSelection, medalFragmentLabel, medalLevel, medalSlotLabel, medalSpendLines,
    nextMedalLevel, sameMedalId, validateMedalQuote, validateMedalReceipt } from './MedalModel';
import type { MedalCommitRequest, MedalQuote, MedalQuoteRequest, MedalReceipt, MedalSnapshot, MedalState } from './MedalModel';

type MedalRead = () => ItemActionView | null;
type MedalPage = 'overview' | 'materials' | 'receipt';
interface MedalCommands {
    snapshot(): MedalSnapshot | null;
    newSession(): Uint8Array;
    open(read: MedalRead): void;
    current(read: MedalRead): boolean;
    refresh(): void;
    confirm(title: string, message: string, action: string): Promise<boolean>;
    quote(request: MedalQuoteRequest): Promise<MedalQuote>;
    submit(request: MedalCommitRequest, revision: MedalQuote['roleRevision']): Promise<void>;
}
interface Review { readonly request: MedalQuoteRequest; readonly quote: MedalQuote; }
interface Submission { readonly request: MedalCommitRequest; readonly quote: MedalQuote; }
interface MedalView {
    readonly role: string;
    readonly scope: string;
    readonly session: Uint8Array;
    readonly read: MedalRead;
    readonly selection: MedalFragmentSelection;
    page: MedalPage;
    notice: string;
    review?: Review;
    submitted?: Submission;
    receipt?: MedalReceipt;
}

// 勋章使用已保存的详情模板。角色页留在底层，原队列和滚动位置继续保留。
export class GameMedal {
    private view?: MedalView;

    constructor(private readonly commands: MedalCommands) {}

    open(): void {
        const snapshot = this.commands.snapshot();
        if (!snapshot) return;
        const view: MedalView = { role: snapshot.role, scope: snapshot.scope, session: this.commands.newSession(),
            read: () => this.render(view), selection: new MedalFragmentSelection(), page: 'overview', notice: '' };
        this.view = view;
        this.commands.open(view.read);
    }

    reconcile(request: MedalCommitRequest, receipt: MedalReceipt): MedalReceipt {
        const submitted = this.view?.submitted;
        return validateMedalReceipt(request, receipt, submitted && sameCommit(request, submitted.request) ? submitted.quote : undefined);
    }

    accept(role: string, request: MedalCommitRequest, receipt: MedalReceipt): void {
        const view = this.view;
        if (!view?.submitted || view.role !== role || !sameCommit(request, view.submitted.request) ||
            !this.commands.current(view.read) || this.commands.snapshot()?.scope !== view.scope) return;
        view.submitted = undefined;
        view.review = undefined;
        view.selection.reset();
        view.notice = '';
        view.receipt = receipt;
        view.page = 'receipt';
        this.commands.refresh();
    }

    private render(view: MedalView): ItemActionView | null {
        const snapshot = this.commands.snapshot();
        if (!snapshot || this.view !== view || view.scope !== snapshot.scope || view.role !== snapshot.role) return null;
        if (view.page === 'receipt' && view.receipt) return this.renderReceipt(view, snapshot, view.receipt);
        if (view.page === 'materials' && view.review) return this.renderQuote(view, snapshot, view.review);
        if (view.page === 'materials') return this.renderMaterials(view, snapshot);
        const totals = view.selection.inventoryTotals(snapshot);
        const level = snapshot.medal?.level ?? 0, available = nextMedalLevel(snapshot);
        const next = snapshot.rules.levels[level];
        const lines = [...stateLines(snapshot, snapshot.medal), `角色等级上限 ${snapshot.roleLevel}级`,
            `本规则开放至 ${snapshot.rules.levels.length}级`, `背包绑定 ${totals.bound}片 · 非绑 ${totals.free}片`];
        if (next) lines.push(`下一目标 ${next.level}级 · ${next.cost}片`, ...gainLines(snapshot, next.mainAttack, next.defense));
        else lines.push('已达当前开放上限');
        if (next && !available) lines.push(`角色需达 ${next.level}级`);
        if (available && totals.bound + totals.free < available.cost) lines.push('缺口来源 · 世界首领奖励');
        return { title: '角色勋章', lines, actions: [{ label: '选择碎片', enabled: !!available && !snapshot.blocked,
            run: () => this.showMaterials(view) }] };
    }

    private renderMaterials(view: MedalView, snapshot: MedalSnapshot): ItemActionView {
        const selected = view.selection.selectedTotals(snapshot), totals = view.selection.inventoryTotals(snapshot);
        const next = nextMedalLevel(snapshot), cost = next?.cost ?? 0n;
        const lines = [`需要 ${cost}片 · 已选 ${selected.bound + selected.free}片`,
            `选择 绑定 ${selected.bound}片 · 非绑 ${selected.free}片`,
            `背包绑定 ${totals.bound}片 · 非绑 ${totals.free}片`];
        if (next && totals.bound + totals.free < cost) lines.push('缺口来源 · 世界首领奖励');
        if (view.notice) lines.unshift(view.notice);
        const focus = view.selection.focused(snapshot), count = focus ? view.selection.count(focus) : 0n;
        return { title: '勋章碎片', lines, actions: [
            { label: '查看升级报价', enabled: !!view.selection.request(snapshot, view.session), run: () => this.readQuote(view) },
            { label: view.selection.allowBound ? '关闭绑片' : '允许绑片', enabled: !snapshot.blocked, run: () => this.changeBound(view) },
            { label: '返回勋章', enabled: true, run: () => this.showOverview(view) },
        ], optionsFirst: true, options: view.selection.options(snapshot).map(item => ({
            key: assetId(item.instanceId), label: `${medalFragmentLabel(item)} · 已选${view.selection.count(item)}`,
            enabled: !snapshot.blocked && !!next,
            choose: () => {
                const current = this.active(view);
                if (current) { view.selection.pick(current, item); view.notice = ''; }
            },
        })), quantity: focus ? {
            value: count.toString(),
            minus: !snapshot.blocked && count > 0n ? () => this.adjust(view, -1n) : undefined,
            plus: !snapshot.blocked && selected.bound + selected.free < cost && count < BigInt(focus.quantity.toString())
                ? () => this.adjust(view, 1n) : undefined,
        } : undefined };
    }

    private renderQuote(view: MedalView, snapshot: MedalSnapshot, review: Review): ItemActionView {
        const quote = review.quote, valid = this.validReview(review, snapshot);
        const label = valid ? quote.medal ? '确认升级' : '确认激活' : '重新报价';
        const lines = [`勋章 ${quote.medal?.level ?? 0} → ${quote.targetLevel}级`,
            ...gainLines(snapshot, quote.targetMainAttack.toString(), quote.targetDefense.toString()), '成功率 100% · 角色专属',
            '本次依次消耗', ...medalSpendLines(quote.fragments)];
        if (!valid) lines.unshift('报价已变化，请重新报价');
        return { title: '勋章升级报价', lines, actions: [
            { label, enabled: valid || !!view.selection.request(snapshot, view.session),
                run: () => valid ? this.commit(view, review) : this.readQuote(view) },
            { label: '返回选碎片', enabled: true, run: () => this.showMaterials(view) },
        ] };
    }

    private renderReceipt(view: MedalView, snapshot: MedalSnapshot, receipt: MedalReceipt): ItemActionView {
        const authoritative = !!snapshot.medal && snapshot.medal.version.greaterThanOrEqual(receipt.medal.version);
        const lines = [...stateLines(snapshot, receipt.medal), '本次依次消耗', ...medalSpendLines(receipt.fragments)];
        if (!authoritative) lines.unshift('等待角色勋章状态同步');
        const hasNext = authoritative && !!nextMedalLevel(snapshot);
        return { title: receipt.beforeLevel ? '勋章升级完成' : '勋章激活完成', lines, actions: [
            { label: hasNext ? '继续升级' : '返回勋章', enabled: !hasNext || !snapshot.blocked,
                run: () => hasNext ? this.showMaterials(view) : this.showOverview(view) },
        ] };
    }

    private active(view: MedalView, writing = true): MedalSnapshot | undefined {
        const snapshot = this.commands.snapshot();
        if (this.view === view && this.commands.current(view.read) && snapshot?.scope === view.scope &&
            snapshot.role === view.role && (!writing || !snapshot.blocked)) return snapshot;
        return undefined;
    }

    private showOverview(view: MedalView): void {
        if (!this.active(view, false)) return;
        view.selection.reset();
        view.page = 'overview';
        view.review = undefined;
        view.receipt = undefined;
        view.notice = '';
        this.commands.refresh();
    }

    private showMaterials(view: MedalView): void {
        const snapshot = this.active(view, false);
        if (!snapshot || !nextMedalLevel(snapshot)) return;
        view.page = 'materials';
        view.review = undefined;
        view.receipt = undefined;
        view.notice = '';
        this.commands.refresh();
    }

    private adjust(view: MedalView, amount: bigint): void {
        const snapshot = this.active(view);
        if (snapshot) { view.selection.adjust(snapshot, amount); view.notice = ''; }
    }

    private async changeBound(view: MedalView): Promise<void> {
        if (!this.active(view)) return;
        if (!view.selection.allowBound) {
            if (!await this.commands.confirm('允许绑定碎片', '允许选择背包里的绑定碎片\n勋章成长始终归当前角色',
                '继续开启') || !this.active(view)) return;
            if (!await this.commands.confirm('再次确认绑定投入', '升级会扣除报价中列出的原碎片\n重新打开勋章时默认关闭',
                '确认开启') || !this.active(view)) return;
        }
        view.selection.setAllowBound(!view.selection.allowBound);
        view.notice = '';
        this.commands.refresh();
    }

    private async readQuote(view: MedalView): Promise<void> {
        const snapshot = this.active(view), request = snapshot && view.selection.request(snapshot, view.session);
        if (!request) return;
        view.review = undefined;
        view.notice = '';
        try {
            const quote = await this.commands.quote(request), current = this.active(view);
            if (!current) return;
            validateMedalQuote(quote, request, current);
            view.review = { request, quote };
        } catch (error) {
            if (!this.active(view)) return;
            view.notice = error instanceof Error ? error.message : '勋章报价未确认，请重试';
        }
        this.commands.refresh();
    }

    private validReview(review: Review, snapshot: MedalSnapshot): boolean {
        try { validateMedalQuote(review.quote, review.request, snapshot); return true; }
        catch { return false; }
    }

    private async commit(view: MedalView, review: Review): Promise<void> {
        const snapshot = this.active(view);
        if (!snapshot || view.review !== review || !this.validReview(review, snapshot)) return;
        const quote = review.quote;
        const request = { quoteId: new Uint8Array(quote.quoteId), uiSessionId: new Uint8Array(quote.uiSessionId) };
        const submitted: Submission = { request, quote };
        view.submitted = submitted;
        try { await this.commands.submit(request, quote.roleRevision); }
        catch (error) {
            const code = (error as { code?: number })?.code;
            if (code !== CodeMedalInputs && code !== CodeMedalQuoteChanged && code !== CodeOperationConflict) throw error;
            if (!this.active(view) || view.submitted !== submitted) return;
            view.submitted = undefined;
            view.review = undefined;
            view.notice = '勋章或碎片已变化，请重新确认';
            this.commands.refresh();
        }
    }
}

function sameCommit(left: MedalCommitRequest, right: MedalCommitRequest): boolean {
    return sameMedalId(left.quoteId, right.quoteId) && sameMedalId(left.uiSessionId, right.uiSessionId);
}

function stateLines(snapshot: MedalSnapshot, medal?: MedalState | null): readonly string[] {
    const configured = medalLevel(medal, snapshot.rules);
    return [`${medalSlotLabel(medal)} · 角色专属`,
        configured || !medal ? `当前本职攻击 +${configured?.mainAttack ?? 0} · 物防 +${configured?.defense ?? 0}` : '等级已保存，当前规则未显示该档属性'];
}

function gainLines(snapshot: MedalSnapshot, attack: number | string, defense: number | string): readonly string[] {
    const before = medalLevel(snapshot.medal, snapshot.rules);
    return [`本职攻击 +${before?.mainAttack ?? 0} → +${attack}`, `物防 +${before?.defense ?? 0} → +${defense}`];
}
