import Long from 'long';

export type WarehouseScope = 1 | 2;
export interface WarehouseConfig {
    personal_initial_slots: number; personal_expansion_slots: number; personal_expansion_yuanbao_units: number;
    shared_slots: number; query_page_limit: number;
}
export interface WarehouseItem {
    instanceId: Uint8Array; itemId: number; quantity: Long; version: Long; bound: boolean; quality: number;
    sourceKind: string; sourceId: string; locked: boolean; recycleBlockFlags: number;
}
export interface WarehouseSlot { slot: Long; item: WarehouseItem | null; }
export interface WarehousePage {
    role: { originServerId: Long; roleId: Uint8Array }; scope: WarehouseScope; revision: Long; capacity: Long;
    startSlot: Long; limit: number; items: Array<{ slot: Long; item: WarehouseItem }>; gold: Long; yuanbaoUnits: Long;
}
export interface WarehouseQuery { scope: WarehouseScope; startSlot: Long; limit: number; }
export interface WarehouseRequest {
    scope: WarehouseScope; action: number; warehouseRevision: Long; warehouseSlot: Long;
    itemInstanceId: Uint8Array; itemVersion: Long; quantity: Long; currency: number; amount: Long;
}
export interface WarehouseSnapshot {
    ready: boolean; attachment: string; role: string; rule: string; config: WarehouseConfig;
    page: WarehousePage | null; inventory: readonly WarehouseItem[];
    wallet: { gold: Long; yuanbaoUnits: Long } | null;
}
export interface WarehousePreview {
    readonly attachment: string; readonly role: string; readonly rule: string;
    readonly request: WarehouseRequest; readonly originalItem: string | null;
    readonly walletAmount: string | null; readonly configKey: string;
}

