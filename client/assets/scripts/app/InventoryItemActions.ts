import type { ItemAction, ItemActionView } from './GameItemActions';
import { manualRecycleBlockReason } from './AutoRecycleModel';
import type { RecycleItem } from './AutoRecycleModel';
import { equipmentActionReason, equipmentDetailLines } from './EquipmentModel';
import type { EquipmentDefinition, EquipmentSnapshot } from './EquipmentModel';
import { canRecycleBook, pageMergeGroup, sourceName } from './SkillAssetsModel';
import type { SkillAssetDefinition, SkillAssetItem, SkillAssetsSnapshot } from './SkillAssetsModel';
import type { SkillRow } from './SkillModel';
import type { CastingSupplyConfig } from './CastingSupplyModel';
import { miningDurability, orePurityText } from './MiningModel';
import type { MiningItem } from './MiningModel';

interface InventoryActionDefinition extends EquipmentDefinition, SkillAssetDefinition { readonly use_enabled?: boolean; }
interface InventoryActionInput {
    readonly item: SkillAssetItem & RecycleItem & Pick<MiningItem, 'orePurity' | 'durabilityU' | 'maxDurabilityU'>;
    readonly definition: InventoryActionDefinition | undefined;
    readonly ready: boolean;
    readonly blockedReason: string;
    readonly equipment: EquipmentSnapshot;
    readonly assets: SkillAssetsSnapshot;
    readonly supply?: CastingSupplyConfig;
    readonly skill?: SkillRow;
    readonly playerAlive: boolean;
    readonly miningReason?: string;
}
interface InventoryActions {
    equip(): Promise<void>;
    recycle(): Promise<void>;
    lock(): Promise<void>;
    usePotion(): Promise<void>;
    learn(): Promise<void>;
    recycleBook(): Promise<void>;
    mine?(): Promise<void>;
}

// 使用当前角色与背包数据生成操作；处理器由上层绑定同一个原实例。
export function inventoryItemActionView(input: InventoryActionInput, handlers: InventoryActions): ItemActionView {
    const { item, definition, equipment, assets, supply, skill } = input;
    const lines = definition?.kind === 'equipment' ? equipmentDetailLines(definition, item, equipment.level, equipment.job, false)
        : [`数量 ${item.quantity}`, item.bound ? '绑定物品' : '非绑定物品', `来源 ${sourceName(item.sourceKind)}`, `品质 ${item.quality ?? 0}`];
    const actions: ItemAction[] = [];
    const add = (label: string, run: () => Promise<void>, reason?: string | null): void => {
        if (reason && !lines.includes(reason)) lines.push(reason);
        actions.push({ label, run, enabled: input.ready && !reason });
    };
    switch (definition?.kind) {
        case 'equipment':
            add('穿戴装备', handlers.equip, equipmentActionReason(equipment, item, false));
            add('分解装备', handlers.recycle, manualRecycleBlockReason(item));
            add(item.locked ? '解锁装备' : '锁定装备', handlers.lock, (item.recycleBlockFlags ?? 0) & 14 ? '装备正在处理，不能更改锁定' : '');
            break;
        case 'casting_supply':
            add('装入耗材', handlers.equip, supply?.enabled && supply.item_ids.includes(item.itemId) ? '' : '当前耗材未开放装入');
            break;
        case 'potion':
            if (definition.use_enabled) add('使用药品', handlers.usePotion, input.playerAlive ? '' : '角色已倒下，不能使用药品');
            break;
        case 'book':
            if (skill && !skill.learned) add('学习技能', handlers.learn, skill.canLearn ? '' : skill.status);
            else lines.push(skill ? '已学习此技能' : '当前职业不能学习此技能');
            if (canRecycleBook(item, assets.recycle)) {
                add('拆成书页', handlers.recycleBook, item.locked || item.recycleBlockFlags ? '该技能书受保护' : '');
            } else lines.push('此来源或品质不能拆书为书页');
            break;
        case 'skill_page':
            add('合并书页', handlers.lock, pageMergeGroup(item, assets.inventory?.items ?? [], assets.definitions).reason);
            break;
        case 'mining_ore': {
            const purity = orePurityText(item.orePurity);
            if (purity) lines.push(purity);
            break;
        }
        case 'mining_tool': {
            const durability = miningDurability(item);
            if (durability) lines.push(durability);
            if (handlers.mine) add('使用此锄头', handlers.mine, input.miningReason);
            break;
        }
    }
    if (!input.ready && input.blockedReason) lines.push(input.blockedReason);
    return { title: definition?.name ?? '物品详情', lines, actions };
}
