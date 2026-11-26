import { Button, Label, Node } from 'cc';
import { equipmentSlotName } from './EquipmentModel';
import type { EquippedSelection } from './EquipmentModel';

const recordedSlots = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 100] as const;
interface RoleAppearance { readonly job: number; readonly gender?: number; readonly name: string; }
interface EquipmentLabel { readonly item_id: number; readonly name: string; }

// 固定装备槽对应服务端部位，角色画像复用已保存的原版各职业男女外观。
export class GameRoleEquipment {
    private readonly board: Node;
    private readonly portraits: Node;
    private readonly characterName: Label;
    private readonly slots: ReadonlyArray<{ id: number; node: Node; button: Button; label: Label }>;

    constructor(panel: Node) {
        const board = panel.getChildByName('EquipmentBoard');
        const portraits = board?.getChildByName('Portrait'), characterName = board?.getChildByName('CharacterName')?.getComponent(Label);
        if (!board || !portraits || !characterName) throw new Error('角色预览尚未完整保存在场景');
        this.board = board;
        this.portraits = portraits;
        this.characterName = characterName;
        this.slots = recordedSlots.map(id => {
            const node = board.getChildByName(`Slot${id}`), button = node?.getComponent(Button), label = node?.getChildByName('Label')?.getComponent(Label);
            if (!node || !button || !label) throw new Error(`角色装备部位未保存 ${id}`);
            return { id, node, button, label };
        });
    }

    refresh(role: RoleAppearance | null, equipped: readonly EquippedSelection[], definitions: readonly EquipmentLabel[],
        supplySlot: number | undefined, selectedSlot: number | undefined, ready: boolean,
        enhancementLevels?: ReadonlyMap<number, number>): void {
        if (!this.board.activeInHierarchy) return;
        const gender = role?.gender ?? (role?.job === 1 ? 1 : 0), portrait = role && `Job${role.job}Gender${gender}`;
        for (const node of this.portraits.children) node.active = node.name === portrait;
        this.characterName.string = role?.name ?? '';
        const wearing = new Map(equipped.map(value => [value.slot, value.item]));
        for (const slot of this.slots) {
            slot.node.active = slot.id !== 100 || supplySlot === 100;
            const item = wearing.get(slot.id);
            const name = item && definitions.find(value => value.item_id === item.itemId)?.name;
            const enhancement = enhancementLevels?.get(slot.id);
            const prefix = selectedSlot === slot.id ? '● ' : '';
            const title = item ? name ?? '已装备' : slot.id === 100 ? '施法耗材' : equipmentSlotName(slot.id);
            slot.label.string = `${prefix}${title}${enhancement ? ` +${enhancement}` : ''}`;
            // 空部位也可强化，独立施法耗材位仍需装备后才可点击。
            slot.button.interactable = ready && (!!item || enhancement !== undefined);
        }
    }
}
