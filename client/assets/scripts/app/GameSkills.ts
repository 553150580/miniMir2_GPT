import { Button, isValid, Label, Node, Toggle } from 'cc';
import { List } from '../ui/List';
import { PopupView } from '../ui/PopupView';
import { isDirectionalSkill, isGroundSkill, SkillRow, SkillSnapshot, skillActionLabel, skillActivation, skillDescriptionLines, skillMode, skillModeStatus, skillPassiveText, skillRows } from './SkillModel';

// 仅绑定保存的技能面板与条目；布局、按钮和模板均由Creator资产提供。
export class GameSkills {
    private readonly panel: Node;
    private readonly list: List;
    private selectedId = 0;
    private rows: SkillRow[] = [];
    private input: SkillSnapshot | null = null;
    private key = '';
    private quickSkillId = 0;

    constructor(safe: Node, private readonly popup: PopupView, private readonly showDetails?: (title: string, lines: readonly string[]) => void) {
        const panel = safe.getChildByPath('UIMgr/Panels/Skills');
        const list = panel?.getChildByName('List')?.getComponent(List);
        if (!panel || !list) throw new Error('技能面板尚未保存为Creator资产');
        this.panel = panel; this.list = list;
        list.node.on(List.EVENT_ITEM_CLICK, this.onSelected, this);
    }

    get selected(): SkillRow | null { return this.rows.find(row => row.definition.id === this.selectedId) ?? null; }

    refresh(input: SkillSnapshot, quickSkillId = this.quickSkillId): void {
        this.quickSkillId = quickSkillId;
        this.input = input; this.rows = skillRows(input);
        if (!this.rows.some(row => row.definition.id === this.selectedId)) this.selectedId = this.rows[0]?.definition.id ?? 0;
        // 原生Toggle的点击会先改变勾选；即使数据键相同，也必须无通知恢复权威状态。
        const current = this.selected, toggleNode = this.saved('ModeToggle'), toggle = toggleNode.getComponent(Toggle)!;
        const usesToggle = !!current && skillActivation(current.definition) === 'toggle';
        toggleNode.active = usesToggle; this.saved('Use').active = !usesToggle;
        const mode = current ? skillMode(input, current.definition.id) : undefined;
        toggle.setIsCheckedWithoutNotify(!!mode?.enabled);
        toggle.interactable = !!(input.ready && current?.learned && usesToggle && mode);
        this.text('ModeToggle/Label', mode ? mode.enabled ? '已开启' : '未开启' : '同步中');
        const key = JSON.stringify({ ready: input.ready, castingSupplyEnabled: input.castingSupplyEnabled, rows: this.rows.map(row => [row.definition.id, row.status, row.cost, row.canLearn, skillModeStatus(input, row.definition)]), selected: this.selectedId, quick: quickSkillId });
        if (key === this.key) return;
        this.key = key;
        this.list.setItems(this.rows, (node, data) => {
            const row = data as SkillRow;
            const text = node.getChildByName('Text')?.getComponent(Label);
            const summary = skillActivation(row.definition) === 'passive_proc' ? skillModeStatus(input, row.definition) : this.description(row, input);
            if (text) text.string = `${row.definition.name}${row.definition.active ? '' : '（被动）'}\n${row.status}\n${summary}`;
            const marker = node.getChildByName('Selected');
            if (marker) marker.active = row.definition.id === this.selectedId;
        }, true);
        const selected = this.selected;
        this.text('Message', selected ? `${selected.definition.name} · ${selected.status}\n${this.description(selected, input)}` : '本职业暂无可用技能');
        const learn = this.saved('Learn').getComponent(Button)!;
        const use = this.saved('Use').getComponent(Button)!;
        learn.interactable = !!selected?.canLearn;
        use.interactable = !!(input.ready && selected?.learned && selected.definition.active
            && (!selected.definition.material_quantity || input.castingSupplyEnabled !== false)
            && (!['toggle', 'charge'].includes(skillActivation(selected.definition)) || skillMode(input, selected.definition.id))
            && !(skillActivation(selected.definition) === 'charge' && skillMode(input, selected.definition.id)?.ready));
        this.saved('BindQuick').getComponent(Button)!.interactable = !!(selected?.learned && selected.definition.active);
        this.text('BindQuick/Label', selected?.definition.id === quickSkillId ? '当前快捷' : '设为快捷');
        this.text('Learn/Label', selected?.learned ? '已学会' : '学习');
        this.text('Use/Label', selected ? skillActionLabel(input, selected) : '施放');
    }

    dispose(): void {
        if (isValid(this.list, true) && isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, this.onSelected, this);
        this.input = null;
    }

    private onSelected(_index: number, row: SkillRow): void {
        this.selectedId = row.definition.id;
        if (this.input) this.refresh(this.input);
        const lines = skillDescriptionLines(row, this.input!);
        if (this.showDetails) this.showDetails(row.definition.name, lines);
        else this.popup.show('Details', { title: row.definition.name, message: lines.join('\n') });
    }
    private description(row: SkillRow, input: SkillSnapshot): string {
        const activation = skillActivation(row.definition), mode = skillModeStatus(input, row.definition);
        if (!row.definition.active) return `${skillPassiveText(row)}${mode ? `\n${mode}` : ''}`;
        if (activation === 'toggle') return `${mode} · 普攻按模式出招`;
        if (activation === 'charge') return `${mode} · 充能消耗魔法 ${row.cost}`;
        const target = row.definition.target_kind === 'friendly_self' ? '仅自身' : row.definition.target_kind === 'enemy_self' ? `自身周围${row.definition.area_radius ?? 0}格`
            : isDirectionalSkill(row.definition) ? `前方${row.definition.line_cells ?? 0}格` : `射程 ${row.definition.range}格`;
        return `魔法消耗 ${row.cost} · ${target}${row.definition.material_quantity ? ` · 护符${row.definition.material_quantity}` : ''}`;
    }
    private saved(path: string): Node {
        const node = this.panel.getChildByPath(path); if (!node) throw new Error(`技能面板缺少保存节点：${path}`); return node;
    }
    private text(path: string, value: string): void { const label = this.saved(path).getComponent(Label); if (label) label.string = value; }
}
