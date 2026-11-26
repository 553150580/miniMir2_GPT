import Long from 'long';
import { GameConnection, Packet, ProtocolResponseError, newOperationId } from './GameConnection';
import { SceneManager } from '../core/scene/SceneManager';
import { BaselineAssembler } from './BaselineAssembler';
import { PendingOperations } from './PendingOperations';
import type { PendingRoleCreation } from './PendingRoleCreation';
import type { OriginServer } from './ServerDirectoryModel';
import type { WorldAssets } from './WorldAssets';
import { protocolErrorMessage } from './GameErrors';
import type { AttackEvent, CastEvent, CastStepEvent, ImpactEvent } from './CastPlayback';
import { acceptsSkillModeState, attackSkillSlotCount, SkillModeState } from './SkillModel';
import type { DisplacementEvent } from './EntityDisplacement';
import { attachmentKey, MapTravelModel, travelInteger } from './MapTravelModel';
import type { AttachmentIdentity } from './MapTravelModel';
import { RoleBindingModel } from './RoleBindingModel';
import type { RoleBindingObservation } from './RoleBindingModel';
import { TeleportModel } from './TeleportModel';
import { BossTransition } from './BossTransition';
import { DungeonTransition } from './DungeonTransition';
import { PetBornModel } from './PetBornModel';
import { EquipmentRecycleModel, observedAutoRecycle } from './AutoRecycleModel';
import { observedWingState } from './WingModel';
import { ExperienceGrantModel, observedProgression } from './LevelGrowthModel';
import { WarehouseReceiptGuard } from './WarehouseModel';
import type { WarehouseReceipt } from './WarehouseModel';
import { validateRoleCardState, type RoleCardState } from './RoleCardProtocol';
import { itemMarketPageSize, validateItemMarketState, type ItemMarketScope, type ItemMarketState } from './ItemMarketProtocol';
import { goldMarketPageSize, validateGoldMarketState, type GoldMarketScope, type GoldMarketState } from './GoldMarketProtocol';
const assembler = new BaselineAssembler();
const bindingModel = new RoleBindingModel();
const recycleModel = new EquipmentRecycleModel();
const experienceModel = new ExperienceGrantModel();
let quiescingReady: { entry: any; binding: string; connection: number; ready: boolean } | null = null;
let confirmedCommand = Long.UONE;
let localOperation: { id: string; connection: number; binding: string; role: string; scene: string; baseline: string; next: Long } | null = null;
let marketRead: Promise<any> | null = null; // 两个市场共用服务端的角色只读预览槽。

