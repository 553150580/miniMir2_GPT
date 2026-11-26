// 洗炼模型核对原装备、原石堆、左右词条及本次服务端报价。
import Long from 'long';
import type { EquipmentDefinition, EquipmentItem, EquippedSelection } from './EquipmentModel';
import { equipmentRefineGold, equipmentRefineQuality, refineChanceText } from './EquipmentRefineRules';
import type { EquipmentRefineRules } from './EquipmentRefineRules';
import { assetId, SkillMaterialSelection, yuanbaoText } from './SkillAssetsModel';

const maxVersion = 0x7fffffffffffffffn;
export const refineId = (value: unknown): value is Uint8Array => value instanceof Uint8Array && value.length === 16;
export const sameRefineId = (left: Uint8Array, right: Uint8Array): boolean =>
    refineId(left) && refineId(right) && left.every((value, index) => value === right[index]);
export const refineVersion = (value: unknown): value is Long => Long.isLong(value) && value.unsigned &&
    !value.isZero() && BigInt(value.toString()) < maxVersion;
export const refineAmount = (value: unknown): value is Long => Long.isLong(value) && !value.unsigned && !value.isNegative();
const integer = (value: unknown, min: number, max: number): value is number =>
    typeof value === 'number' && Number.isSafeInteger(value) && value >= min && value <= max;
const fixed = (value: Long): bigint => BigInt(value.toString());

export interface RefineAffix {
    readonly slot: number;
    readonly attributeId: number;
    readonly tier: number;
    readonly valueUnits: Long;
}
export interface RefinePending {
    readonly pendingId: Uint8Array;
    readonly affixes: readonly RefineAffix[];
    readonly lockedSlots: readonly number[];
}
export interface RefineItem extends EquipmentItem {
    readonly version: Long;
    readonly quantity: Long;
    readonly refineAffixes?: readonly RefineAffix[];
    readonly refinePending?: RefinePending | null;
}
export interface RefineSnapshot {
    readonly role: string;
    readonly scope: string;
    readonly revision: Long;
    readonly sceneEpoch: Long;
    readonly items: readonly EquipmentItem[];
    readonly equipped: readonly EquippedSelection[];
    readonly catalog: readonly EquipmentDefinition[];
    readonly rules: EquipmentRefineRules;
    readonly blocked: boolean;
}
export interface RefineTarget {
    readonly instanceId: Uint8Array;
    readonly equipped: boolean;
    readonly version: Long;
}
export interface RefineStone {
    readonly instanceId: Uint8Array;
    readonly version: Long;
    readonly quantity: Long;
}
export interface RefineSpend {
    readonly item: RefineItem;
    readonly quantity: Long;
}
export interface RefineQuoteRequest {
    readonly sceneEpoch: Long;
    readonly target: RefineTarget;
    readonly lockedSlots: readonly number[];
    readonly stones: readonly RefineStone[];
    readonly allowBound: boolean;
    readonly uiSessionId: Uint8Array;
}
export interface RefineFees {
    readonly gold: Long;
    readonly boundGold: Long;
    readonly yuanbaoUnits: Long;
    readonly boundYuanbaoUnits: Long;
}
export interface RefineQuote extends RefineFees {
    readonly quoteId: Uint8Array;
    readonly uiSessionId: Uint8Array;
    readonly sceneEpoch: Long;
    readonly roleRevision: Long;
    readonly expiresAtUnixMs: Long;
    readonly target: RefineItem;
    readonly equipped: boolean;
    readonly lockedSlots: readonly number[];
    readonly stones: readonly RefineSpend[];
    readonly allowBound: boolean;
    readonly outputBound: boolean;
}
export type RefineAction = {
    readonly target: RefineTarget;
    readonly uiSessionId: Uint8Array;
} & ({ readonly action: 1; readonly quoteId: Uint8Array; readonly allowBound: boolean } |
    { readonly action: 2 | 3; readonly pendingId: Uint8Array });

export const sameRefineSlots = (left: readonly number[], right: readonly number[]): boolean =>
    left.length === right.length && left.every((slot, index) => slot === right[index]);

