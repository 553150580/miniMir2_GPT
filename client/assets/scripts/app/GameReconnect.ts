import { Button, EditBox, Label, Node, isValid } from 'cc';
import type { EventTouch } from 'cc';
import { connectRoleEntry, isEntryTicketError, isRoleAccountAuthenticationError } from './RoleEntry';
import { currentEntryIdentity, isCurrentEntry, roleIdentity, session } from './GameSession';
import type { AttachmentIdentity } from './MapTravelModel';

export function accountIdentity(id: Uint8Array): string {
    return Array.from(id, value => value.toString(16).padStart(2, '0')).join('');
}

export class GameReconnect {
    private readonly status: Label;
    private readonly password: EditBox;
    private readonly button: Button;
    private readonly buttonLabel: Label;
    private readonly unsubscribe: () => void;
    private readonly account = session.account;
    private readonly server = session.originServer;
    private readonly role = session.role;
    private readonly identity = roleIdentity();
    private attempt?: AbortController;
    private needsPassword = false;
    private returning = false;
    private disposed = false;

    constructor(private readonly root: Node, private readonly worldReady: () => boolean, private readonly offline: () => void,
        private readonly attach: () => Promise<unknown>, private readonly exitGame: () => Promise<void>, private readonly tap: () => void) {
        this.status = root.getChildByPath('Panel/Message')!.getComponent(Label)!;
        this.password = root.getChildByPath('Panel/Password')!.getComponent(EditBox)!;
        this.button = root.getChildByPath('Panel/Reconnect')!.getComponent(Button)!;
        this.buttonLabel = this.button.node.getChildByName('Label')!.getComponent(Label)!;
        this.button.node.on(Button.EventType.CLICK, this.retry, this);
        this.root.on(Node.EventType.TOUCH_END, this.returnFromBackdrop, this);
        this.unsubscribe = session.connection.onStatus(this.disconnected);
        if (session.connection.disconnectReason) this.disconnected(session.connection.disconnectReason);
    }

    dispose(): void {
        this.disposed = true;
        this.unsubscribe();
        this.attempt?.abort();
        if (isValid(this.button.node, true)) this.button.node.off(Button.EventType.CLICK, this.retry, this);
        if (isValid(this.root, true)) this.root.off(Node.EventType.TOUCH_END, this.returnFromBackdrop, this);
    }

    private disconnected = (message: string): void => {
        if (this.returning) return;
        this.offline();
        this.root.active = true;
        this.showStatus(this.attempt ? '重连期间连接中断，请重试。' : `${message}，请点击重新连接。`);
    };

    private returnFromBackdrop = (event: EventTouch): void => {
        if (event.target !== this.root || !this.root.active || this.returning || this.disposed) return;
        event.propagationStopped = true;
        this.returning = true;
        this.attempt?.abort();
        this.attempt = undefined;
        this.password.string = '';
        this.password.node.active = false;
        this.button.interactable = false;
        this.tap();
        this.status.string = session.connection.token ? '正在返回选角…' : '正在返回登录…';
        void this.exitGame().catch(error => {
            if (this.disposed) return;
            this.returning = false;
            this.password.node.active = this.needsPassword;
            this.button.interactable = true;
            this.buttonLabel.string = '重新连接';
            this.showStatus(error instanceof Error ? error.message : '返回失败，请重试。');
        });
    };

    private showStatus(message: string): void {
        this.status.string = `${message}\n轻触背景可返回${session.connection.token ? '选角' : '登录'}。`;
    }

    private retry = (): void => {
        if (!this.root.active || this.attempt || this.returning || this.disposed) return;
        this.tap();
        const password = this.needsPassword ? this.password.string : '';
        if (this.needsPassword && !/^[A-Za-z0-9_]{6,20}$/.test(password)) {
            this.showStatus('请输入当前账号的密码。');
            return;
        }
        const attempt = new AbortController();
        this.attempt = attempt;
        this.password.string = '';
        this.password.node.active = false;
        this.button.interactable = false;
        this.buttonLabel.string = '连接中';
        void this.connect(attempt, password).finally(() => {
            if (this.attempt !== attempt) return;
            this.attempt = undefined;
            if (this.disposed) return;
            this.password.node.active = this.needsPassword;
            this.button.interactable = true;
            this.buttonLabel.string = '重新连接';
        });
    };

