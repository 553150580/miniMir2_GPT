import { _decorator, Button, Component, isValid, Node } from 'cc';
import { ViewStack } from './ViewStack';

const { ccclass, property } = _decorator;

@ccclass('Tab')
export class Tab extends Component {
    static readonly EVENT_CHANGED = 'tab-changed';
    @property([Button]) buttons: Button[] = [];
    @property(ViewStack) viewStack: ViewStack | null = null;
    @property selectedIndex = 0;
    private handlers: Array<() => void> = [];

    protected onLoad(): void {
        if (!this.buttons.length) this.buttons = this.node.children
            .map(node => node.getComponent(Button)).filter((button): button is Button => button !== null);
        this.handlers = this.buttons.map((_button, index) => () => { this.select(index); });
        this.select(this.selectedIndex);
    }

    protected onEnable(): void {
        this.buttons.forEach((button, index) => {
            if (isValid(button, true) && isValid(button.node, true)) {
                button.node.on(Button.EventType.CLICK, this.handlers[index], this);
            }
        });
    }

    protected onDisable(): void {
        this.buttons.forEach((button, index) => {
            if (isValid(button, true) && isValid(button.node, true)) {
                button.node.off(Button.EventType.CLICK, this.handlers[index], this);
            }
        });
    }

    select(index: number): boolean {
        if (!isValid(this, true) || !isValid(this.node, true)) return false;
        if (!Number.isInteger(index) || index < 0 || index >= this.buttons.length) return false;
        if (this.viewStack && (!isValid(this.viewStack, true) || !this.viewStack.show(index))) return false;
        this.selectedIndex = index;
        this.buttons.forEach((button, buttonIndex) => {
            if (!isValid(button, true) || !isValid(button.node, true)) return;
            const selected: Node | null = button.node.getChildByPath('Selected');
            if (isValid(selected, true)) selected.active = buttonIndex === index;
        });
        this.node.emit(Tab.EVENT_CHANGED, index);
        return true;
    }
}
