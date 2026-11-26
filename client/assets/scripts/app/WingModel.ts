import Long from 'long';
import type { EquipmentItem } from './EquipmentModel';
import { assetId, sourceName } from './SkillAssetsModel';

// 翅膀为独立实例，不加入普通装备槽及背包物品数组。
export interface WingState {
    readonly wingId: Uint8Array;
    readonly rank: number;
    readonly appearanceTier: number;
    readonly hidden: boolean;
    readonly luck: number;
    readonly oilToSix: number;
    readonly oilToNine: number;
    readonly version: Long;
}

export interface WingRules {
    readonly featherItemId: number;
    readonly maxRank: number;
    readonly appearanceTiers: readonly number[];
    readonly rankCosts: readonly bigint[];
}

export interface WingSnapshot {
    readonly scope: string;
    readonly role: string;
    readonly sceneEpoch: Long;
    readonly revision: Long;
    readonly rules: WingRules;
    readonly wing: WingState | null;
    readonly inventory: readonly EquipmentItem[];
    readonly blocked: boolean;
    readonly allowLuckOil?: boolean;
}

export interface WingFeatherRef {
    readonly instanceId: Uint8Array;
    readonly version: Long;
    readonly quantity: Long;
}

export interface WingFeatherSpend { readonly item: EquipmentItem; readonly quantity: Long; }

export interface WingQuoteRequest {
    readonly sceneEpoch: Long;
    readonly wingId: Uint8Array;
    readonly wingVersion: Long;
    readonly feathers: readonly WingFeatherRef[];
    readonly allowBound: boolean;
    readonly uiSessionId: Uint8Array;
}

export interface WingQuote {
    readonly quoteId: Uint8Array;
    readonly sceneEpoch: Long;
    readonly wing?: WingState | null;
    readonly targetRank: number;
    readonly targetAppearanceTier: number;
    readonly feathers: readonly WingFeatherSpend[];
    readonly allowBound: boolean;
    readonly expiresAtUnixMs: Long;
    readonly roleRevision: Long;
    readonly uiSessionId: Uint8Array;
}

export interface WingGrowthAction {
    readonly action: 1;
    readonly quoteId: Uint8Array;
    readonly uiSessionId: Uint8Array;
    readonly allowBound: boolean;
    readonly wingId: Uint8Array;
    readonly wingVersion: Long;
}

export interface WingAppearanceAction {
    readonly action: 2;
    readonly wingId: Uint8Array;
    readonly wingVersion: Long;
    readonly appearanceTier: number;
    readonly hidden: boolean;
}

export type WingActionRequest = WingGrowthAction | WingAppearanceAction;

export interface WingReceipt {
    readonly action: 1 | 2;
    readonly wing: WingState;
    readonly quoteId?: Uint8Array | null;
    readonly feathers: readonly WingFeatherSpend[];
}

type Feather = EquipmentItem & { readonly version: Long; readonly quantity: Long };
const maxRevision = 0x7fffffffffffffffn;
const id16 = (value: unknown): value is Uint8Array => value instanceof Uint8Array && value.length === 16;
const sameId = (left: Uint8Array | undefined | null, right: Uint8Array | undefined | null): boolean =>
    id16(left) && id16(right) && assetId(left) === assetId(right);
const noId = (value: Uint8Array | undefined | null): boolean => value == null || value instanceof Uint8Array && value.length === 0;
const unsigned = (value: unknown): value is Long => Long.isLong(value) && value.unsigned;
const positive = (value: unknown): value is Long => unsigned(value) && !value.isZero();
const signedAmount = (value: unknown): value is Long => Long.isLong(value) && !value.unsigned && value.greaterThan(Long.ZERO);
const integer = (value: unknown, min: number, max = 0xffffffff): value is number =>
    typeof value === 'number' && Number.isSafeInteger(value) && value >= min && value <= max;
const big = (value: Long): bigint => BigInt(value.toString());
const min = (a: bigint, b: bigint): bigint => a < b ? a : b;

// 缺省候选时隐藏角色面板里的独立翅膀入口。
export function wingRules(value: unknown): WingRules | undefined {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
    const raw = value as { feather_item_id?: unknown; max_rank?: unknown; appearance_tiers?: unknown; ranks?: unknown };
    if (!integer(raw.feather_item_id, 1) || !integer(raw.max_rank, 1) || !Array.isArray(raw.appearance_tiers) ||
        raw.appearance_tiers[0] !== 1 || raw.appearance_tiers.some((tier, index, tiers) =>
            !integer(tier, 1, raw.max_rank as number) || index > 0 && tier <= tiers[index - 1]) ||
        !Array.isArray(raw.ranks) || !raw.ranks.length || raw.ranks.length > raw.max_rank || raw.ranks.some((row, index) =>
            !row || row.rank !== index + 1 || !integer(row.feathers, 1, Number.MAX_SAFE_INTEGER))) return undefined;
    // 成本仍待配置的阶位可以观察，不能当作零价升级或标成已满阶。
    return { featherItemId: raw.feather_item_id, maxRank: raw.max_rank, appearanceTiers: raw.appearance_tiers.slice(),
        rankCosts: raw.ranks.map(row => BigInt(row.feathers)) };
}

