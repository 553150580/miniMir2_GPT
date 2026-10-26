import Long from 'long';

export interface StorageLike {
    getItem(key: string): string | null;
    setItem(key: string, value: string): void;
    removeItem(key: string): void;
}

export interface PendingOperation {
    readonly id: number;
    readonly value: Record<string, unknown>;
}

export type PendingErrorCode = 'INVALID_ROLE' | 'INVALID_OPERATION' | 'PENDING_EXISTS'
    | 'CAPACITY' | 'LIMIT' | 'CORRUPT_STORAGE' | 'STORAGE_FAILURE';

export class PendingOperationError extends Error {
    constructor(readonly code: PendingErrorCode, message: string, readonly detail?: unknown) {
        super(message);
        this.name = 'PendingOperationError';
    }
}

const MAX_ROLES = 5;
const MAX_BYTES = 64 * 1024;
const ALLOWED_IDS = new Set([40001, 40002, 40006, 30002]);
type ObjectValue = Record<string, unknown>;

function invalid(message: string): never {
    throw new PendingOperationError('INVALID_OPERATION', message);
}

function record(value: unknown, fields: readonly string[]): ObjectValue {
    if (value === null || typeof value !== 'object' || Array.isArray(value)
        || (Object.getPrototypeOf(value) !== Object.prototype && Object.getPrototypeOf(value) !== null)) {
        return invalid('未决操作必须为普通对象');
    }
    const result = value as ObjectValue;
    const keys = Object.keys(result);
    if (keys.length !== fields.length || fields.some(key => !Object.prototype.hasOwnProperty.call(result, key))) {
        return invalid('未决操作包含缺失或未允许的字段');
    }
    return result;
}

function roleId(role: unknown): string {
    if (typeof role !== 'string' || !/^[A-Za-z0-9:_-]{1,160}$/.test(role)) {
        throw new PendingOperationError('INVALID_ROLE', '需要明确的角色永久ID字符串');
    }
    return role;
}

function uint32(value: unknown): number {
    if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0 || value > 0xffffffff) {
        return invalid('操作中的 uint32 字段无效');
    }
    return value;
}

function longValue(value: unknown, stored: boolean): Long {
    if (stored) {
        const tag = record(value, ['$type', 'value', 'unsigned']);
        if (tag.$type !== 'Long' || typeof tag.value !== 'string' || tag.value.length > 20
            || !/^-?(0|[1-9][0-9]*)$/.test(tag.value) || typeof tag.unsigned !== 'boolean') {
            return invalid('64位数值标记损坏');
        }
        const result = Long.fromString(tag.value, tag.unsigned);
        if (result.toString() !== tag.value) return invalid('64位数值超限或非规范表示');
        return result;
    }
    if (!Long.isLong(value)) return invalid('64位字段必须使用 Long，不能使用 Number');
    const item = value as Long;
    if (!Number.isInteger(item.low) || item.low < -0x80000000 || item.low > 0x7fffffff
        || !Number.isInteger(item.high) || item.high < -0x80000000 || item.high > 0x7fffffff
        || typeof item.unsigned !== 'boolean') return invalid('Long 位模式无效');
    return Long.fromBits(item.low, item.high, item.unsigned);
}

function bytes16(value: unknown, stored: boolean): Uint8Array {
    if (stored) {
        const tag = record(value, ['$type', 'hex']);
        if (tag.$type !== 'Bytes16' || typeof tag.hex !== 'string' || !/^[0-9a-f]{32}$/.test(tag.hex)) {
            return invalid('128位标识标记损坏');
        }
        const bytes = new Uint8Array(16);
        for (let index = 0; index < 16; ++index) bytes[index] = parseInt(tag.hex.slice(index * 2, index * 2 + 2), 16);
        return bytes;
    }
    if (!(value instanceof Uint8Array) || value.length !== 16) return invalid('操作和物品ID必须为16字节');
    return new Uint8Array(value);
}

function operation(value: unknown, stored: boolean): ObjectValue {
    const context = record(value, ['opId', 'commandSeq', 'expectedRevision', 'expectedRuleVersion']);
    return {
        opId: bytes16(context.opId, stored),
        commandSeq: longValue(context.commandSeq, stored),
        expectedRevision: longValue(context.expectedRevision, stored),
        expectedRuleVersion: longValue(context.expectedRuleVersion, stored),
    };
}

function normalize(value: unknown, stored: boolean): PendingOperation {
    const input = record(value, ['id', 'value']);
    const id = uint32(input.id);
    if (!ALLOWED_IDS.has(id)) return invalid('不允许持久化此消息');
    let payload: ObjectValue;
    if (id === 40001) {
        const item = record(input.value, ['itemInstanceId', 'slot', 'operation']);
        payload = { itemInstanceId: bytes16(item.itemInstanceId, stored), slot: uint32(item.slot),
            operation: operation(item.operation, stored) };
    } else if (id === 40002) {
        const item = record(input.value, ['itemInstanceIds', 'operation']);
        if (!Array.isArray(item.itemInstanceIds) || !item.itemInstanceIds.length || item.itemInstanceIds.length > MAX_BYTES) {
            return invalid('分解物品列表无效');
        }
        payload = { itemInstanceIds: item.itemInstanceIds.map(entry => bytes16(entry, stored)),
            operation: operation(item.operation, stored) };
    } else if (id === 40006) {
        const item = record(input.value, ['benefitKind', 'operation']);
        payload = { benefitKind: uint32(item.benefitKind), operation: operation(item.operation, stored) };
    } else {
        const item = record(input.value, ['deathId', 'action', 'operation']);
        payload = { deathId: bytes16(item.deathId, stored), action: uint32(item.action),
            operation: operation(item.operation, stored) };
    }
    return { id, value: payload };
}

