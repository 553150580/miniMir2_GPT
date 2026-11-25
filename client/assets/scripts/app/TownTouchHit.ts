import { Node, Sprite, Texture2D, UITransform, Vec3, view } from 'cc';

// 保留 Creator 相机、触摸窗口及父遮罩检查，只缩小实际可交互的像素区域。
export function bindTownTouchHit(node: Node, opaque: (x: number, y: number, width: number) => boolean): void {
    const transform = node.getComponent(UITransform)!;
    const hitRectangle = transform.hitTest.bind(transform);
    const ui = new Vec3(), local = new Vec3();
    transform.hitTest = (screenPoint, windowId) => {
        if (!hitRectangle(screenPoint, windowId)) return false;
        const viewport = view.getViewportRect();
        ui.set((screenPoint.x - viewport.x) / view.getScaleX(), (screenPoint.y - viewport.y) / view.getScaleY(), 0);
        transform.convertToNodeSpaceAR(ui, local);
        const x = local.x + transform.width * transform.anchorPoint.x;
        const y = transform.height * (1 - transform.anchorPoint.y) - local.y;
        return x >= 0 && y >= 0 && x < transform.width && y < transform.height && opaque(x, y, transform.width);
    };
}

// 城镇角色、宠物沿用当前正在显示的原图。仅在触摸命中外框时采样，不遍历站姿与行走帧。
export class TownActorTouch {
    private context?: CanvasRenderingContext2D | null;

    bind(body: Sprite): void {
        bindTownTouchHit(body.node, (x, y) => {
            const frame = body.spriteFrame;
            if (!frame) return false;
            const rect = frame.rect, size = frame.originalSize, offset = frame.offset;
            // trim=false 时缩进要与 Creator 的原尺寸精灵四边一致，透明裁边也要穿透。
            const left = (size.width - rect.width) / 2 + offset.x;
            const top = (size.height - rect.height) / 2 - offset.y;
            const column = Math.floor(x - left), row = Math.floor(y - top);
            if (column < 0 || row < 0 || column >= rect.width || row >= rect.height) return false;
            // 自定义旅行图集使用子矩形；动态合图则要回原纹理，合图可能没有可读的 CPU 像素。
            const original = frame.original, texture = original?._texture ?? frame.texture;
            const source = texture instanceof Texture2D ? texture.image?.data : undefined;
            if (!source || ArrayBuffer.isView(source) || frame.rotated || typeof document === 'undefined') return true;
            if (this.context === undefined) {
                const canvas = document.createElement('canvas'); canvas.width = canvas.height = 1;
                this.context = canvas.getContext('2d', { willReadFrequently: true });
            }
            const context = this.context;
            if (!context) return true;
            try {
                context.clearRect(0, 0, 1, 1);
                context.drawImage(source, (original?._x ?? rect.x) + column, (original?._y ?? rect.y) + row, 1, 1, 0, 0, 1, 1);
                return context.getImageData(0, 0, 1, 1).data[3] !== 0;
            } catch {
                // 某些平台不提供可读图像。保留裁边内的实体点击，并丢弃可能被标记为不可读的画布。
                this.context = undefined;
                return true;
            }
        });
    }
}
