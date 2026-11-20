export interface AutoSkillSettings {
    readonly enabled: boolean;
    readonly disabledSkillIds: readonly number[];
}

type SkillStorage = { getItem(key: string): string | null; setItem(key: string, value: string): void };
type Preference = 'quick' | 'auto';

function validId(id: unknown): id is number {
    return typeof id === 'number' && Number.isInteger(id) && id > 0 && id <= 0xffffffff;
}

export function observeAutoSkillSettings(value: unknown): AutoSkillSettings | undefined {
    const settings = value as Partial<AutoSkillSettings> | null | undefined;
    if (typeof settings?.enabled !== 'boolean' || !Array.isArray(settings.disabledSkillIds) || settings.disabledSkillIds.length > 64
        || !settings.disabledSkillIds.every(validId) || new Set(settings.disabledSkillIds).size !== settings.disabledSkillIds.length) return undefined;
    return { enabled: settings.enabled, disabledSkillIds: settings.disabledSkillIds.slice().sort((a, b) => a - b) };
}

function preferenceKey(role: string, preference: Preference): string {
    return `minimir.skills.v1.${role}.${preference}`;
}

function readPreference(storage: SkillStorage | undefined, role: string, preference: Preference): unknown {
    try {
        const saved = storage?.getItem(preferenceKey(role, preference));
        return saved ? JSON.parse(saved) : undefined;
    } catch { return undefined; }
}

export function savedQuickSkill(storage: SkillStorage | undefined, role: string): number {
    const saved = readPreference(storage, role, 'quick');
    return validId(saved) ? saved : 0;
}

export function savedAutoSkillSettings(storage: SkillStorage | undefined, role: string): AutoSkillSettings | undefined {
    return observeAutoSkillSettings(readPreference(storage, role, 'auto'));
}

export function saveSkillPreference(storage: SkillStorage | undefined, role: string, preference: Preference, value: number | AutoSkillSettings): void {
    storage?.setItem(preferenceKey(role, preference), JSON.stringify(value));
}
