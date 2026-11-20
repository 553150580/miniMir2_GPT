export const originalActorAudioClips = [
    '51', '54', '57', '64', '70', '73', '138', '139', '144', '145',
    '470-2', '470-4', '470-5', '570-5',
] as const;

export type OriginalActorClip = typeof originalActorAudioClips[number];
export type OriginalActorAction = 'attack' | 'swing' | 'struck' | 'death';
export type OriginalActorIdentity = { entityKind: number; definitionId: number };

type ActionSounds = Partial<Record<OriginalActorAction, OriginalActorClip>>;

// MYF 怪物表第13行 Appr27，Actor.pas 2971–2976 按外观计算，原 sound.lst 第176–181行核对。
const scarecrowSounds: ActionSounds = { attack: '470-2', struck: '470-4', death: '470-5' };
// 本项目骷髅来自 MYF 第381行变异骷髅 Appr37。原 sound.lst 第230–235行的574为空。
const skeletonSounds: ActionSounds = { attack: '54', swing: '64', death: '570-5' };

function monsterSounds(actor: OriginalActorIdentity): ActionSounds | undefined {
    if (actor.entityKind === 3 && actor.definitionId === 1) return scarecrowSounds;
    if (actor.entityKind === 2 && actor.definitionId === 2) return skeletonSounds;
    return undefined;
}

function isPlayer(actor: OriginalActorIdentity): boolean {
    return actor.entityKind === 1 && (actor.definitionId === 0 || actor.definitionId === 1 || actor.definitionId === 2);
}

export function originalActorSound(actor: OriginalActorIdentity, action: OriginalActorAction, job?: number, weaponItemId?: number | null): OriginalActorClip | undefined {
    if (!isPlayer(actor)) return monsterSounds(actor)?.[action];
    const actorJob = job ?? actor.definitionId;
    if (actorJob !== actor.definitionId) return undefined;
    // Actor.pas 2841–2846 按性别选人声，本项目战士与道士为男、法师为女。
    if (action === 'struck') return actorJob === 1 ? '139' : '138';
    if (action === 'death') return actorJob === 1 ? '145' : '144';
    // 玩家单一挥击声由攻击事件触发，独立 swing 槽只供怪物的第二层原声。
    if (action === 'attack') {
        if (weaponItemId === null) return '57';
        if (weaponItemId === 218) return '51';
    }
    return undefined;
}

export function originalWeaponImpactSound(attacker: OriginalActorIdentity, target: OriginalActorIdentity, weaponItemId?: number | null): OriginalActorClip | undefined {
    if (!isPlayer(attacker) || (!isPlayer(target) && !monsterSounds(target))) return undefined;
    // Actor.pas 2903–2908、2931–2937 的着体声与攻击动作挥击声独立。
    if (weaponItemId === null) return '73';
    if (weaponItemId === 218) return '70';
    return undefined;
}
