import { _decorator, Component, EventTouch, game, Game, isValid, Node, UITransform, Vec3, view, warn } from 'cc';
import { direction8, NavigationIntent } from './NavigationIntent';

const { ccclass, property } = _decorator;

@ccclass('TouchJoystick')
export class TouchJoystick extends Component {
    static readonly EVENT_DIRECTION = 'joystick-direction';
    @property(Node) knob: Node | null = null;
    @property radius = 72;
    @property deadZone = 0.18;
    private navigation?: NavigationIntent;
    private touchId: number | null = null;
    private center = new Vec3();
    private transform: UITransform | null = null;

    bind(navigation: NavigationIntent): void { this.navigation = navigation; }

    protected onLoad(): void {
        this.knob ??= this.node.getChildByPath('Knob');
        this.transform = this.getComponent(UITransform);
        if (this.knob) this.center.set(this.knob.position);
        if (!this.knob || !this.transform) warn('摇杆需要编辑器保存的 UITransform 与 Knob');
    }

    protected onEnable(): void {
        this.node.on(Node.EventType.TOUCH_START, this.onStart, this);
        this.node.on(Node.EventType.TOUCH_MOVE, this.onMove, this);
        this.node.on(Node.EventType.TOUCH_END, this.onEnd, this);
        this.node.on(Node.EventType.TOUCH_CANCEL, this.onEnd, this);
        game.on(Game.EVENT_HIDE, this.release, this);
        view.on('canvas-resize', this.release, this);
        view.on('design-resolution-changed', this.release, this);
    }

    protected onDisable(): void {
        if (isValid(this.node, true)) {
            this.node.off(Node.EventType.TOUCH_START, this.onStart, this);
            this.node.off(Node.EventType.TOUCH_MOVE, this.onMove, this);
            this.node.off(Node.EventType.TOUCH_END, this.onEnd, this);
            this.node.off(Node.EventType.TOUCH_CANCEL, this.onEnd, this);
        }
        game.off(Game.EVENT_HIDE, this.release, this);
        view.off('canvas-resize', this.release, this);
        view.off('design-resolution-changed', this.release, this);
        this.release();
    }

    // 可由面板打开、失焦协调层调用；永远不取消地面目标。
    release(): void {
        this.touchId = null;
        if (isValid(this.knob, true)) this.knob.setPosition(this.center);
        this.navigation?.releaseJoystick();
        if (isValid(this, true) && isValid(this.node, true)) {
            this.node.emit(TouchJoystick.EVENT_DIRECTION, { x: 0, y: 0 });
        }
    }

    private onStart(event: EventTouch): void {
        event.propagationStopped = true;
        if (this.touchId !== null || event.getID() === null) return;
        this.touchId = event.getID();
        this.applyTouch(event);
    }

    private onMove(event: EventTouch): void {
        event.propagationStopped = true;
        if (event.getID() === this.touchId && this.touchId !== null) this.applyTouch(event);
    }

    private onEnd(event: EventTouch): void {
        event.propagationStopped = true;
        if (event.getID() === this.touchId && this.touchId !== null) this.release();
    }

    private applyTouch(event: EventTouch): void {
        if (!isValid(this, true) || !isValid(this.node, true) || !isValid(this.transform, true)) return;
        const point = event.getUILocation();
        const local = this.transform.convertToNodeSpaceAR(new Vec3(point.x, point.y, 0));
        const radius = Number.isFinite(this.radius) ? Math.max(1, this.radius) : 72;
        const x = local.x - this.center.x;
        const y = local.y - this.center.y;
        const length = Math.hypot(x, y);
        const scale = length > radius ? radius / length : 1;
        if (isValid(this.knob, true)) this.knob.setPosition(this.center.x + x * scale, this.center.y + y * scale, this.center.z);
        const deadZone = Number.isFinite(this.deadZone) ? Math.min(0.95, Math.max(0, this.deadZone)) : 0.18;
        const direction = direction8(x / radius, y / radius, deadZone);
        // 在死区内按住摇杆只释放方向；真正推向任意方向才主动接管导航。
        this.navigation?.setJoystick(direction);
        this.node.emit(TouchJoystick.EVENT_DIRECTION, direction);
    }
}
