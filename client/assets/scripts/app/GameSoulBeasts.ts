// 本文件复用保存的详情列表，管理魂兽名录、三池选择和碎片确认。
import Long from 'long';
import type { ItemActionView } from './GameItemActions';

interface Beast {
    typeId: number; // 原名录编号。
    star: number; // 当前星级。
    affixIds: number[]; // 依次选中的伤害、防守和特效词条。
}
interface StarRule {
    salvage: number; // 分解当前星级的回收量。
    upgrade: number; // 升至当前星级的消耗。
}
interface BeastType {
    id: number; // 唯一类型编号。
    name: string; // 名录和首领历史用名。
    synthesis: number; // 合成一星的碎片费用。
    stars: StarRule[]; // 按一至七星排列的数额。
}
interface Affix {
    id: number; // 所属池和效果的稳定编号。
    name: string; // 界面显示的属性名称。
    unit: 'points' | 'bp'; // 属性点或基点。
    values: number[]; // 各星级对应的实际贡献。
}
interface SoulRules {
    types: BeastType[]; // 可收藏的原名录。
    affixes: Affix[]; // 本次已开放的各池选项。
}
interface SoulCollection {
    revision: Long; // 魂兽增减和变更时推进的修订。
    fragments: Long; // 角色余额，不进入物品栏。
    attachedTypeId: number; // 当前附体编号，零为空。
    beasts: Beast[]; // 已持有的唯一类型记录。
}
export interface SoulSnapshot {
    scope: string; // 原角色及入场附着。
    revision: Long; // 当前完整角色资产修订。
    rules: SoulRules; // 已加载的当前规则。
    state: SoulCollection; // 真实库存中投影的收藏。
    blocked: boolean; // 当前是否允许写入。
}
export interface SoulAction {
    action: number; // 本次玩家明确选择的动作。
    typeId: number; // 动作所指的原类型。
    affixId: number; // 仅换词条时填写的选项编号。
    soulRevision: Long; // 玩家确认时所见的收藏版本。
    fragmentDelta: Long; // 确认页所见的带符号碎片增减。
}
interface SoulReceipt {
    action: number; // 已确认的原动作或发奖。
    typeId: number; // 已结算的原类型。
    fragmentDelta: Long; // 实际发生的碎片变化。
    collection: SoulCollection; // 原操作完成时的收藏。
}
interface Commands {
    snapshot(): SoulSnapshot | null; // 读取当前场景的角色收藏。
    open(read: () => ItemActionView | null): void; // 用原保存详情显示成长界面。
    refresh(): void; // 原库存或选择变化后刷新列表。
    submit(request: SoulAction, revision: Long): Promise<void>; // 进入现有可重放经济请求。
}
interface View {
    scope: string; // 详情打开时的角色附着。
    typeId: number; // 详情所属类型，零时显示名录。
    configure: boolean; // 是否进入词条与分解页。
    affixId: number; // 等待玩家保存的词条。
    review?: { title: string; request: SoulAction; revision: Long }; // 原确认页所示的固定条款。
}

// 原动作编号和工作簿词条池使用同一双端约定。
const attach = 1, detach = 2, select = 3, upgrade = 4, salvage = 5, synthesis = 6, grant = 7;
const pools = ['伤害', '防守', '特效'];
const pool = (id: number) => Math.floor((id - 1) / 7);
const known = (id: unknown, max: number): id is number => typeof id === 'number' && Number.isInteger(id) && id >= 1 && id <= max;
const signed = (value: unknown): value is Long => Long.isLong(value) && !value.unsigned;
const revision = (value: unknown): value is Long => Long.isLong(value) && value.unsigned && value.lessThanOrEqual(Long.MAX_VALUE.toUnsigned());

