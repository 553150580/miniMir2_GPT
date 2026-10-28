import { attachmentKey, travelInteger } from './MapTravelModel';
import type { AttachmentIdentity, TravelInteger } from './MapTravelModel';

export interface RoleBindingPoint { readonly mapId: string; readonly center: { readonly x: number; readonly y: number }; }
export interface RoleBindingObservation {
    readonly role: string; readonly sceneEpoch: string; readonly roleRevision: string; readonly sourceSeq: string;
    readonly binding: RoleBindingPoint | null; readonly cause: 1 | 2; readonly safeZoneId: string; readonly trusted: boolean;
}
export interface RoleBindingPacket {
    readonly role?: { readonly originServerId: TravelInteger; readonly roleId: Uint8Array };
    readonly sceneEpoch: TravelInteger; readonly roleRevision: TravelInteger; readonly sourceSeq: TravelInteger;
    readonly binding?: { readonly mapId: TravelInteger; readonly center?: { readonly x: number; readonly y: number } } | null;
    readonly cause: number; readonly safeZoneId: TravelInteger; readonly trusted: boolean;
}
function roleKey(value: RoleBindingPacket['role']): string {
    if (!value || !(value.roleId instanceof Uint8Array) || value.roleId.length !== 16) throw new Error('绑定观察缺少完整角色身份');
    return travelInteger(value.originServerId, true) + ':' + Array.from(value.roleId, byte => byte.toString(16).padStart(2, '0')).join('');
}
function bindingKey(value: RoleBindingObservation): string {
    return value.binding ? `${value.binding.mapId}:${value.binding.center.x},${value.binding.center.y}:${value.trusted}` : 'unbound';
}
function normalize(value: RoleBindingPacket): RoleBindingObservation {
    if (![1, 2].includes(value.cause) || typeof value.trusted !== 'boolean') throw new Error('绑定观察来源无效');
    const sourceSeq = travelInteger(value.sourceSeq), safeZoneId = travelInteger(value.safeZoneId), roleRevision = travelInteger(value.roleRevision, true);
    let binding: RoleBindingPoint | null = null;
    if (value.binding != null) {
        const center = value.binding.center;
        if (!center || ![center.x, center.y].every(n => Number.isSafeInteger(n) && n >= 0 && n <= 2147483647)) throw new Error('绑定中心坐标无效');
        binding = Object.freeze({ mapId: travelInteger(value.binding.mapId, true), center: Object.freeze({ ...center }) });
    }
    // 历史无证明坐标只作观察；可信绑定必须来自服务端已提交的原绑定receipt。
    if ((value.trusted && (!binding || sourceSeq === '0' || safeZoneId === '0')) || (!value.trusted && (sourceSeq !== '0' || safeZoneId !== '0'))
        || BigInt(sourceSeq) > BigInt(roleRevision)
        || (!binding && safeZoneId !== '0') || (value.cause === 2 && !value.trusted)) throw new Error('绑定证明与状态不一致');
    return Object.freeze({ role: roleKey(value.role), sceneEpoch: travelInteger(value.sceneEpoch, true), roleRevision,
        sourceSeq, binding, cause: value.cause as 1 | 2, safeZoneId, trusted: value.trusted });
}

// 只接收当前附着的服务端提交观察，不根据玩家位置/安全区几何推断绑定，也不持有经济命令或Pending。
export class RoleBindingModel {
    private scope = '';
    private lastBinding?: string;
    private current: RoleBindingObservation | null = null;
    private committedNotice: RoleBindingObservation | null = null;
    get state(): RoleBindingObservation | null { return this.current; }
    get notice(): RoleBindingObservation | null { return this.committedNotice; }
    bind(identity: AttachmentIdentity): void { this.clear(); this.scope = attachmentKey(identity); }
    clear(): void { this.scope = ''; this.current = null; this.committedNotice = null; this.lastBinding = undefined; }
    accept(value: RoleBindingPacket, identity: AttachmentIdentity, transportBinding: TravelInteger): boolean {
        try {
            if (!this.scope || this.scope !== attachmentKey(identity) || travelInteger(transportBinding) !== identity.binding.toString()) return false;
            const next = normalize(value);
            if (next.role !== identity.role || next.sceneEpoch !== identity.scene.toString()) return false;
            const previous = this.current, key = bindingKey(next);
            if (previous) {
                const revision = BigInt(next.roleRevision), oldRevision = BigInt(previous.roleRevision), sequence = BigInt(next.sourceSeq), oldSequence = BigInt(previous.sourceSeq);
                if (revision < oldRevision || sequence < oldSequence) return false;
                const sameProof = key === bindingKey(previous) && next.safeZoneId === previous.safeZoneId;
                if ((revision === oldRevision && (!sameProof || sequence !== oldSequence)) || (sequence === oldSequence && !sameProof)) return false;
                if (revision === oldRevision && sequence === oldSequence && sameProof) return false;
            }
            this.current = next;
            if (this.committedNotice && bindingKey(this.committedNotice) !== key) this.committedNotice = null;
            if (next.cause === 1 && this.lastBinding === undefined) this.lastBinding = key;
            if (next.cause === 2 && key !== this.lastBinding) { this.committedNotice = next; this.lastBinding = key; }
            return true;
        } catch { return false; }
    }
}

export function roleBindingMapName(value: RoleBindingObservation, catalog?: readonly { readonly id: string; readonly name?: string }[]): string {
    if (!value.binding) return '';
    return catalog?.find(row => row.id === value.binding!.mapId)?.name || `地图${value.binding.mapId}`;
}
export function roleBindingText(value: RoleBindingObservation | null, catalog?: readonly { readonly id: string; readonly name?: string }[]): string {
    if (!value) return '绑定信息同步中';
    if (!value.binding) return '尚未绑定';
    const { x, y } = value.binding.center;
    return `绑定：${roleBindingMapName(value, catalog)} (${x},${y})` + (value.trusted ? '' : '\n历史记录待服务端确认');
}
