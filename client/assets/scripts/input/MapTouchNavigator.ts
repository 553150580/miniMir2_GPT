import { _decorator, Component, EventTouch, game, Game, isValid, Node, Vec2, view, warn } from 'cc';
import { GridPoint, NavigationIntent } from './NavigationIntent';

const { ccclass, property } = _decorator;
export type ScreenToGrid = (location: Vec2) => GridPoint | null;

@ccclass('MapTouchNavigator')
export class MapTouchNavigator extends Component {
    static readonly EVENT_GROUND_TARGET = 'ground-navigation-target';
    @property tapSlop = 18;
    @property maxTapSeconds = 0.45;
    private navigation?: NavigationIntent;
    private screenToGrid?: ScreenToGrid;
    private touchId: number | null = null;
    private startPoint = new Vec2();
    private startTime = 0;
    private moved = false;

    bind(navigation: NavigationIntent, screenToGrid: ScreenToGrid): void {
        this.navigation = navigation;
        this.screenToGrid = screenToGrid;
    }

    protected onEnable(): void {
        this.node.on(Node.EventType.TOUCH_START, this.onStart, this);
        this.node.on(Node.EventType.TOUCH_MOVE, this.onMove, this);
        this.node.on(Node.EventType.TOUCH_END, this.onEnd, this);
        this.node.on(Node.EventType.TOUCH_CANCEL, this.resetTouch, this);
        game.on(Game.EVENT_HIDE, this.resetTouch, this);
        view.on('canvas-resize', this.resetTouch, this);
        view.on('design-resolution-changed', this.resetTouch, this);
    }

    protected onDisable(): void {
        if (isValid(this.node, true)) {
            this.node.off(Node.EventType.TOUCH_START, this.onStart, this);
            this.node.off(Node.EventType.TOUCH_MOVE, this.onMove, this);
            this.node.off(Node.EventType.TOUCH_END, this.onEnd, this);
            this.node.off(Node.EventType.TOUCH_CANCEL, this.resetTouch, this);
        }
        game.off(Game.EVENT_HIDE, this.resetTouch, this);
        view.off('canvas-resize', this.resetTouch, this);
        view.off('design-resolution-changed', this.resetTouch, this);
        this.resetTouch();
    }

    private onStart(event: EventTouch): void {
        // 实体子节点、聊天和面板事件不能被当作地面轻触。
        if (event.target !== this.node || this.touchId !== null || event.getID() === null) return;
        this.touchId = event.getID();
        this.startPoint.set(event.getUILocation());
        this.startTime = game.totalTime;
        this.moved = false;
        event.propagationStopped = true;
    }

    private onMove(event: EventTouch): void {
        if (event.getID() !== this.touchId || this.touchId === null) return;
        if (Vec2.distance(this.startPoint, event.getUILocation()) > Math.max(0, this.tapSlop)) this.moved = true;
        event.propagationStopped = true;
    }

    private onEnd(event: EventTouch): void {
        if (event.getID() !== this.touchId || this.touchId === null) return;
        event.propagationStopped = true;
        const valid = !this.moved && game.totalTime - this.startTime <= Math.max(0, this.maxTapSeconds) * 1000
            && Vec2.distance(this.startPoint, event.getUILocation()) <= Math.max(0, this.tapSlop);
        this.resetTouch();
        if (!valid || !this.navigation || !this.screenToGrid) return;
        try {
            const target = this.screenToGrid(event.getUILocation());
            if (target && this.navigation.setGroundTarget(target)) this.node.emit(MapTouchNavigator.EVENT_GROUND_TARGET, target);
        } catch (error) { warn('地图坐标转换失败，保留原导航目标', error); }
    }

    private resetTouch(): void { this.touchId = null; this.moved = false; }
}
