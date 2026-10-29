import clientPb from '../generated/protobuf/client_pb.js';
import Long from 'long';
import { protocolErrorMessage } from './GameErrors';
import { inputTraceMessage, traceInput } from './InputTrace';
import { NavigationCancelAssembler } from './NavigationCancelAssembler';

const pb = clientPb.minimir.v1;
type MessageType = { encode(value: any): { finish(): Uint8Array }; decode(value: Uint8Array): any };
export interface Packet { id: number; seq: number; binding: Long; error: number; value: any; }
// 明确的协议拒绝与网络中断/未知提交分开，调用方不能把超时当作服务端撤销。
export class ProtocolResponseError extends Error {
    constructor(readonly messageId: number, readonly code: number) { super(protocolErrorMessage(messageId, code)); }
}

const codecs: Record<number, MessageType> = {
    1: pb.Heartbeat, 2: pb.HeartbeatResult, 4: pb.EnterWorldAck, 5: pb.SceneBaselineChunk, 6: pb.SyncReady,
    10001: pb.LocalLoginRequest, 10002: pb.LoginResult, 10003: pb.RoleDirectoryRequest,
    10004: pb.RoleDirectoryResult, 10005: pb.CreateRoleRequest, 10006: pb.RoleSummary,
    10007: pb.EntryTicketRequest, 10008: pb.EntryTicketResult,
    10009: pb.LocalLoginRequest, 10010: pb.LoginResult,
    20001: pb.EnterLineRequest, 20002: pb.EnterLineResult, 20006: pb.MoveIntent,
    20007: pb.MapTravelPrepare, 20008: pb.MapTravelPlan, 20009: pb.MapTravelCommit, 20010: pb.MapTravelState,
    20011: pb.BindingState,
    30001: pb.DeathNotice, 30002: pb.ReviveChoice, 30003: pb.CastIntent, 30004: pb.AutoBattleIntent, 30005: pb.HostingState, 30006: pb.CastAccepted,
    30007: pb.AttackAccepted, 30008: pb.CombatImpact, 30009: pb.SkillModeIntent, 30010: pb.SkillModeState, 30011: pb.AttackIntent,
    30012: pb.CastStep, 30013: pb.EntityDisplacement, 30014: pb.TeleportResult, 30015: pb.PetBorn,
    40001: pb.EquipRequest, 40002: pb.RecycleRequest, 40003: pb.InventoryResult,
    40004: pb.WalletState, 40005: pb.OperationResult, 40006: pb.ClaimHostingTimeRequest,
    40007: pb.BuyHostingTimeRequest, 40008: pb.HostingReport,
    40009: pb.UseItemRequest, 40010: pb.LearnSkillRequest,
    40011: pb.SkillAssetPreviewRequest, 40012: pb.SkillAssetPreview, 40013: pb.SkillAssetCommitRequest,
    40014: pb.CastWithMaterialRequest, 40015: pb.SetCastingSupplyRequest,
};

function variable(value: Long): number[] {
    const bytes: number[] = [];
    let remaining = value.toUnsigned();
    do { const byte = remaining.and(127).toInt(); remaining = remaining.shiftRightUnsigned(7); bytes.push(byte | (remaining.isZero() ? 0 : 128)); }
    while (!remaining.isZero());
    return bytes;
}

// 协议源保留snake_case；界面模型使用camelCase。Long与bytes始终保留精确类型。
function names(value: any, encode: boolean): any {
    if (value == null || typeof value !== 'object' || Long.isLong(value) || value instanceof Uint8Array) return value;
    if (Array.isArray(value)) return value.map(item => names(item, encode));
    const result: Record<string, any> = {};
    for (const key in value) {
        if (typeof value[key] === 'function' || key === '$type') continue;
        const converted = encode ? key.replace(/[A-Z]/g, letter => '_' + letter.toLowerCase()) : key.replace(/_([a-z])/g, (_match, letter) => letter.toUpperCase());
        result[converted] = names(value[key], encode);
    }
    return result;
}

