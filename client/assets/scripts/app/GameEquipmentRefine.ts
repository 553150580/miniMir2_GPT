// 洗炼的双列比较、材料覆盖层和原操作重放。所有固定控件来自保存场景。
import { Button, Color, isValid, Label, Node, Toggle } from 'cc';
import type Long from 'long';
import { PopupView } from '../ui/PopupView';
import { assetId, yuanbaoText } from './SkillAssetsModel';
import { GameItemActions } from './GameItemActions';
import type { ItemActionView } from './GameItemActions';
import { availableRefineStones, findRefineItem, freezeRefineTarget, refineAffixText, RefineStoneSelection,
    refineFeeLines, refineQualityChances, refineTargetState, sameRefineId, validateRefineQuote } from './EquipmentRefineModel';
import type { RefineAction, RefineItem, RefineQuote, RefineQuoteRequest, RefineSnapshot } from './EquipmentRefineModel';
import { validateRefineReceipt } from './EquipmentRefineReceipt';
import type { RefineReceipt } from './EquipmentRefineReceipt';

interface RefineControls {
    snapshot(): RefineSnapshot | null;
    newSession(): Uint8Array;
    quote(request: RefineQuoteRequest): Promise<RefineQuote>;
    submit(request: RefineAction, expectedRevision?: Long): Promise<void>;
    tap(): void;
    error(error: unknown): void;
}

interface Review { readonly request: RefineQuoteRequest; readonly quote: RefineQuote; }
interface View {
    readonly scope: string;
    readonly role: string;
    readonly id: Uint8Array;
    readonly equipped: boolean;
    readonly uiSessionId: Uint8Array;
    readonly materials: RefineStoneSelection;
    readonly locks: Set<number>;
    review?: Review;
    action?: RefineAction;
    receipt?: RefineReceipt;
    working: boolean;
    notice: string;
}

type TargetState = NonNullable<ReturnType<typeof refineTargetState>>;
interface Selection { readonly snapshot: RefineSnapshot; readonly item: RefineItem; readonly target: TargetState; }
const slotCount = 4;
const activeColor = new Color(225, 222, 212, 255);
const mutedColor = new Color(169, 174, 170, 255);
const rareColor = new Color(255, 203, 102, 255);

export class GameEquipmentRefine {
    private readonly panel: Node;
    private readonly materialPanel: Node;
    private readonly materialActions: GameItemActions;
    private readonly handlers: { node: Node; event: string; listener: () => void }[] = [];
    private view?: View;

    constructor(safe: Node, private readonly popup: PopupView, private readonly transport: RefineControls) {
        const panel = safe.getChildByPath('PopupView/Panels/EquipmentRefine');
        const materialPanel = safe.getChildByPath('PopupView/Panels/EquipmentRefineMaterials');
        if (!panel || !materialPanel) throw new Error('洗炼的左右词条与材料面板未保存在场景');
        this.panel = panel;
        this.materialPanel = materialPanel;
        this.materialActions = new GameItemActions(safe, popup, transport.error, materialPanel.name);
        this.on('MaterialButton', Button.EventType.CLICK, view => this.materialActions.open(() => this.materialView(view)));
        this.on('BoundToggle', Toggle.EventType.TOGGLE, view => this.toggleBinding(view));
        this.on('RollButton', Button.EventType.CLICK, view => this.askQuote(view));
        this.on('ApplyButton', Button.EventType.CLICK, view => this.decide(view, 2));
        this.on('DiscardButton', Button.EventType.CLICK, view => this.decide(view, 3));
        for (let slot = 0; slot < slotCount; ++slot) {
            this.on('Lock' + (slot + 1), Toggle.EventType.TOGGLE, view => this.lock(view, slot));
        }
        popup.node.on(PopupView.EVENT_CHANGED, this.refresh, this);
    }

    open(instanceId: Uint8Array, equipped: boolean): void {
        const snapshot = this.transport.snapshot();
        const item = snapshot && findRefineItem(snapshot, instanceId, equipped);
        const target = snapshot && item && refineTargetState(snapshot, item);
        if (!snapshot || !item || !target) return;
        this.view = { scope: snapshot.scope, role: snapshot.role, id: new Uint8Array(instanceId), equipped,
            uiSessionId: this.transport.newSession(), materials: new RefineStoneSelection(), locks: new Set(), working: false, notice: '' };
        this.popup.show(this.panel.name);
    }

    // 原操作回执可以先到。只在同角色同件的新库存到达后恢复操作按钮。
    reconcile(request: RefineAction, value: RefineReceipt): RefineReceipt {
        const view = this.view;
        const quote = request.action === 1 && view?.review && sameRefineId(request.quoteId, view.review.quote.quoteId) ? view.review.quote : undefined;
        return validateRefineReceipt(request, value, quote);
    }