export function refineItemShape(value: unknown): value is RefineItem {
    if (!value || typeof value !== 'object') return false;
    const item = value as RefineItem;
    return refineId(item.instanceId) && integer(item.itemId, 1, 0xffffffff) && typeof item.bound === 'boolean' &&
        refineVersion(item.version) && refineAmount(item.quantity) && !item.quantity.isZero();
}

// 装备所在位置是操作身份的一部分；换装之后不会自动转用其它容器的实例。
export function findRefineItem(snapshot: RefineSnapshot, id: Uint8Array, equipped: boolean): RefineItem | undefined {
    const items = equipped ? snapshot.equipped.map(entry => entry.item) : snapshot.items;
    const item = items.find(value => sameRefineId(value.instanceId, id));
    return refineItemShape(item) && item.quantity.toString() === '1' ? item : undefined;
}

export function observedRefineAffixes(rows: readonly RefineAffix[] | undefined, slots: number, full = false): readonly RefineAffix[] | undefined {
    const values = rows ?? [];
    if (!Array.isArray(values) || values.length > slots || (full && values.length !== slots)) return undefined;
    const seen = new Set<number>();
    for (const row of values) {
        if (!row || !integer(row.slot, 0, slots - 1) || seen.has(row.slot) || !integer(row.attributeId, 1, 16) ||
            !integer(row.tier, 1, 5) || !refineAmount(row.valueUnits) || row.valueUnits.isZero()) return undefined;
        seen.add(row.slot);
    }
    return values;
}

export function observedRefinePending(item: RefineItem, slots: number): RefinePending | undefined {
    const pending = item.refinePending;
    if (!pending || !refineId(pending.pendingId) || !observedRefineAffixes(pending.affixes, slots, true) ||
        !Array.isArray(pending.lockedSlots) || pending.lockedSlots.length >= slots) return undefined;
    const current = observedRefineAffixes(item.refineAffixes, slots);
    if (!current) return undefined;
    const seen = new Set<number>();
    for (const slot of pending.lockedSlots) {
        const old = current.find(row => row.slot === slot), next = pending.affixes.find(row => row.slot === slot);
        if (!old || !next || seen.has(slot) || old.attributeId !== next.attributeId || old.tier !== next.tier ||
            !old.valueUnits.equals(next.valueUnits)) return undefined;
        seen.add(slot);
    }
    return pending;
}

// 只允许对符合快照规则的目标显示、选择锁槽和提交。已付待选的保护位允许当前洗炼流程继续。
export function refineTargetState(snapshot: RefineSnapshot, item: RefineItem) {
    const quality = equipmentRefineQuality(snapshot.rules, item);
    const definition = snapshot.catalog.find(row => row.item_id === item.itemId && row.kind === 'equipment');
    const gold = definition?.required_level ? equipmentRefineGold(snapshot.rules, definition.required_level) : undefined;
    const current = quality && observedRefineAffixes(item.refineAffixes, quality.slots);
    if (!quality || !definition || gold === undefined || !current) return undefined;
    const pending = observedRefinePending(item, quality.slots);
    const flags = item.recycleBlockFlags ?? 0;
    const known = (rows: readonly RefineAffix[]): boolean => rows.every(row =>
        (snapshot.rules.attributes.get(row.attributeId)?.minQuality ?? 5) <= (item.quality ?? 0)) &&
        (quality.repeats || new Set(rows.map(row => row.attributeId)).size === rows.length);
    const blocked = !integer(flags, 0, 15) || !!item.locked || (flags & 5) !== 0 ||
        ((flags & 2) !== 0 && !pending) || (!!item.refinePending && (!pending || !known(pending.affixes))) || !known(current);
    return { ...quality, definition, gold, current, pending, blocked };
}

export function freezeRefineTarget(item: RefineItem, equipped: boolean): RefineTarget {
    return { instanceId: new Uint8Array(item.instanceId), version: item.version, equipped };
}

