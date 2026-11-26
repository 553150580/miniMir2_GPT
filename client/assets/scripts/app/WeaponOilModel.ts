import Long from 'long';
import { equipmentCurrentLuck } from './EquipmentModel';
import type { EquipmentDefinition, EquipmentItem, EquippedSelection } from './EquipmentModel';
import { assetId, sourceName } from './SkillAssetsModel';
import { observedWingState } from './WingModel';
import type { WingState } from './WingModel';

export interface WeaponOilSnapshot {
    readonly scope: string;
    readonly role: string;
    readonly sceneEpoch: Long;
    readonly revision: Long;
    readonly items: readonly EquipmentItem[];
    readonly equipped: readonly EquippedSelection[];
    readonly catalog: readonly (EquipmentDefinition & { readonly name: string })[];
    readonly oilItemId: number;
    readonly blocked: boolean;
    readonly wing?: WingState | null;
    readonly wingContributionPpm?: readonly number[];
}

export interface WeaponOilTarget { readonly instanceId: Uint8Array; readonly equipped: boolean; }
export interface WingOilTarget { readonly wingId: Uint8Array; }
export type LuckOilTarget = WeaponOilTarget | WingOilTarget;

export interface WeaponOilItemRef extends WeaponOilTarget { readonly version: Long; }
export interface WingOilRef extends WingOilTarget { readonly version: Long; }

type OilRef = { readonly weapon: WeaponOilItemRef; readonly wing?: null } |
    { readonly wing: WingOilRef; readonly weapon?: null };
type OilState = { readonly weapon: EquipmentItem; readonly wing?: null } |
    { readonly wing: WingState; readonly weapon?: null };

export type WeaponOilQuoteRequest = OilRef & {
    readonly sceneEpoch: Long;
    readonly allowBound: boolean;
    readonly uiSessionId: Uint8Array;
};

export type WeaponOilCommitRequest = OilRef & {
    readonly quoteId: Uint8Array;
    readonly allowBound: boolean;
    readonly uiSessionId: Uint8Array;
};

export type WeaponOilQuote = OilState & {
    readonly quoteId: Uint8Array;
    readonly uiSessionId: Uint8Array;
    readonly sceneEpoch: Long;
    readonly roleRevision: Long;
    readonly expiresAtUnixMs: Long;
    readonly oil: EquipmentItem;
    readonly equipped: boolean;
    readonly allowBound: boolean;
    readonly currentLuck: number;
    readonly successLuck: number;
    readonly failureLuck: number;
    readonly successPpm: number;
    readonly pityUsed: number;
    readonly pityAfter: number;
    readonly guaranteed: boolean;
    readonly outputBound: boolean;
};

export type WeaponOilReceipt = OilState & {
    readonly quoteId: Uint8Array;
    readonly oil: EquipmentItem;
    readonly equipped: boolean;
    readonly beforeLuck: number;
    readonly success: boolean;
    readonly guaranteed: boolean;
    readonly pityUsed: number;
    readonly pityAfter: number;
};

const maxRevision = 0x7fffffffffffffffn;
const id16 = (value: unknown): value is Uint8Array => value instanceof Uint8Array && value.length === 16;
const sameId = (left: Uint8Array, right: Uint8Array): boolean => id16(left) && id16(right) && assetId(left) === assetId(right);
const inRange = (value: number, min: number, max: number): boolean => Number.isInteger(value) && value >= min && value <= max;
const positiveLong = (value: unknown): value is Long => Long.isLong(value) && value.unsigned && !value.isZero();
const itemQuantity = (value: unknown): value is Long => Long.isLong(value) && !value.unsigned && value.greaterThan(Long.ZERO);
const failureLuck = (luck: number): number => luck < 3 ? luck : luck < 6 ? 3 : 6;
const quoteError = '原培养对象、油堆或报价已变化，请重新报价';
type ObservedItem = EquipmentItem & { readonly version: Long; readonly quantity: Long };

function validItem(item: EquipmentItem | null | undefined): item is ObservedItem {
    return !!item && id16(item.instanceId) && positiveLong(item.version) && BigInt(item.version.toString()) <= maxRevision &&
        itemQuantity(item.quantity) && inRange(item.itemId, 1, 0xffffffff) && typeof item.bound === 'boolean';
}

