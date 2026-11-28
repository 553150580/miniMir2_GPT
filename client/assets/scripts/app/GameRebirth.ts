import Long from 'long';
import { CodeOperationConflict, CodeRebirthInputs, CodeRebirthQuoteChanged, CodeRebirthUnavailable } from '../generated/protocol_ids';
import { assetId } from './SkillAssetsModel';
import { progressionDetailLines } from './LevelGrowthModel';
import type { ProgressionState } from './LevelGrowthModel';
import type { ItemActionView } from './GameItemActions';

// 报价面板使用原角色、原场景和原成长状态。
export interface RebirthSnapshot {
    scope: string; role: string; sceneEpoch: Long; revision: Long; level: number; experience: Long;
    progression: ProgressionState; stages: readonly RebirthStage[]; blocked: boolean;
    items: readonly { instanceId: Uint8Array; itemId: number; version: Long; quantity: Long; bound: boolean; locked: boolean }[];
}
interface RebirthStage {
    required_level: number; challenge_name: string; credential_item_id: number; material_item_id: number;
    material_cost: number; gold_cost: number;
}
export interface RebirthCommitRequest { quoteId: Uint8Array; uiSessionId: Uint8Array; }
interface RebirthQuoteRequest { sceneEpoch: Long; uiSessionId: Uint8Array; }
interface RebirthSpend { instanceId: Uint8Array; itemId: number; version: Long; quantity: Long; }
interface RebirthTransition {
    previousCap: number; newCap: number; previousLevel: number; newLevel: number; newCount: number;
    releasedExperience: Long; newExperience: Long;
}
export interface RebirthReceipt {
    quoteId: Uint8Array; challengeId: Uint8Array; transition: RebirthTransition;
    credential: RebirthSpend; materials: RebirthSpend[]; gold: Long; boundGold: Long;
}
interface RebirthQuote {
    preview: RebirthReceipt; sceneEpoch: Long; roleRevision: Long; expiresAtUnixMs: Long; uiSessionId: Uint8Array;
}
type ReadView = () => ItemActionView | null;
interface RebirthCommands {
    snapshot(): RebirthSnapshot | null;
    newSession(): Uint8Array;
    open(read: ReadView): void;
    current(read: ReadView): boolean;
    refresh(): void;
    quote(request: RebirthQuoteRequest): Promise<RebirthQuote>;
    submit(request: RebirthCommitRequest, revision: Long): Promise<void>;
}
interface RebirthView {
    scope: string; role: string; session: Uint8Array; read: ReadView; notice: string;
    quote?: RebirthQuote; submitted?: RebirthCommitRequest; receipt?: RebirthReceipt;
}

// 回执在当前面板按完整报价核对，重新连接后仍校验原报价身份和金额形状。
export function validateRebirthReceipt(request: RebirthCommitRequest, receipt: RebirthReceipt, expected?: RebirthReceipt): RebirthReceipt {
    const t = receipt?.transition;
    if (!sameId(request.quoteId, receipt?.quoteId) || !validId(receipt?.challengeId) || !t ||
        !Number.isInteger(t.newCount) || t.newCount < 1 || t.previousCap !== 49 + t.newCount ||
        t.previousLevel !== t.previousCap || t.newCap !== t.previousCap + 1 || ![t.previousCap, t.newCap].includes(t.newLevel) ||
        !amount(t.releasedExperience) || !amount(t.newExperience) ||
        (t.newLevel === t.previousLevel ? !t.newExperience.equals(t.releasedExperience) : !t.newExperience.isZero() || t.releasedExperience.isZero()) ||
        !amount(receipt.gold) || !amount(receipt.boundGold) || (receipt.gold.isZero() && receipt.boundGold.isZero()) ||
        !Array.isArray(receipt.materials) || !receipt.materials.length) throw new Error('转生回执有误，保留原操作核对');
    const instances = new Set<string>();
    for (const line of [receipt.credential, ...receipt.materials]) {
        if (!validId(line?.instanceId) || !Number.isInteger(line.itemId) || line.itemId <= 0 ||
            !Long.isLong(line.version) || line.version.isZero() || line.version.isNegative() ||
            !amount(line.quantity) || line.quantity.isZero() || instances.has(assetId(line.instanceId))) {
            throw new Error('转生材料回执有误，保留原操作核对');
        }
        instances.add(assetId(line.instanceId));
    }
    if (!receipt.credential.quantity.equals(Long.ONE) || receipt.materials.some(line => line.itemId === receipt.credential.itemId) ||
        (expected && receiptKey(expected) !== receiptKey(receipt))) throw new Error('转生回执与原报价不符');
    return receipt;
}

