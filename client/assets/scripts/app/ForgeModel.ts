import Long from 'long';
import { equipmentSlotName } from './EquipmentModel';
import { assetInteger } from './SkillAssetsModel';

export interface ForgeRecipe {
    readonly blueprint_item_id: number;
    readonly output_item_id: number;
    readonly ore_item_id: number;
    readonly ore_count: number;
    readonly quality: number;
}

export interface ForgeDefinition {
    readonly item_id: number;
    readonly name: string;
    readonly kind: string;
    readonly required_level?: number;
    readonly equip_slot?: number;
}

export interface ForgeRef { readonly instanceId: Uint8Array; readonly version: Long; }
export interface ForgeItem extends ForgeRef {
    readonly itemId: number;
    readonly quantity: Long;
    readonly bound: boolean;
    readonly locked?: boolean;
    readonly recycleBlockFlags?: number;
    readonly orePurity?: number;
    readonly sourceKind?: string;
    readonly sourceId?: string;
}

export interface ForgeSnapshot {
    readonly scope: string;
    readonly role: string;
    readonly sceneEpoch: Long;
    readonly revision: Long;
    readonly items: readonly ForgeItem[];
    readonly catalog: readonly ForgeDefinition[];
    readonly recipes: readonly ForgeRecipe[];
    readonly blocked: boolean;
}

export interface ForgeQuoteRequest {
    readonly sceneEpoch: Long;
    readonly blueprint: ForgeRef;
    readonly ores: readonly ForgeRef[];
    readonly allowBound: boolean;
}

export interface ForgeQuote {
    readonly quoteId: Uint8Array;
    readonly sceneEpoch: Long;
    readonly roleRevision: Long;
    readonly expiresAtUnixMs: Long;
    readonly blueprint: ForgeItem;
    readonly ores: readonly ForgeItem[];
    readonly allowBound: boolean;
    readonly outputItemId: number;
    readonly quality: number;
    readonly requiredLevel: number;
    readonly slot: number;
    readonly successPpm: number;
    readonly gold: Long;
    readonly boundGold: Long;
    readonly outputBound: boolean;
}

export interface ForgeOutput {
    readonly instanceId: Uint8Array;
    readonly itemId: number;
    readonly bound: boolean;
    readonly quality: number;
    readonly initialLuck: number;
}

export interface ForgeReceipt {
    readonly quoteId: Uint8Array;
    readonly success: boolean;
    readonly output?: ForgeOutput | null;
    readonly gold: Long;
    readonly boundGold: Long;
}

const qualityName = ['普通', '精良', '稀有', '史诗', '传说'];
const maxRevision = 0x7fffffffffffffffn;
const id16 = (value: unknown): value is Uint8Array => value instanceof Uint8Array && value.length === 16;
const sameId = (left: Uint8Array, right: Uint8Array): boolean => id16(left) && id16(right) && left.every((byte, index) => byte === right[index]);
const count = (value: unknown, min: number, max: number): value is number => typeof value === 'number' && Number.isInteger(value) && value >= min && value <= max;
const positive = (value: unknown): value is Long => Long.isLong(value) && value.unsigned && !value.isZero();
const funds = (value: unknown): value is Long => Long.isLong(value) && !value.unsigned && !value.isNegative();
const usable = (item: ForgeItem): boolean => id16(item.instanceId) && positive(item.version) && assetInteger(item.version) < maxRevision &&
    Long.isLong(item.quantity) && !item.quantity.isNegative() && item.quantity.equals(Long.ONE) && typeof item.bound === 'boolean' && !item.locked && !item.recycleBlockFlags;
const quoteError = '原打造报价或材料已变化，请重新报价';

function copyRef(item: ForgeRef): ForgeRef { return { instanceId: new Uint8Array(item.instanceId), version: item.version }; }

function sameRef(left: ForgeRef, right: ForgeRef): boolean {
    return sameId(left.instanceId, right.instanceId) && positive(left.version) && positive(right.version) && left.version.equals(right.version);
}

function shortId(id: Uint8Array): string { return Array.from(id.slice(-3), byte => byte.toString(16).padStart(2, '0')).join(''); }

