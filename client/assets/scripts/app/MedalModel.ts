import Long from 'long';
import type { EquipmentItem } from './EquipmentModel';
import { assetId } from './SkillAssetsModel';

// 勋章属于当前角色。等级为零时没有持久状态，也没有背包物品或装备槽。
export interface MedalState { readonly level: number; readonly version: Long; }
export interface MedalLevel {
    readonly level: number;
    readonly cost: bigint;
    readonly mainAttack: number;
    readonly defense: number;
}
export interface MedalRules {
    readonly fragmentItemId: number;
    readonly levels: readonly MedalLevel[];
}
export interface MedalSnapshot {
    readonly scope: string;
    readonly role: string;
    readonly roleLevel: number;
    readonly sceneEpoch: Long;
    readonly revision: Long;
    readonly medal?: MedalState | null;
    readonly rules: MedalRules;
    readonly inventory: readonly EquipmentItem[];
    readonly blocked: boolean;
}

export interface MedalFragmentRef {
    readonly instanceId: Uint8Array;
    readonly version: Long;
    readonly quantity: Long;
}
export interface MedalFragmentSpend extends MedalFragmentRef { readonly bound: boolean; }
export interface MedalQuoteRequest {
    readonly sceneEpoch: Long;
    readonly medalVersion: Long;
    readonly fragments: readonly MedalFragmentRef[];
    readonly allowBound: boolean;
    readonly uiSessionId: Uint8Array;
}
export interface MedalQuote {
    readonly quoteId: Uint8Array;
    readonly sceneEpoch: Long;
    readonly medal?: MedalState | null;
    readonly targetLevel: number;
    readonly targetMainAttack: Long;
    readonly targetDefense: Long;
    readonly fragments: readonly MedalFragmentSpend[];
    readonly allowBound: boolean;
    readonly expiresAtUnixMs: Long;
    readonly roleRevision: Long;
    readonly uiSessionId: Uint8Array;
}
export interface MedalCommitRequest {
    readonly quoteId: Uint8Array;
    readonly uiSessionId: Uint8Array;
}
export interface MedalReceipt {
    readonly quoteId: Uint8Array;
    readonly beforeLevel: number;
    readonly medal: MedalState;
    readonly fragments: readonly MedalFragmentSpend[];
}

// 与服务端持久修订范围一致，数量始终用正有符号 64 位整数。
const maxRevision = 0x7fffffffffffffffn;
type Fragment = EquipmentItem & { readonly version: Long; readonly quantity: Long };
const asBig = (value: Long): bigint => BigInt(value.toString());
const id = (value: unknown): value is Uint8Array => value instanceof Uint8Array && value.length === 16;
const integer = (value: unknown, minimum: number, maximum = 0xffffffff): value is number =>
    typeof value === 'number' && Number.isSafeInteger(value) && value >= minimum && value <= maximum;
const unsigned = (value: unknown): value is Long => Long.isLong(value) && value.unsigned;
const revision = (value: unknown): value is Long => unsigned(value) && !value.isZero() && asBig(value) <= maxRevision;
const quantity = (value: unknown): value is Long => Long.isLong(value) && !value.unsigned && value.greaterThan(Long.ZERO);
const exactSigned = (value: unknown, expected: number): boolean =>
    Long.isLong(value) && !value.unsigned && asBig(value) === BigInt(expected);
const lower = (left: bigint, right: bigint): bigint => left < right ? left : right;

export function sameMedalId(left: unknown, right: unknown): boolean {
    return id(left) && id(right) && assetId(left) === assetId(right);
}

// 只有服务端随版本发布的连续逐级配置才显示入口和属性。
export function medalRules(value: unknown): MedalRules | undefined {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
    const raw = value as { fragment_item_id?: unknown; levels?: unknown };
    if (!integer(raw.fragment_item_id, 1) || !Array.isArray(raw.levels) || !raw.levels.length) return undefined;
    const levels: MedalLevel[] = [];
    for (const row of raw.levels) {
        const previous = levels[levels.length - 1];
        if (!row || row.level !== levels.length + 1 || !integer(row.fragment_cost, 1, Number.MAX_SAFE_INTEGER) ||
            !integer(row.main_attack, previous?.mainAttack ?? 0) || !integer(row.defense, previous?.defense ?? 0)) return undefined;
        levels.push({ level: row.level, cost: BigInt(row.fragment_cost), mainAttack: row.main_attack, defense: row.defense });
    }
    return { fragmentItemId: raw.fragment_item_id, levels };
}

export function observedMedal(value: MedalState | null | undefined, previous?: MedalState): MedalState {
    if (!value || !integer(value.level, 1) || !revision(value.version)) throw new Error('角色勋章状态无效');
    if (previous && (value.level < previous.level || value.version.lessThan(previous.version) ||
        (value.level === previous.level) !== value.version.equals(previous.version))) throw new Error('角色勋章等级或版本已变化');
    return value;
}

export function medalSlotLabel(medal?: MedalState | null): string {
    return medal ? `勋章 · ${medal.level}级` : '勋章 · 未激活';
}

