import Long from 'long';
import { newOperationId } from './GameConnection';
import { protocolErrorMessage } from './GameErrors';
import type { ItemAction, ItemActionView } from './GameItemActions';

interface AdRole {
    readonly roleId: Uint8Array; // 永久角色编号。
    readonly originServerId: Long; // 创建此角色的原入口服。
}

interface AdContext {
    readonly scope: string; // 原账号、角色及场景附着。
    readonly role: AdRole; // 发给账号服核对的原角色。
}

interface AdRequest {
    readonly role: AdRole; // 当前账号验证通过的原受益角色。
    readonly requestId?: Uint8Array; // 首次发起和未知结果重试使用同一编号。
    readonly attemptId?: Uint8Array; // 重查服务端原观看凭据。
}

enum AdStage { None, Awaiting, Verified, Granted, Denied, Expired }

interface AdReceipt {
    readonly attemptId: Uint8Array; // 实际到账对应的观看凭据。
    readonly grantedSeconds: number; // 本次原奖励的增加量。
    readonly balanceSeconds: Long; // 本次到账时保存的余额。
    readonly operatingDay: string; // 原播放完成的上海日期。
}

interface AdState {
    readonly role: AdRole; // 账号服返回的原角色。
    readonly rewardSeconds: number; // 新观看适用的到账秒数。
    readonly accountDailyLimit: number; // 当日账号总限量。
    readonly accountRemaining: number; // 账号当日剩余机会。
    readonly channelAvailable: boolean; // 账号服已装配可信渠道。
    readonly otherRolePending: boolean; // 该账号有另一角色正在等待原结果。
    readonly stage: AdStage; // 从原服务端原凭据读取的阶段。
    readonly resultCode: number; // 永久拒绝或超时原因。
    readonly attemptId: Uint8Array; // 服务端固定的观看凭据。
    readonly provider: string; // 当前原凭据使用的渠道。
    readonly placement: string; // 渠道的激励广告位。
    readonly beneficiary: string; // 不向渠道暴露原账号的受益标记。
    readonly watchUntilUnixMs: Long; // 原视频的最晚播放时间。
    readonly receipt?: AdReceipt; // 只有服务端已经确认入账时存在。
}

interface Playback {
    readonly provider: string; // 宿主通过此标识选择对应广告 SDK。
    readonly placement: string; // 待播放的渠道广告位。
    readonly beneficiary: string; // 原角色在渠道上的绑定标记。
    readonly attemptId: string; // 由渠道原样返回给账号服的观看凭据。
}

interface PlaybackBridge {
    play(terms: Playback): Promise<void>; // 宿主播放并提交给渠道，界面只等待服务端结果。
}

interface AdActions {
    context(): AdContext | undefined;
    request(value: AdRequest, signal: AbortSignal): Promise<AdState>;
    show(read: () => ItemActionView | null): void;
    current(read: () => ItemActionView | null): boolean;
    refresh(): void;
}

const checkDelays = [300, 750, 1250, 2000, 3000]; // 一次观看结束后只做这一组有界查证。

// bridge 在宿主接入渠道后可用，普通开发网页默认没有这项能力。
function bridge(): PlaybackBridge | undefined {
    const value = (globalThis as { minimirRewardedAd?: PlaybackBridge }).minimirRewardedAd;
    return typeof value?.play === 'function' ? value : undefined;
}

// sameBytes 保留旧请求和永久角色的二进制身份。
function sameBytes(a: Uint8Array, b: Uint8Array): boolean {
    return a instanceof Uint8Array && a.length === b.length && a.every((byte, index) => byte === b[index]);
}

// timeText 保留零碎秒数，支持显示超过一天的托管余额。
function timeText(seconds: number): string {
    const hours = Math.floor(seconds / 3600), minutes = Math.floor(seconds / 60) % 60, tail = seconds % 60;
    return [hours && `${hours} 小时`, minutes && `${minutes} 分钟`, tail && `${tail} 秒`].filter(Boolean).join(' ') || '0 秒';
}

// GameRewardedAd 管理托管页的原观看状态和服务器回执。余额同步沿用游戏服的托管消息。
export class GameRewardedAd {
    private read?: () => ItemActionView | null; // 当前使用已保存详情框的原面板。
    private controller?: AbortController; // 打开下一页或退出游戏时结束旧账号请求。
    private state?: AdState; // 服务端刚核对的原观看或到账状态。
    private retry?: { scope: string; id: Uint8Array }; // 账号 RPC 未确认时重试原请求。
    private notice = ''; // 宿主播放或查询当时的可恢复结果。

    constructor(private readonly actions: AdActions) {}

    // open 先显示原角色的最新状态，换服或附着变更后忽略当前页的迟到结果。
    async open(): Promise<void> {
        const context = this.actions.context();
        if (!context) return;
        this.dispose();
        const controller = this.controller = new AbortController();
        this.state = undefined;
        this.notice = '';
        const read = (): ItemActionView | null => this.actions.context()?.scope === context.scope ? this.view(context) : null;
        this.read = read;
        this.actions.show(read);
        await this.load(context, { role: context.role }, controller);
    }

    // dispose 结束账号查询。已经被渠道核验的广告由服务端继续履约。
    dispose(): void {
        this.controller?.abort();
        this.read = undefined;
    }

