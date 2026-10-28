import { GridProjection, LOGICAL_DIRECTIONS } from './GridProjection';
import type { ProjectionGeometry, ProjectionPoint } from './GridProjection';
import type { SourceMapModel } from './SourceMapModel';

// 复用既有实体/技能所需的投影契约，所有公共几何方法均改用来源矩形坐标。
// GridProjection构造只校验公共尺寸；派生构造随后登记真实来源几何，不把来源地图声明成菱形。
export class SourceProjection extends GridProjection {
    declare readonly geometry: ProjectionGeometry;
    declare readonly surfaceWidth: number;
    declare readonly surfaceHeight: number;
    declare readonly worldHeight: number;
    declare readonly floorCenterY: number;
    constructor(readonly source: SourceMapModel) {
        const g = source.manifest;
        super({ width: g.width, height: g.height, tile_width: g.tile_width, tile_height: g.tile_height, soil_thickness_px: 0 });
        this.geometry = g; this.surfaceWidth = g.pixel_width; this.surfaceHeight = g.pixel_height;
        this.worldHeight = g.pixel_height; this.floorCenterY = 0;
    }
    project(x: number, y: number): ProjectionPoint {
        const point = this.source.project(x + .5, y + .5); return { x: point.x, y: -point.y };
    }
    unproject(point: ProjectionPoint): ProjectionPoint {
        const cell = this.source.unproject({ x: point.x, y: -point.y }); return { x: cell.x - .5, y: cell.y - .5 };
    }
    cellAt(point: ProjectionPoint): ProjectionPoint | null { return this.source.cellAt({ x: point.x, y: -point.y }); }
    delta(x: number, y: number): ProjectionPoint { return { x: x * 48, y: -y * 32 }; }
    visualDirection(logicalDirection: number, count = 8): number {
        const direction = LOGICAL_DIRECTIONS[logicalDirection]; if (!direction) throw new Error('来源地图逻辑方向无效');
        return this.nativeAngle(this.delta(direction.x, direction.y), count);
    }
    visualDirectionBetween(origin: ProjectionPoint, target: ProjectionPoint, count = 16, fallback = 4): number {
        return origin.x === target.x && origin.y === target.y ? this.visualDirection(fallback, count)
            : this.nativeAngle(this.delta(target.x - origin.x, target.y - origin.y), count);
    }
    logicalDirectionFromScreen(x: number, y: number): number | null {
        if (![x, y].every(Number.isFinite) || (!x && !y)) return null;
        return (Math.round(Math.atan2(x / 48, y / 32) * 4 / Math.PI) + 8) % 8;
    }
    corners(): readonly ProjectionPoint[] {
        return [{ x: 0, y: 0 }, { x: this.surfaceWidth, y: 0 }, { x: this.surfaceWidth, y: -this.surfaceHeight }, { x: 0, y: -this.surfaceHeight }];
    }
    private nativeAngle(vector: ProjectionPoint, count: number): number {
        if (!Number.isInteger(count) || count <= 0) throw new Error('来源地图视觉方向数无效');
        return (Math.round(Math.atan2(vector.x, vector.y) * count / (2 * Math.PI)) + count) % count;
    }
}
