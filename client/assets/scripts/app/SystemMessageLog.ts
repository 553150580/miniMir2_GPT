import { normalizeGameErrorMessage } from './GameErrors';

export type MessageInteger = string | number | bigint | { toString(): string };

export interface SystemMessage {
    readonly id: number;
    readonly title: string;
    readonly message: string;
}

interface ItemObservation {
    itemId: number;
    quantity?: MessageInteger;
    bound?: boolean;
}

export interface MessageSnapshot {
    roleIdentity: string;
    role?: { name?: string; level?: number } | null;
    inventory?: {
        level?: number;
        experience?: MessageInteger;
        items?: readonly ItemObservation[];
        equipped?: readonly { item?: ItemObservation | null }[];
    } | null;
    wallet?: Partial<Record<'gold' | 'boundGold' | 'yuanbaoUnits' | 'boundYuanbaoUnits', MessageInteger>> | null;
    baseline?: { entities?: readonly { entityHandle: number; entityKind: number; hp?: MessageInteger }[] } | null;
    hosting?: { remainHostingSec?: MessageInteger; autoBattleEnabled?: boolean } | null;
    report?: {
        hostingSessionId?: Uint8Array;
        settlementSequence?: MessageInteger;
        usedSeconds?: MessageInteger;
        kills?: MessageInteger;
        experience?: MessageInteger;
        drops?: readonly ItemObservation[];
    } | null;
    entry?: { sceneEpoch?: MessageInteger; baselineId?: MessageInteger; playerHandle?: number } | null;
    gameplay?: { items?: readonly { item_id: number; name: string }[] } | null;
    map?: { name?: string } | null;
    ready: boolean;
    lastError?: string;
}

function integer(value: MessageInteger | undefined): bigint {
    if (value === undefined) return 0n;
    if (typeof value === 'number' && !Number.isSafeInteger(value)) throw new Error('消息数值必须保持整数精度');
    const text = value.toString();
    if (!/^-?\d+$/.test(text)) throw new Error('消息数值格式无效');
    return BigInt(text);
}

function amount(value: bigint, yuanbao: boolean): string {
    const magnitude = value < 0n ? -value : value;
    const text = yuanbao ? `${magnitude / 10n}.${magnitude % 10n}` : magnitude.toString();
    return value < 0n ? `-${text}` : text;
}

function itemTotals(snapshot: MessageSnapshot): Map<string, { id: number; bound: boolean; count: bigint }> {
    const result = new Map<string, { id: number; bound: boolean; count: bigint }>();
    const owned: ItemObservation[] = Array.from(snapshot.inventory?.items ?? []);
    // 穿戴只改变容器，背包与装备合并观察，避免把换装误报成物品消失。
    for (const equipped of snapshot.inventory?.equipped ?? []) if (equipped.item) owned.push(equipped.item);
    for (const item of owned) {
        const key = `${item.itemId}:${item.bound ? 1 : 0}`;
        const previous = result.get(key);
        result.set(key, { id: item.itemId, bound: !!item.bound, count: (previous?.count ?? 0n) + integer(item.quantity) });
    }
    return result;
}

// 只记录当前会话看到的确定状态变化；首次/重连基线不推算历史收益。
export class SystemMessageLog {
    private entries: SystemMessage[] = [];
    private serial = 0;
    private role = '';
    private binding = '';
    private wasReady = false;
    private entered = false;
    private lastError = '';
    private lastNavigationNotice = '';
    private previousInventory: { level: number; experience: bigint; items: ReturnType<typeof itemTotals> } | null = null;
    private previousWallet: bigint[] | null = null;
    private previousDead: boolean | null = null;
    private previousHosting: { seconds: bigint; auto: boolean } | null = null;
    private readonly seenReports = new Set<string>();
    private change = 0;
    readonly capacity: number;

    constructor(capacity = 100) {
        this.capacity = Math.max(1, Math.min(100, Number.isFinite(capacity) ? Math.floor(capacity) : 100));
    }

    get messages(): readonly SystemMessage[] { return this.entries; }
    get version(): number { return this.change; }

    filter(query: string): readonly SystemMessage[] {
        const needle = query.trim().toLocaleLowerCase();
        return needle ? this.entries.filter(item => `${item.title}\n${item.message}`.toLocaleLowerCase().includes(needle)) : this.entries;
    }
    navigationUnreachable(identity: string): boolean {
        if (!identity || identity === this.lastNavigationNotice) return false;
        this.lastNavigationNotice = identity; this.append('导航', '当前无法到达该位置'); return true;
    }

