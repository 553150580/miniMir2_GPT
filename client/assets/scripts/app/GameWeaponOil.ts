import { CodeEquipmentChanged, CodeOperationConflict, CodeWeaponOilInputs, CodeWeaponOilQuoteChanged } from '../generated/protocol_ids';
import { equipmentCurrentLuck } from './EquipmentModel';
import type { ItemActionView } from './GameItemActions';
import { assetId } from './SkillAssetsModel';
import { validateWeaponOilQuote, weaponOilAvailable, weaponOilBlock, weaponOilCommit, weaponOilQuoteLines,
    weaponOilReceipt, weaponOilReceiptLines, weaponOilRequest, weaponOilTarget, weaponOilTargets } from './WeaponOilModel';
import type { WeaponOilCommitRequest, WeaponOilQuote, WeaponOilQuoteRequest, WeaponOilReceipt, WeaponOilSnapshot, WeaponOilTarget } from './WeaponOilModel';

type OilRead = () => ItemActionView | null;

interface OilCommands {
    snapshot(): WeaponOilSnapshot | null;
    newSession(): Uint8Array;
    open(read: OilRead): void;
    current(read: OilRead): boolean;
    refresh(): void;
    confirm(title: string, message: string, action?: string): Promise<boolean>;
    quote(request: WeaponOilQuoteRequest): Promise<WeaponOilQuote>;
    submit(request: WeaponOilCommitRequest): Promise<void>;
}

interface OilView {
    readonly scope: string;
    readonly role: string;
    readonly sessionId: Uint8Array;
    readonly read: OilRead;
    target: WeaponOilTarget;
    allowBound: boolean;
    review?: { request: WeaponOilQuoteRequest; quote: WeaponOilQuote };
    result?: WeaponOilReceipt;
    notice: string;
}

// 培养界面复用物品详情的已有按钮与列表。绑定开关只属于本次打开，临时确认和换选武器不换会话。
export class GameWeaponOil {
    private view?: OilView;

    constructor(private readonly commands: OilCommands) {}

    open(target: WeaponOilTarget): void {
        const snapshot = this.commands.snapshot();
        if (!snapshot || snapshot.blocked || !weaponOilTarget(snapshot, target)) throw new Error('当前武器不能打开祝福油培养');
        const view: OilView = { scope: snapshot.scope, role: snapshot.role, sessionId: this.commands.newSession(),
            read: () => this.render(view), target: copyTarget(target), allowBound: false, notice: '' };
        this.view = view;
        this.commands.open(view.read);
    }

    reconcile(request: WeaponOilCommitRequest, receipt: WeaponOilReceipt): WeaponOilReceipt {
        const view = this.view;
        return weaponOilReceipt(request, receipt, view && this.matches(view, request) ? view.review?.quote : undefined);
    }

    accept(role: string, request: WeaponOilCommitRequest, receipt: WeaponOilReceipt): void {
        const view = this.view;
        if (!view || role !== view.role || !this.matches(view, request) || !this.commands.current(view.read) ||
            this.commands.snapshot()?.scope !== view.scope) return;
        view.result = receipt;
        view.review = undefined;
        view.notice = '';
        this.commands.refresh();
    }

    private matches(view: OilView, request: WeaponOilCommitRequest): boolean {
        const quote = view.review?.quote;
        return !!quote && assetId(quote.quoteId) === assetId(request.quoteId) && assetId(view.sessionId) === assetId(request.uiSessionId);
    }

    private render(view: OilView): ItemActionView | null {
        const snapshot = this.commands.snapshot();
        if (!snapshot || snapshot.scope !== view.scope || this.view !== view) return null;
        const name = (itemId: number): string => snapshot.catalog.find(row => row.item_id === itemId)?.name ?? `物品${itemId}`;
        const current = weaponOilTarget(snapshot, view.target);
        const reason = weaponOilBlock(snapshot, view.target, view.allowBound);
        const review = view.review;
        const valid = !!review && this.validReview(review, snapshot);
        const stock = weaponOilAvailable(snapshot);
        const lines = view.result ? weaponOilReceiptLines(view.result, name) : review ? weaponOilQuoteLines(review.quote, name) :
            [...(current ? [`当前武器 ${name(current.itemId)}`, `${view.target.equipped ? '穿戴中' : '背包'} · 幸运+${equipmentCurrentLuck(current)}`] : []),
                `非绑油 ${stock.free}瓶`, `绑定油 ${stock.bound}瓶`, `使用绑定资源 ${view.allowBound ? '已开启' : '未开启'}`,
                '查看报价后确认本次原油堆'];
        if (reason) lines.push(reason);
        if (review && !valid) lines.unshift('报价已变化，请重新报价');
        if (view.notice) lines.unshift(view.notice);
        const actions = [
            { label: valid ? '使用祝福油' : review ? '重新报价' : view.result ? '查看下一瓶' : '查看报价',
                enabled: !reason, run: () => review && valid ? this.useOil(view, review) : this.readQuote(view) },
            { label: view.allowBound ? '关闭绑材' : '开启绑材', enabled: !snapshot.blocked, run: () => this.changeBound(view) },
        ];
        if (review || view.result) actions.push({ label: '返回选武器', enabled: !snapshot.blocked, run: async () => {
            if (!this.active(view)) return;
            view.review = undefined;
            view.result = undefined;
            view.notice = '';
            this.commands.refresh();
        } });
        const options = !review && !view.result ? weaponOilTargets(snapshot).map(({ target, item }) => ({
            key: `${target.equipped ? '穿戴' : '背包'}:${assetId(target.instanceId)}`,
            label: `${sameTarget(view.target, target) ? '已选' : '选择'} ${target.equipped ? '穿戴' : '背包'} ${name(item.itemId)} +${equipmentCurrentLuck(item)}`,
            enabled: !snapshot.blocked && !sameTarget(view.target, target),
            choose: () => this.selectTarget(view, target),
        })) : undefined;
        return { title: view.result ? '祝福油结算' : review ? '祝福油报价' : '武器祝福油', lines, actions, options };
    }

