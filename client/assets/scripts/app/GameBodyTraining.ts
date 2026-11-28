// 本文件管理炼体系别选择、报价确认及已提交操作的回执校验。
import Long from 'long';
import { CodeBodyTrainingInputs, CodeBodyTrainingQuoteChanged, CodeBodyTrainingUnavailable, CodeOperationConflict } from '../generated/protocol_ids';
import type { ItemActionView } from './GameItemActions';
import { yuanbaoText } from './SkillAssetsModel';

// 权威库存同步各系等级，旧版本缺少的新增字段按零级显示。
interface BodyTrainingState {
    healthLevel: number; manaLevel: number; damageLevel?: number; absorptionLevel?: number;
    criticalLevel?: number; speedLevel?: number; resistanceLevel?: number;
}
interface Track { id: number; name: string; gold_base?: number; yuanbao_base_units?: number; bonus_per_rank: number; }
interface Step { from: number; cost_multiplier: number; success_ppm: number; }
interface TrainingRules { tracks: Track[]; steps: Step[]; health_cap_bp: number; mana_cap_bp: number; }
export interface BodyTrainingSnapshot {
    role: string; scope: string; sceneEpoch: Long; revision: Long; level: number; blocked: boolean;
    state: BodyTrainingState; rules: TrainingRules; canRebirth: boolean;
}
interface TrainingTerms {
    track: number; beforeLevel: number; targetLevel: number; successPpm: number;
    gold: Long; boundGold: Long; yuanbaoUnits?: Long; boundYuanbaoUnits?: Long; beforeBonus: Long; targetBonus: Long;
}
interface TrainingQuote {
    quoteId: Uint8Array; uiSessionId: Uint8Array; sceneEpoch: Long; roleRevision: Long; expiresAtUnixMs: Long; terms: TrainingTerms;
}
export interface BodyTrainingCommit { quoteId: Uint8Array; uiSessionId: Uint8Array; track: number; }
export interface BodyTrainingReceipt { quoteId: Uint8Array; terms: TrainingTerms; success: boolean; afterLevel: number; }
type Read = () => ItemActionView | null;
interface Commands {
    snapshot(): BodyTrainingSnapshot | null;
    newSession(): Uint8Array;
    open(read: Read): void;
    current(read: Read): boolean;
    refresh(): void;
    growthDetails(): void;
    quote(request: { sceneEpoch: Long; uiSessionId: Uint8Array; track: number }): Promise<TrainingQuote>;
    submit(request: BodyTrainingCommit, revision: Long): Promise<void>;
}
interface View {
    role: string; scope: string; session: Uint8Array; read: Read; notice: string;
    track?: number; // 当前从详情列表选中的系别。
    quote?: TrainingQuote; receipt?: BodyTrainingReceipt;
}

const damageTrack = 1, absorptionTrack = 2, healthTrack = 3, manaTrack = 4, criticalTrack = 5, speedTrack = 6, resistanceTrack = 7;
const knownTrack = (id: number) => Number.isInteger(id) && id >= damageTrack && id <= resistanceTrack;
const yuanbaoTrack = (id: number) => id >= criticalTrack && id <= resistanceTrack;
const validId = (v: unknown): v is Uint8Array => v instanceof Uint8Array && v.length === 16;
const same = (a: unknown, b: unknown) => validId(a) && validId(b) && a.every((octet, i) => octet === b[i]);
const amount = (n: unknown): n is Long => Long.isLong(n) && !n.unsigned && !n.isNegative();
const cap = (s: BodyTrainingSnapshot) => Math.min(s.level, s.rules.steps.length);

// 把权威库存各系等级投影到工作簿的系别编号。
function rank(s: BodyTrainingSnapshot, id: number): number {
    switch (id) {
        case damageTrack: return s.state.damageLevel ?? 0;
        case absorptionTrack: return s.state.absorptionLevel ?? 0;
        case healthTrack: return s.state.healthLevel;
        case manaTrack: return s.state.manaLevel;
        case criticalTrack: return s.state.criticalLevel ?? 0;
        case speedTrack: return s.state.speedLevel ?? 0;
        case resistanceTrack: return s.state.resistanceLevel ?? 0;
        default: return 0;
    }
}