// validateSoulReceipt 按原操作身份核复已落库的收藏和确切碎片增减。
export function validateSoulReceipt(value: SoulReceipt, request?: SoulAction): SoulReceipt {
    const state = value?.collection;
    if (!state || !revision(state.revision) || state.revision.isZero() || !signed(state.fragments) || state.fragments.isNegative() ||
        !known(value.typeId, 13) || !signed(value.fragmentDelta) || !Array.isArray(state.beasts) || state.beasts.length > 13 ||
        state.beasts.some((row, index) => !known(row.typeId, 13) || (index > 0 && row.typeId <= state.beasts[index - 1].typeId) ||
            !known(row.star, 7) || row.affixIds?.length !== 3 || row.affixIds.some((id, slot) => !known(id, 21) || pool(id) !== slot)) ||
        (state.attachedTypeId !== 0 && !state.beasts.some(row => row.typeId === state.attachedTypeId))) {
        throw new Error('魂兽回执的收藏状态无效');
    }
    const beast = state.beasts.find(row => row.typeId === value.typeId);
    if (!request) {
        if (value.action !== grant || value.fragmentDelta.isNegative() || !beast) throw new Error('魂兽奖项的原回执无效');
        return value;
    }
    if (value.action !== request.action || value.typeId !== request.typeId || !value.fragmentDelta.equals(request.fragmentDelta) ||
        !state.revision.equals(request.soulRevision.add(Long.UONE)) ||
        (request.action === salvage ? !!beast : !beast) ||
        request.action === attach && state.attachedTypeId !== request.typeId || request.action === detach && state.attachedTypeId !== 0 ||
        request.action === synthesis && beast?.star !== 1 || request.action === upgrade && (beast?.star ?? 0) < 2 ||
        request.action === select && beast?.affixIds[pool(request.affixId)] !== request.affixId) {
        throw new Error('魂兽回执与原确认不符');
    }
    return value;
}

// GameSoulBeasts 每次从当前永久角色的权威库存读取收藏。
export class GameSoulBeasts {
    private view?: View; // 当前详情状态，旧页面回调不得读取新角色的资产。

    constructor(private readonly commands: Commands) {}

    // open 重置名录选择并复用原保存详情。
    open(): void {
        const snapshot = this.commands.snapshot();
        if (!snapshot) return;
        const view: View = { scope: snapshot.scope, typeId: 0, configure: false, affixId: 0 };
        this.view = view;
        this.commands.open(() => this.render(view));
    }

    // snapshot 只允许同一角色、同一附着的活动详情读取库存。
    private snapshot(view: View): SoulSnapshot | undefined {
        const current = this.commands.snapshot();
        return this.view === view && current?.scope === view.scope ? current : undefined;
    }