export function observedWingState(value: WingState, previous?: WingState): WingState {
    if (!value || !id16(value.wingId) || !integer(value.rank, 1) || !integer(value.appearanceTier, 1, value.rank) ||
        typeof value.hidden !== 'boolean' || !integer(value.luck, 0, 9) || !integer(value.oilToSix, 0) ||
        !integer(value.oilToNine, 0) || (value.luck < 3 && value.oilToSix > 0) || (value.luck < 6 && value.oilToNine > 0) ||
        !positive(value.version) || big(value.version) > maxRevision) {
        throw new Error('翅膀原实例状态无效');
    }
    if (previous && (!sameId(value.wingId, previous.wingId) || value.version.lessThan(previous.version) || value.rank < previous.rank ||
        value.oilToSix < previous.oilToSix || value.oilToNine < previous.oilToNine ||
        value.version.equals(previous.version) && !sameWing(value, previous))) throw new Error('翅膀原身份或版本已变化');
    return value;
}

export function nextWingCost(snapshot: WingSnapshot): bigint {
    return snapshot.rules.rankCosts[snapshot.wing?.rank ?? 0] ?? 0n;
}

export function wingSlotLabel(wing?: WingState | null): string {
    return wing ? `翅膀 · ${wing.rank}阶${wing.hidden ? ' · 外观隐藏' : ''}` : '翅膀 · 未激活';
}

export function wingStateLines(wing?: WingState | null): string[] {
    if (!wing) return ['当前未激活', '使用羽毛激活角色专属翅膀'];
    return [`翅膀 ${wing.rank}阶 · 角色专属`, `外观 ${wing.appearanceTier}阶 · ${wing.hidden ? '已隐藏' : '已显示'}`,
        `当前幸运+${wing.luck}`, `3→6段累计用油 ${wing.oilToSix}瓶`, `6→9段累计用油 ${wing.oilToNine}瓶`,
        `原翅膀尾号 ${assetId(wing.wingId).slice(-8)}`];
}

function feather(item: EquipmentItem | undefined): item is Feather {
    return !!item && id16(item.instanceId) && positive(item.version) && big(item.version) < maxRevision &&
        signedAmount(item.quantity) && typeof item.bound === 'boolean' && !item.locked && !(item.recycleBlockFlags ?? 0) &&
        !!item.sourceKind && !!item.sourceId;
}

function allowedFeathers(snapshot: WingSnapshot, allowBound: boolean): Feather[] {
    return snapshot.inventory.filter((item): item is Feather =>
        item.itemId === snapshot.rules.featherItemId && feather(item) && (allowBound || !item.bound));
}

export function wingFeatherLabel(item: EquipmentItem): string {
    return `${item.bound ? '绑定' : '非绑'}羽毛 ${item.quantity}根 · ${assetId(item.instanceId).slice(-6)}`;
}

function sameWing(left?: WingState | null, right?: WingState | null): boolean {
    if (!left || !right) return !left && !right;
    return sameId(left.wingId, right.wingId) && left.version.equals(right.version) && left.rank === right.rank &&
        left.appearanceTier === right.appearanceTier && left.hidden === right.hidden && left.luck === right.luck &&
        left.oilToSix === right.oilToSix && left.oilToNine === right.oilToNine;
}

function sameFeather(left: EquipmentItem, right: EquipmentItem): boolean {
    return feather(left) && feather(right) && sameId(left.instanceId, right.instanceId) && left.version.equals(right.version) &&
        left.quantity.equals(right.quantity) && left.bound === right.bound && left.itemId === right.itemId &&
        (left.sourceKind ?? '') === (right.sourceKind ?? '') && (left.sourceId ?? '') === (right.sourceId ?? '');
}

// 每项选择钉在原羽毛堆版本。失效选择只停止使用，不能自动换另一堆。
export class WingFeatherSelection {
    private readonly selected = new Map<string, WingFeatherRef>();
    private focus: Feather | undefined;
    allowBound = false;

    options(snapshot: WingSnapshot): Feather[] {
        return allowedFeathers(snapshot, this.allowBound).sort((a, b) => Number(b.bound) - Number(a.bound));
    }

