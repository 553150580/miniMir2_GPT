// 基准画面以内等比缩小，额外空间转为逻辑视口；固定控件的逻辑尺寸不变。
export function viewportGeometry(width: number, height: number): { width: number; height: number; scale: number } {
    if (!Number.isFinite(width) || !Number.isFinite(height) || width <= 0 || height <= 0) throw new Error('窗口尺寸必须为有限正数');
    const scale = Math.min(width / 720, height / 1280);
    return { width: width / scale, height: height / scale, scale };
}
