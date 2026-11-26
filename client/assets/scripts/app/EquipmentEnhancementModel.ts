import Long from 'long';
import { equipmentDurabilityText, equipmentSlotName } from './EquipmentModel';
import type { EquipmentItem, EquippedSelection } from './EquipmentModel';
import { assetId, sourceName } from './SkillAssetsModel';

interface EnhancementLevelRule {
    readonly stoneCost: bigint;
    readonly successPpm: number;
    readonly bonusBp: number;
}

interface EnhancementRules {
    readonly stoneItemId: number;
    readonly levels: readonly EnhancementLevelRule[];
}

// 当前附着的角色看见的库存、槽位及对应规则。
export interface EquipmentEnhancementSnapshot {
    readonly scope: string;
    readonly role: string;
    readonly sceneEpoch: Long;
    readonly revision: Long;
    readonly level: number;
    readonly slots: ReadonlyMap<number, number>;
    readonly rules: EnhancementRules;
    readonly items: readonly EquipmentItem[];
    readonly equipped: readonly EquippedSelection[];
    readonly catalog: readonly { readonly item_id: number; readonly name: string }[];
    readonly blocked: boolean;
}

interface StoneRef { readonly instanceId: Uint8Array; readonly version: Long; readonly quantity: Long; }
interface StoneSpend { readonly item: EquipmentItem; readonly quantity: Long; }

export interface EquipmentEnhancementQuoteRequest {
    readonly sceneEpoch: Long;
    readonly slot: number;
    readonly stones: readonly StoneRef[];
    readonly allowBound: boolean;
    readonly uiSessionId: Uint8Array;
}

export interface EquipmentEnhancementQuote {
    readonly quoteId: Uint8Array;
    readonly sceneEpoch: Long;
    readonly slot: number;
    readonly currentLevel: number;
    readonly targetLevel: number;
    readonly currentBonusBp: number;
    readonly targetBonusBp: number;
    readonly successPpm: number;
    readonly stones: readonly StoneSpend[];
    readonly allowBound: boolean;
    readonly expiresAtUnixMs: Long;
    readonly roleRevision: Long;
    readonly uiSessionId: Uint8Array;
}

export interface EquipmentEnhancementCommit {
    readonly quoteId: Uint8Array;
    readonly uiSessionId: Uint8Array;
    readonly slot: number;
}

export interface EquipmentEnhancementReceipt {
    readonly quoteId: Uint8Array;
    readonly slot: number;
    readonly beforeLevel: number;
    readonly afterLevel: number;
    readonly success: boolean;
    readonly stones: readonly StoneSpend[];
}

type StoneItem = EquipmentItem & { readonly version: Long; readonly quantity: Long };
const slotCount = 10;
const maxLevel = 50;
const maxRevision = 0x7fffffffffffffffn;
const positiveLong = (value: unknown): value is Long => Long.isLong(value) && !value.isZero() && !value.isNegative();
const id16 = (value: unknown): value is Uint8Array => value instanceof Uint8Array && value.length === 16;
const int = (value: unknown, minimum: number, maximum: number): value is number =>
    typeof value === 'number' && Number.isSafeInteger(value) && value >= minimum && value <= maximum;
const big = (value: Long): bigint => BigInt(value.toString());
const min = (a: bigint, b: bigint): bigint => a < b ? a : b;

// 只接受连续、显式的工作簿投影。缺少强化配置时沿用原装备面板。
export function equipmentEnhancementRules(value: unknown): EnhancementRules | undefined {
    if (!value || typeof value !== 'object') return undefined;
    const input = value as { stone_item_id?: unknown; levels?: unknown };
    if (!int(input.stone_item_id, 1, 0xffffffff) || !Array.isArray(input.levels) ||
        !input.levels.length || input.levels.length > maxLevel) return undefined;
    const levels: EnhancementLevelRule[] = [];
    for (const [index, row] of input.levels.entries()) {
        if (!row || row.current_level !== index || !int(row.stone_cost, 1, 0xffffffff) ||
            !int(row.success_ppm, 1, 1_000_000) || !int(row.bonus_bp, 1, 10_000)) return undefined;
        levels.push({ stoneCost: BigInt(row.stone_cost), successPpm: row.success_ppm, bonusBp: row.bonus_bp });
    }
    return { stoneItemId: input.stone_item_id, levels };
}

