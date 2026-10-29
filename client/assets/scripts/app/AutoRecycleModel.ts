import Long from 'long';

export interface AutoRecyclePolicy {
    enabled: boolean; qualityIds: number[]; maxRequiredLevel: number; protectedItemIds: number[];
    protectBound: boolean; protectUnbound: boolean; revision: Long;
}
export interface RecycleItem { instanceId: Uint8Array; itemId: number; quantity: Long; bound: boolean; version: Long; quality: number; locked?: boolean; recycleBlockFlags?: number; }
export interface RecycleDefinition { item_id: number; name: string; kind: string; required_level?: number; recycle_gold?: number; quality?: number; }
export interface AutoRecycleSnapshot {
    ready: boolean; role: string; scope: string; rule: Long; revision: Long; policy: AutoRecyclePolicy | null;
    items: readonly RecycleItem[]; equipped: readonly { item?: RecycleItem | null }[]; definitions: readonly RecycleDefinition[];
    recycling?: { mode: string; outputs: readonly unknown[] };
}
export interface AutoRecyclePreview {
    readonly scope: string; readonly role: string; readonly inventoryRevision: string; readonly rule: string;
    readonly policy: AutoRecyclePolicy; readonly applyExisting: boolean; readonly count: number;
    readonly gold: bigint; readonly boundGold: bigint; readonly names: readonly string[];
    readonly skipped: readonly string[];
}
export const RECYCLE_QUALITY_NAMES = ['普通', '精良', '稀有', '史诗', '传说'];
function uint(value: unknown): value is number { return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 && value <= 0xffffffff; }
function integer(value: Long, label: string): bigint {
    if (!Long.isLong(value) || value.isNegative()) throw new Error(`${label}必须保持非负Long`);
    return BigInt(value.toString());
}
function ids(value: unknown, quality: boolean): number[] {
    if (!Array.isArray(value) || value.some(id => !uint(id) || (quality ? id > 4 : id === 0)) || new Set(value).size !== value.length) throw new Error('自动回收筛选ID无效或重复');
    return value.slice().sort((a, b) => a - b);
}
export function observedAutoRecycle(value: AutoRecyclePolicy): AutoRecyclePolicy {
    if (!value || typeof value.enabled !== 'boolean' || typeof value.protectBound !== 'boolean' || typeof value.protectUnbound !== 'boolean' || !uint(value.maxRequiredLevel)) throw new Error('自动回收策略格式无效');
    integer(value.revision, '策略修订');
    return { enabled: value.enabled, qualityIds: ids(value.qualityIds, true), maxRequiredLevel: value.maxRequiredLevel,
        protectedItemIds: ids(value.protectedItemIds, false), protectBound: value.protectBound, protectUnbound: value.protectUnbound,
        revision: Long.fromString(value.revision.toString(), true) };
}
export function defaultAutoRecycle(): AutoRecyclePolicy { return { enabled: false, qualityIds: [], maxRequiredLevel: 0, protectedItemIds: [], protectBound: false, protectUnbound: false, revision: Long.UZERO }; }
export function protectedEquipmentNames(value: readonly number[], definitions: readonly RecycleDefinition[]): string {
    return value.map(id => { const row = definitions.find(item => item.item_id === id && item.kind === 'equipment');
        if (!row) throw new Error('已保护装备类型缺少名称定义，请等待资料同步'); return row.name; }).join('，');
}
export function parseProtectedEquipmentNames(text: string, definitions: readonly RecycleDefinition[]): number[] {
    if (!text.trim()) return [];
    const result = text.split(/[,，]/).map(name => {
        const matches = definitions.filter(row => row.kind === 'equipment' && row.name === name.trim());
        if (matches.length !== 1) throw new Error(matches.length ? '装备名称有歧义，请等待配置修正：' + name.trim() : '未知装备名称：' + name.trim());
        return matches[0].item_id;
    });
    return ids(result, false);
}
export function recycleItemKey(value: Uint8Array): string {
    if (!(value instanceof Uint8Array) || value.length !== 16) throw new Error('装备实例ID必须为16字节');
    return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
}
export function manualRecycleBlockReason(item: RecycleItem, equipped = false): string {
    const flags = item.recycleBlockFlags ?? 0;
    if (!uint(flags) || flags > 15) return '装备保护状态尚未识别';
    if (item.locked || (flags & 1)) return '装备已锁定';
    if (flags & 2) return '装备有未确认预览';
    if (flags & 4) return '装备正在被占用';
    if (equipped || (flags & 8)) return '装备正在穿戴';
    return '';
}
export function autoRecycleMatches(policy: AutoRecyclePolicy, item: RecycleItem, definition: RecycleDefinition | undefined, equipped = false): boolean {
    if (!policy.enabled || !definition || definition.kind !== 'equipment' || manualRecycleBlockReason(item, equipped)
        || !policy.qualityIds.includes(item.quality) || policy.protectedItemIds.includes(item.itemId)
        || (item.bound ? policy.protectBound : policy.protectUnbound)) return false;
    if (!uint(definition.required_level) || definition.required_level < 1 || !uint(definition.quality) || definition.quality > 4 || definition.quality !== item.quality
        || !Number.isSafeInteger(definition.recycle_gold) || definition.recycle_gold! < 0) return false;
    return policy.maxRequiredLevel === 0 || definition.required_level <= policy.maxRequiredLevel;
}
export function createAutoRecyclePreview(snapshot: AutoRecycleSnapshot, value: AutoRecyclePolicy, applyExisting: boolean): AutoRecyclePreview {
    if (!snapshot.ready || !snapshot.role || !snapshot.scope || !snapshot.policy) throw new Error('自动回收资料尚未同步');
    const policy = observedAutoRecycle(value);
    if (!snapshot.recycling || snapshot.recycling.mode !== 'development_gold_only' || !Array.isArray(snapshot.recycling.outputs) || snapshot.recycling.outputs.length !== 0) throw new Error('当前分解配置尚未完整同步');
    if (applyExisting && !policy.enabled) throw new Error('应用当前背包前需明确开启策略；关闭请使用仅保存');
    if (!policy.revision.equals(snapshot.policy.revision)) throw new Error('自动回收设置已变化，请恢复服务器设置后重试');
    if (policy.protectedItemIds.some(id => !snapshot.definitions.some(row => row.item_id === id && row.kind === 'equipment'))) throw new Error('保护类型必须来自当前装备配置');
    integer(snapshot.revision, '背包修订'); integer(snapshot.rule, '规则版本');
    if (typeof applyExisting !== 'boolean') throw new Error('需要明确当前背包处理方式');
    const equipped = new Set(snapshot.equipped.filter(row => row.item).map(row => recycleItemKey(row.item!.instanceId)));
    let gold = 0n, boundGold = 0n, count = 0; const names: string[] = [], skipped: string[] = [];
    if (applyExisting) for (const item of snapshot.items) {
        const definition = snapshot.definitions.find(row => row.item_id === item.itemId);
        if (!autoRecycleMatches(policy, item, definition, equipped.has(recycleItemKey(item.instanceId)))) {
            if (!definition || definition.kind === 'equipment') skipped.push(`${definition?.name ?? '未识别装备'}：${manualRecycleBlockReason(item, equipped.has(recycleItemKey(item.instanceId)))
                || (policy.protectedItemIds.includes(item.itemId) ? '按类型保护' : (item.bound ? policy.protectBound : policy.protectUnbound) ? '按绑定状态保护'
                    : !uint(definition?.required_level) || definition.required_level < 1 ? '需求等级未知' : definition?.quality !== item.quality || !Number.isSafeInteger(definition?.recycle_gold) ? '缺少当前品质分解定义' : '未命中品质或等级筛选')}`);
            continue;
        }
        if (integer(item.quantity, '装备数量') !== 1n) { skipped.push(`${definition!.name}：装备数量状态不支持回收`); continue; }
        const earned = BigInt(definition!.recycle_gold!); if (item.bound) boundGold += earned; else gold += earned;
        ++count; names.push(definition!.name);
    }
    Object.freeze(policy.qualityIds); Object.freeze(policy.protectedItemIds); Object.freeze(policy);
    return Object.freeze({ scope: snapshot.scope, role: snapshot.role, inventoryRevision: snapshot.revision.toString(), rule: snapshot.rule.toString(),
        policy, applyExisting, count, gold, boundGold, names: Object.freeze(names), skipped: Object.freeze(skipped) });
}
export function validateAutoRecyclePreview(preview: AutoRecyclePreview, snapshot: AutoRecycleSnapshot): void {
    if (!snapshot.ready || preview.scope !== snapshot.scope || preview.role !== snapshot.role || preview.inventoryRevision !== snapshot.revision.toString()
        || preview.rule !== snapshot.rule.toString() || !snapshot.policy || !preview.policy.revision.equals(snapshot.policy.revision)) throw new Error('自动回收确认期间资料已变化，请重新确认');
}
export function autoRecycleConfirmation(preview: AutoRecyclePreview): string {
    const mode = preview.applyExisting ? `同时处理当前背包：预计回收${preview.count}件\n金币 ${preview.gold}，绑定金币 ${preview.boundGold}` : '仅保存策略，不立即回收当前背包';
    // 保存Confirm的Label不自动换行；只按原因汇总重复项，保留全部保护/缺定义原因。
    const reasonNames: Record<string, string> = { '装备已锁定': '锁定', '装备有未确认预览': '未确认预览', '装备正在被占用': '占用', '装备正在穿戴': '穿戴',
        '装备保护状态尚未识别': '保护未知', '按类型保护': '类型保护', '按绑定状态保护': '绑定保护', '需求等级未知': '等级未知',
        '缺少当前品质分解定义': '品质缺配方', '未命中品质或等级筛选': '筛选不符', '装备数量状态不支持回收': '数量异常' };
    const reasons = Array.from(new Set(preview.skipped.map(value => { const reason = value.includes('：') ? value.slice(value.indexOf('：') + 1) : value; return reasonNames[reason] ?? reason; })));
    const text = `${preview.policy.enabled ? '开启' : '关闭'}自动回收\n${mode}${reasons.length ? `\n跳过${preview.skipped.length}件：${reasons.join('；')}` : ''}\n穿戴、锁定、未确认预览及占用均保护。\n回收不可撤销，以服务器提交结果为准。`;
    const lines = text.split('\n').flatMap(line => { const characters = Array.from(line), result: string[] = []; for (let offset = 0; offset < characters.length; offset += 19) result.push(characters.slice(offset, offset + 19).join('')); return result; });
    // 未知将来原因仍完整保留；超出保存窗口时拒绝确认，不能截断风险说明后继续发送。
    if (lines.length > 8) throw new Error('回收说明超出确认窗口，请减少当前处理范围后重试');
    return lines.join('\n');
}
export function autoRecyclePolicyText(policy: AutoRecyclePolicy | null): string {
    if (!policy) return '等待服务端自动回收设置';
    return `已生效：${policy.enabled ? '开启' : '关闭'} · 修订${policy.revision}\n品质：${policy.qualityIds.map(id => RECYCLE_QUALITY_NAMES[id]).join('、') || '未选择'} · 需求等级${policy.maxRequiredLevel === 0 ? '不限' : '≤' + policy.maxRequiredLevel}`;
}
export function recycleDiscardText(reason: string): string { return reason === 'bag_full' ? '背包已满，自动回收后仍无空间' : reason ? '其它服务器丢弃原因（待核验）' : ''; }
export function autoRecycleReportLines(report: any): string[] {
    if (!report) return ['暂无已提交托管战报'];
    const value = (key: string): bigint => report[key] === undefined ? 0n : integer(report[key], '战报' + key);
    const splitKnown = report.rawExperience !== undefined && value('rawExperience') === value('experience') + value('rejectedExperience') && value('pooledExperience') <= value('experience');
    return [`托管 ${value('usedSeconds')} 秒，击杀 ${value('kills')}，经验 ${value('experience')}`,
        ...(splitKnown ? [`原经验 ${value('rawExperience')} · 实际接收 ${value('experience')}`, `计入待用池 ${value('pooledExperience')} · 未接收 ${value('rejectedExperience')}`] : report.rawExperience !== undefined ? ['旧战报未提供完整经验分账，不据此推算'] : []),
        `金币 ${report.earned?.gold?.toString() ?? '0'}，绑定金币 ${report.earned?.boundGold?.toString() ?? '0'}`,
        `自动回收 ${value('recycleCount')} 件 · 金币 ${value('recycleGold')}，绑定金币 ${value('recycleBoundGold')}`,
        `总掉落 ${value('totalDropCount')} 件 · 放弃 ${value('discardCount')} 件${report.lastDiscardReason ? '，原因：' + recycleDiscardText(report.lastDiscardReason) : ''}`,
        `停止状态：${({ 0: '仍可继续', 1: '托管时长耗尽', 2: '角色倒下', 3: '背包无可用空间' } as Record<number, string>)[report.stopReason ?? 0] ?? '服务器已停止托管'} · 掉落列表最多100条，仅为样本`, '以上均为原托管区间已提交事实，不重新判定绑定'];
}
export interface RecycleNotice { readonly role: string; readonly revision: string; readonly identity: string; readonly trigger: number;
    readonly count: string; readonly gold: string; readonly boundGold: string; readonly discarded: string; readonly reason: string; }