function tagged(value: unknown): unknown {
    if (Long.isLong(value)) return { $type: 'Long', value: value.toString(), unsigned: value.unsigned };
    if (value instanceof Uint8Array) {
        return { $type: 'Bytes16', hex: Array.from(value).map(byte => byte.toString(16).padStart(2, '0')).join('') };
    }
    if (Array.isArray(value)) return value.map(tagged);
    if (value !== null && typeof value === 'object') {
        const result: ObjectValue = {};
        for (const key of Object.keys(value)) result[key] = tagged((value as ObjectValue)[key]);
        return result;
    }
    return value;
}

// 统计真实 UTF-8 字节，不能用 JS 的 UTF-16 字符数放宽持久数据上限。
function withinLimit(text: string): boolean {
    let bytes = 0;
    for (let index = 0; index < text.length; ++index) {
        const code = text.charCodeAt(index);
        if (code < 0x80) ++bytes;
        else if (code < 0x800) bytes += 2;
        else if (code >= 0xd800 && code <= 0xdbff && index + 1 < text.length
            && text.charCodeAt(index + 1) >= 0xdc00 && text.charCodeAt(index + 1) <= 0xdfff) { bytes += 4; ++index; }
        else bytes += 3;
        if (bytes > MAX_BYTES) return false;
    }
    return true;
}

// 只保存白名单经济请求，不读取 DOM；调用方显式注入按账号隔离的存储命名空间。
export class PendingOperations {
    private records = new Map<string, PendingOperation>();

    constructor(private readonly storage?: StorageLike, private readonly key = 'minimir.pending-operations.v1') {
        this.records = this.read();
    }

    get(role: string): PendingOperation | null {
        roleId(role);
        this.records = this.read();
        const pending = this.records.get(role);
        return pending ? normalize(pending, false) : null;
    }

    set(role: string, value: PendingOperation): void {
        roleId(role);
        const pending = normalize(value, false);
        const next = this.read();
        const existing = next.get(role);
        if (existing) {
            if (JSON.stringify(tagged(existing)) !== JSON.stringify(tagged(pending))) {
                throw new PendingOperationError('PENDING_EXISTS', '此角色已有未决操作，必须先沿原身份对账');
            }
            this.records = next;
            return;
        }
        if (next.size >= MAX_ROLES) throw new PendingOperationError('CAPACITY', '未决操作最多保留五个角色');
        next.set(role, pending);
        this.write(next);
        this.records = next;
    }

    clear(role: string): void {
        roleId(role);
        const next = this.read();
        if (next.delete(role)) this.write(next);
        this.records = next;
    }

    private read(): Map<string, PendingOperation> {
        if (!this.storage) return new Map(this.records);
        let raw: string | null;
        try { raw = this.storage.getItem(this.key); }
        catch (error) { throw new PendingOperationError('STORAGE_FAILURE', '读取未决操作失败，暂停新经济操作', error); }
        if (raw === null) return new Map();
        try {
            if (typeof raw !== 'string' || !withinLimit(raw)) return invalid('持久数据超过64KiB');
            const document = record(JSON.parse(raw), ['version', 'operations']);
            if (document.version !== 1 || !Array.isArray(document.operations) || document.operations.length > MAX_ROLES) {
                return invalid('持久数据版本或角色数无效');
            }
            const result = new Map<string, PendingOperation>();
            for (const item of document.operations) {
                const entry = record(item, ['role', 'id', 'value']);
                const role = roleId(entry.role);
                if (result.has(role)) return invalid('持久数据包含重复角色');
                result.set(role, normalize({ id: entry.id, value: entry.value }, true));
            }
            return result;
        } catch (error) {
            throw new PendingOperationError('CORRUPT_STORAGE', '未决操作记录损坏，必须人工核对，禁止创建替代操作', error);
        }
    }

    private write(records: Map<string, PendingOperation>): void {
        const operations = Array.from(records.entries()).map(([role, pending]) => ({ role, id: pending.id, value: tagged(pending.value) }));
        const raw = JSON.stringify({ version: 1, operations });
        if (!withinLimit(raw)) throw new PendingOperationError('LIMIT', '未决操作记录超过64KiB');
        if (!this.storage) return;
        try {
            if (records.size) this.storage.setItem(this.key, raw);
            else this.storage.removeItem(this.key);
        } catch (error) { throw new PendingOperationError('STORAGE_FAILURE', '保存未决操作失败，禁止发送尚未持久化的请求', error); }
    }
}