// 场景快照完整列出十个槽，空槽同样保有自身的等级。
export function observedEnhancementSlots(value: unknown, rules: EnhancementRules): ReadonlyMap<number, number> | undefined {
    if (!Array.isArray(value) || value.length !== slotCount) return undefined;
    const result = new Map<number, number>();
    for (const row of value) {
        if (!row || !int(row.slot, 0, slotCount - 1) || !int(row.level, 0, rules.levels.length) || result.has(row.slot)) return undefined;
        result.set(row.slot, row.level);
    }
    return result;
}

export function enhancementNext(snapshot: EquipmentEnhancementSnapshot, slot: number): EnhancementLevelRule | undefined {
    const current = snapshot.slots.get(slot);
    return current === undefined || current >= snapshot.level ? undefined : snapshot.rules.levels[current];
}

function enhancementBonus(rules: EnhancementRules, level: number): number {
    return rules.levels.slice(0, level).reduce((sum, row) => sum + row.bonusBp, 0);
}

// 按整数单位显示候选百分比，保留 0.1% 的真实分档差异。
function percent(value: number, scale: 100 | 10_000): string {
    const whole = Math.floor(value / scale);
    const remainder = String(value % scale).padStart(scale === 100 ? 2 : 4, '0').replace(/0+$/, '');
    return `${whole}${remainder ? '.' + remainder : ''}%`;
}

export function enhancementSlotLines(snapshot: EquipmentEnhancementSnapshot, slot: number): string[] {
    const level = snapshot.slots.get(slot);
    if (level === undefined) return ['当前部位状态尚未同步'];
    const worn = snapshot.equipped.find(entry => entry.slot === slot)?.item;
    const name = snapshot.catalog.find(value => value.item_id === worn?.itemId)?.name;
    const durability = worn ? equipmentDurabilityText(worn) : '';
    const contribution = percent(enhancementBonus(snapshot.rules, level), 100);
    const rules = enhancementNext(snapshot, slot);
    return [`${equipmentSlotName(slot)}强化 +${level}`, `基础属性加成 +${contribution}`, worn ? `当前穿戴 ${name ?? '装备'}` : '当前为空部位，穿戴后生效',
        ...(durability.includes('已损坏') ? ['装备已损坏，修好后生效'] : []),
        rules ? `下一级消耗 ${rules.stoneCost}颗 · 成功率 ${percent(rules.successPpm, 10_000)}` :
            level >= snapshot.level ? `当前角色等级上限 ${snapshot.level}` : '下一等级待开放'];
}

function stone(item: EquipmentItem | undefined): item is StoneItem {
    return !!item && int(item.itemId, 1, 0xffffffff) && id16(item.instanceId) && positiveLong(item.version) && item.version.unsigned && big(item.version) < maxRevision &&
        positiveLong(item.quantity) && !item.quantity.unsigned && typeof item.bound === 'boolean' && !item.locked &&
        !(item.recycleBlockFlags ?? 0);
}

function sameStone(left: StoneItem, right: StoneItem): boolean {
    return assetId(left.instanceId) === assetId(right.instanceId) && left.version.equals(right.version) && left.quantity.equals(right.quantity) &&
        left.itemId === right.itemId && left.bound === right.bound && left.sourceKind === right.sourceKind && left.sourceId === right.sourceId;
}

function observedStones(snapshot: EquipmentEnhancementSnapshot): StoneItem[] {
    return snapshot.items.filter((item): item is StoneItem => item.itemId === snapshot.rules.stoneItemId && stone(item));
}

export function enhancementStock(snapshot: EquipmentEnhancementSnapshot): { free: bigint; bound: bigint } {
    let free = 0n, bound = 0n;
    for (const item of observedStones(snapshot)) {
        if (item.bound) bound += big(item.quantity);
        else free += big(item.quantity);
    }
    return { free, bound };
}

export function enhancementStoneLabel(item: EquipmentItem): string {
    return `${item.bound ? '绑定' : '非绑'}石 ${item.quantity}颗 · ${assetId(item.instanceId).slice(-6)}`;
}

// 只记玩家点选的原堆与版本，库存变化后不自动换料。
export class EquipmentEnhancementSelection {
    private readonly refs = new Map<string, StoneRef>();
    private focus?: StoneItem;
    allowBound = false;

    options(snapshot: EquipmentEnhancementSnapshot): StoneItem[] {
        return observedStones(snapshot).filter(item => this.allowBound || !item.bound).sort((left, right) => Number(right.bound) - Number(left.bound));
    }

    count(item: StoneItem): bigint {
        const ref = this.refs.get(assetId(item.instanceId));
        return ref?.version.equals(item.version) && ref.quantity.lessThanOrEqual(item.quantity) ? big(ref.quantity) : 0n;
    }