export function availableRefineStones(snapshot: RefineSnapshot, allowBound: boolean): RefineItem[] {
    return snapshot.items.filter((item): item is RefineItem => item.itemId === snapshot.rules.stoneItemId &&
        refineItemShape(item) && !item.locked && !(item.recycleBlockFlags ?? 0) && !!item.sourceKind && !!item.sourceId &&
        (allowBound || !item.bound));
}

// 点堆按本次缺口选取，随后可用加减按钮精确选择。不会替换已变动的原材料。
export class RefineStoneSelection {
    private readonly selected = new SkillMaterialSelection();
    private focus?: { id: Uint8Array; version: Long };
    allowBound = false;

    clear(): void { this.selected.clear(); this.focus = undefined; }
    count(item: RefineItem): bigint { return this.selected.quantity(item); }
    options(snapshot: RefineSnapshot): RefineItem[] { return availableRefineStones(snapshot, this.allowBound); }
    focused(snapshot: RefineSnapshot): RefineItem | undefined {
        const focus = this.focus;
        return focus && this.options(snapshot).find(item => sameRefineId(item.instanceId, focus.id) && item.version.equals(focus.version));
    }
    private picks(snapshot: RefineSnapshot): RefineStone[] {
        const items = this.options(snapshot);
        this.selected.reconcile(items);
        return this.selected.selected(items).map(entry => ({ instanceId: entry.instanceId,
            version: Long.fromString(entry.version.toString(), true), quantity: Long.fromString(entry.quantity.toString()) }));
    }
    total(snapshot: RefineSnapshot): bigint { return this.picks(snapshot).reduce((sum, row) => sum + fixed(row.quantity), 0n); }
    pick(snapshot: RefineSnapshot, item: RefineItem, cost: bigint): void {
        const current = this.options(snapshot).find(row => sameRefineId(row.instanceId, item.instanceId) && row.version.equals(item.version));
        if (!current) return;
        this.focus = { id: new Uint8Array(current.instanceId), version: current.version };
        const remaining = cost - this.total(snapshot), quantity = fixed(current.quantity);
        if (this.count(current) > 0n) return;
        const amount = remaining > quantity ? quantity : remaining;
        if (amount > 0n) this.selected.adjust(current, Number(amount)); // 单次明确上限为 16 枚。
    }
    adjust(snapshot: RefineSnapshot, cost: bigint, delta: -1 | 1): void {
        const total = this.total(snapshot), item = this.focused(snapshot);
        if (!item) return;
        const count = this.count(item);
        if (delta < 0 ? count > 0n : count < fixed(item.quantity) && total < cost) this.selected.adjust(item, delta);
    }
    request(snapshot: RefineSnapshot, target: RefineItem, equipped: boolean, lockedSlots: readonly number[], uiSessionId: Uint8Array): RefineQuoteRequest | undefined {
        const state = refineTargetState(snapshot, target), stones = this.picks(snapshot);
        if (!state || state.blocked || snapshot.blocked || !refineId(uiSessionId) || stones.reduce((sum, row) => sum + fixed(row.quantity), 0n) !== state.stones ||
            lockedSlots.length >= state.slots || (lockedSlots.length > 0 && !snapshot.rules.locks.has(lockedSlots.length)) ||
            lockedSlots.some((slot, index) => (index > 0 && slot <= lockedSlots[index - 1]) || !state.current.some(affix => affix.slot === slot))) return undefined;
        return { sceneEpoch: snapshot.sceneEpoch, target: freezeRefineTarget(target, equipped), lockedSlots: lockedSlots.slice(),
            stones, allowBound: this.allowBound, uiSessionId: new Uint8Array(uiSessionId) };
    }
}

function spendMatches(spend: RefineSpend, ref: RefineStone, actual: RefineItem): boolean {
    return !!spend && refineItemShape(spend.item) && refineAmount(spend.quantity) && spend.quantity.equals(ref.quantity) &&
        sameRefineId(spend.item.instanceId, ref.instanceId) && spend.item.version.equals(ref.version) && spend.item.bound === actual.bound &&
        spend.item.itemId === actual.itemId && spend.item.quantity.equals(actual.quantity) &&
        spend.item.sourceId === actual.sourceId && spend.item.sourceKind === actual.sourceKind;
}

