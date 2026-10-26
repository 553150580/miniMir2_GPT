import { _decorator, Component, screen, view, ResolutionPolicy } from 'cc';
import { viewportGeometry } from './ViewportGeometry';
const { ccclass } = _decorator;

@ccclass('ViewportAdapter')
export class ViewportAdapter extends Component {
    private lastWidth = 0;
    private lastHeight = 0;
    onLoad(): void { this.resize(); view.on('canvas-resize', this.resize, this); }
    onDestroy(): void { view.off('canvas-resize', this.resize, this); }
    private resize(): void {
        const frame = screen.windowSize;
        if (frame.width <= 0 || frame.height <= 0) return;
        if (frame.width === this.lastWidth && frame.height === this.lastHeight) return;
        this.lastWidth = frame.width; this.lastHeight = frame.height;
        // 逻辑画布与窗口保持同一宽高比，两个方向使用完全相同的缩放比例。
        // 多出的逻辑宽/高交给Widget拉伸地图容器，固定HUD本身不改尺寸。
        const logical = viewportGeometry(frame.width, frame.height);
        view.setDesignResolutionSize(logical.width, logical.height, ResolutionPolicy.SHOW_ALL);
    }
}
