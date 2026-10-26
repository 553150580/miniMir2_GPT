import { _decorator, Component, EventTouch, isValid, Node, warn } from 'cc';
import { PanelStackState } from './PanelStackState';

const { ccclass, property } = _decorator;

@ccclass('UIMgr')
export class UIMgr extends Component {
    static readonly EVENT_CHANGED = 'ui-stack-changed';

    @property(Node) panelRoot: Node | null = null;
    @property(Node) backdrop: Node | null = null;
    private readonly stack = new PanelStackState<Node>();

    get top(): Node | null { return this.stack.top; }
    get isOpen(): boolean { return this.stack.size > 0; }

    protected onLoad(): void {
        this.panelRoot ??= this.node.getChildByPath('Panels');
        this.backdrop ??= this.node.getChildByPath('Backdrop');
        if (!this.panelRoot || !this.backdrop) warn('UIMgr 需要编辑器保存的 Panels 与 Backdrop 引用');
        this.refresh();
    }

    protected onEnable(): void {
        if (isValid(this.backdrop, true)) this.backdrop.on(Node.EventType.TOUCH_END, this.onBackdrop, this);
    }
    protected onDisable(): void {
        if (isValid(this.backdrop, true)) this.backdrop.off(Node.EventType.TOUCH_END, this.onBackdrop, this);
    }

    open(name: string): Node | null {
        if (!isValid(this, true) || !isValid(this.node, true) || !isValid(this.panelRoot, true)) return null;
        const panel = this.panelRoot?.children.find(child => child.name === name) ?? null;
        if (!panel) { warn(`普通面板未登记：${name}`); return null; }
        this.stack.open(panel);
        this.refresh();
        this.node.emit(UIMgr.EVENT_CHANGED, this.top);
        return panel;
    }

    back(): boolean {
        if (!isValid(this, true) || !isValid(this.node, true)) return false;
        if (!this.stack.back()) return false;
        this.refresh();
        this.node.emit(UIMgr.EVENT_CHANGED, this.top);
        return true;
    }

    closeAll(): void {
        if (!isValid(this, true) || !isValid(this.node, true)) return;
        this.stack.clear();
        this.refresh();
        this.node.emit(UIMgr.EVENT_CHANGED, null);
    }

    private refresh(): void {
        // 隐藏下层保留已有节点、组件数据及 ScrollView 的滚动位置。
        if (isValid(this.panelRoot, true)) for (const panel of this.panelRoot.children) {
            if (isValid(panel, true)) panel.active = panel === this.top;
        }
        if (isValid(this.backdrop, true)) this.backdrop.active = this.isOpen;
    }

    private onBackdrop(event: EventTouch): void {
        event.propagationStopped = true;
        this.back();
    }
}