export function forgeRecipeFor(snapshot: ForgeSnapshot, itemId: number): ForgeRecipe | undefined {
    return snapshot.recipes.find(row => row.blueprint_item_id === itemId && count(row.ore_count, 1, 8) && count(row.quality, 0, 4) &&
        snapshot.catalog.some(item => item.item_id === row.blueprint_item_id && item.kind === 'forge_blueprint') &&
        snapshot.catalog.some(item => item.item_id === row.ore_item_id && item.kind === 'mining_ore') &&
        snapshot.catalog.some(item => item.item_id === row.output_item_id && item.kind === 'equipment'));
}

// 每次打开只认原图纸及原开窗附着，开关默认关闭，矿石选择保存实例和版本。
export class ForgeSelection {
    private readonly selected: ForgeRef[] = [];
    allowBound = false;
    private readonly blueprintRef: ForgeRef;

    constructor(readonly scope: string, blueprint: ForgeItem, readonly recipe: ForgeRecipe) {
        this.blueprintRef = copyRef(blueprint);
    }

    blueprint(snapshot: ForgeSnapshot): ForgeItem | undefined {
        if (snapshot.scope !== this.scope) return undefined;
        return snapshot.items.find(item => item.itemId === this.recipe.blueprint_item_id && usable(item) && sameRef(item, this.blueprintRef));
    }

    options(snapshot: ForgeSnapshot): readonly ForgeItem[] {
        if (snapshot.scope !== this.scope) return [];
        return snapshot.items.filter(item => item.itemId === this.recipe.ore_item_id && usable(item) && (!item.bound || this.allowBound) &&
            count(item.orePurity, 1, 100) && item.sourceKind === 'mining' && /^[0-9a-f]{32}$/.test(item.sourceId ?? ''));
    }

    chosen(snapshot: ForgeSnapshot): readonly ForgeItem[] {
        const options = this.options(snapshot);
        return this.selected.flatMap(ref => options.filter(option => sameRef(ref, option)));
    }

    has(snapshot: ForgeSnapshot, item: ForgeItem): boolean { return this.chosen(snapshot).some(option => sameRef(option, item)); }

    toggle(snapshot: ForgeSnapshot, item: ForgeItem): void {
        if (snapshot.blocked || !this.blueprint(snapshot) || !this.options(snapshot).some(option => sameRef(option, item))) return;
        const index = this.selected.findIndex(ref => sameRef(ref, item));
        if (index >= 0) { this.selected.splice(index, 1); return; }
        const current = this.chosen(snapshot);
        if (current.length >= this.recipe.ore_count) return;
        this.selected.splice(0, this.selected.length, ...current.map(copyRef), copyRef(item));
    }

    setAllowBound(snapshot: ForgeSnapshot, enabled: boolean): void {
        this.allowBound = enabled;
        this.selected.splice(0, this.selected.length, ...this.chosen(snapshot).map(copyRef));
    }

    request(snapshot: ForgeSnapshot): ForgeQuoteRequest | undefined {
        const blueprint = this.blueprint(snapshot), ores = this.chosen(snapshot);
        if (!blueprint || snapshot.blocked || blueprint.bound && !this.allowBound || ores.length !== this.recipe.ore_count) return undefined;
        return { sceneEpoch: snapshot.sceneEpoch, blueprint: copyRef(blueprint), ores: ores.map(copyRef), allowBound: this.allowBound };
    }
}