// 已展示的费用、系别与等级必须和实际成交一致。新连接仍核对原操作形状。
export function validateBodyTrainingReceipt(request: BodyTrainingCommit, receipt: BodyTrainingReceipt, quoted?: TrainingTerms): BodyTrainingReceipt {
    const t = receipt?.terms;
    if (!same(request.quoteId, receipt?.quoteId) || !validTerms(t) || t.track !== request.track || typeof receipt.success !== 'boolean' ||
        receipt.afterLevel !== t.beforeLevel + (receipt.success ? 1 : 0) || (!receipt.success && t.successPpm === 1000000) ||
        (quoted && (!quoted.gold.equals(t.gold) || !quoted.boundGold.equals(t.boundGold) ||
            !(quoted.yuanbaoUnits ?? Long.ZERO).equals(t.yuanbaoUnits ?? Long.ZERO) ||
            !(quoted.boundYuanbaoUnits ?? Long.ZERO).equals(t.boundYuanbaoUnits ?? Long.ZERO) || !quoted.beforeBonus.equals(t.beforeBonus) ||
            !quoted.targetBonus.equals(t.targetBonus) || quoted.track !== t.track || quoted.beforeLevel !== t.beforeLevel))) {
        throw new Error('炼体回执与原操作不符，保留原身份核对');
    }
    return receipt;
}

// GameBodyTraining 复用角色成长入口和已保存的详情列表与按钮。
export class GameBodyTraining {
    private view?: View; // 当前角色和当前附着拥有的原视图。

    constructor(private readonly commands: Commands) {}

    open(): void {
        const s = this.commands.snapshot();
        if (!s) return;
        const view: View = { role: s.role, scope: s.scope, session: this.commands.newSession(), notice: '', read: () => this.render(view) };
        this.view = view;
        this.commands.open(view.read);
    }

    reconcile(request: BodyTrainingCommit, receipt: BodyTrainingReceipt): BodyTrainingReceipt {
        const quote = this.view?.quote;
        return validateBodyTrainingReceipt(request, receipt, same(request.quoteId, quote?.quoteId) ? quote?.terms : undefined);
    }

    accept(role: string, request: BodyTrainingCommit, receipt: BodyTrainingReceipt): void {
        const view = this.view;
        if (!view || view.role !== role || !same(request.quoteId, view.quote?.quoteId) || !this.active(view)) return;
        view.quote = undefined;
        view.receipt = receipt;
        this.commands.refresh();
    }

    private scoped(view: View): BodyTrainingSnapshot | undefined {
        const current = this.commands.snapshot();
        return this.view === view && current?.role === view.role && current.scope === view.scope ? current : undefined;
    }

    private active(view: View): BodyTrainingSnapshot | undefined {
        return this.commands.current(view.read) ? this.scoped(view) : undefined;
    }