// 使用保存的角色成长入口和通用详情面板，保留原场景资源。
export class GameRebirth {
    private view?: RebirthView; // 当前打开的角色成长会话。

    constructor(private readonly commands: RebirthCommands) {}

    open(): void {
        const s = this.commands.snapshot();
        if (!s) return;
        const view: RebirthView = { scope: s.scope, role: s.role, session: this.commands.newSession(), notice: '',
            read: () => this.render(view) };
        this.view = view;
        this.commands.open(view.read);
    }

    reconcile(request: RebirthCommitRequest, receipt: RebirthReceipt): RebirthReceipt {
        const view = this.view, submitted = view?.submitted;
        const expected = submitted && sameId(request.quoteId, submitted.quoteId) &&
            sameId(request.uiSessionId, submitted.uiSessionId) ? view?.quote?.preview : undefined;
        return validateRebirthReceipt(request, receipt, expected);
    }

    accept(role: string, request: RebirthCommitRequest, receipt: RebirthReceipt): void {
        const view = this.view;
        if (!view?.submitted || role !== view.role || !sameId(view.submitted.quoteId, request.quoteId) || !this.active(view)) return;
        view.receipt = receipt;
        view.submitted = undefined;
        view.quote = undefined;
        this.commands.refresh();
    }

    private render(view: RebirthView): ItemActionView | null {
        const s = this.scoped(view);
        if (!s) return null;
        if (view.receipt) return { title: '转生完成', lines: receiptLines(view.receipt),
            actions: [{ label: '查看当前成长', enabled: true, run: () => { view.receipt = undefined; this.commands.refresh(); } }] };
        const stage = s.stages[s.progression.rebirthCount];
        const eligible = !!stage && s.level === stage.required_level && s.progression.atCap && !s.progression.migrationRequired;
        if (view.quote && stage) {
            const quote = view.quote, exact = validQuote(quote, view.session, s, stage);
            return { title: '转生报价', lines: [...(!exact ? ['报价已变化，请重新查看'] : []), ...receiptLines(quote.preview)],
                actions: [{ label: exact ? '确认转生' : '重新报价', enabled: eligible && !s.blocked,
                    run: () => exact ? this.commit(view, quote) : this.readQuote(view) }] };
        }
        const lines = [`当前等级 ${s.level} · 上限 ${s.progression.levelCap}`, `已转生 ${s.progression.rebirthCount} 次`,
            `待用经验 ${s.progression.pendingExperience} / ${s.progression.pendingExperienceLimit}`];
        if (stage) {
            lines.push(`达到 ${stage.required_level} 级可开放上限 ${stage.required_level + 1} 级`,
                `凭证来源 · 个人首领 ${stage.challenge_name}`, `金币 ${stage.gold_cost} · 绑定材料 ${stage.material_cost} 份`);
        } else lines.push('已完成当前开放档位');
        if (view.notice) lines.unshift(view.notice);
        lines.push(...progressionDetailLines(s.level, s.experience, s.progression).slice(2, 4));
        return { title: '角色成长', lines, actions: stage ?
            [{ label: '查看转生报价', enabled: eligible && !s.blocked, run: () => this.readQuote(view) }] : [] };
    }

    private scoped(view: RebirthView): RebirthSnapshot | undefined {
        const snapshot = this.commands.snapshot();
        return this.view === view && snapshot?.scope === view.scope && snapshot.role === view.role ? snapshot : undefined;
    }

    private active(view: RebirthView): RebirthSnapshot | undefined {
        return this.commands.current(view.read) ? this.scoped(view) : undefined;
    }

    private async readQuote(view: RebirthView): Promise<void> {
        const s = this.active(view), stage = s?.stages[s.progression.rebirthCount];
        if (!s || s.blocked || !stage) return;
        view.quote = undefined;
        view.notice = '';
        try {
            const quote = await this.commands.quote({ sceneEpoch: s.sceneEpoch, uiSessionId: view.session });
            const current = this.active(view);
            if (!current) return;
            if (!validQuote(quote, view.session, current, stage)) throw new Error('转生报价已变化，请重新查看');
            view.quote = quote;
        } catch (error) {
            if (!this.active(view)) return;
            view.notice = error instanceof Error ? error.message : '转生报价未确认';
        }
        this.commands.refresh();
    }

