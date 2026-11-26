import { equipmentBaseAttributeLines, equipmentDetailLines, equipmentDurabilityText, equipmentSlotName } from './EquipmentModel';
import type { EquipmentDefinition, EquipmentItem, EquippedSelection } from './EquipmentModel';
import { equipmentLevelLines } from './LevelGrowthModel';
import { orePurityText } from './MiningModel';
import type { PotionRules } from './PotionModel';
import { assetId, assetInteger, sourceName, yuanbaoText } from './SkillAssetsModel';
import type { VendorRow, VendorSnapshot } from './NpcShopModel';

type Currency = 'gold' | 'ingot' | 'bound_ingot';
interface NpcOffer {
    readonly id: number;
    readonly item_id: number;
    readonly quantity: number;
    readonly currency: Currency;
    readonly price_units: number;
    readonly required_role_level: number;
    readonly max_purchase_quantity: number;
}
interface NpcMineralBuyback {
    readonly item_id: number;
    readonly numerator: number;
    readonly denominator: number;
    readonly rounding: 'floor_per_ore' | 'exact';
}
export interface NpcShopDefinition {
    readonly key: string;
    readonly roles: readonly string[];
    readonly offers: readonly NpcOffer[];
    readonly sell_prices: readonly { item_id: number; price_gold: number }[];
    readonly mineral_buyback?: readonly NpcMineralBuyback[];
    readonly can_repair: boolean;
}
interface NpcInventoryItem extends EquipmentItem {
    readonly orePurity?: number;
    readonly sourceId?: string;
}
interface NpcInventory {
    readonly items: readonly NpcInventoryItem[];
    readonly equipped: readonly EquippedSelection[];
    readonly level: number;
}
interface NpcCatalogItem extends EquipmentDefinition { readonly potion?: PotionRules; }

export function shopItemApplies(definition: { readonly allowed_jobs?: readonly number[] } | undefined, job: number): boolean {
    return !!definition && (!definition.allowed_jobs?.length || definition.allowed_jobs.includes(job));
}

export function npcShopForRole(gameplay: { npc_shops?: readonly NpcShopDefinition[] }, role: string): NpcShopDefinition | undefined {
    return gameplay.npc_shops?.find(shop => shop.roles.includes(role));
}

function offerText(offer: NpcOffer): string {
    const amount = offer.currency === 'gold' ? `${offer.price_units}金币`
        : `${yuanbaoText(offer.price_units)}${offer.currency === 'bound_ingot' ? '绑定元宝' : '元宝'}`;
    return `每份 ${offer.quantity}件 · ${amount}`;
}

// 矿石按原实例逐块预估，确认金额和实际入账类型以服务端报价为准。
function mineralSale(item: NpcInventoryItem, name: string, offer: NpcMineralBuyback): VendorRow | undefined {
    const purity = item.orePurity ?? 0, purityText = orePurityText(purity);
    if (!purityText || item.sourceKind !== 'mining' || !/^[0-9a-f]{32}$/.test(item.sourceId ?? '') ||
        assetInteger(item.version) >= 0x7fffffffffffffffn || assetInteger(item.quantity) !== 1n ||
        [item.durabilityU, item.maxDurabilityU, item.originalDurabilityU].some(value => assetInteger(value ?? 0) !== 0n)) return undefined;
    const { numerator, denominator, rounding } = offer;
    if (![numerator, denominator].every(value => Number.isInteger(value) && value > 0 && value <= 0xffffffff) ||
        (rounding !== 'floor_per_ore' && (rounding !== 'exact' || denominator !== 1))) return undefined;
    const price = BigInt(purity) * BigInt(numerator) / BigInt(denominator);
    return {
        key: assetId(item.instanceId), itemId: item.itemId, name, instanceId: item.instanceId,
        itemVersion: item.version, maxQuantity: 1, mineral: { purity, bound: item.bound },
        detail: `${purityText} · ${price}${item.bound ? '绑定金币' : '金币'}/块`,
        itemDetails: [item.bound ? '绑定物品' : '非绑定物品', `来源 ${sourceName(item.sourceKind)}`],
        disabledReason: item.locked || (item.recycleBlockFlags ?? 0) & 14 ? '物品已锁定或正在处理'
            : price === 0n ? '该矿石当前无收购价' : undefined,
    };
}