const U64_MAX = 0xffffffffffffffffn, I64_MAX = 0x7fffffffffffffffn;
export function warehouseInteger(value: unknown, signed = false): bigint {
    if (!Long.isLong(value)) throw new Error('仓库64位字段必须保留Long');
    const text = (value as Long).toString();
    if (!/^(0|[1-9]\d*)$/.test(text)) throw new Error('仓库数值不能为负数');
    const result = BigInt(text);
    if (result > (signed ? I64_MAX : U64_MAX)) throw new Error('仓库数值超出范围');
    return result;
}
export function warehouseId(value: Uint8Array): string {
    if (!(value instanceof Uint8Array) || value.length !== 16) throw new Error('仓库物品身份无效');
    return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
}
export function warehouseRole(value: WarehousePage['role']): string {
    return warehouseInteger(value.originServerId).toString() + ':' + warehouseId(value.roleId);
}
export function warehouseConfig(value: unknown): WarehouseConfig {
    const keys = ['personal_initial_slots', 'personal_expansion_slots', 'personal_expansion_yuanbao_units', 'shared_slots', 'query_page_limit'] as const;
    if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).length !== keys.length) throw new Error('仓库目录尚未完整同步');
    const config = value as WarehouseConfig;
    for (const key of keys) if (!Object.prototype.hasOwnProperty.call(config, key) || !Number.isSafeInteger(config[key]) || config[key] <= 0) throw new Error('仓库目录字段无效');
    if (config.query_page_limit > 0xffffffff) throw new Error('仓库分页窗口无效');
    return { ...config };
}
function itemKey(item: WarehouseItem): string {
    const quantity = warehouseInteger(item.quantity, true), version = warehouseInteger(item.version);
    if (!quantity || !version || version > I64_MAX || !Number.isSafeInteger(item.itemId) || item.itemId <= 0 || item.itemId > 0xffffffff
        || typeof item.bound !== 'boolean' || typeof item.locked !== 'boolean' || !Number.isInteger(item.quality) || item.quality < 0 || item.quality > 4
        || !Number.isInteger(item.recycleBlockFlags) || item.recycleBlockFlags < 0 || item.recycleBlockFlags > 15
        || Boolean(item.recycleBlockFlags & 1) !== item.locked || typeof item.sourceKind !== 'string' || typeof item.sourceId !== 'string') throw new Error('原物品资料不完整，请重新查询');
    return JSON.stringify([warehouseId(item.instanceId), item.itemId, quantity.toString(), version.toString(), item.bound, item.quality, item.locked, item.recycleBlockFlags, item.sourceKind, item.sourceId]);
}
function cloneItem(item: WarehouseItem): WarehouseItem {
    itemKey(item);
    return { ...item, instanceId: new Uint8Array(item.instanceId), quantity: Long.fromString(item.quantity.toString()), version: Long.fromString(item.version.toString(), true) };
}
export function warehouseItemReason(item: WarehouseItem | null, scope: WarehouseScope): string {
    if (!item) return '先选择原物品';
    try { itemKey(item); } catch (error) { return (error as Error).message; }
    if (warehouseInteger(item.version) >= I64_MAX) return '原物品版本不能继续迁移';
    if (item.recycleBlockFlags & 14) return '穿戴、待确认或占用中的物品不能存取';
    return scope === 2 && item.bound ? '共享仓库只接受非绑定物品' : '';
}
export function observedWarehousePage(value: WarehousePage, query: WarehouseQuery, role: string, configInput: WarehouseConfig): WarehousePage {
    const config = warehouseConfig(configInput), capacity = warehouseInteger(value.capacity), start = warehouseInteger(value.startSlot);
    warehouseInteger(value.revision);
    if (warehouseRole(value.role) !== role || value.scope !== query.scope || start !== warehouseInteger(query.startSlot)
        || value.limit !== query.limit || !Number.isInteger(value.limit) || value.limit < 1 || value.limit > config.query_page_limit
        || capacity < 1n || start >= capacity || value.scope === 2 && capacity !== BigInt(config.shared_slots)
        || value.scope === 1 && (capacity < BigInt(config.personal_initial_slots) || (capacity - BigInt(config.personal_initial_slots)) % BigInt(config.personal_expansion_slots))) throw new Error('仓库页身份或分页范围不匹配');
    const gold = warehouseInteger(value.gold, true), yuanbao = warehouseInteger(value.yuanbaoUnits, true);
    if (value.scope === 1 && (gold || yuanbao)) throw new Error('个人仓库不能保存货币');
    if (!Array.isArray(value.items) || value.items.length > value.limit) throw new Error('仓库页条目超出窗口');
    const slots = new Set<string>(), ids = new Set<string>(), end = start + BigInt(value.limit) < capacity ? start + BigInt(value.limit) : capacity;
    const items = value.items.map(row => {
        const slot = warehouseInteger(row.slot), id = warehouseId(row.item.instanceId);
        if (slot < start || slot >= end || slots.has(slot.toString()) || ids.has(id)) throw new Error('仓位越界或原物品重复');
        slots.add(slot.toString()); ids.add(id);
        if (row.item.recycleBlockFlags & 8 || value.scope === 2 && row.item.bound) throw new Error('仓库页包含非法穿戴或绑定物品');
        return { slot: Long.fromString(slot.toString(), true), item: cloneItem(row.item) };
    });
    return { ...value, role: { originServerId: Long.fromString(value.role.originServerId.toString(), true), roleId: new Uint8Array(value.role.roleId) },
        revision: Long.fromString(value.revision.toString(), true), capacity: Long.fromString(capacity.toString(), true), startSlot: Long.fromString(start.toString(), true),
        gold: Long.fromString(gold.toString()), yuanbaoUnits: Long.fromString(yuanbao.toString()), items };
}
export function warehouseSlots(page: WarehousePage): WarehouseSlot[] {
    const start = warehouseInteger(page.startSlot), capacity = warehouseInteger(page.capacity), result: WarehouseSlot[] = [];
    for (let index = 0; index < page.limit && start + BigInt(index) < capacity; index++) {
        const slot = Long.fromString((start + BigInt(index)).toString(), true);
        result.push({ slot, item: page.items.find(row => row.slot.equals(slot))?.item ?? null });
    }
    return result;
}
export function warehouseQuantity(text: string): Long {
    if (!/^[1-9]\d*$/.test(text) || BigInt(text) > I64_MAX) throw new Error('数量需为有效正整数');
    return Long.fromString(text);
}
export function warehouseMoney(text: string, currency: number): Long {
    if (currency === 1) return warehouseQuantity(text);
    if (currency !== 2 || !/^(0|[1-9]\d*)(\.\d)?$/.test(text)) throw new Error('元宝金额最多填写一位小数');
    const [whole, fraction = '0'] = text.split('.'), amount = BigInt(whole) * 10n + BigInt(fraction);
    if (!amount || amount > I64_MAX) throw new Error('金额需大于零且不能超出范围');
    return Long.fromString(amount.toString());
}
export function warehouseMoneyText(value: Long): string { const n = warehouseInteger(value, true); return `${n / 10n}.${n % 10n}`; }
export function createWarehousePreview(snapshot: WarehouseSnapshot, action: number, slot: Long, item: WarehouseItem | null, text: string, currency = 0): WarehousePreview {
    const config = warehouseConfig(snapshot.config), page = snapshot.page;
    if (!snapshot.ready || !snapshot.attachment || !snapshot.role || !page || warehouseRole(page.role) !== snapshot.role) throw new Error('等待当前角色仓库查询完成');
    if (![1, 2, 3, 4, 5].includes(action) || warehouseInteger(page.revision) >= I64_MAX) throw new Error('仓库动作或原版本无效');
    const request: WarehouseRequest = { scope: page.scope, action, warehouseRevision: Long.fromString(page.revision.toString(), true), warehouseSlot: Long.UZERO,
        itemInstanceId: new Uint8Array(0), itemVersion: Long.UZERO, quantity: Long.ZERO, currency: 0, amount: Long.ZERO };
    let originalItem: string | null = null, walletAmount: string | null = null;
    if (action === 1 || action === 2) {
        const reason = warehouseItemReason(item, page.scope); if (reason) throw new Error(reason);
        const row = warehouseSlots(page).find(row => row.slot.equals(slot));
        if (!row) throw new Error('先选择当前页的原仓位');
        const current = action === 1 ? snapshot.inventory.find(row => warehouseId(row.instanceId) === warehouseId(item!.instanceId)) : row.item;
        if (!current || itemKey(current) !== itemKey(item!) || action === 1 && row.item) throw new Error(action === 1 ? '存入需选择当前空仓位与原背包物品' : '原仓位物品已变化');
        const quantity = warehouseQuantity(text); if (quantity.greaterThan(current.quantity)) throw new Error('所选数量超过原物品数量');
        request.warehouseSlot = Long.fromString(slot.toString(), true); request.itemInstanceId = new Uint8Array(current.instanceId);
        request.itemVersion = Long.fromString(current.version.toString(), true); request.quantity = quantity; originalItem = itemKey(current);
    } else if (action === 3 || action === 4) {
        if (page.scope !== 2 || ![1, 2].includes(currency) || !snapshot.wallet) throw new Error('共享仓库才可存取金币与元宝');
        const amount = warehouseMoney(text, currency), balance = action === 3 ? currency === 1 ? snapshot.wallet.gold : snapshot.wallet.yuanbaoUnits : currency === 1 ? page.gold : page.yuanbaoUnits;
        if (warehouseInteger(balance, true) < warehouseInteger(amount, true)) throw new Error('原币种余额不足，不会换用绑定货币');
        request.currency = currency; request.amount = amount; walletAmount = action === 3 ? balance.toString() : null;
    } else {
        if (page.scope !== 1 || !snapshot.wallet) throw new Error('仅个人仓库可扩页');
        if (warehouseInteger(snapshot.wallet.yuanbaoUnits, true) < BigInt(config.personal_expansion_yuanbao_units)) throw new Error('非绑定元宝不足，不能扩页');
        walletAmount = snapshot.wallet.yuanbaoUnits.toString();
    }
    return { attachment: snapshot.attachment, role: snapshot.role, rule: snapshot.rule, request, originalItem, walletAmount, configKey: JSON.stringify(config) };
}
export function validateWarehousePreview(preview: WarehousePreview, snapshot: WarehouseSnapshot): void {
    if (!snapshot.ready || snapshot.attachment !== preview.attachment || snapshot.role !== preview.role || snapshot.rule !== preview.rule || JSON.stringify(warehouseConfig(snapshot.config)) !== preview.configKey
        || !snapshot.page || snapshot.page.scope !== preview.request.scope || !snapshot.page.revision.equals(preview.request.warehouseRevision)) throw new Error('原角色或仓库已变化，请重新查询确认');
    const p = preview.request, item = p.action === 1 ? snapshot.inventory.find(row => warehouseId(row.instanceId) === warehouseId(p.itemInstanceId))
        : p.action === 2 ? snapshot.page.items.find(row => row.slot.equals(p.warehouseSlot))?.item : null;
    const text = p.action <= 2 ? p.quantity.toString() : p.currency === 2 ? warehouseMoneyText(p.amount) : p.amount.toString();
    const current = createWarehousePreview(snapshot, p.action, p.warehouseSlot, item ?? null, text, p.currency);
    if (current.originalItem !== preview.originalItem || current.walletAmount !== preview.walletAmount) throw new Error('原物品或确认的钱包已变化，请重新确认');
}
export function warehouseConfirmation(preview: WarehousePreview, config: WarehouseConfig, name: string): string {
    const p = preview.request, heading = p.scope === 1 ? '个人仓库' : '共享仓库';
    if (p.action <= 2) return [heading, `${p.action === 1 ? '存入' : '取出'} ${name}`, `数量：${p.quantity}`, `仓位：${warehouseInteger(p.warehouseSlot) + 1n}`, '保留原来源、绑定和锁定', '不与其它堆叠自动合并', '物品发生变化时，请刷新后重新确认'].join('\n');
    if (p.action === 5) return [heading, `增加 ${config.personal_expansion_slots} 格`, `扣除元宝 ${warehouseMoneyText(Long.fromNumber(config.personal_expansion_yuanbao_units))}`,
        '只用非绑定元宝', '不会换用绑定元宝', '确认后以服务器结果为准'].join('\n');
    return [heading, `${p.action === 3 ? '存入' : '取出'}${p.currency === 1 ? '金币' : '元宝'}`, `金额：${p.currency === 1 ? p.amount : warehouseMoneyText(p.amount)}`,
        '只转移当前非绑定币种', '不会换用绑定货币', '确认后核对原仓库版本'].join('\n');
}

