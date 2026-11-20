import { travelInteger } from './MapTravelModel';

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

// NPC只加载与人物同一套站立姿态所需的身体和头发，不持有玩家武器与战斗帧。
export function townNpcFrames(manifest: any, npc: TownNpc): string[] {
    const layers = manifest.profiles[npc.profile]?.layers?.filter((layer: any) => layer.package !== 'Weapon');
    const keys = layers?.map((layer: any) => layer.actions.standing?.directions[4]?.frames[0]);
    if (!keys?.length || keys.some((key: string) => !key || !manifest.frames[key])) throw new Error(`城镇NPC缺少站立帧：${npc.id}`);
    return keys.filter((key: string) => !manifest.frames[key].declared_empty);
}
