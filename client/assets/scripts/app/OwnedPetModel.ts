import Long from 'long';
import { PetTrainingState, petTrainingLines, validatePetObservation } from './PetTrainingModel';

// 独立于当前地图entities的owner活宠观察；异图不产生可点击实体或占格。
export interface OwnedPetState {
    readonly entityHandle: number;
    readonly life: Long;
    readonly definitionId: number;
    readonly mapId: Long;
    readonly hp: Long;
    readonly hpMax: Long;
    readonly petTraining?: PetTrainingState | null;
    readonly recallSkillIds: readonly number[];
    readonly followSuppressed: boolean;
}
const uint32 = (value: number): boolean => Number.isInteger(value) && value > 0 && value <= 0xffffffff;
function observedLong(value: Long, positive: boolean, signed: boolean): Long {
    if (!Long.isLong(value)) throw new Error('活宠64位观察必须保留Long');
    const amount = BigInt(value.toString());
    if (amount < (positive ? 1n : 0n) || amount > (signed ? 0x7fffffffffffffffn : 0xffffffffffffffffn)) throw new Error('活宠64位观察越界');
    return Long.fromString(amount.toString(), !signed);
}
export function parseOwnedPet(value: OwnedPetState | null | undefined): OwnedPetState | null {
    if (value == null) return null;
    if (!uint32(value.entityHandle) || !uint32(value.definitionId) || typeof value.followSuppressed !== 'boolean'
        || !Array.isArray(value.recallSkillIds) || value.recallSkillIds.some(id => !uint32(id)) || new Set(value.recallSkillIds).size !== value.recallSkillIds.length) throw new Error('owner活宠观察结构无效');
    const life = observedLong(value.life, false, false), mapId = observedLong(value.mapId, true, false);
    const hp = observedLong(value.hp, true, true), hpMax = observedLong(value.hpMax, true, true);
    if (hp.greaterThan(hpMax)) throw new Error('owner活宠生命超过上限');
    validatePetObservation({ entityKind: 2, hp, petTraining: value.petTraining });
    const training = value.petTraining;
    const petTraining = training ? Object.freeze({ trainingLevel: training.trainingLevel, maxTrainingLevel: training.maxTrainingLevel,
        experience: observedLong(training.experience, false, false), nextLevelExperience: observedLong(training.nextLevelExperience, false, false) }) : null;
    return Object.freeze({ entityHandle: value.entityHandle, life, definitionId: value.definitionId, mapId, hp, hpMax, petTraining,
        recallSkillIds: Object.freeze(Array.from(value.recallSkillIds)), followSuppressed: value.followSuppressed });
}
export function ownedPetIdentity(value: OwnedPetState | null | undefined): string {
    const pet = parseOwnedPet(value);
    return pet ? JSON.stringify([pet.entityHandle, pet.life.toString(), pet.definitionId, pet.mapId.toString(), pet.hp.toString(), pet.hpMax.toString(),
        pet.petTraining ? [pet.petTraining.trainingLevel, pet.petTraining.maxTrainingLevel, pet.petTraining.experience.toString(), pet.petTraining.nextLevelExperience.toString()] : null,
        pet.recallSkillIds, pet.followSuppressed]) : 'null';
}
export function ownedPetCanRecall(value: OwnedPetState | null | undefined, skillId: number): boolean {
    const pet = parseOwnedPet(value);
    return !!pet && pet.recallSkillIds.includes(skillId);
}
export function ownedPetLines(value: OwnedPetState | null | undefined, currentMap?: { toString(): string }): string[] {
    const pet = parseOwnedPet(value);
    if (!pet) return ['当前没有存活宠物'];
    const here = currentMap?.toString() === pet.mapId.toString();
    const lines = ['宠物', `驻留地图 ${pet.mapId}${currentMap ? here ? ' · 当前地图' : ' · 异图驻留' : ''}`, `生命 ${pet.hp} / ${pet.hpMax}`];
    if (pet.followSuppressed) lines.push('跟随已暂停');
    const training = petTrainingLines({ entityKind: 2, hp: pet.hp, petTraining: pet.petTraining });
    lines.push(...(training.length ? training : ['未提供训练数据']));
    return lines;
}

export function validateOwnedPetEntities(value: OwnedPetState | null, entities: readonly { entityHandle: number; entityKind: number; definitionId?: number; life?: { toString(): string }; hp?: { toString(): string }; hpMax?: { toString(): string }; isPetCorpse?: boolean; petTraining?: PetTrainingState | null }[], currentMap?: Long): void {
    if (!value) return;
    const entity = entities.find(row => row.entityHandle === value.entityHandle);
    const sameMap = currentMap ? value.mapId.toString() === currentMap.toString() : undefined;
    if (sameMap === true && !entity || sameMap === false && entity) throw new Error('活宠驻留地图与当前实体不一致');
    const trainingKey = (training: PetTrainingState | null | undefined): string => training ? JSON.stringify([training.trainingLevel, training.maxTrainingLevel, training.experience.toString(), training.nextLevelExperience.toString()]) : 'null';
    if (entity && (entity.entityKind !== 2 || entity.isPetCorpse || entity.definitionId !== value.definitionId || entity.life?.toString() !== value.life.toString()
        || entity.hp?.toString() !== value.hp.toString() || entity.hpMax?.toString() !== value.hpMax.toString() || trainingKey(entity.petTraining) !== trainingKey(value.petTraining))) throw new Error('活宠与同句柄实体的生命或属性不一致');
}
