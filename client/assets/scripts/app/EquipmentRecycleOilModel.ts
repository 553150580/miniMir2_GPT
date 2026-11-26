import Long from 'long';
import { equipmentCurrentLuck } from './EquipmentModel';
import type { EquipmentItem } from './EquipmentModel';
import { assetId, assetInteger } from './SkillAssetsModel';

// 油的概率读取已同步玩法候选，旧玩法保持原分解界面。
export interface EquipmentRecycleOilRule {
    readonly denominator: number;
    readonly oilItemId: number;
}

export interface RecycleOilRoll {
    readonly equipmentInstanceId: Uint8Array;
    readonly luck: number;
    readonly oil?: EquipmentItem | null;
}

export interface RecycleOilReceipt { readonly rolls: readonly RecycleOilRoll[]; }

function positiveUint32(value: unknown): value is number {
    return typeof value === 'number' && Number.isInteger(value) && value > 0 && value <= 0xffffffff;
}

function id16(value: unknown): value is Uint8Array {
    return value instanceof Uint8Array && value.length === 16;
}

export function equipmentRecycleOilRule(value: unknown, oil: unknown): EquipmentRecycleOilRule | undefined {
    if (!value || typeof value !== 'object' || Array.isArray(value) || !oil || typeof oil !== 'object' || Array.isArray(oil)) return undefined;
    const { denominator } = value as { denominator?: unknown };
    const { item_id: oilItemId } = oil as { item_id?: unknown };
    return positiveUint32(denominator) && positiveUint32(oilItemId) ? { denominator, oilItemId } : undefined;
}

// 分解概率按整数计算。若百分数不能在两位小数内精确表示，明确标为近似。
export function recycleOilPercent(luck: number, denominator: number): string {
    const base = BigInt(denominator), scaled = BigInt(Math.min(luck, denominator)) * 10_000n;
    const rounded = (scaled * 2n + base) / (2n * base);
    const fraction = String(rounded % 100n).padStart(2, '0').replace(/0+$/, '');
    return `${scaled % base === 0n ? '' : '约'}${rounded / 100n}${fraction ? '.' + fraction : ''}%`;
}

export function equipmentRecycleOilLines(rule: EquipmentRecycleOilRule, item: EquipmentItem, protectedReason = ''): string[] {
    const luck = equipmentCurrentLuck(item);
    if (protectedReason) return ['装备受保护，当前不能分解出油'];
    const original = assetInteger(item.originalDurabilityU ?? 0), max = assetInteger(item.maxDurabilityU ?? 0),
        current = assetInteger(item.durabilityU ?? 0);
    if (max <= 0n || max > original || current > max) return ['持久未确认，暂不显示出油概率'];
    if (current === 0n) return ['破损分解金币 0', '破损装备不判定祝福油'];
    if (!luck) return ['幸运+0，不判定祝福油'];
    return [`分解出油概率 ${recycleOilPercent(luck, rule.denominator)}`, `命中得1瓶${item.bound ? '绑定' : '非绑定'}祝福油`];
}

export function equipmentRecycleOilConfirmation(name: string, item: EquipmentItem, rule: EquipmentRecycleOilRule): string {
    return [`分解${name}？`, `当前幸运+${equipmentCurrentLuck(item)} · ${item.bound ? '绑定' : '非绑定'}`,
        ...equipmentRecycleOilLines(rule, item), '产物继承原装备绑定', '分解不可撤销'].join('\n');
}

// 只接收属于原请求的永久判定。没有判定凭证时不能从库存变化推断出油或失败。
export function validateRecycleOilReceipt(itemIds: readonly Uint8Array[], value?: RecycleOilReceipt | null, oilItemId?: number): readonly RecycleOilRoll[] {
    if (value == null) return [];
    const error = '分解祝福油回执不匹配，保留原请求等待核对';
    const originals = new Set(itemIds.map(assetId)), seen = new Set<string>();
    if (!Array.isArray(value.rolls) || value.rolls.length > originals.size) throw new Error(error);
    for (const roll of value.rolls) {
        if (!roll || !id16(roll.equipmentInstanceId) || !positiveUint32(roll.luck) || roll.luck > 9) throw new Error(error);
        const source = assetId(roll.equipmentInstanceId);
        if (!originals.has(source) || seen.has(source)) throw new Error(error);
        seen.add(source);
        const oil = roll.oil;
        if (oil == null) continue;
        if (!id16(oil.instanceId) || !positiveUint32(oil.itemId) || (oilItemId !== undefined && oil.itemId !== oilItemId) ||
            typeof oil.bound !== 'boolean' || !Long.isLong(oil.version) || !oil.version.unsigned || !oil.version.equals(Long.UONE) ||
            !Long.isLong(oil.quantity) || oil.quantity.unsigned || !oil.quantity.equals(Long.ONE) ||
            oil.sourceKind !== 'equipment_recycle' || oil.sourceId !== source) throw new Error(error);
        const id = assetId(oil.instanceId);
        if (originals.has(id) || seen.has(id)) throw new Error(error);
        seen.add(id);
    }
    return value.rolls;
}

export function recycleOilResultSummary(rolls: readonly RecycleOilRoll[]): string {
    const count = rolls.filter(row => !!row.oil).length;
    return count ? `分解获得${count}瓶祝福油` : '本次未获得祝福油';
}

export function recycleOilResultLines(rolls: readonly RecycleOilRoll[]): string[] {
    return rolls.flatMap(roll => [`原装备尾号 ${assetId(roll.equipmentInstanceId).slice(-8)}`, `分解幸运+${roll.luck}`,
        ...(roll.oil ? [`获得1瓶${roll.oil.bound ? '绑定' : '非绑定'}祝福油`, '来源 装备分解'] : ['本件未获得祝福油'])]);
}
