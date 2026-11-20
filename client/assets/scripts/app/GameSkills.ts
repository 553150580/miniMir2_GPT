import { Button, isValid, Label, Node, Toggle } from 'cc';
import { List } from '../ui/List';
import { PopupView } from '../ui/PopupView';
import { isDirectionalSkill, isGroundSkill, isSummonRecall, needsCastingSupply, SkillRow, SkillSnapshot, skillActionLabel, skillActivation, skillDescriptionLines, skillMode, skillModeStatus, skillPassiveText, skillRows } from './SkillModel';

// 仅绑定保存的技能面板与条目；布局、按钮和模板均由Creator资产提供。
export class GameSkills {
    private readonly panel: Node;
    private readonly list: List;
    private selectedId = 0;
    private rows: SkillRow[] = [];
    private input: SkillSnapshot | null = null;
    private key = '';
    private quickSkillId = 0;
    private boundQuickSkillId = 0;

    constructor(safe: Node, private readonly popup: PopupView, private readonly showDetails?: (title: string, lines: readonly string[]) => void) {
        const panel = safe.getChildByPath('UIMgr/Panels/Skills');
        const list = panel?.getChildByName('List')?.getComponent(List);
        if (!panel || !list) throw new Error('技能面板尚未保存为Creator资产');
        this.panel = panel; this.list = list;
        list.node.on(List.EVENT_ITEM_CLICK, this.onSelected, this);
    }

    get selected(): SkillRow | null { return this.rows.find(row => row.definition.id === this.selectedId) ?? null; }

    openDetails(): void {
        const row = this.selected, input = this.input;
        if (!row || !input) return;
        const lines = skillDescriptionLines(row, input);
        if (this.showDetails) this.showDetails(row.definition.name, lines);
        else this.popup.show('Details', { title: row.definition.name, message: lines.join('\n') });
    }

    refresh(input: SkillSnapshot, quickSkillId = this.quickSkillId, boundQuickSkillId = this.boundQuickSkillId): void {
        this.quickSkillId = quickSkillId;
        this.boundQuickSkillId = boundQuickSkillId;
        this.input = input; this.rows = skillRows(input);
        if (!this.rows.some(row => row.definition.id === this.selectedId)) this.selectedId = this.rows[0]?.definition.id ?? 0;
        const current = this.selected, toggleNode = this.saved('ModeToggle'), toggle = toggleNode.getComponent(Toggle)!;
        const usesToggle = !!current && skillActivation(current.definition) === 'toggle';
        toggleNode.active = usesToggle; this.saved('Use').active = !usesToggle;
        const mode = current ? skillMode(input, current.definition.id) : undefined;
        const pending = input.modePending?.skillId === current?.definition.id ? input.modePending : undefined;
        // 勾选只显示已确认状态，回执期间用文案和禁用态反馈本次点击。
        toggle.setIsCheckedWithoutNotify(!!mode?.enabled);
        toggle.interactable = !!(input.ready && current?.learned && usesToggle && mode && !input.modePending);
        this.text('ModeToggle/Label', pending ? pending.enabled ? '正在开启' : '正在关闭' : mode ? mode.enabled ? '已开启' : '未开启' : '同步中');
        const key = JSON.stringify({ ready: input.ready, castingSupplyEnabled: input.castingSupplyEnabled, mapNoPets: input.mapNoPets, auto: input.autoSkillSettings, pending: input.modePending, rows: this.rows.map(row => [row.definition.id, row.status, row.cost, row.canLearn, skillActionLabel(input, row), skillModeStatus(input, row.definition)]), selected: this.selectedId, quick: quickSkillId, boundQuick: boundQuickSkillId });
        if (key === this.key) return;
        this.key = key;
        this.list.setItems(this.rows, (node, data) => {
            const row = data as SkillRow;
            const text = node.getChildByName('Text')?.getComponent(Label);
            const summary = skillActivation(row.definition) === 'passive_proc' ? skillModeStatus(input, row.definition) : this.description(row, input);
            if (text) text.string = `${row.definition.name}${row.definition.active ? '' : '（被动）'}\n${row.canLearn ? '未学 · 点下方学习' : row.status}\n${summary}`;
            const marker = node.getChildByName('Selected');
            if (marker) marker.active = row.definition.id === this.selectedId;
        }, true);
        const selected = this.selected;
        this.text('Message', selected ? `${selected.definition.name} · ${selected.canLearn ? '未学 · 点下方学习' : selected.status}\n${this.description(selected, input)}` : '本职业暂无可用技能');
        const learn = this.saved('Learn').getComponent(Button)!;
        const use = this.saved('Use').getComponent(Button)!;
        learn.interactable = !!selected?.canLearn;
        use.interactable = !!(input.ready && selected?.learned && selected.definition.active
            && (!needsCastingSupply(input, selected) || input.castingSupplyEnabled !== false)
            && !(selected.definition.kind === 'summon_pet' && input.mapNoPets)
            && !(skillActivation(selected.definition) === 'charge' && input.modePending)
            && (!['toggle', 'charge'].includes(skillActivation(selected.definition)) || skillMode(input, selected.definition.id))
            && !(skillActivation(selected.definition) === 'charge' && skillMode(input, selected.definition.id)?.ready));
        this.saved('BindQuick').getComponent(Button)!.interactable = !!(input.ready && selected?.learned && selected.definition.active);
        this.text('BindQuick/Label', selected?.definition.id === quickSkillId
            ? quickSkillId === boundQuickSkillId ? '当前手动快捷' : '默认手动快捷' : '设为手动快捷');
        this.text('Learn/Label', selected?.learned ? '已学会' : '学习');
        this.text('Use/Label', pending ? '正在充能' : selected ? skillActionLabel(input, selected) : '施放');
        this.setButton('Details', !!selected);
        this.setButton('AutoCast', input.ready && !!input.autoSkillSettings,
            input.autoSkillSettings ? input.autoSkillSettings.enabled ? '自动施法开' : '自动施法关' : '自动施法');
        this.setButton('SkillAuto', !!(input.ready && input.autoSkillSettings && selected?.learned && selected.definition.auto_enabled),
            this.automaticLabel(input, selected));
    }