    accept(role: string, request: RefineAction, receipt: RefineReceipt): void {
        const view = this.view;
        if (!view || view.role !== role || view.equipped !== request.target.equipped ||
            !sameRefineId(view.id, request.target.instanceId) || !sameRefineId(view.uiSessionId, request.uiSessionId)) return;
        view.receipt = receipt;
        view.review = undefined;
        view.materials.clear();
        view.notice = request.action === 1 ? '新词条已生成，可采纳或放弃' : request.action === 2 ? '已采纳候选' : '已放弃候选，保留原词条';
        if (this.popup.top === this.materialPanel) this.popup.back();
        this.refresh();
    }

    refresh = (): void => {
        const view = this.view;
        if (!view) return;
        if (this.popup.top === this.materialPanel) { this.materialActions.refresh(); return; }
        if (this.popup.top !== this.panel) return;
        const selection = this.current(view);
        if (!selection) { this.unavailable(); return; }
        const { snapshot, item, target } = selection;
        if (view.receipt && item.version.greaterThanOrEqual(view.receipt.target.version)) {
            view.receipt = undefined;
            view.action = undefined;
        }
        const busy = this.blocked(view, selection);
        const pending = target.pending;
        const lockSlots = this.lockedSlots(view, target);
        this.label('Equipment').string = `${target.definition.name} · ${['', '精良', '稀有', '史诗', '传说'][item.quality ?? 0]} · ${item.bound ? '已绑定' : '非绑定'}`;
        this.label('State').string = view.receipt ? '操作已确认，正在同步原装备'
            : view.notice || (target.blocked ? '原装备受保护，暂不可洗炼' :
                pending ? '最新候选已付费 · 采纳后生效' : '请选择原石堆生成候选');
        for (let slot = 0; slot < slotCount; ++slot) {
            const left = target.current.find(affix => affix.slot === slot);
            const right = pending?.affixes.find(affix => affix.slot === slot);
            this.setAffix('Current', slot, slot < target.slots, left, snapshot);
            this.setAffix('Candidate', slot, slot < target.slots, right, snapshot);
            const checked = lockSlots.includes(slot);
            const canLock = !!left && !busy && (checked || (lockSlots.length < target.slots - 1 && snapshot.rules.locks.has(lockSlots.length + 1)));
            this.setToggle('Lock' + (slot + 1), checked, canLock, checked ? '已锁' : canLock ? '锁定' : '—');
        }
        const count = view.materials.total(snapshot);
        this.label('Ledger/Cost').string = `洗炼石 ${target.stones}枚 · 金币 ${target.gold}`;
        this.label('Ledger/Materials').string = `已选 ${count}/${target.stones}枚 · 可用 ${this.stock(snapshot, view.materials.allowBound)}枚`;
        this.label('Ledger/LockCost').string = lockSlots.length ? `锁 ${lockSlots.length}槽 · 本次另付 ${yuanbaoText(snapshot.rules.locks.get(lockSlots.length)!)}元宝`
            : '本次未锁槽';
        this.setToggle('BoundToggle', view.materials.allowBound, !busy, `绑定资源：${view.materials.allowBound ? '开' : '关'}`);
        this.button('MaterialButton', !busy);
        this.button('RollButton', !busy && count === target.stones, pending ? '继续洗炼' : '查看报价');
        this.button('ApplyButton', !busy && !!pending);
        this.button('DiscardButton', !busy && !!pending);
    };

    private current(view: View): Selection | undefined {
        const snapshot = this.transport.snapshot();
        if (this.view !== view || !snapshot || snapshot.scope !== view.scope || snapshot.role !== view.role) return undefined;
        const item = findRefineItem(snapshot, view.id, view.equipped);
        const target = item && refineTargetState(snapshot, item);
        return item && target ? { snapshot, item, target } : undefined;
    }

    private visible(view: View): boolean {
        return this.view === view && (this.popup.top === this.panel || this.popup.top === this.materialPanel);
    }

    private blocked(view: View, current: Selection): boolean {
        return view.working || !!view.action || !!view.receipt || current.snapshot.blocked || current.target.blocked;
    }

    private stock(snapshot: RefineSnapshot, allowBound: boolean): bigint {
        return availableRefineStones(snapshot, allowBound).reduce((sum, item) => sum + BigInt(item.quantity.toString()), 0n);
    }

    private lockedSlots(view: View, target: TargetState): number[] {
        return Array.from(view.locks).filter(slot => target.current.some(affix => affix.slot === slot)).sort((left, right) => left - right);
    }

