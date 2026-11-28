// 本文件绑定已保存的商城入口和详情组件，确认原报价后才发出购买命令。
import type Long from 'long';
import type { ItemActionView } from './GameItemActions';
import { yuanbaoText } from './SkillAssetsModel';
import { growthShopAvailable, growthShopCurrency, validateGrowthShopCatalog, validateGrowthShopQuote } from './GrowthShopProtocol';
import type { GrowthShopCommit, GrowthShopOffer, GrowthShopQuote, GrowthShopState } from './GrowthShopProtocol';

type Read = () => ItemActionView | null; // 已保存详情面板当前绑定的渲染回调。

interface ShopContext {
    scope: string; // 当前永久角色和场景附着身份。
    scene: Long; // 当前场景编号。
    rule: Long; // 已加载并确认的玩法规则。
    blocked: boolean; // 角色资产槽是否正在重放或不可写。
    yuanbao: Long; // 当前角色普通元宝。
    boundYuanbao: Long; // 当前角色绑定元宝。
}
interface Commands {
    context(): ShopContext | undefined; // 读取当前功能页的场景、钱包与可操作性。
    open(read: Read): void; // 打开原保存列表和按钮。
    current(read: Read): boolean; // 检查详情是否仍为当前弹窗。
    refresh(): void; // 通知详情刷新选项、金额和按钮。
    query(scene: Long, product: number, count: number): Promise<GrowthShopState>; // 读取目录或生成原报价。
    submit(request: GrowthShopCommit, at: Long): Promise<void>; // 复用原经济请求和断线重放。
}
interface ShopView {
    scope: string; // 打开时的角色附着。
    read: Read; // 原保存弹窗使用的同一回调。
    state?: GrowthShopState; // 原目录和当日实际限购。
    quote?: GrowthShopQuote; // 原确认页和当前购买条款。
    loading: boolean; // 当前目录请求是否进行中。
    selected: number; // 玩家选中的商品编号，零时展示目录。
    count: number; // 玩家选中的购买份数。
}

// GameGrowthShop 把原目录和购买状态渲染到编辑器保存的通用详情。
export class GameGrowthShop {
    private view?: ShopView; // 最近一次打开的商城。

    constructor(private readonly commands: Commands) {}

    // open 打开角色所属目录，身份变化时丢弃旧响应。
    async open(): Promise<void> {
        const context = this.commands.context();
        if (!context) return;
        const view: ShopView = { scope: context.scope, read: () => this.render(view), loading: false, selected: 0, count: 1 };
        this.view = view;
        this.commands.open(view.read);
        await this.refreshDirectory(view);
    }

    // confirmedQuote 供原购买回执对拍确切金额。断线后以原已提交身份查账。
    get confirmedQuote(): GrowthShopQuote | undefined { return this.view?.quote; }

    // active 同时检查角色附着、操作页和当前已保存的详情绑定。
    private active(view: ShopView): ShopContext | undefined {
        const context = this.commands.context();
        return this.view === view && context?.scope === view.scope && this.commands.current(view.read) ? context : undefined;
    }

    // request 按原附着获取服务端目录，拒收换图或角色切换后的回执。
    private async request(view: ShopView, product: number, count: number): Promise<GrowthShopState | undefined> {
        const current = this.active(view);
        if (!current) return;
        const result = await this.commands.query(current.scene, product, count);
        if (!this.active(view)) return;
        if (!result.sceneEpoch?.equals(current.scene) || !result.ruleVersion?.equals(current.rule)) throw new Error('商城角色或规则已变化');
        validateGrowthShopCatalog(result);
        return result;
    }

    // refreshDirectory 只更新原角色当前运营日，目录查询不触发任何资产变更。
    private async refreshDirectory(view: ShopView): Promise<void> {
        view.loading = true;
        this.commands.refresh();
        try {
            const next = await this.request(view, 0, 0);
            if (next) view.state = next;
        } finally {
            view.loading = false;
            this.commands.refresh();
        }
    }

