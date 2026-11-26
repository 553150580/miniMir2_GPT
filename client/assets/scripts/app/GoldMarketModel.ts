import Long from 'long';
import { GoldMarketActionKind as Action, GoldMarketSide as Side, type GoldMarketPlace, type GoldMarketRules } from './GoldMarketProtocol';
import { marketAmount, marketMoney } from './ItemMarketPresentation';
import { warehouseMoney } from './WarehouseModel';

export interface GoldMarketPreview {
    readonly request: GoldMarketPlace;
    readonly gold: Long; // 这笔委托覆盖的非绑定金币总量。
    readonly yuanbao: Long; // 该限价下的最高买入支出或挂单总报价。
    readonly confirmation: string;
}

export function goldMarketDuration(seconds: number): string {
    const days = Math.floor(seconds / 86400), hours = Math.floor((seconds % 86400) / 3600), minutes = Math.floor((seconds % 3600) / 60);
    return [days ? `${days}天` : '', hours ? `${hours}小时` : '', minutes ? `${minutes}分钟` : '', seconds % 60 ? `${seconds % 60}秒` : ''].join('');
}

export function goldMarketTerms(rules: GoldMarketRules): string {
    return `参考价 ${marketMoney(rules.referenceUnitPriceYuanbaoUnits)} 元宝/手\n限价 ${marketMoney(rules.minUnitPriceYuanbaoUnits)}～${marketMoney(rules.maxUnitPriceYuanbaoUnits)} 元宝/手`;
}

// 预览与确认之间保留同一条款摘要。所有乘法在 BigInt 下完成，再转换为网络 Long。
export function previewGoldMarket(rules: GoldMarketRules, side: Side, lotText: string, priceText: string,
    wallet: { gold: Long; yuanbao: Long }): GoldMarketPreview {
    if (side !== Side.Buy && side !== Side.Sell) throw new Error('请选择买入或卖出');
    const lots = warehouseMoney(lotText.trim(), 1), price = warehouseMoney(priceText.trim(), 2);
    if (price.lessThan(rules.minUnitPriceYuanbaoUnits) || price.greaterThan(rules.maxUnitPriceYuanbaoUnits) || !price.modulo(rules.priceStepYuanbaoUnits).isZero()) {
        throw new Error('每手报价超出当前限价或小数步长，请重新填写');
    }
    const gold = marketAmount(rules.goldPerLot, lots), yuanbao = marketAmount(price, lots);
    const buying = side === Side.Buy;
    if (buying ? wallet.yuanbao.lessThan(yuanbao) : wallet.gold.lessThan(gold)) throw new Error(buying ? '非绑定元宝余额不足' : '非绑定金币余额不足');
    const request: GoldMarketPlace = { action: Action.Place, side, lots, unitPriceYuanbaoUnits: price,
        termsHash: rules.termsHash, expectedDurationSeconds: rules.orderDurationSeconds };
    const money = buying ? `${marketMoney(yuanbao)} 非绑定元宝` : `${gold} 非绑定金币`;
    const commission = `${Math.floor(rules.sellerCommissionBp / 100)}${rules.sellerCommissionBp % 100 ? '.' + String(rules.sellerCommissionBp % 100).padStart(2, '0') : ''}%`;
    return { request, gold, yuanbao, confirmation: `${buying ? '买入' : '卖出'} ${lots} 手，每手 ${marketMoney(price)} 元宝\n冻结 ${money}，委托 ${goldMarketDuration(rules.orderDurationSeconds)}\n成交按在簿报价，卖方佣金 ${commission}` };
}
