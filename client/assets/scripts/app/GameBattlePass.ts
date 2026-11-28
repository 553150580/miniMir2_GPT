// 本文件用保存的详情面板展示战令日期、档位、确认差额和历史补领。
import type Long from 'long';
import { battlePassDelta, validateBattlePassState } from './BattlePassProtocol';
import type { BattlePassClaim, BattlePassReward, BattlePassState } from './BattlePassProtocol';
import type { ItemAction, ItemActionView } from './GameItemActions';
import { yuanbaoText } from './SkillAssetsModel';

type Read = () => ItemActionView | null; // 当前保存弹窗的唯一内容绑定。
type Page = 'days' | 'reward' | 'tiers' | 'tier' | 'history';

interface Context {
    scope: string; // 当前角色附着身份。
    scene: Long; // 当前场景编号。
    rule: Long; // 当前生效规则。
    blocked: boolean; // 等待原资产确认时关闭新领取。
}

interface Commands {
    context(): Context | undefined; // 读取当前附着和面板状态。
    show(read: Read): void; // 打开保存的详情弹窗。
    current(read: Read): boolean; // 检查是否仍是当前活动弹窗。
    refresh(): void; // 重读当前详情。
    report(error: unknown): void; // 展示当前请求的可恢复错误。
    itemName(item: number): string; // 用当前规则目录显示实际模板名。
    query(scene: Long, start: string, before: string): Promise<BattlePassState>; // 读取指定周期和历史页。
    claim(request: BattlePassClaim, revision: Long): Promise<void>; // 原资产确认与重放。
}

interface View {
    scope: string; // 打开时的角色和场景。
    read: Read; // 弹窗的原读回调。
    page: Page; // 当前显示日程、确认、档位或历史。
    pending: boolean; // 当前目录正在加载。
    day: number; // 选中的相对日，零为一次奖。
    pickedTier: number; // 用于查看完整奖表的档位。
    start: string; // 选择查看的周期，空字符串表示当前。
    before: string; // 历史分页游标。
    state?: BattlePassState; // 当前权威目录。
}

const cash = (fen: Long): string => {
    const amount = BigInt(fen.toString());
    return `${amount / 100n}.${(amount % 100n).toString().padStart(2, '0')} 元`;
};
const tierName = (state: BattlePassState, tier: number): string =>
    tier === 1 ? '免费档' : cash(state.tiers[tier - 1].totalCashFen).replace(/\.00 元$/, ' 元') + '档';
const hasReward = (reward: BattlePassReward): boolean => reward.materials.length > 0 || !reward.boundGold.isZero() || !reward.boundYuanbaoUnits.isZero();

// GameBattlePass 负责一次活动弹窗的全部查询和领取状态。
export class GameBattlePass {
    private view?: View; // 最近一次打开且仍关联原角色的弹窗。
    private confirmation?: { request: BattlePassClaim; due: BattlePassReward }; // 在途原确认差额，用于对账。

    constructor(private readonly commands: Commands) {}

    // open 进入当前角色战令，旧角色或已关闭面板的查询结果失效。
    async open(): Promise<void> {
        const context = this.commands.context();
        if (!context) return;
        const view: View = { scope: context.scope, read: () => this.render(view), page: 'days', pending: false,
            day: 0, pickedTier: 1, start: '', before: '' };
        this.view = view;
        this.commands.show(view.read);
        await this.load(view, '', '');
    }

    // expectedReward 仅同一原操作可使用尚存在的确认条款。
    expectedReward(request: BattlePassClaim): BattlePassReward | undefined {
        const current = this.confirmation;
        return current?.request.seasonStart === request.seasonStart && current.request.day === request.day &&
            current.request.tier === request.tier && current.request.claimedTier === request.claimedTier ? current.due : undefined;
    }

    // active 确认查询和领取仍属于主面板上的原角色附着。
    private active(view: View): Context | undefined {
        const context = this.commands.context();
        return this.view === view && context?.scope === view.scope && this.commands.current(view.read) ? context : undefined;
    }

    // load 在选中历史期或回执到账后刷新原角色视图。
    private async load(view: View, start: string, before: string): Promise<void> {
        const context = this.active(view);
        if (!context || view.pending) return;
        view.pending = true;
        this.commands.refresh();
        try {
            const state = await this.commands.query(context.scene, start, before);
            if (!this.active(view)) return;
            if (!state.sceneEpoch?.equals(context.scene) || !state.ruleVersion?.equals(context.rule) || start && state.seasonStart !== start) {
                throw new Error('战令已切换场景或周期');
            }
            validateBattlePassState(state);
            view.state = state;
            view.start = start;
            view.before = before;
        } finally {
            view.pending = false;
            this.commands.refresh();
        }
    }

    // render 根据当前视图将固定控件映射到真实日程和奖表。
    private render(view: View): ItemActionView | null {
        const context = this.commands.context();
        if (!context || context.scope !== view.scope || this.view !== view) return null;
        const state = view.state;
        if (view.pending || !state) return { title: '战令', lines: [view.pending ? '正在读取战令' : '战令暂未读取'], actions: [
            { label: '刷新战令', enabled: !view.pending, run: () => this.load(view, view.start, view.before) },
        ] };
        const back = this.change(view, 'days', '返回战令');
        if (view.page === 'reward') return this.reward(view, state, context, back);
        if (view.page === 'tier') return this.tier(view, state);
        if (view.page === 'tiers') return { title: '战令档位', lines: [], actions: [back],
            options: state.tiers.map(row => ({ key: `tier-${row.tier}`, label: `${tierName(state, row.tier)}${row.tier === state.tier ? ' · 已持有' : ''}`,
                enabled: true, choose: () => { view.pickedTier = row.tier; view.page = 'tier'; } })) };
        if (view.page === 'history') return this.history(view, state, back);
        return this.days(view, state);
    }

