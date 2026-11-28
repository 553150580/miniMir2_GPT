// 本文件读取战令权威档位，并用整数核对实际到账差额。
import Long from 'long';
import { assetId } from './SkillAssetsModel';

export interface BattlePassReward {
    boundGold: Long; // 绑定金币整数。
    boundYuanbaoUnits: Long; // 十分之一绑定元宝的内部单位。
    materials: readonly { itemId: number; quantity: Long }[]; // 按模板排序的绑定材料。
}

export interface BattlePassTier {
    tier: number; // 免费为一，付费档为二和三。
    totalCashFen: Long; // 此档完整现金价格。
    upgradeCashFen: Long; // 角色当前档升到此档的确切差额。
    daily: BattlePassReward; // 每个合格日的总奖励。
    season: BattlePassReward; // 周期一次的总奖励。
}

export interface BattlePassState {
    sceneEpoch: Long; // 原角色附着的场景编号。
    roleRevision: Long; // 领取时可作为期望修订的原快照。
    ruleVersion: Long; // 当前生效的玩法协议版本。
    seasonStart: string; // 30 天周期的上海起始日。
    seasonEnd: string; // 第一个不再属于该周期的上海日期。
    operatingDay: string; // 服务器当前上海日期。
    tier: number; // 当前最高持有档位。
    seasonClaimedTier: number; // 周期一次奖已发到的档位。
    activity: number; // 本日已确认的任务活跃。
    activityTarget: number; // 原周期冻结的活跃门槛。
    tiers: readonly BattlePassTier[]; // 该周期的完整档位配置。
    days: readonly { day: number; qualified: boolean; claimedTier: number }[]; // 30 日的原资格与最高已领档。
    history: readonly string[]; // 角色已经参加过的历史页。
    historyHasMore: boolean; // 该页是否可向更早周期翻页。
}

export interface BattlePassClaim {
    seasonStart: string; // 原周期身份。
    day: number; // 相对日，零为一次奖。
    tier: number; // 确认时准备领取到的档位。
    claimedTier: number; // 确认时已经领取的原档。
}

export interface BattlePassReceipt {
    seasonStart: string; // 原到账的周期。
    day: number; // 原真实相对日或一次奖。
    previousTier: number; // 原最高已领档。
    tier: number; // 原成功后已发到的档位。
    boundGold: Long; // 本笔实际绑定金币。
    boundYuanbaoUnits: Long; // 本笔实际绑定元宝。
    items: readonly { instanceId: Uint8Array; itemId: number; sourceKind: string; bound: boolean; quantity: Long }[]; // 实际入包实例。
}

// battlePassDate 校验由服务器返回的年月日形状和真实日期。
export function battlePassDate(value: unknown): value is string {
    if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
    const ts = Date.parse(value + 'T00:00:00Z');
    return Number.isFinite(ts) && new Date(ts).toISOString().slice(0, 10) === value;
}

const units = (value: unknown): value is Long => Long.isLong(value) && !value.unsigned && !value.isNegative();
const tierNumber = (value: number, zero = false) => Number.isInteger(value) && value >= (zero ? 0 : 1) && value <= 3;

// validReward 检查四钱包中的绑定单位和唯一材料顺序。
function validReward(reward: BattlePassReward): boolean {
    return !!reward && units(reward.boundGold) && units(reward.boundYuanbaoUnits) && reward.materials?.length <= 8 &&
        reward.materials.every((row, index) => Number.isInteger(row.itemId) && row.itemId > (reward.materials[index - 1]?.itemId ?? 0) &&
            units(row.quantity) && !row.quantity.isZero());
}

