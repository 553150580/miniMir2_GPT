import { PotionInteger } from './PotionModel';
import type { PetTrainingState } from './PetTrainingModel';

export interface EntityStatusState {
    statusId: number; sourceSkillId: number; skillLevel: number; remainingMs: number;
    maxPhysicalDefenceBonus: PotionInteger; maxMagicDefenceBonus: PotionInteger;
    damageReductionPercent: number; teleportManaPenaltyPercent?: number; casterHandle: number; casterLife: PotionInteger;
}
export interface CombatAttributes {
    physicalAttackMin: PotionInteger; physicalAttackMax: PotionInteger; physicalDefenceMin: PotionInteger; physicalDefenceMax: PotionInteger;
    magicDefenceMin: PotionInteger; magicDefenceMax: PotionInteger; magicAttackMin: PotionInteger; magicAttackMax: PotionInteger;
    spiritAttackMin: PotionInteger; spiritAttackMax: PotionInteger; accuracy: PotionInteger; agility: PotionInteger; damageReductionPercent: number;
}
export interface CombatEntity {
    entityHandle: number; entityKind: number; definitionId?: number; life: PotionInteger;
    hp: PotionInteger; hpMax?: PotionInteger; position?: { x: number; y: number };
    moveIntervalMs?: number; attackIntervalMs?: number; isPetCorpse?: boolean; petTraining?: PetTrainingState | null;
    statuses?: readonly EntityStatusState[]; combatAttributes?: CombatAttributes | null;
}
export interface StatusDefinition { id: number; name: string; short_name: string; }
const NAMES: Record<number, string> = { 14: '幽灵盾', 15: '神圣战甲术', 31: '魔法盾' };
const SHORT: Record<number, string> = { 14: '幽盾', 15: '战甲', 31: '魔盾' };
export function statusName(id: number, definitions: readonly StatusDefinition[] = []): string { return definitions.find(row => row.id === id)?.name || NAMES[id] || `状态${id}`; }
export function statusShortName(id: number, definitions: readonly StatusDefinition[] = []): string { return definitions.find(row => row.id === id)?.short_name || SHORT[id] || '状态'; }
export function statusSeconds(status: EntityStatusState, receivedAt: number, now: number): number {
    if (!Number.isSafeInteger(status.remainingMs) || status.remainingMs < 0 || status.remainingMs > 0xffffffff
        || ![receivedAt, now].every(Number.isFinite)) throw new Error('状态剩余时间无效');
    return Math.max(0, Math.ceil((status.remainingMs - Math.max(0, now - receivedAt)) / 1000));
}
export function statusLines(status: EntityStatusState, receivedAt: number, now: number, definitions: readonly StatusDefinition[] = []): string[] {
    const seconds = statusSeconds(status, receivedAt, now);
    const lines = [statusName(status.statusId, definitions), `技能等级 ${status.skillLevel}`, seconds ? `剩余 ${seconds} 秒` : '剩余0秒 · 等待状态更新'];
    if (BigInt(status.maxPhysicalDefenceBonus.toString())) lines.push(`物理防御上限 +${status.maxPhysicalDefenceBonus}`);
    if (BigInt(status.maxMagicDefenceBonus.toString())) lines.push(`魔法防御上限 +${status.maxMagicDefenceBonus}`);
    if (status.teleportManaPenaltyPercent) lines.push(`传送技能魔法消耗 +${status.teleportManaPenaltyPercent}%`, '仅影响传送技能，成功传送刷新持续时间');
    if (status.damageReductionPercent) lines.push(`伤害减免 ${status.damageReductionPercent}%`);
    if (status.statusId === 31) lines.push('受伤会缩短持续时间', '已有魔法盾时重施不刷新');
    return lines;
}
export function combatAttributeLines(value?: CombatAttributes | null): string[] {
    if (!value) return ['正在同步战斗属性'];
    return [`物理攻击 ${value.physicalAttackMin}–${value.physicalAttackMax}`, `物理防御 ${value.physicalDefenceMin}–${value.physicalDefenceMax}`,
        `魔法防御 ${value.magicDefenceMin}–${value.magicDefenceMax}`, `魔法攻击 ${value.magicAttackMin}–${value.magicAttackMax}`,
        `道术攻击 ${value.spiritAttackMin}–${value.spiritAttackMax}`, `准确 ${value.accuracy} · 敏捷 ${value.agility}`, `伤害减免 ${value.damageReductionPercent}%`];
}
// 详情列表的每项来自已保存模板。按字符切成短行，不缩小字号或生成另一套布局。
export function detailLines(lines: readonly string[]): string[] {
    return lines.flatMap(line => line.split('\n').flatMap(part => {
        const points = Array.from(part); if (!points.length) return [''];
        return Array.from({ length: Math.ceil(points.length / 20) }, (_v, index) => points.slice(index * 20, (index + 1) * 20).join(''));
    }));
}
