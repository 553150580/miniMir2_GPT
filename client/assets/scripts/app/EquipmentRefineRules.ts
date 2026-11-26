// 洗炼公开规则只用于显示与前端校验，实际随机和结算由服务端完成。
import type { EquipmentDefinition, EquipmentItem } from './EquipmentModel';
import { equipmentCurrentLuck } from './EquipmentModel';
import { assetInteger } from './SkillAssetsModel';

interface RefineQuality {
    readonly slots: number;
    readonly stones: bigint;
    readonly repeats: boolean;
}

interface RefineAttribute {
    readonly name: string;
    readonly minQuality: number;
}

export interface EquipmentRefineRules {
    readonly stoneItemId: number;
    readonly qualities: ReadonlyMap<number, RefineQuality>;
    readonly targets: ReadonlyMap<number, number>;
    readonly attributes: ReadonlyMap<number, RefineAttribute>;
    readonly gold: readonly { min: number; max: number; amount: bigint }[];
    readonly locks: ReadonlyMap<number, bigint>;
    readonly tiers: readonly { tier: number; weight: number }[];
    readonly recycle: { itemId: number; source: string; quality: number; stage: number; probability: number; quantity: bigint };
}

const uint = (value: unknown, min = 0, max = 0xffffffff): value is number =>
    typeof value === 'number' && Number.isSafeInteger(value) && value >= min && value <= max;
const object = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value);
const rows = (value: unknown, maximum: number): value is Record<string, unknown>[] =>
    Array.isArray(value) && value.length > 0 && value.length <= maximum && value.every(object);

// 缺少契约或出现歧义时保持入口关闭，默认包继续展示原装备详情。
export function equipmentRefineRules(raw: unknown): EquipmentRefineRules | undefined {
    if (!object(raw) || !uint(raw.stone_item_id, 1) || !rows(raw.targets, 200) || !rows(raw.qualities, 4) ||
        !rows(raw.affixes, 16) || !rows(raw.gold_bands, 32) || !rows(raw.lock_costs, 4) ||
        !rows(raw.tiers, 5) || raw.tiers.length !== 5 || !object(raw.recycle)) return undefined;
    const qualities = new Map<number, RefineQuality>();
    for (const quality of raw.qualities) {
        if (!uint(quality.quality, 1, 4) || quality.slot_count !== quality.quality ||
            !uint(quality.stone_cost, 2, 16) || quality.stone_cost !== (2 ** quality.quality) ||
            typeof quality.allow_duplicate_attr !== 'boolean' || qualities.has(quality.quality)) return undefined;
        qualities.set(quality.quality, { slots: quality.slot_count, stones: BigInt(quality.stone_cost), repeats: quality.allow_duplicate_attr });
    }
    const targets = new Map<number, number>();
    for (const target of raw.targets) {
        if (!uint(target.item_id, 1) || !uint(target.quality, 1, 4) || targets.has(target.item_id) ||
            !qualities.has(target.quality) || target.item_id === raw.stone_item_id) return undefined;
        targets.set(target.item_id, target.quality);
    }
    const attributes = new Map<number, RefineAttribute>();
    for (const affix of raw.affixes) {
        if (!uint(affix.id, 1, 16) || typeof affix.name !== 'string' || !affix.name.trim() || !uint(affix.min_quality, 1, 4) ||
            attributes.has(affix.id)) return undefined;
        attributes.set(affix.id, { name: affix.name, minQuality: affix.min_quality });
    }
    const gold: { min: number; max: number; amount: bigint }[] = [];
    for (const band of raw.gold_bands) {
        const { min_level: min, max_level: max, gold: amount } = band;
        if (!uint(min, 1) || !uint(max) || (max !== 0 && max < min) || !uint(amount, 1) ||
            gold.some(previous => min <= (previous.max || Infinity) && previous.min <= (max || Infinity))) return undefined;
        gold.push({ min, max, amount: BigInt(amount) });
    }
    const locks = new Map<number, bigint>();
    for (const entry of raw.lock_costs) {
        if (!uint(entry.locked_slots, 0, 3) || !uint(entry.ingot_u) ||
            (entry.locked_slots === 0) !== (entry.ingot_u === 0) || locks.has(entry.locked_slots)) return undefined;
        locks.set(entry.locked_slots, BigInt(entry.ingot_u));
    }
    const tiers: { tier: number; weight: number }[] = [];
    for (const [index, row] of raw.tiers.entries()) {
        if (row.tier !== index + 1 || !uint(row.weight_ppm, 1, 1_000_000)) return undefined;
        tiers.push({ tier: row.tier, weight: row.weight_ppm });
    }
    const rec = raw.recycle;
    if (!uint(rec.item_id, 1) || !uint(rec.quality, 1, 4) || !uint(rec.stage, 0, 99) || rec.source_kind !== 'monster_drop' ||
        !uint(rec.probability_ppm, 1, 1_000_000) || !uint(rec.quantity, 1, 1) ||
        tiers.reduce((sum, row) => sum + row.weight, 0) !== 1_000_000) return undefined;
    return { stoneItemId: raw.stone_item_id, qualities, targets, attributes, gold, locks, tiers,
        recycle: { itemId: rec.item_id, source: rec.source_kind, quality: rec.quality, stage: rec.stage,
            probability: rec.probability_ppm, quantity: BigInt(rec.quantity) } };
}

export function equipmentRefineQuality(rules: EquipmentRefineRules | undefined, item: EquipmentItem | undefined): RefineQuality | undefined {
    const quality = item && rules?.targets.get(item.itemId);
    return quality === item?.quality && quality !== undefined ? rules?.qualities.get(quality) : undefined;
}

export function equipmentRefineGold(rules: EquipmentRefineRules, level: number): bigint | undefined {
    return rules.gold.find(band => level >= band.min && (level <= band.max || !band.max))?.amount;
}

// 精确显示百万分率，不使用浮点运算改写实际概率。
export function refineChanceText(ppm: number): string {
    const remainder = String(ppm % 10_000).padStart(4, '0').replace(/0+$/u, '');
    return `${Math.floor(ppm / 10_000)}${remainder ? '.' + remainder : ''}%`;
}

// 分解前展示当前隔离配方与原装备绑定，最终产出由已提交回执确认。
export function refineRecycleHint(rules: EquipmentRefineRules | undefined, item: EquipmentItem,
    definition: EquipmentDefinition | undefined, protectedReason = ''): string {
    const rec = rules?.recycle, durability = assetInteger(item.durabilityU ?? 0), maximum = assetInteger(item.maxDurabilityU ?? 0);
    if (!rec || protectedReason || item.itemId !== rec.itemId || item.quality !== rec.quality ||
        item.sourceKind !== rec.source || !/^[1-9]\d*$/u.test(item.sourceId ?? '') || definition?.equipment_stage !== rec.stage ||
        equipmentCurrentLuck(item) !== 0 || durability <= 0n || durability > maximum || maximum > assetInteger(item.originalDurabilityU ?? 0)) return '';
    return `洗炼石 ${refineChanceText(rec.probability)} · 命中得 ${rec.quantity}枚${item.bound ? '绑定' : '非绑'}石`;
}