// 连接与数据跨场景保留，不通过持久UI节点重建固定布局。
export const session = {
    connection: new GameConnection(),
    scenes: null as SceneManager | null,
    account: null as { id: string; name: string } | null,
    originServer: null as OriginServer | null,
    role: null as any,
    roles: [] as any[],
    entry: null as any,
    entryBinding: '0',
    entryConnection: 0,
    entryRole: '',
    entryTravelId: '',
    entryServerTransitionId: '',
    entryBossTransition: false,
    bossTransition: new BossTransition(),
    entryDungeonTransition: false,
    dungeonTransition: new DungeonTransition(),
    teleport: new TeleportModel(),
    petBorn: new PetBornModel(),
    // 表现只量接收后的单调时长，不拿客户端Date与服务器Unix做差。
    presentationNow: (): number => performance.now(),
    travel: new MapTravelModel(),
    travelState: null as any,
    bindingState: null as RoleBindingObservation | null,
    bindingNotice: null as RoleBindingObservation | null,
    baseline: null as any,
    inventory: null as any,
    wallet: null as any,
    hosting: null as any,
    skillModes: null as SkillModeState | null,
    skillModesReceivedAt: 0,
    report: null as any,
    recycleNotice: null as import('./AutoRecycleModel').RecycleNotice | null,
    experienceNotice: null as import('./LevelGrowthModel').ExperienceNotice | null,
    warehouseReceipts: new WarehouseReceiptGuard(),
    warehouseResult: null as WarehouseReceipt | null,
    warehouseNotice: null as { role: string; op: string; scope: number; action: number; scene: string; baseline: string } | null,
    progressionCurve: null as string | null,
    death: null as any,
    ready: false,
    command: Long.UONE,
    pending: null as PendingOperations | null,
    roleCreation: null as PendingRoleCreation | null,
    preparedWorld: null as WorldAssets | null,
    lastError: '',
    update: new Set<() => void>(),
    casts: new Set<(event: CastEvent) => void>(),
    attacks: new Set<(event: AttackEvent) => void>(),
    impacts: new Set<(event: ImpactEvent) => void>(),
    castSteps: new Set<(event: CastStepEvent) => void>(),
    displacements: new Set<(event: DisplacementEvent) => void>(),
    quickSkills: new Map<string, number>(),
};
session.connection.subscribe((packet: Packet) => {
    if (packet.error) {
        // 月卡能力探测只有打开托管时触发，明确拒绝由对应请求处理，不污染旧服状态栏。
        if (![40035, 40036, 40039, 40040, 40042, 40043].includes(packet.id)) session.lastError = protocolErrorMessage(packet.id, packet.error);
    }
    else switch (packet.id) {
        case 20002:
            acceptEntry(packet); break;
        case 20023:
        case 20024:
            if (session.entry && session.entryBinding === packet.binding.toString() && session.entryConnection === session.connection.generation
                && !session.travel.active && !session.teleport.blocksInput) {
                try {
                    if (packet.id === 20024) session.dungeonTransition.observe(packet.value, session.entry);
                    else session.dungeonTransition.observeProgress(packet.value, session.entry);
                }
                catch (error) { session.lastError = error instanceof Error ? error.message : String(error); }
            }
            break;
        case 20014:
        case 20020:
            if (session.entryBinding === packet.binding.toString() && session.entryConnection === session.connection.generation) {
                try {
                    const value = packet.value;
                    session.bossTransition.observe(packet.id === 20014 ? 3 : 4, value.challengeId ?? value.occurrenceId, value.state, value.sceneEpoch, session.entry, value.errorCode);
                } catch (error) { session.lastError = error instanceof Error ? error.message : String(error); }
            }
            break;
        case 5:
            if (session.entry) {
                const complete = assembler.accept(packet.value, session.entry.sceneEpoch, session.entry.baselineId, session.entry.roomId);
                if (complete && (!session.baseline || complete.stateSequence.greaterThan(session.baseline.stateSequence))) {
                    session.baseline = complete;
                    if (complete.skillModeState) acceptModes(complete.skillModeState);
                }
            }
            break;
        case 6:
            if (session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch) && packet.value.baselineId.equals(session.entry.baselineId)
                && (!session.travel.blocksInput || (!!session.entryTravelId && session.travel.phase === 'awaiting_ready'))
                && (!session.teleport.blocksInput || (!!session.entryServerTransitionId && session.teleport.phase === 'awaiting_ready'))) {
                session.ready = true;
                const next = packet.value.nextEconomicCommandSeq;
                if (next.greaterThan(confirmedCommand)) confirmedCommand = next;
                if (confirmedCommand.greaterThan(session.command)) session.command = confirmedCommand;
                const identity = currentEntryIdentity(); if (identity && session.entryTravelId) session.travel.ready(identity);
                if (identity && session.entryServerTransitionId && !session.entryBossTransition && !session.entryDungeonTransition) session.teleport.ready(identity);
            }
            break;
        case 20010:
            if (session.role && session.travel.acceptState(packet.value, { connection: session.connection.generation, binding: packet.binding, role: roleIdentity() })) {
                session.travelState = packet.value;
                if (packet.value.phase === 1) {
                    quiescingReady ??= { entry: session.entry, binding: session.entryBinding, connection: session.entryConnection, ready: session.ready };
                    session.ready = false;
                } else if (packet.value.phase === 2) {
                    if (session.entry?.sceneEpoch.toString() === packet.value.fromSceneEpoch.toString()) session.ready = false;
                    quiescingReady = null;
                } else {
                    if (quiescingReady && session.entry === quiescingReady.entry && session.entryBinding === quiescingReady.binding
                        && session.connection.generation === quiescingReady.connection && session.entry?.sceneEpoch.toString() === packet.value.fromSceneEpoch.toString()) session.ready = quiescingReady.ready;
                    quiescingReady = null;
                }
            }
            break;
        case 40003:
            try {
                if (session.inventory && packet.value.revision.lessThan(session.inventory.revision)) break;
                if (packet.value.autoRecycle) packet.value.autoRecycle = observedAutoRecycle(packet.value.autoRecycle);
                if (packet.value.wing || session.inventory?.wing) packet.value.wing = observedWingState(packet.value.wing, session.inventory?.wing);
                if (packet.value.progression) packet.value.progression = observedProgression(packet.value.progression);
                recycleModel.synchronize(packet.value.revision);
                experienceModel.synchronize(packet.value.revision);
                session.inventory = packet.value;
            } catch (error) { session.lastError = error instanceof Error ? error.message : String(error); }
            break;
        case 20011: {
            const identity = currentEntryIdentity();
            if (session.role && identity && isCurrentEntry(identity) && bindingModel.accept(packet.value, identity, packet.binding)) {
                session.bindingState = bindingModel.state; session.bindingNotice = bindingModel.notice;
            }
            break;
        }
        case 40004: session.wallet = packet.value; break;
        case 30005: session.hosting = packet.value; break;
        case 30010: acceptModes(packet.value); break;
        case 30001: session.death = packet.value; break;
        case 30006:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.casts)) listener(packet.value);
            }
            break;
        case 30007:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.attacks)) listener(packet.value);
            }
            break;
        case 30008:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.impacts)) listener(packet.value);
            }
            break;
        case 30012:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.castSteps)) listener(packet.value);
            }
            break;
        case 30013:
            if ((session.ready || session.travel.phase === 'quiescing') && session.entry && packet.value.sceneEpoch.equals(session.entry.sceneEpoch)) {
                for (const listener of Array.from(session.displacements)) listener(packet.value);
            }
            break;
        case 30014: {
            const identity = currentEntryIdentity();
            const player = session.baseline?.entities.find((entity: any) => entity.entityHandle === session.entry?.playerHandle);
            if (session.role && identity && player && isCurrentEntry(identity) && session.teleport.accept(packet.value, identity, packet.binding, player.life)) {
                if (session.teleport.blocksInput) {
                    // 迁移已由服务端提交；旧手动旅行及其异步预备资源不能升级成此次授权。
                    session.travel.reset(); session.travelState = null; session.ready = false; quiescingReady = null;
                }
            }
            break;
        }
        case 30015: {
            const identity = currentEntryIdentity();
            if ((session.ready || session.travel.phase === 'quiescing') && session.role && identity && isCurrentEntry(identity) && BigInt(session.entry.resourceVersion.toString()) >= 18n)
                session.petBorn.accept(packet.value, identity, packet.binding, session.presentationNow());
            break;
        }
        case 40008: session.report = packet.value; break;
        case 40018: {
            const identity = currentEntryIdentity();
            if ((session.ready || session.travel.phase === 'quiescing') && session.role && identity && isCurrentEntry(identity)
                && packet.binding.toString() === identity.binding.toString() && recycleModel.accept(packet.value, roleIdentity())) session.recycleNotice = recycleModel.notice;
            break;
        }
        case 40019: {
            const identity = currentEntryIdentity(), curve = session.progressionCurve;
            if ((session.ready || session.travel.phase === 'quiescing') && session.role && identity && isCurrentEntry(identity)
                && packet.binding.toString() === identity.binding.toString() && BigInt(session.entry.resourceVersion.toString()) >= 20n
                && typeof curve === 'string' && session.inventory?.progression?.curveVersion === curve
                && experienceModel.accept(packet.value, roleIdentity(), session.entry.resourceVersion, curve)) session.experienceNotice = experienceModel.notice;
            break;
        }
        case 40023: {
            const identity = currentEntryIdentity();
            if (session.role && identity && isCurrentEntry(identity) && packet.binding.toString() === identity.binding.toString()
                && session.warehouseReceipts.accept(packet.value, attachmentKey(identity), roleIdentity())) {
                session.warehouseResult = packet.value;
                session.warehouseNotice = { role: roleIdentity(), op: bytesIdentity(packet.value.opId), scope: packet.value.scope, action: packet.value.action,
                    scene: identity.scene.toString(), baseline: identity.baseline.toString() };
            }
            break;
        }
        case 40005: acceptOperationReceipt(packet); break;
    }
    notify();
});
session.connection.onStatus(message => { session.lastError = message; session.ready = false; session.travel.reset(); session.teleport.clear(); session.bossTransition.clear(); session.dungeonTransition.clear(); session.petBorn.clear(); bindingModel.clear(); session.bindingState = null; session.bindingNotice = null; recycleModel.detach(); session.recycleNotice = null; experienceModel.detach(); session.experienceNotice = null; session.progressionCurve = null; quiescingReady = null; localOperation = null; notify(); });

