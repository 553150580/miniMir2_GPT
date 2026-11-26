import { Button, Label, Node } from 'cc';
import Long from 'long';
import type { PopupView } from '../ui/PopupView';
import type { RoleCardBuyRequest, RoleCardDailyReceipt, RoleCardDailyRequest, RoleCardPurchaseReceipt, RoleCardState } from './RoleCardProtocol';

interface RoleCardContext {
    readonly scope: string; // 同一角色的普通场景附着身份，含连接与基线。
    readonly role: string; // 角色永久身份，成功回执只更新该角色的面板。
    readonly ruleVersion: Long; // 当前已确认资源规则版本。
    readonly balance: Long; // 当前非绑定元宝余额。
    readonly pending: boolean; // 原账号资产通道有未确认操作时禁止新操作。
}

interface RoleCardActions {
    query(productId?: number): Promise<RoleCardState>;
    buy(request: RoleCardBuyRequest): Promise<void>;
    claim(request: RoleCardDailyRequest): Promise<void>;
}

const shanghai = new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit',
    day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
const maxDisplayedDate = Long.fromString('8640000000000000');

function yuanbao(units: Long): string {
    const fraction = units.modulo(10);
    return fraction.isZero() ? units.div(10).toString() : `${units.div(10)}.${fraction}`;
}

function cardName(kind: number): string { return kind === 1 ? '特权月卡' : '托管月卡'; }

function cardExpiry(until: Long): string {
    if (until.isZero()) return '未开通';
    if (until.greaterThan(maxDisplayedDate)) return '有效期超出显示范围';
    return `有效期至 ${shanghai.format(new Date(until.toNumber()))}`;
}

// 角色双月卡只绑定编辑器已保存的布局。目录、报价、日领和余额来自当前角色服务端。
export class GameRoleCards {
    private scope = ''; // 已读目录所属场景。角色切换和新票会使旧数据失效。
    private state?: RoleCardState;
    private notice = '';
    private busy = false;
    private checking = false;
    private checked = false; // 同一场景只做一次静默能力探测，重新打开托管允许重试。
    private disposed = false;
    private readonly wallet: Label;
    private readonly privilege: Label;
    private readonly hosting: Label;
    private readonly status: Label;
    private readonly buyPrivilege: Button;
    private readonly buyHosting: Button;
    private readonly claimButton: Button;

    constructor(private readonly entry: Node, private readonly panel: Node,
        private readonly popup: Pick<PopupView, 'confirm'>, private readonly context: () => RoleCardContext | undefined,
        private readonly actions: RoleCardActions) {
        this.wallet = this.label('Wallet');
        this.privilege = this.label('PrivilegeInfo');
        this.hosting = this.label('HostingInfo');
        this.status = this.label('Status');
        this.buyPrivilege = this.button('BuyPrivilege');
        this.buyHosting = this.button('BuyHosting');
        this.claimButton = this.button('Claim');
        this.entry.active = false;
    }

    get available(): boolean { return this.entry.active; }

    // 托管打开时独立探测月卡。没有目录的旧服仍按原托管流程使用。
    discover(): void {
        if (!this.checking) this.checked = false;
        this.sync();
    }

    open(): Promise<void> { return this.reload(false); }

    sync(): void {
        if (this.disposed) return;
        const current = this.context();
        const scope = current?.scope ?? '';
        if (scope !== this.scope) {
            this.scope = scope; this.state = undefined; this.notice = ''; this.checked = false;
            this.checking = false; this.busy = false;
        }
        this.entry.active = !!scope && !!this.state?.products.length;
        const panelOpen = this.panel.activeInHierarchy;
        if (panelOpen) this.render(current);
        if ((panelOpen || this.entry.parent?.activeInHierarchy) && current && !this.checked && !this.checking) void this.reload(true);
    }

    async confirmPurchase(kind: number): Promise<void> {
        const current = this.begin();
        if (!current) return;
        try {
            const selected = this.state?.products.find(item => item.kind === kind);
            if (!selected) return;
            const quoted = await this.actions.query(selected.productId);
            const quote = quoted.quote;
            if (!this.current(current.scope) || !this.panel.activeInHierarchy) return;
            const cardUntil = kind === 1 ? quoted.privilegeUntilUnixMs : quoted.hostingUntilUnixMs;
            if (!quote || !quote.ruleVersion.equals(current.ruleVersion) || !quote.previousUntilUnixMs.equals(cardUntil)) {
                throw new Error('当前月卡条件变化，请重新报价');
            }
            this.state = quoted;
            const title = cardName(kind);
            if (!await this.popup.confirm('Confirm', `${title} ${quote.product.days} 天\n支付 ${yuanbao(quote.product.priceYuanbaoUnits)} 非绑定元宝\n${cardExpiry(quote.previousUntilUnixMs)}`, '月卡购买确认')) return;
            const latest = this.current(current.scope);
            if (!latest || !this.panel.activeInHierarchy || latest.pending || !latest.ruleVersion.equals(quote.ruleVersion)) {
                throw new Error('角色或月卡规则已变化，请重新报价');
            }
            if (latest.balance.lessThan(quote.product.priceYuanbaoUnits)) throw new Error('非绑定元宝余额不足');
            await this.actions.buy({ quoteId: quote.quoteId, productId: selected.productId });
        } catch (error) {
            this.fail(current.scope, error);
        } finally { this.finish(current.scope); }
    }

