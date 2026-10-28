import { Button, game, instantiate, isValid, Label, Node, Prefab } from 'cc';
import { List } from '../ui/List';
import { PopupView } from '../ui/PopupView';
import { UIMgr } from '../ui/UIMgr';
import { combatAttributeLines, CombatEntity, detailLines, StatusDefinition, statusLines, statusName, statusSeconds, statusShortName } from './CombatStatusModel';

// 状态条、普通战斗面板和覆盖详情均绑定保存资产，不新建固定UI。
export class GameCombatStatus {
    private readonly bar: Node;
    private readonly overview: List;
    private readonly details: List;
    private readonly badges = new Map<number, { node: Node; handler: () => void }>();
    private entities: readonly CombatEntity[] = [];
    private sequence = '';
    private receivedAt = 0;
    private lastSecond = '';
    private ready = false;
    private detail: { handle: number; life: string; id: number } | null = null;
    constructor(safe: Node, private readonly ui: UIMgr, private readonly popup: PopupView, private readonly template: Prefab, private readonly playerHandle: number, private readonly definitions: readonly StatusDefinition[] = []) {
        const bar = safe.getChildByPath('MapViewport/BuffBar'), overview = safe.getChildByPath('UIMgr/Panels/CombatStatus/List')?.getComponent(List),
            details = safe.getChildByPath('PopupView/Panels/StatusDetails/List')?.getComponent(List);
        if (!bar || !overview || !details) throw new Error('状态条和滚动状态面板尚未保存');
        this.bar = bar; this.overview = overview; this.details = details;
    }
    refresh(baseline: { sceneEpoch?: { toString(): string }; stateSequence: { toString(): string }; entities: readonly CombatEntity[] } | null, ready: boolean): void {
        this.ready = ready;
        const key = baseline ? `${baseline.sceneEpoch ?? ''}:${baseline.stateSequence}` : '';
        if (baseline && this.sequence !== key) {
            this.sequence = key; this.receivedAt = game.totalTime; this.entities = baseline.entities; this.lastSecond = '';
        }
        this.update();
    }
    update(): void {
        const player = this.entities.find(value => value.entityHandle === this.playerHandle), states = player?.statuses ?? [];
        const now = game.totalTime, key = JSON.stringify([this.ready, this.sequence, states.map(row => [row.statusId, statusSeconds(row, this.receivedAt, now)])]);
        if (this.lastSecond === key) return; this.lastSecond = key;
        const retained = new Set<number>();
        for (const status of states) {
            retained.add(status.statusId);
            let badge = this.badges.get(status.statusId);
            if (!badge) {
                const node = instantiate(this.template); node.parent = this.bar;
                const handler = (): void => { const current = this.entities.find(value => value.entityHandle === this.playerHandle); if (this.ready && current) this.showStatus(current.entityHandle, current.life.toString(), status.statusId); };
                node.on(Button.EventType.CLICK, handler, this); badge = { node, handler }; this.badges.set(status.statusId, badge);
            }
            badge.node.getComponent(Button)!.interactable = this.ready;
            badge.node.getChildByName('Name')!.getComponent(Label)!.string = statusShortName(status.statusId, this.definitions);
            badge.node.getChildByName('Time')!.getComponent(Label)!.string = `${statusSeconds(status, this.receivedAt, now)}秒`;
        }
        for (const [id, badge] of this.badges) if (!retained.has(id)) { badge.node.off(Button.EventType.CLICK, badge.handler, this); badge.node.destroy(); this.badges.delete(id); }
        const lines = [...combatAttributeLines(player?.combatAttributes), '当前护佑状态'];
        for (const entity of this.entities.filter(row => row.entityHandle === this.playerHandle || row.entityKind === 2)) {
            const label = entity.entityHandle === this.playerHandle ? '角色' : '宠物';
            if (!entity.statuses?.length) lines.push(`${label}：无护佑状态`);
            for (const status of entity.statuses ?? []) lines.push(`${label} · ${statusName(status.statusId, this.definitions)}`, ...statusLines(status, this.receivedAt, now, this.definitions).slice(1));
        }
        this.bind(this.overview, this.ready ? lines : ['连接未就绪，以下为上次确认状态', ...lines], true);
        if (this.detail && this.popup.top?.name === 'StatusDetails') {
            const entity = this.entities.find(row => row.entityHandle === this.detail!.handle && row.life.toString() === this.detail!.life);
            const state = entity?.statuses?.find(row => row.statusId === this.detail!.id);
            this.bind(this.details, state ? statusLines(state, this.receivedAt, now, this.definitions) : ['该状态已结束或实体已离场'], true);
        }
    }
    openOverview(): void { this.ui.open('CombatStatus'); this.lastSecond = ''; this.update(); }
    showDetails(title: string, lines: readonly string[]): void {
        this.detail = null; this.bind(this.details, lines, false);
        this.popup.show('StatusDetails', { title, message: '上下滑动查看，点击半透明背景返回' });
    }
    private showStatus(handle: number, life: string, id: number): void {
        this.showDetails(statusName(id, this.definitions), []); this.detail = { handle, life, id }; this.lastSecond = ''; this.update();
    }
    private bind(list: List, lines: readonly string[], preserve: boolean): void {
        list.setItems(detailLines(lines), (node, text) => { node.getChildByName('Text')!.getComponent(Label)!.string = String(text); }, preserve);
    }
    dispose(): void { for (const badge of this.badges.values()) if (isValid(badge.node, true)) { badge.node.off(Button.EventType.CLICK, badge.handler, this); badge.node.destroy(); } this.badges.clear(); }
}