    private render(view: View): ItemActionView | null {
        const s = this.scoped(view);
        if (!s) return null;
        if (view.receipt) {
            const r = view.receipt, track = this.track(s, r.terms.track);
            return { title: r.success ? '炼体成功' : '炼体未升级',
                lines: [`${track?.name ?? '炼体'} 当前 ${r.afterLevel} 级`, ...costLines(r.terms)],
                actions: [{ label: '返回炼体', enabled: true, run: () => this.overview(view) }] };
        }
        if (view.quote) {
            const quote = view.quote, t = quote.terms, current = this.validQuote(view, quote, s);
            const lines = [`${this.track(s, t.track)?.name ?? '炼体'} ${t.beforeLevel} → ${t.targetLevel}级`,
                `成功率 ${t.successPpm / 10000}%`, ...costLines(t), `属性额度 ${bonusText(t.track, t.beforeBonus)} → ${bonusText(t.track, t.targetBonus)}`,
                '实际收益受总上限约束', '失败扣费并保留原等级'];
            if (!current) lines.unshift('报价已变化，请重新查看');
            return { title: '炼体报价', lines, actions: [
                { label: current ? '确认炼体' : '重新报价', enabled: !s.blocked && rank(s, t.track) < cap(s),
                    run: () => current ? this.commit(view, quote) : this.readQuote(view, t.track) },
                { label: '返回炼体', enabled: true, run: () => this.overview(view) },
            ] };
        }
        const summary = [...(view.notice ? [view.notice] : []), `本角色可练至 ${cap(s)} 级`];
        const limits = [
            `独立生命加成上限 ${s.rules.health_cap_bp / 100}%`, `独立魔法加成上限 ${s.rules.mana_cap_bp / 100}%`];
        const trackLabel = (track: Track) => `${track.name} ${rank(s, track.id)} / ${cap(s)} 级`;
        const growth = { label: s.canRebirth ? '查看转生' : '成长详情', enabled: true, run: () => this.commands.growthDetails() };
        // 三个固定按钮留给确认动作，系别较多时在现有详情列表切换。
        if (s.rules.tracks.length > 2) {
            const selected = this.track(s, view.track ?? 0) ?? s.rules.tracks[0];
            const options = s.rules.tracks.map(track => ({
                key: String(track.id), label: `${track.id === selected.id ? '✓ ' : ''}${trackLabel(track)}`,
                enabled: true, choose: () => { view.track = track.id; view.notice = ''; },
            }));
            return { title: '炼体', lines: [...summary, ...limits], options, optionsFirst: true, actions: [
                { label: '查看报价', enabled: !s.blocked && rank(s, selected.id) < cap(s), run: () => this.readQuote(view, selected.id) },
                growth,
            ] };
        }
        const actions = s.rules.tracks.map(track => ({ label: track.name + '炼体', enabled: !s.blocked && rank(s, track.id) < cap(s),
            run: () => this.readQuote(view, track.id) }));
        return { title: '炼体', lines: [...summary, ...s.rules.tracks.map(trackLabel), ...limits], actions: [...actions, growth] };
    }

    private overview(view: View): void {
        if (!this.active(view)) return;
        view.quote = undefined;
        view.receipt = undefined;
        view.notice = '';
        this.commands.refresh();
    }

    private track(s: BodyTrainingSnapshot, id: number): Track | undefined {
        return s.rules.tracks.find(track => track.id === id);
    }

    private validQuote(view: View, q: TrainingQuote, s: BodyTrainingSnapshot): boolean {
        const t = q?.terms;
        if (!validTerms(t) || !validId(q.quoteId) || !same(q.uiSessionId, view.session) || !Long.isLong(q.sceneEpoch) ||
            !q.sceneEpoch.equals(s.sceneEpoch) || !Long.isLong(q.roleRevision) || q.roleRevision.isZero() ||
            q.roleRevision.greaterThan(s.revision) || !Long.isLong(q.expiresAtUnixMs) || q.expiresAtUnixMs.lessThanOrEqual(Long.fromNumber(Date.now(), true))) return false;
        const track = this.track(s, t.track), before = rank(s, t.track), step = s.rules.steps[before];
        return !!track && !!step && before < cap(s) && t.beforeLevel === before && step.from === before && t.successPpm === step.success_ppm &&
            quoteCostMatches(t, track, step) &&
            t.beforeBonus.equals(Long.fromNumber(track.bonus_per_rank).multiply(before)) &&
            t.targetBonus.equals(Long.fromNumber(track.bonus_per_rank).multiply(before + 1));
    }