function acceptEntry(packet: Packet): void {
    const value = packet.value;
    try {
        if (!Number.isInteger(value.playerHandle) || value.playerHandle < 1 || value.playerHandle > 0xffffffff || ![1, 2, 3, 4, 5, 7].includes(value.sceneKind)) throw new Error('入场缺少有效玩家身份或地图类型');
        travelInteger(value.sceneEpoch, true); travelInteger(value.baselineId, true); travelInteger(value.roomId, true); travelInteger(value.resourceVersion, true);
        const sameConnection = session.entry && session.entryBinding === packet.binding.toString() && session.entryConnection === session.connection.generation;
        if (sameConnection && BigInt(value.sceneEpoch.toString()) <= BigInt(session.entry.sceneEpoch.toString())) {
            // 同连接已附着的场景不能由重复或迟到Enter回执退回；也不撤销新场景Ready。
            return;
        }
        const transitionId = value.serverTransitionId ?? '';
        if (typeof transitionId !== 'string' || (transitionId && !/^[0-9a-f]{32}$/.test(transitionId))) throw new Error('入场服务端迁移身份无效');
        const identity: AttachmentIdentity = { connection: session.connection.generation, binding: packet.binding, role: roleIdentity(), scene: value.sceneEpoch,
            baseline: value.baselineId, map: value.roomId, travelId: transitionId ? '' : session.travel.id, serverTransitionId: transitionId };
        const forced = !!transitionId && session.teleport.matchesEntry(identity, value.position);
        const boss = !!(sameConnection && transitionId && !forced && !session.teleport.blocksInput && !session.travel.active
            && session.bossTransition.accept(transitionId, value, session.entry));
        const dungeon = !!(sameConnection && transitionId && !forced && !boss && !session.teleport.blocksInput && !session.travel.active
            && session.dungeonTransition.accept(transitionId, value, session.entry));
        if (([3, 4, 5].includes(value.sceneKind) && !boss && !dungeon) || (transitionId && !forced && !boss && !dungeon)) throw new Error('新地图入场缺少匹配的服务端迁移结果');
        if (sameConnection && session.teleport.blocksInput && !forced) throw new Error('新地图入场不匹配服务端瞬移身份');
        const travelling = !forced && session.travel.acceptsEntry(identity);
        if (sameConnection && value.sceneKind === 7 && !travelling) throw new Error('矿区入场不匹配原旅行提交身份');
        if (sameConnection && session.travel.active && !travelling && !forced) throw new Error('新地图入场不匹配旅行提交身份');
        if (travelling && value.sceneKind !== session.travel.plan?.targetSceneKind) throw new Error('新地图入场类型不匹配旅行计划');
        if (forced) session.teleport.attach(identity, value.position);
        if (travelling) session.travel.attach(identity); else session.travel.reset();
        assembler.reset(); session.entry = value; session.baseline = null; session.ready = false; quiescingReady = null;
        session.entryBinding = packet.binding.toString(); session.entryConnection = session.connection.generation; session.entryRole = identity.role; session.entryTravelId = travelling ? identity.travelId : '';
        session.entryServerTransitionId = transitionId;
        session.entryBossTransition = boss;
        session.entryDungeonTransition = dungeon;
        if (!sameConnection) { session.bossTransition.clear(); session.dungeonTransition.clear(); }
        if (!forced) session.teleport.bind(currentEntryIdentity()!);
        bindingModel.bind(currentEntryIdentity()!); session.bindingState = null; session.bindingNotice = null;
        session.petBorn.bind(currentEntryIdentity()!);
        recycleModel.attach(roleIdentity()); session.recycleNotice = null;
        experienceModel.attach(roleIdentity()); session.experienceNotice = null;
        session.progressionCurve = null;
        session.warehouseReceipts.bind(attachmentKey(currentEntryIdentity()!)); session.warehouseResult = null; session.warehouseNotice = null;
        session.inventory = null; session.wallet = null; session.hosting = null; session.report = null; session.death = null; session.skillModes = null; session.skillModesReceivedAt = 0;
        confirmedCommand = value.nextEconomicCommandSeq ?? Long.UONE; session.command = confirmedCommand; localOperation = null;
    } catch (error) { session.ready = false; session.lastError = error instanceof Error ? error.message : String(error); }
}
export function currentEntryIdentity(): AttachmentIdentity | null {
    const entry = session.entry; if (!entry) return null;
    return { connection: session.entryConnection, binding: session.entryBinding, role: session.entryRole,
        scene: entry.sceneEpoch, baseline: entry.baselineId, map: entry.roomId, travelId: session.entryTravelId, serverTransitionId: session.entryServerTransitionId };
}
export function isCurrentEntry(identity: AttachmentIdentity): boolean {
    const current = currentEntryIdentity();
    return !!current && session.connection.generation === identity.connection && session.connection.binding.toString() === identity.binding.toString()
        && roleIdentity() === identity.role && current.connection === identity.connection && current.binding.toString() === identity.binding.toString()
        && current.scene.toString() === identity.scene.toString() && current.baseline.toString() === identity.baseline.toString()
        && current.map.toString() === identity.map.toString() && current.travelId === identity.travelId
        && (current.serverTransitionId ?? '') === (identity.serverTransitionId ?? '');
}