    // render 在同一保存面板中依次显示目录、数量和原确认条款。
    private render(view: ShopView): ItemActionView | null {
        const context = this.commands.context();
        if (this.view !== view || context?.scope !== view.scope) return null;
        const state = view.state;
        if (!state) return { title: '成长商城', lines: [view.loading ? '正在查询商品' : '商品目录尚未获取'], actions: [
            { label: '重新查询', enabled: !view.loading, run: () => this.refreshDirectory(view) },
        ] };
        const chosen = state.offers.find(row => row.productId === view.selected);
        if (!chosen) {
            const options = state.offers.map(row => ({ key: String(row.productId), label: row.currency ?
                `${row.name} · ${yuanbaoText(row.priceUnits)}${growthShopCurrency(row.currency)}/份` : `${row.name} · 价格待定`,
            enabled: true, choose: () => { view.selected = row.productId; view.count = 1; } }));
            return { title: '成长商城', lines: [`${state.operatingDay} · 上海运营日`,
                `元宝 ${yuanbaoText(context.yuanbao)} · 绑定元宝 ${yuanbaoText(context.boundYuanbao)}`], options, actions: [
                { label: '刷新商品', enabled: true, run: () => this.refreshDirectory(view) },
            ] };
        }
        const back = { label: '返回商城', enabled: true, run: () => this.back(view) };
        if (!chosen.currency) return { title: chosen.name, lines: ['商品价格待确认'], actions: [back] };
        if (view.quote) {
            const quote = view.quote, terms = quote.terms;
            return { title: '确认购买', lines: [`${chosen.name} ×${terms.itemQuantity}`, '到账为绑定材料',
                `支付 ${yuanbaoText(terms.totalPriceUnits)} ${growthShopCurrency(terms.currency)}`, terms.operatingDay], actions: [
                { label: '确认购买', enabled: !context.blocked, run: () => this.buy(view, quote, state.roleRevision) },
                { label: '返回商品', enabled: true, run: () => { view.quote = undefined; this.commands.refresh(); } },
            ] };
        }
        return this.product(view, chosen, context, back);
    }

    // product 显示所选币种、单价与可购份数，报价费用最终由服务端确认。
    private product(view: ShopView, offer: GrowthShopOffer, context: ShopContext, back: ItemActionView['actions'][number]): ItemActionView {
        const available = growthShopAvailable(offer);
        return { title: offer.name, lines: [`每份 ${offer.itemPerPack} 个 · 绑定材料`,
            `每份 ${yuanbaoText(offer.priceUnits)} ${growthShopCurrency(offer.currency)}`,
            ...(offer.dailyLimit ? [`今日已购 ${offer.dailyBought}/${offer.dailyLimit} 个`] : []),
            ...(available ? [] : ['今日额度已用完'])],
        quantity: available ? { value: String(view.count), minus: view.count > 1 ? () => { view.count--; } : undefined,
            plus: view.count < available ? () => { view.count++; } : undefined } : undefined,
        actions: [{ label: '查看报价', enabled: !context.blocked && available > 0 && view.count <= available,
            run: () => this.quote(view) }, back] };
    }

    // quote 取回并核对服务端原条款，数量的后续修改须重新报价。
    private async quote(view: ShopView): Promise<void> {
        const product = view.selected, count = view.count;
        const next = await this.request(view, product, count);
        if (!next) return;
        view.quote = validateGrowthShopQuote(next.quote, next, product, count);
        view.state = next;
        this.commands.refresh();
    }

    // buy 只提交玩家最后看到的原身份，成功后重新读取材料的当日限额。
    private async buy(view: ShopView, quote: GrowthShopQuote, revision: Long): Promise<void> {
        const current = this.active(view);
        if (!current || current.blocked || view.quote !== quote) return;
        try {
            await this.commands.submit({ quoteId: quote.quoteId, productId: quote.terms.productId, count: quote.terms.count }, revision);
        } finally {
            if (this.active(view)) { view.quote = undefined; this.commands.refresh(); }
        }
        if (!this.active(view)) return;
        this.back(view);
        await this.refreshDirectory(view);
    }

    // back 返回原商城名录，清除原商品和确认条款。
    private back(view: ShopView): void {
        view.selected = 0;
        view.count = 1;
        view.quote = undefined;
        this.commands.refresh();
    }
}