export function npcShopSnapshot(shop: NpcShopDefinition, scope: string, inventory: NpcInventory | null, ready: boolean,
    definitions: readonly NpcCatalogItem[], allowSpecial: boolean, job: number): VendorSnapshot {
    const byId = new Map(definitions.map(item => [item.item_id, item]));
    const name = (id: number): string => byId.get(id)?.name ?? `物品 ${id}`;
    const owned = (item: EquipmentItem): boolean => item.instanceId?.length === 16 && assetInteger(item.version) > 0n;
    const protectedItem = (item: EquipmentItem): boolean => ((item.recycleBlockFlags ?? 0) & 6) !== 0;
    const ownedDetails = (item: EquipmentItem, worn = false): string[] => {
        const definition = byId.get(item.itemId);
        return definition?.kind === 'equipment' ? equipmentDetailLines(definition, item, inventory?.level, job, worn)
            : [item.bound ? '绑定物品' : '非绑定物品', `品质 ${item.quality ?? 0}`];
    };
    const buy = shop.offers.flatMap((offer): VendorRow[] => {
        const definition = byId.get(offer.item_id);
        if (!definition || !shopItemApplies(definition, job)) return [];
        const itemDetails = definition.kind === 'equipment' ? [equipmentSlotName(definition.equip_slot ?? -1),
            ...equipmentLevelLines(definition, inventory?.level, job), ...equipmentBaseAttributeLines(definition)]
            : definition.potion ? [`恢复总量：生命+${definition.potion.hp} / 魔法+${definition.potion.mp}`] : [];
        return [{
            key: `offer:${offer.id}`, offerId: offer.id, itemId: offer.item_id, name: name(offer.item_id), detail: offerText(offer), itemDetails,
            maxQuantity: BigInt(offer.max_purchase_quantity) * BigInt(offer.quantity), quantityStep: offer.quantity,
            disabledReason: inventory && inventory.level < offer.required_role_level ? `${offer.required_role_level}级可购买` : undefined,
        }];
    });
    const salePrice = new Map(shop.sell_prices.map(price => [price.item_id, BigInt(price.price_gold)]));
    const mineralBuyback = new Map((shop.mineral_buyback ?? []).map(offer => [offer.item_id, offer]));
    const sell = (inventory?.items ?? []).flatMap((item): VendorRow[] => {
        if (byId.get(item.itemId)?.kind === 'mining_ore') {
            const offer = mineralBuyback.get(item.itemId);
            if (!offer || !owned(item)) return [];
            const row = mineralSale(item, name(item.itemId), offer);
            return row ? [row] : [];
        }
        if (item.sourceKind === 'mining' || item.orePurity) return [];
        const price = salePrice.get(item.itemId);
        if (!price || !owned(item)) return [];
        const original = assetInteger(item.originalDurabilityU ?? 0);
        const current = assetInteger(item.durabilityU ?? 0);
        const per = original > 0n ? price * current / original : price;
        return [{
            key: assetId(item.instanceId), itemId: item.itemId, name: name(item.itemId), instanceId: item.instanceId,
            itemVersion: item.version, maxQuantity: item.quantity, detail: `每件 ${per}${item.bound ? '绑定金币' : '金币'} · 持有 ${item.quantity}`,
            itemDetails: ownedDetails(item),
            disabledReason: (item.locked || protectedItem(item) || (item.recycleBlockFlags ?? 0) & 8) ? '物品已锁定或正在处理'
                : per <= 0n ? '该物品当前无收购价' : undefined,
        }];
    });
    const repair: VendorRow[] = [];
    const addRepair = (item: EquipmentItem, slot?: number): void => {
        const durability = equipmentDurabilityText(item);
        if (!durability || !owned(item) || assetInteger(item.durabilityU ?? 0) >= assetInteger(item.maxDurabilityU ?? 0)) return;
        repair.push({
            key: assetId(item.instanceId), itemId: item.itemId, name: name(item.itemId),
            detail: `${slot === undefined ? '背包' : `穿戴 · ${equipmentSlotName(slot)}`} · ${durability}`, maxQuantity: 1,
            itemDetails: ownedDetails(item, slot !== undefined),
            instanceId: item.instanceId, itemVersion: item.version, repairActions: allowSpecial ? [3, 4] : [3],
            disabledReason: protectedItem(item) ? '装备正在处理' : undefined,
        });
    };
    if (shop.can_repair && inventory) {
        for (const wearing of inventory.equipped) addRepair(wearing.item, wearing.slot);
        for (const item of inventory.items) addRepair(item);
    }
    return { shopKey: shop.key, scope, ready, buy, sell, repair, allowsRepair: shop.can_repair };
}