    totals(snapshot: WingSnapshot): { free: bigint; bound: bigint } {
        let free = 0n, bound = 0n;
        for (const item of allowedFeathers(snapshot, true)) {
            if (item.bound) bound += big(item.quantity);
            else free += big(item.quantity);
        }
        return { free, bound };
    }

    chosen(snapshot: WingSnapshot): readonly WingFeatherRef[] {
        return this.options(snapshot).flatMap(item => {
            const ref = this.selected.get(assetId(item.instanceId));
            return ref?.version.equals(item.version) && ref.quantity.lessThanOrEqual(item.quantity) ? [ref] : [];
        });
    }

    count(snapshot: WingSnapshot, item: Feather): bigint {
        const ref = this.selected.get(assetId(item.instanceId));
        return ref?.version.equals(item.version) && ref.quantity.lessThanOrEqual(item.quantity) ? big(ref.quantity) : 0n;
    }

    total(snapshot: WingSnapshot): bigint {
        return this.chosen(snapshot).reduce((sum, ref) => sum + big(ref.quantity), 0n);
    }

    focused(snapshot: WingSnapshot): Feather | undefined {
        return this.options(snapshot).find(item => this.focus && sameFeather(item, this.focus));
    }

    pick(snapshot: WingSnapshot, item: Feather): void {
        if (snapshot.blocked || !this.options(snapshot).some(option => sameFeather(option, item))) return;
        this.focus = item;
        if (this.count(snapshot, item) === 0n) this.write(item, min(big(item.quantity), nextWingCost(snapshot) - this.total(snapshot)));
    }

    adjust(snapshot: WingSnapshot, delta: 1n | -1n): void {
        const item = this.focused(snapshot);
        if (!item || snapshot.blocked) return;
        const before = this.count(snapshot, item);
        const maximum = min(big(item.quantity), nextWingCost(snapshot) - this.total(snapshot) + before);
        this.write(item, min(maximum, before + delta));
    }

    setAllowBound(snapshot: WingSnapshot, enabled: boolean): void {
        this.allowBound = enabled;
        if (enabled) return;
        for (const item of allowedFeathers(snapshot, true)) if (item.bound) this.selected.delete(assetId(item.instanceId));
        if (this.focus?.bound) this.focus = undefined;
    }

    reset(): void {
        this.selected.clear();
        this.focus = undefined;
    }

    request(snapshot: WingSnapshot, uiSessionId: Uint8Array): WingQuoteRequest | undefined {
        const cost = nextWingCost(snapshot), feathers = this.chosen(snapshot);
        if (snapshot.blocked || !cost || !id16(uiSessionId) || !feathers.length || this.total(snapshot) !== cost ||
            snapshot.wing && big(snapshot.wing.version) >= maxRevision) return undefined;
        return { sceneEpoch: snapshot.sceneEpoch, wingId: new Uint8Array(snapshot.wing?.wingId ?? []),
            wingVersion: snapshot.wing?.version ?? Long.UZERO, feathers, allowBound: this.allowBound,
            uiSessionId: new Uint8Array(uiSessionId) };
    }

    private write(item: Feather, count: bigint): void {
        const key = assetId(item.instanceId);
        if (count <= 0n) { this.selected.delete(key); return; }
        this.selected.set(key, { instanceId: new Uint8Array(item.instanceId), version: item.version, quantity: Long.fromString(count.toString()) });
    }
}

// 保留服务端原报价的量价与来源。材料、翅膀或角色修订变化都须重新报价。
export function validateWingQuote(quote: WingQuote, request: WingQuoteRequest, snapshot: WingSnapshot): void {
    const error = '原翅膀或羽毛已变化，请重新报价';
    const next = (snapshot.wing?.rank ?? 0) + 1;
    const appearance = snapshot.rules.appearanceTiers.includes(next) ? next : snapshot.wing?.appearanceTier;
    if (quote?.wing) observedWingState(quote.wing);
    if (!quote || snapshot.blocked || !id16(quote.quoteId) || !sameId(quote.uiSessionId, request.uiSessionId) ||
        !positive(quote.sceneEpoch) || !quote.sceneEpoch.equals(snapshot.sceneEpoch) || !quote.sceneEpoch.equals(request.sceneEpoch) ||
        !positive(quote.roleRevision) || !quote.roleRevision.equals(snapshot.revision) || !positive(quote.expiresAtUnixMs) ||
        quote.allowBound !== request.allowBound || !sameWing(quote.wing, snapshot.wing) || quote.targetRank !== next ||
        quote.targetAppearanceTier !== appearance || !nextWingCost(snapshot) ||
        (snapshot.wing ? !sameId(request.wingId, snapshot.wing.wingId) || !request.wingVersion.equals(snapshot.wing.version)
            : !noId(request.wingId) || !request.wingVersion.isZero())) throw new Error(error);
    const chosen = new Map(request.feathers.map(ref => [assetId(ref.instanceId), ref]));
    const available = new Map(allowedFeathers(snapshot, request.allowBound).map(item => [assetId(item.instanceId), item]));
    if (!Array.isArray(quote.feathers) || quote.feathers.length !== chosen.size) throw new Error(error);
    let total = 0n;
    for (const spend of quote.feathers) {
        if (!spend || !feather(spend.item) || !signedAmount(spend.quantity)) throw new Error(error);
        const key = assetId(spend.item.instanceId), ref = chosen.get(key), item = available.get(key);
        if (!ref || !item || !sameFeather(item, spend.item) || !ref.version.equals(item.version) || !ref.quantity.equals(spend.quantity) ||
            spend.quantity.greaterThan(item.quantity)) throw new Error(error);
        chosen.delete(key);
        total += big(spend.quantity);
    }
    if (total !== nextWingCost(snapshot)) throw new Error(error);
}

