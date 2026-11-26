import { Button, isValid, Label, Node, Toggle } from 'cc';
import { List } from '../ui/List';
import { PopupView } from '../ui/PopupView';
import { UIMgr } from '../ui/UIMgr';
import { attackSkillSlotIDs, canQueueSkill, canSetAutomaticSkill, isDirectionalSkill, isGroundSkill, isSummonRecall, SkillRow, SkillSnapshot, skillActionLabel, skillActivation, skillDescriptionLines, skillMode, skillModeStatus, skillPassiveText, skillRows, skillUseBlock, usesAttackSlot } from './SkillModel';

// 仅绑定保存的技能面板与条目；布局、按钮和模板均由Creator资产提供。
export class GameSkills {
    private readonly panel: Node;
    private readonly list: List;
    private readonly ui: Node;
    private selectedId = 0;
    private rows: SkillRow[] = [];
    private input: SkillSnapshot | null = null;
    private key = '';
    private quickSkillId = 0;
    private boundQuickSkillId = 0;
    // 默认展示的技能只用于预览。只有本次打开后手动点选的技能才会直接入槽。
    private choseSkill = false;
    // 先点槽位时仅记住编辑目标，服务端原确认槽位仍保留。
    private slotTarget: number | null = null;
    private slotListOffset: number | null = null; // 待选时回到首屏，结束后恢复浏览位置。
    private slotSaving = false;
    private slotMessage = '';

