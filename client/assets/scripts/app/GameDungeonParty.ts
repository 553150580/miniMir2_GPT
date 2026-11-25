import { Button, Game, game, isValid, Label, Node, Toggle } from 'cc';
import { List } from '../ui/List';
import type { PopupView } from '../ui/PopupView';
import { CodeLineForbidden, CodeLineFull, CodeLineRecovering, CodeOperationConflict, CodeOperationPending } from '../generated/protocol_ids';
import { newOperationId, ProtocolResponseError } from './GameConnection';
import { protocolErrorMessage } from './GameErrors';
import { attachmentKey } from './MapTravelModel';
import { observeDungeonPartyState, prepareDungeonPartyAction, validateDungeonPartyAction } from './DungeonPartyModel';
import type { DungeonPartyAction, DungeonPartyConditions, DungeonPartyContext, DungeonPartyMember, DungeonPartySnapshot, DungeonPartyTemplate, DungeonPartyView } from './DungeonPartyModel';

const jobs = ['战士', '法师', '道士'] as const;
const jobToggles = ['JobWarrior', 'JobMage', 'JobTaoist'] as const;
const tabs = [['TemplatesTab', 'TemplateList', '副本'], ['RoomsTab', 'RoomList', '招募'], ['MembersTab', 'MemberList', '队伍']] as const;
const refreshIntervalMs = 3000; // 仅在面板前台时重新核对已有招募。
const actions = ['Create', 'Join', 'Leave', 'Kick', 'Update', 'Start'] as const;
type ActionButton = typeof actions[number];
type Request = ReturnType<typeof prepareDungeonPartyAction>['request'];

interface PartyPanelActions {
    context: () => DungeonPartyContext | null;
    request: (request: Request) => Promise<unknown>;
    sound: () => void;
}

// 绑定保存的组队面板。请求和确认按当前附着核对，面板被覆盖时保留滚动。
export class GameDungeonParty {
    private readonly lists: readonly List[];
    private readonly handlers: Array<{ node: Node; event: string; handler: (...args: any[]) => void }> = [];
    private scope = '';
    private snapshot: DungeonPartySnapshot | null = null;
    private tab = 0;
    private selectedTemplate = 0;
    private selectedRoom = '';
    private selectedMember = '';
    private draft: DungeonPartyConditions | null = null;
    private draftSource = '';
    private creation?: { key: string; id: Uint8Array };
    private timer?: ReturnType<typeof setTimeout>;
    private pending?: object;
    private unavailable = false;
    private hidden = false;
    private disposed = false;
    private notice = '';

    constructor(readonly panel: Node, private readonly popup: PopupView, private readonly transport: PartyPanelActions) {
        this.lists = tabs.map(([, path]) => {
            const list = this.saved(path).getComponent(List);
            if (!list) throw new Error(`讨伐列表缺少保存组件 ${path}`);
            return list;
        });
        for (const [index, [tab]] of tabs.entries()) this.button(tab, () => { this.tab = index; this.render(); });
        this.on(this.lists[0].node, List.EVENT_ITEM_CLICK, (_index, value: DungeonPartyTemplate) => {
            if (!this.canInteract() || !this.snapshot?.templates.includes(value)) return;
            this.selectedTemplate = value.templateId; this.updateDraft(); this.render();
        });
        this.on(this.lists[1].node, List.EVENT_ITEM_CLICK, (_index, value: DungeonPartyView) => {
            if (!this.canInteract() || !this.snapshot?.parties.includes(value)) return;
            this.selectedRoom = value.partyId; this.selectedMember = ''; this.render();
        });
        this.on(this.lists[2].node, List.EVENT_ITEM_CLICK, (_index, value: DungeonPartyMember) => {
            if (!this.canInteract() || !this.viewedParty()?.members.includes(value)) return;
            this.selectedMember = value.roleId; this.render();
        });
        for (const name of actions) this.button(name, () => this.chooseAction(name));
        this.button('Refresh', () => { this.notice = ''; void this.execute({ kind: 'query' }); });
        this.button('LevelDown', () => this.changeLevel(-1));
        this.button('LevelUp', () => this.changeLevel(1));
        for (const [index, path] of jobToggles.entries()) this.toggle(path, checked => {
            if (!this.draft || !this.canEdit()) { this.render(); return; }
            const bit = 1 << index;
            const mask = checked ? this.draft.allowedJobsMask | bit : this.draft.allowedJobsMask & ~bit;
            if (mask === 0) this.notice = '请至少选择一种职业';
            else { this.draft = { ...this.draft, allowedJobsMask: mask }; this.notice = ''; }
            this.render();
        });
        this.toggle('AllowSameJob', checked => {
            if (this.draft && this.canEdit()) {
                this.draft = { ...this.draft, allowSameJob: checked }; this.notice = '';
            }
            this.render();
        });
        game.on(Game.EVENT_HIDE, this.pause, this);
        game.on(Game.EVENT_SHOW, this.resume, this);
    }

