import Long from 'long';
import { marketId } from './ItemMarketProtocol';

export enum GoldMarketScope { Buy = 1, Sell, Mine, Escrow }
export enum GoldMarketSide { Buy = 1, Sell }
export enum GoldMarketPhase { Active = 1, Filled, Cancelled, Expired }
export enum GoldMarketActionKind { Place = 1, Cancel, Claim }
export enum GoldMarketEscrowKind { BuyerGold = 1, SellerProceeds, BuyerRefund, SellerGoldRefund }

export interface GoldMarketRules {
    readonly ruleVersion: Long;
    readonly configHash: string;
    readonly termsHash: string; // 本次报价参考价、限价及委托期限的服务端摘要。
    readonly referenceUnitPriceYuanbaoUnits: Long;
    readonly minUnitPriceYuanbaoUnits: Long;
    readonly maxUnitPriceYuanbaoUnits: Long;
    readonly priceStepYuanbaoUnits: Long;
    readonly goldPerLot: Long; // 一手对应的非绑定金币数量。
    readonly orderDurationSeconds: number;
    readonly sellerCommissionBp: number; // 仅卖方实际成交时承担，万分比。
    readonly maxOrdersPerSide: number; // 同账号、运行服、方向的未结委托限额。
}

export interface GoldMarketOrder {
    readonly orderId: Uint8Array;
    readonly version: Long;
    readonly state: GoldMarketPhase;
    readonly side: GoldMarketSide;
    readonly unitPriceYuanbaoUnits: Long;
    readonly remainingLots: Long;
    readonly initialLots: Long;
    readonly expiresAtUnixMs: Long;
    readonly createdAtUnixMs: Long;
    readonly owned: boolean;
    readonly sameAccount: boolean;
}

export interface GoldMarketEscrow {
    readonly escrowId: Uint8Array;
    readonly orderId: Uint8Array;
    readonly version: Long;
    readonly kind: GoldMarketEscrowKind;
    readonly gold: Long;
    readonly yuanbaoUnits: Long;
    readonly createdAtUnixMs: Long;
    readonly reason: number;
}

export interface GoldMarketState {
    readonly sceneEpoch: Long;
    readonly scope: GoldMarketScope;
    readonly enabled: boolean; // 旧委托或待领仍可访问时也开放读取入口。
    readonly canTrade: boolean; // 当前角色可以用此轮条款发新单。
    readonly rules?: GoldMarketRules | null;
    readonly orders: readonly GoldMarketOrder[];
    readonly escrows: readonly GoldMarketEscrow[];
    readonly nextCursor: Uint8Array;
    readonly errorCode: number;
    readonly roleRevision: Long;
}

export type GoldMarketPlace = { readonly action: GoldMarketActionKind.Place; readonly side: GoldMarketSide; readonly lots: Long;
    readonly unitPriceYuanbaoUnits: Long; readonly termsHash: string; readonly expectedDurationSeconds: number; };
export type GoldMarketAction = GoldMarketPlace |
    { readonly action: GoldMarketActionKind.Cancel; readonly orderId: Uint8Array; readonly orderVersion: Long } |
    { readonly action: GoldMarketActionKind.Claim; readonly escrowId: Uint8Array; readonly escrowVersion: Long };

export interface GoldMarketReceipt {
    readonly action: GoldMarketActionKind;
    readonly orderId: Uint8Array;
    readonly orderVersion: Long;
    readonly side: GoldMarketSide;
    readonly state: GoldMarketPhase;
    readonly quantityLots: Long;
    readonly filledLots: Long;
    readonly remainingLots: Long;
    readonly unitPriceYuanbaoUnits: Long;
    readonly goldDelta: Long; // 本次确认立即发生的可用非绑定金币变化。
    readonly yuanbaoDeltaUnits: Long;
    readonly sellerFeeYuanbaoUnits: Long;
    readonly escrowId: Uint8Array;
    readonly escrowVersion: Long;
    readonly expiresAtUnixMs: Long;
}

export const goldMarketPageSize = 20;

function identity(value: unknown): value is Uint8Array {
    return value instanceof Uint8Array && value.length === 16 && value.some(byte => byte !== 0);
}

function long(value: unknown, unsigned = false, minimum = 0): value is Long {
    return Long.isLong(value) && value.unsigned === unsigned && value.greaterThanOrEqual(minimum);
}

function signed(value: unknown): value is Long { return Long.isLong(value) && !value.unsigned; }

function range(value: unknown, min: number, max = 0xffffffff): value is number {
    return typeof value === 'number' && Number.isInteger(value) && value >= min && value <= max;
}

export function goldMarketHash(value: unknown): value is string { return typeof value === 'string' && /^[0-9a-f]{64}$/.test(value); }