export function medalLevel(medal: MedalState | null | undefined, rules: MedalRules): MedalLevel | undefined {
    return medal ? rules.levels[medal.level - 1] : undefined;
}

export function nextMedalLevel(snapshot: MedalSnapshot): MedalLevel | undefined {
    const level = (snapshot.medal?.level ?? 0) + 1;
    if (level > snapshot.roleLevel) return undefined;
    return snapshot.rules.levels[level - 1];
}

function eligible(item: EquipmentItem, rules: MedalRules): item is Fragment {
    return item.itemId === rules.fragmentItemId && id(item.instanceId) && revision(item.version) && asBig(item.version) < maxRevision &&
        quantity(item.quantity) && typeof item.bound === 'boolean' && !item.locked && !(item.recycleBlockFlags ?? 0) &&
        !!item.sourceKind && !!item.sourceId;
}

function allFragments(snapshot: MedalSnapshot): Fragment[] {
    return snapshot.inventory.filter(item => eligible(item, snapshot.rules));
}

export function medalFragmentLabel(item: EquipmentItem): string {
    return `${item.bound ? '绑定' : '非绑'}碎片 ${item.quantity}片 · ${assetId(item.instanceId).slice(-6)}`;
}

export function medalSpendLines(spends: readonly MedalFragmentSpend[]): string[] {
    return spends.map((spend, index) =>
        `${index + 1}. ${spend.bound ? '绑定' : '非绑'}碎片 ${spend.quantity}片 · ${assetId(spend.instanceId).slice(-6)}`);
}

function matchingRef(ref: MedalFragmentRef, item: Fragment): boolean {
    return ref.version.equals(item.version) && ref.quantity.lessThanOrEqual(item.quantity);
}

// 选择顺序决定提交清单顺序。旧堆变更时不换到别的实例，也不更改旧数量。
export class MedalFragmentSelection {
    private readonly selected = new Map<string, MedalFragmentSpend>();
    private focus?: MedalFragmentRef;
    private boundEnabled = false;

    get allowBound(): boolean { return this.boundEnabled; }

    options(snapshot: MedalSnapshot): Fragment[] {
        return allFragments(snapshot).filter(item => this.boundEnabled || !item.bound);
    }

    inventoryTotals(snapshot: MedalSnapshot): { bound: bigint; free: bigint } {
        return this.totals(allFragments(snapshot));
    }

    selectedTotals(snapshot: MedalSnapshot): { bound: bigint; free: bigint } {
        return this.totals(this.chosen(snapshot));
    }

    count(item: Fragment): bigint {
        const ref = this.selected.get(assetId(item.instanceId));
        return ref && ref.bound === item.bound && matchingRef(ref, item) ? asBig(ref.quantity) : 0n;
    }

    focused(snapshot: MedalSnapshot): Fragment | undefined {
        const focus = this.focus;
        return focus && this.options(snapshot).find(item => sameMedalId(item.instanceId, focus.instanceId) && item.version.equals(focus.version));
    }

    pick(snapshot: MedalSnapshot, picked: Fragment): void {
        const item = this.options(snapshot).find(option => sameMedalId(option.instanceId, picked.instanceId) &&
            option.version.equals(picked.version) && option.bound === picked.bound);
        if (!item || snapshot.blocked) return;
        const retained = new Set(this.chosen(snapshot).map(ref => assetId(ref.instanceId)));
        for (const key of Array.from(this.selected.keys())) if (!retained.has(key)) this.selected.delete(key);
        this.focus = item;
        if (!this.count(item)) this.adjust(snapshot, asBig(item.quantity));
    }

    adjust(snapshot: MedalSnapshot, amount: bigint): void {
        const item = this.focused(snapshot), next = nextMedalLevel(snapshot);
        if (!item || !next || snapshot.blocked) return;
        const before = this.count(item), total = this.selectedTotals(snapshot);
        const maximum = lower(asBig(item.quantity), next.cost - (total.bound + total.free - before));
        const chosen = lower(maximum, before + amount), key = assetId(item.instanceId);
        if (chosen <= 0n) this.selected.delete(key);
        else this.selected.set(key, { instanceId: new Uint8Array(item.instanceId), version: item.version,
            quantity: Long.fromString(chosen.toString()), bound: item.bound });
    }

    setAllowBound(enabled: boolean): void {
        this.boundEnabled = enabled;
        if (enabled) return;
        for (const ref of Array.from(this.selected.values())) if (ref.bound) this.selected.delete(assetId(ref.instanceId));
        this.focus = undefined;
    }

    reset(): void {
        this.selected.clear();
        this.focus = undefined;
    }

    request(snapshot: MedalSnapshot, uiSessionId: Uint8Array): MedalQuoteRequest | undefined {
        const next = nextMedalLevel(snapshot), version = snapshot.medal?.version ?? Long.UZERO;
        const selected = this.chosen(snapshot), totals = this.totals(selected);
        if (!next || snapshot.blocked || !id(uiSessionId) || asBig(version) >= maxRevision || !selected.length ||
            selected.length !== this.selected.size || totals.bound + totals.free !== next.cost) return undefined;
        const fragments = selected.map(({ instanceId, version, quantity }) => ({ instanceId, version, quantity }));
        return { sceneEpoch: snapshot.sceneEpoch, medalVersion: version, fragments, allowBound: this.boundEnabled,
            uiSessionId: new Uint8Array(uiSessionId) };
    }