    // 只在场景或可见状态变化时触发首次查询。后续周期由同一个定时器管理。
    refreshVisibility(): void {
        if (this.disposed) return;
        if (!this.panel.activeInHierarchy || this.hidden) { this.stopTimer(); return; }
        const context = this.currentContext();
        if (!context) { this.stopTimer(); return; }
        if (!this.pending && !this.timer && !this.unavailable) void this.execute({ kind: 'query' });
    }

    dispose(): void {
        this.disposed = true;
        this.stopTimer();
        game.off(Game.EVENT_HIDE, this.pause, this);
        game.off(Game.EVENT_SHOW, this.resume, this);
        for (const { node, event, handler } of this.handlers) if (isValid(node, true)) node.off(event, handler, this);
        this.handlers.length = 0;
    }

    private pause(): void { this.hidden = true; this.stopTimer(); }
    private resume(): void { this.hidden = false; this.refreshVisibility(); }

    private currentContext(): DungeonPartyContext | null {
        if (this.disposed || this.hidden || !this.panel.activeInHierarchy) return null;
        const value = this.transport.context(), context = value?.ready ? value : null;
        const key = context ? attachmentKey(context.attachment) : '';
        if (key !== this.scope) {
            this.scope = key; this.snapshot = null; this.unavailable = false; this.notice = '';
            this.selectedRoom = this.selectedMember = this.draftSource = ''; this.selectedTemplate = this.tab = 0;
            this.draft = null; this.creation = undefined; this.pending = undefined;
            this.stopTimer(); this.render();
        }
        return context;
    }

    private changeLevel(direction: number): void {
        const template = this.template();
        if (!this.draft || !template || !this.canEdit()) return;
        this.draft = { ...this.draft, minimumLevel: Math.max(template.minimumLevel, Math.min(0xffffffff, this.draft.minimumLevel + direction)) };
        this.notice = ''; this.render();
    }

    private chooseAction(button: ActionButton): void {
        if (!this.canInteract()) return;
        const own = this.snapshot?.ownParty;
        const template = this.template(), draft = this.draft;
        let action: DungeonPartyAction | undefined;
        if (button === 'Create' && this.tab === 0 && !own && template && draft) {
            const key = JSON.stringify([template.templateId, draft.minimumLevel, draft.allowedJobsMask, draft.allowSameJob]);
            if (this.creation?.key !== key) this.creation = { key, id: newOperationId() };
            action = { kind: 'create', partyId: this.creation.id, templateId: template.templateId, conditions: draft };
        } else if (button === 'Join' && this.tab === 1 && !own && this.selectedRoom) action = { kind: 'join', partyId: this.selectedRoom };
        else if (own) {
            if (button === 'Leave') action = { kind: 'leave', partyId: own.partyId };
            if (button === 'Start') action = { kind: 'start', partyId: own.partyId };
            if (button === 'Kick' && this.selectedMember) action = { kind: 'kick', partyId: own.partyId, targetRoleId: this.selectedMember };
            if (button === 'Update' && draft) action = { kind: 'update', partyId: own.partyId, conditions: draft };
        }
        if (action) { this.notice = ''; void this.execute(action); }
    }