// queryRoleCard 只接受当前角色同一连接和基线上返回的权威卡目录或原报价。
export async function queryRoleCard(productId = 0): Promise<RoleCardState> {
    const identity = currentEntryIdentity();
    if (!session.role || !session.ready || !identity || !isCurrentEntry(identity)) throw new Error('角色月卡状态尚未同步');
    if (!Number.isInteger(productId) || productId < 0 || productId > 0xffffffff) throw new Error('角色月卡商品编号无效');
    const sceneEpoch: Long = session.entry.sceneEpoch;
    const result = await session.connection.request(40035, { sceneEpoch, productId });
    if (!session.role || !session.ready || !isCurrentEntry(identity)) throw new Error('角色或地图已切换，请重新查看月卡');
    if (result?.errorCode) throw new ProtocolResponseError(40036, result.errorCode);
    return validateRoleCardState(result, sceneEpoch, productId);
}

// 道具市场查询只应用于原角色的当前普通场景。分页令牌由服务端签发并原样使用。
export async function queryItemMarket(scope: ItemMarketScope, cursor = new Uint8Array(0)): Promise<ItemMarketState> {
    const { result, sceneEpoch } = await requestMarketPage(40039, itemMarketPageSize, scope, cursor, '道具市场');
    return validateItemMarketState(result, sceneEpoch, scope);
}

