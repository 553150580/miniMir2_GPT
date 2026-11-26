// 原操作的永久回执用于对账和重连重放，背包仍以新的权威库存为准。
import { assetId, sourceName } from './SkillAssetsModel';
import { observedRefineAffixes, observedRefinePending, refineAmount, refineId, refineItemShape, sameRefineId, sameRefineSlots } from './EquipmentRefineModel';
import type { RefineAction, RefineFees, RefineItem, RefineQuote, RefineSpend } from './EquipmentRefineModel';

export interface RefineReceipt extends RefineFees {
    readonly action: number;
    readonly target: RefineItem;
    readonly equipped: boolean;
    readonly quoteId: Uint8Array;
    readonly pendingId: Uint8Array;
    readonly stones: readonly RefineSpend[];
}

export interface RecycleRefineRoll {
    readonly equipmentInstanceId: Uint8Array;
    readonly stone?: RefineItem | null;
}

const zero = (value: Uint8Array | undefined): boolean => value === undefined || value instanceof Uint8Array && value.length === 0;
function checkedStones(stones: readonly RefineSpend[], allowBound: boolean): bigint {
    const seen = new Set<string>();
    let total = 0n;
    for (const row of stones) {
        if (!row || !refineItemShape(row.item) || !refineAmount(row.quantity) || row.quantity.isZero() || row.quantity.greaterThan(row.item.quantity) ||
            row.item.locked || !!row.item.recycleBlockFlags || !row.item.sourceKind || !row.item.sourceId || row.item.bound && !allowBound) {
            throw new Error('洗炼回执的原石堆或绑定无效');
        }
        const id = assetId(row.item.instanceId);
        if (seen.has(id)) throw new Error('洗炼回执重复扣除同一原石堆');
        seen.add(id);
        total += BigInt(row.quantity.toString());
    }
    return total;
}

// 旧操作重连时仍精确核对原装备版本及候选编号。在原页面提交还会校验实际消费与已展示报价一致。
export function validateRefineReceipt(request: RefineAction, value: RefineReceipt, quote?: RefineQuote): RefineReceipt {
    if (!value || !refineItemShape(value.target) || !sameRefineId(value.target.instanceId, request.target.instanceId) ||
        !value.target.version.equals(request.target.version.add(1)) || value.target.quantity.toString() !== '1' ||
        value.equipped !== request.target.equipped || value.action !== request.action || !refineId(value.pendingId) ||
        !Array.isArray(value.stones) || ![value.gold, value.boundGold, value.yuanbaoUnits, value.boundYuanbaoUnits].every(refineAmount) ||
        !Number.isSafeInteger(value.target.quality) || value.target.quality! < 1 || value.target.quality! > 4 ||
        !observedRefineAffixes(value.target.refineAffixes, value.target.quality!)) throw new Error('洗炼回执的原装备或动作无效');
    if (request.action !== 1) {
        if (!sameRefineId(request.pendingId, value.pendingId) || !zero(value.quoteId) || value.target.refinePending || value.stones.length ||
            ![value.gold, value.boundGold, value.yuanbaoUnits, value.boundYuanbaoUnits].every(amount => amount.isZero())) throw new Error('候选决定回执无效');
        return value;
    }
    const candidate = observedRefinePending(value.target, value.target.quality!);
    // 品质已限制在 1 至 4。先在此范围计算石头数，避免 Creator 将 BigInt 幂降级成 Math.pow。
    const stoneCount = BigInt(1 << value.target.quality!);
    if (!sameRefineId(request.quoteId, value.quoteId) || !candidate || !sameRefineId(candidate.pendingId, value.pendingId) ||
        checkedStones(value.stones, request.allowBound) !== stoneCount ||
        value.gold.isZero() && value.boundGold.isZero() || !request.allowBound && (!value.boundGold.isZero() || !value.boundYuanbaoUnits.isZero()) ||
        !value.target.bound && (value.stones.some(row => row.item.bound) || !value.boundGold.isZero() || !value.boundYuanbaoUnits.isZero())) {
        throw new Error('生成候选回执的原报价、实际消耗或绑定无效');
    }
    if (!quote) return value;
    if (quote.target.itemId !== value.target.itemId || quote.target.quality !== value.target.quality || quote.outputBound !== value.target.bound ||
        quote.target.refinePending && sameRefineId(quote.target.refinePending.pendingId, value.pendingId) ||
        !sameRefineSlots(quote.lockedSlots, candidate.lockedSlots) || !quote.gold.equals(value.gold) || !quote.boundGold.equals(value.boundGold) ||
        !quote.yuanbaoUnits.equals(value.yuanbaoUnits) || !quote.boundYuanbaoUnits.equals(value.boundYuanbaoUnits) ||
        quote.stones.length !== value.stones.length) throw new Error('洗炼回执的实际结果偏离原报价');
    for (const paid of value.stones) {
        const original = quote.stones.find(row => sameRefineId(row.item.instanceId, paid.item.instanceId));
        if (!original || !original.quantity.equals(paid.quantity) || !original.item.version.equals(paid.item.version) ||
            original.item.bound !== paid.item.bound || original.item.itemId !== paid.item.itemId ||
            !original.item.quantity.equals(paid.item.quantity) || original.item.sourceId !== paid.item.sourceId ||
            original.item.sourceKind !== paid.item.sourceKind) throw new Error('洗炼回执更换了所选原石堆');
    }
    return value;
}

// 分解命中和未命中都沿原装备编号对账。回执自身不会增加库存。
export function validateRecycleRefineReceipt(inputs: readonly Uint8Array[], value: unknown, stoneId?: number): readonly RecycleRefineRoll[] {
    const body = value as { rolls?: readonly RecycleRefineRoll[] } | null;
    if (!Array.isArray(body?.rolls) || body.rolls.length < 1 || body.rolls.length > inputs.length) throw new Error('分解洗炼石回执数量无效');
    const originals = new Set(inputs.map(assetId)), seen = new Set<string>();
    for (const row of body.rolls) {
        if (!row || !refineId(row.equipmentInstanceId) || !originals.has(assetId(row.equipmentInstanceId))) {
            throw new Error('分解洗炼石回执没有原装备');
        }
        const input = assetId(row.equipmentInstanceId);
        if (seen.has(input)) throw new Error('分解洗炼石回执重复记录原装备');
        seen.add(input);
        if (row.stone != null) {
            if (!refineItemShape(row.stone) || row.stone.quantity.toString() !== '1' || row.stone.version.toString() !== '1' ||
                row.stone.sourceKind !== 'equipment_recycle_refine' || row.stone.sourceId !== input ||
                (stoneId !== undefined && row.stone.itemId !== stoneId)) throw new Error('分解回执的洗炼石来源无效');
            const stone = assetId(row.stone.instanceId);
            if (originals.has(stone) || seen.has(stone)) throw new Error('分解回执的洗炼石实例已出现');
            seen.add(stone);
        }
    }
    return body.rolls;
}

export function recycleRefineResultLines(rolls: readonly RecycleRefineRoll[]): string[] {
    return rolls.map(row => row.stone ? `获得${row.stone.bound ? '绑定' : '非绑定'}洗炼石 1枚 · ${sourceName(row.stone.sourceKind)}` : '本次分解未获得洗炼石');
}