    focused(snapshot: EquipmentEnhancementSnapshot): StoneItem | undefined {
        const focus = this.focus;
        return focus && this.options(snapshot).find(item => sameStone(item, focus));
    }

    total(snapshot: EquipmentEnhancementSnapshot): bigint {
        return this.options(snapshot).reduce((sum, item) => sum + this.count(item), 0n);
    }

    changed(snapshot: EquipmentEnhancementSnapshot): boolean {
        return this.options(snapshot).filter(item => this.count(item) > 0n).length !== this.refs.size;
    }

    pick(snapshot: EquipmentEnhancementSnapshot, slot: number, item: StoneItem): void {
        const current = this.options(snapshot).find(value => sameStone(value, item));
        const cost = enhancementNext(snapshot, slot)?.stoneCost;
        if (snapshot.blocked || !current || cost === undefined) return;
        this.focus = current;
        if (this.count(current) === 0n) this.write(current, min(big(current.quantity), cost - this.total(snapshot)));
    }

    adjust(snapshot: EquipmentEnhancementSnapshot, slot: number, delta: -1n | 1n): void {
        const item = this.focused(snapshot), cost = enhancementNext(snapshot, slot)?.stoneCost;
        if (snapshot.blocked || !item || cost === undefined) return;
        const count = this.count(item);
        this.write(item, min(min(big(item.quantity), cost - this.total(snapshot) + count), count + delta));
    }

    clear(): void {
        this.refs.clear();
        this.focus = undefined;
    }

    request(snapshot: EquipmentEnhancementSnapshot, slot: number, uiSessionId: Uint8Array): EquipmentEnhancementQuoteRequest | undefined {
        const cost = enhancementNext(snapshot, slot)?.stoneCost;
        if (snapshot.blocked || !cost || !id16(uiSessionId)) return undefined;
        const stones = this.options(snapshot).flatMap(item => this.count(item) ? [this.refs.get(assetId(item.instanceId))!] : []);
        if (stones.length !== this.refs.size || stones.reduce((sum, ref) => sum + big(ref.quantity), 0n) !== cost) return undefined;
        return { sceneEpoch: snapshot.sceneEpoch, slot, stones, allowBound: this.allowBound, uiSessionId: new Uint8Array(uiSessionId) };
    }

    private write(item: StoneItem, quantity: bigint): void {
        const key = assetId(item.instanceId);
        if (quantity <= 0n) this.refs.delete(key);
        else this.refs.set(key, { instanceId: new Uint8Array(item.instanceId), version: item.version, quantity: Long.fromString(quantity.toString()) });
    }
}

function spendMap(values: readonly StoneSpend[], error: string): Map<string, StoneSpend & { item: StoneItem }> {
    if (!Array.isArray(values) || !values.length) throw new Error(error);
    const result = new Map<string, StoneSpend & { item: StoneItem }>();
    for (const spend of values) {
        if (!spend || !stone(spend.item) || !positiveLong(spend.quantity) || spend.quantity.unsigned || spend.quantity.greaterThan(spend.item.quantity)) throw new Error(error);
        const key = assetId(spend.item.instanceId);
        if (result.has(key)) throw new Error(error);
        result.set(key, { item: spend.item, quantity: spend.quantity });
    }
    return result;
}

// 核对报价的角色、概率和每个原堆，报价失效只能重新预览。
export function validateEquipmentEnhancementQuote(quote: EquipmentEnhancementQuote, request: EquipmentEnhancementQuoteRequest,
    snapshot: EquipmentEnhancementSnapshot): void {
    const error = '部位、材料或强化配置已变化，请重新报价';
    const current = snapshot.slots.get(request.slot), next = enhancementNext(snapshot, request.slot);
    if (!quote || snapshot.blocked || current === undefined || !next || !id16(quote.quoteId) || !id16(quote.uiSessionId) ||
        assetId(quote.uiSessionId) !== assetId(request.uiSessionId) || !positiveLong(quote.sceneEpoch) || !quote.sceneEpoch.unsigned ||
        !quote.sceneEpoch.equals(snapshot.sceneEpoch) || !quote.sceneEpoch.equals(request.sceneEpoch) || !positiveLong(quote.roleRevision) ||
        !quote.roleRevision.unsigned || quote.roleRevision.greaterThan(snapshot.revision) || !positiveLong(quote.expiresAtUnixMs) ||
        !quote.expiresAtUnixMs.unsigned || quote.slot !== request.slot || quote.currentLevel !== current || quote.targetLevel !== current + 1 ||
        quote.currentBonusBp !== enhancementBonus(snapshot.rules, current) || quote.targetBonusBp !== quote.currentBonusBp + next.bonusBp ||
        quote.successPpm !== next.successPpm || quote.allowBound !== request.allowBound) throw new Error(error);
    const spend = spendMap(quote.stones, error);
    const items = new Map(observedStones(snapshot).map(item => [assetId(item.instanceId), item]));
    if (spend.size !== request.stones.length) throw new Error(error);
    let total = 0n;
    for (const ref of request.stones) {
        const key = assetId(ref.instanceId), original = items.get(key), quoted = spend.get(key);
        if (!original || !quoted || (original.bound && !request.allowBound) || !sameStone(original, quoted.item) ||
            !ref.version.equals(original.version) || !ref.quantity.equals(quoted.quantity)) throw new Error(error);
        spend.delete(key);
        total += big(quoted.quantity);
    }
    if (total !== next.stoneCost) throw new Error(error);
}

