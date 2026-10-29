import { Button, game, instantiate, isValid, Label, Node, Prefab, Sprite } from 'cc';
import { List } from '../ui/List';
import { PopupView } from '../ui/PopupView';
import { UIMgr } from '../ui/UIMgr';
import { combatAttributeLines, CombatEntity, detailLines, StatusDefinition, statusLines, statusName, statusSeconds, statusShortName } from './CombatStatusModel';
import { isLivingPet, petTrainingLines } from './PetTrainingModel';
import { OwnedPetState, ownedPetLines, parseOwnedPet } from './OwnedPetModel';

// 状态条、普通战斗面板和覆盖详情均绑定保存资产，不新建固定UI。
export class GameCombatStatus {
    private readonly bar: Node;
    private readonly overview: List;
    private readonly details: List;
    private readonly badges = new Map<number, { node: Node; handler: () => void }>();
    private entities: readonly CombatEntity[] = [];
    private ownedPet: OwnedPetState | null = null;
    private sequence = '';
    private receivedAt = 0;
    private lastSecond = '';
    private ready = false;
    private scope = '';
    private disposed = false;
    private detail: { kind: 'status' | 'entity' | 'pet'; scope: string; handle?: number; life?: string; id?: number } | null = null;
    constructor(safe: Node, private readonly ui: UIMgr, private readonly popup: PopupView, private readonly template: Prefab, private readonly playerHandle: number, private readonly definitions: readonly StatusDefinition[] = [], private readonly expectedScope?: { sceneEpoch: { toString(): string }; baselineId: { toString(): string }; mapId?: { toString(): string } }) {
        const bar = safe.getChildByPath('MapViewport/BuffBar'), overview = safe.getChildByPath('UIMgr/Panels/CombatStatus/List')?.getComponent(List),
            details = safe.getChildByPath('PopupView/Panels/StatusDetails/List')?.getComponent(List);
        if (!bar || !overview || !details) throw new Error('状态条和滚动状态面板尚未保存');
        this.bar = bar; this.overview = overview; this.details = details;
    }
    refresh(baseline: { sceneEpoch?: { toString(): string }; baselineId?: { toString(): string }; stateSequence: { toString(): string }; entities: readonly CombatEntity[]; ownedPet?: OwnedPetState | null } | null, ready: boolean): void {
        if (this.disposed) return;
        const scope = baseline ? `${baseline.sceneEpoch ?? ''}:${baseline.baselineId ?? ''}` : '';
        if (baseline && this.expectedScope && scope !== `${this.expectedScope.sceneEpoch}:${this.expectedScope.baselineId}`) return;
        if (baseline && this.scope === scope && this.sequence && BigInt(baseline.stateSequence.toString()) < BigInt(this.sequence.slice(this.sequence.lastIndexOf(':') + 1))) return;
        this.ready = ready;
        const key = baseline ? `${scope}:${baseline.stateSequence}` : '';
        if (baseline && this.sequence !== key) {
            this.sequence = key; this.scope = scope; this.receivedAt = game.totalTime; this.entities = baseline.entities; this.ownedPet = parseOwnedPet(baseline.ownedPet); this.lastSecond = '';
        }
        this.update();
    }
    update(): void {
        if (this.disposed) return;
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
        for (const [id, badge] of Array.from(this.badges)) if (!retained.has(id)) { this.removeBadge(badge); this.badges.delete(id); }
        const lines = [...combatAttributeLines(player?.combatAttributes), '当前护佑状态'];
        for (const entity of this.entities.filter(row => row.entityHandle === this.playerHandle || isLivingPet(row))) {
            const label = entity.entityHandle === this.playerHandle ? '角色' : '宠物';
            if (!entity.statuses?.length) lines.push(`${label}：无护佑状态`);
            for (const status of entity.statuses ?? []) lines.push(`${label} · ${statusName(status.statusId, this.definitions)}`, ...statusLines(status, this.receivedAt, now, this.definitions).slice(1));
        }
        const corpses = this.entities.filter(row => row.isPetCorpse).length;
        if (corpses) lines.push(`宠物尸体 ${corpses}具 · 点击尸体查看详情`);
        this.bind(this.overview, this.ready ? lines : ['连接未就绪，以下为上次确认状态', ...lines], true);
        if (this.detail && this.popup.top?.name === 'StatusDetails') {
            const entity = this.detail.scope === this.scope ? this.detail.kind === 'pet' ? undefined
                : this.entities.find(row => row.entityHandle === this.detail!.handle && row.life.toString() === this.detail!.life) : undefined;
            let current: string[];
            if (this.detail.kind === 'pet') {
                current = this.detail.scope === this.scope ? this.petLines(now) : ['当前没有存活宠物'];
            } else if (this.detail.kind === 'status') {
                const state = entity?.statuses?.find(row => row.statusId === this.detail!.id);
                current = state ? statusLines(state, this.receivedAt, now, this.definitions) : ['该状态已结束或实体已离场'];
            } else current = entity ? this.entityLines(entity, now) : ['该实体已离场'];
            this.bind(this.details, this.ready ? current : ['连接未就绪，以下为上次确认状态', ...current], true);
        }
    }
    openOverview(): void { if (this.disposed) return; this.ui.open('CombatStatus'); this.lastSecond = ''; this.update(); }
    showDetails(title: string, lines: readonly string[]): void {
        if (this.disposed) return;
        this.detail = null; this.bind(this.details, lines, false);
        this.popup.show('StatusDetails', { title, message: '上下滑动查看，点击半透明背景返回' });
    }
    showEntity(handle: number, life: string): void {
        if (this.disposed) return;
        this.showDetails('实体详情', []); this.detail = { kind: 'entity', scope: this.scope, handle, life }; this.lastSecond = ''; this.update();
    }
    showPet(): void {
        if (this.disposed) return;
        this.showDetails('宠物', []); this.detail = { kind: 'pet', scope: this.scope }; this.lastSecond = ''; this.update();
    }
    private petLines(now: number): string[] {
        const pet = this.ownedPet, lines = ownedPetLines(pet, this.expectedScope?.mapId);
        if (!pet || this.expectedScope?.mapId && this.expectedScope.mapId.toString() !== pet.mapId.toString()) return lines;
        const entity = this.entities.find(row => row.entityHandle === pet.entityHandle && row.life.toString() === pet.life.toString() && isLivingPet(row));
        if (!entity) return lines;
        if (entity.position) lines.push(`位置 ${entity.position.x}, ${entity.position.y}`);
        lines.push(...combatAttributeLines(entity.combatAttributes));
        if (entity.moveIntervalMs) lines.push(`移动间隔 ${entity.moveIntervalMs}毫秒`);
        if (entity.attackIntervalMs) lines.push(`攻击间隔 ${entity.attackIntervalMs}毫秒`);
        for (const status of entity.statuses ?? []) lines.push(...statusLines(status, this.receivedAt, now, this.definitions));
        return lines;
    }
    private entityLines(entity: CombatEntity, now: number): string[] {
        const lines = [entity.isPetCorpse ? '宠物尸体' : ({ 1: '角色', 2: '宠物', 3: '怪物', 4: '掉落' } as Record<number, string>)[entity.entityKind] ?? '实体'];
        if (entity.position) lines.push(`位置 ${entity.position.x}, ${entity.position.y}`);
        lines.push(`生命 ${entity.hp} / ${entity.hpMax ?? entity.hp}`, ...combatAttributeLines(entity.combatAttributes));
        if (entity.entityKind === 2) {
            const training = petTrainingLines(entity);
            lines.push(...(training.length ? training : ['未提供训练数据']));
        }
        if (entity.moveIntervalMs) lines.push(`移动间隔 ${entity.moveIntervalMs}毫秒`);
        if (entity.attackIntervalMs) lines.push(`攻击间隔 ${entity.attackIntervalMs}毫秒`);
        for (const status of entity.statuses ?? []) lines.push(...statusLines(status, this.receivedAt, now, this.definitions));
        return lines;
    }
    private showStatus(handle: number, life: string, id: number): void {
        this.showDetails(statusName(id, this.definitions), []); this.detail = { kind: 'status', scope: this.scope, handle, life, id }; this.lastSecond = ''; this.update();
    }
    private bind(list: List, lines: readonly string[], preserve: boolean): void {
        list.setItems(detailLines(lines), (node, text) => { node.getChildByName('Text')!.getComponent(Label)!.string = String(text); }, preserve);
    }
    private removeBadge(badge: { node: Node; handler: () => void }): void {
        if (!isValid(badge.node, true)) return;
        for (const sprite of badge.node.getComponentsInChildren(Sprite)) { sprite.spriteFrame = null; sprite.customMaterial = null; }
        badge.node.off(Button.EventType.CLICK, badge.handler, this); badge.node.active = false; badge.node.parent = null; badge.node.destroy();
    }
    dispose(): void { this.disposed = true; this.entities = []; this.ownedPet = null; this.detail = null; for (const badge of Array.from(this.badges.values())) this.removeBadge(badge); this.badges.clear(); }
}