function terms(value: unknown): value is GoldMarketRules {
    const row = value as Partial<GoldMarketRules> | null;
    return !!row && long(row.ruleVersion, true, 1) && goldMarketHash(row.configHash) && goldMarketHash(row.termsHash) &&
        long(row.referenceUnitPriceYuanbaoUnits, false, 1) && long(row.minUnitPriceYuanbaoUnits, false, 1) && long(row.maxUnitPriceYuanbaoUnits, false, 1) &&
        row.minUnitPriceYuanbaoUnits.lessThanOrEqual(row.referenceUnitPriceYuanbaoUnits) && row.referenceUnitPriceYuanbaoUnits.lessThanOrEqual(row.maxUnitPriceYuanbaoUnits) &&
        long(row.priceStepYuanbaoUnits, false, 1) && long(row.goldPerLot, false, 1) &&
        range(row.orderDurationSeconds, 1) && range(row.sellerCommissionBp, 1, 10000) && range(row.maxOrdersPerSide, 1);
}

function order(value: unknown): value is GoldMarketOrder {
    const row = value as Partial<GoldMarketOrder> | null;
    return !!row && identity(row.orderId) && long(row.version, true, 1) && range(row.state, 1, 4) && range(row.side, 1, 2) &&
        long(row.unitPriceYuanbaoUnits, false, 1) && long(row.remainingLots) && long(row.initialLots, false, 1) && row.remainingLots.lessThanOrEqual(row.initialLots) &&
        long(row.expiresAtUnixMs, false, 1) && long(row.createdAtUnixMs, false, 1) && typeof row.owned === 'boolean' && typeof row.sameAccount === 'boolean';
}

function escrow(value: unknown): value is GoldMarketEscrow {
    const row = value as Partial<GoldMarketEscrow> | null;
    if (!row || !identity(row.escrowId) || !identity(row.orderId) || !long(row.version, true, 1) || !range(row.kind, 1, 4) ||
        !long(row.gold) || !long(row.yuanbaoUnits) || !long(row.createdAtUnixMs, false, 1) || !range(row.reason, 1, 4)) return false;
    const gold = row.kind === GoldMarketEscrowKind.BuyerGold || row.kind === GoldMarketEscrowKind.SellerGoldRefund;
    return gold ? !row.gold.isZero() && row.yuanbaoUnits.isZero() : row.gold.isZero() && !row.yuanbaoUnits.isZero();
}

export function validateGoldMarketState(value: unknown, sceneEpoch: Long, scope: GoldMarketScope): GoldMarketState {
    const state = value as Partial<GoldMarketState> | null;
    if (!state || !long(state.sceneEpoch, true, 1) || !state.sceneEpoch.equals(sceneEpoch) || state.scope !== scope ||
        typeof state.enabled !== 'boolean' || typeof state.canTrade !== 'boolean' || !long(state.roleRevision, true, 1) ||
        (state.rules != null && !terms(state.rules)) || (state.canTrade && (!state.enabled || !state.rules)) ||
        !Array.isArray(state.orders) || !Array.isArray(state.escrows) || state.orders.length + state.escrows.length > goldMarketPageSize ||
        (scope === GoldMarketScope.Escrow ? !!state.orders.length : !!state.escrows.length) ||
        !state.orders.every(order) || !state.escrows.every(escrow) || !(state.nextCursor instanceof Uint8Array) || state.nextCursor.length > 128) {
        throw new Error('金币市场状态未确认，请刷新');
    }
    return state as GoldMarketState;
}

// 仅能用同一命令的首次确认清除未决记录。挂单形态必须明确为未撮合的活动委托。
export function validateGoldMarketReceipt(value: unknown, request: GoldMarketAction): GoldMarketReceipt {
    const row = value as Partial<GoldMarketReceipt> | null;
    if (!row || row.action !== request.action || !identity(row.orderId) || !signed(row.goldDelta) || !signed(row.yuanbaoDeltaUnits)) {
        throw new Error('金币市场回执未确认原操作，请沿原身份重新核对');
    }
    switch (request.action) {
    case GoldMarketActionKind.Place:
        if (row.side !== request.side || row.state !== GoldMarketPhase.Active || !long(row.orderVersion, true, 1) ||
            !long(row.quantityLots, false, 1) || !row.quantityLots.equals(request.lots) || !long(row.filledLots) || !row.filledLots.isZero() ||
            !long(row.remainingLots) || !row.remainingLots.equals(request.lots) || !long(row.unitPriceYuanbaoUnits, false, 1) ||
            !row.unitPriceYuanbaoUnits.equals(request.unitPriceYuanbaoUnits) || !long(row.expiresAtUnixMs, false, 1)) {
            throw new Error('金币挂单回执未确认原价量，请重新核对');
        }
        break;
    case GoldMarketActionKind.Cancel:
        if (marketId(row.orderId) !== marketId(request.orderId) || row.state !== GoldMarketPhase.Cancelled) throw new Error('金币撤单回执未确认原委托');
        break;
    case GoldMarketActionKind.Claim:
        if (!identity(row.escrowId) || marketId(row.escrowId) !== marketId(request.escrowId)) throw new Error('金币领取回执未确认原托管资产');
        break;
    }
    return row as GoldMarketReceipt;
}
