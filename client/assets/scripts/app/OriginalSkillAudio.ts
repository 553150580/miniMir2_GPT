export interface OriginalSkillSounds {
    readonly cast?: string;
    readonly fly?: string;
    readonly impact?: string;
}

// 原1.80客户端Wav/sound.lst第871至947行按声音号给出文件。MYF ApexM2_Magic的MagID即此处的技能编号。
// Delphi Actor.pas第2187至2189行按10000+MagID×10的0/1/2段分别生成起手、飞行和爆发音。
// PlayScn.pas第1449至1470、1586至1618行区分是否飞行；爆发声可在非飞行的释放时直接播放。
const spellSounds: Readonly<Partial<Record<number, OriginalSkillSounds>>> = {
    1: { cast: 'M1-1', fly: 'M1-2', impact: 'M1-3' },
    2: { cast: 'M2-1', impact: 'M2-3' },
    5: { cast: 'M5-1', fly: 'M5-2', impact: 'M5-3' },
    8: { cast: 'M8-2' },
    9: { cast: 'M9-1', impact: 'M9-3' },
    10: { cast: 'M10-1' },
    // 原列表的M11-1没有随客户端提供，现存的M11-2用于爆发。
    11: { impact: 'M11-2' },
    14: { fly: 'M14-2', impact: 'M14-3' },
    15: { fly: 'M15-2', impact: 'M15-3' },
    // 仅收录骷髅施法音。宠物出生使用独立表现事件。
    17: { cast: 'M17-1' },
    21: { cast: 'M21-1' },
    23: { cast: 'M23-1', impact: 'M23-3' },
    // 地狱雷光所属mtFireWind并未进入飞行分支。
    24: { cast: 'M24-1' },
    29: { cast: 'M29-1', impact: 'M29-3' },
    31: { cast: 'M31-1' },
    33: { cast: 'M33-1', impact: 'M33-3' },
};

// 物理招式音独立于魔法序号。原Actor在挥砍第2帧使用SoundUtil.pas第115至122行及sound.lst第83至90行的代码。
const strikeSounds: Readonly<Partial<Record<number, readonly [male: string, female?: string]>>> = {
    7: ['M7-1', 'M7-2'],
    12: ['M12-1'],
    25: ['M25-1'],
    26: ['M26-3'],
};

// 原空间移动退场与到达分别触发109和110，见Actor.pas的SM_SPACEMOVE_HIDE与SM_SPACEMOVE_SHOW。
export const originalTeleportAudio = { departure: '109', arrival: '110' } as const;

export function originalSkillAudio(skillId: number): OriginalSkillSounds | undefined {
    return spellSounds[skillId];
}

export function originalSkillStrikeAudio(skillId: number, gender: number): string | undefined {
    const sounds = strikeSounds[skillId];
    return gender === 1 ? sounds?.[1] ?? sounds?.[0] : sounds?.[0];
}

export const originalSkillAudioClips: readonly string[] = Array.from(new Set([
    ...Object.values(spellSounds).flatMap(sounds => sounds ? Object.values(sounds) : []),
    ...Object.values(strikeSounds).flatMap(sounds => sounds?.filter((clip): clip is string => clip !== undefined) ?? []),
    originalTeleportAudio.departure,
    originalTeleportAudio.arrival,
]));
