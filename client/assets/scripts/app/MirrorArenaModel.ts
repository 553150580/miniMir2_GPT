import { travelInteger } from './MapTravelModel';
import type { TravelInteger } from './MapTravelModel';
import type { AttackEvent, CastEvent, CastStepEvent, ImpactEvent } from './CastPlayback';
import type { DisplacementEvent } from './EntityDisplacement';

// 镜像展示使用独立的场次、角色投影和观测流水。
export const MirrorArenaPhase = { Preparing: 1, Fighting: 2, Settling: 3, Finished: 4 } as const;
export type MirrorPhase = typeof MirrorArenaPhase[keyof typeof MirrorArenaPhase];

export interface MirrorArenaOffer {
    readonly id: string; readonly offerId: Uint8Array;
    readonly defenderName: string; readonly defenderJob: number; readonly defenderLevel: number; readonly defenseVersion: string;
}
export interface MirrorArenaRecord {
    readonly id: string; readonly challengeId: Uint8Array;
    readonly attackerName: string; readonly defenderName: string; readonly defenseVersion: string;
    readonly outcome: number; readonly awardedPoints: string; readonly startedAtUnixMs: string; readonly finishedAtUnixMs: string;
}
export interface MirrorArenaLobby {
    readonly open: boolean; readonly seasonStartUnixMs: string; readonly seasonEndUnixMs: string;
    readonly dailyLimit: number; readonly entriesUsed: number; readonly seasonPoints: string; readonly streak: number;
    readonly offers: readonly MirrorArenaOffer[]; readonly attackHistory: readonly MirrorArenaRecord[];
    readonly defenseHistory: readonly MirrorArenaRecord[]; readonly publishedDefenseVersion: string;
    readonly season: MirrorSeasonLedger | null;
}
export interface MirrorStanding {
    readonly rank: number; readonly name: string; readonly points: string; readonly matches: number; readonly lastMatchAtUnixMs: string;
}
export interface MirrorSeasonReward {
    readonly id: string; readonly rewardId: Uint8Array; readonly name: string; readonly rank: number; readonly boundYuanbao: string;
}
export interface MirrorSeasonLedger {
    readonly standings: readonly MirrorStanding[]; readonly ownRank: number;
    readonly last: { readonly rank: number; readonly points: string; readonly credit: string; readonly matches: number } | null;
    readonly rewards: readonly MirrorSeasonReward[]; readonly moreRewards: boolean;
    readonly points: string;
    readonly exchange: { readonly price: string; readonly itemId: number; readonly quantity: string } | null;
}
export interface MirrorBattle {
    readonly key: string; readonly challenge: MirrorArenaRecord; readonly phase: MirrorPhase;
    readonly epoch: string; readonly mapId: string; readonly deadlineUnixMs: string;
}
export type MirrorCombatEvent =
    | { readonly messageId: 30006; readonly value: CastEvent }
    | { readonly messageId: 30007; readonly value: AttackEvent }
    | { readonly messageId: 30008; readonly value: ImpactEvent }
    | { readonly messageId: 30012; readonly value: CastStepEvent }
    | { readonly messageId: 30013; readonly value: DisplacementEvent };
export type MirrorCombatMessageId = MirrorCombatEvent['messageId'];
export type DecodeMirrorCombat = (id: MirrorCombatMessageId, payload: Uint8Array) => unknown;
export interface MirrorEntity {
    readonly entityHandle: number; readonly entityKind: number; readonly definitionId: number;
    readonly position: { x: number; y: number }; readonly direction: number; readonly life: TravelInteger;
    readonly hp: TravelInteger; readonly hpMax: TravelInteger; readonly mp: TravelInteger; readonly mpMax: TravelInteger;
    readonly displayName?: string;
}
export interface MirrorArenaFrame {
    readonly battleKey: string; readonly sequence: string; readonly serverUnixMs: string;
    readonly entities: readonly MirrorEntity[]; readonly observations: readonly MirrorCombatEvent[];
    readonly fireWallCells: readonly { x: number; y: number }[];
}
const maxMirrorGridSide = 17; // 与服务端挑战地图的尺寸门禁一致；实际地图范围由独立视口再次核对。
export interface MirrorArenaSnapshot {
    readonly enabled: boolean; readonly attached: boolean; readonly pending: boolean; readonly errorCode: number;
    readonly lobby: MirrorArenaLobby | null; readonly battle: MirrorBattle | null; readonly frame: MirrorArenaFrame | null;
}

