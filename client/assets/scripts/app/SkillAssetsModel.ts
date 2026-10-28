import { PotionInteger, PotionItem } from './PotionModel';

export interface SkillAssetItem extends PotionItem { sourceKind?: string; sourceId?: string; quality?: number; }
export interface SkillAssetDefinition { item_id: number; name: string; kind: string; skill_id?: number; }
export interface ShopOffer {
    offer_id: number; item_id: number; name: string; quantity: number; currency_kind: 'gold' | 'ingot' | 'bound_ingot';
    price_units: number; required_role_level: number; output_bound: boolean; max_purchase_quantity: number;
}
export interface SkillUpgradeRule {
    skill_id: number; level: number; required_role_level: number; book_costs: readonly { item_id: number; quantity: number }[];
    page_item_id: number; page_cost: number;
}
export interface SkillBookRecycleRule { item_id: number; book_quality: number; page_item_id: number; pages_per_book: number; allowed_source_kinds: readonly string[]; }
export interface AssetWallet { revision: PotionInteger; gold: PotionInteger; boundGold: PotionInteger; yuanbaoUnits: PotionInteger; boundYuanbaoUnits: PotionInteger; }
export interface SkillAssetsSnapshot {
    ready: boolean; roleIdentity: string; sceneEpoch: PotionInteger; ruleVersion: PotionInteger;
    inventory: { revision: PotionInteger; level: number; items: readonly SkillAssetItem[]; learnedSkills?: readonly { skillId: number; level: number }[] } | null;
    wallet: AssetWallet | null; definitions: readonly SkillAssetDefinition[];
    skills: readonly { id: number; name: string; job_id: number; max_level: number }[];
    offers: readonly ShopOffer[]; upgrades: readonly SkillUpgradeRule[]; recycle: readonly SkillBookRecycleRule[];
}
export interface MaterialChoice { instanceId: Uint8Array; version: PotionInteger; quantity: bigint; }
export interface SkillAssetQuote {
    previewId: Uint8Array; kind: number; ruleVersion: PotionInteger; configHash: string; roleRevision: PotionInteger; expiresAtUnixMs: PotionInteger;
    skillId: number; fromLevel: number; toLevel: number; offerId: number; quantity: PotionInteger;
    spend: AssetWallet; materials: readonly { item: SkillAssetItem; consumeQuantity: PotionInteger }[]; outputs: readonly SkillAssetItem[];
    stats: readonly { key: string; beforeMin: PotionInteger; beforeMax: PotionInteger; afterMin: PotionInteger; afterMax: PotionInteger }[];
}
interface FrozenItem { id: string; version: string; quantity: string; bound: boolean; sourceKind: string; sourceId: string; quality: number; }
export interface SkillAssetContext {
    roleIdentity: string; sceneEpoch: string; ruleVersion: string; revision: string; walletRevision: string;
    wallet: readonly string[]; materials: readonly FrozenItem[];
}

export function assetInteger(value: PotionInteger): bigint {
    if (typeof value === 'number' && !Number.isSafeInteger(value)) throw new Error('资产数量必须保持整数精度');
    const text = value.toString(); if (!/^(?:0|[1-9]\d*)$/.test(text)) throw new Error('资产数量无效'); return BigInt(text);
}
export function assetId(value: Uint8Array): string {
    if (!(value instanceof Uint8Array) || value.length !== 16) throw new Error('资产实例标识无效');
    return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
}
export function sourceName(value: string | undefined): string {
    return ({ npc_shop: 'NPC书店', initial: '开发初始补给', initial_grant: '开发初始补给', monster_drop: '怪物掉落', skill_recycle: '拆书所得' } as Record<string, string>)[value ?? ''] ?? '未知来源';
}
function frozenItem(item: SkillAssetItem): FrozenItem {
    return { id: assetId(item.instanceId), version: assetInteger(item.version).toString(), quantity: assetInteger(item.quantity).toString(),
        bound: item.bound, sourceKind: item.sourceKind ?? '', sourceId: item.sourceId ?? '', quality: item.quality ?? 0 };
}
function walletValues(wallet: AssetWallet): string[] { return [wallet.gold, wallet.boundGold, wallet.yuanbaoUnits, wallet.boundYuanbaoUnits].map(value => assetInteger(value).toString()); }