export interface WarehouseTicket { readonly attachment: string; readonly generation: number; readonly query: WarehouseQuery; }
// 最多一笔在途读取和每类一页；旧读取/旧附着不能覆盖当前页，不缓存整仓。
export class WarehousePager {
    private attachment = ''; private generation = 0;
    private pending: WarehouseTicket | null = null;
    private readonly pages = new Map<WarehouseScope, WarehousePage>();
    bind(attachment: string): void { if (attachment === this.attachment) return; this.attachment = attachment; this.generation++; this.pending = null; this.pages.clear(); }
    begin(query: WarehouseQuery): WarehouseTicket {
        if (!this.attachment) throw new Error('仓库尚未附着当前角色');
        warehouseInteger(query.startSlot);
        if (![1, 2].includes(query.scope) || !Number.isInteger(query.limit) || query.limit < 1) throw new Error('仓库查询范围无效');
        const ticket = { attachment: this.attachment, generation: ++this.generation, query: { ...query, startSlot: Long.fromString(query.startSlot.toString(), true) } };
        this.pending = ticket; return ticket;
    }
    accept(ticket: WarehouseTicket, value: WarehousePage, role: string, config: WarehouseConfig): boolean {
        if (this.pending !== ticket || ticket.attachment !== this.attachment || ticket.generation !== this.generation) return false;
        const page = observedWarehousePage(value, ticket.query, role, config), previous = this.pages.get(page.scope);
        if (previous && previous.revision.greaterThan(page.revision)) { this.pending = null; return false; }
        if (previous && previous.revision.equals(page.revision)) {
            if (!previous.capacity.equals(page.capacity) || !previous.gold.equals(page.gold) || !previous.yuanbaoUnits.equals(page.yuanbaoUnits)) throw new Error('同一仓库修订的容量或余额冲突');
            const left = warehouseInteger(previous.startSlot), right = warehouseInteger(page.startSlot), start = left > right ? left : right;
            const leftEnd = left + BigInt(previous.limit), rightEnd = right + BigInt(page.limit), end = leftEnd < rightEnd ? leftEnd : rightEnd;
            for (let slot = start; slot < end && slot < warehouseInteger(page.capacity); slot++) {
                const before = previous.items.find(row => row.slot.toString() === slot.toString())?.item;
                const after = page.items.find(row => row.slot.toString() === slot.toString())?.item;
                if ((before ? itemKey(before) : '') !== (after ? itemKey(after) : '')) throw new Error('同一仓库修订的原仓位物品冲突');
            }
        }
        this.pending = null; this.pages.set(page.scope, page); return true;
    }
    fail(ticket: WarehouseTicket): boolean { if (this.pending !== ticket) return false; this.pending = null; return true; }
    page(scope: WarehouseScope): WarehousePage | null { return this.pages.get(scope) ?? null; }
    invalidate(scope: WarehouseScope): void {
        this.pages.delete(scope);
        if (this.pending?.query.scope === scope) { this.generation++; this.pending = null; }
    }
    clear(): void { this.bind(''); }
}

