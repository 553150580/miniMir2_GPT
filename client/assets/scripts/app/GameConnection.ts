import clientPb from '../generated/protobuf/client_pb.js';
import Long from 'long';
import { protocolErrorMessage } from './GameErrors';
import { inputTraceMessage, traceInput } from './InputTrace';
import { NavigationCancelAssembler } from './NavigationCancelAssembler';
import { isMirrorCombatMessage } from './MirrorArenaModel';

const pb = clientPb.minimir.v1;
const accountRpcTimeoutMs = 10000;
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
    10011: pb.ServerDirectoryRequest, 10012: pb.ServerDirectoryResult,
    20001: pb.EnterLineRequest, 20002: pb.EnterLineResult, 20006: pb.MoveIntent,
    20007: pb.MapTravelPrepare, 20008: pb.MapTravelPlan, 20009: pb.MapTravelCommit, 20010: pb.MapTravelState,
    20011: pb.BindingState,
    20012: pb.PersonalBossEnter, 20013: pb.PersonalBossLeave, 20014: pb.PersonalBossState,
    20015: pb.PersonalBossQuery, 20016: pb.PersonalBossInfo,
    20017: pb.WorldBossQuery, 20018: pb.WorldBossEnter, 20019: pb.WorldBossLeave, 20020: pb.WorldBossState,
    20021: pb.DungeonPartyRequest, 20022: pb.DungeonPartyState, 20023: pb.DungeonBattleState, 20024: pb.DungeonSceneTransition,
    20025: pb.MirrorArenaRequest, 20026: pb.MirrorArenaState, 20027: pb.MirrorArenaCombatFrame,
    20028: pb.MiningIntent, 20029: pb.MiningState,
    30001: pb.DeathNotice, 30002: pb.ReviveChoice, 30003: pb.CastIntent, 30004: pb.AutoBattleIntent, 30005: pb.HostingState, 30006: pb.CastAccepted,
    30007: pb.AttackAccepted, 30008: pb.CombatImpact, 30009: pb.SkillModeIntent, 30010: pb.SkillModeState, 30011: pb.AttackIntent,
    30012: pb.CastStep, 30013: pb.EntityDisplacement, 30014: pb.TeleportResult, 30015: pb.PetBorn,
    30016: pb.AutoTargetIntent,
    40001: pb.EquipRequest, 40002: pb.RecycleRequest, 40003: pb.InventoryResult,
    40004: pb.WalletState, 40005: pb.OperationResult, 40006: pb.ClaimHostingTimeRequest,
    40007: pb.BuyHostingTimeRequest, 40008: pb.HostingReport,
    40009: pb.UseItemRequest, 40010: pb.LearnSkillRequest,
    40011: pb.SkillAssetPreviewRequest, 40012: pb.SkillAssetPreview, 40013: pb.SkillAssetCommitRequest,
    40014: pb.CastWithMaterialRequest, 40015: pb.SetCastingSupplyRequest,
    40016: pb.SetAutoRecycleRequest, 40017: pb.SetItemLockRequest,
    40018: pb.EquipmentRecycleResult,
    40019: pb.ExperienceGrantResult,
    40020: pb.WarehouseQuery, 40021: pb.WarehousePage, 40022: pb.WarehouseAction, 40023: pb.WarehouseActionResult,
    40024: pb.VendorQuoteRequest, 40025: pb.VendorQuoteResult, 40026: pb.VendorCommitRequest,
    40027: pb.TaskCodexStateRequest, 40028: pb.TaskCodexStateResult, 40029: pb.TaskCodexClaimRequest,
    40030: pb.ClaimChallengeReward, 40031: pb.SetSkillSlotsRequest,
    40032: pb.HostingQuoteRequest, 40033: pb.HostingQuoteResult,
    40034: pb.MirrorArenaExchangeRequest,
    40035: pb.RoleCardQuery, 40036: pb.RoleCardState,
    40037: pb.BuyRoleCardRequest, 40038: pb.ClaimRoleCardDailyRequest,
    40039: pb.ItemMarketQuery, 40040: pb.ItemMarketState, 40041: pb.ItemMarketAction,
    40042: pb.GoldMarketQuery, 40043: pb.GoldMarketState, 40044: pb.GoldMarketAction,
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