    private active(view: OilView): WeaponOilSnapshot | undefined {
        const snapshot = this.commands.snapshot();
        if (this.view === view && this.commands.current(view.read) && snapshot?.scope === view.scope && !snapshot.blocked) return snapshot;
        return undefined;
    }

    private selectTarget(view: OilView, target: WeaponOilTarget): void {
        const snapshot = this.active(view);
        if (!snapshot || !weaponOilTarget(snapshot, target)) return;
        view.target = copyTarget(target);
        view.review = undefined;
        view.result = undefined;
        view.notice = '';
        this.commands.refresh();
    }

    private async changeBound(view: OilView): Promise<void> {
        if (!this.active(view)) return;
        if (!view.allowBound) {
            if (!await this.commands.confirm('开启绑定祝福油',
                '开启后优先使用绑定油\n使用绑定油会绑定原武器\n绑定后不能交易或共享存仓', '继续开启') || !this.active(view)) return;
            if (!await this.commands.confirm('再次确认绑定投入',
                '失败、回退也会绑定武器\n只累积保底进度也会绑定\n本面板重开后默认关闭', '确认开启') || !this.active(view)) return;
        }
        view.allowBound = !view.allowBound;
        view.review = undefined;
        view.result = undefined;
        view.notice = '';
        this.commands.refresh();
    }

    private async readQuote(view: OilView): Promise<void> {
        const snapshot = this.active(view);
        const request = snapshot && weaponOilRequest(snapshot, view.target, view.allowBound, view.sessionId);
        if (!request) return;
        view.notice = '';
        view.review = undefined;
        view.result = undefined;
        try {
            const quote = await this.commands.quote(request);
            const current = this.active(view);
            if (!current || !this.sameTerms(view, request)) return;
            validateWeaponOilQuote(quote, request, current);
            view.review = { request, quote };
        } catch (error) {
            if (!this.active(view) || !this.sameTerms(view, request)) return;
            view.notice = error instanceof Error ? error.message : '祝福油报价未确认，请重试';
        }
        this.commands.refresh();
    }

    private sameTerms(view: OilView, request: WeaponOilQuoteRequest): boolean {
        return sameTarget(view.target, request.weapon) && view.allowBound === request.allowBound;
    }

    private validReview(review: NonNullable<OilView['review']>, snapshot: WeaponOilSnapshot): boolean {
        try { validateWeaponOilQuote(review.quote, review.request, snapshot); return true; }
        catch { return false; }
    }

    private async useOil(view: OilView, review: NonNullable<OilView['review']>): Promise<void> {
        const original = this.active(view);
        if (!original || view.review !== review || !this.validReview(review, original)) return;
        // 报价页已完整展示结果与绑定，本按钮提交明确看到的同一报价。
        try { await this.commands.submit(weaponOilCommit(review.quote)); }
        catch (error) {
            const code = (error as { code?: number })?.code;
            if (code !== CodeWeaponOilInputs && code !== CodeWeaponOilQuoteChanged && code !== CodeEquipmentChanged && code !== CodeOperationConflict) throw error;
            if (!this.active(view) || view.review !== review) return;
            view.review = undefined;
            view.notice = '原武器或油堆已变化，请重新报价';
            this.commands.refresh();
        }
    }
}

function sameTarget(left: WeaponOilTarget, right: WeaponOilTarget): boolean {
    return left.equipped === right.equipped && assetId(left.instanceId) === assetId(right.instanceId);
}

function copyTarget(target: WeaponOilTarget): WeaponOilTarget {
    return { equipped: target.equipped, instanceId: new Uint8Array(target.instanceId) };
}
