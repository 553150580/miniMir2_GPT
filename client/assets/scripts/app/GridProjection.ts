export interface ProjectionPoint { x: number; y: number; }
export interface ProjectedDepth extends ProjectionPoint { layer: number; id: string; }
export function compareProjectedDepth(a: ProjectedDepth, b: ProjectedDepth): number {
    return b.y - a.y || a.x - b.x || a.layer - b.layer || a.id.localeCompare(b.id);
}
export interface ProjectionGeometry {
    width: number; height: number; tile_width: number; tile_height: number; soil_thickness_px: number;
    projection?: string;
}
export const LOGICAL_DIRECTIONS: readonly ProjectionPoint[] = [
    { x: 0, y: -1 }, { x: 1, y: -1 }, { x: 1, y: 0 }, { x: 1, y: 1 },
    { x: 0, y: 1 }, { x: -1, y: 1 }, { x: -1, y: 0 }, { x: -1, y: -1 },
];

// 逻辑格始终x向右、y向下；只在表现边界转成Cocos中向上的菱形坐标。
export class GridProjection {
    readonly surfaceWidth: number;
    readonly surfaceHeight: number;
    readonly worldHeight: number;
    readonly floorCenterY: number;
    constructor(readonly geometry: ProjectionGeometry) {
        if (![geometry.width, geometry.height].every(value => Number.isSafeInteger(value) && value > 0)
            || ![geometry.tile_width, geometry.tile_height].every(value => Number.isFinite(value) && value > 0)
            || !Number.isFinite(geometry.soil_thickness_px) || geometry.soil_thickness_px < 0
            || (geometry.projection && geometry.projection !== 'isometric_diamond')) throw new Error('地图菱形投影配置无效');
        this.surfaceWidth = (geometry.width + geometry.height) * geometry.tile_width / 2;
        this.surfaceHeight = (geometry.width + geometry.height) * geometry.tile_height / 2;
        this.worldHeight = this.surfaceHeight + geometry.soil_thickness_px;
        this.floorCenterY = geometry.soil_thickness_px / 2;
    }
    project(x: number, y: number): ProjectionPoint {
        const g = this.geometry;
        return { x: (x - y - (g.width - g.height) / 2) * g.tile_width / 2,
            y: this.floorCenterY - (x + y - (g.width + g.height - 2) / 2) * g.tile_height / 2 };
    }
    unproject(point: ProjectionPoint): ProjectionPoint {
        const g = this.geometry, y = point.y - this.floorCenterY;
        return { x: point.x / g.tile_width - y / g.tile_height + (g.width - 1) / 2,
            y: -point.x / g.tile_width - y / g.tile_height + (g.height - 1) / 2 };
    }
    cellAt(point: ProjectionPoint): ProjectionPoint | null {
        if (![point.x, point.y].every(Number.isFinite)) return null;
        const cell = this.unproject(point), x = Math.floor(cell.x + .5), y = Math.floor(cell.y + .5);
        return x >= 0 && y >= 0 && x < this.geometry.width && y < this.geometry.height ? { x, y } : null;
    }
    delta(x: number, y: number): ProjectionPoint { return { x: (x - y) * this.geometry.tile_width / 2, y: -(x + y) * this.geometry.tile_height / 2 }; }
    visualDirection(logicalDirection: number, count = 8): number {
        const direction = LOGICAL_DIRECTIONS[logicalDirection];
        if (!direction) throw new Error('地图逻辑方向无效');
        return this.angle(this.delta(direction.x, direction.y), count);
    }
    visualDirectionBetween(origin: ProjectionPoint, target: ProjectionPoint, count = 16, fallback = 4): number {
        return origin.x === target.x && origin.y === target.y ? this.visualDirection(fallback, count)
            : this.angle(this.delta(target.x - origin.x, target.y - origin.y), count);
    }
    logicalDirectionFromScreen(x: number, y: number): number | null {
        if (![x, y].every(Number.isFinite) || (!x && !y)) return null;
        const dx = x / this.geometry.tile_width - y / this.geometry.tile_height;
        const dy = -x / this.geometry.tile_width - y / this.geometry.tile_height;
        return (Math.round(Math.atan2(dx, -dy) * 4 / Math.PI) + 8) % 8;
    }
    corners(): readonly ProjectionPoint[] {
        const g = this.geometry;
        return [this.project(-.5, -.5), this.project(g.width - .5, -.5), this.project(g.width - .5, g.height - .5), this.project(-.5, g.height - .5)];
    }
    private angle(vector: ProjectionPoint, count: number): number {
        return (Math.round(Math.atan2(vector.x, vector.y) * count / (2 * Math.PI)) + count) % count;
    }
}