function usableItem(item: EquipmentItem | undefined, equipped: boolean): item is ObservedItem {
    return validItem(item) && BigInt(item.version.toString()) < maxRevision && !item.locked &&
        inRange(item.recycleBlockFlags ?? 0, 0, 15) && !((item.recycleBlockFlags ?? 0) & (equipped ? 7 : 15));
}

function sameObservedItem(left: EquipmentItem | null | undefined, right: EquipmentItem | null | undefined): boolean {
    return validItem(left) && validItem(right) && sameId(left.instanceId, right.instanceId) && left.itemId === right.itemId &&
        left.version.equals(right.version) && left.quantity.equals(right.quantity) && left.bound === right.bound &&
        (left.sourceKind ?? '') === (right.sourceKind ?? '') && (left.sourceId ?? '') === (right.sourceId ?? '');
}

// 公开目录未启用候选时，普通背包与角色界面不出现用油入口。
export function weaponOilItemId(value: unknown): number {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return 0;
    const id = (value as { item_id?: number }).item_id;
    return id !== undefined && inRange(id, 1, 0xffffffff) ? id : 0;
}

// 贡献档位来自同一翅膀候选，缺条件时保持原武器培养入口。
export function wingOilContributions(value: unknown): readonly number[] | undefined {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
    const { allow_luck_oil: allowed, luck_contribution_ppm: ppm } = value as { allow_luck_oil?: unknown; luck_contribution_ppm?: unknown };
    return allowed === true && Array.isArray(ppm) && ppm.length === 10 && ppm[0] === 0 &&
        ppm.every((step, index) => inRange(step, 0, 1_000_000) && (index === 0 || step >= ppm[index - 1])) ? ppm : undefined;
}

export function wingOilTarget(snapshot: WeaponOilSnapshot, target?: WingOilTarget): WingState | undefined {
    const wing = snapshot.wing;
    return snapshot.wingContributionPpm && wing && (!target || sameId(wing.wingId, target.wingId)) ? wing : undefined;
}

export function weaponOilTarget(snapshot: WeaponOilSnapshot, target: WeaponOilTarget): EquipmentItem | undefined {
    const item = target.equipped
        ? snapshot.equipped.find(entry => entry.slot === 0 && sameId(entry.item.instanceId, target.instanceId))?.item
        : snapshot.items.find(entry => sameId(entry.instanceId, target.instanceId));
    const definition = item && snapshot.catalog.find(row => row.item_id === item.itemId);
    return definition?.kind === 'equipment' && definition.equip_slot === 0 ? item : undefined;
}

export function weaponOilTargets(snapshot: WeaponOilSnapshot): readonly { target: WeaponOilTarget; item: EquipmentItem }[] {
    const worn = snapshot.equipped.find(entry => entry.slot === 0)?.item;
    const weaponIds = new Set(snapshot.catalog.filter(row => row.kind === 'equipment' && row.equip_slot === 0).map(row => row.item_id));
    const entries = snapshot.items.filter(item => weaponIds.has(item.itemId)).map(item => ({ target: { instanceId: item.instanceId, equipped: false }, item }));
    if (worn && weaponIds.has(worn.itemId)) entries.unshift({ target: { instanceId: worn.instanceId, equipped: true }, item: worn });
    return entries;
}

export function weaponOilAvailable(snapshot: WeaponOilSnapshot): { free: bigint; bound: bigint } {
    let free = 0n, bound = 0n;
    for (const item of snapshot.items) {
        if (item.itemId !== snapshot.oilItemId || !usableItem(item, false)) continue;
        if (item.bound) bound += BigInt(item.quantity.toString());
        else free += BigInt(item.quantity.toString());
    }
    return { free, bound };
}

