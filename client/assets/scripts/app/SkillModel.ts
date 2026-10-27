import { PotionInteger, PotionItem } from './PotionModel';

export interface SkillDefinition {
    id: number; name: string; job_id: number; book_item_id: number; learn_level: number; initial_level: number;
    kind: 'fireball' | 'healing' | 'fencing'; active: boolean; range: number;
    base_cost: number; level_cost: number; cast_ms: number; impact_base_ms: number; projectile_ms_per_cell: number; accuracy_per_level: number;
}
export interface SkillInventory {
    revision: PotionInteger; level: number; items: readonly PotionItem[];
    learnedSkills?: readonly { skillId: number; level: number }[];
}
export interface SkillSnapshot {
    ready: boolean; roleIdentity: string; job: number; sceneEpoch: PotionInteger; ruleVersion: PotionInteger;
    inventory: SkillInventory | null; definitions: readonly SkillDefinition[];
}
export interface SkillRow {
    definition: SkillDefinition; learned: boolean; level: number; canLearn: boolean; cost: number; status: string;
}
export interface SkillEntity {
    entityHandle: number; entityKind: number; hp: PotionInteger; mp?: PotionInteger;
    position: { x: number; y: number }; direction: number;
}
export interface SkillLearnPreview {
    readonly skillId: number; readonly roleIdentity: string; readonly sceneEpoch: string; readonly ruleVersion: string;
    readonly revision: string; readonly instanceHex: string; readonly itemVersion: string; readonly quantity: string;
    readonly message: string;
}

