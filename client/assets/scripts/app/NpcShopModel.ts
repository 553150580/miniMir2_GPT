import { assetId, assetInteger, yuanbaoText } from './SkillAssetsModel';

export type VendorInteger = Parameters<typeof assetInteger>[0];
export type VendorMode = 1 | 2 | 3;
export type VendorAction = 1 | 2 | 3 | 4;

export interface VendorRow {
    readonly key: string;
    readonly itemId: number;
    readonly name: string;
    readonly detail: string;
    readonly offerId?: number;
    readonly maxQuantity: VendorInteger;
    readonly quantityStep?: VendorInteger;
    readonly instanceId?: Uint8Array;
    readonly itemVersion?: VendorInteger;
    readonly repairActions?: readonly (3 | 4)[];
    readonly disabledReason?: string;
}

export interface VendorSnapshot {
    readonly shopKey: string;
    readonly scope: string;
    readonly ready: boolean;
    readonly reason?: string;
    readonly allowsRepair?: boolean;
    readonly buy: readonly VendorRow[];
    readonly sell: readonly VendorRow[];
    readonly repair: readonly VendorRow[];
}

export interface VendorSelection {
    readonly shopKey: string;
    readonly scope: string;
    readonly key: string;
    readonly itemId: number;
    readonly offerId?: number;
    readonly instanceId?: Uint8Array;
    readonly itemVersion?: VendorInteger;
    readonly quantity: bigint;
    readonly action: VendorAction;
}

export interface VendorQuote {
    readonly quoteId: Uint8Array;
    readonly expiresAtUnixMs: VendorInteger;
    readonly action: VendorAction;
    readonly npcId: VendorInteger;
    readonly itemId: number;
    readonly offerId?: number;
    readonly instanceId?: Uint8Array;
    readonly quantity: VendorInteger;
    readonly bound?: boolean;
    readonly price: VendorInteger;
    readonly gold: VendorInteger;
    readonly boundGold: VendorInteger;
    readonly ingot: VendorInteger;
    readonly boundIngot: VendorInteger;
    readonly durabilityU?: VendorInteger;
    readonly maxDurabilityU?: VendorInteger;
    readonly originalDurabilityU?: VendorInteger;
    readonly repairedDurabilityU?: VendorInteger;
    readonly repairedMaxDurabilityU?: VendorInteger;
}

export function vendorRows(snapshot: VendorSnapshot | undefined, mode: VendorMode): readonly VendorRow[] {
    if (!snapshot) return [];
    return mode === 1 ? snapshot.buy : mode === 2 ? snapshot.sell : snapshot.repair;
}

export function vendorSelection(snapshot: VendorSnapshot | undefined, mode: VendorMode, row: VendorRow | undefined,
    action: VendorAction, quantity: bigint): VendorSelection | undefined {
    if (!snapshot?.ready || !snapshot.scope || !row || row.disabledReason || !row.key || row.itemId <= 0 || !Number.isSafeInteger(row.itemId)
        || (mode === 3 ? action < 3 || quantity !== 1n || !row.repairActions?.includes(action as 3 | 4) : action !== mode)
        || quantity < 1n || quantity > assetInteger(row.maxQuantity)) return undefined;
    const step = assetInteger(row.quantityStep ?? 1);
    if (step < 1n || quantity % step !== 0n || (action === 1 && (!row.offerId || !Number.isSafeInteger(row.offerId) || row.offerId < 1))) return undefined;
    if (action !== 1 && (!row.instanceId || row.instanceId.length !== 16 || row.itemVersion === undefined || assetInteger(row.itemVersion) < 1n)) return undefined;
    return { shopKey: snapshot.shopKey, scope: snapshot.scope, key: row.key, itemId: row.itemId, quantity, action,
        ...(row.offerId !== undefined && { offerId: row.offerId }), ...(row.instanceId && { instanceId: new Uint8Array(row.instanceId) }),
        ...(row.itemVersion !== undefined && { itemVersion: assetInteger(row.itemVersion) }) };
}

function optionalInstanceId(value: Uint8Array | undefined): string {
    return value?.length ? assetId(value) : '';
}

export function sameVendorSelection(previous: VendorSelection, current: VendorSelection | undefined): boolean {
    return !!current && current.scope === previous.scope && current.shopKey === previous.shopKey && current.key === previous.key
        && current.itemId === previous.itemId && current.offerId === previous.offerId && current.action === previous.action && current.quantity === previous.quantity
        && current.itemVersion?.toString() === previous.itemVersion?.toString()
        && optionalInstanceId(current.instanceId) === optionalInstanceId(previous.instanceId);
}

const titles: Record<VendorAction, string> = { 1: '购买', 2: '出售', 3: '普通修理', 4: '特殊修理' };
export function vendorActionName(action: VendorAction): string { return titles[action]; }

export function vendorQuoteText(name: string, selection: VendorSelection, quote: VendorQuote): string {
    if (quote.action !== selection.action || quote.itemId !== selection.itemId || (selection.action === 1 && quote.offerId !== selection.offerId)
        || assetInteger(quote.quantity) !== selection.quantity
        || optionalInstanceId(quote.instanceId) !== optionalInstanceId(selection.instanceId) || assetInteger(quote.npcId) < 1n) {
        throw new Error('报价与原商品或装备不一致，请重新报价');
    }
    const funds = [[quote.boundGold, '绑定金币'], [quote.gold, '金币'], [quote.boundIngot, '绑定元宝'], [quote.ingot, '元宝']] as const;
    const money = funds.filter(([value]) => assetInteger(value) > 0n)
        .map(([value, label]) => `${label}${label.endsWith('元宝') ? yuanbaoText(value) : assetInteger(value)}`);
    const price = assetInteger(quote.price);
    if (!money.length && price > 0n) throw new Error('报价缺少钱包明细，请重新报价');
    const lines = [`${titles[selection.action]} ${name} ×${selection.quantity}`,
        `${selection.action === 2 ? '获得' : '费用'} ${money.length ? money.join('、') : '0'}`];
    if (typeof quote.bound === 'boolean' && selection.action < 3) lines.push(quote.bound ? '物品绑定' : '物品非绑定');
    if (selection.action >= 3) {
        const [before, max, original, after, repairedMax] = [
            quote.durabilityU, quote.maxDurabilityU, quote.originalDurabilityU, quote.repairedDurabilityU, quote.repairedMaxDurabilityU,
        ].map(value => {
            if (value === undefined) throw new Error('修理报价缺少持久信息');
            return assetInteger(value);
        });
        if (before > max || max > original || after > repairedMax || repairedMax > original) throw new Error('修理报价持久信息无效');
        // 服务器以千分之一持久为单位，按策划向上取整显示上限、当前值和恢复量。
        const display = (value: bigint): bigint => (value + 999n) / 1000n;
        lines.push(`持久 ${display(before)}/${display(max)} → ${display(after)}/${display(repairedMax)}`,
            `恢复 ${display(after > before ? after - before : 0n)} · 原始上限 ${display(original)}`,
            `当前上限${max === repairedMax ? '不变' : `调整为 ${display(repairedMax)}`} · 绑定状态保持`);
    }
    return lines.join('\n');
}
