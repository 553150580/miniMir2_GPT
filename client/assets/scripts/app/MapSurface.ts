import { Color, Graphics, Node, Sprite, UITransform } from 'cc';
import { GridProjection, ProjectionPoint } from './GridProjection';
import type { WorldAssets } from './WorldAssets';

function polygon(graphics: Graphics, points: readonly ProjectionPoint[], color: Color): void {
    graphics.fillColor = color; graphics.moveTo(points[0].x, points[0].y);
    for (const point of points.slice(1)) graphics.lineTo(point.x, point.y);
    graphics.close(); graphics.fill();
}
function segment(graphics: Graphics, a: ProjectionPoint, b: ProjectionPoint): void { graphics.moveTo(a.x, a.y); graphics.lineTo(b.x, b.y); }

// 只绘制地图几何；土层、地板、细网格与目标容器均为已保存的场景节点。
export function prepareMapSurface(world: Node, assets: WorldAssets, projection: GridProjection): void {
    const saved = (path: string): Node => { const value = world.getChildByPath(path); if (!value) throw new Error(`地图缺少保存节点：${path}`); return value; };
    const soil = saved('Soil').getComponent(Graphics)!, grid = saved('GridLines').getComponent(Graphics)!;
    const surface = saved('Surface'), marker = saved('TargetMarker');
    world.getComponent(Sprite)!.enabled = false;
    world.getComponent(UITransform)!.setContentSize(projection.surfaceWidth, projection.worldHeight);
    surface.setPosition(0, projection.floorCenterY);
    surface.getComponent(UITransform)!.setContentSize(projection.surfaceWidth, projection.surfaceHeight);
    const sprite = surface.getComponent(Sprite)!; sprite.spriteFrame = assets.background; sprite.trim = false; sprite.sizeMode = Sprite.SizeMode.CUSTOM;
    const [, east, south, west] = projection.corners(), depth = projection.geometry.soil_thickness_px;
    const lower = (point: ProjectionPoint, y = depth): ProjectionPoint => ({ x: point.x, y: point.y - y });
    soil.clear();
    polygon(soil, [west, south, lower(south), lower(west)], new Color(100, 72, 43, 255));
    polygon(soil, [south, east, lower(east), lower(south)], new Color(68, 48, 32, 255));
    // 层纹及草皮属于地图材质表现，厚度始终取配置土层尺寸，窗口不参与缩放。
    for (const fraction of [.18, .40, .66, .87]) {
        soil.strokeColor = new Color(35, 25, 18, 100); soil.lineWidth = 1.5;
        segment(soil, lower(west, depth * fraction), lower(south, depth * fraction));
        segment(soil, lower(south, depth * fraction), lower(east, depth * fraction)); soil.stroke();
    }
    polygon(soil, [west, south, lower(south, 3), lower(west, 3)], new Color(62, 78, 37, 255));
    polygon(soil, [south, east, lower(east, 3), lower(south, 3)], new Color(43, 59, 28, 255));
    soil.strokeColor = new Color(0, 0, 0, 110); soil.lineWidth = 2;
    segment(soil, lower(west, 3), lower(south, 3)); segment(soil, lower(south, 3), lower(east, 3)); soil.stroke();
    grid.clear(); grid.strokeColor = new Color(17, 26, 20, 65); grid.lineWidth = .75;
    for (let x = 0; x <= projection.geometry.width; x++) segment(grid, projection.project(x - .5, -.5), projection.project(x - .5, projection.geometry.height - .5));
    for (let y = 0; y <= projection.geometry.height; y++) segment(grid, projection.project(-.5, y - .5), projection.project(projection.geometry.width - .5, y - .5));
    grid.stroke();
    marker.getComponent(Sprite)!.enabled = false;
    const target = saved('TargetMarker/Diamond').getComponent(Graphics)!, halfW = projection.geometry.tile_width / 2, halfH = projection.geometry.tile_height / 2;
    target.clear(); polygon(target, [{ x: 0, y: halfH }, { x: halfW, y: 0 }, { x: 0, y: -halfH }, { x: -halfW, y: 0 }], new Color(255, 220, 90, 65));
    target.strokeColor = new Color(255, 220, 90, 210); target.lineWidth = 1.5;
    target.moveTo(0, halfH); target.lineTo(halfW, 0); target.lineTo(0, -halfH); target.lineTo(-halfW, 0); target.close(); target.stroke();
}