// 服务端报价绑定实际原料、场景与角色修订；角色变化后不能继续使用旧报价。
export function validateForgeQuote(quote: ForgeQuote, selection: ForgeSelection, snapshot: ForgeSnapshot): void {
    const recipe = selection.recipe, request = selection.request({ ...snapshot, blocked: false }), blueprint = selection.blueprint(snapshot), originals = selection.chosen(snapshot);
    const output = snapshot.catalog.find(item => item.item_id === recipe.output_item_id);
    if (!quote || !request || !blueprint || !id16(quote.quoteId) || !positive(quote.sceneEpoch) || !quote.sceneEpoch.equals(request.sceneEpoch) ||
        !positive(quote.roleRevision) || !quote.roleRevision.equals(snapshot.revision) || !positive(quote.expiresAtUnixMs) || quote.allowBound !== request.allowBound ||
        !quote.blueprint || !usable(quote.blueprint) || !sameRef(quote.blueprint, blueprint) || quote.blueprint.itemId !== blueprint.itemId ||
        quote.blueprint.bound !== blueprint.bound || !Array.isArray(quote.ores) || quote.ores.length !== originals.length ||
        quote.outputItemId !== recipe.output_item_id || output?.kind !== 'equipment' || quote.slot !== output.equip_slot ||
        quote.requiredLevel !== output.required_level || quote.quality !== recipe.quality || !count(quote.successPpm, 1, 1_000_000) ||
        !funds(quote.gold) || !funds(quote.boundGold)) throw new Error(quoteError);
    const gold = assetInteger(quote.gold), boundGold = assetInteger(quote.boundGold);
    if (gold + boundGold <= 0n || (!request.allowBound && boundGold !== 0n)) throw new Error(quoteError);
    const seen = new Set<ForgeItem>();
    for (const ore of quote.ores) {
        const original = originals.find(item => sameRef(item, ore));
        if (!original || !usable(ore) || seen.has(original) || ore.itemId !== original.itemId || ore.bound !== original.bound ||
            ore.orePurity !== original.orePurity || ore.sourceKind !== original.sourceKind || ore.sourceId !== original.sourceId) throw new Error(quoteError);
        seen.add(original);
    }
    if (quote.outputBound !== (blueprint.bound || originals.some(ore => ore.bound) || boundGold > 0n)) throw new Error(quoteError);
}

export function forgeReceipt(quoteId: Uint8Array, value: ForgeReceipt, expected?: ForgeQuote): ForgeReceipt {
    if (!value || !sameId(value.quoteId, quoteId) || typeof value.success !== 'boolean' || !funds(value.gold) || !funds(value.boundGold) ||
        assetInteger(value.gold) + assetInteger(value.boundGold) <= 0n || value.success !== !!value.output) throw new Error('打造回执与原订单不一致');
    const output = value.output;
    if (output && (!id16(output.instanceId) || !count(output.itemId, 1, 0xffffffff) || !count(output.quality, 0, 4) ||
        typeof output.bound !== 'boolean' || !count(output.initialLuck, 0, 3))) throw new Error('打造回执的原装备身份与属性不完整');
    if (expected && (!expected.gold.equals(value.gold) || !expected.boundGold.equals(value.boundGold) || output &&
        (output.itemId !== expected.outputItemId || output.bound !== expected.outputBound || output.quality !== expected.quality))) {
        throw new Error('打造回执金额或装备与已确认报价不同');
    }
    return value;
}

export const forgeChance = (quote: ForgeQuote): string => `${quote.successPpm / 10_000}%`;
export const forgeFeeLines = (quote: Pick<ForgeQuote, 'gold' | 'boundGold'>): string[] => [`金币 ${quote.gold}`, `绑定金币 ${quote.boundGold}`];
export const forgeOreLabel = (item: ForgeItem): string => `纯度${item.orePurity} ${item.bound ? '绑定' : '非绑'} ${shortId(item.instanceId)}`;

export function forgeQuoteLines(quote: ForgeQuote, name: (itemId: number) => string): string[] {
    return [`产物 ${name(quote.outputItemId)}`, `${quote.requiredLevel}级 ${qualityName[quote.quality]} ${equipmentSlotName(quote.slot)}`,
        `成功率 ${forgeChance(quote)} · ${quote.outputBound ? '产物绑定' : '产物非绑定'}`, ...forgeFeeLines(quote),
        '失败照扣黑铁矿和金币', '失败保留图纸',
        `原图纸 ${quote.blueprint.bound ? '绑定' : '非绑定'} ${shortId(quote.blueprint.instanceId)}`, `图纸版本 ${quote.blueprint.version}`,
        ...quote.ores.flatMap((ore, index) => [`矿石${index + 1} ${forgeOreLabel(ore)}`, `矿石${index + 1}版本 ${ore.version}`])];
}

export function forgeReceiptLines(receipt: ForgeReceipt, name: (itemId: number) => string): string[] {
    const fees = forgeFeeLines(receipt);
    return receipt.output ? [name(receipt.output.itemId), qualityName[receipt.output.quality], receipt.output.bound ? '装备绑定' : '装备非绑定',
        ...(receipt.output.initialLuck > 0 ? [`幸运+${receipt.output.initialLuck}`] : []), '已消耗图纸与黑铁矿', ...fees]
        : ['此次打造未成功', '已扣黑铁矿和金币', '原图纸保留', ...fees];
}
