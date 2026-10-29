import Long from 'long';
import { equipmentLevelLines, equipmentLevelReason, GrowthEquipmentDefinition } from './LevelGrowthModel';
import type { PotionItem } from './PotionModel';
import { sourceName } from './SkillAssetsModel';

export interface EquipmentItem extends PotionItem { quality?: number; sourceKind?: string; locked?: boolean; recycleBlockFlags?: number; }
export interface EquipmentDefinition extends GrowthEquipmentDefinition { item_id: number; equip_slot?: number; equipment_stage?: number; base_attributes?: unknown; }
export interface EquippedSelection { readonly slot: number; readonly item: EquipmentItem; }
export interface EquipmentSnapshot {
    ready: boolean; scope: string; roleIdentity: string; ruleVersion: { toString(): string }; level: number; job: number;
    items: readonly EquipmentItem[]; equipped: readonly EquippedSelection[]; definitions: readonly EquipmentDefinition[];
}
export interface EquipmentPreview {
    readonly scope: string; readonly roleIdentity: string; readonly ruleVersion: string;
    readonly itemInstanceId: Uint8Array; readonly itemId: number; readonly itemVersion: Long; readonly slot: number; readonly unequip: boolean;
}

function sameId(left: Uint8Array, right: Uint8Array): boolean {
    return left instanceof Uint8Array && right instanceof Uint8Array && left.length === 16 && right.length === 16
        && left.every((value, index) => value === right[index]);
}
function version(value: { toString(): string }): Long {
    const text = value?.toString();
    // 永久存储使用有符号版本且提交要递增；协议uint64不代表能消耗最大版本。
    if (typeof value === 'number' || !text || !/^[1-9]\d*$/.test(text) || BigInt(text) >= 0x7fffffffffffffffn) throw new Error('原装备版本无效，请重新选择');
    return Long.fromString(text, true);
}
function original(snapshot: EquipmentSnapshot, id: Uint8Array, unequip: boolean, slot: number): EquipmentItem | undefined {
    return unequip ? snapshot.equipped.find(entry => entry.slot === slot && sameId(entry.item.instanceId, id))?.item
        : snapshot.items.find(item => sameId(item.instanceId, id));
}
export function equipmentSlotName(slot: number): string {
    return ['武器', '衣服', '头盔', '项链', '左手镯', '右手镯', '左戒指', '右戒指', '腰带', '鞋子'][slot] ?? '装备部位';
}
export function equipmentActionReason(snapshot: EquipmentSnapshot, item: EquipmentItem | null | undefined, unequip: boolean, slot?: number): string {
    if (!snapshot.ready || !snapshot.scope || !snapshot.roleIdentity) return '角色场景尚未就绪';
    if (!item) return unequip ? '先选择已穿戴装备' : '先选择背包装备';
    const definition = snapshot.definitions.find(row => row.item_id === item.itemId);
    if (!definition || definition.kind !== 'equipment' || !Number.isInteger(definition.equip_slot) || definition.equip_slot! < 0 || definition.equip_slot! > 9) return '该物品不能作为普通装备穿脱';
    const targetSlot = slot ?? definition.equip_slot!;
    if (targetSlot !== definition.equip_slot || !original(snapshot, item.instanceId, unequip, targetSlot)) return '原装备所在位置已变化，请重新选择';
    const flags = item.recycleBlockFlags ?? 0;
    if (!Number.isInteger(flags) || flags < 0 || flags > 15 || (flags & 6) || (!unequip && (flags & 8))) return '装备正在处理或有未确认结果，请稍后再试';
    if (item.quantity.toString() !== '1') return '原装备数量无效，请重新同步';
    try { version(item.version); } catch (error) { return (error as Error).message; }
    return unequip ? '' : equipmentLevelReason(definition, snapshot.level, snapshot.job);
}
export function createEquipmentPreview(snapshot: EquipmentSnapshot, item: EquipmentItem, unequip: boolean, slot?: number): EquipmentPreview {
    const reason = equipmentActionReason(snapshot, item, unequip, slot); if (reason) throw new Error(reason);
    const definition = snapshot.definitions.find(row => row.item_id === item.itemId)!;
    const targetSlot = slot ?? definition.equip_slot!, current = original(snapshot, item.instanceId, unequip, targetSlot)!;
    if (current.itemId !== item.itemId || version(current.version).toString() !== version(item.version).toString()) throw new Error('原装备已变化，请重新选择');
    const currentReason = equipmentActionReason(snapshot, current, unequip, targetSlot); if (currentReason) throw new Error(currentReason);
    return { scope: snapshot.scope, roleIdentity: snapshot.roleIdentity, ruleVersion: snapshot.ruleVersion.toString(),
        itemInstanceId: new Uint8Array(item.instanceId), itemId: item.itemId, itemVersion: version(item.version), slot: targetSlot, unequip };
}
// 确认不冻结整个背包修订：无关的已提交经验可推进；原实例、版本、容器和部位必须仍一致。
export function validateEquipmentPreview(preview: EquipmentPreview, snapshot: EquipmentSnapshot): void {
    if (!snapshot.ready || snapshot.scope !== preview.scope || snapshot.roleIdentity !== preview.roleIdentity || snapshot.ruleVersion.toString() !== preview.ruleVersion) throw new Error('原角色场景已变化，请重新确认');
    const item = original(snapshot, preview.itemInstanceId, preview.unequip, preview.slot);
    if (!item || item.itemId !== preview.itemId || version(item.version).toString() !== preview.itemVersion.toString()) throw new Error('原装备已变化，请重新选择');
    const reason = equipmentActionReason(snapshot, item, preview.unequip, preview.slot); if (reason) throw new Error(reason);
}
export function equipmentConfirmation(preview: EquipmentPreview, name: string): string {
    return [`${preview.unequip ? '卸下' : '穿戴'}${name}？`, `部位：${equipmentSlotName(preview.slot)}`,
        preview.unequip ? '装备将返回背包' : '同部位原装备将返回背包', '背包空间不足时不会丢弃装备', '确认时仍会核对原装备'].join('\n');
}

