import { Asset, AudioClip, JsonAsset, Prefab, resources, SpriteFrame } from 'cc';
import { GridProjection } from './GridProjection';
import { SourceMapAssets } from './SourceMapAssets';

interface WorldMapEntry {
    readonly id: string; readonly name?: string; readonly resource_key: string; readonly scene_kind: number;
    readonly width?: number; readonly height?: number; readonly safe_zones?: readonly unknown[];
}
export type WorldAssetsInvalidation = 'dispose' | 'source';
function mapIdString(value: unknown): string {
    if (typeof value !== 'string' && typeof value !== 'bigint' && (!value || typeof value !== 'object' || typeof (value as { toString?: unknown }).toString !== 'function')) throw new Error('地图ID必须使用十进制字符串或64位对象');
    const id = typeof value === 'string' ? value : value.toString();
    if (typeof id !== 'string' || !/^(0|[1-9]\d*)$/.test(id) || BigInt(id) > 18446744073709551615n) throw new Error('地图ID超出uint64十进制范围');
    return id;
}
// 旧profile仅兼容原first-map；目录存在时严格按其字符串ID选择，不回落成另一张图。
export function selectWorldMap(gameplay: any, mapId?: { toString(): string } | string): WorldMapEntry {
    const requested = mapId === undefined ? undefined : mapIdString(mapId);
    if (gameplay?.map_catalog === undefined) {
        if (requested !== undefined && requested !== '1') throw new Error('旧profile未配置请求的地图');
        return { id: '1', resource_key: 'development/maps/first-map', scene_kind: 1 };
    }
    const catalog = gameplay.map_catalog;
    if (!Array.isArray(catalog) || !catalog.length || catalog[0]?.scene_kind !== 1) throw new Error('地图目录缺少首项个人地图');
    const ids = new Set<string>();
    for (const entry of catalog) {
        if (!entry || typeof entry.id !== 'string') throw new Error('地图目录ID必须是十进制字符串');
        const id = mapIdString(entry.id);
        if (ids.has(id) || ![1, 2].includes(entry.scene_kind) || ![entry.width, entry.height].every(value => Number.isSafeInteger(value) && value > 0)
            || typeof entry.resource_key !== 'string' || !entry.resource_key || /[\\:]/.test(entry.resource_key)
            || entry.resource_key.split('/').some((part: string) => !part || part === '.' || part === '..')) throw new Error('地图目录条目无效');
        ids.add(id);
    }
    const selected = requested === undefined ? catalog[0] : catalog.find((entry: WorldMapEntry) => entry.id === requested);
    if (!selected) throw new Error('地图目录没有请求的地图'); return selected;
}

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
    propPrefab!: Prefab;
    beamPrefab!: Prefab;
    statusBadgePrefab!: Prefab;
    background: SpriteFrame | null = null;
    sourceMap?: SourceMapAssets;
    mapId = '1';
    get mapBackground(): SpriteFrame | null { return this.background; }
    readonly frames = new Map<string, SpriteFrame>();
    readonly audioClips = new Map<string, AudioClip>();
    private owned = new Set<Asset>();
    private disposed = false;
    private disposing = false;
    private notifying = false;
    private readonly beforeInvalidation = new Set<(reason: WorldAssetsInvalidation) => void>();
    private releaseSourceInvalidation?: () => void;

    // 保存Surface/HUD和动态Sprite都不因赋帧自动持有素材，必须先同步解绑再归还引用。
    onBeforeInvalidation(listener: (reason: WorldAssetsInvalidation) => void): () => void {
        if (this.disposed || this.disposing || this.notifying) throw new Error('世界资源已经释放或正在失效');
        this.beforeInvalidation.add(listener); return () => { this.beforeInvalidation.delete(listener); };
    }
    private notifyBeforeInvalidation(reason: WorldAssetsInvalidation): void {
        if (this.notifying) throw new Error('世界资源失效通知不能重入');
        this.notifying = true; let failed = false, failure: unknown;
        try {
            for (const listener of Array.from(this.beforeInvalidation)) {
                try { listener(reason); } catch (error) { if (!failed) { failed = true; failure = error; } }
            }
        } finally { this.notifying = false; }
        // 某个消费者解绑失败时仍通知其余消费者，并保留全部资源引用供原owner重试。
        if (failed) throw failure;
    }

    private own<T extends Asset>(asset: T): T {
        if (!this.owned.has(asset)) { asset.addRef(); this.owned.add(asset); }
        return asset;
    }

    static async prepare(job: number, progress: (completed: number, total: number) => void, mapId?: { toString(): string } | string): Promise<WorldAssets> {
        const result = new WorldAssets();
        try {
            result.manifest = result.own(await load<JsonAsset>('development/manifest', JsonAsset)).json;
            result.gameplay = result.own(await load<JsonAsset>('development/gameplay', JsonAsset)).json;
            const selected = selectWorldMap(result.gameplay, mapId); result.mapId = selected.id;
            if (selected.scene_kind === 2) {
                const streaming = result.gameplay.source_map_streaming;
                if (!streaming || ![streaming.load_concurrency, streaming.cache_capacity].every(value => Number.isSafeInteger(value) && value > 0)) throw new Error('来源地图缺少显式流式并发/缓存预算');
                result.sourceMap = await SourceMapAssets.prepare(selected.resource_key, { concurrency: streaming.load_concurrency, capacity: streaming.cache_capacity });
                result.releaseSourceInvalidation = result.sourceMap.onBeforeInvalidation(() => {
                    if (!result.disposing) result.notifyBeforeInvalidation('source');
                });
                const source = result.sourceMap.model.manifest;
                if (source.id !== selected.id || source.width !== selected.width || source.height !== selected.height) throw new Error('来源地图清单与目录身份/完整尺寸不一致');
                result.map = Object.freeze({ ...source, name: selected.name, scene_kind: selected.scene_kind, safe_zones: selected.safe_zones });
                result.propPrefab = result.sourceMap.propPrefab; result.background = result.sourceMap.minimap;
            } else {
                result.map = result.own(await load<JsonAsset>(selected.resource_key, JsonAsset)).json;
                if (selected.width !== undefined || result.map.id !== undefined) {
                    const id = typeof result.map.id === 'number' && Number.isSafeInteger(result.map.id) && result.map.id >= 0 ? String(result.map.id) : mapIdString(result.map.id);
                    if (id !== selected.id) throw new Error('个人地图资源与目录身份不一致');
                }
                const projection = new GridProjection(result.map), mapInfo = result.manifest.map;
                if (result.map.surface_width !== projection.surfaceWidth || result.map.surface_height !== projection.surfaceHeight
                    || result.map.world_height !== projection.worldHeight || result.map.background_offset_y !== projection.floorCenterY
                    || mapInfo?.width !== result.map.width || mapInfo?.height !== result.map.height
                    || mapInfo?.tile_width !== result.map.tile_width || mapInfo?.tile_height !== result.map.tile_height
                    || mapInfo?.projection !== result.map.projection
                    || ['soil_thickness_px', 'surface_width', 'surface_height', 'world_height', 'background_offset_y'].some(key => mapInfo?.[key] !== result.map[key])
                    || (selected.width !== undefined && selected.width !== result.map.width) || (selected.height !== undefined && selected.height !== result.map.height)) throw new Error('地图地板、投影与来源清单不一致，请更新同一份资源');
                result.propPrefab = result.own(await load<Prefab>('prefabs/MapProp', Prefab));
                result.background = result.own(await load<SpriteFrame>(`${selected.resource_key}/spriteFrame`, SpriteFrame));
            }
            result.prefab = result.own(await load<Prefab>('prefabs/Entity', Prefab));
            result.effectPrefab = result.own(await load<Prefab>('prefabs/Effect', Prefab));
            result.beamPrefab = result.own(await load<Prefab>('prefabs/Beam', Prefab));
            result.statusBadgePrefab = result.own(await load<Prefab>('prefabs/StatusBadge', Prefab));
            const profileIds = new Set<string>();
            const player = result.gameplay.jobs.find((row: any) => row.job_id === job);
            if (!player) throw new Error('当前职业缺少外观资源');
            profileIds.add(player.profile);
            for (const row of result.gameplay.monster_profiles) profileIds.add(row.profile);
            for (const row of result.gameplay.pet_profiles) if (row.job_id === job) profileIds.add(row.profile);
            const keys = new Set<string>();
            // 可接收的常驻状态与职业无关，入场前持有，不在受益时临时异步加载。
            for (const key of [...effectFrameKeys(result.manifest.statuses), ...effectFrameKeys(result.manifest.teleport)]) {
                if (!result.manifest.frames[key]) throw new Error(`缺少常驻状态帧定义：${key}`);
                if (!result.manifest.frames[key].declared_empty) keys.add(key);
            }
            for (const prop of result.map.props ?? []) {
                const frame = result.manifest.frames[prop.frame];
                if (!frame || frame.declared_empty) throw new Error(`地图物件缺少原始帧：${prop.frame}`);
                keys.add(prop.frame);
            }
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
            // 出场与施法声音只使用已核原音频；与身体帧一样在入场前持有，不能让加载延后服务器出生。
            if (job === 2) for (const [key, value] of Object.entries(result.manifest.audio ?? {}) as Array<[string, any]>) {
                if (!/^[a-z0-9_]+$/.test(key) || typeof value.path !== 'string' || !/^audio\/[a-z0-9_]+\.wav$/.test(value.path)) throw new Error('音频资源路径无效');
                result.audioClips.set(key, result.own(await load<AudioClip>(`development/${value.path.replace(/\.wav$/, '')}`, AudioClip)));
            }
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
        if (this.disposing || this.notifying) throw new Error('世界资源释放不能重入');
        this.disposing = true;
        try {
            this.notifyBeforeInvalidation('dispose');
            this.sourceMap?.dispose(); this.sourceMap = undefined;
            this.releaseSourceInvalidation?.(); this.releaseSourceInvalidation = undefined;
            this.disposed = true; this.background = null;
            for (const asset of Array.from(this.owned)) asset.decRef();
            this.owned.clear(); this.frames.clear(); this.audioClips.clear(); this.beforeInvalidation.clear();
        } finally { this.disposing = false; }
    }
}