interface MirrorState {
    readonly errorCode: number; readonly lobby: MirrorArenaLobby; readonly challenge: MirrorArenaRecord | null;
    readonly phase: MirrorPhase | 0; readonly epoch: string; readonly mapId: string; readonly deadlineUnixMs: string;
}

// 业务标识以十六进制用于比较，协议请求保留原 16 字节。
export function mirrorID(value: unknown): string {
    if (!(value instanceof Uint8Array) || value.length !== 16 || !value.some(byte => byte !== 0)) return '';
    return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
}

function fields(value: unknown): Record<string, unknown> {
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('镜像竞技字段缺失');
    return value as Record<string, unknown>;
}
function u32(value: unknown, max = 0xffffffff): number {
    if (typeof value !== 'number' || !Number.isInteger(value) || value < 0 || value > max) throw new Error('镜像竞技计数无效');
    return value;
}
function u64(value: unknown, positive = false): string {
    return travelInteger(value as TravelInteger, positive);
}
function nonnegativeI64(value: unknown, positive = false): string {
    const parsed = u64(value, positive);
    if (BigInt(parsed) > 9223372036854775807n) throw new Error('镜像竞技时间或分值无效');
    return parsed;
}
function text(value: unknown): string {
    if (typeof value !== 'string' || !value.trim() || value.length > 128) throw new Error('镜像竞技名称无效');
    return value;
}
function list(value: unknown, limit: number): readonly unknown[] {
    if (!Array.isArray(value) || value.length > limit) throw new Error('镜像竞技列表超限');
    return value;
}
function unique<T extends { id: string }>(values: T[]): readonly T[] {
    if (new Set(values.map(value => value.id)).size !== values.length) throw new Error('镜像竞技列表身份重复');
    return values;
}
function record(value: unknown): MirrorArenaRecord {
    const row = fields(value), id = mirrorID(row.challengeId);
    if (!id) throw new Error('镜像挑战身份无效');
    const outcome = u32(row.outcome, 3), awardedPoints = nonnegativeI64(row.awardedPoints);
    const startedAtUnixMs = nonnegativeI64(row.startedAtUnixMs, true), finishedAtUnixMs = nonnegativeI64(row.finishedAtUnixMs);
    if (outcome === 0 ? finishedAtUnixMs !== '0' || awardedPoints !== '0' : BigInt(finishedAtUnixMs) < BigInt(startedAtUnixMs))
        throw new Error('镜像竞技结果尚未确认');
    return { id, challengeId: new Uint8Array(row.challengeId as Uint8Array), attackerName: text(row.attackerName), defenderName: text(row.defenderName),
        defenseVersion: u64(row.defenseVersion, true), outcome, awardedPoints, startedAtUnixMs, finishedAtUnixMs };
}
function offer(value: unknown): MirrorArenaOffer {
    const row = fields(value), id = mirrorID(row.offerId), defenderLevel = u32(row.defenderLevel);
    if (!id || !defenderLevel) throw new Error('镜像对手身份无效');
    return { id, offerId: new Uint8Array(row.offerId as Uint8Array), defenderName: text(row.defenderName),
        defenderJob: u32(row.defenderJob, 2), defenderLevel, defenseVersion: u64(row.defenseVersion, true) };
}