const BASE_RANGES = [
    ['physical_attack', '物理攻击'], ['magic_attack', '魔法攻击'], ['spirit_attack', '道术攻击'],
    ['physical_defence', '物理防御'], ['magic_defence', '魔法防御'],
] as const;
export function equipmentBaseAttributeLines(definition: EquipmentDefinition | undefined): string[] {
    if (definition?.kind !== 'equipment') return [];
    const value = definition.base_attributes;
    const keys = BASE_RANGES.flatMap(([key]) => [key + '_min', key + '_max']).concat('attack_speed');
    if (!value || typeof value !== 'object' || Array.isArray(value)) return ['基础属性尚未同步'];
    const data = value as Record<string, unknown>;
    if (Object.keys(data).length !== keys.length || keys.some(key => !Object.prototype.hasOwnProperty.call(data, key) || !Number.isSafeInteger(data[key]))) return ['基础属性尚未同步'];
    if ((data.attack_speed as number) < 0) return ['基础属性尚未同步'];
    for (const [key] of BASE_RANGES) if ((data[key + '_min'] as number) < 0 || (data[key + '_min'] as number) > (data[key + '_max'] as number)) return ['基础属性尚未同步'];
    return ['装备基础属性', ...BASE_RANGES.map(([key, name]) => `${name} ${data[key + '_min']}–${data[key + '_max']}`), `攻速加成 ${data.attack_speed}`, '角色当前总属性请查看战斗属性'];
}
export function equipmentDetailLines(definition: EquipmentDefinition | undefined, item: EquipmentItem, level: number | undefined, job: number | undefined, worn: boolean): string[] {
    if (!definition || definition.kind !== 'equipment') return ['装备定义尚未同步，请重新查看'];
    const stage = definition.equipment_stage;
    const stageKnown = Number.isSafeInteger(stage) && stage! >= 0 && Number.isSafeInteger(definition.required_level) && definition.required_level! >= 1
        && stage === Math.floor((definition.required_level! - 1) / 10);
    return [worn ? '已穿戴装备' : '背包装备', equipmentSlotName(definition.equip_slot ?? -1), `数量 ${item.quantity}`, item.bound ? '绑定物品' : '非绑定物品',
        `品质 ${item.quality ?? '未知'}`, `来源 ${sourceName(item.sourceKind)}`,
        stageKnown ? `装备阶段 T${stage! + 1}` : '装备阶段尚未同步', ...equipmentLevelLines(definition, level, job), ...equipmentBaseAttributeLines(definition),
        ...(item.locked ? ['装备已锁定，回收受保护'] : []), ...((item.recycleBlockFlags ?? 0) & 6 ? ['装备正在处理或有未确认结果'] : [])];
}