// 金币市场只应用当前角色附着的原条款和分页结果。
export async function queryGoldMarket(scope: GoldMarketScope, cursor = new Uint8Array(0)): Promise<GoldMarketState> {
    const { result, sceneEpoch } = await requestMarketPage(40042, goldMarketPageSize, scope, cursor, '金币市场');
    return validateGoldMarketState(result, sceneEpoch, scope);
}

async function requestMarketPage(message: 40039 | 40042, pageSize: number, scope: number, cursor: Uint8Array, label: string): Promise<{ result: any; sceneEpoch: Long }> {
    const identity = currentEntryIdentity();
    if (!session.role || !session.ready || !identity || !isCurrentEntry(identity)) throw new Error(`${label}尚未同步`);
    const sceneEpoch: Long = session.entry.sceneEpoch;
    const current = (): boolean => !!session.role && session.ready && isCurrentEntry(identity);
    const read = async (): Promise<any> => {
        if (!current()) throw new Error(`角色或地图已切换，请重新查看${label}`);
        return session.connection.request(message, { sceneEpoch, scope, pageSize, cursor });
    };
    // 打开原功能栏会同时发现两种市场，等上一读回包释放原角色槽再发下一读。
    const queued = marketRead ? marketRead.then(read, read) : read();
    marketRead = queued;
    const finish = (): void => { if (marketRead === queued) marketRead = null; };
    void queued.then(finish, finish);
    const result = await queued;
    if (!current()) throw new Error(`角色或地图已切换，请重新查看${label}`);
    if (result?.errorCode) throw new ProtocolResponseError(message + 1, result.errorCode);
    return { result, sceneEpoch };
}

