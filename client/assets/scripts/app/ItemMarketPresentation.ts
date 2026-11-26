import Long from 'long';
import { ItemMarketActionKind, ItemMarketPhase, type ItemMarketAction, type ItemMarketItem, type ItemMarketOrder } from './ItemMarketProtocol';
import { warehouseMoney, warehouseMoneyText } from './WarehouseModel';

const maxAmount = (1n << 63n) - 1n;
const displayLimit = Long.fromString('8640000000000000');
const shanghai = new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });

export const marketMoney = warehouseMoneyText;

export function marketDeadline(at: Long): string {
    return at.greaterThan(displayLimit) ? '到期时间超出显示范围' : shanghai.format(new Date(at.toNumber()));
}

export function marketPhase(state: ItemMarketPhase): string {
    return ['','竞购','直购','已成交','已撤单','已到期'][state] ?? '';
}

export function marketAmount(price: Long, quantity: Long): Long {
    const total = BigInt(price.toString()) * BigInt(quantity.toString());
    if (total <= 0n || total > maxAmount) throw new Error('整单金额超出范围，请减少数量或单价');
    return Long.fromString(total.toString());
}

// 按当前角色关系、阶段和特权决定哪些动作可见。到期特权仍可撤自己的无预购订单。
export function marketOrderActions(order: ItemMarketOrder, canTrade: boolean): readonly ItemMarketActionKind[] {
    if (order.state !== ItemMarketPhase.Auction && order.state !== ItemMarketPhase.Direct || order.remainingQuantity.isZero()) return [];
    if (order.owned) return order.activeBidCount === 0 ? [ItemMarketActionKind.Cancel] : [];
    if (!canTrade || order.sameAccount) return [];
    if (order.state === ItemMarketPhase.Direct) return [ItemMarketActionKind.Direct];
    if (order.myActiveBid) return order.nextUnitPriceYuanbaoUnits.greaterThan(order.unitPriceYuanbaoUnits) ? [ItemMarketActionKind.Raise] : [];
    return [ItemMarketActionKind.Bid];
}

// 本地只过滤明显无法交易的背包数据，来源及具体交易资格仍由服务端检查。
export function marketBagCandidate(item: ItemMarketItem, equipment: boolean): boolean {
    return item.instanceId.length === 16 && !!item.sourceKind && !!item.sourceId && !item.bound && !item.locked && !item.recycleBlockFlags &&
        !item.quantity.isZero() && (!equipment || item.quality >= 2);
}

export function marketListing(item: ItemMarketItem, quantityText: string, priceText: string): ItemMarketAction {
    const quantity = warehouseMoney(quantityText.trim(), 1);
    const price = warehouseMoney(priceText.trim(), 2);
    if (quantity.greaterThan(item.quantity)) throw new Error('上架数量超出背包内此物品的数量');
    marketAmount(price, quantity);
    return { action: ItemMarketActionKind.List, itemInstanceId: new Uint8Array(item.instanceId), itemVersion: item.version,
        quantity, unitPriceYuanbaoUnits: price };
}

export function marketOrderAction(order: ItemMarketOrder, kind: ItemMarketActionKind, canTrade: boolean, quantityText = ''): ItemMarketAction {
    if (!marketOrderActions(order, canTrade).includes(kind) || kind === ItemMarketActionKind.List || kind === ItemMarketActionKind.Claim) {
        throw new Error('当前订单不能执行该动作，请刷新后查看');
    }
    const quantity = kind === ItemMarketActionKind.Direct ? warehouseMoney(quantityText.trim(), 1) : order.remainingQuantity;
    if (quantity.greaterThan(order.remainingQuantity)) throw new Error('直购数量超过当前订单库存');
    marketAmount(order.unitPriceYuanbaoUnits, quantity);
    return { action: kind, orderId: new Uint8Array(order.orderId), orderVersion: order.version,
        quantity, unitPriceYuanbaoUnits: order.unitPriceYuanbaoUnits };
}
