import { Button, isValid, Label, Node } from 'cc';
import { List } from '../ui/List';
import { PopupView } from '../ui/PopupView';
import { detailLines } from './CombatStatusModel';

export interface ItemAction {
    readonly label: string;
    readonly enabled: boolean;
    readonly run: () => void | Promise<void>;
}

export interface ItemActionView {
    readonly title: string;
    readonly lines: readonly string[];
    readonly actions: readonly ItemAction[];
    readonly options?: readonly { readonly label: string; readonly key: string; readonly enabled: boolean; readonly choose: () => void }[];
    readonly optionsFirst?: boolean;
    readonly quantity?: { readonly value: string; readonly minus?: () => void; readonly plus?: () => void };
}

const actionNames = ['Action1', 'Action2', 'Action3'] as const;

// 详情始终读取原条目的最新数据，操作前再确认范围，报价确认继续由原按钮流程处理。
export class GameItemActions {
    private readonly panel: Node;
    private readonly list: List;
    private readonly handlers: { node: Node; run: () => void }[] = [];
    private read?: () => ItemActionView | null;
    private busy = false;
    private rendered = '';

    constructor(safe: Node, private readonly popup: PopupView, private readonly report: (error: unknown) => void, panelName = 'ItemActions') {
        const panel = safe.getChildByPath('PopupView/Panels/' + panelName);
        const list = panel?.getChildByName('List')?.getComponent(List);
        if (!panel || !list) throw new Error('物品详情尚未完整保存在场景');
        this.panel = panel;
        this.list = list;
        for (const [index, name] of actionNames.entries()) this.bind(name, () => this.activate(index));
        this.list.node.on(List.EVENT_ITEM_CLICK, this.chooseOption, this);
        this.bind('Minus', () => this.adjust('minus'));
        this.bind('Plus', () => this.adjust('plus'));
        this.popup.node.on(PopupView.EVENT_CHANGED, this.refresh, this);
    }

    isCurrent(read: () => ItemActionView | null): boolean { return this.read === read && this.popup.top === this.panel; }

    open(read: () => ItemActionView | null): void {
        this.read = read;
        this.busy = false;
        this.rendered = '';
        this.popup.show(this.panel.name);
        this.refresh();
    }

    refresh = (): void => {
        if (!this.read || this.popup.top !== this.panel) return;
        const view = this.read();
        this.label('Title', view?.title ?? '物品已变化');
        this.label('Message', view ? '点击背景返回' : '请返回列表重新选择');
        const description = detailLines(view?.lines ?? ['当前场景或原物品已变化']);
        const options = view?.options ?? [];
        const key = JSON.stringify([view?.optionsFirst, description, options.map(option => [option.key, option.label, option.enabled])]);
        if (key !== this.rendered) {
            const choices = options.map(({ key, label }) => ({ key, label }));
            this.list.setItems(view?.optionsFirst ? [...choices, ...description] : [...description, ...choices], (node, row) => {
                node.getChildByName('Text')!.getComponent(Label)!.string = typeof row === 'string' ? row : (row as { label: string }).label;
            }, !!this.rendered);
            this.rendered = key;
        }
        for (const [index, name] of actionNames.entries()) {
            const action = view?.actions[index];
            this.button(name, !!action, !!action?.enabled && !this.busy, action?.label);
        }
        const quantity = view?.quantity;
        this.panel.getChildByName('Quantity')!.active = !!quantity;
        if (quantity) this.label('Quantity', `数量 ${quantity.value}`);
        this.button('Minus', !!quantity, !!quantity?.minus && !this.busy);
        this.button('Plus', !!quantity, !!quantity?.plus && !this.busy);
    };

    // 只有当前详情明确提供操作的条目才可点击，旧只读详情仍使用同一份保存列表。
    private chooseOption = (index: number, row: unknown): void => {
        if (this.busy || this.popup.top !== this.panel) return;
        const view = this.read?.();
        const option = view?.options?.[view.optionsFirst ? index : index - detailLines(view.lines).length];
        if (!option?.enabled || option.key !== (row as { key?: string } | undefined)?.key) return;
        option.choose();
        this.refresh();
    };

    private async activate(index: number): Promise<void> {
        const read = this.read;
        if (!read || this.busy || this.popup.top !== this.panel) return;
        const action = read()?.actions[index];
        if (!action?.enabled) return;
        this.busy = true;
        this.refresh();
        try { await action.run(); }
        catch (error) { if (this.read === read) this.report(error); }
        finally {
            if (this.read === read) { this.busy = false; this.refresh(); }
        }
    }

    private adjust(direction: 'minus' | 'plus'): void {
        if (this.busy || this.popup.top !== this.panel) return;
        this.read?.()?.quantity?.[direction]?.();
        this.refresh();
    }

    private bind(name: string, run: () => void): void {
        const node = this.panel.getChildByName(name)!;
        node.on(Button.EventType.CLICK, run, this);
        this.handlers.push({ node, run });
    }

    private button(name: string, visible: boolean, enabled: boolean, label?: string): void {
        const node = this.panel.getChildByName(name)!;
        node.active = visible;
        node.getComponent(Button)!.interactable = enabled;
        if (label !== undefined) this.label(`${name}/Label`, label);
    }

    private label(path: string, text: string): void { this.panel.getChildByPath(path)!.getComponent(Label)!.string = text; }

    dispose(): void {
        this.read = undefined;
        if (isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, this.chooseOption, this);
        if (isValid(this.popup.node, true)) this.popup.node.off(PopupView.EVENT_CHANGED, this.refresh, this);
        for (const { node, run } of this.handlers) if (isValid(node, true)) node.off(Button.EventType.CLICK, run, this);
        this.handlers.length = 0;
    }
}
