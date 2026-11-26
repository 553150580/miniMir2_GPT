import Long from 'long';

export enum ItemMarketScope { Public = 1, Mine, Escrow }
export enum ItemMarketPhase { Auction = 1, Direct, Sold, Cancelled, Expired }
export enum ItemMarketActionKind { List = 1, Bid, Raise, Direct, Cancel, Claim }
export enum ItemMarketEscrowKind { Item = 1, Payout, Refund }

export interface ItemMarketItem {
    readonly instanceId: Uint8Array; // 本人可操作的原物品实例，公开浏览数据可以省略。
    readonly itemId: number; // 服务端物品定义编号。
    readonly quantity: Long; // 当前物品数量。
    readonly version: Long; // 本人可操作原实例版本。
    readonly bound: boolean; // 绑定物品不能上架。
    readonly locked: boolean; // 锁定物品不能上架。
    readonly recycleBlockFlags: number; // 现有资产占用、装备中与待确认保护。
    readonly quality: number; // 原数值品质，按装备规则排除低品质装备。
    readonly sourceKind: string; // 原来源，不能由玩家改变。
    readonly sourceId: string; // 原实例使用的来源身份。
}

export interface ItemMarketOrder {
    readonly orderId: Uint8Array;
    readonly version: Long; // 购买确认时必须保留这份订单版本。
    readonly state: ItemMarketPhase;
    readonly item: ItemMarketItem;
    readonly remainingQuantity: Long; // 当前订单的剩余库存。
    readonly unitPriceYuanbaoUnits: Long;
    readonly endAtUnixMs: Long;
    readonly activeBidCount: number;
    readonly owned: boolean; // 本角色是原卖家。
    readonly sameAccount: boolean; // 卖家与当前角色在一个账号。
    readonly myActiveBid: boolean; // 本角色有有效预购且可参与加价。
    readonly nextUnitPriceYuanbaoUnits: Long; // 服务端已经算出的下一次加价单价。
}

export interface ItemMarketEscrow {
    readonly escrowId: Uint8Array;
    readonly orderId: Uint8Array;
    readonly version: Long;
    readonly kind: ItemMarketEscrowKind;
    readonly item?: ItemMarketItem | null;
    readonly yuanbaoUnits: Long;
    readonly createdAtUnixMs: Long;
}

export interface ItemMarketState {
    readonly sceneEpoch: Long;
    readonly scope: ItemMarketScope;
    readonly enabled: boolean; // 当前规则是否装配道具市场。
    readonly canTrade: boolean; // 当前角色是否具备发起新交易的资格。
    readonly orders: readonly ItemMarketOrder[];
    readonly escrows: readonly ItemMarketEscrow[];
    readonly nextCursor: Uint8Array; // 只能原样向服务器继续请求，不解析内部格式。
    readonly errorCode: number;
    readonly roleRevision: Long;
}

export type ItemMarketAction = {
    readonly action: ItemMarketActionKind.List; readonly itemInstanceId: Uint8Array; readonly itemVersion: Long;
    readonly quantity: Long; readonly unitPriceYuanbaoUnits: Long;
} | {
    readonly action: ItemMarketActionKind.Bid | ItemMarketActionKind.Raise | ItemMarketActionKind.Direct | ItemMarketActionKind.Cancel;
    readonly orderId: Uint8Array; readonly orderVersion: Long; readonly quantity: Long; readonly unitPriceYuanbaoUnits: Long;
} | {
    readonly action: ItemMarketActionKind.Claim; readonly escrowId: Uint8Array; readonly escrowVersion: Long; readonly quantity: Long;
};

export interface ItemMarketReceipt {
    readonly action: ItemMarketActionKind;
    readonly orderId: Uint8Array;
    readonly orderVersion: Long;
    readonly itemId: number;
    readonly quantity: Long;
    readonly unitPriceYuanbaoUnits: Long;
    readonly yuanbaoDeltaUnits: Long; // 原角色可用非绑定元宝变化，成本为负。
    readonly taxYuanbaoUnits: Long;
    readonly escrowId: Uint8Array;
    readonly escrowVersion: Long;
}

export const itemMarketPageSize = 20; // 每页的有界查询预算。

export function marketId(value: Uint8Array): string { return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join(''); }

function id(value: unknown): value is Uint8Array {
    return value instanceof Uint8Array && value.length === 16 && value.some(byte => byte !== 0);
}

