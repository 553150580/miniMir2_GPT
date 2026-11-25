// 本文件选择原版地表贴图，只使用地图目录的身份及尺寸，不改变地图逻辑。
export const regionalMapStylesPath = 'development/map-styles/manifest';

interface RegionalMapGeometry {
    readonly id: string;
    readonly scene_kind: number;
    readonly width?: number;
    readonly height?: number;
}

interface RegionalMapStyle {
    readonly map_id: string;
    readonly scene_kind: 1 | 2;
    readonly width: number;
    readonly height: number;
    readonly frame: string;
}

// regionalMapStyle 只接受受控目录的同场景地表。没有地图条目时沿用地图自身贴图。
export function regionalMapStyle(value: unknown, target: RegionalMapGeometry): RegionalMapStyle | undefined {
    const catalog = value as { schema_version?: unknown; styles?: unknown } | null | undefined;
    if (catalog?.schema_version !== 1 || !Array.isArray(catalog.styles)) throw new Error('区域地表清单无效');
    const seen = new Set<string>();
    let selected: RegionalMapStyle | undefined;
    for (const row of catalog.styles) {
        if (!row || typeof row.map_id !== 'string' || !/^[1-9]\d{0,19}$/.test(row.map_id) || seen.has(row.map_id)
            || (row.scene_kind !== 1 && row.scene_kind !== 2) || !Number.isSafeInteger(row.width) || row.width <= 0
            || !Number.isSafeInteger(row.height) || row.height <= 0 || typeof row.frame !== 'string'
            || !/^development\/map-styles\/(?:[a-z0-9][a-z0-9_-]*\/)*[a-z0-9][a-z0-9_-]*$/.test(row.frame)) {
            throw new Error('区域地表条目无效');
        }
        seen.add(row.map_id);
        if (target.id === row.map_id) selected = row;
    }
    if (!selected) return undefined;
    if (selected.scene_kind !== target.scene_kind || selected.width !== target.width || selected.height !== target.height) {
        throw new Error('区域地表与当前地图身份或尺寸不一致');
    }
    return selected;
}