export function weaponOilBlock(snapshot: WeaponOilSnapshot, target: LuckOilTarget, allowBound: boolean): string {
    if ('wingId' in target) {
        const wing = wingOilTarget(snapshot, target);
        if (!wing || !positiveLong(wing.version) || BigInt(wing.version.toString()) >= maxRevision) return '原翅膀已变化，请重选';
        if (wing.luck >= 9) return '幸运已达+9，无需消耗';
    } else {
        const item = weaponOilTarget(snapshot, target);
        if (!item) return '原武器已离开此位置，请重选';
        if (!usableItem(item, target.equipped) || item.quantity.toString() !== '1') return '当前武器已锁定或被占用';
        if (equipmentCurrentLuck(item) >= 9) return '幸运已达+9，无需消耗';
    }
    if (snapshot.blocked) return '当前角色暂不能使用祝福油';
    const stock = weaponOilAvailable(snapshot);
    if (stock.free === 0n && !(allowBound && stock.bound > 0n)) return stock.bound ? '开启绑材后才能使用绑定油' : '背包没有可用祝福油';
    return '';
}

function copyRef(item: EquipmentItem, equipped: boolean): WeaponOilItemRef {
    return { instanceId: new Uint8Array(item.instanceId), equipped, version: Long.fromString(item.version.toString(), true) };
}

export function weaponOilRequest(snapshot: WeaponOilSnapshot, target: LuckOilTarget, allowBound: boolean, uiSessionId: Uint8Array): WeaponOilQuoteRequest | undefined {
    if (weaponOilBlock(snapshot, target, allowBound) || !id16(uiSessionId)) return undefined;
    const terms = { sceneEpoch: snapshot.sceneEpoch, allowBound, uiSessionId: new Uint8Array(uiSessionId) };
    return 'wingId' in target ? { ...terms, wing: wingRef(wingOilTarget(snapshot, target)!) } :
        { ...terms, weapon: copyRef(weaponOilTarget(snapshot, target)!, target.equipped) };
}

function wingRef(wing: WingState): WingOilRef {
    return { wingId: new Uint8Array(wing.wingId), version: wing.version };
}

function sameRef(ref: WeaponOilItemRef, item: EquipmentItem | null | undefined): boolean {
    return validItem(item) && sameId(ref.instanceId, item.instanceId) && positiveLong(ref.version) && ref.version.equals(item.version);
}

function wingPity(wing: WingState, luck: number): number {
    return luck < 3 ? 0 : luck < 6 ? wing.oilToSix : wing.oilToNine;
}

// 服务端决定赔率和原油堆。客户端只核对当前身份、实物、保底档位和绑定结果。
export function validateWeaponOilQuote(quote: WeaponOilQuote, request: WeaponOilQuoteRequest, snapshot: WeaponOilSnapshot): void {
    const target = request.wing ?? request.weapon;
    if (!quote || !target || !!request.wing === !!request.weapon || !!quote.wing === !!quote.weapon ||
        weaponOilBlock(snapshot, target, request.allowBound) || !id16(quote.quoteId) ||
        !sameId(quote.uiSessionId, request.uiSessionId) || !positiveLong(quote.sceneEpoch) || !quote.sceneEpoch.equals(request.sceneEpoch) ||
        !quote.sceneEpoch.equals(snapshot.sceneEpoch) || !positiveLong(quote.roleRevision) || !quote.roleRevision.equals(snapshot.revision) ||
        !positiveLong(quote.expiresAtUnixMs) || quote.allowBound !== request.allowBound) throw new Error(quoteError);
    let luck: number;
    if (request.wing) {
        const wing = wingOilTarget(snapshot, request.wing);
        if (!wing || !quote.wing || quote.equipped !== false || quote.outputBound !== true ||
            !request.wing.version.equals(wing.version) || !positiveLong(quote.wing.version) || !quote.wing.version.equals(wing.version) ||
            quote.pityUsed !== wingPity(wing, wing.luck)) throw new Error(quoteError);
        observedWingState(quote.wing, wing);
        luck = wing.luck;
    } else {
        const weapon = weaponOilTarget(snapshot, request.weapon);
        if (!weapon || !quote.weapon || quote.equipped !== request.weapon.equipped || !sameRef(request.weapon, quote.weapon) ||
            !sameObservedItem(weapon, quote.weapon) || quote.outputBound !== (weapon.bound || quote.oil?.bound) ||
            equipmentCurrentLuck(quote.weapon) !== equipmentCurrentLuck(weapon)) throw new Error(quoteError);
        luck = equipmentCurrentLuck(weapon);
    }
    const oil = validItem(quote.oil) && snapshot.items.find(item => sameId(item.instanceId, quote.oil.instanceId));
    const stock = weaponOilAvailable(snapshot);
    if (!oil || !usableItem(oil, false) || !sameObservedItem(oil, quote.oil) || oil.itemId !== snapshot.oilItemId ||
        oil.bound !== (request.allowBound && stock.bound > 0n)) throw new Error(quoteError);
    if (!inRange(quote.currentLuck, 0, 8) || quote.currentLuck !== luck ||
        quote.successLuck !== luck + 1 || quote.failureLuck !== failureLuck(luck) || !inRange(quote.successPpm, 1, 1_000_000) ||
        !inRange(quote.pityAfter, luck < 3 ? 0 : 1, luck < 3 ? 0 : 0xffffffff) ||
        !inRange(quote.pityUsed, 0, luck < 3 ? 0 : 0xffffffff) ||
        quote.guaranteed !== (quote.pityAfter > 0 && quote.pityUsed >= quote.pityAfter) ||
        quote.guaranteed && quote.successPpm !== 1_000_000) throw new Error(quoteError);
}