    observe(snapshot: MessageSnapshot): boolean {
        const start = this.change;
        if (!snapshot.roleIdentity) return false;
        if (this.role !== snapshot.roleIdentity) {
            this.role = snapshot.roleIdentity; this.entries = []; this.change++;
            this.binding = ''; this.entered = false; this.wasReady = false; this.lastError = ''; this.lastNavigationNotice = '';
            this.seenReports.clear(); this.resetObservations();
        }
        const binding = snapshot.entry ? `${snapshot.entry.sceneEpoch ?? ''}:${snapshot.entry.baselineId ?? ''}` : '';
        if (binding && binding !== this.binding) {
            this.binding = binding; this.wasReady = false; this.resetObservations();
        }
        const error = normalizeGameErrorMessage(snapshot.lastError ?? '');
        if (error && error !== this.lastError) this.append('提示', error);
        this.lastError = error;
        if (!snapshot.ready) {
            if (this.wasReady) this.append('连接状态', '等待重新同步角色与场景');
            this.wasReady = false; this.resetObservations();
            return start !== this.change;
        }
        if (!this.wasReady) {
            this.append(this.entered ? '连接已恢复' : '进入地图', this.entered
                ? '角色与场景已重新同步'
                : `${snapshot.role?.name ?? '角色'} · ${snapshot.map?.name ?? '个人地图'}`);
            this.entered = true; this.wasReady = true;
        }
        this.observeInventory(snapshot);
        this.observeWallet(snapshot);
        const player = snapshot.entry?.playerHandle ? snapshot.baseline?.entities?.find(entity => entity.entityHandle === snapshot.entry!.playerHandle) : undefined;
        if (player) {
            const dead = integer(player.hp) <= 0n;
            if (this.previousDead !== null && dead !== this.previousDead) this.append(dead ? '角色倒下' : '角色恢复', dead
                ? '生命值已归零，可查看当前复活选项' : '生命值已恢复，可以继续行动');
            this.previousDead = dead;
        }
        if (snapshot.hosting) {
            const current = { seconds: integer(snapshot.hosting.remainHostingSec), auto: !!snapshot.hosting.autoBattleEnabled };
            if (this.previousHosting) {
                if (current.seconds !== this.previousHosting.seconds) this.append('托管时长更新', `剩余 ${current.seconds} 秒`);
                if (current.auto !== this.previousHosting.auto) this.append('战斗设置', current.auto ? '自动战斗已开启' : '自动战斗已关闭');
            }
            this.previousHosting = current;
        }
        this.observeReport(snapshot);
        return start !== this.change;
    }

    private observeInventory(snapshot: MessageSnapshot): void {
        if (!snapshot.inventory) return;
        const current = { level: snapshot.inventory.level ?? snapshot.role?.level ?? 1,
            experience: integer(snapshot.inventory.experience), items: itemTotals(snapshot) };
        const previous = this.previousInventory;
        if (previous) {
            if (current.level !== previous.level) this.append(current.level > previous.level ? '角色升级' : '等级更新', `${previous.level} 级 → ${current.level} 级`);
            if (current.experience !== previous.experience) {
                const delta = current.experience - previous.experience;
                if (current.level === previous.level && delta > 0n) this.append('经验获得', `经验 +${delta}`);
                else this.append('经验进度', `${current.level} 级，当前经验 ${current.experience}`);
            }
            const keys = new Set(Array.from(previous.items.keys()).concat(Array.from(current.items.keys())));
            for (const key of keys) {
                const before = previous.items.get(key), after = current.items.get(key);
                const delta = (after?.count ?? 0n) - (before?.count ?? 0n);
                if (!delta) continue;
                const item = after ?? before!;
                const name = snapshot.gameplay?.items?.find(row => row.item_id === item.id)?.name ?? `物品 ${item.id}`;
                this.append(delta > 0n ? '物品增加' : '物品减少', `${name} ×${delta > 0n ? delta : -delta}（${item.bound ? '绑定' : '非绑定'}）`);
            }
        }
        this.previousInventory = current;
    }

    private observeWallet(snapshot: MessageSnapshot): void {
        if (!snapshot.wallet) return;
        const fields = ['gold', 'boundGold', 'yuanbaoUnits', 'boundYuanbaoUnits'] as const;
        const names = ['金币', '绑定金币', '元宝', '绑定元宝'];
        const current = fields.map(field => integer(snapshot.wallet![field]));
        if (this.previousWallet) for (let index = 0; index < fields.length; index++) {
            const delta = current[index] - this.previousWallet[index];
            if (!delta) continue;
            this.append('余额更新', `${names[index]} ${delta > 0n ? '+' : ''}${amount(delta, index >= 2)}（当前 ${amount(current[index], index >= 2)}）`);
        }
        this.previousWallet = current;
    }

    private observeReport(snapshot: MessageSnapshot): void {
        const report = snapshot.report;
        if (!report) return;
        const used = integer(report.usedSeconds), kills = integer(report.kills), experience = integer(report.experience);
        if (!used && !kills && !experience && !report.drops?.length) return;
        const session = Array.from(report.hostingSessionId ?? [], byte => byte.toString(16).padStart(2, '0')).join('');
        const key = `${session}:${report.settlementSequence ?? ''}:${used}:${kills}:${experience}`;
        if (this.seenReports.has(key)) return;
        this.seenReports.add(key);
        if (this.seenReports.size > this.capacity) this.seenReports.delete(this.seenReports.values().next().value!);
        this.append('托管战报', `托管 ${used} 秒，击杀 ${kills}，经验 ${experience}`);
    }

    private resetObservations(): void {
        this.previousInventory = null; this.previousWallet = null; this.previousDead = null; this.previousHosting = null;
    }

    private append(title: string, message: string): void {
        this.entries.push({ id: ++this.serial, title, message });
        if (this.entries.length > this.capacity) this.entries.splice(0, this.entries.length - this.capacity);
        this.change++;
    }
}
