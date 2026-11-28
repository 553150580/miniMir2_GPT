// 本文件定义商城的权威条款、原请求，以及到账回执和数量校验。
import Long from 'long';
import { assetId } from './SkillAssetsModel';

export interface GrowthShopOffer {
    productId: number; // 独立币种对应的商品编号。
    itemId: number; // 实际发货的材料模板。
    name: string; // 工作簿的展示名。
    currency: number; // 普通元宝为 1，绑定元宝为 2，待开放为零。
    priceUnits: Long; // 每份价格，按十分之一元宝计。
    itemPerPack: number; // 每份所含材料数。
    maxCount: number; // 单次购买份数上限。
    dailyLimit: number; // 同一材料的当日共享上限。
    dailyBought: number; // 当日已到账材料数。
}

export interface GrowthShopTerms {
    productId: number; // 确认单商品编号。
    itemId: number; // 确认单入包材料。
    currency: number; // 确认的实际扣款币种。
    unitPriceUnits: Long; // 原单份价格。
    count: number; // 确认份数。
    itemQuantity: Long; // 确认到账的实际个数。
    totalPriceUnits: Long; // 原全部费用。
    operatingDay: string; // 该报价适用的上海运营日。
    dailyLimit: number; // 原日限额。
    dailyBoughtBefore: number; // 报价时用过的材料额度。
}

export interface GrowthShopQuote {
    quoteId: Uint8Array; // 服务端短期确认编号。
    expiresAtUnixMs: Long; // 首次确认的服务端截止时刻。
    terms: GrowthShopTerms; // 玩家实际看到的购买条款。
}

export interface GrowthShopState {
    sceneEpoch: Long; // 返回目录所针对的当前场景。
    roleRevision: Long; // 服务端生成目录时的资产版本。
    ruleVersion: Long; // 当前规则版本。
    operatingDay: string; // 当前实际上海运营日。
    offers: GrowthShopOffer[]; // 权威在售与待开放目录。
    quote?: GrowthShopQuote; // 指定商品和份数时才返回。
}

export interface GrowthShopCommit {
    quoteId: Uint8Array; // 持久化后重放的原报价身份。
    productId: number; // 原商品编号。
    count: number; // 原选择份数。
}

export interface GrowthShopReceipt {
    quoteId: Uint8Array; // 成交使用的原报价。
    terms: GrowthShopTerms; // 已确认的原条款。
    items: { instanceId: Uint8Array; itemId: number; quantity: Long; bound: boolean; sourceKind: string }[]; // 已到账的绑定堆。
    balanceUnits: Long; // 原确认时该钱包扣款后的余额。
}

const whole = (value: number, allowZero = false) => Number.isInteger(value) && value >= (allowZero ? 0 : 1) && value <= 0xffffffff;
const money = (value: unknown): value is Long => Long.isLong(value) && !value.unsigned && !value.isNegative();
const day = (value: unknown): value is string => typeof value === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(value);

// growthShopCurrency 只格式化实际开放的两个钱包。
export function growthShopCurrency(currency: number): string { return currency === 2 ? '绑定元宝' : '元宝'; }

// growthShopAvailable 按材料数量折算当前还能购买的整份数。
export function growthShopAvailable(offer: GrowthShopOffer): number {
    return offer.dailyLimit ? Math.max(0, Math.min(offer.maxCount, Math.floor((offer.dailyLimit - offer.dailyBought) / offer.itemPerPack))) : offer.maxCount;
}

// validateTerms 通过 Long 和 BigInt 检查价格、到账数和额度各自守恒。
function validateTerms(t: GrowthShopTerms): void {
    if (!t || !whole(t.productId) || !whole(t.itemId) || (t.currency !== 1 && t.currency !== 2) || !whole(t.count) || t.count > 1000 ||
        !money(t.unitPriceUnits) || t.unitPriceUnits.isZero() || !money(t.totalPriceUnits) || !money(t.itemQuantity) || t.itemQuantity.isZero() ||
        !day(t.operatingDay) || !whole(t.dailyLimit, true) || !whole(t.dailyBoughtBefore, true) ||
        BigInt(t.totalPriceUnits.toString()) !== BigInt(t.unitPriceUnits.toString()) * BigInt(t.count) ||
        t.dailyLimit > 0 && BigInt(t.dailyBoughtBefore) + BigInt(t.itemQuantity.toString()) > BigInt(t.dailyLimit)) {
        throw new Error('商城结算条款无效');
    }
}