export function wingGrowthAction(quote: WingQuote): WingGrowthAction {
    return { action: 1, quoteId: new Uint8Array(quote.quoteId), uiSessionId: new Uint8Array(quote.uiSessionId), allowBound: quote.allowBound,
        wingId: new Uint8Array(quote.wing?.wingId ?? []), wingVersion: quote.wing?.version ?? Long.UZERO };
}

export function wingAppearanceAction(snapshot: WingSnapshot, appearance: number, hidden: boolean): WingAppearanceAction | undefined {
    const wing = snapshot.wing;
    if (snapshot.blocked || !wing || big(wing.version) >= maxRevision || !snapshot.rules.appearanceTiers.includes(appearance) ||
        appearance > wing.rank || (wing.appearanceTier === appearance && wing.hidden === hidden)) return undefined;
    return { action: 2, wingId: new Uint8Array(wing.wingId), wingVersion: wing.version, appearanceTier: appearance, hidden };
}

// 重连核对原请求与永久回执，同一翅膀只推进一个版本；正常提交再对拍已经确认的预览。
export function validateWingReceipt(request: WingActionRequest, value: WingReceipt, expected?: WingQuote | WingState): WingReceipt {
    const error = '翅膀回执不匹配，保留原请求等待核对';
    if (!value || value.action !== request.action) throw new Error(error);
    const wing = observedWingState(value.wing), first = request.action === 1 && noId(request.wingId);
    if (!Array.isArray(value.feathers) || big(wing.version) !== big(request.wingVersion) + 1n ||
        (first ? wing.rank !== 1 || wing.appearanceTier !== 1 || wing.hidden || wing.luck || wing.oilToSix || wing.oilToNine
            : !sameId(wing.wingId, request.wingId))) throw new Error(error);
    if (request.action === 2) {
        if (!noId(value.quoteId) || value.feathers.length || wing.appearanceTier !== request.appearanceTier || wing.hidden !== request.hidden ||
            (expected && 'rank' in expected && !sameWing({ ...expected, version: wing.version, appearanceTier: wing.appearanceTier, hidden: wing.hidden }, wing))) {
            throw new Error(error);
        }
        return value;
    }
    if (!sameId(value.quoteId, request.quoteId) || !value.feathers.length) throw new Error(error);
    const spent = new Map<string, WingFeatherSpend>();
    for (const spend of value.feathers) {
        if (!spend || !feather(spend.item) || !signedAmount(spend.quantity) || spend.quantity.greaterThan(spend.item.quantity) ||
            (spend.item.bound && !request.allowBound)) throw new Error(error);
        const key = assetId(spend.item.instanceId);
        if (spent.has(key)) throw new Error(error);
        spent.set(key, spend);
    }
    if (expected && 'targetRank' in expected) {
        const prior = expected.wing;
        if (wing.rank !== expected.targetRank || wing.appearanceTier !== expected.targetAppearanceTier ||
            (prior && (wing.hidden !== prior.hidden || wing.luck !== prior.luck || wing.oilToSix !== prior.oilToSix || wing.oilToNine !== prior.oilToNine)) ||
            spent.size !== expected.feathers.length || expected.feathers.some(row => {
                const actual = spent.get(assetId(row.item.instanceId));
                return !actual || !sameFeather(row.item, actual.item) || !row.quantity.equals(actual.quantity);
            })) throw new Error(error);
    }
    return value;
}

export function wingSpendLines(rows: readonly WingFeatherSpend[]): string[] {
    return rows.flatMap(row => [`消耗 ${row.quantity}根${row.item.bound ? '绑定' : '非绑'}羽毛`,
        `原羽毛堆 ${assetId(row.item.instanceId).slice(-6)} · ${sourceName(row.item.sourceKind)}`]);
}