// 库存的独立修订可以提高，原装备、所有扣料及本次费用保持精确。
export function validateRefineQuote(quote: RefineQuote, request: RefineQuoteRequest, snapshot: RefineSnapshot): void {
    const item = findRefineItem(snapshot, request.target.instanceId, request.target.equipped);
    const state = item && refineTargetState(snapshot, item);
    if (!item || !state || state.blocked || !quote || !refineId(quote.quoteId) || !sameRefineId(quote.uiSessionId, request.uiSessionId) ||
        !refineVersion(quote.expiresAtUnixMs) ||
        !refineVersion(quote.roleRevision) || fixed(quote.roleRevision) > fixed(snapshot.revision) ||
        !Long.isLong(quote.sceneEpoch) || !quote.sceneEpoch.equals(request.sceneEpoch) || !quote.sceneEpoch.equals(snapshot.sceneEpoch) ||
        !refineItemShape(quote.target) || !sameRefineId(quote.target.instanceId, request.target.instanceId) ||
        !quote.target.version.equals(request.target.version) || !item.version.equals(request.target.version) || quote.target.bound !== item.bound ||
        quote.target.itemId !== item.itemId || quote.target.quantity.toString() !== '1' || quote.equipped !== request.target.equipped ||
        !Array.isArray(quote.lockedSlots) || !sameRefineSlots(quote.lockedSlots, request.lockedSlots) || quote.allowBound !== request.allowBound ||
        !Array.isArray(quote.stones) || quote.stones.length !== request.stones.length ||
        ![quote.gold, quote.boundGold, quote.yuanbaoUnits, quote.boundYuanbaoUnits].every(refineAmount)) throw new Error('洗炼报价已变化，请重新查看');
    const stock = availableRefineStones(snapshot, request.allowBound);
    const used = new Set<string>();
    for (const ref of request.stones) {
        const key = assetId(ref.instanceId);
        const actual = stock.find(row => sameRefineId(row.instanceId, ref.instanceId) && row.version.equals(ref.version));
        const spend = quote.stones.find(row => refineItemShape(row?.item) && sameRefineId(row.item.instanceId, ref.instanceId));
        if (used.has(key) || !actual || !spend || !spendMatches(spend, ref, actual)) throw new Error('原洗炼石已变化，请重新选择');
        used.add(key);
    }
    const paidGold = fixed(quote.gold) + fixed(quote.boundGold), paidIngot = fixed(quote.yuanbaoUnits) + fixed(quote.boundYuanbaoUnits);
    const boundSpend = quote.stones.some(row => row.item.bound) || !quote.boundGold.isZero() || !quote.boundYuanbaoUnits.isZero();
    if (paidGold !== state.gold || paidIngot !== (snapshot.rules.locks.get(request.lockedSlots.length) ?? 0n) ||
        (!request.allowBound && boundSpend) || quote.outputBound !== (item.bound || boundSpend)) throw new Error('洗炼报价的实际费用或绑定不符');
}

export function refineFeeLines(fees: RefineFees): string[] {
    return [`金币 ${fees.gold} · 绑定金币 ${fees.boundGold}`,
        `元宝 ${yuanbaoText(fees.yuanbaoUnits)} · 绑定元宝 ${yuanbaoText(fees.boundYuanbaoUnits)}`];
}

export function refineQualityChances(rules: EquipmentRefineRules): string[] {
    return rules.tiers.map(row => `${row.tier}档数值 · ${refineChanceText(row.weight)}`);
}

export function refineAffixText(rules: EquipmentRefineRules, affix: RefineAffix): string {
    const value = fixed(affix.valueUnits);
    return `${rules.attributes.get(affix.attributeId)?.name ?? '未知词条'} +${value / 10n}.${value % 10n}\n${affix.tier}档`;
}