export function weaponOilCommit(quote: WeaponOilQuote): WeaponOilCommitRequest {
    const terms = { quoteId: new Uint8Array(quote.quoteId), uiSessionId: new Uint8Array(quote.uiSessionId), allowBound: quote.allowBound };
    return quote.wing ? { ...terms, wing: wingRef(quote.wing) } : { ...terms, weapon: copyRef(quote.weapon, quote.equipped) };
}

// 重连只有原请求及永久回执时也校验原实例、版本推进和合法结果，不以新库存替代原证据。
export function weaponOilReceipt(request: WeaponOilCommitRequest, value: WeaponOilReceipt, expected?: WeaponOilQuote): WeaponOilReceipt {
    const error = '祝福油回执与原操作不符，保留原请求等待核对';
    if (!value || !!request.wing === !!request.weapon || !!value.wing === !!value.weapon ||
        !sameId(request.quoteId, value.quoteId) || !validItem(value.oil) || (value.oil.bound && !request.allowBound) || !inRange(value.beforeLuck, 0, 8) ||
        typeof value.success !== 'boolean' || typeof value.guaranteed !== 'boolean' ||
        !inRange(value.pityAfter, value.beforeLuck < 3 ? 0 : 1, value.beforeLuck < 3 ? 0 : 0xffffffff) ||
        !inRange(value.pityUsed, value.beforeLuck < 3 ? 0 : 1, value.beforeLuck < 3 ? 0 : 0xffffffff) ||
        value.guaranteed !== (value.pityAfter > 0 && value.pityUsed > value.pityAfter) || (value.guaranteed && !value.success) ||
        weaponOilReceiptLuck(value) !== (value.success ? value.beforeLuck + 1 : failureLuck(value.beforeLuck))) throw new Error(error);

    if (request.wing) {
        const wing = value.wing, before = expected?.wing;
        if (!wing || value.equipped !== false || !sameId(request.wing.wingId, wing.wingId) || !positiveLong(wing.version) ||
            BigInt(wing.version.toString()) !== BigInt(request.wing.version.toString()) + 1n || wingPity(wing, value.beforeLuck) !== value.pityUsed) {
            throw new Error(error);
        }
        observedWingState(wing, before ?? undefined);
        if (expected && (!before || !before.version.equals(request.wing.version) || wing.rank !== before.rank ||
            wing.appearanceTier !== before.appearanceTier || wing.hidden !== before.hidden ||
            wing.oilToSix !== before.oilToSix + Number(value.beforeLuck >= 3 && value.beforeLuck < 6) ||
            wing.oilToNine !== before.oilToNine + Number(value.beforeLuck >= 6))) throw new Error(error);
    } else {
        const weapon = value.weapon, before = expected?.weapon;
        if (!validItem(weapon) || !sameId(request.weapon.instanceId, weapon.instanceId) || value.equipped !== request.weapon.equipped ||
            BigInt(weapon.version.toString()) !== BigInt(request.weapon.version.toString()) + 1n || weapon.quantity.toString() !== '1' ||
            (value.oil.bound && !weapon.bound)) throw new Error(error);
        if (expected && (!before || before.itemId !== weapon.itemId || before.initialLuck !== weapon.initialLuck || expected.outputBound !== weapon.bound)) {
            throw new Error('祝福油回执与原报价不符，保留原请求等待核对');
        }
    }
    if (expected && (!sameObservedItem(expected.oil, value.oil) || expected.currentLuck !== value.beforeLuck ||
        expected.guaranteed !== value.guaranteed || expected.pityAfter !== value.pityAfter ||
        value.pityUsed !== expected.pityUsed + (expected.pityAfter > 0 ? 1 : 0))) {
        throw new Error('祝福油回执与原报价不符，保留原请求等待核对');
    }
    return value;
}