    dispose(): void {
        if (isValid(this.list, true) && isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, this.onSelected, this);
        this.input = null;
    }

    private onSelected(_index: number, row: SkillRow): void {
        this.selectedId = row.definition.id;
        if (this.input) this.refresh(this.input);
    }
    private automaticLabel(input: SkillSnapshot, row: SkillRow | null): string {
        if (!row?.learned) return '学习后可设置';
        const activation = skillActivation(row.definition);
        if (activation === 'toggle' || activation === 'charge') return '随普攻';
        if (!row.definition.active) return '被动生效';
        return row.definition.auto_enabled && !input.autoSkillSettings?.disabledSkillIds.includes(row.definition.id) ? '自动使用' : '仅手动';
    }
    private description(row: SkillRow, input: SkillSnapshot): string {
        const activation = skillActivation(row.definition), mode = skillModeStatus(input, row.definition);
        if (!row.definition.active) return `${skillPassiveText(row)}${mode ? `\n${mode}` : ''}`;
        if (activation === 'toggle' && row.definition.kind === 'thrusting') return `${mode} · 普攻时前方第2格有怪才触发`;
        if (activation === 'toggle') return `${mode} · 普攻按模式出招`;
        if (activation === 'charge') return `${mode} · 充能消耗魔法 ${row.cost}`;
        if (row.definition.kind === 'summon_pet') return `魔法消耗 ${row.cost} · ${isSummonRecall(input, row) ? '召回原宠 · 不耗护符' : `新召唤 · 护符${row.definition.material_quantity ?? 0}`}`;
        const displacement=row.definition.displacement, low=displacement?.distance_base_by_level[row.level];
        const target = displacement ? `${row.definition.kind==='repulsion'?'推开':'冲撞'} ${low}–${low!+displacement.distance_random_bound-1}格`
            : row.definition.target_kind === 'friendly_self' ? '仅自身' : row.definition.target_kind === 'enemy_self' ? `自身周围${row.definition.area_radius ?? 0}格`
            : isDirectionalSkill(row.definition) ? `前方${row.definition.line_cells ?? 0}格` : `射程 ${row.definition.range}格`;
        return `魔法消耗 ${row.cost} · ${target}${row.definition.material_quantity ? ` · 护符${row.definition.material_quantity}` : ''}`;
    }
    private saved(path: string): Node {
        const node = this.panel.getChildByPath(path); if (!node) throw new Error(`技能面板缺少保存节点：${path}`); return node;
    }
    private setButton(path: string, interactable: boolean, label?: string): void {
        const node = this.panel.getChildByPath(path), button = node?.getComponent(Button);
        if (!button) return;
        button.interactable = interactable;
        if (label !== undefined) this.text(`${path}/Label`, label);
    }
    private text(path: string, value: string): void { const label = this.saved(path).getComponent(Label); if (label) label.string = value; }
}
