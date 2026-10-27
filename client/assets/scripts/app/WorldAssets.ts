import { Asset, JsonAsset, Prefab, resources, SpriteFrame } from 'cc';

export function effectFrameKeys(value: any): string[] {
    if (!value || typeof value !== 'object') return [];
    if (Array.isArray(value)) return value.flatMap(part => typeof part === 'string' && /^[A-Za-z0-9]+:\d+$/.test(part) ? [part] : effectFrameKeys(part));
    return Object.values(value).flatMap(effectFrameKeys);
}

function load<T extends Asset>(path: string, type: any): Promise<T> {
    return new Promise((resolve, reject) => resources.load(path, type, (error, value) => error ? reject(error) : resolve(value as T)));
}

// 入场取票前准备本职业实际需要的整套动作，播放过程中不临时请求帧。
export class WorldAssets {
    manifest: any;
    map: any;
    gameplay: any;
    prefab!: Prefab;
    effectPrefab!: Prefab;
    background!: SpriteFrame;
    readonly frames = new Map<string, SpriteFrame>();
    private owned = new Set<Asset>();
    private disposed = false;

    private own<T extends Asset>(asset: T): T {
        if (!this.owned.has(asset)) { asset.addRef(); this.owned.add(asset); }
        return asset;
    }

    static async prepare(job: number, progress: (completed: number, total: number) => void): Promise<WorldAssets> {
        const result = new WorldAssets();
        try {
            result.manifest = result.own(await load<JsonAsset>('development/manifest', JsonAsset)).json;
            result.map = result.own(await load<JsonAsset>('development/maps/first-map', JsonAsset)).json;
            result.gameplay = result.own(await load<JsonAsset>('development/gameplay', JsonAsset)).json;
            result.prefab = result.own(await load<Prefab>('prefabs/Entity', Prefab));
            result.effectPrefab = result.own(await load<Prefab>('prefabs/Effect', Prefab));
            result.background = result.own(await load<SpriteFrame>('development/maps/first-map/spriteFrame', SpriteFrame));
            const profileIds = new Set<string>();
            const player = result.gameplay.jobs.find((row: any) => row.job_id === job);
            if (!player) throw new Error('当前职业缺少外观资源');
            profileIds.add(player.profile);
            for (const row of result.gameplay.monster_profiles) profileIds.add(row.profile);
            for (const row of result.gameplay.pet_profiles) if (row.job_id === job) profileIds.add(row.profile);
            const keys = new Set<string>();
            for (const id of profileIds) {
                const profile = result.manifest.profiles[id];
                if (!profile) throw new Error(`缺少外观资源：${id}`);
                for (const layer of profile.layers) for (const action of Object.values(layer.actions) as any[]) {
                    for (const direction of action.directions) for (const key of direction.frames) {
                        const definition = result.manifest.frames[key];
                        if (!definition) throw new Error(`缺少动作帧定义：${key}`);
                        if (!definition.declared_empty) keys.add(key);
                    }
                }
            }
            for (const skill of result.gameplay.skills ?? []) {
                if (skill.job_id !== job || (!skill.active && skill.activation_kind !== 'passive_proc')) continue;
                const effect = result.manifest.effects?.[String(skill.id)];
                if (!effect) throw new Error(`缺少技能表现资源：${skill.id}`);
                for (const key of effectFrameKeys(effect)) {
                    if (!result.manifest.frames[key]) throw new Error(`缺少技能帧定义：${key}`);
                    if (!result.manifest.frames[key].declared_empty) keys.add(key);
                }
            }
            const pending = Array.from(keys);
            let next = 0, completed = 0;
            let failure: unknown = null;
            progress(0, pending.length);
            // 有界并发，出错后等待已开始的请求结束，再统一释放持有引用。
            await Promise.all(Array.from({ length: Math.min(12, pending.length) }, async () => {
                while (!failure && next < pending.length) {
                    const key = pending[next++];
                    try {
                        const path = `development/${result.manifest.frames[key].png.replace(/\.png$/i, '')}/spriteFrame`;
                        result.frames.set(key, result.own(await load<SpriteFrame>(path, SpriteFrame)));
                        progress(++completed, pending.length);
                    } catch (error) { failure = error; }
                }
            }));
            if (failure) throw failure;
            return result;
        } catch (error) { result.dispose(); throw error; }
    }

    profile(state: any, job: number): any {
        const row = state.entityKind === 1 ? this.gameplay.jobs.find((value: any) => value.job_id === job)
            : state.entityKind === 2 ? this.gameplay.pet_profiles.find((value: any) => value.definition_id === state.definitionId && value.job_id === job)
            : state.entityKind === 3 ? this.gameplay.monster_profiles.find((value: any) => value.definition_id === state.definitionId) : null;
        return row ? this.manifest.profiles[row.profile] : null;
    }

    dispose(): void {
        if (this.disposed) return;
        this.disposed = true;
        for (const asset of this.owned) asset.decRef();
        this.owned.clear(); this.frames.clear();
    }
}