export function encodePacket(id: number, seq: number, binding: Long, value: any): Uint8Array {
    const codec = codecs[id];
    if (!codec) throw new Error(`未知消息：${id}`);
    const head = [1, 0, id >> 8, id & 255, ...variable(Long.fromNumber(seq, true)), ...variable(binding)];
    const body = codec.encode(names(value, true)).finish();
    if (head.length + body.length > 65536) throw new Error('消息超过单帧限制');
    const result = new Uint8Array(head.length + body.length); result.set(head); result.set(body, head.length); return result;
}

export function decodePacket(data: Uint8Array): Packet {
    if (data.length < 6 || data.length > 65536 || data[0] !== 1 || (data[1] & ~1) !== 0) throw new Error('协议头或帧长度错误');
    let offset = 4;
    const read = (limit: number): Long => {
        let result = Long.UZERO;
        for (let index = 0; index < limit; index++) {
            if (offset >= data.length) throw new Error('变长整数被截断');
            const byte = data[offset++];
            if (index === limit - 1 && ((limit === 10 && byte > 1) || (limit === 5 && byte > 15))) throw new Error('变长整数溢出');
            result = result.or(Long.fromInt(byte & 127, true).shiftLeft(index * 7));
            if ((byte & 128) === 0) return result;
        }
        throw new Error('变长整数超长');
    };
    const id = data[2] * 256 + data[3], seq = read(5).toNumber(), binding = read(10);
    const error = data[1] & 1 ? read(5).toNumber() : 0;
    const value = error ? null : names(codecs[id]?.decode(data.subarray(offset)), false);
    if (!error && !codecs[id]) throw new Error(`未支持消息：${id}`);
    return { id, seq, binding, error, value };
}

export class GameConnection {
    token = '';
    binding = Long.UZERO;
    private sequence = 0;
    private lifecycle = 0;
    private socket?: WebSocket;
    private listeners = new Set<(packet: Packet) => void>();
    private statusListeners = new Set<(status: string) => void>();
    private pending = new Map<number, { responses: readonly number[]; assemble?: (chunk: any) => any | null; ok: (value: any) => void; fail: (error: Error) => void; timer: ReturnType<typeof setTimeout> }>();
    get generation(): number { return this.lifecycle; }
    get connected(): boolean { return this.socket?.readyState === WebSocket.OPEN; }

    subscribe(listener: (packet: Packet) => void): () => void { this.listeners.add(listener); return () => this.listeners.delete(listener); }
    onStatus(listener: (status: string) => void): () => void { this.statusListeners.add(listener); return () => this.statusListeners.delete(listener); }

    async rpc(id: number, value: any): Promise<any> {
        const seq = ++this.sequence;
        const response = await fetch(`${location.origin}/account/rpc`, {
            method: 'POST', headers: { 'Content-Type': 'application/octet-stream', ...(this.token ? { Authorization: `Bearer ${this.token}` } : {}) },
            body: encodePacket(id, seq, Long.UZERO, value),
        });
        if (!response.ok) throw new Error(`认证服务返回HTTP ${response.status}`);
        const packet = decodePacket(new Uint8Array(await response.arrayBuffer()));
        if (packet.seq !== seq) throw new Error('认证响应序号不匹配');
        if (packet.error) throw new ProtocolResponseError(packet.id, packet.error);
        return packet.value;
    }