    async claimDaily(): Promise<void> {
        const current = this.begin();
        if (!current) return;
        try {
            if (!this.state?.privilegeClaimable) return;
            await this.actions.claim({ operatingDay: this.state.operatingDay });
        } catch (error) {
            this.fail(current.scope, error);
        } finally { this.finish(current.scope); }
    }

    acceptPurchase(role: string, receipt: RoleCardPurchaseReceipt): void {
        if (this.context()?.role !== role) return;
        this.notice = `${cardName(receipt.quote.product.kind)}购买成功\n${cardExpiry(receipt.newUntilUnixMs)}`;
        void this.reload(true);
    }

    acceptDaily(role: string, receipt: RoleCardDailyReceipt): void {
        if (this.context()?.role !== role) return;
        this.notice = `已领取 ${yuanbao(receipt.grantedUnits)} 绑定元宝`;
        void this.reload(true);
    }

    dispose(): void { this.disposed = true; }

    private current(scope: string): RoleCardContext | undefined {
        const now = this.context();
        return !this.disposed && now?.scope === scope && this.scope === scope ? now : undefined;
    }

    private begin(): RoleCardContext | undefined {
        this.sync();
        const current = this.current(this.scope);
        if (!current || current.pending || this.busy || this.checking || !this.panel.activeInHierarchy) return undefined;
        this.busy = true; this.notice = ''; this.render(current);
        return current;
    }

    private finish(scope: string): void {
        const current = this.current(scope);
        if (!current) return;
        this.busy = false; this.render(current);
    }

    private fail(scope: string, error: unknown): void {
        if (!this.current(scope)) return;
        this.notice = error instanceof Error ? error.message : String(error);
        void this.reload(true);
    }

    private async reload(quiet: boolean): Promise<void> {
        const current = this.current(this.scope);
        if (!current || this.checking) return;
        this.checked = true; this.checking = true;
        this.render(current);
        try {
            const state = await this.actions.query();
            if (this.current(current.scope)) this.state = state;
        } catch (error) {
            if (!this.current(current.scope)) return;
            if (!quiet) this.notice = error instanceof Error ? error.message : String(error);
        } finally {
            if (this.current(current.scope)) { this.checking = false; this.sync(); }
        }
    }

    private render(current?: RoleCardContext): void {
        if (!this.panel.activeInHierarchy) return;
        this.wallet.string = current ? `非绑定元宝 ${yuanbao(current.balance)}` : '等待余额同步';
        const state = this.state;
        this.privilege.string = `特权月卡\n${state ? cardExpiry(state.privilegeUntilUnixMs) : '等待状态'}`;
        this.hosting.string = `托管月卡\n${state ? cardExpiry(state.hostingUntilUnixMs) : '等待状态'}`;
        const enabled = !!current && !current.pending && !this.busy && !this.checking;
        for (const [kind, button] of [[1, this.buyPrivilege], [2, this.buyHosting]] as const) {
            const product = state?.products.find(item => item.kind === kind);
            button.node.active = !!product;
            button.interactable = enabled && !!product;
            if (product) this.buttonLabel(button).string = `购买 · ${yuanbao(product.priceYuanbaoUnits)} 元宝 / ${product.days} 天`;
        }
        this.claimButton.node.active = !!state?.products.length;
        this.claimButton.interactable = enabled && !!state?.privilegeClaimable;
        this.buttonLabel(this.claimButton).string = state?.privilegeClaimable ? `领取 ${yuanbao(state.dailyPrivilegeUnits)} 绑定元宝` : '今日特权奖励不可领';
        this.status.string = this.notice || (this.checking ? '正在查询月卡' : state ? '' : '当前月卡状态未同步');
    }

    private label(path: string): Label { return this.panel.getChildByPath(path)!.getComponent(Label)!; }
    private button(path: string): Button { return this.panel.getChildByPath(path)!.getComponent(Button)!; }
    private buttonLabel(button: Button): Label { return button.node.getChildByName('Label')!.getComponent(Label)!; }
}
