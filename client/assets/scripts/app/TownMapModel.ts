import { travelInteger } from './MapTravelModel';
import type { HeadProfile } from './EntityHeadAnchor';

export interface TownDirectoryMapRow {
    readonly mapId: string;
    readonly label: string;
    readonly destination: 1 | 2;
    readonly current: boolean;
}
export interface TownDirectoryRegion {
    readonly regionId: string;
    readonly label: string;
    readonly maps: readonly TownDirectoryMapRow[];
}

export interface TownNpc {
    readonly id: string;
    readonly name: string;
    readonly role: string;
    readonly position: { readonly x: number; readonly y: number };
    readonly profile: 'player_male' | 'player_female';
}

export function townDirectoryRegions(gameplay: any, current: string): TownDirectoryRegion[] {
    const townIds = gameplay?.town_travel?.destination_map_ids, catalog = gameplay?.map_catalog, regions = gameplay?.map_regions;
    if (!Array.isArray(townIds) || !Array.isArray(catalog) || !Array.isArray(regions) || !regions.length) throw new Error('地图目录尚未完整同步');
    const seen = new Set<string>(), seenRegions = new Set<string>(), result: TownDirectoryRegion[] = [];
    for (const region of regions) {
        if (!region || typeof region.id !== 'string' || !region.id || seenRegions.has(region.id) || typeof region.name !== 'string' || !region.name.trim()
            || !Array.isArray(region.map_ids) || !region.map_ids.length) throw new Error('地图目录区域无效');
        seenRegions.add(region.id);
        const maps: TownDirectoryMapRow[] = [];
        for (const value of region.map_ids) {
            if (typeof value !== 'string') throw new Error('地图目录ID必须为十进制字符串');
            const mapId = travelInteger(value, true), entry = catalog.find(map => map.id === mapId);
            if (seen.has(mapId) || !entry || ![1, 2].includes(entry.scene_kind) || typeof entry.name !== 'string' || !entry.name.trim()
                || (entry.scene_kind === 2 && (entry.compact_town !== true || !townIds.includes(mapId)))) throw new Error('地图目录目标无效');
            seen.add(mapId);
            maps.push({ mapId, label: entry.name, destination: entry.scene_kind === 2 ? 1 : 2, current: mapId === current });
        }
        result.push({ regionId: region.id, label: region.name, maps });
    }
    return result;
}

export function compactTownMap(floor: any, entry: any): any {
    if (entry.scene_kind !== 2 || entry.width !== 17 || entry.height !== 17 || floor.width !== entry.width || floor.height !== entry.height
        || floor.projection !== 'isometric_diamond' || typeof entry.name !== 'string' || !entry.name.trim() || !Array.isArray(entry.blocked)
        || !Array.isArray(entry.safe_zones) || !Array.isArray(entry.npcs) || !entry.npcs.length) throw new Error('紧凑城镇目录或地板不完整');
    const inBounds = (point: any): boolean => !!point && Number.isInteger(point.x) && Number.isInteger(point.y)
        && point.x >= 0 && point.y >= 0 && point.x < entry.width && point.y < entry.height;
    const pointKey = (point: { x: number; y: number }): string => `${point.x},${point.y}`;
    const blocked = new Set<string>();
    for (const point of entry.blocked) {
        if (!inBounds(point) || blocked.has(pointKey(point))) throw new Error('紧凑城镇阻挡格无效');
        blocked.add(pointKey(point));
    }
    if (!inBounds(entry.player_spawn) || blocked.has(pointKey(entry.player_spawn))) throw new Error('紧凑城镇出生格无效');
    const ids = new Set<string>(), positions = new Set<string>();
    for (const npc of entry.npcs as TownNpc[]) {
        if (!npc || ![npc.id, npc.name, npc.role].every(value => typeof value === 'string' && !!value.trim())
            || ids.has(npc.id) || !inBounds(npc.position) || positions.has(pointKey(npc.position))
            || pointKey(npc.position) === pointKey(entry.player_spawn) || !['player_male', 'player_female'].includes(npc.profile)) throw new Error('紧凑城镇NPC无效');
        ids.add(npc.id); positions.add(pointKey(npc.position));
    }
    return { ...floor, id: entry.id, name: entry.name, scene_kind: 2, blocked: entry.blocked, player_spawn: entry.player_spawn,
        safe_zones: entry.safe_zones, npcs: entry.npcs, props: entry.props ?? [] };
}