    private chosen(snapshot: MedalSnapshot): MedalFragmentSpend[] {
        const available = new Map(this.options(snapshot).map(item => [assetId(item.instanceId), item]));
        return Array.from(this.selected.values()).filter(ref => {
            const item = available.get(assetId(ref.instanceId));
            return !!item && ref.bound === item.bound && matchingRef(ref, item);
        });
    }

    private totals(values: readonly { readonly bound: boolean; readonly quantity: Long }[]): { bound: bigint; free: bigint } {
        let bound = 0n, free = 0n;
        for (const item of values) {
            if (item.bound) bound += asBig(item.quantity);
            else free += asBig(item.quantity);
        }
        return { bound, free };
    }
}

function usableSpend(spend: MedalFragmentSpend): boolean {
    return !!spend && id(spend.instanceId) && revision(spend.version) && quantity(spend.quantity) && typeof spend.bound === 'boolean';
}

function sameSpend(left: MedalFragmentSpend, right: MedalFragmentSpend): boolean {
    return sameMedalId(left.instanceId, right.instanceId) && left.version.equals(right.version) &&
        left.quantity.equals(right.quantity) && left.bound === right.bound;
}

// 允许库存修订在报价后因其他收益推进，仍按原材料实例、数量、绑定和原角色报价确认。
export function validateMedalQuote(quote: MedalQuote, request: MedalQuoteRequest, snapshot: MedalSnapshot): void {
    const error = '勋章或碎片已变化，请重新报价';
    const next = nextMedalLevel(snapshot), before = snapshot.medal;
    const version = before?.version ?? Long.UZERO;
    if (quote?.medal) observedMedal(quote.medal);
    if (!quote || !next || snapshot.blocked || !id(quote.quoteId) || !sameMedalId(request.uiSessionId, quote.uiSessionId) ||
        !revision(quote.sceneEpoch) || !quote.sceneEpoch.equals(request.sceneEpoch) || !quote.sceneEpoch.equals(snapshot.sceneEpoch) ||
        !revision(quote.roleRevision) || quote.roleRevision.greaterThan(snapshot.revision) ||
        !revision(quote.expiresAtUnixMs) ||
        request.allowBound !== quote.allowBound || !request.medalVersion.equals(version) ||
        (quote.medal?.level ?? 0) !== (before?.level ?? 0) || !(quote.medal?.version ?? Long.UZERO).equals(version) ||
        quote.targetLevel !== next.level || !exactSigned(quote.targetMainAttack, next.mainAttack) || !exactSigned(quote.targetDefense, next.defense) ||
        !Array.isArray(quote.fragments) || quote.fragments.length !== request.fragments.length || !quote.fragments.length) throw new Error(error);
    const used = new Set<string>();
    const items = new Map(allFragments(snapshot).map(item => [assetId(item.instanceId), item]));
    let total = 0n;
    for (const [index, spend] of quote.fragments.entries()) {
        if (!usableSpend(spend) || spend.bound && !request.allowBound) throw new Error(error);
        const key = assetId(spend.instanceId), ref = request.fragments[index], item = items.get(key);
        if (used.has(key) || !item || !sameMedalId(ref.instanceId, spend.instanceId) || !matchingRef(spend, item) || !matchingRef(ref, item) ||
            !ref.quantity.equals(spend.quantity) || item.bound !== spend.bound) throw new Error(error);
        used.add(key);
        total += asBig(spend.quantity);
    }
    if (total !== next.cost) throw new Error(error);
}

// 永久回执重连时仍可独立校验，当前面板还须逐项对拍之前读到的报价。
export function validateMedalReceipt(request: MedalCommitRequest, receipt: MedalReceipt, expected?: MedalQuote): MedalReceipt {
    const error = '勋章回执不匹配，保留原请求等待核对';
    if (!receipt || !sameMedalId(request.quoteId, receipt.quoteId) || !integer(receipt.beforeLevel, 0) ||
        !Array.isArray(receipt.fragments) || !receipt.fragments.length) throw new Error(error);
    const medal = observedMedal(receipt.medal);
    if (medal.level !== receipt.beforeLevel + 1 || receipt.fragments.some(spend => !usableSpend(spend)) ||
        new Set(receipt.fragments.map(spend => assetId(spend.instanceId))).size !== receipt.fragments.length) throw new Error(error);
    if (expected && (receipt.beforeLevel !== (expected.medal?.level ?? 0) || medal.level !== expected.targetLevel ||
        asBig(medal.version) !== asBig(expected.medal?.version ?? Long.UZERO) + 1n ||
        receipt.fragments.length !== expected.fragments.length || receipt.fragments.some((spend, index) =>
            !sameSpend(spend, expected.fragments[index])))) throw new Error(error);
    return receipt;
}