// validateGrowthShopQuote 对拍目录中的同一商品、币种及购买份数。
export function validateGrowthShopQuote(q: GrowthShopQuote | undefined, state: GrowthShopState, productId: number, count: number): GrowthShopQuote {
    if (!q || !assetId(q.quoteId) || !money(q.expiresAtUnixMs) || q.expiresAtUnixMs.isZero()) throw new Error('商城报价缺失');
    const t = q.terms, offer = state.offers.find(row => row.productId === productId);
    validateTerms(t);
    if (!offer || t.productId !== productId || t.itemId !== offer.itemId || t.currency !== offer.currency || t.count !== count ||
        t.operatingDay !== state.operatingDay || t.dailyLimit !== offer.dailyLimit || t.dailyBoughtBefore !== offer.dailyBought ||
        !t.unitPriceUnits.equals(offer.priceUnits) || BigInt(t.itemQuantity.toString()) !== BigInt(count) * BigInt(offer.itemPerPack)) {
        throw new Error('商城报价与原商品不一致');
    }
    return q;
}

// validateGrowthShopCatalog 检查同一附着下收到的有界目录。
export function validateGrowthShopCatalog(state: GrowthShopState): void {
    if (!state || !Long.isLong(state.roleRevision) || !state.roleRevision.unsigned || state.roleRevision.isZero() || !day(state.operatingDay) ||
        !Array.isArray(state.offers) || state.offers.length === 0 || state.offers.length > 16 || state.offers.some((o, i) =>
            !whole(o.productId) || i > 0 && o.productId <= state.offers[i - 1].productId || !whole(o.itemId) || typeof o.name !== 'string' || !o.name ||
            (o.currency !== 0 && o.currency !== 1 && o.currency !== 2) || !money(o.priceUnits) || (o.currency === 0) !== o.priceUnits.isZero() ||
            !whole(o.itemPerPack) || !whole(o.dailyLimit, true) || !whole(o.dailyBought, true) || !whole(o.maxCount, true) || o.maxCount > 1000 ||
            (o.currency === 0) !== (o.maxCount === 0))) {
        throw new Error('商城商品目录有误');
    }
}

// validateGrowthShopReceipt 确认原购买身份、每个绑定实例和实际到账合计。
export function validateGrowthShopReceipt(value: GrowthShopReceipt, request: GrowthShopCommit, confirmed?: GrowthShopQuote): GrowthShopReceipt {
    if (!value || !assetId(value.quoteId) || assetId(value.quoteId) !== assetId(request.quoteId) || !money(value.balanceUnits) ||
        !Array.isArray(value.items) || value.items.length === 0 || value.items.length > 1000) throw new Error('商城原回执无效');
    const t = value.terms;
    validateTerms(t);
    if (t.productId !== request.productId || t.count !== request.count) throw new Error('商城原份数与确认单不符');
    if (confirmed && assetId(confirmed.quoteId) === assetId(request.quoteId)) {
        const original = confirmed.terms;
        if (t.itemId !== original.itemId || t.currency !== original.currency || !t.unitPriceUnits.equals(original.unitPriceUnits) ||
            !t.itemQuantity.equals(original.itemQuantity) || !t.totalPriceUnits.equals(original.totalPriceUnits) ||
            t.operatingDay !== original.operatingDay || t.dailyLimit !== original.dailyLimit || t.dailyBoughtBefore !== original.dailyBoughtBefore) {
            throw new Error('商城回执与确认页金额或到账条件不同');
        }
    }
    const ids = new Set<string>();
    let total = 0n;
    for (const item of value.items) {
        const id = assetId(item.instanceId);
        if (!id || ids.has(id) || item.itemId !== t.itemId || item.bound !== true || item.sourceKind !== 'growth_shop' ||
            !money(item.quantity) || item.quantity.isZero()) throw new Error('商城材料到账明细无效');
        ids.add(id);
        total += BigInt(item.quantity.toString());
    }
    if (total !== BigInt(t.itemQuantity.toString())) throw new Error('商城实际到账数量与原订单不符');
    return value;
}