    private active(context: AdContext, controller: AbortController): boolean {
        return !controller.signal.aborted && this.controller === controller &&
            this.actions.context()?.scope === context.scope && !!this.read && this.actions.current(this.read);
    }

    // load 仅在同一原账号、角色与详情页接收回执。
    private async load(context: AdContext, request: AdRequest, controller: AbortController): Promise<AdState | undefined> {
        if (!this.active(context, controller)) return;
        try {
            const state = await this.actions.request(request, controller.signal);
            if (!this.active(context, controller)) return;
            if (!state?.role || !sameBytes(state.role.roleId, context.role.roleId) ||
                !Long.isLong(state.role.originServerId) || !state.role.originServerId.equals(context.role.originServerId) ||
                !Number.isInteger(state.stage) || state.stage < AdStage.None || state.stage > AdStage.Expired ||
                (state.stage !== AdStage.None && state.attemptId?.length !== 16) ||
                (request.attemptId && !sameBytes(state.attemptId, request.attemptId)) ||
                ((state.stage === AdStage.Granted) !== !!state.receipt) ||
                (state.receipt && (!sameBytes(state.receipt.attemptId, state.attemptId) ||
                    !Long.isLong(state.receipt.balanceSeconds) || state.receipt.balanceSeconds.isNegative() ||
                    state.receipt.balanceSeconds.greaterThan(0xffffffff) || state.receipt.grantedSeconds < 1))) {
                throw new Error('激励视频回执与原角色或观看不符');
            }
            this.state = state;
            this.notice = '';
            this.actions.refresh();
            return state;
        } catch (error) {
            if (this.active(context, controller)) {
                this.notice = error instanceof Error ? error.message : '视频状态暂时无法确认';
                this.actions.refresh();
            }
            return undefined;
        }
    }

    // play 启动真实宿主 SDK。播放 Promise 只是开始查证的信号。
    private async play(context: AdContext, controller: AbortController): Promise<void> {
        const host = bridge();
        if (!host || !this.active(context, controller)) return;
        let state = this.state;
        if (state?.stage !== AdStage.Awaiting) {
            const id = this.retry?.scope === context.scope ? this.retry.id : newOperationId();
            this.retry = { scope: context.scope, id };
            state = await this.load(context, { role: context.role, requestId: id }, controller);
            if (state) this.retry = undefined;
        }
        if (state?.stage !== AdStage.Awaiting || !this.active(context, controller) ||
            !Long.isLong(state.watchUntilUnixMs) || state.watchUntilUnixMs.lessThanOrEqual(Date.now())) return;
        const attemptId = Array.from(state.attemptId, byte => byte.toString(16).padStart(2, '0')).join('');
        let playbackError = false;
        try {
            await host.play({ provider: state.provider, placement: state.placement, beneficiary: state.beneficiary, attemptId });
        } catch {
            playbackError = true;
        }
        for (const delay of checkDelays) {
            if (!this.active(context, controller)) return;
            await new Promise<void>(resolve => setTimeout(resolve, delay));
            const result = await this.load(context, { role: context.role, attemptId: state.attemptId }, controller);
            if (result && result.stage !== AdStage.Awaiting && result.stage !== AdStage.Verified) return;
            if (playbackError && result?.stage === AdStage.Awaiting) {
                this.notice = '播放尚未确认，可继续播放或刷新状态';
                this.actions.refresh();
                return;
            }
        }
    }

    private view(context: AdContext): ItemActionView {
        const state = this.state, controller = this.controller!;
        const lines: string[] = [];
        if (state) {
            lines.push(`每次核验奖励 ${timeText(state.rewardSeconds)}`,
                `账号今日剩余 ${state.accountRemaining}/${state.accountDailyLimit}`);
            if (state.otherRolePending) lines.push('原角色仍在等待视频结果');
            if (!state.channelAvailable || !bridge()) lines.push('当前渠道暂不可播放');
            if (state.stage === AdStage.Awaiting) lines.push('等待渠道核验完播');
            if (state.stage === AdStage.Verified) lines.push('完播已核验，等待原角色到账');
            if (state.resultCode) lines.push(protocolErrorMessage(10014, state.resultCode));
            if (state.receipt) lines.push(`本次到账 ${timeText(state.receipt.grantedSeconds)}`,
                `到账后余额 ${timeText(state.receipt.balanceSeconds.toNumber())}`, `完播运营日 ${state.receipt.operatingDay}`);
        } else if (!this.notice) lines.push('正在读取视频状态');
        if (this.notice) lines.push(this.notice);
        const actions: ItemAction[] = [];
        if (state?.channelAvailable && bridge() && !state.otherRolePending && state.accountRemaining > 0 &&
            state.stage !== AdStage.Verified && (state.stage !== AdStage.Awaiting ||
                Long.isLong(state.watchUntilUnixMs) && state.watchUntilUnixMs.greaterThan(Date.now()))) {
            actions.push({ label: state.stage === AdStage.Awaiting ? '继续播放' : '观看视频', enabled: true,
                run: () => this.play(context, controller) });
        }
        actions.push({ label: '刷新状态', enabled: true, run: async () => {
            await this.load(context, { role: context.role }, controller);
        } });
        return { title: '激励视频', lines, actions };
    }
}