    private async connect(attempt: AbortController, password: string): Promise<void> {
        const connection = session.connection;
        let token = connection.token;
        let generation: number | undefined;
        const scopeIsCurrent = (): boolean => !this.disposed && this.attempt === attempt && session.account === this.account
            && session.originServer === this.server && session.role === this.role && roleIdentity() === this.identity && connection.token === token;
        const requireScope = (): void => {
            if (!scopeIsCurrent() || attempt.signal.aborted) throw new Error('重连身份已失效');
        };
        const deadline = setTimeout(() => attempt.abort(), 35000);
        try {
            requireScope();
            if (!this.account) throw new Error('缺少当前账号，请重新登录');
            if (!this.server?.originServerId.equals(this.role.key.originServerId)) throw new Error('当前区服与角色归属不匹配');
            if (this.needsPassword) {
                this.showStatus('正在验证当前账号…');
                const verified = await connection.rpc(10001, { account: this.account.name, password }, attempt.signal);
                requireScope();
                if (accountIdentity(verified.accountId) !== this.account.id) throw new Error('账号身份已变化');
                connection.token = token = verified.accountToken;
                this.needsPassword = false;
            }
            this.showStatus('正在认证并连接原角色…');
            generation = await connectRoleEntry(connection, this.role.key, requireScope, attempt.signal);
            const identity = currentEntryIdentity();
            if (!identity || identity.connection !== generation || !isCurrentEntry(identity)) throw new Error('未收到有效的角色入场回执');
            this.showStatus('正在同步当前地图…');
            await this.waitForWorld(attempt.signal, identity);
            requireScope();
            session.lastError = '';
            this.root.active = false;
        } catch (error) {
            if (!scopeIsCurrent()) return;
            if (generation !== undefined && connection.generation === generation) connection.close();
            this.offline();
            if (isRoleAccountAuthenticationError(error)) {
                this.needsPassword = true;
                this.showStatus('登录凭证已失效，请输入当前账号的密码。');
            } else if (isEntryTicketError(error)) {
                this.needsPassword = true;
                this.showStatus('重新取票后仍未能进入，请输入当前账号的密码重新认证。');
            } else {
                const message = attempt.signal.aborted ? '重连超时' : (error instanceof Error ? error.message : '重连失败');
                this.showStatus(`${message}，请重试。`);
            }
        } finally { clearTimeout(deadline); }
    }

    // 同一连接可先确认个人图，再收到服务端恢复的首领场基线。每次只等待当前场景。
    private waitForWorld(signal: AbortSignal, initial: AttachmentIdentity): Promise<void> {
        return new Promise((resolve, reject) => {
            let finished = false;
            let loading: AttachmentIdentity | undefined;
            let mounted: AttachmentIdentity | undefined;
            const finish = (error?: unknown): void => {
                if (finished) return;
                finished = true;
                session.update.delete(check);
                signal.removeEventListener('abort', abort);
                if (error) reject(error); else resolve();
            };
            const abort = (): void => finish(new Error('重连同步超时'));
            const check = (): void => {
                if (finished) return;
                const entry = currentEntryIdentity();
                if (!session.role || !entry || entry.connection !== initial.connection || entry.role !== initial.role ||
                    entry.binding.toString() !== initial.binding.toString() || !isCurrentEntry(entry)) {
                    finish(new Error('地图同步期间连接中断'));
                    return;
                }
                if (mounted && isCurrentEntry(mounted)) {
                    if (session.ready && this.worldReady()) finish();
                    return;
                }
                if (loading && isCurrentEntry(loading)) return;
                loading = entry;
                void Promise.resolve().then(() => {
                    if (!finished && isCurrentEntry(entry)) return this.attach();
                }).then(() => {
                    if (finished || loading !== entry) return;
                    loading = undefined;
                    if (isCurrentEntry(entry)) mounted = entry;
                    check();
                }, error => {
                    if (finished || loading !== entry) return;
                    loading = undefined;
                    if (isCurrentEntry(entry)) finish(error);
                    else check();
                });
            };
            if (signal.aborted) { abort(); return; }
            session.update.add(check);
            signal.addEventListener('abort', abort, { once: true });
            check();
        });
    }
}
