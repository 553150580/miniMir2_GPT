import type { GameConnection, Packet } from './GameConnection';
import { protocolErrorMessage } from './GameErrors';
import type { ItemActionView } from './GameItemActions';
import { isMiningState, miningAction, miningDurability, MiningModel, orePurityText } from './MiningModel';
import type { MiningScope, MiningState } from './MiningModel';

const miningIntentId = 20028;
const miningStateId = 20029;

// 保存策略通过原角色经济请求，矿区控制器只冻结确认时的附着和可选矿种。
interface MiningRecycleCommands {
    canSave(): boolean;
    confirmEnable(names: readonly string[]): Promise<boolean>;
    save(enabled: boolean): Promise<void>;
}

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
        private readonly itemName: (id: number) => string | undefined,
        private readonly recycle?: MiningRecycleCommands) {
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
        if (last) {
            const disposition = state.lastRecycleGold?.isZero() === false || state.lastRecycleBoundGold?.isZero() === false ? ' · 已回收' : '';
            return `${this.active ? '挖矿中' : '已停止'} · ${this.itemName(last.itemId) ?? '矿石'} ${orePurityText(last.orePurity)}${disposition}`;
        }
        return this.active ? '挖矿中 · 等待出矿' : state.canStart ? '待挖矿 · 轻触开始' : '暂不能挖矿 · 查看矿况';
    }

    details(): string[] {
        const state = this.model.state;
        if (!state) return [this.lastError || '正在确认挖矿状态'];
        const lines = [this.active ? '挖矿中' : state.canStart ? '待挖矿' : '暂不能挖矿',
            `判定 ${state.attempts.toString()} 次 · 出矿 ${state.successes.toString()} 次`];
        if (state.tool) lines.push(this.itemName(state.tool.itemId) ?? '锄头', miningDurability(state.tool));
        if (state.recycleItemIds?.length) lines.push(`矿石自动回收 ${state.autoRecycleEnabled ? '已开启' : '已关闭'}`,
            `回收矿种 ${this.recycleNames(state.recycleItemIds).join('、')}`);
        if (state.lastOre) {
            const ore = state.lastOre;
            lines.push(`最近出矿 ${this.itemName(ore.itemId) ?? '矿石'}`, orePurityText(ore.orePurity),
                ore.bound ? '绑定矿石' : '非绑定矿石');
            if (state.lastRecycleGold?.isZero() === false) lines.push(`本次回收 ${state.lastRecycleGold.toString()}金币`);
            if (state.lastRecycleBoundGold?.isZero() === false) lines.push(`本次回收 ${state.lastRecycleBoundGold.toString()}绑定金币`);
        }
        const error = this.lastError || (state.errorCode ? protocolErrorMessage(miningStateId, state.errorCode) : '');
        return [...lines, error].filter(Boolean);
    }

    // 只在服务器提供白名单后打开带操作的矿况；旧规则仍沿用原只读窗口。
    recycleDetails(): (() => ItemActionView | null) | null {
        const original = this.currentScope();
        const commands = this.recycle;
        if (!original || !commands || !this.model.state?.recycleItemIds?.length) return null;
        return () => {
            const current = this.currentScope(), state = this.model.state;
            if (current?.key !== original.key || !state?.recycleItemIds?.length) return null;
            return { title: '矿况', lines: this.details(), actions: [{
                label: state.autoRecycleEnabled ? '关闭回收' : '开启回收',
                enabled: !this.pending && commands.canSave(), run: () => this.saveRecycle(original, state, commands),
            }] };
        };
    }

    private recycleNames(ids: readonly number[]): string[] {
        return ids.map(id => this.itemName(id) ?? `矿石 ${id}`);
    }

    private async saveRecycle(original: MiningScope, state: MiningState, commands: MiningRecycleCommands): Promise<void> {
        const ids = state.recycleItemIds;
        if (!ids?.length) return;
        const enabled = !state.autoRecycleEnabled;
        const stillCurrent = (): boolean => this.currentScope()?.key === original.key && commands.canSave() && !this.pending &&
            this.model.state?.recycleItemIds?.join() === ids.join() && !!this.model.state?.autoRecycleEnabled === !!state.autoRecycleEnabled;
        if (!stillCurrent() || (enabled && !await commands.confirmEnable(this.recycleNames(ids)))) return;
        if (stillCurrent()) await commands.save(enabled);
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