// 仅记录当前附着看到的已提交通知；首份Inventory建立水位，健康重入不重播历史。
export class EquipmentRecycleModel {
    private role = '';
    private highWater = 0n;
    private firstInventory = true;
    notice: RecycleNotice | null = null;
    attach(role: string): void {
        if (this.role !== role) { this.role = role; this.highWater = 0n; }
        this.firstInventory = true; this.notice = null;
    }
    synchronize(revision: Long): void { if (this.firstInventory) { this.firstInventory = false; const value = integer(revision, '角色修订'); if (value > this.highWater) this.highWater = value; } }
    detach(): void { this.notice = null; this.firstInventory = true; }
    accept(value: any, role: string): boolean {
        if (!value || role !== this.role || this.firstInventory || ![1, 2, 3].includes(value.trigger)) return false;
        try {
            const incoming = `${integer(value.role.originServerId, '入口服')}:${recycleItemKey(value.role.roleId)}`;
            if (incoming !== role) return false;
            const revision = integer(value.roleRevision, '提交修订'), source = integer(value.sourceSeq, '来源序号');
            integer(value.policyRevision, '策略修订');
            if (revision === 0n || revision <= this.highWater || source > revision) return false;
            const op = value.opId;
            if (!(op instanceof Uint8Array) || (value.trigger === 1 ? source === 0n || op.length !== 0 : source !== 0n || op.length !== 16)) return false;
            const count = integer(value.recycledCount, '回收数量'), gold = integer(value.gold, '回收金币'), bound = integer(value.boundGold, '回收绑定金币'), discarded = integer(value.discardedCount, '放弃数量');
            if (typeof value.discardReason !== 'string') return false;
            this.highWater = revision;
            this.notice = Object.freeze({ role, revision: revision.toString(), identity: value.trigger === 1 ? 'source:' + source : 'op:' + recycleItemKey(op), trigger: value.trigger,
                count: count.toString(), gold: gold.toString(), boundGold: bound.toString(), discarded: discarded.toString(), reason: value.discardReason });
            return true;
        } catch { return false; }
    }
}
