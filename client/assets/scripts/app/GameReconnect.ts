import { Button, EditBox, Label, Node, isValid } from 'cc';
import { CodeUnauthenticated } from '../generated/protocol_ids';
import { ProtocolResponseError } from './GameConnection';
import { currentEntryIdentity, isCurrentEntry, roleIdentity, session } from './GameSession';

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
    private readonly role = session.role;
    private readonly identity = roleIdentity();
    private attempt?: AbortController;
    private needsPassword = false;
    private disposed = false;

    constructor(private readonly root: Node, private readonly worldReady: () => boolean, private readonly offline: () => void,
        private readonly attach: () => Promise<unknown>, private readonly tap: () => void) {
        this.status = root.getChildByPath('Panel/Message')!.getComponent(Label)!;
        this.password = root.getChildByPath('Panel/Password')!.getComponent(EditBox)!;
        this.button = root.getChildByPath('Panel/Reconnect')!.getComponent(Button)!;
        this.buttonLabel = this.button.node.getChildByName('Label')!.getComponent(Label)!;
        this.button.node.on(Button.EventType.CLICK, this.retry, this);
        this.unsubscribe = session.connection.onStatus(this.disconnected);
        if (session.connection.disconnectReason) this.disconnected(session.connection.disconnectReason);
    }

    dispose(): void {
        this.disposed = true;
        this.unsubscribe();
        this.attempt?.abort();
        if (isValid(this.button.node, true)) this.button.node.off(Button.EventType.CLICK, this.retry, this);
    }

    private disconnected = (message: string): void => {
        this.offline();
        this.root.active = true;
        this.status.string = this.attempt ? '重连期间连接中断，请重试。' : `${message}，请点击重新连接。`;
    };

    private retry = (): void => {
        if (!this.root.active || this.attempt || this.disposed) return;
        this.tap();
        const password = this.needsPassword ? this.password.string : '';
        if (this.needsPassword && !/^[A-Za-z0-9_]{6,20}$/.test(password)) {
            this.status.string = '请输入当前账号的密码。';
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
            && session.role === this.role && roleIdentity() === this.identity && connection.token === token;
        const requireScope = (): void => {
            if (!scopeIsCurrent() || attempt.signal.aborted) throw new Error('重连身份已失效');
        };
        const deadline = setTimeout(() => attempt.abort(), 35000);
        try {
            requireScope();
            if (!this.account) throw new Error('缺少当前账号，请重新登录');
            if (this.needsPassword) {
                this.status.string = '正在验证当前账号…';
                const verified = await connection.rpc(10001, { account: this.account.name, password }, attempt.signal);
                requireScope();
                if (accountIdentity(verified.accountId) !== this.account.id) throw new Error('账号身份已变化');
                connection.token = token = verified.accountToken;
                this.needsPassword = false;
            }
            this.status.string = '正在认证并连接原角色…';
            const ticket = await connection.rpc(10007, { role: this.role.key }, attempt.signal);
            requireScope();
            // 取票始终使用当前角色的原区服号和角色ID，不复用旧连接的绑定或短票。
            const entry = connection.connect(ticket.ticket, attempt.signal);
            generation = connection.generation;
            await entry;
            requireScope();
            const identity = currentEntryIdentity();
            if (!identity || identity.connection !== generation || !isCurrentEntry(identity)) throw new Error('未收到有效的角色入场回执');
            this.status.string = '正在同步当前地图…';
            await this.waitForWorld(attempt.signal, () => isCurrentEntry(identity));
            requireScope();
            session.lastError = '';
            this.root.active = false;
        } catch (error) {
            if (!scopeIsCurrent()) return;
            if (generation !== undefined && connection.generation === generation) connection.close();
            this.offline();
            if (error instanceof ProtocolResponseError && error.messageId === 10008 && error.code === CodeUnauthenticated) {
                this.needsPassword = true;
                this.status.string = '登录凭证已过期，请输入当前账号的密码。';
            } else {
                const message = attempt.signal.aborted ? '重连超时' : (error instanceof Error ? error.message : '重连失败');
                this.status.string = `${message}，请重试。`;
            }
        } finally { clearTimeout(deadline); }
    }

    private waitForWorld(signal: AbortSignal, entryCurrent: () => boolean): Promise<void> {
        return new Promise((resolve, reject) => {
            const finish = (error?: unknown): void => {
                session.update.delete(check);
                signal.removeEventListener('abort', abort);
                if (error) reject(error); else resolve();
            };
            const abort = (): void => finish(new Error('重连同步超时'));
            let attached = false;
            const check = (): void => {
                if (!entryCurrent()) finish(new Error('地图同步期间连接中断'));
                else if (attached && session.ready && this.worldReady()) finish();
            };
            if (signal.aborted) { abort(); return; }
            session.update.add(check);
            signal.addEventListener('abort', abort, { once: true });
            void this.attach().then(() => { attached = true; check(); }, finish);
        });
    }
}