interface TownNpcPart {
    readonly frames: readonly string[];
    readonly frameMs: number;
}

// 原画独立清单使用本作 NPC 编号挂接。依赖完整才允许挂载，不让错图或缺图悄悄替代原画。
export function mergeTownNpcAppearances(manifest: any, extra: any, npcs: readonly TownNpc[]): any {
    if (extra?.schema_version !== 1 || !plainRecord(extra.npc_profiles) || !plainRecord(extra.profiles)
        || !plainRecord(extra.frames)) throw new Error('城镇NPC原画清单无效');
    for (const [id, raw] of Object.entries(extra.profiles)) {
        const profile = raw as any;
        const standing = profile?.layers?.[0]?.actions?.standing;
        if (!/^npc_original_\d+$/.test(id) || manifest.profiles[id] || profile?.id !== id
            || profile.layers?.length !== 1 || profile.layers[0].package !== 'Npc'
            || !Number.isSafeInteger(standing?.count) || standing.count < 1
            || !Number.isSafeInteger(standing.duration_ms) || standing.duration_ms < 1
            || !Array.isArray(standing.directions) || standing.directions.length !== 3
            || standing.directions.some((direction: any) => !Array.isArray(direction?.frames)
                || direction.frames.length !== standing.count || direction.frames.some((key: string) => !owns(extra.frames, key)))) {
            throw new Error(`城镇NPC原画外观无效：${id}`);
        }
    }
    for (const [key, raw] of Object.entries(extra.frames)) {
        const frame = raw as any;
        if (!/^Npc:\d+$/.test(key) || manifest.frames[key] || !/^town-npcs\/frames\/frame_\d+\.png$/.test(frame?.png)
            || ![frame.width, frame.height, frame.offset_x, frame.offset_y].every(Number.isSafeInteger)
            || frame.width < 1 || frame.height < 1 || frame.declared_empty !== false) throw new Error(`城镇NPC原画帧无效：${key}`);
    }
    for (const npc of npcs) {
        const profile = extra.npc_profiles[npc.id];
        if (typeof profile !== 'string' || !owns(extra.profiles, profile)) throw new Error(`城镇NPC缺少原画：${npc.id}`);
    }
    return { ...manifest, profiles: { ...manifest.profiles, ...extra.profiles }, frames: { ...manifest.frames, ...extra.frames },
        townNpcProfiles: extra.npc_profiles };
}

function plainRecord(value: unknown): value is Record<string, unknown> {
    return !!value && typeof value === 'object' && !Array.isArray(value);
}

function owns(value: Record<string, unknown>, key: string): boolean {
    return Object.prototype.hasOwnProperty.call(value, key);
}

// 参考商人表没有朝向。原画采用序列的默认方向；旧开发包仍显示原本的站立姿态。
export function townNpcAppearance(manifest: any, npc: TownNpc): { profile: HeadProfile; parts: TownNpcPart[] } {
    const original = manifest.townNpcProfiles?.[npc.id];
    const profile = manifest.profiles[original ?? npc.profile];
    const layers = profile?.layers?.filter((layer: any) => layer.package !== 'Weapon');
    if (!layers?.length) throw new Error(`城镇NPC缺少站立帧：${npc.id}`);
    const parts: TownNpcPart[] = [];
    for (const layer of layers) {
        const standing = layer.actions.standing;
        const frames = standing?.directions[original ? 0 : 4]?.frames;
        const keys: string[] = original ? frames : frames?.slice(0, 1);
        if (!keys?.length || keys.some(key => !key || !manifest.frames[key])) throw new Error(`城镇NPC缺少站立帧：${npc.id}`);
        const visible = keys.filter(key => !manifest.frames[key].declared_empty);
        if (visible.length) parts.push({ frames: visible, frameMs: original ? standing.duration_ms : 0 });
    }
    return { profile, parts };
}

// 提前准备当前 NPC 的完整站立动作，进入场景后可直接播放。
export function townNpcPreloadFrames(manifest: any, npc: TownNpc): string[] {
    return townNpcAppearance(manifest, npc).parts.flatMap(part => part.frames);
}