    private async readQuote(view: View, track: number): Promise<void> {
        const s = this.active(view);
        if (!s || s.blocked || rank(s, track) >= cap(s)) return;
        view.notice = '';
        view.quote = undefined;
        try {
            const q = await this.commands.quote({ sceneEpoch: s.sceneEpoch, uiSessionId: view.session, track });
            const current = this.active(view);
            if (!current) return;
            if (q.terms.track !== track || !this.validQuote(view, q, current)) throw new Error('炼体报价已变化，请重新查看');
            view.quote = q;
        } catch (error) {
            if (!this.active(view)) return;
            view.notice = error instanceof Error ? error.message : '炼体报价未确认';
        }
        this.commands.refresh();
    }

    private async commit(view: View, quote: TrainingQuote): Promise<void> {
        const s = this.active(view);
        if (!s || s.blocked || view.quote !== quote || !this.validQuote(view, quote, s)) return;
        const request = { quoteId: new Uint8Array(quote.quoteId), uiSessionId: new Uint8Array(view.session), track: quote.terms.track };
        try { await this.commands.submit(request, quote.roleRevision); }
        catch (error) {
            const code = (error as { code?: number })?.code;
            if (![CodeBodyTrainingUnavailable, CodeBodyTrainingQuoteChanged, CodeBodyTrainingInputs, CodeOperationConflict].some(known => known === code)) throw error;
            if (!this.active(view) || view.quote !== quote) return;
            view.quote = undefined;
            view.notice = error instanceof Error ? error.message : '炼体条件已变化';
            this.commands.refresh();
        }
    }
}

// 校验单次阶梯、币种、正费用和递增的 int64 属性额度。
function validTerms(t: TrainingTerms | undefined): t is TrainingTerms {
    if (!t || !knownTrack(t.track) || !amount(t.gold) || !amount(t.boundGold) ||
        !amount(t.yuanbaoUnits ?? Long.ZERO) || !amount(t.boundYuanbaoUnits ?? Long.ZERO)) return false;
    const hasGold = !t.gold.isZero() || !t.boundGold.isZero();
    const hasYuanbao = !(t.yuanbaoUnits ?? Long.ZERO).isZero() || !(t.boundYuanbaoUnits ?? Long.ZERO).isZero();
    return hasYuanbao === yuanbaoTrack(t.track) && hasGold !== hasYuanbao &&
        Number.isInteger(t.beforeLevel) && t.beforeLevel >= 0 && t.beforeLevel < 0xffffffff &&
        t.targetLevel === t.beforeLevel + 1 && t.successPpm === 1000000 - t.beforeLevel % 10 * 100000 &&
        amount(t.beforeBonus) && amount(t.targetBonus) && t.targetBonus.greaterThan(t.beforeBonus);
}

// 用整数最小单位核对报价分账，保留原金币回执缺少元宝字段的兼容。
function quoteCostMatches(t: TrainingTerms, track: Track, step: Step): boolean {
    const ingot = yuanbaoTrack(t.track);
    const base = ingot ? track.yuanbao_base_units : track.gold_base;
    if (base === undefined || !Number.isSafeInteger(base) || base <= 0) return false;
    const ordinary = ingot ? t.yuanbaoUnits ?? Long.ZERO : t.gold;
    const bound = ingot ? t.boundYuanbaoUnits ?? Long.ZERO : t.boundGold;
    return BigInt(ordinary.toString()) + BigInt(bound.toString()) === BigInt(base) * BigInt(step.cost_multiplier);
}

// 分两行显示实际绑定和非绑定扣款，沿用项目的元宝精度。
function costLines(t: TrainingTerms): string[] {
    if (yuanbaoTrack(t.track)) return [`绑定元宝 ${yuanbaoText(t.boundYuanbaoUnits ?? Long.ZERO)}`, `普通元宝 ${yuanbaoText(t.yuanbaoUnits ?? Long.ZERO)}`];
    return [`绑定金币 ${t.boundGold}`, `普通金币 ${t.gold}`];
}

// 生命与魔法显示点数，其余系别按基点显示百分比。
function bonusText(track: number, value: Long): string {
    return track === healthTrack || track === manaTrack ? value.toString() : `${value.toNumber() / 100}%`;
}