// validateBattlePassState 拒收越档、漏日或与原现金差额不符的页面。
export function validateBattlePassState(value: BattlePassState): void {
    if (!value || !Long.isLong(value.roleRevision) || !value.roleRevision.unsigned || value.roleRevision.isZero() ||
        !battlePassDate(value.seasonStart) || !battlePassDate(value.seasonEnd) || !battlePassDate(value.operatingDay) ||
        Date.parse(value.seasonEnd) - Date.parse(value.seasonStart) !== 30 * 86400000 ||
        !tierNumber(value.tier) || !tierNumber(value.seasonClaimedTier, true) || value.seasonClaimedTier > value.tier ||
        !Number.isInteger(value.activityTarget) || value.activityTarget < 1 || !Number.isInteger(value.activity) || value.activity < 0 || value.activity > value.activityTarget ||
        value.tiers?.length !== 3 || value.tiers.some((row, index) => row.tier !== index + 1 || !units(row.totalCashFen) || !units(row.upgradeCashFen) ||
            (index === 0 ? !row.totalCashFen.isZero() : !row.totalCashFen.greaterThan(value.tiers[index - 1].totalCashFen)) ||
            !validReward(row.daily) || !validReward(row.season)) || value.days?.length !== 30 ||
        value.days.some((row, index) => row.day !== index + 1 || !tierNumber(row.claimedTier, true) || row.claimedTier > value.tier || !row.qualified && row.claimedTier !== 0) ||
        value.history?.length > 8 || value.history.some((day, index) => !battlePassDate(day) || index > 0 && day >= value.history[index - 1])) {
        throw new Error('战令周期或奖励配置异常');
    }
    const original = value.tiers[value.tier - 1].totalCashFen;
    for (const row of value.tiers) {
        if (!row.upgradeCashFen.isZero() && (row.tier <= value.tier || !row.upgradeCashFen.equals(row.totalCashFen.subtract(original)))) {
            throw new Error('战令升档差额异常');
        }
    }
}

// battlePassDelta 将同一天已经收到的部分从当前档总奖中扣除。
export function battlePassDelta(state: BattlePassState, day: number, previous: number): BattlePassReward {
    const period = day ? 'daily' : 'season';
    const current = state.tiers[state.tier - 1][period], before = state.tiers[previous - 1]?.[period];
    const result = { boundGold: current.boundGold.subtract(before?.boundGold ?? Long.ZERO),
        boundYuanbaoUnits: current.boundYuanbaoUnits.subtract(before?.boundYuanbaoUnits ?? Long.ZERO),
        materials: current.materials.map(item => ({ ...item, quantity: item.quantity.subtract(before?.materials.find(old => old.itemId === item.itemId)?.quantity ?? Long.ZERO) })) };
    if (!units(result.boundGold) || !units(result.boundYuanbaoUnits) || result.materials.some(item => !units(item.quantity))) throw new Error('战令应领差额异常');
    result.materials = result.materials.filter(item => !item.quantity.isZero());
    return result;
}

// validateBattlePassReceipt 以稳定原操作为依据检查绑定到账，已有确认页时逐项核对。
export function validateBattlePassReceipt(receipt: BattlePassReceipt, request: BattlePassClaim, due?: BattlePassReward): void {
    if (!receipt || receipt.seasonStart !== request.seasonStart || receipt.day !== request.day ||
        receipt.tier !== request.tier || receipt.previousTier !== request.claimedTier || !units(receipt.boundGold) ||
        !units(receipt.boundYuanbaoUnits) || !Array.isArray(receipt.items) || receipt.items.length > 256) throw new Error('战令原领取回执不符');
    const instances = new Set<string>();
    const material = new Map<number, bigint>();
    for (const item of receipt.items) {
        const id = assetId(item.instanceId);
        if (!id || instances.has(id) || !Number.isInteger(item.itemId) || item.itemId < 1 || item.bound !== true ||
            item.sourceKind !== 'battle_pass' || !units(item.quantity) || item.quantity.isZero()) throw new Error('战令绑定材料回执异常');
        instances.add(id);
        material.set(item.itemId, (material.get(item.itemId) ?? 0n) + BigInt(item.quantity.toString()));
    }
    if (material.size === 0 && receipt.boundGold.isZero() && receipt.boundYuanbaoUnits.isZero()) throw new Error('战令到账差额为空');
    if (due && (!due.boundGold.equals(receipt.boundGold) || !due.boundYuanbaoUnits.equals(receipt.boundYuanbaoUnits) ||
        due.materials.length !== material.size || due.materials.some(row => material.get(row.itemId) !== BigInt(row.quantity.toString())))) {
        throw new Error('战令发放金额或材料与原差额不同');
    }
}
