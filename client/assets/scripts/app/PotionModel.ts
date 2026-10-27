export type PotionInteger = string | number | bigint | { toString(): string };

export interface PotionRules {
    hp: number;
    mp: number;
    tick_ms: number;
    base_per_tick: number;
    levels_per_step: number;
    use_delay_ms: number;
    queue_cap: number;
}

export interface PotionDefinition {
    item_id: number;
    name: string;
    kind: string;
    use_enabled?: boolean;
    potion?: PotionRules;
}

export interface PotionItem {
    instanceId: Uint8Array;
    itemId: number;
    quantity: PotionInteger;
    bound: boolean;
    version: PotionInteger;
}

export interface PotionSnapshot {
    ready: boolean;
    roleIdentity: string;
    sceneEpoch: PotionInteger;
    ruleVersion: PotionInteger;
    inventory: { revision: PotionInteger; level: number; items: readonly PotionItem[] } | null;
    definitions: readonly PotionDefinition[];
    player: { hp: PotionInteger; hpMax: PotionInteger; mp: PotionInteger; mpMax: PotionInteger } | null;
}

export interface PotionPreview {
    readonly roleIdentity: string;
    readonly sceneEpoch: string;
    readonly ruleVersion: string;
    readonly inventoryRevision: string;
    readonly instanceHex: string;
    readonly itemId: number;
    readonly itemVersion: string;
    readonly quantity: string;
    readonly level: number;
    readonly title: string;
    readonly message: string;
    readonly requiresConfirmation: boolean;
}

function integer(value: PotionInteger): bigint {
    if (typeof value === 'number' && !Number.isSafeInteger(value)) throw new Error('药品数值必须保持整数精度');
    const text = value.toString();
    if (!/^(?:0|[1-9]\d*)$/.test(text)) throw new Error('药品状态数值无效');
    const number = BigInt(text);
    if (number > 0xffffffffffffffffn) throw new Error('药品状态数值超限');
    return number;
}

function instanceHex(value: Uint8Array): string {
    if (!(value instanceof Uint8Array) || value.length !== 16) throw new Error('药品实例标识无效，请重新同步背包');
    return Array.from(value, byte => byte.toString(16).padStart(2, '0')).join('');
}

function rules(value: PotionRules | undefined): PotionRules {
    const fields: Array<keyof PotionRules> = ['hp', 'mp', 'tick_ms', 'base_per_tick', 'levels_per_step', 'use_delay_ms', 'queue_cap'];
    if (!value || !fields.every(key => Number.isSafeInteger(value[key]) && value[key] >= 0)
        || value.hp + value.mp <= 0 || value.tick_ms <= 0 || value.levels_per_step <= 0 || value.queue_cap <= 0) {
        throw new Error('药品恢复规则尚未准备好');
    }
    return value;
}

// 快捷栏按服务端库存当前顺序选择一个明确实例，不另设绑定优先政策。
export function createPotionPreview(snapshot: PotionSnapshot, itemId: number): PotionPreview {
    if (!snapshot.ready || !snapshot.roleIdentity || !snapshot.inventory || !snapshot.player) throw new Error('场景与背包尚未同步，暂不能使用药品');
    if (integer(snapshot.player.hp) === 0n) throw new Error('角色已倒下，不能使用药品');
    const definition = snapshot.definitions.find(item => item.item_id === itemId && item.kind === 'potion' && item.use_enabled);
    if (!definition) throw new Error('该药品暂未开放使用');
    const effect = rules(definition.potion);
    const item = snapshot.inventory.items.find(entry => entry.itemId === itemId && integer(entry.quantity) > 0n);
    if (!item) throw new Error(`背包中没有可用的${definition.name}`);
    const level = snapshot.inventory.level;
    if (!Number.isSafeInteger(level) || level < 1) throw new Error('角色等级尚未同步');
    const perStep = effect.base_per_tick + Math.floor(level / effect.levels_per_step);
    if (!Number.isSafeInteger(perStep) || perStep <= 0) throw new Error('药品分步恢复规则无效');
    const fullHP = effect.hp > 0 && integer(snapshot.player.hp) >= integer(snapshot.player.hpMax);
    const fullMP = effect.mp > 0 && integer(snapshot.player.mp) >= integer(snapshot.player.mpMax);
    const requiresConfirmation = (effect.hp === 0 || fullHP) && (effect.mp === 0 || fullMP);
    const lines = [
        `${definition.name} · ${item.bound ? '绑定' : '非绑定'}`,
        `所选实例数量：${integer(item.quantity)}，消耗1个`,
        `恢复总量：生命+${effect.hp} / 魔法+${effect.mp}`,
        `每${effect.tick_ms}毫秒分步恢复`,
        `生命、魔法每步各最多${perStep}点`,
    ];
    if (fullHP || fullMP) lines.push(`当前${fullHP && fullMP ? '生命和魔法' : fullHP ? '生命' : '魔法'}已满，仍会消耗1个`);
    lines.push('超出上限的恢复不会返还。确认使用？');
    return Object.freeze({
        roleIdentity: snapshot.roleIdentity,
        sceneEpoch: integer(snapshot.sceneEpoch).toString(),
        ruleVersion: integer(snapshot.ruleVersion).toString(),
        inventoryRevision: integer(snapshot.inventory.revision).toString(),
        instanceHex: instanceHex(item.instanceId), itemId,
        itemVersion: integer(item.version).toString(), quantity: integer(item.quantity).toString(), level,
        title: '使用药品', message: lines.join('\n'), requiresConfirmation,
    });
}

// 确认只接受预览中的原角色、原场景和原实例；状态变化不自动换另一瓶药。
export function validatePotionPreview(preview: PotionPreview, snapshot: PotionSnapshot): void {
    const changed = '药品或场景状态已变化，请重新点击药品预览';
    if (!snapshot.ready || !snapshot.inventory || !snapshot.player || snapshot.roleIdentity !== preview.roleIdentity
        || integer(snapshot.sceneEpoch).toString() !== preview.sceneEpoch
        || integer(snapshot.ruleVersion).toString() !== preview.ruleVersion
        || integer(snapshot.inventory.revision).toString() !== preview.inventoryRevision
        || snapshot.inventory.level !== preview.level) throw new Error(changed);
    const item = snapshot.inventory.items.find(entry => instanceHex(entry.instanceId) === preview.instanceHex);
    if (!item || item.itemId !== preview.itemId || integer(item.version).toString() !== preview.itemVersion
        || integer(item.quantity).toString() !== preview.quantity || integer(item.quantity) === 0n) throw new Error(changed);
    if (!snapshot.definitions.some(item => item.item_id === preview.itemId && item.kind === 'potion' && item.use_enabled)) throw new Error(changed);
    if (integer(snapshot.player.hp) === 0n) throw new Error('角色已倒下，不能使用药品');
}

export function potionInstanceId(preview: PotionPreview): Uint8Array {
    if (!/^[0-9a-f]{32}$/.test(preview.instanceHex)) throw new Error('药品实例标识无效');
    return Uint8Array.from({ length: 16 }, (_value, index) => parseInt(preview.instanceHex.slice(index * 2, index * 2 + 2), 16));
}