export function weaponOilReceiptLuck(receipt: WeaponOilReceipt): number {
    return receipt.wing ? receipt.wing.luck : equipmentCurrentLuck(receipt.weapon);
}

const shortId = (item: EquipmentItem): string => assetId(item.instanceId).slice(-8);
const binding = (bound: boolean): string => bound ? '绑定' : '非绑定';

function pityLines(luck: number, used: number, after: number, guaranteed: boolean): string[] {
    if (!after) return ['当前阶段无额外保底'];
    const stage = luck < 6 ? '3→6' : '6→9';
    return [`本次所属阶段 ${stage}`, `累计耗油 ${used}/${after}瓶`,
        guaranteed ? '本瓶保底，逐瓶升一级' : used === after ? '下一瓶起本段逐步必成' : `再累计${after - used}瓶后逐步必成`];
}

export function wingOilLines(wing: WingState, contributions?: readonly number[]): string[] {
    const ppm = contributions?.[wing.luck];
    return [`翅膀 ${wing.rank}阶`, '角色专属（不变）', `原翅膀尾号 ${assetId(wing.wingId).slice(-8)}`, `当前幸运+${wing.luck}`,
        ...(ppm === undefined ? [] : [`幸运一击贡献 ${ppm / 10_000}%`]),
        `3→6段累计用油 ${wing.oilToSix}瓶`, `6→9段累计用油 ${wing.oilToNine}瓶`];
}

export function weaponOilQuoteLines(quote: WeaponOilQuote, name: (itemId: number) => string, contributions?: readonly number[]): string[] {
    const subject = quote.wing ? wingOilLines(quote.wing, contributions) :
        [`武器 ${name(quote.weapon.itemId)}`, `${quote.equipped ? '穿戴中' : '背包'} · 末尾${shortId(quote.weapon)}`];
    return [...subject,
        `幸运+${quote.currentLuck} → +${quote.successLuck}`, `成功率 ${quote.successPpm / 10_000}%`,
        `失败${quote.failureLuck === quote.currentLuck ? '保留' : '回退至'}幸运+${quote.failureLuck}`, ...pityLines(quote.currentLuck, quote.pityUsed, quote.pityAfter, quote.guaranteed),
        `消耗1瓶${binding(quote.oil.bound)}${name(quote.oil.itemId)}`, `原油堆 末尾${shortId(quote.oil)}`,
        `来源 ${sourceName(quote.oil.sourceKind)}`, ...(quote.wing ? ['失败也扣油，翅膀归属不变'] :
            [`装备 ${binding(quote.weapon.bound)} → ${binding(quote.outputBound)}`, '失败也扣油，按同样规则绑定'])];
}

export function weaponOilReceiptLines(receipt: WeaponOilReceipt, name: (itemId: number) => string, contributions?: readonly number[]): string[] {
    const after = weaponOilReceiptLuck(receipt);
    const subject = receipt.wing ? wingOilLines(receipt.wing, contributions) :
        [`武器 ${name(receipt.weapon.itemId)}`, `原武器 末尾${shortId(receipt.weapon)}`];
    return [...subject,
        `${receipt.success ? '成功' : after < receipt.beforeLuck ? '失败回退' : '失败保留'} · 幸运+${receipt.beforeLuck} → +${after}`,
        `已扣1瓶${binding(receipt.oil.bound)}${name(receipt.oil.itemId)}`, `原油堆 末尾${shortId(receipt.oil)}`,
        ...pityLines(receipt.beforeLuck, receipt.pityUsed, receipt.pityAfter, receipt.guaranteed),
        ...(receipt.success && (after === 3 || after === 6) ? ['进入下一阶段，单独累计'] : []),
        ...(receipt.wing ? [] : [`结算后装备${binding(receipt.weapon.bound)}`])];
}
