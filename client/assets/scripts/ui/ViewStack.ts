import { _decorator, Component, isValid, Node } from 'cc';

const { ccclass, property } = _decorator;

@ccclass('ViewStack')
export class ViewStack extends Component {
    static readonly EVENT_CHANGED = 'view-stack-changed';
    @property([Node]) pages: Node[] = [];
    @property selectedIndex = 0;

    protected onLoad(): void { this.show(this.selectedIndex); }

    show(index: number): boolean {
        if (!isValid(this, true) || !isValid(this.node, true)) return false;
        if (!this.pages.length) this.pages = this.node.children.slice();
        if (!Number.isInteger(index) || index < 0 || index >= this.pages.length) return false;
        if (!isValid(this.pages[index], true)) return false;
        this.selectedIndex = index;
        this.pages.forEach((page, pageIndex) => { if (isValid(page, true)) page.active = pageIndex === index; });
        this.node.emit(ViewStack.EVENT_CHANGED, index);
        return true;
    }
}
