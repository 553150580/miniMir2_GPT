import type { GameConnection, Packet } from './GameConnection';
import { protocolErrorMessage } from './GameErrors';
import { isMiningState, miningAction, miningDurability, MiningModel, orePurityText } from './MiningModel';
import type { MiningScope } from './MiningModel';

const miningIntentId = 20028;
const miningStateId = 20029;

// 矿区借用保存的主操作钮和矿况详情；此控制器只管理权威状态、当前附着和触屏意图。
export class GameMining {
    private readonly model = new MiningModel();
    private readonly unsubscribe: () => void;
    private pending = false;
    private queriedRevision: string | null = null;
    private disposed = false;
    private lastError = '';

    constructor(private readonly connection: Pick<GameConnection, 'request' | 'subscribe'>,
        private readonly scope: () => MiningScope | null, private readonly refreshHUD: () => void,
        private readonly itemName: (id: number) => string | undefined) {
        this.unsubscribe = this.connection.subscribe(this.receive);
    }

    // 角色确认当前地图后才读取状态。普通场景没有矿区查询，也不保存旧矿区结果。
    refresh(): void {
        const scope = this.currentScope();
        if (!scope || this.pending || this.model.state?.active) return;
        const revision = scope.inventoryRevision?.toString() ?? '';
        if (this.queriedRevision === revision) return;
        this.queriedRevision = revision;
        void this.request(scope, miningAction.query).catch(() => { /* 错误在本矿区状态行显示，可轻触核对。 */ });
    }

    get active(): boolean { return !!this.model.state?.active; }
    get canToggle(): boolean { return !this.pending && (!this.model.state || this.model.state.active || this.model.state.canStart); }
    get actionText(): string {
        return this.pending ? '确认中' : this.active ? '停止挖矿' : this.model.state ? '开始挖矿' : '核对挖矿';
    }
    get objective(): string {
        if (this.lastError) return this.lastError;
        const state = this.model.state;
        if (!state) return '正在确认挖矿状态';
        if (state.errorCode) return protocolErrorMessage(miningStateId, state.errorCode);
        const last = state.lastOre;
        if (last) return `${this.active ? '挖矿中' : '已停止'} · ${this.itemName(last.itemId) ?? '矿石'} ${orePurityText(last.orePurity)}`;
        return this.active ? '挖矿中 · 等待出矿' : state.canStart ? '待挖矿 · 轻触开始' : '暂不能挖矿 · 查看矿况';
    }

    details(): string[] {
        const state = this.model.state;
        if (!state) return [this.lastError || '正在确认挖矿状态'];
        const lines = [this.active ? '挖矿中' : state.canStart ? '待挖矿' : '暂不能挖矿',
            `判定 ${state.attempts.toString()} 次 · 出矿 ${state.successes.toString()} 次`];
        if (state.tool) lines.push(this.itemName(state.tool.itemId) ?? '锄头', miningDurability(state.tool));
        if (state.lastOre) {
            const ore = state.lastOre;
            lines.push(`最近出矿 ${this.itemName(ore.itemId) ?? '矿石'}`, orePurityText(ore.orePurity),
                ore.bound ? '绑定矿石' : '非绑定矿石');
        }
        const error = this.lastError || (state.errorCode ? protocolErrorMessage(miningStateId, state.errorCode) : '');
        return [...lines, error].filter(Boolean);
    }

    get toolBlock(): string {
        return this.pending ? '正在确认挖矿状态' : this.active ? '请先停止当前挖矿'
            : !this.model.state?.canStart ? '当前不能开始挖矿' : '';
    }

    async toggle(): Promise<void> {
        const scope = this.currentScope();
        if (!scope || !this.canToggle) return;
        const action = !this.model.state ? miningAction.query : this.active ? miningAction.stop : miningAction.start;
        await this.request(scope, action);
    }

    async useTool(instance: Uint8Array): Promise<void> {
        const scope = this.currentScope();
        if (!scope || this.toolBlock || instance.length !== 16) throw new Error('当前锄头或矿区状态已变化');
        await this.request(scope, miningAction.start, instance);
    }

    private currentScope(): MiningScope | null {
        const scope = this.disposed ? null : this.scope();
        if (this.model.attach(scope)) {
            this.pending = false;
            this.queriedRevision = null;
            this.lastError = '';
        }
        return scope;
    }

    private receive = (packet: Packet): void => {
        if (packet.id !== miningStateId || packet.error) return;
        const scope = this.currentScope();
        if (scope && packet.binding.toString() === scope.binding && this.model.observe(scope, packet.value)) this.refreshHUD();
    };

    private async request(original: MiningScope, action: number, tool?: Uint8Array): Promise<void> {
        if (this.pending) return;
        this.pending = true;
        this.lastError = '';
        this.refreshHUD();
        try {
            const result = await this.connection.request(miningIntentId, {
                sceneEpoch: original.sceneEpoch, action, ...(tool && { toolInstanceId: tool }),
            });
            const current = this.currentScope();
            if (current?.key !== original.key) return;
            if (!isMiningState(result, current)) throw new Error('矿区确认未匹配原场景');
            this.model.observe(current, result);
            if (action !== miningAction.query && result.errorCode) throw new Error(protocolErrorMessage(miningStateId, result.errorCode));
        } catch (error) {
            if (this.currentScope()?.key !== original.key) return;
            this.lastError = error instanceof Error ? error.message : String(error);
            throw error;
        } finally {
            if (this.currentScope()?.key === original.key) {
                this.pending = false;
                this.refreshHUD();
            }
        }
    }

    dispose(): void {
        this.disposed = true;
        this.unsubscribe();
        this.currentScope();
    }
}