// 选择记录属于原实例和原版本；库存变化时只移除失效选择，不替换到另一堆材料。
export class SkillMaterialSelection {
    private readonly choices = new Map<string, { version: string; quantity: bigint }>();
    quantity(item: SkillAssetItem): bigint { return this.choices.get(assetId(item.instanceId))?.quantity ?? 0n; }
    clear(): void { this.choices.clear(); }
    adjust(item: SkillAssetItem, delta: number): void {
        if (!Number.isSafeInteger(delta)) throw new Error('材料选择增量无效');
        const id = assetId(item.instanceId), version = assetInteger(item.version).toString();
        const previous = this.choices.get(id); if (previous && previous.version !== version) throw new Error('原材料版本已变化，请重新选择');
        const next = (previous?.quantity ?? 0n) + BigInt(delta), available = assetInteger(item.quantity);
        if (next < 0n || next > available) throw new Error('所选数量超出当前实例库存');
        if (next === 0n) this.choices.delete(id); else this.choices.set(id, { version, quantity: next });
    }
    reconcile(items: readonly SkillAssetItem[]): boolean {
        let changed = false;
        for (const [id, selected] of this.choices) {
            const item = items.find(value => assetId(value.instanceId) === id);
            if (!item || assetInteger(item.version).toString() !== selected.version || assetInteger(item.quantity) < selected.quantity) { this.choices.delete(id); changed = true; }
        }
        return changed;
    }
    selected(items: readonly SkillAssetItem[]): MaterialChoice[] {
        const result = items.flatMap(item => {
            const selected = this.choices.get(assetId(item.instanceId));
            if (!selected) return [];
            if (selected.version !== assetInteger(item.version).toString() || selected.quantity > assetInteger(item.quantity)) throw new Error('材料已变化，请重新选择');
            return [{ instanceId: new Uint8Array(item.instanceId), version: item.version, quantity: selected.quantity }];
        });
        if (result.length !== this.choices.size) throw new Error('原材料实例已离开背包，请重新选择');
        return result;
    }
}

export function canRecycleBook(item: SkillAssetItem, rules: readonly SkillBookRecycleRule[]): boolean {
    return assetInteger(item.quantity) > 0n && !!item.sourceId && rules.some(rule => rule.item_id === item.itemId && rule.book_quality === (item.quality ?? 0)
        && rule.allowed_source_kinds.includes(item.sourceKind ?? ''));
}
export function adjustPurchaseQuantity(current: bigint, delta: number, maximum: number): bigint {
    if (!Number.isSafeInteger(delta) || !Number.isSafeInteger(maximum) || maximum < 1) throw new Error('商品购买数量配置无效');
    const next = current + BigInt(delta);
    if (next < 1n || next > BigInt(maximum)) throw new Error(`购买数量须为1～${maximum}份`);
    return next;
}
export function upgradeRequirements(rule: SkillUpgradeRule): Map<number, bigint> {
    const result = new Map<number, bigint>();
    for (const row of rule.book_costs) result.set(row.item_id, (result.get(row.item_id) ?? 0n) + assetInteger(row.quantity));
    if (rule.page_cost) result.set(rule.page_item_id, (result.get(rule.page_item_id) ?? 0n) + assetInteger(rule.page_cost));
    return result;
}
export function materialSelectionMatches(rule: SkillUpgradeRule, choices: readonly MaterialChoice[], items: readonly SkillAssetItem[]): boolean {
    const totals = new Map<number, bigint>();
    for (const choice of choices) {
        const item = items.find(value => assetId(value.instanceId) === assetId(choice.instanceId));
        if (!item) return false; totals.set(item.itemId, (totals.get(item.itemId) ?? 0n) + choice.quantity);
    }
    const required = upgradeRequirements(rule);
    return totals.size === required.size && Array.from(required).every(([id, quantity]) => totals.get(id) === quantity);
}
export function captureSkillAssetContext(snapshot: SkillAssetsSnapshot, choices: readonly MaterialChoice[]): SkillAssetContext {
    if (!snapshot.ready || !snapshot.inventory || !snapshot.wallet || !snapshot.roleIdentity
        || assetInteger(snapshot.inventory.revision) !== assetInteger(snapshot.wallet.revision)) throw new Error('角色、背包与钱包尚未同步');
    const materials = choices.map(choice => {
        const item = snapshot.inventory!.items.find(value => assetId(value.instanceId) === assetId(choice.instanceId));
        if (!item || assetInteger(item.version) !== assetInteger(choice.version) || choice.quantity <= 0n || choice.quantity > assetInteger(item.quantity)) throw new Error('原材料已变化，请重新选择');
        return frozenItem(item);
    });
    return { roleIdentity: snapshot.roleIdentity, sceneEpoch: snapshot.sceneEpoch.toString(), ruleVersion: snapshot.ruleVersion.toString(),
        revision: snapshot.inventory.revision.toString(), walletRevision: snapshot.wallet.revision.toString(), wallet: walletValues(snapshot.wallet), materials };
}
export function validateSkillAssetContext(context: SkillAssetContext, quote: SkillAssetQuote, snapshot: SkillAssetsSnapshot, now: number): void {
    const changed = '报价或资产状态已变化，请重新预览';
    if (!snapshot.ready || !snapshot.inventory || !snapshot.wallet || snapshot.roleIdentity !== context.roleIdentity
        || snapshot.sceneEpoch.toString() !== context.sceneEpoch || snapshot.ruleVersion.toString() !== context.ruleVersion
        || quote.ruleVersion.toString() !== context.ruleVersion || quote.roleRevision.toString() !== context.revision
        || snapshot.inventory.revision.toString() !== context.revision || snapshot.wallet.revision.toString() !== context.walletRevision
        || JSON.stringify(walletValues(snapshot.wallet)) !== JSON.stringify(context.wallet)
        || !Number.isSafeInteger(now) || assetInteger(quote.expiresAtUnixMs) <= BigInt(now)) throw new Error(changed);
    assetId(quote.previewId);
    for (const material of context.materials) {
        const item = snapshot.inventory.items.find(value => assetId(value.instanceId) === material.id);
        if (!item || JSON.stringify(frozenItem(item)) !== JSON.stringify(material)) throw new Error(changed);
    }
}
export function validateQuotedMaterials(quote: SkillAssetQuote, choices: readonly MaterialChoice[], snapshot: SkillAssetsSnapshot): void {
    if (quote.materials.length !== choices.length) throw new Error('报价材料与原选择不一致，请重新预览');
    const seen = new Set<string>();
    for (const material of quote.materials) {
        const id = assetId(material.item.instanceId), choice = choices.find(value => assetId(value.instanceId) === id);
        const original = snapshot.inventory?.items.find(value => assetId(value.instanceId) === id);
        if (seen.has(id) || !choice || !original || assetInteger(material.consumeQuantity) !== choice.quantity
            || assetInteger(material.item.version) !== assetInteger(choice.version)
            || JSON.stringify(frozenItem(original)) !== JSON.stringify(frozenItem(material.item))) throw new Error('报价材料与原选择不一致，请重新预览');
        seen.add(id);
    }
    if (!snapshot.wallet) throw new Error('钱包尚未同步');
    const balance = walletValues(snapshot.wallet), spend = walletValues(quote.spend);
    if (spend.some((value, index) => BigInt(value) > BigInt(balance[index]))) throw new Error('报价支出超出当前钱包余额');
}