    // render 将角色名录、当前能力和二次确认投影到已有列表与按钮。
    private render(view: View): ItemActionView | null {
        const s = this.snapshot(view);
        if (!s) return null;
        const type = s.rules.types.find(row => row.id === view.typeId);
        const beast = s.state.beasts.find(row => row.typeId === view.typeId);
        const balance = `通用碎片 ${s.state.fragments}`;
        if (!type) {
            const options = s.rules.types.map(row => {
                const owned = s.state.beasts.find(item => item.typeId === row.id);
                const label = `${row.name} ${owned ? owned.star + '星' : '未收录'}${s.state.attachedTypeId === row.id ? ' · 附体' : ''}`;
                return { key: String(row.id), label, enabled: true, choose: () => { view.typeId = row.id; } };
            });
            return { title: '魂兽名录', lines: [balance, `已收藏 ${s.state.beasts.length}/${s.rules.types.length}`], options, actions: [] };
        }
        const attached = s.state.attachedTypeId === type.id;
        if (view.review) {
            const review = view.review, delta = review.request.fragmentDelta;
            return { title: review.title, lines: [type.name, `通用碎片 ${delta.isNegative() ? '消耗 ' + delta.negate() : '获得 ' + delta}`, balance],
                actions: [{ label: '确认', enabled: !s.blocked && s.state.revision.equals(review.request.soulRevision),
                    run: () => this.send(view, review.request, review.revision) },
                { label: '返回魂兽', enabled: true, run: () => { view.review = undefined; this.commands.refresh(); } }] };
        }
        const back = { label: '返回名录', key: 'catalog', enabled: true, choose: () => this.back(view) };
        if (!beast) {
            return { title: type.name, lines: ['未收录', balance, `合成一星需 ${type.synthesis} 碎片`], options: [back],
                actions: [{ label: '合成', enabled: !s.blocked && s.state.fragments.greaterThanOrEqual(Long.fromNumber(type.synthesis)),
                    run: () => this.review(view, s, synthesis, -type.synthesis, '确认合成') }] };
        }
        const effects = s.rules.affixes.filter(row => beast.affixIds.includes(row.id)).map(row => this.affixLabel(row, beast.star));
        if (view.configure) {
            const options = s.rules.affixes.map(row => ({
                key: String(row.id), label: (view.affixId === row.id ? '✓ ' : '') + this.affixLabel(row, beast.star),
                enabled: !beast.affixIds.includes(row.id), choose: () => { view.affixId = row.id; },
            }));
            return { title: '配置魂兽', lines: [type.name, ...effects], options, optionsFirst: true, actions: [
                { label: '保存词条', enabled: !s.blocked && options.some(row => row.key === String(view.affixId) && row.enabled),
                    run: () => this.send(view, this.action(s, select, type.id, 0, view.affixId), s.revision) },
                { label: '分解', enabled: !s.blocked && !attached,
                    run: () => this.review(view, s, salvage, type.stars[beast.star - 1].salvage, '确认分解') },
                { label: '返回魂兽', enabled: true, run: () => { view.configure = false; view.affixId = 0; this.commands.refresh(); } },
            ] };
        }
        const next = type.stars[beast.star];
        return { title: type.name, lines: [`${beast.star}/${type.stars.length}星 · ${attached ? '已附体' : '未附体'}`, balance, ...effects,
            ...(next ? [`升至 ${beast.star + 1} 星需 ${next.upgrade} 碎片`] : [])], options: [back], actions: [
            { label: attached ? '解除附体' : '附体', enabled: !s.blocked,
                run: () => this.send(view, this.action(s, attached ? detach : attach, type.id), s.revision) },
            { label: '升星', enabled: !s.blocked && !!next && s.state.fragments.greaterThanOrEqual(Long.fromNumber(next.upgrade)),
                run: () => this.review(view, s, upgrade, -next.upgrade, '确认升星') },
            { label: '配置与分解', enabled: true, run: () => { view.configure = true; this.commands.refresh(); } },
        ] };
    }

    // affixLabel 按当前星级和单位格式化一条词条。
    private affixLabel(row: Affix, star: number): string {
        const value = row.values[star - 1];
        return `${pools[pool(row.id)]} · ${row.name} +${row.unit === 'bp' ? value / 100 + '%' : value}`;
    }

    // back 回到全名录并清除原详情中的确认和选项。
    private back(view: View): void {
        view.typeId = 0;
        view.configure = false;
        view.affixId = 0;
        view.review = undefined;
    }

    // action 固定原收藏修订及玩家看到的碎片变化。
    private action(s: SoulSnapshot, action: number, typeId: number, amount = 0, affixId = 0): SoulAction {
        return { action, typeId, affixId, soulRevision: s.state.revision, fragmentDelta: Long.fromNumber(amount) };
    }

    // review 保存确认条款，库存的其他变更不会重写已看到的费用。
    private review(view: View, s: SoulSnapshot, action: number, amount: number, title: string): void {
        view.review = { title, request: this.action(s, action, view.typeId, amount), revision: s.revision };
        this.commands.refresh();
    }

    // send 按同一份原条款提交并在成功后恢复详情或名录。
    private async send(view: View, request: SoulAction, at: Long): Promise<void> {
        const s = this.snapshot(view);
        if (!s || s.blocked || !s.state.revision.equals(request.soulRevision)) return;
        await this.commands.submit(request, at);
        view.review = undefined;
        view.affixId = 0;
        if (request.action === salvage) this.back(view);
        this.commands.refresh();
    }
}
