import { Button, Label, Node } from 'cc';
import type { PopupView } from '../ui/PopupView';
import { CodeHostingPurchaseBalance, CodeHostingPurchaseFunds } from '../generated/protocol_ids';
import { hostingDuration, hostingPurchaseLimit, hostingPurchaseStep, hostingQuoteMessage, validateHostingQuote,
    type HostingPurchaseContext, type HostingPurchaseRequest, type HostingQuote } from './HostingPurchaseModel';

interface HostingViewContext extends HostingPurchaseContext {
    role: string; // 永久角色身份，只允许其自己的成功回执出现在当前面板。
    pending: boolean; // 共用资产账本中的在途或待核对状态。
}

interface HostingActions {
    quote(context: HostingViewContext, seconds: number): Promise<HostingQuote>;
    buy(request: HostingPurchaseRequest): Promise<void>;
    reason(code: number): string;
}

// 托管面板只绑定保存的按钮和标签，原经济操作仍由共用未决账本管理。
export class GameHosting {
    private discovery?: { scope: string; step: number; notice: string };
    private discovering = ''; // 同一个场景同时只查询一次有效报价档位。
    private seconds = 0;
    private busy = false;
    private disposed = false;
    private remaining?: number;
    private problem = '';
    private receipt?: { role: string; message: string };
    private readonly minus: Node;
    private readonly plus: Node;
    private readonly quantity: Node;
    private readonly quoteButton: Node;
    private readonly receiptLabel: Node;
    private readonly messageLabel: Label;

    constructor(private readonly panel: Node, private readonly popup: Pick<PopupView, 'confirm'>,
        private readonly context: () => HostingViewContext | undefined, private readonly actions: HostingActions) {
        this.minus = this.saved('BuyMinus');
        this.plus = this.saved('BuyPlus');
        this.quantity = this.saved('BuyQuantity');
        this.quoteButton = this.saved('BuyQuote');
        this.receiptLabel = this.saved('BuyReceipt');
        this.messageLabel = this.saved('Message').getComponent(Label)!;
    }

    open(): void {
        this.discovery = undefined;
        this.problem = '';
        this.render();
    }

    refresh(remaining?: number): void {
        this.remaining = remaining;
        this.render();
    }

    dispose(): void { this.disposed = true; }

    acceptReceipt(role: string, message: string): void {
        this.receipt = { role, message };
        this.problem = '';
    }

    adjust(direction: -1 | 1): void {
        const context = this.context();
        const step = context && this.availableStep(context);
        if (!context || !step || this.busy || context.pending) return;
        this.seconds = Math.max(step, Math.min(Math.floor(hostingPurchaseLimit / step) * step, this.seconds + direction * step));
        this.problem = '';
        this.render();
    }

    async confirmPurchase(): Promise<void> {
        const context = this.context();
        if (this.disposed || !context || !this.availableStep(context) || !this.panel.activeInHierarchy || this.busy || context.pending
            || context.balance >= hostingPurchaseLimit) return;
        const seconds = this.seconds;
        this.busy = true;
        this.problem = '';
        this.render();
        try {
            const quote = await this.actions.quote(context, seconds);
            const current = this.live(context.scope);
            if (!current) return;
            if (quote.errorCode) throw new Error(this.actions.reason(quote.errorCode));
            validateHostingQuote(quote, current, seconds);
            if (!await this.popup.confirm('Confirm', hostingQuoteMessage(quote), '金币补时确认')) return;
            const final = this.live(context.scope);
            if (!final) return;
            const request = validateHostingQuote(quote, final, seconds);
            await this.actions.buy(request);
        } catch (error) {
            if (this.live(context.scope)) this.problem = error instanceof Error ? error.message : String(error);
            throw error;
        } finally {
            this.busy = false;
            this.render();
        }
    }

    private live(scope: string): HostingViewContext | undefined {
        const context = this.context();
        return !this.disposed && this.panel.activeInHierarchy && context?.scope === scope ? context : undefined;
    }

    private availableStep(context: HostingViewContext): number {
        return this.discovery?.scope === context.scope ? this.discovery.step : 0;
    }

    private async discover(context: HostingViewContext): Promise<void> {
        this.discovering = context.scope;
        try {
            const quote = await this.actions.quote(context, 0);
            if (this.disposed || this.context()?.scope !== context.scope) return;
            const code = quote.errorCode ?? 0;
            // 钱包或剩余时长暂时不符合购买条件时，仍保留服务端当前有效的粒度。
            const available = code === 0 || code === CodeHostingPurchaseBalance || code === CodeHostingPurchaseFunds;
            const step = available ? hostingPurchaseStep(quote) : 0;
            this.discovery = { scope: context.scope, step, notice: code ? this.actions.reason(code) : '' };
            this.seconds = step;
        } catch (error) {
            if (!this.disposed && this.context()?.scope === context.scope)
                this.discovery = { scope: context.scope, step: 0, notice: error instanceof Error ? error.message : '无法查询托管补时' };
        } finally {
            if (this.discovering === context.scope) this.discovering = '';
            this.render();
        }
    }

    private render(): void {
        if (this.disposed) return;
        const context = this.context();
        if (context && this.panel.activeInHierarchy && this.discovery?.scope !== context.scope && this.discovering !== context.scope)
            void this.discover(context);
        const step = context ? this.availableStep(context) : 0;
        const visible = step > 0;
        for (const node of [this.minus, this.plus, this.quantity, this.quoteButton]) node.active = visible;
        const ready = visible && !this.busy && !context?.pending && (context?.balance ?? hostingPurchaseLimit) < hostingPurchaseLimit;
        this.minus.getComponent(Button)!.interactable = ready && this.seconds > step;
        this.plus.getComponent(Button)!.interactable = ready && this.seconds + step <= hostingPurchaseLimit;
        this.quoteButton.getComponent(Button)!.interactable = ready;
        if (visible) this.quantity.getComponent(Label)!.string = `购买 ${hostingDuration(this.seconds)}`;
        this.saved('BuyQuote/Label').getComponent(Label)!.string = this.busy ? '正在确认报价' : '查看补时报价';
        const notice = this.problem || (context?.pending ? '原操作待核对，连接恢复后查询回执'
            : context && this.discovery?.scope === context.scope ? this.discovery.notice : context ? '正在查询金币补时' : '当前场景无法查询金币补时');
        this.messageLabel.string = [this.remaining === undefined ? '等待托管余额同步' : `剩余托管 ${hostingDuration(this.remaining)}`,
            '在线自动战斗不扣托管时间', notice].filter(Boolean).join('\n');
        const receipt = context && this.receipt?.role === context.role ? this.receipt.message : '';
        this.receiptLabel.active = !!receipt;
        this.receiptLabel.getComponent(Label)!.string = receipt;
    }

    private saved(path: string): Node {
        const node = this.panel.getChildByPath(path);
        if (!node) throw new Error(`托管面板缺少保存节点：${path}`);
        return node;
    }
}
