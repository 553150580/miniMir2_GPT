import { _decorator, Button, Component, EventTouch, isValid, Label, Node, warn } from 'cc';
import { PanelStackState } from './PanelStackState';

const { ccclass, property } = _decorator;
export interface PopupText { readonly title?: string; readonly message?: string; }

@ccclass('PopupView')
export class PopupView extends Component {
    static readonly EVENT_CHANGED = 'popup-stack-changed';

    @property(Node) panelRoot: Node | null = null;
    @property(Node) backdrop: Node | null = null;
    private readonly stack = new PanelStackState<Node>();
    private readonly pending = new Map<Node, (accepted: boolean) => void>();
    private readonly buttons: Array<{ button: Button; handler: () => void }> = [];

    get top(): Node | null { return this.stack.top; }
    get isOpen(): boolean { return this.stack.size > 0; }

    protected onLoad(): void {
        this.panelRoot ??= this.node.getChildByPath('Panels');
        this.backdrop ??= this.node.getChildByPath('Backdrop');
        if (!this.panelRoot || !this.backdrop) warn('PopupView 需要编辑器保存的 Panels 与 Backdrop 引用');
        for (const panel of this.panelRoot?.children ?? []) {
            for (const [path, accepted] of [['ConfirmButton', true], ['CancelButton', false]] as const) {
                const button = panel.getChildByPath(path)?.getComponent(Button);
                if (button) this.buttons.push({ button, handler: () => {
                    if (this.top === panel) this.closeTop(accepted);
                } });
            }
        }
        this.refresh();
    }

    protected onEnable(): void {
        if (isValid(this.backdrop, true)) this.backdrop.on(Node.EventType.TOUCH_END, this.onBackdrop, this);
        for (const { button, handler } of this.buttons) if (isValid(button, true) && isValid(button.node, true)) {
            button.node.on(Button.EventType.CLICK, handler, this);
        }
    }

    protected onDisable(): void {
        if (isValid(this.backdrop, true)) this.backdrop.off(Node.EventType.TOUCH_END, this.onBackdrop, this);
        for (const { button, handler } of this.buttons) if (isValid(button, true) && isValid(button.node, true)) {
            button.node.off(Button.EventType.CLICK, handler, this);
        }
    }

    protected onDestroy(): void {
        for (const resolve of this.pending.values()) resolve(false);
        this.pending.clear();
        this.buttons.length = 0;
        this.stack.clear();
    }

    show(name: string, text: PopupText = {}): Node | null {
        if (!isValid(this, true) || !isValid(this.node, true) || !isValid(this.panelRoot, true)) return null;
        const panel = this.panelRoot?.children.find(child => child.name === name) ?? null;
        if (!panel) { warn(`覆盖弹窗未登记：${name}`); return null; }
        // 替换同一确认视图时先结束旧请求，避免悬挂确认结果。
        this.pending.get(panel)?.(false);
        this.pending.delete(panel);
        if (text.title !== undefined) this.setLabel(panel, 'Title', text.title);
        if (text.message !== undefined) this.setLabel(panel, 'Message', text.message);
        this.stack.open(panel);
        this.refresh();
        this.node.emit(PopupView.EVENT_CHANGED, this.top);
        return panel;
    }

    confirm(name: string, message: string, title = '确认'): Promise<boolean> {
        const panel = this.show(name, { title, message });
        return panel ? new Promise(resolve => this.pending.set(panel, resolve)) : Promise.resolve(false);
    }

    accept(): boolean { return this.closeTop(true); }
    back(): boolean { return this.closeTop(false); }

    private closeTop(accepted: boolean): boolean {
        if (!isValid(this, true) || !isValid(this.node, true)) return false;
        const panel = this.stack.back();
        if (!panel) return false;
        const resolve = this.pending.get(panel);
        this.pending.delete(panel);
        this.refresh();
        this.node.emit(PopupView.EVENT_CHANGED, this.top);
        resolve?.(accepted);
        return true;
    }

    private refresh(): void {
        // 此层不引用 UIMgr，因此关闭详情不会重建、隐藏或清理普通面板。
        if (isValid(this.panelRoot, true)) for (const panel of this.panelRoot.children) {
            if (isValid(panel, true)) panel.active = panel === this.top;
        }
        if (isValid(this.backdrop, true)) this.backdrop.active = this.isOpen;
    }

    private setLabel(panel: Node, path: string, value: string): void {
        const label = panel.getChildByPath(path)?.getComponent(Label);
        if (label) label.string = value;
    }

    private onBackdrop(event: EventTouch): void {
        event.propagationStopped = true;
        this.back();
    }
}