    private lock(view: View, slot: number): void {
        const current = this.current(view);
        if (current && !this.blocked(view, current) && current.target.current.some(affix => affix.slot === slot)) {
            if (view.locks.has(slot)) view.locks.delete(slot);
            else if (view.locks.size < current.target.slots - 1 && current.snapshot.rules.locks.has(view.locks.size + 1)) view.locks.add(slot);
        }
        this.refresh();
    }

    private async toggleBinding(view: View): Promise<void> {
        const current = this.current(view);
        this.refresh();
        if (!current || this.blocked(view, current)) return;
        if (!view.materials.allowBound) {
            if (!await this.popup.confirm('Confirm', '允许选择绑定洗炼石，也可支付绑定金币或绑定元宝。', '开启绑定资源', '继续开启') ||
                !this.visible(view) || !this.current(view)) return;
            if (!await this.popup.confirm('Confirm', '实际使用任一绑定资源时，装备立即绑定当前角色。\n放弃词条后仍保留装备绑定。', '确认装备归属', '确认开启') ||
                !this.visible(view) || !this.current(view)) return;
        }
        view.materials.allowBound = !view.materials.allowBound;
        view.materials.clear();
        view.notice = '';
        this.refresh();
    }

    private materialView(view: View): ItemActionView | null {
        const current = this.current(view);
        if (!current) return null;
        const { snapshot, target } = current;
        const ready = !this.blocked(view, current), options = view.materials.options(snapshot), total = view.materials.total(snapshot);
        const focus = view.materials.focused(snapshot);
        return { title: '选择原洗炼石', optionsFirst: true,
            lines: [`已选 ${total}/${target.stones}枚`, view.notice || '点选原石堆，再用加减调整数量',
                ...(options.length ? [] : ['当前没有可用的原石堆'])],
            options: options.map(item => ({ key: assetId(item.instanceId), enabled: ready,
                label: `${focus && sameRefineId(focus.instanceId, item.instanceId) ? '● ' : ''}${this.stoneName(item)} · 取${view.materials.count(item)}`,
                choose: () => { view.materials.pick(snapshot, item, target.stones); } })),
            actions: [{ label: '查看报价', enabled: ready && total === target.stones, run: () => this.askQuote(view) },
                { label: '清空选择', enabled: ready && total > 0n, run: () => { view.materials.clear(); this.materialActions.refresh(); } }],
            ...(focus ? { quantity: { value: view.materials.count(focus).toString(),
                minus: ready && view.materials.count(focus) > 0n ? () => view.materials.adjust(snapshot, target.stones, -1) : undefined,
                plus: ready && total < target.stones && view.materials.count(focus) < BigInt(focus.quantity.toString())
                    ? () => view.materials.adjust(snapshot, target.stones, 1) : undefined } } : {}) };
    }

    private stoneName(item: RefineItem): string {
        return `${item.bound ? '绑定' : '非绑'}石 ${item.quantity}枚 · ${assetId(item.instanceId).slice(-6)}`;
    }

    private async askQuote(view: View): Promise<void> {
        const current = this.current(view);
        if (!current || this.blocked(view, current)) return;
        const request = view.materials.request(current.snapshot, current.item, view.equipped, this.lockedSlots(view, current.target), view.uiSessionId);
        if (!request) return;
        view.notice = '';
        view.working = true;
        this.refresh();
        try {
            const quote = await this.transport.quote(request);
            const latest = this.current(view);
            if (!latest || !this.visible(view)) return;
            validateRefineQuote(quote, request, latest.snapshot);
            const review = { request, quote };
            view.review = review;
            this.materialActions.open(() => this.quoteView(view, review));
        } catch (error) {
            if (this.visible(view)) view.notice = this.message(error);
        } finally {
            view.working = false;
            this.refresh();
        }
    }

    private quoteView(view: View, review: Review): ItemActionView | null {
        const current = this.current(view);
        if (!current || view.review !== review) return null;
        const quote = review.quote;
        let reason = '';
        try { validateRefineQuote(quote, review.request, current.snapshot); }
        catch (error) { reason = this.message(error); }
        return { title: '确认洗炼报价',
            lines: [`${current.target.definition.name} · ${quote.outputBound ? '结算后绑定本角色' : '保持非绑定'}`,
                ...quote.stones.map(row => `扣 ${row.quantity}枚${row.item.bound ? '绑定' : '非绑'}石 · ${assetId(row.item.instanceId).slice(-6)}`),
                ...refineFeeLines(quote),
                quote.lockedSlots.length ? `锁定左侧第 ${quote.lockedSlots.map(slot => slot + 1).join('、')} 槽` : '本次未锁槽',
                ...(current.target.pending ? ['本次会覆盖右侧上一份候选'] : []),
                '确认后立即扣费', ...refineQualityChances(current.snapshot.rules), ...[reason, view.notice].filter(Boolean)],
            actions: [{ label: '确认洗炼', enabled: !reason && !this.blocked(view, current),
                run: () => this.generate(view, review) },
                { label: '返回选石', enabled: !view.working, run: () => {
                    view.review = undefined; this.materialActions.open(() => this.materialView(view));
                } }] };
    }

