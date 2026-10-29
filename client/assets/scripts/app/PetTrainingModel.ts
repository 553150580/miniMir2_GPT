import Long from 'long';

export interface PetTrainingState {
    trainingLevel: number;
    maxTrainingLevel: number;
    experience: Long;
    nextLevelExperience: Long;
}
export interface PetObservation {
    entityKind: number;
    hp: { toString(): string } | bigint | string | number;
    isPetCorpse?: boolean;
    petTraining?: PetTrainingState | null;
}
const uint32 = (value: number): boolean => Number.isInteger(value) && value >= 0 && value <= 0xffffffff;
function trainingInteger(value: Long): bigint {
    if (!Long.isLong(value) || !value.unsigned && value.isNegative()) throw new Error('宠物训练经验必须保留非负Long');
    return BigInt(value.toString());
}
// 训练只观察完整实体基线；允许单次升级后仍保留超过下一级门槛的来源经验。
export function validatePetObservation(entity: PetObservation): void {
    if (entity.isPetCorpse != null && typeof entity.isPetCorpse !== 'boolean') throw new Error('宠物尸体标识无效');
    if (entity.isPetCorpse && (entity.entityKind !== 2 || entity.hp == null || BigInt(entity.hp.toString()) !== 0n)) throw new Error('宠物尸体必须是已倒下的宠物实体');
    const value = entity.petTraining;
    if (!value) return;
    if (entity.entityKind !== 2 || !uint32(value.trainingLevel) || !uint32(value.maxTrainingLevel) || value.trainingLevel > value.maxTrainingLevel) throw new Error('宠物训练等级无效');
    trainingInteger(value.experience);
    const next = trainingInteger(value.nextLevelExperience);
    if (value.trainingLevel === value.maxTrainingLevel ? next !== 0n : next === 0n) throw new Error('宠物训练门槛与满级状态不一致');
}
export function isLivingPet(entity: PetObservation): boolean {
    return entity.entityKind === 2 && !entity.isPetCorpse && BigInt(entity.hp.toString()) > 0n;
}
export function petTrainingLines(entity: PetObservation): string[] {
    validatePetObservation(entity);
    const value = entity.petTraining;
    if (!value) return [];
    const lines = [`训练等级 ${value.trainingLevel} / ${value.maxTrainingLevel}`];
    if (value.trainingLevel === value.maxTrainingLevel) lines.push(`训练经验 ${value.experience}`, '已达训练等级上限');
    else lines.push(`训练经验 ${value.experience} / ${value.nextLevelExperience}`);
    return lines;
}
