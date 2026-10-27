import { GridProjection, LOGICAL_DIRECTIONS, ProjectionPoint } from './GridProjection';

export interface BeamFrame { width: number; height: number; offset_x: number; offset_y: number; }
function rotate(point: ProjectionPoint, radians: number): ProjectionPoint {
    return { x: point.x * Math.cos(radians) - point.y * Math.sin(radians), y: point.x * Math.sin(radians) + point.y * Math.cos(radians) };
}
// 原图及原WIL偏移作为整体刚体旋转；Mask只裁起点到逻辑端点，不拉伸原图或逐帧拟合主轴。
export function beamGeometry(projection: GridProjection, logicalDirection: number, cells: number, vectors: readonly (readonly number[])[], frames: readonly BeamFrame[]) {
    if (!Number.isSafeInteger(cells) || cells < 1 || vectors.length !== 8 || !frames.length) throw new Error('疾光原图方向或范围配置缺失');
    const visualDirection = projection.visualDirection(logicalDirection), native = vectors[visualDirection], logical = LOGICAL_DIRECTIONS[logicalDirection];
    if (!native || native.length !== 2 || !native.every(Number.isFinite) || (!native[0] && !native[1])) throw new Error('疾光原图方向无效');
    const delta = projection.delta(logical.x * cells, logical.y * cells), angle = Math.atan2(delta.y, delta.x), nativeAngle = Math.atan2(native[1], native[0]);
    const halfSourceWidth = Math.abs(vectors[2][0]) / 2, halfSourceHeight = Math.abs(vectors[0][1]) / 2;
    const position = (frame: BeamFrame): ProjectionPoint => rotate({ x: frame.offset_x - halfSourceWidth, y: halfSourceHeight - frame.offset_y }, -nativeAngle);
    let cross = 1;
    for (const frame of frames) {
        const left = frame.offset_x - halfSourceWidth, top = halfSourceHeight - frame.offset_y;
        for (const x of [left, left + frame.width]) for (const y of [top, top - frame.height]) cross = Math.max(cross, Math.abs(rotate({ x, y }, -nativeAngle).y));
    }
    return { visualDirection, length: Math.hypot(delta.x, delta.y), height: (cross + 1) * 2,
        angleDegrees: angle * 180 / Math.PI, spriteAngleDegrees: -nativeAngle * 180 / Math.PI,
        rigidRotationDegrees: (((angle - nativeAngle) * 180 / Math.PI + 180) % 360 + 360) % 360 - 180, position };
}