    private async execute(action: DungeonPartyAction): Promise<void> {
        const context = this.currentContext();
        if (!context || this.pending) return;
        const token = {};
        this.pending = token;
        this.stopTimer();
        this.render();
        try {
            const prepared = prepareDungeonPartyAction(context, action, this.snapshot);
            if (!await this.confirmAction(action, context)) return;
            const current = this.currentContext();
            if (this.pending !== token || !current) return;
            if (!validateDungeonPartyAction(prepared, current, this.snapshot)) throw new Error('组队状态已变化，请刷新后重试');
            const raw = await this.transport.request(prepared.request);
            const attached = this.currentContext();
            if (this.pending !== token || !attached) return;
            const observed = observeDungeonPartyState(raw, prepared, attached);
            if (!observed) throw new Error('本次组队响应已失效，请刷新');
            this.unavailable = false;
            const hadParty = !!this.snapshot?.ownParty;
            this.snapshot = observed;
            if (observed.errorCode) this.notice = this.errorMessage(observed.errorCode);
            else {
                if (!hadParty && observed.ownParty) this.tab = 2;
                if (action.kind === 'leave') this.tab = 1;
                if (action.kind === 'update') this.notice = '招募条件已更新';
            }
            if (observed.ownParty) this.creation = undefined;
            this.updateDraft();
        } catch (error) {
            if (this.pending !== token || !this.currentContext()) return;
            if (action.kind === 'query' && error instanceof ProtocolResponseError && error.code === CodeLineForbidden) {
                this.unavailable = true; this.snapshot = null; this.notice = '';
            } else this.notice = error instanceof ProtocolResponseError ? this.errorMessage(error.code)
                : error instanceof Error ? error.message : '组队请求未完成';
        } finally {
            if (this.pending === token) {
                this.pending = undefined;
                if (this.currentContext()) { this.render(); this.schedule(); }
            }
        }
    }

    private async confirmAction(action: DungeonPartyAction, context: DungeonPartyContext): Promise<boolean> {
        const own = this.snapshot?.ownParty;
        if (action.kind === 'start' && own) return this.popup.confirm('Confirm',
            `${this.template()?.displayName ?? '讨伐链'} · ${own.members.length} 名队员\n入场时每人扣 1 次。正常失败或超时不退还次数。\n全队确认地图后开始战斗。`, '确认开战');
        if (action.kind === 'kick' && own) {
            const member = own.members.find(value => value.roleId === action.targetRoleId);
            return this.popup.confirm('Confirm', `移除队员 ${member?.displayName ?? ''}？`, '移除队员');
        }
        if (action.kind === 'leave' && own && this.isLeader(context))
            return this.popup.confirm('Confirm', '队长退出会解散本次招募。', '解散队伍');
        return true;
    }

    private updateDraft(): void {
        const own = this.snapshot?.ownParty;
        if (own) this.selectedTemplate = own.templateId;
        else if (!this.template()) this.selectedTemplate = this.snapshot?.templates[0]?.templateId ?? 0;
        if (!this.selectedRecruitment()) this.selectedRoom = '';
        if (!this.viewedParty()?.members.some(member => member.roleId === this.selectedMember)) this.selectedMember = '';
        const template = this.template();
        if (!template) { this.draftSource = ''; this.draft = null; return; }
        const baseline = own ?? { minimumLevel: template.minimumLevel, allowedJobsMask: 7, allowSameJob: false };
        const source = JSON.stringify([own?.partyId ?? '', template.templateId, baseline.minimumLevel, baseline.allowedJobsMask, baseline.allowSameJob]);
        if (source === this.draftSource) return;
        this.draftSource = source;
        this.draft = { minimumLevel: baseline.minimumLevel, allowedJobsMask: baseline.allowedJobsMask, allowSameJob: baseline.allowSameJob };
    }