    private async generate(view: View, review: Review): Promise<void> {
        const current = this.current(view);
        if (!current || this.blocked(view, current) || view.review !== review) return;
        validateRefineQuote(review.quote, review.request, current.snapshot);
        await this.perform(view, { action: 1, target: review.request.target, uiSessionId: view.uiSessionId,
            quoteId: new Uint8Array(review.quote.quoteId), allowBound: review.quote.allowBound }, review.quote.roleRevision);
    }

    private async decide(view: View, action: 2 | 3): Promise<void> {
        const current = this.current(view);
        const pending = current?.target.pending;
        if (!current || !pending || this.blocked(view, current)) return;
        const label = action === 2 ? '采纳候选' : '放弃候选';
        const message = action === 2 ? '右侧候选将替换当前词条。' : '当前词条会保留，已支付的费用与绑定仍生效。';
        if (!await this.popup.confirm('Confirm', message, label, label) || !this.visible(view)) return;
        const latest = this.current(view);
        if (!latest || this.blocked(view, latest) || !latest.target.pending || !sameRefineId(pending.pendingId, latest.target.pending.pendingId)) {
            view.notice = '原候选已变化，请重新查看';
            this.refresh();
            return;
        }
        // 穿戴磨损只更新版本。确认仍是同一候选时，提交最新观察到的原版本。
        await this.perform(view, { action, target: freezeRefineTarget(latest.item, view.equipped), uiSessionId: view.uiSessionId,
            pendingId: new Uint8Array(pending.pendingId) });
    }

    private async perform(view: View, request: RefineAction, expectedRevision?: Long): Promise<void> {
        view.action = request;
        view.notice = '';
        this.refresh();
        try { await this.transport.submit(request, expectedRevision); }
        catch (error) {
            view.action = undefined;
            view.notice = this.message(error);
        } finally { this.refresh(); }
    }

    private message(error: unknown): string { return error instanceof Error ? error.message : '原洗炼操作待核对'; }

    private on(path: string, event: string, fn: (view: View) => void | Promise<void>): void {
        const node = this.panel.getChildByName(path)!;
        const listener = (): void => {
            if (!this.view || this.popup.top !== this.panel) return;
            this.transport.tap();
            Promise.resolve(fn(this.view)).catch(this.transport.error);
        };
        node.on(event, listener, this);
        this.handlers.push({ node, event, listener });
    }

    private label(path: string): Label { return this.panel.getChildByPath(path)!.getComponent(Label)!; }
    private button(path: string, enabled: boolean, text?: string): void {
        this.panel.getChildByName(path)!.getComponent(Button)!.interactable = enabled;
        if (text) this.label(path + '/Label').string = text;
    }
    private setToggle(path: string, checked: boolean, enabled: boolean, text: string): void {
        const toggle = this.panel.getChildByName(path)!.getComponent(Toggle)!;
        toggle.interactable = enabled;
        toggle.setIsCheckedWithoutNotify(checked);
        this.label(path + '/Label').string = text;
    }
    private setAffix(column: string, index: number, active: boolean, affix: TargetState['current'][number] | undefined, snapshot: RefineSnapshot): void {
        const label = this.label(`${column}/Row${index + 1}/Value`);
        label.string = affix ? refineAffixText(snapshot.rules, affix)
            : active ? (column === 'Current' ? '空词条' : '待生成') : '未开放';
        label.color = affix ? (affix.tier >= 4 ? rareColor : activeColor) : mutedColor;
    }
    private unavailable(): void {
        this.label('Equipment').string = '';
        this.label('State').string = '原装备或附着已变化，请返回详情';
        for (const name of ['MaterialButton', 'RollButton', 'ApplyButton', 'DiscardButton']) this.button(name, false);
        this.setToggle('BoundToggle', false, false, '绑定资源：关');
        for (let index = 1; index <= slotCount; ++index) {
            this.setToggle('Lock' + index, false, false, '—');
            this.label(`Current/Row${index}/Value`).string = this.label(`Candidate/Row${index}/Value`).string = '—';
        }
        for (const name of ['Cost', 'Materials', 'LockCost']) this.label('Ledger/' + name).string = '';
    }

    dispose(): void {
        this.view = undefined;
        this.materialActions.dispose();
        if (isValid(this.popup.node, true)) this.popup.node.off(PopupView.EVENT_CHANGED, this.refresh, this);
        for (const { node, event, listener } of this.handlers) if (isValid(node, true)) node.off(event, listener, this);
        this.handlers.length = 0;
    }
}