// 排名、待领和报价各自保留权威快照。旧协议或仅含战斗状态的帧不刷新结算视图。
function readSeasonLedger(state: Record<string, unknown>): MirrorSeasonLedger | null {
    if (!state.seasonSettlementEnabled && !state.lastSettlement && !state.seasonPendingRewardsMore &&
        (!Array.isArray(state.seasonPendingRewards) || !state.seasonPendingRewards.length) &&
        (state.exchangePoints == null || nonnegativeI64(state.exchangePoints) === '0')) return null;
    const standings = list(state.standings ?? [], 20).map((value, index) => {
        const row = fields(value), rank = u32(row.rank);
        if (rank !== index + 1) throw new Error('镜像赛季榜单顺位无效');
        return { rank, name: text(row.name), points: nonnegativeI64(row.points), matches: u32(row.matches),
            lastMatchAtUnixMs: nonnegativeI64(row.lastMatchAtUnixMs, true) };
    });
    const rewards = unique(list(state.seasonPendingRewards ?? [], 20).map(value => {
        const row = fields(value), id = mirrorID(row.rewardId), rank = u32(row.sourceRank);
        const ranking = row.sourceKind === 'mirror_season_rank';
        const name = ranking ? '赛季排名奖' : row.sourceKind === 'mirror_season_participation' ? '赛季参与奖' : '';
        if (!id || !name || ranking !== (rank > 0)) throw new Error('镜像赛季奖励身份无效');
        return { id, name, rewardId: new Uint8Array(row.rewardId as Uint8Array), rank, boundYuanbao: nonnegativeI64(row.boundYuanbaoUnits, true) };
    }));
    const last = state.lastSettlement == null ? null : fields(state.lastSettlement);
    const points = nonnegativeI64(state.exchangePoints ?? '0');
    const exchangePrice = nonnegativeI64(state.exchangePrice ?? '0');
    const exchangeQuantity = nonnegativeI64(state.exchangePackQuantity ?? '0'), itemId = u32(state.exchangeItemId ?? 0);
    const exchange = itemId || exchangePrice !== '0' || exchangeQuantity !== '0' ? { price: exchangePrice, quantity: exchangeQuantity, itemId } : null;
    if (exchange && (!itemId || exchangePrice === '0' || exchangeQuantity === '0')) throw new Error('镜像兑换报价不完整');
    if (last && (!mirrorID(last.seasonId) || !u32(last.rank) || nonnegativeI64(last.points) !== nonnegativeI64(last.exchangeCredit)))
        throw new Error('镜像赛季原结转回执不一致');
    return { standings, ownRank: u32(state.ownRank ?? 0), rewards, moreRewards: state.seasonPendingRewardsMore === true, points, exchange,
        last: last && { rank: u32(last.rank), matches: u32(last.matches), points: nonnegativeI64(last.points), credit: nonnegativeI64(last.exchangeCredit) } };
}

// 仅解读当前附着的普通场景代次与版本；战斗阶段包可缺省次数和名单。
export function readMirrorState(value: unknown, scene: string, rule: string): MirrorState | null {
    try {
        const state = fields(value);
        if (u64(state.sceneEpoch, true) !== scene || u64(state.ruleVersion, true) !== rule || typeof state.open !== 'boolean') return null;
        const errorCode = u32(state.errorCode), phase = u32(state.battleState, 4) as MirrorPhase | 0;
        const epoch = u64(state.battleSceneEpoch), mapId = u64(state.battleMapId);
        const deadlineUnixMs = nonnegativeI64(state.battleDeadlineUnixMs);
        const challenge = state.challenge == null ? null : record(state.challenge);
        const open = state.open, seasonStartUnixMs = nonnegativeI64(state.seasonStartUnixMs), seasonEndUnixMs = nonnegativeI64(state.seasonEndUnixMs);
        const dailyLimit = u32(state.dailyLimit), entriesUsed = u32(state.entriesUsed);
        const lobby: MirrorArenaLobby = { open, seasonStartUnixMs, seasonEndUnixMs, dailyLimit, entriesUsed,
            seasonPoints: nonnegativeI64(state.seasonPoints), streak: u32(state.streak),
            publishedDefenseVersion: u64(state.publishedDefenseVersion),
            offers: unique(list(state.offers, 100).map(offer)),
            attackHistory: unique(list(state.attackHistory, 100).map(record)), defenseHistory: unique(list(state.defenseHistory, 100).map(record)),
            season: readSeasonLedger(state) };
        if (lobby.attackHistory.some(row => !row.outcome) || lobby.defenseHistory.some(row => !row.outcome) ||
            open && (seasonStartUnixMs === '0' || BigInt(seasonEndUnixMs) <= BigInt(seasonStartUnixMs)) || entriesUsed > dailyLimit ||
            errorCode && (open || phase || challenge) || !open && (dailyLimit || challenge || phase)) return null;
        if (!phase && (epoch !== '0' || mapId !== '0' || deadlineUnixMs !== '0' || challenge?.outcome)) return null;
        if (phase && (!challenge || epoch === '0' || mapId === '0' ||
            (phase <= MirrorArenaPhase.Fighting && deadlineUnixMs === '0') ||
            (phase === MirrorArenaPhase.Finished) !== (challenge.outcome !== 0))) return null;
        return { errorCode, lobby, challenge, phase, epoch, mapId, deadlineUnixMs };
    } catch { return null; }
}

