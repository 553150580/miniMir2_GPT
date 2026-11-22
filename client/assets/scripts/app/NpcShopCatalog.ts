import { equipmentDurabilityText, equipmentSlotName } from './EquipmentModel';
import type { EquipmentItem, EquippedSelection } from './EquipmentModel';
import { assetId, assetInteger, yuanbaoText } from './SkillAssetsModel';
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
export interface NpcShopDefinition {
    readonly key: string;
    readonly roles: readonly string[];
    readonly offers: readonly NpcOffer[];
    readonly sell_prices: readonly { item_id: number; price_gold: number }[];
    readonly can_repair: boolean;
}
interface NpcInventory {
    readonly items: readonly EquipmentItem[];
    readonly equipped: readonly EquippedSelection[];
    readonly level: number;
}

export function npcShopForRole(gameplay: { npc_shops?: readonly NpcShopDefinition[] }, role: string): NpcShopDefinition | undefined {
    return gameplay.npc_shops?.find(shop => shop.roles.includes(role));
}

function offerText(offer: NpcOffer): string {
    const amount = offer.currency === 'gold' ? `${offer.price_units}金币`
        : `${yuanbaoText(offer.price_units)}${offer.currency === 'bound_ingot' ? '绑定元宝' : '元宝'}`;
    return `每份 ${offer.quantity}件 · ${amount}`;
}

export function npcShopSnapshot(shop: NpcShopDefinition, scope: string, inventory: NpcInventory | null, ready: boolean,
    names: readonly { item_id: number; name: string }[], allowSpecial: boolean): VendorSnapshot {
    const name = (id: number): string => names.find(item => item.item_id === id)?.name ?? `物品 ${id}`;
    const owned = (item: EquipmentItem): boolean => item.instanceId?.length === 16 && assetInteger(item.version) > 0n;
    const protectedItem = (item: EquipmentItem): boolean => ((item.recycleBlockFlags ?? 0) & 6) !== 0;
    const buy = shop.offers.map((offer): VendorRow => ({
        key: `offer:${offer.id}`, offerId: offer.id, itemId: offer.item_id, name: name(offer.item_id), detail: offerText(offer),
        maxQuantity: BigInt(offer.max_purchase_quantity) * BigInt(offer.quantity), quantityStep: offer.quantity,
        disabledReason: inventory && inventory.level < offer.required_role_level ? `${offer.required_role_level}级可购买` : undefined,
    }));
    const salePrice = new Map(shop.sell_prices.map(price => [price.item_id, BigInt(price.price_gold)]));
    const sell = (inventory?.items ?? []).flatMap((item): VendorRow[] => {
        const price = salePrice.get(item.itemId);
        if (!price || !owned(item)) return [];
        const original = assetInteger(item.originalDurabilityU ?? 0);
        const current = assetInteger(item.durabilityU ?? 0);
        const per = original > 0n ? price * current / original : price;
        return [{
            key: assetId(item.instanceId), itemId: item.itemId, name: name(item.itemId), instanceId: item.instanceId,
            itemVersion: item.version, maxQuantity: item.quantity, detail: `每件 ${per}${item.bound ? '绑定金币' : '金币'} · 持有 ${item.quantity}`,
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
