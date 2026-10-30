import Long from 'long';
import type { StorageLike } from './PendingOperations';

export interface RoleCreationRequest {
    originServerId: Long; slot: number; job: number; name: string; opId: Uint8Array;
}

function operationId(value: Uint8Array): string {
    if (!(value instanceof Uint8Array) || value.length !== 16) throw new Error('建角操作身份无效');
    return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
}
function checked(value: RoleCreationRequest): RoleCreationRequest {
    if (!Long.isLong(value.originServerId) || !/^[1-9]\d*$/.test(value.originServerId.toString())
        || !Number.isInteger(value.slot) || value.slot < 1 || value.slot > 5 || !Number.isInteger(value.job) || value.job < 0 || value.job > 2
        || typeof value.name !== 'string' || value.name !== value.name.trim()
        || Array.from(value.name.normalize('NFKC')).length < 1 || Array.from(value.name.normalize('NFKC')).length > 16) throw new Error('原建角参数无效');
    operationId(value.opId);
    return { originServerId: Long.fromString(value.originServerId.toString(), true), slot: value.slot, job: value.job, name: value.name, opId: new Uint8Array(value.opId) };
}
function document(value: RoleCreationRequest): string {
    const request = checked(value);
    return JSON.stringify({ version: 1, originServerId: request.originServerId.toString(), slot: request.slot, job: request.job, name: request.name, opId: operationId(request.opId) });
}

// 每个账号/原始服只保留一笔建角。原请求发送前落盘，不保存账号口令或票据。
export class PendingRoleCreation {
    constructor(private readonly storage: StorageLike, private readonly key: string) {}
    get(): RoleCreationRequest | null {
        let raw: string | null;
        try { raw = this.storage.getItem(this.key); } catch { throw new Error('读取原建角记录失败，暂停新建角'); }
        if (raw === null) return null;
        try {
            if (raw.length > 4096) throw new Error('记录过长');
            const value = JSON.parse(raw);
            if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).sort().join(',') !== 'job,name,opId,originServerId,slot,version'
                || value.version !== 1 || typeof value.originServerId !== 'string' || !/^[1-9]\d{0,19}$/.test(value.originServerId)
                || BigInt(value.originServerId) > 0xffffffffffffffffn || typeof value.opId !== 'string' || !/^[0-9a-f]{32}$/.test(value.opId)) throw new Error('记录形状无效');
            return checked({ ...value, originServerId: Long.fromString(value.originServerId, true),
                opId: Uint8Array.from({ length: 16 }, (_item, index) => parseInt(value.opId.slice(index * 2, index * 2 + 2), 16)) });
        } catch { throw new Error('原建角记录损坏，须先核对，不能分配替代操作'); }
    }
    set(value: RoleCreationRequest): void {
        const raw = document(value), current = this.get();
        if (current) {
            if (document(current) !== raw) throw new Error('已有原建角操作待核对，不能更换操作或参数');
            return;
        }
        try { this.storage.setItem(this.key, raw); } catch { throw new Error('保存原建角记录失败，尚未发送请求'); }
    }
    clear(opId: Uint8Array): void {
        const current = this.get();
        if (!current) return;
        if (operationId(current.opId) !== operationId(opId)) throw new Error('建角记录已变化，不能清除其它操作');
        try { this.storage.removeItem(this.key); } catch { throw new Error('清理已确认建角记录失败，请沿原操作再核对'); }
    }
}