function integer(value: PotionInteger): bigint {
    if (typeof value === 'number' && !Number.isSafeInteger(value)) throw new Error('技能状态数值必须保持整数精度');
    const text = value.toString(); if (!/^(?:0|[1-9]\d*)$/.test(text)) throw new Error('技能状态数值无效');
    return BigInt(text);
}
function hex(value: Uint8Array): string {
    if (!(value instanceof Uint8Array) || value.length !== 16) throw new Error('技能书实例标识无效');
    return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
}
export function skillCost(definition: SkillDefinition, level: number): number {
    const result = definition.base_cost + definition.level_cost * level;
    if (!Number.isSafeInteger(level) || level < 0 || !Number.isSafeInteger(result) || result < 0) throw new Error('技能消耗配置无效');
    return result;
}
export function skillAccuracy(row: SkillRow): number {
    const result = row.level * row.definition.accuracy_per_level;
    if (!Number.isSafeInteger(result) || result < 0) throw new Error('技能准确加成配置无效');
    return row.learned ? result : 0;
}
export function skillRows(snapshot: SkillSnapshot): SkillRow[] {
    return snapshot.definitions.filter(definition => definition.job_id === snapshot.job).map(definition => {
        const learned = snapshot.inventory?.learnedSkills?.find(skill => skill.skillId === definition.id);
        const hasBook = snapshot.inventory?.items.some(item => item.itemId === definition.book_item_id && integer(item.quantity) > 0n) ?? false;
        const eligible = (snapshot.inventory?.level ?? 0) >= definition.learn_level;
        return { definition, learned: learned !== undefined, level: learned?.level ?? definition.initial_level,
            cost: skillCost(definition, learned?.level ?? definition.initial_level),
            canLearn: snapshot.ready && !learned && eligible && hasBook,
            status: learned ? learned.level === 0 ? '已学 · 初学（等级0）' : `已学 · 等级${learned.level}`
                : !eligible ? `${definition.learn_level}级可学习` : hasBook ? '拥有技能书 · 可学习' : '缺少对应技能书' };
    }).sort((a, b) => a.definition.id - b.definition.id);
}
export function primarySkill(snapshot: SkillSnapshot): SkillRow | null {
    return skillRows(snapshot).find(row => row.definition.active) ?? null;
}
export function createSkillLearnPreview(snapshot: SkillSnapshot, skillId: number): SkillLearnPreview {
    if (!snapshot.ready || !snapshot.inventory || !snapshot.roleIdentity) throw new Error('角色与背包尚未同步');
    const row = skillRows(snapshot).find(value => value.definition.id === skillId);
    if (!row) throw new Error('当前职业不能学习此技能');
    if (row.learned) throw new Error('已经学会此技能');
    if (snapshot.inventory.level < row.definition.learn_level) throw new Error(`需要达到${row.definition.learn_level}级`);
    const book = snapshot.inventory.items.find(item => item.itemId === row.definition.book_item_id && integer(item.quantity) > 0n);
    if (!book) throw new Error('背包中没有对应技能书');
    return Object.freeze({ skillId, roleIdentity: snapshot.roleIdentity, sceneEpoch: integer(snapshot.sceneEpoch).toString(),
        ruleVersion: integer(snapshot.ruleVersion).toString(), revision: integer(snapshot.inventory.revision).toString(),
        instanceHex: hex(book.instanceId), itemVersion: integer(book.version).toString(), quantity: integer(book.quantity).toString(),
        message: `学习${row.definition.name}\n消耗1本对应技能书\n${book.bound ? '绑定' : '非绑定'}技能书，所选数量${integer(book.quantity)}\n学会后为初学（等级0）\n确认学习？` });
}
export function validateSkillLearnPreview(preview: SkillLearnPreview, snapshot: SkillSnapshot): void {
    const changed = '技能或背包状态已变化，请重新预览';
    if (!snapshot.ready || !snapshot.inventory || snapshot.roleIdentity !== preview.roleIdentity
        || integer(snapshot.sceneEpoch).toString() !== preview.sceneEpoch || integer(snapshot.ruleVersion).toString() !== preview.ruleVersion
        || integer(snapshot.inventory.revision).toString() !== preview.revision) throw new Error(changed);
    const row = skillRows(snapshot).find(value => value.definition.id === preview.skillId);
    const book = snapshot.inventory.items.find(item => hex(item.instanceId) === preview.instanceHex);
    if (!row || row.learned || snapshot.inventory.level < row.definition.learn_level || !book || book.itemId !== row.definition.book_item_id
        || integer(book.quantity).toString() !== preview.quantity || integer(book.version).toString() !== preview.itemVersion) throw new Error(changed);
}
export function skillBookInstance(preview: SkillLearnPreview): Uint8Array {
    return Uint8Array.from({ length: 16 }, (_value, index) => parseInt(preview.instanceHex.slice(index * 2, index * 2 + 2), 16));
}
export function gridDirection(origin: { x: number; y: number }, target: { x: number; y: number }, fallback = 4, count = 8): number {
    const dx = target.x - origin.x, dy = target.y - origin.y;
    if (!dx && !dy) return fallback;
    return (Math.round(Math.atan2(dx, -dy) * count / (2 * Math.PI)) + count) % count;
}
export function selectSkillTarget(row: SkillRow, entities: readonly SkillEntity[], selectedHandle: number | null): SkillEntity {
    if (!row.learned) throw new Error('尚未学会该技能');
    if (!row.definition.active) throw new Error('该技能为被动技能，无需主动施放');
    const player = entities.find(entity => entity.entityKind === 1);
    if (!player || integer(player.hp) === 0n) throw new Error('角色已倒下，不能施放技能');
    if (integer(player.mp ?? 0) < BigInt(row.cost)) throw new Error('魔法不足');
    const distance = (target: SkillEntity): number => Math.max(Math.abs(target.position.x - player.position.x), Math.abs(target.position.y - player.position.y));
    const legal = (target: SkillEntity): boolean => integer(target.hp) > 0n
        && (row.definition.kind === 'healing' ? target.entityKind === 1 || target.entityKind === 2 : target.entityKind === 3);
    const selected = entities.find(entity => entity.entityHandle === selectedHandle);
    if (selected && integer(selected.hp) === 0n && (row.definition.kind === 'healing'
        ? selected.entityKind === 1 || selected.entityKind === 2 : selected.entityKind === 3)) throw new Error('所选目标已倒下，请重新选择');
    let target = selected && legal(selected) ? selected : undefined;
    if (!target && row.definition.kind === 'healing') target = player;
    if (!target) target = entities.filter(legal).slice().sort((a, b) => distance(a) - distance(b) || a.entityHandle - b.entityHandle)[0];
    if (!target) throw new Error('没有可施放的目标');
    if (distance(target) > row.definition.range) throw new Error('目标超出技能射程，请靠近后施放');
    return target;
}