    private async commit(view: RebirthView, quote: RebirthQuote): Promise<void> {
        const s = this.active(view), stage = s?.stages[s.progression.rebirthCount];
        if (!s || !stage || s.blocked || view.quote !== quote || !validQuote(quote, view.session, s, stage)) return;
        const request = { quoteId: new Uint8Array(quote.preview.quoteId), uiSessionId: new Uint8Array(view.session) };
        view.submitted = request;
        try { await this.commands.submit(request, quote.roleRevision); }
        catch (error) {
            const code = (error as { code?: number })?.code;
            if (![CodeRebirthUnavailable, CodeRebirthQuoteChanged, CodeRebirthInputs, CodeOperationConflict].some(known => known === code)) throw error;
            if (!this.active(view) || view.submitted !== request) return;
            view.submitted = undefined;
            view.quote = undefined;
            view.notice = error instanceof Error ? error.message : '投入已变化，请重新报价';
            this.commands.refresh();
        }
    }
}

// 报价必须仍对应原面板、成长余额和每个可扣原堆。
function validQuote(quote: RebirthQuote, session: Uint8Array, s: RebirthSnapshot, stage: RebirthStage): boolean {
    try {
        if (!sameId(quote.uiSessionId, session) || !quote.sceneEpoch.equals(s.sceneEpoch) || quote.roleRevision.isZero() ||
            quote.roleRevision.greaterThan(s.revision) || quote.expiresAtUnixMs.lessThanOrEqual(Long.fromNumber(Date.now(), true))) return false;
        const value = validateRebirthReceipt({ quoteId: quote.preview?.quoteId, uiSessionId: session }, quote.preview);
        if (value.transition.previousLevel !== s.level || value.transition.previousCap !== s.progression.levelCap ||
            value.transition.newCount !== s.progression.rebirthCount + 1 || !value.transition.releasedExperience.equals(s.progression.pendingExperience) ||
            !value.gold.add(value.boundGold).equals(Long.fromNumber(stage.gold_cost)) ||
            value.credential.itemId !== stage.credential_item_id ||
            value.materials.some(line => line.itemId !== stage.material_item_id) ||
            !value.materials.reduce((sum, line) => sum.add(line.quantity), Long.ZERO).equals(Long.fromNumber(stage.material_cost))) return false;
        return [value.credential, ...value.materials].every(line => s.items.some(item => sameId(item.instanceId, line.instanceId) &&
            item.itemId === line.itemId && item.bound && !item.locked && item.version.equals(line.version) && item.quantity.greaterThanOrEqual(line.quantity)));
    } catch { return false; }
}

function validId(value: unknown): value is Uint8Array { return value instanceof Uint8Array && value.length === 16; }
function sameId(left: unknown, right: unknown): boolean { return validId(left) && validId(right) && left.every((byte, i) => byte === right[i]); }
function amount(value: unknown): value is Long { return Long.isLong(value) && !value.unsigned && !value.isNegative(); }
function receiptKey(value: RebirthReceipt): string {
    const t = value.transition;
    return [assetId(value.challengeId), t.previousCap, t.newCap, t.previousLevel, t.newLevel, t.newCount, t.releasedExperience.toString(),
        t.newExperience.toString(), value.gold.toString(), value.boundGold.toString(), ...[value.credential, ...value.materials]
            .map(line => [assetId(line.instanceId), line.itemId, line.version.toString(), line.quantity.toString()].join('/'))].join('|');
}
function receiptLines(value: RebirthReceipt): string[] {
    const t = value.transition;
    return [`等级上限 ${t.previousCap} → ${t.newCap}级`, `实际等级 ${t.previousLevel} → ${t.newLevel}级`,
        `释放待用经验 ${t.releasedExperience}`, `扣除绑定金币 ${value.boundGold} · 金币 ${value.gold}`,
        `转生凭证 1 份 · 尾号 ${assetId(value.credential.instanceId).slice(-6)}`,
        ...value.materials.map(line => `绑定材料 ${line.quantity} 份 · 尾号 ${assetId(line.instanceId).slice(-6)}`)];
}