function integer(value: unknown, unsigned = false, positive = false): value is Long {
    return Long.isLong(value) && value.unsigned === unsigned && !value.isNegative() && (!positive || !value.isZero());
}

function count(value: unknown, minimum = 0): value is number {
    return typeof value === 'number' && Number.isInteger(value) && value >= minimum && value <= 0xffffffff;
}

function marketOrder(value: unknown): value is ItemMarketOrder {
    const row = value as Partial<ItemMarketOrder> | null;
    return !!row && id(row.orderId) && integer(row.version, true, true) && count(row.state, 1) && row.state <= 5 &&
        count(row.item?.itemId, 1) && integer(row.remainingQuantity) && integer(row.unitPriceYuanbaoUnits, false, true) &&
        integer(row.endAtUnixMs, false, true) && count(row.activeBidCount) && integer(row.nextUnitPriceYuanbaoUnits) &&
        typeof row.owned === 'boolean' && typeof row.sameAccount === 'boolean' && typeof row.myActiveBid === 'boolean';
}

function marketEscrow(value: unknown): value is ItemMarketEscrow {
    const row = value as Partial<ItemMarketEscrow> | null;
    return !!row && id(row.escrowId) && id(row.orderId) && integer(row.version, true, true) && integer(row.createdAtUnixMs, false, true) &&
        (row.kind === ItemMarketEscrowKind.Item ? count(row.item?.itemId, 1) && integer(row.item?.quantity, false, true) :
            (row.kind === ItemMarketEscrowKind.Payout || row.kind === ItemMarketEscrowKind.Refund) && integer(row.yuanbaoUnits, false, true));
}

// 验证原场景、分页与托管范围，保留所有线上的 Long 和指纹身份。
export function validateItemMarketState(value: unknown, sceneEpoch: Long, scope: ItemMarketScope): ItemMarketState {
    const state = value as Partial<ItemMarketState> | null;
    if (!state || !integer(state.sceneEpoch, true, true) || !state.sceneEpoch.equals(sceneEpoch) || state.scope !== scope ||
        typeof state.enabled !== 'boolean' || typeof state.canTrade !== 'boolean' || !integer(state.roleRevision, true, true) ||
        !Array.isArray(state.orders) || !Array.isArray(state.escrows) || state.orders.length + state.escrows.length > itemMarketPageSize ||
        (scope === ItemMarketScope.Escrow ? state.orders.length !== 0 : state.escrows.length !== 0) ||
        !state.orders.every(marketOrder) || !state.escrows.every(marketEscrow) ||
        !(state.nextCursor instanceof Uint8Array) || state.nextCursor.length > 128) throw new Error('道具市场数据未确认，请刷新');
    return state as ItemMarketState;
}

// 成功回执必须对应原动作和目标。回执不全时让账本保留原操作等待重新核对。
export function validateItemMarketReceipt(value: unknown, request: ItemMarketAction): ItemMarketReceipt {
    const receipt = value as Partial<ItemMarketReceipt> | null;
    if (!receipt || receipt.action !== request.action || !id(receipt.orderId) || !integer(receipt.orderVersion, true) ||
        !integer(receipt.quantity) || !integer(receipt.unitPriceYuanbaoUnits) || !integer(receipt.taxYuanbaoUnits) ||
        !Long.isLong(receipt.yuanbaoDeltaUnits) || receipt.yuanbaoDeltaUnits.unsigned || !count(receipt.itemId)) {
        throw new Error('道具市场回执未确认原操作，请沿原身份重新核对');
    }
    if (request.action === ItemMarketActionKind.Claim) {
        if (!id(receipt.escrowId) || marketId(receipt.escrowId) !== marketId(request.escrowId)) throw new Error('未确认原托管项目，请沿原操作重新核对');
    } else {
        if (receipt.orderVersion.isZero() || (request.action !== ItemMarketActionKind.List && marketId(receipt.orderId) !== marketId(request.orderId))) {
            throw new Error('未确认原道具订单，请沿原操作重新核对');
        }
        if ((request.action === ItemMarketActionKind.List || request.action === ItemMarketActionKind.Direct) && !receipt.quantity.equals(request.quantity)) {
            throw new Error('未确认原道具数量，请沿原操作重新核对');
        }
    }
    return receipt as ItemMarketReceipt;
}
