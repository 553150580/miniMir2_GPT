import { Button, isValid, Label, Node } from 'cc';
import { List } from '../ui/List';
import { PopupView } from '../ui/PopupView';
import { SkillRow, SkillSnapshot, skillAccuracy, skillRows } from './SkillModel';

// 仅绑定保存的技能面板与条目；布局、按钮和模板均由Creator资产提供。
export class GameSkills {
    private readonly panel: Node;
    private readonly list: List;
    private selectedId = 0;
    private rows: SkillRow[] = [];
    private input: SkillSnapshot | null = null;
    private key = '';

    constructor(safe: Node, private readonly popup: PopupView) {
        const panel = safe.getChildByPath('UIMgr/Panels/Skills');
        const list = panel?.getChildByName('List')?.getComponent(List);
        if (!panel || !list) throw new Error('技能面板尚未保存为Creator资产');
        this.panel = panel; this.list = list;
        list.node.on(List.EVENT_ITEM_CLICK, this.onSelected, this);
    }

    get selected(): SkillRow | null { return this.rows.find(row => row.definition.id === this.selectedId) ?? null; }

    refresh(input: SkillSnapshot): void {
        this.input = input; this.rows = skillRows(input);
        if (!this.rows.some(row => row.definition.id === this.selectedId)) this.selectedId = this.rows[0]?.definition.id ?? 0;
        const key = JSON.stringify({ ready: input.ready, rows: this.rows.map(row => [row.definition.id, row.status, row.cost, row.canLearn]), selected: this.selectedId });
        if (key === this.key) return;
        this.key = key;
        this.list.setItems(this.rows, (node, data) => {
            const row = data as SkillRow;
            const text = node.getChildByName('Text')?.getComponent(Label);
            if (text) text.string = `${row.definition.name}${row.definition.active ? '' : '（被动）'}\n${row.status}\n${row.definition.active ? `魔法消耗 ${row.cost} · 射程 ${row.definition.range}格` : `当前准确 +${skillAccuracy(row)}`}`;
            const marker = node.getChildByName('Selected');
            if (marker) marker.active = row.definition.id === this.selectedId;
        }, true);
        const selected = this.selected;
        this.text('Message', selected ? `${selected.definition.name} · ${selected.status}\n${selected.definition.active ? `魔法消耗 ${selected.cost}` : `当前准确 +${skillAccuracy(selected)} · 无需主动施放`}` : '本职业暂无可用技能');
        const learn = this.saved('Learn').getComponent(Button)!;
        const use = this.saved('Use').getComponent(Button)!;
        learn.interactable = !!selected?.canLearn;
        use.interactable = !!(input.ready && selected?.learned && selected.definition.active);
        this.text('Learn/Label', selected?.learned ? '已学会' : '学习');
        this.text('Use/Label', selected && !selected.definition.active ? selected.learned ? '被动生效' : '被动技能' : '施放');
    }

    dispose(): void {
        if (isValid(this.list, true) && isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, this.onSelected, this);
        this.input = null;
    }

    private onSelected(_index: number, row: SkillRow): void {
        this.selectedId = row.definition.id;
        if (this.input) this.refresh(this.input);
        this.popup.show('Details', { title: row.definition.name,
            message: `${row.status}\n${row.definition.active ? `消耗魔法 ${row.cost}\n射程 ${row.definition.range}格` : `被动技能，无需主动施放\n当前准确 +${skillAccuracy(row)}`}\n${row.definition.learn_level}级可使用对应技能书学习\n升级需选择书本与书页，先查看报价` });
    }
    private saved(path: string): Node {
        const node = this.panel.getChildByPath(path); if (!node) throw new Error(`技能面板缺少保存节点：${path}`); return node;
    }
    private text(path: string, value: string): void { const label = this.saved(path).getComponent(Label); if (label) label.string = value; }
}