    // days 只展示今天与已经达成资格的日期，漏领奖励按原日期选择。
    private days(view: View, state: BattlePassState): ItemActionView {
        const current = Date.parse(state.operatingDay) - Date.parse(state.seasonStart);
        const today = current >= 0 && state.operatingDay < state.seasonEnd ? Math.floor(current / 86400000) + 1 : 0;
        const options = state.days.filter(row => row.qualified || row.day === today).map(row => ({
            key: `day-${row.day}`, label: `${row.day === today ? '今日' : '第 ' + row.day + ' 天'} · ${!row.qualified ? '活跃未达成' :
                row.claimedTier === state.tier ? '已领' : row.claimedTier ? '可补差额' : '可领取'}`,
            enabled: true, choose: () => { view.day = row.day; view.page = 'reward'; },
        }));
        if (hasReward(state.tiers[state.tier - 1].season)) options.unshift({ key: 'cycle', label: '周期一次奖励',
            enabled: true, choose: () => { view.day = 0; view.page = 'reward'; } });
        return { title: '战令', lines: [`${state.seasonStart} 至 ${state.seasonEnd} 零点`, `当前${tierName(state, state.tier)}`,
            ...(today ? [`今日活跃 ${state.activity}/${state.activityTarget}，完成每日任务后领取`] : ['历史周期'])], options, actions: [
            { label: '刷新战令', enabled: true, run: () => this.load(view, view.start, '') },
            this.change(view, 'tiers', '档位与奖励'), this.change(view, 'history', '历史周期'),
        ] };
    }

    // reward 显示当前日期实际可领差额并确认对应的原请求。
    private reward(view: View, state: BattlePassState, ctx: Context, back: ItemAction): ItemActionView {
        const row = state.days[view.day - 1];
        const claimed = row?.claimedTier ?? state.seasonClaimedTier;
        const eligible = !view.day || row?.qualified;
        const due = battlePassDelta(state, view.day, claimed);
        const title = view.day ? '第 ' + view.day + ' 天奖励' : '周期一次奖励';
        if (!eligible) return { title, lines: [`今日活跃 ${state.activity}/${state.activityTarget}`, '领取每日任务奖励可增加活跃度'], actions: [back] };
        if (!hasReward(due)) return { title, lines: ['该档奖励已全部领取'], actions: [back] };
        return { title, lines: [claimed ? `原已领${tierName(state, claimed)}，本次领取差额` : `领取${tierName(state, state.tier)}绑定奖励`, ...this.rewardLines(due)], actions: [
            { label: '领取奖励', enabled: !ctx.blocked, run: () => this.claim(view, state, claimed, due) }, back,
        ] };
    }

    // tier 同时展示此档每日总奖、周期奖和当前角色的现金差额。
    private tier(view: View, state: BattlePassState): ItemActionView {
        const row = state.tiers[view.pickedTier - 1];
        const lines = [row.totalCashFen.isZero() ? '免费开放' : `完整价格 ${cash(row.totalCashFen)}`];
        if (!row.upgradeCashFen.isZero()) lines.push(`当前升档差额 ${cash(row.upgradeCashFen)}`, '现金渠道待接入');
        lines.push('合格日奖励', ...this.rewardLines(row.daily));
        if (hasReward(row.season)) lines.push('周期一次奖励', ...this.rewardLines(row.season));
        return { title: tierName(state, row.tier), lines, actions: [this.change(view, 'tiers', '返回档位')] };
    }

    // history 使用服务器游标查最近和更早周期，逐期展示已有资格。
    private history(view: View, state: BattlePassState, back: ItemAction): ItemActionView {
        const options = state.history.map(start => ({ key: start, label: `${start} 周期`, enabled: true,
            choose: () => { void this.load(view, start, '').then(() => { view.page = 'days'; this.commands.refresh(); }).catch(error => this.commands.report(error)); } }));
        const actions: ItemAction[] = [{ label: '查看当前周期', enabled: true,
            run: async () => { await this.load(view, '', ''); view.page = 'days'; } }, back];
        if (state.historyHasMore) actions.push({ label: '较早周期', enabled: true, run: () => this.load(view, view.start, state.history[state.history.length - 1]) });
        return { title: '历史战令', lines: options.length ? [] : ['暂无已参与周期'], options, actions };
    }

    // change 适配现有详情固定操作按钮的同步刷新约定。
    private change(view: View, page: Page, label: string): ItemAction {
        return { label, enabled: true, run: () => { view.page = page; } };
    }

    // rewardLines 保留绑定钱包和材料确切个数。
    private rewardLines(reward: BattlePassReward): string[] {
        const lines = reward.materials.map(row => `${this.commands.itemName(row.itemId)} ×${row.quantity}`);
        if (!reward.boundGold.isZero()) lines.push(`绑定金币 ${reward.boundGold}`);
        if (!reward.boundYuanbaoUnits.isZero()) lines.push(`绑定元宝 ${yuanbaoText(reward.boundYuanbaoUnits)}`);
        return lines;
    }

    // claim 只提交玩家看到的档位差额，到账后同步已领状态。
    private async claim(view: View, state: BattlePassState, claimedTier: number, due: BattlePassReward): Promise<void> {
        const context = this.active(view);
        if (!context || context.blocked) return;
        const request: BattlePassClaim = { seasonStart: state.seasonStart, day: view.day, tier: state.tier, claimedTier };
        this.confirmation = { request, due };
        try { await this.commands.claim(request, state.roleRevision); }
        finally { this.confirmation = undefined; }
        if (!this.active(view)) return;
        view.page = 'days';
        await this.load(view, view.start, view.before);
    }
}