function notify(): void { for (const update of Array.from(session.update)) { try { update(); } catch (error) { session.lastError = error instanceof Error ? error.message : String(error); } } }
function acceptModes(value: SkillModeState): void {
    if (session.entry && acceptsSkillModeState(session.skillModes, value, session.entry.sceneEpoch)) {
        session.skillModes = value; session.skillModesReceivedAt = Date.now();
    }
}

export function roleIdentity(): string {
    if (!session.role) throw new Error('尚未选择角色');
    return session.role.key.originServerId.toString() + ':' + Array.from(session.role.key.roleId as Uint8Array, value => value.toString(16).padStart(2, '0')).join('');
}

export function operation(expectedSkillSlotIds?: readonly number[]): any {
    if (!session.ready) throw new Error('场景基线尚未确认');
    const inventory = session.inventory, observedSlots = inventory?.skillSlotIds;
    // 只有带原四槽的编辑可用服务端槽位比较；协议仍携带实际读到的库存修订。
    const editingConfirmedSlots = expectedSkillSlotIds?.length === attackSkillSlotCount &&
        observedSlots?.length === attackSkillSlotCount && !inventory.revision.isZero() &&
        expectedSkillSlotIds.every((id, index) => id === observedSlots[index]);
    if (session.bindingState && (!inventory || BigInt(session.bindingState.roleRevision) > BigInt(inventory.revision.toString()) && !editingConfirmedSlots))
        throw new Error('角色资料正在同步，请稍后操作');
    const result = { opId: newOperationId(), commandSeq: session.command,
        expectedRevision: inventory?.revision ?? Long.UZERO,
        expectedRuleVersion: session.entry?.resourceVersion ?? Long.UONE };
    session.command = session.command.add(1);
    localOperation = { id: bytesIdentity(result.opId), connection: session.connection.generation, binding: session.connection.binding.toString(),
        role: roleIdentity(), scene: session.entry.sceneEpoch.toString(), baseline: session.entry.baselineId.toString(), next: session.command };
    return result;
}

function bytesIdentity(value: Uint8Array): string { return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join(''); }
function acceptOperationReceipt(packet: Packet): void {
    const value = packet.value; if (value.pending) return;
    const next = value.commandSeq.add(1); if (next.greaterThan(confirmedCommand)) confirmedCommand = next;
    const current = localOperation && value.opId instanceof Uint8Array && bytesIdentity(value.opId) === localOperation.id
        && session.connection.generation === localOperation.connection && packet.binding.toString() === localOperation.binding
        && session.entryRole === localOperation.role && session.entry?.sceneEpoch.toString() === localOperation.scene
        && session.entry?.baselineId.toString() === localOperation.baseline && session.command.equals(localOperation.next);
    // 当前op无副作用拒绝可以撤回本地推测；其它回执不能把新场景/新op水位拉低。
    if (current) { session.command = confirmedCommand; localOperation = null; }
    else if (confirmedCommand.greaterThan(session.command)) session.command = confirmedCommand;
}