export interface WarehouseReceipt {
    role: WarehousePage['role']; roleRevision: Long; opId: Uint8Array; scope: WarehouseScope; action: number;
    warehouseRevision: Long; capacity: Long; changes: Array<{ slot: Long; before?: WarehouseItem | null; after?: WarehouseItem | null }>;
    gold: Long; yuanbaoUnits: Long;
}
// 只接受当前附着明确发出/沿原journal重查的单一op；原回执从不写回页缓存。
export class WarehouseReceiptGuard {
    private attachment = '';
    private expected: { id: string; scope: WarehouseScope; action: number; revision: bigint } | null = null;
    private consumed = '';
    bind(attachment: string): void { if (attachment !== this.attachment) { this.attachment = attachment; this.expected = null; this.consumed = ''; } }
    expect(attachment: string, opId: Uint8Array, request: Pick<WarehouseRequest, 'scope' | 'action' | 'warehouseRevision'>): void {
        this.bind(attachment);
        if (!attachment || ![1, 2].includes(request.scope) || ![1, 2, 3, 4, 5].includes(request.action)) throw new Error('仓库操作附着无效');
        this.expected = { id: warehouseId(opId), scope: request.scope, action: request.action, revision: warehouseInteger(request.warehouseRevision) };
    }
    cancel(opId: Uint8Array): void { if (this.expected?.id === warehouseId(opId)) this.expected = null; }
    accept(value: WarehouseReceipt, attachment: string, role: string): boolean {
        if (attachment !== this.attachment || !this.expected || warehouseRole(value.role) !== role) return false;
        const id = warehouseId(value.opId), expected = this.expected;
        if (id !== expected.id || id === this.consumed || value.scope !== expected.scope || value.action !== expected.action
            || warehouseInteger(value.warehouseRevision) !== expected.revision + 1n || !warehouseInteger(value.roleRevision) || !warehouseInteger(value.capacity)) return false;
        warehouseInteger(value.gold, true); warehouseInteger(value.yuanbaoUnits, true);
        if (value.scope === 1 && (!value.gold.isZero() || !value.yuanbaoUnits.isZero()) || !Array.isArray(value.changes) || value.changes.length > 1) throw new Error('仓库回执内容无效');
        if (value.action >= 3 && value.changes.length) throw new Error('货币或扩页回执不能带物品变更');
        for (const change of value.changes) {
            if (warehouseInteger(change.slot) >= warehouseInteger(value.capacity) || !change.before && !change.after) throw new Error('仓库回执仓位无效');
            if (change.before) itemKey(change.before); if (change.after) itemKey(change.after);
        }
        this.consumed = id; this.expected = null; return true;
    }
}