    private render(): void {
        if (this.disposed || !this.panel.activeInHierarchy) return;
        const context = this.transport.context();
        const snapshot = this.snapshot, own = snapshot?.ownParty;
        const template = this.template(), room = this.selectedRecruitment(), party = own ?? (this.tab === 0 ? undefined : room);
        const draft = this.draft, shownConditions = own || this.tab === 0 ? draft : room;
        const canAct = this.canInteract(), leader = this.isLeader(context), editable = this.canEdit();
        for (const [index, [tab, list, title]] of tabs.entries()) {
            this.saved(list).active = this.tab === index;
            this.label(`${tab}/Label`, this.tab === index ? `● ${title}` : title);
        }
        this.lists[0].setItems(snapshot?.templates ?? [], (node, row) => {
            const value = row as DungeonPartyTemplate;
            this.row(node, `${value.templateId === this.selectedTemplate ? '● ' : ''}${value.displayName}\n${value.minimumLevel} 级起 · ${value.durationSeconds} 秒`, !own && canAct);
        });
        this.lists[1].setItems(snapshot?.parties ?? [], (node, row) => {
            const value = row as DungeonPartyView;
            this.row(node, `${value.partyId === this.selectedRoom ? '● ' : ''}${value.members[0].displayName} · ${value.members.length}/3\n${this.templateName(value.templateId)} · ${value.minimumLevel} 级起`, !own && canAct);
        });
        this.lists[2].setItems(party?.members ?? [], (node, row) => {
            const value = row as DungeonPartyMember;
            this.row(node, `${value.roleId === this.selectedMember ? '● ' : ''}${value.displayName}${value.roleId === party?.leaderRoleId ? ' · 队长' : ''}\n${jobs[value.job]} ${value.level} 级 · ${value.online ? '在线' : '离线'}`, canAct);
        });
        this.label('Status', !this.scope ? '等待角色与场景同步' : this.unavailable ? '本服暂未开放讨伐链'
            : snapshot?.challengeId ? '场次已确认，正在入场' : snapshot?.starting ? '正在确认整队开战'
            : own ? `原队伍 · ${own.members.length}/3 人` : snapshot ? `公开招募 ${snapshot.parties.length}${snapshot.more ? '+' : ''} 支` : '正在查询本服招募');
        this.label('Selection', party ? `${this.templateName(party.templateId)} · ${party.members[0].displayName}`
            : template?.displayName ?? '请选择副本或招募');
        this.label('MinimumLevel', `等级 ${shownConditions?.minimumLevel ?? '—'}`);
        this.label('Conditions', this.tab === 1 && room && !own ? `招募要求 ${room.minimumLevel} 级 · ${this.jobText(room)}`
            : editable ? '招募条件' : own ? this.jobText(own) : '招募条件');
        this.label('Message', this.notice || (own ? '开战前请全队回到普通野外' : snapshot && this.tab === 1 && !snapshot.parties.length ? '当前没有可加入的招募' : ''));
        this.enable('Create', canAct && this.tab === 0 && !own && !!template && (context?.level ?? 0) >= template.minimumLevel);
        this.enable('Join', canAct && this.tab === 1 && !own && !!room && !!context && context.level >= room.minimumLevel &&
            !!(room.allowedJobsMask & (1 << context.job)) && (room.allowSameJob || !room.members.some(member => member.job === context.job)));
        this.enable('Leave', canAct && !!own);
        this.enable('Kick', canAct && leader && !!own?.members.some(member => member.roleId === this.selectedMember && member.roleId !== own.leaderRoleId));
        this.enable('Update', canAct && leader && !!draft && !!own && (draft.minimumLevel !== own.minimumLevel ||
            draft.allowedJobsMask !== own.allowedJobsMask || draft.allowSameJob !== own.allowSameJob));
        this.enable('Start', canAct && leader && context?.sceneKind === 1 && !!own?.members.every(member => member.online));
        this.enable('Refresh', !!this.scope && !this.pending);
        this.enable('LevelDown', editable && !!template && !!draft && draft.minimumLevel > template.minimumLevel);
        this.enable('LevelUp', editable && !!draft && draft.minimumLevel < 0xffffffff);
        for (const [index, path] of jobToggles.entries()) this.renderToggle(path, editable, !!((shownConditions?.allowedJobsMask ?? 0) & (1 << index)));
        this.renderToggle('AllowSameJob', editable, !!shownConditions?.allowSameJob);
    }