// 镜像整帧中的内层数据没有传输帧头，只允许已登记的格战表现解码。
export function decodeMirrorCombatPayload(id: number, payload: Uint8Array): unknown {
    if (!isMirrorCombatMessage(id)) return null;
    return names(codecs[id].decode(payload), false);
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

// 票据指定本次连接地址。同源开发代理可用根路径，公网 HTTPS 页面只使用安全 WebSocket。
export function gameSocketAddress(endpoint: unknown, accountOrigin: string): string {
    if (typeof endpoint !== 'string' || !endpoint || /[\s\\#]/u.test(endpoint)) throw new Error('入场票据缺少有效的游戏连接地址');
    if (!/^\/(?!\/)/u.test(endpoint) && !/^wss?:\/\/[^/]/iu.test(endpoint)) throw new Error('游戏连接地址无效');
    const origin = new URL(accountOrigin);
    origin.protocol = origin.protocol === 'https:' ? 'wss:' : 'ws:';
    let target: URL;
    try { target = new URL(endpoint, origin); }
    catch { throw new Error('游戏连接地址无效'); }
    if (!target.hostname || target.username || target.password || target.hash || (target.protocol !== 'ws:' && target.protocol !== 'wss:')
        || (origin.protocol === 'wss:' && target.protocol !== 'wss:')) throw new Error('游戏连接地址无效');
    return target.href;
}

export class GameConnection {
    token = '';
    binding = Long.UZERO;
    private sequence = 0;
    private lifecycle = 0;
    private socket?: WebSocket;
    private disconnectMessage = '';
    private heartbeat?: ReturnType<typeof setTimeout>;
    private listeners = new Set<(packet: Packet) => void>();
    private statusListeners = new Set<(status: string) => void>();
    private pending = new Map<number, { responses: readonly number[]; assemble?: (chunk: any) => any | null; ok: (value: any) => void; fail: (error: Error) => void; timer: ReturnType<typeof setTimeout> }>();
    get generation(): number { return this.lifecycle; }
    get connected(): boolean { return this.socket?.readyState === WebSocket.OPEN; }
    get disconnectReason(): string { return this.disconnectMessage; }

    subscribe(listener: (packet: Packet) => void): () => void { this.listeners.add(listener); return () => this.listeners.delete(listener); }
    onStatus(listener: (status: string) => void): () => void { this.statusListeners.add(listener); return () => this.statusListeners.delete(listener); }

    async rpc(id: number, value: any, signal?: AbortSignal): Promise<any> {
        if (signal?.aborted) throw new Error('账号请求已取消');
        const seq = ++this.sequence;
        const controller = new AbortController();
        const cancel = (): void => controller.abort();
        signal?.addEventListener('abort', cancel, { once: true });
        let timedOut = false;
        // 截止时间同时约束响应头和响应体，上层取消也立即传给同一个请求。
        const deadline = setTimeout(() => {
            if (controller.signal.aborted) return;
            timedOut = true;
            cancel();
        }, accountRpcTimeoutMs);
        try {
            const response = await fetch(`${location.origin}/account/rpc`, {
                method: 'POST', headers: { 'Content-Type': 'application/octet-stream', ...(this.token ? { Authorization: `Bearer ${this.token}` } : {}) },
                body: encodePacket(id, seq, Long.UZERO, value), signal: controller.signal,
            });
            if (response.status === 504) throw new Error('账号服务响应超时');
            if (!response.ok) throw new Error(`认证服务返回HTTP ${response.status}`);
            const packet = decodePacket(new Uint8Array(await response.arrayBuffer()));
            if (packet.seq !== seq) throw new Error('认证响应序号不匹配');
            if (packet.error) throw new ProtocolResponseError(packet.id, packet.error);
            return packet.value;
        } catch (error) {
            if (timedOut) throw new Error('账号服务响应超时');
            if (signal?.aborted) throw new Error('账号请求已取消');
            throw error;
        } finally {
            clearTimeout(deadline);
            signal?.removeEventListener('abort', cancel);
        }
    }

    async connect(ticket: string, endpoint: string, signal?: AbortSignal): Promise<void> {
        if (signal?.aborted) throw new Error('游戏连接已取消');
        if (typeof ticket !== 'string' || !ticket) throw new Error('入场票据无效');
        const address = gameSocketAddress(endpoint, location.origin);
        this.close(); this.binding = Long.UZERO;
        const socket = new WebSocket(address); this.socket = socket;
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
        let cancelOpen: (() => void) | undefined;
        const abort = (): void => {
            cancelOpen?.();
            if (this.socket === socket) this.close();
        };
        signal?.addEventListener('abort', abort, { once: true });
        try {
            await new Promise<void>((ok, fail) => {
                const failOpen = (message: string): void => {
                    clearTimeout(timer);
                    fail(new Error(message));
                };
                const timer = setTimeout(() => {
                    failOpen('游戏连接超时');
                    if (this.socket === socket) this.report('游戏连接超时');
                }, 10000);
                cancelOpen = () => failOpen('游戏连接已取消');
                socket.onopen = () => { clearTimeout(timer); ok(); };
                socket.onerror = () => failOpen('游戏连接失败');
                socket.onclose = () => {
                    const current = this.socket === socket;
                    failOpen(current ? '游戏连接已断开' : '旧游戏连接准备已经失效');
                    if (current) this.report('游戏连接已断开');
                };
            });
            if (this.socket !== socket || this.lifecycle !== generation) throw new Error('旧游戏连接准备已经失效');
            await this.request(20001, { accountTicket: ticket });
            if (this.socket !== socket || this.lifecycle !== generation) throw new Error('旧游戏入场回执已经失效');
            this.pollHeartbeat(generation);
        } finally { signal?.removeEventListener('abort', abort); }
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
            const queryResponses: Partial<Record<number, number>> = { 20012: 20014, 20015: 20016, 20017: 20020, 20018: 20020, 20021: 20022, 20025: 20026, 20028: 20029, 40035: 40036, 40039: 40040, 40042: 40043 };
            const responses = id === 20007 ? [20008, 20010] : [cancelNavigation ? 5 : id === 1 ? 2 : id === 20001 ? 20002 : id === 20009 ? 20010 : id === 30004 ? 30005 : id === 30009 ? 30010 : id === 40011 ? 40012 : id === 40020 ? 40021 : id === 40024 ? 40025 : id === 40027 ? 40028 : id === 40032 ? 40033 : queryResponses[id] ?? 40005];
            this.pending.set(seq, { responses, assemble, ok, fail, timer });
            try {
                this.socket!.send(encodePacket(id, seq, this.binding, value));
                traceInput(inputTraceMessage('send', id, seq, value));
            } catch (error) { this.pending.delete(seq); clearTimeout(timer); fail(error instanceof Error ? error : new Error(String(error))); }
        });
    }

    close(): void { this.disconnectMessage = ''; this.closeSocket(); }
    private pollHeartbeat(generation: number): void {
        this.heartbeat = setTimeout(() => {
            if (this.lifecycle !== generation) return;
            if (typeof document !== 'undefined' && document.hidden) { this.pollHeartbeat(generation); return; }
            // 服务器在传输层回复心跳，房间处理或资产回执延迟不代表连接已断开。
            void this.request(1, { clientMonotonicMs: Long.fromNumber(performance.now(), true) }).then(
                () => { if (this.lifecycle === generation) this.pollHeartbeat(generation); },
                () => {
                    if (this.lifecycle !== generation) return;
                    if (typeof document !== 'undefined' && document.hidden) this.pollHeartbeat(generation);
                    else this.report('游戏连接超时');
                },
            );
        }, 10000);
    }
    private closeSocket(): void {
        clearTimeout(this.heartbeat);
        this.lifecycle++;
        const old = this.socket;
        this.socket = undefined;
        old?.close();
        this.rejectPending();
    }
    private rejectPending(): void { for (const item of Array.from(this.pending.values())) { clearTimeout(item.timer); item.fail(new Error('连接已关闭')); } this.pending.clear(); }
    private report(message: string): void {
        this.closeSocket();
        this.disconnectMessage = message;
        for (const listener of Array.from(this.statusListeners)) listener(message);
    }
}

export function newOperationId(): Uint8Array { const bytes = new Uint8Array(16); crypto.getRandomValues(bytes); return bytes; }