export function enhancementCommit(quote: EquipmentEnhancementQuote): EquipmentEnhancementCommit {
    return { quoteId: new Uint8Array(quote.quoteId), uiSessionId: new Uint8Array(quote.uiSessionId), slot: quote.slot };
}

// 重连没有原报价快照时仍核对原报价号、槽位和结果结构，再清除本地未决请求。
export function validateEquipmentEnhancementReceipt(request: EquipmentEnhancementCommit, receipt: EquipmentEnhancementReceipt,
    quote?: EquipmentEnhancementQuote): EquipmentEnhancementReceipt {
    const error = '强化回执的原部位、等级或投入无效';
    if (!receipt || !id16(receipt.quoteId) || assetId(receipt.quoteId) !== assetId(request.quoteId) ||
        receipt.slot !== request.slot || !int(receipt.slot, 0, slotCount - 1) || !int(receipt.beforeLevel, 0, maxLevel - 1) ||
        typeof receipt.success !== 'boolean' || receipt.afterLevel !== receipt.beforeLevel + Number(receipt.success)) throw new Error(error);
    const actual = spendMap(receipt.stones, error);
    if (new Set(Array.from(actual.values(), ({ item }) => item.itemId)).size !== 1) throw new Error(error);
    if (quote) {
        const expected = spendMap(quote.stones, error);
        if (receipt.beforeLevel !== quote.currentLevel || actual.size !== expected.size || quote.successPpm === 1_000_000 && !receipt.success) throw new Error(error);
        for (const [key, spent] of actual) {
            const original = expected.get(key);
            if (!original || !sameStone(original.item, spent.item) || !original.quantity.equals(spent.quantity)) throw new Error(error);
        }
    }
    return receipt;
}

function enhancementSpendLines(values: readonly StoneSpend[]): string[] {
    return values.map(({ item, quantity }) => `${item.bound ? '绑定' : '非绑'}石 ${quantity}颗 · ${assetId(item.instanceId).slice(-6)} · ${sourceName(item.sourceKind)}`);
}

export function enhancementQuoteLines(quote: EquipmentEnhancementQuote): string[] {
    return [`${equipmentSlotName(quote.slot)} +${quote.currentLevel} → +${quote.targetLevel}`,
        `基础属性加成 +${percent(quote.currentBonusBp, 100)} → +${percent(quote.targetBonusBp, 100)}`,
        `成功率 ${percent(quote.successPpm, 10_000)}`, ...enhancementSpendLines(quote.stones),
        '失败保持原等级，强化石照常消耗', '强化归属部位，穿戴装备的绑定保持原样'];
}

export function enhancementReceiptLines(receipt: EquipmentEnhancementReceipt): string[] {
    return [`${equipmentSlotName(receipt.slot)}${receipt.success ? '强化成功' : '强化未成功'}`,
        `部位强化 +${receipt.beforeLevel} → +${receipt.afterLevel}`, ...enhancementSpendLines(receipt.stones), '原扣石结果已由服务端确认'];
}

// 无关收益可能先推进角色修订，仍要看到本次原石堆版本改变后才开放下一笔。
export function enhancementReceiptObserved(snapshot: EquipmentEnhancementSnapshot, receipt: EquipmentEnhancementReceipt, quotedRevision: Long): boolean {
    if (!snapshot.revision.greaterThan(quotedRevision) || (snapshot.slots.get(receipt.slot) ?? -1) < receipt.afterLevel) return false;
    return receipt.stones.every(({ item }) => {
        const current = snapshot.items.find(candidate => id16(candidate.instanceId) && assetId(candidate.instanceId) === assetId(item.instanceId));
        return !current || positiveLong(current.version) && current.version.greaterThan(item.version as Long);
    });
}