    async connect(ticket: string): Promise<void> {
        this.close(); this.binding = Long.UZERO;
        const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
        const socket = new WebSocket(`${protocol}//${location.host}/game/ws`); this.socket = socket;
        const generation = this.lifecycle;
        socket.binaryType = 'arraybuffer';
        socket.onmessage = event => {
            if (this.socket !== socket || !(event.data instanceof ArrayBuffer)) return;
            try {
                const packet = decodePacket(new Uint8Array(event.data));
                if (!this.binding.isZero() && !packet.binding.equals(this.binding)) return;
                if (packet.id === 20002 && !packet.error) {
                    if (packet.binding.isZero()) throw new Error('入场缺少非零连接绑定');
                    this.binding = packet.binding;
                }
                if (!packet.error) traceInput(inputTraceMessage('receive', packet.id, packet.seq, packet.value));
                const pending = this.pending.get(packet.seq);
                if (pending && (packet.error || pending.responses.includes(packet.id))) {
                    try {
                        const value = packet.error ? null : pending.assemble ? pending.assemble(packet.value) : packet.value;
                        if (packet.error || value !== null) {
                            this.pending.delete(packet.seq); clearTimeout(pending.timer);
                            if (packet.error) pending.fail(new ProtocolResponseError(packet.id, packet.error)); else pending.ok(value);
                        }
                    } catch (error) {
                        this.pending.delete(packet.seq); clearTimeout(pending.timer);
                        pending.fail(error instanceof Error ? error : new Error(String(error))); throw error;
                    }
                }
                for (const listener of Array.from(this.listeners)) listener(packet);
            } catch (error) { this.report(error instanceof Error ? error.message : String(error)); }
        };
        socket.onclose = () => { if (this.socket === socket) { this.rejectPending(); this.report('连接已断开，重新连接需重新认证取票'); } };
        await new Promise<void>((ok, fail) => {
            const timer = setTimeout(() => { socket.close(); fail(new Error('游戏连接超时')); }, 10000);
            socket.onopen = () => { clearTimeout(timer); ok(); };
            socket.onerror = () => { clearTimeout(timer); fail(new Error('游戏连接失败')); };
        });
        if (this.socket !== socket || this.lifecycle !== generation) throw new Error('旧游戏连接准备已经失效');
        await this.request(20001, { accountTicket: ticket });
        if (this.socket !== socket || this.lifecycle !== generation) throw new Error('旧游戏入场回执已经失效');
    }

    send(id: number, value: any): number {
        if (this.socket?.readyState !== WebSocket.OPEN) throw new Error('游戏尚未连接');
        const seq = ++this.sequence; this.socket.send(encodePacket(id, seq, this.binding, value));
        traceInput(inputTraceMessage('send', id, seq, value)); return seq;
    }

    requestNavigationCancel(sceneEpoch: Long, baselineId: Long, inputSequence: number, mapId?: Long): Promise<any> {
        const assembly = new NavigationCancelAssembler(sceneEpoch, baselineId, mapId);
        return this.request(20006, { sceneEpoch, inputSequence, cancelNavigation: true }, chunk => assembly.accept(chunk));
    }
    request(id: number, value: any, assemble?: (chunk: any) => any | null): Promise<any> {
        if (this.socket?.readyState !== WebSocket.OPEN) return Promise.reject(new Error('游戏尚未连接'));
        const seq = ++this.sequence;
        const cancelNavigation = id === 20006 && value?.cancelNavigation === true;
        if (cancelNavigation && !assemble) return Promise.reject(new Error('导航取消必须等待匹配的完整基线'));
        return new Promise((ok, fail) => {
            const timer = setTimeout(() => { this.pending.delete(seq); fail(new Error(cancelNavigation ? '导航取消确认超时，请稍后重新退出' : '操作回执超时；经济操作需沿原身份核对，不能自动创建新操作')); }, 10000);
            const responses = id === 20007 ? [20008, 20010] : [cancelNavigation ? 5 : id === 20001 ? 20002 : id === 20009 ? 20010 : id === 30004 ? 30005 : id === 40011 ? 40012 : 40005];
            this.pending.set(seq, { responses, assemble, ok, fail, timer });
            try {
                this.socket!.send(encodePacket(id, seq, this.binding, value));
                traceInput(inputTraceMessage('send', id, seq, value));
            } catch (error) { this.pending.delete(seq); clearTimeout(timer); fail(error instanceof Error ? error : new Error(String(error))); }
        });
    }

    close(): void { this.lifecycle++; const old = this.socket; this.socket = undefined; old?.close(); this.rejectPending(); }
    private rejectPending(): void { for (const item of Array.from(this.pending.values())) { clearTimeout(item.timer); item.fail(new Error('连接已关闭')); } this.pending.clear(); }
    private report(message: string): void { this.lifecycle++; for (const listener of Array.from(this.statusListeners)) listener(message); }
}

export function newOperationId(): Uint8Array { const bytes = new Uint8Array(16); crypto.getRandomValues(bytes); return bytes; }