const STAT_NAMES: Record<string, string> = { accuracy_bonus: '准确加成', base_power: '技能基础威力', mp_cost: '魔法消耗', max_attack_bonus: '攻击上限加成',
    push_distance:'推开距离（格）',dash_distance:'冲撞距离（格）',push_base_success_percent:'基础成功率（%，另加等级差）',monster_damage:'对怪物伤害',
    duration_ms: '持续时间(毫秒)', teleport_mana_penalty_percent: '传送耗蓝加成(%)', damage_reduction_percent: '伤害减免（%）', duration_rank_factor: '持续时间阶数系数（非秒数）', base_duration_seconds: '基础持续时间（秒，另加道术项）', material_quantity: '每次施法护符用量' };
export function skillQuoteLines(quote: SkillAssetQuote, definitions: readonly SkillAssetDefinition[]): string[] {
    const name = (id: number): string => definitions.find(row => row.item_id === id)?.name ?? `物品${id}`;
    const lines: string[] = [];
    if (quote.kind === 2) lines.push(`技能等级 ${quote.fromLevel} → ${quote.toLevel}`);
    for (const stat of quote.stats) {
        lines.push(STAT_NAMES[stat.key] ?? '数值对照');
        if (stat.beforeMin.toString() === stat.beforeMax.toString()) lines.push(`当前值 ${stat.beforeMin}`);
        else lines.push(`当前下限 ${stat.beforeMin}`, `当前上限 ${stat.beforeMax}`);
        if (stat.afterMin.toString() === stat.afterMax.toString()) lines.push(`下一阶 ${stat.afterMin}`);
        else lines.push(`下一阶下限 ${stat.afterMin}`, `下一阶上限 ${stat.afterMax}`);
    }
    const spend = quote.spend;
    lines.push(`金币支出 ${spend.gold}`, `绑定金币支出 ${spend.boundGold}`,
        `元宝支出 ${yuanbaoText(spend.yuanbaoUnits)}`, `绑定元宝支出 ${yuanbaoText(spend.boundYuanbaoUnits)}`);
    for (const value of quote.materials) {
        lines.push(`消耗 ${name(value.item.itemId)}`, `数量 ${value.consumeQuantity} · 实例${assetId(value.item.instanceId).slice(-6)}`,
            `${value.item.bound ? '绑定' : '非绑定'} · ${sourceName(value.item.sourceKind)} · 品质${value.item.quality ?? 0}`);
    }
    for (const item of quote.outputs) lines.push(`获得 ${name(item.itemId)}`, `数量 ${item.quantity}`,
        `${item.bound ? '绑定' : '非绑定'} · ${sourceName(item.sourceKind)} · 品质${item.quality ?? 0}`);
    if (quote.kind === 2) lines.push('材料只用于角色技能成长，不产生可转卖属性');
    if (quote.kind === 1) lines.push('NPC购买的技能书不能分解为书页');
    lines.push('报价未冻结资产，确认时仍需校验');
    return lines;
}
export function yuanbaoText(value: PotionInteger): string { const amount = assetInteger(value); return `${amount / 10n}.${amount % 10n}`; }
