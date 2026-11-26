import Long from 'long';

// 新矿区使用地图目录和现有地图往返；这些意图不消耗玩家经济命令序号。
export const miningSceneKind = 7;
export const miningAction = { query: 0, start: 1, stop: 2 } as const;

export interface MiningScope {
    readonly key: string;       // 当前连接、角色、场景和基线的完整附着键。
    readonly binding: string;   // 外层数据包的绑定代次。
    readonly sceneEpoch: Long;
    readonly mapId: Long;
    readonly inventoryRevision?: Long; // 停采后补充工具或腾出背包时重新核对资格。
}

export interface MiningItem {
    readonly itemId: number;
    readonly bound?: boolean;
    readonly orePurity?: number;
    readonly durabilityU?: Long;
    readonly maxDurabilityU?: Long;
}

export interface MiningState {
    readonly sceneEpoch: Long;
    readonly mapId: Long;
    readonly stateSequence: Long; // 当前矿区观察的递增水位。
    readonly canStart: boolean;
    readonly active: boolean;
    readonly errorCode: number;
    readonly tool?: MiningItem;
    readonly lastOre?: MiningItem;
    readonly attempts: Long;
    readonly successes: Long;
}

// 只接受当前场景已确认的完整观察，忽略旧图和乱序回包。
export function isMiningState(value: unknown, scope: MiningScope): value is MiningState {
    if (!value || typeof value !== 'object') return false;
    const state = value as MiningState;
    return Long.isLong(state.sceneEpoch) && state.sceneEpoch.equals(scope.sceneEpoch) &&
        Long.isLong(state.mapId) && state.mapId.equals(scope.mapId) &&
        unsignedMiningInteger(state.stateSequence) && !state.stateSequence.isZero() &&
        unsignedMiningInteger(state.attempts) && unsignedMiningInteger(state.successes) &&
        state.successes.lessThanOrEqual(state.attempts) && typeof state.canStart === 'boolean' && typeof state.active === 'boolean' &&
        Number.isInteger(state.errorCode) && state.errorCode >= 0 && state.errorCode <= 0xffffffff &&
        (!state.tool || (validItemId(state.tool.itemId) && unsignedMiningInteger(state.tool.durabilityU) &&
            unsignedMiningInteger(state.tool.maxDurabilityU) && state.tool.durabilityU.lessThanOrEqual(state.tool.maxDurabilityU))) &&
        (!state.lastOre || (validItemId(state.lastOre.itemId) && orePurityText(state.lastOre.orePurity) !== '' &&
            typeof state.lastOre.bound === 'boolean'));
}

function unsignedMiningInteger(value: unknown): value is Long {
    return Long.isLong(value) && value.unsigned;
}

function validItemId(value: number): boolean {
    return Number.isInteger(value) && value > 0 && value <= 0xffffffff;
}

// 旧物品纯度为0或缺省，不凭模板补造矿石属性。
export function orePurityText(purity: number | undefined): string {
    return Number.isInteger(purity) && purity! >= 1 && purity! <= 100 ? `纯度 ${purity}` : '';
}

// 工具按1/1,000持久量变化，矿区明细保留小数，让每次扣减都能核对。
export function miningDurability(item: Pick<MiningItem, 'durabilityU' | 'maxDurabilityU'>): string {
    const { durabilityU: current, maxDurabilityU: maximum } = item;
    if (!unsignedMiningInteger(current) || !unsignedMiningInteger(maximum) || maximum.isZero()) return '';
    const exact = (value: Long): string => {
        const units = BigInt(value.toString());
        return `${units / 1000n}.${String(units % 1000n).padStart(3, '0')}`;
    };
    return `锄头持久 ${exact(current)} / ${exact(maximum)}`;
}

// 当前完整矿区观察只接受单调递增，一次附着不会继承前一座矿或前一次会话的水位。
export class MiningModel {
    private key = '';
    state: MiningState | null = null;

    attach(scope: MiningScope | null): boolean {
        const key = scope?.key ?? '';
        if (key === this.key) return false;
        this.key = key;
        this.state = null;
        return true;
    }

    observe(scope: MiningScope, value: unknown): boolean {
        if (scope.key !== this.key || !isMiningState(value, scope) ||
            this.state && value.stateSequence.lessThanOrEqual(this.state.stateSequence)) return false;
        this.state = value;
        return true;
    }
}