    private template(): DungeonPartyTemplate | undefined { return this.snapshot?.templates.find(row => row.templateId === this.selectedTemplate); }
    private templateName(id: number): string { return this.snapshot?.templates.find(row => row.templateId === id)?.displayName ?? '讨伐链'; }
    private selectedRecruitment(): DungeonPartyView | undefined { return this.snapshot?.parties.find(row => row.partyId === this.selectedRoom); }
    private viewedParty(): DungeonPartyView | undefined { return this.snapshot?.ownParty ?? this.selectedRecruitment(); }
    private isLeader(context = this.transport.context()): boolean {
        const leader = this.snapshot?.ownParty?.leaderRoleId;
        return !!(leader && context?.attachment.role.endsWith(':' + leader));
    }
    private canInteract(): boolean { return !!this.snapshot && !this.pending && !this.snapshot.starting && !this.snapshot.challengeId && !this.hidden; }
    private canEdit(): boolean {
        return this.canInteract() && !!this.draft && (this.snapshot?.ownParty ? this.isLeader() : this.tab === 0);
    }
    private jobText(value: DungeonPartyConditions): string {
        return `${jobs.filter((_job, index) => value.allowedJobsMask & (1 << index)).join('／')}${value.allowSameJob ? ' · 同职业可加入' : ''}`;
    }
    private errorMessage(code: number): string {
        if (code === CodeOperationConflict) return '名单或招募条件已变化，请核对后重试';
        if (code === CodeOperationPending) return '队伍正在开战，请等待确认';
        if (code === CodeLineFull) return '所选队伍已满，请刷新招募';
        if (code === CodeLineRecovering) return '请全队返回普通野外并确认连接后重试';
        if (code === CodeLineForbidden) return '队伍条件或剩余入场次数不满足';
        return protocolErrorMessage(20021, code);
    }

    private schedule(): void {
        if (this.timer || this.unavailable || this.hidden || !this.panel.activeInHierarchy) return;
        this.timer = setTimeout(() => { this.timer = undefined; void this.execute({ kind: 'query' }); }, refreshIntervalMs);
    }
    private stopTimer(): void { if (this.timer) clearTimeout(this.timer); this.timer = undefined; }
    private saved(path: string): Node {
        const node = this.panel.getChildByPath(path);
        if (!node) throw new Error(`讨伐面板缺少保存节点 ${path}`);
        return node;
    }
    private label(path: string, value: string): void { this.saved(path).getComponent(Label)!.string = value; }
    private enable(path: string, value: boolean): void { this.saved(path).getComponent(Button)!.interactable = value; }
    private renderToggle(path: string, enabled: boolean, checked: boolean): void {
        const toggle = this.saved(path).getComponent(Toggle)!;
        toggle.interactable = enabled; toggle.setIsCheckedWithoutNotify(checked);
    }
    private row(node: Node, label: string, enabled: boolean): void {
        node.getChildByName('Text')!.getComponent(Label)!.string = label;
        node.getComponent(Button)!.interactable = enabled;
    }
    private on(node: Node, event: string, handler: (...args: any[]) => void): void {
        node.on(event, handler, this);
        this.handlers.push({ node, event, handler });
    }
    private button(path: string, fn: () => void): void {
        this.on(this.saved(path), Button.EventType.CLICK, () => { this.transport.sound(); fn(); });
    }
    private toggle(path: string, fn: (checked: boolean) => void): void {
        const node = this.saved(path), toggle = node.getComponent(Toggle)!;
        this.on(node, Toggle.EventType.TOGGLE, () => { this.transport.sound(); fn(toggle.isChecked); });
    }
}
