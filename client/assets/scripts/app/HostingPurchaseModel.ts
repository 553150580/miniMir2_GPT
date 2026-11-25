import type { PotionInteger } from './PotionModel';
import { assetId, assetInteger } from './SkillAssetsModel';

// 单次购买和购买前余额使用同一个 24 小时边界，买后的累计余额可以超过它。
export const hostingPurchaseLimit = 24 * 60 * 60;

export interface HostingPurchaseContext {
    scope: string; level: number; sceneEpoch: PotionInteger; ruleVersion: PotionInteger; revision: PotionInteger;
    balance: number; boundGold: PotionInteger; gold: PotionInteger;
}

export interface HostingQuote {
    quoteId: Uint8Array; sceneEpoch: PotionInteger; roleRevision: PotionInteger; ruleVersion: PotionInteger; configHash: string;
    quoteVersion: PotionInteger; bandMinLevel: number; bandMaxLevel: number; granularitySeconds: number; requestedSeconds: number;
    referenceGoldPerHour: PotionInteger; goldPerHour: PotionInteger; totalGold: PotionInteger; beforeSeconds: PotionInteger;
    boundGoldCost: PotionInteger; goldCost: PotionInteger; expiresAtUnixMs: PotionInteger; errorCode?: number;
}

export interface HostingPurchaseRequest { requestedSeconds: number; quoteId: Uint8Array; }
export interface HostingPurchaseReceipt { quote: HostingQuote; balanceSeconds: PotionInteger; }

// 保留零散秒数，避免把 23 小时 59 分 59 秒显示成已满 24 小时。
export function hostingDuration(seconds: number): string {
    const hours = Math.floor(seconds / 3600), minutes = Math.floor(seconds % 3600 / 60), rest = seconds % 60;
    return `${hours ? `${hours}小时` : ''}${minutes ? `${minutes}分` : ''}${rest || !seconds ? `${rest}秒` : ''}`;
}

export function hostingPurchaseStep(quote: HostingQuote): number {
    const step = quote.granularitySeconds;
    if (!Number.isInteger(step) || step < 1 || step > hostingPurchaseLimit || assetInteger(quote.quoteVersion) < 1n)
        throw new Error('托管购买档位无效，请重新查询');
    return step;
}

// 报价和回执共享同一套整数运算。服务器原金额仍是最终交易依据。
function hostingQuoteAmounts(quote: HostingQuote) {
    const step = hostingPurchaseStep(quote), seconds = quote.requestedSeconds;
    const reference = assetInteger(quote.referenceGoldPerHour), hourly = assetInteger(quote.goldPerHour);
    const total = assetInteger(quote.totalGold), bound = assetInteger(quote.boundGoldCost), gold = assetInteger(quote.goldCost);
    const before = assetInteger(quote.beforeSeconds);
    if (!Number.isInteger(seconds) || seconds < 1 || seconds > hostingPurchaseLimit || seconds % step !== 0 || reference <= 0n
        || hourly !== reference * 2n || total !== (hourly * BigInt(seconds) + 3599n) / 3600n || total !== bound + gold
        || before >= BigInt(hostingPurchaseLimit)) throw new Error('托管补时报价或到账金额无效');
    return { reference, hourly, total, bound, gold, before, seconds };
}

export function validateHostingQuote(quote: HostingQuote, context: HostingPurchaseContext, seconds: number): HostingPurchaseRequest {
    const money = hostingQuoteAmounts(quote);
    const boundWallet = assetInteger(context.boundGold);
    if (quote.errorCode || quote.requestedSeconds !== seconds || assetInteger(quote.sceneEpoch) !== assetInteger(context.sceneEpoch)
        || assetInteger(quote.roleRevision) !== assetInteger(context.revision) || assetInteger(quote.ruleVersion) !== assetInteger(context.ruleVersion)
        || !quote.configHash || assetInteger(quote.expiresAtUnixMs) <= 0n || context.level < quote.bandMinLevel
        || (quote.bandMaxLevel !== 0 && context.level > quote.bandMaxLevel) || quote.bandMinLevel < 1
        || money.before !== BigInt(context.balance) || money.bound !== (boundWallet < money.total ? boundWallet : money.total)
        || money.gold > assetInteger(context.gold)) throw new Error('托管报价的原角色、档位或钱包已变化，请重新报价');
    assetId(quote.quoteId);
    return { requestedSeconds: seconds, quoteId: new Uint8Array(quote.quoteId) };
}

export function hostingQuoteMessage(quote: HostingQuote): string {
    const money = hostingQuoteAmounts(quote);
    const band = quote.bandMaxLevel ? `${quote.bandMinLevel}–${quote.bandMaxLevel}级` : `${quote.bandMinLevel}级及以上`;
    return [`${band} · 报价版本 ${assetInteger(quote.quoteVersion)}`,
        `标准毛金币 G_ref：${money.reference}金币/小时`,
        `每小时售价（2倍）：${money.hourly}金币`,
        `购买 ${hostingDuration(money.seconds)} · 合计 ${money.total}金币`,
        `先扣绑定金币 ${money.bound}，再扣金币 ${money.gold}`,
        '消耗金币补时，用于离线获取经验和材料'].join('\n');
}

// 原请求或原报价不匹配时保留未决记录，由连接恢复继续核对。
export function hostingReceiptMessage(receipt: HostingPurchaseReceipt, request: HostingPurchaseRequest): string {
    const quote = receipt?.quote;
    if (!quote || quote.errorCode || quote.requestedSeconds !== request.requestedSeconds || assetId(quote.quoteId) !== assetId(request.quoteId))
        throw new Error('补时回执与原报价不一致，保留原请求等待核对');
    const money = hostingQuoteAmounts(quote), balance = assetInteger(receipt.balanceSeconds);
    if (balance !== money.before + BigInt(money.seconds)) throw new Error('补时到账回执不完整，保留原请求等待核对');
    return [`已购 ${hostingDuration(money.seconds)} · 余额 ${hostingDuration(Number(balance))}`,
        `扣绑定金币 ${money.bound}、金币 ${money.gold} · 报价版本 ${assetInteger(quote.quoteVersion)}`].join('\n');
}