export function isMirrorCombatMessage(value: unknown): value is MirrorCombatMessageId {
    return value === 30006 || value === 30007 || value === 30008 || value === 30012 || value === 30013;
}

function validEntity(value: unknown): value is MirrorEntity {
    const entity = fields(value), handle = u32(entity.entityHandle), kind = u32(entity.entityKind);
    if (handle === 1 || handle === 2) {
        if (kind !== 1) return false;
        u32(entity.definitionId, 2);
    } else if ((handle !== 101 && handle !== 102) || kind !== 2 || !u32(entity.definitionId)) return false;
    gridPoint(entity.position);
    u32(entity.direction, 7);
    u64(entity.life, true);
    return BigInt(nonnegativeI64(entity.hp)) <= BigInt(nonnegativeI64(entity.hpMax, true)) &&
        BigInt(nonnegativeI64(entity.mp)) <= BigInt(nonnegativeI64(entity.mpMax));
}

function gridPoint(value: unknown): { x: number; y: number } {
    const point = fields(value);
    return { x: u32(point.x, maxMirrorGridSide - 1), y: u32(point.y, maxMirrorGridSide - 1) };
}

// 只解码镜像允许的五种表演事件，未知或残缺的事件不会影响合法的整帧实体。
function readObservation(value: unknown, epoch: string, decode: DecodeMirrorCombat): MirrorCombatEvent | null {
    try {
        const { messageId, payload } = fields(value);
        if (!isMirrorCombatMessage(messageId) || !(payload instanceof Uint8Array) || !payload.length) return null;
        const event = fields(decode(messageId, payload));
        if (u64(event.sceneEpoch, true) !== epoch) return null;
        if (messageId === 30013) {
            if (![1, 2, 101, 102].includes(u32(event.entityHandle))) return null;
            u64(event.entityLife, true);
            u64(event.movementRevision, true);
        } else {
            // 跨时刻飞行道具可能在来源身体已阵亡后命中，仍验证它的原句柄和同场身份。
            const handles = [0, 1, 2, 101, 102], caster = u32(event.casterHandle);
            if (!handles.includes(caster) || !caster && messageId !== 30008) return null;
            if (messageId !== 30012 && !handles.includes(u32(event.targetHandle))) return null;
            if (messageId === 30006 || messageId === 30012) u64(event.castId, true);
            else u64(event.eventId, true);
        }
        return { messageId, value: event } as unknown as MirrorCombatEvent;
    } catch { return null; }
}

// 原场、连接、双方身体和整帧序号均通过后才交给独立视口。
export function readMirrorFrame(value: unknown, scene: string, battle: MirrorBattle, lastSequence: string, decode: DecodeMirrorCombat): MirrorArenaFrame | null {
    try {
        if (battle.phase < MirrorArenaPhase.Fighting || battle.phase >= MirrorArenaPhase.Finished) return null;
        const frame = fields(value);
        if (u64(frame.sceneEpoch, true) !== scene || mirrorID(frame.challengeId) !== battle.challenge.id ||
            u64(frame.battleSceneEpoch, true) !== battle.epoch) return null;
        const sequence = u64(frame.sequence, true), serverUnixMs = nonnegativeI64(frame.serverUnixMs, true);
        if (BigInt(sequence) <= BigInt(lastSequence)) return null;
        const entities = list(frame.entities, 4);
        if (entities.length < 2 || !entities.every(validEntity)) return null;
        const handles = new Set(entities.map(entity => entity.entityHandle));
        if (handles.size !== entities.length || !handles.has(1) || !handles.has(2)) return null;
        const fireWallCells = list(frame.fireWallCells ?? [], maxMirrorGridSide * maxMirrorGridSide).map(gridPoint);
        if (new Set(fireWallCells.map(point => point.x + point.y * maxMirrorGridSide)).size !== fireWallCells.length) return null;
        const observations: MirrorCombatEvent[] = [];
        for (const raw of list(frame.observations, 64)) {
            const event = readObservation(raw, battle.epoch, decode);
            if (event) observations.push(event);
        }
        return { battleKey: battle.key, sequence, serverUnixMs, entities, observations, fireWallCells };
    } catch { return null; }
}