    constructor(safe: Node, private readonly popup: PopupView, private readonly saveSlot: (index: number) => Promise<void>,
        private readonly showDetails?: (title: string, lines: readonly string[]) => void) {
        const panel = safe.getChildByPath('UIMgr/Panels/Skills');
        const list = panel?.getChildByName('List')?.getComponent(List);
        const ui = safe.getChildByName('UIMgr');
        if (!panel || !list || !ui) throw new Error('技能面板尚未保存为Creator资产');
        this.panel = panel; this.list = list;
        this.ui = ui;
        list.node.on(List.EVENT_ITEM_CLICK, this.onSelected, this);
        ui.on(UIMgr.EVENT_CHANGED, this.onPanelChanged, this);
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
        if (this.input && (this.input.roleIdentity !== input.roleIdentity || this.input.sceneEpoch.toString() !== input.sceneEpoch.toString()
            || this.input.ready && !input.ready)) this.resetSlotChoice();
        this.input = input; this.rows = skillRows(input);
        if (!this.rows.some(row => row.definition.id === this.selectedId)) this.selectedId = this.rows[0]?.definition.id ?? 0;
        this.renderAttackSlots(input);
        const current = this.selected, toggleNode = this.saved('ModeToggle'), toggle = toggleNode.getComponent(Toggle)!;
        const usesToggle = !!current && skillActivation(current.definition) === 'toggle';
        toggleNode.active = usesToggle; this.saved('Use').active = !usesToggle;
        const mode = current ? skillMode(input, current.definition.id) : undefined;
        const pending = input.modePending?.skillId === current?.definition.id ? input.modePending : undefined;
        // 勾选只显示已确认状态，回执期间用文案和禁用态反馈本次点击。
        toggle.setIsCheckedWithoutNotify(!!mode?.enabled);
        toggle.interactable = !!(input.ready && current?.learned && usesToggle && mode && !input.modePending && !skillUseBlock(input, current));
        this.text('ModeToggle/Label', pending ? pending.enabled ? '正在开启' : '正在关闭' : mode ? mode.enabled ? '已开启' : '未开启' : '同步中');
        const choosing = this.slotTarget;
        const key = JSON.stringify({ ready: input.ready, scene: input.sceneKind, castingSupplyEnabled: input.castingSupplyEnabled, mapNoPets: input.mapNoPets, auto: input.autoSkillSettings, slots: input.inventory?.skillSlotIds, pending: input.modePending, rows: this.rows.map(row => [row.definition.id, row.status, row.cost, row.canLearn, skillActionLabel(input, row), skillModeStatus(input, row.definition)]), selected: this.selectedId, quick: quickSkillId, boundQuick: boundQuickSkillId, choosing });
        if (key === this.key) return;
        this.key = key;
        // 槽位待选时，已学且可入槽的技能优先显示。其余技能保留，方便查看缺书、等级与技能详情。
        const displayRows = choosing === null ? this.rows : this.rows.slice().sort((a, b) => Number(canQueueSkill(b)) - Number(canQueueSkill(a)));
        const assigned = choosing === null ? 0 : attackSkillSlotIDs(input.inventory?.skillSlotIds)[choosing];
        this.list.setItems(displayRows, (node, data) => {
            const row = data as SkillRow;
            const text = node.getChildByName('Text')?.getComponent(Label);
            const summary = choosing !== null ? this.slotCandidateText(row, choosing, assigned)
                : skillActivation(row.definition) === 'passive_proc' ? skillModeStatus(input, row.definition) : this.description(row, input);
            if (text) text.string = `${row.definition.name}${row.definition.active ? '' : '（被动）'}\n${row.canLearn ? '未学 · 点下方学习' : row.status}\n${summary}`;
            const marker = node.getChildByName('Selected');
            if (marker) marker.active = row.definition.id === this.selectedId;
        }, true);
        if (choosing === null && this.slotListOffset !== null) {
            this.list.restoreScrollOffset(this.slotListOffset);
            this.slotListOffset = null;
        }
        const selected = this.selected;
        this.text('Message', selected ? `${selected.definition.name} · ${selected.canLearn ? '未学 · 点下方学习' : selected.status}\n${this.description(selected, input)}` : '本职业暂无可用技能');
        const learn = this.saved('Learn').getComponent(Button)!;
        const use = this.saved('Use').getComponent(Button)!;
        learn.interactable = !!selected?.canLearn;
        use.interactable = !!(input.ready && selected?.learned && selected.definition.active && !skillUseBlock(input, selected)
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
        this.setButton('SkillAuto', !!(input.ready && input.autoSkillSettings && canSetAutomaticSkill(input, selected)),
            this.automaticLabel(input, selected));
    }

    // 用已保存按钮显示待选状态和权威槽位，不在回执前展示新技能。
    private renderAttackSlots(input: SkillSnapshot): void {
        const slots = attackSkillSlotIDs(input.inventory?.skillSlotIds);
        const selected = this.selected, available = !!input.canConfigureSlots && !this.slotSaving;
        for (const [index, id] of slots.entries()) {
            const button = this.panel.getChildByName(`AttackSlot${index + 1}`);
            const definition = input.definitions.find(skill => skill.id === id);
            const label = button?.getChildByName('Label')?.getComponent(Label);
            const control = button?.getComponent(Button);
            if (label) label.string = `${index + 1} · ${index === this.slotTarget ? '待选' : definition?.name ?? '空位'}`;
            if (control) control.interactable = available;
        }
        this.setButton('ClearSlots', available && slots.some(id => id !== 0));
        const activeSlot = selected ? slots.indexOf(selected.definition.id) + 1 : 0;
        let hint = '轻触槽位或选择已学技能';
        if (!input.ready || !input.inventory) hint = '技能槽正在同步';
        else if (input.sceneKind !== 1 && input.sceneKind !== 2) hint = '回城镇或普通野外可调整技能槽';
        else if (this.slotSaving) hint = '正在保存技能槽';
        else if (!input.canConfigureSlots) hint = '正在核对操作，请稍候';
        else if (this.slotMessage) hint = this.slotMessage;
        else if (this.slotTarget !== null) hint = this.rows.some(canQueueSkill)
            ? `第 ${this.slotTarget + 1} 槽待选，请点已学技能` : '暂无已学可入槽技能';
        else if (this.choseSkill && selected) {
            if (!selected.learned) hint = '学习此技能后，再设置技能槽';
            else if (!canQueueSkill(selected)) hint = '此技能不能放入攻击槽';
            else if (activeSlot) hint = `已在第 ${activeSlot} 槽，点同槽移除`;
            else hint = '轻触槽位保存所选技能';
        }
        this.text('Hint', hint);
    }

    // 先选技能时直接保存。先点槽位时等待下一次有效技能点击，再提交同一权威操作。
    chooseAttackSlot(index: number): void | Promise<void> {
        const input = this.input;
        if (!input?.canConfigureSlots || this.slotSaving) return;
        this.slotMessage = '';
        if (this.slotTarget === null && this.choseSkill && canQueueSkill(this.selected)) return this.saveChosenSlot(index);
        if (this.slotTarget === null) this.slotListOffset = this.list.scrollView?.getScrollOffset().y ?? 0;
        this.slotTarget = this.slotTarget === index ? null : index;
        this.choseSkill = false;
        this.refresh(input);
        if (this.slotTarget !== null) this.list.restoreScrollOffset(0);
    }

    dispose(): void {
        if (isValid(this.list, true) && isValid(this.list.node, true)) this.list.node.off(List.EVENT_ITEM_CLICK, this.onSelected, this);
        if (isValid(this.ui, true)) this.ui.off(UIMgr.EVENT_CHANGED, this.onPanelChanged, this);
        this.input = null;
    }

    private onSelected(_index: number, row: SkillRow): void {
        this.selectedId = row.definition.id;
        if (!this.input) return;
        const target = this.slotTarget;
        this.choseSkill = target === null;
        this.slotMessage = '';
        this.refresh(this.input);
        if (target !== null && this.input.canConfigureSlots && canQueueSkill(this.selected) && !this.slotSaving) void this.saveChosenSlot(target);
    }

    private onPanelChanged(top: Node | null): void {
        if (top === this.panel) return;
        this.resetSlotChoice();
        if (this.input) this.refresh(this.input);
    }

    private resetSlotChoice(): void {
        this.choseSkill = false;
        this.slotTarget = null;
        this.slotMessage = '';
    }

    private async saveChosenSlot(index: number): Promise<void> {
        this.slotSaving = true;
        this.slotTarget = null;
        if (this.input) this.refresh(this.input);
        try {
            await this.saveSlot(index);
            if (!this.choseSkill && this.panel.activeInHierarchy) this.slotMessage = `第 ${index + 1} 槽已保存`;
        } catch (error) {
            if (this.panel.activeInHierarchy) this.slotMessage = error instanceof Error ? error.message : '技能槽保存失败，请重试';
        } finally {
            this.slotSaving = false;
            if (this.input) this.renderAttackSlots(this.input);
        }
    }

    private slotCandidateText(row: SkillRow, index: number, assigned: number): string {
        if (!usesAttackSlot(row.definition)) return '此技能不能入攻击槽';
        if (!row.learned) return '学习后可入槽';
        return row.definition.id === assigned ? '已在此槽 · 点选移除' : `点选放入第 ${index + 1} 槽`;
    }
    private automaticLabel(input: SkillSnapshot, row: SkillRow | null): string {
        if (!row?.learned) return '学习后可设置';
        const activation = skillActivation(row.definition);
        if (activation === 'toggle' || activation === 'charge') return '随普攻';
        if (!row.definition.active) return '被动生效';
        if (row.definition.kind === 'summon_pet' && canSetAutomaticSkill(input, row)) {
            return input.autoSkillSettings?.disabledSkillIds.includes(row.definition.id) ? '槽位自动关' : '槽位自动开';
        }
        return row.definition.auto_enabled && !input.autoSkillSettings?.disabledSkillIds.includes(row.definition.id) ? '自动使用' : '仅手动';
    }
    private description(row: SkillRow, input: SkillSnapshot): string {
        const activation = skillActivation(row.definition), mode = skillModeStatus(input, row.definition);
        if (!row.definition.active) return `${skillPassiveText(row)}${mode ? `\n${mode}` : ''}`;
        const block = skillUseBlock(input, row);
        if (block) return block.reason;
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
