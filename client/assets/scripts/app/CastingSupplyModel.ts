import { PotionInteger, PotionItem } from './PotionModel';
import { SkillDefinition } from './SkillModel';
export interface CastingSupplyConfig { enabled: boolean; slot: number; item_ids: readonly number[]; }
export interface CastingSupplySnapshot {
    ready: boolean; roleIdentity: string; sceneEpoch: PotionInteger; ruleVersion: PotionInteger; playerLife: PotionInteger;
    inventory: { revision: PotionInteger; items: readonly PotionItem[]; equipped: readonly { slot: number; item: PotionItem }[] } | null;
    config?: CastingSupplyConfig;
}
export interface SupplySelection {
    role: string; scene: string; rule: string; life: string; revision: string;
    instanceId: Uint8Array; version: string; quantity: string; itemId: number; bound: boolean;
}
function integer(value: PotionInteger): bigint { const text = value.toString(); if (!/^(?:0|[1-9]\d*)$/.test(text)) throw new Error('护符数量或版本无效'); return BigInt(text); }
function sameId(a: Uint8Array, b: Uint8Array): boolean { return a.length === 16 && b.length === 16 && a.every((value, index) => value === b[index]); }
function allowed(snapshot: CastingSupplySnapshot, item: PotionItem): boolean {
    return !!snapshot.config?.enabled && snapshot.config.slot === 100 && snapshot.config.item_ids.includes(item.itemId);
}
export function captureSupply(snapshot: CastingSupplySnapshot, item: PotionItem): SupplySelection {
    if (!snapshot.config?.enabled) throw new Error('施法护符暂未开放');
    if (!snapshot.ready || !snapshot.inventory || !snapshot.roleIdentity) throw new Error('角色和背包尚未同步');
    if (!allowed(snapshot, item) || integer(item.quantity) < 1n || item.instanceId.length !== 16) throw new Error('当前物品不是已开放的施法护符');
    return { role: snapshot.roleIdentity, scene: integer(snapshot.sceneEpoch).toString(), rule: integer(snapshot.ruleVersion).toString(),
        life: integer(snapshot.playerLife).toString(), revision: integer(snapshot.inventory.revision).toString(), instanceId: new Uint8Array(item.instanceId),
        version: integer(item.version).toString(), quantity: integer(item.quantity).toString(), itemId: item.itemId, bound: item.bound };
}
export function selectedCastingSupply(snapshot: CastingSupplySnapshot, definition: SkillDefinition): SupplySelection {
    if (!snapshot.config?.enabled) throw new Error('施法护符暂未开放');
    const equipped = snapshot.inventory?.equipped.find(entry => entry.slot === snapshot.config?.slot)?.item;
    if (!equipped || equipped.itemId !== definition.material_item_id) throw new Error('请先在背包中装入对应施法护符');
    const required = definition.material_quantity;
    if (!Number.isSafeInteger(required) || required! < 1 || integer(equipped.quantity) < BigInt(required!)) throw new Error('已装护符数量不足，施法前请更换');
    return captureSupply(snapshot, equipped);
}
export function validateSupply(selection: SupplySelection, snapshot: CastingSupplySnapshot, equipped: boolean, lockRevision = true): PotionItem {
    const changed = '原护符、角色或背包状态已变化，请重新选择';
    if (!snapshot.ready || !snapshot.inventory || selection.role !== snapshot.roleIdentity || selection.scene !== snapshot.sceneEpoch.toString()
        || selection.rule !== snapshot.ruleVersion.toString() || selection.life !== snapshot.playerLife.toString()
        || (lockRevision && selection.revision !== snapshot.inventory.revision.toString())) throw new Error(changed);
    const item = equipped ? snapshot.inventory.equipped.find(entry => entry.slot === snapshot.config?.slot)?.item
        : snapshot.inventory.items.find(value => sameId(value.instanceId, selection.instanceId));
    if (!item || !allowed(snapshot, item) || !sameId(item.instanceId, selection.instanceId) || item.itemId !== selection.itemId
        || item.version.toString() !== selection.version || item.quantity.toString() !== selection.quantity || item.bound !== selection.bound) throw new Error(changed);
    return item;
}
